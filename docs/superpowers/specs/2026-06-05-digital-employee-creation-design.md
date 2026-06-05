# 数字员工创建闭环设计

日期：2026-06-05  
状态：已确认，待实现计划

## 1. 背景

SuperTeam 已有数字员工业务身份、团队治理配置、数字员工个人配置修订、生效配置快照、唯一执行实例、Runtime provisioning preflight、Runtime command 下发和数字员工 run 执行闭环。

当前缺口集中在创建入口：

- Web 创建页仍停留在名称、角色、团队和配置预览，未提交 OpenAPI 已要求的 `runtime_node_id` 与 `provider_type`。
- 创建页没有 Owner、专业类型、能力选择、上下文/审批/输出契约等结构化配置。
- 前端缺少一个从后端拿创建候选的接口，只能拼团队治理和 Runtime 状态，容易把策略逻辑散到 Web。
- 创建成功的产品语义需要统一为 `ready`：准备好可被调用，但不自动运行任务。

本设计只覆盖专业执行型数字员工创建闭环，不覆盖数字员工健康总览、配置编辑器、能力同步修复和项目协调员创建。

## 2. 核心口径

数字员工是可复用的专业执行型业务身份。创建成功后的 `ready` 仅表示以下事实已经准备好：

- 身份和团队归属已创建。
- `owner_user_id` 已绑定当前创建者这个人类用户。
- 专业类型、角色画像、能力选择、上下文/审批/输出契约已形成初始个人配置修订。
- 团队治理配置与个人配置已合成为生效配置快照。
- 唯一执行实例已绑定 Runtime Agent 和 Provider。
- Runtime Agent 已完成 `provision_instance` 预置。

`ready` 不表示已经发起 run。后续运行入口有两条：

- 数字员工详情页手动发起任务。
- 项目、流程或调度器指定该数字员工执行任务。

项目协调员不进入数字员工创建类型。项目协调员属于项目创建和项目运行时的协调逻辑，可以由后端状态机承载，也可以调用专用 PI 小型 Agent 能力；它的身份由项目场景、汇报对象、协调偏向和注入提示词生成，不作为可复用数字员工被创建和管理。

## 3. 目标

- 新增数字员工创建向导，覆盖身份、Owner、专业类型、能力边界、治理覆盖、Runtime/Provider 和创建预检。
- 新增 `digital_employees.owner_user_id` 与 `digital_employees.employee_type` 独立列。
- `employee_type` 使用服务端注册表，不使用数据库 enum。
- 新增 `GET /api/v1/digital-employees/create-options?team_id=...`，由后端返回创建候选。
- 升级 `POST /api/v1/digital-employees`，在一次创建操作中完成身份、初始配置、生效配置、执行实例和 Runtime 预置。
- 创建成功后返回 `status=ready`，但不创建 run，不创建任务执行记录。
- 保持 Web -> Control Plane -> Runtime Agent -> Provider 边界，Web 不直连 Runtime 或 Provider。

## 4. 非目标

- 不做项目协调员创建。
- 不做数字员工首页健康总览、异常侧栏、能力来源统计和快速修复入口。
- 不做 AGENTS.md 个人编辑器；创建时继承团队默认 AGENTS.md。
- 不做能力同步、MCP 探测、导入、导出。
- 不实现 Runtime 自动选择、跨 Runtime fallback 或迁移。
- 不把专业类型做成 DB enum。
- 不提供裸 JSON 配置编辑器。

## 5. 方案选择

### 5.1 方案 A：创建编排接口和专业类型注册表

新增 `owner_user_id`、`employee_type`、`create-options`，并将创建接口升级为后端编排入口。前端只展示候选和提交选择，策略校验集中在 Control Plane。

优点：

- 符合 Control Plane 作为业务事实源的边界。
- Owner、专业类型、能力选择和 Runtime/Provider 候选都能审计。
- 前端不需要理解团队策略细节。
- 专业类型可以后续扩展到租户级注册表。

结论：采用。

### 5.2 方案 B：只修前端提交现有字段

只让 Web 提交 `runtime_node_id` 和 `provider_type`，不新增 Owner、专业类型和候选接口。

优点是实现最快。缺点是专业模板、责任链和创建预检无法形成稳定产品能力。

结论：不采用。

### 5.3 方案 C：完整能力绑定模型

同时新增员工能力绑定表、MCP 绑定表、AGENTS.md 版本表和健康投影。

长期更完整，但超出本轮“创建闭环优先”的范围。

结论：不采用。

## 6. 数据库设计

### 6.1 `digital_employees` 新增列

新增 forward migration：

```sql
ALTER TABLE digital_employees
  ADD COLUMN owner_user_id UUID,
  ADD COLUMN employee_type VARCHAR(100);
```

迁移需要为历史行回填：

- `owner_user_id` 可回填为默认开发管理员或创建记录能追溯到的用户；无法追溯时使用默认租户管理员并在迁移注释和 changelog 中说明。
- `employee_type` 回填为 `general_engineer` 或按历史角色映射。

回填后收紧为：

```sql
ALTER TABLE digital_employees
  ALTER COLUMN owner_user_id SET NOT NULL,
  ALTER COLUMN employee_type SET NOT NULL;
```

新增索引：

```sql
CREATE INDEX idx_digital_employees_owner_status
  ON digital_employees(tenant_id, owner_user_id, status, created_at DESC)
  WHERE deleted_at IS NULL;

CREATE INDEX idx_digital_employees_type_status
  ON digital_employees(tenant_id, employee_type, status, created_at DESC)
  WHERE deleted_at IS NULL;
```

不增加数据库 enum，不用封闭 CHECK 约束限制专业类型。

### 6.2 JSONB 的使用边界

继续使用 JSONB 保存版本化策略和快照：

- `digital_employees.permission_policy`
- `digital_employees.context_policy`
- `digital_employees.approval_policy`
- `digital_employee_config_revisions.role_profile`
- `digital_employee_config_revisions.capability_selection`
- `digital_employee_config_revisions.context_policy_override`
- `digital_employee_config_revisions.approval_policy_override`
- `digital_employee_config_revisions.output_contract_addendum`
- `digital_employee_effective_configs.effective_config_snapshot`

用户不直接编辑裸 JSON。Web 使用结构化表单，后端 DTO 校验后写入 JSONB。

字段提升规则：

- 需要筛选、统计、责任链或强授权判断的字段用独立列。
- 创建时的 Owner、专业类型、团队、状态和 Runtime/Provider 绑定使用独立列或现有执行实例列。
- 版本化治理内容、能力选择和输出契约继续用 JSONB 快照。

## 7. 专业类型注册表

首版在 Control Plane 内置注册表，后续可扩展为租户级注册配置。

内置类型示例：

- `database_admin`：数据库管理
- `devops_engineer`：DevOps 运维
- `frontend_engineer`：前端开发
- `backend_engineer`：后端开发
- `fullstack_engineer`：全栈开发
- `implementation_engineer`：实施工程师
- `general_engineer`：通用工程执行

每个类型定义：

- `type`
- `label`
- `description`
- `default_role`
- `default_risk_level`
- `default_role_profile`
- `recommended_skill_keys`
- `recommended_mcp_servers`
- `recommended_external_capabilities`
- `recommended_provider_types`
- `default_context_policy_override`
- `default_approval_policy_override`
- `default_output_contract_addendum`

注册表只给默认值和候选建议，最终提交仍必须满足团队治理策略。

## 8. API 设计

### 8.1 创建候选接口

新增：

```http
GET /api/v1/digital-employees/create-options?team_id={teamId}
```

授权：

- `Authorizer.Check(actor=user, action=employee.create, resource=tenant)`
- 读取团队治理时也应满足团队可读或治理可读边界；首版可通过员工创建权限覆盖，但审计原因要明确为 `digital employee create options`。

响应：

```json
{
  "team_config": {
    "id": "uuid",
    "revision_number": 3,
    "status": "active",
    "summary": {
      "allowed_provider_types": ["codex", "opencode"],
      "allowed_skills_count": 8,
      "allowed_mcp_servers_count": 3
    }
  },
  "employee_types": [
    {
      "type": "database_admin",
      "label": "数据库管理",
      "description": "负责数据库巡检、故障诊断、变更评估和运维证据整理。",
      "default_role": "database_admin",
      "default_risk_level": "medium",
      "recommended_skill_keys": ["database-troubleshooting"],
      "recommended_mcp_servers": ["postgres-readonly"],
      "recommended_provider_types": ["codex"],
      "default_output_contract": {
        "required_artifacts": ["diagnosis_report", "change_risk_summary"]
      }
    }
  ],
  "capability_options": {
    "skills": [],
    "mcp_servers": [],
    "external_capabilities": [],
    "provider_types": ["codex", "opencode"]
  },
  "runtime_provider_options": [
    {
      "runtime_node_id": "uuid",
      "node_id": "node-ops-01",
      "runtime_name": "运维节点 01",
      "provider_type": "codex",
      "health_status": "healthy",
      "current_load": 1,
      "max_slots": 4,
      "agent_home_dir_available": true
    }
  ],
  "policy_defaults": {
    "context_policy_override": {},
    "approval_policy_override": {},
    "output_contract_addendum": {}
  }
}
```

没有当前 active 团队治理配置时返回 422。没有可用 Runtime/Provider 时返回 200，并在 `runtime_provider_options` 中给出禁用候选及 `disabled_reason`；前端阻止提交。

### 8.2 创建接口

升级：

```http
POST /api/v1/digital-employees
```

请求新增字段：

```json
{
  "team_id": "uuid",
  "name": "数据库巡检员工",
  "employee_type": "database_admin",
  "role": "database_admin",
  "description": "负责数据库巡检和故障诊断",
  "risk_level": "medium",
  "runtime_node_id": "uuid",
  "provider_type": "codex",
  "role_profile": {},
  "capability_selection": {
    "enabled_skills": ["database-troubleshooting"],
    "enabled_mcp_servers": ["postgres-readonly"],
    "enabled_external_capabilities": []
  },
  "context_policy_override": {},
  "approval_policy_override": {},
  "output_contract_addendum": {},
  "session_policy": {},
  "workspace_policy": {},
  "metadata": {}
}
```

不接受客户端 `owner_user_id`。后端从当前登录用户写入。

响应中的 `DigitalEmployee` 增加：

- `owner_user_id`
- `employee_type`

创建成功后：

- `digital_employees.status = ready`
- `digital_employee_execution_instances.status = ready`
- 创建一条初始 `digital_employee_config_revisions`
- 创建一条 approved `digital_employee_effective_configs`
- 不创建 run
- 不创建任务执行记录

### 8.3 错误语义

- 团队不存在或不是 active：400 或 404，按现有团队接口语义。
- 团队无 active 治理配置：422。
- `employee_type` 不在注册表：400。
- 能力选择超出团队策略：400，返回 path 级错误。
- Runtime 不在线、session 无效、WebSocket 未连接：503。
- Provider 不健康或不被团队策略允许：503 或 400，错误码区分 `provider_unavailable` 与 `provider_policy_denied`。
- Runtime 预置失败：503，并触发补偿，不暴露 ready 员工。

## 9. Control Plane 服务行为

创建编排可以扩展现有 `employee.Service.CreateDraft`，也可以拆出更明确的 `CreateReadyEmployee`。实现计划阶段再根据代码规模决定命名，但外部语义是创建 ready 员工。

创建步骤：

1. 解析当前 tenant/user。
2. `Authorizer.Check(user, employee.create, tenant)`。
3. 校验 `team_id`、`name`、`role`、`employee_type`、`runtime_node_id`、`provider_type`。
4. 从专业类型注册表加载默认值。
5. 读取团队 active governance config。
6. 校验能力选择是团队允许范围的子集。
7. 调用现有 Runtime provisioning preflight，校验 Runtime/Provider。
8. 创建 `digital_employees`，写入 `owner_user_id`、`employee_type`、状态先为 `draft` 或内部 `provisioning` 过程状态。
9. 创建初始 `digital_employee_config_revisions`。
10. 预览并创建 approved `digital_employee_effective_configs`，`approved_by` 使用当前用户。
11. 创建唯一 execution instance，状态为 `provisioning`。
12. 下发 `provision_instance`。
13. Runtime 成功回写后，将员工和执行实例置为 `ready`。
14. 返回创建结果。

补偿规则：

- 在下发 Runtime 前失败，不应留下员工身份。
- Runtime 预置失败或等待失败，沿用并扩展 `AbortProvisionedDigitalEmployee`，清理或软删除员工、配置修订、生效配置和执行实例。
- 任何失败都不能让调度器看到可调度的 ready 员工。

审计：

- 读取 create-options 记录轻量审计或请求日志。
- 创建成功记录 `digital_employee.created_ready`。
- Runtime 预置成功记录 `digital_employee_instance_provisioned`。
- 创建失败和预置失败记录失败原因、Runtime、Provider、employee_type 和 request id。

## 10. Web 创建向导

入口：`/employees` 的“创建数字员工”按钮跳转 `/employees/new`。

采用四步向导。

### 10.1 身份

字段：

- 团队
- 名称
- 专业类型
- 角色
- 描述
- 风险等级
- Owner 当前用户只读展示

选择团队后加载 `create-options`。切换专业类型时填入默认 role、风险等级和推荐配置，用户可调整。

### 10.2 能力边界

字段：

- Skills 多选
- MCP servers 多选
- 外部能力多选

候选只来自团队允许范围。默认选中专业类型推荐项。页面展示团队默认 AGENTS.md 已继承，不提供个人 AGENTS.md 编辑器。

### 10.3 治理覆盖

结构化表单：

- 上下文来源和范围
- 需要人工确认的动作
- 输出格式
- 必交工件

表单结果写入：

- `context_policy_override`
- `approval_policy_override`
- `output_contract_addendum`

### 10.4 Runtime 与确认

字段：

- Runtime/Provider 组合选择

只允许选择在线、健康、团队允许、agent home dir 可用的组合。确认页展示：

- Owner
- 团队治理版本
- 专业类型
- 能力数量
- Runtime
- Provider
- 创建后状态：`ready`

主按钮文案：`创建并准备好`。

成功后跳转 `/employees/{employeeId}`，不自动发起 run。

## 11. OpenAPI 与客户端

`contracts/control-plane/openapi.yaml` 需要新增：

- `DigitalEmployeeCreateOptions`
- `EmployeeTypeOption`
- `CapabilityOptions`
- `RuntimeProviderCreateOption`
- `CreateDigitalEmployeeRequest.employee_type`
- `CreateDigitalEmployeeRequest.role_profile`
- `CreateDigitalEmployeeRequest.capability_selection`
- `CreateDigitalEmployeeRequest.context_policy_override`
- `CreateDigitalEmployeeRequest.approval_policy_override`
- `CreateDigitalEmployeeRequest.output_contract_addendum`
- `DigitalEmployee.owner_user_id`
- `DigitalEmployee.employee_type`

TypeScript client 新增：

- `getDigitalEmployeeCreateOptions(options, teamId)`
- 扩展 `CreateDigitalEmployeeInput`
- 扩展 `DigitalEmployee`

Go handler DTO 与 OpenAPI 保持同名字段。

## 12. 权限与安全

- `create-options` 和 `create` 都需要 Console session。
- 创建使用 `employee.create` action，resource 为 tenant。
- `owner_user_id` 只能从登录态写入。
- `approved_by` 只能从登录态写入。
- 客户端传入 `owner_user_id`、`approved_by` 等字段时必须忽略或拒绝；建议拒绝明确错误，避免误导调用方。
- Runtime/Provider 合法性以后端 preflight 为准，前端候选只用于体验。
- Provider-specific 配置留在 Runtime/Provider adapter，不进入数字员工核心模型。
- `provision_instance` payload 继续脱敏敏感字段。

## 13. 测试策略

Control Plane service tests：

- 创建成功写入 `owner_user_id` 和 `employee_type`。
- 客户端不能伪造 Owner。
- 未知 `employee_type` 被拒绝。
- 能力选择超出团队策略被拒绝。
- 创建成功生成员工、配置修订、生效配置、执行实例和 `provision_instance` command。
- Runtime 预置失败后不会留下 ready 员工。
- 团队无 active config 返回 `ErrEffectiveConfigRequired`。
- Runtime/Provider 不可用沿用 `ErrRuntimeUnavailable` / `ErrProviderUnavailable`。

Route tests：

- `GET /create-options` 校验鉴权、team_id 解析和响应字段。
- `POST /digital-employees` 映射新增字段。
- `POST /digital-employees` 不接受客户端 owner/approved_by。
- 授权 action/resource 仍为 `employee.create` + tenant。

Migration and sqlc tests：

- 新增列、索引和注释存在。
- sqlc 查询覆盖新增字段。
- initial schema 和 forward migration checksum 更新。

Web API tests：

- `getDigitalEmployeeCreateOptions` query 正确。
- `createDigitalEmployee` 发送新字段和 cookie credentials。

Web component tests：

- 向导必须选择团队、专业类型和 Runtime/Provider。
- 专业类型切换带出推荐配置。
- 能力候选只显示 create-options 返回的团队允许项。
- 无 active 团队治理时展示阻断。
- 无可用 Runtime/Provider 时展示阻断。
- 成功后跳转详情页。

Contract and verification：

- `go test ./apps/control-plane/internal/...`
- `pnpm --filter @superteam/web test -- employees`
- `pnpm --filter @superteam/web typecheck`
- `go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -config contracts/control-plane/oapi-codegen.server.yaml contracts/control-plane/openapi.yaml`
- `git diff --check`

## 14. 实施顺序建议

1. 数据库迁移和 sqlc 查询补齐 `owner_user_id`、`employee_type`。
2. Control Plane 增加专业类型注册表。
3. 新增 create-options service、handler、OpenAPI 和 tests。
4. 升级创建 service：Owner 注入、专业类型、能力选择、初始配置修订、生效配置批准和 provisioning 补偿。
5. 扩展 Web API client。
6. 新增 `/employees/new` 分步创建向导。
7. 更新列表页展示 `employee_type` 和 owner 摘要。
8. 跑测试和契约校验。

## 15. 后续阶段

后续可以独立设计：

- 数字员工能力健康总览。
- AGENTS.md、Skills、MCP、Runtime 同步状态和修复入口。
- 员工配置编辑与版本审批。
- 数字员工导入/导出。
- 租户级专业类型注册表。
- 项目创建时的项目协调员虚拟身份和 PI 小型 Agent 注入策略。
