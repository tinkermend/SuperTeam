# Plan Phase Refactor — Plan 8: 删除 tool 死闸门 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 删除那道结构上不可能失败的 tool 闸门（`tool.binding` / `tool.authorization` / `tool.available`），以及它带出来的一整串死代码。MCP 可用性由 provider 负责,控制平面这层不做检查。

**Architecture:** 纯减法。删 `predispatch_gate.go` 的 tool 检查分支与 `PreDispatchToolSnapshot`；删 adapter 里整段 `requiredTools` 逻辑及随之成为孤儿的 `capabilities` 字段/接口/注入；删 workflow 侧的 tool metadata 读取与合并函数。

**Tech Stack:** Go 1.x + `testify/require`。门禁 `corepack pnpm verify:control-plane`。

## Global Constraints

取自 spec `docs/superpowers/specs/2026-07-10-project-plan-phase-refactor-design.md`（§1.7,勘误后）:

- **MCP 可用性不由控制平面检查。** runtime 派发载荷带 `mcp_servers`,`mcp_config.rs:102` 按 provider 写原生配置（`codex.toml`/`claude.mcp.json`/`opencode.json`）,provider 自己的加载器负责挂载。
- **那道闸门是死代码。** 它读 `input_requirements["tool_requirements"]`,服务端从未写过该键（63 个任务 0 命中）,`requiredTools` 恒为空,闸门 early return。三个 check 结构上不可能失败。
- **`capability.Service.ListEffectiveMCPServers` 本身保留**——它还服务于 `/effective-mcp-servers` API 端点（`api/server.go`）。删的只是 adapter 里那次只服务于死闸门的调用。
- 提交信息末尾附 `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`。

---

## 前置:删除范围（已核实）

| 目标 | 位置 | 状态 |
|---|---|---|
| `tool.*` 检查分支 | `internal/project/predispatch_gate.go:313-328` | 删除 |
| `PreDispatchToolSnapshot` 结构 + `Snapshot.Tools` 字段 | `:95`,`:57` | 删除 |
| adapter `requiredTools` 段 + `effectiveMCPServerNames` | `internal/app/planning_profile_adapter.go:320-352`,`:494` | 删除 |
| adapter `capabilities` 字段 + `gateCapabilityReader` 接口 | `:53`,接口定义处 | 删除（仅 tool 段用） |
| `app.go:508` 的 `capabilities` 注入 | `internal/app/app.go` | 删除 |
| workflow 侧 tool metadata 读取 + `mergePreDispatchToolSnapshot` | `internal/workflow/.../predispatch_gate.go:743-748,612` | 删除 |

---

## File Structure

| 文件 | 动作 |
|---|---|
| `apps/control-plane/internal/project/predispatch_gate.go` | 删 tool 检查、`PreDispatchToolSnapshot`、`Snapshot.Tools` |
| `apps/control-plane/internal/project/predispatch_gate_test.go` | 删/改 tool 相关用例 |
| `apps/control-plane/internal/app/planning_profile_adapter.go` | 删 `requiredTools` 段、`capabilities` 字段、`effectiveMCPServerNames`、`gateCapabilityReader` |
| `apps/control-plane/internal/app/planning_profile_adapter_test.go` | 删 tool 相关用例 |
| `apps/control-plane/internal/app/app.go` | 删 `gateAdapter` 的 `capabilities` 注入 |
| `apps/control-plane/internal/workflow/projectcoordination/predispatch_gate.go` | 删 tool metadata 读取与 `mergePreDispatchToolSnapshot` |

---

### Task 1: 删 `predispatch_gate.go` 的 tool 检查与快照结构

**Files:**
- Modify: `apps/control-plane/internal/project/predispatch_gate.go`
- Test: `apps/control-plane/internal/project/predispatch_gate_test.go`

**Interfaces:**
- Consumes: 无。
- Produces: `EvaluatePreDispatchGate` 不再产出 `tool.*` check 与 blocker；`PreDispatchGateSnapshot` 不再有 `Tools` 字段。

- [ ] **Step 1: 改写测试**

删除（或改写）断言 tool 行为的用例。先用 `grep` 定位:

```bash
grep -n "tool.binding\|tool.authorization\|tool.available\|Tools:" apps/control-plane/internal/project/predispatch_gate_test.go
```

把断言 `tool.*` check 存在/阻断的用例,改为断言它们**不再出现**:

```go
func TestEvaluatePreDispatchGateHasNoToolChecks(t *testing.T) {
	now := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	snapshot := readyPreDispatchGateSnapshot(projectID, taskID, employeeID)

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID: projectID, ProjectTaskID: taskID, SelectedEmployeeID: employeeID,
		AttemptNo: 1, DispatchReason: DispatchReasonRootReady,
	}, snapshot, now)

	for _, check := range result.Checks {
		require.NotContains(t, []string{"tool.binding", "tool.authorization", "tool.available"}, check.Key)
	}
	require.Empty(t, result.Blockers)
}
```

- [ ] **Step 2: 删 `:313-328` 的 tool 检查分支**

整段删除:

```go
	if len(snapshot.Tools.ExpiredAuthorizations) > 0 {
		...
	} else if len(snapshot.Tools.MissingBindings) > 0 {
		...
	} else if len(snapshot.Tools.RetryableUnavailable) > 0 {
		...
	} else {
		addCheck("tool.available", "passed", nil)
	}
```

- [ ] **Step 3: 删 `PreDispatchToolSnapshot` 与 `Snapshot.Tools`**

`:95` 的 `type PreDispatchToolSnapshot struct {...}` 整体删除。`PreDispatchGateSnapshot`（`:50` 附近）中的 `Tools PreDispatchToolSnapshot` 字段删除。

- [ ] **Step 4: 编译（预期失败——adapter 仍引用 `Tools`）**

```bash
go build ./internal/...
```

预期:编译失败,因 `planning_profile_adapter.go` 仍返回 `PreDispatchToolSnapshot`。记录失败点,Task 2 修。

- [ ] **Step 5: Commit（含测试）**

```bash
git add apps/control-plane/internal/project/
git commit -m "$(cat <<'EOF'
refactor(gate): remove the tool.* checks from the dispatch gate

The gate read input_requirements["tool_requirements"], which the server never
writes (0 of 63 tasks carry it), so requiredTools was always empty and
tool.binding / tool.authorization / tool.available could never fail. MCP
availability is the provider's responsibility, not the control plane's.

PreDispatchToolSnapshot is removed here; the adapter still returns a zero value of
it and is fixed in the next commit.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

> **若执行者要求每个提交可构建**:把 Task 1 与 Task 2 合并为一次提交（Task 2 删 adapter 引用后即可编译）。

---

### Task 2: 删 adapter 的 `requiredTools` 段及孤儿依赖

**Files:**
- Modify: `apps/control-plane/internal/app/planning_profile_adapter.go`
- Modify: `apps/control-plane/internal/app/app.go`
- Test: `apps/control-plane/internal/app/planning_profile_adapter_test.go`

**Interfaces:**
- Consumes: Task 1。
- Produces: `GetEmployeeCapabilitySnapshot` 返回签名变为 `(project.PreDispatchCapabilitySnapshot, error)`（无 toolSnapshot）；adapter 不再持 `capabilities` 字段。

- [ ] **Step 1: 改 `GetEmployeeCapabilitySnapshot` 签名与实现**

`planning_profile_adapter.go:314` 起整段替换:

```go
func (a preDispatchGateAdapter) GetEmployeeCapabilitySnapshot(ctx context.Context, tenantID, employeeID uuid.UUID, task project.ProjectTask) (project.PreDispatchCapabilitySnapshot, error) {
	// Capability and tool state are advisory only (see §1.6/§1.7). The gate no
	// longer has a tool snapshot, and MCP availability is the provider's concern.
	return project.PreDispatchCapabilitySnapshot{}, nil
}
```

- [ ] **Step 2: 删 `effectiveMCPServerNames`（`:494`）、`capabilities` 字段（`:53`）、`gateCapabilityReader` 接口**

逐个 `grep` 确认无其他引用后删除:

```bash
grep -n "effectiveMCPServerNames\|gateCapabilityReader\|a.capabilities" apps/control-plane/internal/app/planning_profile_adapter.go
```

`app.go:508` 的 `gateAdapter := preDispatchGateAdapter{ capabilities: ... }` 去掉 `capabilities` 字段（或整个构造若无他字段）。同时清理 `app.go` 里 `capability` 包的 import（若不再用）。

- [ ] **Step 3: 删测试用例**

```bash
grep -n "MissingMCP\|MarksRequiredTools\|tool_requirements\|ToolSnapshot" apps/control-plane/internal/app/planning_profile_adapter_test.go
```

删除断言 tool 行为的用例（如 `TestPreDispatchGateAdapterReportsMissingMCPBinding`、`TestPreDispatchGateAdapterMarksRequiredToolsRetryableOnCapabilityError`）。

- [ ] **Step 4: 编译并跑包**

```bash
go build ./internal/... && go test ./internal/app/ ./internal/project/
```

预期:PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/control-plane/internal/app/ apps/control-plane/internal/project/predispatch_gate.go
git commit -m "$(cat <<'EOF'
refactor(gate): drop the adapter's tool snapshot and its orphaned dependencies

GetEmployeeCapabilitySnapshot returns an empty capability snapshot. With the tool
gate gone, the capabilities field, the gateCapabilityReader interface, the
effectiveMCPServerNames helper, and the app.go injection all become dead and are
removed. capability.Service.ListEffectiveMCPServers stays — it serves
/effective-mcp-servers.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: 删 workflow 侧 tool metadata 读取

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/predispatch_gate.go`

**Interfaces:**
- Consumes: Task 1、2。
- Produces: workflow 侧不再填 `snapshot.Tools`。

- [ ] **Step 1: 删读取与合并函数**

`:743-748` 的 `snapshot.Tools.*` 赋值删除。`:240` 的 `snapshot.Tools = mergePreDispatchToolSnapshot(...)` 删除。`:612` 的 `mergePreDispatchToolSnapshot` 函数整体删除。

```bash
grep -n "snapshot.Tools\|mergePreDispatchToolSnapshot" apps/control-plane/internal/workflow/projectcoordination/predispatch_gate.go
```

预期删除后无残留。

- [ ] **Step 2: 编译并跑包**

```bash
go build ./internal/... && go test ./internal/workflow/projectcoordination/
```

预期:PASS。

- [ ] **Step 3: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/
git commit -m "$(cat <<'EOF'
refactor(gate): remove the workflow-side tool snapshot reads

applyGateTaskMetadata no longer populates snapshot.Tools from
missing_tool_bindings / expired_tool_authorizations / retryable_unavailable_tools
(keys nothing writes), and mergePreDispatchToolSnapshot is removed.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: 分层门禁与残留确认

**Files:** 无改动。

- [ ] **Step 1: 门禁**

```bash
corepack pnpm verify:control-plane
```

- [ ] **Step 2: 残留确认**

```bash
grep -rn "PreDispatchToolSnapshot\|tool.binding\|tool.authorization\|tool.available\|effectiveMCPServerNames\|mergePreDispatchToolSnapshot" apps/control-plane/internal | grep -v _test
```

预期:无输出。

```bash
grep -rn "ListEffectiveMCPServers" apps/control-plane/internal/capability/ apps/control-plane/internal/api/
```

预期:`capability.Service` 与 `/effective-mcp-servers` 端点仍在（保留项）。

- [ ] **Step 3: 真实 E2E（轻量）**

```bash
scripts/dev-services.sh restart control-plane
```

派发一个会用到 MCP 的任务（或任意任务）,确认:

```bash
DB=$(grep -m1 'url:' apps/control-plane/config/config.yaml | sed 's/.*url: *//;s/"//g')
psql "$DB" -tAc "select jsonb_path_query_array(checks, '\$[*].key') from superteam.project_task_dispatch_gate_results order by checked_at desc limit 1;"
```

预期:checks 数组中**不含** `tool.binding`/`tool.authorization`/`tool.available`,任务仍正常派发（其他真实闸门放行）。

- [ ] **Step 4: 清理夹具**

---

## Self-Review

**Spec coverage**

| Spec 章节 | 任务 |
|---|---|
| §1.7 删 tool 死闸门 | Task 1, 2, 3 |
| §1.7 `ListEffectiveMCPServers` 保留（服务 `/effective-mcp-servers`） | Task 4 Step 2 验证 |

**独立性**:本 plan 与 Plan 4/5/6 无依赖,可优先做（清债）。Plan 5 的 E2E 会更干净,因为 `tool.available` 这个永不失败的 check 不再干扰。

**Type consistency**:
- `GetEmployeeCapabilitySnapshot` 签名变更（去掉 tool 返回值）是**破坏性**的——所有调用方（`app.go` 装配、workflow 侧）都要同步。Task 2 Step 2 已覆盖 `app.go`;workflow 侧若直接读 tool 返回值,Task 3 覆盖。
- 删除顺序:Task 1 删消费方（gate 检查 + 结构）→ Task 2 删生产方（adapter）→ Task 3 删 workflow 残留。Task 1 单独提交不可构建,但与 Task 2 合并即可；已在 Task 1 Step 5 注明。

**Placeholder scan**:无 TBD/TODO。每个删除点都有精确行号 + grep 验证命令。

**已知风险**:
- `GetEmployeeCapabilitySnapshot` 签名变更是破坏性的。若 workflow 层在别处（非 `applyGateTaskMetadata`）调用它并读 tool 返回值,会编译失败——Task 3 处理 `applyGateTaskMetadata`;执行者跑 `go build` 时会暴露任何遗漏点。
- 删 `gateCapabilityReader` 接口后,`app.go` 可能不再 import `capability` 包,需清理 import。
- 若 Plan 2b 的测试里仍有引用 `Tools` 字段的旧断言（Plan 1 留下的过渡）,一并删除。
