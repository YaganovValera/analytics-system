// common/telemetry/config.go ---
package telemetry

import (
	"fmt"
	"time"
)

type Config struct {
	Endpoint        string        `mapstructure:"endpoint"`
	Insecure        bool          `mapstructure:"insecure"`
	ReconnectPeriod time.Duration `mapstructure:"reconnect_period"`
	Timeout         time.Duration `mapstructure:"timeout"`
	SamplerRatio    float64       `mapstructure:"sampler_ratio"`
	ServiceName     string        `mapstructure:"service_name"`
	ServiceVersion  string        `mapstructure:"service_version"`
}

func (c *Config) ApplyDefaults() {
	if c.Timeout <= 0 {
		c.Timeout = 5 * time.Second
	}
	if c.ReconnectPeriod <= 0 {
		c.ReconnectPeriod = 5 * time.Second
	}
	if c.SamplerRatio <= 0 || c.SamplerRatio > 1 {
		c.SamplerRatio = 1.0
	}
}

func (cfg Config) Validate() error {
	switch {
	case cfg.Endpoint == "":
		return fmt.Errorf("telemetry: endpoint is required")
	case cfg.ServiceName == "":
		return fmt.Errorf("telemetry: service name is required")
	case cfg.ServiceVersion == "":
		return fmt.Errorf("telemetry: service version is required")
	case cfg.SamplerRatio < 0 || cfg.SamplerRatio > 1:
		return fmt.Errorf("telemetry: sampler ratio must be between 0.0 and 1.0, got %v", cfg.SamplerRatio)
	default:
		return nil
	}
}
