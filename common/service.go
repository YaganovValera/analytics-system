// common/service.go
package common

import "sync"

var (
	initServiceOnce sync.Once
	serviceRegistry []func(string)
)

// RegisterServiceLabelSetter позволяет компонентам регистрировать установку service label.
func RegisterServiceLabelSetter(fn func(string)) {
	serviceRegistry = append(serviceRegistry, fn)
}

// InitServiceName задаёт имя сервиса для всех зарегистрированных подписчиков.
func InitServiceName(name string) {
	initServiceOnce.Do(func() {
		if name == "" {
			panic("common.InitServiceName: empty service name")
		}
		for _, fn := range serviceRegistry {
			fn(name)
		}
	})
}
