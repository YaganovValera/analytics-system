// auth/internal/metrics/metrics.go
package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	once sync.Once

	GRPCRequestsTotal *prometheus.CounterVec
	LoginTotal        *prometheus.CounterVec
	RefreshTotal      *prometheus.CounterVec
	ValidateTotal     *prometheus.CounterVec
	RevokeTotal       *prometheus.CounterVec
	LogoutTotal       *prometheus.CounterVec
	IssuedTokens      *prometheus.CounterVec
)

func Register(r prometheus.Registerer) {
	once.Do(func() {
		if r == nil {
			r = prometheus.DefaultRegisterer
		}

		GRPCRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "auth", Subsystem: "grpc", Name: "requests_total",
			Help: "Total gRPC requests by method",
		}, []string{"method"})

		LoginTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "auth", Subsystem: "login", Name: "total",
			Help: "Total login attempts by status",
		}, []string{"status"})

		RefreshTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "auth", Subsystem: "refresh", Name: "total",
			Help: "Total refresh token operations by result",
		}, []string{"result"})

		ValidateTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "auth", Subsystem: "validate", Name: "total",
			Help: "Token validation attempts by result",
		}, []string{"result"})

		RevokeTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "auth", Subsystem: "revoke", Name: "total",
			Help: "Total revocations by result",
		}, []string{"result"})

		LogoutTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "auth", Subsystem: "logout", Name: "total",
			Help: "Total logouts",
		}, []string{})

		IssuedTokens = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "auth", Subsystem: "token", Name: "issued_total",
			Help: "Total issued tokens by type",
		}, []string{"type"})

		collectors := []prometheus.Collector{
			GRPCRequestsTotal, LoginTotal, RefreshTotal, ValidateTotal,
			RevokeTotal, LogoutTotal, IssuedTokens,
		}
		for _, c := range collectors {
			_ = r.Register(c)
		}
	})
}
