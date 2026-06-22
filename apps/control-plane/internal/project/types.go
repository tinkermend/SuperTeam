package project

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidProject               = errors.New("invalid project")
	ErrInvalidProjectMember         = errors.New("invalid project member")
	ErrProjectNotFound              = errors.New("project not found")
	ErrProjectConflict              = errors.New("project conflict")
	ErrProjectArchived              = errors.New("project archived")
	ErrProjectTaskForbidden         = errors.New("project task forbidden")
	ErrProjectTaskGraphPending      = errors.New("project task graph pending implementation")
	ErrInvalidProjectEvidence       = errors.New("invalid project evidence")
	ErrInvalidProjectAcceptance     = errors.New("invalid project acceptance")
	ErrProjectArchiveBlocked        = errors.New("project archive blocked")
	ErrUnauthorizedProjectTeamScope = errors.New("unauthorized project team scope")
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

type ReviewerSelectionReason string

const (
	ReviewerSelectionProjectReviewerDefault    ReviewerSelectionReason = "project_reviewer_default"
	ReviewerSelectionProjectHumanOwnerFallback ReviewerSelectionReason = "project_human_owner_fallback"
	ReviewerSelectionUserSelected              ReviewerSelectionReason = "user_selected"
)

type ReviewerPreference struct {
	ReviewerUserID   uuid.UUID
	SelectionReason  ReviewerSelectionReason
	DisplayName      *string
	ProjectRole      ProjectRole
	ResolvedFromRule bool
}

type ProjectEventType string

const (
	ProjectEventCreated         ProjectEventType = "project.created"
	ProjectEventConfigChanged   ProjectEventType = "project.config.changed"
	ProjectEventArchived        ProjectEventType = "project.archived"
	ProjectEventDemandSubmitted ProjectEventType = "demand.submitted"

	ProjectEventWorkflowSignaled               ProjectEventType = "workflow.signaled"
	ProjectEventCoordinationJobCreated         ProjectEventType = "coordination_job.created"
	ProjectEventRouteDecisionCreated           ProjectEventType = "route_decision.created"
	ProjectEventTaskCreated                    ProjectEventType = "project_task.created"
	ProjectEventTaskGraphPlanned               ProjectEventType = "project_task_graph.planned"
	ProjectEventTaskDispatchGateChecked        ProjectEventType = "project_task.dispatch_gate.checked"
	ProjectEventTaskDispatchGateBlocked        ProjectEventType = "project_task.dispatch_gate.blocked"
	ProjectEventTaskDispatchGateWaitingHuman   ProjectEventType = "project_task.dispatch_gate.waiting_human"
	ProjectEventTaskDispatchGateRetryLater     ProjectEventType = "project_task.dispatch_gate.retry_later"
	ProjectEventTaskDispatchGateReplanRequired ProjectEventType = "project_task.dispatch_gate.replan_required"
	ProjectEventTaskDispatched                 ProjectEventType = "project_task.dispatched"
	ProjectEventTaskDispatchFailed             ProjectEventType = "project_task.dispatch_failed"
	ProjectEventTaskContractMissing            ProjectEventType = "project_task.contract_missing"
	ProjectEventTaskWaitingHuman               ProjectEventType = "project_task.waiting_human"
	ProjectEventTaskCancelled                  ProjectEventType = "project_task.cancelled"
	ProjectEventTaskCompleted                  ProjectEventType = "project_task.completed"
	ProjectEventTaskFailed                     ProjectEventType = "project_task.failed"
	ProjectEventTaskResultSubmitted            ProjectEventType = "project_task.result.submitted"
	ProjectEventTaskResultRecorded             ProjectEventType = "project_task.result.recorded"
	ProjectEventTaskResultAccepted             ProjectEventType = "project_task.result.accepted"
	ProjectEventTaskResultRejected             ProjectEventType = "project_task.result.rejected"
	ProjectEventTaskResultBlocked              ProjectEventType = "project_task.result.blocked"
	ProjectEventTaskResultRetryableFailed      ProjectEventType = "project_task.result.retryable_failed"
	ProjectEventTaskResultFailedRetryable      ProjectEventType = ProjectEventTaskResultRetryableFailed
	ProjectEventTaskResultValidationFailed     ProjectEventType = "project_task.result.validation_failed"
	ProjectEventTaskRevisionRequested          ProjectEventType = "project_task.revision.requested"
	ProjectEventTaskRevisionCreated            ProjectEventType = "project_task.revision.created"
	ProjectEventTaskReplanRequested            ProjectEventType = "project_task.replan.requested"
	ProjectEventDemandSummaryCreated           ProjectEventType = "demand.summary.created"
	ProjectEventTransferRequested              ProjectEventType = "transfer.requested"
	ProjectEventDecisionRequested              ProjectEventType = "decision.requested"
	ProjectEventDecisionSubmitted              ProjectEventType = "decision.submitted"
	ProjectEventEvidenceLinked                 ProjectEventType = "project.evidence.linked"
	ProjectEventEvidenceVerified               ProjectEventType = "project.evidence.verified"
	ProjectEventArtifactLinked                 ProjectEventType = "project.artifact.linked"
	ProjectEventReportLinked                   ProjectEventType = "project.report.linked"
	ProjectEventBudgetRecorded                 ProjectEventType = "project.budget.recorded"
	ProjectEventAcceptanceSubmitted            ProjectEventType = "project.acceptance.submitted"
	ProjectEventArchiveSnapshotCreated         ProjectEventType = "project.archive_snapshot.created"
	ProjectEventArchiveRetentionPending        ProjectEventType = "project.archive.retention_pending"
	ProjectEventLendingEmployeeSkipped         ProjectEventType = "project.lending.employee_skipped"
)

type EvidenceVerificationStatus string

const (
	EvidenceVerificationStatusSubmitted  EvidenceVerificationStatus = "submitted"
	EvidenceVerificationStatusLinked     EvidenceVerificationStatus = "linked"
	EvidenceVerificationStatusVerified   EvidenceVerificationStatus = "verified"
	EvidenceVerificationStatusRejected   EvidenceVerificationStatus = "rejected"
	EvidenceVerificationStatusSuperseded EvidenceVerificationStatus = "superseded"
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
	ProjectDemandStatusPlanned         ProjectDemandStatus = "planned"
	ProjectDemandStatusExecuting       ProjectDemandStatus = "executing"
	ProjectDemandStatusCompleted       ProjectDemandStatus = "completed"
	ProjectDemandStatusFailed          ProjectDemandStatus = "failed"
	ProjectDemandStatusCancelled       ProjectDemandStatus = "cancelled"
)

// projectDemandStatusRank orders demand lifecycle states so status can only be
// advanced forward. Terminal states share the highest rank; intake states share 0.
func projectDemandStatusRank(status ProjectDemandStatus) int {
	switch status {
	case ProjectDemandStatusPlanned:
		return 1
	case ProjectDemandStatusExecuting:
		return 2
	case ProjectDemandStatusCompleted, ProjectDemandStatusFailed, ProjectDemandStatusCancelled:
		return 3
	default:
		// submitted / recorded / planning_pending and any unknown intake state
		return 0
	}
}

// ProjectDemandStatusCanAdvance reports whether a demand may move from current to
// next without regressing its lifecycle. Equal ranks are not re-applied.
func ProjectDemandStatusCanAdvance(current, next ProjectDemandStatus) bool {
	return projectDemandStatusRank(next) > projectDemandStatusRank(current)
}

type WorkflowInstanceStatus string

const (
	WorkflowInstanceStatusPlanning     WorkflowInstanceStatus = "planning"
	WorkflowInstanceStatusRunning      WorkflowInstanceStatus = "running"
	WorkflowInstanceStatusWaitingHuman WorkflowInstanceStatus = "waiting_human"
	WorkflowInstanceStatusFailed       WorkflowInstanceStatus = "failed"
	WorkflowInstanceStatusCompleted    WorkflowInstanceStatus = "completed"
	WorkflowInstanceStatusCancelled    WorkflowInstanceStatus = "cancelled"
	WorkflowInstanceStatusUnknown      WorkflowInstanceStatus = "unknown"
)

type ListWorkflowInstancesRequest struct {
	TenantID    uuid.UUID
	ActorUserID uuid.UUID
	Query       string
	ProjectID   *uuid.UUID
	Status      *WorkflowInstanceStatus
	Limit       int32
	Offset      int32
}

type WorkflowInstanceProgress struct {
	TotalNodes        int32
	CompletedNodes    int32
	RunningNodes      int32
	BlockedNodes      int32
	WaitingHumanNodes int32
	PlannedNodes      int32
	FailedNodes       int32
	CancelledNodes    int32
}

type WorkflowInstanceCurrentBlocker struct {
	Type       string
	Title      string
	ResourceID *uuid.UUID
}

type WorkflowInstancePriority struct {
	Value  string
	Label  string
	Source string
}

type WorkflowInstanceRisk struct {
	Level  string
	Label  string
	Source string
}

type WorkflowInstanceSLA struct {
	DueAt            *time.Time
	RemainingSeconds *int32
	Breached         bool
	Label            string
	Source           string
}

type WorkflowInstanceRecentEvent struct {
	EventType  string
	Summary    string
	OccurredAt time.Time
}

type WorkflowInstanceSummary struct {
	DemandID                  uuid.UUID
	ProjectID                 uuid.UUID
	ProjectName               string
	Title                     string
	SubmittedByUserID         uuid.UUID
	SubmittedByDisplayName    string
	Status                    WorkflowInstanceStatus
	StatusReason              string
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
	SelectedCoordinationJobID *uuid.UUID
	Progress                  WorkflowInstanceProgress
	CurrentBlocker            *WorkflowInstanceCurrentBlocker
	Priority                  *WorkflowInstancePriority
	Risk                      *WorkflowInstanceRisk
	SLA                       *WorkflowInstanceSLA
	RecentEvent               *WorkflowInstanceRecentEvent
}

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
	ID                         uuid.UUID
	TenantID                   uuid.UUID
	ProjectID                  uuid.UUID
	DemandID                   *uuid.UUID
	Title                      string
	Summary                    *string
	Status                     string
	AssignedDigitalEmployeeID  *uuid.UUID
	RuntimeTaskID              *uuid.UUID
	DigitalEmployeeRunID       *uuid.UUID
	RiskLevel                  *string
	RequiresHumanApproval      bool
	CoordinationJobID          *uuid.UUID
	RouteDecisionID            *uuid.UUID
	PlannedTaskKey             *string
	TaskKind                   *string
	StageIndex                 *int32
	ExpectedOutputs            []any
	InputRequirements          map[string]any
	HandoffContract            map[string]any
	PlannerMetadata            map[string]any
	BlockedByTaskIDs           []uuid.UUID
	CurrentAttemptID           *uuid.UUID
	LatestDispatchGateResultID *uuid.UUID
	AcceptedPlanRevisionID     *uuid.UUID
	DecompositionClaimKey      *string
	AttemptCount               int32
	MaxAttempts                *int32
	RetryNotBefore             *time.Time
	WaitingReason              *string
	WaitingRequestID           *uuid.UUID
	TerminalReason             *string
	TerminalEventID            *uuid.UUID
	CancelledBy                *string
	FailedBy                   *string
	StatusChangedAt            time.Time
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

type ProjectTaskExecutionPacket struct {
	Version              string
	ProjectID            string
	ProjectTaskID        string
	Title                string
	Summary              string
	ExpectedOutputs      []any
	InputRequirements    map[string]any
	HandoffContract      map[string]any
	DependencyOutputs    []ProjectTaskDependencyOutput
	HumanDecisionRefs    []ProjectTaskHumanDecisionRef
	ForbiddenScopes      []string
	RiskLevel            string
	StopForHumanCriteria []string
}

type ProjectTaskDependencyOutput struct {
	ProjectTaskID string
	Conclusion    string
	EvidenceRefs  []any
	ArtifactRefs  []any
}

type ProjectTaskHumanDecisionRef struct {
	DecisionRequestID string
	DecisionType      string
	StatusSnapshot    string
}

const (
	ContextUpdateDeliveryHotInject       = "hot_inject"
	ContextUpdateDeliveryNextAttempt     = "next_attempt"
	ContextUpdateDeliveryWaitingHuman    = "waiting_human"
	ContextUpdateDeliveryCancelAndReplan = "cancel_and_replan"
)

const (
	ProjectTaskLivenessBlockedByDependency = "blocked_by_dependency"
	ProjectTaskLivenessReadyToDispatch     = "ready_to_dispatch"
	ProjectTaskLivenessQueued              = "queued"
	ProjectTaskLivenessRunning             = "running"
	ProjectTaskLivenessWaitingHuman        = "waiting_human"
	ProjectTaskLivenessRetryScheduled      = "retry_scheduled"
	ProjectTaskLivenessLeaseLost           = "lease_lost"
	ProjectTaskLivenessTimedOut            = "timed_out"
	ProjectTaskLivenessTerminal            = "terminal"
)

const (
	ProjectTaskStatusPlanned      = "planned"
	ProjectTaskStatusQueued       = "queued"
	ProjectTaskStatusRunning      = "running"
	ProjectTaskStatusWaitingHuman = "waiting_human"
	ProjectTaskStatusCompleted    = "completed"
	ProjectTaskStatusFailed       = "failed"
	ProjectTaskStatusCancelled    = "cancelled"
)

const (
	ProjectTaskAttemptStatusQueued       = "queued"
	ProjectTaskAttemptStatusRunning      = "running"
	ProjectTaskAttemptStatusSucceeded    = "succeeded"
	ProjectTaskAttemptStatusFailed       = "failed"
	ProjectTaskAttemptStatusCancelled    = "cancelled"
	ProjectTaskAttemptStatusLost         = "lost"
	ProjectTaskAttemptStatusTimedOut     = "timed_out"
	ProjectTaskAttemptStatusWaitingHuman = "waiting_human"
)

const (
	FailureFamilyTransientRuntime      = "transient_runtime"
	FailureFamilyTransientProvider     = "transient_provider"
	FailureFamilyTimeout               = "timeout"
	FailureFamilyInvalidContract       = "invalid_contract"
	FailureFamilyApprovalRequired      = "approval_required"
	FailureFamilyPermissionRequired    = "permission_required"
	FailureFamilyNonRetryableExecution = "non_retryable_execution"
	FailureFamilyBusinessCancelled     = "business_cancelled"
	FailureFamilyPlanInvalid           = "plan_invalid"
	FailureFamilyRequirementChanged    = "requirement_changed"
	FailureFamilyAcceptanceRequired    = "acceptance_required"
)

const (
	HumanWaitReasonMissingContext     = "missing_context"
	HumanWaitReasonClarification      = "clarification"
	HumanWaitReasonApprovalRequired   = "approval_required"
	HumanWaitReasonPermissionRequired = "permission_required"
	HumanWaitReasonPlanInvalid        = "plan_invalid"
	HumanWaitReasonAcceptanceRequired = "acceptance_required"
	HumanWaitReasonRuntimeRecovery    = "runtime_recovery"
	HumanWaitReasonBudgetApproval     = "budget_approval"
)

const (
	HumanWaitResolutionApprove           = "approve"
	HumanWaitResolutionResumeSameTask    = "resume_same_task"
	HumanWaitResolutionCancelAndReplan   = "cancel_and_replan"
	HumanWaitResolutionCancelWithoutPlan = "cancel_without_replan"
	HumanWaitResolutionMarkFailed        = "mark_failed"
)

type ProjectTaskAttempt struct {
	ID                            uuid.UUID
	TenantID                      uuid.UUID
	ProjectTaskID                 uuid.UUID
	AttemptNo                     int32
	Status                        string
	DigitalEmployeeRunID          *uuid.UUID
	RuntimeTaskID                 *uuid.UUID
	RuntimeNodeID                 *uuid.UUID
	ProviderSessionID             *string
	ExecutionContextPacket        map[string]any
	ExecutionContextPacketVersion string
	LeaseToken                    string
	LeaseExpiresAt                *time.Time
	RenewedAt                     *time.Time
	LostAt                        *time.Time
	StartedAt                     *time.Time
	FinishedAt                    *time.Time
	TimeoutAt                     *time.Time
	Retryable                     *bool
	FailureFamily                 *string
	FailureMessage                *string
	IdempotencyKey                string
	DispatchGateResultID          *uuid.UUID
	CreatedEventID                *uuid.UUID
	TerminalEventID               *uuid.UUID
	CreatedAt                     time.Time
	UpdatedAt                     time.Time
}

type QueueProjectTaskRequest struct {
	TenantID                      uuid.UUID
	ProjectID                     uuid.UUID
	ProjectTaskID                 uuid.UUID
	ProjectTaskAttemptID          *uuid.UUID
	DigitalEmployeeID             uuid.UUID
	DigitalEmployeeRunID          *uuid.UUID
	RuntimeTaskID                 *uuid.UUID
	RuntimeNodeID                 *uuid.UUID
	IdempotencyKey                string
	LeaseToken                    string
	LeaseExpiresAt                *time.Time
	ExecutionContextPacket        map[string]any
	ExecutionContextPacketVersion string
	DispatchGateResultID          *uuid.UUID
}

type QueueProjectTaskResult struct {
	Task    ProjectTask
	Attempt ProjectTaskAttempt
	Event   ProjectEvent
}

type HumanActionRequest map[string]any

type PreDispatchGateResult struct {
	ID                     uuid.UUID
	TenantID               uuid.UUID
	ProjectID              uuid.UUID
	ProjectTaskID          uuid.UUID
	AcceptedPlanRevisionID *uuid.UUID
	PlannedTaskKey         *string
	SelectedEmployeeID     uuid.UUID
	AttemptNo              int32
	DispatchReason         string
	IdempotencyKey         string
	DispatchToken          string
	Status                 string
	CheckedAt              time.Time
	Checks                 []PreDispatchGateCheck
	Blockers               []PreDispatchGateBlocker
	HumanActionRequest     HumanActionRequest
	RetryAfter             *time.Time
	AttemptID              *uuid.UUID
	DecisionRequestID      *uuid.UUID
	CreatedEventID         *uuid.UUID
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type RecordPreDispatchGateResultRequest struct {
	TenantID               uuid.UUID
	ProjectID              uuid.UUID
	ProjectTaskID          uuid.UUID
	AcceptedPlanRevisionID *uuid.UUID
	PlannedTaskKey         *string
	SelectedEmployeeID     uuid.UUID
	AttemptNo              int32
	DispatchReason         string
	IdempotencyKey         string
	DispatchToken          string
	Status                 string
	CheckedAt              time.Time
	Checks                 []PreDispatchGateCheck
	Blockers               []PreDispatchGateBlocker
	HumanActionRequest     HumanActionRequest
	RetryAfter             *time.Time
	CreatedEventID         *uuid.UUID
}

type ListPreDispatchGateResultsRequest struct {
	TenantID      uuid.UUID
	ProjectID     uuid.UUID
	ProjectTaskID uuid.UUID
	Limit         int32
	Offset        int32
}

type LinkPreDispatchGateAttemptRequest struct {
	TenantID      uuid.UUID
	ProjectID     uuid.UUID
	ProjectTaskID uuid.UUID
	GateResultID  uuid.UUID
	AttemptID     uuid.UUID
}

type LinkPreDispatchGateDecisionRequest struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	ProjectTaskID     uuid.UUID
	GateResultID      uuid.UUID
	DecisionRequestID uuid.UUID
}

type MoveProjectTaskToWaitingHumanForPreDispatchGateRequest struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	ProjectTaskID     uuid.UUID
	GateResultID      uuid.UUID
	DecisionRequestID uuid.UUID
	EventID           *uuid.UUID
	WaitingReason     string
}

type ProjectTaskAttemptContextUpdate struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	ProjectTaskID  uuid.UUID
	AttemptID      *uuid.UUID
	UpdateKind     string
	Payload        map[string]any
	DeliveryMode   string
	CreatedEventID *uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type RecordAttemptContextUpdateRequest struct {
	TenantID      uuid.UUID
	ProjectID     uuid.UUID
	ProjectTaskID uuid.UUID
	AttemptID     *uuid.UUID
	UpdateKind    string
	Payload       map[string]any
}

type RecordProjectTaskAttemptContextUpdateRepositoryRequest struct {
	TenantID       uuid.UUID
	ProjectTaskID  uuid.UUID
	AttemptID      *uuid.UUID
	UpdateKind     string
	Payload        map[string]any
	DeliveryMode   string
	CreatedEventID *uuid.UUID
}

type ProjectTaskLiveness struct {
	ProjectTaskID         uuid.UUID
	Liveness              string
	Reason                string
	BlockingDependencyIDs []uuid.UUID
	CurrentAttemptID      *uuid.UUID
	AttemptStatus         string
	WaitingRequestID      *uuid.UUID
	RetryNotBefore        *time.Time
	LeaseExpiresAt        *time.Time
	NextAction            string
}

type DecomposeAcceptedPlanRevisionRequest struct {
	TenantID               uuid.UUID
	ProjectID              uuid.UUID
	DemandID               uuid.UUID
	CoordinationJobID      uuid.UUID
	RouteDecisionID        uuid.UUID
	AcceptedPlanRevisionID uuid.UUID
	PlanFingerprint        string
	DecompositionClaimKey  string
	Tasks                  []ProjectTaskGraphCreateTask
}

type DecomposeAcceptedPlanRevisionResult struct {
	Tasks        []ProjectTask
	Dependencies []ProjectTaskDependency
	Replayed     bool
}

type BindProjectTaskRunRequest struct {
	TenantID             uuid.UUID
	ProjectTaskID        uuid.UUID
	DigitalEmployeeRunID uuid.UUID
	RuntimeTaskID        uuid.UUID
	LatestEventID        *uuid.UUID
	CurrentStatuses      []string
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
	ID                   uuid.UUID
	TenantID             uuid.UUID
	ProjectID            uuid.UUID
	ApprovalRequestID    uuid.UUID
	CoordinationJobID    *uuid.UUID
	ProjectTaskID        *uuid.UUID
	TargetUserID         uuid.UUID
	DecisionType         string
	TitleSnapshot        string
	SummarySnapshot      *string
	RiskLevelSnapshot    *string
	StatusSnapshot       string
	CreatedEventID       *uuid.UUID
	ResolvedEventID      *uuid.UUID
	CreatedAt            time.Time
	UpdatedAt            time.Time
	ResolvedAt           *time.Time
	DispatchGateResultID *uuid.UUID
}

type DecisionInboxProjector interface {
	UpsertProjectDecisionRequest(ctx context.Context, decision DecisionRequest) error
	ResolveProjectDecisionRequest(ctx context.Context, decision DecisionRequest) error
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
	ID                 uuid.UUID
	TenantID           uuid.UUID
	ProjectID          uuid.UUID
	SubmittedByUserID  uuid.UUID
	Title              string
	Content            *string
	SourceType         DemandSourceType
	SourceRefs         map[string]any
	Attachments        []any
	ReviewerPreference *ReviewerPreference
	Status             ProjectDemandStatus
	CreatedEventID     *uuid.UUID
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type DemandLaunchDetail struct {
	Demand           ProjectDemand
	Project          Project
	Reviewer         *ReviewerPreference
	CoordinationJobs []CoordinationJob
	RouteDecisions   []RouteDecision
	ProjectTasks     []ProjectTask
	DecisionRequests []DecisionRequest
	RecentEvents     []ProjectEvent
}

type ProjectConfigRevision struct {
	ID                 uuid.UUID
	TenantID           uuid.UUID
	ProjectID          uuid.UUID
	RevisionNumber     int32
	ConfigSnapshot     map[string]any
	ChangeSummary      *string
	CreatedByUserID    uuid.UUID
	CreatedEventID     *uuid.UUID
	CreatedAt          time.Time
	ChangedSections    []any
	PreviousRevisionID *uuid.UUID
	PolicyFingerprint  *string
	DiffSummary        map[string]any
}

type ProjectEvidenceRef struct {
	ID                 uuid.UUID
	TenantID           uuid.UUID
	ProjectID          uuid.UUID
	ProjectTaskID      *uuid.UUID
	RouteDecisionID    *uuid.UUID
	ExecutionSummaryID *uuid.UUID
	EvidenceType       string
	Title              string
	Summary            *string
	SourceType         string
	SourceRef          string
	ArtifactRefID      *uuid.UUID
	SubmittedByType    string
	SubmittedByID      *uuid.UUID
	VerificationStatus EvidenceVerificationStatus
	Metadata           map[string]any
	CreatedEventID     *uuid.UUID
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type ProjectArtifactRef struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	ProjectTaskID   *uuid.UUID
	ArtifactID      *uuid.UUID
	ArtifactType    string
	Title           string
	ObjectRef       string
	ContentType     *string
	SizeBytes       *int64
	Checksum        *string
	RetentionStatus string
	RetentionHoldID *uuid.UUID
	Metadata        map[string]any
	CreatedEventID  *uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ProjectReportRef struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	ReportType      string
	Title           string
	Summary         *string
	ObjectRef       string
	Format          string
	GeneratedByType string
	GeneratedByID   *uuid.UUID
	CreatedEventID  *uuid.UUID
	CreatedAt       time.Time
}

type ProjectBudgetLedgerEntry struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	CoordinationJobID *uuid.UUID
	ProjectTaskID     *uuid.UUID
	DigitalEmployeeID *uuid.UUID
	CostType          string
	EstimatedTokens   *int64
	ActualTokens      *int64
	EstimatedCost     string
	ActualCost        string
	Source            string
	Reason            *string
	CreatedEventID    *uuid.UUID
	CreatedAt         time.Time
}

type ProjectBudgetSummary struct {
	EstimatedTokens int64
	ActualTokens    int64
	EstimatedCost   string
	ActualCost      string
	LedgerCount     int32
}

type ProjectAcceptanceRecord struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	ProjectID        uuid.UUID
	AcceptedByUserID uuid.UUID
	Status           string
	Conclusion       string
	Summary          *string
	EvidenceRefIDs   []uuid.UUID
	ReportRefIDs     []uuid.UUID
	UnresolvedRisks  []any
	CreatedEventID   *uuid.UUID
	CreatedAt        time.Time
}

type ProjectArchivePreview struct {
	ProjectID           uuid.UUID
	EvidenceCount       int64
	ArtifactCount       int64
	ReportCount         int64
	RetentionPending    bool
	BlockedReasons      []any
	EstimatedObjectRefs []any
}

type ProjectArchiveSnapshot struct {
	ID                   uuid.UUID
	TenantID             uuid.UUID
	ProjectID            uuid.UUID
	SnapshotType         string
	Status               string
	ObjectRef            *string
	Summary              *string
	IncludedCounts       map[string]any
	RetainedArtifactIDs  []uuid.UUID
	RetentionLockEventID *uuid.UUID
	CreatedByUserID      uuid.UUID
	CreatedEventID       *uuid.UUID
	CreatedAt            time.Time
}

type ArchiveArtifactLocker interface {
	LockProjectArtifacts(ctx context.Context, tenantID, projectID uuid.UUID, artifactIDs []uuid.UUID) (ArchiveArtifactLockResult, error)
}

type ArchiveArtifactLockResult struct {
	HoldIDs     []uuid.UUID
	ArtifactIDs []uuid.UUID
	EventID     *uuid.UUID
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
	TenantID                uuid.UUID
	ProjectID               uuid.UUID
	SubmittedByUserID       uuid.UUID
	Title                   string
	Content                 string
	SourceType              DemandSourceType
	SourceRefs              map[string]any
	Attachments             []any
	ReviewerUserID          *uuid.UUID
	ReviewerSelectionReason ReviewerSelectionReason
}

type CreateEvidenceRefServiceRequest struct {
	TenantID           uuid.UUID
	ProjectID          uuid.UUID
	ActorType          string
	ActorID            uuid.UUID
	ProjectTaskID      *uuid.UUID
	RouteDecisionID    *uuid.UUID
	ExecutionSummaryID *uuid.UUID
	EvidenceType       string
	Title              string
	Summary            string
	SourceType         string
	SourceRef          string
	ArtifactRefID      *uuid.UUID
	SubmittedByType    string
	SubmittedByID      *uuid.UUID
	Metadata           map[string]any
}

type CreateAcceptanceServiceRequest struct {
	TenantID         uuid.UUID
	ProjectID        uuid.UUID
	AcceptedByUserID uuid.UUID
	Status           string
	Conclusion       string
	Summary          string
	EvidenceRefIDs   []uuid.UUID
	ReportRefIDs     []uuid.UUID
	UnresolvedRisks  []any
}

type CreateArchiveSnapshotServiceRequest struct {
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	CreatedByUserID uuid.UUID
	SnapshotType    string
	Summary         string
	ObjectRef       string
}

type ResolveApprovalRequest struct {
	TenantID          uuid.UUID
	ApprovalRequestID uuid.UUID
	DecidedByUserID   uuid.UUID
	Decision          string
	Comment           string
	Payload           map[string]any
}

type ResolveDecisionRequest struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DecisionRequestID uuid.UUID
	DecidedByUserID   uuid.UUID
	Decision          string
	Comment           string
	Payload           map[string]any
}

type CompleteProjectTaskRequest struct {
	TenantID              uuid.UUID
	RuntimeNodeID         uuid.UUID
	ProjectTaskID         uuid.UUID
	DigitalEmployeeID     uuid.UUID
	Conclusion            string
	EvidenceRefs          []any
	ArtifactRefs          []any
	ConfidenceFactors     map[string]any
	Uncertainty           string
	MissingInformation    []any
	RecommendedNextAction string
	RequiresHumanReview   bool
}

type ProjectTaskAttemptRuntimeRequest struct {
	TenantID          uuid.UUID
	AttemptID         uuid.UUID
	ProjectTaskID     uuid.UUID
	RuntimeNodeID     uuid.UUID
	LeaseToken        string
	IdempotencyKey    string
	ProviderSessionID *string
}

type StartProjectTaskAttemptRequest struct {
	ProjectTaskAttemptRuntimeRequest
}

type RenewProjectTaskAttemptLeaseRequest struct {
	ProjectTaskAttemptRuntimeRequest
	LeaseExpiresAt *time.Time
}

type CompleteProjectTaskAttemptRequest struct {
	ProjectTaskAttemptRuntimeRequest
	DigitalEmployeeID     uuid.UUID
	Conclusion            string
	EvidenceRefs          []any
	ArtifactRefs          []any
	ConfidenceFactors     map[string]any
	Uncertainty           string
	MissingInformation    []any
	RecommendedNextAction string
	RequiresHumanReview   bool
}

type FailProjectTaskRequest struct {
	TenantID          uuid.UUID
	RuntimeNodeID     uuid.UUID
	ProjectTaskID     uuid.UUID
	DigitalEmployeeID uuid.UUID
	FailureSummary    string
}

type FailProjectTaskAttemptRequest struct {
	ProjectTaskAttemptRuntimeRequest
	DigitalEmployeeID uuid.UUID
	FailureSummary    string
	FailureFamily     string
	Retryable         *bool
}

type WaitHumanProjectTaskAttemptRequest struct {
	ProjectTaskAttemptRuntimeRequest
	DigitalEmployeeID          uuid.UUID
	Reason                     string
	Summary                    string
	MissingContextRefs         []any
	SuggestedResolutionOptions []string
}

type ResolveProjectTaskHumanWaitRequest struct {
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	ProjectTaskID   uuid.UUID
	ActorUserID     uuid.UUID
	Resolution      string
	ResponseSummary string
	ContextRefs     []any
}

type RequestProjectTaskTransferRequest struct {
	TenantID                    uuid.UUID
	RuntimeNodeID               uuid.UUID
	ProjectTaskID               uuid.UUID
	DigitalEmployeeID           uuid.UUID
	Reason                      string
	SuggestedEmployeeType       string
	SuggestedDigitalEmployeeIDs []uuid.UUID
	MissingContextRefs          []any
}

type RetryWorkflowSignalRequest struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
	EventID   uuid.UUID
	ActorID   uuid.UUID
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
