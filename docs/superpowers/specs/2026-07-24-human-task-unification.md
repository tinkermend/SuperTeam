# 人类待办统一 Spec（验收割裂治理 · HumanTask 单一对象）

- 日期：2026-07-24
- 状态：**已落地**（P0–P2 见 CHANGELOG 2026-07-24；本文件保留为交付基线，不再作施工图改写）
- 演进：负荷原则、HumanTask 读权威终态、飞书 kind 分级、词表「下游放行」→ 见 `2026-07-25-human-task-load-budget-and-channel-grading.md`
- 目标读者：理解已交付模型与缺陷根因的后续会话；实施增量请读演进 SPEC
- 交付性质：跨层重构（契约 + CP + Web + 读模型）。本 spec 负责「人类待办」这一个域：它的产生、呈现、执行与收敛。**不负责**权限中心域（见 `2026-07-20-permission-center-refactor.md`）。

---

## 1. 背景：一次真实复验暴露的问题

2026-07-24 用真实链路（Temporal / CP:8080 / Web:3000 / runtime-agent + claude-code + deepseek planner）做了一次针对性复验：新建 3 个复验项目、跑通 3 次真实执行，浏览器点击 + curl + 直连 PG 三路交叉验证。

起因是人类反馈「验收动作割裂在两个菜单且不同步」。复验证实问题存在，且**比反馈更严重**：收件箱里官方提供的「同意」按钮会把需求推进永久卡死状态。

### 1.1 五个界面对同一条需求的五种说法

同一时刻（14:50）、同一条需求 `dbd24727`（项目 `4627824a`）：

| 界面 | 它的说法 | 真实状态 |
|---|---|---|
| 收件箱 | 无待办 | 需求 `acceptance_pending`，等人签署 |
| 流程编排列表 | 已完成 | 同上 |
| 需求详情页 | 待验收（点通过 → 404） | 同上 |
| 项目 → 审批 tab | 需求验收 **已批准** | 同上 |
| 项目头部 | 执行中 1 | 任务已 completed |

### 1.2 实测缺陷清单（每条都有现场证据）

**F1〔致命〕收件箱「需求验收」的按钮是哑动作，点一次需求永久卡死**

| 时间 | 事实 |
|---|---|
| 14:48:03 | 卡出现：`item_type=project_decision`、high、动作 = 同意/驳回/要求补证、`deep_link=/workflows/{demand}` |
| 14:49:58 | 浏览器点「同意」→ 提交 200、卡消失、收件箱归零 |
| 同刻 DB | `demand_criterion_verdicts` **0 行**；demand 仍 `acceptance_pending` |
| 协调线程 | `LoadHumanDecisionRoute` → **`ApplyPreDispatchGateDecision`**（default 分支的任务派发闸活动），与需求验收无关 |
| 14:50:35 | 回需求页点「最终验收 通过」→ **HTTP 404**，toast「签署失败，请重试」 |

此后无解：收件箱无卡、签署 404；闸门重开依赖新的任务完成/失败信号（`GetApprovalRequestByResource` 只查 pending），而该需求已无在跑任务。

三处机制叠加：
- `apps/control-plane/internal/inbox/service.go:363-365`：`len(req.Actions)==0 → DefaultActions(req.ItemType)`，把 `DecisionActions("demand_acceptance")`（同文件 :451，刻意返回空动作集，注释写明"纯深链、不给按钮"）**静默覆盖**成 同意/驳回/要求补证。
- `apps/control-plane/internal/workflow/projectcoordination/workflow.go:367-402`：决策路由 switch **无 `demand_acceptance` 分支**，落 default 的 `applyPreDispatchGateDecision`。
- `apps/control-plane/internal/project/service.go:6424`：`SignDemandCriterionVerdict` 的存在性闸要求存在 pending `demand_acceptance` 决策；决策被点掉后签署即 404。

**F2〔结构〕一个闸门被两条投影管道抢同一行**

`inbox_items` 唯一键 `(tenant_id, source_approval_request_id)`（`storage/queries/inbox.sql:67+` 的 `UpsertInboxItemByApprovalSource`）。`ApprovalProjectorAdapter` 与 `DecisionProjectorAdapter`（`inbox/adapters.go:31-58` / `:76-120`）写同一行，后写覆盖先写。

实测（项目 B `72e20451` / 需求 `e69156cd`）：
- 14:59:38 建卡：`project_decision` / `/workflows/{demand}` / 有 `source_project_id`
- 15:00:13 需求页正常签署后：同一行变成 `approval` / `/approvals` / **`source_project_id` = NULL**

后果：已处理的验收事项丢项目归属、深链跳权限中心、类型从"项目决策"变"审批"。且 resolve 时传入的 `DecisionRequest.InboxContext` 为空 → `contextPayload = {}` 覆盖 `context_payload` → 前端所有基于 `decision_type` 的解释文案（`inbox-action-dialog.tsx:291` 的 `decisionFraming`）在这些卡上不渲染。实测 `demand_acceptance` **开放态**的 `context_payload` 本来就没有 `decision_type` 键（`project_store.go:4165-4171` 未写入），所以那套文案对需求验收卡从未生效过。

**F3〔用户原述场景，已复现〕项目验收卡指回需求页，那里无事可做**

项目 C `b6da7a1c` / 需求 `6ca177e8`：15:03:25 开卡「验收 · 输出当前登录用户名的中文简报」高风险，`deep_link=/workflows/{demand}`（`adapters.go:94-98` 优先用 `primary_demand_id`）。打开该页只有"验收判据血缘（全部已满足）"，**没有任何验收动作**，必须退回收件箱才能处理。

同卡还暴露：列表元数据行直接打印英文枚举 `· project_acceptance`；判据正文是 planner 原文英文；证据条目里有裸 `attestation:project-task-attempt:…` 长 ID。

**F4〔结构〕一份工作被问三到四遍，且都叫"验收"**

炬枢平台 `6e45fb1a` 单条需求的真实决策序列：
```
11:08:07  plan_review              确认项目计划版本            low
11:08:33  project_task_approval    高风险动作需要确认           high   ← 派发前拦
11:14:17  project_task_acceptance  执行存储容量分析并生成报告     low    ← 同一任务跑完再拦
11:16:56  demand_acceptance        需求验收：…                 high
11:18:26  （人类在需求页签署）
11:18:27  project_acceptance       验收 · …                   high   ← 1 秒后
11:24:45  （人类在收件箱同意）→ 项目归档
```
后三张问的都是"这次执行做得对不对"，风险等级还不一致。`project_acceptance` 的标题由 `project_acceptance_presentation.go:104` 生成为 `验收 · {主需求标题}`——用需求名当项目级卡的身份。

`project_task_acceptance` 的触发条件（`project/service.go:5326 projectTaskRequiresAcceptance`）四选一：
1. `ResultContract.HumanReviewRequest != nil`
2. `task.RequiresHumanApproval`  ← **同一标志也触发派发前的 `project_task_approval`，导致双拦**
3. `task.RiskLevel ∈ {high, critical}`
4. `req.RequiresHumanReview`

**F5〔可见性〕「等待人工」信号同时有假阳性和假阴性**

`storage/queries/project.sql:280-302`（`ListWorkflowInstances`）实例状态只由任务计数 + 未决决策推导，**不读 `demand_status`**；且 `waiting_human_nodes` 的过滤条件是 `requires_human_approval OR status IN ('waiting_human','pending_review')`——`requires_human_approval` 是**粘性**的，任务终态后仍计入。

实测：
- 真正卡在待验收的 `dbd24727` → 读模型判 **completed** → 被 `scope=active` 过滤掉（:404-411），从"运行中"视图消失，落进"已归档/已完成"
- 早已 completed 的 `040251ee` / `96aac42d` → 读模型判 **waiting_human**（任务 `87a15aad` 已 completed 但 `requires_human_approval=t`）

**F6〔可见性〕规划失败静默，且无法关闭**

实测两次真实失败：14:38:16 `PlanDemandRoute` 超时（deepseek 上游挂起 >90s，planner 超时 120s / activity 180s）、14:57:08 `unexpected EOF`。两条需求永远停在 `planning_pending`，`workflow-instances` 判为 `planning`，UI 显示"规划中 / 任务正在规划"，**无收件箱事项、无失败态、无取消入口**（API 无关闭/取消需求端点）。存量另有一条同类僵尸已 4 小时 24 分（`c6e894fc`，事件为"项目没有可参与规划的数字员工"）。

**F7〔旁路写入口〕项目页两处与事实相反 / 无效**

- **项目 → 审批 tab**：聚合 decision requests 并宣称"在项目内处理负责人决策"，显示「需求验收：… 已批准」，与真实需求状态相反。
- **项目 → 资产 → 验收**：`project-acceptance-panel.tsx:101` 的自由表单在**已归档项目**上仍可填写提交 → `POST /projects/{id}/acceptance` 409，toast 是英文原文「create project acceptance request failed with status 409: project archived」。验收人字段显示裸 UUID（`:119`）。该端点（`project/service.go:1817 CreateAcceptanceRecord`）**不解决 pending 决策、不归档、不改项目状态**，与闸门完全平行。

**F8〔归档不收敛〕**

炬枢平台 11:21:14 建的计划确认卡，在 11:24:45 项目归档后仍 `open`，直到 14:34:19 才被处理（滞留 3 小时）；且归档后第二条需求继续跑完（11:25–11:36），`maybeOpenProjectAcceptanceReview` 因项目非 running 而静默 no-op，不再产生任何项目级闸。

---

## 2. 根因

**平台没有「人类待办」这个一等业务对象。** 现有的待办是三张表（`approval_requests` / `project_decision_requests` / `inbox_items`）在四个菜单上的六种投影：谁最后写谁说了算，动作集由 `itemType` 兜底而非由 `kind` 决定，深链在三处各自推导。因此"同一件事在不同菜单不同名字、不同按钮、不同落点、不同结论"是结构必然，不是个别 bug。

菜单混乱是症状，投影混乱是病。

---

## 3. 目标与非目标

### 目标
1. 确立唯一的人类待办读模型 `HumanTask`：一个闸门 = 一条待办 = 一套动作契约 = 一个权威落点。
2. 消灭 F1 的卡死路径：卡片上出现的每个动作都必须有已实现的执行路径写业务事实。
3. 把三层验收压成"派发放行 / 阶段放行 / 验收签署 / 结项确认"四种语义不重名的闸，并按下游依赖裁剪任务级拦截。
4. 菜单归位：收件箱是唯一待办入口，业务页是证据现场与同对象视图，删除旁路写入口。
5. 修复"等待人工"信号的假阳性与假阴性；让规划失败可见、可关闭。

### 非目标
- 不改权限中心域（另一份 spec）。
- 不改判据引擎、对抗评审、review gate 的判定逻辑（只改它们产出的**人类待办**如何呈现与执行）。
- 不改飞书卡片的交互形态（但词表与 kind 变更需同步，见 §7.4）。

---

## 4. 目标模型：`HumanTask`

### 4.1 读模型字段

```
HumanTask {
  id                uuid          # 稳定 ID，等于其来源决策请求 ID
  layer             enum          # task | demand | project
  kind              string        # 见 §4.2 注册表
  title             string        # 中文，身份由 layer 决定（见 §4.3）
  why               string        # 一句话说明"为什么需要你"，服务端产出
  evidence          []Evidence    # 判据 / 结论 / 交付物摘录，卡上直接可读
  actions           []Action      # 见 §4.4；允许为空
  primary_surface   string        # 唯一权威落点 URL，服务端唯一来源
  progress          Progress      # {step, total, label}，本闭环进度
  state             enum          # open | resolved | cancelled
  risk_level        enum
  project_id        uuid
  demand_id         uuid?
  task_id           uuid?
}

Action {
  key               string
  label             string        # 中文
  effect            string        # 中文，一句话说明"点了会发生什么"
  handler           string        # 服务端 handler 标识，见 §4.4 注册表
  requires_comment  bool
  kind              enum          # execute | navigate
}
```

### 4.2 kind 注册表（唯一事实源，服务端）

| kind | layer | 中文名 | 何时产生 | 权威落点 |
|---|---|---|---|---|
| `plan_review` | demand | 计划确认 | 计划版本需人类确认 | `/workflows/{demand}` |
| `dispatch_release` | task | **执行放行** | 高风险动作派发前（原 `project_task_approval`） | `/projects/{id}?tab=tasks&focus={task}` |
| `downstream_release` | task | **阶段放行** | 任务完成且**有下游依赖**时（原 `project_task_acceptance`，见 §5.2） | `/projects/{id}?tab=tasks&focus={task}` |
| `acceptance_sign` | demand | **验收签署** | 需求收敛闸挂起（原 `demand_acceptance`） | `/workflows/{demand}` |
| `closure_confirm` | project | **结项确认** | 项目全部需求终态（原 `project_acceptance`） | `/projects/{id}?tab=closure` |
| `planning_failed` | demand | 规划失败 | 规划超时/失败/无可用执行者（**新增**，见 §5.5） | `/workflows/{demand}` |
| `task_failure_recovery` | task | 任务失败恢复 | 不变 | 不变 |
| `planning_gap` | demand | 规划缺口 | 不变 | 不变 |
| 其余 `project_task_*` | task | 保持现名 | 不变 | 不变 |

> **`kind` 只是展示 / 契约层词表，不改底层 `decision_type` 技术值**（拍板 #6）。存量 `project_decision_requests` 行、协调线程 `workflow.go` 的 switch、飞书 `cards.go` 的 5 处 switch 全部依赖现有枚举值；改值要迁移 + 改飞书 + 改协调线程，收益为零。实施时建立 `decision_type → kind` 的单向映射表，`decision_type` 保持不动。

> 命名待确认项：`dispatch_release`（派发前，"这个高风险动作要不要做"）与 `downstream_release`（完成后，"能否让下游基于这个产出继续"）中文名分别取「执行放行」「阶段放行」，两者字面接近。若人类认为易混，把后者改为「下游放行」即可，语义更直白；**其余命名已拍板不再变动**。

### 4.3 标题身份规则

标题的主体名必须与 layer 一致，禁止跨层借名：
- `layer=task` → 任务标题
- `layer=demand` → `验收签署 · {需求标题}`
- `layer=project` → `结项确认 · {项目名称}`（需求清单退到 `why` / `evidence`）

这条直接废掉 F3/F4 里"项目级卡用需求名当身份"的 `project_acceptance_presentation.go:104`。

### 4.4 action → handler 注册表与 CI 断言

服务端维护 `(kind, action.key) → handler` 显式注册表。**CI 断言**：注册表里每个条目都能解析到一个已实现的 handler；反过来，任何 kind 未登记的 action 一律不得下发。这是 F1 的根治手段——"能点"与"点了有用"从此有契约。

首批登记：

| kind | action | handler | 效果 |
|---|---|---|---|
| `acceptance_sign` | `approved` | `SignAllPendingDemandCriteria`（新增，见 §5.1） | 逐条写 human satisfied verdict → 收敛需求 |
| `acceptance_sign` | `rejected` | `SignDemandCriterionUnsatisfied` | 首条 pending 判据写 unsatisfied → 需求失败 |
| `acceptance_sign` | `needs_more_evidence` | `HoldDemandForEvidence` | 保持 acceptance_pending，写审计与意见，卡保留 |
| `closure_confirm` | `approved` / `rejected` / `needs_more_evidence` | 现有 `ApplyProjectAcceptanceDecision` | 不变 |
| `planning_failed` | `retry_planning` / `reassign` / `close_demand` | 见 §5.5 | 重跑规划 / 换执行者 / 关闭需求 |

---

## 5. 变更细则

### 5.1 acceptance_sign：收件箱一键整单通过（拍板 #1）

新增 service 方法 `SignAllPendingDemandCriteria(ctx, tenantID, demandID, actorUserID, reason)`：
- 解析当前有效 plan revision，取 `ResolveUnsatisfiedBlockingCriteria` 得到全部待签判据；
- 逐条调用现有 `SignDemandCriterionVerdict` 的内核（复用其判权、幂等、收敛与 reconcile 逻辑，不新写一条并行路径）；
- 全部 satisfied 后由既有 `convergeDemandSignOff` 完成收敛、解决决策、必要时开结项闸。

约束：
- 只签 human-signable 判据（`human_judgment` / `adversarial_review` / `review_gate`）。
- **遇到未满足的 `automated_test` blocking 判据 → 整体拒绝**（拍板 #7），返回明确中文错误「还有 N 条自动判据未满足，无法整单通过」。人类签不了 automated_test（`service.go:6443` 会 `ErrInvalidProject`），半签只会制造新的不一致。
- 任一条签署失败则整体失败并返回明确错误，不留半签状态（reconcile 路径已能自愈，但接口必须报错而非静默）。
- 卡片 `evidence` 必须携带 `pending_criteria_detail`（判据原文 + 证据摘录），**收件箱上直接可读**——不能让人在看不到判据的情况下点通过（当前弹窗完全不展示判据，见 F1 现场）。这是 Console 允许一键的**前提条件**，与飞书"签署紧邻证据"的原则同源。

**覆盖对抗评审与违规检测的边界（拍板 #8）**：一键通过**可以**覆盖 `adversarial_review` 判官驳回与 `review_gate` 检出的违规（既有 tier-3 human override 语义，`service.go:6433-6448`），但：
- 待签集合中一旦含 `adversarial_review` / `review_gate` 判据，**处理意见变为必填**（覆盖理由），前端与服务端都要校验；
- 卡片必须显式警示「含 N 条对抗评审驳回 / M 条检出违规」，并列出它们的判据原文与判官/检测结论；
- 审计事件与 verdict 的 `reason` 必须标记为**人类覆盖**（`override=true` 或等价字段），使"人类推翻了 AI 判定"在审计中可检索——这是自治姿态基线里"人类守门可覆盖"的落账要求，不能静默通过。

**飞书渠道不跟进一键（拍板 #9）**：`apps/feishu-connector/internal/cards/cards.go:399-401` 刻意维持卡内逐条签署、不给一键全过，本次**保持不变**。两渠道交互粒度不同但原则一致（签署紧邻证据）。实施时不得顺手给飞书加"全部通过"。

### 5.2 任务级拦截按下游依赖裁剪（拍板 #2）

改写 `projectTaskRequiresAcceptance`（`project/service.go:5326`）：

```
需要 downstream_release  ⟺  该任务存在未终态的下游依赖任务
                             AND (provider 请求人类复核 OR runtime 上报 RequiresHumanReview
                                  OR task.RiskLevel ∈ {high, critical})
```

同时：
- **删除 `task.RequiresHumanApproval` 作为完成后拦截条件**——它已在派发前触发 `dispatch_release`，不得双拦（F4）。
- 叶子任务（无下游）一律不拦，产出直接汇入需求判据证据。
- `RiskLevel=high` 的叶子任务如需人类把关，由 planner 在需求判据里注入 `human_judgment` 判据（既有 `ensureHumanJudgmentCriterion` 机制），在 `acceptance_sign` 一次性处理。

下游依赖的判定用既有 task graph（`ResolveReadyDownstream` 同源），实施时复用同一查询，不新造口径。

### 5.3 结项确认与「通过并结项」（拍板 #3）

- `project_acceptance` → `closure_confirm`，标题以项目名为身份，动作文案改为「确认结项并归档 / 退回返工 / 要求补证」。
- 需求页验收签署面板增加「通过并结项」勾选框，**默认不勾选**。勾选时前端在签署请求上带 `also_close_project=true`，服务端在 `convergeDemandSignOff` 判定项目已可结项后，用同一 actor 直接执行结项（写同样的审计与 acceptance record），不再产生 `closure_confirm` 卡。
- 未勾选则维持现状：签署后开一张 `closure_confirm` 卡。

### 5.4 投影合一与深链单一来源（治 F2/F3）

1. **一个闸门只写一条待办、只由一条管道写**：`ApprovalProjectorAdapter` 不再向 inbox 投影**已有对应 decision request** 的审批（`approval_requests` 退回审计/授权记录）。保留它对无 decision 对应的纯审批类事项的投影。
2. resolve 路径禁止字段回退：`DecisionProjectorAdapter.upsert` 对 `context_payload` / `source_project_id` / `source_task_id` 用 `COALESCE(EXCLUDED.x, inbox_items.x)` 语义（SQL 层改 `UpsertInboxItem*` 的 DO UPDATE 子句），resolve 时不得把已有上下文清空。
3. **`primary_surface` 是唯一深链来源**。删除以下三处各自推导：
   - `inbox/adapters.go:90-98` 的 route 分支（改为写服务端计算好的 `primary_surface`）
   - `apps/web/src/features/inbox/components/inbox-action-dialog.tsx:162` 的 `reviewHref`
   - `apps/web/src/features/inbox/components/inbox-shell.tsx` 的 `resolveInboxHref` / `resolveWorkflowInstanceHref` / `resolveWorkflowTemplateHref`
4. **删除 `inbox/service.go:363-365` 的空动作兜底**。空动作集合法：前端渲染单个「去处理」导航按钮跳 `primary_surface`。

### 5.5 规划失败可见可关闭（治 F6）

- `PlanDemandRoute` 活动最终失败（重试耗尽）时，把需求置为新状态 `planning_failed`，并产出 `kind=planning_failed` 待办，`why` 带失败原因（超时 / 上游错误 / 无可用执行者）。
- 动作：`retry_planning`（重跑规划）、`reassign`（跳转补员）、`close_demand`（关闭需求）。
- 新增 `POST /api/v1/project-demands/{demandId}/close`（关闭需求，写审计，需求转 `cancelled`）——当前 API 完全没有关闭需求的能力，这是僵尸永存的直接原因。
- 状态词表拆分：`planning_pending` 不再一词多义，拆为「排队规划中 / 等你确认计划 / 规划失败」三态映射（`status-labels.ts`）。

### 5.6 「等待人工」信号修复（治 F5）

改 `storage/queries/project.sql` 的 `ListWorkflowInstances`：
- 实例状态改为 **`demand_status` 优先**：`acceptance_pending → waiting_human`（标签「待验收」）、`planning_failed → failed`；其余维持任务推导。
- `waiting_human_nodes` 的过滤改为 `status IN ('waiting_human','pending_review') OR (requires_human_approval AND status NOT IN (终态集))`——终态任务不再永久计入。

### 5.7 归档收敛（治 F8）

- `ArchiveProject` 事务内级联把该项目所有 `open` 待办置 `cancelled`（写审计原因"项目已归档"）。
- 已归档项目拒收新需求（`SubmitProjectDemand` 返回 409 + 中文文案）。

---

## 6. 菜单归位（IA）

### 6.1 收件箱 = 唯一待办入口
- 按 layer 分组呈现：**计划确认 / 执行放行 / 阶段放行 / 验收签署 / 结项确认 / 异常处理**。
- 每张卡带闭环进度条：`任务 1/1 → 验收签署 待你 → 结项 未开始`（数据来自 `HumanTask.progress`）。
- 卡片正文展示 `why` + `evidence`（验收签署卡必须展示判据原文与证据摘录）。
- 元数据行禁止出现英文技术枚举（F3 的 `· project_acceptance`），一律经 `status-labels.ts` 映射。
- **词表单一事实源**：中文词表目前有两套独立实现——web `apps/web/src/lib/status-labels.ts` 与飞书 `apps/feishu-connector/internal/cards/cards.go:531 decisionTypeLabel`。本次的 kind 词表必须两处同步，否则 Console 与飞书对同一闸门说法分叉。实施时在飞书侧加护栏测试（对齐 `status-labels.guard.test.ts` 的做法），断言两套词表的 key 集合一致。

### 6.2 流程编排 = 证据现场
- 保留验收签署面板（它与收件箱卡是**同一 HumanTask 的两个视图**，任一侧提交，另一侧秒级消失）。
- 移除其它决策入口。
- 判据文案中文化：planner 提示词约束判据 `statement` 必须中文（F3 实测为英文原文）；存量英文判据不回填。

### 6.3 项目管理
- **审批 tab → 只读「决策历史」**：去掉"在项目内处理负责人决策"的第二写入口与动作按钮，只展示历史与跳转。
- **资产 → 验收 子 tab → 独立「结项」tab**：有 pending `closure_confirm` 时渲染与收件箱同一动作集；无则只读结论；已归档只读。
- **删除自由验收表单**（拍板 #4）：下线 `POST /api/v1/projects/{projectId}/acceptance` 与 `project-acceptance-panel.tsx` 的提交路径。
  - 调用方已核对（2026-07-24）：路由 `api/server.go:431` → `project/handler.go:1249 CreateAcceptance` → `project/service.go:1877/1817`，**该 service 方法只被这个 HTTP handler 使用**；飞书 connector 无调用（`apps/feishu-connector/internal/cpclient/client.go` 无匹配）。归档闭环走的是 `ApplyProjectAcceptanceDecision` → `repository.CreateAcceptanceRecordWithEvent`，与本端点无关。
  - 因此可整条删除：路由 + handler + `Service.CreateAcceptance`/`CreateAcceptanceRecord` + 契约（`contracts/control-plane/openapi.yaml:1935` 的 `/api/v1/projects/{projectId}/acceptance` POST）+ web `createProjectAcceptance`。**仓储层 `CreateAcceptanceRecordWithEvent` 必须保留**。
  - 契约删除后走 `generate:control-plane` 与 `verify:contracts`。
- 验收人等主体一律显示名称（服务端补名），禁止裸 UUID。

### 6.4 权限中心
不再承接业务验收类深链（`/approvals` 深链随 `primary_surface` 改造自然消失）。

---

## 7. 实施顺序

### P0 止血（各项独立可分别上线）
1. **删 `inbox/service.go:363-365` 空动作兜底 + 为 `acceptance_sign` 登记真实 handler（§5.1）** —— 消除 F1 卡死路径。这一条必须最先做。
2. resolve 投影不清空 `context_payload` / `source_project_id`（§5.4.2）。
3. `primary_surface` 单一来源，前端三处推导下线（§5.4.3）。
4. `ListWorkflowInstances` 状态口径修复（§5.6）。
5. 归档级联取消未决待办（§5.7）。

### P1 结构
6. `HumanTask` 契约（`contracts/control-plane/openapi.yaml` + `generate:control-plane`）与投影合一（§4、§5.4.1）。
7. kind/action 注册表 + CI 断言（§4.4）。
8. 任务级拦截按下游依赖裁剪 + 去双拦（§5.2）。
9. 项目页两个写入口降级 / 下线（§6.3）。
10. 规划失败待办 + 关闭需求 API（§5.5）。

### P2 体验
11. 词表分层与全站命名替换（§4.2）、闭环进度条、收件箱分组（§6.1）。
12. planner 判据中文化约束、裸 UUID / 裸英文枚举清理。
13. 「通过并结项」合并动作（§5.3）。

---

## 8. 验收标准（真实链路，非 mock）

每条都必须在真实服务上跑通并留证据，`verify:*` 通过不等于完成：

- **G1**：新建单任务需求跑到 `acceptance_pending`，在**收件箱**点「同意」→ DB 里出现 human satisfied verdict、需求转 `completed`、决策 resolved、卡消失。（直接反证 F1）
- **G2**：同上场景改为在**需求页**逐条签署 → 收件箱对应卡 5 秒内消失；反向亦然（收件箱处理后需求页闸门消失）。
- **G3**：签署后开出的结项卡，标题以**项目名**为身份，`primary_surface` 落在项目结项 tab，且该页可直接完成结项。（直接反证 F3）
- **G4**：多任务需求中，有下游的任务触发「阶段放行」，叶子任务不触发；`RequiresHumanApproval` 的任务只在派发前拦一次。（反证 F4）
- **G5**：制造规划失败（可临时指向不可用 planner）→ 出现「规划失败」待办，`close_demand` 能把需求关掉，列表不再显示"规划中"。（反证 F6）
- **G6**：`acceptance_pending` 需求在流程编排"运行中"视图可见且标为待验收；已完成且曾 `requires_human_approval` 的实例不再显示等待人工。（反证 F5）
- **G7**：归档项目后，该项目所有未决待办即时变 `cancelled`；已归档项目的结项 tab 只读、无可提交表单。（反证 F7/F8）
- **G8**：全站抽查无裸 UUID / 无英文技术枚举出现在用户可见位置；`status-labels.guard.test.ts` 通过。

门禁：`verify:control-plane`、`verify:web`、`verify:contracts`、`make -C apps/control-plane migrate-validate`（如有迁移）。

---

## 9. 人类拍板结论（2026-07-24）

1. **`acceptance_sign` 在收件箱给"一键整单通过"**（不是只给导航）。→ §5.1
2. **任务级拦截按下游依赖保留**：有下游保留并改名「阶段放行」，叶子任务砍掉，`RequiresHumanApproval` 双拦无条件修掉。→ §5.2
3. **「通过并结项」默认不勾选**。→ §5.3
4. **项目页自由验收表单下线**（端点一并下线，下线前核对飞书调用方）。→ §6.3
5. **命名采用「执行放行 / 阶段放行 / 验收签署 / 结项确认」**；仅「执行放行 vs 阶段放行」字面接近，若需更直白可把后者改为「下游放行」，其余不再变动。→ §4.2
6. **`kind` 只做展示 / 契约层词表，`decision_type` 技术值不动**（避免迁移 + 飞书 + 协调线程三处破坏，收益为零）。→ §4.2
7. **一键通过遇未满足的 `automated_test` 判据 → 整体报错**，不做部分签署。→ §5.1
8. **一键通过可覆盖对抗评审驳回与检出违规，但强制填覆盖理由**，卡片显式警示，审计标记人类覆盖。→ §5.1
9. **飞书维持卡内逐条签署，不加一键全过**；Console 一键的前提是卡上直接展示判据与证据。→ §5.1

---

## 11. 开工前置清单（2026-07-24 实测，按阻塞度排序）

| # | 事项 | 状态 | 说明 |
|---|---|---|---|
| 1 | **修 `verify:control-plane` 编译失败** | **阻塞** | `internal/runtime/scheduler_test.go` 的 `mockSchedulerRepository` 缺 `GetNodeByID`（`Repository` 接口在 07-23 `461855ed` 加的方法）。门禁当前是红的，不修无法判断本 spec 的改动是否引入回归。约 2 行。 |
| 2 | 分支与 worktree | 待做 | 共享 checkout 规则：开 `feat/human-task-unification` 分支 + 独立 worktree 再动手，禁止在主 checkout 切分支。 |
| 3 | planner 稳定性 | 待定 | dev 配置 `apps/control-plane/config/config.yaml`（已 gitignore）planner.model = `deepseek-v4-pro`，07-24 复验中 3 次上游挂起（>90s）致 `PlanDemandRoute` 超时；同期 `deepseek-chat` 0.7s 正常。G1/G4/G5 都要走真实规划，建议临时切 `deepseek-chat`，或接受重试成本。 |
| 4 | dev 数据清理 | 待定 | 复验遗留见 §10。建议保留 `dbd24727`（G1 回归夹具），其余清掉，否则污染 G5/G6 的列表断言。 |
| 5 | G4 夹具设计 | 待做 | G4 需要"多任务且有依赖"的需求，任务数由 planner 决定。需先设计一条必然分解成 2+ 有依赖任务的需求文案并试跑确认，再写断言。 |
| 6 | 基线记录 | 已完成 | 2026-07-24 15:26 于 `20488eb8`：`verify:web` PASS（119 文件 / 911 用例）、`verify:contracts` PASS、`verify:control-plane` **FAIL**（见第 1 项）。 |

---

## 10. 复验遗留物（实施前可清理）

复验在 dev 库留下的数据，与本 spec 实施无关，但会干扰 E2E：

| 对象 | 状态 | 说明 |
|---|---|---|
| 项目 `4627824a` 验收菜单复验 | running | 含**卡死需求** `dbd24727`（F1 现场）+ 僵尸规划需求 `84fd333d` |
| 项目 `72e20451` 验收菜单复验B | running | 含僵尸规划需求 `d18649ff` |
| 项目 `b6da7a1c` 验收菜单复验C | archived | 闭环干净，可留作对照 |
| 需求 `c6e894fc`（流程空态验证 `d52727ee`） | planning_pending | 存量僵尸，已滞留 >4 小时 |

建议保留 `dbd24727` 直到 G1 修复验证完成（它是最好的回归夹具），其余可清理。
