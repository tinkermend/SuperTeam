# ProjectTask Recovery And Human Wait Implementation Plan

> 复核状态：06-22 ProjectTask recovery支持retry scheduling

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add durable retry, lease-lost/timeout classification, typed `waiting_human`, and acceptance gates on top of attempt-aware ProjectTask execution.

**Architecture:** This is phase 3 of `docs/superpowers/specs/2026-06-20-project-task-durable-closure-design.md`. It depends on the Control Plane attempt model and Runtime attempt endpoints from phases 1 and 2. Control Plane owns recovery decisions; Runtime reports structured failure or wait-human facts.

**Tech Stack:** Go project service, pgx/sqlc, Temporal project coordinator signals, Runtime Agent Rust writeback, OpenAPI contracts, project events and decision/transfer request models.

---

## Source Spec

Implement this plan against:

- `docs/superpowers/specs/2026-06-20-project-task-durable-closure-design.md`
- `docs/superpowers/plans/2026-06-20-project-task-durable-closure-control-plane.md`
- `docs/superpowers/plans/2026-06-20-project-task-runtime-attempt-contract.md`

## File Structure

Modify:

- `apps/control-plane/internal/project/types.go`
  - Adds failure classification, wait-human request types, acceptance request types, and recovery result enums.
- `apps/control-plane/internal/project/service.go`
  - Adds retry classification, typed wait-human, acceptance gate, and resolve-human-wait logic.
- `apps/control-plane/internal/project/repository.go`
  - Adds wait-human, retry scheduling, acceptance, and context-specific terminal writes.
- `apps/control-plane/internal/project/pg_repository.go`
  - Implements transactional retry, waiting-human, acceptance, cancel/replan hooks.
- `apps/control-plane/internal/storage/queries/project.sql`
  - Adds update queries for retry scheduling, wait-human, acceptance, and terminal decisions.
- `apps/control-plane/internal/project/service_test.go`
  - Tests failure classification, retry scheduling, waiting-human, acceptance, rejection, and invalid transitions.
- `apps/control-plane/internal/project/pg_repository_test.go`
  - Tests atomic repository behavior.
- `apps/control-plane/internal/workflow/projectcoordination/types.go`
  - Adds signals/results for acceptance and recovery decisions.
- `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
  - Adds append-only recovery task creation and downstream hold/unhold behavior.
- `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`
  - Tests retry/reassign/rework/cancel recovery graph behavior.
- `contracts/control-plane/openapi.yaml`
  - Adds wait-human attempt endpoint and acceptance/resolve request schemas.
- `apps/runtime-agent/src/commands/executor.rs`
  - Adds wait-human writeback payload builder.
- `apps/runtime-agent/src/controlplane/client.rs`
  - Adds `wait_project_task_attempt_human`.
- `apps/runtime-agent/tests/runtime_command_executor_test.rs`
  - Tests Runtime Agent wait-human writeback.
- `CHANGELOG.md`

## Task 1: Add Recovery And Wait-Human Domain Types

**Files:**

- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/service_test.go`

- [ ] **Step 1: Add failing tests for transient retry**

In `apps/control-plane/internal/project/service_test.go`, add:

```go
func TestFailProjectTaskAttemptTransientRuntimeSchedulesRetry(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	nodeID := uuid.New()
	employeeID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Status:                    ProjectTaskStatusRunning,
		CurrentAttemptID:          &attemptID,
		AttemptCount:              1,
		MaxAttempts:               int32Ptr(3),
		AssignedDigitalEmployeeID: &employeeID,
	})
	repo.projectTaskAttempts = append(repo.projectTaskAttempts, ProjectTaskAttempt{
		ID:            attemptID,
		TenantID:      tenantID,
		ProjectTaskID: taskID,
		Status:        ProjectTaskAttemptStatusRunning,
		LeaseToken:    "lease-token-1",
		RuntimeNodeID: &nodeID,
	})

	task, err := service.FailProjectTaskAttempt(context.Background(), FailProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: ProjectTaskAttemptRuntimeRequest{
			TenantID:       tenantID,
			AttemptID:      attemptID,
			ProjectTaskID:  taskID,
			RuntimeNodeID:  nodeID,
			LeaseToken:     "lease-token-1",
			IdempotencyKey: "fail-transient-1",
		},
		DigitalEmployeeID: employeeID,
		FailureSummary:    "runtime node restarted",
		FailureFamily:     FailureFamilyTransientRuntime,
		Retryable:         boolPtr(true),
	})
	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusQueued, task.Status)
	require.NotEqual(t, attemptID, *task.CurrentAttemptID)
	require.Equal(t, int32(2), task.AttemptCount)
}
```

- [ ] **Step 2: Add domain constants**

In `apps/control-plane/internal/project/types.go`, add:

```go
const (
	FailureFamilyTransientRuntime      = "transient_runtime"
	FailureFamilyTransientProvider     = "transient_provider"
	FailureFamilyTimeout               = "timeout"
	FailureFamilyInvalidContract       = "invalid_contract"
	FailureFamilyApprovalRequired      = "approval_required"
	FailureFamilyPermissionRequired    = "permission_required"
	FailureFamilyNonRetryableExecution = "non_retryable_execution"
	FailureFamilyBusinessCancelled     = "business_cancelled"
	FailureFamilyPlanInvalid           = "plan_invalid"
	FailureFamilyRequirementChanged    = "requirement_changed"
	FailureFamilyAcceptanceRequired    = "acceptance_required"
)

const (
	HumanWaitReasonMissingContext     = "missing_context"
	HumanWaitReasonClarification      = "clarification"
	HumanWaitReasonApprovalRequired   = "approval_required"
	HumanWaitReasonPermissionRequired = "permission_required"
	HumanWaitReasonPlanInvalid        = "plan_invalid"
	HumanWaitReasonAcceptanceRequired = "acceptance_required"
)

const (
	HumanWaitResolutionResumeSameTask    = "resume_same_task"
	HumanWaitResolutionCancelAndReplan   = "cancel_and_replan"
	HumanWaitResolutionCancelWithoutPlan = "cancel_without_replan"
	HumanWaitResolutionMarkFailed        = "mark_failed"
)
```

Add request types:

```go
type WaitHumanProjectTaskAttemptRequest struct {
	ProjectTaskAttemptRuntimeRequest
	DigitalEmployeeID           uuid.UUID
	Reason                      string
	Summary                     string
	MissingContextRefs          []any
	SuggestedResolutionOptions  []string
}

type ResolveProjectTaskHumanWaitRequest struct {
	TenantID       uuid.UUID
	ProjectID      uuid.UUID
	ProjectTaskID  uuid.UUID
	ActorUserID    uuid.UUID
	Resolution     string
	ResponseSummary string
	ContextRefs     []any
}
```

- [ ] **Step 3: Run tests and verify missing methods**

Run:

```bash
cd apps/control-plane && go test ./internal/project -run 'TransientRuntime|HumanWait|Acceptance' -count=1
```

Expected: FAIL because recovery methods are not implemented.

- [ ] **Step 4: Commit domain constants**

Run:

```bash
git add apps/control-plane/internal/project/types.go apps/control-plane/internal/project/service_test.go
git commit -m "feat(control-plane): define project task recovery types"
```

## Task 2: Implement Retry Scheduling And Exhaustion

**Files:**

- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/storage/queries/project.sql`
- Modify: generated files under `apps/control-plane/internal/storage/queries/`
- Modify: `apps/control-plane/internal/project/service_test.go`

- [ ] **Step 1: Add repository queries**

Append:

```sql
-- name: ScheduleProjectTaskRetry :one
UPDATE project_tasks
SET status = 'queued',
    current_attempt_id = sqlc.arg('current_attempt_id')::uuid,
    attempt_count = attempt_count + 1,
    retry_not_before = sqlc.narg('retry_not_before')::timestamptz,
    waiting_reason = NULL,
    waiting_request_id = NULL,
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    status_changed_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status IN ('running', 'waiting_human')
RETURNING *;

-- name: MoveProjectTaskToWaitingHuman :one
UPDATE project_tasks
SET status = 'waiting_human',
    waiting_reason = sqlc.arg('waiting_reason')::varchar,
    waiting_request_id = sqlc.narg('waiting_request_id')::uuid,
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    status_changed_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status IN ('queued', 'running')
RETURNING *;
```

- [ ] **Step 2: Regenerate sqlc**

Run:

```bash
make -C apps/control-plane generate-sqlc
```

Expected: exits 0.

- [ ] **Step 3: Implement retry policy helper**

In `apps/control-plane/internal/project/service.go`, add:

```go
func projectTaskFailureAction(task ProjectTask, failureFamily string, retryable *bool) string {
	if retryable != nil && !*retryable {
		if failureFamily == FailureFamilyBusinessCancelled || failureFamily == FailureFamilyPlanInvalid || failureFamily == FailureFamilyRequirementChanged {
			return ProjectTaskStatusCancelled
		}
		return ProjectTaskStatusFailed
	}
	switch failureFamily {
	case FailureFamilyTransientRuntime, FailureFamilyTransientProvider, FailureFamilyTimeout:
		maxAttempts := int32(1)
		if task.MaxAttempts != nil {
			maxAttempts = *task.MaxAttempts
		}
		if task.AttemptCount < maxAttempts {
			return ProjectTaskStatusQueued
		}
		return ProjectTaskStatusWaitingHuman
	case FailureFamilyInvalidContract, FailureFamilyApprovalRequired, FailureFamilyPermissionRequired:
		return ProjectTaskStatusWaitingHuman
	case FailureFamilyBusinessCancelled, FailureFamilyPlanInvalid, FailureFamilyRequirementChanged:
		return ProjectTaskStatusCancelled
	default:
		return ProjectTaskStatusFailed
	}
}
```

- [ ] **Step 4: Implement fail recovery path**

Update `FailProjectTaskAttempt` to:

1. Validate current attempt.
2. Mark current attempt `failed`, `lost`, or `timed_out` based on `FailureFamily`.
3. Evaluate `projectTaskFailureAction`.
4. If action is `queued`, create a new attempt in the same transaction and set task to `queued`.
5. If action is `waiting_human`, set task to `waiting_human` with reason derived from failure family.
6. If action is terminal, set `failed` or `cancelled`.

- [ ] **Step 5: Run retry tests**

Run:

```bash
cd apps/control-plane && go test ./internal/project -run 'TransientRuntimeSchedulesRetry|RetryExhaustionMovesToWaitingHuman|NonRetryableExecutionFailsTask' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit retry behavior**

Run:

```bash
git add apps/control-plane/internal/storage/queries apps/control-plane/internal/project
git commit -m "feat(control-plane): schedule project task retries"
```

## Task 3: Add Wait-Human Attempt Endpoint

**Files:**

- Modify: `contracts/control-plane/openapi.yaml`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/project/handler.go`
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/service_test.go`

- [ ] **Step 1: Add OpenAPI path**

Add:

```yaml
  /api/v1/runtime/project-task-attempts/{attemptId}/wait-human:
    post:
      operationId: waitHumanProjectTaskAttempt
      summary: Pause a ProjectTask attempt and request human input
      parameters:
        - name: attemptId
          in: path
          required: true
          schema:
            type: string
            format: uuid
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/WaitHumanProjectTaskAttemptRequest"
      responses:
        "202":
          description: Human wait accepted
```

Add schema:

```yaml
    WaitHumanProjectTaskAttemptRequest:
      allOf:
        - $ref: "#/components/schemas/ProjectTaskAttemptRuntimeFields"
        - type: object
          required:
            - digital_employee_id
            - reason
            - summary
          properties:
            digital_employee_id:
              type: string
              format: uuid
            reason:
              type: string
              enum:
                - missing_context
                - clarification
                - approval_required
                - permission_required
                - plan_invalid
                - acceptance_required
            summary:
              type: string
            missing_context_refs:
              type: array
              items: {}
            suggested_resolution_options:
              type: array
              items:
                type: string
```

- [ ] **Step 2: Register route**

In `apps/control-plane/internal/api/server.go`, add:

```go
r.Post("/wait-human", s.projectHandler.WaitHumanProjectTaskAttempt)
```

under `project-task-attempts/{attemptId}`.

- [ ] **Step 3: Implement handler and service**

Add handler method `WaitHumanProjectTaskAttempt`, decode body, call `service.WaitHumanProjectTaskAttempt`.

Service behavior:

- Validate attempt request.
- Mark attempt `waiting_human`.
- Mark task `waiting_human`.
- Append `project_task.waiting_human` event.
- Create a decision/request object when reason is approval, permission, acceptance, missing context, or clarification.

- [ ] **Step 4: Run tests**

Run:

```bash
cd apps/control-plane && go test ./internal/api ./internal/project -run 'WaitHumanProjectTaskAttempt|MoveProjectTaskToWaitingHuman' -count=1
corepack pnpm verify:contracts
```

Expected: PASS.

- [ ] **Step 5: Commit wait-human endpoint**

Run:

```bash
git add contracts/control-plane/openapi.yaml apps/control-plane/internal/api apps/control-plane/internal/project
git commit -m "feat(runtime): add project task wait-human writeback"
```

## Task 4: Add Acceptance Gate

**Files:**

- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/service_test.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`

- [ ] **Step 1: Add acceptance gate test**

In `apps/control-plane/internal/project/service_test.go`, add:

```go
func TestCompleteHighRiskAttemptRequiresAcceptanceBeforeCompleted(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	nodeID := uuid.New()
	employeeID := uuid.New()
	high := "high"
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Status:                    ProjectTaskStatusRunning,
		CurrentAttemptID:          &attemptID,
		RiskLevel:                 &high,
		AssignedDigitalEmployeeID: &employeeID,
	})
	repo.projectTaskAttempts = append(repo.projectTaskAttempts, ProjectTaskAttempt{
		ID:            attemptID,
		TenantID:      tenantID,
		ProjectTaskID: taskID,
		Status:        ProjectTaskAttemptStatusRunning,
		LeaseToken:    "lease-token-1",
		RuntimeNodeID: &nodeID,
	})

	_, err = service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: ProjectTaskAttemptRuntimeRequest{
			TenantID:       tenantID,
			AttemptID:      attemptID,
			ProjectTaskID:  taskID,
			RuntimeNodeID:  nodeID,
			LeaseToken:     "lease-token-1",
			IdempotencyKey: "complete-high-risk-1",
		},
		DigitalEmployeeID: employeeID,
		Conclusion:        "候选结果已完成",
	})
	require.NoError(t, err)
	task := repo.mustProjectTask(taskID)
	require.Equal(t, ProjectTaskStatusWaitingHuman, task.Status)
	require.NotNil(t, task.WaitingReason)
	require.Equal(t, HumanWaitReasonAcceptanceRequired, *task.WaitingReason)
}
```

- [ ] **Step 2: Implement acceptance gate**

In `apps/control-plane/internal/project/service.go`, add:

```go
func projectTaskRequiresAcceptance(task ProjectTask, req CompleteProjectTaskAttemptRequest) bool {
	if task.RequiresHumanApproval {
		return true
	}
	if task.RiskLevel != nil {
		switch strings.ToLower(strings.TrimSpace(*task.RiskLevel)) {
		case "high", "critical":
			return true
		}
	}
	return req.RequiresHumanReview
}
```

Update `CompleteProjectTaskAttempt`:

- If `projectTaskRequiresAcceptance` is false, complete task.
- If true, mark attempt `succeeded`, task `waiting_human`, waiting reason `acceptance_required`, create acceptance request/event, and do not unlock downstream dependencies.

- [ ] **Step 3: Add resolution tests**

Add tests:

- acceptance approved moves task `completed`;
- acceptance rejected with `resume_same_task` creates new queued attempt;
- acceptance rejected with `cancel_and_replan` cancels task and creates recovery graph;
- acceptance rejected with `mark_failed` moves task `failed`.

Use the `ResolveProjectTaskHumanWaitRequest` type from Task 1.

- [ ] **Step 4: Run acceptance tests**

Run:

```bash
cd apps/control-plane && go test ./internal/project ./internal/workflow/projectcoordination -run 'Acceptance|ResolveProjectTaskHumanWait' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit acceptance gate**

Run:

```bash
git add apps/control-plane/internal/project apps/control-plane/internal/workflow/projectcoordination
git commit -m "feat(control-plane): gate project task completion on acceptance"
```

## Task 5: Runtime Agent Wait-Human Writeback

**Files:**

- Modify: `apps/runtime-agent/src/controlplane/client.rs`
- Modify: `apps/runtime-agent/src/commands/executor.rs`
- Modify: `apps/runtime-agent/tests/runtime_command_executor_test.rs`

- [ ] **Step 1: Add failing Runtime test**

In `apps/runtime-agent/tests/runtime_command_executor_test.rs`, add a fake provider output that requests human input:

```rust
#[tokio::test]
async fn start_session_waits_human_when_provider_reports_missing_context() {
    let capture = CapturedRuntimeCommandWritebacks::default();
    let http_server = serve_runtime_command_capture(capture.clone()).await;
    let payload = project_task_session_payload("employee-1")
        .with_summary_json(serde_json::json!({
            "requires_human_review": true,
            "wait_human_reason": "missing_context",
            "missing_context_refs": ["customer_scope"],
            "recommended_next_action": "Ask the human owner for the missing customer scope."
        }));

    run_runtime_command_with_payload(&http_server, payload).await;

    let wait = wait_for_project_task_writeback(capture.project_task_wait_human.clone()).await;
    assert_eq!(wait.attempt_id, PROJECT_TASK_ATTEMPT_ID);
    assert_eq!(wait.body["reason"], "missing_context");
}
```

Extend the current test helpers in `apps/runtime-agent/tests/runtime_command_executor_test.rs` instead of introducing a second harness:

```rust
#[derive(Clone, Default)]
struct CommandCompletionCapture {
    complete: Arc<Mutex<Option<CapturedCommandWriteback>>>,
    fail: Arc<Mutex<Option<CapturedCommandWriteback>>>,
    event: Arc<Mutex<Vec<CapturedCommandWriteback>>>,
    project_task_complete: Arc<Mutex<Option<CapturedProjectTaskWriteback>>>,
    project_task_fail: Arc<Mutex<Option<CapturedProjectTaskWriteback>>>,
    project_task_wait_human: Arc<Mutex<Option<CapturedProjectTaskAttemptWriteback>>>,
}

#[derive(Clone, Debug)]
struct CapturedProjectTaskAttemptWriteback {
    attempt_id: String,
    authorization: Option<String>,
    node_id: Option<String>,
    payload: Value,
}

async fn capture_project_task_wait_human_writeback(
    AxumPath(attempt_id): AxumPath<String>,
    State(capture): State<CommandCompletionCapture>,
    headers: HeaderMap,
    Json(payload): Json<Value>,
) -> StatusCode {
    *capture
        .project_task_wait_human
        .lock()
        .expect("project task wait human lock") = Some(CapturedProjectTaskAttemptWriteback {
        attempt_id,
        authorization: header_value(&headers, "authorization"),
        node_id: header_value(&headers, "x-node-id"),
        payload,
    });
    StatusCode::NO_CONTENT
}
```

Add this route to `serve_command_completion_writebacks`:

```rust
.route(
    "/api/v1/runtime/project-task-attempts/{attempt_id}/wait-human",
    post(capture_project_task_wait_human_writeback),
)
```

- [ ] **Step 2: Add client method**

In `apps/runtime-agent/src/controlplane/client.rs`, add:

```rust
pub async fn wait_human_project_task_attempt(
    &self,
    attempt_id: &str,
    body: &ProjectTaskWaitHumanWriteback,
) -> Result<()> {
    let url = format!("{}/api/v1/runtime/project-task-attempts/{}/wait-human", self.base_url, attempt_id);
    self.post_json(&url, body).await
}
```

- [ ] **Step 3: Add wait-human payload builder**

In `apps/runtime-agent/src/commands/executor.rs`, add `ProjectTaskWaitHumanWriteback` builder that maps provider JSON fields:

- `wait_human_reason`
- `missing_context_refs`
- `recommended_next_action`
- `requires_human_review`

to the Control Plane request.

- [ ] **Step 4: Run Runtime wait-human test**

Run:

```bash
cd apps/runtime-agent && cargo test --test runtime_command_executor_test wait_human -- --nocapture
```

Expected: PASS.

- [ ] **Step 5: Commit Runtime wait-human**

Run:

```bash
git add apps/runtime-agent/src/controlplane/client.rs apps/runtime-agent/src/commands/executor.rs apps/runtime-agent/tests/runtime_command_executor_test.rs
git commit -m "feat(runtime-agent): write back project task human waits"
```

## Task 6: Phase Verification

**Files:**

- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add changelog entry**

Run:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Add:

```markdown
- [YYYY-MM-DD HH:MM] ProjectTask recovery now supports retry scheduling, typed waiting-human pauses, and acceptance-gated completion.
```

- [ ] **Step 2: Run phase gates**

Run:

```bash
make -C apps/control-plane generate-sqlc
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
cd apps/control-plane && go test ./internal/project ./internal/workflow/projectcoordination ./internal/api -count=1
cd apps/runtime-agent && cargo test --test runtime_command_executor_test project_task -- --nocapture
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 3: Commit verification**

Run:

```bash
git add CHANGELOG.md apps/control-plane apps/runtime-agent contracts/control-plane/openapi.yaml apps/web/src/lib/api/generated
git commit -m "chore: verify project task recovery phase"
```

Include only files with actual diffs.
