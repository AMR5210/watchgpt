package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AMR5210/watchgpt/backend/internal/requestctx"
)

func TestRequestIDAcceptsIncomingHeader(t *testing.T) {
	const incomingRequestID = "test-request-123"

	var contextRequestID string
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contextRequestID = requestctx.RequestID(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(requestIDHeader, incomingRequestID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if contextRequestID != incomingRequestID {
		t.Fatalf("context request ID = %q, want %q", contextRequestID, incomingRequestID)
	}
	if got := rec.Header().Get(requestIDHeader); got != incomingRequestID {
		t.Fatalf("response request ID = %q, want %q", got, incomingRequestID)
	}
}

func TestRequestIDGeneratesMissingHeader(t *testing.T) {
	var contextRequestID string
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contextRequestID = requestctx.RequestID(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if contextRequestID == "" {
		t.Fatal("context request ID is empty")
	}
	if got := rec.Header().Get(requestIDHeader); got != contextRequestID {
		t.Fatalf("response request ID = %q, want context request ID %q", got, contextRequestID)
	}
}
