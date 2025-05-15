// common/service.go
package common

import "sync"

var (
	initServiceOnce sync.Once
	serviceRegistry []func(string)
	registryMu      sync.Mutex
)

// RegisterServiceLabelSetter позволяет компонентам регистрировать установку service label.
// ВАЖНО: вызывать до InitServiceName. Потокобезопасно.
func RegisterServiceLabelSetter(fn func(string)) {
	registryMu.Lock()
	defer registryMu.Unlock()
	serviceRegistry = append(serviceRegistry, fn)
}

// InitServiceName задаёт имя сервиса для всех зарегистрированных подписчиков.
// Паника при пустом имени.
func InitServiceName(name string) {
	initServiceOnce.Do(func() {
		if name == "" {
			panic("common.InitServiceName: empty service name")
		}

		registryMu.Lock()
		defer registryMu.Unlock()
		for _, fn := range serviceRegistry {
			fn(name)
		}
	})
}
