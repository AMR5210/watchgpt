package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AMR5210/watchgpt/backend/internal/requestctx"
)

func TestRateLimiterIdentityPrefersAuthenticatedUser(t *testing.T) {
	rl := NewRateLimiter("localhost:6379", 60, 0)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer token")
	req = req.WithContext(requestctx.WithUser(req.Context(), requestctx.User{ID: "user-123"}))

	got := rl.identity(req)
	want := hashIdentity("user:user-123")

	if got != want {
		t.Fatalf("identity = %q, want %q", got, want)
	}
}

func TestRateLimiterIdentityFallsBackToForwardedFor(t *testing.T) {
	rl := NewRateLimiter("localhost:6379", 60, 0)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 10.0.0.1")

	got := rl.identity(req)
	want := hashIdentity("203.0.113.10")

	if got != want {
		t.Fatalf("identity = %q, want %q", got, want)
	}
}
