# 数字员工空白自定义创建设计

日期：2026-07-06
状态：已确认，待实现计划

## 1. 背景

当前数字员工创建页已经收敛为“选择创建路径与模板 -> 配置预检 -> 身份/能力/治理/执行器配置 -> 确认创建”。上一轮产品方向将 `空白自定义` 保留为禁用状态，避免出现没有真实数据流的半成品入口。

本轮要补齐的是 `空白自定义` 的真实配置能力。用户已经确认采用“复用现有员工类型与团队治理校验，不新增任意自定义类型”的方案：空白自定义不是绕过 `employee_type` 的新模型，而是一个不套用专业模板默认能力和治理覆盖的创建路径。

## 2. 目标

- 开放 `空白自定义` 创建路径。
- 空白路径仍必须选择一个后端已有且团队允许的 `employee_type`。
- 空白路径不注入模板推荐技能、MCP、外部能力、上下文覆盖或审批覆盖。
- 空白路径复用现有四步配置向导，让用户手动补齐身份、能力、治理和执行器。
- 创建提交继续使用现有 `POST /api/v1/digital-employees` 契约。
- 后端继续作为最终校验层，校验员工类型、团队 allowlist、能力越权、Provider 和初始有效配置。

## 3. 非目标

- 不新增任意自定义员工类型。
- 不修改 `CreateDigitalEmployeeInput` 或 OpenAPI 创建契约。
- 不新增数据库迁移。
- 不新增后端创建分支。
- 不实现“从团队角色复制”或“从历史员工克隆”。
- 不在创建页暴露原始 JSON 高级编辑器。
- 不改变 Runtime Agent、Provider adapter 或项目任务调度流程。

## 4. 产品口径

数字员工创建有两个可用入口：

- `从专业模板创建`：选择专业模板，模板负责初始化角色、能力和治理默认值。
- `空白自定义`：选择底层员工类型，类型只作为后端治理校验和角色画像基础；能力和治理覆盖由用户在向导中手动配置。

`空白自定义` 里的“空白”只表示不套用专业模板草稿，不表示跳过员工类型、团队治理或后端校验。

## 5. 状态模型

前端创建页新增草稿状态 `creation_mode`：

```ts
type CreationMode = "template" | "blank_custom";
```

`template` 路径沿用现有 `applyTypeDefaults` 行为：

- 写入 `employee_type`
- 写入默认 `role`
- 写入模板默认能力选择
- 写入模板默认上下文覆盖
- 写入模板默认审批覆盖
- 写入风险默认值

`blank_custom` 路径新增 `applyBlankTypeDefaults` 行为：

- 写入 `employee_type`
- 可用该类型的 `default_role` 初始化 `role`
- 写入基础风险默认值
- 将 `capability_selection.enabled_skills` 置为空数组
- 将 `capability_selection.enabled_mcp_servers` 置为空数组
- 将 `capability_selection.enabled_external_capabilities` 置为空数组
- 将 `context_policy_override` 置为 `{}`
- 将 `approval_policy_override` 置为 `{}`

切换创建路径或切换底层类型会重置当前草稿。若用户已经进入配置并编辑过字段，切换前需要确认会丢弃当前草稿。

## 6. 选择阶段交互

左侧 `创建路径` 面板保留四个入口：

- `从专业模板创建`：可用。
- `空白自定义`：可用。
- `从团队角色复制`：禁用，显示 `暂未开放`。
- `从历史员工克隆`：禁用，显示 `暂未开放`。

右侧内容按 `creation_mode` 切换。

### 6.1 专业模板模式

继续展示现有模板选择表：

- 模板名称和描述
- 默认角色
- 模板能力数量
- 风险等级
- 默认注入摘要

选中模板后进入配置预检。

### 6.2 空白自定义模式

展示“选择员工类型”表，数据来源仍是 `GET /api/v1/digital-employees/create-options` 的 `employee_types`。

表格展示：

- 员工类型名称和说明
- `employee_type` 标识
- 默认角色
- 风险建议
- 选择操作

页面说明必须明确：员工类型用于后端治理校验；空白自定义不会自动注入模板推荐技能、MCP、外部能力、上下文覆盖或审批覆盖。

选中员工类型后进入配置预检。

## 7. 配置阶段交互

配置阶段继续复用现有四步向导：

- 身份
- 能力
- 治理
- 执行器

配置页顶部摘要按路径变化：

- 模板路径显示 `已选模板`，展示模板名称、说明、默认角色、技能数量和 MCP 数量。
- 空白路径显示 `空白自定义草稿`，展示底层员工类型、说明、默认角色和“能力手动配置”的提示。

摘要区提供 `更换创建路径` 操作。若草稿已编辑，点击时提示切换会重置当前草稿。

能力步骤在空白路径下默认没有任何选中项，用户必须显式勾选技能、MCP Server 和外部能力。候选列表仍由团队治理和 `create-options.capability_options` 决定。

治理步骤保持现有受控表单，先只支持每日 Token 预算上限和已有摘要，不引入 JSON 编辑器。

执行器步骤沿用现有 Provider 选择和环境变量配置；Provider 仍来自真实可用候选和团队策略，不由模板或空白路径直接决定。

## 8. 数据提交

创建提交继续调用：

```http
POST /api/v1/digital-employees
```

提交体仍使用现有 `CreateDigitalEmployeeInput`。

模板路径提交：

- `employee_type` 来自模板选择
- `role`、`capability_selection`、`context_policy_override`、`approval_policy_override` 来自模板初始化后的当前草稿
- `provider_type`、预算、环境变量来自用户配置

空白路径提交：

- `employee_type` 来自员工类型选择
- `role` 来自用户当前填写值
- `capability_selection` 来自用户手动选择
- `context_policy_override` 默认为 `{}`
- `approval_policy_override` 默认为 `{}`
- `budget_policy` 来自治理步骤
- `provider_type` 来自执行器步骤
- `environment_variables` 仅提交 name/value 都存在的行

为了后续审计和详情展示，前端可以在 `role_profile` 或 `metadata` 写入 `creation_mode: "blank_custom"`。该字段只表达创建来源，不作为后端业务分支条件。

## 9. 校验与错误处理

前端校验：

- 必须选择创建路径。
- 必须选择模板或底层员工类型。
- 必须选择头像。
- 必须填写名称。
- 必须填写角色。
- 必须选择 Provider。
- 每日 Token 预算上限如填写，必须是正整数。

创建选项加载失败时，展示现有错误提示并阻止继续。

如果团队治理变更导致当前员工类型不再可用，前端重置草稿并回到对应选择表。

配置预检继续使用 `creation_checks`。若存在 `blocked` 检查项，不允许进入确认创建。

后端校验不新增分支。`CreateDigitalEmployee` 继续负责：

- `employee_type` 存在且受支持
- 团队允许该员工类型
- Provider 类型受支持且被团队允许
- 能力选择不越过团队 allowlist
- 初始有效配置没有阻断错误

## 10. 视觉与组件约束

实现必须延续当前 SuperTeam v3 Soft-Flat 风格：

- 页面继续使用现有 `ShellPageHeader`、`Main`、`IconTile`、按钮、表单和卡片模式。
- 不引入新的视觉语言或营销式布局。
- 创建路径面板保持工作台式密度。
- 员工类型选择表与模板表保持相同信息密度和操作方式。
- 所有用户可见文案默认使用简体中文。

## 11. 实现范围

主要修改：

- `apps/web/src/features/employees/create.tsx`
- `apps/web/src/features/employees/create.test.tsx`

可能修改：

- `apps/web/src/lib/api/employees.ts`，仅在类型需要表达 `creation_mode` 元数据或测试需要补齐类型时调整。

不修改：

- `contracts/control-plane/openapi.yaml`
- `apps/control-plane/**`
- `apps/runtime-agent/**`
- 数据库迁移

## 12. 验收标准

- `空白自定义` 在创建路径中可点击，并与 `从专业模板创建` 明确区分。
- 空白路径必须先选择一个允许的员工类型。
- 空白路径进入配置后，能力选择默认全空。
- 空白路径不会注入模板推荐技能、MCP、外部能力、上下文覆盖或审批覆盖。
- 空白路径和模板路径都复用同一个四步配置向导。
- 确认创建页能区分显示 `专业模板` 或 `空白自定义草稿`。
- 创建请求不新增必需字段，仍能被现有后端创建接口处理。
- 团队策略、Provider 和能力越权仍由后端最终校验。

## 13. 测试计划

前端定向测试：

- 创建路径中 `空白自定义` 可用，复制和克隆路径仍禁用。
- 选择空白路径后展示员工类型表，而不是模板表。
- 空白路径选择员工类型后进入预检。
- 空白路径进入配置后能力选择默认全空。
- 模板路径仍按模板默认值初始化能力和治理覆盖。
- 切换路径或类型时，在已编辑草稿情况下出现确认提示。
- 空白路径提交体包含 `employee_type`、用户填写角色、空白或手动选择的能力、Provider、预算和环境变量。
- 确认创建页显示空白自定义来源。

命令：

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx
git diff --check
```

真实链路验证：

- 启动或确认当前 Web 与 Control Plane 加载的是最新代码。
- 使用真实登录会话访问 `/employees/new`。
- 选择 `空白自定义`，选择一个真实返回的员工类型。
- 走完身份、能力、治理和执行器步骤。
- 若 Runtime Provider 可用，完成一次真实创建并进入员工详情页。
- 若 Runtime Provider 不可用，必须看到真实 `creation_checks` 或 Provider 空状态给出的可解释阻断，不把阻断状态误判为功能可用。

## 14. 后续计划边界

本设计确认后，下一步进入实现计划。实现计划应保持本轮范围：只实现空白自定义作为第二条创建路径，不引入任意自定义类型、后端契约扩展、JSON 高级编辑器、复制路径或克隆路径。
