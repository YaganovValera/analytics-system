// preprocessor/internal/storage/timescaledb/config.go
package timescaledb

import "fmt"

// Config описывает настройки подключения и миграций к TimescaleDB.
type Config struct {
	// DSN — строка подключения к базе, например postgres://user:pass@host:port/db?sslmode=disable
	DSN string `mapstructure:"dsn"`
	// MigrationsDir — путь к директории с миграционными файлами
	MigrationsDir string `mapstructure:"migrations_dir"`
}

// ApplyDefaults устанавливает значения по умолчанию для Config.
func (c *Config) ApplyDefaults() {
	if c.MigrationsDir == "" {
		c.MigrationsDir = "/app/migrations/timescaledb"
	}
}

// Validate проверяет корректность конфигурации и возвращает ошибку, если что-то не так.
func (c *Config) Validate() error {
	if c.DSN == "" {
		return fmt.Errorf("timescaledb: dsn must be provided")
	}
	if c.MigrationsDir == "" {
		return fmt.Errorf("timescaledb: migrations_dir must be provided")
	}
	return nil
}
