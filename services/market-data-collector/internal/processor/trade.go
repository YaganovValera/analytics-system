// github.com/YaganovValera/analytics-system/services/market-data-collector/internal/processor/trade.go
package processor

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/YaganovValera/analytics-system/common/kafka"
	"github.com/YaganovValera/analytics-system/common/logger"
	marketdata "github.com/YaganovValera/analytics-system/proto/gen/go/v1/marketdata"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/binance"

	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const EventTypeTrade = "trade"

type tradeProcessor struct {
	producer kafka.Producer
	topic    string
	log      *logger.Logger
}

func NewTradeProcessor(p kafka.Producer, topic string, log *logger.Logger) Processor {
	return &tradeProcessor{producer: p, topic: topic, log: log.Named("trade")}
}

func (tp *tradeProcessor) Process(ctx context.Context, raw binance.RawMessage) error {
	if raw.Type != EventTypeTrade {
		return nil
	}

	ctx, span := otel.Tracer("collector/processor/trade").Start(ctx, "Process")
	defer span.End()
	metrics.EventsTotal.Inc()

	var evt struct {
		EventType string `json:"e"` // тип события (должен быть "trade")
		Ev        int64  `json:"E"` // event time (Unix ms)
		S         string `json:"s"` // символ
		P         string `json:"p"` // цена
		Q         string `json:"q"` // объём
		T         int64  `json:"t"` // trade ID
	}

	if err := json.Unmarshal(raw.Data, &evt); err != nil {
		metrics.ParseErrors.Inc()
		tp.log.WithContext(ctx).Error("failed to unmarshal trade",
			zap.ByteString("raw", raw.Data),
			zap.Error(err),
		)
		span.RecordError(err)
		return nil
	}

	price, err := strconv.ParseFloat(evt.P, 64)
	if err != nil {
		metrics.ParseErrors.Inc()
		tp.log.WithContext(ctx).Error("invalid price", zap.String("p", evt.P), zap.Error(err))
		span.RecordError(err)
		return nil
	}

	qty, err := strconv.ParseFloat(evt.Q, 64)
	if err != nil {
		metrics.ParseErrors.Inc()
		tp.log.WithContext(ctx).Error("invalid quantity", zap.String("q", evt.Q), zap.Error(err))
		span.RecordError(err)
		return nil
	}

	msg := &marketdata.MarketData{
		Timestamp: timestamppb.New(time.UnixMilli(evt.Ev)),
		Symbol:    evt.S,
		Price:     price,
		Volume:    qty,
		TradeId:   strconv.FormatInt(evt.T, 10),
	}

	bytes, err := proto.Marshal(msg)
	if err != nil {
		metrics.SerializeErrors.Inc()
		tp.log.WithContext(ctx).Error("marshal trade failed", zap.Error(err))
		span.RecordError(err)
		return err
	}

	start := time.Now()
	err = tp.producer.Publish(ctx, tp.topic, nil, bytes)
	if err != nil {
		metrics.PublishErrors.Inc()
		tp.log.WithContext(ctx).Error("publish trade failed", zap.Error(err))
		span.RecordError(err)
		return err
	}
	metrics.PublishLatency.Observe(time.Since(start).Seconds())
	return nil
}
