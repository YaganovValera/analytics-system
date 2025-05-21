// auth/internal/jwt/role.go
package jwt

import (
	"fmt"
	"strings"
)

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleUser   Role = "user"
	RoleViewer Role = "viewer"
)

// ValidRoles возвращает все допустимые роли.
func ValidRoles() []Role {
	return []Role{RoleAdmin, RoleUser, RoleViewer}
}

// IsValidRole проверяет, допустима ли роль (строковое значение).
func IsValidRole(r string) bool {
	switch Role(strings.ToLower(r)) {
	case RoleAdmin, RoleUser, RoleViewer:
		return true
	default:
		return false
	}
}

// NormalizeRoles очищает, приводит к нижнему регистру, удаляет дубликаты и валидирует.
func NormalizeRoles(raw []string) ([]string, error) {
	seen := make(map[string]struct{}, len(raw))
	var result []string

	for _, r := range raw {
		role := strings.ToLower(strings.TrimSpace(r))
		if !IsValidRole(role) {
			return nil, fmt.Errorf("invalid role: %q. Allowed: %v", role, ValidRoles())
		}
		if _, ok := seen[role]; !ok {
			seen[role] = struct{}{}
			result = append(result, role)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("at least one valid role required")
	}
	return result, nil
}
