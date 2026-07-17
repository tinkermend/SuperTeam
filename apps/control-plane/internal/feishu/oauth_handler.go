package feishu

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/api/middleware"
)

// OAuthHTTPHandler 飞书 OAuth 绑定端点:Start 需 Console 会话(绑定当前登录用户),
// Callback 公开(一次性 state 即凭证,来源于 Start 会话)。
type OAuthHTTPHandler struct {
	service *Service
}

func NewOAuthHTTPHandler(service *Service) *OAuthHTTPHandler {
	return &OAuthHTTPHandler{service: service}
}

// Start 生成 state 并 302 到飞书授权页。query: app_config_id(可空,缺省首个 active)、return_to。
func (h *OAuthHTTPHandler) Start(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil {
		http.Error(w, "console identity not found in context", http.StatusUnauthorized)
		return
	}
	appConfigID := uuid.Nil
	if raw := strings.TrimSpace(r.URL.Query().Get("app_config_id")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			http.Error(w, "invalid app_config_id", http.StatusBadRequest)
			return
		}
		appConfigID = parsed
	}
	authorizeURL, err := h.service.StartOAuth(r.Context(), tenantID, userID, appConfigID, r.URL.Query().Get("return_to"))
	if err != nil {
		writeFeishuBindingError(w, err)
		return
	}
	http.Redirect(w, r, authorizeURL, http.StatusFound)
}

// Callback 消费飞书回跳(code+state),完成绑定后 302 回 Web。
// 失败也 302(带 feishu_bind_error),因为浏览器在顶级导航语境,裸 500 体验不可用。
func (h *OAuthHTTPHandler) Callback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	returnTo, err := h.service.CompleteOAuth(r.Context(), code, state)
	if err != nil {
		fallback := h.service.sanitizeReturnTo("")
		http.Redirect(w, r, fallback+"?feishu_bind_error="+url.QueryEscape(bindErrorCode(err)), http.StatusFound)
		return
	}
	separator := "?"
	if strings.Contains(returnTo, "?") {
		separator = "&"
	}
	http.Redirect(w, r, returnTo+separator+"feishu_bound=1", http.StatusFound)
}

func bindErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrOAuthStateInvalid):
		return "state_invalid"
	case errors.Is(err, ErrNoAppConfig):
		return "no_app_config"
	default:
		return "bind_failed"
	}
}

func writeFeishuBindingError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNoAppConfig), errors.Is(err, ErrAppConfigNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, ErrClientRequired), errors.Is(err, ErrSealerRequired):
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	case errors.Is(err, ErrInvalidInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}
