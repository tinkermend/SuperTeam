# Plan Phase Refactor — Plan 6: 会话降维到（员工, 血缘根） Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 B'（B 的返工任务）续接 B 那个任务的 provider 会话,而不是开新会话或共用员工级长会话。会话键从「员工级」降到「（员工, 任务血缘根）」级:同一血缘根复用,同员工的不同任务隔离。

**Architecture:** 判定与查询放在 **control-plane**（它持有血缘根,runtime 不持有）。`provider_sessions` 加 `project_task_id` 维度（存血缘根）。control-plane 派发时按血缘根解析会话,把 `provider_session_id` 传给 runtime；runtime 的 `reusable_provider_session` 仅保留"是否复用"的布尔语义,会话身份由 control-plane 决定。

**Tech Stack:** Go 1.x + sqlc、Rust、PostgreSQL 迁移。门禁 `corepack pnpm verify:control-plane`、`corepack pnpm verify:runtime-agent`。

## Global Constraints

取自 spec `docs/superpowers/specs/2026-07-10-project-plan-phase-refactor-design.md`（§4.9）:

- **约束三:续接会话用于干活；裁决一律新开干净上下文。** B' 续接 B 的会话是为了继承上下文继续干活。
- **会话键是血缘根,不是 `project_task_id`。** B' 是新任务（新 id）；按 task_id 键控会给 B' 一条空会话,丢掉要继承的上下文。`revisionRootTaskID`（`project_store.go:3117`）沿 `RevisionOfTaskID` 回溯,主图任务的根是自己。
- **runtime 不做血缘判定。** control-plane 持有 `RevisionOfTaskID`,runtime 不持有。会话归属在 control-plane 解析,runtime 只接收 `provider_session_id`。
- 不改 `hasCycle`。
- 提交信息末尾附 `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`。

---

## 前置:机制现状

| 机制 | 位置 | 现状 |
|---|---|---|
| `reusable_provider_session` | `apps/runtime-agent/src/commands/executor.rs:2941` | 只看 `recoverable && mode != Ephemeral`,**不看 task** |
| `provider_sessions` 唯一键 | `uq_provider_sessions_external (tenant, provider_type, provider_session_id)` | **无 task 维度**,故同一员工一条长会话 |
| runtime payload metadata | `apps/control-plane/internal/employee/run_service.go:951` | 带 `project_task_id`、`project_task_attempt_id`,**不带血缘根** |
| `RevisionOfTaskID` / `revisionRootTaskID` | `project_store.go:3117` | control-plane 侧持有,不进 runtime |

**病根**:`reusable_provider_session` 与 `provider_sessions` 都没有任务维度,导致员工在同一项目做多个任务时共用一条长会话——上下文互相污染,且与 Plan 5 的「B' 续接 B」语义冲突（B' 会续到 D 的会话）。

**改造落点**:
- runtime 侧 `reusable_provider_session` 不改语义（仍只答布尔）。会话身份（`provider_session_id`）由 control-plane 决定后注入。
- control-plane 派发时:按（员工, 血缘根）解析会话 → 若复用,传该 session id；否则不传,runtime 新建。
- `provider_sessions` 加 `project_task_root_id`（存血缘根）,用于"同一根复用同一条会话"。

---

## File Structure

| 文件 | 职责 | 动作 |
|---|---|---|
| `apps/control-plane/internal/storage/migrations/055_provider_session_task_scope.sql` | 迁移 | Task 1:加 `project_task_root_id` |
| `apps/control-plane/internal/storage/queries/provider_session*.sql` | 会话查询 | Task 2:按血缘根查会话 |
| `apps/control-plane/internal/employee/run_service.go` | 派发时注入 session id | Task 3:血缘根解析 + 注入 metadata |
| `apps/runtime-agent/src/commands/payload.rs` / `executor.rs` | runtime 会话复用 | Task 4:保留布尔语义,身份由 metadata |

---

### Task 1: `provider_sessions` 加血缘根维度

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/055_provider_session_task_scope.sql`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`

**Interfaces:**
- Consumes: 无。
- Produces: `provider_sessions.project_task_root_id uuid`（nullable,存血缘根；旧数据为 null 表示员工级长会话,向后兼容）。

- [ ] **Step 1: 写迁移**

```sql
-- Provider session is scoped to (employee, task lineage root), not just employee.
-- A revision task B' shares its root (B) with the original, so it resumes B's
-- session. A different task D under the same employee gets a fresh session. See
-- the 2026-07-10 plan-phase refactor spec §4.9.
ALTER TABLE provider_sessions
    ADD COLUMN project_task_root_id UUID;

CREATE INDEX idx_provider_sessions_task_root
    ON provider_sessions (tenant_id, digital_employee_id, project_task_root_id);

COMMENT ON COLUMN provider_sessions.project_task_root_id IS 'Task lineage root this session is scoped to; null for pre-refactor employee-level sessions.';
```

- [ ] **Step 2: 更新 atlas.sum**

```bash
atlas migrate hash --dir file://apps/control-plane/internal/storage/migrations
```

- [ ] **Step 3: 校验**

```bash
make -C apps/control-plane migrate-validate
# 若无 docker: atlas migrate validate --dir file://apps/control-plane/internal/storage/migrations
```

- [ ] **Step 4: Commit**

```bash
git add apps/control-plane/internal/storage/migrations/
git commit -m "$(cat <<'EOF'
db: scope provider_sessions to task lineage root

A provider session was keyed by employee, so every task an employee ran shared one
long context — polluted across tasks, and at odds with the plan-5 notion that a
revision task resumes its source's session. The session now carries the task
lineage root; B' resolves to B's session, a different task D gets its own.

Existing rows keep null (employee-level legacy sessions), so this is backward
compatible.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: 按血缘根查询会话

**Files:**
- Modify: `apps/control-plane/internal/storage/queries/provider_session.sql`（或新建,参照既有 query 文件命名）
- Regenerate: `make -C apps/control-plane generate-sqlc`
- Test: `apps/control-plane/internal/employee/*_test.go`

**Interfaces:**
- Consumes: Task 1 的 `project_task_root_id`；control-plane 的 `revisionRootTaskID`（`project_store.go:3117`）。
- Produces: 查询 `FindProviderSessionForTaskRoot(tenant, employee, taskRoot) → provider_session_id | nil`。

- [ ] **Step 1: 写查询**

在 `provider_session.sql`（或新建）新增:

```sql
-- name: FindProviderSessionForTaskRoot :one
SELECT provider_session_id
FROM provider_sessions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND project_task_root_id = sqlc.arg('project_task_root_id')::uuid
  AND status = 'active'
ORDER BY last_active_at DESC
LIMIT 1;
```

- [ ] **Step 2: 生成 sqlc**

```bash
make -C apps/control-plane generate-sqlc
```

- [ ] **Step 3: 写测试**

```go
// 同一 (employee, root) 第二次派发应命中第一次的 session；
// 不同 root 应返回 nil（新建）。
```

参照 `employee` 包既有 repository 测试模式。

- [ ] **Step 4: Commit**

```bash
git add apps/control-plane/internal/
git commit -m "$(cat <<'EOF'
feat(storage): look up provider session by task lineage root

Adds FindProviderSessionForTaskRoot so a revision task resolves to its source's
session. The query matches (tenant, employee, root); a different task root yields
no row, so a fresh session is created.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: 派发时注入血缘根与会话 id

**Files:**
- Modify: `apps/control-plane/internal/employee/run_service.go`（`projectTaskRunMetadata` `:945` 与派发路径）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`（`revisionRootTaskID` 已存在,确认可取）

**Interfaces:**
- Consumes: Task 2 的 `FindProviderSessionForTaskRoot`；`revisionRootTaskID`。
- Produces: runtime command 的 metadata 带 `provider_session_id`（若复用）与 `revision_root_task_id`。

- [ ] **Step 1: 在 `projectTaskRunMetadata` 注入血缘根**

`run_service.go:945` 的 `projectTaskRunMetadata`,在 `project_task_attempt_id` 之后:

```go
	// The runtime needs the lineage root only for display/audit; session identity
	// is resolved here and passed as provider_session_id below.
	if taskRootID := taskLineageRoot(req); taskRootID != "" {
		metadata["revision_root_task_id"] = taskRootID
	}
```

`taskLineageRoot(req)` 取该任务的血缘根。**`StartProjectTaskRunRequest`（`run_types.go:172`）只带 `ProjectTaskID`,不带 `RevisionOfTaskID`**;preflight（`GetRunPreflight`,`run_service.go:127`）也不带。故需在 `createAndDispatchRun` 里按 `req.ProjectTaskID` 查 `project_tasks.revision_of_task_id`:

```go
// fetch once, reuse for metadata + session lookup
taskRecord, err := s.repository.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
if err != nil {
	return StartProjectTaskRunResult{}, fmt.Errorf("get project task for lineage: %w", err)
}
taskRoot := req.ProjectTaskID.String()
if taskRecord.RevisionOfTaskID != nil && *taskRecord.RevisionOfTaskID != uuid.Nil {
	taskRoot = taskRecord.RevisionOfTaskID.String()
}
```

`GetProjectTask` 已存在（repository 上）。若签名不同,用 `ListProjectTasksByCoordinationJob` 或等价查询取 `RevisionOfTaskID`。执行者先 `grep -rn "GetProjectTask" apps/control-plane/internal/employee` 确认可用签名。

- [ ] **Step 2: 按血缘根解析会话并注入 `provider_session_id`**

在派发组装 runtime command 时（`run_service.go` 内,构造 `RuntimeSessionCommandPayload`/metadata 处）:

```go
	if preflight.SessionPolicy.Recoverable && preflight.SessionPolicy.Mode != "ephemeral" {
		root := taskLineageRoot(req)
		if root != "" {
			if sessionID, err := repo.FindProviderSessionForTaskRoot(ctx, tenantID, employeeID, root); err == nil && sessionID != "" {
				metadata["provider_session_id"] = sessionID
			}
		}
	}
```

> **执行者注意**:具体 `preflight`/`repo` 的取用方式依 `run_service.go` 既有结构。若 session_policy 在别处读取,在对应位置注入。关键是:**会话身份在此决定,runtime 不再自决**。

- [ ] **Step 3: 测试**

```go
// B' 派发时 metadata.provider_session_id == B 的 session id；
// D（同员工不同根）派发时 metadata 无 provider_session_id（或为 B 外的值）。
```

- [ ] **Step 4: Commit**

```bash
git add apps/control-plane/internal/
git commit -m "$(cat <<'EOF'
feat(dispatch): inject lineage-root session id into runtime payload

The control plane resolves which provider session a task should resume and passes
it as provider_session_id. A revision task resolves to its source's session; a
different task gets none and the runtime creates a fresh one. The runtime no longer
decides session identity on its own.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: runtime 会话身份由 control-plane 决定

**Files:**
- Modify: `apps/runtime-agent/src/commands/payload.rs`（确认 `provider_session_id` 从 metadata 读）
- Modify: `apps/runtime-agent/src/commands/executor.rs`（`reusable_provider_session` 语义不变,身份用 metadata 值）

**Interfaces:**
- Consumes: Task 3 注入的 `metadata["provider_session_id"]` 与 `metadata["revision_root_task_id"]`。
- Produces: runtime 用 control-plane 给定的 session id 续接；缺失则新建。

- [ ] **Step 1: 确认 `provider_session_id` 入口**

`payload.rs` 的 `RuntimeSessionCommandPayload` 已有 `provider_session_id`（`SessionPolicy.provider_session_id`,从 `session_policy` 读）。Task 3 注入的是 **metadata** 级别——需让 runtime 优先用 metadata 的值。在 `payload.rs` 构造处:

```rust
// Session identity is decided by the control plane (Task 3). If it passed a
// provider_session_id, use it; otherwise the runtime creates a new session.
let provider_session_id = metadata_string(&self.metadata, "provider_session_id")
    .or_else(|| non_empty_session_id(&self.session_policy.provider_session_id));
```

> 执行者按既有 `payload.rs` 构造方式集成;关键是 metadata 的 `provider_session_id` 优先。

- [ ] **Step 2: `reusable_provider_session` 不改**

它仍只答布尔（`recoverable && mode != Ephemeral`）。会话身份已由 control-plane 注入,runtime 只决定"要不要复用这条已给定的 session"。

- [ ] **Step 3: runtime 写回 session 时记录血缘根**

runtime 把新建/复用的 session 写回 control-plane（既有写回路径）时,带上 `revision_root_task_id`（从 metadata 取）,使 `provider_sessions.project_task_root_id` 填充。

- [ ] **Step 4: Rust 测试**

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --lib
```

新增/扩展用例:metadata 带 `provider_session_id` 时,runtime 使用它；不带时按既有逻辑。

- [ ] **Step 5: Commit**

```bash
git add apps/runtime-agent/
git commit -m "$(cat <<'EOF'
fix(runtime): use control-plane-provided session id for task lineage

The runtime no longer decides which session to resume. It reads
provider_session_id from metadata (set by the control plane per Task 3); when
absent it creates a new session. The reusable_provider_session flag keeps its
boolean meaning. New sessions are written back with the lineage root so the
provider_sessions scoping holds.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: 分层门禁与真实 E2E

- [ ] **Step 1: 门禁**

```bash
corepack pnpm verify:control-plane
corepack pnpm verify:runtime-agent
```

- [ ] **Step 2: 真实 E2E —— B' 续接 B 的会话**

构造 `a→b`,让 b 执行后触发上游补做（Plan 5）产生 b'。查库:

```bash
psql "$DB" -tAc "select digital_employee_id, project_task_root_id, provider_session_id from superteam.provider_sessions where digital_employee_id='<b的员工>' ;"
```

预期:b 与 b'（同根）共用一条 session；同员工的另一任务 d 是不同的 session。

> **依赖 Plan 5**:b' 由 Plan 5 的上游补做产生。若 Plan 5 未合并,改用手动方式:对同一任务触发 `revision_needed`（同一员工返工）验证 b 与其返工任务共用 session。

- [ ] **Step 3: 清理夹具**

---

## Self-Review

**Spec coverage**

| Spec 章节 | 任务 |
|---|---|
| §4.9 会话降维到（员工, 任务） | Task 1, 2, 3, 4 |
| §4.9 键用 `revisionRootTaskID` 而非 `project_task_id` | Task 1（列名 `project_task_root_id`）+ Task 3 |
| §3 约束三:裁决新开干净上下文 | 不在本 plan（裁决路径属 Plan 4+证据地基）;本 plan 只做"干活"会话 |

**依赖**:**Task 5 Step 2 的 E2E 判据依赖 Plan 5 合并**（需 b' 由上游补做产生）。若 Plan 5 未合并,改用同员工返工路径验证,或只验证 Task 1–4 的单测。

**Type consistency**:
- `provider_sessions.project_task_root_id` 在 Task 1 定义,Task 2 查询用,Task 4 写回填。
- `metadata["provider_session_id"]`、`metadata["revision_root_task_id"]` 在 Task 3 写、Task 4 读,键名一致。
- `FindProviderSessionForTaskRoot(tenant, employee, root) → provider_session_id` 在 Task 2 定义,Task 3 调用。

**Placeholder scan**:Task 2/3 的测试标注"参照既有模式"（repository 测试 fixture 较重）,其余步骤有完整代码块。

**已知风险**:
- runtime 写回 session 时填 `project_task_root_id` 需要改写回路径的 payload（Task 4 Step 3）。若写回契约不含此字段,需扩展 runtime→control-plane 的写回结构——执行者确认后补。
- 向后兼容:旧 session 行 `project_task_root_id` 为 null,`FindProviderSessionForTaskRoot` 不会命中它们——即旧员工级长会话不再被复用,新任务开新会话。这是预期的（正是要摆脱员工级长会话）,但可能影响在途任务。**合并时应确认无关键任务依赖旧长会话的上下文。**
- `taskLineageRoot` 必须查库获取:`StartProjectTaskRunRequest` 与 `GetRunPreflight` 都不带 `RevisionOfTaskID`（已核实 `run_types.go:156/172`）。Task 3 Step 1 已给出查 `project_tasks.revision_of_task_id` 的方式。若 `GetProjectTask` 签名不可用,改用 `ListProjectTasksByCoordinationJob` 取血缘。
