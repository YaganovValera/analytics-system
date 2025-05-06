// pkg/telemetry/otel_test.go
package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/logger"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name       string
		cfg        Config
		expectsErr bool
	}{
		{"missing endpoint", Config{ServiceName: "svc", ServiceVersion: "v1"}, true},
		{"missing serviceName", Config{Endpoint: "host:1234", ServiceVersion: "v1"}, true},
		{"missing version", Config{Endpoint: "host:1234", ServiceName: "svc"}, true},
		{"all set", Config{Endpoint: "host:1234", ServiceName: "svc", ServiceVersion: "v1"}, false},
	}
	for _, tc := range tests {
		err := validateConfig(tc.cfg)
		if tc.expectsErr && err == nil {
			t.Errorf("%s: expected error, got nil", tc.name)
		}
		if !tc.expectsErr && err != nil {
			t.Errorf("%s: expected no error, got %v", tc.name, err)
		}
	}
}

func TestApplyDefaults(t *testing.T) {
	cfg := Config{Endpoint: "e", ServiceName: "s", ServiceVersion: "v"}
	applyDefaults(&cfg)
	if cfg.Timeout != 5*time.Second {
		t.Errorf("expected default Timeout=5s, got %v", cfg.Timeout)
	}
	if cfg.ReconnectPeriod != 5*time.Second {
		t.Errorf("expected default ReconnectPeriod=5s, got %v", cfg.ReconnectPeriod)
	}
	if cfg.SamplerRatio != 1.0 {
		t.Errorf("expected default SamplerRatio=1.0, got %v", cfg.SamplerRatio)
	}
}

func TestInitTracer_Success(t *testing.T) {
	ctx := context.Background()
	log, _ := logger.New(logger.Config{Level: "info", DevMode: true})
	svcCfg := Config{
		Endpoint:       "localhost:4317",
		ServiceName:    "testsvc",
		ServiceVersion: "v0.1",
		Insecure:       true,
	}
	shutdown, err := InitTracer(ctx, svcCfg, log)
	if err != nil {
		t.Fatalf("InitTracer failed: %v", err)
	}
	// shutdown should not error
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
}
