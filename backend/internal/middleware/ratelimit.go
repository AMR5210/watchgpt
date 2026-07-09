package middleware

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AMR5210/watchgpt/backend/internal/requestctx"
	"github.com/redis/go-redis/v9"
)

const rateLimitKeyPrefix = "watchgpt:rate_limit:"

type RateLimiter struct {
	client  *redis.Client
	maxReqs int
	window  time.Duration
	script  *redis.Script
}

type rateLimitResult struct {
	allowed    bool
	remaining  int
	retryAfter time.Duration
}

func NewRateLimiter(redisAddr string, maxReqs int, window time.Duration) *RateLimiter {
	client := redis.NewClient(&redis.Options{
		Addr:         redisAddr,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		PoolSize:     20,
	})

	return &RateLimiter{
		client:  client,
		maxReqs: maxReqs,
		window:  window,
		script: redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]

redis.call("ZREMRANGEBYSCORE", key, 0, now - window)

local count = redis.call("ZCARD", key)
if count >= limit then
	local oldest = redis.call("ZRANGE", key, 0, 0, "WITHSCORES")
	local retry_after = window
	if oldest[2] ~= nil then
		retry_after = math.max(0, (tonumber(oldest[2]) + window) - now)
	end
	return {0, count, retry_after}
end

redis.call("ZADD", key, now, member)
redis.call("PEXPIRE", key, window)

return {1, limit - count - 1, 0}
`),
	}
}

func (rl *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := rl.allow(r.Context(), rl.identity(r))
		if err != nil {
			requestctx.Logger(r.Context()).Warn("rate limit check failed, allowing request", "error", err)
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.maxReqs))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.remaining))

		if !result.allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(result.retryAfter.Seconds()+0.999)))
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) allow(ctx context.Context, identity string) (rateLimitResult, error) {
	now := time.Now().UnixMilli()
	member := fmt.Sprintf("%d-%s", now, randomSuffix())

	values, err := rl.script.Run(ctx, rl.client, []string{rateLimitKeyPrefix + identity},
		now,
		rl.window.Milliseconds(),
		rl.maxReqs,
		member,
	).Slice()
	if err != nil {
		return rateLimitResult{}, err
	}
	if len(values) != 3 {
		return rateLimitResult{}, fmt.Errorf("unexpected redis rate limit result length: %d", len(values))
	}

	allowed, err := redisInt(values[0])
	if err != nil {
		return rateLimitResult{}, fmt.Errorf("parse allowed: %w", err)
	}
	countOrRemaining, err := redisInt(values[1])
	if err != nil {
		return rateLimitResult{}, fmt.Errorf("parse count: %w", err)
	}
	retryAfterMillis, err := redisInt(values[2])
	if err != nil {
		return rateLimitResult{}, fmt.Errorf("parse retry after: %w", err)
	}

	result := rateLimitResult{
		allowed:    allowed == 1,
		remaining:  int(countOrRemaining),
		retryAfter: time.Duration(retryAfterMillis) * time.Millisecond,
	}
	if !result.allowed {
		result.remaining = 0
	}

	return result, nil
}

func (rl *RateLimiter) identity(r *http.Request) string {
	if userID := requestctx.UserID(r.Context()); userID != "" {
		return hashIdentity("user:" + userID)
	}
	if auth := r.Header.Get("Authorization"); auth != "" {
		return hashIdentity(auth)
	}
	if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		return hashIdentity(strings.TrimSpace(strings.Split(forwardedFor, ",")[0]))
	}

	return hashIdentity(r.RemoteAddr)
}

func hashIdentity(identity string) string {
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:])
}

func redisInt(value any) (int64, error) {
	switch v := value.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case string:
		return strconv.ParseInt(v, 10, 64)
	default:
		return 0, fmt.Errorf("unexpected value type %T", value)
	}
}

func randomSuffix() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "fallback"
	}
	return hex.EncodeToString(b[:])
}
