# Plan Phase Refactor — Plan 4: 计划级验收判据与归属 Implementation Plan
> 复核状态：已实现（基于CHANGELOG证据）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让计划自带一组「计划级验收判据」,每条判据声明由哪些任务的 `produces` 来满足,并在计划落库前校验这份归属的完整性。这是 spec §4.3「人类评审两块确认」和 §4.7「人类裁决触发延展」共同的承重对象。

**Architecture:** `PlanRevisionPayload` 新增 `PlanAcceptanceCriteria []`,每条带 `statement` 与 `satisfied_by [task_key]`。落库校验复用 Plan 3 的 `producers` 映射,确保每条归属指向已存在且唯一的 task_key。planner 提示词要求产出这组判据。前端在计划评审界面把「调度顺序」与「验收判据」作为两块呈现给人类。

**Tech Stack:** Go 1.x + `testify/require`、Temporal;React + TanStack。测试 `go test ./internal/workflow/projectcoordination/`;门禁 `corepack pnpm verify:control-plane`、`corepack pnpm verify:web`。

## Global Constraints

取自 spec `docs/superpowers/specs/2026-07-10-project-plan-phase-refactor-design.md`(含历次勘误):

- **约束一:闸门只读代码可判定的事实,不读任何 LLM 自述。** `satisfied_by` 是「计划内部引用完整性」检查,与 `blocked_by_keys`、Plan 3 的 `produces`/`required_inputs` 同类——校验的是同一份人类批准的计划自不自洽,不与任何外部词表比对。
- **判据的 `arbiter` 本期只有 `human`**(spec §4.5):计划级判据不引入通用审查员,裁决权留在人类。本 plan 不实现 `verification_method` 注册表(`intent-acceptance-criteria` spec 负责)。
- **`satisfied_by` 引用 `task_key`,不引用 `produces` key**:一条计划级判据归属「哪些任务负责产出这条判据的证据」,直接指 task。这与 Plan 3 的 `required_inputs`→`produces`(跨字段匹配)不同——前者同字段,粒度到任务。Plan 5 的图延展若需更细(重跑某任务的 produces 子集),那时再升级到 produces key。
- 提交信息末尾附 `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`。

---

## 前置:依赖与现状

**依赖 Plan 3(已合并 `206f7e85`)。** Plan 3 引入了 `PlanRevisionTask.Produces` 与 `producers` 映射、以及 `required_inputs` 的祖先可达校验。本 plan 的 `satisfied_by` 引用 `task_key`,校验它是否为计划中的真实任务(成员关系,非 producers 映射)。理由:判据归属到「任务」这一级,而非「任务的某个产出键」,更简单且与 DAG 一致。

**现状:计划级判据不存在,但数据通路已就绪。**

- `PlanRevisionPayload`(`plan_revision_payload.go:14`)顶层只有 `Summary/Assumptions/RiskAssessment/HumanReview/Tasks/FinalSummaryContract`,**无计划级判据**。
- `planRevisionReviewContext`(`project_store.go:2922`)已经把整个 `tasks`(含 `produces`、`blocked_by_keys`、任务级 `acceptance_criteria`)塞进审批上下文。**顺序与任务级判据的数据其实已到前端**——人类审批时看不到,只是因为 `DecisionRequest` 顶层 snapshot 只有 `Title/Summary/RiskLevel` 三个字段(§1.4),且前端没渲染。
- 任务级 `acceptance_criteria`(`plan_revision_payload.go:54`)已强制非空(`:187`),保留不动。本 plan 引入的是它的上一级——计划级。

**本 plan 不碰:**
- 运行期的裁决与图延展(`satisfied_by` 怎么用于定位重跑任务)——属 Plan 5。
- `verification_method` / `arbiter` 的取值扩展——属 intent-acceptance spec。
- 会话降维——属 Plan 6。

---

## File Structure

| 文件 | 职责 | 动作 |
|---|---|---|
| `apps/control-plane/internal/workflow/projectcoordination/plan_revision_payload.go` | 计划修订载荷类型 | Task 1:新增 `PlanAcceptanceCriteria` 类型与字段;Task 2:落库校验 |
| `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go` | planner 输出解析与提示词 | Task 1:解析 `plan_acceptance_criteria`;Task 3:提示词 |
| `apps/control-plane/internal/workflow/projectcoordination/planner.go` | `RouteDecisionPlan` 类型 | Task 1:新增 `PlanAcceptanceCriteria []` |
| `apps/control-plane/internal/workflow/projectcoordination/project_store.go` | 计划落库 + 评审上下文 | Task 2:校验在 review 提交处生效;Task 4:评审上下文透传判据 |
| `apps/web/src/features/projects/components/project-plan-review*.tsx` | 计划评审 UI | Task 5:展示两块(顺序 + 判据) |

---

### Task 1: 计划级判据类型与解析

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/plan_revision_payload.go:14`（`PlanRevisionPayload`）、新增类型
- Modify: `apps/control-plane/internal/workflow/projectcoordination/planner.go:42`（`RouteDecisionPlan`）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go`（解析 `:340` 透传、`:370`/`:395` 结构体、`:434` 字面量）
- Test: `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner_test.go`

**Interfaces:**
- Consumes: Plan 3 的 `PlanRevisionTask.Produces`、`decodePlannerStringArray`、`nonNilStrings`。
- Produces: `PlanRevisionPayload.PlanAcceptanceCriteria []PlanAcceptanceCriterion`;`RouteDecisionPlan.PlanAcceptanceCriteria`。`PlanAcceptanceCriterion{ ID string; Statement string; SatisfiedBy []string }`。新函数 `decodePlanAcceptanceCriteria(raw json.RawMessage) []PlanAcceptanceCriterion`。Task 2、Task 4 依赖。

- [ ] **Step 1: 写失败测试**

追加到 `openai_compatible_planner_test.go`:

```go
func TestDecodePlanAcceptanceCriteriaParsesCriteria(t *testing.T) {
	raw := json.RawMessage(`[
		{"id":"ac1","statement":"主机健康指标已采集并报告","satisfied_by":["collect_metrics"]},
		{"id":"ac2","statement":"健康评估基于采集数据给出结论","satisfied_by":["collect_metrics","assess_health"]}
	]`)

	got := decodePlanAcceptanceCriteria(raw)
	require.Len(t, got, 2)
	require.Equal(t, "ac1", got[0].ID)
	require.Equal(t, "主机健康指标已采集并报告", got[0].Statement)
	require.Equal(t, []string{"collect_metrics", "assess_health"}, got[1].SatisfiedBy)
}

func TestDecodePlanAcceptanceCriteriaToleratesAbsentAndMalformed(t *testing.T) {
	require.Empty(t, decodePlanAcceptanceCriteria(nil))
	require.Empty(t, decodePlanAcceptanceCriteria(json.RawMessage(`null`)))
	// An entry missing satisfied_by is still a criterion; the validator (Task 2)
	// will reject it if needed. Here we only check it does not panic and drops
	// entries with no statement.
	got := decodePlanAcceptanceCriteria(json.RawMessage(`[{"id":"x","satisfied_by":["a"]},{"satisfied_by":["b"]}]`))
	require.Len(t, got, 1)
	require.Equal(t, "x", got[0].ID)
}
```

- [ ] **Step 2: 运行测试，确认它失败**

```bash
go test ./internal/workflow/projectcoordination/ -run TestDecodePlanAcceptanceCriteria -v
```

预期：编译失败 —— `decodePlanAcceptanceCriteria`、`PlanAcceptanceCriterion` 未定义。

- [ ] **Step 3: 定义类型**

追加到 `plan_revision_payload.go`（`PlanRevisionFinalSummaryContract` 之后）:

```go
// PlanAcceptanceCriterion is a plan-level acceptance standard the human reviews and
// approves. SatisfiedBy names the task keys whose produces feed this criterion.
// It is checked for plan-internal integrity the same way blocked_by_keys and
// required_inputs are — see the 2026-07-10 plan-phase refactor spec §4.2/§4.3.
// It is NOT a second capability vocabulary: the keys it references are produced by
// the same planner call, in the same plan, approved by the same human.
type PlanAcceptanceCriterion struct {
	ID          string   `json:"id"`
	Statement   string   `json:"statement"`
	SatisfiedBy []string `json:"satisfied_by"`
}
```

`PlanRevisionPayload`（`:14`）的 `Tasks` 行之后新增:

```go
	PlanAcceptanceCriteria []PlanAcceptanceCriterion `json:"plan_acceptance_criteria,omitempty"`
```

`RouteDecisionPlan`（`planner.go:42`）的 `Tasks` 行之后新增:

```go
	PlanAcceptanceCriteria []PlanAcceptanceCriterion
```

- [ ] **Step 4: 实现 `decodePlanAcceptanceCriteria`**

追加到 `openai_compatible_planner.go`（紧邻 `decodePlannerStringArray`）:

```go
// decodePlanAcceptanceCriteria parses plan-level acceptance criteria from the
// planner output. Entries with no statement are dropped; satisfied_by is trimmed.
func decodePlanAcceptanceCriteria(raw json.RawMessage) []PlanAcceptanceCriterion {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil
	}
	var decoded []struct {
		ID          string          `json:"id"`
		Statement   string          `json:"statement"`
		SatisfiedBy json.RawMessage `json:"satisfied_by"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	out := make([]PlanAcceptanceCriterion, 0, len(decoded))
	for _, entry := range decoded {
		if strings.TrimSpace(entry.Statement) == "" {
			continue
		}
		out = append(out, PlanAcceptanceCriterion{
			ID:          strings.TrimSpace(entry.ID),
			Statement:   strings.TrimSpace(entry.Statement),
			SatisfiedBy: nonNilStrings(decodePlannerStringArray(entry.SatisfiedBy)),
		})
	}
	return out
}
```

- [ ] **Step 5: 接进解析链路**

`plannerJSON` 结构体（`openai_compatible_planner.go:358`，承载 LLM 返回的顶层）新增字段:

```go
	PlanAcceptanceCriteria json.RawMessage `json:"plan_acceptance_criteria"`
```

判据是计划级，不进 `plannerTask`（`:367`）。`decodePlannerStringArray` 已存在，可用于 `satisfied_by`。

两处 `RouteDecisionPlan{...}` 字面量需补 `PlanAcceptanceCriteria`:

1. **`:320`**（主解析路径，变量 `decoded` 即 `plannerJSON`）：

```go
	plan := RouteDecisionPlan{
		Reason:                  decoded.Reason,
		RequiresHumanReview:     decoded.RequiresHumanReview,
		BudgetEstimate:          nonNilMap(decoded.BudgetEstimate),
		TemplateKey:             decoded.TemplateKey,
		PlannerMetadata:         sanitizePlannerMetadata(decoded.PlannerMetadata),
		PlanAcceptanceCriteria:  decodePlanAcceptanceCriteria(decoded.PlanAcceptanceCriteria),
		Tasks:                   make([]PlannedTask, 0, len(decoded.Tasks)),
	}
```

2. **`:680`**（heuristic fallback，单任务「required_review_execute_demand」）：不涉及 LLM 输出，`PlanAcceptanceCriteria` 留 nil 即可——但 `RouteDecisionPlan` 现在有此字段，Go 不强制初始化，nil 等价于空。无需显式写。

校验（Task 2）对 nil/空的判据放行，故 fallback 路径不受影响。

- [ ] **Step 6: 运行测试，确认通过**

```bash
go test ./internal/workflow/projectcoordination/ -run TestDecodePlanAcceptanceCriteria -v
go build ./internal/...
```

预期：PASS。

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/
git commit -m "$(cat <<'EOF'
feat(planning): plans carry plan-level acceptance criteria

Each criterion states an acceptance standard and the task keys whose produces
satisfy it. This is the object humans review and approve, and the anchor plan 5
will use to locate which tasks to re-run when a criterion is not met.

It is not a second capability vocabulary. SatisfiedBy references keys produced by
the same planner call, in the same plan, approved by the same human — checked for
internal integrity the same way blocked_by_keys already is.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: 归属完整性校验

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/graph_validation.go`（`ValidateRouteDecisionGraph`，紧邻 Plan 3 的 `producers`/`required_inputs` 校验）
- Test: `apps/control-plane/internal/workflow/projectcoordination/graph_validation_test.go`

**Interfaces:**
- Consumes: Task 1 的 `PlanAcceptanceCriterion`；Plan 3 的 `producers` 映射构建方式（每 key 唯一生产者）。
- Produces: `ValidateRouteDecisionGraph` 新增拒绝理由 `acceptance_criterion_has_no_satisfier` 与 `satisfied_by_task_not_found`。

- [ ] **Step 1: 写失败测试**

```go
func TestValidateRouteDecisionPlanRejectsCriterionWithUnknownSatisfier(t *testing.T) {
	plan := RouteDecisionPlan{
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "ac1", Statement: "done", SatisfiedBy: []string{"no_such_task"}},
		},
		Tasks: []PlannedTask{
			{Key: "real_task", SelectedEmployeeID: uuid.New(), EmployeeSelectionReason: "only one", SelectionConfidence: 0.9, AcceptanceCriteria: []string{"done"}},
		},
	}
	snapshot := snapshotForPlan(plan)

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12})

	require.Error(t, err)
	require.Contains(t, err.Error(), "no_such_task")
}

func TestValidateRouteDecisionPlanRejectsCriterionWithNoSatisfier(t *testing.T) {
	plan := RouteDecisionPlan{
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "ac1", Statement: "done", SatisfiedBy: nil},
		},
		Tasks: []PlannedTask{
			{Key: "a", SelectedEmployeeID: uuid.New(), EmployeeSelectionReason: "only one", SelectionConfidence: 0.9, AcceptanceCriteria: []string{"done"}},
		},
	}
	snapshot := snapshotForPlan(plan)

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12})

	require.Error(t, err)
}

func TestValidateRouteDecisionPlanAcceptsCriterionWithRealSatisfier(t *testing.T) {
	plan := RouteDecisionPlan{
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "ac1", Statement: "done", SatisfiedBy: []string{"a"}},
		},
		Tasks: []PlannedTask{
			{Key: "a", SelectedEmployeeID: uuid.New(), EmployeeSelectionReason: "only one", SelectionConfidence: 0.9, AcceptanceCriteria: []string{"done"}},
		},
	}
	snapshot := snapshotForPlan(plan)

	require.NoError(t, ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12}))
}
```

`snapshotForPlan` 已存在于 Plan 3 的测试中（`graph_validation_test.go`）。若 `PlanAcceptanceCriteria` 字段在 `PlannedTask` 上不存在（它在 `RouteDecisionPlan` 上），按 Task 1 Step 3 已加在 `RouteDecisionPlan`。

- [ ] **Step 2: 运行测试，确认它失败**

```bash
go test ./internal/workflow/projectcoordination/ -run 'TestValidateRouteDecisionPlanRejectsCriterion|TestValidateRouteDecisionPlanAcceptsCriterionWithRealSatisfier' -v
```

预期：FAIL —— 校验逻辑尚未存在，`NoSatisfier` 用例会 PASS（当前不校验），`UnknownSatisfier` 与 `NoSatisfier` 应为 FAIL。若 `snapshotForPlan` 在本测试文件缺失，从 `plan_revision_payload_test.go` 复制（Plan 3 已定义）。

- [ ] **Step 3: 在 `ValidateRouteDecisionGraph` 中加校验**

定位 Plan 3 引入的 `producers` 映射（已构建 task_key → key 的关系）。在 `required_inputs` 校验之后、`return nil` 之前新增:

```go
	// Plan-level acceptance criteria: each satisfied_by must name a real task key.
	// produces-key uniqueness is already enforced, so a satisfier resolves to one
	// task — but satisfied_by references the task key directly, not a produces key,
	// so we check membership in the task set.
	taskKeys := map[string]struct{}{}
	for _, task := range plan.Tasks {
		taskKeys[task.Key] = struct{}{}
	}
	for _, criterion := range plan.PlanAcceptanceCriteria {
		if len(criterion.SatisfiedBy) == 0 {
			return invalidRouteDecision("plan acceptance criterion %q has no satisfied_by task; a criterion must be backed by at least one task", criterion.ID)
		}
		for _, satisfier := range criterion.SatisfiedBy {
			if _, ok := taskKeys[satisfier]; !ok {
				return invalidRouteDecision("plan acceptance criterion %q satisfied_by %q is not a task in this plan", criterion.ID, satisfier)
			}
		}
	}
```

> **注意**：`satisfied_by` 引用的是 **task_key**（计划级判据归属"哪个任务负责产出这条判据的证据"），不是 produces key。task_key 的唯一性由 DAG 本身保证。这与 `required_inputs`→`produces` 的匹配不同——后者跨字段，前者同字段。如果后续要让 `satisfied_by` 引用 produces key（更细粒度），那是 Plan 5 的设计选择，本 plan 用 task_key 以保持简单。

- [ ] **Step 4: 运行测试，确认通过**

```bash
go test ./internal/workflow/projectcoordination/ -run TestValidateRouteDecisionPlan -v
```

预期：三个新用例 PASS，既有用例不回归。

- [ ] **Step 5: 跑整包**

```bash
go test ./internal/workflow/projectcoordination/
```

预期：PASS。

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/
git commit -m "$(cat <<'EOF'
feat(planning): reject a plan whose acceptance criterion has no satisfier

A plan-level acceptance criterion is what a human approves. If its satisfied_by
names a task that is not in the plan, or names nothing at all, the plan is
inconsistent and must not reach a human as if it were sound. Same class of check
as blocked_by_keys and required_inputs.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: planner 提示词产出判据

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go`（提示词字符串数组）

**Interfaces:**
- Consumes: Task 1。
- Produces: 提示词要求 planner 输出顶层 `plan_acceptance_criteria`。

- [ ] **Step 1: 加提示词**

在 `buildPlannerSystemPrompt` 的字段说明区（`runtime_requirements` 那句之后）追加:

```go
		"plan_acceptance_criteria is a list of plan-level acceptance standards, each with id (short snake_case), statement (one sentence a human can judge), and satisfied_by (the task keys whose work feeds this criterion). Every criterion must be satisfied_by at least one task that exists in the plan. These are what the human owner reviews and approves before execution begins; state them as outcomes, not as steps.",
```

- [ ] **Step 2: 跑 planner 测试**

```bash
go test ./internal/workflow/projectcoordination/ -run Planner -v
```

预期：PASS。既有 fixture 若不带 `plan_acceptance_criteria`，解析时该字段为 nil，校验放行（Task 2 只在判据存在且 satisfied_by 缺失时拒绝；判据整体缺失不在校验范围——一个计划可以暂时无计划级判据，由人类在评审时补）。

> **设计决定**：本 plan **不强制**每个计划必须有 `plan_acceptance_criteria`。理由：planner 可能判断不出判据，强制会让计划频繁被拒。判据缺失时，人类在评审界面看到「本计划未声明验收判据」并自行决定。若要强制非空，是后续产品决定。

- [ ] **Step 3: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go
git commit -m "$(cat <<'EOF'
feat(planner): prompt emits plan-level acceptance criteria

The planner now outputs plan_acceptance_criteria at the top level. Each criterion
states an outcome and the tasks that feed it. Absence is tolerated — a human can
add criteria at review time — but a present criterion with a dangling satisfied_by
is rejected (Task 2).

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: 评审上下文透传判据

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go:2922`（`planRevisionReviewContext`）

**Interfaces:**
- Consumes: Task 1。
- Produces: `planRevisionReviewContext` 新增 `plan_acceptance_criteria` 键。前端 Task 5 读它。

- [ ] **Step 1: 透传**

`planRevisionReviewContext`（`:2922`）的 map 中新增:

```go
		"plan_acceptance_criteria": input.Payload.PlanAcceptanceCriteria,
```

- [ ] **Step 2: 编译并跑包**

```bash
go build ./internal/... && go test ./internal/workflow/projectcoordination/
```

预期：PASS。

- [ ] **Step 3: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/project_store.go
git commit -m "$(cat <<'EOF'
feat(planning): plan review context carries acceptance criteria

The decision context shown to a human reviewer now includes the plan-level
acceptance criteria alongside the tasks. The snapshot fields (title/summary/risk)
are unchanged; this is the payload the UI renders into the two review blocks.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: 前端展示两块确认内容

**Files:**
- Modify: `apps/web/src/features/projects/`（计划评审组件；先 `rg -l "plan_review\|plan-review\|确认项目计划" apps/web/src` 定位）
- Test: 对应 `.test.tsx`

**Interfaces:**
- Consumes: Task 4 的 `plan_acceptance_criteria` 与既有的 `tasks`（含 `blocked_by_keys`、`produces`）。
- Produces: 评审界面新增「调度顺序」与「验收判据」两个区块。

- [ ] **Step 1: 定位评审组件**

```bash
rg -l "plan_review|确认项目计划|plan-review" apps/web/src
```

确认组件路径（如 `project-plan-review-panel.tsx` 或 `project-operational-detail.tsx` 的 plan_review tab）。

- [ ] **Step 2: 阅读现有评审渲染**

确认 `context_payload` / `trace.context_payload` 中 `tasks` 与（Task 4 后的）`plan_acceptance_criteria` 如何到达组件。按 `DESIGN.md` 的 v3 Soft-Flat 规范：实底 `SoftCard`/`WorkSurface`，`StatusPill` 表语义，不引入玻璃。

- [ ] **Step 3: 写组件测试**

新增/扩展测试，断言渲染：
- 调度顺序：按 `blocked_by_keys` 拓扑排序的任务列表，每项显示 title、`selected_employee_id`（或其名称）、`employee_selection_reason`。
- 验收判据：`plan_acceptance_criteria` 逐条显示 `statement` 与 `satisfied_by`（解析为任务 title）。

测试 fixture 用一个含两任务（`a`→`b`，`blocked_by_keys` 体现顺序）和两条判据的计划。

- [ ] **Step 4: 实现渲染**

按 TDD：先让测试失败（断言新文本存在），再实现。遵循 `DESIGN.md`：审计/证据面用实底脆数据面，长内容（判据 statement）用 clamp/展开。

- [ ] **Step 5: 运行 Web 测试**

```bash
corepack pnpm --filter @superteam/web test <组件文件名>
```

预期：PASS（注意：仓库 web 套件有既有的 `route.fulfill` 故障，只跑该文件）。

- [ ] **Step 6: Commit**

```bash
git add apps/web/
git commit -m "$(cat <<'EOF'
feat(web): show task ordering and acceptance criteria in plan review

The plan review decision now presents two blocks: the dispatch order (tasks in
topological order with their executor and reason) and the plan-level acceptance
criteria (each with its statement and backing tasks). A human approving a plan
sees what they are approving, not just a summary line.

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
corepack pnpm verify:web
```

预期：PASS。

- [ ] **Step 2: 重启并加载新代码**

```bash
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh status
```

预期：`control-plane: running ... healthy`。

- [ ] **Step 3: 真实 E2E —— 评审界面看到两块**

提交一个需求，等 plan_review 决策生成。通过 API 取决策上下文:

```bash
DB=$(grep -m1 'url:' apps/control-plane/config/config.yaml | sed 's/.*url: *//;s/"//g')
psql "$DB" -tAc "select context_payload->'plan_acceptance_criteria' from superteam.project_decision_requests where decision_type='plan_review' order by created_at desc limit 1;"
```

预期：非空数组，每条含 `statement` 与 `satisfied_by`（指向真实 task key）。

浏览器打开项目详情的计划评审，确认「调度顺序」与「验收判据」两块渲染（用 codex chrome plug 或 Playwright，按 CLAUDE.md「Web 仿真测试用 codex chrome plug」）。

- [ ] **Step 4: 确认坏判据被拒（单测为主）**

```bash
go test ./internal/workflow/projectcoordination/ -run 'TestValidateRouteDecisionPlanRejectsCriterion' -v
```

预期：PASS。真实 E2E 难以诱导 planner 产出坏判据，单测为准。

- [ ] **Step 5: 清理一次性夹具**

归档测试项目、停用测试员工。

---

## Self-Review

**Spec coverage**

| Spec 章节 | 任务 |
|---|---|
| §4.2 `plan_acceptance_criteria` + `satisfied_by` 新增 | Task 1 |
| §4.2 落库校验：`satisfied_by` 指向真实 task | Task 2 |
| §4.3 人类评审两块（顺序 + 判据） | Task 4（后端透传）+ Task 5（前端） |
| §4.5 `arbiter` 本期只 `human` | Task 1（类型未含 verification_method；留 intent spec） |

**本 plan 不认领**：运行期裁决与图延展（Plan 5）、`verification_method` 注册表（intent spec）、会话降维（Plan 6）、tool 死闸门删除（Plan 8）。

**Type consistency**

- `PlanAcceptanceCriterion{ ID, Statement, SatisfiedBy []string }` 在 Task 1 定义；Task 2、Task 4、Task 5 均消费此类型。
- `RouteDecisionPlan.PlanAcceptanceCriteria` 与 `PlanRevisionPayload.PlanAcceptanceCriteria` 同名同类型，分别承载内存态与持久态。
- `decodePlanAcceptanceCriteria(raw json.RawMessage) []PlanAcceptanceCriterion` 在 Task 1 定义并消费。
- `satisfied_by` 引用 **task_key**（不是 produces key）——Task 2 Step 3 注释明确，与 `required_inputs`→`produces` 的跨字段匹配区分。若 Plan 5 要改用 produces key，是那时的事。
- Task 5 的 `snapshotForPlan` 复用 Plan 3 已定义的同名测试辅助；若在 `graph_validation_test.go` 已存在则不重复定义。

**Placeholder scan**：Task 5 Step 1 的组件路径需 `rg` 定位（真实存在但我不在此臆测文件名），其余无 TBD / TODO。每个改码步骤有完整代码块与预期输出。

**已知风险**

Task 2 的校验**不强制**计划必须有 `plan_acceptance_criteria`（判据缺失时放行）。这是有意的产品决定：planner 可能判不出判据，强制会让计划频繁被拒，且人类可在评审时补。若产品要求强制非空，需另加规则——本 plan 不做。

`satisfied_by` 用 task_key 而非 produces key。优点：简单、与 DAG 一致。缺点：粒度粗——一条判据归属"任务 a"，无法表达"a 的产出 X 是关键"。Plan 5 的图延展若需要更细的归属（重跑 a 的某个 produces 子集而非整个 a），需升级为 produces key 引用。本 plan 留口子，注释已标注。

Task 5（前端）依赖定位现有评审组件。若该组件耦合严重或不存在专门的 plan_review 渲染，Task 5 改动面会偏大——执行者应先 `rg` 确认，必要时拆分。

Task 6 Step 3 的浏览器验证依赖 codex chrome plug 可用；若不可用，标该步为阻塞，不以此阻断整 plan。

E2E 在共享远端 dev 库上创建真实项目并消耗真实 token。Step 5 清理不是可选项。
