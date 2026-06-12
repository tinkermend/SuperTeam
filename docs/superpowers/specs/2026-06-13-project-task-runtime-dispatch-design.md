# ProjectTask Runtime 执行分派桥接设计

日期：2026-06-13  
状态：已确认，待实现计划

## 1. 背景

Project Management V1 已经实现虚拟协调线程、RouteDecision、ProjectTask、执行摘要、转派请求和 Runtime 到 ProjectTask 的回写端点。但当前 `DispatchProjectTask` 只写项目事件并把 ProjectTask 状态从 `planned/pending` 改成 `assigned`，没有创建 `DigitalEmployeeRun`，也没有把 ProjectTask 绑定到真实 `task_runs`。

这会造成主链路断裂：

- 协调线程认为任务已分派。
- Runtime 真实执行链路没有收到命令。
- `/runtime/project-tasks/{id}/complete|fail|transfer-requests` 要求 ProjectTask 已绑定 `digital_employee_run_id`，但前向分派从未写入该字段。
- 项目事件和测试会给出“协调完成”的表象，但项目拿不到真实执行产物、证据和验收依据。

本设计补齐 ProjectTask 到数字员工执行闭环的前向桥接。

## 2. 目标

- `DispatchProjectTask` 成功时必须创建真实 `DigitalEmployeeRun`。
- 执行必须复用现有 `DigitalEmployeeRunService.CreateRun -> task_runs -> runtime command -> Runtime Agent` 主链路。
- ProjectTask 必须绑定 `digital_employee_run_id` 和 `runtime_task_id`。
- Runtime 后续通过现有 ProjectTask 写回端点完成执行摘要、失败和转派请求。
- 分派失败时不得把 ProjectTask 标成 `assigned`。
- 修复要保持 workflow/activity 可测试，不把完整 employee service 大面积耦合进 project coordination 包。

## 3. 非目标

- 不接入旧 `runtime.Poller.NotifyTask` 和 `task.Task` 长轮询模型。
- 不新增公开 Web API 或 OpenAPI 合约。
- 不引入 outbox worker、后台 dispatcher 或消息队列。
- 不重构 `DigitalEmployeeRunService` 的 runtime command 协议。
- 不改 Runtime Agent provider 执行协议。
- 不新增 `dispatch_failed` ProjectTask 状态；失败通过事件和 coordination job failure 暴露。

## 4. 方案选择

### 4.1 方案 A：协调 Activity 通过窄接口创建 run 并绑定 ProjectTask

在 `projectcoordination.ProjectStore` 注入一个小接口，例如 `ProjectTaskRunStarter`。`DispatchProjectTask` 读取项目事实，调用该接口创建 run，然后写项目事件并绑定 ProjectTask。

优点：

- 直接补上当前断点。
- 复用现有数字员工执行主链路。
- workflow/activity 只依赖窄接口，测试简单。
- 后续如果演进到 outbox，可替换该接口实现。

结论：采用。

### 4.2 方案 B：把 dispatch 全部搬进 project.Service

由 project service 直接依赖 run service 并封装 dispatch。Activity 只调用 project service。

优点是业务语义集中；缺点是当前 Temporal activity store 已经围绕 repository 组织，改造依赖注入会牵扯更大，并且 project service 与 employee run service 的边界更容易打结。

结论：不采用。

### 4.3 方案 C：异步 outbox/dispatcher 桥接

`DispatchProjectTask` 写 outbox，后台 worker 消费后创建 run 并绑定。

优点是长期可靠性更强；缺点是当前阶段成本高，需要消费幂等、重试、死信和运维面。对当前主干断裂来说过重。

结论：暂不采用。

## 5. 架构边界

新增窄接口：

```go
type ProjectTaskRunStarter interface {
    StartProjectTaskRun(ctx context.Context, req StartProjectTaskRunRequest) (StartProjectTaskRunResult, error)
}
```

建议放置：

- 接口定义放在 `projectcoordination` 包内，表达 Activity 对外部执行能力的最小需求。
- adapter 放在 Control Plane app 组装附近或独立的小 package 中，内部调用 `employee.DigitalEmployeeRunService.CreateRun`。

职责划分：

- `ProjectStore` 负责项目域事实：读取 Project、Demand、ProjectTask，写 ProjectEvent，绑定 ProjectTask。
- `ProjectTaskRunStarter` 负责执行域动作：构造并调用 `CreateRun`，返回 run 绑定信息。
- `DigitalEmployeeRunService` 继续负责 preflight、执行实例校验、预算校验、active run 冲突、创建 `task_runs`、创建 runtime command receipt 和 `ConnectionRegistry.Dispatch`。

ProjectTask 与 run 的绑定通过 repository 新增原子方法完成：

```go
BindProjectTaskRun(ctx, req BindProjectTaskRunRequest) (ProjectTask, error)
```

绑定写入：

- `digital_employee_run_id = run.ID`
- `runtime_task_id = run.TaskID`
- `latest_event_id = dispatch_event_id`
- `status = assigned`

## 6. 数据流

成功路径：

1. Workflow 调用 `DispatchProjectTask(tenantID, projectID, taskID)`。
2. Activity 读取 ProjectTask 并校验：
   - task 属于当前 project。
   - status 是 `planned` 或 `pending`。
   - `assigned_digital_employee_id` 存在。
   - `digital_employee_run_id` 为空。
3. Activity 读取 Project，使用 `human_owner_user_id` 作为 `CreateRun.UserID`。
4. Activity 读取关联 ProjectDemand，用需求标题、需求内容、任务标题和任务摘要组合 run `Objective` 与 `Prompt`。
5. Activity 调用 `ProjectTaskRunStarter.StartProjectTaskRun`。
6. adapter 调用 `DigitalEmployeeRunService.CreateRun`。
7. run service 创建 `task_runs`，生成 `run.ID`、`run.TaskID`，并通过 runtime command 推给 Runtime。
8. Activity 写 `ProjectEventTaskDispatched`。
9. Activity 调用 `BindProjectTaskRun`，绑定 run 并将 ProjectTask 状态推进到 `assigned`。
10. Runtime 后续通过现有 `/api/v1/runtime/project-tasks/{id}/complete|fail|transfer-requests` 回写项目结果。

`ProjectEventTaskDispatched` payload 至少包含：

- `project_task_id`
- `digital_employee_id`
- `digital_employee_run_id`
- `runtime_task_id`
- `runtime_node_id`
- `node_id`
- `dispatch_actor_type = project_coordinator`
- `dispatch_user_id = human_owner_user_id`

`CreateRun` 参数约定：

- `UserID` 使用项目 `human_owner_user_id`。
- `DigitalEmployeeID` 使用 ProjectTask 的 `assigned_digital_employee_id`。
- `Objective` 使用 ProjectTask 标题。
- `Prompt` 包含需求标题、需求内容、任务摘要、项目 ID、需求 ID、ProjectTask ID 和期望输出提示。
- `Metadata` 标记 `source=project_task_dispatch`、`actor_type=project_coordinator`、`project_id`、`demand_id`、`project_task_id`。
- `IdempotencyKey` 固定为 `project-task:{project_task_id}`。

## 7. 幂等和一致性

Temporal activity 可能重试，因此分派必须幂等。

规则：

- 同一个 `project-task:{project_task_id}` 重试应返回同一个 run，或允许重新调用 run service 后拿到等价 active/idempotent run。
- `BindProjectTaskRun` 允许同一个 ProjectTask 重复绑定同一个 `digital_employee_run_id` 和 `runtime_task_id`，视为成功。
- ProjectTask 已绑定不同 run 时返回冲突错误，避免一个 ProjectTask 对应多个真实执行。
- ProjectTask 处于 terminal 状态时不能绑定 run。
- 成功绑定后再次执行 `DispatchProjectTask` 应按已绑定 run 的幂等成功处理，不能创建第二个 run。

实现时应优先让 repository 的绑定方法负责原子校验，避免 service 层先读后写造成并发竞态。

## 8. 错误处理

### 8.1 前置校验失败

ProjectTask 不存在、不属于 project、缺少 assigned digital employee、状态不允许分派时：

- 不创建 run。
- 不写 `ProjectEventTaskDispatched`。
- 返回错误，让 coordination job 暴露失败。

### 8.2 run 创建或 runtime dispatch 失败

数字员工没有 ready execution instance、Runtime 未连接、已有不同 active run、预算不足或 runtime command dispatch 失败时：

- ProjectTask 保持 `planned` 或原状态。
- 写项目失败事件。
- `DispatchProjectTask` 返回错误，让 Temporal activity 和 coordination job 失败。
- 不写 `digital_employee_run_id`。

失败事件建议新增 `ProjectEventTaskDispatchFailed`。如果实现阶段为了避免迁移或前端状态扩展，也可以使用现有 project event 类型并在 payload 中明确 `dispatch_status=failed`；但事件语义必须能被测试和排查识别。

失败事件 payload 至少包含：

- `project_task_id`
- `digital_employee_id`
- `error`
- `error_family`
- `retryable`
- `dispatch_actor_type = project_coordinator`

### 8.3 run 已创建但绑定失败

这是半成功路径：

- 下一次 activity 重试用相同 idempotency key 获取同一 run，然后再次绑定。
- 如果绑定同一 run 成功或已存在同一绑定，整体视为成功。
- 如果 ProjectTask 已绑定不同 run，返回冲突错误并写失败事件。
- 如果写 dispatched 事件成功但绑定失败，短期允许重复 dispatched 事件；绑定必须保持幂等，后续可用事件幂等键优化。

## 9. Runtime 写回

本设计不改写回 API。

现有 `taskAndProjectForWriteback` 依赖：

- ProjectTask 的 `assigned_digital_employee_id` 匹配回写员工。
- ProjectTask 的 `digital_employee_run_id` 已存在。
- `project_tasks.digital_employee_run_id` 能 join 到 `task_runs.id`。
- Runtime node ID 与 run 的 `runtime_node_id` 一致。
- ProjectTask 状态是 `assigned` 或 `running`。

分派成功绑定 run 后，上述校验自然成立。Runtime 可以继续调用：

- `/api/v1/runtime/project-tasks/{id}/complete`
- `/api/v1/runtime/project-tasks/{id}/fail`
- `/api/v1/runtime/project-tasks/{id}/transfer-requests`

## 10. 测试策略

### 10.1 ProjectStore 单元测试

新增覆盖：

- 成功分派会调用 `ProjectTaskRunStarter`。
- 成功分派会写 `ProjectEventTaskDispatched`。
- 成功分派会调用 `BindProjectTaskRun`，写入 run id、runtime task id 和 latest event id。
- run 创建失败时不绑定 ProjectTask，ProjectTask 保持原状态，并写失败事件。
- 已绑定同一 run 的重试视为成功。
- 已绑定不同 run 返回冲突。

### 10.2 repository/sqlc 测试

新增 `BindProjectTaskRun` 查询测试：

- `planned/pending -> assigned`。
- 写入 `digital_employee_run_id`、`runtime_task_id`、`latest_event_id`。
- 同一 run 幂等重放成功。
- 不同 run 绑定冲突。
- terminal 状态不能绑定。

### 10.3 服务链路测试

新增一个接近真实链路的后端测试：

1. fake `ProjectTaskRunStarter` 返回 `RunID`、`RuntimeTaskID`、`RuntimeNodeID` 和 `NodeID`。
2. `DispatchProjectTask` 成功绑定 ProjectTask。
3. 调用 `CompleteProjectTask`。
4. 断言 runtime node 校验通过，并生成 execution summary 与 completed signal。

不需要真 Runtime。`DigitalEmployeeRunService.CreateRun` 的 runtime command dispatch 已有独立测试，本设计只验证 ProjectTask 到 run 的桥接。

建议验证命令：

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination ./apps/control-plane/internal/project ./apps/control-plane/internal/employee -count=1
```

如果实现阶段没有修改 OpenAPI，不需要运行 `pnpm verify:contracts`。如果只改 Control Plane 后端，不需要前端浏览器截图。

## 11. 实施顺序建议

1. 新增 ProjectTask run starter 接口和 fake 测试。
2. 新增 `BindProjectTaskRun` repository 方法和 sqlc 查询。
3. 让 `DispatchProjectTask` 在成功路径创建 run、写事件、绑定 ProjectTask。
4. 在 app container 中注入 run starter adapter。
5. 补失败事件语义和测试。
6. 跑后端定向验证。

## 12. 验收标准

- 协调线程分派 ProjectTask 后会产生真实 `DigitalEmployeeRun`。
- ProjectTask 成功分派后持有 `digital_employee_run_id` 和 `runtime_task_id`。
- Runtime ProjectTask 写回不再因为缺少 run 绑定而被拒绝。
- Runtime 不可用或 run 创建失败时 ProjectTask 不会被标成 `assigned`。
- Temporal activity 重试不会创建多个 run 或把一个 ProjectTask 绑定到不同 run。
- 新测试能在移除 run starter 调用或 ProjectTask 绑定时失败。
