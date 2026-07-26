package tenant

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/teamguard"
)

type Repository interface {
	CreateTeam(ctx context.Context, params CreateTeamParams) (TeamRecord, error)
	CreateTeamWithInitialMembers(ctx context.Context, params CreateTeamWithInitialMembersParams) (TeamRecord, error)
	ListTeams(ctx context.Context, params ListTeamsParams) ([]TeamRecord, error)
	ListTeamSummaries(ctx context.Context, params ListTeamSummariesParams) ([]TeamListItemRecord, error)
	GetTeamSummary(ctx context.Context, tenantID, teamID uuid.UUID) (TeamListItemRecord, error)
	GetTeam(ctx context.Context, tenantID, teamID uuid.UUID) (TeamRecord, error)
	UpdateTeam(ctx context.Context, params UpdateTeamParams) (TeamRecord, error)
	UpdateTeamConstitution(ctx context.Context, tenantID, teamID, actorUserID uuid.UUID, constitution map[string]any) (TeamRecord, error)
	// 宪法版本化（spec §5.3）：保存与回滚都是追加新版本，历史不改写。
	CreateTeamConstitutionRevision(ctx context.Context, params CreateTeamConstitutionRevisionParams) (TeamConstitutionRevision, error)
	ListTeamConstitutionRevisions(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int32) ([]TeamConstitutionRevision, error)
	GetTeamConstitutionRevision(ctx context.Context, tenantID, teamID uuid.UUID, revisionNumber int32) (TeamConstitutionRevision, error)
	DeleteTeam(ctx context.Context, tenantID, teamID, actorUserID uuid.UUID) error
	// P2 审核确认制:待确认队列/恢复/确认物理删除(spec 2026-07-18-team-lifecycle-convergence §2)。
	ListPendingDeleteTeams(ctx context.Context, tenantID uuid.UUID) ([]PendingDeleteTeamRecord, error)
	ListStalePendingDeleteTeams(ctx context.Context, staleBefore time.Time) ([]PendingDeleteTeamRecord, error)
	ResolveOrphanPendingDeleteReminders(ctx context.Context) error
	RestorePendingDeleteTeam(ctx context.Context, tenantID, teamID, actorUserID uuid.UUID) (TeamRecord, error)
	ConfirmTeamDelete(ctx context.Context, tenantID, teamID, actorUserID uuid.UUID) (TeamRecord, error)
	ListTeamMembers(ctx context.Context, params ListTeamMembersParams) ([]TeamMemberRecord, error)
	GetTeamMember(ctx context.Context, tenantID, teamID, membershipID uuid.UUID) (TeamMemberRecord, error)
	RequireActiveTenantLevelMembership(ctx context.Context, tenantID, userID uuid.UUID) error
	AddTeamMember(ctx context.Context, params AddTeamMemberParams) (TeamMemberRecord, error)
	GrantTeamMemberRole(ctx context.Context, in GrantTeamRoleInput) (TeamMemberRecord, error)
	BindTeamDigitalEmployee(ctx context.Context, params BindTeamDigitalEmployeeParams) error
	UnbindTeamDigitalEmployee(ctx context.Context, params BindTeamDigitalEmployeeParams) error
	ListDigitalEmployeeDetachBlockers(ctx context.Context, tenantID, employeeID uuid.UUID) ([]DetachBlocker, error)
	DisableTeamMemberRole(ctx context.Context, params DisableTeamMemberRoleParams) (TeamMemberRecord, error)
	ChangeTeamMemberRole(ctx context.Context, params ChangeTeamMemberRoleParams) (TeamMemberRecord, error)
	CountTeamOwners(ctx context.Context, tenantID, teamID uuid.UUID) (int32, error)
}

type CreateTeamParams struct {
	TenantID          uuid.UUID
	Slug              string
	Name              string
	Description       string
	Status            TeamStatus
	HumanOwnerUserIDs []uuid.UUID
	Metadata          map[string]any
}

type CreateTeamWithInitialMembersParams struct {
	TenantID                  uuid.UUID
	ActorUserID               uuid.UUID
	Slug                      string
	Name                      string
	Description               string
	Status                    TeamStatus
	OwnerUserIDs              []uuid.UUID
	InitialMembers            []InitialTeamMemberInput
	InitialDigitalEmployeeIDs []uuid.UUID
	Metadata                  map[string]any
}

type ListTeamsParams struct {
	TenantID         uuid.UUID
	Status           TeamStatus
	GovernanceStatus GovernanceSummaryStatus
	Q                string
	Offset           int32
	Limit            int32
}

type ListTeamSummariesParams = ListTeamsParams

type UpdateTeamParams struct {
	ActorUserID       uuid.UUID
	TenantID          uuid.UUID
	TeamID            uuid.UUID
	Slug              string
	Name              string
	Description       string
	HumanOwnerUserIDs []uuid.UUID
	Metadata          map[string]any
}

type ListTeamMembersParams struct {
	TenantID uuid.UUID
	TeamID   uuid.UUID
	Offset   int32
	Limit    int32
}

type AddTeamMemberParams struct {
	ActorUserID uuid.UUID
	TenantID    uuid.UUID
	TeamID      uuid.UUID
	UserID      uuid.UUID
	Role        string
}

type BindTeamDigitalEmployeeParams struct {
	TenantID    uuid.UUID
	TeamID      uuid.UUID
	EmployeeID  uuid.UUID
	ActorUserID uuid.UUID
}

// DetachBlocker 是数字员工脱离团队（移出回候岗 / 换队）的阻断项。判据与消息
// 由 teamguard 包统一持有——employee 包的换队路径用同一套，不允许两处各写一份。
type DetachBlocker = teamguard.DetachBlocker

type DisableTeamMemberRoleParams struct {
	TenantID     uuid.UUID
	TeamID       uuid.UUID
	MembershipID uuid.UUID
	ActorUserID  uuid.UUID
}

// ChangeTeamMemberRoleParams 直接角色变更（member ⇄ viewer）。特权角色不走这里，
// 走权限中心审批（requestTeamPrivilegedRole）。
type CreateTeamConstitutionRevisionParams struct {
	TenantID    uuid.UUID
	TeamID      uuid.UUID
	Rules       []ConstitutionRule
	ChangeNote  string
	ActorUserID uuid.UUID
}

type ChangeTeamMemberRoleParams struct {
	TenantID     uuid.UUID
	TeamID       uuid.UUID
	MembershipID uuid.UUID
	Role         string
	ActorUserID  uuid.UUID
}

type TeamRecord = Team

// PendingDeleteTeamRecord 待确认删除队列条目:附带删除时间(滞留时长)与发起人。
type PendingDeleteTeamRecord struct {
	Team
	DeletedAt         time.Time
	DeleteRequestedBy *uuid.UUID
}

type TeamListItemRecord = TeamListItem

type TeamMemberRecord = TeamMember
