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

func IsValidRole(r string) bool {
	switch Role(r) {
	case RoleAdmin, RoleUser, RoleViewer:
		return true
	default:
		return false
	}
}

func DeduplicateAndValidateRoles(raw []string) ([]string, error) {
	seen := map[string]struct{}{}
	var clean []string

	for _, r := range raw {
		role := strings.ToLower(strings.TrimSpace(r))
		if !IsValidRole(role) {
			return nil, fmt.Errorf("invalid role: %s", role)
		}
		if _, ok := seen[role]; !ok {
			seen[role] = struct{}{}
			clean = append(clean, role)
		}
	}
	if len(clean) == 0 {
		return nil, fmt.Errorf("no valid roles provided")
	}
	return clean, nil
}
