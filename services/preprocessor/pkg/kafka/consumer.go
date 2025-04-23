// preprocessor/pkg/kafka/consumer.go
package kafka

import (
	"context"
	"log"

	"github.com/IBM/sarama"
)

// ConsumerGroup wraps sarama.ConsumerGroup
type ConsumerGroup struct {
	group  sarama.ConsumerGroup
	topics []string
}

// NewConsumerGroup создаёт нового ConsumerGroup для заданных брокеров, groupID и тем
func NewConsumerGroup(brokers []string, groupID string, topics []string) (*ConsumerGroup, error) {
	cfg := sarama.NewConfig()
	// Настройки
	cfg.Version = sarama.V2_1_0_0
	cfg.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategyRoundRobin()
	cfg.Consumer.Offsets.Initial = sarama.OffsetNewest

	group, err := sarama.NewConsumerGroup(brokers, groupID, cfg)
	if err != nil {
		return nil, err
	}
	return &ConsumerGroup{group: group, topics: topics}, nil
}

// Start запускает бесконечный цикл Consume в отдельной горутине.
// handler — функция, которая будет вызываться для каждого сообщения.
func (c *ConsumerGroup) Start(ctx context.Context, handler func(message []byte)) {
	go func() {
		for {
			if err := c.group.Consume(ctx, c.topics, &handlerGroup{handler: handler}); err != nil {
				log.Printf("kafka consume error: %v", err)
			}
			// При завершении контекста выходим из цикла
			if ctx.Err() != nil {
				return
			}
		}
	}()
}

// Close аккуратно закрывает ConsumerGroup
func (c *ConsumerGroup) Close() error {
	return c.group.Close()
}

// handlerGroup реализует sarama.ConsumerGroupHandler
type handlerGroup struct {
	handler func([]byte)
}

func (h *handlerGroup) Setup(session sarama.ConsumerGroupSession) error   { return nil }
func (h *handlerGroup) Cleanup(session sarama.ConsumerGroupSession) error { return nil }

func (h *handlerGroup) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		h.handler(msg.Value)
		session.MarkMessage(msg, "")
	}
	return nil
}
