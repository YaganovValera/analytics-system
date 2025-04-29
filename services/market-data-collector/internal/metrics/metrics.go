package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	once sync.Once

	// EventsTotal — общее число принятых RawMessage из WebSocket.
	EventsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "collector",
		Subsystem: "ws",
		Name:      "events_total",
		Help:      "Total number of events received from WebSocket",
	})

	// PublishErrors — число ошибок при публикации сообщений в Kafka.
	PublishErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "collector",
		Subsystem: "kafka",
		Name:      "publish_errors_total",
		Help:      "Total number of errors when publishing to Kafka",
	})

	// BufferDrops — число сообщений, отброшенных из-за переполнения буфера.
	BufferDrops = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "collector",
		Subsystem: "ws",
		Name:      "buffer_drops_total",
		Help:      "Number of messages dropped because buffer was full",
	})

	// PublishLatency — гистограмма задержек от получения WS до публикации в Kafka.
	PublishLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "collector",
		Subsystem: "pipeline",
		Name:      "publish_latency_seconds",
		Help:      "Latency from receiving WS event to publishing to Kafka (seconds)",
		Buckets:   prometheus.DefBuckets,
	})
)

// Register регистрирует все метрики в заданном реестре.
// Можно вызвать без аргументов, чтобы зарегистрировать в DefaultRegisterer.
func Register(registerers ...prometheus.Registerer) {
	once.Do(func() {
		var reg prometheus.Registerer
		if len(registerers) > 0 && registerers[0] != nil {
			reg = registerers[0]
		} else {
			reg = prometheus.DefaultRegisterer
		}
		reg.MustRegister(
			EventsTotal,
			PublishErrors,
			BufferDrops,
			PublishLatency,
		)
	})
}
