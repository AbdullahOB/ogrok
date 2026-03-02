package server

import (
	"net/http"
	"strings"
)

type AuthMiddleware struct {
	validTokens map[string]bool
}

func NewAuthMiddleware(tokens []string) *AuthMiddleware {
	m := make(map[string]bool, len(tokens))
	for _, t := range tokens {
		m[t] = true
	}
	return &AuthMiddleware{validTokens: m}
}

func (a *AuthMiddleware) ValidateToken(token string) bool {
	return a.validTokens[token]
}

func (a *AuthMiddleware) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
			return
		}

		if !a.ValidateToken(parts[1]) {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
