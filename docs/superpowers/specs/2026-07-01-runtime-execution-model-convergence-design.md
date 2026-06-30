# Runtime execution model convergence 设计

日期：2026-07-01
状态：已确认，待实现计划

## 1. 背景

当前主干里存在三条与“执行任务”相关的路径：

- Runtime Agent legacy polling：`TaskExecutor -> /api/v1/runtime/tasks/claim -> execute_task -> /runtime/tasks/{id}/events|complete|fail|lease`。
- DigitalEmployee Run：Control Plane 创建 `tasks + task_runs + runtime_command_receipts`，通过 Runtime WebSocket 下发 `start_session`，Runtime Agent 执行 Provider 并写回 runtime command terminal 状态。
- ProjectTaskAttempt：Project coordinator 通过 DigitalEmployee Run 下发 `start_session`，在 metadata 中携带 `project_task_id`、`project_task_attempt_id`、`lease_token`，Runtime Agent 再调用 `/api/v1/runtime/project-task-attempts/{attemptId}/started|complete|fail|wait-human`。

这三条路径不是平等的业务模型。现有系统的主方向已经是 command-driven Runtime execution 和 attempt-scoped ProjectTask closure。legacy polling 路径仍默认运行，但它已经和 UUID-first Control Plane contract 脱节：

- Rust `apps/runtime-agent/src/controlplane/models.rs` 中 `Task.id` 仍是 `i64`。
- Control Plane `tasks.id`、OpenAPI `Task.id`、runtime task path parameter 均是 UUID。
- `/api/v1/runtime/tasks/claim` 成功返回 UUID 字符串后，Rust client 会按 `i64` 反序列化失败。
- legacy `update_task_status` 打到 `/api/v1/tasks/{id}/status`，这是 Console task route，不是 Runtime writeback route。

因此下一步不是继续补兼容，而是把执行入口收敛为单一路径，并下线 legacy polling。

## 2. 目标

- Runtime Agent 默认不再启动 legacy `TaskExecutor` polling、execution 和 lease renewal loops。
- ProjectTask 真实执行只允许通过 `DigitalEmployeeRunService.CreateRun -> runtime command start_session -> ProjectTaskAttempt writeback` 闭环。
- `tasks/task_runs` 保留为 DigitalEmployee Run 的底层执行记录，不再作为独立 runtime long-poll 任务源。
- `project_tasks` 是项目业务任务状态权威源。
- `project_task_attempts` 是项目任务执行尝试、租约和 Runtime 写回权威源。
- `task_runs` 是数字员工运行状态源。
- `runtime_command_receipts` 是 Runtime 命令投递和回执状态源。
- 明确 deprecate `/api/v1/runtime/tasks/*` legacy endpoints，避免新开发继续接入。
- 新增测试证明普通 pending task 不会被当前 Runtime Agent 默认 claim 后卡死。
- 建立真实链路 smoke，证明 ProjectTask dispatch 到 attempt terminal writeback 可用。

## 3. 非目标

- 不重写 ProjectTask durable closure 设计；本设计只做执行入口收敛和 legacy polling 下线。
- 不删除 `tasks` 或 `task_runs` 表；它们仍承载 DigitalEmployee Run。
- 不把 Control Plane 直接改成本地命令执行器。
- 不让 Runtime Agent 负责业务重试、人类审批或项目状态策略。
- 不在本阶段新增消息队列、outbox worker 或新的 Provider 协议。
- 不为了兼容 legacy polling 把 Rust `Task.id` 改成 UUID 并继续维护两套执行循环。

## 4. 方案选择

### 4.1 推荐方案：下线 Runtime Agent legacy polling，保留端点为兼容外壳

Runtime Agent 删除或禁用默认 `TaskExecutor` 启动，只保留 Runtime WebSocket command loop、heartbeat、capability upsert 和 local run store。

Control Plane 暂时保留 `/api/v1/runtime/tasks/*` endpoints，但将它们标记 deprecated，并加 guard：

- `/api/v1/runtime/tasks/claim` 不再派发 command-driven task。
- 对普通 legacy pending task，短期返回 `204 No Content` 或受配置开关控制。
- writeback endpoints 只保留到清理期，不作为新链路使用。

优点：

- 最小化当前运行风险，避免 Runtime Agent claim UUID task 后卡死。
- 不破坏已经基于 `task_runs` 的 DigitalEmployee Run 数据模型。
- 强制新开发只看 command-driven 和 attempt-scoped 路径。
- 后续删除 legacy endpoint 时范围清晰。

缺点：

- 需要清理部分测试和文档。
- 如果仍有手工创建普通 `tasks` 并期待 runtime claim 的旧功能，会变成 no-op，需要显式迁移到 DigitalEmployee Run 或删除入口。

### 4.2 不推荐：把 legacy polling 迁到 UUID 并继续运行

把 Rust `Task.id` 改成 `String` 或 UUID，修正 URL 和 response 解析，让 legacy polling 继续能执行。

不推荐原因：

- 会延长两套执行模型寿命。
- `tasks.status`、`task_runs.status`、`project_task_attempts.status` 的推进仍会分裂。
- 新开发仍可能误接 `/runtime/tasks/*`，继续绕开 RuntimeCommand receipt、ProjectTaskAttempt lease 和 project events。
- 只能修类型错误，不能修架构分裂。

### 4.3 不推荐：立即删除所有 `/runtime/tasks/*` endpoints

直接删除 Control Plane route、OpenAPI contract、Runtime client 和测试。

不推荐原因：

- 当前代码和测试仍引用这些 endpoints，直接删除会扩大改动面。
- 如果本地或旧自动化仍探测这些 endpoints，会产生一次性破坏。
- 更适合作为第二阶段，在确认无消费者后执行。

## 5. 目标架构

执行主链路统一为：

```text
ProjectTask planned
  -> Project coordinator DispatchProjectTask
  -> DigitalEmployeeRunService.CreateRun
  -> tasks row + task_runs row + runtime_command_receipts row
  -> Runtime WebSocket start_session
  -> RuntimeCommandExecutor starts Provider run
  -> ProjectTaskAttempt started writeback
  -> runtime command events and terminal writeback
  -> ProjectTaskAttempt complete/fail/wait-human writeback
  -> project_tasks status and project_events update
```

状态权威：

- `project_tasks.status`：业务任务当前状态，供项目、需求、DAG、验收和下游依赖判断使用。
- `project_task_attempts.status`：某次真实执行尝试状态，供 lease、超时、重试和 Runtime writeback 幂等使用。
- `task_runs.status`：数字员工 run 状态，供员工工作台、Provider session 和运行记录使用。
- `runtime_command_receipts.status`：Runtime command 投递、接收和终态回执状态，供 Runtime 连接、重发、排查和审计使用。

`tasks.status` 的定位收窄：

- `tasks` 是 DigitalEmployee Run 的底层 task envelope。
- 新执行链路不得直接通过 `/runtime/tasks/claim` 消费 `tasks`。
- `tasks.status` 不作为 ProjectTask 或 Runtime command 的权威状态。

## 6. 具体设计

### 6.1 Runtime Agent 默认禁用 legacy TaskExecutor

在 `RuntimeDaemon.run()` 中移除或配置化以下默认启动：

- `TaskExecutor::new(...)`
- `executor.run().await`
- `polling_loop`
- `execution_loop`
- `lease_renewal_loop`

Runtime Agent 主循环只保留：

- enrollment/session auth；
- capability upsert；
- Runtime command WebSocket loop；
- heartbeat loop；
- Runtime command execution；
- local run HTTP smoke server，若当前配置已启用。

如果为了渐进迁移保留代码，必须满足：

- 默认配置关闭。
- 开关名称带 legacy 语义，例如 `runtime.enable_legacy_task_polling=false`。
- 测试证明默认不会请求 `/api/v1/runtime/tasks/claim`。
- 日志明确提示该模式 deprecated。

### 6.2 Control Plane legacy claim guard

`RuntimeHandler.ClaimTask` 当前会扫描 pending task 并派发普通 task。实现阶段需要改成以下语义之一：

推荐短期语义：

- 如果未启用 legacy compatibility，直接返回 `204 No Content`。
- 如果启用 compatibility，仍必须只允许显式 legacy task，并且不得 claim `provider_run_protocol=provider-run/v1` 的 command-driven task。

显式 legacy task 判定建议：

- `params.legacy_runtime_polling == true`；
- 或系统配置 `runtime.allow_legacy_task_claim=true` 且任务不含 `provider_run_protocol`。

默认不得把普通 `tasks` 当成可 claim 工作。

### 6.3 Runtime task endpoints deprecation

OpenAPI 和 handler 注释标记 deprecated：

- `POST /api/v1/runtime/tasks/claim`
- `POST /api/v1/runtime/tasks/{taskId}/events`
- `POST /api/v1/runtime/tasks/{taskId}/complete`
- `POST /api/v1/runtime/tasks/{taskId}/fail`
- `POST /api/v1/runtime/tasks/{taskId}/lease`

文档中写明替代路径：

- 数字员工执行：`/api/v1/digital-employees/{id}/runs` 或内部 `DigitalEmployeeRunService.CreateRun`。
- Runtime command writeback：`/api/v1/runtime/commands/{commandId}/events|complete|fail|cancelled|timed-out`。
- 项目任务执行：`/api/v1/runtime/project-task-attempts/{attemptId}/started|lease|complete|result|fail|wait-human`。

### 6.4 ProjectTask dispatch 保持 attempt-scoped writeback

Project coordinator dispatch 必须继续构造 `start_session` metadata：

- `source = project_task_dispatch`
- `project_id`
- `demand_id`
- `project_task_id`
- `project_task_attempt_id`
- `project_task_lease_token`
- `runtime_node_id`
- `handoff_contract.completion_path = project_task_attempt_writeback`
- `expected_outputs`
- `input_requirements`

Runtime Agent 只从这组 metadata 构造 `ProjectTaskWritebackContext`，并只调用 attempt endpoints。

没有 attempt metadata 的普通 DigitalEmployee Run 只更新 runtime command 和 task run，不得尝试推进 ProjectTask。

### 6.5 状态同步规则

Runtime Agent Provider run 开始：

- 先记录本地 run。
- 若存在 `ProjectTaskWritebackContext`，调用 attempt started。
- started 写回失败时，本地 run 应失败，runtime command 应 fail，并尝试 project task fail writeback。

Provider run 完成：

- runtime command terminal writeback 必须先写。
- 若存在 `ProjectTaskWritebackContext`：
  - 结果要求人类介入时，调用 attempt wait-human。
  - 否则调用 attempt complete。
- ProjectTask final status 由 Control Plane service 和 repository 原子推进，不由 Runtime Agent 直接决定。

Provider run 失败或取消：

- runtime command fail/cancel writeback。
- 若存在 `ProjectTaskWritebackContext`，调用 attempt fail。
- 失败分类和 retryable 只作为事实输入；重试策略由 Control Plane 决定。

### 6.6 数据迁移和兼容窗口

本阶段不需要 schema migration。

需要一次数据风险检查：

- 查询非 terminal 的普通 `tasks`，排除 `params.provider_run_protocol='provider-run/v1'` 的 DigitalEmployee Run envelope。
- 如果存在旧 legacy pending/claimed/running task，输出迁移报告：
  - 可删除；
  - 可转成 DigitalEmployee Run；
  - 可手工标记 cancelled/failed；
  - 或临时打开 legacy compatibility 处理。

不自动迁移历史数据，避免误改业务事实。

## 7. 错误处理

- Runtime WebSocket 不可用：DigitalEmployee Run 保持 queued/dispatching，command receipt 记录 failed 或 pending，ProjectTask 不进入 running。
- attempt started 写回失败：本地 run fail，runtime command fail，并记录 project-task fail best-effort。
- attempt terminal writeback 失败：runtime command 已 terminal 时不得回滚；记录错误日志和 runtime event，Control Plane 后续通过 command receipt 和 attempt liveness 进行补偿或人工处理。
- legacy claim 被调用：默认 `204`，并写低噪声 deprecated metric/log，不 claim 任何 task。
- 普通 `tasks` 创建 API 仍可用于 Console/测试，但不得暗示 Runtime Agent 会自动消费。

## 8. 测试策略

### 8.1 Rust Runtime Agent

新增或调整测试：

- `RuntimeDaemon` 默认不发起 `/api/v1/runtime/tasks/claim`。
- WebSocket command loop 仍能处理 `start_session`。
- `task.claim` WS command 继续被识别为 unsupported。
- `RuntimeCommandExecutor` 对 project_task_dispatch metadata 调用 attempt started 和 terminal writeback。
- 若保留 legacy 开关，开启开关时才允许 claim loop 启动。

### 8.2 Go Control Plane

新增或调整测试：

- `/api/v1/runtime/tasks/claim` 默认返回 `204`，不调用 `AssignTask`。
- command-driven task 永远不会被 claim。
- 显式 legacy compatibility 关闭时，普通 pending task 不会被 claim。
- ProjectTask dispatch 仍创建 DigitalEmployee Run、ProjectTaskAttempt，并写入 runtime command metadata。
- attempt started/complete/fail/wait-human endpoints 保持现有状态机和幂等校验。

### 8.3 Contract

- OpenAPI 标记 legacy runtime task endpoints deprecated。
- `pnpm verify:contracts` 必须通过。
- Rust client 不再依赖 OpenAPI 中 legacy `Task` response；如仍保留 client 方法，只能在 legacy feature/test 中使用。

### 8.4 真实链路 smoke

完成实现后必须做一次真实链路验证：

1. 确认当前 Control Plane、Runtime Agent、数据库运行的是最新代码。
2. 创建或选择一个 runtime-ready digital employee。
3. 创建项目和需求，触发 Project coordinator dispatch。
4. 确认 Runtime Agent 通过 WS 收到 `start_session`，没有调用 `/runtime/tasks/claim`。
5. 确认 ProjectTaskAttempt 进入 `running`。
6. 让 Provider 返回可完成结果。
7. 确认：
   - `runtime_command_receipts.status` terminal；
   - `task_runs.status` terminal；
   - `project_task_attempts.status` terminal；
   - `project_tasks.status` terminal 或 `waiting_human`；
   - `project_events` 有 dispatched、started/terminal 对应事件。

如果真实 Provider 或安全 workspace 不可用，只能声明局部验证，不能声明执行链路可用。

## 9. 实施阶段

### 阶段 1：默认止血

- Runtime Agent 默认禁用 legacy TaskExecutor。
- Control Plane `/runtime/tasks/claim` 默认 no-op。
- 增加回归测试防止再次默认 claim。

完成标准：启动 Runtime Agent 不会请求 `/api/v1/runtime/tasks/claim`，ProjectTask command-driven path 不受影响。

### 阶段 2：契约标记和测试清理

- OpenAPI 标记 deprecated。
- 调整依赖 claim 行为的测试，明确 legacy compatibility。
- 清理文档中把 `/runtime/tasks/*` 当主执行入口的描述。

完成标准：新开发从 docs/contracts 只能看到 command-driven path 是主路径。

### 阶段 3：代码删除

- 删除 Rust legacy `Task` model、client 方法、executor loops 和相关测试。
- 删除或收口 Control Plane legacy task runtime endpoints。
- 若仍保留 Console task CRUD，明确其不被 Runtime Agent 自动执行。

完成标准：仓库中没有默认可运行的 legacy polling execution path。

## 10. 风险和决策点

- 如果还有真实用户依赖普通 `tasks` long-poll 执行，阶段 1 会让这类任务停止自动执行。实施前需要用数据库查询和代码搜索确认消费者。
- 如果选择保留 compatibility 开关，必须设置到默认关闭，并限制为短期迁移工具，不能成为新的支持路径。
- ProjectTask 完成链路的真实验证依赖 Runtime Agent、Provider binary、认证、数据库和安全 workspace；实现完成后如果这些依赖不满足，不能声明功能可用。
- 删除 legacy endpoint 前，需要确认 OpenAPI 客户端、测试、脚本和自动化没有硬依赖。

## 11. 成功判定

- Runtime Agent 默认执行入口只有 Runtime command WebSocket。
- ProjectTask 的执行开始、续租、完成、失败和等待人类都只通过 ProjectTaskAttempt endpoints。
- 普通 `tasks` 不会被 Runtime Agent 默认 claim。
- 新测试覆盖 legacy no-op 和 command-driven success path。
- 真实链路 smoke 能证明 ProjectTask dispatch 到 Provider terminal writeback 闭环。
