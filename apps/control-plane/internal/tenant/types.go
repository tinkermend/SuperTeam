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

func (s GovernanceSummaryStatus) IsValid() bool {
	switch s {
	case GovernanceSummaryNotConfigured, GovernanceSummaryDraftPending, GovernanceSummaryActive, GovernanceSummaryNeedsUpdate:
		return true
	default:
		return false
	}
}

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

const (
	TeamRoleOwner    = "owner"
	TeamRoleAdmin    = "admin"
	TeamRoleApprover = "approver"
	TeamRoleMember   = "member"
	TeamRoleViewer   = "viewer"
)

type TeamMemberRoleRequestStatus string

const (
	TeamMemberRoleRequestStatusPending  TeamMemberRoleRequestStatus = "pending"
	TeamMemberRoleRequestStatusApproved TeamMemberRoleRequestStatus = "approved"
	TeamMemberRoleRequestStatusRejected TeamMemberRoleRequestStatus = "rejected"
)

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
	HumanOwner       *TeamHumanOwner
	Metadata         map[string]any
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type TeamHumanOwner struct {
	UserID      uuid.UUID
	Username    string
	DisplayName string
	Email       string
	Status      string
	Avatar      *UserAvatarConfig
}

type UserAvatarConfig struct {
	Provider string         `json:"provider"`
	Style    string         `json:"style"`
	Seed     string         `json:"seed"`
	Options  map[string]any `json:"options,omitempty"`
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

type TeamMember struct {
	MembershipID     uuid.UUID
	TenantID         uuid.UUID
	TeamID           uuid.UUID
	UserID           uuid.UUID
	Username         string
	DisplayName      string
	Email            string
	AccountStatus    string
	Avatar           *UserAvatarConfig
	Role             string
	MembershipStatus string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type TeamMemberRoleRequest struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	TeamID         uuid.UUID
	TargetUserID   uuid.UUID
	RequestedRole  string
	RequestedBy    uuid.UUID
	Status         TeamMemberRoleRequestStatus
	Reason         string
	DecidedBy      *uuid.UUID
	DecidedAt      *time.Time
	DecisionReason string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CreateTeamRequest struct {
	TenantID         uuid.UUID
	ActorUserID      uuid.UUID
	Slug             string
	Name             string
	Status           TeamStatus
	HumanOwnerUserID *uuid.UUID
	InitialMembers   []InitialTeamMemberInput
	Metadata         map[string]any
}

type InitialTeamMemberInput struct {
	UserID uuid.UUID `json:"user_id"`
	Role   string    `json:"role"`
}

type ListTeamsRequest struct {
	TenantID         uuid.UUID
	Status           TeamStatus
	GovernanceStatus GovernanceSummaryStatus
	Q                string
	Offset           int32
	Limit            int32
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

type AddTeamMemberRequest struct {
	TenantID uuid.UUID
	TeamID   uuid.UUID
	UserID   uuid.UUID
	Role     string
}

type RemoveTeamMemberRequest struct {
	TenantID     uuid.UUID
	TeamID       uuid.UUID
	MembershipID uuid.UUID
}

type CreateRoleRequestRequest struct {
	TenantID      uuid.UUID
	TeamID        uuid.UUID
	TargetUserID  uuid.UUID
	RequestedRole string
	RequestedBy   uuid.UUID
	Reason        string
}

type DecideRoleRequestRequest struct {
	TenantID       uuid.UUID
	TeamID         uuid.UUID
	RequestID      uuid.UUID
	DecidedBy      uuid.UUID
	DecisionReason string
}
