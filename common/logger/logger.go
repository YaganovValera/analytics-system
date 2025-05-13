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
	traceIDKey   contextKey = "trace_id"
	requestIDKey contextKey = "request_id"
)

// -----------------------------------------------------------------------------
// Config & setup
// -----------------------------------------------------------------------------

// Config описывает, как инициализировать zap-логгер.
// Level   — debug | info | warn | error (по умолчанию info)
// DevMode — true → человекочитаемый вывод, иначе JSON.
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

// New создаёт новый Logger по заданному Config.
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

// Init — вынесенный «паникующий» конструктор для bootstrap.
// Если конфиг невалиден, приложение упадёт сразу (иначе дальше валидных логов не будет).
func Init(level string, devMode bool) *Logger {
	log, err := New(Config{Level: level, DevMode: devMode})
	if err != nil {
		panic("logger.Init: " + err.Error())
	}
	return log
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func buildZapConfig(dev bool) zap.Config {
	if dev {
		cfg := zap.NewDevelopmentConfig()
		ec := &cfg.EncoderConfig
		ec.TimeKey = "ts"
		ec.EncodeTime = zapcore.ISO8601TimeEncoder
		ec.CallerKey = "caller"
		ec.EncodeCaller = zapcore.ShortCallerEncoder
		return cfg
	}

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
// Context helpers & methods
// -----------------------------------------------------------------------------

func (l *Logger) Sync() { _ = l.raw.Sync() }
func (l *Logger) Named(name string) *Logger {
	return &Logger{raw: l.raw.Named(name)}
}

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

func (l *Logger) Debug(msg string, fields ...zap.Field) { l.raw.Debug(msg, fields...) }
func (l *Logger) Info(msg string, fields ...zap.Field)  { l.raw.Info(msg, fields...) }
func (l *Logger) Warn(msg string, fields ...zap.Field)  { l.raw.Warn(msg, fields...) }
func (l *Logger) Error(msg string, fields ...zap.Field) { l.raw.Error(msg, fields...) }

func ContextWithTraceID(ctx context.Context, tid string) context.Context {
	return context.WithValue(ctx, traceIDKey, tid)
}
func ContextWithRequestID(ctx context.Context, rid string) context.Context {
	return context.WithValue(ctx, requestIDKey, rid)
}
