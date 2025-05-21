// internal/app/app.go
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/YaganovValera/analytics-system/common/httpserver"
	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/YaganovValera/analytics-system/common/serviceid"
	"github.com/YaganovValera/analytics-system/common/telemetry"

	"github.com/YaganovValera/analytics-system/services/auth/internal/config"
	"github.com/YaganovValera/analytics-system/services/auth/internal/jwt"
	"github.com/YaganovValera/analytics-system/services/auth/internal/storage/postgres"
	"github.com/YaganovValera/analytics-system/services/auth/internal/transport/grpc"
	"github.com/YaganovValera/analytics-system/services/auth/internal/usecase"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

func Run(ctx context.Context, cfg *config.Config, log *logger.Logger) error {
	serviceid.InitServiceName(cfg.ServiceName)

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
	defer db.Close()

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
	login := usecase.NewLoginHandler(userRepo, tokenRepo, jwtSigner, log)
	refresh := usecase.NewRefreshTokenHandler(tokenRepo, jwtSigner, jwtSigner, log)
	validate := usecase.NewValidateTokenHandler(jwtSigner)
	revoke := usecase.NewRevokeTokenHandler(tokenRepo, log)
	logout := usecase.NewLogoutHandler(tokenRepo)

	// === gRPC Server ===
	grpcServer := grpc.NewServer(login, validate, refresh, revoke, logout, log)
	grpcSrv := grpc.NewGRPCServer(cfg.HTTP.Port+1, grpcServer, log)

	// === HTTP server (healthz, readyz, metrics) ===
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

	// === Run both
	log.WithContext(ctx).Info("auth: starting services")
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return httpSrv.Run(ctx) })
	g.Go(func() error { return grpcSrv.Serve(ctx) })

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
