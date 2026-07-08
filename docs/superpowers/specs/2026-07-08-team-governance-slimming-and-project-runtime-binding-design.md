# 团队治理瘦身 + 项目绑定可运行节点 — 设计

- 日期：2026-07-08
- 范围：Control Plane 契约 / Go / DB 迁移 / Web 控制台
- 状态：设计已确认（含探查纠偏），待写实现计划
- 拆分：本设计拆成两份实现计划——A 团队治理瘦身、B 项目绑定可运行节点。先做 A。

## 1. 背景与问题

当前"团队治理"把大量本不属于团队的配置堆在 `TeamConfigRevision`（契约 `TeamConfigRevision` 8301、`DigitalEmployeeCreateTeamConfig` 10126、DB `tenant_team_config_revisions`）上：`approval_policy`、`context_policy`、`artifact_contract`、`internal_collaboration_policy`、`runtime_scope_policy`，以及作为"能力天花板白名单"的 `capability_policy.allowed_*`、`allowed_employee_types`、`allowed_provider_types`。

这是"团队 = 封闭治理沙盒、员工被团队锁死"的旧模型产物，与现有业务模型冲突：

- 员工不绑定任何 runtime agent。
- 项目绑定多个 runtime agent。
- 项目可跨多个团队调用数字员工。

"团队限定 provider / employee type / runtime scope"这类白名单与真实调度路径矛盾：员工被 A 团队"允许的 provider"限制，却要在项目绑定的、属于别处的 runtime 上跑，白名单无从执行。

### 1.1 探查纠偏（关键）：团队能力已有真实归属，存在双事实源

代码探查发现团队公共能力**已经**以真实绑定表存在，`capability_policy.allowed_*` 是**冗余的第二事实源**：

| 能力 | 真实归属（现有） | 冗余白名单（要删） |
| --- | --- | --- |
| 团队技能 | `skill_team_bindings`（迁移 018，team-capabilities-tab 用 `bindTeamSkill/listTeamSkills`） | `capability_policy.allowed_skills` |
| 团队 MCP | `team_mcp_bindings` + `team_mcp_servers`（迁移 037） | `capability_policy.allowed_mcp_servers` |
| 团队宪法 | **无独立归属**，仅 `tenant_team_config_revisions.constitution` blob | — |
| 团队外部能力 | **无绑定表，也无外部能力注册表**（仅自由字符串） | `capability_policy.allowed_external_capabilities` |

而员工创建的继承却读 `capability_policy.allowed_*`（service.go:346/364/949/1909），**不读绑定表** → 团队技能/MCP 有两个事实源，会漂移。这是"团队治理"遗留的真实祸根。

被错误放在团队上的"运行范围"，其合法归宿是**项目**（计划 B）。

### 1.2 探查纠偏（关键）：团队 revision 是派发前置门禁的一半输入

`digital_employee_effective_configs`（员工"有效配置"审批）是派发/运行的强制前置：

- `run_service.go:822/860`：员工跑任务前必须 `HasApprovedEffectiveConfig`，否则 `ErrEffectiveConfigRequired` 挡住。
- `project/service.go:993-996`：项目派发闸——员工 effective config 未批准就返回 `employee_workspace_pending`，阻断派发。
- effective config 快照输入 = 团队 revision id + 员工 revision id。
- 控制台**无任何 UI 触发这道审批**（前端零引用）——一个无 UI 可满足却挡着派发的死闸，与 golden path 跑不通吻合。

**决定：一并删除这道门禁**。删除 `digital_employee_effective_configs`（合并快照 + 审批）+ `HasApprovedEffectiveConfig` 前置门 + 项目派发的 effective-config gate + 团队 `tenant_team_config_revisions`。

**保留**：`digital_employee_config_revisions` 作为员工自身配置（role_profile / constitution_addendum / capability_selection / context_policy_override / approval_policy_override）的事实源。运行时有效能力 = 员工个人配置 ∪ 团队基线（绑定表 + constitution 列），读/派发时合成，不再有审批快照。

## 2. 目标模型（A）

### 2.1 团队（Team）终态

团队公共能力 = 复用现有绑定表 + 一个新的宪法列，**不引入新的能力列/第三事实源**：

```
团队技能基线  = skill_team_bindings（重命名为 team_skill_bindings）
团队 MCP 基线 = team_mcp_bindings（不动）
团队宪法      = tenant_teams.constitution（新增 jsonb 列，{ hard_rules: string[] }）
团队身份      = tenant_teams 现有列（slug/name/display/human_owner_user_ids）
```

- 团队负责人（team owner）负责团队管理：数字员工添加、技能绑定、MCP 绑定、宪法维护。与项目人类负责人（审批、通知、任务管理）职责不同，互不混淆。
- **外部能力移出本次范围**：团队层无外部能力绑定表、平台无外部能力注册表；`allowed_external_capabilities` 及员工创建里的外部能力 plumbing 一并删除，等真正要引用外部能力时再单独设计实现。

### 2.2 命名约定（team-first）

统一团队维度绑定表命名为 `team_<subject>_bindings`：

- `team_mcp_bindings`：不动（已符合）。
- `skill_team_bindings` → **重命名** `team_skill_bindings`（迁移 rename + 更新 sqlc 查询与 Go 引用）。
- 本次不新增外部能力绑定表。

### 2.3 能力语义：继承基线（叠加，无上限）

团队的技能 / MCP 对成员是**继承基线**，不是白名单天花板：

```
员工有效技能  = team_skill_bindings(团队)      ∪ 员工个人技能绑定
员工有效 MCP   = team_mcp_bindings(团队)        ∪ 员工个人 MCP 绑定
员工有效宪法  = tenant_teams.constitution.hard_rules ++ 员工 constitution_addendum
无团队员工：团队部分为空，只保留个人部分。
```

- 团队基线对员工**纯只读继承、不可退订**：公共能力是强制基线，成员一律拥有，只能在其上叠加个人能力。
- 持久化只存员工个人追加部分；继承在读取/派发时合成。
- 团队不再对 approval、context、provider 类型、employee 类型有任何发言权。
- **员工创建的继承来源从 `capability_policy.allowed_*` 改指到 `team_skill_bindings` / `team_mcp_bindings`**，消除双事实源。

## 3. 删除与改动清单（A · 全栈）

### 3.1 DB 迁移（`apps/control-plane/internal/storage/migrations/`）

两个迁移，遵循"先加后删"以保证每步可重放、代码始终可编译可测：

- **迁移 M1（加列 + 回填 + 改名）：**
  - `tenant_teams` 增加 `constitution JSONB NOT NULL DEFAULT '{}'::jsonb`。
  - 回填：把每个团队 active `tenant_team_config_revisions.constitution` 迁到 `tenant_teams.constitution`（无 active 取 `{}`）。
  - 重命名 `skill_team_bindings` → `team_skill_bindings`。
- **迁移 M2（删旧治理 + effective-config，代码不再引用后执行）：**
  - 删除 `tenant_team_config_revisions` 表。
  - 删除 `digital_employee_effective_configs` 表（含对团队 revision 的外键）。
  - 保留 `digital_employee_config_revisions`（员工自身配置事实源）。
- 迁移必须可重放，更新 `atlas.sum`，用 `make -C apps/control-plane migrate-validate` 校验。

### 3.2 契约（`contracts/control-plane/openapi.yaml`）

- 删除 `TeamConfigRevision`、`CreateTeamConfigRevisionRequest` 及团队治理 revision 相关路径（config-revisions / governance 的 current / drafts / approve / reject / diff 全套）。
- 删除 effective-config 相关 schema 与路径（`.../effective-configs/preview`、`.../effective-configs/approve`、`.../effective-config`）；保留 `.../config-revisions`（员工自身配置）如仍需要。
- `DigitalEmployeeCreateTeamConfig` 精简为 team baseline：`{ team_id?, constitution, skills[], mcp_servers[] }`（`skills`/`mcp_servers` 来自绑定表）；删除 `allowed_employee_types`、`allowed_provider_types`、`allowed_skills`、`allowed_mcp_servers`、`allowed_external_capabilities`、六个 policy blob、`revision_number`、`status`。
- `DigitalEmployeeCapabilityOptions`：删除 `external_capabilities`；`skills`/`mcp_servers`/`provider_types` 恒为平台全量可选池。
- `DigitalEmployeeCreateOptions`：`employee_types` 恒为平台全量（不再按团队裁剪）；删除团队治理相关 `creation_checks`。
- Team schema 增加 `constitution`（`{ hard_rules: string[] }`）。
- 重新生成 gen 代码，跑契约验证。

### 3.3 Control Plane Go

- `apps/control-plane/internal/employee/service.go`：
  - 删除 `ErrEffectiveConfigRequired` 闸（约 492）、`allowedEmployeeTypesFromTeamConfig`（400）、`validateEmployeeTypeAllowedByTeamConfig`（498）、`employeeTypesForTeamConfig` 团队裁剪、`capability_policy.allowed_*` 天花板过滤（346/364/366/949/952/1909 等）、外部能力相关 `AllowedExternalCaps` / `ExternalCapabilities`。
  - `capabilityOptionsForTeamConfig` → 返回平台全量池；团队 baseline（skills/mcp）改从 `team_skill_bindings` / `team_mcp_bindings` 读取，作为只读继承展示与运行时合成来源。
- `apps/control-plane/internal/tenant`：删除 `TeamConfigRevision` 类型、draft/approve/reject/diff service 与 repository、对应 handler 与路由（server.go:347-362）；team 增加 `constitution` 读写；`skill_team_bindings` 相关 sqlc 查询与 Go 引用改名到 `team_skill_bindings`。
- `apps/control-plane/internal/employee/run_service.go`：删除 `HasApprovedEffectiveConfig` 前置门（822/860/902/1031）及 `ErrEffectiveConfigRequired` 相关 preflight；员工能力从个人配置 ∪ 团队基线合成。
- `apps/control-plane/internal/employee` effective-config：删除 `PreviewDigitalEmployeeEffectiveConfig` / `ApproveDigitalEmployeeEffectiveConfig` / `GetDigitalEmployeeEffectiveConfig`（handler.go:600/638/680）、effective-config repository 与快照构建；保留 `digital_employee_config_revisions` 相关 CRUD。
- `apps/control-plane/internal/project/service.go:993-996`：删除派发的 `EffectiveConfigStatus` gate（`employee_workspace_pending`）。
- `apps/control-plane/internal/api/server.go`：删除 config-revisions / governance 路由（347-362）与 effective-config 路由（251-254）。
- 清理各 `*.gen.go` 中被删 schema 的生成代码。

### 3.4 Web 控制台

- 删除 `apps/web/src/features/teams/components/team-governance-tab.tsx` 及 draft/approve/reject/diff API 调用；团队详情 governance tab 改为**宪法编辑 tab**（只编辑 `constitution.hard_rules`，团队负责人可编辑）。技能/MCP 继续用现有 `team-capabilities-tab.tsx`（绑定表）。
- `apps/web/src/features/employees/create.tsx`：
  - `configSteps` 从 `["身份","能力","治理","Provider 类型"]` 改为三步，删除"治理"步及团队治理版本/允许员工类型/允许 Provider 展示（1516-1531）。
  - 员工自有 approval/context 默认值并入"能力"或"身份"步。
  - 删除外部能力相关展示与 `enabled_external_capabilities` 处理。
  - 继承能力保持**只读**展示（承接现有"团队继承能力"，`inheritedCapabilitySelection` / `withoutValues` 复用），来源改为团队技能/MCP 绑定。
- `apps/web/src/lib/api/teams.ts` 等：删除 governance draft/diff API 客户端；`skill_team_bindings` 对应客户端命名如涉及一并对齐（表改名不影响前端 API 路径时可不动，实现时确认）。

## 4. 数据流与错误处理（A）

- **有效能力解析**为单一事实源：团队基线来自绑定表，持久化只存个人追加，运行时合成 `团队基线 ∪ 个人`。
- 删除 `ErrEffectiveConfigRequired` 后，团队存在即可建员工；无团队员工团队基线为空。
- 员工类型来自平台注册表 `DefaultEmployeeTypeDefinitions()`；provider 可用性来自 runtime 节点上真实注册的 provider。
- 迁移兼容：constitution 按 3.1 回填；`capability_policy.allowed_*`（含外部能力）随 revision 表删除，不迁移（技能/MCP 真实归属在绑定表，已存在）。

## 5. 测试与验证（A）

### 5.1 自动化测试

- Web（`corepack pnpm --filter ./apps/web run test`）：员工创建三步向导快照/交互；继承能力只读展示（来源绑定表）；团队详情宪法编辑 tab；无外部能力项。
- Go：员工创建不再依赖 team revision；有效能力从绑定表合成；`team_skill_bindings` 改名后查询通过；删除治理后旧路由/字段回归清理。
- 契约：重新生成 + 契约验证。

### 5.2 真实端到端验证（CLAUDE.md 强制完成条件）

迁移 → `scripts/dev-services.sh restart control-plane` → Web 真实建团队（绑定公共技能/MCP、编辑宪法）→ 建员工（验证从绑定表继承 + 平台全量类型/provider，无治理步、无外部能力）→ **派任务不再被 effective-config 审批门挡住**（验证 golden path 派发前置不再要求 approved effective config）→ 走真实 Web/接口确认结果不是 mock、缓存或旧服务。涉及迁移与前后端行为，必须确认运行中的服务已加载当前代码。

## 6. 明确排除的范围

- 项目 `team_id` 字段：现为可选锚点，仅参与 `validateProjectTeamScopeAccess` 权限校验，与跨团队 `members[]` 并存，无真冲突，本次不动。
- 项目自身的 `approval_policy` / `coordination_policy` / `evidence_policy`：属于项目/人类决策层，本次不涉及。
- 员工自有 approval/context 默认值语义：保留现状，仅调整创建向导呈现位置。
- 外部能力：团队与员工创建路径中的外部能力 plumbing 本次删除；外部能力的注册表与团队绑定留待真正要引用外部能力时单独设计。
- 项目绑定可运行节点：属计划 B，见 §7。

## 7. 计划 B — 项目绑定可运行节点

### 7.1 探查纠偏（关键）：项目已有单节点 placement

代码探查发现项目**已有** runtime 落点子系统，B 必须在其上调和而非新造派发链：

- `project_placements` 表（迁移 040）：`(tenant_id, project_id) WHERE placement_status='active'` 唯一 → **每项目仅一个 active 落点**。
- 仅能**手动** `PUT /projects/{id}/runtime-placement` 设置（`UpsertProjectPlacement` 无自动调用方）。
- predispatch gate（`predispatch_gate.go`）：无 active placement 即 `runtime.placement_missing` 阻断派发，要求 `bind_runtime`。
- 就绪度（`service.go` `GetProjectRuntimeReadiness`）：以 active placement 为准，再查节点 online/可用。

现状即"建项目 → 手动 pin 一个节点 → 才能派发"，手动 pin 是 golden path 的一处卡点。

### 7.2 目标模型：资格集（多，创建时选）+ 单 active placement（派发自动选）

```
新表 project_runtime_nodes (tenant_id, project_id, runtime_node_id, created_at)
    项目 ↔ N 个「可运行资格」节点；创建时多选写入，强制 ≥1。

CreateProjectRequest 新增 runtime_node_ids: uuid[]（required, minItems 1）。

project_placements（单 active = 当前实际落点）：语义不变，但
    - PutProjectRuntimePlacement 约束：只能选资格集(project_runtime_nodes)内的节点。
    - 派发时自动选：无 active placement 但资格集有 online 节点时，
      派发自动从资格集挑一个 online 节点 upsert placement（消除手动 pin，
      顺带解掉 placement_missing 卡点）。

predispatch gate / 就绪度：基本不动（仍看 active placement，只是现在能自动获得）。
```

### 7.3 改动清单（B）

- **DB 迁移 M3**：新增 `project_runtime_nodes` 表（FK 到 `projects(tenant_id,id)` ON DELETE CASCADE、`runtime_nodes(node_id 或 id)`）。
- **契约**：`CreateProjectRequest` 加 `runtime_node_ids: uuid[]`（required, minItems 1）；如需读取资格集，加 `GET /projects/{projectId}/runtime-nodes`。重生成。
- **Go project**：`CreateProject` 校验 `runtime_node_ids` 非空、同租户、节点存在，插入 `project_runtime_nodes`；`PutProjectRuntimePlacement` 增加资格集约束；派发路径（predispatch 或 dispatch 入口）在无 active placement 时从资格集自动选 online 节点 upsert placement。
- **Web**：项目创建向导（`create-project` 系列）在 policies 步前/后加「可运行节点」多选步或分区，复用 `listRuntimeNodes`，强制 ≥1；`create-project-draft.ts` 加 `runtimeNodeIds`；提交映射到 `runtime_node_ids`。
- **测试 + 真实 e2e**：建项目多选 ≥1 节点 → 不手动 pin 直接派发 → 派发自动从资格集选 online 节点、不再 `placement_missing`。

### 7.4 B 明确排除

- 不改 `project_placements` 单 active 语义（不做多 active）。
- 不引入项目级 runtime_scope_policy blob（用显式资格集表取代）。
