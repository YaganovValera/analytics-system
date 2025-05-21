// market-data-collector/internal/processor/depth.go
package processor

import (
	"context"
	"encoding/json"
	"fmt"
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

const EventTypeDepth = "depthUpdate"

type depthProcessor struct {
	producer kafka.Producer
	topic    string
	log      *logger.Logger
}

func NewDepthProcessor(p kafka.Producer, topic string, log *logger.Logger) Processor {
	return &depthProcessor{producer: p, topic: topic, log: log.Named("depth")}
}

func (dp *depthProcessor) Process(ctx context.Context, raw binance.RawMessage) error {
	if raw.Type != EventTypeDepth {
		return nil
	}

	ctx, span := otel.Tracer("collector/processor/depth").Start(ctx, "Process")
	defer span.End()
	metrics.EventsTotal.Inc()

	var evt struct {
		EventType string     `json:"e"` // always "depthUpdate"
		EventTime int64      `json:"E"` // renamed: Ev → EventTime
		Symbol    string     `json:"s"` // renamed: S → Symbol
		Bids      [][]string `json:"b"` // renamed: B → Bids
		Asks      [][]string `json:"a"` // renamed: A → Asks
	}

	if err := json.Unmarshal(raw.Data, &evt); err != nil {
		metrics.ParseErrors.Inc()
		dp.log.WithContext(ctx).Error("failed to unmarshal depth",
			zap.ByteString("raw", raw.Data),
			zap.Error(err),
		)
		span.SetAttributes(attribute.String("raw_data", string(raw.Data)))
		span.RecordError(err)
		return nil
	}

	convert := func(pair []string) (*marketdata.OrderBookLevel, error) {
		if len(pair) < 2 {
			return nil, fmt.Errorf("malformed level: %v", pair)
		}
		price, err := strconv.ParseFloat(pair[0], 64)
		if err != nil {
			return nil, err
		}
		qty, err := strconv.ParseFloat(pair[1], 64)
		if err != nil {
			return nil, err
		}
		return &marketdata.OrderBookLevel{Price: price, Quantity: qty}, nil
	}

	bids := make([]*marketdata.OrderBookLevel, 0, len(evt.Bids))
	for _, entry := range evt.Bids {
		if lvl, err := convert(entry); err == nil {
			bids = append(bids, lvl)
		} else {
			dp.log.WithContext(ctx).Debug("skipped malformed bid",
				zap.String("symbol", evt.Symbol),
				zap.Any("entry", entry),
				zap.Error(err),
			)
		}
	}

	asks := make([]*marketdata.OrderBookLevel, 0, len(evt.Asks))
	for _, entry := range evt.Asks {
		if lvl, err := convert(entry); err == nil {
			asks = append(asks, lvl)
		} else {
			dp.log.WithContext(ctx).Debug("skipped malformed ask",
				zap.String("symbol", evt.Symbol),
				zap.Any("entry", entry),
				zap.Error(err),
			)
		}
	}

	msg := &marketdata.OrderBookSnapshot{
		Timestamp: timestamppb.New(time.UnixMilli(evt.EventTime)),
		Symbol:    evt.Symbol,
		Bids:      bids,
		Asks:      asks,
	}

	bytes, err := proto.Marshal(msg)
	if err != nil {
		metrics.SerializeErrors.Inc()
		dp.log.WithContext(ctx).Error("marshal depth failed",
			zap.String("symbol", evt.Symbol),
			zap.Int("bids", len(bids)),
			zap.Int("asks", len(asks)),
			zap.Error(err),
		)
		span.SetAttributes(
			attribute.String("symbol", evt.Symbol),
			attribute.Int("bids", len(bids)),
			attribute.Int("asks", len(asks)),
		)
		span.RecordError(err)
		return err
	}

	start := time.Now()
	key := []byte(evt.Symbol)
	err = dp.producer.Publish(ctx, dp.topic, key, bytes)

	if err != nil {
		metrics.PublishErrors.Inc()
		dp.log.WithContext(ctx).Error("publish depth failed",
			zap.String("symbol", evt.Symbol),
			zap.Int("bids", len(bids)),
			zap.Int("asks", len(asks)),
			zap.Error(err),
		)
		span.SetAttributes(attribute.String("symbol", evt.Symbol))
		span.RecordError(err)
		return err
	}

	metrics.PublishLatency.Observe(time.Since(start).Seconds())
	return nil
}
