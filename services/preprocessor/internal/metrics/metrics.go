// preprocessor/internal/metrics/metrics.go

package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	once               sync.Once
	ProcessedTotal     *prometheus.CounterVec
	FlushedTotal       *prometheus.CounterVec
	FlushLatency       *prometheus.HistogramVec
	RestoreErrorsTotal *prometheus.CounterVec
)

// Register инициализирует и регистрирует все метрики агрегатора.
func Register(r prometheus.Registerer) {
	once.Do(func() {
		if r == nil {
			r = prometheus.DefaultRegisterer
		}

		ProcessedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "preprocessor", Subsystem: "aggregator", Name: "processed_total",
			Help: "Total MarketData ticks processed",
		}, []string{"interval"})

		FlushedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "preprocessor", Subsystem: "aggregator", Name: "flushed_total",
			Help: "Total finalized candles flushed",
		}, []string{"interval"})

		FlushLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "preprocessor", Subsystem: "aggregator", Name: "flush_latency_seconds",
			Help:    "Time between last tick and flush",
			Buckets: prometheus.DefBuckets,
		}, []string{"interval"})

		RestoreErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "preprocessor", Subsystem: "aggregator", Name: "restore_errors_total",
			Help: "Number of failed partial bar restore attempts",
		}, []string{"interval"})

		collectors := []prometheus.Collector{
			ProcessedTotal,
			FlushedTotal,
			FlushLatency,
			RestoreErrorsTotal,
		}
		for _, c := range collectors {
			if err := r.Register(c); err != nil {
				// игнорируем попытку повторной регистрации
				if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
					panic(err)
				}
			}
		}
	})
}
