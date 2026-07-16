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
	ErrProjectRuntimeNodesRequired  = errors.New("project requires at least one runtime node")
	// ErrProjectTaskNoEligibleOnlineNode means the project's runtime node
	// eligibility set has no node that is currently online with capacity (and,
	// when checked, supporting the required provider). Retryable: the set may
	// gain a usable node (a node comes online, frees a slot, or an operator
	// widens the eligibility set).
	ErrProjectTaskNoEligibleOnlineNode = errors.New("project task has no eligible online runtime node")
	// ErrProjectTaskPinnedNodeOffline means the task's current attempt is
	// already bound to a runtime node (task hard-pin) and that node is
	// currently offline or out of capacity. Per the anti-drift rule the task
	// must wait for that specific node to recover rather than being
	// re-selected onto a different one.
	ErrProjectTaskPinnedNodeOffline = errors.New("project task's pinned runtime node is offline")
	ErrProjectDeleteBlocked         = errors.New("project delete blocked")
	// ErrInvalidCoordinationMode means a demand's coordination_mode is neither
	// empty nor one of the known modes (plan, loop).
	ErrInvalidCoordinationMode = errors.New("invalid coordination_mode")
)

const ProjectDeleteBlockedCode = "project_delete_blocked"

// Demand coordination modes. "plan" is the default single-pass planning
// flow; "loop" opts a demand into the tri-mode handoff loop.
const (
	CoordinationModePlan = "plan"
	CoordinationModeLoop = "loop"
)

type ProjectDeleteBlocker struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Status string `json:"status"`
	Title  string `json:"title"`
}

type ProjectDeleteBlockedError struct {
	Blockers []ProjectDeleteBlocker
}

func (e *ProjectDeleteBlockedError) Error() string {
	return ErrProjectDeleteBlocked.Error()
}

func (e *ProjectDeleteBlockedError) Unwrap() error {
	return ErrProjectDeleteBlocked
}

type ProjectDeletePreview struct {
	ProjectID   uuid.UUID
	ProjectName string
	CanDelete   bool
	Blockers    []ProjectDeleteBlocker
	Warnings    ProjectDeleteWarnings
	Message     string
}

type ProjectDeleteWarnings struct {
	PendingDecisionCount       int32 `json:"pending_decision_count"`
	WaitingHumanTaskCount      int32 `json:"waiting_human_task_count"`
	OpenInboxCount             int32 `json:"open_inbox_count"`
	ActiveMemberCount          int32 `json:"active_member_count"`
	DigitalEmployeeMemberCount int32 `json:"digital_employee_member_count"`
	RuntimeNodeBindingCount    int32 `json:"runtime_node_binding_count"`
	AffinityCount              int32 `json:"affinity_count"`
}

type ProjectDeleteCascadeResult struct {
	MemberCount      int
	TaskCount        int
	DecisionCount    int
	ApprovalCount    int
	InboxCount       int
	RuntimeNodeCount int
	AffinityCount    int
	PlacementCount   int
}

type DeleteProjectRequest struct {
	TenantID    uuid.UUID
	ProjectID   uuid.UUID
	ActorUserID uuid.UUID
}

type SoftDeleteProjectCascadeParams struct {
	TenantID    uuid.UUID
	ProjectID   uuid.UUID
	DeletedAt   time.Time
	ActorUserID uuid.UUID
	Project     Project
}

type ProjectDeleteAuditEventParams struct {
	TenantID      uuid.UUID
	ActorUserID   uuid.UUID
	Project       Project
	CascadeResult ProjectDeleteCascadeResult
	DeletedAt     time.Time
}

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

	ProjectEventRuntimePlacementUpdated          ProjectEventType = "project.runtime_placement.updated"
	ProjectEventRuntimePlacementReleased         ProjectEventType = "project.runtime_placement.released"
	ProjectEventCoordinationBlocked              ProjectEventType = "coordination.blocked"
	ProjectEventScenarioTemplateResolutionFailed ProjectEventType = "scenario_template.resolution_failed"
	ProjectEventWorkflowCoordinationFailed       ProjectEventType = "workflow.coordination_failed"
	ProjectEventTaskDispatchBlocked              ProjectEventType = "project_task.dispatch_blocked"

	ProjectEventWorkflowSignaled                ProjectEventType = "workflow.signaled"
	ProjectEventCoordinationJobCreated          ProjectEventType = "coordination_job.created"
	ProjectEventRouteDecisionCreated            ProjectEventType = "route_decision.created"
	ProjectEventTaskCreated                     ProjectEventType = "project_task.created"
	ProjectEventTaskGraphPlanned                ProjectEventType = "project_task_graph.planned"
	ProjectEventTaskDispatchGateChecked         ProjectEventType = "project_task.dispatch_gate.checked"
	ProjectEventTaskDispatchGateBlocked         ProjectEventType = "project_task.dispatch_gate.blocked"
	ProjectEventTaskDispatchGateWaitingHuman    ProjectEventType = "project_task.dispatch_gate.waiting_human"
	ProjectEventTaskDispatchGateRetryLater      ProjectEventType = "project_task.dispatch_gate.retry_later"
	ProjectEventTaskDispatchGateReplanRequired  ProjectEventType = "project_task.dispatch_gate.replan_required"
	ProjectEventTaskDispatched                  ProjectEventType = "project_task.dispatched"
	ProjectEventTaskDispatchFailed              ProjectEventType = "project_task.dispatch_failed"
	ProjectEventTaskRetryScheduled              ProjectEventType = "project_task.retry_scheduled"
	ProjectEventTaskAttemptLost                 ProjectEventType = "project_task.attempt_lost"
	ProjectEventTaskRecoveryRequested           ProjectEventType = "project_task.recovery_requested"
	ProjectEventTaskContractMissing             ProjectEventType = "project_task.contract_missing"
	ProjectEventTaskWaitingHuman                ProjectEventType = "project_task.waiting_human"
	ProjectEventTaskCancelled                   ProjectEventType = "project_task.cancelled"
	ProjectEventTaskCompleted                   ProjectEventType = "project_task.completed"
	ProjectEventTaskFailed                      ProjectEventType = "project_task.failed"
	ProjectEventTaskResultSubmitted             ProjectEventType = "project_task.result.submitted"
	ProjectEventTaskResultRecorded              ProjectEventType = "project_task.result.recorded"
	ProjectEventTaskResultAccepted              ProjectEventType = "project_task.result.accepted"
	ProjectEventTaskResultRejected              ProjectEventType = "project_task.result.rejected"
	ProjectEventTaskResultBlocked               ProjectEventType = "project_task.result.blocked"
	ProjectEventTaskResultRetryableFailed       ProjectEventType = "project_task.result.retryable_failed"
	ProjectEventTaskResultFailedRetryable       ProjectEventType = ProjectEventTaskResultRetryableFailed
	ProjectEventTaskResultValidationFailed      ProjectEventType = "project_task.result.validation_failed"
	ProjectEventTaskRevisionRequested           ProjectEventType = "project_task.revision.requested"
	ProjectEventTaskRevisionCreated             ProjectEventType = "project_task.revision.created"
	ProjectEventTaskReplanRequested             ProjectEventType = "project_task.replan.requested"
	ProjectEventDemandSummaryCreated            ProjectEventType = "demand.summary.created"
	ProjectEventTransferRequested               ProjectEventType = "transfer.requested"
	ProjectEventDecisionRequested               ProjectEventType = "decision.requested"
	ProjectEventDecisionSubmitted               ProjectEventType = "decision.submitted"
	ProjectEventEvidenceLinked                  ProjectEventType = "project.evidence.linked"
	ProjectEventEvidenceVerified                ProjectEventType = "project.evidence.verified"
	ProjectEventArtifactLinked                  ProjectEventType = "project.artifact.linked"
	ProjectEventReportLinked                    ProjectEventType = "project.report.linked"
	ProjectEventBudgetRecorded                  ProjectEventType = "project.budget.recorded"
	ProjectEventAcceptanceSubmitted             ProjectEventType = "project.acceptance.submitted"
	ProjectEventArchiveSnapshotCreated          ProjectEventType = "project.archive_snapshot.created"
	ProjectEventArchiveRetentionPending         ProjectEventType = "project.archive.retention_pending"
	ProjectEventLendingEmployeeSkipped          ProjectEventType = "project.lending.employee_skipped"
	ProjectEventTaskUpstreamSupplementRejected  ProjectEventType = "project_task.upstream_supplement_rejected"
	ProjectEventTaskUpstreamSupplementExhausted ProjectEventType = "project_task.upstream_supplement_exhausted"
	ProjectEventDemandReplanningReopened        ProjectEventType = "demand.replanning_reopened"
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
	// ProjectDemandStatusAcceptancePending is the convergence gate's hold state:
	// every project task under the demand has reached a terminal state and none
	// failed, but at least one blocking acceptance criterion for the demand's
	// current plan revision still lacks a satisfied verdict (see
	// PgRepository.recomputeProjectDemandStatusWithQueries and
	// CountUnsatisfiedBlockingCriteria). A human must sign off the remaining
	// criteria (demand_acceptance decision) before the demand can advance to
	// completed. Ranked below the terminal group so AreAllProjectDemandsTerminal
	// / project-level acceptance correctly wait for it.
	ProjectDemandStatusAcceptancePending ProjectDemandStatus = "acceptance_pending"
	ProjectDemandStatusCompleted         ProjectDemandStatus = "completed"
	ProjectDemandStatusFailed            ProjectDemandStatus = "failed"
	ProjectDemandStatusCancelled         ProjectDemandStatus = "cancelled"
)

// projectDemandStatusRank orders demand lifecycle states so status can only be
// advanced forward. Terminal states share the highest rank; intake states share 0.
// acceptance_pending sits between executing and the terminal group: reachable
// from executing, and itself able to advance to completed or failed (a human
// rejecting acceptance fails the demand), but never back to executing.
func projectDemandStatusRank(status ProjectDemandStatus) int {
	switch status {
	case ProjectDemandStatusPlanned:
		return 1
	case ProjectDemandStatusExecuting:
		return 2
	case ProjectDemandStatusAcceptancePending:
		return 3
	case ProjectDemandStatusCompleted, ProjectDemandStatusFailed, ProjectDemandStatusCancelled:
		return 4
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
	RepoBinding            ProjectRepoBinding
	ScenarioTemplateKey    *string
	ArchivedAt             *time.Time
	DeletedAt              *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type ProjectRepoBindingStatus string

const (
	ProjectRepoBindingStatusUnbound ProjectRepoBindingStatus = "unbound"
	ProjectRepoBindingStatusBound   ProjectRepoBindingStatus = "bound"
)

type ProjectRepoBinding struct {
	Status           ProjectRepoBindingStatus
	URL              string
	DefaultBranch    string
	GitCredentialRef *string
	Scope            []string
}

type ProjectRepoBindingInput struct {
	URL              string   `json:"url"`
	DefaultBranch    string   `json:"default_branch"`
	GitCredentialRef *string  `json:"git_credential_ref"`
	Scope            []string `json:"scope"`
}

type ProjectRuntimePlacementStatus string

const (
	ProjectRuntimePlacementStatusMissing                    ProjectRuntimePlacementStatus = "missing"
	ProjectRuntimePlacementStatusReady                      ProjectRuntimePlacementStatus = "ready"
	ProjectRuntimePlacementStatusRuntimeOffline             ProjectRuntimePlacementStatus = "runtime_offline"
	ProjectRuntimePlacementStatusCommandChannelDisconnected ProjectRuntimePlacementStatus = "command_channel_disconnected"
	ProjectRuntimePlacementStatusProviderUnavailable        ProjectRuntimePlacementStatus = "provider_unavailable"
	ProjectRuntimePlacementStatusCapacityFull               ProjectRuntimePlacementStatus = "capacity_full"
	ProjectRuntimePlacementStatusWorkspacePending           ProjectRuntimePlacementStatus = "workspace_pending"
	ProjectRuntimePlacementStatusContractMismatch           ProjectRuntimePlacementStatus = "contract_mismatch"
)

type ProjectRuntimePlacementState string

const (
	ProjectRuntimePlacementStateActive   ProjectRuntimePlacementState = "active"
	ProjectRuntimePlacementStateReleased ProjectRuntimePlacementState = "released"
	ProjectRuntimePlacementStateLost     ProjectRuntimePlacementState = "lost"
)

type ProjectRuntimePlacement struct {
	ID              uuid.UUID                    `json:"id"`
	TenantID        uuid.UUID                    `json:"tenant_id,omitempty"`
	ProjectID       uuid.UUID                    `json:"project_id"`
	RuntimeNodeID   uuid.UUID                    `json:"runtime_node_id"`
	PlacementStatus ProjectRuntimePlacementState `json:"placement_status"`
	PlacementReason string                       `json:"placement_reason,omitempty"`
	AssignedAt      time.Time                    `json:"assigned_at,omitempty"`
	ReleasedAt      *time.Time                   `json:"released_at,omitempty"`
	CreatedAt       time.Time                    `json:"created_at,omitempty"`
	UpdatedAt       time.Time                    `json:"updated_at,omitempty"`
}

type PutProjectRuntimePlacementRequest struct {
	TenantID              uuid.UUID
	ProjectID             uuid.UUID
	RuntimeNodeID         uuid.UUID
	ActorUserID           uuid.UUID
	Reason                string
	ExpectedProviderTypes []string
}

type GetProjectRuntimePlacementRequest struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
}

type ReleaseProjectRuntimePlacementRequest struct {
	TenantID    uuid.UUID
	ProjectID   uuid.UUID
	ActorUserID uuid.UUID
	Reason      string
}

type ProjectRuntimePlacementReadiness struct {
	PlacementStatus         ProjectRuntimePlacementStatus `json:"placement_status"`
	RuntimeNodeID           *uuid.UUID                    `json:"runtime_node_id,omitempty"`
	RuntimeNodeName         string                        `json:"runtime_node_name,omitempty"`
	CommandChannelConnected bool                          `json:"command_channel_connected"`
	ProviderCapabilities    []string                      `json:"provider_capabilities,omitempty"`
	RequiredProviderTypes   []string                      `json:"required_provider_types,omitempty"`
	EmployeeReadiness       []ProjectEmployeeReadiness    `json:"employee_readiness,omitempty"`
	BlockingReasons         []ProjectReadinessReason      `json:"blocking_reasons,omitempty"`
	NextActions             []ProjectReadinessAction      `json:"next_actions,omitempty"`
}

type ProjectEmployeeReadiness struct {
	DigitalEmployeeID uuid.UUID `json:"digital_employee_id"`
	DisplayName       string    `json:"display_name,omitempty"`
	ProviderType      string    `json:"provider_type,omitempty"`
	CanPlan           bool      `json:"can_plan"`
	CanDispatch       bool      `json:"can_dispatch"`
	ReasonCode        string    `json:"reason_code,omitempty"`
	ReasonMessage     string    `json:"reason_message,omitempty"`
}

type ProjectReadinessReason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ProjectReadinessAction struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

type ProjectTaskAttestationStatus string

const (
	ProjectTaskAttestationStatusSucceeded ProjectTaskAttestationStatus = "succeeded"
	ProjectTaskAttestationStatusFailed    ProjectTaskAttestationStatus = "failed"
	ProjectTaskAttestationStatusCancelled ProjectTaskAttestationStatus = "cancelled"
	ProjectTaskAttestationStatusTimedOut  ProjectTaskAttestationStatus = "timed_out"
)

type ProjectTaskAttestationProviderAuthMode string

const (
	ProjectTaskAttestationProviderAuthModeHost               ProjectTaskAttestationProviderAuthMode = "host"
	ProjectTaskAttestationProviderAuthModeEmployee           ProjectTaskAttestationProviderAuthMode = "employee"
	ProjectTaskAttestationProviderAuthModeExplicitCredential ProjectTaskAttestationProviderAuthMode = "explicit_credential"
)

type ProjectTaskAttestation struct {
	ID                        uuid.UUID
	TenantID                  uuid.UUID
	ProjectID                 uuid.UUID
	ProjectTaskID             uuid.UUID
	AttemptID                 uuid.UUID
	RuntimeNodeID             uuid.UUID
	DigitalEmployeeID         uuid.UUID
	CapabilityManifestVersion string
	ProviderAuthMode          ProjectTaskAttestationProviderAuthMode
	ProviderSessionID         *string
	AttestationType           string
	Status                    ProjectTaskAttestationStatus
	CommandArgv               []any
	ExitCode                  *int32
	DurationMs                *int64
	LogRef                    *string
	StdoutSha256              *string
	StderrSha256              *string
	ArtifactRefs              []any
	ArtifactHashes            map[string]any
	GitBranch                 *string
	GitBaseRef                *string
	GitHeadSha                *string
	GitDiffSha256             *string
	Metadata                  map[string]any
	IdempotencyKey            string
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

type CreateProjectTaskAttestationRequest struct {
	TenantID                  uuid.UUID                              `json:"tenant_id,omitempty"`
	ProjectID                 uuid.UUID                              `json:"project_id"`
	ProjectTaskID             uuid.UUID                              `json:"project_task_id"`
	AttemptID                 uuid.UUID                              `json:"attempt_id"`
	RuntimeNodeID             uuid.UUID                              `json:"runtime_node_id"`
	DigitalEmployeeID         uuid.UUID                              `json:"digital_employee_id"`
	CapabilityManifestVersion string                                 `json:"capability_manifest_version"`
	ProviderAuthMode          ProjectTaskAttestationProviderAuthMode `json:"provider_auth_mode"`
	ProviderSessionID         *string                                `json:"provider_session_id"`
	AttestationType           string                                 `json:"attestation_type"`
	Status                    ProjectTaskAttestationStatus           `json:"status"`
	CommandArgv               []any                                  `json:"command_argv"`
	ExitCode                  *int32                                 `json:"exit_code"`
	DurationMs                *int64                                 `json:"duration_ms"`
	LogRef                    *string                                `json:"log_ref"`
	StdoutSha256              *string                                `json:"stdout_sha256"`
	StderrSha256              *string                                `json:"stderr_sha256"`
	ArtifactRefs              []any                                  `json:"artifact_refs"`
	ArtifactHashes            map[string]any                         `json:"artifact_hashes"`
	GitBranch                 *string                                `json:"git_branch"`
	GitBaseRef                *string                                `json:"git_base_ref"`
	GitHeadSha                *string                                `json:"git_head_sha"`
	GitDiffSha256             *string                                `json:"git_diff_sha256"`
	Metadata                  map[string]any                         `json:"metadata"`
	IdempotencyKey            string                                 `json:"idempotency_key"`
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
	LatestTaskResultID         *uuid.UUID
	RevisionOfTaskID           *uuid.UUID
	AcceptedPlanRevisionID     *uuid.UUID
	DecompositionClaimKey      *string
	PlanIteration              int32
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

type ProjectTaskResult struct {
	ID                 uuid.UUID
	TenantID           uuid.UUID
	ProjectID          uuid.UUID
	ProjectTaskID      uuid.UUID
	AttemptID          *uuid.UUID
	ExecutionSummaryID *uuid.UUID
	ResultStatus       TaskResultStatus
	ValidationStatus   string
	Decision           TaskResultDecision
	Contract           TaskResultContract
	ValidationErrors   []string
	ValidationWarnings []string
	IdempotencyKey     string
	DecisionRequestID  *uuid.UUID
	RevisionTaskID     *uuid.UUID
	CreatedEventID     *uuid.UUID
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type ProjectDemandSummary struct {
	ID                 uuid.UUID
	TenantID           uuid.UUID
	ProjectID          uuid.UUID
	DemandID           uuid.UUID
	Status             string
	Conclusion         string
	SummaryPayload     map[string]any
	ReportRefID        *uuid.UUID
	AcceptanceRequired bool
	IdempotencyKey     string
	CreatedEventID     *uuid.UUID
	CreatedAt          time.Time
	UpdatedAt          time.Time
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
	ProjectTaskRecoveryActionNoop           = "no_op"
	ProjectTaskRecoveryActionRetryScheduled = "retry_scheduled"
	ProjectTaskRecoveryActionWaitingHuman   = "waiting_human"
	ProjectTaskRecoveryActionFailed         = "failed"
)

type ProjectTaskRecoveryAction struct {
	Action         string
	FailureFamily  string
	Retryable      bool
	RetryNotBefore *time.Time
	WaitingReason  string
	TerminalReason string
}

type RecoverProjectTaskDispatchFailureRequest struct {
	TenantID       uuid.UUID
	ProjectID      uuid.UUID
	ProjectTaskID  uuid.UUID
	FailureEventID uuid.UUID
}

type RecoverProjectTaskDispatchFailureResult struct {
	Task     ProjectTask
	Event    ProjectEvent
	Decision DecisionRequest
	Action   string
}

type RecoverProjectTaskAttemptRequest struct {
	TenantID      uuid.UUID
	ProjectID     uuid.UUID
	ProjectTaskID uuid.UUID
	AttemptID     uuid.UUID
	FailureFamily string
	Summary       string
	Now           time.Time
}

type SweepProjectTaskAttemptRecoveryRequest struct {
	TenantID uuid.UUID
	Now      time.Time
	Limit    int32
}

type SweepProjectTaskAttemptRecoveryResult struct {
	RecoveredAttemptIDs []uuid.UUID
	RecoveredTaskIDs    []uuid.UUID
}

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
	FailureFamilyDispatchTransient     = "dispatch_transient"
	FailureFamilyRuntimeStartTimeout   = "runtime_start_timeout"
	FailureFamilyRuntimeLeaseLost      = "runtime_lease_lost"
	FailureFamilyProviderStart         = "transient_provider_start"
	FailureFamilyProviderConfig        = "provider_configuration"
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
	DigitalEmployeeID             *uuid.UUID
	ProviderType                  *string
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
	BudgetWallClockLimitSec       *int32
	BudgetLastHeartbeatAt         *time.Time
	BudgetConsumedWallClockSec    int32
	BudgetConsumedTokens          int32
	BudgetTrippedAt               *time.Time
	BudgetTripReason              *string
	CreatedAt                     time.Time
	UpdatedAt                     time.Time
}

type RecordProjectTaskAttemptBudgetHeartbeatRequest struct {
	TenantID             uuid.UUID `json:"tenant_id,omitempty"`
	ProjectID            uuid.UUID `json:"project_id,omitempty"`
	ProjectTaskID        uuid.UUID `json:"project_task_id"`
	AttemptID            uuid.UUID `json:"attempt_id"`
	ConsumedWallClockSec int32     `json:"consumed_wall_clock_sec"`
	ConsumedTokens       int32     `json:"consumed_tokens"`
}

type ProjectTaskAttemptBudgetHeartbeatResult struct {
	Attempt    ProjectTaskAttempt
	Tripped    bool
	TripReason string
}

type QueueProjectTaskRequest struct {
	TenantID                      uuid.UUID
	ProjectID                     uuid.UUID
	ProjectTaskID                 uuid.UUID
	ProjectTaskAttemptID          *uuid.UUID
	DigitalEmployeeID             uuid.UUID
	ProviderType                  string
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

type BindProjectTaskAttemptRunRequest struct {
	TenantID                      uuid.UUID
	ProjectID                     uuid.UUID
	ProjectTaskID                 uuid.UUID
	AttemptID                     uuid.UUID
	DigitalEmployeeRunID          uuid.UUID
	RuntimeTaskID                 uuid.UUID
	RuntimeNodeID                 uuid.UUID
	ProviderType                  string
	ExecutionContextPacket        map[string]any
	ExecutionContextPacketVersion string
}

type ProjectTaskAttemptRunBindingResult struct {
	Task    ProjectTask
	Attempt ProjectTaskAttempt
}

type FailQueuedProjectTaskAttemptDispatchStartRequest struct {
	TenantID               uuid.UUID
	ProjectID              uuid.UUID
	ProjectTaskID          uuid.UUID
	AttemptID              uuid.UUID
	DigitalEmployeeID      uuid.UUID
	LeaseToken             string
	FailureSummary         string
	FailureFamily          string
	Retryable              bool
	RetryNotBefore         *time.Time
	RestoreTaskStatus      string
	ClearCurrentAttempt    bool
	DispatchFailureEventID *uuid.UUID
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
	PlanRevisionID       *uuid.UUID
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
	CoordinationMode   string
	// ScenarioTemplateKey binds this demand to a scenario template,
	// overriding the project's default; nil falls back to the project's
	// ScenarioTemplateKey (and ultimately the generic fallback).
	ScenarioTemplateKey *string
}

type DemandLaunchDetail struct {
	Demand             ProjectDemand
	Project            Project
	Reviewer           *ReviewerPreference
	CoordinationJobs   []CoordinationJob
	RouteDecisions     []RouteDecision
	ProjectTasks       []ProjectTask
	ExecutionSummaries []ExecutionSummary
	DecisionRequests   []DecisionRequest
	RecentEvents       []ProjectEvent
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

// DemandConstraintExemption is a first-class human decision record: a
// human_owner exempting a single demand from a bound scenario template's
// governance constraint (e.g. role_independence) instead of restaffing the
// project pool. Persisted by Service.ResolveDecision when a planning_gap
// decision is resolved "exempted" (constraint_kind/roles read from the
// decision's own approval-request ContextPayload gap), and consumed by
// LoadProjectCoordinationSnapshot + EnforceScenarioTemplateGovernance to skip
// the exempted constraint on replan. Scoped to a single demand; the
// (tenant_id, demand_id, constraint_kind) uniqueness makes creation idempotent.
type DemandConstraintExemption struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DemandID          uuid.UUID
	ConstraintKind    string
	Roles             []string
	GrantedByUserID   uuid.UUID
	DecisionRequestID *uuid.UUID
	CreatedAt         time.Time
}

// DemandAcceptanceCriterion is a plan-level acceptance criterion snapshotted
// into demand_acceptance_criteria when its plan revision is decomposed into
// tasks. It fixes the judged criteria for that plan revision so the
// convergence gate, lineage and human sign-off all read a table instead of
// re-parsing the plan revision payload JSONB. See migration 064 and
// projectcoordination.PlanAcceptanceCriterion (the payload-side type this is
// snapshotted from, post normalizeCriterionDefaults).
type DemandAcceptanceCriterion struct {
	ID                 uuid.UUID
	TenantID           uuid.UUID
	ProjectID          uuid.UUID
	DemandID           uuid.UUID
	PlanRevisionID     uuid.UUID
	CriterionID        string
	Statement          string
	VerificationMethod string
	Severity           string
	SatisfiedBy        []string
	CreatedAt          time.Time
}

// DemandCriterionVerdict is one judgment against a demand_acceptance_criteria
// row: verdict "satisfied"/"unsatisfied", who judged it (judge_type
// "executor"/"human"), and the evidence behind it. Executor-sourced verdicts
// carry ProjectTaskID (one row per task per criterion, via
// uq_demand_verdicts_task); human sign-off verdicts leave it nil (one global
// row per criterion, via uq_demand_verdicts_human). See migration 064.
type DemandCriterionVerdict struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	ProjectID      uuid.UUID
	DemandID       uuid.UUID
	PlanRevisionID uuid.UUID
	CriterionID    string
	Verdict        string
	JudgeType      string
	JudgeID        uuid.UUID
	Reason         string
	EvidenceRefs   []string
	ProjectTaskID  *uuid.UUID
	CreatedAt      time.Time
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
	RepoBinding        *ProjectRepoBindingInput
	RuntimeNodeIDs     []uuid.UUID
	// ScenarioTemplateKey binds the project to a scenario template; nil means
	// the generic fallback (planning behaves exactly as an unbound project).
	ScenarioTemplateKey *string
}

// ProjectRuntimeNode is a runtime node bound to a project's eligibility set —
// the pool of nodes a task dispatched under this project may be placed on.
type ProjectRuntimeNode struct {
	ID            uuid.UUID
	TenantID      uuid.UUID
	ProjectID     uuid.UUID
	RuntimeNodeID uuid.UUID
	CreatedAt     time.Time
}

// ProjectEmployeeNodeAffinity records the soft, per-(project, digital employee)
// preference for a runtime node. It is written after a node is chosen for a new
// task so that later tasks for the same employee prefer the same node (sticky
// placement) while still allowing a cross-task switch when that node is offline.
type ProjectEmployeeNodeAffinity struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DigitalEmployeeID uuid.UUID
	RuntimeNodeID     uuid.UUID
	LastRunAt         *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
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
	RepoBinding        *ProjectRepoBindingInput
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
	CoordinationMode        string
	// ScenarioTemplateKey binds this demand to a scenario template; nil
	// means fall back to the project's default (and generic beyond that).
	ScenarioTemplateKey *string
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
	// TargetExitDeliverable is the human's replacement exit deliverable choice;
	// only meaningful when Decision is request_changes. It pins the replan's
	// exit via CoordinationSnapshot.PinnedExitDeliverable.
	TargetExitDeliverable string
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
	RawLog            *ProjectTaskAttemptRawLog
}

// ProjectTaskAttemptRawLog points at the unparsed provider stdout/stderr the
// runtime uploaded. LogSha256 is reported by the process that produced the
// bytes, so it is a checksum until the control plane recomputes it.
type ProjectTaskAttemptRawLog struct {
	LogStore      string
	LogRef        string
	LogBytes      int64
	LogSha256     string
	LogCompressed bool
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
	ResultContract        *TaskResultContract
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
	ResultContract    *TaskResultContract
}

type WaitHumanProjectTaskAttemptRequest struct {
	ProjectTaskAttemptRuntimeRequest
	DigitalEmployeeID          uuid.UUID
	Reason                     string
	Summary                    string
	MissingContextRefs         []any
	SuggestedResolutionOptions []string
	ResultContract             *TaskResultContract
}

type SubmitProjectTaskAttemptResultRequest struct {
	ProjectTaskAttemptRuntimeRequest
	ResultContract TaskResultContract
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
