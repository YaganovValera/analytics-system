// common/kafka/consumer/config.go
package consumer

import (
	"fmt"

	"github.com/YaganovValera/analytics-system/common/backoff"
)

type Config struct {
	Brokers []string       `mapstructure:"brokers"`
	GroupID string         `mapstructure:"group_id"`
	Version string         `mapstructure:"version"`
	Backoff backoff.Config `mapstructure:"backoff"`
}

func (c *Config) ApplyDefaults() {
	if c.Version == "" {
		c.Version = "2.8.0"
	}
}

func (c Config) Validate() error {
	if len(c.Brokers) == 0 {
		return fmt.Errorf("kafka consumer: brokers required")
	}
	if c.GroupID == "" {
		return fmt.Errorf("kafka consumer: GroupID required")
	}
	if c.Version == "" {
		return fmt.Errorf("kafka consumer: Version required")
	}
	return nil
}
