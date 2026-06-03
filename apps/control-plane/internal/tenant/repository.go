package tenant

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	CreateTeam(ctx context.Context, params CreateTeamParams) (TeamRecord, error)
	ListTeams(ctx context.Context, params ListTeamsParams) ([]TeamRecord, error)
	ListTeamSummaries(ctx context.Context, params ListTeamSummariesParams) ([]TeamListItemRecord, error)
	GetTeamSummary(ctx context.Context, tenantID, teamID uuid.UUID) (TeamListItemRecord, error)
	GetTeam(ctx context.Context, tenantID, teamID uuid.UUID) (TeamRecord, error)
	UpdateTeam(ctx context.Context, params UpdateTeamParams) (TeamRecord, error)
	SetTeamStatus(ctx context.Context, params SetTeamStatusParams) (TeamRecord, error)
	CreateTeamConfigRevision(ctx context.Context, params CreateTeamConfigRevisionParams) (TeamConfigRevisionRecord, error)
	GetTeamConfigRevision(ctx context.Context, tenantID, revisionID uuid.UUID) (TeamConfigRevisionRecord, error)
	GetCurrentTeamConfigRevision(ctx context.Context, tenantID, teamID uuid.UUID) (TeamConfigRevisionRecord, error)
	GetNextTeamConfigRevisionNumber(ctx context.Context, tenantID, teamID uuid.UUID) (int32, error)
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
	TenantID         uuid.UUID
	Slug             string
	Name             string
	Status           TeamStatus
	HumanOwnerUserID *uuid.UUID
	Metadata         map[string]any
}

type ListTeamsParams struct {
	TenantID uuid.UUID
	Status   TeamStatus
	Q        string
	Offset   int32
	Limit    int32
}

type ListTeamSummariesParams = ListTeamsParams

type UpdateTeamParams struct {
	TenantID         uuid.UUID
	TeamID           uuid.UUID
	Slug             string
	Name             string
	HumanOwnerUserID *uuid.UUID
	Metadata         map[string]any
}

type SetTeamStatusParams struct {
	TenantID uuid.UUID
	TeamID   uuid.UUID
	Status   TeamStatus
}

type CreateTeamConfigRevisionParams struct {
	TenantID                    uuid.UUID
	TeamID                      uuid.UUID
	RevisionNumber              int32
	Constitution                map[string]any
	CapabilityPolicy            map[string]any
	ContextPolicy               map[string]any
	ApprovalPolicy              map[string]any
	ArtifactContract            map[string]any
	InternalCollaborationPolicy map[string]any
	RuntimeScopePolicy          map[string]any
	HumanOwnerUserID            *uuid.UUID
	Status                      TeamConfigRevisionStatus
	ApprovedBy                  *uuid.UUID
	ApprovedAt                  *time.Time
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

type TeamConfigRevisionRecord = TeamConfigRevision

type TeamMemberRecord = TeamMember

type TeamMemberRoleRequestRecord = TeamMemberRoleRequest
