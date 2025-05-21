// auth/internal/interceptor/rbac_interceptor.go

package interceptor

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// methodRoles задаёт допустимые роли для gRPC методов.
var methodRoles = map[string][]string{
	"/market.auth.v1.AuthService/RevokeToken": {"admin"},
	"/market.auth.v1.AuthService/Register":    {"admin"},
}

// UnaryRBACInterceptor проверяет, имеет ли пользователь доступ к методу по ролям.
func UnaryRBACInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		allowed, ok := methodRoles[info.FullMethod]
		if !ok {
			// метод открыт — пропускаем
			return handler(ctx, req)
		}
		if err := RequireRoles(ctx, allowed...); err != nil {
			return nil, status.Errorf(codes.PermissionDenied,
				"rbac: access denied for method %q, required roles: %s",
				info.FullMethod, strings.Join(allowed, ", "),
			)
		}
		return handler(ctx, req)
	}
}
