// auth/internal/storage/postgres/token.go
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type tokenRepo struct {
	db *pgxpool.Pool
}

func NewTokenRepo(db *pgxpool.Pool) TokenRepository {
	return &tokenRepo{db: db}
}

func (r *tokenRepo) Store(ctx context.Context, t *RefreshToken) error {
	const q = `
	INSERT INTO refresh_tokens (id, user_id, jti, token, issued_at, expires_at)
	VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := r.db.Exec(ctx, q, t.ID, t.UserID, t.JTI, t.Token, t.IssuedAt, t.ExpiresAt)
	if err != nil {
		return fmt.Errorf("store refresh token: %w", err)
	}
	return nil
}

func (r *tokenRepo) FindByJTI(ctx context.Context, jti string) (*RefreshToken, error) {
	const q = `
	SELECT id, user_id, jti, token, issued_at, expires_at, revoked_at
	FROM refresh_tokens WHERE jti = $1`
	row := r.db.QueryRow(ctx, q, jti)

	var t RefreshToken
	err := row.Scan(&t.ID, &t.UserID, &t.JTI, &t.Token, &t.IssuedAt, &t.ExpiresAt, &t.RevokedAt)
	if err != nil {
		return nil, fmt.Errorf("token not found: %w", err)
	}
	return &t, nil
}

func (r *tokenRepo) RevokeByJTI(ctx context.Context, jti string) error {
	now := time.Now().UTC()
	const q = `UPDATE refresh_tokens SET revoked_at = $1 WHERE jti = $2`
	_, err := r.db.Exec(ctx, q, now, jti)
	if err != nil {
		return fmt.Errorf("revoke token failed: %w", err)
	}
	return nil
}
