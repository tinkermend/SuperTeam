package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/auth"
	"github.com/superteam/control-plane/internal/runtime"
)

type contextKey string

const (
	NodeIDKey           contextKey = "node_id"
	UserIDKey           contextKey = "user_id"
	TenantIDKey         contextKey = "tenant_id"
	RuntimeNodeIDKey    contextKey = "runtime_node_id"
	RuntimeSessionIDKey contextKey = "runtime_session_id"
	RuntimeTokenKey     contextKey = "runtime_token"
)

type AuthService interface {
	ValidateRuntimeToken(ctx context.Context, nodeID, token string) error
}

type RuntimeSessionAuthService interface {
	ValidateRuntimeSession(ctx context.Context, token string) (*runtime.RuntimeSessionValidation, error)
}

type ConsoleUserAuthService interface {
	GetCurrentUserContext(ctx context.Context, token string) (*auth.CurrentUserContext, error)
}

func ConsoleUserAuth(authService ConsoleUserAuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authService == nil {
				http.Error(w, "console auth is not configured", http.StatusUnauthorized)
				return
			}
			cookie, err := r.Cookie(auth.SessionCookieName)
			if err != nil || strings.TrimSpace(cookie.Value) == "" {
				http.Error(w, "missing session cookie", http.StatusUnauthorized)
				return
			}
			current, err := authService.GetCurrentUserContext(r.Context(), cookie.Value)
			if err != nil {
				http.Error(w, "invalid session", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), UserIDKey, current.User.ID)
			ctx = context.WithValue(ctx, TenantIDKey, current.TenantID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RuntimeAuth(authService AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(w, r)
			if !ok {
				return
			}
			nodeID := r.Header.Get("X-Node-ID")
			if nodeID == "" {
				http.Error(w, "missing X-Node-ID header", http.StatusUnauthorized)
				return
			}

			if err := authService.ValidateRuntimeToken(r.Context(), nodeID, token); err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			ctx := withRuntimeIdentity(r.Context(), runtimeIdentity{
				NodeID: nodeID,
				Token:  token,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RuntimeSessionAuth(authService RuntimeSessionAuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(w, r)
			if !ok {
				return
			}
			ctx, ok := contextWithRuntimeSession(w, r.Context(), authService, token)
			if !ok {
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RuntimeSessionOrLegacyAuth(sessionAuth RuntimeSessionAuthService, legacyAuth AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(w, r)
			if !ok {
				return
			}
			if sessionAuth != nil {
				if ctx, valid := contextWithRuntimeSession(nil, r.Context(), sessionAuth, token); valid {
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			if legacyAuth != nil {
				nodeID := r.Header.Get("X-Node-ID")
				if nodeID != "" {
					if err := legacyAuth.ValidateRuntimeToken(r.Context(), nodeID, token); err == nil {
						ctx := withRuntimeIdentity(r.Context(), runtimeIdentity{
							NodeID: nodeID,
							Token:  token,
						})
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}
			}
			http.Error(w, "invalid runtime authentication", http.StatusUnauthorized)
		})
	}
}

type runtimeIdentity struct {
	NodeID           string
	TenantID         uuid.UUID
	RuntimeNodeID    uuid.UUID
	RuntimeSessionID uuid.UUID
	Token            string
}

func bearerToken(w http.ResponseWriter, r *http.Request) (string, bool) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		http.Error(w, "missing authorization header", http.StatusUnauthorized)
		return "", false
	}

	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" || strings.TrimSpace(parts[1]) == "" {
		http.Error(w, "invalid authorization header", http.StatusUnauthorized)
		return "", false
	}
	return strings.TrimSpace(parts[1]), true
}

func contextWithRuntimeSession(w http.ResponseWriter, ctx context.Context, authService RuntimeSessionAuthService, token string) (context.Context, bool) {
	if authService == nil {
		if w != nil {
			http.Error(w, "runtime session auth is not configured", http.StatusUnauthorized)
		}
		return ctx, false
	}
	validation, err := authService.ValidateRuntimeSession(ctx, token)
	if err != nil {
		if w != nil {
			http.Error(w, "invalid runtime session", http.StatusUnauthorized)
		}
		return ctx, false
	}
	return withRuntimeIdentity(ctx, runtimeIdentity{
		NodeID:           validation.NodeID,
		TenantID:         validation.TenantID,
		RuntimeNodeID:    validation.RuntimeNodeID,
		RuntimeSessionID: validation.SessionID,
		Token:            token,
	}), true
}

func withRuntimeIdentity(ctx context.Context, identity runtimeIdentity) context.Context {
	ctx = context.WithValue(ctx, NodeIDKey, identity.NodeID)
	ctx = context.WithValue(ctx, RuntimeTokenKey, identity.Token)
	if identity.TenantID != uuid.Nil {
		ctx = context.WithValue(ctx, TenantIDKey, identity.TenantID)
	}
	if identity.RuntimeNodeID != uuid.Nil {
		ctx = context.WithValue(ctx, RuntimeNodeIDKey, identity.RuntimeNodeID)
	}
	if identity.RuntimeSessionID != uuid.Nil {
		ctx = context.WithValue(ctx, RuntimeSessionIDKey, identity.RuntimeSessionID)
	}
	return ctx
}

func GetNodeID(ctx context.Context) string {
	if nodeID, ok := ctx.Value(NodeIDKey).(string); ok {
		return nodeID
	}
	return ""
}

func GetUserID(ctx context.Context) uuid.UUID {
	if userID, ok := ctx.Value(UserIDKey).(uuid.UUID); ok {
		return userID
	}
	return uuid.Nil
}

func GetTenantID(ctx context.Context) uuid.UUID {
	if tenantID, ok := ctx.Value(TenantIDKey).(uuid.UUID); ok {
		return tenantID
	}
	return uuid.Nil
}

func GetRuntimeNodeID(ctx context.Context) uuid.UUID {
	if runtimeNodeID, ok := ctx.Value(RuntimeNodeIDKey).(uuid.UUID); ok {
		return runtimeNodeID
	}
	return uuid.Nil
}

func GetRuntimeSessionID(ctx context.Context) uuid.UUID {
	if runtimeSessionID, ok := ctx.Value(RuntimeSessionIDKey).(uuid.UUID); ok {
		return runtimeSessionID
	}
	return uuid.Nil
}

func GetRuntimeToken(ctx context.Context) string {
	if token, ok := ctx.Value(RuntimeTokenKey).(string); ok {
		return token
	}
	return ""
}

const (
	ServiceNameKey    contextKey = "service_name"
	ActingUserIDKey   contextKey = "acting_user_id"
	ActingOpenIDKey   contextKey = "acting_open_id"
	ServiceTokenIDKey contextKey = "service_token_id"
)

// ServiceAuthService 验证外部服务凭据(如 feishu-connector),返回凭据归属租户等信息。
type ServiceAuthService interface {
	ValidateServiceToken(ctx context.Context, serviceName, token string) (ServiceIdentity, error)
}

// ServiceIdentity 是服务凭据验证结果的中间件视图。
type ServiceIdentity struct {
	TokenID  uuid.UUID
	TenantID uuid.UUID
}

// OnBehalfOfResolver 校验 on-behalf-of 声明:acting user 必须存在且其飞书绑定
// 的 open_id 与请求声明一致(服务端反查绑定表,防 connector 被攻破后任意冒充)。
type OnBehalfOfResolver interface {
	VerifyOnBehalfOf(ctx context.Context, tenantID, actingUserID uuid.UUID, openID string) error
}

// ServiceAuth 认证外部服务:Bearer token + X-Service-Name。可选的
// X-On-Behalf-Of + X-Feishu-Open-Id 头声明行为人,经 resolver 核对后注入
// UserIDKey——下游 handler 对 on-behalf-of 请求可复用 Console 同款身份读取。
func ServiceAuth(authService ServiceAuthService, resolver OnBehalfOfResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authService == nil {
				http.Error(w, "service auth is not configured", http.StatusUnauthorized)
				return
			}
			token, ok := bearerToken(w, r)
			if !ok {
				return
			}
			serviceName := strings.TrimSpace(r.Header.Get("X-Service-Name"))
			if serviceName == "" {
				http.Error(w, "missing X-Service-Name header", http.StatusUnauthorized)
				return
			}
			identity, err := authService.ValidateServiceToken(r.Context(), serviceName, token)
			if err != nil {
				http.Error(w, "invalid service token", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), ServiceNameKey, serviceName)
			ctx = context.WithValue(ctx, ServiceTokenIDKey, identity.TokenID)
			ctx = context.WithValue(ctx, TenantIDKey, identity.TenantID)

			actingUserRaw := strings.TrimSpace(r.Header.Get("X-On-Behalf-Of"))
			if actingUserRaw != "" {
				actingUserID, err := uuid.Parse(actingUserRaw)
				if err != nil || actingUserID == uuid.Nil {
					http.Error(w, "invalid X-On-Behalf-Of header", http.StatusBadRequest)
					return
				}
				openID := strings.TrimSpace(r.Header.Get("X-Feishu-Open-Id"))
				if openID == "" {
					http.Error(w, "missing X-Feishu-Open-Id header for on-behalf-of request", http.StatusBadRequest)
					return
				}
				if resolver == nil {
					http.Error(w, "on-behalf-of is not configured", http.StatusForbidden)
					return
				}
				if err := resolver.VerifyOnBehalfOf(ctx, identity.TenantID, actingUserID, openID); err != nil {
					http.Error(w, "on-behalf-of identity mismatch", http.StatusForbidden)
					return
				}
				ctx = context.WithValue(ctx, UserIDKey, actingUserID)
				ctx = context.WithValue(ctx, ActingUserIDKey, actingUserID)
				ctx = context.WithValue(ctx, ActingOpenIDKey, openID)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetServiceName 返回服务身份名(非服务请求返回空串)。
func GetServiceName(ctx context.Context) string {
	if v, ok := ctx.Value(ServiceNameKey).(string); ok {
		return v
	}
	return ""
}

// GetActingOpenID 返回 on-behalf-of 请求声明的飞书 open_id(无则空串)。
func GetActingOpenID(ctx context.Context) string {
	if v, ok := ctx.Value(ActingOpenIDKey).(string); ok {
		return v
	}
	return ""
}
