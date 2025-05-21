// internal/usecase/refresh.go
package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/YaganovValera/analytics-system/common/backoff"
	"github.com/YaganovValera/analytics-system/common/logger"
	"go.uber.org/zap"

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
		return nil, fmt.Errorf("parse refresh: %w", err)
	}

	token, err := h.tokens.FindByJTI(ctx, claims.JTI)
	if err != nil {
		metrics.RefreshTotal.WithLabelValues("fail").Inc()
		h.log.WithContext(ctx).Error("refresh token lookup failed", zap.Error(err))
		return nil, fmt.Errorf("token lookup: %w", err)
	}

	if token.RevokedAt != nil {
		metrics.RefreshTotal.WithLabelValues("fail").Inc()
		h.log.WithContext(ctx).Warn("refresh token already revoked", zap.String("jti", claims.JTI))
		return nil, fmt.Errorf("refresh token was revoked")
	}

	access, accessClaims, err := h.signer.Generate(claims.UserID, claims.Roles, jwt.AccessToken)
	if err != nil {
		h.log.WithContext(ctx).Error("generate access failed", zap.Error(err))
		return nil, fmt.Errorf("generate access: %w", err)
	}
	refresh, refreshClaims, err := h.signer.Generate(claims.UserID, claims.Roles, jwt.RefreshToken)
	if err != nil {
		h.log.WithContext(ctx).Error("generate refresh failed", zap.Error(err))
		return nil, fmt.Errorf("generate refresh: %w", err)
	}

	err = backoff.Execute(ctx, backoff.Config{MaxElapsedTime: 2 * time.Second}, func(ctx context.Context) error {
		return h.tokens.Store(ctx, &postgres.RefreshToken{
			ID:        claims.JTI,
			UserID:    claims.UserID,
			JTI:       refreshClaims.JTI,
			Token:     refresh,
			IssuedAt:  refreshClaims.IssuedAt.Time,
			ExpiresAt: refreshClaims.ExpiresAt.Time,
		})
	}, nil)
	if err != nil {
		metrics.RefreshTotal.WithLabelValues("fail").Inc()
		h.log.WithContext(ctx).Error("store refresh failed", zap.Error(err))
		return nil, fmt.Errorf("store refresh: %w", err)
	}

	metrics.IssuedTokens.WithLabelValues("access").Inc()
	metrics.IssuedTokens.WithLabelValues("refresh").Inc()
	metrics.RefreshTotal.WithLabelValues("ok").Inc()

	return &authpb.RefreshTokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(time.Until(accessClaims.ExpiresAt.Time).Seconds()),
	}, nil
}
