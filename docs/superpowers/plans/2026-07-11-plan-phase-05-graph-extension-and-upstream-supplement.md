# Plan Phase Refactor — Plan 5: 上游补做与图延展 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让一个被上游产出不足卡住的任务（C），能自动定位到产出缺失输入的任务（owner，通常在上游），追加 owner 的返工任务并重跑下游——而不是停死在 `waiting_human` 或派回 C 自己空转。图仍是 DAG，「循环」只发生在时间和血缘上。

**Architecture:** 两批次，**批次 A 可独立合并，批次 B 依赖 A**。批次 A：`blocked` 申报的 `missing_inputs` 解析 + 新 decision + 新 activity（追加 owner 任务）；批次 B：`plan_iteration` 计数 + 结构化熔断指纹 + 阈值四级回退。

**Tech Stack:** Go 1.x + `testify/require`、Temporal、sqlc、PostgreSQL 迁移。测试 `go test ./internal/...`；门禁 `corepack pnpm verify:control-plane`。

## Global Constraints

取自 spec `docs/superpowers/specs/2026-07-10-project-plan-phase-refactor-design.md`（含历次勘误）:

- **约束一:闸门只读代码可判定的事实。** `missing_inputs` 必须是 C 自己 `input_requirements.required_inputs` 里声明过的 key（Plan 3 引入）——报清单外的 key 是契约违规，转人类。
- **约束二:跨员工交接只经结果契约。** owner 解析靠查计划的 `produces`（Plan 3），不读 transcript，不靠模型判断「谁的错」。
- **不对称原则（§4.6）:** 允许申报「我卡住了」（无收益，有界），禁止自述「我完成了」。`missing_inputs` 是前者。
- **熔断后平台不猜（§4.8）:** 连续两次相同失败指纹即转人类，平台不自行决定改派 A。
- **`task.accepted_plan_revision_changed`（terminal）必须放行 iteration N 的追加节点**——否则 loop 一启动就自杀。只有判据集合变化才回人类。
- 提交信息末尾附 `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`。

---

## 前置:机制现状（大量可复用）

审查运行期代码后，Plan 5 **不是从零搭**，下列已存在:

| 机制 | 位置 | 状态 |
|---|---|---|
| `RevisionAttempt` decision + `createRevisionTaskForResult` | `workflow.go:511`，`project_store.go:701` | 已在用，**但派给 `source.AssignedDigitalEmployeeID`（同一个人）**——这是要改的 |
| `resolveReadyDownstream` | `workflow.go:774` | 已在用，完成一个任务后释放下游 |
| `dispatchProjectTasks` | `workflow.go:786` | 已在用 |
| `revisionRootTaskID` 血缘 | `project_store.go:3117` | 已在用，单任务返工计数靠它 |
| `revisionMaxAttempts` 三级回退 | `project_store.go:3137` | 已在用 |
| `repeatedRevisionFailure` 指纹 | `project_store.go:3168` | 已在用，**但指纹含模型散文（`Summary`）**——批次 B 修 |
| `TaskResultBlocker.RequiredBy` | `task_result_contract.go:201` | 自由文本，**要加结构化 `missing_inputs`** |

**Plan 5 要新增的，只有这几样:**
1. `Blocker.MissingInputs []string`（结构化，取代自由文本 `RequiredBy` 的判决作用）。
2. `TaskResultDecisionBlockedResolvableUpstream`（新 decision 值）。
3. `CreateUpstreamSupplementTasks` activity（受派人是 owner，不是 source）。
4. 迁移 054 `project_tasks.plan_iteration`（图级延展计数）。
5. 结构化熔断指纹（已由 Plan 1 部分完成，扩展 `missing_inputs` 与 `satisfied_by`）。
6. `max_plan_iterations` 四级回退。

---

## Batch A: 阻塞解析与上游补做（可独立合并）

### Task A1: `Blocker` 携带结构化 `missing_inputs`

**Files:**
- Modify: `apps/control-plane/internal/project/task_result_contract.go:198`（`TaskResultBlocker`）
- Modify: `apps/control-plane/internal/project/task_result_contract.go:563`（`mapTaskResultDecision` 的 `blocked` 分支）
- Test: `apps/control-plane/internal/project/task_result_contract_test.go`

**Interfaces:**
- Consumes: Plan 3 的 `input_requirements.required_inputs`（计划里声明过）。
- Produces: `TaskResultBlocker.MissingInputs []string \`json:"missing_inputs,omitempty"\``；新 decision 值 `TaskResultDecisionBlockedResolvableUpstream`。Task A2 消费。

- [ ] **Step 1: 写失败测试**

```go
func TestMapTaskResultDecisionBlockedResolvableWhenMissingInputsKnown(t *testing.T) {
	task := project.ProjectTask{
		InputRequirements: map[string]any{"required_inputs": []any{"load_test_report"}},
	}
	// A future-state where the blocker's missing_inputs intersects the declared
	// required_inputs. Resolution is mechanical: the platform can find the owner.
	result := project.TaskResultContract{
		Status:  project.TaskResultStatusBlocked,
		Summary: "no load test report",
		Blocker: &project.TaskResultBlocker{
			Reason:        "no load test report",
			MissingInputs: []string{"load_test_report"},
		},
	}

	decision := mapTaskResultDecision(task, result)
	require.Equal(t, project.TaskResultDecisionBlockedResolvableUpstream, decision)
}

func TestMapTaskResultDecisionBlockedFallsBackToHumanWhenInputNotDeclared(t *testing.T) {
	// The employee invents an input name it never declared in required_inputs.
	// That is a contract violation, not something to resolve automatically.
	task := project.ProjectTask{
		InputRequirements: map[string]any{"required_inputs": []any{"load_test_report"}},
	}
	result := project.TaskResultContract{
		Status:  project.TaskResultStatusBlocked,
		Summary: "need something else",
		Blocker: &project.TaskResultBlocker{
			Reason:        "need something else",
			MissingInputs: []string{"undisclosed_thing"},
		},
	}

	decision := mapTaskResultDecision(task, result)
	require.Equal(t, project.TaskResultDecisionBlockedWaitingHuman, decision)
}
```

`mapTaskResultDecision` 若是未导出函数，测试放 `project` 包内（`task_result_contract_test.go`）。

- [ ] **Step 2: 运行测试，确认失败**

```bash
go test ./internal/project/ -run TestMapTaskResultDecisionBlocked -v
```

预期：编译失败——`MissingInputs` 字段、`TaskResultDecisionBlockedResolvableUpstream` 未定义。

- [ ] **Step 3: 加字段与 decision 常量**

`TaskResultBlocker`（`:198`）:

```go
type TaskResultBlocker struct {
	Reason           string          `json:"reason,omitempty"`
	ResolutionPrompt string          `json:"resolution_prompt,omitempty"`
	RequiredBy       string          `json:"required_by,omitempty"`
	// MissingInputs are produces-keys the employee declares it needs but did not
	// receive. Each must appear in this task's input_requirements.required_inputs
	// (Plan 3); the platform resolves the owner by lookup, never by asking a model
	// who is at fault. See the 2026-07-10 plan-phase refactor spec §4.6(a).
	MissingInputs    []string        `json:"missing_inputs,omitempty"`
	ContextRefs      []TaskResultRef `json:"context_refs,omitempty"`
}
```

在 `TaskResultDecision` 常量块新增:

```go
	// BlockedResolvableUpstream means the employee is starved by an upstream task's
	// output. The platform appends the owner (plus downstream) rather than waiting
	// on a human or bouncing back to the same employee.
	TaskResultDecisionBlockedResolvableUpstream TaskResultDecision = "blocked_resolvable_upstream"
```

- [ ] **Step 4: 在 `mapTaskResultDecision` 的 `blocked` 分支分流**

将 `case TaskResultStatusBlocked: return TaskResultDecisionBlockedWaitingHuman`（`:563`）替换:

```go
	case TaskResultStatusBlocked:
		// Every missing input must be one this task declared in required_inputs.
		// An undeclared name is a contract violation -> human.
		if result.Blocker != nil && len(result.Blocker.MissingInputs) > 0 {
			declared := stringSetFromAny(task.InputRequirements["required_inputs"])
			allDeclared := true
			for _, missing := range result.Blocker.MissingInputs {
				if !declared[missing] {
					allDeclared = false
					break
				}
			}
			if allDeclared {
				return TaskResultDecisionBlockedResolvableUpstream
			}
		}
		return TaskResultDecisionBlockedWaitingHuman
```

`stringSetFromAny` 已存在（`task_result_contract.go` 内，`validateCompletedTaskResult` 用过）。

- [ ] **Step 5: 运行测试，确认通过**

```bash
go test ./internal/project/ -run TestMapTaskResultDecisionBlocked -v
go test ./internal/project/
```

预期：PASS。

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/project/
git commit -m "$(cat <<'EOF'
feat(result): a blocked task can declare which inputs it is missing

A blocked employee names the produces-keys it needs in Blocker.MissingInputs.
Every name must be one this task declared in input_requirements.required_inputs
(Plan 3); otherwise it is a contract violation and goes to a human. When all are
declared, the decision becomes BlockedResolvableUpstream so the platform can
append the owner rather than bouncing back to the same starved employee.

This is the half of §1.3 that CreateRevisionTaskForResult could not do: it
always reassigned the same employee, so a starved C redid C with the same input
until the budget ran out.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task A2: 上游补做 activity

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`（新增 `CreateUpstreamSupplementTasks`）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow.go`（新增 activity 包装 + 在 `handleEmployeeTaskCompleted` 加分支）
- Test: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`

**Interfaces:**
- Consumes: Task A1 的 `Blocker.MissingInputs` 与 `TaskResultDecisionBlockedResolvableUpstream`；Plan 3 的 `produces` 映射；`CreateProjectTaskRequest`（`repository.go:265`）。
- Produces: `CreateUpstreamSupplementTasks(input) ([]uuid.UUID, error)` —— 返回新创建的 owner 任务 id（含下游）。受派人是 owner，不是 source。

- [ ] **Step 1: 写失败测试**

```go
func TestCreateUpstreamSupplementTasksDispatchesToOwner(t *testing.T) {
	// Plan: a produces "load_test_report"; b requires it and is now blocked.
	// The supplement must create a' for owner a, not for b.
	// （用现有 ProjectStore 测试设施构造此计划；具体 fixture 见 project_store_test.go 既有的 setup 模式）
	ownerID := uuid.New()
	sourceEmployeeID := uuid.New()

	result := s.CreateUpstreamSupplementTasks(ctx, CreateUpstreamSupplementInput{
		TenantID:     tenantID,
		ProjectID:    projectID,
		SourceTaskID: sourceTaskID, // task b
		MissingInputs: []string{"load_test_report"},
	})

	// The created task's assigned employee is the owner, not the blocked employee.
	require.NotEmpty(t, result.TaskIDs)
	for _, id := range result.TaskIDs {
		task := mustGetTask(t, s, id)
		require.Equal(t, ownerID, *task.AssignedDigitalEmployeeID)
	}
}
```

> **执行者注意**：上面的 fixture 需构造一个含 `a→b`、`a.produces=[load_test_report]`、`b.required_inputs=[load_test_report]` 的计划并落库。参照 `project_store_test.go` 既有 setup（`TestCreateRevisionTaskForResult...` 系列）的模式。若 fixture 过重，先写一个单元级测试覆盖 owner 解析函数 `resolveOwnersForInputs(plan, missingInputs) []string`，再在集成测试里验证受派人。

- [ ] **Step 2: 实现 owner 解析与 activity**

`project_store.go` 新增:

```go
// CreateUpstreamSupplementInput describes a blocked task and the inputs it lacks.
type CreateUpstreamSupplementInput struct {
	TenantID     uuid.UUID
	ProjectID    uuid.UUID
	SourceTaskID uuid.UUID
	MissingInputs []string
}

// CreateUpstreamSupplementResult reports the newly created owner tasks.
type CreateUpstreamSupplementResult struct {
	TaskIDs []uuid.UUID
}

// CreateUpstreamSupplementTasks finds, for each missing input, the task whose
// produces contains it (Plan 3 guarantees exactly one producer), and appends a
// revision task assigned to that owner — NOT to the blocked source. Downstream
// re-runs are the caller's responsibility (resolveReadyDownstream), since the
// source itself is downstream of the owner.
func (s *ProjectStore) CreateUpstreamSupplementTasks(ctx context.Context, input CreateUpstreamSupplementInput) (CreateUpstreamSupplementResult, error) {
	if s.repository == nil {
		return CreateUpstreamSupplementResult{}, ErrActivityStoreRequired
	}
	source, err := s.repository.GetProjectTask(ctx, input.TenantID, input.SourceTaskID)
	if err != nil {
		return CreateUpstreamSupplementResult{}, err
	}
	// Build produces-key -> owner task, across the plan.
	siblings, err := s.repository.ListProjectTasksByCoordinationJob(ctx, input.TenantID, input.ProjectID, *source.CoordinationJobID)
	if err != nil {
		return CreateUpstreamSupplementResult{}, err
	}
	owners := map[string]project.ProjectTask{} // produces-key -> task
	for _, t := range siblings {
		for _, key := range plannerProducesFromMetadata(t.PlannerMetadata) {
			owners[key] = t
		}
	}
	seen := map[uuid.UUID]struct{}{}
	var taskIDs []uuid.UUID
	for _, missing := range input.MissingInputs {
		owner, ok := owners[missing]
		if !ok {
			// Plan 3 guarantees a producer exists; missing here means plan drift.
			return CreateUpstreamSupplementResult{}, project.ErrInvalidProject
		}
		if _, dup := seen[owner.ID]; dup {
			continue
		}
		seen[owner.ID] = struct{}{}
		created, err := s.repository.CreateProjectTask(ctx, project.CreateProjectTaskRequest{
			TenantID:                  input.TenantID,
			ProjectID:                 input.ProjectID,
			DemandID:                  source.DemandID,
			Title:                     owner.Title,
			Summary:                   "上游补做：" + strings.Join(input.MissingInputs, ", "),
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: owner.AssignedDigitalEmployeeID,
			CoordinationJobID:         source.CoordinationJobID,
			RouteDecisionID:           source.RouteDecisionID,
			PlannedTaskKey:            owner.PlannedTaskKey,
			TaskKind:                  owner.TaskKind,
			StageIndex:                owner.StageIndex,
			RevisionOfTaskID:          &owner.ID,
			AcceptedPlanRevisionID:    source.AcceptedPlanRevisionID,
			ExpectedOutputs:           owner.ExpectedOutputs,
			InputRequirements:          owner.InputRequirements,
			HandoffContract:           owner.HandoffContract,
			PlannerMetadata:           s.revisionPlannerMetadataForSupplement(owner, input.MissingInputs),
			BlockedByTaskIDs:          nil,
		})
		if err != nil {
			return CreateUpstreamSupplementResult{}, err
		}
		taskIDs = append(taskIDs, created.ID)
	}
	return CreateUpstreamSupplementResult{TaskIDs: taskIDs}, nil
}
```

辅助 `plannerProducesFromMetadata`（若 Plan 3 未暴露，从 `planner_metadata.produces` 读出）:

```go
func plannerProducesFromMetadata(metadata map[string]any) []string {
	raw, _ := metadata["produces"].([]any)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
```

`revisionPlannerMetadataForSupplement` 复用既有 `revisionPlannerMetadata`（`project_store.go` 内）的思路，附加 `supplement_for`、`missing_inputs`。

`workflow.go` 新增 activity 包装（仿 `createRevisionTaskForResult`）:

```go
func (a *Activities) CreateUpstreamSupplementTasks(ctx context.Context, input CreateUpstreamSupplementInput) (CreateUpstreamSupplementResult, error) {
	if a.store == nil {
		return CreateUpstreamSupplementResult{}, ErrActivityStoreRequired
	}
	return a.store.CreateUpstreamSupplementTasks(ctx, input)
}

func createUpstreamSupplementTasks(ctx workflow.Context, tenantID, projectID, sourceTaskID uuid.UUID, missingInputs []string) (CreateUpstreamSupplementResult, error) {
	var result CreateUpstreamSupplementResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).CreateUpstreamSupplementTasks, CreateUpstreamSupplementInput{
		TenantID: tenantID, ProjectID: projectID, SourceTaskID: sourceTaskID, MissingInputs: missingInputs,
	}).Get(ctx, &result); err != nil {
		return CreateUpstreamSupplementResult{}, err
	}
	return result, nil
}
```

- [ ] **Step 3: 在 `handleEmployeeTaskCompleted` 加分支**

`workflow.go:518` 的 `RevisionAttempt` 分支之后、`:534` 的 fallthrough 之前，新增:

```go
	if decision.Decision == string(project.TaskResultDecisionBlockedResolvableUpstream) {
		blocker := decision.Blocker // 需让 inspectTaskResultDecision 携带 Blocker，见 Step 4
		supplement, err := createUpstreamSupplementTasks(ctx, input.TenantID, input.ProjectID, signal.ProjectTaskID, blocker.MissingInputs)
		if err != nil {
			return taskCompletionPending{}, err
		}
		// Dispatch the owner(s); the source (b) is downstream and re-runs via
		// resolveReadyDownstream when a' completes.
		return taskCompletionPending{}, dispatchProjectTasks(ctx, input.TenantID, input.ProjectID, supplement.TaskIDs, project.DispatchReasonRetry)
	}
```

- [ ] **Step 4: `inspectTaskResultDecision` 携带 `Blocker`**

定位 `inspectTaskResultDecision`（`workflow.go`），让它的返回结构含 `Blocker *project.TaskResultBlocker`，从 result contract 填入。Task A1 的 `mapTaskResultDecision` 已能据此分流。

- [ ] **Step 5: 运行测试**

```bash
go test ./internal/workflow/projectcoordination/ -run UpstreamSupplement -v
go test ./internal/...
```

预期：PASS。

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/
git commit -m "$(cat <<'EOF'
feat(coordination): append the owner when a task is starved upstream

CreateUpstreamSupplementTasks resolves each missing input to the task that
produces it (Plan 3's uniqueness guarantee) and appends a revision task assigned
to that owner — not to the blocked source. The source, being downstream, re-runs
through resolveReadyDownstream once the owner completes.

This fixes the §1.3 dead loop: a starved C was reassigned to C with the same
starved input until the attempt budget ran out. Now B is asked to supply what C
declared it needed.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task A3: 派发闸门放行 iteration N 的追加节点

**Files:**
- Modify: `apps/control-plane/internal/project/predispatch_gate.go`（`task.accepted_plan_revision_changed` 检查）

**Interfaces:**
- Consumes: Task A2 的 `RevisionOfTaskID` 血缘。
- Produces: 追加节点（`RevisionOfTaskID` 非空）不触发 `replan_required`。

- [ ] **Step 1: 定位检查**

```bash
grep -n "accepted_plan_revision_changed" apps/control-plane/internal/project/predispatch_gate.go
```

- [ ] **Step 2: 放行**

该检查现为「task 的 `AcceptedPlanRevisionID` 必须等于当前接受的版本」。追加节点继承 source 的 `AcceptedPlanRevisionID`（Task A2 透传），故本就该匹配——**确认它确实透传了，无需改闸门**。若闸门额外比对 `PlannedTaskKey` 导致新节点被误判，加例外: `RevisionOfTaskID != nil` 时跳过该检查。

- [ ] **Step 3: 测试**

```go
// 一个 RevisionOfTaskID 非空、AcceptedPlanRevisionID 匹配的任务，不应被该检查阻断。
```

- [ ] **Step 4: Commit**

```bash
git add apps/control-plane/internal/project/predispatch_gate.go
git commit -m "$(cat <<'EOF'
fix(gate): a supplement task is not a plan revision change

An appended owner task inherits the source's AcceptedPlanRevisionID and carries a
RevisionOfTaskID. The accepted_plan_revision_changed check must not treat that as
a drift requiring replan — iteration N appends within the same plan revision.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

## Batch B: 延展计数、熔断、阈值（依赖 Batch A）

### Task B1: 迁移 054 `plan_iteration`

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/054_project_task_plan_iteration.sql`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`

**Interfaces:**
- Consumes: 无。
- Produces: `project_tasks.plan_iteration INTEGER NOT NULL DEFAULT 0`。

- [ ] **Step 1: 写迁移**

```sql
-- Graph-level extension counter. 0 = original plan; N = appended in the Nth
-- extension round. Per-task rework (revision of the same task) is tracked via
-- RevisionOfTaskID lineage; this counts whole-graph extension rounds, which
-- lineage cannot. See the 2026-07-10 plan-phase refactor spec §4.8.
ALTER TABLE project_tasks
    ADD COLUMN plan_iteration INTEGER NOT NULL DEFAULT 0;

COMMENT ON COLUMN project_tasks.plan_iteration IS 'Graph extension round: 0 for the original plan, N for tasks appended in the Nth round.';
```

- [ ] **Step 2: 更新 atlas.sum**

```bash
atlas migrate hash --dir file://apps/control-plane/internal/storage/migrations
```

- [ ] **Step 3: 校验**

```bash
make -C apps/control-plane migrate-validate
```

> 若本地无 docker，用 `atlas migrate validate --dir file://apps/control-plane/internal/storage/migrations`（仅目录+sum 完整性）。

- [ ] **Step 4: Commit**

```bash
git add apps/control-plane/internal/storage/migrations/
git commit -m "$(cat <<'EOF'
db: add plan_iteration to count graph extension rounds

RevisionOfTaskID lineage tracks rework of one task, but cannot group "all tasks
appended in the Nth extension round." plan_iteration does that: 0 for the original
plan, N for tasks appended in round N.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task B2: 结构化熔断指纹（扩展 `missing_inputs` / `satisfied_by`）

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go:3043`（`revisionFailureFingerprint`，Plan 1 已改为只取 `status`+`RequestedChanges`）
- Test: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`

**Interfaces:**
- Consumes: Task A1 的 `Blocker.MissingInputs`。
- Produces: 指纹含 `status` + sorted(`RequestedChanges`) + sorted(`MissingInputs`)。

- [ ] **Step 1: 测试**

```go
func TestRevisionFailureFingerprintCoversMissingInputs(t *testing.T) {
	a := project.TaskResultContract{
		Status:  project.TaskResultStatusBlocked,
		Blocker: &project.TaskResultBlocker{MissingInputs: []string{"x", "y"}},
	}
	b := project.TaskResultContract{
		Status:  project.TaskResultStatusBlocked,
		Blocker: &project.TaskResultBlocker{MissingInputs: []string{"y", "x"}},
	}
	require.Equal(t, revisionFailureFingerprint(a), revisionFailureFingerprint(b))

	// Different inputs -> different fingerprint.
	c := project.TaskResultContract{
		Status:  project.TaskResultStatusBlocked,
		Blocker: &project.TaskResultBlocker{MissingInputs: []string{"z"}},
	}
	require.NotEqual(t, revisionFailureFingerprint(a), revisionFailureFingerprint(c))
}
```

- [ ] **Step 2: 扩展指纹**

`revisionFailureFingerprint` 在 Plan 1 基础上，`RevisionRequest` 段之后新增 `Blocker` 段:

```go
	if contract.Blocker != nil {
		inputs := append([]string(nil), contract.Blocker.MissingInputs...)
		sort.Strings(inputs)
		parts = append(parts, inputs...)
	}
```

- [ ] **Step 3: 测试 + 提交**

```bash
go test ./internal/workflow/projectcoordination/ -run TestRevisionFailureFingerprint -v
git commit -am "$(cat <<'EOF'
fix(coordination): fingerprint blocked failures by missing inputs

A blocked task blocked on the same inputs is the same failure. Plan 1 removed
the model prose (Summary) from the fingerprint; this adds the structured
MissingInputs so a recurring upstream-starvation loop trips the breaker.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task B3: `max_plan_iterations` 四级回退

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`（新增 `maxPlanIterations` + 在 `CreateUpstreamSupplementTasks` 前检查）
- Modify: `apps/control-plane/internal/project/handler.go`（`coordination_policy` 已透传，确认 `max_plan_iterations` 可读）

**Interfaces:**
- Consumes: `projects.coordination_policy.max_plan_iterations`（已装配进 `CoordinationSnapshot.CoordinationPolicy`，`planner.go:16`）。
- Produces: `maxPlanIterations(task project.ProjectTask, policy map[string]any) int`；超过即 `CreateUpstreamSupplementResult{Exhausted: true}`。

- [ ] **Step 1: 测试**

```go
func TestMaxPlanIterationsFallsBackToDefault(t *testing.T) {
	require.Equal(t, defaultMaxPlanIterations, maxPlanIterations(nil))
	require.Equal(t, 5, maxPlanIterations(map[string]any{"max_plan_iterations": 5}))
	require.Equal(t, defaultMaxPlanIterations, maxPlanIterations(map[string]any{"max_plan_iterations": "bad"}))
}
```

- [ ] **Step 2: 实现**

```go
const defaultMaxPlanIterations = 3

func maxPlanIterations(policy map[string]any) int {
	switch v := policy["max_plan_iterations"].(type) {
	case float64:
		if v > 0 {
			return int(v)
		}
	case json.Number:
		if n, err := v.Int64(); err == nil && n > 0 {
			return int(n)
		}
	}
	return defaultMaxPlanIterations
}
```

在 `CreateUpstreamSupplementTasks` 开头（`source` 取出后）:

```go
	if s.currentPlanIteration(siblings) >= maxPlanIterations(snapshotCoordinationPolicy(...)) {
		return CreateUpstreamSupplementResult{Exhausted: true}, nil
	}
```

`currentPlanIteration` 取 `max(plan_iteration)` across siblings。`snapshotCoordinationPolicy` 从快照取——若 `CreateUpstreamSupplementTasks` 拿不到快照，把 `maxPlanIterations` 作为入参从 workflow 层传入（workflow 有 `CoordinationSnapshot`）。

- [ ] **Step 3: 工作流处理 `Exhausted`**

`handleEmployeeTaskCompleted` 的 `BlockedResolvableUpstream` 分支，`supplement.Exhausted` 为真时走人类评审（仿 `:524` 的 `requestProjectTaskIterationExhaustedReview`）。

- [ ] **Step 4: 测试 + 提交**

```bash
go test ./internal/...
git commit -am "$(cat <<'EOF'
feat(coordination): bound graph extension with max_plan_iterations

CreateUpstreamSupplementTasks stops appending once the graph has extended
max_plan_iterations rounds. The threshold reads from
projects.coordination_policy (a live field Plan 2 already assembles), with a
constant floor. Exhaustion routes to a human, who decides whether to replan.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task B4: 分层门禁与真实 E2E

- [ ] **Step 1: 门禁**

```bash
corepack pnpm verify:control-plane
```

- [ ] **Step 2: 真实 E2E —— 上游补做链路**

构造 `a→b`、`a.produces=[load_test_report]`、`b.required_inputs=[load_test_report]` 的项目。让 b 执行后申报 `blocked{missing_inputs:[load_test_report]}`（可能需在 b 的 prompt 里指示它「若缺输入就报 blocked」）。

预期（库验证）:

```bash
psql "$DB" -tAc "select id, assigned_digital_employee_id, revision_of_task_id, plan_iteration from superteam.project_tasks where project_id='...' order by created_at;"
```

- 出现一个 `revision_of_task_id` 指向 a、`assigned_digital_employee_id` = a 的员工、`plan_iteration=1` 的新任务。
- 该任务的受派人**是 a 的员工，不是 b 的**（核心判据，修 §1.3 死循环）。

- [ ] **Step 3: 熔断验证**

人为制造同一 `missing_inputs` 的连续两次 blocked → 第三次走人类评审。

- [ ] **Step 4: 清理夹具**

---

## Self-Review

**Spec coverage**

| Spec 章节 | 任务 |
|---|---|
| §4.6(a) 阻塞申报 → 追加 owner + 下游 | A1, A2, A3 |
| §4.6(c) attestation 不支持断言 | **未纳入**——依赖证据地基 spec 的读路径，Plan 5 不做 |
| §4.7 人类裁决触发延展 | B3（Exhausted 路人类）；判据级裁决的延展属 Plan 4+5 协同 |
| §4.8 `plan_iteration` + 四级回退 + 指纹熔断 | B1, B2, B3 |
| §1.3 修复返工派回同一人 | A2 核心判据 |

**批次依赖**：A 可独立合并（A1→A2→A3 顺序）；B 依赖 A（B2 用 A1 的 MissingInputs，B3 在 A2 的 activity 里加阈值）。建议合并顺序：A 先（即便 B 未完成，blocked 至少能自动补做、不再死循环），B 后（加边界）。

**Type consistency**：
- `TaskResultBlocker.MissingInputs []string` 在 A1 定义，A2、B2 消费。
- `TaskResultDecisionBlockedResolvableUpstream` 在 A1 定义，A2 消费。
- `CreateUpstreamSupplementInput/Result` 在 A2 定义，B3 扩展（加 `Exhausted`）。
- `maxPlanIterations`、`currentPlanIteration` 在 B3 定义。

**Placeholder scan**：Task A2 Step 1 的 fixture 标注「参照既有 setup」，因集成测试构造计划较重，执行者据实调整；其余步骤有完整代码。

**已知风险**：
- Task A2 的 owner 解析遍历 `planner_metadata.produces`。若某历史计划的 `produces` 未落库（Plan 3 之前的计划），解析会漏。这是预期的——只对 Plan 3 之后的计划生效。
- Task B3 的 `maxPlanIterations` 需要 `CoordinationSnapshot.CoordinationPolicy`。若 `CreateUpstreamSupplementTasks` 在 activity 层拿不到快照，阈值要作为入参从 workflow 传入——执行者确认数据可用性后选一种。
- E2E 依赖诱导 b 申报 blocked，planner/employee 行为不完全可控，Step 2 可能需多次尝试；若不可控，以 A1 的单测（decision 分流）+ A2 的单测（owner 解析）为准，E2E 标「尽力验证」。
- 这是整个重构最大的 plan，Temporal workflow 改动（A2 Step 3）有回归风险，必须跑 `verify:control-plane` 全量。
