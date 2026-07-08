package middlewares

import (
	"log"
	"net/http"
	"runtime/debug"
)

// Recover captura cualquier panic dentro del handler, lo loguea con stack trace,
// y devuelve un 500 con JSON en lugar de cortar la conexión sin respuesta.
// Sin esto, un panic se ve como "socket hang up" del lado del cliente
// porque Go cierra la conexión sin escribir nada.
func Recover(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Printf("PANIC en %s %s: %v\n%s", r.Method, r.URL.Path, rec, debug.Stack())
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"internal server error"}`))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
