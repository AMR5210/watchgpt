package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/AMR5210/watchgpt/backend/internal/metrics"
)

func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		metrics.RequestsInFlight.Inc()
		defer metrics.RequestsInFlight.Dec()

		wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r)

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(wrapped.status)

		metrics.RequestsTotal.WithLabelValues(r.Method, r.URL.Path, status).Inc()
		metrics.RequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Preserve Flusher interface for streaming
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
