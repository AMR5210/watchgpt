package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/AMR5210/watchgpt/backend/internal/proxy"
	"github.com/AMR5210/watchgpt/backend/internal/requestctx"
)

type StreamHandler struct {
	openai streamer
}

type streamer interface {
	ChatStream(ctx context.Context, messages []proxy.ChatMessage, onToken func(string)) error
}

func NewStreamHandler(client streamer) *StreamHandler {
	return &StreamHandler{openai: client}
}

func (h *StreamHandler) Stream(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
		return
	}

	if len(req.Messages) == 0 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "messages array is required"})
		return
	}

	proxyMsgs := make([]proxy.ChatMessage, len(req.Messages))
	for i, m := range req.Messages {
		proxyMsgs[i] = proxy.ChatMessage{
			Role:    m.Role,
			Content: m.Content,
			Image:   m.Image,
		}
	}

	// Set headers for streaming
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Use ResponseController — works through all middleware wrappers
	rc := http.NewResponseController(w)

	err := h.openai.ChatStream(r.Context(), proxyMsgs, func(token string) {
		fmt.Fprintf(w, "data: %s\n\n", token)
		rc.Flush()
	})

	if err != nil {
		requestctx.Logger(r.Context()).Error("stream failed", "error", err)
		if proxy.IsCircuitOpen(err) {
			fmt.Fprintf(w, "data: [ERROR] AI service temporarily unavailable\n\n")
		} else {
			fmt.Fprintf(w, "data: [ERROR] %s\n\n", err.Error())
		}
		rc.Flush()
		return
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	rc.Flush()
}
