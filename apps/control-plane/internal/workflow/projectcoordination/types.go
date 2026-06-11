package projectcoordination

import "github.com/google/uuid"

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
