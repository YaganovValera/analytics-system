// ---- common/service.go ----
package common

import (
	"sync"

	"github.com/YaganovValera/analytics-system/common/backoff"
	consumer "github.com/YaganovValera/analytics-system/common/kafka/consumer"
	producer "github.com/YaganovValera/analytics-system/common/kafka/producer"
)

// ServiceNameKey — ключ лейбла для метрик всех подсистем.
const ServiceNameKey = "service"

var (
	initServiceOnce sync.Once
)

// InitServiceName задаёт единое имя сервиса для backoff, Kafka‑producer и Kafka‑consumer.
// Вызывать в main() до любых отправок метрик/логов.
func InitServiceName(name string) {
	initServiceOnce.Do(func() {
		if name == "" {
			panic("common.InitServiceName: empty service name")
		}
		backoff.SetServiceLabel(name)
		producer.SetServiceLabel(name)
		consumer.SetServiceLabel(name)
	})
}
