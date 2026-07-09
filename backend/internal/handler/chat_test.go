package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AMR5210/watchgpt/backend/internal/proxy"
)

func TestChatReturnsReply(t *testing.T) {
	handler := NewChatHandler(fakeOpenAI{chatReply: "hello back"})
	body := `{"messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.Chat(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp ChatResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Reply != "hello back" {
		t.Fatalf("reply = %q, want hello back", resp.Reply)
	}
}

func TestChatRejectsEmptyMessages(t *testing.T) {
	handler := NewChatHandler(fakeOpenAI{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat", strings.NewReader(`{"messages":[]}`))
	rec := httptest.NewRecorder()

	handler.Chat(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestChatReturnsBadGatewayWhenOpenAIFails(t *testing.T) {
	handler := NewChatHandler(fakeOpenAI{chatErr: errors.New("upstream failed")})
	body := `{"messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.Chat(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

type fakeOpenAI struct {
	analyzeReply string
	analyzeErr   error
	chatReply    string
	chatErr      error
	streamErr    error
}

func (f fakeOpenAI) Analyze(ctx context.Context, base64Image, prompt string) (string, error) {
	return f.analyzeReply, f.analyzeErr
}

func (f fakeOpenAI) Chat(ctx context.Context, messages []proxy.ChatMessage) (string, error) {
	return f.chatReply, f.chatErr
}

func (f fakeOpenAI) ChatStream(ctx context.Context, messages []proxy.ChatMessage, onToken func(string)) error {
	if f.streamErr != nil {
		return f.streamErr
	}
	onToken("hello")
	onToken(" back")
	return nil
}
