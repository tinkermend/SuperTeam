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

// 团队生命周期收敛（spec 2026-07-18-team-lifecycle-convergence）：撤销归档/停用，
// 存活团队唯一状态为 active；删除由 deleted_at 表达（软删+审计），不再经 status。
const (
	TeamStatusActive TeamStatus = "active"
)

func (s TeamStatus) IsValid() bool {
	return s == TeamStatusActive
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

const (
	TeamRoleOwner    = "owner"
	TeamRoleAdmin    = "admin"
	TeamRoleApprover = "approver"
	TeamRoleMember   = "member"
	TeamRoleViewer   = "viewer"
)

const (
	MaxDigitalEmployeesPerTeam = 10
	MaxTeamDescriptionLength   = 280
)

type ValidationIssue struct {
	Field    string
	Message  string
	Severity string
}

type Team struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	Slug              string
	Name              string
	Description       string
	Status            TeamStatus
	HumanOwnerUserIDs []uuid.UUID
	HumanOwners       []TeamHumanOwner
	Constitution      map[string]any
	Metadata          map[string]any
	CreatedAt         time.Time
	UpdatedAt         time.Time
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
	PendingDraftCount    int32
	RiskSummary          string
}

type TeamOverview struct {
	Team                 *Team
	MemberCount          int32
	DigitalEmployeeCount int32
	CapabilityCount      int32
	PendingDraftCount    int32
	PendingItemCount     int32
	AllowedActions       []AllowedTeamAction
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

type CreateTeamRequest struct {
	TenantID                  uuid.UUID
	ActorUserID               uuid.UUID
	Slug                      string
	Name                      string
	Description               string
	Status                    TeamStatus
	HumanOwnerUserIDs         []uuid.UUID
	InitialMembers            []InitialTeamMemberInput
	InitialDigitalEmployeeIDs []uuid.UUID
	Metadata                  map[string]any
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
	TenantID          uuid.UUID
	TeamID            uuid.UUID
	Name              string
	Slug              string
	Description       *string
	HumanOwnerUserIDs []uuid.UUID
	Metadata          map[string]any
}

type UpdateTeamConstitutionRequest struct {
	TenantID     uuid.UUID
	TeamID       uuid.UUID
	Constitution map[string]any
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
