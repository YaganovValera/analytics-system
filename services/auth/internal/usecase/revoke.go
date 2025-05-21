// auth/internal/usecase/revoke.go
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

	if req == nil || req.Token == "" || req.Type != authpb.TokenType_REFRESH {
		metrics.RevokeTotal.WithLabelValues("invalid").Inc()
		return nil, fmt.Errorf("only refresh token revocation supported")
	}

	claims, err := jwt.ParseUnverifiedJTI(req.Token)
	if err != nil {
		metrics.RevokeTotal.WithLabelValues("invalid").Inc()
		h.log.WithContext(ctx).Warn("parse jti failed", zap.Error(err))
		return nil, fmt.Errorf("parse: %w", err)
	}

	err = backoff.Execute(ctx, backoff.Config{MaxElapsedTime: 2 * time.Second}, func(ctx context.Context) error {
		return h.tokens.RevokeByJTI(ctx, claims.JTI)
	}, nil)
	if err != nil {
		metrics.RevokeTotal.WithLabelValues("fail").Inc()
		h.log.WithContext(ctx).Error("revoke failed", zap.Error(err))
		return nil, fmt.Errorf("revoke failed: %w", err)
	}

	metrics.RevokeTotal.WithLabelValues("ok").Inc()
	return &authpb.RevokeTokenResponse{Revoked: true}, nil
}
