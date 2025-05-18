package httpserver

import (
	"log"
	"net/http"
	"runtime/debug"

	"github.com/go-chi/cors"
)

// RecoverMiddleware перехватывает паники и возвращает 500.
func RecoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rcv := recover(); rcv != nil {
				log.Printf("panic: %v\n%s", rcv, debug.Stack())
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// CORSMiddleware возвращает permissive CORS.
func CORSMiddleware() Middleware {
	return cors.AllowAll().Handler
}
