# Temporal ProjectCoordinator Continue-As-New 设计

日期：2026-07-01
> 复核状态：CHANGELOG 2026-07-01 15:15记录ProjectCoordinator增加历史安全续跑；锚点抽查发现workflow.go含ProjectCoordinator/ContinueAsNew/project_decision_requests
状态：已确认，待实施计划

## 1. 背景

`ProjectCoordinatorWorkflow` 是每个 Project 的虚拟协调线程，当前以 signal-driven 无限循环处理需求提交、成员或策略变化、任务完成或失败、人类决策和 shutdown。这个模型符合“项目内置独占协调状态机”的定位，但长期运行的 Temporal Workflow 会持续累积 Event History。项目生命周期较长、需求和审批较多时，history 会接近 Temporal 限制，最终影响 replay 和协调能力。

本设计解决 coordinator history 增长问题，同时保持 SuperTeam 的层次边界：

- Temporal 负责项目协调顺序、一致性、signal 路由、幂等推进和长等待。
- Control Plane DB 是项目、任务、审批、DAG、结果、工件和审计的事实源。
- Runtime Agent 和 Provider 负责具体执行、Provider 进程或会话、工作目录、日志、工件和底层 Codex / Claude Code 会话恢复。

因此，coordinator 不保存 Provider 会话，也不把 Runtime 执行现场复制进 Temporal history。Continue-As-New 后，coordinator 必须能通过数据库事实判断后续 signal 应该推进哪条业务分支。

## 2. 目标

- 防止 ProjectCoordinator Workflow 的 Temporal Event History 无限增长。
- 让 coordinator 能在 Continue-As-New 后继续处理新 signal。
- 去掉 workflow 长期 pending map 对业务语义的依赖。
- 让 `HumanDecisionSubmitted` 通过数据库中的 `project_decision_requests` 路由，而不是依赖 Go 内存状态。
- 支持动态 DAG：任务结果缺输入时，可以创建补证任务、等待人类审批、由其他数字员工补充数据，并在数据到位后继续原 DAG。
- 保持 Provider 会话恢复归 Runtime/Provider 层，Temporal 不接管 Codex / Claude Code 会话状态。
- 允许开发环境删除旧项目闭环数据和旧 Temporal executions，不兼容旧 run。

## 3. 非目标

- 不为旧 ProjectCoordinator run 做兼容迁移。
- 不把 Provider 会话、终端日志、完整 prompt、工作区文件或长文本上下文写入 coordinator checkpoint。
- 不新增一套独立的 coordinator checkpoint 表保存大块 pending JSON。
- 不在本设计中重构 Web 页面。
- 不改变 Project 作为业务闭环容器、Runtime 作为执行层、Provider 作为具体执行器的职责边界。

## 4. 总体方案

采用“无状态 coordinator + DB 路由事实 + Continue-As-New”的方案。

`ProjectCoordinatorWorkflow` 可以在一次 signal 处理过程中使用局部变量，但不能把长期业务 pending 状态只保存在 workflow 内存 map 中。每次 Continue-As-New 只传递最小输入：

```go
type ProjectCoordinatorInput struct {
    TenantID   uuid.UUID
    ProjectID  uuid.UUID
    WorkflowID string
    Generation int
}
```

启动或续跑后，workflow 注册 signal channel 并等待事件。收到 signal 后，通过 activity 查询数据库事实并推进对应业务状态。每轮 signal 完整处理后，workflow 检查 Temporal 是否建议 Continue-As-New，或测试配置中的低阈值是否已触发；如果触发，返回 `workflow.NewContinueAsNewError`，用相同 `WorkflowID` 开启下一代 run。

## 5. 数据模型

现有表已经覆盖大部分事实：

- `projects`：项目业务闭环容器。
- `project_demands`：用户需求。
- `project_tasks`：DAG 节点，包含原始任务、补证任务、重试或改派任务。
- `project_task_dependencies`：DAG 边。
- `project_task_results`：任务结构化结果和结果审查来源。
- `project_task_dispatch_gate_results`：分派前 gate 和人类处理引用。
- `project_plan_revisions`：计划版本和 accepted plan 分解来源。
- `approval_requests` / `approval_decisions`：人类审批事实源。
- `project_decision_requests`：项目侧人类决策投影。
- `project_events`：协调动作和审计记录。

需要补一个最小关联字段：

```sql
ALTER TABLE project_decision_requests
    ADD COLUMN plan_revision_id UUID;

CREATE INDEX idx_project_decision_requests_plan_revision
    ON project_decision_requests(tenant_id, project_id, plan_revision_id)
    WHERE plan_revision_id IS NOT NULL;
```

用途：

- `decision_type = 'plan_review'` 时，`plan_revision_id` 直接定位被审批的计划版本。
- `decision_type = 'task_failure_recovery'` 继续使用 `project_task_id`。
- `decision_type = 'project_acceptance'` 使用 `project_id`。
- dispatch gate 决策继续使用 `dispatch_gate_result_id`。

不要在 `project_decision_requests` 增加大块 checkpoint payload。需要上下文时保存稳定 ID 或引用，由 activity 查询对应事实表。

## 6. Workflow 行为

### 6.1 DemandSubmitted

保持现有主路径：

1. 创建 `project_coordination_jobs`。
2. 加载 `CoordinationSnapshot`。
3. 调用 planner 生成 route decision / plan revision。
4. 持久化 `project_plan_revisions`。
5. 如果计划可自动接受，分解为 ProjectTask DAG 并分派 root-ready 任务。
6. 如果计划需要人类确认，创建 `approval_requests` 和 `project_decision_requests`，同时写入 `plan_revision_id`。

### 6.2 HumanDecisionSubmitted

这是核心改造点。

收到 signal 后，workflow 不再先查 `pendingReviews`、`pendingFailureRecoveries` 或 `pendingAcceptance`。改为执行 activity：

1. 查询 `project_decision_requests` by `tenant_id + project_id + decision_request_id`。
2. 如果不存在，记录 observed event，结束本轮。
3. 如果状态不是 `pending`，记录 already-resolved observed event，结束本轮。
4. 按 `decision_type` 路由。

路由规则：

- `plan_review`：使用 `plan_revision_id` 找到 plan revision。接受则 accept revision、分解 DAG、dispatch ready tasks；要求修改则 reject 当前 revision 并重新规划；驳回或取消则结束 coordination job 或进入等待用户输入状态。
- `task_failure_recovery`：使用 `project_task_id` 找失败任务，执行 retry、reassign、cancel downstream 或 needs-more-evidence 策略。
- `project_acceptance`：使用 `project_id` 写入验收记录；接受则 archive project，驳回或需要补证则回到 running。
- `project_task_approval` 或 dispatch gate 类型：使用 `dispatch_gate_result_id` 应用 gate decision，并在通过后继续 dispatch。

所有分支都必须在数据库层幂等。重复 signal、Temporal replay 或 activity retry 不能重复创建审批、任务、依赖或事件。

### 6.3 EmployeeTaskCompleted

任务完成后 coordinator 不依赖 Temporal 内存判断下游，而是读数据库：

1. 记录 `workflow.signaled` 或对应任务完成事件。
2. 查询任务结果和依赖状态。
3. 如果结果满足依赖要求，解锁并 dispatch 下游。
4. 如果结果声明缺少输入、证据或上下文，创建补证任务和依赖边，必要时创建人类审批。
5. 如果项目所有需求已完成，创建 `project_acceptance` 人类决策。

### 6.4 EmployeeTaskFailed

失败处理继续通过持久化事实推进：

1. 标记或保持下游 blocked。
2. 创建 `task_failure_recovery` 审批和 `project_decision_requests`。
3. 人类决策回来后通过 `decision_type + project_task_id` 恢复，不依赖 workflow map。

### 6.5 Continue-As-New

每轮 signal 处理完整结束后检查：

- `workflow.GetInfo(ctx).GetContinueAsNewSuggested()`。
- 测试或配置阈值，例如低 history length 触发，用于 deterministic workflow test。

触发后：

```go
return workflow.NewContinueAsNewError(ctx, ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
    TenantID:   input.TenantID,
    ProjectID:  input.ProjectID,
    WorkflowID: input.WorkflowID,
    Generation: input.Generation + 1,
})
```

限制：

- 不在 activity 中间 Continue-As-New。
- 不在一次业务推进未完成时截断。
- 不把 pending map 或 Provider 会话写进 input。
- signal 处理完成后再截断，避免丢业务动作。

## 7. 动态 DAG 和补证流程

Temporal 能处理“DAG 中某个节点需要补充数据，再让另一个数字员工补充，审批后继续原任务流程”的协调，但数据传递必须落在 Control Plane 事实表和 artifact/context 引用中。

示例：

1. planner 生成 DAG：`A -> B -> C`。
2. `A` 完成后写回 `project_task_results`，结果声明 `B` 缺少某份输入。
3. coordinator 读取任务结果，创建补证任务 `D`。
4. `project_task_dependencies` 增加 `B` 依赖 `D`。
5. 如补证动作需要人类许可，创建 `project_decision_requests`。
6. 人类审批通过后 dispatch `D` 给合适数字员工。
7. `D` 完成后把 artifact/context ref 写回。
8. coordinator 检查 `B` 的依赖已满足，将 `B` 恢复为 dispatchable 并继续原 DAG。

这个流程中：

- Temporal 保证 signal 串行处理、长等待、retry 和 coordinator run 恢复。
- Control Plane DB 保证 DAG、审批、结果和证据可恢复。
- Runtime/Provider 保证执行任务和恢复具体 Agent 会话。

## 8. 开发环境数据清理

本设计不兼容旧 ProjectCoordinator run。实施前允许清理开发环境中影响本任务的旧项目闭环数据，但必须保留用户登录账号相关数据。

允许删除：

- `projects` 及其项目闭环子表。
- `project_demands`、`project_tasks`、`project_task_dependencies`、`project_events`。
- `project_coordination_jobs`、`project_route_decisions`、`project_plan_revisions`、`project_plan_decomposition_claims`。
- `project_task_attempts`、`project_task_results`、`project_task_dispatch_gate_results`、`project_task_attempt_context_updates`。
- `project_decision_requests`、项目产生的 `approval_requests` / `approval_decisions`。
- 项目产生的 `inbox_items`、项目证据、报告、artifact、验收、归档、预算、placement、attestation 等数据。
- Temporal namespace 中旧 ProjectCoordinator workflow executions。

必须保留：

- `auth_users`。
- `auth_sessions`。
- `web_login_logs`。
- 用户 profile、用户团队 scope、可登录账号所需的基础团队和权限数据。
- 其他不属于项目闭环且影响登录或控制台基础访问的数据。

清理脚本必须先打印待清理表和 count，再执行删除，最后回查 count。不能用无保护的全库 drop。

## 9. 幂等和错误处理

所有 activity 必须满足以下要求：

- 创建审批请求前先查是否已有同资源 pending request，或依赖唯一约束防重。
- 创建 `project_decision_requests` 时写入稳定关联字段，并能在重复调用时返回 existing record。
- 分解 accepted plan 使用现有 decomposition claim 保证精确一次。
- 创建补证任务必须使用稳定 planned key 或 idempotency key。
- 创建依赖边必须可重复执行。
- append event 如属于协调动作，应使用可查询的 source key 防止重复关键事件；纯 observed event 可接受重复但应控制数量。
- `HumanDecisionSubmitted` 遇到已 resolved decision 时必须 no-op，不能二次推进。

错误处理：

- 查询不到 decision request：记录 observed event，避免 workflow 崩溃。
- decision type 不支持：记录错误事件并返回非重试错误，避免无限 retry。
- 必要关联字段缺失，例如 `plan_review` 缺 `plan_revision_id`：返回明确错误，阻止错误推进。
- 外部 Provider 不可用不影响 coordinator Continue-As-New 设计；任务执行失败走 Runtime/Provider 写回和失败恢复分支。

## 10. 测试和验证

### 10.1 单元测试

- `HumanDecisionSubmitted` 不依赖 pending map，按 `project_decision_requests.decision_type` 路由。
- `plan_review` 通过 `plan_revision_id` 定位并 accept/reject/request changes。
- `task_failure_recovery` 通过 `project_task_id` 恢复。
- `project_acceptance` 通过 `project_id` 恢复。
- dispatch gate 决策通过 `dispatch_gate_result_id` 恢复。

### 10.2 Workflow 测试

- 设置低 history 或测试阈值触发 Continue-As-New。
- Continue-As-New 后仍能处理 `plan_review` 人类决策。
- Continue-As-New 后仍能处理 task failure recovery。
- Continue-As-New 后仍能处理 project acceptance。
- 重复 signal 不重复创建审批、任务、依赖或关键事件。

### 10.3 存储和迁移测试

- migration 增加 `project_decision_requests.plan_revision_id` 和索引。
- sqlc query 支持按 decision request 读取恢复所需字段。
- 创建 `plan_review` 决策时写入 `plan_revision_id`。

### 10.4 真实链路 smoke

实施完成后必须跑真实链路：

1. 启动 Temporal、Control Plane、Web、Runtime Agent。
2. 创建新 Project。
3. 提交 demand，生成 plan revision。
4. 走一次 plan review 人类决策。
5. 生成 ProjectTask DAG 并 dispatch root task。
6. 模拟或真实完成一个任务，触发下游解锁。
7. 覆盖至少一次补证或失败恢复路径。
8. 确认 Continue-As-New 后 workflow 仍能接收 signal 并推进。

如果真实 Runtime/Provider 不可用，不能声明 Runtime 执行链路可用；只能声明 coordinator 局部验证结果。

## 11. 风险

- 如果 `plan_review` 没有稳定 `plan_revision_id`，Continue-As-New 后无法可靠恢复审批上下文。
- 如果某些 activity 缺少幂等保护，Continue-As-New 和 retry 会放大重复写入问题。
- 如果清理脚本误删登录账号或权限数据，会影响 Web 登录；清理必须先 count、再执行、再回查。
- 如果只通过 workflow unit test，不足以证明真实项目协调链路可用。

## 12. 实施顺序建议

1. 补 migration、sqlc query 和 domain 字段。
2. 修改 `RequestPlanRevisionReview` 写入 `plan_revision_id`。
3. 新增 `ResolveHumanDecisionFromStore` 类 activity 或 store 方法，统一读取 decision request 和关联事实。
4. 改造 `HumanDecisionSubmitted` 路由，移除对长期 pending map 的依赖。
5. 加入 Continue-As-New 检查和 `Generation`。
6. 补 workflow/unit/storage 测试。
7. 编写开发库清理脚本或命令说明，并保留登录账号相关数据。
8. 跑真实链路 smoke。
