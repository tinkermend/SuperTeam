# 数字员工工作台列表页与 Overview 接口设计

日期：2026-06-06
状态：已确认，待实现计划

## 1. 背景

SuperTeam 已有数字员工创建向导、数字员工业务身份、唯一执行实例、Runtime/Provider 绑定、run 发起与事件回写能力。当前 `/employees` 页面仍是最小卡片列表：前端先拉员工列表，再逐行请求执行实例，无法高效展示总览指标、Runtime 绑定状态、最近运行、治理摘要、预算摘要和筛选结果。

本轮目标是把 `/employees` 重新设计成数字员工工作台列表页。页面参考提供的浅色控制台截图的信息结构：左侧导航保持现有 Shell，主内容区以指标卡、筛选工具条和高密度表格呈现数字员工可执行状态。创建数字员工相关功能已经实现，本轮不改创建页。

用户进一步确认：详情页功能会很重，后续需要覆盖 Dashboard 说明、Instructions、Skills、Configuration、Runs、Budget 等视图。因此本轮不重做 `/employees/$employeeId` 详情页，只为后续详情页预留可复用的 summary DTO 和聚合接口边界，避免重复建设。

## 2. 范围

本轮覆盖：

- 新增 `GET /api/v1/digital-employees/overview`，作为数字员工工作台 read model。
- `/employees` 使用 overview 接口渲染指标、筛选、表格和操作入口。
- 保留 `/employees/new` 创建入口，但不修改创建向导。
- 保留 `/employees/$employeeId` 详情入口，但不实现新的详情 tabs。
- 保留现有 `GET /api/v1/digital-employees`、`GET /execution-instance`、runs API，避免破坏团队页和当前详情页。

本轮不覆盖：

- 不修改数字员工创建流程。
- 不实现详情页的 Dashboard、Instructions、Skills、Configuration、Runs、Budget tabs。
- 不读取 Runtime 本地落盘文件内容。
- 不实现预算流水、成本中心或完整 token 成本模型。
- 不做配置编辑器、宪法编辑器、MCP/skills 绑定编辑器。
- 不实现 Runtime 自动选择、fallback、迁移或跨 Runtime 调度。

## 3. 方案选择

采用方案 A：员工工作台列表 + 可复用摘要接口。

新增 `GET /api/v1/digital-employees/overview`，一次返回列表页需要的 summary、items、filters 和 pagination。每个 item 由稳定 summary DTO 组成：

- `identity_summary`
- `execution_summary`
- `latest_run_summary`
- `governance_summary`
- `budget_summary`

这样列表页不需要逐行拼接 Runtime、run、governance 和 budget 状态。后续详情页可以复用相同 summary DTO，再追加深层接口读取 instructions 文件内容、skills/MCP 明细、run transcript、budget ledger 等详情数据。

不采用在现有 `GET /digital-employees` 上继续膨胀字段的方式，因为基础列表和工作台 read model 的职责不同。也不采用前端聚合现有接口，因为会制造 N+1 请求和复杂错误状态。

## 4. 后端 API

### 4.1 请求

新增：

```http
GET /api/v1/digital-employees/overview
```

Query 参数：

- `q`：搜索员工名称、角色、描述。
- `team_id`：按团队过滤。
- `status`：按数字员工状态过滤。
- `employee_type`：按员工类型过滤。
- `provider_type`：按 Provider 过滤。
- `runtime_node_id`：按 Runtime node 过滤。
- `risk_level`：按风险等级过滤。
- `execution_status`：按执行实例状态过滤，允许值为执行实例状态加 overview 专用值 `missing`。
- `run_status`：按最近运行状态过滤，允许值为 run 状态加 overview 专用值 `none`。
- `limit`：默认 50，最大 100。
- `offset`：默认 0。

授权：

- 复用 `employee.read`。
- 列表资源仍以 tenant 为读取资源。
- 带 `team_id` 时只作为数据过滤条件，不把团队页逻辑搬进员工模块。

### 4.2 响应

响应结构：

```json
{
  "summary": {
    "total_count": 18,
    "runnable_count": 14,
    "running_count": 5,
    "waiting_runtime_count": 2,
    "error_count": 1,
    "high_risk_count": 3
  },
  "items": [],
  "filters": {},
  "pagination": {
    "limit": 50,
    "offset": 0,
    "total_count": 18
  }
}
```

`items[]` 中每行包含：

```json
{
  "identity_summary": {
    "id": "uuid",
    "name": "需求分析员",
    "description": "需求澄清、验收标准、上下文切片",
    "team_id": "uuid",
    "team_name": "产品组",
    "owner_user_id": "uuid",
    "owner_display_name": "王佩",
    "employee_type": "requirements_analyst",
    "employee_type_label": "需求分析",
    "role": "requirements_analyst",
    "status": "active",
    "risk_level": "medium"
  },
  "execution_summary": {
    "execution_instance_id": "uuid",
    "status": "ready",
    "runtime_node_id": "uuid",
    "node_id": "runtime-cn-01",
    "runtime_name": "cn-01",
    "runtime_status": "online",
    "provider_type": "codex",
    "provider_status": "healthy",
    "health_status": "healthy",
    "agent_home_dir_available": true
  },
  "latest_run_summary": {
    "run_id": "uuid",
    "task_id": "uuid",
    "status": "completed",
    "title": "审查需求",
    "started_at": "2026-06-06T10:00:00Z",
    "finished_at": "2026-06-06T10:04:00Z",
    "updated_at": "2026-06-06T10:04:00Z",
    "duration_sec": 240,
    "token_usage": 1600,
    "error_message": ""
  },
  "governance_summary": {
    "effective_config_id": "uuid",
    "status": "approved",
    "team_revision_number": 3,
    "employee_revision_number": 1,
    "skills_count": 8,
    "mcp_servers_count": 3,
    "constitution_ref": "effective-config://uuid/constitution"
  },
  "budget_summary": {
    "usage_tokens_30d": 16000,
    "run_count_30d": 12,
    "cost_amount_30d": null,
    "currency": "USD",
    "source": "run_usage_projection"
  }
}
```

字段规则：

- 没有执行实例时 `execution_summary.status` 返回 `missing`，Runtime/Provider 字段允许为空。
- 没有最近运行时 `latest_run_summary` 返回 `null`。
- 没有已批准生效配置时 `governance_summary.status` 返回 `missing`。
- 预算数据没有真实成本来源时 `budget_summary.source` 返回 `unavailable`，数值字段返回 `null` 或 0，不能伪造成本。
- `filters` 返回当前租户下可用的团队、员工类型、状态、Provider、Runtime、风险等级和执行状态候选，供前端渲染筛选器。

`filters` 结构使用统一候选项，避免前端写死显示名：

```json
{
  "filters": {
    "teams": [{ "value": "uuid", "label": "产品组" }],
    "employee_types": [{ "value": "requirements_analyst", "label": "需求分析" }],
    "statuses": [{ "value": "active", "label": "活跃中" }],
    "providers": [{ "value": "codex", "label": "Codex" }],
    "runtime_nodes": [{ "value": "uuid", "label": "runtime-cn-01" }],
    "risk_levels": [{ "value": "medium", "label": "中风险" }],
    "execution_statuses": [{ "value": "missing", "label": "未绑定 Runtime" }],
    "run_statuses": [{ "value": "none", "label": "暂无运行" }]
  }
}
```

`missing` 和 `none` 只属于 overview read model，不写回核心业务状态表。

### 4.3 数据流

后端由 employee 模块实现 overview service 和 repository query。主要聚合应在数据库层完成，避免前端或服务层按行循环查询。

建议查询策略：

- `digital_employees` 为主表。
- 左连接 `tenant_teams` 取团队名。
- 左连接 `auth_users` 取 owner 展示名。
- 左连接 `digital_employee_execution_instances` 取唯一执行实例。
- 左连接 `runtime_nodes` 和最新 provider capability 取 Runtime/Provider 状态。
- lateral join 或 `DISTINCT ON (digital_employee_id)` 取最近 run。
- 左连接当前 approved effective config 和当前员工配置修订，计算 governance summary。
- 预算首版从最近 30 天 run 的 usage/result/metadata 中做轻量投影；没有可靠字段时返回 unavailable。

排序默认按员工 `created_at DESC`。如果后续需要按最近运行或异常优先排序，应单独扩展 query 参数，不在本轮隐式改变。

## 5. 前端页面

`/employees` 页面改为工作台布局：

1. 顶部标题区
   标题为“数字员工”，说明为“业务身份、执行实例、运行状态与治理摘要”。右侧保留“创建数字员工”按钮，跳转到现有 `/employees/new`。

2. 指标区
   使用 `MetricCard` 和 `SemanticIconTile`，展示：
   - 数字员工总数
   - 可执行员工
   - 运行中任务
   - Runtime 待绑定
   - 异常或高风险

3. 工具条
   搜索框和筛选器走后端 query 参数。筛选包括状态、团队、员工类型、Provider、Runtime、风险等级、执行状态。筛选变更后重置 offset。

4. 主表格
   使用现有 `Table` 组件，保留横向滚动。列包括：
   - 数字员工名称、描述
   - 所属团队
   - 员工类型 / 角色
   - 执行端：Runtime node + Provider
   - 当前状态：员工状态 + 执行实例状态
   - 风险
   - 最近运行
   - 治理摘要
   - 预算摘要
   - 操作

5. 操作列
   - `详情`：跳转 `/employees/$employeeId`。
   - `任务`：本轮仍跳详情页，后续详情页实现 Runs tab 后可改为带 `tab=runs`。
   - `配置`：本轮仍跳详情页，后续详情页实现 Configuration tab 后可改为带 `tab=configuration`。

6. 状态和空态
   - loading 显示骨架或加载文本。
   - error 显示失败说明和重试按钮。
   - empty 显示“暂无数字员工”和创建入口。
   - 没有执行实例时行内显示“未绑定 Runtime”，并用 warning 状态。

视觉遵循 `DESIGN.md` 的浅色液态玻璃控制台风格。不要把详情页截图的暗色 Paperclip 风格直接搬到 SuperTeam；截图只作为详情页信息架构参考。

## 6. 后续详情页复用边界

本轮 summary DTO 为后续详情页服务，但不提前实现详情页。

后续详情页可复用：

- `identity_summary`：详情页 Dashboard 顶部身份区。
- `execution_summary`：Dashboard 和 Configuration 中的 Runtime/Provider 绑定摘要。
- `latest_run_summary`：Dashboard 与 Runs tab 的最近运行入口。
- `governance_summary`：Instructions、Skills、Configuration 的摘要入口。
- `budget_summary`：Budget tab 顶部指标。

后续详情页需要新增的深层接口：

- `GET /api/v1/digital-employees/{employeeId}/dashboard`
- `GET /api/v1/digital-employees/{employeeId}/instructions`
- `GET /api/v1/digital-employees/{employeeId}/skills`
- `GET /api/v1/digital-employees/{employeeId}/configuration`
- `GET /api/v1/digital-employees/{employeeId}/budget`
- runs 详情继续复用现有 runs/events API，并按需要扩展 transcript/session 读取。

这些接口的事实源仍应是 Control Plane 持久化对象。Runtime 落盘状态只作为诊断来源，不作为业务事实入口。

## 7. 错误处理

后端：

- 无效 UUID、无效枚举、负数分页返回 400。
- 未授权返回 403 或现有授权错误格式。
- repository 错误必须脱敏。
- 单个员工缺失执行实例、最近运行或预算数据不应导致整个 overview 失败。

前端：

- overview 请求失败时显示整页错误和重试。
- 单行缺少执行实例显示 warning，不视为页面失败。
- 预算不可用显示“暂无预算数据”，不显示 0 元成本来暗示真实统计。
- 筛选无结果显示可清除筛选的空状态。

## 8. 测试计划

Go：

- route test 覆盖 `GET /api/v1/digital-employees/overview` 鉴权、query 参数映射、响应 JSON、错误脱敏。
- service test 覆盖默认分页、筛选校验、summary 计数、预算 unavailable 语义。
- repository test 覆盖有执行实例、无执行实例、最近运行失败、有效配置缺失、Provider unhealthy。
- OpenAPI 生成检查保持通过。

Web：

- `EmployeesView` 改为使用 overview 接口。
- 测试指标卡、筛选、表格行、详情/任务/配置跳转。
- 测试 loading、empty、error、筛选触发重新请求。
- 保留创建页测试，不改创建向导。

建议回归命令：

```bash
go test ./apps/control-plane/internal/...
pnpm --filter @superteam/web test -- employees
pnpm --filter @superteam/web typecheck
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config apps/control-plane/internal/api/oapi-codegen.yaml contracts/control-plane/openapi.yaml
git diff --check
```

如本地依赖缺失，至少执行可用的窄测试、OpenAPI 静态校验和 `git diff --check`，并在交付时说明未执行项。

## 9. 交付边界

实现完成后需要更新 `CHANGELOG.md`，时间使用本地 `Asia/Shanghai`，格式为 `YYYY-MM-DD HH:mm`。

提交应只包含本轮相关文件：

- Control Plane overview API、service、repository、OpenAPI 和测试。
- Web `/employees` 工作台页面、API client 类型和测试。
- `CHANGELOG.md`。

不得把当前工作区中与本轮无关的既有改动带入提交。
