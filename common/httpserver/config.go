// common/httpserver/config.go

package httpserver

import (
	"fmt"
	"time"
)

// Config определяет настройки HTTP-сервера.
type Config struct {
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	IdleTimeout     time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
	MetricsPath     string        `mapstructure:"metrics_path"`
	HealthzPath     string        `mapstructure:"healthz_path"`
	ReadyzPath      string        `mapstructure:"readyz_path"`
}

func (c *Config) ApplyDefaults() {
	if c.ReadTimeout <= 0 {
		c.ReadTimeout = 10 * time.Second
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = 15 * time.Second
	}
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = 60 * time.Second
	}
	if c.ShutdownTimeout <= 0 {
		c.ShutdownTimeout = 5 * time.Second
	}
	if c.MetricsPath == "" {
		c.MetricsPath = "/metrics"
	}
	if c.HealthzPath == "" {
		c.HealthzPath = "/healthz"
	}
	if c.ReadyzPath == "" {
		c.ReadyzPath = "/readyz"
	}
}

func (c Config) Validate() error {
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("httpserver: port must be valid")
	}
	return nil
}
