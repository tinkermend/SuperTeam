# ProjectTask Dispatch Runtime Recovery Implementation Plan
> 复核状态：与配对spec相同——CHANGELOG 2026-07-02 17:57记录ProjectTask dispatch/runtime recovery首版；锚点抽查发现project_task_attempts与recovery相关数据库结构

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make dispatch failures, stale queued attempts, lost running attempts, and Provider/session startup failures produce a durable retry or human-recovery next action instead of silently leaving ProjectTasks near `planned/queued/running`.

**Architecture:** Reuse the existing ProjectTask attempt failure recovery machinery for attempts that already exist, and add a focused recovery path for dispatch failures that happen before an attempt is created. Keep Runtime Agent as fact writer only; Control Plane owns retry/backoff, attempt terminalization, and waiting-human decisions. Dispatch retries are bounded by counting `project_task.dispatch_failed` events against `max_attempts` (`attempt_count` never increments for pure dispatch failures), and the coordinator workflow arms a timer to re-dispatch when the backoff expires (dispatch otherwise only runs from signal handlers, so a scheduled retry would wait for an unrelated signal).

**Tech Stack:** Go Control Plane, sqlc/Atlas-backed Postgres repository, Temporal project coordination activities, existing project decision/inbox projection, `corepack pnpm` verification scripts.

---

## File Structure

- Modify `apps/control-plane/internal/project/types.go`: add recovery event constants, dispatch recovery request/result types, and failure-family constants for runtime/provider startup.
- Modify `apps/control-plane/internal/project/repository.go`: add a narrow `ProjectTaskDispatchRecoveryRepository` interface and request/result structs for the dispatch recovery transaction boundary.
- Modify `apps/control-plane/internal/project/service.go`: add `RecoverProjectTaskDispatchFailure`, `RecoverStaleQueuedProjectTaskAttempt`, `RecoverLostProjectTaskAttempt`, and small pure decision helpers.
- Modify `apps/control-plane/internal/project/service_test.go`: add memory-repository tests for dispatch recovery decisions and stale/lost attempt recovery behavior.
- Modify `apps/control-plane/internal/project/pg_repository.go`: implement atomic dispatch recovery and stale/lost attempt writebacks; reuse `RecoverProjectTaskAttemptFailureWriteback` where an attempt exists.
- Modify `apps/control-plane/internal/project/pg_repository_test.go`: add Postgres-backed tests for transaction behavior, idempotency, and active attempt release.
- Modify `apps/control-plane/internal/storage/queries/project.sql`: add sqlc queries for latest dispatch failure, dispatch failure counting, dispatch retry scheduling, dispatch waiting-human transition, stale queued listing, and expired running listing.
- Regenerate `apps/control-plane/internal/storage/queries/*.go` with `make -C apps/control-plane generate-sqlc`.
- Modify `apps/control-plane/internal/workflow/projectcoordination/types.go`: add `RecoverTaskDispatchFailureInput` and result types.
- Modify `apps/control-plane/internal/workflow/projectcoordination/activities.go`: add activity and store interface method for dispatch failure recovery.
- Modify `apps/control-plane/internal/workflow/projectcoordination/project_store.go`: implement workflow-facing recovery through the project service using the existing repository and inbox.
- Modify `apps/control-plane/internal/workflow/projectcoordination/workflow.go`: call recovery after recorded dispatch failures and arm a retry-backoff timer that re-dispatches the task when recovery schedules a retry.
- Modify `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`: assert recorded dispatch failure invokes recovery and still lets workflow continue.
- Modify `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`: assert run-start failure writes dispatch_failed and recovery can schedule retry or waiting-human.
- Do not modify `apps/control-plane/internal/app/app.go` in this plan. Background sweeping lands as service/repository methods plus tests; always-on worker wiring is a later plan.
- Do not modify `docs/superpowers/specs/2026-07-02-project-task-dispatch-runtime-recovery-design.md` during implementation unless a blocking contradiction is found and confirmed with the user.

## Current Baseline To Preserve

The current repository already has:

- `project_task.dispatch_failed` event support in `project.ProjectEventTaskDispatchFailed`.
- `DispatchProjectTask` writing dispatch failure events and returning `ProjectTaskDispatchError{FailureRecorded:true}`.
- `FailProjectTaskAttempt` routing transient Runtime/Provider/timeout failures through `RecoverProjectTaskAttemptFailureWriteback`.
- `project_tasks.retry_not_before`, `attempt_count`, `max_attempts`, `waiting_reason`, and `waiting_request_id`.
- `project_task_attempts` terminal statuses `lost`, `timed_out`, `failed`, and `waiting_human`.
- `task_failure_recovery` and other decision/inbox flows.

Do not replace these. Fill the missing gap: dispatch failures without attempts, queued attempts that never start, and running attempts whose lease is lost without terminal writeback.

## Task 1: Domain Types And Tests For Recovery Decisions

**Files:**
- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/service.go`
- Test: `apps/control-plane/internal/project/service_test.go`

- [ ] **Step 1: Add failing tests for pure recovery decisions**

Add these tests near existing ProjectTask attempt tests in `apps/control-plane/internal/project/service_test.go`.

The helper takes a `dispatchFailureCount` argument: the number of `project_task.dispatch_failed` events recorded for the task, including the current failure. Dispatch retries must be bounded by this count — `attempt_count` only increments when an attempt is queued (`QueueProjectTask` / `ScheduleProjectTaskRetry`), so it stays 0 for pure dispatch failures and would allow unlimited dispatch retries.

```go
func TestProjectTaskDispatchRecoveryActionSchedulesRetryForRetryableFailure(t *testing.T) {
	task := ProjectTask{
		ID:          uuid.New(),
		Status:      ProjectTaskStatusPlanned,
		MaxAttempts: serviceTestInt32Ptr(3),
	}
	event := ProjectEvent{
		ID:        uuid.New(),
		EventType: ProjectEventTaskDispatchFailed,
		Payload: map[string]any{
			"retryable": true,
			"error":     "runtime node is not connected",
		},
	}

	action := projectTaskDispatchRecoveryAction(task, event, 1)

	require.Equal(t, ProjectTaskRecoveryActionRetryScheduled, action.Action)
	require.Equal(t, FailureFamilyTransientRuntime, action.FailureFamily)
	require.True(t, action.Retryable)
	require.Equal(t, HumanWaitReasonRuntimeRecovery, action.WaitingReason)
}

func TestProjectTaskDispatchRecoveryActionMovesNonRetryableFailureToWaitingHuman(t *testing.T) {
	task := ProjectTask{
		ID:          uuid.New(),
		Status:      ProjectTaskStatusPlanned,
		MaxAttempts: serviceTestInt32Ptr(3),
	}
	event := ProjectEvent{
		ID:        uuid.New(),
		EventType: ProjectEventTaskDispatchFailed,
		Payload: map[string]any{
			"retryable": false,
			"error":     "invalid run input",
		},
	}

	action := projectTaskDispatchRecoveryAction(task, event, 1)

	require.Equal(t, ProjectTaskRecoveryActionWaitingHuman, action.Action)
	require.Equal(t, FailureFamilyInvalidContract, action.FailureFamily)
	require.False(t, action.Retryable)
	require.Equal(t, HumanWaitReasonPlanInvalid, action.WaitingReason)
}

func TestProjectTaskDispatchRecoveryActionMovesRetryExhaustionToWaitingHuman(t *testing.T) {
	task := ProjectTask{
		ID:          uuid.New(),
		Status:      ProjectTaskStatusPlanned,
		MaxAttempts: serviceTestInt32Ptr(3),
	}
	event := ProjectEvent{
		ID:        uuid.New(),
		EventType: ProjectEventTaskDispatchFailed,
		Payload: map[string]any{
			"retryable": true,
			"error":     "runtime node is not connected",
		},
	}

	action := projectTaskDispatchRecoveryAction(task, event, 3)

	require.Equal(t, ProjectTaskRecoveryActionWaitingHuman, action.Action)
	require.Equal(t, FailureFamilyTransientRuntime, action.FailureFamily)
	require.Equal(t, HumanWaitReasonRuntimeRecovery, action.WaitingReason)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestProjectTaskDispatchRecoveryAction' -count=1
```

Expected: FAIL with undefined `projectTaskDispatchRecoveryAction` and recovery action constants.

- [ ] **Step 3: Add domain constants and action struct**

In `apps/control-plane/internal/project/types.go`, add event constants after `ProjectEventTaskDispatchFailed`:

```go
ProjectEventTaskRetryScheduled     ProjectEventType = "project_task.retry_scheduled"
ProjectEventTaskAttemptLost        ProjectEventType = "project_task.attempt_lost"
ProjectEventTaskRecoveryRequested  ProjectEventType = "project_task.recovery_requested"
```

Add failure-family constants near existing `FailureFamily*` constants:

```go
FailureFamilyDispatchTransient    = "dispatch_transient"
FailureFamilyRuntimeStartTimeout  = "runtime_start_timeout"
FailureFamilyRuntimeLeaseLost     = "runtime_lease_lost"
FailureFamilyProviderStart        = "transient_provider_start"
FailureFamilyProviderConfig       = "provider_configuration"
```

Add recovery action constants and result type near ProjectTask status constants:

```go
const (
	ProjectTaskRecoveryActionNoop           = "no_op"
	ProjectTaskRecoveryActionRetryScheduled = "retry_scheduled"
	ProjectTaskRecoveryActionWaitingHuman   = "waiting_human"
	ProjectTaskRecoveryActionFailed         = "failed"
)

type ProjectTaskRecoveryAction struct {
	Action         string
	FailureFamily  string
	Retryable      bool
	RetryNotBefore *time.Time
	WaitingReason  string
	TerminalReason string
}
```

- [ ] **Step 4: Add pure recovery decision helpers**

In `apps/control-plane/internal/project/service.go`, add these helpers near `projectTaskFailureAction`.

```go
const defaultDispatchRecoveryBackoff = 2 * time.Minute

func projectTaskDispatchRecoveryAction(task ProjectTask, event ProjectEvent, dispatchFailureCount int64) ProjectTaskRecoveryAction {
	retryable := boolPayload(event.Payload, "retryable", true)
	failureFamily := dispatchRecoveryFailureFamily(event, retryable)
	waitingReason := humanWaitReasonForFailureFamily(failureFamily)
	if failureFamily == FailureFamilyDispatchTransient || failureFamily == FailureFamilyRuntimeStartTimeout || failureFamily == FailureFamilyRuntimeLeaseLost || failureFamily == FailureFamilyProviderStart {
		waitingReason = HumanWaitReasonRuntimeRecovery
	}
	if !retryable {
		return ProjectTaskRecoveryAction{
			Action:        ProjectTaskRecoveryActionWaitingHuman,
			FailureFamily: failureFamily,
			Retryable:     false,
			WaitingReason: waitingReason,
		}
	}
	if projectTaskDispatchRetryAvailable(task, dispatchFailureCount) {
		retryAt := time.Now().UTC().Add(defaultDispatchRecoveryBackoff)
		return ProjectTaskRecoveryAction{
			Action:         ProjectTaskRecoveryActionRetryScheduled,
			FailureFamily:  failureFamily,
			Retryable:      true,
			RetryNotBefore: &retryAt,
			WaitingReason:  waitingReason,
		}
	}
	return ProjectTaskRecoveryAction{
		Action:        ProjectTaskRecoveryActionWaitingHuman,
		FailureFamily: failureFamily,
		Retryable:     true,
		WaitingReason: waitingReason,
	}
}

func boolPayload(payload map[string]any, key string, fallback bool) bool {
	if payload == nil {
		return fallback
	}
	value, ok := payload[key]
	if !ok {
		return fallback
	}
	if b, ok := value.(bool); ok {
		return b
	}
	if s, ok := value.(string); ok {
		return strings.EqualFold(strings.TrimSpace(s), "true")
	}
	return fallback
}

// dispatchRecoveryFailureFamily classifies a dispatch failure. Current
// dispatch_failed events carry "error_family" fixed to "project_task_dispatch"
// (see dispatchFailurePayload), so the "failure_family" passthrough only
// applies to future writers; classification normally comes from error text.
func dispatchRecoveryFailureFamily(event ProjectEvent, retryable bool) string {
	if payloadFamily, ok := event.Payload["failure_family"].(string); ok {
		payloadFamily = strings.TrimSpace(payloadFamily)
		if payloadFamily != "" {
			return payloadFamily
		}
	}
	errorText := strings.ToLower(strings.TrimSpace(stringPayload(event.Payload, "error")))
	switch {
	case strings.Contains(errorText, "permission"), strings.Contains(errorText, "unauthorized"), strings.Contains(errorText, "forbidden"):
		return FailureFamilyPermissionRequired
	case strings.Contains(errorText, "invalid"), strings.Contains(errorText, "contract"):
		return FailureFamilyInvalidContract
	case strings.Contains(errorText, "provider"):
		if retryable {
			return FailureFamilyProviderStart
		}
		return FailureFamilyProviderConfig
	default:
		if retryable {
			return FailureFamilyTransientRuntime
		}
		return FailureFamilyInvalidContract
	}
}

func stringPayload(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	if value, ok := payload[key].(string); ok {
		return value
	}
	return ""
}

// projectTaskDispatchRetryAvailable bounds dispatch retries by the number of
// recorded dispatch_failed events. attempt_count cannot be used here: it only
// increments when an attempt is queued, so it stays 0 for pure dispatch
// failures and would allow unlimited dispatch retries.
func projectTaskDispatchRetryAvailable(task ProjectTask, dispatchFailureCount int64) bool {
	maxAttempts := int64(1)
	if task.MaxAttempts != nil {
		maxAttempts = int64(*task.MaxAttempts)
	}
	return dispatchFailureCount < maxAttempts
}
```

`service.go` already imports `strings` and `time`; preserve existing imports and let `gofmt` sort them.

- [ ] **Step 5: Run the tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestProjectTaskDispatchRecoveryAction' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/project/types.go apps/control-plane/internal/project/service.go apps/control-plane/internal/project/service_test.go
git commit -m "feat(control-plane): classify project task dispatch recovery"
```

## Task 2: Repository Support For Dispatch Recovery Without Attempt

**Files:**
- Modify: `apps/control-plane/internal/storage/queries/project.sql`
- Generated: `apps/control-plane/internal/storage/queries/project.sql.go`
- Generated: `apps/control-plane/internal/storage/queries/querier.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Test: `apps/control-plane/internal/project/pg_repository_test.go`

- [ ] **Step 1: Add failing Postgres tests**

Add tests near ProjectTask attempt writeback tests in `apps/control-plane/internal/project/pg_repository_test.go`.

```go
func TestPgRepositoryRecoverDispatchFailureSchedulesRetryWithoutAttempt(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	recoveryRepo := repo.(ProjectTaskDispatchRecoveryRepository)
	projectID := createProjectFixture(t, repo, tenantID)
	employeeID := uuid.New()
	maxAttempts := int32(3)
	task, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "dispatch retry",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
		MaxAttempts:               &maxAttempts,
	})
	require.NoError(t, err)
	failureEvent, err := repo.AppendProjectEvent(context.Background(), AppendProjectEventRequest{
		TenantID:     tenantID,
		ProjectID:    projectID,
		EventType:    ProjectEventTaskDispatchFailed,
		ActorType:    "project_coordinator",
		ActorID:      task.ID.String(),
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(task.ID.String()),
		Summary:      "项目任务分派失败",
		Payload: map[string]any{
			"project_task_id": task.ID.String(),
			"retryable":       true,
			"error":           "runtime node is not connected",
		},
	})
	require.NoError(t, err)
	retryAt := time.Now().UTC().Add(2 * time.Minute)

	result, err := recoveryRepo.RecoverProjectTaskDispatchFailure(context.Background(), RecoverProjectTaskDispatchFailureWritebackRequest{
		TenantID:       tenantID,
		ProjectID:      projectID,
		ProjectTaskID:  task.ID,
		FailureEventID: failureEvent.ID,
		Action: ProjectTaskRecoveryAction{
			Action:         ProjectTaskRecoveryActionRetryScheduled,
			FailureFamily: FailureFamilyTransientRuntime,
			Retryable:      true,
			RetryNotBefore: &retryAt,
			WaitingReason:  HumanWaitReasonRuntimeRecovery,
		},
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusPlanned, result.Task.Status)
	require.NotNil(t, result.Task.RetryNotBefore)
	require.Equal(t, ProjectEventTaskRetryScheduled, result.Event.EventType)
	require.Equal(t, task.ID.String(), result.Event.Payload["project_task_id"])
	require.Equal(t, failureEvent.ID.String(), result.Event.Payload["dispatch_failed_event_id"])
	require.Equal(t, FailureFamilyTransientRuntime, result.Event.Payload["failure_family"])
	require.Nil(t, result.Task.CurrentAttemptID)
}

func TestPgRepositoryRecoverDispatchFailureMovesToWaitingHuman(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	recoveryRepo := repo.(ProjectTaskDispatchRecoveryRepository)
	projectID := createProjectFixture(t, repo, tenantID)
	employeeID := uuid.New()
	task, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "dispatch wait human",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	failureEvent, err := repo.AppendProjectEvent(context.Background(), AppendProjectEventRequest{
		TenantID:     tenantID,
		ProjectID:    projectID,
		EventType:    ProjectEventTaskDispatchFailed,
		ActorType:    "project_coordinator",
		ActorID:      task.ID.String(),
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(task.ID.String()),
		Summary:      "项目任务分派失败",
		Payload: map[string]any{
			"project_task_id": task.ID.String(),
			"retryable":       false,
			"error":           "invalid run input",
		},
	})
	require.NoError(t, err)

	result, err := recoveryRepo.RecoverProjectTaskDispatchFailure(context.Background(), RecoverProjectTaskDispatchFailureWritebackRequest{
		TenantID:       tenantID,
		ProjectID:      projectID,
		ProjectTaskID:  task.ID,
		FailureEventID: failureEvent.ID,
		Action: ProjectTaskRecoveryAction{
			Action:         ProjectTaskRecoveryActionWaitingHuman,
			FailureFamily: FailureFamilyInvalidContract,
			Retryable:      false,
			WaitingReason:  HumanWaitReasonPlanInvalid,
		},
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusWaitingHuman, result.Task.Status)
	require.Equal(t, HumanWaitReasonPlanInvalid, *result.Task.WaitingReason)
	require.NotNil(t, result.Task.WaitingRequestID)
	require.Equal(t, ProjectEventTaskRecoveryRequested, result.Event.EventType)
	require.Equal(t, "project_task_recovery", result.Decision.DecisionType)
}
```

Assert only that a decision exists and is linked to the task; do not mutate private repository state in the test.

- [ ] **Step 2: Run the failing tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestPgRepositoryRecoverDispatchFailure' -count=1
```

Expected: FAIL with undefined `ProjectTaskDispatchRecoveryRepository` and writeback request type.

- [ ] **Step 3: Add sqlc queries**

Append these queries to `apps/control-plane/internal/storage/queries/project.sql` near ProjectTask attempt queries.

```sql
-- name: GetProjectTaskLatestDispatchFailureEvent :one
SELECT *
FROM project_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND event_type = 'project_task.dispatch_failed'
  AND actor_id = sqlc.arg('project_task_id')::uuid::text
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: CountProjectTaskDispatchFailureEvents :one
SELECT COUNT(*)
FROM project_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND event_type = 'project_task.dispatch_failed'
  AND actor_id = sqlc.arg('project_task_id')::uuid::text;

-- name: ScheduleProjectTaskDispatchRetry :one
UPDATE project_tasks
SET status = 'planned',
    retry_not_before = sqlc.arg('retry_not_before')::timestamptz,
    waiting_reason = NULL,
    waiting_request_id = NULL,
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    status_changed_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status IN ('planned', 'waiting_human')
  AND current_attempt_id IS NULL
RETURNING *;

-- name: MoveProjectTaskDispatchFailureToWaitingHuman :one
UPDATE project_tasks
SET status = 'waiting_human',
    waiting_reason = sqlc.arg('waiting_reason')::varchar,
    waiting_request_id = sqlc.narg('waiting_request_id')::uuid,
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    status_changed_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status IN ('planned', 'waiting_human')
  AND current_attempt_id IS NULL
RETURNING *;
```

`ScheduleProjectTaskDispatchRetry` resets the task to `planned`: leaving a `waiting_human` task with cleared waiting fields would be an inconsistent state. The service-level guard (Task 3) refuses to run recovery while a pending waiting-human decision exists (`waiting_request_id` set), so this transition never clobbers an unresolved decision.

- [ ] **Step 4: Generate sqlc**

Run:

```bash
make -C apps/control-plane generate-sqlc
```

Expected: regenerated query bindings compile. If `sqlc` is missing, stop and report the missing tool; do not hand-edit generated files.

- [ ] **Step 5: Add repository interface and writeback structs**

In `apps/control-plane/internal/project/repository.go`, add:

```go
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
```

- [ ] **Step 6: Implement PgRepository methods**

In `apps/control-plane/internal/project/pg_repository.go`, add:

```go
func (r *PgRepository) GetProjectTaskLatestDispatchFailureEvent(ctx context.Context, tenantID, projectID, projectTaskID uuid.UUID) (ProjectEvent, error) {
	row, err := r.q.GetProjectTaskLatestDispatchFailureEvent(ctx, queries.GetProjectTaskLatestDispatchFailureEventParams{
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: projectTaskID,
	})
	if err != nil {
		return ProjectEvent{}, projectRepositoryError(err)
	}
	return eventFromRecord(row)
}

func (r *PgRepository) CountProjectTaskDispatchFailureEvents(ctx context.Context, tenantID, projectID, projectTaskID uuid.UUID) (int64, error) {
	count, err := r.q.CountProjectTaskDispatchFailureEvents(ctx, queries.CountProjectTaskDispatchFailureEventsParams{
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: projectTaskID,
	})
	if err != nil {
		return 0, projectRepositoryError(err)
	}
	return count, nil
}

func (r *PgRepository) ListStaleQueuedProjectTaskAttempts(ctx context.Context, tenantID uuid.UUID, startedBefore time.Time, limit int32) ([]ProjectTaskAttempt, error) {
	rows, err := r.q.ListStaleQueuedProjectTaskAttempts(ctx, queries.ListStaleQueuedProjectTaskAttemptsParams{
		TenantID:      tenantID,
		StartedBefore: pgtype.Timestamptz{Time: startedBefore, Valid: true},
		Limit:         limit,
	})
	if err != nil {
		return nil, projectRepositoryError(err)
	}
	attempts := make([]ProjectTaskAttempt, 0, len(rows))
	for _, row := range rows {
		attempt, err := projectTaskAttemptFromRecord(row)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	return attempts, nil
}

func (r *PgRepository) ListExpiredRunningProjectTaskAttempts(ctx context.Context, tenantID uuid.UUID, now time.Time, limit int32) ([]ProjectTaskAttempt, error) {
	rows, err := r.q.ListExpiredRunningProjectTaskAttempts(ctx, queries.ListExpiredRunningProjectTaskAttemptsParams{
		TenantID: tenantID,
		Now:      pgtype.Timestamptz{Time: now, Valid: true},
		Limit:    limit,
	})
	if err != nil {
		return nil, projectRepositoryError(err)
	}
	attempts := make([]ProjectTaskAttempt, 0, len(rows))
	for _, row := range rows {
		attempt, err := projectTaskAttemptFromRecord(row)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	return attempts, nil
}

func (r *PgRepository) RecoverProjectTaskDispatchFailure(ctx context.Context, req RecoverProjectTaskDispatchFailureWritebackRequest) (ProjectTaskWritebackResult, error) {
	return withProjectQueries(ctx, r, "project task dispatch recovery", func(q *queries.Queries) (ProjectTaskWritebackResult, error) {
		task, err := q.GetProjectTask(ctx, queries.GetProjectTaskParams{TenantID: req.TenantID, ID: req.ProjectTaskID})
		if err != nil {
			return ProjectTaskWritebackResult{}, projectRepositoryError(err)
		}
		projectTask, err := taskFromRecord(task)
		if err != nil {
			return ProjectTaskWritebackResult{}, err
		}
		if projectTask.ProjectID != req.ProjectID {
			return ProjectTaskWritebackResult{}, ErrProjectNotFound
		}

		switch req.Action.Action {
		case ProjectTaskRecoveryActionRetryScheduled:
			return r.recoverDispatchFailureRetryWithQueries(ctx, q, projectTask, req)
		case ProjectTaskRecoveryActionWaitingHuman:
			return r.recoverDispatchFailureWaitingHumanWithQueries(ctx, q, projectTask, req)
		case ProjectTaskRecoveryActionFailed:
			return r.recoverDispatchFailureFailedWithQueries(ctx, q, projectTask, req)
		default:
			return ProjectTaskWritebackResult{Task: projectTask}, nil
		}
	})
}
```

Then add helper methods:

```go
func (r *PgRepository) recoverDispatchFailureRetryWithQueries(ctx context.Context, q *queries.Queries, task ProjectTask, req RecoverProjectTaskDispatchFailureWritebackRequest) (ProjectTaskWritebackResult, error) {
	if req.Action.RetryNotBefore == nil {
		return ProjectTaskWritebackResult{}, ErrInvalidProject
	}
	event, err := r.appendProjectEventWithQueries(ctx, q, AppendProjectEventRequest{
		TenantID:     req.TenantID,
		ProjectID:    req.ProjectID,
		EventType:    ProjectEventTaskRetryScheduled,
		ActorType:    "project_coordinator",
		ActorID:      task.ID.String(),
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(task.ID.String()),
		Summary:      "项目任务已安排重新分派",
		Payload: map[string]any{
			"project_task_id":          task.ID.String(),
			"dispatch_failed_event_id": req.FailureEventID.String(),
			"failure_family":           req.Action.FailureFamily,
			"retryable":                req.Action.Retryable,
			"retry_not_before":         req.Action.RetryNotBefore.Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	row, err := q.ScheduleProjectTaskDispatchRetry(ctx, queries.ScheduleProjectTaskDispatchRetryParams{
		TenantID:       req.TenantID,
		ProjectID:      req.ProjectID,
		ID:             task.ID,
		RetryNotBefore: pgtype.Timestamptz{Time: *req.Action.RetryNotBefore, Valid: true},
		LatestEventID:  nullUUID(&event.ID),
	})
	if err != nil {
		return ProjectTaskWritebackResult{}, projectRepositoryError(err)
	}
	updated, err := taskFromRecord(row)
	if err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	return ProjectTaskWritebackResult{Task: updated, Event: event}, nil
}
```

Use the repository's existing `nullUUID`, `strPtr`, `taskFromRecord`, `appendProjectEventWithQueries`, `createDecisionRequestWithQueries`, `updateProjectTaskStatusWithQueries`, and `projectRepositoryError` helpers. Follow existing pgx `pgtype.Timestamptz` usage in the file for imports.

Add the waiting-human helper:

```go
func (r *PgRepository) recoverDispatchFailureWaitingHumanWithQueries(ctx context.Context, q *queries.Queries, task ProjectTask, req RecoverProjectTaskDispatchFailureWritebackRequest) (ProjectTaskWritebackResult, error) {
	projectRow, err := q.GetProject(ctx, queries.GetProjectParams{TenantID: req.TenantID, ID: req.ProjectID})
	if err != nil {
		return ProjectTaskWritebackResult{}, projectRepositoryError(err)
	}
	projectRecord, err := projectFromRecord(projectRow)
	if err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	event, err := r.appendProjectEventWithQueries(ctx, q, AppendProjectEventRequest{
		TenantID:     req.TenantID,
		ProjectID:    req.ProjectID,
		EventType:    ProjectEventTaskRecoveryRequested,
		ActorType:    "project_coordinator",
		ActorID:      task.ID.String(),
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(task.ID.String()),
		Summary:      "项目任务分派失败，需要人工恢复决策",
		Payload: map[string]any{
			"project_task_id":          task.ID.String(),
			"dispatch_failed_event_id": req.FailureEventID.String(),
			"failure_family":           req.Action.FailureFamily,
			"retryable":                req.Action.Retryable,
			"waiting_reason":           req.Action.WaitingReason,
		},
	})
	if err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	decision, err := r.createDecisionRequestWithQueries(ctx, q, CreateDecisionRequestRequest{
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		ProjectTaskID:     &task.ID,
		TargetUserID:      projectRecord.HumanOwnerUserID,
		DecisionType:      "project_task_recovery",
		TitleSnapshot:     task.Title,
		SummarySnapshot:   "项目任务分派失败，需要人工恢复决策",
		RiskLevelSnapshot: stringValue(task.RiskLevel),
		StatusSnapshot:    "pending",
		CreatedEventID:    &event.ID,
	})
	if err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	row, err := q.MoveProjectTaskDispatchFailureToWaitingHuman(ctx, queries.MoveProjectTaskDispatchFailureToWaitingHumanParams{
		TenantID:         req.TenantID,
		ProjectID:        req.ProjectID,
		ID:               task.ID,
		WaitingReason:    req.Action.WaitingReason,
		WaitingRequestID: nullUUID(&decision.ID),
		LatestEventID:    nullUUID(&event.ID),
	})
	if err != nil {
		return ProjectTaskWritebackResult{}, projectRepositoryError(err)
	}
	updated, err := taskFromRecord(row)
	if err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	return ProjectTaskWritebackResult{Task: updated, Event: event, Decision: decision}, nil
}
```

Add the failed helper:

```go
func (r *PgRepository) recoverDispatchFailureFailedWithQueries(ctx context.Context, q *queries.Queries, task ProjectTask, req RecoverProjectTaskDispatchFailureWritebackRequest) (ProjectTaskWritebackResult, error) {
	reason := strings.TrimSpace(req.Action.TerminalReason)
	if reason == "" {
		reason = "dispatch recovery marked the project task failed"
	}
	event, err := r.appendProjectEventWithQueries(ctx, q, AppendProjectEventRequest{
		TenantID:     req.TenantID,
		ProjectID:    req.ProjectID,
		EventType:    ProjectEventTaskFailed,
		ActorType:    "project_coordinator",
		ActorID:      task.ID.String(),
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(task.ID.String()),
		Summary:      reason,
		Payload: map[string]any{
			"project_task_id":          task.ID.String(),
			"dispatch_failed_event_id": req.FailureEventID.String(),
			"failure_family":           req.Action.FailureFamily,
			"terminal_reason":          reason,
		},
	})
	if err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	updated, err := r.updateProjectTaskStatusWithQueries(ctx, q, req.TenantID, task.ID, ProjectTaskStatusFailed, &event.ID, []string{ProjectTaskStatusPlanned, ProjectTaskStatusWaitingHuman})
	if err != nil {
		return ProjectTaskWritebackResult{}, projectRepositoryError(err)
	}
	return ProjectTaskWritebackResult{Task: updated, Event: event}, nil
}
```

- [ ] **Step 7: Run repository tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestPgRepositoryRecoverDispatchFailure' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/control-plane/internal/storage/queries/project.sql apps/control-plane/internal/storage/queries/project.sql.go apps/control-plane/internal/storage/queries/querier.go apps/control-plane/internal/project/repository.go apps/control-plane/internal/project/pg_repository.go apps/control-plane/internal/project/pg_repository_test.go
git commit -m "feat(control-plane): persist project task dispatch recovery"
```

## Task 3: Project Service Methods For Dispatch And Attempt Recovery

**Files:**
- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/service_test.go`

- [ ] **Step 1: Add failing service tests**

Add tests in `apps/control-plane/internal/project/service_test.go`.

```go
func TestRecoverProjectTaskDispatchFailureSchedulesRetry(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newDispatchRecoveryFixture(repo, ProjectTaskStatusPlanned, 0, 3, true)

	result, err := service.RecoverProjectTaskDispatchFailure(context.Background(), RecoverProjectTaskDispatchFailureRequest{
		TenantID:       fixture.tenantID,
		ProjectID:      fixture.projectID,
		ProjectTaskID:  fixture.taskID,
		FailureEventID: fixture.failureEventID,
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskRecoveryActionRetryScheduled, result.Action)
	task, err := repo.GetProjectTask(context.Background(), fixture.tenantID, fixture.taskID)
	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusPlanned, task.Status)
	require.NotNil(t, task.RetryNotBefore)
	require.Len(t, repo.events, 2)
	require.Equal(t, ProjectEventTaskRetryScheduled, repo.events[1].EventType)
}

func TestRecoverProjectTaskDispatchFailureCreatesWaitingHumanDecision(t *testing.T) {
	repo := newMemoryRepository()
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, inbox, nil)
	require.NoError(t, err)
	fixture := newDispatchRecoveryFixture(repo, ProjectTaskStatusPlanned, 0, 3, false)

	result, err := service.RecoverProjectTaskDispatchFailure(context.Background(), RecoverProjectTaskDispatchFailureRequest{
		TenantID:       fixture.tenantID,
		ProjectID:      fixture.projectID,
		ProjectTaskID:  fixture.taskID,
		FailureEventID: fixture.failureEventID,
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskRecoveryActionWaitingHuman, result.Action)
	task, err := repo.GetProjectTask(context.Background(), fixture.tenantID, fixture.taskID)
	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusWaitingHuman, task.Status)
	require.NotNil(t, task.WaitingRequestID)
	require.Len(t, inbox.upserts, 1)
	require.Equal(t, "project_task_recovery", inbox.upserts[0].DecisionType)
}

func TestRecoverProjectTaskDispatchFailureNoopsWhenDecisionPending(t *testing.T) {
	repo := newMemoryRepository()
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, inbox, nil)
	require.NoError(t, err)
	fixture := newDispatchRecoveryFixture(repo, ProjectTaskStatusPlanned, 0, 3, false)

	first, err := service.RecoverProjectTaskDispatchFailure(context.Background(), RecoverProjectTaskDispatchFailureRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
	})
	require.NoError(t, err)
	require.Equal(t, ProjectTaskRecoveryActionWaitingHuman, first.Action)

	second, err := service.RecoverProjectTaskDispatchFailure(context.Background(), RecoverProjectTaskDispatchFailureRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
	})
	require.NoError(t, err)
	require.Equal(t, ProjectTaskRecoveryActionNoop, second.Action)
	require.Len(t, inbox.upserts, 1)
	require.Len(t, repo.decisionRequests, 1)
}

func TestRecoverProjectTaskDispatchFailureExhaustsRetriesByFailureCount(t *testing.T) {
	repo := newMemoryRepository()
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, inbox, nil)
	require.NoError(t, err)
	fixture := newDispatchRecoveryFixture(repo, ProjectTaskStatusPlanned, 0, 3, true)
	for i := 0; i < 2; i++ {
		repo.events = append(repo.events, ProjectEvent{
			ID:        uuid.New(),
			TenantID:  fixture.tenantID,
			ProjectID: fixture.projectID,
			EventType: ProjectEventTaskDispatchFailed,
			ActorType: "project_coordinator",
			ActorID:   fixture.taskID.String(),
			Payload: map[string]any{
				"project_task_id": fixture.taskID.String(),
				"retryable":       true,
				"error":           "runtime node is not connected",
			},
			CreatedAt: time.Now().UTC(),
		})
	}

	result, err := service.RecoverProjectTaskDispatchFailure(context.Background(), RecoverProjectTaskDispatchFailureRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskRecoveryActionWaitingHuman, result.Action)
	task, err := repo.GetProjectTask(context.Background(), fixture.tenantID, fixture.taskID)
	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusWaitingHuman, task.Status)
}
```

The exhaustion test proves the retry bound: 3 dispatch failures with `max_attempts=3` must escalate to waiting-human even though `attempt_count` is still 0. The noop test proves Temporal activity retries cannot duplicate decisions.

Add fixture helper:

```go
type dispatchRecoveryFixture struct {
	tenantID       uuid.UUID
	projectID      uuid.UUID
	taskID         uuid.UUID
	failureEventID uuid.UUID
}

func newDispatchRecoveryFixture(repo *memoryRepository, taskStatus string, attemptCount, maxAttempts int32, retryable bool) dispatchRecoveryFixture {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	ownerID := uuid.New()
	employeeID := uuid.New()
	now := time.Now().UTC()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "Dispatch recovery",
		Goal:             "Recover dispatch",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "dispatch recovery task",
		Status:                    taskStatus,
		AssignedDigitalEmployeeID: &employeeID,
		AttemptCount:              attemptCount,
		MaxAttempts:               &maxAttempts,
		CreatedAt:                 now,
		UpdatedAt:                 now,
	})
	eventID := uuid.New()
	repo.events = append(repo.events, ProjectEvent{
		ID:        eventID,
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: ProjectEventTaskDispatchFailed,
		ActorType: "project_coordinator",
		ActorID:   taskID.String(),
		Payload: map[string]any{
			"project_task_id": taskID.String(),
			"retryable":       retryable,
			"error":           map[bool]string{true: "runtime node is not connected", false: "invalid run input"}[retryable],
		},
		CreatedAt: now,
	})
	return dispatchRecoveryFixture{tenantID: tenantID, projectID: projectID, taskID: taskID, failureEventID: eventID}
}
```

- [ ] **Step 2: Run failing tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestRecoverProjectTaskDispatchFailure' -count=1
```

Expected: FAIL with undefined request/result and service method.

- [ ] **Step 3: Add service request/result types**

In `apps/control-plane/internal/project/types.go`, add:

```go
type RecoverProjectTaskDispatchFailureRequest struct {
	TenantID       uuid.UUID
	ProjectID      uuid.UUID
	ProjectTaskID  uuid.UUID
	FailureEventID uuid.UUID
}

type RecoverProjectTaskDispatchFailureResult struct {
	Task     ProjectTask
	Event    ProjectEvent
	Decision DecisionRequest
	Action   string
}

type RecoverProjectTaskAttemptRequest struct {
	TenantID      uuid.UUID
	ProjectID     uuid.UUID
	ProjectTaskID uuid.UUID
	AttemptID     uuid.UUID
	FailureFamily string
	Summary       string
	Now           time.Time
}

type SweepProjectTaskAttemptRecoveryRequest struct {
	TenantID uuid.UUID
	Now      time.Time
	Limit    int32
}

type SweepProjectTaskAttemptRecoveryResult struct {
	RecoveredAttemptIDs []uuid.UUID
	RecoveredTaskIDs    []uuid.UUID
}
```

- [ ] **Step 4: Implement service dispatch recovery method**

In `apps/control-plane/internal/project/service.go`, add:

```go
func (s *Service) RecoverProjectTaskDispatchFailure(ctx context.Context, req RecoverProjectTaskDispatchFailureRequest) (*RecoverProjectTaskDispatchFailureResult, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ProjectTaskID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	repository, ok := s.repository.(ProjectTaskDispatchRecoveryRepository)
	if !ok {
		return nil, ErrInvalidProject
	}
	task, err := s.repository.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
	if err != nil {
		return nil, err
	}
	if task.ProjectID != req.ProjectID {
		return nil, ErrProjectNotFound
	}
	if task.CurrentAttemptID != nil {
		return &RecoverProjectTaskDispatchFailureResult{Task: task, Action: ProjectTaskRecoveryActionNoop}, nil
	}
	if task.Status == ProjectTaskStatusWaitingHuman && task.WaitingRequestID != nil {
		return &RecoverProjectTaskDispatchFailureResult{Task: task, Action: ProjectTaskRecoveryActionNoop}, nil
	}
	if task.RetryNotBefore != nil && task.RetryNotBefore.After(time.Now().UTC()) {
		return &RecoverProjectTaskDispatchFailureResult{Task: task, Action: ProjectTaskRecoveryActionNoop}, nil
	}
	event, err := repository.GetProjectTaskLatestDispatchFailureEvent(ctx, req.TenantID, req.ProjectID, req.ProjectTaskID)
	if err != nil {
		return nil, err
	}
	if req.FailureEventID != uuid.Nil && event.ID != req.FailureEventID {
		return &RecoverProjectTaskDispatchFailureResult{Task: task, Event: event, Action: ProjectTaskRecoveryActionNoop}, nil
	}
	dispatchFailureCount, err := repository.CountProjectTaskDispatchFailureEvents(ctx, req.TenantID, req.ProjectID, req.ProjectTaskID)
	if err != nil {
		return nil, err
	}
	action := projectTaskDispatchRecoveryAction(task, event, dispatchFailureCount)
	result, err := repository.RecoverProjectTaskDispatchFailure(ctx, RecoverProjectTaskDispatchFailureWritebackRequest{
		TenantID:       req.TenantID,
		ProjectID:      req.ProjectID,
		ProjectTaskID:  req.ProjectTaskID,
		FailureEventID: event.ID,
		Action:         action,
	})
	if err != nil {
		return nil, err
	}
	if result.Decision.ID != uuid.Nil && s.inbox != nil {
		if err := s.inbox.UpsertProjectDecisionRequest(ctx, result.Decision); err != nil {
			return nil, err
		}
	}
	return &RecoverProjectTaskDispatchFailureResult{
		Task:     result.Task,
		Event:    result.Event,
		Decision: result.Decision,
		Action:   action.Action,
	}, nil
}
```

The two noop guards make this method idempotent under Temporal activity retries: a pending waiting-human decision is never duplicated, and a not-yet-due retry is never re-scheduled. `FailureEventID` is optional — the coordinator workflow cannot transport it across the activity boundary (see Task 4) and omits it; the service resolves the latest `dispatch_failed` event itself.

- [ ] **Step 5: Update memory repository**

In `apps/control-plane/internal/project/service_test.go`, implement `ProjectTaskDispatchRecoveryRepository` on `memoryRepository`:

```go
func (r *memoryRepository) GetProjectTaskLatestDispatchFailureEvent(ctx context.Context, tenantID, projectID, projectTaskID uuid.UUID) (ProjectEvent, error) {
	for i := len(r.events) - 1; i >= 0; i-- {
		event := r.events[i]
		if event.TenantID == tenantID && event.ProjectID == projectID && event.EventType == ProjectEventTaskDispatchFailed && event.ActorID == projectTaskID.String() {
			return event, nil
		}
	}
	return ProjectEvent{}, ErrProjectNotFound
}

func (r *memoryRepository) CountProjectTaskDispatchFailureEvents(ctx context.Context, tenantID, projectID, projectTaskID uuid.UUID) (int64, error) {
	var count int64
	for _, event := range r.events {
		if event.TenantID == tenantID && event.ProjectID == projectID && event.EventType == ProjectEventTaskDispatchFailed && event.ActorID == projectTaskID.String() {
			count++
		}
	}
	return count, nil
}

func (r *memoryRepository) RecoverProjectTaskDispatchFailure(ctx context.Context, req RecoverProjectTaskDispatchFailureWritebackRequest) (ProjectTaskWritebackResult, error) {
	task, err := r.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
	if err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	switch req.Action.Action {
	case ProjectTaskRecoveryActionRetryScheduled:
		event, err := r.AppendProjectEvent(ctx, AppendProjectEventRequest{
			TenantID:  req.TenantID,
			ProjectID: req.ProjectID,
			EventType: ProjectEventTaskRetryScheduled,
			ActorType: "project_coordinator",
			ActorID:   req.ProjectTaskID.String(),
			Payload: map[string]any{
				"project_task_id":          req.ProjectTaskID.String(),
				"dispatch_failed_event_id": req.FailureEventID.String(),
				"failure_family":           req.Action.FailureFamily,
				"retryable":                req.Action.Retryable,
			},
		})
		if err != nil {
			return ProjectTaskWritebackResult{}, err
		}
		for i := range r.tasks {
			if r.tasks[i].TenantID == req.TenantID && r.tasks[i].ID == req.ProjectTaskID {
				r.tasks[i].Status = ProjectTaskStatusPlanned
				r.tasks[i].RetryNotBefore = req.Action.RetryNotBefore
				r.tasks[i].WaitingReason = nil
				r.tasks[i].WaitingRequestID = nil
				r.tasks[i].LatestEventID = &event.ID
				return ProjectTaskWritebackResult{Task: r.tasks[i], Event: event}, nil
			}
		}
	case ProjectTaskRecoveryActionWaitingHuman:
		event, err := r.AppendProjectEvent(ctx, AppendProjectEventRequest{
			TenantID:  req.TenantID,
			ProjectID: req.ProjectID,
			EventType: ProjectEventTaskRecoveryRequested,
			ActorType: "project_coordinator",
			ActorID:   req.ProjectTaskID.String(),
			Payload: map[string]any{
				"project_task_id":          req.ProjectTaskID.String(),
				"dispatch_failed_event_id": req.FailureEventID.String(),
				"failure_family":           req.Action.FailureFamily,
				"retryable":                req.Action.Retryable,
			},
		})
		if err != nil {
			return ProjectTaskWritebackResult{}, err
		}
		decision, err := r.CreateDecisionRequest(ctx, CreateDecisionRequestRequest{
			TenantID:          req.TenantID,
			ProjectID:         req.ProjectID,
			ProjectTaskID:     &req.ProjectTaskID,
			TargetUserID:      r.projects[req.ProjectID].HumanOwnerUserID,
			DecisionType:      "project_task_recovery",
			TitleSnapshot:     task.Title,
			SummarySnapshot:   "项目任务分派失败，需要人工恢复决策",
			RiskLevelSnapshot: stringValue(task.RiskLevel),
			StatusSnapshot:    "pending",
			CreatedEventID:    &event.ID,
		})
		if err != nil {
			return ProjectTaskWritebackResult{}, err
		}
		for i := range r.tasks {
			if r.tasks[i].TenantID == req.TenantID && r.tasks[i].ID == req.ProjectTaskID {
				r.tasks[i].Status = ProjectTaskStatusWaitingHuman
				r.tasks[i].WaitingReason = &req.Action.WaitingReason
				r.tasks[i].WaitingRequestID = &decision.ID
				r.tasks[i].LatestEventID = &event.ID
				return ProjectTaskWritebackResult{Task: r.tasks[i], Event: event, Decision: decision}, nil
			}
		}
	default:
		return ProjectTaskWritebackResult{Task: task}, nil
	}
	return ProjectTaskWritebackResult{}, ErrProjectNotFound
}
```

Add candidate-list methods to the same memory repository:

```go
func (r *memoryRepository) ListStaleQueuedProjectTaskAttempts(ctx context.Context, tenantID uuid.UUID, startedBefore time.Time, limit int32) ([]ProjectTaskAttempt, error) {
	result := []ProjectTaskAttempt{}
	for _, attempt := range r.projectTaskAttempts {
		if attempt.TenantID != tenantID || attempt.Status != ProjectTaskAttemptStatusQueued || attempt.StartedAt != nil {
			continue
		}
		if !attempt.CreatedAt.Before(startedBefore) {
			continue
		}
		task, err := r.GetProjectTask(ctx, tenantID, attempt.ProjectTaskID)
		if err != nil || task.Status != ProjectTaskStatusQueued {
			continue
		}
		result = append(result, attempt)
		if int32(len(result)) >= limit {
			break
		}
	}
	return result, nil
}

func (r *memoryRepository) ListExpiredRunningProjectTaskAttempts(ctx context.Context, tenantID uuid.UUID, now time.Time, limit int32) ([]ProjectTaskAttempt, error) {
	result := []ProjectTaskAttempt{}
	for _, attempt := range r.projectTaskAttempts {
		if attempt.TenantID != tenantID || attempt.Status != ProjectTaskAttemptStatusRunning || attempt.LeaseExpiresAt == nil || !attempt.LeaseExpiresAt.Before(now) {
			continue
		}
		task, err := r.GetProjectTask(ctx, tenantID, attempt.ProjectTaskID)
		if err != nil || task.Status != ProjectTaskStatusRunning {
			continue
		}
		result = append(result, attempt)
		if int32(len(result)) >= limit {
			break
		}
	}
	return result, nil
}
```

- [ ] **Step 6: Run service tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestRecoverProjectTaskDispatchFailure|TestProjectTaskDispatchRecoveryAction' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/project/types.go apps/control-plane/internal/project/service.go apps/control-plane/internal/project/service_test.go
git commit -m "feat(control-plane): recover project task dispatch failures"
```

## Task 4: Workflow Activity Calls Recovery After Recorded Dispatch Failure

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/types.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/activities.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow.go`
- Test: `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`
- Test: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`

- [ ] **Step 1: Add failing workflow test**

Modify `TestProjectCoordinatorContinuesAfterRecordedDispatchFailure` in `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go` to assert the new recovery call:

```go
require.Contains(t, store.calls, "RecoverTaskDispatchFailure")
require.Contains(t, store.calls, "FinishCoordinationJob")
```

Add fields to `recordingActivityStore`:

```go
recoverInputs  []RecoverTaskDispatchFailureInput
recoverResults []RecoverTaskDispatchFailureResult
recoverErr     error
```

Add method (default to waiting-human so tests without queued results do not arm retry timers):

```go
func (s *recordingActivityStore) RecoverTaskDispatchFailure(ctx context.Context, input RecoverTaskDispatchFailureInput) (RecoverTaskDispatchFailureResult, error) {
	s.calls = append(s.calls, "RecoverTaskDispatchFailure")
	s.recoverInputs = append(s.recoverInputs, input)
	if s.recoverErr != nil {
		return RecoverTaskDispatchFailureResult{}, s.recoverErr
	}
	if len(s.recoverResults) > 0 {
		result := s.recoverResults[0]
		s.recoverResults = s.recoverResults[1:]
		return result, nil
	}
	return RecoverTaskDispatchFailureResult{Action: project.ProjectTaskRecoveryActionWaitingHuman}, nil
}
```

Add a second failing test for the retry-backoff timer, mirroring the setup of `TestProjectCoordinatorContinuesAfterRecordedDispatchFailure`. The Temporal test environment auto-advances timers, so the 2-minute backoff completes immediately in test time; the shutdown callback must be registered after the backoff (10 minutes of mock time):

```go
func TestProjectCoordinatorRedispatchesAfterRetryBackoff(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	executorID := uuid.New()
	retryAt := time.Now().Add(2 * time.Minute)
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{
			ProjectID: uuid.New(),
			Demand: DemandSnapshot{
				ID:      uuid.New(),
				Title:   "验证 Runtime",
				Content: "检查心跳",
			},
			DigitalEmployeePool: []ProjectMemberSnapshot{
				{PrincipalID: executorID, ProjectRole: "executor", Status: "active"},
			},
		},
		jobID:         uuid.New(),
		routeID:       uuid.New(),
		routeEventID:  uuid.New(),
		taskID:        uuid.New(),
		dispatchEvent: uuid.New(),
		dispatchErr:   &ProjectTaskDispatchError{FailureRecorded: true, Err: project.ErrInvalidProject},
		recoverResults: []RecoverTaskDispatchFailureResult{
			{Action: project.ProjectTaskRecoveryActionRetryScheduled, RetryNotBefore: &retryAt},
			{Action: project.ProjectTaskRecoveryActionWaitingHuman},
		},
	}
	store.dispatchableTaskIDs = []uuid.UUID{store.taskID}
	activities := NewActivities(store, HeuristicRoutePlanner{})
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalDemandSubmitted, DemandSubmitted{
			ProjectID:         store.snapshot.ProjectID,
			DemandID:          store.snapshot.Demand.ID,
			SubmittedByUserID: uuid.New(),
			CreatedEventID:    uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Minute)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  store.snapshot.ProjectID,
		WorkflowID: "project-coordinator:" + store.snapshot.ProjectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Len(t, store.recoverInputs, 2)
}
```

`require.Len(t, store.recoverInputs, 2)` is the timer proof: the second recovery call can only happen if the timer re-dispatched the task and that dispatch failed again. The second recovery returns waiting-human, so no further timer is armed and the loop terminates.

- [ ] **Step 2: Run failing workflow test**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectCoordinatorContinuesAfterRecordedDispatchFailure|TestProjectCoordinatorRedispatchesAfterRetryBackoff' -count=1
```

Expected: FAIL because `RecoverTaskDispatchFailure` is never called and no retry timer re-dispatches.

- [ ] **Step 3: Add workflow types**

In `apps/control-plane/internal/workflow/projectcoordination/types.go`, add:

```go
type RecoverTaskDispatchFailureInput struct {
	TenantID      uuid.UUID
	ProjectID     uuid.UUID
	ProjectTaskID uuid.UUID
}

type RecoverTaskDispatchFailureResult struct {
	Action         string
	RetryNotBefore *time.Time
}
```

There is deliberately no `FailureEventID` field: a `*ProjectTaskDispatchError` does not survive the Temporal activity boundary (the workflow receives a `*temporal.ApplicationError`, which is why `dispatchFailureRecorded` already falls back to matching `appErr.Type()`), so the workflow cannot learn the failure event ID from the dispatch error. The recovery service resolves the latest `project_task.dispatch_failed` event itself.

- [ ] **Step 4: Extend activity store and activity**

In `apps/control-plane/internal/workflow/projectcoordination/activities.go`, add to `ActivityStore`:

```go
RecoverTaskDispatchFailure(ctx context.Context, input RecoverTaskDispatchFailureInput) (RecoverTaskDispatchFailureResult, error)
```

Add activity method:

```go
func (a *Activities) RecoverTaskDispatchFailure(ctx context.Context, input RecoverTaskDispatchFailureInput) (RecoverTaskDispatchFailureResult, error) {
	if a.store == nil {
		return RecoverTaskDispatchFailureResult{}, ErrActivityStoreRequired
	}
	return a.store.RecoverTaskDispatchFailure(ctx, input)
}
```

- [ ] **Step 5: Implement ProjectStore recovery method**

In `apps/control-plane/internal/workflow/projectcoordination/project_store.go`, add:

```go
type dispatchRecoveryService interface {
	RecoverProjectTaskDispatchFailure(ctx context.Context, req project.RecoverProjectTaskDispatchFailureRequest) (*project.RecoverProjectTaskDispatchFailureResult, error)
}
```

`ProjectStore` only has `project.Repository`, so keep app wiring unchanged. Build a short-lived project service inside the store method and pass the existing repository and inbox:

```go
func (s *ProjectStore) RecoverTaskDispatchFailure(ctx context.Context, input RecoverTaskDispatchFailureInput) (RecoverTaskDispatchFailureResult, error) {
	if s.repository == nil {
		return RecoverTaskDispatchFailureResult{}, ErrActivityStoreRequired
	}
	service, err := project.NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(s.repository, project.NoopCoordinatorSignalClient{}, nil, s.inbox, nil)
	if err != nil {
		return RecoverTaskDispatchFailureResult{}, err
	}
	result, err := service.RecoverProjectTaskDispatchFailure(ctx, project.RecoverProjectTaskDispatchFailureRequest{
		TenantID:      input.TenantID,
		ProjectID:     input.ProjectID,
		ProjectTaskID: input.ProjectTaskID,
	})
	if err != nil {
		return RecoverTaskDispatchFailureResult{}, err
	}
	return RecoverTaskDispatchFailureResult{Action: result.Action, RetryNotBefore: result.Task.RetryNotBefore}, nil
}
```

This keeps app wiring small and avoids a new worker dependency in this plan.

- [ ] **Step 6: Leave `ProjectTaskDispatchError` unchanged**

Do not extend `ProjectTaskDispatchError` with a failure event ID. Concrete error types do not survive the Temporal activity boundary: in real execution the workflow receives a `*temporal.ApplicationError` whose original struct fields are gone (`dispatchFailureRecorded` in `types.go` already handles exactly this by matching `appErr.Type() == "ProjectTaskDispatchError"`). Any `FailureEventID` carried through the error would always read as zero outside unit tests. The recovery service resolves the latest `project_task.dispatch_failed` event for the task instead, which is equivalent because `DispatchProjectTask` records the failure event immediately before returning the error.

- [ ] **Step 7: Call recovery after recorded dispatch failure and arm the retry timer**

In `dispatchProjectTasks` in `workflow.go`, change the recorded failure branch:

```go
if !dispatchFailureRecorded(err) {
	return err
}
workflow.GetLogger(ctx).Warn("dispatch project task failed", "task_id", taskID.String(), "error", err.Error())
var recovery RecoverTaskDispatchFailureResult
if recoverErr := workflow.ExecuteActivity(ctx, (*Activities).RecoverTaskDispatchFailure, RecoverTaskDispatchFailureInput{
	TenantID:      tenantID,
	ProjectID:     projectID,
	ProjectTaskID: taskID,
}).Get(ctx, &recovery); recoverErr != nil {
	return recoverErr
}
if recovery.Action == project.ProjectTaskRecoveryActionRetryScheduled && recovery.RetryNotBefore != nil {
	scheduleDispatchRetry(ctx, tenantID, projectID, taskID, *recovery.RetryNotBefore)
}
continue
```

Add helper:

```go
// scheduleDispatchRetry re-dispatches one task after its retry backoff.
// dispatchProjectTasks only runs from signal handlers, so without this timer a
// retry-scheduled task would wait for an unrelated signal. The loop is bounded:
// recovery stops returning retry_scheduled once dispatch failures reach
// max_attempts and moves the task to waiting_human instead.
func scheduleDispatchRetry(ctx workflow.Context, tenantID, projectID, taskID uuid.UUID, retryAt time.Time) {
	delay := retryAt.Sub(workflow.Now(ctx))
	if delay < 0 {
		delay = 0
	}
	workflow.Go(ctx, func(gctx workflow.Context) {
		if err := workflow.Sleep(gctx, delay); err != nil {
			return
		}
		if err := dispatchProjectTasks(gctx, tenantID, projectID, []uuid.UUID{taskID}, project.DispatchReasonRetry); err != nil {
			workflow.GetLogger(gctx).Error("retry dispatch failed", "task_id", taskID.String(), "error", err.Error())
		}
	})
}
```

Use `workflow.Now`/`workflow.Sleep` (never `time.Now`/`time.Sleep`) to keep the workflow deterministic. The re-dispatch goes through the normal `DispatchProjectTask` activity, so a repeated failure re-enters recovery and either schedules the next bounded retry or escalates to waiting-human.

- [ ] **Step 8: Run workflow tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectCoordinatorContinuesAfterRecordedDispatchFailure|TestProjectCoordinatorRedispatchesAfterRetryBackoff|TestActivitiesDispatchProjectTaskWrapsTerminalErrorAsNonRetryable|TestProjectStoreDispatchProjectTaskRunStartFailureKeepsTaskPlanned' -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/types.go apps/control-plane/internal/workflow/projectcoordination/activities.go apps/control-plane/internal/workflow/projectcoordination/project_store.go apps/control-plane/internal/workflow/projectcoordination/workflow.go apps/control-plane/internal/workflow/projectcoordination/workflow_test.go apps/control-plane/internal/workflow/projectcoordination/project_store_test.go
git commit -m "feat(control-plane): recover recorded project task dispatch failures"
```

## Task 5: Recover Stale Queued And Lost Running Attempts

**Files:**
- Modify: `apps/control-plane/internal/storage/queries/project.sql`
- Generated: `apps/control-plane/internal/storage/queries/project.sql.go`
- Generated: `apps/control-plane/internal/storage/queries/querier.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/service.go`
- Test: `apps/control-plane/internal/project/service_test.go`
- Test: `apps/control-plane/internal/project/pg_repository_test.go`

- [ ] **Step 1: Add failing tests for stale queued and lease lost**

Add service tests:

```go
func TestRecoverStaleQueuedProjectTaskAttemptSchedulesRetry(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusQueued, ProjectTaskAttemptStatusQueued)
	repo.tasks[0].MaxAttempts = serviceTestInt32Ptr(3)

	result, err := service.RecoverStaleQueuedProjectTaskAttempt(context.Background(), RecoverProjectTaskAttemptRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		AttemptID:     fixture.attemptID,
		FailureFamily: FailureFamilyRuntimeStartTimeout,
		Summary:       "Runtime did not acknowledge start before deadline",
		Now:           time.Now().UTC(),
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusQueued, result.Status)
	require.Equal(t, ProjectTaskAttemptStatusLost, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.projectTaskAttempts, 2)
	require.Equal(t, ProjectTaskAttemptStatusQueued, repo.projectTaskAttempts[1].Status)
}

func TestRecoverLostProjectTaskAttemptMovesToWaitingHumanWhenRetryExhausted(t *testing.T) {
	repo := newMemoryRepository()
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, inbox, nil)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	repo.tasks[0].AttemptCount = 1
	repo.tasks[0].MaxAttempts = serviceTestInt32Ptr(1)

	result, err := service.RecoverLostProjectTaskAttempt(context.Background(), RecoverProjectTaskAttemptRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		AttemptID:     fixture.attemptID,
		FailureFamily: FailureFamilyRuntimeLeaseLost,
		Summary:       "Runtime lease expired",
		Now:           time.Now().UTC(),
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusWaitingHuman, result.Status)
	require.Equal(t, ProjectTaskAttemptStatusLost, repo.projectTaskAttempts[0].Status)
	require.Len(t, inbox.upserts, 1)
}

func TestSweepStaleQueuedProjectTaskAttemptsRecoversCandidates(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	now := time.Now().UTC()
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusQueued, ProjectTaskAttemptStatusQueued)
	repo.tasks[0].MaxAttempts = serviceTestInt32Ptr(3)
	repo.projectTaskAttempts[0].CreatedAt = now.Add(-10 * time.Minute)

	result, err := service.SweepStaleQueuedProjectTaskAttempts(context.Background(), SweepProjectTaskAttemptRecoveryRequest{
		TenantID: fixture.tenantID,
		Now:      now,
		Limit:    10,
	})

	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{fixture.attemptID}, result.RecoveredAttemptIDs)
	require.Equal(t, []uuid.UUID{fixture.taskID}, result.RecoveredTaskIDs)
}

func TestSweepExpiredRunningProjectTaskAttemptsRecoversCandidates(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	now := time.Now().UTC()
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	repo.tasks[0].MaxAttempts = serviceTestInt32Ptr(3)
	expiresAt := now.Add(-time.Minute)
	repo.projectTaskAttempts[0].LeaseExpiresAt = &expiresAt

	result, err := service.SweepExpiredRunningProjectTaskAttempts(context.Background(), SweepProjectTaskAttemptRecoveryRequest{
		TenantID: fixture.tenantID,
		Now:      now,
		Limit:    10,
	})

	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{fixture.attemptID}, result.RecoveredAttemptIDs)
	require.Equal(t, []uuid.UUID{fixture.taskID}, result.RecoveredTaskIDs)
}
```

- [ ] **Step 2: Add failing Postgres test**

Add one Postgres integration test:

```go
func TestRecoverStaleQueuedAttemptReleasesActiveAttemptAndCreatesRetry(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	writebacks := repo.(ProjectTaskAttemptWritebackRepository)
	projectID := createProjectFixture(t, repo, tenantID)
	employeeID := uuid.New()
	maxAttempts := int32(3)
	task, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "stale queued",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
		MaxAttempts:               &maxAttempts,
	})
	require.NoError(t, err)
	nodeID := uuid.New()
	queued, err := repo.QueueProjectTaskWithAttempt(context.Background(), QueueProjectTaskRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		ProjectTaskID:     task.ID,
		DigitalEmployeeID: employeeID,
		RuntimeNodeID:     &nodeID,
		IdempotencyKey:    "project-task:" + task.ID.String() + ":attempt:1:queue",
		LeaseToken:        "lease-stale",
	})
	require.NoError(t, err)

	result, err := writebacks.RecoverProjectTaskAttemptFailureWriteback(context.Background(), RecoverProjectTaskAttemptFailureWritebackRequest{
		Task:                  queued.Task,
		Attempt:               queued.Attempt,
		Failure:               FailProjectTaskAttemptRequest{ProjectTaskAttemptRuntimeRequest: ProjectTaskAttemptRuntimeRequest{TenantID: tenantID, AttemptID: queued.Attempt.ID, ProjectTaskID: task.ID, RuntimeNodeID: nodeID, LeaseToken: queued.Attempt.LeaseToken, IdempotencyKey: "stale-" + queued.Attempt.ID.String()}, FailureSummary: "Runtime did not start", FailureFamily: FailureFamilyRuntimeStartTimeout},
		AttemptTerminalStatus: ProjectTaskAttemptStatusLost,
		TaskTargetStatus:      ProjectTaskStatusQueued,
		WaitingReason:         HumanWaitReasonRuntimeRecovery,
		RetryAttemptID:        uuid.New(),
		RetryLeaseToken:       "retry-lease-stale",
		RetryIdempotencyKey:   "project-task:" + task.ID.String() + ":attempt:2:retry:stale",
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusQueued, result.Task.Status)
	oldAttempt, err := repo.GetProjectTaskAttempt(context.Background(), tenantID, queued.Attempt.ID)
	require.NoError(t, err)
	require.Equal(t, ProjectTaskAttemptStatusLost, oldAttempt.Status)
	require.NotEqual(t, queued.Attempt.ID, *result.Task.CurrentAttemptID)
}
```

Use the existing `ProjectTaskAttemptWritebackRepository`; do not add a second recovery repository interface for attempt failures.

- [ ] **Step 3: Run failing tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestRecoverStaleQueuedProjectTaskAttempt|TestRecoverLostProjectTaskAttempt|TestSweepStaleQueuedProjectTaskAttempts|TestSweepExpiredRunningProjectTaskAttempts|TestRecoverStaleQueuedAttemptReleasesActiveAttempt' -count=1
```

Expected: FAIL with undefined service methods or repository methods.

- [ ] **Step 4: Add list queries for sweep candidates**

Append to `apps/control-plane/internal/storage/queries/project.sql`:

```sql
-- name: ListStaleQueuedProjectTaskAttempts :many
SELECT pta.*
FROM project_task_attempts pta
JOIN project_tasks pt ON pt.tenant_id = pta.tenant_id AND pt.id = pta.project_task_id
WHERE pta.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pta.status = 'queued'
  AND pt.status = 'queued'
  AND pta.started_at IS NULL
  AND pta.created_at < sqlc.arg('started_before')::timestamptz
ORDER BY pta.created_at ASC
LIMIT sqlc.arg('limit')::integer;

-- name: ListExpiredRunningProjectTaskAttempts :many
SELECT pta.*
FROM project_task_attempts pta
JOIN project_tasks pt ON pt.tenant_id = pta.tenant_id AND pt.id = pta.project_task_id
WHERE pta.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pta.status = 'running'
  AND pt.status = 'running'
  AND pta.lease_expires_at IS NOT NULL
  AND pta.lease_expires_at < sqlc.arg('now')::timestamptz
ORDER BY pta.lease_expires_at ASC
LIMIT sqlc.arg('limit')::integer;
```

Run:

```bash
make -C apps/control-plane generate-sqlc
```

- [ ] **Step 5: Add service methods by reusing existing writeback**

In `apps/control-plane/internal/project/service.go`, add:

```go
func (s *Service) RecoverStaleQueuedProjectTaskAttempt(ctx context.Context, req RecoverProjectTaskAttemptRequest) (*ProjectTask, error) {
	if req.FailureFamily == "" {
		req.FailureFamily = FailureFamilyRuntimeStartTimeout
	}
	return s.recoverProjectTaskAttempt(ctx, req, ProjectTaskAttemptStatusLost)
}

func (s *Service) RecoverLostProjectTaskAttempt(ctx context.Context, req RecoverProjectTaskAttemptRequest) (*ProjectTask, error) {
	if req.FailureFamily == "" {
		req.FailureFamily = FailureFamilyRuntimeLeaseLost
	}
	return s.recoverProjectTaskAttempt(ctx, req, ProjectTaskAttemptStatusLost)
}

func (s *Service) SweepStaleQueuedProjectTaskAttempts(ctx context.Context, req SweepProjectTaskAttemptRecoveryRequest) (SweepProjectTaskAttemptRecoveryResult, error) {
	if req.TenantID == uuid.Nil {
		return SweepProjectTaskAttemptRecoveryResult{}, ErrInvalidProject
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	repository, ok := s.repository.(ProjectTaskDispatchRecoveryRepository)
	if !ok {
		return SweepProjectTaskAttemptRecoveryResult{}, ErrInvalidProject
	}
	startedBefore := now.Add(-5 * time.Minute)
	attempts, err := repository.ListStaleQueuedProjectTaskAttempts(ctx, req.TenantID, startedBefore, limit)
	if err != nil {
		return SweepProjectTaskAttemptRecoveryResult{}, err
	}
	return s.recoverAttemptCandidates(ctx, attempts, FailureFamilyRuntimeStartTimeout, "Runtime did not acknowledge project task attempt start before deadline", now)
}

func (s *Service) SweepExpiredRunningProjectTaskAttempts(ctx context.Context, req SweepProjectTaskAttemptRecoveryRequest) (SweepProjectTaskAttemptRecoveryResult, error) {
	if req.TenantID == uuid.Nil {
		return SweepProjectTaskAttemptRecoveryResult{}, ErrInvalidProject
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	repository, ok := s.repository.(ProjectTaskDispatchRecoveryRepository)
	if !ok {
		return SweepProjectTaskAttemptRecoveryResult{}, ErrInvalidProject
	}
	attempts, err := repository.ListExpiredRunningProjectTaskAttempts(ctx, req.TenantID, now, limit)
	if err != nil {
		return SweepProjectTaskAttemptRecoveryResult{}, err
	}
	return s.recoverAttemptCandidates(ctx, attempts, FailureFamilyRuntimeLeaseLost, "Runtime lease expired before terminal writeback", now)
}

func (s *Service) recoverAttemptCandidates(ctx context.Context, attempts []ProjectTaskAttempt, failureFamily, summary string, now time.Time) (SweepProjectTaskAttemptRecoveryResult, error) {
	result := SweepProjectTaskAttemptRecoveryResult{
		RecoveredAttemptIDs: []uuid.UUID{},
		RecoveredTaskIDs:    []uuid.UUID{},
	}
	for _, attempt := range attempts {
		task, err := s.repository.GetProjectTask(ctx, attempt.TenantID, attempt.ProjectTaskID)
		if err != nil {
			return result, err
		}
		recovered, err := s.recoverProjectTaskAttempt(ctx, RecoverProjectTaskAttemptRequest{
			TenantID:      attempt.TenantID,
			ProjectID:     task.ProjectID,
			ProjectTaskID: attempt.ProjectTaskID,
			AttemptID:     attempt.ID,
			FailureFamily: failureFamily,
			Summary:       summary,
			Now:           now,
		}, ProjectTaskAttemptStatusLost)
		if err != nil {
			return result, err
		}
		result.RecoveredAttemptIDs = append(result.RecoveredAttemptIDs, attempt.ID)
		result.RecoveredTaskIDs = append(result.RecoveredTaskIDs, recovered.ID)
	}
	return result, nil
}

func (s *Service) recoverProjectTaskAttempt(ctx context.Context, req RecoverProjectTaskAttemptRequest, attemptTerminalStatus string) (*ProjectTask, error) {
	req.Summary = strings.TrimSpace(req.Summary)
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ProjectTaskID == uuid.Nil || req.AttemptID == uuid.Nil || req.Summary == "" {
		return nil, ErrInvalidProject
	}
	task, err := s.repository.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
	if err != nil {
		return nil, err
	}
	if task.ProjectID != req.ProjectID {
		return nil, ErrProjectNotFound
	}
	attempt, err := s.repository.GetProjectTaskAttempt(ctx, req.TenantID, req.AttemptID)
	if err != nil {
		return nil, err
	}
	writebackRepository, err := s.projectTaskAttemptWritebackRepository()
	if err != nil {
		return nil, err
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	retryAt := now.Add(defaultDispatchRecoveryBackoff)
	retryable := true
	action := projectTaskFailureAction(task, recoveryFailureFamilyForAction(req.FailureFamily), &retryable)
	result, err := writebackRepository.RecoverProjectTaskAttemptFailureWriteback(ctx, RecoverProjectTaskAttemptFailureWritebackRequest{
		Task:                  task,
		Attempt:               attempt,
		Failure:               FailProjectTaskAttemptRequest{ProjectTaskAttemptRuntimeRequest: ProjectTaskAttemptRuntimeRequest{TenantID: req.TenantID, AttemptID: req.AttemptID, ProjectTaskID: req.ProjectTaskID, RuntimeNodeID: uuidValue(attempt.RuntimeNodeID), LeaseToken: attempt.LeaseToken, IdempotencyKey: "recovery-" + req.AttemptID.String()}, FailureSummary: req.Summary, FailureFamily: recoveryFailureFamilyForAction(req.FailureFamily), Retryable: &retryable},
		AttemptTerminalStatus: attemptTerminalStatus,
		TaskTargetStatus:      action,
		WaitingReason:         HumanWaitReasonRuntimeRecovery,
		RetryAttemptID:        uuid.New(),
		RetryLeaseToken:       "retry-" + uuid.NewString(),
		RetryIdempotencyKey:   projectTaskRetryIdempotencyKey(task, "recovery-"+req.AttemptID.String()),
		RetryNotBefore:        &retryAt,
	})
	if err != nil {
		return nil, err
	}
	if result.Decision.ID != uuid.Nil && s.inbox != nil {
		if err := s.inbox.UpsertProjectDecisionRequest(ctx, result.Decision); err != nil {
			return nil, err
		}
	}
	return &result.Task, nil
}

func recoveryFailureFamilyForAction(failureFamily string) string {
	switch failureFamily {
	case FailureFamilyRuntimeStartTimeout, FailureFamilyRuntimeLeaseLost:
		return FailureFamilyTransientRuntime
	case FailureFamilyProviderStart:
		return FailureFamilyTransientProvider
	default:
		if strings.TrimSpace(failureFamily) == "" {
			return FailureFamilyTransientRuntime
		}
		return failureFamily
	}
}

func uuidValue(value *uuid.UUID) uuid.UUID {
	if value == nil {
		return uuid.Nil
	}
	return *value
}
```

For control-plane recovery, use the attempt's persisted `RuntimeNodeID` when present. If it is nil, pass `uuid.Nil` and keep the repository update keyed by tenant/task/attempt/lease; do not weaken Runtime writeback validation paths.

`RetryNotBefore` reuses `defaultDispatchRecoveryBackoff` so the replacement attempt carries an explicit backoff instead of immediately re-queueing against a Runtime that may still be down. Attempt-recovery retries stay bounded independently of this: each replacement attempt goes through `ScheduleProjectTaskRetry`, which increments `attempt_count`, so `projectTaskFailureAction` escalates to waiting-human once `max_attempts` is reached.

- [ ] **Step 6: Run tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestRecoverStaleQueuedProjectTaskAttempt|TestRecoverLostProjectTaskAttempt|TestSweepStaleQueuedProjectTaskAttempts|TestSweepExpiredRunningProjectTaskAttempts|TestRecoverStaleQueuedAttemptReleasesActiveAttempt' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/storage/queries/project.sql apps/control-plane/internal/storage/queries/project.sql.go apps/control-plane/internal/storage/queries/querier.go apps/control-plane/internal/project/repository.go apps/control-plane/internal/project/pg_repository.go apps/control-plane/internal/project/service.go apps/control-plane/internal/project/service_test.go apps/control-plane/internal/project/pg_repository_test.go
git commit -m "feat(control-plane): recover stale project task attempts"
```

## Task 6: Ready-Task Filtering And Retry Wakeup

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Test: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`
- Do not modify: `apps/control-plane/internal/project/pg_repository.go` for this task.

- [ ] **Step 1: Add failing dispatchability tests**

Add tests near `ListDispatchableTasks` tests in `project_store_test.go`.

```go
func TestProjectStoreListDispatchableTasksSkipsFutureRetry(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	jobID := uuid.New()
	taskID := uuid.New()
	future := time.Now().UTC().Add(time.Hour)
	repo := &projectStoreMemoryRepository{
		tasks: []project.ProjectTask{{
			ID:                taskID,
			TenantID:          tenantID,
			ProjectID:         projectID,
			CoordinationJobID: &jobID,
			Status:            project.ProjectTaskStatusPlanned,
			RetryNotBefore:    &future,
		}},
	}
	store := NewProjectStore(repo).WithClock(func() time.Time { return time.Now().UTC() })

	ready, err := store.ListDispatchableTasks(context.Background(), ListDispatchableTasksInput{TenantID: tenantID, ProjectID: projectID, CoordinationJobID: jobID})

	require.NoError(t, err)
	require.Empty(t, ready)
}

func TestProjectStoreListDispatchableTasksIncludesDueRetry(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	jobID := uuid.New()
	taskID := uuid.New()
	now := time.Now().UTC()
	past := now.Add(-time.Minute)
	repo := &projectStoreMemoryRepository{
		tasks: []project.ProjectTask{{
			ID:                taskID,
			TenantID:          tenantID,
			ProjectID:         projectID,
			CoordinationJobID: &jobID,
			Status:            project.ProjectTaskStatusPlanned,
			RetryNotBefore:    &past,
		}},
	}
	store := NewProjectStore(repo).WithClock(func() time.Time { return now })

	ready, err := store.ListDispatchableTasks(context.Background(), ListDispatchableTasksInput{TenantID: tenantID, ProjectID: projectID, CoordinationJobID: jobID})

	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{taskID}, ready)
}
```

- [ ] **Step 2: Run failing tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectStoreListDispatchableTasks.*Retry' -count=1
```

Expected: future retry test FAILS because `ListDispatchableTasks` currently ignores `retry_not_before`.

- [ ] **Step 3: Filter by retry_not_before**

In `ListDispatchableTasks` in `project_store.go`, update candidate filtering:

```go
now := s.now()
for _, task := range tasks {
	if !projectTaskDispatchAllowed(task.Status) {
		continue
	}
	if task.RetryNotBefore != nil && task.RetryNotBefore.After(now) {
		continue
	}
	candidates = append(candidates, task)
	candidateIDs = append(candidateIDs, task.ID)
}
```

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectStoreListDispatchableTasks.*Retry' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/project_store.go apps/control-plane/internal/workflow/projectcoordination/project_store_test.go
git commit -m "fix(control-plane): honor project task retry wakeup"
```

## Task 7: Verification, Contracts, And Real Smoke

**Files:**
- Modify: `CHANGELOG.md`
- No production code changes unless earlier tasks require small fixes.

- [ ] **Step 1: Run focused Go tests**

Run:

```bash
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/workflow/projectcoordination -run 'Recover|DispatchFailure|ListDispatchableTasks.*Retry|ProjectTaskAttempt' -count=1
```

Expected: PASS.

- [ ] **Step 2: Run broader affected package tests**

Run:

```bash
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/workflow/projectcoordination ./apps/control-plane/internal/storage -count=1
```

Expected: PASS.

- [ ] **Step 3: Run contract check**

Run:

```bash
corepack pnpm verify:contracts
```

Expected: PASS. If OpenAPI was not touched, this still guards generated drift.

- [ ] **Step 4: Run diff hygiene**

Run:

```bash
git diff --check
```

Expected: no output.

- [ ] **Step 5: Add changelog entry**

Run:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Add a `CHANGELOG.md` entry with that timestamp. Use this format:

```markdown
- YYYY-MM-DD HH:MM：ProjectTask dispatch/runtime recovery 首版：dispatch failure 已记录后会转成 retry scheduled 或 waiting-human recovery decision，dispatch 重试按 `dispatch_failed` 事件计数对 `max_attempts` 封顶，coordinator 在 retry 到期后用 workflow timer 自动重派（不再依赖后续 signal）；恢复对 Temporal activity 重试幂等（pending decision 不重复、未到期 retry 不重排）；queued 未 started 和 running lease lost 可通过 Control Plane recovery 终态化旧 attempt 并按 retry policy 推进（带退避）；`ListDispatchableTasks` 尊重 `retry_not_before`，避免未到期重试被立即重新分派。验证：`go test ./apps/control-plane/internal/project ./apps/control-plane/internal/workflow/projectcoordination ./apps/control-plane/internal/storage -count=1`、`corepack pnpm verify:contracts`、`git diff --check` 通过。
```

- [ ] **Step 6: Real-chain smoke against local services**

Check service state:

```bash
scripts/dev-services.sh status
```

If Control Plane is not running current code, restart it:

```bash
scripts/dev-services.sh restart control-plane
```

Run a real API or workflow smoke that triggers dispatch failure while Runtime is unavailable or selected Runtime is not dispatchable. The smoke must prove:

- a `project_task.dispatch_failed` event is written;
- recovery writes either `project_task.retry_scheduled` with `retry_not_before` or `project_task.recovery_requested`;
- task no longer relies only on a raw dispatch_failed event for next action.

If a safe authenticated project-demand flow is already available locally, use it. If auth/session or seed data is unavailable, report the exact blocker and do not claim real-chain completion.

- [ ] **Step 7: Commit final verification docs**

```bash
git add CHANGELOG.md
git commit -m "chore: document project task recovery verification"
```

## Self-Review Checklist

- Spec coverage:
  - Dispatch failure without attempt: Tasks 1-4.
  - Queued not started: Task 5.
  - Running lease lost: Task 5.
  - Provider/session startup failure: Task 5 reuses failure-family recovery; explicit Provider classification is in Task 1.
  - Retry wakeup: Task 4 (workflow timer after retry_scheduled) + Task 6 (dispatchable filtering of not-yet-due retries).
  - Verification and changelog: Task 7.
- Bounded retries: dispatch retries are capped by counting `dispatch_failed` events against `max_attempts` (Tasks 1-3) — `attempt_count` stays 0 for pure dispatch failures and cannot be the bound; attempt recovery consumes `attempt_count` via `ScheduleProjectTaskRetry` and carries an explicit backoff (Task 5).
- Idempotency: recovery noops when a waiting-human decision is pending or a retry is not yet due, so Temporal activity retries cannot duplicate decisions or events (Task 3).
- No event-ID transport through activity errors: concrete error types do not survive the Temporal boundary (`dispatchFailureRecorded` matches `appErr.Type()`); the service resolves the latest `dispatch_failed` event itself (Task 4).
- State consistency: `ScheduleProjectTaskDispatchRetry` resets the task to `planned` so no task is left `waiting_human` with cleared waiting fields (Task 2); the service guard prevents clobbering an unresolved decision.
- No placeholders: every task has file paths, test names, commands, and expected outcomes.
- Type consistency: recovery action names use `ProjectTaskRecoveryAction*`; waiting-human uses existing `HumanWaitReasonRuntimeRecovery`; decisions use `project_task_recovery`.
