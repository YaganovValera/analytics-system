package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/YaganovValera/analytics-system/common/backoff"
	"github.com/YaganovValera/analytics-system/common/logger"
)

// -----------------------------------------------------------------------------
// Prometheus-метрики
// -----------------------------------------------------------------------------

var (
	serviceLabel = "unknown"
	wsConnects   = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "binance_ws", Name: "connects_total",
		Help: "Total WebSocket connection attempts",
	}, []string{"service"})
	wsConnectErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "binance_ws", Name: "connect_errors_total",
		Help: "Total initial connection errors",
	}, []string{"service"})
	wsReconnects = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "binance_ws", Name: "reconnects_total",
		Help: "Total reconnections after read failures",
	}, []string{"service"})
	wsMessages = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "binance_ws", Name: "messages_received_total",
		Help: "Total messages received from WebSocket",
	}, []string{"service"})
	wsSubscribeErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "binance_ws", Name: "subscribe_errors_total",
		Help: "Errors during SUBSCRIBE",
	}, []string{"service"})
	wsReadErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "binance_ws", Name: "read_errors_total",
		Help: "Errors during ReadLoop",
	}, []string{"service"})
	wsPingErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "binance_ws", Name: "ping_errors_total",
		Help: "WebSocket ping failures",
	}, []string{"service"})
	wsBufferDrops = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "binance_ws", Name: "buffer_drops_total",
		Help: "Dropped frames due to full buffer",
	}, []string{"service"})
)

func SetServiceLabel(name string) { serviceLabel = name }

var tracer = otel.Tracer("binance-ws")

// -----------------------------------------------------------------------------
// Public types
// -----------------------------------------------------------------------------

type RawMessage struct {
	Data []byte // JSON-пейлоад
	Type string // значение поля "e" из JSON: "trade" или "depthUpdate"
}

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
		return fmt.Errorf("binance-ws: URL required")
	}
	if len(c.Streams) == 0 {
		return fmt.Errorf("binance-ws: at least one stream required")
	}
	return nil
}

// -----------------------------------------------------------------------------
// Connector
// -----------------------------------------------------------------------------

type binanceConnector struct {
	cfg         Config
	log         *logger.Logger
	subscribeID uint64

	mu         sync.Mutex
	conn       *websocket.Conn
	cancelPing context.CancelFunc

	closed atomic.Bool
}

func NewConnector(cfg Config, log *logger.Logger) (Connector, error) {
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &binanceConnector{cfg: cfg, log: log.Named("binance-ws")}, nil
}

func (c *binanceConnector) Stream(ctx context.Context) (<-chan RawMessage, error) {
	ch := make(chan RawMessage, c.cfg.BufferSize)
	go c.run(ctx, ch)
	return ch, nil
}

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
		if ctx.Err() != nil || c.closed.Load() {
			c.log.Info("ws: stopping")
			return
		}

		// connect
		wsConnects.WithLabelValues(serviceLabel).Inc()
		ctxConn, spanConn := tracer.Start(ctx, "WS.Connect", trace.WithAttributes(attribute.String("url", c.cfg.URL)))
		conn, err := c.connect(ctxConn)
		spanConn.End()
		if err != nil {
			wsConnectErrors.WithLabelValues(serviceLabel).Inc()
			c.log.Error("connect failed", zap.Error(err))
			return
		}
		c.mu.Lock()
		c.conn = conn
		c.mu.Unlock()
		c.log.Info("connected")

		// pinger
		cancel := c.startPinger(ctx, conn)
		c.mu.Lock()
		c.cancelPing = cancel
		c.mu.Unlock()

		// subscribe
		ctxSub, spanSub := tracer.Start(ctx, "WS.Subscribe")
		err = backoff.Execute(ctxSub, c.cfg.BackoffConfig, c.log, func(ctx context.Context) error {
			return c.subscribe(ctx, conn)
		})
		spanSub.End()
		if err != nil {
			wsSubscribeErrors.WithLabelValues(serviceLabel).Inc()
			c.log.Error("subscribe failed", zap.Error(err))
			cancel()
			_ = conn.Close()
			return
		}

		// read-loop
		ctxRead, spanRead := tracer.Start(ctx, "WS.ReadLoop")
		if err := c.readLoop(ctxRead, conn, ch); err != nil {
			wsReadErrors.WithLabelValues(serviceLabel).Inc()
			spanRead.RecordError(err)
			c.log.Warn("read loop error, reconnecting", zap.Error(err))
			wsReconnects.WithLabelValues(serviceLabel).Inc()
		}
		spanRead.End()

		// cleanup
		cancel()
		_ = conn.Close()
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

	pctx, cancel := context.WithCancel(ctx)
	ticker := time.NewTicker(c.cfg.ReadTimeout / 3)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-pctx.Done():
				return
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(time.Second))
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second)); err != nil {
					wsPingErrors.WithLabelValues(serviceLabel).Inc()
					c.log.Warn("ping failed", zap.Error(err))
				}
			}
		}
	}()
	return cancel
}

func (c *binanceConnector) subscribe(ctx context.Context, conn *websocket.Conn) error {
	if err := ctx.Err(); err != nil {
		return err
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
		_, frame, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		wsMessages.WithLabelValues(serviceLabel).Inc()

		// Если Binance присылает envelope {"stream":..., "data":{...}}
		raw := frame
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(frame, &envelope); err == nil && len(envelope.Data) > 0 {
			raw = envelope.Data
		}

		// Определяем тип по полю "e"
		msgType := "unknown"
		var mtype struct {
			Event string `json:"e"`
		}
		if err := json.Unmarshal(raw, &mtype); err == nil && mtype.Event != "" {
			msgType = mtype.Event
		}

		// Шлём дальше (drop при переполнении)
		select {
		case ch <- RawMessage{Data: raw, Type: msgType}:
		default:
			wsBufferDrops.WithLabelValues(serviceLabel).Inc()
			c.log.Warn("buffer full, dropping", zap.String("type", msgType))
		}
	}
}
