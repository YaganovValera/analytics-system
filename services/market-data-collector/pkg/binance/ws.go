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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/YaganovValera/analytics-system/common/backoff"
	"github.com/YaganovValera/analytics-system/common/logger"
)

/*
   ============================================================================
   Prometheus-метрики
   ============================================================================
*/

var (
	serviceLabel = "unknown" // переопределяется через SetServiceLabel()

	wsConnects = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "collector", Subsystem: "binance_ws", Name: "connects_total",
			Help: "Total WebSocket connection attempts",
		},
		[]string{"service"},
	)
	wsConnectErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "collector", Subsystem: "binance_ws", Name: "connect_errors_total",
			Help: "Total WebSocket connection errors on first try",
		},
		[]string{"service"},
	)
	wsReconnects = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "collector", Subsystem: "binance_ws", Name: "reconnects_total",
			Help: "Total WebSocket reconnections after read-loop failures",
		},
		[]string{"service"},
	)
	wsMessages = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "collector", Subsystem: "binance_ws", Name: "messages_received_total",
			Help: "Total messages received from WebSocket",
		},
		[]string{"service"},
	)
	wsSubscribeErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "collector", Subsystem: "binance_ws", Name: "subscribe_errors_total",
			Help: "Total subscription errors",
		},
		[]string{"service"},
	)
	wsReadErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "collector", Subsystem: "binance_ws", Name: "read_errors_total",
			Help: "Total read errors from WebSocket",
		},
		[]string{"service"},
	)
	wsPingErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "collector", Subsystem: "binance_ws", Name: "ping_errors_total",
			Help: "Total ping failures",
		},
		[]string{"service"},
	)
	wsBufferDrops = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "collector", Subsystem: "binance_ws", Name: "buffer_drops_total",
			Help: "Number of messages dropped because buffer was full",
		},
		[]string{"service"},
	)
)

// SetServiceLabel вызывается из common.InitServiceName().
func SetServiceLabel(name string) { serviceLabel = name }

var tracer = otel.Tracer("binance-ws")

/*
   ============================================================================
   Public API
   ============================================================================
*/

// RawMessage – то, что уйдёт в Processor
type RawMessage struct {
	Data []byte // завёрнутый или «сырой» JSON конкретного события
	Type string // trade | depthUpdate | …
}

// Config описывает коннектор
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
	switch {
	case c.URL == "":
		return fmt.Errorf("binance-ws: URL is required")
	case len(c.Streams) == 0:
		return fmt.Errorf("binance-ws: at least one stream is required")
	default:
		return nil
	}
}

/*
   ============================================================================
   Implementation
   ============================================================================
*/

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
	return &binanceConnector{
		cfg: cfg,
		log: log.Named("binance-ws"),
	}, nil
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

/*
   ---------------------------------------------------------------------------
   Core loop
   ---------------------------------------------------------------------------
*/

func (c *binanceConnector) run(ctx context.Context, ch chan<- RawMessage) {
	defer close(ch)

	for {
		// graceful shutdown
		if ctx.Err() != nil || c.closed.Load() {
			c.log.Info("ws: stopping run-loop")
			return
		}

		/* 1. Connect -------------------------------------------------------- */
		wsConnects.WithLabelValues(serviceLabel).Inc()
		ctxConn, span := tracer.Start(ctx, "WS.Connect",
			trace.WithAttributes(attribute.String("url", c.cfg.URL)))
		conn, err := c.connect(ctxConn)
		span.End()
		if err != nil {
			wsConnectErrors.WithLabelValues(serviceLabel).Inc()
			c.log.Error("ws: connect failed", zap.Error(err))
			return
		}
		c.mu.Lock()
		c.conn = conn
		c.mu.Unlock()
		c.log.Info("ws: connected", zap.String("url", c.cfg.URL))

		/* 2. Pinger --------------------------------------------------------- */
		c.mu.Lock()
		c.cancelPing = c.startPinger(ctx, conn)
		c.mu.Unlock()

		/* 3. Subscribe ------------------------------------------------------ */
		ctxSub, spanSub := tracer.Start(ctx, "WS.Subscribe")
		err = backoff.Execute(ctxSub, c.cfg.BackoffConfig, c.log,
			func(ctx context.Context) error { return c.subscribe(ctx, conn) })
		spanSub.End()
		if err != nil {
			wsSubscribeErrors.WithLabelValues(serviceLabel).Inc()
			c.log.Error("ws: subscribe failed", zap.Error(err))
			_ = conn.Close()
			continue
		}

		/* 4. Read-loop ------------------------------------------------------ */
		ctxRead, spanRead := tracer.Start(ctx, "WS.ReadLoop")
		if err := c.readLoop(ctxRead, conn, ch); err != nil {
			wsReadErrors.WithLabelValues(serviceLabel).Inc()
			spanRead.RecordError(err)
			c.log.Warn("ws: read-loop error, reconnecting", zap.Error(err))
			wsReconnects.WithLabelValues(serviceLabel).Inc()
		}
		spanRead.End()

		/* cleanup & reconnect */
		c.cancelPing()
		_ = conn.Close()
	}
}

/*
   ---------------------------------------------------------------------------
   Helpers
   ---------------------------------------------------------------------------
*/

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
					wsPingErrors.WithLabelValues(serviceLabel).Inc()
					c.log.Warn("ws: ping failed", zap.Error(err))
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

/*
   ---------------------------------------------------------------------------
   The **fixed** read-loop
   ---------------------------------------------------------------------------
*/

// There are two возможных формата сообщений:
//   1) «Raw Stream»: при соединении через `/ws` + SUBSCRIBE (combined = false)
//        {... "e":"trade", "E":..., ... }                     ← нет wrapper-а
//   2) «Combined Stream»: если combined=true или изначально `/stream?...`
//        { "stream":"btcusdt@trade", "data": { ... } }        ← есть wrapper
//
// Старая версия кода обрабатывала **только** (2) => все реальные события
// от Binance («raw-stream») отметались как `unknown`, поэтому в Kafka ничего
// не летело.  Ниже – универсальная обработка обоих вариантов.

func (c *binanceConnector) readLoop(ctx context.Context, conn *websocket.Conn, out chan<- RawMessage) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		_, rawBytes, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		wsMessages.WithLabelValues(serviceLabel).Inc()

		// Пытаемся распознать wrapper.
		var wrapper struct {
			Stream string          `json:"stream"` // btcusdt@trade …
			Data   json.RawMessage `json:"data"`   // реальное событие
		}
		_ = json.Unmarshal(rawBytes, &wrapper) // ошибки игнорируем – просто значит raw-stream

		eventBytes := rawBytes
		if len(wrapper.Data) != 0 {
			eventBytes = wrapper.Data // это combined-stream
		}

		// Выясняем тип события (если сможем)
		var meta struct {
			Event string `json:"e"` // trade | depthUpdate | kline | …
		}
		_ = json.Unmarshal(eventBytes, &meta)
		msgType := meta.Event
		if msgType == "" {
			msgType = "unknown"
		}

		// неблокирующая отправка с дропом – как и раньше
		select {
		case out <- RawMessage{Data: eventBytes, Type: msgType}:
		default:
			wsBufferDrops.WithLabelValues(serviceLabel).Inc()
			c.log.Warn("ws: buffer full, dropping message", zap.String("type", msgType))
		}
	}
}
