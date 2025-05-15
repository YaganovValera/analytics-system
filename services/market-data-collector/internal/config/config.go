// github.com/YaganovValera/analytics-system/services/market-data-collector/internal/config/config.go
package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/YaganovValera/analytics-system/common/backoff"
	"github.com/YaganovValera/analytics-system/common/config"
)

// Config — основная структура конфигурации.
type Config struct {
	ServiceName    string        `mapstructure:"service_name"`
	ServiceVersion string        `mapstructure:"service_version"`
	Binance        BinanceConfig `mapstructure:"binance"`
	Kafka          KafkaConfig   `mapstructure:"kafka"`
	Telemetry      Telemetry     `mapstructure:"telemetry"`
	Logging        Logging       `mapstructure:"logging"`
	HTTP           HTTPConfig    `mapstructure:"http"`
}

type BinanceConfig struct {
	WSURL            string         `mapstructure:"ws_url"`
	Symbols          []string       `mapstructure:"symbols"`
	ReadTimeout      time.Duration  `mapstructure:"read_timeout"`
	SubscribeTimeout time.Duration  `mapstructure:"subscribe_timeout"`
	Backoff          backoff.Config `mapstructure:"backoff"`
	BufferSize       int            `mapstructure:"buffer_size"`
}

type KafkaConfig struct {
	Brokers        []string       `mapstructure:"brokers"`
	RawTopic       string         `mapstructure:"raw_topic"`
	OrderBookTopic string         `mapstructure:"orderbook_topic"`
	Timeout        time.Duration  `mapstructure:"timeout"`
	Acks           string         `mapstructure:"acks"`
	Compression    string         `mapstructure:"compression"`
	FlushFrequency time.Duration  `mapstructure:"flush_frequency"`
	FlushMessages  int            `mapstructure:"flush_messages"`
	Backoff        backoff.Config `mapstructure:"backoff"`
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

// Load загружает и валидирует конфигурацию.
func Load(path string) (*Config, error) {
	var cfg Config
	err := config.Load(config.Options{
		Path:      path,
		EnvPrefix: "COLLECTOR",
		Out:       &cfg,
		Defaults: map[string]interface{}{
			"service_name":    "market-data-collector",
			"service_version": "v1.0.0",

			"binance.ws_url":            "wss://stream.binance.com:9443/ws",
			"binance.read_timeout":      "30s",
			"binance.subscribe_timeout": "5s",
			"binance.symbols":           []string{"btcusdt@trade"},
			"binance.buffer_size":       100,

			"kafka.acks":            "all",
			"kafka.timeout":         "15s",
			"kafka.compression":     "none",
			"kafka.flush_frequency": "0s",
			"kafka.flush_messages":  0,

			"telemetry.otel_endpoint": "otel-collector:4317",
			"telemetry.insecure":      false,

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
		},
		DebugOutput: false,
	})
	if err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}
	return &cfg, nil
}

// Validate проверяет значения конфига.
func (c *Config) Validate() error {
	if c.ServiceName == "" {
		return fmt.Errorf("service_name is required")
	}
	if c.ServiceVersion == "" {
		return fmt.Errorf("service_version is required")
	}
	if c.Binance.WSURL == "" {
		return fmt.Errorf("binance.ws_url is required")
	}
	if len(c.Binance.Symbols) == 0 {
		return fmt.Errorf("binance.symbols must not be empty")
	}
	if c.Binance.ReadTimeout <= 0 {
		return fmt.Errorf("binance.read_timeout must be > 0")
	}
	if c.Binance.SubscribeTimeout <= 0 {
		return fmt.Errorf("binance.subscribe_timeout must be > 0")
	}
	if len(c.Kafka.Brokers) == 0 {
		return fmt.Errorf("kafka.brokers is required")
	}
	if c.Kafka.RawTopic == "" || c.Kafka.OrderBookTopic == "" {
		return fmt.Errorf("kafka.raw_topic and kafka.orderbook_topic are required")
	}
	if !validAcks(c.Kafka.Acks) {
		return fmt.Errorf("kafka.acks must be one of [all, leader, none]")
	}
	if !validCompression(c.Kafka.Compression) {
		return fmt.Errorf("kafka.compression must be one of [none, gzip, snappy, lz4, zstd]")
	}
	if c.Telemetry.OTLPEndpoint == "" {
		return fmt.Errorf("telemetry.otel_endpoint is required")
	}
	if !validLogLevel(c.Logging.Level) {
		return fmt.Errorf("logging.level must be one of [debug, info, warn, error]")
	}
	if err := validateHTTP(&c.HTTP); err != nil {
		return err
	}
	return nil
}

func validAcks(s string) bool {
	switch strings.ToLower(s) {
	case "all", "leader", "none":
		return true
	}
	return false
}

func validCompression(s string) bool {
	switch strings.ToLower(s) {
	case "none", "gzip", "snappy", "lz4", "zstd":
		return true
	}
	return false
}

func validLogLevel(s string) bool {
	switch strings.ToLower(s) {
	case "debug", "info", "warn", "error":
		return true
	}
	return false
}

func validateHTTP(h *HTTPConfig) error {
	if h.Port <= 0 || h.Port > 65535 {
		return fmt.Errorf("http.port must be between 1 and 65535")
	}
	if h.ReadTimeout <= 0 || h.WriteTimeout <= 0 || h.IdleTimeout <= 0 || h.ShutdownTimeout <= 0 {
		return fmt.Errorf("http timeouts must be > 0")
	}
	for _, path := range []string{h.MetricsPath, h.HealthzPath, h.ReadyzPath} {
		if !strings.HasPrefix(path, "/") {
			return fmt.Errorf("http path %q must start with '/'", path)
		}
	}
	return nil
}

// Print выводит загруженный конфиг в консоль.
func (c *Config) Print() {
	b, _ := json.MarshalIndent(c, "", "  ")
	fmt.Println("Loaded configuration:\n", string(b))
}
