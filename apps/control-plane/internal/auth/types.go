package auth

import "time"

// User 用户模型
type User struct {
	ID           int64      `db:"id"`
	Username     string     `db:"username"`
	PasswordHash string     `db:"password_hash"`
	Status       string     `db:"status"`
	CreatedAt    time.Time  `db:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at"`
	LastLoginAt  *time.Time `db:"last_login_at"`
}

// Session 会话模型
type Session struct {
	ID         string    `json:"id"`
	UserID     int64     `json:"user_id"`
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
	LoginEventSucceeded       = "login_succeeded"
	LoginEventFailed          = "login_failed"
	LoginEventLogoutSucceeded = "logout_succeeded"

	LoginResultSucceeded = "succeeded"
	LoginResultFailed    = "failed"

	LoginFailureInvalidCredentials = "invalid_credentials"
	LoginFailureUserDisabled       = "user_disabled"
)

// LoginLog Web 控制台登录日志。
type LoginLog struct {
	ID            int64
	EventType     string
	UserID        *int64
	Username      string
	SessionID     string
	ClientIP      string
	UserAgent     string
	Result        string
	FailureReason string
	CreatedAt     time.Time
}

// CreateLoginLogParams 创建 Web 控制台登录日志所需字段。
type CreateLoginLogParams struct {
	EventType     string
	UserID        *int64
	Username      string
	SessionID     string
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
