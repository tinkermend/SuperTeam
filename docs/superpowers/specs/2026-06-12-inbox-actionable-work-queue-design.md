# 2026-06-12-inbox-actionable-work-queue-design

## 1. 设计目标

收件箱是面向当前人类用户的可操作工作队列，用来回答“现在有哪些事项需要我处理”。它不是通知中心，不承载只读消息流，也不替代审批中心、项目详情或任务中心。

本设计第一版要实现：

- 左侧菜单新增 `收件箱`，放在 `工作台` 后、`任务中心` 前。
- 默认展示 `我的待办`，同时为有权限用户提供 `团队待办` 视图。
- 只展示可操作事项，不展示普通通知、审计事件或项目事件流。
- 通过 `inbox_items` read model 聚合待办事项。
- 第一版只接入全局审批和项目人类决策来源。
- 支持在收件箱内做轻量处理，但处理事实仍写回来源模块。
- 不支持已读、未读、手动归档或稍后处理。

## 2. 当前上下文

已有事实源和页面基础：

- `approval_requests` 是全局审批请求事实表。
- `approval_decisions` 保存人类审批处理记录。
- `project_decision_requests` 是项目侧人类决策查询投影，事实源指向 `approval_request_id`。
- 项目详情页已经能展示项目内人类决策卡片并处理 pending 决策。
- `/approvals` 已有左侧菜单和占位页，但尚未实现全局审批中心列表。
- `/tasks` 仍是占位页，不应承载审批、验收、异常介入等跨来源事项。

既有项目管理设计已经确定：

- 项目侧保存上下文、事件关联和跳转。
- 全局 `approval` 模块保存审批事实。
- 审批处理结果必须回到项目事件流。

收件箱在这个架构中只做可操作事项的用户工作队列，不创建平行审批模型。

## 3. 范围边界

### 3.1 包含

后端：

- 新增 `inbox` 模块。
- 新增 `inbox_items` 表作为跨模块 read model。
- 新增 Inbox 查询、badge 和动作 API。
- 审批和项目决策创建、处理时同步 Inbox 投影。
- 支持 `mine` 和 `team` 两种视图。
- 团队视图第一版只读，不允许代处理。

前端：

- 新增 `/inbox` 路由。
- 新增左侧菜单 `收件箱` 和个人待处理 badge。
- 默认展示 `我的待办`。
- 提供 `团队待办` Tab。
- 支持类型、风险、项目和目标处理人筛选。
- 支持来源上下文跳转。
- 支持轻量内联处理动作。

### 3.2 不包含

第一版不做：

- 普通通知中心。
- 只读通知、系统消息、项目事件流、审计事件流。
- 已读、未读、手动归档、稍后处理。
- 管理员代处理他人的待办。
- 复杂自定义表单，例如验收表单、预算额度调整表单或转派候选人选择。
- Runtime 异常、预算拦截、验收和补证来源的完整接入。
- 外部消息渠道触达，例如飞书、钉钉、邮件。

## 4. 产品决策

### 4.1 信息架构

左侧菜单新增一级入口：

```text
工作区
- 工作台
- 收件箱
- 任务中心
- 数字员工
- 技能管理
- 项目管理
- 团队管理
```

`收件箱` 使用 `Inbox` 或 `MailCheck` 图标，展示当前用户 open item 数。它不是 `任务中心` 的二级入口，也不替代 `审批中心`。

### 4.2 默认视图

`/inbox` 默认进入 `我的待办`。

`我的待办` 只展示 `target_user_id = 当前用户` 且 `status = open` 的事项。

`团队待办` 给具备团队或租户管理权限的用户查看 open items，用于发现瓶颈和跳转上下文。团队视图不允许默认代处理。

### 4.3 条目消失规则

Inbox 条目是否消失由来源业务状态决定：

- 来源审批被批准、拒绝或要求补证后，条目变为 resolved。
- 来源审批或项目决策被取消后，条目变为 cancelled。
- 来源对象发生新状态变化时，投影更新 `last_activity_at` 和展示快照。

用户不能通过归档、已读或忽略手动隐藏 open item。

### 4.4 内联处理

Inbox 可以处理轻量动作。动作来自 `approval_requests.options` 投影出的 action schema，并兼容以下默认动作：

- `approved`
- `rejected`
- `needs_more_evidence`

每个条目必须提供 `查看上下文`。用户可以先跳到项目详情或审批详情，再完成判断。

## 5. 后端架构

新增模块：

```text
apps/control-plane/internal/inbox/
  service.go
  repository.go
  pg_repository.go
  types.go
```

新增 SQL 和迁移：

```text
apps/control-plane/internal/storage/migrations/016_inbox_items.sql
apps/control-plane/internal/storage/queries/inbox.sql
```

Inbox 模块职责：

- 管理 `inbox_items` read model。
- 提供查询、badge 和动作路由。
- 提供来源模块调用的 projector 方法。
- 不拥有审批或项目决策状态机。
- 不直接修改项目事件流。

来源模块职责：

- `approval` 模块继续拥有审批状态和决策记录。
- `project` 模块继续拥有项目侧决策投影和项目事件。
- 来源模块在创建或处理事实后调用 Inbox projector 同步投影。

## 6. 数据模型

新增表：`inbox_items`

字段建议：

```sql
id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
tenant_id UUID NOT NULL,
team_id UUID,
target_user_id UUID NOT NULL,
scope VARCHAR(50) NOT NULL,
item_type VARCHAR(100) NOT NULL,
source_type VARCHAR(100) NOT NULL,
source_id UUID NOT NULL,
source_project_id UUID,
source_task_id UUID,
source_approval_request_id UUID,
title VARCHAR(255) NOT NULL,
summary TEXT,
risk_level VARCHAR(50),
priority VARCHAR(50),
status VARCHAR(50) NOT NULL,
action_schema JSONB NOT NULL DEFAULT '[]'::jsonb,
context_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
deep_link JSONB NOT NULL DEFAULT '{}'::jsonb,
resolved_at TIMESTAMPTZ,
last_activity_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
```

状态字段：

```text
status = open | resolved | cancelled
scope = personal | team
```

第一版所有可处理事项默认是 `scope = personal`。团队视图通过权限查询租户或团队范围内的 open items，不依赖 `scope = team`。`scope = team` 只预留给未来真正分配给团队的事项。

索引建议：

```sql
CREATE INDEX idx_inbox_items_tenant_target_status_activity
  ON inbox_items(tenant_id, target_user_id, status, last_activity_at DESC);

CREATE INDEX idx_inbox_items_tenant_status_activity
  ON inbox_items(tenant_id, status, last_activity_at DESC);

CREATE INDEX idx_inbox_items_tenant_project_status_activity
  ON inbox_items(tenant_id, source_project_id, status, last_activity_at DESC)
  WHERE source_project_id IS NOT NULL;

CREATE UNIQUE INDEX uq_inbox_items_tenant_source
  ON inbox_items(tenant_id, source_type, source_id);

CREATE UNIQUE INDEX uq_inbox_items_tenant_approval_source
  ON inbox_items(tenant_id, source_approval_request_id)
  WHERE source_approval_request_id IS NOT NULL;
```

不添加跨模块 FK。`source_type + source_id`、`source_approval_request_id`、`source_project_id` 等引用由应用层校验租户和来源一致性。这样符合现有数据库规范，也避免 Inbox read model 破坏审批、项目、Runtime 等模块的长期演进。

## 7. 来源投影规则

### 7.1 全局审批来源

当只有全局审批，没有项目决策投影时：

```text
item_type = approval
source_type = approval_request
source_id = approval_requests.id
source_approval_request_id = approval_requests.id
target_user_id = approval_requests.target_user_id
status = open when approval status is pending
```

当审批状态变为 `approved`、`rejected` 或 `needs_more_evidence` 时，Inbox item 同步为 `resolved`。

当审批状态变为 `cancelled` 时，Inbox item 同步为 `cancelled`。

### 7.2 项目决策来源

项目决策类待办以 `project_decision_requests` 为主来源：

```text
item_type = project_decision
source_type = project_decision_request
source_id = project_decision_requests.id
source_project_id = project_decision_requests.project_id
source_task_id = project_decision_requests.project_task_id
source_approval_request_id = project_decision_requests.approval_request_id
target_user_id = project_decision_requests.target_user_id
```

同一个 `approval_request_id` 只能生成一个 Inbox item。若审批 projector 已经先创建了 `approval` item，项目 projector 必须使用 `source_approval_request_id` 查找并升级该 item 为项目决策类 item，而不是创建重复记录。

项目决策处理完成后：

- `approval_requests` 写入最终状态。
- `approval_decisions` 写入处理记录。
- `project_decision_requests.status_snapshot` 同步。
- `project_events` 写入 `decision.submitted`。
- `inbox_items.status` 同步为 `resolved`。

## 8. 同步与一致性

第一版优先使用同事务同步：

1. 来源服务完成事实写入。
2. 同事务调用 Inbox projector upsert 或 resolve。
3. 返回给 API handler。

如果后续来源增加、事件量变大或需要跨服务异步处理，可以引入 outbox/event projector。第一版不引入 outbox。

一致性原则：

- 来源对象是事实源。
- Inbox item 是 read model。
- 查询或处理时发现投影过期，应以来源状态为准同步投影。
- `action_schema` 只做展示和前置校验，最终合法性仍由来源服务校验。

需要预留回补能力：

```text
RebuildOpenItems(ctx, tenantID)
```

该能力用于从 `approval_requests` 和 `project_decision_requests` 重建 open items，修复投影缺失或历史数据接入问题。

## 9. API 设计

### 9.1 List Inbox Items

```text
GET /api/v1/inbox/items
```

Query：

```text
view = mine | team
status = open | resolved | cancelled
item_type
risk_level
project_id
target_user_id
limit
offset
```

默认：

```text
view = mine
status = open
limit = 50
offset = 0
```

`target_user_id` 只允许团队视图使用。

响应：

```json
{
  "items": [],
  "pagination": {
    "limit": 50,
    "offset": 0,
    "has_more": false
  },
  "summary": {
    "open_count": 3,
    "high_risk_count": 1,
    "blocked_count": 0
  }
}
```

### 9.2 Inbox Badge

```text
GET /api/v1/inbox/badge
```

响应：

```json
{
  "mine_open_count": 3,
  "team_open_count": 12,
  "high_risk_count": 1
}
```

`team_open_count` 只对具备团队视图权限的用户返回真实值；普通用户可以返回 0 或省略。

### 9.3 Execute Inbox Action

```text
POST /api/v1/inbox/items/{id}/actions
```

请求：

```json
{
  "action": "approved",
  "comment": "上下文充分，同意继续。",
  "payload": {}
}
```

处理流程：

1. 读取 Inbox item。
2. 校验 item 属于当前租户且 `status = open`。
3. 校验当前用户是 `target_user_id`。
4. 校验 action 存在于 action schema。
5. 根据 `source_type` 路由到 `approval` 或 `project` 服务。
6. 来源服务完成状态流转和审计事件。
7. Inbox projector 同步 item 状态。
8. 返回更新后的 item 和来源处理结果。

响应：

```json
{
  "item": {},
  "source_result": {
    "source_type": "project_decision_request",
    "source_id": "00000000-0000-0000-0000-000000000000",
    "status": "approved"
  }
}
```

## 10. 错误处理

建议错误码：

```text
400 invalid_inbox_action
403 inbox_view_forbidden
403 inbox_action_forbidden
404 inbox_item_not_found
409 inbox_item_stale
422 inbox_source_unavailable
500 inbox_projection_failed
```

语义：

- `invalid_inbox_action`：action 不在 item action schema 中。
- `inbox_view_forbidden`：当前用户无团队视图权限。
- `inbox_action_forbidden`：当前用户不是目标处理人，或团队视图尝试代处理。
- `inbox_item_not_found`：item 不存在或不属于当前租户。
- `inbox_item_stale`：item 仍是 open，但来源对象已处理或状态不一致。
- `inbox_source_unavailable`：来源对象缺失、跨租户不一致或 source router 不支持。
- `inbox_projection_failed`：来源事实写入成功但投影同步失败。

第一版应尽量通过同事务避免来源成功但投影失败。如果仍发生投影失败，不能把来源处理说成失败；必须记录错误并提供回补能力。

## 11. 权限与审计

权限：

- `mine` 视图：当前登录用户可访问自己的 open items。
- `team` 视图：需要团队管理员、项目负责人或后续统一授权接口允许。
- Inbox action：第一版只允许 `target_user_id` 本人处理。
- 管理员代处理不在第一版范围。

审计：

- Inbox 自身不写独立审批决策。
- 实际处理动作由来源模块写入审批决策、项目事件和审计事件。
- Inbox 可以在 action handler 写轻量审计事件，记录用户从 Inbox 发起处理，但不能替代来源审计。

## 12. 前端设计

新增路由：

```text
/inbox
```

页面结构：

- Header：标题 `收件箱`，副标题 `需要你处理的事项`。
- Tabs：`我的待办`、`团队待办`。
- 筛选条：类型、风险等级、项目、目标处理人、排序。
- 主列表：高密度列表或表格。
- 详情抽屉或右侧面板：展示上下文、来源、风险、动作和跳转入口。

列表项展示：

```text
标题
摘要
类型
风险等级
来源项目
目标处理人
最后活动时间
动作按钮
查看上下文
```

内联动作：

- 列表行最多展示两个主动作。
- 其他动作放入更多菜单。
- 动作点击后打开确认弹层或详情动作区。
- 支持填写 comment。
- 成功后刷新列表，item 从 `我的待办` 消失。
- 失败时保留 item 并展示错误原因。

团队视图：

- 展示团队或租户 open items。
- 支持按处理人、项目、风险筛选。
- 非目标处理人只显示 `查看上下文`。
- 如果当前用户刚好是目标处理人，可以显示处理动作。

React Query 行为：

- queryKey 必须包含 view、status、filter、page。
- queryKey 变化时使用 `placeholderData: keepPreviousData`。
- 已有数据后台刷新时不能卸载主内容。
- 选中项、展开项和当前视图不因 refetch 重置；只有新数据不再包含该 item 时才回退默认状态。

空状态：

- `我的待办` 为空：显示当前没有需要你处理的事项。
- `团队待办` 为空：显示当前没有团队待处理事项。
- 不使用 mock 数据冒充业务能力。

## 13. 与通知中心的演进关系

方案二必须为未来方案三留出边界，但不能提前实现通知中心。

后续如需完整通知中心，应新增独立模型：

```text
notification_events
notification_deliveries
notification_preferences
inbox_item_states 或 notification_user_states
```

演进关系：

```text
Inbox = actionable work queue
Notifications = read-only event/message feed
Badge/Topbar = 两者的聚合入口
```

只读通知不应直接写入 `inbox_items`，否则会污染第一版“真实待办”的产品定位。

## 14. 测试策略

后端：

- Inbox service 创建审批 open item。
- 项目决策 projector 通过 `source_approval_request_id` 防重复。
- 审批 resolved 后 item 自动 resolved。
- 非目标用户 action 返回 403。
- team view 能查询但不能代处理。
- stale item 能同步或返回 409。
- repository 查询按 `last_activity_at DESC` 排序。
- badge 只统计当前用户 open items。

前端：

- 侧边栏新增 `收件箱` 且位置正确。
- 默认进入 `我的待办`。
- 列表展示标题、风险、项目、目标处理人和动作。
- team view 对非目标用户不展示处理按钮。
- action 成功后 item 从列表消失。
- action 失败时保留 item 并展示错误。
- queryKey 变化保留旧数据，后台刷新不卸载主内容。
- 空状态不使用 mock 数据。

契约与生成：

- 修改 OpenAPI 后运行 `pnpm generate:control-plane`。
- 执行 `pnpm verify:contracts`。
- 修改 sqlc 查询或 schema 后运行 `make -C apps/control-plane generate-sqlc`。

## 15. 验收标准

- 左侧菜单出现 `收件箱`，位于 `工作台` 与 `任务中心` 之间。
- `/inbox` 默认展示当前用户 open items。
- 第一版只展示审批和项目决策类可操作事项。
- 由项目决策生成的待办不会与同一个全局审批重复。
- 当前目标用户可以在 Inbox 内处理合法动作。
- 团队视图可查看团队待办，但不能代处理。
- 处理成功后审批事实源、项目决策投影和 Inbox 投影状态一致。
- Inbox 不展示纯通知，不支持已读、未读、手动归档或稍后处理。
- Runtime、预算、验收等后续来源可以通过新增 projector 接入，不需要重写前端主列表模型。
