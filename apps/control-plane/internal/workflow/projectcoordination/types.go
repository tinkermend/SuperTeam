package projectcoordination

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/project"
	"go.temporal.io/sdk/temporal"
)

const (
	SignalDemandSubmitted           = "DemandSubmitted"
	SignalProjectPolicyChanged      = "ProjectPolicyChanged"
	SignalProjectMemberChanged      = "ProjectMemberChanged"
	SignalEmployeeTaskCompleted     = "EmployeeTaskCompleted"
	SignalEmployeeTaskFailed        = "EmployeeTaskFailed"
	SignalEmployeeTransferRequested = "EmployeeTransferRequested"
	SignalHumanDecisionSubmitted    = "HumanDecisionSubmitted"
	SignalShutdown                  = "Shutdown"
)

type ProjectCoordinatorInput struct {
	TenantID   uuid.UUID
	ProjectID  uuid.UUID
	WorkflowID string
	Generation int
}

type DemandSubmitted struct {
	DemandID          uuid.UUID
	ProjectID         uuid.UUID
	SubmittedByUserID uuid.UUID
	CreatedEventID    uuid.UUID
}

type ProjectPolicyChanged struct {
	ProjectID        uuid.UUID
	ConfigRevisionID uuid.UUID
	ChangedEventID   uuid.UUID
}

type ProjectMemberChanged struct {
	ProjectID        uuid.UUID
	ChangedMemberIDs []uuid.UUID
	ChangedEventID   uuid.UUID
}

type EmployeeTaskCompleted struct {
	ProjectTaskID      uuid.UUID
	ExecutionSummaryID uuid.UUID
	CompletedEventID   uuid.UUID
}

type EmployeeTaskFailed struct {
	ProjectTaskID  uuid.UUID
	FailureSummary string
	FailedEventID  uuid.UUID
}

type EmployeeTransferRequested struct {
	ProjectTaskID     uuid.UUID
	TransferRequestID uuid.UUID
	RequestedEventID  uuid.UUID
}

type HumanDecisionSubmitted struct {
	ApprovalRequestID uuid.UUID
	DecisionRequestID uuid.UUID
	Decision          string
	Payload           map[string]any
	ResolvedEventID   uuid.UUID
	// TargetExitDeliverable carries the human's replacement exit deliverable
	// choice when Decision is request_changes. It pins the replan's exit via
	// CoordinationSnapshot.PinnedExitDeliverable; empty means no override.
	TargetExitDeliverable string `json:"target_exit_deliverable,omitempty"`
}

type ShutdownSignal struct{}

type LoadSnapshotInput struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
	DemandID  uuid.UUID
}

type CreateCoordinationJobInput struct {
	TenantID       uuid.UUID
	ProjectID      uuid.UUID
	WorkflowID     string
	TriggerEventID uuid.UUID
	JobType        string
}

type PersistRouteDecisionInput struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
	JobID     uuid.UUID
	DemandID  uuid.UUID
	Decision  RouteDecisionPlan
}

type PersistPlanRevisionInput struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DemandID          uuid.UUID
	CoordinationJobID uuid.UUID
	RouteDecisionID   uuid.UUID
	Decision          RouteDecisionPlan
	SupersedeOpen     bool
	SupersedeReason   *string
	// CoordinationMode gates the plan-confirmation status: plan-mode demands
	// (empty/"plan") always land in PendingReview; only autonomous modes
	// (loop/chat) keep the conditional-Accepted auto-dispatch path. Sourced
	// from snapshot.Demand.CoordinationMode at the workflow call site.
	CoordinationMode string
}

type PlanRevisionResult struct {
	ID              uuid.UUID
	Status          string
	RevisionNumber  int32
	PlanFingerprint string
	Payload         PlanRevisionPayload
	ReviewRequired  bool
	CreatedEventID  uuid.UUID
}

type RequestPlanRevisionReviewInput struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	CoordinationJobID uuid.UUID
	DemandID          uuid.UUID
	PlanRevisionID    uuid.UUID
	PlanFingerprint   string
	Payload           PlanRevisionPayload
	CreatedEventID    uuid.UUID
}

type ResolvePlanRevisionReviewInput struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DemandID          uuid.UUID
	CoordinationJobID uuid.UUID
	PlanRevisionID    uuid.UUID
	DecisionRequestID uuid.UUID
	Decision          string
	Payload           map[string]any
	ActorUserID       uuid.UUID
}

type LoadHumanDecisionRouteInput struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DecisionRequestID uuid.UUID
}

type HumanDecisionRouteResult struct {
	Decision    ProjectDecisionSnapshot
	PlanReview  *PlanReviewRoute
	PlanningGap *PlanningGapRoute
}

// PlanningGapRoute carries the demand a planning_gap decision is about, resolved
// from the decision's approval-request context payload, so the coordinator can
// reopen and replan that demand when the human resolves the decision restaffed.
type PlanningGapRoute struct {
	ProjectID uuid.UUID
	DemandID  uuid.UUID
}

// ReopenProjectDemandForReplanningInput reopens a failed demand back into
// planning_pending (the planning_gap → restaffed path) via an activity boundary.
type ReopenProjectDemandForReplanningInput struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
	DemandID  uuid.UUID
}

type ProjectDecisionSnapshot struct {
	ID                   uuid.UUID
	ProjectID            uuid.UUID
	DecisionType         string
	StatusSnapshot       string
	CoordinationJobID    uuid.UUID
	ProjectTaskID        uuid.UUID
	PlanRevisionID       uuid.UUID
	DispatchGateResultID uuid.UUID
	CreatedEventID       uuid.UUID
}

type PlanReviewRoute struct {
	ProjectID         uuid.UUID
	DemandID          uuid.UUID
	CoordinationJobID uuid.UUID
	RouteDecisionID   uuid.UUID
	PlanRevisionID    uuid.UUID
	PlanFingerprint   string
	Payload           PlanRevisionPayload
	RouteEventID      uuid.UUID
	PlanEventID       uuid.UUID
	OutputEventIDs    []uuid.UUID
}

type DecomposeAcceptedPlanRevisionInput struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DemandID          uuid.UUID
	CoordinationJobID uuid.UUID
	RouteDecisionID   uuid.UUID
	PlanRevisionID    uuid.UUID
	PlanFingerprint   string
	Payload           PlanRevisionPayload
}

type ListDispatchableTasksInput struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	CoordinationJobID uuid.UUID
}

type ResolveReadyDownstreamInput struct {
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	CompletedTaskID uuid.UUID
}

type InspectTaskResultDecisionInput struct {
	TenantID      uuid.UUID
	ProjectID     uuid.UUID
	ProjectTaskID uuid.UUID
}

type InspectTaskResultDecisionResult struct {
	ResultID         uuid.UUID
	Decision         string
	Exhausted        bool
	Blocker          *project.TaskResultBlocker
	CoordinationMode string
}

type CreateRevisionTaskForResultInput struct {
	TenantID     uuid.UUID
	ProjectID    uuid.UUID
	SourceTaskID uuid.UUID
	ResultID     uuid.UUID
}

type CreateRevisionTaskForResultResult struct {
	TaskID    uuid.UUID
	Exhausted bool
}

// CreateUpstreamSupplementInput describes a blocked task and the inputs it lacks.
type CreateUpstreamSupplementInput struct {
	TenantID      uuid.UUID
	ProjectID     uuid.UUID
	SourceTaskID  uuid.UUID
	MissingInputs []string
}

// CreateUpstreamSupplementResult reports the owner tasks appended to supply the
// blocked task's missing inputs. Exhausted is true when the graph has already
// extended max_plan_iterations rounds and no further supplement was created.
type CreateUpstreamSupplementResult struct {
	TaskIDs   []uuid.UUID
	Exhausted bool
}

type IsProjectAcceptanceReadyInput struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
}

type RequestProjectAcceptanceReviewInput struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
}

// EnsureDemandAcceptanceDecisionForTaskInput carries just the task the
// coordinator's completed/failed signal fired for; ProjectStore resolves the
// task's demand internally, since neither EmployeeTaskCompleted nor
// EmployeeTaskFailed carries a demand ID directly.
type EnsureDemandAcceptanceDecisionForTaskInput struct {
	TenantID      uuid.UUID
	ProjectID     uuid.UUID
	ProjectTaskID uuid.UUID
}

type RequestProjectTaskIterationExhaustedReviewInput struct {
	TenantID       uuid.UUID
	ProjectID      uuid.UUID
	ProjectTaskID  uuid.UUID
	ResultID       uuid.UUID
	Reason         string
	Summary        string
	CreatedEventID uuid.UUID
}

// RequestUpstreamSupplementReviewInput requests a human decision gate before the
// coordinator creates upstream supplement tasks for a blocked task's missing inputs.
type RequestUpstreamSupplementReviewInput struct {
	TenantID         uuid.UUID
	ProjectID        uuid.UUID
	ProjectTaskID    uuid.UUID
	ResultID         uuid.UUID
	CompletedEventID uuid.UUID
	MissingInputs    []string
}

type ApplyProjectAcceptanceDecisionInput struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DecisionRequestID uuid.UUID
	Decision          string
	Payload           map[string]any
}

type HoldDownstreamForFailureInput struct {
	TenantID       uuid.UUID
	ProjectID      uuid.UUID
	FailedTaskID   uuid.UUID
	FailureSummary string
	FailedEventID  uuid.UUID
}

type ApplyFailureRecoveryDecisionInput struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DecisionRequestID uuid.UUID
	Decision          string
	Payload           map[string]any
}

type ApplyFailureRecoveryDecisionResult struct {
	ReadyTaskIDs []uuid.UUID
}

type ApplyPreDispatchGateDecisionInput struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DecisionRequestID uuid.UUID
	Decision          string
	Payload           map[string]any
}

type ApplyPreDispatchGateDecisionResult struct {
	ReadyTaskIDs []uuid.UUID
}

type FailureRecoveryAction struct {
	Action               string
	NewDigitalEmployeeID *uuid.UUID
}

type AppendProjectEventInput struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
	EventType string
	Summary   string
}

type DispatchProjectTaskInput struct {
	TenantID       uuid.UUID
	ProjectID      uuid.UUID
	TaskID         uuid.UUID
	DispatchReason string
}

// RecoverTaskDispatchFailureInput deliberately carries no failure event ID: a
// *ProjectTaskDispatchError does not survive the Temporal activity boundary
// (the workflow only sees a *temporal.ApplicationError), so the recovery
// service resolves the latest dispatch_failed event itself.
type RecoverTaskDispatchFailureInput struct {
	TenantID      uuid.UUID
	ProjectID     uuid.UUID
	ProjectTaskID uuid.UUID
}

type RecoverTaskDispatchFailureResult struct {
	Action         string
	RetryNotBefore *time.Time
}

type StartProjectTaskRunRequest struct {
	TenantID             uuid.UUID
	ProjectID            uuid.UUID
	DemandID             uuid.UUID
	ProjectTaskID        uuid.UUID
	ProjectTaskAttemptID uuid.UUID
	DigitalEmployeeID    uuid.UUID
	DispatchUserID       uuid.UUID
	Objective            string
	Prompt               string
	IdempotencyKey       string
	Metadata             map[string]any
	WorkspaceMode        string
	BaseRef              string
	ProjectGit           map[string]any
}

type StartProjectTaskRunResult struct {
	RunID         uuid.UUID
	RuntimeTaskID uuid.UUID
	RuntimeNodeID uuid.UUID
	NodeID        string
	ProviderType  string
}

// ProjectTaskRunStartError lets the run starter adapter classify whether a failed
// run start is transient (retryable) or terminal, without coupling the coordination
// store to the employee package's error sentinels.
type ProjectTaskRunStartError struct {
	Retryable bool
	Err       error
}

func (e *ProjectTaskRunStartError) Error() string {
	if e == nil || e.Err == nil {
		return "project task run start failed"
	}
	return e.Err.Error()
}

func (e *ProjectTaskRunStartError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type ProjectTaskDispatchError struct {
	FailureRecorded bool
	Err             error
}

var ErrProjectTaskDispatchRetryLater = errors.New("project task dispatch retry later")

func (e *ProjectTaskDispatchError) Error() string {
	if e == nil || e.Err == nil {
		return "project task dispatch failed"
	}
	return e.Err.Error()
}

func (e *ProjectTaskDispatchError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func dispatchFailureRecorded(err error) bool {
	for current := err; current != nil; current = errors.Unwrap(current) {
		if dispatchErr, ok := current.(*ProjectTaskDispatchError); ok {
			return dispatchErr.FailureRecorded
		}
		if appErr, ok := current.(*temporal.ApplicationError); ok && appErr.Type() == "ProjectTaskDispatchError" {
			return true
		}
	}
	return false
}

type FinishCoordinationJobInput struct {
	TenantID       uuid.UUID
	JobID          uuid.UUID
	Status         string
	OutputEventIDs []uuid.UUID
}

// PlanningGap is the structured, machine-actionable form of a structural
// no-suitable-employee rejection: the free-text Diagnosis message tells a human
// what happened, PlanningGap tells downstream surfaces (web, future automation)
// exactly which constraint failed and what the three ways out are, without
// having to re-parse the diagnosis prose. It originates where the knowledge
// lives — governance's enforceRoleIndependence/structuralGapForPlan
// (template_governance.go) — travels as a temporal ApplicationError detail
// across the PlanDemandRoute activity boundary, and is threaded through
// RejectDemandPlanningInput.Gap into the coordination.blocked event payload
// (project_store.go). Absent (nil) on any diagnosis that is not a structural
// role_independence gap, and on replays of histories recorded before this field
// existed — see noSuitableEmployeeDiagnosis in workflow.go.
type PlanningGap struct {
	ConstraintKind       string   `json:"constraint_kind"`                 // e.g. role_independence
	Roles                []string `json:"roles,omitempty"`                 // [reviewer developer]
	RequiredCapabilities []string `json:"required_capabilities,omitempty"` // union of the constrained roles' required_capabilities
	ActiveExecutorCount  int      `json:"active_executor_count"`
	Options              []string `json:"options"` // [restaff exempt lending]
}

// RejectDemandPlanningInput terminally rejects a demand whose route could not be
// planned (the ErrNoSuitableEmployee family). It moves the demand out of
// planning_pending into a human-visible terminal state and records the diagnosis
// on a demand-scoped coordination.blocked event.
type RejectDemandPlanningInput struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DemandID          uuid.UUID
	CoordinationJobID uuid.UUID
	Diagnosis         string
	Gap               *PlanningGap `json:"gap,omitempty"`
	OutputEventIDs    []uuid.UUID
}

type CoordinationJobResult struct {
	ID uuid.UUID
}

type RouteDecisionResult struct {
	ID             uuid.UUID
	CreatedEventID uuid.UUID
}

type ProjectTaskResult struct {
	ID uuid.UUID
}

type ProjectEventResult struct {
	ID uuid.UUID
}

type DecisionRequestResult struct {
	ID uuid.UUID
}

// --- Adversarial AI review (autonomy posture Phase B) ---

// RunAdversarialReviewInput carries everything the pure judge engine needs to
// decide one adversarial_review acceptance criterion. It is DB-free by
// construction: Task 4's caller assembles the reviewed task's evidence (via
// collectUpstreamResults / the task result contract) and populates the budget
// verdict, so the engine never touches the store. JudgeCountPolicy is the raw
// projects.coordination_policy.adversarial_review_judges value (0 = unset →
// default 3, hard cap 7). BudgetExhausted is populated by Task 4 from
// (*ProjectStore).revisionBudgetExhausted for the reviewed task.
type RunAdversarialReviewInput struct {
	TenantID         uuid.UUID
	ProjectID        uuid.UUID
	CriterionID      string
	ReviewedTaskID   uuid.UUID
	Assertion        string   // the criterion's decidable assertion text
	EvidenceSummary  string   // reviewed task's result summary
	Deliverables     []string // reviewed task's declared deliverables
	EvidenceRefs     []string // reviewed task's evidence references
	JudgeCountPolicy int      // coordination_policy.adversarial_review_judges (0 = default)
	BudgetExhausted  bool     // Task 4 populates from revisionBudgetExhausted
	Model            string   // model id for judge calls (optional passthrough)
}

// AdversarialLens is one adversarial perspective: a stable key plus a
// refute-not-confirm system prompt.
type AdversarialLens struct {
	Key          string
	SystemPrompt string
}

// AdversarialJudgement is one judge's structured verdict on the criterion.
// Verdict is "refuted" | "accepted".
type AdversarialJudgement struct {
	Lens    string
	Verdict string
	Reason  string
}

// AdversarialReviewResult aggregates the N judges. Aggregate is
// "satisfied" (minority or no refute) | "unsatisfied" (majority refute) |
// "escalate_human" (budget exhausted; Task 4 turns this into a tier-3 hold).
type AdversarialReviewResult struct {
	CriterionID    string
	ReviewedTaskID uuid.UUID
	Judgements     []AdversarialJudgement
	Aggregate      string
	RefutedCount   int
	JudgeCount     int
}
