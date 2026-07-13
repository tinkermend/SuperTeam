# 交接契约执行闭环（上游 result 注入与逐项履约核对）

- 状态：待评审（设计决策已于 2026-07-13 会话对齐，未实现）
- 日期：2026-07-13
- 范围：让任务链上的**信息**跟着**调度**一起流动——上游任务的 result_contract 注入直接下游的派工单，交接声明获得 schema、图校验和执行期逐项核对，缺项走已有补做循环。
- 前置：`2026-07-09-provider-transcript-tool-event-capture-design.md`（已落地）提供证据指针的地基；`2026-07-09-evidence-grounding-artifact-collection-design.md` 提供引用解引用的读路径（本 spec 的"按引用透传"依赖其落地，P1 可先注入 summary 文本不阻塞）。
- 后继：`2026-07-13-scenario-template-registry-design.md`（场景模板）依赖本 spec 先行——契约执行闭环不存在时，模板沉淀的默认契约同样无人执行。

## 1. 问题

DAG 链式执行只有**调度序**，没有**信息流**。四个库中实证（2026-07-09/07-13 对真实 dev 库诊断）：

1. **下游看不到上游产出。** 派工单 prompt 由 `projectTaskRunPrompt`（`project_store.go:2682`）拼装，内容只有：项目/需求 ID、原始需求内容、本任务标题摘要、expected_outputs、input_requirements、handoff_contract。**没有任何上游 result。** A→B→C 链上，C 被正确地在 B 完成后拉起（`ResolveReadyDownstream` 已工作），但 C 拿到的仍是原始需求——B 做了什么它一无所知。
2. **`input_context_refs` 是无解引用的字符串。** planner 会写 `["demand","repo"]` 之类，派发时没有任何代码把它变成内容。
3. **`handoff_contract.required_outputs` 52/52 条 NULL。** planner prompt（`openai_compatible_planner.go:277`）只要求"有 handoff_contract 这个对象"，没规定内部 schema；服务端 `decodeRequiredPlannerObject` 只验"是个对象"。
4. **`contract_payload.verifications` 20/20 条空。** `TaskResultVerification`（`task_result_contract.go:140`）定义了从未填充。

共同根因：**声明没有消费方**。派发不注入、回写不核对、verdict 不引用——一个无人执行的契约字段，LLM 写了也是白写，自然退化成空壳。

## 2. 目标与非目标

**目标**

1. 派发下游任务时，其**全部直接 blocker** 的 result_contract（summary + 交付物清单 + 证据/工件引用）注入派工单。
2. `handoff_contract.required_outputs` 获得机器可读 schema，规划期图校验拒绝"引用了不存在产出"的交接。
3. 上游回写时逐项对 required_outputs 核对交付；缺项不得静默判 completed，进入已有的 `blocked_resolvable_upstream` 补做循环。
4. 注入体积有界，不随链条长度增长。

**非目标**

- 不做传递闭包注入（见 §3.1，这是本 spec 的核心反目标）。
- 不做验收判据与 verdict 绑定（intent spec 范围）。
- 不做场景模板默认契约（后继 spec 范围）；本期 planner 按新 schema 现场生成契约。
- 不新建"任务间聊天"或消息通道——交接仍是结构化对象，符合宪法。

## 3. 架构决策

### 3.1 注入范围 = 直接前驱，一条依赖边 = 一份交接

A→B→C→D 链上，C 的派工单只注入 B 的 result；D 只注入 C 的。**不做传递闭包**：

1. **成本有界**：注入量由入度决定，不随链长增长。闭包注入在长链上是 O(N²) token 膨胀。
2. **责任清晰**：B 的 result 是 B 对 C 的契约。D 直接拿 A 的原始产出会绕过 B、C 两级加工，令中间任务的合成与判断作废。
3. **倒逼规划质量**：自动透传会让 planner 永远不必想清楚"这条边上交接什么"——required_outputs 全 NULL 就是这个病。

"汇总任务需要全部上游"不需要特殊机制——那是 **fan-in**：planner 让汇总任务直接依赖 T1/T2/T3 三条边，按本规则自然拿到三份直接前驱 result。图结构表达需求，注入逻辑不开后门。

### 3.2 跨级信息传递的两个合法通道

要防的真实风险是**信息在链条中途丢失**（A 产出的 commit sha，C 需要但 B 的 result 没带）。不靠跨级注入兜底：

- **逐级声明**：下游 required_outputs 声明需要什么 → 图校验保证某个直接 blocker 的 `produces` 承诺产出它 → 上游回写时核对履约。B 没带 = B 未履约，走补做，而不是 C 越级翻料。
- **按引用透传、按需解引用**：result_contract 携带 refs（工件指针、证据指针、raw transcript log_ref），refs 可逐级出现在下一份 result 中；下游深挖时按引用取切片。这是宪法"执行时只注入当前任务需要的上下文切片"的机制化。

### 3.3 声明的真实性来自消费，不来自生成

一个声明"落地"须四层锚定，本 spec 负责其中两层：

| 层 | 机制 | 归属 |
|---|---|---|
| 生成 | 场景模板实例化（而非每次凭空发明） | 后继模板 spec |
| **规划期校验** | **图一致性：required ⊆ ∪(直接 blocker 的 produces)** | **本 spec** |
| 人类确认 | plan revision review 生效门 | 已存在（dev 自动接受）；收紧属 intent spec |
| **执行期核对** | **回写逐项履约核对，缺项即补做** | **本 spec** |

## 4. 组件设计

### 4.1 required_outputs schema

`handoff_contract.required_outputs` 从任意 JSON 收紧为数组，每项：

```json
{
  "name": "head_commit",
  "kind": "git_commit",
  "description": "包含全部变更的提交 SHA",
  "required": true
}
```

`kind` 走注册表（`git_commit` / `branch_ref` / `artifact_ref` / `conclusion` / `evidence_ref`…），服务端校验存在性，**不做封闭枚举**（宪法约束）。planner prompt 补一段 schema 说明与示例；`decodeRequiredPlannerObject` 升级为按 schema 反序列化，坏形状拒绝（复用 `invalidRouteDecision(reason…)` 诊断机制，2026-06-25 已落地）。

同时 `produces`（planner 已输出，`openai_compatible_planner.go:390`）从 `[]string` 收紧为同构条目，作为图校验的供给侧。

### 4.2 图校验扩展（graph_validation.go）

对每个任务的每条 `required_outputs[i]`：存在某个 `blocked_by_keys` 中的上游任务，其 `produces` 含同 `name`（或同 `kind` 且唯一）条目。不满足 → `invalidRouteDecision("task %s requires output %q not produced by any blocker")`，计划被拒。**想象出来的交接在建图时就活不下来。**

无依赖的根任务 required_outputs 必须为空（它的输入只有 demand 本身）。

### 4.3 派发注入（project_store.go 派发路径）

`DispatchProjectTask` 组装时（`projectTaskRunPrompt` :2682 与 `projectTaskDispatchExecutionContextPacket` :2710 两处同步）：

1. 查本任务全部直接 blocker（`project_task_dependencies` 已有）。
2. 取每个 blocker 的最新成功 result_contract（`project_task_results` / execution_summary）。
3. 注入结构化 `upstream_results` 段：

```json
"upstream_results": [{
  "task_id": "…", "task_title": "…", "employee_id": "…",
  "summary": "…",
  "deliverables": [{"name": "head_commit", "kind": "git_commit", "value": "abc123…"}],
  "evidence_refs": ["…"], "artifact_refs": ["…"],
  "raw_log_ref": "runs/{tenant}/{attempt}/manifest.json"
}]
```

**体积控制**：每个上游的 summary 截断（4KB，复用 `truncate_excerpt` 语义）；deliverables/refs 全量保留（本来就是指针和短值）。截断时标记 `summary_truncated: true`，完整内容凭 raw_log_ref/evidence 引用可取（依赖证据地基 spec 的读路径；未落地前 UI 提示同 transcript spec §4.7 的占位处理）。

### 4.4 回写逐项核对（Complete 路径）

`CompleteProjectTaskWriteback` 事务内，对该任务 handoff_contract.required_outputs 逐项在 result_contract 的 deliverables 中查找：

- 全部命中 → 正常 completed，`verifications` 填充逐项核对结果（`TaskResultVerification` 第一次有真实数据）。
- 缺 `required: true` 项 → 任务**不判 completed**，标记 `completed_with_unmet_outputs`（语义沿用现有 `blocked_resolvable_upstream` 家族），交由协调线程走**已落地**的上游补做机制（迁移 055、`upstreamSupplementTaskKey`，`project_store.go:3209`）——本 spec 不新建返工机制，只是给已有机制接上第一个真实的触发源。

核对是**形态核对**（名字/kind 存在且非空），不是语义验真——"值是否正确"属 intent spec 的 verification 范围。核对结果本身写入 ledger（`event_type=handoff.verified` 走既有 `create_execution_ledger_event`），人在 Web 时间线可见。

### 4.5 员工侧输出要求

`projectTaskRunPrompt` 的"结果契约要求"段补一句：result_contract 必须含 `deliverables` 数组，逐项对应派工单 handoff_contract.required_outputs 的 name/kind 并给出 value 或 ref。（现有 prompt 已要求 acceptance_results 逐条覆盖 acceptance_criteria，同一模式。）

## 5. 错误处理

| 场景 | 行为 |
|---|---|
| blocker 无成功 result（异常态派发） | 注入 `{"status":"unavailable"}` 占位并记 warn；不阻断派发（调度门已保证正常路径不会发生） |
| 上游 result 缺 deliverables 字段（旧数据/旧员工输出） | 按缺项处理走补做；不做静默兼容 |
| required_outputs 为空的任务 | 跳过核对，行为与今天一致（渐进迁移：旧计划不受影响） |
| 注入后 prompt 超限 | summary 已截断兜底；deliverables 指针不裁剪 |
| 图校验误杀（planner 表达差异） | 拒绝理由带 task key + 缺失 output 名（复用 reason-bearing 诊断），planner 按 maxAttempts 重试 |

## 6. 测试策略

**单元（Go）**：schema 反序列化坏形状拒绝；图校验缺出处拒绝/满足通过/根任务非空拒绝；注入组装（多 blocker、fan-in、截断）；回写核对（全命中/缺可选项/缺必选项→补做标记）。

**真实 E2E（完成的必要条件，沿用 transcript spec 的口径）**：

1. 双任务链需求（A 产出一个事实，如"把当前 git HEAD 写入结论"，B 消费它复述）：查库确认 B 的派工单 prompt/context_packet 含 A 的 `upstream_results.deliverables`，且 B 的最终 result 使用了该值而非重新执行获取。
2. 缺项场景：让 A 的 result 故意缺一个 required 交付物 → A 不判 completed、补做任务被创建（`#upstream-supplement-` key 出现）。
3. fan-in：汇总任务依赖两个上游，派工单含两份 upstream_results。
4. 图校验：提交一个 planner 无法满足出处的需求形态，确认计划被拒且拒绝理由可读。
5. `verifications` 在真实库中第一次非空。

## 7. 后续

- 场景模板 spec：默认 required_outputs 按 task_kind 沉淀，planner 从"现场生成契约"变为"模板实例化"。
- intent spec：verification 从形态核对升级为语义验真（verdict 绑 criterion + attestation）。
- 证据地基 spec：upstream_results 中 refs 的解引用读路径。

## 8. P1 落地修订（2026-07-13）

实现前对真实代码的探查发现需求侧声明机制已端到端存在，P1 据此对 §4 做三处修订（决策依据见 `docs/superpowers/plans/2026-07-13-handoff-loop-p1.md`）：

1. **不新增 `handoff_contract.required_outputs` 字段。** 需求侧复用既有 `input_requirements.required_inputs`（planner 已输出、图校验已消费、员工自报 blocked 的 `missing_inputs` 已按它判定 `blocked_resolvable_upstream`）；供给侧复用既有 `produces`（PlannedTask 字段，持久化于 ProjectTask.PlannerMetadata）。§4.1 的 typed schema 落在 **result_contract.deliverables**（`{name, kind, value|ref, summary}`，Go+Rust 双侧），name 是与 produces/required_inputs 的连接键；kind 为自由字符串，注册表待有消费方再立（YAGNI）。
2. **§4.2 图校验落地为"直接 blocker"收紧**：原实现校验 required_inputs ⊆ 祖先 produces（传递可达），P1 收紧为必须由**直接 blocker** 生产（一条边=一份交接，§3.1），拒绝理由提示补依赖边。
3. **§4.4 缺项处理的 P1 语义 = 校验失败 → 既有 rejected+waitHuman 路径**（任务不判 completed，人类看到 `handoff_deliverable_missing:<name>` 精确清单），而非原文的"接补做循环"。理由：补做机制的触发语义是"下游被上游产出饿住"（消费侧自报），生产者自身未履约属返工家族；且宪法要求此类业务判断暂停等人。自动返工接线（unfulfilled → revision_attempt 或 supplement）列为 P2。配套可观测性：通过时平台向 contract.Verification 追加 `handoff_fulfillment` 条目并写 `handoff.verified` ledger 事件；拒绝时写 `handoff.unfulfilled`。

## 9. P1 落地记录（2026-07-13）

分支 `feat/handoff-loop-p1`。§6 真实 E2E（项目 `46e7206f`，真实 claude-code + local-dev-node）：

| # | 判据 | 结果 |
|---|---|---|
| 1 | 链式注入 | A 交付 `secret_code=1783911737`；B 的 execution_context_packet 含 upstream_results（A 的 deliverables/evidence/summary）；B 结论「壹柒捌叁玖壹壹柒叁柒」并自述未重跑命令——值逐字来自上游 |
| 2 | 缺项拒绝 | 员工按测试指令交错名 deliverable → `validation_errors=["handoff_deliverable_missing:handoff_test_result"]`、decision=validation_failed、任务 waiting_human 未判 completed、`handoff.unfulfilled` 事件、`project_task_clarification` 决策生成 |
| 3 | fan-in | 汇总任务 packet `upstream_results` 恰 2 条（kernel_name + work_dir），结论合并两值；两个并行任务分派两名员工真实并发 |
| 4 | 图校验拒绝 | 单测覆盖（直接 blocker 收紧 + fan-in 通过 + 跨级拒绝）；真实 planner 有修复循环会掩蔽拒绝，不做 E2E 强求 |
| 5 | verifications 非空 | `handoff_fulfillment` 平台核对条目入库，`handoff.verified` 事件 6 条 |

**E2E 附带观察（预存在缺陷，待立项）**：并行任务分派同一员工撞"单员工单活跃 run"限制 → 派发失败进 `project_task_recovery`；人批准恢复后重试仍撞同一冲突，任务滞留 waiting_human（实例 task `4dee7323`）。属调度并发约束与恢复重试策略问题，早于本 spec 存在。
