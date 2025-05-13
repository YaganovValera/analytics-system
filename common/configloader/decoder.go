package configloader

import (
	"reflect"
	"strconv"

	"github.com/mitchellh/mapstructure"
)

func decode(input map[string]interface{}, target interface{}) error {
	hook := mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
		stringToBoolHook,
	)
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName:    "mapstructure",
		Result:     target,
		DecodeHook: hook,
	})
	if err != nil {
		return err
	}
	return decoder.Decode(input)
}

func stringToBoolHook(f, t reflect.Kind, data interface{}) (interface{}, error) {
	if f == reflect.String && t == reflect.Bool {
		return strconv.ParseBool(data.(string))
	}
	return data, nil
}
