# 团队治理瘦身（Plan A）Implementation Plan
> 复核状态：已实现（基于CHANGELOG证据与锚点抽查）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把"团队治理 revision + 员工 effective-config 审批门禁"整条旧模型全栈删除，团队降为「公共技能(绑定表) + MCP(绑定表) + 宪法(新列)」，员工能力改为「团队基线 ∪ 个人」运行时合成。

**Architecture:** 复用现有 `skill_team_bindings`(改名 `team_skill_bindings`)/`team_mcp_bindings` 作为团队能力基线，新增 `tenant_teams.constitution` 列承载宪法；删除 `tenant_team_config_revisions`、`digital_employee_effective_configs` 及其 `HasApprovedEffectiveConfig` 派发前置门；契约按纵向切片随各 Go 任务同步精简并重生成。

**Tech Stack:** Go (control-plane, chi, sqlc, Atlas migrations), OpenAPI 契约 + oapi-codegen, React + TanStack Router/Query + Vitest (apps/web)。

## Global Constraints

- 依据 spec：`docs/superpowers/specs/2026-07-08-team-governance-slimming-and-project-runtime-binding-design.md`。
- 迁移唯一目录 `apps/control-plane/internal/storage/migrations/`；新增/修改迁移后更新 `atlas.sum`，用 `make -C apps/control-plane migrate-validate` 校验。
- Web 测试只用 `corepack pnpm --filter ./apps/web run test`；禁止 `npx playwright install` / `npx vitest run`。
- Web 内部跳转用 TanStack Router `Link`/`navigate`。
- 契约改动后必须重新生成并跑契约验证。
- 命名约定 team-first：`team_<subject>_bindings`。
- 保留 `digital_employee_config_revisions`（员工自身配置事实源）；只删团队 revision 与 effective-config。
- 每个任务结束时仓库可编译、相关测试通过、独立提交。
- 提交信息结尾附：`Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`。

## File Structure（本计划触及）

- 迁移：`apps/control-plane/internal/storage/migrations/046_*.sql`、`047_*.sql`、`atlas.sum`
- sqlc：`apps/control-plane/internal/storage/queries/{skill_runtime,employee_execution,tenant_team_config,digital_employee_config}.sql(.go)`
- Go employee：`apps/control-plane/internal/employee/{service,repository,pg_repository,handler,run_service,types}.go`
- Go tenant：`apps/control-plane/internal/tenant/{types,service,repository,pg_repository,handler}.go`
- Go project：`apps/control-plane/internal/project/service.go`
- 路由：`apps/control-plane/internal/api/server.go`
- 契约：`contracts/control-plane/openapi.yaml` + 生成物 `apps/control-plane/gen/control_plane.gen.go`、`apps/control-plane/internal/api/gen/control_plane.gen.go`
- Web：`apps/web/src/features/teams/components/team-governance-tab.tsx`、`team-detail-layout.tsx`、`apps/web/src/features/employees/create.tsx`、`apps/web/src/lib/api/teams.ts`

---

### Task 1: 迁移 M1 — 团队宪法列 + 回填 + 绑定表改名

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/046_team_constitution_and_skill_binding_rename.sql`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`
- Test: `apps/control-plane/internal/storage/migrations_test.go`

**Interfaces:**
- Produces: 列 `tenant_teams.constitution jsonb`（默认 `{}`，回填自 active revision）；表 `team_skill_bindings`（原 `skill_team_bindings`）。

- [ ] **Step 1: 写迁移 SQL**

`046_team_constitution_and_skill_binding_rename.sql`：
```sql
-- Team constitution becomes a first-class team column; backfill from the
-- currently-active governance revision. Rename skill_team_bindings to the
-- team-first convention (team_<subject>_bindings).

ALTER TABLE tenant_teams
    ADD COLUMN IF NOT EXISTS constitution JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE tenant_teams t
SET constitution = r.constitution
FROM tenant_team_config_revisions r
WHERE r.tenant_id = t.tenant_id
  AND r.team_id = t.id
  AND r.status = 'active';

ALTER TABLE skill_team_bindings RENAME TO team_skill_bindings;
```

- [ ] **Step 2: 重算 atlas.sum**

Run: `make -C apps/control-plane migrate-validate`
Expected: 校验通过（可重放、无 hash 冲突）；`atlas.sum` 更新。若本地非 Docker dev 库，按 DATABASE_DESIGN.md 用 `DEV_URL` 覆盖。

- [ ] **Step 3: 写迁移断言测试**

在 `migrations_test.go` 增加：应用全部迁移后，`tenant_teams` 含 `constitution` 列、`team_skill_bindings` 存在、`skill_team_bindings` 不存在。
```go
func TestMigration046TeamConstitutionAndRename(t *testing.T) {
    db := applyAllMigrations(t) // 复用现有 helper
    assertColumnExists(t, db, "tenant_teams", "constitution")
    assertTableExists(t, db, "team_skill_bindings")
    assertTableAbsent(t, db, "skill_team_bindings")
}
```
（若 `assertColumnExists`/`assertTableExists`/`assertTableAbsent` helper 不存在，用现有 migrations_test 的查询模式实现：查 `information_schema.columns` / `information_schema.tables`。）

- [ ] **Step 4: 跑测试**

Run: `go test ./apps/control-plane/internal/storage/... -run TestMigration046 -v`
Expected: PASS

- [ ] **Step 5: 提交**
```bash
git add apps/control-plane/internal/storage/migrations/046_team_constitution_and_skill_binding_rename.sql apps/control-plane/internal/storage/migrations/atlas.sum apps/control-plane/internal/storage/migrations_test.go
git commit -m "feat(migrate): add tenant_teams.constitution and rename skill_team_bindings"
```

---

### Task 2: sqlc/Go — `skill_team_bindings` → `team_skill_bindings` 全量改名

**Files:**
- Modify: `apps/control-plane/internal/storage/queries/skill_runtime.sql`
- Modify: `apps/control-plane/internal/storage/queries/employee_execution.sql`
- Regenerate: `apps/control-plane/internal/storage/queries/skill_runtime.sql.go`、`employee_execution.sql.go`
- Modify tests: `apps/control-plane/internal/storage/queries/queries_test.go`、`employee_execution_static_test.go`

**Interfaces:**
- Consumes: Task 1 的 `team_skill_bindings` 表。
- Produces: 全仓库不再有 `skill_team_bindings` 字面量。

- [ ] **Step 1: 定位所有引用**

Run: `rg -n "skill_team_bindings" apps/control-plane`
Expected: 命中 `skill_runtime.sql`、`employee_execution.sql`、两个 `.sql.go`、两个测试。

- [ ] **Step 2: 改 SQL 源**

在 `skill_runtime.sql`、`employee_execution.sql` 中把 `skill_team_bindings` 全部替换为 `team_skill_bindings`（仅表名，查询逻辑不变）。

- [ ] **Step 3: 重新生成 sqlc**

Run: `make -C apps/control-plane generate-sqlc`
Expected: `.sql.go` 中表名同步更新。

- [ ] **Step 4: 改测试里的字面量**

`queries_test.go`、`employee_execution_static_test.go` 中的 `skill_team_bindings` → `team_skill_bindings`。

- [ ] **Step 5: 跑测试 + 构建**

Run: `go build ./apps/control-plane/... && go test ./apps/control-plane/internal/storage/queries/... -v`
Expected: 构建通过，PASS，且 `rg -n "skill_team_bindings" apps/control-plane` 无输出。

- [ ] **Step 6: 提交**
```bash
git add apps/control-plane/internal/storage/queries
git commit -m "refactor(storage): rename skill_team_bindings to team_skill_bindings"
```

---

### Task 3a: employee 仓库 — 团队基线读取器（替换 GetCurrentTeamConfigRevision）

**Files:**
- Modify: `apps/control-plane/internal/employee/repository.go`（接口）
- Modify: `apps/control-plane/internal/employee/pg_repository.go`（实现，约 147）
- Modify: `apps/control-plane/internal/employee/types.go`（新增 `TeamBaseline`）
- Test: `apps/control-plane/internal/employee/pg_repository_test.go`（或对应 repo 测试）

**Interfaces:**
- Consumes: `team_skill_bindings`、`team_mcp_bindings`、`tenant_teams.constitution`。
- Produces：
```go
type TeamBaseline struct {
    Constitution map[string]any // { hard_rules: []string }
    Skills       []string       // 团队公共技能标识
    MCPServers   []string       // 团队公共 MCP 标识
}
// 接口方法（替换 GetCurrentTeamConfigRevision）：
GetTeamBaseline(ctx context.Context, tenantID, teamID uuid.UUID) (TeamBaseline, error)
```

- [ ] **Step 1: 写失败测试**

在 repo 测试中：给定一个 team 绑定了 2 个技能、1 个 MCP、constitution 有 1 条 hard_rule，`GetTeamBaseline` 返回对应集合。
```go
func TestGetTeamBaseline(t *testing.T) {
    repo, ctx, tenantID, teamID := seedTeamWithBindings(t) // 绑定 skillA,skillB / mcpX / hard_rules:["r1"]
    got, err := repo.GetTeamBaseline(ctx, tenantID, teamID)
    require.NoError(t, err)
    require.ElementsMatch(t, []string{"skillA", "skillB"}, got.Skills)
    require.ElementsMatch(t, []string{"mcpX"}, got.MCPServers)
    require.Equal(t, []any{"r1"}, got.Constitution["hard_rules"])
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./apps/control-plane/internal/employee/... -run TestGetTeamBaseline -v`
Expected: FAIL（`GetTeamBaseline` 未定义）。

- [ ] **Step 3: 实现 `GetTeamBaseline`**

`types.go` 增加 `TeamBaseline`。`pg_repository.go`：从 `team_skill_bindings`（active）取 skill 标识、`team_mcp_bindings`（active）取 mcp 标识、`tenant_teams.constitution` 取宪法 jsonb。用现有 sqlc 查询或新增查询（在 `queries/` 增 `GetTeamSkillBindings`/`GetTeamMcpBindings`/`GetTeamConstitution`，随 sqlc 生成）。删除 `GetCurrentTeamConfigRevision` 实现与接口方法。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./apps/control-plane/internal/employee/... -run TestGetTeamBaseline -v`
Expected: PASS

- [ ] **Step 5: 提交**
```bash
git add apps/control-plane/internal/employee apps/control-plane/internal/storage/queries
git commit -m "feat(employee): add GetTeamBaseline sourcing from binding tables"
```

---

### Task 3b: employee service — 拆掉治理闸/白名单，改用团队基线 + 平台全量池

**Files:**
- Modify: `apps/control-plane/internal/employee/service.go`（217/220、346-366、400-421、489-498、622、949-953）
- Modify: `apps/control-plane/internal/employee/handler.go`（1040 `AllowedSkills` 等选项字段）
- Modify: `contracts/control-plane/openapi.yaml`（`DigitalEmployeeCreateTeamConfig`、`DigitalEmployeeCapabilityOptions` 精简）
- Regenerate: 两个 `control_plane.gen.go`
- Test: `apps/control-plane/internal/employee/service_test.go`

**Interfaces:**
- Consumes: Task 3a `GetTeamBaseline`。
- Produces: `CreateDigitalEmployee` 不再需要 active 团队 revision；`DigitalEmployeeCreateOptions.employee_types`/`capability_options` 恒为平台全量；`team_config` 精简为 `{ team_id?, constitution, skills[], mcp_servers[] }`（skills/mcp 来自基线）。

- [ ] **Step 1: 写失败测试**

- 建员工时团队无 revision 也应成功（不再 `ErrEffectiveConfigRequired`）。
- `GetDigitalEmployeeCreateOptions` 的 `employee_types` = 平台全量 `DefaultEmployeeTypeDefinitions()`，与团队无关。
- `team_config.skills`/`mcp_servers` = 团队基线（来自绑定表），非 `capability_policy.allowed_*`。
```go
func TestCreateEmployeeWithoutTeamRevision(t *testing.T) {
    svc, ctx, tenantID, teamID := newEmployeeServiceWithTeamNoRevision(t)
    _, err := svc.CreateDigitalEmployee(ctx, validCreateReq(tenantID, teamID))
    require.NoError(t, err)
}
func TestCreateOptionsUsePlatformFullEmployeeTypes(t *testing.T) {
    svc, ctx, tenantID, teamID := newEmployeeServiceWithTeamNoRevision(t)
    opts, err := svc.GetDigitalEmployeeCreateOptions(ctx, tenantID, &teamID)
    require.NoError(t, err)
    require.Len(t, opts.EmployeeTypes, len(employee.DefaultEmployeeTypeDefinitions()))
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./apps/control-plane/internal/employee/... -run 'TestCreateEmployeeWithoutTeamRevision|TestCreateOptionsUsePlatformFullEmployeeTypes' -v`
Expected: FAIL（当前仍有 revision 闸/白名单裁剪）。

- [ ] **Step 3: 改 service.go**

删除：
- `ErrEffectiveConfigRequired` 两处闸（217-220、489-492）——改为团队存在即可（`EnsureTeamExists` 保留），团队基线用 `GetTeamBaseline`。
- `validateEmployeeTypeAllowedByTeamConfig`（498 调用 + 622 定义）、`allowedEmployeeTypesFromTeamConfig`（400）、`employeeTypesForTeamConfig` 的团队裁剪 → `employee_types` 恒为 `DefaultEmployeeTypeDefinitions()`。
- `capability_policy.allowed_*` 天花板过滤（346、364、366、949-953）与外部能力（348、366、952、`AllowedExternalCaps`）。
- `capabilityOptionsForTeamConfig` → 平台全量池；`team_config.skills/mcp_servers` = `TeamBaseline.Skills/MCPServers`。
`handler.go:1040` 起的 options 响应结构体删掉 `AllowedSkills`/`AllowedMcp`/`AllowedExternalCaps`/`AllowedEmployeeTypes`/`AllowedProviderTypes` 字段。

- [ ] **Step 4: 精简契约 + 重生成**

`openapi.yaml`：`DigitalEmployeeCreateTeamConfig` 删 `allowed_*`(5个) + 6 policy blob + `revision_number` + `status`，保留 `{ id, tenant_id, team_id?, constitution, skills[], mcp_servers[] }`；`DigitalEmployeeCapabilityOptions` 删 `external_capabilities`。
Run: `corepack pnpm generate:control-plane`（`go generate ./internal/api`，重生成 control_plane gen.go）；`git diff` 确认两个 gen.go 同步。

- [ ] **Step 5: 跑测试 + 契约验证**

Run: `go test ./apps/control-plane/internal/employee/... -v && corepack pnpm verify:contracts`
Expected: PASS；契约验证通过。

- [ ] **Step 6: 提交**
```bash
git add apps/control-plane contracts
git commit -m "feat(employee): drop team-governance gates and whitelists from create flow"
```

---

### Task 4: 删除 effective-config 审批子系统 + 派发前置门

**Files:**
- Modify: `apps/control-plane/internal/employee/run_service.go`（822、860、902、1031）
- Modify: `apps/control-plane/internal/employee/handler.go`（600、638、680）
- Modify: `apps/control-plane/internal/employee/pg_repository.go`（890、1898 快照构建 + effective-config CRUD）
- Modify: `apps/control-plane/internal/employee/repository.go`（接口去掉 effective-config 方法）
- Modify: `apps/control-plane/internal/project/service.go`（55、993-996）
- Modify: `apps/control-plane/internal/api/server.go`（251-254）
- Modify: `contracts/control-plane/openapi.yaml`（effective-config schema/路径）+ 重生成
- Test: `run_service` 相关测试、`project/service_test.go`

**Interfaces:**
- Produces: 派发/运行不再要求 `HasApprovedEffectiveConfig`；无 effective-config 路由。保留 `digital_employee_config_revisions`。

- [ ] **Step 1: 写失败测试**

派发/运行前置不再因缺少 approved effective config 而阻断。
```go
func TestRunAllowedWithoutApprovedEffectiveConfig(t *testing.T) {
    svc, ctx, args := newRunServiceReadyToDispatch(t) // 员工无 approved effective config
    err := svc.preflightForRun(ctx, args) // 或对应导出的前置校验入口
    require.NoError(t, err)
}
```
（若 preflight 未导出，测试通过公开的 dispatch/run 入口断言不再返回 `ErrEffectiveConfigRequired`。）

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./apps/control-plane/internal/employee/... -run TestRunAllowedWithoutApprovedEffectiveConfig -v`
Expected: FAIL（当前返回 `ErrEffectiveConfigRequired`）。

- [ ] **Step 3: 删除代码**

- `run_service.go`：删 822/860 的 `HasApprovedEffectiveConfig` 阻断与 `ErrEffectiveConfigRequired`；902/1031 的相关字段与遥测项。
- `project/service.go`：删 55 `EffectiveConfigStatus` 字段与 993-996 的 `employee_workspace_pending` gate。
- `handler.go`：删 `PreviewDigitalEmployeeEffectiveConfig`/`ApproveDigitalEmployeeEffectiveConfig`/`GetDigitalEmployeeEffectiveConfig`（600/638/680）。
- `pg_repository.go`：删 effective-config 快照构建与读写（890、1898 及相关方法）。
- `repository.go`：接口去掉 effective-config 方法。
- `server.go`：删 251-254 路由。

- [ ] **Step 4: 精简契约 + 重生成**

`openapi.yaml` 删 effective-config schema 与 3 条路径；`corepack pnpm generate:control-plane`。

- [ ] **Step 5: 跑测试 + 构建 + 契约验证**

Run: `go build ./apps/control-plane/... && go test ./apps/control-plane/internal/employee/... ./apps/control-plane/internal/project/... -v && corepack pnpm verify:contracts`
Expected: PASS；`rg -n "HasApprovedEffectiveConfig|EffectiveConfigStatus" apps/control-plane` 无非注释残留。

- [ ] **Step 6: 提交**
```bash
git add apps/control-plane contracts
git commit -m "feat(employee): remove effective-config approval subsystem and dispatch gate"
```

---

### Task 5: 删除团队治理 revision 服务/路由，新增团队宪法读写

**Files:**
- Modify: `apps/control-plane/internal/tenant/types.go`（52/95/164/260 等 revision 类型；`Team` 加 `Constitution`）
- Modify: `apps/control-plane/internal/tenant/{service,repository,pg_repository,handler}.go`
- Modify: `apps/control-plane/internal/api/server.go`（347-362）
- Modify: `apps/control-plane/internal/storage/queries/tenant_team_config.sql`（删 revision 查询）+ 重生成
- Modify: `contracts/control-plane/openapi.yaml`（删 `TeamConfigRevision`/`CreateTeamConfigRevisionRequest` + 相关路径；`Team` 加 `constitution`；加团队宪法更新端点）+ 重生成
- Test: `apps/control-plane/internal/tenant/service_test.go`

**Interfaces:**
- Produces：
```go
// tenant.Team 增加：
Constitution map[string]any
// service/repository：
UpdateTeamConstitution(ctx context.Context, tenantID, teamID uuid.UUID, constitution map[string]any) (Team, error)
GetTeam(...) // 返回含 Constitution
```
- HTTP：`PATCH /api/v1/teams/{teamId}/constitution`（body `{ hard_rules: []string }`）。

- [ ] **Step 1: 写失败测试**

```go
func TestUpdateTeamConstitution(t *testing.T) {
    svc, ctx, tenantID, teamID := newTenantServiceWithTeam(t)
    team, err := svc.UpdateTeamConstitution(ctx, tenantID, teamID, map[string]any{"hard_rules": []string{"r1", "r2"}})
    require.NoError(t, err)
    require.Equal(t, []string{"r1", "r2"}, toStringSlice(team.Constitution["hard_rules"]))
    reload, _ := svc.GetTeam(ctx, tenantID, teamID)
    require.Len(t, toStringSlice(reload.Constitution["hard_rules"]), 2)
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./apps/control-plane/internal/tenant/... -run TestUpdateTeamConstitution -v`
Expected: FAIL（方法未定义）。

- [ ] **Step 3: 实现宪法读写 + 删除 revision**

- `types.go`：`Team` 加 `Constitution map[string]any`；删除 `TeamConfigRevision`、`TeamConfigRevisionStatus`、draft/diff 相关类型（52/95/164/260）。
- `pg_repository.go`：`GetTeam` 读 `tenant_teams.constitution`；实现 `UpdateTeamConstitution`（写 `tenant_teams.constitution`）；删除 revision CRUD/draft/approve/reject/diff。
- `service.go`/`handler.go`：删除 `CreateTeamConfigRevision`/`GetCurrentTeamConfigRevision`/`ListGovernanceDrafts`/`CreateGovernanceDraft`/`UpdateGovernanceDraft`/`ApproveGovernanceDraft`/`RejectGovernanceDraft`/`PreviewGovernanceDiff`；加 `UpdateTeamConstitution` handler。
- `server.go`：删 347-362，新增 `r.Patch("/teams/{teamId}/constitution", s.tenantHandler.UpdateTeamConstitution)`。
- `tenant_team_config.sql`：删 revision 查询，`make -C apps/control-plane generate-sqlc`。

- [ ] **Step 4: 精简契约 + 重生成**

`openapi.yaml`：删 `TeamConfigRevision`/`CreateTeamConfigRevisionRequest` 及 config-revisions/governance 路径；`Team` 加 `constitution`；加 `PATCH /teams/{teamId}/constitution`。`corepack pnpm generate:control-plane`。

- [ ] **Step 5: 跑测试 + 构建 + 契约验证**

Run: `go build ./apps/control-plane/... && go test ./apps/control-plane/internal/tenant/... -v && corepack pnpm verify:contracts`
Expected: PASS；`rg -n "TeamConfigRevision|GovernanceDraft" apps/control-plane/internal` 无残留。

- [ ] **Step 6: 提交**
```bash
git add apps/control-plane contracts
git commit -m "feat(tenant): replace team governance revisions with constitution column"
```

---

### Task 6: 迁移 M2 — 删除 tenant_team_config_revisions 与 digital_employee_effective_configs

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/047_drop_team_governance_and_effective_config.sql`
- Modify: `atlas.sum`
- Test: `migrations_test.go`

**Interfaces:**
- Consumes: Task 3/4/5（代码已不再引用这两张表）。

- [ ] **Step 1: 确认无代码引用**

Run: `rg -n "tenant_team_config_revisions|digital_employee_effective_configs" apps/control-plane --glob '!*_test.go'`
Expected: 仅迁移文件命中（001/003 历史 + 047），无 sqlc/Go 逻辑引用。若仍有命中，回到对应任务清理后再继续。

- [ ] **Step 2: 写迁移 SQL**

`047_drop_team_governance_and_effective_config.sql`：
```sql
-- Team governance revisions and the effective-config approval snapshot are
-- replaced by team baseline (bindings + constitution) composed at read time.
DROP TABLE IF EXISTS digital_employee_effective_configs;
DROP TABLE IF EXISTS tenant_team_config_revisions;
```

- [ ] **Step 3: 校验迁移**

Run: `make -C apps/control-plane migrate-validate`
Expected: 通过；`atlas.sum` 更新。

- [ ] **Step 4: 断言测试**
```go
func TestMigration047DropsGovernanceTables(t *testing.T) {
    db := applyAllMigrations(t)
    assertTableAbsent(t, db, "tenant_team_config_revisions")
    assertTableAbsent(t, db, "digital_employee_effective_configs")
    assertTableExists(t, db, "digital_employee_config_revisions") // 保留
}
```

- [ ] **Step 5: 跑测试**

Run: `go test ./apps/control-plane/internal/storage/... -run TestMigration047 -v`
Expected: PASS

- [ ] **Step 6: 提交**
```bash
git add apps/control-plane/internal/storage/migrations/047_drop_team_governance_and_effective_config.sql apps/control-plane/internal/storage/migrations/atlas.sum apps/control-plane/internal/storage/migrations_test.go
git commit -m "feat(migrate): drop team governance revisions and effective-config tables"
```

---

### Task 7: Web — 团队详情 governance tab 改为宪法编辑；删除治理 API 客户端

**Files:**
- Delete: `apps/web/src/features/teams/components/team-governance-tab.tsx`
- Create: `apps/web/src/features/teams/components/team-constitution-tab.tsx`
- Modify: `apps/web/src/features/teams/components/team-detail-layout.tsx`（17-19、154-160、governance TabsContent）
- Modify: `apps/web/src/lib/api/teams.ts`（删 revision/draft/diff 类型与函数；加 `updateTeamConstitution`）
- Test: `apps/web/src/features/teams/components/create-team-page.test.tsx` 或新增 tab 测试

**Interfaces:**
- Consumes: 后端 `PATCH /teams/{teamId}/constitution` + `Team.constitution`。
- Produces: 团队详情用「概览 / 能力 / 宪法」三 tab（能力 tab 仍是现有绑定表 UI）。

- [ ] **Step 1: 写失败测试**

新 `team-constitution-tab.test.tsx`：渲染时展示既有 hard_rules，编辑并保存调用 `updateTeamConstitution`。
```tsx
it("saves edited constitution hard rules", async () => {
  render(<TeamConstitutionTab apiOptions={opts} teamId="t1" constitution={{ hard_rules: ["r1"] }} canEdit />);
  await userEvent.type(screen.getByLabelText("团队宪法"), "\nr2");
  await userEvent.click(screen.getByRole("button", { name: "保存宪法" }));
  expect(updateTeamConstitutionSpy).toHaveBeenCalledWith(opts, "t1", { hard_rules: ["r1", "r2"] });
});
```

- [ ] **Step 2: 跑测试确认失败**

Run: `corepack pnpm --filter ./apps/web run test -- team-constitution-tab`
Expected: FAIL（组件/函数不存在）。

- [ ] **Step 3: 实现宪法 tab + API + 接线**

- `teams.ts`：删 `TeamConfigRevision*`/`GovernanceDraftInput`/`create/get/update/approve/reject/previewDiff` 治理函数；加 `updateTeamConstitution(opts, teamId, { hard_rules })` 打 `PATCH /teams/{teamId}/constitution`。
- 新建 `team-constitution-tab.tsx`：一个 textarea（每行一条 hard_rule）+ 保存按钮，复用 `team-governance-tab` 里 `lineList/arrayText` 逻辑，去掉 approval/diff/JSON 预览。
- `team-detail-layout.tsx`：`import` 换成 `TeamConstitutionTab`；governance TabsTrigger 文案改「宪法」，TabsContent 渲染新组件（传 `overview.constitution`）；删除 `canCreateGovernance`/`canApproveGovernance` 中已无意义的部分，`canEdit` 用 `team.governance.edit`（或按现有权限键保留一个编辑权限）。
- 删除 `team-governance-tab.tsx`。

- [ ] **Step 4: 跑测试**

Run: `corepack pnpm --filter ./apps/web run test -- team-constitution-tab team-detail`
Expected: PASS

- [ ] **Step 5: 提交**
```bash
git add apps/web/src/features/teams apps/web/src/lib/api/teams.ts
git commit -m "feat(web): replace team governance tab with constitution editor"
```

---

### Task 8: Web — 员工创建三步向导，删治理步/外部能力，只读继承基线

**Files:**
- Modify: `apps/web/src/features/employees/create.tsx`（74 configSteps、1516-1531 治理步、1396-1431 继承展示、外部能力项、validateStep）
- Modify: `apps/web/src/features/employees/create.test.tsx`
- Modify: `apps/web/src/features/employees/config.tsx`（若共享治理/外部能力字段）

**Interfaces:**
- Consumes: Task 3b 精简后的 `DigitalEmployeeCreateOptions`（无 allowed_*/external_capabilities，`team_config` 为基线）。
- Produces: `configSteps = ["身份", "能力", "Provider 类型"]`；继承能力只读；无外部能力。

- [ ] **Step 1: 改测试预期（先失败）**

`create.test.tsx`：向导为三步、无「治理」步、无「外部能力」项；继承技能/MCP 只读展示；缺 team revision 不阻断创建。
```tsx
it("renders a three-step wizard without a governance step", () => {
  renderCreate(optionsWithBaseline());
  expect(screen.queryByText("治理")).not.toBeInTheDocument();
  expect(screen.getByText("Provider 类型")).toBeInTheDocument();
});
it("shows inherited team skills as read-only", () => {
  renderCreate(optionsWithBaseline({ skills: ["skillA"] }));
  expect(screen.getByText("团队继承能力")).toBeInTheDocument();
  expect(screen.queryByRole("checkbox", { name: "skillA" })).not.toBeInTheDocument();
});
```

- [ ] **Step 2: 跑测试确认失败**

Run: `corepack pnpm --filter ./apps/web run test -- employees/create`
Expected: FAIL

- [ ] **Step 3: 改 create.tsx**

- `configSteps` → `["身份", "能力", "Provider 类型"]`；删除 stepIndex/validateStep 中 "治理" 分支与 1516-1531 治理面板；员工自有 approval/context 默认合并入「能力」步（保留只读默认展示或简单编辑）。
- 删除外部能力相关：`inheritedCapabilities.enabled_external_capabilities` 展示、`enabled_external_capabilities` 选择、SummaryItem「外部能力」。
- 继承能力保持只读：`inheritedCapabilitySelection(options)` 来源已是基线；`withoutValues` 从平台全量池排除继承项后作为「可追加」项。
- 删除「允许员工类型/允许 Provider/团队治理版本」等 SummaryItem。

- [ ] **Step 4: 跑测试**

Run: `corepack pnpm --filter ./apps/web run test -- employees/create employees/config`
Expected: PASS

- [ ] **Step 5: 提交**
```bash
git add apps/web/src/features/employees
git commit -m "feat(web): slim employee create to three steps with read-only inheritance"
```

---

### Task 9: 真实端到端验证（CLAUDE.md 强制完成条件）

**Files:** 无（验证任务）。

**Interfaces:** Consumes 全部前序任务。

- [ ] **Step 1: 迁移 + 重启**

Run: `scripts/dev-services.sh restart control-plane`
Expected: Atlas 迁移自动执行（046/047），Control Plane 起来；`scripts/dev-services.sh status` 全绿。

- [ ] **Step 2: Web 真实建团队**

用 Chrome plug 或真实 UI：建团队 → 绑定 ≥1 公共技能、≥1 公共 MCP → 宪法 tab 加 hard_rule 保存。
Expected: 保存成功，刷新后仍在（走真实 `PATCH /teams/{id}/constitution`，非 mock）。

- [ ] **Step 3: Web 真实建员工**

在该团队下建员工：向导三步、无治理步、无外部能力；「能力」步展示团队继承技能/MCP（只读）+ 平台全量可追加项；员工类型/Provider 为平台全量。
Expected: 创建成功，员工有效能力 = 团队基线 ∪ 个人（curl `GET` 员工详情/有效能力接口确认）。

- [ ] **Step 4: 派发不被 effective-config 门挡**

对该员工发起一次任务派发（真实项目/任务路径）。
Expected: 不再返回 `employee_workspace_pending` / `ErrEffectiveConfigRequired`；派发进入正常调度（受 runtime 可用性等真实条件约束，但非被审批门阻断）。

- [ ] **Step 5: 收尾检查**

Run: `.codex/skills/superteam-completion-check/SKILL.md` 流程；`rg -n "TeamConfigRevision|HasApprovedEffectiveConfig|skill_team_bindings|allowed_external_capabilities" apps contracts` 应无遗留。
Expected: 完成前检查通过；无旧模型残留。

- [ ] **Step 6: 合并与分支收尾**

按 CLAUDE.md：合并 main 后基于 main 当前代码再走一次真实仿真验证通过，才删分支/worktree；若验证阻塞则标记阻塞、不声明完成。

## Self-Review

- **Spec 覆盖**：§2.1 团队终态→T1/T2/T3a/T5/T7；§1.2 effective-config 删除→T4/T6/T9-Step4；§2.3 继承合成→T3a/T3b/T8；§3.1 迁移→T1/T6；§3.2 契约→T3b/T4/T5；§3.3 Go→T3/T4/T5；§3.4 Web→T7/T8；§5.2 e2e→T9。无未覆盖项。
- **占位扫描**：命令已核实为仓库真实 target——sqlc `make -C apps/control-plane generate-sqlc`、control-plane 契约生成 `corepack pnpm generate:control-plane`、契约验证 `corepack pnpm verify:contracts`、迁移校验 `make -C apps/control-plane migrate-validate`；无 TODO/TBD。
- **类型一致**：`TeamBaseline{Constitution,Skills,MCPServers}`、`GetTeamBaseline`、`UpdateTeamConstitution`、`Team.Constitution` 在 T3a/T3b/T5 间命名一致。
