// services/preprocessor/internal/app/preprocessor.go
package app

import (
	"context"
	"fmt"

	"github.com/IBM/sarama"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/YaganovValera/analytics-system/common"
	httpserver "github.com/YaganovValera/analytics-system/common/httpserver"
	commonkafka "github.com/YaganovValera/analytics-system/common/kafka"
	consumer "github.com/YaganovValera/analytics-system/common/kafka/consumer"
	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/YaganovValera/analytics-system/common/telemetry"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/config"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/processor"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/storage"
)

// Run wires up and runs the preprocessor service.
func Run(ctx context.Context, cfg *config.Config, log *logger.Logger) error {
	// 0) Сквозной service-label
	common.InitServiceName(cfg.ServiceName)

	// 1) Prometheus-метрики
	metrics.Register(nil)

	// 2) OpenTelemetry
	shutdownTracer, err := telemetry.InitTracer(ctx, telemetry.Config{
		Endpoint:       cfg.Telemetry.OTLPEndpoint,
		ServiceName:    cfg.ServiceName,
		ServiceVersion: cfg.ServiceVersion,
		Insecure:       cfg.Telemetry.Insecure,
	}, log)
	if err != nil {
		return fmt.Errorf("telemetry init: %w", err)
	}
	defer shutdownTracer(context.Background())

	// 3) Postgres (TimescaleDB)
	pgStorage, err := storage.NewPostgres(ctx, cfg.Postgres, log)
	if err != nil {
		return fmt.Errorf("postgres init: %w", err)
	}
	defer pgStorage.Close()

	// 4) Kafka consumer
	kafkaConsumer, err := consumer.New(ctx, consumer.Config{
		Brokers: cfg.Kafka.Brokers,
		GroupID: cfg.ServiceName,
		Version: sarama.MaxVersion.String(),
		Backoff: cfg.Kafka.Backoff,
	}, log)
	if err != nil {
		return fmt.Errorf("kafka consumer init: %w", err)
	}

	// 5) Processor
	proc := processor.NewProcessor(pgStorage, log)

	// 6) HTTP/metrics server
	readiness := func() error { return pgStorage.Ping(ctx) }
	httpSrv, err := httpserver.New(
		httpserver.Config{
			Addr:            fmt.Sprintf(":%d", cfg.HTTP.Port),
			ReadTimeout:     cfg.HTTP.ReadTimeout,
			WriteTimeout:    cfg.HTTP.WriteTimeout,
			IdleTimeout:     cfg.HTTP.IdleTimeout,
			ShutdownTimeout: cfg.HTTP.ShutdownTimeout,
			MetricsPath:     cfg.HTTP.MetricsPath,
			HealthzPath:     cfg.HTTP.HealthzPath,
			ReadyzPath:      cfg.HTTP.ReadyzPath,
		},
		readiness,
		log,
	)
	if err != nil {
		return fmt.Errorf("http server init: %w", err)
	}

	log.Info("preprocessor: components initialized, entering run-loop")

	// 7) Concurrent loops
	g, ctx := errgroup.WithContext(ctx)

	// 7.1 HTTP
	g.Go(func() error {
		return httpSrv.Start(ctx)
	})

	// 7.2 Kafka → Processor
	g.Go(func() error {
		return kafkaConsumer.Consume(ctx, []string{cfg.Kafka.RawTopic}, func(msg *commonkafka.Message) error {
			return proc.Process(ctx, msg)
		})
	})

	// 8) Wait & shutdown
	if err := g.Wait(); err != nil && ctx.Err() == nil {
		log.WithContext(ctx).Error("runtime error", zap.Error(err))
	}
	if err := kafkaConsumer.Close(); err != nil {
		log.Error("kafka consumer close", zap.Error(err))
	}

	log.Info("preprocessor shutdown complete")
	return ctx.Err()
}
