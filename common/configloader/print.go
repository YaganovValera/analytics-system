package configloader

import (
	"encoding/json"
	"fmt"
)

// PrintConfig выводит конфиг в читаемом виде.
func PrintConfig(v interface{}) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println("Loaded configuration:\n", string(b))
}
