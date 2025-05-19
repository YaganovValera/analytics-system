// common/kafka/producer/config.go ---
package producer

import (
	"fmt"
	"time"

	"github.com/YaganovValera/analytics-system/common/backoff"
)

type Config struct {
	Brokers        []string       `mapstructure:"brokers"`
	RequiredAcks   string         `mapstructure:"required_acks"`
	Timeout        time.Duration  `mapstructure:"timeout"`
	Compression    string         `mapstructure:"compression"`
	FlushFrequency time.Duration  `mapstructure:"flush_frequency"`
	FlushMessages  int            `mapstructure:"flush_messages"`
	Backoff        backoff.Config `mapstructure:"backoff"`
}

func (c *Config) ApplyDefaults() {
	if c.Timeout <= 0 {
		c.Timeout = 5 * time.Second
	}
	if c.RequiredAcks == "" {
		c.RequiredAcks = "all"
	}
	if c.Compression == "" {
		c.Compression = "none"
	}
}

func (c Config) Validate() error {
	if len(c.Brokers) == 0 {
		return fmt.Errorf("kafka producer: brokers required")
	}
	return nil
}
