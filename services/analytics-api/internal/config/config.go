// analytics-api/internal/config/config.go
package config

import (
	"fmt"

	commoncfg "github.com/YaganovValera/analytics-system/common/config"
	commonhttp "github.com/YaganovValera/analytics-system/common/httpserver"
	consumercfg "github.com/YaganovValera/analytics-system/common/kafka/consumer"
	commonlogger "github.com/YaganovValera/analytics-system/common/logger"
	commontelemetry "github.com/YaganovValera/analytics-system/common/telemetry"
	timescale "github.com/YaganovValera/analytics-system/services/analytics-api/internal/storage/timescaledb"
)

type Config struct {
	ServiceName    string `mapstructure:"service_name"`
	ServiceVersion string `mapstructure:"service_version"`

	Logging   commonlogger.Config    `mapstructure:"logging"`
	Telemetry commontelemetry.Config `mapstructure:"telemetry"`
	HTTP      commonhttp.Config      `mapstructure:"http"`

	Kafka     consumercfg.Config `mapstructure:"kafka"`
	TopicBase string             `mapstructure:"topic_base"`

	Timescale timescale.Config `mapstructure:"timescaledb"`
}

// Load читает конфигурацию из YAML и/или окружения.
func Load(path string) (*Config, error) {
	var cfg Config

	if err := commoncfg.Load(commoncfg.Options{
		Path:      path,
		EnvPrefix: "ANALYTICS",
		Out:       &cfg,
		Defaults: map[string]interface{}{
			// Service
			"service_name":    "analytics-api",
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
			"telemetry.service_name":     "analytics-api",
			"telemetry.service_version":  "v1.0.0",

			// HTTP
			"http.port":             8082,
			"http.read_timeout":     "10s",
			"http.write_timeout":    "15s",
			"http.idle_timeout":     "60s",
			"http.shutdown_timeout": "5s",
			"http.metrics_path":     "/metrics",
			"http.healthz_path":     "/healthz",
			"http.readyz_path":      "/readyz",

			// Kafka
			"kafka.brokers":  []string{"kafka:9092"},
			"kafka.version":  "2.8.0",
			"kafka.group_id": "analytics-api",
			"kafka.backoff": map[string]interface{}{
				"initial_interval": "1s",
				"max_interval":     "30s",
				"max_elapsed_time": "5m",
			},
			"topic_base": "candles",

			// TimescaleDB
			"timescaledb.dsn":            "postgres://user:pass@timescaledb:5432/analytics?sslmode=disable",
			"timescaledb.migrations_dir": "/app/migrations/timescaledb",
		},
	}); err != nil {
		return nil, fmt.Errorf("config load failed: %w", err)
	}

	cfg.ApplyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// ApplyDefaults вызывает ApplyDefaults() всех вложенных компонентов.
func (c *Config) ApplyDefaults() {
	c.Logging.ApplyDefaults()
	c.Telemetry.ApplyDefaults()
	c.HTTP.ApplyDefaults()
	c.Kafka.ApplyDefaults()
}

// Validate проверяет все вложенные компоненты.
func (c *Config) Validate() error {
	if c.ServiceName == "" || c.ServiceVersion == "" {
		return fmt.Errorf("service name/version required")
	}
	if err := c.Logging.Validate(); err != nil {
		return fmt.Errorf("logging: %w", err)
	}
	if err := c.Telemetry.Validate(); err != nil {
		return fmt.Errorf("telemetry: %w", err)
	}
	if err := c.HTTP.Validate(); err != nil {
		return fmt.Errorf("http: %w", err)
	}
	if err := c.Kafka.Validate(); err != nil {
		return fmt.Errorf("kafka: %w", err)
	}
	if err := c.Timescale.Validate(); err != nil {
		return fmt.Errorf("timescaledb: %w", err)
	}
	if c.TopicBase == "" {
		return fmt.Errorf("topic_base is required")
	}
	return nil
}
