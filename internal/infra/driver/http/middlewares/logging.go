package middlewares

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

func Logging(logger *log.Logger, logRequests bool, logResponses bool, maxBodyBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			var reqBody []byte
			if logRequests && r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodHead {
				reqBody, _ = io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
				r.Body = io.NopCloser(bytes.NewBuffer(reqBody))
			}

			shouldCaptureResponse := logResponses && r.URL.Path != "/api/v1/query/stream"

			if shouldCaptureResponse {
				rw := &responseCaptureWriter{
					ResponseWriter: w,
					status:         http.StatusOK,
				}

				next.ServeHTTP(rw, r)

				reqText := prettyBody(reqBody)
				respText := prettyBody(rw.body.Bytes())

				logger.Print(buildBlock(
					"HTTP REQUEST",
					map[string]string{
						"Timestamp": time.Now().Format("2006-01-02 15:04:05.000"),
						"Method":    r.Method,
						"Path":      r.URL.Path,
						"Status":    fmt.Sprintf("%d", rw.status),
						"Duration":  time.Since(start).String(),
						"Client IP": clientIP(r),
					},
					reqText,
					respText,
				))
				return
			}

			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)

			reqText := prettyBody(reqBody)

			logger.Print(buildBlock(
				"HTTP REQUEST",
				map[string]string{
					"Timestamp": time.Now().Format("2006-01-02 15:04:05.000"),
					"Method":    r.Method,
					"Path":      r.URL.Path,
					"Status":    fmt.Sprintf("%d", sw.status),
					"Duration":  time.Since(start).String(),
					"Client IP": clientIP(r),
				},
				reqText,
				"",
			))
		})
	}
}

func prettyBody(raw []byte) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return "(empty)"
	}

	var out bytes.Buffer
	if json.Indent(&out, raw, "", "  ") == nil {
		return out.String()
	}
	return string(raw)
}

func buildBlock(title string, meta map[string]string, request string, response string) string {
	var b strings.Builder

	b.WriteString("================================================================================\n")
	b.WriteString(title + "\n")
	b.WriteString("================================================================================\n")

	for k, v := range meta {
		b.WriteString(fmt.Sprintf("%-10s: %s\n", k, v))
	}

	b.WriteString("\n-------------------------------- REQUEST ---------------------------------------\n\n")
	b.WriteString(request)
	b.WriteString("\n")

	if response != "" {
		b.WriteString("\n-------------------------------- RESPONSE --------------------------------------\n\n")
		b.WriteString(response)
		b.WriteString("\n")
	}

	b.WriteString("\n================================================================================\n")
	return b.String()
}

func clientIP(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip != "" {
		return ip
	}
	ip = r.Header.Get("X-Real-IP")
	if ip != "" {
		return ip
	}
	return r.RemoteAddr
}
