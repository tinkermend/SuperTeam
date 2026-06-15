# Workflow Instances Read Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the Control Plane read model that lists the current console user's visible demand/workflow instances for the Workflow Orchestration page.

**Architecture:** Keep `ProjectDemand` as the workflow instance fact source. Add a project-domain list request, repository query, service status aggregation, HTTP handler, route, and OpenAPI schema without adding a migration. The API returns already-filtered and already-sorted workflow instance summaries so the Web layer displays facts instead of recomputing business state.

**Tech Stack:** Go 1.25, chi/net/http, PostgreSQL/sqlc, OpenAPI/oapi-codegen, existing project package test helpers.

---

## Source Spec

Implement this subplan against:

- `docs/superpowers/specs/2026-06-15-workflow-orchestration-graph-design.md`

This subplan covers only:

- `GET /api/v1/workflow-instances`
- current-user visibility filtering
- service-owned status/progress aggregation
- route and contract coverage

It does not implement Web UI or xyflow rendering. Those are covered by:

- `docs/superpowers/plans/2026-06-15-workflow-shell-and-list.md`
- `docs/superpowers/plans/2026-06-15-workflow-xyflow-graph.md`

## File Structure

Modify:

- `apps/control-plane/internal/project/types.go`
  Add workflow instance request, status, progress, blocker, and summary domain types.
- `apps/control-plane/internal/project/repository.go`
  Add `ListWorkflowInstances`.
- `apps/control-plane/internal/project/service.go`
  Validate list input, normalize pagination, and compute status priority.
- `apps/control-plane/internal/project/handler.go`
  Add handler-service method, request parsing, JSON response structs, and response mappers.
- `apps/control-plane/internal/project/pg_repository.go`
  Wire the sqlc query and map rows into domain summaries.
- `apps/control-plane/internal/storage/queries/project.sql`
  Add `ListWorkflowInstances`.
- `apps/control-plane/internal/api/server.go`
  Register `GET /api/v1/workflow-instances` under console auth.
- `contracts/control-plane/openapi.yaml`
  Document the endpoint and schemas.

Test:

- `apps/control-plane/internal/project/service_test.go`
- `apps/control-plane/internal/project/pg_repository_test.go`
- `apps/control-plane/internal/project/handler_test.go`
- `apps/control-plane/internal/api/project_routes_test.go`

Generated:

- Regenerate sqlc output with `make -C apps/control-plane generate-sqlc`.
- Regenerate OpenAPI output with `pnpm generate:control-plane`.

## Task 1: Domain Types And Service Aggregation

**Files:**

- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/service.go`
- Test: `apps/control-plane/internal/project/service_test.go`

- [ ] **Step 1: Write failing service tests for status priority**

Add these tests near the existing `TestGetProjectTaskGraphRequiresFilterAndDoesNotApplyHiddenLimit` tests in `apps/control-plane/internal/project/service_test.go`:

```go
func TestListWorkflowInstancesNormalizesPaginationAndStatusPriority(t *testing.T) {
	tenantID := uuid.New()
	actorID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	repo := &workflowInstanceServiceRepository{
		memoryRepository: newMemoryRepository(),
		items: []WorkflowInstanceSummary{{
			DemandID: demandID,
			ProjectID: projectID,
			ProjectName: "支付巡检",
			Title: "定位支付成功率下降",
			SubmittedByUserID: actorID,
			Status: WorkflowInstanceStatusUnknown,
			Progress: WorkflowInstanceProgress{
				TotalNodes: 3,
				CompletedNodes: 1,
				RunningNodes: 1,
				BlockedNodes: 1,
				WaitingHumanNodes: 1,
			},
		}},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	items, err := service.ListWorkflowInstances(context.Background(), ListWorkflowInstancesRequest{
		TenantID: tenantID,
		ActorUserID: actorID,
		Limit: 0,
		Offset: -4,
	})
	if err != nil {
		t.Fatalf("list workflow instances: %v", err)
	}
	if repo.lastReq.Limit != 20 || repo.lastReq.Offset != 0 {
		t.Fatalf("expected normalized pagination, got limit=%d offset=%d", repo.lastReq.Limit, repo.lastReq.Offset)
	}
	if len(items) != 1 {
		t.Fatalf("expected one workflow instance, got %#v", items)
	}
	if items[0].Status != WorkflowInstanceStatusWaitingHuman {
		t.Fatalf("expected waiting_human to outrank running and planning, got %#v", items[0])
	}
}

func TestListWorkflowInstancesRejectsMissingActor(t *testing.T) {
	tenantID := uuid.New()
	repo := &workflowInstanceServiceRepository{memoryRepository: newMemoryRepository()}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.ListWorkflowInstances(context.Background(), ListWorkflowInstancesRequest{
		TenantID: tenantID,
	})
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected invalid project error, got %v", err)
	}
	if repo.calls != 0 {
		t.Fatalf("expected invalid request not to call repository, got %d calls", repo.calls)
	}
}

type workflowInstanceServiceRepository struct {
	*memoryRepository
	calls int
	lastReq ListWorkflowInstancesRequest
	items []WorkflowInstanceSummary
}

func (r *workflowInstanceServiceRepository) ListWorkflowInstances(ctx context.Context, req ListWorkflowInstancesRequest) ([]WorkflowInstanceSummary, error) {
	r.calls++
	r.lastReq = req
	return append([]WorkflowInstanceSummary(nil), r.items...), nil
}
```

- [ ] **Step 2: Run the service tests and verify RED**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestListWorkflowInstances' -count=1
```

Expected: FAIL because `ListWorkflowInstancesRequest`, `WorkflowInstanceSummary`, `WorkflowInstanceStatusWaitingHuman`, and `Service.ListWorkflowInstances` do not exist.

- [ ] **Step 3: Add workflow instance domain types**

In `apps/control-plane/internal/project/types.go`, add:

```go
type WorkflowInstanceStatus string

const (
	WorkflowInstanceStatusPlanning     WorkflowInstanceStatus = "planning"
	WorkflowInstanceStatusRunning      WorkflowInstanceStatus = "running"
	WorkflowInstanceStatusWaitingHuman WorkflowInstanceStatus = "waiting_human"
	WorkflowInstanceStatusFailed       WorkflowInstanceStatus = "failed"
	WorkflowInstanceStatusCompleted    WorkflowInstanceStatus = "completed"
	WorkflowInstanceStatusCancelled    WorkflowInstanceStatus = "cancelled"
	WorkflowInstanceStatusUnknown      WorkflowInstanceStatus = "unknown"
)

type ListWorkflowInstancesRequest struct {
	TenantID    uuid.UUID
	ActorUserID uuid.UUID
	Query       string
	ProjectID   *uuid.UUID
	Status      *WorkflowInstanceStatus
	Limit       int32
	Offset      int32
}

type WorkflowInstanceProgress struct {
	TotalNodes        int32
	CompletedNodes    int32
	RunningNodes      int32
	BlockedNodes      int32
	WaitingHumanNodes int32
}

type WorkflowInstanceCurrentBlocker struct {
	Type       string
	Title      string
	ResourceID *uuid.UUID
}

type WorkflowInstanceSummary struct {
	DemandID               uuid.UUID
	ProjectID              uuid.UUID
	ProjectName            string
	Title                  string
	SubmittedByUserID      uuid.UUID
	SubmittedByDisplayName string
	Status                 WorkflowInstanceStatus
	StatusReason           string
	CreatedAt              time.Time
	UpdatedAt              time.Time
	SelectedCoordinationJobID *uuid.UUID
	Progress               WorkflowInstanceProgress
	CurrentBlocker         *WorkflowInstanceCurrentBlocker
}
```

If `types.go` does not already import `time`, add it beside the existing `uuid` import.

- [ ] **Step 4: Add repository interface method**

In `apps/control-plane/internal/project/repository.go`, add this method to `Repository`:

```go
ListWorkflowInstances(ctx context.Context, req ListWorkflowInstancesRequest) ([]WorkflowInstanceSummary, error)
```

- [ ] **Step 5: Add service method and status normalization**

In `apps/control-plane/internal/project/service.go`, add:

```go
func (s *Service) ListWorkflowInstances(ctx context.Context, req ListWorkflowInstancesRequest) ([]WorkflowInstanceSummary, error) {
	if req.TenantID == uuid.Nil || req.ActorUserID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	req.Query = strings.TrimSpace(req.Query)
	req.Limit, req.Offset = normalizePagination(req.Limit, req.Offset)
	items, err := s.repository.ListWorkflowInstances(ctx, req)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Status = normalizeWorkflowInstanceStatus(items[i])
	}
	if req.Status != nil {
		filtered := make([]WorkflowInstanceSummary, 0, len(items))
		for _, item := range items {
			if item.Status == *req.Status {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	return items, nil
}

func normalizeWorkflowInstanceStatus(item WorkflowInstanceSummary) WorkflowInstanceStatus {
	switch item.Status {
	case WorkflowInstanceStatusFailed, WorkflowInstanceStatusCancelled:
		return item.Status
	}
	if item.Progress.WaitingHumanNodes > 0 {
		return WorkflowInstanceStatusWaitingHuman
	}
	if item.Progress.RunningNodes > 0 {
		return WorkflowInstanceStatusRunning
	}
	if item.Progress.TotalNodes == 0 {
		return WorkflowInstanceStatusPlanning
	}
	if item.Progress.CompletedNodes == item.Progress.TotalNodes {
		return WorkflowInstanceStatusCompleted
	}
	if item.Status != "" {
		return item.Status
	}
	return WorkflowInstanceStatusUnknown
}
```

Keep `strings` and `uuid` imports deduplicated.

- [ ] **Step 6: Run service tests and verify GREEN**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestListWorkflowInstances' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit the service slice**

Run:

```bash
git add apps/control-plane/internal/project/types.go apps/control-plane/internal/project/repository.go apps/control-plane/internal/project/service.go apps/control-plane/internal/project/service_test.go
git commit -m "feat: add workflow instance service model"
```

Expected: commit succeeds and contains only the service/domain test slice.

## Task 2: Repository Query And Mapping

**Files:**

- Modify: `apps/control-plane/internal/storage/queries/project.sql`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Test: `apps/control-plane/internal/project/pg_repository_test.go`
- Generated: `apps/control-plane/internal/storage/queries/*.go`

- [ ] **Step 1: Write failing repository visibility and sorting test**

Add this test near task graph read tests in `apps/control-plane/internal/project/pg_repository_test.go`:

```go
func TestListWorkflowInstancesFiltersVisibleDemandsAndSortsRunningFirst(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	ctx := context.Background()
	visibleProjectID := createProjectFixture(t, repo, tenantID)
	hiddenProjectID := createProjectFixture(t, repo, tenantID)
	actorID := uuid.New()
	visibleDemandID := createDemandFixture(t, repo, tenantID, visibleProjectID)
	hiddenDemandID := createDemandFixture(t, repo, tenantID, hiddenProjectID)
	jobID := createCoordinationJobFixture(t, repo, tenantID, visibleProjectID)
	routeID := createRouteDecisionFixture(t, repo, tenantID, visibleProjectID, jobID, visibleDemandID)
	employeeID := uuid.New()
	stage := int32(1)

	_, err := repo.ReplaceProjectMembers(ctx, tenantID, visibleProjectID, []ProjectMemberInput{{
		PrincipalType: PrincipalTypeHumanUser,
		PrincipalID: actorID,
		ProjectRole: ProjectRoleObserver,
		DisplayNameSnapshot: strPtr("观察者"),
	}})
	require.NoError(t, err)
	_, err = repo.CreateProjectTaskGraph(ctx, CreateProjectTaskGraphRequest{
		TenantID: tenantID,
		ProjectID: visibleProjectID,
		DemandID: visibleDemandID,
		CoordinationJobID: jobID,
		RouteDecisionID: routeID,
		Tasks: []ProjectTaskGraphCreateTask{{
			Key: "root",
			Title: "定位问题",
			Status: "assigned",
			AssignedDigitalEmployeeID: employeeID,
			TaskKind: "analysis",
			StageIndex: &stage,
			RiskLevel: "medium",
			ExpectedOutputs: []any{"summary"},
			InputRequirements: map[string]any{},
			HandoffContract: map[string]any{},
			PlannerMetadata: map[string]any{},
		}},
	})
	require.NoError(t, err)

	items, err := repo.ListWorkflowInstances(ctx, ListWorkflowInstancesRequest{
		TenantID: tenantID,
		ActorUserID: actorID,
		Limit: 20,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, visibleDemandID, items[0].DemandID)
	require.NotEqual(t, hiddenDemandID, items[0].DemandID)
	require.Equal(t, int32(1), items[0].Progress.TotalNodes)
	require.Equal(t, int32(1), items[0].Progress.RunningNodes)
	require.Equal(t, WorkflowInstanceStatusRunning, items[0].Status)
	require.Equal(t, &jobID, items[0].SelectedCoordinationJobID)
}
```

- [ ] **Step 2: Run repository test and verify RED**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestListWorkflowInstancesFiltersVisibleDemandsAndSortsRunningFirst -count=1
```

Expected: FAIL because `PgRepository.ListWorkflowInstances` and the sqlc query do not exist.

- [ ] **Step 3: Add sqlc query**

Add this query to `apps/control-plane/internal/storage/queries/project.sql` after demand or task list queries:

```sql
-- name: ListWorkflowInstances :many
WITH visible_demands AS (
    SELECT
        d.id AS demand_id,
        d.project_id,
        p.name AS project_name,
        d.title,
        d.submitted_by_user_id,
        d.status AS demand_status,
        d.created_at,
        d.source_refs,
        COALESCE(d.updated_at, d.created_at) AS demand_updated_at
    FROM project_demands d
    JOIN projects p ON p.tenant_id = d.tenant_id AND p.id = d.project_id
    WHERE d.tenant_id = sqlc.arg('tenant_id')::uuid
      AND (sqlc.narg('project_id')::uuid IS NULL OR d.project_id = sqlc.narg('project_id')::uuid)
      AND (
        sqlc.narg('q')::text IS NULL
        OR d.title ILIKE '%' || sqlc.narg('q')::text || '%'
        OR COALESCE(d.content, '') ILIKE '%' || sqlc.narg('q')::text || '%'
        OR p.name ILIKE '%' || sqlc.narg('q')::text || '%'
      )
      AND (
        p.human_owner_user_id = sqlc.arg('actor_user_id')::uuid
        OR p.leader_user_id = sqlc.arg('actor_user_id')::uuid
        OR p.acceptance_user_id = sqlc.arg('actor_user_id')::uuid
        OR EXISTS (
          SELECT 1
          FROM project_members pm
          WHERE pm.tenant_id = p.tenant_id
            AND pm.project_id = p.id
            AND pm.principal_type = 'human_user'
            AND pm.principal_id = sqlc.arg('actor_user_id')::uuid
            AND pm.status = 'active'
        )
      )
),
task_counts AS (
    SELECT
        tenant_id,
        project_id,
        demand_id,
        COUNT(*)::int AS total_nodes,
        COUNT(*) FILTER (WHERE status IN ('completed', 'done', 'success'))::int AS completed_nodes,
        COUNT(*) FILTER (WHERE status IN ('assigned', 'running', 'in_progress'))::int AS running_nodes,
        COUNT(*) FILTER (WHERE status IN ('blocked'))::int AS blocked_nodes,
        COUNT(*) FILTER (WHERE requires_human_approval OR status IN ('waiting_human', 'pending_review'))::int AS waiting_human_nodes,
        COUNT(*) FILTER (WHERE status IN ('failed'))::int AS failed_nodes,
        COUNT(*) FILTER (WHERE status IN ('cancelled'))::int AS cancelled_nodes,
        MAX(updated_at) AS task_updated_at
    FROM project_tasks
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND demand_id IS NOT NULL
    GROUP BY tenant_id, project_id, demand_id
),
decision_counts AS (
    SELECT
        tenant_id,
        project_id,
        COUNT(*) FILTER (WHERE status_snapshot IN ('pending', 'requested'))::int AS pending_decisions,
        MAX(updated_at) AS decision_updated_at
    FROM project_decision_requests
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
    GROUP BY tenant_id, project_id
),
latest_jobs AS (
    SELECT DISTINCT ON (tenant_id, project_id, trigger_event_id)
        tenant_id,
        project_id,
        trigger_event_id,
        id AS selected_coordination_job_id,
        status AS job_status,
        GREATEST(COALESCE(finished_at, started_at), started_at, created_at) AS job_updated_at
    FROM project_coordination_jobs
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
    ORDER BY tenant_id, project_id, trigger_event_id, created_at DESC
)
SELECT
    vd.demand_id,
    vd.project_id,
    vd.project_name,
    vd.title,
    vd.submitted_by_user_id,
    COALESCE(NULLIF(vd.source_refs->>'submitted_by_display_name', ''), vd.submitted_by_user_id::text) AS submitted_by_display_name,
    CASE
      WHEN COALESCE(tc.cancelled_nodes, 0) > 0 OR vd.demand_status = 'cancelled' THEN 'cancelled'
      WHEN COALESCE(tc.failed_nodes, 0) > 0 THEN 'failed'
      WHEN COALESCE(dc.pending_decisions, 0) > 0 OR COALESCE(tc.waiting_human_nodes, 0) > 0 THEN 'waiting_human'
      WHEN COALESCE(tc.running_nodes, 0) > 0 THEN 'running'
      WHEN COALESCE(tc.total_nodes, 0) = 0 THEN 'planning'
      WHEN tc.completed_nodes = tc.total_nodes THEN 'completed'
      ELSE 'unknown'
    END::text AS status,
    CASE
      WHEN COALESCE(dc.pending_decisions, 0) > 0 THEN '等待人工决策'
      WHEN COALESCE(tc.failed_nodes, 0) > 0 THEN '存在失败任务'
      WHEN COALESCE(tc.running_nodes, 0) > 0 THEN '任务执行中'
      WHEN COALESCE(tc.total_nodes, 0) = 0 THEN '任务正在规划'
      ELSE ''
    END::text AS status_reason,
    vd.created_at,
    GREATEST(
      vd.demand_updated_at,
      COALESCE(tc.task_updated_at, vd.demand_updated_at),
      COALESCE(dc.decision_updated_at, vd.demand_updated_at),
      COALESCE(lj.job_updated_at, vd.demand_updated_at)
    ) AS updated_at,
    lj.selected_coordination_job_id,
    COALESCE(tc.total_nodes, 0)::int AS total_nodes,
    COALESCE(tc.completed_nodes, 0)::int AS completed_nodes,
    COALESCE(tc.running_nodes, 0)::int AS running_nodes,
    COALESCE(tc.blocked_nodes, 0)::int AS blocked_nodes,
    (COALESCE(tc.waiting_human_nodes, 0) + COALESCE(dc.pending_decisions, 0))::int AS waiting_human_nodes
FROM visible_demands vd
LEFT JOIN task_counts tc ON tc.project_id = vd.project_id AND tc.demand_id = vd.demand_id
LEFT JOIN decision_counts dc ON dc.project_id = vd.project_id
LEFT JOIN latest_jobs lj ON lj.project_id = vd.project_id
ORDER BY
    CASE
      WHEN COALESCE(tc.total_nodes, 0) = 0 THEN 1
      WHEN COALESCE(dc.pending_decisions, 0) > 0 OR COALESCE(tc.waiting_human_nodes, 0) > 0 THEN 3
      WHEN COALESCE(tc.failed_nodes, 0) > 0 THEN 4
      WHEN COALESCE(tc.running_nodes, 0) > 0 THEN 2
      WHEN tc.completed_nodes = tc.total_nodes THEN 5
      ELSE 6
    END ASC,
    updated_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
```

- [ ] **Step 4: Regenerate sqlc**

Run:

```bash
make -C apps/control-plane generate-sqlc
```

Expected: command exits 0 and generated query files include `ListWorkflowInstances`.

- [ ] **Step 5: Implement PgRepository mapping**

In `apps/control-plane/internal/project/pg_repository.go`, add:

```go
func (r *PgRepository) ListWorkflowInstances(ctx context.Context, req ListWorkflowInstancesRequest) ([]WorkflowInstanceSummary, error) {
	rows, err := r.q.ListWorkflowInstances(ctx, queries.ListWorkflowInstancesParams{
		TenantID: req.TenantID,
		ActorUserID: req.ActorUserID,
		ProjectID: nullUUID(req.ProjectID),
		Q: nullText(req.Query),
		Limit: req.Limit,
		Offset: req.Offset,
	})
	if err != nil {
		return nil, projectRepositoryError(err)
	}
	items := make([]WorkflowInstanceSummary, 0, len(rows))
	for _, row := range rows {
		status := WorkflowInstanceStatus(row.Status)
		items = append(items, WorkflowInstanceSummary{
			DemandID: row.DemandID,
			ProjectID: row.ProjectID,
			ProjectName: row.ProjectName,
			Title: row.Title,
			SubmittedByUserID: row.SubmittedByUserID,
			SubmittedByDisplayName: row.SubmittedByDisplayName,
			Status: status,
			StatusReason: row.StatusReason,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
			SelectedCoordinationJobID: ptrUUID(row.SelectedCoordinationJobID),
			Progress: WorkflowInstanceProgress{
				TotalNodes: int32(row.TotalNodes),
				CompletedNodes: int32(row.CompletedNodes),
				RunningNodes: int32(row.RunningNodes),
				BlockedNodes: int32(row.BlockedNodes),
				WaitingHumanNodes: int32(row.WaitingHumanNodes),
			},
		})
	}
	return items, nil
}
```

If generated row field names differ, map the generated names directly; keep JSON/domain field names unchanged.

- [ ] **Step 6: Run repository test and verify GREEN**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestListWorkflowInstancesFiltersVisibleDemandsAndSortsRunningFirst -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit repository slice**

Run:

```bash
git add apps/control-plane/internal/storage/queries/project.sql apps/control-plane/internal/storage/queries apps/control-plane/internal/project/pg_repository.go apps/control-plane/internal/project/pg_repository_test.go
git commit -m "feat: add workflow instance repository query"
```

Expected: commit succeeds and includes SQL, generated sqlc output, repository mapping, and repository tests.

## Task 3: HTTP Handler, Route, And Contract

**Files:**

- Modify: `apps/control-plane/internal/project/handler.go`
- Modify: `apps/control-plane/internal/project/handler_test.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/api/project_routes_test.go`
- Modify: `contracts/control-plane/openapi.yaml`
- Generated: `apps/control-plane/internal/api/gen/control_plane.gen.go` and related generated outputs

- [ ] **Step 1: Write failing handler test**

Add to `apps/control-plane/internal/project/handler_test.go`:

```go
func TestListWorkflowInstancesReturnsSummaries(t *testing.T) {
	tenantID := uuid.New()
	actorID := uuid.New()
	demandID := uuid.New()
	projectID := uuid.New()
	jobID := uuid.New()
	service := &handlerTestService{
		workflowInstances: []WorkflowInstanceSummary{{
			DemandID: demandID,
			ProjectID: projectID,
			ProjectName: "生产巡检",
			Title: "支付成功率下降",
			SubmittedByUserID: actorID,
			SubmittedByDisplayName: "张晓明",
			Status: WorkflowInstanceStatusRunning,
			StatusReason: "任务执行中",
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
			SelectedCoordinationJobID: &jobID,
			Progress: WorkflowInstanceProgress{TotalNodes: 2, RunningNodes: 1},
		}},
	}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflow-instances?status=running&limit=10&q=支付", nil)
	req = withConsoleContext(req, tenantID, actorID)
	resp := httptest.NewRecorder()

	handler.ListWorkflowInstances(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected workflow instances 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.workflowInstancesReq.TenantID != tenantID || service.workflowInstancesReq.ActorUserID != actorID || service.workflowInstancesReq.Query != "支付" || service.workflowInstancesReq.Limit != 10 {
		t.Fatalf("unexpected workflow instance request: %#v", service.workflowInstancesReq)
	}
	if service.workflowInstancesReq.Status == nil || *service.workflowInstancesReq.Status != WorkflowInstanceStatusRunning {
		t.Fatalf("expected running status filter, got %#v", service.workflowInstancesReq.Status)
	}
	var body []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body) != 1 || body[0]["demand_id"] != demandID.String() || body[0]["status"] != "running" {
		t.Fatalf("unexpected workflow instance body: %#v", body)
	}
	progress := body[0]["progress"].(map[string]any)
	if progress["total_nodes"].(float64) != 2 || progress["running_nodes"].(float64) != 1 {
		t.Fatalf("unexpected progress: %#v", progress)
	}
}
```

Add fields and method to `handlerTestService` in the same file:

```go
workflowInstances []WorkflowInstanceSummary
workflowInstancesReq ListWorkflowInstancesRequest

func (s *handlerTestService) ListWorkflowInstances(ctx context.Context, req ListWorkflowInstancesRequest) ([]WorkflowInstanceSummary, error) {
	s.workflowInstancesReq = req
	return s.workflowInstances, nil
}
```

- [ ] **Step 2: Run handler test and verify RED**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestListWorkflowInstancesReturnsSummaries -count=1
```

Expected: FAIL because the handler method and response structs do not exist.

- [ ] **Step 3: Add handler service method and handler**

In `apps/control-plane/internal/project/handler.go`, add `ListWorkflowInstances` to `HandlerService`:

```go
ListWorkflowInstances(ctx context.Context, req ListWorkflowInstancesRequest) ([]WorkflowInstanceSummary, error)
```

Add handler method:

```go
func (h *HTTPHandler) ListWorkflowInstances(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, ok := consoleIdentity(w, r)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	limit, offset, ok := paginationFromRequest(w, r)
	if !ok {
		return
	}
	req := ListWorkflowInstancesRequest{
		TenantID: tenantID,
		ActorUserID: actorID,
		Query: r.URL.Query().Get("q"),
		Limit: limit,
		Offset: offset,
	}
	if raw := r.URL.Query().Get("project_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			http.Error(w, "invalid project_id", http.StatusBadRequest)
			return
		}
		req.ProjectID = &id
	}
	if raw := r.URL.Query().Get("status"); raw != "" {
		status := WorkflowInstanceStatus(raw)
		req.Status = &status
	}
	items, err := service.ListWorkflowInstances(r.Context(), req)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workflowInstanceResponses(items))
}
```

- [ ] **Step 4: Add response structs and mappers**

In `apps/control-plane/internal/project/handler.go`, add near other response structs:

```go
type workflowInstanceResponse struct {
	DemandID string `json:"demand_id"`
	ProjectID string `json:"project_id"`
	ProjectName string `json:"project_name"`
	Title string `json:"title"`
	SubmittedByUserID string `json:"submitted_by_user_id"`
	SubmittedByDisplayName string `json:"submitted_by_display_name"`
	Status string `json:"status"`
	StatusReason string `json:"status_reason"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	SelectedCoordinationJobID *string `json:"selected_coordination_job_id,omitempty"`
	Progress workflowInstanceProgressResponse `json:"progress"`
	CurrentBlocker *workflowInstanceCurrentBlockerResponse `json:"current_blocker,omitempty"`
}

type workflowInstanceProgressResponse struct {
	TotalNodes int32 `json:"total_nodes"`
	CompletedNodes int32 `json:"completed_nodes"`
	RunningNodes int32 `json:"running_nodes"`
	BlockedNodes int32 `json:"blocked_nodes"`
	WaitingHumanNodes int32 `json:"waiting_human_nodes"`
}

type workflowInstanceCurrentBlockerResponse struct {
	Type string `json:"type"`
	Title string `json:"title"`
	ResourceID *string `json:"resource_id,omitempty"`
}
```

Add mapper:

```go
func workflowInstanceResponses(items []WorkflowInstanceSummary) []workflowInstanceResponse {
	responses := make([]workflowInstanceResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, workflowInstanceResponseFromDomain(item))
	}
	return responses
}

func workflowInstanceResponseFromDomain(item WorkflowInstanceSummary) workflowInstanceResponse {
	var jobID *string
	if item.SelectedCoordinationJobID != nil {
		jobID = strPtr(item.SelectedCoordinationJobID.String())
	}
	var blocker *workflowInstanceCurrentBlockerResponse
	if item.CurrentBlocker != nil {
		var resourceID *string
		if item.CurrentBlocker.ResourceID != nil {
			resourceID = strPtr(item.CurrentBlocker.ResourceID.String())
		}
		blocker = &workflowInstanceCurrentBlockerResponse{
			Type: item.CurrentBlocker.Type,
			Title: item.CurrentBlocker.Title,
			ResourceID: resourceID,
		}
	}
	return workflowInstanceResponse{
		DemandID: item.DemandID.String(),
		ProjectID: item.ProjectID.String(),
		ProjectName: item.ProjectName,
		Title: item.Title,
		SubmittedByUserID: item.SubmittedByUserID.String(),
		SubmittedByDisplayName: item.SubmittedByDisplayName,
		Status: string(item.Status),
		StatusReason: item.StatusReason,
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
		SelectedCoordinationJobID: jobID,
		Progress: workflowInstanceProgressResponse{
			TotalNodes: item.Progress.TotalNodes,
			CompletedNodes: item.Progress.CompletedNodes,
			RunningNodes: item.Progress.RunningNodes,
			BlockedNodes: item.Progress.BlockedNodes,
			WaitingHumanNodes: item.Progress.WaitingHumanNodes,
		},
		CurrentBlocker: blocker,
	}
}
```

- [ ] **Step 5: Run handler test and verify GREEN**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestListWorkflowInstancesReturnsSummaries -count=1
```

Expected: PASS.

- [ ] **Step 6: Write failing route test**

In `apps/control-plane/internal/api/project_routes_test.go`, add a workflow instance request in `TestProjectRoutesUseConsoleAuthAndProjectService` after the project list route:

```go
workflowReq := httptest.NewRequest(http.MethodGet, "/api/v1/workflow-instances?status=running&limit=9&q=支付", nil)
workflowReq.AddCookie(cookie)
workflowResp := httptest.NewRecorder()
server.ServeHTTP(workflowResp, workflowReq)
if workflowResp.Code != http.StatusOK {
	t.Fatalf("expected workflow instances route to succeed, got %d: %s", workflowResp.Code, workflowResp.Body.String())
}
if service.workflowInstancesReq.TenantID != expectedTenantID || service.workflowInstancesReq.ActorUserID != user.ID || service.workflowInstancesReq.Query != "支付" || service.workflowInstancesReq.Limit != 9 {
	t.Fatalf("expected workflow instances context/query from route, got %#v", service.workflowInstancesReq)
}
```

Add `workflowInstancesReq project.ListWorkflowInstancesRequest` and the method to `routeProjectService`:

```go
func (s *routeProjectService) ListWorkflowInstances(ctx context.Context, req project.ListWorkflowInstancesRequest) ([]project.WorkflowInstanceSummary, error) {
	s.workflowInstancesReq = req
	return []project.WorkflowInstanceSummary{}, nil
}
```

- [ ] **Step 7: Run route test and verify RED**

Run:

```bash
go test ./apps/control-plane/internal/api -run TestProjectRoutesUseConsoleAuthAndProjectService -count=1
```

Expected: FAIL with 404 for `/api/v1/workflow-instances`.

- [ ] **Step 8: Register route**

In `apps/control-plane/internal/api/server.go`, inside the console-auth project handler route group, add:

```go
r.Get("/workflow-instances", s.projectHandler.ListWorkflowInstances)
```

Place it next to `/projects` and `/project-demands/{demandId}/launch-detail` because it is a cross-project read endpoint.

- [ ] **Step 9: Run route test and verify GREEN**

Run:

```bash
go test ./apps/control-plane/internal/api -run TestProjectRoutesUseConsoleAuthAndProjectService -count=1
```

Expected: PASS.

- [ ] **Step 10: Update OpenAPI contract**

In `contracts/control-plane/openapi.yaml`, add path:

```yaml
  /api/v1/workflow-instances:
    get:
      operationId: listWorkflowInstances
      summary: List visible workflow instances
      parameters:
        - name: q
          in: query
          schema:
            type: string
        - name: project_id
          in: query
          schema:
            type: string
            format: uuid
        - name: status
          in: query
          schema:
            $ref: "#/components/schemas/WorkflowInstanceStatus"
        - $ref: "#/components/parameters/Limit"
        - $ref: "#/components/parameters/Offset"
      responses:
        "200":
          description: Visible workflow instances
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: "#/components/schemas/WorkflowInstanceSummary"
        "400":
          $ref: "#/components/responses/Error"
```

Add schemas:

```yaml
    WorkflowInstanceStatus:
      type: string
      enum:
        - planning
        - running
        - waiting_human
        - failed
        - completed
        - cancelled
        - unknown
    WorkflowInstanceProgress:
      type: object
      required:
        - total_nodes
        - completed_nodes
        - running_nodes
        - blocked_nodes
        - waiting_human_nodes
      properties:
        total_nodes:
          type: integer
          format: int32
        completed_nodes:
          type: integer
          format: int32
        running_nodes:
          type: integer
          format: int32
        blocked_nodes:
          type: integer
          format: int32
        waiting_human_nodes:
          type: integer
          format: int32
    WorkflowInstanceCurrentBlocker:
      type: object
      required:
        - type
        - title
      properties:
        type:
          type: string
        title:
          type: string
        resource_id:
          type: string
          format: uuid
    WorkflowInstanceSummary:
      type: object
      required:
        - demand_id
        - project_id
        - project_name
        - title
        - submitted_by_user_id
        - submitted_by_display_name
        - status
        - status_reason
        - created_at
        - updated_at
        - progress
      properties:
        demand_id:
          type: string
          format: uuid
        project_id:
          type: string
          format: uuid
        project_name:
          type: string
        title:
          type: string
        submitted_by_user_id:
          type: string
          format: uuid
        submitted_by_display_name:
          type: string
        status:
          $ref: "#/components/schemas/WorkflowInstanceStatus"
        status_reason:
          type: string
        created_at:
          type: string
          format: date-time
        updated_at:
          type: string
          format: date-time
        selected_coordination_job_id:
          type: string
          format: uuid
        progress:
          $ref: "#/components/schemas/WorkflowInstanceProgress"
        current_blocker:
          $ref: "#/components/schemas/WorkflowInstanceCurrentBlocker"
```

- [ ] **Step 11: Regenerate contracts**

Run:

```bash
pnpm generate:control-plane
pnpm verify:contracts
```

Expected: both commands exit 0; generated OpenAPI server/client files include workflow instance models.

- [ ] **Step 12: Run backend focused tests**

Run:

```bash
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/api -run 'TestListWorkflowInstances|TestProjectRoutesUseConsoleAuthAndProjectService' -count=1
```

Expected: PASS.

- [ ] **Step 13: Commit handler and contract slice**

Run:

```bash
git add apps/control-plane/internal/project/handler.go apps/control-plane/internal/project/handler_test.go apps/control-plane/internal/api/server.go apps/control-plane/internal/api/project_routes_test.go contracts/control-plane/openapi.yaml apps/control-plane/internal/api/gen apps/control-plane/gen apps/web/src/lib/api/generated
git commit -m "feat: expose workflow instance list API"
```

Expected: commit succeeds and includes route, handler, OpenAPI, and generated outputs.

## Task 4: Final Backend Verification

**Files:**

- No source changes expected.

- [ ] **Step 1: Run project package tests**

Run:

```bash
go test ./apps/control-plane/internal/project -count=1
```

Expected: PASS.

- [ ] **Step 2: Run API package tests**

Run:

```bash
go test ./apps/control-plane/internal/api -count=1
```

Expected: PASS.

- [ ] **Step 3: Run contract verification**

Run:

```bash
pnpm verify:contracts
```

Expected: PASS.

- [ ] **Step 4: Run whitespace check**

Run:

```bash
git diff --check
```

Expected: no output.

- [ ] **Step 5: Record backend status**

In the implementation notes or final task summary, record:

```text
Backend local verification:
- go test ./apps/control-plane/internal/project -count=1
- go test ./apps/control-plane/internal/api -count=1
- pnpm verify:contracts
- git diff --check

Real-chain status:
- This subplan only exposes the API. Full browser/API smoke is required after the Web workflow page is implemented.
```
