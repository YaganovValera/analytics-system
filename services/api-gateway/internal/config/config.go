// api-gateway/internal/config/config.go
package config

import (
	"fmt"

	commoncfg "github.com/YaganovValera/analytics-system/common/config"
	commonhttp "github.com/YaganovValera/analytics-system/common/httpserver"
	commonlogger "github.com/YaganovValera/analytics-system/common/logger"
	commontel "github.com/YaganovValera/analytics-system/common/telemetry"
)

// Config описывает параметры запуска api-gateway.
type Config struct {
	ServiceName    string              `mapstructure:"service_name"`
	ServiceVersion string              `mapstructure:"service_version"`
	Logging        commonlogger.Config `mapstructure:"logging"`
	HTTP           commonhttp.Config   `mapstructure:"http"`
	Telemetry      commontel.Config    `mapstructure:"telemetry"`
}

func Load(path string) (*Config, error) {
	var cfg Config
	if err := commoncfg.Load(commoncfg.Options{
		Path:      path,
		EnvPrefix: "APIGW",
		Out:       &cfg,
		Defaults: map[string]interface{}{
			"service_name":    "api-gateway",
			"service_version": "v1.0.0",

			"logging.level":    "info",
			"logging.dev_mode": false,
			"logging.format":   "console",

			"http.port":             8080,
			"http.read_timeout":     "10s",
			"http.write_timeout":    "15s",
			"http.idle_timeout":     "60s",
			"http.shutdown_timeout": "5s",
			"http.metrics_path":     "/metrics",
			"http.healthz_path":     "/healthz",
			"http.readyz_path":      "/readyz",

			"telemetry.endpoint":         "otel-collector:4317",
			"telemetry.insecure":         true,
			"telemetry.reconnect_period": "5s",
			"telemetry.timeout":          "5s",
			"telemetry.sampler_ratio":    1.0,
			"telemetry.service_name":     "market-data-collector",
			"telemetry.service_version":  "v1.0.0",
		},
	}); err != nil {
		return nil, fmt.Errorf("config load failed: %w", err)
	}

	cfg.Logging.ApplyDefaults()
	cfg.Telemetry.ApplyDefaults()
	cfg.HTTP.ApplyDefaults()

	if cfg.ServiceName == "" || cfg.ServiceVersion == "" {
		return nil, fmt.Errorf("service name/version is required")
	}
	if err := cfg.Logging.Validate(); err != nil {
		return nil, fmt.Errorf("logging config invalid: %w", err)
	}
	if err := cfg.HTTP.Validate(); err != nil {
		return nil, fmt.Errorf("http config invalid: %w", err)
	}
	if err := cfg.Telemetry.Validate(); err != nil {
		return nil, fmt.Errorf("telemetry config invalid: %w", err)
	}

	return &cfg, nil
}
