// common/backoff/backoff.go
package backoff

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"

	"github.com/YaganovValera/analytics-system/common/logger"
)

// -----------------------------------------------------------------------------
// Metrics & service label
// -----------------------------------------------------------------------------

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

// SetServiceLabel must be called once from common.InitServiceName(..)
// before the first Execute(..).  See common/service.go.
func SetServiceLabel(name string) { serviceLabel = name }

// -----------------------------------------------------------------------------
// Configuration
// -----------------------------------------------------------------------------

// Config contains tunables for exponential back-off.
//
// All zero values are treated as “use reasonable default”.
type Config struct {
	// InitialInterval is the first delay before retrying.
	InitialInterval time.Duration

	// RandomizationFactor adds ±jitter to each delay.
	// Accepted range: 0.0 ≤ f ≤ 1.0
	RandomizationFactor float64

	// Multiplier multiplies the previous delay to get the next one
	// ( e.g. 2 → doubles on every retry ).
	Multiplier float64

	// MaxInterval caps each individual delay.
	MaxInterval time.Duration

	// MaxElapsedTime is the total time allowed for all retries
	// before giving up.  Zero → unlimited.
	MaxElapsedTime time.Duration

	// PerAttemptTimeout limits the execution time of every single
	// user function call.  Zero → no per-attempt timeout.
	PerAttemptTimeout time.Duration
}

// applyDefaults fills cfg with safe defaults in-place.
func (c *Config) applyDefaults() {
	if c.InitialInterval <= 0 {
		c.InitialInterval = time.Second
	}
	if c.RandomizationFactor <= 0 {
		c.RandomizationFactor = 0.5
	}
	if c.Multiplier <= 0 {
		c.Multiplier = 2.0
	}
	if c.MaxInterval <= 0 {
		c.MaxInterval = 30 * time.Second
	}
}

// validate performs cheap sanity checks.
func (c Config) validate() error {
	if c.RandomizationFactor < 0 || c.RandomizationFactor > 1 {
		return fmt.Errorf("backoff: RandomizationFactor must be in [0,1]")
	}
	if c.Multiplier < 1 {
		return fmt.Errorf("backoff: Multiplier must be ≥ 1")
	}
	return nil
}

// RetryableFunc is a unit of work that may be re-executed until it
// succeeds or the back-off strategy gives up.
type RetryableFunc func(ctx context.Context) error

// -----------------------------------------------------------------------------
// Errors
// -----------------------------------------------------------------------------

// ErrMaxRetries is returned from Execute(..) when the function was still
// failing after all retries were exhausted.
type ErrMaxRetries struct {
	Err      error // last error returned by fn
	Attempts int   // number of attempts performed
}

func (e *ErrMaxRetries) Error() string {
	return fmt.Sprintf("backoff: %d attempt(s) failed: %v", e.Attempts, e.Err)
}
func (e *ErrMaxRetries) Unwrap() error { return e.Err }

// Permanent marks an error as non-retryable.
func Permanent(err error) error { return backoff.Permanent(err) }

// -----------------------------------------------------------------------------
// Core
// -----------------------------------------------------------------------------

// Execute runs fn() with an exponential back-off defined by cfg, emitting
// Prometheus metrics and structured logs via log.
func Execute(ctx context.Context, cfg Config, log *logger.Logger, fn RetryableFunc) error {
	// Prepare configuration.
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return fmt.Errorf("backoff: invalid config: %w", err)
	}

	// Build strategy.
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = cfg.InitialInterval
	bo.RandomizationFactor = cfg.RandomizationFactor
	bo.Multiplier = cfg.Multiplier
	bo.MaxInterval = cfg.MaxInterval
	if cfg.MaxElapsedTime > 0 {
		bo.MaxElapsedTime = cfg.MaxElapsedTime
	} else {
		bo.MaxElapsedTime = backoff.Stop
	}
	boCtx := backoff.WithContext(bo, ctx)

	// Instrumented execution.
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
	notify := func(err error, delay time.Duration) {
		metrics.Retries.WithLabelValues(serviceLabel).Inc()
		metrics.Delays.WithLabelValues(serviceLabel).Observe(delay.Seconds())
		log.Warn("back-off retry",
			zap.Int("attempt", attempts),
			zap.Duration("delay", delay),
			zap.Error(err),
		)
	}

	if err := backoff.RetryNotify(operation, boCtx, notify); err != nil {
		metrics.Failures.WithLabelValues(serviceLabel).Inc()
		log.Error("back-off give-up",
			zap.Int("attempts", attempts),
			zap.Error(err),
		)
		return &ErrMaxRetries{Err: err, Attempts: attempts}
	}

	metrics.Successes.WithLabelValues(serviceLabel).Inc()
	return nil
}
