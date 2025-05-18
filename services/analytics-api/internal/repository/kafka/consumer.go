// github.com/YaganovValera/analytics-system/services/analytics-api/internal/repository/kafka/consumer.go
// internal/repository/kafka/consumer.go
package kafka

import (
	"context"
	"fmt"
	"strings"

	commonkafka "github.com/YaganovValera/analytics-system/common/kafka"
	commonconsumer "github.com/YaganovValera/analytics-system/common/kafka/consumer"
	"github.com/YaganovValera/analytics-system/common/logger"
	analyticspb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/analytics"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/config"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/transport"

	"go.uber.org/zap"
)

type Repository interface {
	ConsumeCandles(ctx context.Context, topic string, symbol string) (<-chan *analyticspb.Candle, error)
	Close() error
}

type kafkaRepo struct {
	consumer commonkafka.Consumer
	log      *logger.Logger
}

func New(ctx context.Context, cfg config.KafkaConfig, log *logger.Logger) (Repository, error) {
	consumer, err := commonconsumer.New(ctx, commonconsumer.Config{
		Brokers: cfg.Brokers,
		GroupID: cfg.GroupID,
		Version: cfg.Version,
		Backoff: cfg.Backoff,
	}, log)
	if err != nil {
		return nil, fmt.Errorf("kafka consumer init: %w", err)
	}
	return &kafkaRepo{consumer: consumer, log: log.Named("kafka")}, nil
}

func (r *kafkaRepo) ConsumeCandles(ctx context.Context, topic string, symbol string) (<-chan *analyticspb.Candle, error) {
	out := make(chan *analyticspb.Candle, 100)
	go func() {
		err := r.consumer.Consume(ctx, []string{topic}, func(msg *commonkafka.Message) error {
			if len(msg.Key) > 0 && !strings.EqualFold(string(msg.Key), symbol) {
				return nil
			}
			candle, err := transport.UnmarshalCandleFromBytes(msg.Value)
			if err != nil {
				r.log.WithContext(ctx).Error("unmarshal candle failed", zap.Error(err))
				return nil
			}
			out <- candle
			return nil
		})
		if err != nil {
			r.log.WithContext(ctx).Error("kafka consume failed", zap.Error(err))
		}
		close(out)
	}()
	return out, nil
}

func (r *kafkaRepo) Close() error {
	return r.consumer.Close()
}
