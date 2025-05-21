// api-gateway/internal/transport/http/middleware.go
package http

import (
	"context"
	"net/http"
	"strings"

	authpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/auth"
)

type Middleware struct {
	authClient authpb.AuthServiceClient
}

func NewMiddleware(authClient authpb.AuthServiceClient) *Middleware {
	return &Middleware{authClient}
}

type contextKey string

const (
	ctxUserID contextKey = "user_id"
	ctxRoles  contextKey = "roles"
)

func (m *Middleware) JWTMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")

		resp, err := m.authClient.ValidateToken(r.Context(), &authpb.ValidateTokenRequest{Token: token})
		if err != nil || !resp.Valid {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), ctxUserID, resp.Username)
		ctx = context.WithValue(ctx, ctxRoles, resp.Roles)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *Middleware) RBAC(allowed []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.Context().Value(ctxRoles)
			roles, ok := raw.([]string)
			if !ok {
				http.Error(w, "missing roles", http.StatusForbidden)
				return
			}
			for _, role := range roles {
				for _, want := range allowed {
					if role == want {
						next.ServeHTTP(w, r)
						return
					}
				}
			}
			http.Error(w, "forbidden", http.StatusForbidden)
		})
	}
}
