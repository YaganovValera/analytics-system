// services/preprocessor/internal/processor/agg.go
package processor

import (
	"context"
	"fmt"
	"time"

	"github.com/YaganovValera/analytics-system/common/kafka"
	"github.com/YaganovValera/analytics-system/common/logger"
	analyticspb "github.com/YaganovValera/analytics-system/proto/v1/analytics"
	commonpb "github.com/YaganovValera/analytics-system/proto/v1/common"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/preprocessor/pkg/redis"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// -----------------------------------------------------------------------------
// Internal helpers & types
// -----------------------------------------------------------------------------

var tracer = otel.Tracer("processor-aggregator")

// barState — alias на protobuf Candle (используем *barState, чтобы не копировать
// внутренний protoimpl.MessageState с мьютексом).
type barState analyticspb.Candle

func newBar(symbol string, start time.Time, price, vol float64) *barState {
	return &barState{
		OpenTime: timestamppb.New(start),
		Symbol:   symbol,
		Open:     price,
		High:     price,
		Low:      price,
		Close:    price,
		Volume:   vol,
	}
}

func unmarshalBar(data []byte) (*barState, error) {
	var c analyticspb.Candle
	if err := proto.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return (*barState)(&c), nil
}

func marshalBar(b *barState) ([]byte, error) {
	return proto.Marshal((*analyticspb.Candle)(b))
}

func intervalToDuration(iv commonpb.AggregationInterval) time.Duration {
	switch iv {
	case commonpb.AggregationInterval_AGG_INTERVAL_1_MINUTE:
		return time.Minute
	case commonpb.AggregationInterval_AGG_INTERVAL_5_MINUTES:
		return 5 * time.Minute
	case commonpb.AggregationInterval_AGG_INTERVAL_15_MINUTES:
		return 15 * time.Minute
	case commonpb.AggregationInterval_AGG_INTERVAL_1_HOUR:
		return time.Hour
	case commonpb.AggregationInterval_AGG_INTERVAL_4_HOURS:
		return 4 * time.Hour
	case commonpb.AggregationInterval_AGG_INTERVAL_1_DAY:
		return 24 * time.Hour
	default:
		panic(fmt.Sprintf("unsupported interval %v", iv))
	}
}

func makeKey(symbol string, iv commonpb.AggregationInterval, start time.Time) string {
	return fmt.Sprintf("%s:%d:%d", symbol, iv, start.UnixMilli())
}

// -----------------------------------------------------------------------------
// Aggregator
// -----------------------------------------------------------------------------

type Aggregator struct {
	storage   redis.Storage
	producer  kafka.Producer
	intervals []commonpb.AggregationInterval
	symbols   []string
	topic     string
	log       *logger.Logger
}

// NewAggregator wires deps.
func NewAggregator(
	storage redis.Storage,
	producer kafka.Producer,
	intervals []commonpb.AggregationInterval,
	symbols []string,
	topic string,
	log *logger.Logger,
) *Aggregator {
	return &Aggregator{
		storage:   storage,
		producer:  producer,
		intervals: intervals,
		symbols:   symbols,
		topic:     topic,
		log:       log,
	}
}

// Start creates ticker goroutine per interval.
func (a *Aggregator) Start(ctx context.Context) {
	for _, iv := range a.intervals {
		iv := iv
		d := intervalToDuration(iv)

		go func() {
			first := time.Now().UTC().Truncate(d).Add(d)
			timer := time.NewTimer(time.Until(first))
			defer timer.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case tick := <-timer.C:
					a.flushInterval(ctx, iv, tick.Add(-d), tick)
					timer.Reset(d)
				}
			}
		}()
	}
}

// flushInterval publishes all bars for [start,end) for every symbol.
func (a *Aggregator) flushInterval(ctx context.Context, iv commonpb.AggregationInterval, start, end time.Time) {
	for _, symbol := range a.symbols {
		key := makeKey(symbol, iv, start)

		data, err := a.storage.Get(ctx, key)
		if err != nil {
			continue
		}
		_ = a.storage.Delete(ctx, key)

		bar, err := unmarshalBar(data)
		if err != nil {
			metrics.AggregationErrors.Inc()
			a.log.WithContext(ctx).Error("unmarshal bar", zap.String("key", key), zap.Error(err))
			continue
		}

		bar.OpenTime = timestamppb.New(start)
		bar.CloseTime = timestamppb.New(end)
		bar.Symbol = symbol

		payload, err := marshalBar(bar)
		if err != nil {
			metrics.AggregationErrors.Inc()
			a.log.WithContext(ctx).Error("marshal candle", zap.String("symbol", symbol), zap.Error(err))
			continue
		}

		pubCtx, span := tracer.Start(ctx, "PublishCandle",
			trace.WithAttributes(attribute.String("symbol", symbol), attribute.String("interval", iv.String())))
		begin := time.Now()
		err = a.producer.Publish(pubCtx, a.topic, nil, payload)
		span.End()

		if err != nil {
			metrics.PublishErrors.Inc()
			a.log.WithContext(ctx).Error("publish candle", zap.String("symbol", symbol), zap.Error(err))
			continue
		}

		metrics.CandlesPublished.Inc()
		metrics.PublishLatency.Observe(time.Since(begin).Seconds())
	}
}
