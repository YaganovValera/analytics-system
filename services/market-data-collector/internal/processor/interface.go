package processor

import (
	"context"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/binance"
)

// Processor определяет контракт на обработку сырых WS-сообщений.
type Processor interface {
	// Process разбирает одно сообщение и публикует результат в Kafka.
	Process(ctx context.Context, raw binance.RawMessage) error
}
