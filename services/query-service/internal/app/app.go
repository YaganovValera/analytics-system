// query-service/internal/app/app.go

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
	querypb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/query"
	"github.com/YaganovValera/analytics-system/services/query-service/internal/config"
	"github.com/YaganovValera/analytics-system/services/query-service/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/query-service/internal/storage/timescaledb"
	"github.com/YaganovValera/analytics-system/services/query-service/internal/transport/grpc"
	"github.com/YaganovValera/analytics-system/services/query-service/internal/usecase"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	stdgrpc "google.golang.org/grpc"
)

func Run(ctx context.Context, cfg *config.Config, log *logger.Logger) error {
	serviceid.InitServiceName(cfg.ServiceName)
	metrics.Register(nil)

	// === Tracing ===
	cfg.Telemetry.ServiceName = cfg.ServiceName
	cfg.Telemetry.ServiceVersion = cfg.ServiceVersion
	shutdownTracer, err := telemetry.InitTracer(ctx, cfg.Telemetry, log)
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}
	defer shutdownSafe(ctx, "telemetry", shutdownTracer, log)

	// === TimescaleDB ===
	repo, err := timescaledb.New(cfg.Timescale, log)
	if err != nil {
		return fmt.Errorf("timescaledb init: %w", err)
	}
	defer shutdownSafe(ctx, "timescaledb", func(ctx context.Context) error {
		repo.Close()
		return nil
	}, log)

	// === Usecase ===
	exec := usecase.NewExecutor(repo, log)

	// === gRPC Server ===
	grpcSrv := stdgrpc.NewServer(
		stdgrpc.StatsHandler(otelgrpc.NewServerHandler()),
	)
	querypb.RegisterQueryServiceServer(grpcSrv, grpc.NewServer(exec))

	grpcAddr := fmt.Sprintf(":%d", cfg.HTTP.Port+1)
	grpcLis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return fmt.Errorf("grpc listen failed: %w", err)
	}

	// === HTTP Server (healthz, metrics) ===
	readiness := func() error {
		ctxPing, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return repo.Ping(ctxPing)
	}
	httpSrv, err := httpserver.New(cfg.HTTP, readiness, log,
		nil,
		httpserver.RecoverMiddleware,
		httpserver.CORSMiddleware(),
	)
	if err != nil {
		return fmt.Errorf("http server init: %w", err)
	}

	// === Run both ===
	log.Info("starting query-service", zap.String("grpc", grpcAddr), zap.Int("http_port", cfg.HTTP.Port))

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return grpcSrv.Serve(grpcLis) })
	g.Go(func() error { return httpSrv.Run(ctx) })

	if err := g.Wait(); err != nil {
		if ctx.Err() == context.Canceled {
			log.Info("query-service shut down cleanly")
			return nil
		}
		log.Error("query-service exited with error", zap.Error(err))
		return err
	}

	log.Info("shutdown complete")
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
