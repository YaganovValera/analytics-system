// auth/internal/usecase/login.go
package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/YaganovValera/analytics-system/common/backoff"
	"github.com/YaganovValera/analytics-system/common/logger"
	authpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/auth"
	"github.com/YaganovValera/analytics-system/services/auth/internal/jwt"
	"github.com/YaganovValera/analytics-system/services/auth/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/auth/internal/storage/postgres"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

var loginTracer = otel.Tracer("auth/usecase/login")

type loginHandler struct {
	users  postgres.UserRepository
	tokens postgres.TokenRepository
	signer jwt.Signer
	log    *logger.Logger
}

func NewLoginHandler(users postgres.UserRepository, tokens postgres.TokenRepository, signer jwt.Signer, log *logger.Logger) LoginHandler {
	return &loginHandler{users, tokens, signer, log.Named("login")}
}

func (h *loginHandler) Handle(ctx context.Context, req *authpb.LoginRequest) (*authpb.LoginResponse, error) {
	ctx, span := loginTracer.Start(ctx, "Login")
	defer span.End()

	if req == nil || req.Username == "" || req.Password == "" {
		metrics.LoginTotal.WithLabelValues("invalid").Inc()
		return nil, errors.New("missing credentials")
	}

	user, err := h.users.FindByUsername(ctx, req.Username)
	if err != nil {
		metrics.LoginTotal.WithLabelValues("fail").Inc()
		h.log.WithContext(ctx).Warn("user not found", zap.String("username", req.Username), zap.Error(err))
		return nil, fmt.Errorf("user not found")
	}

	timeout := 200 * time.Millisecond
	ctxHash, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	hashCh := make(chan error, 1)
	go func() {
		hashCh <- bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	}()

	select {
	case err := <-hashCh:
		if err != nil {
			metrics.LoginTotal.WithLabelValues("fail").Inc()
			h.log.WithContext(ctx).Warn("invalid password", zap.Error(err))
			return nil, fmt.Errorf("invalid credentials")
		}
	case <-ctxHash.Done():
		metrics.LoginTotal.WithLabelValues("fail").Inc()
		h.log.WithContext(ctx).Warn("password hash timeout")
		return nil, fmt.Errorf("password check timed out")
	}

	for _, role := range user.Roles {
		if !jwt.IsValidRole(role) {
			h.log.WithContext(ctx).Error("invalid user role",
				zap.String("username", user.Username),
				zap.String("role", role),
			)
			return nil, fmt.Errorf("user has invalid role: %s", role)
		}
	}

	access, accessClaims, err := h.signer.Generate(user.ID, user.Roles, jwt.AccessToken)
	if err != nil {
		h.log.WithContext(ctx).Error("generate access token failed", zap.Error(err))
		return nil, fmt.Errorf("generate access: %w", err)
	}
	refresh, refreshClaims, err := h.signer.Generate(user.ID, user.Roles, jwt.RefreshToken)
	if err != nil {
		h.log.WithContext(ctx).Error("generate refresh token failed", zap.Error(err))
		return nil, fmt.Errorf("generate refresh: %w", err)
	}

	err = backoff.Execute(ctx, backoff.Config{MaxElapsedTime: 2 * time.Second}, func(ctx context.Context) error {
		return h.tokens.Store(ctx, &postgres.RefreshToken{
			ID:        uuid.NewString(),
			UserID:    user.ID,
			JTI:       refreshClaims.JTI,
			Token:     refresh,
			IssuedAt:  refreshClaims.IssuedAt.Time,
			ExpiresAt: refreshClaims.ExpiresAt.Time,
		})
	}, nil)
	if err != nil {
		metrics.LoginTotal.WithLabelValues("fail").Inc()
		h.log.WithContext(ctx).Error("store refresh token failed", zap.Error(err))
		return nil, fmt.Errorf("store refresh: %w", err)
	}

	metrics.IssuedTokens.WithLabelValues("access").Inc()
	metrics.IssuedTokens.WithLabelValues("refresh").Inc()
	metrics.LoginTotal.WithLabelValues("ok").Inc()

	return &authpb.LoginResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(time.Until(accessClaims.ExpiresAt.Time).Seconds()),
	}, nil
}
