// auth/internal/storage/postgres/config.go
package postgres

import "fmt"

type Config struct {
	DSN           string `mapstructure:"dsn"`
	MigrationsDir string `mapstructure:"migrations_dir"`
}

func (c *Config) ApplyDefaults() {
	if c.MigrationsDir == "" {
		c.MigrationsDir = "/app/migrations/postgres"
	}
}

func (c Config) Validate() error {
	if c.DSN == "" {
		return fmt.Errorf("postgres: dsn is required")
	}
	if c.MigrationsDir == "" {
		return fmt.Errorf("postgres: migrations_dir is required")
	}
	return nil
}
