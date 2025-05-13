package shutdown

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// WaitForSignals блокирует выполнение до SIGINT/SIGTERM,
// вызывает cancel() и логирует завершение.
func WaitForSignals(ctx context.Context, cancel context.CancelFunc, log *zap.Logger) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Info("shutdown: signal received", zap.String("signal", sig.String()))
		cancel()
	case <-ctx.Done():
		// context already cancelled
	}
}

// GracefulShutdown выполняет shutdown-функции с таймаутом.
// Используется, например, для `http.Server.Shutdown(ctx)`.
func GracefulShutdown(name string, timeout time.Duration, fn func(ctx context.Context) error, log *zap.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.Info("shutdown: stopping " + name)
	if err := fn(ctx); err != nil {
		log.Error("shutdown: error in "+name, zap.Error(err))
	} else {
		log.Info("shutdown: " + name + " stopped cleanly")
	}
}
