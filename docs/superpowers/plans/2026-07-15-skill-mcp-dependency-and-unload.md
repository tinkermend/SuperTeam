# 技能↔MCP 依赖 + 会话级自动卸载 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 技能可声明依赖注册表中的 MCP 能力（装载派发时只校验不授权，缺失阻断）；家目录 MCP 配置改为会话作用域（会话开始注入+清单，会话结束回滚，残留兜底）。

**Architecture:** 依赖关系存独立表 `skill_mcp_dependencies`，capability 模块（Go）承载读写 API 与删除保护；派发闸门挂进 `run_service.prepareStartSessionDependencies` 既有 `validateRuntimeSkillDependencies` 同级位置；runtime-agent（Rust）把家目录 MCP 物化从 ProvisionInstance 前移到会话开始并配整文件快照 manifest 回滚。前端三处：技能详情依赖区块、MCP 页被依赖+删除保护、员工面板缺依赖警示。

**Tech Stack:** Go (chi + sqlc/pgx + atlas), Rust (anyhow + serde + toml), React (TanStack Query + vitest browser mode), OpenAPI 契约。

**Spec:** `docs/superpowers/specs/2026-07-15-skill-mcp-dependency-and-unload-design.md`

## Global Constraints

- 授权语义：**依赖只校验不授权**——MCP 生效集合仍只由员工/团队绑定产生，校验失败阻断派发，绝不因依赖扩集 payload。
- `mcp_servers` 与 `skills` 都是软删除（`deleted_at`），FK RESTRICT 只保护硬删；删除保护必须应用层实现（409）。
- 生产迁移唯一目录 `apps/control-plane/internal/storage/migrations/`；新迁移编号 **062**；变更后 `atlas migrate hash` 更新 `atlas.sum` 并 `make -C apps/control-plane migrate-validate`。
- 修改 `contracts/control-plane/openapi.yaml` 后必须跑 `corepack pnpm generate:control-plane`。
- 验证只用仓库脚本：`verify:foundation` / `verify:web` / `verify:runtime-agent` / `verify:db`；Web 测试 `corepack pnpm --filter @superteam/web test`，禁止 `npx vitest run`。
- Web 内部跳转用 TanStack Router `Link`/`navigate`；组件优先组合 `@/components/superteam` 现成 V3 组件（DESIGN.md:90/119）。
- Rust 无 tracing/log，best-effort 失败用 `eprintln!`；错误统一 `anyhow`。
- Go 测试断言风格 `t.Fatalf`（非 testify）；capability 模块测试用内存 fake（无真 DB）。
- 实现在新分支（建议 `feat/skill-mcp-dependency-unload`），不要在 `feat/scenario-template-p2a` 上做。

---

### Task 1: 迁移 062 `skill_mcp_dependencies`

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/062_skill_mcp_dependencies.sql`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`（由命令生成）

**Interfaces:**
- Produces: 表 `skill_mcp_dependencies(id, tenant_id, skill_id, mcp_server_id, note, created_at)`，唯一约束 `(tenant_id, skill_id, mcp_server_id)`，供 Task 2 的 sqlc 查询使用。

- [ ] **Step 1: 写迁移文件**

```sql
-- 062_skill_mcp_dependencies.sql
-- 技能对注册表 MCP 能力的依赖声明（只校验不授权，见 spec 2026-07-15）。
-- 两侧实体均为软删除，FK 仅保护硬删路径；应用层负责删除保护。

CREATE TABLE IF NOT EXISTS skill_mcp_dependencies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    skill_id UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    mcp_server_id UUID NOT NULL REFERENCES mcp_servers(id) ON DELETE RESTRICT,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_skill_mcp_dependencies_tenant_skill_server
    ON skill_mcp_dependencies(tenant_id, skill_id, mcp_server_id);

CREATE INDEX IF NOT EXISTS idx_skill_mcp_dependencies_tenant_server
    ON skill_mcp_dependencies(tenant_id, mcp_server_id);
```

（无 `updated_at` 列，不需要触发器。）

- [ ] **Step 2: 更新 atlas.sum 并校验**

```bash
cd apps/control-plane && atlas migrate hash --dir file://internal/storage/migrations
make -C apps/control-plane migrate-validate
```
Expected: validate 输出无错误退出码 0。（本地非 Docker dev 库可 `DEV_URL=... make -C apps/control-plane migrate-validate`。）

- [ ] **Step 3: 应用到 dev 库确认建表**

```bash
./scripts/dev-services.sh restart control-plane   # start/restart 自动跑 Atlas 迁移
```
Expected: control-plane 日志无迁移错误；`psql` 或后续 sqlc 测试可见表存在。

- [ ] **Step 4: Commit**

```bash
git add apps/control-plane/internal/storage/migrations/062_skill_mcp_dependencies.sql apps/control-plane/internal/storage/migrations/atlas.sum
git commit -m "feat(db): skill_mcp_dependencies 依赖表（迁移062）"
```

---

### Task 2: sqlc 查询 + capability 仓储方法

**Files:**
- Create: `apps/control-plane/internal/storage/queries/skill_mcp_dependencies.sql`
- Modify: `apps/control-plane/internal/capability/types.go`（追加领域类型）
- Modify: `apps/control-plane/internal/capability/service.go:19-44`（Repository 接口追加方法）
- Modify: `apps/control-plane/internal/capability/pg_repository.go`（实现）
- 生成物: `apps/control-plane/internal/storage/queries/skill_mcp_dependencies.sql.go`

**Interfaces:**
- Consumes: Task 1 的表。
- Produces（Task 3/5/6 依赖的精确签名）:
  - `type SkillMCPDependency struct { ID, TenantID, SkillID, MCPServerID uuid.UUID; Note string; CreatedAt time.Time; ServerKey, ServerName, RiskLevel, ServerStatus string; AuthStrategy MCPAuthStrategy }`
  - `type DependentSkill struct { SkillID uuid.UUID; Slug, Name string }`
  - `type SkillMCPDependencyInput struct { MCPServerID uuid.UUID; Note string }`
  - Repository 接口方法：
    - `ListSkillMCPDependencies(ctx context.Context, tenantID, skillID uuid.UUID) ([]SkillMCPDependency, error)`
    - `ReplaceSkillMCPDependencies(ctx context.Context, tenantID, skillID uuid.UUID, items []SkillMCPDependencyInput) ([]SkillMCPDependency, error)`
    - `ListDependentSkills(ctx context.Context, tenantID, serverID uuid.UUID) ([]DependentSkill, error)`
    - `ListSkillMCPDependenciesForSkills(ctx context.Context, tenantID uuid.UUID, skillIDs []uuid.UUID) ([]SkillMCPDependency, error)`

- [ ] **Step 1: 写 sqlc 查询文件**

`apps/control-plane/internal/storage/queries/skill_mcp_dependencies.sql`：

```sql
-- name: ListSkillMCPDependencies :many
SELECT d.id, d.tenant_id, d.skill_id, d.mcp_server_id, d.note, d.created_at,
       m.server_key, m.name AS server_name, m.auth_strategy, m.risk_level,
       m.status AS server_status
FROM skill_mcp_dependencies d
JOIN mcp_servers m ON m.id = d.mcp_server_id AND m.deleted_at IS NULL
WHERE d.tenant_id = sqlc.arg('tenant_id')::uuid
  AND d.skill_id = sqlc.arg('skill_id')::uuid
ORDER BY m.server_key ASC;

-- name: DeleteSkillMCPDependenciesForSkill :exec
DELETE FROM skill_mcp_dependencies
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND skill_id = sqlc.arg('skill_id')::uuid;

-- name: InsertSkillMCPDependency :exec
INSERT INTO skill_mcp_dependencies (tenant_id, skill_id, mcp_server_id, note)
VALUES (sqlc.arg('tenant_id')::uuid, sqlc.arg('skill_id')::uuid,
        sqlc.arg('mcp_server_id')::uuid, sqlc.arg('note')::text);

-- name: ListDependentSkillsForMCPServer :many
SELECT d.skill_id, s.slug, s.name
FROM skill_mcp_dependencies d
JOIN skills s ON s.id = d.skill_id AND s.deleted_at IS NULL
WHERE d.tenant_id = sqlc.arg('tenant_id')::uuid
  AND d.mcp_server_id = sqlc.arg('mcp_server_id')::uuid
ORDER BY s.slug ASC;

-- name: ListSkillMCPDependenciesForSkills :many
SELECT d.id, d.tenant_id, d.skill_id, d.mcp_server_id, d.note, d.created_at,
       m.server_key, m.name AS server_name, m.auth_strategy, m.risk_level,
       m.status AS server_status
FROM skill_mcp_dependencies d
JOIN mcp_servers m ON m.id = d.mcp_server_id AND m.deleted_at IS NULL
WHERE d.tenant_id = sqlc.arg('tenant_id')::uuid
  AND d.skill_id = ANY(sqlc.arg('skill_ids')::uuid[])
ORDER BY d.skill_id, m.server_key ASC;
```

- [ ] **Step 2: 生成 sqlc**

```bash
make -C apps/control-plane generate-sqlc
```
Expected: 生成 `skill_mcp_dependencies.sql.go`，`go build ./...` 通过。

- [ ] **Step 3: types.go 加领域类型**

在 `apps/control-plane/internal/capability/types.go` 的 `EffectiveMCPServer`（216 行）之后追加：

```go
// SkillMCPDependency declares that a skill requires an MCP registry definition
// at load time. Validation-only: it never grants the MCP to an employee.
type SkillMCPDependency struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	SkillID      uuid.UUID
	MCPServerID  uuid.UUID
	Note         string
	CreatedAt    time.Time
	ServerKey    string
	ServerName   string
	AuthStrategy MCPAuthStrategy
	RiskLevel    string
	ServerStatus string
}

// DependentSkill is a reverse lookup row: an active skill depending on an MCP definition.
type DependentSkill struct {
	SkillID uuid.UUID
	Slug    string
	Name    string
}

// SkillMCPDependencyInput is one desired dependency in a declarative replace.
type SkillMCPDependencyInput struct {
	MCPServerID uuid.UUID
	Note        string
}
```

- [ ] **Step 4: Repository 接口与 PgRepository 实现**

`service.go` 的 `Repository` 接口（19-44 行区）追加四个方法（签名见上方 Interfaces）。`pg_repository.go` 追加实现（模式对照现有 `ListMCPServerDefinitions`，`pg_repository.go:225-238`）：

```go
func (r *PgRepository) ListSkillMCPDependencies(ctx context.Context, tenantID, skillID uuid.UUID) ([]SkillMCPDependency, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	rows, err := r.q.ListSkillMCPDependencies(ctx, queries.ListSkillMCPDependenciesParams{TenantID: tenantID, SkillID: skillID})
	if err != nil {
		return nil, err
	}
	deps := make([]SkillMCPDependency, 0, len(rows))
	for _, row := range rows {
		deps = append(deps, skillMCPDependencyFromRow(row.ID, row.TenantID, row.SkillID, row.McpServerID, row.Note, row.CreatedAt, row.ServerKey, row.ServerName, row.AuthStrategy, row.RiskLevel, row.ServerStatus))
	}
	return deps, nil
}

// ReplaceSkillMCPDependencies is delete-then-insert without a transaction (PgRepository
// only wraps *queries.Queries). Partial failure leaves the skill with fewer declared
// dependencies, which fails closed: the dispatch gate blocks on missing bindings, never
// silently grants.
func (r *PgRepository) ReplaceSkillMCPDependencies(ctx context.Context, tenantID, skillID uuid.UUID, items []SkillMCPDependencyInput) ([]SkillMCPDependency, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	if err := r.q.DeleteSkillMCPDependenciesForSkill(ctx, queries.DeleteSkillMCPDependenciesForSkillParams{TenantID: tenantID, SkillID: skillID}); err != nil {
		return nil, err
	}
	for _, item := range items {
		if err := r.q.InsertSkillMCPDependency(ctx, queries.InsertSkillMCPDependencyParams{
			TenantID: tenantID, SkillID: skillID, McpServerID: item.MCPServerID, Note: item.Note,
		}); err != nil {
			return nil, err
		}
	}
	return r.ListSkillMCPDependencies(ctx, tenantID, skillID)
}

func (r *PgRepository) ListDependentSkills(ctx context.Context, tenantID, serverID uuid.UUID) ([]DependentSkill, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	rows, err := r.q.ListDependentSkillsForMCPServer(ctx, queries.ListDependentSkillsForMCPServerParams{TenantID: tenantID, McpServerID: serverID})
	if err != nil {
		return nil, err
	}
	out := make([]DependentSkill, 0, len(rows))
	for _, row := range rows {
		out = append(out, DependentSkill{SkillID: row.SkillID, Slug: row.Slug, Name: row.Name})
	}
	return out, nil
}

func (r *PgRepository) ListSkillMCPDependenciesForSkills(ctx context.Context, tenantID uuid.UUID, skillIDs []uuid.UUID) ([]SkillMCPDependency, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	if len(skillIDs) == 0 {
		return nil, nil
	}
	rows, err := r.q.ListSkillMCPDependenciesForSkills(ctx, queries.ListSkillMCPDependenciesForSkillsParams{TenantID: tenantID, SkillIds: skillIDs})
	if err != nil {
		return nil, err
	}
	deps := make([]SkillMCPDependency, 0, len(rows))
	for _, row := range rows {
		deps = append(deps, skillMCPDependencyFromRow(row.ID, row.TenantID, row.SkillID, row.McpServerID, row.Note, row.CreatedAt, row.ServerKey, row.ServerName, row.AuthStrategy, row.RiskLevel, row.ServerStatus))
	}
	return deps, nil
}

func skillMCPDependencyFromRow(id, tenantID, skillID, serverID uuid.UUID, note string, createdAt time.Time, serverKey, serverName, authStrategy, riskLevel, serverStatus string) SkillMCPDependency {
	return SkillMCPDependency{
		ID: id, TenantID: tenantID, SkillID: skillID, MCPServerID: serverID,
		Note: note, CreatedAt: createdAt, ServerKey: serverKey, ServerName: serverName,
		AuthStrategy: MCPAuthStrategy(authStrategy), RiskLevel: riskLevel, ServerStatus: serverStatus,
	}
}
```

注意：sqlc 生成的行结构字段名以生成物为准（如 `McpServerID`/`SkillIds` 的大小写），编译报错时对照 `skill_mcp_dependencies.sql.go` 修正。

- [ ] **Step 5: 编译确认 + Commit**

```bash
cd apps/control-plane && go build ./... && go vet ./internal/capability/
git add internal/storage/queries/skill_mcp_dependencies.sql internal/storage/queries/skill_mcp_dependencies.sql.go internal/capability/
git commit -m "feat(capability): skill_mcp_dependencies sqlc 查询与仓储方法"
```

---

### Task 3: capability service 依赖读写 + MCP 删除保护

**Files:**
- Modify: `apps/control-plane/internal/capability/types.go`（请求类型 + `ErrConflict`）
- Modify: `apps/control-plane/internal/capability/service.go`
- Modify: `apps/control-plane/internal/capability/service_test.go`
- Modify: `apps/control-plane/internal/skill/service.go:201-225`（DeleteSkill 清理依赖行）
- Modify: `apps/control-plane/internal/skill/pg_repository.go`、`apps/control-plane/internal/skill/service.go:20-33`（Repository 接口）

**Interfaces:**
- Consumes: Task 2 的仓储方法。
- Produces（Task 4/5 依赖）:
  - `func (s *Service) ListSkillMCPDependencies(ctx context.Context, req ListSkillMCPDependenciesRequest) ([]SkillMCPDependency, error)`
  - `func (s *Service) ReplaceSkillMCPDependencies(ctx context.Context, req ReplaceSkillMCPDependenciesRequest) ([]SkillMCPDependency, error)`
  - `func (s *Service) ListDependentSkills(ctx context.Context, req ListDependentSkillsRequest) ([]DependentSkill, error)`
  - `func (s *Service) ListSkillMCPDependenciesForRuntime(ctx context.Context, tenantID uuid.UUID, skillIDs []uuid.UUID) ([]SkillMCPDependency, error)`（供 runtime adapter，跳过 user 校验，对照 `ListEffectiveMCPConfigForRuntime` service.go:424 模式）
  - `var ErrConflict = errors.New("conflict")`（handler 映射 409）
  - `DeleteMCPServerDefinition` 行为变化：存在活跃依赖技能时返回 `ErrConflict`

- [ ] **Step 1: 写失败测试（service_test.go 追加，沿用内存 fake repo 模式）**

在 `service_test.go` 的 fake `repo` 上补充字段与方法实现（fake 需实现 Task 2 新增的四个 Repository 方法；用 map/slice 内存实现），然后：

```go
func TestServiceReplaceSkillMCPDependenciesValidatesServerExists(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, nil)
	tenantID, userID, skillID := uuid.New(), uuid.New(), uuid.New()
	_, err := svc.ReplaceSkillMCPDependencies(context.Background(), ReplaceSkillMCPDependenciesRequest{
		TenantID: tenantID, UserID: userID, SkillID: skillID,
		Items: []SkillMCPDependencyInput{{MCPServerID: uuid.New()}},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for unknown mcp server, got %v", err)
	}
}

func TestServiceDeleteMCPServerDefinitionBlockedByDependentSkills(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, nil)
	tenantID, userID := uuid.New(), uuid.New()
	serverID := repo.seedDefinition(tenantID, "github-mcp")           // fake helper：预置定义
	repo.seedDependency(tenantID, uuid.New(), serverID)               // fake helper：预置依赖
	err := svc.DeleteMCPServerDefinition(context.Background(), DeleteMCPServerDefinitionRequest{
		TenantID: tenantID, UserID: userID, ServerID: serverID,
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict when skills depend on server, got %v", err)
	}
}
```

（fake 的 `seedDefinition`/`seedDependency` 为测试文件内 helper，返回/记录到 fake 内存 map；若既有 fake 结构名不同，以现有 `service_test.go` 的 fake 命名为准扩展。）

- [ ] **Step 2: 跑测试确认失败**

```bash
cd apps/control-plane && go test ./internal/capability/ -run 'SkillMCPDependencies|BlockedByDependentSkills' -v
```
Expected: FAIL（方法/类型不存在，编译错误即算失败信号）。

- [ ] **Step 3: 实现**

`types.go` 追加：

```go
var ErrConflict = errors.New("conflict")

type ListSkillMCPDependenciesRequest struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
	SkillID  uuid.UUID
}

type ReplaceSkillMCPDependenciesRequest struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
	SkillID  uuid.UUID
	Items    []SkillMCPDependencyInput
}

type ListDependentSkillsRequest struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
	ServerID uuid.UUID
}
```

`service.go` 追加（校验风格对照 `ListMCPServerDefinitions` service.go:257-268）：

```go
func (s *Service) ListSkillMCPDependencies(ctx context.Context, req ListSkillMCPDependenciesRequest) ([]SkillMCPDependency, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if req.TenantID == uuid.Nil || req.UserID == uuid.Nil || req.SkillID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id, user_id and skill_id are required", ErrInvalidInput)
	}
	return s.repository.ListSkillMCPDependencies(ctx, req.TenantID, req.SkillID)
}

func (s *Service) ReplaceSkillMCPDependencies(ctx context.Context, req ReplaceSkillMCPDependenciesRequest) ([]SkillMCPDependency, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if req.TenantID == uuid.Nil || req.UserID == uuid.Nil || req.SkillID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id, user_id and skill_id are required", ErrInvalidInput)
	}
	seen := map[uuid.UUID]struct{}{}
	for _, item := range req.Items {
		if item.MCPServerID == uuid.Nil {
			return nil, fmt.Errorf("%w: mcp_server_id is required", ErrInvalidInput)
		}
		if _, dup := seen[item.MCPServerID]; dup {
			return nil, fmt.Errorf("%w: duplicate mcp_server_id %s", ErrInvalidInput, item.MCPServerID)
		}
		seen[item.MCPServerID] = struct{}{}
		if _, err := s.repository.GetMCPServerDefinition(ctx, req.TenantID, item.MCPServerID); err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, fmt.Errorf("%w: mcp server %s not found", ErrInvalidInput, item.MCPServerID)
			}
			return nil, err
		}
	}
	return s.repository.ReplaceSkillMCPDependencies(ctx, req.TenantID, req.SkillID, req.Items)
}

func (s *Service) ListDependentSkills(ctx context.Context, req ListDependentSkillsRequest) ([]DependentSkill, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if req.TenantID == uuid.Nil || req.UserID == uuid.Nil || req.ServerID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id, user_id and server_id are required", ErrInvalidInput)
	}
	return s.repository.ListDependentSkills(ctx, req.TenantID, req.ServerID)
}

// ListSkillMCPDependenciesForRuntime skips user validation: it serves the run-service
// dispatch gate, mirroring ListEffectiveMCPConfigForRuntime.
func (s *Service) ListSkillMCPDependenciesForRuntime(ctx context.Context, tenantID uuid.UUID, skillIDs []uuid.UUID) ([]SkillMCPDependency, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	return s.repository.ListSkillMCPDependenciesForSkills(ctx, tenantID, skillIDs)
}
```

`DeleteMCPServerDefinition`（service.go:270-281）在委托 repository 前加删除保护：

```go
	dependents, err := s.repository.ListDependentSkills(ctx, req.TenantID, req.ServerID)
	if err != nil {
		return err
	}
	if len(dependents) > 0 {
		slugs := make([]string, 0, len(dependents))
		for _, d := range dependents {
			slugs = append(slugs, d.Slug)
		}
		return fmt.Errorf("%w: mcp server is required by skills: %s", ErrConflict, strings.Join(slugs, ", "))
	}
```

注意 `GetMCPServerDefinition` 仓储方法签名以现有 `Repository` 接口为准（若为 request 结构入参则相应调整调用）。

`skill/service.go` 的 `DeleteSkill`（201-225 行）在软删成功后清理依赖行：skill `Repository` 接口加 `DeleteSkillMCPDependencies(ctx context.Context, tenantID, skillID uuid.UUID) error`，`skill/pg_repository.go` 实现直接调 `r.q.DeleteSkillMCPDependenciesForSkill(...)`（同一个 `queries` 生成包），DeleteSkill 中在删除归档之后调用，失败仅包装返回。

- [ ] **Step 4: 跑测试确认通过**

```bash
cd apps/control-plane && go test ./internal/capability/ ./internal/skill/ -v
```
Expected: PASS（含既有测试不回归；skill fake repo 需补新接口方法空实现）。

- [ ] **Step 5: Commit**

```bash
git add apps/control-plane/internal/capability/ apps/control-plane/internal/skill/
git commit -m "feat(capability): 技能MCP依赖读写服务与删除保护(409)"
```

---

### Task 4: HTTP handler + 路由 + OpenAPI 契约

**Files:**
- Modify: `apps/control-plane/internal/capability/handler.go`（三个 handler + `HandlerService` 接口 16-37 行 + `writeHandlerError` 876 行加 `ErrConflict→409` + 请求/响应 DTO）
- Modify: `apps/control-plane/internal/capability/handler_test.go`
- Modify: `apps/control-plane/internal/api/server.go:414-437`（capability 路由组加三条路由）
- Modify: `contracts/control-plane/openapi.yaml`（三条路径 + 三个 schema）
- 生成物: `apps/control-plane/gen/control_plane.gen.go`

**Interfaces:**
- Consumes: Task 3 的 service 方法。
- Produces（Task 8 web client 依赖的 HTTP 契约）:
  - `GET /api/v1/skills/{skillId}/mcp-dependencies` → 200 `SkillMCPDependency[]`（authz `mcp_registry.read`）
  - `PUT /api/v1/skills/{skillId}/mcp-dependencies` body `{"items":[{"mcp_server_id":"<uuid>","note":""}]}` → 200 `SkillMCPDependency[]`（authz `mcp_registry.manage`）
  - `GET /api/v1/mcp-servers/{serverId}/dependent-skills` → 200 `DependentSkill[]`（authz `mcp_registry.read`）
  - `DELETE /api/v1/mcp-servers/{serverId}` 有依赖时 → 409
  - JSON 字段：SkillMCPDependency `{id, skill_id, mcp_server_id, note, server_key, server_name, auth_strategy, risk_level, server_status, created_at}`；DependentSkill `{skill_id, slug, name}`

- [ ] **Step 1: 写失败 handler 测试**

`handler_test.go` 追加（fake `handlerService` 结构补三个新方法与记录字段，模式对照现有 234-350 行）：

```go
func TestHandlerReplaceSkillMCPDependenciesUsesManageAction(t *testing.T) {
	service := &handlerService{}
	authorizer := &handlerAuthorizer{allowed: true}
	handler := NewHandler(service)
	handler.SetAuthorizer(authorizer)

	skillID := uuid.New()
	body := strings.NewReader(`{"items":[{"mcp_server_id":"` + uuid.New().String() + `","note":"api"}]}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/skills/"+skillID.String()+"/mcp-dependencies", body)
	req = requestWithConsoleIdentity(req, uuid.New(), uuid.New())
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("skillId", skillID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	resp := httptest.NewRecorder()
	handler.ReplaceSkillMCPDependencies(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.Code, resp.Body.String())
	}
	if authorizer.checks[0].Action != authz.ActionMCPRegistryManage {
		t.Fatalf("expected manage action, got %s", authorizer.checks[0].Action)
	}
}

func TestHandlerDeleteMCPServerDefinitionConflictMapsTo409(t *testing.T) {
	service := &handlerService{deleteDefinitionErr: fmt.Errorf("%w: mcp server is required by skills: a", ErrConflict)}
	authorizer := &handlerAuthorizer{allowed: true}
	handler := NewHandler(service)
	handler.SetAuthorizer(authorizer)

	serverID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/mcp-servers/"+serverID.String(), nil)
	req = requestWithConsoleIdentity(req, uuid.New(), uuid.New())
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("serverId", serverID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	resp := httptest.NewRecorder()
	handler.DeleteMCPServerDefinition(resp, req)
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.Code)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
cd apps/control-plane && go test ./internal/capability/ -run 'SkillMCPDependenciesUsesManage|ConflictMapsTo409' -v
```
Expected: FAIL / 编译错误。

- [ ] **Step 3: 实现 handler + 路由**

`handler.go`：`HandlerService` 接口追加三个方法签名；`writeHandlerError` 追加：

```go
	case errors.Is(err, ErrConflict):
		writeError(w, http.StatusConflict, err.Error())
```
（具体写错误的辅助函数名以该文件现状为准，对照 `ErrNotFound→404` 分支同样写法。）

三个 handler（对照 `ListMCPServerDefinitions` handler.go:298-314）：

```go
func (h *HTTPHandler) ListSkillMCPDependencies(w http.ResponseWriter, r *http.Request) {
	skillID, ok := uuidParam(w, r, "skillId", "skill id")
	if !ok {
		return
	}
	tenantID := middleware.GetTenantID(r.Context())
	tenantID, userID, ok := h.authorize(w, r, authz.ActionMCPRegistryRead, authz.ResourceRef{Type: authz.ResourceTenant, ID: tenantID.String()}, "skill mcp dependencies read", nil)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	deps, err := service.ListSkillMCPDependencies(r.Context(), ListSkillMCPDependenciesRequest{TenantID: tenantID, UserID: userID, SkillID: skillID})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, skillMCPDependencyResponses(deps))
}

func (h *HTTPHandler) ReplaceSkillMCPDependencies(w http.ResponseWriter, r *http.Request) {
	skillID, ok := uuidParam(w, r, "skillId", "skill id")
	if !ok {
		return
	}
	tenantID := middleware.GetTenantID(r.Context())
	tenantID, userID, ok := h.authorize(w, r, authz.ActionMCPRegistryManage, authz.ResourceRef{Type: authz.ResourceTenant, ID: tenantID.String()}, "skill mcp dependencies replace", nil)
	if !ok {
		return
	}
	var body replaceSkillMCPDependenciesRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeHandlerError(w, fmt.Errorf("%w: invalid json body", ErrInvalidInput))
		return
	}
	items := make([]SkillMCPDependencyInput, 0, len(body.Items))
	for _, item := range body.Items {
		serverID, err := uuid.Parse(item.MCPServerID)
		if err != nil {
			writeHandlerError(w, fmt.Errorf("%w: invalid mcp_server_id", ErrInvalidInput))
			return
		}
		items = append(items, SkillMCPDependencyInput{MCPServerID: serverID, Note: item.Note})
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	deps, err := service.ReplaceSkillMCPDependencies(r.Context(), ReplaceSkillMCPDependenciesRequest{TenantID: tenantID, UserID: userID, SkillID: skillID, Items: items})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, skillMCPDependencyResponses(deps))
}

func (h *HTTPHandler) ListDependentSkills(w http.ResponseWriter, r *http.Request) {
	serverID, ok := uuidParam(w, r, "serverId", "server id")
	if !ok {
		return
	}
	tenantID := middleware.GetTenantID(r.Context())
	tenantID, userID, ok := h.authorize(w, r, authz.ActionMCPRegistryRead, authz.ResourceRef{Type: authz.ResourceTenant, ID: tenantID.String()}, "mcp dependent skills read", nil)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	skills, err := service.ListDependentSkills(r.Context(), ListDependentSkillsRequest{TenantID: tenantID, UserID: userID, ServerID: serverID})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dependentSkillResponses(skills))
}
```

DTO（对照 `mcpDefinitionResponse` 616-633）：

```go
type replaceSkillMCPDependenciesRequestBody struct {
	Items []struct {
		MCPServerID string `json:"mcp_server_id"`
		Note        string `json:"note"`
	} `json:"items"`
}

type skillMCPDependencyResponse struct {
	ID           string `json:"id"`
	SkillID      string `json:"skill_id"`
	MCPServerID  string `json:"mcp_server_id"`
	Note         string `json:"note"`
	ServerKey    string `json:"server_key"`
	ServerName   string `json:"server_name"`
	AuthStrategy string `json:"auth_strategy"`
	RiskLevel    string `json:"risk_level"`
	ServerStatus string `json:"server_status"`
	CreatedAt    string `json:"created_at"`
}

type dependentSkillResponse struct {
	SkillID string `json:"skill_id"`
	Slug    string `json:"slug"`
	Name    string `json:"name"`
}

func skillMCPDependencyResponses(deps []SkillMCPDependency) []skillMCPDependencyResponse {
	out := make([]skillMCPDependencyResponse, 0, len(deps))
	for _, d := range deps {
		out = append(out, skillMCPDependencyResponse{
			ID: d.ID.String(), SkillID: d.SkillID.String(), MCPServerID: d.MCPServerID.String(),
			Note: d.Note, ServerKey: d.ServerKey, ServerName: d.ServerName,
			AuthStrategy: string(d.AuthStrategy), RiskLevel: d.RiskLevel, ServerStatus: d.ServerStatus,
			CreatedAt: formatTime(d.CreatedAt),
		})
	}
	return out
}

func dependentSkillResponses(skills []DependentSkill) []dependentSkillResponse {
	out := make([]dependentSkillResponse, 0, len(skills))
	for _, s := range skills {
		out = append(out, dependentSkillResponse{SkillID: s.SkillID.String(), Slug: s.Slug, Name: s.Name})
	}
	return out
}
```

`server.go` capability 路由组（430 行 `r.Delete("/mcp-servers/{serverId}", ...)` 之后）：

```go
			r.Get("/skills/{skillId}/mcp-dependencies", s.capabilityHandler.ListSkillMCPDependencies)
			r.Put("/skills/{skillId}/mcp-dependencies", s.capabilityHandler.ReplaceSkillMCPDependencies)
			r.Get("/mcp-servers/{serverId}/dependent-skills", s.capabilityHandler.ListDependentSkills)
```

- [ ] **Step 4: OpenAPI 契约 + 生成**

`contracts/control-plane/openapi.yaml` 在 `/api/v1/mcp-servers/{serverId}`（2259-2271）之后追加路径（错误响应统一 `$ref: "#/components/responses/Error"`，命名对照 `MCPServerDefinition` 惯例）：

```yaml
  /api/v1/skills/{skillId}/mcp-dependencies:
    get:
      operationId: listSkillMCPDependencies
      summary: List MCP dependencies declared by a skill
      tags: [capabilities]
      parameters:
        - name: skillId
          in: path
          required: true
          schema: { type: string, format: uuid }
      responses:
        "200":
          description: Skill MCP dependencies
          content:
            application/json:
              schema:
                type: array
                items: { $ref: "#/components/schemas/SkillMCPDependency" }
        default: { $ref: "#/components/responses/Error" }
    put:
      operationId: replaceSkillMCPDependencies
      summary: Declaratively replace the MCP dependencies of a skill
      tags: [capabilities]
      parameters:
        - name: skillId
          in: path
          required: true
          schema: { type: string, format: uuid }
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/ReplaceSkillMCPDependenciesRequest" }
      responses:
        "200":
          description: Replaced skill MCP dependencies
          content:
            application/json:
              schema:
                type: array
                items: { $ref: "#/components/schemas/SkillMCPDependency" }
        default: { $ref: "#/components/responses/Error" }
  /api/v1/mcp-servers/{serverId}/dependent-skills:
    get:
      operationId: listMCPServerDependentSkills
      summary: List active skills depending on an MCP server definition
      tags: [capabilities]
      parameters:
        - name: serverId
          in: path
          required: true
          schema: { type: string, format: uuid }
      responses:
        "200":
          description: Dependent skills
          content:
            application/json:
              schema:
                type: array
                items: { $ref: "#/components/schemas/DependentSkill" }
        default: { $ref: "#/components/responses/Error" }
```

`components/schemas`（`MCPServerDefinition` 9022 附近之后）：

```yaml
    SkillMCPDependency:
      type: object
      required: [id, skill_id, mcp_server_id, note, server_key, server_name, auth_strategy, risk_level, server_status, created_at]
      properties:
        id: { type: string, format: uuid }
        skill_id: { type: string, format: uuid }
        mcp_server_id: { type: string, format: uuid }
        note: { type: string }
        server_key: { type: string }
        server_name: { type: string }
        auth_strategy: { type: string }
        risk_level: { type: string }
        server_status: { type: string }
        created_at: { type: string, format: date-time }
    ReplaceSkillMCPDependenciesRequest:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items:
            type: object
            required: [mcp_server_id]
            properties:
              mcp_server_id: { type: string, format: uuid }
              note: { type: string }
    DependentSkill:
      type: object
      required: [skill_id, slug, name]
      properties:
        skill_id: { type: string, format: uuid }
        slug: { type: string }
        name: { type: string }
```

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
```
Expected: 生成无 diff 冲突、契约验证通过。

- [ ] **Step 5: 跑测试 + Commit**

```bash
cd apps/control-plane && go test ./internal/capability/ ./internal/api/ -v
git add apps/control-plane/internal/capability/ apps/control-plane/internal/api/server.go contracts/control-plane/openapi.yaml apps/control-plane/gen/
git commit -m "feat(api): 技能MCP依赖读写与反向查询端点(+409删除保护)"
```

---

### Task 5: run_service 派发闸门（缺依赖阻断）

**Files:**
- Modify: `apps/control-plane/internal/employee/run_service.go`（lister 接口 + setter + 校验）
- Modify: `apps/control-plane/internal/employee/types.go`（依赖记录类型）
- Modify: `apps/control-plane/internal/app/app.go`（adapter + 接线）
- Modify: `apps/control-plane/internal/employee/run_service_test.go`

**Interfaces:**
- Consumes: Task 3 的 `ListSkillMCPDependenciesForRuntime`。
- Produces:
  - `employee/types.go`: `type SkillMCPDependencyRecord struct { SkillID uuid.UUID; MCPServerID string; ServerKey string }`
  - `run_service.go`: `type SkillMCPDependencyLister interface { ListSkillMCPDependenciesForSkills(ctx context.Context, tenantID uuid.UUID, skillIDs []uuid.UUID) ([]SkillMCPDependencyRecord, error) }` + `func (s *DigitalEmployeeRunService) SetSkillMCPDependencyLister(l SkillMCPDependencyLister)`
  - 阻断错误：`fmt.Errorf("%w: skill_mcp_dependencies_not_satisfied: %s", ErrInvalidInput, ...)`（与既有 `skill_dependencies_not_satisfied` 闸门同风格，走同一失败呈现链路）

- [ ] **Step 1: 写失败测试**

`run_service_test.go` 追加（fake 结构对照既有 `TestRunServiceCreateRunDispatchesStartSession` 134 行的构造方式；fake repo/dispatcher 复用现有 helper）：

```go
type fakeSkillMCPDependencyLister struct {
	records []SkillMCPDependencyRecord
}

func (f *fakeSkillMCPDependencyLister) ListSkillMCPDependenciesForSkills(ctx context.Context, tenantID uuid.UUID, skillIDs []uuid.UUID) ([]SkillMCPDependencyRecord, error) {
	return f.records, nil
}

func TestRunServiceBlocksDispatchWhenSkillMCPDependencyMissing(t *testing.T) {
	// 构造：员工挂 1 个 runtime 技能，技能依赖 server-a；mcpLister 返回空（未绑定）。
	// 断言：CreateRun 返回错误含 skill_mcp_dependencies_not_satisfied 与 server_key，
	// 且 dispatcher.commands 为空（未派发）。
	// 具体 fake 组装照抄 TestRunServiceCreateRunDispatchesStartSession 的前置，
	// 额外 SetSkillLister 返回一条 SkillRuntimeRecord{ID: skillID, Slug: "deploy-helper"}，
	// SetSkillMCPDependencyLister(&fakeSkillMCPDependencyLister{records: []SkillMCPDependencyRecord{{SkillID: skillID, MCPServerID: "srv-1", ServerKey: "github-mcp"}}})
}

func TestRunServiceDispatchesWhenSkillMCPDependencySatisfied(t *testing.T) {
	// 同上，但 mcpLister 返回一条 RuntimeMCPServerPayload{ServerID: "srv-1", ServerKey: "github-mcp", ...}。
	// 断言派发成功 len(dispatcher.commands)==1。
}
```

（测试主体按注释写全，fake mcpLister 已有既有模式可抄——run_service 现有测试如何注入 mcpLister 以现文件为准。）

- [ ] **Step 2: 跑测试确认失败**

```bash
cd apps/control-plane && go test ./internal/employee/ -run 'SkillMCPDependency' -v
```
Expected: FAIL / 编译错误。

- [ ] **Step 3: 实现**

`employee/types.go` 追加：

```go
// SkillMCPDependencyRecord is the run-service projection of a skill's MCP dependency.
// MCPServerID is a string to compare directly against RuntimeMCPServerPayload.ServerID.
type SkillMCPDependencyRecord struct {
	SkillID     uuid.UUID
	MCPServerID string
	ServerKey   string
}
```

`run_service.go`：接口 + 字段 + setter（对照 `RuntimeMCPLister`/`SetMCPLister` 56-60/100 行），并在 `prepareStartSessionDependencies`（630-690 行）的 `validateRuntimeSkillDependencies` 调用之后追加：

```go
	if err := s.validateSkillMCPDependencies(ctx, tenantID, deps); err != nil {
		return deps, err
	}
```

```go
// validateSkillMCPDependencies enforces "dependency validates, never grants": every MCP
// dependency of a loaded skill must already be present in the env-satisfied effective
// MCP set (deps.runtimeMCP). Missing => dispatch is blocked with a structured reason.
func (s *DigitalEmployeeRunService) validateSkillMCPDependencies(ctx context.Context, tenantID uuid.UUID, deps startSessionDependencies) error {
	if s.skillMCPDependencyLister == nil || len(deps.runtimeSkills) == 0 {
		return nil
	}
	skillIDs := make([]uuid.UUID, 0, len(deps.runtimeSkills))
	slugByID := make(map[uuid.UUID]string, len(deps.runtimeSkills))
	for _, runtimeSkill := range deps.runtimeSkills {
		skillIDs = append(skillIDs, runtimeSkill.ID)
		slugByID[runtimeSkill.ID] = runtimeSkill.Slug
	}
	records, err := s.skillMCPDependencyLister.ListSkillMCPDependenciesForSkills(ctx, tenantID, skillIDs)
	if err != nil {
		return fmt.Errorf("list skill mcp dependencies: %w", err)
	}
	available := make(map[string]struct{}, len(deps.runtimeMCP))
	for _, server := range deps.runtimeMCP {
		available[server.ServerID] = struct{}{}
	}
	var messages []string
	for _, record := range records {
		if _, ok := available[record.MCPServerID]; ok {
			continue
		}
		messages = append(messages, fmt.Sprintf("技能 %s 依赖 MCP %s：未绑定或缺环境变量", slugByID[record.SkillID], record.ServerKey))
	}
	if len(messages) == 0 {
		return nil
	}
	return fmt.Errorf("%w: skill_mcp_dependencies_not_satisfied: %s", ErrInvalidInput, strings.Join(messages, "; "))
}
```

`app.go` adapter（对照 `runtimeMCPListerAdapter` 92-118 行）与接线（565 行 `SetMCPLister` 之后）：

```go
type skillMCPDependencyListerAdapter struct {
	capability *capability.Service
}

func (a skillMCPDependencyListerAdapter) ListSkillMCPDependenciesForSkills(ctx context.Context, tenantID uuid.UUID, skillIDs []uuid.UUID) ([]employee.SkillMCPDependencyRecord, error) {
	if a.capability == nil {
		return nil, nil
	}
	deps, err := a.capability.ListSkillMCPDependenciesForRuntime(ctx, tenantID, skillIDs)
	if err != nil {
		return nil, err
	}
	out := make([]employee.SkillMCPDependencyRecord, 0, len(deps))
	for _, dep := range deps {
		out = append(out, employee.SkillMCPDependencyRecord{
			SkillID:     dep.SkillID,
			MCPServerID: dep.MCPServerID.String(),
			ServerKey:   dep.ServerKey,
		})
	}
	return out, nil
}
```

```go
	runService.SetSkillMCPDependencyLister(skillMCPDependencyListerAdapter{capability: capabilityService})
```

- [ ] **Step 4: 跑测试确认通过 + Commit**

```bash
cd apps/control-plane && go test ./internal/employee/ ./internal/app/ -v
git add apps/control-plane/internal/employee/ apps/control-plane/internal/app/app.go
git commit -m "feat(run): 派发闸门校验技能MCP依赖, 缺失阻断(只校验不授权)"
```

---

### Task 6: 员工技能依赖状态端点（面板数据源）

**Files:**
- Modify: `apps/control-plane/internal/capability/types.go`、`service.go`、`handler.go`、`service_test.go`、`handler_test.go`
- Modify: `apps/control-plane/internal/api/server.go`（capability 路由组加一条）
- Modify: `apps/control-plane/internal/app/app.go`（skill lister 注入 capability service）
- Modify: `contracts/control-plane/openapi.yaml`

**Interfaces:**
- Consumes: Task 2 仓储、既有 `ListEffectiveMCPConfigForRuntime`（`EffectiveMCPServer.MissingEnvVars`）、skill 模块 `ListSkillsForRuntime`。
- Produces（Task 12 面板依赖）:
  - `GET /api/v1/digital-employees/{employeeId}/skill-mcp-dependency-status` → 200：
    ```json
    [{"skill_id":"...","skill_slug":"deploy-helper","dependencies":[
      {"mcp_server_id":"...","server_key":"github-mcp","server_name":"GitHub MCP",
       "status":"missing_binding","missing_env_vars":[]}]}]
    ```
    `status ∈ satisfied | missing_binding | blocked_missing_env`
  - capability 内新接口：`type EmployeeRuntimeSkillLister interface { ListEmployeeRuntimeSkillRefs(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]RuntimeSkillRef, error) }`；`type RuntimeSkillRef struct { ID uuid.UUID; Slug string }`；`func (s *Service) SetEmployeeRuntimeSkillLister(l EmployeeRuntimeSkillLister)`

- [ ] **Step 1: 写失败 service 测试**

```go
func TestServiceEvaluatesEmployeeSkillMCPDependencyStatus(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, nil)
	tenantID, userID, employeeID, skillID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	boundServer := repo.seedDefinition(tenantID, "bound-mcp")     // 已绑定且 env 满足
	envBlocked := repo.seedDefinition(tenantID, "env-mcp")       // 已绑定但缺 env（fake effective 返回 MissingEnvVars）
	unbound := repo.seedDefinition(tenantID, "unbound-mcp")      // 未绑定
	repo.seedDependency(tenantID, skillID, boundServer)
	repo.seedDependency(tenantID, skillID, envBlocked)
	repo.seedDependency(tenantID, skillID, unbound)
	repo.seedEffective(employeeID, boundServer, nil)                          // fake helper
	repo.seedEffective(employeeID, envBlocked, []string{"GH_TOKEN"})          // fake helper
	svc.SetEmployeeRuntimeSkillLister(fakeRuntimeSkillLister{refs: []RuntimeSkillRef{{ID: skillID, Slug: "deploy-helper"}}})

	statuses, err := svc.EvaluateEmployeeSkillMCPDependencies(context.Background(), EvaluateEmployeeSkillMCPDependenciesRequest{TenantID: tenantID, UserID: userID, DigitalEmployeeID: employeeID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 1 || len(statuses[0].Dependencies) != 3 {
		t.Fatalf("expected 1 skill with 3 dependencies, got %+v", statuses)
	}
	byKey := map[string]string{}
	for _, dep := range statuses[0].Dependencies {
		byKey[dep.ServerKey] = dep.Status
	}
	if byKey["bound-mcp"] != "satisfied" || byKey["env-mcp"] != "blocked_missing_env" || byKey["unbound-mcp"] != "missing_binding" {
		t.Fatalf("unexpected statuses: %v", byKey)
	}
}
```

（fake repo 需实现 effective 列表来源——`EvaluateEmployeeSkillMCPDependencies` 内部经 `ListEffectiveMCPConfigForRuntime` 走 repository 的 effective 查询方法，fake 对应方法返回 seed 数据；以现有 fake 对 effective 查询的实现方式为准扩展。）

- [ ] **Step 2: 跑测试确认失败**

```bash
cd apps/control-plane && go test ./internal/capability/ -run 'EvaluatesEmployeeSkillMCP' -v
```
Expected: FAIL / 编译错误。

- [ ] **Step 3: 实现**

`types.go`：

```go
type RuntimeSkillRef struct {
	ID   uuid.UUID
	Slug string
}

type EvaluateEmployeeSkillMCPDependenciesRequest struct {
	TenantID          uuid.UUID
	UserID            uuid.UUID
	DigitalEmployeeID uuid.UUID
}

type EmployeeSkillMCPDependencyStatus struct {
	SkillID      uuid.UUID
	SkillSlug    string
	Dependencies []EmployeeSkillMCPDependencyItem
}

type EmployeeSkillMCPDependencyItem struct {
	MCPServerID    uuid.UUID
	ServerKey      string
	ServerName     string
	Status         string // satisfied | missing_binding | blocked_missing_env
	MissingEnvVars []string
}
```

`service.go`：

```go
type EmployeeRuntimeSkillLister interface {
	ListEmployeeRuntimeSkillRefs(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]RuntimeSkillRef, error)
}

func (s *Service) SetEmployeeRuntimeSkillLister(l EmployeeRuntimeSkillLister) {
	s.employeeRuntimeSkillLister = l
}

func (s *Service) EvaluateEmployeeSkillMCPDependencies(ctx context.Context, req EvaluateEmployeeSkillMCPDependenciesRequest) ([]EmployeeSkillMCPDependencyStatus, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if req.TenantID == uuid.Nil || req.UserID == uuid.Nil || req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id, user_id and employee_id are required", ErrInvalidInput)
	}
	if s.employeeRuntimeSkillLister == nil {
		return nil, nil
	}
	refs, err := s.employeeRuntimeSkillLister.ListEmployeeRuntimeSkillRefs(ctx, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		return nil, nil
	}
	skillIDs := make([]uuid.UUID, 0, len(refs))
	for _, ref := range refs {
		skillIDs = append(skillIDs, ref.ID)
	}
	deps, err := s.repository.ListSkillMCPDependenciesForSkills(ctx, req.TenantID, skillIDs)
	if err != nil {
		return nil, err
	}
	effective, err := s.ListEffectiveMCPConfigForRuntime(ctx, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return nil, err
	}
	missingEnvByServer := map[uuid.UUID][]string{}
	boundServers := map[uuid.UUID]struct{}{}
	for _, server := range effective {
		boundServers[server.ServerID] = struct{}{}
		if len(server.MissingEnvVars) > 0 {
			missingEnvByServer[server.ServerID] = server.MissingEnvVars
		}
	}
	bySkill := map[uuid.UUID]*EmployeeSkillMCPDependencyStatus{}
	ordered := make([]*EmployeeSkillMCPDependencyStatus, 0, len(refs))
	for _, ref := range refs {
		status := &EmployeeSkillMCPDependencyStatus{SkillID: ref.ID, SkillSlug: ref.Slug}
		bySkill[ref.ID] = status
		ordered = append(ordered, status)
	}
	for _, dep := range deps {
		item := EmployeeSkillMCPDependencyItem{MCPServerID: dep.MCPServerID, ServerKey: dep.ServerKey, ServerName: dep.ServerName, Status: "satisfied", MissingEnvVars: []string{}}
		if _, bound := boundServers[dep.MCPServerID]; !bound {
			item.Status = "missing_binding"
		} else if missing, blocked := missingEnvByServer[dep.MCPServerID]; blocked {
			item.Status = "blocked_missing_env"
			item.MissingEnvVars = missing
		}
		if status, ok := bySkill[dep.SkillID]; ok {
			status.Dependencies = append(status.Dependencies, item)
		}
	}
	out := make([]EmployeeSkillMCPDependencyStatus, 0, len(ordered))
	for _, status := range ordered {
		out = append(out, *status)
	}
	return out, nil
}
```

注意：`ListEffectiveMCPConfigForRuntime` 会排除缺 env 的绑定还是标注 `MissingEnvVars` 以现实现（service.go:424-438 与其 repository 查询）为准——若它已排除缺 env 项，则此处改调 console 版 effective 查询（`ListEffectiveMCPBindingsV2` 路径）以拿到 `MissingEnvVars` 明细；测试断言不变。

handler（authz `mcp_registry.read`，resource tenant）+ 路由：

```go
			r.Get("/digital-employees/{employeeId}/skill-mcp-dependency-status", s.capabilityHandler.ListEmployeeSkillMCPDependencyStatus)
```

响应 DTO JSON：`{skill_id, skill_slug, dependencies: [{mcp_server_id, server_key, server_name, status, missing_env_vars}]}`。openapi.yaml 加对应路径与 `EmployeeSkillMCPDependencyStatus` schema（写法对照 Task 4 的块），然后 `corepack pnpm generate:control-plane`。

`app.go` 接线（`SetMCPLister` 565 行附近）：

```go
type employeeRuntimeSkillListerAdapter struct {
	skills *skill.Service
}

func (a employeeRuntimeSkillListerAdapter) ListEmployeeRuntimeSkillRefs(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]capability.RuntimeSkillRef, error) {
	if a.skills == nil {
		return nil, nil
	}
	records, err := a.skills.ListSkillsForRuntime(ctx, tenantID, digitalEmployeeID)
	if err != nil {
		return nil, err
	}
	refs := make([]capability.RuntimeSkillRef, 0, len(records))
	for _, record := range records {
		refs = append(refs, capability.RuntimeSkillRef{ID: record.ID, Slug: record.Slug})
	}
	return refs, nil
}
```

```go
	capabilityService.SetEmployeeRuntimeSkillLister(employeeRuntimeSkillListerAdapter{skills: skillService})
```

- [ ] **Step 4: 跑测试确认通过 + Commit**

```bash
cd apps/control-plane && go test ./internal/capability/ ./internal/app/ -v && cd ../.. && corepack pnpm verify:contracts
git add apps/control-plane/ contracts/control-plane/openapi.yaml
git commit -m "feat(capability): 员工技能MCP依赖状态评估端点"
```

---

### Task 7: Rust — 会话注入清单与回滚（mcp_config.rs）

**Files:**
- Modify: `apps/runtime-agent/src/mcp_config.rs`

**Interfaces:**
- Consumes: 既有 `materialize_mcp_config`（mcp_config.rs:17）、`atomic_write`（workspace_files.rs:158）、`RuntimeMCPServerPayload`（commands/payload.rs:65-87）。
- Produces（Task 8 executor 依赖）:
  - `pub fn manifest_path(agent_home_dir: &Path) -> PathBuf`（= `agent_home/.superteam/mcp-session-manifest.json`）
  - `pub fn inject_session_mcp_config(agent_home_dir: &Path, provider_type: &str, servers: &[RuntimeMCPServerPayload]) -> Result<Vec<PathBuf>>`——先回滚残留，再快照+写 manifest+物化
  - `pub fn rollback_session_mcp_config(agent_home_dir: &Path) -> Result<()>`——按 manifest 还原/删除并移除 manifest；无 manifest 时是 no-op

- [ ] **Step 1: 写失败测试（`#[cfg(test)] mod tests` 内追加，沿用 tempdir + `github_server()` helper）**

```rust
#[test]
fn inject_records_manifest_and_rollback_restores_prior_codex_config() {
    let dir = tempfile::tempdir().unwrap();
    let codex_dir = dir.path().join(".codex");
    std::fs::create_dir_all(&codex_dir).unwrap();
    std::fs::write(codex_dir.join("config.toml"), "theme = \"dark\"\n").unwrap();

    let written = inject_session_mcp_config(dir.path(), "codex", &[github_server()]).unwrap();
    assert_eq!(written.len(), 1);
    let injected = std::fs::read_to_string(&written[0]).unwrap();
    assert!(injected.contains("[mcp_servers.github]"));
    assert!(injected.contains("theme"));
    assert!(manifest_path(dir.path()).exists());

    rollback_session_mcp_config(dir.path()).unwrap();
    let restored = std::fs::read_to_string(codex_dir.join("config.toml")).unwrap();
    assert_eq!(restored, "theme = \"dark\"\n");
    assert!(!manifest_path(dir.path()).exists());
}

#[test]
fn rollback_deletes_file_created_by_injection() {
    let dir = tempfile::tempdir().unwrap();
    inject_session_mcp_config(dir.path(), "claude-code", &[github_server()]).unwrap();
    assert!(dir.path().join(".mcp.json").exists());
    rollback_session_mcp_config(dir.path()).unwrap();
    assert!(!dir.path().join(".mcp.json").exists());
}

#[test]
fn inject_rolls_back_residual_manifest_before_injecting() {
    let dir = tempfile::tempdir().unwrap();
    // 第一次注入后不回滚，模拟异常退出残留
    inject_session_mcp_config(dir.path(), "claude-code", &[github_server()]).unwrap();
    // 第二次注入应先回滚残留（.mcp.json 删除）再重新注入
    let written = inject_session_mcp_config(dir.path(), "claude-code", &[github_server()]).unwrap();
    assert_eq!(written.len(), 1);
    let manifest_raw = std::fs::read_to_string(manifest_path(dir.path())).unwrap();
    // 残留回滚后重拍快照：本次快照必须记录"文件不存在"，而不是把上次注入内容当原值
    assert!(manifest_raw.contains("\"existed\": false") || manifest_raw.contains("\"existed\":false"));
}

#[test]
fn rollback_without_manifest_is_noop() {
    let dir = tempfile::tempdir().unwrap();
    rollback_session_mcp_config(dir.path()).unwrap();
}

#[test]
fn inject_with_empty_servers_clears_residual_and_writes_nothing() {
    let dir = tempfile::tempdir().unwrap();
    inject_session_mcp_config(dir.path(), "opencode", &[github_server()]).unwrap();
    let written = inject_session_mcp_config(dir.path(), "opencode", &[]).unwrap();
    assert!(written.is_empty());
    assert!(!manifest_path(dir.path()).exists());
    assert!(!dir.path().join("opencode.json").exists());
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml mcp_config
```
Expected: FAIL / 编译错误（函数不存在）。

- [ ] **Step 3: 实现**

在 `mcp_config.rs` 追加（`serde` derive 需 `use serde::{Deserialize, Serialize};`；目标路径推导复用 `materialize_mcp_config` 内的 join 逻辑，抽成私有 helper）：

```rust
const SESSION_MANIFEST_DIR: &str = ".superteam";
const SESSION_MANIFEST_FILE: &str = "mcp-session-manifest.json";

#[derive(Debug, Serialize, Deserialize)]
struct McpSessionManifest {
    entries: Vec<McpManifestEntry>,
}

#[derive(Debug, Serialize, Deserialize)]
struct McpManifestEntry {
    path: PathBuf,
    existed: bool,
    #[serde(default)]
    previous_content: Option<String>,
}

pub fn manifest_path(agent_home_dir: &Path) -> PathBuf {
    agent_home_dir.join(SESSION_MANIFEST_DIR).join(SESSION_MANIFEST_FILE)
}

/// Session-scoped home-dir MCP injection: roll back any residual manifest first
/// (crash fallback), snapshot the target file, persist the manifest, then materialize.
/// The manifest is written BEFORE the config: if materialization fails midway the
/// next session still restores the pre-session state.
pub fn inject_session_mcp_config(
    agent_home_dir: &Path,
    provider_type: &str,
    servers: &[RuntimeMCPServerPayload],
) -> Result<Vec<PathBuf>> {
    rollback_session_mcp_config(agent_home_dir)
        .with_context(|| "rollback residual mcp session manifest before injecting")?;
    if servers.is_empty() {
        return Ok(Vec::new());
    }
    let target = home_mcp_config_target(agent_home_dir, provider_type)?;
    let existed = target.exists();
    let previous_content = if existed {
        Some(fs::read_to_string(&target).with_context(|| {
            format!("snapshot existing mcp config {}", target.display())
        })?)
    } else {
        None
    };
    let manifest = McpSessionManifest {
        entries: vec![McpManifestEntry { path: target.clone(), existed, previous_content }],
    };
    let mpath = manifest_path(agent_home_dir);
    if let Some(parent) = mpath.parent() {
        fs::create_dir_all(parent)
            .with_context(|| format!("create manifest dir {}", parent.display()))?;
    }
    let manifest_bytes = serde_json::to_vec_pretty(&manifest)?;
    crate::workspace_files::atomic_write(&mpath, &manifest_bytes)?;
    materialize_mcp_config(agent_home_dir, provider_type, servers)
}

/// Restores every file recorded in the session manifest (previous content for files
/// that existed, deletion for files the injection created), then removes the manifest.
/// No manifest means nothing was injected: no-op.
pub fn rollback_session_mcp_config(agent_home_dir: &Path) -> Result<()> {
    let mpath = manifest_path(agent_home_dir);
    if !mpath.exists() {
        return Ok(());
    }
    let raw = fs::read_to_string(&mpath)
        .with_context(|| format!("read mcp session manifest {}", mpath.display()))?;
    let manifest: McpSessionManifest = serde_json::from_str(&raw)
        .with_context(|| format!("parse mcp session manifest {}", mpath.display()))?;
    for entry in &manifest.entries {
        if entry.existed {
            if let Some(content) = &entry.previous_content {
                if let Some(parent) = entry.path.parent() {
                    fs::create_dir_all(parent)?;
                }
                crate::workspace_files::atomic_write(&entry.path, content.as_bytes())?;
            }
        } else if entry.path.exists() {
            fs::remove_file(&entry.path)
                .with_context(|| format!("remove injected mcp config {}", entry.path.display()))?;
        }
    }
    fs::remove_file(&mpath)
        .with_context(|| format!("remove mcp session manifest {}", mpath.display()))?;
    Ok(())
}
```

`home_mcp_config_target`：把 `materialize_mcp_config`（17-60 行）内的 target 推导（含 `starts_with(agent_home_dir)` 越界防御、provider 未知报错）提取为：

```rust
fn home_mcp_config_target(agent_home_dir: &Path, provider_type: &str) -> Result<PathBuf> { ... }
```

`materialize_mcp_config` 改为调用该 helper（行为不变，既有 7 个测试必须保持通过）。注意 `atomic_write` 要求父目录存在（workspace_files.rs:162-167 会 bail）——`materialize_mcp_config` 现状已处理 `.codex/` 目录创建，helper 提取时保留。

- [ ] **Step 4: 跑测试确认通过**

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml mcp_config
```
Expected: 新增 5 个测试 + 既有 7 个测试全部 PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/runtime-agent/src/mcp_config.rs
git commit -m "feat(runtime): 会话级MCP注入清单与整文件快照回滚"
```

---

### Task 8: Rust — executor 接线（会话注入/收尾回滚/移除 provision 写入）

**Files:**
- Modify: `apps/runtime-agent/src/commands/executor.rs`

**Interfaces:**
- Consumes: Task 7 的 `inject_session_mcp_config` / `rollback_session_mcp_config`。
- Produces: 行为变化——家目录 MCP 配置只在会话期间存在；ProvisionInstance 不再写 MCP 配置（payload 中 `mcp_servers` 字段保留但 runtime 忽略）。

- [ ] **Step 1: 会话开始注入**

`ensure_command_instance`（executor.rs:985-1049）在 agent_home 解析（998 行 `PathBuf::from(agent_home_dir_text)`）之后、`materialize_task_mcp_config` 调用（1027 行）之前插入：

```rust
        if let Err(error) = crate::mcp_config::inject_session_mcp_config(
            &agent_home_dir,
            &payload.provider_type,
            &payload.mcp_servers,
        ) {
            return Err(self.recorded_error(command_id, error));
        }
```

（`ensure_command_instance` 对 StartSession/Resume/SendInput 都会执行；重复注入是幂等的——`inject_session_mcp_config` 先回滚残留再以相同 payload 重拍快照重写，最终内容一致。）

- [ ] **Step 2: 会话收尾回滚（三处）**

(a) 主收尾点 `drain_provider_events`（executor.rs:2796-2909）：在 2872-2877 行 `finalize_raw_log(...)` 附近（heartbeat stop 之后、成功/失败写回之前的共同路径）加：

```rust
    if let Some(agent_home) = &spec.agent_home_dir {
        if let Err(error) = crate::mcp_config::rollback_session_mcp_config(agent_home) {
            eprintln!(
                "mcp session rollback failed for run {} at {}: {error:#}",
                run_id,
                agent_home.display()
            );
        }
    }
```

（`spec: RunSpec` 已含 `agent_home_dir: Option<PathBuf>`，见构造 298-329 行；回滚失败仅 `eprintln!` best-effort，不传播——残留由下次会话兜底。）

(b) `handle_input_command` 内 `runs.finish_failed` 的早期失败分支（367、382、419 行附近）：在每个分支 `record_run_finished` 之后加同样的回滚调用（此作用域有 agent_home 的来源变量，从 `CommandWorkspace.agent_home_dir` 或构造 RunSpec 用的同一值取；若某分支发生在 agent_home 解析之前则跳过——注入也尚未发生）。

(c) `handle_stop_command`（462-550 行）：不加回滚——`cancel_run` 会促使 provider 事件流终结，`drain_provider_events` 的 (a) 收尾点统一执行回滚；若 drain 已不在运行（进程重启后 stop），残留由下次会话注入前兜底。此决策写注释在 (a) 处。

- [ ] **Step 3: 移除 ProvisionInstance 的 MCP 写入**

删除 `handle_provision_instance` 内 644-656 行的 `materialize_mcp_config` 调用块，替换为注释：

```rust
        // MCP home config is session-scoped since the skill-mcp-dependency spec
        // (2026-07-15): it is injected by ensure_command_instance at session start and
        // rolled back at session end. Provisioning no longer materializes it; the
        // payload field is accepted and ignored for backward compatibility.
```

- [ ] **Step 4: 编译 + 全量 Rust 测试**

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml
```
Expected: PASS（若 executor 相关既有测试断言 provision 写 MCP 文件，改断言为不写）。

- [ ] **Step 5: Commit**

```bash
git add apps/runtime-agent/src/commands/executor.rs
git commit -m "feat(runtime): 家目录MCP配置会话作用域化——开始注入/收尾回滚/provision不再写入"
```

---

### Task 9: Web API client 函数

**Files:**
- Modify: `apps/web/src/lib/api/skills.ts`
- Modify: `apps/web/src/lib/api/capabilities.ts`

**Interfaces:**
- Consumes: Task 4/6 的 HTTP 契约。
- Produces（Task 10-12 依赖的精确签名）:
  - `skills.ts`: `interface SkillMcpDependency { id: string; skill_id: string; mcp_server_id: string; note: string; server_key: string; server_name: string; auth_strategy: string; risk_level: string; server_status: string; created_at: string }`
  - `skills.ts`: `listSkillMcpDependencies(options: ApiClientOptions, skillId: string): Promise<SkillMcpDependency[]>`
  - `skills.ts`: `replaceSkillMcpDependencies(options: ApiClientOptions, skillId: string, input: { items: Array<{ mcp_server_id: string; note?: string }> }): Promise<SkillMcpDependency[]>`
  - `capabilities.ts`: `interface DependentSkill { skill_id: string; slug: string; name: string }` + `listMcpServerDependentSkills(options, serverId): Promise<DependentSkill[]>`
  - `capabilities.ts`: `interface EmployeeSkillMcpDependencyStatus { skill_id: string; skill_slug: string; dependencies: Array<{ mcp_server_id: string; server_key: string; server_name: string; status: "satisfied" | "missing_binding" | "blocked_missing_env"; missing_env_vars: string[] }> }` + `listEmployeeSkillMcpDependencyStatus(options, employeeId): Promise<EmployeeSkillMcpDependencyStatus[]>`

- [ ] **Step 1: 实现（capabilities.ts 用 getJson 风格；skills.ts 沿用该文件手写 fetcher 风格或引入 client 便捷函数，与文件现状一致即可）**

`capabilities.ts` 追加：

```ts
export interface DependentSkill {
  skill_id: string;
  slug: string;
  name: string;
}

export function listMcpServerDependentSkills(
  options: ApiClientOptions,
  serverId: string,
): Promise<DependentSkill[]> {
  const encodedServerId = encodePathSegment(serverId);
  return getJson<DependentSkill[]>(
    options,
    `/api/v1/mcp-servers/${encodedServerId}/dependent-skills`,
    "mcp server dependent skills",
  );
}

export interface EmployeeSkillMcpDependencyItem {
  mcp_server_id: string;
  server_key: string;
  server_name: string;
  status: "satisfied" | "missing_binding" | "blocked_missing_env";
  missing_env_vars: string[];
}

export interface EmployeeSkillMcpDependencyStatus {
  skill_id: string;
  skill_slug: string;
  dependencies: EmployeeSkillMcpDependencyItem[];
}

export function listEmployeeSkillMcpDependencyStatus(
  options: ApiClientOptions,
  employeeId: string,
): Promise<EmployeeSkillMcpDependencyStatus[]> {
  const encodedEmployeeId = encodePathSegment(employeeId);
  return getJson<EmployeeSkillMcpDependencyStatus[]>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/skill-mcp-dependency-status`,
    "employee skill mcp dependency status",
  );
}
```

`skills.ts` 追加（`putJson`/`getJson` 从 `./client` 引入）：

```ts
export interface SkillMcpDependency {
  id: string;
  skill_id: string;
  mcp_server_id: string;
  note: string;
  server_key: string;
  server_name: string;
  auth_strategy: string;
  risk_level: string;
  server_status: string;
  created_at: string;
}

export function listSkillMcpDependencies(
  options: ApiClientOptions,
  skillId: string,
): Promise<SkillMcpDependency[]> {
  return getJson<SkillMcpDependency[]>(
    options,
    `/api/v1/skills/${encodeURIComponent(skillId)}/mcp-dependencies`,
    "skill mcp dependencies",
  );
}

export function replaceSkillMcpDependencies(
  options: ApiClientOptions,
  skillId: string,
  input: { items: Array<{ mcp_server_id: string; note?: string }> },
): Promise<SkillMcpDependency[]> {
  return putJson<SkillMcpDependency[]>(
    options,
    `/api/v1/skills/${encodeURIComponent(skillId)}/mcp-dependencies`,
    input,
    "replace skill mcp dependencies",
  );
}
```

- [ ] **Step 2: typecheck + Commit**

```bash
corepack pnpm --filter @superteam/web typecheck
git add apps/web/src/lib/api/skills.ts apps/web/src/lib/api/capabilities.ts
git commit -m "feat(web): 技能MCP依赖与被依赖查询 API client"
```

---

### Task 10: 技能详情页"依赖 MCP"区块

**Files:**
- Modify: `apps/web/src/features/skills/detail.tsx`
- Modify: `apps/web/src/features/skills/detail.test.tsx`

**Interfaces:**
- Consumes: Task 9 的 `listSkillMcpDependencies` / `replaceSkillMcpDependencies`，`listMcpServerDefinitions`（capabilities.ts:265）。
- Produces: 详情页 `MasterDetailLayout` 内新增 `DetailSection`（title="依赖 MCP"），依赖行含移除按钮，`Select` 从注册表添加。

- [ ] **Step 1: 写失败测试（fetcher 注入范式，对照 detail.test.tsx:74-94）**

```tsx
const dependencyFixture = {
  id: "dep-1",
  skill_id: "skill-1",
  mcp_server_id: "srv-1",
  note: "",
  server_key: "github-mcp",
  server_name: "GitHub MCP",
  auth_strategy: "bearer_env",
  risk_level: "low",
  server_status: "active",
  created_at: "2026-07-15T00:00:00Z",
};

test("renders skill mcp dependencies section", async () => {
  const fetcher = createSkillFetcher({
    // createSkillFetcher 需扩展路由: /mcp-dependencies 返回 [dependencyFixture],
    // /api/v1/mcp-servers 返回注册表定义列表(供添加选择器)
  });
  const screen = renderSkillDetail(fetcher);
  await expect.element(screen.getByText("依赖 MCP")).toBeVisible();
  await expect.element(screen.getByText("github-mcp")).toBeVisible();
});

test("replaces dependencies when removing one", async () => {
  // render 后点击依赖行"移除"按钮, vi.waitFor 断言 fetcher 收到
  // PUT /api/v1/skills/skill-1/mcp-dependencies 且 body.items 为 []
});
```

（测试主体按注释写全；`createSkillFetcher` 是既有 helper，按 `url.pathname` 分派扩展两条路由即可。）

- [ ] **Step 2: 跑测试确认失败**

```bash
corepack pnpm --filter @superteam/web test -- detail
```
Expected: FAIL（区块不存在）。

- [ ] **Step 3: 实现区块**

在 `SkillArchiveDetail` 的 `MasterDetailLayout`（detail.tsx:178-250）detail 栏追加区块（复用 `DetailSection` 269-294、行样式仿 `BindingList` 296-342、选择器抄 employee-capabilities-panel.tsx:311-330 的 `Select`）：

```tsx
function SkillMcpDependenciesSection({
  apiBaseUrl,
  fetcher,
  skillId,
}: {
  apiBaseUrl: string;
  fetcher?: typeof globalThis.fetch;
  skillId: string;
}) {
  const queryClient = useQueryClient();
  const options = { baseUrl: apiBaseUrl, fetcher };
  const dependencies = useQuery({
    queryKey: ["skill-mcp-dependencies", skillId],
    queryFn: () => listSkillMcpDependencies(options, skillId),
  });
  const registry = useQuery({
    queryKey: ["mcp-server-definitions"],
    queryFn: () => listMcpServerDefinitions(options),
  });
  const [selectedServerId, setSelectedServerId] = useState("");
  const replaceMutation = useMutation({
    mutationFn: (items: Array<{ mcp_server_id: string }>) =>
      replaceSkillMcpDependencies(options, skillId, { items }),
    onSuccess: () => {
      setSelectedServerId("");
      void queryClient.invalidateQueries({ queryKey: ["skill-mcp-dependencies", skillId] });
    },
  });

  const current = dependencies.data ?? [];
  const candidates = (registry.data ?? []).filter(
    (definition) => !current.some((dep) => dep.mcp_server_id === definition.id),
  );

  return (
    <DetailSection
      icon={<Network />}
      title="依赖 MCP"
      action={
        <div className="flex items-center gap-2">
          <Select value={selectedServerId} onValueChange={setSelectedServerId}>
            <SelectTrigger className="w-52" size="sm">
              <SelectValue placeholder="从注册表选择..." />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {candidates.map((definition) => (
                  <SelectItem key={definition.id} value={definition.id}>
                    {definition.name}（{definition.server_key}）
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <V3Button
            size="sm"
            disabled={!selectedServerId || replaceMutation.isPending}
            onClick={() =>
              replaceMutation.mutate([
                ...current.map((dep) => ({ mcp_server_id: dep.mcp_server_id })),
                { mcp_server_id: selectedServerId },
              ])
            }
          >
            添加依赖
          </V3Button>
        </div>
      }
    >
      {current.length === 0 ? (
        <V3EmptyState title="未声明依赖" description="该技能装载时不校验 MCP 能力" />
      ) : (
        <ul className="flex flex-col gap-2">
          {current.map((dep) => (
            <li key={dep.id} className="flex items-center justify-between gap-3 rounded-md border px-3 py-2">
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">{dep.server_name}</p>
                <p className="truncate font-mono text-xs text-muted-foreground">
                  {dep.server_key} · {dep.auth_strategy} · {dep.risk_level}
                </p>
              </div>
              <div className="flex items-center gap-2">
                <StatusPill tone={dep.server_status === "active" ? "ok" : "warn"}>
                  {dep.server_status}
                </StatusPill>
                <V3Button
                  aria-label={`移除依赖 ${dep.server_key}`}
                  variant="ghost"
                  size="sm"
                  disabled={replaceMutation.isPending}
                  onClick={() =>
                    replaceMutation.mutate(
                      current
                        .filter((item) => item.id !== dep.id)
                        .map((item) => ({ mcp_server_id: item.mcp_server_id })),
                    )
                  }
                >
                  <Trash2 />
                </V3Button>
              </div>
            </li>
          ))}
        </ul>
      )}
      <p className="mt-2 text-xs text-muted-foreground">
        依赖只做装载校验：员工执行任务时若未绑定所依赖的 MCP（或缺环境变量），派发会被阻断等待人工处理；依赖不会自动为员工开通能力。
      </p>
    </DetailSection>
  );
}
```

挂载位置：detail 栏"安装范围"区块之后。导入按需补（`Network`/`Trash2` 来自 lucide-react；`Select*` 来自 `@/components/ui/select`；`V3Button`/`V3EmptyState`/`StatusPill` 来自 `@/components/superteam`）。设计遵循 DESIGN.md：状态色只用 StatusPill，一行一个语义色；不手搓卡片。

- [ ] **Step 4: 跑测试确认通过 + Commit**

```bash
corepack pnpm --filter @superteam/web test -- detail
git add apps/web/src/features/skills/
git commit -m "feat(web): 技能详情页依赖MCP声明区块"
```

---

### Task 11: MCP 管理页被依赖展示与删除保护

**Files:**
- Modify: `apps/web/src/features/mcp/index.tsx`
- Modify: `apps/web/src/features/mcp/index.test.tsx`

**Interfaces:**
- Consumes: Task 9 的 `listMcpServerDependentSkills`；既有 `deleteMcpServerDefinition`。
- Produces: 删除按钮改为先弹 `ConfirmDialog`；对话框内异步显示被依赖技能；后端 409 时错误提示呈现依赖详情。

- [ ] **Step 1: 写失败测试（vi.mock 范式，对照 index.test.tsx:33-37）**

```tsx
// vi.mock("@/lib/api/capabilities") 的工厂里补 listMcpServerDependentSkills: vi.fn()
test("delete asks for confirmation and shows dependent skills", async () => {
  vi.mocked(listMcpServerDefinitions).mockResolvedValue([githubDefinition]);
  vi.mocked(listMcpServerDependentSkills).mockResolvedValue([
    { skill_id: "skill-1", slug: "deploy-helper", name: "部署助手" },
  ]);
  const screen = render(withClient(<McpManagementPage />));
  await screen.getByRole("button", { name: /删除/ }).click();
  await expect.element(screen.getByText(/deploy-helper/)).toBeVisible();
  // 确认后才调用 deleteMcpServerDefinition
  await screen.getByRole("button", { name: "删除" }).click();
  await vi.waitFor(() => {
    expect(vi.mocked(deleteMcpServerDefinition)).toHaveBeenCalledWith(
      expect.anything(),
      githubDefinition.id,
    );
  });
});
```

（按钮可访问名以实现为准，删除行按钮加 `aria-label="删除 <name>"`。）

- [ ] **Step 2: 跑测试确认失败**

```bash
corepack pnpm --filter @superteam/web test -- mcp
```
Expected: FAIL。

- [ ] **Step 3: 实现**

`index.tsx`：加状态 `const [pendingDelete, setPendingDelete] = useState<McpServerDefinition | null>(null)`；行内删除按钮（index.tsx:382-386）改为 `onClick={() => setPendingDelete(definition)}` 并加 `aria-label`。页面末尾挂 `ConfirmDialog`（用法对照 employees/detail.tsx:373-395）：

```tsx
const dependentSkills = useQuery({
  queryKey: ["mcp-dependent-skills", pendingDelete?.id],
  queryFn: () => listMcpServerDependentSkills({ baseUrl: apiBaseUrl }, pendingDelete!.id),
  enabled: pendingDelete !== null,
});

<ConfirmDialog
  open={pendingDelete !== null}
  onOpenChange={(open) => {
    if (!open) setPendingDelete(null);
  }}
  title={`删除 MCP 定义 ${pendingDelete?.name ?? ""}`}
  desc={
    dependentSkills.data && dependentSkills.data.length > 0
      ? `该定义被 ${dependentSkills.data.length} 个技能依赖（${dependentSkills.data
          .map((skill) => skill.slug)
          .join("、")}），删除将被服务端拒绝。请先在技能详情页移除依赖。`
      : "删除后员工绑定与任务装载将不再包含该定义。此操作不可撤销。"
  }
  confirmText="删除"
  destructive
  disabled={(dependentSkills.data?.length ?? 0) > 0}
  isLoading={deleteMutation.isPending}
  handleConfirm={() => {
    if (pendingDelete) {
      deleteMutation.mutate(pendingDelete.id, { onSettled: () => setPendingDelete(null) });
    }
  }}
/>
```

`deleteMutation` 增加 `onError` 把 `ApiRequestError`（409）的 detail 呈现在页面既有错误位（该页现有错误呈现方式为准）。顺带核对该页表格/表单样式与 `docs/design-system/data-display.md` 的表格规范（V3Table 已用，检查密度与空态文案即可，不做无关重构）。

- [ ] **Step 4: 跑测试确认通过 + Commit**

```bash
corepack pnpm --filter @superteam/web test -- mcp
git add apps/web/src/features/mcp/
git commit -m "feat(web): MCP删除确认对话框展示被依赖技能并阻止误删"
```

---

### Task 12: 员工能力面板缺依赖警示

**Files:**
- Modify: `apps/web/src/features/employees/components/employee-capabilities-panel.tsx`
- Create: `apps/web/src/features/employees/components/employee-capabilities-panel.test.tsx`

**Interfaces:**
- Consumes: Task 9 的 `listEmployeeSkillMcpDependencyStatus`。
- Produces: 个人技能卡内每个技能行下方渲染未满足的 MCP 依赖警示；面板顶部无全局大面积色块（DESIGN.md:67——非阻断场景用行内警示）。

- [ ] **Step 1: 写失败测试**

新建测试文件（面板 props 是 `{ apiOptions, employeeId }`，用 fetcher 注入范式；mock 依赖状态返回一条 `missing_binding`）：

```tsx
test("shows missing mcp dependency warning under skill row", async () => {
  const fetcher = createPanelFetcher({
    "/skill-mcp-dependency-status": [
      {
        skill_id: "skill-1",
        skill_slug: "deploy-helper",
        dependencies: [
          {
            mcp_server_id: "srv-1",
            server_key: "github-mcp",
            server_name: "GitHub MCP",
            status: "missing_binding",
            missing_env_vars: [],
          },
        ],
      },
    ],
    // 其余面板 query（skills/bindings/definitions/env）返回最小 fixture
  });
  const screen = render(
    withClient(<EmployeeCapabilitiesPanel apiOptions={{ baseUrl: "http://cp", fetcher }} employeeId="emp-1" />),
  );
  await expect.element(screen.getByText(/依赖 MCP github-mcp 未绑定/)).toBeVisible();
});
```

- [ ] **Step 2: 跑测试确认失败**

```bash
corepack pnpm --filter @superteam/web test -- employee-capabilities-panel
```
Expected: FAIL。

- [ ] **Step 3: 实现**

面板加 query：

```tsx
const skillMcpDependencyStatus = useQuery({
  queryKey: ["skill-mcp-dependency-status", employeeId],
  queryFn: () => listEmployeeSkillMcpDependencyStatus(apiOptions, employeeId),
});
```

在 `EmployeeSkillRow`（panel.tsx:421-466）传入该技能的未满足依赖列表（父组件按 `skill_id` 匹配），行内 pills 区（447-449 行 `skillLoadStatePills` 渲染处）追加：

```tsx
{unsatisfiedMcpDeps.map((dep) => (
  <StatusPill key={dep.mcp_server_id} tone="warn">
    缺 MCP {dep.server_key}
  </StatusPill>
))}
```

行下方警示文案（样式抄 `EffectiveMcpRegistrySection` 的缺 env 警示，panel.tsx:668-674）：

```tsx
{unsatisfiedMcpDeps.length > 0 ? (
  <p className="text-xs text-destructive">
    {unsatisfiedMcpDeps
      .map((dep) =>
        dep.status === "missing_binding"
          ? `依赖 MCP ${dep.server_key} 未绑定`
          : `依赖 MCP ${dep.server_key} 缺环境变量 ${dep.missing_env_vars.join(", ")}`,
      )
      .join("；")}
    ，任务派发将被阻断。请在右侧"个人 MCP"绑定或补齐环境变量。
  </p>
) : null}
```

- [ ] **Step 4: 跑测试确认通过 + Commit**

```bash
corepack pnpm --filter @superteam/web test -- employee-capabilities-panel
git add apps/web/src/features/employees/components/
git commit -m "feat(web): 员工面板技能行显示未满足的MCP依赖警示"
```

---

### Task 13: 分层门禁 + 真实 E2E

**Files:** 无新增（验证任务）。

- [ ] **Step 1: 分层门禁全绿**

```bash
corepack pnpm verify:foundation
corepack pnpm verify:web
corepack pnpm verify:runtime-agent
corepack pnpm verify:db
```
Expected: 全部 PASS。任何失败先修再进入 E2E。

- [ ] **Step 2: 服务起齐并确认加载当前代码**

```bash
./scripts/dev-services.sh restart control-plane
./scripts/dev-services.sh restart runtime-agent
./scripts/dev-services.sh status
```
Expected: 四个服务 running；control-plane 迁移 062 已应用。

- [ ] **Step 3: 真实链路 E2E（浏览器 + curl 混合，全程记录证据）**

场景 A——依赖声明与校验阻断：
1. `/mcp` 页注册一个 `auth_strategy=none` 的 HTTP MCP 定义（或复用已有）。
2. 技能详情页给某个已归档技能添加对它的依赖，刷新确认持久化。
3. 选一名**装载了该技能但未绑定该 MCP** 的员工，员工面板确认出现"缺 MCP"警示。
4. 对该员工发起任务派发（任务中枢），确认派发被阻断且失败原因含 `skill_mcp_dependencies_not_satisfied` 与技能/MCP 名（curl 查 run/任务详情接口核对结构化原因）。
5. 员工面板绑定该 MCP → 警示消失 → 重新派发成功。

场景 B——会话级注入与卸载（需 runtime 真实 smoke）：
1. 派发前记录员工 agent_home 下 provider 配置文件状态（如 `.mcp.json` 不存在，或 `.codex/config.toml` 预置一行自定义键）。
2. 派发任务，会话运行期间检查 agent_home：配置文件含本次绑定的 MCP entries，`.superteam/mcp-session-manifest.json` 存在。
3. 会话结束（完成或手动 stop）后检查：配置文件恢复原状（新建的被删除/预置键还原），manifest 消失。
4. 残留兜底：手动构造残留 manifest（复制一份改名回去）再派发一次，确认注入前先回滚不报错。

场景 C——删除保护：对被依赖的 MCP 定义在 `/mcp` 页点删除，确认对话框列出依赖技能且确认按钮禁用；curl 直接 DELETE 确认 409。

- [ ] **Step 4: 清理 E2E 数据 + 记录结论**

删除测试用 MCP 定义/依赖/绑定；E2E 通过后按 `$superteam-completion-check` 技能收尾。若任何环节无法真实验证（如 runtime provider 不可用），**标记阻塞并说明缺失依赖，不得以未验证状态交付**。

- [ ] **Step 5: Commit（如 E2E 过程中有修复）并汇报**

---

## Self-Review 已核对

- Spec 覆盖：迁移(§1→Task1-2)、API(§2→Task3-4,6)、闸门(§2→Task5)、会话作用域卸载(§3→Task7-8)、UI 三处(§4→Task10-12)、E2E(§5→Task13)、YAGNI 清单未越界。
- 类型一致性：`SkillMCPDependency`/`DependentSkill`/`RuntimeSkillRef`/`SkillMCPDependencyRecord`/`inject_session_mcp_config`/`rollback_session_mcp_config` 各任务间签名一致；web 类型与 Go DTO 字段名逐一对应。
- 已知留白（非占位符，是明确交给实现者的现场核对项）：sqlc 生成行结构字段大小写、`GetMCPServerDefinition` 仓储签名、`ListEffectiveMCPConfigForRuntime` 是否排除缺 env 项（Task 6 已写两种处理）、既有 fake 结构命名。
