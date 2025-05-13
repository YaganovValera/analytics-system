// common/kafka/interface.go
// Пакет kafka задаёт минимальные контракты обмена сообщениями, не тянет
// за собой Sarama и никак не зависит от конкретной реализации.
package kafka

import (
	"context"
	"time"
)

// Message представляет запись, полученную из Kafka.
type Message struct {
	Key       []byte            // ключ сообщения (может быть nil)
	Value     []byte            // полезная нагрузка
	Topic     string            // имя топика
	Partition int32             // раздел
	Offset    int64             // смещение
	Timestamp time.Time         // время публикации
	Headers   map[string][]byte // заголовки Kafka (если есть)
}

// Consumer описывает читателя одного или нескольких топиков.
//
// Consume(ctx, topics, handler) блокирует, пока:
//   - контекст не будет отменён;
//   - либо произойдёт невосстанавливаемая ошибка, которую метод вернёт.
//
// Для каждого сообщения вызывается handler; если handler возвращает ошибку,
// сообщение не коммитится (идея at-least-once, поведение реализации
// зависит от конкретного драйвера).
type Consumer interface {
	Consume(ctx context.Context, topics []string, handler func(msg *Message) error) error
	Close() error
}

// Producer публикует сообщения в Kafka.
type Producer interface {
	Publish(ctx context.Context, topic string, key, value []byte) error
	PublishMessage(ctx context.Context, msg *Message) error
	Ping(ctx context.Context) error
	Close() error
}
