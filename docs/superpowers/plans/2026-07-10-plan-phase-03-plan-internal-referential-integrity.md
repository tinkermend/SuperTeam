# Plan Phase Refactor — Plan 3: 计划内部引用完整性（`produces` / `required_inputs`）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让计划自己声明「谁产出什么、谁需要什么」，并在落库前校验这份声明的内部引用完整性——每个 `required_inputs` 的 key 必须由某个**祖先**任务 `produces`，且全计划唯一。这是 §4.6(a) 那句「平台查计划求归属，C 从不评价 B」唯一可能的实现基础。

**Architecture:** 新增两个计划内作用域的字段：`produces: []string`（任务承诺产出的稳定 key）与 `input_requirements.required_inputs: []string`（任务需要的 key）。`expected_outputs` 保持自然语言不动。`graph_validation` 新增两条校验：引用完整性（祖先可达）与产出键唯一性，与既有的 `hasCycle` 属同一类。planner 的自由填充字段迁出 `input_requirements`，移入 `planner_metadata.planner_notes`。

**Tech Stack:** Go 1.x + `testify/require`、Temporal。测试 `go test ./internal/...`；门禁 `corepack pnpm verify:control-plane`。

## Global Constraints

取自 spec `docs/superpowers/specs/2026-07-10-project-plan-phase-refactor-design.md`（含 Plan 1/2 后的三处勘误）：

- **约束一：闸门只读代码可判定的事实，不读任何 LLM 自述。**
- **约束二：跨员工交接只经结果契约与证据引用，不经 transcript。** `produces` / `required_inputs` 是这条约束的载体。
- `expected_outputs` **不改**：它是自然语言，只给人看。`validateCompletedTaskResult`（`task_result_contract.go:382`）认得的三个保留名 `evidence_refs` / `artifact_refs` / `verification` 行为不变。
- 不改 `hasCycle`：任务图永远是 DAG。
- **不接通工具闸门**（spec §1.7）。`planning_profile_adapter.go:324` 读的 `input_requirements["tool_requirements"]` 在本 plan 之后仍然恒为空——这是有意的，见下方「本 plan 明确不做」。
- 提交信息末尾附 `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`。

---

## 前置：为什么 `produces` 不是第二个 capability 词表

审阅者读到「planner 自己发明一组 key，然后平台拿这些 key 做集合匹配」，第一反应应当是警觉——这正是 capability 闸门的形状。**必须先说清楚区别，否则这个 plan 就该被驳回。**

| | capability | `produces` / `required_inputs` |
|---|---|---|
| 生产者 | 人在员工配置里填 `external_capabilities` | planner，同一次调用 |
| 消费者 | planner 合成 `required_capabilities` | planner，同一次调用 |
| 两者是否同一份文档 | **否**，跨文档 | **是**，同一份计划 |
| 是否被同一个人批准 | 否 | **是**，人类批准该计划 |
| 校验对象 | 两个无注册表词表的差集 | 一份文档的**内部引用完整性** |
| 同类既有检查 | 无 | `blocked_by_keys` 必须指向真实 task_key；`hasCycle` |

`produces` 不与任何外部词表比对，因此不需要注册表。它与 `blocked_by_keys` 是同一件事：**一份计划必须自洽。**

## 前置：spec 在编写本 plan 时的一处勘误

spec §4.2 初稿写「`required_inputs[k]` 能在某个上游任务的 `expected_outputs` 中找到」。**做不到。** 库中 63 个任务的 `expected_outputs` 实际取值是自然语言句子：

```
"网络连通性检查报告，包含接口状态、连通性结果和数据包分析"
"Exit code from git status"
"最终验收结论"
```

拿 key 匹配散文，永远匹配不上。已引入 `produces` 并修正 spec §4.2 / §4.6。

## 前置：本 plan 明确不做

**不接通工具闸门。** spec §1.7 记录：`planning_profile_adapter.go:324` 读 `input_requirements["tool_requirements"]`，而服务端从未写过该键（63 个任务命中 0 次），闸门 early return，`tool.binding` / `tool.authorization` / `tool.available` 三个检查结构上不可能失败。

本 plan 会给 `input_requirements` 定义 schema，但**不会**顺手把 `tool_requirements` 塞进去。按 SuperTeam 的 MCP 架构，控制平面这层不做 MCP 可用性检查：runtime 派发载荷带 `mcp_servers`，`mcp_config.rs:102` 按 provider 写出原生配置（`codex.toml` / `claude.mcp.json` / `opencode.json`），由 provider 自己的加载器挂载。因此那道 tool 闸门是**该删的死代码**，不是该接的线——见 spec §1.7，删除工作记为 Plan 8。

---

## 依赖

**本 plan 依赖 Plan 2 已合并**：Plan 2 删除了 `planning_profile_adapter.go:315-317` 与 `workflow/.../predispatch_gate.go:744-759` 对 `input_requirements` 中能力键的读取。若这些读取仍在，本 plan 的 schema 收紧会让它们静默读到空值——虽不崩溃，但会掩盖 Plan 2 未完成的事实。

执行前确认：

```bash
grep -rn 'InputRequirements\["required_capabilities"\]\|inputRequirements\["missing_capabilities"\]' apps/control-plane/internal | grep -v _test
```

预期：无输出。有输出则先完成 Plan 2。

---

## File Structure

| 文件 | 职责 | 动作 |
|---|---|---|
| `apps/control-plane/internal/workflow/projectcoordination/planner.go` | `PlannedTask` 类型 | Task 1：新增 `Produces []string` |
| `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go` | planner 输出解析与提示词 | Task 1：解析 `produces`、`required_inputs`；自由字段迁 `planner_notes` |
| `apps/control-plane/internal/workflow/projectcoordination/graph_validation.go` | 计划校验 | Task 2：祖先可达性；Task 3：产出键唯一性 |
| `apps/control-plane/internal/workflow/projectcoordination/plan_revision_payload.go` | 计划落库载荷 | Task 4：透传 `produces`，`input_requirements` 只留 `required_inputs` |
| `apps/control-plane/internal/workflow/projectcoordination/project_store.go` | 任务落库 | Task 4：写入 schema 化的 `input_requirements` |

---

### Task 1: planner 产出 `produces` 与 `required_inputs`

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/planner.go`（`PlannedTask` 结构体，`SelectionConfidence` 附近）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go:274`（提示词）、`:370` 与 `:395`（结构体）、`:434`（字面量）、`:340`（透传）
- Test: `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner_test.go`

**Interfaces:**
- Consumes: Plan 2 的 `PlannedTask.SelectionConfidence`（本 plan 不改它）。
- Produces: `PlannedTask.Produces []string`。`PlannedTask.InputRequirements map[string]any` 保留，但语义收紧为「只有 `required_inputs` 一个键参与判定」。新函数 `plannerRequiredInputs(raw map[string]any) []string`。Task 2、Task 3、Task 4 依赖它们。

- [ ] **Step 1: 写失败测试**

追加到 `openai_compatible_planner_test.go`：

```go
func TestPlannerRequiredInputsReadsOnlyTheDefinedKey(t *testing.T) {
	raw := map[string]any{
		"required_inputs": []any{"load_test_report", "baseline_metrics"},
		// Free-form noise the planner has always emitted. It must not leak into
		// anything that decides.
		"repository": "superteam",
		"scope":      "one host",
	}

	require.Equal(t, []string{"load_test_report", "baseline_metrics"}, plannerRequiredInputs(raw))
}

func TestPlannerRequiredInputsHandlesAbsentAndMalformed(t *testing.T) {
	require.Empty(t, plannerRequiredInputs(nil))
	require.Empty(t, plannerRequiredInputs(map[string]any{}))
	require.Empty(t, plannerRequiredInputs(map[string]any{"required_inputs": "not an array"}))
	require.Equal(t, []string{"a"}, plannerRequiredInputs(map[string]any{
		"required_inputs": []any{"a", "", "   "},
	}), "blank entries are dropped, not preserved")
}
```

- [ ] **Step 2: 运行测试，确认它失败**

```bash
go test ./internal/workflow/projectcoordination/ -run TestPlannerRequiredInputs -v
```

预期：编译失败 —— `plannerRequiredInputs` 未定义。

- [ ] **Step 3: 实现 `plannerRequiredInputs`**

追加到 `openai_compatible_planner.go`：

```go
// plannerRequiredInputs extracts the one key of input_requirements that decides
// anything. Everything else in that map is free-form planner prose and must not
// reach a gate or a validator. See the 2026-07-10 plan-phase refactor spec §4.2.
func plannerRequiredInputs(raw map[string]any) []string {
	values, ok := raw["required_inputs"].([]any)
	if !ok {
		return nil
	}
	inputs := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			continue
		}
		if trimmed := strings.TrimSpace(text); trimmed != "" {
			inputs = append(inputs, trimmed)
		}
	}
	return inputs
}
```

- [ ] **Step 4: 运行测试，确认通过**

```bash
go test ./internal/workflow/projectcoordination/ -run TestPlannerRequiredInputs -v
```

预期：两个用例 PASS。

- [ ] **Step 5: 接入 `Produces` 字段**

`planner.go` 的 `PlannedTask` 中，在 `ExpectedOutputs` 下方新增：

```go
	// Produces are plan-scoped output keys other tasks may declare as
	// required_inputs. Unlike ExpectedOutputs (prose, for humans), these are
	// matched by the validator.
	Produces                    []string
```

`openai_compatible_planner.go:370` 的 `plannerTask` 新增 `Produces []string \`json:"produces"\``；`:395` 的 raw 结构体新增 `Produces json.RawMessage \`json:"produces"\``；`:434` 的字面量中新增 `Produces: decodePlannerStringArray(raw.Produces),`；`:340` 的透传中新增 `Produces: nonNilStrings(task.Produces),`。

`decodePlannerStringArray` 已存在（`:479`），它会把单个字符串包成一元切片——对 `produces` 是合理的容错。

- [ ] **Step 6: 提示词**

`openai_compatible_planner.go:274` 的字段清单中加入 `produces`。追加两句：

```go
		"produces is a list of short, stable, snake_case keys naming the artifacts this task hands to downstream tasks, for example load_test_report. Every key another task lists in input_requirements.required_inputs must appear in the produces of one of its ancestors.",
		"input_requirements.required_inputs lists the produces keys this task consumes from upstream. Put any other context you want to record under planner_notes instead; nothing else in input_requirements is read.",
```

- [ ] **Step 7: 编译并跑 planner 测试**

```bash
go build ./internal/... && go test ./internal/workflow/projectcoordination/ -run Planner
```

预期：PASS。既有 fixture 无 `produces` 时该字段为 `nil`，Task 2 的校验才会拒绝——本 task 不拒绝任何东西。

- [ ] **Step 8: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/
git commit -m "$(cat <<'EOF'
feat(planning): plans declare what each task produces and consumes

expected_outputs cannot serve as the handoff vocabulary: in production it holds
prose — "网络连通性检查报告，包含接口状态、连通性结果和数据包分析", "Exit code from
git status". Matching a key against a sentence never succeeds, and the whole
"the platform looks up the owner, C never judges B" design rests on that lookup.

produces is a list of plan-scoped keys. It is not a second capability vocabulary:
producer and consumer are emitted by one planner call, live in one document, and
are approved by one human. The validator checks a plan is internally consistent,
exactly as it already checks blocked_by_keys and acyclicity.

input_requirements keeps only required_inputs. The prose the planner has always
put there moves to planner_notes, where nothing reads it.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: 引用完整性校验（祖先可达）

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/graph_validation.go`
- Test: `apps/control-plane/internal/workflow/projectcoordination/graph_validation_test.go`

**Interfaces:**
- Consumes: Task 1 的 `PlannedTask.Produces` 与 `plannerRequiredInputs`。
- Produces: 新函数 `ancestorKeys(tasks []PlannedTask, key string) map[string]struct{}`（沿 `BlockedByKeys` 反向可达闭包，不含自身）。**校验加在 `ValidateRouteDecisionGraph`（`graph_validation.go:85`）内**，不是 `ValidateRouteDecisionPlan`——`hasCycle` 与 `blocked_by_keys` 的引用完整性都在那里，本校验与它们同类。`ValidateRouteDecisionPlan:26` 会先调用它，所以 LLM 路径照常覆盖。Task 3 复用 `producers` 映射。

- [ ] **Step 1: 写失败测试**

```go
func planTaskWithIO(key string, blockedBy []string, produces []string, requires []string) PlannedTask {
	return PlannedTask{
		Key:                     key,
		SelectedEmployeeID:      uuid.New(),
		EmployeeSelectionReason: "test",
		SelectionConfidence:     0.9,
		AcceptanceCriteria:      []string{"done"},
		BlockedByKeys:           blockedBy,
		Produces:                produces,
		InputRequirements:       map[string]any{"required_inputs": stringsToAny(requires)},
	}
}

func TestValidateRouteDecisionPlanAcceptsRequiredInputFromAncestor(t *testing.T) {
	plan := RouteDecisionPlan{Tasks: []PlannedTask{
		planTaskWithIO("a", nil, []string{"load_test_report"}, nil),
		planTaskWithIO("b", []string{"a"}, nil, []string{"load_test_report"}),
	}}
	snapshot := snapshotForPlan(plan)

	require.NoError(t, ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12}))
}

func TestValidateRouteDecisionPlanRejectsRequiredInputWithNoProducer(t *testing.T) {
	plan := RouteDecisionPlan{Tasks: []PlannedTask{
		planTaskWithIO("a", nil, []string{"something_else"}, nil),
		planTaskWithIO("b", []string{"a"}, nil, []string{"load_test_report"}),
	}}
	snapshot := snapshotForPlan(plan)

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12})

	require.Error(t, err)
	require.Contains(t, err.Error(), "load_test_report")
}

func TestValidateRouteDecisionPlanRejectsRequiredInputFromNonAncestor(t *testing.T) {
	// c produces the key, but c is a sibling of b, not an ancestor: nothing
	// guarantees c runs first.
	plan := RouteDecisionPlan{Tasks: []PlannedTask{
		planTaskWithIO("a", nil, nil, nil),
		planTaskWithIO("b", []string{"a"}, nil, []string{"load_test_report"}),
		planTaskWithIO("c", []string{"a"}, []string{"load_test_report"}, nil),
	}}
	snapshot := snapshotForPlan(plan)

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12})

	require.Error(t, err)
	require.Contains(t, err.Error(), "load_test_report")
}

func TestAncestorKeysWalksTransitively(t *testing.T) {
	tasks := []PlannedTask{
		{Key: "a"},
		{Key: "b", BlockedByKeys: []string{"a"}},
		{Key: "c", BlockedByKeys: []string{"b"}},
	}

	ancestors := ancestorKeys(tasks, "c")

	require.Contains(t, ancestors, "a")
	require.Contains(t, ancestors, "b")
	require.NotContains(t, ancestors, "c")
}
```

`snapshotForPlan` 是测试辅助函数，为计划中每个 `SelectedEmployeeID` 造一个 planning profile：

```go
func snapshotForPlan(plan RouteDecisionPlan) CoordinationSnapshot {
	pool := make([]DigitalEmployeePlanningProfile, 0, len(plan.Tasks))
	for _, task := range plan.Tasks {
		pool = append(pool, DigitalEmployeePlanningProfile{DigitalEmployeeID: task.SelectedEmployeeID})
	}
	return CoordinationSnapshot{DigitalEmployeePool: pool}
}
```

- [ ] **Step 2: 运行测试，确认它失败**

```bash
go test ./internal/workflow/projectcoordination/ -run 'TestValidateRouteDecisionPlanRejectsRequiredInput|TestAncestorKeys' -v
```

预期：编译失败 —— `ancestorKeys` 未定义。

已确认存在、可直接使用的辅助：`stringsToAny`、`nonNilStrings`、`decodePlannerStringArray`、`invalidRouteDecision`。`graph_validation.go` 与 `openai_compatible_planner.go` 均已 import `strings`。

- [ ] **Step 3: 实现 `ancestorKeys`**

追加到 `graph_validation.go`，紧邻 `hasCycle`：

```go
// ancestorKeys returns every task key reachable by walking BlockedByKeys upward
// from key, excluding key itself. Safe on cyclic input: hasCycle rejects those
// first, and the visited set makes this terminate regardless.
func ancestorKeys(tasks []PlannedTask, key string) map[string]struct{} {
	dependencies := make(map[string][]string, len(tasks))
	for _, task := range tasks {
		dependencies[task.Key] = task.BlockedByKeys
	}
	ancestors := map[string]struct{}{}
	var visit func(string)
	visit = func(current string) {
		for _, blocker := range dependencies[current] {
			if _, seen := ancestors[blocker]; seen {
				continue
			}
			ancestors[blocker] = struct{}{}
			visit(blocker)
		}
	}
	visit(key)
	delete(ancestors, key)
	return ancestors
}
```

- [ ] **Step 4: 在 `ValidateRouteDecisionGraph` 中加校验**

位置：`graph_validation.go` 中 `if hasCycle(plan.Tasks) { ... }` 之后、`return nil` 之前（约 `:146-149`）。

环必须先被拒绝，否则「祖先」无意义。`ValidateRouteDecisionPlan:26` 是 `ValidateRouteDecisionGraph` 的唯一业务调用者，因此 LLM 路径自动覆盖。追加：

```go
	producers := map[string]string{}
	for _, task := range plan.Tasks {
		for _, key := range task.Produces {
			producers[key] = task.Key
		}
	}
	for _, task := range plan.Tasks {
		ancestors := ancestorKeys(plan.Tasks, task.Key)
		for _, required := range plannerRequiredInputs(task.InputRequirements) {
			producer, ok := producers[required]
			if !ok {
				return invalidRouteDecision("task %q: required input %q is produced by no task in this plan", task.Key, required)
			}
			if _, reachable := ancestors[producer]; !reachable {
				return invalidRouteDecision("task %q: required input %q is produced by task %q, which is not an ancestor", task.Key, required, producer)
			}
		}
	}
```

- [ ] **Step 5: 运行测试，确认通过**

```bash
go test ./internal/workflow/projectcoordination/ -run 'TestValidateRouteDecisionPlan|TestAncestorKeys' -v
```

预期：四个新用例 PASS。

- [ ] **Step 6: 跑整包**

```bash
go test ./internal/workflow/projectcoordination/
```

预期：PASS。既有 fixture 无 `produces` 且无 `required_inputs` → 循环体不执行 → 不受影响。

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/
git commit -m "$(cat <<'EOF'
feat(planning): reject a plan whose required input has no upstream producer

A sibling that happens to produce the key is not enough — nothing orders it
before the consumer. The producer must be an ancestor along blocked_by_keys.

This is what lets the platform resolve ownership by lookup at runtime rather than
asking a model who is to blame. A plan that passes this check cannot strand a
task on a missing input that no one ever promised.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: 产出键唯一性

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/graph_validation.go`
- Test: `apps/control-plane/internal/workflow/projectcoordination/graph_validation_test.go`

**Interfaces:**
- Consumes: Task 2 的 `producers` 映射构建方式。
- Produces: `ValidateRouteDecisionPlan` 新增拒绝理由 `duplicate_produces_key`。Plan 5 的 `owner` 解析依赖「一个 key 恰有一个生产者」。

- [ ] **Step 1: 写失败测试**

```go
func TestValidateRouteDecisionPlanRejectsDuplicateProducesKey(t *testing.T) {
	plan := RouteDecisionPlan{Tasks: []PlannedTask{
		planTaskWithIO("a", nil, []string{"report"}, nil),
		planTaskWithIO("b", nil, []string{"report"}, nil),
	}}
	snapshot := snapshotForPlan(plan)

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12})

	require.Error(t, err)
	require.Contains(t, err.Error(), "report")
}

func TestValidateRouteDecisionPlanRejectsBlankProducesKey(t *testing.T) {
	plan := RouteDecisionPlan{Tasks: []PlannedTask{
		planTaskWithIO("a", nil, []string{"  "}, nil),
	}}
	snapshot := snapshotForPlan(plan)

	require.Error(t, ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12}))
}
```

- [ ] **Step 2: 运行测试，确认它失败**

```bash
go test ./internal/workflow/projectcoordination/ -run 'TestValidateRouteDecisionPlanRejectsDuplicateProducesKey|TestValidateRouteDecisionPlanRejectsBlankProducesKey' -v
```

预期：`Duplicate` FAIL（Task 2 的 `producers[key] = task.Key` 静默覆盖，后者胜出）；`Blank` FAIL（空 key 被当作合法生产者）。

**为什么唯一性是必须的**：Plan 5 的 `owner = producers[k]`。若两个任务承诺同一个 key，`owner` 取决于 map 写入顺序——补做会被派给一个不确定的员工。

- [ ] **Step 3: 收紧 `producers` 构建**

替换 Task 2 中的构建循环：

```go
	producers := map[string]string{}
	for _, task := range plan.Tasks {
		for _, key := range task.Produces {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				return invalidRouteDecision("task %q: produces contains an empty key", task.Key)
			}
			if owner, exists := producers[trimmed]; exists {
				return invalidRouteDecision("produces key %q is claimed by both task %q and task %q; a key must have exactly one producer", trimmed, owner, task.Key)
			}
			producers[trimmed] = task.Key
		}
	}
```

- [ ] **Step 4: 运行测试，确认通过**

```bash
go test ./internal/workflow/projectcoordination/ -run TestValidateRouteDecisionPlan -v
```

预期：全部 PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/
git commit -m "$(cat <<'EOF'
feat(planning): a produces key must have exactly one producer

Ownership resolution reads producers[key]. Two tasks claiming the same key would
make the owner depend on map insertion order, so a supplement task would be
dispatched to an arbitrary employee.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: 落库只保留 schema 化的 `input_requirements`

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go:516-518`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/plan_revision_payload.go:51,105,228`
- Test: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`

**Interfaces:**
- Consumes: Task 1–3。
- Produces: `project_tasks.input_requirements` 只含 `required_inputs`（与既有的 `input_context_refs`）；planner 的自由字段落 `planner_metadata.planner_notes`。`project_tasks` 新增 `planner_metadata.produces`。Plan 5 从这里读 `owner`。

- [ ] **Step 1: 写失败测试**

```go
func TestPlannedTaskInputRequirementsKeepOnlyRequiredInputs(t *testing.T) {
	planned := PlannedTask{
		Key:      "b",
		Produces: []string{"summary"},
		InputRequirements: map[string]any{
			"required_inputs": []any{"load_test_report"},
			"repository":      "superteam",
			"scope":           "one host",
		},
	}

	stored, notes := plannedTaskInputRequirements(planned)

	require.Equal(t, map[string]any{"required_inputs": []any{"load_test_report"}}, stored)
	require.Equal(t, map[string]any{"repository": "superteam", "scope": "one host"}, notes)
}
```

- [ ] **Step 2: 运行测试，确认它失败**

```bash
go test ./internal/workflow/projectcoordination/ -run TestPlannedTaskInputRequirements -v
```

预期：编译失败 —— `plannedTaskInputRequirements` 未定义。

- [ ] **Step 3: 实现拆分函数**

追加到 `project_store.go`：

```go
// plannedTaskInputRequirements splits the planner's input_requirements map into
// the one schema'd key that decides things and the prose that does not. The
// prose is kept — it is useful to a human reading the plan — but it is stored
// where no validator or gate can reach it.
func plannedTaskInputRequirements(task PlannedTask) (stored map[string]any, notes map[string]any) {
	stored = map[string]any{}
	notes = map[string]any{}
	for key, value := range task.InputRequirements {
		if key == "required_inputs" {
			stored[key] = value
			continue
		}
		notes[key] = value
	}
	return stored, notes
}
```

- [ ] **Step 4: 接入落库**

`project_store.go:516-518` 替换：

```go
		inputRequirements, plannerNotes := plannedTaskInputRequirements(plannedTask)
		if len(plannedTask.InputContextRefs) > 0 {
			inputRequirements["input_context_refs"] = append([]string(nil), plannedTask.InputContextRefs...)
		}
```

`metadata` 字面量（`:528`）中新增两个键：

```go
			"produces":      stringsToAny(plannedTask.Produces),
			"planner_notes": plannerNotes,
```

`plan_revision_payload.go` 的 `:51` 新增 `Produces []string \`json:"produces,omitempty"\``，`:105` 与 `:228` 各透传一行。

- [ ] **Step 5: 运行测试，确认通过**

```bash
go test ./internal/workflow/projectcoordination/ -run TestPlannedTaskInputRequirements -v
```

预期：PASS。

- [ ] **Step 6: 编译并跑整包**

```bash
go build ./internal/... && go test ./internal/...
```

预期：PASS。注意 `planning_profile_adapter.go:324` 仍读 `input_requirements["tool_requirements"]`——现在它读到的必然是空，与本 plan 之前**行为完全相同**（spec §1.7）。不要在本 plan 里改它。

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/
git commit -m "$(cat <<'EOF'
refactor(planning): input_requirements stores only what decides something

Everything else the planner writes there — repository, scope, items, value — moves
to planner_metadata.planner_notes. It stays readable to a human and stays out of
reach of every validator and gate.

produces is recorded on planner_metadata so ownership can be resolved from a
persisted task, not only from the in-flight plan.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: 分层门禁与真实 E2E

**Files:** 无改动。本 task 是验收。

**Interfaces:**
- Consumes: Task 1–4。
- Produces: 可复述的验收结论。

- [ ] **Step 1: 门禁**

```bash
corepack pnpm verify:control-plane
```

预期：PASS。

- [ ] **Step 2: 重启并确认加载新代码**

```bash
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh status
```

预期：`control-plane: running ... healthy`。

- [ ] **Step 3: 真实 E2E —— 两段式需求产出自洽的计划**

提交一个明确需要两步的需求，例如「先采集这台主机的 CPU 与内存指标，再基于采集结果写一份健康评估」。两名 claude-code 员工在池中。

```bash
DB=$(grep -m1 'url:' apps/control-plane/config/config.yaml | sed 's/.*url: *//;s/"//g')
psql "$DB" -tAc "select planned_task_key,
                        planner_metadata->'produces',
                        input_requirements->'required_inputs'
                 from superteam.project_tasks
                 where project_id='<PROJECT_ID>' order by created_at;"
```

预期：上游任务的 `produces` 非空；下游任务的 `required_inputs` 中的每个 key 都出现在上游的 `produces` 里。

- [ ] **Step 4: 确认自由字段已迁出**

```bash
psql "$DB" -tAc "select jsonb_object_keys(input_requirements) from superteam.project_tasks where project_id='<PROJECT_ID>';"
```

预期：只出现 `required_inputs`（以及可能的 `input_context_refs`）。**不得**再出现 `repository` / `scope` / `items` / `value` / `demand_content`。

```bash
psql "$DB" -tAc "select planner_metadata ? 'planner_notes' from superteam.project_tasks where project_id='<PROJECT_ID>' limit 1;"
```

预期：`t`。

- [ ] **Step 5: 确认坏计划被拒**

无法在 E2E 中可靠诱导模型产出坏计划。改为直接验证拒绝路径已生效：

```bash
go test ./internal/workflow/projectcoordination/ -run 'TestValidateRouteDecisionPlanRejects' -v
```

预期：三条拒绝用例全部 PASS。真实 E2E 只需证明**好计划能通过**（Step 3）。

- [ ] **Step 6: 清理一次性夹具**

归档测试项目、停用测试员工。共享 dev 库不留活跃垃圾数据。

---

## Self-Review

**Spec coverage**

| Spec 章节 | 任务 |
|---|---|
| §4.2 `input_requirements: { required_inputs }` 改造 | Task 1, 4 |
| §4.2 `produces` 新增（及「为何不是第二个 capability 词表」） | Task 1，本 plan 前置一节 |
| §4.2 落库校验：`required_inputs[k]` 由祖先 `produces` | Task 2 |
| §4.2 落库校验：`produces` 键唯一 | Task 3 |
| §4.2 自由字段迁 `planner_notes` | Task 4 |
| §1.7 工具闸门 | **明确不做**，见前置一节 |

**本 plan 不认领**：`plan_acceptance_criteria` 与 `satisfied_by`（Plan 4）、阻塞申报与图延展与迁移 054（Plan 5）、会话降维（Plan 6）、工具死闸门删除（Plan 8）。

**Type consistency**

- `plannerRequiredInputs(raw map[string]any) []string` 在 Task 1 定义；Task 2 与 Task 4 均调用同名函数，参数同为 `task.InputRequirements`。
- `ancestorKeys(tasks []PlannedTask, key string) map[string]struct{}` 在 Task 2 定义，Task 3 通过 `producers` 映射间接受益，不重复定义。
- `plannedTaskInputRequirements(task PlannedTask) (map[string]any, map[string]any)` 在 Task 4 定义并在同 task 内消费。
- `PlannedTask.Produces []string` 在 Task 1 定义；Task 2/3/4 消费。`PlannedTask.ExpectedOutputs` **保持 `[]string` 原状且不参与判定**。
- 测试辅助 `planTaskWithIO` 与 `snapshotForPlan` 在 Task 2 Step 1 定义，Task 3 复用——Task 3 的测试不重复定义它们。

**Placeholder scan**：Task 5 Step 3/4 的 `<PROJECT_ID>` 是执行时才知道的运行期值，不是计划占位符；其余无 TBD / TODO。

**已知风险**

Plan 2 的 Task 3 删除了 `graph_validation.go` 中最后一处 `strings.Join` 用法，可能连带删掉 `"strings"` import。Plan 3 的 Task 3 重新引入 `strings.TrimSpace`——若编译报 `undefined: strings`，把 import 加回去。

Task 2 的校验对**既有的、无 `produces` 的计划**是无害的（循环体不执行）。但一旦 Task 1 的提示词生效，模型开始输出 `produces` 与 `required_inputs`，坏计划会被拒绝并进入 `synthesizeRequiredReviewPlan` 修复重试。若模型反复产出引用不完整的计划，需求会以计划失败告终而非 `no_suitable_employee`。这是**正确的**行为——计划有洞就该被拒——但会在上线初期抬高计划失败率。Task 5 Step 3 应确认修复重试确实能收敛。

Task 4 改变了 `project_tasks.input_requirements` 的形状。**既有行数据不迁移**：老任务的 `repository` / `scope` 等键留在原处，无人读取，无害。若将来要清理，另立迁移。

Task 5 Step 3–4 在共享远端 dev 库上创建真实项目并消耗真实 token。Step 6 的清理不是可选项。
