// pkg/logger/logger.go
package logger

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// contextKey — тип ключа для context.Value, чтобы избежать коллизий.
type contextKey string

const (
	// TraceIDKey используется для хранения trace ID в контексте.
	TraceIDKey contextKey = "trace_id"
	// RequestIDKey используется для хранения request ID в контексте.
	RequestIDKey contextKey = "request_id"
)

// Logger объединяет *zap.Logger и *zap.SugaredLogger,
// а также обеспечивает метод Sync().
type Logger struct {
	raw   *zap.Logger
	sugar *zap.SugaredLogger
}

// New создаёт Logger с заданным уровнем и режимом.
// При завершении работы приложения обязательно вызовите logger.Sync().
func New(level string, devMode bool) (*Logger, error) {
	// 1. Настройка базового конфига.
	var cfg zap.Config
	if devMode {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
		cfg.Sampling = &zap.SamplingConfig{Initial: 100, Thereafter: 100}
	}

	// 2. Разбор уровня логирования.
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", level, err)
	}
	cfg.Level = zap.NewAtomicLevelAt(lvl)

	// 3. Форматирование вывода.
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.CallerKey = "caller"
	cfg.EncoderConfig.StacktraceKey = "stacktrace"

	// 4. Сборка логгера (skip один уровень вызова для корректного caller).
	raw, err := cfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		return nil, fmt.Errorf("failed to build logger: %w", err)
	}

	return &Logger{
		raw:   raw,
		sugar: raw.Sugar(),
	}, nil
}

// Sugar возвращает *zap.SugaredLogger.
func (l *Logger) Sugar() *zap.SugaredLogger {
	return l.sugar
}

// Sync сбрасывает буферизированные записи. Вызывать перед выходом.
func (l *Logger) Sync() error {
	return l.raw.Sync()
}

// Named создаёт новый логгер с namespace-приставкой.
func (l *Logger) Named(name string) *Logger {
	rawN := l.raw.Named(name)
	return &Logger{
		raw:   rawN,
		sugar: rawN.Sugar(),
	}
}

// WithContext возвращает *zap.SugaredLogger с полями trace_id и request_id,
// если они присутствуют в ctx.
func (l *Logger) WithContext(ctx context.Context) *zap.SugaredLogger {
	fields := make([]interface{}, 0, 2)
	if tid := ctx.Value(TraceIDKey); tid != nil {
		fields = append(fields, "trace_id", tid)
	}
	if rid := ctx.Value(RequestIDKey); rid != nil {
		fields = append(fields, "request_id", rid)
	}
	if len(fields) > 0 {
		return l.sugar.With(fields...)
	}
	return l.sugar
}

// ContextWithTraceID возвращает новый контекст с заданным trace ID.
func ContextWithTraceID(ctx context.Context, tid string) context.Context {
	return context.WithValue(ctx, TraceIDKey, tid)
}

// ContextWithRequestID возвращает новый контекст с заданным request ID.
func ContextWithRequestID(ctx context.Context, rid string) context.Context {
	return context.WithValue(ctx, RequestIDKey, rid)
}
