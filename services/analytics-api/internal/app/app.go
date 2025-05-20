// github.com/YaganovValera/analytics-system/services/analytics-api/internal/app/app.go
package app

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/YaganovValera/analytics-system/common/httpserver"
	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/YaganovValera/analytics-system/common/serviceid"
	"github.com/YaganovValera/analytics-system/common/telemetry"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/config"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/storage/kafka"
	timescaledb "github.com/YaganovValera/analytics-system/services/analytics-api/internal/storage/timescaledb"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/transport/grpc"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/usecase"

	analyticspb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/analytics"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	grpcstd "google.golang.org/grpc"
)

func Run(ctx context.Context, cfg *config.Config, log *logger.Logger) error {
	serviceid.InitServiceName(cfg.ServiceName)
	metrics.Register(nil)

	// === Telemetry ===
	cfg.Telemetry.ServiceName = cfg.ServiceName
	cfg.Telemetry.ServiceVersion = cfg.ServiceVersion
	shutdownTracer, err := telemetry.InitTracer(ctx, cfg.Telemetry, log)
	if err != nil {
		return fmt.Errorf("init tracer: %w", err)
	}
	defer shutdownSafe(ctx, "telemetry", shutdownTracer, log)

	// === TimescaleDB ===
	db, err := timescaledb.New(cfg.Timescale, log)
	if err != nil {
		return fmt.Errorf("timescaledb: %w", err)
	}
	defer db.Close()

	// === Kafka Consumer ===
	kafkaRepo, err := kafka.New(ctx, cfg.Kafka, log)
	if err != nil {
		return fmt.Errorf("kafka: %w", err)
	}
	defer kafkaRepo.Close()

	// === Usecases ===
	getHandler := usecase.NewGetCandlesHandler(db)
	streamHandler := usecase.NewStreamCandlesHandler(kafkaRepo, cfg.TopicBase)
	subscribeHandler := usecase.NewSubscribeCandlesHandler(kafkaRepo, cfg.TopicBase, log)

	// === gRPC Server ===
	grpcServer := grpcstd.NewServer(
		grpcstd.StatsHandler(otelgrpc.NewServerHandler()),
	)
	analyticspb.RegisterAnalyticsServiceServer(grpcServer,
		grpc.NewServer(getHandler, streamHandler, subscribeHandler))

	grpcLis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.HTTP.Port+1))
	if err != nil {
		return fmt.Errorf("listen grpc: %w", err)
	}

	// === HTTP Server ===
	readiness := func() error {
		ctxPing, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return db.Ping(ctxPing)
	}

	httpSrv, err := httpserver.New(
		cfg.HTTP,
		readiness,
		log,
		nil,
		httpserver.RecoverMiddleware,
		httpserver.CORSMiddleware())
	if err != nil {
		return fmt.Errorf("httpserver init: %w", err)
	}

	log.WithContext(ctx).Info("analytics-api: starting services")
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error { return httpSrv.Run(ctx) })
	g.Go(func() error { return grpcServer.Serve(grpcLis) })

	if err := g.Wait(); err != nil {
		if ctx.Err() == context.Canceled {
			log.WithContext(ctx).Info("analytics-api shut down cleanly")
			return nil
		}
		log.WithContext(ctx).Error("analytics-api exited with error", zap.Error(err))
		return err
	}

	log.WithContext(ctx).Info("analytics-api shut down complete")
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
