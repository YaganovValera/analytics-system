// query-service/internal/usecase/handler.go
package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/YaganovValera/analytics-system/services/query-service/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/query-service/internal/storage/timescaledb"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

type Executor interface {
	Execute(ctx context.Context, rawQuery string, params map[string]string) (columns []string, rows [][]string, err error)
}

type handler struct {
	log  *logger.Logger
	repo timescaledb.Repository
}

func NewExecutor(repo timescaledb.Repository, log *logger.Logger) Executor {
	return &handler{
		repo: repo,
		log:  log.Named("usecase"),
	}
}

func (h *handler) Execute(ctx context.Context, rawQuery string, params map[string]string) ([]string, [][]string, error) {
	ctx, span := otel.Tracer("query/usecase").Start(ctx, "Execute")
	defer span.End()

	start := time.Now()
	method := "Execute"

	metrics.GRPCRequestsTotal.WithLabelValues(method).Inc()

	if strings.TrimSpace(rawQuery) == "" {
		h.log.WithContext(ctx).Warn("empty query")
		return nil, nil, errors.New("query must not be empty")
	}

	if len(params) > 25 {
		h.log.WithContext(ctx).Warn("too many query parameters", zap.Int("param_count", len(params)))
		return nil, nil, errors.New("too many parameters")
	}

	h.log.WithContext(ctx).Info("executing SQL", zap.String("query", rawQuery), zap.Int("params", len(params)))

	columns, rows, err := h.repo.ExecuteSQL(ctx, rawQuery, params)
	if err != nil {
		h.log.WithContext(ctx).Error("execution failed", zap.Error(err))
		span.RecordError(err)
		return nil, nil, fmt.Errorf("execution failed: %w", err)
	}

	span.SetAttributes(attribute.Int("rows", len(rows)), attribute.Int("cols", len(columns)))
	h.log.WithContext(ctx).Info("execution complete",
		zap.Int("rows", len(rows)),
		zap.Int("cols", len(columns)),
		zap.Duration("latency", time.Since(start)),
	)

	return columns, rows, nil
}
