// services/preprocessor/internal/config/config.go
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

	"github.com/YaganovValera/analytics-system/common/backoff"
	commonpb "github.com/YaganovValera/analytics-system/proto/v1/common"
)

// -----------------------------------------------------------------------------
// Структуры
// -----------------------------------------------------------------------------

type Config struct {
	ServiceName    string `mapstructure:"service_name"`
	ServiceVersion string `mapstructure:"service_version"`

	Binance   BinanceConfig   `mapstructure:"binance"`
	Kafka     KafkaConfig     `mapstructure:"kafka"`
	Redis     RedisConfig     `mapstructure:"redis"`
	Telemetry TelemetryConfig `mapstructure:"telemetry"`
	Logging   LoggingConfig   `mapstructure:"logging"`
	HTTP      HTTPConfig      `mapstructure:"http"`

	Processor ProcessorConfig `mapstructure:"processor"`
}

type ProcessorConfig struct {
	Intervals []commonpb.AggregationInterval `mapstructure:"intervals"`
}

type BinanceConfig struct {
	RawTopic string         `mapstructure:"raw_topic"`
	Symbols  []string       `mapstructure:"symbols"`
	Backoff  backoff.Config `mapstructure:"backoff"`
}

type KafkaConfig struct {
	Brokers      []string       `mapstructure:"brokers"`
	CandlesTopic string         `mapstructure:"candles_topic"`
	Timeout      time.Duration  `mapstructure:"timeout"`
	Acks         string         `mapstructure:"acks"`
	Compression  string         `mapstructure:"compression"`
	Backoff      backoff.Config `mapstructure:"backoff"`
}

type RedisConfig struct {
	URL     string         `mapstructure:"url"`
	TTL     time.Duration  `mapstructure:"ttl"`
	Backoff backoff.Config `mapstructure:"backoff"`
}

type TelemetryConfig struct {
	OTLPEndpoint string `mapstructure:"otel_endpoint"`
	Insecure     bool   `mapstructure:"insecure"`
}

type LoggingConfig struct {
	Level   string `mapstructure:"level"`
	DevMode bool   `mapstructure:"dev_mode"`
}

// --- HTTP ---

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

// -----------------------------------------------------------------------------
// Load
// -----------------------------------------------------------------------------

func Load(path string) (*Config, error) {
	v := viper.New()

	/* ---------- 1) defaults ---------- */

	v.SetDefault("service_name", "preprocessor")
	v.SetDefault("service_version", "v1.0.0")

	// Binance
	v.SetDefault("binance.raw_topic", "marketdata.raw")
	v.SetDefault("binance.symbols", []string{"btcusdt@trade"})

	// Kafka
	v.SetDefault("kafka.candles_topic", "marketdata.candles")
	v.SetDefault("kafka.timeout", "15s")
	v.SetDefault("kafka.acks", "all")
	v.SetDefault("kafka.compression", "none")

	// Redis
	v.SetDefault("redis.ttl", "10m")

	// Telemetry
	v.SetDefault("telemetry.otel_endpoint", "otel-collector:4317")
	v.SetDefault("telemetry.insecure", false)

	// Logging
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.dev_mode", false)

	// HTTP (полный набор)
	v.SetDefault("http.port", 8090)
	v.SetDefault("http.read_timeout", "10s")
	v.SetDefault("http.write_timeout", "15s")
	v.SetDefault("http.idle_timeout", "60s")
	v.SetDefault("http.shutdown_timeout", "5s")
	v.SetDefault("http.metrics_path", "/metrics")
	v.SetDefault("http.healthz_path", "/healthz")
	v.SetDefault("http.readyz_path", "/readyz")

	// Processor intervals (по умолчанию — все поддерживаемые)
	v.SetDefault("processor.intervals", []commonpb.AggregationInterval{
		commonpb.AggregationInterval_AGG_INTERVAL_1_MINUTE,
		commonpb.AggregationInterval_AGG_INTERVAL_5_MINUTES,
		commonpb.AggregationInterval_AGG_INTERVAL_15_MINUTES,
		commonpb.AggregationInterval_AGG_INTERVAL_1_HOUR,
		commonpb.AggregationInterval_AGG_INTERVAL_4_HOURS,
		commonpb.AggregationInterval_AGG_INTERVAL_1_DAY,
	})

	/* ---------- 2) env ---------- */

	v.SetEnvPrefix("PREPROCESSOR")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	/* ---------- 3) optional file ---------- */

	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config %q: %w", path, err)
		}
	}

	/* ---------- 4) decode ---------- */

	var cfg Config
	decodeHook := mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
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
		return nil, fmt.Errorf("create config decoder: %w", err)
	}
	if err := decoder.Decode(v.AllSettings()); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	/* ---------- 5) validate ---------- */

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	return &cfg, nil
}

// -----------------------------------------------------------------------------
// Validation helpers
// -----------------------------------------------------------------------------

func (c *Config) Validate() error {
	// service
	if c.ServiceName == "" {
		return fmt.Errorf("service_name is required")
	}
	if c.ServiceVersion == "" {
		return fmt.Errorf("service_version is required")
	}

	// binance
	if c.Binance.RawTopic == "" {
		return fmt.Errorf("binance.raw_topic is required")
	}
	if len(c.Binance.Symbols) == 0 {
		return fmt.Errorf("binance.symbols must contain at least one entry")
	}

	// kafka
	if len(c.Kafka.Brokers) == 0 {
		return fmt.Errorf("kafka.brokers is required")
	}
	if c.Kafka.CandlesTopic == "" {
		return fmt.Errorf("kafka.candles_topic is required")
	}
	if c.Kafka.Timeout <= 0 {
		return fmt.Errorf("kafka.timeout must be > 0")
	}
	switch strings.ToLower(c.Kafka.Acks) {
	case "all", "leader", "none":
	default:
		return fmt.Errorf("kafka.acks must be one of [all, leader, none]")
	}
	switch strings.ToLower(c.Kafka.Compression) {
	case "none", "gzip", "snappy", "lz4", "zstd":
	default:
		return fmt.Errorf("kafka.compression must be one of [none, gzip, snappy, lz4, zstd]")
	}

	// redis
	if c.Redis.URL == "" {
		return fmt.Errorf("redis.url is required")
	}
	if c.Redis.TTL <= 0 {
		return fmt.Errorf("redis.ttl must be > 0")
	}

	// telemetry
	if c.Telemetry.OTLPEndpoint == "" {
		return fmt.Errorf("telemetry.otel_endpoint is required")
	}

	// logging
	switch strings.ToLower(c.Logging.Level) {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("logging.level must be one of [debug, info, warn, error]")
	}

	// http
	if err := validateHTTP(&c.HTTP); err != nil {
		return err
	}

	// processor.intervals
	if len(c.Processor.Intervals) == 0 {
		return fmt.Errorf("processor.intervals must contain at least one aggregation interval")
	}
	for _, iv := range c.Processor.Intervals {
		if !isSupportedInterval(iv) {
			return fmt.Errorf("unsupported aggregation interval: %v", iv)
		}
	}

	return nil
}

func validateHTTP(h *HTTPConfig) error {
	if h.Port <= 0 || h.Port > 65535 {
		return fmt.Errorf("http.port must be between 1 and 65535")
	}
	durations := map[string]time.Duration{
		"http.read_timeout":     h.ReadTimeout,
		"http.write_timeout":    h.WriteTimeout,
		"http.idle_timeout":     h.IdleTimeout,
		"http.shutdown_timeout": h.ShutdownTimeout,
	}
	for k, d := range durations {
		if d <= 0 {
			return fmt.Errorf("%s must be > 0", k)
		}
	}
	paths := map[string]string{
		"http.metrics_path": h.MetricsPath,
		"http.healthz_path": h.HealthzPath,
		"http.readyz_path":  h.ReadyzPath,
	}
	for k, p := range paths {
		if !strings.HasPrefix(p, "/") {
			return fmt.Errorf("%s must start with '/'", k)
		}
	}
	return nil
}

func isSupportedInterval(iv commonpb.AggregationInterval) bool {
	switch iv {
	case commonpb.AggregationInterval_AGG_INTERVAL_1_MINUTE,
		commonpb.AggregationInterval_AGG_INTERVAL_5_MINUTES,
		commonpb.AggregationInterval_AGG_INTERVAL_15_MINUTES,
		commonpb.AggregationInterval_AGG_INTERVAL_1_HOUR,
		commonpb.AggregationInterval_AGG_INTERVAL_4_HOURS,
		commonpb.AggregationInterval_AGG_INTERVAL_1_DAY:
		return true
	default:
		return false
	}
}

// -----------------------------------------------------------------------------
// Debug print
// -----------------------------------------------------------------------------

func (c *Config) Print() {
	b, _ := json.MarshalIndent(c, "", "  ")
	fmt.Println("Loaded configuration:\n", string(b))
}
