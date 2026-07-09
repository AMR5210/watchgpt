package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/AMR5210/watchgpt/backend/internal/metrics"
	"github.com/AMR5210/watchgpt/backend/internal/requestctx"
	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisCache(addr string, ttl time.Duration) *RedisCache {
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		PoolSize:     20,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		slog.Warn("redis not available, caching disabled", "error", err)
	} else {
		slog.Info("redis connected", "addr", addr)
	}

	return &RedisCache{client: client, ttl: ttl}
}

// HashKey creates a cache key from image data + prompt.
func HashKey(imageBase64 string, prompt string) string {
	h := sha256.New()
	h.Write([]byte(prompt))
	// Only hash first 1000 chars of image to keep it fast
	if len(imageBase64) > 1000 {
		h.Write([]byte(imageBase64[:1000]))
	} else {
		h.Write([]byte(imageBase64))
	}
	h.Write([]byte(imageBase64[len(imageBase64)-min(len(imageBase64), 1000):]))
	return "watchgpt:" + hex.EncodeToString(h.Sum(nil))
}

// Get retrieves a cached response
func (c *RedisCache) Get(ctx context.Context, key string) (string, bool) {
	val, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		metrics.CacheMisses.Inc()
		return "", false
	}
	if err != nil {
		requestctx.Logger(ctx).Warn("cache get error", "error", err)
		metrics.CacheMisses.Inc()
		return "", false
	}
	metrics.CacheHits.Inc()
	return val, true
}

// Set stores a response in cache
func (c *RedisCache) Set(ctx context.Context, key string, value string) {
	if err := c.client.Set(ctx, key, value, c.ttl).Err(); err != nil {
		requestctx.Logger(ctx).Warn("cache set error", "error", err)
	}
}
