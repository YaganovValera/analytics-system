// auth/internal/usecase/logout.go
package usecase

import (
	"context"

	"github.com/YaganovValera/analytics-system/services/auth/internal/storage/postgres"
	"go.opentelemetry.io/otel"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var logoutTracer = otel.Tracer("auth/usecase/logout")

type logoutHandler struct {
	tokens postgres.TokenRepository
}

func NewLogoutHandler(tokens postgres.TokenRepository) LogoutHandler {
	return &logoutHandler{tokens}
}

func (h *logoutHandler) Handle(ctx context.Context, jti string) error {
	ctx, span := logoutTracer.Start(ctx, "Logout")
	defer span.End()

	token, err := h.tokens.FindByJTI(ctx, jti)
	if err != nil {
		return status.Errorf(codes.NotFound, "refresh token not found: %v", err)
	}
	if token.RevokedAt != nil {
		return status.Error(codes.AlreadyExists, "token already revoked")
	}
	return h.tokens.RevokeByJTI(ctx, jti)
}
