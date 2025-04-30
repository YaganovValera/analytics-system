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
)

var tracer = otel.Tracer("kafka-producer")

// Config holds Kafka producer settings.
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
	if c.FlushFrequency < 0 {
		c.FlushFrequency = 0
	}
	if c.FlushMessages < 0 {
		c.FlushMessages = 0
	}
}

func (c *Config) validate() error {
	if len(c.Brokers) == 0 {
		return fmt.Errorf("kafka: Brokers is required")
	}
	return nil
}

func buildSaramaConfig(c Config) (*sarama.Config, error) {
	sc := sarama.NewConfig()

	// RequiredAcks
	switch strings.ToLower(c.RequiredAcks) {
	case "", "all":
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

	// Batching
	sc.Producer.Flush.Frequency = c.FlushFrequency
	sc.Producer.Flush.Messages = c.FlushMessages

	// Compression
	switch strings.ToLower(c.Compression) {
	case "", "none":
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

	// Enable idempotence
	sc.Producer.Idempotent = true

	return sc, nil
}

// Producer wraps a sarama.SyncProducer with backoff, metrics, and tracing.
type Producer struct {
	prod       sarama.SyncProducer
	logger     *logger.Logger
	backoffCfg backoff.Config
}

// NewProducer creates and connects a Kafka SyncProducer with retries.
func NewProducer(ctx context.Context, cfg Config, log *logger.Logger) (*Producer, error) {
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	log = log.Named("kafka-producer")

	sc, err := buildSaramaConfig(cfg)
	if err != nil {
		return nil, err
	}

	var syncProd sarama.SyncProducer
	connect := func(ctx context.Context) error {
		producerConnectAttempts.Inc()
		var err error
		syncProd, err = sarama.NewSyncProducer(cfg.Brokers, sc)
		if err != nil {
			producerConnectFailures.Inc()
		}
		return err
	}

	// Trace the connection attempt
	ctx, span := tracer.Start(ctx, "Connect",
		trace.WithAttributes(attribute.StringSlice("brokers", cfg.Brokers)))
	if err := backoff.Execute(ctx, cfg.Backoff, log, connect); err != nil {
		span.RecordError(err)
		span.End()
		log.Error("kafka: connect failed", zap.Error(err))
		return nil, fmt.Errorf("kafka: NewProducer: %w", err)
	}
	span.End()

	// Wrap with OTEL instrumentation
	wrapped := otelsarama.WrapSyncProducer(sc, syncProd)
	log.Info("kafka: producer ready", zap.Strings("brokers", cfg.Brokers))

	return &Producer{prod: wrapped, logger: log, backoffCfg: cfg.Backoff}, nil
}

// Publish sends a message with retry, emits metrics, and traces.
func (p *Producer) Publish(ctx context.Context, topic string, key, value []byte) error {
	ctx, span := tracer.Start(ctx, "Publish",
		trace.WithAttributes(attribute.String("topic", topic)))
	start := time.Now()

	send := func(ctx context.Context) error {
		msg := &sarama.ProducerMessage{
			Topic: topic,
			Key:   sarama.ByteEncoder(key),
			Value: sarama.ByteEncoder(value),
		}
		_, _, err := p.prod.SendMessage(msg)
		return err
	}

	err := backoff.Execute(ctx, p.backoffCfg, p.logger, send)
	lat := time.Since(start).Seconds()
	producerPublishLatency.Observe(lat)

	if err != nil {
		producerPublishErrors.Inc()
		span.RecordError(err)
		p.logger.Error("kafka: publish failed",
			zap.String("topic", topic), zap.Error(err))
		span.End()
		return err
	}

	producerPublishSuccess.Inc()
	p.logger.Debug("kafka: publish succeeded",
		zap.String("topic", topic), zap.Float64("latency_s", lat))
	span.End()
	return nil
}

// Close gracefully closes the Kafka producer.
func (p *Producer) Close() error {
	if err := p.prod.Close(); err != nil {
		p.logger.Error("kafka: producer close failed", zap.Error(err))
		return err
	}
	p.logger.Info("kafka: producer closed")
	return nil
}
