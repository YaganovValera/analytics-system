// auth/internal/storage/postgres/postgres.go
package postgres

import (
	"context"
	"fmt"
	"time"

	migr "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/YaganovValera/analytics-system/common/logger"
)

func Connect(cfg Config, log *logger.Logger) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pgxCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	db, err := pgxpool.NewWithConfig(ctx, pgxCfg)
	if err != nil {
		return nil, fmt.Errorf("pgx connect: %w", err)
	}
	if err := db.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pgx ping: %w", err)
	}
	log.Info("PostgreSQL connected")
	return db, nil
}

func ApplyMigrations(cfg Config, log *logger.Logger) error {
	m, err := migr.New(
		"file://"+cfg.MigrationsDir,
		cfg.DSN,
	)
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migr.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	log.Info("PostgreSQL migrations applied")
	return nil
}
