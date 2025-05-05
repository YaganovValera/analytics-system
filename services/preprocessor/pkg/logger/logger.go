package logger

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type contextKey string

const (
	TraceIDKey   contextKey = "trace_id"
	RequestIDKey contextKey = "request_id"
)

// Config holds parameters for logger creation.
type Config struct {
	Level   string // e.g. "debug", "info"
	DevMode bool   // true → development config
}

// Logger wraps a *zap.Logger.
type Logger struct {
	raw *zap.Logger
}

// New creates a new Logger from Config.
// It builds the zap.Config, sets level, and skips one caller frame.
func New(cfg Config) (*Logger, error) {
	zapCfg := buildZapConfig(cfg.DevMode)
	if err := setZapLevel(&zapCfg, cfg.Level); err != nil {
		return nil, err
	}
	zl, err := zapCfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		return nil, err
	}
	return &Logger{raw: zl}, nil
}

// buildZapConfig returns a base zap.Config for dev or prod.
func buildZapConfig(dev bool) zap.Config {
	if dev {
		return zap.NewDevelopmentConfig()
	}
	prod := zap.NewProductionConfig()
	// sampling: log 1 in 100 after first 100 entries
	prod.Sampling = &zap.SamplingConfig{Initial: 100, Thereafter: 100}
	ec := &prod.EncoderConfig
	ec.TimeKey = "ts"
	ec.EncodeTime = zapcore.ISO8601TimeEncoder
	ec.CallerKey = "caller"
	ec.EncodeCaller = zapcore.ShortCallerEncoder
	ec.StacktraceKey = "stacktrace"
	return prod
}

// setZapLevel parses and applies the log level.
func setZapLevel(cfg *zap.Config, level string) error {
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return fmt.Errorf("invalid log level %q: %w", level, err)
	}
	cfg.Level = zap.NewAtomicLevelAt(lvl)
	return nil
}

// Sync flushes any buffered log entries.
func (l *Logger) Sync() {
	_ = l.raw.Sync()
}

// Named returns a sub-logger with the given name.
func (l *Logger) Named(name string) *Logger {
	return &Logger{raw: l.raw.Named(name)}
}

// WithContext annotates the logger with trace_id/request_id from ctx.
func (l *Logger) WithContext(ctx context.Context) *Logger {
	fields := []zap.Field{}
	if v := ctx.Value(TraceIDKey); v != nil {
		if tid, ok := v.(string); ok {
			fields = append(fields, zap.String(string(TraceIDKey), tid))
		}
	}
	if v := ctx.Value(RequestIDKey); v != nil {
		if rid, ok := v.(string); ok {
			fields = append(fields, zap.String(string(RequestIDKey), rid))
		}
	}
	if len(fields) > 0 {
		return &Logger{raw: l.raw.With(fields...)}
	}
	return l
}

// Info logs at Info level.
func (l *Logger) Info(msg string, fields ...zap.Field) {
	l.raw.Info(msg, fields...)
}

// Debug logs at Debug level.
func (l *Logger) Debug(msg string, fields ...zap.Field) {
	l.raw.Debug(msg, fields...)
}

// Warn logs at Warn level.
func (l *Logger) Warn(msg string, fields ...zap.Field) {
	l.raw.Warn(msg, fields...)
}

// Error logs at Error level.
func (l *Logger) Error(msg string, fields ...zap.Field) {
	l.raw.Error(msg, fields...)
}

// ContextWithTraceID returns a new context with trace ID set.
func ContextWithTraceID(ctx context.Context, tid string) context.Context {
	return context.WithValue(ctx, TraceIDKey, tid)
}

// ContextWithRequestID returns a new context with request ID set.
func ContextWithRequestID(ctx context.Context, rid string) context.Context {
	return context.WithValue(ctx, RequestIDKey, rid)
}
