package requestctx

import (
	"context"
	"log/slog"
)

type contextKey string

const (
	requestIDKey contextKey = "request_id"
	userKey      contextKey = "user"
)

type User struct {
	ID       string
	Username string
	Scopes   []string
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

func RequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value(requestIDKey).(string); ok {
		return requestID
	}
	return ""
}

func WithUser(ctx context.Context, user User) context.Context {
	return context.WithValue(ctx, userKey, user)
}

func UserFromContext(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(userKey).(User)
	return user, ok
}

func UserID(ctx context.Context) string {
	if user, ok := UserFromContext(ctx); ok {
		return user.ID
	}
	return ""
}

func Logger(ctx context.Context) *slog.Logger {
	logger := slog.Default()
	if requestID := RequestID(ctx); requestID != "" {
		logger = logger.With("request_id", requestID)
	}
	if userID := UserID(ctx); userID != "" {
		logger = logger.With("user_id", userID)
	}
	return logger
}
