// internal/usecase/login.go
package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	authpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/auth"
	"github.com/YaganovValera/analytics-system/services/auth/internal/jwt"
	"github.com/YaganovValera/analytics-system/services/auth/internal/storage/postgres"
	"go.uber.org/zap"

	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
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
		return nil, errors.New("missing credentials")
	}

	user, err := h.users.FindByUsername(ctx, req.Username)
	if err != nil {
		h.log.WithContext(ctx).Warn("login failed", zap.Error(err))
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
			h.log.WithContext(ctx).Warn("invalid password", zap.Error(err))
			return nil, fmt.Errorf("invalid credentials")
		}
	case <-ctxHash.Done():
		h.log.WithContext(ctx).Warn("password hash timeout")
		return nil, fmt.Errorf("password check timed out")
	}

	access, accessClaims, err := h.signer.Generate(user.ID, user.Roles, jwt.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("generate access: %w", err)
	}
	refresh, refreshClaims, err := h.signer.Generate(user.ID, user.Roles, jwt.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("generate refresh: %w", err)
	}

	err = h.tokens.Store(ctx, &postgres.RefreshToken{
		ID:        uuid.NewString(),
		UserID:    user.ID,
		JTI:       refreshClaims.JTI,
		Token:     refresh,
		IssuedAt:  refreshClaims.IssuedAt.Time,
		ExpiresAt: refreshClaims.ExpiresAt.Time,
	})
	if err != nil {
		return nil, fmt.Errorf("store refresh: %w", err)
	}

	return &authpb.LoginResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(time.Until(accessClaims.ExpiresAt.Time).Seconds()),
	}, nil
}
