package http

import "context"

// HTTPServer описывает контракт HTTP-сервера с жизненным циклом.
type HTTPServer interface {
	// Start запускает сервер и блокирует до отмены ctx.
	Start(ctx context.Context) error
}
