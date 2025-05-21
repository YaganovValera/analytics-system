// internal/usecase/revoke.go
package usecase

import (
	"context"
	"fmt"

	"github.com/YaganovValera/analytics-system/common/logger"
	authpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/auth"
	"github.com/YaganovValera/analytics-system/services/auth/internal/jwt"
	"github.com/YaganovValera/analytics-system/services/auth/internal/storage/postgres"

	"go.opentelemetry.io/otel"
)

var revokeTracer = otel.Tracer("auth/usecase/revoke")

type revokeHandler struct {
	tokens postgres.TokenRepository
	log    *logger.Logger
}

func NewRevokeTokenHandler(tokens postgres.TokenRepository, log *logger.Logger) RevokeTokenHandler {
	return &revokeHandler{tokens, log.Named("revoke")}
}

func (h *revokeHandler) Handle(ctx context.Context, req *authpb.RevokeTokenRequest) (*authpb.RevokeTokenResponse, error) {
	ctx, span := revokeTracer.Start(ctx, "Revoke")
	defer span.End()

	if req.Token == "" || req.Type != authpb.TokenType_REFRESH {
		return nil, fmt.Errorf("only refresh token revocation supported")
	}

	// Парсим jti
	claims, err := jwt.ParseUnverifiedJTI(req.Token)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	if err := h.tokens.RevokeByJTI(ctx, claims.JTI); err != nil {
		return nil, fmt.Errorf("revoke failed: %w", err)
	}
	return &authpb.RevokeTokenResponse{Revoked: true}, nil
}
