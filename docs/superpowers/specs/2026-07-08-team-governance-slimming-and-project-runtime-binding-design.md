# 团队治理瘦身 + 项目绑定可运行节点 — 设计

- 日期：2026-07-08
- 范围：Control Plane 契约 / Go / DB 迁移 / Web 控制台
- 状态：设计已确认，待写实现计划

## 1. 背景与问题

当前"团队治理"把大量本不属于团队的配置堆在了 `TeamConfigRevision`（契约 `TeamConfigRevision` 8301、`DigitalEmployeeCreateTeamConfig` 10126）上：`approval_policy`、`context_policy`、`artifact_contract`、`internal_collaboration_policy`、`runtime_scope_policy`，以及作为"能力天花板白名单"的 `capability_policy.allowed_*`、`allowed_employee_types`、`allowed_provider_types`。

这套是"团队 = 封闭治理沙盒、员工被团队锁死"的旧模型产物，和现有业务模型直接冲突：

- 员工不绑定任何 runtime agent。
- 项目绑定多个 runtime agent。
- 项目可跨多个团队调用数字员工。

于是"团队限定 provider / employee type / runtime scope"这类白名单和真实调度路径矛盾：员工被 A 团队"允许的 provider"限制，却要在项目绑定的、属于别处的 runtime 上跑，白名单无从执行。

团队的设计初衷是把**公共部分抽出来**：团队有团队公共的技能、MCP、外部能力和宪法；员工加入团队即自动继承这些公共能力与宪法，个人再叠加专属能力——与现实团队一致。

同时，被错误地放在团队上的"运行范围"，其合法归宿是**项目**：项目创建时应显式多选可运行的 runtime 节点，而不是塞进一个 policy blob。

## 2. 目标模型

### 2.1 团队（Team）终态

```
Team = {
  id, tenant_id, slug, name, display(metadata),
  human_owner_user_ids,                 // 团队负责人：管理员工、技能、MCP
  constitution:          { hard_rules: string[] },   // 团队宪法，成员继承
  skills:                text[],        // 团队公共技能，继承基线
  mcp_servers:           text[],        // 团队公共 MCP，继承基线
  external_capabilities: text[],        // 团队公共外部能力，继承基线
}
```

团队负责人（team owner）负责团队管理：数字员工的添加、技能管理、MCP 管理、外部能力管理、宪法维护。这与项目人类负责人（负责审批、通知、任务管理）职责不同，互不混淆。

### 2.2 能力语义：继承基线（叠加，无上限）

团队的技能 / MCP / 外部能力对成员是**继承基线**，不是白名单天花板：

```
员工有效技能     = 团队.skills                ∪ 员工.personal_skills
员工有效 MCP      = 团队.mcp_servers           ∪ 员工.personal_mcp_servers
员工有效外部能力  = 团队.external_capabilities  ∪ 员工.personal_external_capabilities
员工有效宪法     = 团队.constitution.hard_rules ++ 员工.constitution_addendum
无团队员工：团队部分为空，只保留个人部分。
```

- 团队基线对员工是**纯只读继承，不可退订**：公共能力是强制基线，成员一律拥有，只能在其上叠加个人能力。
- 持久化只存员工的**个人追加**部分；继承在读取 / 派发时合成。
- 团队不再对 approval、context、provider 类型、employee 类型有任何发言权。

### 2.3 项目绑定可运行节点（新增）

从团队删除的"运行范围"落到项目，并升级为一等的显式多选：

```
CreateProjectRequest 新增：
  runtime_node_ids: uuid[]     // 项目可运行的 runtime 节点，多选，创建时强制至少一个

DB 新增关联表：
  project_runtime_nodes (project_id, runtime_node_id, tenant_id, created_at)
  // 一个项目 ↔ 多个 runtime 节点

调度约束：
  项目派任务时只在绑定的 runtime_node_ids 集合内选节点；
  provider 可用性 = 这些节点上真实注册的 provider。
```

## 3. 删除清单（全栈）

### 3.1 契约（`contracts/control-plane/openapi.yaml`）

- 删除 `TeamConfigRevision`、`CreateTeamConfigRevisionRequest` 及团队治理 revision 相关路径（draft / current / approve / reject / diff 全套）。
- `DigitalEmployeeCreateTeamConfig` 精简为 team baseline：`{ team_id?, constitution, skills[], mcp_servers[], external_capabilities[] }`；删除 `allowed_employee_types`、`allowed_provider_types`、`allowed_skills`、`allowed_mcp_servers`、`allowed_external_capabilities`、六个 policy blob、`revision_number`、`status`。
- `DigitalEmployeeCreateOptions`：`employee_types` 恒为平台全量、`capability_options` 恒为平台全量可选池（不再按团队裁剪）；删除团队治理相关 `creation_checks`。
- Team schema 增加 `constitution`、`skills`、`mcp_servers`、`external_capabilities` 字段。
- `CreateProjectRequest` 增加 `runtime_node_ids: uuid[]`（required，minItems 1）。
- 重新生成 gen 代码，跑契约验证。

### 3.2 Control Plane Go

- `apps/control-plane/internal/employee/service.go`：删除 `ErrEffectiveConfigRequired` 闸（约 492）、`allowedEmployeeTypesFromTeamConfig`（400）、`validateEmployeeTypeAllowedByTeamConfig`（498）、`employeeTypesForTeamConfig` 的团队裁剪逻辑、`capability_policy.allowed_*` 天花板过滤；`capabilityOptionsForTeamConfig` 改为返回平台全量池 + 团队基线。
- `apps/control-plane/internal/tenant`：删除 `TeamConfigRevision` 类型、draft/approve/reject/diff service 与 repository；team 增加 constitution / skills / mcp_servers / external_capabilities 读写。
- `apps/control-plane/internal/project`：新增 `project_runtime_nodes` 绑定的 service / repository / handler；创建时校验 `runtime_node_ids` 非空、同租户、节点存在；`team_id` 保持不变（可选锚点，仅参与 `validateProjectTeamScopeAccess` 权限校验，不动）。
- 清理各 `*.gen.go` 中被删 schema 的生成代码。

### 3.3 DB 迁移（`apps/control-plane/internal/storage/migrations/`）

- 新增 team 列：`constitution jsonb`、`skills text[]`、`mcp_servers text[]`、`external_capabilities text[]`。
- 数据搬运：把每个团队 active `team_config_revision` 的 `constitution.hard_rules` 与 `capability_policy.allowed_skills / allowed_mcp_servers / allowed_external_capabilities` 迁到新 team 列（值保留，语义从"天花板"变"基线"）；无 active revision 的团队取空。
- 删除 `team_config_revisions` 表及相关列。
- 新增 `project_runtime_nodes` 表。
- 迁移必须可重放，更新 `atlas.sum`，用 `make -C apps/control-plane migrate-validate` 校验。

### 3.4 Web 控制台

- 删除 `apps/web/src/features/teams/components/team-governance-tab.tsx` 及其 draft/approve/reject/diff API 调用；团队详情改为直接编辑 constitution + skills + mcp_servers + external_capabilities（团队负责人可编辑）。
- `apps/web/src/features/employees/create.tsx`：删除"治理"步（`configSteps` 从 `["身份","能力","治理","Provider 类型"]` 变为三步）；删除团队治理版本 / 允许员工类型 / 允许 Provider 展示（1516–1531）；员工自有 approval/context 默认值并入"能力"或"身份"步；继承能力保持**只读**展示（承接现有"团队继承能力"，`inheritedCapabilitySelection` / `withoutValues` 复用）。
- 项目创建向导（`apps/web/src/features/projects/components/create-project/`）：用 `listRuntimeNodes`（`GET /api/v1/runtime/nodes`）拉可用节点，多选勾选，展示节点名 / 状态 / 负载 / provider；强制至少选一个。

## 4. 数据流与错误处理

- **有效能力解析**为单一事实源：持久化只存个人追加，运行时合成 `团队基线 ∪ 个人`。
- 删除 `ErrEffectiveConfigRequired` 后，团队存在即可建员工；无团队员工团队基线为空。
- 员工类型来自平台注册表 `DefaultEmployeeTypeDefinitions()`；provider 可用性来自项目绑定节点上真实注册的 provider。
- 项目 `runtime_node_ids`：创建时强制非空、校验同租户且节点存在；空列表在创建阶段直接拒绝（非派发阶段才发现）。
- 迁移兼容：旧治理数据按 3.3 搬运，其余 policy 列直接 drop。

## 5. 测试与验证

### 5.1 自动化测试

- Web（`corepack pnpm --filter ./apps/web run test`）：员工创建三步向导快照 / 交互；继承能力只读展示；团队详情能力 + 宪法编辑；项目创建节点多选与"至少一个"校验。
- Go：员工创建不再依赖 team revision；有效能力合成；项目 ↔ 节点绑定 CRUD + 权限校验 + 非空校验；删除治理后旧路由 / 字段的回归清理。
- 契约：重新生成 + 契约验证。

### 5.2 真实端到端验证（CLAUDE.md 强制完成条件）

迁移 → `scripts/dev-services.sh restart control-plane` → Web 真实建团队（配公共技能 / MCP / 外部能力 / 宪法）→ 建员工（验证继承 + 平台全量类型 / provider，无治理步）→ 建项目（多选节点绑定，至少一个）→ 走真实 Web / 接口确认结果不是 mock、缓存或旧服务。涉及迁移与前后端行为，必须确认运行中的服务已加载当前代码。

## 6. 明确排除的范围

- 项目 `team_id` 字段：现为可选锚点，仅参与 `validateProjectTeamScopeAccess` 权限校验，与跨团队 `members[]` 并存，无真冲突，本次不动。
- 项目自身的 `approval_policy` / `coordination_policy` / `evidence_policy`：属于项目 / 人类决策层，本次不涉及。
- 员工自有 approval/context 默认值的语义：保留现状，仅调整创建向导的呈现位置。
