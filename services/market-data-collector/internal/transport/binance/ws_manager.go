// market-data-collector/internal/transport/binance/ws_manager.go

package binance

import (
	"context"
	"sync"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/binance"
)

// WSManager управляет текущим WebSocket подключением.
type WSManager struct {
	conn binance.Connector

	mu       sync.Mutex
	cancelFn context.CancelFunc
}

func NewWSManager(conn binance.Connector) *WSManager {
	return &WSManager{conn: conn}
}

// Start запускает StreamWithMetrics с управляемым контекстом.
func (w *WSManager) Start(ctx context.Context) (<-chan binance.RawMessage, func(), error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Завершаем предыдущий, если был
	if w.cancelFn != nil {
		w.cancelFn()
	}

	streamCtx, cancel := context.WithCancel(ctx)
	w.cancelFn = cancel

	msgCh, err := StreamWithMetrics(streamCtx, w.conn)
	if err != nil {
		cancel()
		w.cancelFn = nil
		return nil, nil, err
	}

	return msgCh, cancel, nil
}

// Stop завершает текущий поток.
func (w *WSManager) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cancelFn != nil {
		w.cancelFn()
		w.cancelFn = nil
	}
}
