# 动态项目规划 V1 Phase 3：PreDispatchGate 与安全分派

日期：2026-06-21
状态：已确认，待实施计划
上级设计：`2026-06-21-dynamic-project-planning-orchestration-v1-design.md`

## 1. 背景

Phase 2 让 accepted plan 能稳定分解为 ProjectTask DAG，但 ProjectTask 进入 Runtime 前仍需要最后一层 Control Plane 校验。计划生成时满足条件，不代表分派时仍满足条件：

- 数字员工可能被停用或移出项目；
- Runtime 节点可能掉线；
- Provider 或工作区可能不可用；
- MCP/connector 授权可能失效；
- 人工审批可能尚未完成；
- 上游依赖可能失败、阻塞或结果不符合验收标准；
- 预算、并发、租约或执行槽位可能已变化。

PreDispatchGate 的目标是在任务真正进入 Runtime 前，把这些已知前置条件统一检查清楚，避免把明显不可执行或高风险任务直接交给 Runtime。

## 2. 目标

- 在 ProjectTask 从 `planned` 进入 `queued/running` 前执行统一 gate。
- 对权限、工具、Runtime、Provider、依赖、预算、负载、审批和 context readiness 进行校验。
- 将 gate 结果持久化为可审计事实。
- 对需要人工操作的情况创建明确的人类交互请求。
- 保证 gate 与 dispatch 幂等，不因 workflow retry 重复创建 run 或审批。
- 保持边界：Control Plane 做策略和校验，Runtime 做执行。

## 3. 非目标

- 不把 Runtime 执行失败提前归因到 gate；gate 只检查分派前已知条件。
- 不在 gate 中执行本地命令、数据库查询任务正文或 Provider 任务。
- 不让 Runtime Agent 决定业务审批和风险策略。
- 不在本阶段实现完整结果验收；结果循环在 Phase 4。
- 不绕过 ProjectTask 状态机直接启动 Provider。

## 4. Gate 触发点

触发时机：

- ProjectTask 由 accepted plan 分解后成为 root ready task；
- 上游任务完成后 downstream 变为 ready；
- 人类批准或补充上下文后任务恢复；
- Runtime 暂态不可用后重试分派；
- 重试 attempt 或换员工前。

调用位置：

- project coordinator workflow 可调 activity；
- task dispatch service 在创建 attempt/run 前强制调用；
- API 手动触发 dispatch 时同样走 gate。

任何创建 Runtime run 的入口都必须经过同一个 gate。

## 5. Gate 输入

Gate 输入对象：

```json
{
  "project_id": "uuid",
  "project_task_id": "uuid",
  "accepted_plan_revision_id": "uuid",
  "planned_task_key": "inspect-db",
  "selected_employee_id": "uuid",
  "attempt_policy": {},
  "current_context": {},
  "dispatch_reason": "dependency_unlocked"
}
```

Gate 内部加载：

- ProjectTask 当前状态和契约；
- dependency 状态和 upstream result；
- selected employee 当前 project membership；
- employee planning profile 当前快照；
- permissions、tool bindings、MCP、connector；
- Runtime node、Provider、workspace、slot 状态；
- budget、concurrency、retry policy；
- pending human decisions；
- tenant/team/project policy。

## 6. Gate 检查项

### 6.1 任务状态

- task 必须处于可分派状态，例如 `planned` 或恢复后的 `waiting_human` resolved；
- task 不能是 `completed/failed/cancelled`；
- 当前没有 active attempt 或 active run；
- retry policy 允许新 attempt。

### 6.2 依赖

- 所有 dependency 的上游 task 必须 completed；
- 上游结果必须满足 acceptance criteria 或已被人类接受；
- 上游 blocked/failed/revision_needed 会阻塞 downstream；
- dependency 数据异常时不能继续分派。

### 6.3 数字员工

- selected employee 仍是项目可调度数字员工；
- 员工状态 active；
- 员工没有被 policy 禁止；
- 负载和执行槽位允许；
- 如需要换员工，必须走 reassign/replan 决策，不在 gate 中静默替换。

### 6.4 能力和权限

- required capabilities 仍满足；
- hard missing capability 不能通过；
- 必须权限 granted；
- 需要人工授权的 permission 已完成 approval；
- context policy 允许注入当前上下文。

### 6.5 工具和外部能力

- MCP server/tool 绑定可用；
- connector 授权未过期；
- external capability 注册存在且可调用；
- 工具不可用时区分 retry_later 和 blocked。

### 6.6 Runtime 和 Provider

- Runtime node online；
- Provider type 可用；
- workspace 已准备；
- runtime slot 可用；
- 任务所需 provider contract version 满足；
- Runtime 不可用时不能创建 run。

### 6.7 预算和风险

- 项目预算未超限；
- task 预算和超时策略存在；
- 高风险动作已有 human approval；
- 数据库写入、迁移、部署、删除、权限扩大等动作必须检查审批。

### 6.8 上下文完整性

- required input context refs 可解析；
- 敏感上下文符合注入策略；
- 如果缺少用户澄清，进入 `waiting_human`。

## 7. Gate 结果

建议结构：

```json
{
  "id": "uuid",
  "project_task_id": "uuid",
  "status": "passed",
  "checked_at": "2026-06-21T00:00:00Z",
  "checks": [
    {
      "key": "runtime.ready",
      "status": "passed",
      "details": {}
    }
  ],
  "blockers": [],
  "human_action_request": null,
  "retry_after": null,
  "dispatch_token": "stable-token"
}
```

状态：

- `passed`：允许创建 attempt/run；
- `waiting_human`：需要人类审批、澄清、补证或授权；
- `blocked`：当前无法继续，需外部修复或重新规划；
- `retry_later`：暂态条件不满足，例如 Runtime slot 暂不可用；
- `replan_required`：任务契约已不成立，需要重新规划。

## 8. 状态推进

Gate 与 ProjectTask 状态关系：

- `passed`：创建 attempt，task 进入 `queued`，等待 Runtime/Provider started 写回后进入 `running`。
- `waiting_human`：task 进入 `waiting_human`，创建 human decision/request。
- `blocked`：task 记录 blocker，按现有状态模型进入 blocked equivalent 或 waiting_human/recovery。
- `retry_later`：task 保持可调度状态，记录 retry_after，不创建 run。
- `replan_required`：触发 plan-level recovery/replan，不创建 run。

如果当前状态模型尚未有 `blocked`，可用 `waiting_human` 加 typed blocker 表达，但最终应与 ProjectTask durable closure 状态收敛。

## 9. 幂等性

Gate 幂等键建议：

- `project_task_id`
- `dispatch_reason`
- `dependency_version` 或 upstream result version
- `selected_employee_id`
- `attempt_sequence`

规则：

- 同一幂等键下，已 passed 且 run 已创建时直接返回既有 run/attempt；
- waiting_human 不重复创建相同 approval；
- retry_later 可以更新时间和原因；
- replan_required 只创建一个 recovery/replan 请求；
- gate result 必须可追踪到后续 attempt 或 human action。

## 10. 人类交互

Gate 可创建以下人类请求：

- `permission_approval`
- `risk_approval`
- `missing_context`
- `tool_authorization`
- `runtime_recovery`
- `budget_approval`
- `replan_decision`

请求必须包含：

- `project_id`
- `project_task_id`
- `dispatch_gate_result_id`
- 原因和证据；
- 可选动作；
- 默认超时或升级策略；
- continuation target。

人类完成后，协调器重新运行 gate，而不是直接跳过 gate。

## 11. 事件和审计

需要写入：

- `project_events`：gate passed/blocked/waiting_human/retry_later；
- audit log：检查摘要和决策原因；
- task timeline：供项目详情页展示；
- execution ledger 可在后续阶段索引 gate 与 attempt 的关系。

注意不要把密钥、完整连接串、敏感 SQL、完整日志写入 gate details。

## 12. 验证

单元测试：

- 每种 gate status；
- 依赖未完成时不能通过；
- employee 被停用时不能通过；
- 权限缺失创建 approval；
- Runtime 不在线时 retry_later 或 blocked；
- 高风险数据库写入需要 risk approval；
- waiting_human 不重复创建同一请求；
- passed 后重复调用不会重复创建 run。

集成测试：

- ready task 通过 gate 并创建 attempt；
- 缺 MCP binding 的任务在 Runtime 前被阻塞；
- human approval 完成后 gate 重新通过；
- budget hard stop 阻止 dispatch；
- upstream failed 阻止 downstream。

真实 smoke：

- 创建一个可通过 gate 的 task，确认 Runtime 收到任务；
- 创建一个缺权限或缺工具的 task，确认不创建 Runtime run，并生成可见 gate blocker。

## 13. 验收标准

- 所有 Runtime run 创建入口都必须经过 PreDispatchGate。
- 已知前置条件不满足时不会启动 Provider。
- gate 结果可审计、可展示、可恢复。
- 人类操作完成后通过重新 gate 恢复，而不是手动改状态。
- gate 与 dispatch 具备幂等性，不会因重试重复创建 run 或 approval。
