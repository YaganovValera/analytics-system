// github.com/YaganovValera/analytics-system/services/preprocessor/internal/storage/timescaledb/timescaledb.go

// internal/storage/timescaledb/timescaledb.go
package timescaledb

import (
	"context"
	"fmt"
	"time"

	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/YaganovValera/analytics-system/common/serviceid"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/aggregator"
	migr "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var tracer = otel.Tracer("preprocessor/storage/timescaledb")

// ApplyMigrations применяет все миграции в директории.
func ApplyMigrations(cfg Config, log *logger.Logger) error {
	// Регистрируем имя сервиса для всех common-метрик/трейсов
	serviceid.InitServiceName(cfg.MigrationsDir)

	m, err := migr.New(
		fmt.Sprintf("file://%s", cfg.MigrationsDir),
		cfg.DSN,
	)
	if err != nil {
		return fmt.Errorf("timescaledb: migrate init: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migr.ErrNoChange {
		return fmt.Errorf("timescaledb: migrate up: %w", err)
	}

	log.Info("timescaledb: migrations applied successfully")
	return nil
}

// TimescaleWriter умеет писать свечи в БД.
type TimescaleWriter struct {
	db  *pgxpool.Pool
	log *logger.Logger
}

// NewTimescaleWriter коннектится к TimescaleDB и пингует её.
func NewTimescaleWriter(cfg Config, log *logger.Logger) (*TimescaleWriter, error) {
	// Контекст для подключения
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

	return &TimescaleWriter{
		db:  db,
		log: log.Named("timescaledb"),
	}, nil
}

// FlushCandle вставляет или игнорирует дубликат.
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
		w.log.WithContext(ctx).
			Error("timescaledb insert failed",
				zap.String("symbol", c.Symbol),
				zap.String("interval", c.Interval),
				zap.Error(err))
		return fmt.Errorf("timescaledb insert: %w", err)
	}
	return nil
}

// Ping проверяет доступность БД.
func (w *TimescaleWriter) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return w.db.Ping(ctx)
}

// Close завершает пул соединений.
func (w *TimescaleWriter) Close() {
	w.db.Close()
}
