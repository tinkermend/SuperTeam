package oplog

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type contextKey string

const (
	clientIPKey  contextKey = "oplog_client_ip"
	userAgentKey contextKey = "oplog_user_agent"
	requestIDKey contextKey = "oplog_request_id"
	usernameKey  contextKey = "oplog_username"
	userIDKey    contextKey = "oplog_user_id"
	tenantIDKey  contextKey = "oplog_tenant_id"
)

// Meta is request-scoped actor/network data copied into operation logs.
type Meta struct {
	TenantID  uuid.UUID
	UserID    uuid.UUID
	Username  string
	ClientIP  string
	UserAgent string
	RequestID string
}

// RequestMeta stores client IP / UA / request id on every request.
func RequestMeta(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ctx = context.WithValue(ctx, clientIPKey, clientIP(r))
		ctx = context.WithValue(ctx, userAgentKey, r.UserAgent())
		if requestID := strings.TrimSpace(r.Header.Get("X-Request-ID")); requestID != "" {
			ctx = context.WithValue(ctx, requestIDKey, requestID)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// WithActor stores the authenticated console user on the request context.
func WithActor(ctx context.Context, tenantID, userID uuid.UUID, username string) context.Context {
	if tenantID != uuid.Nil {
		ctx = context.WithValue(ctx, tenantIDKey, tenantID)
	}
	if userID != uuid.Nil {
		ctx = context.WithValue(ctx, userIDKey, userID)
	}
	if username = strings.TrimSpace(username); username != "" {
		ctx = context.WithValue(ctx, usernameKey, username)
	}
	return ctx
}

// MetaFromContext reads request-scoped operation-log fields.
func MetaFromContext(ctx context.Context) Meta {
	if ctx == nil {
		return Meta{}
	}
	meta := Meta{}
	if tenantID, ok := ctx.Value(tenantIDKey).(uuid.UUID); ok {
		meta.TenantID = tenantID
	}
	if userID, ok := ctx.Value(userIDKey).(uuid.UUID); ok {
		meta.UserID = userID
	}
	if username, ok := ctx.Value(usernameKey).(string); ok {
		meta.Username = username
	}
	if clientIP, ok := ctx.Value(clientIPKey).(string); ok {
		meta.ClientIP = clientIP
	}
	if userAgent, ok := ctx.Value(userAgentKey).(string); ok {
		meta.UserAgent = userAgent
	}
	if requestID, ok := ctx.Value(requestIDKey).(string); ok {
		meta.RequestID = requestID
	}
	return meta
}

func clientIP(r *http.Request) string {
	if forwardedFor := r.Header.Get("x-forwarded-for"); forwardedFor != "" {
		host, _, err := net.SplitHostPort(forwardedFor)
		if err == nil {
			return host
		}
		return forwardedFor
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
