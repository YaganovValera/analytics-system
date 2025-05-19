// common/backoff/backoff.go
package backoff

import (
	"context"
	"fmt"
	"time"

	cbackoff "github.com/cenkalti/backoff/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/YaganovValera/analytics-system/common/serviceid"
)

func init() {
	serviceid.Register(SetServiceLabel)
}

var (
	serviceLabel = "unknown"

	metrics = struct {
		Retries   *prometheus.CounterVec
		Failures  *prometheus.CounterVec
		Successes *prometheus.CounterVec
		Delays    *prometheus.HistogramVec
	}{
		Retries: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "common", Subsystem: "backoff", Name: "retries_total",
				Help: "Number of back-off retry attempts",
			},
			[]string{"service"},
		),
		Failures: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "common", Subsystem: "backoff", Name: "failures_total",
				Help: "Number of operations that gave up after retries",
			},
			[]string{"service"},
		),
		Successes: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "common", Subsystem: "backoff", Name: "successes_total",
				Help: "Number of operations that eventually succeeded",
			},
			[]string{"service"},
		),
		Delays: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "common", Subsystem: "backoff", Name: "retry_delay_seconds",
				Help:    "Histogram of retry delays (seconds)",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"service"},
		),
	}
)

func SetServiceLabel(name string) {
	serviceLabel = name
}

type RetryableFunc func(ctx context.Context) error

// NotifyFunc используется для уведомлений о попытках
type NotifyFunc func(ctx context.Context, err error, delay time.Duration, attempt int)

type ErrMaxRetries struct {
	Err      error
	Attempts int
}

func (e *ErrMaxRetries) Error() string {
	return fmt.Sprintf("backoff: %d attempt(s) failed: %v", e.Attempts, e.Err)
}

func (e *ErrMaxRetries) Unwrap() error { return e.Err }
func Permanent(err error) error        { return cbackoff.Permanent(err) }

// Execute выполняет fn с exponential backoff и метриками.
// Не логирует ошибки напрямую — логика выносится в notify.
func Execute(ctx context.Context, cfg Config, fn RetryableFunc, notify NotifyFunc) error {
	bo := cbackoff.NewExponentialBackOff()
	bo.InitialInterval = cfg.InitialInterval
	bo.RandomizationFactor = cfg.RandomizationFactor
	bo.Multiplier = cfg.Multiplier
	bo.MaxInterval = cfg.MaxInterval
	if cfg.MaxElapsedTime > 0 {
		bo.MaxElapsedTime = cfg.MaxElapsedTime
	}
	boCtx := cbackoff.WithContext(bo, ctx)

	attempts := 0
	operation := func() error {
		attempts++
		if cfg.PerAttemptTimeout > 0 {
			atCtx, cancel := context.WithTimeout(ctx, cfg.PerAttemptTimeout)
			defer cancel()
			return fn(atCtx)
		}
		return fn(ctx)
	}

	notifyWrap := func(err error, delay time.Duration) {
		metrics.Retries.WithLabelValues(serviceLabel).Inc()
		metrics.Delays.WithLabelValues(serviceLabel).Observe(delay.Seconds())
		if notify != nil {
			notify(ctx, err, delay, attempts)
		}
	}

	if err := cbackoff.RetryNotify(operation, boCtx, notifyWrap); err != nil {
		metrics.Failures.WithLabelValues(serviceLabel).Inc()
		return &ErrMaxRetries{Err: err, Attempts: attempts}
	}

	metrics.Successes.WithLabelValues(serviceLabel).Inc()
	return nil
}
