# Plan Phase Refactor — Plan 2: 计划期能力控制流退役与选人置信度 Implementation Plan
> 复核状态：已实现（基于CHANGELOG证据）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让虚构的能力词表彻底停止驱动控制流——它不再归零选人打分、不再强制人工审批、不再拒绝计划——并引入一个由 planner 直接输出、不经任何服务端打分器的 `selection_confidence`。

**Architecture:** 能力维度从 `ScorePlanningProfile` 中摘除（其余五个有事实源的维度保留）；`ApplyPlanningProfileScores` 仍写 `matched/missing` 供展示，但不再据此翻转审批开关；`graph_validation` 删掉「`required_capabilities` 为空即拒绝」以及两条已成死代码的能力拒绝规则；`selection_confidence` 作为新字段由 planner 输出，低于阈值时整个计划以 `no_suitable_employee` 回到人类。

**Tech Stack:** Go 1.x + `testify/require`、Temporal。测试 `go test ./internal/...`；门禁 `corepack pnpm verify:control-plane`。

## Global Constraints

取自 spec `docs/superpowers/specs/2026-07-10-project-plan-phase-refactor-design.md`（提交 `5a439586`，含 Plan 1 后的勘误）：

- **约束一：闸门只读代码可判定的事实，不读任何 LLM 自述。**
- **约束四：计划级批准不豁免动作级批准。** 本 plan 移除的是「虚构词表触发的审批」，`risk.approval` 与真实风险触发的 `RequiresHumanApproval` 一律保留。
- `external_capabilities` 字段不删，降级为描述性字段。本 plan 不触碰它的存储。
- `ScorePlanningProfile` 的其余五个维度（`scoreRole`、`scoreRuntime`、`scorePermissionsAndTools`、`scoreLoad`、`scoreReliability`）**保留**——它们打的是有事实源的分。
- `selection_confidence` **绝不能**由 `ScorePlanningProfile` 派生（spec §4.2、§1.6）。
- 不改 `hasCycle`：任务图永远是 DAG。
- 提交信息末尾附 `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`。

---

## 前置：与 Plan 1 的关系，以及范围调整

Plan 1（分支 `plan/phase-01-remove-fictional-gate`）已删除**派发时**的能力闸门。审查该实现时发现 spec §1.1 的诊断有误，已于 `5a439586` 勘误：`missing_capabilities` 是服务端算的，不是 LLM 抄的。真实缺陷是**同一结论被审判两次**，Plan 1 只删掉了第二次（人类批准之后那次）。

**第一次审判仍在计划阶段生效，且无路可逃**（spec §1.6）：

| 模型的选择 | 后果 |
|---|---|
| 编造 `required_capabilities` | `missing` 必非空 → `scoreCapabilities:409` 生成 `HardFailure` → `ScorePlanningProfile:178` 把 `Score` **归零** → `ApplyPlanningProfileScores:78` **强制人工审批** |
| 不写 `required_capabilities` | `graph_validation.go:34` **拒绝整个计划** |

本 plan 拆掉这两条路上的机关。

**范围调整**：原分解表把 `required_inputs` 与 `satisfied_by` 校验也放进 Plan 2。审查后重新切分——`satisfied_by` 依附于 `plan_acceptance_criteria`，那是 Plan 4 引入的对象，校验应与被校验对象同批落地。修订后的分解：

| # | 子计划 | 依赖 | 状态 |
|---|---|---|---|
| 1 | 拆除派发闸门与散文指纹 | 无 | 已完成（待合并） |
| 2 | **计划期能力控制流退役 + `selection_confidence`**（本文） | 无 | 本文 |
| 3 | `required_inputs` 结构化 + 上游生产者校验 | 2 | 未写 |
| 4 | 计划级判据 + `satisfied_by` + 人类评审两块 | 3 | 未写 |
| 5 | 阻塞申报 → 上游补做 + 图延展 + 迁移 054 | 4 | 未写 |
| 6 | 会话降维到血缘根 | 无 | 未写 |

---

## File Structure

| 文件 | 职责 | 动作 |
|---|---|---|
| `apps/control-plane/internal/workflow/projectcoordination/planning_profile.go` | 员工打分 | Task 1：能力维度不再产生 `HardFailure`，不再计入 `Score` |
| `apps/control-plane/internal/workflow/projectcoordination/graph_validation.go` | 计划校验 + 打分应用 | Task 2：不再翻转审批开关；Task 3：删三条能力规则 |
| `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go` | planner 输出解析 | Task 4：新增 `selection_confidence`；Task 5：`no_suitable_employee` |
| `apps/control-plane/internal/workflow/projectcoordination/planner.go` | `PlannedTask` 类型 | Task 4：新增字段 |
| `apps/control-plane/internal/workflow/projectcoordination/plan_revision_payload.go` | 计划落库载荷 | Task 4：透传新字段 |
| `apps/control-plane/internal/workflow/projectcoordination/graph_validation.go` | 阈值与 `ErrNoSuitableEmployee` | Task 5：新增，无需改 `CoordinationSnapshot` |

---

### Task 1: 能力不再归零选人打分

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/planning_profile.go:388-414`
- Test: `apps/control-plane/internal/workflow/projectcoordination/planning_profile_test.go`

**Interfaces:**
- Consumes: 无（本 plan 起点）
- Produces: `ScorePlanningProfile(profile DigitalEmployeePlanningProfile, req PlanningTaskRequirements) PlanningProfileScore` 签名不变。`score.MatchedCapabilities` / `score.MissingCapabilities` 仍被填充（供展示）；`score.HardFailures` **不再**包含 `missing_capability:*`；`score.Score` 不再受能力影响。Task 2、Task 3 依赖此行为。

- [ ] **Step 1: 写失败测试**

追加到 `planning_profile_test.go`：

```go
func TestScorePlanningProfileDoesNotHardFailOnMissingCapability(t *testing.T) {
	profile := DigitalEmployeePlanningProfile{
		DigitalEmployeeID: uuid.New(),
		Capabilities:      []PlanningCapability{{Key: "bash_execution"}},
	}
	req := PlanningTaskRequirements{
		// A name the planner invented. There is no registry it could have drawn from.
		RequiredCapabilities: []string{"quantum-ledger.reconcile_verification"},
	}

	score := ScorePlanningProfile(profile, req)

	require.Empty(t, score.HardFailures, "an unmatched capability name is not a hard failure")
	require.Greater(t, score.Score, 0, "score must survive an unmatched capability")
	require.Equal(t, []string{"quantum-ledger.reconcile_verification"}, score.MissingCapabilities,
		"the diff is still reported for display")
}

func TestScorePlanningProfileScoreIsIndependentOfCapabilities(t *testing.T) {
	profile := DigitalEmployeePlanningProfile{DigitalEmployeeID: uuid.New()}

	withNone := ScorePlanningProfile(profile, PlanningTaskRequirements{})
	withInvented := ScorePlanningProfile(profile, PlanningTaskRequirements{
		RequiredCapabilities: []string{"a", "b", "c"},
	})

	require.Equal(t, withNone.Score, withInvented.Score)
}
```

- [ ] **Step 2: 运行测试，确认它失败**

```bash
go test ./internal/workflow/projectcoordination/ -run TestScorePlanningProfile -v
```

预期：`TestScorePlanningProfileDoesNotHardFailOnMissingCapability` FAIL —— `HardFailures` 含 `missing_capability:quantum-ledger.reconcile_verification`，且 `Score` 因 `planning_profile.go:178-181` 被归零。

- [ ] **Step 3: 改写 `scoreCapabilities`**

`planning_profile.go:388`，整体替换（注意原函数尾部有一处重复的 `if len(req.RequiredCapabilities) == 0` 死分支，一并去掉）：

```go
// scoreCapabilities records the capability diff for display but contributes a
// constant to the score.
//
// Both sides of the diff are free text with no registry: required_capabilities is
// synthesised by the planner because the prompt offers no vocabulary, and the
// employee's Capabilities come from external_capabilities, which nothing
// validates and the runtime never reads. Letting that diff zero the score (via
// HardFailures) meant an invented name could disqualify a perfectly capable
// employee. See the 2026-07-10 plan-phase refactor spec §1.6.
func scoreCapabilities(profile DigitalEmployeePlanningProfile, req PlanningTaskRequirements, result *PlanningProfileScore) int {
	available := map[string]struct{}{}
	for _, capability := range profile.Capabilities {
		key := normalizePlanningString(capability.Key)
		if key != "" {
			available[key] = struct{}{}
		}
	}
	for _, required := range req.RequiredCapabilities {
		key := normalizePlanningString(required)
		if key == "" {
			continue
		}
		if _, ok := available[key]; ok {
			result.MatchedCapabilities = append(result.MatchedCapabilities, key)
			continue
		}
		result.MissingCapabilities = append(result.MissingCapabilities, key)
	}
	return 40
}
```

`HardFailures` 的其他来源（`profile.HardFailures`，见 `ScorePlanningProfile:168`）不动。

- [ ] **Step 4: 运行测试，确认通过**

```bash
go test ./internal/workflow/projectcoordination/ -run TestScorePlanningProfile -v
```

预期：两个用例 PASS。

- [ ] **Step 5: 跑整包，捕获依赖旧行为的测试**

```bash
go test ./internal/workflow/projectcoordination/
```

预期：`graph_validation_test.go:191` 的 `TestValidateRouteDecisionPlanRejectsHardMissingCapabilityWithoutReview` FAIL——它断言的正是要删除的行为。**不要在本 task 修它**，Task 3 会连同规则一起删除。若要让本 task 的提交保持绿色，先给该测试加 `t.Skip("removed in Task 3")`，并在 Task 3 删除整个测试函数。

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/planning_profile.go apps/control-plane/internal/workflow/projectcoordination/planning_profile_test.go apps/control-plane/internal/workflow/projectcoordination/graph_validation_test.go
git commit -m "$(cat <<'EOF'
fix(planning): an invented capability name no longer disqualifies an employee

scoreCapabilities turned every unmatched capability into a HardFailure, and one
HardFailure zeroes the whole SelectionScore. Both sides of that diff are free
text with no registry — the planner invents required_capabilities because the
prompt gives it no vocabulary, and external_capabilities is an unvalidated field
the runtime never reads.

The diff is still recorded for display. It no longer decides anything.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: 能力不再强制人工审批

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/graph_validation.go:62-82`
- Test: `apps/control-plane/internal/workflow/projectcoordination/graph_validation_test.go`

**Interfaces:**
- Consumes: Task 1 的 `score.HardFailures` 不再含能力项。
- Produces: `ApplyPlanningProfileScores(snapshot CoordinationSnapshot, plan *RouteDecisionPlan)` 签名不变。仍写 `task.SelectionScore` / `MatchedCapabilities` / `MissingCapabilities` / `PlanningProfileSnapshotHash`；**不再**因能力翻转 `RequiresHumanApproval` / `RequiresHumanReview`。`profile.HardFailures`（有事实源，如员工不可调度）仍翻转它们。

- [ ] **Step 1: 写失败测试**

```go
func TestApplyPlanningProfileScoresDoesNotForceApprovalOnMissingCapability(t *testing.T) {
	employeeID := uuid.New()
	snapshot := CoordinationSnapshot{
		DigitalEmployeePool: []DigitalEmployeePlanningProfile{{
			DigitalEmployeeID: employeeID,
			Capabilities:      []PlanningCapability{{Key: "bash_execution"}},
		}},
	}
	plan := RouteDecisionPlan{Tasks: []PlannedTask{{
		Key:                  "t1",
		SelectedEmployeeID:   employeeID,
		RequiredCapabilities: []string{"invented.capability"},
	}}}

	ApplyPlanningProfileScores(snapshot, &plan)

	require.False(t, plan.RequiresHumanReview, "a fictional vocabulary must not trigger human review")
	require.False(t, plan.Tasks[0].RequiresHumanApproval)
	require.Equal(t, []string{"invented.capability"}, plan.Tasks[0].MissingCapabilities,
		"still recorded for display")
}

func TestApplyPlanningProfileScoresStillForcesApprovalOnProfileHardFailure(t *testing.T) {
	employeeID := uuid.New()
	snapshot := CoordinationSnapshot{
		DigitalEmployeePool: []DigitalEmployeePlanningProfile{{
			DigitalEmployeeID: employeeID,
			// A real, server-derived fact — not a capability name.
			HardFailures: []string{"employee_not_dispatchable"},
		}},
	}
	plan := RouteDecisionPlan{Tasks: []PlannedTask{{Key: "t1", SelectedEmployeeID: employeeID}}}

	ApplyPlanningProfileScores(snapshot, &plan)

	require.True(t, plan.RequiresHumanReview)
	require.True(t, plan.Tasks[0].RequiresHumanApproval)
}
```

- [ ] **Step 2: 运行测试，确认第一个失败**

```bash
go test ./internal/workflow/projectcoordination/ -run TestApplyPlanningProfileScores -v
```

预期：`...DoesNotForceApprovalOnMissingCapability` FAIL（`graph_validation.go:78` 因 `MissingCapabilities` 非空而置真）；`...StillForcesApprovalOnProfileHardFailure` PASS。

- [ ] **Step 3: 收紧翻转条件**

`graph_validation.go:78-81`：

```go
		if len(score.HardFailures) > 0 {
			task.RequiresHumanApproval = true
			plan.RequiresHumanReview = true
		}
```

去掉 `|| len(score.MissingCapabilities) > 0`。经 Task 1 之后，`score.HardFailures` 只承载 `profile.HardFailures`——那是有事实源的。

- [ ] **Step 4: 运行测试，确认两个都通过**

```bash
go test ./internal/workflow/projectcoordination/ -run TestApplyPlanningProfileScores -v
```

预期：PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/graph_validation.go apps/control-plane/internal/workflow/projectcoordination/graph_validation_test.go
git commit -m "$(cat <<'EOF'
fix(planning): stop a fictional vocabulary from forcing human approval

Every task was force-approved, because external_capabilities is effectively
always empty while the planner always invents required_capabilities, so
MissingCapabilities was always non-empty. Human approval fired on vocabulary
mismatch rather than on risk, which dilutes risk.approval.

Profile hard failures still force approval. Those are facts.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: 计划校验删掉三条能力规则

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/graph_validation.go:34-35`（空 required 即拒绝）、`:53-57`（两条死规则）
- Test: `apps/control-plane/internal/workflow/projectcoordination/graph_validation_test.go:191`（删除整个测试函数）

**Interfaces:**
- Consumes: Task 1、Task 2。
- Produces: `ValidateRouteDecisionPlan(snapshot CoordinationSnapshot, plan RouteDecisionPlan, policy GraphValidationPolicy) error` 签名不变；不再因 `required_capabilities` 的内容拒绝任何计划。`hasInvalidRequirementString` 的形状校验（空串/未 trim）保留。

- [ ] **Step 1: 写失败测试**

```go
func TestValidateRouteDecisionPlanAcceptsEmptyRequiredCapabilities(t *testing.T) {
	employeeID := uuid.New()
	snapshot := CoordinationSnapshot{
		DigitalEmployeePool: []DigitalEmployeePlanningProfile{{DigitalEmployeeID: employeeID}},
	}
	plan := RouteDecisionPlan{
		RequiresHumanReview: false,
		Tasks: []PlannedTask{{
			Key:                     "t1",
			SelectedEmployeeID:      employeeID,
			EmployeeSelectionReason: "only executor in pool",
			RequiredCapabilities:    nil,
			AcceptanceCriteria:      []string{"command exits zero"},
			ExpectedOutputs:         []string{"evidence_refs"},
		}},
	}

	require.NoError(t, ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12}))
}
```

- [ ] **Step 2: 运行测试，确认它失败**

```bash
go test ./internal/workflow/projectcoordination/ -run TestValidateRouteDecisionPlanAcceptsEmptyRequiredCapabilities -v
```

预期：FAIL，错误信息 `task "t1": required_capabilities is empty and the task is not flagged for human review`。

**这条规则的荒谬之处**：它逼着模型去发明能力名，而发明出来的名字又必然对不上——那正是 Task 1 与 Task 2 刚拆掉的两个惩罚。

- [ ] **Step 3: 删除 `graph_validation.go:34-35`**

```go
		if !plan.RequiresHumanReview && !task.RequiresHumanApproval && len(task.RequiredCapabilities) == 0 {
			return invalidRouteDecision("task %q: required_capabilities is empty and the task is not flagged for human review", task.Key)
		}
```

- [ ] **Step 4: 删除 `graph_validation.go:53-57` 的两条死规则**

```go
		if len(score.HardFailures) > 0 && !reviewRequired {
			return invalidRouteDecision("task %q: selected employee %s has %d capability hard-failure(s) but the task is not flagged for human review", task.Key, task.SelectedEmployeeID, len(score.HardFailures))
		}
		if len(score.MissingCapabilities) > 0 && !reviewRequired {
			return invalidRouteDecision("task %q: selected employee %s is missing %d required capability/-ies (%s) but the task is not flagged for human review", task.Key, task.SelectedEmployeeID, len(score.MissingCapabilities), strings.Join(score.MissingCapabilities, ", "))
		}
```

它们**不可达**：`ValidateRouteDecisionPlan` 的唯一调用点（`openai_compatible_planner.go:133,142`）永远在 `ApplyPlanningProfileScores` 之后，而后者在 `HardFailures` 非空时已把 `reviewRequired` 置真。

随之删除现在无用的局部变量 `score` 与 `reviewRequired`（若 `profile` 仅用于「有无 planning profile」的存在性检查，保留该检查）。

删除 `graph_validation_test.go:191` 的 `TestValidateRouteDecisionPlanRejectsHardMissingCapabilityWithoutReview` 整个函数（Task 1 Step 5 给它加的 `t.Skip` 一并消失）。

- [ ] **Step 5: 编译并跑整包**

```bash
go build ./internal/... && go test ./internal/workflow/projectcoordination/
```

预期：PASS。若 `strings` 变为未使用，删除该 import。

- [ ] **Step 6: 确认无残留**

```bash
grep -n "RequiredCapabilities\|MissingCapabilities" apps/control-plane/internal/workflow/projectcoordination/graph_validation.go
```

预期：只在 `planningTaskRequirements`（`:163`）与 `ApplyPlanningProfileScores` 的展示性赋值中出现，不出现在任何 `return invalidRouteDecision(...)` 附近。

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/graph_validation.go apps/control-plane/internal/workflow/projectcoordination/graph_validation_test.go
git commit -m "$(cat <<'EOF'
fix(planning): stop rejecting plans over a vocabulary that does not exist

Three rules keyed on required_capabilities. One rejected a plan whose
required_capabilities was empty, which is to say it required the model to invent
names from a vocabulary it was never given. The other two rejected plans on
capability mismatch and were unreachable anyway: ApplyPlanningProfileScores
always runs first and had already flipped reviewRequired.

Shape validation of the requirement lists is unchanged.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: planner 直接输出 `selection_confidence`

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/planner.go:64` 附近（`PlannedTask`）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go:274`（提示词）、`:370` 与 `:395`（结构体）、`:417`、`:434`（解析）、新增解码函数
- Modify: `apps/control-plane/internal/workflow/projectcoordination/plan_revision_payload.go:58,112,249,309`（透传）
- Test: `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `PlannedTask.SelectionConfidence float64`（0.0–1.0）。新函数 `decodePlannerSelectionConfidence(raw json.RawMessage) (float64, error)`。`PlannedTask.SelectionScore int` **保留**——经 Task 1 之后它是五个有事实源维度的加权分，与置信度是两回事。Task 5 消费 `SelectionConfidence`。

- [ ] **Step 1: 写失败测试**

```go
func TestDecodePlannerSelectionConfidenceAcceptsFractions(t *testing.T) {
	value, err := decodePlannerSelectionConfidence(json.RawMessage(`0.85`))
	require.NoError(t, err)
	require.InDelta(t, 0.85, value, 1e-9)
}

func TestDecodePlannerSelectionConfidenceRejectsOutOfRange(t *testing.T) {
	_, err := decodePlannerSelectionConfidence(json.RawMessage(`1.5`))
	require.Error(t, err)

	_, err = decodePlannerSelectionConfidence(json.RawMessage(`-0.1`))
	require.Error(t, err)
}

func TestDecodePlannerSelectionConfidenceRejectsMissing(t *testing.T) {
	_, err := decodePlannerSelectionConfidence(json.RawMessage(``))
	require.Error(t, err)

	_, err = decodePlannerSelectionConfidence(json.RawMessage(`null`))
	require.Error(t, err)
}
```

**为什么不复用 `decodePlannerSelectionScore`**：它把任何落在 `[0,1]` 的值映射为 `0`（`openai_compatible_planner.go:461-463`）。模型输出 `0.85` 会变成 `0`。置信度必须走自己的解码器。

- [ ] **Step 2: 运行测试，确认它失败**

```bash
go test ./internal/workflow/projectcoordination/ -run TestDecodePlannerSelectionConfidence -v
```

预期：编译失败 —— `decodePlannerSelectionConfidence` 未定义。

- [ ] **Step 3: 实现解码器**

追加到 `openai_compatible_planner.go`（紧邻 `decodePlannerSelectionScore`）：

```go
// decodePlannerSelectionConfidence parses the planner's own confidence that the
// selected employee fits the task.
//
// It is deliberately separate from decodePlannerSelectionScore, which maps any
// value in [0,1] to 0 — a 0.85 confidence would silently become 0. Confidence is
// also never derived from ScorePlanningProfile: that scorer is a weighted sum of
// server-side facts, not a judgement about a natural-language description.
func decodePlannerSelectionConfidence(raw json.RawMessage) (float64, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return 0, fmt.Errorf("selection_confidence is required")
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil {
		return 0, fmt.Errorf("selection_confidence must be a number: %w", err)
	}
	parsed, err := strconv.ParseFloat(number.String(), 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("selection_confidence must be finite")
	}
	if parsed < 0 || parsed > 1 {
		return 0, fmt.Errorf("selection_confidence must be within [0,1], got %v", parsed)
	}
	return parsed, nil
}
```

- [ ] **Step 4: 运行测试，确认通过**

```bash
go test ./internal/workflow/projectcoordination/ -run TestDecodePlannerSelectionConfidence -v
```

预期：三个用例 PASS。

- [ ] **Step 5: 接进 `PlannedTask` 与解析链路**

`planner.go:64` 附近，在 `SelectionScore int` 下方新增：

```go
	SelectionConfidence         float64
```

`openai_compatible_planner.go:370` 的 `plannerTask` 新增 `SelectionConfidence float64 \`json:"selection_confidence"\``；`:395` 的 raw 结构体新增 `SelectionConfidence json.RawMessage \`json:"selection_confidence"\``；`:417` 下方新增：

```go
	selectionConfidence, err := decodePlannerSelectionConfidence(raw.SelectionConfidence)
	if err != nil {
		return err
	}
```

`:434` 的字面量中新增 `SelectionConfidence: selectionConfidence,`；`:334` 的透传中新增 `SelectionConfidence: task.SelectionConfidence,`。

`plan_revision_payload.go` 的 `:58`、`:112`、`:249`、`:309` 四处照 `SelectionScore` 的写法各加一行 `SelectionConfidence`（字段名 `selection_confidence,omitempty`）。

- [ ] **Step 6: 提示词要求模型输出该字段**

`openai_compatible_planner.go:274` 的字段清单里，在 `selection_score` 之后插入 `selection_confidence`。并追加一句：

```go
		"selection_confidence is your own 0.0-1.0 confidence that the selected employee's described role and experience fit this task. Judge it from the employee's description, not from capability name overlap.",
```

- [ ] **Step 7: 编译并跑 planner 测试**

```bash
go build ./internal/... && go test ./internal/workflow/projectcoordination/ -run Planner
```

预期：PASS。既有的 planner fixture 若缺 `selection_confidence`，解析会报错——这是有意的（该字段必填）。给 fixture 补上 `"selection_confidence": 0.9`。

- [ ] **Step 8: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/
git commit -m "$(cat <<'EOF'
feat(planning): planner emits its own selection_confidence

The employee's fitness for a task is a judgement about a natural-language
description. It cannot come from ScorePlanningProfile, which is a weighted sum of
server-side facts, and it cannot reuse decodePlannerSelectionScore, which folds
any value in [0,1] down to 0 — a 0.85 confidence would silently read as 0.

SelectionScore is kept. After the capability dimension was removed it is a
meaningful aggregate of role, runtime, permissions, load and reliability.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: 置信度不足即回到人类

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go:129-135`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/graph_validation.go`（新增校验）
- Test: `apps/control-plane/internal/workflow/projectcoordination/graph_validation_test.go`

**Interfaces:**
- Consumes: Task 4 的 `PlannedTask.SelectionConfidence`。
- Produces: 新错误 `ErrNoSuitableEmployee`（包级），由 `ValidateRouteDecisionPlan` 在任一任务的置信度低于阈值时返回。新函数 `selectionConfidenceThreshold(policy map[string]any) float64`，三级回退：`projects.coordination_policy.selection_confidence_threshold` → Tenant Profile（本 plan 留 hook，不实现）→ 常量 `defaultSelectionConfidenceThreshold = 0.7`。`CoordinationSnapshot.CoordinationPolicy` 已存在且已装配，无需新增。

- [ ] **Step 1: 写失败测试**

```go
func TestValidateRouteDecisionPlanRejectsLowConfidence(t *testing.T) {
	employeeID := uuid.New()
	snapshot := CoordinationSnapshot{
		DigitalEmployeePool: []DigitalEmployeePlanningProfile{{DigitalEmployeeID: employeeID}},
	}
	plan := RouteDecisionPlan{Tasks: []PlannedTask{{
		Key:                     "t1",
		SelectedEmployeeID:      employeeID,
		EmployeeSelectionReason: "closest match, but weak",
		SelectionConfidence:     0.4,
		AcceptanceCriteria:      []string{"command exits zero"},
		ExpectedOutputs:         []string{"evidence_refs"},
	}}}

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12})

	require.ErrorIs(t, err, ErrNoSuitableEmployee)
}

func TestValidateRouteDecisionPlanAcceptsConfidenceAtThreshold(t *testing.T) {
	employeeID := uuid.New()
	snapshot := CoordinationSnapshot{
		DigitalEmployeePool: []DigitalEmployeePlanningProfile{{DigitalEmployeeID: employeeID}},
	}
	plan := RouteDecisionPlan{Tasks: []PlannedTask{{
		Key:                     "t1",
		SelectedEmployeeID:      employeeID,
		EmployeeSelectionReason: "exact match",
		SelectionConfidence:     0.7,
		AcceptanceCriteria:      []string{"command exits zero"},
		ExpectedOutputs:         []string{"evidence_refs"},
	}}}

	require.NoError(t, ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12}))
}

func TestSelectionConfidenceThresholdPrefersProjectPolicy(t *testing.T) {
	require.InDelta(t, 0.9,
		selectionConfidenceThreshold(map[string]any{"selection_confidence_threshold": 0.9}), 1e-9)
	require.InDelta(t, defaultSelectionConfidenceThreshold,
		selectionConfidenceThreshold(nil), 1e-9)
	require.InDelta(t, defaultSelectionConfidenceThreshold,
		selectionConfidenceThreshold(map[string]any{"selection_confidence_threshold": "not a number"}), 1e-9)
}
```

- [ ] **Step 2: 运行测试，确认它失败**

```bash
go test ./internal/workflow/projectcoordination/ -run 'TestValidateRouteDecisionPlanRejectsLowConfidence|TestSelectionConfidenceThreshold' -v
```

预期：编译失败 —— `ErrNoSuitableEmployee`、`selectionConfidenceThreshold`、`defaultSelectionConfidenceThreshold` 均未定义。

- [ ] **Step 3: 实现阈值与错误**

`graph_validation.go` 顶部：

```go
// defaultSelectionConfidenceThreshold is the fallback floor. The real knob is
// projects.coordination_policy.selection_confidence_threshold — a per-project
// decision, not an ops constant.
const defaultSelectionConfidenceThreshold = 0.7

// ErrNoSuitableEmployee means the planner could not find an employee it believed
// fit the task. The demand goes back to the human with the planner's reasons; it
// is not a plan defect to be repaired.
var ErrNoSuitableEmployee = errors.New("no suitable employee")

func selectionConfidenceThreshold(policy map[string]any) float64 {
	raw, ok := policy["selection_confidence_threshold"]
	if !ok {
		return defaultSelectionConfidenceThreshold
	}
	switch value := raw.(type) {
	case float64:
		if value > 0 && value <= 1 {
			return value
		}
	case json.Number:
		if parsed, err := value.Float64(); err == nil && parsed > 0 && parsed <= 1 {
			return parsed
		}
	}
	return defaultSelectionConfidenceThreshold
}
```

若 `errors` / `encoding/json` 未 import，补上。

- [ ] **Step 4: 在 `ValidateRouteDecisionPlan` 的任务循环里加校验**

在 `EmployeeSelectionReason` 非空校验之后：

```go
		if task.SelectionConfidence < selectionConfidenceThreshold(snapshot.CoordinationPolicy) {
			return fmt.Errorf("%w: task %q: employee %s scored %.2f", ErrNoSuitableEmployee, task.Key, task.SelectedEmployeeID, task.SelectionConfidence)
		}
```

`CoordinationSnapshot.CoordinationPolicy map[string]any` **已存在**（`planner.go:16`），并已由 `project_store.go:351` 从 `projects.coordination_policy` 装配。`requiredHumanReviewPolicyEnabled`（`openai_compatible_planner.go:651`）是它的既有读者，可作为写法参照。**本 task 无需新增字段或改装配。**

- [ ] **Step 5: 低置信度不得走「修复」分支**

`openai_compatible_planner.go:133` 起的 `if err := ValidateRouteDecisionPlan(...); err != nil` 块内，`synthesizeRequiredReviewPlan` 的修复重试是为「计划形状不合规」准备的。低置信度不是形状问题，修复无意义。在进入修复前拦截：

```go
		if err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12}); err != nil {
			if errors.Is(err, ErrNoSuitableEmployee) {
				return RouteDecisionPlan{}, err
			}
			if contextErr := terminalContextError(ctx); contextErr != nil {
				return RouteDecisionPlan{}, contextErr
			}
			// ... 既有修复逻辑不动
```

- [ ] **Step 6: 运行测试，确认通过**

```bash
go test ./internal/workflow/projectcoordination/
```

预期：全部 PASS。

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/
git commit -m "$(cat <<'EOF'
feat(planning): return no_suitable_employee below the confidence threshold

If the planner does not believe any employee in the pool fits the task, the
demand goes back to the human with its reasons. That is an answer, not a defect,
so it must not enter the plan-repair retry that exists for malformed shapes.

The threshold lives on projects.coordination_policy, a field that until now was
stored and passed through but never read. The constant is only a floor.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: 分层门禁与真实 E2E

**Files:** 无改动。本 task 是验收。

**Interfaces:**
- Consumes: Task 1–5。
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

- [ ] **Step 3: 真实 E2E —— 编造的能力名不再造成任何后果**

建一个 `capability_bindings.external_capabilities` 为空的 claude-code 员工，提交一个需求。批准计划（若仍有 `risk.approval` 则批准之），等任务派发。

```bash
DB=$(grep -m1 'url:' apps/control-plane/config/config.yaml | sed 's/.*url: *//;s/"//g')
psql "$DB" -tAc "select planner_metadata->'employee_selection'->>'selection_score',
                        planner_metadata->'employee_selection'->'missing_capabilities',
                        requires_human_approval
                 from superteam.project_tasks order by created_at desc limit 1;"
```

预期三件事同时成立：

- `missing_capabilities` **非空**（模型仍会编造能力名，字段仍作展示用）。
- `selection_score` **大于 0**（Task 1：能力不再归零打分）。
- `requires_human_approval` 为 **false**，除非该任务本身高风险（Task 2：能力不再强制审批）。

这三条一起证明：词表还在被记录，但不再决定任何事。

- [ ] **Step 4: 验证低置信度回到人类**

构造一个与员工描述明显不匹配的需求（例如员工描述是「执行 shell 命令」，需求是「设计企业级财务对账架构」）。

预期：需求不落计划，协调事件中出现 `no_suitable_employee` 的错误族；`project_tasks` 中无新行。

- [ ] **Step 5: 验证空 `required_capabilities` 的计划能落库**

若模型此次未输出 `required_capabilities`，计划应正常落库（Task 3）。若模型总是输出，直接在 `graph_validation_test.go` 的单测中覆盖即可，不必强求 E2E 复现。

- [ ] **Step 6: 清理一次性夹具**

归档测试项目、停用测试员工。共享 dev 库不留活跃垃圾数据。

---

## Self-Review

**Spec coverage**

| Spec 章节 | 任务 |
|---|---|
| §1.6 表格第 1 行（HardFailure → 归零 → 强制审批） | Task 1, 2 |
| §1.6 表格第 2 行（空 required 即拒绝） | Task 3 |
| §1.6「两条死规则」 | Task 3 |
| §5 删除清单后四行 | Task 1, 2, 3 |
| §4.2 `selection_confidence` 不得由 `ScorePlanningProfile` 派生 | Task 4 |
| §4.2 `no_suitable_employee` | Task 5 |
| §4.8 阈值三级回退 | Task 5（Tenant Profile 层留 hook） |
| §4.8 `coordination_policy` 新增 `selection_confidence_threshold` 键 | Task 5 |

**本 plan 不认领**：`required_inputs` 结构化与上游生产者校验（Plan 3）、`plan_acceptance_criteria` 与 `satisfied_by`（Plan 4）、图延展与迁移 054（Plan 5）、会话降维（Plan 6）。

**Type consistency**

- `ScorePlanningProfile` 与 `ApplyPlanningProfileScores` 签名在 Task 1–3 全程不变；只有 `score.HardFailures` 的**内容**收窄。
- `PlannedTask.SelectionScore int` 保留；`PlannedTask.SelectionConfidence float64` 新增。两者不互相派生，Task 4 的提交信息里写明了原因。
- `decodePlannerSelectionConfidence(raw json.RawMessage) (float64, error)` 在 Task 4 定义，Task 5 不直接调用它（Task 5 消费的是已解析的 `PlannedTask.SelectionConfidence`）。
- `ErrNoSuitableEmployee` 与 `selectionConfidenceThreshold` 均在 Task 5 定义并在同 task 内消费。

**Placeholder scan**：无 TBD / TODO。每个改码步骤都有完整代码块与预期输出。

**已知风险**

Task 1 Step 5 会让 `graph_validation_test.go:191` 的既有测试失败。计划要求先加 `t.Skip("removed in Task 3")` 保持提交绿色，Task 3 再删除该函数。若执行者跳过这一步，Task 1 的提交将带着一个红色测试。

Task 5 Step 5 修改的是 `openai_compatible_planner.go` 的错误分支。执行者须确认 `errors.Is` 的语义链完整——`ValidateRouteDecisionPlan` 必须用 `fmt.Errorf("%w: ...", ErrNoSuitableEmployee)` 包装，否则 `errors.Is` 不成立，低置信度计划会掉进修复重试。

Task 6 Step 3–4 在共享远端 dev 库上创建真实项目并消耗真实 token。Step 6 的清理不是可选项。
