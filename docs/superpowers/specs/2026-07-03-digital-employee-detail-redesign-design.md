# 数字员工详情页重构设计

> 日期：2026-07-03
> 范围：`apps/web` 数字员工详情页 + Control Plane 已有数据的接口暴露
> 原型：本次会话由用户提供的数字员工详情页原型图（矩枢平台 · 后端实现员），未落盘为文件

## 1. 背景与目标

当前数字员工详情页（`apps/web/src/features/employees/detail.tsx`）是**操作导向**的：核心是「开始任务」表单、「停止」按钮、实时「事件流」和内联 capabilities 面板，概览信息薄。

目标原型是**观察 + 配置导向**的控制台详情页：

- 详情头卡（头像 + 名称 + 状态 + 会话身份 + 角色/负责人/描述 + 主操作）
- 执行指标条（Provider、Runtime + 命令通道、累计/近7天+趋势、成功率、平均耗时/P90、成功/失败/人工停止、当前状态）
- 左右两栏：左「历史执行任务」表（可筛选、分页），右「生效上下文」面板（基本信息、技能、MCP、宪法与记忆、环境变量）
- 底部「下次任务会注入的上下文包」按注入顺序只读链

本设计把详情页重构为原型 IA，并**补齐已有数据但未暴露的接口**；不伪造任何无数据源的指标。

### 范围决策（已与用户确认）

1. **数据边界**：原型指标中，凡底层数据已存在于 Control Plane（主要是 `task_runs`、`digital_employee_effective_configs`、环境变量汇总）但没有接口暴露的，**补充接口能力**；底层完全无数据源的（记忆条目数/记忆来源）**占位「待接入」，不伪造**。
2. **操作功能去向（默认，待 review 确认）**：采用「保留但收进抽屉/弹窗」。详情页主体按原型做成观察/配置导向；`开始任务`收到头部动作按钮触发的抽屉；点历史执行行打开包含`事件流`的运行详情抽屉；capabilities 折进「生效上下文」面板 + 查看全部。**不删除任何现有可用功能。** 若 review 阶段改为「详情页只读、移除开始任务」，只需去掉开始任务抽屉入口，其余不变。

## 2. 数据可用性矩阵

| 原型元素 | 数据来源 | 处理 |
| --- | --- | --- |
| Provider / Runtime 执行位置 / 命令通道 / 当前状态 | 现有 `getDigitalEmployee` + `getDigitalEmployeeExecutionInstance` + `getRuntimeOverview` | 已可用 |
| 累计执行、成功率、成功/失败/人工停止、平均耗时、P90、近7天+环比趋势 | `task_runs`（有 `status`/`created_at`/`started_at`/`finished_at`/`timed_out` + `idx_task_runs_employee_status`） | **新增聚合接口** |
| 历史执行表（任务标题、项目名、会话ID、Runtime、状态、耗时、工件数、结果、时间）+ 状态/项目/时间筛选 + 分页总数 | `task_runs` join `tasks` → `projects`；`work_products` 数组长度 | **扩展列表接口** |
| 生效上下文：技能 个人/团队继承/生效、MCP 同、宪法层级、工作目录 | 现有 `getDigitalEmployeeEffectiveConfig` 返回的 `effective_config` JSONB（已由后端组合） | **优先前端派生**；仅当某计数不在 JSONB 中才补后端字段 |
| 环境变量 已配置/缺失/总数 + 缺失变量名 | 现有 `listEmployeeEnvironmentVariables`（`configured` 布尔 + `name`） | 客户端可算 |
| 记忆条目数 / 记忆来源 | **无 memory 表，全平台无数据源** | **占位「待接入」，不伪造数字** |

## 3. 统计口径（全部时间，来自 `task_runs`）

- 成功 = `status = completed`
- 失败 = `status IN (failed, timed_out)`
- 人工停止 = `status = cancelled`
- 累计执行 = 全部运行数（校验：68 + 5 + 3 = 76 ✓）
- 成功率 = 成功 / 累计（校验：68 / 76 = 89.5% ✓）
- 平均耗时 = `avg(finished_at - started_at)`（仅 `finished_at`、`started_at` 均非空的行）
- P90 耗时 = `percentile_cont(0.9) within group (order by finished_at - started_at)`
- 近7天 = `created_at >= now() - interval '7 days'` 的计数
- 环比趋势 = (近7天 − 前7天) / 前7天；前7天 = `[now-14d, now-7d)`；前7天为 0 时不显示百分比、只显示绝对值

所有口径均单条 SQL 用 `COUNT(*) FILTER (...)` + `percentile_cont` 聚合完成，租户 + 员工维度，命中现有索引。

## 4. 后端改动（Control Plane）

### 4.1 执行统计聚合接口

- 新增 sqlc 查询 `GetDigitalEmployeeRunStats`（`internal/storage/queries/employee_execution.sql`），按 `tenant_id + digital_employee_id` 对 `task_runs` 聚合，返回：`total_count`、`succeeded_count`、`failed_count`、`cancelled_count`、`success_rate`、`avg_duration_sec`、`p90_duration_sec`、`last_7d_count`、`prev_7d_count`。
- 新增 HTTP 端点 `GET /digital-employees/{id}/run-stats`（沿用现有员工路由分组与鉴权中间件）。
- 契约：在 `contracts/` 中新增该响应 schema，走生成 + 契约验证流程。

### 4.2 运行历史列表增强

- 扩展现有运行列表查询/端点（`listDigitalEmployeeRuns` 对应的后端）：
  - join `tasks`（取任务标题）→ `projects`（取项目名）
  - 返回 `work_products` 数组长度（工件数）与 `duration_sec`（`finished_at - started_at`）
  - 新增查询参数：`status`（单/多状态）、`project_id`、`from`/`to`（时间窗，默认近30天）、`limit`/`offset`
  - 返回分页 `total_count`
  - 附带可用筛选项：出现过的 `projects`、`statuses`（供前端筛选下拉，避免前端硬编码枚举）
- 契约同步更新并重新生成。

### 4.3 生效上下文（尽量不动后端）

- 先确认 `effective_config` JSONB 是否已含技能个人/团队拆分、MCP 拆分、宪法层级、工作目录。**若已含则纯前端派生**。
- 仅当缺某个原型需要的计数时，才在 effective config 读取端点补计算字段（不新建表）。

### 4.4 迁移

- 预计**无需新迁移**（复用 `task_runs` 与既有索引）。若聚合查询在真实数据规模下需要额外索引，再按 `DATABASE_DESIGN.md` 走迁移并更新 `atlas.sum`。

## 5. 前端改动（`apps/web`）

### 5.1 页面结构（`features/employees/detail.tsx` 重构）

复用 `@/components/superteam` v3 组件，不手搓卡片/表格/pill：

1. **详情头卡** — 头像（`features/employees/avatar.tsx` 的 `Avatar`）+ 名称 + `StatusPill` + 会话身份副标题 + 角色/负责人/描述 meta 行。动作区：`返回列表`（Link→`/employees`）、`编辑员工配置`（Link→`/employees/$employeeId/config`）、`更换 Runtime`（主按钮，链接到 config 的 runtime 段或复用现有绑定流程）、`查看审计`（Link→`/audit` 带员工筛选 search 参数）、`开始任务`（打开抽屉，默认方案 A）。
2. **执行指标条** — `V3MetricCard` 栅格（响应式换行）：Provider、Runtime 执行位置（含命令通道 `StatusPill`）、累计执行、近7天（含趋势）、成功率、平均耗时（副标 P90）、成功、失败、人工停止、当前状态。数据来自 4.1 接口 + 现有 instance/runtime 查询。
3. **左右两栏** `grid lg:grid-cols-[minmax(0,1.2fr)_minmax(320px,0.8fr)]`：
   - **左·历史执行任务** — `WorkSurface` + `V3Table`（列：任务/项目、会话ID、Runtime、状态 `StatusPill`、耗时、工件、结果、时间）。工具栏：状态/项目/时间 `V3Segmented`/下拉 + 刷新 `V3IconButton`。`V3Pagination` 页脚。四态用 `V3StateSurface`。行点击 → 运行详情抽屉（含结果、失败原因、`事件流`、活跃运行的停止按钮 —— 迁移自现有 `ResultBlock`/`FailureBlock`/`RunEventRow`/stop 逻辑）。
   - **右·生效上下文** — `SoftCard` 面板，标题带 `编辑`（Link→config）。子块：基本信息（Provider/角色/所属团队/状态/创建/更新/Runtime 执行位置/工作空间/命令通道）、技能（个人/团队继承/生效 + 进度条 + 查看全部→`/skills`）、MCP（个人/团队/生效 + 查看全部→`/mcp`）、宪法与记忆（宪法层级 + **记忆条目「待接入」占位** + 最近更新 + 记忆来源 + 查看详情）、环境变量（已配置/缺失/总数 + 缺失变量 pill + 查看详情）。
4. **注入顺序链**（`SoftCard`）— 「下次任务会注入的上下文包（按注入顺序 · 只读）」：固定 8 节点 `角色说明 → 宪法 → 记忆 → 个人技能 → 团队继承技能 → MCP → 环境变量 → 工作目录`，每节点 `IconTile` + 摘要计数；`记忆`节点标「待接入」中性态。

### 5.2 API 客户端（`lib/api/employees.ts`）

- 新增 `getDigitalEmployeeRunStats(id)` + `DigitalEmployeeRunStats` 类型。
- 扩展运行列表函数签名，支持筛选参数与增强返回字段（任务标题、项目名、工件数、耗时、total、可用筛选项）。

### 5.3 状态与四态

覆盖 loading / empty / error / permission denied / disabled / 长任务执行中；活跃运行时指标条与历史表按现有 `refetchInterval` 轮询逻辑刷新。

## 6. 组件边界

- **详情页容器** `EmployeeDetailView`：只做数据编排与布局，子块拆成可独立理解的组件：
  - `EmployeeDetailHeader`（头卡 + 动作）
  - `EmployeeMetricsStrip`（指标条，输入 stats + instance + runtime）
  - `EmployeeRunHistoryTable`（表 + 筛选 + 分页 + 行点击回调）
  - `RunDetailDrawer`（结果/失败/事件流/停止）
  - `StartTaskDrawer`（迁移自现有开始任务表单）
  - `EffectiveContextPanel`（生效上下文各子块，输入 effective config + env vars）
  - `ContextInjectionChain`（注入顺序链）
- 每个组件：明确输入 props、无隐藏全局依赖、可单测。

## 7. 验证

- 前端：`corepack pnpm --filter ./apps/web run test`；受影响组件测试更新。
- 后端：sqlc 生成、契约生成 + 契约验证、`make -C apps/control-plane` 相关校验。
- **真实端到端**（收尾必做，`$superteam-completion-check`）：`scripts/dev-services.sh status` 确认服务加载当前代码 → 浏览器打开 `/employees/$employeeId` → 校验指标条数字来自真实 `run-stats` 接口、历史表筛选/分页走真实接口、生效上下文来自真实 effective config、记忆节点为「待接入」而非假数字、开始任务/停止/事件流抽屉链路可用；确认无横向溢出、四态正确。

## 8. 明确不做（YAGNI）

- 不新建 memory 子系统 / memory 表；记忆条目仅占位。
- 不做客户端样本近似统计（会导致口径失真）。
- 不引入与本页无关的重构。

## 9. 开放问题（review 阶段确认）

1. 操作功能去向：确认默认「保留收进抽屉」是否符合预期，或改为「详情页只读、移除开始任务」。
2. `更换 Runtime` 主操作：复用 config 页 runtime 段，还是需要独立切换弹窗？
3. `effective_config` JSONB 是否已含技能/MCP 个人vs团队拆分与宪法层级——决定是否需要 4.3 的后端补字段。
