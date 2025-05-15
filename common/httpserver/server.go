package httpserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/YaganovValera/analytics-system/common/logger"
)

// ReadyChecker returns nil if the service is ready to serve.
type ReadyChecker func() error

// HTTPServer defines Run(context) error.
type HTTPServer interface {
	Run(ctx context.Context) error
}

// Config defines timeouts and paths for the HTTP server.
type Config struct {
	Addr            string // e.g. ":8080"
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	MetricsPath     string
	HealthzPath     string
	ReadyzPath      string
}

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

func (c Config) validate() error {
	if c.Addr == "" {
		return fmt.Errorf("httpserver: Addr is required")
	}
	return nil
}

type server struct {
	httpServer      *http.Server
	shutdownTimeout time.Duration
	log             *logger.Logger
}

// New constructs an HTTPServer with metrics and health endpoints, plus extra routes.
func New(cfg Config, check ReadyChecker, log *logger.Logger, extra map[string]http.Handler) (HTTPServer, error) {
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
			log.Warn("http: readyz check failed", zap.Error(err))
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(fmt.Sprintf("NOT READY: %v", err)))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("READY"))
	})

	for path, handler := range extra {
		mux.Handle(path, handler)
	}

	httpSrv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background() // переопределяется в Run
		},
	}

	return &server{
		httpServer:      httpSrv,
		shutdownTimeout: cfg.ShutdownTimeout,
		log:             log.Named("http-server"),
	}, nil
}

// Run starts the server and shuts down gracefully on context cancellation.
func (s *server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

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
