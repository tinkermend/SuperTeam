# ProjectTask dispatch/runtime recovery 设计

日期：2026-07-02
> 复核状态：CHANGELOG 2026-07-02 17:57记录ProjectTask dispatch/runtime recovery首版；锚点抽查发现project_task_attempts与recovery相关数据库结构
状态：已确认，待实现计划

## 1. 背景

当前 `DispatchProjectTask` 已经把项目任务分派桥接到真实 `DigitalEmployeeRun` 链路，并通过 `QueueProjectTaskWithAttempt` 创建 `project_task_attempts`。但 dispatch 失败和 Runtime 启动失败后的恢复闭环仍不完整。

当前行为：

- run start 或 queue attempt 失败时，`DispatchProjectTask` 写 `project_task.dispatch_failed`。
- `dispatchProjectTasks` 看到失败已记录后继续执行，避免 Project Coordinator workflow 因单个任务分派失败而死亡。
- task 通常保持在 `planned` 或原状态附近；如果失败发生在 queued/running 后，attempt 状态依赖 Runtime 后续写回或已有 lease/started 逻辑。

这个设计能保护 workflow，但没有把“失败已记录”推进成下一步恢复动作。结果是任务可能只靠事件暴露，缺少自动 retry/backoff、超时回收、人工决策入口和可观测 next action。

本设计补齐 Control Plane 侧的恢复决策层，首版覆盖：

- dispatch 已失败且未创建 attempt；
- queued attempt 长时间未收到 Runtime `/started`；
- running attempt lease lost；
- Provider/session 启动失败。

## 2. 目标

- 保持 `DispatchProjectTask` 的当前边界：负责 gate、创建 run、queue attempt 和写 dispatch facts，不在 activity 内做长等待恢复。
- 引入 `ProjectTaskRecoveryService`，把 dispatch/runtime/provider 启动类失败推进为可恢复状态。
- 对可重试失败写入 backoff 和 `retry_not_before`，到期后重新 dispatch。
- 对不可自动判断或不可重试失败，将 ProjectTask 推进到 `waiting_human`，创建人工恢复决策。
- 对 retry policy 耗尽的任务进入 `waiting_human` 或明确终态 `failed`，不静默停留在 `planned/queued/running`。
- 对 queued 未 started 和 running lease lost 的 attempt 进行终态化，释放 active attempt 槽位。
- 让项目详情和执行 trace 能显示 next action：等待重试、等待人工、已失败、或无动作。

## 3. 非目标

- 不在本阶段重做 result contract、验收、revision loop 或完整 replan engine。
- 不新增 `dispatch_failed` ProjectTask 主状态；`project_task.dispatch_failed` 仍是事件事实。
- 不让 Runtime Agent 决定业务 retry、waiting-human 或 replan 策略。
- 不让 workflow 在 dispatch activity 内无限阻塞或 sleep 到 retry 时间。
- 不实现复杂恢复工作台；首版复用现有 Inbox/decision 体系。
- 不改变 ProjectTask 的 append-only replan 原则。

## 4. 已确认方案

采用 Control Plane `ProjectTaskRecoveryService`，由 workflow activity 和后台 sweeper 触发。

不采用直接在 `DispatchProjectTask` 内部循环 retry 的方案，因为它只能覆盖 run start/queue 阶段，不能统一处理 queued 未 started、lease lost 和 Provider 启动失败。

不采用所有失败都转人工的方案，因为 Runtime 临时离线、Provider 临时不可用和 slot 暂时不足应优先自动恢复，避免制造人工噪音。

## 5. 架构边界

`DispatchProjectTask` 负责：

- 执行 pre-dispatch gate；
- 调用 `ProjectTaskRunStarter` 创建 `DigitalEmployeeRun`；
- 调用 `QueueProjectTaskWithAttempt` 绑定 attempt/run/runtime facts；
- 写 `project_task.dispatched` 或 `project_task.dispatch_failed`；
- 对已经记录的失败返回可识别错误，让 workflow 继续。

`ProjectTaskRecoveryService` 负责：

- 读取 task、attempt、event、dispatch gate、run/runtime facts；
- 分类失败；
- 应用 retry policy 和 backoff；
- 原子推进 task/attempt 状态；
- 写恢复事件；
- 必要时创建人工 recovery decision/inbox。

Project Coordinator workflow 负责：

- dispatch 失败已记录时继续处理其他 task；
- 在 dispatch batch 后调用 recovery activity，或由 recovery sweeper 补偿；
- 不把 workflow memory 当作恢复事实源。

Runtime Agent 负责：

- 写回 attempt started、lease、complete、fail、wait-human；
- 上报 Provider/session 启动失败的结构化事实；
- 不持有业务恢复策略。

## 6. 核心状态流

### 6.1 Dispatch 失败且未创建 attempt

输入事实：

- task 处于 `planned` 或已被人类恢复后的 `waiting_human`；
- 无 active attempt；
- 存在最新 `project_task.dispatch_failed`；
- event payload 带 `retryable`、错误原因和错误分类线索。

状态推进：

- `retryable=true` 且未超过 retry policy：保持 task 可调度，写 `retry_not_before`，写 `project_task.retry_scheduled`。
- `retryable=false` 或缺少配置、权限、契约输入：task -> `waiting_human`，`waiting_reason=dispatch_recovery_required`。
- 明确不可恢复且策略要求终态：task -> `failed`，写 terminal event。

### 6.2 Queued attempt 未收到 started

输入事实：

- task.status = `queued`；
- current attempt.status = `queued`；
- `started_at IS NULL`；
- queued 时间或 deadline 已超过 `start_deadline`。

状态推进：

- attempt -> `lost` 或 `timed_out`，`failure_family=runtime_start_timeout`。
- task 清理 active attempt 或进入 retry scheduling 状态。
- retry policy 允许时写 `retry_not_before` 和 `project_task.retry_scheduled`。
- retry policy 不允许时 task -> `waiting_human`，`waiting_reason=runtime_recovery_required`。

### 6.3 Running attempt lease lost

输入事实：

- task.status = `running`；
- attempt.status = `running`；
- `lease_expires_at < now` 或 Runtime node 已不可用；
- 没有 terminal writeback。

状态推进：

- attempt -> `lost`，写 attempt lost event。
- retry policy 允许时同 task 安排新 attempt。
- retry policy 耗尽时 task -> `waiting_human`，由人类选择 retry/reassign/cancel/replan。
- 只有明确业务策略要求时才直接 task -> `failed`。

### 6.4 Provider/session 启动失败

输入事实：

- Runtime command 或 run/attempt 写回 Provider/session 启动失败；
- 失败可归类为 transient provider、provider configuration、permission required 或 non-retryable provider start。

状态推进：

- `transient_provider_start`：自动 retry/backoff。
- `provider_configuration`：task -> `waiting_human`，请求配置修复或换 Provider。
- `permission_required`：task -> `waiting_human`，请求授权。
- retry policy 耗尽：task -> `waiting_human`，请求人工恢复决策。

## 7. Recovery 决策模型

建议内部统一结构：

```go
type ProjectTaskRecoveryAction struct {
    Action        string
    FailureFamily string
    Retryable     bool
    RetryNotBefore *time.Time
    WaitingReason *string
    DecisionType  *string
    TerminalReason *string
}
```

`Action` 首版取值：

- `no_op`：已有更新或事实不足，不重复推进。
- `retry_scheduled`：写 backoff，等待重新 dispatch。
- `waiting_human`：创建人工恢复决策。
- `failed`：进入终态失败。

失败分类首版：

- `dispatch_transient`
- `runtime_start_timeout`
- `runtime_lease_lost`
- `transient_provider_start`
- `provider_configuration`
- `permission_required`
- `invalid_dispatch_contract`
- `retry_policy_exhausted`

## 8. 数据和 Repository 需求

首版优先复用现有字段：

- `project_tasks.retry_not_before`
- `project_tasks.max_attempts`
- `project_tasks.attempt_count`
- `project_tasks.waiting_reason`
- `project_tasks.waiting_request_id`
- `project_tasks.current_attempt_id`
- `project_task_attempts.status`
- `project_task_attempts.retryable`
- `project_task_attempts.failure_family`
- `project_task_attempts.failure_message`
- `project_task_attempts.lost_at`
- `project_task_attempts.timeout_at`
- `project_events`

需要新增或补齐 repository 能力：

- 读取 task 最新 `project_task.dispatch_failed`。
- 查找 stale queued attempts。
- 查找 lease expired running attempts。
- 原子 terminalize attempt 并推进 task recovery 状态。
- 原子写 `retry_not_before` 和 `project_task.retry_scheduled`。
- 原子创建 waiting-human recovery decision 并更新 task。

如现有 event 类型不足，新增：

- `project_task.retry_scheduled`
- `project_task.attempt_lost`
- `project_task.recovery_requested`

## 9. Workflow 和 Sweeper 触发

Workflow activity：

- `RecoverTaskDispatchFailure(tenantID, projectID, taskID, eventID)`。
- `dispatchProjectTasks` 继续在 dispatch failure recorded 时不中断 batch。
- batch 后对失败任务调用 recovery activity。

后台 sweeper：

- 周期性调用 `SweepStaleQueuedAttempts(now)`。
- 周期性调用 `SweepLostRunningAttempts(now)`。
- 每次扫描必须限流、分页、按 tenant/project/task 加幂等锁或使用数据库条件更新避免重复推进。

重新 dispatch：

- 不在 recovery service 中直接调用 `DispatchProjectTask`。
- 到期 retry 由 coordinator workflow、sweeper 或 ready-task 调度 activity 统一触发。
- 重复触发必须通过 task 状态、active attempt 和 idempotency key 保证不重复创建 run/attempt。

## 10. 人工恢复决策

复用现有 decision/inbox 体系，新增或扩展 decision type：

- `project_task_recovery`

首版 actions：

- `retry`：清理 waiting 状态，按同 task 创建新 attempt。
- `cancel`：task -> `cancelled`。

预留 actions：

- `reassign`
- `replan`

Decision payload 必须包含：

- project_id；
- project_task_id；
- latest_attempt_id；
- failure_family；
- failure_message；
- retryable；
- suggested_action；
- allowed_actions；
- evidence event IDs。

## 11. 可观测性和前端边界

首版不做独立恢复工作台。项目执行详情和 trace 需要展示：

- 最近失败分类和摘要；
- 是否已安排重试；
- `retry_not_before`；
- 是否等待人工；
- waiting reason；
- 最新 recovery decision 状态。

Operational read model 的 next action 应能区分：

- `retry_scheduled`
- `waiting_human`
- `stale_queued`
- `lease_lost`
- `terminal_failed`

## 12. 幂等性要求

- 同一个 `project_task.dispatch_failed` event 只能生成一次 recovery action。
- 同一个 stale queued attempt 只能 terminalize 一次。
- 同一个 running attempt lease lost 只能标记 lost 一次。
- 自动 retry 不能重复创建 run/attempt；仍使用现有 dispatch idempotency key 和 active attempt 约束。
- waiting-human decision 不得因 sweeper 重复扫描而重复创建。
- workflow replay、activity retry、sweeper 并发必须只产生同一组事实。

## 13. 测试计划

后端单测：

- dispatch failed `retryable=true` -> `retry_not_before` + `project_task.retry_scheduled`。
- dispatch failed `retryable=false` -> task `waiting_human` + recovery decision。
- retry policy exhausted -> `waiting_human` 或 `failed`，按策略断言。
- queued stale -> attempt `lost/timed_out` + task retry scheduled。
- running lease expired -> attempt `lost` + task retry scheduled。
- Provider transient start failure -> retry scheduled。
- Provider configuration failure -> waiting human。
- 重复 recovery 调用不重复写 event/decision。

Workflow test：

- dispatch failure recorded 后 workflow 不死，且 recovery activity 被调用。
- recovery activity 失败时按 Temporal retry 语义处理，不能吞掉未记录失败。

Repository/Postgres 测试：

- terminalize attempt 和 task recovery 更新在同一事务中完成。
- 并发 sweeper 只能有一个成功推进。
- active attempt 唯一约束在 retry 创建前被释放。

真实 smoke：

- Runtime 不在线或不可分派时触发 dispatch failure，task 最终显示 retry scheduled 或 waiting human，而不是静默停在 planned。
- queued attempt 人为制造 started 超时，确认 attempt lost、task next action 正确。

## 14. 风险和开放点

- 需要确认现有 `retry_not_before` 是否已经被 ready-task 调度读取；如果没有，首版必须同时补最小 retry wakeup。
- 需要决定 start deadline 默认值，建议从短可配置值开始，例如 2 到 5 分钟。
- 需要明确哪些 dispatch errors 属于 non-retryable，避免错误地自动重试配置或权限问题。
- 如果现有 run/task_run 已失败但 ProjectTask attempt 还未同步，RecoveryService 需要定义事实优先级，避免状态互相覆盖。
- `reassign/replan` 首版只预留 decision payload，不落地完整动作，否则范围会扩大到 DAG recovery engine。
