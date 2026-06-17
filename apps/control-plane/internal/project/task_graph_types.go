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
}

type ProjectTaskGraphNode struct {
	Task           ProjectTask
	StatusReason   string
	UpdatedAt      *time.Time
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
	Status            string
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
