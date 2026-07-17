package feishu

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/authz"
)

// AdminHTTPHandler 飞书应用配置管理端点(Console 会话;secret 明文只在请求体中出现一次)。
type AdminHTTPHandler struct {
	service    *Service
	authorizer authz.Authorizer
}

func NewAdminHTTPHandler(service *Service) *AdminHTTPHandler {
	return &AdminHTTPHandler{service: service}
}

func (h *AdminHTTPHandler) SetAuthorizer(authorizer authz.Authorizer) {
	h.authorizer = authorizer
}

func (h *AdminHTTPHandler) authorize(w http.ResponseWriter, r *http.Request, action string) (uuid.UUID, bool) {
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

type upsertAppConfigRequest struct {
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

type appConfigResponse struct {
	ID     string `json:"id"`
	AppID  string `json:"app_id"`
	Status string `json:"status"`
}

// UpsertAppConfig 写入或轮换租户飞书应用配置(secret 加密落库,响应不回显)。
func (h *AdminHTTPHandler) UpsertAppConfig(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorize(w, r, authz.ActionCredentialCreate)
	if !ok {
		return
	}
	var req upsertAppConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.AppID) == "" || strings.TrimSpace(req.AppSecret) == "" {
		http.Error(w, "app_id and app_secret are required", http.StatusBadRequest)
		return
	}
	cfg, err := h.service.UpsertAppConfig(r.Context(), tenantID, req.AppID, req.AppSecret)
	if err != nil {
		writeFeishuError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, appConfigResponse{
		ID:     cfg.ID.String(),
		AppID:  cfg.AppID,
		Status: cfg.Status,
	})
}

// ListAppConfigs 列出租户 active 应用配置(不含 secret)。
func (h *AdminHTTPHandler) ListAppConfigs(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorize(w, r, authz.ActionCredentialRead)
	if !ok {
		return
	}
	configs, err := h.service.repo.ListActiveAppConfigs(r.Context(), tenantID)
	if err != nil {
		writeFeishuError(w, err)
		return
	}
	out := make([]appConfigResponse, 0, len(configs))
	for _, cfg := range configs {
		out = append(out, appConfigResponse{ID: cfg.ID.String(), AppID: cfg.AppID, Status: cfg.Status})
	}
	writeJSON(w, http.StatusOK, map[string]any{"configs": out})
}
