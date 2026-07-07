# 数字员工创建流程重设计

日期：2026-07-07
状态：已确认，待实现计划

## 1. 背景

当前数字员工创建页已经具备创建路径、预检、身份、能力、治理、Provider 和确认创建等基础能力，但信息架构和产品语义存在四个问题：

- `空白自定义` 仍要求用户选择“员工类型”，与“用户正在定义一个新数字员工身份”的心智冲突。
- 选择某些团队后，`create-options` 会因为缺少 active governance config 返回 422，前端直接暴露技术错误。
- “角色”字段容易被理解为团队权限角色、项目角色或人类组织角色，但当前代码实际把它写入数字员工 `role_profile`，用于身份职责画像。
- 能力配置没有清晰区分团队继承能力和员工个人扩展能力。

另外，Provider 当前在 Web 文案中被称为“Provider 偏好”，但产品语义应为数字员工必选的执行器类型。数字员工创建时必须选择一个 Provider 类型，可以是 `codex`、`opencode` 或 `claude-code`。Runtime 节点是否在线只影响后续项目运行准备，不改变 Provider 必选语义。

本设计覆盖并修正 `2026-07-06-digital-employee-blank-custom-create-design.md` 中“空白自定义仍必须选择后端已有员工类型并对用户暴露员工类型表”的旧口径。实现时应以本设计为准。

## 2. 目标

- 将 `空白自定义` 定义为直接创建一个自定义数字员工身份，不再在用户路径中要求选择员工类型。
- 将“员工类型”降为系统内部治理字段；空白自定义路径由系统映射到内部 `custom_agent` 治理基类。
- 将“角色”用户可见文案改为“职责定位”，继续落到后端现有 `role` / `role_profile.role` 字段。
- 将“Provider 偏好”改为必选的“Provider 类型”或“执行器类型”。
- 将团队归属设为可选；选择团队时继承团队治理与团队能力，不选团队时走租户级默认治理。
- 将团队治理缺失从技术 422 转为清晰业务阻断。
- 将能力配置拆成只读的团队继承能力和可编辑的员工扩展能力。
- 创建成功后通过现有 effective skills / MCP / config 能力展示最终生效结果。

## 3. 非目标

- 不实现数字员工直接绑定 Runtime 节点。
- 不在创建时提交 `runtime_node_id`。
- 不把 Runtime 在线状态作为员工创建的硬阻断，除非团队策略或系统配置没有任何可选 Provider 类型。
- 不引入 JSON 高级编辑器。
- 不实现“从团队角色复制”或“从历史员工克隆”。
- 不把人类组织角色、团队权限角色或项目成员角色并入数字员工职责定位字段。
- 不绕过后端团队治理、Provider allowlist、能力 allowlist 或有效配置校验。

## 4. 产品语义

数字员工创建是在定义一个可被项目调度使用的 AI 执行业务身份。创建表单里的字段按以下语义解释：

| 用户可见概念 | 后端/内部字段 | 语义 |
| --- | --- | --- |
| 名称 | `name` | 数字员工显示名称 |
| 头像 | `avatar_asset_id` | 数字员工身份视觉标识 |
| 职责定位 | `role`, `role_profile.role` | 数字员工承担的业务职责说明 |
| 创建方式 | `creation_mode` 或 metadata | 模板创建或空白自定义，仅表达草稿来源 |
| 内部治理基类 | `employee_type` | 后端治理、默认策略和 allowlist 校验字段；空白自定义对用户隐藏 |
| 归属团队 | `team_id` | 可选团队归属，决定团队治理和团队能力继承 |
| 团队继承能力 | effective skills / MCP / capability bindings | 由团队绑定而来，创建页只读展示 |
| 员工扩展能力 | employee skill / MCP bindings 或 create capability selection | 用户为该员工额外选择的能力 |
| Provider 类型 | `provider_type` | 必选执行器类型，限定为 `codex`、`opencode`、`claude-code` |

`空白自定义` 的“空白”只表示不套用专业模板的职责、能力和治理建议，不表示跳过平台治理。后端仍需要内部 `employee_type` 时，由系统自动写入 `custom_agent`。

## 5. 用户流程

创建页采用以下步骤：

1. `创建方式`
   - `空白自定义`：直接定义一个新的数字员工身份。
   - `从模板创建`：使用模板初始化职责、治理和能力建议。
   - `从团队角色复制`、`从历史员工克隆` 可继续保持未开放。

2. `身份定义`
   - 填写名称、头像、职责定位和描述。
   - 空白自定义路径不出现“选择员工类型”。
   - 模板路径可以展示模板来源，但职责定位可由用户修改。

3. `归属团队`
   - 用户可以不选择团队。
   - 不选团队时，显示“租户级默认治理”。
   - 选择团队后调用 `create-options` 获取团队治理、能力和 Provider 候选。
   - 如果团队缺少 active governance config，本步骤阻断，显示业务原因和下一步动作。

4. `能力配置`
   - 上半区展示团队继承能力，只读，标记“团队继承”。
   - 下半区展示员工扩展能力，可选，标记“员工扩展”。
   - 已由团队继承的能力不允许重复选择为员工扩展。
   - 技能、MCP 和外部能力候选来自当前登录用户可见范围，再按团队策略过滤。

5. `治理预检`
   - 展示团队治理、员工配置、上下文、审批、能力边界和 Provider 边界检查。
   - 不直接展示 `employee effective config required: active team governance config is required`。
   - 阻断项必须给出业务原因和可行动作。

6. `Provider 类型`
   - 必选单选：`Codex`、`OpenCode`、`Claude Code`。
   - 标题和确认页均不得再出现“Provider 偏好”。
   - 展示当前支持该 Provider 的 Runtime 节点数量作为运行准备提示。
   - 没有在线 Runtime 不阻断创建，只提示“创建后运行前需要可用 Runtime”。

7. `确认创建`
   - 汇总身份、团队、团队继承能力、员工扩展能力、治理预检和 Provider 类型。
   - 空白自定义显示“自定义身份”。
   - 不展示对用户无意义的内部员工类型。

## 6. 接口与数据流

### 6.1 创建选项

继续使用：

```http
GET /api/v1/digital-employees/create-options
```

请求不带 `team_id` 时，返回租户级默认创建选项：

- 默认治理策略。
- 系统支持的 Provider 类型。
- 当前登录用户可见的技能、MCP 和外部能力候选。

请求带 `team_id` 且团队有 active governance config 时，返回团队创建选项：

- `team_config_status: "active"`。
- 团队治理配置摘要。
- 团队继承能力。
- 员工可扩展能力候选。
- 团队策略允许的 Provider 类型。

请求带 `team_id` 但团队缺少 active governance config 时，接口返回结构化 422 错误：

- HTTP status：`422`
- error code：`team_governance_config_required`
- message：可包含后端诊断文本，但前端不得直接透出英文技术错误。

前端收到该 code 后必须显示业务阻断：

```text
该团队尚未启用治理配置，不能在此团队下创建数字员工。
```

动作：

- `先不归属团队创建`
- `前往团队治理配置`

### 6.2 创建提交

继续使用：

```http
POST /api/v1/digital-employees
```

必填字段：

- `name`
- `avatar_asset_id`
- `role`
- `provider_type`
- `employee_type`

可选字段：

- `team_id`
- `description`
- `capability_selection`
- `context_policy_override`
- `approval_policy_override`
- `budget_policy`
- `environment_variables`
- `metadata` 或 `role_profile` 中的 `creation_mode`

空白自定义路径由前端提交内部 `employee_type = "custom_agent"`，但 UI 不展示该字段，也不让用户选择该字段。`creation_mode: "blank_custom"` 可写入 `metadata` 或 `role_profile` 作为审计来源，不作为后端分支的唯一依据。

团队继承能力不作为员工个人扩展能力提交。员工扩展能力只提交用户主动选择的个人扩展项。

### 6.3 创建后生效能力

创建成功后，详情页继续通过 effective 读取合并结果：

- 团队继承 skills：来自 `skill_team_bindings`。
- 员工扩展 skills：来自 `skill_agent_bindings`。
- 团队继承 MCP：来自团队 MCP binding。
- 员工扩展 MCP：来自员工 MCP binding。

列表必须保留来源标记：

- `团队继承`
- `员工扩展`

团队继承项只读，员工扩展项可移除或编辑。

## 7. 后端设计

### 7.1 内部自定义员工类型

新增或明确一个内部自定义员工类型：

- 标准 key：`custom_agent`。
- 展示名：`自定义数字员工`。
- 默认职责定位为空或通用默认值，但前端仍要求用户填写职责定位。
- 默认能力选择为空。
- 默认审批、上下文、预算策略使用租户或团队治理约束后的安全默认值。

如果历史数据或早期实现已经写入 `custom`，后端可以兼容读取或规范化为 `custom_agent`；新创建请求和新响应统一使用 `custom_agent`。

该类型必须进入后端员工类型注册表，以便现有 `CreateDigitalEmployee` 仍可复用：

- `EmployeeTypeDefinitionByType`
- `validateEmployeeTypeAllowedByTeamConfig`
- `initialEmployeeConfigInput`
- `validateInitialEffectiveConfig`

如果团队治理配置中显式限制 `allowed_employee_types`，需要决定是否允许 `custom_agent`。推荐规则：

- 团队允许 `custom_agent` 时，可在该团队下创建空白自定义员工。
- 团队未允许 `custom_agent` 时，前端在选择团队后显示“该团队不允许创建自定义数字员工”。
- 租户级无团队创建允许 `custom_agent`。

### 7.2 Provider 校验

后端继续要求 `provider_type` 必填，并限制支持集合：

- `codex`
- `opencode`
- `claude-code`

为避免前端使用下划线版本，前后端需统一值。当前测试中出现过 `claude_code`，但后端支持集合是 `claude-code`。实现计划必须明确做一次兼容决策：

- 推荐标准值为 `claude-code`。
- 前端展示 `Claude Code`，提交 `claude-code`。
- 如需兼容旧数据或旧测试，后端可在 `normalizeProviderType` 中把 `claude_code` 规范化为 `claude-code`，但输出和新提交统一为 `claude-code`。

团队策略限制 Provider 时，所选 Provider 必须在 allowlist 内。没有在线 Runtime 不应导致创建失败；项目运行准备阶段再判断 Runtime 可执行性。

### 7.3 团队治理缺失

当前 `GetCreateOptions` 和 `CreateDigitalEmployee` 在团队缺少 active governance config 时会返回 `ErrEffectiveConfigRequired`。本设计不取消后端校验，但需要让错误可被前端稳定识别。

建议增加稳定错误 code：

- `team_governance_config_required`

前端不得依赖英文错误字符串匹配。

### 7.4 能力边界

后端继续作为最终兜底：

- 员工扩展技能不能越过团队策略或用户可见范围。
- 员工扩展 MCP 不能越过团队策略或用户可见范围。
- 外部能力不能越过团队策略。
- 初始 effective config 不能存在 blocking validation errors。

创建页可以隐藏不可选项，但不能替代后端校验。

## 8. 前端设计

### 8.1 信息架构

创建页应避免以下用户可见词：

- `Provider 偏好`
- 空白路径中的 `选择员工类型`
- 用作权限概念的 `角色`

推荐文案：

- `Provider 类型`
- `执行器类型`
- `职责定位`
- `自定义身份`
- `团队继承`
- `员工扩展`

### 8.2 能力配置

能力配置页面采用两段式布局：

```text
团队继承能力（只读）
  技能
  MCP
  外部能力

员工扩展能力（可选）
  技能市场候选
  MCP 注册表候选
  外部能力候选
```

候选过滤规则：

- 登录用户不可见的能力不展示。
- 团队策略不允许的能力不展示。
- 已团队继承的能力不出现在员工扩展候选里，或展示为禁用并说明“已由团队继承”。

### 8.3 Provider 类型

Provider 选项用单选控件：

- `Codex`
- `OpenCode`
- `Claude Code`

每项展示：

- Provider 显示名。
- 提交值。
- 当前 Runtime 支持数量。
- 是否被团队策略允许。

没有在线 Runtime 时显示 warning，不禁用该 Provider：

```text
当前没有在线 Runtime 节点支持该 Provider；创建后运行前需要可用 Runtime。
```

没有任何可选 Provider 时阻断：

```text
当前没有可选 Provider 类型，请检查团队能力边界或系统 Provider 配置。
```

### 8.4 团队治理阻断

团队步骤在收到 `team_governance_config_required` 后进入阻断状态：

- 不允许继续能力配置。
- 显示业务原因。
- 提供改为无团队创建的动作。
- 提供前往团队治理配置的动作。

## 9. 错误处理

| 场景 | 用户可见处理 |
| --- | --- |
| 团队缺少 active governance config | 阻断团队路径，提示启用团队治理配置 |
| 团队不允许自定义员工 | 阻断该团队路径，提示选择其他团队或使用模板 |
| 没有可选 Provider 类型 | 阻断创建，提示检查团队能力边界或系统配置 |
| Provider 无在线 Runtime | 允许创建，提示运行前需要 Runtime |
| 员工扩展能力越权 | 前端不展示，后端拒绝并显示可读错误 |
| MCP 必需环境变量缺失 | 创建页提示，绑定或运行 preflight 继续兜底 |
| create-options 加载失败 | 阻断当前步骤，支持重试 |
| 创建提交失败 | 保留表单状态，展示后端业务错误 |

## 10. 视觉与组件约束

本功能属于低数据密度的创建入口，可以继续使用当前 v3 Soft-Flat 创建页风格。实现必须遵守：

- 先复用 `apps/web/src/components/superteam/` 的项目级组件。
- 不引入营销页式 hero 或新的视觉系统。
- 能力继承/扩展列表需要可扫读，使用稳定卡片或表格，不做装饰性大面积色块。
- Provider 类型使用单选控件，不使用模糊的文本按钮模拟选择。
- 阻断状态使用明确的状态面板和动作按钮。
- 所有用户可见文案默认简体中文。

## 11. 实现范围

主要修改：

- `apps/web/src/features/employees/create.tsx`
- `apps/web/src/features/employees/create.test.tsx`
- `apps/web/src/lib/api/employees.ts`
- `apps/control-plane/internal/employee/employee_types.go`
- `apps/control-plane/internal/employee/service.go`
- `apps/control-plane/internal/employee/service_test.go`
- `apps/control-plane/internal/employee/handler.go`
- `contracts/control-plane/openapi.yaml`，仅当需要新增结构化错误或 create-options 字段时修改。

可能修改：

- `apps/control-plane/internal/api/employee_routes_test.go`
- `apps/control-plane/internal/skill/*`
- `apps/control-plane/internal/capability/*`
- `apps/web/src/features/employees/components/employee-capabilities-panel.tsx`

不修改：

- Runtime Agent Provider 执行逻辑。
- 项目 Runtime placement/readiness 的事实源语义。
- 数字员工直接 Runtime 绑定模型。

## 12. 验收标准

- 空白自定义路径不再出现“选择员工类型”。
- 空白自定义路径能创建内部 `custom_agent` 类型员工，用户不需要理解该内部类型。
- 创建页所有用户可见位置不再出现 `Provider 偏好`。
- 未选择 Provider 类型时不能确认创建。
- Provider 类型只能选择 `codex`、`opencode`、`claude-code`。
- `Claude Code` 前端提交值与后端支持值一致。
- 团队治理缺失时显示业务阻断，不暴露英文 422。
- 团队继承能力只读展示。
- 员工扩展能力可从用户可见且团队允许的技能/MCP/外部能力中选择。
- 已由团队继承的能力不会重复提交为员工扩展能力。
- 创建成功后详情页 effective 能力能区分团队继承和员工扩展。
- 没有在线 Runtime 时仍可创建员工，但页面提示运行前需要 Runtime。

## 13. 测试计划

### Web 单测

覆盖：

- 空白自定义路径不出现员工类型选择表。
- 空白自定义路径显示“自定义身份”和“职责定位”。
- 页面不出现 `Provider 偏好`。
- Provider 类型未选中时确认按钮不可用。
- Provider 类型选择后，提交体包含 `provider_type`。
- `Claude Code` 提交值为 `claude-code`。
- 团队治理缺失显示业务阻断和两个动作。
- 团队继承能力只读，员工扩展能力可选。
- 已继承能力不会重复提交。

命令：

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx
```

### Control Plane 单测

覆盖：

- 空白自定义映射到内部 `custom_agent`。
- `custom_agent` 默认能力为空。
- 缺少 `provider_type` 被拒绝。
- 未知 Provider 被拒绝。
- `claude_code` 如需兼容，会被规范化为 `claude-code`。
- 团队缺少 active governance config 返回稳定错误 code。
- 团队策略外 Provider 被拒绝。
- 团队策略外能力被拒绝。

命令：

```bash
go test ./apps/control-plane/internal/employee ./apps/control-plane/internal/api
```

### 契约与代码生成

如果修改 OpenAPI 或生成代码：

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
```

### 通用检查

```bash
git diff --check
```

## 14. 真实链路验收

实现完成后需要在当前代码和真实服务上验证：

1. 使用真实登录会话访问 `/employees/new`。
2. 选择 `空白自定义`。
3. 填写名称、头像、职责定位和描述。
4. 不选择团队，选择 Provider 类型，完成创建。
5. 进入员工详情页，确认 Provider 类型、职责定位和 effective 能力展示正确。
6. 选择一个有 active governance config 的团队，完成创建。
7. 选择一个没有 active governance config 的团队，确认前端显示业务阻断。
8. 在 Runtime 全部离线或无匹配 Runtime 的情况下，确认创建页提示运行准备风险但不把它当成员工创建失败。

若真实服务、认证、数据库迁移或 Runtime 状态无法满足验证条件，不能声明功能完成，只能报告阻塞项。

## 15. 后续计划边界

下一步应进入实现计划，拆分为：

- 后端自定义员工类型与结构化错误。
- create-options 能力继承/扩展候选。
- Web 创建向导信息架构调整。
- Provider 类型必选与命名修正。
- 测试与真实链路验收。

本设计不直接进入实现，不改变当前工作区已有未提交变更。
