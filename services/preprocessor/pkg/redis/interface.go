package redis

import (
	"context"
)

// Storage описывает контракт хранилища состояния незавершённых баров.
type Storage interface {
	// Get возвращает значение по ключу или ErrNotFound, если нет.
	// Ошибки соединения и пр. возвращаются как error.
	Get(ctx context.Context, key string) ([]byte, error)
	// Set сохраняет значение по ключу с TTL.
	Set(ctx context.Context, key string, value []byte) error
	// Delete удаляет ключ.
	Delete(ctx context.Context, key string) error
	// Close закрывает клиент и освобождает ресурсы.
	Close() error
}
