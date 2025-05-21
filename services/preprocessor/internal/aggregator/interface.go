// preprocessor/internal/aggregator/interface.go
package aggregator

import (
	"context"
	"time"

	marketdatapb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/marketdata"
)

// Aggregator обрабатывает поступающие tick-данные и агрегирует в OHLCV свечи.
type Aggregator interface {
	// Process обрабатывает tick и обновляет соответствующий бар.
	Process(ctx context.Context, data *marketdatapb.MarketData) error

	// FlushAll завершает и выгружает все in-progress бары (по таймеру или shutdown).
	FlushAll(ctx context.Context) error

	// Close завершает работу агрегатора, закрывает фоновые потоки.
	Close() error
}

// FlushSink — интерфейс для публикации завершённых свечей (Kafka, Timescale).
type FlushSink interface {
	FlushCandle(ctx context.Context, candle *Candle) error
}

// PartialBarStorage — интерфейс к хранилищу незавершённых свечей (Redis).
type PartialBarStorage interface {
	Save(ctx context.Context, candle *Candle) error
	Load(ctx context.Context, symbol, interval string) (*Candle, error)
	Delete(ctx context.Context, symbol, interval string) error

	// Новые явные методы
	LoadAt(ctx context.Context, symbol, interval string, ts time.Time) (*Candle, error)
	DeleteAt(ctx context.Context, symbol, interval string, ts time.Time) error

	Close() error
}
