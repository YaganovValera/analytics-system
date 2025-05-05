package backoff

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/YaganovValera/analytics-system/services/preprocessor/pkg/logger"
	"github.com/cenkalti/backoff/v4"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// Config holds parameters for exponential backoff retry.
type Config struct {
	InitialInterval     time.Duration // default: 1s
	RandomizationFactor float64       // default: 0.5
	Multiplier          float64       // default: 2.0
	MaxInterval         time.Duration // default: 30s
	MaxElapsedTime      time.Duration // default: unlimited
	PerAttemptTimeout   time.Duration // default: unlimited
}

// RetryableFunc defines the operation to retry.
type RetryableFunc func(ctx context.Context) error

// ErrMaxRetries indicates retries exhausted.
type ErrMaxRetries struct {
	Err      error
	Attempts int
}

func (e *ErrMaxRetries) Error() string {
	return fmt.Sprintf("backoff: %d attempts failed: %v", e.Attempts, e.Err)
}
func (e *ErrMaxRetries) Unwrap() error { return e.Err }

var (
	retriesTotal   prometheus.Counter
	failuresTotal  prometheus.Counter
	successesTotal prometheus.Counter
	delayHistogram prometheus.Histogram
	registerOnce   sync.Once
)

func initMetrics() {
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
		Help: "Number of operations succeeded",
	})
	delayHistogram = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "collector", Subsystem: "backoff", Name: "retry_delay_seconds",
		Help:    "Histogram of retry delays in seconds",
		Buckets: prometheus.DefBuckets,
	})
}

// Execute runs fn with exponential backoff and collects metrics.
// Returns ErrMaxRetries if all attempts fail.
func Execute(ctx context.Context, cfg Config, log *logger.Logger, fn RetryableFunc) error {
	// register metrics once
	registerOnce.Do(func() {
		initMetrics()
		prometheus.MustRegister(retriesTotal, failuresTotal, successesTotal, delayHistogram)
	})

	// apply defaults
	if cfg.InitialInterval <= 0 {
		cfg.InitialInterval = time.Second
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

	// build backoff policy
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = cfg.InitialInterval
	bo.RandomizationFactor = cfg.RandomizationFactor
	bo.Multiplier = cfg.Multiplier
	bo.MaxInterval = cfg.MaxInterval
	if cfg.MaxElapsedTime > 0 {
		bo.MaxElapsedTime = cfg.MaxElapsedTime
	}

	// wrap with context
	boCtx := backoff.WithContext(bo, ctx)
	attempts := 0

	// define operation
	operation := func() error {
		attempts++
		// per-attempt timeout
		if cfg.PerAttemptTimeout > 0 {
			atCtx, cancel := context.WithTimeout(ctx, cfg.PerAttemptTimeout)
			defer cancel()
			return fn(atCtx)
		}
		return fn(ctx)
	}

	// notify on retry
	notify := func(err error, delay time.Duration) {
		retriesTotal.Inc()
		delayHistogram.Observe(delay.Seconds())
		log.Warn("backoff retry",
			zap.Error(err),
			zap.Duration("delay", delay),
			zap.Int("attempt", attempts),
		)
	}

	// run retry
	err := backoff.RetryNotify(operation, boCtx, notify)
	if err != nil {
		failuresTotal.Inc()
		log.Error("backoff give up",
			zap.Error(err),
			zap.Int("attempts", attempts),
		)
		return &ErrMaxRetries{Err: err, Attempts: attempts}
	}

	successesTotal.Inc()
	return nil
}
