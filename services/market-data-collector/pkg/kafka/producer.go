// services/market-data-collector/pkg/kafka/producer.go
package kafka

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/IBM/sarama"
	"github.com/dnwe/otelsarama"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/backoff"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/logger"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	producerConnectAttempts = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "kafka_producer", Name: "connect_attempts_total",
		Help: "Total number of attempts to connect Kafka producer",
	})
	producerConnectFailures = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "kafka_producer", Name: "connect_failures_total",
		Help: "Total number of failed Kafka producer connects",
	})
	producerPublishSuccess = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "kafka_producer", Name: "publish_success_total",
		Help: "Total number of successful Kafka publishes",
	})
	producerPublishErrors = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "kafka_producer", Name: "publish_errors_total",
		Help: "Total number of failed Kafka publishes",
	})
	producerPublishLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "collector", Subsystem: "kafka_producer", Name: "publish_latency_seconds",
		Help:    "Histogram of Kafka publish latency in seconds",
		Buckets: prometheus.DefBuckets,
	})
	producerPingSuccess = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "kafka_producer", Name: "ping_success_total",
		Help: "Total number of successful Kafka pings",
	})
	producerPingErrors = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "kafka_producer", Name: "ping_errors_total",
		Help: "Total number of failed Kafka pings",
	})
)

var tracer = otel.Tracer("kafka-producer")

// Config хранит настройки Kafka продьюсера.
type Config struct {
	Brokers        []string
	RequiredAcks   string
	Timeout        time.Duration
	Compression    string
	FlushFrequency time.Duration
	FlushMessages  int
	Backoff        backoff.Config
}

func (c *Config) applyDefaults() {
	if c.Timeout <= 0 {
		c.Timeout = 5 * time.Second
	}
	if c.RequiredAcks == "" {
		c.RequiredAcks = "all"
	}
	if c.Compression == "" {
		c.Compression = "none"
	}
}

func (c *Config) validate() error {
	if len(c.Brokers) == 0 {
		return fmt.Errorf("kafka: brokers required")
	}
	return nil
}

func buildSaramaConfig(c Config) (*sarama.Config, error) {
	sc := sarama.NewConfig()
	switch strings.ToLower(c.RequiredAcks) {
	case "all":
		sc.Producer.RequiredAcks = sarama.WaitForAll
	case "leader":
		sc.Producer.RequiredAcks = sarama.WaitForLocal
	case "none":
		sc.Producer.RequiredAcks = sarama.NoResponse
	default:
		return nil, fmt.Errorf("kafka: invalid RequiredAcks %q", c.RequiredAcks)
	}
	sc.Producer.Return.Successes = true
	sc.Producer.Return.Errors = true
	sc.Producer.Timeout = c.Timeout

	if f := c.FlushFrequency; f > 0 {
		sc.Producer.Flush.Frequency = f
	}
	if m := c.FlushMessages; m > 0 {
		sc.Producer.Flush.Messages = m
	}

	switch strings.ToLower(c.Compression) {
	case "none":
		sc.Producer.Compression = sarama.CompressionNone
	case "gzip":
		sc.Producer.Compression = sarama.CompressionGZIP
	case "snappy":
		sc.Producer.Compression = sarama.CompressionSnappy
	case "lz4":
		sc.Producer.Compression = sarama.CompressionLZ4
	case "zstd":
		sc.Producer.Compression = sarama.CompressionZSTD
	default:
		return nil, fmt.Errorf("kafka: invalid Compression %q", c.Compression)
	}

	sc.Producer.Idempotent = true
	sc.Net.MaxOpenRequests = 1

	return sc, nil
}

type kafkaProducer struct {
	prod       sarama.SyncProducer
	client     sarama.Client
	logger     *logger.Logger
	backoffCfg backoff.Config
}

func NewProducer(ctx context.Context, cfg Config, log *logger.Logger) (Producer, error) {
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	log = log.Named("kafka-producer")

	sc, err := buildSaramaConfig(cfg)
	if err != nil {
		return nil, err
	}

	client, err := sarama.NewClient(cfg.Brokers, sc)
	if err != nil {
		return nil, fmt.Errorf("kafka: new client: %w", err)
	}

	// при ошибке ниже обязательно закрываем client
	var syncProd sarama.SyncProducer
	connect := func(ctx context.Context) error {
		producerConnectAttempts.Inc()
		p, err := sarama.NewSyncProducerFromClient(client)
		if err != nil {
			producerConnectFailures.Inc()
			return err
		}
		syncProd = p
		return nil
	}

	ctxConn, span := tracer.Start(ctx, "Connect",
		trace.WithAttributes(attribute.StringSlice("brokers", cfg.Brokers)))
	if err := backoff.Execute(ctxConn, cfg.Backoff, log, connect); err != nil {
		span.RecordError(err)
		span.End()
		client.Close()
		log.Error("kafka: connect failed", zap.Error(err))
		return nil, fmt.Errorf("kafka: NewProducer: %w", err)
	}
	span.End()

	wrapped := otelsarama.WrapSyncProducer(sc, syncProd)
	log.Info("kafka: producer ready", zap.Strings("brokers", cfg.Brokers))

	return &kafkaProducer{
		prod:       wrapped,
		client:     client,
		logger:     log,
		backoffCfg: cfg.Backoff,
	}, nil
}

func (k *kafkaProducer) Publish(ctx context.Context, topic string, key, value []byte) error {
	ctxPub, span := tracer.Start(ctx, "Publish",
		trace.WithAttributes(attribute.String("topic", topic)))
	start := time.Now()

	send := func(ctx context.Context) error {
		msg := &sarama.ProducerMessage{
			Topic: topic,
			Key:   sarama.ByteEncoder(key),
			Value: sarama.ByteEncoder(value),
		}
		_, _, err := k.prod.SendMessage(msg)
		return err
	}

	err := backoff.Execute(ctxPub, k.backoffCfg, k.logger, send)
	lat := time.Since(start).Seconds()
	producerPublishLatency.Observe(lat)

	if err != nil {
		producerPublishErrors.Inc()
		span.RecordError(err)
		k.logger.Error("kafka: publish failed",
			zap.String("topic", topic), zap.Error(err))
		span.End()
		return err
	}

	producerPublishSuccess.Inc()
	k.logger.Debug("kafka: publish succeeded",
		zap.String("topic", topic), zap.Float64("latency_s", lat))
	span.End()
	return nil
}

func (k *kafkaProducer) Ping() error {
	_, span := tracer.Start(context.Background(), "Ping")
	err := k.client.RefreshMetadata()
	if err != nil {
		producerPingErrors.Inc()
		span.RecordError(err)
	} else {
		producerPingSuccess.Inc()
	}
	span.End()
	return err
}

func (k *kafkaProducer) Close() error {
	errProd := k.prod.Close()
	if errProd != nil {
		k.logger.Error("kafka: producer close failed", zap.Error(errProd))
	}
	errClient := k.client.Close()
	if errClient != nil {
		k.logger.Error("kafka: client close failed", zap.Error(errClient))
	}
	k.logger.Info("kafka: producer closed")
	if errProd != nil {
		return errProd
	}
	return errClient
}
