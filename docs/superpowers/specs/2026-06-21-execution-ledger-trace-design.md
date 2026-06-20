# Execution Ledger 与项目执行证据链读模型设计

## 1. 背景

SuperTeam 已经有几类执行事实：

- `project_task_attempts` 记录项目任务的执行尝试、Runtime 租约、Provider session、失败分类和终态事件。
- `project_execution_summaries` 记录数字员工回写的最终结论、证据、工件、不确定性和人工复核要求。
- `provider_sessions` / `provider_session_events` 记录 Provider 会话和 Provider 事件。
- `runtime_command_receipts` 记录 Runtime 命令下发、回写、结果和错误。
- `project_events` 记录项目业务事件。

这些事实目前能支撑局部排障，但项目验收、人类审批和失败重试仍缺一个项目级执行证据链视角。负责人看到的主要是最终 `conclusion`，难以判断结论来自哪些 Provider session、MCP/tool/capability 调用、输入输出摘要、失败分类、artifact 和 evidence。

本设计采用方案 3-lite：以 Execution Ledger 作为长期架构方向，第一期只落项目验收必需的读模型和关键写入点，不一次性替换现有 Runtime/Provider/MCP 采集链路。

## 2. 目标

- 建立统一的执行事实索引，能串起项目任务 attempt、Provider session/event、Runtime command receipt、tool/MCP/capability invocation、artifact/evidence 和 execution summary。
- 给项目详情页、人类审批、失败重试和项目验收提供同一套 `execution-trace` 读模型。
- 第一阶段控制工作量，复用现有事实表，不重写 Runtime Agent 的 Provider/MCP 采集协议。
- 为未来完整平台保留演进空间：后续 Runtime/Provider/MCP 真正结构化采集时继续写同一套 ledger/projection，Web 和审批读模型不需要推倒重做。

## 3. 非目标

- 不用 ledger 替代 `project_task_attempts` 的状态机职责。
- 不用 ledger 替代 `project_events` 的业务事件流。
- 不把完整 prompt、完整 tool args、完整 provider raw payload 或大日志正文直接存入 ledger。
- 不在第一期实现完整 MCP server/tool 运行时采集协议。
- 不在第一期迁移或删除 `provider_session_events`、`runtime_command_receipts`、`project_execution_summaries`。

## 4. 核心模型

新增 append-only 表 `execution_ledger_events`。它是跨 Runtime、Provider、MCP、tool、external capability 的统一执行事实索引和摘要。

建议字段：

| 字段 | 说明 |
| --- | --- |
| `id` | UUID 主键 |
| `tenant_id` | 租户 ID |
| `team_id` | 团队 ID，可从项目或任务上下文带入 |
| `project_id` | 项目 ID |
| `project_task_id` | 项目任务 ID，可空以支持项目级事件 |
| `project_task_attempt_id` | 项目任务 attempt ID，可空 |
| `event_type` | 执行事件类型 |
| `source_type` | 来源类型，如 `project_task_attempt`、`provider_session_event`、`runtime_command_receipt` |
| `source_id` | 来源记录 ID 或稳定外部 ID |
| `actor_type` | `digital_employee`、`runtime_node`、`provider`、`system` 等 |
| `actor_id` | actor 标识 |
| `runtime_node_id` | Runtime 节点 ID |
| `provider_type` | Provider 类型，如 `codex`、`claude`、`opencode` |
| `provider_session_id` | Provider external session ID |
| `input_summary` | 输入摘要 |
| `output_summary` | 输出摘要 |
| `error_family` | 错误分类 |
| `error_code` | 细分错误码 |
| `error_message` | 短错误说明 |
| `retryable` | 是否可重试 |
| `artifact_refs` | 工件引用数组 |
| `evidence_refs` | 证据引用数组 |
| `metadata` | 结构化扩展数据 |
| `occurred_at` | 事件发生时间 |
| `created_at` | 记录写入时间 |
| `idempotency_key` | 幂等键 |

第一期事件类型：

- `attempt.started`
- `attempt.completed`
- `attempt.failed`
- `attempt.waiting_human`
- `summary.created`
- `provider.session.started`
- `provider.event`
- `tool.call`
- `mcp.tool_call`
- `capability.invocation`
- `artifact.linked`
- `evidence.linked`

后续可扩展事件类型，但业务核心不要依赖数据库 enum。服务端用注册表或校验函数约束已支持类型。

## 5. 与现有表的边界

- `project_task_attempts` 继续是任务 attempt 生命周期事实，保存状态、租约、失败分类、Runtime/Provider 关联和终态事件。
- `provider_session_events` 继续保存 Provider 原始或半结构化事件；ledger 只保存项目可读摘要和来源引用。
- `runtime_command_receipts` 继续保存命令 payload/result/error；ledger 只保存项目可读摘要和来源引用。
- `project_execution_summaries` 继续保存最终业务结论；ledger 写 `summary.created` 事件以把 summary 纳入同一证据链。
- `project_events` 继续保存业务事件，不承载细粒度执行过程。
- `artifact_refs` / `evidence_refs` 是验收证据引用。ledger 可以在 invocation 或 summary 阶段提前挂 refs，最终 summary 汇总关键 refs。

## 6. 写入流

新增 Control Plane 内部服务，例如 `ExecutionLedgerService`，封装 ledger 写入、幂等、摘要清洗和安全过滤。业务服务不得直接拼 SQL 写 ledger。

第一期写入点：

1. Runtime 调用 `StartProjectTaskAttempt` 成功后，Control Plane 写 `attempt.started`。
2. Runtime 调用 `CompleteProjectTaskAttempt` 成功后，同事务写 `attempt.completed` 和 `summary.created`。
3. Runtime 调用 `FailProjectTaskAttempt` 成功后写 `attempt.failed`，带 `failure_family`、`failure_summary`、`retryable`。
4. Runtime 调用 `WaitHumanProjectTaskAttempt` 成功后写 `attempt.waiting_human`，带等待原因和摘要。
5. `materializeTaskCompletionEvidence` 创建 evidence/artifact refs 后写 `artifact.linked`、`evidence.linked`。
6. 现有 Provider writeback 创建 `provider_session_events` 后，同步写 `provider.event` ledger 索引，保留 `source_type=provider_session_event` 和 `source_id`。
7. MCP/tool/external capability 第一期通过 Control Plane 内部方法 `RecordExecutionInvocation` 写 `tool.call`、`mcp.tool_call` 或 `capability.invocation`。后续 Runtime 真采集也走同一入口。

幂等规则：

- ledger 写入必须有 `idempotency_key`。
- 来源型事件使用 `{source_type}:{source_id}:{event_type}`。
- attempt 事件使用 `project_task_attempt:{attempt_id}:{event_type}`。
- summary 事件使用 `project_execution_summary:{summary_id}:summary.created`。
- invocation 事件使用调用方传入的稳定 invocation id 或 `{attempt_id}:{event_type}:{sequence}`。

一致性规则：

- 对审批和验收强依赖的 `attempt.completed`、`attempt.failed`、`summary.created` 应与原 writeback 同事务写入。
- 非关键 `provider.event` ledger 索引写失败时，不应回滚已落库 Provider 原始事件，但必须记录 `project_events` 或服务日志告警，避免静默丢证据。
- ledger 写入失败不能把已完成的业务任务误判为失败；但是如果缺少 `summary.created` 或 attempt 终态 ledger，项目 trace API 必须能暴露断链状态。

## 7. 项目级读 API

新增：

`GET /api/v1/projects/{projectId}/execution-trace`

响应按项目任务和 attempt 分组，而不是返回裸 ledger 列表。

示意结构：

```json
{
  "project_id": "uuid",
  "summary": {
    "attempt_count": 3,
    "failed_attempt_count": 1,
    "human_review_required_count": 1,
    "artifact_ref_count": 2,
    "evidence_ref_count": 4,
    "latest_error_family": "provider_error"
  },
  "attempts": [
    {
      "project_task_id": "uuid",
      "attempt_id": "uuid",
      "attempt_no": 1,
      "status": "succeeded",
      "runtime_node_id": "uuid",
      "provider_type": "codex",
      "provider_session_id": "codex-session-id",
      "started_at": "2026-06-21T10:00:00Z",
      "finished_at": "2026-06-21T10:04:00Z",
      "failure_family": null,
      "retryable": null,
      "events": [
        {
          "id": "uuid",
          "event_type": "provider.event",
          "input_summary": "读取项目上下文并启动 Provider turn",
          "output_summary": "Provider 返回任务执行结果摘要",
          "error_family": null,
          "artifact_refs": [],
          "evidence_refs": [],
          "source_type": "provider_session_event",
          "source_id": "uuid",
          "occurred_at": "2026-06-21T10:03:30Z"
        }
      ],
      "summary": {
        "execution_summary_id": "uuid",
        "conclusion": "任务已完成",
        "requires_human_review": false
      }
    }
  ]
}
```

查询原则：

- API 读取 ledger，同时补充 `project_task_attempts`、`project_execution_summaries` 的必要字段。
- 如果第一期部分历史数据没有 ledger，可从现有 attempt/summary 表生成轻量 fallback event，但必须标记 `source_type=projection_fallback`。
- 默认按 `occurred_at ASC, created_at ASC` 排序。
- 支持按 `project_task_id`、`attempt_id`、`event_type`、`error_family` 过滤。

## 8. Web 展示

在项目详情页新增“执行证据链”区域，建议靠近当前“执行摘要”面板。它不替代摘要，而是解释摘要如何产生。

第一期展示：

- Trace 总览：attempt 数、失败数、需人工复核数、artifact/evidence 数、最近错误分类。
- 按任务分组的 attempts。
- 每个 attempt 展示状态、attempt_no、Runtime node、Provider session、耗时、失败分类、retryable。
- 每个 attempt 下展示 timeline：
  - `attempt.started`
  - `provider.session.started`
  - `provider.event`
  - `tool.call`
  - `mcp.tool_call`
  - `capability.invocation`
  - `artifact.linked`
  - `evidence.linked`
  - `summary.created`
  - `attempt.completed` / `attempt.failed` / `attempt.waiting_human`

事件卡片只展示摘要和 refs：

- 输入摘要
- 输出摘要
- 错误分类
- artifact/evidence refs
- source refs，例如 provider event 或 runtime receipt

审批和验收联动：

- 人类审批弹窗展示与当前 decision/task 相关的 trace 子集。
- 项目验收区域展示最终 summary 和支撑 trace。
- 失败重试入口展示最近失败 attempt 的 `error_family`、`retryable`、关键 output/error summary 和证据引用。

前端第一期不做复杂图谱，只做可扫描的分组列表和 timeline，保持现有运维型项目详情风格。

## 9. 错误分类

第一期稳定错误 family：

- `provider_error`
- `runtime_error`
- `capability_denied`
- `capability_error`
- `mcp_server_error`
- `tool_input_invalid`
- `timeout`
- `missing_context`
- `human_review_required`
- `contract_violation`
- `unknown`

`error_code` 用于更细分的服务端错误码。Web 默认展示 `error_family` 和短摘要，详细 metadata 折叠展示。

## 10. 权限、安全和审计

- `execution-trace` API 只允许有项目可见权限的用户读取。
- 读模型必须遵循租户和团队边界，查询索引以 `tenant_id` 开头。
- ledger 不保存 secret、完整 prompt、完整 tool args 或完整 provider raw payload。
- 凭据、原始日志和大 payload 应继续放在受控对象存储或原始事件表中，通过 refs 访问。
- capability/MCP invocation 必须记录授权结果或失败原因。
- 高风险 capability 的审批链后续通过 `decision_request_id`、`approval_request_id` 或 metadata 关联。
- ledger 是审计读模型的一部分，不做硬删除；项目归档或删除应遵循现有 artifact/evidence 保留策略。

## 11. 实施分期

第一期：

1. 新增 `execution_ledger_events` migration、sqlc query、domain type、repository/service。
2. 在项目 task attempt writeback 和 execution summary 写入路径补 ledger 写入。
3. 在 Provider writeback 路径创建 `provider.event` ledger 索引。
4. 增加 `RecordExecutionInvocation` 内部服务入口，先支持服务端手动记录 tool/MCP/capability 调用。
5. 新增 `GET /projects/{projectId}/execution-trace` API 和 OpenAPI contract。
6. Web 项目详情新增执行证据链面板。
7. 补充 Go/API/Web 测试和一次真实链路 smoke。

第二期：

1. Runtime Agent 将 Provider/MCP/tool/capability 事件结构化回写到 Control Plane。
2. 增加 source refs 的详情跳转，例如 provider event、runtime receipt、artifact/evidence 详情。
3. 审批弹窗和验收区按 decision/task 过滤 trace 子集。
4. 增加历史数据 backfill 或 projection fallback 清理策略。

第三期：

1. 统一更多执行域事件到 ledger。
2. 将项目验收、失败重试、成本和审计报表从 execution trace projection 读取。
3. 根据真实查询压力决定是否增加独立 projection 表或物化视图。

## 12. 验证计划

局部测试：

- Repository/service 测试：attempt start/complete/fail/wait-human 生成幂等 ledger events。
- Provider writeback 测试：`provider_session_events` 创建后，project trace 能看到 `provider.event`。
- API 测试：`GET /projects/{id}/execution-trace` 返回按 task/attempt 分组的有序 trace。
- Web 测试：项目详情能渲染 trace 总览、attempt timeline、失败分类和 refs。
- Contract 测试：OpenAPI 生成和契约验证通过。
- Migration/sqlc 测试：migration 可应用，sqlc 生成代码无漂移。

真实链路验证：

- 启动当前 Web、Control Plane、Runtime Agent。
- 通过真实项目任务触发 Runtime writeback。
- 确认数据库中存在 project task attempt、execution summary、execution ledger events。
- 通过 API 读取 execution trace，确认 attempt、summary、ledger event 三者一致。
- 打开项目详情页，确认执行证据链不是 mock、缓存或旧服务结果。

如果真实 Provider、Runtime 或认证环境不可用，不能声明功能可用，只能交付局部验证结果并标记真实链路阻塞。

## 13. 设计决策

- 选择 Execution Ledger 作为长期方向，避免把 `capability_invocations` 做成孤立旁支表。
- 第一阶段不推翻现有事实表，降低迁移风险。
- Web 面向 `execution-trace` API，而不是直接拼底层表，保证后续 projection 变化不影响页面。
- Ledger 只保存摘要和 refs，避免把敏感上下文和大 payload 复制到审计索引。
- 事件类型使用服务端校验字符串，保持 Provider 和 Capability 类型的 registry-first 扩展方式。
