// preprocessor/pkg/redis/cache.go
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	client *redis.Client
	ttl    time.Duration
}

// New создаёт Redis‑клиент и проверяет соединение
func New(addr, password string, db, ttlSeconds int) (*Cache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &Cache{
		client: client,
		ttl:    time.Duration(ttlSeconds) * time.Second,
	}, nil
}

// Close закрывает соединение с Redis
func (c *Cache) Close() error {
	return c.client.Close()
}

// StoreRaw кладёт сообщение в список key=raw:<symbol> с автоматическим ttl
func (c *Cache) StoreRaw(ctx context.Context, symbol string, data []byte) error {
	key := fmt.Sprintf("raw:%s", symbol)
	if err := c.client.LPush(ctx, key, data).Err(); err != nil {
		return fmt.Errorf("LPUSH %s: %w", key, err)
	}
	if err := c.client.Expire(ctx, key, c.ttl).Err(); err != nil {
		return fmt.Errorf("EXPIRE %s: %w", key, err)
	}
	return nil
}
