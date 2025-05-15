// github.com/YaganovValera/analytics-system/services/market-data-collector/internal/processor/interface.go
package processor

import (
	"context"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/binance"
)

// Processor defines the interface for message handlers.
type Processor interface {
	Process(ctx context.Context, raw binance.RawMessage) error
}
