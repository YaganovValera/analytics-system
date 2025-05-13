// services/analytics-api/internal/config/config.go
package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

// Config хранит все настройки микросервиса Analytics API.
type Config struct {
	ServiceName    string `mapstructure:"service_name"`
	ServiceVersion string `mapstructure:"service_version"`

	HTTP      HTTPConfig      `mapstructure:"http"`
	Postgres  PostgresConfig  `mapstructure:"postgres"`
	Telemetry TelemetryConfig `mapstructure:"telemetry"`
	Logging   LoggingConfig   `mapstructure:"logging"`
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

type PostgresConfig struct {
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

type TelemetryConfig struct {
	OTLPEndpoint string  `mapstructure:"otel_endpoint"`
	Insecure     bool    `mapstructure:"insecure"`
	Sampler      float64 `mapstructure:"sampler"`
}

type LoggingConfig struct {
	Level   string `mapstructure:"level"`
	DevMode bool   `mapstructure:"dev_mode"`
}

func Load(path string) (*Config, error) {
	v := viper.New()

	// 1) defaults
	v.SetDefault("service_name", "analytics-api")
	v.SetDefault("service_version", "v1.0.0")

	// HTTP defaults
	v.SetDefault("http.port", 9090)
	v.SetDefault("http.read_timeout", "10s")
	v.SetDefault("http.write_timeout", "15s")
	v.SetDefault("http.idle_timeout", "60s")
	v.SetDefault("http.shutdown_timeout", "5s")
	v.SetDefault("http.metrics_path", "/metrics")
	v.SetDefault("http.healthz_path", "/healthz")
	v.SetDefault("http.readyz_path", "/readyz")

	// Postgres defaults
	v.SetDefault("postgres.dsn", "postgres://user:pass@timescaledb:5432/analytics?sslmode=disable")
	v.SetDefault("postgres.max_open_conns", 10)
	v.SetDefault("postgres.max_idle_conns", 5)
	v.SetDefault("postgres.conn_max_lifetime", "1h")

	// Telemetry defaults
	v.SetDefault("telemetry.otel_endpoint", "otel-collector:4317")
	v.SetDefault("telemetry.insecure", false)
	v.SetDefault("telemetry.sampler", 1.0)

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.dev_mode", false)

	// 2) ENV
	v.SetEnvPrefix("ANALYTICS_API")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// 3) File
	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config %q: %w", path, err)
		}
	}

	// 4) Decode
	var cfg Config
	decodeHook := mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		func(f, t reflect.Kind, data interface{}) (interface{}, error) {
			if f == reflect.String && t == reflect.Bool {
				return strconv.ParseBool(data.(string))
			}
			return data, nil
		},
	)
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName:    "mapstructure",
		Result:     &cfg,
		DecodeHook: decodeHook,
	})
	if err != nil {
		return nil, fmt.Errorf("create decoder: %w", err)
	}
	if err := decoder.Decode(v.AllSettings()); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	// 5) Validate
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.ServiceName == "" {
		return fmt.Errorf("service_name is required")
	}
	if c.ServiceVersion == "" {
		return fmt.Errorf("service_version is required")
	}

	// HTTP
	if c.HTTP.Port <= 0 || c.HTTP.Port > 65535 {
		return fmt.Errorf("http.port must be between 1 and 65535")
	}
	if c.HTTP.ReadTimeout <= 0 {
		return fmt.Errorf("http.read_timeout must be >0")
	}
	if c.HTTP.WriteTimeout <= 0 {
		return fmt.Errorf("http.write_timeout must be >0")
	}
	if c.HTTP.IdleTimeout <= 0 {
		return fmt.Errorf("http.idle_timeout must be >0")
	}
	if c.HTTP.ShutdownTimeout <= 0 {
		return fmt.Errorf("http.shutdown_timeout must be >0")
	}
	for key, path := range map[string]string{
		"http.metrics_path": c.HTTP.MetricsPath,
		"http.healthz_path": c.HTTP.HealthzPath,
		"http.readyz_path":  c.HTTP.ReadyzPath,
	} {
		if !strings.HasPrefix(path, "/") {
			return fmt.Errorf("%s must start with '/'", key)
		}
	}

	// Postgres
	if c.Postgres.DSN == "" {
		return fmt.Errorf("postgres.dsn is required")
	}
	if c.Postgres.MaxOpenConns <= 0 {
		return fmt.Errorf("postgres.max_open_conns must be >0")
	}
	if c.Postgres.MaxIdleConns < 0 {
		return fmt.Errorf("postgres.max_idle_conns must be >=0")
	}
	if c.Postgres.ConnMaxLifetime <= 0 {
		return fmt.Errorf("postgres.conn_max_lifetime must be >0")
	}

	// Telemetry
	if c.Telemetry.OTLPEndpoint == "" {
		return fmt.Errorf("telemetry.otel_endpoint is required")
	}
	if c.Telemetry.Sampler < 0 || c.Telemetry.Sampler > 1 {
		return fmt.Errorf("telemetry.sampler must be between 0.0 and 1.0")
	}

	// Logging
	switch strings.ToLower(c.Logging.Level) {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("logging.level must be one of [debug, info, warn, error]")
	}

	return nil
}

// Print выводит текущий конфиг в JSON.
func (c *Config) Print() {
	b, _ := json.MarshalIndent(c, "", "  ")
	fmt.Println("Loaded configuration:\n", string(b))
}
