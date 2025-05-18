package serviceid

import "sync"

var (
	initServiceOnce sync.Once
	serviceRegistry []func(string)
	registryMu      sync.Mutex
)

// Register позволяет зарегистрировать callback для установки имени сервиса.
// Вызывать до InitServiceName.
func Register(fn func(string)) {
	registryMu.Lock()
	defer registryMu.Unlock()
	serviceRegistry = append(serviceRegistry, fn)
}

// InitServiceName вызывает все callbacks с заданным именем сервиса.
func InitServiceName(name string) {
	initServiceOnce.Do(func() {
		if name == "" {
			panic("serviceid.InitServiceName: empty service name")
		}
		registryMu.Lock()
		defer registryMu.Unlock()
		for _, fn := range serviceRegistry {
			fn(name)
		}
	})
}
