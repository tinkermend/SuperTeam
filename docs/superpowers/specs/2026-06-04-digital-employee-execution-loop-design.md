# 数字员工执行闭环设计

日期：2026-06-04  
状态：已确认，待实现计划

## 1. 背景

SuperTeam 已完成 Runtime 接入、短期 Runtime Session、Runtime WebSocket 命令通道、数字员工业务身份、唯一执行实例、Provider Session 基础表，以及 Runtime Agent 本地 `start_session`、`resume_session`、`send_input`、`stop_session` 命令执行层。

当前缺口是：数字员工还不能从 Web 详情页发起一次完整执行，并把 Provider 事件、结果、失败原因和停止动作稳定回到 Control Plane 与 Web 页面。执行闭环需要同时覆盖创建时的 Runtime 目录预置、运行时命令下发、Runtime 回写、事件持久化和 Web 观测。

## 2. 核心模型

固定边界：

```text
Web
  <-> Control Plane
    <-> Runtime Agent
      <-> Provider
```

模型定义：

- Runtime Agent 是客户侧或开发者机器上的执行宿主，可以提供多个 Agent Provider 能力，例如 `claude-code`、`opencode`。
- Digital Employee 是 Control Plane 中的业务身份，必须归属于团队。
- Execution Instance 是某个数字员工在某个 Runtime Agent 上的唯一本地会话目录绑定。
- 一个 Runtime Agent 可以承载多个数字员工的执行实例目录。
- Provider 是 Runtime Agent 驱动的真实执行器，不直接连接 Control Plane 或 Web。
- 数字员工开始任务时，Control Plane 下发命令到绑定的 Runtime Agent，由 Runtime Agent 在该数字员工目录内驱动 Provider 执行。
- Provider 事件和结果必须先经过 Runtime Agent，再由 Runtime Agent 回写 Control Plane；Web 只展示 Control Plane 持久化后的状态。

## 3. 目标

- Web 创建数字员工时必须选择 Runtime Agent 和 Provider。
- 创建数字员工成功前，Runtime Agent 必须为该数字员工预置独立会话目录。
- 创建时必须把团队默认宪法、skills 集合、MCP 能力等治理资产推送到对应数字员工目录。
- 新增 `POST /api/v1/digital-employees/{employeeId}/runs` 执行入口。
- Control Plane 在执行前校验数字员工状态、生效治理配置、执行实例、Runtime 连接和 Provider 能力。
- Control Plane 生成 `task_id`、`run_id`、`command_id`，并通过 `ConnectionRegistry.Dispatch` 下发 `start_session`。
- Runtime Agent 执行后通过 HTTP 回写 provider events、结果、失败原因和取消结果。
- Runtime Agent 到 Provider 之间使用版本化 `provider-run/v1` 协议承载输入、事件、结果、能力和诊断信息。
- 执行事件与完整 Provider 日志分离：Control Plane 持久化可查询事件摘要，完整 stdout/stderr/raw JSON 通过 `log_ref` 或 artifact 引用管理。
- 执行结果必须能结构化回写 `usage`、`model`、`cost`、`session_state`、`work_products` 和分层错误诊断。
- Web 数字员工详情页展示执行中状态、事件流、结果、失败原因和停止动作。

## 4. 非目标

- 不定义 Runtime 本地数字员工子目录结构和具体文件格式。
- 不实现一个数字员工绑定多个执行实例。
- 不实现 Runtime 自动调度、fallback 或跨 Runtime 迁移。
- 不让 Web 直连 Runtime Agent 或 Provider。
- 不把 Runtime WebSocket 改为全双向事件协议。
- 不让 Provider 直接持有平台 token。
- 不在本阶段实现复杂 approval 流程；执行前只要求团队治理配置已生效。
- 不照搬 Paperclip 的 issue checkout、timer heartbeat 或 Control Plane 本地 CLI adapter 模型；SuperTeam 的真实执行仍必须发生在 Runtime Agent 内。

## 5. 方案选择

### 5.1 方案 A：任务中心最小闭环

`POST /digital-employees/{id}/runs` 只创建 `task` 和 `task_run`，下发 `start_session`，Runtime 回写只进入 `task_events`。

优点是改动小。缺点是 Provider Session 恢复、数字员工会话历史和 Provider 维度审计不足，后续会补一层相同事件投影。

结论：不采用。

### 5.2 方案 B：数字员工 Run 编排服务，复用任务主线，双投影事件

新增 Control Plane 应用服务 `DigitalEmployeeRunService`。服务层复用 `tasks`、`task_runs`、`task_events` 作为执行记录主线，同时写入 `provider_sessions` 和 `provider_session_events` 作为 Provider 会话投影。

优点：

- 符合现有数据库模型。
- `task_id/run_id/command_id` 能形成可追踪执行链。
- Provider Session 维度不会丢失。
- HTTP 回写作为持久化事实源，便于重试、幂等和测试。
- Web 仍只依赖 Control Plane。

结论：采用。

### 5.3 方案 C：全双向 WebSocket 执行通道

Runtime WebSocket 同时承载命令、ack、事件流、结果和停止响应。

长期实时性更强，但第一版必须同时解决 ack、断线重放、序列号去重、背压、重连补偿和持久化失败处理。当前阶段复杂度过高。

结论：暂不采用。

## 6. 创建数字员工与 Runtime 预置

`POST /api/v1/digital-employees` 升级为创建业务身份和预置 Runtime 目录的强一致入口。

请求至少包含：

- `team_id`
- `name`
- `role`
- `runtime_node_id`
- `provider_type`
- 可选个人配置覆盖字段

创建前置条件：

- 数字员工必须归属于团队。
- 团队必须存在当前生效治理配置。
- Runtime node 必须在线。
- Runtime enrollment 必须已批准。
- Runtime session 必须有效。
- Runtime WebSocket 必须已连接。
- Runtime 上报的目标 Provider capability 必须可用且健康。

创建过程：

1. Control Plane 校验团队、Runtime、Provider 和治理配置。
2. Control Plane 创建数字员工业务身份。
3. Control Plane 创建唯一 execution instance。
4. Control Plane 合成团队默认治理资产和员工初始配置快照。
5. Control Plane 下发 `provision_instance` 到 Runtime Agent。
6. Runtime Agent 为该数字员工预置独立会话目录，并写入团队默认宪法、skills 集合、MCP 能力等治理资产。
7. Control Plane 收到预置成功后返回创建成功，执行实例状态为 `ready`。

失败语义：

- Runtime WebSocket 不在线时直接拒绝创建。
- 团队缺少当前生效治理配置时直接拒绝创建。
- Provider capability 不健康时直接拒绝创建。
- Runtime 预置失败时创建请求失败，不保留半成品数字员工和执行实例。

目录结构和文件格式不在本阶段固定。`provision_instance` 使用版本化 payload 承载治理资产，后续可独立细化 Runtime 本地落盘规范。

## 7. 执行 Run API

新增 Web 用户接口：

- `POST /api/v1/digital-employees/{employeeId}/runs`
- `GET /api/v1/digital-employees/{employeeId}/runs`
- `GET /api/v1/digital-employees/{employeeId}/runs/{runId}`
- `GET /api/v1/digital-employees/{employeeId}/runs/{runId}/events`
- `POST /api/v1/digital-employees/{employeeId}/runs/{runId}/stop`

`POST /runs` 请求体采用轻量 envelope：

- `objective` 或 `prompt` 必填。
- `context_refs` 可选。
- `artifact_refs` 可选。
- `output_schema` 可选。
- `allowed_actions` 可选。
- `idempotency_key` 可选；同一数字员工、同一 key 的重复请求必须返回同一 run 或明确冲突。
- `timeout_sec` 可选；缺省使用团队或数字员工执行策略。
- `grace_sec` 可选；用于停止或超时后的优雅退出窗口。
- `metadata` 可选。

Control Plane 负责合成 Runtime command payload，包括：

- `command_id`
- `task_id`
- `run_id`
- `digital_employee_id`
- `execution_instance_id`
- `provider_type`
- `provider_run_protocol`
- `session_policy`
- `session_params`
- `idempotency_key`
- `timeout_sec`
- `grace_sec`
- `objective`
- `prompt`
- `context_refs`
- `artifact_refs`
- `output_schema`
- `allowed_actions`
- `forbidden_actions`
- `secret_refs`
- `governance_snapshot_id`
- `metadata`

执行前置条件：

- 数字员工状态必须为 `ready` 或 `active`。
- 数字员工必须未禁用、未归档、未删除。
- 必须存在已批准的生效治理配置。
- 必须存在唯一 execution instance，且状态为 `ready` 或 `active`。
- 绑定 Runtime 必须在线且 WebSocket 已连接。
- 绑定 Provider capability 必须 healthy。

## 8. Runtime 命令与 Provider Run Protocol

Runtime WebSocket 第一版只作为 Control Plane 到 Runtime Agent 的命令下发通道。`start_session` 命令内部必须携带版本化 Provider 执行协议，建议命名为 `provider-run/v1`。

Provider adapter 输入：

- `run_id`
- `command_id`
- `execution_instance_id`
- `provider_type`
- `objective`
- `prompt`
- `context_refs`
- `artifact_refs`
- `allowed_actions`
- `forbidden_actions`
- `output_schema`
- `timeout_sec`
- `grace_sec`
- `secret_refs`
- `session_policy`
- `session_params`
- `governance_snapshot_id`
- `metadata`

Provider adapter 能力声明：

- `provider_type`
- `provider_version`
- `models`
- `supports_resume`
- `supports_stop`
- `supports_timeout`
- `supports_structured_output`
- `supports_artifact_refs`
- `requires_workspace`
- `required_secret_refs`

Provider adapter 结果：

- `status`：`completed`、`failed`、`cancelled`、`timed_out`
- `summary`
- `result_json`
- `exit_code`
- `signal`
- `timed_out`
- `error_code`
- `error_family`
- `retry_not_before`
- `usage`
- `model`
- `cost`
- `provider_session_external_id`
- `session_state_patch`
- `work_products`
- `log_ref`
- `raw_result_ref`

Runtime Agent 对 Claude Code、OpenCode 等具体 Provider 的 CLI、PTY、stdio、JSON stream 或 HTTP 差异只在 adapter 内消化。Control Plane 只理解 `provider-run/v1` 的结构化输入、事件、结果和错误族。

## 9. Runtime HTTP 回写

Runtime Agent 执行期间通过 HTTP 回写事实源。WebSocket 断线不应影响 Runtime 已产生事实的最终回写，只影响 Control Plane 是否还能继续下发新命令。

新增 Runtime-auth HTTP 接口：

- `POST /api/v1/runtime/commands/{commandId}/events`
- `POST /api/v1/runtime/commands/{commandId}/complete`
- `POST /api/v1/runtime/commands/{commandId}/fail`
- `POST /api/v1/runtime/commands/{commandId}/cancelled`
- `POST /api/v1/runtime/commands/{commandId}/timed-out`

每个事件至少包含：

- `event_type`
- `sequence_number`
- `payload`
- 可选 `provider_session_external_id`
- 可选 `session_state_patch`
- 可选 `log_ref`
- 可选 `raw_event_ref`
- 可选 `metadata`

Control Plane 根据 `command_id` 找到平台绑定：

- `tenant_id`
- `team_id`
- `task_id`
- `run_id`
- `digital_employee_id`
- `execution_instance_id`
- `runtime_node_id`
- `provider_type`

校验规则：

- Runtime session token 必须有效。
- Runtime node 必须与 command 绑定的 execution instance 匹配。
- command 必须属于未终止 run。
- `command_id + sequence_number` 必须幂等，Runtime 重试不能产生重复事件。
- 终态回写必须幂等，同一个 command 不能从一个终态切换到另一个终态。

## 10. 事件、日志与诊断

Runtime 回写事件后，Control Plane 服务层同时写两份投影：

- `task_events`：run 时间线，服务任务中心、数字员工 run 页面和通用审计。
- `provider_session_events`：Provider 会话事件，服务数字员工会话历史、Provider session 恢复、排障和审计。

事件投影只保存可查询摘要，不保存完整 Provider 输出流。完整 stdout、stderr、raw provider JSON、长 prompt preview 和大体积诊断信息必须写入日志或工件存储，再以 `log_ref`、`raw_event_ref`、`raw_result_ref` 引用。

日志引用至少记录：

- `log_ref`
- `storage_kind`
- `object_key`
- `sha256`
- `byte_size`
- `redaction_policy`
- `stdout_excerpt`
- `stderr_excerpt`
- `created_at`

诊断字段至少覆盖：

- `exit_code`
- `signal`
- `timed_out`
- `error_code`
- `error_family`
- `retry_not_before`
- `runtime_node_id`
- `provider_type`
- `provider_version`
- `provider_session_external_id`

规则：

- `session_started` 事件创建或更新 `provider_sessions`。
- Provider session external id 保存到 `provider_sessions.provider_session_id`。
- `provider_session_events.command_id` 必须记录平台命令 ID。
- `task_events.run_id` 必须记录平台 run ID。
- 终态事件更新 `task_runs` 和 `tasks` 的状态、结果、诊断、日志引用和工作产物。
- 日志和事件必须做脱敏，不能把 secret 明文、认证 header、私钥、token 或未授权上下文写入 Web 可见 payload。

## 11. Run Admission 与会话状态

第一版对每个数字员工采用保守 admission 规则：同一数字员工同一时间最多只能有一个非终态 run，非终态包括 `queued`、`dispatching`、`running`、`cancelling`。

规则：

- 如果已有非终态 run，新建 run 默认返回冲突。
- 如果请求携带相同 `idempotency_key`，Control Plane 返回已存在 run。
- 如果请求携带相同 `idempotency_key` 但 payload 关键字段不同，Control Plane 返回幂等冲突。
- 后续需要并发执行时，再引入 execution slots、priority、queue 和 policy，不在 MVP 中隐式开放。

Provider 会话状态不能只保存一个 external id。需要保留 adapter 可恢复所需的结构化状态：

- `provider_session_external_id`
- `session_display_id`
- `session_params`
- `session_state`
- `last_sequence_number`
- `last_command_id`
- `last_run_id`
- `last_error_family`
- `last_runtime_seen_at`

`session_params` 和 `session_state` 由 Runtime Agent 根据 `provider-run/v1` 维护，Control Plane 只做结构化持久化和展示，不解释 Provider 私有字段。

## 12. 状态机、停止与超时

数字员工 run 对 Web 暴露以下状态：

```text
queued -> dispatching -> running -> completed
                               -> failed
                               -> timed_out
                               -> cancelling -> cancelled
                                            -> failed
                                            -> timed_out
```

状态含义：

- `queued`：Control Plane 已创建 task/run，尚未下发命令。
- `dispatching`：正在通过 WebSocket 下发 `start_session`。
- `running`：Runtime 已接受命令或回写首个 provider event。
- `cancelling`：Web 请求停止，Control Plane 已下发或正在下发 `stop_session`。
- `completed`：Runtime 回写完成结果。
- `failed`：前置校验、下发、Runtime 处理、Provider 执行或回写处理失败。
- `timed_out`：Runtime 或 Provider 超过 `timeout_sec`，并完成超时处理。
- `cancelled`：Runtime 确认 Provider run 已停止。

`cancelling` 和 `timed_out` 是一等持久化状态。实现时需要扩展 task/run 状态枚举、状态机和 Web 类型定义，不能只在内存或前端派生。

Web 停止调用：

```text
POST /api/v1/digital-employees/{employeeId}/runs/{runId}/stop
```

Control Plane 行为：

1. 校验 run 属于该数字员工。
2. 校验 run 处于 `dispatching`、`running` 或其他可停止状态。
3. 写入 `stop_requested` 事件。
4. 将 run 状态置为 `cancelling`。
5. 通过 `ConnectionRegistry.Dispatch` 下发 `stop_session`。
6. Runtime 在 `grace_sec` 内优雅停止 Provider。
7. 如果 Provider 未退出，Runtime 可执行强制停止。
8. Runtime 回写 `cancelled`、`failed` 或 `timed_out` 后，Control Plane 写入终态。

如果 Runtime 不在线或停止命令下发失败：

- 不把 run 标记为 `cancelled`。
- 记录失败原因。
- Web 展示停止失败或 Runtime 未连接。

失败原因分层：

- `preflight_failed`：数字员工、团队治理、Runtime 或 Provider 能力校验失败。
- `admission_rejected`：同一数字员工已有非终态 run 或幂等冲突。
- `dispatch_failed`：WebSocket 下发失败。
- `runtime_rejected`：Runtime 解析命令、校验环境或预置目录失败。
- `provider_failed`：Claude Code、OpenCode 等 Provider 执行失败。
- `timeout`：执行超过 `timeout_sec`。
- `writeback_failed`：Runtime 回写被 Control Plane 拒绝，Runtime 本地应可记录并重试。

## 13. 恢复与 Liveness

执行系统不能出现无解释的永久中间态。Control Plane 与 Runtime Agent 都需要恢复语义。

Control Plane 恢复规则：

- 服务启动后扫描超出阈值的 `dispatching`、`running`、`cancelling` run。
- 如果绑定 Runtime WebSocket 在线，查询或等待 Runtime 回写最新状态。
- 如果 Runtime 长时间离线，将 run 标记为 `failed` 或保持 `running` 但展示 `runtime_disconnected`，具体阈值由策略决定。
- 对已收到终态但事件投影不完整的 run，允许通过幂等回写补齐事件。

Runtime Agent 恢复规则：

- Runtime 重启后恢复本地仍在执行的 Provider 进程或识别其已结束状态。
- Runtime 必须能基于 `command_id` 重试未成功提交的事件和终态回写。
- Runtime 对同一个 `command_id` 重复收到 `start_session` 时必须幂等，不能启动两个 Provider 进程。
- Runtime 对 `stop_session` 重复下发必须幂等。

## 14. Work Products 与 Artifact

Provider 最终结果不只是一段文本。Runtime 回写的结果应包含结构化 `work_products`，便于 Web、审批和后续流程消费。

支持的工作产物类型：

- `artifact`
- `document`
- `preview_url`
- `runtime_service`
- `branch`
- `commit`
- `pull_request`
- `external_url`
- `handoff`

每个 work product 至少包含：

- `type`
- `title`
- `summary`
- `ref`
- `metadata`
- `created_at`

Control Plane 仍通过 `artifact` 模块管理长期文件、日志、报告、附件和执行产物。`work_products` 是 run 结果中的结构化索引，不替代 artifact 存储。

## 15. Provider 环境与治理资产校验

创建数字员工和发起 run 前都需要验证 Runtime 与 Provider 环境。

创建时校验：

- Runtime WebSocket 在线。
- Provider capability healthy。
- Runtime 支持 `provision_instance`。
- Runtime 能接收团队默认治理资产 payload。
- 治理资产只通过结构化 payload 和 secret refs 传递，不直接推送明文密钥。

执行前校验：

- Runtime 仍在线。
- Provider capability 仍 healthy。
- Runtime 已完成该 execution instance 的预置。
- Provider adapter 的 `validate_environment` 或等价能力返回通过。
- skills、MCP、宪法快照版本与 Control Plane 记录一致；不一致时先拒绝执行或触发显式同步。

## 16. Web 详情页

新增 `/employees/{employeeId}` 数字员工详情页。列表页只展示摘要和入口，不承载完整事件流。

详情页区域：

- 顶部摘要：员工名称、团队、状态、风险等级、Runtime、Provider、最近运行状态。
- 执行实例区：Runtime、Provider、目录预置状态、治理资产版本、最后同步时间和不可运行原因。
- 运行控制区：输入 `objective/prompt`，可选展开结构化字段。
- 当前执行区：展示 running/cancelling run、开始时间、command_id、timeout 和停止按钮。
- 事件流：按 sequence 展示 provider events，第一版用 TanStack Query polling 从 Control Plane 拉取。
- 结果区：展示完成结果、summary、usage、cost、work products、artifact refs。
- 失败区：展示 `error_family`、`error_code`、exit code、signal、timed out、stderr excerpt 和 log ref。
- 历史 runs：列出最近 runs，点击切换查看事件、诊断和结果。

交互规则：

- 员工不是 `ready` 或 `active` 时禁用开始执行。
- Runtime 不在线或 WebSocket 未连接时禁用开始执行。
- Provider capability 不健康时禁用开始执行。
- 已有非终态 run 时禁用新建 run，除非后续策略明确开放队列。
- running 时展示停止按钮。
- 点击停止后展示 `cancelling` 和“停止中”。
- 停止下发失败时展示失败原因，不伪装为 `cancelled`。

视觉遵循 `DESIGN.md` 的企业控制台风格：高信息密度、状态 badge、细边框面板、语义色和清晰操作反馈。

## 17. 权限与审计

授权动作：

- 创建数字员工继续使用 `employee.create`，但需要覆盖 Runtime/Provider 预置条件。
- 读取 run 和事件可使用 `employee.read`。
- 发起 run 新增 `employee.run.create`。
- 停止 run 新增 `employee.run.stop`。
- 读取完整日志和 raw result 可单独收敛到 `employee.run.log.read` 或 artifact 权限。

审计要求：

- 创建数字员工时记录 Runtime 与 Provider 选择。
- Runtime 预置成功或失败记录审计。
- 发起 run、停止 run、Runtime 回写失败和 Provider 执行失败都应有可追踪事件。
- timeout、强制停止、重试回写、幂等冲突和环境校验失败必须记录审计。
- 授权仍通过统一 `Authorizer.Check(actor, action, resource)`，不在 handler 中散落权限判断。

## 18. 测试策略

Control Plane 单元测试：

- 创建数字员工时必须选择 Runtime 和 Provider。
- Runtime WebSocket 不在线时创建失败且不保留半成品。
- 团队无当前生效治理配置时创建失败。
- Provider capability 不健康时创建失败。
- `POST /runs` 创建 task、run、command 并下发 `start_session`。
- 已有非终态 run 时再次创建 run 返回冲突。
- 相同 `idempotency_key` 重试返回同一 run。
- `stop` 进入 `cancelling` 并下发 `stop_session`。
- timeout 回写进入 `timed_out`。
- Runtime 回写事件按 `command_id + sequence_number` 幂等写入双投影。
- 终态回写不能从 `completed` 改写为 `failed`。
- 完整日志只保存引用和摘要，不把 raw stdout/stderr 全量写入事件表。

Runtime Agent 测试：

- `provision_instance` 能接收治理资产 payload 并预置数字员工目录。
- `provision_instance` 不固定本地子目录结构和文件格式。
- `validate_environment` 能暴露 Provider 不可执行、缺少 secret 或版本不兼容。
- `start_session` 在对应 execution instance 上驱动 Provider。
- 重复 `start_session` 不启动第二个 Provider 进程。
- Provider events 通过 HTTP 回写 Control Plane。
- 超时后先优雅停止，再在 `grace_sec` 后强制停止。
- `stop_session` 重复下发幂等。

Web 测试：

- 数字员工创建表单要求 Runtime 和 Provider。
- 数字员工详情页可发起 run。
- 已有 running/cancelling run 时禁用新建 run。
- 详情页展示 running、events、usage、work products、result 和 failure。
- 停止后显示 `cancelling`，终态回写后显示 `cancelled`。
- timeout 后显示 `timed_out` 和诊断摘要。

集成 smoke：

1. 启动 Control Plane、Web 和 Runtime Agent。
2. 创建或确认团队当前生效治理配置。
3. 选择 Runtime 和 Provider 创建数字员工。
4. 确认 Runtime 预置成功。
5. 发起 tiny run。
6. 确认事件、日志引用、session state、work products 和结果落库并在 Web 展示。
7. 发起可停止 run，点击停止，确认 `cancelling -> cancelled`。
8. 发起超时 run，确认 `running -> timed_out`。

## 19. 实施顺序建议

1. 扩展数据库和 sqlc 查询，补齐 run 状态、command 绑定、幂等事件、session state、log ref、diagnostic 和 work products 所需字段或表。
2. 新增 Control Plane `DigitalEmployeeRunService`。
3. 升级数字员工创建接口，接入 Runtime/Provider 选择和 `provision_instance`。
4. 扩展 Runtime command contract，新增 `provision_instance`，并引入 `provider-run/v1`。
5. 新增 Runtime HTTP 回写接口和双投影写入。
6. 新增 run admission、idempotency、timeout 和 stop 状态机。
7. 新增 run API 和 stop API。
8. 新增 Web 数字员工详情页。
9. 跑单元测试、Web 测试和本地端到端 smoke。

实施计划阶段需要进一步拆分每一步的具体文件、测试命令和迁移编号。
