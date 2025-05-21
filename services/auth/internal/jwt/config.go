// auth/internal/jwt/config.go

package jwt

import (
	"fmt"
	"time"
)

// JWTConfig описывает параметры генерации и валидации JWT.
type JWTConfig struct {
	Secret     string `mapstructure:"secret"`
	AccessTTL  string `mapstructure:"access_ttl"`
	RefreshTTL string `mapstructure:"refresh_ttl"`
	Issuer     string `mapstructure:"issuer"`
	Audience   string `mapstructure:"audience"`
}

func (c JWTConfig) Validate() error {
	if c.AccessTTL == "" {
		return fmt.Errorf("jwt: access_ttl is required")
	}
	if c.RefreshTTL == "" {
		return fmt.Errorf("jwt: refresh_ttl is required")
	}
	if c.Issuer == "" || c.Audience == "" {
		return fmt.Errorf("jwt: issuer and audience are required")
	}
	_, err := time.ParseDuration(c.AccessTTL)
	if err != nil {
		return fmt.Errorf("invalid access_ttl: %w", err)
	}
	_, err = time.ParseDuration(c.RefreshTTL)
	if err != nil {
		return fmt.Errorf("invalid refresh_ttl: %w", err)
	}

	return nil
}
