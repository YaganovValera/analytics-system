// services/market-data-collector/internal/config/config.go
package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/backoff"
)

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
	WSURL       string         `mapstructure:"ws_url"`
	Symbols     []string       `mapstructure:"symbols"`
	ReadTimeout time.Duration  `mapstructure:"read_timeout"`
	Backoff     backoff.Config `mapstructure:"backoff"`
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
	Port int `mapstructure:"port"`
}

// Load reads defaults, environment variables (COLLECTOR_*),
// an optional config file at path, then decodes into Config and validates.
func Load(path string) (*Config, error) {
	v := viper.New()

	// 1) Defaults
	v.SetDefault("service_name", "market-data-collector")
	v.SetDefault("service_version", "v1.0.0")

	v.SetDefault("binance.ws_url", "wss://stream.binance.com:9443/ws")
	v.SetDefault("binance.read_timeout", "30s")
	v.SetDefault("binance.symbols", []string{"btcusdt@trade"})

	v.SetDefault("kafka.acks", "all")
	v.SetDefault("kafka.timeout", "15s")
	v.SetDefault("kafka.compression", "none")
	v.SetDefault("kafka.flush_frequency", "0s")
	v.SetDefault("kafka.flush_messages", 0)

	v.SetDefault("telemetry.insecure", false)
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.dev_mode", false)

	v.SetDefault("http.port", 8080)

	// 2) Environment
	v.SetEnvPrefix("COLLECTOR")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// 3) Config file
	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config %q: %w", path, err)
		}
	}

	// 4) Decode into struct
	var cfg Config
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName: "mapstructure",
		Result:  &cfg,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		),
	})
	if err != nil {
		return nil, fmt.Errorf("create decoder: %w", err)
	}
	if err := decoder.Decode(v.AllSettings()); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	// 5) Validation
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	return &cfg, nil
}

// Validate ensures all required fields are set and correct.
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
		return fmt.Errorf("binance.symbols must contain at least one item")
	}

	if len(c.Kafka.Brokers) == 0 {
		return fmt.Errorf("kafka.brokers is required")
	}
	if c.Kafka.RawTopic == "" || c.Kafka.OrderBookTopic == "" {
		return fmt.Errorf("kafka.raw_topic and kafka.orderbook_topic are required")
	}
	acks := strings.ToLower(c.Kafka.Acks)
	switch acks {
	case "all", "leader", "none":
	default:
		return fmt.Errorf("kafka.acks must be one of [all, leader, none]")
	}
	comp := strings.ToLower(c.Kafka.Compression)
	switch comp {
	case "none", "gzip", "snappy", "lz4", "zstd":
	default:
		return fmt.Errorf("kafka.compression must be one of [none, gzip, snappy, lz4, zstd]")
	}

	if c.Telemetry.OTLPEndpoint == "" {
		return fmt.Errorf("telemetry.otel_endpoint is required")
	}

	lvl := strings.ToLower(c.Logging.Level)
	switch lvl {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("logging.level must be one of [debug, info, warn, error]")
	}

	if c.HTTP.Port <= 0 || c.HTTP.Port > 65535 {
		return fmt.Errorf("http.port must be between 1 and 65535")
	}
	return nil
}

// Print writes the loaded configuration as JSON.
func (c *Config) Print() {
	b, _ := json.MarshalIndent(c, "", "  ")
	fmt.Println("Loaded configuration:\n", string(b))
}
