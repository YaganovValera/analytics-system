// services/preprocessor/internal/metrics/metrics.go
package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	once              sync.Once
	ProcessedMessages prometheus.Counter
	ProcessErrors     prometheus.Counter
	InsertErrors      prometheus.Counter
	InsertLatency     prometheus.Histogram
)

// Register initializes and registers all metrics exactly once.
// If r == nil, uses prometheus.DefaultRegisterer; duplicate registrations are ignored.
func Register(r prometheus.Registerer) {
	once.Do(func() {
		if r == nil {
			r = prometheus.DefaultRegisterer
		}

		ProcessedMessages = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "preprocessor", Subsystem: "processor", Name: "processed_messages_total",
			Help: "Total number of MarketData messages processed",
		})
		ProcessErrors = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "preprocessor", Subsystem: "processor", Name: "process_errors_total",
			Help: "Total number of errors during message processing",
		})
		InsertErrors = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "preprocessor", Subsystem: "processor", Name: "insert_errors_total",
			Help: "Total number of errors inserting raw data into TimescaleDB",
		})
		InsertLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "preprocessor", Subsystem: "processor", Name: "insert_latency_seconds",
			Help:    "Latency of inserting MarketData into TimescaleDB",
			Buckets: prometheus.DefBuckets,
		})

		collectors := []prometheus.Collector{
			ProcessedMessages,
			ProcessErrors,
			InsertErrors,
			InsertLatency,
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
