// github.com/YaganovValera/analytics-system/services/analytics-api/internal/repository/timescaledb/query.go
// internal/repository/timescaledb/query.go
package timescaledb

import (
	"context"
	"fmt"
	"time"

	"github.com/YaganovValera/analytics-system/common/logger"
	analyticspb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/analytics"
	commonpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/common"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Repository interface {
	QueryCandles(ctx context.Context, symbol string, interval string, start, end time.Time, page *commonpb.Pagination) ([]*analyticspb.Candle, string, error)
	Ping(ctx context.Context) error
	Close()
}

type timescaleRepo struct {
	db  *pgxpool.Pool
	log *logger.Logger
}

func New(cfg config.TimescaleConfig, log *logger.Logger) (Repository, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := pgxpool.New(ctx, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("pgxpool connect: %w", err)
	}
	if err := db.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pgx ping: %w", err)
	}

	return &timescaleRepo{db: db, log: log.Named("timescaledb")}, nil
}

func (r *timescaleRepo) QueryCandles(ctx context.Context, symbol, interval string, start, end time.Time, page *commonpb.Pagination) ([]*analyticspb.Candle, string, error) {
	pageSize := int32(500)
	if page != nil && page.PageSize > 0 {
		pageSize = page.PageSize
	}
	query := `SELECT time, open, high, low, close, volume FROM candles WHERE symbol = $1 AND interval = $2 AND time >= $3 AND time < $4 ORDER BY time LIMIT $5`
	rows, err := r.db.Query(ctx, query, symbol, interval, start.UTC(), end.UTC(), pageSize)
	if err != nil {
		return nil, "", fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	var candles []*analyticspb.Candle
	for rows.Next() {
		var ts time.Time
		var c analyticspb.Candle
		c.Symbol = symbol
		if err := rows.Scan(&ts, &c.Open, &c.High, &c.Low, &c.Close, &c.Volume); err != nil {
			return nil, "", err
		}
		c.OpenTime = timestamppb.New(ts)
		c.CloseTime = timestamppb.New(ts.Add(time.Minute))
		candles = append(candles, &c)
	}
	return candles, "", nil
}

func (r *timescaleRepo) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return r.db.Ping(ctx)
}

func (r *timescaleRepo) Close() {
	r.db.Close()
}
