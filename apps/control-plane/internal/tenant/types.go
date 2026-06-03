package tenant

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput = errors.New("invalid tenant input")
	ErrNotFound     = errors.New("tenant not found")
)

type TeamStatus string

const (
	TeamStatusActive   TeamStatus = "active"
	TeamStatusDisabled TeamStatus = "disabled"
	TeamStatusArchived TeamStatus = "archived"
)

func (s TeamStatus) IsValid() bool {
	switch s {
	case TeamStatusActive, TeamStatusDisabled, TeamStatusArchived:
		return true
	default:
		return false
	}
}

type GovernanceSummaryStatus string

const (
	GovernanceSummaryNotConfigured GovernanceSummaryStatus = "not_configured"
	GovernanceSummaryDraftPending  GovernanceSummaryStatus = "draft_pending"
	GovernanceSummaryActive        GovernanceSummaryStatus = "active"
	GovernanceSummaryNeedsUpdate   GovernanceSummaryStatus = "needs_update"
)

type AllowedTeamAction string

type TeamConfigRevisionStatus string

const (
	TeamConfigRevisionStatusDraft    TeamConfigRevisionStatus = "draft"
	TeamConfigRevisionStatusActive   TeamConfigRevisionStatus = "active"
	TeamConfigRevisionStatusRejected TeamConfigRevisionStatus = "rejected"
	TeamConfigRevisionStatusArchived TeamConfigRevisionStatus = "archived"
)

func (s TeamConfigRevisionStatus) IsValid() bool {
	switch s {
	case TeamConfigRevisionStatusDraft, TeamConfigRevisionStatusActive, TeamConfigRevisionStatusRejected, TeamConfigRevisionStatusArchived:
		return true
	default:
		return false
	}
}

type ValidationIssue struct {
	Field    string
	Message  string
	Severity string
}

type GovernanceDraftInput struct {
	Constitution                map[string]any
	CapabilityPolicy            map[string]any
	ContextPolicy               map[string]any
	ApprovalPolicy              map[string]any
	ArtifactContract            map[string]any
	InternalCollaborationPolicy map[string]any
	RuntimeScopePolicy          map[string]any
	HumanOwnerUserID            *uuid.UUID
}

type GovernanceDiffSummary struct {
	AddedHardRules       int32
	ChangedCapabilities  int32
	ChangedApprovalRules int32
	Warnings             []ValidationIssue
	BlockingErrors       []ValidationIssue
}

type Team struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	Slug             string
	Name             string
	Status           TeamStatus
	HumanOwnerUserID *uuid.UUID
	Metadata         map[string]any
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type TeamListItem struct {
	Team
	MemberCount          int32
	DigitalEmployeeCount int32
	CapabilityCount      int32
	GovernanceStatus     GovernanceSummaryStatus
	CurrentRevision      *int32
	PendingDraftCount    int32
	RiskSummary          string
}

type TeamOverview struct {
	Team                 *Team
	MemberCount          int32
	DigitalEmployeeCount int32
	CapabilityCount      int32
	CurrentRevision      *TeamConfigRevision
	PendingDraftCount    int32
	PendingItemCount     int32
	AllowedActions       []AllowedTeamAction
}

type TeamConfigRevision struct {
	ID                          uuid.UUID
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
	CreatedAt                   time.Time
	UpdatedAt                   time.Time
}

type CreateTeamRequest struct {
	TenantID         uuid.UUID
	Slug             string
	Name             string
	Status           TeamStatus
	HumanOwnerUserID *uuid.UUID
	Metadata         map[string]any
}

type ListTeamsRequest struct {
	TenantID uuid.UUID
	Status   TeamStatus
	Q        string
	Offset   int32
	Limit    int32
}

type UpdateTeamRequest struct {
	TenantID         uuid.UUID
	TeamID           uuid.UUID
	Name             string
	Slug             string
	HumanOwnerUserID *uuid.UUID
	Metadata         map[string]any
}

type ChangeTeamStatusRequest struct {
	TenantID uuid.UUID
	TeamID   uuid.UUID
	Status   TeamStatus
}

type CreateTeamConfigRevisionRequest struct {
	TenantID                    uuid.UUID
	TeamID                      uuid.UUID
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
}
