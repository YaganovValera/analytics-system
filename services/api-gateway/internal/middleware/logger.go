// api-gateway/internal/middleware/logger.go
package middleware

import (
	"net/http"
	"time"

	"github.com/YaganovValera/analytics-system/common/ctxkeys"
	"github.com/YaganovValera/analytics-system/common/logger"
	"go.uber.org/zap"
)

// RequestLoggerMiddleware логирует входящие HTTP-запросы с контекстом.
func RequestLoggerMiddleware(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ctx := r.Context()
			ww := wrapWriter(w)

			// передаём дальше
			next.ServeHTTP(ww, r)

			status := ww.Status()
			duration := time.Since(start)

			entry := log.WithContext(ctx)

			fields := []zap.Field{
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", status),
				zap.Float64("latency_ms", float64(duration.Milliseconds())),
			}

			if userID, ok := ctx.Value(ctxkeys.UserIDKey).(string); ok && userID != "" {
				fields = append(fields, zap.String("user_id", userID))
			}
			if traceID, ok := ctx.Value(ctxkeys.TraceIDKey).(string); ok && traceID != "" {
				fields = append(fields, zap.String("trace_id", traceID))
			}

			switch {
			case status >= 500:
				entry.Error("HTTP request", fields...)
			case status >= 400:
				entry.Warn("HTTP request", fields...)
			default:
				entry.Info("HTTP request", fields...)
			}
		})
	}
}

// responseWriterWrapper позволяет перехватить статус ответа.
type responseWriterWrapper struct {
	http.ResponseWriter
	status int
}

func wrapWriter(w http.ResponseWriter) *responseWriterWrapper {
	return &responseWriterWrapper{ResponseWriter: w, status: http.StatusOK}
}

func (rw *responseWriterWrapper) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriterWrapper) Status() int {
	return rw.status
}
