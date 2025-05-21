// api-gateway/internal/middleware/jwt_cache.go
package middleware

import (
	"sync"
	"time"

	authpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/auth"
)

// cachedToken хранит claims и срок действия токена.
type cachedToken struct {
	claims    *authpb.ValidateTokenResponse
	expiresAt time.Time
}

// JWTCache — потокобезопасный кэш для access-токенов.
type JWTCache struct {
	mu    sync.RWMutex
	store map[string]cachedToken
}

// NewJWTCache создаёт и запускает фоновую очистку кэша.
func NewJWTCache() *JWTCache {
	c := &JWTCache{
		store: make(map[string]cachedToken),
	}
	go c.cleanupLoop()
	return c
}

// Get проверяет токен в кэше.
func (c *JWTCache) Get(token string) (*authpb.ValidateTokenResponse, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.store[token]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.claims, true
}

// Put сохраняет claims с TTL на основе expires_at.
func (c *JWTCache) Put(token string, claims *authpb.ValidateTokenResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	exp := time.Now().Add(15 * time.Minute)
	if claims.ExpiresAt != nil {
		exp = claims.ExpiresAt.AsTime()
	}
	c.store[token] = cachedToken{
		claims:    claims,
		expiresAt: exp,
	}
}

// cleanupLoop очищает протухшие записи.
func (c *JWTCache) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for now := range ticker.C {
		c.mu.Lock()
		for k, v := range c.store {
			if now.After(v.expiresAt) {
				delete(c.store, k)
			}
		}
		c.mu.Unlock()
	}
}
