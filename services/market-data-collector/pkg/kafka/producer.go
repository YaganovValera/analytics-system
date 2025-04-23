package kafka

import (
	"context"

	"github.com/IBM/sarama"
)

type Producer struct {
	topic string
	prod  sarama.SyncProducer
}

func New(brokers []string, topic string) (*Producer, error) {
	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true
	p, err := sarama.NewSyncProducer(brokers, cfg)
	if err != nil {
		return nil, err
	}
	return &Producer{topic: topic, prod: p}, nil
}

func (p *Producer) Publish(ctx context.Context, msg []byte) error {
	_, _, err := p.prod.SendMessage(&sarama.ProducerMessage{
		Topic: p.topic,
		Value: sarama.ByteEncoder(msg),
	})
	return err
}
