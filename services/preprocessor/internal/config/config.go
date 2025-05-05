package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"

	"github.com/YaganovValera/analytics-system/services/preprocessor/pkg/backoff"
)

// Config — корневой конфиг для preprocessor.
type Config struct {
	ServiceName    string      `mapstructure:"service_name"`
	ServiceVersion string      `mapstructure:"service_version"`
	Kafka          KafkaConfig `mapstructure:"kafka"`
	Redis          RedisConfig `mapstructure:"redis"`
	Telemetry      Telemetry   `mapstructure:"telemetry"`
	Logging        Logging     `mapstructure:"logging"`
	HTTP           HTTPConfig  `mapstructure:"http"`
}

// KafkaConfig хранит настройки Kafka.
type KafkaConfig struct {
	Brokers           []string           `mapstructure:"brokers"`
	RawTopic          string             `mapstructure:"raw_topic"`
	OrderBookTopic    string             `mapstructure:"orderbook_topic"`
	AggregationTopics []AggregationTopic `mapstructure:"aggregation_topics"`
	GroupID           string             `mapstructure:"group_id"`
	Backoff           backoff.Config     `mapstructure:"backoff"`
}

// AggregationTopic определяет, куда отправлять по интервалу.
type AggregationTopic struct {
	Interval string `mapstructure:"interval"`
	Topic    string `mapstructure:"topic"`
}

// RedisConfig хранит параметры Redis-кэша.
type RedisConfig struct {
	Address  string        `mapstructure:"address"`
	Password string        `mapstructure:"password"`
	DB       int           `mapstructure:"db"`
	TTL      time.Duration `mapstructure:"ttl"`
}

// Telemetry и Logging — как обычно.
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

// Load загружает конфиг из файла, ENV и дефолтов, затем валидирует.
func Load(path string) (*Config, error) {
	v := viper.New()

	// Дефолты
	v.SetDefault("service_name", "preprocessor")
	v.SetDefault("service_version", "v1.0.0")

	v.SetDefault("kafka.brokers", []string{"kafka:9092"})
	v.SetDefault("kafka.group_id", "preprocessor-group")
	v.SetDefault("redis.address", "redis:6379")
	v.SetDefault("redis.db", 0)
	v.SetDefault("redis.ttl", "5m")

	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.dev_mode", false)
	v.SetDefault("http.port", 8090)

	// ENV
	v.SetEnvPrefix("PRE")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Файл
	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config %q: %w", path, err)
		}
	}

	// Decode с хуками (длительности, слайсы, bool)
	var cfg Config
	hook := mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
	)
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName:    "mapstructure",
		Result:     &cfg,
		DecodeHook: hook,
	})
	if err != nil {
		return nil, fmt.Errorf("decoder init: %w", err)
	}
	if err := decoder.Decode(v.AllSettings()); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	// Валидация
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	return &cfg, nil
}

// Validate проверяет обязательные поля.
func (c *Config) Validate() error {
	if c.ServiceName == "" {
		return fmt.Errorf("service_name is required")
	}
	if c.ServiceVersion == "" {
		return fmt.Errorf("service_version is required")
	}

	// Kafka
	if len(c.Kafka.Brokers) == 0 {
		return fmt.Errorf("kafka.brokers is required")
	}
	if c.Kafka.RawTopic == "" || c.Kafka.OrderBookTopic == "" {
		return fmt.Errorf("kafka.raw_topic and kafka.orderbook_topic are required")
	}
	if len(c.Kafka.AggregationTopics) == 0 {
		return fmt.Errorf("kafka.aggregation_topics must not be empty")
	}
	for _, at := range c.Kafka.AggregationTopics {
		if at.Interval == "" || at.Topic == "" {
			return fmt.Errorf("each aggregation_topics entry must have interval and topic")
		}
	}
	if c.Kafka.GroupID == "" {
		return fmt.Errorf("kafka.group_id is required")
	}

	// Redis
	if c.Redis.Address == "" {
		return fmt.Errorf("redis.address is required")
	}
	if c.Redis.TTL <= 0 {
		return fmt.Errorf("redis.ttl must be > 0")
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

	// HTTP
	if c.HTTP.Port < 1 || c.HTTP.Port > 65535 {
		return fmt.Errorf("http.port must be between 1 and 65535")
	}
	return nil
}

// Print выводит конфиг в JSON.
func (c *Config) Print() {
	if b, err := json.MarshalIndent(c, "", "  "); err == nil {
		fmt.Println("Loaded configuration:\n", string(b))
	}
}
