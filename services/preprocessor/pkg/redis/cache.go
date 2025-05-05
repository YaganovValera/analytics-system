// services/preprocessor/pkg/redis/cache.go
package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/go-redis/redis/v8"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/YaganovValera/analytics-system/services/preprocessor/pkg/logger"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	redisSetErrors = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "preprocessor", Subsystem: "redis_cache", Name: "set_errors_total",
		Help: "Количество ошибок при записи в Redis",
	})
	redisGetErrors = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "preprocessor", Subsystem: "redis_cache", Name: "get_errors_total",
		Help: "Количество ошибок при чтении из Redis",
	})
	redisGetMiss = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "preprocessor", Subsystem: "redis_cache", Name: "get_misses_total",
		Help: "Количество промахов при чтении из Redis (ключ не найден)",
	})
	redisGetHit = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "preprocessor", Subsystem: "redis_cache", Name: "get_hits_total",
		Help: "Количество успешных чтений из Redis",
	})
)

var tracer = otel.Tracer("redis-cache")

// Config дублируется из internal/config для удобства pkg.
type Config struct {
	Address  string
	Password string
	DB       int
	TTL      time.Duration
}

// NewCache создаёт подключение к Redis и пингует его.
func NewCache(cfg Config, log *logger.Logger) (Cache, error) {
	log = log.Named("redis")
	opts := &goredis.Options{
		Addr:     cfg.Address,
		Password: cfg.Password,
		DB:       cfg.DB,
	}
	client := goredis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis: ping failed: %w", err)
	}
	log.Info("redis: connected", zap.String("addr", cfg.Address), zap.Int("db", cfg.DB))
	return &cacheImpl{client: client, logger: log}, nil
}

type cacheImpl struct {
	client *goredis.Client
	logger *logger.Logger
}

// Set сохраняет value по ключу key с TTL.
func (c *cacheImpl) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	ctx, span := tracer.Start(ctx, "Redis.Set",
		trace.WithAttributes(attribute.String("key", key)))
	defer span.End()

	if err := c.client.Set(ctx, key, value, ttl).Err(); err != nil {
		redisSetErrors.Inc()
		span.RecordError(err)
		c.logger.Error("redis: set failed", zap.Error(err), zap.String("key", key))
		return err
	}
	return nil
}

// Get возвращает value по ключу key. Если ключ отсутствует, возвращает (nil, nil).
func (c *cacheImpl) Get(ctx context.Context, key string) ([]byte, error) {
	ctx, span := tracer.Start(ctx, "Redis.Get",
		trace.WithAttributes(attribute.String("key", key)))
	defer span.End()

	val, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == goredis.Nil {
			redisGetMiss.Inc()
			return nil, nil
		}
		redisGetErrors.Inc()
		span.RecordError(err)
		c.logger.Error("redis: get failed", zap.Error(err), zap.String("key", key))
		return nil, err
	}
	redisGetHit.Inc()
	return val, nil
}

// Close корректно закрывает клиент.
func (c *cacheImpl) Close() error {
	return c.client.Close()
}
