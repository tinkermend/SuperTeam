package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/superteam/control-plane/internal/auth"
)

type AuthService interface {
	AuthenticateUser(ctx context.Context, username, password string) (*auth.User, error)
	GenerateRuntimeToken(ctx context.Context, nodeID string, expiresAt time.Time) (string, error)
}

type AuthHandler struct {
	authService AuthService
}

func NewAuthHandler(authService AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	user, err := h.authService.AuthenticateUser(r.Context(), req.Username, req.Password)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user_id":  user.ID,
		"username": user.Username,
	})
}

func (h *AuthHandler) GenerateToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeID string `json:"node_id"`
		TTL    int64  `json:"ttl"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.TTL == 0 {
		req.TTL = 720 * 3600
	}

	expiresAt := time.Now().Add(time.Duration(req.TTL) * time.Second)
	token, err := h.authService.GenerateRuntimeToken(r.Context(), req.NodeID, expiresAt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":      token,
		"expires_at": expiresAt,
	})
}
