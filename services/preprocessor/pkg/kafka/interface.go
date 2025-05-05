// services/preprocessor/pkg/kafka/interface.go
package kafka

import (
	"context"

	"github.com/IBM/sarama"
)

// Consumer описывает возможности Kafka consumer’а.
type Consumer interface {
	Subscribe(ctx context.Context, topics []string, handler func(*sarama.ConsumerMessage) error) error
	Close() error
}
