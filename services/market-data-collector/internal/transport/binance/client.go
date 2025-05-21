// market-data-collector/internal/transport/binance/client.go
package binance

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/binance"
)

var tracer = otel.Tracer("collector/transport/binance")

// StreamWithMetrics wraps the raw connector with tracing and metrics.
func StreamWithMetrics(ctx context.Context, conn binance.Connector) (<-chan binance.RawMessage, error) {
	ctx, span := tracer.Start(ctx, "binance.stream")
	defer span.End()

	stream, err := conn.Stream(ctx)
	if err != nil {
		IncError("connect")
		span.RecordError(err)
		return nil, err
	}
	IncConnect("ok")

	out := make(chan binance.RawMessage, cap(stream))
	go func() {
		defer close(out)
		for msg := range stream {
			_, span := tracer.Start(ctx, "binance.read")
			span.SetAttributes(attribute.String("event_type", msg.Type))
			IncMessage(msg.Type)
			select {
			case out <- msg:
			default:
				IncDrop(msg.Type)
			}
			span.End()
		}
	}()
	return out, nil
}
