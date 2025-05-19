// common/redis/metrics.go
package redis

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	hits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "common", Subsystem: "redis", Name: "hits_total",
			Help: "Successful Redis operations",
		},
		[]string{"service"},
	)
	fails = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "common", Subsystem: "redis", Name: "failures_total",
			Help: "Failed Redis operations",
		},
		[]string{"service"},
	)
)
