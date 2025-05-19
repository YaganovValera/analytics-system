// common/redis/config.go
package redis

import (
	"fmt"

	"github.com/YaganovValera/analytics-system/common/backoff"
)

type Config struct {
	Addr        string
	Password    string
	DB          int
	ServiceName string
	Backoff     backoff.Config
}

func (c *Config) ApplyDefaults() {
	if c.DB < 0 {
		c.DB = 0
	}
}

func (c Config) Validate() error {
	if c.Addr == "" {
		return fmt.Errorf("redis: Addr is required")
	}
	if c.DB < 0 {
		return fmt.Errorf("redis: DB index must be ≥ 0, got %d", c.DB)
	}
	return nil
}
