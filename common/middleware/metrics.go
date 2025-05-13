package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	reqs = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "http",
			Name:      "requests_total",
			Help:      "Total HTTP requests",
		},
		[]string{"path", "method", "code"},
	)
	duration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "http",
			Name:      "request_duration_seconds",
			Help:      "Request duration",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"path", "method"},
	)
)

func init() {
	prometheus.MustRegister(reqs, duration)
}

func Metrics() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, status: 200}
			next.ServeHTTP(rw, r)

			reqs.WithLabelValues(r.URL.Path, r.Method, strconv.Itoa(rw.status)).Inc()
			duration.WithLabelValues(r.URL.Path, r.Method).Observe(time.Since(start).Seconds())
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
