// auth/internal/app/app.go
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

	authpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/auth"

	"github.com/YaganovValera/analytics-system/services/auth/internal/config"
	"github.com/YaganovValera/analytics-system/services/auth/internal/interceptor"
	"github.com/YaganovValera/analytics-system/services/auth/internal/jwt"
	"github.com/YaganovValera/analytics-system/services/auth/internal/metrics"
	postgres "github.com/YaganovValera/analytics-system/services/auth/internal/storage/postgres"
	grpcTransport "github.com/YaganovValera/analytics-system/services/auth/internal/transport/grpc"
	"github.com/YaganovValera/analytics-system/services/auth/internal/usecase"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
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

	// === PostgreSQL ===
	if err := postgres.ApplyMigrations(cfg.Postgres, log); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	db, err := postgres.Connect(cfg.Postgres, log)
	if err != nil {
		return fmt.Errorf("postgres connect: %w", err)
	}
	defer shutdownSafe(ctx, "postgres", func(ctx context.Context) error {
		db.Close()
		return nil
	}, log)

	// === JWT Signer/Verifier ===
	accessTTL, _ := time.ParseDuration(cfg.JWT.AccessTTL)
	refreshTTL, _ := time.ParseDuration(cfg.JWT.RefreshTTL)
	jwtSigner, err := jwt.NewHS256(cfg.JWT.Secret, cfg.JWT.Issuer, cfg.JWT.Audience, accessTTL, refreshTTL)
	if err != nil {
		return fmt.Errorf("jwt signer: %w", err)
	}

	// === Repositories ===
	userRepo := postgres.NewUserRepo(db)
	tokenRepo := postgres.NewTokenRepo(db)

	// === Usecases ===
	h := usecase.NewHandler(
		usecase.NewLoginHandler(userRepo, tokenRepo, jwtSigner, log),
		usecase.NewRefreshTokenHandler(tokenRepo, jwtSigner, jwtSigner, log),
		usecase.NewValidateTokenHandler(jwtSigner),
		usecase.NewRevokeTokenHandler(tokenRepo, log),
		usecase.NewLogoutHandler(tokenRepo),
		usecase.NewRegisterHandler(userRepo, tokenRepo, jwtSigner, log),
	)

	// === gRPC Server ===
	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.UnaryInterceptor(interceptor.UnaryJWTInterceptor(jwtSigner)),
	)

	authpb.RegisterAuthServiceServer(grpcServer, grpcTransport.NewServer(h, jwtSigner))

	grpcAddr := fmt.Sprintf(":%d", cfg.HTTP.Port+1)
	grpcLis, err := net.Listen("tcp", grpcAddr)

	if err != nil {
		return fmt.Errorf("grpc listen: %w", err)
	}

	// === HTTP Server ===
	readiness := func() error {
		ctxPing, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return db.Ping(ctxPing)
	}
	httpSrv, err := httpserver.New(cfg.HTTP, readiness, log,
		nil,
		httpserver.RecoverMiddleware,
		httpserver.CORSMiddleware(),
	)
	if err != nil {
		return fmt.Errorf("httpserver init: %w", err)
	}

	log.WithContext(ctx).Info("auth: starting services")
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error { return grpcServer.Serve(grpcLis) })
	g.Go(func() error { return httpSrv.Run(ctx) })

	if err := g.Wait(); err != nil {
		if ctx.Err() == context.Canceled {
			log.WithContext(ctx).Info("auth shut down cleanly")
			return nil
		}
		log.WithContext(ctx).Error("auth exited with error", zap.Error(err))
		return err
	}

	log.WithContext(ctx).Info("auth shut down complete")
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
