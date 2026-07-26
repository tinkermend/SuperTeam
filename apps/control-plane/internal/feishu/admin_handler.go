package feishu

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
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

type upsertAppConfigResponse struct {
	Config appConfigResponse  `json:"config"`
	Verify ConnectivityReport `json:"verify"`
}

type setAppConfigStatusRequest struct {
	Status string `json:"status"`
}

type verifyAppConfigRequest struct {
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

// UpsertAppConfig 写入或轮换租户飞书应用配置(secret 加密落库,响应不回显),并返回连通自检。
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
	cfg, report, err := h.service.UpsertAppConfig(r.Context(), tenantID, req.AppID, req.AppSecret)
	if err != nil {
		writeFeishuError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, upsertAppConfigResponse{
		Config: appConfigResponse{ID: cfg.ID.String(), AppID: cfg.AppID, Status: cfg.Status},
		Verify: report,
	})
}

// ListAppConfigs 列出租户全部应用配置(含 unverified/disabled;不含 secret)。
func (h *AdminHTTPHandler) ListAppConfigs(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorize(w, r, authz.ActionCredentialRead)
	if !ok {
		return
	}
	configs, err := h.service.ListAppConfigs(r.Context(), tenantID)
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

// SetAppConfigStatus 启停通道(disabled 不再下发给 connector)。
func (h *AdminHTTPHandler) SetAppConfigStatus(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorize(w, r, authz.ActionCredentialCreate)
	if !ok {
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "configId")))
	if err != nil || id == uuid.Nil {
		http.Error(w, "invalid config id", http.StatusBadRequest)
		return
	}
	var req setAppConfigStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := h.service.SetAppConfigStatus(r.Context(), tenantID, id, req.Status)
	if err != nil {
		writeFeishuError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, appConfigResponse{ID: cfg.ID.String(), AppID: cfg.AppID, Status: cfg.Status})
}

// VerifyAppConfig 用请求体中的明文凭据做连通自检,不落库(编辑表单"先测再存"用)。
func (h *AdminHTTPHandler) VerifyAppConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorize(w, r, authz.ActionCredentialRead); !ok {
		return
	}
	var req verifyAppConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.AppID) == "" || strings.TrimSpace(req.AppSecret) == "" {
		http.Error(w, "app_id and app_secret are required", http.StatusBadRequest)
		return
	}
	report := h.service.VerifyAppCredentials(r.Context(), req.AppID, req.AppSecret)
	writeJSON(w, http.StatusOK, report)
}

// ContactSync 通讯录批量反查绑定(管理员触发,零用户操作初始化)。
func (h *AdminHTTPHandler) ContactSync(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorize(w, r, authz.ActionCredentialCreate)
	if !ok {
		return
	}
	reports, err := h.service.ContactSync(r.Context(), tenantID)
	if err != nil {
		writeFeishuBindingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reports": reports})
}

type identityListItem struct {
	AuthUserID string `json:"auth_user_id"`
	OpenID     string `json:"open_id"`
	BoundVia   string `json:"bound_via"`
}

// ListIdentities 全量绑定列表(用户管理页展示绑定状态)。
func (h *AdminHTTPHandler) ListIdentities(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorize(w, r, authz.ActionCredentialRead)
	if !ok {
		return
	}
	identities, err := h.service.ListIdentitiesByTenant(r.Context(), tenantID)
	if err != nil {
		writeFeishuBindingError(w, err)
		return
	}
	out := make([]identityListItem, 0, len(identities))
	for _, identity := range identities {
		out = append(out, identityListItem{
			AuthUserID: identity.AuthUserID.String(),
			OpenID:     identity.OpenID,
			BoundVia:   identity.BoundVia,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"identities": out})
}
