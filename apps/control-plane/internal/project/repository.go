package project

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	CreateProject(ctx context.Context, req CreateProjectRequest, projectID uuid.UUID, workflowID string) (Project, error)
	GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (Project, error)
	ListProjects(ctx context.Context, req ListProjectsRequest) ([]Project, error)
	ListWorkflowInstances(ctx context.Context, req ListWorkflowInstancesRequest) ([]WorkflowInstanceSummary, error)
	UpdateProjectConfig(ctx context.Context, req UpdateProjectConfigRequest) (Project, error)
	ArchiveProject(ctx context.Context, tenantID, projectID uuid.UUID) (Project, error)
	TransitionProjectStatus(ctx context.Context, tenantID, projectID uuid.UUID, fromStatuses []string, toStatus string) (Project, error)
	AreAllProjectDemandsTerminal(ctx context.Context, tenantID, projectID uuid.UUID) (bool, error)
	ReplaceProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID, members []ProjectMemberInput) ([]ProjectMember, error)
	ListProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectMember, error)
	ListProjectTasks(ctx context.Context, tenantID, projectID uuid.UUID, status *string, limit, offset int32) ([]ProjectTask, error)
	AppendProjectEvent(ctx context.Context, event AppendProjectEventRequest) (ProjectEvent, error)
	GetProjectEvent(ctx context.Context, tenantID, projectID, eventID uuid.UUID) (ProjectEvent, error)
	GetProjectEventByTypeAndActor(ctx context.Context, tenantID, projectID uuid.UUID, eventType ProjectEventType, actorID string) (ProjectEvent, error)
	ListProjectEvents(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectEvent, error)
	CreateProjectDemand(ctx context.Context, req SubmitProjectDemandRequest, status ProjectDemandStatus, createdEventID *uuid.UUID) (ProjectDemand, error)
	ListProjectDemands(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectDemand, error)
	CreateConfigRevision(ctx context.Context, req UpdateProjectConfigRequest, project Project, eventID uuid.UUID) (ProjectConfigRevision, error)
	GetProjectDemand(ctx context.Context, tenantID, demandID uuid.UUID) (ProjectDemand, error)
	GetProjectTask(ctx context.Context, tenantID, projectTaskID uuid.UUID) (ProjectTask, error)
	CreateCoordinationJob(ctx context.Context, req CreateCoordinationJobRequest) (CoordinationJob, error)
	FinishCoordinationJob(ctx context.Context, req FinishCoordinationJobRequest) (CoordinationJob, error)
	ListCoordinationJobs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]CoordinationJob, error)
	ListDemandLaunchCoordinationJobs(ctx context.Context, tenantID, projectID, demandID uuid.UUID, createdEventID *uuid.UUID, limit int32) ([]CoordinationJob, error)
	CreateRouteDecision(ctx context.Context, req CreateRouteDecisionRequest) (RouteDecision, error)
	ListRouteDecisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]RouteDecision, error)
	ListDemandLaunchRouteDecisions(ctx context.Context, tenantID, projectID, demandID uuid.UUID, limit int32) ([]RouteDecision, error)
	CreatePlanRevision(ctx context.Context, req CreatePlanRevisionRequest) (PlanRevision, error)
	GetPlanRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (PlanRevision, error)
	ListPlanRevisions(ctx context.Context, req ListPlanRevisionsRequest) ([]PlanRevision, error)
	ListPlanRevisionsForDemand(ctx context.Context, tenantID, projectID, demandID uuid.UUID) ([]PlanRevision, error)
	AcceptPlanRevision(ctx context.Context, req AcceptPlanRevisionRequest) (PlanRevision, error)
	RejectPlanRevision(ctx context.Context, req RejectPlanRevisionRequest) (PlanRevision, error)
	CreateProjectTask(ctx context.Context, req CreateProjectTaskRequest) (ProjectTask, error)
	CreateProjectTaskGraph(ctx context.Context, req CreateProjectTaskGraphRequest) (CreateProjectTaskGraphResult, error)
	ListProjectTaskDependencies(ctx context.Context, tenantID, projectID uuid.UUID, dependentTaskIDs []uuid.UUID) ([]ProjectTaskDependency, error)
	ListDependentsOfTask(ctx context.Context, tenantID, projectID, blockerTaskID uuid.UUID) ([]uuid.UUID, error)
	ListUnresolvedBlockersForTasks(ctx context.Context, tenantID, projectID uuid.UUID, dependentTaskIDs []uuid.UUID) ([]ProjectTaskDependencyReadiness, error)
	ListProjectTasksByCoordinationJob(ctx context.Context, tenantID, projectID, coordinationJobID uuid.UUID) ([]ProjectTask, error)
	GetProjectTaskCompletionContract(ctx context.Context, tenantID, taskID uuid.UUID) (ProjectTaskCompletionContract, error)
	GetCoordinationJobByTrigger(ctx context.Context, tenantID uuid.UUID, workflowID string, triggerEventID uuid.UUID, jobType string) (CoordinationJob, error)
	GetRouteDecision(ctx context.Context, tenantID, routeDecisionID uuid.UUID) (RouteDecision, error)
	GetRouteDecisionByCoordinationJob(ctx context.Context, tenantID, coordinationJobID uuid.UUID) (RouteDecision, error)
	GetProjectTaskGraph(ctx context.Context, req GetProjectTaskGraphRequest) (ProjectTaskGraph, error)
	ListDemandLaunchProjectTasks(ctx context.Context, tenantID, projectID, demandID uuid.UUID, limit int32) ([]ProjectTask, error)
	RecordProjectTaskResult(ctx context.Context, req RecordProjectTaskResultRequest) (ProjectTaskResult, error)
	ListProjectTaskResults(ctx context.Context, req ListProjectTaskResultsRequest) ([]ProjectTaskResult, error)
	LinkProjectTaskLatestResult(ctx context.Context, tenantID, projectID, projectTaskID, resultID uuid.UUID) (ProjectTask, error)
	LinkProjectTaskResultDecisionRequest(ctx context.Context, tenantID, projectID, resultID, decisionRequestID uuid.UUID) (ProjectTaskResult, error)
	LinkProjectTaskResultRevisionTask(ctx context.Context, tenantID, projectID, resultID, revisionTaskID uuid.UUID) (ProjectTaskResult, error)
	CreateProjectTaskAttestation(ctx context.Context, req CreateProjectTaskAttestationRequest) (ProjectTaskAttestation, error)
	ListProjectTaskAttestations(ctx context.Context, tenantID, projectID, projectTaskID uuid.UUID, limit, offset int32) ([]ProjectTaskAttestation, error)
	CreateProjectDemandSummary(ctx context.Context, req CreateProjectDemandSummaryRequest) (ProjectDemandSummary, error)
	GetLatestProjectDemandSummary(ctx context.Context, tenantID, projectID, demandID uuid.UUID) (ProjectDemandSummary, error)
	QueueProjectTaskWithAttempt(ctx context.Context, req QueueProjectTaskRequest) (QueueProjectTaskResult, error)
	RecordPreDispatchGateResult(ctx context.Context, req RecordPreDispatchGateResultRequest) (PreDispatchGateResult, error)
	GetPreDispatchGateResult(ctx context.Context, tenantID, projectID, gateResultID uuid.UUID) (PreDispatchGateResult, error)
	GetPreDispatchGateResultByKey(ctx context.Context, tenantID, projectID, projectTaskID uuid.UUID, idempotencyKey string) (PreDispatchGateResult, error)
	ListPreDispatchGateResults(ctx context.Context, req ListPreDispatchGateResultsRequest) ([]PreDispatchGateResult, error)
	LinkPreDispatchGateAttempt(ctx context.Context, req LinkPreDispatchGateAttemptRequest) (PreDispatchGateResult, error)
	LinkPreDispatchGateDecisionRequest(ctx context.Context, req LinkPreDispatchGateDecisionRequest) (PreDispatchGateResult, error)
	MoveProjectTaskToWaitingHumanForPreDispatchGate(ctx context.Context, req MoveProjectTaskToWaitingHumanForPreDispatchGateRequest) (ProjectTask, error)
	GetProjectTaskAttempt(ctx context.Context, tenantID, attemptID uuid.UUID) (ProjectTaskAttempt, error)
	UpdateProjectTaskAttemptBudgetHeartbeat(ctx context.Context, req RecordProjectTaskAttemptBudgetHeartbeatRequest, tripReason string) (ProjectTaskAttempt, error)
	GetCurrentProjectTaskAttempt(ctx context.Context, tenantID, projectTaskID uuid.UUID) (ProjectTaskAttempt, error)
	RecordProjectTaskAttemptContextUpdate(ctx context.Context, req RecordProjectTaskAttemptContextUpdateRepositoryRequest) (ProjectTaskAttemptContextUpdate, error)
	DecomposeAcceptedPlanRevision(ctx context.Context, req DecomposeAcceptedPlanRevisionRequest) (DecomposeAcceptedPlanRevisionResult, error)
	UpdateProjectTaskStatus(ctx context.Context, tenantID, projectTaskID uuid.UUID, status string, eventID *uuid.UUID, currentStatuses []string) (ProjectTask, error)
	BindProjectTaskRun(ctx context.Context, req BindProjectTaskRunRequest) (ProjectTask, error)
	AdvanceProjectDemandStatus(ctx context.Context, tenantID, projectID, demandID uuid.UUID, target ProjectDemandStatus) error
	ProjectTaskEventExists(ctx context.Context, tenantID, projectID uuid.UUID, eventType ProjectEventType, actorID string) (bool, error)
	AssignProjectTask(ctx context.Context, tenantID, projectTaskID uuid.UUID, status string, assignedDigitalEmployeeID, eventID *uuid.UUID) (ProjectTask, error)
	CreateExecutionSummary(ctx context.Context, req CreateExecutionSummaryRequest) (ExecutionSummary, error)
	ListExecutionSummaries(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ExecutionSummary, error)
	CreateExecutionLedgerEvent(ctx context.Context, req CreateExecutionLedgerEventRequest) (ExecutionLedgerEvent, error)
	ListProjectExecutionLedgerEvents(ctx context.Context, req GetExecutionTraceRequest) ([]ExecutionLedgerEvent, error)
	ListProjectTaskAttemptsForExecutionTrace(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectTaskAttempt, error)
	CreateTransferRequest(ctx context.Context, req CreateTransferRequestRequest) (TransferRequest, error)
	ListTransferRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]TransferRequest, error)
	CreateDecisionRequest(ctx context.Context, req CreateDecisionRequestRequest) (DecisionRequest, error)
	GetDecisionRequest(ctx context.Context, tenantID, projectID, decisionRequestID uuid.UUID) (DecisionRequest, error)
	GetDecisionRequestByPlanRevision(ctx context.Context, tenantID, projectID, planRevisionID uuid.UUID) (DecisionRequest, error)
	ResolveDecisionRequest(ctx context.Context, req ResolveDecisionRequestRepositoryRequest) (DecisionRequest, error)
	ListDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]DecisionRequest, error)
	ListDemandLaunchDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, coordinationJobIDs, projectTaskIDs []uuid.UUID, limit int32) ([]DecisionRequest, error)
	ListDemandLaunchEvents(ctx context.Context, tenantID, projectID, demandID uuid.UUID, createdEventID *uuid.UUID, projectTaskIDs, decisionRequestIDs []uuid.UUID, limit int32) ([]ProjectEvent, error)
	CreateEvidenceRef(ctx context.Context, req CreateEvidenceRefRequest) (ProjectEvidenceRef, error)
	CreateEvidenceRefWithEvent(ctx context.Context, req CreateEvidenceRefWithEventRequest) (ProjectEvidenceRefWriteResult, error)
	ListEvidenceRefs(ctx context.Context, tenantID, projectID uuid.UUID, status *EvidenceVerificationStatus, limit, offset int32) ([]ProjectEvidenceRef, error)
	UpdateEvidenceVerificationStatus(ctx context.Context, req UpdateEvidenceVerificationStatusRequest) (ProjectEvidenceRef, error)
	UpdateEvidenceVerificationStatusWithEvent(ctx context.Context, req UpdateEvidenceVerificationStatusWithEventRequest) (ProjectEvidenceRefWriteResult, error)
	CreateArtifactRef(ctx context.Context, req CreateArtifactRefRequest) (ProjectArtifactRef, error)
	ListArtifactRefs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArtifactRef, error)
	UpdateArtifactRetention(ctx context.Context, req UpdateArtifactRetentionRequest) (ProjectArtifactRef, error)
	CreateReportRef(ctx context.Context, req CreateReportRefRequest) (ProjectReportRef, error)
	ListReportRefs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectReportRef, error)
	CreateBudgetLedgerEntry(ctx context.Context, req CreateBudgetLedgerEntryRequest) (ProjectBudgetLedgerEntry, error)
	ListBudgetLedger(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectBudgetLedgerEntry, error)
	GetBudgetSummary(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectBudgetSummary, error)
	CreateAcceptanceRecord(ctx context.Context, req CreateAcceptanceRecordRequest) (ProjectAcceptanceRecord, error)
	CreateAcceptanceRecordWithEvent(ctx context.Context, req CreateAcceptanceRecordWithEventRequest) (ProjectAcceptanceRecordWriteResult, error)
	GetLatestAcceptanceRecord(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectAcceptanceRecord, error)
	CreateArchiveSnapshot(ctx context.Context, req CreateArchiveSnapshotRequest) (ProjectArchiveSnapshot, error)
	CreateArchiveSnapshotWithEvent(ctx context.Context, req CreateArchiveSnapshotWithEventRequest) (ProjectArchiveSnapshotWriteResult, error)
	CreateArchiveSnapshotWithEventAndArchiveProject(ctx context.Context, req CreateArchiveSnapshotWithEventRequest) (ProjectArchiveSnapshotWriteResult, error)
	ListArchiveSnapshots(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArchiveSnapshot, error)
	ListConfigRevisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectConfigRevision, error)
	GetConfigRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (ProjectConfigRevision, error)
}

type ProjectTaskRuntimeBindingRepository interface {
	GetProjectTaskRunRuntimeNodeID(ctx context.Context, tenantID, projectTaskID, runID uuid.UUID) (uuid.UUID, error)
}

type ProjectTaskRunWorkProductRepository interface {
	GetProjectTaskRunWorkProducts(ctx context.Context, tenantID, runID uuid.UUID) ([]any, error)
}

type ProjectTaskWritebackRepository interface {
	CompleteProjectTaskWriteback(ctx context.Context, req CompleteProjectTaskWritebackRequest) (ProjectTaskWritebackResult, error)
	FailProjectTaskWriteback(ctx context.Context, req FailProjectTaskWritebackRequest) (ProjectTaskWritebackResult, error)
	RequestProjectTaskTransferWriteback(ctx context.Context, req RequestProjectTaskTransferWritebackRequest) (ProjectTaskTransferWritebackResult, error)
}

type ProjectTaskAttemptWritebackRepository interface {
	StartProjectTaskAttemptWriteback(ctx context.Context, req StartProjectTaskAttemptRequest) (ProjectTaskAttemptWritebackResult, error)
	RenewProjectTaskAttemptLeaseWriteback(ctx context.Context, req RenewProjectTaskAttemptLeaseRequest) (ProjectTaskAttempt, error)
	CompleteProjectTaskAttemptWriteback(ctx context.Context, req CompleteProjectTaskAttemptRequest) (ProjectTaskWritebackResult, error)
	CompleteProjectTaskAttemptResultWriteback(ctx context.Context, req CompleteProjectTaskAttemptResultWritebackRequest) (ProjectTaskWritebackResult, error)
	CompleteProjectTaskAttemptAcceptanceWriteback(ctx context.Context, req CompleteProjectTaskAttemptAcceptanceWritebackRequest) (ProjectTaskWritebackResult, error)
	CompleteProjectTaskAttemptAcceptanceResultWriteback(ctx context.Context, req CompleteProjectTaskAttemptAcceptanceResultWritebackRequest) (ProjectTaskWritebackResult, error)
	FailProjectTaskAttemptWriteback(ctx context.Context, req FailProjectTaskAttemptRequest) (ProjectTaskWritebackResult, error)
	RecoverProjectTaskAttemptFailureWriteback(ctx context.Context, req RecoverProjectTaskAttemptFailureWritebackRequest) (ProjectTaskWritebackResult, error)
	WaitHumanProjectTaskAttemptWriteback(ctx context.Context, req WaitHumanProjectTaskAttemptWritebackRequest) (ProjectTaskWritebackResult, error)
}

type CompleteProjectTaskAttemptResultWritebackRequest struct {
	Complete CompleteProjectTaskAttemptRequest
	Result   RecordProjectTaskResultRequest
}

type CompleteProjectTaskAttemptAcceptanceResultWritebackRequest struct {
	Acceptance CompleteProjectTaskAttemptAcceptanceWritebackRequest
	Result     RecordProjectTaskResultRequest
}

type ProviderEventExecutionLedgerRepository interface {
	CreateProviderSessionEventLedgerEvent(ctx context.Context, tenantID, digitalEmployeeRunID, providerSessionEventID uuid.UUID) (ExecutionLedgerEvent, error)
}

type ProjectTaskHumanWaitResolutionRepository interface {
	ResolveProjectTaskHumanWaitWriteback(ctx context.Context, req ResolveProjectTaskHumanWaitWritebackRequest) (ProjectTaskWritebackResult, error)
}

type AppendProjectEventRequest struct {
	TenantID     uuid.UUID
	ProjectID    uuid.UUID
	EventType    ProjectEventType
	ActorType    string
	ActorID      string
	ResourceType *string
	ResourceID   *string
	Summary      string
	Payload      map[string]any
}

type CreateCoordinationJobRequest struct {
	TenantID         uuid.UUID
	ProjectID        uuid.UUID
	WorkflowID       string
	TriggerEventID   *uuid.UUID
	JobType          string
	Status           string
	InputSnapshotRef map[string]any
}

type FinishCoordinationJobRequest struct {
	TenantID       uuid.UUID
	ID             uuid.UUID
	Status         string
	OutputEventIDs []any
}

type CreateRouteDecisionRequest struct {
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
}

type CreatePlanRevisionRequest struct {
	TenantID               uuid.UUID
	TeamID                 *uuid.UUID
	ProjectID              uuid.UUID
	DemandID               uuid.UUID
	CoordinationJobID      *uuid.UUID
	RouteDecisionID        *uuid.UUID
	Status                 string
	Payload                map[string]any
	PlannerProvider        *string
	PlannerModel           *string
	PlannerInputHash       *string
	PlanFingerprint        string
	ValidationErrors       []string
	ValidationWarnings     []string
	ReviewRequired         bool
	ReviewReason           *string
	SupersedeOpenRevisions bool
	SupersedeReason        *string
	CreatedEventID         *uuid.UUID
}

type ListPlanRevisionsRequest struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
	DemandID  *uuid.UUID
	Limit     int32
	Offset    int32
}

type AcceptPlanRevisionRequest struct {
	TenantID   uuid.UUID
	ProjectID  uuid.UUID
	RevisionID uuid.UUID
	AcceptedBy *uuid.UUID
}

type RejectPlanRevisionRequest struct {
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	RevisionID      uuid.UUID
	RejectedBy      *uuid.UUID
	RejectionReason *string
}

type CreateProjectTaskRequest struct {
	TenantID                  uuid.UUID
	ProjectID                 uuid.UUID
	DemandID                  *uuid.UUID
	Title                     string
	Summary                   string
	Status                    string
	AssignedDigitalEmployeeID *uuid.UUID
	RuntimeTaskID             *uuid.UUID
	DigitalEmployeeRunID      *uuid.UUID
	RiskLevel                 string
	RequiresHumanApproval     bool
	CoordinationJobID         *uuid.UUID
	RouteDecisionID           *uuid.UUID
	PlannedTaskKey            *string
	TaskKind                  *string
	StageIndex                *int32
	RevisionOfTaskID          *uuid.UUID
	AcceptedPlanRevisionID    *uuid.UUID
	DecompositionClaimKey     *string
	ExpectedOutputs           []any
	InputRequirements         map[string]any
	HandoffContract           map[string]any
	PlannerMetadata           map[string]any
	BlockedByTaskIDs          []uuid.UUID
}

type RecordProjectTaskResultRequest struct {
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
	CreatedEventID     *uuid.UUID
	DecisionRequestID  *uuid.UUID
	RevisionTaskID     *uuid.UUID
}

type ListProjectTaskResultsRequest struct {
	TenantID      uuid.UUID
	ProjectID     uuid.UUID
	ProjectTaskID uuid.UUID
	Limit         int32
	Offset        int32
}

type CreateProjectDemandSummaryRequest struct {
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
}

type CreateProjectTaskGraphRequest struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DemandID          uuid.UUID
	CoordinationJobID uuid.UUID
	RouteDecisionID   uuid.UUID
	Tasks             []ProjectTaskGraphCreateTask
}

type ProjectTaskGraphCreateTask struct {
	Key                       string
	Title                     string
	Summary                   string
	Status                    string
	AssignedDigitalEmployeeID uuid.UUID
	TaskKind                  string
	StageIndex                *int32
	RiskLevel                 string
	RequiresHumanApproval     bool
	ExpectedOutputs           []any
	InputRequirements         map[string]any
	HandoffContract           map[string]any
	PlannerMetadata           map[string]any
	BlockedByKeys             []string
}

type CreateProjectTaskGraphResult struct {
	Tasks        []ProjectTaskGraphTaskResult
	Dependencies []ProjectTaskDependency
	GraphEventID uuid.UUID
}

type CreateProjectTaskDependencyRequest struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	CoordinationJobID *uuid.UUID
	DependentTaskID   uuid.UUID
	BlockerTaskID     uuid.UUID
}

type RewireProjectTaskDependenciesRequest struct {
	TenantID         uuid.UUID
	ProjectID        uuid.UUID
	DependentTaskIDs []uuid.UUID
	OldBlockerTaskID uuid.UUID
	NewBlockerTaskID uuid.UUID
}

type ProjectTaskGraphTaskResult struct {
	ID             uuid.UUID
	PlannedTaskKey string
	StageIndex     *int32
	CreatedEventID uuid.UUID
	IsRoot         bool
}

type ProjectTaskDependencyReadiness struct {
	DependentTaskID              uuid.UUID
	BlockerTaskID                uuid.UUID
	BlockerStatus                string
	LatestTaskResultID           *uuid.UUID
	LatestResultStatus           TaskResultStatus
	LatestResultDecision         TaskResultDecision
	LatestResultValidationStatus string
	AcceptanceSatisfied          bool
}

// ProjectTaskResultAcceptedForDependencyUnlock reports whether a structured task
// result is strong enough to release downstream dependency gates.
func ProjectTaskResultAcceptedForDependencyUnlock(result ProjectTaskResult) bool {
	return result.ResultStatus == TaskResultStatusCompleted &&
		result.Decision == TaskResultDecisionCompleteAccepted &&
		strings.EqualFold(strings.TrimSpace(result.ValidationStatus), "accepted")
}

type ProjectTaskCompletionContract struct {
	ID                   uuid.UUID
	TenantID             uuid.UUID
	ProjectID            uuid.UUID
	ExpectedOutputs      []any
	HandoffContract      map[string]any
	DigitalEmployeeRunID *uuid.UUID
}

type GetProjectTaskGraphRequest struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	CoordinationJobID *uuid.UUID
	DemandID          *uuid.UUID
	Limit             int32
	Offset            int32
}

type CreateExecutionSummaryRequest struct {
	TenantID              uuid.UUID
	ProjectID             uuid.UUID
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
	TransferRequestID     *uuid.UUID
	CreatedEventID        *uuid.UUID
}

type CompleteProjectTaskWritebackRequest struct {
	Task                   ProjectTask
	Summary                CreateExecutionSummaryRequest
	Event                  AppendProjectEventRequest
	AllowedCurrentStatuses []string
}

type FailProjectTaskWritebackRequest struct {
	Task                   ProjectTask
	Event                  AppendProjectEventRequest
	AllowedCurrentStatuses []string
}

type RequestProjectTaskTransferWritebackRequest struct {
	Task                   ProjectTask
	Event                  AppendProjectEventRequest
	Transfer               CreateTransferRequestRequest
	AllowedCurrentStatuses []string
}

type RecoverProjectTaskAttemptFailureWritebackRequest struct {
	Task                  ProjectTask
	Attempt               ProjectTaskAttempt
	Failure               FailProjectTaskAttemptRequest
	AttemptTerminalStatus string
	TaskTargetStatus      string
	WaitingReason         string
	RetryAttemptID        uuid.UUID
	RetryLeaseToken       string
	RetryIdempotencyKey   string
	RetryNotBefore        *time.Time
}

type WaitHumanProjectTaskAttemptWritebackRequest struct {
	Task     ProjectTask
	Attempt  ProjectTaskAttempt
	Wait     WaitHumanProjectTaskAttemptRequest
	Decision CreateDecisionRequestRequest
}

// ProjectTaskDispatchRecoveryRepository is the narrow transaction boundary for
// recovering dispatch failures that happen before any attempt exists.
type ProjectTaskDispatchRecoveryRepository interface {
	GetProjectTaskLatestDispatchFailureEvent(ctx context.Context, tenantID, projectID, projectTaskID uuid.UUID) (ProjectEvent, error)
	CountProjectTaskDispatchFailureEvents(ctx context.Context, tenantID, projectID, projectTaskID uuid.UUID) (int64, error)
	RecoverProjectTaskDispatchFailure(ctx context.Context, req RecoverProjectTaskDispatchFailureWritebackRequest) (ProjectTaskWritebackResult, error)
	ListStaleQueuedProjectTaskAttempts(ctx context.Context, tenantID uuid.UUID, startedBefore time.Time, limit int32) ([]ProjectTaskAttempt, error)
	ListExpiredRunningProjectTaskAttempts(ctx context.Context, tenantID uuid.UUID, now time.Time, limit int32) ([]ProjectTaskAttempt, error)
}

type RecoverProjectTaskDispatchFailureWritebackRequest struct {
	TenantID       uuid.UUID
	ProjectID      uuid.UUID
	ProjectTaskID  uuid.UUID
	FailureEventID uuid.UUID
	Action         ProjectTaskRecoveryAction
}

type CompleteProjectTaskAttemptAcceptanceWritebackRequest struct {
	Task     ProjectTask
	Attempt  ProjectTaskAttempt
	Complete CompleteProjectTaskAttemptRequest
	Decision CreateDecisionRequestRequest
}

type ResolveProjectTaskHumanWaitWritebackRequest struct {
	Task                ProjectTask
	CurrentAttempt      ProjectTaskAttempt
	Resolve             ResolveProjectTaskHumanWaitRequest
	TargetStatus        string
	RetryAttemptID      uuid.UUID
	RetryLeaseToken     string
	RetryIdempotencyKey string
}

type ProjectTaskWritebackResult struct {
	Task     ProjectTask
	Event    ProjectEvent
	Summary  ExecutionSummary
	Decision DecisionRequest
}

type ProjectTaskAttemptWritebackResult struct {
	Task    ProjectTask
	Attempt ProjectTaskAttempt
	Event   ProjectEvent
}

type ProjectTaskTransferWritebackResult struct {
	Task     ProjectTask
	Event    ProjectEvent
	Transfer TransferRequest
}

type CreateTransferRequestRequest struct {
	TenantID                     uuid.UUID
	ProjectID                    uuid.UUID
	ProjectTaskID                uuid.UUID
	RequestedByDigitalEmployeeID uuid.UUID
	Reason                       string
	SuggestedEmployeeType        string
	SuggestedDigitalEmployeeIDs  []uuid.UUID
	MissingContextRefs           []any
	Status                       string
	CreatedEventID               *uuid.UUID
}

type CreateDecisionRequestRequest struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	ApprovalRequestID uuid.UUID
	CoordinationJobID *uuid.UUID
	ProjectTaskID     *uuid.UUID
	PlanRevisionID    *uuid.UUID
	TargetUserID      uuid.UUID
	DecisionType      string
	TitleSnapshot     string
	SummarySnapshot   string
	RiskLevelSnapshot string
	StatusSnapshot    string
	CreatedEventID    *uuid.UUID
}

type ResolveDecisionRequestRepositoryRequest struct {
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	ID              uuid.UUID
	StatusSnapshot  string
	ResolvedEventID *uuid.UUID
}

type CreateEvidenceRefRequest struct {
	TenantID           uuid.UUID
	ProjectID          uuid.UUID
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
	VerificationStatus EvidenceVerificationStatus
	Metadata           map[string]any
	CreatedEventID     *uuid.UUID
}

type CreateEvidenceRefWithEventRequest struct {
	Event    AppendProjectEventRequest
	Evidence CreateEvidenceRefRequest
}

type ProjectEvidenceRefWriteResult struct {
	Event    ProjectEvent
	Evidence ProjectEvidenceRef
}

type UpdateEvidenceVerificationStatusRequest struct {
	TenantID           uuid.UUID
	ProjectID          uuid.UUID
	ID                 uuid.UUID
	VerificationStatus EvidenceVerificationStatus
	Metadata           map[string]any
}

type UpdateEvidenceVerificationStatusWithEventRequest struct {
	Event    AppendProjectEventRequest
	Evidence UpdateEvidenceVerificationStatusRequest
}

type CreateArtifactRefRequest struct {
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	ProjectTaskID   *uuid.UUID
	ArtifactID      *uuid.UUID
	ArtifactType    string
	Title           string
	ObjectRef       string
	ContentType     string
	SizeBytes       *int64
	Checksum        string
	RetentionStatus string
	RetentionHoldID *uuid.UUID
	Metadata        map[string]any
	CreatedEventID  *uuid.UUID
}

type UpdateArtifactRetentionRequest struct {
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	ID              uuid.UUID
	RetentionStatus string
	RetentionHoldID *uuid.UUID
}

type CreateReportRefRequest struct {
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	ReportType      string
	Title           string
	Summary         string
	ObjectRef       string
	Format          string
	GeneratedByType string
	GeneratedByID   *uuid.UUID
	CreatedEventID  *uuid.UUID
}

type CreateBudgetLedgerEntryRequest struct {
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
	Reason            string
	CreatedEventID    *uuid.UUID
}

type CreateAcceptanceRecordRequest struct {
	TenantID         uuid.UUID
	ProjectID        uuid.UUID
	AcceptedByUserID uuid.UUID
	Status           string
	Conclusion       string
	Summary          string
	EvidenceRefIDs   []uuid.UUID
	ReportRefIDs     []uuid.UUID
	UnresolvedRisks  []any
	CreatedEventID   *uuid.UUID
}

type CreateAcceptanceRecordWithEventRequest struct {
	Event      AppendProjectEventRequest
	Acceptance CreateAcceptanceRecordRequest
}

type ProjectAcceptanceRecordWriteResult struct {
	Event      ProjectEvent
	Acceptance ProjectAcceptanceRecord
}

type CreateArchiveSnapshotRequest struct {
	TenantID             uuid.UUID
	ProjectID            uuid.UUID
	SnapshotType         string
	Status               string
	ObjectRef            string
	Summary              string
	IncludedCounts       map[string]any
	RetainedArtifactIDs  []uuid.UUID
	RetentionLockEventID *uuid.UUID
	CreatedByUserID      uuid.UUID
	CreatedEventID       *uuid.UUID
}

type CreateArchiveSnapshotWithEventRequest struct {
	Event    AppendProjectEventRequest
	Snapshot CreateArchiveSnapshotRequest
}

type ProjectArchiveSnapshotWriteResult struct {
	Event    ProjectEvent
	Snapshot ProjectArchiveSnapshot
}
