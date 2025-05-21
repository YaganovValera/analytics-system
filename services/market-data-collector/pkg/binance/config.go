// market-data-collector/pkg/binance/config.go
package binance

import (
	"fmt"
	"time"

	"github.com/YaganovValera/analytics-system/common/backoff"
)

// Config holds WebSocket configuration for Binance connector.
type Config struct {
	URL              string         `mapstructure:"ws_url"`
	Streams          []string       `mapstructure:"streams"`
	BufferSize       int            `mapstructure:"buffer_size"`
	ReadTimeout      time.Duration  `mapstructure:"read_timeout"`
	SubscribeTimeout time.Duration  `mapstructure:"subscribe_timeout"`
	BackoffConfig    backoff.Config `mapstructure:"backoff"`
}

// ApplyDefaults applies fallback defaults if values are unset.
func (c *Config) ApplyDefaults() {
	if c.BufferSize <= 0 {
		c.BufferSize = 100
	}
	if c.ReadTimeout <= 0 {
		c.ReadTimeout = 30 * time.Second
	}
	if c.SubscribeTimeout <= 0 {
		c.SubscribeTimeout = 5 * time.Second
	}
}

// Validate checks config for required fields.
func (c *Config) Validate() error {
	switch {
	case c.URL == "":
		return fmt.Errorf("binance: URL is required")
	case len(c.Streams) == 0:
		return fmt.Errorf("binance: at least one stream is required")
	default:
		return nil
	}
}
