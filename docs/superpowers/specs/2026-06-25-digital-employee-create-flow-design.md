# 数字员工创建流程收敛设计

日期：2026-06-25
状态：已确认，待实现计划

## 1. 背景

当前数字员工创建页已经具备两段式结构：先选择创建路径和专业模板，再进入身份、能力、治理、运行四步配置。后端也已经通过 `GET /api/v1/digital-employees/create-options?team_id=...` 返回团队治理、模板、能力候选、Runtime Provider 候选和创建预检。

本轮要解决的不是新增完整创建能力，而是收敛已有创建体验中的产品歧义：

- 模板卡展示 Provider 信息，容易让用户误以为模板可以定义或锁定 Provider。
- `空白自定义` 和 `从空白开始自定义` 看起来可用，但当前没有真实分支逻辑。
- 进入配置页后仍展示完整模板列表，用户已经选定模板后还会看到第二个模板市场。
- Runtime 绑定列表展示大量禁用项，虽然数据来自真实后端预检，但体验上容易被理解为 mock 或占位数据。

## 2. 目标

- 模板只负责初始化角色、能力和治理默认值，不负责 Provider 决策。
- 进入配置页后不再显示完整模板列表，只显示当前已选模板摘要和更换入口。
- `空白自定义` 保留为禁用状态并标明暂未开放，不产生半成品交互。
- Runtime 运行步骤只显示真实可绑定的 Runtime Provider 候选。
- 不修改后端契约，不新增 mock 数据，不在前端伪造可绑定状态。

## 3. 非目标

- 不实现真正的空白自定义创建。
- 不新增模板自定义编辑器。
- 不修改 `create-options` 响应结构。
- 不改 Runtime Agent、Provider adapter、数据库迁移或 OpenAPI 契约。
- 不实现自动 Runtime fallback、跨节点迁移或创建后自动执行任务。

## 4. 核心口径

数字员工创建流程分成两个明确阶段。

第一阶段是起步选择：

- 用户选择可用创建路径。首版只有 `从专业模板创建` 可用。
- 用户选择一个专业模板。
- 页面展示创建预检和即将创建摘要。

第二阶段是详细配置：

- 用户进入身份、能力、治理、运行四步向导。
- 模板只作为初始草稿来源，不再作为配置页主体内容。
- 用户最终提交的 `runtime_node_id` 和 `provider_type` 只来自运行步骤中真实可绑定的 Runtime Provider 候选。

Provider 是运行绑定事实，不是模板事实。模板可以继续在后端定义 `recommended_provider_types` 作为内部建议字段，但当前创建向导不把它作为主要用户决策展示。

## 5. 交互设计

### 5.1 选择阶段

创建路径面板保持四个入口的视觉结构，但状态需要明确：

- `从专业模板创建`：可用，当前选中。
- `从团队角色复制`：禁用，显示 `暂未开放`。
- `从历史员工克隆`：禁用，显示 `暂未开放`。
- `空白自定义`：禁用，显示 `暂未开放`。

模板选择面板仍显示可用专业模板卡。模板卡展示：

- 模板名称和描述。
- 默认角色。
- 推荐技能数量。
- 推荐 MCP 数量。
- 默认风险等级。

模板卡不再展示 Provider 指标。

### 5.2 配置阶段

进入配置后，主区域保留四步表单。原来的完整模板列表不再出现。

配置页顶部新增一个紧凑的已选模板摘要条：

- 显示当前模板名称。
- 显示模板说明的简短摘要。
- 显示默认角色或当前角色。
- 提供 `更换模板` 操作。

`更换模板` 返回选择阶段。若用户已经编辑过配置字段，点击更换时需要确认会重置当前草稿；取消则留在当前配置页。

已选模板摘要条只提供上下文，不承担再次选择多个模板的功能。

## 6. 字段与状态规则

模板可以初始化：

- `employee_type`
- `role`
- `risk_level`
- `capability_selection.enabled_skills`
- `capability_selection.enabled_mcp_servers`
- `capability_selection.enabled_external_capabilities`
- `context_policy_override`
- `approval_policy_override`

模板不能初始化或锁定：

- `runtime_node_id`
- `provider_type`
- `runtime_binding`

Runtime 绑定由运行步骤统一决定：

- 读取 `runtime_provider_options`。
- 只渲染 `available=true` 的候选。
- 一个 Runtime 支持多个可用 Provider 时，按 Runtime/Provider 组合显示多行候选。
- 可用候选只有一个时自动选中。
- 可用候选超过一个时必须用户显式单选。
- 可用候选为空时不能提交创建。

不可用 Runtime Provider 候选不在运行步骤展示。创建预检仍可以保留后端返回的 `0/N 个运行绑定可用`，用于解释当前团队为什么无法继续创建。

## 7. 数据流

前端继续使用现有接口：

```http
GET /api/v1/digital-employees/create-options?team_id={teamId}
GET /api/v1/digital-employee-avatar-assets
POST /api/v1/digital-employees
```

创建提交时：

- `employee_type` 来自模板选择。
- 角色、能力和治理字段来自用户当前草稿。
- `runtime_node_id` 和 `provider_type` 来自运行步骤选中的可用 Runtime Provider 候选。
- 前端不把模板的 Provider 推荐字段写入提交体。

后端仍负责最终校验团队策略、Runtime 在线状态、Provider 健康状态和运行范围策略。

## 8. 错误处理

- 创建选项接口失败时，沿用现有错误提示并阻止进入配置。
- 团队没有可用模板时，选择阶段展示空状态并阻止进入配置。
- `空白自定义` 禁用态不改变草稿，不触发进入配置。
- 运行步骤没有可用 Runtime Provider 时，展示空状态：`当前团队没有可绑定的 Runtime Provider，请检查 Runtime 在线状态、Provider 健康状态或团队运行策略。`
- 创建按钮在缺少 Runtime 绑定时保持禁用。
- 创建提交失败时，沿用当前表单错误展示，不吞掉后端错误信息。

## 9. 实现范围

Web 侧主要调整：

- `apps/web/src/features/employees/create.tsx`
- `apps/web/src/features/employees/create.test.tsx`

预期改动：

- 调整创建路径按钮状态，禁用未实现路径。
- 删除模板卡中的 Provider 指标。
- 将配置页完整模板列表替换为已选模板摘要条。
- 增加更换模板确认逻辑。
- 运行步骤过滤 `available=true` 的 Runtime Provider 候选。
- 更新创建预检和运行步骤的空状态文案。
- 补充相关前端测试。

后端不需要改动，除非实现过程中发现 `create-options` 真实返回与现有类型定义不一致。

## 10. 验收标准

- 模板选择页不再暗示模板可以定义 Provider。
- `空白自定义` 明确不可用，点击不会进入配置或改变草稿。
- 进入配置页后不再展示完整模板列表。
- 配置页显示当前已选模板摘要和更换入口。
- Runtime 步骤只展示 `available=true` 的 Runtime Provider 候选。
- 没有可用 Runtime Provider 时，创建按钮禁用并展示明确空状态。
- 创建请求中的 `runtime_node_id` 和 `provider_type` 来自用户在运行步骤选中的真实候选。

## 11. 测试计划

前端测试：

- 模板卡不再显示 Provider 指标。
- 进入配置后不再出现完整模板列表，只出现已选模板摘要。
- `空白自定义` 为禁用态，不能进入配置。
- 运行步骤只渲染 `available=true` 的 Runtime Provider。
- 没有可用 Runtime Provider 时创建按钮禁用并展示空状态。
- 多个可用 Runtime Provider 时必须显式选择。
- 单个可用 Runtime Provider 时保持自动选中。

局部验证：

- `corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx`
- `git diff --check`

真实链路验证：

- 实现阶段如要声明创建流程可用，需要启动当前 Web 和 Control Plane，访问真实创建页，确认 `create-options` 返回真实 Runtime Provider 候选，并通过页面完成一次创建或走到真实后端返回的可解释阻断状态。

## 12. 后续计划边界

本设计确认后，下一步进入实现计划编写。实现计划应保持本轮范围，不把真正空白自定义、后端契约拆分或 Runtime fallback 纳入同一次实施。
