# Runtime Enrollment and Digital Employee Execution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 Runtime 自发现接入、短期 Runtime Session、数字员工唯一执行实例、Provider Session 事件回传和对应 Web 管理入口。

**Architecture:** Control Plane 负责 Runtime 接入审批、短期会话、数字员工业务身份、执行实例和事件持久化；Runtime Agent 只作为客户侧执行宿主管理 Provider、workspace、session 和事件回传。Provider 输出固定经过 `Provider -> Runtime Agent -> Control Plane -> Web`，Web 不直接连接 Runtime 或 Provider。

**Tech Stack:** Go 1.25, chi/net/http, pgx/v5, sqlc, PostgreSQL, oapi-codegen, nhooyr.io/websocket, Rust Tokio, reqwest, tokio-tungstenite, serde, React 19, Vite, TanStack Router, TanStack Query, shadcn/ui, Vitest.

---

## Scope

本计划实现 spec `docs/superpowers/specs/2026-06-02-runtime-enrollment-digital-employee-execution-design.md` 的 MVP 主链路：

- Runtime 使用环境级 `bootstrap_key` 进入 pending enrollment。
- Web 管理员审批 Runtime 接入。
- Runtime 获取短期 session token，并自动续期。
- Runtime 主动建立 outbound WebSocket，HTTP polling 作为降级接口保留。
- Runtime 上报 provider、workspace、capacity capability。
- 数字员工支持 `draft | ready | active | disabled | error`。
- 一个数字员工最多绑定一个 execution instance。
- Control Plane 下发 `ensure_instance`、`start_session`、`resume_session`、`send_input`。
- Runtime Agent 管理 Claude Code / OpenCode Provider Session。
- Provider Session 事件由 Runtime Agent 回传 Control Plane，Web 从 Control Plane 展示。

本计划不实现：

- 一个数字员工绑定多个 execution instance。
- 自动调度、跨 Runtime 迁移、实例池、复杂 fallback。
- Runtime 自动扫描客户机器 workspace。
- Provider 直接连接平台。
- 完整人类员工模型。

## File Structure

### Contracts

- Modify: `contracts/control-plane/openapi.yaml`
  - 增加 Runtime enrollment、approve/reject/revoke、session renew、capabilities、commands polling、digital employees、provider sessions API。
- Modify: `apps/control-plane/internal/api/generate.go`
  - 保持 generated server 使用聚合后的 `openapi.yaml`。
- Regenerate: `apps/control-plane/internal/api/gen/server.gen.go`
  - oapi-codegen 生成的 server/types。

### Database and SQL

- Modify: `apps/control-plane/internal/storage/migrations/001_initial.sql`
  - 增加 `runtime_bootstrap_keys`、`runtime_enrollments`、`runtime_sessions`、`runtime_capabilities`、`digital_employees`、`digital_employee_execution_instances`、`provider_sessions`、`provider_session_events`。
  - 保留 `runtime_node_scopes` 兼容现有权限中心，但不作为新执行权限来源。
- Create: `apps/control-plane/internal/storage/queries/runtime_enrollment.sql`
- Create: `apps/control-plane/internal/storage/queries/employee_execution.sql`
- Create: `apps/control-plane/internal/storage/queries/provider_session.sql`
- Regenerate: `apps/control-plane/internal/storage/queries/*.go`

### Control Plane Backend

- Modify: `apps/control-plane/internal/runtime/models.go`
  - 增加 enrollment、session、capability、command 领域类型。
- Modify: `apps/control-plane/internal/runtime/repository.go`
  - 增加 enrollment/session/capability/command repository 方法。
- Modify: `apps/control-plane/internal/runtime/pg_repository.go`
  - 实现新增 repository 方法。
- Modify: `apps/control-plane/internal/runtime/service.go`
  - 增加 `EnrollHello`、`ApproveEnrollment`、`RejectEnrollment`、`RevokeEnrollment`、`IssueRuntimeSession`、`RenewRuntimeSession`、`UpsertCapabilities`。
- Create: `apps/control-plane/internal/runtime/session_token.go`
  - 生成随机 session token，bcrypt hash 存库。
- Create: `apps/control-plane/internal/runtime/connection.go`
  - Runtime WebSocket connection registry 和 command dispatch。
- Modify: `apps/control-plane/internal/api/middleware/auth.go`
  - 新增 runtime session token 校验接口。
- Modify: `apps/control-plane/internal/api/handlers/runtime.go`
  - 增加 enrollment、approval、renew、capabilities、WebSocket、commands polling handlers。
- Modify: `apps/control-plane/internal/api/server.go`
  - 拆分 bootstrap enrollment route、runtime session protected route、Web user protected route。
- Modify: `apps/control-plane/internal/app/app.go`
  - 注入 runtime connection registry 和新的 service dependency。

### Digital Employee Backend

- Create: `apps/control-plane/internal/employee/types.go`
- Create: `apps/control-plane/internal/employee/repository.go`
- Create: `apps/control-plane/internal/employee/pg_repository.go`
- Create: `apps/control-plane/internal/employee/service.go`
- Create: `apps/control-plane/internal/employee/handler.go`
- Create: `apps/control-plane/internal/employee/service_test.go`
- Modify: `apps/control-plane/internal/api/server.go`
  - 注册数字员工 API。
- Modify: `apps/control-plane/internal/app/app.go`
  - 构造 employee repository/service/handler。

### Runtime Agent

- Modify: `apps/runtime-agent/src/config.rs`
  - 将长期 `auth_token` 替换为 `bootstrap_key`，保留 env override。
- Modify: `apps/runtime-agent/config.example.yaml`
- Modify: `apps/runtime-agent/config.yaml`
  - 本地配置使用 `bootstrap_key`。
- Modify: `apps/runtime-agent/src/controlplane/models.rs`
  - 增加 enrollment/session/capability/command/provider session models。
- Modify: `apps/runtime-agent/src/controlplane/client.rs`
  - 增加 enroll hello、renew session、capabilities、events、command ack API。
- Create: `apps/runtime-agent/src/session.rs`
  - Runtime session token 内存状态和续期判断。
- Create: `apps/runtime-agent/src/controlplane/ws.rs`
  - outbound WebSocket command loop。
- Create: `apps/runtime-agent/src/instances.rs`
  - ensure_instance、agent_home_dir 管理。
- Modify: `apps/runtime-agent/src/daemon.rs`
  - 启动流程从 register 改为 enroll -> session -> ws -> heartbeat/capability。
- Modify: `apps/runtime-agent/src/providers/claude.rs`
  - 保留 session_id/resume 行为，确保事件携带 provider session。
- Modify: `apps/runtime-agent/src/providers/opencode.rs`
  - 同步 session 行为。

### Web

- Modify: `apps/web/src/lib/api/runtime.ts`
  - 增加 enrollment、approval、capabilities 类型和 API client。
- Create: `apps/web/src/lib/api/employees.ts`
  - 数字员工和 execution instance API client。
- Modify: `apps/web/src/lib/api/index.ts`
  - 导出 employees client。
- Create: `apps/web/src/features/runtime/index.tsx`
- Create: `apps/web/src/features/runtime/index.test.tsx`
- Modify: `apps/web/src/routes/_authenticated/runtime/index.tsx`
  - 使用真实 Runtime 节点页面替换占位。
- Create: `apps/web/src/features/employees/index.tsx`
- Create: `apps/web/src/features/employees/index.test.tsx`
- Modify: `apps/web/src/routes/_authenticated/employees/index.tsx`
  - 使用真实数字员工页面替换占位。

### Docs

- Modify: `CHANGELOG.md`
  - 记录 Runtime 接入和数字员工执行实例变更。

---

## Task 1: Add Contract and Database Shape

**Files:**
- Modify: `contracts/control-plane/openapi.yaml`
- Modify: `apps/control-plane/internal/storage/migrations/001_initial.sql`
- Create: `apps/control-plane/internal/storage/queries/runtime_enrollment.sql`
- Create: `apps/control-plane/internal/storage/queries/employee_execution.sql`
- Create: `apps/control-plane/internal/storage/queries/provider_session.sql`
- Regenerate: `apps/control-plane/internal/storage/queries/*.go`
- Regenerate: `apps/control-plane/internal/api/gen/server.gen.go`

- [ ] **Step 1: Add failing storage query tests**

Append these tests to `apps/control-plane/internal/storage/queries/queries_test.go`:

```go
func TestRuntimeEnrollmentAndSessionQueries(t *testing.T) {
	ctx := context.Background()
	q := testQueries

	key, err := q.CreateRuntimeBootstrapKey(ctx, queries.CreateRuntimeBootstrapKeyParams{
		Name:    "customer-env",
		KeyHash: "$2a$10$abcdefghijklmnopqrstuvabcdefghijklmnoqrstuvwxyz123456",
		Status: "active",
	})
	require.NoError(t, err)

	enrollment, err := q.UpsertRuntimeEnrollment(ctx, queries.UpsertRuntimeEnrollmentParams{
		NodeID:         "customer-vm-01",
		TenantID:       defaultTenantID,
		BootstrapKeyID: key.ID,
		Status:         "pending",
		Metadata:       []byte(`{"hostname":"customer-vm-01"}`),
	})
	require.NoError(t, err)
	require.Equal(t, "pending", enrollment.Status)

	approved, err := q.UpdateRuntimeEnrollmentStatus(ctx, queries.UpdateRuntimeEnrollmentStatusParams{
		ID:     enrollment.ID,
		Status: "approved",
	})
	require.NoError(t, err)
	require.Equal(t, "approved", approved.Status)

	session, err := q.CreateRuntimeSession(ctx, queries.CreateRuntimeSessionParams{
		RuntimeNodeID: approved.RuntimeNodeID,
		TokenHash:     "$2a$10$sessionabcdefghijklmnopqrstuvabcdefghijklmno123",
		ExpiresAt:     pgtype.Timestamptz{Time: time.Now().UTC().Add(12 * time.Hour), Valid: true},
	})
	require.NoError(t, err)

	found, err := q.GetRuntimeSessionByTokenHash(ctx, session.TokenHash)
	require.NoError(t, err)
	require.Equal(t, session.ID, found.ID)
}

func TestDigitalEmployeeExecutionQueries(t *testing.T) {
	ctx := context.Background()
	q := testQueries

	node, err := q.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "employee-runtime-01",
		Name:               "employee runtime",
		SupportedProviders: []byte(`["claude-code"]`),
		MaxSlots:           2,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           []byte(`{}`),
		LastHeartbeatAt:    pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	require.NoError(t, err)

	employee, err := q.CreateDigitalEmployee(ctx, queries.CreateDigitalEmployeeParams{
		TenantID:    defaultTenantID,
		Name:        "前端开发员工一",
		Description: pgtype.Text{String: "负责前端开发", Valid: true},
		Status:      "draft",
		Config:      []byte(`{"role":"frontend"}`),
	})
	require.NoError(t, err)

	instance, err := q.UpsertDigitalEmployeeExecutionInstance(ctx, queries.UpsertDigitalEmployeeExecutionInstanceParams{
		DigitalEmployeeID: employee.ID,
		RuntimeNodeID:     node.ID,
		ProviderType:      "claude-code",
		AgentHomeDir:      "/data/superteam/workspaces/agents/" + employee.ID.String(),
		WorkspacePolicy:   []byte(`{"base_dir":"/data/superteam/workspaces"}`),
		SessionPolicy:     "reuse_latest",
		Status:            "ready",
	})
	require.NoError(t, err)
	require.Equal(t, employee.ID, instance.DigitalEmployeeID)
}
```

- [ ] **Step 2: Run query tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/storage/queries -run 'TestRuntimeEnrollmentAndSessionQueries|TestDigitalEmployeeExecutionQueries' -count=1
```

Expected: FAIL because sqlc query methods such as `CreateRuntimeBootstrapKey`, `UpsertRuntimeEnrollment`, and `CreateDigitalEmployee` do not exist.

- [ ] **Step 3: Add database tables**

In `apps/control-plane/internal/storage/migrations/001_initial.sql`, add these tables under the Runtime and Employee sections:

```sql
CREATE TABLE runtime_bootstrap_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid REFERENCES tenants(id),
    name VARCHAR(255) NOT NULL,
    key_hash VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE runtime_enrollments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid REFERENCES tenants(id),
    runtime_node_id UUID REFERENCES runtime_nodes(id),
    bootstrap_key_id UUID NOT NULL REFERENCES runtime_bootstrap_keys(id),
    node_id VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    approved_by UUID REFERENCES auth_users(id),
    approved_at TIMESTAMPTZ,
    rejected_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    status_reason TEXT,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, node_id)
);

CREATE TABLE runtime_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid REFERENCES tenants(id),
    runtime_node_id UUID NOT NULL REFERENCES runtime_nodes(id),
    token_hash VARCHAR(255) UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE runtime_capabilities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid REFERENCES tenants(id),
    runtime_node_id UUID NOT NULL REFERENCES runtime_nodes(id) ON DELETE CASCADE,
    capability_type VARCHAR(100) NOT NULL,
    capability_key VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (runtime_node_id, capability_type, capability_key)
);

CREATE TABLE digital_employees (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid REFERENCES tenants(id),
    team_id UUID REFERENCES tenant_teams(id),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    status VARCHAR(50) NOT NULL DEFAULT 'draft',
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    disabled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE digital_employee_execution_instances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid REFERENCES tenants(id),
    digital_employee_id UUID NOT NULL REFERENCES digital_employees(id) ON DELETE CASCADE,
    runtime_node_id UUID NOT NULL REFERENCES runtime_nodes(id),
    provider_type VARCHAR(100) NOT NULL,
    agent_home_dir TEXT NOT NULL,
    workspace_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    session_policy VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'ready',
    runtime_selector JSONB NOT NULL DEFAULT '{}'::jsonb,
    capacity_requirements JSONB NOT NULL DEFAULT '{}'::jsonb,
    fallback_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    disabled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (digital_employee_id)
);

CREATE TABLE provider_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid REFERENCES tenants(id),
    digital_employee_id UUID NOT NULL REFERENCES digital_employees(id),
    execution_instance_id UUID NOT NULL REFERENCES digital_employee_execution_instances(id),
    runtime_node_id UUID NOT NULL REFERENCES runtime_nodes(id),
    provider_type VARCHAR(100) NOT NULL,
    provider_session_id VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL,
    recoverable BOOLEAN NOT NULL DEFAULT true,
    last_active_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (runtime_node_id, provider_type, provider_session_id)
);

CREATE TABLE provider_session_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid REFERENCES tenants(id),
    provider_session_id UUID NOT NULL REFERENCES provider_sessions(id) ON DELETE CASCADE,
    digital_employee_id UUID NOT NULL REFERENCES digital_employees(id),
    execution_instance_id UUID NOT NULL REFERENCES digital_employee_execution_instances(id),
    runtime_node_id UUID NOT NULL REFERENCES runtime_nodes(id),
    event_type VARCHAR(100) NOT NULL,
    sequence_number INTEGER NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    raw_event_ref TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider_session_id, sequence_number)
);
```

Add indexes near the existing index section:

```sql
CREATE INDEX idx_runtime_enrollments_status ON runtime_enrollments(status);
CREATE INDEX idx_runtime_enrollments_node_id ON runtime_enrollments(node_id);
CREATE INDEX idx_runtime_sessions_runtime_node_id ON runtime_sessions(runtime_node_id);
CREATE INDEX idx_runtime_sessions_token_hash ON runtime_sessions(token_hash);
CREATE INDEX idx_runtime_capabilities_runtime_node_id ON runtime_capabilities(runtime_node_id);
CREATE INDEX idx_digital_employees_tenant_id ON digital_employees(tenant_id);
CREATE INDEX idx_digital_employees_status ON digital_employees(status);
CREATE INDEX idx_execution_instances_runtime_node_id ON digital_employee_execution_instances(runtime_node_id);
CREATE INDEX idx_provider_sessions_employee ON provider_sessions(digital_employee_id);
CREATE INDEX idx_provider_session_events_session_seq ON provider_session_events(provider_session_id, sequence_number);
```

- [ ] **Step 4: Add sqlc queries**

Create `apps/control-plane/internal/storage/queries/runtime_enrollment.sql`:

```sql
-- name: CreateRuntimeBootstrapKey :one
INSERT INTO runtime_bootstrap_keys (name, key_hash, status)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListActiveRuntimeBootstrapKeys :many
SELECT * FROM runtime_bootstrap_keys
WHERE status = 'active'
  AND revoked_at IS NULL
ORDER BY created_at DESC;

-- name: UpsertRuntimeEnrollment :one
INSERT INTO runtime_enrollments (tenant_id, runtime_node_id, bootstrap_key_id, node_id, status, metadata, last_seen_at)
VALUES ($1, NULL, $2, $3, $4, $5, NOW())
ON CONFLICT (tenant_id, node_id) DO UPDATE SET
  bootstrap_key_id = EXCLUDED.bootstrap_key_id,
  metadata = EXCLUDED.metadata,
  last_seen_at = NOW(),
  updated_at = NOW()
RETURNING *;

-- name: UpdateRuntimeEnrollmentStatus :one
UPDATE runtime_enrollments
SET status = $2,
    approved_at = CASE WHEN $2 = 'approved' THEN NOW() ELSE approved_at END,
    rejected_at = CASE WHEN $2 = 'rejected' THEN NOW() ELSE rejected_at END,
    revoked_at = CASE WHEN $2 = 'revoked' THEN NOW() ELSE revoked_at END,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: AttachEnrollmentRuntimeNode :one
UPDATE runtime_enrollments
SET runtime_node_id = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ListRuntimeEnrollments :many
SELECT * FROM runtime_enrollments
WHERE ($1::varchar IS NULL OR status = $1)
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CreateRuntimeSession :one
INSERT INTO runtime_sessions (runtime_node_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetRuntimeSessionByTokenHash :one
SELECT * FROM runtime_sessions
WHERE token_hash = $1
  AND revoked_at IS NULL
  AND expires_at > NOW();

-- name: RenewRuntimeSession :one
UPDATE runtime_sessions
SET expires_at = $2,
    last_seen_at = NOW(),
    updated_at = NOW()
WHERE id = $1
  AND revoked_at IS NULL
RETURNING *;

-- name: RevokeRuntimeSessionsForNode :exec
UPDATE runtime_sessions
SET revoked_at = COALESCE(revoked_at, NOW()),
    updated_at = NOW()
WHERE runtime_node_id = $1
  AND revoked_at IS NULL;

-- name: UpsertRuntimeCapability :one
INSERT INTO runtime_capabilities (runtime_node_id, capability_type, capability_key, status, details, last_seen_at)
VALUES ($1, $2, $3, $4, $5, NOW())
ON CONFLICT (runtime_node_id, capability_type, capability_key) DO UPDATE SET
  status = EXCLUDED.status,
  details = EXCLUDED.details,
  last_seen_at = NOW(),
  updated_at = NOW()
RETURNING *;
```

Create `apps/control-plane/internal/storage/queries/employee_execution.sql`:

```sql
-- name: CreateDigitalEmployee :one
INSERT INTO digital_employees (tenant_id, team_id, name, description, status, config)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetDigitalEmployee :one
SELECT * FROM digital_employees
WHERE id = $1;

-- name: ListDigitalEmployees :many
SELECT * FROM digital_employees
WHERE tenant_id = $1
  AND ($2::varchar IS NULL OR status = $2)
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: UpdateDigitalEmployeeStatus :one
UPDATE digital_employees
SET status = $2,
    disabled_at = CASE WHEN $2 = 'disabled' THEN NOW() ELSE disabled_at END,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpsertDigitalEmployeeExecutionInstance :one
INSERT INTO digital_employee_execution_instances (
  digital_employee_id,
  runtime_node_id,
  provider_type,
  agent_home_dir,
  workspace_policy,
  session_policy,
  status,
  runtime_selector,
  capacity_requirements,
  fallback_policy
) VALUES ($1, $2, $3, $4, $5, $6, $7, '{}'::jsonb, '{}'::jsonb, '{}'::jsonb)
ON CONFLICT (digital_employee_id) DO UPDATE SET
  runtime_node_id = EXCLUDED.runtime_node_id,
  provider_type = EXCLUDED.provider_type,
  agent_home_dir = EXCLUDED.agent_home_dir,
  workspace_policy = EXCLUDED.workspace_policy,
  session_policy = EXCLUDED.session_policy,
  status = EXCLUDED.status,
  updated_at = NOW()
RETURNING *;

-- name: GetExecutionInstanceByEmployeeID :one
SELECT * FROM digital_employee_execution_instances
WHERE digital_employee_id = $1;

-- name: ListExecutionInstancesForRuntime :many
SELECT * FROM digital_employee_execution_instances
WHERE runtime_node_id = $1
ORDER BY created_at DESC;
```

Create `apps/control-plane/internal/storage/queries/provider_session.sql`:

```sql
-- name: UpsertProviderSession :one
INSERT INTO provider_sessions (
  digital_employee_id,
  execution_instance_id,
  runtime_node_id,
  provider_type,
  provider_session_id,
  status,
  recoverable,
  last_active_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
ON CONFLICT (runtime_node_id, provider_type, provider_session_id) DO UPDATE SET
  status = EXCLUDED.status,
  recoverable = EXCLUDED.recoverable,
  last_active_at = NOW(),
  updated_at = NOW()
RETURNING *;

-- name: GetLatestProviderSessionForEmployee :one
SELECT * FROM provider_sessions
WHERE digital_employee_id = $1
  AND recoverable = true
ORDER BY last_active_at DESC
LIMIT 1;

-- name: CreateProviderSessionEvent :one
INSERT INTO provider_session_events (
  provider_session_id,
  digital_employee_id,
  execution_instance_id,
  runtime_node_id,
  event_type,
  sequence_number,
  payload,
  raw_event_ref
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListProviderSessionEvents :many
SELECT * FROM provider_session_events
WHERE provider_session_id = $1
ORDER BY sequence_number ASC
LIMIT $2 OFFSET $3;
```

- [ ] **Step 5: Extend OpenAPI contract**

In `contracts/control-plane/openapi.yaml`, add paths:

```yaml
  /api/v1/runtime/enroll/hello:
    post:
      operationId: runtimeEnrollHello
      summary: Runtime Agent self-discovery hello
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/RuntimeEnrollHelloRequest"
      responses:
        "200":
          description: Enrollment state
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/RuntimeEnrollHelloResponse"

  /api/v1/runtime/enrollments:
    get:
      operationId: listRuntimeEnrollments
      summary: List Runtime enrollment records
      responses:
        "200":
          description: Runtime enrollments
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: "#/components/schemas/RuntimeEnrollment"

  /api/v1/runtime/enrollments/{enrollmentId}/approve:
    post:
      operationId: approveRuntimeEnrollment
      summary: Approve a Runtime enrollment
      parameters:
        - name: enrollmentId
          in: path
          required: true
          schema:
            type: string
            format: uuid
      responses:
        "200":
          description: Approved enrollment
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/RuntimeEnrollment"

  /api/v1/runtime/session/renew:
    post:
      operationId: renewRuntimeSession
      summary: Renew a Runtime session token
      responses:
        "200":
          description: Renewed Runtime session
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/RuntimeSessionResponse"

  /api/v1/runtime/capabilities:
    post:
      operationId: upsertRuntimeCapabilities
      summary: Upsert Runtime capabilities
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/RuntimeCapabilitiesRequest"
      responses:
        "202":
          description: Capabilities accepted

  /api/v1/digital-employees:
    get:
      operationId: listDigitalEmployees
      summary: List digital employees
      responses:
        "200":
          description: Digital employees
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: "#/components/schemas/DigitalEmployee"
    post:
      operationId: createDigitalEmployee
      summary: Create a digital employee draft
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/CreateDigitalEmployeeRequest"
      responses:
        "201":
          description: Created digital employee
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/DigitalEmployee"
```

Add component schemas:

```yaml
    RuntimeEnrollHelloRequest:
      type: object
      required: [node_id, bootstrap_key, capabilities]
      properties:
        node_id:
          type: string
        bootstrap_key:
          type: string
        metadata:
          type: object
          additionalProperties: true
        capabilities:
          $ref: "#/components/schemas/RuntimeCapabilitiesRequest"

    RuntimeEnrollHelloResponse:
      type: object
      required: [status]
      properties:
        status:
          type: string
          enum: [pending, approved, rejected, revoked]
        session:
          $ref: "#/components/schemas/RuntimeSessionResponse"

    RuntimeEnrollment:
      type: object
      required: [id, node_id, status, created_at, updated_at]
      properties:
        id:
          type: string
          format: uuid
        node_id:
          type: string
        status:
          type: string
          enum: [pending, approved, rejected, revoked]
        metadata:
          type: object
          additionalProperties: true
        created_at:
          type: string
          format: date-time
        updated_at:
          type: string
          format: date-time

    RuntimeSessionResponse:
      type: object
      required: [token, expires_at]
      properties:
        token:
          type: string
        expires_at:
          type: string
          format: date-time

    RuntimeCapabilitiesRequest:
      type: object
      required: [providers, workspace, capacity]
      properties:
        providers:
          type: array
          items:
            type: object
            required: [kind, enabled]
            properties:
              kind:
                type: string
              enabled:
                type: boolean
              binary_path:
                type: string
              version:
                type: string
              status:
                type: string
        workspace:
          type: object
          required: [base_dir]
          properties:
            base_dir:
              type: string
        capacity:
          type: object
          required: [max_concurrent_tasks]
          properties:
            max_concurrent_tasks:
              type: integer

    CreateDigitalEmployeeRequest:
      type: object
      required: [name]
      properties:
        name:
          type: string
        description:
          type: string
        team_id:
          type: string
          format: uuid
        config:
          type: object
          additionalProperties: true

    DigitalEmployee:
      type: object
      required: [id, name, status, created_at, updated_at]
      properties:
        id:
          type: string
          format: uuid
        name:
          type: string
        description:
          type: string
        status:
          type: string
          enum: [draft, ready, active, disabled, error]
        execution_instance:
          $ref: "#/components/schemas/DigitalEmployeeExecutionInstance"
        created_at:
          type: string
          format: date-time
        updated_at:
          type: string
          format: date-time

    DigitalEmployeeExecutionInstance:
      type: object
      required: [id, runtime_node_id, provider_type, session_policy, status]
      properties:
        id:
          type: string
          format: uuid
        runtime_node_id:
          type: string
          format: uuid
        provider_type:
          type: string
        agent_home_dir:
          type: string
        session_policy:
          type: string
          enum: [new, resume, reuse_latest, ephemeral]
        status:
          type: string
```

- [ ] **Step 6: Generate code**

Run:

```bash
make -C apps/control-plane generate-sqlc
pnpm generate:control-plane
```

Expected: sqlc and OpenAPI generated files include new query methods and runtime/digital employee request/response types.

- [ ] **Step 7: Run storage tests**

Run:

```bash
go test ./apps/control-plane/internal/storage/queries -run 'TestRuntimeEnrollmentAndSessionQueries|TestDigitalEmployeeExecutionQueries' -count=1
```

Expected: PASS if `TEST_DATABASE_URL` / `TEST_REDIS_URL` are configured; documented skip if not configured.

- [ ] **Step 8: Commit**

```bash
git add contracts/control-plane/openapi.yaml apps/control-plane/internal/storage/migrations/001_initial.sql apps/control-plane/internal/storage/queries apps/control-plane/internal/api/gen
git commit -m "feat: add runtime enrollment execution schema"
```

---

## Task 2: Implement Control Plane Runtime Enrollment and Session Service

**Files:**
- Modify: `apps/control-plane/internal/runtime/models.go`
- Modify: `apps/control-plane/internal/runtime/repository.go`
- Modify: `apps/control-plane/internal/runtime/pg_repository.go`
- Modify: `apps/control-plane/internal/runtime/service.go`
- Create: `apps/control-plane/internal/runtime/session_token.go`
- Create: `apps/control-plane/internal/runtime/service_enrollment_test.go`

- [ ] **Step 1: Write service tests**

Create `apps/control-plane/internal/runtime/service_enrollment_test.go`:

```go
package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestEnrollHelloCreatesPendingEnrollment(t *testing.T) {
	repo := newMemoryRuntimeRepo()
	svc, err := NewService(repo)
	require.NoError(t, err)
	repo.bootstrapKey = RuntimeBootstrapKey{
		ID:       uuid.New(),
		TenantID: DefaultTenantID,
		KeyHash:  hashRuntimeSecretForTest(t, "env-key"),
		Status:   "active",
	}

	resp, err := svc.EnrollHello(context.Background(), EnrollHelloRequest{
		NodeID:       "customer-vm-01",
		BootstrapKey: "env-key",
		Metadata:     map[string]any{"hostname": "customer-vm-01"},
	})

	require.NoError(t, err)
	require.Equal(t, EnrollmentStatusPending, resp.Status)
	require.Empty(t, resp.SessionToken)
	require.Equal(t, "customer-vm-01", repo.enrollment.NodeID)
}

func TestEnrollHelloIssuesSessionForApprovedEnrollment(t *testing.T) {
	repo := newMemoryRuntimeRepo()
	svc, err := NewService(repo)
	require.NoError(t, err)
	key := RuntimeBootstrapKey{
		ID:       uuid.New(),
		TenantID: DefaultTenantID,
		KeyHash:  hashRuntimeSecretForTest(t, "env-key"),
		Status:   "active",
	}
	node := NodeRecord{ID: uuid.New(), NodeID: "customer-vm-01", Name: "customer-vm-01", Status: "online"}
	repo.bootstrapKey = key
	repo.node = node
	repo.enrollment = RuntimeEnrollment{
		ID:            uuid.New(),
		TenantID:      DefaultTenantID,
		RuntimeNodeID: &node.ID,
		NodeID:        "customer-vm-01",
		Status:        EnrollmentStatusApproved,
	}

	resp, err := svc.EnrollHello(context.Background(), EnrollHelloRequest{
		NodeID:       "customer-vm-01",
		BootstrapKey: "env-key",
	})

	require.NoError(t, err)
	require.Equal(t, EnrollmentStatusApproved, resp.Status)
	require.NotEmpty(t, resp.SessionToken)
	require.True(t, resp.SessionExpiresAt.After(time.Now().UTC()))
}
```

Also add the in-memory repository helpers in the same test file:

```go
type memoryRuntimeRepo struct {
	bootstrapKey RuntimeBootstrapKey
	enrollment   RuntimeEnrollment
	node         NodeRecord
	session      RuntimeSession
}

func newMemoryRuntimeRepo() *memoryRuntimeRepo { return &memoryRuntimeRepo{} }

func hashRuntimeSecretForTest(t *testing.T, secret string) string {
	t.Helper()
	hash, err := HashRuntimeSecret(secret)
	require.NoError(t, err)
	return hash
}
```

Implement the methods needed by the tests on `memoryRuntimeRepo`; each method should update the struct fields and return stored records. Use exact service interface names introduced in Step 3.

- [ ] **Step 2: Run test to verify failure**

Run:

```bash
go test ./apps/control-plane/internal/runtime -run 'TestEnrollHello' -count=1
```

Expected: FAIL because enrollment models and service methods do not exist.

- [ ] **Step 3: Add runtime enrollment models**

Add to `apps/control-plane/internal/runtime/models.go`:

```go
const DefaultTenantID = "00000000-0000-0000-0000-000000000001"

type EnrollmentStatus string

const (
	EnrollmentStatusPending  EnrollmentStatus = "pending"
	EnrollmentStatusApproved EnrollmentStatus = "approved"
	EnrollmentStatusRejected EnrollmentStatus = "rejected"
	EnrollmentStatusRevoked  EnrollmentStatus = "revoked"
)

type RuntimeBootstrapKey struct {
	ID       uuid.UUID
	TenantID uuid.UUID
	KeyHash  string
	Status   string
}

type RuntimeEnrollment struct {
	ID            uuid.UUID
	TenantID      uuid.UUID
	RuntimeNodeID *uuid.UUID
	NodeID        string
	Status        EnrollmentStatus
	Metadata      map[string]any
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type RuntimeSession struct {
	ID            uuid.UUID
	TenantID      uuid.UUID
	RuntimeNodeID uuid.UUID
	TokenHash     string
	ExpiresAt     time.Time
	LastSeenAt    time.Time
}

type EnrollHelloRequest struct {
	NodeID       string
	BootstrapKey string
	Metadata     map[string]any
}

type EnrollHelloResponse struct {
	Status           EnrollmentStatus
	SessionToken     string
	SessionExpiresAt time.Time
}

type IssueRuntimeSessionRequest struct {
	RuntimeNodeID uuid.UUID
	TenantID      uuid.UUID
	TTL           time.Duration
}
```

- [ ] **Step 4: Add token helpers**

Create `apps/control-plane/internal/runtime/session_token.go`:

```go
package runtime

import (
	"crypto/rand"
	"encoding/base64"

	"golang.org/x/crypto/bcrypt"
)

func GenerateRuntimeSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func HashRuntimeSecret(secret string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	return string(hash), err
}

func VerifyRuntimeSecret(hash, secret string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret)) == nil
}
```

- [ ] **Step 5: Extend repository interface**

Add to `apps/control-plane/internal/runtime/repository.go`:

```go
	ListActiveBootstrapKeys(ctx context.Context) ([]RuntimeBootstrapKey, error)
	UpsertEnrollment(ctx context.Context, params UpsertEnrollmentParams) (RuntimeEnrollment, error)
	UpdateEnrollmentStatus(ctx context.Context, enrollmentID uuid.UUID, status EnrollmentStatus) (RuntimeEnrollment, error)
	AttachEnrollmentRuntimeNode(ctx context.Context, enrollmentID, runtimeNodeID uuid.UUID) (RuntimeEnrollment, error)
	ListEnrollments(ctx context.Context, status *EnrollmentStatus, limit, offset int32) ([]RuntimeEnrollment, error)
	CreateRuntimeSession(ctx context.Context, params CreateRuntimeSessionParams) (RuntimeSession, error)
	GetRuntimeSessionByTokenHash(ctx context.Context, tokenHash string) (RuntimeSession, error)
	RenewRuntimeSession(ctx context.Context, sessionID uuid.UUID, expiresAt time.Time) (RuntimeSession, error)
	RevokeRuntimeSessionsForNode(ctx context.Context, runtimeNodeID uuid.UUID) error
```

Add param structs:

```go
type UpsertEnrollmentParams struct {
	TenantID       uuid.UUID
	BootstrapKeyID uuid.UUID
	NodeID         string
	Status         EnrollmentStatus
	Metadata       []byte
}

type CreateRuntimeSessionParams struct {
	RuntimeNodeID uuid.UUID
	TokenHash     string
	ExpiresAt     time.Time
}
```

- [ ] **Step 6: Implement service methods**

Add to `apps/control-plane/internal/runtime/service.go`:

```go
const RuntimeSessionTTL = 12 * time.Hour

func (s *Service) EnrollHello(ctx context.Context, req EnrollHelloRequest) (*EnrollHelloResponse, error) {
	if strings.TrimSpace(req.NodeID) == "" {
		return nil, errors.New("node_id is required")
	}
	if strings.TrimSpace(req.BootstrapKey) == "" {
		return nil, errors.New("bootstrap_key is required")
	}

	keys, err := s.repository.ListActiveBootstrapKeys(ctx)
	if err != nil {
		return nil, err
	}
	var key *RuntimeBootstrapKey
	for _, candidate := range keys {
		if VerifyRuntimeSecret(candidate.KeyHash, req.BootstrapKey) {
			matched := candidate
			key = &matched
			break
		}
	}
	if key == nil {
		return nil, errors.New("invalid bootstrap key")
	}

	metadata := []byte(`{}`)
	if req.Metadata != nil {
		metadata, err = json.Marshal(req.Metadata)
		if err != nil {
			return nil, err
		}
	}

	enrollment, err := s.repository.UpsertEnrollment(ctx, UpsertEnrollmentParams{
		TenantID:        key.TenantID,
		BootstrapKeyID: key.ID,
		NodeID:          req.NodeID,
		Status:          EnrollmentStatusPending,
		Metadata:        metadata,
	})
	if err != nil {
		return nil, err
	}

	resp := &EnrollHelloResponse{Status: enrollment.Status}
	if enrollment.Status != EnrollmentStatusApproved || enrollment.RuntimeNodeID == nil {
		return resp, nil
	}

	token, expiresAt, err := s.issueRuntimeSession(ctx, IssueRuntimeSessionRequest{
		RuntimeNodeID: *enrollment.RuntimeNodeID,
		TenantID:      enrollment.TenantID,
		TTL:           RuntimeSessionTTL,
	})
	if err != nil {
		return nil, err
	}
	resp.SessionToken = token
	resp.SessionExpiresAt = expiresAt
	return resp, nil
}

func (s *Service) issueRuntimeSession(ctx context.Context, req IssueRuntimeSessionRequest) (string, time.Time, error) {
	token, err := GenerateRuntimeSecret()
	if err != nil {
		return "", time.Time{}, err
	}
	hash, err := HashRuntimeSecret(token)
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().UTC().Add(req.TTL)
	_, err = s.repository.CreateRuntimeSession(ctx, CreateRuntimeSessionParams{
		RuntimeNodeID: req.RuntimeNodeID,
		TokenHash:     hash,
		ExpiresAt:     expiresAt,
	})
	return token, expiresAt, err
}
```

- [ ] **Step 7: Implement pg repository mapping**

Add mappings in `apps/control-plane/internal/runtime/pg_repository.go` using generated sqlc methods. For example:

```go
func (r *PgRepository) CreateRuntimeSession(ctx context.Context, params CreateRuntimeSessionParams) (RuntimeSession, error) {
	session, err := r.q.CreateRuntimeSession(ctx, queries.CreateRuntimeSessionParams{
		RuntimeNodeID: params.RuntimeNodeID,
		TokenHash:     params.TokenHash,
		ExpiresAt:     pgtype.Timestamptz{Time: params.ExpiresAt, Valid: true},
	})
	if err != nil {
		return RuntimeSession{}, err
	}
	return RuntimeSession{
		ID:            session.ID,
		TenantID:      session.TenantID,
		RuntimeNodeID: session.RuntimeNodeID,
		TokenHash:     session.TokenHash,
		ExpiresAt:     session.ExpiresAt.Time,
		LastSeenAt:    session.LastSeenAt.Time,
	}, nil
}
```

- [ ] **Step 8: Run runtime service tests**

Run:

```bash
go test ./apps/control-plane/internal/runtime -run 'TestEnrollHello' -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add apps/control-plane/internal/runtime apps/control-plane/internal/storage/queries
git commit -m "feat: add runtime enrollment service"
```

---

## Task 3: Wire Runtime Enrollment, Session Auth, and HTTP Routes

**Files:**
- Modify: `apps/control-plane/internal/api/middleware/auth.go`
- Modify: `apps/control-plane/internal/api/handlers/runtime.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/app/app.go`
- Modify: `apps/control-plane/internal/api/routes_test.go`

- [ ] **Step 1: Add route tests**

Append to `apps/control-plane/internal/api/routes_test.go`:

```go
func TestRuntimeEnrollHelloRouteDoesNotRequireRuntimeSession(t *testing.T) {
	handler := handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{})
	server := NewServerWithAuthz(&handlers.TaskHandler{}, handler, &routeAuthService{}, &routeRuntimeSessionAuth{}, &routeAuthorizer{}, nil)

	body := strings.NewReader(`{"node_id":"node-1","bootstrap_key":"env-key","capabilities":{"providers":[],"workspace":{"base_dir":"/tmp"},"capacity":{"max_concurrent_tasks":1}}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/enroll/hello", body)
	res := httptest.NewRecorder()
	server.ServeHTTP(res, req)

	if res.Code == http.StatusUnauthorized {
		t.Fatalf("enroll hello must not require runtime session auth")
	}
}

func TestRuntimeHeartbeatRequiresRuntimeSession(t *testing.T) {
	handler := handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{})
	server := NewServerWithAuthz(&handlers.TaskHandler{}, handler, &routeAuthService{}, &routeRuntimeSessionAuth{}, &routeAuthorizer{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/heartbeat", strings.NewReader(`{"current_load":0,"status":"online"}`))
	res := httptest.NewRecorder()
	server.ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without runtime session, got %d", res.Code)
	}
}
```

- [ ] **Step 2: Run route tests and verify failure**

Run:

```bash
go test ./apps/control-plane/internal/api -run 'TestRuntimeEnrollHelloRouteDoesNotRequireRuntimeSession|TestRuntimeHeartbeatRequiresRuntimeSession' -count=1
```

Expected: FAIL because routes and middleware split are not implemented.

- [ ] **Step 3: Add runtime session auth middleware**

Modify `apps/control-plane/internal/api/middleware/auth.go`:

```go
type RuntimeSessionAuthService interface {
	ValidateRuntimeSession(ctx context.Context, token string) (nodeID string, err error)
}

func RuntimeSessionAuth(authService RuntimeSessionAuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			parts := strings.SplitN(auth, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, "invalid authorization header", http.StatusUnauthorized)
				return
			}
			nodeID, err := authService.ValidateRuntimeSession(r.Context(), parts[1])
			if err != nil {
				http.Error(w, "invalid runtime session", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), NodeIDKey, nodeID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
```

- [ ] **Step 4: Add runtime handler methods**

Add methods to `apps/control-plane/internal/api/handlers/runtime.go`:

```go
func (h *RuntimeHandler) EnrollHello(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeID       string                 `json:"node_id"`
		BootstrapKey string                 `json:"bootstrap_key"`
		Metadata     map[string]any         `json:"metadata"`
		Capabilities map[string]any         `json:"capabilities"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := h.runtimeService.EnrollHello(r.Context(), runtime.EnrollHelloRequest{
		NodeID:       req.NodeID,
		BootstrapKey: req.BootstrapKey,
		Metadata:     req.Metadata,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": resp.Status,
		"session": map[string]any{
			"token":      resp.SessionToken,
			"expires_at": resp.SessionExpiresAt,
		},
	})
}
```

Extend `RuntimeService` interface in the same file to include:

```go
	EnrollHello(ctx context.Context, req runtime.EnrollHelloRequest) (*runtime.EnrollHelloResponse, error)
```

- [ ] **Step 5: Split runtime routes**

Modify `apps/control-plane/internal/api/server.go` so:

```go
r.Route("/api/v1/runtime", func(r chi.Router) {
	r.Post("/enroll/hello", s.runtimeHandler.EnrollHello)

	r.Group(func(r chi.Router) {
		r.Use(middleware.RuntimeSessionAuth(s.runtimeAuthService))
		r.Post("/heartbeat", s.runtimeHandler.Heartbeat)
		r.Post("/capabilities", s.runtimeHandler.UpsertCapabilities)
		r.Post("/session/renew", s.runtimeHandler.RenewSession)
		r.Post("/tasks/claim", s.runtimeHandler.ClaimTask)
		r.Post("/tasks/{taskId}/events", s.runtimeHandler.PushEvents)
		r.Post("/tasks/{taskId}/complete", s.runtimeHandler.CompleteTask)
		r.Post("/tasks/{taskId}/fail", s.runtimeHandler.FailTask)
		r.Post("/tasks/{taskId}/lease", s.runtimeHandler.RenewLease)
	})
})
```

Keep Web user protected routes for listing nodes/enrollments and approval actions.

- [ ] **Step 6: Run route tests**

Run:

```bash
go test ./apps/control-plane/internal/api -run 'TestRuntimeEnrollHelloRouteDoesNotRequireRuntimeSession|TestRuntimeHeartbeatRequiresRuntimeSession' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/api apps/control-plane/internal/app
git commit -m "feat: wire runtime enrollment routes"
```

---

## Task 4: Add Runtime WebSocket Command Channel

**Files:**
- Modify: `apps/control-plane/go.mod`
- Modify: `apps/control-plane/go.sum`
- Create: `apps/control-plane/internal/runtime/connection.go`
- Create: `apps/control-plane/internal/runtime/connection_test.go`
- Modify: `apps/control-plane/internal/api/handlers/runtime.go`
- Modify: `apps/control-plane/internal/api/server.go`

- [ ] **Step 1: Add connection registry tests**

Create `apps/control-plane/internal/runtime/connection_test.go`:

```go
package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConnectionRegistryDispatchesCommand(t *testing.T) {
	registry := NewConnectionRegistry()
	conn := registry.Register("node-1")
	defer registry.Unregister("node-1", conn.ID)

	err := registry.Dispatch(context.Background(), "node-1", RuntimeCommand{
		Type: "ensure_instance",
		ID:   "cmd-1",
		Payload: map[string]any{
			"digital_employee_id": "employee-1",
		},
	})
	require.NoError(t, err)

	select {
	case command := <-conn.Commands:
		require.Equal(t, "cmd-1", command.ID)
		require.Equal(t, "ensure_instance", command.Type)
	case <-time.After(time.Second):
		t.Fatal("expected command")
	}
}
```

- [ ] **Step 2: Run test and verify failure**

Run:

```bash
go test ./apps/control-plane/internal/runtime -run TestConnectionRegistryDispatchesCommand -count=1
```

Expected: FAIL because connection registry does not exist.

- [ ] **Step 3: Implement connection registry**

Create `apps/control-plane/internal/runtime/connection.go`:

```go
package runtime

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"
)

var ErrRuntimeNotConnected = errors.New("runtime not connected")

type RuntimeCommand struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

type RuntimeConnection struct {
	ID       string
	NodeID   string
	Commands chan RuntimeCommand
}

type ConnectionRegistry struct {
	mu          sync.RWMutex
	connections map[string]*RuntimeConnection
}

func NewConnectionRegistry() *ConnectionRegistry {
	return &ConnectionRegistry{connections: make(map[string]*RuntimeConnection)}
}

func (r *ConnectionRegistry) Register(nodeID string) *RuntimeConnection {
	conn := &RuntimeConnection{
		ID:       uuid.NewString(),
		NodeID:   nodeID,
		Commands: make(chan RuntimeCommand, 64),
	}
	r.mu.Lock()
	r.connections[nodeID] = conn
	r.mu.Unlock()
	return conn
}

func (r *ConnectionRegistry) Unregister(nodeID, connectionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.connections[nodeID]; ok && existing.ID == connectionID {
		delete(r.connections, nodeID)
		close(existing.Commands)
	}
}

func (r *ConnectionRegistry) Dispatch(ctx context.Context, nodeID string, command RuntimeCommand) error {
	r.mu.RLock()
	conn := r.connections[nodeID]
	r.mu.RUnlock()
	if conn == nil {
		return ErrRuntimeNotConnected
	}
	select {
	case conn.Commands <- command:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
```

- [ ] **Step 4: Add websocket dependency**

Run:

```bash
cd apps/control-plane
go get nhooyr.io/websocket@v1.8.17
```

Expected: `apps/control-plane/go.mod` and `apps/control-plane/go.sum` include `nhooyr.io/websocket`.

- [ ] **Step 5: Add WebSocket handler**

Add to `apps/control-plane/internal/api/handlers/runtime.go`:

```go
func (h *RuntimeHandler) WebSocket(w http.ResponseWriter, r *http.Request) {
	nodeID := middleware.GetNodeID(r.Context())
	if nodeID == "" {
		http.Error(w, "node_id not found in context", http.StatusUnauthorized)
		return
	}
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer ws.Close(websocket.StatusNormalClosure, "done")

	conn := h.connectionRegistry.Register(nodeID)
	defer h.connectionRegistry.Unregister(nodeID, conn.ID)

	for {
		select {
		case command, ok := <-conn.Commands:
			if !ok {
				return
			}
			body, _ := json.Marshal(command)
			if err := ws.Write(r.Context(), websocket.MessageText, body); err != nil {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}
```

Add `connectionRegistry *runtime.ConnectionRegistry` to `RuntimeHandler` and constructor.

- [ ] **Step 6: Register WS route**

In `apps/control-plane/internal/api/server.go`, add under runtime session protected routes:

```go
r.Get("/ws", s.runtimeHandler.WebSocket)
```

- [ ] **Step 7: Run tests**

Run:

```bash
go test ./apps/control-plane/internal/runtime ./apps/control-plane/internal/api -run 'TestConnectionRegistryDispatchesCommand|TestRuntimeRoutes' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/control-plane/go.mod apps/control-plane/go.sum apps/control-plane/internal/runtime apps/control-plane/internal/api apps/control-plane/internal/app
git commit -m "feat: add runtime websocket command channel"
```

---

## Task 5: Update Runtime Agent Enrollment, Session, and Capabilities

**Files:**
- Modify: `apps/runtime-agent/Cargo.toml`
- Modify: `apps/runtime-agent/src/config.rs`
- Modify: `apps/runtime-agent/config.example.yaml`
- Modify: `apps/runtime-agent/src/controlplane/models.rs`
- Modify: `apps/runtime-agent/src/controlplane/client.rs`
- Create: `apps/runtime-agent/src/session.rs`
- Modify: `apps/runtime-agent/src/daemon.rs`
- Modify: `apps/runtime-agent/tests/daemon_test.rs`

- [ ] **Step 1: Write config tests**

Append to `apps/runtime-agent/tests/daemon_test.rs`:

```rust
#[test]
fn runtime_config_loads_bootstrap_key_from_file_and_env() {
    let temp = tempfile::tempdir().expect("tempdir");
    let config_path = temp.path().join("runtime-agent.yaml");
    std::fs::write(
        &config_path,
        r#"
runtime:
  node_id: file-node
  control_plane_url: http://control-plane:8081
  bootstrap_key: file-bootstrap
"#,
    )
    .expect("write config");

    let file_config = RuntimeConfig::load(Some(&config_path), RuntimeConfigOverrides::default()).expect("load file config");
    assert_eq!(file_config.runtime.bootstrap_key, "file-bootstrap");

    let env_config = RuntimeConfig::load_with_env(
        Some(&config_path),
        [("RUNTIME_AGENT_BOOTSTRAP_KEY", "env-bootstrap")],
        RuntimeConfigOverrides::default(),
    )
    .expect("load env config");
    assert_eq!(env_config.runtime.bootstrap_key, "env-bootstrap");
}
```

- [ ] **Step 2: Run test and verify failure**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml runtime_config_loads_bootstrap_key_from_file_and_env
```

Expected: FAIL because `bootstrap_key` field does not exist.

- [ ] **Step 3: Replace long-lived auth token config**

Modify `apps/runtime-agent/src/config.rs`:

```rust
pub struct RuntimeSection {
    pub node_id: String,
    pub control_plane_url: String,
    pub bootstrap_key: String,
    pub heartbeat_interval: u64,
    pub max_concurrent_tasks: u16,
}
```

Replace file/env/override handling from `auth_token` to `bootstrap_key`:

```rust
struct FileRuntimeSection {
    node_id: Option<String>,
    control_plane_url: Option<String>,
    bootstrap_key: Option<String>,
    heartbeat_interval: Option<u64>,
    max_concurrent_tasks: Option<u16>,
}
```

In env parsing, use:

```rust
"RUNTIME_AGENT_BOOTSTRAP_KEY" => self.runtime.bootstrap_key = value.to_string(),
```

Validation:

```rust
if self.runtime.bootstrap_key.trim().is_empty() {
    anyhow::bail!("runtime.bootstrap_key is required");
}
```

- [ ] **Step 4: Add enrollment models**

Modify `apps/runtime-agent/src/controlplane/models.rs`:

```rust
#[derive(Debug, Clone, Serialize)]
pub struct EnrollHelloRequest {
    pub node_id: String,
    pub bootstrap_key: String,
    pub metadata: HashMap<String, serde_json::Value>,
    pub capabilities: RuntimeCapabilitiesRequest,
}

#[derive(Debug, Clone, Deserialize)]
pub struct EnrollHelloResponse {
    pub status: EnrollmentStatus,
    pub session: Option<RuntimeSessionResponse>,
}

#[derive(Debug, Clone, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub enum EnrollmentStatus {
    Pending,
    Approved,
    Rejected,
    Revoked,
}

#[derive(Debug, Clone, Deserialize)]
pub struct RuntimeSessionResponse {
    pub token: String,
    pub expires_at: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct RuntimeCapabilitiesRequest {
    pub providers: Vec<RuntimeProviderCapability>,
    pub workspace: RuntimeWorkspaceCapability,
    pub capacity: RuntimeCapacityCapability,
}

#[derive(Debug, Clone, Serialize)]
pub struct RuntimeProviderCapability {
    pub kind: String,
    pub enabled: bool,
    pub binary_path: String,
    pub status: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct RuntimeWorkspaceCapability {
    pub base_dir: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct RuntimeCapacityCapability {
    pub max_concurrent_tasks: u16,
}
```

- [ ] **Step 5: Add client methods**

Modify `apps/runtime-agent/src/controlplane/client.rs`:

```rust
pub async fn enroll_hello(&self, req: EnrollHelloRequest) -> Result<EnrollHelloResponse> {
    let url = format!("{}/api/v1/runtime/enroll/hello", self.base_url);
    let response = self.client.post(&url).json(&req).send().await.context("send enroll hello")?;
    if !response.status().is_success() {
        anyhow::bail!("Enroll hello failed with status {}: {}", response.status(), response.text().await.unwrap_or_default());
    }
    response.json::<EnrollHelloResponse>().await.context("parse enroll hello")
}

pub fn with_session_token(base_url: impl Into<String>, token: impl Into<String>, node_id: impl Into<String>) -> Self {
    Self::with_node_id(base_url, token, node_id)
}
```

- [ ] **Step 6: Add in-memory session state**

Create `apps/runtime-agent/src/session.rs`:

```rust
#[derive(Debug, Clone)]
pub struct RuntimeSession {
    pub token: String,
    pub expires_at: String,
}

impl RuntimeSession {
    pub fn is_empty(&self) -> bool {
        self.token.trim().is_empty()
    }
}
```

Export it in `apps/runtime-agent/src/lib.rs`:

```rust
pub mod session;
```

- [ ] **Step 7: Update daemon startup**

Modify `apps/runtime-agent/src/daemon.rs` so startup does:

```rust
let bootstrap_client = ControlPlaneClient::new(&self.config.runtime.control_plane_url, "");
let enroll = bootstrap_client.enroll_hello(EnrollHelloRequest {
    node_id: self.config.runtime.node_id.clone(),
    bootstrap_key: self.config.runtime.bootstrap_key.clone(),
    metadata: HashMap::new(),
    capabilities: build_capabilities(&self.config),
}).await?;

if enroll.status != EnrollmentStatus::Approved {
    println!("runtime-agent node={} enrollment={:?}; waiting for approval", self.config.runtime.node_id, enroll.status);
    return Ok(());
}

let session = enroll.session.context("approved enrollment missing session")?;
let client = ControlPlaneClient::with_session_token(
    &self.config.runtime.control_plane_url,
    session.token,
    &self.config.runtime.node_id,
);
```

Keep heartbeat and executor startup using the session-backed `client`.

- [ ] **Step 8: Run Rust tests**

Run:

```bash
cargo fmt --manifest-path apps/runtime-agent/Cargo.toml --check
cargo test --manifest-path apps/runtime-agent/Cargo.toml
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add apps/runtime-agent
git commit -m "feat: enroll runtime agent with bootstrap key"
```

---

## Task 6: Add Digital Employee Backend and Execution Instance Service

**Files:**
- Create: `apps/control-plane/internal/employee/types.go`
- Create: `apps/control-plane/internal/employee/repository.go`
- Create: `apps/control-plane/internal/employee/pg_repository.go`
- Create: `apps/control-plane/internal/employee/service.go`
- Create: `apps/control-plane/internal/employee/handler.go`
- Create: `apps/control-plane/internal/employee/service_test.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/app/app.go`

- [ ] **Step 1: Write service tests**

Create `apps/control-plane/internal/employee/service_test.go`:

```go
package employee

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCreateDraftDigitalEmployee(t *testing.T) {
	repo := newMemoryRepository()
	service := NewService(repo)

	created, err := service.CreateDraft(context.Background(), CreateDraftInput{
		TenantID:    defaultTenantID,
		Name:        "前端开发员工一",
		Description: "负责前端开发",
		Config:      map[string]any{"role": "frontend"},
	})

	require.NoError(t, err)
	require.Equal(t, "前端开发员工一", created.Name)
	require.Equal(t, StatusDraft, created.Status)
}

func TestBindExecutionInstanceReplacesExistingInstance(t *testing.T) {
	repo := newMemoryRepository()
	service := NewService(repo)
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()

	first, err := service.BindExecutionInstance(context.Background(), BindExecutionInput{
		DigitalEmployeeID: employeeID,
		RuntimeNodeID:     runtimeNodeID,
		ProviderType:      "claude-code",
		AgentHomeDir:      "/data/superteam/workspaces/agents/" + employeeID.String(),
		SessionPolicy:     "reuse_latest",
	})
	require.NoError(t, err)

	second, err := service.BindExecutionInstance(context.Background(), BindExecutionInput{
		DigitalEmployeeID: employeeID,
		RuntimeNodeID:     runtimeNodeID,
		ProviderType:      "opencode",
		AgentHomeDir:      "/data/superteam/workspaces/agents/" + employeeID.String(),
		SessionPolicy:     "new",
	})
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, "opencode", second.ProviderType)
}
```

- [ ] **Step 2: Run test and verify failure**

Run:

```bash
go test ./apps/control-plane/internal/employee -count=1
```

Expected: FAIL because package does not exist.

- [ ] **Step 3: Add types**

Create `apps/control-plane/internal/employee/types.go`:

```go
package employee

import "github.com/google/uuid"

type Status string

const (
	StatusDraft    Status = "draft"
	StatusReady    Status = "ready"
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
	StatusError    Status = "error"
)

type DigitalEmployee struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	TeamID      *uuid.UUID
	Name        string
	Description string
	Status      Status
	Config      map[string]any
	Execution   *ExecutionInstance
}

type ExecutionInstance struct {
	ID                uuid.UUID
	DigitalEmployeeID uuid.UUID
	RuntimeNodeID     uuid.UUID
	ProviderType      string
	AgentHomeDir      string
	WorkspacePolicy   map[string]any
	SessionPolicy     string
	Status            string
}

type CreateDraftInput struct {
	TenantID    uuid.UUID
	TeamID      *uuid.UUID
	Name        string
	Description string
	Config      map[string]any
}

type BindExecutionInput struct {
	DigitalEmployeeID uuid.UUID
	RuntimeNodeID     uuid.UUID
	ProviderType      string
	AgentHomeDir      string
	WorkspacePolicy   map[string]any
	SessionPolicy     string
}
```

- [ ] **Step 4: Add service**

Create `apps/control-plane/internal/employee/service.go`:

```go
package employee

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateDraft(ctx context.Context, input CreateDraftInput) (*DigitalEmployee, error) {
	if strings.TrimSpace(input.Name) == "" {
		return nil, errors.New("name is required")
	}
	config := input.Config
	if config == nil {
		config = map[string]any{}
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	return s.repo.CreateDraft(ctx, CreateDraftParams{
		TenantID:    input.TenantID,
		TeamID:      input.TeamID,
		Name:        strings.TrimSpace(input.Name),
		Description: input.Description,
		Status:      StatusDraft,
		Config:      configJSON,
	})
}

func (s *Service) BindExecutionInstance(ctx context.Context, input BindExecutionInput) (*ExecutionInstance, error) {
	if input.DigitalEmployeeID == uuid.Nil || input.RuntimeNodeID == uuid.Nil {
		return nil, errors.New("digital_employee_id and runtime_node_id are required")
	}
	if strings.TrimSpace(input.ProviderType) == "" {
		return nil, errors.New("provider_type is required")
	}
	if strings.TrimSpace(input.AgentHomeDir) == "" {
		return nil, errors.New("agent_home_dir is required")
	}
	workspacePolicy := input.WorkspacePolicy
	if workspacePolicy == nil {
		workspacePolicy = map[string]any{}
	}
	workspaceJSON, err := json.Marshal(workspacePolicy)
	if err != nil {
		return nil, err
	}
	return s.repo.UpsertExecutionInstance(ctx, UpsertExecutionParams{
		DigitalEmployeeID: input.DigitalEmployeeID,
		RuntimeNodeID:     input.RuntimeNodeID,
		ProviderType:      input.ProviderType,
		AgentHomeDir:      input.AgentHomeDir,
		WorkspacePolicy:   workspaceJSON,
		SessionPolicy:     input.SessionPolicy,
		Status:            "ready",
	})
}
```

Import `github.com/google/uuid` in this file.

- [ ] **Step 5: Add repository interface and pg repository**

Create `apps/control-plane/internal/employee/repository.go`:

```go
package employee

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	CreateDraft(ctx context.Context, params CreateDraftParams) (*DigitalEmployee, error)
	List(ctx context.Context, tenantID uuid.UUID, status *Status, limit, offset int32) ([]*DigitalEmployee, error)
	Get(ctx context.Context, id uuid.UUID) (*DigitalEmployee, error)
	UpsertExecutionInstance(ctx context.Context, params UpsertExecutionParams) (*ExecutionInstance, error)
}

type CreateDraftParams struct {
	TenantID    uuid.UUID
	TeamID      *uuid.UUID
	Name        string
	Description string
	Status      Status
	Config      []byte
}

type UpsertExecutionParams struct {
	DigitalEmployeeID uuid.UUID
	RuntimeNodeID     uuid.UUID
	ProviderType      string
	AgentHomeDir      string
	WorkspacePolicy   []byte
	SessionPolicy     string
	Status            string
}
```

Create `apps/control-plane/internal/employee/pg_repository.go` using generated sqlc methods from Task 1.

- [ ] **Step 6: Add HTTP handler**

Create `apps/control-plane/internal/employee/handler.go`:

```go
package employee

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) CreateDigitalEmployee(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		TeamID      *uuid.UUID     `json:"team_id"`
		Config      map[string]any `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	created, err := h.service.CreateDraft(r.Context(), CreateDraftInput{
		TenantID:    uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		TeamID:      req.TeamID,
		Name:        req.Name,
		Description: req.Description,
		Config:      req.Config,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}
```

- [ ] **Step 7: Wire app and routes**

Modify `apps/control-plane/internal/app/app.go`:

```go
employeeRepository := employee.NewPgRepository(q)
employeeService := employee.NewService(employeeRepository)
employeeHandler := employee.NewHandler(employeeService)
```

Modify `apps/control-plane/internal/api/server.go` to register:

```go
r.Route("/api/v1/digital-employees", func(r chi.Router) {
	r.Use(middleware.UserAuth(s.authService))
	r.Get("/", s.employeeHandler.ListDigitalEmployees)
	r.Post("/", s.employeeHandler.CreateDigitalEmployee)
})
```

- [ ] **Step 8: Run backend tests**

Run:

```bash
go test ./apps/control-plane/internal/employee ./apps/control-plane/internal/api ./apps/control-plane/internal/app -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add apps/control-plane/internal/employee apps/control-plane/internal/api apps/control-plane/internal/app
git commit -m "feat: add digital employee execution service"
```

---

## Task 7: Implement Ensure Instance and Provider Session Event Flow

**Files:**
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/control-plane/internal/runtime/connection.go`
- Modify: `apps/control-plane/internal/api/handlers/runtime.go`
- Create: `apps/runtime-agent/src/instances.rs`
- Modify: `apps/runtime-agent/src/controlplane/models.rs`
- Modify: `apps/runtime-agent/src/controlplane/client.rs`
- Modify: `apps/runtime-agent/src/providers/claude.rs`
- Modify: `apps/runtime-agent/src/providers/opencode.rs`
- Modify: `apps/runtime-agent/src/daemon.rs`

- [ ] **Step 1: Add Runtime Agent instance directory tests**

Create `apps/runtime-agent/tests/instances_test.rs`:

```rust
use superteam_runtime_agent::instances::{ensure_instance, EnsureInstanceRequest};

#[test]
fn ensure_instance_creates_agent_home_directories() {
    let temp = tempfile::tempdir().expect("tempdir");
    let result = ensure_instance(EnsureInstanceRequest {
        base_dir: temp.path().to_path_buf(),
        execution_instance_id: "instance-1".to_string(),
    })
    .expect("ensure instance");

    assert!(result.agent_home_dir.ends_with("agents/instance-1"));
    assert!(result.agent_home_dir.join("state").is_dir());
    assert!(result.agent_home_dir.join("sessions").is_dir());
    assert!(result.agent_home_dir.join("runs").is_dir());
}
```

- [ ] **Step 2: Run test and verify failure**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml ensure_instance_creates_agent_home_directories
```

Expected: FAIL because `instances` module does not exist.

- [ ] **Step 3: Implement instance directory module**

Create `apps/runtime-agent/src/instances.rs`:

```rust
use std::path::PathBuf;

#[derive(Debug, Clone)]
pub struct EnsureInstanceRequest {
    pub base_dir: PathBuf,
    pub execution_instance_id: String,
}

#[derive(Debug, Clone)]
pub struct EnsureInstanceResult {
    pub agent_home_dir: PathBuf,
}

pub fn ensure_instance(request: EnsureInstanceRequest) -> anyhow::Result<EnsureInstanceResult> {
    let agent_home_dir = request
        .base_dir
        .join("agents")
        .join(sanitize_segment(&request.execution_instance_id)?);
    std::fs::create_dir_all(agent_home_dir.join("state"))?;
    std::fs::create_dir_all(agent_home_dir.join("sessions"))?;
    std::fs::create_dir_all(agent_home_dir.join("runs"))?;
    Ok(EnsureInstanceResult { agent_home_dir })
}

fn sanitize_segment(value: &str) -> anyhow::Result<String> {
    if value.is_empty() || value.contains('/') || value.contains('\\') || value == "." || value == ".." {
        anyhow::bail!("invalid execution instance id");
    }
    Ok(value.to_string())
}
```

Export it from `apps/runtime-agent/src/lib.rs`:

```rust
pub mod instances;
```

- [ ] **Step 4: Add Runtime command models**

In `apps/runtime-agent/src/controlplane/models.rs`:

```rust
#[derive(Debug, Clone, Deserialize)]
pub struct RuntimeCommand {
    pub id: String,
    #[serde(rename = "type")]
    pub command_type: RuntimeCommandType,
    pub payload: serde_json::Value,
}

#[derive(Debug, Clone, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum RuntimeCommandType {
    EnsureInstance,
    StartSession,
    ResumeSession,
    SendInput,
    StopSession,
}

#[derive(Debug, Clone, Deserialize)]
pub struct EnsureInstanceCommand {
    pub execution_instance_id: String,
}
```

- [ ] **Step 5: Add command handling in Runtime Agent**

Create `apps/runtime-agent/src/controlplane/ws.rs`:

```rust
use futures_util::StreamExt;
use tokio_tungstenite::connect_async;

use crate::config::RuntimeConfig;
use crate::controlplane::models::{RuntimeCommand, RuntimeCommandType, EnsureInstanceCommand};
use crate::instances::{ensure_instance, EnsureInstanceRequest};

pub async fn run_command_loop(config: RuntimeConfig, session_token: String) -> anyhow::Result<()> {
    let ws_url = config.runtime.control_plane_url.replace("http://", "ws://").replace("https://", "wss://") + "/api/v1/runtime/ws";
    let request = http::Request::builder()
        .uri(ws_url)
        .header("Authorization", format!("Bearer {}", session_token))
        .body(())?;
    let (mut socket, _) = connect_async(request).await?;
    while let Some(message) = socket.next().await {
        let message = message?;
        if !message.is_text() {
            continue;
        }
        let command: RuntimeCommand = serde_json::from_str(message.to_text()?)?;
        match command.command_type {
            RuntimeCommandType::EnsureInstance => {
                let payload: EnsureInstanceCommand = serde_json::from_value(command.payload)?;
                let _ = ensure_instance(EnsureInstanceRequest {
                    base_dir: config.workspace.base_dir.clone(),
                    execution_instance_id: payload.execution_instance_id,
                })?;
            }
            _ => {}
        }
    }
    Ok(())
}
```

Add dependencies in `apps/runtime-agent/Cargo.toml`:

```toml
tokio-tungstenite = "0.28"
futures-util = "0.3"
http = "1"
```

- [ ] **Step 6: Preserve provider session IDs**

In `apps/runtime-agent/src/providers/claude.rs`, keep current resume handling and ensure system events map session id:

```rust
ProviderEvent::SessionStarted {
    session_id: session_id.to_string(),
}
```

In `apps/runtime-agent/src/providers/opencode.rs`, add the same behavior if OpenCode emits a session identifier; if it does not, emit session metadata from Runtime Agent command payload.

- [ ] **Step 7: Run Rust tests**

Run:

```bash
cargo fmt --manifest-path apps/runtime-agent/Cargo.toml --check
cargo test --manifest-path apps/runtime-agent/Cargo.toml
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/runtime-agent
git commit -m "feat: handle runtime instance commands"
```

---

## Task 8: Build Runtime Node Web Page

**Files:**
- Modify: `apps/web/src/lib/api/runtime.ts`
- Create: `apps/web/src/features/runtime/index.tsx`
- Create: `apps/web/src/features/runtime/index.test.tsx`
- Modify: `apps/web/src/routes/_authenticated/runtime/index.tsx`

- [ ] **Step 1: Write Web API client tests**

Append to `apps/web/src/lib/api/runtime.test.ts`:

```ts
it("approves runtime enrollment with cookie credentials", async () => {
  const fetcher = vi.fn().mockResolvedValue(
    new Response(JSON.stringify({ id: "11111111-1111-4111-8111-111111111111", node_id: "node-1", status: "approved" }), {
      status: 200,
      headers: { "content-type": "application/json" },
    }),
  );

  await approveRuntimeEnrollment({ baseUrl: "http://control-plane.local", fetcher }, "11111111-1111-4111-8111-111111111111");

  expect(fetcher).toHaveBeenCalledWith(
    "http://control-plane.local/api/v1/runtime/enrollments/11111111-1111-4111-8111-111111111111/approve",
    expect.objectContaining({ method: "POST", credentials: "include" }),
  );
});
```

- [ ] **Step 2: Run test and verify failure**

Run:

```bash
pnpm --filter @superteam/web test -- runtime.test.ts
```

Expected: FAIL because `approveRuntimeEnrollment` does not exist.

- [ ] **Step 3: Add Runtime enrollment API client**

Modify `apps/web/src/lib/api/runtime.ts`:

```ts
export type RuntimeEnrollmentStatus = "pending" | "approved" | "rejected" | "revoked";

export type RuntimeEnrollment = {
  id: string;
  node_id: string;
  status: RuntimeEnrollmentStatus;
  metadata?: Record<string, unknown>;
  created_at?: string;
  updated_at?: string;
};

export async function listRuntimeEnrollments(options: ApiClientOptions): Promise<RuntimeEnrollment[]> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/v1/runtime/enrollments"), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "GET",
  });
  return parseJson<RuntimeEnrollment[]>(response, "runtime enrollments");
}

export async function approveRuntimeEnrollment(options: ApiClientOptions, enrollmentId: string): Promise<RuntimeEnrollment> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/runtime/enrollments/${encodeURIComponent(enrollmentId)}/approve`), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "POST",
  });
  return parseJson<RuntimeEnrollment>(response, "approve runtime enrollment");
}
```

- [ ] **Step 4: Add Runtime feature page**

Create `apps/web/src/features/runtime/index.tsx`:

```tsx
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Server, X } from "lucide-react";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { approveRuntimeEnrollment, listRuntimeEnrollments, listRuntimeNodes } from "@/lib/api/runtime";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

export function RuntimeNodesPage() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const queryClient = useQueryClient();
  const enrollments = useQuery({ queryKey: ["runtime-enrollments"], queryFn: () => listRuntimeEnrollments({ baseUrl: apiBaseUrl }) });
  const nodes = useQuery({ queryKey: ["runtime-nodes"], queryFn: () => listRuntimeNodes({ baseUrl: apiBaseUrl }) });
  const approve = useMutation({
    mutationFn: (id: string) => approveRuntimeEnrollment({ baseUrl: apiBaseUrl }, id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["runtime-enrollments"] });
      void queryClient.invalidateQueries({ queryKey: ["runtime-nodes"] });
    },
  });

  return (
    <div className="space-y-4 p-4">
      <div>
        <h1 className="text-2xl font-semibold">Runtime 节点</h1>
        <p className="text-sm text-muted-foreground">管理客户侧 Runtime Agent 接入、在线状态和宿主能力。</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>待接入节点</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {(enrollments.data ?? []).filter((item) => item.status === "pending").map((item) => (
            <div key={item.id} className="flex items-center justify-between rounded-md border p-3">
              <div className="flex items-center gap-3">
                <Server className="size-4" />
                <span className="font-medium">{item.node_id}</span>
                <Badge variant="secondary">待接入</Badge>
              </div>
              <Button type="button" size="sm" disabled={approve.isPending} onClick={() => approve.mutate(item.id)}>
                <Check className="mr-2 size-4" />
                接入
              </Button>
            </div>
          ))}
          {enrollments.data?.filter((item) => item.status === "pending").length === 0 ? <p className="text-sm text-muted-foreground">暂无待接入节点。</p> : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>已接入节点</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {(nodes.data ?? []).map((node) => (
            <div key={node.node_id} className="flex items-center justify-between rounded-md border p-3">
              <div>
                <p className="font-medium">{node.name || node.node_id}</p>
                <p className="text-sm text-muted-foreground">{node.supported_providers.join(", ") || "无 Provider"}</p>
              </div>
              <Badge variant={node.status === "online" ? "default" : "secondary"}>{node.status}</Badge>
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  );
}
```

- [ ] **Step 5: Route to feature page**

Modify `apps/web/src/routes/_authenticated/runtime/index.tsx`:

```tsx
import { createFileRoute } from "@tanstack/react-router";
import { RuntimeNodesPage } from "@/features/runtime";

export const Route = createFileRoute("/_authenticated/runtime/")({
  component: RuntimeNodesPage,
});
```

- [ ] **Step 6: Run web tests**

Run:

```bash
pnpm --filter @superteam/web test -- runtime
pnpm --filter @superteam/web typecheck
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/web/src/lib/api/runtime.ts apps/web/src/lib/api/runtime.test.ts apps/web/src/features/runtime apps/web/src/routes/_authenticated/runtime/index.tsx
git commit -m "feat: add runtime enrollment web page"
```

---

## Task 9: Build Digital Employee Web Page and Final Verification

**Files:**
- Create: `apps/web/src/lib/api/employees.ts`
- Modify: `apps/web/src/lib/api/index.ts`
- Create: `apps/web/src/features/employees/index.tsx`
- Create: `apps/web/src/features/employees/index.test.tsx`
- Modify: `apps/web/src/routes/_authenticated/employees/index.tsx`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add employees API client**

Create `apps/web/src/lib/api/employees.ts`:

```ts
import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type DigitalEmployeeStatus = "draft" | "ready" | "active" | "disabled" | "error";

export type DigitalEmployee = {
  id: string;
  name: string;
  description?: string;
  status: DigitalEmployeeStatus;
  execution_instance?: {
    id: string;
    runtime_node_id: string;
    provider_type: string;
    agent_home_dir?: string;
    session_policy: "new" | "resume" | "reuse_latest" | "ephemeral";
    status: string;
  };
  created_at?: string;
  updated_at?: string;
};

export async function listDigitalEmployees(options: ApiClientOptions): Promise<DigitalEmployee[]> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/v1/digital-employees"), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "GET",
  });
  return parseJson<DigitalEmployee[]>(response, "digital employees");
}

export async function createDigitalEmployee(options: ApiClientOptions, input: { name: string; description?: string }): Promise<DigitalEmployee> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/v1/digital-employees"), {
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
    body: JSON.stringify(input),
  });
  return parseJson<DigitalEmployee>(response, "create digital employee");
}
```

Modify `apps/web/src/lib/api/index.ts`:

```ts
export * from "./employees";
```

- [ ] **Step 2: Add employees page**

Create `apps/web/src/features/employees/index.tsx`:

```tsx
import { useQuery } from "@tanstack/react-query";
import { Bot } from "lucide-react";
import { listDigitalEmployees } from "@/lib/api/employees";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export function EmployeesPage() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const employees = useQuery({ queryKey: ["digital-employees"], queryFn: () => listDigitalEmployees({ baseUrl: apiBaseUrl }) });

  return (
    <div className="space-y-4 p-4">
      <div>
        <h1 className="text-2xl font-semibold">数字员工</h1>
        <p className="text-sm text-muted-foreground">管理数字员工业务身份和唯一执行实例。</p>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>员工列表</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {(employees.data ?? []).map((employee) => (
            <div key={employee.id} className="flex items-center justify-between rounded-md border p-3">
              <div className="flex items-center gap-3">
                <Bot className="size-4" />
                <div>
                  <p className="font-medium">{employee.name}</p>
                  <p className="text-sm text-muted-foreground">{employee.description || "未配置描述"}</p>
                </div>
              </div>
              <Badge variant={employee.status === "active" ? "default" : "secondary"}>{employee.status}</Badge>
            </div>
          ))}
          {employees.data?.length === 0 ? <p className="text-sm text-muted-foreground">暂无数字员工。</p> : null}
        </CardContent>
      </Card>
    </div>
  );
}
```

- [ ] **Step 3: Route to employees page**

Modify `apps/web/src/routes/_authenticated/employees/index.tsx`:

```tsx
import { createFileRoute } from "@tanstack/react-router";
import { EmployeesPage } from "@/features/employees";

export const Route = createFileRoute("/_authenticated/employees/")({
  component: EmployeesPage,
});
```

- [ ] **Step 4: Add changelog entry**

Append to `CHANGELOG.md`:

```md
## 2026-06-02

- 新增 Runtime 自发现接入设计的首批实现：Runtime enrollment、短期 session、能力上报和 Web 接入入口。
- 新增数字员工唯一执行实例主链路：数字员工草稿、执行实例绑定、Provider Session 事件模型和 Web 管理入口。
- 调整 Runtime Agent 配置主线：长期保存环境级 bootstrap key，不再把长期 runtime token 作为正式接入前提。
```

- [ ] **Step 5: Run full verification**

Run:

```bash
pnpm verify:contracts
pnpm test:go
pnpm test:rust
pnpm --filter @superteam/web test
pnpm --filter @superteam/web typecheck
pnpm --filter @superteam/web build
```

Expected:

- `verify:contracts` PASS.
- Go tests PASS, or storage query integration tests skip when `TEST_DATABASE_URL` / `TEST_REDIS_URL` are unset.
- Rust tests PASS.
- Web tests/typecheck/build PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/lib/api apps/web/src/features/employees apps/web/src/routes/_authenticated/employees/index.tsx CHANGELOG.md
git commit -m "feat: add digital employee execution web page"
```

---

## Self-Review

Spec coverage:

- Runtime self-discovery via `bootstrap_key`: Task 1, Task 2, Task 3, Task 5.
- Pending approval and Web接入: Task 3, Task 8.
- Short runtime sessions and renewal: Task 1, Task 2, Task 3, Task 5.
- Outbound WebSocket command channel: Task 4, Task 7.
- Runtime capabilities: Task 1, Task 5, Task 8.
- Digital employee unique execution instance: Task 1, Task 6, Task 9.
- Provider Session new/resume/reuse model: Task 1, Task 7.
- Provider output only through Runtime Agent: Task 7.
- Web page split between Runtime nodes and Digital employees: Task 8, Task 9.
- Changelog and verification: Task 9.

Type consistency:

- Runtime external key remains `node_id`.
- Runtime database identity uses `runtime_node_id` UUID.
- Digital employee identity uses `digital_employee_id` UUID.
- Execution instance identity uses `execution_instance_id` UUID.
- Provider session external ID remains `provider_session_id`.
- One digital employee has one execution instance through `UNIQUE (digital_employee_id)`.

Known implementation notes:

- Bootstrap keys are stored as bcrypt hashes. The service must call `ListActiveBootstrapKeys` and compare each candidate with `VerifyRuntimeSecret`; do not try to hash the presented plaintext and look it up by equality.
- The OpenAPI snippets in this plan identify required paths and schemas but must be merged carefully into the existing `openapi.yaml` without removing current task/runtime paths.
- Existing `runtime_node_scopes` powers the current permissions center. Keep it until a separate migration moves scope semantics onto digital employees or execution instances.
