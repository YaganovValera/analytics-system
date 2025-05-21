// auth/internal/jwt/jwt.go
package jwt

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

type TokenType string

const (
	AccessToken  TokenType = "access"
	RefreshToken TokenType = "refresh"
)

// Signer генерирует токены.
type Signer interface {
	Generate(userID string, roles []string, typ TokenType) (string, *Claims, error)
}

// Verifier проверяет подпись, срок, структуру.
type Verifier interface {
	Parse(token string) (*Claims, error)
}

func ParseUnverifiedJTI(tokenStr string) (*Claims, error) {
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())

	token, _, err := parser.ParseUnverified(tokenStr, &Claims{})
	if err != nil {
		return nil, fmt.Errorf("parse unverified: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, fmt.Errorf("invalid claims")
	}
	return claims, nil
}
