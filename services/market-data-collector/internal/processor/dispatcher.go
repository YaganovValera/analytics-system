// github.com/YaganovValera/analytics-system/services/market-data-collector/internal/processor/dispatcher.go
package processor

import (
	"context"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/binance"

	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

var dispatcherTracer = otel.Tracer("collector/processor/dispatcher")

// DispatchStream читает сообщения из Binance WS и передаёт их процессору.
func DispatchStream(ctx context.Context, in <-chan binance.RawMessage, p Processor, log *zap.Logger) error {
	ctx, span := dispatcherTracer.Start(ctx, "DispatchStream")
	defer span.End()

	for msg := range in {
		if err := p.Process(ctx, msg); err != nil {
			log.Error("failed to process message",
				zap.String("type", msg.Type),
				zap.ByteString("raw", msg.Data),
				zap.Error(err),
			)
			metrics.PublishErrors.Inc()
		}
	}

	return nil
}
