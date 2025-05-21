// auth/internal/usecase/validate.go
package usecase

import (
	"context"
	"fmt"

	authpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/auth"

	"github.com/YaganovValera/analytics-system/services/auth/internal/jwt"
	"github.com/YaganovValera/analytics-system/services/auth/internal/metrics"

	"go.opentelemetry.io/otel"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var validateTracer = otel.Tracer("auth/usecase/validate")

type validateHandler struct {
	verifier jwt.Verifier
}

func NewValidateTokenHandler(verifier jwt.Verifier) ValidateTokenHandler {
	return &validateHandler{verifier}
}

func (h *validateHandler) Handle(ctx context.Context, token string) (*authpb.ValidateTokenResponse, error) {
	_, span := validateTracer.Start(ctx, "Validate")
	defer span.End()

	claims, err := h.verifier.Parse(token)
	if err != nil {
		metrics.ValidateTotal.WithLabelValues("invalid").Inc()
		return nil, fmt.Errorf("token invalid: %w", err)
	}
	metrics.ValidateTotal.WithLabelValues("ok").Inc()

	return &authpb.ValidateTokenResponse{
		Valid:     true,
		Username:  claims.Subject,
		Roles:     claims.Roles,
		ExpiresAt: timestamppb.New(claims.ExpiresAt.Time),
	}, nil
}
