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

type CreateProjectTasksInput struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
	DemandID  uuid.UUID
	Decision  RouteDecisionPlan
}

type RequestRouteDecisionReviewInput struct {
	TenantID            uuid.UUID
	ProjectID           uuid.UUID
	CoordinationJobID   uuid.UUID
	DemandID            uuid.UUID
	RouteDecisionID     uuid.UUID
	Decision            RouteDecisionPlan
	ProjectTaskIDs      []uuid.UUID
	RouteCreatedEventID uuid.UUID
}

type AppendProjectEventInput struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
	EventType string
	Summary   string
}

type DispatchProjectTaskInput struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
	TaskID    uuid.UUID
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
