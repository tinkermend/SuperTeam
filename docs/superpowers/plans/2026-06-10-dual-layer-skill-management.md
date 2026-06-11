# Dual Layer Skill Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现团队公共 Skills/MCP、数字员工个人 Instructions/Skills/MCP、个人凭据池三层治理界面和控制平面 API。

**Architecture:** 复用现有 `skills`、`skill_team_bindings`、`skill_agent_bindings` 作为技能主数据和双层绑定事实；新增凭据、团队 MCP、个人 MCP、个人 Instructions 文件表。Control Plane 提供可审计的 REST API，Web 在团队详情和数字员工配置页分别呈现物理隔离的管理入口，并通过 effective capabilities 只读预览合并结果。

**Tech Stack:** Go + chi/net/http + pgx/sqlc + PostgreSQL migrations + OpenAPI；React + TanStack Query + Monaco Editor + shadcn/ui + SuperTeam liquid components + Vitest Browser。

---

## Scope And Guardrails

- 本计划覆盖一个 cohesive feature，不拆成多个独立计划，因为团队层能力、个人层能力、凭据池和合并预览需要同一套 API 契约才能端到端验证。
- 不重命名现有 `skill_agent_bindings`，它已经通过 `digital_employee_id` 表达数字员工个人技能绑定；新增代码统一把对外语义命名为 employee personal skill binding。
- 不修改历史初始 migration；新增 `013_dual_layer_skill_management.sql`。
- 数据库默认不为跨模块关系添加 FK。`credential_id`、`team_id`、`digital_employee_id`、`skill_id` 使用 UUID 引用和应用层校验，符合 `DATABASE_DESIGN.md` 的 audit-preserving 和 application-controlled first 规则。
- 凭据响应永不返回明文 token。创建时接收 `credential_value`，服务端使用 AES-GCM 封存到 `encrypted_value`；列表和 MCP 配置响应只返回 `credential_name`、`credential_type`、`last_four`。
- `CONTROL_PLANE_CREDENTIAL_KEY` 为 base64 编码的 32 字节 AES key。缺失时仍可读取非敏感能力数据，但创建凭据和解析 MCP Authorization header 必须返回 `ErrCredentialKeyMissing`。
- 团队公共 skill/MCP 是强制继承，只能在团队页管理；数字员工页继承区只读。个人 skill/MCP 只能在数字员工页管理。
- Instructions 文件仅属于数字员工个人层，不从团队继承。第一版支持 `AGENTS.md`、`SOUL.md` 和任意安全相对路径 Markdown/text 文件。
- 所有新增文档和页面文案使用简体中文。CHANGELOG 每条新增变更使用 `Asia/Shanghai` 时间，格式 `YYYY-MM-DD HH:mm`。

## File Structure

Backend storage:

- Create: `apps/control-plane/internal/storage/migrations/013_dual_layer_skill_management.sql`
  - 新增 `user_credentials`、`team_mcp_servers`、`digital_employee_mcp_bindings`、`digital_employee_instruction_files`。
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`
  - 运行 Atlas 校验刷新。
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
  - 校验新增表、索引、中文注释和禁止返回明文字段。
- Create: `apps/control-plane/internal/storage/queries/capability.sql`
  - 凭据、团队 MCP、个人 MCP、effective MCP 查询。
- Create: `apps/control-plane/internal/storage/queries/employee_instructions.sql`
  - 数字员工 Instructions 文件列表和 upsert 查询。
- Modify: `apps/control-plane/internal/storage/queries/querier.go`
- Modify: `apps/control-plane/internal/storage/queries/models.go`
- Generated: `apps/control-plane/internal/storage/queries/capability.sql.go`
  - 运行 `make generate-sqlc` 后生成。
- Generated: `apps/control-plane/internal/storage/queries/employee_instructions.sql.go`
  - 运行 `make generate-sqlc` 后生成。

Backend domain:

- Create: `apps/control-plane/internal/capability/types.go`
  - 凭据、MCP server、effective capability DTO、错误类型。
- Create: `apps/control-plane/internal/capability/crypto.go`
  - AES-GCM 凭据封存和解封。
- Create: `apps/control-plane/internal/capability/service.go`
  - 凭据、团队 MCP、个人 MCP、effective MCP、Authorization header 逻辑。
- Create: `apps/control-plane/internal/capability/pg_repository.go`
  - sqlc repository adapter。
- Create: `apps/control-plane/internal/capability/handler.go`
  - REST handler 和响应脱敏。
- Create: `apps/control-plane/internal/capability/service_test.go`
- Create: `apps/control-plane/internal/capability/handler_test.go`
- Modify: `apps/control-plane/internal/app/app.go`
  - 初始化 capability service/handler。
- Modify: `apps/control-plane/internal/api/server.go`
  - 注册 capability routes。
- Modify: `apps/control-plane/internal/authz/types.go`
  - 增加 `credential.*`、`team.capability.manage`、`employee.capability.edit` actions。
- Modify: `apps/control-plane/internal/api/team_routes_test.go`
  - 覆盖团队 MCP routes 授权。
- Modify: `apps/control-plane/internal/api/employee_routes_test.go`
  - 覆盖员工 personal MCP 和 effective capabilities routes。

Backend skill and employee modules:

- Modify: `apps/control-plane/internal/skill/types.go`
  - 增加 team/employee binding request/response domain types。
- Modify: `apps/control-plane/internal/skill/service.go`
  - 增加团队安装、团队卸载、个人安装、个人卸载、员工 effective skill merge。
- Modify: `apps/control-plane/internal/skill/pg_repository.go`
  - 直接 SQL 实现绑定查询和写入。
- Modify: `apps/control-plane/internal/skill/handler.go`
  - 增加 team skills 和 employee skills endpoints。
- Modify: `apps/control-plane/internal/skill/service_test.go`
- Modify: `apps/control-plane/internal/api/skill_routes_test.go`
- Modify: `apps/control-plane/internal/employee/types.go`
  - 增加 instruction file request/response types。
- Modify: `apps/control-plane/internal/employee/repository.go`
  - 增加 instruction file repository 方法。
- Modify: `apps/control-plane/internal/employee/pg_repository.go`
  - 实现 instruction file 查询和 upsert。
- Modify: `apps/control-plane/internal/employee/service.go`
  - 实现 instruction file 校验。
- Modify: `apps/control-plane/internal/employee/handler.go`
  - 增加 instruction file endpoints。
- Modify: `apps/control-plane/internal/employee/service_test.go`
- Modify: `apps/control-plane/internal/api/employee_routes_test.go`

Contracts and Web API:

- Modify: `contracts/control-plane/openapi.yaml`
  - 增加 credentials、team skills、employee skills、team MCP、employee MCP、instructions、effective capabilities paths/schemas。
- Modify: `apps/web/src/lib/api/skills.ts`
  - 增加 bind/list/unbind API。
- Modify: `apps/web/src/lib/api/skills.test.ts`
- Create: `apps/web/src/lib/api/capabilities.ts`
  - 凭据、MCP、effective capabilities API client。
- Create: `apps/web/src/lib/api/capabilities.test.ts`
- Modify: `apps/web/src/lib/api/employees.ts`
  - 增加 instruction file API types/functions。
- Modify: `apps/web/src/lib/api/employees.test.ts`

Web UI:

- Modify: `apps/web/src/features/teams/components/team-capabilities-tab.tsx`
  - 从治理草稿占位 JSON 改为公共 Skills/MCP 管理。
- Modify: `apps/web/src/features/teams/index.test.tsx`
  - 覆盖团队公共技能安装和 MCP 创建。
- Create: `apps/web/src/features/employees/components/instruction-files-panel.tsx`
  - 文件列表 + Monaco Markdown 编辑器。
- Create: `apps/web/src/features/employees/components/employee-capabilities-panel.tsx`
  - 个人 Skills/MCP 编辑 + 团队继承只读预览。
- Modify: `apps/web/src/features/employees/config.tsx`
  - 用 Tabs 承载 Instructions、Capabilities、Legacy JSON 高级配置。
- Modify: `apps/web/src/features/employees/config.test.tsx`
  - 覆盖指令文件保存、个人技能安装、继承技能只读、个人 MCP 绑定。

Docs:

- Modify: `CHANGELOG.md`
  - 实现完成时追加一条带 Asia/Shanghai 时间的变更。

## Task 1: Storage Migration For Credentials, MCP, And Instruction Files

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/013_dual_layer_skill_management.sql`
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`

- [ ] **Step 1: Write the failing migration test**

Append this test to `apps/control-plane/internal/storage/migrations_test.go`:

```go
func TestDualLayerSkillManagementMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/013_dual_layer_skill_management.sql")
	if err != nil {
		t.Fatalf("read dual layer skill management migration: %v", err)
	}
	sql := string(body)

	required := []string{
		"CREATE TABLE IF NOT EXISTS user_credentials",
		"CREATE TABLE IF NOT EXISTS team_mcp_servers",
		"CREATE TABLE IF NOT EXISTS digital_employee_mcp_bindings",
		"CREATE TABLE IF NOT EXISTS digital_employee_instruction_files",
		"encrypted_value TEXT NOT NULL",
		"credential_id UUID",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_user_credentials_owner_name_active",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_team_mcp_servers_team_name_active",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_digital_employee_mcp_bindings_employee_name_active",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_digital_employee_instruction_files_path_active",
		"COMMENT ON TABLE user_credentials IS '个人凭据池，保存用户可复用的外部能力授权令牌密文'",
		"COMMENT ON TABLE team_mcp_servers IS '团队公共 MCP 服务器配置，团队下数字员工强制继承'",
		"COMMENT ON TABLE digital_employee_mcp_bindings IS '数字员工个人 MCP 服务器配置'",
		"COMMENT ON TABLE digital_employee_instruction_files IS '数字员工个人 Instructions 文件内容'",
	}
	for _, expected := range required {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected migration to contain %q", expected)
		}
	}

	forbidden := []string{
		"credential_value",
		"REFERENCES user_credentials",
		"ON DELETE CASCADE",
	}
	for _, value := range forbidden {
		if strings.Contains(sql, value) {
			t.Fatalf("migration must not contain %q", value)
		}
	}
}
```

- [ ] **Step 2: Run the migration test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestDualLayerSkillManagementMigration -count=1
```

Expected: FAIL with `read dual layer skill management migration`.

- [ ] **Step 3: Create the migration**

Create `apps/control-plane/internal/storage/migrations/013_dual_layer_skill_management.sql`:

```sql
CREATE TABLE IF NOT EXISTS user_credentials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    user_id UUID NOT NULL,
    name TEXT NOT NULL,
    credential_type VARCHAR(80) NOT NULL,
    encrypted_value TEXT NOT NULL,
    last_four VARCHAR(8) NOT NULL DEFAULT '',
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    disabled_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_user_credentials_owner_name_active
    ON user_credentials(tenant_id, user_id, name)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_user_credentials_owner_type
    ON user_credentials(tenant_id, user_id, credential_type, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS team_mcp_servers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    team_id UUID NOT NULL,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    credential_id UUID,
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    disabled_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_team_mcp_servers_team_name_active
    ON team_mcp_servers(tenant_id, team_id, name)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_team_mcp_servers_team_status
    ON team_mcp_servers(tenant_id, team_id, status, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS digital_employee_mcp_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    digital_employee_id UUID NOT NULL,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    credential_id UUID,
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    disabled_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_digital_employee_mcp_bindings_employee_name_active
    ON digital_employee_mcp_bindings(tenant_id, digital_employee_id, name)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_digital_employee_mcp_bindings_employee_status
    ON digital_employee_mcp_bindings(tenant_id, digital_employee_id, status, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS digital_employee_instruction_files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    digital_employee_id UUID NOT NULL,
    path TEXT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    checksum_sha256 VARCHAR(64) NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_digital_employee_instruction_files_path_active
    ON digital_employee_instruction_files(tenant_id, digital_employee_id, path)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_digital_employee_instruction_files_employee_path
    ON digital_employee_instruction_files(tenant_id, digital_employee_id, path)
    WHERE deleted_at IS NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_user_credentials_updated_at'
    ) THEN
        CREATE TRIGGER update_user_credentials_updated_at
        BEFORE UPDATE ON user_credentials
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_team_mcp_servers_updated_at'
    ) THEN
        CREATE TRIGGER update_team_mcp_servers_updated_at
        BEFORE UPDATE ON team_mcp_servers
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_digital_employee_mcp_bindings_updated_at'
    ) THEN
        CREATE TRIGGER update_digital_employee_mcp_bindings_updated_at
        BEFORE UPDATE ON digital_employee_mcp_bindings
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_digital_employee_instruction_files_updated_at'
    ) THEN
        CREATE TRIGGER update_digital_employee_instruction_files_updated_at
        BEFORE UPDATE ON digital_employee_instruction_files
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

COMMENT ON TABLE user_credentials IS '个人凭据池，保存用户可复用的外部能力授权令牌密文';
COMMENT ON COLUMN user_credentials.id IS '个人凭据主键 UUID';
COMMENT ON COLUMN user_credentials.tenant_id IS '凭据所属租户 ID';
COMMENT ON COLUMN user_credentials.user_id IS '凭据所属用户 ID';
COMMENT ON COLUMN user_credentials.name IS '凭据显示名称，同一用户下未删除时唯一';
COMMENT ON COLUMN user_credentials.credential_type IS '凭据类型，例如 mcp_token，由服务端注册表校验';
COMMENT ON COLUMN user_credentials.encrypted_value IS '服务端封存后的凭据密文，API 永不返回明文';
COMMENT ON COLUMN user_credentials.last_four IS '凭据明文末尾四位或更短尾标，用于用户识别';
COMMENT ON COLUMN user_credentials.status IS '凭据状态，例如 active 或 disabled';
COMMENT ON COLUMN user_credentials.metadata IS '凭据扩展元数据 JSON';
COMMENT ON COLUMN user_credentials.disabled_at IS '凭据禁用时间';
COMMENT ON COLUMN user_credentials.deleted_at IS '凭据软删除时间';
COMMENT ON COLUMN user_credentials.created_at IS '凭据创建时间';
COMMENT ON COLUMN user_credentials.updated_at IS '凭据更新时间';

COMMENT ON TABLE team_mcp_servers IS '团队公共 MCP 服务器配置，团队下数字员工强制继承';
COMMENT ON COLUMN team_mcp_servers.id IS '团队 MCP 配置主键 UUID';
COMMENT ON COLUMN team_mcp_servers.tenant_id IS '团队 MCP 所属租户 ID';
COMMENT ON COLUMN team_mcp_servers.team_id IS '团队 MCP 所属团队 ID';
COMMENT ON COLUMN team_mcp_servers.name IS '团队 MCP 显示名称，同一团队下未删除时唯一';
COMMENT ON COLUMN team_mcp_servers.url IS '团队 MCP 远程 HTTP 地址';
COMMENT ON COLUMN team_mcp_servers.credential_id IS '引用的个人凭据 ID，由应用层校验归属和类型';
COMMENT ON COLUMN team_mcp_servers.status IS '团队 MCP 状态，例如 active 或 disabled';
COMMENT ON COLUMN team_mcp_servers.metadata IS '团队 MCP 扩展元数据 JSON';
COMMENT ON COLUMN team_mcp_servers.disabled_at IS '团队 MCP 禁用时间';
COMMENT ON COLUMN team_mcp_servers.deleted_at IS '团队 MCP 软删除时间';
COMMENT ON COLUMN team_mcp_servers.created_by IS '创建团队 MCP 的用户 ID';
COMMENT ON COLUMN team_mcp_servers.created_at IS '团队 MCP 创建时间';
COMMENT ON COLUMN team_mcp_servers.updated_at IS '团队 MCP 更新时间';

COMMENT ON TABLE digital_employee_mcp_bindings IS '数字员工个人 MCP 服务器配置';
COMMENT ON COLUMN digital_employee_mcp_bindings.id IS '数字员工 MCP 配置主键 UUID';
COMMENT ON COLUMN digital_employee_mcp_bindings.tenant_id IS '数字员工 MCP 所属租户 ID';
COMMENT ON COLUMN digital_employee_mcp_bindings.digital_employee_id IS '数字员工 ID';
COMMENT ON COLUMN digital_employee_mcp_bindings.name IS '个人 MCP 显示名称，同一数字员工下未删除时唯一';
COMMENT ON COLUMN digital_employee_mcp_bindings.url IS '个人 MCP 远程 HTTP 地址';
COMMENT ON COLUMN digital_employee_mcp_bindings.credential_id IS '引用的个人凭据 ID，由应用层校验归属和类型';
COMMENT ON COLUMN digital_employee_mcp_bindings.status IS '个人 MCP 状态，例如 active 或 disabled';
COMMENT ON COLUMN digital_employee_mcp_bindings.metadata IS '个人 MCP 扩展元数据 JSON';
COMMENT ON COLUMN digital_employee_mcp_bindings.disabled_at IS '个人 MCP 禁用时间';
COMMENT ON COLUMN digital_employee_mcp_bindings.deleted_at IS '个人 MCP 软删除时间';
COMMENT ON COLUMN digital_employee_mcp_bindings.created_by IS '创建个人 MCP 的用户 ID';
COMMENT ON COLUMN digital_employee_mcp_bindings.created_at IS '个人 MCP 创建时间';
COMMENT ON COLUMN digital_employee_mcp_bindings.updated_at IS '个人 MCP 更新时间';

COMMENT ON TABLE digital_employee_instruction_files IS '数字员工个人 Instructions 文件内容';
COMMENT ON COLUMN digital_employee_instruction_files.id IS '数字员工 Instructions 文件主键 UUID';
COMMENT ON COLUMN digital_employee_instruction_files.tenant_id IS '文件所属租户 ID';
COMMENT ON COLUMN digital_employee_instruction_files.digital_employee_id IS '数字员工 ID';
COMMENT ON COLUMN digital_employee_instruction_files.path IS '文件相对路径，例如 AGENTS.md 或 SOUL.md';
COMMENT ON COLUMN digital_employee_instruction_files.content IS '文件文本内容';
COMMENT ON COLUMN digital_employee_instruction_files.size_bytes IS '文件内容字节数';
COMMENT ON COLUMN digital_employee_instruction_files.checksum_sha256 IS '文件内容 SHA256 校验值';
COMMENT ON COLUMN digital_employee_instruction_files.metadata IS '文件扩展元数据 JSON';
COMMENT ON COLUMN digital_employee_instruction_files.deleted_at IS '文件软删除时间';
COMMENT ON COLUMN digital_employee_instruction_files.created_at IS '文件创建时间';
COMMENT ON COLUMN digital_employee_instruction_files.updated_at IS '文件更新时间';
```

- [ ] **Step 4: Run migration test and refresh Atlas checksum**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestDualLayerSkillManagementMigration -count=1
cd apps/control-plane && atlas migrate hash --dir "file://internal/storage/migrations"
```

Expected: migration test PASS, `apps/control-plane/internal/storage/migrations/atlas.sum` updated.

- [ ] **Step 5: Commit**

```bash
git add apps/control-plane/internal/storage/migrations/013_dual_layer_skill_management.sql apps/control-plane/internal/storage/migrations/atlas.sum apps/control-plane/internal/storage/migrations_test.go
git commit -m "feat: add dual layer capability storage"
```

## Task 2: Capability SQL Queries

**Files:**
- Create: `apps/control-plane/internal/storage/queries/capability.sql`
- Modify generated: `apps/control-plane/internal/storage/queries/capability.sql.go`
- Modify generated: `apps/control-plane/internal/storage/queries/models.go`
- Modify generated: `apps/control-plane/internal/storage/queries/querier.go`
- Test: `apps/control-plane/internal/storage/queries/queries_test.go`

- [ ] **Step 1: Write the failing query test**

Append this test to `apps/control-plane/internal/storage/queries/queries_test.go`:

```go
func TestCapabilityQueriesCreateCredentialAndMergeMCPServers(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	tenantID := seedTestTenant(t, testDB)
	userID := seedTestAuthUser(t, testDB, "capability-owner")
	teamID := seedTestTeam(t, testDB, tenantID, "platform", "平台工程")
	employee, err := testQueries.CreateDigitalEmployee(ctx, CreateDigitalEmployeeParams{
		TenantID:  tenantID,
		TeamID:    pgtype.UUID{Bytes: teamID, Valid: true},
		Name:      "capability-agent",
		Role:      "capability_test",
		Status:    "draft",
		RiskLevel: "low",
	})
	if err != nil {
		t.Fatalf("create digital employee: %v", err)
	}
	employeeID := employee.ID

	credential, err := testQueries.CreateUserCredential(ctx, CreateUserCredentialParams{
		TenantID:       tenantID,
		UserID:         userID,
		Name:           "ops-token",
		CredentialType: "mcp_token",
		EncryptedValue: "sealed-token",
		LastFour:       "7890",
	})
	if err != nil {
		t.Fatalf("create credential: %v", err)
	}

	if _, err := testQueries.CreateTeamMCPServer(ctx, CreateTeamMCPServerParams{
		TenantID:     tenantID,
		TeamID:       teamID,
		Name:         "ops-mcp",
		Url:          "https://mcp.example.com",
		CredentialID: pgtype.UUID{Bytes: credential.ID, Valid: true},
		CreatedBy:    pgtype.UUID{Bytes: userID, Valid: true},
	}); err != nil {
		t.Fatalf("create team mcp: %v", err)
	}

	if _, err := testQueries.CreateDigitalEmployeeMCPBinding(ctx, CreateDigitalEmployeeMCPBindingParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		Name:              "personal-mcp",
		Url:               "https://personal-mcp.example.com",
		CredentialID:      pgtype.UUID{Bytes: credential.ID, Valid: true},
		CreatedBy:         pgtype.UUID{Bytes: userID, Valid: true},
	}); err != nil {
		t.Fatalf("create employee mcp: %v", err)
	}

	merged, err := testQueries.ListEffectiveMCPServersForEmployee(ctx, ListEffectiveMCPServersForEmployeeParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		t.Fatalf("list effective mcp: %v", err)
	}
	if len(merged) != 2 {
		t.Fatalf("expected 2 merged mcp servers, got %d", len(merged))
	}
	if merged[0].SourceScope != "team" || merged[1].SourceScope != "employee" {
		t.Fatalf("expected team before employee source ordering, got %#v", merged)
	}
}
```

- [ ] **Step 2: Run the query test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/storage/queries -run TestCapabilityQueriesCreateCredentialAndMergeMCPServers -count=1
```

Expected: FAIL because `CreateUserCredential` and related query methods do not exist.

- [ ] **Step 3: Add SQL queries**

Create `apps/control-plane/internal/storage/queries/capability.sql`:

```sql
-- name: CreateUserCredential :one
INSERT INTO user_credentials (
    tenant_id,
    user_id,
    name,
    credential_type,
    encrypted_value,
    last_four,
    metadata
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('user_id')::uuid,
    sqlc.arg('name')::text,
    sqlc.arg('credential_type')::varchar,
    sqlc.arg('encrypted_value')::text,
    sqlc.arg('last_four')::varchar,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
)
RETURNING *;

-- name: ListUserCredentials :many
SELECT *
FROM user_credentials
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND user_id = sqlc.arg('user_id')::uuid
  AND deleted_at IS NULL
  AND (
    sqlc.narg('credential_type')::varchar IS NULL
    OR credential_type = sqlc.narg('credential_type')::varchar
  )
ORDER BY created_at DESC, name ASC;

-- name: GetUserCredential :one
SELECT *
FROM user_credentials
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND user_id = sqlc.arg('user_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL;

-- name: CreateTeamMCPServer :one
INSERT INTO team_mcp_servers (
    tenant_id,
    team_id,
    name,
    url,
    credential_id,
    metadata,
    created_by
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('team_id')::uuid,
    sqlc.arg('name')::text,
    sqlc.arg('url')::text,
    sqlc.narg('credential_id')::uuid,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb),
    sqlc.narg('created_by')::uuid
)
RETURNING *;

-- name: ListTeamMCPServers :many
SELECT
    tm.id,
    tm.tenant_id,
    tm.team_id,
    tm.name,
    tm.url,
    tm.credential_id,
    tm.status,
    tm.metadata,
    tm.disabled_at,
    tm.deleted_at,
    tm.created_by,
    tm.created_at,
    tm.updated_at,
    COALESCE(uc.name, '') AS credential_name,
    COALESCE(uc.credential_type, '') AS credential_type,
    COALESCE(uc.last_four, '') AS credential_last_four
FROM team_mcp_servers tm
LEFT JOIN user_credentials uc ON uc.tenant_id = tm.tenant_id
    AND uc.id = tm.credential_id
    AND uc.deleted_at IS NULL
WHERE tm.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tm.team_id = sqlc.arg('team_id')::uuid
  AND tm.deleted_at IS NULL
ORDER BY tm.created_at DESC, tm.name ASC;

-- name: DeleteTeamMCPServer :exec
UPDATE team_mcp_servers
SET deleted_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL;

-- name: CreateDigitalEmployeeMCPBinding :one
INSERT INTO digital_employee_mcp_bindings (
    tenant_id,
    digital_employee_id,
    name,
    url,
    credential_id,
    metadata,
    created_by
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('name')::text,
    sqlc.arg('url')::text,
    sqlc.narg('credential_id')::uuid,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb),
    sqlc.narg('created_by')::uuid
)
RETURNING *;

-- name: ListDigitalEmployeeMCPBindings :many
SELECT
    em.id,
    em.tenant_id,
    em.digital_employee_id,
    em.name,
    em.url,
    em.credential_id,
    em.status,
    em.metadata,
    em.disabled_at,
    em.deleted_at,
    em.created_by,
    em.created_at,
    em.updated_at,
    COALESCE(uc.name, '') AS credential_name,
    COALESCE(uc.credential_type, '') AS credential_type,
    COALESCE(uc.last_four, '') AS credential_last_four
FROM digital_employee_mcp_bindings em
LEFT JOIN user_credentials uc ON uc.tenant_id = em.tenant_id
    AND uc.id = em.credential_id
    AND uc.deleted_at IS NULL
WHERE em.tenant_id = sqlc.arg('tenant_id')::uuid
  AND em.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND em.deleted_at IS NULL
ORDER BY em.created_at DESC, em.name ASC;

-- name: DeleteDigitalEmployeeMCPBinding :exec
UPDATE digital_employee_mcp_bindings
SET deleted_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL;

-- name: ListEffectiveMCPServersForEmployee :many
WITH target_employee AS (
    SELECT tenant_id, id AS digital_employee_id, team_id
    FROM digital_employees
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND id = sqlc.arg('digital_employee_id')::uuid
      AND deleted_at IS NULL
)
SELECT
    tm.id,
    tm.tenant_id,
    target_employee.digital_employee_id,
    tm.team_id,
    tm.name,
    tm.url,
    tm.credential_id,
    tm.status,
    'team'::text AS source_scope,
    true AS inherited,
    COALESCE(uc.name, '') AS credential_name,
    COALESCE(uc.credential_type, '') AS credential_type,
    COALESCE(uc.last_four, '') AS credential_last_four,
    tm.created_at,
    tm.updated_at
FROM target_employee
JOIN team_mcp_servers tm ON tm.tenant_id = target_employee.tenant_id
    AND tm.team_id = target_employee.team_id
    AND tm.deleted_at IS NULL
LEFT JOIN user_credentials uc ON uc.tenant_id = tm.tenant_id
    AND uc.id = tm.credential_id
    AND uc.deleted_at IS NULL
UNION ALL
SELECT
    em.id,
    em.tenant_id,
    em.digital_employee_id,
    NULL::uuid AS team_id,
    em.name,
    em.url,
    em.credential_id,
    em.status,
    'employee'::text AS source_scope,
    false AS inherited,
    COALESCE(uc.name, '') AS credential_name,
    COALESCE(uc.credential_type, '') AS credential_type,
    COALESCE(uc.last_four, '') AS credential_last_four,
    em.created_at,
    em.updated_at
FROM digital_employee_mcp_bindings em
LEFT JOIN user_credentials uc ON uc.tenant_id = em.tenant_id
    AND uc.id = em.credential_id
    AND uc.deleted_at IS NULL
WHERE em.tenant_id = sqlc.arg('tenant_id')::uuid
  AND em.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND em.deleted_at IS NULL
ORDER BY inherited DESC, name ASC;
```

- [ ] **Step 4: Regenerate sqlc and run query test**

Run:

```bash
cd apps/control-plane && make generate-sqlc
go test ./internal/storage/queries -run TestCapabilityQueriesCreateCredentialAndMergeMCPServers -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/control-plane/internal/storage/queries/capability.sql apps/control-plane/internal/storage/queries/capability.sql.go apps/control-plane/internal/storage/queries/models.go apps/control-plane/internal/storage/queries/querier.go apps/control-plane/internal/storage/queries/queries_test.go
git commit -m "feat: add capability sql queries"
```

## Task 3: Capability Service, Credential Sealing, And MCP Token Injection

**Files:**
- Create: `apps/control-plane/internal/capability/types.go`
- Create: `apps/control-plane/internal/capability/crypto.go`
- Create: `apps/control-plane/internal/capability/service.go`
- Create: `apps/control-plane/internal/capability/pg_repository.go`
- Test: `apps/control-plane/internal/capability/service_test.go`

- [ ] **Step 1: Write failing service tests**

Create `apps/control-plane/internal/capability/service_test.go`:

```go
package capability

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestServiceCreatesCredentialWithSealedValueAndRedactedResponse(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	sealer, err := NewAESGCMCredentialSealer(key)
	if err != nil {
		t.Fatalf("new sealer: %v", err)
	}
	repo := &serviceRepo{}
	service := NewService(repo, sealer)
	tenantID := uuid.New()
	userID := uuid.New()

	created, err := service.CreateCredential(context.Background(), CreateCredentialRequest{
		TenantID:        tenantID,
		UserID:          userID,
		Name:            "ops-token",
		CredentialType:  CredentialTypeMCPToken,
		CredentialValue: "sk-test-1234567890",
	})
	if err != nil {
		t.Fatalf("create credential: %v", err)
	}

	if created.EncryptedValue != "" {
		t.Fatalf("credential response must not expose encrypted value")
	}
	if created.LastFour != "7890" {
		t.Fatalf("expected last four 7890, got %q", created.LastFour)
	}
	if repo.createdCredential.EncryptedValue == "" || strings.Contains(repo.createdCredential.EncryptedValue, "sk-test") {
		t.Fatalf("expected sealed credential value, got %q", repo.createdCredential.EncryptedValue)
	}
}

func TestServiceBuildsMCPAuthorizationHeaderFromCredential(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	sealer, err := NewAESGCMCredentialSealer(key)
	if err != nil {
		t.Fatalf("new sealer: %v", err)
	}
	sealed, err := sealer.Seal("mcp-secret-token")
	if err != nil {
		t.Fatalf("seal token: %v", err)
	}
	tenantID := uuid.New()
	userID := uuid.New()
	credentialID := uuid.New()
	repo := &serviceRepo{
		credential: Credential{
			ID:             credentialID,
			TenantID:       tenantID,
			UserID:         userID,
			Name:           "ops-token",
			CredentialType: CredentialTypeMCPToken,
			EncryptedValue: sealed,
			LastFour:       "oken",
			Status:         "active",
		},
	}
	service := NewService(repo, sealer)

	header, err := service.BuildMCPAuthorizationHeader(context.Background(), ResolveCredentialRequest{
		TenantID:     tenantID,
		UserID:       userID,
		CredentialID: credentialID,
	})
	if err != nil {
		t.Fatalf("build authorization header: %v", err)
	}
	if header != "Bearer mcp-secret-token" {
		t.Fatalf("expected bearer header, got %q", header)
	}
}

func TestServiceRejectsCredentialCreateWithoutSealer(t *testing.T) {
	service := NewService(&serviceRepo{}, nil)
	_, err := service.CreateCredential(context.Background(), CreateCredentialRequest{
		TenantID:        uuid.New(),
		UserID:          uuid.New(),
		Name:            "ops-token",
		CredentialType:  CredentialTypeMCPToken,
		CredentialValue: "secret",
	})
	if err == nil || !strings.Contains(err.Error(), "credential encryption key is required") {
		t.Fatalf("expected missing key error, got %v", err)
	}
}

type serviceRepo struct {
	createdCredential Credential
	credential        Credential
}

func (r *serviceRepo) CreateCredential(_ context.Context, req CreateCredentialStoreRequest) (Credential, error) {
	r.createdCredential = Credential{
		ID:             uuid.New(),
		TenantID:       req.TenantID,
		UserID:         req.UserID,
		Name:           req.Name,
		CredentialType: req.CredentialType,
		EncryptedValue: req.EncryptedValue,
		LastFour:       req.LastFour,
		Status:         "active",
	}
	return r.createdCredential, nil
}

func (r *serviceRepo) ListCredentials(context.Context, ListCredentialsRequest) ([]Credential, error) {
	return nil, nil
}

func (r *serviceRepo) GetCredential(context.Context, ResolveCredentialRequest) (Credential, error) {
	return r.credential, nil
}

func (r *serviceRepo) CreateTeamMCPServer(context.Context, CreateTeamMCPServerRequest) (MCPServer, error) {
	return MCPServer{}, nil
}

func (r *serviceRepo) ListTeamMCPServers(context.Context, TeamScopedRequest) ([]MCPServer, error) {
	return nil, nil
}

func (r *serviceRepo) DeleteTeamMCPServer(context.Context, DeleteTeamMCPServerRequest) error {
	return nil
}

func (r *serviceRepo) CreateEmployeeMCPBinding(context.Context, CreateEmployeeMCPBindingRequest) (MCPServer, error) {
	return MCPServer{}, nil
}

func (r *serviceRepo) ListEmployeeMCPBindings(context.Context, EmployeeScopedRequest) ([]MCPServer, error) {
	return nil, nil
}

func (r *serviceRepo) DeleteEmployeeMCPBinding(context.Context, DeleteEmployeeMCPBindingRequest) error {
	return nil
}

func (r *serviceRepo) ListEffectiveMCPServers(context.Context, EmployeeScopedRequest) ([]MCPServer, error) {
	return nil, nil
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/capability -run TestService -count=1
```

Expected: FAIL because package files do not exist.

- [ ] **Step 3: Add capability domain types**

Create `apps/control-plane/internal/capability/types.go`:

```go
package capability

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

const CredentialTypeMCPToken = "mcp_token"

var (
	ErrInvalidInput          = errors.New("invalid capability input")
	ErrNotFound              = errors.New("capability resource not found")
	ErrCredentialKeyMissing  = errors.New("credential encryption key is required")
	ErrCredentialTypeInvalid = errors.New("invalid credential type")
)

type Credential struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	UserID         uuid.UUID
	Name           string
	CredentialType string
	EncryptedValue string
	LastFour       string
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type MCPServer struct {
	ID                 uuid.UUID
	TenantID           uuid.UUID
	TeamID             *uuid.UUID
	DigitalEmployeeID  *uuid.UUID
	Name               string
	URL                string
	CredentialID       *uuid.UUID
	CredentialName     string
	CredentialType     string
	CredentialLastFour string
	Status             string
	SourceScope        string
	Inherited          bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type CreateCredentialRequest struct {
	TenantID        uuid.UUID
	UserID          uuid.UUID
	Name            string
	CredentialType  string
	CredentialValue string
}

type CreateCredentialStoreRequest struct {
	TenantID       uuid.UUID
	UserID         uuid.UUID
	Name           string
	CredentialType string
	EncryptedValue string
	LastFour       string
}

type ListCredentialsRequest struct {
	TenantID       uuid.UUID
	UserID         uuid.UUID
	CredentialType string
}

type ResolveCredentialRequest struct {
	TenantID     uuid.UUID
	UserID       uuid.UUID
	CredentialID uuid.UUID
}

type TeamScopedRequest struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
	TeamID   uuid.UUID
}

type EmployeeScopedRequest struct {
	TenantID          uuid.UUID
	UserID            uuid.UUID
	DigitalEmployeeID uuid.UUID
}

type CreateTeamMCPServerRequest struct {
	TenantID     uuid.UUID
	UserID       uuid.UUID
	TeamID       uuid.UUID
	Name         string
	URL          string
	CredentialID *uuid.UUID
}

type DeleteTeamMCPServerRequest struct {
	TenantID uuid.UUID
	TeamID   uuid.UUID
	ServerID uuid.UUID
}

type CreateEmployeeMCPBindingRequest struct {
	TenantID          uuid.UUID
	UserID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	Name              string
	URL               string
	CredentialID      *uuid.UUID
}

type DeleteEmployeeMCPBindingRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	BindingID         uuid.UUID
}
```

- [ ] **Step 4: Add AES-GCM credential sealer**

Create `apps/control-plane/internal/capability/crypto.go`:

```go
package capability

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const sealedCredentialPrefix = "aesgcm:v1:"

type CredentialSealer interface {
	Seal(plainText string) (string, error)
	Open(sealed string) (string, error)
}

type AESGCMCredentialSealer struct {
	aead cipher.AEAD
}

func NewAESGCMCredentialSealer(base64Key string) (*AESGCMCredentialSealer, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(base64Key))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid credential key encoding", ErrCredentialKeyMissing)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("%w: credential key must decode to 32 bytes", ErrCredentialKeyMissing)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &AESGCMCredentialSealer{aead: aead}, nil
}

func (s *AESGCMCredentialSealer) Seal(plainText string) (string, error) {
	if s == nil || s.aead == nil {
		return "", ErrCredentialKeyMissing
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	cipherText := s.aead.Seal(nil, nonce, []byte(plainText), nil)
	payload := append(nonce, cipherText...)
	return sealedCredentialPrefix + base64.StdEncoding.EncodeToString(payload), nil
}

func (s *AESGCMCredentialSealer) Open(sealed string) (string, error) {
	if s == nil || s.aead == nil {
		return "", ErrCredentialKeyMissing
	}
	encoded := strings.TrimPrefix(strings.TrimSpace(sealed), sealedCredentialPrefix)
	if encoded == sealed {
		return "", fmt.Errorf("%w: invalid sealed credential prefix", ErrInvalidInput)
	}
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("%w: invalid sealed credential payload", ErrInvalidInput)
	}
	nonceSize := s.aead.NonceSize()
	if len(payload) <= nonceSize {
		return "", fmt.Errorf("%w: sealed credential payload is too short", ErrInvalidInput)
	}
	plainText, err := s.aead.Open(nil, payload[:nonceSize], payload[nonceSize:], nil)
	if err != nil {
		return "", err
	}
	return string(plainText), nil
}
```

- [ ] **Step 5: Add service logic**

Create `apps/control-plane/internal/capability/service.go`:

```go
package capability

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

type Repository interface {
	CreateCredential(ctx context.Context, req CreateCredentialStoreRequest) (Credential, error)
	ListCredentials(ctx context.Context, req ListCredentialsRequest) ([]Credential, error)
	GetCredential(ctx context.Context, req ResolveCredentialRequest) (Credential, error)
	CreateTeamMCPServer(ctx context.Context, req CreateTeamMCPServerRequest) (MCPServer, error)
	ListTeamMCPServers(ctx context.Context, req TeamScopedRequest) ([]MCPServer, error)
	DeleteTeamMCPServer(ctx context.Context, req DeleteTeamMCPServerRequest) error
	CreateEmployeeMCPBinding(ctx context.Context, req CreateEmployeeMCPBindingRequest) (MCPServer, error)
	ListEmployeeMCPBindings(ctx context.Context, req EmployeeScopedRequest) ([]MCPServer, error)
	DeleteEmployeeMCPBinding(ctx context.Context, req DeleteEmployeeMCPBindingRequest) error
	ListEffectiveMCPServers(ctx context.Context, req EmployeeScopedRequest) ([]MCPServer, error)
}

type Service struct {
	repository Repository
	sealer     CredentialSealer
}

func NewService(repository Repository, sealer CredentialSealer) *Service {
	return &Service{repository: repository, sealer: sealer}
}

func (s *Service) CreateCredential(ctx context.Context, req CreateCredentialRequest) (Credential, error) {
	if err := validateCredentialRequest(req); err != nil {
		return Credential{}, err
	}
	if s == nil || s.repository == nil {
		return Credential{}, fmt.Errorf("%w: repository is required", ErrInvalidInput)
	}
	if s.sealer == nil {
		return Credential{}, ErrCredentialKeyMissing
	}
	sealed, err := s.sealer.Seal(req.CredentialValue)
	if err != nil {
		return Credential{}, err
	}
	created, err := s.repository.CreateCredential(ctx, CreateCredentialStoreRequest{
		TenantID:       req.TenantID,
		UserID:         req.UserID,
		Name:           strings.TrimSpace(req.Name),
		CredentialType: strings.TrimSpace(req.CredentialType),
		EncryptedValue: sealed,
		LastFour:       lastFour(req.CredentialValue),
	})
	if err != nil {
		return Credential{}, err
	}
	created.EncryptedValue = ""
	return created, nil
}

func (s *Service) ListCredentials(ctx context.Context, req ListCredentialsRequest) ([]Credential, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidInput)
	}
	items, err := s.repository.ListCredentials(ctx, req)
	if err != nil {
		return nil, err
	}
	for index := range items {
		items[index].EncryptedValue = ""
	}
	return items, nil
}

func (s *Service) BuildMCPAuthorizationHeader(ctx context.Context, req ResolveCredentialRequest) (string, error) {
	if s == nil || s.repository == nil {
		return "", fmt.Errorf("%w: repository is required", ErrInvalidInput)
	}
	if s.sealer == nil {
		return "", ErrCredentialKeyMissing
	}
	credential, err := s.repository.GetCredential(ctx, req)
	if err != nil {
		return "", err
	}
	if credential.CredentialType != CredentialTypeMCPToken {
		return "", ErrCredentialTypeInvalid
	}
	plain, err := s.sealer.Open(credential.EncryptedValue)
	if err != nil {
		return "", err
	}
	return "Bearer " + plain, nil
}

func (s *Service) CreateTeamMCPServer(ctx context.Context, req CreateTeamMCPServerRequest) (MCPServer, error) {
	if err := validateMCPInput(req.TenantID, req.UserID, req.Name, req.URL); err != nil {
		return MCPServer{}, err
	}
	return s.repository.CreateTeamMCPServer(ctx, req)
}

func (s *Service) ListTeamMCPServers(ctx context.Context, req TeamScopedRequest) ([]MCPServer, error) {
	return s.repository.ListTeamMCPServers(ctx, req)
}

func (s *Service) DeleteTeamMCPServer(ctx context.Context, req DeleteTeamMCPServerRequest) error {
	return s.repository.DeleteTeamMCPServer(ctx, req)
}

func (s *Service) CreateEmployeeMCPBinding(ctx context.Context, req CreateEmployeeMCPBindingRequest) (MCPServer, error) {
	if err := validateMCPInput(req.TenantID, req.UserID, req.Name, req.URL); err != nil {
		return MCPServer{}, err
	}
	return s.repository.CreateEmployeeMCPBinding(ctx, req)
}

func (s *Service) ListEmployeeMCPBindings(ctx context.Context, req EmployeeScopedRequest) ([]MCPServer, error) {
	return s.repository.ListEmployeeMCPBindings(ctx, req)
}

func (s *Service) DeleteEmployeeMCPBinding(ctx context.Context, req DeleteEmployeeMCPBindingRequest) error {
	return s.repository.DeleteEmployeeMCPBinding(ctx, req)
}

func (s *Service) ListEffectiveMCPServers(ctx context.Context, req EmployeeScopedRequest) ([]MCPServer, error) {
	return s.repository.ListEffectiveMCPServers(ctx, req)
}

func validateCredentialRequest(req CreateCredentialRequest) error {
	if req.TenantID == uuid.Nil || req.UserID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id and user_id are required", ErrInvalidInput)
	}
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("%w: credential name is required", ErrInvalidInput)
	}
	if strings.TrimSpace(req.CredentialType) != CredentialTypeMCPToken {
		return ErrCredentialTypeInvalid
	}
	if strings.TrimSpace(req.CredentialValue) == "" {
		return fmt.Errorf("%w: credential_value is required", ErrInvalidInput)
	}
	return nil
}

func validateMCPInput(tenantID, userID uuid.UUID, name, rawURL string) error {
	if tenantID == uuid.Nil || userID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id and user_id are required", ErrInvalidInput)
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: mcp name is required", ErrInvalidInput)
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%w: valid mcp url is required", ErrInvalidInput)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return fmt.Errorf("%w: mcp url must use http or https", ErrInvalidInput)
	}
	return nil
}

func lastFour(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 4 {
		return value
	}
	return value[len(value)-4:]
}
```

- [ ] **Step 6: Add pg repository adapter**

Create `apps/control-plane/internal/capability/pg_repository.go` with sqlc mappings:

```go
package capability

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgRepository struct {
	q *queries.Queries
}

func NewPgRepository(q *queries.Queries) Repository {
	return &PgRepository{q: q}
}

func (r *PgRepository) CreateCredential(ctx context.Context, req CreateCredentialStoreRequest) (Credential, error) {
	row, err := r.q.CreateUserCredential(ctx, queries.CreateUserCredentialParams{
		TenantID:       req.TenantID,
		UserID:         req.UserID,
		Name:           req.Name,
		CredentialType: req.CredentialType,
		EncryptedValue: req.EncryptedValue,
		LastFour:       req.LastFour,
	})
	if err != nil {
		return Credential{}, err
	}
	return credentialFromQuery(row), nil
}

func (r *PgRepository) ListCredentials(ctx context.Context, req ListCredentialsRequest) ([]Credential, error) {
	rows, err := r.q.ListUserCredentials(ctx, queries.ListUserCredentialsParams{
		TenantID:       req.TenantID,
		UserID:         req.UserID,
		CredentialType: textPtr(req.CredentialType),
	})
	if err != nil {
		return nil, err
	}
	items := make([]Credential, 0, len(rows))
	for _, row := range rows {
		items = append(items, credentialFromQuery(row))
	}
	return items, nil
}

func (r *PgRepository) GetCredential(ctx context.Context, req ResolveCredentialRequest) (Credential, error) {
	row, err := r.q.GetUserCredential(ctx, queries.GetUserCredentialParams{
		TenantID: req.TenantID,
		UserID:   req.UserID,
		ID:       req.CredentialID,
	})
	if err != nil {
		return Credential{}, mapCapabilityNoRows(err)
	}
	return credentialFromQuery(row), nil
}

func (r *PgRepository) CreateTeamMCPServer(ctx context.Context, req CreateTeamMCPServerRequest) (MCPServer, error) {
	row, err := r.q.CreateTeamMCPServer(ctx, queries.CreateTeamMCPServerParams{
		TenantID:     req.TenantID,
		TeamID:       req.TeamID,
		Name:         req.Name,
		Url:          req.URL,
		CredentialID: uuidPtrToPgtype(req.CredentialID),
		CreatedBy:    uuidToPgtype(req.UserID),
	})
	if err != nil {
		return MCPServer{}, err
	}
	teamID := row.TeamID
	return MCPServer{
		ID:           row.ID,
		TenantID:     row.TenantID,
		TeamID:       &teamID,
		Name:         row.Name,
		URL:          row.Url,
		CredentialID: pgtypeToUUIDPtr(row.CredentialID),
		Status:       row.Status,
		SourceScope:  "team",
		Inherited:    true,
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
	}, nil
}

func (r *PgRepository) ListTeamMCPServers(ctx context.Context, req TeamScopedRequest) ([]MCPServer, error) {
	rows, err := r.q.ListTeamMCPServers(ctx, queries.ListTeamMCPServersParams{TenantID: req.TenantID, TeamID: req.TeamID})
	if err != nil {
		return nil, err
	}
	items := make([]MCPServer, 0, len(rows))
	for _, row := range rows {
		teamID := row.TeamID
		items = append(items, MCPServer{
			ID:                 row.ID,
			TenantID:           row.TenantID,
			TeamID:             &teamID,
			Name:               row.Name,
			URL:                row.Url,
			CredentialID:       pgtypeToUUIDPtr(row.CredentialID),
			CredentialName:     row.CredentialName,
			CredentialType:     row.CredentialType,
			CredentialLastFour: row.CredentialLastFour,
			Status:             row.Status,
			SourceScope:        "team",
			Inherited:          true,
			CreatedAt:          row.CreatedAt.Time,
			UpdatedAt:          row.UpdatedAt.Time,
		})
	}
	return items, nil
}

func (r *PgRepository) DeleteTeamMCPServer(ctx context.Context, req DeleteTeamMCPServerRequest) error {
	return r.q.DeleteTeamMCPServer(ctx, queries.DeleteTeamMCPServerParams{TenantID: req.TenantID, TeamID: req.TeamID, ID: req.ServerID})
}

func (r *PgRepository) CreateEmployeeMCPBinding(ctx context.Context, req CreateEmployeeMCPBindingRequest) (MCPServer, error) {
	row, err := r.q.CreateDigitalEmployeeMCPBinding(ctx, queries.CreateDigitalEmployeeMCPBindingParams{
		TenantID:          req.TenantID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		Name:              req.Name,
		Url:               req.URL,
		CredentialID:      uuidPtrToPgtype(req.CredentialID),
		CreatedBy:         uuidToPgtype(req.UserID),
	})
	if err != nil {
		return MCPServer{}, err
	}
	employeeID := row.DigitalEmployeeID
	return MCPServer{
		ID:                row.ID,
		TenantID:          row.TenantID,
		DigitalEmployeeID: &employeeID,
		Name:              row.Name,
		URL:               row.Url,
		CredentialID:      pgtypeToUUIDPtr(row.CredentialID),
		Status:            row.Status,
		SourceScope:       "employee",
		Inherited:         false,
		CreatedAt:         row.CreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
	}, nil
}

func (r *PgRepository) ListEmployeeMCPBindings(ctx context.Context, req EmployeeScopedRequest) ([]MCPServer, error) {
	rows, err := r.q.ListDigitalEmployeeMCPBindings(ctx, queries.ListDigitalEmployeeMCPBindingsParams{TenantID: req.TenantID, DigitalEmployeeID: req.DigitalEmployeeID})
	if err != nil {
		return nil, err
	}
	items := make([]MCPServer, 0, len(rows))
	for _, row := range rows {
		employeeID := row.DigitalEmployeeID
		items = append(items, MCPServer{
			ID:                 row.ID,
			TenantID:           row.TenantID,
			DigitalEmployeeID:  &employeeID,
			Name:               row.Name,
			URL:                row.Url,
			CredentialID:       pgtypeToUUIDPtr(row.CredentialID),
			CredentialName:     row.CredentialName,
			CredentialType:     row.CredentialType,
			CredentialLastFour: row.CredentialLastFour,
			Status:             row.Status,
			SourceScope:        "employee",
			Inherited:          false,
			CreatedAt:          row.CreatedAt.Time,
			UpdatedAt:          row.UpdatedAt.Time,
		})
	}
	return items, nil
}

func (r *PgRepository) DeleteEmployeeMCPBinding(ctx context.Context, req DeleteEmployeeMCPBindingRequest) error {
	return r.q.DeleteDigitalEmployeeMCPBinding(ctx, queries.DeleteDigitalEmployeeMCPBindingParams{TenantID: req.TenantID, DigitalEmployeeID: req.DigitalEmployeeID, ID: req.BindingID})
}

func (r *PgRepository) ListEffectiveMCPServers(ctx context.Context, req EmployeeScopedRequest) ([]MCPServer, error) {
	rows, err := r.q.ListEffectiveMCPServersForEmployee(ctx, queries.ListEffectiveMCPServersForEmployeeParams{TenantID: req.TenantID, DigitalEmployeeID: req.DigitalEmployeeID})
	if err != nil {
		return nil, err
	}
	items := make([]MCPServer, 0, len(rows))
	for _, row := range rows {
		items = append(items, MCPServer{
			ID:                 row.ID,
			TenantID:           row.TenantID,
			TeamID:             pgtypeToUUIDPtr(row.TeamID),
			DigitalEmployeeID:  &row.DigitalEmployeeID,
			Name:               row.Name,
			URL:                row.Url,
			CredentialID:       pgtypeToUUIDPtr(row.CredentialID),
			CredentialName:     row.CredentialName,
			CredentialType:     row.CredentialType,
			CredentialLastFour: row.CredentialLastFour,
			Status:             row.Status,
			SourceScope:        row.SourceScope,
			Inherited:          row.Inherited,
			CreatedAt:          row.CreatedAt.Time,
			UpdatedAt:          row.UpdatedAt.Time,
		})
	}
	return items, nil
}

func credentialFromQuery(row queries.UserCredential) Credential {
	return Credential{
		ID:             row.ID,
		TenantID:       row.TenantID,
		UserID:         row.UserID,
		Name:           row.Name,
		CredentialType: row.CredentialType,
		EncryptedValue: row.EncryptedValue,
		LastFour:       row.LastFour,
		Status:         row.Status,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}
}

func mapCapabilityNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func uuidToPgtype(value uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: value, Valid: value != uuid.Nil}
}

func uuidPtrToPgtype(value *uuid.UUID) pgtype.UUID {
	if value == nil || *value == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *value, Valid: true}
}

func pgtypeToUUIDPtr(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	parsed := value.Bytes
	return &parsed
}

func textPtr(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

var _ Repository = (*PgRepository)(nil)
var _ = fmt.Sprintf
```

- [ ] **Step 7: Run service tests**

Run:

```bash
go test ./apps/control-plane/internal/capability -run TestService -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/control-plane/internal/capability
git commit -m "feat: add capability service"
```

## Task 4: Skill Binding Service And Routes

**Files:**
- Modify: `apps/control-plane/internal/skill/types.go`
- Modify: `apps/control-plane/internal/skill/service.go`
- Modify: `apps/control-plane/internal/skill/pg_repository.go`
- Modify: `apps/control-plane/internal/skill/handler.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Test: `apps/control-plane/internal/skill/service_test.go`
- Test: `apps/control-plane/internal/api/skill_routes_test.go`

- [ ] **Step 1: Write failing skill merge service test**

Append to `apps/control-plane/internal/skill/service_test.go`:

```go
func TestServiceListsEffectiveEmployeeSkillsWithTeamInheritedFirst(t *testing.T) {
	repo := &serviceTestRepository{
		effectiveSkills: []EffectiveEmployeeSkill{
			{
				Skill: Skill{ID: uuid.New(), Name: "diagnose", Slug: "diagnose", Status: SkillStatusInstalled},
				SourceScope: "team",
				Inherited:   true,
				ReadOnly:    true,
			},
			{
				Skill: Skill{ID: uuid.New(), Name: "personal-review", Slug: "personal-review", Status: SkillStatusInstalled},
				SourceScope: "employee",
				Inherited:   false,
				ReadOnly:    false,
			},
		},
	}
	service := NewService(repo)
	items, err := service.ListEffectiveEmployeeSkills(context.Background(), ListEffectiveEmployeeSkillsRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("list effective employee skills: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(items))
	}
	if !items[0].Inherited || !items[0].ReadOnly || items[0].SourceScope != "team" {
		t.Fatalf("expected first skill to be readonly inherited team skill, got %#v", items[0])
	}
	if items[1].Inherited || items[1].ReadOnly || items[1].SourceScope != "employee" {
		t.Fatalf("expected second skill to be editable employee skill, got %#v", items[1])
	}
}
```

Extend the existing `serviceTestRepository` in the same file with methods and fields:

```go
effectiveSkills []EffectiveEmployeeSkill

func (r *serviceTestRepository) BindSkillToTeam(context.Context, BindTeamSkillRequest) (*Skill, error) {
	return nil, nil
}

func (r *serviceTestRepository) UnbindSkillFromTeam(context.Context, BindTeamSkillRequest) error {
	return nil
}

func (r *serviceTestRepository) ListTeamSkills(context.Context, ListTeamSkillsRequest) ([]*Skill, error) {
	return nil, nil
}

func (r *serviceTestRepository) BindSkillToEmployee(context.Context, BindEmployeeSkillRequest) (*Skill, error) {
	return nil, nil
}

func (r *serviceTestRepository) UnbindSkillFromEmployee(context.Context, BindEmployeeSkillRequest) error {
	return nil
}

func (r *serviceTestRepository) ListEffectiveEmployeeSkills(context.Context, ListEffectiveEmployeeSkillsRequest) ([]EffectiveEmployeeSkill, error) {
	return r.effectiveSkills, nil
}
```

- [ ] **Step 2: Run service test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/skill -run TestServiceListsEffectiveEmployeeSkillsWithTeamInheritedFirst -count=1
```

Expected: FAIL because request and effective skill types do not exist.

- [ ] **Step 3: Add skill binding domain types**

In `apps/control-plane/internal/skill/types.go`, add:

```go
type BindTeamSkillRequest struct {
	TenantID uuid.UUID
	TeamID   uuid.UUID
	SkillID  uuid.UUID
}

type ListTeamSkillsRequest struct {
	TenantID uuid.UUID
	TeamID   uuid.UUID
}

type BindEmployeeSkillRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	SkillID           uuid.UUID
}

type ListEffectiveEmployeeSkillsRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
}

type EffectiveEmployeeSkill struct {
	Skill       Skill
	SourceScope string
	Inherited   bool
	ReadOnly    bool
}
```

- [ ] **Step 4: Extend repository interface and service methods**

In `apps/control-plane/internal/skill/service.go`, extend `Repository`:

```go
BindSkillToTeam(ctx context.Context, req BindTeamSkillRequest) (*Skill, error)
UnbindSkillFromTeam(ctx context.Context, req BindTeamSkillRequest) error
ListTeamSkills(ctx context.Context, req ListTeamSkillsRequest) ([]*Skill, error)
BindSkillToEmployee(ctx context.Context, req BindEmployeeSkillRequest) (*Skill, error)
UnbindSkillFromEmployee(ctx context.Context, req BindEmployeeSkillRequest) error
ListEffectiveEmployeeSkills(ctx context.Context, req ListEffectiveEmployeeSkillsRequest) ([]EffectiveEmployeeSkill, error)
```

Add service methods:

```go
func (s *Service) BindSkillToTeam(ctx context.Context, req BindTeamSkillRequest) (*Skill, error) {
	if err := validateTeamSkillRequest(req); err != nil {
		return nil, err
	}
	return s.repository.BindSkillToTeam(ctx, req)
}

func (s *Service) UnbindSkillFromTeam(ctx context.Context, req BindTeamSkillRequest) error {
	if err := validateTeamSkillRequest(req); err != nil {
		return err
	}
	return s.repository.UnbindSkillFromTeam(ctx, req)
}

func (s *Service) ListTeamSkills(ctx context.Context, req ListTeamSkillsRequest) ([]*Skill, error) {
	if req.TenantID == uuid.Nil || req.TeamID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id and team_id are required", ErrInvalidInput)
	}
	return s.repository.ListTeamSkills(ctx, req)
}

func (s *Service) BindSkillToEmployee(ctx context.Context, req BindEmployeeSkillRequest) (*Skill, error) {
	if err := validateEmployeeSkillRequest(req); err != nil {
		return nil, err
	}
	return s.repository.BindSkillToEmployee(ctx, req)
}

func (s *Service) UnbindSkillFromEmployee(ctx context.Context, req BindEmployeeSkillRequest) error {
	if err := validateEmployeeSkillRequest(req); err != nil {
		return err
	}
	return s.repository.UnbindSkillFromEmployee(ctx, req)
}

func (s *Service) ListEffectiveEmployeeSkills(ctx context.Context, req ListEffectiveEmployeeSkillsRequest) ([]EffectiveEmployeeSkill, error) {
	if req.TenantID == uuid.Nil || req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id and digital_employee_id are required", ErrInvalidInput)
	}
	return s.repository.ListEffectiveEmployeeSkills(ctx, req)
}

func validateTeamSkillRequest(req BindTeamSkillRequest) error {
	if req.TenantID == uuid.Nil || req.TeamID == uuid.Nil || req.SkillID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id, team_id and skill_id are required", ErrInvalidInput)
	}
	return nil
}

func validateEmployeeSkillRequest(req BindEmployeeSkillRequest) error {
	if req.TenantID == uuid.Nil || req.DigitalEmployeeID == uuid.Nil || req.SkillID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id, digital_employee_id and skill_id are required", ErrInvalidInput)
	}
	return nil
}
```

- [ ] **Step 5: Implement pg repository skill bindings**

Add these methods to `apps/control-plane/internal/skill/pg_repository.go`:

```go
func (r *PgRepository) BindSkillToTeam(ctx context.Context, req BindTeamSkillRequest) (*Skill, error) {
	if _, err := r.db.Exec(ctx, `
INSERT INTO skill_team_bindings (tenant_id, skill_id, team_id)
VALUES ($1, $2, $3)
ON CONFLICT (tenant_id, skill_id, team_id) DO NOTHING`, req.TenantID, req.SkillID, req.TeamID); err != nil {
		return nil, err
	}
	return r.GetSkill(ctx, GetSkillRequest{TenantID: req.TenantID, SkillID: req.SkillID})
}

func (r *PgRepository) UnbindSkillFromTeam(ctx context.Context, req BindTeamSkillRequest) error {
	_, err := r.db.Exec(ctx, `
DELETE FROM skill_team_bindings
WHERE tenant_id = $1 AND skill_id = $2 AND team_id = $3`, req.TenantID, req.SkillID, req.TeamID)
	return err
}

func (r *PgRepository) ListTeamSkills(ctx context.Context, req ListTeamSkillsRequest) ([]*Skill, error) {
	rows, err := r.db.Query(ctx, `
SELECT s.id, s.tenant_id, s.slug, s.name, s.description, s.version, s.source, s.risk_level, s.status, s.icon_key, s.color_token, s.tags, s.created_at, s.updated_at
FROM skill_team_bindings stb
JOIN skills s ON s.tenant_id = stb.tenant_id AND s.id = stb.skill_id AND s.deleted_at IS NULL
WHERE stb.tenant_id = $1 AND stb.team_id = $2
ORDER BY s.name ASC`, req.TenantID, req.TeamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var skills []*Skill
	for rows.Next() {
		item := &Skill{}
		if err := rows.Scan(
			&item.ID, &item.TenantID, &item.Slug, &item.Name, &item.Description,
			&item.Version, &item.Source, &item.RiskLevel, &item.Status,
			&item.IconKey, &item.ColorToken, &item.Tags, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		skills = append(skills, item)
	}
	return skills, rows.Err()
}

func (r *PgRepository) BindSkillToEmployee(ctx context.Context, req BindEmployeeSkillRequest) (*Skill, error) {
	if _, err := r.db.Exec(ctx, `
INSERT INTO skill_agent_bindings (tenant_id, skill_id, digital_employee_id, status, updated_at)
VALUES ($1, $2, $3, 'enabled', NOW())
ON CONFLICT (tenant_id, skill_id, digital_employee_id)
DO UPDATE SET status = 'enabled', updated_at = NOW()`, req.TenantID, req.SkillID, req.DigitalEmployeeID); err != nil {
		return nil, err
	}
	return r.GetSkill(ctx, GetSkillRequest{TenantID: req.TenantID, SkillID: req.SkillID})
}

func (r *PgRepository) UnbindSkillFromEmployee(ctx context.Context, req BindEmployeeSkillRequest) error {
	_, err := r.db.Exec(ctx, `
DELETE FROM skill_agent_bindings
WHERE tenant_id = $1 AND skill_id = $2 AND digital_employee_id = $3`, req.TenantID, req.SkillID, req.DigitalEmployeeID)
	return err
}

func (r *PgRepository) ListEffectiveEmployeeSkills(ctx context.Context, req ListEffectiveEmployeeSkillsRequest) ([]EffectiveEmployeeSkill, error) {
	rows, err := r.db.Query(ctx, `
WITH target_employee AS (
    SELECT tenant_id, id AS digital_employee_id, team_id
    FROM digital_employees
    WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL
)
SELECT s.id, s.tenant_id, s.slug, s.name, s.description, s.version, s.source, s.risk_level, s.status, s.icon_key, s.color_token, s.tags, s.created_at, s.updated_at,
       'team'::text AS source_scope, true AS inherited, true AS read_only
FROM target_employee te
JOIN skill_team_bindings stb ON stb.tenant_id = te.tenant_id AND stb.team_id = te.team_id
JOIN skills s ON s.tenant_id = stb.tenant_id AND s.id = stb.skill_id AND s.deleted_at IS NULL
UNION
SELECT s.id, s.tenant_id, s.slug, s.name, s.description, s.version, s.source, s.risk_level, s.status, s.icon_key, s.color_token, s.tags, s.created_at, s.updated_at,
       'employee'::text AS source_scope, false AS inherited, false AS read_only
FROM skill_agent_bindings sab
JOIN skills s ON s.tenant_id = sab.tenant_id AND s.id = sab.skill_id AND s.deleted_at IS NULL
WHERE sab.tenant_id = $1 AND sab.digital_employee_id = $2 AND sab.status = 'enabled'
ORDER BY inherited DESC, name ASC`, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []EffectiveEmployeeSkill
	for rows.Next() {
		item := EffectiveEmployeeSkill{}
		if err := rows.Scan(
			&item.Skill.ID,
			&item.Skill.TenantID,
			&item.Skill.Slug,
			&item.Skill.Name,
			&item.Skill.Description,
			&item.Skill.Version,
			&item.Skill.Source,
			&item.Skill.RiskLevel,
			&item.Skill.Status,
			&item.Skill.IconKey,
			&item.Skill.ColorToken,
			&item.Skill.Tags,
			&item.Skill.CreatedAt,
			&item.Skill.UpdatedAt,
			&item.SourceScope,
			&item.Inherited,
			&item.ReadOnly,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
```

- [ ] **Step 6: Add handler routes**

In `apps/control-plane/internal/skill/handler.go`, extend `HandlerService` with the new service methods and add response structs:

```go
type effectiveEmployeeSkillResponse struct {
	Skill       skillResponse `json:"skill"`
	SourceScope string        `json:"source_scope"`
	Inherited   bool          `json:"inherited"`
	ReadOnly    bool          `json:"read_only"`
}
```

Add handlers:

```go
func (h *HTTPHandler) ListTeamSkills(w http.ResponseWriter, r *http.Request) {
	teamID, err := uuid.Parse(chi.URLParam(r, "teamId"))
	if err != nil || teamID == uuid.Nil {
		http.Error(w, "invalid team id", http.StatusBadRequest)
		return
	}
	tenantID, ok := h.authorizeSkillAction(w, r, authz.ActionTeamCapabilityBind, authz.ResourceRef{Type: authz.ResourceTeam, ID: teamID.String()}, "team skill read")
	if !ok {
		return
	}
	items, err := h.service.ListTeamSkills(r.Context(), ListTeamSkillsRequest{TenantID: tenantID, TeamID: teamID})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, skillResponses(items))
}

func (h *HTTPHandler) BindTeamSkill(w http.ResponseWriter, r *http.Request) {
	teamID, err := uuid.Parse(chi.URLParam(r, "teamId"))
	if err != nil || teamID == uuid.Nil {
		http.Error(w, "invalid team id", http.StatusBadRequest)
		return
	}
	tenantID, ok := h.authorizeSkillAction(w, r, authz.ActionTeamCapabilityBind, authz.ResourceRef{Type: authz.ResourceTeam, ID: teamID.String()}, "team skill bind")
	if !ok {
		return
	}
	var req struct {
		SkillID uuid.UUID `json:"skill_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	item, err := h.service.BindSkillToTeam(r.Context(), BindTeamSkillRequest{TenantID: tenantID, TeamID: teamID, SkillID: req.SkillID})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, skillResponseFromDomain(item))
}

func (h *HTTPHandler) UnbindTeamSkill(w http.ResponseWriter, r *http.Request) {
	teamID, err := uuid.Parse(chi.URLParam(r, "teamId"))
	if err != nil || teamID == uuid.Nil {
		http.Error(w, "invalid team id", http.StatusBadRequest)
		return
	}
	skillID, ok := skillIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeSkillAction(w, r, authz.ActionTeamCapabilityUnbind, authz.ResourceRef{Type: authz.ResourceTeam, ID: teamID.String()}, "team skill unbind")
	if !ok {
		return
	}
	if err := h.service.UnbindSkillFromTeam(r.Context(), BindTeamSkillRequest{TenantID: tenantID, TeamID: teamID, SkillID: skillID}); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

Add employee handlers using `employeeIdFromRequest` local helper or a new `uuid.Parse(chi.URLParam(r, "employeeId"))` block:

```go
func effectiveEmployeeSkillResponses(items []EffectiveEmployeeSkill) []effectiveEmployeeSkillResponse {
	responses := make([]effectiveEmployeeSkillResponse, 0, len(items))
	for _, item := range items {
		skillCopy := item.Skill
		responses = append(responses, effectiveEmployeeSkillResponse{
			Skill:       skillResponseFromDomain(&skillCopy),
			SourceScope: item.SourceScope,
			Inherited:   item.Inherited,
			ReadOnly:    item.ReadOnly,
		})
	}
	return responses
}

func (h *HTTPHandler) ListEffectiveEmployeeSkills(w http.ResponseWriter, r *http.Request) {
	employeeID, err := uuid.Parse(chi.URLParam(r, "employeeId"))
	if err != nil || employeeID == uuid.Nil {
		http.Error(w, "invalid employee id", http.StatusBadRequest)
		return
	}
	tenantID, ok := h.authorizeSkillAction(w, r, authz.ActionEmployeeRead, authz.ResourceRef{Type: authz.ResourceEmployee, ID: employeeID.String()}, "employee effective skills read")
	if !ok {
		return
	}
	items, err := h.service.ListEffectiveEmployeeSkills(r.Context(), ListEffectiveEmployeeSkillsRequest{TenantID: tenantID, DigitalEmployeeID: employeeID})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, effectiveEmployeeSkillResponses(items))
}

func (h *HTTPHandler) BindEmployeeSkill(w http.ResponseWriter, r *http.Request) {
	employeeID, err := uuid.Parse(chi.URLParam(r, "employeeId"))
	if err != nil || employeeID == uuid.Nil {
		http.Error(w, "invalid employee id", http.StatusBadRequest)
		return
	}
	tenantID, ok := h.authorizeSkillAction(w, r, authz.ActionEmployeeCapabilityEdit, authz.ResourceRef{Type: authz.ResourceEmployee, ID: employeeID.String()}, "employee skill bind")
	if !ok {
		return
	}
	var req struct {
		SkillID uuid.UUID `json:"skill_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	item, err := h.service.BindSkillToEmployee(r.Context(), BindEmployeeSkillRequest{TenantID: tenantID, DigitalEmployeeID: employeeID, SkillID: req.SkillID})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, skillResponseFromDomain(item))
}

func (h *HTTPHandler) UnbindEmployeeSkill(w http.ResponseWriter, r *http.Request) {
	employeeID, err := uuid.Parse(chi.URLParam(r, "employeeId"))
	if err != nil || employeeID == uuid.Nil {
		http.Error(w, "invalid employee id", http.StatusBadRequest)
		return
	}
	skillID, ok := skillIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeSkillAction(w, r, authz.ActionEmployeeCapabilityEdit, authz.ResourceRef{Type: authz.ResourceEmployee, ID: employeeID.String()}, "employee skill unbind")
	if !ok {
		return
	}
	if err := h.service.UnbindSkillFromEmployee(r.Context(), BindEmployeeSkillRequest{TenantID: tenantID, DigitalEmployeeID: employeeID, SkillID: skillID}); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 7: Register routes**

In `apps/control-plane/internal/api/server.go`, inside the skill handler group add:

```go
r.Get("/teams/{teamId}/skills", s.skillHandler.ListTeamSkills)
r.Post("/teams/{teamId}/skills", s.skillHandler.BindTeamSkill)
r.Delete("/teams/{teamId}/skills/{skillId}", s.skillHandler.UnbindTeamSkill)
r.Get("/digital-employees/{employeeId}/skills", s.skillHandler.ListEffectiveEmployeeSkills)
r.Post("/digital-employees/{employeeId}/skills", s.skillHandler.BindEmployeeSkill)
r.Delete("/digital-employees/{employeeId}/skills/{skillId}", s.skillHandler.UnbindEmployeeSkill)
```

- [ ] **Step 8: Run skill tests**

Run:

```bash
go test ./apps/control-plane/internal/skill -run TestServiceListsEffectiveEmployeeSkillsWithTeamInheritedFirst -count=1
go test ./apps/control-plane/internal/api -run TestSkillRoutes -count=1
```

Expected: PASS after adding route coverage to `skill_routes_test.go`.

- [ ] **Step 9: Commit**

```bash
git add apps/control-plane/internal/skill apps/control-plane/internal/api/server.go apps/control-plane/internal/api/skill_routes_test.go
git commit -m "feat: add team and employee skill bindings"
```

## Task 5: Employee Instruction Files Backend

**Files:**
- Create: `apps/control-plane/internal/storage/queries/employee_instructions.sql`
- Generated: `apps/control-plane/internal/storage/queries/employee_instructions.sql.go`
- Modify: `apps/control-plane/internal/employee/types.go`
- Modify: `apps/control-plane/internal/employee/repository.go`
- Modify: `apps/control-plane/internal/employee/pg_repository.go`
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/control-plane/internal/employee/handler.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Test: `apps/control-plane/internal/employee/service_test.go`
- Test: `apps/control-plane/internal/api/employee_routes_test.go`

- [ ] **Step 1: Write failing service test**

Append to `apps/control-plane/internal/employee/service_test.go`:

```go
func TestServiceUpsertsInstructionFileWithNormalizedPathAndChecksum(t *testing.T) {
	repo := &employeeServiceRepository{}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	employeeID := uuid.New()

	file, err := service.UpsertInstructionFile(context.Background(), UpsertInstructionFileRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		Path:              "docs/../AGENTS.md",
		Content:           "# 工作原则\n\n只读取当前任务需要的上下文。",
	})
	if err != nil {
		t.Fatalf("upsert instruction file: %v", err)
	}
	if file.Path != "AGENTS.md" {
		t.Fatalf("expected normalized AGENTS.md path, got %q", file.Path)
	}
	if file.SizeBytes == 0 || file.ChecksumSHA256 == "" {
		t.Fatalf("expected size and checksum, got %#v", file)
	}
}

func TestServiceRejectsUnsafeInstructionPath(t *testing.T) {
	service, err := NewService(&employeeServiceRepository{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = service.UpsertInstructionFile(context.Background(), UpsertInstructionFileRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: uuid.New(),
		Path:              "../AGENTS.md",
		Content:           "# bad",
	})
	if err == nil {
		t.Fatal("expected unsafe path to be rejected")
	}
}
```

Extend the test repository with:

```go
func (r *employeeServiceRepository) ListInstructionFiles(context.Context, ListInstructionFilesRequest) ([]InstructionFile, error) {
	return nil, nil
}

func (r *employeeServiceRepository) UpsertInstructionFile(_ context.Context, req UpsertInstructionFileStoreRequest) (InstructionFile, error) {
	return InstructionFile{
		ID:             uuid.New(),
		TenantID:       req.TenantID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		Path:           req.Path,
		Content:        req.Content,
		SizeBytes:      req.SizeBytes,
		ChecksumSHA256: req.ChecksumSHA256,
	}, nil
}
```

- [ ] **Step 2: Run the service test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/employee -run TestService.*Instruction -count=1
```

Expected: FAIL because instruction types and methods do not exist.

- [ ] **Step 3: Add instruction domain types**

In `apps/control-plane/internal/employee/types.go`, add:

```go
type InstructionFile struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	DigitalEmployeeID uuid.UUID
	Path           string
	Content        string
	SizeBytes      int64
	ChecksumSHA256 string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ListInstructionFilesRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
}

type UpsertInstructionFileRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	Path              string
	Content           string
}

type UpsertInstructionFileStoreRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	Path              string
	Content           string
	SizeBytes         int64
	ChecksumSHA256    string
}
```

- [ ] **Step 4: Extend repository and service**

In `apps/control-plane/internal/employee/repository.go`, add:

```go
ListInstructionFiles(ctx context.Context, req ListInstructionFilesRequest) ([]InstructionFile, error)
UpsertInstructionFile(ctx context.Context, req UpsertInstructionFileStoreRequest) (InstructionFile, error)
```

In `apps/control-plane/internal/employee/service.go`, add:

```go
func (s *Service) ListInstructionFiles(ctx context.Context, req ListInstructionFilesRequest) ([]InstructionFile, error) {
	if req.TenantID == uuid.Nil || req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id and digital_employee_id are required", ErrInvalidInput)
	}
	return s.repository.ListInstructionFiles(ctx, req)
}

func (s *Service) UpsertInstructionFile(ctx context.Context, req UpsertInstructionFileRequest) (InstructionFile, error) {
	if req.TenantID == uuid.Nil || req.DigitalEmployeeID == uuid.Nil {
		return InstructionFile{}, fmt.Errorf("%w: tenant_id and digital_employee_id are required", ErrInvalidInput)
	}
	pathValue := normalizeInstructionPath(req.Path)
	if pathValue == "" {
		return InstructionFile{}, fmt.Errorf("%w: instruction path is invalid", ErrInvalidInput)
	}
	sum := sha256.Sum256([]byte(req.Content))
	return s.repository.UpsertInstructionFile(ctx, UpsertInstructionFileStoreRequest{
		TenantID:          req.TenantID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		Path:              pathValue,
		Content:           req.Content,
		SizeBytes:         int64(len([]byte(req.Content))),
		ChecksumSHA256:    hex.EncodeToString(sum[:]),
	})
}

func normalizeInstructionPath(value string) string {
	clean := path.Clean(strings.TrimSpace(strings.ReplaceAll(value, "\\", "/")))
	if clean == "." || clean == "" || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return ""
	}
	if !strings.HasSuffix(clean, ".md") && !strings.HasSuffix(clean, ".txt") {
		return ""
	}
	return clean
}
```

Add imports to `service.go`:

```go
import (
	"crypto/sha256"
	"encoding/hex"
	"path"
)
```

- [ ] **Step 5: Implement pg repository methods**

Create `apps/control-plane/internal/storage/queries/employee_instructions.sql`:

```sql
-- name: ListDigitalEmployeeInstructionFiles :many
SELECT id,
    tenant_id,
    digital_employee_id,
    path,
    content,
    size_bytes,
    checksum_sha256,
    created_at,
    updated_at
FROM digital_employee_instruction_files
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND deleted_at IS NULL
ORDER BY CASE
    WHEN path = 'AGENTS.md' THEN 0
    WHEN path = 'SOUL.md' THEN 1
    ELSE 2
END,
path ASC;

-- name: UpsertDigitalEmployeeInstructionFile :one
INSERT INTO digital_employee_instruction_files (
    tenant_id,
    digital_employee_id,
    path,
    content,
    size_bytes,
    checksum_sha256,
    updated_at
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('path')::text,
    sqlc.arg('content')::text,
    sqlc.arg('size_bytes')::bigint,
    sqlc.arg('checksum_sha256')::varchar,
    NOW()
)
ON CONFLICT (tenant_id, digital_employee_id, path) WHERE deleted_at IS NULL
DO UPDATE SET content = EXCLUDED.content,
    size_bytes = EXCLUDED.size_bytes,
    checksum_sha256 = EXCLUDED.checksum_sha256,
    updated_at = NOW()
RETURNING id,
    tenant_id,
    digital_employee_id,
    path,
    content,
    size_bytes,
    checksum_sha256,
    created_at,
    updated_at;
```

Regenerate sqlc before editing the repository:

```bash
cd apps/control-plane && make generate-sqlc
```

In `apps/control-plane/internal/employee/pg_repository.go`, add:

```go
func (r *PgRepository) ListInstructionFiles(ctx context.Context, req ListInstructionFilesRequest) ([]InstructionFile, error) {
	rows, err := r.q.ListDigitalEmployeeInstructionFiles(ctx, queries.ListDigitalEmployeeInstructionFilesParams{
		TenantID:          req.TenantID,
		DigitalEmployeeID: req.DigitalEmployeeID,
	})
	if err != nil {
		return nil, err
	}
	files := make([]InstructionFile, 0, len(rows))
	for _, row := range rows {
		files = append(files, InstructionFile{
			ID:                row.ID,
			TenantID:          row.TenantID,
			DigitalEmployeeID: row.DigitalEmployeeID,
			Path:              row.Path,
			Content:           row.Content,
			SizeBytes:         row.SizeBytes,
			ChecksumSHA256:    row.ChecksumSha256,
			CreatedAt:         row.CreatedAt.Time,
			UpdatedAt:         row.UpdatedAt.Time,
		})
	}
	return files, nil
}

func (r *PgRepository) UpsertInstructionFile(ctx context.Context, req UpsertInstructionFileStoreRequest) (InstructionFile, error) {
	row, err := r.q.UpsertDigitalEmployeeInstructionFile(ctx, queries.UpsertDigitalEmployeeInstructionFileParams{
		TenantID:          req.TenantID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		Path:              req.Path,
		Content:           req.Content,
		SizeBytes:         req.SizeBytes,
		ChecksumSha256:    req.ChecksumSHA256,
	})
	if err != nil {
		return InstructionFile{}, err
	}
	return InstructionFile{
		ID:                row.ID,
		TenantID:          row.TenantID,
		DigitalEmployeeID: row.DigitalEmployeeID,
		Path:              row.Path,
		Content:           row.Content,
		SizeBytes:         row.SizeBytes,
		ChecksumSHA256:    row.ChecksumSha256,
		CreatedAt:         row.CreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
	}, nil
}
```

- [ ] **Step 6: Add HTTP handlers and routes**

Extend `HandlerService` in `apps/control-plane/internal/employee/handler.go`:

```go
ListInstructionFiles(ctx context.Context, req ListInstructionFilesRequest) ([]InstructionFile, error)
UpsertInstructionFile(ctx context.Context, req UpsertInstructionFileRequest) (InstructionFile, error)
```

Add response type and handlers:

```go
type instructionFileResponse struct {
	ID             string `json:"id"`
	Path           string `json:"path"`
	Content        string `json:"content"`
	SizeBytes      int64  `json:"size_bytes"`
	ChecksumSHA256 string `json:"checksum_sha256"`
	UpdatedAt      string `json:"updated_at,omitempty"`
}

func (h *HTTPHandler) ListInstructionFiles(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, &employeeID, "employee instruction files read")
	if !ok {
		return
	}
	files, err := h.service.ListInstructionFiles(r.Context(), ListInstructionFilesRequest{TenantID: tenantID, DigitalEmployeeID: employeeID})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	responses := make([]instructionFileResponse, 0, len(files))
	for _, file := range files {
		responses = append(responses, instructionFileResponse{
			ID:             file.ID.String(),
			Path:           file.Path,
			Content:        file.Content,
			SizeBytes:      file.SizeBytes,
			ChecksumSHA256: file.ChecksumSHA256,
			UpdatedAt:      formatTime(file.UpdatedAt),
		})
	}
	writeJSON(w, http.StatusOK, responses)
}

func (h *HTTPHandler) UpsertInstructionFile(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeConfigCreate, &employeeID, "employee instruction file upsert")
	if !ok {
		return
	}
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	file, err := h.service.UpsertInstructionFile(r.Context(), UpsertInstructionFileRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		Path:              req.Path,
		Content:           req.Content,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, instructionFileResponse{
		ID:             file.ID.String(),
		Path:           file.Path,
		Content:        file.Content,
		SizeBytes:      file.SizeBytes,
		ChecksumSHA256: file.ChecksumSHA256,
		UpdatedAt:      formatTime(file.UpdatedAt),
	})
}
```

Register in `apps/control-plane/internal/api/server.go`:

```go
r.Get("/digital-employees/{employeeId}/instructions", s.employeeHandler.ListInstructionFiles)
r.Put("/digital-employees/{employeeId}/instructions", s.employeeHandler.UpsertInstructionFile)
```

- [ ] **Step 7: Run employee tests**

Run:

```bash
go test ./apps/control-plane/internal/employee -run TestService.*Instruction -count=1
go test ./apps/control-plane/internal/api -run TestEmployeeRoutes -count=1
```

Expected: PASS after adding route coverage to `employee_routes_test.go`.

- [ ] **Step 8: Commit**

```bash
git add apps/control-plane/internal/storage/queries/employee_instructions.sql apps/control-plane/internal/storage/queries/employee_instructions.sql.go apps/control-plane/internal/storage/queries/models.go apps/control-plane/internal/storage/queries/querier.go apps/control-plane/internal/employee apps/control-plane/internal/api/server.go apps/control-plane/internal/api/employee_routes_test.go
git commit -m "feat: add employee instruction files"
```

## Task 6: Capability HTTP Handler And Server Wiring

**Files:**
- Create: `apps/control-plane/internal/capability/handler.go`
- Create: `apps/control-plane/internal/capability/handler_test.go`
- Modify: `apps/control-plane/internal/authz/types.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/app/app.go`
- Test: `apps/control-plane/internal/capability/handler_test.go`

- [ ] **Step 1: Write failing handler test**

Create `apps/control-plane/internal/capability/handler_test.go`:

```go
package capability

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestHandlerCreatesCredentialWithoutReturningSecret(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	service := &handlerService{}
	handler := NewHandler(service)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/user-credentials", bytes.NewBufferString(`{"name":"ops-token","credential_type":"mcp_token","credential_value":"secret-token-1234"}`))
	req = req.WithContext(context.WithValue(req.Context(), testTenantKey{}, tenantID))
	req = req.WithContext(context.WithValue(req.Context(), testUserKey{}, userID))
	rec := httptest.NewRecorder()

	handler.CreateCredential(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := body["credential_value"]; ok {
		t.Fatalf("response must not include credential_value: %#v", body)
	}
	if _, ok := body["encrypted_value"]; ok {
		t.Fatalf("response must not include encrypted_value: %#v", body)
	}
	if service.createCredentialReq.CredentialValue != "secret-token-1234" {
		t.Fatalf("expected raw value to reach service create request")
	}
}
```

Add a small helper in `handler_test.go` that injects the same tenant and user IDs read by `middleware.GetTenantID` and `middleware.GetUserID`; use the repo's existing middleware helper when it is already available in nearby API tests.

- [ ] **Step 2: Run handler test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/capability -run TestHandlerCreatesCredentialWithoutReturningSecret -count=1
```

Expected: FAIL because handler does not exist.

- [ ] **Step 3: Add authz action constants**

In `apps/control-plane/internal/authz/types.go`, add:

```go
ResourceCredential = "credential"
```

Add actions:

```go
ActionCredentialRead   = "credential.read"
ActionCredentialCreate = "credential.create"
ActionCredentialDelete = "credential.delete"
ActionEmployeeCapabilityEdit = "employee.capability.edit"
```

- [ ] **Step 4: Create capability HTTP handler**

Create `apps/control-plane/internal/capability/handler.go`:

```go
package capability

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/authz"
)

type HandlerService interface {
	CreateCredential(ctx context.Context, req CreateCredentialRequest) (Credential, error)
	ListCredentials(ctx context.Context, req ListCredentialsRequest) ([]Credential, error)
	CreateTeamMCPServer(ctx context.Context, req CreateTeamMCPServerRequest) (MCPServer, error)
	ListTeamMCPServers(ctx context.Context, req TeamScopedRequest) ([]MCPServer, error)
	DeleteTeamMCPServer(ctx context.Context, req DeleteTeamMCPServerRequest) error
	CreateEmployeeMCPBinding(ctx context.Context, req CreateEmployeeMCPBindingRequest) (MCPServer, error)
	ListEmployeeMCPBindings(ctx context.Context, req EmployeeScopedRequest) ([]MCPServer, error)
	DeleteEmployeeMCPBinding(ctx context.Context, req DeleteEmployeeMCPBindingRequest) error
	ListEffectiveMCPServers(ctx context.Context, req EmployeeScopedRequest) ([]MCPServer, error)
}

type HTTPHandler struct {
	service    HandlerService
	authorizer authz.Authorizer
}

func NewHandler(service HandlerService) *HTTPHandler {
	return &HTTPHandler{service: service}
}

func (h *HTTPHandler) SetAuthorizer(authorizer authz.Authorizer) {
	h.authorizer = authorizer
}

func (h *HTTPHandler) CreateCredential(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorize(w, r, authz.ActionCredentialCreate, authz.ResourceRef{Type: authz.ResourceCredential, ID: middleware.GetUserID(r.Context()).String()}, "credential create")
	if !ok {
		return
	}
	var req struct {
		Name            string `json:"name"`
		CredentialType  string `json:"credential_type"`
		CredentialValue string `json:"credential_value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	item, err := h.service.CreateCredential(r.Context(), CreateCredentialRequest{
		TenantID:        tenantID,
		UserID:          userID,
		Name:            req.Name,
		CredentialType:  req.CredentialType,
		CredentialValue: req.CredentialValue,
	})
	if err != nil {
		writeCapabilityError(w, err)
		return
	}
	writeCapabilityJSON(w, http.StatusCreated, credentialResponseFromDomain(item))
}

func (h *HTTPHandler) ListCredentials(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorize(w, r, authz.ActionCredentialRead, authz.ResourceRef{Type: authz.ResourceCredential, ID: middleware.GetUserID(r.Context()).String()}, "credential read")
	if !ok {
		return
	}
	items, err := h.service.ListCredentials(r.Context(), ListCredentialsRequest{TenantID: tenantID, UserID: userID, CredentialType: r.URL.Query().Get("credential_type")})
	if err != nil {
		writeCapabilityError(w, err)
		return
	}
	writeCapabilityJSON(w, http.StatusOK, credentialResponses(items))
}

func (h *HTTPHandler) CreateTeamMCPServer(w http.ResponseWriter, r *http.Request) {
	teamID, ok := uuidParam(w, r, "teamId", "invalid team id")
	if !ok {
		return
	}
	tenantID, userID, ok := h.authorize(w, r, authz.ActionTeamCapabilityBind, authz.ResourceRef{Type: authz.ResourceTeam, ID: teamID.String()}, "team mcp create")
	if !ok {
		return
	}
	var req struct {
		Name         string     `json:"name"`
		URL          string     `json:"url"`
		CredentialID *uuid.UUID `json:"credential_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	item, err := h.service.CreateTeamMCPServer(r.Context(), CreateTeamMCPServerRequest{TenantID: tenantID, UserID: userID, TeamID: teamID, Name: req.Name, URL: req.URL, CredentialID: req.CredentialID})
	if err != nil {
		writeCapabilityError(w, err)
		return
	}
	writeCapabilityJSON(w, http.StatusCreated, mcpServerResponseFromDomain(item))
}

func (h *HTTPHandler) ListTeamMCPServers(w http.ResponseWriter, r *http.Request) {
	teamID, ok := uuidParam(w, r, "teamId", "invalid team id")
	if !ok {
		return
	}
	tenantID, userID, ok := h.authorize(w, r, authz.ActionTeamCapabilityBind, authz.ResourceRef{Type: authz.ResourceTeam, ID: teamID.String()}, "team mcp read")
	if !ok {
		return
	}
	items, err := h.service.ListTeamMCPServers(r.Context(), TeamScopedRequest{TenantID: tenantID, UserID: userID, TeamID: teamID})
	if err != nil {
		writeCapabilityError(w, err)
		return
	}
	writeCapabilityJSON(w, http.StatusOK, mcpServerResponses(items))
}

func (h *HTTPHandler) DeleteTeamMCPServer(w http.ResponseWriter, r *http.Request) {
	teamID, ok := uuidParam(w, r, "teamId", "invalid team id")
	if !ok {
		return
	}
	serverID, ok := uuidParam(w, r, "serverId", "invalid server id")
	if !ok {
		return
	}
	tenantID, _, ok := h.authorize(w, r, authz.ActionTeamCapabilityUnbind, authz.ResourceRef{Type: authz.ResourceTeam, ID: teamID.String()}, "team mcp delete")
	if !ok {
		return
	}
	if err := h.service.DeleteTeamMCPServer(r.Context(), DeleteTeamMCPServerRequest{TenantID: tenantID, TeamID: teamID, ServerID: serverID}); err != nil {
		writeCapabilityError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) CreateEmployeeMCPBinding(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := uuidParam(w, r, "employeeId", "invalid employee id")
	if !ok {
		return
	}
	tenantID, userID, ok := h.authorize(w, r, authz.ActionEmployeeCapabilityEdit, authz.ResourceRef{Type: authz.ResourceEmployee, ID: employeeID.String()}, "employee mcp create")
	if !ok {
		return
	}
	var req struct {
		Name         string     `json:"name"`
		URL          string     `json:"url"`
		CredentialID *uuid.UUID `json:"credential_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	item, err := h.service.CreateEmployeeMCPBinding(r.Context(), CreateEmployeeMCPBindingRequest{TenantID: tenantID, UserID: userID, DigitalEmployeeID: employeeID, Name: req.Name, URL: req.URL, CredentialID: req.CredentialID})
	if err != nil {
		writeCapabilityError(w, err)
		return
	}
	writeCapabilityJSON(w, http.StatusCreated, mcpServerResponseFromDomain(item))
}

func (h *HTTPHandler) ListEmployeeMCPBindings(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := uuidParam(w, r, "employeeId", "invalid employee id")
	if !ok {
		return
	}
	tenantID, userID, ok := h.authorize(w, r, authz.ActionEmployeeCapabilityEdit, authz.ResourceRef{Type: authz.ResourceEmployee, ID: employeeID.String()}, "employee mcp read")
	if !ok {
		return
	}
	items, err := h.service.ListEmployeeMCPBindings(r.Context(), EmployeeScopedRequest{TenantID: tenantID, UserID: userID, DigitalEmployeeID: employeeID})
	if err != nil {
		writeCapabilityError(w, err)
		return
	}
	writeCapabilityJSON(w, http.StatusOK, mcpServerResponses(items))
}

func (h *HTTPHandler) DeleteEmployeeMCPBinding(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := uuidParam(w, r, "employeeId", "invalid employee id")
	if !ok {
		return
	}
	bindingID, ok := uuidParam(w, r, "bindingId", "invalid binding id")
	if !ok {
		return
	}
	tenantID, _, ok := h.authorize(w, r, authz.ActionEmployeeCapabilityEdit, authz.ResourceRef{Type: authz.ResourceEmployee, ID: employeeID.String()}, "employee mcp delete")
	if !ok {
		return
	}
	if err := h.service.DeleteEmployeeMCPBinding(r.Context(), DeleteEmployeeMCPBindingRequest{TenantID: tenantID, DigitalEmployeeID: employeeID, BindingID: bindingID}); err != nil {
		writeCapabilityError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) ListEffectiveMCPServers(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := uuidParam(w, r, "employeeId", "invalid employee id")
	if !ok {
		return
	}
	tenantID, userID, ok := h.authorize(w, r, authz.ActionEmployeeRead, authz.ResourceRef{Type: authz.ResourceEmployee, ID: employeeID.String()}, "employee effective mcp read")
	if !ok {
		return
	}
	items, err := h.service.ListEffectiveMCPServers(r.Context(), EmployeeScopedRequest{TenantID: tenantID, UserID: userID, DigitalEmployeeID: employeeID})
	if err != nil {
		writeCapabilityError(w, err)
		return
	}
	writeCapabilityJSON(w, http.StatusOK, mcpServerResponses(items))
}

func (h *HTTPHandler) authorize(w http.ResponseWriter, r *http.Request, action string, resource authz.ResourceRef, auditReason string) (uuid.UUID, uuid.UUID, bool) {
	if h == nil || h.authorizer == nil {
		http.Error(w, "capability authorization is not configured", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil {
		http.Error(w, "console identity not found in context", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}
	decision, err := h.authorizer.Check(r.Context(), authz.CheckRequest{
		Actor:      authz.ActorRef{Type: authz.ActorUser, ID: userID.String()},
		Action:     action,
		Resource:   resource,
		TenantID:   tenantID,
		AuditReason: auditReason,
	})
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return uuid.Nil, uuid.Nil, false
	}
	if !decision.Allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}
	return tenantID, userID, true
}

type credentialResponse struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	CredentialType string `json:"credential_type"`
	LastFour       string `json:"last_four"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at,omitempty"`
	UpdatedAt      string `json:"updated_at,omitempty"`
}

type mcpServerResponse struct {
	ID                 string  `json:"id"`
	TeamID             string  `json:"team_id,omitempty"`
	DigitalEmployeeID  string  `json:"digital_employee_id,omitempty"`
	Name               string  `json:"name"`
	URL                string  `json:"url"`
	CredentialID       string  `json:"credential_id,omitempty"`
	CredentialName     string  `json:"credential_name,omitempty"`
	CredentialType     string  `json:"credential_type,omitempty"`
	CredentialLastFour string  `json:"credential_last_four,omitempty"`
	Status             string  `json:"status"`
	SourceScope        string  `json:"source_scope"`
	Inherited          bool    `json:"inherited"`
	CreatedAt          string  `json:"created_at,omitempty"`
	UpdatedAt          string  `json:"updated_at,omitempty"`
}

func credentialResponseFromDomain(item Credential) credentialResponse {
	return credentialResponse{ID: item.ID.String(), Name: item.Name, CredentialType: item.CredentialType, LastFour: item.LastFour, Status: item.Status, CreatedAt: formatCapabilityTime(item.CreatedAt), UpdatedAt: formatCapabilityTime(item.UpdatedAt)}
}

func credentialResponses(items []Credential) []credentialResponse {
	responses := make([]credentialResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, credentialResponseFromDomain(item))
	}
	return responses
}

func mcpServerResponseFromDomain(item MCPServer) mcpServerResponse {
	response := mcpServerResponse{ID: item.ID.String(), Name: item.Name, URL: item.URL, CredentialName: item.CredentialName, CredentialType: item.CredentialType, CredentialLastFour: item.CredentialLastFour, Status: item.Status, SourceScope: item.SourceScope, Inherited: item.Inherited, CreatedAt: formatCapabilityTime(item.CreatedAt), UpdatedAt: formatCapabilityTime(item.UpdatedAt)}
	if item.TeamID != nil {
		response.TeamID = item.TeamID.String()
	}
	if item.DigitalEmployeeID != nil {
		response.DigitalEmployeeID = item.DigitalEmployeeID.String()
	}
	if item.CredentialID != nil {
		response.CredentialID = item.CredentialID.String()
	}
	return response
}

func mcpServerResponses(items []MCPServer) []mcpServerResponse {
	responses := make([]mcpServerResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, mcpServerResponseFromDomain(item))
	}
	return responses
}

func uuidParam(w http.ResponseWriter, r *http.Request, key, message string) (uuid.UUID, bool) {
	value, err := uuid.Parse(chi.URLParam(r, key))
	if err != nil || value == uuid.Nil {
		http.Error(w, message, http.StatusBadRequest)
		return uuid.Nil, false
	}
	return value, true
}

func writeCapabilityJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeCapabilityError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrCredentialKeyMissing), errors.Is(err, ErrCredentialTypeInvalid):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func formatCapabilityTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
```

- [ ] **Step 5: Wire app and routes**

Modify `apps/control-plane/internal/api/server.go`:

```go
capabilityHandler *capability.HTTPHandler
```

Add setter:

```go
func (s *Server) SetCapabilityHandler(capabilityHandler *capability.HTTPHandler) {
	s.capabilityHandler = capabilityHandler
	if capabilityHandler != nil {
		capabilityHandler.SetAuthorizer(s.authorizer)
	}
	s.registerRoutes()
}
```

Inside `/api/v1` register:

```go
if s.capabilityHandler != nil {
	r.Group(func(r chi.Router) {
		r.Use(middleware.ConsoleUserAuth(s.authService))
		r.Get("/user-credentials", s.capabilityHandler.ListCredentials)
		r.Post("/user-credentials", s.capabilityHandler.CreateCredential)
		r.Get("/teams/{teamId}/mcp-servers", s.capabilityHandler.ListTeamMCPServers)
		r.Post("/teams/{teamId}/mcp-servers", s.capabilityHandler.CreateTeamMCPServer)
		r.Delete("/teams/{teamId}/mcp-servers/{serverId}", s.capabilityHandler.DeleteTeamMCPServer)
		r.Get("/digital-employees/{employeeId}/mcp-bindings", s.capabilityHandler.ListEmployeeMCPBindings)
		r.Post("/digital-employees/{employeeId}/mcp-bindings", s.capabilityHandler.CreateEmployeeMCPBinding)
		r.Delete("/digital-employees/{employeeId}/mcp-bindings/{bindingId}", s.capabilityHandler.DeleteEmployeeMCPBinding)
		r.Get("/digital-employees/{employeeId}/effective-mcp-servers", s.capabilityHandler.ListEffectiveMCPServers)
	})
}
```

Modify `apps/control-plane/internal/app/app.go`:

```go
CapabilityService *capability.Service
CapabilityHandler *capability.HTTPHandler
```

Import `os` and `github.com/superteam/control-plane/internal/capability`, then initialize:

```go
var credentialSealer capability.CredentialSealer
if key := os.Getenv("CONTROL_PLANE_CREDENTIAL_KEY"); key != "" {
	sealer, err := capability.NewAESGCMCredentialSealer(key)
	if err != nil {
		return nil, err
	}
	credentialSealer = sealer
}
capabilityRepository := capability.NewPgRepository(q)
capabilityService := capability.NewService(capabilityRepository, credentialSealer)
capabilityHandler := capability.NewHandler(capabilityService)
server.SetCapabilityHandler(capabilityHandler)
```

- [ ] **Step 6: Run capability package and app tests**

Run:

```bash
go test ./apps/control-plane/internal/capability -count=1
go test ./apps/control-plane/internal/app -count=1
go test ./apps/control-plane/internal/api -run 'Test(TeamRoutes|EmployeeRoutes|SkillRoutes)' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/capability apps/control-plane/internal/authz/types.go apps/control-plane/internal/api/server.go apps/control-plane/internal/app/app.go apps/control-plane/internal/api/team_routes_test.go apps/control-plane/internal/api/employee_routes_test.go
git commit -m "feat: expose capability management routes"
```

## Task 7: OpenAPI Contract And Web API Clients

**Files:**
- Modify: `contracts/control-plane/openapi.yaml`
- Modify: `apps/web/src/lib/api/skills.ts`
- Modify: `apps/web/src/lib/api/skills.test.ts`
- Create: `apps/web/src/lib/api/capabilities.ts`
- Create: `apps/web/src/lib/api/capabilities.test.ts`
- Modify: `apps/web/src/lib/api/employees.ts`
- Modify: `apps/web/src/lib/api/employees.test.ts`

- [ ] **Step 1: Write failing Web API tests**

Create `apps/web/src/lib/api/capabilities.test.ts`:

```ts
import { describe, expect, it, vi } from "vitest";
import {
  createEmployeeMcpBinding,
  createTeamMcpServer,
  createUserCredential,
  listEffectiveMcpServers,
  listTeamMcpServers,
  listUserCredentials,
} from "@/lib/api/capabilities";

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status,
  });
}

describe("capabilities api", () => {
  it("creates credentials without expecting returned secret fields", async () => {
    const fetcher = vi.fn(async () =>
      jsonResponse({
        id: "credential-1",
        name: "ops-token",
        credential_type: "mcp_token",
        last_four: "7890",
        status: "active",
      }, 201),
    );

    const credential = await createUserCredential(
      { baseUrl: "http://control-plane.local", fetcher },
      { name: "ops-token", credential_type: "mcp_token", credential_value: "sk-test-7890" },
    );

    expect(credential).toEqual({
      id: "credential-1",
      name: "ops-token",
      credential_type: "mcp_token",
      last_four: "7890",
      status: "active",
    });
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/user-credentials",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("lists and creates team and employee mcp servers", async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input));
      const method = init?.method ?? "GET";
      if (url.pathname === "/api/v1/user-credentials" && method === "GET") {
        return jsonResponse([{ id: "credential-1", name: "ops-token", credential_type: "mcp_token", last_four: "7890", status: "active" }]);
      }
      if (url.pathname === "/api/v1/teams/team-1/mcp-servers" && method === "GET") {
        return jsonResponse([{ id: "mcp-1", team_id: "team-1", name: "ops-mcp", url: "https://mcp.example.com", credential_id: "credential-1", source_scope: "team", inherited: true, status: "active" }]);
      }
      if (url.pathname === "/api/v1/teams/team-1/mcp-servers" && method === "POST") {
        expect(JSON.parse(String(init?.body))).toEqual({ name: "ops-mcp", url: "https://mcp.example.com", credential_id: "credential-1" });
        return jsonResponse({ id: "mcp-1", team_id: "team-1", name: "ops-mcp", url: "https://mcp.example.com", credential_id: "credential-1", source_scope: "team", inherited: true, status: "active" }, 201);
      }
      if (url.pathname === "/api/v1/digital-employees/employee-1/mcp-bindings" && method === "POST") {
        expect(JSON.parse(String(init?.body))).toEqual({ name: "personal-mcp", url: "https://personal.example.com", credential_id: "credential-1" });
        return jsonResponse({ id: "mcp-2", digital_employee_id: "employee-1", name: "personal-mcp", url: "https://personal.example.com", credential_id: "credential-1", source_scope: "employee", inherited: false, status: "active" }, 201);
      }
      if (url.pathname === "/api/v1/digital-employees/employee-1/effective-mcp-servers" && method === "GET") {
        return jsonResponse([{ id: "mcp-1", team_id: "team-1", digital_employee_id: "employee-1", name: "ops-mcp", url: "https://mcp.example.com", source_scope: "team", inherited: true, status: "active" }]);
      }
      return jsonResponse({ error: `unhandled ${method} ${url.pathname}` }, 500);
    });

    await expect(listUserCredentials({ baseUrl: "http://control-plane.local", fetcher }, "mcp_token")).resolves.toHaveLength(1);
    await expect(listTeamMcpServers({ baseUrl: "http://control-plane.local", fetcher }, "team-1")).resolves.toHaveLength(1);
    await expect(createTeamMcpServer({ baseUrl: "http://control-plane.local", fetcher }, "team-1", { name: "ops-mcp", url: "https://mcp.example.com", credential_id: "credential-1" })).resolves.toMatchObject({ inherited: true });
    await expect(createEmployeeMcpBinding({ baseUrl: "http://control-plane.local", fetcher }, "employee-1", { name: "personal-mcp", url: "https://personal.example.com", credential_id: "credential-1" })).resolves.toMatchObject({ inherited: false });
    await expect(listEffectiveMcpServers({ baseUrl: "http://control-plane.local", fetcher }, "employee-1")).resolves.toHaveLength(1);
  });
});
```

- [ ] **Step 2: Run Web API tests and verify they fail**

Run:

```bash
pnpm --dir apps/web test apps/web/src/lib/api/capabilities.test.ts
```

Expected: FAIL because `capabilities.ts` does not exist.

- [ ] **Step 3: Create capabilities Web API client**

Create `apps/web/src/lib/api/capabilities.ts`:

```ts
import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type CredentialType = "mcp_token";

export type UserCredential = {
  id: string;
  name: string;
  credential_type: CredentialType;
  last_four: string;
  status: string;
  created_at?: string;
  updated_at?: string;
};

export type CreateUserCredentialInput = {
  name: string;
  credential_type: CredentialType;
  credential_value: string;
};

export type McpServer = {
  id: string;
  team_id?: string;
  digital_employee_id?: string;
  name: string;
  url: string;
  credential_id?: string;
  credential_name?: string;
  credential_type?: string;
  credential_last_four?: string;
  status: string;
  source_scope: "team" | "employee";
  inherited: boolean;
  created_at?: string;
  updated_at?: string;
};

export type CreateMcpServerInput = {
  name: string;
  url: string;
  credential_id?: string;
};

async function getJson<T>(options: ApiClientOptions, path: string, resource: string): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "GET",
  });
  return parseJson<T>(response, resource);
}

async function postJson<T>(options: ApiClientOptions, path: string, input: unknown, resource: string): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });
  return parseJson<T>(response, resource);
}

async function deleteJson(options: ApiClientOptions, path: string): Promise<void> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    credentials: "include",
    method: "DELETE",
  });
  if (!response.ok) {
    await parseJson<unknown>(response, "delete capability resource");
  }
}

function encodePathSegment(value: string) {
  return encodeURIComponent(value);
}

export function listUserCredentials(options: ApiClientOptions, credentialType?: CredentialType): Promise<UserCredential[]> {
  const search = new URLSearchParams();
  if (credentialType) {
    search.set("credential_type", credentialType);
  }
  const query = search.toString();
  return getJson<UserCredential[]>(options, `/api/v1/user-credentials${query ? `?${query}` : ""}`, "user credentials");
}

export function createUserCredential(options: ApiClientOptions, input: CreateUserCredentialInput): Promise<UserCredential> {
  return postJson<UserCredential>(options, "/api/v1/user-credentials", input, "create user credential");
}

export function listTeamMcpServers(options: ApiClientOptions, teamId: string): Promise<McpServer[]> {
  return getJson<McpServer[]>(options, `/api/v1/teams/${encodePathSegment(teamId)}/mcp-servers`, "team mcp servers");
}

export function createTeamMcpServer(options: ApiClientOptions, teamId: string, input: CreateMcpServerInput): Promise<McpServer> {
  return postJson<McpServer>(options, `/api/v1/teams/${encodePathSegment(teamId)}/mcp-servers`, input, "create team mcp server");
}

export function deleteTeamMcpServer(options: ApiClientOptions, teamId: string, serverId: string): Promise<void> {
  return deleteJson(options, `/api/v1/teams/${encodePathSegment(teamId)}/mcp-servers/${encodePathSegment(serverId)}`);
}

export function listEmployeeMcpBindings(options: ApiClientOptions, employeeId: string): Promise<McpServer[]> {
  return getJson<McpServer[]>(options, `/api/v1/digital-employees/${encodePathSegment(employeeId)}/mcp-bindings`, "employee mcp bindings");
}

export function createEmployeeMcpBinding(options: ApiClientOptions, employeeId: string, input: CreateMcpServerInput): Promise<McpServer> {
  return postJson<McpServer>(options, `/api/v1/digital-employees/${encodePathSegment(employeeId)}/mcp-bindings`, input, "create employee mcp binding");
}

export function deleteEmployeeMcpBinding(options: ApiClientOptions, employeeId: string, bindingId: string): Promise<void> {
  return deleteJson(options, `/api/v1/digital-employees/${encodePathSegment(employeeId)}/mcp-bindings/${encodePathSegment(bindingId)}`);
}

export function listEffectiveMcpServers(options: ApiClientOptions, employeeId: string): Promise<McpServer[]> {
  return getJson<McpServer[]>(options, `/api/v1/digital-employees/${encodePathSegment(employeeId)}/effective-mcp-servers`, "effective mcp servers");
}
```

- [ ] **Step 4: Extend skills Web API client**

In `apps/web/src/lib/api/skills.ts`, add:

```ts
export type EffectiveEmployeeSkill = {
  skill: Skill;
  source_scope: "team" | "employee";
  inherited: boolean;
  read_only: boolean;
};

export async function listTeamSkills(options: ApiClientOptions, teamId: string): Promise<Skill[]> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/teams/${encodeURIComponent(teamId)}/skills`), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "GET",
  });
  return parseJson<Skill[]>(response, "team skills");
}

export async function bindTeamSkill(options: ApiClientOptions, teamId: string, skillId: string): Promise<Skill> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/teams/${encodeURIComponent(teamId)}/skills`), {
    body: JSON.stringify({ skill_id: skillId }),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });
  return parseJson<Skill>(response, "bind team skill");
}

export async function unbindTeamSkill(options: ApiClientOptions, teamId: string, skillId: string): Promise<void> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/teams/${encodeURIComponent(teamId)}/skills/${encodeURIComponent(skillId)}`), {
    credentials: "include",
    method: "DELETE",
  });
  if (!response.ok) {
    await parseJson<unknown>(response, "unbind team skill");
  }
}

export async function listEmployeeSkills(options: ApiClientOptions, employeeId: string): Promise<EffectiveEmployeeSkill[]> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/digital-employees/${encodeURIComponent(employeeId)}/skills`), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "GET",
  });
  return parseJson<EffectiveEmployeeSkill[]>(response, "employee skills");
}

export async function bindEmployeeSkill(options: ApiClientOptions, employeeId: string, skillId: string): Promise<Skill> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/digital-employees/${encodeURIComponent(employeeId)}/skills`), {
    body: JSON.stringify({ skill_id: skillId }),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });
  return parseJson<Skill>(response, "bind employee skill");
}

export async function unbindEmployeeSkill(options: ApiClientOptions, employeeId: string, skillId: string): Promise<void> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/digital-employees/${encodeURIComponent(employeeId)}/skills/${encodeURIComponent(skillId)}`), {
    credentials: "include",
    method: "DELETE",
  });
  if (!response.ok) {
    await parseJson<unknown>(response, "unbind employee skill");
  }
}
```

- [ ] **Step 5: Extend employee Web API client**

In `apps/web/src/lib/api/employees.ts`, add:

```ts
export type InstructionFile = {
  id: string;
  path: string;
  content: string;
  size_bytes: number;
  checksum_sha256: string;
  updated_at?: string;
};

export type UpsertInstructionFileInput = {
  path: string;
  content: string;
};

export function listInstructionFiles(options: ApiClientOptions, employeeId: string): Promise<InstructionFile[]> {
  const encodedEmployeeId = encodePathSegment(employeeId);
  return getJson<InstructionFile[]>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/instructions`,
    "employee instruction files",
  );
}

async function putJson<T>(options: ApiClientOptions, path: string, input: unknown, resource: string): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "PUT",
  });
  return parseJson<T>(response, resource);
}

export function upsertInstructionFile(options: ApiClientOptions, employeeId: string, input: UpsertInstructionFileInput): Promise<InstructionFile> {
  const encodedEmployeeId = encodePathSegment(employeeId);
  return putJson<InstructionFile>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/instructions`,
    input,
    "upsert employee instruction file",
  );
}
```

- [ ] **Step 6: Update OpenAPI**

In `contracts/control-plane/openapi.yaml`, add paths for:

```yaml
  /api/v1/user-credentials:
    get:
      operationId: listUserCredentials
      summary: List current user credentials
    post:
      operationId: createUserCredential
      summary: Create a user credential
  /api/v1/teams/{teamId}/skills:
    get:
      operationId: listTeamSkills
      summary: List team inherited skills
    post:
      operationId: bindTeamSkill
      summary: Bind a skill to a team
  /api/v1/teams/{teamId}/skills/{skillId}:
    delete:
      operationId: unbindTeamSkill
      summary: Remove a team skill binding
  /api/v1/teams/{teamId}/mcp-servers:
    get:
      operationId: listTeamMCPServers
      summary: List team MCP servers
    post:
      operationId: createTeamMCPServer
      summary: Create a team MCP server
  /api/v1/teams/{teamId}/mcp-servers/{serverId}:
    delete:
      operationId: deleteTeamMCPServer
      summary: Delete a team MCP server
  /api/v1/digital-employees/{employeeId}/instructions:
    get:
      operationId: listEmployeeInstructionFiles
      summary: List employee instruction files
    put:
      operationId: upsertEmployeeInstructionFile
      summary: Upsert an employee instruction file
  /api/v1/digital-employees/{employeeId}/skills:
    get:
      operationId: listEmployeeSkills
      summary: List effective employee skills
    post:
      operationId: bindEmployeeSkill
      summary: Bind a personal employee skill
  /api/v1/digital-employees/{employeeId}/skills/{skillId}:
    delete:
      operationId: unbindEmployeeSkill
      summary: Remove a personal employee skill binding
  /api/v1/digital-employees/{employeeId}/mcp-bindings:
    get:
      operationId: listEmployeeMCPBindings
      summary: List personal employee MCP bindings
    post:
      operationId: createEmployeeMCPBinding
      summary: Create a personal employee MCP binding
  /api/v1/digital-employees/{employeeId}/mcp-bindings/{bindingId}:
    delete:
      operationId: deleteEmployeeMCPBinding
      summary: Delete a personal employee MCP binding
  /api/v1/digital-employees/{employeeId}/effective-mcp-servers:
    get:
      operationId: listEffectiveMCPServers
      summary: List merged team and personal MCP servers
```

Add schemas for `UserCredential`, `CreateUserCredentialRequest`, `MCPServer`, `CreateMCPServerRequest`, `InstructionFile`, `UpsertInstructionFileRequest`, and `EffectiveEmployeeSkill`. Match the TypeScript field names exactly.

- [ ] **Step 7: Run client and contract tests**

Run:

```bash
pnpm --dir apps/web test apps/web/src/lib/api/capabilities.test.ts apps/web/src/lib/api/skills.test.ts apps/web/src/lib/api/employees.test.ts
node scripts/verify-foundation-contracts.mjs
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add contracts/control-plane/openapi.yaml apps/web/src/lib/api/skills.ts apps/web/src/lib/api/skills.test.ts apps/web/src/lib/api/capabilities.ts apps/web/src/lib/api/capabilities.test.ts apps/web/src/lib/api/employees.ts apps/web/src/lib/api/employees.test.ts
git commit -m "feat: add dual layer capability client contracts"
```

## Task 8: Team Public Skills And MCP UI

**Files:**
- Modify: `apps/web/src/features/teams/components/team-capabilities-tab.tsx`
- Modify: `apps/web/src/features/teams/index.test.tsx`

- [ ] **Step 1: Write failing team UI test**

Append to `apps/web/src/features/teams/index.test.tsx`:

```tsx
it("manages team public skills and mcp servers in the capabilities tab", async () => {
  const fetcher = createTeamsFetcher({
    extraRoutes: {
      "GET /api/v1/skills": [
        {
          id: "skill-diagnose",
          tenant_id: "tenant-1",
          slug: "diagnose",
          name: "diagnose",
          description: "系统化诊断流程",
          version: "v1.0.0",
          source: "internal_market",
          risk_level: "low",
          status: "installed",
          icon_key: "stethoscope",
          color_token: "cyan",
          tags: ["诊断"],
          files: [],
          team_bindings: [],
          agent_bindings: [],
        },
      ],
      "GET /api/v1/teams/team-1/skills": [],
      "GET /api/v1/user-credentials?credential_type=mcp_token": [
        { id: "credential-1", name: "ops-token", credential_type: "mcp_token", last_four: "7890", status: "active" },
      ],
      "GET /api/v1/teams/team-1/mcp-servers": [],
    },
  });
  const screen = await renderTeamsView(fetcher);

  await userEvent.click(screen.getByRole("tab", { name: "治理与能力" }));
  await expect.element(screen.getByRole("heading", { name: "公共技能" })).toBeVisible();
  await expect.element(screen.getByRole("heading", { name: "公共 MCP" })).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "安装 diagnose" }));

  expect(fetcher).toHaveBeenCalledWith(
    expect.stringContaining("/api/v1/teams/team-1/skills"),
    expect.objectContaining({ method: "POST" }),
  );

  await userEvent.fill(screen.getByLabelText("MCP 名称"), "ops-mcp");
  await userEvent.fill(screen.getByLabelText("MCP URL"), "https://mcp.example.com");
  await userEvent.click(screen.getByRole("combobox", { name: "凭据" }));
  await userEvent.click(screen.getByRole("option", { name: "ops-token ****7890" }));
  await userEvent.click(screen.getByRole("button", { name: "添加公共 MCP" }));

  expect(fetcher).toHaveBeenCalledWith(
    expect.stringContaining("/api/v1/teams/team-1/mcp-servers"),
    expect.objectContaining({ method: "POST" }),
  );
});
```

Extend the local `createTeamsFetcher` helper in `apps/web/src/features/teams/index.test.tsx` with this optional route table before running the test:

```tsx
type ExtraRoutes = Record<string, unknown>;

function routeKey(input: RequestInfo | URL, init?: RequestInit) {
  const url = new URL(String(input));
  return `${init?.method ?? "GET"} ${url.pathname}${url.search}`;
}

function routeResponse(extraRoutes: ExtraRoutes | undefined, input: RequestInfo | URL, init?: RequestInit) {
  const key = routeKey(input, init);
  if (!extraRoutes || !(key in extraRoutes)) {
    return undefined;
  }
  const status = (init?.method ?? "GET") === "POST" ? 201 : 200;
  return jsonResponse(extraRoutes[key], status);
}
```

At the top of the fetcher implementation, call:

```tsx
const extraResponse = routeResponse(options.extraRoutes, input, init);
if (extraResponse) {
  return extraResponse;
}
```

- [ ] **Step 2: Run the team UI test and verify it fails**

Run:

```bash
pnpm --dir apps/web test apps/web/src/features/teams/index.test.tsx -t "manages team public skills"
```

Expected: FAIL because the tab still renders governance draft placeholders.

- [ ] **Step 3: Replace capabilities tab with public capability management**

Modify `apps/web/src/features/teams/components/team-capabilities-tab.tsx` so it imports:

```tsx
import { Boxes, KeyRound, Network, Plus, ShieldCheck, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { LiquidCard, SemanticIconTile, StatusBadge } from "@/components/superteam";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { ApiClientOptions } from "@/lib/api/client";
import { createTeamMcpServer, deleteTeamMcpServer, listTeamMcpServers, listUserCredentials } from "@/lib/api/capabilities";
import { bindTeamSkill, listSkills, listTeamSkills, unbindTeamSkill } from "@/lib/api/skills";
```

Replace the component body with this shape:

```tsx
export function TeamCapabilitiesTab({ apiOptions, canEdit, teamId }: TeamCapabilitiesTabProps) {
  const queryClient = useQueryClient();
  const [mcpName, setMcpName] = useState("");
  const [mcpUrl, setMcpUrl] = useState("");
  const [credentialId, setCredentialId] = useState<string>("none");

  const marketplace = useQuery({
    queryKey: ["skills", ""],
    queryFn: () => listSkills(apiOptions),
    placeholderData: keepPreviousData,
  });
  const teamSkills = useQuery({
    queryKey: ["team-skills", teamId],
    queryFn: () => listTeamSkills(apiOptions, teamId),
    placeholderData: keepPreviousData,
  });
  const credentials = useQuery({
    queryKey: ["user-credentials", "mcp_token"],
    queryFn: () => listUserCredentials(apiOptions, "mcp_token"),
    placeholderData: keepPreviousData,
  });
  const mcpServers = useQuery({
    queryKey: ["team-mcp-servers", teamId],
    queryFn: () => listTeamMcpServers(apiOptions, teamId),
    placeholderData: keepPreviousData,
  });

  const installedSkillIds = useMemo(() => new Set((teamSkills.data ?? []).map((skill) => skill.id)), [teamSkills.data]);
  const availableSkills = (marketplace.data ?? []).filter((skill) => !installedSkillIds.has(skill.id));

  const bindSkill = useMutation({
    mutationFn: (skillId: string) => bindTeamSkill(apiOptions, teamId, skillId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["team-skills", teamId] });
    },
  });
  const unbindSkill = useMutation({
    mutationFn: (skillId: string) => unbindTeamSkill(apiOptions, teamId, skillId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["team-skills", teamId] });
    },
  });
  const createMcp = useMutation({
    mutationFn: () => createTeamMcpServer(apiOptions, teamId, { name: mcpName, url: mcpUrl, credential_id: credentialId === "none" ? undefined : credentialId }),
    onSuccess: () => {
      setMcpName("");
      setMcpUrl("");
      setCredentialId("none");
      void queryClient.invalidateQueries({ queryKey: ["team-mcp-servers", teamId] });
    },
  });
  const deleteMcp = useMutation({
    mutationFn: (serverId: string) => deleteTeamMcpServer(apiOptions, teamId, serverId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["team-mcp-servers", teamId] });
    },
  });

  return (
    <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
      <div className="flex min-w-0 flex-col gap-4">
        <LiquidCard className="rounded-lg">
          <CardHeader className="border-b">
            <div className="flex items-center gap-3">
              <SemanticIconTile tone="primary" size="sm"><ShieldCheck /></SemanticIconTile>
              <CardTitle className="text-base">公共技能</CardTitle>
            </div>
          </CardHeader>
          <CardContent className="grid gap-3 p-4 md:grid-cols-2">
            {(teamSkills.data ?? []).map((skill) => (
              <div className="flex min-w-0 items-center justify-between gap-3 rounded-md border bg-background p-3" key={skill.id}>
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{skill.name}</p>
                  <p className="truncate text-xs text-muted-foreground">{skill.description}</p>
                </div>
                <Button aria-label={`移除 ${skill.name}`} disabled={!canEdit || unbindSkill.isPending} onClick={() => unbindSkill.mutate(skill.id)} size="icon" type="button" variant="ghost">
                  <Trash2 />
                </Button>
              </div>
            ))}
            {(teamSkills.data ?? []).length === 0 ? <p className="text-sm text-muted-foreground">暂无公共技能</p> : null}
          </CardContent>
        </LiquidCard>

        <LiquidCard className="rounded-lg">
          <CardHeader className="border-b">
            <div className="flex items-center gap-3">
              <SemanticIconTile tone="artifact" size="sm"><Boxes /></SemanticIconTile>
              <CardTitle className="text-base">技能市场</CardTitle>
            </div>
          </CardHeader>
          <CardContent className="grid gap-3 p-4 md:grid-cols-2">
            {availableSkills.map((skill) => (
              <div className="flex min-w-0 items-center justify-between gap-3 rounded-md border bg-background p-3" key={skill.id}>
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{skill.name}</p>
                  <div className="mt-2 flex flex-wrap gap-1">
                    {skill.tags.map((tag) => <Badge key={tag} variant="outline">{tag}</Badge>)}
                  </div>
                </div>
                <Button disabled={!canEdit || bindSkill.isPending} onClick={() => bindSkill.mutate(skill.id)} size="sm" type="button" variant="outline">
                  安装 {skill.name}
                </Button>
              </div>
            ))}
          </CardContent>
        </LiquidCard>
      </div>

      <LiquidCard className="rounded-lg">
        <CardHeader className="border-b">
          <div className="flex items-center gap-3">
            <SemanticIconTile tone="warning" size="sm"><Network /></SemanticIconTile>
            <CardTitle className="text-base">公共 MCP</CardTitle>
          </div>
        </CardHeader>
        <CardContent className="flex flex-col gap-4 p-4">
          <div className="grid gap-3">
            <div className="grid gap-2">
              <Label htmlFor="team-mcp-name">MCP 名称</Label>
              <Input id="team-mcp-name" disabled={!canEdit} value={mcpName} onChange={(event) => setMcpName(event.target.value)} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="team-mcp-url">MCP URL</Label>
              <Input id="team-mcp-url" disabled={!canEdit} value={mcpUrl} onChange={(event) => setMcpUrl(event.target.value)} placeholder="https://mcp.example.com" />
            </div>
            <div className="grid gap-2">
              <Label>凭据</Label>
              <Select disabled={!canEdit} value={credentialId} onValueChange={setCredentialId}>
                <SelectTrigger aria-label="凭据">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">不使用凭据</SelectItem>
                  {(credentials.data ?? []).map((credential) => (
                    <SelectItem key={credential.id} value={credential.id}>{credential.name} ****{credential.last_four}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <Button disabled={!canEdit || !mcpName.trim() || !mcpUrl.trim() || createMcp.isPending} onClick={() => createMcp.mutate()} type="button">
              <Plus data-icon="inline-start" />
              添加公共 MCP
            </Button>
          </div>
          <div className="flex flex-col gap-2">
            {(mcpServers.data ?? []).map((server) => (
              <div className="rounded-md border bg-background p-3" key={server.id}>
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{server.name}</p>
                    <p className="truncate text-xs text-muted-foreground">{server.url}</p>
                    {server.credential_name ? <p className="mt-1 text-xs text-muted-foreground"><KeyRound className="me-1 inline size-3" />{server.credential_name} ****{server.credential_last_four}</p> : null}
                  </div>
                  <StatusBadge tone="success">团队继承</StatusBadge>
                </div>
                <Button className="mt-2" disabled={!canEdit || deleteMcp.isPending} onClick={() => deleteMcp.mutate(server.id)} size="sm" type="button" variant="ghost">
                  移除
                </Button>
              </div>
            ))}
          </div>
        </CardContent>
      </LiquidCard>
    </div>
  );
}
```

- [ ] **Step 4: Run team UI test**

Run:

```bash
pnpm --dir apps/web test apps/web/src/features/teams/index.test.tsx -t "manages team public skills"
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/features/teams/components/team-capabilities-tab.tsx apps/web/src/features/teams/index.test.tsx
git commit -m "feat: add team public capability management"
```

## Task 9: Employee Instructions And Personal Capabilities UI

**Files:**
- Create: `apps/web/src/features/employees/components/instruction-files-panel.tsx`
- Create: `apps/web/src/features/employees/components/employee-capabilities-panel.tsx`
- Modify: `apps/web/src/features/employees/config.tsx`
- Modify: `apps/web/src/features/employees/config.test.tsx`

- [ ] **Step 1: Write failing employee config UI test**

Append to `apps/web/src/features/employees/config.test.tsx`:

```tsx
it("edits instruction files and manages personal capabilities with inherited preview", async () => {
  const fetcher = createEmployeeConfigFetcher({
    extraRoutes: {
      "GET /api/v1/digital-employees/employee-1/instructions": [
        { id: "instruction-1", path: "AGENTS.md", content: "# 原则", size_bytes: 8, checksum_sha256: "hash" },
      ],
      "PUT /api/v1/digital-employees/employee-1/instructions": { id: "instruction-1", path: "AGENTS.md", content: "# 新原则", size_bytes: 12, checksum_sha256: "hash2" },
      "GET /api/v1/digital-employees/employee-1/skills": [
        { skill: skillFixture("skill-team", "diagnose"), source_scope: "team", inherited: true, read_only: true },
        { skill: skillFixture("skill-personal", "review"), source_scope: "employee", inherited: false, read_only: false },
      ],
      "GET /api/v1/skills": [skillFixture("skill-extra", "sql-review")],
      "GET /api/v1/user-credentials?credential_type=mcp_token": [
        { id: "credential-1", name: "ops-token", credential_type: "mcp_token", last_four: "7890", status: "active" },
      ],
      "GET /api/v1/digital-employees/employee-1/mcp-bindings": [],
      "GET /api/v1/digital-employees/employee-1/effective-mcp-servers": [
        { id: "mcp-team", name: "ops-mcp", url: "https://mcp.example.com", source_scope: "team", inherited: true, status: "active" },
      ],
    },
  });
  const screen = await renderEmployeeConfigView(fetcher);

  await userEvent.click(screen.getByRole("tab", { name: "宪法/人格" }));
  await expect.element(screen.getByRole("button", { name: "AGENTS.md" })).toBeVisible();
  await userEvent.fill(screen.getByLabelText("Instructions 编辑器"), "# 新原则");
  await userEvent.click(screen.getByRole("button", { name: "保存文件" }));
  expect(fetcher).toHaveBeenCalledWith(expect.stringContaining("/instructions"), expect.objectContaining({ method: "PUT" }));

  await userEvent.click(screen.getByRole("tab", { name: "能力设置" }));
  await expect.element(screen.getByText("diagnose")).toBeVisible();
  await expect.element(screen.getByText("团队继承")).toBeVisible();
  await expect.element(screen.getByRole("button", { name: "移除 diagnose" })).toBeDisabled();
  await userEvent.click(screen.getByRole("button", { name: "安装 sql-review" }));
  expect(fetcher).toHaveBeenCalledWith(expect.stringContaining("/skills"), expect.objectContaining({ method: "POST" }));

  await userEvent.fill(screen.getByLabelText("个人 MCP 名称"), "personal-mcp");
  await userEvent.fill(screen.getByLabelText("个人 MCP URL"), "https://personal.example.com");
  await userEvent.click(screen.getByRole("combobox", { name: "个人 MCP 凭据" }));
  await userEvent.click(screen.getByRole("option", { name: "ops-token ****7890" }));
  await userEvent.click(screen.getByRole("button", { name: "添加个人 MCP" }));
  expect(fetcher).toHaveBeenCalledWith(expect.stringContaining("/mcp-bindings"), expect.objectContaining({ method: "POST" }));
});
```

Add this local helper in the test file:

```tsx
function skillFixture(id: string, name: string) {
  return {
    id,
    tenant_id: "tenant-1",
    slug: name,
    name,
    description: `${name} 描述`,
    version: "v1.0.0",
    source: "internal_market",
    risk_level: "low",
    status: "installed",
    icon_key: "blocks",
    color_token: "teal",
    tags: ["自动化"],
    files: [],
    team_bindings: [],
    agent_bindings: [],
  };
}
```

- [ ] **Step 2: Run the employee UI test and verify it fails**

Run:

```bash
pnpm --dir apps/web test apps/web/src/features/employees/config.test.tsx -t "edits instruction files"
```

Expected: FAIL because the config page still uses JSON textareas only.

- [ ] **Step 3: Create instruction files panel**

Create `apps/web/src/features/employees/components/instruction-files-panel.tsx`:

```tsx
import Editor from "@monaco-editor/react";
import { FileText, Plus, Save } from "lucide-react";
import { useEffect, useState } from "react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { LiquidCard, SemanticIconTile } from "@/components/superteam";
import { Button } from "@/components/ui/button";
import { CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { listInstructionFiles, upsertInstructionFile, type InstructionFile } from "@/lib/api/employees";
import type { ApiClientOptions } from "@/lib/api/client";
import { cn } from "@/lib/utils";

type InstructionFilesPanelProps = {
  apiOptions: ApiClientOptions;
  employeeId: string;
};

export function InstructionFilesPanel({ apiOptions, employeeId }: InstructionFilesPanelProps) {
  const queryClient = useQueryClient();
  const [selectedPath, setSelectedPath] = useState("AGENTS.md");
  const [newPath, setNewPath] = useState("SOUL.md");
  const [draftContent, setDraftContent] = useState("");
  const files = useQuery({
    queryKey: ["employee-instruction-files", employeeId],
    queryFn: () => listInstructionFiles(apiOptions, employeeId),
    placeholderData: keepPreviousData,
  });
  const selectedFile = (files.data ?? []).find((file) => file.path === selectedPath) ?? (files.data ?? [])[0];

  useEffect(() => {
    setDraftContent(selectedFile?.content ?? "");
    if (selectedFile?.path) {
      setSelectedPath(selectedFile.path);
    }
  }, [selectedFile?.content, selectedFile?.path]);

  const saveFile = useMutation({
    mutationFn: () => upsertInstructionFile(apiOptions, employeeId, { path: selectedPath, content: draftContent }),
    onSuccess: (file) => {
      queryClient.setQueryData<InstructionFile[]>(["employee-instruction-files", employeeId], (current = []) => {
        const exists = current.some((item) => item.path === file.path);
        if (exists) {
          return current.map((item) => (item.path === file.path ? file : item));
        }
        return [...current, file];
      });
    },
  });

  const createFile = () => {
    const trimmed = newPath.trim();
    if (!trimmed) return;
    setSelectedPath(trimmed);
    setDraftContent(`# ${trimmed.replace(/\.(md|txt)$/i, "")}\n`);
  };

  return (
    <div className="grid gap-4 xl:grid-cols-[280px_minmax(0,1fr)]">
      <LiquidCard className="rounded-lg">
        <CardHeader className="border-b">
          <div className="flex items-center gap-3">
            <SemanticIconTile tone="primary" size="sm"><FileText /></SemanticIconTile>
            <CardTitle className="text-base">文件</CardTitle>
          </div>
        </CardHeader>
        <CardContent className="flex flex-col gap-3 p-4">
          {(files.data ?? []).map((file) => (
            <button
              className={cn("rounded-md px-3 py-2 text-left text-sm hover:bg-muted", selectedPath === file.path && "bg-primary/10 text-primary")}
              key={file.path}
              onClick={() => setSelectedPath(file.path)}
              type="button"
            >
              {file.path}
            </button>
          ))}
          <div className="grid gap-2 border-t pt-3">
            <Label htmlFor="new-instruction-path">新文件</Label>
            <div className="flex gap-2">
              <Input id="new-instruction-path" value={newPath} onChange={(event) => setNewPath(event.target.value)} />
              <Button aria-label="新建文件" onClick={createFile} size="icon" type="button" variant="outline">
                <Plus />
              </Button>
            </div>
          </div>
        </CardContent>
      </LiquidCard>
      <LiquidCard className="min-w-0 rounded-lg">
        <CardHeader className="border-b">
          <div className="flex min-w-0 items-center justify-between gap-3">
            <CardTitle className="truncate text-base">{selectedPath}</CardTitle>
            <Button disabled={!selectedPath || saveFile.isPending} onClick={() => saveFile.mutate()} type="button">
              <Save data-icon="inline-start" />
              保存文件
            </Button>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          <div className="h-[560px] min-w-0 overflow-hidden rounded-b-lg border-t bg-background">
            <Editor
              height="100%"
              language={selectedPath.endsWith(".md") ? "markdown" : "plaintext"}
              onChange={(value) => setDraftContent(value ?? "")}
              options={{ fontSize: 13, minimap: { enabled: false }, scrollBeyondLastLine: false, wordWrap: "on" }}
              theme="vs"
              value={draftContent}
              aria-label="Instructions 编辑器"
            />
          </div>
        </CardContent>
      </LiquidCard>
    </div>
  );
}
```

- [ ] **Step 4: Create employee capabilities panel**

Create `apps/web/src/features/employees/components/employee-capabilities-panel.tsx`:

```tsx
import { KeyRound, Network, Plus, ShieldCheck, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { LiquidCard, SemanticIconTile, StatusBadge } from "@/components/superteam";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { ApiClientOptions } from "@/lib/api/client";
import { createEmployeeMcpBinding, deleteEmployeeMcpBinding, listEffectiveMcpServers, listEmployeeMcpBindings, listUserCredentials } from "@/lib/api/capabilities";
import { bindEmployeeSkill, listEmployeeSkills, listSkills, unbindEmployeeSkill } from "@/lib/api/skills";

type EmployeeCapabilitiesPanelProps = {
  apiOptions: ApiClientOptions;
  employeeId: string;
};

export function EmployeeCapabilitiesPanel({ apiOptions, employeeId }: EmployeeCapabilitiesPanelProps) {
  const queryClient = useQueryClient();
  const [mcpName, setMcpName] = useState("");
  const [mcpUrl, setMcpUrl] = useState("");
  const [credentialId, setCredentialId] = useState("none");
  const allSkills = useQuery({ queryKey: ["skills", ""], queryFn: () => listSkills(apiOptions), placeholderData: keepPreviousData });
  const effectiveSkills = useQuery({ queryKey: ["employee-skills", employeeId], queryFn: () => listEmployeeSkills(apiOptions, employeeId), placeholderData: keepPreviousData });
  const credentials = useQuery({ queryKey: ["user-credentials", "mcp_token"], queryFn: () => listUserCredentials(apiOptions, "mcp_token"), placeholderData: keepPreviousData });
  const personalMcp = useQuery({ queryKey: ["employee-mcp-bindings", employeeId], queryFn: () => listEmployeeMcpBindings(apiOptions, employeeId), placeholderData: keepPreviousData });
  const effectiveMcp = useQuery({ queryKey: ["effective-mcp-servers", employeeId], queryFn: () => listEffectiveMcpServers(apiOptions, employeeId), placeholderData: keepPreviousData });

  const effectiveSkillIds = useMemo(() => new Set((effectiveSkills.data ?? []).map((item) => item.skill.id)), [effectiveSkills.data]);
  const availableSkills = (allSkills.data ?? []).filter((skill) => !effectiveSkillIds.has(skill.id));

  const bindSkill = useMutation({
    mutationFn: (skillId: string) => bindEmployeeSkill(apiOptions, employeeId, skillId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["employee-skills", employeeId] });
    },
  });
  const unbindSkill = useMutation({
    mutationFn: (skillId: string) => unbindEmployeeSkill(apiOptions, employeeId, skillId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["employee-skills", employeeId] });
    },
  });
  const createMcp = useMutation({
    mutationFn: () => createEmployeeMcpBinding(apiOptions, employeeId, { name: mcpName, url: mcpUrl, credential_id: credentialId === "none" ? undefined : credentialId }),
    onSuccess: () => {
      setMcpName("");
      setMcpUrl("");
      setCredentialId("none");
      void queryClient.invalidateQueries({ queryKey: ["employee-mcp-bindings", employeeId] });
      void queryClient.invalidateQueries({ queryKey: ["effective-mcp-servers", employeeId] });
    },
  });
  const deleteMcp = useMutation({
    mutationFn: (bindingId: string) => deleteEmployeeMcpBinding(apiOptions, employeeId, bindingId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["employee-mcp-bindings", employeeId] });
      void queryClient.invalidateQueries({ queryKey: ["effective-mcp-servers", employeeId] });
    },
  });

  return (
    <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
      <div className="flex min-w-0 flex-col gap-4">
        <LiquidCard className="rounded-lg">
          <CardHeader className="border-b">
            <div className="flex items-center gap-3">
              <SemanticIconTile tone="primary" size="sm"><ShieldCheck /></SemanticIconTile>
              <CardTitle className="text-base">技能</CardTitle>
            </div>
          </CardHeader>
          <CardContent className="grid gap-3 p-4 md:grid-cols-2">
            {(effectiveSkills.data ?? []).map((item) => (
              <div className="flex min-w-0 items-center justify-between gap-3 rounded-md border bg-background p-3" key={`${item.source_scope}:${item.skill.id}`}>
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{item.skill.name}</p>
                  <div className="mt-2 flex gap-2">
                    <StatusBadge tone={item.inherited ? "info" : "success"}>{item.inherited ? "团队继承" : "个人技能"}</StatusBadge>
                    {item.skill.tags.slice(0, 2).map((tag) => <Badge key={tag} variant="outline">{tag}</Badge>)}
                  </div>
                </div>
                <Button aria-label={`移除 ${item.skill.name}`} disabled={item.read_only || unbindSkill.isPending} onClick={() => unbindSkill.mutate(item.skill.id)} size="icon" type="button" variant="ghost">
                  <Trash2 />
                </Button>
              </div>
            ))}
          </CardContent>
        </LiquidCard>

        <LiquidCard className="rounded-lg">
          <CardHeader className="border-b">
            <CardTitle className="text-base">个人技能市场</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-3 p-4 md:grid-cols-2">
            {availableSkills.map((skill) => (
              <div className="flex min-w-0 items-center justify-between gap-3 rounded-md border bg-background p-3" key={skill.id}>
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{skill.name}</p>
                  <p className="truncate text-xs text-muted-foreground">{skill.description}</p>
                </div>
                <Button disabled={bindSkill.isPending} onClick={() => bindSkill.mutate(skill.id)} size="sm" type="button" variant="outline">
                  安装 {skill.name}
                </Button>
              </div>
            ))}
          </CardContent>
        </LiquidCard>
      </div>

      <LiquidCard className="rounded-lg">
        <CardHeader className="border-b">
          <div className="flex items-center gap-3">
            <SemanticIconTile tone="warning" size="sm"><Network /></SemanticIconTile>
            <CardTitle className="text-base">个人 MCP</CardTitle>
          </div>
        </CardHeader>
        <CardContent className="flex flex-col gap-4 p-4">
          <div className="grid gap-3">
            <div className="grid gap-2">
              <Label htmlFor="employee-mcp-name">个人 MCP 名称</Label>
              <Input id="employee-mcp-name" value={mcpName} onChange={(event) => setMcpName(event.target.value)} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="employee-mcp-url">个人 MCP URL</Label>
              <Input id="employee-mcp-url" value={mcpUrl} onChange={(event) => setMcpUrl(event.target.value)} placeholder="https://mcp.example.com" />
            </div>
            <div className="grid gap-2">
              <Label>个人 MCP 凭据</Label>
              <Select value={credentialId} onValueChange={setCredentialId}>
                <SelectTrigger aria-label="个人 MCP 凭据">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">不使用凭据</SelectItem>
                  {(credentials.data ?? []).map((credential) => (
                    <SelectItem key={credential.id} value={credential.id}>{credential.name} ****{credential.last_four}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <Button disabled={!mcpName.trim() || !mcpUrl.trim() || createMcp.isPending} onClick={() => createMcp.mutate()} type="button">
              <Plus data-icon="inline-start" />
              添加个人 MCP
            </Button>
          </div>
          <div className="flex flex-col gap-2">
            {(effectiveMcp.data ?? []).map((server) => (
              <div className="rounded-md border bg-background p-3" key={`${server.source_scope}:${server.id}`}>
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{server.name}</p>
                    <p className="truncate text-xs text-muted-foreground">{server.url}</p>
                    {server.credential_name ? <p className="mt-1 text-xs text-muted-foreground"><KeyRound className="me-1 inline size-3" />{server.credential_name} ****{server.credential_last_four}</p> : null}
                  </div>
                  <StatusBadge tone={server.inherited ? "info" : "success"}>{server.inherited ? "团队继承" : "个人 MCP"}</StatusBadge>
                </div>
                {!server.inherited && personalMcp.data?.some((item) => item.id === server.id) ? (
                  <Button className="mt-2" disabled={deleteMcp.isPending} onClick={() => deleteMcp.mutate(server.id)} size="sm" type="button" variant="ghost">
                    移除
                  </Button>
                ) : null}
              </div>
            ))}
          </div>
        </CardContent>
      </LiquidCard>
    </div>
  );
}
```

- [ ] **Step 5: Wire panels into employee config page**

In `apps/web/src/features/employees/config.tsx`, import:

```tsx
import { Tabs, TabsContent } from "@/components/ui/tabs";
import { LiquidTabsList, LiquidTabsTrigger } from "@/components/superteam";
import { InstructionFilesPanel } from "@/features/employees/components/instruction-files-panel";
import { EmployeeCapabilitiesPanel } from "@/features/employees/components/employee-capabilities-panel";
```

Wrap the existing form in an advanced tab and add two new tabs:

```tsx
{employee.data ? (
  <Tabs defaultValue="instructions" className="space-y-4">
    <LiquidTabsList className="max-w-xl">
      <LiquidTabsTrigger value="instructions">宪法/人格</LiquidTabsTrigger>
      <LiquidTabsTrigger value="capabilities">能力设置</LiquidTabsTrigger>
      <LiquidTabsTrigger value="advanced">高级配置</LiquidTabsTrigger>
    </LiquidTabsList>
    <TabsContent value="instructions">
      <InstructionFilesPanel apiOptions={apiOptions} employeeId={employeeId} />
    </TabsContent>
    <TabsContent value="capabilities">
      <EmployeeCapabilitiesPanel apiOptions={apiOptions} employeeId={employeeId} />
    </TabsContent>
    <TabsContent value="advanced">
      {advancedConfigForm}
    </TabsContent>
  </Tabs>
) : null}
```

Define `advancedConfigForm` immediately before `return (` by assigning the current JSON configuration form JSX to a constant:

```tsx
const advancedConfigForm = (
  <form className="space-y-4" noValidate onSubmit={handleSubmit}>
    <Card>
      <CardHeader>
        <CardTitle>角色配置</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div>
          <Label htmlFor="role-profile">Role Profile (JSON)</Label>
          <Textarea id="role-profile" value={roleProfile} onChange={(e) => setRoleProfile(e.target.value)} rows={4} className="font-mono text-xs" />
        </div>
        <div>
          <Label htmlFor="constitution">Constitution Addendum (JSON)</Label>
          <Textarea id="constitution" value={constitutionAddendum} onChange={(e) => setConstitutionAddendum(e.target.value)} rows={4} className="font-mono text-xs" />
        </div>
      </CardContent>
    </Card>
    <Card>
      <CardHeader>
        <CardTitle>能力与策略</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div>
          <Label htmlFor="capability">Capability Selection (JSON)</Label>
          <Textarea id="capability" value={capabilitySelection} onChange={(e) => setCapabilitySelection(e.target.value)} rows={4} className="font-mono text-xs" />
        </div>
        <div>
          <Label htmlFor="context-policy">Context Policy Override (JSON)</Label>
          <Textarea id="context-policy" value={contextPolicyOverride} onChange={(e) => setContextPolicyOverride(e.target.value)} rows={4} className="font-mono text-xs" />
        </div>
        <div>
          <Label htmlFor="approval-policy">Approval Policy Override (JSON)</Label>
          <Textarea id="approval-policy" value={approvalPolicyOverride} onChange={(e) => setApprovalPolicyOverride(e.target.value)} rows={4} className="font-mono text-xs" />
        </div>
        <div>
          <Label htmlFor="output-contract">Output Contract Addendum (JSON)</Label>
          <Textarea id="output-contract" value={outputContractAddendum} onChange={(e) => setOutputContractAddendum(e.target.value)} rows={4} className="font-mono text-xs" />
        </div>
      </CardContent>
    </Card>
    <Card>
      <CardHeader>
        <CardTitle>预算策略</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        <Label htmlFor="config-daily-token-limit">每日 Token 预算上限</Label>
        <Input
          id="config-daily-token-limit"
          inputMode="numeric"
          min={1}
          onChange={(event) => {
            setDailyTokenLimit(event.target.value);
            setBudgetError("");
          }}
          placeholder="不填写表示无预算上限"
          type="number"
          aria-invalid={Boolean(budgetError)}
          value={dailyTokenLimit}
        />
        {budgetError ? <p className="text-sm text-destructive">{budgetError}</p> : null}
        <p className="text-xs text-muted-foreground">预算会进入新的配置版本，批准后生效。</p>
      </CardContent>
    </Card>
    <div className="flex gap-3">
      <Button type="submit" disabled={createRevision.isPending}>
        <Save />
        保存配置
      </Button>
      {createRevision.isSuccess ? <p className="text-sm text-green-600">配置已保存</p> : null}
      {createRevision.isError ? <p className="text-sm text-destructive">保存失败</p> : null}
    </div>
  </form>
);
```

- [ ] **Step 6: Run employee UI tests**

Run:

```bash
pnpm --dir apps/web test apps/web/src/features/employees/config.test.tsx -t "edits instruction files"
pnpm --dir apps/web test apps/web/src/features/employees/config.test.tsx
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/web/src/features/employees/components/instruction-files-panel.tsx apps/web/src/features/employees/components/employee-capabilities-panel.tsx apps/web/src/features/employees/config.tsx apps/web/src/features/employees/config.test.tsx
git commit -m "feat: add employee personal capability configuration"
```

## Task 10: End-To-End Verification And Changelog

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Run backend verification**

Run:

```bash
go test ./apps/control-plane/internal/storage ./apps/control-plane/internal/storage/queries ./apps/control-plane/internal/capability ./apps/control-plane/internal/skill ./apps/control-plane/internal/employee ./apps/control-plane/internal/api ./apps/control-plane/internal/app -count=1
```

Expected: PASS.

- [ ] **Step 2: Run frontend verification**

Run:

```bash
pnpm --dir apps/web test apps/web/src/lib/api/capabilities.test.ts apps/web/src/lib/api/skills.test.ts apps/web/src/lib/api/employees.test.ts apps/web/src/features/teams/index.test.tsx apps/web/src/features/employees/config.test.tsx
```

Expected: PASS.

- [ ] **Step 3: Run contract verification**

Run:

```bash
node scripts/verify-foundation-contracts.mjs
```

Expected: PASS.

- [ ] **Step 4: Add changelog entry**

Get local Asia/Shanghai time:

```bash
TZ=Asia/Shanghai date "+%Y-%m-%d %H:%M"
```

Add a new heading to the top of `CHANGELOG.md` whose heading text is exactly the command output, followed by this bullet:

```markdown
- 实现双层技能管理：团队公共 Skills/MCP、数字员工个人 Instructions/Skills/MCP、个人凭据池和只读继承预览。
```

- [ ] **Step 5: Run final full checks**

Run:

```bash
go test ./apps/control-plane/... -count=1
pnpm --dir apps/web test
```

Expected: PASS. If Playwright browser dependencies are missing, record the exact missing dependency message and keep the Vitest Browser command output as the frontend verification evidence.

- [ ] **Step 6: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: record dual layer skill management"
```

## Self-Review

- Spec coverage:
  - 团队公共 Skills：Task 4 backend routes，Task 8 UI。
  - 团队公共 MCP：Task 1/2 storage，Task 3/6 backend，Task 8 UI。
  - 数字员工 Instructions：Task 5 backend，Task 9 UI。
  - 数字员工个人 Skills：Task 4 backend，Task 9 UI。
  - 数字员工个人 MCP：Task 1/2/3/6 backend，Task 9 UI。
  - 团队强制 + 个人特有合并预览：Task 4 effective skills，Task 2/3 effective MCP，Task 9 UI。
  - 凭据池和 MCP token 注入：Task 3 service and `BuildMCPAuthorizationHeader`。
  - 权限演进入口：Task 6 authz actions。
- Placeholder scan:
  - Plan avoids blocked-marker language and unresolved type names in code snippets.
  - Task 5 now uses deterministic sqlc queries for instruction files instead of a raw DB access branch.
- Type consistency:
  - Web API uses `McpServer`, backend response uses `mcpServerResponse`; JSON fields match `credential_id`, `source_scope`, `inherited`.
  - Skill merge uses `EffectiveEmployeeSkill` consistently in Go and TypeScript.
  - Instruction APIs use `InstructionFile` and `UpsertInstructionFile` consistently.
