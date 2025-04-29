// pkg/backoff/backoff.go
package backoff

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/logger"
	"github.com/cenkalti/backoff/v4"
	"github.com/prometheus/client_golang/prometheus"
)

// Config хранит настройки экспоненциального backoff-а и опционального таймаута на попытку.
type Config struct {
	InitialInterval     time.Duration // начальный интервал (если 0 — default 1s)
	RandomizationFactor float64       // jitter (если 0 — default 0.5)
	Multiplier          float64       // множитель (если 0 — default 2.0)
	MaxInterval         time.Duration // макс. интервал (если 0 — default 30s)
	MaxElapsedTime      time.Duration // общее время ретраев (если 0 — без лимита)
	PerAttemptTimeout   time.Duration // таймаут одной попытки (0 = без)
}

// RetryableFunc описывает функцию с поддержкой контекста.
type RetryableFunc func(ctx context.Context) error

// BackoffError возвращается, когда операции исчерпаны.
type BackoffError struct {
	Err      error // итоговая ошибка (context или fn)
	Attempts int   // число совершённых попыток
}

func (e *BackoffError) Error() string {
	return fmt.Sprintf("backoff: failed after %d attempts: %v", e.Attempts, e.Err)
}
func (e *BackoffError) Unwrap() error { return e.Err }

// Метрики для retry-механизма.
var (
	retriesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "backoff", Name: "retries_total",
		Help: "Number of retry attempts",
	})
	failuresTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "backoff", Name: "failures_total",
		Help: "Number of operations giving up after retries",
	})
	successesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "collector", Subsystem: "backoff", Name: "successes_total",
		Help: "Number of operations succeeded (possibly after retries)",
	})
	retryDelayHistogram = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "collector", Subsystem: "backoff", Name: "retry_delay_seconds",
		Help:    "Histogram of retry delays in seconds",
		Buckets: prometheus.DefBuckets,
	})

	registerOnce sync.Once
)

// registerMetrics безопасно регистрирует все метрики.
func registerMetrics(r prometheus.Registerer) {
	registerOnce.Do(func() {
		r.MustRegister(retriesTotal, failuresTotal, successesTotal, retryDelayHistogram)
	})
}

// WithBackoff выполняет fn с экспоненциальным backoff-ом и метриками.
// При завершении приложения можно вызвать logger.Sync(), если нужно.
func WithBackoff(
	ctx context.Context,
	cfg Config,
	log *logger.Logger,
	fn RetryableFunc,
) error {
	// 1) Регистрация метрик в default-регистратор (можно заменить на свой).
	registerMetrics(prometheus.DefaultRegisterer)

	// 2) Присвоение default-значений.
	if cfg.InitialInterval <= 0 {
		cfg.InitialInterval = 1 * time.Second
	}
	if cfg.RandomizationFactor <= 0 {
		cfg.RandomizationFactor = 0.5
	}
	if cfg.Multiplier <= 0 {
		cfg.Multiplier = 2.0
	}
	if cfg.MaxInterval <= 0 {
		cfg.MaxInterval = 30 * time.Second
	}
	// MaxElapsedTime == 0 → без лимита (оставляем NewExponentialBackOff.Default)

	// 3) Создаём backoff.
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = cfg.InitialInterval
	bo.RandomizationFactor = cfg.RandomizationFactor
	bo.Multiplier = cfg.Multiplier
	bo.MaxInterval = cfg.MaxInterval
	if cfg.MaxElapsedTime > 0 {
		bo.MaxElapsedTime = cfg.MaxElapsedTime
	}

	boCtx := backoff.WithContext(bo, ctx)

	// 4) Счётчик попыток.
	var attempts int

	// 5) Операция с таймаутом на попытку.
	operation := func() error {
		attempts++
		if t := cfg.PerAttemptTimeout; t > 0 {
			ctxAttempt, cancel := context.WithTimeout(ctx, t)
			defer cancel()
			return fn(ctxAttempt)
		}
		return fn(ctx)
	}

	// 6) Notify-функция.
	notify := func(err error, delay time.Duration) {
		retriesTotal.Inc()
		retryDelayHistogram.Observe(delay.Seconds())
		log.Sugar().Warnw("backoff retry", "error", err, "delay", delay, "attempt", attempts)
	}

	// 7) Запуск RetryNotify.
	err := backoff.RetryNotify(operation, boCtx, notify)
	if err != nil {
		failuresTotal.Inc()
		log.Sugar().Errorw("backoff give up", "error", err, "attempts", attempts)
		return &BackoffError{Err: err, Attempts: attempts}
	}

	// 8) Успешное завершение (по крайней мере одна попытка).
	successesTotal.Inc()
	return nil
}
