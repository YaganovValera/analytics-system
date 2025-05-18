// github.com/YaganovValera/analytics-system/services/analytics-api/internal/config/config.go
package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/YaganovValera/analytics-system/common/backoff"
	"github.com/YaganovValera/analytics-system/common/config"
)

type Config struct {
	ServiceName    string          `mapstructure:"service_name"`
	ServiceVersion string          `mapstructure:"service_version"`
	Logging        Logging         `mapstructure:"logging"`
	Telemetry      Telemetry       `mapstructure:"telemetry"`
	HTTP           HTTPConfig      `mapstructure:"http"`
	Timescale      TimescaleConfig `mapstructure:"timescaledb"`
	Kafka          KafkaConfig     `mapstructure:"kafka"`
}

type Logging struct {
	Level   string `mapstructure:"level"`
	DevMode bool   `mapstructure:"dev_mode"`
}

type Telemetry struct {
	OTLPEndpoint string `mapstructure:"otel_endpoint"`
	Insecure     bool   `mapstructure:"insecure"`
}

type HTTPConfig struct {
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	IdleTimeout     time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
	MetricsPath     string        `mapstructure:"metrics_path"`
	HealthzPath     string        `mapstructure:"healthz_path"`
	ReadyzPath      string        `mapstructure:"readyz_path"`
}

type TimescaleConfig struct {
	DSN string `mapstructure:"dsn"`
}

type KafkaConfig struct {
	Brokers     []string       `mapstructure:"brokers"`
	GroupID     string         `mapstructure:"group_id"`
	Version     string         `mapstructure:"version"`
	TopicPrefix string         `mapstructure:"topic_prefix"`
	Backoff     backoff.Config `mapstructure:"backoff"`
}

func Load(path string) (*Config, error) {
	var cfg Config
	err := config.Load(config.Options{
		Path:      path,
		EnvPrefix: "ANALYTICS",
		Out:       &cfg,
		Defaults: map[string]interface{}{
			"service_name":    "analytics-api",
			"service_version": "v1.0.0",

			"logging.level":    "info",
			"logging.dev_mode": false,

			"telemetry.otel_endpoint": "otel-collector:4317",
			"telemetry.insecure":      true,

			"http.port":             8082,
			"http.read_timeout":     "10s",
			"http.write_timeout":    "15s",
			"http.idle_timeout":     "60s",
			"http.shutdown_timeout": "5s",
			"http.metrics_path":     "/metrics",
			"http.healthz_path":     "/healthz",
			"http.readyz_path":      "/readyz",

			"timescaledb.dsn": "postgres://user:pass@timescaledb:5432/analytics?sslmode=disable",

			"kafka.group_id":     "analytics-api",
			"kafka.version":      "2.8.0",
			"kafka.topic_prefix": "candles",
		},
	})
	if err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}
	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.ServiceName == "" || c.ServiceVersion == "" {
		return fmt.Errorf("service name and version are required")
	}
	if c.Telemetry.OTLPEndpoint == "" {
		return fmt.Errorf("telemetry.otel_endpoint is required")
	}
	if err := validateLogLevel(c.Logging.Level); err != nil {
		return err
	}
	if err := validateHTTP(&c.HTTP); err != nil {
		return err
	}
	if len(c.Kafka.Brokers) == 0 || c.Kafka.GroupID == "" || c.Kafka.Version == "" || c.Kafka.TopicPrefix == "" {
		return fmt.Errorf("kafka config incomplete")
	}
	if c.Timescale.DSN == "" {
		return fmt.Errorf("timescaledb.dsn is required")
	}
	return nil
}

func validateLogLevel(level string) error {
	switch strings.ToLower(level) {
	case "debug", "info", "warn", "error":
		return nil
	default:
		return fmt.Errorf("invalid logging.level: %s", level)
	}
}

func validateHTTP(cfg *HTTPConfig) error {
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return fmt.Errorf("http.port must be between 1 and 65535")
	}
	if cfg.ReadTimeout <= 0 || cfg.WriteTimeout <= 0 || cfg.IdleTimeout <= 0 || cfg.ShutdownTimeout <= 0 {
		return fmt.Errorf("http timeouts must be positive")
	}
	for _, path := range []string{cfg.MetricsPath, cfg.HealthzPath, cfg.ReadyzPath} {
		if !strings.HasPrefix(path, "/") {
			return fmt.Errorf("http path %q must start with '/'", path)
		}
	}
	return nil
}

func (c *Config) Print() {
	b, _ := json.MarshalIndent(c, "", "  ")
	fmt.Println("Loaded configuration:\n", string(b))
}
