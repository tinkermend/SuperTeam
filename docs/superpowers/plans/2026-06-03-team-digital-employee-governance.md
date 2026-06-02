# Team Digital Employee Governance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first version of team-governed digital employee creation: teams define public governance boundaries, digital employees inherit those boundaries, and effective configuration previews/approvals enforce the boundary before activation.

**Architecture:** Extend the current Control Plane with a new `tenant` service for team profiles and config revisions, then extend the existing `employee` service with personal config revisions and effective config validation. Web adds a `/teams` page plus a digital employee creation flow that previews team inheritance and blocking validation before approval.

**Tech Stack:** Go 1.25, chi/net/http, pgx/v5, sqlc, PostgreSQL, OpenAPI YAML, React 19, Vite, TanStack Router, TanStack Query, shadcn/ui, Vitest browser tests.

---

## Scope Check

This plan implements one product slice from `docs/superpowers/specs/2026-06-03-team-digital-employee-governance-design.md`: creating and governing digital employees through team configuration. It intentionally does not implement full task orchestration, cross-team transfer workflow, or a capability registry marketplace.

## File Structure

### Backend Database and SQL

- Modify: `apps/control-plane/internal/storage/migrations/001_initial.sql`
  - Add `human_owner_user_id` to `tenant_teams`.
  - Add `tenant_team_config_revisions`.
  - Add `digital_employee_config_revisions`.
  - Add `digital_employee_effective_configs`.
  - Add indexes, triggers, and Chinese comments.
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
  - Assert new tables are UUID-first and commented.
- Modify: `apps/control-plane/internal/storage/queries/queries_test.go`
  - Add integration coverage for team config and effective config queries.
- Create: `apps/control-plane/internal/storage/queries/tenant_team_config.sql`
  - sqlc queries for teams and team config revisions.
- Create: `apps/control-plane/internal/storage/queries/digital_employee_config.sql`
  - sqlc queries for employee config revisions and effective config snapshots.
- Regenerate: `apps/control-plane/internal/storage/queries/*.go`

### Backend Domain Services

- Create: `apps/control-plane/internal/tenant/types.go`
  - Team profile, team config revision, structured policy value types.
- Create: `apps/control-plane/internal/tenant/repository.go`
  - Repository interface and parameter records.
- Create: `apps/control-plane/internal/tenant/pg_repository.go`
  - sqlc-backed repository.
- Create: `apps/control-plane/internal/tenant/service.go`
  - Team creation, listing, config revision creation, current config lookup.
- Create: `apps/control-plane/internal/tenant/handler.go`
  - HTTP handlers for `/api/v1/teams`.
- Create: `apps/control-plane/internal/tenant/service_test.go`
  - In-memory service tests.
- Modify: `apps/control-plane/internal/employee/types.go`
  - Add personal config revision and effective config domain types.
- Modify: `apps/control-plane/internal/employee/repository.go`
  - Add repository methods for personal revisions and effective snapshots.
- Modify: `apps/control-plane/internal/employee/pg_repository.go`
  - Implement new repository methods.
- Modify: `apps/control-plane/internal/employee/service.go`
  - Add `CreateConfigRevision`, `PreviewEffectiveConfig`, `ApproveEffectiveConfig`.
- Modify: `apps/control-plane/internal/employee/handler.go`
  - Add handlers for config revision, preview, and approve.
- Modify: `apps/control-plane/internal/employee/service_test.go`
  - Add boundary validation tests.
- Modify: `apps/control-plane/internal/app/app.go`
  - Wire tenant repository/service/handler.
- Modify: `apps/control-plane/internal/api/server.go`
  - Register team routes and employee config routes.
- Modify: `apps/control-plane/internal/api/routes_test.go`
  - Add route registration/auth tests.
- Modify: `apps/control-plane/internal/api/employee_routes_test.go`
  - Add config preview/approve route tests.

### Contracts and Changelog

- Modify: `contracts/control-plane/openapi.yaml`
  - Add team schemas and employee effective config endpoints.
- Modify: `CHANGELOG.md`
  - Add a concise Unreleased entry.

### Web

- Create: `apps/web/src/lib/api/teams.ts`
  - Team API client.
- Create: `apps/web/src/lib/api/teams.test.ts`
  - Team client request tests.
- Modify: `apps/web/src/lib/api/employees.ts`
  - Add config revision, preview, and approve client methods.
- Modify: `apps/web/src/lib/api/employees.test.ts`
  - Add employee governance API tests.
- Create: `apps/web/src/features/teams/index.tsx`
  - Team list and governance summary page.
- Create: `apps/web/src/features/teams/index.test.tsx`
  - Team page rendering tests.
- Create: `apps/web/src/routes/_authenticated/teams/index.tsx`
  - TanStack route for `/teams`.
- Modify: `apps/web/src/features/employees/index.tsx`
  - Add create digital employee flow and effective config preview panel.
- Modify: `apps/web/src/features/employees/index.test.tsx`
  - Add create flow tests.
- Modify: `apps/web/src/components/layout/data/sidebar-data.ts`
  - Add “团队管理”.
- Modify: `apps/web/src/components/layout/sidebar-data.test.ts`
  - Update expected icon tone map.
- Regenerate: `apps/web/src/routeTree.gen.ts`

---

### Task 1: Add Database Shape for Team Config and Effective Employee Config

**Files:**
- Modify: `apps/control-plane/internal/storage/migrations/001_initial.sql`
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
- Modify: `apps/control-plane/internal/storage/queries/queries_test.go`
- Create: `apps/control-plane/internal/storage/queries/tenant_team_config.sql`
- Create: `apps/control-plane/internal/storage/queries/digital_employee_config.sql`
- Regenerate: `apps/control-plane/internal/storage/queries/*.go`

- [ ] **Step 1: Write failing schema tests**

Add the new tables to `uuidFirstTables` in `apps/control-plane/internal/storage/migrations_test.go`:

```go
var uuidFirstTables = []string{
	"tenants",
	"tenant_profiles",
	"tenant_teams",
	"tenant_team_config_revisions",
	"auth_users",
	"tenant_members",
	"runtime_nodes",
	"runtime_node_scopes",
	"auth_runtime_tokens",
	"runtime_bootstrap_keys",
	"runtime_enrollments",
	"runtime_sessions",
	"runtime_capabilities",
	"auth_sessions",
	"digital_employees",
	"digital_employee_config_revisions",
	"digital_employee_effective_configs",
	"digital_employee_execution_instances",
	"provider_sessions",
	"provider_session_events",
	"tasks",
	"task_runs",
	"runtime_leases",
	"task_state_history",
	"task_events",
	"task_artifacts",
	"audit_events",
	"web_login_logs",
	"web_operation_logs",
}
```

Add assertions to `TestInitialSchemaIsUUIDFirst`:

```go
for _, expected := range []string{
	"CREATE TABLE tenant_team_config_revisions",
	"CREATE TABLE digital_employee_config_revisions",
	"CREATE TABLE digital_employee_effective_configs",
	"human_owner_user_id UUID",
	"COMMENT ON TABLE tenant_team_config_revisions IS",
	"COMMENT ON COLUMN tenant_team_config_revisions.internal_collaboration_policy IS",
	"COMMENT ON TABLE digital_employee_effective_configs IS",
	"COMMENT ON COLUMN digital_employee_effective_configs.validation_result IS",
} {
	if !strings.Contains(sql, expected) {
		t.Fatalf("expected team governance schema to contain %q", expected)
	}
}
```

- [ ] **Step 2: Run schema tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/storage -run 'TestInitialSchemaIsUUIDFirst' -count=1
```

Expected: FAIL because the three new tables and comments do not exist.

- [ ] **Step 3: Add failing sqlc integration tests**

Append to `apps/control-plane/internal/storage/queries/queries_test.go`:

```go
func TestTeamConfigAndDigitalEmployeeEffectiveConfigQueries(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	owner, err := testQueries.CreateUser(ctx, queries.CreateUserParams{
		Username:     "ops-owner",
		DisplayName:  pgtype.Text{String: "Ops Owner", Valid: true},
		Email:        pgtype.Text{String: "ops-owner@example.com", Valid: true},
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	})
	require.NoError(t, err)

	team, err := testQueries.CreateTenantTeam(ctx, queries.CreateTenantTeamParams{
		TenantID:         tenantID,
		Slug:             "ops",
		Name:             "运维团队",
		HumanOwnerUserID: uuid.NullUUID{UUID: owner.ID, Valid: true},
		Metadata:         []byte(`{"domain":"operations"}`),
	})
	require.NoError(t, err)

	teamConfig, err := testQueries.CreateTenantTeamConfigRevision(ctx, queries.CreateTenantTeamConfigRevisionParams{
		TenantID:                    tenantID,
		TeamID:                      team.ID,
		RevisionNumber:              1,
		Constitution:                []byte(`{"hard_rules":["禁止执行未审批的生产写操作"]}`),
		CapabilityPolicy:            []byte(`{"allowed_mcp_servers":["prometheus"],"allowed_skills":["incident-diagnosis"],"allowed_plugins":["log-viewer"],"allowed_provider_types":["codex"]}`),
		ContextPolicy:               []byte(`{"sources":["monitoring","logs"]}`),
		ApprovalPolicy:              []byte(`{"min_risk_for_human":"high","write_actions_require_human":true}`),
		ArtifactContract:            []byte(`{"required":["Finding","Risk","DecisionRequest"]}`),
		InternalCollaborationPolicy: []byte(`{"allowed_request_types":["info_request","review_request","artifact_request"],"max_auto_rounds":2,"max_auto_participants":3}`),
		RuntimeScopePolicy:          []byte(`{"allowed_provider_types":["codex"]}`),
		HumanOwnerUserID:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		Status:                      "active",
		ApprovedBy:                  uuid.NullUUID{UUID: owner.ID, Valid: true},
		ApprovedAt:                  pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	require.NoError(t, err)

	employee, err := testQueries.CreateDigitalEmployee(ctx, queries.CreateDigitalEmployeeParams{
		TenantID: tenantID,
		TeamID:   uuid.NullUUID{UUID: team.ID, Valid: true},
		Name:     "数据库运维员工",
		Role:     "database_operator",
		Status:   "draft",
		RiskLevel: "medium",
	})
	require.NoError(t, err)

	employeeConfig, err := testQueries.CreateDigitalEmployeeConfigRevision(ctx, queries.CreateDigitalEmployeeConfigRevisionParams{
		TenantID:                 tenantID,
		DigitalEmployeeID:        employee.ID,
		RevisionNumber:           1,
		RoleProfile:              []byte(`{"specialty":"postgres"}`),
		ConstitutionAddendum:     []byte(`{"required_output_rules":["输出慢查询证据"]}`),
		CapabilitySelection:      []byte(`{"enabled_mcp_servers":["prometheus"],"enabled_skills":["incident-diagnosis"],"enabled_plugins":["log-viewer"]}`),
		ContextPolicyOverride:    []byte(`{"sources":["monitoring"]}`),
		ApprovalPolicyOverride:   []byte(`{"min_risk_for_human":"high"}`),
		OutputContractAddendum:   []byte(`{"required":["SlowQueryFinding"]}`),
		Status:                   "draft",
	})
	require.NoError(t, err)

	effective, err := testQueries.CreateDigitalEmployeeEffectiveConfig(ctx, queries.CreateDigitalEmployeeEffectiveConfigParams{
		TenantID:                     tenantID,
		DigitalEmployeeID:            employee.ID,
		TenantTeamConfigRevisionID:    teamConfig.ID,
		EmployeeConfigRevisionID:      employeeConfig.ID,
		EffectiveConfigSnapshot:       []byte(`{"team":{"revision":1},"employee":{"revision":1}}`),
		ValidationResult:              []byte(`{"blocking_errors":[],"warnings":[]}`),
		Status:                        "approved",
		ApprovedBy:                    uuid.NullUUID{UUID: owner.ID, Valid: true},
		ApprovedAt:                    pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	require.NoError(t, err)
	require.Equal(t, employee.ID, effective.DigitalEmployeeID)

	current, err := testQueries.GetCurrentTenantTeamConfigRevision(ctx, queries.GetCurrentTenantTeamConfigRevisionParams{
		TenantID: tenantID,
		TeamID:   team.ID,
	})
	require.NoError(t, err)
	require.Equal(t, teamConfig.ID, current.ID)
}
```

- [ ] **Step 4: Run sqlc query test and verify it fails**

Run:

```bash
ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 go test ./apps/control-plane/internal/storage/queries -run TestTeamConfigAndDigitalEmployeeEffectiveConfigQueries -count=1
```

Expected without DB env: the package exits with the existing skip message. Expected with DB env: FAIL because query methods such as `CreateTenantTeamConfigRevision` do not exist.

- [ ] **Step 5: Add migration tables and comments**

In `apps/control-plane/internal/storage/migrations/001_initial.sql`, modify `tenant_teams`:

```sql
    human_owner_user_id UUID,
```

Add these tables after `tenant_teams`:

```sql
CREATE TABLE tenant_team_config_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    team_id UUID NOT NULL,
    revision_number INTEGER NOT NULL,
    constitution JSONB NOT NULL DEFAULT '{}'::jsonb,
    capability_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    context_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    approval_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    artifact_contract JSONB NOT NULL DEFAULT '{}'::jsonb,
    internal_collaboration_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    runtime_scope_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    human_owner_user_id UUID,
    status VARCHAR(50) NOT NULL DEFAULT 'draft',
    approved_by UUID,
    approved_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, team_id, revision_number)
);
```

Add these tables after `digital_employees`:

```sql
CREATE TABLE digital_employee_config_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    digital_employee_id UUID NOT NULL,
    revision_number INTEGER NOT NULL,
    role_profile JSONB NOT NULL DEFAULT '{}'::jsonb,
    constitution_addendum JSONB NOT NULL DEFAULT '{}'::jsonb,
    capability_selection JSONB NOT NULL DEFAULT '{}'::jsonb,
    context_policy_override JSONB NOT NULL DEFAULT '{}'::jsonb,
    approval_policy_override JSONB NOT NULL DEFAULT '{}'::jsonb,
    output_contract_addendum JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(50) NOT NULL DEFAULT 'draft',
    approved_by UUID,
    approved_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, digital_employee_id, revision_number)
);

CREATE TABLE digital_employee_effective_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    digital_employee_id UUID NOT NULL,
    tenant_team_config_revision_id UUID NOT NULL,
    employee_config_revision_id UUID NOT NULL,
    effective_config_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    validation_result JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(50) NOT NULL DEFAULT 'pending_approval',
    approved_by UUID,
    approved_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Do not add database foreign keys for these team, employee, runtime, and approval references in this slice. This follows `DATABASE_DESIGN.md` application-controlled relationship rules: service methods validate tenant/team consistency, status, permissions, and runtime readiness before writes, while the database keeps UUID references, unique constraints, indexes, and audit snapshots.

Add indexes:

```sql
CREATE UNIQUE INDEX uq_tenant_team_config_revisions_current
    ON tenant_team_config_revisions(tenant_id, team_id)
    WHERE status = 'active' AND archived_at IS NULL;
CREATE INDEX idx_tenant_team_config_revisions_team
    ON tenant_team_config_revisions(tenant_id, team_id, revision_number DESC);
CREATE INDEX idx_digital_employee_config_revisions_employee
    ON digital_employee_config_revisions(tenant_id, digital_employee_id, revision_number DESC);
CREATE UNIQUE INDEX uq_digital_employee_effective_configs_active
    ON digital_employee_effective_configs(tenant_id, digital_employee_id)
    WHERE status = 'approved' AND revoked_at IS NULL;
CREATE INDEX idx_digital_employee_effective_configs_employee
    ON digital_employee_effective_configs(tenant_id, digital_employee_id, created_at DESC);
```

Add `updated_at` triggers:

```sql
CREATE TRIGGER update_tenant_team_config_revisions_updated_at BEFORE UPDATE ON tenant_team_config_revisions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_digital_employee_config_revisions_updated_at BEFORE UPDATE ON digital_employee_config_revisions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_digital_employee_effective_configs_updated_at BEFORE UPDATE ON digital_employee_effective_configs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
```

Add this comment block after the existing `tenant_teams` comments:

```sql
COMMENT ON COLUMN tenant_teams.human_owner_user_id IS '团队负责人用户ID，第一版用于团队级审批、升级和跨团队交接决策';

COMMENT ON TABLE tenant_team_config_revisions IS '团队治理配置版本表';
COMMENT ON COLUMN tenant_team_config_revisions.id IS '团队治理配置版本ID';
COMMENT ON COLUMN tenant_team_config_revisions.tenant_id IS '租户ID';
COMMENT ON COLUMN tenant_team_config_revisions.team_id IS '团队ID';
COMMENT ON COLUMN tenant_team_config_revisions.revision_number IS '团队配置版本号，同一团队内递增';
COMMENT ON COLUMN tenant_team_config_revisions.constitution IS '团队公共宪法，定义硬性规则、工作原则和禁止行为';
COMMENT ON COLUMN tenant_team_config_revisions.capability_policy IS '团队公共能力策略，定义允许的MCP、技能、插件和Provider类型';
COMMENT ON COLUMN tenant_team_config_revisions.context_policy IS '团队公共上下文策略，定义可注入上下文来源、范围和保留规则';
COMMENT ON COLUMN tenant_team_config_revisions.approval_policy IS '团队公共审批策略，定义风险阈值和必须人类审批的动作';
COMMENT ON COLUMN tenant_team_config_revisions.artifact_contract IS '团队工件契约，定义交接时必须产出的结构化对象';
COMMENT ON COLUMN tenant_team_config_revisions.internal_collaboration_policy IS '团队内部协作策略，定义同团队数字员工自动问询的边界';
COMMENT ON COLUMN tenant_team_config_revisions.runtime_scope_policy IS '团队Runtime范围策略，定义可使用的执行节点、Provider和环境边界';
COMMENT ON COLUMN tenant_team_config_revisions.human_owner_user_id IS '该版本配置的团队负责人用户ID';
COMMENT ON COLUMN tenant_team_config_revisions.status IS '配置状态：draft、active、archived';
COMMENT ON COLUMN tenant_team_config_revisions.approved_by IS '批准该配置版本的用户ID';
COMMENT ON COLUMN tenant_team_config_revisions.approved_at IS '配置版本批准时间';
COMMENT ON COLUMN tenant_team_config_revisions.archived_at IS '配置版本归档时间';
COMMENT ON COLUMN tenant_team_config_revisions.created_at IS '创建时间';
COMMENT ON COLUMN tenant_team_config_revisions.updated_at IS '更新时间';

COMMENT ON TABLE digital_employee_config_revisions IS '数字员工个人治理配置版本表';
COMMENT ON COLUMN digital_employee_config_revisions.id IS '数字员工个人配置版本ID';
COMMENT ON COLUMN digital_employee_config_revisions.tenant_id IS '租户ID';
COMMENT ON COLUMN digital_employee_config_revisions.digital_employee_id IS '数字员工ID';
COMMENT ON COLUMN digital_employee_config_revisions.revision_number IS '个人配置版本号，同一数字员工内递增';
COMMENT ON COLUMN digital_employee_config_revisions.role_profile IS '角色画像，描述数字员工专业方向和职责';
COMMENT ON COLUMN digital_employee_config_revisions.constitution_addendum IS '个人宪法补充，只能收紧或补充团队宪法';
COMMENT ON COLUMN digital_employee_config_revisions.capability_selection IS '个人能力选择，只能从团队允许范围内启用MCP、技能和插件';
COMMENT ON COLUMN digital_employee_config_revisions.context_policy_override IS '个人上下文策略覆盖，只能收紧团队上下文策略';
COMMENT ON COLUMN digital_employee_config_revisions.approval_policy_override IS '个人审批策略覆盖，只能收紧团队审批策略';
COMMENT ON COLUMN digital_employee_config_revisions.output_contract_addendum IS '个人输出契约补充，定义额外交接工件要求';
COMMENT ON COLUMN digital_employee_config_revisions.status IS '配置状态：draft、pending_approval、active、archived';
COMMENT ON COLUMN digital_employee_config_revisions.approved_by IS '批准该个人配置版本的用户ID';
COMMENT ON COLUMN digital_employee_config_revisions.approved_at IS '个人配置版本批准时间';
COMMENT ON COLUMN digital_employee_config_revisions.archived_at IS '个人配置版本归档时间';
COMMENT ON COLUMN digital_employee_config_revisions.created_at IS '创建时间';
COMMENT ON COLUMN digital_employee_config_revisions.updated_at IS '更新时间';

COMMENT ON TABLE digital_employee_effective_configs IS '数字员工生效治理配置快照表';
COMMENT ON COLUMN digital_employee_effective_configs.id IS '生效配置快照ID';
COMMENT ON COLUMN digital_employee_effective_configs.tenant_id IS '租户ID';
COMMENT ON COLUMN digital_employee_effective_configs.digital_employee_id IS '数字员工ID';
COMMENT ON COLUMN digital_employee_effective_configs.tenant_team_config_revision_id IS '参与合成的团队配置版本ID';
COMMENT ON COLUMN digital_employee_effective_configs.employee_config_revision_id IS '参与合成的个人配置版本ID';
COMMENT ON COLUMN digital_employee_effective_configs.effective_config_snapshot IS '团队配置与个人配置合成后的生效治理配置快照';
COMMENT ON COLUMN digital_employee_effective_configs.validation_result IS '生效配置校验结果，包含阻断错误和警告';
COMMENT ON COLUMN digital_employee_effective_configs.status IS '生效配置状态：pending_approval、approved、revoked';
COMMENT ON COLUMN digital_employee_effective_configs.approved_by IS '批准生效配置的用户ID';
COMMENT ON COLUMN digital_employee_effective_configs.approved_at IS '生效配置批准时间';
COMMENT ON COLUMN digital_employee_effective_configs.revoked_at IS '生效配置撤销时间';
COMMENT ON COLUMN digital_employee_effective_configs.created_at IS '创建时间';
COMMENT ON COLUMN digital_employee_effective_configs.updated_at IS '更新时间';
```

- [ ] **Step 6: Add sqlc queries**

Create `apps/control-plane/internal/storage/queries/tenant_team_config.sql`:

```sql
-- name: CreateTenantTeam :one
INSERT INTO tenant_teams (tenant_id, slug, name, status, human_owner_user_id, metadata)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('slug')::varchar,
    sqlc.arg('name')::varchar,
    sqlc.arg('status')::varchar,
    sqlc.narg('human_owner_user_id')::uuid,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
)
RETURNING *;

-- name: ListTenantTeams :many
SELECT *
FROM tenant_teams
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: GetTenantTeam :one
SELECT *
FROM tenant_teams
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL;

-- name: CreateTenantTeamConfigRevision :one
INSERT INTO tenant_team_config_revisions (
    tenant_id,
    team_id,
    revision_number,
    constitution,
    capability_policy,
    context_policy,
    approval_policy,
    artifact_contract,
    internal_collaboration_policy,
    runtime_scope_policy,
    human_owner_user_id,
    status,
    approved_by,
    approved_at
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('team_id')::uuid,
    sqlc.arg('revision_number')::integer,
    COALESCE(sqlc.arg('constitution')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('capability_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('context_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('approval_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('artifact_contract')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('internal_collaboration_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('runtime_scope_policy')::jsonb, '{}'::jsonb),
    sqlc.narg('human_owner_user_id')::uuid,
    sqlc.arg('status')::varchar,
    sqlc.narg('approved_by')::uuid,
    sqlc.narg('approved_at')::timestamptz
)
RETURNING *;

-- name: GetCurrentTenantTeamConfigRevision :one
SELECT *
FROM tenant_team_config_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND status = 'active'
  AND archived_at IS NULL
ORDER BY revision_number DESC
LIMIT 1;

-- name: GetTenantTeamConfigRevision :one
SELECT *
FROM tenant_team_config_revisions
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND archived_at IS NULL;

-- name: GetNextTenantTeamConfigRevisionNumber :one
SELECT (COALESCE(MAX(revision_number), 0) + 1)::integer
FROM tenant_team_config_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid;
```

Create `apps/control-plane/internal/storage/queries/digital_employee_config.sql`:

```sql
-- name: CreateDigitalEmployeeConfigRevision :one
INSERT INTO digital_employee_config_revisions (
    tenant_id,
    digital_employee_id,
    revision_number,
    role_profile,
    constitution_addendum,
    capability_selection,
    context_policy_override,
    approval_policy_override,
    output_contract_addendum,
    status,
    approved_by,
    approved_at
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('revision_number')::integer,
    COALESCE(sqlc.arg('role_profile')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('constitution_addendum')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('capability_selection')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('context_policy_override')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('approval_policy_override')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('output_contract_addendum')::jsonb, '{}'::jsonb),
    sqlc.arg('status')::varchar,
    sqlc.narg('approved_by')::uuid,
    sqlc.narg('approved_at')::timestamptz
)
RETURNING *;

-- name: GetLatestDigitalEmployeeConfigRevision :one
SELECT *
FROM digital_employee_config_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
ORDER BY revision_number DESC
LIMIT 1;

-- name: GetDigitalEmployeeConfigRevision :one
SELECT *
FROM digital_employee_config_revisions
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND archived_at IS NULL;

-- name: GetNextDigitalEmployeeConfigRevisionNumber :one
SELECT (COALESCE(MAX(revision_number), 0) + 1)::integer
FROM digital_employee_config_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid;

-- name: CreateDigitalEmployeeEffectiveConfig :one
INSERT INTO digital_employee_effective_configs (
    tenant_id,
    digital_employee_id,
    tenant_team_config_revision_id,
    employee_config_revision_id,
    effective_config_snapshot,
    validation_result,
    status,
    approved_by,
    approved_at
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('tenant_team_config_revision_id')::uuid,
    sqlc.arg('employee_config_revision_id')::uuid,
    COALESCE(sqlc.arg('effective_config_snapshot')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('validation_result')::jsonb, '{}'::jsonb),
    sqlc.arg('status')::varchar,
    sqlc.narg('approved_by')::uuid,
    sqlc.narg('approved_at')::timestamptz
)
RETURNING *;

-- name: GetLatestDigitalEmployeeEffectiveConfig :one
SELECT *
FROM digital_employee_effective_configs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
ORDER BY created_at DESC
LIMIT 1;
```

- [ ] **Step 7: Regenerate sqlc**

Run:

```bash
make -C apps/control-plane generate-sqlc
```

Expected: output includes `sqlc generation complete`.

- [ ] **Step 8: Run database and storage verification**

Run:

```bash
go test ./apps/control-plane/internal/storage -count=1
go test ./apps/control-plane/internal/storage/queries -run TestTeamConfigAndDigitalEmployeeEffectiveConfigQueries -count=1
```

Expected: first command PASS. Second command either prints the existing skip message when DB env is absent, or PASS when DB env is configured.

- [ ] **Step 9: Commit database shape**

```bash
git add apps/control-plane/internal/storage/migrations/001_initial.sql \
  apps/control-plane/internal/storage/migrations_test.go \
  apps/control-plane/internal/storage/queries \
  apps/control-plane/internal/storage/queries/queries_test.go
git commit -m "feat: add team governance schema"
```

### Task 2: Add Team Domain Service and Routes

**Files:**
- Create: `apps/control-plane/internal/tenant/types.go`
- Create: `apps/control-plane/internal/tenant/repository.go`
- Create: `apps/control-plane/internal/tenant/pg_repository.go`
- Create: `apps/control-plane/internal/tenant/service.go`
- Create: `apps/control-plane/internal/tenant/handler.go`
- Create: `apps/control-plane/internal/tenant/service_test.go`
- Modify: `apps/control-plane/internal/app/app.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/api/routes_test.go`

- [ ] **Step 1: Write failing service tests**

Create `apps/control-plane/internal/tenant/service_test.go`:

```go
package tenant

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCreateTeamRequiresHumanOwner(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID: uuid.New(),
		Slug:     "ops",
		Name:     "运维团队",
	})
	if err == nil {
		t.Fatalf("expected missing human owner to fail")
	}
}

func TestCreateTeamConfigRevisionDefaultsActiveStatus(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	ownerID := uuid.New()
	team, err := service.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:         tenantID,
		Slug:             "ops",
		Name:             "运维团队",
		HumanOwnerUserID: &ownerID,
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	revision, err := service.CreateConfigRevision(context.Background(), CreateConfigRevisionRequest{
		TenantID:         tenantID,
		TeamID:           team.ID,
		HumanOwnerUserID: &ownerID,
		Constitution: map[string]any{
			"hard_rules": []any{"禁止执行未审批的生产写操作"},
		},
		CapabilityPolicy: map[string]any{
			"allowed_skills": []any{"incident-diagnosis"},
		},
	})
	if err != nil {
		t.Fatalf("create config revision: %v", err)
	}

	if revision.Status != TeamConfigRevisionStatusActive {
		t.Fatalf("expected active revision, got %q", revision.Status)
	}
	if revision.RevisionNumber != 1 {
		t.Fatalf("expected first revision number 1, got %d", revision.RevisionNumber)
	}
}
```

- [ ] **Step 2: Run service tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/tenant -count=1
```

Expected: FAIL because package `tenant` does not exist.

- [ ] **Step 3: Add tenant domain types**

Create `apps/control-plane/internal/tenant/types.go`:

```go
package tenant

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrInvalidInput = errors.New("invalid tenant input")

type TeamStatus string
type TeamConfigRevisionStatus string

const (
	TeamStatusActive   TeamStatus = "active"
	TeamStatusDisabled TeamStatus = "disabled"

	TeamConfigRevisionStatusDraft  TeamConfigRevisionStatus = "draft"
	TeamConfigRevisionStatusActive TeamConfigRevisionStatus = "active"
)

type Team struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	Slug             string
	Name             string
	Status           TeamStatus
	HumanOwnerUserID *uuid.UUID
	Metadata         map[string]any
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type TeamConfigRevision struct {
	ID                          uuid.UUID
	TenantID                    uuid.UUID
	TeamID                      uuid.UUID
	RevisionNumber              int32
	Constitution                map[string]any
	CapabilityPolicy            map[string]any
	ContextPolicy               map[string]any
	ApprovalPolicy              map[string]any
	ArtifactContract            map[string]any
	InternalCollaborationPolicy map[string]any
	RuntimeScopePolicy          map[string]any
	HumanOwnerUserID            *uuid.UUID
	Status                      TeamConfigRevisionStatus
	ApprovedBy                  *uuid.UUID
	ApprovedAt                  *time.Time
	CreatedAt                   time.Time
	UpdatedAt                   time.Time
}

type CreateTeamRequest struct {
	TenantID         uuid.UUID
	Slug             string
	Name             string
	HumanOwnerUserID *uuid.UUID
	Metadata         map[string]any
}

type ListTeamsRequest struct {
	TenantID uuid.UUID
	Status   TeamStatus
	Offset   int32
	Limit    int32
}

type CreateConfigRevisionRequest struct {
	TenantID                    uuid.UUID
	TeamID                      uuid.UUID
	HumanOwnerUserID            *uuid.UUID
	Constitution                map[string]any
	CapabilityPolicy            map[string]any
	ContextPolicy               map[string]any
	ApprovalPolicy              map[string]any
	ArtifactContract            map[string]any
	InternalCollaborationPolicy map[string]any
	RuntimeScopePolicy          map[string]any
	ApprovedBy                  *uuid.UUID
}
```

- [ ] **Step 4: Add repository interface and in-memory-compatible records**

Create `apps/control-plane/internal/tenant/repository.go`:

```go
package tenant

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	CreateTeam(ctx context.Context, params CreateTeamParams) (TeamRecord, error)
	ListTeams(ctx context.Context, params ListTeamsParams) ([]TeamRecord, error)
	GetTeam(ctx context.Context, tenantID, teamID uuid.UUID) (TeamRecord, error)
	CreateTeamConfigRevision(ctx context.Context, params CreateTeamConfigRevisionParams) (TeamConfigRevisionRecord, error)
	GetTeamConfigRevision(ctx context.Context, tenantID, revisionID uuid.UUID) (TeamConfigRevisionRecord, error)
	GetCurrentTeamConfigRevision(ctx context.Context, tenantID, teamID uuid.UUID) (TeamConfigRevisionRecord, error)
	GetNextTeamConfigRevisionNumber(ctx context.Context, tenantID, teamID uuid.UUID) (int32, error)
}

type CreateTeamParams struct {
	TenantID         uuid.UUID
	Slug             string
	Name             string
	Status           TeamStatus
	HumanOwnerUserID *uuid.UUID
	Metadata         map[string]any
}

type ListTeamsParams struct {
	TenantID uuid.UUID
	Status   TeamStatus
	Offset   int32
	Limit    int32
}

type CreateTeamConfigRevisionParams struct {
	TenantID                    uuid.UUID
	TeamID                      uuid.UUID
	RevisionNumber              int32
	Constitution                map[string]any
	CapabilityPolicy            map[string]any
	ContextPolicy               map[string]any
	ApprovalPolicy              map[string]any
	ArtifactContract            map[string]any
	InternalCollaborationPolicy map[string]any
	RuntimeScopePolicy          map[string]any
	HumanOwnerUserID            *uuid.UUID
	Status                      TeamConfigRevisionStatus
	ApprovedBy                  *uuid.UUID
	ApprovedAt                  *time.Time
}

type TeamRecord = Team
type TeamConfigRevisionRecord = TeamConfigRevision
```

- [ ] **Step 5: Add service implementation**

Create `apps/control-plane/internal/tenant/service.go` with:

```go
package tenant

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidInput)
	}
	return &Service{repository: repository}, nil
}

func (s *Service) CreateTeam(ctx context.Context, req CreateTeamRequest) (*Team, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.HumanOwnerUserID == nil || *req.HumanOwnerUserID == uuid.Nil {
		return nil, fmt.Errorf("%w: human_owner_user_id is required", ErrInvalidInput)
	}
	slug := strings.TrimSpace(req.Slug)
	name := strings.TrimSpace(req.Name)
	if slug == "" || name == "" {
		return nil, fmt.Errorf("%w: slug and name are required", ErrInvalidInput)
	}

	record, err := s.repository.CreateTeam(ctx, CreateTeamParams{
		TenantID:         req.TenantID,
		Slug:             slug,
		Name:             name,
		Status:           TeamStatusActive,
		HumanOwnerUserID: req.HumanOwnerUserID,
		Metadata:         cloneMap(req.Metadata),
	})
	if err != nil {
		return nil, fmt.Errorf("create team: %w", err)
	}
	return &record, nil
}

func (s *Service) CreateConfigRevision(ctx context.Context, req CreateConfigRevisionRequest) (*TeamConfigRevision, error) {
	if req.TenantID == uuid.Nil || req.TeamID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id and team_id are required", ErrInvalidInput)
	}
	if req.HumanOwnerUserID == nil || *req.HumanOwnerUserID == uuid.Nil {
		return nil, fmt.Errorf("%w: human_owner_user_id is required", ErrInvalidInput)
	}
	next, err := s.repository.GetNextTeamConfigRevisionNumber(ctx, req.TenantID, req.TeamID)
	if err != nil {
		return nil, fmt.Errorf("get next team config revision number: %w", err)
	}
	now := time.Now().UTC()
	record, err := s.repository.CreateTeamConfigRevision(ctx, CreateTeamConfigRevisionParams{
		TenantID:                    req.TenantID,
		TeamID:                      req.TeamID,
		RevisionNumber:              next,
		Constitution:                cloneMap(req.Constitution),
		CapabilityPolicy:            cloneMap(req.CapabilityPolicy),
		ContextPolicy:               cloneMap(req.ContextPolicy),
		ApprovalPolicy:              cloneMap(req.ApprovalPolicy),
		ArtifactContract:            cloneMap(req.ArtifactContract),
		InternalCollaborationPolicy: cloneMap(req.InternalCollaborationPolicy),
		RuntimeScopePolicy:          cloneMap(req.RuntimeScopePolicy),
		HumanOwnerUserID:            req.HumanOwnerUserID,
		Status:                      TeamConfigRevisionStatusActive,
		ApprovedBy:                  req.ApprovedBy,
		ApprovedAt:                  &now,
	})
	if err != nil {
		return nil, fmt.Errorf("create team config revision: %w", err)
	}
	return &record, nil
}

func cloneMap(value map[string]any) map[string]any {
	cloned := make(map[string]any)
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}
```

- [ ] **Step 6: Add `ListTeams`, `GetTeam`, and `GetCurrentConfigRevision`**

Append to `apps/control-plane/internal/tenant/service.go`:

```go
func (s *Service) ListTeams(ctx context.Context, req ListTeamsRequest) ([]*Team, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.Limit <= 0 {
		req.Limit = 50
	}
	if req.Limit > 100 {
		req.Limit = 100
	}
	records, err := s.repository.ListTeams(ctx, ListTeamsParams{
		TenantID: req.TenantID,
		Status:   req.Status,
		Offset:   req.Offset,
		Limit:    req.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	teams := make([]*Team, 0, len(records))
	for _, record := range records {
		copied := record
		teams = append(teams, &copied)
	}
	return teams, nil
}

func (s *Service) GetTeam(ctx context.Context, tenantID, teamID uuid.UUID) (*Team, error) {
	if tenantID == uuid.Nil || teamID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id and team_id are required", ErrInvalidInput)
	}
	record, err := s.repository.GetTeam(ctx, tenantID, teamID)
	if err != nil {
		return nil, fmt.Errorf("get team: %w", err)
	}
	return &record, nil
}

func (s *Service) GetCurrentConfigRevision(ctx context.Context, tenantID, teamID uuid.UUID) (*TeamConfigRevision, error) {
	if tenantID == uuid.Nil || teamID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id and team_id are required", ErrInvalidInput)
	}
	record, err := s.repository.GetCurrentTeamConfigRevision(ctx, tenantID, teamID)
	if err != nil {
		return nil, fmt.Errorf("get current team config revision: %w", err)
	}
	return &record, nil
}
```

- [ ] **Step 7: Implement pg repository**

Create `apps/control-plane/internal/tenant/pg_repository.go` with sqlc mappings. Use `json.Marshal` for maps, `json.Unmarshal` for response JSONB, and mirror the helper style from `apps/control-plane/internal/employee/pg_repository.go`.

The core method signatures must be:

```go
func NewPgRepository(q *queries.Queries) *PgRepository
func (r *PgRepository) CreateTeam(ctx context.Context, params CreateTeamParams) (TeamRecord, error)
func (r *PgRepository) ListTeams(ctx context.Context, params ListTeamsParams) ([]TeamRecord, error)
func (r *PgRepository) GetTeam(ctx context.Context, tenantID, teamID uuid.UUID) (TeamRecord, error)
func (r *PgRepository) CreateTeamConfigRevision(ctx context.Context, params CreateTeamConfigRevisionParams) (TeamConfigRevisionRecord, error)
func (r *PgRepository) GetTeamConfigRevision(ctx context.Context, tenantID, revisionID uuid.UUID) (TeamConfigRevisionRecord, error)
func (r *PgRepository) GetCurrentTeamConfigRevision(ctx context.Context, tenantID, teamID uuid.UUID) (TeamConfigRevisionRecord, error)
func (r *PgRepository) GetNextTeamConfigRevisionNumber(ctx context.Context, tenantID, teamID uuid.UUID) (int32, error)
```

- [ ] **Step 8: Add HTTP handler**

Create `apps/control-plane/internal/tenant/handler.go`. It must use `middleware.ConsoleUserAuth` context values and the same `authz.ActionRuntimeScopeManage` action used by digital employee routes until a dedicated team-management action exists.

Handler methods:

```go
func NewHandler(service HandlerService) *HTTPHandler
func (h *HTTPHandler) SetAuthorizer(authorizer authz.Authorizer)
func (h *HTTPHandler) ListTeams(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) CreateTeam(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) GetTeam(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) CreateTeamConfigRevision(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) GetCurrentTeamConfigRevision(w http.ResponseWriter, r *http.Request)
```

- [ ] **Step 9: Wire app and server**

Modify `apps/control-plane/internal/app/app.go`:

```go
TenantService *tenant.Service
TenantHandler *tenant.HTTPHandler
```

In `NewContainer`, create:

```go
tenantRepository := tenant.NewPgRepository(q)
tenantService, err := tenant.NewService(tenantRepository)
if err != nil {
	return nil, err
}
tenantHandler := tenant.NewHandler(tenantService)
```

Call:

```go
server.SetTenantHandler(tenantHandler)
```

Modify `apps/control-plane/internal/api/server.go`:

```go
tenantHandler *tenant.HTTPHandler
```

Add:

```go
func (s *Server) SetTenantHandler(tenantHandler *tenant.HTTPHandler) {
	s.tenantHandler = tenantHandler
	if tenantHandler != nil {
		tenantHandler.SetAuthorizer(s.authorizer)
	}
	s.registerRoutes()
}
```

Register routes inside `/api/v1` with console auth:

```go
if s.tenantHandler != nil {
	r.Group(func(r chi.Router) {
		r.Use(middleware.ConsoleUserAuth(s.authService))
		r.Get("/teams", s.tenantHandler.ListTeams)
		r.Post("/teams", s.tenantHandler.CreateTeam)
		r.Get("/teams/{teamId}", s.tenantHandler.GetTeam)
		r.Post("/teams/{teamId}/config-revisions", s.tenantHandler.CreateTeamConfigRevision)
		r.Get("/teams/{teamId}/config-revisions/current", s.tenantHandler.GetCurrentTeamConfigRevision)
	})
}
```

- [ ] **Step 10: Run tenant backend tests**

Run:

```bash
go test ./apps/control-plane/internal/tenant -count=1
go test ./apps/control-plane/internal/api -run 'TestTeamRoutes|TestDigitalEmployee' -count=1
```

Expected: PASS after route tests are added.

- [ ] **Step 11: Commit team backend**

```bash
git add apps/control-plane/internal/tenant \
  apps/control-plane/internal/app/app.go \
  apps/control-plane/internal/api/server.go \
  apps/control-plane/internal/api/routes_test.go
git commit -m "feat: add team governance service"
```

### Task 3: Extend Digital Employee Service with Config Revisions and Effective Config Validation

**Files:**
- Modify: `apps/control-plane/internal/employee/types.go`
- Modify: `apps/control-plane/internal/employee/repository.go`
- Modify: `apps/control-plane/internal/employee/pg_repository.go`
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/control-plane/internal/employee/service_test.go`
- Modify: `apps/control-plane/internal/employee/handler.go`
- Modify: `apps/control-plane/internal/api/employee_routes_test.go`

- [ ] **Step 1: Write failing validation tests**

First update `TestCreateDraftDigitalEmployeeDefaultsAndTrims` in `apps/control-plane/internal/employee/service_test.go` to pass a team ID:

```go
teamID := uuid.New()
created, err := svc.CreateDraft(context.Background(), CreateDraftRequest{
	TenantID: tenantID,
	TeamID:   &teamID,
	Name:     "  Finance reviewer  ",
	Role:     "  finance_reviewer  ",
})
```

Then add this case to the existing `TestServiceValidation` validation table:

```go
{
	name: "create requires team",
	run: func() error {
		_, err := svc.CreateDraft(context.Background(), CreateDraftRequest{TenantID: tenantID, Name: "employee", Role: "reviewer"})
		return err
	},
},
```

Append these new preview validation tests to the same file:

```go
func TestPreviewEffectiveConfigBlocksCapabilityOutsideTeamAllowlist(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamRevisionID := uuid.New()
	employeeRevisionID := uuid.New()

	preview, err := svc.PreviewEffectiveConfig(context.Background(), PreviewEffectiveConfigRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: uuid.New(),
		TeamConfig: TeamConfigInput{
			ID: teamRevisionID,
			CapabilityPolicy: map[string]any{
				"allowed_skills": []any{"incident-diagnosis"},
			},
		},
		EmployeeConfig: EmployeeConfigInput{
			ID: employeeRevisionID,
			CapabilitySelection: map[string]any{
				"enabled_skills": []any{"database-troubleshooting"},
			},
		},
	})
	if err != nil {
		t.Fatalf("preview effective config: %v", err)
	}
	if len(preview.Validation.BlockingErrors) != 1 {
		t.Fatalf("expected one blocking error, got %#v", preview.Validation.BlockingErrors)
	}
	if preview.Validation.BlockingErrors[0].Code != "capability_outside_team_allowlist" {
		t.Fatalf("unexpected blocking code: %#v", preview.Validation.BlockingErrors)
	}
}

func TestPreviewEffectiveConfigBlocksContextOutsideTeamScope(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	preview, err := svc.PreviewEffectiveConfig(context.Background(), PreviewEffectiveConfigRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: uuid.New(),
		TeamConfig: TeamConfigInput{
			ID: uuid.New(),
			ContextPolicy: map[string]any{
				"sources": []any{"monitoring", "logs"},
			},
		},
		EmployeeConfig: EmployeeConfigInput{
			ID: uuid.New(),
			ContextPolicyOverride: map[string]any{
				"sources": []any{"monitoring", "customer_profile"},
			},
		},
	})
	if err != nil {
		t.Fatalf("preview effective config: %v", err)
	}
	if len(preview.Validation.BlockingErrors) != 1 {
		t.Fatalf("expected one blocking error, got %#v", preview.Validation.BlockingErrors)
	}
	if preview.Validation.BlockingErrors[0].Code != "context_outside_team_scope" {
		t.Fatalf("unexpected blocking code: %#v", preview.Validation.BlockingErrors)
	}
}

func TestPreviewEffectiveConfigBlocksApprovalPolicyDowngrade(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	preview, err := svc.PreviewEffectiveConfig(context.Background(), PreviewEffectiveConfigRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: uuid.New(),
		TeamConfig: TeamConfigInput{
			ID: uuid.New(),
			ApprovalPolicy: map[string]any{
				"min_risk_for_human":        "high",
				"write_actions_require_human": true,
			},
		},
		EmployeeConfig: EmployeeConfigInput{
			ID: uuid.New(),
			ApprovalPolicyOverride: map[string]any{
				"min_risk_for_human":        "critical",
				"write_actions_require_human": false,
			},
		},
	})
	if err != nil {
		t.Fatalf("preview effective config: %v", err)
	}
	if len(preview.Validation.BlockingErrors) != 2 {
		t.Fatalf("expected two blocking errors, got %#v", preview.Validation.BlockingErrors)
	}
}

func TestPreviewEffectiveConfigAllowsTeamInternalCollaborationPolicy(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	preview, err := svc.PreviewEffectiveConfig(context.Background(), PreviewEffectiveConfigRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: uuid.New(),
		TeamConfig: TeamConfigInput{
			ID: uuid.New(),
			InternalCollaborationPolicy: map[string]any{
				"allowed_request_types": []any{"info_request", "review_request", "artifact_request"},
				"max_auto_rounds":       float64(2),
			},
		},
		EmployeeConfig: EmployeeConfigInput{
			ID: uuid.New(),
			CapabilitySelection: map[string]any{
				"enabled_skills": []any{},
			},
		},
	})
	if err != nil {
		t.Fatalf("preview effective config: %v", err)
	}
	if len(preview.Validation.BlockingErrors) != 0 {
		t.Fatalf("expected no blocking errors, got %#v", preview.Validation.BlockingErrors)
	}
	if preview.EffectiveConfig["internal_collaboration_policy"] == nil {
		t.Fatalf("expected team internal collaboration policy in effective config")
	}
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestServiceValidation|TestPreviewEffectiveConfig' -count=1
```

Expected: FAIL because `team_id` is not required yet and preview types/methods do not exist.

- [ ] **Step 3: Add employee config domain types**

Add to `apps/control-plane/internal/employee/types.go`:

```go
type ConfigRevisionStatus string
type EffectiveConfigStatus string

const (
	ConfigRevisionStatusDraft ConfigRevisionStatus = "draft"

	EffectiveConfigStatusPendingApproval EffectiveConfigStatus = "pending_approval"
	EffectiveConfigStatusApproved        EffectiveConfigStatus = "approved"
	EffectiveConfigStatusRevoked         EffectiveConfigStatus = "revoked"
)

type ValidationIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field"`
}

type EffectiveConfigValidation struct {
	BlockingErrors []ValidationIssue `json:"blocking_errors"`
	Warnings       []ValidationIssue `json:"warnings"`
}

type TeamConfigInput struct {
	ID                          uuid.UUID
	Constitution                map[string]any
	CapabilityPolicy            map[string]any
	ContextPolicy               map[string]any
	ApprovalPolicy              map[string]any
	ArtifactContract            map[string]any
	InternalCollaborationPolicy map[string]any
	RuntimeScopePolicy          map[string]any
}

type EmployeeConfigInput struct {
	ID                       uuid.UUID
	RoleProfile              map[string]any
	ConstitutionAddendum     map[string]any
	CapabilitySelection      map[string]any
	ContextPolicyOverride    map[string]any
	ApprovalPolicyOverride   map[string]any
	OutputContractAddendum   map[string]any
}

type DigitalEmployeeConfigRevision struct {
	ID                     uuid.UUID
	TenantID               uuid.UUID
	DigitalEmployeeID      uuid.UUID
	RevisionNumber         int32
	RoleProfile            map[string]any
	ConstitutionAddendum   map[string]any
	CapabilitySelection    map[string]any
	ContextPolicyOverride  map[string]any
	ApprovalPolicyOverride map[string]any
	OutputContractAddendum map[string]any
	Status                 ConfigRevisionStatus
	ApprovedBy             *uuid.UUID
	ApprovedAt             *time.Time
	ArchivedAt             *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type CreateDigitalEmployeeConfigRevisionRequest struct {
	TenantID               uuid.UUID
	DigitalEmployeeID      uuid.UUID
	RoleProfile            map[string]any
	ConstitutionAddendum   map[string]any
	CapabilitySelection    map[string]any
	ContextPolicyOverride  map[string]any
	ApprovalPolicyOverride map[string]any
	OutputContractAddendum map[string]any
	Status                 ConfigRevisionStatus
	ApprovedBy             *uuid.UUID
}

type EffectiveConfigPreview struct {
	DigitalEmployeeID uuid.UUID
	TeamConfigID      uuid.UUID
	EmployeeConfigID  uuid.UUID
	EffectiveConfig   map[string]any
	Validation        EffectiveConfigValidation
}

type DigitalEmployeeEffectiveConfig struct {
	ID                         uuid.UUID
	TenantID                   uuid.UUID
	DigitalEmployeeID          uuid.UUID
	TenantTeamConfigRevisionID uuid.UUID
	EmployeeConfigRevisionID   uuid.UUID
	EffectiveConfigSnapshot    map[string]any
	ValidationResult           map[string]any
	Status                     EffectiveConfigStatus
	ApprovedBy                 *uuid.UUID
	ApprovedAt                 *time.Time
	RevokedAt                  *time.Time
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

type PreviewEffectiveConfigRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	TeamConfig        TeamConfigInput
	EmployeeConfig    EmployeeConfigInput
}

type PreviewEffectiveConfigByRevisionIDsRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	TeamConfigID      uuid.UUID
	EmployeeConfigID  uuid.UUID
}

type ApproveEffectiveConfigRequest struct {
	Preview    PreviewEffectiveConfigByRevisionIDsRequest
	ApprovedBy uuid.UUID
}
```

- [ ] **Step 4: Implement preview validation**

Before adding preview validation, update `CreateDraft` in `apps/control-plane/internal/employee/service.go` immediately after the tenant check:

```go
if req.TeamID == nil || *req.TeamID == uuid.Nil {
	return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
}
```

Then add to `apps/control-plane/internal/employee/service.go`:

```go
func (s *Service) PreviewEffectiveConfig(ctx context.Context, req PreviewEffectiveConfigRequest) (*EffectiveConfigPreview, error) {
	if req.TenantID == uuid.Nil || req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id and employee_id are required", ErrInvalidInput)
	}
	validation := EffectiveConfigValidation{}
	validation.BlockingErrors = append(validation.BlockingErrors, validateCapabilitySubset(req.TeamConfig.CapabilityPolicy, req.EmployeeConfig.CapabilitySelection)...)
	validation.BlockingErrors = append(validation.BlockingErrors, validateContextSubset(req.TeamConfig.ContextPolicy, req.EmployeeConfig.ContextPolicyOverride)...)
	validation.BlockingErrors = append(validation.BlockingErrors, validateApprovalOverride(req.TeamConfig.ApprovalPolicy, req.EmployeeConfig.ApprovalPolicyOverride)...)

	effective := map[string]any{
		"team_config_revision_id":     req.TeamConfig.ID.String(),
		"employee_config_revision_id": req.EmployeeConfig.ID.String(),
		"constitution": map[string]any{
			"team":     cloneMap(req.TeamConfig.Constitution),
			"addendum": cloneMap(req.EmployeeConfig.ConstitutionAddendum),
		},
		"capability_policy":              cloneMap(req.TeamConfig.CapabilityPolicy),
		"capability_selection":           cloneMap(req.EmployeeConfig.CapabilitySelection),
		"context_policy":                 cloneMap(req.TeamConfig.ContextPolicy),
		"context_policy_override":        cloneMap(req.EmployeeConfig.ContextPolicyOverride),
		"approval_policy":                cloneMap(req.TeamConfig.ApprovalPolicy),
		"approval_policy_override":       cloneMap(req.EmployeeConfig.ApprovalPolicyOverride),
		"artifact_contract":              cloneMap(req.TeamConfig.ArtifactContract),
		"output_contract_addendum":       cloneMap(req.EmployeeConfig.OutputContractAddendum),
		"internal_collaboration_policy":  cloneMap(req.TeamConfig.InternalCollaborationPolicy),
		"runtime_scope_policy":           cloneMap(req.TeamConfig.RuntimeScopePolicy),
	}
	return &EffectiveConfigPreview{
		DigitalEmployeeID: req.DigitalEmployeeID,
		TeamConfigID:      req.TeamConfig.ID,
		EmployeeConfigID:  req.EmployeeConfig.ID,
		EffectiveConfig:   effective,
		Validation:        validation,
	}, nil
}

func (s *Service) PreviewEffectiveConfigByRevisionIDs(ctx context.Context, req PreviewEffectiveConfigByRevisionIDsRequest) (*EffectiveConfigPreview, error) {
	if req.TenantID == uuid.Nil || req.DigitalEmployeeID == uuid.Nil || req.TeamConfigID == uuid.Nil || req.EmployeeConfigID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id, employee_id, team_config_id and employee_config_id are required", ErrInvalidInput)
	}
	teamConfig, err := s.repository.GetTeamConfigRevision(ctx, req.TenantID, req.TeamConfigID)
	if err != nil {
		return nil, fmt.Errorf("get team config revision: %w", err)
	}
	employeeConfig, err := s.repository.GetDigitalEmployeeConfigRevision(ctx, req.TenantID, req.DigitalEmployeeID, req.EmployeeConfigID)
	if err != nil {
		return nil, fmt.Errorf("get digital employee config revision: %w", err)
	}
	return s.PreviewEffectiveConfig(ctx, PreviewEffectiveConfigRequest{
		TenantID:          req.TenantID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		TeamConfig:        teamConfig,
		EmployeeConfig:    employeeConfig,
	})
}
```

Add helpers:

```go
func validateCapabilitySubset(teamPolicy, employeeSelection map[string]any) []ValidationIssue {
	var issues []ValidationIssue
	checks := []struct {
		allowedKey string
		enabledKey string
		field      string
	}{
		{"allowed_mcp_servers", "enabled_mcp_servers", "capability_selection.enabled_mcp_servers"},
		{"allowed_skills", "enabled_skills", "capability_selection.enabled_skills"},
		{"allowed_plugins", "enabled_plugins", "capability_selection.enabled_plugins"},
		{"allowed_external_capabilities", "enabled_external_capabilities", "capability_selection.enabled_external_capabilities"},
		{"allowed_provider_types", "enabled_provider_types", "capability_selection.enabled_provider_types"},
	}
	for _, check := range checks {
		allowed := stringSet(teamPolicy[check.allowedKey])
		for _, enabled := range stringList(employeeSelection[check.enabledKey]) {
			if !allowed[enabled] {
				issues = append(issues, ValidationIssue{
					Code:    "capability_outside_team_allowlist",
					Message: enabled + " is not allowed by the team capability policy",
					Field:   check.field,
				})
			}
		}
	}
	return issues
}

func validateContextSubset(teamPolicy, employeeOverride map[string]any) []ValidationIssue {
	var issues []ValidationIssue
	for _, key := range []string{"sources", "knowledge_bases", "documents", "repositories", "log_sources"} {
		allowed := stringSet(teamPolicy[key])
		for _, selected := range stringList(employeeOverride[key]) {
			if !allowed[selected] {
				issues = append(issues, ValidationIssue{
					Code:    "context_outside_team_scope",
					Message: selected + " is not allowed by the team context policy",
					Field:   "context_policy_override." + key,
				})
			}
		}
	}
	return issues
}

func validateApprovalOverride(teamPolicy, employeeOverride map[string]any) []ValidationIssue {
	var issues []ValidationIssue
	teamRisk, hasTeamRisk := riskRank(teamPolicy["min_risk_for_human"])
	employeeRisk, hasEmployeeRisk := riskRank(employeeOverride["min_risk_for_human"])
	if hasTeamRisk && hasEmployeeRisk && employeeRisk > teamRisk {
		issues = append(issues, ValidationIssue{
			Code:    "approval_policy_downgrade",
			Message: "employee min_risk_for_human lowers the team approval requirement",
			Field:   "approval_policy_override.min_risk_for_human",
		})
	}
	if teamPolicy["write_actions_require_human"] == true && employeeOverride["write_actions_require_human"] == false {
		issues = append(issues, ValidationIssue{
			Code:    "approval_policy_downgrade",
			Message: "employee override cannot disable team write action approval",
			Field:   "approval_policy_override.write_actions_require_human",
		})
	}
	return issues
}

func riskRank(value any) (int, bool) {
	switch value {
	case "low":
		return 1, true
	case "medium":
		return 2, true
	case "high":
		return 3, true
	case "critical":
		return 4, true
	default:
		return 0, false
	}
}

func stringSet(value any) map[string]bool {
	result := make(map[string]bool)
	for _, item := range stringList(value) {
		result[item] = true
	}
	return result
}

func stringList(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
			result = append(result, strings.TrimSpace(text))
		}
	}
	return result
}
```

Use the existing `cloneMap` helper already present in `apps/control-plane/internal/employee/service.go`.

- [ ] **Step 5: Add repository methods for persistence**

Extend `apps/control-plane/internal/employee/repository.go` with:

```go
CreateDigitalEmployeeConfigRevision(ctx context.Context, params CreateConfigRevisionParams) (DigitalEmployeeConfigRevisionRecord, error)
GetTeamConfigRevision(ctx context.Context, tenantID, teamConfigRevisionID uuid.UUID) (TeamConfigInput, error)
GetDigitalEmployeeConfigRevision(ctx context.Context, tenantID, digitalEmployeeID, employeeConfigRevisionID uuid.UUID) (EmployeeConfigInput, error)
GetNextDigitalEmployeeConfigRevisionNumber(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (int32, error)
CreateDigitalEmployeeEffectiveConfig(ctx context.Context, params CreateEffectiveConfigParams) (DigitalEmployeeEffectiveConfigRecord, error)
```

Add these structs in the same file:

```go
type CreateConfigRevisionParams struct {
	TenantID               uuid.UUID
	DigitalEmployeeID      uuid.UUID
	RevisionNumber         int32
	RoleProfile            map[string]any
	ConstitutionAddendum   map[string]any
	CapabilitySelection    map[string]any
	ContextPolicyOverride  map[string]any
	ApprovalPolicyOverride map[string]any
	OutputContractAddendum map[string]any
	Status                 ConfigRevisionStatus
	ApprovedBy             *uuid.UUID
	ApprovedAt             *time.Time
}

type DigitalEmployeeConfigRevisionRecord struct {
	ID                     uuid.UUID
	TenantID               uuid.UUID
	DigitalEmployeeID      uuid.UUID
	RevisionNumber         int32
	RoleProfile            map[string]any
	ConstitutionAddendum   map[string]any
	CapabilitySelection    map[string]any
	ContextPolicyOverride  map[string]any
	ApprovalPolicyOverride map[string]any
	OutputContractAddendum map[string]any
	Status                 ConfigRevisionStatus
	ApprovedBy             *uuid.UUID
	ApprovedAt             *time.Time
	ArchivedAt             *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type CreateEffectiveConfigParams struct {
	TenantID                   uuid.UUID
	DigitalEmployeeID          uuid.UUID
	TenantTeamConfigRevisionID uuid.UUID
	EmployeeConfigRevisionID   uuid.UUID
	EffectiveConfigSnapshot    map[string]any
	ValidationResult           map[string]any
	Status                     EffectiveConfigStatus
	ApprovedBy                 *uuid.UUID
	ApprovedAt                 *time.Time
}

type DigitalEmployeeEffectiveConfigRecord struct {
	ID                         uuid.UUID
	TenantID                   uuid.UUID
	DigitalEmployeeID          uuid.UUID
	TenantTeamConfigRevisionID uuid.UUID
	EmployeeConfigRevisionID   uuid.UUID
	EffectiveConfigSnapshot    map[string]any
	ValidationResult           map[string]any
	Status                     EffectiveConfigStatus
	ApprovedBy                 *uuid.UUID
	ApprovedAt                 *time.Time
	RevokedAt                  *time.Time
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}
```

- [ ] **Step 6: Implement config revision creation and `ApproveEffectiveConfig`**

Add to `apps/control-plane/internal/employee/service.go`:

```go
func (s *Service) CreateConfigRevision(ctx context.Context, req CreateDigitalEmployeeConfigRevisionRequest) (*DigitalEmployeeConfigRevision, error) {
	if req.TenantID == uuid.Nil || req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id and employee_id are required", ErrInvalidInput)
	}
	status := req.Status
	if status == "" {
		status = ConfigRevisionStatusDraft
	}
	next, err := s.repository.GetNextDigitalEmployeeConfigRevisionNumber(ctx, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return nil, fmt.Errorf("get next digital employee config revision number: %w", err)
	}
	var approvedAt *time.Time
	if req.ApprovedBy != nil {
		now := time.Now().UTC()
		approvedAt = &now
	}
	record, err := s.repository.CreateDigitalEmployeeConfigRevision(ctx, CreateConfigRevisionParams{
		TenantID:               req.TenantID,
		DigitalEmployeeID:      req.DigitalEmployeeID,
		RevisionNumber:         next,
		RoleProfile:            cloneMap(req.RoleProfile),
		ConstitutionAddendum:   cloneMap(req.ConstitutionAddendum),
		CapabilitySelection:    cloneMap(req.CapabilitySelection),
		ContextPolicyOverride:  cloneMap(req.ContextPolicyOverride),
		ApprovalPolicyOverride: cloneMap(req.ApprovalPolicyOverride),
		OutputContractAddendum: cloneMap(req.OutputContractAddendum),
		Status:                 status,
		ApprovedBy:             req.ApprovedBy,
		ApprovedAt:             approvedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("create digital employee config revision: %w", err)
	}
	return configRevisionFromRecord(record), nil
}

func configRevisionFromRecord(record DigitalEmployeeConfigRevisionRecord) *DigitalEmployeeConfigRevision {
	return &DigitalEmployeeConfigRevision{
		ID:                     record.ID,
		TenantID:               record.TenantID,
		DigitalEmployeeID:      record.DigitalEmployeeID,
		RevisionNumber:         record.RevisionNumber,
		RoleProfile:            cloneMap(record.RoleProfile),
		ConstitutionAddendum:   cloneMap(record.ConstitutionAddendum),
		CapabilitySelection:    cloneMap(record.CapabilitySelection),
		ContextPolicyOverride:  cloneMap(record.ContextPolicyOverride),
		ApprovalPolicyOverride: cloneMap(record.ApprovalPolicyOverride),
		OutputContractAddendum: cloneMap(record.OutputContractAddendum),
		Status:                 record.Status,
		ApprovedBy:             record.ApprovedBy,
		ApprovedAt:             record.ApprovedAt,
		ArchivedAt:             record.ArchivedAt,
		CreatedAt:              record.CreatedAt,
		UpdatedAt:              record.UpdatedAt,
	}
}
```

Then add `ApproveEffectiveConfig` to the same file:


```go
func (s *Service) ApproveEffectiveConfig(ctx context.Context, req ApproveEffectiveConfigRequest) (*DigitalEmployeeEffectiveConfig, error) {
	preview, err := s.PreviewEffectiveConfigByRevisionIDs(ctx, req.Preview)
	if err != nil {
		return nil, err
	}
	if len(preview.Validation.BlockingErrors) > 0 {
		return nil, fmt.Errorf("%w: effective config has blocking errors", ErrInvalidInput)
	}
	if req.ApprovedBy == uuid.Nil {
		return nil, fmt.Errorf("%w: approved_by is required", ErrInvalidInput)
	}
	instance, err := s.repository.GetDigitalEmployeeExecutionInstanceByEmployeeID(ctx, req.Preview.TenantID, req.Preview.DigitalEmployeeID)
	if err != nil {
		return nil, fmt.Errorf("%w: active execution instance is required before approval", ErrInvalidInput)
	}
	if instance.Status != ExecutionInstanceStatusReady && instance.Status != ExecutionInstanceStatusActive {
		return nil, fmt.Errorf("%w: execution instance must be ready or active before approval", ErrInvalidInput)
	}
	approvedAt := time.Now().UTC()
	record, err := s.repository.CreateDigitalEmployeeEffectiveConfig(ctx, CreateEffectiveConfigParams{
		TenantID:                   req.Preview.TenantID,
		DigitalEmployeeID:          req.Preview.DigitalEmployeeID,
		TenantTeamConfigRevisionID: req.Preview.TeamConfigID,
		EmployeeConfigRevisionID:   req.Preview.EmployeeConfigID,
		EffectiveConfigSnapshot:    preview.EffectiveConfig,
		ValidationResult: map[string]any{
			"blocking_errors": preview.Validation.BlockingErrors,
			"warnings":        preview.Validation.Warnings,
		},
		Status:     EffectiveConfigStatusApproved,
		ApprovedBy: &req.ApprovedBy,
		ApprovedAt: &approvedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("create effective config: %w", err)
	}
	return effectiveConfigFromRecord(record), nil
}

func effectiveConfigFromRecord(record DigitalEmployeeEffectiveConfigRecord) *DigitalEmployeeEffectiveConfig {
	return &DigitalEmployeeEffectiveConfig{
		ID:                         record.ID,
		TenantID:                   record.TenantID,
		DigitalEmployeeID:          record.DigitalEmployeeID,
		TenantTeamConfigRevisionID: record.TenantTeamConfigRevisionID,
		EmployeeConfigRevisionID:   record.EmployeeConfigRevisionID,
		EffectiveConfigSnapshot:    cloneMap(record.EffectiveConfigSnapshot),
		ValidationResult:           cloneMap(record.ValidationResult),
		Status:                     record.Status,
		ApprovedBy:                 record.ApprovedBy,
		ApprovedAt:                 record.ApprovedAt,
		RevokedAt:                  record.RevokedAt,
		CreatedAt:                  record.CreatedAt,
		UpdatedAt:                  record.UpdatedAt,
	}
}
```

- [ ] **Step 7: Add HTTP routes for preview and approve**

Extend `HandlerService` in `apps/control-plane/internal/employee/handler.go`:

```go
CreateConfigRevision(ctx context.Context, req CreateDigitalEmployeeConfigRevisionRequest) (*DigitalEmployeeConfigRevision, error)
PreviewEffectiveConfigByRevisionIDs(ctx context.Context, req PreviewEffectiveConfigByRevisionIDsRequest) (*EffectiveConfigPreview, error)
ApproveEffectiveConfig(ctx context.Context, req ApproveEffectiveConfigRequest) (*DigitalEmployeeEffectiveConfig, error)
```

Add these handlers below `UpsertDigitalEmployeeExecutionInstance`:

```go
func (h *HTTPHandler) CreateDigitalEmployeeConfigRevision(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, "digital employee config revision create")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	var req struct {
		RoleProfile            map[string]any        `json:"role_profile"`
		ConstitutionAddendum   map[string]any        `json:"constitution_addendum"`
		CapabilitySelection    map[string]any        `json:"capability_selection"`
		ContextPolicyOverride  map[string]any        `json:"context_policy_override"`
		ApprovalPolicyOverride map[string]any        `json:"approval_policy_override"`
		OutputContractAddendum map[string]any        `json:"output_contract_addendum"`
		Status                 ConfigRevisionStatus  `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	revision, err := service.CreateConfigRevision(r.Context(), CreateDigitalEmployeeConfigRevisionRequest{
		TenantID:               tenantID,
		DigitalEmployeeID:      employeeID,
		RoleProfile:            req.RoleProfile,
		ConstitutionAddendum:   req.ConstitutionAddendum,
		CapabilitySelection:    req.CapabilitySelection,
		ContextPolicyOverride:  req.ContextPolicyOverride,
		ApprovalPolicyOverride: req.ApprovalPolicyOverride,
		OutputContractAddendum: req.OutputContractAddendum,
		Status:                 req.Status,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, configRevisionResponseFromDomain(revision))
}

func (h *HTTPHandler) PreviewDigitalEmployeeEffectiveConfig(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, "digital employee effective config preview")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	var req struct {
		TeamConfig     struct{ ID uuid.UUID `json:"id"` } `json:"team_config"`
		EmployeeConfig struct{ ID uuid.UUID `json:"id"` } `json:"employee_config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	preview, err := service.PreviewEffectiveConfigByRevisionIDs(r.Context(), PreviewEffectiveConfigByRevisionIDsRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		TeamConfigID:      req.TeamConfig.ID,
		EmployeeConfigID:  req.EmployeeConfig.ID,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, effectiveConfigPreviewResponseFromDomain(preview))
}

func (h *HTTPHandler) ApproveDigitalEmployeeEffectiveConfig(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, "digital employee effective config approve")
	if !ok {
		return
	}
	userID := middleware.GetUserID(r.Context())
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	var req struct {
		Preview struct {
			TeamConfig     struct{ ID uuid.UUID `json:"id"` } `json:"team_config"`
			EmployeeConfig struct{ ID uuid.UUID `json:"id"` } `json:"employee_config"`
		} `json:"preview"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	effective, err := service.ApproveEffectiveConfig(r.Context(), ApproveEffectiveConfigRequest{
		ApprovedBy: userID,
		Preview: PreviewEffectiveConfigByRevisionIDsRequest{
			TenantID:          tenantID,
			DigitalEmployeeID: employeeID,
			TeamConfigID:      req.Preview.TeamConfig.ID,
			EmployeeConfigID:  req.Preview.EmployeeConfig.ID,
		},
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, effectiveConfigResponseFromDomain(effective))
}
```

Add these response helpers near the existing employee response helpers:

```go
func configRevisionResponseFromDomain(revision *DigitalEmployeeConfigRevision) map[string]any {
	return map[string]any{
		"id":                       revision.ID,
		"tenant_id":                revision.TenantID,
		"digital_employee_id":      revision.DigitalEmployeeID,
		"revision_number":          revision.RevisionNumber,
		"role_profile":             revision.RoleProfile,
		"constitution_addendum":    revision.ConstitutionAddendum,
		"capability_selection":     revision.CapabilitySelection,
		"context_policy_override":  revision.ContextPolicyOverride,
		"approval_policy_override": revision.ApprovalPolicyOverride,
		"output_contract_addendum": revision.OutputContractAddendum,
		"status":                   revision.Status,
	}
}

func effectiveConfigPreviewResponseFromDomain(preview *EffectiveConfigPreview) map[string]any {
	return map[string]any{
		"digital_employee_id": preview.DigitalEmployeeID,
		"effective_config":    preview.EffectiveConfig,
		"validation": map[string]any{
			"blocking_errors": preview.Validation.BlockingErrors,
			"warnings":        preview.Validation.Warnings,
		},
	}
}

func effectiveConfigResponseFromDomain(config *DigitalEmployeeEffectiveConfig) map[string]any {
	return map[string]any{
		"id":                             config.ID,
		"tenant_id":                      config.TenantID,
		"digital_employee_id":            config.DigitalEmployeeID,
		"tenant_team_config_revision_id": config.TenantTeamConfigRevisionID,
		"employee_config_revision_id":    config.EmployeeConfigRevisionID,
		"effective_config_snapshot":      config.EffectiveConfigSnapshot,
		"validation_result":              config.ValidationResult,
		"status":                         config.Status,
		"approved_by":                    config.ApprovedBy,
		"approved_at":                    config.ApprovedAt,
	}
}
```

In `apps/control-plane/internal/api/employee_routes_test.go`, extend `routeEmployeeService` with these fields and methods so the widened `HandlerService` compiles and route tests can assert request mapping:

```go
configRevisionReq employee.CreateDigitalEmployeeConfigRevisionRequest
previewReq        employee.PreviewEffectiveConfigByRevisionIDsRequest
approveReq        employee.ApproveEffectiveConfigRequest
configCalled      bool
previewCalled     bool
approveCalled     bool
```

```go
func (s *routeEmployeeService) CreateConfigRevision(ctx context.Context, req employee.CreateDigitalEmployeeConfigRevisionRequest) (*employee.DigitalEmployeeConfigRevision, error) {
	s.configCalled = true
	s.configRevisionReq = req
	now := time.Now().UTC()
	return &employee.DigitalEmployeeConfigRevision{
		ID:                uuid.New(),
		TenantID:          req.TenantID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		RevisionNumber:    1,
		RoleProfile:       req.RoleProfile,
		CapabilitySelection: req.CapabilitySelection,
		Status:            employee.ConfigRevisionStatusDraft,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

func (s *routeEmployeeService) PreviewEffectiveConfigByRevisionIDs(ctx context.Context, req employee.PreviewEffectiveConfigByRevisionIDsRequest) (*employee.EffectiveConfigPreview, error) {
	s.previewCalled = true
	s.previewReq = req
	return &employee.EffectiveConfigPreview{
		DigitalEmployeeID: req.DigitalEmployeeID,
		TeamConfigID:      req.TeamConfigID,
		EmployeeConfigID:  req.EmployeeConfigID,
		EffectiveConfig:   map[string]any{},
		Validation:        employee.EffectiveConfigValidation{BlockingErrors: []employee.ValidationIssue{}, Warnings: []employee.ValidationIssue{}},
	}, nil
}

func (s *routeEmployeeService) ApproveEffectiveConfig(ctx context.Context, req employee.ApproveEffectiveConfigRequest) (*employee.DigitalEmployeeEffectiveConfig, error) {
	s.approveCalled = true
	s.approveReq = req
	now := time.Now().UTC()
	return &employee.DigitalEmployeeEffectiveConfig{
		ID:                         uuid.New(),
		TenantID:                   req.Preview.TenantID,
		DigitalEmployeeID:          req.Preview.DigitalEmployeeID,
		TenantTeamConfigRevisionID: req.Preview.TeamConfigID,
		EmployeeConfigRevisionID:   req.Preview.EmployeeConfigID,
		EffectiveConfigSnapshot:    map[string]any{},
		ValidationResult:           map[string]any{"blocking_errors": []any{}, "warnings": []any{}},
		Status:                     employee.EffectiveConfigStatusApproved,
		ApprovedBy:                 &req.ApprovedBy,
		ApprovedAt:                 &now,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	}, nil
}
```

Update the existing create request bodies in `TestDigitalEmployeeRoutesUseConsoleTenant` and `TestDigitalEmployeeRoutesRequireManagementAuthorization` to include `team_id`:

```go
teamID := uuid.New()
createReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees", strings.NewReader(`{"team_id":"`+teamID.String()+`","name":"Requirements analyst","role":"requirements_analyst"}`))
```

Add these new forbidden-route cases to `TestDigitalEmployeeRoutesRequireManagementAuthorization`:

```go
teamConfigID := uuid.New().String()
employeeConfigID := uuid.New().String()
tests = append(tests,
	{name: "create config revision", method: http.MethodPost, path: "/api/v1/digital-employees/" + employeeID + "/config-revisions", body: `{"role_profile":{},"capability_selection":{}}`},
	{name: "preview effective config", method: http.MethodPost, path: "/api/v1/digital-employees/" + employeeID + "/effective-configs/preview", body: `{"team_config":{"id":"` + teamConfigID + `"},"employee_config":{"id":"` + employeeConfigID + `"}}`},
	{name: "approve effective config", method: http.MethodPost, path: "/api/v1/digital-employees/" + employeeID + "/effective-configs/approve", body: `{"preview":{"team_config":{"id":"` + teamConfigID + `"},"employee_config":{"id":"` + employeeConfigID + `"}}}`},
)
```

Update `routeEmployeeService.called()` to include `configCalled`, `previewCalled`, and `approveCalled`.

In `TestDigitalEmployeeRoutesUseConsoleTenant`, after the execution-instance assertion, post the three governance requests and assert request mapping:

```go
teamConfigID := uuid.New()
employeeConfigID := uuid.New()

configReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees/"+created.ID+"/config-revisions", strings.NewReader(`{"role_profile":{"specialty":"postgres"},"capability_selection":{"enabled_skills":[]}}`))
configReq.Header.Set("Content-Type", "application/json")
configReq.AddCookie(cookie)
configResp := httptest.NewRecorder()
server.ServeHTTP(configResp, configReq)
if configResp.Code != http.StatusCreated {
	t.Fatalf("expected config revision create to succeed, got %d: %s", configResp.Code, configResp.Body.String())
}
if service.configRevisionReq.TenantID != expectedTenantID || service.configRevisionReq.DigitalEmployeeID != service.createdID {
	t.Fatalf("expected config revision tenant/employee %s/%s, got %s/%s", expectedTenantID, service.createdID, service.configRevisionReq.TenantID, service.configRevisionReq.DigitalEmployeeID)
}

previewReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees/"+created.ID+"/effective-configs/preview", strings.NewReader(`{"team_config":{"id":"`+teamConfigID.String()+`"},"employee_config":{"id":"`+employeeConfigID.String()+`"}}`))
previewReq.Header.Set("Content-Type", "application/json")
previewReq.AddCookie(cookie)
previewResp := httptest.NewRecorder()
server.ServeHTTP(previewResp, previewReq)
if previewResp.Code != http.StatusOK {
	t.Fatalf("expected effective config preview to succeed, got %d: %s", previewResp.Code, previewResp.Body.String())
}
if service.previewReq.TeamConfigID != teamConfigID || service.previewReq.EmployeeConfigID != employeeConfigID {
	t.Fatalf("expected preview config ids %s/%s, got %s/%s", teamConfigID, employeeConfigID, service.previewReq.TeamConfigID, service.previewReq.EmployeeConfigID)
}

approveReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees/"+created.ID+"/effective-configs/approve", strings.NewReader(`{"preview":{"team_config":{"id":"`+teamConfigID.String()+`"},"employee_config":{"id":"`+employeeConfigID.String()+`"}}}`))
approveReq.Header.Set("Content-Type", "application/json")
approveReq.AddCookie(cookie)
approveResp := httptest.NewRecorder()
server.ServeHTTP(approveResp, approveReq)
if approveResp.Code != http.StatusOK {
	t.Fatalf("expected effective config approve to succeed, got %d: %s", approveResp.Code, approveResp.Body.String())
}
if service.approveReq.Preview.TeamConfigID != teamConfigID || service.approveReq.Preview.EmployeeConfigID != employeeConfigID {
	t.Fatalf("expected approve config ids %s/%s, got %s/%s", teamConfigID, employeeConfigID, service.approveReq.Preview.TeamConfigID, service.approveReq.Preview.EmployeeConfigID)
}
```

Register in `apps/control-plane/internal/api/server.go`:

```go
r.Post("/digital-employees/{employeeId}/config-revisions", s.employeeHandler.CreateDigitalEmployeeConfigRevision)
r.Post("/digital-employees/{employeeId}/effective-configs/preview", s.employeeHandler.PreviewDigitalEmployeeEffectiveConfig)
r.Post("/digital-employees/{employeeId}/effective-configs/approve", s.employeeHandler.ApproveDigitalEmployeeEffectiveConfig)
```

- [ ] **Step 8: Run employee tests**

Run:

```bash
go test ./apps/control-plane/internal/employee -count=1
go test ./apps/control-plane/internal/api -run 'TestDigitalEmployeeRoutes' -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit employee governance backend**

```bash
git add apps/control-plane/internal/employee \
  apps/control-plane/internal/api/server.go \
  apps/control-plane/internal/api/employee_routes_test.go
git commit -m "feat: validate digital employee effective config"
```

### Task 4: Update OpenAPI Contract and Contract Verification

**Files:**
- Modify: `contracts/control-plane/openapi.yaml`
- Modify: `scripts/verify-foundation-contracts.mjs` only if it explicitly enumerates route paths.
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add contract route assertions if needed**

Inspect:

```bash
rg -n "digital-employees|runtime|tasks|teams" scripts/verify-foundation-contracts.mjs contracts/control-plane/openapi.yaml
```

If `scripts/verify-foundation-contracts.mjs` enumerates canonical paths, add:

```js
"/api/v1/teams",
"/api/v1/teams/{teamId}",
"/api/v1/teams/{teamId}/config-revisions",
"/api/v1/teams/{teamId}/config-revisions/current",
"/api/v1/digital-employees/{employeeId}/config-revisions",
"/api/v1/digital-employees/{employeeId}/effective-configs/preview",
"/api/v1/digital-employees/{employeeId}/effective-configs/approve",
```

- [ ] **Step 2: Extend OpenAPI paths**

Add to `contracts/control-plane/openapi.yaml`:

```yaml
  /api/v1/teams:
    get:
      summary: List tenant teams
      operationId: listTeams
      responses:
        "200":
          description: Team list
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: "#/components/schemas/Team"
    post:
      summary: Create a tenant team
      operationId: createTeam
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/CreateTeamRequest"
      responses:
        "201":
          description: Team created
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Team"
```

Add these path blocks immediately after `/api/v1/teams`:

```yaml
  /api/v1/teams/{teamId}:
    get:
      summary: Get a tenant team
      operationId: getTeam
      parameters:
        - name: teamId
          in: path
          required: true
          schema:
            type: string
            format: uuid
      responses:
        "200":
          description: Team detail
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Team"
        "404":
          description: Team not found
  /api/v1/teams/{teamId}/config-revisions:
    post:
      summary: Create a team config revision
      operationId: createTeamConfigRevision
      parameters:
        - name: teamId
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
              $ref: "#/components/schemas/CreateTeamConfigRevisionRequest"
      responses:
        "201":
          description: Team config revision created
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/TeamConfigRevision"
  /api/v1/teams/{teamId}/config-revisions/current:
    get:
      summary: Get the current active team config revision
      operationId: getCurrentTeamConfigRevision
      parameters:
        - name: teamId
          in: path
          required: true
          schema:
            type: string
            format: uuid
      responses:
        "200":
          description: Current active team config revision
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/TeamConfigRevision"
        "404":
          description: Active team config revision not found
  /api/v1/digital-employees/{employeeId}/config-revisions:
    post:
      summary: Create a digital employee personal config revision
      operationId: createDigitalEmployeeConfigRevision
      parameters:
        - name: employeeId
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
              $ref: "#/components/schemas/CreateDigitalEmployeeConfigRevisionRequest"
      responses:
        "201":
          description: Digital employee config revision created
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/DigitalEmployeeConfigRevision"
  /api/v1/digital-employees/{employeeId}/effective-configs/preview:
    post:
      summary: Preview digital employee effective config
      operationId: previewDigitalEmployeeEffectiveConfig
      parameters:
        - name: employeeId
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
              $ref: "#/components/schemas/EffectiveConfigPreviewRequest"
      responses:
        "200":
          description: Effective config preview
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/EffectiveConfigPreview"
  /api/v1/digital-employees/{employeeId}/effective-configs/approve:
    post:
      summary: Approve digital employee effective config
      operationId: approveDigitalEmployeeEffectiveConfig
      parameters:
        - name: employeeId
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
              $ref: "#/components/schemas/ApproveEffectiveConfigRequest"
      responses:
        "200":
          description: Effective config approved
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/DigitalEmployeeEffectiveConfig"
```

- [ ] **Step 3: Add schemas**

Add schemas:

```yaml
    Team:
      type: object
      required:
        - id
        - tenant_id
        - slug
        - name
        - status
      properties:
        id:
          type: string
          format: uuid
        tenant_id:
          type: string
          format: uuid
        slug:
          type: string
        name:
          type: string
        status:
          type: string
        human_owner_user_id:
          type: string
          format: uuid
        metadata:
          type: object
          additionalProperties: true
    TeamConfigRevision:
      type: object
      required:
        - id
        - tenant_id
        - team_id
        - revision_number
        - constitution
        - capability_policy
        - context_policy
        - approval_policy
        - artifact_contract
        - internal_collaboration_policy
        - runtime_scope_policy
        - status
      properties:
        id:
          type: string
          format: uuid
        tenant_id:
          type: string
          format: uuid
        team_id:
          type: string
          format: uuid
        revision_number:
          type: integer
          format: int32
        constitution:
          type: object
          additionalProperties: true
        capability_policy:
          type: object
          additionalProperties: true
        context_policy:
          type: object
          additionalProperties: true
        approval_policy:
          type: object
          additionalProperties: true
        artifact_contract:
          type: object
          additionalProperties: true
        internal_collaboration_policy:
          type: object
          additionalProperties: true
        runtime_scope_policy:
          type: object
          additionalProperties: true
        human_owner_user_id:
          type: string
          format: uuid
        status:
          type: string
```

Replace the existing `CreateDigitalEmployeeRequest` schema with this version, then continue the same `components.schemas` block with the new governance schemas:

```yaml
    CreateDigitalEmployeeRequest:
      type: object
      required:
        - team_id
        - name
        - role
      properties:
        team_id:
          type: string
          format: uuid
        name:
          type: string
        role:
          type: string
        description:
          type: string
        permission_policy:
          type: object
          additionalProperties: true
        context_policy:
          type: object
          additionalProperties: true
        approval_policy:
          type: object
          additionalProperties: true
        risk_level:
          type: string
        metadata:
          type: object
          additionalProperties: true
    CreateTeamRequest:
      type: object
      required:
        - slug
        - name
      properties:
        slug:
          type: string
        name:
          type: string
        human_owner_user_id:
          type: string
          format: uuid
        metadata:
          type: object
          additionalProperties: true
    CreateTeamConfigRevisionRequest:
      type: object
      properties:
        constitution:
          type: object
          additionalProperties: true
        capability_policy:
          type: object
          additionalProperties: true
        context_policy:
          type: object
          additionalProperties: true
        approval_policy:
          type: object
          additionalProperties: true
        artifact_contract:
          type: object
          additionalProperties: true
        internal_collaboration_policy:
          type: object
          additionalProperties: true
        runtime_scope_policy:
          type: object
          additionalProperties: true
        human_owner_user_id:
          type: string
          format: uuid
        status:
          type: string
    DigitalEmployeeConfigRevision:
      type: object
      required:
        - id
        - tenant_id
        - digital_employee_id
        - revision_number
        - role_profile
        - constitution_addendum
        - capability_selection
        - context_policy_override
        - approval_policy_override
        - output_contract_addendum
        - status
      properties:
        id:
          type: string
          format: uuid
        tenant_id:
          type: string
          format: uuid
        digital_employee_id:
          type: string
          format: uuid
        revision_number:
          type: integer
          format: int32
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
        status:
          type: string
    CreateDigitalEmployeeConfigRevisionRequest:
      type: object
      properties:
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
        status:
          type: string
    ConfigRevisionRef:
      type: object
      required:
        - id
      properties:
        id:
          type: string
          format: uuid
    EffectiveConfigPreviewRequest:
      type: object
      required:
        - team_config
        - employee_config
      properties:
        team_config:
          $ref: "#/components/schemas/ConfigRevisionRef"
        employee_config:
          $ref: "#/components/schemas/ConfigRevisionRef"
    EffectiveConfigValidationIssue:
      type: object
      required:
        - code
        - message
      properties:
        code:
          type: string
        message:
          type: string
        field:
          type: string
    EffectiveConfigValidation:
      type: object
      required:
        - blocking_errors
        - warnings
      properties:
        blocking_errors:
          type: array
          items:
            $ref: "#/components/schemas/EffectiveConfigValidationIssue"
        warnings:
          type: array
          items:
            $ref: "#/components/schemas/EffectiveConfigValidationIssue"
    EffectiveConfigPreview:
      type: object
      required:
        - digital_employee_id
        - effective_config
        - validation
      properties:
        digital_employee_id:
          type: string
          format: uuid
        effective_config:
          type: object
          additionalProperties: true
        validation:
          $ref: "#/components/schemas/EffectiveConfigValidation"
    ApproveEffectiveConfigRequest:
      type: object
      required:
        - preview
      properties:
        preview:
          $ref: "#/components/schemas/EffectiveConfigPreviewRequest"
    DigitalEmployeeEffectiveConfig:
      type: object
      required:
        - id
        - tenant_id
        - digital_employee_id
        - tenant_team_config_revision_id
        - employee_config_revision_id
        - effective_config_snapshot
        - validation_result
        - status
      properties:
        id:
          type: string
          format: uuid
        tenant_id:
          type: string
          format: uuid
        digital_employee_id:
          type: string
          format: uuid
        tenant_team_config_revision_id:
          type: string
          format: uuid
        employee_config_revision_id:
          type: string
          format: uuid
        effective_config_snapshot:
          type: object
          additionalProperties: true
        validation_result:
          $ref: "#/components/schemas/EffectiveConfigValidation"
        status:
          type: string
```

- [ ] **Step 4: Update changelog**

In `CHANGELOG.md` under `[Unreleased] -> Added`, add:

```markdown
#### 团队与数字员工治理设计落地 (2026-06-03)

- 新增团队治理配置与数字员工生效配置计划，团队作为无层级职能治理模板，数字员工继承团队 MCP、技能、插件、宪法、上下文、审批和内部协作边界。
```

- [ ] **Step 5: Run contract verification**

Run:

```bash
pnpm verify:contracts
```

Expected: PASS.

- [ ] **Step 6: Commit contracts**

```bash
git add contracts/control-plane/openapi.yaml scripts/verify-foundation-contracts.mjs CHANGELOG.md
git commit -m "docs: add team governance contract paths"
```

### Task 5: Add Web API Clients for Teams and Employee Effective Config

**Files:**
- Create: `apps/web/src/lib/api/teams.ts`
- Create: `apps/web/src/lib/api/teams.test.ts`
- Modify: `apps/web/src/lib/api/employees.ts`
- Modify: `apps/web/src/lib/api/employees.test.ts`
- Modify: `apps/web/src/lib/api/index.ts`

- [ ] **Step 1: Write failing teams API tests**

Create `apps/web/src/lib/api/teams.test.ts`:

```ts
import { describe, expect, it, vi } from "vitest";
import { createTeam, createTeamConfigRevision, getCurrentTeamConfigRevision, listTeams } from "./teams";

describe("team API", () => {
  it("lists teams with cookie credentials", async () => {
    const teams = [{ id: "team-1", tenant_id: "tenant-1", slug: "ops", name: "运维团队", status: "active" }];
    const fetcher = vi.fn(async () => new Response(JSON.stringify(teams), { headers: { "content-type": "application/json" }, status: 200 }));

    await expect(listTeams({ baseUrl: "http://control-plane.local", fetcher })).resolves.toEqual(teams);

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/teams", {
      credentials: "include",
      headers: { accept: "application/json" },
      method: "GET",
    });
  });

  it("creates team config revisions with policy JSON", async () => {
    const revision = { id: "rev-1", team_id: "team-1", revision_number: 1, status: "active" };
    const fetcher = vi.fn(async () => new Response(JSON.stringify(revision), { headers: { "content-type": "application/json" }, status: 201 }));

    await createTeamConfigRevision(
      { baseUrl: "http://control-plane.local", fetcher },
      "team 1/ops",
      {
        constitution: { hard_rules: ["禁止执行未审批的生产写操作"] },
        capability_policy: { allowed_skills: ["incident-diagnosis"] },
        internal_collaboration_policy: { max_auto_rounds: 2 },
      },
    );

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/teams/team%201%2Fops/config-revisions", expect.objectContaining({
      credentials: "include",
      method: "POST",
    }));
  });
});
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
pnpm --filter @superteam/web test -- teams.test.ts
```

Expected: FAIL because `./teams` does not exist.

- [ ] **Step 3: Add teams API client**

Create `apps/web/src/lib/api/teams.ts`:

```ts
import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type Team = {
  id: string;
  tenant_id: string;
  slug: string;
  name: string;
  status: string;
  human_owner_user_id?: string;
  metadata?: Record<string, unknown>;
};

export type TeamConfigRevision = {
  id: string;
  tenant_id: string;
  team_id: string;
  revision_number: number;
  constitution: Record<string, unknown>;
  capability_policy: Record<string, unknown>;
  context_policy: Record<string, unknown>;
  approval_policy: Record<string, unknown>;
  artifact_contract: Record<string, unknown>;
  internal_collaboration_policy: Record<string, unknown>;
  runtime_scope_policy: Record<string, unknown>;
  status: string;
};

export type CreateTeamInput = {
  slug: string;
  name: string;
  human_owner_user_id: string;
  metadata?: Record<string, unknown>;
};

export type CreateTeamConfigRevisionInput = Partial<
  Pick<
    TeamConfigRevision,
    | "constitution"
    | "capability_policy"
    | "context_policy"
    | "approval_policy"
    | "artifact_contract"
    | "internal_collaboration_policy"
    | "runtime_scope_policy"
  >
>;

export async function listTeams(options: ApiClientOptions): Promise<Team[]> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/v1/teams"), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "GET",
  });
  return parseJson<Team[]>(response, "teams");
}

export async function createTeam(options: ApiClientOptions, input: CreateTeamInput): Promise<Team> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/v1/teams"), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });
  return parseJson<Team>(response, "create team");
}

export async function createTeamConfigRevision(
  options: ApiClientOptions,
  teamId: string,
  input: CreateTeamConfigRevisionInput,
): Promise<TeamConfigRevision> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/teams/${encodeURIComponent(teamId)}/config-revisions`), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });
  return parseJson<TeamConfigRevision>(response, "create team config revision");
}

export async function getCurrentTeamConfigRevision(options: ApiClientOptions, teamId: string): Promise<TeamConfigRevision> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/teams/${encodeURIComponent(teamId)}/config-revisions/current`), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "GET",
  });
  return parseJson<TeamConfigRevision>(response, "current team config revision");
}
```

- [ ] **Step 4: Extend employee API client tests**

Ensure `apps/web/src/lib/api/employees.test.ts` imports `approveDigitalEmployeeEffectiveConfig`, `createDigitalEmployeeConfigRevision`, and `previewDigitalEmployeeEffectiveConfig` from `./employees`, then add:

```ts
// In the existing "creates digital employee with cookie credentials" test, pass team_id:
{
  name: "需求分析员工",
  role: "requirements_analyst",
  team_id: "team-1",
}

// In the same test, update the expected JSON body:
body: JSON.stringify({
  name: "需求分析员工",
  role: "requirements_analyst",
  team_id: "team-1",
}),

it("creates digital employee config revisions", async () => {
  const revision = { id: "employee-rev-1", digital_employee_id: "employee-1", revision_number: 1, status: "draft" };
  const fetcher = vi.fn(async () => new Response(JSON.stringify(revision), { headers: { "content-type": "application/json" }, status: 201 }));

  await expect(
    createDigitalEmployeeConfigRevision(
      { baseUrl: "http://control-plane.local", fetcher },
      "employee-1",
      {
        role_profile: { specialty: "postgres" },
        capability_selection: { enabled_skills: ["incident-diagnosis"] },
        status: "draft",
      },
    ),
  ).resolves.toEqual(revision);

  expect(fetcher).toHaveBeenCalledWith(
    "http://control-plane.local/api/v1/digital-employees/employee-1/config-revisions",
    expect.objectContaining({ credentials: "include", method: "POST" }),
  );
});

it("previews effective config and exposes blocking errors", async () => {
  const preview = {
    digital_employee_id: "employee-1",
    effective_config: {},
    validation: {
      blocking_errors: [{ code: "capability_outside_team_allowlist", message: "bad", field: "capability_selection.enabled_skills" }],
      warnings: [],
    },
  };
  const fetcher = vi.fn(async () => new Response(JSON.stringify(preview), { headers: { "content-type": "application/json" }, status: 200 }));

  await expect(
    previewDigitalEmployeeEffectiveConfig(
      { baseUrl: "http://control-plane.local", fetcher },
      "employee-1",
      { team_config: { id: "team-rev-1" }, employee_config: { id: "employee-rev-1" } },
    ),
  ).resolves.toEqual(preview);

  expect(fetcher).toHaveBeenCalledWith(
    "http://control-plane.local/api/v1/digital-employees/employee-1/effective-configs/preview",
    expect.objectContaining({ credentials: "include", method: "POST" }),
  );
});

it("approves effective config with the selected revision ids", async () => {
  const approved = {
    id: "effective-config-1",
    digital_employee_id: "employee-1",
    tenant_team_config_revision_id: "team-rev-1",
    employee_config_revision_id: "employee-rev-1",
    effective_config_snapshot: {},
    validation_result: { blocking_errors: [], warnings: [] },
    status: "approved",
  };
  const fetcher = vi.fn(async () => new Response(JSON.stringify(approved), { headers: { "content-type": "application/json" }, status: 200 }));

  await expect(
    approveDigitalEmployeeEffectiveConfig(
      { baseUrl: "http://control-plane.local", fetcher },
      "employee-1",
      { preview: { team_config: { id: "team-rev-1" }, employee_config: { id: "employee-rev-1" } } },
    ),
  ).resolves.toEqual(approved);

  expect(fetcher).toHaveBeenCalledWith(
    "http://control-plane.local/api/v1/digital-employees/employee-1/effective-configs/approve",
    expect.objectContaining({ credentials: "include", method: "POST" }),
  );
});
```

- [ ] **Step 5: Add employee API methods**

Modify `apps/web/src/lib/api/employees.ts`:

```ts
export type CreateDigitalEmployeeInput = {
  name: string;
  role: string;
  team_id: string;
  description?: string;
};

export type DigitalEmployeeConfigRevision = {
  id: string;
  tenant_id?: string;
  digital_employee_id: string;
  revision_number: number;
  role_profile?: Record<string, unknown>;
  constitution_addendum?: Record<string, unknown>;
  capability_selection?: Record<string, unknown>;
  context_policy_override?: Record<string, unknown>;
  approval_policy_override?: Record<string, unknown>;
  output_contract_addendum?: Record<string, unknown>;
  status: string;
};

export type CreateDigitalEmployeeConfigRevisionInput = {
  role_profile?: Record<string, unknown>;
  constitution_addendum?: Record<string, unknown>;
  capability_selection?: Record<string, unknown>;
  context_policy_override?: Record<string, unknown>;
  approval_policy_override?: Record<string, unknown>;
  output_contract_addendum?: Record<string, unknown>;
  status?: string;
};

export type EffectiveConfigValidationIssue = {
  code: string;
  message: string;
  field: string;
};

export type EffectiveConfigPreview = {
  digital_employee_id: string;
  effective_config: Record<string, unknown>;
  validation: {
    blocking_errors: EffectiveConfigValidationIssue[];
    warnings: EffectiveConfigValidationIssue[];
  };
};

export type EffectiveConfigPreviewInput = {
  team_config: { id: string };
  employee_config: { id: string };
};

export type DigitalEmployeeEffectiveConfig = {
  id: string;
  tenant_id?: string;
  digital_employee_id: string;
  tenant_team_config_revision_id: string;
  employee_config_revision_id: string;
  effective_config_snapshot: Record<string, unknown>;
  validation_result: EffectiveConfigPreview["validation"];
  status: string;
  approved_by?: string;
  approved_at?: string;
};

export type ApproveEffectiveConfigInput = {
  preview: EffectiveConfigPreviewInput;
};

export async function createDigitalEmployeeConfigRevision(
  options: ApiClientOptions,
  employeeId: string,
  input: CreateDigitalEmployeeConfigRevisionInput,
): Promise<DigitalEmployeeConfigRevision> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/digital-employees/${encodeURIComponent(employeeId)}/config-revisions`), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });
  return parseJson<DigitalEmployeeConfigRevision>(response, "create digital employee config revision");
}

export async function previewDigitalEmployeeEffectiveConfig(
  options: ApiClientOptions,
  employeeId: string,
  input: EffectiveConfigPreviewInput,
): Promise<EffectiveConfigPreview> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/digital-employees/${encodeURIComponent(employeeId)}/effective-configs/preview`), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });
  return parseJson<EffectiveConfigPreview>(response, "preview digital employee effective config");
}

export async function approveDigitalEmployeeEffectiveConfig(
  options: ApiClientOptions,
  employeeId: string,
  input: ApproveEffectiveConfigInput,
): Promise<DigitalEmployeeEffectiveConfig> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/digital-employees/${encodeURIComponent(employeeId)}/effective-configs/approve`), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });
  return parseJson<DigitalEmployeeEffectiveConfig>(response, "approve digital employee effective config");
}
```

- [ ] **Step 6: Run API client tests**

Run:

```bash
pnpm --filter @superteam/web test -- teams.test.ts employees.test.ts
```

Expected: PASS.

- [ ] **Step 7: Commit Web API clients**

```bash
git add apps/web/src/lib/api/teams.ts \
  apps/web/src/lib/api/teams.test.ts \
  apps/web/src/lib/api/employees.ts \
  apps/web/src/lib/api/employees.test.ts \
  apps/web/src/lib/api/index.ts
git commit -m "feat: add team governance web clients"
```

### Task 6: Add Teams Page and Navigation

**Files:**
- Create: `apps/web/src/features/teams/index.tsx`
- Create: `apps/web/src/features/teams/index.test.tsx`
- Create: `apps/web/src/routes/_authenticated/teams/index.tsx`
- Modify: `apps/web/src/components/layout/data/sidebar-data.ts`
- Modify: `apps/web/src/components/layout/sidebar-data.test.ts`
- Regenerate: `apps/web/src/routeTree.gen.ts`

- [ ] **Step 1: Write failing sidebar test**

Modify `apps/web/src/components/layout/sidebar-data.test.ts`:

```ts
const expectedIconTones = new Map([
  ["工作台", "primary"],
  ["任务中心", "task"],
  ["数字员工", "employee"],
  ["团队管理", "permission"],
  ["流程编排", "workflow"],
  ["外部能力", "capability"],
  ["审批中心", "approval"],
  ["Runtime 节点", "runtime"],
  ["权限中心", "permission"],
  ["用户管理", "neutral"],
  ["审计日志", "audit"],
]);
```

- [ ] **Step 2: Run sidebar test and verify it fails**

Run:

```bash
pnpm --filter @superteam/web test -- sidebar-data.test.ts
```

Expected: FAIL because “团队管理” is not in sidebar data.

- [ ] **Step 3: Add teams page test**

Create `apps/web/src/features/teams/index.test.tsx`:

```tsx
import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { TeamsView } from "@/features/teams";

vi.mock("@/components/layout/header", () => ({ Header: ({ children }: { children: ReactNode }) => <header>{children}</header> }));
vi.mock("@/components/layout/main", () => ({ Main: ({ children }: { children: ReactNode }) => <main>{children}</main> }));
vi.mock("@/components/search", () => ({ Search: () => <button type="button">Search</button> }));
vi.mock("@/components/theme-switch", () => ({ ThemeSwitch: () => <button type="button">Toggle theme</button> }));

function createQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

describe("TeamsView", () => {
  it("renders teams and governance boundaries", async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(String(input));
      if (url.pathname === "/api/v1/teams") {
        return new Response(JSON.stringify([
          { id: "team-1", tenant_id: "tenant-1", slug: "ops", name: "运维团队", status: "active", human_owner_user_id: "user-1" },
        ]), { headers: { "content-type": "application/json" }, status: 200 });
      }
      if (url.pathname === "/api/v1/teams/team-1/config-revisions/current") {
        return new Response(JSON.stringify({
          id: "rev-1",
          team_id: "team-1",
          revision_number: 1,
          constitution: { hard_rules: ["禁止执行未审批的生产写操作"] },
          capability_policy: { allowed_skills: ["incident-diagnosis"] },
          internal_collaboration_policy: { max_auto_rounds: 2 },
          status: "active",
        }), { headers: { "content-type": "application/json" }, status: 200 });
      }
      return new Response("{}", { status: 404 });
    }) as unknown as typeof fetch;

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByRole("heading", { name: "团队管理" })).toBeVisible();
    await expect.element(screen.getByText("运维团队")).toBeVisible();
    await expect.element(screen.getByText("禁止执行未审批的生产写操作")).toBeVisible();
    await expect.element(screen.getByText("incident-diagnosis")).toBeVisible();
  });
});
```

- [ ] **Step 4: Implement Teams page**

Create `apps/web/src/features/teams/index.tsx` with:

```tsx
import { useQuery } from "@tanstack/react-query";
import { UsersRound } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import { getCurrentTeamConfigRevision, listTeams, type Team } from "@/lib/api/teams";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

export function TeamsPage() {
  return <TeamsView apiBaseUrl={resolveControlPlaneUrl()} />;
}

export function TeamsView({ apiBaseUrl, fetcher }: { apiBaseUrl: string; fetcher?: typeof fetch }) {
  const teams = useQuery({ queryKey: ["teams"], queryFn: () => listTeams({ baseUrl: apiBaseUrl, fetcher }) });
  return (
    <>
      <Header><Search /><ThemeSwitch /></Header>
      <Main>
        <div className="mb-4 flex items-center gap-3">
          <div className="flex size-10 items-center justify-center rounded-md border bg-muted"><UsersRound /></div>
          <div>
            <h1 className="text-2xl font-bold tracking-tight">团队管理</h1>
            <p className="text-sm text-muted-foreground">公共宪法、能力边界、上下文、审批和内部协作策略。</p>
          </div>
        </div>
        <Card>
          <CardHeader><CardTitle>团队列表</CardTitle></CardHeader>
          <CardContent className="space-y-3">
            {teams.isLoading ? <p className="text-sm text-muted-foreground">加载中</p> : null}
            {teams.isError ? <p className="text-sm text-destructive">团队列表加载失败</p> : null}
            {(teams.data ?? []).map((team) => <TeamRow key={team.id} apiBaseUrl={apiBaseUrl} team={team} fetcher={fetcher} />)}
          </CardContent>
        </Card>
      </Main>
    </>
  );
}

function TeamRow({ apiBaseUrl, team, fetcher }: { apiBaseUrl: string; team: Team; fetcher?: typeof fetch }) {
  const config = useQuery({
    queryKey: ["team-config-current", team.id],
    queryFn: () => getCurrentTeamConfigRevision({ baseUrl: apiBaseUrl, fetcher }, team.id),
  });
  const hardRules = Array.isArray(config.data?.constitution?.hard_rules) ? config.data.constitution.hard_rules.join(", ") : "未配置硬性规则";
  const skills = Array.isArray(config.data?.capability_policy?.allowed_skills) ? config.data.capability_policy.allowed_skills.join(", ") : "未配置技能边界";
  return (
    <div className="rounded-md border p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <p className="font-medium">{team.name}</p>
          <p className="text-xs text-muted-foreground">{team.slug}</p>
        </div>
        <Badge variant={team.status === "active" ? "default" : "secondary"}>{team.status}</Badge>
      </div>
      <p className="mt-3 text-sm text-muted-foreground">{hardRules}</p>
      <p className="mt-1 text-sm text-muted-foreground">{skills}</p>
    </div>
  );
}
```

- [ ] **Step 5: Add route and sidebar item**

Create `apps/web/src/routes/_authenticated/teams/index.tsx`:

```tsx
import { createFileRoute } from "@tanstack/react-router";
import { TeamsPage } from "@/features/teams";

export const Route = createFileRoute("/_authenticated/teams/")({
  component: TeamsPage,
});
```

Modify `apps/web/src/components/layout/data/sidebar-data.ts`:

```ts
import { UsersRound } from "lucide-react";
```

Add below 数字员工:

```ts
{
  title: "团队管理",
  url: "/teams",
  icon: UsersRound,
  iconTone: "permission",
},
```

- [ ] **Step 6: Regenerate route tree through Vite plugin**

Run:

```bash
pnpm --filter @superteam/web typecheck
```

Expected: PASS and `apps/web/src/routeTree.gen.ts` includes `/teams/`.

- [ ] **Step 7: Run Web tests**

Run:

```bash
pnpm --filter @superteam/web test -- teams/index.test.tsx sidebar-data.test.ts
```

Expected: PASS.

- [ ] **Step 8: Commit teams page**

```bash
git add apps/web/src/features/teams \
  apps/web/src/routes/_authenticated/teams/index.tsx \
  apps/web/src/components/layout/data/sidebar-data.ts \
  apps/web/src/components/layout/sidebar-data.test.ts \
  apps/web/src/routeTree.gen.ts
git commit -m "feat: add team governance page"
```

### Task 7: Add Digital Employee Create Flow with Effective Config Preview

**Files:**
- Modify: `apps/web/src/features/employees/index.tsx`
- Modify: `apps/web/src/features/employees/index.test.tsx`

- [ ] **Step 1: Write failing create-flow test**

Append to `apps/web/src/features/employees/index.test.tsx`:

```tsx
it("previews effective config before creating an active employee", async () => {
  const requests: Array<{ method: string; pathname: string; body?: unknown }> = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";
    const body = typeof init?.body === "string" ? JSON.parse(init.body) : undefined;
    requests.push({ method, pathname: url.pathname, body });
    if (url.pathname === "/api/v1/digital-employees" && method === "GET") {
      return new Response(JSON.stringify([]), { headers: { "content-type": "application/json" }, status: 200 });
    }
    if (url.pathname === "/api/v1/teams" && method === "GET") {
      return new Response(JSON.stringify([{ id: "team-1", tenant_id: "tenant-1", slug: "ops", name: "运维团队", status: "active" }]), { headers: { "content-type": "application/json" }, status: 200 });
    }
    if (url.pathname === "/api/v1/teams/team-1/config-revisions/current" && method === "GET") {
      return new Response(JSON.stringify({
        id: "team-config-rev-1",
        tenant_id: "tenant-1",
        team_id: "team-1",
        revision_number: 1,
        constitution: { hard_rules: ["禁止执行未审批的生产写操作"] },
        capability_policy: { allowed_skills: ["incident-diagnosis"] },
        context_policy: {},
        approval_policy: {},
        artifact_contract: {},
        internal_collaboration_policy: { max_auto_rounds: 2 },
        runtime_scope_policy: {},
        status: "active",
      }), { headers: { "content-type": "application/json" }, status: 200 });
    }
    if (url.pathname === "/api/v1/digital-employees" && method === "POST") {
      return new Response(JSON.stringify({ id: "employee-1", name: "数据库运维员工", role: "database_operator", status: "draft" }), { headers: { "content-type": "application/json" }, status: 201 });
    }
    if (url.pathname === "/api/v1/digital-employees/employee-1/config-revisions" && method === "POST") {
      return new Response(JSON.stringify({
        id: "employee-config-rev-1",
        tenant_id: "tenant-1",
        digital_employee_id: "employee-1",
        revision_number: 1,
        role_profile: { role: "database_operator" },
        capability_selection: { enabled_skills: [] },
        status: "draft",
      }), { headers: { "content-type": "application/json" }, status: 201 });
    }
    if (url.pathname === "/api/v1/digital-employees/employee-1/effective-configs/preview" && method === "POST") {
      return new Response(JSON.stringify({ digital_employee_id: "employee-1", effective_config: {}, validation: { blocking_errors: [], warnings: [] } }), { headers: { "content-type": "application/json" }, status: 200 });
    }
    return new Response("{}", { status: 404 });
  }) as unknown as typeof fetch;
  const screen = await renderEmployeesView(fetcher);

  await vi.waitFor(() => {
    expect(requests).toContainEqual({ method: "GET", pathname: "/api/v1/teams", body: undefined });
  });
  await userEvent.click(screen.getByRole("button", { name: "创建数字员工" }));
  await expect.element(screen.getByLabelText("归属团队")).toBeVisible();
  await userEvent.type(screen.getByLabelText("名称"), "数据库运维员工");
  await userEvent.type(screen.getByLabelText("角色"), "database_operator");
  await userEvent.click(screen.getByRole("button", { name: "预览生效配置" }));

  await vi.waitFor(() => {
    expect(requests).toContainEqual({
      method: "POST",
      pathname: "/api/v1/digital-employees",
      body: { name: "数据库运维员工", role: "database_operator", team_id: "team-1" },
    });
    expect(requests).toContainEqual({
      method: "POST",
      pathname: "/api/v1/digital-employees/employee-1/effective-configs/preview",
      body: {
        team_config: { id: "team-config-rev-1" },
        employee_config: { id: "employee-config-rev-1" },
      },
    });
  });
  await expect.element(screen.getByText("可提交负责人确认")).toBeVisible();
});
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
pnpm --filter @superteam/web test -- employees/index.test.tsx
```

Expected: FAIL because the create button and preview flow do not exist.

- [ ] **Step 3: Add create panel state**

Modify `apps/web/src/features/employees/index.tsx`:

```tsx
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { createDigitalEmployee, createDigitalEmployeeConfigRevision, previewDigitalEmployeeEffectiveConfig } from "@/lib/api/employees";
import { getCurrentTeamConfigRevision, listTeams, type Team } from "@/lib/api/teams";
```

Inside `EmployeesView`, add:

```tsx
const queryClient = useQueryClient();
const [isCreating, setIsCreating] = useState(false);
const teamsQuery = useQuery({
  queryKey: ["teams"],
  queryFn: () => listTeams({ baseUrl: apiBaseUrl, fetcher }),
});
```

Add a button in the page header:

```tsx
<Button type="button" disabled={teamsQuery.isLoading || (teamsQuery.data ?? []).length === 0} onClick={() => setIsCreating(true)}>
  创建数字员工
</Button>
```

- [ ] **Step 4: Add create form component**

Append in the same file:

```tsx
function CreateEmployeePanel({
  apiBaseUrl,
  fetcher,
  teams,
  onCreated,
}: {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  teams: Team[];
  onCreated: () => void;
}) {
  const [name, setName] = useState("");
  const [role, setRole] = useState("");
  const [selectedTeamId, setSelectedTeamId] = useState("");
  const [previewMessage, setPreviewMessage] = useState("");
  const selectedTeam = teams.find((team) => team.id === selectedTeamId) ?? teams[0];
  const effectiveTeamId = selectedTeam?.id ?? "";
  const create = useMutation({
    mutationFn: () => createDigitalEmployee({ baseUrl: apiBaseUrl, fetcher }, { name, role, team_id: effectiveTeamId }),
  });
  const preview = useMutation({
    mutationFn: async () => {
      if (!effectiveTeamId) {
        throw new Error("team is required");
      }
      const teamConfig = await getCurrentTeamConfigRevision({ baseUrl: apiBaseUrl, fetcher }, effectiveTeamId);
      const employee = await create.mutateAsync();
      const employeeConfig = await createDigitalEmployeeConfigRevision(
        { baseUrl: apiBaseUrl, fetcher },
        employee.id,
        {
          role_profile: { role },
          capability_selection: { enabled_skills: [] },
          status: "draft",
        },
      );
      return previewDigitalEmployeeEffectiveConfig(
        { baseUrl: apiBaseUrl, fetcher },
        employee.id,
        {
          team_config: { id: teamConfig.id },
          employee_config: { id: employeeConfig.id },
        },
      );
    },
    onSuccess: (result) => {
      setPreviewMessage(result.validation.blocking_errors.length === 0 ? "可提交负责人确认" : "存在阻断错误");
      onCreated();
    },
  });

  return (
    <Card>
      <CardHeader><CardTitle>创建数字员工</CardTitle></CardHeader>
      <CardContent className="space-y-3">
        <label className="grid gap-1 text-sm">
          名称
          <Input value={name} onChange={(event) => setName(event.target.value)} />
        </label>
        <label className="grid gap-1 text-sm">
          角色
          <Input value={role} onChange={(event) => setRole(event.target.value)} />
        </label>
        <label className="grid gap-1 text-sm">
          归属团队
          <select
            className="h-9 rounded-md border bg-background px-3 text-sm"
            value={effectiveTeamId}
            onChange={(event) => setSelectedTeamId(event.target.value)}
          >
            {teams.map((team) => (
              <option key={team.id} value={team.id}>{team.name}</option>
            ))}
          </select>
        </label>
        <Button type="button" disabled={preview.isPending || name.trim() === "" || role.trim() === "" || !effectiveTeamId} onClick={() => preview.mutate()}>
          预览生效配置
        </Button>
        {previewMessage ? <p className="text-sm text-muted-foreground">{previewMessage}</p> : null}
      </CardContent>
    </Card>
  );
}
```

- [ ] **Step 5: Render create panel and invalidate employees**

Inside `EmployeesView`, render:

```tsx
{teamsQuery.isError ? <p className="mb-3 text-sm text-destructive">团队列表加载失败</p> : null}
{isCreating ? (
  <div className="mb-4">
    <CreateEmployeePanel
      apiBaseUrl={apiBaseUrl}
      fetcher={fetcher}
      teams={teamsQuery.data ?? []}
      onCreated={() => {
        void queryClient.invalidateQueries({ queryKey: ["digital-employees"] });
      }}
    />
  </div>
) : null}
```

- [ ] **Step 6: Run employees Web tests**

Run:

```bash
pnpm --filter @superteam/web test -- employees/index.test.tsx
```

Expected: PASS.

- [ ] **Step 7: Commit employee create flow**

```bash
git add apps/web/src/features/employees/index.tsx apps/web/src/features/employees/index.test.tsx
git commit -m "feat: preview digital employee effective config"
```

### Task 8: Final Verification

**Files:**
- No new files unless verification reveals a defect.

- [ ] **Step 1: Run control-plane tests**

Run:

```bash
pnpm test:go
```

Expected: PASS.

- [ ] **Step 2: Run Web tests and typecheck**

Run:

```bash
pnpm --filter @superteam/web test
pnpm --filter @superteam/web typecheck
```

Expected: PASS.

- [ ] **Step 3: Run foundation verification**

Run:

```bash
pnpm verify:foundation
```

Expected: PASS. If Rust tests are unrelatedly slow but pass individually, report the exact command outputs and do not mark the foundation gate as passed unless the full command exits 0.

- [ ] **Step 4: Inspect final git state**

Run:

```bash
git status --short
git log --oneline -8
```

Expected: only intentional tracked changes are present before final commit or all planned commits are already made.

- [ ] **Step 5: Record final changelog state if missed**

If `CHANGELOG.md` does not yet contain the team governance entry, add it under `[Unreleased] -> Added`:

```markdown
#### 团队与数字员工治理模型 (2026-06-03)

- 新增团队治理配置、数字员工个人配置和生效配置预览/审批计划，支持团队公共 MCP、技能、插件、宪法、上下文、审批、内部协作和 Runtime 范围边界。
```

- [ ] **Step 6: Final commit if needed**

```bash
git add CHANGELOG.md
git commit -m "chore: record team governance rollout"
```

Run this commit only if `CHANGELOG.md` changed during final verification.
