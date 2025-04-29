// pkg/kafka/producer.go
package kafka

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/IBM/sarama"
	"github.com/dnwe/otelsarama"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/backoff"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/logger"
)

// Config хранит настройки Kafka-продьюсера.
type Config struct {
	Brokers        []string       // список bootstrap-брокеров
	RequiredAcks   string         // "all", "leader" или "none" (по умолчанию "all")
	Timeout        time.Duration  // sarama.Producer.Timeout (по умолчанию 5s)
	Compression    string         // "none","gzip","snappy","lz4","zstd" (по умолчанию "none")
	FlushFrequency time.Duration  // sarama.Producer.Flush.Frequency (по умолчанию 0)
	FlushMessages  int            // sarama.Producer.Flush.Messages (по умолчанию 0)
	Backoff        backoff.Config // настройки initial-connect retry
}

// validate проверяет поля Config и заполняет дефолты.
func (c *Config) validate() error {
	var errs []string

	if len(c.Brokers) == 0 {
		errs = append(errs, "Brokers is required")
	}
	switch v := strings.ToLower(c.RequiredAcks); v {
	case "", "all", "leader", "none":
		// ok
	default:
		errs = append(errs, fmt.Sprintf("invalid RequiredAcks %q", c.RequiredAcks))
	}
	if c.Timeout <= 0 {
		c.Timeout = 5 * time.Second
	}
	switch v := strings.ToLower(c.Compression); v {
	case "", "none", "gzip", "snappy", "lz4", "zstd":
		// ok
	default:
		errs = append(errs, fmt.Sprintf("invalid Compression %q", c.Compression))
	}
	if c.FlushFrequency < 0 {
		errs = append(errs, "FlushFrequency must be >= 0")
	}
	if c.FlushMessages < 0 {
		errs = append(errs, "FlushMessages must be >= 0")
	}

	if len(errs) > 0 {
		return fmt.Errorf("kafka Config validation failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

// Producer обёртывает sarama.SyncProducer и наш Logger.
type Producer struct {
	prod       sarama.SyncProducer
	logger     *logger.Logger
	backoffCfg backoff.Config
}

// NewProducer создаёт и подключает SyncProducer с retry через backoff.
// Логгер именуется как "kafka-producer". При выходе из приложения
// вызовите log.Sync() для сброса буферов.
func NewProducer(ctx context.Context, cfg Config, log *logger.Logger) (*Producer, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	log = log.Named("kafka-producer")

	// 1) Инициализация переменной для Sarama
	var syncProd sarama.SyncProducer

	// 2) Функция подключения
	connect := func(ctxTry context.Context) error {
		sc := sarama.NewConfig()

		// RequiredAcks
		switch strings.ToLower(cfg.RequiredAcks) {
		case "", "all":
			sc.Producer.RequiredAcks = sarama.WaitForAll
		case "leader":
			sc.Producer.RequiredAcks = sarama.WaitForLocal
		case "none":
			sc.Producer.RequiredAcks = sarama.NoResponse
		}

		sc.Producer.Return.Successes = true
		sc.Producer.Return.Errors = true

		// Timeout и батчинг
		sc.Producer.Timeout = cfg.Timeout
		if f := cfg.FlushFrequency; f > 0 {
			sc.Producer.Flush.Frequency = f
		}
		if m := cfg.FlushMessages; m > 0 {
			sc.Producer.Flush.Messages = m
		}

		// Compression
		switch strings.ToLower(cfg.Compression) {
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
		}

		var err error
		syncProd, err = sarama.NewSyncProducer(cfg.Brokers, sc)
		return err
	}

	// 3) Retry initial connect
	if err := backoff.WithBackoff(ctx, cfg.Backoff, log, connect); err != nil {
		log.Sugar().Errorw("kafka: failed to create producer", "error", err)
		return nil, fmt.Errorf("NewProducer: %w", err)
	}

	// 4) Обёртка для OpenTelemetry
	wrapped := otelsarama.WrapSyncProducer(nil, syncProd)
	log.Sugar().Infow("kafka: producer ready", "brokers", cfg.Brokers)
	return &Producer{prod: wrapped, logger: log, backoffCfg: cfg.Backoff}, nil
}

// Publish отправляет сообщение в Kafka с retry и respects ctx cancellation.
func (p *Producer) Publish(ctx context.Context, topic string, key, value []byte) error {
	if err := ctx.Err(); err != nil {
		p.logger.Sugar().Warnw("kafka: publish cancelled before send", "topic", topic, "error", err)
		return err
	}

	op := func(ctxTry context.Context) error {
		msg := &sarama.ProducerMessage{
			Topic:     topic,
			Key:       sarama.ByteEncoder(key),
			Value:     sarama.ByteEncoder(value),
			Timestamp: time.Now(),
		}
		// Устанавливаем WriteDeadline под капотом Sarama через Timeout.
		partition, offset, err := p.prod.SendMessage(msg)
		if err != nil {
			p.logger.Sugar().Errorw("kafka: publish error", "topic", topic, "error", err)
			return err
		}
		p.logger.Sugar().Debugw("kafka: message sent", "topic", topic, "partition", partition, "offset", offset)
		return nil
	}

	if err := backoff.WithBackoff(ctx, p.backoffCfg, p.logger, op); err != nil {
		p.logger.Sugar().Errorw("kafka: publish giving up", "topic", topic, "error", err)
		return err
	}
	return nil
}

// Close корректно закрывает продьюсер и логирует ошибку, если она есть.
func (p *Producer) Close() error {
	if err := p.prod.Close(); err != nil {
		p.logger.Sugar().Errorw("kafka: producer close error", "error", err)
		return err
	}
	p.logger.Sugar().Infow("kafka: producer closed")
	return nil
}
