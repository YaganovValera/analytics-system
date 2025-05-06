// pkg/logger/logger_test.go
package logger_test

import (
	"context"
	"testing"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/logger"
)

func TestNew_InvalidLevel(t *testing.T) {
	_, err := logger.New(logger.Config{Level: "invalid", DevMode: false})
	if err == nil {
		t.Error("expected error for invalid level, got nil")
	}
}

func TestNew_ValidLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}
	for _, lvl := range levels {
		_, err := logger.New(logger.Config{Level: lvl, DevMode: true})
		if err != nil {
			t.Errorf("expected no error for level %s, got %v", lvl, err)
		}
	}
}

func TestWithContext_TraceAndRequestID(t *testing.T) {
	raw, _ := logger.New(logger.Config{Level: "info", DevMode: true})
	ctx := context.Background()
	ctx = logger.ContextWithTraceID(ctx, "trace-123")
	ctx = logger.ContextWithRequestID(ctx, "req-456")
	enh := raw.WithContext(ctx)
	// ensure the returned logger is non-nil and methods don't panic
	enh.Info("test message")
}

func TestSync_NoPanic(t *testing.T) {
	l, _ := logger.New(logger.Config{Level: "info", DevMode: true})
	// Sync should not panic
	l.Sync()
}
