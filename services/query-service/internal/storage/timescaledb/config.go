// query-service/internal/storage/timescaledb/config.go
package timescaledb

import "fmt"

type Config struct {
	DSN string `mapstructure:"dsn"`
}

func (c *Config) ApplyDefaults() {
	if c.DSN == "" {
		c.DSN = "postgres://user:pass@timescaledb:5432/analytics?sslmode=disable"
	}
}

func (c Config) Validate() error {
	if c.DSN == "" {
		return fmt.Errorf("timescaledb: dsn is required")
	}
	return nil
}
