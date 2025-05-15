// github.com/YaganovValera/analytics-system/services/market-data-collector/internal/processor/depth.go
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
		EventType string     `json:"e"` // тип события (должен быть "depthUpdate")
		Ev        int64      `json:"E"` // event time
		S         string     `json:"s"` // symbol
		B         [][]string `json:"b"` // bids [price, quantity]
		A         [][]string `json:"a"` // asks [price, quantity]
	}

	if err := json.Unmarshal(raw.Data, &evt); err != nil {
		metrics.ParseErrors.Inc()
		dp.log.WithContext(ctx).Error("failed to unmarshal depth",
			zap.ByteString("raw", raw.Data),
			zap.Error(err),
		)
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

	bids := make([]*marketdata.OrderBookLevel, 0, len(evt.B))
	for _, entry := range evt.B {
		if lvl, err := convert(entry); err == nil {
			bids = append(bids, lvl)
		} else {
			dp.log.WithContext(ctx).Debug("skipped malformed bid", zap.Strings("entry", entry), zap.Error(err))
		}
	}

	asks := make([]*marketdata.OrderBookLevel, 0, len(evt.A))
	for _, entry := range evt.A {
		if lvl, err := convert(entry); err == nil {
			asks = append(asks, lvl)
		} else {
			dp.log.WithContext(ctx).Debug("skipped malformed ask", zap.Strings("entry", entry), zap.Error(err))
		}
	}

	msg := &marketdata.OrderBookSnapshot{
		Timestamp: timestamppb.New(time.UnixMilli(evt.Ev)),
		Symbol:    evt.S,
		Bids:      bids,
		Asks:      asks,
	}

	bytes, err := proto.Marshal(msg)
	if err != nil {
		metrics.SerializeErrors.Inc()
		dp.log.WithContext(ctx).Error("marshal depth failed", zap.Error(err))
		span.RecordError(err)
		return err
	}

	start := time.Now()
	err = dp.producer.Publish(ctx, dp.topic, nil, bytes)
	if err != nil {
		metrics.PublishErrors.Inc()
		dp.log.WithContext(ctx).Error("publish depth failed", zap.Error(err))
		span.RecordError(err)
		return err
	}
	metrics.PublishLatency.Observe(time.Since(start).Seconds())
	return nil
}
