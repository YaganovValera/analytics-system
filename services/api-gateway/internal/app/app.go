// api-gateway/internal/app/app.go
package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/YaganovValera/analytics-system/common/httpserver"
	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/YaganovValera/analytics-system/common/serviceid"
	"github.com/YaganovValera/analytics-system/common/telemetry"

	analyticsclient "github.com/YaganovValera/analytics-system/services/api-gateway/internal/client/analytics"
	authclient "github.com/YaganovValera/analytics-system/services/api-gateway/internal/client/auth"
	"github.com/YaganovValera/analytics-system/services/api-gateway/internal/config"
	"github.com/YaganovValera/analytics-system/services/api-gateway/internal/handler"
	"github.com/YaganovValera/analytics-system/services/api-gateway/internal/middleware"
	transport "github.com/YaganovValera/analytics-system/services/api-gateway/internal/transport/http"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func Run(ctx context.Context, cfg *config.Config, log *logger.Logger) error {
	serviceid.InitServiceName(cfg.ServiceName)

	// === OpenTelemetry ===
	cfg.Telemetry.ServiceName = cfg.ServiceName
	cfg.Telemetry.ServiceVersion = cfg.ServiceVersion
	shutdownTracer, err := telemetry.InitTracer(ctx, cfg.Telemetry, log)
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}
	defer shutdownSafe(ctx, "telemetry", func() error { return shutdownTracer(ctx) }, log)

	// === gRPC clients ===
	authConn, err := grpc.NewClient("auth:8085", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("auth grpc dial: %w", err)
	}
	defer shutdownSafe(ctx, "auth-grpc", authConn.Close, log)
	authClient := authclient.New(authConn)

	analyticsConn, err := grpc.NewClient("analytics-api:8083", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("analytics grpc dial: %w", err)
	}
	defer shutdownSafe(ctx, "analytics-grpc", analyticsConn.Close, log)
	analyticsClient := analyticsclient.New(analyticsConn)

	// === HTTP handlers and middleware ===
	h := handler.NewHandler(authClient, analyticsClient)
	m := transport.NewMiddleware(authClient)

	extraRoutes := map[string]http.Handler{
		"/": transport.Routes(h, m),
	}

	readiness := func() error {
		ctxPing, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return authClient.Ping(ctxPing)
	}

	// 1. JWT кэш
	jwtCache := middleware.NewJWTCache()

	// 2. RBAC config
	rbacCfg := middleware.RBACConfig{
		Log: log,
		Permissions: map[middleware.Route][]string{
			{Method: "POST", Path: "/admin/*"}: {"admin"},
			{Method: "GET", Path: "/candles"}:  {"user", "admin", "viewer"},
		},
	}

	// 3. Rate limiter
	rateLimit := middleware.RateLimitMiddleware(middleware.RateLimitConfig{
		RequestsPerSec: 5,
		BurstSize:      10,
		Log:            log,
	})

	httpSrv, err := httpserver.New(
		cfg.HTTP,
		readiness,
		log,
		extraRoutes,

		httpserver.RecoverMiddleware,
		middleware.RequestLoggerMiddleware(log),
		middleware.JWTMiddleware(authClient, jwtCache, log),
		middleware.RBACMiddleware(rbacCfg),
		rateLimit,
		httpserver.CORSMiddleware(),
	)

	if err != nil {
		return fmt.Errorf("httpserver init: %w", err)
	}

	log.WithContext(ctx).Info("api-gateway: starting services")
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return httpSrv.Run(ctx) })

	if err := g.Wait(); err != nil {
		if ctx.Err() == context.Canceled {
			log.WithContext(ctx).Info("api-gateway shut down cleanly")
			return nil
		}
		log.WithContext(ctx).Error("api-gateway exited with error", zap.Error(err))
		return err
	}

	log.WithContext(ctx).Info("api-gateway shut down complete")
	return nil
}

func shutdownSafe(ctx context.Context, name string, fn func() error, log *logger.Logger) {
	log.WithContext(ctx).Info(name + ": shutting down")
	if err := fn(); err != nil {
		log.WithContext(ctx).Error(name+" shutdown failed", zap.Error(err))
	} else {
		log.WithContext(ctx).Info(name + ": shutdown complete")
	}
}
