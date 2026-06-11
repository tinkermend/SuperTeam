package project

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	CreateProject(ctx context.Context, req CreateProjectRequest, projectID uuid.UUID, workflowID string) (Project, error)
	GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (Project, error)
	ListProjects(ctx context.Context, req ListProjectsRequest) ([]Project, error)
	UpdateProjectConfig(ctx context.Context, req UpdateProjectConfigRequest) (Project, error)
	ArchiveProject(ctx context.Context, tenantID, projectID uuid.UUID) (Project, error)
	ReplaceProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID, members []ProjectMemberInput) ([]ProjectMember, error)
	ListProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectMember, error)
	ListProjectTasks(ctx context.Context, tenantID, projectID uuid.UUID, status *string, limit, offset int32) ([]ProjectTask, error)
	AppendProjectEvent(ctx context.Context, event AppendProjectEventRequest) (ProjectEvent, error)
	GetProjectEvent(ctx context.Context, tenantID, projectID, eventID uuid.UUID) (ProjectEvent, error)
	ListProjectEvents(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectEvent, error)
	CreateProjectDemand(ctx context.Context, req SubmitProjectDemandRequest, status ProjectDemandStatus, createdEventID *uuid.UUID) (ProjectDemand, error)
	ListProjectDemands(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectDemand, error)
	CreateConfigRevision(ctx context.Context, req UpdateProjectConfigRequest, project Project, eventID uuid.UUID) (ProjectConfigRevision, error)
	GetProjectDemand(ctx context.Context, tenantID, demandID uuid.UUID) (ProjectDemand, error)
	GetProjectTask(ctx context.Context, tenantID, projectTaskID uuid.UUID) (ProjectTask, error)
	CreateCoordinationJob(ctx context.Context, req CreateCoordinationJobRequest) (CoordinationJob, error)
	FinishCoordinationJob(ctx context.Context, req FinishCoordinationJobRequest) (CoordinationJob, error)
	ListCoordinationJobs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]CoordinationJob, error)
	CreateRouteDecision(ctx context.Context, req CreateRouteDecisionRequest) (RouteDecision, error)
	ListRouteDecisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]RouteDecision, error)
	CreateProjectTask(ctx context.Context, req CreateProjectTaskRequest) (ProjectTask, error)
	UpdateProjectTaskStatus(ctx context.Context, tenantID, projectTaskID uuid.UUID, status string, eventID *uuid.UUID, currentStatuses []string) (ProjectTask, error)
	AssignProjectTask(ctx context.Context, tenantID, projectTaskID uuid.UUID, status string, assignedDigitalEmployeeID, eventID *uuid.UUID) (ProjectTask, error)
	CreateExecutionSummary(ctx context.Context, req CreateExecutionSummaryRequest) (ExecutionSummary, error)
	ListExecutionSummaries(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ExecutionSummary, error)
	CreateTransferRequest(ctx context.Context, req CreateTransferRequestRequest) (TransferRequest, error)
	ListTransferRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]TransferRequest, error)
	CreateDecisionRequest(ctx context.Context, req CreateDecisionRequestRequest) (DecisionRequest, error)
	GetDecisionRequest(ctx context.Context, tenantID, projectID, decisionRequestID uuid.UUID) (DecisionRequest, error)
	ResolveDecisionRequest(ctx context.Context, req ResolveDecisionRequestRepositoryRequest) (DecisionRequest, error)
	ListDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]DecisionRequest, error)
	CreateEvidenceRef(ctx context.Context, req CreateEvidenceRefRequest) (ProjectEvidenceRef, error)
	ListEvidenceRefs(ctx context.Context, tenantID, projectID uuid.UUID, status *EvidenceVerificationStatus, limit, offset int32) ([]ProjectEvidenceRef, error)
	UpdateEvidenceVerificationStatus(ctx context.Context, req UpdateEvidenceVerificationStatusRequest) (ProjectEvidenceRef, error)
	CreateArtifactRef(ctx context.Context, req CreateArtifactRefRequest) (ProjectArtifactRef, error)
	ListArtifactRefs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArtifactRef, error)
	UpdateArtifactRetention(ctx context.Context, req UpdateArtifactRetentionRequest) (ProjectArtifactRef, error)
	CreateReportRef(ctx context.Context, req CreateReportRefRequest) (ProjectReportRef, error)
	ListReportRefs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectReportRef, error)
	CreateBudgetLedgerEntry(ctx context.Context, req CreateBudgetLedgerEntryRequest) (ProjectBudgetLedgerEntry, error)
	ListBudgetLedger(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectBudgetLedgerEntry, error)
	GetBudgetSummary(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectBudgetSummary, error)
	CreateAcceptanceRecord(ctx context.Context, req CreateAcceptanceRecordRequest) (ProjectAcceptanceRecord, error)
	GetLatestAcceptanceRecord(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectAcceptanceRecord, error)
	CreateArchiveSnapshot(ctx context.Context, req CreateArchiveSnapshotRequest) (ProjectArchiveSnapshot, error)
	ListArchiveSnapshots(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArchiveSnapshot, error)
	ListConfigRevisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectConfigRevision, error)
	GetConfigRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (ProjectConfigRevision, error)
}

type ProjectTaskRuntimeBindingRepository interface {
	GetProjectTaskRunRuntimeNodeID(ctx context.Context, tenantID, projectTaskID, runID uuid.UUID) (uuid.UUID, error)
}

type ProjectTaskWritebackRepository interface {
	CompleteProjectTaskWriteback(ctx context.Context, req CompleteProjectTaskWritebackRequest) (ProjectTaskWritebackResult, error)
	FailProjectTaskWriteback(ctx context.Context, req FailProjectTaskWritebackRequest) (ProjectTaskWritebackResult, error)
	RequestProjectTaskTransferWriteback(ctx context.Context, req RequestProjectTaskTransferWritebackRequest) (ProjectTaskTransferWritebackResult, error)
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

type ProjectTaskWritebackResult struct {
	Task    ProjectTask
	Event   ProjectEvent
	Summary ExecutionSummary
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

type UpdateEvidenceVerificationStatusRequest struct {
	TenantID           uuid.UUID
	ProjectID          uuid.UUID
	ID                 uuid.UUID
	VerificationStatus EvidenceVerificationStatus
	Metadata           map[string]any
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
