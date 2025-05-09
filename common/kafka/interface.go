// common/kafka/interface.go
//
// Пакет kafka задаёт минимальные контракты обмена сообщениями, не тянет
// за собой Sarama и никак не зависит от конкретной реализации.
package kafka

import "context"

// Message представляет запись, полученную из Kafka.
type Message struct {
	Key       []byte // ключ сообщения (может быть nil)
	Value     []byte // полезная нагрузка
	Topic     string // имя топика
	Partition int32  // раздел
	Offset    int64  // смещение
}

// Consumer описывает читателя одного или нескольких топиков.
//
//	Consume(ctx, topics, handler) блокирует, пока:
//	  • контекст не будет отменён;
//	  • либо произойдёт невосстанавливаемая ошибка, которую метод вернёт.
//	Для каждого сообщения вызывается handler; если handler возвращает ошибку,
//	сообщение не коммитится (идея at-least-once, поведение реализации
//	зависит от конкретного драйвера).
type Consumer interface {
	Consume(ctx context.Context, topics []string, handler func(msg *Message) error) error
	Close() error
}

// Producer публикует сообщения в Kafka.
type Producer interface {
	// Publish гарантирует, что сообщение будет доставлено согласно политике
	// RequiredAcks; возможен внутренний retry согласно стратегии back-off.
	Publish(ctx context.Context, topic string, key, value []byte) error
	// Ping проверяет достижимость кластера (обновление метаданных).
	Ping(ctx context.Context) error
	Close() error
}
