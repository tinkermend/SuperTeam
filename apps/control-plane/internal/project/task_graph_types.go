package project

import "github.com/google/uuid"

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
}

type ProjectTaskGraphNode struct {
	Task ProjectTask
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
