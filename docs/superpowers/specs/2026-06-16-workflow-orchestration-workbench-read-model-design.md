# 流程编排工作台与 Read Model 补强设计

日期：2026-06-16
状态：已确认，待实施计划

## 1. 摘要

本设计重构 SuperTeam Web 的“流程编排”体验，并补强 Control Plane 的只读聚合模型。

本轮选择“前端 + 后端 read model 补强”方案：保留现有调度、审批、Runtime 和 Provider 写链路，不新增流程写操作；把流程编排拆成“入口页流程实例卡片”和“详情页流程工作台”两个层级。

核心决策：

- `/workflows` 是流程实例入口页。卡片表示一个 demand/流程实例，不表示单个 ProjectTask。
- `/workflows/$demandId` 是流程工作台。顶部显示流程全局摘要，中间显示 ProjectTask DAG，右侧显示选中任务 Inspector。
- SLA、优先级、风险等级是可选增强字段。只有存在真实事实源时展示，缺失时不显示。
- 右侧 Inspector 第一版只读，提供跳转到审批、Runtime、项目资源等页面的链接，不直接处理审批、重试、取消或转派。

本设计延续 `2026-06-15-workflow-orchestration-graph-design.md` 已落地的基础接口和图模型，并把之前“左侧实例列表 + 右侧图”的单屏心智升级为更清晰的两级工作台。

## 2. 背景与当前状态

当前代码已有两条关键读接口：

- `GET /api/v1/workflow-instances`
- `GET /api/v1/projects/{projectId}/task-graph?demand_id={demandId}`

当前 Web 也已有最小流程页：

- `WorkflowInstanceList` 显示可见流程实例。
- `WorkflowDetail` 显示 demand 摘要和 task graph。
- `WorkflowGraphCanvas` 使用 `@xyflow/react` 渲染只读任务图。
- `WorkflowNodeInspector` 已能展示输入、输出、Run、结果和人工决策。

当前不足：

- `/workflows` 和 `/workflows/$demandId` 仍像同一个左右分栏页面，入口页没有形成“流程对象卡片”的入口心智。
- 流程实例列表信息密度不足，只能看标题、项目、状态和进度，缺少运行态判断所需的可选风险、SLA、优先级、最近事件摘要。
- 详情页节点详情通过 Dialog 展示，打断用户观察全局 DAG。
- DAG 没有明确阶段分组、并发分组和常驻右侧 Inspector。
- 后端 list read model 已有基础聚合，但没有把流程卡片所需的增强字段作为稳定契约表达。

## 3. 目标

### 3.1 产品目标

- 用户打开流程编排入口页时，能快速判断“哪个流程需要关注”。
- 用户进入某个流程后，能在同一个工作台里同时看到全局摘要、任务依赖图和选中任务详情。
- 单个任务的详细信息只在详情页出现，不在入口页展开，避免入口页变成任务列表。
- 流程卡片、任务节点卡片和右侧 Inspector 的层级清晰，避免卡片作为万能布局容器。

### 3.2 技术目标

- 保留现有 API 路由，向后兼容补强响应字段。
- 后端负责权限过滤、状态聚合、统计和可选字段解析，前端不重新推断业务事实。
- 前端用 read model 渲染，不在页面内持久化业务状态。
- 真实链路验证覆盖 Control Plane API、Web 页面和真实数据路径。

## 4. 非目标

本轮不做：

- 流程发起或 demand 创建重构。
- 在流程图内审批、驳回、重试、取消、转派、补证或编辑依赖。
- 画布拖拽布局持久化。
- SSE、WebSocket 或 Temporal 内部事件实时流。
- 引入新的调度状态机、Runtime 写链路或 Provider 执行协议。
- 把单个 ProjectTask 展开到入口页卡片。
- 为缺失的 SLA、优先级或风险等级生成假值。

## 5. 信息架构

### 5.1 入口页：流程实例卡片

路由：

```text
/workflows
```

入口页展示流程实例卡片网格和紧凑筛选工具栏。

卡片代表一个 demand/流程实例。卡片展示流程级信息：

- 标题。
- 所属项目。
- 提交人。
- 创建时间或最近更新时间。
- 总状态：`planning`、`running`、`waiting_human`、`failed`、`completed`、`cancelled`、`unknown`。
- 总进度：已完成 / 总节点。
- 汇总计数：运行中、等待人工、阻塞、失败。
- 当前阻塞摘要。
- 最近事件摘要。
- 可选优先级。
- 可选风险等级。
- 可选 SLA 信息。

入口卡片不展示：

- 单个任务标题。
- 单个任务负责人。
- 单个任务输入、输出、日志。
- 单个任务依赖边。
- 单节点完整阻塞详情。

入口页点击卡片进入：

```text
/workflows/$demandId
```

### 5.2 详情页：流程工作台

路由：

```text
/workflows/$demandId
```

详情页由三块组成：

1. 顶部全局摘要栏
   展示标题、项目、提交人、状态、进度、可选 SLA/优先级/风险、当前阻塞和打开项目动作。

2. 中央 DAG 画布
   展示 ProjectTask 节点、依赖边、阶段分组和并发关系。任务节点是节点卡片，表达任务级状态。

3. 右侧只读 Inspector
   展示选中任务的负责人、状态、输入要求、输出契约、阻塞、Run、执行结果、人工决策、最近事件和跳转链接。

桌面端默认布局：

```text
顶部：流程摘要栏
下方：左/中 DAG 画布 + 右侧 Inspector
```

移动端：

- 入口页保持单列卡片。
- 详情页画布优先显示，Inspector 作为抽屉或下方折叠面板。

## 6. 后端 Read Model 设计

### 6.1 保留接口

继续使用：

```text
GET /api/v1/workflow-instances
GET /api/v1/projects/{projectId}/task-graph?demand_id={demandId}
GET /api/v1/project-demands/{demandId}/launch-detail
```

不新增写接口。

### 6.2 WorkflowInstanceSummary 补强

现有 `WorkflowInstanceSummary` 保留所有字段，并增加可选字段。

本轮新增：

```text
WorkflowInstanceSummary {
  demand_id: uuid
  project_id: uuid
  project_name: string
  title: string
  submitted_by_user_id: uuid
  submitted_by_display_name: string
  status: WorkflowInstanceStatus
  status_reason: string
  created_at: date-time
  updated_at: date-time
  selected_coordination_job_id?: uuid
  progress: WorkflowInstanceProgress
  current_blocker?: WorkflowInstanceCurrentBlocker
  priority?: WorkflowInstancePriority
  risk?: WorkflowInstanceRisk
  sla?: WorkflowInstanceSLA
  recent_event?: WorkflowInstanceRecentEvent
}
```

#### WorkflowInstancePriority

```text
{
  value: string
  label: string
  source: string
}
```

来源优先级：

1. `project_demands.source_refs.priority` 或 `source_refs.severity`。
2. 最新 route decision 或 coordination job metadata 中的优先级字段。
3. planner metadata 中明确写入的优先级字段。

如果没有来源，后端返回空值，前端不展示。

#### WorkflowInstanceRisk

```text
{
  level: string
  label: string
  source: string
}
```

来源优先级：

1. 当前 demand 对应 task graph 中未完成任务的最高 `risk_level`。
2. 最新 route decision 或 planner metadata 中明确的 risk 字段。
3. 项目 policy 中对当前 demand 场景的默认风险，前提是 policy 有显式命中规则。

如果没有来源，后端返回空值。

#### WorkflowInstanceSLA

```text
{
  due_at?: date-time
  remaining_seconds?: int32
  breached: boolean
  label: string
  source: string
}
```

来源优先级：

1. `project_demands.source_refs.sla_due_at`。
2. `project_demands.source_refs.sla_minutes` + demand `created_at`。
3. coordination policy 中对当前 priority/risk 命中的显式 SLA 规则。

如果没有 due_at 或规则来源，后端返回空值。前端不能自己编 SLA。

#### WorkflowInstanceRecentEvent

```text
{
  event_type: string
  summary: string
  occurred_at: date-time
}
```

来源：

- 当前 demand 相关 project events 中最新一条可读事件。
- 优先选和 demand、coordination job、project task、decision request 直接相关的事件。

### 6.3 WorkflowInstanceProgress 补强

现有字段保留：

```text
total_nodes
completed_nodes
running_nodes
blocked_nodes
waiting_human_nodes
```

新增可选字段：

```text
planned_nodes?: int32
failed_nodes?: int32
cancelled_nodes?: int32
```

这些字段用于入口页卡片的状态摘要和排序，不改变既有状态枚举。

### 6.4 CurrentBlocker 规则

`current_blocker` 是流程级摘要，不是单任务详情。

选择优先级：

1. 未解决且需要人工处理的 `decision_request`。
2. `failed` 任务。
3. `blocked` 任务。
4. 超时 SLA。

返回字段：

```text
{
  type: string
  title: string
  resource_id?: uuid
}
```

`title` 应是可读摘要，例如“等待人工审批回滚方案”或“服务健康巡检失败阻塞下游”。不要返回完整日志或长 JSON。

### 6.5 ProjectTaskGraph 补强

现有 `ProjectTaskGraph` 保留：

```text
nodes
edges
employees
runs
execution_summaries
recent_events
decision_requests
```

新增可选顶层字段：

```text
stage_summaries?: ProjectTaskGraphStageSummary[]
```

结构：

```text
ProjectTaskGraphStageSummary {
  stage_index: int32
  title: string
  total_nodes: int32
  completed_nodes: int32
  running_nodes: int32
  waiting_human_nodes: int32
  blocked_nodes: int32
}
```

生成规则：

- 按 `ProjectTask.stage_index` 聚合。
- 没有 `stage_index` 的节点进入 `stage_index = -1` 的“未分阶段”组。
- title 默认是 `第 N 阶段`；如果 planner metadata 有明确阶段名，可使用阶段名。

`ProjectTaskGraphNode` 新增可选字段：

```text
status_reason?: string
updated_at?: date-time
current_blocker?: WorkflowInstanceCurrentBlocker
```

这些字段只用于详情页节点和 Inspector，不出现在入口页卡片。

### 6.6 权限边界

所有 read model 必须继续按当前 console user 可见项目过滤：

- 项目 human owner。
- 项目 leader。
- 项目 acceptance user。
- active human project member。

前端不得通过猜测 project ID 或 demand ID 读取不可见流程。

## 7. 前端设计

### 7.1 Feature 边界

继续使用：

```text
apps/web/src/features/workflows/
```

组件边界：

```text
WorkflowPage
WorkflowEntrancePage
WorkflowInstanceCardGrid
WorkflowInstanceCard
WorkflowWorkbenchPage
WorkflowSummaryBar
WorkflowGraphCanvas
WorkflowTaskNode
WorkflowStageGroup
WorkflowNodeInspector
workflow-graph-adapter.ts
workflow-status.ts
```

路由行为：

- `/workflows` 渲染入口卡片页。
- `/workflows/$demandId` 渲染工作台。
- 入口页卡片点击进入详情页。
- 详情页可以提供“返回流程列表”。

### 7.2 入口页卡片

入口页采用卡片网格，不用表格作为第一版主视图。

卡片视觉遵循 `DESIGN.md`：

- 卡片是业务对象入口，不是页面内任意容器。
- 状态用语义色、文字和图标共同表达。
- 不展示过多标签，主状态优先，SLA/风险/优先级作为次级信息。
- 卡片 hover 和 selected 不改变布局尺寸。

卡片内容结构：

```text
标题 + 主状态
项目 / 提交人 / 更新时间
进度条或完成数
运行中 / 等待人工 / 阻塞 / 失败 汇总
当前阻塞或最近事件
可选：priority / risk / SLA
```

入口页工具栏：

- 搜索。
- 状态筛选。
- 项目筛选。
- 排序：默认运行优先，可切换最近更新。

### 7.3 详情页工作台

顶部摘要栏展示流程级信息。它与入口卡片共享同一组 read model 字段，但可以更完整。

中央 DAG：

- 节点按阶段分组展示。
- 任务节点卡片展示标题、状态、负责人、summary、风险、人工决策提示。
- 连线表示依赖方向：`blocker_task_id -> dependent_task_id`。
- 选中节点后，不弹 Dialog，而是更新右侧 Inspector。

右侧 Inspector：

- 默认选中最高优先级节点：failed、blocked、waiting_human、running、planned，最后是第一节点。
- 展示任务完整详情。
- 提供跳转链接：
  - 审批中心：存在 decision request 时。
  - Runtime：存在 runtime task 或 run 时。
  - 项目详情：始终可跳转。
  - 证据、报告或工件：仅当 graph 的 events、summaries 或后续资源字段提供真实 ID 时。

Inspector 不提供写操作按钮。第一版不在此处理审批或重试。

## 8. 数据流

### 8.1 入口页

```text
/workflows
  -> listWorkflowInstances({ q, status, project_id, limit, offset })
  -> render WorkflowInstanceCardGrid
```

轮询：

- 默认 5 秒。
- 后台刷新保留旧数据，避免卡片闪烁。
- 用户正在输入搜索时不强制重置筛选。

### 8.2 详情页

```text
/workflows/$demandId
  -> listWorkflowInstances({ q, status, project_id, limit, offset })
  -> getProjectDemandLaunchDetail(demandId)
  -> getProjectTaskGraph(projectId, { demandId })
  -> render summary + graph + inspector
```

如果 `workflow-instances` 找不到当前 demand，但 `launch-detail` 有权限返回，则详情页仍可展示，并提示列表摘要暂不可用。

如果 task graph 为空：

- demand 已提交但未规划：显示规划中。
- coordination job 失败：显示失败摘要。
- 权限不足或接口失败：显示错误状态，不用 mock 图。

## 9. 状态与排序

### 9.1 状态聚合

服务端聚合状态，前端只展示。

优先级：

1. `failed`
2. `cancelled`
3. `waiting_human`
4. `running`
5. `planning`
6. `completed`
7. `unknown`

如果一个流程同时有 running 和 waiting_human，入口卡片主状态为 `waiting_human`，并在统计里保留 running count。

### 9.2 默认排序

默认排序：

1. waiting_human
2. failed
3. running
4. planning
5. unknown
6. completed
7. cancelled

同组按 `updated_at DESC`。

这个排序服务端可先返回运行优先顺序，前端只做稳定展示。若第一版后端只支持最近更新排序，则前端可以临时按服务端 status 排序，但必须在实现计划中标明为过渡。

## 10. 契约与生成

修改 OpenAPI：

- 补强 `WorkflowInstanceSummary`。
- 补强 `WorkflowInstanceProgress`。
- 新增 `WorkflowInstancePriority`。
- 新增 `WorkflowInstanceRisk`。
- 新增 `WorkflowInstanceSLA`。
- 新增 `WorkflowInstanceRecentEvent`。
- 补强 `ProjectTaskGraph`。
- 新增 `ProjectTaskGraphStageSummary`。
- 补强 `ProjectTaskGraphNode`。

字段兼容策略：

- 新增字段全部 optional，避免破坏已有客户端。
- 现有 required 字段不移除、不改名。
- 状态枚举本轮不新增，避免扩散到既有页面。

生成要求：

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
```

如果 sqlc query 改动：

```bash
make -C apps/control-plane generate-sqlc
```

## 11. 实施阶段

### 阶段 1：后端 read model 补强

- 扩展 domain types。
- 扩展 OpenAPI schema。
- 扩展 `ListWorkflowInstances` query 和 mapper。
- 扩展 task graph read model 的 stage summaries 和节点辅助字段。
- 补服务层状态、排序和可选字段解析测试。

### 阶段 2：前端入口页

- 将 `/workflows` 改为流程实例卡片网格。
- 添加工具栏筛选。
- 卡片只展示流程级摘要。
- 点击卡片进入 `/workflows/$demandId`。

### 阶段 3：前端详情页工作台

- 将详情页改成 summary bar + graph + right inspector。
- 去掉节点详情 Dialog 作为主路径。
- 添加阶段分组、选中态、空图、错误、加载中状态。
- Inspector 添加只读详情和跳转链接。

### 阶段 4：验证

- 后端单元测试和 handler/route 测试。
- Web 组件测试。
- 合同生成与验证。
- 真实 Control Plane + Web smoke。

## 12. 测试要求

后端：

- `ListWorkflowInstances` 对可选字段、状态优先级、排序、权限过滤、分页做测试。
- `GetProjectTaskGraph` 对 stage summaries、空图、节点辅助字段做测试。
- handler 测试覆盖新增 JSON 字段。
- route 测试覆盖 query 参数透传。

前端：

- 入口页渲染流程实例卡片，不渲染单任务详情。
- SLA/priority/risk 缺失时不显示。
- SLA/priority/risk 存在时显示。
- 点击卡片进入详情页。
- 详情页默认选中高优先级节点。
- 点击节点更新右侧 Inspector，不打开 Dialog。
- task graph 为空、加载中、加载失败都有稳定状态。

真实验证：

- 启动 Control Plane 和 Web。
- 使用真实认证请求 `GET /api/v1/workflow-instances`。
- 打开 `/workflows`，确认入口页加载真实列表。
- 打开一个真实 demand 的 `/workflows/$demandId`，确认 task graph、右侧 Inspector 和跳转链接来自真实接口。
- 如果没有可用真实 task graph，必须报告验证受阻，不能声称端到端可用。

## 13. 风险与约束

- SLA、优先级、风险字段来源可能短期不完整。通过 optional 字段和隐藏展示规避假数据。
- 入口卡片如果塞入任务详情，会破坏本设计的信息层级。实现时必须通过测试或代码评审防止单任务字段进入入口卡片主体。
- 右侧 Inspector 如果加入写操作，会扩大到审批、Runtime、恢复决策链路。本轮明确不做。
- DAG 大图可能需要自动布局库，但第一版应先复用现有确定性布局，避免引入新依赖。
- 既有 `2026-06-15` 计划中的 `/workflows` 左侧列表心智需要迁移为入口页卡片心智，不能两个体验并存。

## 14. 完成标准

本设计完成后的第一版应满足：

- `/workflows` 是流程实例卡片入口页。
- `/workflows/$demandId` 是流程工作台。
- 入口页不展示单个任务详情。
- 详情页不通过 Dialog 作为任务详情主路径。
- 后端 read model 返回入口卡片和详情工作台所需字段。
- optional 字段缺失时页面稳定且不显示假值。
- 真实接口和真实页面 smoke 通过，或者明确列出无法完成真实验证的阻塞依赖。
