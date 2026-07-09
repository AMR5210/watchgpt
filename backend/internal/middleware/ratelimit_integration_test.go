//go:build integration

package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestRedisRateLimiterLimitsGlobally(t *testing.T) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		t.Skip("REDIS_ADDR is required for integration test")
	}

	rl := NewRateLimiter(redisAddr, 2, time.Minute)
	handler := rl.Limit(okHandler())
	token := "integration-token-" + time.Now().Format("150405.000000000")

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want %d", i+1, rec.Code, http.StatusOK)
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("limited request status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After header is empty")
	}
}
