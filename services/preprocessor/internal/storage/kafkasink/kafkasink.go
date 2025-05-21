// preprocessor/internal/storage/kafkasink/kafkasink.go

package kafkasink

import (
	"context"
	"fmt"

	commonkafka "github.com/YaganovValera/analytics-system/common/kafka"
	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/aggregator"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/transport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var tracer = otel.Tracer("preprocessor/storage/kafkasink")

// Sink публикует завершённые свечи в Kafka.
type Sink struct {
	producer commonkafka.Producer
	prefix   string // e.g. "candles"
	log      *logger.Logger
}

// New создаёт новый Kafka sink.
func New(producer commonkafka.Producer, topicPrefix string, log *logger.Logger) *Sink {
	return &Sink{
		producer: producer,
		prefix:   topicPrefix,
		log:      log.Named("kafka-sink"),
	}
}

// FlushCandle сериализует и публикует Candle в топик candles.{interval} с ключом symbol.
func (s *Sink) FlushCandle(ctx context.Context, c *aggregator.Candle) error {
	ctx, span := tracer.Start(ctx, "KafkaSink.FlushCandle",
		trace.WithAttributes(
			attribute.String("symbol", c.Symbol),
			attribute.String("interval", c.Interval),
		),
	)
	defer span.End()

	bytes, err := transport.MarshalCandleToKafka(ctx, c)
	if err != nil {
		s.log.WithContext(ctx).Error("marshal candle failed", zap.Error(err))
		return fmt.Errorf("kafka-sink: marshal: %w", err)
	}

	topic := fmt.Sprintf("%s.%s", s.prefix, c.Interval)
	key := []byte(c.Symbol)

	if err := s.producer.Publish(ctx, topic, key, bytes); err != nil {
		s.log.WithContext(ctx).Error("publish failed", zap.Error(err))
		return fmt.Errorf("kafka-sink: publish: %w", err)
	}

	return nil
}
