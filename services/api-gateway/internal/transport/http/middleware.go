// api-gateway/internal/transport/http/middleware.go
package http

import (
	"net/http"
	"strings"

	"github.com/YaganovValera/analytics-system/services/api-gateway/internal/client/auth"
	"google.golang.org/grpc/metadata"
)

// Middleware описывает зависимые middleware компоненты.
type Middleware struct {
	auth *auth.Client
}

// NewMiddleware создаёт Middleware.
func NewMiddleware(auth *auth.Client) *Middleware {
	return &Middleware{auth: auth}
}

// WithContext извлекает Authorization токен и прокидывает его как gRPC metadata.
func (m *Middleware) WithContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
