# 人类待办负荷预算与渠道分级 Spec

- 日期：2026-07-25
- 状态：**已确认开放决策，可开工**（拍板见 §8）
- 目标读者：接手实施的独立会话（不共享原始对话上下文，本文档需自包含）
- 基线：`2026-07-24-human-task-unification.md`（**已落地**——对象模型、减闸、投影合一、action→handler 注册表、收件箱分组等）
- 姊妹：`2026-07-17-feishu-integration-design.md`（飞书是投影通道；P1 卡片分级见其 §8.2，本文用 HumanTask `kind` 将其升格为正式策略表）
- 交付性质：原则固化 + 契约终态推进 + 词表修正 + 飞书渠道适配策略。**不重做** 2026-07-24 已交付的止血与减闸；只做增量。
- 现状核对：2026-07-25 逐条对照代码复核过一次（§4.2 现状表、§5.2 前置、§5.3 现状列、§5.4 存量分叉、§6 触及面均为实查结论）。行号是当时快照，实施时以符号名为准。

---

## 1. 背景

2026-07-24 HumanTask 统一解决的是「同一闸门多投影、哑动作卡死、验收一词三用」的结构问题。落地后，产品仍缺一层明确约束：

**人类闸门的默认成本必须下降，否则推送会变成骚扰，验收会变成橡皮图章。**

这与 Claude Code 类工具「把权限一次放开」的原因同源：AI 产出快于人类理解速度时，无穷 Yes/No 只会训练人类无脑点同意——系统看起来「有审批」，实际上没有决策。

2026-07-25 讨论拍板：下一阶段不以「修旧 SPEC」方式改历史文档，另开本文承载演进；旧文保留为已交付基线。

---

## 2. 目标与非目标

### 目标

1. 把「人类负荷预算」写成人类待办域的硬约束：闸门只服务**稀缺决策**，不为「系统安全感」表演。
2. 明确 HumanTask **终态形态**：对外一等读权威（API / 契约 / 产品语言），`inbox_items` 至多是实现细节。
3. 用 HumanTask `kind` 定义 **飞书交互分级表**（可批 / 深链 / 逐条签），与 Console **状态与 handler 同源、交互形态可以分形**。
4. 词表修正：`downstream_release` 中文由「阶段放行」改为「**下游放行**」。
5. 实施顺序：**不阻塞于**飞书剩余联调清单；Console/CP 终态与词表先合，飞书跟映射。

### 非目标

- 不重开 2026-07-24 已关闭的缺陷治理（F1–F8）；回归用既有门禁与夹具。
- 不把飞书做成第二个完整收件箱，也不要求飞书按钮与 Console 一一同形。
- 不在本期把 `decision_type` 技术枚举重命名（继承基线拍板：`kind` 为展示/契约层，`decision_type` 不动）。
- 不改判据引擎、对抗评审、review gate 的判定内核；只约束「是否打扰人、以何种渠道动作呈现」。
- 不做 Chat 对话转发、项目群（仍属飞书 P2）。

---

## 3. 原则：人类负荷预算

### 3.1 默认问句（闸门准入）

打开一张人类待办前，系统（规划器策略 + 闸门代码 + 产品文案）必须能回答：

> **不问这件事，会不会造成不可逆损失，或无法追责？**

- 若否 → **不产生待办、不推飞书**；证据汇入需求/结项层，人类在真正该看的那一层看一次。
- 若是 → 产生唯一 HumanTask，渠道按 §5 分级呈现。

### 3.2 推论（对既有基线的强化，不是推翻）

| 推论 | 含义 |
|---|---|
| 少闸 | 继承基线：叶子任务不拦；禁止 `RequiresHumanApproval` 派发前+完成后双拦 |
| 合闸 | 人类可见语义仅限：计划确认 / 执行放行 / **下游放行** / 验收签署 / 结项确认 / 异常类 |
| 稀缺推送 | 飞书只推「需要人类此刻决策」的卡；结果通知可合并/降噪，避免每个微观状态都私聊 |
| 证据门槛 | 渠道给强动作（一键通过、卡内批准）的前提是证据在该渠道可读；否则降级为深链或逐条 |
| 信任前移 | 安全感来自数字员工边界、Capability 授权、预算熔断、审计与 Attestation，而不是更多确认键 |

### 3.3 反模式（明确禁止）

- 为「看起来很严谨」叠加同义闸门（基线 F4 类问题的再引入）。
- Console 与飞书各发明一套动作语义或各写一条提交路径。
- 证据不足时仍下发「同意」类强动作（基线 F1 类问题的再引入）。
- 用推送频率证明系统在工作。

---

## 4. HumanTask 终态（读权威）

### 4.1 目标形态

```text
写权威：project_decision_requests（+ 协调线程路由）
读权威：HumanTask（OpenAPI / 生成客户端 / 产品语言的一等对象）
投影：  Console 收件箱、业务页同对象视图、飞书 outbox
废止：  对外把 inbox 描述成「另一种待办真相」；前端私自推导深链/动作
```

### 4.2 与现状的差距

基线已把这些语义挂上 `InboxItem`——**语义正确，形态仍是过渡**。精确现状（实施前必读，别按"都已经是字段了"估工）：

| 语义 | 现状载体 | 位置 |
|---|---|---|
| `kind` / `layer` | **契约一等只读字段** | OpenAPI `InboxItem`；服务端 `humanTaskKindAndLayer`（`internal/inbox/adapters.go`） |
| `why` / `evidence` / `progress` / `primary_surface` | **塞在无类型 `context` map 里**（`map[string]any`） | `internal/inbox/adapters.go`（`decisionContext*` 装配段，约 :142-151、:216-222） |

产品与契约语言也仍以 inbox 为中心。本期终态要求：

1. 契约层出现明确的 `HumanTask`（或等价具名 schema），字段对齐基线 §4.1。
2. **把 `why` / `evidence` / `progress` / `primary_surface` 从 `context` 提升为具名 schema 字段**（含各自的子结构：`progress{step,total,label}`、`evidence` 的判据/需求两形态）。只改对外名称而载荷仍是 `map[string]any` **不算**满足本条——见 §9 H2。
3. 列表/详情读路径以 HumanTask 为对外名称；若短期仍复用 `inbox_items` 表，文档与 OpenAPI description 必须写明「存储实现，非第二真相」。
4. 写路径不变：resolve 仍落决策/签署 handler 注册表；禁止新增平行「inbox-only」业务写。
5. 业务页与收件箱是**同一 HumanTask 的两个视图**，提交打同一 handler（继承基线）。

### 4.3 实施策略

- **按终态长字段与投影器**，允许分 PR：先契约+映射层（含上条第 2 点的字段提升），再替换前端类型名与文案。
- `context` 在字段提升后只保留尚未收编的原始快照，不得成为新语义的默认落点。
- 不要求一期物理删表 `inbox_items`；要求一期消灭「inbox 与 HumanTask 两套词」的对外叙述。

---

## 5. 飞书渠道：kind → 交互分级

### 5.1 一致与分形

| 必须一致 | 可以分形 |
|---|---|
| HumanTask 身份（同一 decision / 同一 id） | 信息密度、按钮多少 |
| `kind` 中文词表 | 卡内可操作 vs 深链 |
| open / resolved / cancelled | 卡片视觉 |
| action → handler（入站只调登记动作） | Console 一键整单 vs 飞书逐条/深链 |
| any-of-N 合格处理人 | 未绑定用户静默跳过（投影不阻塞） |

原则句（继承飞书设计 + 基线拍板 #9）：

> **签署动作必须紧邻证据；证据在渠道上不够完整，就不给强动作。**

### 5.2 前置：飞书当前收不到 `kind`（P2 的第 0 步）

分级表以 `kind` 为轴，但**现状全链路是 `decision_type` 轴**，实施前必须先接通：

| 环节 | 现状 | 需要的改动 |
|---|---|---|
| CP → outbox 载荷 | `BuildDecisionCardPayload` 只写 `"decision_type"`（`internal/project/feishu_outbox.go` 约 :98），终态卡重建路径同样（约 :229） | **两处都补 `"kind"`**（复用 `inbox.humanTaskKindAndLayer`，不新写映射） |
| connector 渲染 | `decisionContextElements` / 动作 switch / `decisionTypeLabel` 全按 `decision_type` 分支（`internal/cards/cards.go` 约 :288、:384、:532） | 改按 `kind` 查分级表；`decision_type` 只作兜底 |
| CP 入站动作注册表 | `registeredActionsForDecision(decisionType)` 按 `decision_type` 键（`internal/inbox/decision_action_registry.go` 约 :63/:105） | **不动**（写路径不变，见 §4.2 第 4 条）；分级表只决定"渲染哪些按钮"，按钮 key 仍取自注册表 |

**禁止**在 connector 里再抄一份 `decision_type → kind` 映射来绕过第一行——那会造出第二个身份真相，直接违反 §5.1 第一行与基线的单一注册表原则。

### 5.3 分级表（正式策略；替代「cards.go 隐式 switch」叙述）

「现状」列区分回归断言与净新增工作量，别按「全是重构」估工。

| kind | 中文名 | 飞书默认交互 | 现状 | 说明 |
|---|---|---|---|---|
| `plan_review` | 计划确认 | **卡内可批**（批准 / 请求修改） | 已实现（`decision_type=plan_review` 分支） | 决策短、信息可摘要；仅需改按 kind 取分级 |
| `planning_gap` | 规划缺口 | **卡内可批**（已补员重新规划 / 豁免约束 / 关闭需求） | 已实现 | 与基线异常类一致 |
| `acceptance_sign` | 验收签署 | **卡内逐条签署**，深链只作完整证据血缘兜底；**禁止一键全过** | 已实现且有护栏测试（`cards_test.go` 断言无整单批准） | 证据完整度要求最高；与 Console 一键整单分形。本期是**回归断言**，非新工 |
| `dispatch_release` | 执行放行 | **卡内可批**（放行 / 驳回；**驳回必填理由，放行不填**） | **净新增**：今日走 `default` 分支，只有深链按钮、证据区为空 | 高风险派发前；摘要须含风险与动作意图。理由口径对齐注册表（`rejected` 已 `RequiresComment`） |
| `downstream_release` | 下游放行 | **卡内可批**（放行 / 驳回；驳回必填理由） | **净新增**（同上） | 有下游依赖时；摘要须含上游产出要点 |
| `closure_confirm` | 结项确认 | **卡内可批**（确认结项并归档 / 退回返工 / 要求补证；后两者必填理由） | **净新增**（同上） | 标题身份=项目名（基线已定） |
| `planning_failed` | 规划失败 | **卡内可操作**（重新规划 / 关闭需求；补员可深链） | **净新增**（同上） | 异常必达，避免僵尸「规划中」 |
| `task_failure_recovery` | 任务失败恢复 | **卡内可批或深链**（按动作复杂度） | **净新增**（同上） | 优先给恢复/放弃，复杂排障深链 |
| 结果通知（非 HumanTask 闸门） | — | **只读**，不进待办心智 | 已有 `DecisionResolvedCard` / 需求结果卡 | 降噪口径见下 |

四个「净新增」kind 的工作量各含三件：证据区渲染（今日 `decisionContextElements` 无对应分支）、动作按钮、入站动作与注册表 key 对齐。不要按"改个 switch"排期。

**结果通知降噪口径**（避免"可合并"变成没人做的空话）：同一 decision 的终态只发**一张**结果卡（现有即时置换/`card_update` 路径），同一需求的阶段性完成不逐条私聊、合并进需求终态结果卡。本期不做更复杂的聚合。

升级规则：仅当飞书卡能稳定展示该 kind 所需证据全文（或等价可审计摘要）时，才允许把某 kind 从「深链」抬到「更强卡内动作」。**不得为了渠道长得像而先抬动作再补证据。**

### 5.4 词表护栏

两端词表：

- Console：`apps/web/src/lib/status-labels.ts`（`humanTaskKindLabel`，TypeScript map）
- 飞书：`apps/feishu-connector/internal/cards/cards.go`（今日是 `decisionTypeLabel`，Go switch；本期改为 kind 词表）

**存量分叉（本期必须消除）**：同一决策两边中文名已经不一样——飞书 `plan_review → "计划评审"`、`demand_acceptance → "需求验收(判据签署)"`，Console 对应「计划确认」「验收签署」。因此护栏**必须比对中文值，不能只比 key 集合**；只比 key 会让这处分叉过绿灯。

**护栏机制（必须指定载体，不能"对齐 `status-labels.guard.test.ts` 思路"了事）**：现有 `status-labels.guard.test.ts` 是 Vitest 对 `apps/web/src/features/**/*.tsx` 的正则扫描，**看不到 Go 代码**，跨语言断言做不出来。实施时三选一并在 PR 里写明选了哪个：

1. **契约为单一真相**（推荐）：kind 枚举 + 中文名进 `contracts/control-plane/openapi.yaml`（枚举 + `x-` 扩展或 description 约定），两端各自从生成物/契约文件读；护栏退化为"两端都不自带硬编码词表"。
2. **共享 fixture**：仓库内一份 checked-in JSON（kind → 中文名），TS 与 Go 各写一条测试断言自己的 map 与 fixture 逐键逐值相等。
3. **Go 侧反向校验**：Go 测试解析 `status-labels.ts` 的 `HUMAN_TASK_KIND_LABELS` 字面量并与 Go 词表比对（脆，仅在前两条都不可行时用）。

**已选 option 2**：`contracts/control-plane/human-task-kind-labels.json`；护栏为 `apps/web/src/lib/human-task-kind-labels.guard.test.ts` 与 `apps/feishu-connector/internal/cards/labels_test.go`。本期「下游放行」已纳入 fixture。

### 5.5 与飞书剩余联调的关系

飞书联调清单（绑定反查、any-of-N、结果卡、投影不阻塞等）**继续有效**，但验收口径挂到：

- 同一 HumanTask 在合格人绑定前提下：Console 可见 ⇔ 飞书应收；
- 任一渠道 resolve → 另一渠道不可再操作（卡消失或已处理态）；
- 词表 key 双边一致。

**不要求**联调清单清零后才开始本文 §4 / §6。

---

## 6. 词表变更：下游放行

| 字段 | 旧 | 新 |
|---|---|---|
| kind（不变） | `downstream_release` | `downstream_release` |
| 中文名 | 阶段放行 | **下游放行** |

触及面（实施时显式改，禁止残留用户可见「阶段放行」）：

- `apps/web/src/lib/status-labels.ts`（`HUMAN_TASK_KIND_LABELS.downstream_release`）+ 护栏测试期望值
- 收件箱分组表头（`apps/web/src/features/inbox/components/inbox-item-list.tsx`，分组表 `downstream_release` 那行的 `label`）
- **服务端进度条文案**：`apps/control-plane/internal/inbox/adapters.go` 的 `humanTaskProgress`（约 :272）返回 `"计划 已过 → 阶段放行 待你 → …"`，该 `progress.label` 由 InboxItem 原样下发、前端直接渲染——**漏改这处 H1 必挂**；同文件 :128 / :264 的注释顺带改
- 飞书 `decisionTypeLabel` / kind 映射（`apps/feishu-connector/internal/cards/cards.go`）
- 文档与 CHANGELOG 叙述（旧 SPEC 正文可保留历史用词，或加脚注「已更名为下游放行，见本文」——**不强制改写已落地 SPEC 全文**）

自查命令：`rg -n "阶段放行" apps/ contracts/`（除已落地 SPEC 与历史 CHANGELOG 条目外应为空）。

---

## 7. 实施顺序

### P0（小、可单独合）

1. 词表：「阶段放行」→「下游放行」+ 双边护栏期望更新。
2. 旧 SPEC 文首状态改为已落地，并链到本文（非施工改动）。

### P1（终态契约）

3. OpenAPI 具名 `HumanTask`（或等价），读路径/生成物对齐；Inbox 响应可嵌入或别名同一 schema。
4. **`why` / `evidence` / `progress` / `primary_surface` 从 `context` map 提升为具名字段**（§4.2 第 2 条）；投影器与前端读取同步改。
5. Web 类型与文案转向 HumanTask；行为不变，名称与注释对齐终态。

### P2（渠道分级落地）

6. **前置**：CP 在 outbox 载荷两处补 `kind`（§5.2 表第一行）——不做这步，P2 剩余项无法在不复制映射的前提下开工。
7. 飞书侧按 §5.3 建显式 `kind → InteractionGrade` 表（取代散落 switch 叙述）；入站动作只允许登记 handler。
8. 四个「净新增」kind（`dispatch_release` / `downstream_release` / `closure_confirm` / `planning_failed`）补证据区 + 动作按钮；每个 kind 各自一份真实推送抽检。
9. 按「同对象同状态」契约补齐/回归飞书联调项（投影不阻塞、resolve 后卡态、词表一致）。

### 明确不做的等待

- 不等待飞书 7 项联调全部绿才做 P0/P1。
- 不把 Console「一键整单」强行搬到飞书。

---

## 8. 人类拍板结论（2026-07-25）

1. **人类负荷是核心约束**：不为验收而验收；推送不得变成骚扰；信任边界前移到身份/能力/预算/审计，而非微观确认键。
2. **按终态建 HumanTask 读权威**：对外一等对象；`inbox_items` 不作第二真相；实施可分期，模型按终态长。
3. **飞书按 kind 交互分级**：与 Console 状态/handler 同源、交互可分形；`acceptance_sign` 维持深链或逐条，不一键全过。
4. **`downstream_release` 中文采用「下游放行」**（弃用「阶段放行」）。
5. **实施不阻塞于飞书剩余联调**；CP/Web 终态与词表先合，飞书跟映射。
6. **不改写已落地的 2026-07-24 SPEC 正文作施工图**；演进以本文为准，旧文仅标状态与交叉引用。

---

## 9. 验收标准

每条标注它把关哪一期——**P0/P1 不被飞书项卡住**（§7「明确不做的等待」）。

| 判据 | 把关 | 内容 |
|---|---|---|
| **H1** | P0 | 用户可见文案无「阶段放行」（含服务端 `progress.label`，见 §6 自查命令）；`downstream_release` 一律显示「下游放行」；status-labels 护栏通过 |
| **H2** | P1 | 契约/生成客户端中存在可引用的 HumanTask 具名类型，description 写明与 inbox 存储的关系；且 `why`/`evidence`/`progress`/`primary_surface` 是**具名字段**——生成的客户端类型里它们不再是 `map[string]any` / `Record<string, unknown>`。仅改名不改载荷 = 不通过 |
| **H3** | P2 | 飞书代码中存在显式 kind→交互分级表且与 §5.3 一致；卡片渲染取 payload 里的 `kind`（不在 connector 侧重算映射）；`acceptance_sign` 路径无「一键全过」（回归） |
| **H4** | P2 | 真实链路抽查——同一 `acceptance_sign`：Console 签署或收件箱处理后，飞书侧卡变为已处理/消失（绑定用户前提下）；反向飞书可操作 kind resolve 后 Console 卡同步消失。四个净新增 kind 各至少一次真实推送 + 卡内动作闭环 |
| **H5** | P1（可在 P2 前跑） | 单需求一次跑通产生的 open HumanTask 数**可枚举且无同义重复**：同一 `decision_request` 不出现两张卡；叶子任务无完成后拦（继承基线 G4）；一次跑通里 `acceptance_sign` + `closure_confirm` 至多各一张。抽检以 `project_decision_requests` 实查为准，不靠"感觉不多" |

门禁：`verify:contracts`、`verify:web`、`verify:control-plane`；涉及飞书时对 connector 定向测试 + 真实推送抽检。

**H4 的依赖**：飞书真实推送抽检需绑定用户，any-of-N 项需第二个真人飞书账号（见 `TODO.md` 飞书剩余联调 7 项）。这只阻塞 P2 收尾，不阻塞 P0/P1 声明完成。

---

## 10. 与旧文的关系

| 文档 | 角色 |
|---|---|
| `2026-07-24-human-task-unification.md` | 已交付基线（对象、减闸、投影、注册表） |
| **本文** | 负荷原则、读权威终态、渠道分级、下游放行词表 |
| `2026-07-17-feishu-integration-design.md` | 投影通道架构；§8.2 由本文 §5.3 按 kind 细化/对齐 |
