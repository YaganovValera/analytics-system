// common/backoff/backoff.go
package backoff

import (
	"context"
	"fmt"
	"time"

	cbackoff "github.com/cenkalti/backoff/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"

	"github.com/YaganovValera/analytics-system/common/logger"
)

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

type Config struct {
	InitialInterval     time.Duration
	RandomizationFactor float64
	Multiplier          float64
	MaxInterval         time.Duration
	MaxElapsedTime      time.Duration
	PerAttemptTimeout   time.Duration
}

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

func (c Config) validate() error {
	if c.RandomizationFactor < 0 || c.RandomizationFactor > 1 {
		return fmt.Errorf("backoff: RandomizationFactor must be in [0,1]")
	}
	if c.Multiplier < 1 {
		return fmt.Errorf("backoff: Multiplier must be ≥ 1")
	}
	if c.PerAttemptTimeout < 0 {
		return fmt.Errorf("backoff: PerAttemptTimeout must be ≥ 0")
	}
	return nil
}

type RetryableFunc func(ctx context.Context) error

type ErrMaxRetries struct {
	Err      error
	Attempts int
}

func (e *ErrMaxRetries) Error() string {
	return fmt.Sprintf("backoff: %d attempt(s) failed: %v", e.Attempts, e.Err)
}

func (e *ErrMaxRetries) Unwrap() error { return e.Err }
func Permanent(err error) error        { return cbackoff.Permanent(err) }

func Execute(ctx context.Context, cfg Config, log *logger.Logger, fn RetryableFunc) error {
	if serviceLabel == "unknown" {
		log.Warn("backoff: service label not set", zap.String("service", serviceLabel))
	}

	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return fmt.Errorf("backoff: invalid config: %w", err)
	}

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

	notify := func(err error, delay time.Duration) {
		metrics.Retries.WithLabelValues(serviceLabel).Inc()
		metrics.Delays.WithLabelValues(serviceLabel).Observe(delay.Seconds())
		log.WithContext(ctx).Warn("back-off retry",
			zap.Int("attempt", attempts),
			zap.Duration("delay", delay),
			zap.Error(err),
		)
	}

	if err := cbackoff.RetryNotify(operation, boCtx, notify); err != nil {
		metrics.Failures.WithLabelValues(serviceLabel).Inc()
		log.WithContext(ctx).Error("back-off give-up",
			zap.Int("attempts", attempts),
			zap.Error(err),
		)
		return &ErrMaxRetries{Err: err, Attempts: attempts}
	}

	metrics.Successes.WithLabelValues(serviceLabel).Inc()
	return nil
}
