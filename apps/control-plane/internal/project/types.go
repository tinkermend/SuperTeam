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

	ProjectEventWorkflowSignaled       ProjectEventType = "workflow.signaled"
	ProjectEventCoordinationJobCreated ProjectEventType = "coordination_job.created"
	ProjectEventRouteDecisionCreated   ProjectEventType = "route_decision.created"
	ProjectEventTaskCreated            ProjectEventType = "project_task.created"
	ProjectEventTaskDispatched         ProjectEventType = "project_task.dispatched"
	ProjectEventTaskCompleted          ProjectEventType = "project_task.completed"
	ProjectEventTaskFailed             ProjectEventType = "project_task.failed"
	ProjectEventTransferRequested      ProjectEventType = "transfer.requested"
	ProjectEventDecisionRequested      ProjectEventType = "decision.requested"
	ProjectEventDecisionSubmitted      ProjectEventType = "decision.submitted"
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

type CoordinationJob struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	ProjectID        uuid.UUID
	WorkflowID       string
	TriggerEventID   *uuid.UUID
	JobType          string
	Status           string
	InputSnapshotRef map[string]any
	OutputEventIDs   []any
	StartedAt        *time.Time
	FinishedAt       *time.Time
	CreatedAt        time.Time
}

type RouteDecision struct {
	ID                          uuid.UUID
	TenantID                    uuid.UUID
	ProjectID                   uuid.UUID
	CoordinationJobID           uuid.UUID
	DemandID                    *uuid.UUID
	CandidateDigitalEmployeeIDs []uuid.UUID
	SelectedDigitalEmployeeIDs  []uuid.UUID
	Reason                      string
	InputRequirements           map[string]any
	ExpectedOutputs             []any
	BudgetEstimate              map[string]any
	RequiresHumanReview         bool
	CreatedEventID              *uuid.UUID
	CreatedAt                   time.Time
}

type ExecutionSummary struct {
	ID                    uuid.UUID
	TenantID              uuid.UUID
	ProjectID             uuid.UUID
	ProjectTaskID         uuid.UUID
	DigitalEmployeeID     uuid.UUID
	Conclusion            string
	EvidenceRefs          []any
	ArtifactRefs          []any
	ConfidenceFactors     map[string]any
	Uncertainty           *string
	MissingInformation    []any
	RecommendedNextAction *string
	RequiresHumanReview   bool
	TransferRequestID     *uuid.UUID
	CreatedEventID        *uuid.UUID
	CreatedAt             time.Time
}

type TransferRequest struct {
	ID                           uuid.UUID
	TenantID                     uuid.UUID
	ProjectID                    uuid.UUID
	ProjectTaskID                uuid.UUID
	RequestedByDigitalEmployeeID uuid.UUID
	Reason                       string
	SuggestedEmployeeType        *string
	SuggestedDigitalEmployeeIDs  []uuid.UUID
	MissingContextRefs           []any
	Status                       string
	CreatedEventID               *uuid.UUID
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
}

type DecisionRequest struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	ApprovalRequestID uuid.UUID
	CoordinationJobID *uuid.UUID
	ProjectTaskID     *uuid.UUID
	TargetUserID      uuid.UUID
	DecisionType      string
	TitleSnapshot     string
	SummarySnapshot   *string
	RiskLevelSnapshot *string
	StatusSnapshot    string
	CreatedEventID    *uuid.UUID
	ResolvedEventID   *uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ResolvedAt        *time.Time
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
	SourceRefs        map[string]any
	Attachments       []any
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
	PrincipalType       PrincipalType  `json:"principal_type"`
	PrincipalID         uuid.UUID      `json:"principal_id"`
	ProjectRole         ProjectRole    `json:"project_role"`
	DisplayNameSnapshot string         `json:"display_name_snapshot"`
	Settings            map[string]any `json:"settings"`
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
	Members            *[]ProjectMemberInput
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
	SourceRefs        map[string]any
	Attachments       []any
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
