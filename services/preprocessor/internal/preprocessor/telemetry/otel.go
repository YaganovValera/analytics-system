// market-data-collector/internal/telemetry/otel.go
package telemetry

import (
	"context"
	"log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"google.golang.org/grpc"
)

// InitTracer настраивает OTLP-трассировщик и устанавливает его глобально.
// Возвращает функцию для корректного Shutdown.
func InitTracer(ctx context.Context, collectorEndpoint string, serviceName string) (func(context.Context) error, error) {
	// Создаём OTLP GRPC-экспортер
	clientOpts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(collectorEndpoint),
		otlptracegrpc.WithDialOption(grpc.WithBlock()),
	}
	exporter, err := otlptracegrpc.New(ctx, clientOpts...)
	if err != nil {
		return nil, err
	}

	// Ресурс с атрибутами сервиса
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, err
	}

	// TracerProvider с BatchSpanProcessor
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	// Возвращаем функцию для остановки
	shutdown := func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, time.Second*5)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
			return err
		}
		return nil
	}
	return shutdown, nil
}
