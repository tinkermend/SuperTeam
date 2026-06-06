# 数字员工创建闭环 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建成数字员工创建闭环：Web 四步向导提交创建意图，Control Plane 写入 Owner、专业类型、个人配置、生效配置和唯一执行实例，并通过 Runtime provisioning 后返回 `ready`。

**Architecture:** Web 只负责展示服务端候选和收集结构化选择；Control Plane 是创建编排者和业务事实源，负责专业类型注册表、团队治理校验、Runtime/Provider 候选、Owner 注入、配置合成和 provisioning。Runtime Agent 只接收 `provision_instance` 命令，不承载数字员工业务身份、审批策略或长期状态。

**Tech Stack:** Go + chi/net/http + pgx + sqlc + Atlas migrations + OpenAPI/oapi-codegen；React + TanStack Query + TanStack Router + React Hook Form + Zod + shadcn/ui + lucide-react + Vitest。

---

## 文件结构

本计划只覆盖创建闭环，不实现数字员工概览健康大盘，不实现项目协调员创建，不实现 AGENTS.md 个人编辑器，不实现能力同步或导入导出。

后端新增或修改：

- Create: `apps/control-plane/internal/storage/migrations/008_digital_employee_creation_ready.sql`  
  新增 `digital_employees.owner_user_id`、`digital_employees.employee_type`、索引和中文注释。
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`  
  由 `atlas migrate hash` 更新迁移校验。
- Modify: `apps/control-plane/internal/storage/migrations_test.go`  
  覆盖新迁移的列、索引、注释和不使用 DB enum 的约束。
- Modify: `apps/control-plane/internal/storage/queries/employee_execution.sql`  
  扩展 `CreateDigitalEmployee`、`ListDigitalEmployees`、`GetDigitalEmployee`，新增 Runtime/Provider 创建候选查询。
- Reuse: `apps/control-plane/internal/storage/queries/digital_employee_config.sql`  
  复用既有 `GetCurrentDigitalEmployeeConfigRevision` 和 `CreateDigitalEmployeeConfigRevision` 查询，不为本轮新增查询。
- Generated: `apps/control-plane/internal/storage/queries/*.sql.go`、`apps/control-plane/internal/storage/queries/models.go`、`apps/control-plane/internal/storage/queries/querier.go`  
  由 `make generate-sqlc` 生成。
- Modify: `apps/control-plane/internal/employee/types.go`  
  增加 Owner、专业类型、创建候选、创建请求字段和 active config revision 状态。
- Create: `apps/control-plane/internal/employee/employee_types.go`  
  内置专业类型注册表，不包含项目协调员。
- Modify: `apps/control-plane/internal/employee/repository.go`  
  增加 Owner/专业类型参数和创建候选读取接口。
- Modify: `apps/control-plane/internal/employee/pg_repository.go`  
  映射新增列和候选 SQL。
- Modify: `apps/control-plane/internal/employee/service.go`  
  将创建从 “draft + provisioning” 升级为有界同步的 “ready 创建编排”，DB 本地事实先事务提交，Runtime 等待不持有事务。
- Modify: `apps/control-plane/internal/employee/service_test.go`  
  补服务层测试。
- Modify: `apps/control-plane/internal/app/app.go`  
  给 employee repository 注入 Postgres transaction beginner。
- Modify: `apps/control-plane/internal/employee/handler.go`  
  增加 `create-options` handler，创建时从登录上下文注入 `owner_user_id`。
- Modify: `apps/control-plane/internal/api/server.go`  
  在 `/{employeeId}` 前注册 `/digital-employees/create-options`。
- Modify: `apps/control-plane/internal/api/employee_routes_test.go`  
  覆盖 create-options、Owner 防伪造、路由顺序和授权。
- Modify: `contracts/control-plane/openapi.yaml`  
  增加 create-options path/schema，扩展员工和创建请求 schema。
- Generated: `apps/control-plane/internal/api/gen/control_plane.gen.go`、`apps/control-plane/gen/control_plane.gen.go`  
  由 `pnpm generate:control-plane` 生成。

前端新增或修改：

- Modify: `apps/web/src/lib/api/employees.ts`  
  增加创建候选和完整创建请求类型。
- Modify: `apps/web/src/lib/api/employees.test.ts`  
  覆盖候选请求和创建请求 body。
- Create: `apps/web/src/features/employees/create.tsx`  
  四步创建向导页面。
- Create: `apps/web/src/features/employees/create.test.tsx`  
  覆盖向导路径、禁用候选、提交 body 和成功跳转。
- Modify: `apps/web/src/features/employees/index.tsx`  
  移除内嵌草稿创建面板，列表页只保留创建入口。
- Modify: `apps/web/src/features/employees/index.test.tsx`  
  更新列表页测试。
- Create: `apps/web/src/routes/_authenticated/employees/new.tsx`  
  TanStack Router 创建页。
- Generated: `apps/web/src/routeTree.gen.ts`  
  由 Vite/TanStack Router 插件在测试或构建时生成。
- Modify: `CHANGELOG.md`  
  添加本次实现变更，时间使用 `Asia/Shanghai`。

## 评审修正原则

- 创建接口仍保持同步返回 `ready`，因为本轮产品语义要求“创建成功即准备好可被调用”；但等待 Runtime provisioning 必须有明确上限，首版沿用服务现有 `defaultProvisioningTimeout = 10 * time.Second`，超时返回 `503` 并执行补偿清理。
- 数据库本地事实必须有事务边界：employee、active config revision、approved effective config、execution instance、runtime command receipt 在同一事务中写入；事务提交后才下发 Runtime 命令，等待 Runtime 回写期间不持有 DB transaction。
- Runtime dispatch、provisioning timeout 或 terminal failed 后，补偿清理必须同时处理 employee、execution instance、runtime receipt、config revision 和 effective config，避免留下可见的半成品。
- Web 向导每一步进入下一步前只校验当前步骤字段；Runtime/Provider 只有一个可用候选时才自动选择，多个候选必须由用户显式选择。
- `employee_type` 可被团队治理用 `allowed_employee_types` 过滤；未配置该策略时返回全部服务端注册类型，创建接口仍做最终校验。

## Task 1: 数据库迁移和 sqlc 查询

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/008_digital_employee_creation_ready.sql`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
- Modify: `apps/control-plane/internal/storage/queries/employee_execution.sql`
- Generated: `apps/control-plane/internal/storage/queries/*.sql.go`

- [ ] **Step 1: 写迁移测试**

在 `apps/control-plane/internal/storage/migrations_test.go` 新增：

```go
func TestDigitalEmployeeCreationMigrationAddsOwnerAndType(t *testing.T) {
	body, err := os.ReadFile("migrations/008_digital_employee_creation_ready.sql")
	if err != nil {
		t.Fatalf("read digital employee creation migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"ADD COLUMN IF NOT EXISTS owner_user_id UUID",
		"ADD COLUMN IF NOT EXISTS employee_type VARCHAR(100)",
		"ALTER COLUMN owner_user_id SET NOT NULL",
		"ALTER COLUMN employee_type SET NOT NULL",
		"CREATE INDEX IF NOT EXISTS idx_digital_employees_owner_status",
		"ON digital_employees(tenant_id, owner_user_id, status, created_at DESC)",
		"CREATE INDEX IF NOT EXISTS idx_digital_employees_type_status",
		"ON digital_employees(tenant_id, employee_type, status, created_at DESC)",
		"COMMENT ON COLUMN digital_employees.owner_user_id IS '数字员工归属人类用户ID，由控制平面从登录上下文写入'",
		"COMMENT ON COLUMN digital_employees.employee_type IS '数字员工专业类型，由服务端注册表校验，不使用数据库枚举'",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected digital employee creation migration to contain %q", expected)
		}
	}

	for _, forbidden := range []string{
		"CREATE TYPE employee_type",
		"CHECK (employee_type IN",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("employee_type must stay registry-backed, found %q", forbidden)
		}
	}
}
```

- [ ] **Step 2: 运行迁移测试并确认失败**

Run: `go test ./apps/control-plane/internal/storage -run TestDigitalEmployeeCreationMigrationAddsOwnerAndType -count=1`

Expected: FAIL，错误包含 `read digital employee creation migration`。

- [ ] **Step 3: 新增 forward migration**

创建 `apps/control-plane/internal/storage/migrations/008_digital_employee_creation_ready.sql`：

```sql
ALTER TABLE digital_employees
    ADD COLUMN IF NOT EXISTS owner_user_id UUID,
    ADD COLUMN IF NOT EXISTS employee_type VARCHAR(100);

UPDATE digital_employees de
SET owner_user_id = COALESCE(
    (
        SELECT tm.principal_id
        FROM tenant_members tm
        WHERE tm.tenant_id = de.tenant_id
          AND tm.principal_type = 'user'
          AND tm.status = 'active'
          AND tm.disabled_at IS NULL
          AND (
              tm.team_id = de.team_id
              OR tm.team_id IS NULL
          )
          AND tm.role IN ('owner', 'admin', 'maintainer')
        ORDER BY
          CASE WHEN tm.team_id = de.team_id THEN 0 ELSE 1 END,
          tm.created_at ASC
        LIMIT 1
    ),
    (
        SELECT au.id
        FROM auth_users au
        WHERE au.username = 'admin'
          AND au.deleted_at IS NULL
        ORDER BY au.created_at ASC
        LIMIT 1
    )
)
WHERE de.owner_user_id IS NULL;

UPDATE digital_employees
SET employee_type = CASE
    WHEN role ILIKE '%database%' OR role ILIKE '%db%' OR role ILIKE '%dba%' OR role ILIKE '%mysql%' OR role ILIKE '%postgres%' OR role ILIKE '%postgresql%' OR role ILIKE '%数据库%' THEN 'database_admin'
    WHEN role ILIKE '%devops%' OR role ILIKE '%ops%' OR role ILIKE '%sre%' OR role ILIKE '%platform%' OR role ILIKE '%infra%' OR role ILIKE '%运维%' THEN 'devops_engineer'
    WHEN role ILIKE '%frontend%' OR role ILIKE '%front-end%' OR role ILIKE '%前端%' THEN 'frontend_engineer'
    WHEN role ILIKE '%backend%' OR role ILIKE '%back-end%' OR role ILIKE '%server%' OR role ILIKE '%api%' OR role ILIKE '%后端%' THEN 'backend_engineer'
    WHEN role ILIKE '%fullstack%' OR role ILIKE '%full-stack%' OR role ILIKE '%全栈%' THEN 'fullstack_engineer'
    WHEN role ILIKE '%implementation%' OR role ILIKE '%implement%' OR role ILIKE '%delivery%' OR role ILIKE '%实施%' OR role ILIKE '%交付%' THEN 'implementation_engineer'
    ELSE 'general_engineer'
END
WHERE employee_type IS NULL OR employee_type = '';

ALTER TABLE digital_employees
    ALTER COLUMN owner_user_id SET NOT NULL,
    ALTER COLUMN employee_type SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_digital_employees_owner_status
    ON digital_employees(tenant_id, owner_user_id, status, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_digital_employees_type_status
    ON digital_employees(tenant_id, employee_type, status, created_at DESC)
    WHERE deleted_at IS NULL;

COMMENT ON COLUMN digital_employees.owner_user_id IS '数字员工归属人类用户ID，由控制平面从登录上下文写入';
COMMENT ON COLUMN digital_employees.employee_type IS '数字员工专业类型，由服务端注册表校验，不使用数据库枚举';
COMMENT ON INDEX idx_digital_employees_owner_status IS '按归属人和状态查询数字员工';
COMMENT ON INDEX idx_digital_employees_type_status IS '按专业类型和状态查询数字员工';
```

- [ ] **Step 4: 生产迁移前 dry-run 回填检查**

在真实环境应用迁移前，先运行下列只读查询，确认历史 `role` 会如何映射。`general_engineer` 不是错误，但如果比例异常，需要先补映射词再生成最终迁移。

Run:

```bash
psql "$DATABASE_URL" -c "
SELECT inferred_employee_type, COUNT(*) AS employee_count, array_agg(role ORDER BY role) AS sample_roles
FROM (
    SELECT role,
        CASE
            WHEN role ILIKE '%database%' OR role ILIKE '%db%' OR role ILIKE '%dba%' OR role ILIKE '%mysql%' OR role ILIKE '%postgres%' OR role ILIKE '%postgresql%' OR role ILIKE '%数据库%' THEN 'database_admin'
            WHEN role ILIKE '%devops%' OR role ILIKE '%ops%' OR role ILIKE '%sre%' OR role ILIKE '%platform%' OR role ILIKE '%infra%' OR role ILIKE '%运维%' THEN 'devops_engineer'
            WHEN role ILIKE '%frontend%' OR role ILIKE '%front-end%' OR role ILIKE '%前端%' THEN 'frontend_engineer'
            WHEN role ILIKE '%backend%' OR role ILIKE '%back-end%' OR role ILIKE '%server%' OR role ILIKE '%api%' OR role ILIKE '%后端%' THEN 'backend_engineer'
            WHEN role ILIKE '%fullstack%' OR role ILIKE '%full-stack%' OR role ILIKE '%全栈%' THEN 'fullstack_engineer'
            WHEN role ILIKE '%implementation%' OR role ILIKE '%implement%' OR role ILIKE '%delivery%' OR role ILIKE '%实施%' OR role ILIKE '%交付%' THEN 'implementation_engineer'
            ELSE 'general_engineer'
        END AS inferred_employee_type
    FROM digital_employees
    WHERE deleted_at IS NULL
) mapped
GROUP BY inferred_employee_type
ORDER BY inferred_employee_type;"
```

Expected: 查询成功；人工确认 `general_engineer` 样本角色可接受。

- [ ] **Step 5: 扩展员工写读 SQL**

修改 `apps/control-plane/internal/storage/queries/employee_execution.sql` 的 `CreateDigitalEmployee`，插入 `owner_user_id` 和 `employee_type`：

```sql
-- name: CreateDigitalEmployee :one
INSERT INTO digital_employees (
    tenant_id,
    team_id,
    owner_user_id,
    employee_type,
    name,
    role,
    description,
    status,
    permission_policy,
    context_policy,
    approval_policy,
    risk_level,
    metadata
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.narg('team_id')::uuid,
    sqlc.arg('owner_user_id')::uuid,
    sqlc.arg('employee_type')::varchar,
    sqlc.arg('name')::varchar,
    sqlc.arg('role')::varchar,
    sqlc.narg('description')::text,
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.arg('permission_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('context_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('approval_policy')::jsonb, '{}'::jsonb),
    sqlc.arg('risk_level')::varchar,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
) RETURNING *;
```

`SELECT *` 查询会自动包含新列，sqlc 生成后需要在 repository mapping 中处理。

- [ ] **Step 6: 增加创建候选 Runtime/Provider SQL**

在 `apps/control-plane/internal/storage/queries/employee_execution.sql` 的 preflight 查询前新增：

```sql
-- name: ListRuntimeProviderOptionsForDigitalEmployeeCreate :many
WITH active_team_config AS (
    SELECT *
    FROM tenant_team_config_revisions
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND team_id = sqlc.arg('team_id')::uuid
      AND status = 'active'
      AND archived_at IS NULL
    ORDER BY revision_number DESC
    LIMIT 1
),
runtime_sessions_active AS (
    SELECT DISTINCT re.runtime_node_id
    FROM runtime_sessions rs
    JOIN runtime_enrollments re
      ON re.id = rs.enrollment_id
     AND re.tenant_id = rs.tenant_id
     AND re.runtime_node_id = rs.runtime_node_id
     AND re.status = 'approved'
     AND re.rejected_at IS NULL
     AND re.revoked_at IS NULL
    WHERE rs.tenant_id = sqlc.arg('tenant_id')::uuid
      AND rs.expires_at > NOW()
      AND rs.revoked_at IS NULL
),
provider_capabilities AS (
    SELECT DISTINCT ON (tenant_id, runtime_node_id, provider_type)
        *
    FROM runtime_capabilities
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND capability_type = 'provider'
      AND disabled_at IS NULL
      AND archived_at IS NULL
    ORDER BY tenant_id, runtime_node_id, provider_type, last_seen_at DESC NULLS LAST, updated_at DESC
)
SELECT
    rn.id AS runtime_node_id,
    rn.node_id,
    rn.name AS runtime_name,
    pc.provider_type,
    rn.status AS runtime_status,
    pc.status AS provider_status,
    pc.health_status,
    rn.current_load,
    rn.max_slots,
    COALESCE(
        pc.details ->> 'agent_home_dir',
        pc.metadata ->> 'agent_home_dir',
        pc.workspace_base_dir,
        rn.metadata ->> 'agent_home_dir',
        ''
    )::text AS agent_home_dir,
    (
        active_team_config.id IS NOT NULL
        AND rn.status = 'online'
        AND rn.disabled_at IS NULL
        AND rn.archived_at IS NULL
        AND pc.available = true
        AND pc.status = 'healthy'
        AND pc.health_status = 'healthy'
        AND runtime_sessions_active.runtime_node_id IS NOT NULL
        AND (
            (
                jsonb_typeof(active_team_config.capability_policy -> 'allowed_provider_types') = 'array'
                AND (active_team_config.capability_policy -> 'allowed_provider_types') ? pc.provider_type
            )
            OR (
                jsonb_typeof(active_team_config.runtime_scope_policy -> 'allowed_provider_types') = 'array'
                AND (active_team_config.runtime_scope_policy -> 'allowed_provider_types') ? pc.provider_type
            )
            OR (
                jsonb_typeof(active_team_config.runtime_scope_policy -> 'provider_types') = 'array'
                AND (active_team_config.runtime_scope_policy -> 'provider_types') ? pc.provider_type
            )
        )
        AND CASE
            WHEN NOT (active_team_config.runtime_scope_policy ? 'allowed_runtime_node_ids') THEN true
            WHEN jsonb_typeof(active_team_config.runtime_scope_policy -> 'allowed_runtime_node_ids') = 'array' THEN
                (active_team_config.runtime_scope_policy -> 'allowed_runtime_node_ids') ? rn.id::text
            ELSE false
        END
        AND CASE
            WHEN NOT (active_team_config.runtime_scope_policy ? 'allowed_node_ids') THEN true
            WHEN jsonb_typeof(active_team_config.runtime_scope_policy -> 'allowed_node_ids') = 'array' THEN
                (active_team_config.runtime_scope_policy -> 'allowed_node_ids') ? rn.node_id
            ELSE false
        END
    )::boolean AS available,
    CASE
        WHEN active_team_config.id IS NULL THEN 'active_team_config_required'
        WHEN rn.status <> 'online' OR rn.disabled_at IS NOT NULL OR rn.archived_at IS NOT NULL THEN 'runtime_not_online'
        WHEN runtime_sessions_active.runtime_node_id IS NULL THEN 'runtime_session_inactive'
        WHEN pc.id IS NULL THEN 'provider_missing'
        WHEN pc.available = false OR pc.status <> 'healthy' OR pc.health_status <> 'healthy' THEN 'provider_unhealthy'
        WHEN COALESCE(pc.provider_type, '') = '' THEN 'provider_type_missing'
        WHEN NOT (
            (
                jsonb_typeof(active_team_config.capability_policy -> 'allowed_provider_types') = 'array'
                AND (active_team_config.capability_policy -> 'allowed_provider_types') ? pc.provider_type
            )
            OR (
                jsonb_typeof(active_team_config.runtime_scope_policy -> 'allowed_provider_types') = 'array'
                AND (active_team_config.runtime_scope_policy -> 'allowed_provider_types') ? pc.provider_type
            )
            OR (
                jsonb_typeof(active_team_config.runtime_scope_policy -> 'provider_types') = 'array'
                AND (active_team_config.runtime_scope_policy -> 'provider_types') ? pc.provider_type
            )
        ) THEN 'provider_outside_team_policy'
        ELSE ''
    END::varchar AS disabled_reason
FROM runtime_nodes rn
LEFT JOIN provider_capabilities pc
  ON pc.runtime_node_id = rn.id
 AND pc.tenant_id = rn.tenant_id
LEFT JOIN active_team_config ON TRUE
LEFT JOIN runtime_sessions_active ON runtime_sessions_active.runtime_node_id = rn.id
WHERE rn.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pc.provider_type IS NOT NULL
ORDER BY available DESC, rn.name ASC, pc.provider_type ASC;
```

- [ ] **Step 7: 扩展 provisioning abort SQL 清理半成品配置**

修改 `apps/control-plane/internal/storage/queries/employee_execution.sql` 的 `AbortProvisionedDigitalEmployee`，在现有 CTE 中加入 config/effective config 清理：

```sql
aborted_configs AS (
    UPDATE digital_employee_config_revisions
    SET archived_at = COALESCE(archived_at, NOW()),
        updated_at = NOW()
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
      AND archived_at IS NULL
    RETURNING id
),
aborted_effective_configs AS (
    UPDATE digital_employee_effective_configs
    SET status = CASE
            WHEN status = 'revoked' THEN status
            ELSE 'revoked'
        END,
        revoked_at = COALESCE(revoked_at, NOW()),
        updated_at = NOW()
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
      AND revoked_at IS NULL
    RETURNING id
)
```

Expected: 任一失败路径调用 `AbortProvisionedDigitalEmployee` 后，不会留下未归档 config revision 或仍 approved 的 effective config。

- [ ] **Step 8: 生成 sqlc 代码**

Run: `cd apps/control-plane && make generate-sqlc`

Expected: PASS，输出包含 `sqlc generation complete`，并更新 `internal/storage/queries/*.sql.go`。

- [ ] **Step 9: 更新 Atlas hash**

Run: `cd apps/control-plane && atlas migrate hash --dir file://internal/storage/migrations`

Expected: PASS，`apps/control-plane/internal/storage/migrations/atlas.sum` 增加 `008_digital_employee_creation_ready.sql`。

- [ ] **Step 10: 运行数据库和查询测试**

Run: `go test ./apps/control-plane/internal/storage ./apps/control-plane/internal/storage/queries -count=1`

Expected: PASS。

- [ ] **Step 11: 提交数据库层**

```bash
git add \
  apps/control-plane/internal/storage/migrations/008_digital_employee_creation_ready.sql \
  apps/control-plane/internal/storage/migrations/atlas.sum \
  apps/control-plane/internal/storage/migrations_test.go \
  apps/control-plane/internal/storage/queries
git commit -m "feat: add digital employee creation schema"
```

## Task 2: 专业类型注册表和创建候选服务

**Files:**
- Create: `apps/control-plane/internal/employee/employee_types.go`
- Modify: `apps/control-plane/internal/employee/types.go`
- Modify: `apps/control-plane/internal/employee/repository.go`
- Modify: `apps/control-plane/internal/employee/pg_repository.go`
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/control-plane/internal/employee/service_test.go`

- [ ] **Step 1: 写专业类型注册表测试**

在 `apps/control-plane/internal/employee/service_test.go` 新增：

```go
func TestEmployeeTypeRegistryExcludesProjectCoordinator(t *testing.T) {
	types := DefaultEmployeeTypeDefinitions()
	if len(types) < 6 {
		t.Fatalf("expected professional engineer types, got %#v", types)
	}
	for _, item := range types {
		if strings.Contains(item.Type, "coordinator") || strings.Contains(item.Label, "协调") {
			t.Fatalf("project coordinator must not be a reusable employee type: %#v", item)
		}
	}
	if _, ok := EmployeeTypeDefinitionByType("database_admin"); !ok {
		t.Fatalf("expected database_admin type")
	}
	if _, ok := EmployeeTypeDefinitionByType("devops_engineer"); !ok {
		t.Fatalf("expected devops_engineer type")
	}
}
```

- [ ] **Step 2: 写 create-options 服务测试**

在 `apps/control-plane/internal/employee/service_test.go` 新增：

```go
func TestGetCreateOptionsReturnsTeamPolicyAndRuntimeCandidates(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	teamConfigID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.teams[teamID] = tenantID
	repo.teamConfigs[teamConfigID] = TeamConfigInput{
		ID:             teamConfigID,
		TenantID:       tenantID,
		TeamID:         teamID,
		RevisionNumber: 4,
		Status:         TeamConfigRevisionStatusActive,
		CapabilityPolicy: map[string]any{
			"allowed_skills":         []any{"database-troubleshooting", "incident-diagnosis"},
			"allowed_mcp_servers":    []any{"postgres-readonly"},
			"allowed_provider_types": []any{"codex"},
			"allowed_employee_types": []any{"database_admin"},
		},
		ContextPolicy:  map[string]any{"sources": []any{"runbook", "monitoring"}},
		ApprovalPolicy: map[string]any{"min_risk_for_human": "high"},
	}
	repo.currentTeamConfigByTeam[teamID] = teamConfigID
	repo.runtimeProviderOptions = []RuntimeProviderOption{{
		RuntimeNodeID:          runtimeNodeID,
		NodeID:                 "node-ops-01",
		RuntimeName:            "运维节点 01",
		ProviderType:           "codex",
		RuntimeStatus:          "online",
		ProviderStatus:         "healthy",
		HealthStatus:           "healthy",
		CurrentLoad:            1,
		MaxSlots:               4,
		AgentHomeDir:           "/srv/superteam/agents",
		AgentHomeDirAvailable:  true,
		Available:              true,
		DisabledReason:         "",
	}}

	options, err := svc.GetCreateOptions(context.Background(), CreateOptionsRequest{
		TenantID: tenantID,
		TeamID:   teamID,
	})
	if err != nil {
		t.Fatalf("get create options: %v", err)
	}

	if options.TeamConfig.ID != teamConfigID || options.TeamConfig.RevisionNumber != 4 {
		t.Fatalf("unexpected team config option: %#v", options.TeamConfig)
	}
	if got := options.TeamConfig.AllowedEmployeeTypes; len(got) != 1 || got[0] != "database_admin" {
		t.Fatalf("expected allowed employee types from policy, got %#v", got)
	}
	if len(options.EmployeeTypes) != 1 || options.EmployeeTypes[0].Type != "database_admin" {
		t.Fatalf("expected filtered employee type database_admin, got %#v", options.EmployeeTypes)
	}
	if len(options.RuntimeProviderOptions) != 1 || !options.RuntimeProviderOptions[0].Available {
		t.Fatalf("expected available runtime provider option, got %#v", options.RuntimeProviderOptions)
	}
	if got := options.CapabilityOptions.ProviderTypes; len(got) != 1 || got[0] != "codex" {
		t.Fatalf("expected provider type from team policy, got %#v", got)
	}
}
```

- [ ] **Step 3: 运行服务测试并确认失败**

Run: `go test ./apps/control-plane/internal/employee -run 'TestEmployeeTypeRegistryExcludesProjectCoordinator|TestGetCreateOptionsReturnsTeamPolicyAndRuntimeCandidates' -count=1`

Expected: FAIL，错误包含缺失类型或方法。

- [ ] **Step 4: 增加领域类型**

在 `apps/control-plane/internal/employee/types.go` 增加：

```go
const (
	ConfigRevisionStatusDraft  ConfigRevisionStatus = "draft"
	ConfigRevisionStatusActive ConfigRevisionStatus = "active"
)

type EmployeeTypeDefinition struct {
	Type                                 string         `json:"type"`
	Label                                string         `json:"label"`
	Description                          string         `json:"description"`
	DefaultRole                          string         `json:"default_role"`
	DefaultRiskLevel                     string         `json:"default_risk_level"`
	DefaultRoleProfile                   map[string]any `json:"default_role_profile"`
	RecommendedSkillKeys                 []string       `json:"recommended_skill_keys"`
	RecommendedMCPServers                []string       `json:"recommended_mcp_servers"`
	RecommendedExternalCapabilities      []string       `json:"recommended_external_capabilities"`
	RecommendedProviderTypes             []string       `json:"recommended_provider_types"`
	DefaultContextPolicyOverride          map[string]any `json:"default_context_policy_override"`
	DefaultApprovalPolicyOverride         map[string]any `json:"default_approval_policy_override"`
	DefaultOutputContractAddendum         map[string]any `json:"default_output_contract_addendum"`
}

type TeamConfigCreateOption struct {
	ID                     uuid.UUID      `json:"id"`
	RevisionNumber         int32          `json:"revision_number"`
	Status                 string         `json:"status"`
	AllowedEmployeeTypes   []string       `json:"allowed_employee_types"`
	AllowedProviderTypes   []string       `json:"allowed_provider_types"`
	AllowedSkills          []string       `json:"allowed_skills"`
	AllowedMCPServers      []string       `json:"allowed_mcp_servers"`
	AllowedExternalCaps    []string       `json:"allowed_external_capabilities"`
	CapabilityPolicy       map[string]any `json:"capability_policy"`
	ContextPolicy          map[string]any `json:"context_policy"`
	ApprovalPolicy         map[string]any `json:"approval_policy"`
	RuntimeScopePolicy     map[string]any `json:"runtime_scope_policy"`
}

type CapabilityOptions struct {
	Skills               []string `json:"skills"`
	MCPServers           []string `json:"mcp_servers"`
	ExternalCapabilities []string `json:"external_capabilities"`
	ProviderTypes        []string `json:"provider_types"`
}

type RuntimeProviderOption struct {
	RuntimeNodeID          uuid.UUID `json:"runtime_node_id"`
	NodeID                 string    `json:"node_id"`
	RuntimeName            string    `json:"runtime_name"`
	ProviderType           string    `json:"provider_type"`
	RuntimeStatus          string    `json:"runtime_status"`
	ProviderStatus         string    `json:"provider_status"`
	HealthStatus           string    `json:"health_status"`
	CurrentLoad            int32     `json:"current_load"`
	MaxSlots               int32     `json:"max_slots"`
	AgentHomeDir           string    `json:"agent_home_dir"`
	AgentHomeDirAvailable  bool      `json:"agent_home_dir_available"`
	Available              bool      `json:"available"`
	DisabledReason         string    `json:"disabled_reason"`
}

type PolicyDefaults struct {
	ContextPolicyOverride  map[string]any `json:"context_policy_override"`
	ApprovalPolicyOverride map[string]any `json:"approval_policy_override"`
	OutputContractAddendum map[string]any `json:"output_contract_addendum"`
}

type CreateOptionsRequest struct {
	TenantID uuid.UUID
	TeamID   uuid.UUID
}

type CreateOptions struct {
	TeamConfig             TeamConfigCreateOption  `json:"team_config"`
	EmployeeTypes          []EmployeeTypeDefinition `json:"employee_types"`
	CapabilityOptions      CapabilityOptions       `json:"capability_options"`
	RuntimeProviderOptions []RuntimeProviderOption `json:"runtime_provider_options"`
	PolicyDefaults         PolicyDefaults          `json:"policy_defaults"`
}
```

同时在 `DigitalEmployee`、`DigitalEmployeeRecord`、`CreateDigitalEmployeeParams` 增加：

```go
OwnerUserID  uuid.UUID
EmployeeType string
```

- [ ] **Step 5: 创建专业类型注册表**

创建 `apps/control-plane/internal/employee/employee_types.go`：

```go
package employee

func DefaultEmployeeTypeDefinitions() []EmployeeTypeDefinition {
	definitions := []EmployeeTypeDefinition{
		{
			Type:             "database_admin",
			Label:            "数据库管理",
			Description:      "负责数据库巡检、故障诊断、变更评估和运维证据整理。",
			DefaultRole:      "database_admin",
			DefaultRiskLevel: "medium",
			DefaultRoleProfile: map[string]any{
				"title": "数据库管理工程师",
				"mission": "围绕数据库可靠性、性能、变更风险和证据输出执行专业任务",
			},
			RecommendedSkillKeys:            []string{"database-troubleshooting", "sql-review"},
			RecommendedMCPServers:           []string{"postgres-readonly"},
			RecommendedExternalCapabilities: []string{"change-ticket-read"},
			RecommendedProviderTypes:        []string{"codex", "opencode"},
			DefaultOutputContractAddendum: map[string]any{
				"required_artifacts": []any{"diagnosis_report", "change_risk_summary"},
			},
		},
		{
			Type:             "devops_engineer",
			Label:            "DevOps 运维",
			Description:      "负责运行环境、部署流水线、监控告警和故障恢复任务。",
			DefaultRole:      "devops_engineer",
			DefaultRiskLevel: "high",
			DefaultRoleProfile: map[string]any{
				"title": "DevOps 运维工程师",
				"mission": "围绕节点、部署、监控和恢复执行受控运维任务",
			},
			RecommendedSkillKeys:            []string{"incident-diagnosis", "deployment-review"},
			RecommendedMCPServers:           []string{"observability-readonly"},
			RecommendedExternalCapabilities: []string{"ci-read", "ticket-write"},
			RecommendedProviderTypes:        []string{"codex", "opencode"},
			DefaultApprovalPolicyOverride: map[string]any{
				"write_actions_require_human": true,
			},
		},
		{
			Type:             "frontend_engineer",
			Label:            "前端开发",
			Description:      "负责前端需求拆解、界面实现、交互修复和前端测试。",
			DefaultRole:      "frontend_engineer",
			DefaultRiskLevel: "medium",
			DefaultRoleProfile: map[string]any{
				"title": "前端开发工程师",
				"mission": "围绕 Web 界面、组件、交互和测试执行专业任务",
			},
			RecommendedSkillKeys:     []string{"frontend-implementation", "ui-regression"},
			RecommendedProviderTypes: []string{"codex"},
		},
		{
			Type:             "backend_engineer",
			Label:            "后端开发",
			Description:      "负责后端接口、数据模型、集成逻辑和服务端测试。",
			DefaultRole:      "backend_engineer",
			DefaultRiskLevel: "medium",
			DefaultRoleProfile: map[string]any{
				"title": "后端开发工程师",
				"mission": "围绕 API、服务逻辑、数据访问和测试执行专业任务",
			},
			RecommendedSkillKeys:     []string{"backend-implementation", "api-contract-review"},
			RecommendedProviderTypes: []string{"codex", "opencode"},
		},
		{
			Type:             "fullstack_engineer",
			Label:            "全栈开发",
			Description:      "负责跨前后端的小型闭环实现、联调和回归验证。",
			DefaultRole:      "fullstack_engineer",
			DefaultRiskLevel: "medium",
			DefaultRoleProfile: map[string]any{
				"title": "全栈开发工程师",
				"mission": "围绕端到端功能闭环、联调和验证执行专业任务",
			},
			RecommendedSkillKeys:     []string{"fullstack-implementation", "integration-test"},
			RecommendedProviderTypes: []string{"codex"},
		},
		{
			Type:             "implementation_engineer",
			Label:            "实施工程师",
			Description:      "负责客户侧配置、数据核对、上线准备和交付证据整理。",
			DefaultRole:      "implementation_engineer",
			DefaultRiskLevel: "medium",
			DefaultRoleProfile: map[string]any{
				"title": "实施工程师",
				"mission": "围绕客户侧配置、联调、核验和交付材料执行任务",
			},
			RecommendedSkillKeys:            []string{"implementation-checklist", "data-reconciliation"},
			RecommendedExternalCapabilities: []string{"ticket-read", "doc-write"},
			RecommendedProviderTypes:        []string{"codex"},
		},
		{
			Type:             "general_engineer",
			Label:            "通用工程执行",
			Description:      "负责未细分专业的工程分析、执行和报告任务。",
			DefaultRole:      "general_engineer",
			DefaultRiskLevel: "medium",
			DefaultRoleProfile: map[string]any{
				"title": "通用工程执行员工",
				"mission": "围绕明确任务边界执行工程分析和交付",
			},
			RecommendedProviderTypes: []string{"codex", "opencode"},
		},
	}

	out := make([]EmployeeTypeDefinition, 0, len(definitions))
	for _, definition := range definitions {
		out = append(out, cloneEmployeeTypeDefinition(definition))
	}
	return out
}

func EmployeeTypeDefinitionByType(value string) (EmployeeTypeDefinition, bool) {
	for _, definition := range DefaultEmployeeTypeDefinitions() {
		if definition.Type == value {
			return cloneEmployeeTypeDefinition(definition), true
		}
	}
	return EmployeeTypeDefinition{}, false
}

func cloneEmployeeTypeDefinition(value EmployeeTypeDefinition) EmployeeTypeDefinition {
	value.DefaultRoleProfile = cloneMap(value.DefaultRoleProfile)
	value.RecommendedSkillKeys = stringList(value.RecommendedSkillKeys)
	value.RecommendedMCPServers = stringList(value.RecommendedMCPServers)
	value.RecommendedExternalCapabilities = stringList(value.RecommendedExternalCapabilities)
	value.RecommendedProviderTypes = stringList(value.RecommendedProviderTypes)
	value.DefaultContextPolicyOverride = cloneMap(value.DefaultContextPolicyOverride)
	value.DefaultApprovalPolicyOverride = cloneMap(value.DefaultApprovalPolicyOverride)
	value.DefaultOutputContractAddendum = cloneMap(value.DefaultOutputContractAddendum)
	return value
}
```

- [ ] **Step 6: 扩展 repository 接口和 PG 映射**

在 `apps/control-plane/internal/employee/repository.go` 增加：

```go
GetCurrentTeamConfigRevision(ctx context.Context, tenantID, teamID uuid.UUID) (TeamConfigInput, error)
ListRuntimeProviderOptionsForCreate(ctx context.Context, tenantID, teamID uuid.UUID) ([]RuntimeProviderOption, error)
```

在 `apps/control-plane/internal/employee/pg_repository.go`：

```go
func (r *PgRepository) GetCurrentTeamConfigRevision(ctx context.Context, tenantID, teamID uuid.UUID) (TeamConfigInput, error) {
	revision, err := r.q.GetCurrentTenantTeamConfigRevision(ctx, queries.GetCurrentTenantTeamConfigRevisionParams{
		TenantID: tenantID,
		TeamID:   teamID,
	})
	if err != nil {
		return TeamConfigInput{}, mapNoRows(err)
	}
	return teamConfigInputFromQuery(revision)
}

func (r *PgRepository) ListRuntimeProviderOptionsForCreate(ctx context.Context, tenantID, teamID uuid.UUID) ([]RuntimeProviderOption, error) {
	rows, err := r.q.ListRuntimeProviderOptionsForDigitalEmployeeCreate(ctx, queries.ListRuntimeProviderOptionsForDigitalEmployeeCreateParams{
		TenantID: tenantID,
		TeamID:   teamID,
	})
	if err != nil {
		return nil, err
	}
	options := make([]RuntimeProviderOption, 0, len(rows))
	for _, row := range rows {
		options = append(options, RuntimeProviderOption{
			RuntimeNodeID:         row.RuntimeNodeID,
			NodeID:                row.NodeID,
			RuntimeName:           row.RuntimeName,
			ProviderType:          row.ProviderType,
			RuntimeStatus:         row.RuntimeStatus,
			ProviderStatus:        row.ProviderStatus.String,
			HealthStatus:          row.HealthStatus.String,
			CurrentLoad:           row.CurrentLoad,
			MaxSlots:              row.MaxSlots,
			AgentHomeDir:          row.AgentHomeDir,
			AgentHomeDirAvailable: strings.TrimSpace(row.AgentHomeDir) != "",
			Available:             row.Available,
			DisabledReason:        row.DisabledReason,
		})
	}
	return options, nil
}
```

同时更新 `CreateDigitalEmployee` 参数传递和 `digitalEmployeeRecordFromQuery` 映射：

```go
OwnerUserID:  params.OwnerUserID,
EmployeeType: params.EmployeeType,
```

```go
OwnerUserID:  employee.OwnerUserID,
EmployeeType: employee.EmployeeType,
```

- [ ] **Step 7: 实现 create-options 服务**

在 `apps/control-plane/internal/employee/service.go` 增加：

```go
func (s *Service) GetCreateOptions(ctx context.Context, req CreateOptionsRequest) (*CreateOptions, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if err := s.repository.EnsureTeamExists(ctx, req.TenantID, req.TeamID); err != nil {
		return nil, fmt.Errorf("get team: %w", err)
	}
	teamConfig, err := s.repository.GetCurrentTeamConfigRevision(ctx, req.TenantID, req.TeamID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("%w: active team governance config is required", ErrEffectiveConfigRequired)
		}
		return nil, fmt.Errorf("get current team config revision: %w", err)
	}
	runtimeOptions, err := s.repository.ListRuntimeProviderOptionsForCreate(ctx, req.TenantID, req.TeamID)
	if err != nil {
		return nil, fmt.Errorf("list runtime provider options: %w", err)
	}
	return &CreateOptions{
		TeamConfig: teamConfigCreateOption(teamConfig),
		EmployeeTypes: employeeTypesForTeamConfig(teamConfig),
		CapabilityOptions: capabilityOptionsFromTeamConfig(teamConfig),
		RuntimeProviderOptions: runtimeOptions,
		PolicyDefaults: PolicyDefaults{
			ContextPolicyOverride:  map[string]any{},
			ApprovalPolicyOverride: map[string]any{},
			OutputContractAddendum: map[string]any{},
		},
	}, nil
}
```

新增 helper：

```go
func teamConfigCreateOption(input TeamConfigInput) TeamConfigCreateOption {
	return TeamConfigCreateOption{
		ID:                   input.ID,
		RevisionNumber:       input.RevisionNumber,
		Status:               string(input.Status),
		AllowedEmployeeTypes: stringListFromAnyPolicy(input.CapabilityPolicy, input.RuntimeScopePolicy, "allowed_employee_types"),
		AllowedProviderTypes: stringListFromPolicy(input.CapabilityPolicy, "allowed_provider_types"),
		AllowedSkills:        stringListFromPolicy(input.CapabilityPolicy, "allowed_skills"),
		AllowedMCPServers:    stringListFromPolicy(input.CapabilityPolicy, "allowed_mcp_servers"),
		AllowedExternalCaps:  stringListFromPolicy(input.CapabilityPolicy, "allowed_external_capabilities"),
		CapabilityPolicy:     cloneMap(input.CapabilityPolicy),
		ContextPolicy:        cloneMap(input.ContextPolicy),
		ApprovalPolicy:       cloneMap(input.ApprovalPolicy),
		RuntimeScopePolicy:   cloneMap(input.RuntimeScopePolicy),
	}
}

func capabilityOptionsFromTeamConfig(input TeamConfigInput) CapabilityOptions {
	return CapabilityOptions{
		Skills:               stringListFromPolicy(input.CapabilityPolicy, "allowed_skills"),
		MCPServers:           stringListFromPolicy(input.CapabilityPolicy, "allowed_mcp_servers"),
		ExternalCapabilities: stringListFromPolicy(input.CapabilityPolicy, "allowed_external_capabilities"),
		ProviderTypes:        stringListFromPolicy(input.CapabilityPolicy, "allowed_provider_types"),
	}
}

func employeeTypesForTeamConfig(input TeamConfigInput) []EmployeeTypeDefinition {
	allowed := stringListFromAnyPolicy(input.CapabilityPolicy, input.RuntimeScopePolicy, "allowed_employee_types")
	if len(allowed) == 0 {
		return DefaultEmployeeTypeDefinitions()
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		allowedSet[value] = struct{}{}
	}
	out := []EmployeeTypeDefinition{}
	for _, definition := range DefaultEmployeeTypeDefinitions() {
		if _, ok := allowedSet[definition.Type]; ok {
			out = append(out, definition)
		}
	}
	return out
}

func stringListFromAnyPolicy(primary, secondary map[string]any, key string) []string {
	values := stringListFromPolicy(primary, key)
	if len(values) > 0 {
		return values
	}
	return stringListFromPolicy(secondary, key)
}

func stringListFromPolicy(values map[string]any, key string) []string {
	items, _ := stringListPolicyValue(values, key, key)
	return stringList(items)
}
```

- [ ] **Step 8: 运行服务测试**

Run: `go test ./apps/control-plane/internal/employee -run 'TestEmployeeTypeRegistryExcludesProjectCoordinator|TestGetCreateOptionsReturnsTeamPolicyAndRuntimeCandidates' -count=1`

Expected: PASS。

- [ ] **Step 9: 提交候选服务**

```bash
git add \
  apps/control-plane/internal/employee/types.go \
  apps/control-plane/internal/employee/employee_types.go \
  apps/control-plane/internal/employee/repository.go \
  apps/control-plane/internal/employee/pg_repository.go \
  apps/control-plane/internal/employee/service.go \
  apps/control-plane/internal/employee/service_test.go
git commit -m "feat: add digital employee create options"
```

## Task 3: 创建编排服务升级为 ready 语义

**Files:**
- Modify: `apps/control-plane/internal/employee/types.go`
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/control-plane/internal/employee/repository.go`
- Modify: `apps/control-plane/internal/employee/pg_repository.go`
- Modify: `apps/control-plane/internal/employee/service_test.go`
- Modify: `apps/control-plane/internal/app/app.go`

- [ ] **Step 1: 写创建闭环服务测试**

在 `apps/control-plane/internal/employee/service_test.go` 新增：

```go
func TestCreateDigitalEmployeeCreatesOwnerTypeConfigEffectiveConfigAndProvisioning(t *testing.T) {
	repo := newMemoryRepository()
	dispatcher := newFakeRuntimeCommandDispatcher()
	svc, err := NewServiceWithProvisioning(repo, dispatcher)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	ownerUserID := uuid.New()
	runtimeNodeID := uuid.New()
	teamConfigID := uuid.New()
	repo.teams[teamID] = tenantID
	repo.currentTeamConfigByTeam[teamID] = teamConfigID
	repo.teamConfigs[teamConfigID] = TeamConfigInput{
		ID:             teamConfigID,
		TenantID:       tenantID,
		TeamID:         teamID,
		RevisionNumber: 5,
		Status:         TeamConfigRevisionStatusActive,
		CapabilityPolicy: map[string]any{
			"allowed_skills":         []any{"database-troubleshooting"},
			"allowed_mcp_servers":    []any{"postgres-readonly"},
			"allowed_provider_types": []any{"codex"},
		},
		ContextPolicy: map[string]any{"sources": []any{"monitoring"}},
		ApprovalPolicy: map[string]any{
			"min_risk_for_human":          "medium",
			"write_actions_require_human": true,
		},
	}
	repo.preflight = RuntimeProvisioningPreflight{
		TenantID:              tenantID,
		TeamID:                teamID,
		RuntimeNodeID:         runtimeNodeID,
		NodeID:                "runtime-node-1",
		AgentHomeDir:          "/runtime/agents",
		GovernanceSnapshot:    map[string]any{"team_config_revision_id": teamConfigID.String()},
		HasActiveTeamConfig:   true,
		RuntimeOnline:         true,
		EnrollmentApproved:    true,
		RuntimeSessionActive:  true,
		ProviderAvailable:     true,
		ProviderPolicyAllowed: true,
		RuntimePolicyAllowed:  true,
	}
	repo.waitStatus = string(DigitalEmployeeRunStatusCompleted)
	dispatcher.connected["runtime-node-1"] = true

	created, err := svc.CreateDigitalEmployee(context.Background(), CreateDigitalEmployeeRequest{
		TenantID:      tenantID,
		TeamID:        &teamID,
		OwnerUserID:   ownerUserID,
		EmployeeType:  "database_admin",
		Name:          "数据库运维员工",
		Role:          "database_admin",
		RuntimeNodeID: runtimeNodeID,
		ProviderType:  "codex",
		CapabilitySelection: map[string]any{
			"enabled_skills":         []any{"database-troubleshooting"},
			"enabled_mcp_servers":    []any{"postgres-readonly"},
			"enabled_provider_types": []any{"codex"},
		},
		ContextPolicyOverride: map[string]any{"sources": []any{"monitoring"}},
		ApprovalPolicyOverride: map[string]any{
			"min_risk_for_human":          "medium",
			"write_actions_require_human": true,
		},
	})
	if err != nil {
		t.Fatalf("create digital employee: %v", err)
	}

	if created.Status != DigitalEmployeeStatusReady {
		t.Fatalf("expected ready employee, got %s", created.Status)
	}
	if created.OwnerUserID != ownerUserID || created.EmployeeType != "database_admin" {
		t.Fatalf("unexpected owner/type: %#v", created)
	}
	if repo.createdConfigRevision.Status != ConfigRevisionStatusActive {
		t.Fatalf("expected active initial config revision, got %#v", repo.createdConfigRevision)
	}
	if repo.createdConfigRevision.ApprovedBy == nil || *repo.createdConfigRevision.ApprovedBy != ownerUserID {
		t.Fatalf("expected owner as config approver, got %#v", repo.createdConfigRevision.ApprovedBy)
	}
	if repo.createdEffectiveConfig.Status != EffectiveConfigStatusApproved {
		t.Fatalf("expected approved effective config, got %#v", repo.createdEffectiveConfig)
	}
	if len(dispatcher.commands) != 1 || dispatcher.commands[0].Type != "provision_instance" {
		t.Fatalf("expected one provisioning command, got %#v", dispatcher.commands)
	}
	if repo.transactionCount != 1 {
		t.Fatalf("expected one local transaction, got %d", repo.transactionCount)
	}
	if dispatcher.dispatchedDuringTransaction {
		t.Fatalf("runtime command must be dispatched after local transaction commits")
	}
}
```

- [ ] **Step 2: 写无效专业类型和能力越界测试**

在 `apps/control-plane/internal/employee/service_test.go` 新增：

```go
func TestCreateDigitalEmployeeRejectsUnknownEmployeeType(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithProvisioning(repo, newFakeRuntimeCommandDispatcher())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	repo.teams[teamID] = tenantID

	_, err = svc.CreateDigitalEmployee(context.Background(), CreateDigitalEmployeeRequest{
		TenantID:     tenantID,
		TeamID:       &teamID,
		OwnerUserID:  uuid.New(),
		EmployeeType: "project_coordinator",
		Name:         "项目协调员",
		Role:         "project_coordinator",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for coordinator employee type, got %v", err)
	}
	if repo.createdEmployeeCount != 0 {
		t.Fatalf("expected no employee creation, got %d", repo.createdEmployeeCount)
	}
}

func TestCreateDigitalEmployeeBlocksCapabilityOutsideTeamPolicyBeforeProvisioning(t *testing.T) {
	repo := newMemoryRepository()
	dispatcher := newFakeRuntimeCommandDispatcher()
	svc, err := NewServiceWithProvisioning(repo, dispatcher)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	teamConfigID := uuid.New()
	repo.teams[teamID] = tenantID
	repo.currentTeamConfigByTeam[teamID] = teamConfigID
	repo.teamConfigs[teamConfigID] = TeamConfigInput{
		ID:               teamConfigID,
		TenantID:         tenantID,
		TeamID:           teamID,
		Status:           TeamConfigRevisionStatusActive,
		CapabilityPolicy: map[string]any{"allowed_skills": []any{"database-troubleshooting"}},
	}

	_, err = svc.CreateDigitalEmployee(context.Background(), CreateDigitalEmployeeRequest{
		TenantID:      tenantID,
		TeamID:        &teamID,
		OwnerUserID:   uuid.New(),
		EmployeeType:  "database_admin",
		Name:          "数据库运维员工",
		Role:          "database_admin",
		RuntimeNodeID: uuid.New(),
		ProviderType:  "codex",
		CapabilitySelection: map[string]any{
			"enabled_skills": []any{"deployment-write"},
		},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for capability outside allowlist, got %v", err)
	}
	if len(dispatcher.commands) != 0 {
		t.Fatalf("expected no runtime command before validation passes, got %#v", dispatcher.commands)
	}
}

func TestCreateDigitalEmployeeRollsBackLocalFactsWhenEffectiveConfigFails(t *testing.T) {
	repo := newMemoryRepository()
	repo.failCreateEffectiveConfig = errors.New("effective config insert failed")
	svc, err := NewServiceWithProvisioning(repo, newFakeRuntimeCommandDispatcher())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.CreateDigitalEmployee(context.Background(), validCreateDigitalEmployeeRequest(repo))
	if !errors.Is(err, repo.failCreateEffectiveConfig) {
		t.Fatalf("expected effective config failure, got %v", err)
	}
	if repo.createdEmployeeCount != 0 || len(repo.executionInstances) != 0 || len(repo.runtimeReceipts) != 0 {
		t.Fatalf("expected local transaction rollback, got employees=%d instances=%d receipts=%d", repo.createdEmployeeCount, len(repo.executionInstances), len(repo.runtimeReceipts))
	}
}

func TestCreateDigitalEmployeeProvisioningTimeoutCleansUpCreationFacts(t *testing.T) {
	repo := newMemoryRepository()
	dispatcher := newFakeRuntimeCommandDispatcher()
	svc, err := NewServiceWithProvisioning(repo, dispatcher)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.provisioningTimeout = time.Millisecond
	repo.waitForProvisioning = func(ctx context.Context, tenantID, employeeID, instanceID uuid.UUID) error {
		<-ctx.Done()
		return ctx.Err()
	}

	_, err = svc.CreateDigitalEmployee(context.Background(), validCreateDigitalEmployeeRequest(repo))
	if !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("expected runtime unavailable timeout, got %v", err)
	}
	if len(repo.abortReasons) != 1 {
		t.Fatalf("expected compensation abort, got %#v", repo.abortReasons)
	}
	if repo.hasApprovedEffectiveConfig() || repo.hasVisibleProvisioningEmployee() {
		t.Fatalf("expected compensation to revoke effective config and hide employee")
	}
}
```

- [ ] **Step 3: 运行创建服务测试并确认失败**

Run: `go test ./apps/control-plane/internal/employee -run 'TestCreateDigitalEmployeeCreatesOwnerTypeConfigEffectiveConfigAndProvisioning|TestCreateDigitalEmployeeRejectsUnknownEmployeeType|TestCreateDigitalEmployeeBlocksCapabilityOutsideTeamPolicyBeforeProvisioning|TestCreateDigitalEmployeeRollsBackLocalFactsWhenEffectiveConfigFails|TestCreateDigitalEmployeeProvisioningTimeoutCleansUpCreationFacts' -count=1`

Expected: FAIL，错误包含 `CreateDigitalEmployee` 或 `WithTransaction` 未定义。

- [ ] **Step 4: 定义完整创建请求**

在 `apps/control-plane/internal/employee/types.go` 增加 `CreateDigitalEmployeeRequest`，保留旧 `CreateDraftRequest` 到实现切换完成后再删除：

```go
type CreateDigitalEmployeeRequest struct {
	TenantID               uuid.UUID
	TeamID                 *uuid.UUID
	OwnerUserID            uuid.UUID
	EmployeeType           string
	Name                   string
	Role                   string
	Description            *string
	PermissionPolicy       map[string]any
	ContextPolicy          map[string]any
	ApprovalPolicy         map[string]any
	RiskLevel              string
	Metadata               map[string]any
	RoleProfile            map[string]any
	ConstitutionAddendum   map[string]any
	CapabilitySelection    map[string]any
	ContextPolicyOverride  map[string]any
	ApprovalPolicyOverride map[string]any
	OutputContractAddendum map[string]any
	RuntimeNodeID          uuid.UUID
	ProviderType           string
	SessionPolicy          map[string]any
	WorkspacePolicy        map[string]any
}
```

在 `apps/control-plane/internal/employee/service.go` 增加有界等待配置，`NewService` 和 `NewServiceWithProvisioning` 均设置默认值：

```go
const defaultProvisioningTimeout = 10 * time.Second

type Service struct {
	repository          Repository
	dispatcher          RuntimeCommandDispatcher
	provisioningTimeout time.Duration
}
```

- [ ] **Step 5: 增加事务边界并实现创建编排入口**

在 `apps/control-plane/internal/employee/service.go` 将现有 `CreateDraft` 逻辑迁移到新方法 `CreateDigitalEmployee`。同步把本文件里的既有 `TestCreateDraft...` 测试改名为 `TestCreateDigitalEmployee...`，所有调用都改成 `CreateDigitalEmployee`，并显式传入 `OwnerUserID` 和 `EmployeeType`。

先在 `apps/control-plane/internal/employee/repository.go` 增加事务接口：

```go
type Repository interface {
	WithTransaction(ctx context.Context, fn func(Repository) error) error
	// existing methods...
}
```

在 `apps/control-plane/internal/employee/pg_repository.go` 让 PG repository 持有 transaction beginner：

```go
type txBeginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

type PgRepository struct {
	q  *queries.Queries
	db txBeginner
}

func NewPgRepository(q *queries.Queries, db txBeginner) Repository {
	return &PgRepository{q: q, db: db}
}

func (r *PgRepository) WithTransaction(ctx context.Context, fn func(Repository) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	txRepo := &PgRepository{q: r.q.WithTx(tx), db: r.db}
	if err := fn(txRepo); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}
```

在 `apps/control-plane/internal/app/app.go` 修改构造：

```go
employeeRepository := employee.NewPgRepository(q, stores.Postgres)
```

为了 Task 3 到 Task 4 之间不让 `*Service` 失去旧 handler interface 需要的方法，临时保留一个退役 wrapper。Task 4 改造 handler interface 后删除这个 wrapper：

```go
func (s *Service) CreateDraft(ctx context.Context, req CreateDraftRequest) (*DigitalEmployee, error) {
	return nil, fmt.Errorf("%w: CreateDraft is retired; use CreateDigitalEmployee", ErrInvalidInput)
}
```

新增 `CreateDigitalEmployee` 关键流程：

```go
func (s *Service) CreateDigitalEmployee(ctx context.Context, req CreateDigitalEmployeeRequest) (*DigitalEmployee, error) {
	normalized, definition, err := normalizeCreateDigitalEmployeeRequest(req)
	if err != nil {
		return nil, err
	}
	if err := s.repository.EnsureTeamExists(ctx, normalized.TenantID, *normalized.TeamID); err != nil {
		return nil, fmt.Errorf("get team: %w", err)
	}
	teamConfig, err := s.repository.GetCurrentTeamConfigRevision(ctx, normalized.TenantID, *normalized.TeamID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("%w: active team governance config is required before creating digital employee", ErrEffectiveConfigRequired)
		}
		return nil, fmt.Errorf("get current team config revision: %w", err)
	}
	preflight, err := s.repository.GetRuntimeProvisioningPreflight(ctx, normalized.TenantID, *normalized.TeamID, normalized.RuntimeNodeID, normalized.ProviderType)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("%w: runtime provisioning preflight unavailable", ErrRuntimeUnavailable)
		}
		return nil, fmt.Errorf("get runtime provisioning preflight: %w", err)
	}
	if err := validateRuntimeProvisioningPreflight(preflight); err != nil {
		return nil, err
	}
	if s.dispatcher == nil {
		return nil, fmt.Errorf("%w: runtime command dispatcher is required", ErrRuntimeUnavailable)
	}
	if !s.dispatcher.IsConnected(preflight.NodeID) {
		return nil, fmt.Errorf("%w: runtime node is not connected", ErrRuntimeUnavailable)
	}

	var record DigitalEmployeeRecord
	var instance DigitalEmployeeExecutionInstanceRecord
	var command RuntimeCommand
	err = s.repository.WithTransaction(ctx, func(repo Repository) error {
		record, err = repo.CreateDigitalEmployee(ctx, createDigitalEmployeeParams(normalized))
		if err != nil {
			return fmt.Errorf("create digital employee: %w", err)
		}
		employeeConfig := initialEmployeeConfigInput(normalized, definition)
		preview, err := s.previewEffectiveConfigWithRepository(ctx, repo, PreviewEffectiveConfigRequest{
			TenantID:          normalized.TenantID,
			DigitalEmployeeID: record.ID,
			TeamConfig:        teamConfig,
			EmployeeConfig:    employeeConfig,
		})
		if err != nil {
			return err
		}
		if len(preview.Validation.BlockingErrors) > 0 {
			return fmt.Errorf("%w: effective config has blocking validation errors", ErrInvalidInput)
		}
		configRevision, err := s.createInitialActiveConfigRevision(ctx, repo, normalized, record.ID, employeeConfig)
		if err != nil {
			return fmt.Errorf("create initial config revision: %w", err)
		}
		preview.EmployeeConfigRevisionID = configRevision.ID
		preview.EffectiveConfig["employee_config_revision_id"] = configRevision.ID.String()
		if _, err := s.createApprovedEffectiveConfig(ctx, repo, normalized, record.ID, teamConfig.ID, configRevision.ID, preview); err != nil {
			return fmt.Errorf("create approved effective config: %w", err)
		}
		instance, command, err = s.createProvisioningInstanceAndReceipt(ctx, repo, normalized, record, preflight, preview)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := s.dispatchRuntimeProvisioningCommand(ctx, preflight.NodeID, command); err != nil {
		abortErr := s.abortProvisioning(normalized.TenantID, record.ID, instance.ID, "dispatch provisioning command failed: "+err.Error())
		return nil, provisioningErrorWithAbort(fmt.Errorf("%w: dispatch provisioning command failed", ErrRuntimeUnavailable), abortErr)
	}
	if err := s.waitForProvisioningCompletion(ctx, normalized, record.ID, instance.ID); err != nil {
		return nil, err
	}
	readyRecord, err := s.repository.GetDigitalEmployee(ctx, normalized.TenantID, record.ID)
	if err != nil {
		return nil, fmt.Errorf("get provisioned digital employee: %w", err)
	}
	return employeeFromRecord(readyRecord), nil
}
```

事务中只写 Control Plane 本地事实和 runtime receipt；事务提交后才 dispatch Runtime 命令，等待 Runtime completion 期间不持有 DB transaction。

- [ ] **Step 6: 增加创建编排 helpers**

在 `apps/control-plane/internal/employee/service.go` 增加：

```go
func normalizeCreateDigitalEmployeeRequest(req CreateDigitalEmployeeRequest) (CreateDigitalEmployeeRequest, EmployeeTypeDefinition, error) {
	if req.TenantID == uuid.Nil {
		return req, EmployeeTypeDefinition{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == nil || *req.TeamID == uuid.Nil {
		return req, EmployeeTypeDefinition{}, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if req.OwnerUserID == uuid.Nil {
		return req, EmployeeTypeDefinition{}, fmt.Errorf("%w: owner_user_id is required", ErrInvalidInput)
	}
	employeeType := strings.TrimSpace(req.EmployeeType)
	if employeeType == "" {
		return req, EmployeeTypeDefinition{}, fmt.Errorf("%w: employee_type is required", ErrInvalidInput)
	}
	definition, ok := EmployeeTypeDefinitionByType(employeeType)
	if !ok {
		return req, EmployeeTypeDefinition{}, fmt.Errorf("%w: unknown employee_type", ErrInvalidInput)
	}
	req.EmployeeType = employeeType
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return req, EmployeeTypeDefinition{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	req.Role = strings.TrimSpace(req.Role)
	if req.Role == "" {
		req.Role = definition.DefaultRole
	}
	req.Description = trimOptionalString(req.Description)
	req.RiskLevel = strings.TrimSpace(req.RiskLevel)
	if req.RiskLevel == "" {
		req.RiskLevel = definition.DefaultRiskLevel
	}
	if req.RuntimeNodeID == uuid.Nil {
		return req, EmployeeTypeDefinition{}, fmt.Errorf("%w: runtime_node_id is required", ErrInvalidInput)
	}
	req.ProviderType = strings.TrimSpace(req.ProviderType)
	if req.ProviderType == "" {
		return req, EmployeeTypeDefinition{}, fmt.Errorf("%w: provider_type is required", ErrInvalidInput)
	}
	return req, definition, nil
}

func initialEmployeeConfigInput(req CreateDigitalEmployeeRequest, definition EmployeeTypeDefinition) EmployeeConfigInput {
	roleProfile := cloneMap(definition.DefaultRoleProfile)
	for key, value := range cloneMap(req.RoleProfile) {
		roleProfile[key] = value
	}
	roleProfile["employee_type"] = req.EmployeeType
	roleProfile["role"] = req.Role
	return EmployeeConfigInput{
		ID:                     uuid.New(),
		TenantID:               req.TenantID,
		RevisionNumber:         1,
		RoleProfile:            roleProfile,
		ConstitutionAddendum:   cloneMap(req.ConstitutionAddendum),
		CapabilitySelection:    cloneMap(req.CapabilitySelection),
		ContextPolicyOverride:  mergePolicyMaps(definition.DefaultContextPolicyOverride, req.ContextPolicyOverride),
		ApprovalPolicyOverride: mergePolicyMaps(definition.DefaultApprovalPolicyOverride, req.ApprovalPolicyOverride),
		OutputContractAddendum: mergePolicyMaps(definition.DefaultOutputContractAddendum, req.OutputContractAddendum),
	}
}

func mergePolicyMaps(base, override map[string]any) map[string]any {
	merged := cloneMap(base)
	for key, value := range cloneMap(override) {
		merged[key] = value
	}
	return merged
}

func createDigitalEmployeeParams(req CreateDigitalEmployeeRequest) CreateDigitalEmployeeParams {
	return CreateDigitalEmployeeParams{
		TenantID:         req.TenantID,
		TeamID:           validUUIDPtr(req.TeamID),
		OwnerUserID:      req.OwnerUserID,
		EmployeeType:     req.EmployeeType,
		Name:             req.Name,
		Role:             req.Role,
		Description:      req.Description,
		Status:           DigitalEmployeeStatusDraft,
		PermissionPolicy: cloneMap(req.PermissionPolicy),
		ContextPolicy:    cloneMap(req.ContextPolicy),
		ApprovalPolicy:   cloneMap(req.ApprovalPolicy),
		RiskLevel:        req.RiskLevel,
		Metadata:         cloneMap(req.Metadata),
	}
}
```

实现 `previewEffectiveConfigWithRepository`、`createInitialActiveConfigRevision`、`createApprovedEffectiveConfig` 和 `createProvisioningInstanceAndReceipt` 都显式接收事务内 repository，避免事务中回到外层 `s.repository`：

```go
func (s *Service) createInitialActiveConfigRevision(ctx context.Context, repo Repository, req CreateDigitalEmployeeRequest, employeeID uuid.UUID, input EmployeeConfigInput) (DigitalEmployeeConfigRevisionRecord, error) {
	now := time.Now().UTC()
	approvedBy := req.OwnerUserID
	return repo.CreateDigitalEmployeeConfigRevision(ctx, CreateConfigRevisionParams{
		TenantID:               req.TenantID,
		DigitalEmployeeID:      employeeID,
		RevisionNumber:         1,
		RoleProfile:            cloneMap(input.RoleProfile),
		ConstitutionAddendum:   cloneMap(input.ConstitutionAddendum),
		CapabilitySelection:    cloneMap(input.CapabilitySelection),
		ContextPolicyOverride:  cloneMap(input.ContextPolicyOverride),
		ApprovalPolicyOverride: cloneMap(input.ApprovalPolicyOverride),
		OutputContractAddendum: cloneMap(input.OutputContractAddendum),
		Status:                 ConfigRevisionStatusActive,
		ApprovedBy:             &approvedBy,
		ApprovedAt:             &now,
	})
}

func (s *Service) createApprovedEffectiveConfig(ctx context.Context, repo Repository, req CreateDigitalEmployeeRequest, employeeID, teamConfigID, employeeConfigID uuid.UUID, preview *EffectiveConfigPreview) (*DigitalEmployeeEffectiveConfig, error) {
	now := time.Now().UTC()
	approvedBy := req.OwnerUserID
	record, err := repo.CreateDigitalEmployeeEffectiveConfig(ctx, CreateEffectiveConfigParams{
		TenantID:                 req.TenantID,
		DigitalEmployeeID:        employeeID,
		TeamConfigRevisionID:     teamConfigID,
		EmployeeConfigRevisionID: employeeConfigID,
		EffectiveConfig:          cloneMap(preview.EffectiveConfig),
		ValidationResult:         validationResultMap(preview.Validation),
		Status:                   EffectiveConfigStatusApproved,
		ApprovedBy:               &approvedBy,
		ApprovedAt:               &now,
	})
	if err != nil {
		return nil, err
	}
	return effectiveConfigFromRecord(record), nil
}
```

- [ ] **Step 7: 在 provisioning payload 中加入配置引用**

修改 `buildProvisionInstancePayload` 参数为 `CreateDigitalEmployeeRequest`，并在 payload 中加入：

```go
"employee_type":               employee.EmployeeType,
"owner_user_id":               employee.OwnerUserID.String(),
"role_profile":                cloneMap(req.RoleProfile),
"capability_selection":        cloneMap(req.CapabilitySelection),
"context_policy_override":     cloneMap(req.ContextPolicyOverride),
"approval_policy_override":    cloneMap(req.ApprovalPolicyOverride),
"output_contract_addendum":    cloneMap(req.OutputContractAddendum),
"team_config_revision_id":     teamConfig.ID.String(),
"employee_config_revision_id": preview.EmployeeConfigRevisionID.String(),
```

保留 `redactRuntimeEventPayload` 包装，确保 secret、token、authorization 仍脱敏。

- [ ] **Step 8: 增加有界等待和补偿清理**

修改 `waitForProvisioningCompletion`，使用 `s.provisioningTimeout` 建立子 context：

```go
func (s *Service) waitForProvisioningCompletion(ctx context.Context, req CreateDigitalEmployeeRequest, employeeID, instanceID uuid.UUID) error {
	waitCtx, cancel := context.WithTimeout(ctx, s.provisioningTimeout)
	defer cancel()

	err := s.repository.WaitForDigitalEmployeeExecutionInstanceCompletion(waitCtx, req.TenantID, employeeID, instanceID)
	if err == nil {
		return nil
	}
	reason := "runtime provisioning did not complete"
	if errors.Is(err, context.DeadlineExceeded) {
		reason = "runtime provisioning timed out after " + s.provisioningTimeout.String()
		err = fmt.Errorf("%w: %s", ErrRuntimeUnavailable, reason)
	}
	abortErr := s.abortProvisioning(req.TenantID, employeeID, instanceID, reason)
	return provisioningErrorWithAbort(err, abortErr)
}
```

`abortProvisioning` 必须调用扩展后的 `AbortProvisionedDigitalEmployee`，清理 employee、execution instance、runtime receipt、config revision 和 effective config。HTTP 层将 `ErrRuntimeUnavailable` 映射为 `503`。

- [ ] **Step 9: 更新内存测试仓库**

在 `apps/control-plane/internal/employee/service_test.go` 的 `memoryRepository` 增加字段：

```go
currentTeamConfigByTeam map[uuid.UUID]uuid.UUID
runtimeProviderOptions  []RuntimeProviderOption
createdConfigRevision   CreateConfigRevisionParams
createdEffectiveConfig  CreateEffectiveConfigParams
transactionCount        int
transactionOpen         bool
failCreateEffectiveConfig error
abortReasons            []string
runtimeReceipts         map[uuid.UUID]RuntimeCommandReceiptRecord
```

在 `newMemoryRepository()` 初始化：

```go
currentTeamConfigByTeam: map[uuid.UUID]uuid.UUID{},
```

实现新接口：

```go
func (r *memoryRepository) GetCurrentTeamConfigRevision(ctx context.Context, tenantID, teamID uuid.UUID) (TeamConfigInput, error) {
	revisionID, ok := r.currentTeamConfigByTeam[teamID]
	if !ok {
		return TeamConfigInput{}, ErrNotFound
	}
	revision, ok := r.teamConfigs[revisionID]
	if !ok || revision.TenantID != tenantID || revision.TeamID != teamID {
		return TeamConfigInput{}, ErrNotFound
	}
	return revision, nil
}

func (r *memoryRepository) ListRuntimeProviderOptionsForCreate(ctx context.Context, tenantID, teamID uuid.UUID) ([]RuntimeProviderOption, error) {
	out := make([]RuntimeProviderOption, 0, len(r.runtimeProviderOptions))
	for _, option := range r.runtimeProviderOptions {
		out = append(out, option)
	}
	return out, nil
}
```

同时实现 `WithTransaction`、`WaitForDigitalEmployeeExecutionInstanceCompletion` 和扩展后的 `AbortProvisionedDigitalEmployee` 行为。`WithTransaction` 失败时恢复快照；`AbortProvisionedDigitalEmployee` 应撤销 approved effective config 并归档 config revision。

- [ ] **Step 10: 运行员工服务全量测试**

Run: `go test ./apps/control-plane/internal/employee -count=1`

Expected: PASS。

- [ ] **Step 11: 保留后端创建编排检查点**

Task 3 不单独提交，因为 handler interface 仍在 Task 4 改造。先确认 service 层 diff 明确：

Run: `git diff --stat -- apps/control-plane/internal/employee apps/control-plane/internal/app/app.go`

Expected: 输出只包含 employee 包和 `app.go` 的 repository 构造改动。

## Task 4: HTTP 路由、OpenAPI 和生成代码

**Files:**
- Modify: `apps/control-plane/internal/employee/handler.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/api/employee_routes_test.go`
- Modify: `contracts/control-plane/openapi.yaml`
- Generated: `apps/control-plane/internal/api/gen/control_plane.gen.go`
- Generated: `apps/control-plane/gen/control_plane.gen.go`

- [ ] **Step 1: 写路由测试**

在 `apps/control-plane/internal/api/employee_routes_test.go` 的 `TestDigitalEmployeeRoutesUseConsoleTenant` 增加 create-options 请求，并更新 create 请求包含伪造 Owner：

```go
optionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/create-options?team_id="+teamID.String(), nil)
optionsReq.AddCookie(cookie)
optionsResp := httptest.NewRecorder()
server.ServeHTTP(optionsResp, optionsReq)
if optionsResp.Code != http.StatusOK {
	t.Fatalf("expected create options to succeed, got %d: %s", optionsResp.Code, optionsResp.Body.String())
}
if service.createOptionsReq.TenantID != expectedTenantID || service.createOptionsReq.TeamID != teamID {
	t.Fatalf("unexpected create options request: %#v", service.createOptionsReq)
}

spoofedOwnerID := uuid.New()
createReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees", strings.NewReader(`{
	"team_id":"`+teamID.String()+`",
	"owner_user_id":"`+spoofedOwnerID.String()+`",
	"employee_type":"database_admin",
	"name":"Requirements analyst",
	"role":"database_admin",
	"runtime_node_id":"`+runtimeNodeID.String()+`",
	"provider_type":"codex",
	"capability_selection":{"enabled_skills":["database-troubleshooting"]},
	"session_policy":{"mode":"reuse_latest"},
	"workspace_policy":{"labels":{"tier":"standard"}}
}`))
```

断言服务收到登录用户 ID：

```go
if service.createReq.OwnerUserID != user.ID {
	t.Fatalf("expected owner_user_id from console user %s, got %s", user.ID, service.createReq.OwnerUserID)
}
if service.createReq.EmployeeType != "database_admin" {
	t.Fatalf("expected employee type database_admin, got %q", service.createReq.EmployeeType)
}
```

- [ ] **Step 2: 运行路由测试并确认失败**

Run: `go test ./apps/control-plane/internal/api -run TestDigitalEmployeeRoutesUseConsoleTenant -count=1`

Expected: FAIL，错误包含 create-options 404 或接口缺失。

- [ ] **Step 3: 扩展 handler service 接口**

在 `apps/control-plane/internal/employee/handler.go` 修改 `HandlerService`：

```go
type HandlerService interface {
	GetCreateOptions(ctx context.Context, req CreateOptionsRequest) (*CreateOptions, error)
	CreateDigitalEmployee(ctx context.Context, req CreateDigitalEmployeeRequest) (*DigitalEmployee, error)
	ListDigitalEmployees(ctx context.Context, req ListDigitalEmployeesRequest) ([]*DigitalEmployee, error)
	GetDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployee, error)
	UpdateStatus(ctx context.Context, req UpdateStatusRequest) (*DigitalEmployee, error)
	GetExecutionInstance(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployeeExecutionInstance, error)
	BindExecutionInstance(ctx context.Context, req BindExecutionInstanceRequest) (*DigitalEmployeeExecutionInstance, error)
	CreateConfigRevision(ctx context.Context, req CreateDigitalEmployeeConfigRevisionRequest) (*DigitalEmployeeConfigRevision, error)
	PreviewEffectiveConfigByRevisionIDs(ctx context.Context, req PreviewEffectiveConfigByRevisionIDsRequest) (*EffectiveConfigPreview, error)
	ApproveEffectiveConfig(ctx context.Context, req ApproveEffectiveConfigRequest) (*DigitalEmployeeEffectiveConfig, error)
}
```

删除 Task 3 中临时保留的 `CreateDraft` wrapper，并删除 `CreateDraftRequest` 类型。

- [ ] **Step 4: 实现 create-options handler**

在 `apps/control-plane/internal/employee/handler.go` 增加：

```go
func (h *HTTPHandler) GetCreateOptions(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeCreate, nil, "digital employee create options")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	rawTeamID := r.URL.Query().Get("team_id")
	teamID, err := uuid.Parse(rawTeamID)
	if err != nil || teamID == uuid.Nil {
		http.Error(w, "invalid team_id", http.StatusBadRequest)
		return
	}
	options, err := service.GetCreateOptions(r.Context(), CreateOptionsRequest{
		TenantID: tenantID,
		TeamID:   teamID,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, createOptionsResponseFromDomain(options))
}
```

新增 response mapping structs，字段与 `CreateOptions` JSON 对齐。

确认 `writeHandlerError` 将 `ErrRuntimeUnavailable` 映射为 `http.StatusServiceUnavailable`，保证 provisioning timeout 返回 `503`。

- [ ] **Step 5: 更新创建 handler 的 Owner 注入**

在 `CreateDigitalEmployee` handler request struct 增加业务字段，但不包含 `OwnerUserID`：

```go
var req struct {
	TeamID                 *uuid.UUID     `json:"team_id"`
	EmployeeType           string         `json:"employee_type"`
	Name                   string         `json:"name"`
	Role                   string         `json:"role"`
	Description            *string        `json:"description"`
	PermissionPolicy       map[string]any `json:"permission_policy"`
	ContextPolicy          map[string]any `json:"context_policy"`
	ApprovalPolicy         map[string]any `json:"approval_policy"`
	RiskLevel              string         `json:"risk_level"`
	Metadata               map[string]any `json:"metadata"`
	RoleProfile            map[string]any `json:"role_profile"`
	ConstitutionAddendum   map[string]any `json:"constitution_addendum"`
	CapabilitySelection    map[string]any `json:"capability_selection"`
	ContextPolicyOverride  map[string]any `json:"context_policy_override"`
	ApprovalPolicyOverride map[string]any `json:"approval_policy_override"`
	OutputContractAddendum map[string]any `json:"output_contract_addendum"`
	RuntimeNodeID          uuid.UUID      `json:"runtime_node_id"`
	ProviderType           string         `json:"provider_type"`
	SessionPolicy          map[string]any `json:"session_policy"`
	WorkspacePolicy        map[string]any `json:"workspace_policy"`
}
```

调用服务：

```go
ownerUserID := middleware.GetUserID(r.Context())
employee, err := service.CreateDigitalEmployee(r.Context(), CreateDigitalEmployeeRequest{
	TenantID:               tenantID,
	TeamID:                 req.TeamID,
	OwnerUserID:            ownerUserID,
	EmployeeType:           req.EmployeeType,
	Name:                   req.Name,
	Role:                   req.Role,
	Description:            req.Description,
	PermissionPolicy:       req.PermissionPolicy,
	ContextPolicy:          req.ContextPolicy,
	ApprovalPolicy:         req.ApprovalPolicy,
	RiskLevel:              req.RiskLevel,
	Metadata:               req.Metadata,
	RoleProfile:            req.RoleProfile,
	ConstitutionAddendum:   req.ConstitutionAddendum,
	CapabilitySelection:    req.CapabilitySelection,
	ContextPolicyOverride:  req.ContextPolicyOverride,
	ApprovalPolicyOverride: req.ApprovalPolicyOverride,
	OutputContractAddendum: req.OutputContractAddendum,
	RuntimeNodeID:          req.RuntimeNodeID,
	ProviderType:           req.ProviderType,
	SessionPolicy:          req.SessionPolicy,
	WorkspacePolicy:        req.WorkspacePolicy,
})
```

在 `digitalEmployeeResponse` 和 `employeeResponseFromDomain` 增加：

```go
OwnerUserID  string `json:"owner_user_id"`
EmployeeType string `json:"employee_type"`
```

```go
OwnerUserID:  employee.OwnerUserID.String(),
EmployeeType: employee.EmployeeType,
```

- [ ] **Step 6: 注册路由顺序**

在 `apps/control-plane/internal/api/server.go` 中确保顺序如下：

```go
r.Get("/digital-employees", s.employeeHandler.ListDigitalEmployees)
r.Post("/digital-employees", s.employeeHandler.CreateDigitalEmployee)
r.Get("/digital-employees/create-options", s.employeeHandler.GetCreateOptions)
r.Get("/digital-employees/{employeeId}", s.employeeHandler.GetDigitalEmployee)
```

`create-options` 必须在 `/{employeeId}` 前。

- [ ] **Step 7: 扩展 OpenAPI**

在 `contracts/control-plane/openapi.yaml` 的 `/api/v1/digital-employees` 与 `/{employeeId}` 之间增加：

```yaml
  /api/v1/digital-employees/create-options:
    get:
      operationId: getDigitalEmployeeCreateOptions
      summary: Get digital employee creation options
      parameters:
        - name: team_id
          in: query
          required: true
          schema:
            type: string
            format: uuid
      responses:
        "200":
          description: Digital employee creation options
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/DigitalEmployeeCreateOptions"
        "422":
          $ref: "#/components/responses/Error"
```

扩展 `DigitalEmployee` required：

```yaml
        - owner_user_id
        - employee_type
```

扩展 `DigitalEmployee` properties：

```yaml
        owner_user_id:
          type: string
          format: uuid
        employee_type:
          type: string
```

扩展 `CreateDigitalEmployeeRequest` required：

```yaml
        - employee_type
        - runtime_node_id
        - provider_type
```

扩展 `CreateDigitalEmployeeRequest` properties：

```yaml
        employee_type:
          type: string
        role_profile:
          type: object
          additionalProperties: true
        constitution_addendum:
          type: object
          additionalProperties: true
        capability_selection:
          type: object
          additionalProperties: true
        context_policy_override:
          type: object
          additionalProperties: true
        approval_policy_override:
          type: object
          additionalProperties: true
        output_contract_addendum:
          type: object
          additionalProperties: true
```

新增 schema：

```yaml
    DigitalEmployeeCreateOptions:
      type: object
      required:
        - team_config
        - employee_types
        - capability_options
        - runtime_provider_options
        - policy_defaults
      properties:
        team_config:
          $ref: "#/components/schemas/DigitalEmployeeCreateTeamConfig"
        employee_types:
          type: array
          items:
            $ref: "#/components/schemas/DigitalEmployeeTypeOption"
        capability_options:
          $ref: "#/components/schemas/DigitalEmployeeCapabilityOptions"
        runtime_provider_options:
          type: array
          items:
            $ref: "#/components/schemas/DigitalEmployeeRuntimeProviderOption"
        policy_defaults:
          $ref: "#/components/schemas/DigitalEmployeePolicyDefaults"
```

并定义 `DigitalEmployeeCreateTeamConfig`、`DigitalEmployeeTypeOption`、`DigitalEmployeeCapabilityOptions`、`DigitalEmployeeRuntimeProviderOption`、`DigitalEmployeePolicyDefaults`，字段与后端 response struct 保持一致。

`DigitalEmployeeCreateTeamConfig` 必须包含 `allowed_employee_types`：

```yaml
        allowed_employee_types:
          type: array
          items:
            type: string
```

- [ ] **Step 8: 生成 OpenAPI 代码**

Run: `pnpm generate:control-plane`

Expected: PASS，`apps/control-plane/internal/api/gen/control_plane.gen.go` 和 `apps/control-plane/gen/control_plane.gen.go` 更新。

- [ ] **Step 9: 运行 API 测试**

Run: `go test ./apps/control-plane/internal/api -run 'TestDigitalEmployeeRoutesUseConsoleTenant|TestDigitalEmployeeRouteAuthorizationDenial' -count=1`

Expected: PASS。

- [ ] **Step 10: 提交 API 层**

```bash
git add \
  apps/control-plane/internal/employee \
  apps/control-plane/internal/employee/handler.go \
  apps/control-plane/internal/api/server.go \
  apps/control-plane/internal/api/employee_routes_test.go \
  contracts/control-plane/openapi.yaml \
  apps/control-plane/internal/api/gen/control_plane.gen.go \
  apps/control-plane/gen/control_plane.gen.go
git commit -m "feat: expose digital employee creation api"
```

## Task 5: 前端 API client

**Files:**
- Modify: `apps/web/src/lib/api/employees.ts`
- Modify: `apps/web/src/lib/api/employees.test.ts`

- [ ] **Step 1: 写 API client 测试**

在 `apps/web/src/lib/api/employees.test.ts` 新增：

```ts
it("gets digital employee create options with encoded team id", async () => {
  const options = {
    team_config: {
      id: "55555555-5555-4555-8555-555555555555",
      revision_number: 3,
      status: "active",
      allowed_employee_types: ["database_admin"],
      allowed_provider_types: ["codex"],
      allowed_skills: ["database-troubleshooting"],
      allowed_mcp_servers: ["postgres-readonly"],
      allowed_external_capabilities: [],
      capability_policy: {},
      context_policy: {},
      approval_policy: {},
      runtime_scope_policy: {},
    },
    employee_types: [{ type: "database_admin", label: "数据库管理", description: "负责数据库任务" }],
    capability_options: {
      skills: ["database-troubleshooting"],
      mcp_servers: ["postgres-readonly"],
      external_capabilities: [],
      provider_types: ["codex"],
    },
    runtime_provider_options: [
      {
        runtime_node_id: "33333333-3333-4333-8333-333333333333",
        node_id: "node-ops-01",
        runtime_name: "运维节点 01",
        provider_type: "codex",
        runtime_status: "online",
        provider_status: "healthy",
        health_status: "healthy",
        current_load: 1,
        max_slots: 4,
        agent_home_dir: "/srv/agents",
        agent_home_dir_available: true,
        available: true,
        disabled_reason: "",
      },
    ],
    policy_defaults: {
      context_policy_override: {},
      approval_policy_override: {},
      output_contract_addendum: {},
    },
  };
  const fetcher = vi.fn(async () =>
    new Response(JSON.stringify(options), {
      headers: { "content-type": "application/json" },
      status: 200,
    }),
  );

  await expect(
    getDigitalEmployeeCreateOptions(
      { baseUrl: "http://control-plane.local", fetcher },
      "team 1/ops",
    ),
  ).resolves.toEqual(options);

  expect(fetcher).toHaveBeenCalledWith(
    "http://control-plane.local/api/v1/digital-employees/create-options?team_id=team+1%2Fops",
    {
      credentials: "include",
      headers: { accept: "application/json" },
      method: "GET",
    },
  );
});

it("creates ready digital employee with structured creation body", async () => {
  const employee = {
    id: "11111111-1111-4111-8111-111111111111",
    tenant_id: "22222222-2222-4222-8222-222222222222",
    team_id: "99999999-9999-4999-8999-999999999999",
    owner_user_id: "77777777-7777-4777-8777-777777777777",
    employee_type: "database_admin",
    name: "数据库运维员工",
    role: "database_admin",
    status: "ready",
    permission_policy: {},
    context_policy: {},
    approval_policy: {},
    risk_level: "medium",
    metadata: {},
  };
  const fetcher = vi.fn(async () =>
    new Response(JSON.stringify(employee), {
      headers: { "content-type": "application/json" },
      status: 201,
    }),
  );

  await expect(
    createDigitalEmployee(
      { baseUrl: "http://control-plane.local", fetcher },
      {
        team_id: "99999999-9999-4999-8999-999999999999",
        employee_type: "database_admin",
        name: "数据库运维员工",
        role: "database_admin",
        runtime_node_id: "33333333-3333-4333-8333-333333333333",
        provider_type: "codex",
        capability_selection: {
          enabled_skills: ["database-troubleshooting"],
          enabled_mcp_servers: ["postgres-readonly"],
          enabled_provider_types: ["codex"],
        },
        context_policy_override: {},
        approval_policy_override: {},
        output_contract_addendum: {},
        session_policy: { mode: "reuse_latest" },
        workspace_policy: { labels: { tier: "standard" } },
      },
    ),
  ).resolves.toEqual(employee);

  expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/digital-employees", {
    body: JSON.stringify({
      team_id: "99999999-9999-4999-8999-999999999999",
      employee_type: "database_admin",
      name: "数据库运维员工",
      role: "database_admin",
      runtime_node_id: "33333333-3333-4333-8333-333333333333",
      provider_type: "codex",
      capability_selection: {
        enabled_skills: ["database-troubleshooting"],
        enabled_mcp_servers: ["postgres-readonly"],
        enabled_provider_types: ["codex"],
      },
      context_policy_override: {},
      approval_policy_override: {},
      output_contract_addendum: {},
      session_policy: { mode: "reuse_latest" },
      workspace_policy: { labels: { tier: "standard" } },
    }),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });
});
```

- [ ] **Step 2: 运行前端 API 测试并确认失败**

Run: `pnpm --filter @superteam/web test -- src/lib/api/employees.test.ts`

Expected: FAIL，错误包含 `getDigitalEmployeeCreateOptions` 未导出或类型字段缺失。

- [ ] **Step 3: 扩展前端类型和 API 函数**

在 `apps/web/src/lib/api/employees.ts` 增加：

```ts
export type DigitalEmployeeTypeOption = {
  type: string;
  label: string;
  description: string;
  default_role?: string;
  default_risk_level?: string;
  default_role_profile?: Record<string, unknown>;
  recommended_skill_keys?: string[];
  recommended_mcp_servers?: string[];
  recommended_external_capabilities?: string[];
  recommended_provider_types?: string[];
  default_context_policy_override?: Record<string, unknown>;
  default_approval_policy_override?: Record<string, unknown>;
  default_output_contract_addendum?: Record<string, unknown>;
};

export type DigitalEmployeeCreateOptions = {
  team_config: {
    id: string;
    revision_number: number;
    status: string;
    allowed_employee_types: string[];
    allowed_provider_types: string[];
    allowed_skills: string[];
    allowed_mcp_servers: string[];
    allowed_external_capabilities: string[];
    capability_policy: Record<string, unknown>;
    context_policy: Record<string, unknown>;
    approval_policy: Record<string, unknown>;
    runtime_scope_policy: Record<string, unknown>;
  };
  employee_types: DigitalEmployeeTypeOption[];
  capability_options: {
    skills: string[];
    mcp_servers: string[];
    external_capabilities: string[];
    provider_types: string[];
  };
  runtime_provider_options: Array<{
    runtime_node_id: string;
    node_id: string;
    runtime_name: string;
    provider_type: string;
    runtime_status: string;
    provider_status: string;
    health_status: string;
    current_load: number;
    max_slots: number;
    agent_home_dir: string;
    agent_home_dir_available: boolean;
    available: boolean;
    disabled_reason: string;
  }>;
  policy_defaults: {
    context_policy_override: Record<string, unknown>;
    approval_policy_override: Record<string, unknown>;
    output_contract_addendum: Record<string, unknown>;
  };
};
```

扩展 `DigitalEmployee`：

```ts
owner_user_id: string;
employee_type: string;
```

替换 `CreateDigitalEmployeeInput`：

```ts
export type CreateDigitalEmployeeInput = {
  team_id: string;
  employee_type: string;
  name: string;
  role: string;
  description?: string;
  risk_level?: string;
  role_profile?: Record<string, unknown>;
  constitution_addendum?: Record<string, unknown>;
  capability_selection?: Record<string, unknown>;
  context_policy_override?: Record<string, unknown>;
  approval_policy_override?: Record<string, unknown>;
  output_contract_addendum?: Record<string, unknown>;
  runtime_node_id: string;
  provider_type: string;
  session_policy?: Record<string, unknown>;
  workspace_policy?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
};
```

新增函数：

```ts
export function getDigitalEmployeeCreateOptions(
  options: ApiClientOptions,
  teamId: string,
): Promise<DigitalEmployeeCreateOptions> {
  const searchParams = new URLSearchParams({ team_id: teamId });
  return getJson<DigitalEmployeeCreateOptions>(
    options,
    `/api/v1/digital-employees/create-options?${searchParams.toString()}`,
    "digital employee create options",
  );
}
```

- [ ] **Step 4: 运行前端 API 测试**

Run: `pnpm --filter @superteam/web test -- src/lib/api/employees.test.ts`

Expected: PASS。

- [ ] **Step 5: 提交前端 API client**

```bash
git add apps/web/src/lib/api/employees.ts apps/web/src/lib/api/employees.test.ts
git commit -m "feat: add digital employee creation client"
```

## Task 6: 前端创建向导页面

**Files:**
- Create: `apps/web/src/features/employees/create.tsx`
- Create: `apps/web/src/features/employees/create.test.tsx`
- Create: `apps/web/src/routes/_authenticated/employees/new.tsx`
- Modify: `apps/web/src/features/employees/index.tsx`
- Modify: `apps/web/src/features/employees/index.test.tsx`
- Generated: `apps/web/src/routeTree.gen.ts`

- [ ] **Step 1: 写创建页测试**

创建 `apps/web/src/features/employees/create.test.tsx`：

```tsx
import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { CreateEmployeeView } from "@/features/employees/create";

vi.mock("@/components/layout/header", () => ({
  Header: ({ children }: { children: ReactNode }) => <header>{children}</header>,
}));

vi.mock("@/components/layout/main", () => ({
  Main: ({ children }: { children: ReactNode }) => <main>{children}</main>,
}));

vi.mock("@/components/search", () => ({
  Search: () => <button type="button">Search</button>,
}));

vi.mock("@/components/theme-switch", () => ({
  ThemeSwitch: () => <button type="button">Toggle theme</button>,
}));

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => <a href={to}>{children}</a>,
  useNavigate: () => vi.fn(),
}));

function createQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
}

const defaultRuntimeProviderOptions = [
  {
    runtime_node_id: "runtime-1",
    node_id: "node-ops-01",
    runtime_name: "运维节点 01",
    provider_type: "codex",
    runtime_status: "online",
    provider_status: "healthy",
    health_status: "healthy",
    current_load: 1,
    max_slots: 4,
    agent_home_dir: "/srv/agents",
    agent_home_dir_available: true,
    available: true,
    disabled_reason: "",
  },
];

function createCreationFetcher(runtimeProviderOptions = defaultRuntimeProviderOptions) {
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === "/api/v1/teams" && method === "GET") {
      return new Response(JSON.stringify([{ id: "team-1", name: "运维团队", slug: "ops", status: "active" }]), {
        headers: { "content-type": "application/json" },
        status: 200,
      });
    }

    if (url.pathname === "/api/v1/digital-employees/create-options" && method === "GET") {
      expect(url.searchParams.get("team_id")).toBe("team-1");
      return new Response(JSON.stringify({
        team_config: {
          id: "team-config-1",
          revision_number: 3,
          status: "active",
          allowed_employee_types: ["database_admin"],
          allowed_provider_types: ["codex"],
          allowed_skills: ["database-troubleshooting"],
          allowed_mcp_servers: ["postgres-readonly"],
          allowed_external_capabilities: ["ticket-read"],
          capability_policy: {},
          context_policy: {},
          approval_policy: {},
          runtime_scope_policy: {},
        },
        employee_types: [
          {
            type: "database_admin",
            label: "数据库管理",
            description: "负责数据库巡检、故障诊断、变更评估和运维证据整理。",
            default_role: "database_admin",
            default_risk_level: "medium",
            default_role_profile: { title: "数据库管理工程师" },
            recommended_skill_keys: ["database-troubleshooting"],
            recommended_mcp_servers: ["postgres-readonly"],
            recommended_external_capabilities: ["ticket-read"],
            recommended_provider_types: ["codex"],
          },
        ],
        capability_options: {
          skills: ["database-troubleshooting"],
          mcp_servers: ["postgres-readonly"],
          external_capabilities: ["ticket-read"],
          provider_types: ["codex"],
        },
        runtime_provider_options: runtimeProviderOptions,
        policy_defaults: {
          context_policy_override: {},
          approval_policy_override: {},
          output_contract_addendum: {},
        },
      }), {
        headers: { "content-type": "application/json" },
        status: 200,
      });
    }

    if (url.pathname === "/api/v1/digital-employees" && method === "POST") {
      expect(JSON.parse(String(init?.body))).toEqual({
        team_id: "team-1",
        employee_type: "database_admin",
        name: "数据库运维员工",
        role: "database_admin",
        description: "负责数据库巡检",
        risk_level: "medium",
        role_profile: { title: "数据库管理工程师" },
        capability_selection: {
          enabled_skills: ["database-troubleshooting"],
          enabled_mcp_servers: ["postgres-readonly"],
          enabled_external_capabilities: ["ticket-read"],
          enabled_provider_types: ["codex"],
        },
        context_policy_override: {},
        approval_policy_override: {},
        output_contract_addendum: {},
        runtime_node_id: "runtime-1",
        provider_type: "codex",
        session_policy: { mode: "reuse_latest" },
        workspace_policy: {},
      });
      return new Response(JSON.stringify({
        id: "employee-1",
        tenant_id: "tenant-1",
        team_id: "team-1",
        owner_user_id: "owner-1",
        employee_type: "database_admin",
        name: "数据库运维员工",
        role: "database_admin",
        status: "ready",
        permission_policy: {},
        context_policy: {},
        approval_policy: {},
        risk_level: "medium",
        metadata: {},
      }), {
        headers: { "content-type": "application/json" },
        status: 201,
      });
    }

    return new Response(JSON.stringify({ error: `unhandled ${method} ${url.pathname}` }), {
      headers: { "content-type": "application/json" },
      status: 404,
    });
  }) as unknown as typeof fetch;
  return fetcher;
}

async function renderCreateEmployeeView(fetcher = createCreationFetcher()) {
  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <CreateEmployeeView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

describe("CreateEmployeeView", () => {
  it("creates a ready digital employee through the four-step wizard", async () => {
    const screen = await renderCreateEmployeeView();

    await expect.element(screen.getByRole("heading", { name: "创建数字员工" })).toBeVisible();
    await userEvent.fill(screen.getByLabelText("名称"), "数据库运维员工");
    await userEvent.fill(screen.getByLabelText("描述"), "负责数据库巡检");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await userEvent.click(screen.getByRole("checkbox", { name: "database-troubleshooting" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "postgres-readonly" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "ticket-read" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await expect.element(screen.getByText("运维节点 01")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "创建数字员工" }));

    await expect.element(screen.getByText("创建成功，数字员工已准备好")).toBeVisible();
  });

  it("blocks next step until current identity fields are valid", async () => {
    const screen = await renderCreateEmployeeView();

    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByText("名称不能为空")).toBeVisible();
    await expect.element(screen.getByRole("heading", { name: "身份" })).toBeVisible();
  });

  it("requires explicit runtime selection when multiple runtimes are available", async () => {
    const screen = await renderCreateEmployeeView(createCreationFetcher([
      ...defaultRuntimeProviderOptions,
      { ...defaultRuntimeProviderOptions[0], runtime_node_id: "runtime-2", node_id: "node-ops-02", runtime_name: "运维节点 02" },
    ]));

    await userEvent.fill(screen.getByLabelText("名称"), "数据库运维员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByRole("button", { name: "创建数字员工" })).toBeDisabled();
    await userEvent.click(screen.getByRole("radio", { name: /运维节点 02/ }));
    await expect.element(screen.getByRole("button", { name: "创建数字员工" })).toBeEnabled();
  });
});
```

- [ ] **Step 2: 运行创建页测试并确认失败**

Run: `pnpm --filter @superteam/web test -- src/features/employees/create.test.tsx`

Expected: FAIL，错误包含 `CreateEmployeeView` 模块缺失。

- [ ] **Step 3: 创建路由文件**

创建 `apps/web/src/routes/_authenticated/employees/new.tsx`：

```tsx
import { createFileRoute } from "@tanstack/react-router";
import { CreateEmployeePage } from "@/features/employees/create";

export const Route = createFileRoute("/_authenticated/employees/new")({
  component: CreateEmployeePage,
});
```

- [ ] **Step 4: 创建向导页面骨架和表单 schema**

创建 `apps/web/src/features/employees/create.tsx`，先放入 imports、schema 和 entry components：

```tsx
import { useEffect, useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { Bot, Check, ChevronLeft, ChevronRight, Cpu, ShieldCheck, Sparkles } from "lucide-react";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import { SemanticIconTile, StatusBadge } from "@/components/superteam";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { createDigitalEmployee, getDigitalEmployeeCreateOptions, type DigitalEmployeeCreateOptions } from "@/lib/api/employees";
import { listTeams } from "@/lib/api/teams";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { cn } from "@/lib/utils";

const createEmployeeSchema = z.object({
  team_id: z.string().min(1, "请选择归属团队"),
  employee_type: z.string().min(1, "请选择专业类型"),
  name: z.string().trim().min(1, "名称不能为空"),
  description: z.string().trim().optional(),
  role: z.string().trim().min(1, "角色不能为空"),
  risk_level: z.string().min(1),
  enabled_skills: z.array(z.string()),
  enabled_mcp_servers: z.array(z.string()),
  enabled_external_capabilities: z.array(z.string()),
  runtime_binding: z.string().min(1, "请选择 Runtime 和 Provider"),
});

type CreateEmployeeFormValues = z.infer<typeof createEmployeeSchema>;

const defaultValues: CreateEmployeeFormValues = {
  team_id: "",
  employee_type: "",
  name: "",
  description: "",
  role: "",
  risk_level: "medium",
  enabled_skills: [],
  enabled_mcp_servers: [],
  enabled_external_capabilities: [],
  runtime_binding: "",
};

const stepFields: Array<Array<keyof CreateEmployeeFormValues>> = [
  ["team_id", "employee_type", "name", "role", "risk_level"],
  ["enabled_skills", "enabled_mcp_servers", "enabled_external_capabilities"],
  [],
  ["runtime_binding"],
];

export function CreateEmployeePage() {
  const apiBaseUrl = resolveControlPlaneUrl();
  return <CreateEmployeeView apiBaseUrl={apiBaseUrl} />;
}

type CreateEmployeeViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
};

export function CreateEmployeeView({ apiBaseUrl, fetcher }: CreateEmployeeViewProps) {
  const navigate = useNavigate();
  const [step, setStep] = useState(0);
  const form = useForm<CreateEmployeeFormValues>({
    defaultValues,
    resolver: zodResolver(createEmployeeSchema),
  });
  const selectedTeamId = form.watch("team_id");
  const selectedEmployeeType = form.watch("employee_type");
  const selectedRuntimeBinding = form.watch("runtime_binding");

  const teams = useQuery({
    queryKey: ["teams"],
    queryFn: () => listTeams({ baseUrl: apiBaseUrl, fetcher }),
  });
  const createOptions = useQuery({
    enabled: Boolean(selectedTeamId),
    queryKey: ["digital-employee-create-options", selectedTeamId],
    queryFn: () => getDigitalEmployeeCreateOptions({ baseUrl: apiBaseUrl, fetcher }, selectedTeamId),
  });

  useEffect(() => {
    const firstTeam = teams.data?.[0];
    if (!form.getValues("team_id") && firstTeam) {
      form.setValue("team_id", firstTeam.id, { shouldDirty: false, shouldValidate: true });
    }
  }, [form, teams.data]);

  useEffect(() => {
    const options = createOptions.data;
    if (!options) return;
    const firstType = options.employee_types[0];
    if (firstType && !form.getValues("employee_type")) {
      applyEmployeeTypeDefaults(form, firstType);
    }
    const availableRuntimes = options.runtime_provider_options.filter((option) => option.available);
    if (availableRuntimes.length === 1 && !form.getValues("runtime_binding")) {
      form.setValue("runtime_binding", runtimeBindingValue(availableRuntimes[0]), { shouldValidate: true });
    }
    if (availableRuntimes.length !== 1 && form.getValues("runtime_binding")) {
      form.setValue("runtime_binding", "", { shouldValidate: true });
    }
  }, [createOptions.data, form]);

  const goNext = async () => {
    const valid = await form.trigger(stepFields[step], { shouldFocus: true });
    if (valid) {
      setStep((value) => Math.min(3, value + 1));
    }
  };

  const submit = useMutation({
    mutationFn: (values: CreateEmployeeFormValues) => {
      const options = createOptions.data;
      if (!options) throw new Error("创建候选未加载");
      const employeeType = options.employee_types.find((item) => item.type === values.employee_type);
      const runtime = options.runtime_provider_options.find((item) => runtimeBindingValue(item) === values.runtime_binding);
      if (!employeeType) throw new Error("专业类型不可用");
      if (!runtime || !runtime.available) throw new Error("Runtime 或 Provider 不可用");
      return createDigitalEmployee({ baseUrl: apiBaseUrl, fetcher }, buildCreateInput(values, employeeType, runtime));
    },
    onSuccess: (employee) => {
      void navigate({ to: "/employees/$employeeId", params: { employeeId: employee.id } });
    },
  });

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main className="min-w-0 overflow-x-hidden">
        <CreateEmployeeShell
          createOptions={createOptions.data}
          createOptionsError={createOptions.isError}
          createOptionsLoading={createOptions.isFetching}
          form={form}
          onBack={() => setStep((value) => Math.max(0, value - 1))}
          onNext={goNext}
          onSubmit={form.handleSubmit((values) => submit.mutate(values))}
          selectedEmployeeType={selectedEmployeeType}
          selectedRuntimeBinding={selectedRuntimeBinding}
          step={step}
          submitError={submit.error instanceof Error ? submit.error.message : undefined}
          submitting={submit.isPending}
          teams={teams.data ?? []}
          teamsError={teams.isError}
          teamsLoading={teams.isLoading}
        />
      </Main>
    </>
  );
}
```

- [ ] **Step 5: 实现四步 UI 组件**

在同一文件增加四个 step 组件：

```tsx
const steps = [
  { title: "身份", icon: Bot },
  { title: "能力", icon: Sparkles },
  { title: "治理", icon: ShieldCheck },
  { title: "运行", icon: Cpu },
] as const;

function CreateEmployeeShell(props: {
  createOptions?: DigitalEmployeeCreateOptions;
  createOptionsError: boolean;
  createOptionsLoading: boolean;
  form: ReturnType<typeof useForm<CreateEmployeeFormValues>>;
  onBack: () => void;
  onNext: () => void | Promise<void>;
  onSubmit: () => void;
  selectedEmployeeType: string;
  selectedRuntimeBinding: string;
  step: number;
  submitError?: string;
  submitting: boolean;
  teams: Array<{ id: string; name: string }>;
  teamsError: boolean;
  teamsLoading: boolean;
}) {
  const currentStep = steps[props.step];
  const CurrentIcon = currentStep.icon;
  const canSubmit = props.step === 3 && Boolean(props.selectedRuntimeBinding) && !props.submitting;

  return (
    <Form {...props.form}>
      <form className="space-y-4" onSubmit={(event) => event.preventDefault()}>
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            <SemanticIconTile tone="primary" size="sm">
              <Bot />
            </SemanticIconTile>
            <div>
              <h1 className="text-2xl font-bold tracking-normal">创建数字员工</h1>
              <p className="text-sm text-muted-foreground">创建完成后状态为 ready，可被任务或调度调用。</p>
            </div>
          </div>
          <Button asChild type="button" variant="outline">
            <Link to="/employees">返回列表</Link>
          </Button>
        </div>

        <Card>
          <CardHeader>
            <div className="flex flex-wrap items-center gap-2">
              {steps.map((item, index) => {
                const Icon = item.icon;
                const active = index === props.step;
                const done = index < props.step;
                return (
                  <div className="flex items-center gap-2" key={item.title}>
                    <span className={cn("flex size-8 items-center justify-center rounded-full border", active ? "border-primary bg-primary text-primary-foreground" : "", done ? "border-primary text-primary" : "")}>
                      {done ? <Check className="size-4" /> : <Icon className="size-4" />}
                    </span>
                    <span className={cn("text-sm font-medium text-muted-foreground", active || done ? "text-foreground" : "")}>{item.title}</span>
                    {index < steps.length - 1 ? <span className="h-px w-8 bg-border" /> : null}
                  </div>
                );
              })}
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center gap-2">
              <CurrentIcon className="size-4 text-primary" />
              <h2 className="text-lg font-semibold tracking-normal">{currentStep.title}</h2>
            </div>
            {props.step === 0 ? <IdentityStep form={props.form} options={props.createOptions} teams={props.teams} teamsError={props.teamsError} teamsLoading={props.teamsLoading} /> : null}
            {props.step === 1 ? <CapabilityStep form={props.form} options={props.createOptions} /> : null}
            {props.step === 2 ? <GovernanceStep options={props.createOptions} /> : null}
            {props.step === 3 ? <RuntimeStep form={props.form} loading={props.createOptionsLoading} options={props.createOptions} /> : null}
            {props.createOptionsError ? (
              <Alert variant="destructive">
                <AlertTitle>创建候选加载失败</AlertTitle>
                <AlertDescription>请确认团队已有 active 治理配置。</AlertDescription>
              </Alert>
            ) : null}
            {props.submitError ? <p className="text-sm text-destructive">{props.submitError}</p> : null}
            <div className="flex justify-between border-t pt-4">
              <Button disabled={props.step === 0} onClick={props.onBack} type="button" variant="outline">
                <ChevronLeft className="size-4" />
                上一步
              </Button>
              {props.step < 3 ? (
                <Button onClick={props.onNext} type="button">
                  下一步
                  <ChevronRight className="size-4" />
                </Button>
              ) : (
                <Button disabled={!canSubmit} onClick={props.onSubmit} type="button">
                  创建数字员工
                </Button>
              )}
            </div>
          </CardContent>
        </Card>
      </form>
    </Form>
  );
}
```

`IdentityStep` 使用 `Select` 选择团队和专业类型，名称、描述使用 `Input` 和 `Textarea`。`CapabilityStep` 使用 `Checkbox` 选择 skills、MCP servers、external capabilities。`GovernanceStep` 展示团队策略摘要和默认风险等级，不暴露裸 JSON 编辑器。`RuntimeStep` 使用 `RadioGroup` 列出 `runtime_provider_options`，不可用候选显示 `disabled_reason` 且不能选中。

- [ ] **Step 6: 实现提交 payload helper**

在 `apps/web/src/features/employees/create.tsx` 增加：

```tsx
function runtimeBindingValue(option: DigitalEmployeeCreateOptions["runtime_provider_options"][number]) {
  return `${option.runtime_node_id}:${option.provider_type}`;
}

function applyEmployeeTypeDefaults(form: ReturnType<typeof useForm<CreateEmployeeFormValues>>, option: DigitalEmployeeCreateOptions["employee_types"][number]) {
  form.setValue("employee_type", option.type, { shouldValidate: true });
  form.setValue("role", option.default_role ?? option.type, { shouldValidate: true });
  form.setValue("risk_level", option.default_risk_level ?? "medium", { shouldValidate: true });
  form.setValue("enabled_skills", option.recommended_skill_keys ?? [], { shouldValidate: true });
  form.setValue("enabled_mcp_servers", option.recommended_mcp_servers ?? [], { shouldValidate: true });
  form.setValue("enabled_external_capabilities", option.recommended_external_capabilities ?? [], { shouldValidate: true });
}

function buildCreateInput(
  values: CreateEmployeeFormValues,
  employeeType: DigitalEmployeeCreateOptions["employee_types"][number],
  runtime: DigitalEmployeeCreateOptions["runtime_provider_options"][number],
) {
  return {
    team_id: values.team_id,
    employee_type: values.employee_type,
    name: values.name.trim(),
    role: values.role.trim(),
    description: values.description?.trim() || undefined,
    risk_level: values.risk_level,
    role_profile: employeeType.default_role_profile ?? {},
    capability_selection: {
      enabled_skills: values.enabled_skills,
      enabled_mcp_servers: values.enabled_mcp_servers,
      enabled_external_capabilities: values.enabled_external_capabilities,
      enabled_provider_types: [runtime.provider_type],
    },
    context_policy_override: employeeType.default_context_policy_override ?? {},
    approval_policy_override: employeeType.default_approval_policy_override ?? {},
    output_contract_addendum: employeeType.default_output_contract_addendum ?? {},
    runtime_node_id: runtime.runtime_node_id,
    provider_type: runtime.provider_type,
    session_policy: { mode: "reuse_latest" },
    workspace_policy: {},
  };
}
```

- [ ] **Step 7: 更新列表页创建入口**

在 `apps/web/src/features/employees/index.tsx` 移除内嵌创建面板相关 state 和 mutation，按钮改为链接：

```tsx
<Button asChild type="button">
  <Link to="/employees/new">
    <Plus className="size-4" />
    创建数字员工
  </Link>
</Button>
```

保留列表查询和 `EmployeeRow`。

- [ ] **Step 8: 更新列表页测试**

在 `apps/web/src/features/employees/index.test.tsx` 删除草稿预览相关测试，新增创建入口断言：

```tsx
it("links to the digital employee creation wizard", async () => {
  const fetcher = createEmployeesFetcher();
  const screen = await renderEmployeesView(fetcher);

  await expect
    .element(screen.getByRole("link", { name: "创建数字员工" }))
    .toHaveAttribute("href", "/employees/new");
});
```

Mock `Link` 支持无 `params`：

```tsx
Link: ({ children, params, to }: { children: ReactNode; params?: Record<string, string>; to: string }) => (
  <a href={params?.employeeId ? to.replace("$employeeId", encodeURIComponent(params.employeeId)) : to}>{children}</a>
),
```

- [ ] **Step 9: 运行前端页面测试**

Run: `pnpm --filter @superteam/web test -- src/features/employees/create.test.tsx src/features/employees/index.test.tsx`

Expected: PASS。

- [ ] **Step 10: 运行 typecheck 生成 route tree**

Run: `pnpm --filter @superteam/web typecheck`

Expected: PASS，并更新 `apps/web/src/routeTree.gen.ts`。

- [ ] **Step 11: 提交前端页面**

```bash
git add \
  apps/web/src/features/employees/create.tsx \
  apps/web/src/features/employees/create.test.tsx \
  apps/web/src/features/employees/index.tsx \
  apps/web/src/features/employees/index.test.tsx \
  apps/web/src/routes/_authenticated/employees/new.tsx \
  apps/web/src/routeTree.gen.ts
git commit -m "feat: add digital employee creation wizard"
```

## Task 7: 验证、变更日志和收尾

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: 运行后端验证**

Run: `go test ./apps/control-plane/internal/employee ./apps/control-plane/internal/api ./apps/control-plane/internal/storage ./apps/control-plane/internal/storage/queries -count=1`

Expected: PASS。

- [ ] **Step 2: 运行前端验证**

Run: `pnpm --filter @superteam/web test -- src/lib/api/employees.test.ts src/features/employees/create.test.tsx src/features/employees/index.test.tsx`

Expected: PASS。

- [ ] **Step 3: 运行 OpenAPI 和全局契约验证**

Run: `pnpm verify:contracts`

Expected: PASS。

- [ ] **Step 4: 运行 Web typecheck**

Run: `pnpm --filter @superteam/web typecheck`

Expected: PASS。

- [ ] **Step 5: 运行 Web build**

Run: `pnpm --filter @superteam/web build`

Expected: PASS。

- [ ] **Step 6: 追加 CHANGELOG**

获取本地时间：

Run: `TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'`

Expected: 输出格式为 `2026-06-05 22:09`。

在 `CHANGELOG.md` 添加一条：

```markdown
- 2026-06-05 22:09：完成数字员工创建闭环，实现 Owner 归属、专业类型注册表、创建候选接口、ready 创建编排、前端四步创建向导、OpenAPI 与数据库迁移。
```

实际提交时使用命令输出的当前时间替换示例时间。

- [ ] **Step 7: 运行 diff 检查**

Run: `git diff --check`

Expected: PASS，无 trailing whitespace 或 conflict marker。

- [ ] **Step 8: 提交收尾**

```bash
git add CHANGELOG.md
git commit -m "chore: record digital employee creation changes"
```

- [ ] **Step 9: 最终状态检查**

Run: `git status --short`

Expected: 只允许出现执行前就存在的无关 `?? docx/`，不能有本计划改动的未提交文件。

## 自检清单

- 项目协调员没有进入 `employee_type` 注册表。
- `owner_user_id` 只由后端从 `middleware.GetUserID(r.Context())` 注入。
- 创建成功语义是 `ready`，没有创建 run，没有创建任务执行记录。
- Web 不直连 Runtime 或 Provider。
- `employee_type` 是 `VARCHAR(100)` 加服务端注册表校验，不是数据库 enum。
- 创建页只暴露结构化表单，不提供裸 JSON 编辑器。
- `/digital-employees/create-options` 注册在 `/{employeeId}` 前。
- 迁移是 forward migration，不修改既有 shared initial migration。
- Changelog 使用 `Asia/Shanghai` 本地时间。
