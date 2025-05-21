// internal/usecase/logout.go
package usecase

import (
	"context"

	"github.com/YaganovValera/analytics-system/services/auth/internal/storage/postgres"
	"go.opentelemetry.io/otel"
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
	return h.tokens.RevokeByJTI(ctx, jti)
}
