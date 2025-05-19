// common/logger/logger.go
package logger

import (
	"context"
	"fmt"

	"github.com/YaganovValera/analytics-system/common/ctxkeys"
	"go.uber.org/zap"
)

type Logger struct {
	raw *zap.Logger
}

func New(cfg Config) (*Logger, error) {
	zapCfg := buildZapConfig(cfg.DevMode, cfg.Format)
	if err := setZapLevel(&zapCfg, cfg.Level); err != nil {
		return nil, err
	}

	zl, err := zapCfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		return nil, fmt.Errorf("logger: build zap: %w", err)
	}
	return &Logger{raw: zl}, nil
}

func Init(level string, devMode bool) *Logger {
	log, err := New(Config{Level: level, DevMode: devMode})
	if err != nil {
		panic("logger.Init: " + err.Error())
	}
	return log
}

func (l *Logger) Sync() { _ = l.raw.Sync() }

func (l *Logger) Named(name string) *Logger {
	return &Logger{raw: l.raw.Named(name)}
}

func (l *Logger) WithContext(ctx context.Context) *Logger {
	fields := make([]zap.Field, 0, 2)
	if v, ok := ctx.Value(ctxkeys.TraceIDKey).(string); ok {
		fields = append(fields, zap.String("trace_id", v))
	}
	if v, ok := ctx.Value(ctxkeys.RequestIDKey).(string); ok {
		fields = append(fields, zap.String("request_id", v))
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

func (l *Logger) Sugar() *zap.SugaredLogger {
	return l.raw.Sugar()
}
