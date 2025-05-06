// pkg/telemetry/otel.go
package telemetry

import (
	"context"
	"fmt"
	"time"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.uber.org/zap"
)

// Config holds settings for OpenTelemetry initialization.
type Config struct {
	Endpoint        string        // OTLP collector (host:port)
	ServiceName     string        // e.g. "market-data-collector"
	ServiceVersion  string        // e.g. "v1.0.0"
	Insecure        bool          // true = no TLS
	ReconnectPeriod time.Duration // gRPC reconnection period
	Timeout         time.Duration // timeout for init & shutdown
	SamplerRatio    float64       // [0.0 – 1.0]
}

// InitTracer configures global TracerProvider and returns a shutdown function.
func InitTracer(ctx context.Context, cfg Config, log *logger.Logger) (func(context.Context) error, error) {
	applyDefaults(&cfg)
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	// Use timeout for exporter creation
	initCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	exp, err := newExporter(initCtx, cfg)
	if err != nil {
		log.Error("telemetry: exporter creation failed", zap.Error(err))
		return nil, fmt.Errorf("telemetry exporter: %w", err)
	}

	res, err := newResource(cfg)
	if err != nil {
		log.Error("telemetry: resource creation failed", zap.Error(err))
		return nil, fmt.Errorf("telemetry resource: %w", err)
	}

	tp := newTracerProvider(exp, res, cfg)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{}, propagation.Baggage{},
		),
	)

	log.Info("telemetry: initialized",
		zap.String("endpoint", cfg.Endpoint),
		zap.String("service", cfg.ServiceName),
		zap.String("version", cfg.ServiceVersion),
		zap.Float64("sampler_ratio", cfg.SamplerRatio),
	)

	// shutdown func for graceful exit
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

func applyDefaults(cfg *Config) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.ReconnectPeriod <= 0 {
		cfg.ReconnectPeriod = 5 * time.Second
	}
	if cfg.SamplerRatio <= 0 || cfg.SamplerRatio > 1 {
		cfg.SamplerRatio = 1.0
	}
}

func validateConfig(cfg Config) error {
	if cfg.Endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}
	if cfg.ServiceName == "" {
		return fmt.Errorf("service name is required")
	}
	if cfg.ServiceVersion == "" {
		return fmt.Errorf("service version is required")
	}
	return nil
}

// newExporter returns a sdktrace.SpanExporter (not otlptracegrpc.Exporter).
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
