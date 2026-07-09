package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func Auth(expectedKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				http.Error(w, `{"error":"missing bearer token"}`, http.StatusUnauthorized)
				return
			}

			if subtle.ConstantTimeCompare([]byte(token), []byte(expectedKey)) != 1 {
				http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func bearerToken(auth string) (string, bool) {
	token := strings.TrimPrefix(auth, "Bearer ")
	if token == auth || token == "" {
		return "", false
	}
	return token, true
}
