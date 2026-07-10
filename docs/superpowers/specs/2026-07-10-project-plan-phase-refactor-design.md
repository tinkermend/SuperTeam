# Plan 阶段重构：选人、排序、契约核账与图延展

- 状态：待评审
- 日期：2026-07-10
- 范围：project coordination 的 plan 阶段——前置校验、基于描述的选人与排序、人类评审的两块确认、契约核账式的阻塞申报与自动延展、人类裁决触发的延展、会话降维。
- 关系：
  - `2026-06-21-...-phase-3-predispatch-gate.md` 引入了本 spec 要拆掉的能力闸门。本 spec **推翻**其中的 capability 判据部分，保留其余闸门。
  - `2026-06-21-...-phase-4-result-contract-revision-loop.md`（已确认，未定义终止条件）的 result contract 与 revision 机制**已实现**，本 spec 复用并修正其派发目标。
  - `2026-06-30-intent-acceptance-criteria-design.md` 定义 criterion 与 verification_method。本 spec 只实现「计划级判据 + 归属 + 人类裁决」这一最小子集，`verification_method` 注册表留给该 spec。
  - `2026-06-30-autonomous-outer-loop-...-budget-design.md` 负责预算熔断细则。本 spec 只留调用点。

## 1. 问题

以下每一条都来自当前 `main`（`8cd076c4`）的代码与真实数据，不是推测。

### 1.1 派发闸门在人类批准之后二次否决

> **勘误（2026-07-10，实施 Plan 1 后的代码审查）**：本节初稿断言闸门「照抄 LLM 自述的 `missing_capabilities`」。**这是错的。** 该数组由服务端重算并覆盖。真实缺陷不是抄袭，是双重审判 + 无意义的词表。以下为更正后的诊断。

`predispatch_gate.go:311`（`internal/project/`）：

```go
if len(snapshot.Capabilities.HardMissing) > 0 {
    addCheck("capability.match", "failed", ...)
    addBlocker("capability.hard_missing", PreDispatchGateStatusReplanRequired, "hard", false, ...)
```

`HardMissing` 的值取自 `planner_metadata.employee_selection.missing_capabilities`。而该字段是**服务端算的**，不是 LLM 写的：

- `openai_compatible_planner.go:132` 在 LLM 返回后立刻调用 `ApplyPlanningProfileScores`。
- `graph_validation.go:75-76` 用 `ScorePlanningProfile` 的结果**覆盖** `task.MatchedCapabilities` 与 `task.MissingCapabilities`。
- `planning_profile.go:399-409` 的 `scoreCapabilities` 做的是真集合差：`missing = task.required_capabilities \ profile.Capabilities`。

所以模型即便写 `missing_capabilities: []` 也不会被采信——它会被覆盖。

**真实的缺陷是两条：**

**（a）差集的两端都是无注册表的自由文本。** `required_capabilities` 由 LLM 凭空合成（提示词未给任何候选集）；`profile.Capabilities` 来自 `capability_bindings.external_capabilities`，无词表、无服务端校验、runtime 从不兑现（§1.2）。确定性地算出一个毫无意义的差。

**（b）同一个结论被审判两次，第二次在人类批准之后。**

`ApplyPlanningProfileScores`（`graph_validation.go:78-81`）在**计划阶段**因 missing 非空而置 `RequiresHumanApproval = true` 与 `plan.RequiresHumanReview = true`。**这是对的**——它发生在人类批准之前，把疑点交给人。

而派发闸门在**人类批准之后**把同一个结论再读一遍，升级为 terminal 的 `replan_required`（`workflow/.../predispatch_gate.go:78` 的 `preDispatchGateDecisionTerminal`）。任务死在 `waiting_human`，且不提供任何可执行选项。

人类的批准被一个虚构词表推翻。**闸门要删的是第二次审判，不是第一次。**

### 1.2 能力键没有事实源

`external_capabilities` 的全部消费者：

- `workflow/projectcoordination/planning_profile.go:252` —— 喂给 planner 当事实。
- `employee/service.go:660` `normalizeCapabilityBindings` —— 只把它补成空数组，**不校验键名**。
- `apps/web/.../create.tsx` —— 自由输入框，显示一个计数。

`GET /digital-employees/create-options` 返回的 `capability_options` 是 `{"provider_types":[...], "skills":[], "mcp_servers":[]}`——**没有 `external_capabilities` 词表**。

Runtime 侧从不读它：`apps/runtime-agent/src/` 中 `capability_bindings` 唯一的出现是 `executor.rs` 里的 `serde_json::json!({})`。它不影响 provider 启动、不影响工具可用性、不影响任何执行行为。

对照之下 `skills` 与 `mcp_servers` 有真实注册表，且 runtime 会真的 `materialize_skills`、真的生成 `mcp_config_path`。`tool_requirements` 的闸门（`planning_profile_adapter.go:334`）也确实去查 `ListEffectiveMCPServers` 拿真实状态比对——**那是一个有事实源的闸门，能力那段本该照它写。**

### 1.3 返工只会派回同一个人

`workflow/projectcoordination/project_store.go:705` 的 `CreateRevisionTaskForResult`：

```go
revision, err := s.repository.CreateProjectTask(ctx, project.CreateProjectTaskRequest{
    ...
    AssignedDigitalEmployeeID: source.AssignedDigitalEmployeeID,
```

而 `TaskResultRevisionRequest`（`project/task_result_contract.go:190`）**没有任何指向目标任务的字段**：

```go
type TaskResultRevisionRequest struct {
    Reason, RecommendedTaskTitle, RecommendedTaskSummary string
    ContractChanged  bool
    RequestedChanges []string
}
```

所以：C 因为上游 B 的产出不足而做不下去时，它申报 `revision_needed`，平台给 **C 自己**开一个新任务，输入还是那份不足的输入。C 再跑再申报，直到 `revisionMaxAttempts` 耗尽。**这是一个必然空转的循环。**

`TaskResultBlocker`（`:198`）反而有 `RequiredBy string`，但它是自由文本，且 `blocked` 一律转人类。

### 1.4 人类评审看不到该看的东西

计划评审的决策对象上只有 `title_snapshot`、`summary_snapshot`、`risk_level_snapshot`。

而 planner 输出里 `blocked_by_keys`（顺序）与 `handoff_contract.acceptance_criteria`（任务级判据）都存在，`plan_revision_payload.go:182` 甚至**强制每个任务必须有至少一条判据**，没有就拒绝计划（`missing_acceptance_criteria:<key>`）。

**信息已经产出了，只是没喂给人。** 人类批准时看不到顺序，也看不到判据。

计划级（需求级）验收判据**不存在**——`plan_revision_payload.go` 里只有任务级的。

### 1.5 死字段与重复逻辑

- `PreDispatchCapabilitySnapshot.Unknown`：`workflow/.../predispatch_gate.go:759` 从 `inputRequirements["unknown_capabilities"]` 赋值，而**全仓无任何代码写入该键**，恒为空。
- ~~`projects.coordination_policy` 无任何行为读它~~ —— **勘误（2026-07-10）**：它是活的。`project_store.go:351` 把它装配进 `CoordinationSnapshot`，`requiredHumanReviewPolicyEnabled`（`openai_compatible_planner.go:651`）读其 `require_human_review_for_new_demands` 键。它只是**键很少**，不是无人读。§4.8 的 `max_plan_iterations` 与 §4.2 的 `selection_confidence_threshold` 应作为新键加入，而非「复活死字段」。
- `ProjectEventTaskRevisionRequested` / `RevisionCreated` / `ReplanRequested` / `TaskResultRejected`：`project/types.go` 中定义，**零处生产者**。
- 两个能力快照构造器（`app/planning_profile_adapter.go:314` 与 `workflow/.../predispatch_gate.go:736`）逻辑重复且都从 LLM 输出取值。

### 1.6 虚构词表仍在驱动控制流（Plan 1 未覆盖）

拆掉派发闸门之后，同一个病根仍在计划阶段起作用，且**无路可逃**：

| 模型的选择 | 后果 |
|---|---|
| 编造若干 `required_capabilities` | 员工的 `external_capabilities` 实质恒空 → `missing` 必非空 → `scoreCapabilities:409` 把每条 missing 变成一条 `HardFailure` → `ScorePlanningProfile:178` 把 `Score` **直接归零** → `ApplyPlanningProfileScores:78` **强制人工审批** |
| 不写 `required_capabilities` | `graph_validation.go:34`：`required_capabilities` 为空且未标人工复核 → **整个计划被拒** |

也就是说，**系统强迫模型发明一套它根本没有词表的能力名，然后因为这些名字对不上而惩罚它。**

实证（Plan 1 的 E2E，`project_tasks.id = d97bd715`）：

```json
"selection_score": 0,
"matched_capabilities": [],
"missing_capabilities": ["quantum-ledger.reconcile_verification", "codebase.analysis", ...]
```
`requires_human_approval = true`

那个 `quantum-ledger.reconcile_verification` 是烟测里故意编的名字。它让 `selection_score` 归零，并触发了一次人工审批——而这次审批与风险无关，纯由虚构词表触发。

**两个后果**：

1. **每个任务都被强制人工审批。** 人类审批的触发条件不是风险，是词表对不上。这稀释了 `risk.approval` 的意义（约束四要求两层审批各司其职）。
2. **`selection_score` 恒为 0。** §4.2 的 `selection_confidence`（阈值 0.7）**绝不能复用 `ScorePlanningProfile`**，否则上线即全线判为 `no_suitable_employee`。

`graph_validation.go:53` 与 `:55` 那两条拒绝规则是**死代码**：`ValidateRouteDecisionPlan` 的唯一调用点（`openai_compatible_planner.go:133,142`）永远在 `ApplyPlanningProfileScores` 之后，而后者已经把 `reviewRequired` 置真。

## 2. 目标与非目标

**目标**

1. 前置校验只判平台能客观确认的结构事实；不匹配的任务不调用 LLM。
2. Plan 一次调用完成选人与排序，输出结构化计划；能力匹配由它基于描述完成，产出 `selection_confidence`。
3. 人类评审看到并批准两块内容：**调度顺序** 与 **计划级验收判据**。
4. 运行时的自动动作只建立在**契约核账**上，不建立在任何主观判断上。
5. 图延展（补做 + 下游重跑）有确定性的归属来源与有界的终止条件。
6. `provider_sessions` 降维到 (员工, 任务)。

**非目标**

- 不实现 `verification_method` 注册表（`intent-acceptance-criteria` spec 负责）。
- 不实现预算熔断细则（`autonomous-outer-loop` spec 负责），只留调用点。
- 不做 Chat 模式。它不进本链路，是独立产品面。
- **不引入通用审查员数字员工。** 理由见 §4.5。
- 不改 `hasCycle`：任务图永远是 DAG。

## 3. 设计约束

这四条是硬规则。违反任何一条的设计一律驳回。

**（一）闸门只读代码可判定的事实，不读任何 LLM 自述。**

这正是 §1.1 违反的那条。内核框架（Claude Code）的 plan 模式不是叮嘱模型「先别写」，而是把写工具从工具集里移除——门是机制，不是请求。我们的对应物是 `predispatch_gate.go:219` 比对 `AcceptedPlanRevisionID`，那是对的；`capability.match` 是错的。

**（二）跨员工交接只经结果契约与证据引用，不经 transcript。**

内核的 subagent 返回给父的是**最终报告**，不是它的对话记录。它刻意不让 A 与 B 共享上下文。因此 C 看不到 B 的过程，C 只看到 B 交付的 `expected_outputs` 与证据引用。C 唯一能说的是「我的输入清单第 k 项没拿到」。

**（三）续接会话用于干活；裁决一律新开干净上下文，且裁决者不是执行者。**

若 B' 续接 B 的会话，B' 继承的不只是 B 的工作，还有 B 对自己工作的叙述。让同一条会话判断「B 做得够不够」，是要求一段对话推翻它自己前面的结论。

**（四）计划级批准不豁免动作级批准。**

人类批准「派 B 去改代码」，不等于批准 B 待会儿要执行的 `rm -rf`。`predispatch_gate.go:396` 的 `risk.approval_required` 是动作级闸门，保留。

### 3.1 壳的边界

内核（Claude Code / Codex）是单 agent、单人、同步、上下文即事实源。这层壳存在的理由，是内核**结构上做不到**的四件事：

1. **持久化业务事实**（内核的事实源是模型上下文，会话结束即消失）
2. **跨员工异步交接**（内核只有一个 agent）
3. **人类异步审批**（内核的人就坐在旁边）
4. **审计与证据**（内核的人在看终端，不需要 attestation）

凡是壳里做的事能被内核一句话代替，那就是多余的抽象。据此：

- **DAG 不是过度设计**，它是跨员工异步交接的最小必需品。内核不需要 DAG，因为它只有一个 agent 和一个同步的人。
- **`TodoWrite` 式的「模型自己维护的清单」不能作为调度依据**。内核里它是 advisory，错了人当场看见；我们的人是异步的，可能几小时后才来看。顺序必须持久化、人类批准、闸门据此判定。

## 4. 设计

### 4.1 前置结构校验

任务发起 → 确认所属项目 → 依次检查：

| 检查 | 事实源 | 不满足时 |
|---|---|---|
| 项目内有数字员工 | `project_members` | 直接回绝 |
| 至少一名员工 `status = ready` | `digital_employees` | 直接回绝 |
| 项目绑定的 runtime 节点在线且支持其 provider | `runtime_nodes` / `runtime_capabilities` | 直接回绝 |

**全部不调用 LLM。** 能力匹配不在这里——它没有事实源（§1.2）。

### 4.2 Plan 阶段：一次 LLM 调用

**输入**：需求文本 + 项目内全部数字员工的 `role` / `description` / `persona_memory_markdown` / 已绑定的 `skills` 与 `mcp_servers`。

**输出**（标注哪些字段今天已存在）：

```
tasks[]:
  key, title, summary                                   已有
  selected_employee_id, employee_selection_reason       已有
  selection_confidence: 0.0..1.0                        新增（取代 selection_score）
  blocked_by_keys                                       已有（顺序，DAG）
  input_requirements: { required_inputs: [key] }        改造（见下）
  expected_outputs: [key]                               已有
  acceptance_criteria: [...]                            已有，且已强制非空

plan_acceptance_criteria[]:                             新增
  id
  statement
  arbiter: "human"                                      本期只有这一个取值（§4.5）
  satisfied_by: [task_key, ...]                         归属
```

**`selection_confidence`**：planner 对每个候选员工给出匹配置信度。若某任务的最佳候选低于阈值（默认 0.7，配置见 §4.8），planner 返回 `no_suitable_employee`，整个计划不落库，直接回到人类。这是一次真实的 LLM 判断，不是集合运算。

**它必须由 planner 直接输出，不得由 `ScorePlanningProfile` 派生。** 该打分器一遇 `HardFailure` 就把 `Score` 归零（`planning_profile.go:178`），而每一个对不上的虚构能力名都会产生一条 `HardFailure`（`:409`）。复用它等于让阈值恒不满足——见 §1.6 的实测 `selection_score: 0`。`selection_score` 字段随 `ScorePlanningProfile` 的能力打分一并退役。

**`input_requirements` 的改造**。今天它是 LLM 自由填充的杂物袋（实测键为 `items` / `repository` / `scope` / `value`），且被两个闸门构造器当作能力数组的来源（§1.1）。改造为结构化：

```json
{ "required_inputs": ["load_test_report", "baseline_metrics"] }
```

其余自由字段移入 `planner_metadata.planner_notes`，不参与任何判定。

**删除** `required_capabilities` / `matched_capabilities` / `missing_capabilities` 三个数组的判决作用。`employee_selection_reason` 保留，作为给人看的解释。

**服务端校验**（计划落库前，全部拒绝而非降级）：

| 规则 | 状态 |
|---|---|
| `hasCycle(plan.Tasks)` 拒绝环 | 已有（`graph_validation.go:146`） |
| 每个任务 `acceptance_criteria` 非空 | 已有（`plan_revision_payload.go:182`） |
| 每条 `plan_acceptance_criteria.satisfied_by` 指向存在的 `task_key` | **新增** |
| 每个 `input_requirements.required_inputs[k]` 能在某个**上游**任务的 `expected_outputs` 中找到 | **新增** |

最后一条在计划阶段就堵死「计划有洞」，而不是等到运行时 C 卡住才发现。「上游」的定义是 DAG 可达性：k 的生产者必须是 C 的祖先。

### 4.3 人类评审：两块确认

计划评审的决策对象上补两块内容：

**调度顺序**——按 `blocked_by_keys` 拓扑排序后的线性视图。每步显示：谁执行、`employee_selection_reason`、`selection_confidence`、`expected_outputs`。

**计划级验收判据**——`plan_acceptance_criteria` 逐条展示，标注 `satisfied_by` 归属的任务。

人类批准 = 授权。**此后不得再用任何 LLM 自述阻断这个计划**（约束一）。

驳回时人类补充信息 → 触发新一次 plan 调用 → 产生新的 plan revision → 重新评审。

### 4.4 取证层：平台不裁决，平台取证

取证层只回答**进程级事实**，不回答判据是否达成。

**能提取的**（全部来自 provider 进程写出的字节，模型伪造不了）：

| 事实 | 来源 |
|---|---|
| 某条命令是否执行过、其退出码 | `tool_started.input_excerpt` + `tool_completed.is_error`（`execution_ledger_events`） |
| 某文件是否被写入/编辑 | raw transcript 中的 `tool_use{name:"Write"\|"Edit"}` |
| 某工件是否存在、其 sha256 | 对象存储 + `project_task_attempts.log_sha256` |
| 某证据引用是否指向本次 attempt 的 attestation | `verificationHasAttestationRef`（`task_result_contract.go:419`） |

这些能力**在 `8cd076c4` 之前不存在**。它由 provider transcript / tool 事件捕获提供。

**不能提取的**：「巡检结论准不准」「代码改得合不合理」「报告写得清不清楚」。这些本来就不该由程序判断。

**唯一不能松的规则**：提取不到就标 `unknown`，**绝不标 `passed`**。真正的危险不是漏判，而是把无法提取的事实默认成达标。

取证层的作用是**保证裁决者读到的不是被审者写的作文**。今天的真实案例：模型在 `text` 里写「证据：命令退出码 `0`」，而进程写出的 `tool_result.is_error` 是另一个值。两个值都在——一个是它说的，一个是它做的。

### 4.5 裁决层：只有人类

```
plan_acceptance_criteria[].arbiter = "human"      // 本期唯一取值
```

**不引入通用审查员数字员工。** 一个不针对具体逻辑、只看「结果 vs 目标」的审查员，面对「改代码」这类任务读一遍产出根本判断不了对错；面对「分析问题」它只能顺着结论走。它会给出**看起来像裁决的东西**，而那比没有裁决更危险——它让人以为把关过了。

LLM 可以对任何一条判据产出「我认为它没达成，建议这样补」，**作为建议呈现在人类的评审界面上，不进流程，不触发任何自动动作**。

无论谁裁决，平台都先把取证事实包附在判据旁边：哪些命令跑了、退出码、工件 hash、attestation 引用是否支持断言。

由此，本设计中**没有任何一处是模型评判模型**。LLM 只在两处出现：计划阶段的选人排序（人类批准），评审界面的建议（人类采纳与否）。

### 4.6 自动延展：只发生在契约核账上

以下三类失败是**账对不上**，不是主观判断，因此可以自动触发延展，无需任何裁决者：

**（a）阻塞申报**

C 卡住时只能申报：

```json
{ "status": "blocked",
  "blocker": { "reason": "...", "missing_inputs": ["load_test_report"] } }
```

- `missing_inputs[k]` **必须**出现在 C 自己的 `input_requirements.required_inputs` 中。否则视为契约违规 → 转人类。这堵死「C 凭空索要」。
- 平台（不是 C）查计划求归属：`owner = 计划中 expected_outputs 含 k 的那个任务`。由 §4.2 的落库校验保证 owner 必然存在且是 C 的祖先。
- 追加 `owner'` + `owner` 在 DAG 上的**全部下游**。

`TaskResultBlocker.RequiredBy`（自由文本）由 `missing_inputs`（结构化 key 列表）取代。

**这条路径不能复用现有的返工机制，必须新建。** 两处硬约束挡着：

- `mapTaskResultDecision`（`task_result_contract.go:563`）把 `blocked` 硬映射为 `TaskResultDecisionBlockedWaitingHuman`，直接转人类。
- `CreateRevisionTaskForResult`（`project_store.go:696`）的准入条件是 `Decision == RevisionAttempt && ResultStatus == revision_needed && RevisionRequest != nil`，且它把新任务派给 `source.AssignedDigitalEmployeeID`（同一个人）。

因此新增：

- 新 decision 值 `TaskResultDecisionBlockedResolvableUpstream`：当 `blocker.missing_inputs` 全部能在计划中解析出 owner 时取此值；任一解析不出 → 退回 `BlockedWaitingHuman`。
- 新 activity `CreateUpstreamSupplementTasks`：受派人是 **owner**，不是 source。这是与 `CreateRevisionTaskForResult` 的根本区别，也是 §1.3 的修复点。

`revision_needed` 的语义**保持不变**——「我自己做得不好，我重做」，仍派回同一员工。两个状态不可混用。

**（b）任务级契约未闭合**

`validateCompletedTaskResult`（`task_result_contract.go:382`）已实现的核对：`expected_outputs` 缺项、`acceptance_result` 缺项、`verification.status=failed` 却声称 `completed`、`requires_runtime_attestation` 却没带 attestation 引用。任一命中 → 该任务重跑（现有 revision 机制）。

**（c）attestation 不支持断言**（新增）

今天 `verificationHasAttestationRef`（`:419`）只检查引用**存在**。改为检查它**支持**断言：判据声明「命令 X 退出码为 0」，平台就去该 attempt 的 raw transcript 里找对应的 `tool_result`，`is_error` 对不上即不通过。这是核对，不是判断。

**为什么允许自述「我卡住了」而不允许自述「我完成了」**

「说自己卡住」没有收益，最坏是浪费一轮预算，而 `plan_iteration` 计数与指纹熔断会兜住（§4.8）。「说自己完成」能直接下班。

设计必须让**说谎无利可图的方向敞开，有利可图的方向关死**。这条不对称是整个方案能自洽的根。

### 4.7 人类裁决触发的延展

主图跑完、所有契约闭合后，`plan_acceptance_criteria` 逐条呈给人类（附取证事实包）。

- 全部达成 → 收敛，进项目验收。
- 人类判定某条未达成 → 平台按其 `satisfied_by` 定位任务 → 追加那些任务 + 其 DAG 下游。

**人类是裁决者，平台是定位器和执行器。** 平台不假装能判断目标，也不请一个模型来假装。

**模式**（`plan` / `loop`）**只影响 §4.6(a) 的跨任务自动延展**，不影响其余任何环节：

| 环节 | `plan` 模式 | `loop` 模式 |
|---|---|---|
| §4.6(a) 阻塞申报解析出 upstream owner | 停下，报人类 | 自动追加 `owner'` + 下游 |
| §4.6(b) 任务级契约未闭合 | 自动重跑该任务 | 自动重跑该任务 |
| §4.6(c) attestation 不支持断言 | 自动重跑该任务 | 自动重跑该任务 |
| §4.7 判据裁决 | 人类逐条裁决 | 人类逐条裁决 |

(b) 与 (c) 在两种模式下行为相同，因为那是**已有行为**（现有 revision 机制），不是本 spec 新增的自动化；把它们也做成模式相关会无谓地改变现状。

判据裁决在两种模式下都由人类做。**`loop` 模式不会自动判定「目标达成」**，它只是自动把契约补闭合。

> 这一节是本设计相对最初构想的收窄。「自动迭代直到验收判据满足」做不到，因为判据的裁决者是人类。能自动的只有「跑到所有契约闭合为止」。越过这条线的任何自动化，本质上都是让模型给自己打分。

### 4.8 延展的形态与终止

**图仍是 DAG。** 追加的是**新 key 的新任务**（`RevisionOfTaskID` 记血缘），不是回边。`hasCycle` 与 `plan_revision_payload` 的两处环校验一行不改。「循环」只存在于时间与血缘上；人类在评审界面看到的永远是可拓扑排序的线性顺序。

**下游必须跟着重跑。** 补了 B'，C 的输入变了，旧 C 的产物作废，必须 C'。重跑集合 = `owner ∪ owner 的全部 DAG 下游`。

**延展不触发重新审批。** 追加节点属于同一 plan revision 的 iteration N。现有闸门 `task.accepted_plan_revision_changed`（`predispatch_gate.go:229`，terminal）必须显式放行本情况，否则 loop 模式一启动就自杀。**只有 `plan_acceptance_criteria` 集合本身变化时**才必须回到人类重新审批。

**两个上限，不要合并**：

| 名称 | 含义 | 状态 |
|---|---|---|
| `revision_max_attempts` | 单个任务返工几次 | 已实现（`project_store.go:3110`，三级回退） |
| `max_plan_iterations` | 整张图延展几轮 | **新增，且需要新的持久化计数位** |

**`iteration_key` 是死键，不能作为图级计数的作用域。** `project_store.go:3074/3142/3162/3187` 有四处读它，**全仓零个写入者**；库中 62 个任务里 0 个带该键。

单任务返工的计数今天仍然有效，因为 `priorRevisionTasks`（`:3171`）还有 `sameSource` / `sameRoot` 两条血缘路径（`RevisionOfTaskID`）兜底。但血缘只连接「同一任务的历次返工」，**连不起「整张图的第 N 轮延展」**——B' 与 C' 属于不同的血缘树。

因此新增列（迁移 `054`）：

```sql
ALTER TABLE project_tasks ADD COLUMN plan_iteration INTEGER NOT NULL DEFAULT 0;
```

主图任务为 `0`；第 N 轮延展追加的任务为 `N`。上限判定为
`max(plan_iteration for tasks in this coordination_job) >= max_plan_iterations`。

同时**删除**四处对 `iteration_key` 的读取（`project_store.go:3074/3142/3162/3187`）。它的作用被 `revisionRootTaskID` 血缘（单任务返工）与 `plan_iteration` 列（图级延展）完整覆盖，不需要第三套作用域。

`max_plan_iterations` 四级回退，从具体到一般：

1. 人类批准计划时可覆盖，写进 plan revision。**这是主要旋钮**——他是唯一一个完整看到「要做什么、怎么验收」的人。
2. `projects.coordination_policy.max_plan_iterations`（复活 §1.5 的死字段）。
3. Tenant Profile。
4. 控制平面 `config.yaml`，取代 `project_store.go:33` 的硬编码 `defaultRevisionMaxAttempts int32 = 3`。**仅作兜底。**

`selection_confidence` 的阈值（默认 0.7）走同一套回退，去掉第 1 级。

**都不放 runtime 的 `config.yaml`**：按架构分层，Runtime 不承载业务策略。

**熔断**：复用 `repeatedRevisionFailure`（`project_store.go:3141`）的骨架，但**必须先修掉它的指纹**。

现状 `revisionFailureFingerprint`（`project_store.go:3043`）把 `contract.Summary` 揉进指纹：

```go
parts := []string{string(contract.Status), strings.TrimSpace(contract.Summary)}
```

`Summary` 是**模型写的散文**。同一个失败换一种措辞就是一个新指纹，熔断静默失效。这是当前代码的缺陷，也是把「模型自述」混入控制逻辑的又一处（约束一）。

指纹重定义为**只取结构化字段**：

| 场景 | 指纹 |
|---|---|
| `blocked` | `(source_task_key, sorted(missing_inputs))` |
| `revision_needed` | `(source_task_key, sorted(requested_changes))` |
| 判据未达成 | `(criterion_id, sorted(satisfied_by))` |

任何模型自由文本都不进指纹。

**熔断之后平台不猜。** 若 B'、C' 跑完判据仍不达标，真正的问题可能在 A。按 `satisfied_by` 只会一直重跑 B 和 C。此时转人类，由人类决定是否 replan。平台不猜，是这个设计能自洽的前提。

### 4.9 会话降维到 (员工, 任务)

今天 `provider_sessions` 的键是 `digital_employee_id` + `execution_instance_id`，**没有 `project_task_id`**；`session_policy` 挂在 `digital_employee_execution_instances` 上。库中每个员工恰好一行 session。`reusable_provider_session`（`apps/runtime-agent/src/commands/executor.rs:2941`）只看 `recoverable && mode != Ephemeral`。

**结果是「每个员工一条长会话」**：员工在同一项目做多个任务时上下文互相污染，且长会话会退化。

改造：

- `provider_sessions` 增加 `project_task_id` 维度（迁移 `054`）。
- `session_policy` 从 execution instance 下沉到任务派发载荷。
- `reusable_provider_session` 改为「同一 `(digital_employee_id, revision_root_task_id)` 才复用」。

**键必须是血缘根，不能是 `project_task_id`。** B' 是一个**新任务**（新 id、新 key）；若按 `project_task_id` 键控，B' 会开一条空会话，恰好丢掉我们要它继承的 B 的上下文。`revisionRootTaskID`（`project_store.go:3090`）已存在：它沿 `RevisionOfTaskID` 回溯到血缘根，主图任务的根就是自己。于是 B' 与 B 同根 → 复用；C' 与 C 同根 → 复用；同一员工的另一个任务 D 根不同 → 隔离。

B' 续接 B **那个任务**的会话——继续干活，上下文完整，不用重跑一遍。这与内核的 `SendMessage`（续接已有 agent）同构；而裁决者新开干净上下文，与内核的 `Task`（新起 subagent）同构。

**attestation 仍然只来自进程写出的字节。** 会话复用不改变证据来源，也不给会话里的自述任何裁决权（约束三）。

## 5. 删除清单

| 目标 | 位置 |
|---|---|
| `capability.match` check 与 `capability.hard_missing` blocker | `internal/project/predispatch_gate.go:311-317` |
| `GetEmployeeCapabilitySnapshot` 中重读计划期能力结论的三行 | `internal/app/planning_profile_adapter.go:315-317` |
| `applyGateTaskMetadata` 中的 capability 段（第二个快照构造器） | `internal/workflow/projectcoordination/predispatch_gate.go:744-759` |
| `PreDispatchCapabilitySnapshot.HardMissing` / `.Unknown` | `internal/project/predispatch_gate.go:86-91` |
| plannerTask 的 `RequiredCapabilities` / `MatchedCapabilities` / `MissingCapabilities` 的**判决作用** | `openai_compatible_planner.go:363-365`（字段保留于 `planner_metadata`，仅供展示） |
| 提示词中 "copy the required, matched, and missing capability arrays" | `openai_compatible_planner.go:276` |
| 提示词中 "A task with missing_capabilities must set requires_human_approval..." | `openai_compatible_planner.go:279` |
| `scoreCapabilities` 把 missing 升级为 `HardFailure`（进而归零 `Score`） | `planning_profile.go:409` |
| `ApplyPlanningProfileScores` 因 missing 强制 `RequiresHumanApproval` / `RequiresHumanReview` | `graph_validation.go:78-81` |
| `required_capabilities` 为空即拒绝整个计划 | `graph_validation.go:34-35` |
| 因 `HardFailures` / `MissingCapabilities` 拒绝计划的两条死规则 | `graph_validation.go:53-57` |

闸门保留且只保留有事实源的：`runtime.placement_missing`、`runtime.pinned_node_offline`、槽位、`budget.ready`、`context.ready`、`risk.approval`、skill 可安装、MCP 可达且凭证未过期、`task.accepted_plan_revision_changed`（§4.8 放行延展）。

`external_capabilities` 字段本身**不删**，降级为「给 planner 与人类阅读的能力描述」，从 `capability_bindings` 移入 employee 的描述性字段族。它不再进入任何判定。

`ScorePlanningProfile` 的其余维度（`scoreRole`、`scoreRuntime`、`scorePermissionsAndTools`、`scoreLoad`、`scoreReliability`）保留——它们打的是有事实源的分。只有 `scoreCapabilities` 及其 `HardFailure` 升级被移除。

**Plan 1 只删掉了派发时的第二次审判（§1.1(b)）。上表后四行属于计划阶段，由 Plan 2 处理。**

## 6. 错误处理

| 场景 | 行为 |
|---|---|
| 项目无员工 / 无 ready 员工 / 无在线节点 | 前置回绝，不调 LLM |
| planner 返回 `no_suitable_employee` | 回到人类，附每个候选的 `selection_confidence` 与理由 |
| 计划含环 | 拒绝落库（现有 `hasCycle`） |
| `satisfied_by` 指向不存在的 task_key | 拒绝落库 |
| `required_inputs[k]` 无上游生产者 | 拒绝落库（计划有洞） |
| C 申报 `missing_inputs` 含清单外的 key | 契约违规 → 转人类 |
| 取证提取不到 | 标 `unknown`，绝不标 `passed`；该判据必须由人类裁决 |
| 同一血缘根下连续两次相同结构化失败指纹 | 熔断 → 转人类 |
| `max_plan_iterations` 耗尽 | 停止延展 → 转人类 |
| 延展的重跑集合超出预算 | 调用预算闸门（`autonomous-outer-loop` spec），可拒绝延展 → 转人类 |

## 7. 测试策略

**单元（Go）**

- `plan_revision_payload`：`satisfied_by` 指向不存在的 key → 拒绝；`required_inputs` 无上游生产者 → 拒绝；生产者存在但不是祖先 → 拒绝。
- 阻塞申报：`missing_inputs` 含清单外 key → 契约违规；含清单内 key → 正确解析出 owner。
- 延展集合：给定 DAG `A→B→C→D` 与 `owner=B`，重跑集合为 `{B, C, D}`。
- `task.accepted_plan_revision_changed` 对 iteration N 的追加节点放行，对判据集合变更不放行。
- attestation 支持性核对：判据声明「命令 X 退出码 0」而 transcript 中该 `tool_result.is_error=true` → 不通过。
- `max_plan_iterations` 四级回退各取一级。

**真实 E2E（完成的必要条件）**

`scripts/dev-services.sh start` 起全套服务，在一个含两名 claude-code 员工的项目上：

1. 项目无 ready 员工时提交需求 → 前置回绝，`provider_session_events` 中无任何记录（证明未调 LLM）。
2. 正常需求 → 计划评审界面上能看到拓扑排序后的顺序与 `plan_acceptance_criteria`。
3. 构造 B 的 `expected_outputs` 缺失 → C 申报 `blocked{missing_inputs:[k]}` → 库中出现 `RevisionOfTaskID` 指向 B 的新任务，且**受派人是 B 而非 C**（这是 §1.3 的核心判据）。
4. B' 的 provider session 与 B 同一条（`provider_sessions` 中 `project_task_id` 相同），与该员工其他任务的 session 不同。
5. C' 被重跑（下游重跑）。
6. 人为让判据不达标并连续两次相同失败 → 熔断 → 任务转 `waiting_human`。
7. 一个 `missing_capabilities` 非空的计划**能够正常派发**（证明假闸门已拆除）。

第 3 条与第 7 条是本 spec 的核心判据。

## 8. 已知边界

**平台保证过程可信，不保证结论正确。**

`8cd076c4` 之后，平台能证明「这条命令跑过、退出码是 X、这个文件被改过、这份 raw transcript 未被篡改」。它永远无法证明「巡检结论准确」「代码改得合理」。

因此本设计的自动化程度，取决于**验收判据能被落到可提取事实上的程度**，而不取决于模型多聪明。「测试通过」可核对；「代码写得对」不可核对。这个压力应当放在写判据的人身上，而不是让平台假装能判断。

`arbiter` 本期只有 `human` 一个取值。若将来引入 `agent_review`，必须同时满足：裁决者非执行者、裁决者上下文中不含执行者的自述、裁决落审计行、取证结果为 `unknown` 的判据不得由其自动通过。

## 9. 后续

- `2026-06-30-intent-acceptance-criteria-design.md`：`verification_method` 注册表，使部分判据可从 `human` 降级为机器裁决。
- `2026-06-30-autonomous-outer-loop-...-budget-design.md`：延展前的预算核算与熔断。
- **Chat 模式**：独立立项。其产出默认不进证据链（借鉴 paperclip 的 `sourceTrust: quarantined | promoted`，`packages/shared/src/trust-policy.ts`），须显式提升。
- **`external_capabilities` 的归宿**：本期降级为描述性字段。若将来要恢复其判定作用，必须先有注册表、服务端校验，以及 **runtime 侧的兑现**——三者缺一不可，否则又是一个无事实源的闸门。
