// analytics-api/internal/metrics/metrics.go
package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	once              sync.Once
	GRPCRequestsTotal *prometheus.CounterVec
	StreamEventsTotal *prometheus.CounterVec
	QueryLatency      *prometheus.HistogramVec
)

// Register инициализирует и регистрирует метрики analytics-api.
func Register(r prometheus.Registerer) {
	once.Do(func() {
		if r == nil {
			r = prometheus.DefaultRegisterer
		}

		GRPCRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "analytics", Subsystem: "grpc", Name: "requests_total",
			Help: "Total gRPC requests by method",
		}, []string{"method"})

		StreamEventsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "analytics", Subsystem: "stream", Name: "events_total",
			Help: "Total streamed candle events",
		}, []string{"interval"})

		QueryLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "analytics", Subsystem: "query", Name: "latency_seconds",
			Help:    "Latency of TimescaleDB queries",
			Buckets: prometheus.DefBuckets,
		}, []string{"interval"})

		collectors := []prometheus.Collector{
			GRPCRequestsTotal,
			StreamEventsTotal,
			QueryLatency,
		}
		for _, c := range collectors {
			_ = r.Register(c)
		}
	})
}
