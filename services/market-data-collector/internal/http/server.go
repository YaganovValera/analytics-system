// services/market-data-collector/internal/http/server.go
package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/logger"
)

// ReadyChecker определяет функцию проверки готовности сервиса.
type ReadyChecker func() error

// Server инкапсулирует HTTP-сервер для /metrics, /healthz и /readyz.
type Server struct {
	srv       *http.Server
	readiness ReadyChecker
	log       *logger.Logger
}

// NewServer создаёт новый HTTP-сервер, слушающий addr (например, ":8080").
// readiness — функция, возвращающая ошибку, если сервис не готов.
// log — обёрнутый логгер.
func NewServer(addr string, readiness ReadyChecker, log *logger.Logger) *Server {
	mux := http.NewServeMux()
	// /metrics — Prometheus
	mux.Handle("/metrics", promhttp.Handler())
	// /healthz — liveness probe
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	// /readyz — readiness probe
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := readiness(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(fmt.Sprintf("NOT READY: %v", err)))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("READY"))
	})

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	return &Server{srv: srv, readiness: readiness, log: log}
}

// Start запускает HTTP-сервер и возвращает ошибку, если старт не удался.
// Ожидает ctx.Done() для graceful shutdown.
func (s *Server) Start(ctx context.Context) error {
	go func() {
		s.log.Sugar().Infow("http: starting server", "addr", s.srv.Addr)
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Sugar().Errorw("http: server error", "error", err)
		}
	}()

	// Ожидаем отмены контекста
	<-ctx.Done()
	s.log.Sugar().Infow("http: shutdown signal received")

	// Даем до 5 секунд на завершение
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.srv.Shutdown(shutCtx); err != nil {
		s.log.Sugar().Errorw("http: graceful shutdown failed", "error", err)
		return err
	}
	s.log.Sugar().Infow("http: server stopped gracefully")
	return nil
}
