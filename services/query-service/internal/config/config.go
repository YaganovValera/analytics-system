// query-service/internal/config/config.go
package config

import (
	"fmt"

	commoncfg "github.com/YaganovValera/analytics-system/common/config"
	commonhttp "github.com/YaganovValera/analytics-system/common/httpserver"
	commonlogger "github.com/YaganovValera/analytics-system/common/logger"
	commontelemetry "github.com/YaganovValera/analytics-system/common/telemetry"
	"github.com/YaganovValera/analytics-system/services/query-service/internal/storage/timescaledb"
)

type Config struct {
	ServiceName    string `mapstructure:"service_name"`
	ServiceVersion string `mapstructure:"service_version"`

	Logging   commonlogger.Config    `mapstructure:"logging"`
	Telemetry commontelemetry.Config `mapstructure:"telemetry"`
	HTTP      commonhttp.Config      `mapstructure:"http"`

	Timescale timescaledb.Config `mapstructure:"timescaledb"`
}

func Load(path string) (*Config, error) {
	var cfg Config
	if err := commoncfg.Load(commoncfg.Options{
		Path:      path,
		EnvPrefix: "QUERY",
		Out:       &cfg,
		Defaults: map[string]interface{}{
			"service_name":     "query-service",
			"service_version":  "v1.0.0",
			"logging.level":    "info",
			"logging.dev_mode": false,

			"telemetry.endpoint":         "otel-collector:4317",
			"telemetry.insecure":         true,
			"telemetry.reconnect_period": "5s",
			"telemetry.timeout":          "5s",
			"telemetry.sampler_ratio":    1.0,

			"http.port":             8087,
			"http.read_timeout":     "10s",
			"http.write_timeout":    "15s",
			"http.idle_timeout":     "60s",
			"http.shutdown_timeout": "5s",
			"http.metrics_path":     "/metrics",
			"http.healthz_path":     "/healthz",
			"http.readyz_path":      "/readyz",

			"timescaledb.dsn": "postgres://user:pass@timescaledb:5432/analytics?sslmode=disable",
		},
	}); err != nil {
		return nil, fmt.Errorf("config load failed: %w", err)
	}

	cfg.Logging.ApplyDefaults()
	cfg.Telemetry.ApplyDefaults()
	cfg.HTTP.ApplyDefaults()

	if err := cfg.Logging.Validate(); err != nil {
		return nil, fmt.Errorf("logging config invalid: %w", err)
	}
	if err := cfg.Telemetry.Validate(); err != nil {
		return nil, fmt.Errorf("telemetry config invalid: %w", err)
	}
	if err := cfg.HTTP.Validate(); err != nil {
		return nil, fmt.Errorf("http config invalid: %w", err)
	}
	if err := cfg.Timescale.Validate(); err != nil {
		return nil, fmt.Errorf("timescaledb config invalid: %w", err)
	}

	return &cfg, nil
}
