# 数字员工模板管理功能设计

日期：2026-07-09
> 复核状态：基于锚点抽查
状态：已确认，待实现计划

## 1. 背景

当前"数字员工模板管理"页面（`apps/web/src/features/employees/templates.tsx`）只是对 `GET /api/v1/digital-employees/create-options` 返回结果的只读投影。真正的模板数据是硬编码在 Go 代码里的 10 个 `EmployeeTypeDefinition`（`internal/employee/employee_types.go`），没有数据库表、没有增删改接口，页面文案也明确写着"只读查看内置模板"。

模板在业务上是数字员工创建向导的**默认值来源**（默认角色、推荐技能/MCP/Provider、默认能力选择、默认上下文策略、默认审批策略），不是员工自身的持久化配置——员工创建后走 `digital_employee_config_revisions` 独立演进，与模板脱钩，也没有任何外键指向模板。这意味着模板的增删改风险可控：改动模板不会影响任何已创建的数字员工。

## 2. 目标

- 数字员工模板从硬编码 Go 常量迁移为数据库持久化实体。
- 提供新建模板、编辑（配置）模板、启用/禁用模板、删除模板的完整能力，前后端打通。
- 保留现有 10 个内置模板的数据和创建向导的默认体验，迁移后内置模板与自定义模板享有同等的增删改权限。
- 数字员工创建向导（create-options）改为从数据库读取当前租户下状态为 `active` 的模板。

## 3. 非目标

- 不引入模板的团队级/个人级作用域（scope），模板固定为租户级。
- 不给模板管理新增独立权限点，复用现有 `authz.ActionEmployeeCreate`。
- 不处理真正的多租户 onboarding 流程；当前平台只有单一 bootstrap 租户（`platform.DefaultTenantID`），种子数据直接对该租户写入，等未来有真实租户创建流程时再补种子钩子。
- 不联动/级联修改已创建的数字员工——模板与员工之间无引用关系，删除模板对已有员工零影响。
- `custom_agent`（"自定义数字员工"创建模式的哨兵定义，当前从 UI 列表过滤掉）不迁移进数据库，继续保留为 Go 硬编码哨兵。

## 4. 数据模型

新增迁移 `apps/control-plane/internal/storage/migrations/050_digital_employee_templates.sql`：

```sql
CREATE TABLE digital_employee_templates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  type VARCHAR(64) NOT NULL,
  label VARCHAR(128) NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  default_role VARCHAR(128) NOT NULL DEFAULT '',
  recommended_skills JSONB NOT NULL DEFAULT '[]',
  recommended_mcp_servers JSONB NOT NULL DEFAULT '[]',
  recommended_provider_types JSONB NOT NULL DEFAULT '[]',
  default_capability_selection JSONB NOT NULL DEFAULT '{}',
  default_context_policy_override JSONB NOT NULL DEFAULT '{}',
  default_approval_policy JSONB NOT NULL DEFAULT '{}',
  metadata JSONB NOT NULL DEFAULT '{}',
  status VARCHAR(16) NOT NULL DEFAULT 'active',
  is_system BOOLEAN NOT NULL DEFAULT false,
  deleted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT digital_employee_templates_status_check CHECK (status IN ('active', 'disabled'))
);

CREATE UNIQUE INDEX digital_employee_templates_tenant_type_key
  ON digital_employee_templates (tenant_id, type) WHERE deleted_at IS NULL;

CREATE INDEX digital_employee_templates_tenant_status_idx
  ON digital_employee_templates (tenant_id, status) WHERE deleted_at IS NULL;
```

同一迁移内，为 `platform.DefaultTenantID` 插入现有 9 个内置定义（`database_admin`、`devops_engineer`、`security_engineer`、`qa_engineer`、`frontend_engineer`、`backend_engineer`、`fullstack_engineer`、`implementation_engineer`、`general_engineer`），`is_system = true`，`status = 'active'`，字段值与 `employee_types.go` 现有内容一致（做法参考 `002_seed_dev_admin.sql` 的固定 UUID 种子风格）。

`is_system` 仅用于前端展示"内置"标记，**不**限制任何增删改操作——内置模板与自定义模板权限完全等价。

字段命名和 JSONB 默认值风格与 `038_task_prompt_templates.sql` 保持一致。迁移后必须更新 `atlas.sum` 并跑 `make -C apps/control-plane migrate-validate`。

## 5. 后端架构

延续 `internal/employee` 包内按功能拆文件的既有惯例（`env_*.go`、`run_*.go`），新增：

- `template_types.go` — `EmployeeTemplateRecord`、`CreateEmployeeTemplateParams`、`UpdateEmployeeTemplateParams`、`ListEmployeeTemplatesParams`、`SetEmployeeTemplateStatusParams`
- `template_repository.go` — 在 `Repository` interface（`repository.go`）追加：
  - `ListEmployeeTemplates(ctx, tenantID, activeOnly bool) ([]EmployeeTemplateRecord, error)`
  - `GetEmployeeTemplate(ctx, tenantID, templateID) (EmployeeTemplateRecord, error)`
  - `CreateEmployeeTemplate(ctx, params) (EmployeeTemplateRecord, error)`
  - `UpdateEmployeeTemplate(ctx, params) (EmployeeTemplateRecord, error)`
  - `SetEmployeeTemplateStatus(ctx, tenantID, templateID, status) (EmployeeTemplateRecord, error)`
  - `SoftDeleteEmployeeTemplate(ctx, tenantID, templateID) error`
- sqlc 查询文件 `internal/storage/queries/digital_employee_templates.sql`（+ 生成的 `.sql.go`）
- `template_service.go` — 校验规则：
  - `type` 必须匹配 `^[a-z][a-z0-9_]{1,63}$`，租户内唯一（含软删除记录也不能重复，避免误解），创建后不可修改。
  - `label` 非空。
  - `recommended_skills` / `recommended_mcp_servers` / `recommended_provider_types` 必须是字符串数组。
  - `default_capability_selection` / `default_context_policy_override` / `default_approval_policy` / `metadata` 必须是 JSON 对象（map），非法结构返回 `ErrInvalidInput`。
- `template_handler.go` — HTTP handler，复用 `authz.ActionEmployeeCreate` 鉴权（与现有 `GetCreateOptions` 一致）。

`employee_types.go` 的硬编码切片和 `DefaultEmployeeTypeDefinitions()` / `EmployeeTypeDefinitionByType()` 删除（`custom_agent` 哨兵单独保留一个精简版函数）。`Service.GetCreateOptions`（`service.go:206-253`）改为调用 `s.repository.ListEmployeeTemplates(ctx, req.TenantID, true)` 获取当前租户下 `status = active` 的模板；`normalizeCreateDigitalEmployeeRequest`（`service.go:469-521`）改为通过 `GetEmployeeTemplate`/按 `type` 查询数据库获取默认值种子（禁用或已删除模板仍可用于查已创建员工的历史类型解释，但不再出现在创建向导可选列表中）。

## 6. API 契约

`contracts/control-plane/openapi.yaml` 新增资源化路由（与现有 `/digital-employees/create-options` 并存，不替换）：

```
GET    /api/v1/digital-employee-templates
POST   /api/v1/digital-employee-templates
GET    /api/v1/digital-employee-templates/{templateId}
PATCH  /api/v1/digital-employee-templates/{templateId}
PATCH  /api/v1/digital-employee-templates/{templateId}/status
DELETE /api/v1/digital-employee-templates/{templateId}
```

- `GET` 列表默认返回全部未删除模板（含 disabled），供模板管理页使用；`create-options` 内部调用走 service 层的 `activeOnly=true` 过滤，不额外经过这组 HTTP 路由。
- `POST`/`PATCH` request body 复用 `EmployeeTemplateRecord` 字段（`type` 仅 `POST` 接受，`PATCH` 忽略/拒绝该字段变更）。
- `PATCH .../status` body：`{"status": "active" | "disabled"}`。
- `DELETE` 为软删除（`deleted_at`），返回 204。
- 鉴权：全部复用 `authz.ActionEmployeeCreate`，与 `GetCreateOptions` 一致。

契约修改后需要走生成流程并跑契约验证。

## 7. 前端

`apps/web/src/features/employees/templates.tsx` 从只读投影改为真正的 CRUD 视图：

- **列表页**（`TemplateListView`）：标题/副标题去掉"只读"措辞。表格新增"内置"徽章列（来自 `is_system`），"状态"列改为模板自身的启用/禁用 `StatusPill`（来自 `status` 字段），原先由团队基线继承推导出的"继承团队基线"提示挪到详情页展示（仍然有意义，但不再冒充模板自身状态）。行操作：查看详情 / 配置 / 启用·禁用切换 / 删除（二次确认弹窗）。页面级操作新增"新建模板"，与既有"创建数字员工"并列。
- **新建/配置表单**（新增路由或弹窗 `/employees/templates/new`、`/employees/templates/$templateType/edit`）：字段为 label、description、default_role、recommended_skills（标签输入）、recommended_mcp_servers（标签输入）、recommended_provider_types（多选，来源于已知 provider 类型）、default_capability_selection / default_context_policy_override / default_approval_policy（JSON 编辑器，因为这些本就是自由格式的策略 map，与详情页现有的标签列表/键值展示对应）。`type` 字段仅创建时可填（校验唯一、`^[a-z][a-z0-9_]+$`），编辑态只读展示。
- **删除**：二次确认弹窗，文案"删除后不可恢复，已创建的数字员工不受影响"（准确——无外键关联），调用 `DELETE`，成功后从列表乐观移除。
- 新增 API client：`apps/web/src/lib/api/employee-templates.ts`（参考 `prompt-templates.ts` 风格），导出 `listEmployeeTemplates`、`createEmployeeTemplate`、`getEmployeeTemplate`、`updateEmployeeTemplate`、`setEmployeeTemplateStatus`、`deleteEmployeeTemplate`。
- `template-utils.ts` 精简：删除从团队基线推导假状态的 `templateAvailabilityStatus` 等逻辑中与"状态"相关的部分，改为直接读取 `template.status`；继承基线相关的展示逻辑保留但仅用于详情页的"继承基线"信息块。

## 8. 测试与验证

- 后端：`template_service_test.go`（校验规则）、`template_repository` 对应的集成测试（参照 `pg_repository_test.go` 现有模式）、`handler_test.go` 补充新路由的鉴权与成功/失败路径。
- 迁移：`make -C apps/control-plane migrate-validate` 通过，确认种子数据插入且幂等（重复跑迁移不报错）。
- 前端：`templates.test.tsx` 现有"只读"相关用例更新为 CRUD 交互用例（创建、编辑、启用/禁用、删除的表单校验和 API 调用）。
- 端到端（本仓库强制要求）：通过 `scripts/dev-services.sh status` 确认 Control Plane / Web 实际运行当前代码后，在浏览器里走一遍新建模板 → 配置修改 → 禁用 → 删除的完整链路，并确认创建数字员工向导的可选模板列表随之变化（禁用后消失、新建后出现）。
