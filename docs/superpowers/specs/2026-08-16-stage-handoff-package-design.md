# 阶段交接包（A→B Handoff Package）：从倾倒注入到槽位编译

- 状态：待评审（**v2 修订版**：v1 稿经同日评审并逐项核实代码锚点后修订，修订要点见 §8；本文为 `2026-08-16-scenario-template-closed-loop-hardening.md` §4.6 预告的交接专稿，修订 `2026-07-13-handoff-contract-execution-loop-design.md` §4.3）
- 日期：2026-08-16
- 范围：场景模板串联链上，阶段任务完成 → 下游派发之间的**信息流形态**。把派发期"上游结果整包倾倒 + 4KB 截断"升级为"按下游声明槽位编译的交接包"，并给结论类信息一条受治理的通道。
- 非范围的先行结论（本文 §3.1 依据）：跨员工**不共享 provider 会话**。会话是 runtime 节点本地资产（`session_resume_preflight.go` 的 `session_node_mismatch`/`session_stale` 降级即证），且违反模板 `role_independence` 约束。会话连续只作为同员工的**优化层**（§4.6，P3）。

## 1. 问题

07-13 P1 落地后，调度边与信息流已经打通（直接前驱注入、图校验、履约核对）。但注入形态停留在"倾倒"：

1. **下游声明了要什么，控制面却不按声明给。** 模板骨架已用 `required_inputs_defaults` 声明命名槽位（如 review 需要 `head_commit`），图校验也保证有直接 blocker 的 `produces` 供给；但派发时 `collectUpstreamResults`（`project_store.go:3840`）取的是 blocker 的**整份** result（summary + deliverables + refs 全量），下游再去 JSON 里自己翻。4KB summary 截断（`upstreamSummaryLimitBytes`，`project_store.go:3822`）是"没人对下游需要什么做选择"的症状，不是治理。
2. **平台可结构化核验的事实没有走平台通道。** commit SHA、branch、工件 ref 的传递路径与散文混在一起，无形态核验、无溯源结构。（真值核验——如 commit 是否真为 workspace HEAD——P1 不做，见 §8 遗留。）
3. **上游结论没有受治理的通道。** A 段学到的知识（风险备注、已排除假设、怎么跑起来）只能挤在自由 summary 里被 4KB 截断。RCA 链上"排除了什么"这类负知识丢失后，下游会把同样的路再走一遍。
4. **B 的实际输入不可回放。** `execution_context_packet` 持久化了注入内容（好），但没有版本化结构、没有包级溯源（每份内容来自哪个 attempt/result），审计只能靠人读 JSON。

闭环加固稿 §4.6 与 2026-08-16 拍板（第 1、8 条）已定的方向：**槽位由模板命名 + kind；能平台填的（SHA、branch、工件 ref、attestation）由控制面填进下一员工上下文；模型结论是附加物，缺了不打回；散文不进闸**。本文把该拍板具体化为机制。

## 2. 目标与非目标

**目标**

1. 交接包 = **编译产物**：由控制面在派发期确定性组装（P1 零 LLM），结构分三层——信封（溯源）、槽位（事实）、结论（上游按下游声明的字段写）。
2. 模板不仅定义节点，还定义**边**：`required_inputs_defaults` 升级为带 kind/required 的结构化声明；新增下游可声明的**结论槽位**（notes）。
3. 失败语义分层：静态图平台槽位缺失 = 回写侧硬失败（沿用现有 `handoff_deliverable_missing` 拒绝路径）；派发期编译异常按**边覆盖分档**（闸覆盖边 = `unavailable` 留痕不阻断；未闸覆盖边 = 澄清卡，不静默派发）；结论缺失 = 不打回，走可观测 + 下游结构化质询；导读层（P2 模型编译）失败 = 降级。
4. 交接包版本化持久化于 attempt，包级溯源构成 handoff DAG（扇入 = 多份上游包），为回放/审计/后续按需取上下文（context slice）打地基。
5. 注入体积有界且**小于现状**：结构化槽位 + **有上界的**结论字段（2KB/条、8KB/包）+ 紧凑上游索引，替代整包倾倒 + summary 全文。

**非目标**

- 不改 07-13 §3 的核心决策：一条依赖边 = 一份交接、只注入直接前驱、不做传递闭包、fan-in 靠图结构表达。
- 不做跨员工会话共享（理由见文首）。
- 不给散文建闸门（08-16 拍板 8：禁止"交接必须写全"的反复打回）。
- 不新建任务间消息通道——交接包仍是结构化对象（宪法）；下游质询复用既有 blocked 申报与补做循环（§4.4）。
- 不做 runtime 执行期工件取回通道（下载 presign / MCP 取回工具）——列为 §8 遗留观察项；P2 导读层的 CP 侧解引用不受此限。

## 3. 架构决策

### 3.1 为什么是编译式交接包，而不是共享会话或纯透传

| 备选 | 判定 | 依据 |
|---|---|---|
| 跨员工共享 provider 会话 | 否决 | 会话是 runtime 节点本地文件（换节点/过期即失效，`FindProviderSessionCandidateForTaskRoot` 按员工+任务根圈定）；测试/审查继承开发推理路径违反 `role_independence`；compaction 是不受平台治理的丢失，B 实际所见不可审计；共享可变状态破坏 attempt 幂等与重试隔离；DAG 扇入时"继承哪个会话"无定义 |
| 纯透传（只给任务描述） | 否决 | 原始需求今天已在注入；丢的是上游结论（RCA 负知识尤其致命），事后无法从工件重建 |
| 上游自由生成交接文档 | 否决 | 权威内容来自模型则审计/回放/可信度全坏；撞 08-16 拍板"闸门只卡平台可核验事实" |
| **编译式交接包** | **采用** | 信封与槽位确定性编译（可回放）；结论由上游按下游声明字段写（信息损失最小的一方产料）；导读层可选（见 §4.5） |

对抗未来性：模型上下文窗口在涨，但无关内容稀释注意力质量（lost-in-the-middle）是稳定规律，蒸馏定向 brief 持续优于历史倾倒；多智能体协作的行业方向就是"边界处蒸馏、结构化交接"。provider 的 compaction 会进步，但永远不受平台治理——**平台自持的蒸馏层才是可持续的那层**。

### 3.2 交接包三层结构

```json
"handoff_package": {
  "envelope": {
    "template_key": "software_delivery", "template_version": "…"
  },
  "resolved_inputs": [
    {"name": "head_commit", "kind": "git_commit", "value": "abc123…",
     "source_task_id": "…", "source_result_id": "…", "artifact_ref_id": null,
     "verified": true}
  ],
  "upstream_notes": [
    {"name": "risk_notes", "source_task_id": "…", "value": "…"}
  ],
  "upstream_index": [
    {"task_id": "…", "task_title": "…", "employee_id": "…",
     "other_deliverables": [{"name": "…", "kind": "…", "ref": "…"}],
     "evidence_refs": ["…"], "artifact_refs": ["…"],
     "log_ref": "runs/{tenant}/{attempt}/manifest.json"}
  ]
}
```

- **版本与锚定**：包是 `execution_context_packet` 的 `handoff_package` 键。版本**沿用既有列 `execution_context_packet_version`**（"v1"→"v2"；runtime 已读该列并缺省 v1，`executor.rs:2620`），包内不造第二套版本键。任务/attempt/demand 锚定复用 packet 既有扁平字段，envelope 只留模板锚定；`template_version` 取**实例化时锁定**的快照，不随模板升版漂移。`resolved_inputs[].source_*` 构成包级溯源 DAG——B 的包指向 A 的 attempt/result，补链/打回重跑自然形成新包引用旧包。
- **槽位层**：事实，按值或 ref 传递。`verified: true` 表示平台做了**形态核验**（kind 注册表存在性 + ref 血缘），不含真值核验（§8 遗留）；核验判据复用 handoff assessment 现有逻辑，不新造。
- **结论层**：上游按下游声明的 notes 字段写的结构化散文。**不进闸**（缺字段不打回），质量手段见 §4.4；**体积有界**：值截断 2KB/条、包级预算 8KB（超限截断 + 留痕，沿用 `truncateUpstreamSummary` 模式）。
- **索引层**：未被声明命中的 deliverables 以 `{name, kind, ref}` 索引保留（不丢、降权、不带 summary 全文）。**消费方 = 审计/卷宗/人类与 P2 CP 侧导读编译**（CP 可经 `PresignArtifactContent`，`artifact_storage.go:240`，内部解引用）；下游 agent 执行期当前无按 ref 取回通道（runtime 仅有上传类 presign 与技能档案下载），**不承诺 B 自助深挖**——runtime 取回通道为 §8 遗留观察项。原 `upstream_results` 的 summary 全文不再默认注入。

### 3.3 边契约进模板（spec v3）

模板是流程宪法（08-16 拍板 1），边的声明属于模板，不属于 planner 现场。`SpecSkeletonStep` 扩展（向后兼容：v2 的 `required_inputs_defaults: ["head_commit"]` 归一为 `{name, required: true}`；现状图校验本就把全部 required_inputs 当 produces 供给检查（`graph_validation.go:249`），归一**不收紧**既有模板行为；`notes_defaults` 缺省为空）：

```go
type SpecRequiredInput struct {
    Name     string `json:"name"`
    Kind     string `json:"kind,omitempty"`     // 注册表开放，非封闭枚举
    Required *bool  `json:"required,omitempty"` // 默认 true
}
type SpecNote struct {
    Name        string `json:"name"`
    Description string `json:"description,omitempty"`
}
```

- **v1 稿的 `filled_by` 已砍除**（评审拍板）：`filled_by: upstream` 与 `notes_defaults` 同为"上游写的散文"，机制重叠；且按拍板 8（闸门只卡平台可核验事实）`required + upstream` 组合不能硬失败，`required` 对它是死字段。散文槽位的唯一机制是 `notes_defaults`，平台槽位的唯一机制是 `required_inputs`。
- `required_inputs_defaults`：`[]string | []SpecRequiredInput` 双形态归一。
- `notes_defaults`：下游声明的结论槽位（`what_changed` / `how_to_run` / `risk_notes` / `hypotheses_ruled_out`…），字段集由模板作者按场景定义。
- `handoff_contract`（实例化产物，当前写死 `{"completion_path": …}` 于 `template_instantiate.go:105`）扩展为 `{"completion_path": …, "notes": [...], "consumer_view": "<step title> 下游视角一句话>"}`。`consumer_view` 供 P2 导读层定向与补做工单注入（§4.4）。

### 3.4 失败语义分层（与 08-16 拍板对齐）

| 层 | 缺失/异常行为 | 机制 |
|---|---|---|
| 平台槽位（静态图，required） | **硬失败**（回写侧） | 现有路径不变：`validateCompletedTaskResult` → `handoff_deliverable_missing:<name>` → rejected + waitHuman（07-13 §8.3）。注意该闸校验的是**本任务自身** produces ⊆ deliverables（`task_result_contract.go:455`），对静态图经计划期图校验链式覆盖下游供给 |
| 派发期编译·闸覆盖边异常 | `unavailable` 占位 + warn 留痕，不阻断 | 现状语义（`collectUpstreamResults` 降级哲学："every lookup failure degrades to less context, never to a blocked dispatch"） |
| 派发期编译·**未闸覆盖边**缺失或同名冲突 | **派发前澄清卡，不静默派发** | 动态插入/重接的任务（§4.2 分档判据）其供给关系未经图校验，缺失是设计内路径而非异常态；转 `project_task_clarification`（predispatch human-wait 既有家族），卡面列缺失槽位/冲突来源 |
| 结论槽位 | **不打回** | handoff assessment 投影扩展 `missing_notes`（软，不进判据）；下游 blocked 申报质询（§4.4） |
| 导读层 brief（P2） | **降级** | 无 brief 照常派发 + 留痕事件（不留痕的降级等于静默丢上下文，沿用 session continuity 哲学） |

### 3.5 对 07-13 spec 的修订点

1. **§4.3 注入形态修订**：`collectUpstreamResults` 的"整包倾倒 + 4KB 截断"替换为 §3.2 的槽位编译。§4.3 的其余设计（多 blocker、fan-in、refs 全量保留）继续有效。
2. **§8.1 修订延续**：typed schema 落在 `result_contract.deliverables` 的决策不变；本 spec 新增 `result_contract.handoff_notes`（可选数组 `{name, value}`）。**契约门禁走 control-plane openapi**：`TaskResultContract`（`contracts/control-plane/openapi.yaml:12955`，被 Submit/Complete/Fail/WaitHuman 请求引用）与 assessment 投影（`ProjectTaskGraphHandoffAssessment`，openapi ~9615）**两处**变更 + `generate:control-plane` + `verify:contracts`。`contracts/provider` 现状不建模 result_contract（provider-result schema `additionalProperties: true`），本次**不动**。Go 类型随 openapi 生成；Rust 采集路径同 deliverables（`collect_declared_deliverables` 模式，`runtime-agent/src/artifacts.rs:379`）。
3. §3.1/§3.2/§3.3（直接前驱、两条合法跨级通道、四层锚定）**全部保留**，本 spec 是其"注入逻辑"细节的升级。

## 4. 组件设计

### 4.1 模板解析与实例化

- `spec.go`：`SpecRequiredInput`/`SpecNote` 类型 + 归一（v2 字符串形态 → `{name, required: true}`）；版本 guardrail 沿用 07-18 模式（声明 `spec_version: 3` 才允许 v3-only 字段）。
- `template_instantiate.go`：skeleton step → `PlannedTask` 时，`InputRequirements["required_inputs"]` 携带结构化形态；`HandoffContract` 写入 `notes` 与 `consumer_view`。
- **planner 现场生成路径同步**（`openai_compatible_planner.go`）：非模板实例化的计划（人工修订/恢复链）使用同一 schema，避免两条路径分叉。

### 4.2 派发期编译（`DispatchProjectTask`）

`collectUpstreamResults` → `compileHandoffPackage`：

1. 取全部直接 blocker 的最新成功 result（现状逻辑）。
2. **槽位解析与缺口分档**：对下游每个 `required_inputs` 条目，在 blockers 的 deliverables 里按 name（回落：同 kind 唯一）匹配 → 填 `{name, kind, value|ref, source_*, verified}`。未命中/同名多命中按**边是否被闸覆盖**分档：
   - **闸覆盖边**（下游任务来自已接受的计划分解 `DecomposeAcceptedPlanRevision`，其 required_inputs 过了图校验、生产者 produces 过了回写闸——两闸链式覆盖静态图）：异常态，记 `unavailable` 占位 + warn 留痕，不阻断派发（现状语义）。
   - **未闸覆盖边**（动态插入/重接的任务：revision（`CreateRevisionTaskForResult`）/ 对抗返工（`CreateReworkTaskFromAdversarial`）/ 补做（`CreateUpstreamSupplementTasks`）/ 恢复替换与重接（`createRecoveryReplacementTask` + `rewireRecoverableDependents`）/ 扩编合并（`prepareExpansionMergeTasks`）——这些任务的 produces 与下游 required_inputs 的供给关系未经 `ValidateRouteDecisionGraph`）：派发前转 `project_task_clarification` 卡，**不静默降级派发**。分档判据 = 任务是否属于已接受计划分解快照；动态路径建任务时须携带溯源标记（既有确定性 key/血缘先例）。
   - **同名冲突**（多 blocker 同名 deliverable）：静态图下 produces 全局唯一（`graph_validation.go:239`）不会出现，出现即异常 → 同走澄清卡（附全部来源），**不按顺序取首**。
3. **notes 提取**：按下游 `notes_defaults` 的 name 从各 blocker result 的 `handoff_notes` 提取合并（`source_task_id` 标注来源），应用 §3.2 体积上界。
4. **紧凑索引**：未被声明命中的 deliverables 以 `{name, kind, ref}` 索引保留在 `upstream_index`（消费方与边界见 §3.2 索引层）。
5. 写入 `execution_context_packet`（新增 `handoff_package` 键，`execution_context_packet_version` 列升 "v2"）并同步 `projectTaskRunPrompt`：prompt 呈现 **`resolved_inputs` + `upstream_notes` + `upstream_index` 三段**（替换现 `upstream_results` JSON 段）；"结果契约要求"段增加：`result_contract.handoff_notes` 需覆盖**下游声明的** notes 字段（见 §4.3）。

### 4.3 结论槽位的需求回传

全图在计划确认时一次性建好（`DecomposeAcceptedPlanRevision`），因此上游派发时即可查到其全部下游的 notes 声明：上游派工单的"结果契约要求"列出"你的下游（review/…）需要你交付以下结论字段：risk_notes: …"。**声明有消费方，字段才不会退化成空壳**（07-13 §1 的根因教训）。动态插入的下游（§4.2 未闸覆盖边家族）在其建任务时同样回写上游已派发任务的缺口——缺到 blocked 时走 §4.4 质询。

### 4.4 结论质量手段（不进闸）

1. prompt 侧 schema 要求（§4.3）。
2. handoff assessment 投影（`task_graph_handoff.go`，`assessProjectTaskHandoff` 现有 `Verdict: delivered|missing` 结构是同构扩展点）扩展 `missing_notes` 软条目，验收卡/卷宗可见缺口——人看得见，机器不打回。
3. **下游质询与补链（对齐既有 blocked 申报与补做循环，不新建通道）**。现状已有两块底座：B 可在 blocked 结果里申报 `Blocker.MissingInputs`（`mapTaskResultDecision`，`task_result_contract.go:782`，校验缺口名 ⊆ B 自己声明的 required_inputs）；协调线程已有缺输入补做循环（`CreateUpstreamSupplementTasks`，`project_store.go:1292`，`workflow.go:1279` 触发；缺项事件落账 `recordHandoffUnfulfilledLedgerEvent`，`service.go:4787`）。本 spec **不另起机制**，在其上做三件事：
   - **申报扩展**：blocked 申报的缺口白名单扩为 `required_inputs ∪ notes_defaults`，每项可带 `reason`（openapi 扩展，随 §3.5.2 门禁）。
   - **模式分叉**：plan 模式申报 → 直接 `project_task_clarification` 卡（`service.go:7267` 家族）呈人。loop 模式 → 查该边（blocker→B）**补链预算**（按边记账，幂等，落 handoff ledger 事件家族）。
   - **预算补链**：预算未用 → **复用 `CreateUpstreamSupplementTasks`** 建上游补做任务，工单注入缺口清单与 B 的 `consumer_view`；B 保持既有 blocked 等待语义，补做完成后 B 重派、`compileHandoffPackage` 重编译命中，新包 `source_*` 指向补做 result（handoff DAG 成链）。预算已用（**同边一次**）→ 升级人类澄清卡 + 留痕，不再自动——不做 AI 间无限拉扯（对齐 08-16 拍板 8，但保留 loop 模式的自主性）。

   B **带缺口完成**时在 result 里申报的（`result_contract.input_gaps: [{name, reason}]`）只投影进 assessment（与 `missing_notes` 同视图关联），不触发补链——补链只服务"缺到干不下去"的 blocked 申报。**是 B 的判断权，平台只提供有界的传导通道。**

### 4.5 导读层（P2：模型编译 brief，默认关闭）

- 配置：全局 config 定义模型端点与 prompt 模板；未配置即关闭，行为 = P1。
- 输入：`resolved_inputs` + `upstream_notes` + `upstream_index` + 下游 `consumer_view`；可选沿 `source_*` 溯源多拉一层（handoff DAG 的一跳深引用，缓解链式失真，仍不做传递闭包注入）。导读层是 **CP 侧编译**，可经 `PresignArtifactContent` 内部解引用来源工件（B 无此通道，边界见 §3.2 索引层）。
- 纪律：**事实靠引用、结论靠值；brief 中不得出现 sources 之外的"事实"**；输出附 citation 指回 `source_task_id/refs`。
- 评估红利：交接包版本化 + 冻结，brief 质量可离线回放评测，不影响线上。

### 4.6 边类型二分：handoff edge（默认）与 continuity edge（P3）

- 模板边可声明 `edge_type: continuity`（模板只声明**意图**，生效条件在派发期解析——两端同 role 且 casting 后同员工）：派发走现有 session resume preflight，**槽位与 notes 机制照常、不豁免**——会话带来的是额外深度，不是契约的替代。
- continuity 是**优化不是契约**：`session_node_mismatch`/`session_stale` 退化发生时自动落回 handoff edge 行为，交接包兜底。配套（可后置）：placement 同员工同节点亲和，降低退化率。

## 5. 错误处理

| 场景 | 行为 |
|---|---|
| 闸覆盖边槽位未命中（result 形态异常） | `unavailable` 占位 + warn 留痕，不阻断派发（现状） |
| 未闸覆盖边（动态插入/重接）槽位缺失或同名冲突 | 派发前澄清卡（`project_task_clarification`），不静默降级 |
| 上游 result 无 `handoff_notes`（旧员工/旧数据） | notes 为空数组照常派发；assessment 投影 `missing_notes` 可见 |
| notes 值/总量超限 | 截断（2KB/条、8KB/包）+ 留痕 |
| v2 模板（字符串形态 required_inputs） | 归一为 `{name, required: true}`；图校验本就把全部 required_inputs 当 produces 供给检查，行为与今天等价 |
| packet v2 组装失败 | 降级为 v1 `upstream_results` 形态派发 + 留痕（编译器 bug 不得阻断调度） |
| 导读层调用失败/超时 | 无 brief 派发 + 留痕事件（P2） |
| loop 补链预算耗尽（该边已补过一轮） | 升级人类澄清卡 + 留痕，不再自动 |

## 6. 分期与验收（done looks like）

| 期 | 内容 | 验收 |
|---|---|---|
| **H1a**（确定性编译） | 模板 v3 结构与归一（无 `filled_by`）；实例化与 planner 同步；`compileHandoffPackage`（含缺口分档与澄清卡路径）；notes 需求回传与 `handoff_notes` 双侧契约（openapi 两处 + `generate:control-plane` + `verify:contracts`）；packet v2（版本走既有列）；assessment `missing_notes` + Web（验收卡/卷宗）可见；notes 体积上界 | 真链：① develop→review 链，review 的 packet v2 含 `resolved_inputs.head_commit`（值来自 A 的 deliverable，带 `source_result_id` 溯源），review 结论使用该值；② A 的派工单结果契约要求列出 review 声明的 notes 字段，A 的 result 含 `handoff_notes`，B 的包按 name 命中；③ 软硬分离：漏 `head_commit` 走现有拒绝路径，漏 `risk_notes` 不打回且投影可见；④ fan-in 双 blocker 槽位合并、同名冲突落澄清卡不取首；⑤ 未闸覆盖边（如恢复替换/补做任务）缺槽位落澄清卡，不静默派发；⑥ 同链 v1/v2 包体积对比留观测 |
| **H1b**（缺口治理与补链） | blocked 申报扩展（notes 白名单 + reason）；plan/loop 分叉；按边补链预算与 ledger 记账；复用 `CreateUpstreamSupplementTasks` 注入缺口清单；预算耗尽升级澄清卡；完成态 `input_gaps` 投影 | 真链：① loop 模式链：B blocked 申报缺口（含 reason）→ 协调线程按边预算触发一轮补做（补做工单含缺口清单与 consumer_view）→ 补做完成后 B 重派、新包命中该槽位且 `source_*` 指向补做 result（DAG 成链留痕）；② 同边第二次申报不再自动，落澄清卡（卡面含两轮缺口清单）；③ plan 模式申报直接落澄清卡；④ 完成态 `input_gaps` 仅投影 assessment，不建任务 |
| **H2**（导读层） | 模型配置、brief 编译与 citation、降级留痕、冻结包离线评测 | 真链：配置模型后 B 的 brief 出现在派工单且 citation 可解；关模型行为退回 H1；离线评测报告一份 |
| **H3**（连续边） | `edge_type: continuity` 声明、退化回落、placement 亲和 | 真链：同员工连续步 resume 成功且槽位核验照常；制造 node mismatch 退化后交接包兜底 |

判据按小单口径逐条独立可验证（H1a/H1b 拆期即为此）。排期遵循 2026-08-16 拍板 7（B 期后，D/交接不插队的既有序列由人重排）。

### 落地记录（2026-08-17）

**H1a 真链**（PulseAI 项目，v3 探针模板 + 真实 claude-code，CP/runtime 归属本 checkout）：① develop→review 链，review packet `execution_context_packet_version="v2"`，`resolved_inputs.probe_commit` 带值/`verified`/`source_result_id` 溯源，结论逐字复用；② develop 派工单含 notes 需求回传段（1439 处），链尾任务无回传段；③ `handoff_notes` 双条按名命中 review 包 `upstream_notes`，assessment 投影 `delivered×2`。③硬失败（漏 produces→`handoff_deliverable_missing` 拒绝+澄清卡）补验于 H1b 轮（demand `43d9a7f7`）。

**H1b 真链**：成功环（demand `8f35b1fb`，loop）：review blocked 申报 risk_notes（含 reason/required_by）→ 自动补做（工单含缺口+原因+consumer_view，ledger `handoff.supplement_created`）→ 边重挂 → 补做完成 → review 重派 → `upstream_notes` 命中 → completed，`probe_verdict` 逐字传递。耗尽环（demand `966346d2`，loop）：补做也漏 notes → 同一申报方第二次申报 → 只建 1 个补做、落 `upstream_supplement_review` 卡（卡面"该边已自动补做一轮仍未解决"+本轮缺口与原因），ledger `handoff.budget_exhausted`。

**真链修复四项**（均在 H1b 轮发现并落地）：① blocked 申报后任务转 blocked 并清 run 绑定 + 让渡 attempt 活跃位（`TransitionProjectTaskBlockedForUpstreamSupplement` + `SupersedeBlockedUpstreamSupplementAttempt`）——否则补做完成后申报方永不重派（僵尸 running / "已派发"幂等短路 / `uq_project_task_attempts_active` 冲突三连）；② 补做创建时把申报方依赖边重挂到补做任务（`RewireProjectTaskDependencies`）——v1 起补做产出对消费者不可见；③ 派发期 blocker 已完成但结果未落时暂缓派发（走既有 retry-later 退避，等迟到结果的完成信号重派）；④ 派工单"结果契约要求"补 blocked 申报通道说明（`blocker.required_by` 必填等）——原 prompt 只教 completed，员工无从申报。

**真链未复现、单测覆盖**：fan-in 双 blocker 槽位合并（`TestResolveHandoffSlotsFanInMergesTwoBlockers`；真链被环境噪声阻塞）；plan 模式申报直接落卡（plan/loop 分叉为 059 tri-mode 既有机制，卡面增量为 `upstreamSupplementReviewSummaryWithReasons`；真链发现恢复替换任务不带计划修订 ID 会回退 loop——记入 TODO.md 既有缺陷）。

## 7. 测试策略

**单元（Go）**：v2/v3 spec 归一与 guardrail（既有模板 fixtures 归一前后图校验行为不变）；槽位解析（命中/name 回落 kind 唯一/未命中分档/fan-in 合并/同名冲突走澄清卡）；notes 提取、体积截断与需求回传；packet v2 组装与 v1 降级；assessment `missing_notes` 软条目；补链预算幂等与 plan/loop 分叉。

**契约门禁**：openapi 两处变更（`TaskResultContract.handoff_notes` + 申报 reason、assessment `missing_notes`）过 `generate:control-plane` + `verify:contracts`；`contracts/provider` 本次不动（现状不建模 result_contract）。

**Rust 侧**：`handoff_notes` 采集（对齐 deliverables 既有测试，`artifacts.rs` 模式）。

**真链**：见 §6 各期判据。

## 8. 拍板记录（2026-08-16）与遗留

**已拍板**

1. **未声明槽位保留形态 = 紧凑索引**（`name+kind+ref`，不带 summary 全文）。**索引消费方 = 审计/卷宗/人类与 P2 CP 侧导读编译，不承诺下游 agent 执行期自助取回**（runtime 现无取回通道）；runtime 取回通道列为遗留观察项。
2. **导读层模型配置 = 全局配置**（定位为平台基础管道；未来出现真实租户差异再评估 Tenant Profile 覆盖，不提前建配置面）。
3. **`input_gaps` 在 loop 模式 = 有预算自动补链**：**复用既有补做循环**（`CreateUpstreamSupplementTasks`）加按边预算（同边一次），未解决升级人类澄清卡（§4.4，H1b 验收）。

**评审修订（2026-08-16 同日，v1→v2）**

4. **派发期缺口分档**：回写闸只校验本任务自身 produces、不看下游（`task_result_contract.go:455` 实证），"派发期 missing 只覆盖异常态"的前提只在静态图成立；动态插入/重接边的缺失是设计内路径 → 澄清卡，不静默派发（§3.4/§4.2）。
5. **v3 砍 `filled_by`**：与 `notes_defaults` 机制重叠且 `required+upstream` 不可执行，是死字段（§3.3）。
6. **packet 版本沿用既有 `execution_context_packet_version` 列**升 v2，包内不造嵌套版本键（runtime 已消费该列并缺省 v1）（§3.2）。
7. **notes 体积上界**：值截断 2KB/条、包级预算 8KB，超限截断留痕（§3.2）。
8. **契约门禁补全**：handoff_notes 与 missing_notes 走 control-plane openapi 两处 + `verify:contracts`；`contracts/provider` 不动（§3.5.2）。
9. `raw_log_ref` 更名 **`log_ref`**（对齐既有列名；`runs/{tenant}/{attempt}/manifest.json` 格式为真实派生规则）。

**遗留讨论**

1. **RCA 假设台账**：已确认 RCA 是核心主打卖点，但台账形态（重平台对象 vs 轻量"台账即工件"）待单独讨论定稿。过渡期方案 = `hypotheses_ruled_out`/`hypotheses_remaining` notes 字段 + handoff DAG 引用。
2. **runtime 真值核验**：派发期让 runtime 对 workspace HEAD 等事实槽位复核（`verified` 从形态核验升级真值核验）——观察项，出现失真案例再立项。
3. **下游 agent 工件取回通道**：runtime 下载 presign 或 MCP 取回工具——观察项；P2 导读层 CP 侧解引用不受此限。
