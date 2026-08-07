# Console 状态文案中文化与通用化设计

日期：2026-07-11  
> 复核状态：基于CHANGELOG证据
状态：已确认，待实现计划

## 1. 背景

团队管理多处直接渲染英文 status（如数字员工 `active`、MCP binding `active`），与控制台其余中文界面不一致。

项目管理侧已有共享模块 `apps/web/src/lib/status-labels.ts`，任务/运行/审批/证据等多数走 `statusLabel()`；但项目生命周期状态仍在 `project-config-page`、`project-risk-home`、`project-operational-detail` 各维护一份本地中文表，属于特例化重复。

同一码在不同域语义不同：`active` 在团队为「活跃」、治理为「已生效」、数字员工为「运行中」、共享默认可为「启用中」。不能用一张扁平表硬套所有展示。

## 2. 目标

- 团队管理展示层不再裸渲染英文 status。
- 以 `status-labels.ts` 为 Console 状态文案唯一源；无歧义码进共享表，有歧义码用薄包装覆盖。
- 收敛项目生命周期状态的三处本地重复表，统一为导出的 `projectStatusLabel`。
- 筛选下拉与展示层同源，避免第三份硬编码中文。

## 3. 非目标

- 不做全站扫尾（员工详情、runtime overview、skills 等仍英文或自带 label 的面）；列为后续「状态文案统一」专项。
- 不统一 `StatusPill` tone；tone 可继续本地判断。
- 不改事件时间线叙事文案。
- 不改 `projectPhaseLabel` / `demandStatusLabel` 的既有语义（相位≠状态；需求 `submitted`≠通用「已提交」）。
- 不引入 i18n 框架或多语言切换；本次仅简体中文产品文案。
- 不改 API / 契约中的英文枚举值；仅 UI 展示层映射。

## 4. 方案

采用：**共享码表 + 冲突处薄包装**。

### 4.1 共享模块契约

文件：`apps/web/src/lib/status-labels.ts`

| 规则 | 说明 |
|---|---|
| 无歧义码 | 进入 `STATUS_LABELS`，经 `statusLabel(code)` 解析（trim + lower；未知回退原文） |
| 有歧义码 | 必须用域包装，禁止在团队/治理/员工/项目生命周期展示上直接依赖共享默认 |
| 共享默认 `active` | `"启用中"`，仅给无包装的通用调用 |
| Tone | 本次不进共享模块 |

新增/导出包装：

| 函数 | 覆盖要点 |
|---|---|
| `teamStatusLabel` | `active`→活跃；`disabled`→已禁用；`archived`→已归档 |
| `governanceStatusLabel` | `active`→已生效；`not_configured`→未配置；`draft_pending`→草案待批准；`needs_update`→需更新 |
| `employeeStatusLabel` | `active`→运行中；`error`→异常；其余走共享表（draft/ready/disabled） |
| `projectStatusLabel` | `draft`→草稿；`configuring`→配置中；`running`→运行中；`paused`→已暂停；`acceptance`→验收中；`archived`→已归档 |

可选同文件迁入（保持独立，不并入通用 `statusLabel`）：

- `projectPhaseLabel`
- `demandStatusLabel`

共享表需补齐本次用到的无歧义码（若缺失）：如 `archived`、`paused`、`configuring`、`acceptance`、`error` 等；治理专用码可只放在 `governanceStatusLabel` 覆盖表。

### 4.2 团队管理改动面

| 位置 | 现状 | 改法 |
|---|---|---|
| `team-detail-layout.tsx` `TeamStatusPill` | 本地中文 map | 文案→`teamStatusLabel`；tone 可留本地 |
| `team-overview-tab.tsx` 数字员工状态列 | `{employee.status}` | →`employeeStatusLabel` |
| `team-capabilities-tab.tsx` MCP 绑定状态 | `{binding.status}` | →`statusLabel`（`active` 显示「启用中」） |
| `create-team-step-members.tsx` | 本地 `EMPLOYEE_STATUS_PRESENTATION` | label→`employeeStatusLabel`；tone 可留本地 |
| `create-team-digital-employees-step.tsx` | `状态: {employee.status}` | →`employeeStatusLabel` |
| `team-management-toolbar.tsx` | Select 硬编码中文 | 选项文案→`teamStatusLabel` / `governanceStatusLabel` |

不做：列表统计 pill、角色 badge、风险等级、审计叙事。

### 4.3 项目特例收敛

| 位置 | 处理 |
|---|---|
| `project-config-page.tsx` 本地 `statusLabel` | 删除；改用导出的 `projectStatusLabel` |
| `project-risk-home.tsx` 本地 `projectStatusLabel` | 删除；改用共享导出；筛选选项同源 |
| `project-operational-detail.tsx` 本地 `projectStatusLabel` | 删除；改用共享导出 |

已走共享映射的任务/运行/审批/证据/验收/工件调用点本次不动。

`projectPhaseLabel`、`demandStatusLabel`、事件叙事保留独立语义；若迁入同文件，不得并入通用 `statusLabel`。

## 5. 验证

- 更新受影响组件测试中对英文 status 文案的断言（若有）。
- `corepack pnpm --filter @superteam/web test` 覆盖 teams / projects 相关测试；提交前按仓库约定跑 `verify:web`。
- 浏览器：团队详情概览数字员工状态、能力绑定状态、团队状态 pill、创建流程员工状态为中文；项目列表/详情/配置页生命周期状态仍为既有中文且与共享表一致。

## 6. 后续（非本次）

全站状态文案统一：员工详情、runtime overview、skills 等强制走同一契约；可再评估是否引入 `statusLabel(code, domain)` 命名空间 API。
