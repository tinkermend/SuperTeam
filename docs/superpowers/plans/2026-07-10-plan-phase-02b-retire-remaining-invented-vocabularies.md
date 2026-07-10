# Plan Phase Refactor — Plan 2b: 退役剩余两条虚构词表腿 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `permission_requirements` 停止驱动控制流，并让 `runtime_requirements` 只因**真实的事实不满足**而硬失败、不再因**语法写错**而硬失败。

**Architecture:** `scorePermissionsAndTools` 的 permission 段与 `scoreCapabilities`、tool 段对齐——记录差集供展示，不产生 `HardFailure`。`scoreRuntime` 保留 `provider:<type>` / `runtime_node:<id>` 的真实事实校验与 `provider_status` 硬失败，但把**无法识别的 kind** 从「事实不满足」降级为「无法识别的注解」，并把这两个合法 kind 写进提示词。

**Tech Stack:** Go 1.x + `testify/require`。测试 `go test ./internal/workflow/projectcoordination/`；门禁 `corepack pnpm verify:control-plane`。

## Global Constraints

取自 spec `docs/superpowers/specs/2026-07-10-project-plan-phase-refactor-design.md`（提交 `516124d8`，§1.6 已重写为五条腿）：

- **约束一：闸门只读代码可判定的事实，不读任何 LLM 自述。** 打分器的 `HardFailure` 会归零 `Score` 并强制人工审批（`graph_validation.go:100`），因此它受本约束管辖。
- **`provider_status` 的两处 `HardFailure` 必须保留。** 它来自 runtime 心跳上报的 provider 健康，是事实。
- **`provider:<type>` 与 `runtime_node:<id>` 的不满足必须保留 `HardFailure`。** `profile.RuntimeRequirements.ProviderTypes` 来自员工的执行实例，是事实：把 codex 的任务派给 claude 员工本该被拦。
- `scoreRole` / `scoreLoad` / `scoreReliability` 不产生 `HardFailure`，不动。
- 提交信息末尾附 `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`。

---

## 前置：这是 Plan 2 的第三、第四条腿

`ScorePlanningProfile` 有五个打分维度。一条 `HardFailure` 就把 `Score` 归零（`planning_profile.go:178`）并强制人工审批。这些维度是**并联**的——修一条不够。

| 维度 | 状态 |
|---|---|
| `scoreCapabilities` | Plan 2 已退役 |
| `scorePermissionsAndTools` — tool 段 | `ffbe3132` 已退役 |
| **`scorePermissionsAndTools` — permission 段** | **本 plan Task 1** |
| **`scoreRuntime` — 无法识别的 kind** | **本 plan Task 2** |
| `scoreRuntime` — `provider_status` 与合法 kind 的不满足 | 真实事实，**保留** |

**并联的实证**（两次真实运行）：

```
07-10 22:46   score=0    approval=true     ← 能力已退役，tool 未退役
07-10 23:06   score=80   approval=false    ← tool 也退役后
```

能力退役之后，`tool_requirements` 独自就足以归零。

### permission 腿为什么是空壳

`permission_policy` **没有任何执行侧消费者**：只有 employee CRUD 读写它；`apps/runtime-agent/src/` 里零命中；authz 决策点里零命中。

`digital_employees` 里有 `permission_policy.grants` 的：**0 / 13**。`permissionRequirementSatisfied`（`planning_profile.go:778`）遍历空切片必然返回 false。

而 planner 惯常发明权限名：`file_read`、`code_execution`、`execute_shell_commands`、`read:network`。

**23:06 那次 E2E 通过，是因为该任务的 `permission_requirements` 恰好为 `null`。** 同一批修订里另一个任务带着 `["file_read"]`。判据是靠运气过的。

Runtime 上跑的数字员工是 Claude Code / Codex / OpenCode，其权限边界由 provider 自身的沙箱与审批机制、以及 `risk.approval` 动作级闸门约束，不由一个无人兑现的 `permission_policy` 字段约束。为它维护一套词表只会降低灵活性。

### runtime 腿为什么**不是**纯虚构

这条腿必须外科手术式处理，不能照搬前两条。

`runtimeRequirementSatisfied`（`planning_profile.go:762`）：

```go
switch kind {
case "provider":
	return runtimeProviderReady(runtime.ProviderStatus) && containsString(runtime.ProviderTypes, value)
case "runtime_node":
	return runtime.RuntimeNodeID != "" && normalizePlanningString(runtime.RuntimeNodeID) == value
default:
	return false
}
```

`profile.RuntimeRequirements.ProviderTypes` 来自员工的执行实例——**真实事实**。既有测试 `TestScorePlanningProfileRecordsHardFailures`（`planning_profile_test.go:243`）断言：员工 provider 是 `claude`、任务要求 `provider:codex` → `HardFailure`、`Score=0`。**这是对的，必须保留。**

病在 `default: return false`。`splitRequirement`（`:700`）对 `"codex"` 返回 `kind="codex", value=""`，落进 `default`，被当作「事实不满足」。

**实证**：`07-10 01:22` 的计划修订里 `runtime_requirements: ["codex"]`。模型说对了东西、写错了语法（该写 `provider:codex`），于是那个员工被判 `HardFailure`、`Score=0`、强制审批——**而那个员工恰恰就是 codex**。提示词从未告诉它 `kind:value` 这个格式。

把语法错误判成事实不满足，是本 plan 要修的那一处。

---

## File Structure

| 文件 | 职责 | 动作 |
|---|---|---|
| `apps/control-plane/internal/workflow/projectcoordination/planning_profile.go` | 员工打分 | Task 1：permission 段不再硬失败；Task 2：未知 kind 降级为注解 |
| `apps/control-plane/internal/workflow/projectcoordination/planning_profile_test.go` | 打分单测 | Task 1、2：改写既有断言，新增用例 |
| `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go` | 提示词 | Task 1：permission 建议留空；Task 2：给出 `kind:value` 词表 |

---

### Task 1: permission 不再驱动控制流

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/planning_profile.go:461-477`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go`（提示词，`tool_requirements` 那句之后）
- Test: `apps/control-plane/internal/workflow/projectcoordination/planning_profile_test.go:204`（`TestScorePlanningProfileRecordsHardFailures`）

**Interfaces:**
- Consumes: 无（本 plan 起点）
- Produces: `scorePermissionsAndTools(profile DigitalEmployeePlanningProfile, req PlanningTaskRequirements, result *PlanningProfileScore) int` 签名不变。`result.MatchedPermissions` / `MissingPermissions` 仍被填充（供展示）；`result.HardFailures` **不再**包含 `unsatisfied_permission:*` 或 `permission_or_tool_requirement_unsatisfied`。Task 2 不依赖它。

- [ ] **Step 1: 写失败测试**

追加到 `planning_profile_test.go`：

```go
func TestScorePlanningProfileDoesNotHardFailOnMissingPermission(t *testing.T) {
	profile := DigitalEmployeePlanningProfile{
		DigitalEmployeeID:   uuid.New(),
		RuntimeRequirements: PlanningRuntimeRequirements{ProviderStatus: "ready"},
		// Nothing in the repository ever grants a permission: 0 of 13 employees
		// carry one, and permission_policy has no consumer at all.
		Permissions: nil,
	}

	score := ScorePlanningProfile(profile, PlanningTaskRequirements{
		// Names the planner invents. It was never given a vocabulary.
		PermissionRequirements: []string{"file_read", "code_execution"},
	})

	require.Empty(t, score.HardFailures, "an unmatched permission name is not a hard failure")
	require.Greater(t, score.Score, 0, "score must survive an unmatched permission")
	require.Equal(t, []string{"file_read", "code_execution"}, score.MissingPermissions,
		"the diff is still reported for display")
}
```

同时**改写**既有的 `TestScorePlanningProfileRecordsHardFailures`（`:270-274` 附近）——它断言的正是要删除的行为：

```go
	result = ScorePlanningProfile(profile, PlanningTaskRequirements{
		TaskType:               "database_analysis",
		PermissionRequirements: []string{"database.write:dev_database"},
	})

	require.Empty(t, result.HardFailures, "unmatched permission_requirements must not hard-fail; permission_policy has no consumer")
	require.Equal(t, []string{"database.write:dev_database"}, result.MissingPermissions)
	require.Greater(t, result.Score, 0)
```

- [ ] **Step 2: 运行测试，确认它失败**

```bash
go test ./internal/workflow/projectcoordination/ -run 'TestScorePlanningProfileDoesNotHardFailOnMissingPermission|TestScorePlanningProfileRecordsHardFailures' -v
```

预期：两个都 FAIL —— `HardFailures` 含 `unsatisfied_permission:*`，`Score` 被 `planning_profile.go:178` 归零。

- [ ] **Step 3: 删除 permission 段的两行硬失败**

`planning_profile.go:461-477` 的 permission 循环，删除末尾两行：

```go
		result.MissingPermissions = append(result.MissingPermissions, normalized)
		result.HardFailures = appendUniqueString(result.HardFailures, "permission_or_tool_requirement_unsatisfied")
		result.HardFailures = append(result.HardFailures, "unsatisfied_permission:"+normalized)
```

改为：

```go
		result.MissingPermissions = append(result.MissingPermissions, normalized)
```

并在函数上方补注释（tool 段已有同类注释）：

```go
// permission_requirements are advisory only. permission_policy has no consumer:
// employee CRUD writes it, the runtime never reads it, and the authz decision
// point never reads it. A digital employee's real boundary is the provider's own
// sandbox plus the action-level risk.approval gate. Turning an unmatched name
// into a HardFailure zeroed SelectionScore and forced human approval for a field
// nothing honours. See the 2026-07-10 plan-phase refactor spec §1.6.
```

`permission_or_tool_requirement_unsatisfied` 这个常量此刻已无任何产生者——一并删除该字面量的最后一处使用（tool 段已在 `ffbe3132` 中删除它）。

- [ ] **Step 4: 运行测试，确认通过**

```bash
go test ./internal/workflow/projectcoordination/ -run 'TestScorePlanningProfileDoesNotHardFailOnMissingPermission|TestScorePlanningProfileRecordsHardFailures' -v
```

预期：PASS。

- [ ] **Step 5: 提示词**

在 `openai_compatible_planner.go` 中 `tool_requirements are advisory annotations only...` 那一句之后追加：

```go
		"permission_requirements are advisory annotations only. Prefer an empty array. A digital employee's boundary is enforced by the provider sandbox and by action-level human approval, not by matching permission names at planning time.",
```

- [ ] **Step 6: 跑整包**

```bash
go test ./internal/workflow/projectcoordination/
```

预期：PASS。

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/
git commit -m "$(cat <<'EOF'
fix(planning): permission_requirements no longer disqualifies an employee

permission_policy has no consumer anywhere. Employee CRUD writes it; the runtime
never reads it; the authz decision point never reads it; and 0 of 13 employees
carry a single grant. So every permission name the planner invents — file_read,
code_execution, execute_shell_commands — could only miss, and a miss zeroed
SelectionScore and forced human approval.

The plan-2 acceptance passed only because that task happened to emit no
permission_requirements. Another task in the same plan revision carried
["file_read"].

A digital employee's real boundary is the provider's sandbox and the action-level
risk.approval gate, not a planning-time name match.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: 语法错误不再冒充事实不满足

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/planning_profile.go:125-135`（`PlanningProfileScore`）、`:430-457`（`scoreRuntime`）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go`（提示词）
- Test: `apps/control-plane/internal/workflow/projectcoordination/planning_profile_test.go`

**Interfaces:**
- Consumes: 无（与 Task 1 独立）
- Produces: `PlanningProfileScore` 新增 `UnrecognizedRuntimeRequirements []string \`json:"unrecognized_runtime_requirements,omitempty"\``。新函数 `knownRuntimeRequirementKind(kind string) bool`。`scoreRuntime` 签名不变。**`provider_status` 与合法 kind 不满足的 `HardFailure` 全部保留。**

- [ ] **Step 1: 写失败测试**

```go
func TestScoreRuntimeDoesNotHardFailOnUnrecognizedRequirementKind(t *testing.T) {
	profile := DigitalEmployeePlanningProfile{
		DigitalEmployeeID: uuid.New(),
		RuntimeRequirements: PlanningRuntimeRequirements{
			ProviderTypes:  []string{"codex"},
			ProviderStatus: "ready",
		},
	}

	// The planner named the right thing in the wrong syntax: it should have
	// written provider:codex. The employee IS codex.
	score := ScorePlanningProfile(profile, PlanningTaskRequirements{
		RuntimeRequirements: []string{"codex"},
	})

	require.Empty(t, score.HardFailures, "a malformed requirement is a syntax error, not an unsatisfied fact")
	require.Greater(t, score.Score, 0)
	require.Equal(t, []string{"codex"}, score.UnrecognizedRuntimeRequirements)
	require.Empty(t, score.MissingRuntimeRequirements)
}

func TestScoreRuntimeStillHardFailsOnWellFormedProviderMismatch(t *testing.T) {
	profile := DigitalEmployeePlanningProfile{
		DigitalEmployeeID: uuid.New(),
		RuntimeRequirements: PlanningRuntimeRequirements{
			ProviderTypes:  []string{"claude"},
			ProviderStatus: "ready",
		},
	}

	// Dispatching a codex task to a claude employee is a real mismatch against a
	// real fact. This must keep blocking.
	score := ScorePlanningProfile(profile, PlanningTaskRequirements{
		RuntimeRequirements: []string{"provider:codex"},
	})

	require.Contains(t, score.HardFailures, "runtime_requirement_unsatisfied")
	require.Equal(t, 0, score.Score)
	require.Equal(t, []string{"provider:codex"}, score.MissingRuntimeRequirements)
}

func TestScoreRuntimeStillHardFailsOnUnreadyProvider(t *testing.T) {
	profile := DigitalEmployeePlanningProfile{
		DigitalEmployeeID: uuid.New(),
		RuntimeRequirements: PlanningRuntimeRequirements{
			ProviderTypes:  []string{"codex"},
			ProviderStatus: "unknown",
		},
	}

	score := ScorePlanningProfile(profile, PlanningTaskRequirements{
		RuntimeRequirements: []string{"provider:codex"},
	})

	require.Contains(t, score.HardFailures, "unsatisfied_runtime:provider_status")
	require.Equal(t, 0, score.Score)
}
```

- [ ] **Step 2: 运行测试，确认第一个失败、后两个通过**

```bash
go test ./internal/workflow/projectcoordination/ -run TestScoreRuntime -v
```

预期：`...UnrecognizedRequirementKind` 编译失败（`UnrecognizedRuntimeRequirements` 未定义）；后两个是回归保护，实现后必须仍然 PASS。

- [ ] **Step 3: 新增字段与判别函数**

`planning_profile.go` 的 `PlanningProfileScore`（`:125`），在 `MissingRuntimeRequirements` 下方新增：

```go
	// UnrecognizedRuntimeRequirements are requirement strings whose kind the
	// platform does not know how to evaluate. They are a planner syntax problem,
	// not a fact about the employee, so they never hard-fail and never dilute the
	// score. Display only.
	UnrecognizedRuntimeRequirements []string `json:"unrecognized_runtime_requirements,omitempty"`
```

追加判别函数（紧邻 `runtimeRequirementSatisfied`）：

```go
// knownRuntimeRequirementKind reports whether the platform can evaluate this
// requirement kind at all. It must stay in sync with runtimeRequirementSatisfied.
func knownRuntimeRequirementKind(kind string) bool {
	switch kind {
	case "provider", "runtime_node":
		return true
	default:
		return false
	}
}
```

- [ ] **Step 4: 在 `scoreRuntime` 中分流**

`planning_profile.go:439-452` 的循环，改为：

```go
	for _, requirement := range req.RuntimeRequirements {
		kind, value := splitRequirement(requirement)
		if kind == "" {
			continue
		}
		if !knownRuntimeRequirementKind(kind) {
			// splitRequirement("codex") yields kind="codex", value="". The planner
			// named the right thing in the wrong syntax; that is not evidence the
			// employee lacks anything.
			result.UnrecognizedRuntimeRequirements = append(result.UnrecognizedRuntimeRequirements, normalizeRequirement(requirement))
			continue
		}
		if runtimeRequirementSatisfied(profile.RuntimeRequirements, kind, value) {
			result.MatchedRuntimeRequirements = append(result.MatchedRuntimeRequirements, normalizeRequirement(requirement))
			continue
		}
		missing := normalizeRequirement(requirement)
		result.MissingRuntimeRequirements = append(result.MissingRuntimeRequirements, missing)
		result.HardFailures = appendUniqueString(result.HardFailures, "runtime_requirement_unsatisfied")
		result.HardFailures = append(result.HardFailures, "unsatisfied_runtime:"+missing)
	}
```

函数末尾的 `provider_status` 检查与 `proportionalScore(15, len(Matched), len(Matched)+len(Missing))` **不动**。无法识别项被排除在分母之外，因此不稀释分数：全部无法识别时 `Matched+Missing = 0`，`proportionalScore` 返回权重 `15`。

函数开头 `if len(req.RuntimeRequirements) == 0` 的分支**不动**——它检查的是 `provider_status`，是事实。

- [ ] **Step 5: 运行测试，确认三个都通过**

```bash
go test ./internal/workflow/projectcoordination/ -run TestScoreRuntime -v
```

预期：三个用例全部 PASS。后两个证明真实事实的硬失败没有被误删。

- [ ] **Step 6: 提示词给出词表**

在 `openai_compatible_planner.go` 中追加：

```go
		"runtime_requirements entries must use the form kind:value. Only two kinds are evaluated: provider:<provider_type> such as provider:codex, and runtime_node:<uuid>. A bare token such as codex is not evaluated and is recorded as unrecognized. Prefer an empty array unless the task genuinely requires a specific provider.",
```

- [ ] **Step 7: 跑整包并确认无回归**

```bash
go test ./internal/workflow/projectcoordination/
grep -rn "permission_or_tool_requirement_unsatisfied" apps/control-plane/internal | grep -v _test
```

预期：测试 PASS；`grep` 无输出（该常量已无产生者）。

- [ ] **Step 8: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/
git commit -m "$(cat <<'EOF'
fix(planning): a malformed runtime requirement is a syntax error, not a fact

runtimeRequirementSatisfied evaluates provider:<type> and runtime_node:<id> and
returns false for everything else, so splitRequirement("codex") — kind "codex",
no value — fell through default and was recorded as an unsatisfied fact. The
planner had named the right thing in the wrong syntax, and the employee, which is
in fact codex, was disqualified with SelectionScore 0. The prompt never mentioned
the kind:value form.

Unrecognized kinds are now recorded for display and excluded from the score's
denominator. The prompt states the two evaluable kinds.

What still hard-fails is unchanged and deliberate: a well-formed provider:codex
against a claude employee, and a provider that is not ready. Both are facts the
runtime reports.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: 分层门禁与真实 E2E

**Files:** 无改动。本 task 是验收。

**Interfaces:**
- Consumes: Task 1、Task 2。
- Produces: 可复述的验收结论。

- [ ] **Step 1: 门禁**

```bash
corepack pnpm verify:control-plane
```

预期：PASS。

- [ ] **Step 2: 确认五条腿的现状**

```bash
grep -n "HardFailures = " apps/control-plane/internal/workflow/projectcoordination/planning_profile.go
```

预期：只剩 `scoreRuntime` 中 4 处，全部与 `provider_status` 或合法 kind 的不满足相关。**不得**出现 `unsatisfied_permission` 或 `unsatisfied_tool` 或 `missing_capability`。

- [ ] **Step 3: 重启并加载新代码**

```bash
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh status
```

预期：`control-plane: running ... healthy`。

- [ ] **Step 4: 真实 E2E —— 三个字段同时非空且不再惩罚**

建一个 `external_capabilities` 为空、`permission_policy` 为空的 claude-code 员工，提交一个会让 planner 同时吐出 capability / permission / runtime 三种编造词的需求（例如「读取这台主机的网络配置文件并总结」——历史上此类需求会产出 `file_read`、`read:network`）。

批准计划后查库：

```bash
DB=$(grep -m1 'url:' apps/control-plane/config/config.yaml | sed 's/.*url: *//;s/"//g')
psql "$DB" -tAc "select planner_metadata->'employee_selection'->>'selection_score' as score,
                        requires_human_approval,
                        risk_level
                 from superteam.project_tasks order by created_at desc limit 1;"
```

预期：`score > 0`；`requires_human_approval = false`（除非任务本身高风险）。

**并且**必须确认 planner 这次真的吐出了非空的 `permission_requirements`——否则判据又是靠运气过的，与 Plan 2 的 23:06 一样：

```bash
psql "$DB" -tAc "with t as (select jsonb_array_elements(r.payload->'tasks') as task, r.created_at
                            from superteam.project_plan_revisions r where jsonb_typeof(r.payload->'tasks')='array')
                 select task->'permission_requirements', task->'runtime_requirements'
                 from t order by created_at desc limit 1;"
```

预期：`permission_requirements` 非空。若为空，**本次 E2E 不成立**，改需求重跑，或直接以 Task 1 Step 1 的单测为准并在报告中如实说明。

- [ ] **Step 5: 确认真实事实仍然拦得住**

无法在 E2E 中安全构造「provider 不匹配」的场景（需要一个 provider 与任务要求不符的员工）。以单测为准：

```bash
go test ./internal/workflow/projectcoordination/ -run 'TestScoreRuntimeStillHardFails' -v
```

预期：两个用例 PASS。

- [ ] **Step 6: 清理一次性夹具**

归档测试项目、停用测试员工。共享 dev 库不留活跃垃圾数据。

---

## Self-Review

**Spec coverage**

| Spec 章节 | 任务 |
|---|---|
| §1.6 表格第 3 行（permission 段） | Task 1 |
| §1.6 表格第 4 行（`unsatisfied_runtime:<x>`） | Task 2 |
| §1.6 表格第 5 行（`provider_status`，保留） | Task 2 的回归测试 |
| §5 删除清单新增的两行「待做」 | Task 1, 2 |

**本 plan 不认领**：`produces` / `required_inputs`（Plan 3）、计划级判据（Plan 4）、图延展（Plan 5）、会话降维（Plan 6）、工具死闸门删除（Plan 8，spec §1.7）。

**Type consistency**

- `scorePermissionsAndTools` 与 `scoreRuntime` 签名均不变，只有 `result.HardFailures` 的**内容**收窄。
- `PlanningProfileScore.UnrecognizedRuntimeRequirements []string` 在 Task 2 定义并在同 task 内消费；Task 1 不触碰它。
- `knownRuntimeRequirementKind(kind string) bool` 在 Task 2 定义，与 `runtimeRequirementSatisfied` 的 `switch` 必须保持同步——两者都只认 `provider` 与 `runtime_node`。注释里写明了这条约束。
- Task 1 与 Task 2 **互不依赖**，可乱序执行；但 Task 3 依赖两者。

**Placeholder scan**：无 TBD / TODO。每个改码步骤都有完整代码块与预期输出。

**已知风险**

Task 1 会改写既有的 `TestScorePlanningProfileRecordsHardFailures`。该测试同时覆盖 permission、runtime、profile 三类硬失败；只改 permission 那一段，**不要动 runtime 与 `runtime_contract_missing` 的断言**——后者由 Task 2 的回归测试保护。

Task 2 之后，`proportionalScore(15, ...)` 的分母不再包含无法识别项。若某计划的 `runtime_requirements` **全部**无法识别，该维度返回满分 15。这是有意的：平台无法评价的东西不应扣分，也不应加分——但满分是既有 `total == 0` 分支的行为，此处与之对齐。

Task 3 Step 4 的判据依赖 planner 恰好吐出非空 `permission_requirements`。Plan 2 的 23:06 验收就栽在这里——它通过是因为该任务恰好没有权限要求。若本次仍为空，**必须在报告中如实标注 E2E 未覆盖该路径**，不得默认通过。

Task 3 在共享远端 dev 库上创建真实项目并消耗真实 token。Step 6 的清理不是可选项。
