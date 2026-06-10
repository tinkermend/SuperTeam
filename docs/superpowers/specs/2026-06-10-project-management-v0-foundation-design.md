# 2026-06-10-project-management-v0-foundation-design

## 1. 阶段目标

V0 的目标是把 `项目管理` 从占位页变成真实可用的项目管理入口，落地项目列表、项目运行态详情和项目配置治理的基础闭环。

V0 不追求自动协调，不接入真实 Temporal Workflow，不做 LLM 规划，不做数字员工自动分派。它要先把项目作为业务事实容器立住，并让后续 V1 能在这个事实源上接入协调编排。

## 2. 成功标准

V0 完成后，用户应该能够：

- 从左侧一级菜单进入 `/projects`。
- 创建项目，并设置项目目标、人类负责人、leader、验收人。
- 给项目选择可调度数字员工池。
- 查看项目运行态详情，包括状态、成员、任务、事件和待处理项。
- 进入项目配置页，修改成员、数字员工池和基础协调策略。
- 在项目详情页提交需求到当前项目，并看到需求事件被记录。
- 看到所有关键动作进入 `ProjectEvent` 事件流。

V0 的关键判断标准是：页面显示真实 Control Plane 数据，不用 mock 数据伪装业务能力。

## 3. 范围边界

### 3.1 包含

前端：

- `/projects`
- `/projects/$projectId`
- `/projects/$projectId/config`
- 项目创建入口。
- 项目列表、搜索和状态筛选。
- 项目运行态首屏。
- 项目配置治理页。
- 项目成员与数字员工池管理。
- 项目任务列表。
- 项目事件流。
- 提交需求到当前项目。

后端：

- `apps/control-plane/internal/project` 模块。
- `projects`、`project_members`、`project_tasks`、`project_events`、`project_demands`、`project_config_revisions` 表。
- ProjectOverview 聚合接口。
- ProjectConfig 读写接口。
- OpenAPI 契约与 oapi-codegen 生成。
- Web API client。

### 3.2 不包含

V0 明确不做：

- Temporal Workflow Worker。
- `project-coordinator:{project_id}` 的真实运行。
- LLM 规划和 RouteDecision 生成。
- 数字员工自动分派。
- Runtime 回写到项目任务。
- ExecutionSummary、TransferRequest、DecisionRequest。
- 证据归档、预算流水、项目验收和归档快照。
- 独立“任务发起”一级入口。

V0 可以保留字段与状态占位，但不能把上述能力实现成半成品。

## 4. 前端设计

### 4.1 路由

```text
/projects
  项目管理首页。左侧项目列表，右侧展示默认选中项目。

/projects/$projectId
  指定项目运行态详情。与 /projects 共用布局，但选中项来自 URL。

/projects/$projectId/config
  指定项目配置治理页。
```

### 4.2 项目管理首页

首页布局对应运行态参考图。

左侧项目列表：

- 搜索项目名称、负责人、项目编号。
- 按状态筛选：全部、运行中、配置中、暂停、验收中、已归档。
- 项目卡片展示：名称、状态、负责人、成员数、项目任务数、待处理决策数。
- 点击项目后更新 URL 到 `/projects/$projectId`。

中间运行态：

- 项目标题、状态、人类负责人、Workflow ID 占位。
- 当前阶段、活跃任务、待人工处理、证据完整度。
- 项目任务表：任务标题、分派对象、状态、最近事件、更新时间。
- 项目历史事件：按 `sequence_number` 正序或倒序展示。

右侧参与者与待办：

- 人类负责人、leader、验收人。
- 数字员工池：名称、角色、可用状态、并发槽位。
- V0 的人类决策队列只展示占位或空状态，不处理真实 DecisionRequest。

### 4.3 项目配置页

配置页对应配置治理参考图。

Tab：

- 概览。
- 成员。
- 数字员工池。
- 协调策略。
- 审批规则。
- 证据归档。
- 任务历史。

V0 中各 Tab 的落地深度：

- 概览：可编辑项目名称、目标、场景描述、状态。
- 成员：可设置 human owner、leader、acceptance、observer。
- 数字员工池：可从现有数字员工列表中选择 executor，设置项目内角色和并发槽位。
- 协调策略：保存 JSON 策略配置，提供少量表单项，不触发 Temporal。
- 审批规则：保存 JSON 规则配置，只作为后续 V1/V2 的输入。
- 证据归档：保存 JSON 归档要求，不执行真实证据校验。
- 任务历史：展示当前项目任务列表。

### 4.4 新建项目

V0 使用抽屉或向导，步骤为：

1. 基础信息：名称、目标、描述、团队。
2. 人类角色：负责人必填，leader 与验收人可选。
3. 数字员工池：从现有数字员工中选择 executor。
4. 策略预设：低风险自动派发、高风险需审批、证据归档要求。
5. 确认创建。

创建成功后跳转到 `/projects/$projectId`。

### 4.5 数据加载规则

- 项目列表筛选变化时保留旧列表。
- 项目详情后台刷新时不卸载详情主体。
- 当前选中项目只有在新列表不再包含该项目时才回退。
- 配置保存失败时保留用户输入，并展示错误。
- 切换 Tab 不触发整个配置页重挂载。

## 5. 后端设计

### 5.1 模块结构

```text
apps/control-plane/internal/project/
  types.go
  service.go
  repository.go
  pg_repository.go
  handler.go
```

模块职责：

- 项目基础事实。
- 项目成员关系。
- 项目数字员工池。
- 项目任务事实。
- 项目事件流。
- 项目需求记录。
- 项目配置修订。

模块不直接调用 Runtime，不直接执行 Provider，不直接执行 Temporal Workflow。

### 5.2 数据表

`projects`：

- `id`
- `tenant_id`
- `team_id`
- `name`
- `description`
- `goal`
- `status`
- `human_owner_user_id`
- `leader_user_id`
- `acceptance_user_id`
- `coordination_workflow_id`
- `coordination_status`
- `coordination_policy`
- `approval_policy`
- `evidence_policy`
- `archived_at`
- `created_at`
- `updated_at`

`project_members`：

- `id`
- `tenant_id`
- `project_id`
- `principal_type`
- `principal_id`
- `project_role`
- `display_name_snapshot`
- `status`
- `settings`
- `created_at`
- `updated_at`

`project_tasks`：

- `id`
- `tenant_id`
- `project_id`
- `demand_id`
- `title`
- `summary`
- `status`
- `assigned_digital_employee_id`
- `runtime_task_id`
- `digital_employee_run_id`
- `risk_level`
- `requires_human_approval`
- `latest_event_id`
- `created_at`
- `updated_at`

`project_events`：

- `id`
- `tenant_id`
- `project_id`
- `sequence_number`
- `event_type`
- `actor_type`
- `actor_id`
- `resource_type`
- `resource_id`
- `summary`
- `payload`
- `created_at`

`project_demands`：

- `id`
- `tenant_id`
- `project_id`
- `submitted_by_user_id`
- `title`
- `content`
- `source_type`
- `source_refs`
- `attachments`
- `priority`
- `risk_level`
- `status`
- `created_event_id`
- `created_at`
- `updated_at`

`project_config_revisions`：

- `id`
- `tenant_id`
- `project_id`
- `revision_number`
- `config_snapshot`
- `change_summary`
- `created_by_user_id`
- `created_event_id`
- `created_at`

### 5.3 状态

Project status：

```text
draft
configuring
running
paused
acceptance
archived
```

ProjectTask status：

```text
pending
planned
assigned
running
waiting_human
completed
failed
cancelled
```

ProjectDemand status：

```text
submitted
recorded
planning_pending
cancelled
```

V0 中提交需求后状态停留在 `recorded` 或 `planning_pending`，不进入真实规划。

### 5.4 服务端校验

- 项目名称、目标、人类负责人必填。
- 人类负责人、leader、验收人必须是人类用户。
- 数字员工池成员必须是数字员工。
- `project_role = coordinator` 非法。
- 归档项目不能修改配置、成员或提交新需求。
- 同一项目内同一 principal 的同一活跃 role 不重复。
- 项目事件 `sequence_number` 必须在项目内递增。
- 所有写操作必须校验租户与团队范围。

## 6. API 设计

V0 OpenAPI：

```text
GET    /api/v1/projects
POST   /api/v1/projects
GET    /api/v1/projects/{projectId}
PATCH  /api/v1/projects/{projectId}
POST   /api/v1/projects/{projectId}/archive

GET    /api/v1/projects/{projectId}/overview
GET    /api/v1/projects/{projectId}/members
PUT    /api/v1/projects/{projectId}/members
GET    /api/v1/projects/{projectId}/tasks
GET    /api/v1/projects/{projectId}/events

GET    /api/v1/projects/{projectId}/config
PUT    /api/v1/projects/{projectId}/config

POST   /api/v1/projects/{projectId}/demands
GET    /api/v1/projects/{projectId}/demands
```

`GET /projects/{projectId}/overview` 返回：

```text
project
human_roles
member_summary
digital_employee_pool
status_summary
task_summary
active_tasks
recent_events
coordination_workflow
```

`PUT /projects/{projectId}/config` 必须：

1. 校验负责人和成员。
2. 写 `project_config_revisions`。
3. 更新 `projects` 与 `project_members`。
4. 写 `project_events`。
5. 返回最新 config。

## 7. 前端 API Client

新增：

```text
apps/web/src/lib/api/projects.ts
```

方法：

```text
listProjects()
createProject()
getProject()
updateProject()
archiveProject()
getProjectOverview()
getProjectMembers()
updateProjectMembers()
getProjectConfig()
updateProjectConfig()
listProjectTasks()
listProjectEvents()
submitProjectDemand()
listProjectDemands()
```

React Query key：

```text
["projects", filters]
["project", projectId]
["project-overview", projectId]
["project-config", projectId]
["project-members", projectId]
["project-tasks", projectId, filters]
["project-events", projectId, cursor]
["project-demands", projectId]
```

## 8. 验收标准

功能验收：

- `/projects` 不再展示 `UnimplementedPage`。
- 可以创建项目并进入详情页。
- 可以保存配置，并在刷新后看到真实数据。
- 可以选择项目数字员工池。
- 可以提交需求到当前项目，事件流出现 `demand.submitted`。
- 可以看到项目任务列表和事件流。
- 可以归档项目，归档后禁止继续修改。

技术验收：

- OpenAPI 更新并生成 Go server 类型。
- sqlc 查询生成通过。
- Atlas migration 测试通过。
- Go project service、repository、handler 测试通过。
- 前端 Vitest 覆盖列表、详情、配置和错误态。
- Playwright 访问 `/projects` 与 `/projects/$projectId/config`，截图无明显布局问题。

## 9. 风险

- 如果 V0 过度实现策略配置，会提前侵入 V1/V2。V0 策略只保存，不执行复杂决策。
- 如果 ProjectTask 和 Runtime Task 混为一谈，会破坏项目业务事实。V0 必须区分项目任务与底层执行任务。
- 如果不写 ProjectEvent，后续运行态和审计会失去基础。V0 所有关键写操作必须落事件。
