# Runtime Agent 总览页前后端功能设计

日期：2026-06-05

状态：待评审

## 1. 背景

SuperTeam 已完成 Runtime Agent 接入、短期 Runtime Session、能力上报、Runtime WebSocket 命令通道、数字员工执行实例和 Provider command writeback 的基础链路。当前 Web 控制台 `/runtime` 页面仍停留在较窄的 MVP 状态：能查看待接入 enrollment、批准接入请求和查看已接入节点列表，但还不能作为 Runtime 管理面的总览工作台。

本设计目标是在不展开单个 Runtime Agent 详情页、不实现接入诊断包的前提下，补齐 Runtime Agent 总览页的一期前后端闭环。页面需要让平台管理员快速看到 Runtime 节点在线状态、接入审批、Provider 能力覆盖、阻断或异常事件，并能完成待接入节点的批准或拒绝。

视觉方向沿用 `DESIGN.md` 的浅色液态玻璃企业控制台风格，并参考本次需求截图的信息密度、卡片节奏、Tab 组织和状态表达。页面内容必须来自真实接口、真实落库事实或明确的一期派生数据，不搬运截图里的示例业务数据。

## 2. 目标

- 将 `/runtime` 从简单列表升级为 Runtime 节点总览工作台。
- 新增 Control Plane Console 聚合接口，避免 Web 页面自行拼接多个 Runtime 数据源。
- 新增 `runtime_events` 统一事件流，服务 Runtime 总览和事件审计。
- 支持待接入 Runtime enrollment 的批准和拒绝。
- 展示已接入 Runtime 节点的状态、负载、Provider、范围和最近心跳。
- 展示 Provider 能力矩阵，覆盖 provider 类型、可用状态、健康状态、节点覆盖和最近上报时间。
- 新增“事件审计”Tab，展示 Runtime 管理面事件列表和筛选。
- 保持 Runtime Agent、Provider、Control Plane、Web 的既有边界，Runtime Agent 只上报事实，不承担控制台聚合逻辑。

## 3. 非目标

- 不做单个 Runtime Agent 详情页面。
- 不做 Runtime Agent 详情抽屉。
- 不做接入诊断包生成、下载或审计。
- 不做创建接入密钥功能。
- 不做 Runtime 本地诊断探测。
- 不做 Provider 详情页。
- 不做 Runtime 能力策略编辑或自动调度策略管理。
- 不新增 OpenFGA wiring，继续复用现有 `Authorizer.Check` 边界。
- 不让 Web 直接访问 Runtime Agent 或 Provider。

## 4. 方案选择

### 4.1 方案 A：总览聚合接口 + 独立 `runtime_events` 表

后端新增 Runtime 总览聚合能力，返回指标、待接入节点、已接入节点、Provider 能力矩阵和最近事件。同时新增 `runtime_events` 统一事件流，接入审批、拒绝、撤销、节点状态、capability 上报和 command writeback 关键事实都写入这里。

优点：

- Web 页面只消费稳定聚合接口，状态和错误处理更简单。
- 事件审计 Tab 有明确事实源。
- Runtime 管理面事件不需要临时从多个表拼接。
- 后续 Runtime 详情页可以复用同一事件事实。

缺点：

- 需要 forward migration、sqlc 查询、OpenAPI 契约和服务层写事件。

结论：采用。

### 4.2 方案 B：前端组合多个现有接口 + 单独补事件表

后端只补最少缺口，前端分别拉节点、接入、capability 和事件再组合。

优点：

- 后端聚合接口改动较少。

缺点：

- 当前 capability Console GET route 存在契约和实际路由不一致。
- Web 页面需要承担业务聚合逻辑。
- 多请求下的加载、空态、权限失败和局部错误更复杂。

结论：不采用。

### 4.3 方案 C：Runtime 运维中心大版本

同时实现总览、事件审计、节点详情、接入密钥、诊断包和能力策略管理。

优点：

- 功能更完整。

缺点：

- 明显超过本次任务范围，会提前吞并 Runtime Agent 详情页和诊断包任务。

结论：不采用。

## 5. 架构边界

固定通信链路：

```text
Web
  <-> Control Plane
    <-> Runtime Agent
      <-> Provider
```

边界规则：

- Web 只通过 Control Plane 读取 Runtime 管理面数据。
- Runtime Agent 继续通过 Runtime session、heartbeat、capability report 和 command writeback 上报事实。
- Control Plane 负责 Runtime 总览聚合、事件持久化、接入审批、权限判断和审计。
- Provider 仍由 Runtime Agent 管理，不直接连接 Web 或 Control Plane。
- Runtime 事件流是 Control Plane 的管理面事实，不是 Runtime Agent 的本地日志文件。

后端落点：

- `apps/control-plane/internal/runtime`：新增 overview、event 写入和 event 查询服务。
- `apps/control-plane/internal/storage/queries`：新增 `runtime_events` 写入、列表、聚合和 capability 读取查询。
- `apps/control-plane/internal/api/handlers/runtime.go`：新增 Console 侧 overview/events/capabilities GET handler，补齐 reject 路由的 Web 使用路径。
- `contracts/control-plane/openapi.yaml`：新增 overview/events schema，并修正 capability Console GET 契约与实现一致性。

前端落点：

- `apps/web/src/lib/api/runtime.ts`：新增 overview、events、reject enrollment 和 capability 读取 client。
- `apps/web/src/features/runtime/index.tsx`：升级 Runtime 页面为总览工作台。
- 必要时在 `apps/web/src/features/runtime/` 下拆分页面内组件，但不新增详情路由。

## 6. 数据模型

### 6.1 `runtime_events`

新增 forward migration 创建 `runtime_events` 表。建议字段：

```text
id UUID PRIMARY KEY DEFAULT gen_random_uuid()
tenant_id UUID NOT NULL
runtime_node_id UUID
node_id VARCHAR(255)
event_type VARCHAR(100) NOT NULL
severity VARCHAR(32) NOT NULL
source VARCHAR(100) NOT NULL
title VARCHAR(255) NOT NULL
description TEXT
provider_type VARCHAR(100)
correlation_type VARCHAR(100)
correlation_id VARCHAR(255)
payload JSONB NOT NULL DEFAULT '{}'::jsonb
created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
```

索引：

- `(tenant_id, created_at DESC)` 支撑事件审计默认列表。
- `(tenant_id, runtime_node_id, created_at DESC)` 支撑后续节点详情页复用。
- `(tenant_id, event_type, created_at DESC)` 支撑类型筛选。
- `(tenant_id, severity, created_at DESC)` 支撑严重级别筛选。
- `(tenant_id, provider_type, created_at DESC)` 支撑 Provider 筛选。
- `(tenant_id, correlation_type, correlation_id)` 支撑回溯原始事实。

`event_type` 第一期服务端允许值：

- `enrollment_requested`
- `enrollment_approved`
- `enrollment_rejected`
- `enrollment_revoked`
- `node_online`
- `node_offline`
- `capability_reported`
- `capability_degraded`
- `command_event`
- `command_completed`
- `command_failed`
- `command_cancelled`
- `command_timed_out`

`severity` 第一期允许值：

- `info`
- `success`
- `warning`
- `error`

`source` 第一期允许值：

- `runtime_enrollment`
- `runtime_node`
- `runtime_capability`
- `runtime_command`
- `provider_session`

### 6.2 写入原则

`runtime_events` 不替代原始事实表，只保存总览需要快速读取的归一化事件摘要。原始事实仍在 `runtime_enrollments`、`runtime_nodes`、`runtime_capabilities`、`runtime_command_receipts`、`provider_sessions`、`provider_session_events` 和 `audit_events` 中。

写入点：

- Runtime hello 创建或刷新 pending enrollment 时写 `enrollment_requested`。
- 批准接入写 `enrollment_approved`。
- 拒绝接入写 `enrollment_rejected`。
- 撤销接入写 `enrollment_revoked`。
- heartbeat 只在状态变化或离线检测时写 `node_online` / `node_offline`，避免每次心跳刷屏。
- capability 上报按 provider health 变化或降级写 `capability_reported` / `capability_degraded`。
- command writeback 关键事件写 `command_event`，终态写 `command_completed`、`command_failed`、`command_cancelled` 或 `command_timed_out`。

历史数据：

- 第一期不回填历史事件。
- 页面上线后展示新产生的 `runtime_events`。
- overview 指标仍可从现有表实时聚合，不依赖历史事件完整性。

一致性规则：

- 主事实写入成功但 `runtime_events` 写入失败时，默认不回滚主流程，并记录服务端日志。
- 接入批准和拒绝的事件应尽量与审批事实同事务写入；如果实现成本过高，允许先采用 best-effort，但必须测试主事实成功且事件失败不破坏审批流程。

## 7. API 设计

### 7.1 Runtime Overview

新增：

```http
GET /api/v1/runtime/overview
```

认证与授权：

- Web session required。
- `Authorizer.Check(actor=user, action=runtime_scope.manage, resource=tenant)`。

响应结构：

```json
{
  "summary": {
    "online_nodes": 6,
    "total_nodes": 8,
    "pending_enrollments": 2,
    "active_provider_sessions": 14,
    "blocked_events": 1
  },
  "pending_enrollments": [],
  "nodes": [],
  "provider_capabilities": [],
  "recent_events": []
}
```

字段说明：

- `summary.online_nodes`：服务端按节点状态和心跳阈值派生。
- `summary.total_nodes`：未归档 Runtime 节点数。
- `summary.pending_enrollments`：pending enrollment 数。
- `summary.active_provider_sessions`：active/running Provider session 数。
- `summary.blocked_events`：最近 24 小时内 `severity=error` 或阻断类 command/capability 事件数。
- `pending_enrollments`：总览首页最多返回最近 5 条 pending enrollment。
- `nodes`：总览首页最多返回最近 50 个未归档节点，包含状态、负载、provider、范围摘要、最近心跳。
- `provider_capabilities`：按 provider 类型聚合能力矩阵。
- `recent_events`：最近 10 条 Runtime 管理面事件，读取 `runtime_events`。

### 7.2 Runtime Events

新增：

```http
GET /api/v1/runtime/events?limit&offset&event_type&severity&node_id&provider_type
```

认证与授权：

- Web session required。
- `Authorizer.Check(actor=user, action=runtime_scope.manage, resource=tenant)`。

响应结构：

```json
{
  "items": [],
  "limit": 50,
  "offset": 0
}
```

筛选规则：

- `limit` 默认 50，最大 100。
- `offset` 默认 0。
- `event_type`、`severity`、`node_id`、`provider_type` 都是可选筛选。
- 不返回原始 token、bootstrap key、环境密钥、provider 原始敏感 payload。

### 7.3 Enrollment Actions

复用现有 canonical route：

```http
POST /api/v1/runtime/enrollments/{enrollmentId}/approve
POST /api/v1/runtime/enrollments/{enrollmentId}/reject
```

`reject` 请求体：

```json
{
  "reason": "节点来源不符合接入要求"
}
```

批准和拒绝成功后：

- 服务层写 `runtime_events`。
- Web invalidate overview/events/enrollments/nodes query。
- 页面不进入节点详情。

### 7.4 Runtime Capabilities

修正并补齐 Console 侧读取：

```http
GET /api/v1/runtime/nodes/{nodeId}/capabilities
```

用途：

- 保持 OpenAPI 与实际 route/handler 一致。
- 支撑后续详情页复用。
- 总览页优先使用 `/runtime/overview` 返回的聚合结果，不按节点 N+1 调用该接口。

## 8. 前端页面结构

### 8.1 顶部区域

页面仍位于 `/runtime`。

标题：

```text
Runtime 节点
```

副标题：

```text
接入审批、在线状态、Provider 能力和事件审计
```

顶部操作：

- 保留“刷新状态”。
- 不显示“下载诊断包”。
- 不显示“创建接入密钥”。

### 8.2 Tab

四个 Tab：

- `节点总览`
- `接入审批`
- `能力范围`
- `事件审计`

使用 `LiquidTabsList` 和 `LiquidTabsTrigger`，保持现有液态玻璃控制台风格。

### 8.3 节点总览

首屏四个指标卡：

- 在线节点：`online_nodes / total_nodes`。
- 待接入审批：pending enrollment 数。
- 活跃 Provider 会话：running/active provider session 数。
- 阻断/异常事件：最近 24 小时内 warning/error 或阻断事件数。

下方布局：

- 左侧：待接入节点卡片，展示节点 ID、申请时间、最近 hello、申请 payload 摘要、标签或环境信息；支持批准和拒绝。
- 中间：已接入节点列表，展示节点名称、状态、负载、Provider、范围摘要、最近心跳；本期不可点击进入详情。
- 右侧：Provider 能力矩阵和最近 Runtime 事件。

### 8.4 接入审批

展示 enrollment 列表，支持状态筛选：

- pending
- approved
- rejected
- revoked

本期只允许对 pending 做批准和拒绝。非 pending 记录只读展示，不提供撤销或恢复入口。

批准：

- 使用确认 Dialog 防止误点。
- 成功后刷新 overview/events/enrollments/nodes。

拒绝：

- 使用确认 Dialog 收集拒绝原因。
- 原因不能为空。
- 成功后刷新 overview/events/enrollments/nodes。

### 8.5 能力范围

展示按 Provider 聚合的能力状态：

- provider 类型。
- 覆盖节点数。
- 活跃 session 数。
- 代码编辑、工具调用、文件读写、终端执行等能力状态。
- 最近上报时间。
- 健康状态。

本期只读展示，不做能力策略编辑。

### 8.6 事件审计

展示 `runtime_events` 列表和筛选：

- 时间。
- 严重级别。
- 事件类型。
- Runtime 节点。
- Provider。
- 标题。
- 描述。
- 来源。
- 关联 ID。

本期不做事件详情页、不做诊断包生成、不做下载。

## 9. UI 与交互规则

- 复用 `MetricCard`、`LiquidCard`、`LiquidTabsList`、`LiquidTabsTrigger`、`SemanticIconTile`、`StatusBadge`。
- 使用 `lucide-react` 图标表达刷新、节点、审批、Provider、事件和异常。
- 状态色遵循 `DESIGN.md`：Runtime/信息用青蓝，在线/成功用绿色，等待/预警用琥珀，阻断/失败用红色，审计/历史用灰蓝。
- 所有按钮文字必须能在中文环境下稳定显示，不因加载、禁用或长文字造成布局跳动。
- 表格和列表保持后台工具信息密度，不做营销式大留白。
- 空态必须区分“暂无数据”和“筛选后无结果”。
- 加载态保留页面结构，避免首屏整体闪烁。

## 10. 错误处理

- overview 加载失败时保留页面壳，显示错误提示和重试按钮。
- events 加载失败时只影响事件审计区域，不影响节点总览已加载数据。
- 批准或拒绝失败时展示后端错误原因，不做乐观更新。
- 授权失败统一展示权限不足提示，不泄露 Runtime 节点敏感信息。
- 服务端派生在线/离线和异常状态，前端不自行判断业务状态。
- `runtime_events` 写入失败不应破坏 Runtime hello、审批、capability 上报或 command writeback 主流程。

## 11. 权限与安全

- Console 读 overview/events/capabilities 和写批准/拒绝都要求 Web session。
- Console Runtime 管理能力继续使用 `runtime_scope.manage` 授权动作。
- Runtime session 只能上报 heartbeat、capability、command writeback 和 WebSocket 命令通道，不允许读取 Console 聚合视图。
- `runtime_events.payload` 写入前必须避免保存 bootstrap key、runtime session token、Provider 原始密钥、环境变量密钥和用户敏感输入。
- Web 只展示服务端返回的脱敏字段。

## 12. 测试计划

后端：

- migration/sqlc：`runtime_events` 表、索引、写入、筛选、分页。
- runtime service：overview 聚合指标、provider capability 聚合、recent events 查询。
- event writer：接入请求、批准、拒绝、撤销、capability 降级、command 终态事件。
- route：overview/events/capabilities GET、approve/reject 授权和响应。
- side effect：主事实成功但 event 写入失败时主流程不被破坏。
- OpenAPI：新增 schema 与 route 纳入契约守卫。

前端：

- API client：overview、events、reject enrollment、capability GET 的 path、method、query、body。
- Runtime 页面：指标卡、待接入节点、已接入节点、Provider 能力矩阵、最近事件渲染。
- 接入审批：批准确认、拒绝原因、成功刷新、失败提示。
- 事件审计：筛选、空态、加载失败。
- 样式回归：中文按钮和 Tab 不溢出，列表在桌面宽度下保持稳定。

验收门禁：

- `pnpm verify:contracts`
- 相关 Go tests。
- Web 测试或 `pnpm verify:web`。
- 如实现涉及真实 DB migration，必须检查 live schema 或专用测试库实际表结构。
- 实现完成后更新 `CHANGELOG.md`，新增条目使用 `Asia/Shanghai` 时间，格式为 `YYYY-MM-DD HH:mm`。

## 13. 交付边界

本任务完成后应具备：

- `/runtime` 可作为 Runtime Agent 总览页使用。
- 管理员能刷新状态、查看指标、查看节点、查看 Provider 能力、查看 Runtime 事件。
- 管理员能批准或拒绝 pending Runtime 接入请求。
- 事件审计 Tab 展示统一 `runtime_events`，但不提供诊断包功能。
- 单个 Runtime Agent 详情页仍作为后续独立任务拆分。

本任务不以截图像素级复刻为目标，而以当前 SuperTeam 设计系统和真实 Runtime 管理事实为准。
