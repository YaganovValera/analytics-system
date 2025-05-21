// auth/internal/interceptor/jwt_interceptor.go

package interceptor

import (
	"context"
	"strings"

	"github.com/YaganovValera/analytics-system/common/ctxkeys"
	"github.com/YaganovValera/analytics-system/services/auth/internal/jwt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// UnaryJWTInterceptor validates access token and enriches context.
func UnaryJWTInterceptor(verifier jwt.Verifier) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		authHeader := ""
		if vals := md.Get("authorization"); len(vals) > 0 {
			authHeader = vals[0]
		}
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return nil, status.Error(codes.Unauthenticated, "missing or invalid authorization header")
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		claims, err := verifier.Parse(tokenStr)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
		}

		// Enrich context
		ctx = context.WithValue(ctx, ctxkeys.UserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, ctxkeys.RolesKey, claims.Roles)
		ctx = context.WithValue(ctx, ctxkeys.JTI, claims.JTI)

		return handler(ctx, req)
	}
}

// FromContext извлекает user info из контекста.
type AuthContext struct {
	UserID string
	Roles  []string
	JTI    string
}

func FromContext(ctx context.Context) *AuthContext {
	uid, _ := ctx.Value(ctxkeys.UserIDKey).(string)
	roles, _ := ctx.Value(ctxkeys.RolesKey).([]string)
	jti, _ := ctx.Value(ctxkeys.JTI).(string)
	return &AuthContext{
		UserID: uid,
		Roles:  roles,
		JTI:    jti,
	}
}

// RequireRoles проверяет наличие хотя бы одной из требуемых ролей.
func RequireRoles(ctx context.Context, allowed ...string) error {
	roles, _ := ctx.Value(ctxkeys.RolesKey).([]string)
	roleSet := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		roleSet[r] = struct{}{}
	}
	for _, want := range allowed {
		if _, ok := roleSet[want]; ok {
			return nil
		}
	}
	return status.Error(codes.PermissionDenied, "insufficient role")
}
