// services/market-data-collector/pkg/binance/ws.go
package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/backoff"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/logger"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

var (
	wsConnects = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "binance_ws", Name: "connects_total",
		Help: "Total WebSocket connection attempts",
	})
	wsConnectErrors = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "binance_ws", Name: "connect_errors_total",
		Help: "Total WebSocket connection errors",
	})
	wsReconnects = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "binance_ws", Name: "reconnects_total",
		Help: "Total WebSocket reconnections",
	})
	wsMessages = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "binance_ws", Name: "messages_received_total",
		Help: "Total messages received from WebSocket",
	})
	wsSubscribeErrors = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "binance_ws", Name: "subscribe_errors_total",
		Help: "Total subscription errors",
	})
	wsReadErrors = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "binance_ws", Name: "read_errors_total",
		Help: "Total read errors from WebSocket",
	})
	wsPingErrors = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "binance_ws", Name: "ping_errors_total",
		Help: "Total ping failures",
	})
	wsBufferDrops = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "binance_ws", Name: "buffer_drops_total",
		Help: "Number of messages dropped because buffer was full",
	})
)

var tracer = otel.Tracer("binance-ws")

// RawMessage несёт JSON-байты и тип события.
type RawMessage struct {
	Data []byte
	Type string
}

// Config задаёт WS параметры.
type Config struct {
	URL              string
	Streams          []string
	BufferSize       int
	ReadTimeout      time.Duration
	SubscribeTimeout time.Duration
	BackoffConfig    backoff.Config
}

func (c *Config) applyDefaults() {
	if c.BufferSize <= 0 {
		c.BufferSize = 100
	}
	if c.ReadTimeout <= 0 {
		c.ReadTimeout = 30 * time.Second
	}
	if c.SubscribeTimeout <= 0 {
		c.SubscribeTimeout = 5 * time.Second
	}
}

func (c *Config) validate() error {
	if c.URL == "" {
		return fmt.Errorf("binance-ws: URL is required")
	}
	if len(c.Streams) == 0 {
		return fmt.Errorf("binance-ws: at least one stream is required")
	}
	return nil
}

// binanceConnector — приватная реализация Connector.
type binanceConnector struct {
	cfg         Config
	log         *logger.Logger
	subscribeID uint64

	mu         sync.Mutex
	conn       *websocket.Conn
	cancelPing context.CancelFunc

	closed atomic.Bool
}

// NewConnector создаёт Connector.
func NewConnector(cfg Config, log *logger.Logger) (Connector, error) {
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &binanceConnector{
		cfg: cfg,
		log: log.Named("binance-ws"),
	}, nil
}

// Stream возвращает канал и запускает работу коннектора.
func (c *binanceConnector) Stream(ctx context.Context) (<-chan RawMessage, error) {
	ch := make(chan RawMessage, c.cfg.BufferSize)
	go c.run(ctx, ch)
	return ch, nil
}

// Close сразу закрывает текущее соединение и пинг-рутину.
func (c *binanceConnector) Close() error {
	c.closed.Store(true)

	c.mu.Lock()
	if c.cancelPing != nil {
		c.cancelPing()
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.mu.Unlock()
	return nil
}

func (c *binanceConnector) run(ctx context.Context, ch chan<- RawMessage) {
	defer close(ch)

	for {
		// 1) Выход, если ctx отменён или Close() вызван
		if ctx.Err() != nil || c.closed.Load() {
			c.log.Info("ws: stopping run loop")
			return
		}

		// 2) Подключение
		wsConnects.Inc()
		ctxConn, spanConn := tracer.Start(ctx, "WS.Connect",
			trace.WithAttributes(attribute.String("url", c.cfg.URL)))
		conn, err := c.connect(ctxConn)
		spanConn.End()

		if err != nil {
			wsConnectErrors.Inc()
			wsReconnects.Inc()
			c.log.Error("ws: connect failed", zap.Error(err))
			// после неудачного backoff (exhausted) прекращаем попытки
			return
		}

		// Сохраняем conn для Close()
		c.mu.Lock()
		c.conn = conn
		c.mu.Unlock()

		c.log.Info("ws: connected", zap.String("url", c.cfg.URL))

		// 3) Ping
		cancelPing := c.startPinger(ctx, conn)
		c.mu.Lock()
		c.cancelPing = cancelPing
		c.mu.Unlock()

		// 4) Subscribe с retry
		ctxSub, spanSub := tracer.Start(ctx, "WS.Subscribe")
		err = backoff.Execute(ctxSub, c.cfg.BackoffConfig, c.log, func(ctx context.Context) error {
			return c.subscribe(ctx, conn)
		})
		spanSub.End()
		if err != nil {
			wsSubscribeErrors.Inc()
			c.log.Error("ws: subscribe failed", zap.Error(err))
			cancelPing()
			_ = conn.Close()
			wsReconnects.Inc()
			// после неудачных попыток подписки прекращаем работу
			return
		}

		// 5) ReadLoop
		ctxRead, spanRead := tracer.Start(ctx, "WS.ReadLoop")
		if err := c.readLoop(ctxRead, conn, ch); err != nil {
			wsReadErrors.Inc()
			spanRead.RecordError(err)
			c.log.Warn("ws: read loop error, reconnecting", zap.Error(err))
		}
		spanRead.End()

		// Cleanup перед новой итерацией
		cancelPing()
		_ = conn.Close()
		wsReconnects.Inc()
	}
}

func (c *binanceConnector) connect(ctx context.Context) (*websocket.Conn, error) {
	var conn *websocket.Conn
	err := backoff.Execute(ctx, c.cfg.BackoffConfig, c.log, func(ctx context.Context) error {
		var err error
		conn, _, err = websocket.DefaultDialer.DialContext(ctx, c.cfg.URL, nil)
		return err
	})
	return conn, err
}

func (c *binanceConnector) startPinger(ctx context.Context, conn *websocket.Conn) context.CancelFunc {
	conn.SetReadDeadline(time.Now().Add(c.cfg.ReadTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(c.cfg.ReadTimeout))
	})

	pingCtx, cancel := context.WithCancel(ctx)
	ticker := time.NewTicker(c.cfg.ReadTimeout / 3)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-pingCtx.Done():
				return
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(1 * time.Second))
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(1*time.Second)); err != nil {
					wsPingErrors.Inc()
					c.log.Warn("ws: ping failed", zap.Error(err))
				}
			}
		}
	}()
	return cancel
}

func (c *binanceConnector) subscribe(ctx context.Context, conn *websocket.Conn) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	id := atomic.AddUint64(&c.subscribeID, 1)
	req := map[string]interface{}{
		"method": "SUBSCRIBE",
		"params": c.cfg.Streams,
		"id":     id,
	}
	conn.SetWriteDeadline(time.Now().Add(c.cfg.SubscribeTimeout))
	return conn.WriteJSON(req)
}

func (c *binanceConnector) readLoop(ctx context.Context, conn *websocket.Conn, ch chan<- RawMessage) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_, bytes, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		wsMessages.Inc()

		var env struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(bytes, &env); err != nil {
			c.log.Warn("ws: invalid envelope", zap.Error(err))
			continue
		}

		msgType := "unknown"
		var meta struct {
			Event string `json:"e"`
		}
		if err := json.Unmarshal(env.Data, &meta); err == nil && meta.Event != "" {
			msgType = meta.Event
		}

		select {
		case ch <- RawMessage{Data: env.Data, Type: msgType}:
		default:
			wsBufferDrops.Inc()
			c.log.Warn("ws: buffer full, dropping message", zap.String("type", msgType))
		}
	}
}
