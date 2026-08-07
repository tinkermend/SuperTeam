# 项目协调 DAG Planner 底座设计

日期：2026-06-14  
> 复核状态：锚点抽查：ProjectEventTaskGraphPlanned事件类型存在, 但ErrProjectTaskGraphPending暗示功能仍在进行中; 部分planner代码存在但可能未完全实施
状态：已确认，待实施计划

## 1. 摘要

本设计把当前项目协调线程里的单任务启发式 planner，升级为“DeepSeek 结构化规划 + 任务依赖 DAG + 治理闸门 + 自动就绪分派”的后端底座。

本轮交付只做底座：数据库、Control Plane domain/repository/service、Temporal workflow/activity、DeepSeek planner、DAG 分派、完成契约校验、失败恢复、追加式 replan、OpenAPI 和后端 `task-graph` read model。复杂 Web 流程图、数字员工任务卡片、run 过程和事件决策聚合体验不在本轮实现，等底座稳定后单独设计。

本设计作为 `/Users/tinker/.claude/plans/plan-enchanted-hedgehog.md` 的确认版和修正版。原 plan 的方向成立，但以下修正必须落实：

- 使用 OpenAI-compatible SDK 或协议调用 DeepSeek 模型，不调用 GPT 模型。
- 稳定路径使用 DeepSeek Chat Completions 的 `response_format: {"type":"json_object"}`，再由 Go 侧严格校验；不默认使用 OpenAI `json_schema` structured outputs。
- planner 的 API key、base URL、model 放入 Control Plane 配置结构，并允许环境变量覆盖。
- Web 复杂 UI 后置；本轮只做生成类型和必要编译适配。
- 后端 `task-graph` 聚合读接口纳入本轮底座。
- 失败恢复必须扩展 human decision payload，不能复用 route review 的简单语义。

DeepSeek 文档依据：

- OpenAI-compatible API 与 base URL：https://api-docs.deepseek.com/
- JSON Output：https://api-docs.deepseek.com/guides/json_mode
- Chat Completion `response_format` 仅列出 `text` 与 `json_object`：https://api-docs.deepseek.com/api/create-chat-completion

## 2. 目标

- 让项目协调线程把一个 demand 规划成多个结构化 ProjectTask，并带依赖边。
- 把任务图持久化为 Control Plane 业务事实，而不是只保存在 Temporal workflow 内存里。
- 只分派 ready root tasks；blocker 完成后自动释放下游任务。
- DeepSeek 输出必须先通过服务端 gates，才能落库或分派。
- ProjectTask 完成前必须校验任务级输出契约，避免不完整结果放行下游。
- blocker 失败时阻塞下游，并向项目 human owner 发起恢复决策。
- 支持追加式 retry、reassign 和 rework 子图，不改写历史任务，不在原图上造环。
- 提供后端 `task-graph` read model，作为后续 Web 设计基础。
- 保持 Runtime/Provider 边界：Runtime 执行任务，Control Plane 规划和治理任务。

## 3. 非目标

- 不做最终 Web DAG 视图、数字员工卡片、run timeline 或图布局。
- 不引入 LangChain、CrewAI、AutoGPT 或其他第三方 agent 框架。
- 不把本版协调规划放到 Runtime/Provider 中运行。
- 不用自建 scheduler、heartbeat poller 或锁清扫替代 Temporal。
- 不新增 task kind、provider type、capability type 或 project scenario 的数据库 enum。
- 不做重型 workspace finalize 屏障；本版用任务级 completion contract 兜底。

## 4. 架构边界

项目协调线程仍是 Project 的独占协调状态机。默认 WorkflowID 为 `project-coordinator:{project_id}`，除非项目记录显式给出 workflow ID。

Temporal workflow 只负责编排。它可以保存轻量的 pending route review 和 pending failure recovery 上下文，但不得直接查数据库、调用 DeepSeek、从持久化数据计算依赖就绪，也不得把完整任务图作为 workflow-local 事实源。

以下非确定性逻辑全部在 activity 内：

- 加载项目协调快照。
- 调用 route planner。
- 持久化 RouteDecision 和 ProjectTask graph。
- 计算可分派任务。
- 完成 blocker 后解析 ready downstream。
- 失败 blocker 后阻塞下游。
- 创建失败恢复决策请求。
- 追加恢复或返工子图。

DAG 事实源在 Control Plane：

- `project_tasks`
- `project_task_dependencies`
- 任务级 `expected_outputs`
- 任务级 `input_requirements`
- 任务级 `handoff_contract`
- 任务级 `planner_metadata`
- `project_route_decisions`
- `project_events`

Runtime 和 Provider 只在 Control Plane 分派 ready ProjectTask 后执行具体 run。Planner 不能绕过人类审核、审批、Runtime 租约或 Provider 分派规则。

## 5. 配置设计

Control Plane 配置新增 planner 块：

```yaml
planner:
  provider: "deepseek"
  apiKey: "${DEEPSEEK_API_KEY}"
  baseURL: "https://api.deepseek.com"
  model: "deepseek-v4-pro"
  maxTokens: 8192
  temperature: 0
  maxAttempts: 2
```

规则：

- 本轮 `provider` 支持 `deepseek`。后续其他 OpenAI-compatible provider 通过配置和 adapter 注册扩展。
- `apiKey`、`baseURL`、`model`、`maxTokens`、`temperature`、`maxAttempts` 进入 `config.Config`。
- 环境变量可以覆盖配置文件值，例如 `PLANNER_API_KEY`、`PLANNER_BASE_URL`、`PLANNER_MODEL`、`PLANNER_MAX_TOKENS`、`PLANNER_TEMPERATURE`、`PLANNER_MAX_ATTEMPTS`。
- `config.example.yaml` 只放非密钥示例值；真实本地密钥可放在被 git ignore 的 `config.yaml`。
- 缺少 `apiKey`、`baseURL` 或 `model` 时，生产 planner 回退到 `HeuristicRoutePlanner`，并在 planner metadata 中记录原因。
- 测试注入 fake planner，不调用 DeepSeek。

## 6. 数据模型

Migration 019 给 `project_tasks` 增加任务图字段：

- `coordination_job_id UUID`
- `route_decision_id UUID`
- `planned_task_key VARCHAR(100)`
- `task_kind VARCHAR(100)`
- `stage_index INTEGER`
- `expected_outputs JSONB NOT NULL DEFAULT '[]'::jsonb`
- `input_requirements JSONB NOT NULL DEFAULT '{}'::jsonb`
- `handoff_contract JSONB NOT NULL DEFAULT '{}'::jsonb`
- `planner_metadata JSONB NOT NULL DEFAULT '{}'::jsonb`

这些字段是任务级事实。`project_route_decisions.expected_outputs` 只保留整图或路由级摘要，不能替代任务级契约。

Migration 019 新增 `project_task_dependencies`：

```sql
CREATE TABLE project_task_dependencies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    coordination_job_id UUID,
    dependent_task_id UUID NOT NULL,
    blocker_task_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

索引与唯一性：

- 同一图内 planned key 唯一：`(tenant_id, project_id, coordination_job_id, planned_task_key)`，仅在 coordination job 和 key 存在时生效。
- 同一依赖边唯一：`(tenant_id, dependent_task_id, blocker_task_id)`。
- 按 dependent task 查询 blockers。
- 按 blocker task 反查 downstream。
- 按 coordination job 重建整张图。
- coordination job 按 trigger 幂等复用，route decision 按 coordination job 幂等复用。

`project_tasks.status` 仍是 `VARCHAR`，注释补充 `blocked`。状态流转继续由应用层校验，不引入 DB enum。

新增表和字段都写中文 COMMENT。关系完整性遵守 `DATABASE_DESIGN.md` 的应用层控制原则，本轮不加跨模块外键。

## 7. Repository 设计

Repository 新增图相关方法：

- `CreateProjectTaskGraph(ctx, request) (result, error)`
- `ListProjectTaskDependencies(ctx, tenantID, projectID, dependentTaskIDs)`
- `ListUnresolvedBlockerTaskIDs(ctx, tenantID, projectID, taskIDs)`
- `ListDependentsOfTask(ctx, tenantID, projectID, blockerTaskID)`
- `ListProjectTasksByCoordinationJob(ctx, tenantID, projectID, coordinationJobID)`
- `GetProjectTaskCompletionContract(ctx, tenantID, taskID)`
- `GetCoordinationJobByTrigger(ctx, tenantID, workflowID, triggerEventID, jobType)`
- `GetRouteDecisionByCoordinationJob(ctx, tenantID, coordinationJobID)`
- `GET /api/v1/projects/{projectId}/task-graph` 所需 read model 查询。

`CreateProjectTaskGraph` 是关键事务边界。它在同一个事务内完成：

1. 检查该 coordination job 是否已有完整图。
2. 创建所有任务节点。
3. 创建所有依赖边。
4. 为每个任务写一条 `project_task.created` 事件。
5. 为整张图写一条 `project_task.graph_planned` 事件。
6. 返回 task IDs、planned keys、stage indices 和 root/non-root 信息。

Temporal activity 重试时，如果完整图已存在，直接返回既有图，不产生新副作用。如果发现半图，返回冲突错误并升级人工处理，不能把半张图当作可执行事实。

## 8. Planner 设计

Planner 通过接口隔离：

```go
type RoutePlanner interface {
    Plan(ctx context.Context, snapshot CoordinationSnapshot) (RouteDecisionPlan, error)
}
```

实现：

- `DeepSeekRoutePlanner`：通过 OpenAI-compatible Chat Completions 调 DeepSeek。
- `HeuristicRoutePlanner`：确定性回退，能生成合法最小图。
- 测试用 fake planner。

`CoordinationSnapshot` 必须补充足够的员工画像，才能支持按能力选人。当前 pool 只有 role、status、display name，不够。此设计要求 snapshot 补充 Control Plane 已有的数字员工能力与画像字段，例如 employee type、role summary、skills、capability tags、status 和 project member settings。拿不到的字段要在 prompt 中明确为 unknown，不能让模型编造能力。

`RouteDecisionPlan` 升级为任务图结构：

```go
type RouteDecisionPlan struct {
    CandidateDigitalEmployeeIDs []uuid.UUID
    SelectedDigitalEmployeeIDs  []uuid.UUID
    Reason                      string
    Tasks                       []PlannedTask
    BudgetEstimate              map[string]any
    RequiresHumanReview         bool
    TemplateKey                 string
    PlannerPromptVersion        string
    PlannerMetadata             map[string]any
}

type PlannedTask struct {
    Key                string
    Title              string
    Summary            string
    SelectedEmployeeID uuid.UUID
    StageIndex         int32
    ExpectedOutputs    []string
    InputRequirements  map[string]any
    HandoffContract    map[string]any
    BlockedByKeys      []string
    RiskLevel          string
    TaskKind           string
}
```

为兼容现有代码，可以短期保留旧字段，但新的执行逻辑必须以 `Tasks` 为准。

## 9. DeepSeek JSON 路径

稳定路径使用：

- Chat Completions。
- `response_format: {"type":"json_object"}`。
- `stream: false`。
- 低 temperature，保证规划尽量稳定。
- 有界 `maxTokens`。
- system prompt 必须包含 `json` 字样、完整输出样例、字段说明和硬约束。

DeepSeek beta strict function calling 不作为默认路径。后续如果需要由 DeepSeek 服务端校验 function schema，可以单独评估；本轮事实边界是 Control Plane gates。

Planner prompt 约束：

- 只能从 snapshot 里的 active project digital employees 中选人。
- 不把人类审批、管理或验收职责建模成数字员工任务。
- 至少输出一个 root task。
- task 数量不超过 `MAX_TASKS`。
- 只有真实执行阻塞关系才写依赖边。
- 可并行的分支不要串联。
- `blockedByKeys` 只能引用同一输出图内的 key。
- 跨任务交接要求写入 `handoffContract`。
- 每个任务都要声明 task-level expected outputs。
- `taskKind` 使用开放字符串，不做封闭 enum。

## 10. Planner Gates

LLM 输出永远不能直接落库或分派，必须通过以下 gates：

- Gate 0：严格 JSON 解码到 Go 强类型结构，拒绝未知字段、缺字段和类型错误。
- Gate 1：planned task key 非空且唯一；selected employee 属于候选池、active 且可分派；blocked keys 存在且无自引用。
- Gate 2：图无环，且至少有一个 root task。
- Gate 3：每个任务都有 title、summary、selected employee、expected outputs、input requirements 和 handoff contract；task 数量在配置上限内。
- Gate 4：human review、risk level、budget estimate 与项目 coordination policy 不冲突。
- Gate 5：同一 coordination job 的重试不能生成与已持久化输入不兼容的 planned keys 图。

失败处置：

1. DeepSeek planner 最多重试 `maxAttempts`。
2. 失败后回退到 `HeuristicRoutePlanner`。
3. 回退仍无法生成合法图时，标记 coordination job failed，写审计事件，并请求人工介入。

## 11. Workflow 执行

`handleDemandSubmitted` 从“创建任务后全量分派”改为“创建图后只分派 ready tasks”。

新增或改造的 activities：

- `CreateProjectTasks` 使用 graph 语义，并接收 `CoordinationJobID` 和 `RouteDecisionID`。
- `ListDispatchableTasks` 返回本图内 status 为 `planned` 或 `pending` 且无未完成 blocker 的任务。
- `ResolveReadyDownstream` 查找 completed task 的 dependents，把新就绪任务从 `blocked` 改为 `planned`，并返回给 workflow 分派。
- `HoldDownstreamForFailure` 递归阻塞下游，并创建人类恢复决策。
- `AppendRecoverySubgraph` 创建追加式 retry、reassign 或 rework 子图。

人类 route review 仍然门控整张图。`RequiresHumanReview` 为 true 时，可以先持久化图并创建 review request，但不能分派任务。review approved 后，workflow 调 `ListDispatchableTasks`，只分派 root tasks。

`dispatchProjectTasks` 保留逐个分派和现有幂等行为。分派前 activity 需要校验 objective、prompt、assignee、demand、expected outputs 非空，并重新确认 selected digital employee 仍可分派，避免规划后员工状态漂移。

## 12. 完成契约屏障

`CompleteProjectTask` 在写 `completed` 前读取任务级契约并校验。

标准输出键：

- `execution_summary`：conclusion 非空。
- `evidence_refs`：evidence refs 非空。
- `artifact_refs`：artifact refs 非空。
- `recommended_next_action`：recommended next action 非空。
- `missing_information`：必须显式回写数组。
- `work_products`：绑定 run 必须存在结构化 work products。

`handoff_contract.required_refs` 中的自定义引用必须由 evidence refs、artifact refs 或 work products 满足。

契约不满足时：

- 不把任务标记为 `completed`。
- 不创建代表完成交接的 execution summary。
- 不 signal `EmployeeTaskCompleted`。
- 不释放下游任务。
- 写 `project_task.contract_missing` 事件。
- 任务保持 `running` 或 `assigned`；策略要求人工补证时转为 `waiting_human`。

只有契约满足后，现有完成写回才可以创建事件、创建 execution summary、标记 completed，并 signal workflow。

## 13. 下游唤醒

收到 `EmployeeTaskCompleted` 后，workflow：

1. 记录现有 observed signal 事件。
2. 调用 `ResolveReadyDownstream`。
3. 分派返回的新就绪任务。

`ResolveReadyDownstream` 必须幂等。重复完成 signal 不能重复创建 run、依赖边、事件或状态跃迁。已经 planned、assigned、running、completed、failed 或 cancelled 的下游任务不能被重复改成新就绪。

## 14. 失败恢复

收到 `EmployeeTaskFailed` 后，workflow：

1. 记录现有 observed signal 事件。
2. 调用 `HoldDownstreamForFailure`。
3. 按 project decision request ID 记录 pending failure recovery。

`HoldDownstreamForFailure`：

- 递归查找失败任务的所有下游任务。
- 保持或转为 `blocked`，已终态任务不改写。
- 创建 `decision_type=task_failure_recovery` 的 approval request 和 project decision projection。
- 目标处理人为项目 human owner。
- 通过现有 approval/inbox 通道写事件和收件箱投影。

`HumanDecisionSubmitted` 必须携带 payload：

```go
type HumanDecisionSubmitted struct {
    ApprovalRequestID uuid.UUID
    DecisionRequestID uuid.UUID
    Decision          string
    Payload           map[string]any
    ResolvedEventID   uuid.UUID
}
```

恢复动作从 payload 读取：

- `approved + recovery_action=retry`：创建同 assignee 的 retry 子图。
- `approved + recovery_action=reassign`：必须携带 `new_digital_employee_id`，校验后创建改派子图。
- `approved + recovery_action=cancel_downstream`：取消传递下游任务并记录取消事件。
- `rejected`：按策略停止恢复或取消下游，并记录决策。
- `needs_more_evidence`：下游保持 blocked，记录补证请求。

Failure recovery pending state 必须和 route review pending state 分开。workflow 根据 decision request ID 和 decision type 分流处理。

## 15. 追加式 Replan

返工不修改已完成任务，也不在原图上添加会形成环的边。

当 `B` 发现需要 `A` 补做时，系统追加新子图：

```text
A#1 -> B#1 -> A#2 -> B#2
```

新任务在 planner metadata 中记录：

- `source_task_id`
- `source_execution_summary_id`
- `recovery_reason`
- `supersedes_task_ids`
- `recovery_action`
- `parent_coordination_job_id`

`AppendRecoverySubgraph` 创建新的 coordination job，job type 可为 `replan_after_failure`、`replan_after_transfer` 或 `rework_requested`。新图可以依赖已完成任务作为上下文，但结构必须仍是 acyclic 和 append-only。

简单 retry 和 reassign 可以确定性生成。复杂 rework 可以调用 DeepSeek，但必须通过同一套 planner gates 和图级事务。

## 16. Task-Graph Read API

后端新增：

```text
GET /api/v1/projects/{projectId}/task-graph?coordination_job_id=...&demand_id=...
```

响应包含：

- `nodes`：task ID、planned key、stage index、kind、title、summary、status、risk、human approval flag、coordination job ID、route decision ID、expected outputs、input requirements、handoff contract、planner metadata。
- `edges`：dependent task ID、blocker task ID、coordination job ID、由 blocker 当前状态推导的 edge status。
- `employees`：后续 Web 卡片需要的 assigned digital employee 快照。
- `runs`：digital employee run ID、runtime task ID、run status、provider type、runtime node 摘要。
- `execution_summaries`：任务完成输出。
- `recent_events`：与图中任务和决策相关的项目事件。
- `decision_requests`：route review 和 failure recovery 决策。

这个接口只是 read model，不驱动调度。调度事实仍以 `project_tasks`、`project_task_dependencies`、Runtime writeback 和 project events 为准。

OpenAPI 同步新增 `ProjectTask` 字段和 `ProjectTaskGraph` schema。生成 Go/server types 和 Web API client。Web 只做类型兼容和现有页面编译适配，不做复杂 UI。

## 17. 实施阶段

1. Schema 与 repository 底座：migration 019、sqlc、domain types、图级事务、幂等和依赖查询。
2. Planner 与 gates：config、`RoutePlanner`、DeepSeek JSON planner、启发式回退、gates、prompt version、fake tests。
3. DAG dispatch：只分派 root、route review 门控整图、下游就绪 activity。
4. Completion contract：契约读取、标准输出校验、missing-contract 事件、无效完成不 signal。
5. Failure recovery：阻塞下游、创建 `task_failure_recovery` 决策、human decision signal 支持 payload。
6. Recovery 与 replan actions：retry、reassign、cancel downstream、追加式子图。
7. Task-graph API：后端 read model、OpenAPI、生成客户端、最小 Web 兼容。
8. 真实 smoke：迁移后启动 Temporal worker、Control Plane、Runtime/Provider，并用 DeepSeek planner 跑一个真实两任务 DAG。

## 18. 验证要求

分阶段验证：

- 修改 sqlc query 后运行 `make -C apps/control-plane generate-sqlc`。
- 修改 OpenAPI 后运行 `pnpm generate:control-plane`。
- 修改契约后运行 `pnpm verify:contracts`。
- 为 planner gates、repository graph transaction、workflow DAG、completion contract、failure recovery 写聚焦 Go 测试。
- 跨多个 Go 包时运行 `go test ./apps/control-plane/...` 或仓库对应 control-plane verify target。
- 生成 Web client 或做 Web 类型适配时运行 `pnpm verify:web`。

最终后端底座 smoke：

1. 启动真实 DB、Temporal、Control Plane、一个 Runtime 和一个 Provider。
2. 应用 migration 019。
3. 配置 DeepSeek `baseURL`、model 和 API key。
4. 提交一个由 DeepSeek 规划成至少两个任务的 demand，其中 task 2 被 task 1 阻塞。
5. 确认 task 1 被分派，task 2 为 `blocked`。
6. task 1 带齐 required outputs 完成后，确认 task 2 自动释放并分派。
7. 提交缺少 required evidence 或 artifact 的无效完成，确认下游不会被释放。
8. 让 blocker 失败，确认下游保持 blocked，并向 human owner 产生 `task_failure_recovery` 决策。
9. 用 retry、reassign 或 cancel downstream 处理恢复决策，确认追加式图行为正确。
10. 收尾前运行 `.codex/skills/superteam-completion-check/SKILL.md`。

如果 DeepSeek、Temporal、Runtime、Provider 或安全迁移目标不可用，则不能声明底座可用。最终状态必须说明缺失依赖，不能把本地单元测试当作端到端验证。

## 19. 风险与缓解

- DeepSeek JSON 可能 malformed 或空内容。缓解：有界重试、严格 decode、gates 和启发式回退。
- 员工画像不足会让按能力选人变成猜测。缓解：snapshot 补真实 capability/profile 字段，缺失字段在 prompt 中显式 unknown。
- Temporal activity 重试可能重复写图。缓解：图级事务和幂等索引。
- completion contract 可能阻塞现有 runtime writeback。缓解：历史任务空契约的处理规则必须在迁移行为和测试中明确。
- 失败恢复可能和 route review 混淆。缓解：独立 decision type、payload schema、workflow pending map 和测试。
- `task-graph` read API 可能被误用成调度器。缓解：文档和测试都明确它只读，不参与调度。
- Web 范围可能过早膨胀。缓解：本轮只做后端底座、生成类型和编译适配。

## 20. 已确认决策

本次 brainstorming 已确认：

- 按原全量目标推进，但拆分成可审查的开发阶段。
- 第一版底座包含真实 DeepSeek planner，并保留启发式回退。
- 使用 OpenAI-compatible SDK 或协议调用 DeepSeek，不调用 GPT 模型。
- 稳定结构化输出路径为 `json_object` 加 Go 侧 gates。
- planner API key、base URL、model 放入 Control Plane 配置。
- 复杂 Web 设计和实现后置。
- 后端 `task-graph` read API 纳入底座。
