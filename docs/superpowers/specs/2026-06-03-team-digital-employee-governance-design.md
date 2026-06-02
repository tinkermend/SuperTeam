# 团队与数字员工治理模型 Spec

> 日期：2026-06-03
> 状态：待书面评审
> 决策：团队作为无层级职能治理模板，数字员工作为继承团队边界后的个人执行身份

## 1. 背景

SuperTeam 是企业级数字员工控制平面。当前系统已经具备 Web 控制台、登录、权限中心、Runtime 接入审批、短期 Runtime Session、数字员工基础表、数字员工执行实例和 Provider Session 事件边界。下一步需要先让用户能创建和治理数字员工，而不是直接进入复杂任务编排。

数字员工不是聊天机器人，也不是纯 Provider 配置。它应该模拟企业里的岗位身份：有职责、有团队归属、有可用能力、有上下文边界、有输出规范、有风险策略，并且通过 Runtime Agent 管理下的 Provider Session 执行任务。

本设计参考 Paperclip 的 agent、org chart、skills、adapter、approval 和 heartbeat 思路，但不照搬其“AI 公司”模型。SuperTeam 的团队不做上下级组织树，不让数字员工自由跨团队委派，也不让团队变成流程阶段。团队是企业长期职能单元，负责公共治理模板和能力上限；数字员工是团队内的专业岗位；跨团队交接由团队绑定的人类负责人决策。

## 2. 目标

本 spec 目标是定义第一版团队与数字员工治理模型：

- 团队没有层级，是租户内的长期职能单元。
- 每个团队绑定一个主要人类负责人。
- 团队定义公共 MCP、技能、插件、外部能力、Provider、宪法、上下文策略、审批策略、工件契约、Runtime 范围和内部协作策略。
- 数字员工必须属于一个团队。
- 数字员工继承团队公共配置，并只能在团队允许范围内做个人补充。
- 个人能力不能突破团队能力边界。
- 个人上下文策略只能收窄团队上下文范围。
- 个人审批策略只能提高要求，不能降低团队要求。
- 个人宪法只能追加补充，不能修改或削弱团队硬性规则。
- 团队内低风险信息问询可以自动化，但必须结构化、限流、可审计。
- 跨团队流转、扩权、写操作、风险升级和业务决策必须交给团队人类负责人。

## 3. 非目标

本 spec 不做以下事项：

- 不实现团队层级。
- 不实现复杂组织树或 Paperclip 式 CEO/manager 汇报树。
- 不把团队定义为流程阶段，例如需求团队、开发团队、测试团队之间的固定流水线不是团队模型本身。
- 不让数字员工自由跨团队拉人、转派或执行动作。
- 不做团队配置的复杂自动语义合并。
- 不自动判断自然语言宪法是否削弱团队宪法。
- 不实现完整能力注册市场；第一版可以用结构化 JSON 保存 MCP、技能、插件和外部能力引用。
- 不实现多个团队负责人、值班组、观察者等复杂人类角色。
- 不实现任务级动态负责人模型。

## 4. 核心定义

### 4.1 Team

团队是租户内长期存在的职能治理单元，例如：

- 运维团队
- 研发团队
- 测试团队
- 安全团队
- 需求分析团队

团队不表达上下级组织结构。团队的核心作用是定义公共治理模板和能力上限。

### 4.2 Human Owner

每个团队第一版只绑定一个主要人类负责人。该负责人负责：

- 审批高风险动作。
- 接收团队内数字员工的结构化汇报。
- 判断任务是否关闭、退回、继续处理或转派到其他团队。
- 对团队配置版本变更进行确认。
- 对超出内部协作预算、结论冲突或风险升级的情况做决策。

### 4.3 Digital Employee

数字员工是团队内的专业岗位身份，例如：

- 运维团队中的数据库运维数字员工。
- 运维团队中的缓存运维数字员工。
- 研发团队中的后端开发数字员工。
- 测试团队中的回归测试数字员工。

数字员工必须归属于一个团队。它继承团队公共配置，再追加个人岗位职责、个人能力选择、个人宪法补充、个人上下文收窄和执行实例绑定。

### 4.4 Task / Incident / Workflow Run

任务、故障和流程运行是临时协作场景，不是团队。它们可以指派给团队，也可以由团队负责人选择团队内一个或多个数字员工参与。

## 5. 对象关系

```text
Tenant
  -> Team
      -> Human Owner
      -> Team Constitution
      -> Team Capability Policy
      -> Team Context Policy
      -> Team Approval Policy
      -> Team Artifact Contract
      -> Team Internal Collaboration Policy
      -> Team Runtime Scope
      -> Digital Employee
          -> Personal Role Profile
          -> Personal Constitution Addendum
          -> Personal Capability Selection
          -> Personal Context Narrowing
          -> Personal Approval Override
          -> Execution Instance
```

关键约束：

- 一个数字员工第一版只属于一个团队。
- 数字员工个人 MCP、技能、插件和外部能力必须是团队 allowlist 的子集。
- 数字员工个人上下文策略必须是团队上下文策略的子集。
- 数字员工个人审批策略不能降低团队要求。
- 数字员工个人宪法只能追加 addendum。
- 团队负责人是跨团队转派和高风险决策入口。

## 6. 团队配置

团队配置需要版本化。第一版建议在现有 `tenant_teams` 基础上增加团队配置版本对象。

### 6.1 Team Profile

保存团队基础信息：

- 团队名称。
- 职责域。
- 人类负责人用户 ID。
- 状态。
- 描述和元数据。

### 6.2 Team Constitution

团队公共宪法分成结构化字段：

- `principles`：原则。
- `hard_rules`：硬性禁令。
- `required_output_rules`：必须遵守的输出规则。

个人宪法不能编辑这些字段，只能追加个人补充。

### 6.3 Team Capability Policy

团队能力策略定义数字员工可用能力上限：

- 允许的 MCP Server。
- 允许的技能。
- 允许的插件。
- 允许的外部能力。
- 允许的 Provider 类型。

第一版可以将这些能力保存为结构化 JSON，并保留未来拆成注册表和绑定表的空间。

### 6.4 Team Context Policy

团队上下文策略定义最大上下文范围：

- 可访问的知识库。
- 可访问的文档。
- 可访问的代码仓库。
- 可访问的日志、监控和告警数据。
- 可访问的客户资料或业务资料。

数字员工个人上下文只能选择子集。

### 6.5 Team Approval Policy

团队审批策略定义哪些情况必须暂停给人类负责人：

- 高风险动作。
- 写操作、删除、发布、配置变更、扩容、重启服务等动作。
- 访问团队外数据或能力。
- 任务目标变化。
- 风险等级升高。
- 多个数字员工结论冲突。
- 超过内部协作预算。

数字员工个人审批策略可以提高要求，不能降低要求。

### 6.6 Team Artifact Contract

团队定义统一交接工件格式：

- `Finding`
- `Risk`
- `Artifact`
- `DecisionRequest`
- `ExecutionResult`
- `NextActionProposal`
- `Blocker`

数字员工可以追加岗位字段，但不能删除团队要求字段。

### 6.7 Team Internal Collaboration Policy

团队内部协作策略定义同团队数字员工之间的自动信息问询边界。

允许自动化的请求类型：

- `info_request`
- `review_request`
- `artifact_request`

必须限制：

- 最大自动参与人数。
- 最大自动问询轮次。
- 最大自动成本。
- 最大自动持续时间。
- 结论冲突处理策略。
- 超预算升级策略。

团队内协作不等于自由聊天，所有问询必须写入任务事件流。

### 6.8 Team Runtime Scope

团队 Runtime 范围定义：

- 允许使用的 Runtime 节点或范围。
- 允许的 workspace 根目录。
- 允许的执行环境。
- 允许的 Provider 能力。

数字员工绑定执行实例时必须满足团队 Runtime 范围。

## 7. 数字员工配置继承

数字员工创建流程：

```text
选择团队
  -> 继承团队公共配置
  -> 填写岗位职责
  -> 选择个人启用能力
  -> 补充个人宪法
  -> 收窄上下文策略
  -> 配置个人审批要求
  -> 绑定执行实例
  -> 预览生效配置
  -> 团队负责人确认
  -> 进入 ready 或 active
```

继承规则：

- 团队能力是 allowlist。
- 团队宪法是不可削弱基线。
- 团队上下文是最大范围。
- 团队审批策略是最低要求。
- 团队工件契约是交接格式基线。
- 团队内部协作策略是自动问询上限。

创建结果必须生成生效配置快照：

```text
DigitalEmployee
  team_id
  inherited_tenant_team_config_revision_id
  personal_config_revision_id
  effective_config_snapshot
```

`effective_config_snapshot` 记录当时实际生效的团队配置和个人配置合并结果。团队配置升级时，不应静默改变所有数字员工。团队负责人需要选择：

- 应用到所有数字员工。
- 只应用到新数字员工。
- 对指定数字员工逐个确认。

## 8. 团队内自动协作边界

同团队内低风险信息问询默认不需要人类审批，但必须满足全部条件：

- 发生在同一个任务、故障或流程运行内。
- 发生在同一个团队内。
- 请求类型是 `info_request`、`review_request` 或 `artifact_request`。
- 只请求信息、分析、复核或工件补充。
- 不执行写操作。
- 不扩大上下文权限。
- 不调用团队未允许的能力。
- 不修改任务归属。
- 不修改风险等级。
- 不修改审批策略。
- 输出必须是结构化对象。

建议内部请求对象：

```text
InternalInfoRequest
  task_id
  team_id
  requester_employee_id
  target_employee_id
  request_type
  question
  requested_artifact_type
  reason
  deadline
  budget_snapshot
```

必须升级给团队负责人的情况：

- 跨团队协作或转派。
- 需要访问团队外数据、MCP、插件或外部能力。
- 需要执行写操作。
- 任务目标发生变化。
- 风险等级升高。
- 多个数字员工结论冲突。
- 超过协作人数、轮次、成本或时长上限。
- 需要业务取舍或责任判断。

## 9. 任务和故障协作数据流

### 9.1 故障场景

```text
Incident: 支付接口异常
  -> 运维团队
      -> 数据库运维数字员工产出 Finding: DB 慢查询
      -> 缓存运维数字员工产出 Finding: Redis 命中率下降
      -> 中间件运维数字员工产出 Finding: 网关 5xx 上升
  -> 团队汇总报告
  -> 运维负责人收到 DecisionRequest
      -> 转派研发团队修复代码
      -> 要求运维团队先扩容
      -> 要求安全团队排查攻击
      -> 标记为误报或关闭
```

### 9.2 软件交付场景

```text
需求任务
  -> 需求分析团队产出需求说明 Artifact
  -> 人类负责人审批
  -> 转派研发团队
  -> 开发数字员工产出实现 Artifact
  -> 人类负责人审批
  -> 转派测试团队
  -> 测试数字员工产出测试报告 Artifact
  -> 人类负责人验收
  -> 转派运维团队上线或关闭
```

跨团队流转始终是人类动作。数字员工只能提出 `NextActionProposal` 或 `DecisionRequest`。

## 10. 数据模型建议

当前已有：

- `tenant_teams`
- `digital_employees`
- `digital_employee_execution_instances`
- `runtime_capabilities`
- `provider_sessions`

建议新增或收敛：

```text
tenant_team_config_revisions
  id
  tenant_id
  team_id
  revision_number
  constitution
  capability_policy
  context_policy
  approval_policy
  artifact_contract
  internal_collaboration_policy
  runtime_scope_policy
  human_owner_user_id
  status
  approved_by
  approved_at
  created_at
  updated_at

digital_employee_config_revisions
  id
  tenant_id
  digital_employee_id
  revision_number
  role_profile
  constitution_addendum
  capability_selection
  context_policy_override
  approval_policy_override
  output_contract_addendum
  status
  created_at
  updated_at

digital_employee_effective_configs
  id
  tenant_id
  digital_employee_id
  tenant_team_config_revision_id
  employee_config_revision_id
  effective_config_snapshot
  validation_result
  status
  approved_by
  approved_at
  created_at
  updated_at
```

内部协作请求可在第一版作为任务事件保存。若后续需要独立查询、限流和审计，可拆出：

```text
task_internal_collaboration_requests
task_internal_collaboration_responses
```

## 11. API 边界

第一版建议新增或扩展：

```text
GET  /api/v1/teams
POST /api/v1/teams
GET  /api/v1/teams/{teamId}
POST /api/v1/teams/{teamId}/config-revisions
GET  /api/v1/teams/{teamId}/config-revisions/current

POST /api/v1/digital-employees
GET  /api/v1/digital-employees
GET  /api/v1/digital-employees/{employeeId}
POST /api/v1/digital-employees/{employeeId}/config-revisions
POST /api/v1/digital-employees/{employeeId}/effective-configs/preview
POST /api/v1/digital-employees/{employeeId}/effective-configs/approve
```

`effective-configs/preview` 返回：

- 生效配置快照预览。
- 阻断错误。
- 可保存草稿的警告。
- 团队继承配置摘要。
- 个人补充配置摘要。

`effective-configs/approve` 由团队负责人调用，成功后数字员工可以进入 `ready` 或 `active`。

## 12. Web 页面

### 12.1 团队页面

`/teams` 页面提供团队列表和团队详情。

团队详情展示：

- 基础信息。
- 人类负责人。
- 公共宪法。
- 能力边界。
- 上下文策略。
- 审批策略。
- 内部协作策略。
- 工件契约。
- Runtime 范围。
- 配置版本。

### 12.2 数字员工页面

`/employees` 页面提供数字员工列表、详情和创建向导。

创建向导步骤：

```text
1. 选择团队
2. 定义岗位角色
3. 选择个人能力
4. 补充个人宪法和输出要求
5. 收窄上下文和审批策略
6. 绑定执行实例并预览生效配置
```

第 6 步必须展示：

- 团队继承配置。
- 个人补充配置。
- 阻断错误。
- 草稿警告。
- 是否可提交给团队负责人确认。

## 13. 错误处理

### 13.1 必须阻断

以下情况不能激活数字员工：

- 未选择团队。
- 团队没有人类负责人。
- 个人能力超出团队 allowlist。
- 个人上下文策略扩大团队上下文范围。
- 个人审批策略降低团队审批要求。
- 缺少有效执行实例。
- Runtime 不可用或 Provider 能力不可用。
- 团队配置版本已过期，但员工仍基于旧草稿提交。
- 个人宪法试图覆盖团队硬性禁令。

### 13.2 允许保存草稿

以下情况可以保存为 `draft`，但不能直接 `active`：

- 缺少个人能力选择。
- 缺少个人宪法补充。
- 输出工件契约只使用团队默认。
- 内部协作策略只使用团队默认。
- 团队配置更新后，员工还没有确认是否应用新版本。

## 14. 测试范围

后端服务测试：

- 创建团队配置版本。
- 创建数字员工草稿。
- 预览生效配置。
- 校验个人能力不得超出团队边界。
- 校验个人上下文不得扩大团队范围。
- 校验个人审批策略不得降低团队要求。
- 校验团队配置版本变化后的员工确认逻辑。
- 校验团队内自动协作预算和升级规则。

API/契约测试：

- `/teams` 和 `/digital-employees` API path 与 OpenAPI 一致。
- 创建数字员工时返回继承配置摘要。
- preview 返回阻断错误和警告。
- approve 写入生效配置快照。

Web 测试：

- 团队页面能展示人类负责人、宪法、能力边界和内部协作策略。
- 创建数字员工向导能按步骤推进。
- 超出团队能力时显示阻断错误。
- 生效配置预览能区分团队继承和个人补充。
- 缺少执行实例时不能激活。

## 15. 第一版边界

第一版应控制实现范围：

- 只支持无层级团队。
- 只支持一个团队人类负责人。
- 只支持数字员工单团队归属。
- 能力引用先用结构化 JSON，后续再拆注册表。
- 宪法削弱判断先通过结构化字段避免，而不是做自然语言语义判断。
- 团队内协作只支持信息、复核和工件请求，不支持自动跨团队转派。
- 任务、故障和流程运行统一使用结构化对象交接，不引入自由聊天协作。

这个边界能先支撑“创建数字员工”和“团队内问题处理”的核心产品体验，同时避免第一版过度设计。
