// github.com/YaganovValera/analytics-system/services/market-data-collector/internal/processor/dispatcher.go
package processor

import (
	"context"

	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/binance"
	"go.uber.org/zap"

	"go.opentelemetry.io/otel"
)

var dispatcherTracer = otel.Tracer("collector/processor/dispatcher")

// DispatchRouter маршрутизирует входящие сообщения по типу события.
type DispatchRouter struct {
	processors map[string]Processor
	log        *logger.Logger
}

// NewRouter создает маршрутизатор с логгером.
func NewRouter(log *logger.Logger) *DispatchRouter {
	return &DispatchRouter{
		processors: make(map[string]Processor),
		log:        log.Named("router"),
	}
}

// Register добавляет обработчик для заданного типа событий.
func (r *DispatchRouter) Register(eventType string, proc Processor) {
	r.processors[eventType] = proc
}

// Run запускает основной цикл маршрутизации сообщений.
func (r *DispatchRouter) Run(ctx context.Context, in <-chan binance.RawMessage) error {
	ctx, span := dispatcherTracer.Start(ctx, "DispatchRouter.Run")
	defer span.End()

	for msg := range in {
		evtType := msg.Type
		proc, ok := r.processors[evtType]
		if !ok {
			r.log.WithContext(ctx).Debug("unsupported event type",
				zap.String("event_type", evtType),
			)
			continue
		}

		err := proc.Process(ctx, msg)
		if err != nil {
			r.log.WithContext(ctx).Error("event processing failed",
				zap.String("event_type", evtType),
				zap.Error(err),
			)
			metrics.PublishErrors.Inc()
		}
	}

	return nil
}
