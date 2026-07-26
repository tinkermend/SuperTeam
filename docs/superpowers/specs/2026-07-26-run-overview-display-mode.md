# 运行总览大屏模式 + 项目运行带 Spec（草案）

> 日期：2026-07-26
> 状态：**已对齐拍板（2026-07-26，决议记录见 §8），未实施**
> 性质：在维持「团队底座 + 项目透镜」维度裁决（2026-07-18 spec）不变的前提下，补齐两块缺口——常驻的项目维度聚合（项目运行带）与无交互投屏形态（大屏模式）。
> 关联：`2026-07-18-run-overview-project-overlay-design.md`（维度裁决）；`2026-07-26-canonical-flow-graph-phase1.md`（IA 视角轴：运行总览 = 物理/实时轴）。

---

## 0. 动因

用户提出两个问题：

1. **项目视角缺失**：一个项目的多个任务派给不同数字员工后，运行总览上看不到「这个项目有哪几个任务在运行、状态怎么样」。现状项目透镜能回答，但它是**点击驱动的点查**——不选中项目就什么都不显示，没有常驻的项目聚合视图。
2. **大屏投屏诉求**：运行总览要作为企业内大屏投屏展示。投屏场景**没有交互**，一切靠点击才出现的信息（透镜、员工详情卡、侧栏展开）在墙上信息量为零；且缺少大屏该喊出来的关键数据——有几件事在等人、异常几起、今天干成了什么。

**维度裁决不重开**：底座保持团队维度（资源/物理轴，员工唯一、一人一座），项目不做成容器（重复头像 = 建模错位），这在 07-18 spec 已论证并落地。本 spec 补的是：把项目维度从「透镜（点查）」升级为「运行带（常驻聚合）+ 透镜（放大镜）」，并为无交互场景做镜头编排。

## 1. 现状盘点（实现锚点）

| 部件 | 位置 | 现状 |
|---|---|---|
| 页面入口 | `apps/web/src/features/run-overview/index.tsx` | overview/teams/activity 三查询 10s 轮询 + SSE 秒级插队（节流 2s）；`?employee=`/`?project=` 深链；员工焦点轮播 |
| 侧栏 | `components/runtime-overview-side-panel.tsx` | 6 项 Metric（团队/员工/容量/异常/关联项目/今日 tokens）+ 7 态分布 + 项目透镜区块 + 最新动态 5 条 |
| 项目透镜 | `runtime-overview-project-lens.ts` | 选中项目 → 高亮参与者 + 座位间交接连线；项目列表来自 `aggregateLensProjectOptions`（员工 `projects[]` 反向聚合） |
| 员工数据 | `getDigitalEmployeeOverview` `limit: 100` | 员工节点带 `projects[]`（`activeTaskCount`/`workingTaskCount`/`totalTaskCount`）；**limit 100 截断债** |
| 任务链路 | `getProjectTaskGraph(projectId, {demandId})` | 服务端强制 demand 域，透镜只显选定（默认最新）demand |
| 轮播 | `use-runtime-focus-carousel.ts` | 员工焦点单一镜头；透镜态强制暂停 |

**可直接复用的现成端点**（大屏 KPI 不需要为它们新建后端）：

| 数据 | 端点 | 关键字段 |
|---|---|---|
| 人类等待队列 | `getInboxBadge` → `/api/v1/inbox/badge` | `mine_open_count` / `team_open_count` / `high_risk_count` |
| Runtime 健康 | `getRuntimeOverview`（`lib/api/runtime.ts`） | `online_nodes`/`total_nodes`/`pending_enrollments`/`active_provider_sessions`/`blocked_events` |
| 成本 | `getCostSummary`（`lib/api/costs.ts`） | 今日消耗汇总（侧栏已用 overview 内的 `todayTokensTotal`） |

**缺口（需要后端）**：

- **项目运行聚合**：现有前端反向聚合有两个盲区——员工 limit 100 截断、项目无参与员工（任务全部待派发）时完全不可见；且拿不到「阻塞数 / 待人工确认数」。要让项目运行带的数字可信，需要服务端按项目聚合 `project_tasks` 状态。
- **今日吞吐**：`cumulativeTaskCount` 是累计值，「今日完成任务数」无现成端点（`task_runs` 已有 `project_id` 一等列与时间列，服务端聚合成本低）。

## 2. P1：项目运行带（常驻项目聚合）

把侧栏「项目透镜」区块升级为「项目运行」区块，**不选中也常显**每个活跃项目的运行摘要；透镜降级为它的放大镜交互（点击项目行 → 进透镜，现有行为不变）。

### 2.1 后端：项目运行摘要端点（本 spec 唯一新后端）

`GET /api/v1/projects/run-summary`（名称与契约细节实现时定，走 `contracts/` 生成流程）：

- 返回活跃项目列表，每项：`project_id`、`name`、`status`、按状态聚合的任务计数（运行中 / 排队 / 阻塞 / 待人工 / 今日完成 / 待派发）、最新活动时间、参与员工数。
- 一次查询聚合（`project_tasks` group by project + status），权限口径与 `listProjects` 一致。
- 排序：有阻塞/待人工的在前，其次按最新活动时间。

### 2.2 前端

- `runtime-overview-side-panel.tsx`：项目行常驻显示计数徽标——`运行 N`、`失败 N`（danger 色；`project_tasks.status = failed` 是权威落点，"阻塞"无独立任务状态，不造语义）、`待人工 N`（warn 色）、`待派发 N`；零值不显。数据源切到 run-summary 端点，端点不可用时降级员工反向聚合（细粒度计数缺失置零，`source` 标记）。
- 轮询与 overview 同频 10s，SSE 到达时一并 invalidate。
- 顺带偿债：项目列表不再受员工 limit 100 与参与者盲区影响（全部待派发的项目也可见，显示`待派发 N`）。

## 3. P2：大屏模式（`/run-overview?mode=display`）

### 3.1 形态

- **布局**：去 MasterDetail 侧栏，地图 stage 全宽；顶部一条 **KPI 带**（大字号、远距可读），底部一条**滚动动态流**（复用 activity 数据）。工具栏按钮（刷新/暂停）隐藏，交互不禁用（走到屏前仍可点）。
- **KPI 带**（左→右按「要人处理的事优先」排序）：
  1. `待人工处理 N`（inbox badge `team_open_count`，高危 `high_risk_count` 红色角标）——平台核心是人类守门，这是大屏第一 KPI；
  2. `异常 N`（现有 `errorCount`，>0 红色）；
  3. `运行中 N`（`workingCount`）；
  4. `今日完成运行 N`（执行完成口径：`task_runs` 当日 completed，随 run-summary 聚合返回，见 §8-3）；
  5. `Runtime 在线 N/M`（`online_nodes/total_nodes`，掉线红色）；
  6. `今日消耗 tokens`（现有）。
- **进入方式**：`?mode=display` search param（复用现有 `validateSearch` 模式），可直接做成投屏设备的启动 URL；不新增路由。

### 3.2 镜头编排（无交互时的信息轮转）

扩展现有员工焦点轮播为三种镜头，循环编排：

1. **员工焦点镜头**（现有）：逐个聚焦有动态的员工，楼层跟随。
2. **项目链路镜头**（新）：逐个自动进入活跃项目的透镜态 N 秒——高亮参与者 + 交接连线 + 项目摘要浮层（复用 lens 与 run-summary 数据；demand 取最新，遗留债照旧，见 §5）。
3. **异常插队镜头**（新）：SSE 收到 failed/error 事件时立即插队聚焦该员工/项目，停留时长加倍；处理完回到正常轮转。

交互即暂停整套编排（复用现有 interacted/pause 机制），超时自动恢复。

### 3.3 改动清单

| 文件 | 改动 |
|---|---|
| `index.tsx` | `mode=display` 消费、布局分叉、新增 inbox badge / runtime overview 查询（仅大屏模式启用） |
| `use-runtime-focus-carousel.ts` | 镜头队列从「员工」泛化为「员工 \| 项目 \| 异常插队」三型（纯函数可单测） |
| 新增 `components/display-kpi-band.tsx`、`display-activity-ticker.tsx` | KPI 带与底部动态流 |
| `runtime-overview-side-panel.tsx` | P1 项目运行带（普通模式与大屏共用数据源） |

实现前必须阅读 `DESIGN.md`（大字号/远距密度属于新的内容形态，可能需要补设计规范条目而非页面特例）。

## 4. 数据流与频率

- 大屏模式不改变轮询频率（10s + SSE 插队），新增的 badge/runtime 查询同频；投屏长驻场景依赖 EventSource 自动重连 + 轮询兜底（现有机制，不新建通道）。
- 大屏为长驻页面，需验证 24h 级别内存与查询缓存行为（React Query 默认即可，验收时观察）。

## 5. 非目标与已知承接债

**非目标**：

- 不改底座维度（团队/物理轴不动），不做项目泳道/看板重构。
- 不承载任务级操作（大屏只读，交互仅导航/聚焦）。
- 不做历史趋势图表（大屏 v1 只做实时快照；趋势另立项）。
- 不动 activity/SSE 通道本身。

**承接的已知债（不在本 spec 修）**：

- task-graph 强制 demand 域 → 项目链路镜头只显最新 demand；多 demand 项目全貌需后端新端点，已在 IA 重构范围内讨论，本 spec 不扩。
- 员工 overview limit 100 截断：地图本身失真的既有债；P1 使项目计数脱离该债，地图不管。
- 不可见团队员工地图不可见（07-18 spec §7 已记，待立项）。

## 6. 验收（GATE，真实 E2E）

前置：`scripts/dev-services.sh status` 全栈运行，真实 CP + PG 数据。

- **P1-G1 运行带常显**：不选中任何项目，侧栏项目行显示运行/阻塞/待人工计数，与 DB 直查一致（造 fixture：一项目两任务，一 running 一 blocked）。
- **P1-G2 无参与者项目可见**：造一个任务全部待派发的项目，运行带出现该项目并显示`待派发 N`（现状反向聚合下它不可见，此为回归防线）。
- **P2-G1 大屏布局**：`?mode=display` 直开——无侧栏、KPI 带渲染、数字与普通模式/收件箱徽标一致。
- **P2-G2 镜头轮转**：无交互静置，观察员工镜头 → 项目透镜镜头自动切换，项目镜头出现连线与摘要浮层。
- **P2-G3 异常插队**：真实触发一次任务失败（或直插 fixture），SSE 到达后镜头插队聚焦，KPI 异常数 +1。
- **P2-G4 交互暂停/恢复**：点击任意员工暂停编排，超时后自动恢复轮转。

分层门禁：`corepack pnpm verify:web`（镜头编排纯函数、run-summary 适配必须有单测）；`__screenshots__` 快照更新；契约变更走 `generate:control-plane` + 契约验证。

## 7. 风险

- **KPI 口径漂移**：大屏数字与收件箱/项目页数字不一致会立刻被投屏放大（多人同时看）。对策：KPI 全部取自权威端点（badge/run-summary），不做前端二次推导；G1/P2-G1 显式验证跨页一致。
- **镜头编排复杂度**：三型镜头 + SSE 插队 + 交互暂停的状态机是本 spec 最容易出细碎 bug 的地方。对策：编排逻辑纯函数化 + 单测覆盖插队/恢复/空队列边界。
- **投屏设备登录态**：大屏设备如何维持认证（现有 cookie 会话过期即黑屏）。见 §8-5，可能是范围外运维项，但必须先拍板。

## 8. 对齐决议（2026-07-26 拍板）

1. **受众与信息尺度**：**内部运维墙**（用户拍板）。全量信息上屏，成本/token/项目名不做隐藏；不考虑访客脱敏形态。
2. **KPI 清单**：维持 §3.1 的 6 项（受众定为内部运维墙后成本项保留；未加验收通过数——今日吞吐口径已定执行侧，见 3）。实施中如一行放不下，按 §3.1 序号从尾部裁。
3. **今日吞吐口径**：**执行完成口径**（用户拍板）——按 `task_runs` 当日 completed 计（`task_runs` 已有 `project_id` 一等列与时间列）；不用验收签署口径。KPI 文案用「今日完成运行」避免与业务验收混淆。
4. **run-summary 端点范围**：v1 **只服务运行总览**；契约设计上保持通用（不带 run-overview 专属字段），项目列表页/透镜统一供数归 IA 重构（canonical-flow-graph 后续阶段）再收敛，不在本 spec 扩范围。
5. **大屏认证形态**：**另立运维项，后期再考虑**（用户拍板，已记入根目录 `TODO.md` 2026-07-26 条）。本 spec 交付以现有登录态浏览器可用为验收边界，不解决长驻凭证。
6. **分期节奏**：**分期**——P1（项目运行带 + run-summary 端点）先行合并，对普通模式即时改善；P2（大屏模式）跟进。每期独立走 GATE 与收尾门禁。

**镜头编排规模说明**（自审结论）：三型镜头保留——大屏模式无侧栏，项目运行带不可见，「项目链路镜头」是大屏上项目维度的唯一呈现通道，属承重件而非装饰；若实施中编排状态机复杂度失控，优先砍的是镜头内动效而非项目镜头本身。
