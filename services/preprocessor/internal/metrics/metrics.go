// services/preprocessor/internal/metrics/metrics.go
package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	once              sync.Once
	ConsumedMessages  prometheus.Counter
	AggregationErrors prometheus.Counter
	CandlesPublished  prometheus.Counter
	PublishErrors     prometheus.Counter
	PublishLatency    prometheus.Histogram
)

// Register initializes and registers all metrics exactly once.
// If r is nil, uses prometheus.DefaultRegisterer.
// Duplicate registrations (AlreadyRegisteredError) are ignored.
func Register(r prometheus.Registerer) {
	once.Do(func() {
		if r == nil {
			r = prometheus.DefaultRegisterer
		}

		ConsumedMessages = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "preprocessor", Subsystem: "processor", Name: "consumed_messages_total",
			Help: "Total number of raw messages consumed from Kafka",
		})
		AggregationErrors = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "preprocessor", Subsystem: "processor", Name: "aggregation_errors_total",
			Help: "Total number of errors during OHLCV aggregation",
		})
		CandlesPublished = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "preprocessor", Subsystem: "processor", Name: "candles_published_total",
			Help: "Total number of aggregated candles published to Kafka",
		})
		PublishErrors = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "preprocessor", Subsystem: "processor", Name: "publish_errors_total",
			Help: "Total number of errors publishing candles to Kafka",
		})
		PublishLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "preprocessor", Subsystem: "processor", Name: "publish_latency_seconds",
			Help:    "Latency from aggregation to Kafka publish (seconds)",
			Buckets: prometheus.DefBuckets,
		})

		collectors := []prometheus.Collector{
			ConsumedMessages,
			AggregationErrors,
			CandlesPublished,
			PublishErrors,
			PublishLatency,
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
