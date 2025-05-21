// internal/usecase/refresh.go
package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/YaganovValera/analytics-system/common/logger"
	authpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/auth"
	"github.com/YaganovValera/analytics-system/services/auth/internal/jwt"
	"github.com/YaganovValera/analytics-system/services/auth/internal/storage/postgres"

	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

var refreshTracer = otel.Tracer("auth/usecase/refresh")

type refreshHandler struct {
	tokens   postgres.TokenRepository
	verifier jwt.Verifier
	signer   jwt.Signer
	log      *logger.Logger
}

func NewRefreshTokenHandler(tokens postgres.TokenRepository, verifier jwt.Verifier, signer jwt.Signer, log *logger.Logger) RefreshTokenHandler {
	return &refreshHandler{
		tokens:   tokens,
		verifier: verifier,
		signer:   signer,
		log:      log.Named("refresh"),
	}
}

func (h *refreshHandler) Handle(ctx context.Context, req *authpb.RefreshTokenRequest) (*authpb.RefreshTokenResponse, error) {
	ctx, span := refreshTracer.Start(ctx, "Refresh")
	defer span.End()

	claims, err := h.verifier.Parse(req.RefreshToken)
	if err != nil {
		h.log.WithContext(ctx).Warn("token parse failed", zap.Error(err))
		return nil, fmt.Errorf("invalid refresh token")
	}

	token, err := h.tokens.FindByJTI(ctx, claims.JTI)
	if err != nil {
		h.log.WithContext(ctx).Warn("token lookup failed", zap.Error(err))
		return nil, fmt.Errorf("token not found")
	}

	if token.RevokedAt != nil {
		h.log.WithContext(ctx).Warn("token already revoked", zap.String("jti", claims.JTI))
		return nil, fmt.Errorf("refresh token already revoked")
	}

	// revoke old token immediately to prevent reuse
	if err := h.tokens.RevokeByJTI(ctx, token.JTI); err != nil {
		h.log.WithContext(ctx).Error("revoke failed", zap.Error(err))
		return nil, fmt.Errorf("token revoke error")
	}

	access, accessClaims, err := h.signer.Generate(claims.UserID, claims.Roles, jwt.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("generate access: %w", err)
	}
	refresh, refreshClaims, err := h.signer.Generate(claims.UserID, claims.Roles, jwt.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("generate refresh: %w", err)
	}

	err = h.tokens.Store(ctx, &postgres.RefreshToken{
		ID:        claims.JTI,
		UserID:    claims.UserID,
		JTI:       refreshClaims.JTI,
		Token:     refresh,
		IssuedAt:  refreshClaims.IssuedAt.Time,
		ExpiresAt: refreshClaims.ExpiresAt.Time,
	})
	if err != nil {
		return nil, fmt.Errorf("store refresh: %w", err)
	}

	return &authpb.RefreshTokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(time.Until(accessClaims.ExpiresAt.Time).Seconds()),
	}, nil
}
