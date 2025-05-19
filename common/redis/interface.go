// common/redis/interface.go
package redis

import (
	"context"
	"time"
)

// Client определяет набор операций над Redis.
type Client interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) (int64, error)
	Exists(ctx context.Context, key string) (bool, error)
	Close() error
}
