// services/market-data-collector/pkg/kafka/consumer.go
package kafka

import (
	"context"
	"fmt"

	"github.com/IBM/sarama"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/YaganovValera/analytics-system/services/preprocessor/pkg/backoff"
	"github.com/YaganovValera/analytics-system/services/preprocessor/pkg/logger"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	consumerConnectAttempts = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "kafka_consumer", Name: "connect_attempts_total",
		Help: "Total number of attempts to create Kafka consumer group",
	})
	consumerConnectFailures = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "kafka_consumer", Name: "connect_failures_total",
		Help: "Total number of failed Kafka consumer connects",
	})
	messagesConsumed = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "kafka_consumer", Name: "messages_consumed_total",
		Help: "Total number of messages consumed",
	})
	handlerErrors = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "kafka_consumer", Name: "handler_errors_total",
		Help: "Total number of handler errors",
	})
	rebalanceEvents = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "kafka_consumer", Name: "rebalance_events_total",
		Help: "Total number of consumer group rebalance events",
	})
)

var tracer = otel.Tracer("kafka-consumer")

// Config хранит настройки для Kafka Consumer.
type Config struct {
	Brokers []string
	GroupID string
	Backoff backoff.Config
}

// consumerImpl — приватная реализация Consumer.
type consumerImpl struct {
	group   sarama.ConsumerGroup
	logger  *logger.Logger
	backoff backoff.Config
}

// NewConsumer создаёт и подключает Kafka ConsumerGroup с retry.
func NewConsumer(ctx context.Context, cfg Config, log *logger.Logger) (Consumer, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka: brokers required")
	}
	if cfg.GroupID == "" {
		return nil, fmt.Errorf("kafka: group_id required")
	}
	log = log.Named("kafka-consumer")

	// Sarama config
	sc := sarama.NewConfig()
	sc.Version = sarama.V2_8_0_0
	sc.Consumer.Offsets.Initial = sarama.OffsetOldest
	sc.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategyRange()

	var group sarama.ConsumerGroup
	connect := func(ctx context.Context) error {
		consumerConnectAttempts.Inc()
		var err error
		group, err = sarama.NewConsumerGroup(cfg.Brokers, cfg.GroupID, sc)
		if err != nil {
			consumerConnectFailures.Inc()
		}
		return err
	}

	// Retry connecting the group
	ctx, span := tracer.Start(ctx, "Consumer.Connect",
		trace.WithAttributes(attribute.StringSlice("brokers", cfg.Brokers),
			attribute.String("group_id", cfg.GroupID)))
	if err := backoff.Execute(ctx, cfg.Backoff, log, connect); err != nil {
		span.RecordError(err)
		span.End()
		log.Error("kafka-consumer: connect failed", zap.Error(err))
		return nil, fmt.Errorf("kafka: NewConsumer: %w", err)
	}
	span.End()

	log.Info("kafka-consumer: group ready", zap.Strings("brokers", cfg.Brokers), zap.String("group_id", cfg.GroupID))
	return &consumerImpl{group: group, logger: log, backoff: cfg.Backoff}, nil
}

// Subscribe запускает цикл Consume; handler вызывается на каждое сообщение.
func (c *consumerImpl) Subscribe(
	ctx context.Context,
	topics []string,
	handler func(*sarama.ConsumerMessage) error,
) error {
	h := &groupHandler{handler: handler, logger: c.logger}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.group.Consume(ctx, topics, h); err != nil {
			c.logger.Error("kafka-consumer: consume error", zap.Error(err))
			return err
		}
	}
}

// Close корректно закрывает consumer group.
func (c *consumerImpl) Close() error {
	return c.group.Close()
}

// groupHandler маршрутизует события ConsumerGroup.
type groupHandler struct {
	handler func(*sarama.ConsumerMessage) error
	logger  *logger.Logger
}

// Setup вызывается перед стартом новой сессии (rebalance event).
func (h *groupHandler) Setup(_ sarama.ConsumerGroupSession) error {
	rebalanceEvents.Inc()
	return nil
}

// Cleanup вызывается после окончания сессии.
func (h *groupHandler) Cleanup(_ sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim читает сообщения из claim и вызывает handler.
func (h *groupHandler) ConsumeClaim(
	session sarama.ConsumerGroupSession,
	claim sarama.ConsumerGroupClaim,
) error {
	for msg := range claim.Messages() {
		messagesConsumed.Inc()
		_, span := tracer.Start(session.Context(), "ConsumeMessage",
			trace.WithAttributes(
				attribute.String("topic", msg.Topic),
				attribute.Int64("partition", int64(msg.Partition)),
				attribute.Int64("offset", msg.Offset),
			),
		)
		if err := h.handler(msg); err != nil {
			handlerErrors.Inc()
			h.logger.Error("kafka-consumer: handler error", zap.Error(err),
				zap.String("topic", msg.Topic),
				zap.Int32("partition", msg.Partition),
				zap.Int64("offset", msg.Offset),
			)
			span.RecordError(err)
		}
		session.MarkMessage(msg, "")
		span.End()
	}
	return nil
}
