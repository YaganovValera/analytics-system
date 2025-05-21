// api-gateway/internal/middleware/jwt.go
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/YaganovValera/analytics-system/common/ctxkeys"
	"github.com/YaganovValera/analytics-system/common/logger"
	"go.uber.org/zap"

	authpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/auth"

	"go.opentelemetry.io/otel"
)

var jwtTracer = otel.Tracer("gateway/middleware/jwt")

// JWTMiddleware валидирует токен через AuthService и обогащает контекст.
func JWTMiddleware(auth authpb.AuthServiceClient, cache *JWTCache, log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, span := jwtTracer.Start(r.Context(), "JWTMiddleware")
			defer span.End()

			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				log.WithContext(ctx).Warn("JWT: missing Authorization header")
				return
			}
			token := strings.TrimPrefix(authHeader, "Bearer ")

			// Проверка кэша
			var resp *authpb.ValidateTokenResponse
			if cached, ok := cache.Get(token); ok {
				resp = cached
			} else {
				var err error
				resp, err = auth.ValidateToken(ctx, &authpb.ValidateTokenRequest{Token: token})
				if err != nil || !resp.Valid {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					log.WithContext(ctx).Warn("JWT: token validation failed", zap.Error(err))
					return
				}
				cache.Put(token, resp)
			}

			// Обогащение контекста
			ctx = withAuthContext(ctx, resp)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// withAuthContext обогащает context значениями из proto.
func withAuthContext(ctx context.Context, resp *authpb.ValidateTokenResponse) context.Context {
	ctx = context.WithValue(ctx, ctxkeys.UserIDKey, resp.Username)
	ctx = context.WithValue(ctx, ctxkeys.RolesKey, resp.Roles)

	if meta := resp.Metadata; meta != nil {
		if meta.TraceId != "" {
			ctx = context.WithValue(ctx, ctxkeys.TraceIDKey, meta.TraceId)
		}
		if meta.IpAddress != "" {
			ctx = context.WithValue(ctx, ctxkeys.IPAddressKey, meta.IpAddress)
		}
		if meta.UserAgent != "" {
			ctx = context.WithValue(ctx, ctxkeys.UserAgentKey, meta.UserAgent)
		}
	}
	return ctx
}
