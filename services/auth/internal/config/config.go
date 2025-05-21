// auth/internal/config/config.go
package config

import (
	"fmt"

	commoncfg "github.com/YaganovValera/analytics-system/common/config"
	commonhttp "github.com/YaganovValera/analytics-system/common/httpserver"
	commonlogger "github.com/YaganovValera/analytics-system/common/logger"
	commontel "github.com/YaganovValera/analytics-system/common/telemetry"

	jwtconfig "github.com/YaganovValera/analytics-system/services/auth/internal/jwt"
	pgconfig "github.com/YaganovValera/analytics-system/services/auth/internal/storage/postgres"
)

// Config описывает параметры запуска auth-сервиса.
type Config struct {
	ServiceName    string `mapstructure:"service_name"`
	ServiceVersion string `mapstructure:"service_version"`

	Logging   commonlogger.Config `mapstructure:"logging"`
	Telemetry commontel.Config    `mapstructure:"telemetry"`
	HTTP      commonhttp.Config   `mapstructure:"http"`
	JWT       jwtconfig.JWTConfig `mapstructure:"jwt"`
	Postgres  pgconfig.Config     `mapstructure:"postgres"`
}

// Load читает конфиг и валидирует все вложенные поля.
func Load(path string) (*Config, error) {
	var cfg Config
	if err := commoncfg.Load(commoncfg.Options{
		Path:      path,
		EnvPrefix: "AUTH",
		Out:       &cfg,
		Defaults: map[string]interface{}{
			"service_name":    "auth",
			"service_version": "v1.0.0",

			// Logging
			"logging.level":    "info",
			"logging.dev_mode": false,
			"logging.format":   "console",

			// Telemetry
			"telemetry.endpoint":         "otel-collector:4317",
			"telemetry.insecure":         true,
			"telemetry.reconnect_period": "5s",
			"telemetry.timeout":          "5s",
			"telemetry.sampler_ratio":    1.0,

			// HTTP
			"http.port":             8084,
			"http.read_timeout":     "10s",
			"http.write_timeout":    "15s",
			"http.idle_timeout":     "60s",
			"http.shutdown_timeout": "5s",
			"http.metrics_path":     "/metrics",
			"http.healthz_path":     "/healthz",
			"http.readyz_path":      "/readyz",

			// JWT
			"jwt.secret":      "",
			"jwt.access_ttl":  "15m",
			"jwt.refresh_ttl": "168h",
			"jwt.issuer":      "auth-service",
			"jwt.audience":    "analytics-system",

			// PostgreSQL
			"postgres.dsn":            "postgres://user:pass@postgres:5432/auth?sslmode=disable",
			"postgres.migrations_dir": "/app/migrations/postgres",
		},
	}); err != nil {
		return nil, fmt.Errorf("config load failed: %w", err)
	}

	// Defaults
	cfg.Logging.ApplyDefaults()
	cfg.Telemetry.ApplyDefaults()
	cfg.HTTP.ApplyDefaults()
	cfg.Postgres.ApplyDefaults()

	// Validation
	if cfg.ServiceName == "" || cfg.ServiceVersion == "" {
		return nil, fmt.Errorf("service name/version required")
	}
	if err := cfg.Logging.Validate(); err != nil {
		return nil, fmt.Errorf("logging: %w", err)
	}
	if err := cfg.Telemetry.Validate(); err != nil {
		return nil, fmt.Errorf("telemetry: %w", err)
	}
	if err := cfg.HTTP.Validate(); err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	if err := cfg.JWT.Validate(); err != nil {
		return nil, fmt.Errorf("jwt: %w", err)
	}
	if err := cfg.Postgres.Validate(); err != nil {
		return nil, fmt.Errorf("postgres: %w", err)
	}

	return &cfg, nil
}
