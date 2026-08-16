// Package httpx 收敛各业务 handler 逐字重复的 HTTP 样板:console 判权、
// chi URL 参数解析、JSON 响应写出。业务包只保留语义差异(错误码映射等)。
package httpx

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/authz"
)

// AuthorizeConsoleAction 是 tenant 资源上的 console 判权样板:从请求上下文取
// console 身份,对 tenant 资源执行 action(可选 AuditReason)。
// 授权器未接线时 403,message 带 feature 名(如 "automation")。
// 失败已写响应,调用方直接 return。
func AuthorizeConsoleAction(w http.ResponseWriter, r *http.Request, authorizer authz.Authorizer, feature, action, auditReason string) (uuid.UUID, uuid.UUID, bool) {
	if authorizer == nil {
		http.Error(w, feature+" authorization is not configured", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}
	return CheckConsole(w, r, authorizer, func(tenantID, userID uuid.UUID) authz.CheckRequest {
		return authz.CheckRequest{
			Actor:       authz.ActorRef{Type: authz.ActorUser, ID: userID.String()},
			Action:      action,
			Resource:    authz.ResourceRef{Type: authz.ResourceTenant, ID: tenantID.String()},
			TenantID:    tenantID,
			AuditReason: auditReason,
		}
	})
}

// CheckConsole 取 console 身份并按 build 构造 CheckRequest 判权;资源形态自定义
// (如 credential(self))。授权器为 nil 时 403。失败已写响应。
func CheckConsole(w http.ResponseWriter, r *http.Request, authorizer authz.Authorizer, build func(tenantID, userID uuid.UUID) authz.CheckRequest) (uuid.UUID, uuid.UUID, bool) {
	if authorizer == nil {
		http.Error(w, "authorization is not configured", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil {
		http.Error(w, "console identity not found in context", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}
	decision, err := authorizer.Check(r.Context(), build(tenantID, userID))
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return uuid.Nil, uuid.Nil, false
	}
	if !decision.Allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}
	return tenantID, userID, true
}

// URLParamUUID 解析 chi URL 参数为 UUID;失败写 400(label 用于错误信息,如 "ruleId")。
func URLParamUUID(w http.ResponseWriter, r *http.Request, name, label string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		http.Error(w, "invalid "+label, http.StatusBadRequest)
		return uuid.Nil, false
	}
	return id, true
}

// WriteJSON 是各业务 handler 逐字重复的 JSON 响应写出。
func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
