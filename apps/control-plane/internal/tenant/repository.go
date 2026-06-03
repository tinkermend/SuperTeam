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
	ListTeamConfigDrafts(ctx context.Context, params ListTeamConfigDraftsParams) ([]TeamConfigRevisionRecord, error)
	UpdateTeamConfigRevisionDraft(ctx context.Context, params UpdateTeamConfigRevisionDraftParams) (TeamConfigRevisionRecord, error)
	ApproveTeamConfigRevision(ctx context.Context, params ActivateTeamConfigRevisionParams) (TeamConfigRevisionRecord, error)
	RejectTeamConfigRevision(ctx context.Context, tenantID, teamID, revisionID uuid.UUID) (TeamConfigRevisionRecord, error)
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

type ListTeamConfigDraftsParams struct {
	TenantID uuid.UUID
	TeamID   uuid.UUID
	Offset   int32
	Limit    int32
}

type UpdateTeamConfigRevisionDraftParams struct {
	TenantID                    uuid.UUID
	TeamID                      uuid.UUID
	RevisionID                  uuid.UUID
	Constitution                map[string]any
	CapabilityPolicy            map[string]any
	ContextPolicy               map[string]any
	ApprovalPolicy              map[string]any
	ArtifactContract            map[string]any
	InternalCollaborationPolicy map[string]any
	RuntimeScopePolicy          map[string]any
	HumanOwnerUserID            *uuid.UUID
}

type ActivateTeamConfigRevisionParams struct {
	TenantID   uuid.UUID
	TeamID     uuid.UUID
	RevisionID uuid.UUID
	ApprovedBy uuid.UUID
}

type TeamRecord = Team

type TeamListItemRecord = TeamListItem

type TeamConfigRevisionRecord = TeamConfigRevision
