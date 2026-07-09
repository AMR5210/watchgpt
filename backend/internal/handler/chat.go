package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/AMR5210/watchgpt/backend/internal/proxy"
	"github.com/AMR5210/watchgpt/backend/internal/requestctx"
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Image   string `json:"image,omitempty"`
}

type ChatRequest struct {
	Messages []ChatMessage `json:"messages"`
}

type ChatResponse struct {
	Reply string `json:"reply"`
}

type ChatHandler struct {
	openai chatter
}

type chatter interface {
	Chat(ctx context.Context, messages []proxy.ChatMessage) (string, error)
}

func NewChatHandler(client chatter) *ChatHandler {
	return &ChatHandler{openai: client}
}

func (h *ChatHandler) Chat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
		return
	}

	if len(req.Messages) == 0 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "messages array is required"})
		return
	}

	// Convert handler messages to proxy messages
	proxyMsgs := make([]proxy.ChatMessage, len(req.Messages))
	for i, m := range req.Messages {
		proxyMsgs[i] = proxy.ChatMessage{
			Role:    m.Role,
			Content: m.Content,
			Image:   m.Image,
		}
	}

	reply, err := h.openai.Chat(r.Context(), proxyMsgs)
	if err != nil {
		requestctx.Logger(r.Context()).Error("openai chat failed", "error", err)
		if proxy.IsCircuitOpen(err) {
			writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "AI service temporarily unavailable"})
			return
		}
		writeJSON(w, http.StatusBadGateway, ErrorResponse{Error: "failed to get response from AI service"})
		return
	}

	writeJSON(w, http.StatusOK, ChatResponse{Reply: reply})
}
