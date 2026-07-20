# 权限中心重构 Spec（审批中心 → 权限中心）

- 日期：2026-07-20
- 状态：**已确认开放决策，可开工**（人类拍板见 §10 结论行；§6/§0 已按结论修订）
- 目标读者：接手实施本 spec 的独立会话（不共享本次对话上下文，本文档需自包含）
- 交付性质：平台域级重构。**本 spec 只负责「权限中心」这个域**；数字员工配置页（config.tsx）重构是**另一份 spec**（由另一会话并行进行），两者的接缝在 §7、§8 明确划清。

---

## 1. 背景与问题

平台把两类语义不同的「待人类处理事项」混成了一摊：

- **收件箱（Inbox）**：面向**任务 / 项目任务的审批与验收**——plan_review、demand_acceptance、project_acceptance、task_failure_recovery 等项目协调决策。这是它应有的职责，**保持不变**。
- **审批中心（当前实现）**：**并不是**一个独立域，而是**收件箱之上的一层薄过滤视图**——硬过滤到 `item_type: "project_decision"`，复用收件箱的弹窗和动作执行插件。它贴着"审批中心"的标签，干的却是项目决策的活，**与收件箱重复**。

**正确的域定义（本 spec 的立柱）**：

> **权限中心 = 平台「操作权限审批」域**：管理这个平台上「谁被允许做什么 / 访问什么」的权限授予与变更审批，与「任务验收」（收件箱）彻底分离。它更贴切的名字就是**权限中心**，不叫审批中心。

平台**已经零散长出了权限审批的原语，但没有一个域把它们收拢**（实证见 §3）：
- `TeamMemberRole` 枚举含 **`approver`** 角色（专职审批人）。
- `AllowedTeamAction` 枚举含 **`team.governance.approve`**、**`team.member.request_privileged_role`**、`team.governance.edit`——这些是权限/治理审批动作，不是任务验收。
- `GovernanceSummaryStatus`：`draft_pending → active`——治理配置本身就有"待批准 → 生效"的生命周期，却没有承载这个批准动作的域。
- 数字员工「配置修订」有 `draft/active/archived` 状态与 `ApprovedBy/ApprovedAt` 字段，但**没有任何 approve/activate 端点**，草案永远转不正（详见 §3.4）。

**结论**：权限中心不是从零发明，而是把这些散落的权限审批需求**归拢成一个域并补齐缺失的审批链路**，同时把项目决策**还给收件箱**。

---

## 2. 目标与非目标

### 目标
1. 把「审批中心」重构 / 更名为「权限中心」，定义为**平台操作权限审批域**，与收件箱（任务验收）分离。
2. 建立一个能承载**多种权限审批主体**的模型，审批路由到 **approver 角色**成员（替换当前硬编码的单一 `TargetUserID`）。
3. 首批接入主体（§7）：
   - **S1**：数字员工「治理配置修订」审批与激活（其中含 role / permission_policy 等权限项）——这是权限中心的**第一个真实消费者**。
   - **S2**：团队**特权角色申请**（`team.member.request_privileged_role`，已有 action、当前无审批落点）。
4. 把项目决策类事项从权限中心**移除**（回归收件箱唯一呈现）。

### 非目标（明确划走，避免两 spec 越界）
- **数字员工配置页 UI 重构**（三层结构、裸 JSON→结构化选择器、定位头等）：属**员工配置 spec**，本会话另做。
- **完整审批策略引擎**（多签 / 阈值 / 法定人数 / 升级路由）：v1 只做「approver 角色成员 + 兜底人」的单人审批路由，复杂策略留后续 spec。
- **permission_policy 字段的强制执行**（P-enforce 的运行时注入）：属员工配置 spec。本 spec 只定义 permission_policy 变更**如何作为审批主体进出权限中心**。
- **收件箱自身的改动**：除了确认项目决策仍在收件箱可见外，不动收件箱。

---

## 3. 当前状态（实证，供实施者定位）

### 3.1 前端
- 权限中心页：`apps/web/src/features/approvals/index.tsx`
  - 硬过滤 `item_type: "project_decision"`、`view: "mine"`（`index.tsx:72-87`）。
  - 直接从收件箱借组件：`InboxActionDialog`、`formatContext`、`resolveInboxHref`、`riskTone`、`riskLabel`（`index.tsx:31-37`），成功后连带失效 `inbox-items` / `inbox-badge` 查询键（`index.tsx:116-117`）。
  - 头部：`title="审批中心"` / `subtitle="聚合项目决策和审批事项…"`（`index.tsx:143-144`）；队列说明"默认展示项目决策事项；详情和处理动作复用收件箱能力"（`index.tsx:186-188`）。
  - 行内动作词表：`formatApprovalActionLabel`（同意 / 驳回 / 要求补证，`index.tsx:313-318`）；"查看项目审批"深链（`index.tsx:298`）。
- 收件箱页：`apps/web/src/features/inbox/index.tsx`（暴露 `approval` + `project_decision` 两类、`mine`/`team` 视图）。
- 路由：`apps/web/src/routes/_authenticated/approvals/index.tsx`、`.../inbox/index.tsx`。
- API 封装：`apps/web/src/lib/api/inbox.ts`（`listInboxItems` / `executeInboxAction` / `InboxItem` / `InboxListFilters`）。

### 3.2 后端审批域
- 通用审批实体 `ApprovalRequest`：`apps/control-plane/internal/approval/types.go:49-67`
  - 关键字段：`ResourceType string`、`ResourceID uuid`、`RequesterType/RequesterID`、`TargetUserID`（**单一审批人，硬选**）、`DecisionType string`、`Title`、`Summary`、`RiskLevel`、`Status`、`Options []any`、`ContextPayload map[string]any`。
  - `ResourceType` / `DecisionType` 是**自由字符串** → 存储层**已经足够通用**，可承载新主体。
- 状态枚举 `ApprovalStatus`（`types.go:17-25`）：`pending / approved / rejected / needs_more_evidence / cancelled`。
- 决策枚举 `ApprovalDecision`（`types.go:27-47`）：`approved / rejected / needs_more_evidence / request_changes / restaffed / exempted`；`validDecision` / `statusFromDecision` 是**封闭 switch**（`approval/service.go:112-130`）——加新决策动词要改这里。

### 3.3 收件箱是多域投影（关键约束）
- `/approvals` 与 `/inbox` **读同一份 `inbox_items`**，只是过滤不同。
- `inbox_items` 由**投影适配器**从多个域汇聚：`ApprovalProjectorAdapter`（approval_request → `item_type: approval`）、`DecisionProjectorAdapter`（项目决策 → `item_type: project_decision`）（`apps/control-plane/internal/inbox/adapters.go:22-108`）。
- `ItemType` 枚举封闭：`approval / project_decision / team_pending_delete`（`inbox/types.go:37-42`）；`SourceType` 封闭：`approval_request / project_decision_request / team_pending_delete`（`types.go:46-50`）。
- 动作执行是**固定两适配器注册**：`ApprovalActionAdapter` + `ProjectDecisionActionAdapter`（`inbox/adapters.go:110-164`）。**新主体不能自助注册**，需新写 Go 适配器 + 加枚举 + 在 `apps/control-plane/internal/app/app.go:464-467, 641` 接线。

### 3.4 数字员工配置修订审批：完全缺失（本 spec 要补的核心链路）
- 只有创建入口 `POST /api/v1/digital-employees/{employeeId}/config-revisions`（`apps/control-plane/internal/api/server.go:344`）。
- `CreateConfigRevision` **主动拒绝非 draft 状态**（`apps/control-plane/internal/employee/service.go:1187-1193`）。
- **没有 approve / activate / 状态流转端点**；`GetLatestConfigRevision` + `attachLatestConfigRevision` 是员工生效配置的读取路径。
- 唯一产生 `active` 修订的地方是创建员工时的初始修订（`createInitialActiveConfigRevision`，`service.go:876-905`），内联置 active，无审批流。
- 因此：**草案能创建、但没有任何机制让它 active，也从不进任何审批面**。`ApprovedBy/ApprovedAt` 与 active/archived 枚举目前是预留摆设。

### 3.5 审批策略：无引擎
- `approval_policy`（项目 / 团队上的 `map[string]any`）是**没人消费的自由 JSON blob**，不参与路由决策。
- 现存的人类闸走别的机制：预派发风险闸 `HumanApprovalRequired` 布尔、`coordination_policy["require_human_acceptance"]`。
- 审批人是**创建请求时命令式硬选单个 `TargetUserID`**（如 `projectRecord.HumanOwnerUserID`）。**没有多签 / 阈值 / 升级 / 路由规则**。

### 3.6 权限/治理原语（已存在，待归拢）
- `AllowedTeamAction`：`team.governance.approve` / `team.member.request_privileged_role` / `team.governance.edit` / `team.capability.bind|unbind`（`contracts/control-plane/openapi.yaml:9256-9264`）。
- `TeamMemberRole`：`owner / admin / approver / member / viewer`（`openapi.yaml:9266-9272`）。
- `GovernanceSummaryStatus`：`not_configured / draft_pending / active / needs_update`（`openapi.yaml:9247-9253`）。

---

## 4. 目标领域模型

### 4.1 域分离原则
- **权限中心**：只处理**权限审批主体**（category = `permission`）。
- **收件箱**：任务 / 项目任务验收（project_decision）+ 现有 approval 类不属权限的部分 + team_pending_delete，**维持现状**。
- 判据（哪些改动属"操作权限审批"）：**凡是扩张 / 变更「agent 或人被允许做什么、访问什么」的改动**——员工 role、permission_policy、接入敏感系统的能力 / MCP 授权、团队特权角色。**不属权限**的（persona 记忆、预算上限、普通技能启用）不进权限中心（见 §7 判据表）。

### 4.2 复用还是新建？——推荐「复用 `ApprovalRequest` + 加 category 区分」
现有 `ApprovalRequest` 字段已够通用（§3.2）。**不新建平行存储**，而是：
1. 给 `ApprovalRequest` 增加**分类维度** `category`（枚举 `permission | project_task`，默认 `project_task` 以兼容存量），迁移回填存量为 `project_task`。
2. **权限中心自有只读查询路径**（按 `category=permission` 过滤），**不经收件箱投影**——这是与收件箱"分离"的落点：权限中心直接读审批域，不再是 inbox 的过滤视图。
3. 权限中心的**决策执行**复用 `approval/service.go` 的决策服务（`approved/rejected/needs_more_evidence`），但**按主体分发 apply 回调**（§4.4）。

> 备选（更彻底但更贵）：独立 `permission_approval` 表 + 独立域。仅当未来主体数量与策略复杂度显著上升再考虑。列入 §10 开放决策。

### 4.3 主体注册抽象（让新主体有序接入，替代散落硬编码）
定义一个**权限审批主体（PermissionApprovalSubject）**接口，每个主体登记：
- `resource_type`（如 `digital_employee_config_revision`、`team_privileged_role_request`）
- 支持的 `decision_type` 与**动作词表**（同意 / 驳回 / 要求补证 / …）
- **上下文渲染**：把 `ContextPayload` 渲染成人类可读的"申请了什么权限、当前 vs 变更后"
- **路由**：解析应由谁审批（§4.5）
- **`Apply(ctx, requestID)` 回调**：批准后执行的副作用（幂等）——这是各主体自己的业务动作的入口

首批只需登记 S1、S2 两个主体；抽象的目的是**下一个主体不必再改封闭 switch / 接线**（把 §3.3 的"不能自助注册"债在此偿一部分）。是否做成完整插件注册表 vs 先做一个够两主体用的最小分发，列入 §10。

### 4.4 apply 接缝（跨域，务必清晰——两 spec 的边界）
- **员工配置修订激活的业务动作由 employee 域拥有**：本 spec 要求 employee 域提供 `ActivateConfigRevision(ctx, revisionID)`（draft→active + 把 role/permission 写回员工行 = A2 写回 + 落审计，幂等）。
- **权限中心拥有审批生命周期**：批准时调用该主体登记的 `Apply` → 对 S1 即调用 employee 域的 `ActivateConfigRevision`。
- 即：**"改什么、怎么应用" 归产生侧域（employee）；"谁批、批了触发 apply" 归权限中心**。员工配置 spec 负责实现 `ActivateConfigRevision` 与请求的**产生**（提交治理修订时创建 `category=permission` 的 `ApprovalRequest`）；本 spec 负责**消费与批准触发**。

### 4.5 审批路由（v1，最小可用）
- 路由目标 = **该资源作用域内持 `approver` 角色的成员**（员工→其所属 team 的 approver；无 team 或无 approver 时兜底到 team owner / 平台管理员）。
- v1 落地为「候选审批人集合」而非仍写死单个 `TargetUserID`：可先扩为 `TargetUserIDs []uuid` 或用作用域解析。多签 / 法定人数 / 升级 = 非目标。
- 权限中心页 `view: "mine"` = 我是候选审批人的待办；保留 `team` 视图看作用域内全部。

### 4.6 状态与审计
- 复用 `ApprovalStatus`（pending/approved/rejected/needs_more_evidence/cancelled）。
- 每次决策写 `ApprovalDecisionRecord` + 平台审计（谁、何时、批了什么权限、apply 结果）。这是"操作权限审批"域的合规刚需。

---

## 5. 契约变更（`contracts/control-plane/openapi.yaml`，改后走 `generate:control-plane` + 契约验证）

1. **`ApprovalRequest` schema** 增 `category`（enum `permission | project_task`）。
2. **权限中心读路径**（新增，独立于 inbox）：
   - `GET /api/v1/permission-approvals`：list，过滤 `status` / `risk_level` / `view(mine|team)` / `resource_type`；返回 summary（open_count / high_risk_count / blocked_count，对齐现有 metric 卡）。
   - `POST /api/v1/permission-approvals/{id}/decision`：body `{ decision, note?, evidence_refs? }` → 走决策服务 + 主体 apply。
   - （或复用既有 approval 决策端点——见 §10 决策 D3。）
3. **员工配置修订激活端点**（由**员工配置 spec 实现**，本 spec 依赖其存在）：
   - `POST /api/v1/digital-employees/{employeeId}/config-revisions/{revisionId}/activate`（内部由权限中心 apply 调用；或不暴露 HTTP、仅 Go 域方法 `ActivateConfigRevision`）。接缝定义见 §4.4。
4. **团队特权角色申请**（S2）：确认 / 补齐产生端点（`team.member.request_privileged_role` 目前只是 action 枚举，需要一个创建审批请求的入口）——本 spec 负责把它接进权限中心；产生入口若缺，一并补。
5. 状态词表：`apps/web/src/lib/status-labels.ts` 补权限审批相关枚举中文映射（category、resource_type、新 decision 标签），并过 `status-labels.guard.test.ts`。

---

## 6. 前端重构

1. **更名**：菜单与页面 `审批中心 → 权限中心`；`subtitle` 改为"平台操作权限审批：员工权限 / 特权角色 / 治理配置变更"；icon `ShieldCheck` 可保留（语义仍贴切）。菜单文案位置见侧栏「治理平台」分组。
2. **换数据源**：页面从 `listInboxItems({item_type:"project_decision"})` 改为读**权限审批读路径**（§5.2）。**移除项目决策**——它回归收件箱唯一呈现。
3. **保留的交互资产**（当前页已实现，迁移即可）：按事项并行处理（`pendingItemIds`）、后台失败升级页面横幅、状态/风险筛选、三张 metric 卡、加载/空/错误态（`V3LoadingState/V3EmptyState/V3ErrorState`）。
4. **动作弹窗**：当前复用 `InboxActionDialog`。分离域后，若权限审批的上下文渲染与项目决策差异大，抽一个 `PermissionApprovalDialog`（或参数化现有弹窗）——上下文要展示"申请的权限项 / 当前 vs 变更后 diff / 风险等级 / 申请人"。
5. **路由**：沿用 `/approvals` 更名，还是新增 `/permissions` + 旧路由重定向——见 §10 决策 D1。Web 内跳转一律 TanStack `Link`/`navigate`（禁裸 `<a>`）。
6. **深链**：从员工配置页「待生效」提示、员工列表「配置待生效」应深链到权限中心对应审批项（当前员工列表已有指向 `/approvals` 的链，随更名调整）。

---

## 7. 首批主体细化

### S1 · 数字员工治理配置修订审批（第一个真实消费者）
- **产生**（员工配置 spec 负责）：用户在配置页提交含 role / permission_policy（及敏感能力授权）的治理修订 → 创建 `draft` 配置修订 + 创建 `category=permission` 的 `ApprovalRequest`（`resource_type=digital_employee_config_revision`，`ContextPayload` 含"当前 vs 变更后"的 role/permission/能力 diff、风险等级、申请人）。
- **角色变更遇在役项目**：**提交即拒**（已定）——校验放在**产生侧**（员工域），冲突则拒绝创建修订与审批请求，不让脏请求进权限中心。
- **审批**（本 spec）：approver 看到 → 同意 / 驳回 / 要求补证。
- **apply**（接缝 §4.4）：同意 → 调 employee 域 `ActivateConfigRevision` → draft→active + role/permission 写回员工行 + 审计。
- **判据表（哪些配置改动需要经权限中心）**：

  | 配置项 | 是否权限审批 | 生效方式 |
  |---|---|---|
  | role | 是 | 治理修订 → 权限中心 → 激活 |
  | permission_policy | 是 | 同上 |
  | 敏感能力 / MCP 授权（扩张可访问系统）| 是（建议）| 同上 |
  | persona 记忆 | 否 | 即时 / 轻治理（员工配置 spec 定） |
  | 预算上限 | 否 | 即时 / 轻治理 |
  | 普通技能 / 环境变量 | 否 | 即时（各自端点）|

  > "persona/预算是否算权限审批"最终归属见 §10 决策 D4，但本 spec 的默认立场：**不算**，权限中心只审"权限扩张"。

### S2 · 团队特权角色申请
- `team.member.request_privileged_role` 已是 action 枚举（§3.6），但无审批落点。
- 归入权限中心：成员申请特权角色 → `category=permission`、`resource_type=team_privileged_role_request` 的审批请求 → 路由到 team approver → 批准后授予角色（apply = 写 team member role + 审计）。
- 若产生入口 / 授予动作缺失，本 spec 一并补齐最小实现。

---

## 8. 迁移与兼容

- **存量数据**：`ApprovalRequest.category` 迁移回填为 `project_task`；确认项目决策类在收件箱侧不受影响（收件箱本就显示 project_decision，移除的是**权限中心对它的重复呈现**，不动收件箱数据）。
- **迁移目录**：`apps/control-plane/internal/storage/migrations/`，改后更新 `atlas.sum`，`make -C apps/control-plane migrate-validate` 校验；遵循 `DATABASE_DESIGN.md`。
- **投影兼容**：若保留 inbox 的 `ApprovalProjectorAdapter`，需确保权限类 approval **不再双写进收件箱视图**造成两处都出现（域分离的一致性要点）。决定权限类是否彻底脱离 inbox 投影——见 §10 决策 D2。
- **路由兼容**：见 §10 决策 D1。

---

## 9. 验收标准与端到端验证（真实链路，非 mock）

按项目宪法「真实端到端验证是默认完成条件」。至少覆盖：

1. **员工权限变更闭环**：配置页改 role/permission 提交 → 权限中心（以 approver 身份登录）看到该审批项、含 diff 与风险 → 同意 → 员工配置修订转 active、员工行 role/permission 更新、审计落库。真实 Web + Control Plane + DB 走通。
2. **角色变更遇在役项目 → 提交即拒**：把员工派进一个在役项目，改 role 提交 → 产生侧拒绝，权限中心不出现脏请求。
3. **域分离**：`project_decision` 类**不再出现在权限中心**，且**仍在收件箱可见**且可处理。
4. **特权角色申请（S2）**：申请 → 权限中心可见 → 批准 → 角色授予。
5. **驳回 / 要求补证** 分支各至少一次真实走通。
6. `verify:foundation`（契约 + TS/Go/Rust）、`verify:web`、`verify:control-plane` 通过；`status-labels.guard.test.ts` 通过。
7. 收尾走 `superteam-completion-check` 门禁（`.codex/skills/superteam-completion-check/SKILL.md`）。

> 无法真实验证的部分按宪法**标记阻塞**并说明缺失依赖，不得以"未做真实链路验证"状态交付。

---

## 10. 开放决策（需人类拍板，实施前确认）

- **D1 路由 / 同名冲突**：~~沿用 `/approvals` 仅更名 vs 新增 `/permissions`~~ **【2026-07-20 已定：`/permissions`「权限中心」这个路由与页面已存在，但它是另一个东西——授权观测中心（tabs：授权概览 / 授权审计 / Runtime 范围 / 成员角色 / 权限诊断，背后是 `internal/authzcenter`）。spec 原以为 `/permissions` 空闲，实为同名冲突。结论：不新建路由、不改现有 authz 页职责，而是把「权限审批工作流域」作为**新的一个 tab「权限审批」并入现有 `/permissions` 权限中心**（与「成员角色 / 授权审计」同面，语义一致）。`/approvals` 页退役：project_decision 回收件箱唯一呈现，旧深链重定向到 `/permissions`（权限审批 tab）。】**
- **D2 与 inbox 投影的关系**：~~彻底脱离 vs 双投影~~ **【2026-07-20 已定：category=permission 的 approval **彻底脱离 inbox 投影**，不再写入 `inbox_items`；权限审批 tab 只走独立读路径 `GET /permission-approvals`。】**
- **D3 决策端点**：~~新增 vs 复用既有 approval 决策端点加守卫~~ **【2026-07-20 已定：**新增 `POST /api/v1/permission-approvals/{id}/decision`**（脱离 inbox 后不能再复用 executeInboxAction）。】**
- **D4 persona/预算归属**：~~确认 persona 记忆、预算上限不进权限中心~~ **【2026-07-20 已定：persona 记忆、预算上限不进权限中心，归员工配置的即时生效 / 轻治理；权限中心只审 role / permission / 敏感能力授权。】**
- **D5 主体接入形态**：~~最小分发 vs 通用注册表~~ **【2026-07-20 已定：本期就做**通用主体注册表**（PermissionApprovalSubject 插件式注册，偿 §3.3 硬编码债），S1/S2 作为首批注册主体，下一个主体不改封闭 switch。】**
- **D6 approval_policy**：~~v1 是否接 approval_policy 规则路由~~ **【2026-07-20 已定：v1 **不接** approval_policy；路由用「作用域内 approver 角色成员 + 兜底 owner/admin」，`approval_policy` 保持不启用。】**

### 10.1 排期结论（2026-07-20 已定）
- 本会话（本 spec）**先落域骨架 + S2 端到端**：category 迁移 / 契约 / 独立读+决策路径 / 通用主体注册表 / 前端「权限审批」tab / `/approvals` 退役 / **S2（团队特权角色申请）全链闭环 E2E**。
- **S1（数字员工治理配置修订审批）待接缝就绪**：本会话只落 S1 的**注册槽位 + 上下文渲染 + Apply 调用约定**（调 employee 域 `ActivateConfigRevision` 的接口边界），**不实现** `ActivateConfigRevision` 与产生侧（归员工配置 spec）。S1 的端到端验证在员工配置 spec 交付接缝后进行；本会话对 S1 的 E2E 标记为**阻塞（依赖未就绪）**，符合宪法。

---

## 11. 与「数字员工配置页」spec 的接缝清单（防止两会话越界 / 留缝）

| 事项 | 归属 |
|---|---|
| 配置页三层 UI、裸 JSON→结构化选择器、定位头、保存文案 | 员工配置 spec |
| 配置修订契约扩 role / permission_policy、迁移、sqlc | 员工配置 spec |
| permission_policy schema 定型 + P-enforce 强制点 | 员工配置 spec |
| 角色变更遇在役项目「提交即拒」校验（产生侧）| 员工配置 spec |
| 提交治理修订时**产生** `category=permission` 审批请求 | 员工配置 spec（按本 spec §4.4 约定的 payload 形状）|
| `ActivateConfigRevision`（draft→active + A2 写回，幂等）| 员工配置 spec 实现，本 spec 的 apply 调用 |
| 权限中心域、模型、路由、页面、更名、移走 project_decision | **本 spec** |
| 接收 / 渲染 / 审批 S1、S2 主体 + 批准触发 apply | **本 spec** |
| 团队特权角色申请（S2）接入 | **本 spec** |

两 spec 的唯一硬接口：**`category=permission` 的 `ApprovalRequest` payload 形状**（§4.4 / §7 S1）与 **`ActivateConfigRevision` 语义**。实施前两会话就这两点对齐即可各自推进。
