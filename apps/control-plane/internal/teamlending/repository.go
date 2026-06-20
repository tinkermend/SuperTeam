package teamlending

import (
	"context"

	"github.com/google/uuid"
)

// Repository 团队借调持久化边界（application-controlled first）。
type Repository interface {
	UpsertPolicy(ctx context.Context, params UpsertPolicyParams) (Policy, error)
	GetPolicy(ctx context.Context, tenantID, teamID uuid.UUID) (Policy, error)
	CreateRequest(ctx context.Context, params CreateRequestParams) (Request, error)
	GetRequest(ctx context.Context, tenantID, requestID uuid.UUID) (Request, error)
	ListRequestsByTeam(ctx context.Context, params ListTeamRequestsParams) ([]Request, error)
	ListRequestsByProject(ctx context.Context, params ListProjectRequestsParams) ([]Request, error)
	ApproveRequest(ctx context.Context, params DecideRequestParams) (Request, error)
	RejectRequest(ctx context.Context, params DecideRequestParams) (Request, error)
	RevokeRequest(ctx context.Context, params DecideRequestParams) (Request, error)
	// GetTeamOwnerUserIDs 取团队的人类负责人（借调审批 inbox 目标）。
	GetTeamOwnerUserIDs(ctx context.Context, tenantID, teamID uuid.UUID) ([]uuid.UUID, error)
	// ListEffectiveLendingTeams 返回项目当前持有有效（approved/auto_approved）借调授权的团队集合，
	// 供协调线程挑数字员工前做借调闸门。
	ListEffectiveLendingTeams(ctx context.Context, tenantID, projectID uuid.UUID) ([]uuid.UUID, error)
}

// UpsertPolicyParams 设置/更新借调策略的仓储参数。
type UpsertPolicyParams struct {
	TenantID          uuid.UUID
	TeamID            uuid.UUID
	ActorUserID       uuid.UUID
	AllowLending      bool
	ApprovalMode      ApprovalMode
	BudgetCeiling     string
	CapabilityCeiling map[string]any
	ProjectMatch      map[string]any
}

// CreateRequestParams 创建借调请求的仓储参数（status/授予额度由 service 计算后传入）。
type CreateRequestParams struct {
	TenantID            uuid.UUID
	TeamID              uuid.UUID
	ProjectID           uuid.UUID
	RequestedByUserID   uuid.UUID
	RequestReason       string
	RequestedBudget     string
	RequestedCapability map[string]any
	Status              RequestStatus
	GrantedBudget       string
	GrantedCapability   map[string]any
	IsException         bool
}

// ListTeamRequestsParams 按团队列举借调请求。
type ListTeamRequestsParams struct {
	TenantID uuid.UUID
	TeamID   uuid.UUID
	Status   RequestStatus
	Offset   int32
	Limit    int32
}

// ListProjectRequestsParams 按项目列举借调请求。
type ListProjectRequestsParams struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
	Status    RequestStatus
	Offset    int32
	Limit     int32
}

// DecideRequestParams 裁决借调请求的仓储参数。
type DecideRequestParams struct {
	TenantID          uuid.UUID
	TeamID            uuid.UUID
	RequestID         uuid.UUID
	DecidedByUserID   uuid.UUID
	DecisionReason    string
	GrantedBudget     string
	GrantedCapability map[string]any
}
