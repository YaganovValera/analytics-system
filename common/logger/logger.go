// common/logger/logger.go
//
// Пакет logger предоставляет тонкую обёртку над zap, унифицирующую:
//
//   - настройку уровней через конфиг;
//   - dev/prod-форматы вывода (JSON vs console);
//   - enrich-логи trace-/request-ID, прокидываемые через context;
//   - удобный вызов Sync() для сброса буферов при завершении.
//
// Использование:
//
//	cfg := logger.Config{Level: "debug", DevMode: true}
//	log, err := logger.New(cfg)
//	...
package logger

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// -----------------------------------------------------------------------------
// context keys
// -----------------------------------------------------------------------------

type contextKey string

const (
	// TraceIDKey — ключ для ID распределённого трейса.
	TraceIDKey contextKey = "trace_id"
	// RequestIDKey — ключ корреляции “запрос ↔ лог”.
	RequestIDKey contextKey = "request_id"
)

// -----------------------------------------------------------------------------
// Configuration
// -----------------------------------------------------------------------------

// Config описывает, как инициализировать zap-логгер.
//
// Level   — "debug" | "info" | "warn" | "error" … (по умолчанию "info")
// DevMode — true → цветная консоль в человекочитаемом формате.
type Config struct {
	Level   string
	DevMode bool
}

// applyDefaults подставляет безопасные значения.
func (c *Config) applyDefaults() {
	if c.Level == "" {
		c.Level = "info"
	}
}

// validate проверяет, что уровень распознан zapcore.
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

// Logger — thin-обёртка над *zap.Logger, скрывающая внутренности.
type Logger struct {
	raw *zap.Logger
}

// New создаёт Logger из Config.
// Скип caller-frame = 1, чтобы в логе показывалась линия вызова пакета-клиента.
func New(cfg Config) (*Logger, error) {
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	zapCfg := buildZapConfig(cfg.DevMode)

	// Применяем уровень
	if err := setZapLevel(&zapCfg, cfg.Level); err != nil {
		return nil, err // err уже обёрнут validate()
	}

	zl, err := zapCfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		return nil, fmt.Errorf("logger: build zap: %w", err)
	}
	return &Logger{raw: zl}, nil
}

// buildZapConfig формирует базовый zap.Config для dev или prod.
func buildZapConfig(dev bool) zap.Config {
	if dev {
		return zap.NewDevelopmentConfig()
	}

	prod := zap.NewProductionConfig()
	// Сэмплинг: первые 100 — все, далее 1 из 100.
	prod.Sampling = &zap.SamplingConfig{Initial: 100, Thereafter: 100}

	ec := &prod.EncoderConfig
	ec.TimeKey = "ts"
	ec.EncodeTime = zapcore.ISO8601TimeEncoder
	ec.CallerKey = "caller"
	ec.EncodeCaller = zapcore.ShortCallerEncoder
	ec.StacktraceKey = "stacktrace"

	return prod
}

// setZapLevel выставляет уровень логирования в cfg.
func setZapLevel(cfg *zap.Config, level string) error {
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return err // оборачивается на верхнем уровне
	}
	cfg.Level = zap.NewAtomicLevelAt(lvl)
	return nil
}

// -----------------------------------------------------------------------------
// Public methods
// -----------------------------------------------------------------------------

// Sync принудительно сбрасывает буферы zap; ошибки игнорируются.
func (l *Logger) Sync() { _ = l.raw.Sync() }

// Named создаёт sub-logger с добавленным именем.
func (l *Logger) Named(name string) *Logger { return &Logger{raw: l.raw.Named(name)} }

// WithContext аннотирует логгер полями trace_id / request_id из ctx.
func (l *Logger) WithContext(ctx context.Context) *Logger {
	fields := make([]zap.Field, 0, 2)
	if v, ok := ctx.Value(TraceIDKey).(string); ok {
		fields = append(fields, zap.String(string(TraceIDKey), v))
	}
	if v, ok := ctx.Value(RequestIDKey).(string); ok {
		fields = append(fields, zap.String(string(RequestIDKey), v))
	}
	if len(fields) == 0 {
		return l
	}
	return &Logger{raw: l.raw.With(fields...)}
}

// Sugar возвращает zap.SugaredLogger, если нужен быстрый printf-стиль.
func (l *Logger) Sugar() *zap.SugaredLogger { return l.raw.Sugar() }

// -----------------------------------------------------------------------------
// Shorthand log levels
// -----------------------------------------------------------------------------

func (l *Logger) Debug(msg string, fields ...zap.Field) { l.raw.Debug(msg, fields...) }
func (l *Logger) Info(msg string, fields ...zap.Field)  { l.raw.Info(msg, fields...) }
func (l *Logger) Warn(msg string, fields ...zap.Field)  { l.raw.Warn(msg, fields...) }
func (l *Logger) Error(msg string, fields ...zap.Field) { l.raw.Error(msg, fields...) }

// -----------------------------------------------------------------------------
// Context helpers
// -----------------------------------------------------------------------------

// ContextWithTraceID возвращает новый контекст с trace-ID.
func ContextWithTraceID(ctx context.Context, tid string) context.Context {
	return context.WithValue(ctx, TraceIDKey, tid)
}

// ContextWithRequestID возвращает новый контекст с request-ID.
func ContextWithRequestID(ctx context.Context, rid string) context.Context {
	return context.WithValue(ctx, RequestIDKey, rid)
}
