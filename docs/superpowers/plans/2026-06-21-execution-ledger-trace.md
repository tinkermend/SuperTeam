# Execution Ledger Trace Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a project-level execution trace backed by an Execution Ledger so project acceptance, human approvals, and retries can inspect attempt-level evidence instead of only final conclusions.

**Architecture:** Add append-only `execution_ledger_events` as the cross-domain execution fact index, keep `project_task_attempts` and `project_execution_summaries` as source-of-truth state tables, and expose a project-scoped `execution-trace` projection API. The Web project detail page reads only the trace API, so future Runtime/MCP structured collection can keep writing ledger events without changing the UI contract.

**Tech Stack:** PostgreSQL migrations managed by Atlas, sqlc generated Go queries, Go Control Plane project and employee packages, OpenAPI contract generation, React + TanStack Query Web console, Vitest component tests.

---

## Execution Preflight

The root checkout is currently dirty in unrelated frontend and migration files. Start implementation in an isolated worktree via `superpowers:using-git-worktrees` before touching code. Use branch `codex/execution-ledger-trace`.

Confirm current baseline before Task 1:

```bash
git status --short
git log --oneline -3
```

Expected: unrelated dirty files may exist in the root checkout, but the execution worktree should start clean.

## File Structure

- Create `apps/control-plane/internal/storage/migrations/029_execution_ledger_events.sql`: database table, indexes, comments, and trigger.
- Create `apps/control-plane/internal/storage/queries/execution_ledger.sql`: sqlc queries for creating ledger events, listing project trace events, listing project task attempts for trace, and creating provider-event ledger records.
- Modify `apps/control-plane/internal/storage/migrations/atlas.sum`: generated Atlas migration hash.
- Modify generated sqlc files under `apps/control-plane/internal/storage/queries/`: generated models, querier interface, and query implementation.
- Modify `apps/control-plane/internal/storage/migrations_test.go`: migration shape tests.
- Modify `apps/control-plane/internal/storage/queries/queries_test.go`: query and idempotency tests.
- Create `apps/control-plane/internal/project/execution_trace_types.go`: project-domain event, trace, summary, and request/response types.
- Modify `apps/control-plane/internal/project/repository.go`: repository methods for ledger and trace reads.
- Modify `apps/control-plane/internal/project/pg_repository.go`: mapping helpers, repository methods, and transactional writeback ledger writes.
- Modify `apps/control-plane/internal/project/service.go`: `GetExecutionTrace`, `RecordExecutionInvocation`, trace projection, fallback summary linking, and validation.
- Modify `apps/control-plane/internal/project/service_test.go`: service-level trace tests and memory repository support.
- Modify `apps/control-plane/internal/project/handler.go`: handler interface, route handler, response mapping.
- Modify `apps/control-plane/internal/project/handler_test.go`: handler request/response tests.
- Modify `apps/control-plane/internal/api/server.go`: console-auth project route registration.
- Modify `apps/control-plane/internal/api/project_routes_test.go`: API route smoke for `/execution-trace`.
- Modify `apps/control-plane/internal/employee/run_repository.go`: return provider session event IDs from provider event writes.
- Modify `apps/control-plane/internal/employee/pg_run_repository.go`: return inserted/existing provider event ID.
- Modify `apps/control-plane/internal/employee/run_writeback.go`: optional best-effort Execution Ledger recorder for provider events.
- Modify `apps/control-plane/internal/employee/run_writeback_test.go`: provider event recorder tests.
- Modify `apps/control-plane/internal/app/app.go`: wire the project repository as the provider-event ledger recorder.
- Modify `contracts/control-plane/openapi.yaml`: add route and schemas.
- Modify generated API files under `apps/control-plane/internal/api/gen/` and `apps/control-plane/gen/`.
- Modify `apps/web/src/lib/api/projects.ts`: trace API types and client.
- Create `apps/web/src/features/projects/components/project-execution-trace-panel.tsx`: focused trace UI.
- Create `apps/web/src/features/projects/components/project-execution-trace-panel.test.tsx`: component tests.
- Modify `apps/web/src/features/projects/components/project-operational-detail.tsx`: accept and render trace panel.
- Modify `apps/web/src/features/projects/index.tsx`: query `execution-trace` and pass it to detail.

### Task 1: Database Migration And sqlc Queries

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/029_execution_ledger_events.sql`
- Create: `apps/control-plane/internal/storage/queries/execution_ledger.sql`
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
- Modify generated: `apps/control-plane/internal/storage/queries/*.go`
- Modify generated: `apps/control-plane/internal/storage/migrations/atlas.sum`

- [ ] **Step 1: Write the migration shape test**

Add this test to `apps/control-plane/internal/storage/migrations_test.go`:

```go
func TestExecutionLedgerEventsMigration(t *testing.T) {
	sql := readMigration(t, "029_execution_ledger_events.sql")
	block := createTableBlock(t, sql, "execution_ledger_events")

	expected := []string{
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"tenant_id UUID NOT NULL",
		"team_id UUID",
		"project_id UUID NOT NULL",
		"project_task_id UUID",
		"project_task_attempt_id UUID",
		"event_type VARCHAR(100) NOT NULL",
		"source_type VARCHAR(100) NOT NULL",
		"source_id VARCHAR(255) NOT NULL",
		"actor_type VARCHAR(80) NOT NULL",
		"actor_id VARCHAR(255)",
		"runtime_node_id UUID",
		"provider_type VARCHAR(100)",
		"provider_session_id VARCHAR(255)",
		"input_summary TEXT",
		"output_summary TEXT",
		"error_family VARCHAR(100)",
		"error_code VARCHAR(100)",
		"error_message TEXT",
		"retryable BOOLEAN",
		"artifact_refs JSONB NOT NULL DEFAULT '[]'::jsonb",
		"evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb",
		"metadata JSONB NOT NULL DEFAULT '{}'::jsonb",
		"occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"idempotency_key VARCHAR(255) NOT NULL",
	}
	for _, want := range expected {
		if !strings.Contains(block, want) {
			t.Fatalf("expected execution_ledger_events schema to contain %q, got block:\n%s", want, block)
		}
	}

	assertMigrationContains(t, sql, "CREATE UNIQUE INDEX uq_execution_ledger_events_idempotency")
	assertMigrationContains(t, sql, "CREATE INDEX idx_execution_ledger_events_project_time")
	assertMigrationContains(t, sql, "CREATE INDEX idx_execution_ledger_events_attempt_time")
	assertMigrationContains(t, sql, "COMMENT ON TABLE execution_ledger_events IS '执行账本事件表，记录项目任务执行、Provider、工具、MCP、外部能力和证据链的统一审计索引。'")
	assertMigrationContains(t, sql, "COMMENT ON COLUMN execution_ledger_events.input_summary IS '输入摘要，不保存完整 prompt、secret 或大 payload。'")
}
```

- [ ] **Step 2: Run the migration shape test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestExecutionLedgerEventsMigration -count=1
```

Expected: FAIL because `029_execution_ledger_events.sql` does not exist.

- [ ] **Step 3: Add the migration**

Create `apps/control-plane/internal/storage/migrations/029_execution_ledger_events.sql`:

```sql
CREATE TABLE execution_ledger_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    team_id UUID,
    project_id UUID NOT NULL,
    project_task_id UUID,
    project_task_attempt_id UUID,
    event_type VARCHAR(100) NOT NULL,
    source_type VARCHAR(100) NOT NULL,
    source_id VARCHAR(255) NOT NULL,
    actor_type VARCHAR(80) NOT NULL,
    actor_id VARCHAR(255),
    runtime_node_id UUID,
    provider_type VARCHAR(100),
    provider_session_id VARCHAR(255),
    input_summary TEXT,
    output_summary TEXT,
    error_family VARCHAR(100),
    error_code VARCHAR(100),
    error_message TEXT,
    retryable BOOLEAN,
    artifact_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    idempotency_key VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uq_execution_ledger_events_idempotency
    ON execution_ledger_events(tenant_id, idempotency_key);

CREATE INDEX idx_execution_ledger_events_project_time
    ON execution_ledger_events(tenant_id, project_id, occurred_at ASC, created_at ASC);

CREATE INDEX idx_execution_ledger_events_attempt_time
    ON execution_ledger_events(tenant_id, project_task_attempt_id, occurred_at ASC, created_at ASC)
    WHERE project_task_attempt_id IS NOT NULL;

CREATE INDEX idx_execution_ledger_events_project_type
    ON execution_ledger_events(tenant_id, project_id, event_type, occurred_at DESC);

CREATE INDEX idx_execution_ledger_events_project_error
    ON execution_ledger_events(tenant_id, project_id, error_family, occurred_at DESC)
    WHERE error_family IS NOT NULL;

CREATE TRIGGER update_execution_ledger_events_updated_at
    BEFORE UPDATE ON execution_ledger_events
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE execution_ledger_events IS '执行账本事件表，记录项目任务执行、Provider、工具、MCP、外部能力和证据链的统一审计索引。';
COMMENT ON COLUMN execution_ledger_events.id IS '执行账本事件主键 UUID。';
COMMENT ON COLUMN execution_ledger_events.tenant_id IS '执行账本事件所属租户 ID。';
COMMENT ON COLUMN execution_ledger_events.team_id IS '执行账本事件所属团队 ID，可为空以兼容项目未绑定团队的历史数据。';
COMMENT ON COLUMN execution_ledger_events.project_id IS '执行账本事件所属项目 ID。';
COMMENT ON COLUMN execution_ledger_events.project_task_id IS '执行账本事件关联项目任务 ID，可为空表示项目级执行事件。';
COMMENT ON COLUMN execution_ledger_events.project_task_attempt_id IS '执行账本事件关联项目任务执行尝试 ID。';
COMMENT ON COLUMN execution_ledger_events.event_type IS '执行事件类型，例如 attempt.started、provider.event、mcp.tool_call、summary.created。';
COMMENT ON COLUMN execution_ledger_events.source_type IS '来源事实类型，例如 project_task_attempt、provider_session_event、runtime_command_receipt。';
COMMENT ON COLUMN execution_ledger_events.source_id IS '来源事实 ID 或稳定外部 ID。';
COMMENT ON COLUMN execution_ledger_events.actor_type IS '执行事件主体类型，例如 digital_employee、runtime_node、provider、system。';
COMMENT ON COLUMN execution_ledger_events.actor_id IS '执行事件主体 ID 或外部标识。';
COMMENT ON COLUMN execution_ledger_events.runtime_node_id IS '关联 Runtime 节点 ID。';
COMMENT ON COLUMN execution_ledger_events.provider_type IS '关联 Provider 类型，由服务端注册表校验。';
COMMENT ON COLUMN execution_ledger_events.provider_session_id IS 'Provider 外部会话 ID。';
COMMENT ON COLUMN execution_ledger_events.input_summary IS '输入摘要，不保存完整 prompt、secret 或大 payload。';
COMMENT ON COLUMN execution_ledger_events.output_summary IS '输出摘要，不保存完整 raw payload。';
COMMENT ON COLUMN execution_ledger_events.error_family IS '错误分类，例如 provider_error、runtime_error、missing_context、capability_denied。';
COMMENT ON COLUMN execution_ledger_events.error_code IS '细分错误码。';
COMMENT ON COLUMN execution_ledger_events.error_message IS '短错误说明，禁止写入 secret。';
COMMENT ON COLUMN execution_ledger_events.retryable IS '该事件对应失败是否可重试。';
COMMENT ON COLUMN execution_ledger_events.artifact_refs IS '工件引用数组。';
COMMENT ON COLUMN execution_ledger_events.evidence_refs IS '证据引用数组。';
COMMENT ON COLUMN execution_ledger_events.metadata IS '结构化扩展数据，禁止写入 secret 和完整 raw payload。';
COMMENT ON COLUMN execution_ledger_events.occurred_at IS '事件发生时间。';
COMMENT ON COLUMN execution_ledger_events.idempotency_key IS '执行账本事件幂等键。';
COMMENT ON COLUMN execution_ledger_events.created_at IS '执行账本事件创建时间。';
COMMENT ON COLUMN execution_ledger_events.updated_at IS '执行账本事件更新时间。';
```

- [ ] **Step 4: Add sqlc queries**

Create `apps/control-plane/internal/storage/queries/execution_ledger.sql`:

```sql
-- name: CreateExecutionLedgerEvent :one
INSERT INTO execution_ledger_events (
    tenant_id,
    team_id,
    project_id,
    project_task_id,
    project_task_attempt_id,
    event_type,
    source_type,
    source_id,
    actor_type,
    actor_id,
    runtime_node_id,
    provider_type,
    provider_session_id,
    input_summary,
    output_summary,
    error_family,
    error_code,
    error_message,
    retryable,
    artifact_refs,
    evidence_refs,
    metadata,
    occurred_at,
    idempotency_key
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.narg('team_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.narg('project_task_id')::uuid,
    sqlc.narg('project_task_attempt_id')::uuid,
    sqlc.arg('event_type')::varchar,
    sqlc.arg('source_type')::varchar,
    sqlc.arg('source_id')::varchar,
    sqlc.arg('actor_type')::varchar,
    sqlc.narg('actor_id')::varchar,
    sqlc.narg('runtime_node_id')::uuid,
    sqlc.narg('provider_type')::varchar,
    sqlc.narg('provider_session_id')::varchar,
    sqlc.narg('input_summary')::text,
    sqlc.narg('output_summary')::text,
    sqlc.narg('error_family')::varchar,
    sqlc.narg('error_code')::varchar,
    sqlc.narg('error_message')::text,
    sqlc.narg('retryable')::boolean,
    COALESCE(sqlc.narg('artifact_refs')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('evidence_refs')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('metadata')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('occurred_at')::timestamptz, NOW()),
    sqlc.arg('idempotency_key')::varchar
)
ON CONFLICT (tenant_id, idempotency_key) DO UPDATE SET
    idempotency_key = EXCLUDED.idempotency_key
RETURNING *;

-- name: ListProjectExecutionLedgerEvents :many
SELECT *
FROM execution_ledger_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND (sqlc.narg('project_task_id')::uuid IS NULL OR project_task_id = sqlc.narg('project_task_id')::uuid)
  AND (sqlc.narg('project_task_attempt_id')::uuid IS NULL OR project_task_attempt_id = sqlc.narg('project_task_attempt_id')::uuid)
  AND (sqlc.narg('event_type')::varchar IS NULL OR event_type = sqlc.narg('event_type')::varchar)
  AND (sqlc.narg('error_family')::varchar IS NULL OR error_family = sqlc.narg('error_family')::varchar)
ORDER BY occurred_at ASC, created_at ASC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: ListProjectTaskAttemptsForExecutionTrace :many
SELECT pta.*
FROM project_task_attempts pta
JOIN project_tasks pt
  ON pt.tenant_id = pta.tenant_id
 AND pt.id = pta.project_task_id
WHERE pta.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pt.project_id = sqlc.arg('project_id')::uuid
ORDER BY pt.created_at ASC, pta.attempt_no ASC, pta.created_at ASC;

-- name: CreateProviderSessionEventLedgerEvent :one
INSERT INTO execution_ledger_events (
    tenant_id,
    team_id,
    project_id,
    project_task_id,
    project_task_attempt_id,
    event_type,
    source_type,
    source_id,
    actor_type,
    actor_id,
    runtime_node_id,
    provider_type,
    provider_session_id,
    input_summary,
    output_summary,
    error_family,
    metadata,
    occurred_at,
    idempotency_key
)
SELECT
    pse.tenant_id,
    p.team_id,
    pt.project_id,
    pt.id,
    pta.id,
    'provider.event',
    'provider_session_event',
    pse.id::varchar,
    'provider',
    pse.provider_type,
    pse.runtime_node_id,
    pse.provider_type,
    ps.provider_session_id,
    NULLIF(pse.event_type, ''),
    COALESCE(NULLIF(pse.payload->>'summary', ''), NULLIF(pse.payload->>'text', ''), pse.event_type),
    ps.last_error_family,
    jsonb_build_object(
        'command_id', pse.command_id,
        'sequence_number', pse.sequence_number,
        'raw_event_ref', pse.raw_event_ref,
        'log_ref', pse.log_ref
    ),
    pse.created_at,
    'provider_session_event:' || pse.id::varchar || ':provider.event'
FROM provider_session_events pse
JOIN provider_sessions ps
  ON ps.tenant_id = pse.tenant_id
 AND ps.id = pse.provider_session_id
JOIN project_task_attempts pta
  ON pta.tenant_id = pse.tenant_id
 AND pta.digital_employee_run_id = sqlc.arg('digital_employee_run_id')::uuid
JOIN project_tasks pt
  ON pt.tenant_id = pta.tenant_id
 AND pt.id = pta.project_task_id
JOIN projects p
  ON p.tenant_id = pt.tenant_id
 AND p.id = pt.project_id
WHERE pse.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pse.id = sqlc.arg('provider_session_event_id')::uuid
ON CONFLICT (tenant_id, idempotency_key) DO UPDATE SET
    idempotency_key = EXCLUDED.idempotency_key
RETURNING *;
```

- [ ] **Step 5: Run migration test and sqlc generation**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestExecutionLedgerEventsMigration -count=1
make -C apps/control-plane generate-sqlc
(cd apps/control-plane && atlas migrate hash --dir file://internal/storage/migrations)
```

Expected: migration test PASS; sqlc generates `ExecutionLedgerEvent` model and query methods; `atlas.sum` changes.

- [ ] **Step 6: Add query tests**

Add tests to `apps/control-plane/internal/storage/queries/queries_test.go`:

```go
func TestExecutionLedgerEventQueries(t *testing.T) {
	ctx := context.Background()
	tenantID := testTenantID
	teamID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()

	event, err := testQueries.CreateExecutionLedgerEvent(ctx, queries.CreateExecutionLedgerEventParams{
		TenantID:             tenantID,
		TeamID:               uuid.NullUUID{UUID: teamID, Valid: true},
		ProjectID:            projectID,
		ProjectTaskID:        uuid.NullUUID{UUID: taskID, Valid: true},
		ProjectTaskAttemptID: uuid.NullUUID{UUID: attemptID, Valid: true},
		EventType:            "attempt.started",
		SourceType:           "project_task_attempt",
		SourceID:             attemptID.String(),
		ActorType:            "runtime_node",
		ActorID:              pgtype.Text{String: "runtime-node", Valid: true},
		RuntimeNodeID:        uuid.NullUUID{UUID: uuid.New(), Valid: true},
		InputSummary:         pgtype.Text{String: "Runtime started project task attempt", Valid: true},
		Metadata:             []byte(`{"source":"test"}`),
		IdempotencyKey:       "project_task_attempt:" + attemptID.String() + ":attempt.started",
	})
	require.NoError(t, err)
	require.Equal(t, "attempt.started", event.EventType)

	duplicate, err := testQueries.CreateExecutionLedgerEvent(ctx, queries.CreateExecutionLedgerEventParams{
		TenantID:             tenantID,
		TeamID:               uuid.NullUUID{UUID: teamID, Valid: true},
		ProjectID:            projectID,
		ProjectTaskID:        uuid.NullUUID{UUID: taskID, Valid: true},
		ProjectTaskAttemptID: uuid.NullUUID{UUID: attemptID, Valid: true},
		EventType:            "attempt.started",
		SourceType:           "project_task_attempt",
		SourceID:             attemptID.String(),
		ActorType:            "runtime_node",
		InputSummary:         pgtype.Text{String: "Duplicate write", Valid: true},
		Metadata:             []byte(`{"source":"duplicate"}`),
		IdempotencyKey:       "project_task_attempt:" + attemptID.String() + ":attempt.started",
	})
	require.NoError(t, err)
	require.Equal(t, event.ID, duplicate.ID)

	events, err := testQueries.ListProjectExecutionLedgerEvents(ctx, queries.ListProjectExecutionLedgerEventsParams{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     20,
		Offset:    0,
	})
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, event.ID, events[0].ID)
}
```

- [ ] **Step 7: Run query tests**

Run:

```bash
go test ./apps/control-plane/internal/storage/queries -run TestExecutionLedgerEventQueries -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit Task 1**

Run:

```bash
git add apps/control-plane/internal/storage/migrations/029_execution_ledger_events.sql apps/control-plane/internal/storage/migrations/atlas.sum apps/control-plane/internal/storage/queries apps/control-plane/internal/storage/migrations_test.go
git commit -m "feat(control-plane): add execution ledger schema"
```

### Task 2: Domain Types, Repository Methods, And Trace Projection Foundation

**Files:**
- Create: `apps/control-plane/internal/project/execution_trace_types.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/service_test.go`
- Modify: `apps/control-plane/internal/project/service.go`

- [ ] **Step 1: Write the failing service trace projection test**

Add this test to `apps/control-plane/internal/project/service_test.go`:

```go
func TestGetExecutionTraceGroupsEventsByAttempt(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	summaryID := uuid.New()
	now := time.Now().UTC()
	repo := newMemoryRepository()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:        taskID,
		TenantID:  tenantID,
		ProjectID: projectID,
		Title:     "核对证据链",
		Status:    ProjectTaskStatusCompleted,
	})
	repo.projectTaskAttempts = append(repo.projectTaskAttempts, ProjectTaskAttempt{
		ID:            attemptID,
		TenantID:      tenantID,
		ProjectTaskID: taskID,
		AttemptNo:     1,
		Status:        ProjectTaskAttemptStatusSucceeded,
		StartedAt:     &now,
		FinishedAt:    &now,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	repo.executionSummaries = append(repo.executionSummaries, ExecutionSummary{
		ID:                  summaryID,
		TenantID:            tenantID,
		ProjectID:           projectID,
		ProjectTaskID:       taskID,
		DigitalEmployeeID:   uuid.New(),
		Conclusion:          "证据链完整",
		EvidenceRefs:        []any{map[string]any{"ref": "evidence://1"}},
		ArtifactRefs:        []any{map[string]any{"ref": "artifact://1"}},
		ConfidenceFactors:   map[string]any{"source": "test"},
		MissingInformation:  []any{},
		RequiresHumanReview: false,
		CreatedAt:           now,
	})
	repo.executionLedgerEvents = append(repo.executionLedgerEvents, ExecutionLedgerEvent{
		ID:                   uuid.New(),
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        &taskID,
		ProjectTaskAttemptID: &attemptID,
		EventType:            ExecutionLedgerEventAttemptStarted,
		SourceType:           "project_task_attempt",
		SourceID:             attemptID.String(),
		ActorType:            "runtime_node",
		InputSummary:         strPtr("Runtime started attempt"),
		OccurredAt:           now,
		CreatedAt:            now,
	})
	repo.executionLedgerEvents = append(repo.executionLedgerEvents, ExecutionLedgerEvent{
		ID:                   uuid.New(),
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        &taskID,
		ProjectTaskAttemptID: &attemptID,
		EventType:            ExecutionLedgerEventSummaryCreated,
		SourceType:           "project_execution_summary",
		SourceID:             summaryID.String(),
		ActorType:            "system",
		OutputSummary:        strPtr("证据链完整"),
		OccurredAt:           now.Add(time.Second),
		CreatedAt:            now.Add(time.Second),
	})

	service := NewService(repo, nil)
	trace, err := service.GetExecutionTrace(context.Background(), GetExecutionTraceRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     100,
	})
	require.NoError(t, err)
	require.Equal(t, projectID, trace.ProjectID)
	require.Equal(t, int32(1), trace.Summary.AttemptCount)
	require.Equal(t, int32(1), trace.Summary.ArtifactRefCount)
	require.Equal(t, int32(1), trace.Summary.EvidenceRefCount)
	require.Len(t, trace.Attempts, 1)
	require.Equal(t, attemptID, trace.Attempts[0].AttemptID)
	require.Len(t, trace.Attempts[0].Events, 2)
	require.NotNil(t, trace.Attempts[0].Summary)
	require.Equal(t, summaryID, trace.Attempts[0].Summary.ExecutionSummaryID)
}
```

- [ ] **Step 2: Run the test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestGetExecutionTraceGroupsEventsByAttempt -count=1
```

Expected: FAIL because execution trace types and service methods are missing.

- [ ] **Step 3: Add domain types**

Create `apps/control-plane/internal/project/execution_trace_types.go`:

```go
package project

import (
	"time"

	"github.com/google/uuid"
)

const (
	ExecutionLedgerEventAttemptStarted       = "attempt.started"
	ExecutionLedgerEventAttemptCompleted     = "attempt.completed"
	ExecutionLedgerEventAttemptFailed        = "attempt.failed"
	ExecutionLedgerEventAttemptWaitingHuman  = "attempt.waiting_human"
	ExecutionLedgerEventSummaryCreated       = "summary.created"
	ExecutionLedgerEventProviderSessionStart = "provider.session.started"
	ExecutionLedgerEventProviderEvent        = "provider.event"
	ExecutionLedgerEventToolCall             = "tool.call"
	ExecutionLedgerEventMCPToolCall          = "mcp.tool_call"
	ExecutionLedgerEventCapabilityInvocation = "capability.invocation"
	ExecutionLedgerEventArtifactLinked       = "artifact.linked"
	ExecutionLedgerEventEvidenceLinked       = "evidence.linked"
)

type ExecutionLedgerEvent struct {
	ID                   uuid.UUID
	TenantID             uuid.UUID
	TeamID               *uuid.UUID
	ProjectID            uuid.UUID
	ProjectTaskID        *uuid.UUID
	ProjectTaskAttemptID *uuid.UUID
	EventType            string
	SourceType           string
	SourceID             string
	ActorType            string
	ActorID              *string
	RuntimeNodeID        *uuid.UUID
	ProviderType         *string
	ProviderSessionID    *string
	InputSummary         *string
	OutputSummary        *string
	ErrorFamily          *string
	ErrorCode            *string
	ErrorMessage         *string
	Retryable            *bool
	ArtifactRefs         []any
	EvidenceRefs         []any
	Metadata             map[string]any
	OccurredAt           time.Time
	IdempotencyKey       string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type CreateExecutionLedgerEventRequest struct {
	TenantID             uuid.UUID
	TeamID               *uuid.UUID
	ProjectID            uuid.UUID
	ProjectTaskID        *uuid.UUID
	ProjectTaskAttemptID *uuid.UUID
	EventType            string
	SourceType           string
	SourceID             string
	ActorType            string
	ActorID              *string
	RuntimeNodeID        *uuid.UUID
	ProviderType         *string
	ProviderSessionID    *string
	InputSummary         string
	OutputSummary        string
	ErrorFamily          string
	ErrorCode            string
	ErrorMessage         string
	Retryable            *bool
	ArtifactRefs         []any
	EvidenceRefs         []any
	Metadata             map[string]any
	OccurredAt           *time.Time
	IdempotencyKey       string
}

type GetExecutionTraceRequest struct {
	TenantID             uuid.UUID
	ProjectID            uuid.UUID
	ProjectTaskID        *uuid.UUID
	ProjectTaskAttemptID *uuid.UUID
	EventType            *string
	ErrorFamily          *string
	Limit                int32
	Offset               int32
}

type ProjectExecutionTrace struct {
	ProjectID uuid.UUID
	Summary   ProjectExecutionTraceSummary
	Attempts  []ProjectExecutionTraceAttempt
}

type ProjectExecutionTraceSummary struct {
	AttemptCount             int32
	FailedAttemptCount       int32
	HumanReviewRequiredCount int32
	ArtifactRefCount         int32
	EvidenceRefCount         int32
	LatestErrorFamily        *string
}

type ProjectExecutionTraceAttempt struct {
	ProjectTaskID     uuid.UUID
	AttemptID         uuid.UUID
	AttemptNo         int32
	Status            string
	RuntimeNodeID     *uuid.UUID
	ProviderType      *string
	ProviderSessionID *string
	StartedAt         *time.Time
	FinishedAt        *time.Time
	FailureFamily     *string
	Retryable         *bool
	Events            []ExecutionLedgerEvent
	Summary           *ProjectExecutionTraceAttemptSummary
}

type ProjectExecutionTraceAttemptSummary struct {
	ExecutionSummaryID  uuid.UUID
	Conclusion          string
	RequiresHumanReview bool
	ArtifactRefs        []any
	EvidenceRefs        []any
	CreatedAt           time.Time
}
```

- [ ] **Step 4: Extend repository interfaces**

Add methods to `Repository` in `apps/control-plane/internal/project/repository.go`:

```go
	CreateExecutionLedgerEvent(ctx context.Context, req CreateExecutionLedgerEventRequest) (ExecutionLedgerEvent, error)
	ListProjectExecutionLedgerEvents(ctx context.Context, req GetExecutionTraceRequest) ([]ExecutionLedgerEvent, error)
	ListProjectTaskAttemptsForExecutionTrace(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectTaskAttempt, error)
```

Add a small interface for provider writeback in the same file:

```go
type ProviderEventExecutionLedgerRepository interface {
	CreateProviderSessionEventLedgerEvent(ctx context.Context, tenantID, digitalEmployeeRunID, providerSessionEventID uuid.UUID) (ExecutionLedgerEvent, error)
}
```

- [ ] **Step 5: Add pg repository mappings and methods**

Add the reader mapper to `apps/control-plane/internal/project/pg_repository.go`, next to the existing `*FromRecord` mappers. **Reuse the existing reader helpers** — `ptrText(pgtype.Text) *string`, `ptrUUID(uuid.NullUUID) *uuid.UUID`, `ptrBool(pgtype.Bool) *bool` — and read NOT NULL timestamptz columns via `.Time`, exactly like `executionSummaryFromRecord` and `projectTaskAttemptFromRecord` do. Do **not** invent `uuidPtr` / `stringPtrFromText` / `timeFromTimestamptz` / `boolPtrFromPg` — those names do not exist in the package and would duplicate the `ptr*` helpers.

```go
func executionLedgerEventFromRecord(row queries.ExecutionLedgerEvent) (ExecutionLedgerEvent, error) {
	artifactRefs, err := anySliceFromJSON(row.ArtifactRefs)
	if err != nil {
		return ExecutionLedgerEvent{}, fmt.Errorf("artifact_refs: %w", err)
	}
	evidenceRefs, err := anySliceFromJSON(row.EvidenceRefs)
	if err != nil {
		return ExecutionLedgerEvent{}, fmt.Errorf("evidence_refs: %w", err)
	}
	metadata, err := mapFromJSON(row.Metadata)
	if err != nil {
		return ExecutionLedgerEvent{}, fmt.Errorf("metadata: %w", err)
	}
	return ExecutionLedgerEvent{
		ID:                   row.ID,
		TenantID:             row.TenantID,
		TeamID:               ptrUUID(row.TeamID),
		ProjectID:            row.ProjectID,
		ProjectTaskID:        ptrUUID(row.ProjectTaskID),
		ProjectTaskAttemptID: ptrUUID(row.ProjectTaskAttemptID),
		EventType:            row.EventType,
		SourceType:           row.SourceType,
		SourceID:             row.SourceID,
		ActorType:            row.ActorType,
		ActorID:              ptrText(row.ActorID),
		RuntimeNodeID:        ptrUUID(row.RuntimeNodeID),
		ProviderType:         ptrText(row.ProviderType),
		ProviderSessionID:    ptrText(row.ProviderSessionID),
		InputSummary:         ptrText(row.InputSummary),
		OutputSummary:        ptrText(row.OutputSummary),
		ErrorFamily:          ptrText(row.ErrorFamily),
		ErrorCode:            ptrText(row.ErrorCode),
		ErrorMessage:         ptrText(row.ErrorMessage),
		Retryable:            ptrBool(row.Retryable),
		ArtifactRefs:         artifactRefs,
		EvidenceRefs:         evidenceRefs,
		Metadata:             metadata,
		OccurredAt:           row.OccurredAt.Time,
		IdempotencyKey:       row.IdempotencyKey,
		CreatedAt:            row.CreatedAt.Time,
		UpdatedAt:            row.UpdatedAt.Time,
	}, nil
}
```

Add the two writer helpers that are genuinely missing (the existing `textOrNull(string)` takes a non-pointer string, and there is no `*bool → pgtype.Bool` helper). Reuse the existing `nullUUID(*uuid.UUID)` and `timestamptzPtr(*time.Time)` for the other nullable columns:

```go
func textFromPtr(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func boolToPg(value *bool) pgtype.Bool {
	if value == nil {
		return pgtype.Bool{Valid: false}
	}
	return pgtype.Bool{Bool: *value, Valid: true}
}
```

Add repository methods. The request's nullable text fields (`ActorID`, `ProviderType`, `ProviderSessionID` are `*string`) use `textFromPtr`; the non-pointer string fields (`InputSummary`, `OutputSummary`, `ErrorFamily`, `ErrorCode`, `ErrorMessage`) use `textOrNull`:

```go
func (r *PgRepository) CreateExecutionLedgerEvent(ctx context.Context, req CreateExecutionLedgerEventRequest) (ExecutionLedgerEvent, error) {
	return r.createExecutionLedgerEventWithQueries(ctx, r.q, req)
}

func (r *PgRepository) createExecutionLedgerEventWithQueries(ctx context.Context, q *queries.Queries, req CreateExecutionLedgerEventRequest) (ExecutionLedgerEvent, error) {
	artifactRefs, err := jsonbArray(req.ArtifactRefs, "artifact_refs")
	if err != nil {
		return ExecutionLedgerEvent{}, err
	}
	evidenceRefs, err := jsonbArray(req.EvidenceRefs, "evidence_refs")
	if err != nil {
		return ExecutionLedgerEvent{}, err
	}
	metadata, err := jsonbObject(req.Metadata, "metadata")
	if err != nil {
		return ExecutionLedgerEvent{}, err
	}
	row, err := q.CreateExecutionLedgerEvent(ctx, queries.CreateExecutionLedgerEventParams{
		TenantID:             req.TenantID,
		TeamID:               nullUUID(req.TeamID),
		ProjectID:            req.ProjectID,
		ProjectTaskID:        nullUUID(req.ProjectTaskID),
		ProjectTaskAttemptID: nullUUID(req.ProjectTaskAttemptID),
		EventType:            req.EventType,
		SourceType:           req.SourceType,
		SourceID:             req.SourceID,
		ActorType:            req.ActorType,
		ActorID:              textFromPtr(req.ActorID),
		RuntimeNodeID:        nullUUID(req.RuntimeNodeID),
		ProviderType:         textFromPtr(req.ProviderType),
		ProviderSessionID:    textFromPtr(req.ProviderSessionID),
		InputSummary:         textOrNull(req.InputSummary),
		OutputSummary:        textOrNull(req.OutputSummary),
		ErrorFamily:          textOrNull(req.ErrorFamily),
		ErrorCode:            textOrNull(req.ErrorCode),
		ErrorMessage:         textOrNull(req.ErrorMessage),
		Retryable:            boolToPg(req.Retryable),
		ArtifactRefs:         artifactRefs,
		EvidenceRefs:         evidenceRefs,
		Metadata:             metadata,
		OccurredAt:           timestamptzPtr(req.OccurredAt),
		IdempotencyKey:       req.IdempotencyKey,
	})
	if err != nil {
		return ExecutionLedgerEvent{}, err
	}
	return executionLedgerEventFromRecord(row)
}
```

- [ ] **Step 6: Implement service projection**

Add this method to `apps/control-plane/internal/project/service.go`:

```go
func (s *Service) GetExecutionTrace(ctx context.Context, req GetExecutionTraceRequest) (*ProjectExecutionTrace, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	req.Limit, req.Offset = normalizePagination(req.Limit, req.Offset)
	if req.Limit < 100 {
		req.Limit = 100
	}
	attempts, err := s.repository.ListProjectTaskAttemptsForExecutionTrace(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	events, err := s.repository.ListProjectExecutionLedgerEvents(ctx, req)
	if err != nil {
		return nil, err
	}
	summaries, err := s.repository.ListExecutionSummaries(ctx, req.TenantID, req.ProjectID, 1000, 0)
	if err != nil {
		return nil, err
	}
	trace := buildProjectExecutionTrace(req.ProjectID, attempts, events, summaries)
	return &trace, nil
}
```

Add `buildProjectExecutionTrace` in the same file. It must:

- Create one `ProjectExecutionTraceAttempt` per attempt.
- Attach events by `ProjectTaskAttemptID`.
- Attach `summary.created` events by `SourceID == summary.ID.String()`.
- Fallback attach the newest summary for a task when no `summary.created` event exists.
- Count failed attempts by attempt status.
- Count human review by attached summary `RequiresHumanReview`.
- Count artifact/evidence refs from attached summaries and ledger event refs.
- Set `LatestErrorFamily` from the latest event with `ErrorFamily != nil`.

- [ ] **Step 7: Extend memory repository test fake**

Add `executionLedgerEvents []ExecutionLedgerEvent` to `memoryRepository` in `apps/control-plane/internal/project/service_test.go`.

Add methods:

```go
func (r *memoryRepository) CreateExecutionLedgerEvent(_ context.Context, req CreateExecutionLedgerEventRequest) (ExecutionLedgerEvent, error) {
	for _, event := range r.executionLedgerEvents {
		if event.TenantID == req.TenantID && event.IdempotencyKey == req.IdempotencyKey {
			return event, nil
		}
	}
	now := time.Now().UTC()
	occurredAt := now
	if req.OccurredAt != nil {
		occurredAt = *req.OccurredAt
	}
	event := ExecutionLedgerEvent{
		ID:                   uuid.New(),
		TenantID:             req.TenantID,
		TeamID:               req.TeamID,
		ProjectID:            req.ProjectID,
		ProjectTaskID:        req.ProjectTaskID,
		ProjectTaskAttemptID: req.ProjectTaskAttemptID,
		EventType:            req.EventType,
		SourceType:           req.SourceType,
		SourceID:             req.SourceID,
		ActorType:            req.ActorType,
		ActorID:              req.ActorID,
		RuntimeNodeID:        req.RuntimeNodeID,
		ProviderType:         req.ProviderType,
		ProviderSessionID:    req.ProviderSessionID,
		InputSummary:         strPtrOrNil(req.InputSummary),
		OutputSummary:        strPtrOrNil(req.OutputSummary),
		ErrorFamily:          strPtrOrNil(req.ErrorFamily),
		ErrorCode:            strPtrOrNil(req.ErrorCode),
		ErrorMessage:         strPtrOrNil(req.ErrorMessage),
		Retryable:            req.Retryable,
		ArtifactRefs:         append([]any(nil), req.ArtifactRefs...),
		EvidenceRefs:         append([]any(nil), req.EvidenceRefs...),
		Metadata:             cloneMap(req.Metadata),
		OccurredAt:           occurredAt,
		IdempotencyKey:       req.IdempotencyKey,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	r.executionLedgerEvents = append(r.executionLedgerEvents, event)
	return event, nil
}

func (r *memoryRepository) ListProjectExecutionLedgerEvents(_ context.Context, req GetExecutionTraceRequest) ([]ExecutionLedgerEvent, error) {
	events := make([]ExecutionLedgerEvent, 0, len(r.executionLedgerEvents))
	for _, event := range r.executionLedgerEvents {
		if event.TenantID != req.TenantID || event.ProjectID != req.ProjectID {
			continue
		}
		if req.ProjectTaskID != nil && (event.ProjectTaskID == nil || *event.ProjectTaskID != *req.ProjectTaskID) {
			continue
		}
		if req.ProjectTaskAttemptID != nil && (event.ProjectTaskAttemptID == nil || *event.ProjectTaskAttemptID != *req.ProjectTaskAttemptID) {
			continue
		}
		events = append(events, event)
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].OccurredAt.Equal(events[j].OccurredAt) {
			return events[i].CreatedAt.Before(events[j].CreatedAt)
		}
		return events[i].OccurredAt.Before(events[j].OccurredAt)
	})
	return events, nil
}

func (r *memoryRepository) ListProjectTaskAttemptsForExecutionTrace(_ context.Context, tenantID, projectID uuid.UUID) ([]ProjectTaskAttempt, error) {
	taskProject := map[uuid.UUID]bool{}
	for _, task := range r.tasks {
		if task.TenantID == tenantID && task.ProjectID == projectID {
			taskProject[task.ID] = true
		}
	}
	attempts := []ProjectTaskAttempt{}
	for _, attempt := range r.projectTaskAttempts {
		if attempt.TenantID == tenantID && taskProject[attempt.ProjectTaskID] {
			attempts = append(attempts, attempt)
		}
	}
	sort.SliceStable(attempts, func(i, j int) bool {
		if attempts[i].ProjectTaskID == attempts[j].ProjectTaskID {
			return attempts[i].AttemptNo < attempts[j].AttemptNo
		}
		return attempts[i].CreatedAt.Before(attempts[j].CreatedAt)
	})
	return attempts, nil
}
```

- [ ] **Step 8: Run the service projection test**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestGetExecutionTraceGroupsEventsByAttempt -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit Task 2**

Run:

```bash
git add apps/control-plane/internal/project/execution_trace_types.go apps/control-plane/internal/project/repository.go apps/control-plane/internal/project/pg_repository.go apps/control-plane/internal/project/service.go apps/control-plane/internal/project/service_test.go
git commit -m "feat(project): add execution trace projection"
```

### Task 3: Write Ledger Events In Project Task Attempt Writebacks

**Files:**
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository_test.go`
- Modify: `apps/control-plane/internal/project/service_test.go`

- [ ] **Step 1: Write failing writeback tests**

Add tests in `apps/control-plane/internal/project/service_test.go`:

```go
func TestStartProjectTaskAttemptWritesLedgerEvent(t *testing.T) {
	repo := newMemoryRepository()
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusQueued, ProjectTaskAttemptStatusQueued)
	service := NewService(repo, nil)
	_, err := service.StartProjectTaskAttempt(context.Background(), StartProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("start-ledger"),
	})
	require.NoError(t, err)
	require.Len(t, repo.executionLedgerEvents, 1)
	require.Equal(t, ExecutionLedgerEventAttemptStarted, repo.executionLedgerEvents[0].EventType)
	require.Equal(t, fixture.attemptID, *repo.executionLedgerEvents[0].ProjectTaskAttemptID)
}

func TestCompleteProjectTaskAttemptWritesLedgerEvents(t *testing.T) {
	repo := newMemoryRepository()
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	service := NewService(repo, nil)
	_, err := service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("complete-ledger"),
		Conclusion:            "验收证据已生成",
		EvidenceRefs:          []any{map[string]any{"ref": "evidence://complete"}},
		ArtifactRefs:          []any{map[string]any{"ref": "artifact://complete"}},
		ConfidenceFactors:     map[string]any{"verified": true},
		MissingInformation:    []any{},
		RecommendedNextAction: "进入验收",
	})
	require.NoError(t, err)
	requireLedgerEventTypes(t, repo.executionLedgerEvents, ExecutionLedgerEventAttemptCompleted, ExecutionLedgerEventSummaryCreated)
}
```

Add helper:

```go
func requireLedgerEventTypes(t *testing.T, events []ExecutionLedgerEvent, expected ...string) {
	t.Helper()
	actual := make([]string, 0, len(events))
	for _, event := range events {
		actual = append(actual, event.EventType)
	}
	for _, eventType := range expected {
		require.Contains(t, actual, eventType)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'Test(Start|Complete)ProjectTaskAttemptWritesLedgerEvents?' -count=1
```

Expected: FAIL because writeback paths do not create ledger events.

- [ ] **Step 3: Add service-level ledger calls for start**

In `StartProjectTaskAttempt`, after `StartProjectTaskAttemptWriteback`, add. Use the existing `strPtr(string) *string` helper (defined in `service.go`) for `ActorID` — `stringPtr` takes `*uuid.UUID`, not a string, and will not compile:

```go
	_, _ = s.repository.CreateExecutionLedgerEvent(ctx, CreateExecutionLedgerEventRequest{
		TenantID:             req.TenantID,
		ProjectID:            result.Task.ProjectID,
		ProjectTaskID:        &req.ProjectTaskID,
		ProjectTaskAttemptID: &req.AttemptID,
		EventType:            ExecutionLedgerEventAttemptStarted,
		SourceType:           "project_task_attempt",
		SourceID:             req.AttemptID.String(),
		ActorType:            "runtime_node",
		ActorID:              strPtr(req.RuntimeNodeID.String()),
		RuntimeNodeID:        &req.RuntimeNodeID,
		ProviderSessionID:    req.ProviderSessionID,
		InputSummary:         "Runtime started project task attempt",
		Metadata: map[string]any{
			"project_task_id": req.ProjectTaskID.String(),
			"idempotency_key": req.IdempotencyKey,
		},
		IdempotencyKey: "project_task_attempt:" + req.AttemptID.String() + ":attempt.started",
	})
```

The start ledger write is best-effort because the attempt has already moved to running. The API trace can still expose a fallback started event from the attempt row if this best-effort write fails.

- [ ] **Step 4: Add transactional ledger writes in pg repository terminal writebacks**

In `CompleteProjectTaskAttemptWriteback`, after `FinishProjectTaskAttempt`, call `createExecutionLedgerEventWithQueries` twice inside the existing transaction:

```go
		attemptID := req.AttemptID
		_, err = r.createExecutionLedgerEventWithQueries(ctx, q, CreateExecutionLedgerEventRequest{
			TenantID:             req.TenantID,
			ProjectID:            task.ProjectID,
			ProjectTaskID:        &task.ID,
			ProjectTaskAttemptID: &attemptID,
			EventType:            ExecutionLedgerEventAttemptCompleted,
			SourceType:           "project_task_attempt",
			SourceID:             req.AttemptID.String(),
			ActorType:            "digital_employee",
			ActorID:              strPtr(req.DigitalEmployeeID.String()),
			RuntimeNodeID:        &req.RuntimeNodeID,
			ProviderSessionID:    req.ProviderSessionID,
			OutputSummary:        req.Conclusion,
			ArtifactRefs:         sliceOrEmptyAny(req.ArtifactRefs),
			EvidenceRefs:         sliceOrEmptyAny(req.EvidenceRefs),
			Metadata: map[string]any{
				"project_event_id": event.ID.String(),
			},
			IdempotencyKey: "project_task_attempt:" + req.AttemptID.String() + ":attempt.completed",
		})
		if err != nil {
			return ProjectTaskWritebackResult{}, err
		}
		summaryID := summary.ID
		_, err = r.createExecutionLedgerEventWithQueries(ctx, q, CreateExecutionLedgerEventRequest{
			TenantID:             req.TenantID,
			ProjectID:            task.ProjectID,
			ProjectTaskID:        &task.ID,
			ProjectTaskAttemptID: &attemptID,
			EventType:            ExecutionLedgerEventSummaryCreated,
			SourceType:           "project_execution_summary",
			SourceID:             summary.ID.String(),
			ActorType:            "digital_employee",
			ActorID:              strPtr(req.DigitalEmployeeID.String()),
			RuntimeNodeID:        &req.RuntimeNodeID,
			ProviderSessionID:    req.ProviderSessionID,
			OutputSummary:        summary.Conclusion,
			ArtifactRefs:         summary.ArtifactRefs,
			EvidenceRefs:         summary.EvidenceRefs,
			Metadata: map[string]any{
				"execution_summary_id": summaryID.String(),
			},
			IdempotencyKey: "project_execution_summary:" + summary.ID.String() + ":summary.created",
		})
		if err != nil {
			return ProjectTaskWritebackResult{}, err
		}
```

Add equivalent terminal event writes in:

- `CompleteProjectTaskAttemptAcceptanceWriteback`: use `attempt.completed` and `summary.created`, set `RequiresHumanReview` metadata to true and the task later moves to waiting human.
- `FailProjectTaskAttemptWriteback`: use `attempt.failed`, set `ErrorFamily`, `ErrorMessage`, and `Retryable`.
- `RecoverProjectTaskAttemptFailureWriteback`: use the terminal attempt status mapped by `req.AttemptTerminalStatus`, with `attempt.failed` for failed/timed out/lost and `attempt.waiting_human` when the task target is waiting human.
- `WaitHumanProjectTaskAttemptWriteback`: use `attempt.waiting_human`, set `OutputSummary` to `req.Wait.Summary`.

- [ ] **Step 5: Update memory repository writeback fakes**

In the memory repository writeback methods in `service_test.go`, append the same event types to `r.executionLedgerEvents` after the fake state mutation. Keep idempotency keys identical to production.

- [ ] **Step 6: Run project service tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'Test(Start|Complete|Fail|WaitHuman)ProjectTaskAttempt' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit Task 3**

Run:

```bash
git add apps/control-plane/internal/project/pg_repository.go apps/control-plane/internal/project/service.go apps/control-plane/internal/project/service_test.go
git commit -m "feat(project): record task attempt ledger events"
```

### Task 4: Index Provider Session Events Into The Ledger

**Files:**
- Modify: `apps/control-plane/internal/employee/run_repository.go`
- Modify: `apps/control-plane/internal/employee/pg_run_repository.go`
- Modify: `apps/control-plane/internal/employee/run_writeback.go`
- Modify: `apps/control-plane/internal/employee/run_writeback_test.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/app/app.go`

- [ ] **Step 1: Write failing provider event recorder test**

Add to `apps/control-plane/internal/employee/run_writeback_test.go`:

```go
func TestWritebackEventRecordsProviderLedgerBestEffort(t *testing.T) {
	repo := newFakeRunWritebackRepository()
	run := repo.seedRunningRun()
	recorder := &fakeExecutionLedgerRecorder{}
	service, err := NewDigitalEmployeeRunWritebackService(repo, nil, nil, recorder)
	require.NoError(t, err)

	providerSessionExternalID := "provider-session-ledger"
	err = service.RecordEvent(context.Background(), runtimeIdentity(run), run.CommandID, RuntimeCommandEventWriteback{
		EventType:                 "provider.tool_call",
		SequenceNumber:            7,
		Payload:                   map[string]any{"summary": "调用 MCP 工具"},
		ProviderSessionExternalID: &providerSessionExternalID,
		Metadata:                  map[string]any{"source": "test"},
	})
	require.NoError(t, err)
	require.Len(t, recorder.requests, 1)
	require.Equal(t, run.TenantID, recorder.requests[0].TenantID)
	require.Equal(t, run.ID, recorder.requests[0].DigitalEmployeeRunID)
	require.NotEqual(t, uuid.Nil, recorder.requests[0].ProviderSessionEventID)
}
```

- [ ] **Step 2: Run the test and verify failure**

Run:

```bash
go test ./apps/control-plane/internal/employee -run TestWritebackEventRecordsProviderLedgerBestEffort -count=1
```

Expected: FAIL because `NewDigitalEmployeeRunWritebackService` does not accept a ledger recorder and provider event creation returns no event ID.

- [ ] **Step 3: Return provider session event ID from repository**

Change `DigitalEmployeeRunRepository` in `run_repository.go`:

```go
	CreateProviderSessionEventIfAbsent(ctx context.Context, req CreateProviderSessionEventRecordRequest) (uuid.UUID, error)
```

Update `PgRunRepository.CreateProviderSessionEventIfAbsent`:

```go
	event, err := r.q.CreateProviderSessionEventIfAbsent(ctx, queries.CreateProviderSessionEventIfAbsentParams{
		EventType:           req.EventType,
		SequenceNumber:      req.SequenceNumber,
		Payload:             payload,
		RequestID:           textFromPtr(req.RequestID),
		CommandID:           textFromPtr(req.CommandID),
		RawEventRef:         textFromPtr(req.RawEventRef),
		LogRef:              textFromPtr(req.LogRef),
		SessionStatePatch:   sessionStatePatch,
		Metadata:            metadata,
		ProviderSessionUuid: req.ProviderSessionUUID,
		TenantID:            req.TenantID,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return event.ID, nil
```

Update fakes in `run_writeback_test.go` and `run_service_test.go` to return a stable UUID.

- [ ] **Step 4: Add optional ledger recorder to employee writeback service**

In `run_writeback.go`, add:

```go
type ExecutionLedgerRecorder interface {
	RecordProviderSessionEvent(ctx context.Context, req ProviderSessionEventLedgerRecordRequest) error
}

type ProviderSessionEventLedgerRecordRequest struct {
	TenantID                uuid.UUID
	DigitalEmployeeRunID    uuid.UUID
	ProviderSessionEventID  uuid.UUID
}
```

Change the service struct:

```go
type DigitalEmployeeRunWritebackService struct {
	repository            DigitalEmployeeRunRepository
	audit                 AuditLogger
	runtimeEventRecorders []RuntimeEventRecorder
	executionLedger       ExecutionLedgerRecorder
}
```

Change constructor signature:

```go
func NewDigitalEmployeeRunWritebackService(repository DigitalEmployeeRunRepository, audit AuditLogger, recorders []RuntimeEventRecorder, executionLedger ExecutionLedgerRecorder) (*DigitalEmployeeRunWritebackService, error)
```

Update existing call sites:

- In tests without a ledger recorder, pass `nil`.
- In `apps/control-plane/internal/app/app.go`, pass `[]employee.RuntimeEventRecorder{runtimeEventRecorderAdapter{runtimeService: runtimeService}}` and the project repository adapter from Step 5.

- [ ] **Step 5: Record provider event ledger after provider event insert**

Change `createProviderSessionEvent` to return the provider event ID:

```go
func (s *DigitalEmployeeRunWritebackService) createProviderSessionEvent(ctx context.Context, run *DigitalEmployeeRun, providerSessionUUID uuid.UUID, commandID, eventType string, sequenceNumber int32, payload map[string]any, rawEventRef, logRef *string, sessionStatePatch map[string]any, metadata map[string]any) (uuid.UUID, error)
```

After it returns in `RecordEvent`, call:

```go
providerEventID, err := s.createProviderSessionEvent(ctx, run, providerSessionUUID, commandID, eventType, event.SequenceNumber, event.Payload, event.RawEventRef, event.LogRef, event.SessionStatePatch, event.Metadata)
if err != nil {
	return err
}
if s.executionLedger != nil {
	_ = s.executionLedger.RecordProviderSessionEvent(ctx, ProviderSessionEventLedgerRecordRequest{
		TenantID:               run.TenantID,
		DigitalEmployeeRunID:   run.ID,
		ProviderSessionEventID: providerEventID,
	})
}
```

- [ ] **Step 6: Implement project-side provider event ledger recorder**

In `apps/control-plane/internal/project/pg_repository.go`, add:

```go
func (r *PgRepository) CreateProviderSessionEventLedgerEvent(ctx context.Context, tenantID, digitalEmployeeRunID, providerSessionEventID uuid.UUID) (ExecutionLedgerEvent, error) {
	row, err := r.q.CreateProviderSessionEventLedgerEvent(ctx, queries.CreateProviderSessionEventLedgerEventParams{
		TenantID:               tenantID,
		DigitalEmployeeRunID:   digitalEmployeeRunID,
		ProviderSessionEventID: providerSessionEventID,
	})
	if err != nil {
		return ExecutionLedgerEvent{}, err
	}
	return executionLedgerEventFromRecord(row)
}
```

In `app.go`, add a tiny adapter if direct interface types are in different packages:

```go
type providerEventLedgerRecorder struct {
	repository project.ProviderEventExecutionLedgerRepository
}

func (r providerEventLedgerRecorder) RecordProviderSessionEvent(ctx context.Context, req employee.ProviderSessionEventLedgerRecordRequest) error {
	_, err := r.repository.CreateProviderSessionEventLedgerEvent(ctx, req.TenantID, req.DigitalEmployeeRunID, req.ProviderSessionEventID)
	return err
}
```

- [ ] **Step 7: Run employee tests**

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestWritebackEvent' -count=1
go test ./apps/control-plane/internal/app -run Test -count=1
```

Expected: PASS for employee tests; app package compiles.

- [ ] **Step 8: Commit Task 4**

Run:

```bash
git add apps/control-plane/internal/employee apps/control-plane/internal/project/pg_repository.go apps/control-plane/internal/app/app.go
git commit -m "feat(employee): index provider events in execution ledger"
```

### Task 5: Execution Trace HTTP API And OpenAPI Contract

**Files:**
- Modify: `apps/control-plane/internal/project/handler.go`
- Modify: `apps/control-plane/internal/project/handler_test.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/api/project_routes_test.go`
- Modify: `contracts/control-plane/openapi.yaml`
- Modify generated: `apps/control-plane/internal/api/gen/control_plane.gen.go`
- Modify generated: `apps/control-plane/gen/control_plane.gen.go`

- [ ] **Step 1: Write failing handler test**

Add to `apps/control-plane/internal/project/handler_test.go`:

```go
func TestProjectHandlerListsExecutionTrace(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	attemptID := uuid.New()
	service := &handlerTestService{
		executionTrace: &ProjectExecutionTrace{
			ProjectID: projectID,
			Summary: ProjectExecutionTraceSummary{
				AttemptCount:       1,
				FailedAttemptCount: 0,
				ArtifactRefCount:   1,
				EvidenceRefCount:   1,
			},
			Attempts: []ProjectExecutionTraceAttempt{{
				ProjectTaskID: uuid.New(),
				AttemptID:     attemptID,
				AttemptNo:     1,
				Status:        ProjectTaskAttemptStatusSucceeded,
				Events: []ExecutionLedgerEvent{{
					ID:         uuid.New(),
					TenantID:   tenantID,
					ProjectID:  projectID,
					EventType:  ExecutionLedgerEventAttemptCompleted,
					SourceType: "project_task_attempt",
					SourceID:   attemptID.String(),
					ActorType:  "digital_employee",
					OccurredAt: time.Now().UTC(),
					CreatedAt:  time.Now().UTC(),
				}},
			}},
		},
	}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodGet, "/projects/"+projectID.String()+"/execution-trace?limit=20", nil)
	req = req.WithContext(middleware.WithConsoleIdentity(req.Context(), middleware.ConsoleIdentity{TenantID: tenantID, UserID: uuid.New()}))
	req = withProjectRouteParams(req, projectID)
	rec := httptest.NewRecorder()

	handler.GetExecutionTrace(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body executionTraceResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, projectID.String(), body.ProjectID)
	require.Len(t, body.Attempts, 1)
	require.Equal(t, attemptID.String(), body.Attempts[0].AttemptID)
	require.Equal(t, int32(1), service.executionTraceReq.Limit)
}
```

- [ ] **Step 2: Run handler test and verify failure**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestProjectHandlerListsExecutionTrace -count=1
```

Expected: FAIL because handler response types and method are missing.

- [ ] **Step 3: Add handler service method and response mapping**

In `HandlerService`, add:

```go
	GetExecutionTrace(ctx context.Context, req GetExecutionTraceRequest) (*ProjectExecutionTrace, error)
```

Add handler:

```go
func (h *HTTPHandler) GetExecutionTrace(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	limit, offset, ok := paginationFromRequest(w, r)
	if !ok {
		return
	}
	req := GetExecutionTraceRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     limit,
		Offset:    offset,
	}
	if raw := r.URL.Query().Get("project_task_id"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			writeHandlerError(w, ErrInvalidProject)
			return
		}
		req.ProjectTaskID = &parsed
	}
	if raw := r.URL.Query().Get("attempt_id"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			writeHandlerError(w, ErrInvalidProject)
			return
		}
		req.ProjectTaskAttemptID = &parsed
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("event_type")); raw != "" {
		req.EventType = &raw
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("error_family")); raw != "" {
		req.ErrorFamily = &raw
	}
	trace, err := service.GetExecutionTrace(r.Context(), req)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, executionTraceResponseFromDomain(*trace))
}
```

Add response structs near existing project response types:

```go
type executionTraceResponse struct {
	ProjectID string                          `json:"project_id"`
	Summary   executionTraceSummaryResponse   `json:"summary"`
	Attempts  []executionTraceAttemptResponse `json:"attempts"`
}

type executionTraceSummaryResponse struct {
	AttemptCount             int32   `json:"attempt_count"`
	FailedAttemptCount       int32   `json:"failed_attempt_count"`
	HumanReviewRequiredCount int32   `json:"human_review_required_count"`
	ArtifactRefCount         int32   `json:"artifact_ref_count"`
	EvidenceRefCount         int32   `json:"evidence_ref_count"`
	LatestErrorFamily        *string `json:"latest_error_family,omitempty"`
}
```

Add attempt and event response mappings with the same JSON keys from the design spec.

- [ ] **Step 4: Register route**

In `apps/control-plane/internal/api/server.go`, add next to `execution-summaries`:

```go
r.Get("/projects/{projectId}/execution-trace", s.projectHandler.GetExecutionTrace)
```

- [ ] **Step 5: Add API route test**

Add to `apps/control-plane/internal/api/project_routes_test.go`:

```go
func TestExecutionTraceRouteUsesConsoleTenantAndProjectResource(t *testing.T) {
	server := NewServer()
	service := &routeProjectService{}
	server.SetProjectHandler(project.NewHandler(service))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+service.ensureProjectID().String()+"/execution-trace", nil)
	req = withConsoleAuth(req, service.tenantID, uuid.New())
	rec := httptest.NewRecorder()

	server.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, service.tenantID, service.executionTraceReq.TenantID)
	require.Equal(t, service.ensureProjectID(), service.executionTraceReq.ProjectID)
}
```

- [ ] **Step 6: Update OpenAPI**

Add path to `contracts/control-plane/openapi.yaml`:

```yaml
  /api/v1/projects/{projectId}/execution-trace:
    get:
      operationId: getProjectExecutionTrace
      parameters:
        - $ref: "#/components/parameters/ProjectId"
        - name: project_task_id
          in: query
          schema:
            type: string
            format: uuid
        - name: attempt_id
          in: query
          schema:
            type: string
            format: uuid
        - name: event_type
          in: query
          schema:
            type: string
        - name: error_family
          in: query
          schema:
            type: string
        - $ref: "#/components/parameters/Limit"
        - $ref: "#/components/parameters/Offset"
      responses:
        "200":
          description: Project execution trace
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ProjectExecutionTrace"
```

Add schemas `ProjectExecutionTrace`, `ProjectExecutionTraceSummary`, `ProjectExecutionTraceAttempt`, `ExecutionLedgerEvent`, and `ProjectExecutionTraceAttemptSummary` matching handler JSON.

- [ ] **Step 7: Generate API and run contract verification**

Run:

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/api -run 'ExecutionTrace|ProjectHandlerListsExecutionTrace' -count=1
```

Expected: generated files change; contract verification PASS; targeted Go tests PASS.

- [ ] **Step 8: Commit Task 5**

Run:

```bash
git add contracts/control-plane/openapi.yaml apps/control-plane/internal/api/gen apps/control-plane/gen apps/control-plane/internal/project/handler.go apps/control-plane/internal/project/handler_test.go apps/control-plane/internal/api/server.go apps/control-plane/internal/api/project_routes_test.go
git commit -m "feat(api): expose project execution trace"
```

### Task 6: Web API Client And Execution Trace Panel

**Files:**
- Modify: `apps/web/src/lib/api/projects.ts`
- Create: `apps/web/src/features/projects/components/project-execution-trace-panel.tsx`
- Create: `apps/web/src/features/projects/components/project-execution-trace-panel.test.tsx`
- Modify: `apps/web/src/features/projects/components/project-operational-detail.tsx`
- Modify: `apps/web/src/features/projects/index.tsx`

- [ ] **Step 1: Add Web API types and client**

In `apps/web/src/lib/api/projects.ts`, add:

```ts
export type ExecutionLedgerEvent = {
  id: string;
  event_type: string;
  source_type: string;
  source_id: string;
  actor_type: string;
  actor_id?: string;
  runtime_node_id?: string;
  provider_type?: string;
  provider_session_id?: string;
  input_summary?: string;
  output_summary?: string;
  error_family?: string;
  error_code?: string;
  error_message?: string;
  retryable?: boolean;
  artifact_refs: unknown[];
  evidence_refs: unknown[];
  metadata: Record<string, unknown>;
  occurred_at: string;
  created_at?: string;
};

export type ProjectExecutionTraceAttemptSummary = {
  execution_summary_id: string;
  conclusion: string;
  requires_human_review: boolean;
  artifact_refs: unknown[];
  evidence_refs: unknown[];
  created_at?: string;
};

export type ProjectExecutionTraceAttempt = {
  project_task_id: string;
  attempt_id: string;
  attempt_no: number;
  status: string;
  runtime_node_id?: string;
  provider_type?: string;
  provider_session_id?: string;
  started_at?: string;
  finished_at?: string;
  failure_family?: string;
  retryable?: boolean;
  events: ExecutionLedgerEvent[];
  summary?: ProjectExecutionTraceAttemptSummary;
};

export type ProjectExecutionTrace = {
  project_id: string;
  summary: {
    attempt_count: number;
    failed_attempt_count: number;
    human_review_required_count: number;
    artifact_ref_count: number;
    evidence_ref_count: number;
    latest_error_family?: string;
  };
  attempts: ProjectExecutionTraceAttempt[];
};

export function getProjectExecutionTrace(
  options: ApiClientOptions,
  projectId: string,
  filters: PaginationFilters = {},
): Promise<ProjectExecutionTrace> {
  return getJson<ProjectExecutionTrace>(
    options,
    projectPath(projectId, `/execution-trace${paginationQuery(filters)}`),
    "project execution trace",
  );
}
```

- [ ] **Step 2: Write failing component test**

Create `apps/web/src/features/projects/components/project-execution-trace-panel.test.tsx`:

```tsx
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ProjectExecutionTracePanel } from "./project-execution-trace-panel";
import type { ProjectExecutionTrace } from "@/lib/api/projects";

describe("ProjectExecutionTracePanel", () => {
  it("renders attempt timeline with summaries and error families", () => {
    const trace: ProjectExecutionTrace = {
      project_id: "project-1",
      summary: {
        attempt_count: 1,
        failed_attempt_count: 1,
        human_review_required_count: 0,
        artifact_ref_count: 1,
        evidence_ref_count: 1,
        latest_error_family: "provider_error",
      },
      attempts: [
        {
          project_task_id: "task-1",
          attempt_id: "attempt-1",
          attempt_no: 1,
          status: "failed",
          runtime_node_id: "runtime-1",
          provider_type: "codex",
          provider_session_id: "codex-session-1",
          failure_family: "provider_error",
          retryable: true,
          events: [
            {
              id: "event-1",
              event_type: "provider.event",
              source_type: "provider_session_event",
              source_id: "provider-event-1",
              actor_type: "provider",
              input_summary: "读取项目上下文",
              output_summary: "Provider 返回错误",
              error_family: "provider_error",
              artifact_refs: [],
              evidence_refs: [{ ref: "evidence://1" }],
              metadata: {},
              occurred_at: "2026-06-21T10:00:00Z",
            },
          ],
        },
      ],
    };

    render(<ProjectExecutionTracePanel trace={trace} />);

    expect(screen.getByText("执行证据链")).toBeInTheDocument();
    expect(screen.getByText("1 次")).toBeInTheDocument();
    expect(screen.getByText("provider_error")).toBeInTheDocument();
    const attempt = screen.getByLabelText("执行尝试 1");
    expect(within(attempt).getByText("failed")).toBeInTheDocument();
    expect(within(attempt).getByText("读取项目上下文")).toBeInTheDocument();
    expect(within(attempt).getByText("Provider 返回错误")).toBeInTheDocument();
  });
});
```

- [ ] **Step 3: Run component test and verify failure**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- project-execution-trace-panel.test.tsx
```

Expected: FAIL because the component does not exist.

- [ ] **Step 4: Implement trace panel**

Create `apps/web/src/features/projects/components/project-execution-trace-panel.tsx`:

```tsx
import { Activity, AlertTriangle, Boxes, CheckCircle2, FileCheck2 } from "lucide-react";
import {
  LiquidCard,
  SemanticIconTile,
  StatusBadge,
  type Tone,
} from "@/components/superteam";
import type {
  ExecutionLedgerEvent,
  ProjectExecutionTrace,
  ProjectExecutionTraceAttempt,
} from "@/lib/api/projects";

export function ProjectExecutionTracePanel({
  trace,
}: {
  trace?: ProjectExecutionTrace;
}) {
  if (!trace || trace.attempts.length === 0) {
    return (
      <LiquidCard className="rounded-xl">
        <PanelTitle count="0 次" />
        <div className="p-4 text-sm text-muted-foreground">暂无执行证据链</div>
      </LiquidCard>
    );
  }

  return (
    <LiquidCard className="rounded-xl">
      <PanelTitle count={`${trace.summary.attempt_count} 次`} />
      <div className="grid gap-3 border-b p-4 md:grid-cols-4">
        <Metric label="失败尝试" value={`${trace.summary.failed_attempt_count}`} tone="danger" />
        <Metric label="需人工复核" value={`${trace.summary.human_review_required_count}`} tone="warning" />
        <Metric label="工件" value={`${trace.summary.artifact_ref_count}`} tone="artifact" />
        <Metric label="证据" value={`${trace.summary.evidence_ref_count}`} tone="success" />
      </div>
      {trace.summary.latest_error_family ? (
        <div className="border-b px-4 py-3 text-xs text-muted-foreground">
          最近错误分类：<span className="font-medium text-foreground">{trace.summary.latest_error_family}</span>
        </div>
      ) : null}
      <div className="divide-y">
        {trace.attempts.map((attempt) => (
          <AttemptBlock attempt={attempt} key={attempt.attempt_id} />
        ))}
      </div>
    </LiquidCard>
  );
}

function PanelTitle({ count }: { count: string }) {
  return (
    <div className="flex items-center justify-between gap-3 border-b p-4">
      <div className="flex min-w-0 items-center gap-2">
        <SemanticIconTile tone="primary" size="sm">
          <Activity />
        </SemanticIconTile>
        <div className="min-w-0">
          <h3 className="text-sm font-semibold tracking-normal">执行证据链</h3>
          <p className="text-xs text-muted-foreground">Attempt、Provider、工具和证据事件</p>
        </div>
      </div>
      <StatusBadge tone="neutral">{count}</StatusBadge>
    </div>
  );
}

function Metric({ label, value, tone }: { label: string; value: string; tone: Tone }) {
  return (
    <div className="grid gap-1">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className="text-sm font-semibold">{value}</span>
    </div>
  );
}

function AttemptBlock({ attempt }: { attempt: ProjectExecutionTraceAttempt }) {
  return (
    <section aria-label={`执行尝试 ${attempt.attempt_no}`} className="grid gap-3 p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <StatusBadge tone={attemptTone(attempt.status)}>{attempt.status}</StatusBadge>
          <span className="text-sm font-medium">Attempt {attempt.attempt_no}</span>
          {attempt.failure_family ? (
            <StatusBadge tone="danger">{attempt.failure_family}</StatusBadge>
          ) : null}
          {attempt.retryable ? <StatusBadge tone="warning">可重试</StatusBadge> : null}
        </div>
        <div className="min-w-0 truncate text-xs text-muted-foreground">
          {attempt.provider_type ?? "provider"} · {attempt.provider_session_id ?? "no session"}
        </div>
      </div>
      {attempt.summary ? (
        <div className="rounded-md border p-3 text-sm">
          <div className="flex items-center gap-2">
            <CheckCircle2 className="size-4 text-success" />
            <span className="font-medium">{attempt.summary.conclusion}</span>
          </div>
        </div>
      ) : null}
      <div className="grid gap-2">
        {attempt.events.map((event) => (
          <TraceEventRow event={event} key={event.id} />
        ))}
      </div>
    </section>
  );
}

function TraceEventRow({ event }: { event: ExecutionLedgerEvent }) {
  return (
    <div className="grid gap-2 rounded-md border p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          {event.error_family ? (
            <AlertTriangle className="size-4 text-destructive" />
          ) : (
            <Boxes className="size-4 text-muted-foreground" />
          )}
          <span className="text-sm font-medium">{event.event_type}</span>
        </div>
        <span className="truncate text-xs text-muted-foreground">{event.source_type}</span>
      </div>
      {event.input_summary ? (
        <p className="text-xs text-muted-foreground">输入：{event.input_summary}</p>
      ) : null}
      {event.output_summary ? (
        <p className="text-xs text-muted-foreground">输出：{event.output_summary}</p>
      ) : null}
      <div className="flex flex-wrap gap-2">
        {event.error_family ? <StatusBadge tone="danger">{event.error_family}</StatusBadge> : null}
        {event.artifact_refs.length > 0 ? (
          <StatusBadge tone="artifact">
            <FileCheck2 className="size-3.5" />
            {`${event.artifact_refs.length} 工件`}
          </StatusBadge>
        ) : null}
        {event.evidence_refs.length > 0 ? (
          <StatusBadge tone="success">{`${event.evidence_refs.length} 证据`}</StatusBadge>
        ) : null}
      </div>
    </div>
  );
}

function attemptTone(status: string): Tone {
  switch (status) {
    case "succeeded":
    case "completed":
      return "success";
    case "failed":
    case "timed_out":
    case "lost":
      return "danger";
    case "waiting_human":
      return "warning";
    case "running":
    case "queued":
      return "info";
    default:
      return "neutral";
  }
}
```

- [ ] **Step 5: Wire Web query and props**

In `apps/web/src/features/projects/index.tsx` import `getProjectExecutionTrace`, add query:

```tsx
const executionTraceQuery = useQuery({
  enabled: Boolean(effectiveProjectId),
  queryKey: ["project-execution-trace", effectiveProjectId],
  queryFn: () =>
    getProjectExecutionTrace(apiOptions, effectiveProjectId as string, { limit: 200 }),
  placeholderData: keepPreviousData,
});
```

Filter:

```tsx
const projectExecutionTrace =
  executionTraceQuery.data?.project_id === effectiveProjectId
    ? executionTraceQuery.data
    : undefined;
```

Pass prop:

```tsx
executionTrace={projectExecutionTrace}
```

In `ProjectOperationalDetailProps`, add:

```tsx
executionTrace?: ProjectExecutionTrace;
```

Render near the existing execution summary panel:

```tsx
<ProjectExecutionTracePanel trace={executionTrace} />
```

- [ ] **Step 6: Run Web tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- project-execution-trace-panel.test.tsx
corepack pnpm --filter ./apps/web run typecheck
```

Expected: component test PASS; typecheck PASS.

- [ ] **Step 7: Commit Task 6**

Run:

```bash
git add apps/web/src/lib/api/projects.ts apps/web/src/features/projects/components/project-execution-trace-panel.tsx apps/web/src/features/projects/components/project-execution-trace-panel.test.tsx apps/web/src/features/projects/components/project-operational-detail.tsx apps/web/src/features/projects/index.tsx
git commit -m "feat(web): show project execution trace"
```

### Task 7: Full Local Verification And Real-Chain Smoke

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add changelog entry**

Run:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Add a `CHANGELOG.md` entry using that exact timestamp:

```markdown
- [YYYY-MM-DD HH:MM] Added project execution ledger and execution trace read model for task attempts, Provider events, evidence refs, and Web project detail review.
```

Replace `YYYY-MM-DD HH:MM` with the command output.

- [ ] **Step 2: Run targeted backend tests**

Run:

```bash
go test ./apps/control-plane/internal/storage ./apps/control-plane/internal/storage/queries ./apps/control-plane/internal/project ./apps/control-plane/internal/employee ./apps/control-plane/internal/api -count=1
```

Expected: PASS.

- [ ] **Step 3: Run contract and Web gates**

Run:

```bash
corepack pnpm verify:contracts
corepack pnpm --filter ./apps/web run test
corepack pnpm --filter ./apps/web run typecheck
```

Expected: PASS.

- [ ] **Step 4: Run migration status against the development database**

Run:

```bash
scripts/dev-services.sh status
# config.yaml stores the DSN under the nested key `postgres.url` (inside the
# `postgres:` block), NOT a top-level `database_url`. Extract it explicitly so
# the Makefile's `$(DATABASE_URL)` resolves to the real development DSN.
export DATABASE_URL="$(awk '/^postgres:/{f=1;next} f&&/^[[:space:]]*url:/{sub(/^[[:space:]]*url:[[:space:]]*/,""); gsub(/"/,""); print; exit}' apps/control-plane/config/config.yaml)"
test -n "$DATABASE_URL" || { echo "ERROR: could not extract postgres.url from config.yaml" >&2; exit 1; }
make -C apps/control-plane migrate-status
```

Expected: service status output shows Control Plane database configuration is available; Atlas reports pending migration state or clean state without connection errors.

- [ ] **Step 5: Apply migration only to the intended development database**

Run:

```bash
export DATABASE_URL="$(awk '/^postgres:/{f=1;next} f&&/^[[:space:]]*url:/{sub(/^[[:space:]]*url:[[:space:]]*/,""); gsub(/"/,""); print; exit}' apps/control-plane/config/config.yaml)"
test -n "$DATABASE_URL" || { echo "ERROR: could not extract postgres.url from config.yaml" >&2; exit 1; }
make -C apps/control-plane migrate-up
```

Expected: migration applies successfully. If the database URL cannot be confirmed as the intended development database, stop and ask for confirmation.

- [ ] **Step 6: Restart services that must load current code**

Run:

```bash
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh restart web
scripts/dev-services.sh status
```

Expected: Control Plane and Web are running with current code. Runtime Agent may need restart only if the smoke includes a real Runtime project-task writeback.

- [ ] **Step 7: API smoke for execution trace route**

Use the repo's existing authenticated curl pattern. If a token is already configured in the developer environment, run:

```bash
curl -sS -H "Authorization: Bearer ${SUPERTEAM_DEV_TOKEN}" \
  "http://127.0.0.1:8080/api/v1/projects/${SUPERTEAM_DEV_PROJECT_ID}/execution-trace?limit=20" | jq .
```

Expected: HTTP 200 and JSON with `project_id`, `summary`, and `attempts`. If `SUPERTEAM_DEV_TOKEN` or `SUPERTEAM_DEV_PROJECT_ID` is not available, record the missing auth/project dependency and do not claim real API verification.

- [ ] **Step 8: Real project-task smoke**

Run or reuse a real project task that reaches Runtime writeback. Confirm:

```sql
SELECT event_type, source_type, project_task_attempt_id, error_family, occurred_at
FROM execution_ledger_events
WHERE tenant_id = '<tenant uuid>'
  AND project_id = '<project uuid>'
ORDER BY occurred_at DESC
LIMIT 20;
```

Expected: at least `attempt.started` plus one terminal event such as `attempt.completed`, `attempt.failed`, or `attempt.waiting_human`; completed paths include `summary.created`.

- [ ] **Step 9: Browser smoke**

Open the real Web project detail page and inspect the execution trace panel:

```bash
open "http://127.0.0.1:5173/projects/${SUPERTEAM_DEV_PROJECT_ID}"
```

Expected: the page loads from the running Web, the execution trace panel is visible, and the data matches the API response. If browser auth is missing, record the blocker.

- [ ] **Step 10: Diff hygiene**

Run:

```bash
git diff --check
git status --short
```

Expected: no whitespace errors. Only intended files are modified.

- [ ] **Step 11: Commit verification/changelog**

Run:

```bash
git add CHANGELOG.md
git commit -m "chore: document execution trace delivery"
```

### Task 8: Final Completion Check

**Files:**
- No code files changed in this task unless verification reveals a defect.

- [ ] **Step 1: Run SuperTeam completion skill**

Read and follow `.codex/skills/superteam-completion-check/SKILL.md`.

- [ ] **Step 2: Summarize verification evidence**

Prepare final report in this shape:

```text
现象：项目验收和审批只能看到最终 conclusion，缺 attempt/provider/tool/capability 证据链。
证据：列出新增表、API、Web 面板、关键 tests、真实链路 API/browser/DB 结果。
判断：Execution Ledger 现在作为统一执行事实索引，项目详情读取 execution trace projection。
改动：列出 schema、Control Plane、Provider event indexing、OpenAPI、Web 面板。
验证结果：列出每条命令和真实链路结果；如果真实链路缺 auth/provider/runtime，标记阻塞。
```

- [ ] **Step 3: Do not claim real-chain completion without evidence**

If Step 7 real API, DB, Runtime, or browser smoke was blocked, final answer must say:

```text
阻塞：真实链路验证缺少 <具体依赖>；尚不能声明功能可用。
```
