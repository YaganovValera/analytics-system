// market-data-collector/internal/processor/trade.go
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
	"go.opentelemetry.io/otel/attribute"
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
		EventType string `json:"e"` // always "trade"
		EventTime int64  `json:"E"` // renamed: Ev → EventTime
		Symbol    string `json:"s"` // renamed: S → Symbol
		Price     string `json:"p"` // renamed: P → Price
		Quantity  string `json:"q"` // renamed: Q → Quantity
		TradeID   int64  `json:"t"` // renamed: T → TradeID
	}

	if err := json.Unmarshal(raw.Data, &evt); err != nil {
		metrics.ParseErrors.Inc()
		tp.log.WithContext(ctx).Error("failed to unmarshal trade",
			zap.ByteString("raw", raw.Data),
			zap.Error(err),
		)
		span.SetAttributes(attribute.String("raw_data", string(raw.Data)))
		span.RecordError(err)
		return nil
	}

	price, err := strconv.ParseFloat(evt.Price, 64)
	if err != nil {
		metrics.ParseErrors.Inc()
		tp.log.WithContext(ctx).Error("invalid price",
			zap.String("symbol", evt.Symbol),
			zap.String("price", evt.Price),
			zap.Int64("trade_id", evt.TradeID),
			zap.Error(err),
		)
		span.SetAttributes(
			attribute.String("symbol", evt.Symbol),
			attribute.String("price", evt.Price),
			attribute.Int64("trade_id", evt.TradeID),
		)
		span.RecordError(err)

		return nil
	}

	qty, err := strconv.ParseFloat(evt.Quantity, 64)
	if err != nil {
		metrics.ParseErrors.Inc()
		tp.log.WithContext(ctx).Error("invalid quantity",
			zap.String("symbol", evt.Symbol),
			zap.String("quantity", evt.Quantity),
			zap.Int64("trade_id", evt.TradeID),
			zap.Error(err),
		)
		span.SetAttributes(
			attribute.String("symbol", evt.Symbol),
			attribute.String("quantity", evt.Quantity),
			attribute.Int64("trade_id", evt.TradeID),
		)
		span.RecordError(err)
		return nil
	}

	msg := &marketdata.MarketData{
		Timestamp: timestamppb.New(time.UnixMilli(evt.EventTime)),
		Symbol:    evt.Symbol,
		Price:     price,
		Volume:    qty,
		BidPrice:  0,
		AskPrice:  0,
		TradeId:   strconv.FormatInt(evt.TradeID, 10),
	}

	bytes, err := proto.Marshal(msg)
	if err != nil {
		metrics.SerializeErrors.Inc()
		tp.log.WithContext(ctx).Error("marshal trade failed",
			zap.String("symbol", evt.Symbol),
			zap.Int64("trade_id", evt.TradeID),
			zap.Error(err),
		)
		span.SetAttributes(attribute.String("symbol", evt.Symbol))
		span.RecordError(err)
		return err
	}

	start := time.Now()
	key := []byte(evt.Symbol)
	err = tp.producer.Publish(ctx, tp.topic, key, bytes)

	if err != nil {
		metrics.PublishErrors.Inc()
		tp.log.WithContext(ctx).Error("publish trade failed",
			zap.String("symbol", evt.Symbol),
			zap.Int64("trade_id", evt.TradeID),
			zap.Error(err),
		)
		span.SetAttributes(attribute.String("symbol", evt.Symbol))
		span.RecordError(err)
		return err
	}

	metrics.PublishLatency.Observe(time.Since(start).Seconds())
	return nil
}
