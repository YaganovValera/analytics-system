// query-service/internal/storage/timescaledb/repo.go
package timescaledb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/YaganovValera/analytics-system/services/query-service/internal/metrics"
	"go.uber.org/zap"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("query/storage/timescaledb")

type repo struct {
	db  *pgxpool.Pool
	log *logger.Logger
}

func New(cfg Config, log *logger.Logger) (Repository, error) {
	log = log.Named("timescaledb")
	cfg.ApplyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

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

	log.Info("connected to timescaledb")
	return &repo{db: db, log: log}, nil
}

func (r *repo) Ping(ctx context.Context) error {
	return r.db.Ping(ctx)
}

func (r *repo) Close() {
	r.db.Close()
	r.log.Info("timescaledb connection closed")
}

func (r *repo) ExecuteSQL(ctx context.Context, rawQuery string, params map[string]string) ([]string, [][]string, error) {
	ctx, span := tracer.Start(ctx, "ExecuteSQL")
	defer span.End()

	start := time.Now()
	source := "timescaledb"

	// Безопасность
	if !isSafeSelect(rawQuery) {
		metrics.QueryErrorsTotal.WithLabelValues("unsafe", source).Inc()
		r.log.WithContext(ctx).Warn("unsafe SQL query rejected", zap.String("query", rawQuery))
		return nil, nil, errors.New("only SELECT queries are allowed")
	}

	query, args := prepareQuery(rawQuery, params)
	r.log.WithContext(ctx).Debug("executing SQL query", zap.String("query", query), zap.Int("params", len(args)))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		metrics.QueryErrorsTotal.WithLabelValues("query", source).Inc()
		span.RecordError(err)
		r.log.WithContext(ctx).Error("query failed", zap.String("sql", query), zap.Error(err))
		return nil, nil, err
	}
	defer rows.Close()

	// Колонки
	fields := rows.FieldDescriptions()
	columns := make([]string, len(fields))
	for i, f := range fields {
		columns[i] = string(f.Name)
	}

	// Значения
	var data [][]string
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			metrics.QueryErrorsTotal.WithLabelValues("scan", source).Inc()
			r.log.WithContext(ctx).Error("scan failed", zap.Error(err))
			return nil, nil, err
		}
		row := make([]string, len(vals))
		for i, v := range vals {
			row[i] = fmt.Sprintf("%v", v)
		}
		data = append(data, row)
	}

	// Метрики
	metrics.QueryLatency.WithLabelValues(source).Observe(time.Since(start).Seconds())
	metrics.QuerySuccessTotal.WithLabelValues(source).Inc()
	metrics.QueryRowsReturned.WithLabelValues(source).Observe(float64(len(data)))

	r.log.WithContext(ctx).Info("query executed",
		zap.Int("rows", len(data)),
		zap.Int("cols", len(columns)),
		zap.String("sql", query),
	)

	return columns, data, nil
}

func isSafeSelect(q string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	if !strings.HasPrefix(q, "select") {
		return false
	}
	illegal := []string{"insert", "update", "delete", "drop", "alter", "create", ";", "--"}
	for _, kw := range illegal {
		if strings.Contains(q, kw) {
			return false
		}
	}
	return true
}

// prepareQuery заменяет :param → $1, $2 и собирает аргументы.
func prepareQuery(raw string, params map[string]string) (string, []interface{}) {
	sql := raw
	args := []interface{}{}
	idx := 1
	for key, val := range params {
		placeholder := fmt.Sprintf(":%s", key)
		if strings.Contains(sql, placeholder) {
			sql = strings.ReplaceAll(sql, placeholder, fmt.Sprintf("$%d", idx))
			args = append(args, val)
			idx++
		}
	}
	return sql, args
}
