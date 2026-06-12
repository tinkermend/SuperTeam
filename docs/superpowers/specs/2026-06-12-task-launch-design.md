# 2026-06-12-task-launch-design

## 1. 阶段目标

本设计定义 SuperTeam 第一版“任务发起”入口。

第一版目标是让人类用户用一个全局入口提交需求，选择所属项目和审核人偏好，然后进入一次发起记录详情页，追踪该需求触发的真实项目协调事实。

“任务发起”不是 Runtime 任务创建页，也不是直接创建数字员工执行任务。它提交的是项目需求事实，后续由项目虚拟协调 Workflow 决定是否生成 RouteDecision、ProjectTask、DecisionRequest、转派请求和执行状态。

## 2. 产品定位

### 2.1 核心心智

用户侧心智：

- 我有一个需求。
- 我选择这个需求属于哪个项目。
- 我可以指定或接受默认审核人。
- 提交后由项目协调线程编排。
- 我在发起详情页追踪协调结果和需要人工确认的事项。

系统侧心智：

- `ProjectDemand` 是一次任务发起的事实源。
- `Project` 仍是业务闭环容器。
- `ProjectTask` 是协调者编排出来的项目内工作项。
- Runtime `Task` 是底层执行事实，不作为用户一级业务入口。

### 2.2 导航调整

左侧一级导航调整为：

- 工作台
- 任务发起
- 项目管理
- 数字员工
- 技能管理
- 团队管理

移除当前“任务中心”一级菜单。现有 `/api/v1/tasks`、Runtime task 和相关底层能力不删除，后续可放入 Runtime 节点下的执行队列或运行任务视图。

移除“任务中心”的原因：

- 当前 `/tasks` 只是占位页，业务语义不清。
- 项目内任务已经在项目管理详情和项目配置页展示。
- 继续保留“任务中心”会和任务发起、项目管理、项目任务产生心智重叠。

## 3. 范围边界

### 3.1 第一版包含

后端：

- 扩展提交项目需求的契约，支持审核人偏好。
- 校验审核人必须是当前项目的人类成员。
- 按项目成员角色解析默认审核人。
- 提供按 demand 聚合的发起详情读接口。
- 保持提交需求后 signal 项目协调 Workflow 的现有链路。

前端：

- 新增“任务发起”一级菜单。
- 移除“任务中心”一级菜单。
- 新增任务发起页。
- 新增任务发起详情页。
- 任务发起页支持项目选择、需求描述、标题、高级选项和审核人选择。
- 发起详情页展示 demand、项目、审核人偏好、协调 Job、路由决策、项目任务、人类决策请求和最近事件。

### 3.2 第一版不包含

第一版不做：

- xyflow 流程图。
- 复杂 AI 编排可视化卡片。
- 独立业务任务表。
- Runtime task 的用户侧创建体验。
- 跨项目任务中心。
- 多级审批策略编辑器。
- 数字员工自动聊天或自由协作界面。

## 4. 后端设计

### 4.1 事实模型

继续使用 `ProjectDemand` 表达一次任务发起记录。

提交链路：

```text
人类用户提交任务发起
-> Control Plane 校验项目和审核人
-> 写入 ProjectDemand
-> 写入 demand.submitted ProjectEvent
-> Signal 项目协调 Workflow: DemandSubmitted
-> 协调 Workflow 规划 RouteDecision 和 ProjectTask
-> 必要时创建 DecisionRequest 并等待人类审核
```

### 4.2 提交需求契约

扩展 `SubmitProjectDemandRequest`，新增可选字段：

```text
reviewer_user_id
reviewer_selection_reason
```

字段语义：

- `reviewer_user_id` 是本次需求的人工审核目标偏好。
- 它不是立即审批请求。
- 最终是否创建 `DecisionRequest` 由协调策略、风险判断和路由结果决定。
- 一旦协调者创建 `DecisionRequest`，Workflow 必须等待审核完成后再继续推进后续分发、暂停、补证或状态变更。

`reviewer_selection_reason` 用于记录默认选择来源或用户显式选择原因，例如：

```text
project_reviewer_default
project_human_owner_fallback
user_selected
```

### 4.3 默认审核人规则

默认规则：

1. 优先选择项目成员中 `principal_type = human_user` 且 `project_role = reviewer` 的成员。
2. 如果只有一个 reviewer，自动默认选中。
3. 如果有多个 reviewer，前端默认不强制自动选择，用户需要选择一个；后端仍可接受合法 reviewer。
4. 如果没有 reviewer，回退到 `human_owner_user_id`。
5. 审核人不能是数字员工、团队或项目外用户。

后端必须重新校验前端传入的审核人，不能只信任前端默认值。

### 4.4 聚合详情接口

新增读接口建议：

```text
GET /api/v1/project-demands/{demandId}/launch-detail
```

返回内容：

- demand 本体。
- 所属 project 简要信息。
- resolved reviewer preference。
- coordination jobs。
- route decisions。
- project tasks。
- decision requests。
- recent project events。

聚合接口按 demand 过滤协调事实，避免前端跨多个项目接口拼装业务含义。

### 4.5 Workflow 行为

提交需求后不会立即把任务推给审核人。

协调者在以下场景创建人类决策请求：

- 路由策略要求新需求先审核。
- AI 判断需求存在风险、歧义或权限边界问题。
- 预算、外部写入、上线发布、删除写入或测试失败后的业务判断需要人类确认。
- 数字员工执行中请求转派、补证或暂停。

创建 `DecisionRequest` 后，Workflow 对该 demand 相关后续动作进入等待状态。审核结果到达后：

- 同意：继续分发或继续执行。
- 驳回：记录决策并停止或取消后续任务。
- 补证：生成补证请求或等待用户补充。
- 暂停：项目或需求进入等待人工处理状态。

## 5. 前端设计

### 5.1 任务发起页

路由建议：

```text
/task-launches
```

页面结构：

- 页面标题：任务发起。
- 主输入区：多行需求描述。
- 标题输入：可从描述首行生成，允许手动编辑。
- 项目选择：必选，显示项目名、项目状态、人类负责人。
- 高级选项：默认折叠。
- 提交按钮：文案使用“发起任务”。

项目选择规则：

- 只展示用户可见项目。
- 归档项目不可选。
- 项目切换后重新解析默认审核人。
- 已输入需求描述不因项目切换而清空。

高级选项包含：

- 审核人选择。
- 来源类型。
- 来源引用。
- 附件引用。

审核人选择展示：

- 候选仅包含人类项目成员。
- reviewer 角色优先排在前面。
- human_owner fallback 需要显示来源提示。
- 数字员工不进入候选列表。

### 5.2 发起详情页

路由建议：

```text
/task-launches/$demandId
```

页面布局：

- 左侧或顶部摘要区：需求标题、需求内容、所属项目、提交人、审核人偏好、创建时间、当前状态。
- 主内容区：协调事实。
- 项目跳转按钮：进入所属项目详情。

协调事实区域展示：

- 协调 Job：状态、类型、开始时间、结束时间。
- 路由决策：候选数字员工、选中数字员工、原因、是否需要人类审核。
- 项目任务：标题、状态、分派数字员工、风险等级、是否需要人工审批。
- 人类决策请求：目标审核人、决策类型、状态、风险等级。
- 最近事件：按序号展示与该 demand 相关的项目事件。

无协调数据时显示：

```text
等待项目协调线程处理
```

不要用占位节点伪装流程图。

### 5.3 刷新体验

遵循现有前端规则：

- queryKey 变化时默认保留旧数据。
- 后台刷新不卸载主内容。
- 项目切换不清空本地表单输入。
- 详情刷新失败时保留旧数据，并在局部区域显示重试。

## 6. 错误处理

前端即时校验：

- 需求描述不能为空。
- 标题不能为空。
- 项目不能为空。
- 多 reviewer 且无默认选中时，提交前提示选择审核人。

后端校验：

- 项目必须存在且未归档。
- 提交人必须是人类用户。
- 审核人必须是当前项目人类成员。
- 数字员工、团队、项目外用户不能作为审核人。
- source refs 必须是 JSON object。

错误状态：

- 项目归档：提示项目已归档，不能发起新任务。
- 审核人无效：提示审核人必须是当前项目的人类成员。
- Workflow signal 失败：需求和事件已落库时显示已记录但协调信号失败，允许进入详情页查看恢复状态，避免重复提交。
- 聚合详情加载失败：保留页面框架并提供重试。

## 7. 权限与审计

权限原则：

- 只有具备项目可见性和需求提交权限的人类用户可以发起任务。
- 审核人只来自项目人类成员。
- 协调者不能绕过人工决策。

审计原则：

- 任务发起必须产生 `demand.submitted` 事件。
- 审核人偏好需要进入 demand 或相关 metadata，并能在发起详情页看到。
- DecisionRequest 的创建、解决和后续状态变化必须进入项目事件流。

## 8. 测试计划

Control Plane：

- 提交需求时传入合法 reviewer。
- 提交需求时 reviewer 是数字员工，返回校验错误。
- 提交需求时 reviewer 是项目外人类用户，返回校验错误。
- 项目只有一个 reviewer 时默认解析 reviewer。
- 项目无 reviewer 时回退 human_owner。
- 聚合详情接口按 demand 返回 coordination jobs、route decisions、project tasks、decision requests 和 recent events。
- Workflow signal 失败时不重复创建 demand，保留可恢复事件。

Contracts：

- 修改 OpenAPI 后运行 `pnpm generate:control-plane`。
- 运行 `pnpm verify:contracts`。
- 不手工编辑生成的 Go 文件。

Web：

- 侧栏包含“任务发起”，不包含“任务中心”。
- 任务发起页表单校验。
- 项目选择后解析审核人默认值。
- 多 reviewer 时要求用户选择。
- 无 reviewer 时回退负责人。
- 提交成功后跳转发起详情页。
- 发起详情页渲染 demand、项目、审核人偏好和协调事实。
- 后台刷新保留旧内容。

浏览器验证：

- 真实浏览器访问任务发起页。
- 验证桌面视口下导航、表单、详情页无重叠。
- 移动端至少检查表单字段和按钮不溢出。

## 9. 后续 V2 预留

V2 在发起详情页右侧增加编排流流程图，优先使用 `xyflow`。

流程图数据来自第一版聚合详情接口，不另建虚假前端状态：

- Demand 节点。
- Coordinator 节点。
- RouteDecision 节点。
- ProjectTask 节点。
- DecisionRequest 节点。
- ExecutionSummary 节点。
- TransferRequest 节点。

每个节点卡片展示真实状态、负责人、输入输出、等待原因和最近更新时间。V2 不改变第一版的事实源，只升级可视化方式。
