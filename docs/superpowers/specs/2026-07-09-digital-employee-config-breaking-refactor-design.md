# 数字员工配置破坏性重构设计

日期：2026-07-09
> 复核状态：基于CHANGELOG证据
状态：已确认，待实施计划

## 1. 背景

当前数字员工配置模型把身份画像、个人宪法补充、能力选择、上下文覆盖、审批覆盖、输出契约补充和预算策略混在同一个配置修订表里。这个模型带来三个问题。

第一，`role_profile` 和页面上的“角色配置”容易让用户以为数字员工需要一套复杂角色治理模型，但产品真正需要的是一个稳定的人格画像和少量基础身份字段。

第二，`constitution_addendum` 会制造“数字员工也有 AGENTS.md”的错觉。项目已经有自己的 `AGENTS.md` 或项目宪法，数字员工再拥有个人宪法会与项目规则产生冲突。数字员工只需要人格记忆，不应该持有与项目规则同级的宪法。

第三，`capability_selection`、`context_policy_override`、`approval_policy_override` 和 `output_contract_addendum` 把能力、治理和模型行为约束混在一起。治理不能靠模型输出的不确定性来落实，必须由 Control Plane、Runtime、上下文注入、审批工作流和预算账本硬控制。

本次重构选择破坏性方案：不兼容旧字段，不保留旧数字员工数据，不做旧字段到新字段的内容迁移。当前环境是开发库，若已有数字员工与新模型冲突，直接删除相关数据，以最终一致性为目标。

## 2. 目标

- 从产品、接口、数据库和运行链路中删除旧的数字员工个人治理配置模型。
- 数字员工不再拥有个人 `AGENTS.md`、个人宪法或治理覆盖。
- 用 `persona_memory_markdown` 表达稳定的人格画像，即“人格记忆.md”。
- 用 `capability_bindings` 表达 Skill、MCP、外部能力和环境变量引用的绑定关系。
- 保留 `budget_policy` 作为硬预算策略。
- Runtime 匹配由项目配置决定；数字员工配置不再包含 Runtime 匹配策略。
- agent home、缓存版本、配置版本和同步状态只作为运行状态展示，不作为用户配置。

## 3. 非目标

- 不兼容旧客户端继续写入 `role_profile`、`constitution_addendum`、`capability_selection`、`context_policy_override`、`approval_policy_override` 或 `output_contract_addendum`。
- 不把旧 `role_profile` 或 `constitution_addendum` 合并生成新 `persona_memory_markdown`。
- 不设计长期记忆库策略。长期记忆后续如果需要，应作为远程记忆库检索能力单独建模。
- 不把审批、上下文访问、输出契约或风险控制放入数字员工人格配置。
- 不改项目 Runtime 选择规则。项目绑定 Runtime 池或运行策略仍属于项目配置。
- 不在本阶段支持正式生产数据的保守迁移方案。

## 4. 当前代码事实

- Control Plane 的数字员工配置修订模型当前包含 `RoleProfile`、`ConstitutionAddendum`、`CapabilitySelection`、`ContextPolicyOverride`、`ApprovalPolicyOverride`、`BudgetPolicy` 和 `OutputContractAddendum`。
- `initialRoleProfile` 当前会把 `employee_type` 和 `role` 写入 `role_profile`。
- `initialCapabilitySelection` 当前会把员工类型默认能力与请求里的 `capability_selection` 合并。
- OpenAPI 和生成代码仍暴露 `role_profile`、`constitution_addendum`、`capability_selection` 等旧字段。
- Web 创建页仍展示模板默认角色、模板能力、风险等级和默认注入等信息，并会提交旧配置字段。
- Runtime 侧已经支持 agent home、workspace files、skills、MCP servers 和 environment variables 的物化或投影，这些应继续保留，但它们不再由旧 `capability_selection` 混合字段驱动。
- `provider_type` 已经是当前创建数字员工流程的必填事实，本设计不调整该边界。

## 5. 最终产品模型

数字员工由以下信息组成：

- 基础身份：名称、头像、团队归属、职责定位、简介。
- Provider：沿用当前 `provider_type` 创建事实。
- 人格记忆.md：`persona_memory_markdown`，稳定描述数字员工的人格画像、专业边界、工作方式和表达偏好。
- 能力绑定：`capability_bindings`，描述可用 Skill、MCP HTTP 能力、外部能力和环境变量引用。
- 预算策略：`budget_policy`，描述 token、金额、次数或周期上限等硬预算。
- 运行状态：Runtime、agent home、配置版本、缓存版本、同步状态和最近错误，只读展示。

数字员工不再包含：

- 角色配置 map。
- 个人宪法。
- 能力与策略混合配置。
- 上下文策略覆盖。
- 审批策略覆盖。
- 输出契约补充。
- Runtime 匹配配置。
- 记忆库策略。

## 6. 数据模型

`digital_employee_config_revisions` 应收敛为个人配置版本表，但只保存人格、能力和预算。

目标字段：

- `id`
- `tenant_id`
- `digital_employee_id`
- `revision_number`
- `persona_memory_markdown TEXT NOT NULL DEFAULT ''`
- `capability_bindings JSONB NOT NULL DEFAULT '{}'::jsonb`
- `budget_policy JSONB NOT NULL DEFAULT '{}'::jsonb`
- `status`
- `approved_by`
- `approved_at`
- `archived_at`
- `created_at`
- `updated_at`

删除字段：

- `role_profile`
- `constitution_addendum`
- `capability_selection`
- `context_policy_override`
- `approval_policy_override`
- `output_contract_addendum`

`digital_employees.provider_type` 或等价执行实例字段是既有 Provider 事实。本次迁移不改变该字段和现有创建校验。

## 7. 破坏性数据清理

由于当前是开发数据库，本次迁移不保护旧数字员工数据。

迁移原则：

- 删除与旧数字员工配置模型冲突的数字员工及其相关运行、配置、执行实例、能力绑定、环境变量和工作目录同步记录。
- 如果外键阻挡，按依赖顺序先删除运行与子表，再删除执行实例、配置修订和数字员工主记录。
- 不尝试把旧字段内容写入新字段。
- 迁移后系统只保留符合新模型的数据。

实施计划阶段需要基于真实表结构列出精确删除顺序，并用迁移校验证明迁移可重放。

## 8. 接口契约

OpenAPI 需要删除旧字段并新增最终字段。

创建数字员工请求应保留：

- `name`
- `avatar_asset_id`
- `team_id`
- `employee_type`
- `role` 或职责定位字段
- `description`
- `provider_type`
- `persona_memory_markdown`
- `capability_bindings`
- `budget_policy`
- 初始环境变量引用或初始环境变量写入结构

创建数字员工配置修订请求应只接受：

- `persona_memory_markdown`
- `capability_bindings`
- `budget_policy`
- `status`

数字员工配置修订响应应只返回：

- `persona_memory_markdown`
- `capability_bindings`
- `budget_policy`
- 版本和审批状态元数据

接口层不再接受旧字段。旧字段出现在请求体时必须拒绝并返回明确校验错误，避免误以为系统仍支持旧模型。

## 9. Web 信息架构

创建数字员工流程调整为：

1. **基础身份**：名称、头像、团队、职责定位、简介。
2. **Provider**：沿用当前 `provider_type` 选择步骤。
3. **人格记忆.md**：编辑 `persona_memory_markdown`。
4. **能力绑定**：选择 Skill、MCP HTTP 能力、外部能力，并配置或引用所需环境变量。
5. **预算策略**：配置 token、金额、次数或周期上限。
6. **确认创建**：展示最终摘要。

需要从 UI 删除以下文案和面板：

- 角色配置。
- 能力与策略。
- 个人宪法。
- 治理覆盖。
- 上下文策略覆盖。
- 审批策略覆盖。
- 输出契约补充。
- Runtime 匹配配置。

模板可以继续存在，但模板只允许预填：

- 基础身份建议。
- `persona_memory_markdown` 初始内容。
- `capability_bindings` 初始选择。
- `budget_policy` 默认值。

模板不再表达治理策略或 Runtime 选择。

## 10. 人格记忆.md

`persona_memory_markdown` 是一份稳定身份说明，不是项目规则。

推荐模板：

```md
# 人格画像

# 专业边界

# 工作方式

# 表达偏好

# 协作习惯

# 不应做的事
```

运行时注入顺序：

1. 项目 `AGENTS.md` 或项目宪法。
2. 当前任务指令。
3. 数字员工 `人格记忆.md`。
4. 远程拉取或本地缓存的能力配置。
5. 预算、权限和执行约束的硬校验结果。

如果项目规则和人格记忆冲突，项目规则优先。人格记忆不得覆盖项目规则、权限、审批、预算或 Runtime 限制。

## 11. 能力绑定

`capability_bindings` 是结构化配置，不是自然语言策略。

建议第一版结构：

```json
{
  "skills": [],
  "mcp_servers": [],
  "external_capabilities": [],
  "environment_variable_refs": []
}
```

能力绑定只表达“可用什么”和“需要什么凭据引用”。真正执行前必须由 Control Plane 和 Runtime 校验：

- 员工是否绑定该能力。
- 项目是否允许该能力。
- Runtime 是否具备该能力。
- 环境变量是否存在且授权可用。
- 当前任务是否允许调用该能力。

MCP HTTP 能力仍按既有方向由 Control Plane 管理注册和绑定，Runtime 只负责投影 provider 配置，不托管 MCP 服务。

## 12. Runtime 与执行包

Runtime 不再从旧 `capability_selection` 读取能力信息。

Control Plane 下发 Runtime payload 时应包含：

- `provider_type`
- `persona_memory_markdown`
- `capability_bindings`
- `budget_policy`
- skills 投影结果
- MCP servers 投影结果
- environment variables 投影结果
- workspace files 投影结果

Runtime 可以把 `人格记忆.md` 物化到员工 home 下，便于 provider 或人工排查，但物化文件是受控投影结果，不是项目 `AGENTS.md`，也不是配置事实源。

agent home 缓存版本、MCP 配置版本、Skill 安装版本和文件同步版本只作为状态和调试信息展示。

## 13. 治理边界

治理从数字员工配置中移除，改由平台硬控制。

- 能力调用：Control Plane 授权和 Runtime 执行前校验。
- 上下文访问：上下文注入服务过滤。
- 审批：工作流审批节点控制。
- 预算：预算账本和执行前拦截。
- 文件和命令执行：Runtime 权限和执行槽位控制。
- 输出质量：结构化验收、事后校验和人类验收。

模型人格可以影响表达和工作习惯，但不能承担安全、审批、预算或权限约束。

## 14. 错误处理

- `persona_memory_markdown` 为空时允许创建，但 UI 应提示该员工缺少人格记忆。
- `capability_bindings` 引用不存在的 Skill、MCP 或外部能力时，后端拒绝。
- 环境变量引用缺失时，能力可以保存为未就绪状态，但执行前必须阻断对应能力调用。
- Runtime 缓存版本不匹配时，Runtime 按现有拉取和投影机制刷新；这不是用户配置错误。
- 旧字段出现在新接口请求体时，必须返回明确错误，避免误以为系统仍支持旧模型。

## 15. 测试范围

后端测试：

- 迁移后旧配置字段不存在，新字段存在。
- 旧数字员工相关数据在开发迁移中被清理。
- 创建数字员工保存 `persona_memory_markdown`、`capability_bindings` 和 `budget_policy`。
- 配置修订只接受新字段。
- 旧字段请求被拒绝。
- Runtime payload 包含新字段，不再依赖旧 `capability_selection`。

OpenAPI 和生成测试：

- schema 删除旧字段。
- 生成的 Go 和 TypeScript 类型不再暴露旧配置字段。
- 契约验证通过。

前端测试：

- 创建流程不再出现“角色配置”“能力与策略”“个人宪法”“治理覆盖”等旧文案。
- 人格记忆.md 编辑区能提交 `persona_memory_markdown`。
- 能力绑定提交 `capability_bindings`。
- 预算策略继续提交 `budget_policy`。
- 模板只预填人格、能力绑定和预算，不再预填治理字段。
- 详情页展示人格记忆、能力绑定、预算和运行状态。

Runtime 测试：

- provision payload 能解析 `persona_memory_markdown` 和 `capability_bindings`。
- provider 启动上下文包含人格记忆。
- MCP 和环境变量投影继续可用。
- 缓存版本只影响刷新和状态展示，不作为用户配置解析。

## 16. 真实验证

实现完成后需要走真实链路：

1. 在开发库执行破坏性迁移，确认旧数字员工和旧配置字段被清理。
2. 启动 Control Plane、Web、Runtime Agent。
3. 通过 Web 创建一个新数字员工，按现有流程选择 Provider，填写人格记忆，绑定至少一个能力，配置预算。
4. 打开数字员工详情，确认只展示新模型字段和运行状态。
5. 发起一次真实任务，确认 Runtime 收到 `persona_memory_markdown`、`capability_bindings`、MCP、环境变量和预算数据。
6. 确认项目 `AGENTS.md` 仍是项目规则入口，数字员工没有个人 `AGENTS.md`。
7. 确认旧字段无法通过 API 写入。

收尾前必须使用项目内 `superteam-completion-check` skill。不能把 mock、组件测试、单元测试或构建通过表述为真实链路已验证。
