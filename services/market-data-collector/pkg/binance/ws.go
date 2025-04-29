// pkg/binance/ws.go
package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/backoff"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/logger"
)

// RawMessage несёт оригинальные JSON-данные и их тип (поле "e").
type RawMessage struct {
	Data []byte
	Type string
}

// Config задаёт параметры подключения к Binance WebSocket.
type Config struct {
	WSURL         string         // адрес WebSocket, например "wss://stream.binance.com:9443/ws"
	Symbols       []string       // стримы, напр. ["btcusdt@trade","ethusdt@depth"]
	BufferSize    int            // размер буфера для канала RawMessage
	ReadTimeout   time.Duration  // ReadDeadline, например 30s
	BackoffConfig backoff.Config // настройки экспоненциального бэкоффа
}

// validate проверяет и заполняет default-значения.
func (c *Config) validate() error {
	var errs []string

	if c.WSURL == "" {
		errs = append(errs, "WSURL is required")
	}
	if len(c.Symbols) == 0 {
		errs = append(errs, "at least one Symbol is required")
	}
	if c.BufferSize <= 0 {
		c.BufferSize = 100 // reasonable default
	}
	if c.ReadTimeout <= 0 {
		c.ReadTimeout = 30 * time.Second
	}
	if c.BackoffConfig.InitialInterval <= 0 {
		c.BackoffConfig.InitialInterval = 1 * time.Second
	}
	if c.BackoffConfig.RandomizationFactor <= 0 {
		c.BackoffConfig.RandomizationFactor = 0.5
	}
	if c.BackoffConfig.Multiplier <= 0 {
		c.BackoffConfig.Multiplier = 2.0
	}
	if c.BackoffConfig.MaxInterval <= 0 {
		c.BackoffConfig.MaxInterval = 30 * time.Second
	}
	// MaxElapsedTime == 0 means no overall timeout

	if len(errs) > 0 {
		return fmt.Errorf("invalid Config: %s", strings.Join(errs, "; "))
	}
	return nil
}

// Connector управляет соединением к Binance WS с авто-reconnect.
type Connector struct {
	cfg         Config
	log         *logger.Logger
	subscribeID uint64 // для уникальных подписок
}

// NewConnector создаёт Connector.
// Логгер именуется как "binance-ws" для удобного фильтра в логах.
func NewConnector(cfg Config, log *logger.Logger) (*Connector, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &Connector{
		cfg: cfg,
		log: log.Named("binance-ws"),
	}, nil
}

// Stream запускает подпроцесс чтения и возвращает канал RawMessage.
// Закрытие канала происходит при отмене ctx.
func (c *Connector) Stream(ctx context.Context) (<-chan RawMessage, error) {
	ch := make(chan RawMessage, c.cfg.BufferSize)
	go c.run(ctx, ch)
	return ch, nil
}

func (c *Connector) run(ctx context.Context, ch chan<- RawMessage) {
	defer close(ch)

	for {
		// 1) Проверка отмены контекста
		select {
		case <-ctx.Done():
			c.log.Sugar().Infow("ws: context cancelled, exiting")
			return
		default:
		}

		// 2) Подключаемся с бэкоффом
		var conn *websocket.Conn
		err := backoff.WithBackoff(ctx, c.cfg.BackoffConfig, c.log, func(ctxTry context.Context) error {
			var dialErr error
			conn, _, dialErr = websocket.DefaultDialer.DialContext(ctxTry, c.cfg.WSURL, nil)
			return dialErr
		})
		if err != nil {
			c.log.Sugar().Errorw("ws: failed to connect after retries", "err", err)
			continue
		}
		c.log.Sugar().Infow("ws: connected", "url", c.cfg.WSURL)

		// 3) Контекст для ping-горутины
		connCtx, cancelPing := context.WithCancel(ctx)

		// 4) Настройка read-пинг механизма
		conn.SetReadDeadline(time.Now().Add(c.cfg.ReadTimeout))
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(c.cfg.ReadTimeout))
		})

		// 5) Запуск ping-горутины с WriteDeadline и логированием ошибок
		go func(cConn *websocket.Conn) {
			ticker := time.NewTicker(c.cfg.ReadTimeout / 3)
			defer ticker.Stop()
			for {
				select {
				case <-connCtx.Done():
					return
				case <-ticker.C:
					// перед пингом задаём WriteDeadline
					_ = cConn.SetWriteDeadline(time.Now().Add(time.Second))
					if err := cConn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second)); err != nil {
						c.log.Sugar().Warnw("ws: ping failed", "err", err)
					}
				}
			}
		}(conn)

		// 6) Подписываемся на стримы с уникальным id
		id := atomic.AddUint64(&c.subscribeID, 1)
		req := map[string]interface{}{
			"method": "SUBSCRIBE",
			"params": c.cfg.Symbols,
			"id":     id,
		}
		// WriteDeadline перед подпиской
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteJSON(req); err != nil {
			c.log.Sugar().Errorw("ws: subscribe failed", "err", err, "id", id)
			conn.Close()
			cancelPing()
			continue
		}

		// 7) Чтение сообщений
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				c.log.Sugar().Warnw("ws: read error, reconnecting", "err", err)
				conn.Close()
				cancelPing()
				break
			}

			// Классифицируем событие по полю "e"
			msgType := "unknown"
			var meta struct {
				Event string `json:"e"`
			}
			if uErr := json.Unmarshal(data, &meta); uErr == nil && meta.Event != "" {
				msgType = meta.Event
			}

			// Отправляем, если есть место в буфере
			select {
			case ch <- RawMessage{Data: data, Type: msgType}:
			default:
				c.log.Sugar().Warnw("ws: buffer full, dropping message", "type", msgType)
			}
		}
	}
}
