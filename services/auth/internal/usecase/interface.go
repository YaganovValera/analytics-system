// auth/internal/usecase/interface.go
package usecase

import (
	"context"

	authpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/auth"
)

type LoginHandler interface {
	Handle(ctx context.Context, req *authpb.LoginRequest) (*authpb.LoginResponse, error)
}

type ValidateTokenHandler interface {
	Handle(ctx context.Context, token string) (*authpb.ValidateTokenResponse, error)
}

type RefreshTokenHandler interface {
	Handle(ctx context.Context, req *authpb.RefreshTokenRequest) (*authpb.RefreshTokenResponse, error)
}

type RevokeTokenHandler interface {
	Handle(ctx context.Context, req *authpb.RevokeTokenRequest) (*authpb.RevokeTokenResponse, error)
}

type LogoutHandler interface {
	Handle(ctx context.Context, jti string) error
}

type RegisterHandler interface {
	Handle(ctx context.Context, req *authpb.RegisterRequest) (*authpb.RegisterResponse, error)
}
