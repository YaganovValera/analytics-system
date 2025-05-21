// analytics-api/internal/storage/timescaledb/config.go

package timescaledb

import "fmt"

// Config описывает настройки подключения и миграций к TimescaleDB.
type Config struct {
	DSN string `mapstructure:"dsn"`
}

// Validate проверяет корректность конфигурации и возвращает ошибку, если что-то не так.
func (c *Config) Validate() error {
	if c.DSN == "" {
		return fmt.Errorf("timescaledb: dsn must be provided")
	}
	return nil
}
