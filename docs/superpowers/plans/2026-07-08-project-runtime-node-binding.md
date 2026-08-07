# 项目绑定可运行节点（Plan B）Implementation Plan
> 复核状态：已实现（基于CHANGELOG证据与锚点抽查）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 项目创建时多选可运行 runtime 节点（≥1）作为资格集；派发按「资格集(项目) → 员工亲和(软) → 任务硬钉(反漂移)」三层选节点，任务节点离线则暂停不漂移。

**Architecture:** 新增 `project_runtime_nodes`（资格集）与 `project_employee_node_affinity`（员工亲和）两表；节点解析集中在 `GetProjectTaskRunPreflight → NodeID`（现读单 active placement），改为三层解析：资格集 ∩ online，命中任务已有尝试节点则复用/离线暂停，否则按员工亲和优先、负载兜底并写回亲和；解析出的 NodeID 经 `run_service.go` 落为 runtime task 的 `target_node_id`（pull 模型硬钉，天然禁漂移）。

**Tech Stack:** Go (control-plane, chi, sqlc, Atlas), OpenAPI + oapi-codegen, React + TanStack Router/Query + Vitest。

## Global Constraints

- 依据 spec §7：`docs/superpowers/specs/2026-07-08-team-governance-slimming-and-project-runtime-binding-design.md`。
- 迁移唯一目录 `apps/control-plane/internal/storage/migrations/`；更新 `atlas.sum`，用 `make -C apps/control-plane migrate-validate` 校验。
- 契约生成：`corepack pnpm generate:control-plane`；契约验证：`corepack pnpm verify:contracts`；sqlc：`make -C apps/control-plane generate-sqlc`。
- Web 测试只用 `corepack pnpm --filter ./apps/web run test`；内部跳转用 TanStack Router `Link`/`navigate`。
- `project_placements` 语义不改、不删，仅退出选节点权威。
- Plan B 独立于 Plan A，可在其后或并行执行（无共享文件冲突）。
- 提交信息结尾附：`Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`。
- 迁移号取当前最大 +1（若 A 已落 046/047，则 B 用 048；执行时以 `ls migrations/` 实际最大号 +1 为准）。

## File Structure（本计划触及）

- 迁移：`apps/control-plane/internal/storage/migrations/0NN_project_runtime_nodes_and_affinity.sql`、`atlas.sum`
- sqlc：新增 `apps/control-plane/internal/storage/queries/project_runtime_nodes.sql`(.go)
- Go project：`apps/control-plane/internal/project/{types,service,repository,pg_repository,handler}.go`
- Go employee 解析点：`apps/control-plane/internal/employee/{run_service}.go` + `GetProjectTaskRunPreflight` 实现（`pg_repository.go`）
- predispatch：`apps/control-plane/internal/workflow/projectcoordination/predispatch_gate.go`
- 路由/契约：`apps/control-plane/internal/api/server.go`、`contracts/control-plane/openapi.yaml` + 生成物
- Web：`apps/web/src/features/projects/components/create-project/{create-project-draft.ts,create-project-shell.tsx,project-runtime-nodes-step.tsx(新)}`、`apps/web/src/lib/api/projects.ts`

---

### Task 1: 迁移 — project_runtime_nodes（资格集）+ project_employee_node_affinity（亲和）

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/0NN_project_runtime_nodes_and_affinity.sql`
- Modify: `atlas.sum`
- Test: `apps/control-plane/internal/storage/migrations_test.go`

**Interfaces:**
- Produces: 表 `project_runtime_nodes(tenant_id, project_id, runtime_node_id, created_at)`（唯一 `(project_id, runtime_node_id)`）；`project_employee_node_affinity(tenant_id, project_id, digital_employee_id, runtime_node_id, last_run_at, created_at, updated_at)`（唯一 `(project_id, digital_employee_id)`）。

- [ ] **Step 1: 写迁移 SQL**
```sql
-- Project runtime eligibility set (chosen at creation) + soft per-employee
-- affinity used at dispatch. project_placements stays but no longer drives
-- node selection.
CREATE TABLE project_runtime_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    runtime_node_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_project_runtime_nodes_project
        FOREIGN KEY (tenant_id, project_id)
        REFERENCES projects(tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT uq_project_runtime_nodes UNIQUE (project_id, runtime_node_id)
);

CREATE TABLE project_employee_node_affinity (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    digital_employee_id UUID NOT NULL,
    runtime_node_id UUID NOT NULL,
    last_run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_project_employee_node_affinity_project
        FOREIGN KEY (tenant_id, project_id)
        REFERENCES projects(tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT uq_project_employee_node_affinity UNIQUE (project_id, digital_employee_id)
);
CREATE INDEX idx_project_runtime_nodes_project ON project_runtime_nodes (tenant_id, project_id);
```

- [ ] **Step 2: 校验迁移**

Run: `make -C apps/control-plane migrate-validate`
Expected: 通过；`atlas.sum` 更新。

- [ ] **Step 3: 断言测试**
```go
func TestMigrationProjectRuntimeNodes(t *testing.T) {
    db := applyAllMigrations(t)
    assertTableExists(t, db, "project_runtime_nodes")
    assertTableExists(t, db, "project_employee_node_affinity")
}
```

- [ ] **Step 4: 跑测试**

Run: `go test ./apps/control-plane/internal/storage/... -run TestMigrationProjectRuntimeNodes -v`
Expected: PASS

- [ ] **Step 5: 提交**
```bash
git add apps/control-plane/internal/storage/migrations apps/control-plane/internal/storage/migrations_test.go
git commit -m "feat(migrate): add project_runtime_nodes and project_employee_node_affinity"
```

---

### Task 2: sqlc 查询 — 资格集与亲和的读写

**Files:**
- Create: `apps/control-plane/internal/storage/queries/project_runtime_nodes.sql`
- Regenerate: `project_runtime_nodes.sql.go`
- Test: `apps/control-plane/internal/storage/queries/queries_test.go`

**Interfaces:**
- Produces sqlc 方法：`InsertProjectRuntimeNode`、`ListProjectRuntimeNodes`、`UpsertProjectEmployeeNodeAffinity`、`GetProjectEmployeeNodeAffinity`。

- [ ] **Step 1: 写查询**
```sql
-- name: InsertProjectRuntimeNode :one
INSERT INTO project_runtime_nodes (tenant_id, project_id, runtime_node_id)
VALUES (sqlc.arg('tenant_id')::uuid, sqlc.arg('project_id')::uuid, sqlc.arg('runtime_node_id')::uuid)
ON CONFLICT (project_id, runtime_node_id) DO NOTHING
RETURNING *;

-- name: ListProjectRuntimeNodes :many
SELECT * FROM project_runtime_nodes
WHERE tenant_id = sqlc.arg('tenant_id')::uuid AND project_id = sqlc.arg('project_id')::uuid
ORDER BY created_at ASC;

-- name: UpsertProjectEmployeeNodeAffinity :one
INSERT INTO project_employee_node_affinity (tenant_id, project_id, digital_employee_id, runtime_node_id)
VALUES (sqlc.arg('tenant_id')::uuid, sqlc.arg('project_id')::uuid, sqlc.arg('digital_employee_id')::uuid, sqlc.arg('runtime_node_id')::uuid)
ON CONFLICT (project_id, digital_employee_id)
DO UPDATE SET runtime_node_id = EXCLUDED.runtime_node_id, last_run_at = NOW(), updated_at = NOW()
RETURNING *;

-- name: GetProjectEmployeeNodeAffinity :one
SELECT * FROM project_employee_node_affinity
WHERE tenant_id = sqlc.arg('tenant_id')::uuid AND project_id = sqlc.arg('project_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid;
```

- [ ] **Step 2: 生成 sqlc**

Run: `make -C apps/control-plane generate-sqlc`
Expected: `project_runtime_nodes.sql.go` 生成。

- [ ] **Step 3: 写查询测试 + 跑**
```go
func TestProjectRuntimeNodesQueries(t *testing.T) {
    q, ctx, tenantID, projectID := seedProject(t)
    _, err := q.InsertProjectRuntimeNode(ctx, InsertProjectRuntimeNodeParams{TenantID: tenantID, ProjectID: projectID, RuntimeNodeID: nodeA})
    require.NoError(t, err)
    nodes, err := q.ListProjectRuntimeNodes(ctx, ListProjectRuntimeNodesParams{TenantID: tenantID, ProjectID: projectID})
    require.NoError(t, err)
    require.Len(t, nodes, 1)
}
```
Run: `go test ./apps/control-plane/internal/storage/queries/... -run TestProjectRuntimeNodesQueries -v`
Expected: PASS

- [ ] **Step 4: 提交**
```bash
git add apps/control-plane/internal/storage/queries
git commit -m "feat(storage): queries for project runtime nodes and employee affinity"
```

---

### Task 3: 契约 — CreateProjectRequest.runtime_node_ids + 资格集读取端点

**Files:**
- Modify: `contracts/control-plane/openapi.yaml`（`CreateProjectRequest`；新增 `GET /projects/{projectId}/runtime-nodes`）
- Regenerate: `apps/control-plane/gen/control_plane.gen.go`、`apps/control-plane/internal/api/gen/control_plane.gen.go`

**Interfaces:**
- Produces: 请求体字段 `runtime_node_ids: [uuid]`（required, minItems 1）；`ProjectRuntimeNode` schema `{ runtime_node_id }`。

- [ ] **Step 1: 改契约**

`CreateProjectRequest`：`required` 增加 `runtime_node_ids`；properties 增加
```yaml
        runtime_node_ids:
          type: array
          minItems: 1
          items:
            type: string
            format: uuid
```
新增路径 `GET /api/v1/projects/{projectId}/runtime-nodes` 返回 `ProjectRuntimeNode[]`。

- [ ] **Step 2: 生成 + 验证**

Run: `corepack pnpm generate:control-plane && corepack pnpm verify:contracts`
Expected: 生成物更新；契约验证通过。

- [ ] **Step 3: 提交**
```bash
git add contracts apps/control-plane/gen apps/control-plane/internal/api/gen
git commit -m "feat(contract): add project runtime_node_ids and runtime-nodes endpoint"
```

---

### Task 4: Go project — CreateProject 校验并写入资格集；资格集读端点

**Files:**
- Modify: `apps/control-plane/internal/project/types.go`（`CreateProjectRequest` 加 `RuntimeNodeIDs []uuid.UUID`）
- Modify: `apps/control-plane/internal/project/handler.go`（241-268 解析 `runtime_node_ids`；新增 `ListProjectRuntimeNodes` handler）
- Modify: `apps/control-plane/internal/project/service.go`（`CreateProject` 校验 + 写入；新增 `ListProjectRuntimeNodes`）
- Modify: `apps/control-plane/internal/project/{repository,pg_repository}.go`（`InsertProjectRuntimeNode`/`ListProjectRuntimeNodes` 封装）
- Modify: `apps/control-plane/internal/api/server.go`（注册 GET runtime-nodes 路由）
- Test: `apps/control-plane/internal/project/service_test.go`

**Interfaces:**
- Consumes: Task 2 sqlc 方法、Task 3 契约字段。
- Produces:
```go
// service 层
CreateProject(...) // 校验 len(RuntimeNodeIDs) >= 1、去重、同租户存在，随项目插入 project_runtime_nodes
ListProjectRuntimeNodes(ctx, tenantID, projectID uuid.UUID) ([]ProjectRuntimeNode, error)
// 错误
ErrProjectRuntimeNodesRequired = errors.New("project requires at least one runtime node")
```

- [ ] **Step 1: 写失败测试**
```go
func TestCreateProjectRequiresRuntimeNodes(t *testing.T) {
    svc, ctx, req := newProjectServiceReady(t)
    req.RuntimeNodeIDs = nil
    _, err := svc.CreateProject(ctx, req)
    require.ErrorIs(t, err, project.ErrProjectRuntimeNodesRequired)
}
func TestCreateProjectPersistsRuntimeNodes(t *testing.T) {
    svc, ctx, req := newProjectServiceReady(t) // req.RuntimeNodeIDs = [nodeA, nodeB]
    created, err := svc.CreateProject(ctx, req)
    require.NoError(t, err)
    nodes, err := svc.ListProjectRuntimeNodes(ctx, req.TenantID, created.Project.ID)
    require.NoError(t, err)
    require.Len(t, nodes, 2)
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./apps/control-plane/internal/project/... -run 'TestCreateProjectRequiresRuntimeNodes|TestCreateProjectPersistsRuntimeNodes' -v`
Expected: FAIL

- [ ] **Step 3: 实现**

- `types.go`：`CreateProjectRequest` 加 `RuntimeNodeIDs []uuid.UUID`；新增 `ProjectRuntimeNode` 域类型。
- `handler.go`：`createProjectBody` 加 `RuntimeNodeIDs []uuid.UUID json:"runtime_node_ids"`；`CreateProject`（254）透传；新增 `ListProjectRuntimeNodes` handler + `createProjectResponse` 无需变。
- `service.go` `CreateProject`：入口校验 `len(req.RuntimeNodeIDs) >= 1`（否则 `ErrProjectRuntimeNodesRequired`），去重，逐个 `requireRuntimeNodeForTenant`（复用 285 已有校验器）；在项目插入事务内 `InsertProjectRuntimeNode`。新增 `ListProjectRuntimeNodes`。
- `pg_repository.go`：封装两个 sqlc 调用为域方法。
- `server.go`：`r.Get("/projects/{projectId}/runtime-nodes", s.projectHandler.ListProjectRuntimeNodes)`。

- [ ] **Step 4: 跑测试 + 构建 + 契约**

Run: `go build ./apps/control-plane/... && go test ./apps/control-plane/internal/project/... -run 'TestCreateProject' -v && corepack pnpm verify:contracts`
Expected: PASS

- [ ] **Step 5: 提交**
```bash
git add apps/control-plane
git commit -m "feat(project): require and persist runtime node eligibility set on create"
```

---

### Task 5: 三层节点解析器 — 资格集 ∩ online → 任务硬钉/暂停 → 员工亲和 → 负载兜底

**Files:**
- Modify: `apps/control-plane/internal/employee/pg_repository.go`（`GetProjectTaskRunPreflight` 节点解析）或新增 `project` 侧解析服务被其调用
- Modify: `apps/control-plane/internal/project/service.go`（就绪度 339-378 用同一解析器；退出 `GetActiveProjectPlacement` 作为选节点源）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/predispatch_gate.go`（86-97 gate reason：`placement_missing` → `no_eligible_online_node` / `pinned_node_offline`）
- Test: `apps/control-plane/internal/project/service_test.go`（解析器单测）

**Interfaces:**
- Consumes: Task 2 `ListProjectRuntimeNodes`/`Get/UpsertProjectEmployeeNodeAffinity`；现有任务尝试 `RuntimeNodeID`、runtime 节点 online/负载查询、`run_service.go:267 TargetNodeID = preflight.NodeID`。
- Produces:
```go
// 解析器（放 project 包，employee 侧 GetProjectTaskRunPreflight 调用它填 NodeID）
type NodeResolution struct {
    NodeID   uuid.UUID // 解析出的运行节点
    Pinned   bool      // 是否来自任务硬钉（已有尝试）
    Paused   bool      // 钉定节点离线 → 暂停
    Reason   string    // no_eligible_online_node / pinned_node_offline / ""
}
ResolveProjectTaskNode(ctx, ResolveProjectTaskNodeInput{
    TenantID, ProjectID, DigitalEmployeeID, ProjectTaskID uuid.UUID
    PinnedNodeID *uuid.UUID // 任务已有尝试的 RuntimeNodeID（无则 nil）
    RequiredProvider string
}) (NodeResolution, error)
```

- [ ] **Step 1: 写失败测试（覆盖四条路径）**
```go
func TestResolveNode_NewTaskPrefersAffinity(t *testing.T) {
    // eligible={A,B} online; affinity(P,E)=B → 选 B, Pinned=false
}
func TestResolveNode_NewTaskAffinityOfflineFallsBack(t *testing.T) {
    // affinity=B offline, A online → 选 A（跨任务允许换）
}
func TestResolveNode_PinnedTaskReusesNode(t *testing.T) {
    // PinnedNodeID=A online ∈ eligible → 选 A, Pinned=true（禁漂移）
}
func TestResolveNode_PinnedNodeOfflinePauses(t *testing.T) {
    // PinnedNodeID=A offline → Paused=true, Reason="pinned_node_offline", 不换节点
}
func TestResolveNode_NoEligibleOnlineBlocks(t *testing.T) {
    // eligible 全 offline → Reason="no_eligible_online_node"
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./apps/control-plane/internal/project/... -run TestResolveNode -v`
Expected: FAIL（`ResolveProjectTaskNode` 未定义）。

- [ ] **Step 3: 实现解析器**

算法（严格按 spec §7.2）：
```
eligible = ListProjectRuntimeNodes(P) 的 runtime_node_id 集合
if PinnedNodeID != nil:
    if *PinnedNodeID ∈ eligible 且 online 且有空槽: return {NodeID:*PinnedNodeID, Pinned:true}
    else: return {Paused:true, Reason:"pinned_node_offline"}   // 不换节点
candidates = eligible ∩ online ∩ 有空槽 ∩ 支持 RequiredProvider
if candidates 空: return {Reason:"no_eligible_online_node"}
aff = GetProjectEmployeeNodeAffinity(P,E)
chosen = (aff.node ∈ candidates) ? aff.node : lowestLoad(candidates)
UpsertProjectEmployeeNodeAffinity(P,E,chosen)
return {NodeID: chosen, Pinned:false}
```
- `GetProjectTaskRunPreflight`：把 `projectPreflight.NodeID` 来源从单 active placement 改为 `ResolveProjectTaskNode(...)`，`PinnedNodeID` 取该任务已有尝试的 `RuntimeNodeID`（无尝试则 nil）。`Paused`/`Reason` 非空时返回带阻断信息的 preflight（供 gate 处理）。
- `service.go` 就绪度（339-378）：`RuntimeNodeID` 与阻断原因改用解析器结果，不再直接 `GetActiveProjectPlacement`。
- `predispatch_gate.go`：blocker key `runtime.placement_missing` → 依解析结果映射为 `runtime.no_eligible_online_node`（阻断可重试）或 `runtime.pinned_node_offline`（暂停等待）。保留 `bind_runtime` 动作用于「项目无资格集」这一不应发生的兜底。

- [ ] **Step 4: 跑测试 + 构建**

Run: `go build ./apps/control-plane/... && go test ./apps/control-plane/internal/project/... ./apps/control-plane/internal/employee/... -run 'TestResolveNode|Preflight|PreDispatch' -v`
Expected: PASS

- [ ] **Step 5: 提交**
```bash
git add apps/control-plane
git commit -m "feat(project): three-layer runtime node resolver with affinity and task pinning"
```

---

### Task 6: Web — 项目创建向导「可运行节点」多选（≥1）

**Files:**
- Modify: `apps/web/src/features/projects/components/create-project/create-project-draft.ts`（`ProjectCreateStep` 加 `runtimeNodes`；draft 加 `runtimeNodeIds`；`projectCreateSteps` 加一步）
- Create: `apps/web/src/features/projects/components/create-project/project-runtime-nodes-step.tsx`
- Modify: `apps/web/src/features/projects/components/create-project/create-project-shell.tsx`（渲染新步 + 校验 ≥1）
- Modify: `apps/web/src/lib/api/projects.ts`（`CreateProjectInput` 加 `runtime_node_ids`；提交映射）
- Test: `apps/web/src/features/projects/components/create-project/create-project-page.test.tsx`

**Interfaces:**
- Consumes: `listRuntimeNodes`（`apps/web/src/lib/api/runtime.ts:158`）、Task 3 契约字段。
- Produces: 提交 payload 含 `runtime_node_ids`（≥1，否则该步不可继续）。

- [ ] **Step 1: 写失败测试**
```tsx
it("requires selecting at least one runtime node", async () => {
  renderCreateProject({ nodes: [{ node_id: "n1", name: "A", status: "online" }] });
  await gotoStep("runtimeNodes");
  expect(screen.getByRole("button", { name: "下一步" })).toBeDisabled();
  await userEvent.click(screen.getByRole("checkbox", { name: /A/ }));
  expect(screen.getByRole("button", { name: "下一步" })).toBeEnabled();
});
it("submits selected runtime_node_ids", async () => {
  const { submitSpy } = renderCreateProject({ nodes: [{ node_id: "n1", name: "A", status: "online" }] });
  await fillMinimalProjectAndSelectNode("n1");
  await submitProject();
  expect(submitSpy).toHaveBeenCalledWith(expect.objectContaining({ runtime_node_ids: ["n1"] }));
});
```

- [ ] **Step 2: 跑测试确认失败**

Run: `corepack pnpm --filter ./apps/web run test -- create-project`
Expected: FAIL

- [ ] **Step 3: 实现**

- `create-project-draft.ts`：`ProjectCreateStep` 加 `"runtimeNodes"`；`projectCreateSteps` 在 `digitalEmployees` 后插 `{ id: "runtimeNodes", label: "可运行节点" }`；draft 加 `runtimeNodeIds: string[]`（空数组）；`emptyProjectCreateDraft` 补 `runtimeNodeIds: []`。
- `project-runtime-nodes-step.tsx`：`useQuery(listRuntimeNodes)` 列出节点（名/状态/负载/provider），多选 checkbox 写 `runtimeNodeIds`；offline 节点可选但标注（资格集允许含离线，派发时才过滤）。
- `create-project-shell.tsx`：渲染新步；「下一步」在 `activeStep==="runtimeNodes" && draft.runtimeNodeIds.length===0` 时禁用。
- `projects.ts`：`CreateProjectInput` 加 `runtime_node_ids: string[]`；提交映射 `runtime_node_ids: draft.runtimeNodeIds`。

- [ ] **Step 4: 跑测试**

Run: `corepack pnpm --filter ./apps/web run test -- create-project`
Expected: PASS

- [ ] **Step 5: 提交**
```bash
git add apps/web/src/features/projects apps/web/src/lib/api/projects.ts
git commit -m "feat(web): project create runtime node multi-select (min 1)"
```

---

### Task 7: 真实端到端验证（CLAUDE.md 强制完成条件）

**Files:** 无（验证任务）。

**Interfaces:** Consumes 全部前序任务（若 Plan A 已合并，一并在同一运行环境验证）。

- [ ] **Step 1: 迁移 + 重启**

Run: `scripts/dev-services.sh restart control-plane && scripts/dev-services.sh status`
Expected: 迁移自动执行、服务全绿。至少注册 2 个 runtime 节点（A、B）以验证亲和/兜底/暂停（可用现有 runtime-agent 或 fake 注册；注意 CLAUDE.md：真实执行链需真实 Runtime/Provider smoke）。

- [ ] **Step 2: 建项目多选 ≥1 节点**

Web 建项目：可运行节点步多选 A、B。空选时「下一步」禁用（≥1 强制）。
Expected: 创建成功；`GET /projects/{id}/runtime-nodes` 返回 A、B（curl 真实接口）。

- [ ] **Step 3: 首个任务按亲和/负载落节点**

不手动 pin，直接派发该项目一个任务给员工 E。
Expected: 任务落在资格集内某 online 节点（runtime task `target_node_id` = 该节点）；`project_employee_node_affinity(P,E)` 写入该节点；不再 `placement_missing`。

- [ ] **Step 4: 重试/续跑硬钉不漂移**

让该任务重试一次（同任务）。
Expected: 仍落原节点（`target_node_id` 不变，Pinned）；`service.go:2559` 错配守卫不触发。

- [ ] **Step 5: 钉定节点离线 → 任务暂停**

将该任务所在节点下线，触发同任务再调度。
Expected: 任务进入暂停（gate reason `runtime.pinned_node_offline`），**不迁移**到另一节点。

- [ ] **Step 6: 新任务在上次节点离线时换节点**

派发该员工一个新任务（上次节点仍离线）。
Expected: 新任务落到资格集内另一 online 节点（亲和兜底，跨任务允许换）；affinity 更新为新节点。

- [ ] **Step 7: 收尾检查**

Run: `.codex/skills/superteam-completion-check/SKILL.md` 流程。
Expected: 完成前检查通过；确认运行中的 Web/Control Plane 已加载当前代码、走真实接口非 mock。

- [ ] **Step 8: 合并与分支收尾**

按 CLAUDE.md：合并 main 后基于 main 再走一次真实仿真验证通过，才删分支/worktree；验证阻塞则标记阻塞、不声明完成。

## Self-Review

- **Spec 覆盖**：§7.2 三层模型→T1/T2/T5；资格集创建 ≥1→T3/T4/T6；亲和软黏→T2/T5；任务硬钉+暂停→T5（复用 target_node_id/attempt RuntimeNodeID）；predispatch reason 替换→T5；Web 多选→T6；e2e 四条路径→T7。无未覆盖项。
- **占位扫描**：命令均为真实 target（`make -C apps/control-plane migrate-validate`/`generate-sqlc`、`corepack pnpm generate:control-plane`/`verify:contracts`、`corepack pnpm --filter ./apps/web run test`）；迁移号 `0NN` 为执行时取最大 +1 的显式说明，非占位。
- **类型一致**：`NodeResolution{NodeID,Pinned,Paused,Reason}`、`ResolveProjectTaskNode`、`RuntimeNodeIDs`、`ProjectRuntimeNode`、sqlc `InsertProjectRuntimeNode/ListProjectRuntimeNodes/Upsert|GetProjectEmployeeNodeAffinity` 跨任务命名一致。
- **风险**：T5 触及派发链（preflight/gate/就绪度），需确认 `GetProjectTaskRunPreflight` 能取到任务已有尝试节点以判断硬钉；若该 preflight 签名不含 task/attempt 上下文，T5 Step 3 需先扩展其入参（子步骤：加 `ProjectTaskID`/`PinnedNodeID` 到 preflight 输入）。
