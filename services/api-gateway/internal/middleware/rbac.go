// api-gateway/internal/middleware/rbac.go

package middleware

import (
	"net/http"
	"strings"

	"github.com/YaganovValera/analytics-system/common/ctxkeys"
	"github.com/YaganovValera/analytics-system/common/logger"

	"go.uber.org/zap"
)

type Route struct {
	Method string
	Path   string // может быть exact или prefix
}

type RBACConfig struct {
	Permissions map[Route][]string
	Log         *logger.Logger
}

// RBACMiddleware возвращает HTTP middleware, ограничивающее доступ по ролям.
func RBACMiddleware(cfg RBACConfig) func(http.Handler) http.Handler {
	log := cfg.Log.Named("rbac")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			method := r.Method
			path := r.URL.Path

			requiredRoles := findMatchingRoles(cfg.Permissions, method, path)
			if len(requiredRoles) == 0 {
				// доступ свободен
				next.ServeHTTP(w, r)
				return
			}

			userRoles, _ := ctx.Value(ctxkeys.RolesKey).([]string)
			if !hasAnyRole(userRoles, requiredRoles) {
				log.WithContext(ctx).Warn("RBAC: access denied",
					zap.String("method", method),
					zap.String("path", path),
					zap.Any("required_roles", requiredRoles),
					zap.Any("user_roles", userRoles),
				)
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// findMatchingRoles ищет подходящее правило по методу и префиксу пути.
func findMatchingRoles(perms map[Route][]string, method, path string) []string {
	for route, roles := range perms {
		if strings.EqualFold(route.Method, method) &&
			(strings.HasSuffix(route.Path, "*") && strings.HasPrefix(path, strings.TrimSuffix(route.Path, "*")) ||
				route.Path == path) {
			return roles
		}
	}
	return nil
}

// hasAnyRole проверяет, есть ли у пользователя хоть одна нужная роль.
func hasAnyRole(userRoles, required []string) bool {
	roleSet := make(map[string]struct{}, len(userRoles))
	for _, r := range userRoles {
		roleSet[strings.ToLower(r)] = struct{}{}
	}
	for _, r := range required {
		if _, ok := roleSet[strings.ToLower(r)]; ok {
			return true
		}
	}
	return false
}
