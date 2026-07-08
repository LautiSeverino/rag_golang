package middlewares

import (
	"bytes"
	"net/http"
)

type responseCaptureWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (w *responseCaptureWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseCaptureWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}
