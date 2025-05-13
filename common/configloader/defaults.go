package configloader

import "sync"

var (
	defaultsMu sync.RWMutex
	defaults   = make(map[string]interface{})
)

// RegisterDefaults глобально регистрирует дефолты для всех сервисов.
func RegisterDefaults(k string, v interface{}) {
	defaultsMu.Lock()
	defer defaultsMu.Unlock()
	defaults[k] = v
}

func getDefaults() map[string]interface{} {
	defaultsMu.RLock()
	defer defaultsMu.RUnlock()

	cp := make(map[string]interface{}, len(defaults))
	for k, v := range defaults {
		cp[k] = v
	}
	return cp
}
