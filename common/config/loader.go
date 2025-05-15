package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

// Options определяет параметры загрузки конфигурации.
type Options struct {
	Path        string                 // путь до yaml (может быть пустым)
	EnvPrefix   string                 // префикс для переменных окружения (например "COLLECTOR")
	Defaults    map[string]interface{} // значения по умолчанию
	Out         interface{}            // указатель на структуру конфигурации
	DebugOutput bool                   // печатать конфиг в stdout
}

// Load выполняет загрузку, парсинг и дешифровку конфигурации.
func Load(opts Options) error {
	if opts.Out == nil {
		return fmt.Errorf("config: options.Out must be a pointer")
	}

	v := viper.New()

	// ---------- 1) Defaults ----------
	for k, vdef := range opts.Defaults {
		v.SetDefault(k, vdef)
	}

	// ---------- 2) ENV ----------
	if opts.EnvPrefix != "" {
		v.SetEnvPrefix(opts.EnvPrefix)
	}
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// ---------- 3) YAML файл (опционально) ----------
	if opts.Path != "" {
		v.SetConfigFile(opts.Path)
		if err := v.ReadInConfig(); err != nil {
			return fmt.Errorf("config: read file %q: %w", opts.Path, err)
		}
	}

	// ---------- 4) Decode ----------
	decodeHook := mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
		stringToBoolHook,
	)
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName:    "mapstructure",
		Result:     opts.Out,
		DecodeHook: decodeHook,
	})
	if err != nil {
		return fmt.Errorf("config: decoder init: %w", err)
	}
	if err := dec.Decode(v.AllSettings()); err != nil {
		return fmt.Errorf("config: decode: %w", err)
	}

	if opts.DebugOutput {
		prettyPrint(opts.Out)
	}

	return nil
}

// stringToBoolHook добавляет поддержку преобразования "true"/"false" из строки.
func stringToBoolHook(f, t reflect.Kind, data interface{}) (interface{}, error) {
	if f == reflect.String && t == reflect.Bool {
		return strconv.ParseBool(data.(string))
	}
	return data, nil
}

func prettyPrint(v interface{}) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Println("config: pretty print failed:", err)
		return
	}
	fmt.Println("Loaded configuration:\n", string(b))
}
