package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/YaganovValera/analytics-system/common/backoff"
	"github.com/YaganovValera/analytics-system/common/logger"

	"github.com/go-redis/redis/v8"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var (
	redisMetrics = struct {
		GetErrors        prometheus.Counter
		SetErrors        prometheus.Counter
		DeleteErrors     prometheus.Counter
		OperationLatency prometheus.Histogram
	}{
		GetErrors: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "preprocessor", Subsystem: "redis", Name: "get_errors_total",
			Help: "Total number of errors on Redis GET",
		}),
		SetErrors: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "preprocessor", Subsystem: "redis", Name: "set_errors_total",
			Help: "Total number of errors on Redis SET",
		}),
		DeleteErrors: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "preprocessor", Subsystem: "redis", Name: "delete_errors_total",
			Help: "Total number of errors on Redis DEL",
		}),
		OperationLatency: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: "preprocessor", Subsystem: "redis", Name: "operation_latency_seconds",
			Help:    "Latency of Redis operations",
			Buckets: prometheus.DefBuckets,
		}),
	}
	tracer = otel.Tracer("redis-storage")
)

// ErrNotFound возвращается, если ключ отсутствует.
var ErrNotFound = fmt.Errorf("redis: key not found")

// Config хранит параметры подключения к Redis.
type Config struct {
	URL     string        // e.g. "redis://host:6379/0"
	TTL     time.Duration // default: 10m
	Backoff backoff.Config
}

// applyDefaults задаёт sane defaults.
func (c *Config) applyDefaults() {
	if c.TTL <= 0 {
		c.TTL = 10 * time.Minute
	}
}

// validate проверяет обязательные поля.
func (c *Config) validate() error {
	if c.URL == "" {
		return fmt.Errorf("redis: URL required")
	}
	return nil
}

// redisStorage — продакшен-реализация Storage через go-redis/v8.
type redisStorage struct {
	client     *redis.Client
	ttl        time.Duration
	log        *logger.Logger
	backoffCfg backoff.Config
}

// New создает Storage, соединяется с Redis, с retry и метриками.
func New(ctx context.Context, cfg Config, log *logger.Logger) (Storage, error) {
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	log = log.Named("redis")

	// парсим URL
	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("redis: parse URL: %w", err)
	}
	client := redis.NewClient(opts)

	// Проверяем соединение с retry
	op := func(ctx context.Context) error { return client.Ping(ctx).Err() }
	ctxConn, span := tracer.Start(ctx, "Connect", trace.WithAttributes(attribute.String("url", cfg.URL)))
	if err := backoff.Execute(ctxConn, cfg.Backoff, log, op); err != nil {
		span.RecordError(err)
		span.End()
		return nil, fmt.Errorf("redis connect: %w", err)
	}
	span.End()
	log.Info("redis: connected", zap.String("url", cfg.URL))

	return &redisStorage{
		client:     client,
		ttl:        cfg.TTL,
		log:        log,
		backoffCfg: cfg.Backoff,
	}, nil
}

func (r *redisStorage) Get(ctx context.Context, key string) ([]byte, error) {
	ctxOp, span := tracer.Start(ctx, "Get", trace.WithAttributes(attribute.String("key", key)))
	defer span.End()

	start := time.Now()

	var data []byte
	op := func(ctx context.Context) error {
		val, err := r.client.Get(ctx, key).Bytes()
		if err == redis.Nil {
			return backoff.Permanent(ErrNotFound)
		}
		if err != nil {
			return err
		}
		data = val
		return nil
	}
	if err := backoff.Execute(ctxOp, r.backoffCfg, r.log, op); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrNotFound
		}
		redisMetrics.GetErrors.Inc()
		r.log.WithContext(ctx).Error("redis GET failed", zap.String("key", key), zap.Error(err))
		span.RecordError(err)
		return nil, err
	}
	redisMetrics.OperationLatency.Observe(time.Since(start).Seconds())
	return data, nil
}

func (r *redisStorage) Set(ctx context.Context, key string, value []byte) error {
	ctxOp, span := tracer.Start(ctx, "Set", trace.WithAttributes(attribute.String("key", key)))
	defer span.End()

	start := time.Now()
	op := func(ctx context.Context) error {
		return r.client.Set(ctx, key, value, r.ttl).Err()
	}
	if err := backoff.Execute(ctxOp, r.backoffCfg, r.log, op); err != nil {
		redisMetrics.SetErrors.Inc()
		r.log.WithContext(ctx).Error("redis SET failed", zap.String("key", key), zap.Error(err))
		span.RecordError(err)
		return err
	}
	redisMetrics.OperationLatency.Observe(time.Since(start).Seconds())
	return nil
}

func (r *redisStorage) Delete(ctx context.Context, key string) error {
	ctxOp, span := tracer.Start(ctx, "Delete", trace.WithAttributes(attribute.String("key", key)))
	defer span.End()

	start := time.Now()
	op := func(ctx context.Context) error {
		_, err := r.client.Del(ctx, key).Result()
		return err
	}
	if err := backoff.Execute(ctxOp, r.backoffCfg, r.log, op); err != nil {
		redisMetrics.DeleteErrors.Inc()
		r.log.WithContext(ctx).Error("redis DEL failed", zap.String("key", key), zap.Error(err))
		span.RecordError(err)
		return err
	}
	redisMetrics.OperationLatency.Observe(time.Since(start).Seconds())
	return nil
}

func (r *redisStorage) Close() error {
	return r.client.Close()
}
