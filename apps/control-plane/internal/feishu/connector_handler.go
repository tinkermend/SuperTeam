package feishu

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/api/middleware"
)

// ConnectorHTTPHandler 服务 /api/v1/connector/* 路由组(仅 ServiceAuth 可达)。
type ConnectorHTTPHandler struct {
	service *Service
}

func NewConnectorHTTPHandler(service *Service) *ConnectorHTTPHandler {
	return &ConnectorHTTPHandler{service: service}
}

type bootstrapConfigResponse struct {
	ConfigID  string `json:"config_id"`
	TenantID  string `json:"tenant_id"`
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

// Bootstrap 返回租户全部 active 飞书应用配置(解密 secret),connector 启动时调用。
func (h *ConnectorHTTPHandler) Bootstrap(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		http.Error(w, "service tenant not found in context", http.StatusUnauthorized)
		return
	}
	configs, err := h.service.BootstrapConfigs(r.Context(), tenantID)
	if err != nil {
		writeFeishuError(w, err)
		return
	}
	out := make([]bootstrapConfigResponse, 0, len(configs))
	for _, cfg := range configs {
		out = append(out, bootstrapConfigResponse{
			ConfigID:  cfg.ConfigID.String(),
			TenantID:  cfg.TenantID.String(),
			AppID:     cfg.AppID,
			AppSecret: cfg.AppSecret,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"configs": out})
}

type identityResponse struct {
	AuthUserID string `json:"auth_user_id"`
	OpenID     string `json:"open_id"`
	BoundVia   string `json:"bound_via"`
}

// Identity 按 open_id 反查绑定用户;未绑定 404(connector 引导未绑定用户走 Console)。
func (h *ConnectorHTTPHandler) Identity(w http.ResponseWriter, r *http.Request) {
	appConfigRaw := strings.TrimSpace(r.URL.Query().Get("app_config_id"))
	openID := strings.TrimSpace(r.URL.Query().Get("open_id"))
	appConfigID, err := uuid.Parse(appConfigRaw)
	if err != nil || appConfigID == uuid.Nil || openID == "" {
		http.Error(w, "app_config_id and open_id are required", http.StatusBadRequest)
		return
	}
	identity, err := h.service.ResolveIdentityByOpenID(r.Context(), appConfigID, openID)
	if err != nil {
		writeFeishuError(w, err)
		return
	}
	if identity.TenantID != middleware.GetTenantID(r.Context()) {
		http.Error(w, "feishu identity not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, identityResponse{
		AuthUserID: identity.AuthUserID.String(),
		OpenID:     identity.OpenID,
		BoundVia:   identity.BoundVia,
	})
}

func writeFeishuError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrIdentityNotFound), errors.Is(err, ErrAppConfigNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, ErrInvalidInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrIdentityMismatch):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, ErrSealerRequired):
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
