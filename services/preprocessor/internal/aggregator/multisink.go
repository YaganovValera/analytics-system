// github.com/YaganovValera/analytics-system/services/preprocessor/internal/aggregator/multisink.go

package aggregator

import (
	"context"
	"fmt"
)

// MultiSink вызывает FlushCandle у всех вложенных sinks.
type MultiSink struct {
	sinks []FlushSink
}

// NewMultiSink создаёт обёртку над несколькими sinks.
func NewMultiSink(sinks ...FlushSink) FlushSink {
	return &MultiSink{sinks: sinks}
}

// FlushCandle передаёт свечу во все sinks.
func (m *MultiSink) FlushCandle(ctx context.Context, candle *Candle) error {
	var firstErr error
	for _, sink := range m.sinks {
		if err := sink.FlushCandle(ctx, candle); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if firstErr != nil {
		return fmt.Errorf("multisink: flush failed: %w", firstErr)
	}
	return nil
}
