// analytics-api/internal/storage/timescaledb/reader.go
package timescaledb

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/YaganovValera/analytics-system/common/interval"
	"github.com/YaganovValera/analytics-system/common/logger"
	analyticspb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/analytics"
	commonpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/common"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Repository описывает интерфейс TimescaleDB-хранилища для свечей.
type Repository interface {
	QueryCandles(ctx context.Context, symbol string, interval string, start, end time.Time, page *commonpb.Pagination) ([]*analyticspb.Candle, string, error)
	Ping(ctx context.Context) error
	ListSymbols(ctx context.Context, page *commonpb.Pagination) ([]string, string, error)
	Close()
}

type timescaleRepo struct {
	db  *pgxpool.Pool
	log *logger.Logger
}

// New создает и проверяет подключение к TimescaleDB.
func New(cfg Config, log *logger.Logger) (Repository, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pgxCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("pgx parse config: %w", err)
	}

	db, err := pgxpool.NewWithConfig(ctx, pgxCfg)
	if err != nil {
		return nil, fmt.Errorf("pgxpool init: %w", err)
	}

	if err := db.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pgx ping: %w", err)
	}

	log.Info("timescaledb connected")
	return &timescaleRepo{db: db, log: log.Named("timescaledb")}, nil
}

// QueryCandles возвращает список свечей за указанный период.
func (r *timescaleRepo) QueryCandles(ctx context.Context, symbol, iv string, start, end time.Time, page *commonpb.Pagination) ([]*analyticspb.Candle, string, error) {
	ctx, span := otel.Tracer("storage/timescaledb").Start(ctx, "QueryCandles",
		trace.WithAttributes(
			attribute.String("symbol", symbol),
			attribute.String("interval", iv),
		),
	)
	defer span.End()

	dur, err := interval.Duration(interval.Interval(iv))
	if err != nil {
		return nil, "", fmt.Errorf("invalid interval: %w", err)
	}

	pageSize := int32(500)
	if page != nil && page.PageSize > 0 {
		pageSize = page.PageSize
	}

	if page != nil && page.PageToken != "" {
		tokenTime, err := time.Parse(time.RFC3339Nano, page.PageToken)
		if err != nil {
			return nil, "", fmt.Errorf("invalid page_token: %w", err)
		}
		if tokenTime.After(start) {
			start = tokenTime
		}
	}

	query := `
		SELECT time, open, high, low, close, volume
		FROM candles
		WHERE symbol = $1 AND interval = $2 AND time > $3 AND time < $4
		ORDER BY time ASC
		LIMIT $5
	`

	rows, err := r.db.Query(ctx, query, symbol, iv, start.UTC(), end.UTC(), pageSize)
	if err != nil {
		span.RecordError(err)
		return nil, "", fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var result []*analyticspb.Candle
	var nextToken string

	for rows.Next() {
		var ts time.Time
		var open, high, low, close, volume float64

		if err := rows.Scan(&ts, &open, &high, &low, &close, &volume); err != nil {
			span.RecordError(err)
			return nil, "", fmt.Errorf("row scan failed: %w", err)
		}

		c := &analyticspb.Candle{
			Symbol:    symbol,
			OpenTime:  timestamppb.New(ts),
			CloseTime: timestamppb.New(ts.Add(dur)),
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close,
			Volume:    volume,
		}
		result = append(result, c)
		nextToken = ts.UTC().Format(time.RFC3339Nano)
	}

	return result, nextToken, nil
}

func (r *timescaleRepo) ListSymbols(ctx context.Context, page *commonpb.Pagination) ([]string, string, error) {
	ctx, span := otel.Tracer("storage/timescaledb").Start(ctx, "ListSymbols")
	defer span.End()

	const defaultPageSize = 100
	pageSize := int32(defaultPageSize)
	if page != nil && page.PageSize > 0 {
		pageSize = page.PageSize
	}

	query := `SELECT DISTINCT symbol FROM candles`
	var args []any

	if page != nil && page.PageToken != "" {
		query += ` WHERE symbol > $1`
		args = append(args, page.PageToken)
	}

	query += ` ORDER BY symbol ASC LIMIT $2`
	args = append(args, pageSize+1) // +1 для проверки, есть ли ещё

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		return nil, "", fmt.Errorf("query symbols: %w", err)
	}
	defer rows.Close()

	var result []string
	var nextToken string

	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err != nil {
			span.RecordError(err)
			return nil, "", fmt.Errorf("scan symbol: %w", err)
		}
		result = append(result, symbol)
	}

	// Если выбрали больше, чем pageSize — есть следующая страница
	if int32(len(result)) > pageSize {
		nextToken = result[pageSize]
		result = result[:pageSize]
	}

	return result, nextToken, rows.Err()
}

// Ping проверяет доступность TimescaleDB.
func (r *timescaleRepo) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return r.db.Ping(ctx)
}

// Close закрывает пул соединений.
func (r *timescaleRepo) Close() {
	r.db.Close()
}
