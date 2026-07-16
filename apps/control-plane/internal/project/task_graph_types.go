package project

import (
	"time"

	"github.com/google/uuid"
)

type ProjectTaskDependency struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	CoordinationJobID *uuid.UUID
	DependentTaskID   uuid.UUID
	BlockerTaskID     uuid.UUID
}

type ProjectTaskGraph struct {
	Nodes              []ProjectTaskGraphNode
	Edges              []ProjectTaskGraphEdge
	Employees          []ProjectTaskGraphEmployee
	Runs               []ProjectTaskGraphRun
	ExecutionSummaries []ExecutionSummary
	RecentEvents       []ProjectEvent
	DecisionRequests   []DecisionRequest
	StageSummaries     []ProjectTaskGraphStageSummary
	BlockingFacts      []ProjectTaskGraphBlockingFact
}

type ProjectTaskGraphNode struct {
	Task           ProjectTask
	StatusReason   string
	UpdatedAt      *time.Time
	StartedAt      *time.Time
	FinishedAt     *time.Time
	CurrentBlocker *WorkflowInstanceCurrentBlocker
}

type ProjectTaskGraphEdge struct {
	DependentTaskID   uuid.UUID
	BlockerTaskID     uuid.UUID
	CoordinationJobID *uuid.UUID
	EdgeStatus        string
}

type ProjectTaskGraphEmployee struct {
	DigitalEmployeeID uuid.UUID
	DisplayName       string
	ProjectRole       ProjectRole
	EmployeeRole      string
	AvatarAsset       *ProjectTaskGraphEmployeeAvatarAsset
	Status            string
}

type ProjectTaskGraphEmployeeAvatarAsset struct {
	ID           string
	Label        string
	ImageURL     string
	ThumbnailURL string
}

type ProjectTaskGraphRun struct {
	ProjectTaskID        uuid.UUID
	DigitalEmployeeRunID *uuid.UUID
	RuntimeTaskID        *uuid.UUID
	RuntimeNodeID        *uuid.UUID
	RuntimeNodeSummary   string
	Status               string
	ProviderType         string
}

type ProjectTaskGraphStageSummary struct {
	StageIndex        int32
	Title             string
	TotalNodes        int32
	CompletedNodes    int32
	RunningNodes      int32
	WaitingHumanNodes int32
	BlockedNodes      int32
}

type ProjectTaskGraphBlockingFact struct {
	ReasonCode        string
	Message           string
	ResourceType      string
	ResourceID        string
	RecommendedAction string
	CreatedAt         time.Time
	// Gap is the structured staffing gap (RejectDemandPlanning's PlanningGap,
	// projectcoordination package) carried by a coordination.blocked event's "gap"
	// payload key. Nil for any non-structural diagnosis or event predating the
	// planning_gap decision flow — see projectTaskGraphBlockingFactFromEvent.
	Gap *ProjectTaskGraphBlockingFactGap
	// DecisionRequestID is the pending planning_gap DecisionRequest's id, carried by
	// the same blocked event's "decision_request_id" payload key (RejectDemandPlanning).
	// Empty when the event predates that field or no approval sink was wired.
	DecisionRequestID string
}

// ProjectTaskGraphBlockingFactGap mirrors projectcoordination.PlanningGap's shape
// for the web: the structural constraint a demand's route could not satisfy, plus
// the pool state and resolution options a human sees to act on it.
type ProjectTaskGraphBlockingFactGap struct {
	ConstraintKind       string
	Roles                []string
	RequiredCapabilities []string
	ActiveExecutorCount  int
	Options              []string
}
