# ProjectTask Liveness Context And Smoke Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add execution context packets, deferred context updates, ProjectTask liveness projection, read-model/operational-status convergence, and a real closure smoke for the new durable attempt chain.

**Architecture:** This is phase 4 of `docs/superpowers/specs/2026-06-20-project-task-durable-closure-design.md`. It depends on phases 1-3. It does not change the core status machine; it makes the execution inputs inspectable, makes task liveness diagnosable, and proves the real path through Control Plane, DB, Runtime Agent, and Provider.

**Tech Stack:** Go read models, sqlc, OpenAPI, Runtime Agent payloads, existing project task graph API, operational status SQL, curl/browser smoke through `scripts/dev-services.sh`, real Runtime/Provider smoke.

---

## Source Spec

Implement this plan against:

- `docs/superpowers/specs/2026-06-20-project-task-durable-closure-design.md`
- `docs/superpowers/plans/2026-06-20-project-task-durable-closure-control-plane.md`
- `docs/superpowers/plans/2026-06-20-project-task-runtime-attempt-contract.md`
- `docs/superpowers/plans/2026-06-20-project-task-recovery-human-wait.md`

## File Structure

Modify:

- `apps/control-plane/internal/project/types.go`
  - Adds execution packet, context update, and liveness projection types.
- `apps/control-plane/internal/project/service.go`
  - Builds execution packets and records context updates.
- `apps/control-plane/internal/project/pg_repository.go`
  - Persists context updates and liveness reads.
- `apps/control-plane/internal/storage/queries/project.sql`
  - Adds context update table/queries and liveness projection query.
- `apps/control-plane/internal/storage/migrations/025_project_task_liveness_context.sql`
  - Adds `project_task_attempt_context_updates`.
- `apps/control-plane/internal/project/handler.go`
  - Exposes liveness fields in task graph or a dedicated liveness endpoint.
- `apps/control-plane/internal/api/server.go`
  - Registers endpoint if using a dedicated route.
- `contracts/control-plane/openapi.yaml`
  - Adds liveness fields and context update schemas.
- `apps/control-plane/internal/storage/queries/employee_execution.sql`
  - Updates digital employee operational status facts to use `queued/running/waiting_human` and attempts.
- `apps/control-plane/internal/project/service_test.go`
- `apps/control-plane/internal/project/pg_repository_test.go`
- `apps/control-plane/internal/project/handler_test.go`
- `apps/control-plane/internal/storage/migrations_test.go`
- `apps/runtime-agent/src/commands/executor.rs`
  - Logs and carries execution context packet version.
- `apps/runtime-agent/tests/runtime_command_executor_test.rs`
- `CHANGELOG.md`

## Task 1: Add Execution Context Packet Builder

**Files:**

- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/service_test.go`

- [ ] **Step 1: Add execution packet test**

In `apps/control-plane/internal/project/service_test.go`, add:

```go
func TestBuildProjectTaskExecutionPacketIncludesDependenciesAndHumanDecisions(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	blockerID := uuid.New()
	decisionID := uuid.New()
	task := ProjectTask{
		ID:              taskID,
		TenantID:        tenantID,
		ProjectID:       projectID,
		Title:           "执行上线检查",
		Status:          ProjectTaskStatusPlanned,
		ExpectedOutputs: []any{"deployment_report"},
		InputRequirements: map[string]any{
			"environment": "staging",
		},
		HandoffContract: map[string]any{
			"completion_path": "project_task_attempt_writeback",
		},
		BlockedByTaskIDs: []uuid.UUID{blockerID},
	}
	repo.executionSummaries = append(repo.executionSummaries, ExecutionSummary{
		ID:            uuid.New(),
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: &blockerID,
		Conclusion:    "依赖任务已完成，产出 staging 检查清单。",
		EvidenceRefs:  []any{"evidence://staging-checklist"},
	})
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:            decisionID,
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: &taskID,
		DecisionType:  "approval_required",
		StatusSnapshot: ProjectTaskStatusWaitingHuman,
	})

	packet, err := service.BuildProjectTaskExecutionPacket(context.Background(), task)
	require.NoError(t, err)
	require.Equal(t, "v1", packet.Version)
	require.Equal(t, taskID.String(), packet.ProjectTaskID)
	require.Contains(t, packet.ExpectedOutputs, "deployment_report")
	require.Len(t, packet.DependencyOutputs, 1)
	require.Equal(t, "evidence://staging-checklist", packet.DependencyOutputs[0].EvidenceRefs[0])
	require.Len(t, packet.HumanDecisionRefs, 1)
}
```

- [ ] **Step 2: Add packet types**

In `apps/control-plane/internal/project/types.go`, add:

```go
type ProjectTaskExecutionPacket struct {
	Version              string
	ProjectID            string
	ProjectTaskID        string
	Title                string
	Summary              string
	ExpectedOutputs      []any
	InputRequirements    map[string]any
	HandoffContract      map[string]any
	DependencyOutputs    []ProjectTaskDependencyOutput
	HumanDecisionRefs    []ProjectTaskHumanDecisionRef
	ForbiddenScopes      []string
	RiskLevel            string
	StopForHumanCriteria []string
}

type ProjectTaskDependencyOutput struct {
	ProjectTaskID string
	Conclusion    string
	EvidenceRefs  []any
	ArtifactRefs  []any
}

type ProjectTaskHumanDecisionRef struct {
	DecisionRequestID string
	DecisionType      string
	StatusSnapshot    string
}
```

- [ ] **Step 3: Implement packet builder**

In `apps/control-plane/internal/project/service.go`, add `BuildProjectTaskExecutionPacket`. It must:

- copy task title, summary, expected outputs, input requirements, and handoff contract;
- fetch execution summaries for `BlockedByTaskIDs`;
- include decision requests for the task;
- set `StopForHumanCriteria` to `missing_context`, `approval_required`, `permission_required`, `plan_invalid`;
- set `Version` to `v1`.

- [ ] **Step 4: Run packet tests**

Run:

```bash
cd apps/control-plane && go test ./internal/project -run TestBuildProjectTaskExecutionPacketIncludesDependenciesAndHumanDecisions -count=1
```

Expected: PASS.

- [ ] **Step 5: Wire QueueProjectTask to use packet builder**

In `QueueProjectTask`, if `ExecutionContextPacket` is empty, build it from the task before creating the attempt. Store the serialized packet in `project_task_attempts.execution_context_packet`.

- [ ] **Step 6: Commit execution packet**

Run:

```bash
git add apps/control-plane/internal/project
git commit -m "feat(control-plane): build project task execution packets"
```

## Task 2: Add Deferred Context Updates

**Files:**

- Create: `apps/control-plane/internal/storage/migrations/025_project_task_liveness_context.sql`
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
- Modify: `apps/control-plane/internal/storage/queries/project.sql`
- Modify: generated files under `apps/control-plane/internal/storage/queries/`
- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/service_test.go`

- [ ] **Step 1: Add migration test**

In `apps/control-plane/internal/storage/migrations_test.go`, add:

```go
func TestProjectTaskAttemptContextUpdatesMigration(t *testing.T) {
	sql := readMigrations(t)
	block := createTableBlock(t, sql, "project_task_attempt_context_updates")
	for _, fragment := range []string{
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"tenant_id UUID NOT NULL",
		"project_task_id UUID NOT NULL",
		"attempt_id UUID",
		"update_kind VARCHAR(100) NOT NULL",
		"payload JSONB NOT NULL",
		"delivery_mode VARCHAR(50) NOT NULL",
		"created_event_id UUID",
	} {
		require.Contains(t, block, fragment)
	}
}
```

- [ ] **Step 2: Add migration**

Create `apps/control-plane/internal/storage/migrations/025_project_task_liveness_context.sql`:

```sql
CREATE TABLE project_task_attempt_context_updates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_task_id UUID NOT NULL REFERENCES project_tasks(id) ON DELETE CASCADE,
    attempt_id UUID REFERENCES project_task_attempts(id) ON DELETE SET NULL,
    update_kind VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    delivery_mode VARCHAR(50) NOT NULL,
    created_event_id UUID REFERENCES project_events(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_project_task_context_update_delivery CHECK (
        delivery_mode IN ('hot_inject', 'next_attempt', 'waiting_human', 'cancel_and_replan')
    )
);

CREATE INDEX idx_project_task_context_updates_task
    ON project_task_attempt_context_updates(tenant_id, project_task_id, created_at DESC);

CREATE TRIGGER update_project_task_attempt_context_updates_updated_at
    BEFORE UPDATE ON project_task_attempt_context_updates
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
```

- [ ] **Step 3: Add service test**

In `apps/control-plane/internal/project/service_test.go`, add:

```go
func TestRecordAttemptContextUpdateRoutesContractChangeToReplan(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:               taskID,
		TenantID:         tenantID,
		ProjectID:        projectID,
		Status:           ProjectTaskStatusRunning,
		CurrentAttemptID: &attemptID,
	})

	update, err := service.RecordAttemptContextUpdate(context.Background(), RecordAttemptContextUpdateRequest{
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: taskID,
		AttemptID:     &attemptID,
		UpdateKind:    "requirement_changed",
		Payload:       map[string]any{"new_scope": "include production"},
	})
	require.NoError(t, err)
	require.Equal(t, ContextUpdateDeliveryCancelAndReplan, update.DeliveryMode)
}
```

- [ ] **Step 4: Implement service and repository**

Add types:

```go
const (
	ContextUpdateDeliveryHotInject       = "hot_inject"
	ContextUpdateDeliveryNextAttempt     = "next_attempt"
	ContextUpdateDeliveryWaitingHuman    = "waiting_human"
	ContextUpdateDeliveryCancelAndReplan = "cancel_and_replan"
)

type RecordAttemptContextUpdateRequest struct {
	TenantID      uuid.UUID
	ProjectID     uuid.UUID
	ProjectTaskID uuid.UUID
	AttemptID     *uuid.UUID
	UpdateKind    string
	Payload       map[string]any
}
```

Route `requirement_changed`, `plan_invalid`, and `scope_changed` to `cancel_and_replan`; route `comment`, `additional_context`, and `evidence_ref` to `next_attempt` unless a Provider hot-inject capability is later registered.

- [ ] **Step 5: Run context update tests**

Run:

```bash
make -C apps/control-plane generate-sqlc
cd apps/control-plane && go test ./internal/storage ./internal/project -run 'ContextUpdate|ProjectTaskAttemptContextUpdatesMigration' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit context updates**

Run:

```bash
git add apps/control-plane/internal/storage apps/control-plane/internal/project
git commit -m "feat(control-plane): record project task context updates"
```

## Task 3: Add ProjectTask Liveness Projection

**Files:**

- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/service_test.go`
- Modify: `apps/control-plane/internal/project/handler.go`
- Modify: `contracts/control-plane/openapi.yaml`

- [ ] **Step 1: Add liveness test**

In `apps/control-plane/internal/project/service_test.go`, add:

```go
func TestProjectTaskLivenessProjectionExplainsNextAction(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	waitingReason := HumanWaitReasonMissingContext
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:               taskID,
		TenantID:         tenantID,
		ProjectID:        projectID,
		Status:           ProjectTaskStatusWaitingHuman,
		CurrentAttemptID: &attemptID,
		WaitingReason:    &waitingReason,
	})

	items, err := service.ListProjectTaskLiveness(context.Background(), tenantID, projectID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, ProjectTaskLivenessWaitingHuman, items[0].Liveness)
	require.Equal(t, "human response", items[0].NextAction)
	require.Equal(t, HumanWaitReasonMissingContext, items[0].Reason)
}
```

- [ ] **Step 2: Add liveness types**

In `apps/control-plane/internal/project/types.go`, add:

```go
const (
	ProjectTaskLivenessBlockedByDependency = "blocked_by_dependency"
	ProjectTaskLivenessReadyToDispatch     = "ready_to_dispatch"
	ProjectTaskLivenessQueued              = "queued"
	ProjectTaskLivenessRunning             = "running"
	ProjectTaskLivenessWaitingHuman        = "waiting_human"
	ProjectTaskLivenessRetryScheduled      = "retry_scheduled"
	ProjectTaskLivenessLeaseLost           = "lease_lost"
	ProjectTaskLivenessTimedOut            = "timed_out"
	ProjectTaskLivenessTerminal            = "terminal"
)

type ProjectTaskLiveness struct {
	ProjectTaskID         uuid.UUID
	Liveness              string
	Reason                string
	BlockingDependencyIDs []uuid.UUID
	CurrentAttemptID      *uuid.UUID
	WaitingRequestID      *uuid.UUID
	RetryNotBefore        *time.Time
	LeaseExpiresAt        *time.Time
	NextAction            string
}
```

- [ ] **Step 3: Implement liveness service**

In `service.go`, implement:

```go
func (s *Service) ListProjectTaskLiveness(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectTaskLiveness, error)
```

Rules:

- terminal task -> `terminal`, next action `no-op terminal`;
- task with incomplete dependency -> `blocked_by_dependency`, next action `dependency completion`;
- planned and dependency-free -> `ready_to_dispatch`, next action `dispatch`;
- queued -> `queued`, next action `runtime start`;
- running with expired lease -> `lease_lost`, next action `recovery policy`;
- running -> `running`, next action `lease renew`;
- waiting_human -> `waiting_human`, next action `human response`;
- retry_not_before in future -> `retry_scheduled`, next action `retry wakeup`.

- [ ] **Step 4: Expose liveness through API**

Either add a dedicated endpoint:

```go
r.Get("/projects/{projectId}/task-liveness", s.projectHandler.ListProjectTaskLiveness)
```

or extend existing task graph response with a `liveness` object. Prefer extending the existing task graph response if it already serves the workflow workbench.

- [ ] **Step 5: Run liveness tests and contracts**

Run:

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
cd apps/control-plane && go test ./internal/project ./internal/api -run 'Liveness|TaskGraph' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit liveness projection**

Run:

```bash
git add apps/control-plane/internal/project apps/control-plane/internal/api contracts/control-plane/openapi.yaml apps/web/src/lib/api/generated
git commit -m "feat(control-plane): expose project task liveness"
```

## Task 4: Converge Operational Status And Read Models

**Files:**

- Modify: `apps/control-plane/internal/storage/queries/employee_execution.sql`
- Modify: generated files under `apps/control-plane/internal/storage/queries/`
- Modify: `apps/control-plane/internal/employee/*`
- Modify: `apps/control-plane/internal/project/*`
- Modify: relevant web generated types if contracts changed

- [ ] **Step 1: Add operational status tests**

Update `apps/control-plane/internal/employee/pg_repository_test.go` so `assertEmployeeOverviewOperationalFactsSQL` requires attempt-aware queued facts:

```go
func assertEmployeeOverviewOperationalFactsSQL(t *testing.T, sql string) {
	t.Helper()

	normalizedSQL := normalizeSQL(sql)
	terminalGuard := "pt.requires_human_approval AND pt.status NOT IN ('completed', 'done', 'success', 'cancelled', 'failed')"
	decisionTypeWhereAllowlist := "AND pdr.decision_type IN ('task_failure_recovery', 'route_review', 'project_acceptance')"
	joinStatusNarrowing := "AND ( pt.status IN ('pending', 'planned', 'queued', 'blocked', 'running', 'in_progress', 'waiting_human', 'pending_review', 'failed') OR ( pt.requires_human_approval AND pt.status NOT IN ('completed', 'done', 'success', 'cancelled', 'failed') ) )"
	queuedTaskStatusNarrowing := "count(pt.id) FILTER (WHERE pt.status IN ('queued')) > 0 AS operational_has_queued_work"
	queuedTaskStatusBroadening := "count(pt.id) FILTER (WHERE pt.status IN ('pending', 'planned', 'blocked', 'assigned')) > 0 AS operational_has_queued_work"

	for _, expected := range []string{
		"employee_operational_facts",
		"pending_employee_decisions",
		"project_acceptance",
		"latest_run_error_family",
		"latest_run_error_code",
		"operational_has_employee_scoped_human_blocker",
		"operational_has_project_acceptance_blocker",
		"task_failure_recovery",
		"route_review",
		"project_task_attempts pta",
		"pta.id = pt.current_attempt_id",
		terminalGuard,
		decisionTypeWhereAllowlist,
		joinStatusNarrowing,
		queuedTaskStatusNarrowing,
		"completed",
		"done",
		"success",
		"cancelled",
		"failed",
	} {
		require.Contains(t, normalizedSQL, expected)
	}
	require.NotContains(t, normalizedSQL, queuedTaskStatusBroadening)
	require.NotContains(t, normalizedSQL, "pt.status IN ('planned', 'assigned')")
	require.NotContains(t, sql, "<> 'project_acceptance'")
	require.Equal(t, 3, strings.Count(normalizedSQL, terminalGuard))
}
```

This keeps `TestEmployeeOverviewSQLCarriesOperationalStatusFacts` as the entry point and makes the query fail until queued attempts are represented in the employee overview SQL.

- [ ] **Step 2: Update operational status queries**

In `apps/control-plane/internal/storage/queries/employee_execution.sql`, change project-task facts:

- include `project_tasks.status = 'queued'`;
- include `project_tasks.status = 'running'`;
- include `project_tasks.status = 'waiting_human'`;
- join `project_task_attempts` through `current_attempt_id`;
- stop using `assigned` as a new active state.

- [ ] **Step 3: Regenerate sqlc and run tests**

Run:

```bash
make -C apps/control-plane generate-sqlc
cd apps/control-plane && go test ./internal/employee ./internal/project -run 'OperationalStatus|TaskGraph|Liveness' -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit read-model convergence**

Run:

```bash
git add apps/control-plane/internal/storage/queries apps/control-plane/internal/employee apps/control-plane/internal/project
git commit -m "feat(control-plane): converge task liveness read models"
```

## Task 5: Runtime Agent Carries Packet Version In Logs

**Files:**

- Modify: `apps/runtime-agent/src/commands/executor.rs`
- Modify: `apps/runtime-agent/tests/runtime_command_executor_test.rs`

- [ ] **Step 1: Add Runtime test assertion**

In `apps/runtime-agent/tests/runtime_command_executor_test.rs`, update structured project task writeback test to assert:

```rust
assert_eq!(
    project_complete.body["confidence_factors"]["execution_context_packet_version"],
    "v1"
);
```

- [ ] **Step 2: Add packet version confidence factor**

In `project_task_complete_writeback`, add:

```rust
confidence_factors.insert(
    "execution_context_packet_version".to_string(),
    serde_json::Value::String(context.execution_context_packet_version.clone()),
);
```

- [ ] **Step 3: Run Runtime tests**

Run:

```bash
cd apps/runtime-agent && cargo test --test runtime_command_executor_test start_session_preserves_structured_project_task_writeback_fields -- --nocapture
```

Expected: PASS.

- [ ] **Step 4: Commit packet version logging**

Run:

```bash
git add apps/runtime-agent/src/commands/executor.rs apps/runtime-agent/tests/runtime_command_executor_test.rs
git commit -m "feat(runtime-agent): report execution packet version"
```

## Task 6: Real Chain Smoke

**Files:**

- Create: `scripts/smoke/project-task-durable-closure.sh`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Create smoke script skeleton**

Create `scripts/smoke/project-task-durable-closure.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

scripts/dev-services.sh status

: "${SUPERTEAM_API_BASE:=http://127.0.0.1:8080}"
: "${SUPERTEAM_AUTH_TOKEN:?set SUPERTEAM_AUTH_TOKEN for authenticated smoke}"

curl_json() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  if [[ -n "$body" ]]; then
    curl -fsS -X "$method" "$SUPERTEAM_API_BASE$path" \
      -H "Authorization: Bearer $SUPERTEAM_AUTH_TOKEN" \
      -H "Content-Type: application/json" \
      --data "$body"
  else
    curl -fsS -X "$method" "$SUPERTEAM_API_BASE$path" \
      -H "Authorization: Bearer $SUPERTEAM_AUTH_TOKEN"
  fi
}

echo "Smoke requires seeded team, employee, runtime, and provider configuration from the local dev stack."
echo "Create demand -> wait for planned task -> dispatch -> runtime attempt -> complete -> read liveness."
```

- [ ] **Step 2: Add concrete readback smoke script**

Complete `scripts/smoke-project-task-durable-closure.sh` as a readback smoke against a seeded durable-closure fixture. The script requires these environment variables so it never invents local test data:

```bash
: "${SUPERTEAM_PROJECT_ID:?set SUPERTEAM_PROJECT_ID to a real project id}"
: "${SUPERTEAM_DEMAND_ID:?set SUPERTEAM_DEMAND_ID to a real demand id}"
: "${SUPERTEAM_TASK_ID:?set SUPERTEAM_TASK_ID to a real project task id}"
: "${SUPERTEAM_ATTEMPT_ID:?set SUPERTEAM_ATTEMPT_ID to the current attempt id}"
```

Add these exact checks after the `api` function:

```bash
task_json="$(api GET "/api/v1/projects/$SUPERTEAM_PROJECT_ID/tasks/$SUPERTEAM_TASK_ID")"
echo "$task_json" | jq -e \
  --arg task_id "$SUPERTEAM_TASK_ID" \
  --arg attempt_id "$SUPERTEAM_ATTEMPT_ID" \
  '.id == $task_id and .current_attempt_id == $attempt_id and (.status | IN("queued", "running", "completed", "waiting_human", "failed", "cancelled"))' >/dev/null

attempt_json="$(api GET "/api/v1/runtime/project-task-attempts/$SUPERTEAM_ATTEMPT_ID")"
echo "$attempt_json" | jq -e \
  --arg task_id "$SUPERTEAM_TASK_ID" \
  '.project_task_id == $task_id and (.attempt_no | type == "number") and (.status | IN("queued", "running", "completed", "failed", "cancelled", "waiting_human"))' >/dev/null

liveness_json="$(api GET "/api/v1/projects/$SUPERTEAM_PROJECT_ID/tasks/$SUPERTEAM_TASK_ID/liveness")"
echo "$liveness_json" | jq -e \
  '.next_action.source != null and (.is_terminal | type == "boolean") and (.attempt.status != null)' >/dev/null

summary_json="$(api GET "/api/v1/projects/$SUPERTEAM_PROJECT_ID/demands/$SUPERTEAM_DEMAND_ID/execution-summary")"
echo "$summary_json" | jq -e \
  --arg task_id "$SUPERTEAM_TASK_ID" \
  '.tasks[] | select(.id == $task_id) | .status != null and .liveness.next_action.source != null' >/dev/null

echo "project-task durable closure smoke passed for task $SUPERTEAM_TASK_ID attempt $SUPERTEAM_ATTEMPT_ID"
```

The plan is not complete until this script is run once against a real local Control Plane and returns exit code 0.

- [ ] **Step 3: Run local service status**

Run:

```bash
scripts/dev-services.sh status
```

Expected: Temporal, Control Plane, Web, and Runtime Agent status are visible. If Runtime Agent or Provider is unavailable, stop and report blocker; do not claim real-chain success.

- [ ] **Step 4: Run smoke**

Run:

```bash
SUPERTEAM_AUTH_TOKEN="$SUPERTEAM_AUTH_TOKEN" scripts/smoke/project-task-durable-closure.sh
```

Expected: script exits 0 and prints final task status `completed`, liveness `terminal`, and a non-empty execution summary id.

- [ ] **Step 5: Run final phase gates**

Run:

```bash
corepack pnpm verify:contracts
cd apps/control-plane && go test ./internal/project ./internal/employee ./internal/api -count=1
cd apps/runtime-agent && cargo test --test runtime_command_executor_test project_task -- --nocapture
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 6: Add changelog and commit**

Run:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Add:

```markdown
- [YYYY-MM-DD HH:MM] ProjectTask durable closure now exposes execution packets, liveness projection, and real-chain smoke coverage.
```

Commit:

```bash
git add scripts/smoke/project-task-durable-closure.sh CHANGELOG.md apps/control-plane apps/runtime-agent contracts/control-plane/openapi.yaml apps/web/src/lib/api/generated
git commit -m "feat(project): verify project task durable closure smoke"
```

Include only files with actual diffs.
