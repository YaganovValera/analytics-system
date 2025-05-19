package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/YaganovValera/analytics-system/common/logger"
)

var tracerProvider *sdktrace.TracerProvider

// InitTracer инициализирует глобальный TracerProvider и возвращает Shutdown-функцию.
func InitTracer(ctx context.Context, cfg Config, log *logger.Logger) (func(context.Context) error, error) {
	initCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	exp, err := newExporter(initCtx, cfg)
	if err != nil {
		log.Error("telemetry: exporter creation failed", zap.Error(err), zap.Any("config", cfg))
		return nil, fmt.Errorf("telemetry: exporter: %w", err)
	}

	res, err := newResource(cfg)
	if err != nil {
		log.Error("telemetry: resource creation failed", zap.Error(err), zap.Any("config", cfg))
		return nil, fmt.Errorf("telemetry: resource: %w", err)
	}

	tp := newTracerProvider(exp, res, cfg)
	tracerProvider = tp
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	log.Info("telemetry: initialized",
		zap.String("service", cfg.ServiceName),
		zap.String("version", cfg.ServiceVersion),
	)

	return func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			log.Error("telemetry: shutdown failed", zap.Error(err))
			return err
		}
		log.Info("telemetry: shutdown complete")
		return nil
	}, nil
}

// TracerFor возвращает именованный OpenTelemetry tracer.
func TracerFor(name string) trace.Tracer {
	if tracerProvider == nil {
		return otel.Tracer(name) // fallback
	}
	return tracerProvider.Tracer(name)
}

func newExporter(ctx context.Context, cfg Config) (sdktrace.SpanExporter, error) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
		otlptracegrpc.WithReconnectionPeriod(cfg.ReconnectPeriod),
	}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	return otlptracegrpc.New(ctx, opts...)
}

func newResource(cfg Config) (*resource.Resource, error) {
	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceVersionKey.String(cfg.ServiceVersion),
		),
	)
}

func newTracerProvider(exp sdktrace.SpanExporter, res *resource.Resource, cfg Config) *sdktrace.TracerProvider {
	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SamplerRatio))
	return sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sampler),
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
}
