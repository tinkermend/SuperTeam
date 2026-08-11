# 项目管理首页：项目—任务双层状态组合读模型技术方案

- 日期：2026-08-11
- 状态：提案 v2（已按代码核对复审修订）；**P-1 两项已于 2026-08-11 由人类拍板**（可见性 = C、IA = A），可开工
- 范围：Console `/projects` 列表态 + Control Plane 项目组合读模型 + 数据库聚合与形态门禁
- 原型：`docs/prototypes/project-management-home-redesign-v2/final-layered-project-task-status.png`
- 前置规范：
  - `docs/superpowers/specs/2026-07-29-project-status-layers.md`
  - `docs/superpowers/specs/2026-08-10-projects-home-portfolio-hygiene-design.md`（其实施提交为 `a5c57247`，本方案的现状基线）

> 本方案是 2026-08-10 首页治理后的下一阶段。若实施，本方案将取代其“本期不新建 Project Home 读模型”和“仅做 50 条截断提示”的阶段性约束；原 `GET /api/v1/projects/run-summary` 继续服务运行总览，不扩成第二个项目首页专用契约。

### v2 修订记录（2026-08-11，按代码核对复审）

v1 的领域分层结论保留，落地部分按下列结论重写。每条都对应一处已核实的代码事实，不是风格偏好。

| # | v1 的问题 | v2 处置 |
|---:|---|---|
| 1 | 契约把 `waiting_human`（宽口径）与 `open_decision_count` 并列，重新引入 `a5c57247` 刚修掉的 P0 双计 | 新增 §3.3 承重规则 + `attention.waiting_human_unlinked_count` 字段 + 专项测试 |
| 2 | 要求重写 `GetProjectTaskStatusCounts`，但它是 `archive_readiness` 的闸门输入，与 §2.2 非目标冲突 | 新增 §5.2.1 冻结 `ActiveTasks`、展示桶与闸门互不派生 + 对照测试 |
| 3 | 「沿用项目列表可见性决策点」——该决策点实为租户级，等于放行 | §5.4.1 三选项已拍板 **C**：租户级默认 + `mine_only` 开关，谓词与 `ListWorkflowInstances` 同源 |
| 4 | 无 as-is 盘点；要重写的页面 1 天前刚大改 | 新增 §1.3 去留表；IA 冲突已拍板 **A**（卡片网格取代队列，`ProjectRiskQueue` 退役需先证明能力有落点） |
| 5 | 两个新增索引与既有索引重复，其中一个比既有更差 | §5.3 改为默认不落、以 EXPLAIN 为准，移出 P0 |
| 6 | 2 秒 SLO + 20 并发压测，在 50/500 规模下无判别性；且性能问题 `a5c57247` 已解决 | §7 压缩为形态门禁 + 回归门禁；§1.2 改写立项理由为口径完整性与分页诚实 |
| 7 | 文件清单漏 `project-risk.ts`（web 侧口径事实源）；`failed` 拆桶会让健康度静默降级 | §3.5 连带影响表 + §6.3 健康度接线表 + P3 补文件 |
| 8 | 「严禁全租户聚合」与自身的全局 summary 自相矛盾 | §5.1 限定禁令只对逐卡桶成立 |
| 9 | 默认排序 `created_at DESC`，关注项会被埋到后页 | §4.2 新增 `sort`，默认 `attention` |
| 10 | 桶和不变量失败即拒绝输出 = 自造停机面 | §9 改为降级返回 + `counts_degraded` + 告警 |
| 11 | `completed_today_count` 正在被首页使用但契约未覆盖；`digital_employee_count` 口径未定义 | §4.3 补处置方案与口径钉死表 |
| 12 | 把 `in_progress` 等防御值描述成「修复窄口径」 | §3.2 说明其在 `project_tasks` 上无写入方，不得作为修复理由 |

---

## 1. 背景与问题

### 1.1 领域问题

项目是业务闭环容器，一个项目内存在 N 个项目任务。合法情况下：

- 项目只有一个生命周期状态，用于回答项目是否可调度、是否进入验收、是否归档；
- 项目内多个任务可以同时处于待执行、排队、运行、等待人类、阻断、失败、完成或取消；
- 项目健康度是任务、决策、证据与协调信号的派生结果，不是项目生命周期状态。

因此首页不得把项目压缩成一个“当前执行状态”，也不得因为某个任务失败就把项目生命周期改成失败。目标页面采用双层表达：

```text
项目层：项目总数 / 生命周期分布
任务层：非归档项目内全部任务的互斥状态分布

项目卡：项目生命周期 pill + 任务状态构成条 + 健康度 + 最近活动
```

现有接口只能支撑部分原型：

- `GET /api/v1/projects` 有项目名称、目标、生命周期和负责人，但只返回数组，没有总数与分面统计，也没有 `has_more`；
- `GET /api/v1/projects/run-summary` 有运行、排队、等待人类、失败等运行信号，但没有累计完成、取消和总任务数，且部分桶不是互斥状态桶；
- `GET /api/v1/projects/{id}/overview` 有单项目全量任务统计，但首页逐项目调用会产生 N+1；
- 项目成员、工件均只有单项目接口；系统没有独立“里程碑”领域对象。

结论：不能在前端拼装一个看似完整的任务构成条。必须增加一次性、跨项目、口径完整的首页读模型。

### 1.2 立项理由的口径（不是性能）

**本方案不以性能为立项理由。** `a5c57247`（2026-08-11）已经把列表态从「当前页 40 次 fan-out」改走一次租户级聚合，首屏由约 46 请求降到 3~4 请求。相对现状，本方案再省下的是 3 个廉价并行请求，量级在 100ms 内，不足以支撑一个新端点。

真正立得住的理由只有两条，实施与验收都应围绕它们展开：

1. **口径完整性**：`run-summary` 的桶不互斥且缺 `completed`/`cancelled`/`total`，前端无法拼出「每条任务恰好落一个桶、桶和等于总数」的构成条——这是原型的核心表达，缺一不可；
2. **分页诚实**：`listProjects` 无 `total`/`has_more`，现状只能在满 50 时显示截断提示。项目上限一旦被真实租户突破，页面就在说谎。

性能相关内容退化为回归门禁（不得比现状更差、首屏不得逐项目 fan-out），不再设立独立的 SLO 工程（见 §7）。

### 1.3 现状盘点（as-is）：`/projects` 列表态今天有什么

本节是范围边界。`apps/web/src/features/projects/index.tsx` 现为 1844 行、配套 `index.test.tsx` 113KB，且列表态在 `a5c57247`（1 天前）刚做过大改。**未在下表中标注去留的现有能力，一律默认「保留且不得回归」。**

列表态数据源（`enabled: !routeProjectId`）：

| 来源 | 用途 | 本方案去留 |
|---|---|---|
| `listProjects(≤50, q/status)` | 项目实体、名称、目标、负责人 | **由 portfolio 取代** |
| `listProjectRunSummaries(limit 500)` | 风险/关注投影、队列排序、组合透视、`today_completed_run_count` | **列表态由 portfolio 取代**；端点保留服务运行总览（见 §4.3 对 `completed_today_count` 的处置） |
| `listDigitalEmployees` | 负责人/处理人名称回填 | 保留（名称目录，与 portfolio 正交，`staleTime: 60s`） |
| `listUsers(limit 200)` | 同上 | 保留 |
| `useProjectRiskSignals`（tasks/decisions/evidence/members × limit 100） | **仅选中队列项目**的 triage 明细 | 保留；它本就不在首屏，不构成 fan-out |

列表态 UI 单元（现均在 `components/project-risk-home.tsx`，1018 行）：

| 现有单元 | 现职责 | 本方案去留 |
|---|---|---|
| `ProjectPortfolioSummaryBar` | 顶部组合真值条 | **改造**为 §6.2 的双层摘要，不新建重名组件 |
| `ProjectRiskQueue` | 关注摘要/执行摘要两列队列 + chip 筛选 | **退役**（IA 拍板 A）；chip 筛选并入新 toolbar，退役前须逐项证明能力有落点 |
| `ProjectTriagePanel` | 右栏选中项目的可行动明细 | 保留，未选中时不加载 |
| `ProjectPortfolioPerspectivePanel` | 未选中时的右栏组合透视（方案 C，零额外请求） | 保留；数据源从 run-summary 换为 portfolio |
| chip 默认收敛 + 「更多筛选」 | 风险筛选 | 保留，并入 §6.2 toolbar |
| 满 50 截断提示 | 诚实性兜底 | **删除**，由真分页取代（§4.2） |
| 「我的待办」弱链（不带数字） | 计数唯一出口是侧栏角标 | 保留，本方案不得给它加数字 |
| `reasonLabels`（已迁 `status-labels.ts`） | 中文词表 | 保留，新增桶词表并入同一出口 |
| 「等待超时」chip | 已于 `a5c57247` 下线（run-summary 无等待起点，恒 0） | 保持下线，不得复活 |

**IA 冲突 — 已拍板 A（2026-08-11）**：现状是「队列 + 右栏 triage」的主从布局（`MasterDetailLayout` 语义），原型与 §6.2 是「卡片网格」，二者不是同一个 IA。曾考虑的三选项：

- **A（已选）**：卡片网格取代队列作为主区，`ProjectTriagePanel` 降为卡片点击后的右栏/抽屉。改动最大但与原型一致；
- B：保留队列为主区，仅把任务构成条嵌入队列行。改动最小，但原型的项目卡表达基本落空；
- C：卡片/队列视图切换。成本最高，两套 UI 都要维护四态与可访问性。

**A 的实施约束**：`ProjectRiskQueue`（`project-risk-home.tsx:152`）退役前，其 chip 筛选、负责人行、处理人标签、空态文案必须逐项在新 toolbar / 卡片上找到落点并在 §8.2 有断言。退役是删代码，不是搬代码——删之前先证明功能有去处，见 §11 第 5 步。

顺带确认：§6.3 的「卡片/列表切换只改变表现」指的是同一份 portfolio 响应的两种密度呈现（沿用现有列表态密度习惯），**不是**恢复队列 IA。

---

## 2. 目标、非目标与成功标准

### 2.1 目标

1. 项目生命周期与项目任务状态在契约、界面和词表上明确分层。
2. 单次请求返回首页项目卡、顶部组合统计与分页元数据，不产生逐项目 fan-out。
3. 每个任务恰好落入一个任务状态桶，所有桶之和等于任务总数。
4. 支持项目状态、负责人、任务状态、关键词筛选，并保持服务端分页语义诚实（真 `total` / `has_more`，删除截断提示）。
5. 首屏请求数与延迟**不劣于现状**（`a5c57247` 后的 3~4 请求基线），且不引入逐项目 fan-out。
6. 保留现有风险/关注投影能力，但健康度不得覆盖或替代项目生命周期。

### 2.2 非目标

- 首页不返回或渲染 500 条任务明细。
- 首页不处理审批、决策、失败清理或证据核验；这些动作仍在收件箱或项目内完成。
- 不新增“里程碑”写模型；首页不伪造里程碑。
- 不把工件、成员、事件逐项目加载到首页。
- 不修改项目、需求、任务、人类决策之间的既有状态推进规则。**特别地：本方案对 §5.2 所改查询的闸门语义为零变更**，`archive_readiness` 的 `active_tasks` 阻断项行为必须逐字节不变（见 §5.2 的硬约束与 §8.1 的对照测试）。
- 不替换运行总览使用的 `GET /api/v1/projects/run-summary`。
- 不重做侧栏待办角标、收件箱与「我的待办」的计数出口。

### 2.3 成功标准

打开 `/projects` 后，用户可回答：

> 有哪些项目、各自生命周期如何、每个项目内部的任务状态构成如何、哪些项目需要关注、从哪里进入项目。

同时满足：

- 页面任一数字都可说明统计范围与权威来源；
- 项目状态计数、任务状态计数与项目内详情使用同一桶定义；
- 首屏不发生 `/overview`、`/members`、`/artifacts` 的逐项目请求；
- 项目卡任务桶之和严格等于 `total_tasks`。

---

## 3. 关键领域决策

### 3.1 状态分层

| 层 | 权威对象 | 首页表达 |
|---|---|---|
| 项目生命周期 | `projects.status` | `项目状态 · 已就绪/验收中/...` |
| 项目任务执行 | `project_tasks.status` + 审批等待判据 | 任务构成条与数量 |
| 项目健康度 | 任务/决策/证据/协调投影 | `正常/需关注/阻断` |
| 人类待办 | `project_decision_requests` / Inbox | 首页仅作项目级关注信号，不提供处理动作 |

项目生命周期和任务状态字符串不要求一致。项目卡不得根据任一单任务状态改写项目生命周期。

### 3.2 互斥任务状态桶

新增服务端单一事实源 `ProjectTaskPortfolioBucket`，按下列优先级把每条未 dismissed 任务归入一个桶：

| 优先级 | 桶 | 原始状态/条件 | 中文 |
|---:|---|---|---|
| 1 | `cancelled` | `cancelled` | 已取消 |
| 2 | `completed` | `completed/done/success` | 已完成 |
| 3 | `failed` | `failed/error` | 失败 |
| 4 | `blocked` | `blocked` | 阻断 |
| 5 | `waiting_human` | `waiting_human/pending_human/pending_review/approval_required`，或仍处非终态且 `requires_human_approval=true` | 等待人类 |
| 6 | `running` | `running/in_progress` | 运行中 |
| 7 | `queued` | `queued` | 排队中 |
| 8 | `pending` | `pending/planned/assigned` | 待执行 |
| 9 | `other` | 未登记的防御值 | 其它 |

不变量：

```text
total_tasks = pending + queued + running + waiting_human
            + blocked + failed + completed + cancelled + other
```

`other > 0` 必须记录结构化告警；前端显示“其它 N”，不得静默丢弃。

关于表中的防御值，实施前先读清楚，不要把防御当修复：

- `project_tasks.status` 的实际写入方是 `pending/planned/assigned/running/queued/waiting_human/completed/failed/cancelled`（迁移 013 列注释 + `types.go` 的 `ProjectTaskStatus*` 常量 + `runtimeWritebackProjectTaskStatuses()`）。
- `in_progress/done/success/error/pending_human/pending_review/approval_required` 目前**在 `project_tasks` 上没有写入方**（`in_progress` 只出现在 `employee_execution.sql`）。它们进桶表纯属防御，与 web 侧既有状态集对齐即可，**不得**在 CHANGELOG 或 §5.2 里描述成「修复了窄口径」。
- `blocked` 有写入方，且是本方案唯一真正改变归属的状态——见 §3.5。

### 3.3 等人计数与决策计数的双计规避（承重）

`ListProjectRunSummaries`（`queries/project.sql:2740`）刻意维护**两个**等人字段，且 `a5c57247` 的复审把它定级为 P0 回归修复，实施时不得回退：

| 字段 | 口径 | 消费方 |
|---|---|---|
| `waiting_human_count` | 宽口径，所有卡在人身上的任务，不去重 | 运行总览大屏「待人工」badge 与 `hasActive` |
| `waiting_human_unlinked_count` | 再排除已有 open decision 挂同 `project_task_id` 的任务（orphan） | **项目首页**，因为同屏另有「待决」桶 |

`project-risk.ts:423` 的 `buildProjectRiskSummaryFromCounts` 取的是 `waiting_human_unlinked_count`。实测 `provider-semantic-e2e-p1`：宽口径 19 / orphan 2 / `open_decision_count` 18。

本方案的 §3.2 互斥桶 `waiting_human` **天生是宽口径**（一条任务只能进一个桶，不能因为挂了决策卡就不计）。因此契约必须同时承载两个语义，否则项目卡会渲染成「等待人类 19」+「待决 18」= 用户读到 37 项待办，真值约 20——正是被修掉的那个双计。

**规则（三条同时成立才算实现正确）**：

1. `task_counts.waiting_human` 是**任务层**互斥桶，宽口径，只用于任务构成条与桶和不变量；
2. `attention` 层新增 `waiting_human_unlinked_count`（见 §4.3），**健康度/关注投影只许读这个字段**，不许读 `task_counts.waiting_human`；
3. UI 上任务构成条与 attention 分属两个带独立标签的区域，**不得让二者出现在同一个可相加的行内**（§6.3）。任何把「等人」与「待决」并列成一串数字的呈现都要能通过「两数之和是否会被误读为待办总量」这一问。

### 3.4 统计范围

- 项目层：当前租户全部未软删除项目，包含归档；顶部标签为“全部项目”。
- 任务层：未归档项目中的未 dismissed 项目任务；顶部明确标注“非归档项目任务”。
- 筛选后的项目数量：`pagination.total`；不改变顶部全局组合快照。
- 项目卡：展示该项目全部未 dismissed 任务；归档项目仍可显示历史任务构成，但健康度固定为中性。
- 归档项目卡的任务不计入顶部 `active_project_task_counts`，因此**各卡 `total` 之和 ≠ 顶部 total 是设计内的**；两处标签必须写清范围，UI 不得诱导相加。

### 3.5 `blocked` 从 failed 家族拆出的连带影响

这是本方案唯一真正改变既有数字的分桶动作，必须整链同步，否则同屏两个数字对不上：

| 位置 | 现状 | 改后 |
|---|---|---|
| `ListProjectRunSummaries.failed_count`（`project.sql:2740`） | `failed/error/blocked` | 保持不变（运行总览继续用宽失败） |
| portfolio `task_counts.failed` | — | 仅 `failed/error` |
| portfolio `task_counts.blocked` | — | 仅 `blocked` |
| `project-risk.ts:134` `failedTaskStatuses` | `{failed, error, blocked}` | 保持不变（它作用于任务明细，triage 仍需宽失败） |
| `project-risk.ts` `countBuckets.executionFailed` | `= counts.failed_count`（含 blocked） | **必须改为 `failed + blocked`**，否则只有 blocked 任务的项目健康度静默降级为「正常」 |

换言之：**契约层拆桶，健康度层重新合并**。健康度关心的是「执行受阻」，failed 与 blocked 同族；构成条关心的是「每条任务在哪」，二者必须可分。§10 的 P3 必须包含 `project-risk.ts` 的这处改动与对应单测。

---

## 4. 推荐架构

### 4.1 新增专用读模型接口

新增：

```http
GET /api/v1/projects/portfolio
```

不继续扩展 `run-summary` 的理由：

1. `run-summary` 是运行总览运行带，排序、归档范围和“待人工”语义均服务运行视角；
2. 项目首页需要项目目标、负责人、完整任务构成、组合统计和真实分页；
3. 强行复用会让一个响应承担两个不同产品语义，并增加运行总览连带回归风险。

### 4.2 请求参数

| 参数 | 类型 | 默认 | 说明 |
|---|---|---:|---|
| `q` | string | 空 | 搜索项目名称、目录名、目标 |
| `project_status` | string[] | 全部 | 项目生命周期筛选，可重复传参 |
| `owner_user_id` | uuid | 空 | 人类负责人筛选 |
| `task_state` | enum | 空 | 返回至少含一个该桶任务的项目 |
| `mine_only` | bool | `false` | 仅返回我负责或参与的项目；谓词与 `ListWorkflowInstances` 同源（§5.4.1） |
| `sort` | enum | `attention` | `attention` \| `recent` \| `created`，见下 |
| `limit` | int | 12 | 1～50 |
| `offset` | int | 0 | 服务端分页 |

`mine_only` 同时作用于 `items` 与 `summary`，两者恒为同一可见集合。

**`sort` 是必需参数，不是可选增强。** 现状 `ListProjectRunSummaries` 的排序是「有失败/待人工的项目优先，其次最新活动时间」；若照 §5.1 草稿的 `ORDER BY created_at DESC` 出货，50 个项目 12 条一页时，一个很久以前创建、今天失败的项目会沉到第 4 页——直接违背 §2.3 的「哪些项目需要关注」。

| 值 | 排序键 | 用途 |
|---|---|---|
| `attention`（默认） | `(failed + blocked + waiting_human_unlinked + open_decision) > 0` DESC，然后 `last_activity_at` DESC，再 `id` DESC | 与现状队列排序同源，保证换页不丢关注项 |
| `recent` | `last_activity_at` DESC, `id` DESC | 「最近在动的」 |
| `created` | `created_at` DESC, `id` DESC | 稳定枚举，导出/对账用 |

三者均以 `id DESC` 收尾保证 offset 分页的确定性（`last_activity_at` 可为 NULL，需 `NULLS LAST` 并回退 `projects.updated_at`，与现状 `COALESCE(t.last_activity_at, p.updated_at)` 同源）。

首版使用 offset 分页即可：项目上限为 50，成本和一致性可控。若未来突破该上限，再升级游标分页。**注意 `attention` 排序键随任务状态变化**，翻页期间排序可能漂移；这是 offset 分页的固有代价，本方案接受（页面有手动刷新与 `staleTime`，不做快照隔离）。

### 4.3 响应契约

```json
{
  "summary": {
    "total_projects": 50,
    "project_status_counts": {
      "draft": 2,
      "configuring": 3,
      "running": 27,
      "paused": 1,
      "acceptance": 12,
      "archived": 5
    },
    "active_project_task_counts": {
      "total": 500,
      "pending": 35,
      "queued": 42,
      "running": 68,
      "waiting_human": 21,
      "blocked": 7,
      "failed": 9,
      "completed": 301,
      "cancelled": 15,
      "other": 2
    }
  },
  "items": [
    {
      "project": {
        "id": "uuid",
        "name": "provider-semantic-e2e-p1",
        "goal": "...",
        "status": "running",
        "human_owner_user_id": "uuid",
        "human_owner_user_ids": ["uuid"],
        "coordination_status": "running",
        "updated_at": "2026-08-11T00:51:00Z"
      },
      "owner": {
        "id": "uuid",
        "display_name": "开发管理员"
      },
      "participants": {
        "active_digital_employee_count": 3
      },
      "task_counts": {
        "total": 12,
        "pending": 0,
        "queued": 0,
        "running": 3,
        "waiting_human": 2,
        "blocked": 0,
        "failed": 1,
        "completed": 6,
        "cancelled": 0,
        "other": 0
      },
      "attention": {
        "open_decision_count": 18,
        "waiting_human_unlinked_count": 2,
        "evidence_pending_count": 12,
        "unassigned_count": 0,
        "coordination_anomaly": false
      },
      "last_activity_at": "2026-08-11T00:51:00Z"
    }
  ],
  "pagination": {
    "limit": 12,
    "offset": 0,
    "total": 50,
    "has_more": true
  }
}
```

契约规则：

- 所有计数字段为 required，服务端恒返回 0，不返回 `null`；
- `task_counts.total` 必须通过服务端断言等于各桶之和（断言失败的处置见 §9，**不是返回 5xx**）；
- `last_activity_at` 优先取任务最新更新时间，无任务时回退项目 `updated_at`；
- 首页只返回参与数字员工数量。成员头像预览属于 P1，不作为首版完成条件；
- 不返回最近里程碑或最近工件，避免不存在的领域语义和 N+1。

字段口径钉死（不写死就是下一次口径分叉的种子，本方案要修的正是同一类 bug）：

| 字段 | 口径 | 同源出处 |
|---|---|---|
| `participants.active_digital_employee_count` | **活跃任务**上 `assigned_digital_employee_id` 的 `COUNT(DISTINCT)`；活跃 = 非终态。字段名带 `active_` 前缀，防止读成「项目共有几个员工」 | `ListProjectRunSummaries.participant_employee_count` |
| `attention.unassigned_count` | 活跃且未分派任务数，与上一行同一个活跃集 | 同上 |
| `attention.open_decision_count` | `project_decision_requests` 中 `lower(status_snapshot) ∈ {pending, waiting, requested, open}` | 同上 |
| `attention.waiting_human_unlinked_count` | §3.3 的 orphan 口径；**健康度唯一可读的等人字段** | 同上 |
| `attention.evidence_pending_count` | `project_evidence_refs.verification_status ∈ {submitted, rejected}` | 同上 |
| `attention.coordination_anomaly` | `projects.coordination_status ∉ healthy 集`；归档项目恒 `false` | `project-risk.ts` `healthyCoordinationStatuses` |

**`completed_today_count` 的处置**：`run-summary` 的 `today_completed_run_count` 正在被 `index.tsx:1204` 使用（首页一个 KPI）。它是 `task_runs` 执行口径 + Asia/Shanghai 日窗，与 portfolio 的 `project_tasks` 业务口径**不同源**，硬塞进本契约会制造第三套口径。处置：

- portfolio **不返回**该字段；
- 首页该 KPI 有两条出路，**实施前必须二选一并记录**：(a) 判定为运行视角指标，从项目首页移除、只留运行总览；(b) 保留 KPI，则首页保留一个轻量 `run-summary` 请求专供此数，并在 §6.1 明确首屏请求数为 2 而非 1。
- 推荐 (a)：项目首页的双层表达里没有「今日」这一层，这个 KPI 本就是从运行总览借来的。

---

## 5. Control Plane 与数据库实现

### 5.1 查询形态

在 `apps/control-plane/internal/storage/queries/project.sql` 增加 `GetProjectPortfolio` 读查询。必须先选候选项目，再聚合其关联数据：

```sql
WITH all_projects AS (
  SELECT ...
  FROM projects
  WHERE tenant_id = $tenant_id
    AND deleted_at IS NULL
),
filtered_projects AS (
  SELECT ...
  FROM all_projects
  WHERE ...项目/负责人/关键词/任务状态筛选...
),
candidate_projects AS (
  SELECT ...
  FROM filtered_projects
  ORDER BY created_at DESC, id DESC
  LIMIT $limit OFFSET $offset
),
task_buckets AS (
  SELECT pt.project_id,
         COUNT(*) AS total,
         COUNT(*) FILTER (WHERE bucket = 'running') AS running,
         ...
  FROM normalized_project_tasks pt
  JOIN candidate_projects cp ON cp.id = pt.project_id
  GROUP BY pt.project_id
),
attention_counts AS (...仅 candidate_projects...),
participant_counts AS (...仅 candidate_projects...)
SELECT ...;
```

顶部全局 summary 与分页 total 使用独立聚合 CTE，但同一 SQL 快照返回，避免两个请求的口径漂移。任务状态筛选使用 `EXISTS`，不把任务明细复制到结果集。

**关于「不得全租户聚合」的准确边界**（草稿此处自相矛盾，已修正）：`summary.active_project_task_counts` 按定义就是全租户非归档项目的任务聚合，**它必须扫全量**，这不是缺陷。现有 `ListProjectRunSummaries` 也正是先全租户 `GROUP BY project_id` 再 `LEFT JOIN`。禁令只对**逐卡桶**成立。

严禁：

- **逐卡 `task_buckets` 聚合租户全部任务后再 `LIMIT`**（必须先定 `candidate_projects` 再 join 聚合；`summary` CTE 不受此限）；
- 在 Go 中加载 500 个任务对象后循环统计；
- handler 内逐项目调用 repository；
- 为成员、工件、overview 发 N+1 查询。

允许且预期：`summary` CTE 对全租户未 dismissed 任务做一次 `COUNT(*) FILTER`，与逐卡 CTE 共用同一段桶 CASE。

### 5.2 状态桶单一事实源

现状分叉是真的，且注释在说谎：`GetProjectTaskStatusCounts`（`project.sql:580`）头部写着「分桶口径与 ListProjectRunSummaries 保持一致」，但实际 `failed_tasks` 只数 `status = 'failed'`，而 `ListProjectRunSummaries.failed_count` 数 `failed/error/blocked`。**修复时必须一并纠正这句注释**，否则下一个人还会被它骗。

数据库查询和单项目 `GetProjectTaskStatusCounts` 必须共用同一段 SQL CASE 或同一数据库视图/SQLC 片段。同步收敛的展示口径：

- 等人使用 §3.2 的互斥优先级；
- `blocked` 与 `failed/error` 分桶（连带影响见 §3.5）；
- 任何未知值进入 `other`；
- `running/in_progress` 等防御值按 §3.2 收录，但**不得**称之为「修复窄口径」——`project_tasks` 上没有 `in_progress` 写入方。

#### 5.2.1 硬约束：`ActiveTasks` 与展示桶解耦（承重，违反即回归）

`GetProjectTaskStatusCounts` **不是纯展示投影，它是闸门输入**：`archive_readiness.go:59` 读它的 `ActiveTasks`，`> 0` 即产出 `active_tasks` 归档阻断项。§2.2 已把「不改状态推进规则」列为非目标，因此：

1. `ActiveTasks` 的定义**冻结**为现状：`status NOT IN ('completed','done','success','failed','cancelled')`。注意 `blocked` 与 `error` 在此定义下**算活跃**，这是现行归档闸门的既有行为。
2. **严禁**把 `ActiveTasks` 改写成「非终态展示桶之和」。§3.2 的桶里 `failed` 含 `error`，若实施者顺手认为「failed 桶是终态」，`error` 任务就从活跃翻成终态，带 error 任务的项目会**突然变得可归档**——一个纯读模型改动静默放开了归档闸门。
3. 新增展示桶字段与 `ActiveTasks` **并列返回、互不派生**。宁可两段 `COUNT(*) FILTER` 重复写，也不要让闸门语义依赖展示语义。
4. §8.1 必须有一条对照测试：构造含 `blocked` 与 `error` 任务的项目，断言改动前后 `evaluateArchiveReadiness` 的 blocker 列表逐项相等。

### 5.3 索引与迁移

**默认不落新索引。** 先核对既有索引，再用 `EXPLAIN (ANALYZE, BUFFERS)` 证明确有收益，才允许写迁移。草稿原本提的两个索引经核对都不成立：

| 草稿提议 | 既有索引 | 结论 |
|---|---|---|
| `idx_project_tasks_portfolio_counts (tenant_id, project_id, status) WHERE dismissed_at IS NULL` | `idx_project_tasks_tenant_project_status (tenant_id, project_id, status)`（013:118）；另有 `idx_project_tasks_active_not_dismissed (tenant_id, project_id, updated_at DESC) WHERE dismissed_at IS NULL`（迁移 `20260720165706`） | 仅比 013 多一个 partial 谓词。500 行规模规划器大概率直接 seq scan，收益需实测。**默认不落** |
| `idx_projects_portfolio_active (tenant_id, status, created_at DESC, id DESC) WHERE deleted_at IS NULL` | `idx_projects_tenant_deleted_created (tenant_id, created_at DESC) WHERE deleted_at IS NULL`（迁移 057） | 既有索引**已经命中**默认排序前缀。草稿版把 `status` 插在 `tenant_id` 与 `created_at` 之间，遇到 `project_status` 多值 `IN` 反而拿不到有序扫描，**比既有索引更差**。不落 |

复用现有索引（实施时需先核对确实存在，勿照抄本表）：

- `project_members(tenant_id, project_id)`；
- `project_decision_requests(tenant_id, project_id, status_snapshot, created_at)`；
- `project_evidence_refs(tenant_id, project_id, verification_status)`。

若 `EXPLAIN` 证明确需新索引，迁移放入 `apps/control-plane/internal/storage/migrations/`，更新 `atlas.sum` 并运行 `migrate-validate`；无论落与不落，都必须在性能报告中附 `EXPLAIN` 输出作为依据。**本项不进 P0 交付物**（见 §10）。

### 5.4 服务端分层

| 层 | 改动 |
|---|---|
| Contract | OpenAPI 增加 endpoint、请求参数、`ProjectPortfolio*` schema |
| Repository | 增加 portfolio 查询与 summary/count 映射 |
| Service | 校验 limit、权限、任务桶总和不变量 |
| Handler | 解析筛选、输出响应、记录 Server-Timing |
| Authorization | 租户级 authz `Check` 照旧 + `mine_only` 筛选谓词（§5.4.1，已拍板 C）；**不得写「沿用项目列表可见性」了事** |

#### 5.4.1 可见性口径 — 已拍板 C（2026-08-11）

**结论：租户级为默认，另给 `mine_only` 开关走「与我相关」谓词。** 论证与被否选项见下。

草稿原文「沿用项目列表可见性决策点，不得仅按 tenant_id 绕过对象可见范围」是一句自相矛盾的话——因为**项目列表的可见性决策点就是租户级**：

- `service.go:827` `ListProjects` 无 actor 过滤，直接下推 repository；
- `project.sql:64` `ListProjects` 的 `WHERE` 只有 `tenant_id` + `deleted_at IS NULL`；
- `handler.go:121` 的 authz `Check` 落在 `Resource: ResourceTenant`，不是逐项目对象。

「沿用」它，得到的恰好就是 §12 想规避的那件事：租户内全量可见。

更要紧的是**同一页面上已经存在两套可见范围**：`ListWorkflowInstances`（`project.sql:96-115`）按 `actor ∈ human_owner_user_ids ∪ active project_members` 过滤。也就是说今天用户在 `/projects` 看到的项目列表是租户全量，而队列/工作流条目是「与我相关」——这个不一致是既存的，本方案会把它固化进新契约。

这是产品决策，不是实现细节。三个选项：

| 选项 | 语义 | 代价 |
|---|---|---|
| A 维持租户级 | 与现状 `ListProjects` 一致，portfolio 返回租户全部项目 | 零改动；但把「首页 = 全租户」正式写进新契约，且与同页队列口径继续不一致 |
| B 收敛为「与我相关」 | 对齐 `ListWorkflowInstances` 的 owner ∪ active member | 需同步改 `ListProjects`，否则新旧两个列表接口口径分叉；**会改变现有用户看到的项目集合**，属行为破坏性变更 |
| **C（已选）** 租户级 + 「仅我参与」开关 | 默认 A，给一个 toggle 走 B 的谓词 | 契约多一个参数；两种口径都要测；但不破坏现状且解释得清 |

选 C 的理由：既不破坏现有可见性，又让「与我相关」成为显式的用户选择而非隐式规则。

**C 的实施约束**：

1. §4.2 增加参数 `mine_only`（bool，默认 `false`）。
2. `mine_only=true` 的谓词**必须与 `ListWorkflowInstances`（`project.sql:96-115`）同源**：`actor ∈ projects.human_owner_user_ids` ∪ `project_members` 中 `principal_type='human_user' AND principal_id=actor AND status='active'`。抄一份新谓词就是制造第三套可见性口径——本方案要修的正是这类分叉。
3. **`summary` 与 `items` 必须用同一可见集合**：开关打开时顶部组合统计一并收窄，否则「顶部 50 个项目、列表 3 个项目」无法解释。§3.4 的范围标签需随开关改写（「全部项目」↔「我参与的项目」）。
4. `mine_only` 是筛选，不是授权。租户级 authz `Check`（`handler.go:121`）照旧，不因该参数放宽或收紧。
5. §8.1 权限隔离用例覆盖两种取值；`mine_only=true` 需有「非成员看不到、active member 看得到、非 active member 看不到」三例。
6. 前端把它做成 toolbar 上的显式开关并持久化到 URL（站内跳转走 TanStack Router），刷新后不丢；默认关闭以保持现状体感。

#### 5.4.2 建议类型

```go
type ProjectTaskPortfolioCounts struct { ... }
type ProjectPortfolioItem struct { ... }
type ProjectPortfolioSummary struct { ... }
type ProjectPortfolioResponse struct { ... }
```

### 5.5 超时与保护

- `limit` 服务端上限 50；超过返回 400；
- portfolio SQL statement timeout 建议 800ms；
- 单次响应建议不超过 100KB；
- 不以 Redis/内存缓存作为达标前提；
- 可设置 5～10 秒私有短缓存或 ETag 作为优化，但写后必须可失效，缓存不计入正确性。

---

## 6. Web 实现

### 6.1 数据层

在 `apps/web/src/lib/api/projects.ts` 增加：

```ts
export type ProjectPortfolioFilters = { ... };
export type ProjectTaskPortfolioCounts = { ... };
export type ProjectPortfolioItem = { ... };
export type ProjectPortfolioResponse = { ... };
export function getProjectPortfolio(...): Promise<ProjectPortfolioResponse>;
```

React Query：

```ts
queryKey: ["projects", "portfolio", filters]
placeholderData: keepPreviousData
staleTime: 10_000
```

列表态用 portfolio 取代 `listProjects + run-summary`。**`listUsers` 与 `listDigitalEmployees` 保留**（§1.3）：它们是名称目录、`staleTime: 60s`、跨页共享缓存，与项目数无关，不是 fan-out；砍掉会让负责人/处理人回退裸 UUID，违反 `status-labels` 中文优先约定。

因此首屏请求数为 **3**（portfolio + 两个长缓存目录），若 §4.3 的 `completed_today_count` 选了方案 (b) 则为 4。这仍不劣于现状基线（§2.1 目标 5）。

`useProjectRiskSignals` 保持现状：只对选中的队列/卡片项目发 4 个明细请求，不在首屏触发。

项目详情态继续使用既有详情接口。新增接口部署后，Control Plane 先上线，Web 后上线；不需要破坏旧端点。

### 6.2 页面组件

**本节按 §1.3 的 IA 选项 A 书写**（卡片网格取代队列为主区，triage 降为选中后的右栏）。拍板为 B/C 时本节需重写。

现有单元全部在 `components/project-risk-home.tsx`（1018 行）。**改造既有组件，不新建重名组件**：

| 组件 | 来源 | 职责 |
|---|---|---|
| `ProjectPortfolioSummaryBar` | **改造既有**（`project-risk-home.tsx:104`） | 项目层分布 + 非归档项目任务层堆叠条 |
| `ProjectPortfolioToolbar` | 新建；吸收现有 chip 收敛 + 「更多筛选」 | 搜索、项目状态、负责人、任务状态、排序、视图切换 |
| `ProjectPortfolioGrid` | 新建 | 卡片分页与 loading/empty/error/permission 四态 |
| `ProjectPortfolioCard` | 新建 | 项目生命周期、负责人、健康度、任务构成、最近活动、进入项目 |
| `TaskCompositionBar` | 新建 | 互斥桶可视化、tooltip、图例、无数据态 |
| `ProjectTriagePanel` | **保留既有**（`:482`） | 选中项目的可行动明细，不进首屏 |
| `ProjectPortfolioPerspectivePanel` | **保留既有**（`:793`），换数据源 | 未选中时的右栏组合透视 |
| `ProjectRiskQueue` | **既有（`:152`），选项 A 下退役** | 退役前需确认其 chip 筛选、负责人行、空态文案已在新 toolbar/卡片上有对应落点 |

优先复用 `SoftCard`、`StatusPill`、`IconTile`、`ListToolbar`、`ToolbarSearch`、`Pagination`、`StateSurface`；任务构成条使用 CSS flex，不引入图表库，不为每张卡创建独立 SVG。布局遵循 `DESIGN.md` 与 `MasterDetailLayout` 既有约定。

### 6.3 显示规则

项目卡固定信息顺序：

1. 项目名称 + `项目状态 · {生命周期}`；
2. 项目目标；
3. 人类负责人 + `N 名数字员工`；
4. `任务构成 · total` + 堆叠条 + 非零桶数量；
5. 健康度 pill；
6. 最近活动；
7. `进入项目`。

规则：

- 项目生命周期 pill 和任务构成条必须有独立标签；
- 零任务项目显示“尚无项目任务”，不绘制空比例条；
- `other > 0` 显示“其它 N”，并上报前端遥测；
- 归档项目健康度为中性，不刷新历史关注信号；
- 点击任务桶只改变项目筛选，不在首页展开任务；
- 卡片/列表切换只改变表现，不改变数据口径；
- 所有桶与状态中文经 `apps/web/src/lib/status-labels.ts`，与已迁入的 `reasonLabels` 同一出口，不在组件内散写字面量。

健康度（关注投影）的接线，必须逐条对上，不能只写「继续使用现有投影」：

| 健康度输入桶（`project-risk.ts` `countBuckets`） | 数据来源 | 注意 |
|---|---|---|
| `decisions` | `attention.open_decision_count` | 不变 |
| `waitingHuman` | `attention.waiting_human_unlinked_count` | **只许读这个**，读 `task_counts.waiting_human` 即回归（§3.3） |
| `executionFailed` | `task_counts.failed + task_counts.blocked` | 契约拆桶后必须在此重新合并（§3.5） |
| `evidence` | `attention.evidence_pending_count` | 不变 |
| `coordination` | `attention.coordination_anomaly ? 1 : 0` | 不变 |
| `sla` | 恒 0 | 现状即 0；不得借本次改动复活「等待超时」 |

第 3 项「人类负责人 + N 名数字员工」中的 N 是**活跃任务上的**数字员工数（§4.3），文案需能自证范围，如「3 名员工在办」，不得写成会被读作项目编制的「3 名数字员工」。

### 6.4 响应式与性能

- 接口可取最多 50 条摘要，但桌面首屏默认渲染 12 张卡；分页后再渲染下一页；
- 50 条摘要不做全量 DOM 常驻；
- 任务条首次加载可执行一次 180ms 宽度过渡，`prefers-reduced-motion` 下立即呈现；
- 字体、图标与通用目录沿用全站缓存，不阻塞项目卡主信息；
- 后台刷新保留现有卡片，不闪回 skeleton；
- `data-ready=true` 仅在项目摘要与当前页卡片均完成渲染后设置。它是**测试与前后对比的渲染完成钩子**（§7.2 的基线采样、web 四态断言都用它），不是线上性能上报的终点——§7 已不设页面 SLO 与上报管道。

---

## 7. 性能：形态门禁，不设 SLO 工程

草稿此处原有一套完整 SLO 脚手架（2 秒预算表 + 20 并发 × 5 分钟压测 + 30 次导航采样）。按 §1.2 已删除，理由如下，避免后来者以为是漏写：

- 验收规模是 **50 项目 / 500 任务**。这个量级下 Postgres 的一次 `COUNT(*) FILTER` 聚合是毫秒级，跑压测只能测出网络和 Go 序列化，测不出本方案的任何设计选择；
- 原预算表 2000ms 里有 1350ms 分给「网络传输」和「认证调度安全余量」——空转额度比真实工作量大三倍，这样的预算无论实现多差都会通过，不是判别性门禁；
- 首屏请求数在 `a5c57247` 已从 ~46 降到 3~4，性能问题已经解决过一次。本方案不是性能项目。

**保留的是形态门禁**：性能不达标通常是形态错了（N+1、全表回表、逐项目子查询），门禁应直接判形态。

### 7.1 查询形态门禁（必过）

对 50/500 固定 fixture 运行 `EXPLAIN (ANALYZE, BUFFERS, VERBOSE)`，断言：

- 不扫描其他租户数据；
- 逐卡 `task_buckets` 的实际处理行数 ≤ 候选项目关联任务规模（`summary` CTE 不受此限，见 §5.1）；
- 计划中不出现按项目循环执行 50 次的子查询/`SubPlan`；
- 计划退化时测试失败并保存 explain 输出到性能报告。

### 7.2 回归门禁（必过）

| 指标 | 门槛 | 测法 |
|---|---:|---|
| 首屏请求数 | ≤ 4（§6.1），且**不随项目数增长** | 浏览器 network 断言 + web 测试断言不请求 `/overview` `/members` `/artifacts` |
| Portfolio API 单请求 p95 | ≤ 500ms | 顺序采样 20 次即可，不做并发压测 |
| 页面首屏 | **不劣于改动前基线** | 改动前后各采样 10 次同环境导航，记录中位数与最大值；变慢需给出解释 |

只记录数字，不承诺绝对阈值——2 秒之类的绝对值在这个规模下没有信息量。若未来单租户项目数突破 50（届时 offset 分页也要升级为游标，见 §4.2），再重开性能立项。

验证期间必须记录并复核 Web/Control Plane pid 与 `owner=`，防止服务被其他 worktree 接管导致结论作废。

---

## 8. 测试方案

### 8.1 Control Plane

- 桶映射表驱动测试：每种原始状态与审批标志只进入一个桶；
- 总数不变量：桶之和等于 total；
- dismissed 任务不计入；
- 归档项目不计入顶部 active task summary；
- 关键词、项目状态、负责人、任务状态筛选；
- `sort` 三种取值的排序稳定性，含 `last_activity_at IS NULL` 的回退与 `id` 收尾（§4.2）；
- limit 上下界、offset、total、has_more；
- 未登记状态进入 other 并记录告警；
- 权限隔离：其他租户项目不进入 items/summary；
- `mine_only` 两种取值（§5.4.1）：`false` 得租户全量；`true` 时「非成员看不到 / active member 看得到 / 非 active member 看不到」三例，且 `summary` 与 `items` 同集合收窄；
- repository SQL 测试与 handler 契约测试；
- 50/500 形态 fixture（供 §7.1 的 EXPLAIN 门禁使用，不用于压测）。

承重回归用例（缺一即视为未实施）：

- **归档闸门零变更**：构造含 `blocked` 与 `error` 任务的项目，断言 `evaluateArchiveReadiness` 的 blocker 列表在改动前后逐项相等（§5.2.1）；
- **等人双计**：构造 `waiting_human` 宽口径 = 19、orphan = 2、`open_decision_count` = 18 的项目，断言 `task_counts.waiting_human == 19` 且 `attention.waiting_human_unlinked_count == 2`；两字段串位即应测试失败（§3.3）；
- **`ListProjectRunSummaries` 不受影响**：断言其 `failed_count` 仍含 `blocked`、两个等人字段仍并存（§3.5）。

### 8.2 Web

- 项目生命周期与任务状态同时渲染，不互相覆盖；
- 任务条长度和图例来自 exact counts；
- zero task、other、归档、无负责人名称、接口错误、权限拒绝；
- 筛选更新 queryKey，保留上一页数据直到新响应到达；
- 点击任务桶只筛项目，不请求任务明细；
- 首屏断言不请求 `/overview`、`/members`、`/artifacts`、Inbox；`listUsers` / `listDigitalEmployees` 允许出现（名称目录，§6.1）；
- 卡片/列表视图共享同一响应；
- 10/12/20 分页与文本溢出；
- `counts_degraded=true` 时渲染中性提示且不整页报错（§9）；
- 健康度接线双向守卫：mock 里把 `waiting_human` 设 19、`waiting_human_unlinked_count` 设 2，断言健康度按 2 计；串字段即应变红（与 `a5c57247` 的守卫同法）；
- 只含 blocked 任务的项目：构成条显示「阻断 1」且健康度非「正常」（§3.5）；
- 排序默认 `attention`，切到 `created` 后关注项位置变化符合预期；
- 键盘焦点、状态非仅颜色、暗色主题和 reduced motion。

### 8.3 契约与真实链路

- `corepack pnpm generate:control-plane`
- `corepack pnpm verify:contracts`
- `corepack pnpm verify:control-plane`
- `corepack pnpm --filter @superteam/web test`
- 数据库迁移：仅当 §5.3 最终落了索引才需要 `migrate-validate` + 目标开发库 migrate status/apply；不落则本条 N/A
- 真实浏览器：Web → Portfolio API → PostgreSQL，抽 3 个项目与项目详情任务数交叉核对
- **归档闸门实链核对**：对一个含 blocked/error 任务的项目走归档预览，确认阻断项与改动前一致（§5.2.1）
- `/run-overview` 回归：旧 `run-summary` 行为不变，大屏「待人工」badge 与运行带项目数不变

---

## 9. 可观测性

Control Plane：

- `project_portfolio_request_duration_ms`
- `project_portfolio_db_duration_ms`
- `project_portfolio_items_count`
- `project_portfolio_payload_bytes`
- `project_portfolio_unknown_task_status_count`
- `Server-Timing: db;dur=..., app;dur=...`

Web：

- 上报筛选条件、排序、项目数量、`other` 桶命中；
- 不上报项目名称、目标或任务标题；
- 不引入独立的页面耗时上报管道（§7 已不设页面 SLO）。

告警建议：

- API p95 > 500ms 持续 10 分钟；
- `other` 桶连续出现（说明有未登记状态写入方，需补映射）。

**桶和不变量失败的处置：降级，不是拒绝出货。** 草稿原文「立即错误并拒绝输出错误统计」是个自造停机面：

- 有 `other` 兜底桶时，桶和**在数据上不可能破**——任何未知状态都进 `other`。桶和破只可能源于 SQL 或映射的代码 bug；
- 为一个代码 bug 让整个项目首页返回 5xx，等于用一个显示缺陷换一次页面级停机，代价不对等。

规则：

1. 服务端断言桶和 == total；
2. 失败时**照常返回**，把 `total` 修正为各桶实际之和（保证 UI 的比例条自洽），并置一个响应级 `counts_degraded: true`；
3. 同时记 error 级结构化日志 + `project_portfolio_bucket_invariant_failures` 计数器，命中即告警；
4. 前端读到 `counts_degraded` 时在卡片上显示中性提示（如「计数可能不准」），不隐藏数据、不整页报错；
5. 单测覆盖降级路径本身。

---

## 10. 实施批次

| 批次 | 内容 | 主要文件 |
|---|---|---|
| ~~P-1~~ | ~~人类拍板两项~~ **已完成（2026-08-11）：可见性 = C（租户级 + `mine_only`）、IA = A（卡片网格取代队列）** | 已回填 §5.4.1、§1.3 |
| P0 | 状态桶定义与共享计数查询；`GetProjectTaskStatusCounts` 分桶收敛 + 纠正其失效注释；**`ActiveTasks` 冻结与归档闸门对照测试**；50/500 形态 fixture | `storage/queries/project.sql`、`internal/project/types.go`、`pg_repository.go`、`archive_readiness.go`（**只加测试，不改逻辑**）、`project/*_test.go` |
| P1 | OpenAPI + Portfolio repository/service/handler + 可见性实现 + 观测指标 + 桶和降级路径 | `contracts/control-plane/openapi.yaml`、`internal/project/*` |
| P2 | Web API 类型、React Query 换源、顶部双层摘要改造 | `lib/api/projects.ts`、`features/projects/index.tsx`、`components/project-risk-home.tsx`（`ProjectPortfolioSummaryBar`） |
| P3 | 项目卡、任务构成条、筛选/排序与分页、四态与可访问性；**健康度接线改造**；退役 `ProjectRiskQueue`（IA 选项 A）与截断提示 | `features/projects/components/*`、**`features/projects/project-risk.ts`**、`lib/status-labels.ts` |
| P4 | 单测、契约、迁移门禁、EXPLAIN 形态门禁、真实浏览器与运行总览回归 | tests、性能报告、`CHANGELOG.md` |

索引迁移**不在 P0**：按 §5.3 默认不落，仅当 P4 的 `EXPLAIN` 证明有收益才追加一个独立迁移批次。

顺序不可交换：先拍板，再统一状态桶，再建立契约，最后接前端；否则会把当前不一致口径固化进新 UI。

P3 特别提示：`project-risk.ts`（1113 行）是 web 侧的口径事实源，草稿的文件清单里漏了它。§3.5 的 `executionFailed` 合并与 §6.3 的健康度接线都落在这个文件，漏改的表现是「构成条显示 1 阻断，健康度显示正常」——同屏自相矛盾且不会有测试自然报错。

---

## 11. 发布、兼容与回滚

发布顺序：

1. Control Plane 上线新增 endpoint，旧接口保持不变；
2. 真实环境只读 smoke Portfolio API，与直连 DB 逐项目交叉核对；
3. Web 切换到新 endpoint；
4. 观察 `other` 桶与 `counts_degraded`；
5. 通过后删除首页不再使用的列表态拼装代码，**按 §1.3 的去留表逐项核对**，不删除旧 endpoint。

第 5 步的删除范围以 §1.3 表格为准。未在表中标注去留的现有能力一律保留——列表态一天前刚大改过，凭印象删会掉功能。

回滚：

- Web 可独立回滚到现有项目列表 + run-summary 页面；
- 新 endpoint 为附加契约，回滚 Web 后无害；
- P0 的 `GetProjectTaskStatusCounts` 分桶收敛与 Web 无关，可独立留存；因其 `ActiveTasks` 语义冻结（§5.2.1），回滚 Web 不影响归档闸门；
- 若最终落了索引且造成写放大，可单独迁移删除，不影响数据正确性；
- 不回滚状态桶正确性修复；若发现历史未知状态，进入 `other` 并修复映射。

---

## 12. 风险与处理

| 风险 | 处理 |
|---|---|
| **「等人 + 待决」双计回归**（`a5c57247` 已修过一次的 P0） | §3.3 三条规则：宽口径只进任务层，健康度只读 `waiting_human_unlinked_count`，UI 两层分区；§8.1 有专项断言 |
| **归档闸门被读模型改动静默放开** | §5.2.1 冻结 `ActiveTasks` 定义、展示桶与闸门互不派生；§8.1 有改动前后 blocker 逐项对照 |
| **健康度与构成条自相矛盾**（blocked 拆桶后漏改 web） | §3.5 契约拆桶 / 健康度重新合并；P3 明确包含 `project-risk.ts` |
| 任务桶和项目详情口径再次分叉 | 共享 CASE/查询片段 + 同 fixture 交叉断言；并纠正 `project.sql:580` 那句已失效的「与 ListProjectRunSummaries 一致」注释 |
| Portfolio endpoint 可见性口径不明 | 已拍板 C（§5.4.1）：租户级默认 + `mine_only` 开关；谓词与 `ListWorkflowInstances` 同源，summary 与 items 恒同集合 |
| `mine_only` 谓词另写一份 = 第三套可见性口径 | §5.4.1 约束 2 要求与 `project.sql:96-115` 同源；§8.1 三例覆盖 |
| 顶部全局 summary 与筛选列表被误解 | 明确标签“全部项目/非归档项目任务”，列表显示“筛选结果 N”；归档卡任务不入顶部统计（§3.4） |
| 关注项被创建时间排序埋到后页 | `sort` 默认 `attention`，与现状队列排序同源（§4.2） |
| 重写掉一天前刚验收的列表态能力 | §1.3 去留表 + §11 第 5 步逐项核对；未标注者默认保留 |
| 成员头像诱发 N+1 | P0 只显示员工数量；如需头像，后续在同一读模型批量返回最多 3 个预览 |
| 最近工件诱发 N+1 | 首版不显示；后续用 LATERAL/batch projection，不调用 50 次 artifacts |
| 状态动画拖慢 50 卡渲染 | 每页最多 12 卡、CSS 条、reduced-motion、无图表库 |
| 新索引写放大 | 默认不落；仅当 EXPLAIN 证明有收益才追加独立迁移，可独立回滚（§5.3） |

---

## 13. 完成定义

只有同时满足以下条件，才能声明本方案开发完成：

前置（开工条件，已满足）：

- [x] §5.4.1 可见性口径已拍板并回填：**C — 租户级默认 + `mine_only` 开关**（2026-08-11）；
- [x] §1.3 IA 选项已拍板并回填：**A — 卡片网格取代队列**（2026-08-11）。

承重不变量：

- [ ] 任务桶互斥且桶和 == total；降级路径（`counts_degraded`）本身有单测；
- [ ] **等人不双计**：`task_counts.waiting_human` 宽口径、`attention.waiting_human_unlinked_count` orphan 并存，健康度只读后者，串位即测试失败；
- [ ] **归档闸门零变更**：改动前后 `evaluateArchiveReadiness` blocker 列表逐项相等（含 blocked / error 用例）；
- [ ] **健康度与构成条一致**：只有 blocked 任务的项目，构成条显示阻断且健康度非「正常」；
- [ ] `ListProjectRunSummaries` 行为不变（`failed_count` 仍含 blocked、两个等人字段仍并存）。

功能与形态：

- [ ] OpenAPI、生成代码、SQLC 一致（若最终落了索引，迁移与 `atlas.sum` 一致）；
- [ ] 首页首屏无逐项目 fan-out，请求数 ≤ 4 且不随项目数增长；
- [ ] `sort=attention` 为默认，关注项不因分页被埋到后页；
- [ ] `mine_only` 两种取值行为正确，谓词与 `ListWorkflowInstances` 同源，`summary`/`items` 同集合；
- [ ] 真分页生效，截断提示已删除；
- [ ] §1.3 去留表逐项核对完毕，未标注能力无回归；`ProjectRiskQueue` 退役前其能力已全部有落点；
- [ ] EXPLAIN 形态门禁通过（§7.1），explain 输出已存档；
- [ ] Portfolio API 单请求 p95 ≤ 500ms，页面首屏不劣于改动前基线（§7.2）。

链路与门禁：

- [ ] 页面项目层与任务层数据和项目详情交叉核对一致（抽 3 个项目）；
- [ ] 真实 Web/Control Plane/DB 链路通过且验证期服务 pid/`owner=` 同源；
- [ ] `/run-overview` 回归通过；
- [ ] `verify:contracts`、Control Plane、Web 定向测试与迁移门禁通过；
- [ ] CHANGELOG 记录接口、首页、状态口径与可见性拍板结果。

