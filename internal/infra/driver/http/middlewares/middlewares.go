package middlewares

import (
	"log"
	"net/http"
	"runtime/debug"
	"time"
)

// Logging registra método, path, status y duración de cada request.
// Sin esto, el proceso del servidor no deja ningún rastro de qué request
// llegó, qué tardó, o si terminó con error.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(sw, r)

		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, sw.status, time.Since(start))
	})
}

// Recover captura cualquier panic dentro del handler, lo loguea con stack trace,
// y devuelve un 500 con JSON en lugar de cortar la conexión sin respuesta.
// Sin esto, un panic se ve como "socket hang up" del lado del cliente
// porque Go cierra la conexión sin escribir nada.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("PANIC en %s %s: %v\n%s", r.Method, r.URL.Path, rec, debug.Stack())
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":"internal server error"}`))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
