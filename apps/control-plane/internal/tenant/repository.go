package tenant

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	CreateTeam(ctx context.Context, params CreateTeamParams) (TeamRecord, error)
	ListTeams(ctx context.Context, params ListTeamsParams) ([]TeamRecord, error)
	GetTeam(ctx context.Context, tenantID, teamID uuid.UUID) (TeamRecord, error)
	CreateTeamConfigRevision(ctx context.Context, params CreateTeamConfigRevisionParams) (TeamConfigRevisionRecord, error)
	GetTeamConfigRevision(ctx context.Context, tenantID, revisionID uuid.UUID) (TeamConfigRevisionRecord, error)
	GetCurrentTeamConfigRevision(ctx context.Context, tenantID, teamID uuid.UUID) (TeamConfigRevisionRecord, error)
	GetNextTeamConfigRevisionNumber(ctx context.Context, tenantID, teamID uuid.UUID) (int32, error)
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
	Offset   int32
	Limit    int32
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

type TeamRecord = Team

type TeamConfigRevisionRecord = TeamConfigRevision
