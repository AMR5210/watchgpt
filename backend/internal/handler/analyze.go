package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/AMR5210/watchgpt/backend/internal/cache"
	"github.com/AMR5210/watchgpt/backend/internal/proxy"
	"github.com/AMR5210/watchgpt/backend/internal/requestctx"
)

type AnalyzeRequest struct {
	Image  string `json:"image"`
	Prompt string `json:"prompt,omitempty"`
}

type AnalyzeResponse struct {
	Answer string `json:"answer"`
	Cached bool   `json:"cached"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

const defaultPrompt = "Analyze the content in this image and answer as briefly as possible."

type AnalyzeHandler struct {
	openai analyzer
	cache  responseCache
}

type analyzer interface {
	Analyze(ctx context.Context, base64Image, prompt string) (string, error)
}

type responseCache interface {
	Get(ctx context.Context, key string) (string, bool)
	Set(ctx context.Context, key string, value string)
}

func NewAnalyzeHandler(client analyzer, c responseCache) *AnalyzeHandler {
	return &AnalyzeHandler{openai: client, cache: c}
}

func (h *AnalyzeHandler) Analyze(w http.ResponseWriter, r *http.Request) {
	var req AnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
		return
	}

	if req.Image == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "image field is required"})
		return
	}

	if len(req.Image) > 14000000 {
		writeJSON(w, http.StatusRequestEntityTooLarge, ErrorResponse{Error: "image too large, max 10MB"})
		return
	}

	prompt := req.Prompt
	if prompt == "" {
		prompt = defaultPrompt
	}

	// Check cache
	cacheKey := cache.HashKey(req.Image, prompt)
	if cached, ok := h.cache.Get(r.Context(), cacheKey); ok {
		requestctx.Logger(r.Context()).Info("cache hit", "key", cacheKey[:16])
		writeJSON(w, http.StatusOK, AnalyzeResponse{Answer: cached, Cached: true})
		return
	}

	// Cache miss — call OpenAI
	answer, err := h.openai.Analyze(r.Context(), req.Image, prompt)
	if err != nil {
		requestctx.Logger(r.Context()).Error("openai request failed", "error", err)
		if proxy.IsCircuitOpen(err) {
			writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "AI service temporarily unavailable"})
			return
		}
		writeJSON(w, http.StatusBadGateway, ErrorResponse{Error: "failed to get response from AI service"})
		return
	}

	// Store in cache
	h.cache.Set(r.Context(), cacheKey, answer)

	writeJSON(w, http.StatusOK, AnalyzeResponse{Answer: answer, Cached: false})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
