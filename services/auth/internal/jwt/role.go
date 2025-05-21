// auth/internal/jwt/role.go

package jwt

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleUser   Role = "user"
	RoleViewer Role = "viewer"
)

func IsValidRole(r string) bool {
	switch Role(r) {
	case RoleAdmin, RoleUser, RoleViewer:
		return true
	default:
		return false
	}
}
