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
	UserID     string    `json:"user_id"`
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
