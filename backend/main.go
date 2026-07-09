package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AMR5210/watchgpt/backend/internal/cache"
	"github.com/AMR5210/watchgpt/backend/internal/handler"
	"github.com/AMR5210/watchgpt/backend/internal/middleware"
	"github.com/AMR5210/watchgpt/backend/internal/proxy"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	openaiKey := mustEnv("OPENAI_API_KEY")
	apiKey := getEnv("WATCHGPT_API_KEY", "")
	port := getEnv("PORT", "8080")
	redisAddr := getEnv("REDIS_ADDR", "redis:6379")
	cognitoConfig := middleware.CognitoConfig{
		Region:      getEnv("COGNITO_REGION", getEnv("AWS_REGION", "")),
		UserPoolID:  getEnv("COGNITO_USER_POOL_ID", ""),
		AppClientID: getEnv("COGNITO_APP_CLIENT_ID", ""),
	}
	authMiddleware := configureAuth(apiKey, cognitoConfig)
	maxBodyMB := 10

	// Dependencies
	openaiClient := proxy.NewOpenAIClient(openaiKey)
	redisCache := cache.NewRedisCache(redisAddr, 1*time.Hour)

	// Handlers
	analyzeHandler := handler.NewAnalyzeHandler(openaiClient, redisCache)
	chatHandler := handler.NewChatHandler(openaiClient)
	streamHandler := handler.NewStreamHandler(openaiClient)
	rateLimiter := middleware.NewRateLimiter(redisAddr, 60, 1*time.Minute)

	// Router
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Metrics)
	r.Use(middleware.RequestLogger)
	r.Use(middleware.Recovery)
	r.Use(middleware.MaxBodySize(maxBodyMB))

	// Public
	r.Get("/health", handler.Health)
	r.Handle("/metrics", promhttp.Handler())

	// Protected
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware)
		r.Use(rateLimiter.Limit)
		r.Post("/api/v1/analyze", analyzeHandler.Analyze)
		r.Post("/api/v1/chat", chatHandler.Chat)
		r.Post("/api/v1/stream", streamHandler.Stream)
	})

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 90 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("server starting", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-done
	slog.Info("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("forced shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped gracefully")
}

func mustEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		slog.Error("required environment variable not set", "key", key)
		os.Exit(1)
	}
	return val
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func configureAuth(apiKey string, cognitoConfig middleware.CognitoConfig) func(http.Handler) http.Handler {
	if cognitoConfig.Enabled() {
		slog.Info("using cognito jwt auth", "user_pool_id", cognitoConfig.UserPoolID, "region", cognitoConfig.Region)
		return middleware.CognitoAuth(cognitoConfig)
	}
	if cognitoConfig.PartiallyConfigured() {
		slog.Error("cognito auth is partially configured",
			"has_region", cognitoConfig.Region != "",
			"has_user_pool_id", cognitoConfig.UserPoolID != "",
			"has_app_client_id", cognitoConfig.AppClientID != "",
		)
		os.Exit(1)
	}
	if apiKey != "" {
		slog.Warn("using legacy API key auth; configure Cognito env vars for user JWT auth")
		return middleware.Auth(apiKey)
	}

	slog.Error("no auth configured; set Cognito env vars or WATCHGPT_API_KEY")
	os.Exit(1)
	return nil
}
