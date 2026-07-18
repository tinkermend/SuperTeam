package tenant

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	CreateTeam(ctx context.Context, params CreateTeamParams) (TeamRecord, error)
	CreateTeamWithInitialMembers(ctx context.Context, params CreateTeamWithInitialMembersParams) (TeamRecord, error)
	ListTeams(ctx context.Context, params ListTeamsParams) ([]TeamRecord, error)
	ListTeamSummaries(ctx context.Context, params ListTeamSummariesParams) ([]TeamListItemRecord, error)
	GetTeamSummary(ctx context.Context, tenantID, teamID uuid.UUID) (TeamListItemRecord, error)
	GetTeam(ctx context.Context, tenantID, teamID uuid.UUID) (TeamRecord, error)
	UpdateTeam(ctx context.Context, params UpdateTeamParams) (TeamRecord, error)
	UpdateTeamConstitution(ctx context.Context, tenantID, teamID uuid.UUID, constitution map[string]any) (TeamRecord, error)
	DeleteTeam(ctx context.Context, tenantID, teamID uuid.UUID) error
	ListTeamMembers(ctx context.Context, params ListTeamMembersParams) ([]TeamMemberRecord, error)
	GetTeamMember(ctx context.Context, tenantID, teamID, membershipID uuid.UUID) (TeamMemberRecord, error)
	AddTeamMember(ctx context.Context, params AddTeamMemberParams) (TeamMemberRecord, error)
	DisableTeamMemberRole(ctx context.Context, params DisableTeamMemberRoleParams) (TeamMemberRecord, error)
	CountTeamOwners(ctx context.Context, tenantID, teamID uuid.UUID) (int32, error)
	CreateTeamMemberRoleRequest(ctx context.Context, params CreateTeamMemberRoleRequestParams) (TeamMemberRoleRequestRecord, error)
	GetTeamMemberRoleRequest(ctx context.Context, tenantID, teamID, requestID uuid.UUID) (TeamMemberRoleRequestRecord, error)
	ListTeamMemberRoleRequests(ctx context.Context, params ListTeamMemberRoleRequestsParams) ([]TeamMemberRoleRequestRecord, error)
	ApproveTeamMemberRoleRequest(ctx context.Context, params DecideTeamMemberRoleRequestParams) (TeamMemberRoleRequestRecord, error)
	DecideTeamMemberRoleRequest(ctx context.Context, params DecideTeamMemberRoleRequestParams) (TeamMemberRoleRequestRecord, error)
}

type CreateTeamParams struct {
	TenantID          uuid.UUID
	Slug              string
	Name              string
	Status            TeamStatus
	HumanOwnerUserIDs []uuid.UUID
	Metadata          map[string]any
}

type CreateTeamWithInitialMembersParams struct {
	TenantID                  uuid.UUID
	ActorUserID               uuid.UUID
	Slug                      string
	Name                      string
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
	TenantID          uuid.UUID
	TeamID            uuid.UUID
	Slug              string
	Name              string
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
	TenantID uuid.UUID
	TeamID   uuid.UUID
	UserID   uuid.UUID
	Role     string
}

type DisableTeamMemberRoleParams struct {
	TenantID     uuid.UUID
	TeamID       uuid.UUID
	MembershipID uuid.UUID
}

type CreateTeamMemberRoleRequestParams struct {
	TenantID      uuid.UUID
	TeamID        uuid.UUID
	TargetUserID  uuid.UUID
	RequestedRole string
	RequestedBy   uuid.UUID
	Reason        string
}

type ListTeamMemberRoleRequestsParams struct {
	TenantID uuid.UUID
	TeamID   uuid.UUID
	Status   TeamMemberRoleRequestStatus
	Offset   int32
	Limit    int32
}

type DecideTeamMemberRoleRequestParams struct {
	TenantID       uuid.UUID
	TeamID         uuid.UUID
	RequestID      uuid.UUID
	Status         TeamMemberRoleRequestStatus
	DecidedBy      uuid.UUID
	DecisionReason string
}

type TeamRecord = Team

type TeamListItemRecord = TeamListItem

type TeamMemberRecord = TeamMember

type TeamMemberRoleRequestRecord = TeamMemberRoleRequest
