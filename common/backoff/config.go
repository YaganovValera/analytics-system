// common/backoff/config.go
package backoff

import (
	"fmt"
	"time"
)

type Config struct {
	InitialInterval     time.Duration `mapstructure:"initial_interval"`
	RandomizationFactor float64       `mapstructure:"randomization_factor"`
	Multiplier          float64       `mapstructure:"multiplier"`
	MaxInterval         time.Duration `mapstructure:"max_interval"`
	MaxElapsedTime      time.Duration `mapstructure:"max_elapsed_time"`
	PerAttemptTimeout   time.Duration `mapstructure:"per_attempt_timeout"`
}

func (c *Config) ApplyDefaults() {
	if c.InitialInterval <= 0 {
		c.InitialInterval = time.Second
	}
	if c.RandomizationFactor <= 0 {
		c.RandomizationFactor = 0.5
	}
	if c.Multiplier <= 0 {
		c.Multiplier = 2.0
	}
	if c.MaxInterval <= 0 {
		c.MaxInterval = 30 * time.Second
	}
}

func (c Config) Validate() error {
	if c.RandomizationFactor < 0 || c.RandomizationFactor > 1 {
		return fmt.Errorf("backoff: RandomizationFactor must be in [0,1]")
	}
	if c.Multiplier < 1 {
		return fmt.Errorf("backoff: Multiplier must be ≥ 1")
	}
	if c.PerAttemptTimeout < 0 {
		return fmt.Errorf("backoff: PerAttemptTimeout must be ≥ 0")
	}
	return nil
}
