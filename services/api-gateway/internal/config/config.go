// api-gateway/internal/config/config.go

package config

import (
	"fmt"

	commoncfg "github.com/YaganovValera/analytics-system/common/config"
	httpcfg "github.com/YaganovValera/analytics-system/common/httpserver"
	logcfg "github.com/YaganovValera/analytics-system/common/logger"
	telcfg "github.com/YaganovValera/analytics-system/common/telemetry"
)

// Config определяет структуру конфигурации API Gateway.
type Config struct {
	ServiceName    string         `mapstructure:"service_name"`
	ServiceVersion string         `mapstructure:"service_version"`
	Logging        logcfg.Config  `mapstructure:"logging"`
	HTTP           httpcfg.Config `mapstructure:"http"`
	Telemetry      telcfg.Config  `mapstructure:"telemetry"`
}

// Load читает и валидирует конфигурацию из YAML + ENV.
func Load(path string) (*Config, error) {
	var cfg Config
	if err := commoncfg.Load(commoncfg.Options{
		Path:      path,
		EnvPrefix: "APIGW",
		Out:       &cfg,
		Defaults: map[string]interface{}{
			"service_name":    "api-gateway",
			"service_version": "v1.0.0",

			// Logging
			"logging.level":    "info",
			"logging.dev_mode": false,
			"logging.format":   "console",

			// HTTP
			"http.port":             8080,
			"http.read_timeout":     "10s",
			"http.write_timeout":    "15s",
			"http.idle_timeout":     "60s",
			"http.shutdown_timeout": "5s",
			"http.metrics_path":     "/metrics",
			"http.healthz_path":     "/healthz",
			"http.readyz_path":      "/readyz",

			// Telemetry
			"telemetry.endpoint":         "otel-collector:4317",
			"telemetry.insecure":         true,
			"telemetry.reconnect_period": "5s",
			"telemetry.timeout":          "5s",
			"telemetry.sampler_ratio":    1.0,
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
