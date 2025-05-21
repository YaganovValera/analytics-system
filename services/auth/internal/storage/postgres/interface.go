// auth/internal/storage/postgres/interface.go
package postgres

import (
	"context"
	"time"
)

type User struct {
	ID           string
	Username     string
	PasswordHash string
	Roles        []string
	CreatedAt    time.Time
}

type RefreshToken struct {
	ID        string
	UserID    string
	JTI       string
	Token     string
	IssuedAt  time.Time
	ExpiresAt time.Time
	RevokedAt *time.Time
}

type UserRepository interface {
	FindByUsername(ctx context.Context, username string) (*User, error)
	Create(ctx context.Context, user *User) error
	ExistsByUsername(ctx context.Context, username string) (bool, error)
}

type TokenRepository interface {
	Store(ctx context.Context, token *RefreshToken) error
	FindByJTI(ctx context.Context, jti string) (*RefreshToken, error)
	RevokeByJTI(ctx context.Context, jti string) error
}
