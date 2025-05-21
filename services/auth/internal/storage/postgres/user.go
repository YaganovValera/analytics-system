// internal/storage/postgres/user.go
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type userRepo struct {
	db *pgxpool.Pool
}

func NewUserRepo(db *pgxpool.Pool) UserRepository {
	return &userRepo{db: db}
}

func (r *userRepo) FindByUsername(ctx context.Context, username string) (*User, error) {
	query := `SELECT id, username, password_hash, roles, created_at FROM users WHERE username = $1`
	row := r.db.QueryRow(ctx, query, username)

	var u User
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Roles, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return &u, nil
}

func (r *userRepo) Create(ctx context.Context, u *User) error {
	query := `INSERT INTO users (id, username, password_hash, roles) VALUES ($1, $2, $3, $4)`
	_, err := r.db.Exec(ctx, query, u.ID, u.Username, u.PasswordHash, u.Roles)
	if err != nil {
		return fmt.Errorf("user insert failed: %w", err)
	}
	return nil
}
