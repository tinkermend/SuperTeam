package serviceauth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/authz"
)

// HTTPHandler 服务凭据管理端点(Console 会话 + credential 判权;服务凭据本质是一种凭据)。
type HTTPHandler struct {
	service    *Service
	authorizer authz.Authorizer
}

func NewHTTPHandler(service *Service) *HTTPHandler {
	return &HTTPHandler{service: service}
}

func (h *HTTPHandler) SetAuthorizer(authorizer authz.Authorizer) {
	h.authorizer = authorizer
}

func (h *HTTPHandler) authorize(w http.ResponseWriter, r *http.Request, action string) (uuid.UUID, bool) {
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil {
		http.Error(w, "console identity not found in context", http.StatusForbidden)
		return uuid.Nil, false
	}
	if h.authorizer == nil {
		return tenantID, true
	}
	decision, err := h.authorizer.Check(r.Context(), authz.CheckRequest{
		Actor:    authz.ActorRef{Type: authz.ActorUser, ID: userID.String()},
		Action:   action,
		Resource: authz.ResourceRef{Type: authz.ResourceCredential, ID: userID.String()},
		TenantID: tenantID,
	})
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return uuid.Nil, false
	}
	if !decision.Allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return uuid.Nil, false
	}
	return tenantID, true
}

type issueTokenRequest struct {
	ServiceName string `json:"service_name"`
}

type issueTokenResponse struct {
	ID          string `json:"id"`
	ServiceName string `json:"service_name"`
	Token       string `json:"token"`
}

// IssueToken 签发服务凭据;明文只在响应中出现一次。
func (h *HTTPHandler) IssueToken(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorize(w, r, authz.ActionCredentialCreate)
	if !ok {
		return
	}
	var req issueTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.ServiceName) == "" {
		http.Error(w, "service_name is required", http.StatusBadRequest)
		return
	}
	plaintext, token, err := h.service.IssueToken(r.Context(), tenantID, req.ServiceName)
	if err != nil {
		writeServiceAuthError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(issueTokenResponse{
		ID:          token.ID.String(),
		ServiceName: token.ServiceName,
		Token:       plaintext,
	})
}

// RevokeToken 吊销服务凭据。
func (h *HTTPHandler) RevokeToken(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorize(w, r, authz.ActionCredentialDelete)
	if !ok {
		return
	}
	idRaw := strings.TrimSpace(chi.URLParam(r, "tokenId"))
	id, err := uuid.Parse(idRaw)
	if err != nil || id == uuid.Nil {
		http.Error(w, "invalid token id", http.StatusBadRequest)
		return
	}
	if _, err := h.service.RevokeToken(r.Context(), tenantID, id); err != nil {
		writeServiceAuthError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeServiceAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrTokenNotFound):
		http.Error(w, "service token not found", http.StatusNotFound)
	case errors.Is(err, ErrInvalidServiceToken):
		http.Error(w, "invalid service token request", http.StatusBadRequest)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}
