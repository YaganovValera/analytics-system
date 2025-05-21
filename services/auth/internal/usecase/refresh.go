// internal/usecase/refresh.go
package usecase

import (
	"context"
	"fmt"

	authpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/auth"
	"github.com/YaganovValera/analytics-system/services/auth/internal/jwt"
	"github.com/YaganovValera/analytics-system/services/auth/internal/storage/postgres"

	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

var refreshTracer = otel.Tracer("auth/usecase/refresh")

type refreshHandler struct {
	tokens   postgres.TokenRepository
	signer   jwt.Signer
	verifier jwt.Verifier
	log      *logger.Logger
}

func NewRefreshTokenHandler(tokens postgres.TokenRepository, signer jwt.Signer, verifier jwt.Verifier, log *logger.Logger) RefreshTokenHandler {
	return &refreshHandler{tokens, signer, verifier, log.Named("refresh")}
}

func (h *refreshHandler) Handle(ctx context.Context, req *authpb.RefreshTokenRequest) (*authpb.RefreshTokenResponse, error) {
	ctx, span := refreshTracer.Start(ctx, "Refresh")
	defer span.End()

	claims, err := h.verifier.Parse(req.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh: %w", err)
	}

	// Проверим в хранилище
	rt, err := h.tokens.FindByJTI(ctx, claims.JTI)
	if err != nil || rt.RevokedAt != nil {
		return nil, fmt.Errorf("refresh token revoked or missing")
	}

	access, _, err := h.signer.Generate(claims.UserID, claims.Roles, jwt.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("generate access: %w", err)
	}
	newRefresh, newClaims, err := h.signer.Generate(claims.UserID, claims.Roles, jwt.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("generate refresh: %w", err)
	}

	err = h.tokens.Store(ctx, &postgres.RefreshToken{
		ID:        uuid.NewString(),
		UserID:    claims.UserID,
		JTI:       newClaims.JTI,
		Token:     newRefresh,
		IssuedAt:  newClaims.IssuedAt.Time,
		ExpiresAt: newClaims.ExpiresAt.Time,
	})
	if err != nil {
		return nil, fmt.Errorf("store new refresh: %w", err)
	}

	return &authpb.RefreshTokenResponse{
		AccessToken:  access,
		RefreshToken: newRefresh,
	}, nil
}
