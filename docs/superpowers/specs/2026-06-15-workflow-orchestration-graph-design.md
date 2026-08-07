# 2026-06-15-workflow-orchestration-graph-design
> 复核状态：未找到明确的工作流编排图相关锚点

## 1. 阶段目标

本设计定义 SuperTeam 第一版“流程编排”运行观察页。

任务发起提交后，用户不再停留在“任务发起详情”心智里，而是跳转到流程编排详情页。流程编排页左侧展示当前用户可见的所有 demand/流程实例，右侧展示当前流程实例的规划、执行和结果状态。右侧主视图使用 `@xyflow/react` 展示 ProjectTask DAG，并以附着信息表达审批、阻塞、Runtime run、执行结果和工件。

第一版目标是把已经落地的 ProjectDemand、CoordinationJob、RouteDecision、ProjectTask graph、DecisionRequest、ExecutionSummary、ProjectEvent 等控制平面事实组织成可观察的运行视图，不在画布里直接改变业务状态。

## 2. 产品定位

### 2.1 入口心智

SuperTeam Web 保留两个清晰入口：

- **任务发起**：创建入口。用户提交一个需求事实，由项目协调线程规划后续工作。
- **流程编排**：运行观察入口。用户查看当前可见的所有流程实例、规划图、执行状态、阻塞点和结果。

任务发起页只负责提交 demand。提交成功后跳转到流程编排详情页：

```text
/task-launches -> POST demand -> /workflows/$demandId
```

### 2.2 路由

新增主路由：

```text
/workflows
/workflows/$demandId
```

`/workflows` 默认选中运行优先排序后的第一条流程实例。`/workflows/$demandId` 直接选中指定 demand 对应的流程实例。

现有 `/task-launches/$demandId` 不再作为主体验。第一版保留兼容路由，重定向到 `/workflows/$demandId`，避免“任务发起详情”和“流程详情”两套心智并存。

## 3. 范围边界

### 3.1 第一版包含

后端：

- 新增当前用户可见流程实例列表聚合接口。
- 服务端负责权限过滤、运行优先排序和流程实例聚合状态。
- 复用现有 launch-detail 和 task-graph read model 支撑详情页。

前端：

- 新增“流程编排”一级导航入口。
- 任务发起提交成功后跳转 `/workflows/$demandId`。
- 新增流程编排列表 + 详情布局。
- 引入 `@xyflow/react` 渲染只读 ProjectTask DAG。
- 支持规划中、空图、加载失败、轮询刷新和节点详情查看。
- 支持轻量跳转动作：去审批、打开项目、打开 Runtime run、查看工件或事件详情。

### 3.2 第一版不包含

第一版不做：

- SSE 或 WebSocket 实时推送。
- 画布布局持久化。
- 画布内直接审批、驳回、重试、取消下游或补证。
- 跨流程批量操作。
- 把 Temporal workflow 内部步骤作为主图节点。
- 拖拽编辑流程、人工连线或修改任务依赖。

## 4. 信息架构

流程编排页面采用左右结构。

左侧是流程实例列表，展示当前用户可见的所有 demand/流程实例。列表支持搜索、状态筛选和项目筛选。默认排序是运行优先：

1. 规划中。
2. 执行中。
3. 等待人工。
4. 失败。
5. 已完成或历史。

同一状态组内按最近更新时间倒序。

右侧顶部是当前 demand 摘要，包含标题、项目、发起人、整体状态、进度和轻量跳转操作。

右侧主体是流程图画布。画布以 ProjectTask DAG 为主干，按阶段和依赖展示任务执行关系。审批、阻塞、结果、工件和 Runtime run 作为 badge、小型附着节点或详情面板字段展示。

右侧详情面板展示选中节点的完整事实，包括输入要求、输出契约、handoff contract、负责人、run、执行摘要、决策请求、最近事件和跳转动作。

## 5. 后端 API 与数据流

### 5.1 流程实例列表接口

新增聚合接口：

```text
GET /api/v1/workflow-instances
```

第一版查询参数：

```text
q
project_id
status
limit
offset
```

返回当前用户可见的 demand/流程实例摘要。服务端必须根据当前用户权限过滤项目和 demand，不把不可见项目内流程暴露给前端。

建议响应结构：

```text
WorkflowInstanceSummary {
  demand_id
  project_id
  project_name
  title
  submitted_by_user_id
  submitted_by_display_name
  status
  status_reason
  created_at
  updated_at
  selected_coordination_job_id
  progress {
    total_nodes
    completed_nodes
    running_nodes
    blocked_nodes
    waiting_human_nodes
  }
  current_blocker {
    type
    title
    resource_id
  }
}
```

`status` 由服务端聚合，前端只展示，不自行推断业务事实。第一版状态集合：

```text
planning
running
waiting_human
failed
completed
cancelled
unknown
```

聚合优先级：

1. 有失败或取消的终止事实时为 `failed` 或 `cancelled`。
2. 有未解决人类决策请求时为 `waiting_human`。
3. 有 running 或 assigned 任务时为 `running`。
4. demand 已提交但还没有可展示 graph nodes 时为 `planning`。
5. 图中任务全部完成且没有未解决决策时为 `completed`。
6. 数据不足时为 `unknown`。

### 5.2 详情数据流

现有接口继续作为详情事实源：

```text
GET /api/v1/project-demands/{demandId}/launch-detail
GET /api/v1/projects/{projectId}/task-graph?demand_id={demandId}
```

详情页加载流程：

1. 打开 `/workflows/$demandId`。
2. 请求 `workflow-instances`，左侧选中当前 demand。
3. 请求 `launch-detail`，拿 demand、project、coordination jobs、route decisions、project tasks、decision requests 和 recent events。
4. 如果 `launch-detail.project_tasks` 为空，或 task graph 返回空 nodes，画布显示规划中状态。
5. 一旦 task graph 有 nodes，调用 `task-graph?demand_id=...` 渲染 DAG。
6. 页面每 3 到 5 秒轮询列表摘要和当前详情图。后台刷新不得卸载旧图。

## 6. 前端设计

新增 feature 边界：

```text
apps/web/src/features/workflows/
```

核心组件：

- `WorkflowPage`：路由容器，读取 URL、拉取列表、选择默认流程。
- `WorkflowInstanceList`：左侧流程实例列表，处理搜索、筛选和运行优先展示。
- `WorkflowDetail`：右侧详情壳，组合顶部摘要、画布和详情面板。
- `WorkflowGraphCanvas`：封装 `@xyflow/react`，只接收转换后的 nodes 和 edges。
- `WorkflowTaskNode`：ProjectTask 主节点卡片。
- `WorkflowAttachmentNode`：审批、阻塞、结果和工件等小型附着节点。
- `WorkflowNodeInspector`：选中节点详情和跳转动作。
- `workflow-graph-adapter.ts`：把 `ProjectTaskGraph` 转成 xyflow nodes 和 edges。

`apps/web/src/lib/api/projects.ts` 或相邻 API 模块需要新增：

- `listWorkflowInstances`
- `getProjectTaskGraph`

如果引入新的 workflow API 模块，仍应复用现有 `ApiClientOptions`、`getJson`、query 参数编码和测试风格。

## 7. xyflow 图模型

`WorkflowGraphCanvas` 使用受控 nodes/edges。画布只读，不把拖拽位置写回服务端。

节点 ID：

```text
task:{projectTaskId}
attachment:{type}:{resourceId}
```

主边来自 `ProjectTaskGraphEdge`，方向是：

```text
blocker_task_id -> dependent_task_id
```

第一版布局使用确定性布局函数，不新增 dagre 或 elk 依赖。布局规则：

- 按 `stage_index` 分层。
- 同一阶段横向排布。
- 没有 `stage_index` 的节点放入最后一个未分阶段区域。
- 附着节点靠近关联 ProjectTask，避免抢占主 DAG 层级。

后续如果真实图规模变大，再单独引入自动布局库。

状态视觉遵循 `DESIGN.md` 的企业控制台语义色：

- `completed`：绿色。
- `running`、`assigned`：蓝色。
- `blocked`、`waiting_human`、`pending`：琥珀。
- `failed`、`cancelled`：红色。
- `planned`、`queued`、未知：灰蓝。

ProjectTask 节点展示：

- 标题。
- 状态。
- 数字员工或负责人。
- summary。
- 风险和人工审批 badge。
- 输入/输出摘要。
- 最近更新时间或最近事件摘要。

选中节点后，详情面板展示完整信息，不把长 JSON 和完整 contract 塞进节点卡片。

## 8. 交互设计

画布支持：

- pan。
- zoom。
- fit view。
- 点击节点。
- 点击边查看依赖状态。

画布不支持：

- 拖拽保存布局。
- 直接编辑节点。
- 画布内审批、重试、取消、补证。

轻量动作只做跳转：

- 去审批。
- 打开项目。
- 打开 Runtime run。
- 查看工件。
- 查看事件详情。

轮询刷新时保持当前选中节点。如果选中节点不再存在，优先选择失败、等待人工、运行中的节点；仍没有时选择第一条主任务节点。

## 9. 状态与错误处理

规划中状态分层展示：

- demand 已提交但没有 coordination job：显示“等待项目协调线程接收”。
- 有 coordination job 但没有 graph nodes：显示“任务正在规划”。
- 有 graph nodes：显示流程图。

错误处理：

- 列表加载失败：左侧显示可重试错误，右侧不清空当前 URL 的详情加载状态。
- 详情加载失败：右侧显示局部错误，不清空左侧列表。
- 轮询失败：保留旧数据，在顶部显示轻量刷新失败状态。
- graph API 失败：保留 demand 摘要，画布局部显示流程图加载失败并提供重试。
- sidecar 缺失：节点仍展示，详情字段标为“尚未产生”或“未绑定”，不伪造数据。

后台刷新必须保留旧数据，避免刷新时画布闪烁或选中状态丢失。

## 10. 测试计划

Web：

- 任务发起提交成功后导航到 `/workflows/$demandId`。
- `/task-launches/$demandId` 兼容重定向到 `/workflows/$demandId`。
- 左侧流程实例列表按运行优先排序展示。
- 搜索、状态筛选、项目筛选使用正确 query 参数。
- 规划中空状态根据 launch-detail 和 task graph 正确显示。
- `ProjectTaskGraph` 能转换成 xyflow nodes/edges。
- 点击节点更新详情面板。
- 轮询刷新保留当前选中节点。
- graph API 失败时保留 demand 摘要并显示局部错误。

API client：

- `listWorkflowInstances` 正确编码查询参数。
- `getProjectTaskGraph(projectId, { demandId })` 正确编码 `demand_id`。
- fetch 失败时沿用现有 API error 行为。

Control Plane：

- `GET /api/v1/workflow-instances` 只返回当前用户可见 demand。
- 服务端按运行优先排序。
- 无 graph、部分 graph、失败任务、等待人工和完成任务都能得到正确聚合状态。
- 进度摘要按 task graph 节点统计。
- 分页和筛选不会绕过可见性过滤。

## 11. 真实验证要求

本功能后续实现完成时，不能只用 mock 或组件测试声明可用。至少需要完成：

1. 启动当前 Web、Control Plane 和开发数据库。
2. 确认 Web 使用当前 Control Plane URL。
3. 提交一个真实 demand，确认提交成功后跳转 `/workflows/$demandId`。
4. 规划未完成时确认页面显示真实规划中状态。
5. task graph 生成后确认画布显示真实 nodes 和 edges。
6. 用浏览器或 curl 确认相关 API 来自运行中的当前代码，不是 mock、缓存或旧服务。

如果真实 Runtime/Provider 不可用，只能说明流程图读取和展示链路已验证，不能声明完整执行链路可用。

## 12. 实施顺序建议

1. 后端 `workflow-instances` read API 和聚合状态测试。
2. Web API client 类型和测试。
3. 新增 `/workflows` 路由、导航入口和提交后跳转。
4. 流程实例列表和规划中详情壳。
5. `@xyflow/react` 依赖、图 adapter、只读画布和节点详情。
6. 轮询刷新、选中节点保留和错误状态。
7. 真实链路 smoke。

## 13. 验收标准

- 用户从任务发起提交后进入流程编排详情，而不是任务发起详情。
- 流程编排左侧能看到当前用户可见的所有流程实例，并按运行优先排序。
- 未规划完成时不伪造节点，只显示规划中状态。
- 规划完成后右侧画布展示真实 ProjectTask DAG。
- 节点详情能追踪负责人、输入输出、审批、阻塞、run、结果和事件。
- 第一版只提供跳转类轻量动作，不在画布中直接改变业务状态。
- 刷新和轮询不会导致画布主内容闪烁或选中状态无故丢失。
