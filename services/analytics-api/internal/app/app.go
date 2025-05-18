// github.com/YaganovValera/analytics-system/services/analytics-api/internal/app/app.go
package app

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/YaganovValera/analytics-system/common"
	httpserver "github.com/YaganovValera/analytics-system/common/httpserver"
	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/YaganovValera/analytics-system/common/telemetry"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/config"
	grpcapi "github.com/YaganovValera/analytics-system/services/analytics-api/internal/grpc"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/repository/kafka"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/repository/timescaledb"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/usecase"

	analyticspb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/analytics"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	grpcstd "google.golang.org/grpc"
)

func Run(ctx context.Context, cfg *config.Config, log *logger.Logger) error {
	common.InitServiceName(cfg.ServiceName)
	metrics.Register(nil)

	// Telemetry
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

	// TimescaleDB
	db, err := timescaledb.New(cfg.Timescale, log)
	if err != nil {
		return fmt.Errorf("timescaledb: %w", err)
	}
	defer db.Close()

	// Kafka
	kafkaRepo, err := kafka.New(ctx, cfg.Kafka, log)
	if err != nil {
		return fmt.Errorf("kafka: %w", err)
	}
	defer kafkaRepo.Close()

	// gRPC server
	grpcServer := grpcstd.NewServer(
		grpcstd.StatsHandler(otelgrpc.NewServerHandler()),
	)

	analyticsHandler := grpcapi.NewServer(
		usecase.NewGetCandlesHandler(db),
		usecase.NewStreamCandlesHandler(kafkaRepo),
	)
	analyticspb.RegisterAnalyticsServiceServer(grpcServer, analyticsHandler)

	grpcLis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.HTTP.Port+1))
	if err != nil {
		return fmt.Errorf("listen grpc: %w", err)
	}

	// Readiness проверяет и БД, и Kafka
	readiness := func() error {
		ctxPing, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		if err := db.Ping(ctxPing); err != nil {
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

	log.WithContext(ctx).Info("analytics-api: components initialized, starting run loops")
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error { return httpSrv.Run(ctx) })
	g.Go(func() error { return grpcServer.Serve(grpcLis) })

	if err := g.Wait(); err != nil {
		if ctx.Err() == context.Canceled {
			log.WithContext(ctx).Info("analytics-api exited on context cancel")
			return nil
		}
		log.WithContext(ctx).Error("analytics-api exited with error", zap.Error(err))
		return err
	}

	log.WithContext(ctx).Info("analytics-api exited cleanly")
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
