// common/logger/zap_config.go
package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

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
