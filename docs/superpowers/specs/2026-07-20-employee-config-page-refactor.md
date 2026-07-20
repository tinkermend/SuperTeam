# 数字员工配置页重构 Spec

- 日期：2026-07-20
- 状态：草案，决策已锁定，可开工（审批闭环部分依赖并行的权限中心 spec，见 §9/§11）
- 目标读者：接手实施本 spec 的会话（本文档自包含）
- 姊妹 spec：`docs/superpowers/specs/2026-07-20-permission-center-refactor.md`（权限中心域，另一会话**已在开发**）。两者唯一硬接缝见 §9 与 §11。

---

## 1. 背景与问题

数字员工「配置页」`/employees/{id}/config`（`apps/web/src/features/employees/config.tsx`）现状与问题（实证自代码）：

- **名不副实**：标题"数字员工配置"，实际只编辑 **3 张卡**——人格记忆.md、能力绑定（**裸 JSON**）、每日 Token 预算。它严格镜像后端修订契约 `CreateDigitalEmployeeConfigRevisionRequest`（只收 `persona_memory_markdown / capability_bindings / budget_policy / status`）。用户心智里"配置一个员工"包含的角色、Provider、权限、技能、MCP、环境变量，几乎一个都不在此。
- **入口与内容错位**：主入口是详情页「生效上下文」卡的「编辑」（`effective-context-panel.tsx:37`），那张卡展示 Provider / 角色 / 技能 / MCP / 环境变量，用户点"编辑"期待改这些，落地却是三个几乎不重叠的字段。
- **裸 JSON**：`能力绑定` 让用户手写 `{"external_capabilities":[...],"environment_variable_refs":[...]}`，打错报"必须是有效 JSON object"（`config.tsx:88-104`）。违背平台"中文优先、不裸露技术细节"。
- **修订/审批语义完全不可见且实际是死路**：保存 → 创建 `draft` 修订，文案"批准后生效"（`config.tsx:192`）。但（见 §3.3）**后端没有任何 approve/activate 端点，草案永远转不正**——当前这个页面存下的东西根本不生效。
- **保存反馈误导**：成功显示"配置已保存"（`config.tsx:202`），实则创建了一个永不生效的草案。
- **无定位锚点 + 视觉断层**：页面除名字外不显示在配置谁；用朴素 `Card`，与详情页 `SoftCard`/`IconTile` 设计系统断层。
- **能力管理割裂在别处**：技能 / MCP / 环境变量的真正编辑在**详情页的 Sheet 抽屉**（`EmployeeCapabilitiesPanel`，约 800 行），入口是详情页头部「管理技能与 MCP」按钮（`employee-detail-header.tsx:84`）+ 生效上下文卡两个 ghost 按钮。配置页和详情页 Sheet 形成"双脑分裂"。

---

## 2. 目标与非目标

### 目标
1. 把配置页重构为**一个连贯的三层配置面**（§4），承载员工的全部可配置项，纠正"编辑"入口与内容的错位。
2. **技能 / MCP / 环境变量搬进配置页统一承载**，详情页退化为只读观测；删除详情页的「管理技能与 MCP」Sheet 与相关编辑按钮。
3. **裸 JSON → 结构化选择器**。
4. **让 role / permission_policy 创建后可改**，走**审批版本化**（A2 方案：草案 → 权限中心审批 → 激活写回员工行）。
5. **permission_policy 从死字段变为真强制（P-enforce）**（§5）。
6. 修订/审批语义可见；补齐 draft→active 的激活链路（`ActivateConfigRevision`）。

### 非目标
- **权限中心域本身**（页面、审批模型、路由、approver 路由）：属姊妹 spec，本 spec 只**产生**审批请求并实现**激活回调**。
- **Provider 变更**：Provider **创建后不可改**（绑定执行实例/Runtime 亲和/家目录，锁死是有意的）——定位头只读呈现。
- **敏感能力 / MCP 授权是否也需审批**：v1 **不纳入**权限审批层（仅 role + permission_policy 走审批）；能力/MCP 分级授权列 §12 开放决策，后续 spec。
- 名称 / 头像 / employee_type 变更：不在本 spec。

---

## 3. 当前状态（实证，供实施者定位）

### 3.1 前端
- 配置页：`apps/web/src/features/employees/config.tsx`（`EmployeeConfigView`，3 卡 + `createDigitalEmployeeConfigRevision`）。
- 路由：`apps/web/src/routes/_authenticated/employees/$employeeId/config.tsx`；父布局 `.../employees/$employeeId.tsx`（pathname 以 `/config` 结尾则渲染 `<Outlet>`，否则渲染只读 `EmployeeDetailPage`）。
- 详情页：`apps/web/src/features/employees/detail.tsx`（`EmployeeDetailView`）。关键子件：
  - `components/effective-context-panel.tsx`（生效上下文，含指向 config 的「编辑」链 `:37`）。
  - `components/employee-capabilities-panel.tsx`（**要搬进配置页的主体**，约 800 行：个人技能 + 技能市场 / 个人 MCP 绑定 + 凭据环境变量 + skill→MCP 依赖预检 / 生效 MCP 注册表 / 环境变量增删）。
  - `components/employee-detail-header.tsx`（`:82-84` 「管理技能与 MCP」outline 按钮，`onManageCapabilities`）。
  - `detail.tsx:398-407` 挂 Sheet + `EmployeeCapabilitiesPanel`；`detail.tsx:530` `EmployeeConfigSnapshotSection`（只读 persona/budget）。
- 数据模型 / API：`apps/web/src/lib/api/employees.ts`
  - `DigitalEmployee`（`:28`）：`role`、`provider_type`、`permission_policy: Record<string,unknown>`、`persona_memory_markdown`、`capability_bindings`、`budget_policy`、`risk_level`、`status`、`team_id`、`operational_state`、`metadata.effective_config_status`(approved/draft/stale/missing)。
  - `CapabilityBindings`（`:583`）：`skills?`/`mcp_servers?`（已废弃、服务端剥离）、`external_capabilities?`、`environment_variable_refs?`。
  - `BudgetPolicy`：`daily_token_limit?`。
  - `CreateDigitalEmployeeConfigRevisionInput`（`:649`）：`persona_memory_markdown? / capability_bindings? / budget_policy? / status?`。
  - `DigitalEmployeeConfigRevision`（`:632`）：`revision_number`、`status: draft|active|archived`、`approved_by?`、`approved_at?`。

### 3.2 后端（关键约束）
- 员工根 `/api/v1/digital-employees/{id}` **只有 GET / DELETE**（`contracts/control-plane/openapi.yaml:3402-3433`）——**无 PUT/PATCH**，role/permission/provider 创建后无接口可改。
- 配置修订只有 `POST /config-revisions`（`server.go:344`）；`CreateConfigRevision` **拒绝非 draft**（`apps/control-plane/internal/employee/service.go:1187-1193`）；**无 activate/approve 端点**。
- 员工生效配置读取：`attachLatestConfigRevision`（`service.go`），初始 active 修订由 `createInitialActiveConfigRevision`（`service.go:876-905`）内联生成。
- role / permission_policy 是**员工行上的普通列**（`pg_repository.go:65/1976`），由 executor 匹配 / 规划器直接读员工行。
- 状态/团队变更走 `/status`、`/team`。技能/MCP/环境变量走 `/skills`、`/mcp-bindings-v2`、`/environment-variables`（各自即时端点）。

### 3.3 permission_policy 现状：死字段
- 默认 `{}`，创建时按请求原样存、四处 clone、API 回显。
- **无任何强制点**：authz / runtime / 工具门禁都不读它。
- 唯一读者：规划器 `buildPlanningPermissions` 读 `grants` 键（`apps/control-plane/internal/workflow/projectcoordination/planning_profile.go:319-320`），且**咨询式打分、显式永不硬失败**（同文件 `:459-467` 注释）。
- 运行时**已强制** `allowed_actions`，但它**只来自每次运行请求体**（`run_handler.go:50/74` → `run_types.go:179`），**不来自员工**——没有 `employee.PermissionPolicy → allowed_actions` 的映射。这是 P-enforce 的关键接入点。
- 兄弟字段 `session_policy` / `workspace_policy` 是真被 runtime 消费的（小而定型对象），可作 permission_policy 定型的一致性参照；`approval_policy` 已被掏空。

---

## 4. 目标信息架构：三层配置面

配置页重构为**定位头 + 三层区块**，视觉迁到与详情页一致的 `SoftCard`/`IconTile`/`StatusPill` 基元，`Main width="contained"`。

### 定位头（只读）
名称 (id)、**Provider（只读，不可改）**、当前状态 Pill、风险等级、所属团队、当前生效配置版本 label + `effective_config_status`（approved/draft/stale/missing）。让用户知道在配置谁、当前生效什么版本。

### 层一 · 即时生效（无需审批）
提交即生效，各有其机制：
- **技能 / MCP / 环境变量**：把 `EmployeeCapabilitiesPanel` 从详情页 Sheet **搬进本层**，沿用其现成即时端点（`/skills`、`/mcp-bindings-v2`、`/environment-variables`），保留技能市场、个人 MCP 凭据、skill→MCP 依赖预检、生效 MCP 注册表等既有能力。
- **人格记忆.md**：改 Markdown 编辑 + 预览 + 模板提示（保留项目宪法注入说明：宪法按项目注入，不属员工配置）。
- **预算策略**：每日 Token 上限数字输入（沿用现有校验）。
- persona/预算的写入 → 走**自动激活修订**（§6.2），保留版本历史但即时生效、不需审批。

### 层二 · 权限审批（需权限中心审批，A2）
仅承载**权限项**——`role`、`permission_policy`（结构化，§5）。
- 编辑即生成**草案修订** + **产生 `category=permission` 审批请求**投递权限中心（§9）。
- 区块内显式呈现：**当前生效 vs 草案 diff**、待批准草案状态、"提交后进入权限中心审批、批准后生效"、跳权限中心链接。
- **角色变更遇在役项目 → 提交即拒**（§6.4）。

> external_capabilities（外部能力授权）v1 暂留即时层或维持现状；其"是否属权限扩张需审批"见 §12 开放决策，不阻塞 v1。

### 详情页（退化为只读观测）
删除「管理技能与 MCP」按钮（`employee-detail-header.tsx:84`）、生效上下文卡的两个 ghost 编辑按钮、Sheet + `EmployeeCapabilitiesPanel` 挂载（`detail.tsx:398-407`）。详情页保留：只读生效上下文、调度就绪、运行历史、删除。所有编辑入口指向配置页。

---

## 5. permission_policy 定型 + P-enforce

把 permission_policy 从自由 `map[string]any` 收紧为**定型对象**（与 `session_policy` 一致的小而定型风格），每个键对应真实强制点：

```jsonc
permission_policy: {
  // 1) 资源授权，"scope:resource" 形式；沿用规划器现有读者，正式化为定型键
  //    例: ["database.read:dev_db", "artifact.write:project"]
  "grants": ["<scope>:<resource>"],

  // 2) 员工级动作白名单；派发时并入 run payload 的 allowed_actions（runtime 已强制）
  //    留空 = 不额外收敛，走 provider 沙箱默认；例: ["code.write","shell.exec"]
  "allowed_actions": ["<action>"]
}
```

- **强制接入点（必做，本 spec 的 P-enforce 核心）**：在运行派发构建 payload 处（`apps/control-plane/internal/employee/run_service.go` ~1440-1533，与 `allowed_actions`/`workspace_policy`/`session_policy` 同段），把 `employee.permission_policy.allowed_actions` **并入** run payload 的 `allowed_actions`（与运行请求体的 allowed_actions 取交集或并集——语义见 §12 决策 E3）。runtime 已强制 `allowed_actions`，借此让 permission_policy 立即真生效。
- `grants`：保留规划器 `buildPlanningPermissions` 读取；正式化键名与格式（`scope:resource`）。
- **契约**：`permission_policy` 从 `additionalProperties:true` 收紧为定型 object（`grants: string[]`、`allowed_actions: string[]`），保留对未知键的宽容读以兼容存量 `{}`。
- **前端**：结构化编辑器——`grants` 与 `allowed_actions` 用 chips / 多选，彻底告别裸 JSON。
- 可选第三键 `approval_required_above_risk`（高于此风险的动作强制人工）接现有 `risk.approval` / 预派发闸——**列 §12 决策 E4，v1 可不做**，因为闸集成成本独立且不阻塞前两键的真实强制。

---

## 6. 后端契约与领域变更

改契约后走 `generate:control-plane` + 契约验证；迁移入 `apps/control-plane/internal/storage/migrations/`，更新 `atlas.sum`，`make -C apps/control-plane migrate-validate`，遵循 `DATABASE_DESIGN.md`。**与权限中心会话协调迁移编号防撞号**（§11）。

### 6.1 配置修订扩 role / permission
- `CreateDigitalEmployeeConfigRevisionRequest` 增可选 `role`（string）、`permission_policy`（定型 object，§5）。
- 修订存储表加 `role`、`permission_policy` 列（迁移 + sqlc）。

### 6.2 激活链路 `ActivateConfigRevision`（本 spec 拥有，权限中心 apply 调用）
- 新增域方法 `ActivateConfigRevision(ctx, revisionID)`：draft→active + 把该修订的 role/permission **写回员工行**（A2）+ 落审计。**幂等**。
- 放开 `CreateConfigRevision` 只准 draft 的限制，改为：
  - 修订**只含即时字段**（persona/budget/external_cap，无 role/permission 变更）→ 创建后**立即调用 `ActivateConfigRevision` 自动激活**（无需审批，保留历史）。
  - 修订**含 role/permission 变更** → 状态 `draft` + 产生审批请求（§9），**仅在权限中心批准时**由其调用 `ActivateConfigRevision` 激活。
- 是否暴露 HTTP `POST /config-revisions/{revisionId}/activate` 还是仅 Go 域方法：§12 决策 E2（推荐仅域方法内部调用，避免绕过审批的外部激活入口）。

### 6.3 permission_policy 强制（§5）
- 派发 payload 注入 `allowed_actions`；契约收紧；规划器 `grants` 读取保留。

### 6.4 角色变更「提交即拒」护栏（产生侧）
- 提交含 role 变更的修订时，校验该员工是否有**在役项目占用 / role_independence(SoD)** 冲突（新角色是否仍满足其所在项目的执行池约束）。冲突 → **拒绝创建修订与审批请求**（返结构化错误码，前端提示），不让脏请求进权限中心。
- 复用现有 executor 匹配 / role_independence 判据逻辑（在 project / projectcoordination 中）。

---

## 7. 前端重构清单

1. `config.tsx` 重写为定位头 + 三层（§4）；`Card` → `SoftCard` 等设计系统基元。
2. 把 `EmployeeCapabilitiesPanel` 从详情页 Sheet 迁入配置页层一（组件复用，改挂载位置与容器）。
3. 详情页删按钮 + Sheet（§4 末），退化只读。
4. 裸 JSON `capability_bindings` → 结构化：`external_capabilities` 注册表多选、`environment_variable_refs` chips（能力注册表数据源见 capability API）。
5. permission_policy 结构化编辑器（grants / allowed_actions chips）。
6. 人格记忆 → Markdown 编辑 + 预览 + 模板提示。
7. 层二加**修订可见性**：当前生效 vs 草案 diff、草案状态、跳权限中心链接。
8. **保存语义修正**：层一 → "保存并生效"，成功"已生效"；层二 → "提交权限变更"，成功"已提交，待权限中心审批" + 「去权限中心」链接。
9. 加 unsaved-changes 离开拦截。
10. 状态/枚举一律经 `apps/web/src/lib/status-labels.ts`（补键，过 `status-labels.guard.test.ts`）；对象指称"名称 (id)"，Web 内跳转用 TanStack `Link`/`navigate`。
11. 页面/样式改动前读 `DESIGN.md`；改设计系统/原型后跑 `verify:design-system` / `verify:design-prototypes`。

---

## 8. 字段 → 机制总表

| 字段 | 层 | 生效机制 |
|---|---|---|
| 名称 / Provider / 状态 / 风险 / 团队 / 生效版本 | 定位头 | 只读（Provider 不可改）|
| 技能 / MCP / 环境变量 | 层一 即时 | 各自现成绑定端点 |
| 人格记忆 / 预算 / external_capabilities | 层一 即时 | 自动激活修订（§6.2）|
| **role** | 层二 审批 | 草案 → 权限中心审批 → 激活写回（A2）|
| **permission_policy** | 层二 审批 | 同上；并 P-enforce 强制（§5）|

---

## 9. 审批接缝（依赖权限中心，唯一耦合点）

- **产生**（本 spec）：层二提交 role/permission 变更 → 创建 draft 修订 + 创建 `category=permission` 的 `ApprovalRequest`，`resource_type=digital_employee_config_revision`，`ContextPayload` 含：员工 id/名称、role 与 permission_policy 的**当前 vs 变更后 diff**、风险等级、申请人、目标修订 id。**payload 形状以权限中心 spec §4.4/§7-S1 为准**。
- **消费/批准**（权限中心 spec）：approver 在权限中心处理；批准时调用本 spec 的 `ActivateConfigRevision(revisionID)`。
- **接缝契约（两会话唯一硬接口）**：① `category=permission` 审批请求 payload 形状；② `ActivateConfigRevision` 语义。二者已在权限中心 spec 写死，本 spec 遵从。
- **依赖 `ApprovalRequest.category` 字段**：由**权限中心会话先落**（其 spec §5.1）。本会话建到"创建审批请求"时若该字段尚未合入，用桩件/接口对齐先行开发（见 §10 阶段 4）。

---

## 10. 分阶段实施顺序（权限中心无关的先行、可独立真实验证）

- **阶段 1（纯前端，零后端依赖）**：定位头 + 三层骨架、`SoftCard` 迁移、Markdown 编辑、裸 JSON → 结构化选择器、保存文案与 unsaved 拦截。真实渲染验证。
- **阶段 2（能力合并）**：`EmployeeCapabilitiesPanel` 迁入配置页、详情页删按钮/Sheet 退化只读。真实走技能/MCP/环境变量增删。
- **阶段 3（permission_policy P-enforce）**：契约收紧 + 派发注入 allowed_actions + 前端结构化编辑器。真实 smoke：设 allowed_actions → 派发 → runtime 生效。
- **阶段 4（role/permission 治理，含接缝）**：修订扩 role/permission + `ActivateConfigRevision` + 自动激活/草案分支 + 提交即拒护栏 + 产生审批请求。**其端到端审批闭环验证需权限中心就绪** → 标记「集成待权限中心」，联合补 E2E。

阶段 1–3 可独立完成并交付；阶段 4 的产生侧/激活侧可先行，审批闭环 E2E 与权限中心联调。

---

## 11. 并发共享 checkout 协调（务必与权限中心会话同步）

两会话会同时碰：`contracts/control-plane/openapi.yaml`、`ApprovalRequest` schema（`category`）、生成物、`status-labels.ts`、**迁移编号**。按项目铁律：**只 `git add <显式路径>`、禁 `git add -A`/`git add .`**；**禁在共享工作树切/删分支**；提交前 `git symbolic-ref HEAD` 复核当前分支；同一文件交织改动只暂存自己的 hunk（`git apply --cached`），无法干净切分则用独立 worktree 隔离。**迁移编号与 `ApprovalRequest.category` 落点归属需提前与权限中心会话约定**。

---

## 12. 开放决策

- **E1 external_capabilities/敏感 MCP 授权是否需审批**：~~v1 归即时层~~ **【2026-07-20 已定：v1 归即时层；能力分级授权（哪些算"权限扩张"需权限中心）留后续 spec。】**
- **E2 激活端点形态**：`ActivateConfigRevision` 仅内部域方法（推荐，无外部绕审批入口）vs 暴露 HTTP。
- **E3 allowed_actions 合并语义**：~~取交集还是并集~~ **【2026-07-20 已定：取交集——员工 permission_policy.allowed_actions 是上限，与运行请求 allowed_actions 取交集；员工留空表示不额外收敛。】**
- **E4 approval_required_above_risk 第三键**：本期做还是预留（倾向预留，不阻塞前两键强制）。
- **E5 persona/budget 自动激活修订 vs 直改员工快照**：倾向自动激活修订（保留历史，复用激活机制）。

---

## 13. 验收标准与端到端验证（真实链路）

按宪法「真实端到端验证是默认完成条件」：

1. **配置页三层可视 + Provider 只读**：真实渲染，定位头显示生效版本，Provider 不可改。
2. **技能/MCP/环境变量在配置页增删即时生效**，详情页无编辑入口（Sheet/按钮已删）、纯只读。
3. **persona/预算保存即生效**（自动激活修订，员工生效配置更新），无需审批。
4. **permission_policy P-enforce**：设 `allowed_actions` → 真实派发 → runtime 收到并按之约束。
5. **role/permission 治理闭环**（阶段 4，**集成待权限中心**）：层二改 role → 产生审批请求 → 权限中心批准 → 修订激活 + 员工行 role/permission 更新 + 审计。
6. **角色变更遇在役项目 → 提交即拒**：真实构造在役占用，提交被拒、权限中心无脏请求。
7. `verify:foundation` / `verify:web` / `verify:control-plane` / `verify:design-system` 通过；`status-labels.guard.test.ts` 通过。Web 仿真用 codex chrome plug；执行实例造数按既有环境事实（直插 DB）。
8. 收尾走 `superteam-completion-check` 门禁（`.codex/skills/superteam-completion-check/SKILL.md`）。
9. 阶段 5（审批闭环）无法在权限中心就绪前真实验证的，按宪法**标记阻塞**，不以未验证状态交付。
