// api-gateway/internal/transport/http/routes.go
package http

import (
	"net/http"

	"github.com/YaganovValera/analytics-system/services/api-gateway/internal/handler"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Routes возвращает основной маршрутизатор с подключёнными роутами и middleware.
func Routes(h *handler.Handler, m *Middleware) http.Handler {
	r := chi.NewRouter()

	// Встроенные middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Пользовательские middleware
	r.Use(m.WithContext)

	// Public endpoints
	r.Route("/v1", func(r chi.Router) {
		r.Post("/login", h.Login)
		r.Post("/register", h.Register)
		r.Post("/refresh", h.Refresh)
		r.Get("/candles", h.GetCandles)
	})

	return r
}
