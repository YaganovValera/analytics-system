package binance

import (
	"context"
	"encoding/json"
	"fmt"
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
	// Prometheus metrics
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

// RawMessage carries unmodified JSON payload and its event type.
type RawMessage struct {
	Data []byte // JSON bytes
	Type string // event type, e.g. "trade" or "depthUpdate"
}

// Config defines WebSocket connection settings.
type Config struct {
	URL              string         // e.g. "wss://stream.binance.com:9443/ws"
	Streams          []string       // e.g. ["btcusdt@trade","btcusdt@depth"]
	BufferSize       int            // channel buffer size
	ReadTimeout      time.Duration  // pong wait timeout
	SubscribeTimeout time.Duration  // write deadline for SUBSCRIBE
	BackoffConfig    backoff.Config // retry settings
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

// Connector manages a Binance WebSocket connection with auto-reconnect.
type Connector struct {
	cfg         Config
	log         *logger.Logger
	subscribeID uint64
}

// NewConnector constructs a Connector, applies defaults and validates.
func NewConnector(cfg Config, log *logger.Logger) (*Connector, error) {
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &Connector{
		cfg: cfg,
		log: log.Named("binance-ws"),
	}, nil
}

// Stream begins listening to the WebSocket and sends RawMessages to the returned channel.
func (c *Connector) Stream(ctx context.Context) (<-chan RawMessage, error) {
	ch := make(chan RawMessage, c.cfg.BufferSize)
	go c.run(ctx, ch)
	return ch, nil
}

func (c *Connector) run(ctx context.Context, ch chan<- RawMessage) {
	defer close(ch)

	for {
		if err := ctx.Err(); err != nil {
			c.log.Info("ws: context cancelled, stopping")
			return
		}

		// --- Connect with retry ---
		wsConnects.Inc()
		ctxConn, spanConn := tracer.Start(ctx, "WS.Connect",
			trace.WithAttributes(attribute.String("url", c.cfg.URL)))
		conn, err := c.connect(ctxConn)
		spanConn.End()

		if err != nil {
			wsConnectErrors.Inc()
			wsReconnects.Inc()
			c.log.Error("ws: connect failed", zap.Error(err))
			continue
		}
		c.log.Info("ws: connected", zap.String("url", c.cfg.URL))

		// --- Start pinging ---
		cancelPing := c.startPinger(ctx, conn)

		// --- Subscribe ---
		ctxSub, spanSub := tracer.Start(ctx, "WS.Subscribe")
		if err := c.subscribe(ctxSub, conn); err != nil {
			wsSubscribeErrors.Inc()
			spanSub.RecordError(err)
			spanSub.End()
			cancelPing()
			conn.Close()
			continue
		}
		spanSub.End()

		// --- Read loop ---
		ctxRead, spanRead := tracer.Start(ctx, "WS.ReadLoop")
		if err := c.readLoop(ctxRead, conn, ch); err != nil {
			wsReadErrors.Inc()
			spanRead.RecordError(err)
			c.log.Warn("ws: read loop error, reconnecting", zap.Error(err))
		}
		spanRead.End()

		cancelPing()
		conn.Close()
		wsReconnects.Inc()
	}
}

func (c *Connector) connect(ctx context.Context) (*websocket.Conn, error) {
	var conn *websocket.Conn
	op := func(ctx context.Context) error {
		var err error
		conn, _, err = websocket.DefaultDialer.DialContext(ctx, c.cfg.URL, nil)
		return err
	}
	if err := backoff.Execute(ctx, c.cfg.BackoffConfig, c.log, op); err != nil {
		return nil, err
	}
	return conn, nil
}

func (c *Connector) startPinger(ctx context.Context, conn *websocket.Conn) context.CancelFunc {
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
				conn.SetWriteDeadline(time.Now().Add(time.Second))
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second)); err != nil {
					wsPingErrors.Inc()
					c.log.Warn("ws: ping failed", zap.Error(err))
				}
			}
		}
	}()

	return cancel
}

func (c *Connector) subscribe(ctx context.Context, conn *websocket.Conn) error {
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

func (c *Connector) readLoop(ctx context.Context, conn *websocket.Conn, ch chan<- RawMessage) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
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
			// **Reintroduced** BufferDrops to track dropped messages
			wsBufferDrops.Inc()
			c.log.Warn("ws: buffer full, dropping message", zap.String("type", msgType))
		}
	}
}
