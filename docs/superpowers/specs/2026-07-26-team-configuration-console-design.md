# 团队配置控制台 — 深入分析与设计

- 日期：2026-07-26
- 范围：Web 控制台（团队详情/团队配置）、Control Plane（tenant / skill / capability / employee）、契约、DB 迁移
- 状态：**设计草案，未开工**。2026-07-26 已拍板 D1=宪法接通、D2=能力冲突按「团队接管」收敛（不新增团队级环境变量值存储）、D7=独立配置页、D9=宪法只作约束文本注入（不参与门禁判定）、D10=当前为开发环境，历史数据可直接收敛无需 dry-run；其余待抉择项见 §9
- 前置阅读：`docs/superpowers/specs/2026-07-08-team-governance-slimming-and-project-runtime-binding-design.md`（团队瘦身终态模型）、`DESIGN.md`

---

## 1. 现状事实盘点（带代码锚点）

### 1.1 团队相关写操作现在分布在哪

| 能力 | 入口位置 | 后端 | 门禁（后端） | 门禁（前端实际用的） |
|---|---|---|---|---|
| 人类成员 增/删 | 详情页头卡「配置团队」Sheet（`team-human-members.tsx:94`） | `POST/DELETE /teams/{id}/members` | `team.member.add` / `team.member.remove` | 一致 |
| 人类成员 改角色 | **无 UI，无路由** | — | `team.member.change_role`（常量存在，无路由） | — |
| 人类成员 高权限申请 | **无 UI**（`requestTeamPrivilegedRole` 在 `permission-approvals.ts:127` 零调用） | `POST /teams/{id}/privileged-role-requests` | `team.member.request_privileged_role` | — |
| 数字员工 收编（候岗→本队） | 详情页正文「数字员工」区内嵌下拉（`team-overview-tab.tsx:207`） | `POST /teams/{id}/digital-employees` | `team.update` | `team.update` ✓ |
| 数字员工 移出/换队 | **无 UI**（`reassignDigitalEmployeeTeam` 在 `employees.ts:828` 零调用） | `PUT /digital-employees/{id}/team`（`team_id` **required**，不可置空） | 无团队维度检查，只检查目标团队 active | — |
| 技能 安装/移除 | 详情页正文「公共技能」卡（`team-capabilities-tab.tsx:154`） | `POST/DELETE /teams/{id}/skills` | `team.capability.bind` / `team.capability.unbind` | **`team.governance.edit`** ✗ |
| MCP 绑定/解绑 | 详情页正文「公共 MCP」卡（同上:190） | `POST/DELETE /teams/{id}/mcp-bindings` | **`team.capability.manage`**（第三个动作，且不在 `overviewActions`） | **`team.governance.edit`** ✗ |
| 团队宪法 编辑 | 详情页正文最下方 textarea（`team-constitution-tab.tsx`） | `PATCH /teams/{id}/constitution` | `team.governance.edit` | 一致 |
| 团队 改名/描述/负责人 | **无 UI**（`updateTeam` 在 `teams.ts:230` 有客户端，页面零调用） | `PATCH /teams/{id}` | `team.update` | — |
| 删除团队 | 详情页头卡 | `DELETE /teams/{id}` | `team.delete` | 一致 |
| 团队审计流 | **无 UI**（`listTeamAuditEvents` 在 `teams.ts:347` 零调用） | `GET /teams/{id}/audit` | `team.audit.read` | — |

结论：**"配置团队"这个按钮名不副实**——它只是人类成员管理抽屉。真正的团队配置（数字员工编制、技能、MCP、宪法）散在详情页正文，另有 4 项能力后端已有、前端零接线。

### 1.2 对照：员工与项目都已有独立配置页，团队没有

- `/employees/$employeeId` + `/employees/$employeeId/config`（三层：只读定位头 / 即时生效层 / 权限审批层）
- `/projects/$projectId` + `/projects/$projectId/config`（含 `project-config-revision-history.tsx` 版本历史）
- `/teams/$teamId` — **观察面与写操作混在同一页**，无 `/config`。

### 1.3 数字员工归属是单向阀门（用户提到的核心问题，已证实）

- 进：`POST /teams/{id}/digital-employees`，且 SQL 带 `team_id IS NULL` 守卫（`tenant/pg_repository.go:207`），只能收编候岗员工。
- 换队：`PUT /digital-employees/{id}/team`，`ReassignDigitalEmployeeTeamRequest.team_id` 是 **required**（契约 `openapi.yaml:12594`，服务端 `employee/service.go:1202` 拒绝 nil），**没有任何路径能把 team_id 置回 NULL**。
- 前端：两个入口都没有接线（收编有，移出/换队完全没有）。
- 结果：**数字员工一旦进入团队，产品内再无出路**；`/run-overview` 的"候岗大厅"只对新建未归属员工有意义，无法回流。

三个附带缺陷：
1. `ReassignDigitalEmployeeTeam`（`employee/pg_repository.go:287`）**绕过每团队员工数限额**——`ensureTeamDigitalEmployeeCapacity` 只在创建路径调用（`employee/service.go:517` 附近），换队路径不调，可越过 `max_digital_employees_per_team`。
2. 换队无在役任务守卫。换队副作用是 agent home dir 按 `(team, employee)` 键重算（契约自己写了这条），provider 会话连续性重置；员工有 running 任务时换队没有任何拦截或警告。
3. 换队审计写在 `resource_type=digital_employee`（`employee/pg_repository.go:329`），**不会出现在团队审计流**里。

### 1.4 团队宪法是空转字段（严重）

- 存储：`tenant_teams.constitution` jsonb（迁移 046 加列，047 删掉旧的 `tenant_team_config_revisions`）。
- 写：`PATCH /teams/{id}/constitution`，整块 jsonb 覆盖，**不写审计**，**无版本、无历史、无 diff**。
- 读：只有 `employee/pg_repository.go:349 GetTeamBaseline`，两个调用点都在**员工创建选项/校验**路径（`service.go:147`、`:517`）。
- **`hard_rules` 全仓无任何执行侧消费者**：`rg hard_rules` 命中范围只有 CHANGELOG、测试、docs、Web 组件；`run_service.go` 派发 payload 里注入的是员工 `persona_memory_markdown`（`run_service.go:1644`），**没有团队宪法**；runtime-agent 侧零命中。
- 2026-07-08 设计 §2.3 写的"员工有效宪法 = `tenant_teams.constitution.hard_rules` ++ 员工 `constitution_addendum`"**从未落地**，而 `constitution_addendum` 已在迁移 051 被移除（现存于 `employee/handler.go:765` 的 legacy 字段拒绝列表）。
- UI 文案「约束执行边界的硬性规则」是**当前不成立的承诺**。

### 1.5 技能 / MCP 配置的贫瘠，是相对员工级面板的结构性缺失

同仓已有的员工级面板（`employee-capabilities-panel.tsx`）具备、而团队级完全没有的能力：

| 能力 | 员工级 | 团队级 |
|---|---|---|
| 技能的 MCP 依赖预检（缺绑定/缺 env 会阻断派发） | ✓ `listEmployeeSkillMcpDependencyStatus`，行内红字点名 | ✗ 无 |
| 技能 runtime 依赖状态（缺工具/缺 env/等待上报） | ✓ `skillLoadStatePills` | ✗ 无 |
| 生效能力合成视图（继承 vs 个人、来源标注） | ✓ `EffectiveMcpRegistrySection` | ✗ 无（团队看不到自己下发了什么） |
| 环境变量管理 | ✓ 增/替换/删 + 指纹 + 配置状态 | ✗ **团队级根本没有环境变量概念** |
| MCP 绑定前置校验（缺 env 禁止绑定） | ✓ `canCreateMcp` 要求 `missingEnvVars.length === 0` | ✗ 无校验，绑完只提示一句"团队级绑定为建议性" |
| 移除影响提示 | 部分（继承项只读不可移除） | ✗ 无 |

后端已在注释里承认这个洞：`capability/service.go:204` —
> `Team-level env-var preflight is advisory because each employee carries its own env values.`

即：**团队绑一个需要凭据的 MCP，对全队没有任何实际效果，直到每个员工分别配好 env**，而团队页看不到谁配了、谁没配。团队 MCP 目前是"看起来有、实际不通"的能力。

补充：项目级 MCP 绑定已于 2026-07-22 退役，MCP 只剩团队级与员工级两层——**团队级是唯一的"公共"层**，它的残缺没有替代品兜底。

### 1.5.1 团队/员工同一能力的冲突处理现状（已逐条核实）

两个维度的语义划分本身是对的：**注册表定义**（`mcp_servers.required_env_vars`、`url`、`auth_strategy`）是共享事实源；**团队/员工绑定**各自携带 `credential_env_var`（用哪个变量名取凭据）；**变量的值**只存在员工级（`digital_employee_environment_variables`）。问题不在维度划分，在冲突处理：

| 方向 | 技能 | MCP |
|---|---|---|
| 员工装、团队已有 | **写时拒绝** `ErrTeamAlreadyInherited`（`skill/service.go:397`）；但 `InstallSkill` 把它转成 `AlreadyBound=true` 静默"成功"（`service.go:131`） | **不拒绝**，直接写进 `digital_employee_mcp_bindings_v2`（`CreateEmployeeMCPBindingV2` 零冲突检查） |
| 团队装、员工已有 | **无处理**，`skill_agent_bindings` 行残留 | **无处理**，`digital_employee_mcp_bindings_v2` 行残留 |
| 生效列表 | `NOT EXISTS` 屏蔽员工副本，团队优先（`skill/pg_repository.go:358`） | `NOT EXISTS` 屏蔽员工副本，团队优先（`capability.sql:216-223`） |
| 团队解绑后 | `NOT EXISTS` 不再成立 → 员工旧副本**自动复活** | 同左 |

由此产生三个真实缺陷：

1. **幽灵绑定**：`ListEmployeeMCPBindingsV2`（`capability.sql:126`）**不做去重**，员工配置页「个人 MCP 绑定」区照常显示该条，而同页「生效 MCP 配置」区（去重）不显示 → 同一页两个区自相矛盾，用户无法判断到底生不生效。
2. **凭据静默丢弃**：员工那条的 `credential_env_var` 可能与团队那条不同（如 `GITHUB_TOKEN_ALICE` vs `GITHUB_TOKEN`）；被屏蔽后员工的配置无声失效，缺哪个变量在员工页看不出来。
3. **陈年配置复活**：团队解绑 MCP 这个看似只影响团队的操作，会让员工侧很久以前的、可能早已不该用的旧绑定悄悄回到生效集合。

另核实：`mcp_servers(tenant_id, server_key) WHERE deleted_at IS NULL` 有唯一索引（迁移 037:37），因此按 `mcp_server_id` 去重等价于按 `server_key`，不存在"同 key 多定义"导致投影到 `.mcp.json` 撞名的隐患。

### 1.6 数据与展示缺陷

- `capability_count` 在 `ListTenantTeamSummaries` 与 `GetTenantTeamSummary` 里都是**硬编码 `0::integer`**（`queries/tenant_team_config.sql:54`、`:132`）→ 团队卡片与详情头卡的「N 能力绑定」**恒为 0**，即使团队装了 10 个技能。
- `pending_draft_count` 同样硬编码 0 → 头卡「待审批 N」pill（`team-detail-layout.tsx:80`）**永远不会出现**。
- `risk_summary` 恒为空串。
- `governance_status` 判据是 `constitution = '{}'` → "未配置治理"只反映那个空转字段，与团队是否真的有能力基线无关。

### 1.7 审计覆盖不全且资源类型不统一

已写团队审计：`team.create`、`team.member.add`(resource_type=**team_member**)、`team.digital_employee.bind`(team)、`team.delete`(team)、`team.restore`、`team.delete.confirmed`、`team.member.grant_privileged_role`(**team_member**)。

未写任何审计：**团队宪法修改**、团队技能 bind/unbind、团队 MCP bind/unbind、`team.member.remove`、`team.update`。

`GET /teams/{id}/audit` 按 `resource_type='team'` 过滤（`audit/service.go:32` `ListResourceEvents`），因此**成员相关事件即使写了也不会出现在团队审计流里**。而这个端点本身还没有 UI。

---

## 2. 问题归类

- **A 结构**：配置入口散落、"配置团队"语义窄化；团队是唯一没有 `/config` 页的一等实体。
- **B 能力缺口**：数字员工只进不出；成员角色变更/高权限申请无 UI；团队身份（名称/负责人）无 UI；团队审计无 UI。
- **C 语义空转**：团队宪法无消费者；`capability_count` / `pending_item_count` / `risk_summary` 恒零。
- **D 权限错位**：前端用 `team.governance.edit` 控制能力面板，后端要 `capability.bind` / `capability.unbind` / `capability.manage`；后者还不在 `overviewActions` 里，前端拿不到。
- **E 治理缺口**：团队级配置（一次影响全队）无审批、无版本、无审计、无影响面预览。
- **F 一致性**：换队绕过员工数限额、无在役守卫、审计落错资源。
- **G 冲突处理**：团队/员工同一能力靠读时静默屏蔽，产生幽灵绑定、凭据静默丢弃、解绑后旧配置复活；技能拒绝而 MCP 放行，两条路行为还不一致（§1.5.1）。

---

## 3. 目标模型

**团队 = 人员编制 + 能力基线 + 行为约束**，三者都是"对成员一次性生效"的公共层；团队不干预运行范围（provider / runtime / 项目），那已由 2026-07-08 设计归给项目。

```
团队编制  = 人类成员（tenant_members）+ 数字员工归属（digital_employees.team_id）
团队能力基线 = team_skill_bindings ∪ team_mcp_bindings （+ 待定：团队环境变量）
团队行为约束 = tenant_teams.constitution.hard_rules   （待定：接通或退役，§9-D1）
```

四条不变量：
1. **基线只读继承、不可退订**——沿用 07-08 设计，成员一律拥有团队基线，只能在其上叠加个人能力。
2. **同一能力只留一份，团队胜出**——同一 MCP/技能不得在团队与员工两个维度同时存在生效绑定；收敛在写时物理发生并可预览、可审计，而不是读时静默屏蔽（§5.2.1）。
3. **公共层的每一次写都要能回答"影响谁"**——团队级操作必须在提交前显示影响面（N 名数字员工），提交后落审计。
4. **归属可逆**——数字员工进出团队都是产品内可完成的常规操作，不需要改库。

---

## 4. 信息架构：观察面与配置面分离

```
/teams/$teamId            团队详情（观察面，只读为主）
  头卡：身份 / 状态 / 负责人 / 真实计数 / [配置团队] [删除团队]
  编制概览：人类成员头像栈 + 数字员工表（只读，行内「详情」）
  生效能力（只读）：技能 N / MCP N / 就绪度（M/N 成员可用）
  行为约束（只读）：宪法规则条目 + 最近一次修改人/时间
  最近变更：团队审计流前 5 条 → 「查看全部」

/teams/$teamId/config     团队配置（唯一写入口，与 employees/projects 对齐）
  Tab 1 编制    人类成员（增/删/改角色/高权限申请）· 数字员工（收编/移出/换队）
  Tab 2 能力    技能基线 · MCP 基线 · 成员就绪矩阵
  Tab 3 约束    团队宪法（结构化规则编辑 + diff + 历史）
  Tab 4 身份与生命周期  名称/描述/图标/负责人 · 危险区（删除团队）
  Tab 5 审计    团队变更流水（脆数据面）
```

**D7 已定：独立配置页**（与 employees / projects 对齐）。理由：写操作集中后，权限门禁、离开拦截、草稿、影响面预览、审计回显都只需一处实现；详情页可以变成真正"扫一眼就知道这个团队是什么状态"的观察面。头卡的「配置团队」按钮从打开 Sheet 改为 `Link to /teams/$teamId/config`（TanStack Router `Link`，不用原生 `<a>`）。

容器遵循 `DESIGN.md`：配置页外壳 Soft Card；成员表/员工表/就绪矩阵/审计流为脆数据面（实底、`tabular-nums`、等宽 id）；不使用玻璃卡（配置页是高密度写操作面）。

---

## 5. 分区详细设计

### 5.1 Tab 1 · 编制

**人类成员**（在现有 Sheet 基础上升格为页内分区）
- 表格列：成员（`UserIdentity`）· 直接角色 · 加入时间 · 操作。
- 新增「改角色」：`member ⇄ viewer` 直接生效；升到 `owner/admin/approver` 走 `requestTeamPrivilegedRole` → 权限中心审批。
  - 需要新增路由 `PATCH /teams/{teamId}/members/{memberId}`（`team.member.change_role` 常量已存在，只差路由/handler/service）。
  - 这条与 TODO.md `2026-07-25 团队人类成员普通角色变更 UI + 高权限申请接线` 是同一件事，本设计将其收编，不重复立项。
- 移除最后一名 owner 已有服务端保护（`tenant/service.go:511`），前端需前置禁用并说明。

**数字员工**（本设计的核心补齐）
- 表格列：员工 · 职能 · 状态 · **在役任务数** · 操作（详情 / 移出 / 换队）。
- **收编**：保持现有下拉，但改为搜索选择（候岗员工可能很多），并显示"本团队 X/上限 Y"。
- **移出团队 → 回候岗大厅**：
  - 新增 `DELETE /api/v1/teams/{teamId}/digital-employees/{employeeId}`，语义"解除归属"，落 `team_id = NULL`。
  - 选择新增专用端点而非让 `PUT /digital-employees/{id}/team` 接受 `null`：删除语义用 DELETE 表达更准确，且团队维度端点才能落 `resource_type=team` 的审计。
  - 门禁：`team.update`（与收编对称）。
  - 守卫（服务端硬拒，非仅前端提示）：
    - 员工有 `running` / `dispatched` 任务 → 409 + 明细（用户可先取消或等待）。
    - 员工是任一非终态项目的成员 → 409 + 项目名清单（无团队归属的员工不能参与项目，静默移出会让项目挂起）。
  - 二次确认弹窗必须写明：员工进入候岗大厅、失去本团队技能/MCP 继承（列出将失去的项）、下次派发家目录重算。
- **换队**：复用 `PUT /digital-employees/{id}/team`，补三项服务端修复：
  - 调用 `ensureTeamDigitalEmployeeCapacity` 校验目标团队限额。
  - 与移出同一套在役/项目守卫。
  - 双边权限：源团队 `team.update` **且** 目标团队 `team.update`。
  - 审计改为写两条 `resource_type=team` 事件（源队 `team.digital_employee.transfer_out`、目标队 `.transfer_in`），保留现有 `digital_employee` 维度事件。

### 5.2 Tab 2 · 能力

**技能基线**
- 已装列表每行补：来源（市场/上传）· 版本 · 风险 · **依赖的 MCP** · **依赖的 runtime 工具/env** · 安装时间与安装人 · 就绪状态。
- 就绪状态口径（团队级新语义）：对全队成员求交集——
  - `就绪`：所有成员都能加载。
  - `部分就绪 M/N`：M 名成员满足依赖，点击展开缺哪几名成员、缺什么。
  - `未就绪`：无人满足（典型：依赖的 MCP 团队没绑）。
  - 数据来源：复用 `EvaluateEmployeeSkillMCPDependencies`，新增团队聚合端点 `GET /teams/{teamId}/capability-readiness`。
- **安装前影响面预览**：抽屉里选中技能后显示"将对本团队 N 名数字员工生效"，以及依赖预检结果（缺 MCP 时给一键跳转到 MCP 分区）。
- **移除前影响提示**：列出会失去该技能的成员（已在个人层单独安装的成员标注"不受影响"）。
- 高风险技能（`risk_level=high`）安装是否走审批 → §9-D3。

**MCP 基线**
- 每行补：URL · 必需环境变量 · **凭据来源** · 成员配齐情况 `M/N` · 状态。
- 绑定表单补必需环境变量清单与预检，不再只写一句免责说明。

**成员就绪矩阵**（新增，本 Tab 的关键补齐）
- 行 = 团队数字员工，列 = 团队基线里每个需要 env 的 MCP / 技能，格 = 已配 / 缺失。
- 缺失格可直接就地补该员工的 env（复用现有 `upsertEmployeeEnvironmentVariable`），不必逐个跳员工页。
- **不新增团队级环境变量值存储**（D2 已定）：维度划分保持现状——注册表定义 `required_env_vars`，团队/员工绑定各自声明 `credential_env_var`，**值只存在员工级**。就绪矩阵是让人从团队页一处把值补齐的操作面，不改数据模型、不把凭据降级为团队共享。

### 5.2.1 同一能力的团队/员工冲突：团队接管收敛（D2 已定）

**规则**：同一 `mcp_server_id`（技能同 `skill_id`）在团队与员工两个维度**只允许存在一条生效绑定，团队维度胜出**。收敛在**写时物理发生**，不再靠读时屏蔽。技能与 MCP 适用同一套规则，消除现有的行为不一致。

四个方向的目标行为：

| 方向 | 目标行为 |
|---|---|
| **团队绑定，成员已有个人副本** | 同一事务内软删所有本团队成员的同 server/skill 个人绑定（**接管**），写审计，接口返回被收敛清单 |
| **员工绑定，团队已提供** | **409 拒绝**，错误体点明"该能力已由团队《名称》提供，无需重复绑定"；技能侧同步把现有的 `AlreadyBound=true` 静默成功改为显式返回该语义（前端必须显示原因，不得表现为"已装上"） |
| **团队解绑** | 不复活任何东西（个人副本已在接管时物理清除）。确认弹窗提示"N 名成员将失去此能力，需要保留的请到员工配置页单独绑定" |
| **员工换队 / 被移出团队** | 换队后按新团队基线重算；旧团队接管过的个人绑定**不回滚**（已物理删除），确认弹窗需写明这一点 |

**接管必须先预览再执行**。团队能力 Tab 的绑定确认弹窗展示：
- 将接管 N 名成员的同名个人绑定；
- 其中 M 名成员原先使用**不同的凭据变量名**（逐条列出 `员工名 · 原变量名 → 团队变量名`）——这批人接管后会立即变成"缺环境变量"，弹窗内直接提供批量补值入口（就绪矩阵的内联形态），让"接管 + 补齐"在一个流程里完成；
- 提交后落审计 `team.mcp.takeover` / `team.skill.takeover`，details 记录被收敛的 `(employee_id, previous_credential_env_var)` 列表，可回溯。

**凭据值不自动搬运**：接管只统一"用哪个变量名"，不把员工原变量名下的值复制到团队变量名下。理由：原变量可能被其他 MCP/技能共用，自动复制会让凭据来源变得不可解释；代价（一批成员立刻缺值）通过上面的预览与内联补值消化，不藏。

**读路径的 `NOT EXISTS` 去重保留**为防御性兜底（历史数据、并发写窗口），但正常态下不应再有可屏蔽项；可加一条一致性巡检指标：存在被屏蔽的个人绑定 = 收敛漏网，应告警而非静默。

**历史数据一次性收敛**（D10 已定）：当前是开发环境，迁移直接把所有"团队已绑同 server/skill"的员工个人绑定软删并记审计即可，**不需要 dry-run 预审**。收敛后 `NOT EXISTS` 兜底应查不到任何可屏蔽项——这是迁移的验收判据。

### 5.3 Tab 3 · 约束（团队宪法）

**D1 已定：接通。** 团队宪法必须真正进入执行链，否则不保留该字段。设计如下：

- **编辑形态**：从裸 textarea 改为规则条目列表。每条规则 = `{ id, text, category }`，`category ∈ {禁止, 必须, 需审批}`（服务端注册校验，不做封闭枚举硬编码在前端）。保留"批量粘贴"入口做迁移。
- **保存流程**：编辑 → 「预览变更」显示 diff（新增/删除/修改逐条）+ 必填变更说明 → 提交。
- **版本化**：新增 `team_constitution_revisions`（`tenant_id, team_id, revision_number, rules jsonb, change_note, created_by, created_at`），`tenant_teams.constitution` 保留为当前生效快照（读路径不变，避免全仓改读）。支持查看历史与回滚（回滚 = 以旧内容创建新版本，不改历史）。
- **真接通执行链 —— D9 已定：只作约束文本注入，不参与门禁判定**：
  - `run_service.go` 组装派发 payload 时，按 `employee.team_id` 读团队当前宪法，与员工 `persona_memory_markdown` 合成注入，团队规则在前、员工人格在后，并在 payload 里标注来源（`constitution_source: {team_id, revision_number}`）便于执行轨迹归因。
  - 派发记录里存下当次生效的 `revision_number`，使"这条任务当时受哪版宪法约束"可回溯。
  - 长度预算：规则总字符数上限进系统配置中心（复用 `systemconfig` 注册表模式），超限拒绝保存并提示。
  - **明确不做**：`category=需审批` 的规则**不触发**任何强制审批点，团队宪法不进入 `planTouchesHighRisk` 一类的调度判定。`category` 本期只是给人类看的分类与提示词组织方式。让宪法参与门禁判定需要判别子设计 + 对抗测试，风险与误伤面都大，另行立项。
- **审批**：编辑是否需要 `team.governance.approve` → §9-D4。

### 5.4 Tab 4 · 身份与生命周期

- 名称 / 描述 / 图标（复用 `TeamIconPicker`）/ 负责人集合（至少 1 人，沿用项目负责人 any-of-N 的约束表达）。接线已有的 `PATCH /teams/{teamId}` + `updateTeam` 客户端。
- 危险区：删除团队（沿用现有 pending_delete 两段式流程与文案）。

### 5.5 Tab 5 · 审计

- 接线已有 `GET /teams/{teamId}/audit`，脆数据面呈现：时间 · 操作人 · 动作（经 `status-labels.ts` 中文化）· 对象 · 详情展开。
- 服务端补写缺失事件并**统一为 `resource_type=team`**（成员相关事件同时保留 `team_member` 维度写入，或改为团队维度写、`details` 里带 membership_id → §9-D5）。
- 需要补写的事件：`team.update`、`team.constitution.update`、`team.member.remove`、`team.member.change_role`、`team.skill.bind` / `.unbind`、`team.mcp.bind` / `.unbind`、`team.digital_employee.unbind` / `.transfer_out` / `.transfer_in`。

---

## 6. 后端改动清单

### 6.1 缺陷修复（与新功能解耦，可先行）

1. `queries/tenant_team_config.sql:54,132`：`capability_count` 改为真实统计（`team_skill_bindings` + `team_mcp_bindings` 的 LATERAL 计数）；`pending_draft_count` 处理见 §9-D6。
2. `overviewActions`（`tenant/handler.go:477`）补 `authz.ActionTeamCapabilityManage`。
3. Web：`team-capabilities-tab.tsx` 的 `canEdit` 拆成 `canBindSkill` / `canUnbindSkill` / `canManageMcp`，分别取对应 action，不再复用 `team.governance.edit`。
4. `employee/service.go` `ReassignTeam` 补容量校验。
5. 团队技能/MCP bind/unbind 补审计写入（`skill/pg_repository.go:216/235`、`capability` 对应位置）。

### 6.2 新增（契约 + Go + 迁移）

| 项 | 契约 | Go | 迁移 |
|---|---|---|---|
| 移出团队 | `DELETE /teams/{teamId}/digital-employees/{employeeId}` | tenant handler/service/repo | 无 |
| 换队守卫 | 复用现有路径，补 409 错误码 | employee service | 无 |
| 成员改角色 | `PATCH /teams/{teamId}/members/{memberId}` | tenant | 无 |
| 团队能力就绪 | `GET /teams/{teamId}/capability-readiness` | capability（聚合现有 evaluate） | 无 |
| 宪法版本 | `GET /teams/{teamId}/constitution/revisions`、`{revisionId}` | tenant | 新表 `team_constitution_revisions` |
| 宪法注入 | payload 字段 `constitution_source` | `run_service.go` | 无 |
| MCP/技能接管收敛 | 绑定响应加 `converged[]`；员工侧绑定加 409 | capability + skill service/repo | 一次性收敛历史数据（直接执行，D10） |
| 接管预览 | `GET /teams/{teamId}/capability-conflicts?mcp_server_id=` 或 `?skill_id=` | capability + skill | 无 |

不新增 `team_environment_variables`（D2 已定：值只存员工级）。

契约改完必须 `corepack pnpm generate:control-plane` + `verify:contracts`；迁移改完更新 `atlas.sum` 并 `make -C apps/control-plane migrate-validate`。

---

## 7. 建议的实施分期

- **P0 · 止血与对齐**（不改信息架构，风险最低）：§6.1 五项修复 + 数字员工「移出团队 / 换队」接线（含服务端守卫与审计）。做完就解决了用户点名的"只能加不能踢"和"看到 0 能力绑定"。
  **状态：已实现（2026-07-26，分支 `feat/team-config-p0`，未合并）**，真实链路验证见 CHANGELOG 同日条目。实现中把守卫收进新包 `internal/teamguard`，移出与换队共用同一条 sqlc 查询与同一条错误消息。P0 未做：`pending_draft_count`（D6）留到 P1 与配置页一起（当前唯一产出方是特权角色申请，本身还没有 UI，接了也恒为 0）。
- **P1 · 配置页落地**：新建 `/teams/$teamId/config`，把编制/能力/身份/审计四个 Tab 迁入，详情页转为观察面。成员改角色 + 高权限申请接线并入此期。
  **状态：已实现（2026-07-26，分支 `feat/team-config-p0`，未合并）**，真实链路验证见 CHANGELOG 同日条目。实际落成五分区（另拆出「约束」）。一并做掉 D5（成员审计改 `resource_type=team`）与 D6（`pending_draft_count` 接真实待审批数），并补齐 `team.update` / `team.constitution.update` / `team.member.add|remove|change_role` 审计。
  **踩坑留证**：路由文件里额外 `export` 一个组件会关掉 TanStack Router 的自动代码分割，把整个 teams feature 拽进首屏 chunk（入口 86 KB → 229 KB，触发 bundle-size 门禁）；把逻辑内联进 `component` 即恢复。`employees/$employeeId.tsx` 目前是同一写法（为让测试 import），其详情代码很可能也在首屏包里——**待查独立项**。
- **P2 · 能力面板升格**：接管收敛（§5.2.1，含历史数据 dry-run 与迁移）、依赖预检、影响面预览、成员就绪矩阵。
  **状态：已实现（2026-07-26，分支 `feat/team-config-p0`，未合并）**，真实链路验证见 CHANGELOG 同日条目。落成：写时物理收敛 + 员工侧 409 + 接管预览端点 + 就绪矩阵端点与 UI + 一次性收敛迁移 `20260726021028`。
  **未做**：技能维度的就绪聚合（当前矩阵只覆盖 MCP 的 env 缺失；技能的 runtime 依赖状态仍只在员工页可见）与技能侧的接管预览端点（技能接管已在写时执行并落审计，但绑定前没有预览弹窗）——两项都属于 §5.2 的"依赖预检"细化，留待触达时补。
- **P3 · 宪法接通**：结构化规则 + 版本化 + 派发注入 + token 预算，并把 `governance_status` 判据改成"有无生效规则"。
  **状态：已实现（2026-07-26，分支 `feat/team-config-p0`，未合并）**，真实链路验证见 CHANGELOG 同日条目——真实 Claude Code 跑出宪法里约定的标记 `CONSTITUTION-OK-7731`，证明宪法确实进了 provider 提示词而非只落 payload。
  **实现中发现并修正的前提错误**：员工 `persona_memory_markdown` 在 runtime 侧同样是死字段（`payload.rs` 有字段、全仓无读者，provider 只拿 `prompt`）。本设计 §1.4 原先只指出团队宪法空转，实际是"团队宪法与员工人格都没接到 provider"；P3 的注入在 runtime `provider_prompt()` 一并接通两者。
  **未做**：`governance_status` 判据仍是 `constitution = '{}'`（未改成"有无生效规则"）；D4（删除既有规则需审批）未实现——它需要接权限中心，是独立一层治理，本期只落"接通 + 版本化 + 预算"。

每期的完成条件遵循 CLAUDE.md：真实 Web + Control Plane + DB + Runtime 端到端验证，不以单测/构建通过替代。

---

## 8. 明确不做

- 不恢复 `tenant_team_config_revisions` 式的团队治理沙盒（07-08 已废，approval/context/runtime/provider 白名单不回团队）。
- 不给团队增加 provider / runtime / 项目维度的发言权。
- 不引入团队级"外部能力"绑定（07-08 已明确移出范围）。

---

## 9. 抉择记录与剩余待定项

### 9.0 已拍板（2026-07-26）

- **D1 · 团队宪法 = 接通**。结构化规则 + 版本化 + 派发时注入执行链（§5.3）。不接通就删字段，不保留"存了不用"的中间态。
- **D2 · 能力冲突 = 团队接管收敛**（§5.2.1）。同一 MCP/技能在团队与员工两个维度只保留一份，团队胜出，员工重复项写时物理收敛。维度划分不变、**不新增团队级环境变量值存储**（值仍只在员工级），团队页通过成员就绪矩阵补齐。
- **D7 · 信息架构 = 独立 `/teams/$teamId/config` 页**，与 employees / projects 对齐（§4）。
- **D9 · 宪法作用形态 = 只作约束文本注入**（§5.3）。不参与门禁判定，`category=需审批` 不触发强制审批点；"宪法参与调度判定"另行立项。
- **D10 · 历史数据 = 直接收敛**。当前是开发环境，允许中途的数据变更/删除，迁移不需要 dry-run 预审，以终态正确为准（§5.2.1 末段）。

### 9.1 剩余项的选项说明

> 这几项都有可接受的默认取值（见 §9.2），不阻塞开工；此处保留选项与权衡，便于人类推翻。

**D3 · 高风险技能安装到团队要不要审批？**
一次安装 = 给全队开权限。是走 `team.capability.bind` 直接生效（现状），还是 `risk_level=high` 时强制走权限中心审批（`team.governance.approve`）？后者与已落地的权限中心/员工权限变更审批模式一致。

**D4 · 团队宪法编辑要不要审批？**（D1 已定接通，此项变为必答）
选项：① 编辑即生效（`team.governance.edit`）；② 编辑产生草案，需 `team.governance.approve` 才生效；③ 仅"删除既有规则"需要审批，新增/收紧不需要（推荐——放松约束是风险动作，收紧不是）。

**D5 · 团队审计流的口径**
`/teams/{id}/audit` 现在按 `resource_type='team'` 过滤，成员事件写在 `team_member` 上看不见。
- 方案 A：成员事件改写 `resource_type=team`，membership_id 进 details（团队审计流完整，但按成员维度查审计会丢一个索引口径）。
- 方案 B：端点改为按 team_id 跨资源类型聚合查询（需要审计表按 details 里的 team_id 建索引或加冗余列）。

**D6 · 头卡「待审批 N」pill 保留还是删除？**
`pending_draft_count` 原本指团队治理草案，该概念已随 07-08 退役，现在恒 0。
- 方案 A：改为"与本团队相关的待处理审批数"（权限中心里 resource=本团队的 pending 项）。
- 方案 B：删掉这个 pill 与字段。

**D8 · 移出团队的守卫强度**
员工有在役任务 / 是活跃项目成员时：① 硬拒 409（本设计默认）；② 允许但强二次确认并记录风险；③ 有在役任务硬拒、只是项目成员则警告放行。

### 9.2 实现方默认取值（未被否决即按此实施）

D3–D6、D8 属于可在对应分期开工前定的项。为避免阻塞，实现按以下默认值推进；人类随时可推翻，推翻则更新本节。

| 项 | 默认取值 | 理由 |
|---|---|---|
| D3 高风险技能装到团队 | **走审批**（`risk_level=high` 时经权限中心，`team.governance.approve`） | 一次安装＝给全队开高风险能力，与已落地的员工权限变更审批模式一致 |
| D4 宪法编辑 | **仅"删除既有规则"需审批**，新增/收紧直接生效 | 放松约束是风险动作，收紧不是；避免把日常补规则做成重流程 |
| D5 团队审计流口径 | 成员事件改写 `resource_type=team`，membership_id 进 `details` | 改动小，团队审计流一次补全；成员维度查询目前没有调用方 |
| D6 头卡「待审批 N」 | 改成"与本团队相关的待处理审批数"（权限中心 resource=本团队的 pending） | 该 pill 位置本身有价值，删掉不如接真数据 |
| D8 移出团队守卫 | **硬拒 409**：有 `running`/`dispatched` 任务，或是任一非终态项目成员 | 静默移出会让项目挂起；错误体给出明细，人先处理再移出 |
