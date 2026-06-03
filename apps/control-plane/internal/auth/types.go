package auth

import (
	"time"

	"github.com/google/uuid"
)

// User 用户模型
type User struct {
	ID           uuid.UUID  `db:"id"`
	Username     string     `db:"username"`
	PasswordHash string     `db:"password_hash"`
	Status       string     `db:"status"`
	CreatedAt    time.Time  `db:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at"`
	LastLoginAt  *time.Time `db:"last_login_at"`
}

// Session 会话模型
type Session struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	ExpiresAt  time.Time `json:"expires_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ClientIP   string    `json:"client_ip"`
	UserAgent  string    `json:"user_agent"`
}

// contextKey 用于 context 存储
type contextKey string

const (
	// UserContextKey 用户信息在 context 中的 key
	UserContextKey contextKey = "user"
)

const (
	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
)

const (
	LoginEventSucceeded       = "login_succeeded"
	LoginEventFailed          = "login_failed"
	LoginEventLogoutSucceeded = "logout_succeeded"

	LoginResultSucceeded = "succeeded"
	LoginResultFailed    = "failed"

	LoginFailureInvalidCredentials = "invalid_credentials"
	LoginFailureUserDisabled       = "user_disabled"
)

const (
	OperationModuleAuth              = "auth"
	OperationResourceUser            = "auth_user"
	OperationActionUserCreate        = "user.create"
	OperationActionUserEnable        = "user.enable"
	OperationActionUserDisable       = "user.disable"
	OperationActionUserResetPassword = "user.reset_password"
	OperationResultSucceeded         = "succeeded"
	OperationResultFailed            = "failed"
)

// Actor 表示执行 Web 管理操作的当前登录用户。
type Actor struct {
	UserID   uuid.UUID
	Username string
}

// ListUsersFilter 用户列表过滤条件。
type ListUsersFilter struct {
	Q      string
	Status string
	Limit  int32
	Offset int32
}

// CreateManagedUserInput 创建平台用户的输入。
type CreateManagedUserInput struct {
	Username string
	Password string
}

// LoginLog Web 控制台登录日志。
type LoginLog struct {
	ID            uuid.UUID
	EventType     string
	UserID        *uuid.UUID
	Username      string
	SessionID     *uuid.UUID
	ClientIP      string
	UserAgent     string
	Result        string
	FailureReason string
	CreatedAt     time.Time
}

// CreateLoginLogParams 创建 Web 控制台登录日志所需字段。
type CreateLoginLogParams struct {
	EventType     string
	UserID        *uuid.UUID
	Username      string
	SessionID     *uuid.UUID
	ClientIP      string
	UserAgent     string
	Result        string
	FailureReason string
}

// ListLoginLogsFilter 登录日志列表过滤参数。
type ListLoginLogsFilter struct {
	Limit  int32
	Offset int32
}

// CreateOperationLogParams 创建 Web 控制台操作日志所需字段。
type CreateOperationLogParams struct {
	UserID       *uuid.UUID
	Username     string
	Module       string
	ResourceType string
	ResourceID   string
	Action       string
	Result       string
	ClientIP     string
	UserAgent    string
}
