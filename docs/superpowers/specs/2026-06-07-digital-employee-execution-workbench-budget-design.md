# 数字员工执行工作台与预算治理设计

日期：2026-06-07
状态：已确认，待实现计划

## 1. 背景

`/employees` 当前已经有 `GET /api/v1/digital-employees/overview` 工作台 read model，能聚合数字员工身份、执行实例、最近运行、治理和 30 天 token 用量。但页面展示仍偏表格化，不能突出“数字员工 = 业务身份 + 唯一执行实例 + Runtime/Provider 绑定”的核心语义。

本轮按确认后的执行工作台图片改造数字员工首页：默认只做工作台卡片视图，不做名册切换。页面应区别于团队管理页：团队管理表达组织容器、人类负责人和成员集合；数字员工页表达单个执行对象的状态、绑定、最近运行、治理配置和预算边界。

本轮同时修正预算语义：预算不是前端展示约定，也不是普通 `metadata`，而是数字员工治理配置的一部分。创建和配置页都必须能维护每日 token 预算上限；未填写表示无预算上限。预算达到上限时，Control Plane 必须在发起运行前拦截。

## 2. 目标

- 将 `/employees` 改造成单一工作台视图，按卡片展示数字员工执行对象。
- 扩展 `/api/v1/digital-employees/overview`，一次返回工作台所需的卡片、待处理队列、最近事件和每日预算摘要。
- 新增独立 `budget_policy` 配置块，进入数字员工配置版本和生效配置快照。
- 创建数字员工时可填写每日 token 预算上限；配置页可调整预算并创建新的配置版本。
- 按 `Asia/Shanghai` 业务日统计每日 token 用量，达到预算上限时阻止发起 run。
- 保留现有详情、配置和创建入口，不重做数字员工详情页。

## 3. 非目标

- 不实现名册视图切换。
- 不实现启动、停止或批量运行按钮。
- 不把预算上限写入 `metadata`。
- 不实现成本中心、预算审批流、预算流水账或租户级预算继承。
- 不实现员工自定义时区；本期固定使用 `Asia/Shanghai` 业务日。
- 不重构团队管理页，也不修改团队卡片设计。

## 4. 后端契约

继续使用：

```http
GET /api/v1/digital-employees/overview
```

现有 `summary`、`items`、`filters`、`pagination` 结构保留，并新增工作台字段。

### 4.1 Summary 与待处理队列

`summary` 的展示语义调整为工作台队列：

- `ready_count`：就绪数字员工数量。
- `pending_runtime_binding_count`：待绑定 Runtime 数量。
- `error_count`：异常数字员工数量。
- `pending_config_approval_count`：配置待审批或待更新数量。
- `failed_recent_run_count`：最近运行失败数量。

为了兼容现有调用，必须保留旧字段 `total_count`、`runnable_count`、`running_count`、`waiting_runtime_count`、`high_risk_count`，但新工作台 UI 只使用上述语义。

新增：

```json
{
  "queue_summary": {
    "pending_runtime_binding_count": 2,
    "stale_config_count": 4,
    "failed_recent_run_count": 1
  }
}
```

`queue_summary` 用于右侧“待处理队列”。`stale_config_count` 覆盖配置缺失、待审批和过期三类需要处理的治理状态。

### 4.2 Item 状态

每个 `items[]` 保留原有 summary 子对象，并新增或规范以下派生字段：

```json
{
  "workbench_status": "ready",
  "recent_events": [
    {
      "label": "命令已下发",
      "status": "completed",
      "occurred_at": "2026-06-07T15:10:00Z"
    }
  ]
}
```

`workbench_status` 只允许：

- `ready`：显示为“就绪”。员工可运行、执行绑定可用、治理配置可用。
- `pending_binding`：显示为“待绑定”。缺少执行实例、Runtime Agent 或 Provider 绑定。
- `error`：显示为“异常”。员工、执行实例、Runtime/Provider 健康或最近运行存在阻断异常。

`recent_events` 每个员工最多返回 3 条轻量事件，用于右侧选中员工面板。只返回 `label`、`status`、`occurred_at`，不返回 payload、session state、原始 Provider 输出或敏感 metadata。

### 4.3 卡片字段规则

前端卡片只使用以下展示语义：

- 员工头像、名称、角色/类型、团队。
- 状态只显示“就绪 / 待绑定 / 异常”。
- Runtime 行只显示 `Runtime Agent · Provider`，例如 `local-dev-node · Claude Code`、`prod-node-02 · OpenCode`。Provider 名称来自服务端注册表或 overview filter label，不在前端写死。
- 待绑定时显示 `等待绑定 Runtime Agent`。
- 最近运行只显示 `成功/失败 + 时间`，例如 `成功 · 2 分钟前`、`失败 · 15 分钟前`；无运行显示 `-`。
- 治理行显示 `配置 vN 已审批/待审批 · skills N · MCP N`。
- 底部操作只保留 `详情` 和 `配置`。

卡片不得显示：

- `生效`、`active`、`ready` 原始状态文本。
- `执行实例 ready`。
- `Server` badge 或单独的“未绑定”状态 chip。
- 最近运行标题。
- 启动、停止、播放、更多菜单等运行控制按钮。

## 5. 预算治理

### 5.1 配置模型

数字员工配置版本新增独立配置块：

```json
{
  "budget_policy": {
    "daily_token_limit": 10000
  }
}
```

规则：

- `daily_token_limit` 为正整数。
- `null` 或未填写表示无预算上限。
- `budget_policy` 参与数字员工配置版本、生效配置 preview、approve 和 effective config 快照。
- `budget_policy` 不写入 `digital_employees.metadata`，也不混入 `approval_policy_override`。

创建数字员工时，创建向导在“治理”步骤增加“每日 Token 预算上限”输入。配置页增加明确的“预算策略”区域，用于修改 `daily_token_limit` 并保存新的配置版本。

### 5.2 Overview 预算摘要

`budget_summary` 扩展为：

```json
{
  "daily_token_limit": 10000,
  "usage_tokens_today": 6200,
  "usage_percent_today": 62,
  "limit_exceeded": false,
  "usage_tokens_30d": 16000,
  "run_count_30d": 12,
  "cost_amount_30d": null,
  "currency": "USD",
  "source": "run_usage_projection"
}
```

规则：

- 有每日上限时，前端显示今日用量进度条。
- 无每日上限时，前端显示“无预算上限”，不画百分比进度条。
- `usage_tokens_30d` 只能作为辅助统计，不能冒充预算进度。
- token 用量从 `task_runs.result.usage.total_tokens`、`task_runs.result.total_tokens` 等现有运行结果字段聚合；没有可靠来源时返回空值或 `source=unavailable`。

### 5.3 执行前拦截

`DigitalEmployeeRunService.CreateRun` 的 preflight 增加预算校验：

1. 读取该数字员工已批准的 effective config 中的 `budget_policy.daily_token_limit`。
2. 若无上限，跳过预算拦截。
3. 若有上限，按 `Asia/Shanghai` 当日 00:00 到次日 00:00 统计该员工今日 token 用量。
4. 当今日用量大于或等于上限时，拒绝创建 run。
5. 错误信息必须明确表达预算原因，例如 `employee daily token budget exceeded`。

预算超限不是 Runtime 异常，也不是 Provider 异常。前端详情页或运行入口接到该错误时，应显示“今日 Token 预算已达上限”，避免误导用户排查 Runtime。

## 6. 前端页面

`/employees` 页面结构：

1. 页头：保留标题“数字员工”、说明“业务身份、唯一执行实例和运行状态”、主按钮“创建数字员工”。
2. 顶部队列指标：展示“就绪 / 待绑定 / 异常 / 配置待审批 / 运行失败”。
3. 筛选区：保留搜索、状态、团队、Provider、风险、员工类型、最近运行筛选；不展示名册切换按钮。
4. 主体：桌面为左侧三列卡片工作台 + 右侧待处理队列/选中员工面板；移动端右侧面板下沉到卡片列表之后。
5. 卡片：按第 4.3 节字段规则渲染。
6. 右侧面板：默认选中第一张卡片；点击卡片后本地切换选中项，不额外请求。

右侧面板包括：

- 待处理队列：`待绑定 Runtime`、`配置过期`、`最近运行失败`。
- 选中员工摘要：头像、名称、状态、Runtime/Provider 绑定。
- 最近事件：最多 3 条轻量事件；为空时显示“暂无最近事件”。
- 次要动作：`查看审计`。

加载、错误和空态沿用现有页面规则：

- overview loading：显示页面内加载态。
- overview error：显示错误说明和重试按钮。
- 无数据：显示“暂无数字员工”和创建入口。
- 筛选后无结果：显示“没有符合条件的数字员工”。

## 7. 数据流

- 页面只调用一次 `getDigitalEmployeeOverview(filters)` 渲染工作台。
- 筛选变更更新 query key 并重新请求 overview。
- 默认选中 `items[0]`。
- 当当前选中员工不在新结果中时，切换到新结果的第一项；无结果时清空选中。
- `详情` 跳转 `/employees/$employeeId`。
- `配置` 跳转 `/employees/$employeeId/config`。

## 8. 数据库与迁移

需要新增 forward migration，不修改既有 shared initial migration。

建议范围：

- `digital_employee_config_revisions` 增加 `budget_policy JSONB NOT NULL DEFAULT '{}'::jsonb`。
- `budget_policy` 必须纳入 `digital_employee_effective_configs.effective_config_snapshot`。本期不为 effective config 额外增加独立预算列，预检和 overview 从已批准快照中读取 `budget_policy`。
- 为每日预算聚合补充必要索引。现有 `idx_task_runs_budget_30d` 可作为参考，但每日聚合应按 `tenant_id`、`digital_employee_id`、`finished_at/updated_at/created_at` 的实际查询口径确认。

迁移必须遵循 `DATABASE_DESIGN.md` 的 UUID-first、forward migration 和 sqlc 更新规则。

## 9. 测试与验证

Control Plane：

- overview route contract 覆盖新增 `queue_summary`、`workbench_status`、`recent_events` 和扩展后的 `budget_summary`。
- 创建数字员工测试覆盖 `budget_policy.daily_token_limit` 透传和正整数校验。
- 配置版本测试覆盖 `budget_policy` 保存。
- effective config preview/approve 测试覆盖 `budget_policy` 进入快照。
- Asia/Shanghai 当日 token 聚合测试覆盖跨 UTC 日期边界。
- run preflight 测试覆盖无上限不拦截、未达到上限不拦截、达到上限拒绝创建 run。

Web：

- `/employees` 渲染工作台卡片。
- 状态映射只出现“就绪 / 待绑定 / 异常”。
- Runtime 行只显示 Runtime Agent 与 Provider。
- 最近运行只显示成功/失败和时间。
- 待处理队列和选中员工事件面板渲染正确。
- 有每日预算上限时显示进度；无上限时显示“无预算上限”。
- 创建页预算输入校验。
- 配置页预算保存。

建议验证命令：

```bash
go test ./apps/control-plane/internal/...
pnpm --filter @superteam/web test -- employees
pnpm --filter @superteam/web typecheck
git diff --check
```

涉及可见页面改造后，需要在浏览器打开 `/employees` 做桌面和移动端视觉检查，确认无横向溢出、文字重叠和按钮误导。

## 10. 与既有设计的关系

本设计是对 `docs/superpowers/specs/2026-06-06-digital-employee-workbench-overview-design.md` 的增量修订。既有 overview read model 和聚合方向保留；列表表现从表格改为执行工作台卡片；预算从 30 天统计展示升级为可配置、可审批、可拦截的每日治理边界。
