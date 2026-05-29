package middleware

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const (
	NodeIDKey contextKey = "node_id"
	UserIDKey contextKey = "user_id"
)

type AuthService interface {
	ValidateRuntimeToken(ctx context.Context, nodeID, token string) error
}

func RuntimeAuth(authService AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth == "" {
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(auth, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, "invalid authorization header", http.StatusUnauthorized)
				return
			}

			token := parts[1]
			nodeID := r.Header.Get("X-Node-ID")
			if nodeID == "" {
				http.Error(w, "missing X-Node-ID header", http.StatusUnauthorized)
				return
			}

			if err := authService.ValidateRuntimeToken(r.Context(), nodeID, token); err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), NodeIDKey, nodeID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetNodeID(ctx context.Context) string {
	if nodeID, ok := ctx.Value(NodeIDKey).(string); ok {
		return nodeID
	}
	return ""
}
