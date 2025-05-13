// services/analytics-api/internal/app/app.go
package app

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	"github.com/YaganovValera/analytics-system/common"
	httpserver "github.com/YaganovValera/analytics-system/common/httpserver"
	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/YaganovValera/analytics-system/common/telemetry"

	analyticspb "github.com/YaganovValera/analytics-system/proto/v1/analytics"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/config"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/storage"
)

// Run wires up and runs the Analytics API service.
func Run(ctx context.Context, cfg *config.Config, log *logger.Logger) error {
	// 0) Service-level label
	common.InitServiceName(cfg.ServiceName)

	// 1) Prometheus metrics
	metrics.Register(nil)

	// 2) OpenTelemetry
	shutdownTracer, err := telemetry.InitTracer(ctx, telemetry.Config{
		Endpoint:       cfg.Telemetry.OTLPEndpoint,
		ServiceName:    cfg.ServiceName,
		ServiceVersion: cfg.ServiceVersion,
		Insecure:       cfg.Telemetry.Insecure,
		SamplerRatio:   cfg.Telemetry.Sampler,
	}, log)
	if err != nil {
		return fmt.Errorf("telemetry init: %w", err)
	}
	defer func() {
		log.Info("analytics-api: shutting down telemetry")
		_ = shutdownTracer(context.Background())
	}()

	// 3) Storage (Postgres/TimescaleDB)
	repo, err := storage.NewPostgresRepo(ctx, cfg.Postgres, log)
	if err != nil {
		return fmt.Errorf("storage init: %w", err)
	}

	// 4) gRPC handler
	handler := NewHandler(repo, log)

	// 5) gRPC server
	grpcServer := grpc.NewServer()
	analyticspb.RegisterAnalyticsServiceServer(grpcServer, handler)

	// 6) HTTP/metrics server (health, ready, metrics)
	// Readiness can be a simple ping to Postgres
	readiness := func() error {
		// Ping with timeout to avoid hanging
		ctxPing, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return repo.Ping(ctxPing)
	}

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

	log.Info("analytics-api: components initialized, entering run-loop")

	// 7) Run gRPC and HTTP concurrently
	g, ctx := errgroup.WithContext(ctx)

	// gRPC listener
	g.Go(func() error {
		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.HTTP.Port))
		if err != nil {
			return fmt.Errorf("listen gRPC: %w", err)
		}
		log.Info("analytics-api: gRPC server listening", zap.Int("port", cfg.HTTP.Port))
		return grpcServer.Serve(lis)
	})

	// HTTP server for metrics, health, ready
	g.Go(func() error {
		return httpSrv.Start(ctx)
	})

	// 8) Wait & shutdown
	if err := g.Wait(); err != nil {
		log.WithContext(ctx).Error("analytics-api: exiting with error", zap.Error(err))
	} else {
		log.WithContext(ctx).Info("analytics-api: exiting normally")
	}

	return ctx.Err()
}
