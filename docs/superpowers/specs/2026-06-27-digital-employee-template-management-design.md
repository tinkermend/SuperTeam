# 数字员工模板管理与创建页口径设计

日期：2026-06-27
> 复核状态：已实现（2026-06-28完成）
状态：已确认，待实现计划

## 1. 背景

当前数字员工创建页已经调整为“选择模板 -> 配置预检 -> 完成配置 -> 确认创建”的多步骤流程。模板选择区域改成表格后，能缓解模板数量增加时卡片列表持续下滚的问题，但仍存在几个产品口径问题：

- `模板` 列重复显示 `frontend_engineer` 等英文角色标识，而 `默认角色` 列已经承担这个信息。
- `推荐能力` 容易被理解为平台实时推荐，实际数据来自内置模板定义。
- `默认注入` 当前没有清楚表达真正会进入创建草稿的默认能力。
- 数字员工首页只有 `创建数字员工` 入口，没有进入模板目录、理解模板定义的入口。
- 当前模板来自后端代码内置的 `EmployeeTypeDefinition`，Skill、MCP、Provider 只是字符串 key，不是真正可编辑、可审计的模板资产。

本轮目标是先把信息架构和展示口径摆正，新增只读模板目录，为后续模板资源化预留结构；不在本轮实现模板 CRUD 或数据库模型。

## 2. 目标

- 创建页模板表格只展示当前接口真实具备的信息，不暗示动态推荐或不存在的编辑能力。
- 数字员工首页新增 `模板管理` 入口，进入只读模板目录。
- 模板管理页和模板详情页以只读方式展示当前内置模板定义。
- 模板详情能解释模板定义能力、默认注入能力、团队治理过滤之间的关系。
- 从模板详情可以进入创建页，并预选对应模板。
- UI 命名和路由按未来一等模板资源预留，后续不需要推倒重来。

## 3. 非目标

- 不新增数据库迁移。
- 不新增 `digital_employee_templates` 表。
- 不新增模板创建、编辑、删除、复制接口。
- 不把 Skill、MCP 或 Provider 字符串伪造成真实关联编辑。
- 不修改现有 `POST /api/v1/digital-employees` 创建语义。
- 不改变 Runtime/Provider 可用性展示边界；Runtime 可用性仍在预检和运行步骤展示，不放回模板列表。

## 4. 信息架构

数字员工首页顶部主操作区调整为两个入口：

- 主按钮：`创建数字员工`，进入 `/employees/new`。
- 次按钮：`模板管理`，进入 `/employees/templates`。

模板是创建数字员工的上游配置资产，应放在 `数字员工` 模块下，而不是藏在创建流程中，也不归入 Skill 管理或平台管理。

命名统一为：

- 创建流程步骤：`选择模板`。
- 表格对象：`内置模板`。
- 管理页面：`数字员工模板`。
- 不再把模板选择页标题写成 `专业类型`，避免用户以为这是普通枚举而不是可复用创建模板。

## 5. 创建页模板表格

创建页的模板表格使用脆数据面，继续服务逐行扫描和比较。表格列调整为：

```text
模板 | 默认角色 | 模板能力 | 默认注入 | 风险等级 | 选择
```

字段口径：

- `模板`：只显示图标、中文模板名和描述。不再显示 `type` 或英文角色标识。
- `默认角色`：显示 `default_role`。
- `模板能力`：显示模板定义中的 `recommended_skills`、`recommended_mcp_servers`、`recommended_provider_types` 数量和前几个 key。
- `默认注入`：显示 `default_capability_selection` 中实际会进入创建草稿的 enabled 数量，包括技能、MCP 和 Provider。
- `风险等级`：显示由模板默认审批策略推导的风险等级，例如 `medium`、`high`。不再叫 `风险触发`，因为当前没有展示触发条件。
- `选择`：保持当前单选行为。

文案调整：

- `推荐能力` 改为 `模板能力`。
- `模板只负责带出默认角色、推荐能力和治理默认值` 改为 `模板只负责带出默认角色、模板能力和治理默认值`。
- 搜索仍支持中文模板名、描述、默认角色、技能、MCP、Provider key。

## 6. 模板管理页

新增只读页面：

```text
/employees/templates
```

页面标题：`数字员工模板`。

列表使用表格而不是卡片，因为模板数量增加后需要按能力、风险、状态和适用范围扫描比较。建议列为：

```text
模板 | 默认角色 | 模板能力 | 默认注入 | 适用范围 | 状态 | 操作
```

字段口径：

- `模板`：中文名称、描述和 `内置模板` 标识。
- `默认角色`：`default_role`。
- `模板能力`：`recommended_skills`、`recommended_mcp_servers`、`recommended_provider_types`。
- `默认注入`：`default_capability_selection`。
- `适用范围`：本轮显示 `租户内置` 或 `当前团队可用`，后续扩展为团队、租户、全局。
- `状态`：显示 `可用` 或 `被治理过滤`。
- `操作`：`查看详情`。

本轮不放可点击的 `新建模板` 或 `编辑模板` 主按钮，避免承诺当前后端不存在的能力。页面说明可以明确当前为内置模板目录。

模板管理页数据仍来自现有 `GET /api/v1/digital-employees/create-options?team_id=...`，不新增假接口。

## 7. 模板详情页

新增只读页面：

```text
/employees/templates/$templateType
```

详情页分区：

- `基础信息`：名称、描述、默认角色、风险等级。
- `模板能力`：技能、MCP、Provider 三组 key。
- `默认注入`：创建员工时默认勾选的技能、MCP、Provider。
- `治理影响`：当前团队允许或过滤了哪些能力，解释为什么模板定义了能力，但创建草稿可能默认注入为 0。
- `创建入口`：按钮 `用此模板创建数字员工`。

创建入口跳转到：

```text
/employees/new?template={templateType}
```

创建页读取 `template` query 后，如果该模板存在于当前团队的 `employee_types` 中，则预选该模板；如果不存在，保留默认选择并展示可解释提示。

## 8. 数据流

本轮继续使用现有接口：

```http
GET /api/v1/digital-employees/create-options?team_id={teamId}
GET /api/v1/teams
GET /api/v1/digital-employee-avatar-assets
POST /api/v1/digital-employees
```

模板列表和详情从 `create-options.employee_types` 派生。创建页预选模板只影响前端草稿初始化，不改变后端创建契约。

团队治理影响来自：

- `team_config.allowed_provider_types`
- `team_config.allowed_skills`
- `team_config.allowed_mcp_servers`
- `team_config.allowed_external_capabilities`
- `capability_options`
- `employee_types[*].default_capability_selection`

如果当前接口不足以精确说明某个能力为何被过滤，本轮只展示“受当前团队治理影响”的可解释提示，不伪造缺失原因。

## 9. 后续模板资源化边界

后续真正让模板可创建、可编辑时，不应继续扩展代码内置数组。应新增一等模型 `digital_employee_templates`，核心字段包括：

- `id`
- `tenant_id`
- `team_id`
- `key`
- `name`
- `description`
- `default_role`
- `employee_type`
- `skill_ids` 或 `skill_keys`
- `mcp_server_ids` 或 `server_keys`
- `provider_types`
- `default_capability_selection`
- `context_policy_override`
- `approval_policy`
- `risk_level`
- `status`
- `source = builtin | custom`
- `created_by`
- `updated_by`
- `created_at`
- `updated_at`

真实关联原则：

- Skill 必须关联现有技能注册表，不能只是字符串。
- MCP 必须关联现有 MCP 配置或团队可用 MCP server，不能只是字符串。
- Provider 只允许选择当前注册 Provider 类型和团队治理允许范围。
- 保存模板时做服务端校验，模板定义能力必须存在，并且不能越过团队治理边界。
- 创建员工时使用模板快照，不直接引用可变模板，避免模板后续编辑影响已创建员工的历史配置和审计。

内置模板迁移方式：

- 现有 `DefaultEmployeeTypeDefinitions()` 继续作为 bootstrap seed。
- 新 API 优先读取数据库模板；没有数据库模板时 fallback 到内置定义。
- UI 区分 `内置模板` 和 `自定义模板`。
- 编辑内置模板时走 `复制为自定义模板`，不直接修改内置模板。

## 10. 实现范围

Web 侧预期改动：

- `apps/web/src/features/employees/index.tsx`
- `apps/web/src/features/employees/index.test.tsx`
- `apps/web/src/features/employees/create.tsx`
- `apps/web/src/features/employees/create.test.tsx`
- 新增模板列表页和详情页对应的 feature、route、test 文件。

后端不改动，除非实现中发现现有类型定义与 `create-options` 真实返回不一致。

## 11. 验收标准

- 创建页模板列不再重复显示英文角色或模板 type。
- 创建页不再使用 `推荐能力` 口径，改为 `模板能力`。
- `默认注入` 显示真实进入草稿的默认 enabled 能力数量。
- 数字员工首页有 `模板管理` 次入口。
- `/employees/templates` 能展示当前团队可用的内置模板。
- `/employees/templates/$templateType` 能展示模板详情、默认注入和治理影响说明。
- 从模板详情点击 `用此模板创建数字员工` 后，创建页预选对应模板。
- Runtime/Provider 可用性仍只在预检和运行步骤展示。

## 12. 测试计划

前端 focused tests：

- 创建页表格不再显示模板 type，列名更新为 `模板能力` 和 `风险等级`。
- 创建页 `默认注入` 根据 `default_capability_selection` 展示数量。
- 数字员工首页 `模板管理` 链接指向 `/employees/templates`。
- 模板列表页渲染 `create-options.employee_types`。
- 模板详情页展示基础信息、模板能力、默认注入和治理影响。
- 模板详情页的创建入口跳转到 `/employees/new?template=...`。
- 创建页支持 query 预选模板。

局部验证：

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx src/features/employees/index.test.tsx
corepack pnpm --filter ./apps/web run typecheck
git diff --check
```

真实链路验证：

- 登录真实 Web。
- 访问 `/employees`。
- 点击 `模板管理`。
- 打开一个模板详情。
- 点击 `用此模板创建数字员工`。
- 确认 `/employees/new` 预选该模板。
- 点击 `进入预检`，确认预检仍使用真实 `create-options` 接口和真实 Runtime/Provider 数据。

本轮不改后端持久化，因此不需要数据库迁移验证。
