// common/httpserver/server.go
package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/YaganovValera/analytics-system/common/logger"
)

// -----------------------------------------------------------------------------
// API
// -----------------------------------------------------------------------------

// ReadyChecker возвращает nil, если сервис готов обслуживать запросы.
type ReadyChecker func() error

// HTTPServer — обобщённый контракт запуска/остановки HTTP-сервера.
type HTTPServer interface {
	// Start блокирующе запускает сервер и реагирует на ctx.Done().
	// Метод возвращает ошибку старта ListenAndServe или ctx.Err().
	Start(ctx context.Context) error
}

// Config определяет все таймауты и адрес для HTTP-сервера.
//
// Нулевые значения заменяются дефолтами в applyDefaults().
type Config struct {
	Addr            string        // адрес в формате ":8080"
	ReadTimeout     time.Duration // максимум чтения полного запроса
	WriteTimeout    time.Duration // максимум записи ответа
	IdleTimeout     time.Duration // keep-alive idle
	ShutdownTimeout time.Duration // время на graceful-shutdown
	MetricsPath     string        // endpoint для Prometheus; "" → /metrics
	HealthzPath     string        // endpoint для liveness; "" → /healthz
	ReadyzPath      string        // endpoint для readiness; "" → /readyz
}

// applyDefaults проставляет безопасные дефолты.
func (c *Config) applyDefaults() {
	if c.ReadTimeout <= 0 {
		c.ReadTimeout = 10 * time.Second
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = 15 * time.Second
	}
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = 60 * time.Second
	}
	if c.ShutdownTimeout <= 0 {
		c.ShutdownTimeout = 5 * time.Second
	}
	if c.MetricsPath == "" {
		c.MetricsPath = "/metrics"
	}
	if c.HealthzPath == "" {
		c.HealthzPath = "/healthz"
	}
	if c.ReadyzPath == "" {
		c.ReadyzPath = "/readyz"
	}
}

// validate гарантирует обязательные поля.
func (c Config) validate() error {
	if c.Addr == "" {
		return fmt.Errorf("httpserver: Addr is required")
	}
	return nil
}

// -----------------------------------------------------------------------------
// Реализация
// -----------------------------------------------------------------------------

type server struct {
	httpServer      *http.Server
	shutdownTimeout time.Duration
	check           ReadyChecker
	log             *logger.Logger
}

// New конструирует HTTP-сервер со всеми таймаутами и сервис-эндпоинтами.
func New(cfg Config, check ReadyChecker, log *logger.Logger) (HTTPServer, error) {
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle(cfg.MetricsPath, promhttp.Handler())
	mux.HandleFunc(cfg.HealthzPath, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	mux.HandleFunc(cfg.ReadyzPath, func(w http.ResponseWriter, _ *http.Request) {
		if err := check(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(fmt.Sprintf("NOT READY: %v", err)))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("READY"))
	})

	httpSrv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	return &server{
		httpServer:      httpSrv,
		shutdownTimeout: cfg.ShutdownTimeout,
		check:           check,
		log:             log.Named("http-server"),
	}, nil
}

// Start запускает ListenAndServe и дожидается завершения ctx или ошибки запуска.
func (s *server) Start(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		s.log.Info("http: starting server", zap.String("addr", s.httpServer.Addr))
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("httpserver: listen: %w", err)
		}
		close(errCh)
	}()

	// Ожидание контекста или ошибки старта
	var serveErr error
	select {
	case <-ctx.Done():
		s.log.Info("http: shutdown signal received")
	case err := <-errCh:
		serveErr = err
	}

	// Всегда пытаемся graceful-shutdown с корректным таймаутом
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		s.log.Error("http: graceful shutdown failed", zap.Error(err))
		return err
	}
	s.log.Info("http: server stopped gracefully")

	// Flush буферы zap
	s.log.Sync()
	return serveErr
}
