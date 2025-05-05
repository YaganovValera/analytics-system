// services/preprocessor/pkg/redis/interface.go
package redis

import (
	"context"
	"time"
)

// Cache описывает контракт Redis-кэша.
type Cache interface {
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Get(ctx context.Context, key string) ([]byte, error)
	Close() error
}
