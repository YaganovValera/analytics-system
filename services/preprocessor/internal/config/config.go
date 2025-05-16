// github.com/YaganovValera/analytics-system/services/preprocessor/internal/config/config.go
// internal/config/config.go
package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/YaganovValera/analytics-system/common/backoff"
	"github.com/YaganovValera/analytics-system/common/config"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/storage/redis"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/storage/timescaledb"
)

type Config struct {
	ServiceName    string             `mapstructure:"service_name"`
	ServiceVersion string             `mapstructure:"service_version"`
	Kafka          KafkaConfig        `mapstructure:"kafka"`
	Redis          redis.RedisConfig  `mapstructure:"redis"`
	Timescale      timescaledb.Config `mapstructure:"timescaledb"`
	Telemetry      Telemetry          `mapstructure:"telemetry"`
	Logging        Logging            `mapstructure:"logging"`
	HTTP           HTTPConfig         `mapstructure:"http"`
	Intervals      []string           `mapstructure:"intervals"`
}

type KafkaConfig struct {
	Brokers           []string       `mapstructure:"brokers"`
	Version           string         `mapstructure:"version"`
	GroupID           string         `mapstructure:"group_id"`
	RawTopic          string         `mapstructure:"raw_topic"`
	OutputTopicPrefix string         `mapstructure:"output_topic_prefix"`
	Backoff           backoff.Config `mapstructure:"backoff"`
}

type Telemetry struct {
	OTLPEndpoint string `mapstructure:"otel_endpoint"`
	Insecure     bool   `mapstructure:"insecure"`
}

type Logging struct {
	Level   string `mapstructure:"level"`
	DevMode bool   `mapstructure:"dev_mode"`
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

func Load(path string) (*Config, error) {
	var cfg Config
	err := config.Load(config.Options{
		Path:      path,
		EnvPrefix: "PREPROCESSOR",
		Out:       &cfg,
		Defaults: map[string]interface{}{
			"service_name":    "preprocessor",
			"service_version": "v1.0.0",

			"kafka.version":             "2.8.0",
			"kafka.group_id":            "preprocessor",
			"kafka.raw_topic":           "marketdata.raw",
			"kafka.output_topic_prefix": "candles",

			"redis.addr":     "redis:6379",
			"redis.db":       0,
			"redis.password": "",

			"timescaledb.dsn": "postgres://user:pass@tsdb:5432/dbname?sslmode=disable",

			"telemetry.otel_endpoint": "otel-collector:4317",
			"telemetry.insecure":      true,

			"logging.level":    "info",
			"logging.dev_mode": false,

			"http.port":             8081,
			"http.read_timeout":     "10s",
			"http.write_timeout":    "15s",
			"http.idle_timeout":     "60s",
			"http.shutdown_timeout": "5s",
			"http.metrics_path":     "/metrics",
			"http.healthz_path":     "/healthz",
			"http.readyz_path":      "/readyz",

			"intervals": []string{"1m", "5m", "15m", "1h", "4h", "1d"},
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
	if len(c.Kafka.Brokers) == 0 || c.Kafka.GroupID == "" {
		return fmt.Errorf("kafka brokers and group_id are required")
	}
	if c.Kafka.RawTopic == "" || c.Kafka.OutputTopicPrefix == "" {
		return fmt.Errorf("kafka topics must not be empty")
	}
	if c.Redis.Addr == "" {
		return fmt.Errorf("redis.addr is required")
	}
	if c.Timescale.DSN == "" {
		return fmt.Errorf("timescaledb.dsn is required")
	}
	if c.Telemetry.OTLPEndpoint == "" {
		return fmt.Errorf("telemetry.otel_endpoint is required")
	}
	if err := validateLogLevel(c.Logging.Level); err != nil {
		return err
	}
	return validateHTTP(&c.HTTP)
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
