// github.com/YaganovValera/analytics-system/services/preprocessor/internal/app/preprocessor.go

// services/preprocessor/internal/app/preprocessor.go
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/YaganovValera/analytics-system/common/httpserver"
	"github.com/YaganovValera/analytics-system/common/kafka/consumer"
	"github.com/YaganovValera/analytics-system/common/kafka/producer"
	"github.com/YaganovValera/analytics-system/common/logger"
	commonredis "github.com/YaganovValera/analytics-system/common/redis"
	"github.com/YaganovValera/analytics-system/common/serviceid"
	"github.com/YaganovValera/analytics-system/common/telemetry"

	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/aggregator"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/config"
	precons "github.com/YaganovValera/analytics-system/services/preprocessor/internal/kafka"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/metrics"
	kafkasink "github.com/YaganovValera/analytics-system/services/preprocessor/internal/storage/kafkasink"
	redisadapter "github.com/YaganovValera/analytics-system/services/preprocessor/internal/storage/redis"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/storage/timescaledb"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// Run запускает preprocessor-сервис согласно production-стилю.
func Run(ctx context.Context, cfg *config.Config, log *logger.Logger) error {
	// Инициализируем имя для метрик и трейсинга
	serviceid.InitServiceName(cfg.ServiceName)

	// Регистрируем доменные метрики
	metrics.Register(nil)

	// === Telemetry ===
	cfg.Telemetry.ServiceName = cfg.ServiceName
	cfg.Telemetry.ServiceVersion = cfg.ServiceVersion
	cfg.Telemetry.ApplyDefaults()
	if err := cfg.Telemetry.Validate(); err != nil {
		return fmt.Errorf("telemetry config invalid: %w", err)
	}
	shutdownTracer, err := telemetry.InitTracer(ctx, cfg.Telemetry, log)
	if err != nil {
		return fmt.Errorf("init tracer: %w", err)
	}
	defer shutdownSafe(ctx, "telemetry", shutdownTracer, log)

	// === Redis (PartialBarStorage) ===
	cfg.Redis.ApplyDefaults()
	if err := cfg.Redis.Validate(); err != nil {
		return fmt.Errorf("redis config invalid: %w", err)
	}
	rcli, err := commonredis.New(cfg.Redis, log)
	if err != nil {
		return fmt.Errorf("redis init: %w", err)
	}
	defer shutdownSafe(ctx, "redis", func(ctx context.Context) error { return rcli.Close() }, log)
	storage := redisadapter.NewAdapter(rcli)

	// === TimescaleDB ===
	if err := timescaledb.ApplyMigrations(cfg.Timescale, log); err != nil {
		return fmt.Errorf("timescaledb migrations: %w", err)
	}
	tsWriter, err := timescaledb.NewTimescaleWriter(cfg.Timescale, log)
	if err != nil {
		return fmt.Errorf("timescaledb init: %w", err)
	}
	defer shutdownSafe(ctx, "timescaledb", func(ctx context.Context) error { tsWriter.Close(); return nil }, log)

	// === Kafka Producer ===
	cfg.KafkaProducer.ApplyDefaults()
	if err := cfg.KafkaProducer.Validate(); err != nil {
		return fmt.Errorf("kafka_producer config invalid: %w", err)
	}
	kprod, err := producer.New(ctx, cfg.KafkaProducer, log)
	if err != nil {
		return fmt.Errorf("kafka producer init: %w", err)
	}
	defer shutdownSafe(ctx, "kafka-producer", func(ctx context.Context) error { return kprod.Close() }, log)

	// Sink для агрегированных свечей
	kSink := kafkasink.New(kprod, cfg.OutputTopicPrefix, log)
	flushSink := aggregator.NewMultiSink(tsWriter, kSink)

	// === Aggregator ===
	agg, err := aggregator.NewManager(cfg.Intervals, flushSink, storage, log)
	if err != nil {
		return fmt.Errorf("aggregator init: %w", err)
	}
	defer shutdownSafe(ctx, "aggregator", func(ctx context.Context) error { return agg.Close() }, log)

	// === Kafka Consumer ===
	cfg.KafkaConsumer.ApplyDefaults()
	if err := cfg.KafkaConsumer.Validate(); err != nil {
		return fmt.Errorf("kafka_consumer config invalid: %w", err)
	}
	kcons, err := consumer.New(ctx, cfg.KafkaConsumer, log)
	if err != nil {
		return fmt.Errorf("kafka consumer init: %w", err)
	}
	defer shutdownSafe(ctx, "kafka-consumer", func(ctx context.Context) error { return kcons.Close() }, log)

	streamer := precons.New(kcons, []string{cfg.RawTopic}, agg.Process, log)

	// === HTTP Server ===
	cfg.HTTP.ApplyDefaults()
	if err := cfg.HTTP.Validate(); err != nil {
		return fmt.Errorf("http config invalid: %w", err)
	}
	readiness := func() error {
		ctxPing, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return tsWriter.Ping(ctxPing)
	}
	httpSrv, err := httpserver.New(
		cfg.HTTP,
		readiness,
		log,
		nil,
		httpserver.RecoverMiddleware,
		httpserver.CORSMiddleware(),
	)
	if err != nil {
		return fmt.Errorf("httpserver init: %w", err)
	}

	// === Запуск ===
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return httpSrv.Run(ctx) })
	g.Go(func() error { return streamer.Run(ctx) })

	if err := g.Wait(); err != nil {
		if ctx.Err() == context.Canceled {
			log.WithContext(ctx).Info("preprocessor shut down cleanly")
			return nil
		}
		return fmt.Errorf("preprocessor exited with error: %w", err)
	}

	return nil
}

// shutdownSafe выполняет корректное завершение компонентов с логированием.
func shutdownSafe(ctx context.Context, name string, fn func(context.Context) error, log *logger.Logger) {
	log.WithContext(ctx).Info(name + ": shutting down")
	if err := fn(ctx); err != nil {
		log.WithContext(ctx).Error(name+" shutdown failed", zap.Error(err))
	} else {
		log.WithContext(ctx).Info(name + ": shutdown complete")
	}
}
