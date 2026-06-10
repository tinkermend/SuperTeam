# 2026-06-10-project-management-v1-temporal-coordination-design

## 1. 阶段目标

V1 的目标是在 V0 的项目事实源上接入 Temporal 项目协调，让项目运行态由真实事件和协调决策驱动。

V1 要实现：

- 每个项目绑定一个虚拟协调 Workflow。
- 用户提交需求后 signal 项目 Workflow。
- Workflow 生成结构化 RouteDecision 和 ProjectTask。
- ProjectTask 只能分派给项目数字员工池内员工。
- 数字员工执行结果、失败、转派请求和人类决策都回写项目事件流。

V1 不改变 V0 的菜单心智：项目管理默认仍是运行态详情，配置治理仍是独立页。

## 2. 前置条件

V1 依赖 V0 已完成：

- `projects` 表存在，并包含 `coordination_workflow_id`、`coordination_status`、`coordination_policy`。
- `project_members` 能表达项目数字员工池。
- `project_demands` 能记录用户需求。
- `project_tasks` 能表达项目内业务任务。
- `project_events` 能提供项目事件流。
- `/projects/$projectId` 能展示 overview、任务、成员和事件。

未满足这些条件时不能启动 V1 实施。

## 3. 范围边界

### 3.1 包含

后端：

- Temporal project coordinator Workflow。
- Workflow lifecycle 注册和启动。
- Signals：`DemandSubmitted`、`ProjectPolicyChanged`、`ProjectMemberChanged`、`EmployeeTaskCompleted`、`EmployeeTaskFailed`、`EmployeeTransferRequested`、`HumanDecisionSubmitted`。
- `CoordinationJob`。
- `RouteDecision`。
- `ExecutionSummary`。
- `TransferRequest`。
- 项目侧人类决策投影，审批事实源优先归全局 `approval` 模块。
- Control Plane API 到 Temporal signal 的桥接。
- Runtime 或数字员工回写到项目事件流。

前端：

- 提交需求后展示规划状态。
- RouteDecision 展示。
- 项目任务真实状态更新。
- 人类决策队列处理。
- 最近 Workflow 事件和 signal 状态可观测。

### 3.2 不包含

V1 不做：

- 复杂流程图编辑器。
- 跨项目资源排班。
- 自动学习路由策略。
- 高级证据评分。
- 成本核算和预算预测。
- 项目最终验收归档。

## 4. Temporal 模型

### 4.1 Workflow ID

每个项目一个长期存在的协调 Workflow：

```text
project-coordinator:{project_id}
```

Workflow 是项目内置虚拟协调线程，不是数字员工，不进入 `project_members`。

### 4.2 Workflow 职责

Workflow 负责：

- 串行处理同一项目内的协调决策。
- 接收需求、成员变更、策略变更、执行结果和人类决策。
- 调用 Activity 读取项目快照、成员池、策略和历史结果。
- 调用 Activity 写入 `CoordinationJob`、`RouteDecision`、`ProjectTask`、项目侧人类决策投影和 `ProjectEvent`。
- 等待数字员工结果、人类决策和超时。

Workflow 不负责：

- 直接调用 LLM。
- 直接访问数据库。
- 直接执行 Runtime 命令。
- 直接读取仓库、日志或外部系统。

LLM 调用、数据库读取、HTTP 调用必须在 Activity 中完成。

### 4.3 Signals

```text
DemandSubmitted
- demand_id
- project_id
- submitted_by_user_id
- created_event_id

ProjectPolicyChanged
- project_id
- config_revision_id
- changed_event_id

ProjectMemberChanged
- project_id
- changed_member_ids
- changed_event_id

EmployeeTaskCompleted
- project_task_id
- execution_summary_id
- completed_event_id

EmployeeTaskFailed
- project_task_id
- failure_summary
- failed_event_id

EmployeeTransferRequested
- project_task_id
- transfer_request_id
- requested_event_id

HumanDecisionSubmitted
- approval_request_id
- decision_request_id
- decision
- resolved_event_id
```

### 4.4 Activities

```text
LoadProjectCoordinationSnapshot
CreateCoordinationJob
PlanDemandRoute
PersistRouteDecision
CreateProjectTasks
CreateApprovalRequestForProjectDecision
CreateProjectDecisionProjection
UpdateProjectTaskStatus
PersistExecutionSummary
PersistTransferRequest
AppendProjectEvent
DispatchProjectTask
```

`PlanDemandRoute` 可以调用 LLM，但输出必须是结构化对象，并由服务端校验。

## 5. 数据模型

新增表：

`project_coordination_jobs`：

- `id`
- `tenant_id`
- `project_id`
- `workflow_id`
- `trigger_event_id`
- `job_type`
- `status`
- `input_snapshot_ref`
- `output_event_ids`
- `started_at`
- `finished_at`
- `created_at`

`project_route_decisions`：

- `id`
- `tenant_id`
- `project_id`
- `coordination_job_id`
- `demand_id`
- `candidate_digital_employee_ids`
- `selected_digital_employee_ids`
- `reason`
- `input_requirements`
- `expected_outputs`
- `budget_estimate`
- `requires_human_review`
- `created_event_id`
- `created_at`

`project_execution_summaries`：

- `id`
- `tenant_id`
- `project_id`
- `project_task_id`
- `digital_employee_id`
- `conclusion`
- `evidence_refs`
- `artifact_refs`
- `confidence_factors`
- `uncertainty`
- `missing_information`
- `recommended_next_action`
- `requires_human_review`
- `transfer_request_id`
- `created_event_id`
- `created_at`

`project_transfer_requests`：

- `id`
- `tenant_id`
- `project_id`
- `project_task_id`
- `requested_by_digital_employee_id`
- `reason`
- `suggested_employee_type`
- `suggested_digital_employee_ids`
- `missing_context_refs`
- `status`
- `created_event_id`
- `created_at`
- `updated_at`

`project_decision_requests`：

- `id`
- `tenant_id`
- `project_id`
- `approval_request_id`
- `coordination_job_id`
- `project_task_id`
- `target_user_id`
- `decision_type`
- `title_snapshot`
- `summary_snapshot`
- `risk_level_snapshot`
- `status_snapshot`
- `created_event_id`
- `resolved_event_id`
- `created_at`
- `updated_at`
- `resolved_at`

`project_decision_requests` 不是独立审批引擎，只是项目侧查询投影和事件关联表。审批请求、候选选项、处理人、决策结果、审批状态流转的事实源应归全局 `approval` 模块，例如 `approval_requests` 与 `approval_decisions`。

如果 V1 启动时全局 `approval` 模块仍只有 service 空壳、没有可复用数据结构，则 V1 的第一个后端切片必须先补最小审批核心，再由项目模块通过 `approval_request_id` 引用它。不得在 project 模块内实现一套不可迁移的平行审批流。

## 6. 执行流

### 6.1 提交需求

```text
用户在项目详情提交需求
  -> Control Plane 写 ProjectDemand
  -> Control Plane 写 ProjectEvent: demand.submitted
  -> Control Plane signal DemandSubmitted
  -> Workflow 创建 CoordinationJob
  -> Workflow 调用 PlanDemandRoute Activity
  -> Workflow 写 RouteDecision
  -> Workflow 写 ProjectTask[]
  -> Workflow 写 ProjectEvent: route_decision.created / project_task.created
  -> Workflow 分派 ProjectTask
```

### 6.2 数字员工完成任务

```text
Runtime / Digital Employee writeback
  -> Control Plane 写 ExecutionSummary
  -> Control Plane 更新 ProjectTask
  -> Control Plane 写 ProjectEvent: project_task.completed
  -> Control Plane signal EmployeeTaskCompleted
  -> Workflow 决定是否继续规划、汇总、补证或请求人类决策
```

### 6.3 转派请求

```text
Digital Employee 提交 TransferRequest
  -> Control Plane 写 TransferRequest
  -> Control Plane 写 ProjectEvent: transfer.requested
  -> Control Plane signal EmployeeTransferRequested
  -> Workflow 根据项目数字员工池和策略决定接受或拒绝
  -> 接受则生成新 ProjectTask 或更新原任务
```

数字员工之间不直接自由聊天，不允许私下转派。

### 6.4 人类决策

```text
Workflow 判断需要人工处理
  -> 写全局 ApprovalRequest
  -> 写 project_decision_requests 投影，保存 approval_request_id
  -> 写 ProjectEvent: decision.requested
  -> 前端决策队列展示
  -> 人类负责人批准、驳回或要求补证
  -> approval 模块写 ApprovalDecision
  -> Control Plane 写 ProjectEvent: decision.submitted
  -> Control Plane signal HumanDecisionSubmitted
  -> Workflow 继续
```

## 7. API 设计

V1 新增：

```text
GET    /api/v1/projects/{projectId}/route-decisions
GET    /api/v1/projects/{projectId}/coordination-jobs
GET    /api/v1/projects/{projectId}/decisions
POST   /api/v1/projects/{projectId}/decisions/{decisionId}/resolve
GET    /api/v1/projects/{projectId}/execution-summaries
GET    /api/v1/projects/{projectId}/transfer-requests
```

V0 已有 `POST /projects/{projectId}/demands` 在 V1 中行为升级：

- V0：写需求和事件。
- V1：写需求、写事件、signal Workflow。

V1 必须保持 API 兼容，不能破坏 V0 前端调用。

## 8. 前端变化

项目运行态新增：

- 规划中状态。
- RouteDecision 卡片或详情抽屉。
- ProjectTask 的真实分派员工、状态、最近事件。
- 人类决策队列可操作。
- TransferRequest 提示。
- Workflow 状态区：Workflow ID、最近 signal、最近 coordination job。

配置页新增：

- 修改成员或协调策略后展示“会影响当前 Workflow”的提示。
- 保存后产生 `ProjectPolicyChanged` 或 `ProjectMemberChanged` signal。

## 9. 并发与一致性

并发模型：

```text
一个项目 = 一个协调 Workflow
多个数字员工任务 = 并发执行
同一项目协调决策 = 串行提交
```

必须保证：

- 同一项目 route decision 不并发提交。
- 同一 ProjectTask 不重复分派。
- 成员池变更后，新任务只能使用最新可用员工池。
- 人类决策和自动规划不能同时修改同一任务。
- 所有 signal 处理后必须产生可追踪事件或明确无操作记录。

## 10. 验收标准

功能验收：

- 新建项目后能启动或注册 `project-coordinator:{project_id}`。
- 在项目详情提交需求后，Workflow 被 signal。
- Workflow 生成 RouteDecision 和 ProjectTask。
- ProjectTask 只能分派给项目数字员工池内员工。
- 数字员工完成、失败、转派请求能更新项目运行态。
- 需要人工判断时生成全局 ApprovalRequest，并在项目侧生成带 `approval_request_id` 的决策投影。
- 人类负责人处理决策后 Workflow 继续。

技术验收：

- Temporal workflow test suite 覆盖主要 signals。
- 并发 signal 测试证明同一项目协调决策串行。
- Activity 单元测试覆盖结构化规划输出校验。
- approval 模块集成测试覆盖项目决策创建、处理和回写事件。
- Go handler 测试覆盖 decision resolve 与 route decision 查询。
- 前端测试覆盖规划中、待决策、转派请求和执行完成状态。

## 11. 风险

- Workflow 内执行非确定性逻辑会导致 replay 问题。所有外部调用必须放 Activity。
- LLM 输出不校验会污染项目任务。RouteDecision 必须结构化校验。
- 如果允许数字员工绕过 Workflow 转派，会破坏审计链。
- 如果 project 模块自行实现审批事实源，会和全局 approval 模块产生逻辑分裂。项目侧只能保存审批投影和 `approval_request_id`。
- 如果 V1 修改 V0 的项目配置心智，会让项目管理变成 Workflow 后台，而不是业务项目控制台。
