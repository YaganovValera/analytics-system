// services/market-data-collector/pkg/kafka/interface.go
package kafka

import "context"

// Producer описывает контракт для публикации сообщений в Kafka
// и проверки живости клиента.
type Producer interface {
	// Publish публикует сообщение в заданный топик.
	Publish(ctx context.Context, topic string, key, value []byte) error
	// Ping проверяет, что Kafka доступна (refresh metadata).
	Ping() error
	// Close корректно закрывает продьюсер и клиент.
	Close() error
}
