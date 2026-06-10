package project

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidProject       = errors.New("invalid project")
	ErrInvalidProjectMember = errors.New("invalid project member")
	ErrProjectNotFound      = errors.New("project not found")
	ErrProjectArchived      = errors.New("project archived")
)

type ProjectStatus string

const (
	ProjectStatusDraft       ProjectStatus = "draft"
	ProjectStatusConfiguring ProjectStatus = "configuring"
	ProjectStatusRunning     ProjectStatus = "running"
	ProjectStatusPaused      ProjectStatus = "paused"
	ProjectStatusAcceptance  ProjectStatus = "acceptance"
	ProjectStatusArchived    ProjectStatus = "archived"
)

type PrincipalType string

const (
	PrincipalTypeHumanUser       PrincipalType = "human_user"
	PrincipalTypeDigitalEmployee PrincipalType = "digital_employee"
	PrincipalTypeTeam            PrincipalType = "team"
)

type ProjectRole string

const (
	ProjectRoleOwner      ProjectRole = "owner"
	ProjectRoleLeader     ProjectRole = "leader"
	ProjectRoleAcceptance ProjectRole = "acceptance"
	ProjectRoleExecutor   ProjectRole = "executor"
	ProjectRoleReviewer   ProjectRole = "reviewer"
	ProjectRoleObserver   ProjectRole = "observer"
)

type ProjectEventType string

const (
	ProjectEventCreated         ProjectEventType = "project.created"
	ProjectEventConfigChanged   ProjectEventType = "project.config.changed"
	ProjectEventArchived        ProjectEventType = "project.archived"
	ProjectEventDemandSubmitted ProjectEventType = "demand.submitted"
)

type DemandSourceType string

const (
	DemandSourceManual   DemandSourceType = "manual"
	DemandSourceGithub   DemandSourceType = "github"
	DemandSourceTicket   DemandSourceType = "ticket"
	DemandSourceDocument DemandSourceType = "document"
	DemandSourceLog      DemandSourceType = "log"
)

type ProjectDemandStatus string

const (
	ProjectDemandStatusSubmitted       ProjectDemandStatus = "submitted"
	ProjectDemandStatusRecorded        ProjectDemandStatus = "recorded"
	ProjectDemandStatusPlanningPending ProjectDemandStatus = "planning_pending"
	ProjectDemandStatusCancelled       ProjectDemandStatus = "cancelled"
)

type Project struct {
	ID                     uuid.UUID
	TenantID               uuid.UUID
	TeamID                 *uuid.UUID
	Name                   string
	Description            *string
	Goal                   string
	Status                 ProjectStatus
	HumanOwnerUserID       uuid.UUID
	LeaderUserID           *uuid.UUID
	AcceptanceUserID       *uuid.UUID
	CoordinationWorkflowID string
	CoordinationStatus     string
	CoordinationPolicy     map[string]any
	ApprovalPolicy         map[string]any
	EvidencePolicy         map[string]any
	ArchivedAt             *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type ProjectMember struct {
	ID                  uuid.UUID
	TenantID            uuid.UUID
	ProjectID           uuid.UUID
	PrincipalType       PrincipalType
	PrincipalID         uuid.UUID
	ProjectRole         ProjectRole
	DisplayNameSnapshot *string
	Status              string
	Settings            map[string]any
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type ProjectTask struct {
	ID                        uuid.UUID
	TenantID                  uuid.UUID
	ProjectID                 uuid.UUID
	DemandID                  *uuid.UUID
	Title                     string
	Summary                   *string
	Status                    string
	AssignedDigitalEmployeeID *uuid.UUID
	RiskLevel                 *string
	RequiresHumanApproval     bool
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

type ProjectEvent struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	ProjectID      uuid.UUID
	SequenceNumber int64
	EventType      ProjectEventType
	ActorType      string
	ActorID        string
	ResourceType   *string
	ResourceID     *string
	Summary        *string
	Payload        map[string]any
	CreatedAt      time.Time
}

type ProjectDemand struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	SubmittedByUserID uuid.UUID
	Title             string
	Content           *string
	SourceType        DemandSourceType
	Status            ProjectDemandStatus
	CreatedEventID    *uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type ProjectConfigRevision struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	RevisionNumber  int32
	ConfigSnapshot  map[string]any
	ChangeSummary   *string
	CreatedByUserID uuid.UUID
	CreatedEventID  *uuid.UUID
	CreatedAt       time.Time
}

type ProjectMemberInput struct {
	PrincipalType       PrincipalType
	PrincipalID         uuid.UUID
	ProjectRole         ProjectRole
	DisplayNameSnapshot string
	Settings            map[string]any
}

type CreateProjectRequest struct {
	TenantID           uuid.UUID
	TeamID             *uuid.UUID
	ActorUserID        uuid.UUID
	Name               string
	Description        string
	Goal               string
	HumanOwnerUserID   uuid.UUID
	LeaderUserID       *uuid.UUID
	AcceptanceUserID   *uuid.UUID
	Members            []ProjectMemberInput
	CoordinationPolicy map[string]any
	ApprovalPolicy     map[string]any
	EvidencePolicy     map[string]any
}

type CreateProjectResult struct {
	Project Project
	Members []ProjectMember
}

type UpdateProjectConfigRequest struct {
	TenantID           uuid.UUID
	ProjectID          uuid.UUID
	ActorUserID        uuid.UUID
	Name               string
	Description        string
	Goal               string
	HumanOwnerUserID   uuid.UUID
	LeaderUserID       *uuid.UUID
	AcceptanceUserID   *uuid.UUID
	Members            []ProjectMemberInput
	CoordinationPolicy map[string]any
	ApprovalPolicy     map[string]any
	EvidencePolicy     map[string]any
}

type SubmitProjectDemandRequest struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	SubmittedByUserID uuid.UUID
	Title             string
	Content           string
	SourceType        DemandSourceType
}

type ListProjectsRequest struct {
	TenantID uuid.UUID
	Status   *ProjectStatus
	Query    string
	Limit    int32
	Offset   int32
}

type ProjectStatusSummary struct {
	CurrentPhase string
	IsArchived   bool
}

type ProjectTaskSummary struct {
	ActiveTasks       int
	PendingHumanTasks int
	CompletedTasks    int
	FailedTasks       int
}

type ProjectCoordinationWorkflow struct {
	WorkflowID string
	Status     string
}

type ProjectOverview struct {
	Project              Project
	HumanRoles           []ProjectMember
	DigitalEmployeePool  []ProjectMember
	StatusSummary        ProjectStatusSummary
	TaskSummary          ProjectTaskSummary
	ActiveTasks          []ProjectTask
	RecentEvents         []ProjectEvent
	CoordinationWorkflow ProjectCoordinationWorkflow
}
