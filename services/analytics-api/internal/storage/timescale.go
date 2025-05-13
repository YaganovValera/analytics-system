// services/analytics-api/internal/storage/postgres.go
package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/YaganovValera/analytics-system/common/logger"
	analyticspb "github.com/YaganovValera/analytics-system/proto/v1/analytics"
	commonpb "github.com/YaganovValera/analytics-system/proto/v1/common"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/config"
)

// Repository определяет интерфейс доступа к данным свечей.
type Repository interface {
	GetCandles(
		ctx context.Context,
		symbol string,
		start, end time.Time,
		interval commonpb.AggregationInterval,
		pageSize int,
		pageToken string,
	) (candles []*analyticspb.Candle, nextPageToken string, err error)

	// StreamCandles возвращает канал событий: либо Candle, либо StreamError.
	StreamCandles(
		ctx context.Context,
		symbol string,
		start, end time.Time,
		interval commonpb.AggregationInterval,
	) (<-chan *analyticspb.CandleEvent, error)
}

type postgresRepo struct {
	pool *pgxpool.Pool
	log  *logger.Logger
}

// NewPostgresRepo создаёт подключение и возвращает репозиторий.
func NewPostgresRepo(ctx context.Context, cfg config.PostgresConfig, log *logger.Logger) (Repository, error) {
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
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	log.Info("postgres: connected for analytics-api", zap.String("dsn", cfg.DSN))

	return &postgresRepo{
		pool: pool,
		log:  log.Named("storage"),
	}, nil
}

func (r *postgresRepo) GetCandles(
	ctx context.Context,
	symbol string,
	start, end time.Time,
	interval commonpb.AggregationInterval,
	pageSize int,
	pageToken string,
) ([]*analyticspb.Candle, string, error) {
	// pageToken — ISO8601 timestamp of the last open_time from previous page
	var after time.Time
	if pageToken != "" {
		t, err := time.Parse(time.RFC3339Nano, pageToken)
		if err != nil {
			return nil, "", fmt.Errorf("invalid page_token: %w", err)
		}
		after = t
	}

	// continuous aggregate view name pattern: e.g. candles_1m, candles_5m, etc.
	// допишите реальные имена ваших view
	view := fmt.Sprintf("candles_%s", interval.String())

	q := fmt.Sprintf(`
SELECT open_time, close_time, symbol, open, high, low, close, volume
FROM %s
WHERE symbol = $1
  AND open_time >= $2
  AND open_time <  $3
  AND open_time >  $4
ORDER BY open_time
LIMIT $5
`, view)

	rows, err := r.pool.Query(ctx, q, symbol, start, end, after, pageSize)
	if err != nil {
		return nil, "", fmt.Errorf("query candles: %w", err)
	}
	defer rows.Close()

	candles := make([]*analyticspb.Candle, 0, pageSize)
	var lastTime time.Time
	for rows.Next() {
		var c analyticspb.Candle
		if err := rows.Scan(
			&c.OpenTime,
			&c.CloseTime,
			&c.Symbol,
			&c.Open,
			&c.High,
			&c.Low,
			&c.Close,
			&c.Volume,
		); err != nil {
			return nil, "", fmt.Errorf("scan candle: %w", err)
		}
		candles = append(candles, &c)
		lastTime = c.OpenTime.AsTime()
	}
	if rows.Err() != nil {
		return nil, "", fmt.Errorf("iterate candles: %w", rows.Err())
	}

	nextToken := ""
	if len(candles) == pageSize {
		// next token is the last open_time, in RFC3339Nano
		nextToken = lastTime.Format(time.RFC3339Nano)
	}
	return candles, nextToken, nil
}

func (r *postgresRepo) StreamCandles(
	ctx context.Context,
	symbol string,
	start, end time.Time,
	interval commonpb.AggregationInterval,
) (<-chan *analyticspb.CandleEvent, error) {
	out := make(chan *analyticspb.CandleEvent)

	go func() {
		defer close(out)

		// Re-use same query but no LIMIT, we stream all rows
		view := fmt.Sprintf("candles_%s", interval.String())
		q := fmt.Sprintf(`
SELECT open_time, close_time, symbol, open, high, low, close, volume
FROM %s
WHERE symbol = $1
  AND open_time >= $2
  AND open_time <  $3
ORDER BY open_time
`, view)

		rows, err := r.pool.Query(ctx, q, symbol, start, end)
		if err != nil {
			out <- &analyticspb.CandleEvent{
				Payload: &analyticspb.CandleEvent_StreamError{
					StreamError: &commonpb.StreamError{
						Code:    commonpb.ErrorCode_INTERNAL,
						Message: fmt.Sprintf("query error: %v", err),
					},
				},
			}
			return
		}
		defer rows.Close()

		for rows.Next() {
			var c analyticspb.Candle
			if err := rows.Scan(
				&c.OpenTime,
				&c.CloseTime,
				&c.Symbol,
				&c.Open,
				&c.High,
				&c.Low,
				&c.Close,
				&c.Volume,
			); err != nil {
				out <- &analyticspb.CandleEvent{
					Payload: &analyticspb.CandleEvent_StreamError{
						StreamError: &commonpb.StreamError{
							Code:    commonpb.ErrorCode_INTERNAL,
							Message: fmt.Sprintf("scan error: %v", err),
						},
					},
				}
				return
			}
			out <- &analyticspb.CandleEvent{Payload: &analyticspb.CandleEvent_Candle{Candle: &c}}
		}
		if err := rows.Err(); err != nil {
			out <- &analyticspb.CandleEvent{
				Payload: &analyticspb.CandleEvent_StreamError{
					StreamError: &commonpb.StreamError{
						Code:    commonpb.ErrorCode_INTERNAL,
						Message: fmt.Sprintf("iteration error: %v", err),
					},
				},
			}
		}
	}()

	return out, nil
}
