package auth

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidManagedUserInput = errors.New("invalid managed user input")
	ErrManagedUserNotFound     = errors.New("managed user not found")
)

// User 用户模型
type User struct {
	ID            uuid.UUID        `db:"id"`
	Username      string           `db:"username"`
	DisplayName   string           `db:"display_name"`
	Email         string           `db:"email"`
	PasswordHash  string           `db:"password_hash"`
	Status        string           `db:"status"`
	Avatar        UserAvatarConfig `db:"-"`
	AvatarAssetID string           `db:"avatar_asset_id"`
	CreatedAt     time.Time        `db:"created_at"`
	UpdatedAt     time.Time        `db:"updated_at"`
}

// UserAvatarConfig 表示平台用户头像生成配置。当前只支持 DiceBear，但保留结构化扩展字段。
type UserAvatarConfig struct {
	Provider string         `json:"provider"`
	Style    string         `json:"style"`
	Seed     string         `json:"seed"`
	Options  map[string]any `json:"options,omitempty"`
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
	LoginFailureCaptchaInvalid     = "captcha_invalid"
	LoginFailureCaptchaExpired     = "captcha_expired"
)

type CaptchaChallenge struct {
	Enabled      bool
	ID           uuid.UUID
	ImageDataURL string
	ExpiresAt    time.Time
}

type CaptchaChallengeRecord struct {
	ID         uuid.UUID
	TenantID   uuid.UUID
	AnswerHash string
	ExpiresAt  time.Time
	UsedAt     *time.Time
	ClientIP   string
	UserAgent  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type CreateCaptchaChallengeParams struct {
	ID         uuid.UUID
	TenantID   uuid.UUID
	AnswerHash string
	ExpiresAt  time.Time
	ClientIP   string
	UserAgent  string
}

const (
	OperationModuleAuth                  = "auth"
	OperationResourceUser                = "auth_user"
	OperationActionUserCreate            = "user.create"
	OperationActionUserEnable            = "user.enable"
	OperationActionUserDisable           = "user.disable"
	OperationActionUserResetPassword     = "user.reset_password"
	OperationActionUserUpdateOwnProfile  = "user.update_own_profile"
	OperationActionUserChangeOwnPassword = "user.change_own_password"
	OperationResultSucceeded             = "succeeded"
	OperationResultFailed                = "failed"
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
	TenantID          uuid.UUID
	Username          string
	DisplayName       string
	Password          string
	Avatar            UserAvatarConfig
	AvatarAssetID     string
	SelectableTeamIDs []uuid.UUID
}

// UserProjectTeamScopeSummary 表示人类用户创建项目时可选择的团队授权范围。
type UserProjectTeamScopeSummary struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	UserID          uuid.UUID
	TeamID          uuid.UUID
	Status          string
	GrantedByUserID *uuid.UUID
	RevokedAt       *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Team            UserProjectTeamScopeTeamSummary
}

// UserProjectTeamScopeTeamSummary 表示授权范围中的团队摘要和治理状态。
type UserProjectTeamScopeTeamSummary struct {
	ID                   uuid.UUID
	Slug                 string
	Name                 string
	Status               string
	DigitalEmployeeCount int32
	CurrentRevision      *int32
	PendingDraftCount    int32
	GovernanceStatus     string
	RiskSummary          string
	HumanOwners          []UserProjectTeamScopeOwnerSummary
}

// UserProjectTeamScopeOwnerSummary 表示团队固定人类负责人摘要。
type UserProjectTeamScopeOwnerSummary struct {
	ID            uuid.UUID
	Username      string
	DisplayName   string
	Email         string
	Status        string
	Avatar        UserAvatarConfig
	AvatarAssetID string
}

// UpdateUserProfileInput 更新当前登录用户自服务资料的输入。
type UpdateUserProfileInput struct {
	DisplayName string
	Email       string
	Avatar      UserAvatarConfig
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
	UserID    *uuid.UUID
	EventType string
	Result    string
	Limit     int32
	Offset    int32
}

// OperationLog Web 控制台操作日志领域对象。
type OperationLog struct {
	ID           uuid.UUID
	UserID       *uuid.UUID
	Username     string
	Module       string
	ResourceType string
	ResourceID   string
	Action       string
	Result       string
	RequestID    string
	ClientIP     string
	UserAgent    string
	CreatedAt    time.Time
}

// ListOperationLogsFilter Web 控制台操作日志查询过滤条件。
type ListOperationLogsFilter struct {
	UserID *uuid.UUID
	Module string
	Action string
	Result string
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
