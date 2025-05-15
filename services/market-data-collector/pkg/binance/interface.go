// github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/binance/interface.go
package binance

import "context"

// Connector describes the low-level Binance WebSocket connector.
type Connector interface {
	Stream(ctx context.Context) (<-chan RawMessage, error)
	Close() error
}

// RawMessage represents a parsed WebSocket event.
type RawMessage struct {
	Data []byte // JSON event payload
	Type string // e.g. trade, depthUpdate, kline, etc.
}
