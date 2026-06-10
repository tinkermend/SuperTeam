# 2026-06-10-project-management-menu-frontend-backend-design

## 1. 背景与目标

SuperTeam 的项目管理需要从占位入口演进为真实的项目闭环控制台。项目不是单纯的软件交付项目，也可以是一类具体问题场景的闭环容器，聚合目标、负责人、虚拟协调线程、数字员工池、任务、证据、审批、预算和验收结论。

本设计基于以下材料：

- `docs/design/projectManager/temporal-project-coordination-design.md`
- `docs/design/projectManager/project-management-operations-detail-gpt-image2.png`
- `docs/design/projectManager/project-management-configuration-governance-gpt-image2.png`

本设计解决两个核心问题：

- 项目管理菜单如何组织，避免把项目管理误做成需求提交入口或后台配置中心。
- 项目管理前后端第一阶段如何落地，同时为 Temporal 项目协调、证据和治理能力留出清晰边界。

## 2. 核心决策

### 2.1 菜单口径

左侧全局菜单只保留一个一级入口：

```text
项目管理 -> /projects
```

不在全局左侧菜单展开项目任务、项目成员、项目配置、项目事件等二级菜单。这些能力都属于某个具体项目，应放在项目详情内部。

未来的“任务发起”或“需求提交”是独立心智入口，用于提出需求、选择项目并触发该项目的虚拟协调线程。它不替代项目管理。

### 2.2 页面心智

项目管理默认页采用“运行态优先”：

- 第二张参考图作为项目详情默认页方向，展示项目当前状态、任务、人类决策、成员和历史事件。
- 第三张参考图作为项目配置治理页方向，展示负责人、数字员工池、协调策略、审批规则和证据归档规则。

因此页面关系为：

```text
/projects
  项目管理首页，左侧项目列表，右侧选中项目运行态

/projects/$projectId
  指定项目运行态详情，可直接分享 URL

/projects/$projectId/config
  指定项目配置治理页
```

### 2.3 协调员口径

项目不定义“项目协调员数字员工”。项目绑定一个虚拟协调线程：

```text
WorkflowID = project-coordinator:{project_id}
```

该线程由 Temporal Workflow 承载，是项目内置的独占协调状态机。它不是数字员工，不出现在数字员工列表中，也不是项目成员。项目需要定义的是人类负责人、可选 leader、验收人，以及项目内可调度数字员工池。

## 3. 前端功能设计

### 3.1 项目管理首页

`/projects` 是项目管理入口，负责项目切换和选中项目运行态展示。

页面结构：

```text
顶部
- 标题：项目管理
- 全局搜索：项目、负责人、数字员工、Workflow ID
- 操作：提交需求到当前项目、项目配置、新建项目

左侧：项目切换
- 项目搜索
- 状态筛选
- 项目列表
- 项目状态：运行中、执行中、配置中、验收中、已归档
- 轻量指标：成员数、任务数、待决策数

中间：项目运行态
- 项目标题、状态、人类负责人、Temporal Workflow ID
- 当前阶段、活跃任务、待补证、待人类决策、证据完整度
- 任务看板或任务表
- 项目历史事件流

右侧：参与者与待办
- 当前项目成员
- 数字员工池在线状态和并发槽位
- 人类决策队列
- 协同通知
```

没有项目时显示空状态，引导创建项目。没有选中项目但有项目列表时，默认选中最近活跃项目，并把 URL 更新为 `/projects/$projectId`。

### 3.2 项目运行态详情页

`/projects/$projectId` 与 `/projects` 共用主要布局，但项目由 URL 明确指定。

运行态详情页回答：

```text
这个项目现在处于什么阶段？
有什么任务正在执行？
有没有人类负责人需要处理的决策？
数字员工池是否可用？
最近发生了哪些项目事件？
```

关键组件：

- `ProjectSwitcherPane`：项目列表、筛选、状态徽标。
- `ProjectOperationalHeader`：项目标题、状态、负责人、Workflow ID、快捷操作。
- `ProjectStatusCards`：当前阶段、活跃任务、待补证、待人类决策、证据完整度。
- `ProjectTaskBoard`：按任务状态分组，也支持表格视图。
- `ProjectMembersPanel`：人类负责人、leader、验收人、数字员工池。
- `ProjectDecisionQueue`：待人类处理的暂停点。
- `ProjectEventTimeline`：项目事件流。

### 3.3 项目配置治理页

`/projects/$projectId/config` 用于治理和变更配置，不作为默认详情页。

页面结构：

```text
顶部
- 返回项目详情
- 提交需求到当前项目
- 归档项目
- 保存配置

配置 Tab
- 概览
- 成员
- 数字员工池
- 协调策略
- 审批规则
- 证据归档
- 任务历史
```

主要配置块：

- 基本项目信息：名称、目标、场景描述、状态。
- 人类角色：负责人、leader、验收人、观察者。
- 数字员工池：可调度数字员工、项目内角色、并发槽位、是否可用、风险边界。
- 协调策略：自动规划、低风险自动派发、高风险暂停审批、转派请求处理。
- 审批规则：计划确认、风险动作、补证、验收、变更范围。
- 证据归档：必须产出的证据类型、报告格式、归档保留周期。

### 3.4 前端状态与交互规则

项目管理页面必须遵守当前项目的 UI 数据加载规则：

- 列表、Tab、筛选、分页切换时保留旧数据。
- React Query 使用稳定 queryKey，并在 queryKey 变化时使用 `placeholderData: keepPreviousData` 或等效策略。
- 后台刷新时只显示局部刷新状态，不能卸载主内容。
- 选中项目、展开项、当前视图等本地 UI 状态不因 refetch 重置。
- 只有当新数据不再包含当前选中对象时，才回退到默认选中项目。

## 4. 后端模块设计

新增项目模块：

```text
apps/control-plane/internal/project/
  types.go
  service.go
  repository.go
  pg_repository.go
  handler.go
```

项目模块是项目事实源，负责项目、成员、需求、项目任务、项目事件、路由决策和项目配置聚合。

不放入项目模块的职责：

- 数字员工定义仍归 `employee` 模块。
- 底层 Runtime Task 仍归 `task` 模块。
- Runtime 节点、claim、lease 仍归 `runtime` 模块。
- 企业审批中心仍归 `approval` 模块。
- 工件存储与对象索引仍归 `artifact` 模块。
- Temporal worker 和 workflow runtime 仍归 `workflow` 模块。

### 4.1 核心对象

V0 必须实现：

```text
Project
- 项目容器
- 保存名称、目标、状态、人类负责人、leader、验收人、协调 Workflow ID

ProjectMember
- 项目参与关系
- principal_type: human_user / digital_employee / team
- project_role: owner / leader / acceptance / executor / reviewer / observer
- 不允许 coordinator

ProjectTask
- 项目内业务任务
- 不等同于底层 Runtime task
- 可关联 Runtime Task 或 Digital Employee Run

ProjectEvent
- 项目事件流
- 记录创建、配置变更、成员变更、任务变更、决策和归档动作

ProjectDemand
- 用户提交到某项目的一次需求
- 可由项目详情页发起，也可由未来独立任务发起入口提交
```

V1 引入：

```text
CoordinationJob
RouteDecision
ExecutionSummary
TransferRequest
DecisionRequest
```

V2 引入：

```text
EvidenceRef
ArtifactRef
ReportRef
BudgetLedger
ProjectAcceptance
ProjectArchiveSnapshot
ProjectConfigRevision
```

### 4.2 数据库表建议

遵循 `DATABASE_DESIGN.md`：UUID-first、Tenant-first、Team-aware、事件/运行分层、审计保留、跨模块关系默认由应用层校验。

V0 表：

```text
projects
project_members
project_tasks
project_events
project_demands
project_config_revisions
```

V1 表：

```text
project_coordination_jobs
project_route_decisions
project_execution_summaries
project_transfer_requests
project_decision_requests
```

V2 表：

```text
project_evidence_refs
project_artifact_refs
project_budget_ledger
project_acceptance_records
project_archive_snapshots
```

关键规则：

- 所有业务核心表包含 `tenant_id`。
- 团队级项目包含 `team_id`。
- 高频列表索引以 `tenant_id` 开头。
- 跨模块引用数字员工、用户、Runtime、审批、工件时保存 UUID 和必要快照，默认不加重级联外键。
- `project_role` 不支持 `coordinator`。
- 项目协调线程使用 `coordination_workflow_id` 表达，不使用 `coordinator_employee_id`。
- 项目事件表使用项目内递增 `sequence_number` 支撑有序事件流。

## 5. OpenAPI 与前端 API Client

### 5.1 OpenAPI 路径

V0 API：

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

V1 API：

```text
GET    /api/v1/projects/{projectId}/route-decisions
GET    /api/v1/projects/{projectId}/coordination-jobs
GET    /api/v1/projects/{projectId}/decisions
POST   /api/v1/projects/{projectId}/decisions/{decisionId}/resolve
GET    /api/v1/projects/{projectId}/execution-summaries
GET    /api/v1/projects/{projectId}/transfer-requests
```

V2 API：

```text
GET    /api/v1/projects/{projectId}/evidence
GET    /api/v1/projects/{projectId}/artifacts
GET    /api/v1/projects/{projectId}/budget-ledger
POST   /api/v1/projects/{projectId}/acceptance
POST   /api/v1/projects/{projectId}/archive-snapshot
```

### 5.2 聚合接口

`GET /api/v1/projects/{projectId}/overview` 是运行态首屏聚合接口，避免前端首屏并发请求过多。

返回内容包括：

```text
project
status_summary
human_roles
digital_employee_pool_summary
task_summary
active_tasks
pending_decisions
recent_events
coordination_workflow
```

### 5.3 前端 API Client

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

V1 增加：

```text
listProjectRouteDecisions()
listProjectCoordinationJobs()
listProjectDecisions()
resolveProjectDecision()
listProjectExecutionSummaries()
listProjectTransferRequests()
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
["project-decisions", projectId]
```

## 6. Temporal 协调边界

V0 不要求真实跑完整 Temporal，但数据模型和 API 必须预留：

```text
Project.coordination_workflow_id
Project.coordination_status
Project.coordination_policy
ProjectEvent.event_type = project.workflow.started
ProjectEvent.event_type = project.workflow.signal_sent
ProjectEvent.event_type = project.policy.changed
```

V1 接入 Temporal 后，需求提交流程：

```text
POST /api/v1/projects/{projectId}/demands
  -> 写入 ProjectDemand
  -> 写入 ProjectEvent: demand.submitted
  -> Signal Temporal Workflow: DemandSubmitted
  -> 返回 demand 与当前项目状态
```

数字员工回写流程：

```text
Runtime writeback
  -> 写入 ProjectTask / ExecutionSummary / ProjectEvent
  -> Signal Temporal Workflow: EmployeeTaskCompleted 或 EmployeeTransferRequested
```

项目配置变更流程：

```text
PUT /api/v1/projects/{projectId}/config
  -> 校验人类负责人、数字员工池和策略
  -> 写入 ProjectConfigRevision
  -> 更新 Project / ProjectMember
  -> 写入 ProjectEvent: project.config.changed
  -> V1 起 Signal Temporal Workflow: ProjectPolicyChanged / ProjectMemberChanged
```

同一项目内协调决策必须串行提交。数字员工可以并发执行任务，但 RouteDecision、任务转派、人类暂停点和预算扣减必须通过项目级协调 Workflow 有序处理。

## 7. 权限、校验与错误处理

### 7.1 权限

V0 使用现有统一授权接口预留权限点，避免业务代码散落权限判断。

建议权限动作：

```text
project.read
project.create
project.update
project.archive
project.member.manage
project.config.manage
project.demand.submit
project.task.read
project.decision.resolve
```

后续可映射到 OpenFGA。

### 7.2 服务端校验

必须校验：

- 项目名称、目标、人类负责人不能为空。
- 人类负责人必须是人类用户，不得选择数字员工。
- 数字员工池成员必须是数字员工，不得把人类放入 executor 池。
- `ProjectMember.project_role` 不允许 `coordinator`。
- 项目数字员工池必须来自当前租户和团队允许范围。
- 项目配置变更必须写 ProjectEvent。
- 归档项目后默认禁止新增需求和变更配置。
- V1 起 ProjectTask 只能分派给项目数字员工池内员工。

### 7.3 错误状态

前端必须覆盖：

- 无项目。
- 项目不存在或无权限。
- 项目已归档。
- 数字员工池为空。
- 项目配置保存冲突。
- overview 聚合接口部分数据失败。
- 后台刷新失败但保留已有页面数据。

## 8. 分阶段路线图

### 8.1 V0：项目管理可用骨架

目标：把「项目管理」从占位页变成真实管理入口，先落运行态详情和配置治理主要结构。

范围：

```text
前端
- /projects 项目管理首页
- /projects/$projectId 项目运行态详情
- /projects/$projectId/config 项目配置页
- 新建项目入口
- 项目列表、搜索、状态筛选
- 项目运行态 overview
- 项目成员与数字员工池展示
- 项目任务列表
- 项目事件流
- 项目配置保存

后端
- internal/project 模块
- Project / ProjectMember / ProjectTask / ProjectEvent 基础表
- ProjectConfig 聚合读写
- ProjectOverview 聚合接口
- ProjectDemand 表与接口
- 写项目事件流
- OpenAPI + oapi-codegen + Web API client
```

V0 不做：

```text
- 真实 Temporal Workflow Worker
- 自动 LLM 规划
- 真实数字员工自动分派
- 复杂证据归档
- 预算流水
```

V0 验收：

```text
- 左侧菜单「项目管理」进入真实页面，不再是占位页。
- 可以创建项目并指定人类负责人、leader、验收人。
- 可以给项目选择数字员工池。
- 可以进入项目运行态详情，看到项目状态、成员、任务、事件。
- 可以进入配置页并保存配置。
- 所有关键动作写 ProjectEvent。
- 页面切换项目、筛选、刷新时不卸载已有主内容。
```

### 8.2 V1：Temporal 项目协调接入

目标：让项目运行态由真实协调流程驱动。

范围：

```text
后端
- project-coordinator:{project_id} Workflow 生命周期
- DemandSubmitted signal
- ProjectPolicyChanged signal
- ProjectMemberChanged signal
- EmployeeTaskCompleted signal
- EmployeeTransferRequested signal
- HumanDecisionSubmitted signal
- CoordinationJob
- RouteDecision
- ExecutionSummary
- TransferRequest
- DecisionRequest

前端
- 提交需求到当前项目
- 任务规划状态展示
- RouteDecision 展示
- 人类决策队列可处理
- 数字员工结果回写后更新项目任务
- Workflow ID / signal 状态 / 最近协调事件可观测
```

V1 验收：

```text
- 在项目详情页提交需求后，会写 ProjectDemand 并 signal 项目 Workflow。
- Workflow 生成 RouteDecision 和 ProjectTask。
- ProjectTask 只能分派给项目数字员工池内员工。
- 数字员工完成、失败、转派请求能回写项目事件。
- 需要人工判断时生成 DecisionRequest。
- 人类负责人处理 DecisionRequest 后 Workflow 继续。
- 同一项目协调决策保持串行。
```

### 8.3 V2：治理、证据与归档增强

目标：让项目成为完整可审计闭环容器。

范围：

```text
- EvidenceRef / ArtifactRef / ReportRef
- BudgetLedger
- 项目验收结论
- 项目归档快照
- 配置修订历史
- 风险策略版本
- 审计中心联动
- 成本中心联动
- 跨项目统计与运行健康
```

V2 验收：

```text
- 每个项目阶段都有证据链。
- 每次 RouteDecision 有输入、理由、输出契约和预算估计。
- 每个 ExecutionSummary 有工件、证据和不确定性说明。
- 项目归档后能复盘需求、任务、决策、证据、审批和报告。
- 成本、审计、审批中心能按 project_id 追踪。
```

## 9. 测试策略

### 9.1 V0 测试

前端：

- Vitest 覆盖项目列表、运行态详情、配置页关键状态。
- React Query 行为测试覆盖筛选、分页、Tab 切换时保留旧数据。
- Playwright 真实浏览器访问 `/projects` 和 `/projects/$projectId/config`，截图验证布局。
- 覆盖无项目、加载失败、空数字员工池、无权限状态。

后端：

- Go test 覆盖 project service 状态校验。
- repository 测试覆盖租户隔离、成员写入、事件顺序。
- handler 测试覆盖 OpenAPI 路由、错误码、参数校验。
- migration 测试确保 Atlas/sqlc 生成通过。

### 9.2 V1 测试

- Temporal workflow test suite 覆盖 signal 顺序。
- 成员池变更后分派限制测试。
- 人类决策暂停和恢复测试。
- Runtime 回写到 ProjectEvent 的集成测试。
- 同一项目并发 signal 下的串行协调测试。

## 10. 非目标

本设计不覆盖以下内容：

- 独立“任务发起”一级入口的完整 UI 设计。
- LLM 规划提示词和模型选择策略。
- 复杂流程图编辑器。
- 跨项目资源排班。
- 数字员工之间自由聊天。
- 控制平面直接执行本地命令。

这些能力可以在项目管理 V0/V1 稳定后单独设计。

## 11. 结论

项目管理第一阶段应采用“运行态详情为默认页，配置治理为独立页”的产品结构。

这个结构同时满足三点：

- 负责人打开项目后先看到是否需要处理事情。
- 项目治理能力有清晰入口，不挤压运行态首页。
- 后端能从 V0 的真实项目事实源平滑演进到 V1 的 Temporal 项目协调。

最终推荐路线：

```text
V0：真实项目管理骨架
V1：Temporal 虚拟协调线程接入
V2：证据、预算、归档和审计闭环增强
```
