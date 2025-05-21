// auth/internal/jwt/claims.go
package jwt

import (
	"github.com/golang-jwt/jwt/v5"
)

// Claims описывает payload токена.
type Claims struct {
	jwt.RegisteredClaims
	UserID string   `json:"sub"`
	Roles  []string `json:"roles"`
	JTI    string   `json:"jti"`
}
