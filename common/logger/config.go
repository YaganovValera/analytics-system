// common/logger/config.go ---
package logger

import "fmt"

type Config struct {
	Level   string `mapstructure:"level"`
	DevMode bool   `mapstructure:"dev_mode"`
	Format  string `mapstructure:"format"` // optional: "console" | "json"
}

func (c *Config) ApplyDefaults() {
	if c.Level == "" {
		c.Level = "info"
	}
	if c.Format == "" {
		c.Format = "console"
	}
}

func (c Config) Validate() error {
	if c.Level == "" {
		return fmt.Errorf("logger: level is required")
	}
	return nil
}
