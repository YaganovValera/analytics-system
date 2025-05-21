// services/market-data-collector/internal/config/config.go
package config

import (
	"fmt"

	commonconfig "github.com/YaganovValera/analytics-system/common/config"
	"github.com/YaganovValera/analytics-system/common/httpserver"
	"github.com/YaganovValera/analytics-system/common/kafka/producer"
	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/YaganovValera/analytics-system/common/telemetry"
	pkgBinance "github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/binance"
)

type Config struct {
	ServiceName    string            `mapstructure:"service_name"`
	ServiceVersion string            `mapstructure:"service_version"`
	Telemetry      telemetry.Config  `mapstructure:"telemetry"`
	Logging        logger.Config     `mapstructure:"logging"`
	HTTP           httpserver.Config `mapstructure:"http"`
	Binance        pkgBinance.Config `mapstructure:"binance"`
	Kafka          KafkaConfig       `mapstructure:"kafka"`
}

type KafkaConfig struct {
	producer.Config `mapstructure:",squash"`
	RawTopic        string `mapstructure:"raw_topic"`
	OrderBookTopic  string `mapstructure:"orderbook_topic"`
}

func Load(path string) (*Config, error) {
	var cfg Config
	if err := commonconfig.Load(commonconfig.Options{
		Path:      path,
		EnvPrefix: "MARKETDATA",
		Out:       &cfg,
		Defaults: map[string]interface{}{
			"service_name":    "market-data-collector",
			"service_version": "v1.0.0",

			// Logging
			"logging.level":    "info",
			"logging.dev_mode": false,

			"http.port":             8080,
			"http.read_timeout":     "10s",
			"http.write_timeout":    "15s",
			"http.idle_timeout":     "60s",
			"http.shutdown_timeout": "5s",
			"http.metrics_path":     "/metrics",
			"http.healthz_path":     "/healthz",
			"http.readyz_path":      "/readyz",

			// Telemetry
			"telemetry.endpoint":        "otel-collector:4317",
			"telemetry.insecure":        true,
			"telemetry.service_name":    "market-data-collector",
			"telemetry.service_version": "v1.0.0",

			// Kafka
			"kafka.required_acks":   "all",
			"kafka.timeout":         "15s",
			"kafka.compression":     "none",
			"kafka.flush_frequency": "0s",
			"kafka.flush_messages":  0,
			"kafka.raw_topic":       "marketdata.raw",
			"kafka.orderbook_topic": "marketdata.orderbook",

			// Binance WS
			"binance.ws_url":            "wss://stream.binance.com:9443/ws",
			"binance.buffer_size":       10000,
			"binance.read_timeout":      "30s",
			"binance.subscribe_timeout": "5s",
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

func (c *Config) ApplyDefaults() {
	c.Logging.ApplyDefaults()
	c.HTTP.ApplyDefaults()
	c.Telemetry.ApplyDefaults()
	c.Kafka.Config.ApplyDefaults()
	c.Binance.ApplyDefaults()
}

func (c *Config) Validate() error {
	if c.ServiceName == "" {
		return fmt.Errorf("service_name is required")
	}
	if c.ServiceVersion == "" {
		return fmt.Errorf("service_version is required")
	}
	if err := c.Logging.Validate(); err != nil {
		return fmt.Errorf("logging config: %w", err)
	}
	if err := c.HTTP.Validate(); err != nil {
		return fmt.Errorf("http config: %w", err)
	}
	if err := c.Telemetry.Validate(); err != nil {
		return fmt.Errorf("telemetry config: %w", err)
	}
	if err := c.Kafka.Config.Validate(); err != nil {
		return fmt.Errorf("kafka config: %w", err)
	}
	if c.Kafka.RawTopic == "" {
		return fmt.Errorf("kafka.raw_topic is required")
	}
	if c.Kafka.OrderBookTopic == "" {
		return fmt.Errorf("kafka.orderbook_topic is required")
	}
	if err := c.Binance.Validate(); err != nil {
		return fmt.Errorf("binance config: %w", err)
	}
	return nil
}
