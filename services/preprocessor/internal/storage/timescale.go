// services/preprocessor/internal/storage/postgres.go
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // pgx driver for database/sql
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"

	"github.com/YaganovValera/analytics-system/common/logger"
	marketdatapb "github.com/YaganovValera/analytics-system/proto/v1/marketdata"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/config"
)

const migrationsDir = "services/preprocessor/migrations"

// PostgresStorage отвечает за запись сырых MarketData в TimescaleDB.
type PostgresStorage struct {
	pool *pgxpool.Pool
	log  *logger.Logger
}

// NewPostgres создаёт пул соединений, выполняет миграции и проверяет связь.
func NewPostgres(ctx context.Context, cfg config.PostgresConfig, log *logger.Logger) (*PostgresStorage, error) {
	// 1) Применяем миграции через database/sql + goose
	sqlDB, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres migrate: open DB: %w", err)
	}
	defer sqlDB.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return nil, fmt.Errorf("postgres migrate: set dialect: %w", err)
	}
	if err := goose.Up(sqlDB, migrationsDir); err != nil {
		return nil, fmt.Errorf("postgres migrate: up: %w", err)
	}
	log.Info("postgres: migrations applied")

	// 2) Настройка пула pgxpool
	pgxCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse DSN: %w", err)
	}
	pgxCfg.MaxConns = int32(cfg.MaxOpenConns)
	pgxCfg.MinConns = int32(cfg.MaxIdleConns)
	pgxCfg.MaxConnLifetime = cfg.ConnMaxLifetime

	pool, err := pgxpool.NewWithConfig(ctx, pgxCfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}

	// 3) Проверка соединения
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	log.Info("postgres: connected", zap.String("dsn", cfg.DSN))

	return &PostgresStorage{
		pool: pool,
		log:  log.Named("storage"),
	}, nil
}

// InsertRaw сохраняет одно событие MarketData в таблицу raw_market_data.
func (s *PostgresStorage) InsertRaw(ctx context.Context, m *marketdatapb.MarketData) error {
	const query = `
INSERT INTO raw_market_data
  (event_time, symbol, price, bid_price, ask_price, volume, trade_id)
VALUES
  ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT DO NOTHING;
`
	start := time.Now()
	_, err := s.pool.Exec(ctx, query,
		m.Timestamp.AsTime(),
		m.Symbol,
		m.Price,
		m.BidPrice,
		m.AskPrice,
		m.Volume,
		m.TradeId,
	)
	latency := time.Since(start)
	if err != nil {
		s.log.Error("insert raw_market_data failed", zap.Error(err))
		return err
	}
	s.log.Debug("insert raw_market_data",
		zap.String("symbol", m.Symbol),
		zap.Duration("latency", latency),
	)
	return nil
}

// Ping проверяет доступность БД.
func (s *PostgresStorage) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// Close закрывает пул соединений.
func (s *PostgresStorage) Close() {
	s.pool.Close()
}
