package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnalyzeReturnsCachedResponse(t *testing.T) {
	cache := newFakeCache()
	cache.value = "cached answer"
	cache.ok = true
	handler := NewAnalyzeHandler(fakeOpenAI{}, cache)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analyze", strings.NewReader(`{"image":"abc"}`))
	rec := httptest.NewRecorder()

	handler.Analyze(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp AnalyzeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Cached || resp.Answer != "cached answer" {
		t.Fatalf("response = %+v, want cached answer", resp)
	}
}

func TestAnalyzeCallsOpenAIAndStoresCache(t *testing.T) {
	cache := newFakeCache()
	handler := NewAnalyzeHandler(fakeOpenAI{analyzeReply: "fresh answer"}, cache)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analyze", strings.NewReader(`{"image":"abc","prompt":"p"}`))
	rec := httptest.NewRecorder()

	handler.Analyze(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if cache.storedValue != "fresh answer" {
		t.Fatalf("stored cache value = %q, want fresh answer", cache.storedValue)
	}
}

func TestAnalyzeRejectsMissingImage(t *testing.T) {
	handler := NewAnalyzeHandler(fakeOpenAI{}, newFakeCache())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analyze", strings.NewReader(`{"prompt":"p"}`))
	rec := httptest.NewRecorder()

	handler.Analyze(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAnalyzeReturnsBadGatewayWhenOpenAIFails(t *testing.T) {
	handler := NewAnalyzeHandler(fakeOpenAI{analyzeErr: errors.New("upstream failed")}, newFakeCache())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analyze", strings.NewReader(`{"image":"abc"}`))
	rec := httptest.NewRecorder()

	handler.Analyze(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

type fakeCache struct {
	value       string
	ok          bool
	storedKey   string
	storedValue string
}

func newFakeCache() *fakeCache {
	return &fakeCache{}
}

func (c *fakeCache) Get(ctx context.Context, key string) (string, bool) {
	return c.value, c.ok
}

func (c *fakeCache) Set(ctx context.Context, key string, value string) {
	c.storedKey = key
	c.storedValue = value
}
