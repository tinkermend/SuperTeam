package teamlending

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrInvalidInput 标识请求参数非法（4xx）。
	ErrInvalidInput = errors.New("invalid lending input")
	// ErrNotFound 标识借调策略或请求不存在（404）。
	ErrNotFound = errors.New("lending record not found")
	// ErrPolicyNotFound 标识团队尚未配置借调策略。
	ErrPolicyNotFound = errors.New("team lending policy not found")
	// ErrLendingDisabled 标识团队明确关闭了借调。
	ErrLendingDisabled = errors.New("team lending is disabled")
	// ErrDuplicateRequest 标识同一 (project, team) 已存在有效借调请求。
	ErrDuplicateRequest = errors.New("active lending request already exists")
	// ErrInvalidTransition 标识请求状态机不允许的裁决（如对已裁决请求再次裁决）。
	ErrInvalidTransition = errors.New("invalid lending request status transition")
)

// ApprovalMode 借调审批模式。
type ApprovalMode string

const (
	// ApprovalModeAuto 符合策略天花板时自动放行，超纲则强制转人工。
	ApprovalModeAuto ApprovalMode = "auto"
	// ApprovalModeManual 每次借调都需团队负责人人工审批。
	ApprovalModeManual ApprovalMode = "manual"
)

func (m ApprovalMode) IsValid() bool {
	return m == ApprovalModeAuto || m == ApprovalModeManual
}

// RequestStatus 借调请求状态（D3：请求自带 status）。
type RequestStatus string

const (
	RequestStatusPending      RequestStatus = "pending"
	RequestStatusAutoApproved RequestStatus = "auto_approved"
	RequestStatusApproved     RequestStatus = "approved"
	RequestStatusRejected     RequestStatus = "rejected"
	RequestStatusRevoked      RequestStatus = "revoked"
)

func (s RequestStatus) IsValid() bool {
	switch s {
	case RequestStatusPending, RequestStatusAutoApproved, RequestStatusApproved, RequestStatusRejected, RequestStatusRevoked:
		return true
	default:
		return false
	}
}

// IsTerminal 返回该状态是否为终态（不可再裁决）。
func (s RequestStatus) IsTerminal() bool {
	return s == RequestStatusRejected || s == RequestStatusRevoked
}

// Policy 团队借调策略（供给侧预授权，每团队一条 active）。
type Policy struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	TeamID            uuid.UUID
	AllowLending      bool
	ApprovalMode      ApprovalMode
	BudgetCeiling     string         // 十进制字符串，空串表示未设上限
	CapabilityCeiling map[string]any // 能力/runtime scope 天花板
	ProjectMatch      map[string]any // 可调用的项目匹配条件
	Status            string
	CreatedByUserID   *uuid.UUID
	UpdatedByUserID   *uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Request 团队借调请求（需求侧发起 → 供给侧裁决）。
type Request struct {
	ID                  uuid.UUID
	TenantID            uuid.UUID
	TeamID              uuid.UUID
	ProjectID           uuid.UUID
	Status              RequestStatus
	RequestedByUserID   uuid.UUID
	RequestReason       string
	RequestedBudget     string         // 十进制字符串，空串表示未申请额度
	RequestedCapability map[string]any
	GrantedBudget       string
	GrantedCapability   map[string]any
	IsException         bool
	DecidedByUserID     *uuid.UUID
	DecidedAt           *time.Time
	DecisionReason      string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// UpsertPolicyInput 设置/更新团队借调策略。
type UpsertPolicyInput struct {
	TenantID          uuid.UUID
	TeamID            uuid.UUID
	ActorUserID       uuid.UUID
	AllowLending      bool
	ApprovalMode      ApprovalMode
	BudgetCeiling     string
	CapabilityCeiling map[string]any
	ProjectMatch      map[string]any
}

// CreateRequestInput 项目侧发起借调请求。
type CreateRequestInput struct {
	TenantID            uuid.UUID
	TeamID              uuid.UUID
	ProjectID           uuid.UUID
	RequestedByUserID   uuid.UUID
	RequestReason       string
	RequestedBudget     string
	RequestedCapability map[string]any
}

// DecideRequestInput 团队负责人裁决（approve/reject/revoke）。
type DecideRequestInput struct {
	TenantID          uuid.UUID
	TeamID            uuid.UUID
	RequestID         uuid.UUID
	DecidedByUserID   uuid.UUID
	DecisionReason    string
	GrantedBudget     string         // approve 时可覆盖授予额度
	GrantedCapability map[string]any // approve 时可覆盖授予能力
}
