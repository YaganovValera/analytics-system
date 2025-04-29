// internal/telemetry/otel.go
package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/logger"
)

// InitTracer настраивает глобальный TracerProvider с OTLP/gRPC-экспортером.
// collectorEndpoint — адрес OTLP-коллектора (host:port), serviceName — имя сервиса,
// serviceVersion — версия сервиса (например, из config или сборки),
// insecure — если true, не использовать TLS,
// log — ваш обёрнутый логгер.
// Возвращает функцию shutDown, которую нужно вызвать при graceful-shutdown.
func InitTracer(
	ctx context.Context,
	collectorEndpoint, serviceName, serviceVersion string,
	insecure bool,
	log *logger.Logger,
) (shutdown func(context.Context) error, err error) {
	// 1) Валидация входных параметров
	if collectorEndpoint == "" {
		return nil, fmt.Errorf("telemetry: collectorEndpoint is required")
	}
	if serviceName == "" {
		return nil, fmt.Errorf("telemetry: serviceName is required")
	}
	// 2) Контекст с таймаутом для создания экспортёра
	initCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 3) Настройка экспортёра
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(collectorEndpoint),
		otlptracegrpc.WithReconnectionPeriod(5 * time.Second),
	}
	if insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	exporter, err := otlptracegrpc.New(initCtx, opts...)
	if err != nil {
		log.Sugar().Errorw("telemetry: cannot create OTLP exporter", "error", err)
		return nil, fmt.Errorf("telemetry: cannot create OTLP exporter: %w", err)
	}

	// 4) Ресурс с service.name и service.version
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(serviceVersion),
		),
	)
	if err != nil {
		log.Sugar().Errorw("telemetry: cannot create resource", "error", err)
		return nil, fmt.Errorf("telemetry: cannot create resource: %w", err)
	}

	// 5) Создаём TracerProvider с ParentBased sampler и батчевым экспортёром
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())), // можно заменить на TraceIDRatioBased
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	// 6) Устанавливаем глобально TracerProvider и CompositePropagator
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	log.Sugar().Infow("telemetry: tracer initialized",
		"endpoint", collectorEndpoint,
		"service", serviceName,
		"version", serviceVersion,
	)

	// 7) Функция graceful shutdown
	shutdown = func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			log.Sugar().Errorw("telemetry: tracer shutdown failed", "error", err)
			return err
		}
		log.Sugar().Infow("telemetry: tracer shutdown complete")
		return nil
	}
	return shutdown, nil
}
