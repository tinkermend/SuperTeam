package projectcoordination

import (
	"errors"

	"github.com/google/uuid"
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

type IsProjectAcceptanceReadyInput struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
}

type RequestProjectAcceptanceReviewInput struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
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

type StartProjectTaskRunRequest struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DemandID          uuid.UUID
	ProjectTaskID     uuid.UUID
	DigitalEmployeeID uuid.UUID
	DispatchUserID    uuid.UUID
	Objective         string
	Prompt            string
	IdempotencyKey    string
	Metadata          map[string]any
}

type StartProjectTaskRunResult struct {
	RunID         uuid.UUID
	RuntimeTaskID uuid.UUID
	RuntimeNodeID uuid.UUID
	NodeID        string
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
