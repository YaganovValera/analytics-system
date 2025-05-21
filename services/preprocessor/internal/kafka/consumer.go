// preprocessor/internal/kafka/consumer.go

package kafka

import (
	"context"
	"fmt"

	"github.com/YaganovValera/analytics-system/common/kafka"
	"github.com/YaganovValera/analytics-system/common/logger"
	marketdatapb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/marketdata"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

// ProcessorFunc описывает функцию обработки одного сообщения.
type ProcessorFunc func(ctx context.Context, data *marketdatapb.MarketData) error

// Consumer запускает Kafka Consumer и обрабатывает сообщения из топика.
type Consumer struct {
	impl      kafka.Consumer
	topics    []string
	log       *logger.Logger
	processor ProcessorFunc
}

// New создает Consumer и подписывается на указанные топики.
func New(c kafka.Consumer, topics []string, processor ProcessorFunc, log *logger.Logger) *Consumer {
	return &Consumer{
		impl:      c,
		topics:    topics,
		log:       log.Named("kafka-consumer"),
		processor: processor,
	}
}

// Run запускает бесконечную обработку.
func (c *Consumer) Run(ctx context.Context) error {
	return c.impl.Consume(ctx, c.topics, func(msg *kafka.Message) error {
		data := &marketdatapb.MarketData{}
		if err := proto.Unmarshal(msg.Value, data); err != nil {
			c.log.WithContext(ctx).Error("unmarshal MarketData failed",
				zap.ByteString("raw", msg.Value),
				zap.Error(err),
			)

			return fmt.Errorf("invalid message: %w", err)
		}
		return c.processor(ctx, data)
	})
}

// Close завершает consumer group.
func (c *Consumer) Close() error {
	return c.impl.Close()
}
