// common/service.go
package common

import (
	"github.com/YaganovValera/analytics-system/common/backoff"
	consumer "github.com/YaganovValera/analytics-system/common/kafka/consumer"
	producer "github.com/YaganovValera/analytics-system/common/kafka/producer"
)

// ServiceNameKey — ключ лейбла для метрик всех подсистем.
const ServiceNameKey = "service"

// InitServiceName задаёт единое имя сервиса для backoff, Kafka-producer и Kafka-consumer.
// Нужно вызывать в main() до любых попыток логирования или отправки метрик.
func InitServiceName(name string) {
	backoff.SetServiceLabel(name)
	producer.SetServiceLabel(name)
	consumer.SetServiceLabel(name)
}
