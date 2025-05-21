package prometheus

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	DefaultRegistry = prometheus.DefaultRegisterer

	DefaultGatherer = prometheus.DefaultGatherer
)

func Handler() http.Handler {
	return promhttp.HandlerFor(DefaultGatherer, promhttp.HandlerOpts{})
}

func Register(c prometheus.Collector) {
	DefaultRegistry.MustRegister(c)
}

func MustRegisterMany(cs ...prometheus.Collector) {
	for _, c := range cs {
		DefaultRegistry.MustRegister(c)
	}
}
