# ProjectTask Durable Closure Control Plane Implementation Plan

> 复核状态：06-20 ProjectTask durable closure基础落地

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Control Plane data model and service foundation for ProjectTask durable closure: `assigned -> queued`, `project_task_attempts`, accepted plan revision exact-once decomposition, and dispatch-created attempts.

**Architecture:** This is phase 1 of `docs/superpowers/specs/2026-06-20-project-task-durable-closure-design.md`. Control Plane remains the source of task truth. This phase does not switch Runtime Agent to attempt endpoints; it creates the persistent model, status transitions, repository methods, and dispatch behavior needed by later phases.

**Tech Stack:** Go, PostgreSQL migrations, sqlc, pgx, Temporal project coordination activities, repository/service tests, `corepack pnpm` repo scripts.

---

## Source Spec

Implement this plan against:

- `docs/superpowers/specs/2026-06-20-project-task-durable-closure-design.md`

When this plan conflicts with the spec, update the plan before coding. Do not silently narrow the spec.

## File Structure

Create:

- `apps/control-plane/internal/storage/migrations/024_project_task_attempts.sql`
  - Adds ProjectTask durable-closure columns, `project_task_attempts`, decomposition indexes, comments, and active-attempt constraints.

Modify:

- `apps/control-plane/internal/storage/migrations_test.go`
  - Verifies migration 024 table, fields, indexes, and status comments.
- `apps/control-plane/internal/storage/queries/project.sql`
  - Adds queries for attempt creation, attempt lookup, status transitions, exact-once plan decomposition, and queued dispatch.
- `apps/control-plane/internal/storage/queries/*.go`
  - Regenerate via `make -C apps/control-plane generate-sqlc`.
- `apps/control-plane/internal/project/types.go`
  - Adds ProjectTask status constants, `ProjectTaskAttempt`, attempt requests/results, and accepted-plan decomposition types.
- `apps/control-plane/internal/project/repository.go`
  - Adds attempt and decomposition methods.
- `apps/control-plane/internal/project/pg_repository.go`
  - Implements transactional attempt creation, status transitions, and exact-once decomposition.
- `apps/control-plane/internal/project/pg_repository_test.go`
  - Tests attempt persistence, active attempt uniqueness, `assigned -> queued`, idempotent decomposition, and conflict on mismatched decomposition payload.
- `apps/control-plane/internal/project/service.go`
  - Adds state-machine entrypoints used by coordination and later Runtime endpoints.
- `apps/control-plane/internal/project/service_test.go`
  - Tests service-level status transitions and invalid transition rejection.
- `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
  - Changes `DispatchProjectTask` to create a queued ProjectTask attempt instead of binding `assigned` as the durable execution state.
- `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`
  - Tests dispatch creates `queued + current_attempt_id` and uses attempt idempotency.
- `CHANGELOG.md`
  - Adds a dated backend design/implementation entry if project convention requires it for this phase.

Test-only generated or helper files are allowed only when referenced by the tests in this plan.

## Task 1: Add Migration 024 And Migration Tests

**Files:**

- Create: `apps/control-plane/internal/storage/migrations/024_project_task_attempts.sql`
- Modify: `apps/control-plane/internal/storage/migrations_test.go`

- [ ] **Step 1: Add failing migration tests**

In `apps/control-plane/internal/storage/migrations_test.go`, add tests near existing ProjectTask migration assertions:

```go
func TestProjectTaskAttemptsMigration(t *testing.T) {
	sql := readMigrations(t)
	block := createTableBlock(t, sql, "project_task_attempts")
	for _, fragment := range []string{
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"tenant_id UUID NOT NULL",
		"project_task_id UUID NOT NULL REFERENCES project_tasks(id) ON DELETE CASCADE",
		"attempt_no INTEGER NOT NULL",
		"status VARCHAR(50) NOT NULL",
		"digital_employee_run_id UUID",
		"runtime_task_id UUID",
		"runtime_node_id UUID",
		"provider_session_id VARCHAR(255)",
		"execution_context_packet JSONB NOT NULL DEFAULT '{}'::jsonb",
		"execution_context_packet_version VARCHAR(50) NOT NULL DEFAULT 'v1'",
		"lease_token VARCHAR(255) NOT NULL",
		"lease_expires_at TIMESTAMPTZ",
		"renewed_at TIMESTAMPTZ",
		"lost_at TIMESTAMPTZ",
		"started_at TIMESTAMPTZ",
		"finished_at TIMESTAMPTZ",
		"timeout_at TIMESTAMPTZ",
		"retryable BOOLEAN",
		"failure_family VARCHAR(100)",
		"failure_message TEXT",
		"idempotency_key VARCHAR(255) NOT NULL",
		"created_event_id UUID",
		"terminal_event_id UUID",
	} {
		if !strings.Contains(block, fragment) {
			t.Fatalf("project_task_attempts block missing %q:\n%s", fragment, block)
		}
	}
}

func TestProjectTasksDurableClosureColumns(t *testing.T) {
	sql := readMigrations(t)
	for _, fragment := range []string{
		"ALTER TABLE project_tasks",
		"ADD COLUMN current_attempt_id UUID",
		"ADD COLUMN accepted_plan_revision_id UUID",
		"ADD COLUMN decomposition_claim_key VARCHAR(255)",
		"ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0",
		"ADD COLUMN max_attempts INTEGER",
		"ADD COLUMN retry_not_before TIMESTAMPTZ",
		"ADD COLUMN waiting_reason VARCHAR(100)",
		"ADD COLUMN waiting_request_id UUID",
		"ADD COLUMN terminal_reason VARCHAR(100)",
		"ADD COLUMN terminal_event_id UUID",
		"ADD COLUMN cancelled_by VARCHAR(100)",
		"ADD COLUMN failed_by VARCHAR(100)",
		"ADD COLUMN status_changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"idx_project_tasks_current_attempt",
		"uq_project_tasks_accepted_plan_decomposition",
		"idx_project_task_attempts_active",
		"uq_project_task_attempts_idempotency_key",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migrations missing durable-closure fragment %q", fragment)
		}
	}
}
```

- [ ] **Step 2: Run migration tests and verify they fail**

Run:

```bash
cd apps/control-plane && go test ./internal/storage -run 'TestProjectTaskAttemptsMigration|TestProjectTasksDurableClosureColumns' -count=1
```

Expected: FAIL because migration 024 does not exist yet.

- [ ] **Step 3: Add migration 024**

Create `apps/control-plane/internal/storage/migrations/024_project_task_attempts.sql`:

```sql
ALTER TABLE project_tasks
    ADD COLUMN current_attempt_id UUID,
    ADD COLUMN accepted_plan_revision_id UUID,
    ADD COLUMN decomposition_claim_key VARCHAR(255),
    ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN max_attempts INTEGER,
    ADD COLUMN retry_not_before TIMESTAMPTZ,
    ADD COLUMN waiting_reason VARCHAR(100),
    ADD COLUMN waiting_request_id UUID,
    ADD COLUMN terminal_reason VARCHAR(100),
    ADD COLUMN terminal_event_id UUID,
    ADD COLUMN cancelled_by VARCHAR(100),
    ADD COLUMN failed_by VARCHAR(100),
    ADD COLUMN status_changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

UPDATE project_tasks
SET status = 'queued',
    status_changed_at = NOW()
WHERE status = 'assigned';

CREATE TABLE project_task_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_task_id UUID NOT NULL REFERENCES project_tasks(id) ON DELETE CASCADE,
    attempt_no INTEGER NOT NULL,
    status VARCHAR(50) NOT NULL,
    digital_employee_run_id UUID,
    runtime_task_id UUID,
    runtime_node_id UUID,
    provider_session_id VARCHAR(255),
    execution_context_packet JSONB NOT NULL DEFAULT '{}'::jsonb,
    execution_context_packet_version VARCHAR(50) NOT NULL DEFAULT 'v1',
    lease_token VARCHAR(255) NOT NULL,
    lease_expires_at TIMESTAMPTZ,
    renewed_at TIMESTAMPTZ,
    lost_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    timeout_at TIMESTAMPTZ,
    retryable BOOLEAN,
    failure_family VARCHAR(100),
    failure_message TEXT,
    idempotency_key VARCHAR(255) NOT NULL,
    created_event_id UUID REFERENCES project_events(id) ON DELETE SET NULL,
    terminal_event_id UUID REFERENCES project_events(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_project_task_attempts_status CHECK (
        status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled', 'lost', 'timed_out', 'waiting_human')
    )
);

CREATE UNIQUE INDEX uq_project_task_attempts_task_attempt_no
    ON project_task_attempts(tenant_id, project_task_id, attempt_no);

CREATE UNIQUE INDEX uq_project_task_attempts_idempotency_key
    ON project_task_attempts(tenant_id, idempotency_key);

CREATE INDEX idx_project_task_attempts_task_status
    ON project_task_attempts(tenant_id, project_task_id, status);

CREATE INDEX idx_project_task_attempts_active
    ON project_task_attempts(tenant_id, project_task_id)
    WHERE status IN ('queued', 'running', 'waiting_human');

ALTER TABLE project_tasks
    ADD CONSTRAINT fk_project_tasks_current_attempt
    FOREIGN KEY (current_attempt_id) REFERENCES project_task_attempts(id) ON DELETE SET NULL;

CREATE INDEX idx_project_tasks_current_attempt
    ON project_tasks(tenant_id, current_attempt_id)
    WHERE current_attempt_id IS NOT NULL;

CREATE UNIQUE INDEX uq_project_tasks_accepted_plan_decomposition
    ON project_tasks(tenant_id, project_id, demand_id, accepted_plan_revision_id, planned_task_key)
    WHERE accepted_plan_revision_id IS NOT NULL
      AND demand_id IS NOT NULL
      AND planned_task_key IS NOT NULL;

COMMENT ON COLUMN project_tasks.status IS 'ProjectTask lifecycle status: planned, queued, running, waiting_human, completed, failed, cancelled.';
COMMENT ON COLUMN project_tasks.current_attempt_id IS 'Current active project_task_attempts row for queued/running/waiting_human tasks.';
COMMENT ON COLUMN project_tasks.accepted_plan_revision_id IS 'Accepted plan revision that produced this task.';
COMMENT ON TABLE project_task_attempts IS 'Durable execution attempts for ProjectTask dispatch, lease, retry, and terminal writeback.';

CREATE TRIGGER update_project_task_attempts_updated_at
    BEFORE UPDATE ON project_task_attempts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
```

- [ ] **Step 4: Run migration tests and verify they pass**

Run:

```bash
cd apps/control-plane && go test ./internal/storage -run 'TestProjectTaskAttemptsMigration|TestProjectTasksDurableClosureColumns' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit migration skeleton**

Run:

```bash
git add apps/control-plane/internal/storage/migrations/024_project_task_attempts.sql apps/control-plane/internal/storage/migrations_test.go
git commit -m "feat(control-plane): add project task attempts schema"
```

## Task 2: Add Domain Types And Repository Contracts

**Files:**

- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/service_test.go`

- [ ] **Step 1: Add failing compile-time service test**

In `apps/control-plane/internal/project/service_test.go`, add:

```go
func TestQueueProjectTaskCreatesAttemptAndMovesTaskToQueued(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "实现幂等写回",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
		CreatedAt:                 time.Now().UTC(),
		UpdatedAt:                 time.Now().UTC(),
	})

	result, err := service.QueueProjectTask(context.Background(), QueueProjectTaskRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		IdempotencyKey:    "project-task:" + taskID.String() + ":attempt:1:queue",
		LeaseToken:        "lease-token-1",
	})
	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusQueued, result.Task.Status)
	require.NotNil(t, result.Task.CurrentAttemptID)
	require.Equal(t, int32(1), result.Attempt.AttemptNo)
	require.Equal(t, ProjectTaskAttemptStatusQueued, result.Attempt.Status)
	require.Equal(t, "lease-token-1", result.Attempt.LeaseToken)
}
```

- [ ] **Step 2: Run test and verify it fails to compile**

Run:

```bash
cd apps/control-plane && go test ./internal/project -run TestQueueProjectTaskCreatesAttemptAndMovesTaskToQueued -count=1
```

Expected: FAIL with undefined `ProjectTaskStatusQueued`, `QueueProjectTaskRequest`, or related types.

- [ ] **Step 3: Add domain constants and types**

In `apps/control-plane/internal/project/types.go`, add near ProjectTask definitions:

```go
const (
	ProjectTaskStatusPlanned      = "planned"
	ProjectTaskStatusQueued       = "queued"
	ProjectTaskStatusRunning      = "running"
	ProjectTaskStatusWaitingHuman = "waiting_human"
	ProjectTaskStatusCompleted    = "completed"
	ProjectTaskStatusFailed       = "failed"
	ProjectTaskStatusCancelled    = "cancelled"
)

const (
	ProjectTaskAttemptStatusQueued       = "queued"
	ProjectTaskAttemptStatusRunning      = "running"
	ProjectTaskAttemptStatusSucceeded    = "succeeded"
	ProjectTaskAttemptStatusFailed       = "failed"
	ProjectTaskAttemptStatusCancelled    = "cancelled"
	ProjectTaskAttemptStatusLost         = "lost"
	ProjectTaskAttemptStatusTimedOut     = "timed_out"
	ProjectTaskAttemptStatusWaitingHuman = "waiting_human"
)

type ProjectTaskAttempt struct {
	ID                            uuid.UUID
	TenantID                      uuid.UUID
	ProjectTaskID                 uuid.UUID
	AttemptNo                     int32
	Status                        string
	DigitalEmployeeRunID          *uuid.UUID
	RuntimeTaskID                 *uuid.UUID
	RuntimeNodeID                 *uuid.UUID
	ProviderSessionID             *string
	ExecutionContextPacket        map[string]any
	ExecutionContextPacketVersion string
	LeaseToken                    string
	LeaseExpiresAt                *time.Time
	RenewedAt                     *time.Time
	LostAt                        *time.Time
	StartedAt                     *time.Time
	FinishedAt                    *time.Time
	TimeoutAt                     *time.Time
	Retryable                     *bool
	FailureFamily                 *string
	FailureMessage                *string
	IdempotencyKey                string
	CreatedEventID                *uuid.UUID
	TerminalEventID               *uuid.UUID
	CreatedAt                     time.Time
	UpdatedAt                     time.Time
}

type QueueProjectTaskRequest struct {
	TenantID                      uuid.UUID
	ProjectID                     uuid.UUID
	ProjectTaskID                 uuid.UUID
	DigitalEmployeeID             uuid.UUID
	IdempotencyKey                string
	LeaseToken                    string
	LeaseExpiresAt                *time.Time
	ExecutionContextPacket        map[string]any
	ExecutionContextPacketVersion string
}

type QueueProjectTaskResult struct {
	Task    ProjectTask
	Attempt ProjectTaskAttempt
	Event   ProjectEvent
}

type DecomposeAcceptedPlanRevisionRequest struct {
	TenantID               uuid.UUID
	ProjectID              uuid.UUID
	DemandID               uuid.UUID
	CoordinationJobID      uuid.UUID
	RouteDecisionID        uuid.UUID
	AcceptedPlanRevisionID uuid.UUID
	DecompositionClaimKey  string
	Tasks                  []ProjectTaskGraphCreateTask
}

type DecomposeAcceptedPlanRevisionResult struct {
	Tasks        []ProjectTask
	Dependencies []ProjectTaskDependency
	Replayed     bool
}
```

Extend `ProjectTask` with:

```go
CurrentAttemptID      *uuid.UUID
AcceptedPlanRevisionID *uuid.UUID
DecompositionClaimKey  *string
AttemptCount           int32
MaxAttempts            *int32
RetryNotBefore         *time.Time
WaitingReason          *string
WaitingRequestID       *uuid.UUID
TerminalReason         *string
TerminalEventID        *uuid.UUID
CancelledBy            *string
FailedBy               *string
StatusChangedAt        time.Time
```

- [ ] **Step 4: Add repository interface methods**

In `apps/control-plane/internal/project/repository.go`, add:

```go
	CreateProjectTaskAttempt(ctx context.Context, req QueueProjectTaskRequest, attemptNo int32, eventID *uuid.UUID) (ProjectTaskAttempt, error)
	QueueProjectTaskWithAttempt(ctx context.Context, req QueueProjectTaskRequest) (QueueProjectTaskResult, error)
	GetProjectTaskAttempt(ctx context.Context, tenantID, attemptID uuid.UUID) (ProjectTaskAttempt, error)
	GetCurrentProjectTaskAttempt(ctx context.Context, tenantID, projectTaskID uuid.UUID) (ProjectTaskAttempt, error)
	DecomposeAcceptedPlanRevision(ctx context.Context, req DecomposeAcceptedPlanRevisionRequest) (DecomposeAcceptedPlanRevisionResult, error)
```

- [ ] **Step 5: Run compile test and verify repository fakes are incomplete**

Run:

```bash
cd apps/control-plane && go test ./internal/project -run TestQueueProjectTaskCreatesAttemptAndMovesTaskToQueued -count=1
```

Expected: FAIL because memory repository and service methods are not implemented.

- [ ] **Step 6: Commit domain contract**

Run:

```bash
git add apps/control-plane/internal/project/types.go apps/control-plane/internal/project/repository.go apps/control-plane/internal/project/service_test.go
git commit -m "feat(control-plane): define project task attempt contracts"
```

## Task 3: Add sqlc Queries And Repository Implementation

**Files:**

- Modify: `apps/control-plane/internal/storage/queries/project.sql`
- Modify: generated files under `apps/control-plane/internal/storage/queries/`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository_test.go`

- [ ] **Step 1: Add repository tests for attempt creation**

In `apps/control-plane/internal/project/pg_repository_test.go`, add:

```go
func TestQueueProjectTaskWithAttemptMovesPlannedTaskToQueued(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectForRepositoryTest(t, repo, tenantID)
	employeeID := uuid.New()
	task := createProjectTaskForRepositoryTest(t, repo, tenantID, projectID, ProjectTask{
		Title:                     "验证状态机",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})

	result, err := repo.QueueProjectTaskWithAttempt(context.Background(), QueueProjectTaskRequest{
		TenantID:                      tenantID,
		ProjectID:                     projectID,
		ProjectTaskID:                 task.ID,
		DigitalEmployeeID:             employeeID,
		IdempotencyKey:                "project-task:" + task.ID.String() + ":attempt:1:queue",
		LeaseToken:                    "lease-token-1",
		ExecutionContextPacket:        map[string]any{"task_title": task.Title},
		ExecutionContextPacketVersion: "v1",
	})
	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusQueued, result.Task.Status)
	require.Equal(t, result.Attempt.ID, *result.Task.CurrentAttemptID)
	require.Equal(t, int32(1), result.Attempt.AttemptNo)
	require.Equal(t, ProjectTaskAttemptStatusQueued, result.Attempt.Status)
	require.Equal(t, "lease-token-1", result.Attempt.LeaseToken)
	require.Equal(t, "v1", result.Attempt.ExecutionContextPacketVersion)
}
```

If `createProjectForRepositoryTest` or `createProjectTaskForRepositoryTest` is not already present in `apps/control-plane/internal/project/pg_repository_test.go`, add these local helpers near the other repository-test helpers:

```go
func createProjectForRepositoryTest(t *testing.T, repo *PGRepository, tenantID uuid.UUID) uuid.UUID {
	t.Helper()

	project, err := repo.CreateProject(context.Background(), CreateProjectRequest{
		TenantID: tenantID,
		Name:     "project task durable closure test",
	})
	require.NoError(t, err)
	return project.ID
}

func createProjectTaskForRepositoryTest(t *testing.T, repo *PGRepository, tenantID uuid.UUID, projectID uuid.UUID, task ProjectTask) ProjectTask {
	t.Helper()

	task.TenantID = tenantID
	task.ProjectID = projectID
	created, err := repo.CreateProjectTask(context.Background(), task)
	require.NoError(t, err)
	return created
}
```

- [ ] **Step 2: Run repository test and verify it fails**

Run:

```bash
cd apps/control-plane && go test ./internal/project -run TestQueueProjectTaskWithAttemptMovesPlannedTaskToQueued -count=1
```

Expected: FAIL because sqlc queries and repository method are missing.

- [ ] **Step 3: Add sqlc queries**

Append to `apps/control-plane/internal/storage/queries/project.sql`:

```sql
-- name: CreateProjectTaskAttempt :one
INSERT INTO project_task_attempts (
    tenant_id,
    project_task_id,
    attempt_no,
    status,
    digital_employee_run_id,
    runtime_task_id,
    runtime_node_id,
    execution_context_packet,
    execution_context_packet_version,
    lease_token,
    lease_expires_at,
    idempotency_key,
    created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_task_id')::uuid,
    sqlc.arg('attempt_no')::integer,
    sqlc.arg('status')::varchar,
    sqlc.narg('digital_employee_run_id')::uuid,
    sqlc.narg('runtime_task_id')::uuid,
    sqlc.narg('runtime_node_id')::uuid,
    sqlc.arg('execution_context_packet')::jsonb,
    sqlc.arg('execution_context_packet_version')::varchar,
    sqlc.arg('lease_token')::varchar,
    sqlc.narg('lease_expires_at')::timestamptz,
    sqlc.arg('idempotency_key')::varchar,
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: GetProjectTaskAttempt :one
SELECT * FROM project_task_attempts
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid;

-- name: GetCurrentProjectTaskAttempt :one
SELECT pta.*
FROM project_task_attempts pta
JOIN project_tasks pt ON pt.current_attempt_id = pta.id
WHERE pt.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pt.id = sqlc.arg('project_task_id')::uuid;

-- name: QueueProjectTask :one
UPDATE project_tasks
SET status = 'queued',
    current_attempt_id = sqlc.arg('current_attempt_id')::uuid,
    attempt_count = attempt_count + 1,
    retry_not_before = NULL,
    waiting_reason = NULL,
    waiting_request_id = NULL,
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    status_changed_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status IN ('planned', 'waiting_human')
RETURNING *;
```

- [ ] **Step 4: Regenerate sqlc**

Run:

```bash
make -C apps/control-plane generate-sqlc
```

Expected: command exits 0 and generated files under `apps/control-plane/internal/storage/queries/` update.

- [ ] **Step 5: Implement repository mapping**

In `apps/control-plane/internal/project/pg_repository.go`, add:

```go
func (r *PgRepository) QueueProjectTaskWithAttempt(ctx context.Context, req QueueProjectTaskRequest) (QueueProjectTaskResult, error) {
	return withProjectQueries(ctx, r, "queue project task attempt", func(q *queries.Queries) (QueueProjectTaskResult, error) {
		task, err := r.getProjectTaskWithQueries(ctx, q, req.TenantID, req.ProjectTaskID)
		if err != nil {
			return QueueProjectTaskResult{}, err
		}
		if task.Status != ProjectTaskStatusPlanned && task.Status != ProjectTaskStatusWaitingHuman {
			return QueueProjectTaskResult{}, ErrProjectConflict
		}
		attemptNo := task.AttemptCount + 1
		event, err := r.appendProjectEventWithQueries(ctx, q, AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    req.ProjectID,
			EventType:    ProjectEventTaskDispatched,
			ActorType:    "project_coordinator",
			ActorID:      req.ProjectTaskID.String(),
			ResourceType: strPtr("project_task"),
			ResourceID:   strPtr(req.ProjectTaskID.String()),
			Summary:      "项目任务已进入执行队列",
			Payload: map[string]any{
				"project_task_id": req.ProjectTaskID.String(),
				"attempt_no":      attemptNo,
				"status":          ProjectTaskStatusQueued,
			},
		})
		if err != nil {
			return QueueProjectTaskResult{}, err
		}
		packet := req.ExecutionContextPacket
		if packet == nil {
			packet = map[string]any{}
		}
		packetBytes, err := json.Marshal(packet)
		if err != nil {
			return QueueProjectTaskResult{}, err
		}
		version := strings.TrimSpace(req.ExecutionContextPacketVersion)
		if version == "" {
			version = "v1"
		}
		attemptRow, err := q.CreateProjectTaskAttempt(ctx, queries.CreateProjectTaskAttemptParams{
			TenantID:                      req.TenantID,
			ProjectTaskID:                 req.ProjectTaskID,
			AttemptNo:                     attemptNo,
			Status:                        ProjectTaskAttemptStatusQueued,
			ExecutionContextPacket:        packetBytes,
			ExecutionContextPacketVersion: version,
			LeaseToken:                    req.LeaseToken,
			LeaseExpiresAt:                timestamptzOrNull(req.LeaseExpiresAt),
			IdempotencyKey:                req.IdempotencyKey,
			CreatedEventID:                uuidOrNull(&event.ID),
		})
		if err != nil {
			return QueueProjectTaskResult{}, err
		}
		updatedRow, err := q.QueueProjectTask(ctx, queries.QueueProjectTaskParams{
			TenantID:         req.TenantID,
			ProjectID:        req.ProjectID,
			ID:               req.ProjectTaskID,
			CurrentAttemptID: attemptRow.ID,
			LatestEventID:    uuidOrNull(&event.ID),
		})
		if err != nil {
			return QueueProjectTaskResult{}, err
		}
		return QueueProjectTaskResult{
			Task:    taskFromRecord(updatedRow),
			Attempt: projectTaskAttemptFromRecord(attemptRow),
			Event:   event,
		}, nil
	})
}
```

Add `projectTaskAttemptFromRecord` near other mappers:

```go
func projectTaskAttemptFromRecord(row queries.ProjectTaskAttempt) ProjectTaskAttempt {
	packet := map[string]any{}
	if len(row.ExecutionContextPacket) > 0 {
		_ = json.Unmarshal(row.ExecutionContextPacket, &packet)
	}
	return ProjectTaskAttempt{
		ID:                            row.ID,
		TenantID:                      row.TenantID,
		ProjectTaskID:                 row.ProjectTaskID,
		AttemptNo:                     int32(row.AttemptNo),
		Status:                        row.Status,
		DigitalEmployeeRunID:          uuidPtrFromPg(row.DigitalEmployeeRunID),
		RuntimeTaskID:                 uuidPtrFromPg(row.RuntimeTaskID),
		RuntimeNodeID:                 uuidPtrFromPg(row.RuntimeNodeID),
		ProviderSessionID:             textPtrFromPg(row.ProviderSessionID),
		ExecutionContextPacket:        packet,
		ExecutionContextPacketVersion: row.ExecutionContextPacketVersion,
		LeaseToken:                    row.LeaseToken,
		LeaseExpiresAt:                timePtrFromPg(row.LeaseExpiresAt),
		RenewedAt:                     timePtrFromPg(row.RenewedAt),
		LostAt:                        timePtrFromPg(row.LostAt),
		StartedAt:                     timePtrFromPg(row.StartedAt),
		FinishedAt:                    timePtrFromPg(row.FinishedAt),
		TimeoutAt:                     timePtrFromPg(row.TimeoutAt),
		Retryable:                     boolPtrFromPg(row.Retryable),
		FailureFamily:                 textPtrFromPg(row.FailureFamily),
		FailureMessage:                textPtrFromPg(row.FailureMessage),
		IdempotencyKey:                row.IdempotencyKey,
		CreatedEventID:                uuidPtrFromPg(row.CreatedEventID),
		TerminalEventID:               uuidPtrFromPg(row.TerminalEventID),
		CreatedAt:                     row.CreatedAt.Time,
		UpdatedAt:                     row.UpdatedAt.Time,
	}
}
```

If the helper names differ in this file, add small local helpers that convert `pgtype.UUID`, `pgtype.Text`, `pgtype.Bool`, and `pgtype.Timestamptz` into pointers.

- [ ] **Step 6: Run repository tests**

Run:

```bash
cd apps/control-plane && go test ./internal/project -run 'TestQueueProjectTaskWithAttemptMovesPlannedTaskToQueued' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit repository implementation**

Run:

```bash
git add apps/control-plane/internal/storage/queries apps/control-plane/internal/project/pg_repository.go apps/control-plane/internal/project/pg_repository_test.go
git commit -m "feat(control-plane): persist project task attempts"
```

## Task 4: Add Service State-Machine Entry Point

**Files:**

- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/service_test.go`

- [ ] **Step 1: Implement memory repository support**

In `apps/control-plane/internal/project/service_test.go`, add memory repository methods used by `QueueProjectTask`:

```go
func (r *memoryRepository) QueueProjectTaskWithAttempt(ctx context.Context, req QueueProjectTaskRequest) (QueueProjectTaskResult, error) {
	for i, task := range r.tasks {
		if task.TenantID != req.TenantID || task.ProjectID != req.ProjectID || task.ID != req.ProjectTaskID {
			continue
		}
		if task.Status != ProjectTaskStatusPlanned && task.Status != ProjectTaskStatusWaitingHuman {
			return QueueProjectTaskResult{}, ErrProjectConflict
		}
		event := ProjectEvent{
			ID:        uuid.New(),
			TenantID:  req.TenantID,
			ProjectID: req.ProjectID,
			EventType: ProjectEventTaskDispatched,
			ActorType: "project_coordinator",
			ActorID:   req.ProjectTaskID.String(),
			Summary:   "项目任务已进入执行队列",
			CreatedAt: time.Now().UTC(),
		}
		attemptID := uuid.New()
		attempt := ProjectTaskAttempt{
			ID:                            attemptID,
			TenantID:                      req.TenantID,
			ProjectTaskID:                 req.ProjectTaskID,
			AttemptNo:                     task.AttemptCount + 1,
			Status:                        ProjectTaskAttemptStatusQueued,
			ExecutionContextPacket:        req.ExecutionContextPacket,
			ExecutionContextPacketVersion: nonEmptyString(req.ExecutionContextPacketVersion, "v1"),
			LeaseToken:                    req.LeaseToken,
			LeaseExpiresAt:                req.LeaseExpiresAt,
			IdempotencyKey:                req.IdempotencyKey,
			CreatedEventID:                &event.ID,
			CreatedAt:                     time.Now().UTC(),
			UpdatedAt:                     time.Now().UTC(),
		}
		task.Status = ProjectTaskStatusQueued
		task.CurrentAttemptID = &attemptID
		task.AttemptCount = attempt.AttemptNo
		task.RetryNotBefore = nil
		task.WaitingReason = nil
		task.WaitingRequestID = nil
		task.LatestEventID = &event.ID
		task.StatusChangedAt = time.Now().UTC()
		task.UpdatedAt = task.StatusChangedAt
		r.tasks[i] = task
		r.events = append(r.events, event)
		r.projectTaskAttempts = append(r.projectTaskAttempts, attempt)
		return QueueProjectTaskResult{Task: task, Attempt: attempt, Event: event}, nil
	}
	return QueueProjectTaskResult{}, ErrProjectNotFound
}
```

Add `projectTaskAttempts []ProjectTaskAttempt` to `memoryRepository`. If `nonEmptyString` does not exist, add this helper in the test file:

```go
func nonEmptyString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
```

- [ ] **Step 2: Implement service method**

In `apps/control-plane/internal/project/service.go`, add:

```go
func (s *Service) QueueProjectTask(ctx context.Context, req QueueProjectTaskRequest) (QueueProjectTaskResult, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ProjectTaskID == uuid.Nil || req.DigitalEmployeeID == uuid.Nil {
		return QueueProjectTaskResult{}, ErrInvalidProject
	}
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	req.LeaseToken = strings.TrimSpace(req.LeaseToken)
	if req.IdempotencyKey == "" || req.LeaseToken == "" {
		return QueueProjectTaskResult{}, ErrInvalidProject
	}
	if req.ExecutionContextPacket == nil {
		req.ExecutionContextPacket = map[string]any{}
	}
	if strings.TrimSpace(req.ExecutionContextPacketVersion) == "" {
		req.ExecutionContextPacketVersion = "v1"
	}
	return s.repo.QueueProjectTaskWithAttempt(ctx, req)
}
```

- [ ] **Step 3: Run service tests**

Run:

```bash
cd apps/control-plane && go test ./internal/project -run 'TestQueueProjectTaskCreatesAttemptAndMovesTaskToQueued' -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit service entrypoint**

Run:

```bash
git add apps/control-plane/internal/project/service.go apps/control-plane/internal/project/service_test.go
git commit -m "feat(control-plane): queue project tasks through attempts"
```

## Task 5: Change Project Coordination Dispatch To Queue Attempt

**Files:**

- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`

- [ ] **Step 1: Add failing dispatch test**

In `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`, add:

```go
func TestProjectStoreDispatchProjectTaskCreatesQueuedAttempt(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{
			ID:                 projectID,
			TenantID:           tenantID,
			Name:               "Durable closure",
			Goal:               "Queue attempts",
			Status:             project.ProjectStatusRunning,
			HumanOwnerUserID:   uuid.New(),
			CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
		},
		demand: project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "任务闭环"},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "补齐 ProjectTask 状态机",
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
		}},
	}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, recordingProjectTaskRunStarter{
		result: StartProjectTaskRunResult{
			RunID:         uuid.New(),
			RuntimeTaskID: uuid.New(),
			RuntimeNodeID: uuid.New(),
			NodeID:        "node-1",
		},
	})

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		TaskID:    taskID,
	})
	require.NoError(t, err)
	require.Len(t, repo.queueRequests, 1)
	require.Equal(t, project.ProjectTaskStatusQueued, repo.tasks[0].Status)
	require.NotNil(t, repo.tasks[0].CurrentAttemptID)
}
```

- [ ] **Step 2: Run dispatch test and verify it fails**

Run:

```bash
cd apps/control-plane && go test ./internal/workflow/projectcoordination -run TestProjectStoreDispatchProjectTaskCreatesQueuedAttempt -count=1
```

Expected: FAIL because dispatch still binds `assigned` through `BindProjectTaskRun`.

- [ ] **Step 3: Update dispatch implementation**

In `apps/control-plane/internal/workflow/projectcoordination/project_store.go`, replace the `BindProjectTaskRun` block in `DispatchProjectTask` with a repository call that queues the task:

```go
queued, err := s.repository.QueueProjectTaskWithAttempt(ctx, project.QueueProjectTaskRequest{
	TenantID:          input.TenantID,
	ProjectID:         input.ProjectID,
	ProjectTaskID:     input.TaskID,
	DigitalEmployeeID: *task.AssignedDigitalEmployeeID,
	IdempotencyKey:    projectTaskDispatchIdempotencyKey(task.ID),
	LeaseToken:        "project-task-" + task.ID.String() + "-attempt-1",
	ExecutionContextPacket: map[string]any{
		"project_id":         input.ProjectID.String(),
		"demand_id":          demand.ID.String(),
		"project_task_id":    task.ID.String(),
		"digital_employee_id": task.AssignedDigitalEmployeeID.String(),
		"objective":          task.Title,
		"expected_outputs":   append([]any(nil), task.ExpectedOutputs...),
		"input_requirements": cloneAnyMap(task.InputRequirements),
		"handoff_contract":   projectTaskDispatchHandoffContract(task.HandoffContract),
	},
	ExecutionContextPacketVersion: "v1",
})
if err != nil {
	return s.recordDispatchFailure(ctx, input.TenantID, input.ProjectID, task, err)
}
```

Use `queued.Attempt.ID` in the dispatched event payload:

```go
"project_task_attempt_id": queued.Attempt.ID.String(),
"project_task_status":     project.ProjectTaskStatusQueued,
```

Keep existing run creation for now so the old Runtime command path continues to work until phase 2 switches Runtime endpoints. The durable state must be `queued`, not `assigned`.

- [ ] **Step 4: Update project store fake repository**

In `project_store_test.go`, add `queueRequests []project.QueueProjectTaskRequest` to `projectStoreMemoryRepository`, implement `QueueProjectTaskWithAttempt`, and update assertions that expected `assigned` to expect `queued`.

- [ ] **Step 5: Run project coordination tests**

Run:

```bash
cd apps/control-plane && go test ./internal/workflow/projectcoordination -run 'DispatchProjectTask|ProjectStoreDispatchProjectTask' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit dispatch change**

Run:

```bash
git add apps/control-plane/internal/workflow/projectcoordination/project_store.go apps/control-plane/internal/workflow/projectcoordination/project_store_test.go
git commit -m "feat(control-plane): dispatch project tasks as queued attempts"
```

## Task 6: Add Accepted Plan Revision Exact-Once Decomposition

**Files:**

- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository_test.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`

- [ ] **Step 1: Add repository exact-once tests**

In `apps/control-plane/internal/project/pg_repository_test.go`, add:

```go
func TestDecomposeAcceptedPlanRevisionIsIdempotent(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectForRepositoryTest(t, repo, tenantID)
	demandID := createDemandForRepositoryTest(t, repo, tenantID, projectID)
	revisionID := uuid.New()
	employeeID := uuid.New()

	req := DecomposeAcceptedPlanRevisionRequest{
		TenantID:               tenantID,
		ProjectID:              projectID,
		DemandID:               demandID,
		CoordinationJobID:      uuid.New(),
		RouteDecisionID:        uuid.New(),
		AcceptedPlanRevisionID: revisionID,
		DecompositionClaimKey:  "project-plan-decomposition:" + tenantID.String() + ":" + projectID.String() + ":" + demandID.String() + ":" + revisionID.String(),
		Tasks: []ProjectTaskGraphCreateTask{{
			Key:                       "implement-state-machine",
			Title:                     "实现状态机",
			Status:                    ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: employeeID,
			ExpectedOutputs:           []any{"state-machine-tests"},
		}},
	}

	first, err := repo.DecomposeAcceptedPlanRevision(context.Background(), req)
	require.NoError(t, err)
	second, err := repo.DecomposeAcceptedPlanRevision(context.Background(), req)
	require.NoError(t, err)
	require.True(t, second.Replayed)
	require.Equal(t, first.Tasks[0].ID, second.Tasks[0].ID)
}
```

- [ ] **Step 2: Implement exact-once decomposition**

In `apps/control-plane/internal/project/pg_repository.go`, implement `DecomposeAcceptedPlanRevision` by:

1. Listing existing tasks for `(tenant_id, project_id, demand_id, accepted_plan_revision_id)`.
2. If existing tasks match request keys and payloads, return them with `Replayed: true`.
3. If existing tasks exist but do not match request keys or payloads, return `ErrProjectConflict`.
4. If no existing tasks exist, create graph tasks in one transaction with `AcceptedPlanRevisionID` and `DecompositionClaimKey` set on every task.

- [ ] **Step 3: Run exact-once tests**

Run:

```bash
cd apps/control-plane && go test ./internal/project -run TestDecomposeAcceptedPlanRevisionIsIdempotent -count=1
```

Expected: PASS.

- [ ] **Step 4: Wire project coordination graph persistence**

In `apps/control-plane/internal/workflow/projectcoordination/project_store.go`, update graph persistence to call `DecomposeAcceptedPlanRevision`. In this phase, derive the accepted revision source with this exact rule:

```go
func acceptedPlanRevisionIDForRouteDecision(decision project.RouteDecision) uuid.UUID {
	if decision.AcceptedPlanRevisionID != nil {
		return *decision.AcceptedPlanRevisionID
	}
	if decision.RouteDecisionID != nil {
		return *decision.RouteDecisionID
	}
	if decision.CoordinationJobID != nil {
		return *decision.CoordinationJobID
	}
	return decision.ID
}
```

Also persist the derived value in each task's `planner_metadata.accepted_plan_revision_id`. When a dedicated plan revision table is introduced later, only this helper changes; the decomposition claim key remains stable.

- [ ] **Step 5: Run graph persistence tests**

Run:

```bash
cd apps/control-plane && go test ./internal/workflow/projectcoordination -run 'PersistRouteDecision|CreateProjectTaskGraph|DecomposeAcceptedPlanRevision' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit exact-once decomposition**

Run:

```bash
git add apps/control-plane/internal/project apps/control-plane/internal/workflow/projectcoordination
git commit -m "feat(control-plane): decompose accepted plans exactly once"
```

## Task 7: Phase Verification

**Files:**

- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add changelog entry**

Run:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Add under `CHANGELOG.md` Unreleased backend section:

```markdown
- [YYYY-MM-DD HH:MM] ProjectTask durable closure phase 1: add queued status foundation, project task attempts, and exact-once accepted plan decomposition.
```

Replace `YYYY-MM-DD HH:MM` with the command output.

- [ ] **Step 2: Run phase verification**

Run:

```bash
make -C apps/control-plane generate-sqlc
cd apps/control-plane && go test ./internal/storage ./internal/project ./internal/workflow/projectcoordination -count=1
corepack pnpm verify:contracts
git diff --check
```

Expected:

- sqlc generation exits 0.
- Go tests exit 0.
- contract verification exits 0.
- `git diff --check` exits 0.

- [ ] **Step 3: Commit verification metadata**

Run:

```bash
git add CHANGELOG.md apps/control-plane/internal/storage/queries
git commit -m "chore: verify project task closure phase 1"
```

If generated files did not change after the final `generate-sqlc`, include only `CHANGELOG.md` in this commit.
