// services/market-data-collector/internal/http/server.go
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

// ReadyChecker возвращает nil, если сервис готов.
type ReadyChecker func() error

// Server инкапсулирует HTTP эндпоинты: /metrics, /healthz, /readyz.
type Server struct {
	httpServer *http.Server
	checkReady ReadyChecker
	log        *logger.Logger
}

// NewServer создаёт HTTPServer по адресу addr.
// checkReady вызывается на /readyz.
func NewServer(addr string, checkReady ReadyChecker, log *logger.Logger) HTTPServer {
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

// Start запускает HTTP-сервер и блокирует до отмены ctx или фатальной ошибки запуска.
// По отмене ctx выполняется graceful shutdown с таймаутом 5 секунд.
func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 1)

	// Запускаем сервер в отдельной горутине и сразу ловим ошибки старта.
	go func() {
		s.log.Info("http: starting server", zap.String("addr", s.httpServer.Addr))
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		// инициируем shutdown
		s.log.Info("http: shutdown signal received")
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("http server failed to start: %w", err)
		}
		// errCh закрыт без ошибки => сервер завершился некритично
		return nil
	}

	// graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		s.log.Error("http: graceful shutdown failed", zap.Error(err))
		return err
	}

	s.log.Info("http: server stopped gracefully")
	return nil
}
