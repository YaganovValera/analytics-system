// api-gateway/internal/transport/http/routes.go
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func Routes(h *Handler, mw *Middleware) http.Handler {
	r := chi.NewRouter()

	r.Post("/login", h.Login)
	r.Post("/register", h.Register)
	r.Post("/refresh", h.Refresh)

	r.Group(func(r chi.Router) {
		r.Use(mw.JWTMiddleware)
		r.Use(mw.RBAC([]string{"user", "admin"}))

		r.Get("/candles", h.GetCandles)
	})

	return r
}
