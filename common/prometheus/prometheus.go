package prometheus

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// DefaultRegistry — стандартный глобальный реестр метрик.
	DefaultRegistry = prometheus.DefaultRegisterer

	// DefaultGatherer используется promhttp.Handler'ом.
	DefaultGatherer = prometheus.DefaultGatherer
)

// Handler возвращает HTTP-обработчик для /metrics.
// Обычно подключается в common/httpserver по пути из конфига.
func Handler() http.Handler {
	return promhttp.HandlerFor(DefaultGatherer, promhttp.HandlerOpts{})
}

// Register удобный алиас для регистрации метрик в дефолтный реестр.
// Используется из init() разных пакетов.
func Register(c prometheus.Collector) {
	DefaultRegistry.MustRegister(c)
}

// MustRegisterMany — позволяет регистрировать несколько метрик одной строкой.
func MustRegisterMany(cs ...prometheus.Collector) {
	for _, c := range cs {
		DefaultRegistry.MustRegister(c)
	}
}
