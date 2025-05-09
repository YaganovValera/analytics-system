// common/logger/logger.go

package logger

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// -----------------------------------------------------------------------------
// context keys (неэкспортируемые)
// -----------------------------------------------------------------------------

type contextKey string

const (
	traceIDKey   contextKey = "trace_id"
	requestIDKey contextKey = "request_id"
)

// -----------------------------------------------------------------------------
// Configuration
// -----------------------------------------------------------------------------

// Config описывает, как инициализировать zap-логгер.
// Level   — "debug" | "info" | "warn" | "error" … (по умолчанию "info")
// DevMode — true → человекочитаемый консольный вывод, иначе JSON.
type Config struct {
	Level   string
	DevMode bool
}

func (c *Config) applyDefaults() {
	if c.Level == "" {
		c.Level = "info"
	}
}

func (c Config) validate() error {
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(c.Level)); err != nil {
		return fmt.Errorf("logger: invalid level %q: %w", c.Level, err)
	}
	return nil
}

// -----------------------------------------------------------------------------
// Logger wrapper
// -----------------------------------------------------------------------------

// Logger — тонкая обёртка над *zap.Logger.
type Logger struct {
	raw *zap.Logger
}

// New создаёт Logger по заданному Config.
func New(cfg Config) (*Logger, error) {
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	zapCfg := buildZapConfig(cfg.DevMode)
	if err := setZapLevel(&zapCfg, cfg.Level); err != nil {
		return nil, err
	}

	zl, err := zapCfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		return nil, fmt.Errorf("logger: build zap: %w", err)
	}
	return &Logger{raw: zl}, nil
}

func buildZapConfig(dev bool) zap.Config {
	if dev {
		// dev-режим: консольный вывод, но с едиными ключами, как в prod
		cfg := zap.NewDevelopmentConfig()
		ec := &cfg.EncoderConfig
		ec.TimeKey = "ts"
		ec.EncodeTime = zapcore.ISO8601TimeEncoder
		ec.CallerKey = "caller"
		ec.EncodeCaller = zapcore.ShortCallerEncoder
		return cfg
	}

	// prod-режим: JSON с семплингом
	prod := zap.NewProductionConfig()
	prod.Sampling = &zap.SamplingConfig{Initial: 100, Thereafter: 100}

	ec := &prod.EncoderConfig
	ec.TimeKey = "ts"
	ec.EncodeTime = zapcore.ISO8601TimeEncoder
	ec.CallerKey = "caller"
	ec.EncodeCaller = zapcore.ShortCallerEncoder
	ec.StacktraceKey = "stacktrace"

	return prod
}

func setZapLevel(cfg *zap.Config, level string) error {
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return err
	}
	cfg.Level = zap.NewAtomicLevelAt(lvl)
	return nil
}

// -----------------------------------------------------------------------------
// Public methods
// -----------------------------------------------------------------------------

// Sync сбрасывает все буферы (ошибки игнорируются).
func (l *Logger) Sync() { _ = l.raw.Sync() }

// Named создаёт sub-logger с префиксом.
func (l *Logger) Named(name string) *Logger {
	return &Logger{raw: l.raw.Named(name)}
}

// WithContext добавляет поля trace_id и request_id из контекста.
func (l *Logger) WithContext(ctx context.Context) *Logger {
	fields := make([]zap.Field, 0, 2)
	if v, ok := ctx.Value(traceIDKey).(string); ok {
		fields = append(fields, zap.String(string(traceIDKey), v))
	}
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		fields = append(fields, zap.String(string(requestIDKey), v))
	}
	if len(fields) == 0 {
		return l
	}
	return &Logger{raw: l.raw.With(fields...)}
}

// Sugar возвращает SugaredLogger для printf‑стиля.
func (l *Logger) Sugar() *zap.SugaredLogger {
	return l.raw.Sugar()
}

// Уровни
func (l *Logger) Debug(msg string, fields ...zap.Field) { l.raw.Debug(msg, fields...) }
func (l *Logger) Info(msg string, fields ...zap.Field)  { l.raw.Info(msg, fields...) }
func (l *Logger) Warn(msg string, fields ...zap.Field)  { l.raw.Warn(msg, fields...) }
func (l *Logger) Error(msg string, fields ...zap.Field) { l.raw.Error(msg, fields...) }

// -----------------------------------------------------------------------------
// Context helpers
// -----------------------------------------------------------------------------

// ContextWithTraceID возвращает новый контекст с trace-ID.
func ContextWithTraceID(ctx context.Context, tid string) context.Context {
	return context.WithValue(ctx, traceIDKey, tid)
}

// ContextWithRequestID возвращает новый контекст с request-ID.
func ContextWithRequestID(ctx context.Context, rid string) context.Context {
	return context.WithValue(ctx, requestIDKey, rid)
}
