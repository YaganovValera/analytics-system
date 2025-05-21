// api-gateway/internal/middleware/ratelimit.go
package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/YaganovValera/analytics-system/common/ctxkeys"
	"github.com/YaganovValera/analytics-system/common/logger"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type RateLimitConfig struct {
	RequestsPerSec float64
	BurstSize      int
	Log            *logger.Logger
}

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func RateLimitMiddleware(cfg RateLimitConfig) func(http.Handler) http.Handler {
	var (
		limiters sync.Map
		log      = cfg.Log.Named("ratelimit")
		ttl      = 10 * time.Minute
	)

	// Фоновая очистка
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			now := time.Now()
			limiters.Range(func(key, value any) bool {
				entry := value.(*limiterEntry)
				if now.Sub(entry.lastSeen) > ttl {
					limiters.Delete(key)
				}
				return true
			})
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			id := extractIdentity(ctx, r)

			val, _ := limiters.LoadOrStore(id, &limiterEntry{
				limiter:  rate.NewLimiter(rate.Limit(cfg.RequestsPerSec), cfg.BurstSize),
				lastSeen: time.Now(),
			})
			entry := val.(*limiterEntry)
			entry.lastSeen = time.Now()

			if !entry.limiter.Allow() {
				log.WithContext(ctx).Warn("rate limit exceeded", zap.String("identity", id))
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// extractIdentity извлекает user_id из ctx или IP-адрес из запроса.
func extractIdentity(ctx context.Context, r *http.Request) string {
	if uid, ok := ctx.Value(ctxkeys.UserIDKey).(string); ok && uid != "" {
		return "uid:" + uid
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = strings.Split(r.RemoteAddr, ":")[0]
	}
	return "ip:" + ip
}
