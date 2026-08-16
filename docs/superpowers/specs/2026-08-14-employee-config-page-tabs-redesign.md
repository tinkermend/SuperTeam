# 数字员工配置页 Tab 化重构 Spec

- 日期：2026-08-14
- 状态：草案，三项决策已由人类拍板（§2），可开工
- 目标读者：接手实施本 spec 的会话（本文档自包含）
- 前序 spec：`docs/superpowers/specs/2026-07-20-employee-config-page-refactor.md`（已实施；本 spec 是对其落地结果的纠偏与续做，不推翻其分层判据）

---

## 1. 背景与问题

`/employees/{id}/config`（`apps/web/src/features/employees/config.tsx`，834 行）由 07-20 spec §4「定位头 + 三层区块」落地。实施后又长出「身份资料」「剧本角色」两块，变成 5 段线性堆叠；同时 07-20 spec §4 要求的权限层「当前生效 vs 草案 diff、待批准草案状态」未实现。

人类使用后的直接反馈（2026-08-14）：**「所有配置集中在一个页面，要翻很长才能看到」「布局混乱、样式设计混乱」**。以下是代码实证的问题清单，本 spec 逐条对应处置。

### 1.1 信息架构

| # | 问题 | 证据 |
|---|---|---|
| A1 | 5 分区 / 8 卡 / ~14 输入控件 / 4 个提交按钮全线性堆叠，无任何页内导航；改权限必须滚到底 | `config.tsx:143-274` |
| A2 | **分区标题与卡片标题 class 完全相同**（均 `text-sm font-semibold text-ink`），层级为零，视觉上是 13 个同级标题 —— 「看起来乱」的第一直接原因 | `TierHeading` `:390` vs 卡标题 `:162 :202 :227 :654` |
| A3 | **同一个 `employee.role` 字段在页面两处出现**：定位头「显示标签」只读、权限区「显示标签」可写且走审批，中间隔上千像素 | `:292` vs `:657` |
| A4 | 两个「能力 / 环境变量」不可区分：「能力绑定」卡的 `external_capabilities`/`environment_variable_refs` 声明数组 vs 「技能/MCP/环境变量」区的真实绑定端点 | `:203-220` vs `employee-capabilities-panel.tsx:284` |
| A5 | 页面写了 **3 处「别搞混」免责文案**——需要用说明文字解释两个控件的区别，即 IA 错误的自证 | `:222` `:481` `:669` |
| A6 | 一屏两个页头：`ShellPageHeader` 已给员工名，`LocatorHeader` 立刻重复名 + id + 状态 | `:140` vs `:286` |

### 1.2 交互与状态

- **B1** 4 个独立 dirty + 4 个同权重主 CTA：保存员工说明 `:360` / 保存即时配置 `:249`（管上方 3 张卡，但按钮在卡外，作用域无视觉表达）/ 保存剧本角色 `:549` / 提交权限变更 `:691`。另加技能/MCP/环境变量区的**行内即时写库**。同页 3 种提交语义并存且未被表达。`docs/design-system/page-archetypes.md` 要求「同时仅一个主 CTA」。
- **B2** 无未保存离开拦截。全 `apps/web/src/features/` 下 0 处 `useBlocker`，而 `docs/design-system/form-flows.md:88` 明确要求「有 dirty 时离开有确认」。本页 4 个 dirty 源，是全站最需要它的页面。
- **B3** 反馈与全站不一致：本页用按钮旁绿色小字（`:254 :369 :558 :696`），全站有 `notifySuccess/notifyError`（7 个 feature 在用）。且绿字有实际缺陷——`isSuccess` 一旦为 true 常驻，用户改别的字段时「已保存并生效」仍挂着。
- **B4** 错误无原因：`保存失败`（`:257 :371`）。
- **B5** 加载/错误态违反 `DESIGN.md:82`（「不得把整棵内容树替换成单行加载中」）：`:144` 是 `<p>加载中</p>`，`:145` 是 `<p>加载失败</p>`。
- **B6** 权限提交后是黑箱：刷新后页面完全看不出有一条待批变更在飞。

### 1.3 视觉与设计规范

- **C1** **零内边距卡片（最刺眼项）**：`EmployeeCapabilitiesPanel` 两张 `SoftCard` 只传 `min-w-0`、无任何 padding（`employee-capabilities-panel.tsx:250 :329`），而 `SoftCard` 本身不带内边距（`primitives.tsx:68` 仅 radius/bg/shadow/border）。同页其他卡全是 `p-5` → 「个人技能/个人 MCP」内容贴着卡边框。旁证：`:251` 的 `<div className="gap-3 pb-3">` 中 `gap-3` 在非 flex 容器上是死类。
- **C2** **两套 token 混用**：capabilities panel 内 shadcn 默认语义（`text-muted-foreground`/`bg-background/70`/`rounded-md`）约 22 行，Soft-Flat token（`text-ink*`/`border-line`/`rounded-inner`）仅 2 行；config.tsx 主体全 Soft-Flat。同屏两套灰、两套圆角、两套边框。
- **C3** 栅格节奏断裂：整页单列 `contained`（≈1280），中间插 `xl:grid-cols-2`（`employee-capabilities-panel.tsx:249`），每列约 610px，内部再嵌 `md:grid-cols-[1fr_auto]` 技能行 → 长按钮挤爆行。
- **C4** 同文件两种内层圆角写法：硬编码 `rounded-[14px]`（`:175 :179 :528`）与 token `rounded-inner`（`:506`）并存。
- **C5** 手搓角色复选框：原生 `<input type="checkbox">` + 手写 border/bg（`:504-523`），不走设计系统基元。

### 1.4 内容与文案

- **D1** 所属团队显示「已分配/未分配」而非团队名（`:294`），而 `DigitalEmployee.team_name` 字段就在类型里且注释写明用途（`lib/api/employees.ts:33`）。违反 `DESIGN.md:80`「对象指称用名称」。
- **D2** 契约字段名当标题：外部能力（external_capabilities）/ 环境变量引用（environment_variable_refs）/ 资源授权（grants）/ 动作白名单（allowed_actions）四处括号英文连排。
- **D3** `scope:resource` 格式约束只在 label 出现一次，无校验，输错到派发才暴露。

### 1.5 `employee.role` 的现状：一个已经坏掉的字段

这是本次审计的最重要发现，独立成节。

**它的设计本意**（创建页已完整实现，与人类 2026-08-14 的口述一致）：

- 「剧本角色」`role_keys` 是真正的角色声明——`RoleKeysPicker` 多选、**必填**（`create.tsx:2033`「请至少选择一个剧本角色」）、取自租户角色词表、模板可带 `default_role_keys`（`create.tsx:1865`）、后端 `validateRoleKeys` 对词表校验后写入 `digital_employee_roles`。
- 「职责定位」`role` 是**可选展示别名**，创建页 hint 原文：「仅用于列表展示，不参与编制。未填时用所选剧本角色名称。」（`create.tsx:1354-1360`）
- 提交时 `role = 手填 || firstRoleTitle(role_keys) || employee_type`（`create.tsx:250-253`）——不填即派生自第一个剧本角色的标题。

**它的真实读者**（全仓核查，全部是显示位，无任何调度/匹配读者）：

- `employee-detail-header.tsx:94`（详情页头，**且未给中文标签，裸渲染**）
- `project/service.go:3117` `graph.Employees[i].EmployeeRole = identity.Role`（执行图员工标签）
- `create.tsx:954 :1160 :1234`（创建页摘要）

**它坏在哪**：

1. **平台自己有四个叫法**：「显示标签」（`config.tsx:292 :658`）、「展示角色」（`create.tsx:954`）、「职责定位」（`create.tsx:1160 :1234 :1354`）、详情页头无标签。这是人类看不懂这个字段的直接原因。
2. **创建与编辑是两套值域**：创建时不校验、写入词表中文标题（如「代码审查」）；编辑时 `SubmitPermissionChange` 要求命中硬编码的 9 值英文枚举 `supportedDigitalEmployeeRoles`（`service.go:322-332`，代码内已挂 TODO 承认是权宜之计）→ `service.go:1649` `isValidRole`。
3. **后果：字段事实上锁死**。存量中文值不在集合内，任何编辑（含原样重新提交）必得 400 `role not recognized`；前端错误映射只认 `active work`/`not configured`/`team` 三种（`config.tsx:722-734`），认不出该错误 → 落到兜底文案「提交失败，请稍后重试」，而重试永远不会成功。
4. **名不副实的治理开销**：改一句展示文字会创建 `risk := "high"` 的权限审批请求（`service.go:1690`），占用团队审批人。
5. 封闭英文枚举与工程宪法「Provider 类型与外部能力类型不以业务核心封闭枚举为准，以注册表与服务端校验为准」的方向相悖（此处对应物是角色词表）。

---

## 2. 决策（人类拍板，2026-08-14）

| # | 决策 | 影响 |
|---|---|---|
| **D1** | **`employee.role` 降级为普通身份字段**：移到「身份」Tab、即时保存、删除封闭英文枚举、统一改名「职责描述」、退出权限审批链路 | §5 |
| **D2** | **权限「待审批」状态 + diff 一并做**（含后端读路径） | §6 |
| **D3** | **Tab 化**（而非单页 + 锚点导航） | §4 |

D1 的理由：该字段无任何权限语义，读者全是显示位，真正的角色事实源是 `role_keys`；让一句可选展示别名走高风险审批名不副实，且当前实现使其不可编辑。

---

## 3. 当前状态实证（供实施者定位）

### 3.1 前端

- 配置页：`apps/web/src/features/employees/config.tsx`（`EmployeeConfigView`）；测试 `config.test.tsx`。
- 能力面板：`apps/web/src/features/employees/components/employee-capabilities-panel.tsx`（887 行，技能 + 技能市场 + 个人 MCP + 凭据 + skill→MCP 依赖预检 + 生效 MCP 注册表 + 环境变量）。
- 路由：`apps/web/src/routes/_authenticated/employees/$employeeId/config.tsx`（当前无 `validateSearch`）；父路由 `$employeeId.tsx` 用 `pathname.endsWith("/config")` 决定渲染 `<Outlet>` 还是只读详情页。
- 创建页：`apps/web/src/features/employees/create.tsx`（`RoleKeysPicker` 在 `:1342`，职责定位在 `:1354`）。
- 角色选择器：`apps/web/src/features/employees/role-keys-picker.tsx`（含 `firstRoleTitle`）。
- **Tab 化直接先例（照抄对象）**：`apps/web/src/routes/_authenticated/projects/$projectId/config.tsx` = `validateSearch` + `?tab=` 五页签；渲染在 `apps/web/src/features/projects/components/project-config-page.tsx:629-779`，用 `SoftTabs` / `SoftTabsList` / `SoftTabsTrigger`（带 `data-slot="page-tab-list"` / `data-slot="page-tab"`）+ `SoftTabsContent`。

### 3.2 后端

- 身份资料端点：`PUT /api/v1/digital-employees/{employeeId}/profile` → `handler.go:701-735`，请求体当前**只有 `description` 且必填**；`UpdateProfileRequest{TenantID, DigitalEmployeeID, Description *string}`（`types.go:783`）；authz action `authz.ActionEmployeeProfileUpdate`。
- 权限变更提交：`POST /api/v1/digital-employees/{employeeId}/permission-changes` → `service.go:1638-1703`。校验链：非空 → `isValidRole`（`:1649`）→ 必须有团队（`:1659`）→ `employeeBusy` 提交即拒（`:1663`）→ `ResolveTeamApprover` → `approvals.CreateRequest`，`ResourceType = permission.ResourceTypeEmployeeConfigRevision`、`ResourceID = digitalEmployeeID`、`RiskLevel = "high"`、`ContextPayload` 含 `current_role`/`target_role`/`current_permission_policy`/`target_permission_policy`。
- 批准写回：`Service.ActivateConfigRevision`（`service.go:1707-1735`），从 `ContextPayload` 取 `target_role`/`target_permission_policy`，单侧变更时另一侧从员工行回填，调 `UpdateDigitalEmployeeRolePermission`。
- 角色词表绑定：`ReplaceRoleKeys` / `validateRoleKeys`（`service.go:117 :162-209`），端点见 `replaceDigitalEmployeeRoles`（web `lib/api/employees.ts`）。

### 3.3 待审批读路径：**已存在，不需要新建 SQL**

- `approval.Service.GetRequestByResource(ctx, tenantID, resourceType, resourceID)`（`internal/approval/service.go:90`）
- → `PgRepository.GetApprovalRequestByResource`（`internal/approval/pg_repository.go:110`）
- → SQL `GetApprovalRequestByResource`（`internal/storage/queries/approval.sql:79-86`），**已含 `AND status = 'pending'` + `ORDER BY created_at DESC LIMIT 1`**。

即：按 `(resource_type='digital_employee_config_revision', resource_id=employee_id)` 查最新一条 pending 审批，正是 §6 所需。**本 spec 无需新增迁移。**

---

## 4. 目标信息架构：4 Tab（D3）

### 4.1 分 Tab 的判据

按**生效方式**分，不按对象类型分。理由：「什么时候生效、要不要审批」是本页唯一真实且用户必须理解的差异；用它做 Tab 轴，Tab 名本身就替代了 §1.1 A5 那 3 处「别搞混」免责文案。

| Tab | `?tab=` | 内容 | 保存语义 | 主 CTA |
|---|---|---|---|---|
| **身份** | `identity` | 员工说明、**剧本角色**（`role_keys`，主）、**职责描述**（`role`，次，§5）、Provider / 风险等级 / 所属团队 / 当前生效配置版本（只读事实行）、已声明能力只读回显 | 即时 | 保存（1 个，见 4.3） |
| **能力** | `capabilities` | 技能 + 技能市场、个人 MCP + 凭据、生效 MCP 注册表、环境变量 | 行内即写 | **无**（每行自带动作） |
| **执行配置** | `execution` | 人格记忆.md（编辑/预览）、每日 Token 预算、**外部能力声明**（`external_capabilities` / `environment_variable_refs`）、当前生效版本 label + `effective_config_status` | 按钮保存 → 新配置版本 | 保存（1 个） |
| **权限** | `permission` | 资源授权 `grants`、动作白名单 `allowed_actions`、**待审批状态 + diff**（§6） | 提交 → 审批后生效 | 提交审批（1 个） |

每 Tab 恰好一种保存语义、至多一个主 CTA。首屏从约 5 屏压到 1 屏。

### 4.2 归属判定说明（避免实施时反复）

- **剧本角色进「身份」而不是「权限」**：它是即时生效的身份声明（决定该员工出现在哪些编制席位候选里），不走审批。保留现有「移除角色影响在役编制 → `casting_impact_requires_confirm` 二次确认弹窗」逻辑（`config.tsx:569-580`），**不得删**。
- **外部能力声明进「执行配置」而不是「能力」**（**这是本 spec 的一处纠偏，勿按直觉放回「能力」Tab**）：`external_capabilities` / `environment_variable_refs` 的写入路径是 `POST /config-revisions`，与人格记忆、预算同属一次 revision 提交；而「能力」Tab 的技能 / MCP / 环境变量走各自的即时端点、行内即写、无保存按钮。若把声明放进「能力」Tab，该 Tab 就必须长出一个保存按钮，本 spec「一 Tab 一种保存语义」的整个立论随之瓦解。
  - 代价：「能力」与「能力声明」分处两 Tab。用**命名 + 只读回显**消化：执行配置 Tab 内该区 `SectionHeader` 标题为「能力声明（用于选角佐证，非实际绑定）」；身份 Tab 保留现有只读「已声明能力（参考）」回显（`config.tsx:528-547`）并附跳转链。**只读回显 + 链接**与 §1.1 A5 批判的「两个可编辑控件靠说明文字区分」不是一回事，不算复辟。
  - 若实施者认为该代价不可接受，**停下来问人类**，不要自行把它挪回「能力」Tab。
- **人格记忆 + 预算 + 能力声明同属「执行配置」**：三者共用 `createDigitalEmployeeConfigRevision` 一次提交，一个保存按钮。
- Tab 命名从「执行策略」改为「**执行配置**」：内容含能力声明，「策略」名不副实。

### 4.3 各 Tab 的提交实现（实施者必读）

| Tab | 提交时打的端点 | 说明 |
|---|---|---|
| 身份 | `PUT /profile`（说明 + 职责描述）+ `PUT /roles`（剧本角色） | **一个「保存」按钮 fan-out 到两个端点**。仅对 dirty 的那部分发请求；两者都 dirty 时**串行**（先 profile 后 roles），任一失败即停并保留输入、明确提示哪一部分失败。`PUT /roles` 命中 `casting_impact_requires_confirm` 时弹既有确认框，确认后只重发 roles。 |
| 能力 | 各自即时端点（`/skills`、`/mcp-bindings-v2`、`/environment-variables`） | 沿用 `EmployeeCapabilitiesPanel` 现有实现，不改行为。 |
| 执行配置 | `POST /config-revisions` | 单次提交 **persona + capability_bindings + budget 全量三段**，未改动段从 `employee.data` 当前值回填。沿用现有 `otherCapabilityKeys` 透传与 `RESERVED_CAPABILITY_KEYS` 剥离（`config.tsx:46-54`）——`skills` / `mcp_servers` 是已废弃逻辑键，服务端拒绝非空回传，**绝不重新发送**。 |
| 权限 | `POST /permission-changes` | 见 §6。 |

不得为 Tab 化新增端点或拆分既有契约。

### 4.4 路由与 Tab 状态

- **沿用现有路由 `/employees/$employeeId/config`，Tab 状态走 `?tab=` search 参数**，`validateSearch` 白名单 `identity | capabilities | execution | permission`，非法值回落 `identity`。
- **不新增子路由**，因此**不动** `$employeeId.tsx` 的 `pathname.endsWith("/config")` 判断（该 hack 在 projects 侧同样存在，属既有债，不在本 spec 范围）。
- 切 Tab 时 `navigate({ search })` 同步 URL，保证可分享 / 可回退 / 刷新保位。
- 完全镜像 `projects/$projectId/config.tsx` + `project-config-page.tsx:629-779` 的写法，含 `SoftTabs` 与 `data-slot` 属性，使两个配置页视觉与交互同源。

### 4.5 页面骨架

- `Main width="contained"`（设置/单对象编辑档位，符合 `page-archetypes.md` 总表），**不改 wide**。
- 页头：`ShellPageHeader`（返回详情）+ **精简后的定位头**——删除与 ShellPageHeader 重复的员工名（§1.1 A6），只保留 Provider / 风险 / 团队名 / 当前生效版本 + 状态 pill。
- Tab bar 紧贴定位头下方，`SoftTabs`。
- 分区标题层级：Tab 内分区用 `SectionHeader`（`components/superteam/converge.tsx:98`），卡内标题降一级，与分区标题拉开字重/字号（消除 §1.1 A2）。
- 每个 Tab 内容**懒挂载但状态不丢**：切走再切回不得清空未保存输入（与 §7 第 8 项的离开拦截配套）。

### 4.6 目标文件结构（避免各自乱拆）

现状 `config.tsx` 单文件 834 行承载全部。目标拆分：

| 文件 | 职责 |
|---|---|
| `features/employees/config.tsx` | 壳：查询员工、定位头、`SoftTabs`、四态、离开拦截；**不再承载表单逻辑** |
| `features/employees/components/config-identity-tab.tsx` | 身份 Tab（说明 + 职责描述 + 剧本角色 + 只读事实行 + 已声明能力回显）；含 fan-out 提交（§4.3） |
| `features/employees/components/config-execution-tab.tsx` | 执行配置 Tab（人格记忆 + 预算 + 能力声明 + 版本信息）；单次 revision 提交 |
| `features/employees/components/config-permission-tab.tsx` | 权限 Tab（grants + allowed_actions + 待审批 Callout + diff） |
| `features/employees/components/employee-capabilities-panel.tsx` | **保持组件边界与 props 不变**，整体挂进能力 Tab；本次只改样式（§7 第 1/2/11 项），不拆不改行为 |
| `features/employees/components/permission-change-diff.tsx` | 待审批 diff 展示（新增，纯展示组件，便于单测） |

`ChipsEditor`、`stringArray`、`budgetPolicy*`、`statusTone` 等现有辅助函数按使用方就近下沉或抽到 `features/employees/config-utils.ts`，**不要复制多份**。

---

## 5. `role` 字段收敛（D1 落地）

### 5.1 统一命名：「职责描述」

全站改名清单（**已全量核对，勿凭记忆增删**；改完 `rg '显示标签|展示角色|职责定位' apps/web/src` 应仅剩 §5.1 末条所述的模板页一处或 0 命中）：

| 位置 | 现状 | 改为 |
|---|---|---|
| `config.tsx:292` 定位头 | 显示标签 | 职责描述（只读事实行保留） |
| `config.tsx:657-658` 权限区 | 显示标签 · 当前：X | **删除**（迁入身份 Tab） |
| `create.tsx:954` chip | 展示角色 | 职责描述 |
| `create.tsx:1160 :1234` 摘要 | 职责定位 | 职责描述 |
| `create.tsx:1354` Field label | 职责定位 | 职责描述 |
| `create.tsx:434 :516 :733` 正文说明 | 「按职责定位、能力选择…」等 | 职责描述 |
| `employee-detail-header.tsx:94` | 裸渲染无标签 | 补中文标签托底 |
| `config.test.tsx:154 :158 :175` | 断言「显示标签」 | 同步改断言（含注释） |
| `create.test.tsx:597 :720 :746 :805 :808 :842 :853 :908 :925` | `getByLabelText("职责定位")` ×9 | 同步改断言 |

> `templates.tsx:469`「展示角色」指的是**员工模板的 `default_role`**（另一实体的同源字段），不是 `employee.role`。建议同批对齐为「默认职责描述」以免词表再分叉；若实施者判断模板域应另行处置，可留待模板 spec，但须在 CHANGELOG 记明未对齐。

hint 文案统一采用创建页现有原文：**「仅用于列表展示，不参与编制。未填时用所选剧本角色名称。」**

### 5.2 前端改动

- 权限 Tab **移除** role 输入框与相关 state（`config.tsx:599 :630-631 :656-671`）。
- 身份 Tab 新增「职责描述」`Input`，**可选**，placeholder 展示派生默认值（复用 `firstRoleTitle(role_keys, vocabulary)`），与「员工说明」同属一次「保存」。
- `SubmitPermissionChangeInput` 前端类型移除 `role`。

### 5.3 后端改动

1. `PUT /profile` 收 `role`：
   - `UpdateProfileRequest` 增 `Role *string`（`types.go:783`）。
   - handler（`handler.go:714-730`）：`description` 与 `role` 均改为可选指针，**至少一个非 nil**，否则 400；两者都为 nil 时报 `description or role is required`。空串语义：`description` 沿用「空串清空」；`role` 空串 → **回落为派生值**（服务端按 `role_keys` 首个的词表 title 填充，与创建路径 `service.go:872-878` 的 fallback 语义一致），不得写入空串。
   - `Service.UpdateProfile` 同步处理 `Role`。
2. **删除封闭枚举**：删 `supportedDigitalEmployeeRoles`（`service.go:320-332`）与 `isValidRole`（`service.go:1737-1741`）及其调用点 `service.go:1649`。
3. `SubmitPermissionChange` 不再接受 role：
   - `SubmitPermissionChangeRequest` 移除 `Role` 字段；空判据从 `req.Role == nil && req.PermissionPolicy == nil` 改为 `req.PermissionPolicy == nil`（`service.go:1643`）。
   - `ContextPayload` 不再写 `target_role`；`current_role` 可保留作审批弹窗上下文展示（不参与写回）。
   - `permissionChangeSummary`（`service.go:1755`）去掉 role 分支。
4. **在途兼容（必做，勿省）**：`ActivateConfigRevision`（`service.go:1707`）**保留**读取 `target_role` 的分支，使本次变更前已创建、尚未批准的 pending 审批仍能正确写回；仅不再产生新的携带 `target_role` 的请求。实施时须查开发库确认在途 pending 数量，并在 CHANGELOG 记明。

### 5.4 契约改动（`contracts/control-plane/openapi.yaml`）

- `UpdateDigitalEmployeeProfileRequest`（`:16426`）：`description` 与 `role` 均为可选，移除 `required: [description]`，description 说明补「至少提供 description 或 role 之一」；新增 `role` 属性并写明「可选展示别名；留空则按剧本角色派生」。端点 description（`:5520-5522`）同步更新。
- `SubmitPermissionChangeRequest`：移除 `role`（**BREAKING**，须在 CHANGELOG 标注）。
### 5.5 前端 API 客户端改动（`apps/web/src/lib/api/employees.ts`）

- `UpdateDigitalEmployeeProfileInput`（`:862`）：`description` 与 `role` 均改为可选，注释写明「至少一个」。
- `SubmitPermissionChangeInput`（`:929`）：移除 `role`。
- 新增 `getEmployeePermissionChange(options, employeeId)`（§6）：**必须处理 204** —— `response.status === 204` 时返回 `null`，不得走 `parseJson`（204 无 body，解析会抛）。TanStack Query 侧把 `null` 当合法空态，不当错误。

### 5.6 契约门禁的机械要求（漏一步必红，勿凭印象跳过）

`scripts/verify-foundation-contracts.mjs` 会交叉比对三处，新端点必须**同批**齐全：

1. `contracts/control-plane/openapi.yaml` 新增 path + operationId + 请求/响应 schema；
2. `apps/control-plane/internal/api/server.go` 注册路由（员工路由段在 `:405-429`，本次两处紧邻 `:418` 的 `/profile` 与 `:423` 的 `/permission-changes`）；
3. 跑 `corepack pnpm generate:control-plane` 刷新 `apps/control-plane/internal/api/gen/control_plane.gen.go`（门禁会校验 openapi schemas 与生成类型是否同步——该门禁的由来正是历史上改了 yaml 没跑生成器）。

改 openapi 却不跑生成器 = `verify:contracts` 直接失败。

---

## 6. 权限待审批状态 + diff（D2 落地）

### 6.1 新端点

```
GET /api/v1/digital-employees/{employeeId}/permission-change
```

- 授权：`h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, &employeeID, "employee permission change read")` —— 与 `GetDigitalEmployee`（`handler.go:416`）同一个 action，**不新造 action、不复用写侧的 `ActionEmployeeConfigCreate`**（那是 `SubmitPermissionChange` 用的，见 `handler.go:846`）。
- 实现：调 `approval.Service.GetRequestByResource(ctx, tenantID, permission.ResourceTypeEmployeeConfigRevision, employeeID)`（§3.3，SQL 已过滤 pending + 最新一条）。
- **无待批 → 204 No Content**（仓库既有约定：单例子资源「存在但无记录」返 204 非 404。照抄先例：`apps/control-plane/internal/project/handler.go:1756-1764`，`GetAcceptance` 返回 nil 时 204）。
- 200 响应体：

```jsonc
{
  "request_id": "uuid",
  "status": "pending",
  "risk_level": "high",
  "created_at": "RFC3339",
  "requester_name": "张三",          // 服务端补名，勿裸 UUID
  "approver_name": "李四",           // 同上
  "current_permission_policy": { "grants": [...], "allowed_actions": [...] },
  "target_permission_policy":  { "grants": [...], "allowed_actions": [...] }
}
```

- 名称补名在服务端读路径完成（`DESIGN.md:80`「对象指称用名称」；参考收件箱 `source_project_name` 先例），前端不得逐行请求解析。
- 兼容：若 `ContextPayload` 含存量 `target_role`（§5.3 第 4 点），响应可附 `target_role`/`current_role` 供 diff 展示，前端按存在与否条件渲染。

### 6.2 前端呈现（权限 Tab）

- 顶部 `Callout tone="warn"`：「有 1 条待审批的权限变更 · 由 {approver_name} 审批 · {relative time}」+ 「去权限中心」链接（`/permissions`）。
- diff 区：`grants` / `allowed_actions` 逐项标注 新增 / 移除 / 保持，用中性色 + 文字，不用大面积语义底色（`DESIGN.md` 工作对象界面规则）。
- **有待批时禁用「提交审批」按钮**并说明原因（避免重复提交；后端 `employeeBusy` 与审批人路由不覆盖此情形）。
- 提交成功后 `invalidateQueries` 该 query，使 Callout 立即出现（不靠刷新）。
- 四态齐备：loading（局部 `LoadingState`）、204 空（不渲染 Callout，非错误）、error（`Callout tone="danger"` + 重试）。

---

## 7. 视觉 / 规范必修项（= §11 批一，先于 Tab 化独立交付）

以下 11 项**不依赖后端、不依赖 IA 改动**，可先独立完成并验证。第 7、8、10 项落在 `config.tsx` 上，批二拆文件时随之迁移，不算返工。

1. `employee-capabilities-panel.tsx:250 :329` 两张 `SoftCard` 补 `p-5`；删 `:251` 的死类 `gap-3`。**（收益最大的一行改动）**
2. 该文件约 22 行 shadcn token → Soft-Flat token：`text-muted-foreground`→`text-ink-2`/`text-ink-3`、`bg-background/70`→`bg-card-soft`、`rounded-md`→`rounded-inner`、裸 `border`→`border-line`。
3. `config.tsx:294` 所属团队改用 `employee.team_name`，缺失时「无团队归属」。
4. `config.tsx:144-145` 加载/错误态改 `DetailSkeleton` / `ErrorState`（保留 `ShellPageHeader` 壳）。
5. 全部保存反馈改 `notifySuccess` / `notifyError`，删除 4 处常驻绿字（`:254 :369 :558 :696`）。
6. 分区标题改 `SectionHeader`，卡内标题降一级，拉开层级（消除 §1.1 A2）。
7. `config.tsx:504-523` 手搓 checkbox 改设计系统基元；`rounded-[14px]`（`:175 :179 :528`）→ `rounded-inner`。
8. **新增 dirty 离开拦截**（`useBlocker`，全站首例）：任一 Tab 有未保存变更时拦截路由离开与 Tab 切换，二次确认。无 dirty 不拦（`form-flows.md:60`）。
9. `submitPermissionErrorMessage`（`config.tsx:722-734`）兜底文案去掉「请稍后重试」这类误导性建议，改为可诊断表述 + 保留原始 detail 供展开。
10. 删除 3 处「别搞混」免责文案（`:222 :481 :669`）——Tab 化后不再需要。
11. `xl:grid-cols-2`（`employee-capabilities-panel.tsx:249`）在 contained 宽度下改为单列或提高断点，消除技能行被挤爆（§1.3 C3）。

---

## 8. 验证判据（GATE）

### 8.0 单测改动清单（会大面积红，先心里有数）

- `features/employees/config.test.tsx`：现有断言基于「单页 5 分区」结构，Tab 化后**整体重写**。重写时**必须保留**以下既有覆盖，不得借重构之名丢失：
  - `casting_impact_requires_confirm` → 确认弹窗 → 确认后重发（§4.2）
  - `RESERVED_CAPABILITY_KEYS` 剥离：提交 revision 时 `skills` / `mcp_servers` 不出现在请求体
  - 预算非正整数校验报错
  - 权限提交失败的中文错误映射
  - 新增：`?tab=` 白名单与非法值回落、204 空态不报错、有待批时提交按钮禁用
- `features/employees/create.test.tsx`：9 处 `getByLabelText("职责定位")` 改「职责描述」（§5.1）。
- `components/employee-capabilities-panel.test.tsx`：只改样式则断言应基本不变；若断言绑定了 `text-muted-foreground` 等 class，同步改。
- 后端：`employee` 包内所有引用 `supportedDigitalEmployeeRoles` / `isValidRole` 的测试需删改；`SubmitPermissionChange` 的 role 相关用例删除；新增 `UpdateProfile` 收 role（含空串派生）与新端点 204/200 两态用例。
- Web 测试须串行/分块跑（既有堆内存问题），走 `corepack pnpm verify:web`。

### 8.1 端到端判据

按 `CLAUDE.md`「默认完成条件是真实端到端」：Web + Control Plane + DB（本 spec 不触 Runtime/Provider，能力 Tab 只做样式改动，故 Runtime 腿非必须）。声称 E2E 通过须记录并复核 `control-plane`/`web` 的 pid，`owner=` 异源则结论作废。

| # | 判据 | 通过条件 |
|---|---|---|
| G1 | Tab 化与 URL 同步 | 浏览器打开配置页，四个 Tab 均可切换；`?tab=` 随切换更新；刷新保位；非法 `?tab=xxx` 回落身份 Tab |
| G2 | 职责描述即时保存 | 身份 Tab 把职责描述改成任意中文（如「代码复核」），保存 → 200 → 详情页头与员工列表同步显示新值。**这是当前必 400 的路径，是 D1 的核心回归** |
| G3 | 职责描述留空派生 | 清空职责描述保存 → 服务端回落为剧本角色首个词表 title，不写空串 |
| G4 | 封闭枚举已删 | `rg 'supportedDigitalEmployeeRoles|isValidRole'` 0 命中；权限提交不再接受 role |
| G5 | 在途兼容 | **造法：改后端代码之前**，先在当前 main 上用旧接口提交一次带 role 的权限变更（或直接对 `approval_requests` 插一行 `resource_type='digital_employee_config_revision'`、`context_payload` 含 `target_role`），改完代码后到权限中心批准 → role 仍正确写回（证 `ActivateConfigRevision` 的兼容分支未被误删） |
| G6 | 待审批可见 | 权限 Tab 提交一次变更 → Callout 立即出现（不刷新）→ 刷新后仍在 → diff 正确 → 提交按钮禁用 → 权限中心批准后 Callout 消失且新值生效 |
| G7 | 无待批返 204 | 全新员工 GET permission-change 返 204，前端不渲染 Callout 且不报错 |
| G8 | 剧本角色回归 | 身份 Tab 取消一个已被编制占用的角色 → `casting_impact_requires_confirm` 弹窗仍出现 → 确认后解除编制 |
| G9 | 离开拦截 | 任一 Tab 改动未保存 → 切 Tab / 返回详情 → 二次确认；无改动时不拦 |
| G10 | 视觉 | 浏览器截图确认能力面板卡片有内边距、同屏无两套灰/圆角、无横向溢出、暗色无硬编码浅色 |
| G11 | 门禁 | `corepack pnpm generate:control-plane` → `corepack pnpm verify:control-plane`（= `verify:contracts` + `test:go`）→ `corepack pnpm verify:web`（= web test + typecheck + build）全绿。**只用已登记脚本，禁止 `npx vitest run` / `npx playwright install`** |

---

## 9. 非目标

- **Provider 变更**：仍不可改（绑定执行实例/Runtime 亲和/家目录），定位头只读呈现。
- **能力 / MCP 分级授权是否需审批**：沿用 07-20 spec §12 开放决策，v1 不纳入。
- **详情页改造**：除 `employee-detail-header.tsx:94` 补中文标签外不动；其观测职责（工作节奏、能力导轨、删除）保持。
- **角色词表管理页** `/role-vocabulary`：不在范围。
- **`$employeeId.tsx` 的 `pathname.endsWith("/config")` hack**：既有债，projects 侧同构，另行处置。
- **`external_capabilities` / `environment_variable_refs` 的语义收敛**：本次只做 IA 归位与文案，不改数据模型。

## 10. 开放项与风险

1. **`role` 是否最终退役**：本 spec 采纳「降级为普通身份字段」（决策 A）。若后续确认它与 `role_keys` 完全冗余，可另立 spec 走「退役」（原选项 C，需迁移详情页头 / 执行图标签 / 创建页三处显示点）。本 spec 不预埋该方向。
2. **在途 pending 审批数量未知**：实施首步须查开发库确认，决定 §5.3 第 4 点兼容分支的保留时长。
3. **`grants` 格式校验缺失**（§1.4 D3）：`scope:resource` 无服务端校验，输错到派发才暴露。本 spec 只改文案不加校验，校验另立。
4. **`useBlocker` 全站首例**：若 TanStack Router 版本行为与预期不符，退化为 Tab 切换内拦截 + 路由离开不拦，并在 TODO 记明。

---

## 11. 建议实施顺序（三批，可分提交）

严格依赖：**批一 → 批二 → 批三**。批一不依赖后端，改完人类立刻能看到视觉改善；批三依赖批二的 Tab 骨架存在。

| 批次 | 内容 | 依赖 | 独立可验 |
|---|---|---|---|
| **批一 · 视觉止血** | §7 全部 11 项（其中第 1 项一行改动收益最大），不动 IA、不动后端、不动契约 | 无 | 浏览器截图 + `verify:web` |
| **批二 · Tab 骨架 + role 收敛** | §4 全部（拆文件、`?tab=`、四 Tab 归位）+ §5 全部（前后端 + 契约 + 改名）+ §8.0 单测重写 | 批一 | G1–G5、G8–G10 |
| **批三 · 待审批可见** | §6 全部（新端点 + diff 组件 + Callout） | 批二 | G6、G7 |

批二含 BREAKING 契约变更（`SubmitPermissionChangeRequest` 去 role），提交信息须标注。

## 12. 交接与环境须知（接手会话开工前必读）

- **本文档自包含**，但仍须先读 `CLAUDE.md`（工程宪法）与 `DESIGN.md`（视觉基线）；组件族按 `DESIGN.md` 文档路由表**按需**读取，本 spec 涉及的是 `page-archetypes.md` / `forms.md` / `form-flows.md` / `feedback.md`，不必全读。
- **并行开发**：若在 worktree 里做，**必读** `docs/PARALLEL_DEVELOPMENT.md`。未共享 `SUPERTEAM_DEV_PID_DIR` 时从 worktree 执行 `restart` 是**退出码 0 的静默空操作**——服务仍跑别人的代码，会产出假的验证结论。`scripts/dev-services.sh status` 的 `owner=` 是真实 cwd，验证前后各核一次。
- **共享 checkout 时**：只用 `git add <显式路径>`；不为他人切/删分支；全仓生成（`generate:control-plane`）会吸收他人在途改动，提交前核对暂存内容。
- **验证纪律**：mock/单测/构建通过 ≠ 已验证。§8.1 的 E2E 判据必须在真实 Web + CP + DB 上跑。无法验证的项须标阻塞并说明依赖，不得默认通过。
- **收尾**：变更写 `CHANGELOG.md`（含 §5.3 第 4 点的在途 pending 审批处置结论、§5.1 末条模板页是否对齐）；人类明确延后的事项写根目录 `TODO.md`；收尾走 `superteam-completion-check`（`.codex/skills/superteam-completion-check/SKILL.md`）。
- **遇到 §4.2 所述的「外部能力声明该放哪个 Tab」之类的架构性疑问，先问人类，不要自行改判。**
