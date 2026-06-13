# ProjectTask Runtime Dispatch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Connect project coordination `DispatchProjectTask` to the real digital employee run execution chain and bind each dispatched ProjectTask to the created runtime run, with retry-safe, orphan-free, idempotent semantics.

**Architecture:** `projectcoordination.ProjectStore` remains the activity-backed project coordination store, but it receives a narrow `ProjectTaskRunStarter` dependency. Dispatch validates the ProjectTask, starts a `DigitalEmployeeRun` through the existing run service adapter, then performs side effects in a **failure-safe order**:

1. Start the run (idempotent on `IdempotencyKey = project-task:{task_id}`).
2. **Bind the task first** (`project_tasks.digital_employee_run_id/runtime_task_id`, status → `assigned`). The binding is the durable state that enables runtime writeback, so it must commit before any non-essential side effect.
3. Append the `project_task.dispatched` event. If the append fails the activity retries, and the bound-task short-circuit re-emits the event idempotently (guarded by `ProjectTaskEventExists`), so the event is delivered **exactly once** without orphaning the run.

On run-start failure, dispatch records exactly one `project_task.dispatch_failed` event whose `retryable` flag is **classified** (validation = terminal, runtime-unavailable / active-run-in-flight = transient). Terminal failures are wrapped as non-retryable Temporal application errors so they are not retried, keeping failure events bounded. The workflow dispatch loop continues past a single task's dispatch failure instead of aborting its siblings.

**Tech Stack:** Go, Temporal activities, pgx/sqlc, chi/net/http service stack, existing `employee.DigitalEmployeeRunService`, Go tests with memory fakes.

---

## File Structure

- Modify `apps/control-plane/internal/workflow/projectcoordination/types.go`
  - Add `StartProjectTaskRunRequest`, `StartProjectTaskRunResult`.
  - Add `ProjectTaskRunStartError` (carries retryable classification from the run starter without coupling the store to the `employee` package).
- Modify `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
  - Add `ProjectTaskRunStarter` interface.
  - Add constructor overload accepting run starter.
  - Replace `DispatchProjectTask` status-only behavior with validate → start run → bind run → emit dispatched event, in that order; the bound-task short-circuit re-emits the event idempotently so it is delivered exactly once.
  - Add helpers: dispatch prompt, idempotency key, failure payload (parameterized `retryable`), `reemittedDispatchedPayload` (lean payload for the replay path), and `dispatchErrorRetryable` classification (shared by the store and the activity wrapper).
- Modify `apps/control-plane/internal/workflow/projectcoordination/activities.go`
  - Wrap terminal (`dispatchErrorRetryable == false`) dispatch errors as `temporal.NewNonRetryableApplicationError` so they are recorded once and not retried.
- Modify `apps/control-plane/internal/workflow/projectcoordination/workflow.go`
  - `dispatchProjectTasks` continues past a per-task dispatch failure (logs it) instead of aborting the whole batch.
- Modify `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`
  - Red/green unit tests for successful dispatch (new ordering), retryable run-start failure, terminal run-start failure, idempotent same-run replay, and the `dispatchErrorRetryable` classification.
- Modify `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`
  - Extend `recordingActivityStore` with a configurable dispatch error; add activity-level tests that terminal errors become non-retryable while transient errors stay retryable. Existing call-sequence assertions remain valid because success-path dispatch still calls every task.
- Modify `apps/control-plane/internal/project/types.go`
  - Add `BindProjectTaskRunRequest`.
  - Add `ProjectEventTaskDispatchFailed` constant.
  - Add `ErrProjectConflict` sentinel (used by run-binding conflict semantics in both the real repository and the fakes).
- Modify `apps/control-plane/internal/project/repository.go`
  - Add `BindProjectTaskRun` and `ProjectTaskEventExists` to `Repository`.
- Modify `apps/control-plane/internal/storage/queries/project.sql`
  - Add `BindProjectTaskRun` and `ProjectTaskEventExists` queries.
- Regenerate sqlc output under `apps/control-plane/internal/storage/queries/`
  - Command: `make -C apps/control-plane generate-sqlc`
- Modify `apps/control-plane/internal/project/pg_repository.go`
  - Implement `BindProjectTaskRun`, re-reading on `pgx.ErrNoRows` to return `ErrProjectConflict` (bound to a different run / non-dispatchable state) vs `ErrProjectNotFound`.
  - Implement `ProjectTaskEventExists` for the dispatched-event idempotency check.
- Modify `apps/control-plane/internal/project/service_test.go`
  - Extend memory repository with `BindProjectTaskRun`.
  - Add an integration-style test that dispatch binding enables `CompleteProjectTask`.
- Modify `apps/control-plane/internal/app/app.go`
  - Create a ProjectTask run starter adapter around `employee.DigitalEmployeeRunService` that classifies run-start failures into terminal vs transient (`ProjectTaskRunStartError`).
  - Move `auditService` + run service construction above the Temporal block and inject the adapter into `projectcoordination.NewProjectStoreWithApprovalsInboxAndRunStarter`.
- Modify `CHANGELOG.md`
  - Add a dated backend entry after implementation using `TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'`.

---

### Task 1: Add ProjectStore Dispatch Red Tests

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/types.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`

- [ ] **Step 1: Add run starter result/request types and the classification error**

In `apps/control-plane/internal/workflow/projectcoordination/types.go`, after `DispatchProjectTaskInput`, add:

```go
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
```

- [ ] **Step 2: Add fake run starter and richer memory repository**

In `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`, replace the `projectStoreMemoryRepository` struct definition (keep the existing fields) with:

```go
type projectStoreMemoryRepository struct {
	project.Repository

	projectRecord project.Project
	demand        project.ProjectDemand
	members       []project.ProjectMember
	tasks         []project.ProjectTask
	approvalID    uuid.UUID

	bindRequests     []project.BindProjectTaskRunRequest
	bindErr          error
	events           []project.ProjectEvent
	decisionRequests []project.DecisionRequest
}
```

Add these methods near the existing `CreateProjectTask` and `UpdateProjectTaskStatus` fakes:

```go
func (r *projectStoreMemoryRepository) GetProjectTask(ctx context.Context, tenantID, projectTaskID uuid.UUID) (project.ProjectTask, error) {
	for _, task := range r.tasks {
		if task.TenantID == tenantID && task.ID == projectTaskID {
			return task, nil
		}
	}
	return project.ProjectTask{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) BindProjectTaskRun(ctx context.Context, req project.BindProjectTaskRunRequest) (project.ProjectTask, error) {
	r.bindRequests = append(r.bindRequests, req)
	if r.bindErr != nil {
		return project.ProjectTask{}, r.bindErr
	}
	for i, task := range r.tasks {
		if task.TenantID != req.TenantID || task.ID != req.ProjectTaskID {
			continue
		}
		if task.DigitalEmployeeRunID != nil && *task.DigitalEmployeeRunID != req.DigitalEmployeeRunID {
			return project.ProjectTask{}, project.ErrProjectConflict
		}
		task.Status = "assigned"
		task.DigitalEmployeeRunID = &req.DigitalEmployeeRunID
		task.RuntimeTaskID = &req.RuntimeTaskID
		task.UpdatedAt = time.Now().UTC()
		r.tasks[i] = task
		return task, nil
	}
	return project.ProjectTask{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) ProjectTaskEventExists(ctx context.Context, tenantID, projectID uuid.UUID, eventType project.ProjectEventType, actorID string) (bool, error) {
	for _, event := range r.events {
		if event.TenantID == tenantID && event.ProjectID == projectID && event.EventType == eventType && event.ActorID == actorID {
			return true, nil
		}
	}
	return false, nil
}
```

Also update the existing fake `AppendProjectEvent` in this file so it records `ActorID` (the dispatched-event existence check matches on it); add `ActorID: req.ActorID` to the `project.ProjectEvent` it appends:

```go
func (r *projectStoreMemoryRepository) AppendProjectEvent(ctx context.Context, req project.AppendProjectEventRequest) (project.ProjectEvent, error) {
	event := project.ProjectEvent{ID: uuid.New(), TenantID: req.TenantID, ProjectID: req.ProjectID, EventType: req.EventType, ActorID: req.ActorID, Payload: req.Payload, CreatedAt: time.Now().UTC()}
	r.events = append(r.events, event)
	return event, nil
}
```

Add a fake starter:

```go
type projectTaskRunStarterFake struct {
	requests []StartProjectTaskRunRequest
	result   StartProjectTaskRunResult
	err      error
}

func (f *projectTaskRunStarterFake) StartProjectTaskRun(ctx context.Context, req StartProjectTaskRunRequest) (StartProjectTaskRunResult, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return StartProjectTaskRunResult{}, f.err
	}
	return f.result, nil
}
```

- [ ] **Step 3: Write successful dispatch test**

Add this test to `project_store_test.go`:

```go
func TestProjectStoreDispatchProjectTaskStartsRunAndBindsTask(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	ownerID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID},
		demand: project.ProjectDemand{
			ID:        demandID,
			TenantID:  tenantID,
			ProjectID: projectID,
			Title:     "检查上线证据",
			Content:   stringPtr("需要确认测试报告和回滚方案。"),
		},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "整理证据",
			Summary:                   stringPtr("输出证据清单"),
			Status:                    "planned",
			AssignedDigitalEmployeeID: &employeeID,
		}},
	}
	starter := &projectTaskRunStarterFake{result: StartProjectTaskRunResult{
		RunID:         runID,
		RuntimeTaskID: runtimeTaskID,
		RuntimeNodeID: runtimeNodeID,
		NodeID:        "node-1",
	}}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	if err != nil {
		t.Fatalf("dispatch project task: %v", err)
	}
	if len(starter.requests) != 1 {
		t.Fatalf("expected one run start request, got %d", len(starter.requests))
	}
	req := starter.requests[0]
	if req.DispatchUserID != ownerID || req.DigitalEmployeeID != employeeID || req.IdempotencyKey != "project-task:"+taskID.String() {
		t.Fatalf("unexpected run start request: %#v", req)
	}
	if !strings.Contains(req.Prompt, "需要确认测试报告") || !strings.Contains(req.Prompt, taskID.String()) {
		t.Fatalf("expected prompt to include demand content and task id, got %q", req.Prompt)
	}
	if len(repo.bindRequests) != 1 || repo.bindRequests[0].DigitalEmployeeRunID != runID || repo.bindRequests[0].RuntimeTaskID != runtimeTaskID {
		t.Fatalf("expected bind request, got %#v", repo.bindRequests)
	}
	if repo.tasks[0].Status != "assigned" || repo.tasks[0].DigitalEmployeeRunID == nil || *repo.tasks[0].DigitalEmployeeRunID != runID {
		t.Fatalf("expected assigned bound task, got %#v", repo.tasks[0])
	}
	if len(repo.events) != 1 || repo.events[0].EventType != project.ProjectEventTaskDispatched {
		t.Fatalf("expected dispatched event, got %#v", repo.events)
	}
	if repo.events[0].Payload["digital_employee_run_id"] != runID.String() || repo.events[0].Payload["runtime_task_id"] != runtimeTaskID.String() {
		t.Fatalf("expected run binding payload, got %#v", repo.events[0].Payload)
	}
}
```

Update the `project_store_test.go` import block to include `strings`:

```go
import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/approval"
	"github.com/superteam/control-plane/internal/project"
)
```

Add this helper if it is not already present anywhere in the `projectcoordination` test package (it is not at the time of writing):

```go
func stringPtr(value string) *string {
	return &value
}
```

- [ ] **Step 4: Write failure, terminal, and idempotency tests**

Add:

```go
func TestProjectStoreDispatchProjectTaskRunStartFailureKeepsTaskPlanned(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "需求"},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    "planned",
			AssignedDigitalEmployeeID: &employeeID,
		}},
	}
	// Plain error => default to retryable.
	starter := &projectTaskRunStarterFake{err: errors.New("runtime node is not connected")}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	if err == nil {
		t.Fatal("expected dispatch error")
	}
	if repo.tasks[0].Status != "planned" || len(repo.bindRequests) != 0 {
		t.Fatalf("expected planned unbound task, task=%#v binds=%#v", repo.tasks[0], repo.bindRequests)
	}
	if len(repo.events) != 1 || repo.events[0].EventType != project.ProjectEventTaskDispatchFailed {
		t.Fatalf("expected dispatch failed event, got %#v", repo.events)
	}
	if repo.events[0].Payload["retryable"] != true {
		t.Fatalf("expected retryable failure payload, got %#v", repo.events[0].Payload)
	}
}

func TestProjectStoreDispatchProjectTaskTerminalRunStartFailureMarksNonRetryable(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "需求"},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    "planned",
			AssignedDigitalEmployeeID: &employeeID,
		}},
	}
	starter := &projectTaskRunStarterFake{err: &ProjectTaskRunStartError{Retryable: false, Err: errors.New("invalid run input")}}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	if err == nil {
		t.Fatal("expected dispatch error")
	}
	if len(repo.events) != 1 || repo.events[0].Payload["retryable"] != false {
		t.Fatalf("expected non-retryable failure payload, got %#v", repo.events)
	}
	if repo.tasks[0].Status != "planned" || len(repo.bindRequests) != 0 {
		t.Fatalf("expected planned unbound task, task=%#v binds=%#v", repo.tasks[0], repo.bindRequests)
	}
}

func TestProjectStoreDispatchProjectTaskAlreadyBoundSameRunIsIdempotent(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "需求"},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    "assigned",
			AssignedDigitalEmployeeID: &employeeID,
			DigitalEmployeeRunID:      &runID,
			RuntimeTaskID:             &runtimeTaskID,
		}},
	}
	// The dispatched event already exists, so the idempotent replay must be a pure no-op.
	repo.events = append(repo.events, project.ProjectEvent{TenantID: tenantID, ProjectID: projectID, EventType: project.ProjectEventTaskDispatched, ActorID: taskID.String()})
	starter := &projectTaskRunStarterFake{}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	if err != nil {
		t.Fatalf("expected idempotent success, got %v", err)
	}
	if len(starter.requests) != 0 || len(repo.bindRequests) != 0 || len(repo.events) != 1 {
		t.Fatalf("expected no duplicate side effects, starts=%d binds=%d events=%d", len(starter.requests), len(repo.bindRequests), len(repo.events))
	}
}

func TestProjectStoreDispatchProjectTaskReemitsMissingDispatchedEvent(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	ownerID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "需求"},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    "assigned",
			AssignedDigitalEmployeeID: &employeeID,
			DigitalEmployeeRunID:      &runID,
			RuntimeTaskID:             &runtimeTaskID,
		}},
	}
	// Task is bound but the dispatched event is missing (e.g. a prior attempt crashed
	// after binding); dispatch must re-emit exactly one event without restarting the run.
	starter := &projectTaskRunStarterFake{}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	if err != nil {
		t.Fatalf("expected idempotent success, got %v", err)
	}
	if len(starter.requests) != 0 || len(repo.bindRequests) != 0 {
		t.Fatalf("expected no run start or bind, starts=%d binds=%d", len(starter.requests), len(repo.bindRequests))
	}
	if len(repo.events) != 1 || repo.events[0].EventType != project.ProjectEventTaskDispatched || repo.events[0].Payload["reemitted"] != true {
		t.Fatalf("expected one re-emitted dispatched event, got %#v", repo.events)
	}
	if repo.events[0].Payload["digital_employee_run_id"] != runID.String() {
		t.Fatalf("expected re-emitted payload to carry run id, got %#v", repo.events[0].Payload)
	}
}

func TestDispatchErrorRetryableClassification(t *testing.T) {
	if dispatchErrorRetryable(project.ErrInvalidProject) {
		t.Fatal("expected ErrInvalidProject to be terminal")
	}
	if dispatchErrorRetryable(project.ErrProjectNotFound) {
		t.Fatal("expected ErrProjectNotFound to be terminal")
	}
	if dispatchErrorRetryable(project.ErrProjectConflict) {
		t.Fatal("expected ErrProjectConflict to be terminal")
	}
	if dispatchErrorRetryable(&ProjectTaskRunStartError{Retryable: false, Err: errors.New("x")}) {
		t.Fatal("expected non-retryable start error to be terminal")
	}
	if !dispatchErrorRetryable(&ProjectTaskRunStartError{Retryable: true, Err: errors.New("x")}) {
		t.Fatal("expected retryable start error to be transient")
	}
	if !dispatchErrorRetryable(errors.New("db timeout")) {
		t.Fatal("expected unknown error to default to transient")
	}
}
```

- [ ] **Step 5: Run tests to verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectStoreDispatchProjectTask|TestDispatchErrorRetryableClassification' -count=1
```

Expected: FAIL because `NewProjectStoreWithApprovalsInboxAndRunStarter`, `dispatchErrorRetryable`, `project.BindProjectTaskRunRequest`, `project.ErrProjectConflict`, and `project.ProjectEventTaskDispatchFailed` are not defined yet.

- [ ] **Step 6: Commit red tests only**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/types.go apps/control-plane/internal/workflow/projectcoordination/project_store_test.go
git commit -m "test: cover project task runtime dispatch bridge"
```

---

### Task 2: Add ProjectTask Run Binding Repository Contract

**Files:**
- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/storage/queries/project.sql`
- Generated: `apps/control-plane/internal/storage/queries/project.sql.go`
- Generated: `apps/control-plane/internal/storage/queries/querier.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/service_test.go`

- [ ] **Step 1: Add error sentinel, event type, and bind request type**

In `apps/control-plane/internal/project/types.go`, add the conflict sentinel to the existing error block:

```go
ErrProjectConflict          = errors.New("project conflict")
```

Add the event constant next to `ProjectEventTaskDispatched`:

```go
ProjectEventTaskDispatchFailed     ProjectEventType = "project_task.dispatch_failed"
```

Add this type near `CreateProjectTaskRequest`:

```go
type BindProjectTaskRunRequest struct {
	TenantID             uuid.UUID
	ProjectTaskID        uuid.UUID
	DigitalEmployeeRunID uuid.UUID
	RuntimeTaskID        uuid.UUID
	LatestEventID        *uuid.UUID
	CurrentStatuses      []string
}
```

- [ ] **Step 2: Add repository method**

In `apps/control-plane/internal/project/repository.go`, add to `Repository` after `UpdateProjectTaskStatus`:

```go
BindProjectTaskRun(ctx context.Context, req BindProjectTaskRunRequest) (ProjectTask, error)
ProjectTaskEventExists(ctx context.Context, tenantID, projectID uuid.UUID, eventType ProjectEventType, actorID string) (bool, error)
```

- [ ] **Step 3: Add sqlc query**

In `apps/control-plane/internal/storage/queries/project.sql`, after `UpdateProjectTaskStatus`, add:

```sql
-- name: BindProjectTaskRun :one
UPDATE project_tasks
SET status = 'assigned',
    runtime_task_id = sqlc.arg('runtime_task_id')::uuid,
    digital_employee_run_id = sqlc.arg('digital_employee_run_id')::uuid,
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND (
      status = ANY(sqlc.arg('current_statuses')::varchar[])
      OR (
          status = 'assigned'
          AND runtime_task_id = sqlc.arg('runtime_task_id')::uuid
          AND digital_employee_run_id = sqlc.arg('digital_employee_run_id')::uuid
      )
  )
  AND (
      digital_employee_run_id IS NULL
      OR digital_employee_run_id = sqlc.arg('digital_employee_run_id')::uuid
  )
RETURNING *;

-- name: ProjectTaskEventExists :one
SELECT EXISTS (
    SELECT 1 FROM project_events
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND project_id = sqlc.arg('project_id')::uuid
      AND event_type = sqlc.arg('event_type')::varchar
      AND actor_id = sqlc.arg('actor_id')::varchar
) AS event_exists;
```

Confirm the `project_events` column names (`event_type`, `actor_id`) against the actual table before running sqlc; adjust the query if they differ. The dispatched event is written with `actor_id = task_id` (see `coordinatorEvent`), so the existence check matches on `actor_id`.

Note on conflict semantics: the same-run, already-`assigned` branch makes this query idempotent for retries; a genuine no-match (task missing, bound to a different run, or in a non-dispatchable status) returns zero rows, which the PgRepository translates in Step 5.

- [ ] **Step 4: Regenerate sqlc**

Run:

```bash
make -C apps/control-plane generate-sqlc
```

Expected: PASS and generated `BindProjectTaskRun` method appears in `apps/control-plane/internal/storage/queries/project.sql.go` and `querier.go`.

- [ ] **Step 5: Implement PgRepository method with conflict distinction**

In `apps/control-plane/internal/project/pg_repository.go`, after `UpdateProjectTaskStatus`, add (the file already imports `errors` and `github.com/jackc/pgx/v5`):

```go
func (r *PgRepository) BindProjectTaskRun(ctx context.Context, req BindProjectTaskRunRequest) (ProjectTask, error) {
	row, err := r.q.BindProjectTaskRun(ctx, queries.BindProjectTaskRunParams{
		TenantID:             req.TenantID,
		ID:                   req.ProjectTaskID,
		RuntimeTaskID:        req.RuntimeTaskID,
		DigitalEmployeeRunID: req.DigitalEmployeeRunID,
		LatestEventID:        nullUUID(req.LatestEventID),
		CurrentStatuses:      req.CurrentStatuses,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return r.bindProjectTaskRunConflict(ctx, req)
		}
		return ProjectTask{}, err
	}
	return taskFromRecord(row), nil
}

// bindProjectTaskRunConflict distinguishes a missing task from a real binding
// conflict (task is bound to a different run, or is in a non-dispatchable state).
func (r *PgRepository) bindProjectTaskRunConflict(ctx context.Context, req BindProjectTaskRunRequest) (ProjectTask, error) {
	existing, err := r.q.GetProjectTask(ctx, queries.GetProjectTaskParams{TenantID: req.TenantID, ID: req.ProjectTaskID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProjectTask{}, ErrProjectNotFound
		}
		return ProjectTask{}, err
	}
	task := taskFromRecord(existing)
	if task.DigitalEmployeeRunID != nil && *task.DigitalEmployeeRunID == req.DigitalEmployeeRunID {
		// Already bound to the same run by a prior attempt; treat as idempotent success.
		return task, nil
	}
	return ProjectTask{}, ErrProjectConflict
}

func (r *PgRepository) ProjectTaskEventExists(ctx context.Context, tenantID, projectID uuid.UUID, eventType ProjectEventType, actorID string) (bool, error) {
	return r.q.ProjectTaskEventExists(ctx, queries.ProjectTaskEventExistsParams{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: string(eventType),
		ActorID:   actorID,
	})
}
```

- [ ] **Step 6: Add BindProjectTaskRun to project service memory repository**

In `apps/control-plane/internal/project/service_test.go`, add to `memoryRepository`:

```go
func (r *memoryRepository) BindProjectTaskRun(ctx context.Context, req BindProjectTaskRunRequest) (ProjectTask, error) {
	for i, task := range r.tasks {
		if task.TenantID != req.TenantID || task.ID != req.ProjectTaskID {
			continue
		}
		if task.DigitalEmployeeRunID != nil && *task.DigitalEmployeeRunID != req.DigitalEmployeeRunID {
			return ProjectTask{}, ErrProjectConflict
		}
		allowed := false
		for _, status := range req.CurrentStatuses {
			if task.Status == status {
				allowed = true
				break
			}
		}
		if !allowed && !(task.Status == "assigned" && task.DigitalEmployeeRunID != nil && *task.DigitalEmployeeRunID == req.DigitalEmployeeRunID) {
			return ProjectTask{}, ErrProjectConflict
		}
		task.Status = "assigned"
		task.DigitalEmployeeRunID = &req.DigitalEmployeeRunID
		task.RuntimeTaskID = &req.RuntimeTaskID
		task.UpdatedAt = time.Now().UTC()
		r.tasks[i] = task
		return task, nil
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) ProjectTaskEventExists(ctx context.Context, tenantID, projectID uuid.UUID, eventType ProjectEventType, actorID string) (bool, error) {
	for _, event := range r.events {
		if event.TenantID == tenantID && event.ProjectID == projectID && event.EventType == eventType && event.ActorID == actorID {
			return true, nil
		}
	}
	return false, nil
}
```

If `memoryRepository.AppendProjectEvent` does not already persist the appended `ProjectEvent` (with `ActorID`) into `r.events`, update it to do so, mirroring how the existing tests seed `r.events`.

- [ ] **Step 7: Run compile-focused tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestCompleteProjectTaskWritesSummaryAndSignalsCoordinator -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit repository contract**

```bash
git add apps/control-plane/internal/project/types.go apps/control-plane/internal/project/repository.go apps/control-plane/internal/storage/queries/project.sql apps/control-plane/internal/storage/queries/project.sql.go apps/control-plane/internal/storage/queries/querier.go apps/control-plane/internal/project/pg_repository.go apps/control-plane/internal/project/service_test.go
git commit -m "feat: add project task run binding"
```

---

### Task 3: Implement ProjectStore Dispatch Bridge

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`

- [ ] **Step 1: Add interface and constructor overload**

In `project_store.go`, update the struct and constructors:

```go
type ProjectStore struct {
	repository project.Repository
	approvals  ApprovalCreator
	inbox      project.DecisionInboxProjector
	runStarter ProjectTaskRunStarter
}

type ProjectTaskRunStarter interface {
	StartProjectTaskRun(ctx context.Context, req StartProjectTaskRunRequest) (StartProjectTaskRunResult, error)
}

func NewProjectStoreWithApprovalsAndInbox(repository project.Repository, approvals ApprovalCreator, inbox project.DecisionInboxProjector) *ProjectStore {
	return NewProjectStoreWithApprovalsInboxAndRunStarter(repository, approvals, inbox, nil)
}

func NewProjectStoreWithApprovalsInboxAndRunStarter(repository project.Repository, approvals ApprovalCreator, inbox project.DecisionInboxProjector, runStarter ProjectTaskRunStarter) *ProjectStore {
	return &ProjectStore{repository: repository, approvals: approvals, inbox: inbox, runStarter: runStarter}
}
```

- [ ] **Step 2: Add dispatch helpers**

In `project_store.go`, near `DispatchProjectTask`, add (the file already imports `context` and `github.com/google/uuid`; add `errors`):

```go
func projectTaskDispatchAllowed(status string) bool {
	return status == "planned" || status == "pending"
}

func projectTaskDispatchIdempotencyKey(taskID uuid.UUID) string {
	return "project-task:" + taskID.String()
}

func projectTaskRunPrompt(projectRecord project.Project, demand project.ProjectDemand, task project.ProjectTask) string {
	content := ""
	if demand.Content != nil {
		content = *demand.Content
	}
	summary := ""
	if task.Summary != nil {
		summary = *task.Summary
	}
	return "项目任务执行请求\n" +
		"项目ID: " + projectRecord.ID.String() + "\n" +
		"需求ID: " + demand.ID.String() + "\n" +
		"ProjectTask ID: " + task.ID.String() + "\n" +
		"需求标题: " + demand.Title + "\n" +
		"需求内容: " + content + "\n" +
		"任务标题: " + task.Title + "\n" +
		"任务摘要: " + summary + "\n" +
		"请按项目任务要求执行，并在完成后通过项目任务回写端点提交结论、证据、工件引用和不确定性。"
}

func dispatchFailurePayload(task project.ProjectTask, err error, retryable bool) map[string]any {
	digitalEmployeeID := ""
	if task.AssignedDigitalEmployeeID != nil {
		digitalEmployeeID = task.AssignedDigitalEmployeeID.String()
	}
	return map[string]any{
		"project_task_id":     task.ID.String(),
		"digital_employee_id": digitalEmployeeID,
		"error":               err.Error(),
		"error_family":        "project_task_dispatch",
		"retryable":           retryable,
		"dispatch_actor_type": "project_coordinator",
	}
}

// dispatchErrorRetryable classifies a dispatch error. Validation/state errors and
// terminal run-start failures will not succeed on retry; everything else (transient
// run-start failures, DB hiccups) is retryable. Shared by the store (to set the
// failure event payload) and the activity wrapper (to mark Temporal non-retryable).
func dispatchErrorRetryable(err error) bool {
	switch {
	case errors.Is(err, project.ErrProjectNotFound),
		errors.Is(err, project.ErrInvalidProject),
		errors.Is(err, project.ErrProjectConflict):
		return false
	}
	var startErr *ProjectTaskRunStartError
	if errors.As(err, &startErr) {
		return startErr.Retryable
	}
	return true
}

// reemittedDispatchedPayload rebuilds the dispatched-event payload from the bound
// task row when the event has to be re-emitted on the idempotent replay path. Only
// fields available on the task row are present; runtime_node_id/node_id are omitted
// because they live on the run, not the task. `reemitted: true` marks the lean copy.
func reemittedDispatchedPayload(task project.ProjectTask, projectRecord project.Project) map[string]any {
	payload := map[string]any{
		"project_task_id":     task.ID.String(),
		"dispatch_actor_type": "project_coordinator",
		"dispatch_user_id":    projectRecord.HumanOwnerUserID.String(),
		"reemitted":           true,
	}
	if task.AssignedDigitalEmployeeID != nil {
		payload["digital_employee_id"] = task.AssignedDigitalEmployeeID.String()
	}
	if task.DigitalEmployeeRunID != nil {
		payload["digital_employee_run_id"] = task.DigitalEmployeeRunID.String()
	}
	if task.RuntimeTaskID != nil {
		payload["runtime_task_id"] = task.RuntimeTaskID.String()
	}
	return payload
}
```

- [ ] **Step 3: Replace DispatchProjectTask implementation**

Replace `DispatchProjectTask` with:

```go
func (s *ProjectStore) DispatchProjectTask(ctx context.Context, input DispatchProjectTaskInput) error {
	if s.repository == nil || s.runStarter == nil {
		return ErrActivityStoreRequired
	}
	task, err := s.repository.GetProjectTask(ctx, input.TenantID, input.TaskID)
	if err != nil {
		return err
	}
	if task.ProjectID != input.ProjectID {
		return project.ErrProjectNotFound
	}
	// Idempotent short-circuit: once a run is bound, re-dispatch (including Temporal
	// activity retries) must not start a second run. It still guarantees the dispatched
	// event was emitted, re-emitting it once if a prior attempt bound the run but failed
	// before writing the event. ProjectTaskEventExists keeps this exactly-once.
	if task.DigitalEmployeeRunID != nil {
		exists, err := s.repository.ProjectTaskEventExists(ctx, input.TenantID, input.ProjectID, project.ProjectEventTaskDispatched, input.TaskID.String())
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
		if err != nil {
			return err
		}
		_, err = s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventTaskDispatched, input.TaskID.String(), "项目任务已分派", reemittedDispatchedPayload(task, projectRecord)))
		return err
	}
	if !projectTaskDispatchAllowed(task.Status) || task.AssignedDigitalEmployeeID == nil || task.DemandID == nil {
		return project.ErrInvalidProject
	}
	projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return err
	}
	demand, err := s.repository.GetProjectDemand(ctx, input.TenantID, *task.DemandID)
	if err != nil {
		return err
	}
	run, err := s.runStarter.StartProjectTaskRun(ctx, StartProjectTaskRunRequest{
		TenantID:          input.TenantID,
		ProjectID:         input.ProjectID,
		DemandID:          demand.ID,
		ProjectTaskID:     task.ID,
		DigitalEmployeeID: *task.AssignedDigitalEmployeeID,
		DispatchUserID:    projectRecord.HumanOwnerUserID,
		Objective:         task.Title,
		Prompt:            projectTaskRunPrompt(projectRecord, demand, task),
		IdempotencyKey:    projectTaskDispatchIdempotencyKey(task.ID),
		Metadata: map[string]any{
			"source":          "project_task_dispatch",
			"actor_type":      "project_coordinator",
			"project_id":      input.ProjectID.String(),
			"demand_id":       demand.ID.String(),
			"project_task_id": task.ID.String(),
		},
	})
	if err != nil {
		_, _ = s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventTaskDispatchFailed, input.TaskID.String(), "项目任务分派失败", dispatchFailurePayload(task, err, dispatchErrorRetryable(err))))
		return err
	}
	// Bind the run before emitting the dispatched event. The binding is the durable
	// state that enables runtime writeback, so it must commit before any non-essential
	// side effect. This prevents an orphaned run if the event append fails or the
	// activity is interrupted between starting the run and binding the task.
	if _, err := s.repository.BindProjectTaskRun(ctx, project.BindProjectTaskRunRequest{
		TenantID:             input.TenantID,
		ProjectTaskID:        input.TaskID,
		DigitalEmployeeRunID: run.RunID,
		RuntimeTaskID:        run.RuntimeTaskID,
		CurrentStatuses:      []string{"planned", "pending"},
	}); err != nil {
		return err
	}
	// Emit the dispatched audit event. If this append fails, return the error so Temporal
	// retries; the bound-task short-circuit above then re-emits it idempotently (guarded by
	// ProjectTaskEventExists), so the event is delivered exactly once and the already-bound
	// run is never orphaned.
	_, err = s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventTaskDispatched, input.TaskID.String(), "项目任务已分派", map[string]any{
		"project_task_id":         input.TaskID.String(),
		"digital_employee_id":     task.AssignedDigitalEmployeeID.String(),
		"digital_employee_run_id": run.RunID.String(),
		"runtime_task_id":         run.RuntimeTaskID.String(),
		"runtime_node_id":         run.RuntimeNodeID.String(),
		"node_id":                 run.NodeID,
		"dispatch_actor_type":     "project_coordinator",
		"dispatch_user_id":        projectRecord.HumanOwnerUserID.String(),
	}))
	return err
}
```

- [ ] **Step 4: Run ProjectStore dispatch tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectStoreDispatchProjectTask|TestDispatchErrorRetryableClassification' -count=1
```

Expected: PASS.

- [ ] **Step 5: Run all projectcoordination tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -count=1
```

Expected: PASS. Existing workflow tests should still pass because they use `recordingActivityStore`, not `ProjectStore`.

- [ ] **Step 6: Commit dispatch bridge**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/project_store.go
git commit -m "feat: dispatch project tasks to runtime runs"
```

---

### Task 4: Make Dispatch Failures Retry-Safe

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/activities.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`

- [ ] **Step 1: Wrap terminal dispatch errors as non-retryable**

In `apps/control-plane/internal/workflow/projectcoordination/activities.go`, add the import `"go.temporal.io/sdk/temporal"` and replace `Activities.DispatchProjectTask` with:

```go
func (a *Activities) DispatchProjectTask(ctx context.Context, input DispatchProjectTaskInput) error {
	if a.store == nil {
		return ErrActivityStoreRequired
	}
	err := a.store.DispatchProjectTask(ctx, input)
	if err != nil && !dispatchErrorRetryable(err) {
		// Terminal failures (validation, binding conflict, non-retryable run start) are
		// already recorded as a project_task.dispatch_failed event; do not let Temporal
		// retry them, so the failure event stays a single record.
		return temporal.NewNonRetryableApplicationError("project task dispatch rejected", "ProjectTaskDispatchTerminal", err)
	}
	return err
}
```

- [ ] **Step 2: Let one task's dispatch failure not abort its siblings**

In `apps/control-plane/internal/workflow/projectcoordination/workflow.go`, replace `dispatchProjectTasks` with:

```go
func dispatchProjectTasks(ctx workflow.Context, tenantID, projectID uuid.UUID, taskIDs []uuid.UUID) error {
	for _, taskID := range taskIDs {
		if err := workflow.ExecuteActivity(ctx, (*Activities).DispatchProjectTask, DispatchProjectTaskInput{
			TenantID:  tenantID,
			ProjectID: projectID,
			TaskID:    taskID,
		}).Get(ctx, nil); err != nil {
			// A single task's dispatch failure (terminal rejection or exhausted retries)
			// must not abort dispatch of its siblings. The failure is already recorded as a
			// project_task.dispatch_failed event by the activity store, so the human owner
			// still sees it; coordination proceeds with the tasks that did dispatch.
			workflow.GetLogger(ctx).Warn("dispatch project task failed", "task_id", taskID.String(), "error", err.Error())
			continue
		}
	}
	return nil
}
```

The caller `if err := dispatchProjectTasks(...); err != nil { return err }` keeps compiling (the function now always returns `nil`).

- [ ] **Step 3: Add configurable dispatch error to the recording store and activity tests**

In `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`, add a `dispatchErr error` field to `recordingActivityStore` and return it from its `DispatchProjectTask`:

```go
func (s *recordingActivityStore) DispatchProjectTask(ctx context.Context, input DispatchProjectTaskInput) error {
	s.calls = append(s.calls, "DispatchProjectTask")
	return s.dispatchErr
}
```

Add these tests (add the import `"go.temporal.io/sdk/temporal"` if not present):

```go
func TestActivitiesDispatchProjectTaskWrapsTerminalErrorAsNonRetryable(t *testing.T) {
	store := &recordingActivityStore{dispatchErr: project.ErrInvalidProject}
	activities := NewActivities(store)
	err := activities.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: uuid.New(), ProjectID: uuid.New(), TaskID: uuid.New()})
	var appErr *temporal.ApplicationError
	if !errors.As(err, &appErr) || !appErr.NonRetryable() {
		t.Fatalf("expected non-retryable application error, got %#v", err)
	}
}

func TestActivitiesDispatchProjectTaskKeepsTransientErrorRetryable(t *testing.T) {
	store := &recordingActivityStore{dispatchErr: errors.New("db timeout")}
	activities := NewActivities(store)
	err := activities.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: uuid.New(), ProjectID: uuid.New(), TaskID: uuid.New()})
	if err == nil {
		t.Fatal("expected error")
	}
	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) && appErr.NonRetryable() {
		t.Fatalf("expected retryable error, got non-retryable %#v", err)
	}
}
```

Confirm `workflow_test.go` imports `errors`, `context`, `github.com/google/uuid`, and `github.com/superteam/control-plane/internal/project`; add any that are missing.

- [ ] **Step 4: Run projectcoordination tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -count=1
```

Expected: PASS. Existing workflow call-sequence tests remain valid because the success path still dispatches every task; the new tests cover the terminal/transient classification at the activity boundary.

- [ ] **Step 5: Commit retry-safety changes**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/activities.go apps/control-plane/internal/workflow/projectcoordination/workflow.go apps/control-plane/internal/workflow/projectcoordination/workflow_test.go
git commit -m "feat: make project task dispatch retry-safe"
```

---

### Task 5: Add Run Starter Adapter and App Injection

**Files:**
- Modify: `apps/control-plane/internal/app/app.go`

- [ ] **Step 1: Add adapter type with failure classification**

In `apps/control-plane/internal/app/app.go`, add near the other small adapters (the file already imports `errors` and the `employee` package):

```go
type projectTaskRunStarterAdapter struct {
	runService *employee.DigitalEmployeeRunService
}

func (a projectTaskRunStarterAdapter) StartProjectTaskRun(ctx context.Context, req projectcoordination.StartProjectTaskRunRequest) (projectcoordination.StartProjectTaskRunResult, error) {
	if a.runService == nil {
		return projectcoordination.StartProjectTaskRunResult{}, errors.New("digital employee run service is required")
	}
	idempotencyKey := req.IdempotencyKey
	run, err := a.runService.CreateRun(ctx, employee.CreateDigitalEmployeeRunRequest{
		TenantID:          req.TenantID,
		UserID:            req.DispatchUserID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		Objective:         req.Objective,
		Prompt:            req.Prompt,
		IdempotencyKey:    &idempotencyKey,
		Metadata:          req.Metadata,
	})
	if err != nil {
		return projectcoordination.StartProjectTaskRunResult{}, &projectcoordination.ProjectTaskRunStartError{
			Retryable: runStartRetryable(err),
			Err:       err,
		}
	}
	return projectcoordination.StartProjectTaskRunResult{
		RunID:         run.ID,
		RuntimeTaskID: run.TaskID,
		RuntimeNodeID: run.RuntimeNodeID,
		NodeID:        run.NodeID,
	}, nil
}

// runStartRetryable treats invalid-input as terminal; runtime-unavailable and an
// active-run-in-flight conflict are transient and will succeed on a later retry.
func runStartRetryable(err error) bool {
	return !errors.Is(err, employee.ErrInvalidInput)
}
```

- [ ] **Step 2: Move audit + run service construction before the Temporal worker setup**

In `NewContainerWithConfig`, the run starter adapter must exist before the `if cfg.Temporal.Enabled` block (which builds `coordinationStore`). Move the `auditRepository`/`auditService` creation and the `runRepository`/`runService`/`runWritebackService` creation above that block, preserving construction:

```go
auditRepository := audit.NewPgRepository(q)
auditService, err := audit.NewService(auditRepository)
if err != nil {
	return nil, err
}

runRepository := employee.NewPgRunRepository(q, stores.Postgres)
runService, err := employee.NewDigitalEmployeeRunService(runRepository, runtimeCommands, auditService)
if err != nil {
	return nil, err
}
runWritebackService, err := employee.NewDigitalEmployeeRunWritebackService(runRepository, auditService, runtimeEventRecorderAdapter{runtimeService: runtimeService})
if err != nil {
	return nil, err
}
```

`runtimeCommands` (used by `runService`) and `runtimeService` (used by `runWritebackService`) are already constructed earlier in the function, so this move compiles. `auditService` is only consumed afterward (tenant service, run services), so moving it up is safe.

- [ ] **Step 3: Inject run starter into ProjectStore**

Replace:

```go
coordinationStore := projectcoordination.NewProjectStoreWithApprovalsAndInbox(projectRepository, approvalService, decisionProjector)
```

with:

```go
coordinationStore := projectcoordination.NewProjectStoreWithApprovalsInboxAndRunStarter(
	projectRepository,
	approvalService,
	decisionProjector,
	projectTaskRunStarterAdapter{runService: runService},
)
```

- [ ] **Step 4: Remove the now-relocated original blocks**

Delete the original `auditRepository`/`auditService` block and the original `runRepository`/`runService`/`runWritebackService` block from their previous positions (after the Temporal block / near the handler setup). Handler construction must keep using the already-created `runService` and `runWritebackService`. After the edit there must be exactly one construction of each — build and let the compiler catch any leftover redeclaration or use-before-declaration.

- [ ] **Step 5: Run app package tests**

Run:

```bash
go test ./apps/control-plane/internal/app -count=1
```

Expected: PASS or `[no test files]` with successful compile.

- [ ] **Step 6: Commit app injection**

```bash
git add apps/control-plane/internal/app/app.go
git commit -m "feat: inject project task run starter"
```

---

### Task 6: Add Service-Level Chain Test and Memory Repository Binding

**Files:**
- Modify: `apps/control-plane/internal/project/service_test.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`

- [ ] **Step 1: Add project service chain test**

In `apps/control-plane/internal/project/service_test.go`, add this test near `TestCompleteProjectTaskWritesSummaryAndSignalsCoordinator`:

```go
func TestBindProjectTaskRunEnablesCompleteProjectTaskWriteback(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeNodeID := uuid.New()
	eventID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "planned",
		AssignedDigitalEmployeeID: &employeeID,
	})

	_, err = repo.BindProjectTaskRun(context.Background(), BindProjectTaskRunRequest{
		TenantID:             tenantID,
		ProjectTaskID:        taskID,
		DigitalEmployeeRunID: runID,
		RuntimeTaskID:        runtimeTaskID,
		LatestEventID:        &eventID,
		CurrentStatuses:      []string{"planned", "pending"},
	})
	if err != nil {
		t.Fatalf("bind project task run: %v", err)
	}
	repo.projectTaskRunRuntimeNodes[taskID] = runtimeNodeID

	summary, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "证据充分",
	})
	if err != nil {
		t.Fatalf("complete project task: %v", err)
	}
	if summary.ProjectTaskID != taskID || repo.tasks[0].Status != "completed" || coordinator.completedSignals != 1 {
		t.Fatalf("expected completed writeback, summary=%#v task=%#v signals=%d", summary, repo.tasks[0], coordinator.completedSignals)
	}
}
```

- [ ] **Step 2: Add chain test in projectcoordination package**

In `project_store_test.go`, add this focused bridge test after the dispatch tests:

```go
func TestProjectStoreDispatchProjectTaskBindingEnablesRuntimeWriteback(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "需求"},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    "planned",
			AssignedDigitalEmployeeID: &employeeID,
		}},
	}
	starter := &projectTaskRunStarterFake{result: StartProjectTaskRunResult{
		RunID:         runID,
		RuntimeTaskID: runtimeTaskID,
		RuntimeNodeID: runtimeNodeID,
		NodeID:        "node-1",
	}}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	if err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID}); err != nil {
		t.Fatalf("dispatch project task: %v", err)
	}
	task := repo.tasks[0]
	if task.Status != "assigned" || task.DigitalEmployeeRunID == nil || *task.DigitalEmployeeRunID != runID || task.RuntimeTaskID == nil || *task.RuntimeTaskID != runtimeTaskID {
		t.Fatalf("expected runtime writeback-ready task, got %#v", task)
	}
	if runtimeNodeID == uuid.Nil {
		t.Fatal("expected fake runtime node id to be set")
	}
}
```

- [ ] **Step 3: Run service and coordination tests**

Run:

```bash
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/workflow/projectcoordination -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit tests**

```bash
git add apps/control-plane/internal/project/service_test.go apps/control-plane/internal/workflow/projectcoordination/project_store_test.go
git commit -m "test: verify project task dispatch binding"
```

---

### Task 7: Changelog and Final Verification

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Get actual changelog timestamp**

Run:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Expected: prints the current Asia/Shanghai timestamp. Use the exact output.

- [ ] **Step 2: Add changelog entry**

In `CHANGELOG.md`, add one bullet that starts with the exact timestamp printed by Step 1, followed by this fixed text: `Control Plane: 打通 ProjectTask 协调分派到 DigitalEmployeeRun 的执行桥接，成功分派先创建真实 run 再原子绑定 digital_employee_run_id/runtime_task_id（绑定先于事件，避免孤儿 run），重复分派与 Temporal 重试幂等；失败时按可重试性记录分派失败事件，终态失败标记为不可重试，单个任务分派失败不再中断同批其余任务。`

- [ ] **Step 3: Run focused backend verification**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination ./apps/control-plane/internal/project ./apps/control-plane/internal/employee ./apps/control-plane/internal/app -count=1
```

Expected: PASS.

- [ ] **Step 4: Run broader Control Plane verification if focused tests pass**

Run:

```bash
pnpm verify:control-plane
```

Expected: PASS. If this command is unavailable in the environment, run:

```bash
go test ./apps/control-plane/... -count=1
```

and record the fallback in the final handoff.

- [ ] **Step 5: Check worktree**

Run:

```bash
git status --short
```

Expected: only intended implementation files and `CHANGELOG.md` are modified.

- [ ] **Step 6: Commit final verification changes**

```bash
git add CHANGELOG.md
git commit -m "docs: record project task runtime dispatch"
```

If any generated files or implementation files remain unstaged because earlier commits were intentionally deferred, stage and commit them with the smallest accurate message before this changelog commit.

---

## Self-Review Notes

- Spec coverage: tasks cover the run starter interface, ProjectTask binding repository, `DispatchProjectTask` behavior, retry-safety, app injection, failure semantics, tests, and changelog.
- Scope: plan stays inside Control Plane backend and does not alter OpenAPI, Web UI, or Runtime Agent.
- Type consistency: `StartProjectTaskRunRequest`, `StartProjectTaskRunResult`, `ProjectTaskRunStartError`, `BindProjectTaskRunRequest`, `ProjectEventTaskDispatchFailed`, and `ErrProjectConflict` are introduced before later tasks use them.
- Failure safety (P1): side effects run start → bind → dispatched event, so a started run is never left unbound (no orphaned writeback). Retries are idempotent via the early bound-task short-circuit plus `CreateRun`'s idempotency key; the dispatched event is re-emitted on replay only if missing, giving exactly-once delivery. `dispatchErrorRetryable` classifies failures; terminal ones are recorded once and marked non-retryable; a single task's dispatch failure does not abort its siblings.
- Conflict semantics (P0): `ErrProjectConflict` exists in the `project` package and the real `PgRepository.BindProjectTaskRun` distinguishes "bound to a different run / non-dispatchable" (`ErrProjectConflict`) from "missing task" (`ErrProjectNotFound`), matching the in-memory fakes.
- Dispatched-event durability: the `dispatched` event is delivered exactly once — emitted after bind on the happy path, and re-emitted on the bound-task replay path only when `ProjectTaskEventExists` reports it missing (which also dedupes the commit-ack-lost case). The replay copy is leaner (`reemitted: true`, without `runtime_node_id`/`node_id`, which live on the run rather than the task row). Cost: one extra `ProjectTaskEventExists` read per idempotent replay.
- Known bounded behavior: a *transient* run-start failure is retried by Temporal (`MaximumAttempts = 3`), so it may produce up to 3 `dispatch_failed` events — one per genuine failed attempt — which is bounded and intentional.
- Verification: focused Go tests are required before broader Control Plane verification.
