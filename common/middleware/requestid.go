package middleware

import (
	"context"
	"net/http"

	"github.com/YaganovValera/analytics-system/common/ctxkeys"
	"github.com/google/uuid"
)

func RequestID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := r.Header.Get("X-Request-ID")
			if reqID == "" {
				reqID = uuid.NewString()
			}
			ctx := context.WithValue(r.Context(), ctxkeys.RequestIDKey, reqID)
			w.Header().Set("X-Request-ID", reqID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
