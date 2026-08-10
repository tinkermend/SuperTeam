package project

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type CoordinatorSignalClient interface {
	EnsureProjectCoordinator(ctx context.Context, signal ProjectCoordinatorSignal) error
	SignalDemandSubmitted(ctx context.Context, signal DemandSubmittedSignal) error
	SignalProjectPolicyChanged(ctx context.Context, signal ProjectPolicyChangedSignal) error
	SignalProjectMemberChanged(ctx context.Context, signal ProjectMemberChangedSignal) error
	SignalEmployeeTaskCompleted(ctx context.Context, signal EmployeeTaskCompletedSignal) error
	SignalEmployeeTaskFailed(ctx context.Context, signal EmployeeTaskFailedSignal) error
	SignalEmployeeTransferRequested(ctx context.Context, signal EmployeeTransferRequestedSignal) error
	SignalHumanDecisionSubmitted(ctx context.Context, signal HumanDecisionSubmittedSignal) error
	// SignalProjectTaskRetryScheduled wakes the coordinator to (re)dispatch a
	// task whose attempt-level failure requeued it. Distinct from
	// EmployeeTaskFailed, which opens human recovery rather than auto-retry.
	SignalProjectTaskRetryScheduled(ctx context.Context, signal ProjectTaskRetryScheduledSignal) error
	TerminateProjectCoordinator(ctx context.Context, signal TerminateProjectCoordinatorSignal) error
}

type NoopCoordinatorSignalClient struct{}

func (NoopCoordinatorSignalClient) EnsureProjectCoordinator(context.Context, ProjectCoordinatorSignal) error {
	return nil
}

func (NoopCoordinatorSignalClient) SignalDemandSubmitted(context.Context, DemandSubmittedSignal) error {
	return nil
}

func (NoopCoordinatorSignalClient) SignalProjectPolicyChanged(context.Context, ProjectPolicyChangedSignal) error {
	return nil
}

func (NoopCoordinatorSignalClient) SignalProjectMemberChanged(context.Context, ProjectMemberChangedSignal) error {
	return nil
}

func (NoopCoordinatorSignalClient) SignalEmployeeTaskCompleted(context.Context, EmployeeTaskCompletedSignal) error {
	return nil
}

func (NoopCoordinatorSignalClient) SignalEmployeeTaskFailed(context.Context, EmployeeTaskFailedSignal) error {
	return nil
}

func (NoopCoordinatorSignalClient) SignalEmployeeTransferRequested(context.Context, EmployeeTransferRequestedSignal) error {
	return nil
}

func (NoopCoordinatorSignalClient) SignalHumanDecisionSubmitted(context.Context, HumanDecisionSubmittedSignal) error {
	return nil
}

func (NoopCoordinatorSignalClient) SignalProjectTaskRetryScheduled(context.Context, ProjectTaskRetryScheduledSignal) error {
	return nil
}

func (NoopCoordinatorSignalClient) TerminateProjectCoordinator(context.Context, TerminateProjectCoordinatorSignal) error {
	return nil
}

type ProjectCoordinatorSignal struct {
	TenantID   uuid.UUID
	ProjectID  uuid.UUID
	WorkflowID string
}

type DemandSubmittedSignal struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DemandID          uuid.UUID
	SubmittedByUserID uuid.UUID
	CreatedEventID    uuid.UUID
	WorkflowID        string
}

type ProjectPolicyChangedSignal struct {
	TenantID         uuid.UUID
	ProjectID        uuid.UUID
	ConfigRevisionID uuid.UUID
	ChangedEventID   uuid.UUID
	WorkflowID       string
}

type ProjectMemberChangedSignal struct {
	TenantID         uuid.UUID
	ProjectID        uuid.UUID
	ChangedMemberIDs []uuid.UUID
	ChangedEventID   uuid.UUID
	WorkflowID       string
}

type EmployeeTaskCompletedSignal struct {
	TenantID           uuid.UUID
	ProjectID          uuid.UUID
	ProjectTaskID      uuid.UUID
	ExecutionSummaryID uuid.UUID
	CompletedEventID   uuid.UUID
	WorkflowID         string
}

type EmployeeTaskFailedSignal struct {
	TenantID       uuid.UUID
	ProjectID      uuid.UUID
	ProjectTaskID  uuid.UUID
	FailureSummary string
	FailedEventID  uuid.UUID
	WorkflowID     string
}

type EmployeeTransferRequestedSignal struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	ProjectTaskID     uuid.UUID
	TransferRequestID uuid.UUID
	RequestedEventID  uuid.UUID
	WorkflowID        string
}

type HumanDecisionSubmittedSignal struct {
	TenantID              uuid.UUID
	ProjectID             uuid.UUID
	ApprovalRequestID     uuid.UUID
	DecisionRequestID     uuid.UUID
	Decision              string
	Payload               map[string]any
	ResolvedEventID       uuid.UUID
	WorkflowID            string
	TargetExitDeliverable string
}

// ProjectTaskRetryScheduledSignal asks the coordinator to redispatch a task
// after an attempt-level failure requeued it (or a watchdog recovery did).
// RetryNotBefore is optional; when set in the future the workflow sleeps first.
type ProjectTaskRetryScheduledSignal struct {
	TenantID       uuid.UUID
	ProjectID      uuid.UUID
	ProjectTaskID  uuid.UUID
	RetryNotBefore *time.Time
	WorkflowID     string
}

type TerminateProjectCoordinatorSignal struct {
	TenantID   uuid.UUID
	ProjectID  uuid.UUID
	WorkflowID string
	Reason     string
}
