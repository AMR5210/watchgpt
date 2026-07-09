package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/AMR5210/watchgpt/backend/internal/requestctx"
)

const requestIDHeader = "X-Request-ID"

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// Preserve Flusher interface for streaming
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		requestctx.Logger(r.Context()).Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
		)
	})
}

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := sanitizeRequestID(r.Header.Get(requestIDHeader))
		if requestID == "" {
			requestID = generateRequestID()
		}

		w.Header().Set(requestIDHeader, requestID)
		ctx := requestctx.WithRequestID(r.Context(), requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				requestctx.Logger(r.Context()).Error("panic recovered", "error", err, "stack", string(debug.Stack()))
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func MaxBodySize(megabytes int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, int64(megabytes)*1024*1024)
			next.ServeHTTP(w, r)
		})
	}
}

func generateRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b[:])
}

func sanitizeRequestID(requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || len(requestID) > 128 {
		return ""
	}

	for _, r := range requestID {
		if r < 33 || r > 126 {
			return ""
		}
	}

	return requestID
}
