// common/httpserver/server.go

package httpserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"
)

// ReadyChecker вызывается при запросе /readyz и должен вернуть nil, если готовы зависимости.
type ReadyChecker func() error

// HTTPServer умеет стартовать сервер.
type HTTPServer interface {
	Run(ctx context.Context) error
}

// Middleware позволяет оборачивать HTTP-хендлеры.
type Middleware func(http.Handler) http.Handler

// New создаёт HTTPServer с метриками, healthz и readyz эндпоинтами.
func New(
	cfg Config,
	check ReadyChecker,
	log *logger.Logger,
	extra map[string]http.Handler,
	middlewares ...Middleware,
) (HTTPServer, error) {
	mux := http.NewServeMux()

	// /metrics
	mux.Handle(cfg.MetricsPath, otelhttp.NewHandler(promhttp.Handler(), "metrics"))

	// /healthz
	mux.HandleFunc(cfg.HealthzPath, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("OK"))
	})

	// /readyz
	mux.HandleFunc(cfg.ReadyzPath, func(w http.ResponseWriter, _ *http.Request) {
		if err := check(); err != nil {
			log.Warn("http: readyz check failed", zap.Error(err))
			_, _ = w.Write([]byte(fmt.Sprintf("NOT READY: %v", err)))
			return
		}
		_, _ = w.Write([]byte("READY"))
	})

	// доп. маршруты
	for path, handler := range extra {
		mux.Handle(path, otelhttp.NewHandler(handler, path))
	}

	// middleware
	handler := http.Handler(mux)
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
	}

	return &server{
		httpServer:      srv,
		shutdownTimeout: cfg.ShutdownTimeout,
		log:             log.Named("http-server"),
	}, nil
}

type server struct {
	httpServer      *http.Server
	shutdownTimeout time.Duration
	log             *logger.Logger
}

// Run запускает HTTP-сервер и корректно его завершает по отмене ctx.
func (s *server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	// используем внешний ctx для всего сервера
	s.httpServer.BaseContext = func(_ net.Listener) context.Context {
		return ctx
	}

	go func() {
		s.log.Info("http: starting server", zap.String("addr", s.httpServer.Addr))
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("httpserver: listen: %w", err)
		}
		close(errCh)
	}()

	var serveErr error
	select {
	case <-ctx.Done():
		s.log.Info("http: shutdown signal received")
		serveErr = ctx.Err()
	case err := <-errCh:
		serveErr = err
		if err != nil {
			s.log.Error("http: server error", zap.Error(err))
		}
	}

	// graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		s.log.Error("http: graceful shutdown failed", zap.Error(err))
		return err
	}
	s.log.Info("http: server stopped gracefully")
	s.log.Sync()

	return serveErr
}
