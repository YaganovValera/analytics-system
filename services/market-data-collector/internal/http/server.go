package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/logger"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// ReadyChecker returns nil if the service is ready.
type ReadyChecker func() error

// Server hosts the HTTP endpoints for metrics, liveness, and readiness.
type Server struct {
	httpServer *http.Server
	checkReady ReadyChecker
	log        *logger.Logger
}

// NewServer constructs an HTTP server listening on addr.
// The readiness function is invoked on /readyz.
func NewServer(addr string, checkReady ReadyChecker, log *logger.Logger) *Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := checkReady(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(fmt.Sprintf("NOT READY: %v", err)))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("READY"))
	})

	return &Server{
		httpServer: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
		checkReady: checkReady,
		log:        log.Named("http-server"),
	}
}

// Start runs the HTTP server in a goroutine and waits for ctx cancellation.
// Upon cancellation, it performs a graceful shutdown with a 5s timeout.
func (s *Server) Start(ctx context.Context) error {
	go func() {
		s.log.Info("http: starting server", zap.String("addr", s.httpServer.Addr))
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error("http: server error", zap.Error(err))
		}
	}()

	<-ctx.Done()
	s.log.Info("http: shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		s.log.Error("http: graceful shutdown failed", zap.Error(err))
		return err
	}
	s.log.Info("http: server stopped gracefully")
	return nil
}
