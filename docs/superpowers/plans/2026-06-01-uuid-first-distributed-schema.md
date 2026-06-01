# UUID-first 分布式库表重构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将当前 Control Plane 主链路从 `BIGSERIAL/BIGINT` 重构为 UUID-first 初始 schema，并让现有任务、认证、Runtime、审计和 Web API 测试在重建库后跑通。

**Architecture:** 采用“重写初始 schema，早期环境直接重建库”的策略，不做旧库数据迁移。第一批只落地当前真实链路所需的 UUID、租户/团队骨架、任务运行分层和生成链路同步；完整企业授权、数字员工目录、Capability 注册表进入后续独立计划。

**Tech Stack:** PostgreSQL 16 开发目标 + SQL migration + Atlas migration runner + sqlc + pgx/v5 + Go chi/net/http + oapi-codegen + React/Vite TypeScript。

---

## Scope

本计划实现 spec 的第一批可验证范围：

- 所有 SuperTeam 自有表的 `id` 变为 `UUID PRIMARY KEY DEFAULT gen_random_uuid()`。
- 当前 `tasks`、`auth_users`、`auth_sessions`、`runtime_nodes`、日志和审计链路全部切换为 UUID。
- 新增最小 `tenants`、`tenant_profiles`、`tenant_teams`、`tenant_members`、`runtime_node_scopes`、`runtime_leases` 骨架。
- `task_executions` 重命名为 `task_runs`，代码仍可通过任务服务完成 claim、event、complete、fail。
- OpenAPI、sqlc、Go domain model、handler、Web API client 的内部 ID 类型同步为 UUID 字符串。
- 本地和远端开发库按重建策略处理；执行前必须确认没有需要保留的数据。

本计划不实现：

- 完整 OpenFGA 权限模型。
- 完整数字员工目录页面。
- Capability 注册与授权页面。
- Temporal 工作流表。
- 数据保留型 BIGSERIAL 到 UUID 迁移。

## File Structure

### Database

- Modify: `apps/control-plane/internal/storage/migrations/001_initial.sql`
  UUID-first 初始 schema，包含当前主链路表、最小租户/团队骨架、中文表注释和字段注释。
- Modify: `apps/control-plane/internal/storage/migrations/002_seed_dev_admin.sql`
  插入默认租户、默认团队、开发管理员；不依赖自增 ID。
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`
  重写和删除迁移后必须重新生成 Atlas 校验和。
- Delete: `apps/control-plane/internal/storage/migrations/003_create_auth_sessions.sql`
  已并入新 `001_initial.sql`。
- Delete: `apps/control-plane/internal/storage/migrations/004_create_web_logs.sql`
  已并入新 `001_initial.sql`。
- Delete: `apps/control-plane/internal/storage/migrations/005_comment_auth_users_and_web_operation_logs.sql`
  已并入新 `001_initial.sql`。
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
  增加 UUID-first schema contract tests。

### sqlc

- Modify: `apps/control-plane/sqlc.yaml`
  将 PostgreSQL `uuid` 映射到 `github.com/google/uuid.UUID` 和 `github.com/google/uuid.NullUUID`。
- Modify: `apps/control-plane/internal/storage/queries/auth.sql`
- Modify: `apps/control-plane/internal/storage/queries/tasks.sql`
- Modify: `apps/control-plane/internal/storage/queries/runtime.sql`
- Modify: `apps/control-plane/internal/storage/queries/audit.sql`
- Modify: `apps/control-plane/internal/storage/queries/web_logs.sql`
- Regenerate: `apps/control-plane/internal/storage/queries/*.sql.go`
- Regenerate: `apps/control-plane/internal/storage/queries/models.go`
- Regenerate: `apps/control-plane/internal/storage/queries/querier.go`

### Go Control Plane

- Modify: `apps/control-plane/internal/auth/types.go`
- Modify: `apps/control-plane/internal/auth/models.go`
- Modify: `apps/control-plane/internal/auth/service.go`
- Modify: `apps/control-plane/internal/auth/pg_repository.go`
- Modify: `apps/control-plane/internal/auth/handler.go`
- Modify: `apps/control-plane/internal/task/models.go`
- Modify: `apps/control-plane/internal/task/repository.go`
- Modify: `apps/control-plane/internal/task/service.go`
- Modify: `apps/control-plane/internal/task/pg_repository.go`
- Modify: `apps/control-plane/internal/runtime/models.go`
- Modify: `apps/control-plane/internal/runtime/repository.go`
- Modify: `apps/control-plane/internal/runtime/service.go`
- Modify: `apps/control-plane/internal/runtime/pg_repository.go`
- Modify: `apps/control-plane/internal/api/handlers/task.go`
- Modify: `apps/control-plane/internal/api/handlers/runtime.go`
- Modify: `apps/control-plane/internal/api/handlers/responses.go`
- Modify: `apps/control-plane/internal/audit/service.go`
- Modify tests under `apps/control-plane/internal/**`.

### OpenAPI and Web

- Modify: `contracts/control-plane/openapi.yaml`
- Modify: `contracts/control-plane/auth.yaml`
- Regenerate: `apps/control-plane/internal/api/gen/control_plane.gen.go`
- Regenerate: `apps/control-plane/internal/auth/generated.go`
- Modify: `apps/web/src/lib/api/tasks.ts`
- Modify: `apps/web/src/lib/api/auth.ts`
- Modify tests under `apps/web/src/lib/api/*.test.ts`.

### Docs

- Modify: `CHANGELOG.md`
- Modify or create: `docs/database/rebuild_uuid_schema.md`

---

## Task 1: Add UUID-first Schema Contract Tests

**Files:**
- Modify: `apps/control-plane/internal/storage/migrations_test.go`

- [ ] **Step 1: Add failing schema contract tests**

Append these tests to `apps/control-plane/internal/storage/migrations_test.go`:

```go
func TestInitialSchemaIsUUIDFirst(t *testing.T) {
	body, err := os.ReadFile("migrations/001_initial.sql")
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	sql := string(body)

	for _, forbidden := range []string{
		"BIGSERIAL PRIMARY KEY",
		" user_id BIGINT",
		" creator_id BIGINT",
		" task_id BIGINT",
		" execution_id BIGINT",
		"id VARCHAR(255) PRIMARY KEY",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("initial schema must not contain %q", forbidden)
		}
	}

	for _, expected := range []string{
		"CREATE TABLE tenants",
		"CREATE TABLE tenant_teams",
		"CREATE TABLE runtime_node_scopes",
		"CREATE TABLE runtime_leases",
		"CREATE TABLE auth_sessions",
		"CREATE TABLE task_runs",
		"CREATE TABLE web_login_logs",
		"CREATE TABLE web_operation_logs",
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"tenant_id UUID NOT NULL",
		"user_id UUID NOT NULL",
		"token_hash VARCHAR(255) UNIQUE NOT NULL",
		"creator_id UUID",
		"task_id UUID NOT NULL",
		"run_id UUID",
		"updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"cancelled_at TIMESTAMPTZ",
		"CREATE UNIQUE INDEX uq_task_events_task_sequence",
		"COMMENT ON TABLE tenants IS",
		"COMMENT ON COLUMN tasks.tenant_id IS",
		"COMMENT ON COLUMN tasks.cancelled_at IS",
		"COMMENT ON TABLE web_login_logs IS",
		"COMMENT ON TABLE web_operation_logs IS",
		"COMMENT ON COLUMN web_operation_logs.action IS",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected UUID-first initial schema to contain %q", expected)
		}
	}
}

func TestForwardOnlyAuthAndWebLogMigrationsWereMergedIntoInitialSchema(t *testing.T) {
	body, err := os.ReadFile("migrations/001_initial.sql")
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"CREATE TABLE auth_sessions",
		"token_hash VARCHAR(255) UNIQUE NOT NULL",
		"CREATE TABLE web_login_logs",
		"CREATE TABLE web_operation_logs",
		"event_type VARCHAR(50) NOT NULL CHECK (event_type IN ('login_succeeded', 'login_failed', 'logout_succeeded'))",
		"session_id UUID",
		"request_id VARCHAR(255)",
		"COMMENT ON TABLE auth_users IS 'Web 控制台平台用户表'",
		"COMMENT ON COLUMN auth_users.password_hash IS '用户密码哈希，禁止存储明文密码'",
		"COMMENT ON TABLE web_operation_logs IS 'Web 控制台操作日志表'",
		"COMMENT ON COLUMN web_operation_logs.action IS '操作动作'",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected merged auth/web log schema to contain %q", expected)
		}
	}

	for _, path := range []string{
		"migrations/003_create_auth_sessions.sql",
		"migrations/004_create_web_logs.sql",
		"migrations/005_comment_auth_users_and_web_operation_logs.sql",
	} {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("%s should be merged into 001_initial.sql for rebuild-only schema", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", path, err)
		}
	}
}
```

- [ ] **Step 2: Run the focused tests and confirm failure**

Run:

```bash
cd apps/control-plane
go test ./internal/storage -run 'TestInitialSchemaIsUUIDFirst|TestForwardOnlyAuthAndWebLogMigrationsWereMergedIntoInitialSchema' -count=1
```

Expected: FAIL. The failure should mention existing `BIGSERIAL PRIMARY KEY` or existing merged migration files.

- [ ] **Step 3: Add executable schema/comment contract coverage**

Add one executable migration contract test that applies `001_initial.sql` against an isolated PostgreSQL test database when `TEST_DATABASE_URL` is available. If no test database is configured, it must `t.Skip` with an explicit message and the string-based tests still run.

The test must verify:

- every expected table exists;
- every expected `id` column has `data_type = 'uuid'` and a `gen_random_uuid()` default;
- `task_runs.updated_at` exists with `timestamp with time zone`;
- `auth_sessions.id`, `auth_sessions.user_id`, `web_login_logs.session_id`, `web_operation_logs.user_id`, `tasks.creator_id`, and `task_events.run_id` are UUID columns;
- task, execution, audit, artifact, login log and operation log table blocks do not contain heavy `REFERENCES` dependencies;
- core lifecycle columns such as `disabled_at`, `archived_at`, `deleted_at`, `cancelled_at`, `finished_at`, and `revoked_at` exist where they express state instead of relying on cascade deletion;
- table and column comments exist in `pg_catalog.pg_description`;
- all columns on the 18 SuperTeam-owned tables have non-empty comments.

Keep the string contract tests as fast local feedback; this DB-backed test is the guard against SQL text existing but migration execution or comments being wrong.

- [ ] **Step 4: Commit the failing tests**

```bash
git add apps/control-plane/internal/storage/migrations_test.go
git commit -m "test: define uuid-first schema contract"
```

If the execution policy does not allow an intermediate failing commit, keep the failing test in the working tree and continue directly to Task 2; do not drop the red-green verification step.

---

## Task 2: Rewrite Initial Schema and Seed Data

**Files:**
- Modify: `apps/control-plane/internal/storage/migrations/001_initial.sql`
- Modify: `apps/control-plane/internal/storage/migrations/002_seed_dev_admin.sql`
- Delete: `apps/control-plane/internal/storage/migrations/003_create_auth_sessions.sql`
- Delete: `apps/control-plane/internal/storage/migrations/004_create_web_logs.sql`
- Delete: `apps/control-plane/internal/storage/migrations/005_comment_auth_users_and_web_operation_logs.sql`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`

- [ ] **Step 1: Replace `001_initial.sql` with UUID-first schema**

Use these concrete schema decisions:

- Do not add `pgcrypto` solely for UUID generation. The project target is PostgreSQL 13+ and `gen_random_uuid()` is built in; enable `pgcrypto` only if a future migration needs extra cryptographic functions.
- Add these fixed development IDs as SQL comments and defaults:
  - default tenant: `00000000-0000-0000-0000-000000000001`
  - default team: `00000000-0000-0000-0000-000000000101`
- Create these tables in this order:
  1. `tenants`
  2. `tenant_profiles`
  3. `tenant_teams`
  4. `auth_users`
  5. `tenant_members`
  6. `runtime_nodes`
  7. `runtime_node_scopes`
  8. `auth_runtime_tokens`
  9. `auth_sessions`
  10. `tasks`
  11. `task_runs`
  12. `runtime_leases`
  13. `task_state_history`
  14. `task_events`
  15. `task_artifacts`
  16. `audit_events`
  17. `web_login_logs`
  18. `web_operation_logs`
- Every table above must have `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`.
- Every table and column must have a Chinese `COMMENT ON` statement.
- Add a schema contract test that verifies comments through `pg_catalog` or equivalent DB metadata, not only SQL text fragments. Fast string checks are still useful, but completion requires proving that every non-system table and column created by `001_initial.sql` has a non-empty Chinese comment after migration execution.

The current-chain columns must include this minimum shape:

```sql
CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid,
    team_id UUID,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    creator_id UUID,
    provider_type VARCHAR(100) NOT NULL,
    target_node_id VARCHAR(255),
    assigned_node_id VARCHAR(255),
    status VARCHAR(50) NOT NULL,
    workspace_path TEXT,
    params JSONB NOT NULL DEFAULT '{}'::jsonb,
    priority INTEGER NOT NULL DEFAULT 0,
    idempotency_key VARCHAR(255),
    risk_level VARCHAR(50) NOT NULL DEFAULT 'normal',
    cancelled_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE task_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid,
    task_id UUID NOT NULL,
    node_id VARCHAR(255) NOT NULL,
    runtime_node_id UUID,
    provider_session_id VARCHAR(255),
    status VARCHAR(50) NOT NULL,
    lease_expires_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    result JSONB,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE task_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid,
    task_id UUID NOT NULL,
    run_id UUID,
    event_type VARCHAR(100) NOT NULL,
    sequence_number INTEGER NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uq_task_events_task_sequence
    ON task_events(task_id, sequence_number);

CREATE UNIQUE INDEX uq_task_events_run_sequence
    ON task_events(run_id, sequence_number)
    WHERE run_id IS NOT NULL;

CREATE TABLE auth_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES auth_users(id),
    token_hash VARCHAR(255) UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    client_ip VARCHAR(45),
    user_agent TEXT,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE web_login_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid,
    event_type VARCHAR(50) NOT NULL CHECK (event_type IN ('login_succeeded', 'login_failed', 'logout_succeeded')),
    user_id UUID,
    username VARCHAR(100) NOT NULL,
    session_id UUID,
    client_ip VARCHAR(45),
    user_agent TEXT,
    result VARCHAR(50) NOT NULL,
    failure_reason VARCHAR(255),
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE web_operation_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid,
    user_id UUID,
    username VARCHAR(100),
    module VARCHAR(100) NOT NULL,
    resource_type VARCHAR(100),
    resource_id VARCHAR(255),
    action VARCHAR(100) NOT NULL,
    result VARCHAR(50) NOT NULL,
    request_id VARCHAR(255),
    client_ip VARCHAR(45),
    user_agent TEXT,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Tasks, runs, leases, task history, task events, task artifacts, audit events, login logs, and operation logs intentionally keep UUID reference columns without database FK constraints in this batch. The service layer validates tenant/team/resource consistency, while lifecycle timestamps and immutable log rows preserve history across soft deletion, archival, disablement, and cancellation.

Keep `target_node_id` and `assigned_node_id` as node business keys for compatibility with the current Runtime HTTP contract. The UUID identity of a Runtime node lives in `runtime_nodes.id`; the external Runtime contract continues to authenticate by `X-Node-ID`.

Apply the `update_updated_at_column()` trigger to every table that has `updated_at`, including `task_runs`. This is required because `UpdateTaskRun` sets `updated_at = NOW()`.

- [ ] **Step 2: Update `002_seed_dev_admin.sql`**

Replace the seed with deterministic default tenant/team/admin inserts:

```sql
INSERT INTO tenants (id, slug, name, status)
VALUES (
    '00000000-0000-0000-0000-000000000001'::uuid,
    'default',
    '默认租户',
    'active'
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO tenant_teams (id, tenant_id, slug, name, status)
VALUES (
    '00000000-0000-0000-0000-000000000101'::uuid,
    '00000000-0000-0000-0000-000000000001'::uuid,
    'default',
    '默认团队',
    'active'
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO auth_users (username, display_name, email, password_hash, status)
VALUES (
    'admin',
    '开发管理员',
    'admin@superteam.local',
    '$2b$10$80xO1fy8PgNgH3qmmysLLOYe3RcHh3qVJs17hGbSqltjIJP7lNpfC',
    'active'
)
ON CONFLICT (username) DO NOTHING;

INSERT INTO tenant_members (tenant_id, principal_type, principal_id, role, status)
SELECT
    '00000000-0000-0000-0000-000000000001'::uuid,
    'user',
    id,
    'owner',
    'active'
FROM auth_users
WHERE username = 'admin'
ON CONFLICT DO NOTHING;
```

Keep the seed comments documenting the development credential:

```sql
-- Username: admin
-- Password: admin
```

The bcrypt hash must remain the existing `$2b$` hash for plaintext password `admin`; do not replace it with an unverified hash.

- [ ] **Step 3: Delete merged forward migrations**

```bash
git rm apps/control-plane/internal/storage/migrations/003_create_auth_sessions.sql
git rm apps/control-plane/internal/storage/migrations/004_create_web_logs.sql
git rm apps/control-plane/internal/storage/migrations/005_comment_auth_users_and_web_operation_logs.sql
```

- [ ] **Step 4: Regenerate Atlas migration checksums**

Atlas validates migration contents against `atlas.sum`. Because this task rewrites `001_initial.sql` and removes `003`-`005`, regenerate the checksum file:

```bash
cd apps/control-plane
rm -f internal/storage/migrations/atlas.sum
atlas migrate hash --dir file://internal/storage/migrations
```

Expected: `internal/storage/migrations/atlas.sum` is recreated and contains only the remaining migration files.

- [ ] **Step 5: Run schema contract tests**

Run:

```bash
cd apps/control-plane
rg -n "func TestDevAdminSeedMigrationIsIdempotentAndUsesBcrypt" internal/storage/migrations_test.go
rg -n "func TestDevAdminSeedMigrationIsIdempotentAndUsesBcrypt" internal/storage/migrations_test.go \
  && go test -v ./internal/storage -run 'TestInitialSchemaIsUUIDFirst|TestForwardOnlyAuthAndWebLogMigrationsWereMergedIntoInitialSchema|TestDevAdminSeedMigrationIsIdempotentAndUsesBcrypt' -count=1
```

Expected: the `rg` command finds the pre-existing bcrypt seed test, and all three named tests PASS.

- [ ] **Step 6: Commit schema rewrite**

```bash
git add apps/control-plane/internal/storage/migrations apps/control-plane/internal/storage/migrations_test.go
git commit -m "feat: rewrite initial schema for uuid-first rebuild"
```

---

## Task 3: Update sqlc UUID Mapping and Query SQL

**Files:**
- Modify: `apps/control-plane/sqlc.yaml`
- Modify: `apps/control-plane/internal/storage/queries/auth.sql`
- Modify: `apps/control-plane/internal/storage/queries/tasks.sql`
- Modify: `apps/control-plane/internal/storage/queries/runtime.sql`
- Modify: `apps/control-plane/internal/storage/queries/audit.sql`
- Modify: `apps/control-plane/internal/storage/queries/web_logs.sql`

- [ ] **Step 1: Update sqlc UUID overrides**

In `apps/control-plane/sqlc.yaml`, add these overrides under `gen.go.overrides`:

```yaml
          - db_type: "uuid"
            go_type:
              import: "github.com/google/uuid"
              type: "UUID"
          - db_type: "uuid"
            nullable: true
            go_type:
              import: "github.com/google/uuid"
              type: "NullUUID"
```

Keep the existing `auth_users.password_hash` override.

- [ ] **Step 2: Update `auth.sql`**

Make these concrete changes:

- `GetUser`, `UpdateUser`, `DeleteUser`, `UpdateUserPassword`, `GetUserByID` continue to filter by `id = $1`; sqlc will infer UUID from schema.
- `CreateSession` must stop inserting `id`; database default generates it.
- `CreateSession` inserts `user_id`, `token_hash`, `expires_at`, `last_seen_at`, `client_ip`, `user_agent`.
- Session token queries still filter by `token_hash`.
- This is an intentional architecture decision: `auth_sessions.id` is an internal UUID for DB relations and audit references; the raw cookie token is the authentication credential and only its SHA-256 hash is persisted in `auth_sessions.token_hash`. Do not put the raw token into `auth_sessions.id`.

Replacement for `CreateSession`:

```sql
-- name: CreateSession :one
INSERT INTO auth_sessions (
    user_id,
    token_hash,
    expires_at,
    last_seen_at,
    client_ip,
    user_agent
) VALUES (
    sqlc.arg('user_id')::uuid,
    sqlc.arg('token_hash')::varchar,
    sqlc.arg('expires_at'),
    sqlc.arg('last_seen_at'),
    sqlc.narg('client_ip')::varchar,
    sqlc.narg('user_agent')::text
) RETURNING *;
```

- [ ] **Step 3: Update `tasks.sql`**

Make these concrete changes:

- `creator_id` filter casts to `::uuid`, not `::bigint`.
- Add explicit tenant awareness to task queries:
  - `CreateTask` inserts `tenant_id` with `COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid)`.
  - `ListTasks` filters by `tenant_id`; if no tenant is provided, it defaults to the same default tenant.
  - `ListPendingTasks` also filters by `tenant_id` with the same default.
- Rename execution queries to run queries:
  - `CreateTaskExecution` -> `CreateTaskRun`
  - `UpdateTaskExecution` -> `UpdateTaskRun`
  - `GetTaskExecution` -> `GetTaskRun`
  - `ListTaskExecutions` -> `ListTaskRuns`
  - `GetLatestTaskExecution` -> `GetLatestTaskRun`
- Insert into and select from `task_runs`, not `task_executions`.
- `task_events` uses `run_id`, not `execution_id`.

The event insert must be:

```sql
-- name: CreateTaskEvent :one
INSERT INTO task_events (
    tenant_id,
    task_id,
    run_id,
    event_type,
    sequence_number,
    payload
) VALUES (
    COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid),
    sqlc.arg('task_id')::uuid,
    sqlc.narg('run_id')::uuid,
    sqlc.arg('event_type')::varchar,
    sqlc.arg('sequence_number')::integer,
    sqlc.arg('payload')::jsonb
) RETURNING *;
```

The `CreateTask` and `ListTasks` statements must be rewritten as:

```sql
-- name: CreateTask :one
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
    params
) VALUES (
    COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid),
    sqlc.narg('team_id')::uuid,
    sqlc.arg('title')::varchar,
    sqlc.narg('description')::text,
    sqlc.arg('status')::varchar,
    sqlc.arg('priority')::integer,
    sqlc.arg('provider_type')::varchar,
    sqlc.narg('creator_id')::uuid,
    sqlc.narg('target_node_id')::varchar,
    sqlc.narg('workspace_path')::text,
    COALESCE(sqlc.arg('params')::jsonb, '{}'::jsonb)
) RETURNING *;

-- name: ListTasks :many
SELECT * FROM tasks
WHERE tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
  AND deleted_at IS NULL
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)
  AND (sqlc.narg('creator_id')::uuid IS NULL OR creator_id = sqlc.narg('creator_id')::uuid)
  AND (sqlc.narg('provider_type')::varchar IS NULL OR provider_type = sqlc.narg('provider_type')::varchar)
ORDER BY priority DESC, created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
```

The `ListPendingTasks` statement must also be tenant-scoped:

```sql
-- name: ListPendingTasks :many
SELECT * FROM tasks
WHERE tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
  AND deleted_at IS NULL
  AND status = 'pending'
  AND (target_node_id IS NULL OR target_node_id = sqlc.narg('target_node_id')::varchar)
ORDER BY priority DESC, created_at ASC
LIMIT sqlc.arg('limit');
```

The `UpdateTaskRun` statement must either avoid `updated_at` or the DDL must include it. This plan requires the DDL column, so use:

```sql
-- name: UpdateTaskRun :one
UPDATE task_runs
SET status = sqlc.arg('status')::varchar,
    completed_at = NOW(),
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
RETURNING *;
```

- [ ] **Step 4: Update `web_logs.sql`**

Replace the create statements with complete UUID-aware SQL. For login logs:

```sql
-- name: CreateWebLoginLog :one
INSERT INTO web_login_logs (
    tenant_id,
    event_type,
    user_id,
    username,
    session_id,
    client_ip,
    user_agent,
    result,
    failure_reason,
    details
) VALUES (
    COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid),
    sqlc.arg('event_type')::varchar,
    sqlc.narg('user_id')::uuid,
    sqlc.arg('username')::varchar,
    sqlc.narg('session_id')::uuid,
    sqlc.narg('client_ip')::varchar,
    sqlc.narg('user_agent')::text,
    sqlc.arg('result')::varchar,
    sqlc.narg('failure_reason')::varchar,
    COALESCE(sqlc.narg('details')::jsonb, '{}'::jsonb)
) RETURNING *;
```

For operation logs:

```sql
-- name: CreateWebOperationLog :one
INSERT INTO web_operation_logs (
    tenant_id,
    user_id,
    username,
    module,
    resource_type,
    resource_id,
    action,
    result,
    request_id,
    client_ip,
    user_agent,
    details
) VALUES (
    COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid),
    sqlc.narg('user_id')::uuid,
    sqlc.narg('username')::varchar,
    sqlc.arg('module')::varchar,
    sqlc.narg('resource_type')::varchar,
    sqlc.narg('resource_id')::varchar,
    sqlc.arg('action')::varchar,
    sqlc.arg('result')::varchar,
    sqlc.narg('request_id')::varchar,
    sqlc.narg('client_ip')::varchar,
    sqlc.narg('user_agent')::text,
    COALESCE(sqlc.narg('details')::jsonb, '{}'::jsonb)
) RETURNING *;
```

Keep `resource_id` as text for audit display compatibility; service will store UUID strings there. `session_id` is UUID because it points to the internal `auth_sessions.id`, not the raw cookie token.

- [ ] **Step 5: Update `audit.sql`**

`audit_events.id` becomes UUID automatically. Keep `actor_id` and `resource_id` as text for polymorphic actor/resource references in this batch.

- [ ] **Step 6: Run sqlc and confirm generated UUID types**

Run:

```bash
make -C apps/control-plane generate-sqlc
rg -n "type Task struct|ID +uuid.UUID|CreatorID +uuid.NullUUID|type AuthUser struct|type TaskEvent struct" apps/control-plane/internal/storage/queries/models.go
```

Expected:

- `Task.ID uuid.UUID`
- `ListTasksParams.TenantID uuid.NullUUID`
- `Task.CreatorID uuid.NullUUID`
- `AuthUser.ID uuid.UUID`
- `TaskEvent.ID uuid.UUID`
- `TaskEvent.TenantID uuid.UUID`
- no `pgtype.Int8` for internal IDs.

- [ ] **Step 7: Commit sqlc query changes**

```bash
git add apps/control-plane/sqlc.yaml apps/control-plane/internal/storage/queries
git commit -m "feat: generate storage queries with uuid ids"
```

---

## Task 4: Convert Auth Domain and Web Auth API to UUID

**Files:**
- Modify: `apps/control-plane/internal/auth/types.go`
- Modify: `apps/control-plane/internal/auth/models.go`
- Modify: `apps/control-plane/internal/auth/service.go`
- Modify: `apps/control-plane/internal/auth/pg_repository.go`
- Modify: `apps/control-plane/internal/auth/handler.go`
- Modify: `apps/control-plane/internal/auth/service_test.go`
- Modify: `contracts/control-plane/auth.yaml`
- Regenerate: `apps/control-plane/internal/auth/generated.go`

- [ ] **Step 1: Update auth OpenAPI IDs**

In `contracts/control-plane/auth.yaml`, change user and login-log IDs:

```yaml
schema:
  type: string
  format: uuid
```

Apply this to:

- `/api/auth/users/{id}/status` path parameter.
- `/api/auth/users/{id}/reset-password` path parameter.
- `UserSummary.id`.
- `LoginLogRecord.id`.
- `LoginLogRecord.user_id`.

- [ ] **Step 2: Regenerate auth OpenAPI code**

Run:

```bash
make -C apps/control-plane generate-openapi
```

Expected: `apps/control-plane/internal/auth/generated.go` path IDs and response IDs use `openapi_types.UUID` or `uuid.UUID` depending on oapi-codegen output.

- [ ] **Step 3: Update auth domain types**

Change `apps/control-plane/internal/auth/types.go`:

```go
import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID `db:"id"`
	Username     string    `db:"username"`
	PasswordHash string    `db:"password_hash"`
	Status       string    `db:"status"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
	LastLoginAt  *time.Time `db:"last_login_at"`
}

type Session struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	ExpiresAt  time.Time `json:"expires_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ClientIP   string    `json:"client_ip"`
	UserAgent  string    `json:"user_agent"`
}

type Actor struct {
	UserID   uuid.UUID
	Username string
}

type LoginLog struct {
	ID            uuid.UUID
	EventType     string
	UserID        *uuid.UUID
	Username      string
	SessionID     *uuid.UUID
	ClientIP      string
	UserAgent     string
	Result        string
	FailureReason string
	CreatedAt     time.Time
}

type CreateLoginLogParams struct {
	EventType     string
	UserID        *uuid.UUID
	Username      string
	SessionID     *uuid.UUID
	ClientIP      string
	UserAgent     string
	Result        string
	FailureReason string
}

type CreateOperationLogParams struct {
	UserID       *uuid.UUID
	Username     string
	Module       string
	ResourceType string
	ResourceID   string
	Action       string
	Result       string
	ClientIP     string
	UserAgent    string
}
```

- [ ] **Step 4: Update auth service signatures**

Change repository and service signatures from `int64` to `uuid.UUID`:

```go
GetUserByID(ctx context.Context, id uuid.UUID) (*User, error)
UpdateUserStatus(ctx context.Context, userID uuid.UUID, status string) (*User, error)
UpdateUserPassword(ctx context.Context, userID uuid.UUID, passwordHash string) (*User, error)
CreateSession(ctx context.Context, userID uuid.UUID, clientIP, userAgent string) (*Session, string, error)
```

Replace `strconv.FormatInt(userID, 10)` with:

```go
resourceID := ""
if userID != uuid.Nil {
	resourceID = userID.String()
}
```

Do not generate session IDs in `CreateSession`; let the repository mutate the returned `session.ID` from the `CreateSession` SQL result.

- [ ] **Step 5: Update auth repository UUID conversions**

Use sqlc UUID types directly. For nullable UUID query params generated as `uuid.NullUUID`, create this helper in `apps/control-plane/internal/auth/pg_repository.go`:

```go
func nullUUID(value *uuid.UUID) uuid.NullUUID {
	if value == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *value, Valid: true}
}
```

When `CreateSession` returns a row, assign generated DB ID back to the domain session:

```go
created, err := r.q.CreateSession(ctx, queries.CreateSessionParams{
	UserID: session.UserID,
	TokenHash: tokenHash,
	ExpiresAt: pgtype.Timestamptz{Time: session.ExpiresAt, Valid: true},
	LastSeenAt: pgtype.Timestamptz{Time: session.LastSeenAt, Valid: true},
	ClientIp: pgtype.Text{String: session.ClientIP, Valid: session.ClientIP != ""},
	UserAgent: pgtype.Text{String: session.UserAgent, Valid: session.UserAgent != ""},
})
if err != nil {
	return err
}
session.ID = created.ID
return nil
```

- [ ] **Step 6: Update handler generated type conversions**

If `generated.go` uses `openapi_types.UUID`, convert from `uuid.UUID` with direct assignment where the type alias permits it; otherwise cast using the generated type name.

`toGeneratedUserSummary` must return UUID, not int64:

```go
func toGeneratedUserSummary(user *User) UserSummary {
	return UserSummary{
		Id:       user.ID,
		Status:   UserSummaryStatus(user.Status),
		Username: user.Username,
	}
}
```

For optional login-log user/session IDs:

```go
func optionalUUID(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	return value
}
```

- [ ] **Step 7: Run auth tests and fix compile errors**

Run:

```bash
go test ./apps/control-plane/internal/auth -count=1
go test ./apps/control-plane/internal/api -run Auth -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit auth UUID conversion**

```bash
git add contracts/control-plane/auth.yaml apps/control-plane/internal/auth
git commit -m "feat: convert auth domain to uuid ids"
```

---

## Task 5: Convert Task Domain, Runtime Handler, and Task API to UUID

**Files:**
- Modify: `contracts/control-plane/openapi.yaml`
- Modify: `apps/control-plane/internal/task/models.go`
- Modify: `apps/control-plane/internal/task/repository.go`
- Modify: `apps/control-plane/internal/task/service.go`
- Modify: `apps/control-plane/internal/task/pg_repository.go`
- Modify: `apps/control-plane/internal/api/handlers/task.go`
- Modify: `apps/control-plane/internal/api/handlers/runtime.go`
- Modify: `apps/control-plane/internal/api/handlers/responses.go`
- Modify: `apps/control-plane/internal/task/service_test.go`
- Modify: `apps/control-plane/internal/api/handlers/runtime_test.go`
- Modify: `apps/control-plane/internal/runtime/poller_test.go`

- [ ] **Step 1: Update OpenAPI task IDs**

In `contracts/control-plane/openapi.yaml`, change:

```yaml
TaskId:
  name: taskId
  in: path
  required: true
  schema:
    type: string
    format: uuid
```

Change `Task.id` and `Task.creator_id` to:

```yaml
type: string
format: uuid
```

- [ ] **Step 2: Regenerate Control Plane API code**

Run:

```bash
pnpm generate:control-plane
```

Expected: generated Control Plane API types use UUID strings for task IDs.

- [ ] **Step 3: Update task domain models**

In `apps/control-plane/internal/task/models.go`, import `github.com/google/uuid` and change ID fields:

```go
type Task struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	TeamID         *uuid.UUID
	Title          string
	Description    *string
	CreatorID      *uuid.UUID
	ProviderType   string
	TargetNodeID   *string
	AssignedNodeID *string
	Status         TaskStatus
	WorkspacePath  *string
	Params         []byte
	Priority       int32
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type TaskEvent struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	TaskID         uuid.UUID
	RunID          *uuid.UUID
	EventType      string
	SequenceNumber int32
	Payload        []byte
	CreatedAt      time.Time
}
```

Update request and filter structs so tenant awareness reaches SQL instead of remaining DDL-only:

```go
type CreateTaskRequest struct {
	TenantID      *uuid.UUID
	TeamID        *uuid.UUID
	Title         string
	Description   *string
	CreatorID     *uuid.UUID
	ProviderType  string
	TargetNodeID  *string
	WorkspacePath *string
	Params        []byte
	Priority      int32
}

type ListTasksFilter struct {
	TenantID     *uuid.UUID
	Status       *TaskStatus
	CreatorID    *uuid.UUID
	ProviderType *string
	Limit        int32
	Offset       int32
}
```

Replace `int8FromInt64` and `int64FromInt8` with:

```go
func nullUUIDFromPtr(value *uuid.UUID) uuid.NullUUID {
	if value == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *value, Valid: true}
}

func ptrFromNullUUID(value uuid.NullUUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	return &value.UUID
}
```

- [ ] **Step 4: Update repository interfaces**

In `apps/control-plane/internal/task/repository.go`, change all task/run/event IDs to UUID:

```go
GetTask(ctx context.Context, id uuid.UUID) (TaskRecord, error)
DeleteTask(ctx context.Context, id uuid.UUID) error
GetLatestTaskEventSequence(ctx context.Context, taskID uuid.UUID) (int32, error)
```

Change record fields:

```go
ID        uuid.UUID
TenantID  uuid.UUID
TeamID    uuid.NullUUID
CreatorID uuid.NullUUID
TaskID    uuid.UUID
RunID     uuid.NullUUID
```

Also add `TenantID uuid.UUID` and `TeamID uuid.NullUUID` to `CreateTaskParams`, `ListTasksParams`, `ListPendingTasksParams`, `CreateTaskEventParams`, `TaskRecord`, `TaskEventRecord`, and any sqlc wrapper structs. `Service.CreateTask`, `Service.ListTasks`, runtime claim, and task event creation must pass these values into repository params.

Tenant source rule for this batch:

- Web/API request bodies and query parameters must not expose arbitrary `tenant_id` yet.
- Handlers pass nil tenant/team values unless the value is already available from trusted server-side context.
- Services/repositories normalize nil tenant to the seeded default tenant UUID.
- A later tenant-aware auth middleware plan will replace the default with a trusted tenant from session membership.

- [ ] **Step 5: Update task service validation**

Replace checks like `req.TaskID <= 0` with UUID nil checks:

```go
if req.TaskID == uuid.Nil {
	return nil, errors.New("task_id is required")
}
```

Change service methods:

```go
GetTask(ctx context.Context, taskID uuid.UUID) (*Task, error)
CancelTask(ctx context.Context, taskID uuid.UUID, cancelledBy *string, reason *string) (*Task, error)
```

- [ ] **Step 6: Update task and runtime handlers**

Replace `strconv.ParseInt` path parsing with UUID parsing:

```go
func taskIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return id, true
}
```

Keep `strconv.Atoi` for pagination and timeout parsing.

Change `taskResponse`:

```go
type taskResponse struct {
	ID             string          `json:"id"`
	TenantID       string          `json:"tenant_id"`
	TeamID         *string         `json:"team_id,omitempty"`
	Title          string          `json:"title"`
	Description    *string         `json:"description,omitempty"`
	CreatorID      *string         `json:"creator_id,omitempty"`
	ProviderType   string          `json:"provider_type"`
	TargetNodeID   *string         `json:"target_node_id,omitempty"`
	AssignedNodeID *string         `json:"assigned_node_id,omitempty"`
	Status         task.TaskStatus `json:"status"`
	WorkspacePath  *string         `json:"workspace_path,omitempty"`
	Params         json.RawMessage `json:"params"`
	Priority       int32           `json:"priority"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
}
```

Use a helper:

```go
func optionalUUIDString(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}
```

- [ ] **Step 7: Run task and handler tests**

Run:

```bash
go test ./apps/control-plane/internal/task -count=1
go test ./apps/control-plane/internal/api/handlers -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit task UUID conversion**

```bash
git add contracts/control-plane/openapi.yaml apps/control-plane/internal/task apps/control-plane/internal/api/handlers apps/control-plane/internal/runtime
git commit -m "feat: convert task api and runtime handlers to uuid ids"
```

---

## Task 6: Convert Runtime and Audit Domain IDs to UUID

**Files:**
- Modify: `apps/control-plane/internal/runtime/models.go`
- Modify: `apps/control-plane/internal/runtime/repository.go`
- Modify: `apps/control-plane/internal/runtime/service.go`
- Modify: `apps/control-plane/internal/runtime/pg_repository.go`
- Modify: `apps/control-plane/internal/runtime/service_test.go`
- Modify: `apps/control-plane/internal/runtime/scheduler_test.go`
- Modify: `apps/control-plane/internal/audit/service.go`
- Modify: `apps/control-plane/internal/audit/service_test.go`

- [ ] **Step 1: Update Runtime node record IDs**

In runtime models and repository records:

```go
import "github.com/google/uuid"

type Node struct {
	ID                 uuid.UUID
	NodeID             string
	Name               string
	SupportedProviders []string
	MaxSlots           int32
	CurrentLoad        int32
	Status             NodeStatus
	Metadata           map[string]interface{}
	LastHeartbeatAt    time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type NodeRecord struct {
	ID                 uuid.UUID
	NodeID             string
	Name               string
	SupportedProviders []byte
	MaxSlots           int32
	CurrentLoad        int32
	Status             string
	Metadata           []byte
	LastHeartbeatAt    pgtype.Timestamptz
	CreatedAt          pgtype.Timestamptz
	UpdatedAt          pgtype.Timestamptz
}
```

Remove unused runtime helpers that convert `pgtype.Int8`.

- [ ] **Step 2: Update audit event ID**

In `apps/control-plane/internal/audit/service.go`:

```go
type Event struct {
	ID           uuid.UUID `db:"id"`
	EventType    string    `db:"event_type"`
	ActorType    string    `db:"actor_type"`
	ActorID      string    `db:"actor_id"`
	ResourceType string    `db:"resource_type"`
	ResourceID   string    `db:"resource_id"`
	Action       string    `db:"action"`
	Details      []byte    `db:"details"`
	IPAddress    string    `db:"ip_address"`
	CreatedAt    time.Time `db:"created_at"`
}
```

Update audit tests to use `uuid.New()` instead of `int64(len(...)+1)`.

- [ ] **Step 3: Run runtime and audit tests**

Run:

```bash
go test ./apps/control-plane/internal/runtime -count=1
go test ./apps/control-plane/internal/audit -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit runtime and audit UUID conversion**

```bash
git add apps/control-plane/internal/runtime apps/control-plane/internal/audit
git commit -m "feat: convert runtime and audit ids to uuid"
```

---

## Task 7: Update Storage Integration Tests for UUID Schema

**Files:**
- Modify: `apps/control-plane/internal/storage/queries/queries_test.go`

- [ ] **Step 1: Update cleanup for rebuild schema**

Replace cleanup SQL with UUID-safe truncation:

```go
_, err := db.Exec(ctx, `
	TRUNCATE
		web_operation_logs,
		web_login_logs,
		audit_events,
		task_artifacts,
		task_events,
		task_state_history,
		runtime_leases,
		task_runs,
		tasks,
		auth_sessions,
		auth_runtime_tokens,
		runtime_node_scopes,
		runtime_nodes,
		tenant_members,
		auth_users,
		tenant_teams,
		tenant_profiles,
		tenants
	RESTART IDENTITY CASCADE
`)
```

After cleanup, insert default tenant/team by calling a helper:

```go
func seedDefaultTenant(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ('00000000-0000-0000-0000-000000000001'::uuid, 'default', '默认租户', 'active')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO tenant_teams (id, tenant_id, slug, name, status)
		VALUES (
			'00000000-0000-0000-0000-000000000101'::uuid,
			'00000000-0000-0000-0000-000000000001'::uuid,
			'default',
			'默认团队',
			'active'
		)
		ON CONFLICT (id) DO NOTHING;
	`)
	require.NoError(t, err)
}
```

- [ ] **Step 2: Replace pgtype.Int8 ID usage**

Use `uuid.NullUUID` for nullable IDs:

```go
CreatorID: uuid.NullUUID{UUID: user.ID, Valid: true},
UserID: uuid.NullUUID{UUID: user.ID, Valid: true},
```

Use `uuid.NullUUID{}` for absent optional IDs.

- [ ] **Step 3: Update session log assertions**

When asserting JSON or logs, compare UUIDs directly:

```go
assert.Equal(t, user.ID, success.UserID.UUID)
assert.True(t, success.UserID.Valid)
```

- [ ] **Step 4: Run storage query tests with explicit environment**

Run without DB env first:

```bash
go test ./apps/control-plane/internal/storage/queries -count=1
```

Expected: PASS with skip message when `TEST_DATABASE_URL` and `TEST_REDIS_URL` are not set.

Run with test DB env when available:

```bash
cd apps/control-plane
TEST_DATABASE_URL="$TEST_DATABASE_URL" TEST_REDIS_URL="$TEST_REDIS_URL" go test ./internal/storage/queries -count=1
```

Expected: PASS against a fresh or rebuilt schema.

- [ ] **Step 5: Commit storage test updates**

```bash
git add apps/control-plane/internal/storage/queries/queries_test.go
git commit -m "test: update storage integration tests for uuid schema"
```

---

## Task 8: Update Web API Client Types and Tests

**Files:**
- Modify: `apps/web/src/lib/api/tasks.ts`
- Modify: `apps/web/src/lib/api/tasks.test.ts`
- Modify: `apps/web/src/lib/api/auth.ts`
- Modify: `apps/web/src/lib/api/auth.test.ts`

- [ ] **Step 1: Update task client ID types**

In `apps/web/src/lib/api/tasks.ts`, change number IDs to strings:

```ts
export interface TaskResponse {
  id: string
  tenant_id?: string
  team_id?: string
  title: string
  description?: string
  creator_id?: string
  provider_type: string
  target_node_id?: string
  assigned_node_id?: string
  status: TaskStatus
  workspace_path?: string
  params: Record<string, unknown>
  priority: number
  created_at?: string
  updated_at?: string
}

export async function getTask(options: ApiClientOptions, taskId: string): Promise<TaskResponse>
export async function updateTaskStatus(options: ApiClientOptions, taskId: string, status: TaskStatus): Promise<TaskResponse>
export async function cancelTask(options: ApiClientOptions, taskId: string): Promise<TaskResponse>
```

- [ ] **Step 2: Update auth client ID types**

In `apps/web/src/lib/api/auth.ts`, change user/log IDs to strings:

```ts
export interface AuthUser {
  id: string
  username: string
  status: 'active' | 'disabled'
}

export interface LoginLogRecord {
  id: string
  user_id?: string
  session_id?: string
  username: string
  event_type: 'login_succeeded' | 'login_failed' | 'logout_succeeded'
  result: 'succeeded' | 'failed'
  failure_reason?: string
  client_ip?: string
  user_agent?: string
  created_at: string
}
```

- [ ] **Step 3: Update frontend tests**

Use UUID string fixtures:

```ts
const taskId = '11111111-1111-4111-8111-111111111111'
const userId = '22222222-2222-4222-8222-222222222222'
const sessionId = '33333333-3333-4333-8333-333333333333'
```

Assertions should compare strings, not numbers.

- [ ] **Step 4: Run Web tests and typecheck**

Run:

```bash
pnpm --filter @superteam/web test
pnpm --filter @superteam/web typecheck
```

Expected: PASS.

- [ ] **Step 5: Commit Web UUID client changes**

```bash
git add apps/web/src/lib/api/tasks.ts apps/web/src/lib/api/tasks.test.ts apps/web/src/lib/api/auth.ts apps/web/src/lib/api/auth.test.ts
git commit -m "feat: use uuid strings in web api clients"
```

---

## Task 9: Rebuild Local Development Database and Verify Runtime Flow

**Files:**
- Modify or create: `docs/database/rebuild_uuid_schema.md`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Document rebuild commands**

Create `docs/database/rebuild_uuid_schema.md`:

```markdown
# UUID-first 数据库重建说明

本项目在早期阶段采用重写初始 schema 的方式引入 UUID 主键。执行以下命令会删除当前开发库数据，仅适用于确认没有保留价值的本地或远端开发环境。

## 本地重建

1. 确认当前连接信息：

```bash
sed -n '1,220p' doc/database/conn_info.md
```

2. 备份可选数据：

```bash
pg_dump "$DATABASE_URL" > /tmp/superteam-before-uuid-rebuild.sql
```

3. 删除并重建 schema：

```bash
psql "$DATABASE_URL" -c 'DROP SCHEMA IF EXISTS superteam CASCADE; CREATE SCHEMA superteam;'
```

4. 重新执行迁移：

```bash
make -C apps/control-plane migrate-up DATABASE_URL="$DATABASE_URL"
```

如果修改过迁移文件，先在 `apps/control-plane` 下重新生成 Atlas 校验和：

```bash
rm -f internal/storage/migrations/atlas.sum
atlas migrate hash --dir file://internal/storage/migrations
```

5. 检查 ID 类型：

```sql
SELECT table_name, column_name, data_type, column_default
FROM information_schema.columns
WHERE table_schema = current_schema()
  AND column_name = 'id'
ORDER BY table_name;
```

所有 SuperTeam 自有表的 `id` 都应是 `uuid`。
```

- [ ] **Step 2: Update CHANGELOG**

Add a dated entry:

```markdown
## 2026-06-01

- 重写 Control Plane 初始数据库 schema，所有 SuperTeam 自有表主键切换为 UUID，并启用 `gen_random_uuid()` 默认生成。
- 合并早期 auth session、Web 登录日志、操作日志和中文注释迁移，早期环境采用重建库策略。
- 增加租户与团队骨架，为多团队数字员工管理和分布式 Runtime claim/lease 链路打基础。
```

- [ ] **Step 3: Rebuild DB only after explicit environment confirmation**

Before executing destructive SQL, print the target DB:

```bash
psql "$DATABASE_URL" -c 'select current_user, current_database(), current_schema();'
```

Expected: output shows the intended development database and schema.

Then run the rebuild commands from the doc.

- [ ] **Step 4: Run end-to-end Control Plane verification**

Run:

```bash
make -C apps/control-plane generate-sqlc
pnpm generate:control-plane
go test ./apps/control-plane/...
pnpm --filter @superteam/web test
pnpm --filter @superteam/web typecheck
```

Expected: all commands PASS. Known unrelated failures must be recorded with exact package and error text before continuing.

- [ ] **Step 5: Verify ID columns in live DB**

Run:

```bash
psql "$DATABASE_URL" -Atc "
SELECT table_name || '|' || data_type || '|' || COALESCE(column_default, '')
FROM information_schema.columns
WHERE table_schema = current_schema()
  AND column_name = 'id'
ORDER BY table_name;
"
```

Expected: every SuperTeam self-owned table reports `uuid` and `gen_random_uuid()` in the default expression.

- [ ] **Step 6: Commit docs and verification updates**

```bash
git add docs/database/rebuild_uuid_schema.md CHANGELOG.md
git commit -m "docs: document uuid schema rebuild workflow"
```

---

## Task 10: Final Whole-Repo Verification

**Files:**
- No source changes expected.

- [ ] **Step 1: Run schema residue scan**

Run:

```bash
rg -n "BIGSERIAL PRIMARY KEY|user_id BIGINT|creator_id BIGINT|task_id BIGINT|execution_id BIGINT|format: int64|ParseInt\\(.*task|ID +int64|\\*int64" \
  apps/control-plane/internal contracts/control-plane apps/web/src/lib/api
```

Expected: no matches for internal ID usage. Matches for counts, pagination, TTL, file sizes, timestamps, or non-ID numeric values are acceptable and must be listed separately.

- [ ] **Step 2: Run full verification**

Run:

```bash
pnpm verify:foundation
```

Expected: PASS.

- [ ] **Step 3: Run git diff review**

Run:

```bash
git diff --stat
git diff -- apps/control-plane/internal/storage/migrations/001_initial.sql | sed -n '1,260p'
git diff -- contracts/control-plane/openapi.yaml contracts/control-plane/auth.yaml | sed -n '1,220p'
```

Expected:

- Migration diff shows UUID-first rebuild schema.
- OpenAPI diff shows UUID strings for internal IDs.
- No unrelated files are modified by hand.

- [ ] **Step 4: Commit final verification fixes**

If verification required small fixes:

```bash
git add <fixed-files>
git commit -m "fix: complete uuid schema verification"
```

If no fixes were needed, do not create an empty commit.

---

## Self-Review

Spec coverage:

- UUID-first schema: Tasks 1-3.
- Rebuild-only strategy: Tasks 2 and 9.
- Tenant/team foundation: Task 2.
- sqlc/OpenAPI/Go/Web ID conversion: Tasks 3-8.
- Runtime claim compatibility: Tasks 5-6.
- Verification and rebuild docs: Tasks 9-10.

Known implementation pressure points:

- `task_executions` to `task_runs` is a real rename. Update SQL, repository names, service names, and tests in the same task.
- `auth_sessions.id` becomes DB-generated UUID. Keep the repository method mutating `session.ID` after insert so service shape changes stay small.
- `node_id` remains the Runtime external business key. Do not force Runtime HTTP callers to know `runtime_nodes.id` in this batch.
- `tenant_id` defaults to the seeded default tenant to keep current unauthenticated task routes working until tenant-aware auth middleware exists.

Review fixes incorporated from implementation audit:

- `task_runs.updated_at` is required in DDL because `UpdateTaskRun` writes it.
- `atlas.sum` must be regenerated after rewriting `001_initial.sql` and deleting merged migrations.
- `auth_sessions.id` and cookie token are intentionally separated: UUID for DB/audit relations, raw token only for cookie credential, persisted only through `token_hash`.
- Task SQL must pass and filter `tenant_id` in `CreateTask`, `ListTasks`, `ListPendingTasks`, task events, task runs, artifacts, state history, and all task-scoped read/update/delete queries, not just define tenant columns in DDL.
- `creator_id` and other ID filters use `::uuid`; no `::bigint` casts remain for internal IDs.
- The dev admin bcrypt seed keeps the existing `$2b$` hash for plaintext password `admin`.
- `web_logs.sql` changes are specified with complete `CreateWebLoginLog` and `CreateWebOperationLog` SQL, including UUID `user_id` and `session_id`.
- `pgcrypto` is not required for PostgreSQL 13+ UUID generation; the initial schema should rely on built-in `gen_random_uuid()`.
- The schema contract test checks merged auth/web-log DDL and comments, not only deletion of migration files.
- DB-backed schema contract verification is required when `TEST_DATABASE_URL` is configured so comments and column types are checked after executing the migration, not only through string matching.
- Current API handlers do not accept caller-provided `tenant_id`; they pass nil/default tenant until trusted tenant context exists.
- Intermediate failing-test commits are optional when the execution environment disallows them; the red-green step itself is not optional.
