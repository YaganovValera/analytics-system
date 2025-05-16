// github.com/YaganovValera/analytics-system/services/preprocessor/internal/storage/timescaledb/timescaledb.go

// internal/storage/timescaledb/timescaledb.go
package timescaledb

import (
	"context"
	"fmt"
	"time"

	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/aggregator"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var tracer = otel.Tracer("preprocessor/storage/timescaledb")

// Config описывает конфигурацию подключения к TimescaleDB.
type Config struct {
	DSN string `mapstructure:"dsn"`
}

// TimescaleWriter реализует FlushSink, вставляя свечи в TimescaleDB.
type TimescaleWriter struct {
	db  *pgxpool.Pool
	log *logger.Logger
}

// NewTimescaleWriter подключается к БД по конфигурации.
func NewTimescaleWriter(cfg Config, log *logger.Logger) (*TimescaleWriter, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pgxCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("timescaledb: parse dsn: %w", err)
	}

	db, err := pgxpool.NewWithConfig(ctx, pgxCfg)
	if err != nil {
		return nil, fmt.Errorf("timescaledb: connect: %w", err)
	}

	if err := db.Ping(ctx); err != nil {
		return nil, fmt.Errorf("timescaledb: ping failed: %w", err)
	}

	return &TimescaleWriter{db: db, log: log.Named("timescaledb")}, nil
}

// FlushCandle вставляет одну OHLCV свечу в таблицу candles.
func (w *TimescaleWriter) FlushCandle(ctx context.Context, c *aggregator.Candle) error {
	ctx, span := tracer.Start(ctx, "FlushCandle",
		trace.WithAttributes(
			attribute.String("symbol", c.Symbol),
			attribute.String("interval", c.Interval),
		))
	defer span.End()

	const query = `INSERT INTO candles (
		time, symbol, interval, open, high, low, close, volume
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	ON CONFLICT (symbol, interval, time) DO NOTHING`

	_, err := w.db.Exec(ctx, query,
		c.Start.UTC(),
		c.Symbol,
		c.Interval,
		c.Open,
		c.High,
		c.Low,
		c.Close,
		c.Volume,
	)
	if err != nil {
		span.RecordError(err)
		w.log.WithContext(ctx).Error("insert failed", zap.String("symbol", c.Symbol), zap.String("interval", c.Interval), zap.Error(err))
		return fmt.Errorf("timescaledb insert: %w", err)
	}
	return nil
}

// Close завершает соединение с БД.
func (w *TimescaleWriter) Close() {
	w.db.Close()
}
