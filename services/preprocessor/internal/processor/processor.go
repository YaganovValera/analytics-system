// services/preprocessor/internal/processor/processor.go
package processor

import (
	"context"

	"github.com/YaganovValera/analytics-system/common/kafka"
	"github.com/YaganovValera/analytics-system/common/logger"
	commonpb "github.com/YaganovValera/analytics-system/proto/v1/common"
	marketdatapb "github.com/YaganovValera/analytics-system/proto/v1/marketdata"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/preprocessor/pkg/redis"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

// Processor updates partial bars in Redis.
type Processor struct {
	storage   redis.Storage
	intervals []commonpb.AggregationInterval
	log       *logger.Logger
}

func NewProcessor(storage redis.Storage, intervals []commonpb.AggregationInterval, log *logger.Logger) *Processor {
	return &Processor{
		storage:   storage,
		intervals: intervals,
		log:       log.Named("processor"),
	}
}

func (p *Processor) Process(ctx context.Context, msg *kafka.Message) error {
	metrics.ConsumedMessages.Inc()

	var m marketdatapb.MarketData
	if err := proto.Unmarshal(msg.Value, &m); err != nil {
		metrics.AggregationErrors.Inc()
		p.log.WithContext(ctx).Error("unmarshal MarketData", zap.Error(err))
		return nil
	}

	t := m.Timestamp.AsTime().UTC()
	symbol, price, vol := m.Symbol, m.Price, m.Volume

	for _, iv := range p.intervals {
		d := intervalToDuration(iv)
		start := t.Truncate(d)
		key := makeKey(symbol, iv, start)

		// read state
		raw, err := p.storage.Get(ctx, key)
		var bar *barState
		if err != nil {
			if err == redis.ErrNotFound {
				bar = newBar(symbol, start, price, vol)
			} else {
				metrics.AggregationErrors.Inc()
				p.log.WithContext(ctx).Error("redis GET", zap.String("key", key), zap.Error(err))
				continue
			}
		} else {
			bar, err = unmarshalBar(raw)
			if err != nil {
				metrics.AggregationErrors.Inc()
				p.log.WithContext(ctx).Error("unmarshal bar", zap.Error(err))
				bar = newBar(symbol, start, price, vol)
			}
			// update OHLCV
			bar.Close = price
			bar.Volume += vol
			if price > bar.High {
				bar.High = price
			}
			if price < bar.Low {
				bar.Low = price
			}
		}

		payload, err := marshalBar(bar)
		if err != nil {
			metrics.AggregationErrors.Inc()
			p.log.WithContext(ctx).Error("marshal bar", zap.Error(err))
			continue
		}
		if err := p.storage.Set(ctx, key, payload); err != nil {
			metrics.AggregationErrors.Inc()
			p.log.WithContext(ctx).Error("redis SET", zap.String("key", key), zap.Error(err))
		}
	}

	return nil
}
