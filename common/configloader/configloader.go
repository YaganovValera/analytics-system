package configloader

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Load загружает конфиг в cfgPtr: из YAML + ENV + defaults.
// envPrefix — префикс ENV переменных, например: "ANALYTICS_API"
func Load(path, envPrefix string, cfgPtr interface{}) error {
	v := viper.New()

	// Шаг 1: apply registered defaults
	for key, val := range getDefaults() {
		v.SetDefault(key, val)
	}

	// Шаг 2: environment override
	v.SetEnvPrefix(envPrefix)
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Шаг 3: read file (if provided)
	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return fmt.Errorf("configloader: read config %q: %w", path, err)
		}
	}

	// Шаг 4: decode
	if err := decode(v.AllSettings(), cfgPtr); err != nil {
		return fmt.Errorf("configloader: decode failed: %w", err)
	}

	// Шаг 5: validate if possible
	if v, ok := cfgPtr.(interface{ Validate() error }); ok {
		if err := v.Validate(); err != nil {
			return fmt.Errorf("configloader: validation failed: %w", err)
		}
	}

	return nil
}
