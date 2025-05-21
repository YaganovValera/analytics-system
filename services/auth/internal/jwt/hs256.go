// auth/internal/jwt/hs256.go
package jwt

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type HS256 struct {
	secret     []byte
	issuer     string
	audience   string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewHS256 создаёт Signer/Verifier на HS256.
func NewHS256(secret, issuer, audience string, accessTTL, refreshTTL time.Duration) (*HS256, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("jwt secret too short, must be ≥32 bytes")
	}
	if issuer == "" || audience == "" {
		return nil, fmt.Errorf("issuer and audience required")
	}
	if accessTTL < time.Minute || refreshTTL < time.Minute {
		return nil, fmt.Errorf("TTL too short")
	}
	return &HS256{
		secret:     []byte(secret),
		issuer:     issuer,
		audience:   audience,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}, nil
}

// Generate создаёт JWT токен (access или refresh).
func (h *HS256) Generate(userID string, roles []string, typ TokenType) (string, *Claims, error) {
	now := time.Now()
	var exp time.Time
	switch typ {
	case AccessToken:
		exp = now.Add(h.accessTTL)
	case RefreshToken:
		exp = now.Add(h.refreshTTL)
	default:
		return "", nil, fmt.Errorf("unsupported token type: %s", typ)
	}

	claims := &Claims{
		UserID: userID,
		Roles:  roles,
		JTI:    uuid.NewString(),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    h.issuer,
			Audience:  []string{h.audience},
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(h.secret)
	if err != nil {
		return "", nil, fmt.Errorf("sign jwt: %w", err)
	}
	return signed, claims, nil
}

// Parse валидирует токен и возвращает Claims.
func (h *HS256) Parse(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %T", token.Method)
		}
		return h.secret, nil
	}, jwt.WithIssuer(h.issuer), jwt.WithAudience(h.audience))
	if err != nil {
		return nil, fmt.Errorf("parse jwt: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token or claims")
	}

	return claims, nil
}
