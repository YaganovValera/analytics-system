// market-data-collector/pkg/binance/ws.go
package binance

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/YaganovValera/analytics-system/common/backoff"
	"github.com/YaganovValera/analytics-system/common/logger"
)

type connector struct {
	cfg         Config
	log         *logger.Logger
	subscribeID uint64

	mu         sync.Mutex
	conn       *websocket.Conn
	cancelPing context.CancelFunc

	closed atomic.Bool
}

// NewConnector creates a new Binance WebSocket connector.
func NewConnector(cfg Config, log *logger.Logger) (Connector, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &connector{cfg: cfg, log: log.Named("binance-ws")}, nil
}

func (c *connector) Stream(ctx context.Context) (<-chan RawMessage, error) {
	ch := make(chan RawMessage, c.cfg.BufferSize)
	go c.run(ctx, ch)
	return ch, nil
}

func (c *connector) Close() error {
	c.closed.Store(true)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancelPing != nil {
		c.cancelPing()
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
	return nil
}

func (c *connector) run(ctx context.Context, ch chan<- RawMessage) {
	defer close(ch)

	for {
		if ctx.Err() != nil || c.closed.Load() {
			c.log.Info("ws: stopping run-loop")
			return
		}

		conn, err := c.connect(ctx)
		if err != nil {
			c.log.Error("ws: connect failed", zap.Error(err))
			return
		}
		c.mu.Lock()
		c.conn = conn
		c.cancelPing = c.startPinger(ctx, conn)
		c.mu.Unlock()

		if err := c.subscribe(ctx, conn); err != nil {
			c.log.Error("ws: subscribe failed", zap.Error(err))
			_ = conn.Close()
			continue
		}

		if err := c.readLoop(ctx, conn, ch); err != nil {
			c.log.Warn("ws: read-loop error, reconnecting", zap.Error(err))
		}

		c.cancelPing()
		_ = conn.Close()
	}
}

func (c *connector) connect(ctx context.Context) (*websocket.Conn, error) {
	var conn *websocket.Conn

	err := backoff.Execute(ctx, c.cfg.BackoffConfig, func(ctx context.Context) error {
		var err error
		conn, _, err = websocket.DefaultDialer.DialContext(ctx, c.cfg.URL, nil)
		return err
	}, func(ctx context.Context, err error, delay time.Duration, attempt int) {
		c.log.WithContext(ctx).Warn("binance ws connect retry",
			zap.Int("attempt", attempt),
			zap.Duration("delay", delay),
			zap.Error(err),
			zap.String("url", c.cfg.URL),
		)
	})

	return conn, err
}

func (c *connector) startPinger(ctx context.Context, conn *websocket.Conn) context.CancelFunc {
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
				_ = conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(1*time.Second))
			}
		}
	}()
	return cancel
}

func (c *connector) subscribe(ctx context.Context, conn *websocket.Conn) error {
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

func (c *connector) readLoop(ctx context.Context, conn *websocket.Conn, out chan<- RawMessage) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_, rawBytes, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err,
				websocket.CloseNormalClosure,
				websocket.CloseGoingAway,
				websocket.CloseAbnormalClosure,
			) {
				c.log.Info("ws: connection closed",
					zap.String("symbol", c.cfg.Streams[0]),
					zap.Error(err),
				)
			} else {
				c.log.Warn("ws: read error", zap.Error(err))
			}
			return err
		}

		var wrapper struct {
			Stream string          `json:"stream"`
			Data   json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(rawBytes, &wrapper); err == nil && len(wrapper.Data) > 0 {
			rawBytes = wrapper.Data
		}

		var meta struct {
			Event string `json:"e"`
		}
		_ = json.Unmarshal(rawBytes, &meta)
		msgType := meta.Event
		if msgType == "" {
			msgType = "unknown"
		}

		select {
		case out <- RawMessage{Data: rawBytes, Type: msgType}:
		default:
			c.log.Warn("ws: buffer full, dropping message", zap.String("type", msgType))
		}
	}
}
