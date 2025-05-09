// common/kafka/producer/producer.go
package producer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/IBM/sarama"
	"github.com/dnwe/otelsarama"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/YaganovValera/analytics-system/common/backoff"
	commonkafka "github.com/YaganovValera/analytics-system/common/kafka"
	"github.com/YaganovValera/analytics-system/common/logger"
)

// -----------------------------------------------------------------------------
// Service label (заполняется через common.InitServiceName)
// -----------------------------------------------------------------------------

var serviceLabel = "unknown"

// SetServiceLabel вызывается из common.InitServiceName(..) один раз при старте.
func SetServiceLabel(name string) { serviceLabel = name }

// -----------------------------------------------------------------------------
// Prometheus-метрики
// -----------------------------------------------------------------------------

var producerMetrics = struct {
	ConnectAttempts *prometheus.CounterVec
	ConnectErrors   *prometheus.CounterVec
	PublishSuccess  *prometheus.CounterVec
	PublishErrors   *prometheus.CounterVec
	PublishLatency  *prometheus.HistogramVec
	PingSuccess     *prometheus.CounterVec
	PingErrors      *prometheus.CounterVec
}{
	ConnectAttempts: promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "common", Subsystem: "kafka_producer", Name: "connect_attempts_total",
			Help: "Kafka producer connect attempts",
		},
		[]string{"service"},
	),
	ConnectErrors: promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "common", Subsystem: "kafka_producer", Name: "connect_errors_total",
			Help: "Kafka producer connect errors",
		},
		[]string{"service"},
	),
	PublishSuccess: promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "common", Subsystem: "kafka_producer", Name: "publish_success_total",
			Help: "Successful publishes",
		},
		[]string{"service"},
	),
	PublishErrors: promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "common", Subsystem: "kafka_producer", Name: "publish_errors_total",
			Help: "Publish errors",
		},
		[]string{"service"},
	),
	PublishLatency: promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "common", Subsystem: "kafka_producer", Name: "publish_latency_seconds",
			Help:    "Publish latency (seconds)",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service"},
	),
	PingSuccess: promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "common", Subsystem: "kafka_producer", Name: "ping_success_total",
			Help: "Successful pings",
		},
		[]string{"service"},
	),
	PingErrors: promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "common", Subsystem: "kafka_producer", Name: "ping_errors_total",
			Help: "Ping errors",
		},
		[]string{"service"},
	),
}

// -----------------------------------------------------------------------------
// Tracing
// -----------------------------------------------------------------------------

var tracer = otel.Tracer("kafka-producer")

// -----------------------------------------------------------------------------
// Configuration
// -----------------------------------------------------------------------------

// Config groups all tunables for a Kafka Sync-producer.
//
// Zero values are replaced with sane defaults by applyDefaults().
type Config struct {
	// Brokers — список адресов Kafka-брокеров.
	Brokers []string

	// RequiredAcks определяет стратегию подтверждения брокеров:
	//   "all" (дефолт) | "leader" | "none".
	RequiredAcks string

	// Timeout — максимальное время ожидания ack от кластера.
	Timeout time.Duration

	// Compression указывает алгоритм сжатия:
	//   "none" (дефолт), "gzip", "snappy", "lz4", "zstd".
	Compression string

	// FlushFrequency — периодическое «смывание» буфера продьюсера.
	// Ноль → disable.
	FlushFrequency time.Duration

	// FlushMessages — пороговое кол-во сообщений для смыва.
	// Ноль → disable.
	FlushMessages int

	// Backoff описывает стратегию ретраев подключения и отправки.
	Backoff backoff.Config
}

// applyDefaults заполняет zero-полям безопасные дефолты.
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

// validate выполняет быстрые sanity-checks.
func (c Config) validate() error {
	if len(c.Brokers) == 0 {
		return fmt.Errorf("kafka producer: brokers required")
	}
	return nil
}

// -----------------------------------------------------------------------------
// Private helpers
// -----------------------------------------------------------------------------

func buildSaramaConfig(c Config) (*sarama.Config, error) {
	sc := sarama.NewConfig()

	// RequiredAcks
	switch strings.ToLower(c.RequiredAcks) {
	case "all":
		sc.Producer.RequiredAcks = sarama.WaitForAll
	case "leader":
		sc.Producer.RequiredAcks = sarama.WaitForLocal
	case "none":
		sc.Producer.RequiredAcks = sarama.NoResponse
	default:
		return nil, fmt.Errorf("kafka producer: invalid RequiredAcks %q", c.RequiredAcks)
	}

	// Producer common settings
	sc.Producer.Return.Successes = true
	sc.Producer.Return.Errors = true
	sc.Producer.Timeout = c.Timeout
	sc.Producer.Idempotent = true
	sc.Net.MaxOpenRequests = 1

	// Flush params
	if c.FlushFrequency > 0 {
		sc.Producer.Flush.Frequency = c.FlushFrequency
	}
	if c.FlushMessages > 0 {
		sc.Producer.Flush.Messages = c.FlushMessages
	}

	// Compression
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
		return nil, fmt.Errorf("kafka producer: invalid Compression %q", c.Compression)
	}

	return sc, nil
}

// -----------------------------------------------------------------------------
// Producer implementation
// -----------------------------------------------------------------------------

type kafkaProducer struct {
	prod       sarama.SyncProducer
	client     sarama.Client
	logger     *logger.Logger
	backoffCfg backoff.Config
}

// New создает SyncProducer c ретраями подключения.
func New(ctx context.Context, cfg Config, log *logger.Logger) (commonkafka.Producer, error) {
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	log = log.Named("kafka-producer")

	// Sarama config
	sc, err := buildSaramaConfig(cfg)
	if err != nil {
		return nil, err
	}

	// Kafka client
	client, err := sarama.NewClient(cfg.Brokers, sc)
	if err != nil {
		return nil, fmt.Errorf("kafka producer: new client: %w", err)
	}

	// Создаем продьюсер с back-off-подключением
	var syncProd sarama.SyncProducer
	connect := func(ctx context.Context) error {
		producerMetrics.ConnectAttempts.WithLabelValues(serviceLabel).Inc()
		p, err := sarama.NewSyncProducerFromClient(client)
		if err != nil {
			producerMetrics.ConnectErrors.WithLabelValues(serviceLabel).Inc()
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
		_ = client.Close()
		log.Error("kafka producer connect failed", zap.Error(err))
		return nil, fmt.Errorf("kafka producer: connect: %w", err)
	}
	span.End()

	// Оборачиваем для OpenTelemetry
	wrapped := otelsarama.WrapSyncProducer(sc, syncProd)

	log.Info("kafka producer ready", zap.Strings("brokers", cfg.Brokers))
	return &kafkaProducer{
		prod:       wrapped,
		client:     client,
		logger:     log,
		backoffCfg: cfg.Backoff,
	}, nil
}

// Publish отправляет сообщение в Kafka c ретраями.
func (k *kafkaProducer) Publish(ctx context.Context, topic string, key, value []byte) error {
	ctxPub, span := tracer.Start(ctx, "Publish", trace.WithAttributes(attribute.String("topic", topic)))
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
	latency := time.Since(start)
	producerMetrics.PublishLatency.WithLabelValues(serviceLabel).Observe(latency.Seconds())

	if err != nil {
		producerMetrics.PublishErrors.WithLabelValues(serviceLabel).Inc()
		span.RecordError(err)
		k.logger.Error("publish failed", zap.String("topic", topic), zap.Error(err))
		span.End()
		return err
	}

	producerMetrics.PublishSuccess.WithLabelValues(serviceLabel).Inc()
	k.logger.Debug("publish succeeded",
		zap.String("topic", topic),
		zap.Float64("latency_s", latency.Seconds()),
	)
	span.End()
	return nil
}

// Ping обновляет метаданные клиента, проверяя доступность кластера.
func (k *kafkaProducer) Ping(ctx context.Context) error {
	_, span := tracer.Start(ctx, "Ping")
	err := k.client.RefreshMetadata()
	if err != nil {
		producerMetrics.PingErrors.WithLabelValues(serviceLabel).Inc()
		span.RecordError(err)
	} else {
		producerMetrics.PingSuccess.WithLabelValues(serviceLabel).Inc()
	}
	span.End()
	return err
}

// Close корректно закрывает продьюсер и клиент.
func (k *kafkaProducer) Close() error {
	if err := k.prod.Close(); err != nil {
		k.logger.Error("producer close failed", zap.Error(err))
		return err
	}
	if err := k.client.Close(); err != nil {
		k.logger.Error("client close failed", zap.Error(err))
		return err
	}
	k.logger.Info("kafka producer closed")
	return nil
}
