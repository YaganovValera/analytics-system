// github.com/YaganovValera/analytics-system/services/market-data-collector/internal/transport/binance/metrics.go
package binance

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	once          sync.Once
	wsConnects    *prometheus.CounterVec
	wsErrors      *prometheus.CounterVec
	wsMessages    *prometheus.CounterVec
	wsBufferDrops *prometheus.CounterVec
)

func RegisterMetrics(r prometheus.Registerer) {
	once.Do(func() {
		wsConnects = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "collector", Subsystem: "binance", Name: "connects_total",
			Help: "Total WebSocket connection attempts",
		}, []string{"status"})

		wsErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "collector", Subsystem: "binance", Name: "errors_total",
			Help: "Total categorized WebSocket errors",
		}, []string{"type"})

		wsMessages = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "collector", Subsystem: "binance", Name: "messages_total",
			Help: "Total messages received from Binance WS",
		}, []string{"type"})

		wsBufferDrops = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "collector", Subsystem: "binance", Name: "buffer_drops_total",
			Help: "Messages dropped due to full buffer",
		}, []string{"type"})

		collectors := []prometheus.Collector{wsConnects, wsErrors, wsMessages, wsBufferDrops}
		for _, c := range collectors {
			_ = r.Register(c)
		}
	})
}

func IncConnect(status string)  { wsConnects.WithLabelValues(status).Inc() }
func IncError(errType string)   { wsErrors.WithLabelValues(errType).Inc() }
func IncMessage(msgType string) { wsMessages.WithLabelValues(msgType).Inc() }
func IncDrop(msgType string)    { wsBufferDrops.WithLabelValues(msgType).Inc() }
