# 交接契约执行闭环 P1 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 落地 `docs/superpowers/specs/2026-07-13-handoff-contract-execution-loop-design.md` P1：直接前驱 result 注入派工单、图校验收紧到直接 blocker、result_contract 新增 deliverables、生产者侧回写履约核对。

**Architecture:** 复用既有机制，不新建表/端点/迁移。需求侧沿用 `input_requirements.required_inputs`（已端到端存在），供给侧沿用 `produces`（PlannedTask 字段 + ProjectTask.PlannerMetadata），新增的只有：result_contract 的 `deliverables` 载荷（Go+Rust 两侧）、图校验从"祖先"收紧为"直接 blocker"、派发时注入直接 blocker 的 result、回写时 produces⊆deliverables 核对（缺项走**既有** rejected+waitHuman 路径）。

**Tech Stack:** Go（control-plane）、Rust（runtime-agent 契约透传）、无契约/DB 变更。

## Global Constraints

- 验证走 `corepack pnpm verify:control-plane`（contracts + go test）与 `corepack pnpm verify:runtime-agent`；单测内循环 `go test ./apps/control-plane/internal/<pkg>/ -run <Test>` 或 `cargo test --manifest-path apps/runtime-agent/Cargo.toml <name>`。
- **不新增表、不新增端点、不改 openapi**（deliverables 活在 contract_payload jsonb 与已有 result_contract 字段内）。
- 注入范围 = **直接 blocker**，绝不做传递闭包（spec §3.1）。
- 注入失败不得阻断派发（spec §5）；summary 注入 4KB 截断。
- 缺项履约 P1 语义 = 校验失败 → 既有 `recordRejectedProjectTaskAttemptResultAndWaitHuman`（**不判 completed**，人类可见精确缺项清单）；自动返工（revision/supplement 接线）留 P2。此为对 spec §4.4 的修订（原文写接补做循环），理由：补做机制的触发语义是"下游被饿"，生产者自身未履约的返工属 revision 家族，且宪法要求"测试失败后的业务判断暂停等人"。Task 6 同步修订 spec。
- git 提交尾注 `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。

## 已核实的代码事实（写码前不要重新怀疑）

| 事实 | 位置 |
|---|---|
| 图校验 required_inputs⊆祖先 produces 已存在 | `graph_validation.go:184-195`（`ancestorKeys` + `plannerRequiredInputs`） |
| produces 唯一生产者校验已存在 | `graph_validation.go:171-183` |
| `ProjectTask.PlannerMetadata map[string]any` | `project/types.go:601` |
| `plannerProducesFromMetadata(task.PlannerMetadata)` 已存在（projectcoordination 包） | `project_store.go:791` 调用处 |
| `ValidateTaskResultContract(task ProjectTask, result TaskResultContract) TaskResultValidation` | `task_result_contract.go:243` |
| 校验失败 → rejected+waitHuman 已存在 | `service.go:2757-2759` |
| 派发两路径：A `DispatchProjectTask`(:2270) prompt@:2413 packet@:2368；B `resumeQueuedProjectTaskRunStart`(:2462) prompt@:2520 packet@:2532（仅存量 packet 为空时重建） | `projectcoordination/project_store.go` |
| 直接 blocker 查询 `ListProjectTaskDependencies(ctx,tenant,project,[]uuid{taskID}) []ProjectTaskDependency{BlockerTaskID,...}` | `project/repository.go:50` |
| 最新 result：`s.latestTaskResult(ctx, task)`（读 `task.LatestTaskResultID` + `ListProjectTaskResults`） | `project_store.go:1825` |
| Rust `TaskResultContract` 强类型（Serialize+Default，字段不含 deliverables → **会静默丢字段**） | `controlplane/models.rs:209-236` |
| runtime 从员工文本提取 contract：`parsed_result_contract` | `executor.rs:2208-2270` |
| ledger 通用写入 `repository.CreateExecutionLedgerEvent(ctx, req)`，best-effort 调用示例 | `service.go:2692` |
| dispatch 单测模式：`projectStoreMemoryRepository`(project_store_test.go:4410) + `projectTaskRunStarterFake`(:5873) + `NewProjectStoreWithApprovalsInboxAndRunStarter` | `project_store_test.go:2968` 起 ~24 个 |
| 员工 prompt 组装 `projectTaskRunPrompt` | `project_store.go:2682-2708` |
| `TaskResultContract` 自定义 UnmarshalJSON（单/复数折叠） | `task_result_contract.go:84-97` |

---

### Task 1: Rust 侧 deliverables 透传（防静默丢字段）

**Files:**
- Modify: `apps/runtime-agent/src/controlplane/models.rs:209-236`（TaskResultContract）
- Modify: `apps/runtime-agent/src/commands/executor.rs:2208-2270`（parsed_result_contract）
- Test: `executor.rs` 内既有 tests mod（:3162 附近有 `parsed_result_contract_*` 测试可仿照）

**Interfaces:**
- Produces: `TaskResultContract.deliverables: Vec<serde_json::Value>`，员工 fenced JSON 中的 `deliverables` 数组原样到达 CP。

- [ ] **Step 1: 写失败测试**（仿 :3162 既有测试的构造方式；先读该测试确认 helper）：

```rust
#[test]
fn parsed_result_contract_preserves_deliverables() {
    let text = r#"结论。
```json
{"result_contract":{"status":"completed","summary":"done","deliverables":[{"name":"head_commit","kind":"git_commit","value":"abc123"}]}}
```"#;
    let contract = parsed_result_contract(/* 按既有测试的实参形态传入 text */)
        .expect("contract parsed");
    assert_eq!(contract.deliverables.len(), 1);
    assert_eq!(contract.deliverables[0]["name"], "head_commit");
}
```

- [ ] **Step 2: 跑测试确认编译失败**（无 deliverables 字段）
Run: `cargo test --manifest-path apps/runtime-agent/Cargo.toml parsed_result_contract_preserves_deliverables`

- [ ] **Step 3: 实现**——models.rs 结构体加：

```rust
    #[serde(default)]
    pub deliverables: Vec<serde_json::Value>,
```

executor.rs `parsed_result_contract`：先读 :2208-2270 现状；该函数从提取出的 JSON value 手工构造 struct——按 `acceptance_results` 的同款提取逻辑增加 `deliverables`（数组→Vec<Value>，缺省空）。`synthesized_result_contract`（:2272）与 :2103/:2748 的 `..Default::default()` 构造点核对是否需显式补字段（Default 已覆盖则不用）。

- [ ] **Step 4: 测试通过 + 全量**
Run: `cargo test --manifest-path apps/runtime-agent/Cargo.toml` → 全绿。

- [ ] **Step 5: 提交**
```bash
git add apps/runtime-agent/src/controlplane/models.rs apps/runtime-agent/src/commands/executor.rs
git commit -m "feat(runtime): pass result_contract.deliverables through to control plane

The Rust contract struct is field-typed, so an undeclared field is
silently dropped on re-serialization; downstream handoff fulfillment
depends on this array surviving the hop.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: Go 契约类型 + 生产者侧履约校验

**Files:**
- Modify: `apps/control-plane/internal/project/task_result_contract.go`（类型 + ValidateTaskResultContract）
- Test: `apps/control-plane/internal/project/task_result_contract_test.go`（若无则新建，先 `ls` 确认）

**Interfaces:**
- Produces: `TaskResultDeliverable{Name,Kind,Value,Ref,Summary string}`；`TaskResultContract.Deliverables []TaskResultDeliverable`；`taskPlannerProduces(task ProjectTask) []string`（读 PlannerMetadata，与 projectcoordination 的 `plannerProducesFromMetadata` 同键——**实现前先读它的源码对齐键名与形状**）；ValidateTaskResultContract 在 status=completed 且 produces 非空时逐项核对 Deliverables（Name 匹配且 Value/Ref 至少一个非空），缺项 append 到 validation.Errors（错误文案含缺失名清单）。

- [ ] **Step 1: 写失败测试**

```go
func TestValidateTaskResultContractRejectsMissingDeliverables(t *testing.T) {
	task := ProjectTask{
		PlannerMetadata: map[string]any{"produces": []any{"head_commit", "review_notes"}},
	}
	result := TaskResultContract{
		Status:  TaskResultStatusCompleted,
		Summary: "done",
		Deliverables: []TaskResultDeliverable{{Name: "head_commit", Kind: "git_commit", Value: "abc123"}},
	}
	validation := ValidateTaskResultContract(task, result)
	require.NotEmpty(t, validation.Errors)
	require.Contains(t, strings.Join(validation.Errors, "\n"), "review_notes")
}

func TestValidateTaskResultContractAcceptsFulfilledDeliverables(t *testing.T) {
	task := ProjectTask{
		PlannerMetadata: map[string]any{"produces": []any{"head_commit"}},
	}
	result := TaskResultContract{
		Status:  TaskResultStatusCompleted,
		Summary: "done",
		Deliverables: []TaskResultDeliverable{{Name: "head_commit", Value: "abc123"}},
	}
	validation := ValidateTaskResultContract(task, result)
	require.Empty(t, validation.Errors)
}

func TestValidateTaskResultContractSkipsCheckWithoutProduces(t *testing.T) {
	task := ProjectTask{} // 旧计划无 produces → 行为与今天一致
	result := TaskResultContract{Status: TaskResultStatusCompleted, Summary: "done"}
	validation := ValidateTaskResultContract(task, result)
	require.Empty(t, validation.Errors)
}

func TestTaskResultContractUnmarshalsDeliverables(t *testing.T) {
	var contract TaskResultContract
	require.NoError(t, json.Unmarshal([]byte(`{"status":"completed","summary":"s","deliverables":[{"name":"x","ref":"artifact://1"}]}`), &contract))
	require.Len(t, contract.Deliverables, 1)
	require.Equal(t, "x", contract.Deliverables[0].Name)
}
```

- [ ] **Step 2: 跑确认编译失败**
Run: `go test ./apps/control-plane/internal/project/ -run 'TestValidateTaskResultContract|TestTaskResultContractUnmarshalsDeliverables'`

- [ ] **Step 3: 实现**

```go
// task_result_contract.go 类型区（TaskResultVerification 附近）：
type TaskResultDeliverable struct {
	Name    string `json:"name"`
	Kind    string `json:"kind,omitempty"`
	Value   string `json:"value,omitempty"`
	Ref     string `json:"ref,omitempty"`
	Summary string `json:"summary,omitempty"`
}
```

`TaskResultContract` 加 `Deliverables []TaskResultDeliverable json:"deliverables,omitempty"`——注意自定义 UnmarshalJSON(:84-97) 用了 alias struct，字段要同步进 alias。

```go
// 读 produces：先读 projectcoordination.plannerProducesFromMetadata 源码，键名/形状保持一致
func taskPlannerProduces(task ProjectTask) []string {
	raw, ok := task.PlannerMetadata["produces"]
	if !ok {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	var produces []string
	for _, item := range items {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			produces = append(produces, strings.TrimSpace(s))
		}
	}
	return produces
}
```

`ValidateTaskResultContract`（:243，status completed 分支）追加：

```go
	if result.Status == TaskResultStatusCompleted {
		if produces := taskPlannerProduces(task); len(produces) > 0 {
			delivered := map[string]bool{}
			for _, d := range result.Deliverables {
				if strings.TrimSpace(d.Name) != "" && (strings.TrimSpace(d.Value) != "" || strings.TrimSpace(d.Ref) != "") {
					delivered[strings.TrimSpace(d.Name)] = true
				}
			}
			var missing []string
			for _, name := range produces {
				if !delivered[name] {
					missing = append(missing, name)
				}
			}
			if len(missing) > 0 {
				validation.Errors = append(validation.Errors,
					"result_contract.deliverables 未覆盖 produces 声明的交接产出: "+strings.Join(missing, ", "))
			}
		}
	}
```

（插入位置以现函数结构为准：在 decision 映射前、Errors 聚合处；保证 Errors 非空时 validation 走 invalid → service :2757 rejected+waitHuman。先读 :243-302 确认 Errors 字段名与 invalid 判定。）

- [ ] **Step 4: 测试通过**
Run: `go test ./apps/control-plane/internal/project/ -run 'TestValidateTaskResultContract|TestTaskResultContractUnmarshalsDeliverables'` → PASS；再 `go test ./apps/control-plane/internal/project/` 全包无回归。

- [ ] **Step 5: 提交**
```bash
git add apps/control-plane/internal/project/
git commit -m "feat(project): add result deliverables and producer-side fulfillment check

A completed result whose deliverables do not cover the task's declared
produces is now a validation failure -> existing rejected+wait-human
path; tasks without produces keep today's behavior.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: 履约核对的可观测性（verification 填充 + ledger 事件）

**Files:**
- Modify: `apps/control-plane/internal/project/service.go`（CompleteProjectTaskAttempt :2726-2889）
- Test: `apps/control-plane/internal/project/service_test.go`（先找既有 CompleteProjectTaskAttempt 测试仿照；若服务层测试构造过重，允许把 enrich 逻辑抽成纯函数单测）

**Interfaces:**
- Consumes: Task 2 的 `taskPlannerProduces` / `Deliverables`。
- Produces: 校验通过时 contract.Verification 追加平台核对条目（`Type:"handoff_fulfillment"`, `Method:"platform_produces_check"`，status 用既有 passed 常量——先读 TaskResultVerificationStatus 常量名）；写一条 best-effort ledger 事件 `handoff.verified`（metadata: produces/delivered），拒绝路径写 `handoff.unfulfilled`（metadata 含 missing）。

- [ ] **Step 1: 写失败测试**（纯函数形态）

```go
func TestEnrichContractWithHandoffVerification(t *testing.T) {
	task := ProjectTask{PlannerMetadata: map[string]any{"produces": []any{"head_commit"}}}
	contract := TaskResultContract{
		Status:       TaskResultStatusCompleted,
		Deliverables: []TaskResultDeliverable{{Name: "head_commit", Value: "abc"}},
	}
	enriched := enrichContractWithHandoffVerification(task, contract)
	require.Len(t, enriched.Verification, 1)
	require.Equal(t, "handoff_fulfillment", enriched.Verification[0].Type)
}
```

- [ ] **Step 2: 确认编译失败** → **Step 3: 实现**

```go
func enrichContractWithHandoffVerification(task ProjectTask, contract TaskResultContract) TaskResultContract {
	produces := taskPlannerProduces(task)
	if len(produces) == 0 || contract.Status != TaskResultStatusCompleted {
		return contract
	}
	for _, name := range produces {
		contract.Verification = append(contract.Verification, TaskResultVerification{
			Status:  TaskResultVerificationStatusPassed, // 以实际常量名为准
			Type:    "handoff_fulfillment",
			Method:  "platform_produces_check",
			Summary: "deliverable \"" + name + "\" 已交付（平台核对）",
		})
	}
	return contract
}
```

接线（service.go）：validation 通过后、`projectTaskAttemptResultRecordRequest`（:2804）之前，`contract = enrichContractWithHandoffVerification(result.Task 对应的 task, contract)`（task 变量名以现场为准）。写回成功后（:2863 之后）best-effort ledger（仿 :2692 模式）：

```go
	if produces := taskPlannerProduces(task); len(produces) > 0 {
		_, _ = s.repository.CreateExecutionLedgerEvent(ctx, CreateExecutionLedgerEventRequest{
			TenantID: req.TenantID, ProjectID: task.ProjectID,
			ProjectTaskID: &task.ID, ProjectTaskAttemptID: &req.AttemptID,
			EventType: "handoff.verified", SourceType: "project_task_attempt",
			SourceID: req.AttemptID.String(), ActorType: "control_plane",
			OutputSummary: "交接产出逐项核对通过",
			Metadata: map[string]any{"produces": produces},
			IdempotencyKey: "project_task_attempt:" + req.AttemptID.String() + ":handoff.verified",
		})
	}
```

拒绝路径（:2757-2759 分支内）同型写 `handoff.unfulfilled`，Metadata 带 `validation.Errors`。EventType 若有常量表则加常量（查 `ExecutionLedgerEventAttemptStarted` 的定义处一并声明两个新常量）。

- [ ] **Step 4: 测试通过 + `go test ./apps/control-plane/internal/project/` 全包**
- [ ] **Step 5: 提交** `feat(project): surface handoff fulfillment as verification entries and ledger events`

---

### Task 4: 图校验收紧到直接 blocker + planner prompt 措辞

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/graph_validation.go:184-195`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go`（:286-287 produces/required_inputs 契约措辞 + deliverables 预告）
- Test: `graph_validation_test.go`（用既有 `planTaskWithIO` helper）

**Interfaces:**
- Produces: required_inputs 的生产者必须是**直接 blocker**（原为任意祖先）。

- [ ] **Step 1: 写失败测试**

```go
func TestValidateTaskGraphRejectsRequiredInputFromTransitiveAncestor(t *testing.T) {
	// A -> B -> C；C 声明需要 A 产出的 "fact"。祖先语义下合法，直接前驱语义下必须拒绝。
	a := planTaskWithIO("a", nil, []string{"fact"}, nil)
	b := planTaskWithIO("b", []string{"a"}, []string{"mid"}, []string{"fact"})
	c := planTaskWithIO("c", []string{"b"}, nil, []string{"fact"})
	plan := RouteDecisionPlan{Reason: "chain", Tasks: []PlannedTask{a, b, c}}
	err := ValidateRouteDecisionGraph(plan, planEmployeeIDs(plan), GraphValidationPolicy{MaxTasks: 10})
	require.ErrorIs(t, err, ErrInvalidRouteDecision)
	require.Contains(t, err.Error(), "direct blocker")
}

func TestValidateTaskGraphAcceptsRequiredInputFromDirectBlocker(t *testing.T) {
	a := planTaskWithIO("a", nil, []string{"fact"}, nil)
	b := planTaskWithIO("b", []string{"a"}, nil, []string{"fact"})
	plan := RouteDecisionPlan{Reason: "pair", Tasks: []PlannedTask{a, b}}
	require.NoError(t, ValidateRouteDecisionGraph(plan, planEmployeeIDs(plan), GraphValidationPolicy{MaxTasks: 10}))
}
```

（`planEmployeeIDs` 若不存在，按 `planTaskWithIO` 生成的 SelectedEmployeeID 收集；先读 helper 确认。注意 b 消费 "fact" 时也声明了 required——直接前驱是 a，合法。）

- [ ] **Step 2: 确认失败**（第一个测试当前通过祖先校验 → require.ErrorIs 失败）

- [ ] **Step 3: 实现**——`:184-195` 替换为：

```go
	for _, task := range plan.Tasks {
		direct := map[string]struct{}{}
		for _, blocker := range task.BlockedByKeys {
			direct[blocker] = struct{}{}
		}
		for _, required := range plannerRequiredInputs(task.InputRequirements) {
			producer, ok := producers[required]
			if !ok {
				return invalidRouteDecision("task %q: required input %q is produced by no task in this plan", task.Key, required)
			}
			if _, ok := direct[producer]; !ok {
				return invalidRouteDecision("task %q: required input %q is produced by task %q, which is not a direct blocker; add a dependency edge (one edge = one handoff)", task.Key, required, producer)
			}
		}
	}
```

若 `ancestorKeys` 因此失去全部调用者则一并删除（编译器会告知）。**检查既有测试**：跑 `go test ./apps/control-plane/internal/workflow/projectcoordination/ -run TestValidate` ——若有用例依赖祖先语义（跨级 required），把用例改为补直接边并在断言注释里注明语义变更依据（spec §3.1）。

planner prompt：找到 :286-287 关于 produces/required_inputs 的句子，改为明确 "a required input must be produced by a DIRECT blocker of the consuming task (declare the edge in blocked_by_keys; one edge = one handoff)"，并追加一句："At execution time the platform injects each task's direct blockers' results (upstream_results) into its dispatch request, and a completed task must return result_contract.deliverables covering every name it lists in produces."

- [ ] **Step 4: 全包测试**
Run: `go test ./apps/control-plane/internal/workflow/projectcoordination/` → 全绿（含 planner 快照类测试如有 prompt 断言需同步更新）。

- [ ] **Step 5: 提交** `feat(coordination): require handoff inputs to come from direct blockers`

---

### Task 5: 派发注入 upstream_results

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`（新 helper + prompt/packet 签名与 4 个调用点 :2368/:2413/:2520/:2532）
- Test: `project_store_test.go`（memory repo 模式）

**Interfaces:**
- Consumes: `ListProjectTaskDependencies`、`latestTaskResult`、Task 2 的 `Contract.Deliverables`。
- Produces: `collectUpstreamResults(ctx, tenantID, projectID, task) []map[string]any`；`projectTaskRunPrompt(projectRecord, demand, task, upstreamResults []map[string]any) string`；`projectTaskDispatchExecutionContextPacket(..., upstreamResults []map[string]any)`。

- [ ] **Step 1: 写失败测试**（memory repo 已有 `taskDependencies` / `projectTaskResults` 字段）

```go
func TestDispatchProjectTaskInjectsDirectBlockerResults(t *testing.T) {
	// blocker 任务 completed，带 latestTaskResult（Contract.Summary + Deliverables）；
	// 被派任务 blocked_by blocker。构造仿 TestProjectStoreDispatchProjectTaskStartsRunAndQueuesTask(:2968)。
	repo := ... // planned 任务 + blocker(completed, LatestTaskResultID=rid) + taskDependencies + projectTaskResults[rid]
	var captured StartProjectTaskRunRequest
	starter := &projectTaskRunStarterFake{result: okStartResult(), onStart: func(req StartProjectTaskRunRequest) { captured = req }}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)
	require.NoError(t, store.DispatchProjectTask(ctx, DispatchProjectTaskInput{...}))

	require.Contains(t, captured.Prompt, "upstream_results")
	require.Contains(t, captured.Prompt, "head_commit")          // deliverable name
	require.Contains(t, captured.Prompt, blockerTaskID.String())
	// packet 断言：repo.queueRequests[0].ExecutionContextPacket["upstream_results"] 非空
}

func TestDispatchProjectTaskUpstreamSummaryTruncatedAt4KB(t *testing.T) {
	// blocker result Summary = strings.Repeat("长", 3000)（>4KB UTF-8）
	// 断言 prompt 内 summary 被截断且带 summary_truncated 标记
}

func TestDispatchProjectTaskToleratesBlockerWithoutResult(t *testing.T) {
	// blocker 无 LatestTaskResultID → upstream_results 含 {"result":"unavailable"}，派发不失败
}
```

- [ ] **Step 2: 确认编译失败** → **Step 3: 实现**

```go
const upstreamSummaryLimitBytes = 4096

func truncateUpstreamSummary(summary string) (string, bool) {
	if len(summary) <= upstreamSummaryLimitBytes {
		return summary, false
	}
	end := upstreamSummaryLimitBytes
	for end > 0 && !utf8.RuneStart(summary[end]) {
		end--
	}
	return summary[:end] + "…[truncated]", true
}

// Injection is additive context: any lookup failure degrades to less
// context, never to a blocked dispatch (spec §5).
func (s *ProjectStore) collectUpstreamResults(ctx context.Context, tenantID, projectID uuid.UUID, task project.ProjectTask) []map[string]any {
	deps, err := s.repository.ListProjectTaskDependencies(ctx, tenantID, projectID, []uuid.UUID{task.ID})
	if err != nil || len(deps) == 0 {
		return nil
	}
	results := make([]map[string]any, 0, len(deps))
	for _, dep := range deps {
		blocker, err := s.repository.GetProjectTask(ctx, tenantID, dep.BlockerTaskID)
		if err != nil {
			continue
		}
		entry := map[string]any{
			"task_id":    blocker.ID.String(),
			"task_title": blocker.Title,
			"status":     string(blocker.Status),
		}
		if blocker.AssignedDigitalEmployeeID != nil {
			entry["digital_employee_id"] = blocker.AssignedDigitalEmployeeID.String()
		}
		if result, err := s.latestTaskResult(ctx, blocker); err == nil && result != nil {
			summary, truncated := truncateUpstreamSummary(result.Contract.Summary)
			entry["summary"] = summary
			if truncated {
				entry["summary_truncated"] = true
			}
			if len(result.Contract.Deliverables) > 0 {
				entry["deliverables"] = result.Contract.Deliverables
			}
			if len(result.Contract.EvidenceRefs) > 0 {
				entry["evidence_refs"] = result.Contract.EvidenceRefs
			}
			if len(result.Contract.ArtifactRefs) > 0 {
				entry["artifact_refs"] = result.Contract.ArtifactRefs
			}
		} else {
			entry["result"] = "unavailable"
		}
		results = append(results, entry)
	}
	return results
}
```

prompt（:2682）签名加 `upstreamResults []map[string]any`，在 `handoff_contract:` 行后插入：

```go
		"produces: " + taskContractJSON(plannerProducesFromMetadata(task.PlannerMetadata)) + "\n" +
		"upstream_results: " + taskContractJSON(upstreamResults) + "\n" +
```

结果契约要求句尾追加："result_contract 必须含 deliverables 数组，逐项覆盖 produces 列出的每个产出名（每项含 name 与 value 或 ref）；produces 为空时可省略。upstream_results 是你直接上游任务的真实产出，优先复用其中的值与引用，不要重做上游已完成的工作。"

packet（:2710）签名加参、包体加 `"upstream_results": upstreamResults`（nil 时置 `[]any{}` 或省略，取既有风格）。

四个调用点：Path A 在 gate 之后一次 `upstream := s.collectUpstreamResults(ctx, input.TenantID, input.ProjectID, task)`，:2368/:2413 传入；Path B 在函数入口后收集一次，:2520 传入、:2532（仅重建分支）传入。

- [ ] **Step 4: 全包测试** `go test ./apps/control-plane/internal/workflow/projectcoordination/` → 全绿（既有 ~24 个 dispatch 测试的 prompt/packet 断言若受影响逐一修复）。

- [ ] **Step 5: 提交** `feat(coordination): inject direct-blocker results into dispatch prompt and context packet`

---

### Task 6: 门禁 + spec 修订 + CHANGELOG

**Files:**
- Modify: `docs/superpowers/specs/2026-07-13-handoff-contract-execution-loop-design.md`
- Modify: `CHANGELOG.md`

- [ ] **Step 1**: `corepack pnpm verify:control-plane && corepack pnpm verify:runtime-agent` 全绿。
- [ ] **Step 2**: spec 修订（追加"P1 落地修订（2026-07-13）"小节，不重写正文）：① 需求侧字段复用既有 `input_requirements.required_inputs`，不新增 `handoff_contract.required_outputs`（探查发现该机制已端到端存在，DRY）；② §4.4 缺项处理 P1 = 校验失败→rejected+waitHuman（理由见本计划 Global Constraints），自动返工接线列为 P2；③ 典型 deliverable 形态 = `{name, kind, value|ref, summary}`，kind 自由字符串暂无注册表消费方。
- [ ] **Step 3**: CHANGELOG Unreleased 段新增条目（时间戳用 `TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'` 生成），列四个能力 + 验证证据占位（Task 7 回填）。
- [ ] **Step 4**: 提交 `docs: record handoff loop P1 spec amendments`。

---

### Task 7: 真实 E2E（完成的必要条件）

无代码改动。阻塞规则同 CLAUDE.md：任一环节无法真实验证 → 排障，排不掉标记阻塞，不得以未验证状态声明完成。

- [ ] **Step 1: 服务就位**：`scripts/dev-services.sh restart control-plane runtime-agent`（web 未改可不动）；确认 heartbeat 200。
- [ ] **Step 2: 链式注入（spec §6-1）**：新建项目（2 名 claude-code 员工 + local-dev-node，流程同 07-13 transcript E2E），提交明确双阶段需求（如"任务一：查询当前日期并交付名为 current_date 的产出；任务二：依赖任务一，把 current_date 翻译成中文大写日期"）。等两任务完成后查库：
```sql
-- B 的派工单含 A 的产出
select execution_context_packet->'upstream_results' from project_task_attempts where project_task_id='<B>';
-- A 的 result 有 deliverables，验证条目非空
select contract_payload->'deliverables', contract_payload->'verification' from project_task_results where project_task_id='<A>';
-- ledger 出现 handoff.verified
select count(*) from execution_ledger_events where event_type='handoff.verified';
```
并确认 B 的最终 result 使用了 A 交付的值（读 B 的 summary）。
- [ ] **Step 3: 缺项拒绝（spec §6-2 修订版）**：对一个带 produces 的任务 attempt，用 admin 会话直接 curl `POST /api/v1/runtime/project-task-attempts/{id}/complete`，body 的 result_contract 为 completed 但 deliverables 缺项 → 断言任务未判 completed、进入 waiting_human、`handoff.unfulfilled` ledger 事件存在、validation errors 可读。（真实 API 路径，不需真实员工配合犯错。注意用一次性测试项目，勿污染他人数据。）
- [ ] **Step 4: fan-in（spec §6-3）**：需求"两个并行调查 + 一个汇总"（明示汇总任务依赖两者），断言汇总任务 packet 的 upstream_results 长度为 2。若 planner 不出 fan-in 形态，重试一次措辞；仍不出则记录为 planner 行为限制（图校验/注入逻辑已被 Step 2 与单测覆盖），不阻塞收尾。
- [ ] **Step 5: 图校验拒绝（spec §6-4）**：由单测覆盖（Task 4）；真实 planner 有修复循环会掩蔽拒绝，不做 E2E 强求，记录说明。
- [ ] **Step 6**: 证据回填 CHANGELOG 与 spec"落地记录"；跑 `$superteam-completion-check` 清单；合并 main 后按分支收尾规范做合并后确认再删分支。

## Self-Review

- **Spec 覆盖**：§3.1 直接前驱（Task 4/5）、§3.2 引用透传（Task 5 注入 refs；解引用属证据地基 spec，spec 已声明）、§3.3 两层锚定（Task 4 图校验 + Task 2/3 执行期核对）、§4.1 schema（Task 2 deliverables + 复用 required_inputs，spec 修订于 Task 6）、§4.3 注入（Task 5，含 4KB 截断/unavailable 降级）、§4.4 核对（Task 2/3，P1 语义修订）、§4.5 员工输出要求（Task 5 prompt）、§5 错误处理（各任务对应行为 + Task 5 容错测试）、§6 E2E（Task 7 逐条，§6-2/6-4 按 P1 语义调整并在 spec 修订中说明）。
- **占位符**：Task 1 Step 1 与 Task 5 Step 1 的构造细节标注了"先读既有测试/helper 确认实参形态"——这是对同 session 执行者的读前置指令而非 TBD；其余步骤均给出可落的代码。
- **类型一致性**：`TaskResultDeliverable` 字段在 Task 2 定义、Task 3/5/7 引用一致；`collectUpstreamResults`/`truncateUpstreamSummary`/`taskPlannerProduces`/`enrichContractWithHandoffVerification` 命名前后一致；prompt/packet 新签名在 Task 5 内四个调用点闭合。
