// common/telemetry/otel.go
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
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/YaganovValera/analytics-system/common/logger"
)

// Config содержит параметры для инициализации OpenTelemetry.
type Config struct {
	Endpoint        string        // OTLP-collector "host:port"
	ServiceName     string        // имя сервиса
	ServiceVersion  string        // версия сборки
	Insecure        bool          // true → gRPC без TLS
	ReconnectPeriod time.Duration // период ребута экспортёра
	Timeout         time.Duration // таймаут Init/Shutdown
	SamplerRatio    float64       // 0.0…1.0 — доля выборки span'ов
}

func applyDefaults(cfg *Config) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.ReconnectPeriod <= 0 {
		cfg.ReconnectPeriod = 5 * time.Second
	}
	if cfg.SamplerRatio < 0 || cfg.SamplerRatio > 1 {
		cfg.SamplerRatio = 1
	}
}

func validateConfig(cfg Config) error {
	switch {
	case cfg.Endpoint == "":
		return fmt.Errorf("telemetry: endpoint is required")
	case cfg.ServiceName == "":
		return fmt.Errorf("telemetry: service name is required")
	case cfg.ServiceVersion == "":
		return fmt.Errorf("telemetry: service version is required")
	case cfg.SamplerRatio < 0 || cfg.SamplerRatio > 1:
		return fmt.Errorf("telemetry: sampler ratio must be between 0.0 and 1.0, got %v", cfg.SamplerRatio)
	default:
		return nil
	}
}

// InitTracer инициализирует глобальный TracerProvider и возвращает Shutdown-функцию.
func InitTracer(ctx context.Context, cfg Config, log *logger.Logger) (func(context.Context) error, error) {
	applyDefaults(&cfg)
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

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

func Tracer() trace.Tracer {
	return otel.Tracer("default")
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
