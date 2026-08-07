# Plan Phase Refactor — Plan 1: 拆除假闸门与散文指纹 Implementation Plan
> 复核状态：已实现（基于CHANGELOG证据）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 删除 `capability.match` 这道读取 LLM 自述的假闸门，并把熔断指纹从模型散文改为结构化字段，使派发闸门与熔断机制只依赖代码可判定的事实。

**Architecture:** 三处改动同属一个语义，且必须按序执行以保证每次提交可构建：（1）`internal/project/predispatch_gate.go` 移除 capability 判决分支（字段留到最后一个写入者消失后再删）；（2）两个能力快照构造器（`internal/app/planning_profile_adapter.go`、`internal/workflow/projectcoordination/predispatch_gate.go`）停止从 planner 输出取能力状态；（3）`internal/workflow/projectcoordination/project_store.go` 的 `revisionFailureFingerprint` 只取结构化字段。planner 的三个能力数组保留在 `planner_metadata` 里，仅供展示。

**Tech Stack:** Go 1.x + `testify/require`、sqlc、Temporal。测试用 `go test ./internal/...`。

## Global Constraints

以下取自 spec（`docs/superpowers/specs/2026-07-10-project-plan-phase-refactor-design.md`，提交 `60176ec7`）：

- **约束一：闸门只读代码可判定的事实，不读任何 LLM 自述。** 本 plan 是这条约束的第一次落地。
- 任何模型自由文本（`contract.Summary`、`employee_selection_reason`、`planner_notes`）**不得**进入闸门判定或熔断指纹。
- `external_capabilities` 字段本身不删，降级为描述性字段；本 plan 不触碰它。
- 不改 `hasCycle`：任务图永远是 DAG。
- 闸门保留且只保留有事实源的：`runtime.placement_missing`、`runtime.pinned_node_offline`、槽位、`budget.ready`、`context.ready`、`risk.approval`、skill 可安装、MCP 可达且凭证未过期、`task.accepted_plan_revision_changed`。
- 验证走仓库脚本：`corepack pnpm verify:control-plane`。不要手拼等价命令。
- 提交信息末尾附 `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`。

---

## 前置：本 spec 的五个子计划

本 plan 是第一个。其余四个各自独立可交付，**不要在本 plan 里实现它们**：

| # | 子计划 | 依赖 | 交付物 |
|---|---|---|---|
| 1 | **拆除假闸门与散文指纹**（本文） | 无 | 闸门只读事实；熔断不可被措辞绕过 |
| 2 | plan 输出契约与落库校验 | 无 | `selection_confidence`、`required_inputs`、`satisfied_by` 校验 |
| 3 | 阻塞申报 → 上游补做 + 图延展 | 2 | 新 decision + 新 activity + 迁移 054 `plan_iteration` |
| 4 | 计划级判据与人类评审两块 | 2 | 契约 + Web 评审界面 |
| 5 | 会话降维到血缘根 | 无 | `provider_sessions` + Rust `reusable_provider_session` |

Plan 1 独立可上线：它只**移除**判定逻辑，不新增任何字段或表。移除后，此前被 `capability.hard_missing` 卡死在 `waiting_human` 的任务将能正常派发——这正是 spec §7 E2E 第 7 条的判据。

---

## File Structure

| 文件 | 职责 | 动作 |
|---|---|---|
| `apps/control-plane/internal/project/predispatch_gate.go` | 闸门判定纯函数 | Task 1 删 capability 分支；Task 3 删 `HardMissing` / `Unknown` 字段 |
| `apps/control-plane/internal/project/predispatch_gate_test.go` | 闸门单测 | 改写 hard-missing 用例为「不再阻断」 |
| `apps/control-plane/internal/app/planning_profile_adapter.go` | 闸门快照适配器（能力+工具） | 删除能力三行，保留工具逻辑 |
| `apps/control-plane/internal/app/planning_profile_adapter_test.go` | 适配器单测 | 新增「不再从 LLM 输出取能力」用例 |
| `apps/control-plane/internal/workflow/projectcoordination/predispatch_gate.go` | 第二个快照构造器 | 删除 capability 段 |
| `apps/control-plane/internal/workflow/projectcoordination/project_store.go` | 熔断指纹 | 指纹改为结构化字段 |
| `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go` | 熔断单测 | 新增「改措辞不改指纹」用例 |
| `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go` | planner 提示词 | 删除两句关于 missing_capabilities 的指令 |

---

### Task 1: 闸门不再因 capability 阻断

**Files:**
- Modify: `apps/control-plane/internal/project/predispatch_gate.go:311-317`（判决分支）
- Test: `apps/control-plane/internal/project/predispatch_gate_test.go:213-235`

**Interfaces:**
- Consumes: 无（本 plan 起点）
- Produces: `EvaluatePreDispatchGate(input PreDispatchGateInput, snapshot PreDispatchGateSnapshot, now time.Time) PreDispatchGateEvaluation` 签名不变，但不再产出 `capability.match` check、不再产出 `capability.hard_missing` blocker、不再因能力将状态置为 `replan_required`。`PreDispatchCapabilitySnapshot` 此刻**形状不变**（仍含 `HardMissing` / `Unknown`），收缩在 Task 3。

- [ ] **Step 1: 改写既有测试，让它表达新契约**

`predispatch_gate_test.go:213` 现有的 `TestEvaluatePreDispatchGateRequiresReplanForHardMissingCapability` 断言的是我们要删掉的行为。整体替换为：

```go
func TestEvaluatePreDispatchGateIgnoresPlannerCapabilityClaims(t *testing.T) {
	now := time.Date(2026, 6, 21, 9, 34, 0, 0, time.UTC)
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	snapshot := readyPreDispatchGateSnapshot(projectID, taskID, employeeID)
	// The planner used to be able to kill a task by naming a capability it
	// invented. Capability state is no longer a gate input at all.
	snapshot.Capabilities = PreDispatchCapabilitySnapshot{
		Required: []string{"database.write"},
	}

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID:          projectID,
		ProjectTaskID:      taskID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
	}, snapshot, now)

	require.Equal(t, PreDispatchGateStatusPassed, result.Status)
	require.Empty(t, result.Blockers)
	for _, check := range result.Checks {
		require.NotEqual(t, "capability.match", check.Key)
	}
}
```

- [ ] **Step 2: 运行测试，确认它失败**

```bash
go test ./internal/project/ -run TestEvaluatePreDispatchGateIgnoresPlannerCapabilityClaims -v
```

预期：编译通过但断言失败 —— `HardMissing` 字段仍存在（为空），因此不会进入 blocker 分支；但 `capability.match` 这个 check **仍会被添加**（走 `else` 分支），断言 `NotEqual("capability.match")` 失败。

- [ ] **Step 3: 删除判决分支**

`predispatch_gate.go:311-317`，整块删除：

```go
	if len(snapshot.Capabilities.HardMissing) > 0 {
		addCheck("capability.match", "failed", map[string]any{"hard_missing": append([]string(nil), snapshot.Capabilities.HardMissing...)})
		addBlocker("capability.hard_missing", PreDispatchGateStatusReplanRequired, "hard", false, map[string]any{"hard_missing": append([]string(nil), snapshot.Capabilities.HardMissing...)})
		setStatus(PreDispatchGateStatusReplanRequired)
	} else {
		addCheck("capability.match", "passed", map[string]any{"required": append([]string(nil), snapshot.Capabilities.Required...), "matched": append([]string(nil), snapshot.Capabilities.Matched...)})
	}
```

不留替代 check。能力不是闸门关心的事。

- [ ] **Step 4: 运行测试，确认它通过**

```bash
go test ./internal/project/ -run TestEvaluatePreDispatchGateIgnoresPlannerCapabilityClaims -v
```

预期：PASS。`HardMissing` / `Unknown` 字段此刻仍在结构体上（仍被另两个构造器赋值），只是无人据此判决。字段的删除在 Task 3，那时最后一个写入者才消失——这样每次提交都能构建。

- [ ] **Step 5: 全量构建与测试**

```bash
go build ./internal/... && go test ./internal/project/
```

预期：全部 PASS。

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/project/predispatch_gate.go apps/control-plane/internal/project/predispatch_gate_test.go
git commit -m "$(cat <<'EOF'
refactor(gate): stop letting the planner veto its own dispatch

`capability.match` never matched anything. It read HardMissing straight out of
the planner's JSON, so a model that named an invented capability killed the task
permanently — after a human had already approved that plan — while a model that
wrote an empty array walked through regardless of what the employee could do.

Capability state stays on the snapshot for display and audit. It is no longer a
gate input.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: 适配器不再从 planner 输出取能力

**Files:**
- Modify: `apps/control-plane/internal/app/planning_profile_adapter.go:314-322`
- Test: `apps/control-plane/internal/app/planning_profile_adapter_test.go`（新增用例）

**Interfaces:**
- Consumes: Task 1 的 `PreDispatchCapabilitySnapshot{ Required, Matched }`。
- Produces: `GetEmployeeCapabilitySnapshot(ctx, tenantID, employeeID uuid.UUID, task project.ProjectTask) (project.PreDispatchCapabilitySnapshot, project.PreDispatchToolSnapshot, error)` 签名不变；能力部分恒返回零值，工具部分行为不变（仍查 `ListEffectiveMCPServers`）。

- [ ] **Step 1: 写失败测试**

追加到 `planning_profile_adapter_test.go` 末尾：

```go
func TestPreDispatchGateAdapterDoesNotDeriveCapabilitiesFromPlannerOutput(t *testing.T) {
	adapter := preDispatchGateAdapter{}
	task := project.ProjectTask{
		// The planner is free to write anything into this map; none of it may
		// reach the gate.
		InputRequirements: map[string]any{
			"required_capabilities": []any{"database.write"},
			"missing_capabilities":  []any{"codebase.analysis"},
			"matched_capabilities":  []any{"bash_execution"},
		},
	}

	capabilitySnapshot, _, err := adapter.GetEmployeeCapabilitySnapshot(
		context.Background(), uuid.New(), uuid.New(), task,
	)

	require.NoError(t, err)
	require.Empty(t, capabilitySnapshot.Required)
	require.Empty(t, capabilitySnapshot.Matched)
}
```

- [ ] **Step 2: 运行测试，确认它失败**

```bash
go test ./internal/app/ -run TestPreDispatchGateAdapterDoesNotDeriveCapabilitiesFromPlannerOutput -v
```

预期：FAIL —— `require.Empty(capabilitySnapshot.Required)` 失败，因为 `planning_profile_adapter.go:315` 仍从 `task.InputRequirements["required_capabilities"]` 取到 `["database.write"]`。

- [ ] **Step 3: 删除能力三行**

`planning_profile_adapter.go:314-322`，替换为：

```go
func (a preDispatchGateAdapter) GetEmployeeCapabilitySnapshot(ctx context.Context, tenantID, employeeID uuid.UUID, task project.ProjectTask) (project.PreDispatchCapabilitySnapshot, project.PreDispatchToolSnapshot, error) {
	// Capability keys are free text with no registry and no runtime effect, and
	// the only place they exist is the planner's own output. Deriving a gate
	// input from them would be the model grading itself.
	capabilitySnapshot := project.PreDispatchCapabilitySnapshot{}

	requiredTools := gateStringList(task.InputRequirements["tool_requirements"])
```

后续 `requiredTools` 起的工具逻辑（原 `:324` 起）一行不动——它查的是 `ListEffectiveMCPServers` 的真实状态。

- [ ] **Step 4: 运行测试，确认它通过**

```bash
go test ./internal/app/ -run TestPreDispatchGateAdapterDoesNotDeriveCapabilitiesFromPlannerOutput -v
```

预期：PASS。

- [ ] **Step 5: 确认工具闸门未被误伤**

```bash
go test ./internal/app/ -run 'TestPreDispatchGateAdapter' -v
```

预期：全部 PASS，含 `TestPreDispatchGateAdapterReportsMissingMCPBinding` 与 `TestPreDispatchGateAdapterMarksRequiredToolsRetryableOnCapabilityError`。

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/app/planning_profile_adapter.go apps/control-plane/internal/app/planning_profile_adapter_test.go
git commit -m "$(cat <<'EOF'
refactor(gate): drop capability derivation from planner output

GetEmployeeCapabilitySnapshot took an employeeID and never used it: all three
capability arrays came from task.InputRequirements, which only the planner
writes. The tool snapshot beside it does the opposite — it queries
ListEffectiveMCPServers for real state — and is left untouched as the model for
what a gate input should be.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: 第二个快照构造器停止读能力

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/predispatch_gate.go:744-759`

**Interfaces:**
- Consumes: Task 1 的 `PreDispatchCapabilitySnapshot{ Required, Matched }`。
- Produces: `applyGateTaskMetadata(snapshot *project.PreDispatchGateSnapshot, task project.ProjectTask)` 签名不变；不再触碰 `snapshot.Capabilities`。

- [ ] **Step 1: 定位待删段**

```bash
sed -n 744,760p apps/control-plane/internal/workflow/projectcoordination/predispatch_gate.go
```

预期输出包含 `snapshot.Capabilities.Required = firstNonEmptyStrings(...)`、`snapshot.Capabilities.Matched = ...`、`snapshot.Capabilities.HardMissing = ...`、`snapshot.Capabilities.Unknown = stringsFromAny(inputRequirements["unknown_capabilities"])`。

`unknown_capabilities` 这个键**全仓无人写入**——它恒为空。

- [ ] **Step 2: 删除整段**

删除 `snapshot.Capabilities.*` 的全部四处赋值。保留同函数中 `snapshot.Employee.ProfileSnapshotHash`、`snapshot.Tools.*`、`snapshot.Context.*` 的赋值。

删除后，若 `employeeSelection` 局部变量仅剩 `ProfileSnapshotHash` 一处使用，保留它；不要顺手清理无关代码。

- [ ] **Step 3: 收缩快照结构体**

此刻 `HardMissing` / `Unknown` 的最后一个写入者已消失，可以安全删除。`internal/project/predispatch_gate.go:86-91`：

```go
// PreDispatchCapabilitySnapshot carries the planner's capability reasoning for
// display and audit only. It is never a gate input: the keys are free text with
// no registry, no server-side validation, and no runtime effect, so a model can
// name anything here. See the 2026-07-10 plan-phase refactor spec, constraint 1.
type PreDispatchCapabilitySnapshot struct {
	Required []string
	Matched  []string
}
```

- [ ] **Step 4: 编译**

```bash
go build ./internal/...
```

预期：成功。若报错，说明还有第三处赋值——`grep -rn "HardMissing" apps/control-plane/internal/ | grep -v _test` 定位它。

- [ ] **Step 5: 跑整个 control-plane 测试**

```bash
go test ./internal/...
```

预期：全部 PASS。若 `projectcoordination` 包内有测试断言 `Capabilities.HardMissing`，改为断言 blocker 中不含 `capability.hard_missing`。

- [ ] **Step 6: 确认死字段已随之消失**

```bash
grep -rn "HardMissing\|unknown_capabilities" apps/control-plane/internal/ | grep -v _test
```

预期：无输出。

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/predispatch_gate.go apps/control-plane/internal/project/predispatch_gate.go
git commit -m "$(cat <<'EOF'
refactor(gate): remove the second capability snapshot builder

applyGateTaskMetadata duplicated the adapter's logic and fell back to
planner_metadata.employee_selection, which is where the model's own
missing_capabilities array actually lands. It also populated
Capabilities.Unknown from a key nothing in the repository ever writes.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: 熔断指纹不再吃模型散文

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go:3043-3050`
- Test: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`（新增用例）

**Interfaces:**
- Consumes: 无
- Produces: `revisionFailureFingerprint(contract project.TaskResultContract) string` 签名不变，语义收紧为「只取结构化字段」。Plan 3 的图延展熔断依赖此函数。

- [ ] **Step 1: 写失败测试**

追加到 `project_store_test.go`：

```go
func TestRevisionFailureFingerprintIgnoresModelProse(t *testing.T) {
	changes := []string{"add load test report", "attach baseline metrics"}

	first := revisionFailureFingerprint(project.TaskResultContract{
		Status:  project.TaskResultStatusRevisionNeeded,
		Summary: "I could not finish because the report was missing.",
		RevisionRequest: &project.TaskResultRevisionRequest{
			Reason:           "missing report",
			RequestedChanges: changes,
		},
	})

	// Same structural failure, different prose. The circuit breaker must not be
	// defeated by rewording.
	second := revisionFailureFingerprint(project.TaskResultContract{
		Status:  project.TaskResultStatusRevisionNeeded,
		Summary: "Unfortunately I was unable to complete this task at all.",
		RevisionRequest: &project.TaskResultRevisionRequest{
			Reason:           "the report is nowhere to be found",
			RequestedChanges: changes,
		},
	})

	require.Equal(t, first, second)
}

func TestRevisionFailureFingerprintSeparatesDifferentRequestedChanges(t *testing.T) {
	base := project.TaskResultContract{
		Status:          project.TaskResultStatusRevisionNeeded,
		RevisionRequest: &project.TaskResultRevisionRequest{RequestedChanges: []string{"add load test report"}},
	}
	other := project.TaskResultContract{
		Status:          project.TaskResultStatusRevisionNeeded,
		RevisionRequest: &project.TaskResultRevisionRequest{RequestedChanges: []string{"add baseline metrics"}},
	}

	require.NotEqual(t, revisionFailureFingerprint(base), revisionFailureFingerprint(other))
}

func TestRevisionFailureFingerprintIsOrderInsensitive(t *testing.T) {
	a := project.TaskResultContract{
		Status:          project.TaskResultStatusRevisionNeeded,
		RevisionRequest: &project.TaskResultRevisionRequest{RequestedChanges: []string{"b", "a"}},
	}
	b := project.TaskResultContract{
		Status:          project.TaskResultStatusRevisionNeeded,
		RevisionRequest: &project.TaskResultRevisionRequest{RequestedChanges: []string{"a", "b"}},
	}

	require.Equal(t, revisionFailureFingerprint(a), revisionFailureFingerprint(b))
}
```

- [ ] **Step 2: 运行测试，确认它失败**

```bash
go test ./internal/workflow/projectcoordination/ -run TestRevisionFailureFingerprint -v
```

预期：`TestRevisionFailureFingerprintIgnoresModelProse` FAIL（现实现把 `Summary` 与 `Reason` 都拼进指纹）；`TestRevisionFailureFingerprintIsOrderInsensitive` FAIL（现实现按原顺序拼接）。

- [ ] **Step 3: 重写指纹函数**

`project_store.go:3043`，整体替换：

```go
// revisionFailureFingerprint identifies a failure by its structured shape only.
//
// It deliberately excludes contract.Summary and RevisionRequest.Reason: both are
// free text written by the model. Feeding them in let a reworded failure present
// as a fresh one, silently defeating repeatedRevisionFailure. See the 2026-07-10
// plan-phase refactor spec, constraint 1.
func revisionFailureFingerprint(contract project.TaskResultContract) string {
	parts := []string{string(contract.Status)}
	if contract.RevisionRequest != nil {
		changes := append([]string(nil), contract.RevisionRequest.RequestedChanges...)
		sort.Strings(changes)
		parts = append(parts, changes...)
	}
	if contract.Blocker != nil {
		inputs := append([]string(nil), contract.Blocker.MissingInputs...)
		sort.Strings(inputs)
		parts = append(parts, inputs...)
	}
	return strings.Join(parts, "\n")
}
```

**注意** `contract.Blocker.MissingInputs` 尚不存在——它由 Plan 3 引入。本 plan **不要**加这个分支。本 plan 的函数体到 `RevisionRequest` 为止：

```go
func revisionFailureFingerprint(contract project.TaskResultContract) string {
	parts := []string{string(contract.Status)}
	if contract.RevisionRequest != nil {
		changes := append([]string(nil), contract.RevisionRequest.RequestedChanges...)
		sort.Strings(changes)
		parts = append(parts, changes...)
	}
	return strings.Join(parts, "\n")
}
```

若 `sort` 尚未 import，加入 `"sort"`（该文件已 import `"sort"`，见 `repeatedRevisionFailure` 中的 `sort.SliceStable`）。

- [ ] **Step 4: 运行测试，确认通过**

```bash
go test ./internal/workflow/projectcoordination/ -run TestRevisionFailureFingerprint -v
```

预期：三个用例全部 PASS。

- [ ] **Step 5: 确认熔断整体行为未回归**

```bash
go test ./internal/workflow/projectcoordination/
```

预期：PASS。若既有测试依赖 `Summary` 参与指纹，改测试而非改实现——那种依赖正是本 task 要消除的。

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/project_store.go apps/control-plane/internal/workflow/projectcoordination/project_store_test.go
git commit -m "$(cat <<'EOF'
fix(coordination): fingerprint failures by structure, not by prose

revisionFailureFingerprint hashed contract.Summary, which the model writes. Two
identical failures worded differently produced different fingerprints, so
repeatedRevisionFailure never fired and the circuit breaker was decorative.

Fingerprints now cover status plus sorted RequestedChanges. Order-insensitive so
a reshuffled list is the same failure.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: 提示词不再要求模型自证能力

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go:274,276,279`

**Interfaces:**
- Consumes: 无
- Produces: planner 仍输出 `required_capabilities` / `matched_capabilities` / `missing_capabilities`（落 `planner_metadata.employee_selection`，供展示），但提示词不再把它们与 `requires_human_approval` 绑定。Plan 2 会用 `selection_confidence` 取代 `selection_score`。

- [ ] **Step 1: 读现状**

```bash
sed -n 274,280p apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go
```

预期看到三句：`:274` 的字段清单、`:276` 的 "copy the required, matched, and missing capability arrays"、`:279` 的 "A task with missing_capabilities must set requires_human_approval or make the whole route requires_human_review true."

- [ ] **Step 2: 删除 `:279` 那一句**

它把模型的自证绑上了人类审批开关。删除整行字符串。

- [ ] **Step 3: 改写 `:276`**

原文要求模型 "copy" 三个数组——但没有任何来源可供 copy，模型只能凭空合成。改为：

```go
		"For every task, choose selected_employee_id by comparing planning_profile facts and explain the choice in employee_selection_reason. The capability arrays are advisory annotations shown to a human reviewer; they never gate dispatch.",
```

`:274` 的字段清单保持不变——字段仍要产出，只是不再有判决作用。

- [ ] **Step 4: 跑 planner 单测**

```bash
go test ./internal/workflow/projectcoordination/ -run 'Planner' -v
```

预期：PASS。若有测试断言提示词包含被删的句子，一并更新。

- [ ] **Step 5: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go
git commit -m "$(cat <<'EOF'
refactor(planner): stop asking the model to certify its own capability gaps

The prompt told the model to "copy" required/matched/missing capability arrays,
but nothing existed to copy from — it synthesised them. It then made
missing_capabilities force requires_human_approval, wiring a hallucinated field
into a human gate.

The arrays remain as advisory annotations for the reviewer.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: 分层门禁与真实 E2E

**Files:** 无改动。本 task 是验收。

**Interfaces:**
- Consumes: Task 1–5 的全部改动。
- Produces: 一份可复述的验收结论。

- [ ] **Step 1: 跑仓库门禁**

```bash
corepack pnpm verify:control-plane
```

预期：PASS。

- [ ] **Step 2: 确认无残留**

```bash
grep -rn "capability.hard_missing\|capability.match" apps/control-plane/internal/ | grep -v _test
```

预期：无输出。

- [ ] **Step 3: 重启服务加载新代码**

```bash
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh status
```

预期：`control-plane: running ... healthy`。

- [ ] **Step 4: 真实 E2E —— 一个曾被卡死的计划现在能派发**

这是 spec §7 E2E 第 7 条。用 `admin/admin` 登录，建一个 claude-code 员工（`capability_bindings.external_capabilities` 留空，让 planner 必然报 missing），建项目、提需求、批准计划。

```bash
JAR=$(mktemp)
curl -fsS -c "$JAR" -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' \
  http://127.0.0.1:8081/api/auth/login >/dev/null
```

派发后查库：

```bash
DB=$(grep -m1 'url:' apps/control-plane/config/config.yaml | sed 's/.*url: *//;s/"//g')
psql "$DB" -tAc "select status from superteam.project_tasks order by created_at desc limit 1;"
```

预期：`running` 或 `completed`，**不是** `waiting_human`。

同时确认 planner 确实报了缺失能力（证明闸门是被拆掉而非碰巧没触发）：

```bash
psql "$DB" -tAc "select planner_metadata->'employee_selection'->'missing_capabilities' from superteam.project_tasks order by created_at desc limit 1;"
```

预期：非空数组。

- [ ] **Step 5: 确认闸门记录里不再有该 check**

```bash
psql "$DB" -tAc "select jsonb_path_query_array(checks, '\$[*].key') from superteam.project_task_dispatch_gate_results order by checked_at desc limit 1;"
```

预期：数组中不含 `capability.match`。

- [ ] **Step 6: 清理一次性夹具**

归档测试项目、停用测试员工。不要在共享 dev 库留活跃垃圾数据。

---

## Self-Review

**Spec coverage（本 plan 认领的范围）**

| Spec 章节 | 任务 |
|---|---|
| §1.1 闸门读 LLM 自述 | Task 1, 2, 3 |
| §1.5 死字段 `Unknown`、两个重复构造器 | Task 3 |
| §5 删除清单前 4 行 + 提示词 2 行 | Task 1, 2, 3, 5 |
| §4.8 熔断指纹（结构化） | Task 4（`blocked` 分支留给 Plan 3） |
| §7 E2E 第 7 条 | Task 6 |

**本 plan 明确不认领**（各有归属，见「前置」表）：`selection_confidence`（Plan 2）、`required_inputs` 校验（Plan 2）、`missing_inputs` 与上游补做（Plan 3）、`plan_iteration` 列与迁移 054（Plan 3）、计划级判据与评审 UI（Plan 4）、会话降维（Plan 5）。`§1.5` 的 `coordination_policy` 死字段由 Plan 3 复活；四个死事件枚举由 Plan 3 处理。

**Type consistency**

- `PreDispatchCapabilitySnapshot` 在 Task 1 收缩为 `{ Required, Matched }`；Task 2、Task 3 均按此形状消费，无 `HardMissing` / `Unknown` 残留。
- `revisionFailureFingerprint(contract project.TaskResultContract) string` 签名在 Task 4 前后一致；`Blocker.MissingInputs` 分支明确标注为 Plan 3 引入，本 plan 不写。
- `GetEmployeeCapabilitySnapshot` 与 `applyGateTaskMetadata` 签名均未变，调用方无需改动。

**Placeholder scan**：无 TBD / TODO / "similar to Task N"。每个改码步骤都给出完整代码块与预期输出。

**已知风险**

Task 1–3 必须按序执行。结构体字段的删除被刻意推迟到 Task 3，即最后一个写入者消失之后——这样**每一次提交都能构建**，三处「读 LLM 自述」的删除仍各自可被审阅者独立否决。

Task 6 Step 4 会在共享远端 dev 库（`config.yaml` 指向的 TOS/Postgres）上创建真实项目并消耗真实 token。Step 6 的清理不是可选项。
