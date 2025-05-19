// common/httpserver/config.go

package httpserver

import (
	"fmt"
	"time"
)

// Config определяет настройки HTTP-сервера.
type Config struct {
	Addr            string        // адрес для Listen, например ":8080"
	ReadTimeout     time.Duration // максимальное время чтения запроса
	WriteTimeout    time.Duration // максимальное время записи ответа
	IdleTimeout     time.Duration // максимальное время простоя соединения
	ShutdownTimeout time.Duration // таймаут для graceful shutdown
	MetricsPath     string        // путь для /metrics
	HealthzPath     string        // путь для /healthz
	ReadyzPath      string        // путь для /readyz
}

func (c *Config) applyDefaults() {
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

func (c Config) validate() error {
	if c.Addr == "" {
		return fmt.Errorf("httpserver: Addr is required")
	}
	return nil
}
