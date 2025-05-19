// common/redis/config.go
package redis

import "github.com/YaganovValera/analytics-system/common/backoff"

type Config struct {
	Addr        string
	Password    string
	DB          int
	ServiceName string
	Backoff     backoff.Config
}
