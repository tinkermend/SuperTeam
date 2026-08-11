# 项目管理首页：数据地基收敛与组合职责治理

- 日期：2026-08-10
- 状态：**已实施（v2）+ 复审修复后重取走查摘录（2026-08-11）**——推荐默认 C / S1 / 完整「项目待决」/ dismissed 修复一并落地；未选中右栏 = 组合透视面板  
  - ⚠️ 首轮走查摘录（记「15 项目待决 · 14 等人」）作废：取样早于当轮 orphan 去重改动，与最终代码对不上。以下为复审修复后重取。  
  - 验证期服务：`control-plane` pid=**52727**、`web` pid=**20422**，cwd 均为本 checkout `SuperTeam`（`dev-services.sh status` 确认 owner 同源）。  
  - **等人字段拆分**（复审 P0）：`waiting_human_count` 宽口径归还运行总览，项目首页改读 `waiting_human_unlinked_count`。API 实测 `provider-semantic-e2e-p1` = 宽 19 / orphan 2 / 待决 18 / 失败 1 / 证据待核 12。  
  - `/projects`：副标题、S1 真值条「已加载 6 · 已就绪 3 · 验收中 3 · 协调异常 0」、列名关注摘要·执行摘要、无「待我决策」KPI、无 dashboard-rail；队列行「18 项目待决 · 2 等人 · 1 失败 / 12 证据待核」；空态仅 pill「暂无阻塞」；分页「已加载范围内 N 条」；「我的待办」无数字且跳 `/inbox`（数字唯一出口＝侧栏角标 29）。  
  - 筛选 chip：默认「全部项目 6 / 存在风险 5 / 项目待决 5 / 执行失败 1」；展开更多为「等人任务 1 / 证据待核 4 / 协调异常 0」——**无「等待超时」**（run-summary 无等待起点，列表态恒 0，已下线为死 chip）。  
  - 选中 triage：先「正在加载明细…」占位（**不再渲染按桶合成的假行**），就绪后为真实决策标题 + 深链；「可行动 · 21」与队列行 18+2+1 一致；「其它信号 · 13」= 12 证据 + 1 等待超时（SLA 只在明细出现）。  
  - `/run-overview` 大屏（复审要求补做的连带复验）：「待人工 19」+ 另 4 个项目各 1，5 个项目全部可见（修复前会塌成 2 且 4 个项目 `hasActive` 翻 false 掉出运行带）。  
  - 门禁：web projects+run-overview 301 绿、typecheck 绿、`go test ./internal/project/...` 绿、`verify:contracts` 绿。
- 前置：
  - 运营驾驶舱：`docs/superpowers/specs/2026-07-14-projects-dashboard-design.md`（**本方案修订其右栏默认与 KPI 第 5 卡决策**）
  - 布局宪法：`docs/superpowers/specs/2026-07-14-layout-constitution-design.md`
  - 状态分层：`docs/superpowers/specs/2026-07-29-project-status-layers.md`（**本方案需同步其 §1 词表行**）
  - 运行总览：`docs/superpowers/specs/2026-07-26-run-overview-display-mode.md`（`run-summary` 端点来源）
- 范围：`/projects` **列表态**（无 `routeProjectId`）Web 表现层 + **`run-summary` 端点扩列**（契约变更，无 DB 迁移）
- 实现锚点：
  - `apps/web/src/features/projects/index.tsx`
  - `apps/web/src/features/projects/components/project-risk-home.tsx`
  - `apps/web/src/features/projects/components/project-dashboard-rail.tsx`
  - `apps/web/src/features/projects/project-risk.ts`
  - `apps/web/src/features/projects/hooks/use-project-risk-signals.ts`
  - `apps/control-plane/internal/storage/queries/project.sql`（`ListProjectRunSummaries`）
  - `contracts/control-plane/openapi.yaml`（`ProjectRunSummaryItem`）
  - 测试：`index.test.tsx`、`project-risk.test.ts`

---

## 0. 修订记录（v1 → v2）

| # | v1 主张 | v2 处置 | 依据 |
|---|---|---|---|
| 1 | 本期不动 N+1，记为「P2 后端新建 `ProjectHomeOverview`」 | **提为 B0 首批**；不新建端点，扩展**已落地**的 `run-summary` | `GET /api/v1/projects/run-summary` 已在契约与服务端实现（`openapi.yaml:1043`、`project.sql:2707`），v1 未发现 |
| 2 | 「零后端契约变更」 | 改为「契约仅扩两列，无 DB 迁移」 | 扩列比维持 46 请求便宜得多 |
| 3 | 保留说明句「分页仍对应完整项目列表」 | **删除该句**并补截断提示 | 该句在项目数 >50 时为假（见 §1.4） |
| 4 | 空态文案 `无待办` → `暂无阻塞` | 改为**空态不渲染正文行**，语义只由 pill 承担 | 直接改字符串会让同一格出现两次「暂无阻塞」 |
| 5 | 未选中右栏 A/B 二选一 | 改为 **A/B/C 三选一，C 为推荐默认** | A 的缓解手段正是 07-14 用户要求点名否决的做法（见 §0.1） |
| 6 | 「我的待办 · N」弱链 | 弱链**不带数字**，两个 inbox 请求全删 | 侧栏角标（`app-sidebar.tsx:39`）已是该数字的唯一出口 |
| 7 | 风险「测例大面积失败」 | 下调为低风险 | 实测受影响断言 `index.test.tsx` 10 处 + `project-risk.test.ts` 8 处 |
| 8 | 列名改动只列表头 | 补 `project-risk-home.tsx:583` 与 07-29 spec 词表行 | 漏改会造成新的词表分叉 |
| 9 | H4「可微调字号/位置」 | 给出可执行形态 | 原文非可执行判据 |

### 0.1 需人类先拍板的前置决策（阻塞 §4.1）

`2026-07-14-projects-dashboard-design.md` 的立项背景是一条**有记录的用户硬要求**：

> 用户明确要求"不管屏幕大小铺满整个屏幕"。设计原则：**铺满靠增加信息维度，不靠拉伸现有内容**（每个面板密度有界）。

v1 的推荐方案 A（未选中队列全宽）会把首页退回该 spec 描述的「1.5 个维度」原状，而 v1 §10 给出的缓解是「用全宽队列消化」——即该要求点名否决的「拉伸现有内容」。

**v2 不替人类做这个决定。** §4.1 保留 A，但默认推荐 C（右栏改装组合视角内容），因为 C 同时满足「首页主对象是 Project」与「铺满靠增加维度」。选 A 等于撤销上述用户要求，需在 §12 显式勾选确认。

---

## 1. 背景与问题

### 1.1 现网形态（骨架保留）

```
┌─ 组合 KPI 带（5 卡，含「待我决策」inbox 真值）───────────┐
├─ 项目队列（1fr）──────────────┬─ 右栏 rail=lg ──────────┤
│ 搜索 / 状态 / 风险 chip       │ 未选中：待我决策列表      │
│ 表：项目 | 待办拆分 |         │        + 最近运行动态    │
│     当前处理者 | 最近活动 |   │ 选中：ProjectTriagePanel │
│     进入项目                  │                          │
└───────────────────────────────┴──────────────────────────┘
```

### 1.2 现网数据流与真实代价（v2 实测）

| 层 | 来源 | 请求数 | 覆盖口径 |
|----|------|--------|----------|
| L0 | `listProjects({limit:50,offset:0})` | 1 | **前 50 个项目**，之后客户端切片分页 |
| L1 | 每项目 `tasks`+`decisions`+`evidence`+`members` | **pageSize × 4 = 40** | 仅当前页 |
| L2 | `getInboxBadge` + `listInboxItems(mine,open)` | 2 | 跨项目「我的」 |
| L3 | `listWorkflowInstances({limit:50})` | 1 | 仅用于「最近活动」时间戳 |
| — | `listDigitalEmployees` + `listUsers` | 2 | 补名 |
| | **合计** | **≈46 / 首屏，翻页重打** | |

`useProjectRiskSignals` 的 4 连发 fan-out 是主要成本；`listWorkflowInstances` 拉 50 条实例只为取每项目一个时间戳。

### 1.3 已确认的产品判断（v1 保留）

1. **项目管理首页的核心是项目组合**，不是人待办工作台。主问题是「有哪些项目、状态与阻塞信号、从哪进入」，**不是**「我下一条该批哪条」。
2. **现网队列 + 风险投影比泳道/大卡叙事更贴合**；本方案做职责收敛与地基替换，不更换信息架构主形态。
3. 多任务并存时**不存在**可诚实展示的「唯一当前任务状态」；队列列必须降承诺、改文案，禁止黑话（如对用户说「球权」）。

### 1.4 痛点（v2 修订，新增 H8/H9）

| ID | 痛点 | 用户可感症状 |
|----|------|----------------|
| **H8** | **请求风暴** | 首屏 ≈46 请求、翻页重打；风险列长时间「识别中」，`isCurrentPageRiskSettling` 期间全页降级为 pending |
| **H9** | **计数不诚实** | ①页级风险 vs 全量列表不可比；②`limit:50` 截断后第 51 个项目**无声消失**；③页面固定文案「分页仍对应完整项目列表」在 >50 时为**假**（`project-risk-home.tsx:236`） |
| H1 | Inbox 越权 | KPI「待我决策」+ 右栏决策列表 ≈ 第二收件箱；与侧栏角标重复 |
| H2 | 列名难懂 | 「待办拆分」术语感强；「当前处理者」易被读成「整个项目唯一状态」 |
| H3 | 词表撞车 | 「待决 / 待办 / 待我决策」混用；项目侧计数与 Inbox 条数不可比却无解释 |
| H4 | 项目状态不够一级 | 生命周期 pill 挤在项目列第 3 行，关注信号更抢视线 |
| H5 | KPI / chip 负担 | 五卡等高 + 8 个 chip + 双栏，首屏队列行数偏少 |
| H6 | 右栏未选中内容偏弱 | 「最近运行」与队列「最近活动」重叠；决策流抢主舞台 |
| H7 | triage 增量不足 | 选中后与队列关注摘要大量重复 |

H9 是 H3/H5「mistrust」的**根因**：只要计数还是页级派生 + 静默截断，任何免责说明句都只是把不诚实写得更长。B0 解决根因后，这些说明句可整段删除。

### 1.5 未被利用的既有资产

`GET /api/v1/projects/run-summary`（07-26 运行总览落地）**一次租户级查询**返回逐项目：

`running_count` / `queued_count` / `waiting_human_count` / `failed_count` / `unassigned_count` / `participant_employee_count` / `completed_today_count` / `last_activity_at`

覆盖度对照：

| 队列所需 | run-summary 现状 | 处置 |
|---|---|---|
| 关注摘要 · 等人 | `waiting_human_count` | 直接用 |
| 关注摘要 · 失败 | `failed_count` | 直接用 |
| 执行摘要 · 待分派 | `unassigned_count` | 直接用 |
| 执行摘要 · 执行中 | `running_count` | 直接用（员工名由已有目录补） |
| 最近活动 | `last_activity_at` | 直接用，**整段替换** `listWorkflowInstances` |
| 关注摘要 · 待决 | ❌ | **B0 扩列** `open_decision_count` |
| 关注摘要 · 证据待核 | ❌ | **B0 扩列** `evidence_pending_count` |
| 协调异常 | 项目列表已有 `coordination_status` | 无需扩列 |
| reason 明细（title/深链） | ❌ 也不该有 | 保留 4 连发，**只对选中项目** |

探索原型（`docs/prototypes/project-home-layout-v2/` A–E）不作为实现目标，仅留档（见 §13）。

---

## 2. 目标与非目标

### 2.1 目标

1. **数据地基收敛**：首屏请求 ≈46 → **3**；队列计数从「当前页派生」升级为**租户级真值**。
2. **职责边界清晰**：`/projects` 主对象 = Project；个人待办仅弱导流到 `/inbox`，本页不再持有 inbox 数据。
3. **表意诚实**：不假装单一「当前任务」；不声称分页覆盖完整列表；截断显式可见。
4. **契约变更最小**：仅 `ProjectRunSummaryItem` 扩两列 + 一处口径修复，**无 DB 迁移**。
5. **可回归**：单测断言更新；真实浏览器列表态走查；`run-summary` 口径变更需连带复验运行总览大屏。

### 2.2 非目标

- 不换成泳道 / 叙事大卡 / 底部决策坞作为主形态。
- 不在首页提交审批、批量决策、写证据。
- 不新建 `ProjectHomeOverview` 端点（v1 设想，v2 判定不必要）。
- 不改项目详情路由与运营详情信息架构。
- 不做 Runtime 健康 / 成本进首页。
- 不做 `listProjects` 真服务端分页（记为 §9 后续；本期只做截断诚实化）。

### 2.3 成功标准（验收句）

打开 `/projects` 列表态，用户应能在约 3 秒内回答：

> 有哪些项目、各自**项目生命周期状态**如何、有无**需要关注的信号**、点哪里**进入项目**。

且：页面上出现的每个数字，用户都能说清它统计的是什么范围；没有任何一个项目会在不告知的情况下从列表消失。

用户**不应**在本页被引导为「在这里把我的 20 条审批处理完」。

---

## 3. 页面职责边界（不变量）

| 维度 | 项目管理 `/projects` | 收件箱 `/inbox` |
|------|----------------------|-----------------|
| 主对象 | 项目行 | 待办条目 |
| 主问题 | 组合扫读、进入项目 | 我要处理哪几条 |
| 计数语义 | 项目 status、**租户级项目侧计数** | `mine_open` 等 Inbox 真值 |
| 主 CTA | 新建项目 / 进入项目 | 处理 / 深链上下文 |
| 禁止 | 常驻第二 Inbox 列表、行内批完、**持有 inbox 数据** | 冒充项目目录 |

**握手方式（允许）**：

- 工具区弱链「我的待办 →」，**不带数字**（数字唯一出口 = 侧栏角标 `app-sidebar.tsx:39`）。
- 项目行内关注摘要可含「N 项目待决」（**项目上**的 open 决策数，不是「我的」条数）。
- triage 内可提供「去收件箱」次要链接，不重复渲染 Inbox 流。

**推论**：本页删除 `listInboxItems` 与 `getInboxBadge` 两个查询（v1 只删前者）。

---

## 4. 目标信息架构

### 4.1 布局（右栏三选一）

```
宽容器
┌─ 组合摘要（紧凑真值条，无 Inbox 卡）───────────────────┐
├─ 工具区：搜索 | 状态 | 风险 chip | [我的待办 →] | 新建 ─┤
├─ 项目队列（主列）───────────┬─ 右栏 ─────────────────┤
│ 表（见 §4.3 列模型）        │ 选中：ProjectTriagePanel │
│                             │ 未选中：见下表 A/B/C     │
└─────────────────────────────┴──────────────────────────┘
```

| 选项 | 未选中右栏内容 | 评价 |
|------|----------------|------|
| **C（推荐默认）** | **项目组合透视面板**：状态分布条 / 协调异常项目名单 / 长期无活动项目 / 今日完成 | 主对象仍是 Project，守住 §3 边界；数据全部来自 B0 的**同一个** `run-summary` 响应，零额外请求；满足 07-14「铺满靠增加维度」 |
| A | 不渲染（`detail={undefined}`，队列全宽） | 实现最省（`MasterDetailLayout` 原生支持，见 `layout.tsx:38`），但**撤销 07-14 用户硬要求**，需 §12 显式确认 |
| B | 仅「最近运行」3～5 条 | 与队列「最近活动」列重复 = H6 本身，**不推荐** |

| 决策 | 选择 |
|------|------|
| 「我的待办」 | 工具区弱链，**不带数字**；不是 KPI 卡，不是右栏列表 |
| 最近运行 | 队列「最近活动」列保留（数据源换 `last_activity_at`）；`RunActivityPanel` 列表态下线 |
| 窄容器 | 选中走 Sheet；未选中 C 走 stack、A 无内容 |
| `narrowDetail` | 选 A 时 `"stack"` 分支变死码（`index.tsx:1108`），收敛为常量 `"sheet"` |

**对 07-14 文档的修订声明**：「默认右栏 = 待我决策 + 最近运行」**废止**；Inbox 真值改由侧栏角标单点承担。实施后 07-14 文首复核状态改为「部分被 2026-08-10 方案取代」。**若选 A，需额外注明「铺满靠增加维度」原则一并撤销。**

### 4.2 组合摘要（原 KPI 带）

采用 **S1 紧凑真值条**（单行弱样式 SoftCard），压缩首屏高度：

```
已加载 6 · 已就绪 3 · 验收中 3 · 协调异常 0
```

- **移除**「待我决策」MetricCard（H1）。
- 数字口径：`总数/running/acceptance` 来自已加载项目列表；`协调异常` 来自 `coordination_status`。B0 后可选改用 `run-summary` 的租户级项目数，使「已加载」变为「全部」——若采纳，标签同步改名（§12 勾选）。
- 实现组件：`ProjectPortfolioSummaryBar`（`project-risk-home.tsx:126`）。
- 备选 S2（保留四卡 MetricGrid）仅在产品要求视觉不变时启用。

### 4.3 队列列模型

| 列 | 现名 | 新名 | 内容规则 |
|----|------|------|----------|
| 1 | 项目 | **项目** | 名称（主）+ **生命周期 StatusPill 同行右侧**；负责人降为次行。见下方 H4 处方 |
| 2 | 待办拆分 | **关注摘要** | §5.1；空态只出 pill，不出正文行 |
| 3 | 当前处理者 | **执行摘要** | §5.2；禁止对用户使用「球权」 |
| 4 | 最近活动 | **最近活动** | 数据源换 `run_summary.last_activity_at`；归档行回退 `project.updated_at` |
| 5 | 操作 | **操作** | 「进入项目」不变 |

**H4 可执行处方**（替代 v1 的「可微调字号/位置」）：生命周期 StatusPill 从第 3 行上提到与项目名同一行（名称 `truncate` 后紧跟 pill），负责人行下移。**不新增列**——表格已是 `min-w-[46rem] table-fixed`，加列会挤压关注摘要。

`data-testid` 保持 `project-queue-pending` / `project-queue-attention-headline` / `project-queue-current-handler` 不变，**仅改可见文案与内部 helper**；若改 testid 必须同 PR 改全量断言。

### 4.4 筛选 chip 与说明句

`PROJECT_RISK_FILTERS`（`project-risk.ts:89`）逻辑保留，默认可见集收敛：

| 默认可见 | 收入「更多」 |
|----------|-------------|
| 全部项目 / 存在风险 / **项目待决**（原「待决决策」，改名）/ 执行失败 | 等人任务 / 证据待核 / 等待超时 / 协调异常 |

**说明句处置（H9）**：

- **删除**现网固定句「风险识别基于当前页，筛选仅过滤当前页项目，分页仍对应完整项目列表」（`project-risk-home.tsx:236`）——B0 后前半句不再成立，后半句本就为假。
- **替换为**：仅在命中列表上限时出现的截断提示 —— `已加载前 50 个项目，请用搜索或状态筛选缩小范围`。未命中上限时不显示任何免责句。
- 「项目待决」≠「我的待办」：实现与文案均不得混用 inbox 数据（B0 后本页已无 inbox 数据，从结构上不可能混）。

---

## 5. 文案与投影语义

### 5.1 关注摘要（原「待办拆分」）

**展示结构**（列内）：

1. 风险级别 pill（`projectRiskLevelLabel`）。
2. **主行**：可行动计数拼接（待决 / 等人 / 失败 / 协调异常）。
3. **次行**：软信号（证据待核、等待超时）。
4. **空态：只渲染 pill「暂无阻塞」，不渲染正文行。**

**空态修正（关键）**：v1 提出把 `formatAttentionHeadline` 的 `"无待办"`（`project-risk.ts:609`）改为 `"暂无阻塞"`。直接改会在**两处**造成同格重复——队列格已有 `<StatusPill>暂无阻塞</StatusPill>`（`project-risk-home.tsx:440`），triage 面板已有 `projectRiskLevelLabel` 返回的「暂无阻塞」pill（`:573`）。

正确改法：

- `formatAttentionHeadline` 空态返回 `{ primary: "", detail: undefined, hasActionable: false }`；
- 两处调用点（`:371` 队列行、`:527` triage）在 `primary` 为空串时**不渲染该文本节点**；
- 空态语义唯一由 pill 承担。单测断言相应改为断言空。

**词表对照**：

| 内部 reason | 用户可见（项目侧） |
|---|---|
| `human_decision` | `N 项目待决` |
| `waiting_human` | `N 等人` |
| `execution_failed` | `N 失败` |
| `runtime_or_coordination` | `N 协调异常` |
| `evidence_required` | `N 证据待核` |
| `sla_waiting` | `等待超时` |
| 全空 | （无文本，仅 pill「暂无阻塞」） |

识别中 / 错误：保持「风险识别中」「风险待确认」。B0 后 pending 窗口应显著缩短（3 请求内返回）。

### 5.2 执行摘要（原「当前处理者」）

数据继续走 `currentHandlerMode` 投影（内部可仍叫 ball），**对外文案降承诺**：

| mode | 用户文案 |
|------|----------|
| `waiting_review` | `等待审核 · {姓名}` |
| `executor` | `执行中 · {数字员工名}` |
| `pending_dispatch` | `待分派` |
| `idle` | `无在办执行` |
| 多活跃任务 | 次行加 tooltip：「多任务时为摘要，非全部任务状态」 |

B0 后可零成本增强：`run_summary.running_count > 1` 时直接显示 `执行中 · N 个任务`，无需产品新规则即可诚实表达多任务（v1 §8 列为待办，B0 后变成免费）。

**禁止**：UI 文案出现「球权」。代码注释可保留工程语义。

### 5.3 词表治理完整清单（v1 遗漏项）

| 位置 | 处置 |
|---|---|
| `project-risk-home.tsx:309-310` | 表头「待办拆分」「当前处理者」→ 新列名 |
| **`project-risk-home.tsx:583`** | **triage 面板内硬编码「待办拆分」，v1 未列，必须同改** |
| `project-risk.ts:141` `reasonLabels` | 用户可见枚举**迁入 `apps/web/src/lib/status-labels.ts`**（CLAUDE.md 硬规范：用户可见中文经词表模块）。v1 §7.4 声称「继续走 status-labels.ts」，但这些字符串从不在那里 |
| **`2026-07-29-project-status-layers.md` §1** | 该表「注意力投影」行的字段列写的是 `reasons / 待办拆分`，是已登记词表条目。改名须同步该 spec，否则本方案治的词表撞车会再生一次 |

### 5.4 页头副标题

现：`围绕项目负责人、服务池、计划确认、执行进展和最终结果推进闭环`（偏详情）。

改为：

> 查看项目组合状态与关注信号，进入项目推进闭环。

（`index.tsx:1032` `ShellPageHeader` subtitle。）

---

## 6. 选中 triage 收敛

组件：`ProjectTriagePanel`。

**保留**：项目名、status pill、关闭选中、按 reason 深链（`REASON_META`）。

**强化**：

1. 分区标题「可行动」/「其它信号」显性化。
2. 每条 reason 优先展示 title（具名）。
3. 底部固定：**进入项目**（主）+ **我的待办**（次，链 `/inbox`）。
4. 静默说明一句：`完整审批与决策处理请在收件箱或项目内完成；本页仅导流。`

**削弱**：与队列「关注摘要」重复的大段内容折叠为默认收起的「信号明细」。

**数据（B0 后变化）**：triage 成为**唯一**触发逐项目 4 连发的位置——选中时才对该项目拉 `tasks/decisions/evidence/members` 以取得 title 与深链 id。这是 4 连发从「每页 40 次」降到「每次选中 1 次」的关键，也是 §7 请求数账目的前提。

---

## 7. B0：数据地基（v2 新增首批）

### 7.1 服务端：`ListProjectRunSummaries` 扩两列 + 一处口径修复

`apps/control-plane/internal/storage/queries/project.sql:2707`：

```sql
-- 修复：内层子查询漏筛已清理任务，failed_count 会把 dismissed 的失败任务算进去
FROM project_tasks
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND dismissed_at IS NULL          -- ← 新增
GROUP BY project_id

-- 新增：项目上的 open 决策数（状态集与既有查询同源，见 project.sql:635/1770/1782）
LEFT JOIN (
    SELECT project_id,
           COUNT(*) FILTER (
               WHERE lower(status_snapshot) IN ('pending','waiting','requested','open')
           )::integer AS open_decision_count
    FROM project_decision_requests
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
    GROUP BY project_id
) d ON d.project_id = p.id

-- 新增：待核证据数（verification_status 与前端 evidenceRequiredStatuses 同源）
LEFT JOIN (
    SELECT project_id,
           COUNT(*) FILTER (
               WHERE verification_status IN ('submitted','rejected')
           )::integer AS evidence_pending_count
    FROM project_evidence_refs
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
    GROUP BY project_id
) e ON e.project_id = p.id
```

- 索引：`idx_project_evidence_refs_tenant_status(tenant_id, project_id, verification_status)` 已存在（迁移 015）；决策侧按 `tenant_id` 聚合，若实测慢再评估加索引。
- **无 DB 迁移**——纯查询变更 + sqlc 重生成。

契约：`contracts/control-plane/openapi.yaml:11078` `ProjectRunSummaryItem` 加 `open_decision_count` / `evidence_pending_count`，与既有计数列一致进 `required`（服务端恒返回）。

### 7.2 `dismissed_at` 修复的跨面影响（必须一起验）

该修复**改变运行总览大屏**（`/run-overview` 项目运行带）的 `failed_count` 显示——存在已清理失败任务的项目数字会变小。这是纠正既有偏差（`ListProjectTasks`、`CountProjectTasks` 一贯排除 dismissed，仅 run-summary 未排除），但属于用户可见变化：

- CHANGELOG 需单列一条；
- §11 验证须连带复验运行总览大屏，不能只验项目首页。

### 7.3 前端接线

| 现状 | B0 后 |
|---|---|
| `useProjectRiskSignals(pagedProjects)` → 40 请求 | 下线列表态调用；仅 triage 对**选中单项目**调用 |
| `listWorkflowInstances({limit:50})` | **整段删除**，最近活动改用 `last_activity_at` |
| `listInboxItems` + `getInboxBadge` | **整段删除**（§3） |
| — | 新增 `listProjectRunSummaries({limit})` 1 个 |

首屏请求：`listProjects` + `run-summary` + `listDigitalEmployees`/`listUsers`（补名，可合并缓存）= **3～4 个**。

**摘要拼装改造**：`deriveProjectRiskSummary` 从「消费 4 个明细列表」改为「消费计数 + 项目字段」的**双入口**：

```
列表态：buildProjectRiskSummaryFromCounts(project, runSummaryItem)  // 只出计数与 mode，无 reason 明细
triage：deriveProjectRiskSummary(...)                               // 现状不变，出 reason 明细与深链
```

两条路径的**桶定义必须同源**（复用 `buildAttentionBreakdown` 的字段名与 `ACTIONABLE_REASON_TYPES`），否则队列格与 triage 面板会显示不同的数字——这是本批次最主要的回归风险，单测须交叉断言同一 fixture 下两路径计数一致。

### 7.4 归档项目的显式规则

`run-summary` 带 `WHERE p.status != 'archived'`（`project.sql:2762`），而队列的状态筛选含「归档」。处置：

- 归档行取不到 `run_summary` item → 摘要回退为空（level=`none`、mode=`idle`）。**这与现状语义一致**——`deriveProjectRiskSummary` 对归档项目本就直接返回 none/idle（`project-risk.ts:186-204`），故无需改端点。
- 归档行「最近活动」回退 `project.updated_at`。

### 7.5 截断诚实化（H9 最小修复）

- 保持 `listProjects({limit:50})`，但当 `projects.length === 50` 时在队列头渲染截断提示（§4.4）。
- 「N 个项目」pill 与 Pagination `total` 标签改为「已加载 N」措辞，不声称全量。
- 真服务端分页（`listProjects` 返回 total）记为 §9 后续。

---

## 8. 实现批次

| 批次 | 内容 | 主要文件 | 风险 |
|------|------|----------|------|
| **B0** | run-summary 扩列 + dismissed 口径修复 + 前端换源 + 双入口摘要 + 截断提示 | `project.sql`、`openapi.yaml`、`handler.go`、`types.go`、`index.tsx`、`project-risk.ts`、`use-project-risk-signals.ts` | **中**：双入口计数须同源；连带影响运行总览 |
| **B1** | 职责收敛：去 KPI 待我决策卡、删两个 inbox 查询、右栏按 §12 拍板结果实现（A/C）、`RunActivityPanel` 列表态下线 | `index.tsx`、`project-risk-home.tsx`、`project-dashboard-rail.tsx` | 低（受影响断言 10 处） |
| **B2** | 表意治理：列名 + 空态修正 + handler 文案 + chip 改名收敛 + 副标题 + §5.3 全量词表清单（含 status-labels 迁移与 07-29 spec 同步） | `project-risk-home.tsx`、`project-risk.ts`、`status-labels.ts`、07-29 spec | 低（受影响断言 8 处） |
| **B3** | 组合摘要 S1 + triage 增量与减重 + H4 pill 上提 | `project-risk-home.tsx` | 低 |
| **B4** | CHANGELOG（含 run-summary 口径变更单列一条）；07-14 / 07-29 spec 文首注记；门禁与真实浏览器 | 文档 + 手测 | — |

**顺序不可交换**：B0 先行，否则 B1/B2 会在即将被替换的数据源上改文案，B2 的空态与计数断言要写两遍。B1+B2 可同 PR。

---

## 9. 后续（本期不做）

| 项 | 说明 |
|----|------|
| `listProjects` 真服务端分页 | 契约返回 total，前端改真分页，替代 §7.5 的截断提示 |
| 决策计数索引 | 若 `project_decision_requests` 租户级聚合实测慢，加 `(tenant_id, project_id, status_snapshot)` 索引 |
| 更准的多执行者 mode | B0 已能显示「执行中 · N 个任务」；完整多执行者语义仍需产品规则 |
| 列表/舒适密度切换 | 规范已有，非必须 |
| 探索原型落地 | A–E 不实施 |

---

## 10. 测试与验证

### 10.1 单测

**B0（新增，重点）**：

- `run-summary` 服务端：两个新计数列的 Go 测试（含 dismissed 任务不计入 `failed_count` 的回归用例）。
- **双入口交叉断言**：同一 fixture 下 `buildProjectRiskSummaryFromCounts` 与 `deriveProjectRiskSummary` 的 4 个可行动桶计数必须相等。
- 归档项目取不到 run-summary item 时回退 none/idle。
- 命中 50 上限时出现截断提示；未命中时不出现任何免责句。

**B1/B2（改写既有，实测 18 处）**：

- `index.test.tsx`：删除「默认右栏含决策列表」「KPI 含待我决策」相关断言（10 处）；新增未选中右栏形态（按 A/C 拍板结果）、「我的待办」链 href、**断言页面不再请求 `/api/v1/inbox/*`**。
- `project-risk.test.ts`：`formatAttentionHeadline` 空态返回空串（8 处相关）；`formatProjectQueueHandlerLabel` 各 mode。
- 多视口无横向溢出门禁沿用。

### 10.2 真实浏览器（完成定义）

在**本会话验证期内服务 pid 未被接管**的前提下（记下并复核 `control-plane`/`web` pid，`owner=` 异源则结论作废）：

1. 登录后打开 `/projects`，**开 DevTools Network 计数**：首屏请求 ≤5，翻页不产生 fan-out。
2. 无右栏 Inbox 决策列表；无 KPI「待我决策」大卡；有「我的待办」弱链（无数字）。
3. 列名与空态可读：无阻塞项目只显示一个「暂无阻塞」pill，**不出现两次**。
4. 队列计数与项目内实际待决/等人/失败数一致（抽 2 个项目进详情交叉核对）。
5. 选中一行 → triage 出现（此时才发生该项目的明细请求）→ 关闭恢复。
6. 点「我的待办」→ 进入收件箱。
7. 项目数 >50 时出现截断提示（可临时造数据或调低 limit 验证）。
8. **连带复验 `/run-overview` 大屏运行带**：`failed_count` 因 dismissed 修复而变化，须与项目内失败任务数一致。

### 10.3 门禁

- `corepack pnpm generate:control-plane`（sqlc + openapi 生成）
- `corepack pnpm verify:contracts`（B0 改契约，必跑）
- `corepack pnpm verify:control-plane`
- `corepack pnpm verify:web`
- 无 DB 迁移，不需 `migrate-validate`
- `git diff --check`

---

## 11. 风险与回滚

| 风险 | 缓解 |
|------|------|
| **双入口计数分叉**（队列格与 triage 数字不一致） | 桶定义单一事实源 + 交叉断言单测；E2E 抽 2 项目人工交叉核对 |
| **dismissed 修复改变运行总览显示** | CHANGELOG 单列；E2E 连带复验大屏；这是纠正既有偏差，不回滚 |
| 租户级聚合慢 | 现有索引已覆盖证据侧；实测超阈值则加决策侧索引（§9） |
| 用户依赖右栏「待我决策」快速扫 | 侧栏角标仍在；弱链保留；CHANGELOG 与发布说明写清 |
| 选 A 后大屏空 | 这正是需 §12 确认的取舍；选 C 则不存在 |
| 「执行摘要」仍被误解为全项目状态 | tooltip +「执行中 · N 个任务」显式多任务 |

**回滚**：B0 前端换源可单独回滚（恢复 `useProjectRiskSignals` 列表态调用 + `listWorkflowInstances`）；服务端扩列是纯附加，回滚前端后无害。B1/B2 回滚 = 恢复 `ProjectDashboardRail` 挂载与 KPI 第 5 卡。全程无迁移。

---

## 12. 决策待评审勾选

实现按 v2 建议默认落地（开发会话默认采纳）：

- [x] **未选中右栏 = C 组合透视面板**
- [x] 组合摘要 = **S1 紧凑真值条**
- [x] 摘要总数标签暂仍为「已加载」（列表仍 `limit:50`）；run-summary 计数用于行级信号而非替换组合 total 为「全部」
- [x] 「项目待决」完整四字
- [x] `run-summary` 的 `dismissed_at` 口径修复一并做

---

## 13. 文档与变更记录

- 实施后 `CHANGELOG.md` 记两条：①项目管理首页数据地基收敛与职责治理；②**run-summary `failed_count` 口径修复（排除已清理任务），影响运行总览大屏**。
- 本文件状态改为「已批准 / 已实施」并补验证摘录（含首屏请求数实测值）。
- `2026-07-14-projects-dashboard-design.md` 文首增修订指针；若选 A，另注明「铺满靠增加维度」原则撤销。
- `2026-07-29-project-status-layers.md` §1 词表行同步改名。

---

## 14. 附录：与探索原型的关系

| 原型 | 结论 |
|------|------|
| A 决策优先 / C 泳道 / C2 决策条 | 易使 PM 页 ≈ Inbox，**不采用**为首页主形态 |
| C3 抽屉 | 个人待办按需可接受，但本期用链到收件箱更简单 |
| D 组合表 | 职责方向对，但列语义与「第二张表」创新不足；现网表 + 本方案治理更优 |
| E 叙事卡 | 状态分层表达好，但替换成本高且未证明优于现网；不作为本期 |

---

## 15. 参考代码锚点

- 页面接线：`apps/web/src/features/projects/index.tsx`（`:305` limit 50、`:377` 客户端分页、`:1078` KPI、`:1088` detail 槽位）
- 队列与摘要：`project-risk-home.tsx`（`:126` 摘要条、`:236` 待删说明句、`:309` 表头、`:440` 空态 pill、`:583` triage 词表遗漏）
- 右栏：`project-dashboard-rail.tsx`
- 投影：`project-risk.ts`（`:89` chip、`:141` reasonLabels、`:574` headline、`:754` handler label）
- fan-out：`hooks/use-project-risk-signals.ts`
- 布局原生按需详情：`components/superteam/layout.tsx:38`
- 侧栏角标（inbox 数字唯一出口）：`components/layout/app-sidebar.tsx:39`
- 服务端聚合：`storage/queries/project.sql:2707`、`internal/project/service.go:835`、`handler.go:4774`
- 契约：`contracts/control-plane/openapi.yaml:1043`、`:11078`
