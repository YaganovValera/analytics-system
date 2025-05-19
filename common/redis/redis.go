// common/redis/redis.go
package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/YaganovValera/analytics-system/common/backoff"
	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/YaganovValera/analytics-system/common/serviceid"
	"github.com/go-redis/redis/v8"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type client struct {
	cfg    Config
	rdb    *redis.Client
	tracer trace.Tracer
	log    *logger.Logger
	svc    string
}

// New creates a Redis client and pings the server.
func New(cfg Config, log *logger.Logger) (Client, error) {
	// Init service name for metrics/tracing
	serviceid.InitServiceName(cfg.ServiceName)

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// quick ping with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return &client{
		cfg:    cfg,
		rdb:    rdb,
		tracer: otel.Tracer("common/redis"),
		log:    log.Named("redis"),
		svc:    cfg.ServiceName,
	}, nil
}

func (c *client) Get(ctx context.Context, key string) (string, error) {
	ctx, span := c.tracer.Start(ctx, "Redis.Get", trace.WithAttributes(attribute.String("redis.key", key)))
	defer span.End()

	var val string
	op := func(ctx context.Context) error {
		v, err := c.rdb.Get(ctx, key).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				return backoff.Permanent(err)
			}
			return err
		}
		val = v
		return nil
	}
	notify := func(ctx context.Context, err error, delay time.Duration, attempt int) {
		c.log.WithContext(ctx).Warn("redis GET retry",
			zap.String("key", key),
			zap.Int("attempt", attempt),
			zap.Duration("delay", delay),
			zap.Error(err),
		)
		fails.WithLabelValues(c.svc).Inc()
	}

	if err := backoff.Execute(ctx, c.cfg.Backoff, op, notify); err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		fails.WithLabelValues(c.svc).Inc()
		span.RecordError(err)
		return "", fmt.Errorf("redis GET failed: %w", err)
	}
	hits.WithLabelValues(c.svc).Inc()
	return val, nil
}

func (c *client) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	ctx, span := c.tracer.Start(ctx, "Redis.Set", trace.WithAttributes(attribute.String("redis.key", key)))
	defer span.End()

	op := func(ctx context.Context) error {
		return c.rdb.Set(ctx, key, value, ttl).Err()
	}
	notify := func(ctx context.Context, err error, delay time.Duration, attempt int) {
		c.log.WithContext(ctx).Warn("redis SET retry",
			zap.String("key", key),
			zap.Int("attempt", attempt),
			zap.Duration("delay", delay),
			zap.Error(err),
		)
		fails.WithLabelValues(c.svc).Inc()
	}

	if err := backoff.Execute(ctx, c.cfg.Backoff, op, notify); err != nil {
		fails.WithLabelValues(c.svc).Inc()
		span.RecordError(err)
		return fmt.Errorf("redis SET failed: %w", err)
	}
	hits.WithLabelValues(c.svc).Inc()
	return nil
}

func (c *client) Del(ctx context.Context, keys ...string) (int64, error) {
	ctx, span := c.tracer.Start(ctx, "Redis.Del", trace.WithAttributes(attribute.StringSlice("redis.keys", keys)))
	defer span.End()

	var n int64
	op := func(ctx context.Context) error {
		cnt, err := c.rdb.Del(ctx, keys...).Result()
		if err != nil {
			return err
		}
		n = cnt
		return nil
	}
	notify := func(ctx context.Context, err error, delay time.Duration, attempt int) {
		c.log.WithContext(ctx).Warn("redis DEL retry",
			zap.Strings("keys", keys),
			zap.Int("attempt", attempt),
			zap.Duration("delay", delay),
			zap.Error(err),
		)
		fails.WithLabelValues(c.svc).Inc()
	}

	if err := backoff.Execute(ctx, c.cfg.Backoff, op, notify); err != nil {
		fails.WithLabelValues(c.svc).Inc()
		span.RecordError(err)
		return 0, fmt.Errorf("redis DEL failed: %w", err)
	}
	hits.WithLabelValues(c.svc).Inc()
	return n, nil
}

func (c *client) Exists(ctx context.Context, key string) (bool, error) {
	ctx, span := c.tracer.Start(ctx, "Redis.Exists", trace.WithAttributes(attribute.String("redis.key", key)))
	defer span.End()

	var ex bool
	op := func(ctx context.Context) error {
		v, err := c.rdb.Exists(ctx, key).Result()
		if err != nil {
			return err
		}
		ex = v > 0
		return nil
	}
	notify := func(ctx context.Context, err error, delay time.Duration, attempt int) {
		c.log.WithContext(ctx).Warn("redis EXISTS retry",
			zap.String("key", key),
			zap.Int("attempt", attempt),
			zap.Duration("delay", delay),
			zap.Error(err),
		)
		fails.WithLabelValues(c.svc).Inc()
	}

	if err := backoff.Execute(ctx, c.cfg.Backoff, op, notify); err != nil {
		fails.WithLabelValues(c.svc).Inc()
		span.RecordError(err)
		return false, fmt.Errorf("redis EXISTS failed: %w", err)
	}
	hits.WithLabelValues(c.svc).Inc()
	return ex, nil
}

func (c *client) Close() error {
	return c.rdb.Close()
}
