// analytics-api/internal/storage/kafka/consumer.go
package kafka

import (
	"context"
	"fmt"
	"strings"

	commonkafka "github.com/YaganovValera/analytics-system/common/kafka"
	consumerkafka "github.com/YaganovValera/analytics-system/common/kafka/consumer"
	"github.com/YaganovValera/analytics-system/common/logger"
	analyticspb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/analytics"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/transport"

	"go.uber.org/zap"
	rpcstatus "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
)

// Repository описывает интерфейс чтения свечей из Kafka.
type Repository interface {
	ConsumeCandles(ctx context.Context, topic string, symbol string) (<-chan *analyticspb.CandleEvent, error)
	Close() error
}

type kafkaRepo struct {
	consumer commonkafka.Consumer
	log      *logger.Logger
}

func New(ctx context.Context, cfg consumerkafka.Config, log *logger.Logger) (Repository, error) {
	cons, err := consumerkafka.New(ctx, cfg, log)
	if err != nil {
		return nil, fmt.Errorf("kafka consumer init: %w", err)
	}
	return &kafkaRepo{consumer: cons, log: log.Named("kafka")}, nil
}

func (r *kafkaRepo) ConsumeCandles(ctx context.Context, topic string, symbol string) (<-chan *analyticspb.CandleEvent, error) {
	out := make(chan *analyticspb.CandleEvent, 100)

	go func() {
		defer close(out)
		err := r.consumer.Consume(ctx, []string{topic}, func(msg *commonkafka.Message) error {
			if len(msg.Key) > 0 && !strings.EqualFold(string(msg.Key), symbol) {
				return nil // пропускаем чужие symbol'ы
			}

			candle, err := transport.UnmarshalCandleFromBytes(msg.Value)
			if err != nil {
				r.log.WithContext(ctx).Error("unmarshal candle failed", zap.Error(err))
				out <- &analyticspb.CandleEvent{
					Payload: &analyticspb.CandleEvent_Error{
						Error: &rpcstatus.Status{
							Code:    int32(codes.Internal),
							Message: "failed to unmarshal candle",
						},
					},
				}
				return nil // не прерываем поток
			}

			out <- &analyticspb.CandleEvent{
				Payload: &analyticspb.CandleEvent_Candle{
					Candle: candle,
				},
			}
			return nil
		})

		if err != nil && ctx.Err() == nil {
			r.log.WithContext(ctx).Error("kafka consume failed", zap.Error(err))
			out <- &analyticspb.CandleEvent{
				Payload: &analyticspb.CandleEvent_Error{
					Error: &rpcstatus.Status{
						Code:    int32(codes.Unavailable),
						Message: fmt.Sprintf("kafka consume failed: %v", err),
					},
				},
			}
		}
	}()

	return out, nil
}

func (r *kafkaRepo) Close() error {
	return r.consumer.Close()
}
