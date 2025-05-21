// auth/internal/usecase/refresh.go
package usecase

import (
	"context"
	"time"

	"github.com/YaganovValera/analytics-system/common/backoff"
	"github.com/YaganovValera/analytics-system/common/logger"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/auth"

	"github.com/YaganovValera/analytics-system/services/auth/internal/jwt"
	"github.com/YaganovValera/analytics-system/services/auth/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/auth/internal/storage/postgres"

	"go.opentelemetry.io/otel"
)

var refreshTracer = otel.Tracer("auth/usecase/refresh")

type refreshHandler struct {
	tokens   postgres.TokenRepository
	verifier jwt.Verifier
	signer   jwt.Signer
	log      *logger.Logger
}

func NewRefreshTokenHandler(tokens postgres.TokenRepository, verifier jwt.Verifier, signer jwt.Signer, log *logger.Logger) RefreshTokenHandler {
	return &refreshHandler{tokens, verifier, signer, log.Named("refresh")}
}

func (h *refreshHandler) Handle(ctx context.Context, req *authpb.RefreshTokenRequest) (*authpb.RefreshTokenResponse, error) {
	ctx, span := refreshTracer.Start(ctx, "Refresh")
	defer span.End()

	claims, err := h.verifier.Parse(req.RefreshToken)
	if err != nil {
		metrics.RefreshTotal.WithLabelValues("invalid").Inc()
		h.log.WithContext(ctx).Warn("invalid refresh token", zap.Error(err))
		return nil, status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
	}

	token, err := h.tokens.FindByJTI(ctx, claims.JTI)
	if err != nil {
		metrics.RefreshTotal.WithLabelValues("lookup_error").Inc()
		h.log.WithContext(ctx).Error("refresh token lookup failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to lookup token: %v", err)
	}

	if token.RevokedAt != nil {
		metrics.RefreshTotal.WithLabelValues("reuse_detected").Inc()
		h.log.WithContext(ctx).Warn("refresh token reuse attempt", zap.String("jti", claims.JTI))
		return nil, status.Error(codes.Unauthenticated, "refresh token already used or revoked")
	}

	// Отзываем старый токен
	if err := h.tokens.RevokeByJTI(ctx, claims.JTI); err != nil {
		h.log.WithContext(ctx).Warn("failed to revoke old refresh token",
			zap.String("jti", claims.JTI), zap.Error(err))
	}

	// Генерируем access + refresh
	access, accessClaims, err := h.signer.Generate(claims.UserID, claims.Roles, jwt.AccessToken)
	if err != nil {
		h.log.WithContext(ctx).Error("generate access failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "generate access: %v", err)
	}

	refresh, refreshClaims, err := h.signer.Generate(claims.UserID, claims.Roles, jwt.RefreshToken)
	if err != nil {
		h.log.WithContext(ctx).Error("generate refresh failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "generate refresh: %v", err)
	}

	// Сохраняем новый refresh
	err = backoff.Execute(ctx, backoff.Config{MaxElapsedTime: 2 * time.Second}, func(ctx context.Context) error {
		return h.tokens.Store(ctx, &postgres.RefreshToken{
			ID:        refreshClaims.JTI,
			UserID:    claims.UserID,
			JTI:       refreshClaims.JTI,
			Token:     refresh,
			IssuedAt:  refreshClaims.IssuedAt.Time,
			ExpiresAt: refreshClaims.ExpiresAt.Time,
		})
	}, nil)

	if err != nil {
		metrics.RefreshTotal.WithLabelValues("store_failed").Inc()
		h.log.WithContext(ctx).Error("store refresh failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "store refresh: %v", err)
	}

	metrics.RefreshTotal.WithLabelValues("success").Inc()
	metrics.IssuedTokens.WithLabelValues("access").Inc()
	metrics.IssuedTokens.WithLabelValues("refresh").Inc()

	return &authpb.RefreshTokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(time.Until(accessClaims.ExpiresAt.Time).Seconds()),
	}, nil
}
