package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/backoff"
)

// Config — все параметры сервиса.
type Config struct {
	// Метаданные сервиса для TracerProvider
	ServiceName    string `mapstructure:"service_name"`
	ServiceVersion string `mapstructure:"service_version"`

	Binance struct {
		WSURL       string         `mapstructure:"ws_url"`
		Symbols     []string       `mapstructure:"symbols"`
		ReadTimeout time.Duration  `mapstructure:"read_timeout"`
		Backoff     backoff.Config `mapstructure:"backoff"`
	} `mapstructure:"binance"`

	Kafka struct {
		Brokers []string `mapstructure:"brokers"`
		Topics  struct {
			Raw       string `mapstructure:"raw"`
			OrderBook string `mapstructure:"orderbook"`
		} `mapstructure:"topics"`
		Timeout        time.Duration  `mapstructure:"timeout"`
		Acks           string         `mapstructure:"acks"`
		Compression    string         `mapstructure:"compression"`
		FlushFrequency time.Duration  `mapstructure:"flush_frequency"`
		FlushMessages  int            `mapstructure:"flush_messages"`
		Backoff        backoff.Config `mapstructure:"backoff"`
	} `mapstructure:"kafka"`

	Telemetry struct {
		OTLPEndpoint string `mapstructure:"otel_endpoint"`
		Insecure     bool   `mapstructure:"insecure"`
	} `mapstructure:"telemetry"`

	Logging struct {
		Level   string `mapstructure:"level"`
		DevMode bool   `mapstructure:"dev_mode"`
	} `mapstructure:"logging"`

	Buffer struct {
		Size int `mapstructure:"size"`
	} `mapstructure:"buffer"`

	HTTP struct {
		HealthPort int `mapstructure:"health_port"`
	} `mapstructure:"http"`
}

// LoadConfig загружает конфиг из файла (по --config), ENV и флагов.
func LoadConfig(defaultPath string) (*Config, error) {
	// 1) Дефолты корня и сервисных метаданных
	viper.SetDefault("service_name", "market-data-collector")
	viper.SetDefault("service_version", "v1.0.0")

	// 2) Дефолты Binance, Kafka, Telemetry, Logging, Buffer, HTTP
	viper.SetDefault("binance.ws_url", "wss://stream.binance.com:9443/ws")
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.dev_mode", false)
	viper.SetDefault("buffer.size", 10000)
	viper.SetDefault("http.health_port", 8080)
	viper.SetDefault("kafka.timeout", "15s")
	viper.SetDefault("kafka.acks", "all")
	viper.SetDefault("kafka.compression", "none")
	viper.SetDefault("kafka.flush_frequency", "0s")
	viper.SetDefault("kafka.flush_messages", 0)
	viper.SetDefault("telemetry.insecure", false)

	// 3) Дефолты для backoff в обоих разделах
	for _, prefix := range []string{"binance.backoff", "kafka.backoff"} {
		viper.SetDefault(prefix+".initial_interval", "1s")
		viper.SetDefault(prefix+".randomization_factor", 0.5)
		viper.SetDefault(prefix+".multiplier", 2.0)
		viper.SetDefault(prefix+".max_interval", "30s")
		viper.SetDefault(prefix+".max_elapsed_time", "5m")
		viper.SetDefault(prefix+".per_attempt_timeout", "0s")
	}

	// 4) Флаг --config
	pflag.String("config", defaultPath, "path to config file")
	pflag.Parse()
	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		return nil, fmt.Errorf("bind flags: %w", err)
	}

	// 5) ENV
	viper.SetEnvPrefix("COLLECTOR")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// 6) Чтение файла
	cfgFile := viper.GetString("config")
	if err := viper.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, fmt.Errorf("read config %q: %w", cfgFile, err)
		}
	}

	// 7) Декодирование
	var cfg Config
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
		),
		TagName: "mapstructure",
		Result:  &cfg,
	})
	if err != nil {
		return nil, fmt.Errorf("create decoder: %w", err)
	}
	if err := decoder.Decode(viper.AllSettings()); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	// 8) Валидация
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	return &cfg, nil
}

// Validate проверяет наличие и корректность полей.
func (c *Config) Validate() error {
	// Service metadata
	if c.ServiceName == "" {
		return fmt.Errorf("service_name is required")
	}
	if c.ServiceVersion == "" {
		return fmt.Errorf("service_version is required")
	}
	// Binance
	if c.Binance.WSURL == "" {
		return fmt.Errorf("binance.ws_url is required")
	}
	if len(c.Binance.Symbols) == 0 {
		return fmt.Errorf("binance.symbols is required")
	}
	// Kafka
	if len(c.Kafka.Brokers) == 0 {
		return fmt.Errorf("kafka.brokers is required")
	}
	if c.Kafka.Topics.Raw == "" {
		return fmt.Errorf("kafka.topics.raw is required")
	}
	acks := strings.ToLower(c.Kafka.Acks)
	if acks != "all" && acks != "leader" && acks != "none" {
		return fmt.Errorf("kafka.acks must be one of [all, leader, none]")
	}
	// Compression
	comp := strings.ToLower(c.Kafka.Compression)
	if comp != "none" && comp != "gzip" && comp != "snappy" && comp != "lz4" && comp != "zstd" {
		return fmt.Errorf("kafka.compression must be one of [none, gzip, snappy, lz4, zstd]")
	}
	// Telemetry
	if c.Telemetry.OTLPEndpoint == "" {
		return fmt.Errorf("telemetry.otel_endpoint is required")
	}
	// Logging
	lvl := strings.ToLower(c.Logging.Level)
	switch lvl {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("logging.level must be one of [debug, info, warn, error]")
	}
	// Buffer
	if c.Buffer.Size <= 0 {
		return fmt.Errorf("buffer.size must be > 0")
	}
	// HTTP
	if c.HTTP.HealthPort <= 0 || c.HTTP.HealthPort > 65535 {
		return fmt.Errorf("http.health_port must be a valid port > 0")
	}
	return nil
}

// Print выводит текущий конфиг в JSON.
func (c *Config) Print() error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	fmt.Println("Loaded configuration:\n", string(b))
	return nil
}
