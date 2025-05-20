// github.com/YaganovValera/analytics-system/services/analytics-api/internal/usecase
package usecase

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/YaganovValera/analytics-system/common/logger"
	analyticspb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/analytics"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/storage/kafka"
	timescaledb "github.com/YaganovValera/analytics-system/services/analytics-api/internal/storage/timescaledb"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

// GetCandlesHandler обрабатывает gRPC-запрос GetCandles.
type GetCandlesHandler interface {
	Handle(ctx context.Context, req *analyticspb.QueryCandlesRequest) (*analyticspb.GetCandlesResponse, error)
}

// StreamCandlesHandler обрабатывает gRPC-запрос StreamCandles.
type StreamCandlesHandler interface {
	Handle(ctx context.Context, req *analyticspb.StreamCandlesRequest) (<-chan *analyticspb.CandleEvent, error)
}

// SubscribeCandlesHandler обрабатывает gRPC bi-directional поток SubscribeCandles.
type SubscribeCandlesHandler interface {
	Handle(ctx context.Context, in <-chan *analyticspb.CandleStreamRequest) (<-chan *analyticspb.CandleStreamResponse, error)
}

// ======================= IMPLEMENTATIONS =======================

type getHandler struct {
	db timescaledb.Repository
}

type streamHandler struct {
	kafka     kafka.Repository
	topicBase string
}

type subscribeHandler struct {
	kafka     kafka.Repository
	topicBase string
	log       *logger.Logger
}

func NewGetCandlesHandler(db timescaledb.Repository) GetCandlesHandler {
	return &getHandler{db: db}
}

func NewStreamCandlesHandler(kafka kafka.Repository, topicBase string) StreamCandlesHandler {
	return &streamHandler{kafka: kafka, topicBase: topicBase}
}

func NewSubscribeCandlesHandler(kafka kafka.Repository, topicBase string, log *logger.Logger) SubscribeCandlesHandler {
	return &subscribeHandler{kafka: kafka, topicBase: topicBase, log: log.Named("subscribe")}
}

func (h *getHandler) Handle(ctx context.Context, req *analyticspb.QueryCandlesRequest) (*analyticspb.GetCandlesResponse, error) {
	ctx, span := otel.Tracer("analytics-api/usecase").Start(ctx, "GetCandles")
	defer span.End()

	iv := req.Interval
	ivStr := iv.String()
	start := req.Start.AsTime()
	end := req.End.AsTime()

	candles, nextToken, err := h.db.QueryCandles(ctx, req.Symbol, ivStr, start, end, req.Pagination)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("query failed: %w", err)
	}

	return &analyticspb.GetCandlesResponse{
		Candles:       candles,
		NextPageToken: nextToken,
	}, nil
}

func (h *streamHandler) Handle(ctx context.Context, req *analyticspb.StreamCandlesRequest) (<-chan *analyticspb.CandleEvent, error) {
	ctx, span := otel.Tracer("analytics-api/usecase").Start(ctx, "StreamCandles")
	defer span.End()

	topic := fmt.Sprintf("%s.%s", h.topicBase, req.Interval.String())
	return h.kafka.ConsumeCandles(ctx, topic, req.Symbol)
}

func (h *subscribeHandler) Handle(ctx context.Context, in <-chan *analyticspb.CandleStreamRequest) (<-chan *analyticspb.CandleStreamResponse, error) {
	out := make(chan *analyticspb.CandleStreamResponse, 100)

	var symbol string
	var intervalStr string
	var started int32
	var ackCount, sentCount int64

	ctxStream, cancel := context.WithCancel(ctx)

	go func() {
		defer cancel()
		defer close(out)

		for msg := range in {
			switch payload := msg.Payload.(type) {
			case *analyticspb.CandleStreamRequest_Subscribe:
				if atomic.LoadInt32(&started) != 0 {
					continue // already subscribed
				}
				symbol = payload.Subscribe.Symbol
				intervalStr = payload.Subscribe.Interval.String()
				if symbol == "" || intervalStr == "" {
					continue
				}
				topic := fmt.Sprintf("%s.%s", h.topicBase, intervalStr)
				stream, err := h.kafka.ConsumeCandles(ctxStream, topic, symbol)
				if err != nil {
					h.log.WithContext(ctx).Error("subscription failed", zap.Error(err))
					return
				}
				atomic.StoreInt32(&started, 1)
				go func() {
					for evt := range stream {
						atomic.AddInt64(&sentCount, 1)
						out <- &analyticspb.CandleStreamResponse{
							Payload: &analyticspb.CandleStreamResponse_Event{
								Event: evt,
							},
						}

						if pending := atomic.LoadInt64(&sentCount) - atomic.LoadInt64(&ackCount); pending > 100 {
							out <- &analyticspb.CandleStreamResponse{
								Payload: &analyticspb.CandleStreamResponse_Control{
									Control: &analyticspb.FlowControl{
										Message: "client too slow",
									},
								},
							}
						}
					}
				}()

			case *analyticspb.CandleStreamRequest_Ack:
				atomic.AddInt64(&ackCount, 1)
			}
		}
	}()

	return out, nil
}
