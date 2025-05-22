// query-service/internal/metrics/metrics.go
package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	once sync.Once

	GRPCRequestsTotal *prometheus.CounterVec
	QuerySuccessTotal *prometheus.CounterVec
	QueryErrorsTotal  *prometheus.CounterVec
	QueryLatency      *prometheus.HistogramVec
	QueryRowsReturned *prometheus.HistogramVec
)

// Register инициализирует и регистрирует все метрики сервиса query-service.
func Register(r prometheus.Registerer) {
	once.Do(func() {
		if r == nil {
			r = prometheus.DefaultRegisterer
		}

		GRPCRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "query", Subsystem: "grpc", Name: "requests_total",
			Help: "Total gRPC requests by method",
		}, []string{"method"})

		QuerySuccessTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "query", Subsystem: "core", Name: "success_total",
			Help: "Total successful SELECT queries",
		}, []string{"source"})

		QueryErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "query", Subsystem: "core", Name: "errors_total",
			Help: "Total SQL query failures",
		}, []string{"type", "source"})

		QueryLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "query", Subsystem: "core", Name: "latency_seconds",
			Help:    "Latency of SELECT query execution",
			Buckets: prometheus.DefBuckets,
		}, []string{"source"})

		QueryRowsReturned = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "query", Subsystem: "core", Name: "rows_returned",
			Help:    "Number of rows returned from SQL query",
			Buckets: []float64{0, 1, 10, 100, 1000, 10000},
		}, []string{"source"})

		collectors := []prometheus.Collector{
			GRPCRequestsTotal,
			QuerySuccessTotal,
			QueryErrorsTotal,
			QueryLatency,
			QueryRowsReturned,
		}

		for _, c := range collectors {
			_ = r.Register(c)
		}
	})
}
