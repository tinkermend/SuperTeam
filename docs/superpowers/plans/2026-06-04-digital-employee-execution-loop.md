# Digital Employee Execution Loop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 打通数字员工从创建预置、发起 run、Runtime Agent 执行、Provider 事件回写、Web 详情页观测到停止/超时的完整闭环。

**Architecture:** Control Plane 新增 `DigitalEmployeeRunService`，复用 `tasks/task_runs/task_events` 作为执行主线，同时投影到 `provider_sessions/provider_session_events`。Runtime WebSocket 只负责命令下发，Runtime Agent 通过 Runtime-auth HTTP 回写命令事件、终态、日志引用和 Provider session 状态。Web 新增 `/employees/$employeeId` 详情页，所有状态只读取 Control Plane 持久化事实。

**Tech Stack:** Go + chi/net/http + pgx + sqlc + Atlas、Rust + Tokio + reqwest、React + TanStack Router + TanStack Query + shadcn/ui + lucide-react、Vitest、Go test、cargo test。

---

## Scope Guard

- 已存在的 Runtime command execution layer 是本计划的基础：`apps/runtime-agent/src/commands/*`、`apps/runtime-agent/src/controlplane/ws.rs`、`docs/superpowers/plans/2026-06-04-runtime-command-execution-layer.md`。
- 本计划只扩展 Runtime command payload、Runtime HTTP writeback 和 Control Plane/Web 闭环，不重新实现 Claude/OpenCode provider adapter。
- 不定义 Runtime 本地数字员工目录结构，只实现 `provision_instance` 的抽象 payload、目录存在性和治理资产接收/同步结果。
- 创建数字员工必须等待 Runtime HTTP 回写 `provision_instance` 成功；如果超时或失败，Control Plane 需要清理平台侧 employee、execution instance 和 command receipt，使 Web 不看到半成品。

## File Structure

- Create: `apps/control-plane/internal/storage/migrations/006_digital_employee_run_loop.sql`
  - 新增 command receipt 表、run 诊断字段、日志引用、幂等索引、Provider session state 字段。
- Modify: `apps/control-plane/internal/storage/queries/tasks.sql`
  - 增加数字员工 run 创建、查询、状态更新、事件幂等写入查询。
- Modify: `apps/control-plane/internal/storage/queries/provider_session.sql`
  - 增加 Provider session upsert、session state 更新、事件幂等写入查询。
- Create: `apps/control-plane/internal/storage/queries/runtime_command.sql`
  - command receipt 创建、状态更新、按 command_id 查找和 provisioning 等待轮询查询。
- Modify generated: `apps/control-plane/internal/storage/queries/*.sql.go`、`models.go`、`querier.go`
  - 由 `sqlc generate` 生成。
- Create: `apps/control-plane/internal/employee/run_types.go`
  - run 状态、request/response、writeback payload、诊断、work product domain 类型。
- Create: `apps/control-plane/internal/employee/run_repository.go`
  - `DigitalEmployeeRunRepository` 接口，隔离 sqlc 细节。
- Create: `apps/control-plane/internal/employee/pg_run_repository.go`
  - run repository 的 pgx/sqlc 实现。
- Create: `apps/control-plane/internal/employee/run_service.go`
  - `DigitalEmployeeRunService`，负责 preflight、admission、command 生成、dispatch、stop 和状态机。
- Create: `apps/control-plane/internal/employee/run_writeback.go`
  - Runtime HTTP 回写处理，负责 command 校验、双投影、终态和 provisioning 完成。
- Create: `apps/control-plane/internal/employee/run_handler.go`
  - Web 用户接口：run 创建、列表、详情、事件、停止。
- Modify: `apps/control-plane/internal/employee/handler.go`
  - 注入 run handler service，保留现有员工接口。
- Modify: `apps/control-plane/internal/employee/service.go`
  - 创建数字员工升级为 Runtime provisioning flow。
- Modify: `apps/control-plane/internal/runtime/connection.go`
  - 增加 `IsConnected(nodeID string) bool`，供 create/run preflight 使用。
- Modify: `apps/control-plane/internal/authz/types.go`
  - 增加 `employee.run.create`、`employee.run.stop`、`employee.run.log.read`。
- Create: `apps/control-plane/internal/api/handlers/runtime_command_writeback.go`
  - Runtime-auth HTTP command writeback handler。
- Modify: `apps/control-plane/internal/api/server.go`
  - 注册 console run routes 与 runtime command writeback routes。
- Modify: `apps/control-plane/internal/app/app.go`
  - 装配 run repository、run service、writeback handler，并注入 audit service。
- Modify: `contracts/control-plane/openapi.yaml`
  - 增加 Web run API 和 Runtime command writeback API。
- Modify: `apps/runtime-agent/src/controlplane/models.rs`
  - 增加 writeback request/response、`provision_instance` command type。
- Modify: `apps/runtime-agent/src/controlplane/client.rs`
  - 增加 command events/terminal/provisioning writeback methods。
- Modify: `apps/runtime-agent/src/controlplane/ws.rs`
  - 创建 executor 时注入 writeback client。
- Modify: `apps/runtime-agent/src/commands/payload.rs`
  - 扩展 `provider-run/v1` 字段：`objective`、`output_schema`、`allowed_actions`、`forbidden_actions`、`secret_refs`、`timeout_sec`、`grace_sec`。
- Modify: `apps/runtime-agent/src/commands/executor.rs`
  - 在 provider event drain 中回写 Control Plane；`provision_instance` 成功/失败也回写。
- Modify: `apps/runtime-agent/src/runs.rs`
  - 增加 `TimedOut` 状态、日志引用和命令上下文字段。
- Modify: `apps/web/src/lib/api/employees.ts`
  - 增加 run API client 类型与函数。
- Modify: `apps/web/src/features/employees/index.tsx`
  - 列表行增加详情入口。
- Create: `apps/web/src/features/employees/detail.tsx`
  - 数字员工详情页。
- Create: `apps/web/src/routes/_authenticated/employees/$employeeId.tsx`
  - TanStack Router 动态路由。
- Modify generated: `apps/web/src/routeTree.gen.ts`
  - 由 TanStack Router/Vite 生成或由 `pnpm --filter @superteam/web build` 更新。
- Modify: `CHANGELOG.md`
  - 记录实施完成时间与变更摘要。

---

### Task 1: Database Migration And sqlc Queries

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/006_digital_employee_run_loop.sql`
- Modify: `apps/control-plane/internal/storage/queries/tasks.sql`
- Modify: `apps/control-plane/internal/storage/queries/provider_session.sql`
- Create: `apps/control-plane/internal/storage/queries/runtime_command.sql`
- Modify generated: `apps/control-plane/internal/storage/queries/*.sql.go`
- Test: `apps/control-plane/internal/storage/migrations_test.go`
- Test: `apps/control-plane/internal/storage/queries/queries_test.go`

- [ ] **Step 1: Check migration number collision**

Run:

```bash
ls apps/control-plane/internal/storage/migrations/*.sql
```

Expected: latest migration is `005_add_auth_user_avatar.sql`. If a newer migration exists in the current branch, rename this task's migration file to the next available number and use that new path consistently in the remaining steps.

- [ ] **Step 2: Write migration**

Create `apps/control-plane/internal/storage/migrations/006_digital_employee_run_loop.sql`:

```sql
CREATE TABLE runtime_command_receipts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid,
    command_id VARCHAR(255) NOT NULL,
    command_type VARCHAR(100) NOT NULL,
    runtime_node_id UUID NOT NULL,
    node_id VARCHAR(255) NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    resource_id UUID NOT NULL,
    status VARCHAR(50) NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    result JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_message TEXT,
    dispatched_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, command_id)
);

ALTER TABLE task_runs
    ADD COLUMN command_id VARCHAR(255),
    ADD COLUMN digital_employee_id UUID,
    ADD COLUMN execution_instance_id UUID,
    ADD COLUMN idempotency_key VARCHAR(255),
    ADD COLUMN timeout_sec INTEGER,
    ADD COLUMN grace_sec INTEGER,
    ADD COLUMN diagnostic JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN log_ref TEXT,
    ADD COLUMN raw_result_ref TEXT,
    ADD COLUMN work_products JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN session_state JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN error_code VARCHAR(100),
    ADD COLUMN error_family VARCHAR(100),
    ADD COLUMN exit_code INTEGER,
    ADD COLUMN signal VARCHAR(100),
    ADD COLUMN timed_out BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN provider_session_external_id VARCHAR(255);

ALTER TABLE task_events
    ADD COLUMN command_id VARCHAR(255),
    ADD COLUMN raw_event_ref TEXT,
    ADD COLUMN log_ref TEXT,
    ADD COLUMN metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE provider_sessions
    ADD COLUMN session_display_id VARCHAR(255),
    ADD COLUMN session_params JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN session_state JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN last_sequence_number INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN last_command_id VARCHAR(255),
    ADD COLUMN last_run_id UUID,
    ADD COLUMN last_error_family VARCHAR(100),
    ADD COLUMN last_runtime_seen_at TIMESTAMPTZ;

ALTER TABLE provider_session_events
    ADD COLUMN log_ref TEXT,
    ADD COLUMN session_state_patch JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE UNIQUE INDEX uq_task_runs_command_id
    ON task_runs(tenant_id, command_id)
    WHERE command_id IS NOT NULL;

CREATE UNIQUE INDEX uq_task_runs_employee_idempotency
    ON task_runs(tenant_id, digital_employee_id, idempotency_key)
    WHERE digital_employee_id IS NOT NULL AND idempotency_key IS NOT NULL;

CREATE INDEX idx_task_runs_employee_status
    ON task_runs(tenant_id, digital_employee_id, status, created_at DESC)
    WHERE digital_employee_id IS NOT NULL;

CREATE UNIQUE INDEX uq_task_events_run_sequence
    ON task_events(tenant_id, run_id, sequence_number)
    WHERE run_id IS NOT NULL;

CREATE UNIQUE INDEX uq_provider_session_events_command_sequence
    ON provider_session_events(tenant_id, command_id, sequence_number)
    WHERE command_id IS NOT NULL;

CREATE INDEX idx_runtime_command_receipts_resource
    ON runtime_command_receipts(tenant_id, resource_type, resource_id, created_at DESC);

CREATE TRIGGER update_runtime_command_receipts_updated_at BEFORE UPDATE ON runtime_command_receipts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE runtime_command_receipts IS 'Runtime command dispatch and HTTP writeback receipts';
COMMENT ON COLUMN task_runs.command_id IS 'Control Plane generated runtime command ID';
COMMENT ON COLUMN task_runs.digital_employee_id IS 'Digital employee that owns this run';
COMMENT ON COLUMN task_runs.execution_instance_id IS 'Execution instance used by this run';
COMMENT ON COLUMN task_runs.work_products IS 'Structured run outputs indexed for Web and workflow consumption';
COMMENT ON COLUMN provider_sessions.session_state IS 'Adapter-defined recoverable provider session state';
```

- [ ] **Step 3: Add sqlc query file for command receipts**

Create `apps/control-plane/internal/storage/queries/runtime_command.sql`:

```sql
-- name: CreateRuntimeCommandReceipt :one
INSERT INTO runtime_command_receipts (
    tenant_id,
    command_id,
    command_type,
    runtime_node_id,
    node_id,
    resource_type,
    resource_id,
    status,
    payload,
    dispatched_at
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('command_id')::varchar,
    sqlc.arg('command_type')::varchar,
    sqlc.arg('runtime_node_id')::uuid,
    sqlc.arg('node_id')::varchar,
    sqlc.arg('resource_type')::varchar,
    sqlc.arg('resource_id')::uuid,
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.arg('payload')::jsonb, '{}'::jsonb),
    sqlc.narg('dispatched_at')::timestamptz
) RETURNING *;

-- name: GetRuntimeCommandReceiptByCommandID :one
SELECT *
FROM runtime_command_receipts
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND command_id = sqlc.arg('command_id')::varchar;

-- name: GetRuntimeCommandReceiptByCommandIDForUpdate :one
SELECT *
FROM runtime_command_receipts
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND command_id = sqlc.arg('command_id')::varchar
FOR UPDATE;

-- name: UpdateRuntimeCommandReceiptStatus :one
UPDATE runtime_command_receipts
SET status = sqlc.arg('status')::varchar,
    result = COALESCE(sqlc.arg('result')::jsonb, result),
    error_message = sqlc.narg('error_message')::text,
    completed_at = CASE
        WHEN sqlc.arg('status')::varchar IN ('completed', 'failed', 'cancelled', 'timed_out') THEN COALESCE(completed_at, NOW())
        ELSE completed_at
    END,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND command_id = sqlc.arg('command_id')::varchar
RETURNING *;
```

- [ ] **Step 4: Extend task queries**

Append these queries to `apps/control-plane/internal/storage/queries/tasks.sql`:

```sql
-- name: CreateDigitalEmployeeTaskRun :one
WITH created_task AS (
    INSERT INTO tasks (
        tenant_id,
        team_id,
        title,
        description,
        status,
        priority,
        provider_type,
        creator_id,
        target_node_id,
        workspace_path,
        params,
        idempotency_key,
        risk_level
    ) VALUES (
        sqlc.arg('tenant_id')::uuid,
        sqlc.arg('team_id')::uuid,
        sqlc.arg('title')::varchar,
        sqlc.narg('description')::text,
        'pending',
        sqlc.arg('priority')::integer,
        sqlc.arg('provider_type')::varchar,
        sqlc.narg('creator_id')::uuid,
        sqlc.arg('target_node_id')::varchar,
        sqlc.narg('workspace_path')::text,
        COALESCE(sqlc.arg('params')::jsonb, '{}'::jsonb),
        sqlc.narg('idempotency_key')::varchar,
        COALESCE(sqlc.narg('risk_level')::varchar, 'normal')
    )
    RETURNING *
),
created_run AS (
    INSERT INTO task_runs (
        tenant_id,
        task_id,
        node_id,
        runtime_node_id,
        provider_session_id,
        status,
        command_id,
        digital_employee_id,
        execution_instance_id,
        idempotency_key,
        timeout_sec,
        grace_sec
    )
    SELECT
        created_task.tenant_id,
        created_task.id,
        sqlc.arg('node_id')::varchar,
        sqlc.arg('runtime_node_id')::uuid,
        sqlc.narg('provider_session_id')::varchar,
        sqlc.arg('run_status')::varchar,
        sqlc.arg('command_id')::varchar,
        sqlc.arg('digital_employee_id')::uuid,
        sqlc.arg('execution_instance_id')::uuid,
        sqlc.narg('idempotency_key')::varchar,
        sqlc.narg('timeout_sec')::integer,
        sqlc.narg('grace_sec')::integer
    FROM created_task
    RETURNING *
)
SELECT
    created_task.id AS task_id,
    created_run.id AS run_id,
    created_run.command_id,
    created_task.status AS task_status,
    created_run.status AS run_status
FROM created_task
JOIN created_run ON created_run.task_id = created_task.id;

-- name: GetActiveDigitalEmployeeRun :one
SELECT tr.*
FROM task_runs tr
JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND tr.status IN ('queued', 'dispatching', 'running', 'cancelling')
  AND t.deleted_at IS NULL
ORDER BY tr.created_at DESC
LIMIT 1;

-- name: GetDigitalEmployeeRun :one
SELECT tr.*
FROM task_runs tr
JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND tr.id = sqlc.arg('run_id')::uuid
  AND t.deleted_at IS NULL;

-- name: GetDigitalEmployeeRunByCommandID :one
SELECT tr.*
FROM task_runs tr
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.command_id = sqlc.arg('command_id')::varchar;

-- name: ListDigitalEmployeeRuns :many
SELECT tr.*
FROM task_runs tr
JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND t.deleted_at IS NULL
ORDER BY tr.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: UpdateDigitalEmployeeRunStatus :one
UPDATE task_runs
SET status = sqlc.arg('status')::varchar,
    result = COALESCE(sqlc.narg('result')::jsonb, result),
    error_message = sqlc.narg('error_message')::text,
    diagnostic = COALESCE(sqlc.narg('diagnostic')::jsonb, diagnostic),
    log_ref = COALESCE(sqlc.narg('log_ref')::text, log_ref),
    raw_result_ref = COALESCE(sqlc.narg('raw_result_ref')::text, raw_result_ref),
    work_products = COALESCE(sqlc.narg('work_products')::jsonb, work_products),
    session_state = COALESCE(sqlc.narg('session_state')::jsonb, session_state),
    error_code = COALESCE(sqlc.narg('error_code')::varchar, error_code),
    error_family = COALESCE(sqlc.narg('error_family')::varchar, error_family),
    exit_code = COALESCE(sqlc.narg('exit_code')::integer, exit_code),
    signal = COALESCE(sqlc.narg('signal')::varchar, signal),
    timed_out = CASE WHEN sqlc.arg('status')::varchar = 'timed_out' THEN true ELSE timed_out END,
    provider_session_external_id = COALESCE(sqlc.narg('provider_session_external_id')::varchar, provider_session_external_id),
    completed_at = CASE
        WHEN sqlc.arg('status')::varchar = 'completed' THEN COALESCE(completed_at, NOW())
        ELSE completed_at
    END,
    finished_at = CASE
        WHEN sqlc.arg('status')::varchar IN ('completed', 'failed', 'cancelled', 'timed_out') THEN COALESCE(finished_at, NOW())
        ELSE finished_at
    END,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('run_id')::uuid
RETURNING *;

-- name: CreateTaskEventIfAbsent :one
WITH inserted AS (
    INSERT INTO task_events (
        tenant_id,
        task_id,
        run_id,
        event_type,
        sequence_number,
        payload,
        command_id,
        raw_event_ref,
        log_ref,
        metadata
    ) VALUES (
        sqlc.arg('tenant_id')::uuid,
        sqlc.arg('task_id')::uuid,
        sqlc.arg('run_id')::uuid,
        sqlc.arg('event_type')::varchar,
        sqlc.arg('sequence_number')::integer,
        COALESCE(sqlc.arg('payload')::jsonb, '{}'::jsonb),
        sqlc.narg('command_id')::varchar,
        sqlc.narg('raw_event_ref')::text,
        sqlc.narg('log_ref')::text,
        COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
    )
    ON CONFLICT DO NOTHING
    RETURNING *
)
SELECT * FROM inserted
UNION ALL
SELECT *
FROM task_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND run_id = sqlc.arg('run_id')::uuid
  AND sequence_number = sqlc.arg('sequence_number')::integer
LIMIT 1;
```

- [ ] **Step 5: Extend Provider session queries**

Append these queries to `apps/control-plane/internal/storage/queries/provider_session.sql`:

```sql
-- name: UpsertProviderSessionByExternalID :one
INSERT INTO provider_sessions (
    tenant_id,
    provider_session_id,
    digital_employee_id,
    execution_instance_id,
    runtime_node_id,
    provider_type,
    status,
    recoverable,
    last_active_at,
    session_display_id,
    session_params,
    session_state,
    last_sequence_number,
    last_command_id,
    last_run_id,
    last_error_family,
    last_runtime_seen_at,
    metadata
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('provider_session_id')::varchar,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('execution_instance_id')::uuid,
    sqlc.arg('runtime_node_id')::uuid,
    sqlc.arg('provider_type')::varchar,
    sqlc.arg('status')::varchar,
    sqlc.arg('recoverable')::boolean,
    NOW(),
    sqlc.narg('session_display_id')::varchar,
    COALESCE(sqlc.arg('session_params')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('session_state')::jsonb, '{}'::jsonb),
    sqlc.arg('last_sequence_number')::integer,
    sqlc.narg('last_command_id')::varchar,
    sqlc.narg('last_run_id')::uuid,
    sqlc.narg('last_error_family')::varchar,
    NOW(),
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
)
ON CONFLICT (tenant_id, provider_type, provider_session_id) DO UPDATE SET
    status = EXCLUDED.status,
    last_active_at = NOW(),
    session_display_id = COALESCE(EXCLUDED.session_display_id, provider_sessions.session_display_id),
    session_params = EXCLUDED.session_params,
    session_state = EXCLUDED.session_state,
    last_sequence_number = GREATEST(provider_sessions.last_sequence_number, EXCLUDED.last_sequence_number),
    last_command_id = EXCLUDED.last_command_id,
    last_run_id = EXCLUDED.last_run_id,
    last_error_family = EXCLUDED.last_error_family,
    last_runtime_seen_at = NOW(),
    metadata = EXCLUDED.metadata,
    updated_at = NOW()
RETURNING *;

-- name: CreateProviderSessionEventIfAbsent :one
WITH inserted AS (
    INSERT INTO provider_session_events (
        tenant_id,
        provider_session_id,
        digital_employee_id,
        execution_instance_id,
        runtime_node_id,
        provider_type,
        event_type,
        sequence_number,
        payload,
        request_id,
        command_id,
        raw_event_ref,
        log_ref,
        session_state_patch,
        metadata
    ) SELECT
        ps.tenant_id,
        ps.id,
        ps.digital_employee_id,
        ps.execution_instance_id,
        ps.runtime_node_id,
        ps.provider_type,
        sqlc.arg('event_type')::varchar,
        sqlc.arg('sequence_number')::integer,
        COALESCE(sqlc.arg('payload')::jsonb, '{}'::jsonb),
        sqlc.narg('request_id')::varchar,
        sqlc.narg('command_id')::varchar,
        sqlc.narg('raw_event_ref')::text,
        sqlc.narg('log_ref')::text,
        COALESCE(sqlc.arg('session_state_patch')::jsonb, '{}'::jsonb),
        COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
    FROM provider_sessions ps
    WHERE ps.id = sqlc.arg('provider_session_uuid')::uuid
      AND ps.tenant_id = sqlc.arg('tenant_id')::uuid
    ON CONFLICT DO NOTHING
    RETURNING *
)
SELECT * FROM inserted
UNION ALL
SELECT *
FROM provider_session_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND command_id = sqlc.narg('command_id')::varchar
  AND sequence_number = sqlc.arg('sequence_number')::integer
LIMIT 1;
```

- [ ] **Step 6: Generate sqlc and run focused DB tests**

Run:

```bash
cd apps/control-plane && make generate-sqlc
go test ./internal/storage/... ./internal/employee/...
```

Expected: sqlc generation succeeds; existing storage and employee tests pass.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/storage/migrations/006_digital_employee_run_loop.sql apps/control-plane/internal/storage/queries apps/control-plane/internal/storage/migrations_test.go
git commit -m "feat(control-plane): add digital employee run persistence"
```

---

### Task 2: Run Domain And Repository

**Files:**
- Create: `apps/control-plane/internal/employee/run_types.go`
- Create: `apps/control-plane/internal/employee/run_repository.go`
- Create: `apps/control-plane/internal/employee/pg_run_repository.go`
- Test: `apps/control-plane/internal/employee/run_repository_test.go`

- [ ] **Step 1: Write repository tests**

Create `apps/control-plane/internal/employee/run_repository_test.go` with tests that use a fake sqlc boundary for mapper functions and a real repository integration case under the existing query test environment:

```go
func TestDigitalEmployeeRunStatusTerminal(t *testing.T) {
	require.True(t, DigitalEmployeeRunStatusCompleted.IsTerminal())
	require.True(t, DigitalEmployeeRunStatusFailed.IsTerminal())
	require.True(t, DigitalEmployeeRunStatusCancelled.IsTerminal())
	require.True(t, DigitalEmployeeRunStatusTimedOut.IsTerminal())
	require.False(t, DigitalEmployeeRunStatusRunning.IsTerminal())
	require.False(t, DigitalEmployeeRunStatusCancelling.IsTerminal())
}

func TestRuntimeWritebackEventRedactsSensitivePayload(t *testing.T) {
	event := RuntimeCommandEventWriteback{
		EventType:      "text_delta",
		SequenceNumber: 1,
		Payload: map[string]any{
			"text": "ok",
			"authorization": "Bearer secret",
			"nested": map[string]any{"token": "secret"},
		},
	}
	redacted := redactRuntimeEventPayload(event.Payload)
	require.Equal(t, "[redacted]", redacted["authorization"])
	require.Equal(t, "[redacted]", redacted["nested"].(map[string]any)["token"])
}
```

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestDigitalEmployeeRunStatusTerminal|TestRuntimeWritebackEventRedactsSensitivePayload' -count=1
```

Expected: FAIL because the new types and redaction function do not exist.

- [ ] **Step 2: Add run domain types**

Create `apps/control-plane/internal/employee/run_types.go`:

```go
package employee

import (
	"time"

	"github.com/google/uuid"
)

type DigitalEmployeeRunStatus string

const (
	DigitalEmployeeRunStatusQueued      DigitalEmployeeRunStatus = "queued"
	DigitalEmployeeRunStatusDispatching DigitalEmployeeRunStatus = "dispatching"
	DigitalEmployeeRunStatusRunning     DigitalEmployeeRunStatus = "running"
	DigitalEmployeeRunStatusCancelling  DigitalEmployeeRunStatus = "cancelling"
	DigitalEmployeeRunStatusCompleted   DigitalEmployeeRunStatus = "completed"
	DigitalEmployeeRunStatusFailed      DigitalEmployeeRunStatus = "failed"
	DigitalEmployeeRunStatusCancelled   DigitalEmployeeRunStatus = "cancelled"
	DigitalEmployeeRunStatusTimedOut    DigitalEmployeeRunStatus = "timed_out"
)

func (s DigitalEmployeeRunStatus) IsTerminal() bool {
	switch s {
	case DigitalEmployeeRunStatusCompleted, DigitalEmployeeRunStatusFailed, DigitalEmployeeRunStatusCancelled, DigitalEmployeeRunStatusTimedOut:
		return true
	default:
		return false
	}
}

func (s DigitalEmployeeRunStatus) IsActive() bool {
	switch s {
	case DigitalEmployeeRunStatusQueued, DigitalEmployeeRunStatusDispatching, DigitalEmployeeRunStatusRunning, DigitalEmployeeRunStatusCancelling:
		return true
	default:
		return false
	}
}

type WorkProduct struct {
	Type      string         `json:"type"`
	Title     string         `json:"title"`
	Summary   string         `json:"summary"`
	Ref       string         `json:"ref"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
}

type DigitalEmployeeRun struct {
	ID                        uuid.UUID
	TenantID                  uuid.UUID
	TaskID                    uuid.UUID
	DigitalEmployeeID         uuid.UUID
	ExecutionInstanceID       uuid.UUID
	RuntimeNodeID             uuid.UUID
	NodeID                    string
	CommandID                 string
	ProviderType              string
	Status                    DigitalEmployeeRunStatus
	ProviderSessionExternalID *string
	Result                    map[string]any
	Diagnostic                map[string]any
	LogRef                    *string
	RawResultRef              *string
	WorkProducts              []WorkProduct
	ErrorMessage              *string
	ErrorCode                 *string
	ErrorFamily               *string
	TimeoutSec                *int32
	GraceSec                  *int32
	StartedAt                 time.Time
	FinishedAt                *time.Time
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

type CreateDigitalEmployeeRunRequest struct {
	TenantID          uuid.UUID
	UserID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	Objective         string
	Prompt            string
	ContextRefs        []map[string]any
	ArtifactRefs       []map[string]any
	OutputSchema       map[string]any
	AllowedActions     []string
	IdempotencyKey     *string
	TimeoutSec         *int32
	GraceSec           *int32
	Metadata           map[string]any
}

type StopDigitalEmployeeRunRequest struct {
	TenantID          uuid.UUID
	UserID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	RunID             uuid.UUID
	Reason            string
}

type RuntimeCommandEventWriteback struct {
	EventType                 string         `json:"event_type"`
	SequenceNumber           int32          `json:"sequence_number"`
	Payload                  map[string]any `json:"payload"`
	ProviderSessionExternalID *string        `json:"provider_session_external_id,omitempty"`
	SessionStatePatch         map[string]any `json:"session_state_patch,omitempty"`
	LogRef                   *string        `json:"log_ref,omitempty"`
	RawEventRef              *string        `json:"raw_event_ref,omitempty"`
	Metadata                 map[string]any `json:"metadata,omitempty"`
}

type RuntimeCommandTerminalWriteback struct {
	Status                    DigitalEmployeeRunStatus `json:"status"`
	Summary                   string                   `json:"summary,omitempty"`
	Result                    map[string]any           `json:"result,omitempty"`
	Diagnostic                map[string]any           `json:"diagnostic,omitempty"`
	WorkProducts              []WorkProduct            `json:"work_products,omitempty"`
	ProviderSessionExternalID *string                  `json:"provider_session_external_id,omitempty"`
	SessionStatePatch         map[string]any           `json:"session_state_patch,omitempty"`
	LogRef                    *string                  `json:"log_ref,omitempty"`
	RawResultRef              *string                  `json:"raw_result_ref,omitempty"`
	ErrorMessage              *string                  `json:"error_message,omitempty"`
	ErrorCode                 *string                  `json:"error_code,omitempty"`
	ErrorFamily               *string                  `json:"error_family,omitempty"`
	ExitCode                  *int32                   `json:"exit_code,omitempty"`
	Signal                    *string                  `json:"signal,omitempty"`
	TimedOut                  bool                     `json:"timed_out,omitempty"`
}
```

- [ ] **Step 3: Add repository interface**

Create `apps/control-plane/internal/employee/run_repository.go`:

```go
package employee

import (
	"context"

	"github.com/google/uuid"
)

type DigitalEmployeeRunRepository interface {
	GetRunPreflight(ctx context.Context, tenantID, employeeID uuid.UUID) (RunPreflight, error)
	GetActiveRun(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployeeRun, error)
	GetRun(ctx context.Context, tenantID, employeeID, runID uuid.UUID) (*DigitalEmployeeRun, error)
	GetRunByCommandID(ctx context.Context, tenantID uuid.UUID, commandID string) (*DigitalEmployeeRun, error)
	ListRuns(ctx context.Context, tenantID, employeeID uuid.UUID, limit, offset int32) ([]*DigitalEmployeeRun, error)
	CreateRun(ctx context.Context, req CreateRunRecordRequest) (*DigitalEmployeeRun, error)
	UpdateRunStatus(ctx context.Context, req UpdateRunStatusRequest) (*DigitalEmployeeRun, error)
	CreateTaskEventIfAbsent(ctx context.Context, req CreateRunEventRecordRequest) error
	UpsertProviderSession(ctx context.Context, req UpsertProviderSessionRequest) (uuid.UUID, error)
	CreateProviderSessionEventIfAbsent(ctx context.Context, req CreateProviderSessionEventRecordRequest) error
	CreateCommandReceipt(ctx context.Context, req CreateRuntimeCommandReceiptRequest) error
	GetCommandReceipt(ctx context.Context, tenantID uuid.UUID, commandID string) (*RuntimeCommandReceipt, error)
	UpdateCommandReceipt(ctx context.Context, req UpdateRuntimeCommandReceiptRequest) (*RuntimeCommandReceipt, error)
}
```

Add the concrete record structs in the same file: `RunPreflight`, `CreateRunRecordRequest`, `UpdateRunStatusRequest`, `CreateRunEventRecordRequest`, `UpsertProviderSessionRequest`, `CreateProviderSessionEventRecordRequest`, `CreateRuntimeCommandReceiptRequest`, `UpdateRuntimeCommandReceiptRequest`, and `RuntimeCommandReceipt`. Use the same field names as the SQL query arguments in Task 1.

- [ ] **Step 4: Add pg repository implementation**

Create `apps/control-plane/internal/employee/pg_run_repository.go`. Implement each interface method with `queries.Queries` calls generated in Task 1. Add mapper helpers:

```go
func runStatusFromString(value string) DigitalEmployeeRunStatus {
	return DigitalEmployeeRunStatus(value)
}

func jsonMapFromBytes(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{"_decode_error": err.Error()}
	}
	return out
}

func redactRuntimeEventPayload(payload map[string]any) map[string]any {
	blocked := map[string]struct{}{
		"authorization": {},
		"password":      {},
		"secret":        {},
		"token":         {},
		"api_key":       {},
		"private_key":   {},
	}
	return redactMap(payload, blocked)
}
```

`redactMap` must recursively redact `map[string]any` values and leave arrays/scalars intact.

- [ ] **Step 5: Run focused tests**

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestDigitalEmployeeRunStatusTerminal|TestRuntimeWritebackEventRedactsSensitivePayload' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/employee/run_types.go apps/control-plane/internal/employee/run_repository.go apps/control-plane/internal/employee/pg_run_repository.go apps/control-plane/internal/employee/run_repository_test.go
git commit -m "feat(control-plane): add digital employee run repository"
```

---

### Task 3: Run Service Start, Stop, Admission, And Dispatch

**Files:**
- Create: `apps/control-plane/internal/employee/run_service.go`
- Test: `apps/control-plane/internal/employee/run_service_test.go`
- Modify: `apps/control-plane/internal/runtime/connection.go`
- Test: `apps/control-plane/internal/runtime/connection_test.go`

- [ ] **Step 1: Add connected-state test for `ConnectionRegistry`**

Append to `apps/control-plane/internal/runtime/connection_test.go`:

```go
func TestConnectionRegistryIsConnected(t *testing.T) {
	registry := NewConnectionRegistry()
	require.False(t, registry.IsConnected("node-1"))

	connection := registry.Register("node-1")
	require.True(t, registry.IsConnected("node-1"))

	registry.Unregister("node-1", connection.ID)
	require.False(t, registry.IsConnected("node-1"))
}
```

Run:

```bash
go test ./apps/control-plane/internal/runtime -run TestConnectionRegistryIsConnected -count=1
```

Expected: FAIL because `IsConnected` does not exist.

- [ ] **Step 2: Implement `ConnectionRegistry.IsConnected`**

Add to `apps/control-plane/internal/runtime/connection.go`:

```go
func (r *ConnectionRegistry) IsConnected(nodeID string) bool {
	r.mu.Lock()
	connection := r.connections[nodeID]
	r.mu.Unlock()
	if connection == nil {
		return false
	}
	select {
	case <-connection.Done():
		return false
	default:
		return true
	}
}
```

Run the same test. Expected: PASS.

- [ ] **Step 3: Write run service tests**

Create `apps/control-plane/internal/employee/run_service_test.go` with these test cases and fake repository/dispatcher:

```go
func TestRunServiceCreateRunRejectsActiveRun(t *testing.T) {
	repo := newFakeRunRepository()
	repo.activeRun = &DigitalEmployeeRun{ID: uuid.New(), Status: DigitalEmployeeRunStatusRunning}
	service := newRunServiceForTest(t, repo, newFakeDispatcher(true), newFakeAuditLogger())

	_, err := service.CreateRun(context.Background(), validCreateRunRequest())

	require.ErrorIs(t, err, ErrConflict)
	require.Equal(t, 0, repo.createdRunCount)
}

func TestRunServiceCreateRunDispatchesStartSession(t *testing.T) {
	repo := newFakeRunRepository()
	dispatcher := newFakeDispatcher(true)
	audit := newFakeAuditLogger()
	service := newRunServiceForTest(t, repo, dispatcher, audit)

	run, err := service.CreateRun(context.Background(), validCreateRunRequest())

	require.NoError(t, err)
	require.Equal(t, DigitalEmployeeRunStatusDispatching, run.Status)
	require.Len(t, dispatcher.commands, 1)
	require.Equal(t, "start_session", dispatcher.commands[0].Type)
	require.Contains(t, string(dispatcher.commands[0].Payload), `"provider_run_protocol":"provider-run/v1"`)
	require.Contains(t, string(dispatcher.commands[0].Payload), `"objective":"修复一个测试失败"`)
	require.Contains(t, audit.actions, "employee.run.create")
}

func TestRunServiceStopRunMovesToCancellingAndDispatchesStop(t *testing.T) {
	repo := newFakeRunRepository()
	repo.run = &DigitalEmployeeRun{
		ID:                  fixedRunID,
		TenantID:            fixedTenantID,
		TaskID:              fixedTaskID,
		DigitalEmployeeID:   fixedEmployeeID,
		ExecutionInstanceID: fixedExecutionInstanceID,
		RuntimeNodeID:       fixedRuntimeNodeID,
		NodeID:              "local-dev-node",
		CommandID:           "cmd-start",
		ProviderType:        "claude-code",
		Status:              DigitalEmployeeRunStatusRunning,
	}
	dispatcher := newFakeDispatcher(true)
	service := newRunServiceForTest(t, repo, dispatcher, newFakeAuditLogger())

	run, err := service.StopRun(context.Background(), StopDigitalEmployeeRunRequest{
		TenantID: fixedTenantID, UserID: fixedUserID, DigitalEmployeeID: fixedEmployeeID, RunID: fixedRunID, Reason: "human stop",
	})

	require.NoError(t, err)
	require.Equal(t, DigitalEmployeeRunStatusCancelling, run.Status)
	require.Len(t, dispatcher.commands, 1)
	require.Equal(t, "stop_session", dispatcher.commands[0].Type)
}
```

The fake repository must expose `preflight`, `activeRun`, `run`, `createdRunCount`, and append-only events so assertions can verify no DB write on admission rejection.

- [ ] **Step 4: Implement `DigitalEmployeeRunService`**

Create `apps/control-plane/internal/employee/run_service.go` with:

```go
type RuntimeCommandDispatcher interface {
	IsConnected(nodeID string) bool
	Dispatch(ctx context.Context, nodeID string, command runtime.RuntimeCommand) error
}

type DigitalEmployeeRunService struct {
	repository DigitalEmployeeRunRepository
	dispatcher RuntimeCommandDispatcher
	audit      AuditLogger
	now        func() time.Time
	newID      func() uuid.UUID
}

type AuditLogger interface {
	LogEvent(ctx context.Context, eventType, actorType, actorID, resourceType, resourceID, action string) error
}

func NewDigitalEmployeeRunService(repository DigitalEmployeeRunRepository, dispatcher RuntimeCommandDispatcher, audit AuditLogger) (*DigitalEmployeeRunService, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: run repository is required", ErrInvalidInput)
	}
	if dispatcher == nil {
		return nil, fmt.Errorf("%w: runtime dispatcher is required", ErrInvalidInput)
	}
	return &DigitalEmployeeRunService{
		repository: repository,
		dispatcher: dispatcher,
		audit:      audit,
		now:        time.Now,
		newID:      uuid.New,
	}, nil
}
```

Implement:

- `CreateRun(ctx, req)` trims `objective/prompt`, validates employee status `ready|active`, effective config approved, execution instance `ready|active`, Runtime connected, Provider healthy.
- Checks `GetActiveRun`; if present and no matching idempotency key, returns `ErrConflict`.
- Creates `task`, `task_run`, `runtime_command_receipt`.
- Builds `runtime.RuntimeCommand{ID: commandID, Type: "start_session", Payload: json.RawMessage(...)}`.
- Calls `Dispatch`; on failure marks run `failed` with `dispatch_failed`.
- On success marks run `dispatching` and appends `run_dispatched` event.
- Records audit event `digital_employee_run_created` with action `employee.run.create` after dispatch succeeds; records `digital_employee_run_dispatch_failed` when dispatch fails.
- `StopRun(ctx, req)` validates run ownership and active status, updates run to `cancelling`, appends `stop_requested`, dispatches `stop_session`, and leaves run as `cancelling` unless Runtime later writes terminal state.
- Records audit event `digital_employee_run_stop_requested` with action `employee.run.stop` after `stop_session` dispatch succeeds.

- [ ] **Step 5: Run service and runtime tests**

Run:

```bash
go test ./apps/control-plane/internal/runtime ./apps/control-plane/internal/employee -run 'TestConnectionRegistryIsConnected|TestRunService' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/runtime/connection.go apps/control-plane/internal/runtime/connection_test.go apps/control-plane/internal/employee/run_service.go apps/control-plane/internal/employee/run_service_test.go
git commit -m "feat(control-plane): dispatch digital employee runs"
```

---

### Task 4: Console Run API Routes

**Files:**
- Create: `apps/control-plane/internal/employee/run_handler.go`
- Modify: `apps/control-plane/internal/employee/handler.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/authz/types.go`
- Test: `apps/control-plane/internal/api/employee_routes_test.go`

- [ ] **Step 1: Write route tests**

Append tests to `apps/control-plane/internal/api/employee_routes_test.go`:

```go
func TestDigitalEmployeeRunRoutesCreateAndStop(t *testing.T) {
	server, fakeRunService := newEmployeeRunRouteServer(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees/11111111-1111-4111-8111-111111111111/runs", strings.NewReader(`{
		"objective":"修复一个测试失败",
		"context_refs":[{"kind":"task","id":"ctx-1"}],
		"allowed_actions":["read_repo"],
		"idempotency_key":"idem-1",
		"timeout_sec":60,
		"grace_sec":5
	}`))
	createReq = withConsoleIdentity(createReq)
	createResp := httptest.NewRecorder()
	server.ServeHTTP(createResp, createReq)

	require.Equal(t, http.StatusCreated, createResp.Code)
	require.Equal(t, "修复一个测试失败", fakeRunService.createRequests[0].Objective)

	stopReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees/11111111-1111-4111-8111-111111111111/runs/22222222-2222-4222-8222-222222222222/stop", strings.NewReader(`{"reason":"manual stop"}`))
	stopReq = withConsoleIdentity(stopReq)
	stopResp := httptest.NewRecorder()
	server.ServeHTTP(stopResp, stopReq)

	require.Equal(t, http.StatusOK, stopResp.Code)
	require.Equal(t, "manual stop", fakeRunService.stopRequests[0].Reason)
}
```

Run:

```bash
go test ./apps/control-plane/internal/api -run TestDigitalEmployeeRunRoutesCreateAndStop -count=1
```

Expected: FAIL because routes do not exist.

- [ ] **Step 2: Add authz actions**

Modify `apps/control-plane/internal/authz/types.go`:

```go
ActionEmployeeRunCreate = "employee.run.create"
ActionEmployeeRunStop   = "employee.run.stop"
ActionEmployeeRunLogRead = "employee.run.log.read"
```

Place these constants next to existing employee actions.

- [ ] **Step 3: Add run handler**

Create `apps/control-plane/internal/employee/run_handler.go` with methods:

```go
func (h *HTTPHandler) CreateDigitalEmployeeRun(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) ListDigitalEmployeeRuns(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) GetDigitalEmployeeRun(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) ListDigitalEmployeeRunEvents(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) StopDigitalEmployeeRun(w http.ResponseWriter, r *http.Request)
```

Add a `RunHandlerService` interface in this file with the five matching service methods. Each method must:

- read `employeeId` and `runId` using `uuid.Parse`;
- authorize with `Authorizer.Check`;
- use `employee.run.create` for create, `employee.run.stop` for stop, and `employee.read` for list/detail/events;
- return JSON responses with snake_case fields matching `DigitalEmployeeRun` and `RuntimeCommandEventWriteback`;
- map `ErrInvalidInput` to `400`, `ErrNotFound` to `404`, `ErrConflict` to `409`, and dispatch/preflight failures to `422` or `503` depending on cause.

- [ ] **Step 4: Register routes**

Modify `apps/control-plane/internal/api/server.go` inside the digital employee route group:

```go
r.Post("/digital-employees/{employeeId}/runs", s.employeeHandler.CreateDigitalEmployeeRun)
r.Get("/digital-employees/{employeeId}/runs", s.employeeHandler.ListDigitalEmployeeRuns)
r.Get("/digital-employees/{employeeId}/runs/{runId}", s.employeeHandler.GetDigitalEmployeeRun)
r.Get("/digital-employees/{employeeId}/runs/{runId}/events", s.employeeHandler.ListDigitalEmployeeRunEvents)
r.Post("/digital-employees/{employeeId}/runs/{runId}/stop", s.employeeHandler.StopDigitalEmployeeRun)
```

- [ ] **Step 5: Run route tests**

Run:

```bash
go test ./apps/control-plane/internal/api -run 'TestDigitalEmployeeRunRoutes' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/employee/run_handler.go apps/control-plane/internal/employee/handler.go apps/control-plane/internal/api/server.go apps/control-plane/internal/authz/types.go apps/control-plane/internal/api/employee_routes_test.go
git commit -m "feat(control-plane): expose digital employee run API"
```

---

### Task 5: Runtime Command Writeback API And Dual Projection

**Files:**
- Create: `apps/control-plane/internal/employee/run_writeback.go`
- Create: `apps/control-plane/internal/api/handlers/runtime_command_writeback.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/app/app.go`
- Test: `apps/control-plane/internal/api/handlers/runtime_command_writeback_test.go`
- Test: `apps/control-plane/internal/employee/run_writeback_test.go`

- [ ] **Step 1: Write writeback service tests**

Create `apps/control-plane/internal/employee/run_writeback_test.go` with:

```go
func TestWritebackEventCreatesTaskAndProviderSessionEventsIdempotently(t *testing.T) {
	repo := newFakeRunRepository()
	repo.run = runningRunForWriteback()
	service := NewDigitalEmployeeRunWritebackService(repo, newFakeAuditLogger())

	err := service.RecordEvent(context.Background(), fixedTenantID, "cmd-1", RuntimeCommandEventWriteback{
		EventType:      "text_delta",
		SequenceNumber: 7,
		Payload:        map[string]any{"text": "hello"},
		ProviderSessionExternalID: ptrString("provider-session-1"),
	})
	require.NoError(t, err)
	require.NoError(t, service.RecordEvent(context.Background(), fixedTenantID, "cmd-1", RuntimeCommandEventWriteback{
		EventType:      "text_delta",
		SequenceNumber: 7,
		Payload:        map[string]any{"text": "hello"},
		ProviderSessionExternalID: ptrString("provider-session-1"),
	}))

	require.Equal(t, 1, repo.taskEventCount)
	require.Equal(t, 1, repo.providerEventCount)
}

func TestWritebackTerminalDoesNotChangeExistingTerminalRun(t *testing.T) {
	repo := newFakeRunRepository()
	repo.run = runningRunForWriteback()
	service := NewDigitalEmployeeRunWritebackService(repo, newFakeAuditLogger())

	require.NoError(t, service.Complete(context.Background(), fixedTenantID, "cmd-1", RuntimeCommandTerminalWriteback{
		Status: DigitalEmployeeRunStatusCompleted,
		Result: map[string]any{"summary": "done"},
	}))
	require.ErrorIs(t, service.Fail(context.Background(), fixedTenantID, "cmd-1", RuntimeCommandTerminalWriteback{
		Status: DigitalEmployeeRunStatusFailed,
		ErrorMessage: ptrString("late failure"),
	}), ErrConflict)
}
```

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestWriteback' -count=1
```

Expected: FAIL because writeback service does not exist.

- [ ] **Step 2: Implement writeback service**

Create `apps/control-plane/internal/employee/run_writeback.go` with:

```go
type DigitalEmployeeRunWritebackService struct {
	repository DigitalEmployeeRunRepository
	audit      AuditLogger
}

func NewDigitalEmployeeRunWritebackService(repository DigitalEmployeeRunRepository, audit AuditLogger) *DigitalEmployeeRunWritebackService {
	return &DigitalEmployeeRunWritebackService{repository: repository, audit: audit}
}
```

Implement:

- `RecordEvent(ctx, tenantID, commandID, event)`:
  - load command receipt and run by `commandID`;
  - reject terminal runs except duplicate sequence writes;
  - redact payload;
  - upsert Provider session when `provider_session_external_id` exists;
  - write `task_events` and `provider_session_events` using idempotent queries.
- `Complete(ctx, tenantID, commandID, terminal)`:
  - move run to `completed`;
  - store result, diagnostic, work products, log refs and session state;
  - mark command receipt `completed`.
  - record audit event `digital_employee_run_completed`.
- `Fail(ctx, tenantID, commandID, terminal)`:
  - move run to `failed` with `error_family`, `error_code`, diagnostic and log refs.
  - record audit event `digital_employee_run_failed`.
- `Cancel(ctx, tenantID, commandID, terminal)`:
  - move run to `cancelled`.
  - record audit event `digital_employee_run_cancelled`.
- `TimedOut(ctx, tenantID, commandID, terminal)`:
  - move run to `timed_out` with `timed_out=true`.
  - record audit event `digital_employee_run_timed_out`.
- `CompleteProvisioning(ctx, tenantID, commandID, terminal)`:
  - for `resource_type='digital_employee_execution_instance'`, mark instance `ready`, employee `ready`, command receipt `completed`.
  - record audit event `digital_employee_instance_provisioned`.
- `FailProvisioning(ctx, tenantID, commandID, terminal)`:
  - mark command receipt `failed`, soft-delete employee and execution instance so Web does not see a half-created employee.
  - record audit event `digital_employee_instance_provision_failed`.

- [ ] **Step 3: Add Runtime-auth handler**

Create `apps/control-plane/internal/api/handlers/runtime_command_writeback.go`:

```go
type RuntimeCommandWritebackService interface {
	RecordEvent(ctx context.Context, tenantID uuid.UUID, commandID string, event employee.RuntimeCommandEventWriteback) error
	Complete(ctx context.Context, tenantID uuid.UUID, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error
	Fail(ctx context.Context, tenantID uuid.UUID, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error
	Cancel(ctx context.Context, tenantID uuid.UUID, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error
	TimedOut(ctx context.Context, tenantID uuid.UUID, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error
}

type RuntimeCommandWritebackHandler struct {
	service RuntimeCommandWritebackService
}
```

Add handler methods for:

- `POST /api/v1/runtime/commands/{commandId}/events`
- `POST /api/v1/runtime/commands/{commandId}/complete`
- `POST /api/v1/runtime/commands/{commandId}/fail`
- `POST /api/v1/runtime/commands/{commandId}/cancelled`
- `POST /api/v1/runtime/commands/{commandId}/timed-out`

Use `middleware.GetTenantID(r.Context())` and Runtime session auth. Return `202` for accepted writes and `409` for terminal conflicts.

- [ ] **Step 4: Wire routes and app container**

Modify `apps/control-plane/internal/api/server.go`:

```go
r.Post("/commands/{commandId}/events", s.runtimeCommandWritebackHandler.RecordEvent)
r.Post("/commands/{commandId}/complete", s.runtimeCommandWritebackHandler.Complete)
r.Post("/commands/{commandId}/fail", s.runtimeCommandWritebackHandler.Fail)
r.Post("/commands/{commandId}/cancelled", s.runtimeCommandWritebackHandler.Cancelled)
r.Post("/commands/{commandId}/timed-out", s.runtimeCommandWritebackHandler.TimedOut)
```

These routes belong in the existing Runtime session auth group under `/api/v1/runtime`.

Modify `apps/control-plane/internal/app/app.go` to create `runRepository`, `runService`, `writebackService`, and `RuntimeCommandWritebackHandler`, then pass the handler into the server.

- [ ] **Step 5: Run tests**

Run:

```bash
go test ./apps/control-plane/internal/employee ./apps/control-plane/internal/api/handlers ./apps/control-plane/internal/api -run 'TestWriteback|TestRuntimeCommandWriteback' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/employee/run_writeback.go apps/control-plane/internal/api/handlers/runtime_command_writeback.go apps/control-plane/internal/api/server.go apps/control-plane/internal/app/app.go apps/control-plane/internal/employee/run_writeback_test.go apps/control-plane/internal/api/handlers/runtime_command_writeback_test.go
git commit -m "feat(control-plane): accept runtime command writebacks"
```

---

### Task 6: Digital Employee Creation With Runtime Provisioning

**Files:**
- Modify: `apps/control-plane/internal/employee/types.go`
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/control-plane/internal/employee/handler.go`
- Modify: `apps/control-plane/internal/employee/repository.go`
- Modify: `apps/control-plane/internal/employee/pg_repository.go`
- Modify: `apps/control-plane/internal/storage/queries/employee_execution.sql`
- Test: `apps/control-plane/internal/employee/service_test.go`
- Test: `apps/control-plane/internal/api/employee_routes_test.go`

- [ ] **Step 1: Write creation provisioning tests**

Add to `apps/control-plane/internal/employee/service_test.go`:

```go
func TestCreateDraftRequiresRuntimeProviderAndConnectedRuntime(t *testing.T) {
	repo := newFakeEmployeeRepository()
	dispatcher := newFakeDispatcher(false)
	service := newEmployeeServiceForProvisioningTest(t, repo, dispatcher)

	_, err := service.CreateDraft(context.Background(), CreateDraftRequest{
		TenantID: fixedTenantID,
		TeamID: ptrUUID(fixedTeamID),
		Name: "代码员工",
		Role: "code_operator",
		RuntimeNodeID: fixedRuntimeNodeID,
		ProviderType: "claude-code",
	})

	require.ErrorIs(t, err, ErrRuntimeNotConnected)
	require.Equal(t, 0, repo.createdEmployeeCount)
}

func TestCreateDraftDispatchesProvisionInstanceAndReturnsReadyEmployee(t *testing.T) {
	repo := newFakeEmployeeRepository()
	repo.provisionCompletes = true
	dispatcher := newFakeDispatcher(true)
	service := newEmployeeServiceForProvisioningTest(t, repo, dispatcher)

	employee, err := service.CreateDraft(context.Background(), CreateDraftRequest{
		TenantID: fixedTenantID,
		TeamID: ptrUUID(fixedTeamID),
		Name: "代码员工",
		Role: "code_operator",
		RuntimeNodeID: fixedRuntimeNodeID,
		ProviderType: "claude-code",
	})

	require.NoError(t, err)
	require.Equal(t, DigitalEmployeeStatusReady, employee.Status)
	require.Len(t, dispatcher.commands, 1)
	require.Equal(t, "provision_instance", dispatcher.commands[0].Type)
	require.Contains(t, string(dispatcher.commands[0].Payload), `"provider_type":"claude-code"`)
}
```

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestCreateDraft.*Provision|TestCreateDraftRequiresRuntime' -count=1
```

Expected: FAIL because creation does not require Runtime or dispatch provisioning.

- [ ] **Step 2: Extend create request and handler input**

Modify `CreateDraftRequest` in `apps/control-plane/internal/employee/types.go`:

```go
RuntimeNodeID uuid.UUID
ProviderType  string
SessionPolicy map[string]any
WorkspacePolicy map[string]any
```

Modify `CreateDigitalEmployee` request body in `apps/control-plane/internal/employee/handler.go`:

```go
RuntimeNodeID uuid.UUID `json:"runtime_node_id"`
ProviderType string    `json:"provider_type"`
SessionPolicy map[string]any `json:"session_policy"`
WorkspacePolicy map[string]any `json:"workspace_policy"`
```

- [ ] **Step 3: Add repository operations for provisioning**

Extend `Repository` in `apps/control-plane/internal/employee/repository.go` with:

```go
GetRuntimeProvisioningPreflight(ctx context.Context, tenantID, teamID, runtimeNodeID uuid.UUID, providerType string) (RuntimeProvisioningPreflight, error)
CreateRuntimeCommandReceipt(ctx context.Context, req CreateRuntimeCommandReceiptRequest) error
WaitForRuntimeCommandCompletion(ctx context.Context, tenantID uuid.UUID, commandID string, interval time.Duration) (*RuntimeCommandReceipt, error)
AbortProvisionedDigitalEmployee(ctx context.Context, tenantID, employeeID, executionInstanceID uuid.UUID, reason string) error
```

Add SQL in `employee_execution.sql` to validate:

- team has current active governance config;
- runtime node is online;
- runtime enrollment approved;
- runtime session unexpired;
- provider capability `available=true`, `status='healthy'`, `health_status='healthy'`.

- [ ] **Step 4: Implement provisioning flow**

Modify `apps/control-plane/internal/employee/service.go`:

- reject empty `runtime_node_id` and `provider_type`;
- call `dispatcher.IsConnected(nodeID)` before creating records;
- create employee as `draft`;
- create execution instance as `provisioning`;
- create `runtime_command_receipt` with `command_type='provision_instance'`;
- dispatch `provision_instance` payload with `command_id`, `digital_employee_id`, `execution_instance_id`, `provider_type`, `governance_snapshot`, and `provider_run_protocol='provider-run/v1'`;
- wait up to `10s` for command receipt `completed`;
- on completion, update execution instance `ready` and employee `ready`;
- on failure or timeout, call `AbortProvisionedDigitalEmployee` and return an error.

- [ ] **Step 5: Run creation tests**

Run:

```bash
go test ./apps/control-plane/internal/employee ./apps/control-plane/internal/api -run 'TestCreateDraft.*Provision|TestCreateDigitalEmployee' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/employee apps/control-plane/internal/storage/queries/employee_execution.sql apps/control-plane/internal/api/employee_routes_test.go
git commit -m "feat(control-plane): provision employee execution instances at create"
```

---

### Task 7: Runtime Agent provider-run/v1 Payload And HTTP Writeback

**Files:**
- Modify: `apps/runtime-agent/src/controlplane/models.rs`
- Modify: `apps/runtime-agent/src/controlplane/client.rs`
- Modify: `apps/runtime-agent/src/controlplane/ws.rs`
- Modify: `apps/runtime-agent/src/commands/payload.rs`
- Modify: `apps/runtime-agent/src/commands/executor.rs`
- Modify: `apps/runtime-agent/src/runs.rs`
- Test: `apps/runtime-agent/tests/runtime_command_payload_test.rs`
- Test: `apps/runtime-agent/tests/runtime_command_executor_test.rs`
- Test: `apps/runtime-agent/tests/controlplane_client_test.rs`

- [ ] **Step 1: Write Runtime payload and writeback tests**

Extend `apps/runtime-agent/tests/runtime_command_payload_test.rs`:

```rust
#[test]
fn parses_provider_run_v1_fields() {
    let mut payload = valid_payload();
    payload["provider_run_protocol"] = json!("provider-run/v1");
    payload["objective"] = json!("修复一个测试失败");
    payload["output_schema"] = json!({"type":"object"});
    payload["allowed_actions"] = json!(["read_repo"]);
    payload["forbidden_actions"] = json!(["deploy"]);
    payload["secret_refs"] = json!(["github-token"]);
    payload["timeout_sec"] = json!(60);
    payload["grace_sec"] = json!(5);

    let parsed = RuntimeSessionCommandPayload::from_command(&command(payload)).expect("parse payload");

    assert_eq!(parsed.provider_run_protocol, "provider-run/v1");
    assert_eq!(parsed.objective.as_deref(), Some("修复一个测试失败"));
    assert_eq!(parsed.allowed_actions, vec!["read_repo"]);
    assert_eq!(parsed.forbidden_actions, vec!["deploy"]);
    assert_eq!(parsed.secret_refs, vec!["github-token"]);
    assert_eq!(parsed.timeout_sec, Some(60));
    assert_eq!(parsed.grace_sec, Some(5));
}
```

Add a client test in `apps/runtime-agent/tests/controlplane_client_test.rs` that starts a local HTTP server and asserts `ControlPlaneClient::record_command_event("cmd-1", ...)` posts to `/api/v1/runtime/commands/cmd-1/events` with runtime session auth headers.

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml parses_provider_run_v1_fields record_command_event_posts_runtime_command_endpoint
```

Expected: FAIL because payload fields and client method do not exist.

- [ ] **Step 2: Extend runtime command models and payload parser**

Modify `apps/runtime-agent/src/controlplane/models.rs`:

- add `RuntimeCommandType::ProvisionInstance`;
- parse `"provision_instance"` into that variant;
- add serializable structs `RuntimeCommandEventWriteback` and `RuntimeCommandTerminalWriteback`.

Modify `apps/runtime-agent/src/commands/payload.rs`:

```rust
#[serde(default = "default_provider_run_protocol")]
pub provider_run_protocol: String,
#[serde(default)]
pub objective: Option<String>,
#[serde(default)]
pub output_schema: serde_json::Value,
#[serde(default)]
pub allowed_actions: Vec<String>,
#[serde(default)]
pub forbidden_actions: Vec<String>,
#[serde(default)]
pub secret_refs: Vec<String>,
#[serde(default)]
pub timeout_sec: Option<u64>,
#[serde(default)]
pub grace_sec: Option<u64>,
```

Validate `provider_run_protocol == "provider-run/v1"`.

- [ ] **Step 3: Add writeback client methods**

Modify `apps/runtime-agent/src/controlplane/client.rs`:

```rust
pub async fn record_command_event(&self, command_id: &str, event: RuntimeCommandEventWriteback) -> Result<()>
pub async fn complete_command(&self, command_id: &str, terminal: RuntimeCommandTerminalWriteback) -> Result<()>
pub async fn fail_command(&self, command_id: &str, terminal: RuntimeCommandTerminalWriteback) -> Result<()>
pub async fn cancel_command(&self, command_id: &str, terminal: RuntimeCommandTerminalWriteback) -> Result<()>
pub async fn time_out_command(&self, command_id: &str, terminal: RuntimeCommandTerminalWriteback) -> Result<()>
```

Each method posts to `/api/v1/runtime/commands/{commandId}/...` using `.bearer_auth(&self.token)` and `self.runtime_headers()?`.

- [ ] **Step 4: Inject writeback client into command loop**

Modify `apps/runtime-agent/src/controlplane/ws.rs`:

- build `ControlPlaneClient::with_session_token(config.runtime.control_plane_url.clone(), session_token.clone(), config.node.id.clone())`;
- pass it to `RuntimeCommandExecutor::new_with_writeback(config, Some(client))`;
- keep existing `RuntimeCommandExecutor::new(config)` for tests without HTTP writeback.

- [ ] **Step 5: Write back provider events and terminal state**

Modify `apps/runtime-agent/src/commands/executor.rs`:

- `handle_ensure_instance` also handles `ProvisionInstance`;
- after `ensure_instance` succeeds for provisioning, call `complete_command(command.id, terminal)` with `status="completed"`;
- on provisioning failure, call `fail_command`;
- during `drain_provider_events`, increment sequence numbers and call `record_command_event`;
- on `TurnCompleted`, call `complete_command` with `summary`, `result_json`, `provider_session_external_id`, `log_ref`;
- on `TurnError`, call `fail_command` with `error_family="provider_failed"`;
- on `cancel_run`, call `cancel_command` when the local run enters `Cancelled`;
- if timeout support is added in this task, use `tokio::time::timeout` around provider execution and call `time_out_command`.

- [ ] **Step 6: Run Runtime tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml runtime_command_payload_test runtime_command_executor_test controlplane_client_test
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/runtime-agent/src/controlplane apps/runtime-agent/src/commands apps/runtime-agent/src/runs.rs apps/runtime-agent/tests
git commit -m "feat(runtime-agent): write back provider run events"
```

---

### Task 8: Control Plane OpenAPI Contract

**Files:**
- Modify: `contracts/control-plane/openapi.yaml`
- Modify generated: `apps/control-plane/internal/api/gen/control_plane.gen.go`
- Test: `scripts/verify-foundation-contracts.mjs`

- [ ] **Step 1: Add OpenAPI paths**

Modify `contracts/control-plane/openapi.yaml` and add:

- `POST /api/v1/digital-employees/{employeeId}/runs`
- `GET /api/v1/digital-employees/{employeeId}/runs`
- `GET /api/v1/digital-employees/{employeeId}/runs/{runId}`
- `GET /api/v1/digital-employees/{employeeId}/runs/{runId}/events`
- `POST /api/v1/digital-employees/{employeeId}/runs/{runId}/stop`
- `POST /api/v1/runtime/commands/{commandId}/events`
- `POST /api/v1/runtime/commands/{commandId}/complete`
- `POST /api/v1/runtime/commands/{commandId}/fail`
- `POST /api/v1/runtime/commands/{commandId}/cancelled`
- `POST /api/v1/runtime/commands/{commandId}/timed-out`

Add schemas:

- `DigitalEmployeeRunCreateRequest`
- `DigitalEmployeeRunResponse`
- `DigitalEmployeeRunEventResponse`
- `DigitalEmployeeRunStopRequest`
- `RuntimeCommandEventWriteback`
- `RuntimeCommandTerminalWriteback`
- `WorkProduct`
- `RunDiagnostic`

- [ ] **Step 2: Generate API artifacts**

Run:

```bash
pnpm generate:control-plane
pnpm verify:contracts
```

Expected: generated code updates cleanly and contract verification passes.

- [ ] **Step 3: Commit**

```bash
git add contracts/control-plane/openapi.yaml apps/control-plane/internal/api/gen/control_plane.gen.go
git commit -m "docs(contracts): add digital employee run APIs"
```

---

### Task 9: Web API Client And Employee Detail Page

**Files:**
- Modify: `apps/web/src/lib/api/employees.ts`
- Modify: `apps/web/src/lib/api/employees.test.ts`
- Create: `apps/web/src/features/employees/detail.tsx`
- Create: `apps/web/src/features/employees/detail.test.tsx`
- Modify: `apps/web/src/features/employees/index.tsx`
- Modify: `apps/web/src/features/employees/index.test.tsx`
- Create: `apps/web/src/routes/_authenticated/employees/$employeeId.tsx`

- [ ] **Step 1: Write Web API client tests**

Append to `apps/web/src/lib/api/employees.test.ts`:

```ts
it("creates a digital employee run", async () => {
  const fetcher = vi.fn(async () => jsonResponse({ id: "run-1", status: "dispatching" }));

  await createDigitalEmployeeRun(
    { baseUrl: "http://control-plane.local", fetcher },
    "employee 1/primary",
    {
      objective: "修复一个测试失败",
      context_refs: [{ kind: "task", id: "ctx-1" }],
      allowed_actions: ["read_repo"],
      idempotency_key: "idem-1",
      timeout_sec: 60,
      grace_sec: 5,
    },
  );

  expect(fetcher).toHaveBeenCalledWith(
    "http://control-plane.local/api/v1/digital-employees/employee%201%2Fprimary/runs",
    expect.objectContaining({ method: "POST" }),
  );
});

it("stops a digital employee run", async () => {
  const fetcher = vi.fn(async () => jsonResponse({ id: "run-1", status: "cancelling" }));

  await stopDigitalEmployeeRun({ baseUrl: "http://control-plane.local", fetcher }, "employee-1", "run-1", {
    reason: "manual stop",
  });

  expect(fetcher).toHaveBeenCalledWith(
    "http://control-plane.local/api/v1/digital-employees/employee-1/runs/run-1/stop",
    expect.objectContaining({ method: "POST" }),
  );
});
```

Run:

```bash
pnpm --filter @superteam/web test apps/web/src/lib/api/employees.test.ts
```

Expected: FAIL because client functions do not exist.

- [ ] **Step 2: Add Web API types and functions**

Modify `apps/web/src/lib/api/employees.ts`:

```ts
export type DigitalEmployeeRunStatus =
  | "queued"
  | "dispatching"
  | "running"
  | "cancelling"
  | "completed"
  | "failed"
  | "cancelled"
  | "timed_out";

export type DigitalEmployeeRun = {
  id: string;
  task_id: string;
  digital_employee_id: string;
  execution_instance_id: string;
  runtime_node_id: string;
  node_id: string;
  command_id: string;
  provider_type: string;
  status: DigitalEmployeeRunStatus;
  result?: Record<string, unknown>;
  diagnostic?: Record<string, unknown>;
  work_products?: WorkProduct[];
  error_message?: string;
  error_code?: string;
  error_family?: string;
  log_ref?: string;
  started_at?: string;
  finished_at?: string;
  created_at?: string;
  updated_at?: string;
};

export type WorkProduct = {
  type: string;
  title: string;
  summary: string;
  ref: string;
  metadata?: Record<string, unknown>;
  created_at?: string;
};
```

Add `createDigitalEmployeeRun`, `listDigitalEmployeeRuns`, `getDigitalEmployeeRun`, `listDigitalEmployeeRunEvents`, and `stopDigitalEmployeeRun` using the same `postJson` and `buildApiUrl` patterns as existing employee APIs.

- [ ] **Step 3: Write detail page tests**

Create `apps/web/src/features/employees/detail.test.tsx`:

```tsx
it("shows run controls events result failure and stop button", async () => {
  const fetcher = createEmployeeDetailFetcher({
    employee: { id: "employee-1", name: "代码员工", role: "code_operator", status: "active" },
    instance: { id: "instance-1", digital_employee_id: "employee-1", runtime_node_id: "runtime-1", provider_type: "claude-code", status: "ready" },
    runs: [{ id: "run-1", task_id: "task-1", digital_employee_id: "employee-1", execution_instance_id: "instance-1", runtime_node_id: "runtime-1", node_id: "local-dev-node", command_id: "cmd-1", provider_type: "claude-code", status: "running" }],
    events: [{ event_type: "text_delta", sequence_number: 1, payload: { text: "hello" } }],
  });

  renderWithQueryClient(<EmployeeDetailView apiBaseUrl="http://control-plane.local" employeeId="employee-1" fetcher={fetcher} />);

  expect(await screen.findByText("代码员工")).toBeInTheDocument();
  expect(await screen.findByText("running")).toBeInTheDocument();
  expect(await screen.findByText("hello")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "停止" })).toBeEnabled();
});
```

Run:

```bash
pnpm --filter @superteam/web test apps/web/src/features/employees/detail.test.tsx
```

Expected: FAIL because detail page does not exist.

- [ ] **Step 4: Implement detail page**

Create `apps/web/src/features/employees/detail.tsx`:

- `EmployeeDetailPage` resolves API base URL and renders `EmployeeDetailView`.
- `EmployeeDetailView` queries employee, execution instance, runs and events.
- Top summary shows name, team, status, risk, runtime node, provider and latest run.
- Run control uses `<textarea>` for `objective`, numeric inputs for `timeout_sec` and `grace_sec`, and a primary button with a `Play` icon.
- Current execution panel shows `running` or `cancelling` run, `command_id`, start time and a `Square` icon stop button.
- Event stream renders by `sequence_number`.
- Result panel renders `summary`, `usage`, `cost`, `work_products` and artifact refs.
- Failure panel renders `error_family`, `error_code`, exit code, signal, timed out, stderr excerpt and log ref.
- History list renders recent runs; clicking a run changes selected run and reloads its events.

Use compact panels with 8px-or-less radius and existing shadcn components. Do not put cards inside cards.

- [ ] **Step 5: Add route and list link**

Create `apps/web/src/routes/_authenticated/employees/$employeeId.tsx`:

```tsx
import { createFileRoute } from "@tanstack/react-router";
import { EmployeeDetailPage } from "@/features/employees/detail";

export const Route = createFileRoute("/_authenticated/employees/$employeeId")({
  component: EmployeeDetailRoute,
});

function EmployeeDetailRoute() {
  const { employeeId } = Route.useParams();
  return <EmployeeDetailPage employeeId={employeeId} />;
}
```

Modify `apps/web/src/features/employees/index.tsx` so each row includes a `Link` to `/employees/${employee.id}` with label `详情`.

- [ ] **Step 6: Run Web tests and typecheck**

Run:

```bash
pnpm --filter @superteam/web test apps/web/src/lib/api/employees.test.ts apps/web/src/features/employees/detail.test.tsx apps/web/src/features/employees/index.test.tsx
pnpm --filter @superteam/web typecheck
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/web/src/lib/api/employees.ts apps/web/src/lib/api/employees.test.ts apps/web/src/features/employees apps/web/src/routes/_authenticated/employees apps/web/src/routeTree.gen.ts
git commit -m "feat(web): add digital employee run detail"
```

---

### Task 10: End-To-End Smoke, Changelog, And Final Verification

**Files:**
- Modify: `CHANGELOG.md`
- Optional test helper: `apps/control-plane/internal/api/e2e_test.go`

- [ ] **Step 1: Run backend verification**

Run:

```bash
pnpm verify:control-plane
```

Expected: OpenAPI contract verification and `go test ./apps/control-plane/...` pass.

- [ ] **Step 2: Run Runtime Agent verification**

Run:

```bash
pnpm verify:runtime-agent
```

Expected: contract verification and `cargo test --manifest-path apps/runtime-agent/Cargo.toml` pass.

- [ ] **Step 3: Run Web verification**

Run:

```bash
pnpm verify:web
```

Expected: Vitest, TypeScript, and Vite build pass.

- [ ] **Step 4: Run local smoke**

Start services in separate terminals:

```bash
pnpm dev:control-plane
pnpm dev:runtime-agent
pnpm dev:web
```

Smoke sequence:

```bash
curl -s http://127.0.0.1:7077/health
curl -s http://127.0.0.1:7077/providers
```

Then use Web:

1. Confirm Runtime node is approved and online.
2. Create a team governance config and approve it.
3. Create a digital employee with Runtime and Provider selected.
4. Confirm creation only succeeds after `provision_instance` completion.
5. Open `/employees/{employeeId}`.
6. Start a tiny run with objective `输出 hello 并结束`.
7. Confirm event stream, result, log ref and provider session are visible.
8. Start a longer run and click stop.
9. Confirm `running -> cancelling -> cancelled`.
10. Start a run with tiny timeout and confirm `running -> timed_out`.

- [ ] **Step 5: Update changelog**

Get current local timestamp:

```bash
date '+%Y-%m-%d %H:%M'
```

Append to `CHANGELOG.md`:

```markdown
- YYYY-MM-DD HH:mm 数字员工执行闭环：新增创建时 Runtime 预置、run API、Runtime command writeback、Provider 事件双投影、Web 详情页执行/停止/结果展示。
```

Replace `YYYY-MM-DD HH:mm` with the command output.

- [ ] **Step 6: Final repository checks**

Run:

```bash
git diff --check
git status --short
```

Expected: no whitespace errors; only intended files are modified.

- [ ] **Step 7: Commit**

```bash
git add CHANGELOG.md
git commit -m "chore: record digital employee execution loop"
```

---

## Self-Review

Spec coverage:

- 创建时 Runtime 选择和预置：Task 6、Task 7。
- Runtime WebSocket 不在线拒绝创建：Task 3 `IsConnected`、Task 6 creation preflight。
- 团队默认治理资产推送：Task 6 provisioning payload。
- `POST /digital-employees/{employeeId}/runs`：Task 3、Task 4、Task 8、Task 9。
- `task_id/run_id/command_id`：Task 1、Task 3。
- `ConnectionRegistry.Dispatch start_session/stop_session`：Task 3。
- Runtime HTTP 回写 events/result/failure/cancel/timed_out：Task 5、Task 7。
- 双投影到 `task_events` 和 `provider_session_events`：Task 1、Task 5。
- `cancelling` 和 `timed_out`：Task 1、Task 3、Task 5、Task 7、Task 9。
- 日志引用、诊断、session state、work products：Task 1、Task 2、Task 5、Task 9。
- Web 详情页执行中、事件流、结果、失败原因、停止：Task 9。
- 权限与审计入口：Task 3、Task 4、Task 5、Task 10；run create/stop、dispatch failure、provision success/failure、terminal writeback 都通过 `AuditLogger` 调用 existing audit service。

Type consistency:

- Run status values are consistent across DB strings, Go domain, Runtime terminal writeback, and Web union type.
- Command identity uses `command_id` in API payloads, `RuntimeCommand.ID` in Go dispatch, and Runtime Agent writeback path parameter.
- Provider session external id uses `provider_session_external_id` in writeback and `provider_session_id` for existing Provider session table column.

Execution handoff:

- Prefer subagent-driven execution because the plan crosses DB, Go API, Rust Runtime Agent, and React Web.
- Each task has an isolated commit point and focused test command.
