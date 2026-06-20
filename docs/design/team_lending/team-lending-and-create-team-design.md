# 团队借调模型 + 新建团队页（方案 B）落地设计

> 状态：决策已确认（2026-06-20）。D1=团队级 (project, team)；D2=含超纲强制转人工；D3=请求自带 status + inbox/audit；D4=先前端建团队页再借调后端。
> 关联原型：`docs/prototypes/create-team-concept-b-lifecycle-console.html`
> 关联既有结构：`user_project_team_scopes`(021)、`tenant_teams`(001)、`digital_employees.team_id`、`approval_*`、最新迁移 `024`

## 0. 背景与目标

把"新建团队"从两步抽屉（`CreateTeamDrawer`）改为**全页生命周期创建台**（方案 B），同时把团队的核心对外交互——**数字员工借调**——建模为一等对象。

职责切分（已与负责人确认）：

| | 团队负责人（供给侧） | 项目负责人（需求侧） |
| --- | --- | --- |
| 管 | 团队成员、治理外壳、能力绑定、**借调策略** | 项目目标、任务、预算、验收 |
| 审批 | **借调授权**、能力/治理边界例外 | 业务高风险动作、驳回/补证、验收结论 |

## 1. 领域模型

借调分两层，对应"预授权 + 例外审批"：

### 1.1 借调策略 `team_lending_policy`（供给侧预授权，每团队一条 active）

团队负责人设置一次：本团队员工**是否 / 在什么条件下**可被项目调用。

- `allow_lending BOOLEAN`：是否允许被借调。
- `approval_mode VARCHAR`：`auto` | `manual`。auto = 符合策略自动放行；manual = 每次借调需团队负责人审批。
- `budget_ceiling NUMERIC`：单次借调预算上限（NUMERIC，按 DATABASE_DESIGN.md 金额规则）。
- `capability_ceiling JSONB`：可被借调时允许的能力/runtime scope 天花板（不可超出团队治理外壳）。
- `project_match JSONB`：可被哪些项目调用的匹配条件（标签 / owner 范围；registry-first，不写死枚举）。
- 超出 `budget_ceiling` / `capability_ceiling` 的请求 = **例外**，即使 `approval_mode=auto` 也强制转人工。

### 1.2 借调请求 `team_lending_request`（需求侧发起 → 供给侧裁决）

项目要把某团队的数字员工纳入项目池时产生。

- 粒度：**(project_id, team_id)** 一次授权 = "项目 P 可在策略约束下使用团队 T 的员工"；具体员工由项目协调线程在授权范围内挑选。（见 D1）
- `status VARCHAR`：`pending` | `auto_approved` | `approved` | `rejected` | `revoked`。
- `requested_by_user_id`：项目负责人。
- `decided_by_user_id` / `decided_at` / `decision_reason`：团队负责人裁决。
- `granted_budget NUMERIC` / `granted_capability JSONB`：本次借调的实际额度（≤ 策略天花板）。
- `is_exception BOOLEAN`：是否因超纲转人工。

### 1.3 与 `user_project_team_scopes` 的关系

- `user_project_team_scopes` = **资格门**：管理员授予"某人建项目时**可选**哪些团队"。粗粒度、长期、与具体项目无关。
- `team_lending_request` = **交易**：具体项目实际借调具体团队员工，受借调策略约束，可能需团队负责人审批。
- 两者互补、不替代。借调请求发起的前置校验仍复用 `CanUseTeamForProject`（资格）→ 再走借调策略（条件/额度/审批）。

## 2. 数据库（migration 025，遵循 DATABASE_DESIGN.md）

- 模块前缀：团队属于 `tenant_teams`，借调是团队级新边界 → 建议新前缀 **`team_lending_*`**（待确认是否并入既有前缀）。
- UUID-first / tenant-first / team-aware：两表均含 `id UUID PK`、`tenant_id`、`team_id`。
- `team_lending_policy`：`uq (tenant_id, team_id) WHERE status='active'`；索引 `(tenant_id, team_id)`。
- `team_lending_request`：索引 `(tenant_id, team_id, status)`、`(tenant_id, project_id, status)`；`uq (tenant_id, project_id, team_id) WHERE status IN ('pending','auto_approved','approved')`（同一项目对同一团队不重复有效借调）。
- 状态用 VARCHAR + 应用层校验，仅对稳定基础状态加 `CHECK`；FK 谨慎（application-controlled first）。
- 全表/字段中文 `COMMENT`；`created_at/updated_at` + `update_updated_at_column` 触发器。
- sqlc 查询放 `internal/storage/queries/team_lending.sql`。

## 3. 契约 / 控制面

- OpenAPI 新增（`contracts/`，改后走生成+契约验证）：
  - `GET/PUT /api/v1/teams/{teamId}/lending-policy`
  - `GET /api/v1/teams/{teamId}/lending-requests`、`POST .../approve`、`POST .../reject`
  - `POST /api/v1/projects/{projectId}/lending-requests`（需求侧发起）
- 控制面新增 `internal/teamlending`（service + handler + repo），审计写 `audit_*`，人工审批项进团队负责人 inbox（复用 `inbox_items`/`approval_*`，见 D3）。

## 4. 前端（方案 B 落地）

### 4.1 新建团队全页路由（本轮主交付，仅依赖既有 teams 接口）

- 新增路由 `apps/web/src/routes/_authenticated/teams/new.tsx` → `CreateTeamPage`。
- `apps/web/src/features/teams/index.tsx` 的「新建团队」按钮由 `setCreateOpen(true)` 改为 `navigate({ to: '/teams/new' })`。
- **删除 `CreateTeamDrawer`** 及其测试引用（已确认）；下沉可复用件：`UserSearchSelect`、成员选择逻辑（从 `create-team-members-step` 提取）、`TeamIconTile`、draft→`CreateTeamInput` 映射。
- 布局照方案 B：左栏 身份/负责人/初始成员 三卡，右栏 sticky 预览卡 + 生命周期清单 + 提交区。
- 生命周期右栏中 治理/借调/数字员工/能力/审计 均为**信息行**（创建期不配置）：治理、借调显示 `待配置/待设置` 并在创建成功后可跳对应配置页。
- 负责人卡文案：团队管理者语义（成员增删+配置），审批/验收归项目负责人。
- 提交成功 → 跳团队详情（勾选"创建后前往治理配置"则进治理 tab）。
- 状态覆盖：loading/empty/error/permission/disabled + slug 校验。

### 4.2 借调策略/请求 surface（本轮第二交付，依赖 §2/§3）

- 团队详情新增「借调」tab：编辑借调策略 + 待处理借调请求审批列表。
- 复用现有 `LiquidTabsList`、`StatusBadge` 等设计组件，不另造样式。

## 5. 待确认决策（动数据库前必须定）

- **D1 借调粒度**：(project, team) 团队级授权（推荐，贴合现有 team-scope 粗粒度 + 协调线程挑人） vs (project, digital_employee) 逐员工。
- **D2 审批逻辑范围**：v1 只做 `auto/manual` 开关 vs v1 即含"超纲强制转人工"的例外逻辑（推荐含，因为这是团队负责人价值核心）。
- **D3 审批载体**：借调请求自带 status + 写 inbox/audit（推荐，轻） vs 接入 `approval_*` 通用审批模块。
- **D4 借调策略入口**：创建后在团队详情配置（推荐，创建页保持 `待设置` 信息行） vs 创建页内嵌策略表单。

## 6. 落地顺序（建议）

1. 本设计 + D1–D4 确认。
2. 前端：方案 B 新建团队全页路由（不依赖借调后端，可独立先上 + 真实 E2E）。
3. 后端：migration 025 + sqlc + service/handler + 契约生成验证。
4. 前端：团队详情「借调」tab。
5. 全链路真实 E2E（建团队→设借调策略→项目发起借调→审批→协调线程取员工）。
