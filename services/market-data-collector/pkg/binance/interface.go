// services/market-data-collector/pkg/binance/interface.go
package binance

import "context"

// Connector описывает контракт WebSocket-коннектора Binance.
type Connector interface {
	// Stream запускает горутину чтения и возвращает канал RawMessage.
	// Закрытие происходит при отмене ctx или вызове Close.
	Stream(ctx context.Context) (<-chan RawMessage, error)
	// Close освобождает все ресурсы. После этого Stream больше не работает.
	Close() error
}
