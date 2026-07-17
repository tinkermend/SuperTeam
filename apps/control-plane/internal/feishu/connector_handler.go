package feishu

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/api/middleware"
)

// ConnectorHTTPHandler 服务 /api/v1/connector/* 路由组(仅 ServiceAuth 可达)。
type ConnectorHTTPHandler struct {
	service  *Service
	outbox   OutboxRepository
	projects ProjectGateway
}

func NewConnectorHTTPHandler(service *Service) *ConnectorHTTPHandler {
	return &ConnectorHTTPHandler{service: service}
}

// SetOutboxRepository 注入 outbox 读写(拆开注入以便路由测试用假实现)。
func (h *ConnectorHTTPHandler) SetOutboxRepository(repo OutboxRepository) {
	h.outbox = repo
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

type outboxItemResponse struct {
	ID              string         `json:"id"`
	Kind            string         `json:"kind"`
	ResourceType    string         `json:"resource_type"`
	ResourceID      string         `json:"resource_id"`
	ProjectID       string         `json:"project_id,omitempty"`
	RecipientUserID string         `json:"recipient_user_id"`
	RecipientOpenID string         `json:"recipient_open_id"`
	Payload         map[string]any `json:"payload"`
	Attempts        int32          `json:"attempts"`
	CreatedAt       string         `json:"created_at"`
}

// ListOutbox 返回待投递消息(pending),connector 轮询消费。
func (h *ConnectorHTTPHandler) ListOutbox(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		http.Error(w, "service tenant not found in context", http.StatusUnauthorized)
		return
	}
	limit := int32(20)
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			http.Error(w, "limit must be 1-100", http.StatusBadRequest)
			return
		}
		limit = int32(parsed)
	}
	items, err := h.outbox.ListPendingOutbox(r.Context(), tenantID, limit)
	if err != nil {
		writeFeishuError(w, err)
		return
	}
	out := make([]outboxItemResponse, 0, len(items))
	for _, item := range items {
		resp := outboxItemResponse{
			ID:              item.ID.String(),
			Kind:            item.Kind,
			ResourceType:    item.ResourceType,
			ResourceID:      item.ResourceID.String(),
			RecipientUserID: item.RecipientUserID.String(),
			RecipientOpenID: item.RecipientOpenID,
			Payload:         item.Payload,
			Attempts:        item.Attempts,
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
		}
		if item.ProjectID != nil {
			resp.ProjectID = item.ProjectID.String()
		}
		out = append(out, resp)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

type ackOutboxRequest struct {
	Result          string `json:"result"` // sent | failed
	FeishuMessageID string `json:"feishu_message_id,omitempty"`
	Error           string `json:"error,omitempty"`
}

// AckOutbox 回执投递结果:sent 终态回填消息ID;failed 计数,3 次后标 failed。
func (h *ConnectorHTTPHandler) AckOutbox(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		http.Error(w, "service tenant not found in context", http.StatusUnauthorized)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "outboxId"))
	if err != nil || id == uuid.Nil {
		http.Error(w, "invalid outbox id", http.StatusBadRequest)
		return
	}
	var req ackOutboxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var item OutboxItem
	switch req.Result {
	case "sent":
		item, err = h.outbox.MarkOutboxSent(r.Context(), tenantID, id, req.FeishuMessageID)
	case "failed":
		item, err = h.outbox.MarkOutboxFailed(r.Context(), tenantID, id, req.Error)
	default:
		http.Error(w, "result must be sent or failed", http.StatusBadRequest)
		return
	}
	if err != nil {
		if errors.Is(err, ErrOutboxNotFound) {
			// 幂等:重复 ack 已终态的行返回 200(connector 重试/重复消费安全)。
			writeJSON(w, http.StatusOK, map[string]any{"status": "already_acked"})
			return
		}
		writeFeishuError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": item.Status})
}
