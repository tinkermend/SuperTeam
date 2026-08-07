# 项目 Runtime Placement 与协调闭环设计

日期：2026-07-04
> 复核状态：CHANGELOG 2026-07-05 04:42记录项目Runtime Placement做成一等配置与诊断事实；锚点抽查发现project_placements表与相关索引
状态：待用户审阅，审阅通过后进入实现计划

## 1. 背景

2026-07-04 的真实端到端 smoke 创建了三个运维团队、三个数字员工、一个“系统健康分析项目”和一条“深入分析当前操作系统整体健康运行状态”需求。团队、员工、项目和需求都能创建成功，Runtime Agent 也在线，`local-dev-node` 的 command channel 连接正常，并报告 `codex` provider 可用。

但项目协调没有形成最小闭环：

- Project 配置里有三个 active executor 数字员工。
- Workflow 事件记录 `workflow.signal_failed`。
- Planner raw response 提示 `digital_employee_pool is empty`，并选择了 nil employee。
- task graph 没有 nodes、edges、runs 或 employees。
- route decisions 为空。
- 三个数字员工治理状态为 ready/approved，但执行侧为 `missing`、`pending_binding`、`can_dispatch=false`。

这说明堵塞不在“团队/员工/项目是否能创建”，也不在 Runtime daemon 基本在线状态，而在项目执行准备度没有成为一等产品事实，导致规划、派发和前端诊断用不同事实判断。

## 2. 根因判断

### 2.1 项目员工池被规划前置过滤清空

`ProjectStore.LoadProjectCoordinationSnapshot` 会先读取项目成员，再调用 `runtimeReadyEmployeeIDs`，只把 runtime-ready 的 active 数字员工放入 `DigitalEmployeePool`。当前 `employeeRuntimeSnapshotReady` 要求同时满足：

- employee policy allowed。
- Runtime node online。
- provider available。
- workspace ready。
- slot available。
- contract version accepted。

这把“适合参与规划”的员工和“此刻可以真实派发”的员工合并成了一个条件。结果是：项目明明有 executor 成员，但只要项目 Runtime placement 或 workspace 还没准备好，Planner 输入就变成空池。

### 2.2 存在计划-派发循环依赖

新的项目任务链路已经倾向于 ProjectTask attempt 按项目 Runtime placement 选择 Runtime，旧的数字员工 execution instance 绑定路径也已经不应再作为员工身份绑定 Runtime 的事实源。

但当前规划前置检查仍然通过 `GetEmployeeRuntimeSnapshot` 判断员工是否 runtime-ready。该方法优先走 project-task preflight，缺失时回退旧 execution instance。对新项目来说，ProjectTask 还没规划出来，preflight 事实天然缺失；旧 execution instance 又不应继续作为新架构主事实。于是形成循环：

1. 要规划出 ProjectTask，必须先有 runtime-ready 员工。
2. 要判断 runtime-ready，最好有 project-task preflight 或 project placement。
3. 但 project-task preflight 要等 ProjectTask 规划后才有。

### 2.3 项目 Runtime Placement 数据层存在，但缺产品闭环

数据库已有 `project_placements`，sqlc 也有 `UpsertProjectPlacement` 和 `GetActiveProjectPlacement`。这说明模型上已经认可“Project 到 Runtime 节点的 active placement”。

缺口是：

- 项目创建或项目配置没有稳定入口让用户绑定/切换 Runtime placement。
- 协调器没有把缺 placement 表达成结构化阻塞事实。
- Planner 输入没有明确区分规划资格和派发就绪。
- 前端没有展示项目执行准备度与下一步修复动作。

### 2.4 前端把失败呈现成无限规划中

Workflow 页面显示“规划中 / 等待 Control Plane 继续写入协调事实”，项目事件流只显示泛化记录。用户看不到：

- `workflow.signal_failed`。
- `digital_employee_pool is empty`。
- 项目缺 active Runtime placement。
- 员工缺 execution readiness。
- Runtime/provider/workspace/slot/contract 哪一项失败。

这使真实故障被隐藏成“流程还在等”，不利于用户判断产品开发进度。

## 3. 目标

- Project Runtime Placement 成为一等业务配置和诊断事实。
- Planner 能看到项目内可规划的数字员工池，不因暂时不可派发而被清空。
- Dispatcher 在真实执行前继续严格校验 Runtime、Provider、workspace、slot、contract version。
- 所有协调阻塞都必须持久化为结构化 fact，并能在项目详情、workflow 页面和 task graph 中看到。
- 本地 dev 场景支持把项目绑定到当前在线 `local-dev-node`，形成最小真实闭环。
- 验证必须覆盖 Web、Control Plane、数据库、Temporal 协调、Runtime Agent 和 provider 执行路径，不能只用 mock 或单元测试宣称完成。

## 4. 非目标

- 不把数字员工重新绑定到固定 Runtime。
- 不让 Control Plane 执行本机命令。
- 不让 Runtime Agent 决定项目成员选择、规划策略、人类审批或长期业务状态。
- 不实现多 Runtime 自动调度、跨 Runtime fallback 或复杂容量优化。
- 不把项目类型做成封闭枚举；系统健康分析只是本次 smoke 场景。
- 不删除旧 execution instance 兼容读模型；本设计只把它从新项目协调主路径移开。

## 5. 推荐方案

采用“项目 Runtime Placement 一等化 + 规划资格/派发就绪拆分”。

核心原则：

- **规划资格**回答“这个数字员工是否应该进入项目任务分解与路由候选池”。
- **派发就绪**回答“这个数字员工此刻能否在某个 Runtime 上真实执行一次 ProjectTask attempt”。

Planner 输入只依赖规划资格。Dispatcher 创建 attempt/run 前再执行派发就绪检查。

## 6. 后端设计

### 6.1 Project Runtime Placement Service

新增或补齐项目 Runtime placement 应用服务，围绕已有 `project_placements` 表提供能力：

- 查询项目 active placement。
- 绑定项目到 Runtime node。
- 停用或替换 active placement。
- 返回 placement readiness 诊断。

建议接口：

- `GET /api/v1/projects/{project_id}/runtime-placement`
- `PUT /api/v1/projects/{project_id}/runtime-placement`
- `DELETE /api/v1/projects/{project_id}/runtime-placement`

`PUT` 输入包含：

- `runtime_node_id`
- `reason`
- `expected_provider_types` 可选，用于前端预检查和审计说明。

服务端校验：

- Project 属于当前 tenant。
- Runtime node 属于当前 tenant 或允许的本地 dev tenant 规则。
- Runtime 最近心跳未过期。
- command channel connected。
- Runtime capability 覆盖项目成员池所需 provider。

写入：

- `project_placements` active record。
- project event：`project.runtime_placement.updated`。
- audit record：包含操作者、旧 placement、新 placement、reason。

### 6.2 项目执行准备度读模型

新增 project execution readiness 聚合读模型，供 Web 和协调器复用。

输出建议：

- `placement_status`: `missing | ready | runtime_offline | command_channel_disconnected | provider_unavailable | capacity_full | workspace_pending | contract_mismatch`
- `runtime_node_id`
- `runtime_node_name`
- `command_channel_connected`
- `provider_capabilities`
- `required_provider_types`
- `employee_readiness[]`
- `blocking_reasons[]`
- `next_actions[]`

该读模型不替代 pre-dispatch gate；它是可观察性和预检查。

### 6.3 规划资格

`LoadProjectCoordinationSnapshot` 调整为：

- 先收集 active project members。
- 只按 principal type、project role、member status、治理状态和 provider 固定事实判断规划资格。
- 不再因为 Runtime placement 缺失、workspace 未就绪、slot 不足而把员工排除出 Planner pool。
- 在 `PlanningProfile` 中附加 readiness hints：provider、placement status、runtime readiness summary。

如果没有任何可规划员工，协调器写入结构化阻塞：

- event type：`coordination.blocked`
- reason code：`no_plannable_digital_employee`
- human message：项目没有可参与规划的数字员工

### 6.4 派发就绪

`DispatchProjectTask` 或其 run-starter adapter 在创建真实 run/attempt 前执行强 gate：

- Project active placement 存在。
- Runtime node online。
- command channel connected。
- Runtime 支持数字员工固定 provider。
- Runtime 有可用 slot。
- project workspace 可 materialize。
- Runtime/provider contract version accepted。

失败时不返回泛化错误，也不让 task graph 空白。必须写入：

- `project_route_decisions` 或等价协调决策：`blocked`。
- project event：`project_task.dispatch_blocked`。
- task graph blocking node。
- 若已有 ProjectTask，则 ProjectTask 状态进入 `blocked` 或 `waiting_human`，而不是停在不可解释的 planned/empty。

### 6.5 Workflow Signal 失败处理

`workflow.signal_failed` 应保留底层错误，但还要投影成业务可读事实：

- `workflow.coordination_failed`
- `reason_code`
- `raw_error`
- `recommended_action`

当错误是空员工池、缺 placement、provider 不可用等已知类型时，写入标准 reason code，前端不直接解析 raw planner response。

## 7. 前端设计

### 7.1 项目创建与配置

项目创建完成页或项目设置页增加 Runtime Placement 区块：

- 当前绑定 Runtime。
- Runtime 在线状态。
- command channel 状态。
- provider capabilities。
- 绑定/更换按钮。

本地 dev 可提供“绑定当前本机 Runtime”快捷操作，实际仍调用同一个 API。

### 7.2 项目详情执行准备度

项目详情页显示执行准备度：

- Ready：项目可发起真实执行。
- Missing placement：请选择 Runtime。
- Runtime offline：Runtime 未在线。
- Provider unavailable：当前 Runtime 不支持某些员工 provider。
- Workspace pending：工作区未准备。
- Capacity full：执行槽位不足。

每个状态给一个明确动作，例如“绑定 Runtime”“切换 Runtime”“检查 Runtime Agent”“减少并发或等待槽位”。

### 7.3 Workflow 与任务图

Workflow 页面不能只显示“规划中”。它应读取 coordination blocking facts：

- 若规划失败，显示失败原因。
- 若派发阻塞，显示阻塞 node 和 next action。
- 若等待人工确认，进入 waiting_human。

Task graph 即使没有真实 run，也应显示 blocking node，避免空图误导。

## 8. 数据流

### 8.1 正常闭环

1. 用户创建数字员工，固定 provider，例如 `codex`。
2. 用户创建项目并加入数字员工池。
3. 用户在项目配置中绑定 Runtime placement，例如 `local-dev-node`。
4. 用户提交 ProjectDemand。
5. Coordinator 加载可规划员工池。
6. Planner 产出 route decisions 和 ProjectTasks。
7. Dispatcher 对选中员工做 pre-dispatch gate。
8. Control Plane 创建 ProjectTask attempt 和 DigitalEmployeeRun。
9. Runtime Agent 领取并执行 provider。
10. Runtime 写回事件、日志、工件和结果。
11. Project task graph、workflow、project summary 展示完成事实。

### 8.2 缺 placement

1. 用户提交 ProjectDemand。
2. Coordinator 能看到可规划员工池。
3. Planner 可形成任务意图，或协调器提前发现无法派发。
4. 系统写入 `project_task.dispatch_blocked`，reason=`runtime_placement_missing`。
5. 前端显示“项目未绑定 Runtime”，提供绑定动作。

### 8.3 Runtime/provider 不可用

1. 项目有 placement。
2. Runtime 离线、command channel 断开或不支持 provider。
3. pre-dispatch gate 阻塞。
4. route decision/task graph 显示具体阻塞项。
5. 用户修复 Runtime 后可重试或重新协调。

## 9. 错误处理

错误必须分层：

- **配置缺失**：缺 placement、项目员工池为空、员工 provider 缺失。
- **Runtime 传输失败**：node offline、command channel disconnected。
- **能力不匹配**：provider unavailable、contract mismatch。
- **资源不足**：slot unavailable、capacity full。
- **工作区失败**：workspace materialization/sync failed。
- **Planner 失败**：LLM/planner raw error、输出 schema 不合法。

每类错误都应有稳定 reason code、中文展示文案和推荐动作。raw error 只作为调试信息，不作为前端主文案。

## 10. 测试与验证

### 10.1 后端测试

- `ProjectRuntimePlacementService` 查询、绑定、替换、停用测试。
- placement 缺失时的 readiness 聚合测试。
- Runtime offline/provider unavailable/capacity full 的 readiness 测试。
- `LoadProjectCoordinationSnapshot` 测试：不可派发员工仍进入可规划 pool，并携带 readiness hints。
- dispatch preflight 测试：缺 placement 或 Runtime 不可用时写入 blocked decision/event。
- workflow signal failure 投影测试。

### 10.2 前端测试

- 项目配置页绑定 Runtime。
- 项目详情执行准备度展示。
- Workflow 页面显示 coordination blocked，而不是无限规划中。
- Task graph 显示 blocking node。

### 10.3 真实端到端验证

完成实现后必须用当前运行栈做真实 smoke：

1. `scripts/dev-services.sh status` 确认 Temporal、Control Plane、Web、Runtime Agent。
2. Chrome 登录 Web。
3. 创建主机运维、网络运维、故障分析团队。
4. 创建三个数字员工，provider 选择 `codex`。
5. 创建“系统健康分析项目”。
6. 将三个数字员工加入项目 executor pool。
7. 绑定项目 Runtime placement 到在线 `local-dev-node`。
8. 发起“深入分析当前操作系统整体健康运行状态”需求。
9. 验证 route decision、ProjectTask、attempt/run、Runtime task、provider execution 和 result/artifact。
10. 若真实 provider 缺凭据或外部服务不可用，必须报告准确阻塞依赖，不能声明端到端完成。

## 11. 实施切分建议

实现计划应拆成五个阶段：

1. 后端 placement API 和 readiness 读模型。
2. 规划资格与派发就绪拆分。
3. dispatch blocked facts、workflow failure projection、task graph blocking node。
4. 前端项目 placement 配置、执行准备度、workflow/task graph 诊断。
5. 真实 E2E smoke 和回归测试。

## 12. 风险

- 如果只改 Planner 过滤，会掩盖真实 dispatch 失败。
- 如果只补 placement UI，Runtime 短暂不可用时仍可能再次出现空 pool。
- 如果 blocked facts 没有统一 reason code，前端会继续依赖 raw error。
- 如果真实 smoke 没有 provider 执行，只能证明配置链路，不能证明项目任务执行闭环。

## 13. 验收标准

- 项目能显式绑定、查看和更换 Runtime placement。
- 缺 placement 时，用户能在项目详情和 workflow 页面看到明确阻塞原因。
- Planner 不再因 Runtime 暂不可派发而收到空数字员工池。
- Dispatcher 仍严格阻止不可真实执行的 ProjectTask。
- Task graph 能显示阻塞节点，不再空白。
- “系统健康分析项目”真实 smoke 能从 Web 发起需求并走到 Runtime/provider 执行；若环境阻塞，阻塞项必须可见且可解释。
