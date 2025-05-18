// github.com/YaganovValera/analytics-system/services/analytics-api/internal/usecase/handler.go
package usecase

import (
	"context"
	"errors"
	"fmt"

	analyticspb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/analytics"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/repository/kafka"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/repository/timescaledb"
	"go.opentelemetry.io/otel"
)

type GetCandlesHandler interface {
	Handle(ctx context.Context, req *analyticspb.GetCandlesRequest) (*analyticspb.GetCandlesResponse, error)
}

type StreamCandlesHandler interface {
	Handle(ctx context.Context, req *analyticspb.GetCandlesRequest) (<-chan *analyticspb.CandleEvent, error)
}

type getHandler struct {
	db timescaledb.Repository
}

type streamHandler struct {
	kafka kafka.Repository
}

func NewGetCandlesHandler(db timescaledb.Repository) GetCandlesHandler {
	return &getHandler{db: db}
}

func NewStreamCandlesHandler(kafka kafka.Repository) StreamCandlesHandler {
	return &streamHandler{kafka: kafka}
}

func (h *getHandler) Handle(ctx context.Context, req *analyticspb.GetCandlesRequest) (*analyticspb.GetCandlesResponse, error) {
	ctx, span := otel.Tracer("analytics-api/usecase").Start(ctx, "GetCandles")
	defer span.End()

	interval := req.Interval.String()
	candles, nextToken, err := h.db.QueryCandles(ctx, req.Symbol, interval, req.Start.AsTime(), req.End.AsTime(), req.Pagination)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	return &analyticspb.GetCandlesResponse{
		Candles:       candles,
		NextPageToken: nextToken,
	}, nil
}

func (h *streamHandler) Handle(ctx context.Context, req *analyticspb.GetCandlesRequest) (<-chan *analyticspb.CandleEvent, error) {
	ctx, span := otel.Tracer("analytics-api/usecase").Start(ctx, "StreamCandles")
	defer span.End()

	if req.Symbol == "" || req.Interval == 0 {
		return nil, errors.New("invalid request")
	}

	topic := fmt.Sprintf("candles.%s", req.Interval.String())
	return h.kafka.ConsumeCandles(ctx, topic, req.Symbol)
}
