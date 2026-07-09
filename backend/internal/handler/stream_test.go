package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStreamWritesTokensAndDone(t *testing.T) {
	handler := NewStreamHandler(fakeOpenAI{})
	body := `{"messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stream", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.Stream(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content type = %q, want text/event-stream", got)
	}
	if !strings.Contains(rec.Body.String(), "data: hello") || !strings.Contains(rec.Body.String(), "data: [DONE]") {
		t.Fatalf("stream body missing tokens or done marker: %q", rec.Body.String())
	}
}
