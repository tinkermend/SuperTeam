# ProjectTask durable closure 设计

日期：2026-06-20  
状态：已确认，待实现计划

## 1. 背景

ProjectTask 已经成为项目协调链路中的业务任务载体。当前主干已经具备规划任务、分派任务、绑定数字员工 run、Runtime 写回完成/失败、执行摘要和项目事件等基础能力，但任务生命周期仍然缺少一个统一、持久、可恢复的闭环。

现有问题集中在四类：

- `project_tasks.status` 还混有 `assigned` 这类偏内部绑定语义，不能清楚表达“已派发但 Provider 尚未开始”的真实业务状态。
- Runtime lease 仍有 foundation-stage 痕迹，续租端点没有持久化 `lease_expires_at`、`renewed_at`、`lost_at` 等事实。
- 重试、超时、Runtime 掉线、Provider 暂态失败和人类补充上下文之间没有统一状态机。
- ProjectTask 与真实执行 attempt 的关系不够明确，导致幂等写回、失败恢复、重新规划和审计归因容易混在一起。

本设计把 `project_tasks` 作为业务任务当前状态源，把真实执行尝试拆到 `project_task_attempts`，让任务从规划、排队、执行、等待人类、重试、完成、失败、取消到重新规划都能持久化、幂等和可恢复。

## 2. 目标

- 将 `project_tasks.status` 正式收敛为：
  `planned / queued / running / waiting_human / completed / failed / cancelled`。
- 将现有 `assigned` 语义迁移为 `queued`：已分派给数字员工并创建执行 attempt，但 Provider 尚未确认开始。
- 引入 `project_task_attempts`，记录每次真实执行尝试、run 绑定、Runtime lease、超时、失败分类、幂等键和终态事实。
- 建立明确状态机和 allowed transitions，禁止终态回滚，禁止旧计划被改写成新计划。
- 支持两类恢复：
  - 同一任务契约仍成立时，在同一个 ProjectTask 下创建新 attempt。
  - 任务契约不成立、换计划、换业务路径时，旧 task 终态化并追加新的 `planned` task 或恢复子图。
- 将 `waiting_human` 建成一等暂停状态，用于人类补充上下文、审批、授权、澄清或判断是否重新规划。
- 让 Runtime 写回只认 attempt，消除旧 project-task 直写 API 的长期兼容分支。

## 3. 非目标

- 不保留旧 Runtime project-task 写回 API 的兼容语义。
- 不让 Runtime Agent 决定业务重试、等待人类或重新规划策略。
- 不把数字员工之间的自由聊天作为协作机制；协作必须落在结构化 task、attempt、event、decision/request 和 artifact 上。
- 不在本设计中实现完整前端交互，只定义状态、契约和验证要求。
- 不把 workflow memory 作为任务事实源；Temporal Workflow 只驱动协调，持久事实落在数据库和 project events。

## 4. 已确认决策

### 4.1 queued 是正式 ProjectTask 状态

采用 `planned -> queued -> running`，不继续使用 `assigned` 作为新链路状态。

原因：

- `planned` 表示上层调度已经生成任务契约，尚未进入真实执行生命周期。
- `queued` 表示任务已派发给数字员工并绑定当前 attempt，但 Provider 尚未确认开始。
- `running` 必须由 Runtime/Provider 的 started 写回推进，不能因为 Control Plane 派发成功就直接进入。

### 4.2 多次规划采用 append-only

重新规划不能改写旧计划里的任务。旧任务一旦进入 `queued/running/waiting_human/completed/failed/cancelled`，新规划只能：

- 取消仍未执行或不再成立的旧任务；
- 追加新的 `planned` task；
- 用 dependency、`retry_of_task_id`、`supersedes_task_id` 或 planner metadata 说明恢复关系。

### 4.3 重试采用混合模型

Runtime/Provider 暂态问题在同一个 ProjectTask 下创建新 attempt。需要换契约、换计划、换业务路径或 append-only 恢复时，追加新的 ProjectTask 或子图。

### 4.4 人类介入是一等暂停状态

`waiting_human` 加入正式状态集合。它不是失败，也不是终态。它表示当前 attempt 已暂停或结束，任务等待人类补上下文、审批、授权、澄清或判断是否重新规划。

人类处理后按请求类型分流：

- `missing_context`、`clarification`：同一个 task 创建新 attempt，回到 `queued`。
- `approval_required`、`permission_required`：批准后同 task 新 attempt；拒绝后按业务语义进入 `cancelled` 或 `failed`。
- `requirement_changed`、`plan_invalid`：原 task `cancelled`，协调器追加新 `planned` task 或恢复子图。
- `handoff_required`：同契约换员工可同 task 新 attempt；换契约或换计划时取消旧 task 并追加 replacement task。

### 4.5 Runtime contract 破坏式收敛到 attempt

新 Runtime Agent 只调用 attempt endpoints。旧 project-task 写回路径不保留兼容分支，避免长期双状态源。

## 5. 架构边界

`project_tasks` 是任务契约和当前业务状态源，保存稳定任务事实：

- 任务契约和期望输出；
- 当前 status；
- 当前 assignee；
- 当前 active attempt；
- 重试策略和等待人类原因；
- 终态原因和终态事件。

`project_task_attempts` 是真实执行尝试源，保存一次执行 attempt 的运行事实：

- attempt 编号；
- 绑定的数字员工 run、Runtime command/task、Runtime node；
- lease token、过期时间、续租时间、丢失时间；
- started/finished/timeout 时间；
- Provider session；
- 失败分类、是否可重试、幂等键、终态事件。

`project_events` 是审计和读模型驱动流，记录每一次关键状态转移和恢复决策。

Control Plane 负责：

- 状态机和 allowed transitions；
- retry policy；
- lease 过期后的恢复策略；
- waiting-human 处理；
- append-only recovery/replan；
- task、attempt、event、summary 的事务一致性。

Runtime Agent 负责：

- 接收 attempt 执行 payload；
- 执行本机 Provider；
- 按 attempt 写回 started、lease、complete、fail、wait-human；
- 释放本地资源和执行槽位；
- 不承载业务策略和长期业务状态。

Provider 负责：

- 执行输入；
- 产出结构化事件、结果、artifact 和错误分类原始事实；
- 不直接写业务状态。

## 6. 状态机

主路径：

```text
planned -> queued -> running -> completed
planned -> queued -> running -> failed
planned -> queued -> running -> cancelled
planned -> queued -> running -> waiting_human -> queued -> running -> completed
planned -> queued -> running -> waiting_human -> cancelled + append new planned tasks
```

允许的核心流转：

- `planned -> queued`：任务派发，创建 attempt。
- `waiting_human -> queued`：人类补充后，同 task 创建新 attempt。
- `queued -> running`：Runtime/Provider started 写回。
- `queued -> cancelled`：尚未开始时被业务取消或计划废弃。
- `running -> completed`：attempt 完成并通过任务完成契约校验。
- `running -> failed`：不可恢复执行错误。
- `running -> waiting_human`：需要人类补上下文、审批、授权、澄清或判断。
- `running -> queued`：仅通过“结束当前 attempt + 创建新 attempt”的事务表达，不能直接复用旧 attempt。
- `running -> cancelled`：业务取消、计划不成立、需求变化。

禁止的流转：

- 任意 terminal 状态回到非 terminal。
- 已经开始执行的 task 被重新写回 `planned`。
- 复用旧 attempt 从 `waiting_human` 继续执行。
- 不带当前 active attempt 的 Runtime 写回推进 task 状态。
- 不匹配 lease token、runtime node 或 idempotency key 的写回改变状态。

终态：

- `completed`：任务完成并产生可持久化结果、证据和执行摘要。
- `failed`：执行错误、能力不足或不可恢复失败。
- `cancelled`：业务废弃、计划不成立、需求变化、人类拒绝继续，或上层协调决定停止。

## 7. 数据模型

### 7.1 project_tasks 增强字段

建议新增或补强：

- `status VARCHAR(50) NOT NULL`
- `current_attempt_id UUID NULL`
- `attempt_count INT NOT NULL DEFAULT 0`
- `max_attempts INT NULL`
- `retry_not_before TIMESTAMPTZ NULL`
- `waiting_reason VARCHAR(100) NULL`
- `waiting_request_id UUID NULL`
- `terminal_reason VARCHAR(100) NULL`
- `terminal_event_id UUID NULL`
- `cancelled_by VARCHAR(100) NULL`
- `failed_by VARCHAR(100) NULL`
- `status_changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`

现有 `digital_employee_run_id`、`runtime_task_id` 如果保留，只作为历史字段或迁移辅助字段。新逻辑不得依赖它们判断当前执行事实。

### 7.2 project_task_attempts

新增表：

- `id UUID PRIMARY KEY`
- `tenant_id UUID NOT NULL`
- `project_task_id UUID NOT NULL`
- `attempt_no INT NOT NULL`
- `status VARCHAR(50) NOT NULL`
- `digital_employee_run_id UUID NULL`
- `runtime_task_id UUID NULL`
- `runtime_node_id UUID NULL`
- `provider_session_id VARCHAR(255) NULL`
- `lease_token VARCHAR(255) NOT NULL`
- `lease_expires_at TIMESTAMPTZ NULL`
- `renewed_at TIMESTAMPTZ NULL`
- `lost_at TIMESTAMPTZ NULL`
- `started_at TIMESTAMPTZ NULL`
- `finished_at TIMESTAMPTZ NULL`
- `timeout_at TIMESTAMPTZ NULL`
- `retryable BOOLEAN NULL`
- `failure_family VARCHAR(100) NULL`
- `failure_message TEXT NULL`
- `idempotency_key VARCHAR(255) NOT NULL`
- `created_event_id UUID NULL`
- `terminal_event_id UUID NULL`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`
- `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`

attempt status：

`queued / running / succeeded / failed / cancelled / lost / timed_out / waiting_human`

约束：

- `(tenant_id, project_task_id, attempt_no)` 唯一。
- `(tenant_id, idempotency_key)` 唯一。
- 一个非终态 ProjectTask 最多一个 active `current_attempt_id`。
- attempt 必须属于同租户 task。
- Runtime 写回必须匹配 `current_attempt_id`，除非是同一 terminal writeback 的幂等重放。

### 7.3 project_events

新增或规范事件类型：

- `project_task.queued`
- `project_task.started`
- `project_task.lease_renewed`
- `project_task.lease_lost`
- `project_task.retry_scheduled`
- `project_task.waiting_human`
- `project_task.human_wait_resolved`
- `project_task.completed`
- `project_task.failed`
- `project_task.cancelled`
- `project_task.replanned`

事件 payload 至少包含：

- `project_task_id`
- `attempt_id`，如果事件与 attempt 相关
- `previous_status`
- `next_status`
- `reason`
- `idempotency_key`
- `runtime_node_id`，如果来自 Runtime
- `failure_family`，如果是失败或恢复事件

## 8. Control Plane 内部接口

建议以 project service 为状态机入口，repository 只提供原子事务方法。

`QueueProjectTask`：
把 `planned/waiting_human` 推进到 `queued`，创建新 attempt，写 queued event。

`StartProjectTaskAttempt`：
校验 attempt、lease token、runtime node，把 attempt 和 task 推进到 `running`，写 started event。

`RenewProjectTaskAttemptLease`：
校验 active attempt 和 lease token，刷新 `lease_expires_at`、`renewed_at`，必要时写低频 lease event 或只更新 attempt。

`CompleteProjectTaskAttempt`：
幂等完成 attempt、task、event、summary 和 demand/project read model。完成契约不满足时进入 waiting-human 或拒绝写回，不能伪完成。

`FailProjectTaskAttempt`：
记录 attempt 失败，按错误分类和 retry policy 决定：

- 新 attempt + task 回 `queued`；
- task 进 `waiting_human`；
- task 进 `failed`；
- task 进 `cancelled`。

`CancelProjectTask`：
业务取消 task，必要时中止当前 attempt，写 cancelled event。

`ResolveProjectTaskHumanWait`：
人类处理等待项，结果必须是：

- `resume_same_task`
- `cancel_and_replan`
- `cancel_without_replan`
- `mark_failed`

## 9. Runtime API

删除旧语义：

- `POST /api/v1/runtime/project-tasks/{projectTaskId}/complete`
- `POST /api/v1/runtime/project-tasks/{projectTaskId}/fail`
- `POST /api/v1/runtime/project-tasks/{projectTaskId}/transfer-requests`

新增 attempt-aware API：

- `POST /api/v1/runtime/project-task-attempts/{attemptId}/started`
- `POST /api/v1/runtime/project-task-attempts/{attemptId}/lease`
- `POST /api/v1/runtime/project-task-attempts/{attemptId}/complete`
- `POST /api/v1/runtime/project-task-attempts/{attemptId}/fail`
- `POST /api/v1/runtime/project-task-attempts/{attemptId}/wait-human`

所有 Runtime 写回必须包含：

- `project_task_id`
- `attempt_id`
- `lease_token`
- `runtime_node_id`
- `idempotency_key`
- 当前 Provider session 或 command id，若存在

Control Plane 必须拒绝：

- 不属于当前 active attempt 的非幂等写回；
- lease token 不匹配的写回；
- runtime node 不匹配的写回；
- 已终态 task 的非幂等状态推进；
- 不满足完成契约的 complete 写回。

Runtime claim/dispatch payload 必须包含：

- `project_task_id`
- `attempt_id`
- `lease_token`
- `lease_expires_at`
- 任务契约和 expected outputs；
- 需要写回的 Runtime endpoint base。

## 10. 错误分类与恢复策略

Control Plane 负责错误分类后的策略决策。Runtime 只上报结构化事实。

`transient_runtime`：
Runtime 掉线、节点重启、短暂网络错误、lease 续租失败。当前 attempt 进入 `lost` 或 `failed`。未超 retry policy 时同 task 新 attempt；超出后进 `waiting_human`。

`transient_provider`：
Provider 限流、临时不可用、会话启动失败但可重试。策略同 `transient_runtime`。

`timeout`：
attempt 超过执行窗口。当前 attempt `timed_out`。允许自动重试则同 task 新 attempt；否则进入 `waiting_human`。

`invalid_contract`：
任务契约缺必要输入、期望输出不可满足、上下文不足。task 进入 `waiting_human`，请求类型为 `missing_context`、`clarification` 或 `plan_invalid`。

`approval_required` / `permission_required`：
task 进入 `waiting_human`。批准后同 task 新 attempt；拒绝后按业务语义 `cancelled` 或 `failed`。

`non_retryable_execution`：
数字员工能力不足、Provider 明确不支持、输出无法产生且不是上下文缺失。task `failed`，协调器可追加 reassign/rework 子图。

`business_cancelled` / `plan_invalid` / `requirement_changed`：
task `cancelled`，上层追加新计划或终止下游。

## 11. 幂等与并发

每个外部写入都必须有幂等键。

派发幂等键：

`project-task:{project_task_id}:attempt:{attempt_no}:queue`

Runtime 写回幂等键：

`project-task-attempt:{attempt_id}:{action}:{runtime_command_id_or_provider_event_id}`

规则：

- 重复 started 只返回同一 running attempt。
- 重复 lease 只刷新或返回当前 lease 状态，不写重复业务事件。
- 重复 complete/fail/wait-human 只返回第一次 terminal 结果，不重复 event、summary 或 transfer/decision request。
- complete 与 fail 并发时，只有一个 terminal writeback 成功；另一个必须识别为冲突或幂等重放。
- 新 attempt 创建和 `project_tasks.current_attempt_id` 更新必须在同一事务中完成。
- terminal task 禁止创建新 attempt。

## 12. 人类 loop

数字员工发现需求结论不明确、上下文不足、需要审批或需要授权时，不直接失败，也不继续占用 Runtime slot。

流程：

1. Runtime 调用 `wait-human`，带请求类型、原因、缺失上下文引用和建议选项。
2. Control Plane 将当前 attempt 标记为 `waiting_human` 或 terminalized pause 状态，并释放 lease。
3. `project_tasks.status` 进入 `waiting_human`。
4. 创建人类决策、补充上下文或 transfer/clarification 请求。
5. 人类处理后，Control Plane 根据结果：
   - 同 task 创建新 attempt，回到 `queued`；
   - `cancelled + append new planned tasks`；
   - `cancelled`；
   - `failed`。

这个 loop 是受控的项目协调 loop，不是自由聊天。所有输入、结论、证据和恢复动作都必须结构化持久化。

## 13. 迁移和破坏式收敛

本设计接受 breaking change，优先降低长期复杂度。

迁移策略：

1. 新增状态机、attempt 表、repository 事务方法和服务层接口。
2. 将 dispatch 成功后的状态从 `assigned` 改为 `queued`，并创建当前 attempt。
3. Runtime Agent 改为只使用 attempt endpoints。
4. 删除旧 project-task 写回 handler、OpenAPI path、客户端调用和测试。
5. 更新读模型和 operational status 逻辑，让 queued 来自 `project_tasks.status='queued'` 和 active attempt，而不是旧 assigned 口径。
6. 清理不再作为新链路事实源的 `project_tasks.digital_employee_run_id/runtime_task_id` 依赖。

历史数据处理：

- 迁移可以将非终态 `assigned` 映射为 `queued`。
- 已终态 task 保持终态，不回填 attempt 除非需要历史审计。
- 如果存在无法可靠映射的 active task，应由迁移脚本标记为 `waiting_human` 或 `cancelled`，并写迁移审计事件。

## 14. 测试与验收

### 14.1 状态机测试

- 合法流转通过。
- 非法流转拒绝。
- terminal 状态不可回滚。
- `waiting_human` 的恢复分支正确。
- `assigned` 不再作为新状态使用。

### 14.2 Repository/sqlc 测试

- 创建 attempt 并原子更新 `current_attempt_id`。
- active attempt 唯一。
- lease token 校验。
- started/complete/fail/wait-human 幂等。
- complete 与 fail 并发只有一个成功。
- event 和 summary 不重复写入。

### 14.3 Control Plane 服务测试

- dispatch 生成 `queued + current attempt`。
- started 推进 `running`。
- complete 原子写 task、attempt、event、summary。
- fail 根据错误分类进入 retry、waiting_human、failed 或 cancelled。
- 人类补充上下文后同 task 新 attempt。
- plan invalid 后旧 task cancelled 并追加新 planned 子图。

### 14.4 Runtime Agent 测试

- 执行 payload 使用 attempt id 和 lease token。
- started/lease/complete/fail/wait-human 全部调用 attempt endpoints。
- 旧 project-task 写回路径不存在。
- lease 失败时本地任务取消，并向 Control Plane 写回可分类事实。

### 14.5 契约和真实链路验证

- OpenAPI 删除旧 path，新增 attempt path。
- 生成代码无漂移。
- `corepack pnpm verify:contracts` 通过。
- Control Plane targeted Go tests 通过。
- Runtime Agent targeted Rust tests 通过。
- 真实 smoke：
  1. 创建项目需求；
  2. planner 生成 `planned` task；
  3. dispatch 创建 `queued` task 和 attempt；
  4. Runtime started 推进 `running`；
  5. lease renew 写入持久 lease；
  6. complete 写回；
  7. API 读回 task、attempt、event、summary 和项目读模型。

## 15. 实施边界

建议拆成三阶段计划：

第一阶段：Control Plane 数据模型、状态机、attempt repository/service、OpenAPI breaking contract。

第二阶段：Runtime Agent 调用 attempt endpoints，并删除旧 project-task 写回调用。

第三阶段：恢复策略、waiting-human loop、真实链路 smoke、读模型和 operational status 收敛。

每阶段都必须保持生成代码、契约验证和针对性测试通过。最终不能把 mock、单元测试或构建通过表述为真实链路已验证；只有真实 Web/Control Plane/DB/Runtime/Provider 路径跑通后，才能声明执行链路可用。
