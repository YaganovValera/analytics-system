package httpserver

import (
	"log"
	"net/http"
	"runtime/debug"

	"github.com/go-chi/cors"
)

// responseWriterWrapper отслеживает, был ли вызван WriteHeader.
type responseWriterWrapper struct {
	http.ResponseWriter
	wroteHeader bool
	statusCode  int
}

func (rw *responseWriterWrapper) WriteHeader(code int) {
	if rw.wroteHeader {
		// Пытаемся второй раз — игнорируем
		log.Printf("warning: redundant WriteHeader(%d) after status=%d", code, rw.statusCode)
		return
	}
	rw.wroteHeader = true
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriterWrapper) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// RecoverMiddleware защищает от panic и безопасно возвращает 500
func RecoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := &responseWriterWrapper{ResponseWriter: w}

		defer func() {
			if rcv := recover(); rcv != nil {
				log.Printf("panic recovered: %v\n%s", rcv, debug.Stack())
				if !ww.wroteHeader {
					http.Error(ww, "internal server error", http.StatusInternalServerError)
				} else {
					// Заголовки уже отправлены — просто закрываем соединение
					_, _ = ww.ResponseWriter.Write([]byte{})
				}
			}
		}()

		next.ServeHTTP(ww, r)
	})
}

// CORSMiddleware возвращает permissive CORS.
func CORSMiddleware() Middleware {
	return cors.AllowAll().Handler
}
