// github.com/YaganovValera/analytics-system/services/preprocessor/internal/app/preprocessor.go

// internal/app/preprocessor.go
package app

import (
	"context"
	"fmt"
	"time"

	httpserver "github.com/YaganovValera/analytics-system/common/httpserver"
	"github.com/YaganovValera/analytics-system/common/kafka/consumer"
	"github.com/YaganovValera/analytics-system/common/kafka/producer"
	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/YaganovValera/analytics-system/common/serviceid"
	"github.com/YaganovValera/analytics-system/common/telemetry"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/aggregator"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/config"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/kafka"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/metrics"
	kafkasink "github.com/YaganovValera/analytics-system/services/preprocessor/internal/storage/kafkasink"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/storage/redis"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/storage/timescaledb"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

func Run(ctx context.Context, cfg *config.Config, log *logger.Logger) error {
	serviceid.InitServiceName(cfg.ServiceName)
	metrics.Register(nil)

	shutdownTracer, err := telemetry.InitTracer(ctx, telemetry.Config{
		Endpoint:        cfg.Telemetry.OTLPEndpoint,
		ServiceName:     cfg.ServiceName,
		ServiceVersion:  cfg.ServiceVersion,
		Insecure:        cfg.Telemetry.Insecure,
		SamplerRatio:    1.0,
		ReconnectPeriod: 5 * time.Second,
		Timeout:         5 * time.Second,
	}, log)
	if err != nil {
		return fmt.Errorf("init tracer: %w", err)
	}
	defer shutdownSafe(ctx, "telemetry", shutdownTracer, log)

	rstore, err := redis.NewRedisStorage(cfg.Redis, log)
	if err != nil {
		return fmt.Errorf("redis: %w", err)
	}
	defer shutdownSafe(ctx, "redis", func(ctx context.Context) error {
		return rstore.Close()
	}, log)

	if err := timescaledb.ApplyMigrations(cfg.Timescale.DSN, cfg.Timescale.MigrationsDir, log); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	timescale, err := timescaledb.NewTimescaleWriter(cfg.Timescale, log)
	if err != nil {
		return fmt.Errorf("timescaledb: %w", err)
	}
	defer timescale.Close()

	kprod, err := producer.New(ctx, producer.Config{
		Brokers: cfg.Kafka.Brokers,
		Backoff: cfg.Kafka.Backoff,
	}, log)
	if err != nil {
		return fmt.Errorf("kafka-producer: %w", err)
	}
	defer shutdownSafe(ctx, "kafka-producer", func(ctx context.Context) error {
		return kprod.Close()
	}, log)

	kafkaSink := kafkasink.New(kprod, cfg.Kafka.OutputTopicPrefix, log)
	flushSink := aggregator.NewMultiSink(timescale, kafkaSink)

	agg, err := aggregator.NewManager(cfg.Intervals, flushSink, rstore, log)
	if err != nil {
		return fmt.Errorf("aggregator: %w", err)
	}
	defer agg.Close()

	kc, err := consumer.New(ctx, consumer.Config{
		Brokers: cfg.Kafka.Brokers,
		GroupID: cfg.Kafka.GroupID,
		Version: cfg.Kafka.Version,
		Backoff: cfg.Kafka.Backoff,
	}, log)
	if err != nil {
		return fmt.Errorf("kafka-consumer: %w", err)
	}
	defer kc.Close()

	consumer := kafka.New(kc, []string{cfg.Kafka.RawTopic}, agg.Process, log)

	readiness := func() error {
		ctxPing, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := timescale.Ping(ctxPing); err != nil {
			return fmt.Errorf("timescaledb not ready: %w", err)
		}
		return nil
	}

	httpSrv, err := httpserver.New(httpserver.Config{
		Addr:            fmt.Sprintf(":%d", cfg.HTTP.Port),
		ReadTimeout:     cfg.HTTP.ReadTimeout,
		WriteTimeout:    cfg.HTTP.WriteTimeout,
		IdleTimeout:     cfg.HTTP.IdleTimeout,
		ShutdownTimeout: cfg.HTTP.ShutdownTimeout,
		MetricsPath:     cfg.HTTP.MetricsPath,
		HealthzPath:     cfg.HTTP.HealthzPath,
		ReadyzPath:      cfg.HTTP.ReadyzPath,
	}, readiness, log, nil)
	if err != nil {
		return fmt.Errorf("httpserver: %w", err)
	}

	log.WithContext(ctx).Info("preprocessor: components initialized, starting run loops")
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error { return httpSrv.Run(ctx) })
	g.Go(func() error { return consumer.Run(ctx) })

	if err := g.Wait(); err != nil {
		if ctx.Err() == context.Canceled {
			log.WithContext(ctx).Info("preprocessor exited on context cancel")
			return nil
		}
		log.WithContext(ctx).Error("preprocessor exited with error", zap.Error(err))
		return err
	}

	log.WithContext(ctx).Info("preprocessor exited cleanly")
	return nil
}

func shutdownSafe(ctx context.Context, name string, fn func(context.Context) error, log *logger.Logger) {
	log.WithContext(ctx).Info(name + ": shutting down")
	if err := fn(ctx); err != nil {
		log.WithContext(ctx).Error(name+" shutdown failed", zap.Error(err))
	} else {
		log.WithContext(ctx).Info(name + ": shutdown complete")
	}
}
