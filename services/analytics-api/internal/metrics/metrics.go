// services/analytics-api/internal/metrics/metrics.go
package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	once sync.Once

	// GetCandles metrics
	GetCandlesRequests prometheus.Counter
	GetCandlesErrors   prometheus.Counter
	GetCandlesLatency  prometheus.Histogram

	// StreamCandles metrics
	StreamCandlesRequests prometheus.Counter
	StreamCandlesErrors   prometheus.Counter
	StreamCandlesEvents   prometheus.Counter
)

// Register инициализирует и регистрирует все метрики.
// Если r == nil, используется prometheus.DefaultRegisterer.
// Дублирующая регистрация игнорируется.
func Register(r prometheus.Registerer) {
	once.Do(func() {
		if r == nil {
			r = prometheus.DefaultRegisterer
		}

		GetCandlesRequests = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "analytics_api", Subsystem: "storage", Name: "get_candles_requests_total",
			Help: "Total number of GetCandles calls",
		})
		GetCandlesErrors = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "analytics_api", Subsystem: "storage", Name: "get_candles_errors_total",
			Help: "Total number of errors in GetCandles",
		})
		GetCandlesLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "analytics_api", Subsystem: "storage", Name: "get_candles_latency_seconds",
			Help:    "Latency distribution of GetCandles calls",
			Buckets: prometheus.DefBuckets,
		})

		StreamCandlesRequests = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "analytics_api", Subsystem: "storage", Name: "stream_candles_requests_total",
			Help: "Total number of StreamCandles calls",
		})
		StreamCandlesErrors = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "analytics_api", Subsystem: "storage", Name: "stream_candles_errors_total",
			Help: "Total number of errors in StreamCandles",
		})
		StreamCandlesEvents = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "analytics_api", Subsystem: "storage", Name: "stream_candles_events_total",
			Help: "Total number of CandleEvents emitted by StreamCandles",
		})

		collectors := []prometheus.Collector{
			GetCandlesRequests, GetCandlesErrors, GetCandlesLatency,
			StreamCandlesRequests, StreamCandlesErrors, StreamCandlesEvents,
		}
		for _, c := range collectors {
			if err := r.Register(c); err != nil {
				if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
					panic(err)
				}
			}
		}
	})
}
