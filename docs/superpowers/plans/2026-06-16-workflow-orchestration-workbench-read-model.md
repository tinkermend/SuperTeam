# Workflow Orchestration Workbench Read Model Implementation Plan

> 复核状态：对应spec状态为UNCERTAIN, plan仍为待办

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the Workflow Orchestration experience as a workflow-instance card entrance plus a read-only task graph workbench, backed by enriched Control Plane read models.

**Architecture:** Keep the existing Control Plane scheduling and write paths intact. Extend the current `workflow-instances` and `task-graph` read models with optional display fields, then render `/workflows` as workflow-level cards and `/workflows/$demandId` as a summary bar, DAG canvas, and right-side inspector. All SLA, priority, and risk fields are optional and hidden when the backend has no real fact source.

**Tech Stack:** Go 1.25, PostgreSQL/sqlc, chi/net/http, OpenAPI, pnpm, React, TanStack Query/Router, `@xyflow/react`, Vitest browser tests, SuperTeam design tokens.

---

## Source Spec

Implement against:

- `docs/superpowers/specs/2026-06-16-workflow-orchestration-workbench-read-model-design.md`

Carry forward these hard boundaries:

- No new write APIs.
- No workflow graph write actions.
- `/workflows` cards represent workflow instances, not single tasks.
- `/workflows/$demandId` owns task-level DAG and task-level Inspector details.
- SLA, priority, and risk are optional. Missing facts are not replaced with mock values.
- Inspector is read-only and can link to existing pages.

## File Structure

Modify backend domain and service:

- `apps/control-plane/internal/project/types.go`
  Add workflow summary optional fields, progress counts, task graph stage summaries, and graph node auxiliary fields.
- `apps/control-plane/internal/project/task_graph_types.go`
  Add stage summaries and node helper fields.
- `apps/control-plane/internal/project/service.go`
  Normalize optional slices, derive workflow status priority, sort workflow summaries, and compute graph stage summaries when repository output omits them.
- `apps/control-plane/internal/project/repository.go`
  Keep repository interface names stable; no new write repository methods.
- `apps/control-plane/internal/project/pg_repository.go`
  Map the enriched sqlc rows into domain objects and add task graph node helper values.
- `apps/control-plane/internal/storage/queries/project.sql`
  Extend `ListWorkflowInstances` and task graph read support without migrations.
- `apps/control-plane/internal/project/handler.go`
  Emit optional JSON fields with `omitempty`.
- `contracts/control-plane/openapi.yaml`
  Add optional schemas and fields.

Modify backend tests:

- `apps/control-plane/internal/project/service_test.go`
- `apps/control-plane/internal/project/pg_repository_test.go`
- `apps/control-plane/internal/project/handler_test.go`
- `apps/control-plane/internal/api/project_routes_test.go`

Modify frontend API and workflow feature:

- `apps/web/src/lib/api/projects.ts`
  Add optional workflow summary and task graph types.
- `apps/web/src/features/workflows/index.tsx`
  Split entrance and workbench behavior.
- `apps/web/src/features/workflows/workflow-status.ts`
  Add display helpers for optional status badges and counters.
- `apps/web/src/features/workflows/workflow-graph-adapter.ts`
  Preserve deterministic graph layout and expose stage metadata to UI.
- `apps/web/src/features/workflows/components/workflow-shell.tsx`
- `apps/web/src/features/workflows/components/workflow-instance-list.tsx`
  Keep the component file available for now, but stop rendering it from the main `/workflows` path after the entrance card grid is introduced.
- `apps/web/src/features/workflows/components/workflow-detail.tsx`
  Convert to workbench layout.
- `apps/web/src/features/workflows/components/workflow-graph-canvas.tsx`
- `apps/web/src/features/workflows/components/workflow-task-node.tsx`
- `apps/web/src/features/workflows/components/workflow-node-inspector.tsx`

Modify frontend tests:

- `apps/web/src/features/workflows/index.test.tsx`
- `apps/web/src/features/workflows/workflow-graph-adapter.test.ts`

Generated files:

- Run `make -C apps/control-plane generate-sqlc` after SQL changes.
- Run `corepack pnpm generate:control-plane` after OpenAPI changes.
- Run `corepack pnpm verify:contracts` after generation.

## Task 1: Backend Domain Types And Service Contract

**Files:**

- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/task_graph_types.go`
- Modify: `apps/control-plane/internal/project/service.go`
- Test: `apps/control-plane/internal/project/service_test.go`

- [ ] **Step 1: Add failing service tests for enriched workflow summaries**

Add this test after `TestListWorkflowInstancesRejectsMissingActor` in `apps/control-plane/internal/project/service_test.go`:

```go
func TestListWorkflowInstancesKeepsOptionalReadModelFieldsAndSortsAttentionFirst(t *testing.T) {
	tenantID := uuid.New()
	actorID := uuid.New()
	waitingDemandID := uuid.New()
	runningDemandID := uuid.New()
	completedDemandID := uuid.New()
	dueAt := time.Now().UTC().Add(15 * time.Minute)
	remaining := int32(900)
	repo := &workflowInstanceServiceRepository{
		memoryRepository: newMemoryRepository(),
		items: []WorkflowInstanceSummary{
			{
				DemandID:          completedDemandID,
				ProjectID:         uuid.New(),
				ProjectName:       "归档项目",
				Title:             "复盘归档",
				SubmittedByUserID: actorID,
				Status:            WorkflowInstanceStatusCompleted,
				UpdatedAt:         time.Now().UTC().Add(-2 * time.Minute),
				Progress: WorkflowInstanceProgress{
					TotalNodes:     2,
					CompletedNodes: 2,
				},
			},
			{
				DemandID:          runningDemandID,
				ProjectID:         uuid.New(),
				ProjectName:       "运行项目",
				Title:             "服务巡检",
				SubmittedByUserID: actorID,
				Status:            WorkflowInstanceStatusRunning,
				UpdatedAt:         time.Now().UTC().Add(-1 * time.Minute),
				Progress: WorkflowInstanceProgress{
					TotalNodes:   3,
					RunningNodes: 1,
				},
			},
			{
				DemandID:          waitingDemandID,
				ProjectID:         uuid.New(),
				ProjectName:       "支付项目",
				Title:             "支付成功率下降",
				SubmittedByUserID: actorID,
				Status:            WorkflowInstanceStatusUnknown,
				UpdatedAt:         time.Now().UTC().Add(-3 * time.Minute),
				Progress: WorkflowInstanceProgress{
					TotalNodes:        5,
					CompletedNodes:    2,
					RunningNodes:      1,
					BlockedNodes:      1,
					WaitingHumanNodes: 1,
					PlannedNodes:      1,
					FailedNodes:       0,
					CancelledNodes:    0,
				},
				CurrentBlocker: &WorkflowInstanceCurrentBlocker{
					Type:  "decision_request",
					Title: "等待人工审批回滚方案",
				},
				Priority: &WorkflowInstancePriority{
					Value:  "p1",
					Label:  "P1",
					Source: "source_refs.priority",
				},
				Risk: &WorkflowInstanceRisk{
					Level:  "high",
					Label:  "高风险",
					Source: "project_tasks.risk_level",
				},
				SLA: &WorkflowInstanceSLA{
					DueAt:            &dueAt,
					RemainingSeconds: &remaining,
					Breached:         false,
					Label:            "剩余 15 分钟",
					Source:           "source_refs.sla_due_at",
				},
				RecentEvent: &WorkflowInstanceRecentEvent{
					EventType:  string(ProjectEventDecisionRequested),
					Summary:    "已创建恢复决策请求",
					OccurredAt: time.Now().UTC().Add(-30 * time.Second),
				},
			},
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	items, err := service.ListWorkflowInstances(context.Background(), ListWorkflowInstancesRequest{
		TenantID:    tenantID,
		ActorUserID: actorID,
	})
	if err != nil {
		t.Fatalf("list workflow instances: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected three workflow instances, got %#v", items)
	}
	if items[0].DemandID != waitingDemandID {
		t.Fatalf("expected waiting-human workflow first, got %#v", items)
	}
	if items[0].Status != WorkflowInstanceStatusWaitingHuman {
		t.Fatalf("expected waiting_human status, got %s", items[0].Status)
	}
	if items[0].Priority == nil || items[0].Priority.Label != "P1" {
		t.Fatalf("expected priority field to survive service normalization: %#v", items[0].Priority)
	}
	if items[0].Risk == nil || items[0].Risk.Level != "high" {
		t.Fatalf("expected risk field to survive service normalization: %#v", items[0].Risk)
	}
	if items[0].SLA == nil || items[0].SLA.RemainingSeconds == nil || *items[0].SLA.RemainingSeconds != 900 {
		t.Fatalf("expected SLA field to survive service normalization: %#v", items[0].SLA)
	}
	if items[0].RecentEvent == nil || items[0].RecentEvent.EventType != string(ProjectEventDecisionRequested) {
		t.Fatalf("expected recent event field to survive service normalization: %#v", items[0].RecentEvent)
	}
}
```

- [ ] **Step 2: Add failing service test for task graph stage summaries**

Add this test near `TestGetProjectTaskGraphRequiresFilterAndDoesNotApplyHiddenLimit`:

```go
func TestGetProjectTaskGraphBuildsStageSummariesWhenRepositoryOmitsThem(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	stageOne := int32(1)
	stageTwo := int32(2)
	repo := &taskGraphLimitRepository{
		memoryRepository: newMemoryRepository(),
		graph: ProjectTaskGraph{
			Nodes: []ProjectTaskGraphNode{
				{Task: ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, Title: "入口", Status: "completed", StageIndex: &stageOne}},
				{Task: ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, Title: "巡检", Status: "running", StageIndex: &stageTwo}},
				{Task: ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, Title: "审批", Status: "waiting_human", StageIndex: &stageTwo, RequiresHumanApproval: true}},
			},
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	graph, err := service.GetProjectTaskGraph(context.Background(), GetProjectTaskGraphRequest{
		TenantID: tenantID, ProjectID: projectID, DemandID: &demandID,
	})
	if err != nil {
		t.Fatalf("get graph: %v", err)
	}
	if len(graph.StageSummaries) != 2 {
		t.Fatalf("expected two stage summaries, got %#v", graph.StageSummaries)
	}
	if graph.StageSummaries[0].StageIndex != 1 || graph.StageSummaries[0].CompletedNodes != 1 {
		t.Fatalf("unexpected first stage summary: %#v", graph.StageSummaries[0])
	}
	if graph.StageSummaries[1].StageIndex != 2 || graph.StageSummaries[1].RunningNodes != 1 || graph.StageSummaries[1].WaitingHumanNodes != 1 {
		t.Fatalf("unexpected second stage summary: %#v", graph.StageSummaries[1])
	}
}
```

Update `taskGraphLimitRepository` in the same file to support the injected graph:

```go
type taskGraphLimitRepository struct {
	*memoryRepository
	calls   int
	lastReq GetProjectTaskGraphRequest
	graph   ProjectTaskGraph
}

func (r *taskGraphLimitRepository) GetProjectTaskGraph(ctx context.Context, req GetProjectTaskGraphRequest) (ProjectTaskGraph, error) {
	r.calls++
	r.lastReq = req
	if r.graph.Nodes != nil {
		return r.graph, nil
	}
	count := 55
	if req.Limit > 0 && int(req.Limit) < count {
		count = int(req.Limit)
	}
	nodes := make([]ProjectTaskGraphNode, 0, count)
	for i := 0; i < count; i++ {
		nodes = append(nodes, ProjectTaskGraphNode{
			Task: ProjectTask{
				ID:        uuid.New(),
				TenantID:  req.TenantID,
				ProjectID: req.ProjectID,
				Title:     fmt.Sprintf("graph task %02d", i+1),
				Status:    "planned",
			},
		})
	}
	return ProjectTaskGraph{
		Nodes:              nodes,
		Edges:              []ProjectTaskGraphEdge{},
		Employees:          []ProjectTaskGraphEmployee{},
		Runs:               []ProjectTaskGraphRun{},
		ExecutionSummaries: []ExecutionSummary{},
		RecentEvents:       []ProjectEvent{},
		DecisionRequests:   []DecisionRequest{},
		StageSummaries:     []ProjectTaskGraphStageSummary{},
	}, nil
}
```

- [ ] **Step 3: Run service tests and verify RED**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestListWorkflowInstancesKeepsOptionalReadModelFieldsAndSortsAttentionFirst|TestGetProjectTaskGraphBuildsStageSummariesWhenRepositoryOmitsThem' -count=1
```

Expected: FAIL with undefined types or fields such as `WorkflowInstancePriority`, `WorkflowInstanceSLA`, and `StageSummaries`.

- [ ] **Step 4: Add domain types**

In `apps/control-plane/internal/project/types.go`, add the new domain structs after `WorkflowInstanceCurrentBlocker`:

```go
type WorkflowInstancePriority struct {
	Value  string
	Label  string
	Source string
}

type WorkflowInstanceRisk struct {
	Level  string
	Label  string
	Source string
}

type WorkflowInstanceSLA struct {
	DueAt            *time.Time
	RemainingSeconds *int32
	Breached         bool
	Label            string
	Source           string
}

type WorkflowInstanceRecentEvent struct {
	EventType  string
	Summary    string
	OccurredAt time.Time
}
```

Extend `WorkflowInstanceProgress`:

```go
type WorkflowInstanceProgress struct {
	TotalNodes        int32
	CompletedNodes    int32
	RunningNodes      int32
	BlockedNodes      int32
	WaitingHumanNodes int32
	PlannedNodes      int32
	FailedNodes       int32
	CancelledNodes    int32
}
```

Extend `WorkflowInstanceSummary`:

```go
type WorkflowInstanceSummary struct {
	DemandID                  uuid.UUID
	ProjectID                 uuid.UUID
	ProjectName               string
	Title                     string
	SubmittedByUserID         uuid.UUID
	SubmittedByDisplayName    string
	Status                    WorkflowInstanceStatus
	StatusReason              string
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
	SelectedCoordinationJobID *uuid.UUID
	Progress                  WorkflowInstanceProgress
	CurrentBlocker            *WorkflowInstanceCurrentBlocker
	Priority                  *WorkflowInstancePriority
	Risk                      *WorkflowInstanceRisk
	SLA                       *WorkflowInstanceSLA
	RecentEvent               *WorkflowInstanceRecentEvent
}
```

- [ ] **Step 5: Add task graph stage types**

In `apps/control-plane/internal/project/task_graph_types.go`, update the graph structs:

```go
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

type ProjectTaskGraphStageSummary struct {
	StageIndex        int32
	Title             string
	TotalNodes        int32
	CompletedNodes    int32
	RunningNodes      int32
	WaitingHumanNodes int32
	BlockedNodes      int32
}
```

- [ ] **Step 6: Implement service normalization and sorting helpers**

In `apps/control-plane/internal/project/service.go`, update `ListWorkflowInstances` after the repository call:

```go
for i := range items {
	items[i].Status = normalizeWorkflowInstanceStatus(items[i])
}
sort.SliceStable(items, func(i, j int) bool {
	leftRank := workflowInstanceAttentionRank(items[i].Status)
	rightRank := workflowInstanceAttentionRank(items[j].Status)
	if leftRank != rightRank {
		return leftRank < rightRank
	}
	return items[i].UpdatedAt.After(items[j].UpdatedAt)
})
```

Add the helper:

```go
func workflowInstanceAttentionRank(status WorkflowInstanceStatus) int {
	switch status {
	case WorkflowInstanceStatusWaitingHuman:
		return 0
	case WorkflowInstanceStatusFailed:
		return 1
	case WorkflowInstanceStatusRunning:
		return 2
	case WorkflowInstanceStatusPlanning:
		return 3
	case WorkflowInstanceStatusUnknown:
		return 4
	case WorkflowInstanceStatusCompleted:
		return 5
	case WorkflowInstanceStatusCancelled:
		return 6
	default:
		return 7
	}
}
```

Update `normalizeProjectTaskGraph` to initialize stage summaries:

```go
if graph.StageSummaries == nil {
	graph.StageSummaries = buildProjectTaskGraphStageSummaries(graph.Nodes)
}
```

Add helper functions:

```go
func buildProjectTaskGraphStageSummaries(nodes []ProjectTaskGraphNode) []ProjectTaskGraphStageSummary {
	type mutableSummary struct {
		summary ProjectTaskGraphStageSummary
	}
	byStage := map[int32]*mutableSummary{}
	for _, node := range nodes {
		stage := int32(-1)
		if node.Task.StageIndex != nil {
			stage = *node.Task.StageIndex
		}
		entry := byStage[stage]
		if entry == nil {
			title := "未分阶段"
			if stage >= 0 {
				title = fmt.Sprintf("第 %d 阶段", stage)
			}
			entry = &mutableSummary{summary: ProjectTaskGraphStageSummary{StageIndex: stage, Title: title}}
			byStage[stage] = entry
		}
		entry.summary.TotalNodes++
		switch normalizeTaskStatusForSummary(node.Task.Status) {
		case "completed":
			entry.summary.CompletedNodes++
		case "running":
			entry.summary.RunningNodes++
		case "waiting_human":
			entry.summary.WaitingHumanNodes++
		case "blocked":
			entry.summary.BlockedNodes++
		}
	}
	stages := make([]int, 0, len(byStage))
	for stage := range byStage {
		stages = append(stages, int(stage))
	}
	sort.Ints(stages)
	result := make([]ProjectTaskGraphStageSummary, 0, len(stages))
	for _, stage := range stages {
		result = append(result, byStage[int32(stage)].summary)
	}
	return result
}

func normalizeTaskStatusForSummary(status string) string {
	switch strings.ToLower(status) {
	case "completed", "done", "success":
		return "completed"
	case "assigned", "running", "in_progress":
		return "running"
	case "waiting_human", "pending_review":
		return "waiting_human"
	case "blocked":
		return "blocked"
	default:
		return "other"
	}
}
```

Add `sort` to the existing import block in `apps/control-plane/internal/project/service.go`:

```go
import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)
```

- [ ] **Step 7: Run service tests and verify GREEN**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestListWorkflowInstances|TestGetProjectTaskGraph' -count=1
```

Expected: PASS for the service-level workflow and graph tests.

- [ ] **Step 8: Commit backend domain and service contract**

Run:

```bash
git add apps/control-plane/internal/project/types.go apps/control-plane/internal/project/task_graph_types.go apps/control-plane/internal/project/service.go apps/control-plane/internal/project/service_test.go
git commit -m "feat: extend workflow read model domain"
```

## Task 2: Backend Repository Query And Handler JSON

**Files:**

- Modify: `apps/control-plane/internal/storage/queries/project.sql`
- Modify generated: `apps/control-plane/internal/storage/queries/project.sql.go`
- Modify generated: `apps/control-plane/internal/storage/queries/querier.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/handler.go`
- Test: `apps/control-plane/internal/project/pg_repository_test.go`
- Test: `apps/control-plane/internal/project/handler_test.go`

- [ ] **Step 1: Add repository integration test for optional workflow fields**

Extend `TestListWorkflowInstancesFiltersVisibleDemandsAndSortsRunningFirst` in `apps/control-plane/internal/project/pg_repository_test.go` after the existing `CreateProjectTaskGraph` call:

```go
_, err = repo.AppendProjectEvent(ctx, AppendProjectEventRequest{
	TenantID:     tenantID,
	ProjectID:    visibleProjectID,
	EventType:    ProjectEventDecisionRequested,
	ActorType:    "project_coordinator",
	ActorID:      "project-coordinator:" + visibleProjectID.String(),
	ResourceType: strPtr("project_task"),
	ResourceID:   strPtr(visibleDemandID.String()),
	Summary:      strPtr("已创建恢复决策请求"),
	Payload: map[string]any{
		"demand_id": visibleDemandID.String(),
	},
})
require.NoError(t, err)
```

Add expectations after the existing assertions:

```go
require.NotNil(t, items[0].Risk)
require.Equal(t, "medium", items[0].Risk.Level)
require.Equal(t, "project_tasks.risk_level", items[0].Risk.Source)
require.Equal(t, int32(0), items[0].Progress.FailedNodes)
require.Equal(t, int32(0), items[0].Progress.CancelledNodes)
require.NotNil(t, items[0].RecentEvent)
require.Equal(t, string(ProjectEventDecisionRequested), items[0].RecentEvent.EventType)
require.Equal(t, "已创建恢复决策请求", items[0].RecentEvent.Summary)
```

- [ ] **Step 2: Add handler test for optional JSON fields**

In `TestListWorkflowInstancesReturnsSummaries` in `apps/control-plane/internal/project/handler_test.go`, set optional fields on the fixture:

```go
remaining := int32(600)
dueAt := time.Now().UTC().Add(10 * time.Minute)
service := &handlerTestService{
	workflowInstances: []WorkflowInstanceSummary{{
		DemandID:                  demandID,
		ProjectID:                 projectID,
		ProjectName:               "生产巡检",
		Title:                     "支付成功率下降",
		SubmittedByUserID:         actorID,
		SubmittedByDisplayName:    "张晓明",
		Status:                    WorkflowInstanceStatusRunning,
		StatusReason:              "任务执行中",
		CreatedAt:                 time.Now().UTC(),
		UpdatedAt:                 time.Now().UTC(),
		SelectedCoordinationJobID: &jobID,
		Progress: WorkflowInstanceProgress{
			TotalNodes:   2,
			RunningNodes: 1,
			PlannedNodes: 1,
		},
		CurrentBlocker: &WorkflowInstanceCurrentBlocker{
			Type:  "task",
			Title: "等待数据库巡检",
		},
		Priority: &WorkflowInstancePriority{Value: "p1", Label: "P1", Source: "source_refs.priority"},
		Risk:     &WorkflowInstanceRisk{Level: "high", Label: "高风险", Source: "project_tasks.risk_level"},
		SLA:      &WorkflowInstanceSLA{DueAt: &dueAt, RemainingSeconds: &remaining, Breached: false, Label: "剩余 10 分钟", Source: "source_refs.sla_due_at"},
		RecentEvent: &WorkflowInstanceRecentEvent{
			EventType:  string(ProjectEventDecisionRequested),
			Summary:    "已创建恢复决策请求",
			OccurredAt: time.Now().UTC(),
		},
	}},
}
```

Add response expectations:

```go
if body[0]["priority"].(map[string]any)["label"] != "P1" {
	t.Fatalf("expected priority in response: %#v", body[0])
}
if body[0]["risk"].(map[string]any)["level"] != "high" {
	t.Fatalf("expected risk in response: %#v", body[0])
}
if body[0]["sla"].(map[string]any)["label"] != "剩余 10 分钟" {
	t.Fatalf("expected sla in response: %#v", body[0])
}
if body[0]["recent_event"].(map[string]any)["event_type"] != string(ProjectEventDecisionRequested) {
	t.Fatalf("expected recent event in response: %#v", body[0])
}
if progress["planned_nodes"].(float64) != 1 {
	t.Fatalf("expected planned_nodes in progress: %#v", progress)
}
```

- [ ] **Step 3: Add handler test for task graph stage and node fields**

In `TestGetProjectTaskGraphReturnsNodesEdgesAndDecisions`, set fields in the service graph:

```go
service := &handlerTestService{
	taskGraph: ProjectTaskGraph{
		Nodes: []ProjectTaskGraphNode{{
			Task: ProjectTask{
				ID:                        taskID,
				TenantID:                  tenantID,
				ProjectID:                 projectID,
				DemandID:                  &demandID,
				Title:                     "分析需求",
				Summary:                   strPtr("拆解任务图"),
				Status:                    "blocked",
				AssignedDigitalEmployeeID: &employeeID,
				RiskLevel:                 strPtr("medium"),
				RequiresHumanApproval:     true,
				CoordinationJobID:         &jobID,
				RouteDecisionID:           &routeID,
				PlannedTaskKey:            strPtr("t2"),
				TaskKind:                  strPtr("analysis"),
				StageIndex:                &stageIndex,
				ExpectedOutputs:           []any{"execution_summary"},
				InputRequirements:         map[string]any{"scope": "demand"},
				HandoffContract:           map[string]any{"required_refs": []any{"evidence"}},
				PlannerMetadata:           map[string]any{"provider": "deepseek"},
				UpdatedAt:                 time.Now().UTC(),
			},
			StatusReason: "等待上游任务完成",
			CurrentBlocker: &WorkflowInstanceCurrentBlocker{
				Type:       "project_task",
				Title:      "等待数据库巡检",
				ResourceID: &blockerID,
			},
		}},
		StageSummaries: []ProjectTaskGraphStageSummary{{
			StageIndex:     1,
			Title:          "第 1 阶段",
			TotalNodes:     1,
			BlockedNodes:   1,
			RunningNodes:   0,
			CompletedNodes: 0,
		}},
	}
}
```

Add response expectations after reading `node`:

```go
if node["status_reason"] != "等待上游任务完成" {
	t.Fatalf("expected status reason on graph node: %#v", node)
}
if node["current_blocker"].(map[string]any)["title"] != "等待数据库巡检" {
	t.Fatalf("expected current blocker on graph node: %#v", node)
}
stageSummaries := body["stage_summaries"].([]any)
if len(stageSummaries) != 1 || stageSummaries[0].(map[string]any)["title"] != "第 1 阶段" {
	t.Fatalf("expected stage summaries in graph response: %#v", body)
}
```

- [ ] **Step 4: Run repository and handler tests and verify RED**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestListWorkflowInstancesFiltersVisibleDemandsAndSortsRunningFirst|TestListWorkflowInstancesReturnsSummaries|TestGetProjectTaskGraphReturnsNodesEdgesAndDecisions' -count=1
```

Expected: FAIL because the repository, handler response structs, and generated sqlc rows do not expose the new fields yet.

- [ ] **Step 5: Extend `ListWorkflowInstances` SQL**

In `apps/control-plane/internal/storage/queries/project.sql`, extend `task_counts`:

```sql
COUNT(*) FILTER (WHERE status IN ('planned', 'pending'))::int AS planned_nodes,
COUNT(*) FILTER (WHERE status IN ('failed'))::int AS failed_nodes,
COUNT(*) FILTER (WHERE status IN ('cancelled'))::int AS cancelled_nodes,
MAX(NULLIF(risk_level, '')) FILTER (WHERE status NOT IN ('completed', 'done', 'success', 'cancelled')) AS active_risk_level,
MAX(updated_at) AS task_updated_at
```

Add `latest_events` CTE after `latest_jobs`:

```sql
, latest_events AS (
    SELECT DISTINCT ON (e.tenant_id, e.project_id, demand_id)
        e.tenant_id,
        e.project_id,
        demand_id,
        e.event_type,
        COALESCE(NULLIF(e.summary, ''), e.event_type)::text AS event_summary,
        e.created_at AS event_occurred_at
    FROM (
        SELECT
            pe.*,
            COALESCE(
                NULLIF(pe.payload->>'demand_id', '')::uuid,
                pt.demand_id,
                rd.demand_id
            ) AS demand_id
        FROM project_events pe
        LEFT JOIN project_tasks pt
          ON pt.tenant_id = pe.tenant_id
         AND pt.project_id = pe.project_id
         AND pt.id::text = pe.resource_id
        LEFT JOIN project_route_decisions rd
          ON rd.tenant_id = pe.tenant_id
         AND rd.project_id = pe.project_id
         AND rd.coordination_job_id::text = pe.resource_id
        WHERE pe.tenant_id = sqlc.arg('tenant_id')::uuid
    ) e
    WHERE demand_id IS NOT NULL
    ORDER BY e.tenant_id, e.project_id, demand_id, e.created_at DESC
)
```

Add selected fields:

```sql
COALESCE(tc.planned_nodes, 0)::int AS planned_nodes,
COALESCE(tc.failed_nodes, 0)::int AS failed_nodes,
COALESCE(tc.cancelled_nodes, 0)::int AS cancelled_nodes,
NULLIF(COALESCE(vd.source_refs->>'priority', vd.source_refs->>'severity'), '')::text AS priority_value,
CASE
  WHEN NULLIF(COALESCE(vd.source_refs->>'priority', vd.source_refs->>'severity'), '') IS NULL THEN NULL
  ELSE UPPER(NULLIF(COALESCE(vd.source_refs->>'priority', vd.source_refs->>'severity'), ''))
END::text AS priority_label,
CASE
  WHEN NULLIF(COALESCE(vd.source_refs->>'priority', vd.source_refs->>'severity'), '') IS NULL THEN NULL
  ELSE 'source_refs.priority'
END::text AS priority_source,
NULLIF(tc.active_risk_level, '')::text AS risk_level,
CASE
  WHEN NULLIF(tc.active_risk_level, '') IS NULL THEN NULL
  ELSE NULLIF(tc.active_risk_level, '')
END::text AS risk_label,
CASE
  WHEN NULLIF(tc.active_risk_level, '') IS NULL THEN NULL
  ELSE 'project_tasks.risk_level'
END::text AS risk_source,
NULLIF(vd.source_refs->>'sla_due_at', '')::timestamptz AS sla_due_at,
CASE
  WHEN NULLIF(vd.source_refs->>'sla_due_at', '') IS NULL THEN NULL
  ELSE GREATEST(EXTRACT(EPOCH FROM (NULLIF(vd.source_refs->>'sla_due_at', '')::timestamptz - NOW()))::int, 0)
END::int AS sla_remaining_seconds,
CASE
  WHEN NULLIF(vd.source_refs->>'sla_due_at', '') IS NULL THEN NULL
  ELSE (NULLIF(vd.source_refs->>'sla_due_at', '')::timestamptz < NOW())
END::boolean AS sla_breached,
CASE
  WHEN NULLIF(vd.source_refs->>'sla_due_at', '') IS NULL THEN NULL
  WHEN NULLIF(vd.source_refs->>'sla_due_at', '')::timestamptz < NOW() THEN '已超时'
  ELSE 'SLA 生效'
END::text AS sla_label,
CASE
  WHEN NULLIF(vd.source_refs->>'sla_due_at', '') IS NULL THEN NULL
  ELSE 'source_refs.sla_due_at'
END::text AS sla_source,
le.event_type::text AS recent_event_type,
le.event_summary::text AS recent_event_summary,
le.event_occurred_at::timestamptz AS recent_event_occurred_at
```

Join `latest_events`:

```sql
LEFT JOIN latest_events le
  ON le.project_id = vd.project_id
 AND le.demand_id = vd.demand_id
```

Keep the existing first selected `vd.demand_id` as the demand identifier. Do not add a second `vd.demand_id` column to the `SELECT` list.

- [ ] **Step 6: Regenerate sqlc**

Run:

```bash
make -C apps/control-plane generate-sqlc
```

Expected: command exits 0 and updates `apps/control-plane/internal/storage/queries/project.sql.go` and `apps/control-plane/internal/storage/queries/querier.go` if sqlc signatures changed.

- [ ] **Step 7: Map repository rows into domain fields**

In `apps/control-plane/internal/project/pg_repository.go`, extend the `WorkflowInstanceSummary` mapping:

```go
Progress: WorkflowInstanceProgress{
	TotalNodes:        row.TotalNodes,
	CompletedNodes:    row.CompletedNodes,
	RunningNodes:      row.RunningNodes,
	BlockedNodes:      row.BlockedNodes,
	WaitingHumanNodes: row.WaitingHumanNodes,
	PlannedNodes:      row.PlannedNodes,
	FailedNodes:       row.FailedNodes,
	CancelledNodes:    row.CancelledNodes,
},
Priority: workflowPriorityFromRow(row.PriorityValue, row.PriorityLabel, row.PrioritySource),
Risk:     workflowRiskFromRow(row.RiskLevel, row.RiskLabel, row.RiskSource),
SLA:      workflowSLAFromRow(row.SlaDueAt, row.SlaRemainingSeconds, row.SlaBreached, row.SlaLabel, row.SlaSource),
RecentEvent: workflowRecentEventFromRow(
	row.RecentEventType,
	row.RecentEventSummary,
	row.RecentEventOccurredAt,
),
```

Add helpers near the mapper:

```go
func workflowPriorityFromRow(value, label, source pgtype.Text) *WorkflowInstancePriority {
	if !value.Valid {
		return nil
	}
	return &WorkflowInstancePriority{Value: value.String, Label: textValueOr(label, value.String), Source: textValueOr(source, "unknown")}
}

func workflowRiskFromRow(level, label, source pgtype.Text) *WorkflowInstanceRisk {
	if !level.Valid {
		return nil
	}
	return &WorkflowInstanceRisk{Level: level.String, Label: textValueOr(label, level.String), Source: textValueOr(source, "unknown")}
}

func workflowSLAFromRow(dueAt pgtype.Timestamptz, remaining pgtype.Int4, breached pgtype.Bool, label, source pgtype.Text) *WorkflowInstanceSLA {
	if !dueAt.Valid && !remaining.Valid && !label.Valid {
		return nil
	}
	var due *time.Time
	if dueAt.Valid {
		due = &dueAt.Time
	}
	var seconds *int32
	if remaining.Valid {
		value := remaining.Int32
		seconds = &value
	}
	return &WorkflowInstanceSLA{
		DueAt:            due,
		RemainingSeconds: seconds,
		Breached:         breached.Valid && breached.Bool,
		Label:            textValueOr(label, ""),
		Source:           textValueOr(source, "unknown"),
	}
}

func workflowRecentEventFromRow(eventType, summary pgtype.Text, occurredAt pgtype.Timestamptz) *WorkflowInstanceRecentEvent {
	if !eventType.Valid || !occurredAt.Valid {
		return nil
	}
	return &WorkflowInstanceRecentEvent{
		EventType:  eventType.String,
		Summary:    textValueOr(summary, eventType.String),
		OccurredAt: occurredAt.Time,
	}
}

func textValueOr(value pgtype.Text, fallback string) string {
	if value.Valid && value.String != "" {
		return value.String
	}
	return fallback
}
```

- [ ] **Step 8: Add handler response structs and mappers**

In `apps/control-plane/internal/project/handler.go`, add response types near existing workflow response structs:

```go
type workflowInstancePriorityResponse struct {
	Value  string `json:"value"`
	Label  string `json:"label"`
	Source string `json:"source"`
}

type workflowInstanceRiskResponse struct {
	Level  string `json:"level"`
	Label  string `json:"label"`
	Source string `json:"source"`
}

type workflowInstanceSLAResponse struct {
	DueAt            *string `json:"due_at,omitempty"`
	RemainingSeconds *int32  `json:"remaining_seconds,omitempty"`
	Breached         bool    `json:"breached"`
	Label            string  `json:"label"`
	Source           string  `json:"source"`
}

type workflowInstanceRecentEventResponse struct {
	EventType  string `json:"event_type"`
	Summary    string `json:"summary"`
	OccurredAt string `json:"occurred_at"`
}
```

Extend `workflowInstanceProgressResponse`:

```go
PlannedNodes   int32 `json:"planned_nodes,omitempty"`
FailedNodes    int32 `json:"failed_nodes,omitempty"`
CancelledNodes int32 `json:"cancelled_nodes,omitempty"`
```

Extend `workflowInstanceResponse`:

```go
Priority    *workflowInstancePriorityResponse    `json:"priority,omitempty"`
Risk        *workflowInstanceRiskResponse        `json:"risk,omitempty"`
SLA         *workflowInstanceSLAResponse         `json:"sla,omitempty"`
RecentEvent *workflowInstanceRecentEventResponse `json:"recent_event,omitempty"`
```

Add mapper helpers:

```go
func workflowPriorityResponse(priority *WorkflowInstancePriority) *workflowInstancePriorityResponse {
	if priority == nil {
		return nil
	}
	return &workflowInstancePriorityResponse{Value: priority.Value, Label: priority.Label, Source: priority.Source}
}

func workflowRiskResponse(risk *WorkflowInstanceRisk) *workflowInstanceRiskResponse {
	if risk == nil {
		return nil
	}
	return &workflowInstanceRiskResponse{Level: risk.Level, Label: risk.Label, Source: risk.Source}
}

func workflowSLAResponse(sla *WorkflowInstanceSLA) *workflowInstanceSLAResponse {
	if sla == nil {
		return nil
	}
	var due *string
	if sla.DueAt != nil {
		value := sla.DueAt.Format(time.RFC3339)
		due = &value
	}
	return &workflowInstanceSLAResponse{
		DueAt:            due,
		RemainingSeconds: sla.RemainingSeconds,
		Breached:         sla.Breached,
		Label:            sla.Label,
		Source:           sla.Source,
	}
}

func workflowRecentEventResponse(event *WorkflowInstanceRecentEvent) *workflowInstanceRecentEventResponse {
	if event == nil {
		return nil
	}
	return &workflowInstanceRecentEventResponse{
		EventType:  event.EventType,
		Summary:    event.Summary,
		OccurredAt: event.OccurredAt.Format(time.RFC3339),
	}
}
```

Use these helpers in `workflowInstanceResponseFromDomain`.

- [ ] **Step 9: Add task graph response fields**

In `apps/control-plane/internal/project/handler.go`, extend `projectTaskGraphResponse`:

```go
StageSummaries []projectTaskGraphStageSummaryResponse `json:"stage_summaries,omitempty"`
```

Add:

```go
type projectTaskGraphStageSummaryResponse struct {
	StageIndex        int32  `json:"stage_index"`
	Title             string `json:"title"`
	TotalNodes        int32  `json:"total_nodes"`
	CompletedNodes    int32  `json:"completed_nodes"`
	RunningNodes      int32  `json:"running_nodes"`
	WaitingHumanNodes int32  `json:"waiting_human_nodes"`
	BlockedNodes      int32  `json:"blocked_nodes"`
}
```

Extend `projectTaskGraphNodeResponse`:

```go
StatusReason   string                                  `json:"status_reason,omitempty"`
UpdatedAt      string                                  `json:"updated_at,omitempty"`
CurrentBlocker *workflowInstanceCurrentBlockerResponse `json:"current_blocker,omitempty"`
```

Map graph summaries in `taskGraphResponseFromDomain`:

```go
StageSummaries: taskGraphStageSummaryResponses(graph.StageSummaries),
```

Add helper:

```go
func taskGraphStageSummaryResponses(items []ProjectTaskGraphStageSummary) []projectTaskGraphStageSummaryResponse {
	responses := make([]projectTaskGraphStageSummaryResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, projectTaskGraphStageSummaryResponse{
			StageIndex:        item.StageIndex,
			Title:             item.Title,
			TotalNodes:        item.TotalNodes,
			CompletedNodes:    item.CompletedNodes,
			RunningNodes:      item.RunningNodes,
			WaitingHumanNodes: item.WaitingHumanNodes,
			BlockedNodes:      item.BlockedNodes,
		})
	}
	return responses
}
```

- [ ] **Step 10: Run repository and handler tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestListWorkflowInstancesFiltersVisibleDemandsAndSortsRunningFirst|TestListWorkflowInstancesReturnsSummaries|TestGetProjectTaskGraphReturnsNodesEdgesAndDecisions' -count=1
```

Expected: PASS.

- [ ] **Step 11: Commit repository and handler read model**

Run:

```bash
git add apps/control-plane/internal/storage/queries/project.sql apps/control-plane/internal/storage/queries/project.sql.go apps/control-plane/internal/storage/queries/querier.go apps/control-plane/internal/project/pg_repository.go apps/control-plane/internal/project/handler.go apps/control-plane/internal/project/pg_repository_test.go apps/control-plane/internal/project/handler_test.go
git commit -m "feat: enrich workflow read model responses"
```

## Task 3: OpenAPI And Web API Types

**Files:**

- Modify: `contracts/control-plane/openapi.yaml`
- Modify generated files from `corepack pnpm generate:control-plane`
- Modify: `apps/web/src/lib/api/projects.ts`
- Test: `apps/web/src/lib/api/projects.test.ts`

- [ ] **Step 1: Extend OpenAPI schema**

In `contracts/control-plane/openapi.yaml`, add component schemas:

```yaml
    WorkflowInstancePriority:
      type: object
      required:
        - value
        - label
        - source
      properties:
        value:
          type: string
        label:
          type: string
        source:
          type: string
    WorkflowInstanceRisk:
      type: object
      required:
        - level
        - label
        - source
      properties:
        level:
          type: string
        label:
          type: string
        source:
          type: string
    WorkflowInstanceSLA:
      type: object
      required:
        - breached
        - label
        - source
      properties:
        due_at:
          type: string
          format: date-time
        remaining_seconds:
          type: integer
          format: int32
        breached:
          type: boolean
        label:
          type: string
        source:
          type: string
    WorkflowInstanceRecentEvent:
      type: object
      required:
        - event_type
        - summary
        - occurred_at
      properties:
        event_type:
          type: string
        summary:
          type: string
        occurred_at:
          type: string
          format: date-time
    ProjectTaskGraphStageSummary:
      type: object
      required:
        - stage_index
        - title
        - total_nodes
        - completed_nodes
        - running_nodes
        - waiting_human_nodes
        - blocked_nodes
      properties:
        stage_index:
          type: integer
          format: int32
        title:
          type: string
        total_nodes:
          type: integer
          format: int32
        completed_nodes:
          type: integer
          format: int32
        running_nodes:
          type: integer
          format: int32
        waiting_human_nodes:
          type: integer
          format: int32
        blocked_nodes:
          type: integer
          format: int32
```

Extend `WorkflowInstanceProgress` properties with optional fields:

```yaml
        planned_nodes:
          type: integer
          format: int32
        failed_nodes:
          type: integer
          format: int32
        cancelled_nodes:
          type: integer
          format: int32
```

Extend `WorkflowInstanceSummary` properties:

```yaml
        priority:
          $ref: "#/components/schemas/WorkflowInstancePriority"
        risk:
          $ref: "#/components/schemas/WorkflowInstanceRisk"
        sla:
          $ref: "#/components/schemas/WorkflowInstanceSLA"
        recent_event:
          $ref: "#/components/schemas/WorkflowInstanceRecentEvent"
```

Extend `ProjectTaskGraph` properties:

```yaml
        stage_summaries:
          type: array
          items:
            $ref: "#/components/schemas/ProjectTaskGraphStageSummary"
```

Extend `ProjectTaskGraphNode` properties:

```yaml
        status_reason:
          type: string
        updated_at:
          type: string
          format: date-time
        current_blocker:
          $ref: "#/components/schemas/WorkflowInstanceCurrentBlocker"
```

- [ ] **Step 2: Update frontend API types**

In `apps/web/src/lib/api/projects.ts`, add types:

```ts
export type WorkflowInstancePriority = {
  value: string;
  label: string;
  source: string;
};

export type WorkflowInstanceRisk = {
  level: string;
  label: string;
  source: string;
};

export type WorkflowInstanceSLA = {
  due_at?: string;
  remaining_seconds?: number;
  breached: boolean;
  label: string;
  source: string;
};

export type WorkflowInstanceRecentEvent = {
  event_type: string;
  summary: string;
  occurred_at: string;
};

export type ProjectTaskGraphStageSummary = {
  stage_index: number;
  title: string;
  total_nodes: number;
  completed_nodes: number;
  running_nodes: number;
  waiting_human_nodes: number;
  blocked_nodes: number;
};
```

Extend `WorkflowInstanceProgress`:

```ts
  planned_nodes?: number;
  failed_nodes?: number;
  cancelled_nodes?: number;
```

Extend `WorkflowInstanceSummary`:

```ts
  priority?: WorkflowInstancePriority;
  risk?: WorkflowInstanceRisk;
  sla?: WorkflowInstanceSLA;
  recent_event?: WorkflowInstanceRecentEvent;
```

Extend `ProjectTaskGraphNode`:

```ts
  status_reason?: string;
  updated_at?: string;
  current_blocker?: WorkflowInstanceCurrentBlocker;
```

Extend `ProjectTaskGraph`:

```ts
  stage_summaries?: ProjectTaskGraphStageSummary[];
```

- [ ] **Step 3: Add API client type test**

In `apps/web/src/lib/api/projects.test.ts`, add a test that exercises the optional fields through `listWorkflowInstances`:

```ts
it("preserves optional workflow instance read model fields", async () => {
  const fetcher = vi.fn(async () =>
    jsonResponse([
      {
        created_at: "2026-06-16T08:00:00Z",
        current_blocker: { type: "decision_request", title: "等待人工审批" },
        demand_id: "demand-1",
        priority: { value: "p1", label: "P1", source: "source_refs.priority" },
        progress: {
          blocked_nodes: 1,
          cancelled_nodes: 0,
          completed_nodes: 2,
          failed_nodes: 0,
          planned_nodes: 1,
          running_nodes: 1,
          total_nodes: 5,
          waiting_human_nodes: 1,
        },
        project_id: "project-1",
        project_name: "支付项目",
        recent_event: {
          event_type: "decision.requested",
          occurred_at: "2026-06-16T08:05:00Z",
          summary: "已创建恢复决策请求",
        },
        risk: { level: "high", label: "高风险", source: "project_tasks.risk_level" },
        selected_coordination_job_id: "job-1",
        sla: {
          breached: false,
          due_at: "2026-06-16T08:30:00Z",
          label: "剩余 25 分钟",
          remaining_seconds: 1500,
          source: "source_refs.sla_due_at",
        },
        status: "waiting_human",
        status_reason: "等待人工决策",
        submitted_by_display_name: "张晓明",
        submitted_by_user_id: "user-1",
        title: "支付成功率下降",
        updated_at: "2026-06-16T08:05:00Z",
      },
    ]),
  ) as unknown as typeof fetch;

  const items = await listWorkflowInstances({ baseUrl: "http://control-plane.local", fetcher });

  expect(items[0]?.priority?.label).toBe("P1");
  expect(items[0]?.risk?.level).toBe("high");
  expect(items[0]?.sla?.remaining_seconds).toBe(1500);
  expect(items[0]?.recent_event?.event_type).toBe("decision.requested");
  expect(items[0]?.progress.planned_nodes).toBe(1);
});
```

- [ ] **Step 4: Generate contracts and run API tests**

Run:

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
corepack pnpm --filter @superteam/web test src/lib/api/projects.test.ts
```

Expected: all commands exit 0.

- [ ] **Step 5: Commit contract and API type changes**

Run:

```bash
git add contracts/control-plane/openapi.yaml apps/web/src/lib/api/projects.ts apps/web/src/lib/api/projects.test.ts
git add apps/control-plane/internal/api/gen/control_plane.gen.go
git commit -m "feat: update workflow read model contracts"
```

## Task 4: Workflow Entrance Page

**Files:**

- Modify: `apps/web/src/features/workflows/index.tsx`
- Create: `apps/web/src/features/workflows/components/workflow-entrance.tsx`
- Create: `apps/web/src/features/workflows/components/workflow-instance-card.tsx`
- Modify: `apps/web/src/features/workflows/workflow-status.ts`
- Test: `apps/web/src/features/workflows/index.test.tsx`

- [ ] **Step 1: Rewrite workflow tests for entrance page behavior**

In `apps/web/src/features/workflows/index.test.tsx`, replace the test named `renders visible workflow instances and selected planning detail` with:

```tsx
it("renders workflow instance cards on the entrance page without task details", async () => {
  const screen = await renderWorkflowView({ demandId: undefined });

  await expect.element(screen.getByRole("heading", { name: "流程编排" })).toBeVisible();
  await expect.element(screen.getByRole("link", { name: /支付成功率下降/ })).toBeVisible();
  await expect.element(screen.getByText("支付项目")).toBeVisible();
  await expect.element(screen.getByText("等待协调线程生成任务")).toBeVisible();
  await expect.element(screen.getByText("任务正在规划")).not.toBeInTheDocument();
  await expect.element(screen.getByTestId("workflow-canvas")).not.toBeInTheDocument();
});
```

Add a test for optional fields:

```tsx
it("shows optional SLA priority and risk on workflow cards only when present", async () => {
  const fetcher = createWorkflowFetcher({
    instances: [
      makeWorkflowInstance("demand-running", {
        priority: { value: "p1", label: "P1", source: "source_refs.priority" },
        risk: { level: "high", label: "高风险", source: "project_tasks.risk_level" },
        sla: {
          breached: false,
          due_at: "2026-06-16T09:00:00Z",
          label: "剩余 18 分钟",
          remaining_seconds: 1080,
          source: "source_refs.sla_due_at",
        },
      }),
      makeWorkflowInstance("demand-pr", {
        demand_id: "demand-pr",
        project_name: "代码审查项目",
        title: "PR 审查",
      }),
    ],
  });

  const screen = await renderWorkflowView({ demandId: undefined, fetcher });

  await expect.element(screen.getByText("P1")).toBeVisible();
  await expect.element(screen.getByText("高风险")).toBeVisible();
  await expect.element(screen.getByText("剩余 18 分钟")).toBeVisible();
  expect(screen.getByRole("link", { name: /PR 审查/ }).element().textContent).not.toContain("P1");
});
```

- [ ] **Step 2: Run frontend tests and verify RED**

Run:

```bash
corepack pnpm --filter @superteam/web test src/features/workflows/index.test.tsx
```

Expected: FAIL because `/workflows` still redirects/selects a detail layout and does not render entrance cards.

- [ ] **Step 3: Split `WorkflowView` into entrance and workbench branches**

In `apps/web/src/features/workflows/index.tsx`, keep list fetching in `WorkflowView`, but render entrance when `demandId` is absent:

```tsx
if (!demandId) {
  return (
    <WorkflowShell>
      <WorkflowEntrance
        instances={instances}
        isError={listQuery.isError}
        isLoading={listQuery.isLoading}
      />
    </WorkflowShell>
  );
}
```

Remove the effect that automatically navigates `/workflows` to the first demand. Keep stale demand replacement only when a non-empty `demandId` is present and not found after the list loads.

Import:

```tsx
import { WorkflowEntrance } from "./components/workflow-entrance";
```

- [ ] **Step 4: Create `WorkflowEntrance`**

Create `apps/web/src/features/workflows/components/workflow-entrance.tsx`:

```tsx
import { WorkflowInstanceCard } from "./workflow-instance-card";
import { LiquidCard } from "@/components/superteam";
import type { WorkflowInstanceSummary } from "@/lib/api/projects";

type WorkflowEntranceProps = {
  instances: WorkflowInstanceSummary[];
  isError?: boolean;
  isLoading?: boolean;
};

export function WorkflowEntrance({ instances, isError, isLoading }: WorkflowEntranceProps) {
  if (isError) {
    return (
      <LiquidCard className="rounded-xl p-6 text-sm text-destructive">
        流程实例加载失败
      </LiquidCard>
    );
  }

  if (isLoading) {
    return (
      <LiquidCard className="rounded-xl p-6 text-sm text-muted-foreground">
        正在加载流程实例
      </LiquidCard>
    );
  }

  if (instances.length === 0) {
    return (
      <LiquidCard className="rounded-xl p-6 text-sm text-muted-foreground">
        暂无可见流程实例
      </LiquidCard>
    );
  }

  return (
    <div className="grid gap-4 md:grid-cols-2 2xl:grid-cols-3">
      {instances.map((instance) => (
        <WorkflowInstanceCard instance={instance} key={instance.demand_id} />
      ))}
    </div>
  );
}
```

- [ ] **Step 5: Create workflow instance card component**

Create `apps/web/src/features/workflows/components/workflow-instance-card.tsx`:

```tsx
import { Link } from "@tanstack/react-router";
import { AlertCircle, Clock3, FolderKanban, GitBranch, ShieldAlert } from "lucide-react";
import { StatusBadge } from "@/components/superteam";
import type { WorkflowInstanceSummary } from "@/lib/api/projects";
import { workflowStatusLabel, workflowStatusTone } from "../workflow-status";

type WorkflowInstanceCardProps = {
  instance: WorkflowInstanceSummary;
};

export function WorkflowInstanceCard({ instance }: WorkflowInstanceCardProps) {
  const completed = instance.progress.completed_nodes;
  const total = instance.progress.total_nodes;
  const percent = total > 0 ? Math.round((completed / total) * 100) : 0;

  return (
    <Link
      aria-label={`${instance.title}，${workflowStatusLabel(instance.status)}`}
      className="group block rounded-xl border bg-card/95 p-4 text-card-foreground shadow-sm transition hover:-translate-y-0.5 hover:shadow-[var(--superteam-shadow-mid)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      params={{ demandId: instance.demand_id }}
      to="/workflows/$demandId"
    >
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="line-clamp-2 text-base font-semibold tracking-normal">
            {instance.title}
          </h2>
          <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <span className="inline-flex min-w-0 items-center gap-1">
              <FolderKanban className="size-3.5 shrink-0" />
              <span className="truncate">{instance.project_name}</span>
            </span>
            <span className="inline-flex items-center gap-1">
              <Clock3 className="size-3.5" />
              {formatDateTime(instance.updated_at)}
            </span>
          </div>
        </div>
        <StatusBadge tone={workflowStatusTone(instance.status)}>
          {workflowStatusLabel(instance.status)}
        </StatusBadge>
      </div>

      <div className="mt-4">
        <div className="flex items-center justify-between text-xs text-muted-foreground">
          <span>已完成 {completed}/{total}</span>
          <span>{percent}%</span>
        </div>
        <div className="mt-2 h-2 overflow-hidden rounded-full bg-muted">
          <div className="h-full rounded-full bg-[color:var(--superteam-info)]" style={{ width: `${percent}%` }} />
        </div>
      </div>

      <div className="mt-4 grid grid-cols-3 gap-2 text-xs">
        <Counter label="运行中" value={instance.progress.running_nodes} />
        <Counter label="等待人工" value={instance.progress.waiting_human_nodes} />
        <Counter label="阻塞" value={instance.progress.blocked_nodes} />
      </div>

      <div className="mt-4 min-h-10 rounded-lg border bg-background/70 p-3 text-xs leading-5 text-muted-foreground">
        {instance.current_blocker ? (
          <span className="inline-flex gap-2 text-foreground">
            <AlertCircle className="mt-0.5 size-3.5 shrink-0 text-[color:var(--superteam-warning)]" />
            {instance.current_blocker.title}
          </span>
        ) : instance.recent_event ? (
          <span className="inline-flex gap-2">
            <GitBranch className="mt-0.5 size-3.5 shrink-0" />
            {instance.recent_event.summary}
          </span>
        ) : (
          instance.status_reason || "等待更新"
        )}
      </div>

      <div className="mt-3 flex flex-wrap gap-2">
        {instance.priority ? <StatusBadge tone="danger">{instance.priority.label}</StatusBadge> : null}
        {instance.risk ? (
          <StatusBadge tone={instance.risk.level === "high" ? "danger" : "warning"}>
            <ShieldAlert className="size-3.5" />
            {instance.risk.label}
          </StatusBadge>
        ) : null}
        {instance.sla ? (
          <StatusBadge tone={instance.sla.breached ? "danger" : "warning"}>
            {instance.sla.label}
          </StatusBadge>
        ) : null}
      </div>
    </Link>
  );
}

function Counter({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-lg border bg-background/70 px-2 py-1.5">
      <div className="font-semibold text-foreground">{value}</div>
      <div className="text-muted-foreground">{label}</div>
    </div>
  );
}

function formatDateTime(value: string): string {
  if (!value) return "未更新";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}
```

- [ ] **Step 6: Run frontend entrance tests**

Run:

```bash
corepack pnpm --filter @superteam/web test src/features/workflows/index.test.tsx
```

Expected: entrance tests PASS. Dialog-related tests may still fail until Task 5 converts the detail page.

- [ ] **Step 7: Commit entrance page**

Run:

```bash
git add apps/web/src/features/workflows/index.tsx apps/web/src/features/workflows/components/workflow-entrance.tsx apps/web/src/features/workflows/components/workflow-instance-card.tsx apps/web/src/features/workflows/index.test.tsx
git commit -m "feat: add workflow instance entrance cards"
```

## Task 5: Workflow Workbench And Right Inspector

**Files:**

- Modify: `apps/web/src/features/workflows/components/workflow-detail.tsx`
- Modify: `apps/web/src/features/workflows/components/workflow-graph-canvas.tsx`
- Modify: `apps/web/src/features/workflows/components/workflow-task-node.tsx`
- Modify: `apps/web/src/features/workflows/components/workflow-node-inspector.tsx`
- Modify: `apps/web/src/features/workflows/workflow-graph-adapter.ts`
- Test: `apps/web/src/features/workflows/index.test.tsx`
- Test: `apps/web/src/features/workflows/workflow-graph-adapter.test.ts`

- [ ] **Step 1: Replace dialog tests with right-inspector tests**

In `apps/web/src/features/workflows/index.test.tsx`, replace `opens node details in a dialog when a task card is clicked` with:

```tsx
it("updates the right inspector when a task card is clicked", async () => {
  const graph = makeGraph(
    [
      makeGraphNode("task-failed", "失败任务", "failed", {
        expected_outputs: ["失败报告"],
      }),
      makeGraphNode("task-assigned", "巡检任务", "assigned", {
        expected_outputs: ["巡检报告"],
      }),
    ],
    {
      execution_summaries: [
        {
          artifact_refs: [],
          confidence_factors: {},
          conclusion: "巡检任务结论",
          digital_employee_id: "employee-2",
          evidence_refs: [],
          id: "summary-assigned",
          missing_information: [],
          project_id: "project-1",
          project_task_id: "task-assigned",
          requires_human_review: false,
          tenant_id: "tenant-1",
        } satisfies ProjectExecutionSummary,
      ],
      runs: [
        {
          project_task_id: "task-assigned",
          provider_type: "codex",
          runtime_node_summary: "runtime-b",
          runtime_task_id: "runtime-task-assigned",
          status: "queued",
        } satisfies ProjectTaskGraphRun,
      ],
    },
  );
  const screen = await renderWorkflowView({
    demandId: "demand-running",
    fetcher: createWorkflowFetcher({ graph }),
  });

  await expect.element(screen.getByRole("dialog", { name: "节点详情" })).not.toBeInTheDocument();
  await expect.element(screen.getByText("失败报告")).toBeVisible();

  await userEvent.click(screen.getByRole("button", { name: "巡检任务" }));

  await expect.element(screen.getByRole("heading", { name: "巡检任务" })).toBeVisible();
  await expect.element(screen.getByText("巡检报告")).toBeVisible();
  await expect.element(screen.getByText("queued · codex · runtime-b")).toBeVisible();
});
```

Replace `opens the parent task details when a decision attachment node is clicked` with:

```tsx
it("updates the inspector to the parent task when a decision attachment node is clicked", async () => {
  const graph = makeGraph(
    [
      makeGraphNode("task-review", "待审批任务", "waiting_human", {
        expected_outputs: ["审批结果"],
      }),
    ],
    {
      decision_requests: [
        {
          approval_request_id: "approval-1",
          decision_type: "human_approval",
          id: "decision-1",
          project_id: "project-1",
          project_task_id: "task-review",
          status_snapshot: "pending",
          target_user_id: "owner-1",
          tenant_id: "tenant-1",
          title_snapshot: "确认上线风险",
        } satisfies ProjectDecisionRequest,
      ],
    },
  );
  const screen = await renderWorkflowView({
    demandId: "demand-running",
    fetcher: createWorkflowFetcher({ graph }),
  });

  await userEvent.click(screen.getByRole("button", { name: "确认上线风险" }));

  await expect.element(screen.getByRole("heading", { name: "待审批任务" })).toBeVisible();
  await expect.element(screen.getByText("审批结果")).toBeVisible();
  await expect.element(screen.getByRole("link", { name: /审批/ })).toBeVisible();
});
```

- [ ] **Step 2: Add stage summary adapter test**

In `apps/web/src/features/workflows/workflow-graph-adapter.test.ts`, add:

```ts
it("keeps task nodes grouped by stage index for workbench stage summaries", () => {
  const graph = makeGraph();
  graph.nodes = [
    makeTask("task-1", "completed", { stage_index: 1, title: "入口" }),
    makeTask("task-2", "running", { stage_index: 2, title: "巡检" }),
    makeTask("task-3", "waiting_human", { stage_index: 2, title: "审批" }),
  ];

  const result = buildWorkflowGraphElements(graph);

  const taskOne = result.nodes.find((node) => node.id === "task:task-1");
  const taskTwo = result.nodes.find((node) => node.id === "task:task-2");
  const taskThree = result.nodes.find((node) => node.id === "task:task-3");
  expect(taskOne?.position.x).toBeLessThan(taskTwo?.position.x ?? 0);
  expect(taskTwo?.position.x).toBe(taskThree?.position.x);
});
```

Update the current `makeTask(id, status)` helper to accept an overrides object:

```ts
function makeTask(
  id: string,
  status: string,
  overrides: Partial<ProjectTaskGraph["nodes"][number]> = {},
): ProjectTaskGraph["nodes"][number] {
  return {
    demand_id: "demand-1",
    expected_outputs: [],
    handoff_contract: {},
    id,
    input_requirements: {},
    planner_metadata: {},
    project_id: "project-1",
    requires_human_approval: false,
    summary: id,
    status,
    tenant_id: "tenant-1",
    title: id,
    ...overrides,
  };
}
```

- [ ] **Step 3: Run workbench tests and verify RED**

Run:

```bash
corepack pnpm --filter @superteam/web test src/features/workflows/index.test.tsx src/features/workflows/workflow-graph-adapter.test.ts
```

Expected: FAIL because `WorkflowDetail` still uses `NodeDetailDialog`.

- [ ] **Step 4: Convert `WorkflowDetail` to summary, canvas, inspector layout**

In `apps/web/src/features/workflows/components/workflow-detail.tsx`, remove `Dialog`, `NodeDetailDialog`, and `isNodeDialogOpen`. Render the graph and inspector in a two-column workbench:

```tsx
{isGraphReady && graph ? (
  <div className="grid min-w-0 gap-4 p-4 @5xl/workflow-graph:grid-cols-[minmax(0,1fr)_360px]">
    <div className="min-w-0">
      <WorkflowGraphCanvas
        graph={graph}
        onNodeOpen={setSelectedNodeId}
        onSelectedNodeChange={setSelectedNodeId}
        selectedNodeId={selectedNodeId}
      />
    </div>
    <WorkflowNodeInspector
      graph={graph}
      selectedTask={selectedTask}
      variant="card"
    />
  </div>
) : (
  <div className="p-5 text-sm leading-6 text-muted-foreground">
    当前需求已进入流程实例，任务节点会在协调线程规划完成后显示。
  </div>
)}
```

Keep `DemandSummaryBar` as the top global summary. Add optional status counts from `instance.progress` in the summary action area:

```tsx
<StatusBadge tone="info">运行中 {instance.progress.running_nodes}</StatusBadge>
<StatusBadge tone="warning">等待人工 {instance.progress.waiting_human_nodes}</StatusBadge>
<StatusBadge tone="danger">阻塞 {instance.progress.blocked_nodes}</StatusBadge>
```

- [ ] **Step 5: Make canvas click update inspector without opening dialog**

In `apps/web/src/features/workflows/components/workflow-graph-canvas.tsx`, keep the prop name `onNodeOpen` to minimize call-site churn, but document behavior by using it as selection callback:

```tsx
onNodeClick={(_, node) => {
  const selectedId = node.parentId ?? node.id;
  onSelectedNodeChange(selectedId);
  onNodeOpen(selectedId);
}}
```

Do not import or render any Dialog component in the workflow detail path.

- [ ] **Step 6: Expand node inspector rows**

In `apps/web/src/features/workflows/components/workflow-node-inspector.tsx`, keep `variant="card"` and remove dialog-specific branches if they no longer serve tests. Add rows:

```tsx
<InspectorRow label="负责人" value={employeeNameForTask(graph, selectedTask)} />
<InspectorRow label="阻塞" value={selectedTask.current_blocker?.title ?? selectedTask.status_reason ?? "暂无阻塞"} />
<InspectorRow label="交接契约" value={formatValue(selectedTask.handoff_contract)} />
```

Add helper:

```tsx
function employeeNameForTask(graph: ProjectTaskGraph, task: ProjectTaskGraphNode): string {
  if (!task.assigned_digital_employee_id) return "未分配";
  return (
    graph.employees.find(
      (employee) => employee.digital_employee_id === task.assigned_digital_employee_id,
    )?.display_name ?? task.assigned_digital_employee_id
  );
}
```

Keep existing jump links:

- Runtime link when `run.runtime_task_id` exists.
- Approval link when decision requests exist.

- [ ] **Step 7: Show richer task node fields**

In `apps/web/src/features/workflows/components/workflow-task-node.tsx`, add optional risk and run status badges:

```tsx
{data.riskLevel ? (
  <span className="inline-flex max-w-full items-center gap-1.5 rounded-full border bg-background/70 px-2.5 py-1 text-xs text-[color:var(--superteam-warning)]">
    <ShieldCheck className="size-3.5 shrink-0" />
    <span className="truncate">{data.riskLevel}</span>
  </span>
) : null}
{data.runStatus ? (
  <span className="inline-flex max-w-full items-center gap-1.5 rounded-full border bg-background/70 px-2.5 py-1 text-xs text-muted-foreground">
    <Bot className="size-3.5 shrink-0 text-[color:var(--superteam-info)]" />
    <span className="truncate">Run {data.runStatus}</span>
  </span>
) : null}
```

- [ ] **Step 8: Run workbench tests**

Run:

```bash
corepack pnpm --filter @superteam/web test src/features/workflows/index.test.tsx src/features/workflows/workflow-graph-adapter.test.ts
```

Expected: PASS.

- [ ] **Step 9: Commit workbench UI**

Run:

```bash
git add apps/web/src/features/workflows/components/workflow-detail.tsx apps/web/src/features/workflows/components/workflow-graph-canvas.tsx apps/web/src/features/workflows/components/workflow-task-node.tsx apps/web/src/features/workflows/components/workflow-node-inspector.tsx apps/web/src/features/workflows/workflow-graph-adapter.ts apps/web/src/features/workflows/index.test.tsx apps/web/src/features/workflows/workflow-graph-adapter.test.ts
git commit -m "feat: add workflow workbench inspector"
```

## Task 6: Full Verification And Real Smoke

**Files:**

- No source files are intentionally modified in this task.
- Use existing scripts and commands.

- [ ] **Step 1: Run backend focused tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestListWorkflowInstances|TestGetProjectTaskGraph|TestProjectTaskGraph|TestWorkflow' -count=1
```

Expected: PASS.

- [ ] **Step 2: Run API route tests**

Run:

```bash
go test ./apps/control-plane/internal/api -run 'TestProjectRoutes|TestWorkflow' -count=1
```

Expected: PASS.

- [ ] **Step 3: Run contract generation and verification**

Run:

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
```

Expected: both commands exit 0 and leave no unexpected generated diff beyond this feature.

- [ ] **Step 4: Run Web verification**

Run:

```bash
corepack pnpm verify:web
```

Expected: PASS.

- [ ] **Step 5: Run whitespace and status checks**

Run:

```bash
git diff --check
git status --short
```

Expected: `git diff --check` exits 0. `git status --short` only shows intentional files for this implementation and any pre-existing unrelated files such as `docs/design/userManager/`.

- [ ] **Step 6: Start or restart local services**

Use the existing project service script:

```bash
./scripts/dev-services.sh restart all
```

Expected: Control Plane is healthy at `http://127.0.0.1:8081/health`, and Web is healthy at `http://127.0.0.1:3000/`.

- [ ] **Step 7: Smoke the real workflow instances API**

Use the seeded local development administrator from `apps/control-plane/internal/storage/migrations/002_seed_dev_admin.sql` to obtain a Console session cookie:

```bash
mkdir -p .scratch
rm -f .scratch/workflow-smoke.cookies .scratch/workflow-instances.json
curl -sS \
  -c .scratch/workflow-smoke.cookies \
  -H "content-type: application/json" \
  -X POST \
  -d '{"username":"admin","password":"admin"}' \
  "http://127.0.0.1:8081/api/auth/login" >/dev/null
curl -sS \
  -b .scratch/workflow-smoke.cookies \
  "http://127.0.0.1:8081/api/v1/workflow-instances?limit=5" \
  | tee .scratch/workflow-instances.json \
  | jq .
```

Expected: JSON array. At least one item should contain `demand_id`, `project_id`, `status`, `progress`, and no fake optional fields when there is no source.

If the seeded `admin` / `admin` login is disabled in the target database, record authentication as a verification blocker and do not claim real API smoke passed.

- [ ] **Step 8: Smoke the real Web routes**

Open the Web app in the browser:

```text
http://127.0.0.1:3000/workflows
```

Expected:

- `/workflows` shows workflow instance cards.
- Cards do not show task titles, task owners, task logs, or task input/output.
- Cards with missing optional fields do not show empty badges.

Select the first real demand ID returned by the API smoke:

```bash
DEMAND_ID="$(jq -r '.[0].demand_id // empty' .scratch/workflow-instances.json)"
test -n "$DEMAND_ID"
printf 'http://127.0.0.1:3000/workflows/%s\n' "$DEMAND_ID"
```

Open the printed URL in the browser.

Expected:

- Summary bar loads from real demand/workflow data.
- Task graph uses real task graph data when available.
- Clicking a task updates the right Inspector.
- No node-detail Dialog appears as the main detail path.
- Runtime and Approval links appear only when the corresponding graph facts exist.

If no real demand has a task graph, record this as a blocker for full end-to-end graph verification and report which API calls were still tested.

## Completion Gate

Before saying the implementation is complete:

- Use the repo-local `$superteam-completion-check` skill.
- Report which commands passed.
- Report whether the real API and real Web smoke passed.
- If the real graph smoke is blocked by missing auth, missing services, missing data, or unavailable Runtime/Provider, state the blocker directly and do not call the task fully end-to-end verified.
