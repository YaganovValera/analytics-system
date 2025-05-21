// auth/internal/usecase/register.go
package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/YaganovValera/analytics-system/common/backoff"
	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/YaganovValera/analytics-system/services/auth/internal/jwt"
	"github.com/YaganovValera/analytics-system/services/auth/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/auth/internal/storage/postgres"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/auth"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type registerHandler struct {
	users  postgres.UserRepository
	tokens postgres.TokenRepository
	signer jwt.Signer
	log    *logger.Logger
}

func NewRegisterHandler(users postgres.UserRepository, tokens postgres.TokenRepository, signer jwt.Signer, log *logger.Logger) RegisterHandler {
	return &registerHandler{users, tokens, signer, log.Named("register")}
}

func (h *registerHandler) Handle(ctx context.Context, req *authpb.RegisterRequest) (*authpb.RegisterResponse, error) {
	ctx, span := otel.Tracer("auth/usecase/register").Start(ctx, "Register")
	defer span.End()

	if req == nil || strings.TrimSpace(req.Username) == "" || strings.TrimSpace(req.Password) == "" || len(req.Roles) == 0 {
		metrics.LoginTotal.WithLabelValues("invalid").Inc()
		return nil, status.Error(codes.InvalidArgument, "missing required fields")

	}

	const (
		minUsernameLength = 3
		minPasswordLength = 8
		maxUsernameLength = 128
	)

	username := strings.ToLower(strings.TrimSpace(req.Username))
	password := strings.TrimSpace(req.Password)

	if len(username) < minUsernameLength || len(username) > maxUsernameLength {
		return nil, status.Errorf(codes.InvalidArgument, "username must be between %d and %d characters", minUsernameLength, maxUsernameLength)
	}
	if len(password) < minPasswordLength {
		return nil, status.Errorf(codes.InvalidArgument, "password must be at least %d characters", minPasswordLength)
	}

	roles, err := jwt.NormalizeRoles(req.Roles)
	if err != nil {
		metrics.RegisterTotal.WithLabelValues("invalid").Inc()
		h.log.WithContext(ctx).Warn("invalid roles in register", zap.Error(err))
		return nil, status.Errorf(codes.InvalidArgument, "invalid roles: %v", err)
	}

	exists, err := h.users.ExistsByUsername(ctx, username)
	if err != nil {
		h.log.WithContext(ctx).Error("check username failed", zap.Error(err))
		return nil, fmt.Errorf("check username: %w", err)
	}
	if exists {
		return nil, status.Error(codes.AlreadyExists, "username already exists")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		h.log.WithContext(ctx).Error("hash password failed", zap.Error(err))
		return nil, fmt.Errorf("hash password: %w", err)
	}

	userID := uuid.NewString()
	user := &postgres.User{
		ID:           userID,
		Username:     username,
		PasswordHash: string(hash),
		Roles:        roles,
	}
	if err := h.users.Create(ctx, user); err != nil {
		h.log.WithContext(ctx).Error("create user failed", zap.Error(err))
		return nil, fmt.Errorf("create user: %w", err)
	}

	access, accessClaims, err := h.signer.Generate(userID, roles, jwt.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("generate access: %w", err)
	}
	refresh, refreshClaims, err := h.signer.Generate(userID, roles, jwt.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("generate refresh: %w", err)
	}

	err = backoff.Execute(ctx, backoff.Config{MaxElapsedTime: 2 * time.Second}, func(ctx context.Context) error {
		return h.tokens.Store(ctx, &postgres.RefreshToken{
			ID:        refreshClaims.JTI,
			UserID:    userID,
			JTI:       refreshClaims.JTI,
			Token:     refresh,
			IssuedAt:  refreshClaims.IssuedAt.Time,
			ExpiresAt: refreshClaims.ExpiresAt.Time,
		})
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("store refresh: %w", err)
	}

	metrics.RegisterTotal.WithLabelValues("success").Inc()
	metrics.IssuedTokens.WithLabelValues("access").Inc()
	metrics.IssuedTokens.WithLabelValues("refresh").Inc()

	return &authpb.RegisterResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(time.Until(accessClaims.ExpiresAt.Time).Seconds()),
	}, nil
}
