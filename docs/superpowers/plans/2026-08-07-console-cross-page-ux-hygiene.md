# 控制台跨页体验共性问题整改方案

> 状态：完整关闭（MVP+骨架+人机词+PR4a+1.B 盘点）；PR4b/Loading backlog 见 TODO.md  
> 日期：2026-08-07  
> 范围：`apps/web` 为主；读路径补名 / summary 写端按盘点结果可选触及 `apps/control-plane` + `contracts/`  
> 依据：DESIGN.md「面向用户文本与枚举显示」；代码核证（收件箱 / 技能详情·列表 / 运行总览 / 侧栏 / 路由 title / ObjectRef）  
> 修订：纳入方案评审结论——D3↔ObjectRef↔DESIGN 对齐、1.B 数据驱动可选、summary 生产者分批、加载态 in-scope 边界、人机词 surface 扩表、触点补全

**Goal:** 消除跨页共性问题中已核证的 P0/P1，并把 P2 人机状态词收敛进现有 `status-labels.ts` 事实源，避免再长出第三套词表。

**非目标：**

- 不重做收件箱/技能/运行总览信息架构
- 不改后端业务状态机语义，只改用户可见展示与读路径补名
- 不把治理页（角色词表、场景模板编辑）的技术字段（`role_key` 编辑器）强行中文化——那些是配置标识面，不是工作面
- 不本轮扫清全站所有 `LoadingState`（约 40+ 处）；只改 §3.3 / §4 Batch 3 的 **in-scope** 页，其余进 Explicit backlog
- 不强制迁移历史 inbox `summary` 快照；只约束新开单写端
- 不另起平行词表或 i18n 体系

**与近期并行工作的边界：**

- 能力投影 / MCP 绑定等新 UI **同样遵守**本方案 P0 指称与工作面中文规则；护栏扫描默认覆盖新工作面组件，不另开豁免。
- 并发会话若同改 `status-labels.ts`、`CHANGELOG.md`、生成物：只改必要键/函数，显式路径 `git add`，禁止整文件无关 reformat。

---

## 0. 范围切割（In scope / Backlog）

| 层级 | 内容 | 交付 |
|---|---|---|
| **MVP 止血** | PR1 护栏 + DESIGN；PR2 前端指称/ref meta/Provider；侧栏「技能市场」；`usePageTitle` | 用户可见痛点立刻下降，不绑 CP |
| **体验完整** | MVP + Batch 3 限定骨架 + Batch 4 人机词 | 导航/加载/标签/状态词一致 |
| **数据驱动可选** | Batch 1.B 补名（仅当盘点证明 enrich 缺口） | 可能空 PR 或仅契约注释 |
| **写端分批** | Batch 2.4 summary（按 kind 清单分 1–2 个 PR） | 新开单中文摘要 |
| **Explicit backlog** | 审计/四类日志/系统配置/角色词表/Runtime 页的 LoadingState；全站枚举漏扫；历史 summary 迁移脚本 | 不阻塞本方案关闭 |

---

## 1. 问题清单与核证结论

| 优先级 | 问题 | 核证 | 主因（代码落点） |
|---|---|---|---|
| P0 | 对象指称裸 UUID | **存在** | 收件箱 `formatContext` 名缺失时 ``项目 ${source_project_id}``；meta 行整段 `font-mono` 包业务文案；技能绑定 `team_name \|\| team_id`；项目绑定 `meta: project_id`；员工详情 blocker ``project ${project_id}``；技能列表旁全量 `team_id` mono |
| P0 | 英文技术词/枚举泄漏到工作面 | **存在** | ① 关联 ref `meta: "demand_id ↗"` 等字段名直渲 ② `item.summary` 服务端快照原样展示 ③ 运行总览 `providerType` 未走 `providerDisplayName` ④ 局部状态/键未进词表 |
| P1 | 导航命名 vs 页内标题不一致 | **存在** | 侧栏 `技能管理`（`sidebar-data.ts`）vs 页 H1/返回链与员工/项目入口多为 `技能市场`；`shell-background.test.tsx` 等写死「技能管理」 |
| P1 | 路由切换全页「加载中」闪白 | **部分存在** | 技能详情/运行总览/流程实例/团队首屏等整段 `LoadingState`；列表页已有 Skeleton 未统一；全站 40+ 处不在本轮全清 |
| P1 | document title 不随路由变化 | **存在** | `index.html` 静态「炬枢平台」；路由无 `head`/`document.title`（有 `NavigationProgress`，与 title 无关） |
| P2 | 同一人机等待概念多套词 | **存在且被测试固化** | 收件箱「待你/待我处理」、运行总览「待确认/待人工/待人工处理」、员工「待人工确认」、自动化「待你处理」、项目「待决决策/待我决策」、词表 `waiting_human→等待人工` |

规范已写在 `DESIGN.md` §面向用户文本；本方案是**补齐落地与收敛缺口**，不是新规范。

### 1.1 已知张力（实施前必须拍板，默认如下）

| 张力 | 现状 | 本方案默认（D3） |
|---|---|---|
| 名缺失回退 | DESIGN 写「回退显示 id」；`ObjectRef` 无名称 → **全量 UUID** chip | 统一为 **`未命名{类型} (短id)`** + 可复制全 id；同步改 DESIGN + `ObjectRef`/配套 helper |
| `shortId` | `object-ref.tsx` 内私有；员工/用户等各有本地拷贝 | **export 唯一 `shortId`（或经 `missingObjectLabel`）**，禁止页面再抄第三套 |
| Skill 空名 | 后端 `list*Bindings` 已 JOIN 补名；空名 ≈ 源删/名空 | **以前端回退为主**；不默认开「补 join」PR |
| Inbox 空名 | 已有 `enrichSourceNames` + 测例 | **先盘点原因**（源删 / 无 project id / 旧数据）；enrich 缺口才改 CP |

---

## 2. 设计原则（整改必须遵守）

1. **名称主、标识辅**  
   用户可见主文本 = 对象名称；标识仅 `ObjectRef` / 短 id chip / 技术详情区 mono。禁止 `font-mono` 包一整段「名称+全 UUID」。
2. **名缺失统一回退（D3）**  
   展示文案：`未命名{类型} (短id)`；全 id 仅通过 chip 点击复制或 title。`ObjectRef` 无名称时也应走该语义（可 `kind` 可选参数或旁路 `missingObjectLabel` + chip），**禁止**业务列表主文本直接甩 36 位 UUID。
3. **读路径补名，前端不 N+1**  
   列表/详情 API 批量附 `*_name`；空串与 null 同等视为缺失。前端不逐行请求解析名称。
4. **词表唯一事实源**  
   状态/枚举/关联类型/ref meta / 人机等待 → `apps/web/src/lib/status-labels.ts`（及已有域函数如 `operational-status.ts`，触达时收敛到词表函数）。禁止页面内新建平行映射。
5. **工作面 vs 配置面**  
   - 工作面（收件箱、运行总览、项目驾驶舱、技能使用/安装态）：禁止 API 字段名、snake_case 枚举原文、provider slug。  
   - 配置/治理面（角色词表、场景模板 key、技能档案 **slug**）：可展示稳定技术标识，须中文标签托底（如 `角色标识` / `技能标识`）。
6. **摘要中文化优先写端**  
   `inbox.summary` / `SummarySnapshot` 是写入快照；**正确做法是生产者写中文**。前端默认**不做**整句 regex 改写（D2）。
7. **加载保留壳（有界）**  
   In-scope 页：Shell 头始终挂；`isPending && !data` 用 `TableSkeleton` / `CardGridSkeleton` / `DetailSkeleton`；有 `placeholderData`/`keepPreviousData` 时保留旧内容。  
   与 `patterns.tsx` 一致：未知布局可用 `LoadingState`；**已知布局禁止整树单行「加载中…」撑满 Main**。本轮不扫 backlog 页。
8. **Provider 显示名**  
   统一 `providerDisplayName`（`provider-label.ts` 唯一源；成本页删除重复 `PROVIDER_LABELS`）。品牌专名可保留英文。禁止工作面直渲 `claude-code`。
9. **新工作面默认纳入护栏**  
   能力投影、MCP、项目绑定等新增组件不另开例外。

---

## 3. 目标体验（验收口径）

### 3.1 对象指称

| 场景 | 合格 | 不合格 |
|---|---|---|
| 收件箱列表 meta | `项目名 · 计划确认` 或 `未命名项目 (25a6b54b) · 计划确认`（业务文案非 mono） | `项目 25a6b54b-xxxx-…` 全 UUID；整行 mono 业务文案 |
| 收件箱关联对象 | `关联项目 · 名称` + meta「打开项目」 | meta `source_project_id ↗` |
| 技能绑定（详情/列表） | `默认团队` 或 `未命名团队 (00000000)` + 可选短 id chip | 主名位置整段 UUID；列表旁全量 id 当主信息 |
| 员工 blocker 等 | `未命名项目 (短id)` 或项目名 | ``project ${uuid}`` |
| 任意业务列表 | 名称；悬停/复制拿全 id | 主列裸 UUID |

### 3.2 文案与枚举

| 场景 | 合格 | 不合格 |
|---|---|---|
| 收件箱摘要（新开单） | 中文业务句 | 正文出现 `role_key=` / `waiting_human` / `software_delivery` 未映射 |
| 关联 ref meta | 中文动作/类型（打开需求、任务、审计） | `demand_id ↗`、`task_title` |
| 运行总览员工卡 | `Claude Code` | `claude-code` |
| 人机等待（跨页） | 见 §4 Batch 4；同视角一词 | 同屏无说明混用「待确认」「待人工」「等待人工」「待人工处理」 |

### 3.3 导航 / 加载 / Title

| 场景 | 合格 |
|---|---|
| 技能入口 | 侧栏与 H1/返回链/aria/测试 **同一词「技能市场」**（D1） |
| 路由切换（in-scope 页） | 侧栏+顶栏+页头保留；内容区骨架；**不是** Main 内单行 LoadingState 白闪 |
| 浏览器标签 | `炬枢 · 收件箱` / `炬枢 · 技能市场` / 详情 `炬枢 · {对象名}`（名异步到达后更新） |

### 3.4 可观察完成条件（避免空泛「无白闪」）

- 骨架：in-scope 页在 `isPending && !data` 时，DOM 中存在 Skeleton 结构（或等价占位块），且 `ShellPageHeader`（若该页有）仍在。  
- P0 指称：列表/meta 主文本 **不匹配** 36 位 UUID 主串（允许短 id 段）。  
- Title：进入 `/inbox` 后 `document.title` 包含「收件箱」。

---

## 4. 分批实施计划

### Batch 0 — 基线、规范对齐与护栏（0.5d）

**目的：** 防止改完再回潮；消掉 D3 与现状张力。

- [ ] **0.1** 在 `DESIGN.md`「面向用户文本」补可执行细则（短段即可）：  
  - 名缺失回退：`未命名{类型} (短id)` + 可复制全 id（**替换**旧「回退显示 id」表述，与 D3 一致）；  
  - 关联 meta / kicker **禁止 API 字段名**；  
  - 加载态：已知布局禁止整树 `LoadingState` 替换内容（并指向 patterns 骨架）；  
  - document title：`炬枢 · {页名}`，详情可带对象名；  
  - 人机等待：指针到 `humanWaitLabel` / 本方案 Batch 4（一句即可）。  
- [ ] **0.2** `ObjectRef` / helper 对齐 D3：  
  - export `shortId` 或仅 export `missingObjectLabel(kind, id)`（推荐后者为文案源、chip 仍可复制全 id）；  
  - 无名称时主文本不再默认「仅全 UUID」（可用 `未命名…` + full chip，或 kind 缺省时保留 full chip 但业务调用方必须传 kind/missing label）。  
  - 实施时选一种 API，**全站业务列表统一走它**。  
- [ ] **0.3** 扩展 `status-labels.guard.test.ts`（能自动化的）：  
  - 扫描禁止作为 **JSX 可见文本** 的字面量：`"demand_id ↗"`、`"source_task_id ↗"`、`"source_project_id ↗"`、`"source_approval_request_id ↗"`，以及裸 meta 形态的 `"task_title"`（注意勿误伤对象字段访问）；  
  - **Allowlist**（误报）：`role-vocabulary/**`、场景模板编辑、inbox 动作弹窗字段表（若展示「需求 ID」类配置标签）、测试文件；  
  - glob 至少覆盖 `src/features/**/*.tsx`；若有余力加 `src/routes/**/*.tsx`；  
  - `providerType` 直渲：静态扫不稳则 **不硬撑**，靠 0.4 定向单测 + 人工 `rg`。  
- [ ] **0.4** 定向单测夹具：  
  - 收件箱 meta 名缺失 → 不含 36 位 UUID 主串；  
  - 技能 binding 空名 → `未命名…`；  
  - 运行总览员工卡 provider slug → 显示名。  

**完成定义：** DESIGN/ObjectRef 与 D3 一致；护栏/夹具红绿可复现。

---

### Batch 1 — P0 对象指称

#### 1.A 前端展示收敛（先做，立即止血，~0.5–1d）

- [ ] **1.1 收件箱列表 meta**（`inbox-item-list.tsx` `formatContext`）  
  - 名缺失：`missingObjectLabel("project", id)`，**禁止** ``项目 ${fullUuid}``。  
  - meta 行去掉「整段 mono 包业务文案」：业务名常规字重；仅短 id 用 mono chip。  
- [ ] **1.2 收件箱详情关联区**（`inbox-shell.tsx` `buildRelatedReferences`）  
  - `label` 名称优先；无名称用 `未命名…(短id)`，禁止 label 尾部全 UUID。  
  - **删除**字段名 meta；改为：  

    | 原 meta | 新 meta |
    |---|---|
    | `demand_id ↗` / `demand` | `打开需求`（有 href）/ `需求` |
    | `task_title` | `任务` |
    | `source_task_id ↗` | `打开任务` |
    | `source_project_id ↗` | `打开项目` |
    | `source_approval_request_id ↗` | `打开审批` |
    | `审计` | 保留 |

  - 映射进 `status-labels.ts` 的 `relatedRefMetaLabel()`（推荐）或 inbox 单文件导出，**键集中一处**。  
  - meta 行若仍用 mono，仅限短动作文案可接受；**禁止** mono 字段名。  
- [ ] **1.3 技能详情 + 列表绑定**（`skills/detail.tsx`、`skills/index.tsx`）  
  - 名：`trim` 后空 → `missingObjectLabel`；**禁止** `name: id`。  
  - 项目绑定 **禁止** `meta: project_id` 全 UUID；meta 用「项目绑定」等中文。  
  - 列表行旁全量 `team_id`/`agent_id`：改为短 id chip 或去掉主信息位。  
- [ ] **1.4 员工等同类泄漏**（同 PR 顺手，避免漏 P0）  
  - `employees/detail.tsx` blocker：``project ${id}`` → 项目名或 `missingObjectLabel("project", id)`。  
  - 审计/日志/执行轨迹 mono 全 id **豁免**。  
  - 配置面员工 id 行（`employees/config.tsx`）可标豁免，不强制改。  
- [ ] **1.5 通用**  
  - 一律复用 `missingObjectLabel` / `ObjectRef`；禁止再复制 `shortId`。  

#### 1.B 读路径补名（**数据驱动可选**，默认 0–0.5d）

> **不要默认按「缺 join」施工。** 多数路径已有批量补名。

- [ ] **1.6 空名盘点（必做，短）**  
  - **Inbox**：`enrichSourceNames` 已存在。查真实/dev 数据：空 `source_project_name` 是源删、未挂 project、还是 enrich 未调用？补测仅当发现回归缺口。  
  - **Skills**：`listTeam/Agent/ProjectBindings` 已 SQL JOIN；空名以前端 1.A 闭环。  
  - **其他列表**：触达即同一 D3，不单独开大 PR。  
- [ ] **1.7** 仅当盘点证明 CP 缺口：读路径 batch 补名（inbox 先例），禁止 per-row；OpenAPI 注释「读时补名；缺失=来源不可用」。  
- [ ] **1.8** 若动契约：`verify:contracts` + CP 相关测试。  

**完成定义（1.A 必达；1.B 视盘点）：**

- 单测：名缺失不出现 36 位 UUID 作为列表/meta **主文本**。  
- 真链路：见 §7 造数；收件箱 ≥1 条、技能绑定（含默认团队）主文案为名称或「未命名·短id」。

---

### Batch 2 — P0 英文/枚举工作面泄漏

#### 2.1 关联 meta / UI chrome（前端，快）

- 见 1.2；与 Batch 1.A 同 PR。

#### 2.2 Provider 显示名（前端，快）

- [ ] **2.2.1** `run-overview/.../employee-detail-card.tsx`：`providerDisplayName(providerType)`。  
- [ ] **2.2.2** `rg` 工作面：`provider_type` / `providerType` 直渲 → `providerDisplayName`；`costs` 页删除局部 `PROVIDER_LABELS`。  
- [ ] **2.2.3** 日志/审计排障可保留 slug，标签「Provider 标识」，与显示名分行。

#### 2.3 状态枚举（词表，与 Batch 4 可衔接）

- [ ] **2.3.1** 盘点工作面仍直渲的枚举（guard + 人工扫：pill、运行总览 `status-maps.ts`、项目 KPI）。  
- [ ] **2.3.2** 缺键补 `status-labels.ts` + 测试；触达即删局部英文 map。  
- [ ] **2.3.3** 本批先保证**不出现英文原文**；`waiting_human` 中文收敛以 Batch 4 为准（本批可用临时中文，避免英文泄漏）。

#### 2.4 摘要（summary）中文化 — **写端分批，清单驱动**

> `SummarySnapshot` 分散在多处，**不是**「改 adapters 一处」就能关单。先清单后改。

**已知生产者区域（实施时补全表格行与样例）：**

| 区域 | 路径提示 | 优先 |
|---|---|---|
| Inbox 适配 | `inbox/adapters.go`（透传 decision summary） | 透传层；真正文案在上游 |
| 项目服务开单 | `project/service.go` 多处 `SummarySnapshot` | 高（人类决策） |
| 选角/扩编 | `project/casting_expansion.go` | 高（易含 role_key） |
| 协调器写决策 | `workflow/projectcoordination/project_store.go` | 高（计划确认/失败/验收等） |
| 预派发闸门 | `.../predispatch_gate.go` | 中 |
| 其他 | `rg SummarySnapshot` 全量扫漏 | 按命中补行 |

- [ ] **2.4.0** 产出《summary 生产者清单》：kind / 调用点 / 当前样例 / 目标中文句；**先高频**（计划确认、项目决策、分派失败恢复）。  
- [ ] **2.4.1** 按清单改模板：变量用已解析中文名；禁止 `role_key`/`branch_ref`/`software_delivery` 原文裸拼。若必须带 key：``建议角色：开发（developer）``。  
- [ ] **2.4.2** 前端：默认不做整句 regex；结构化 context 展示走词表。  
- [ ] **2.4.3** 存量 open 事项不强制迁移；新开单必须中文。dev 一次性脚本仅可选。  
- [ ] **2.4.4** 可拆 **PR4a（高频 kind）/ PR4b（长尾）**，避免巨型 PR。

**完成定义：**

- 清单内高频 kind：新开一条，摘要与关联区无 API 字段名、无未映射枚举原文。  
- 运行总览 Provider 为显示名。  
- 相关单测 + guard 绿。

---

### Batch 3 — P1 导航 / Title / 加载壳（1–1.5d）

#### 3.1 技能命名统一

**产品裁定（D1，默认直接做，与员工/项目入口已用「技能市场」一致）：**

- 统一 **「技能市场」**；侧栏 `技能管理` → `技能市场`。  
- 同步：`sidebar-data` 测试、`shell-background.test.tsx`、`rg "技能管理"` 工作面入口。  
- 未来治理向目录再拆二级，不在本批。

- [ ] **3.1.1** 改侧栏 + 相关测试。  
- [ ] **3.1.2** 全文对齐 H1/返回/aria。  
- [ ] **3.1.3** 技能详情 subtitle 的 **slug mono**：视为配置面标识，保留但确保旁有「技能档案」等中文托底（现状基本满足，勿误删）。

#### 3.2 Document title

- [ ] **3.2.1** 优先 `usePageTitle(segment: string)`（`useEffect` 设 `document.title`，卸载可恢复 fallback）。TanStack `head` 若已稳可用，**不阻塞**。  
- [ ] **3.2.2** 格式：`炬枢 · {页名}`；详情 `炬枢 · {名称}`（异步更新）。  
- [ ] **3.2.3** 认证壳主要路由：pathname 映射表 + 详情页覆盖（收件箱、任务中枢、运行总览、项目、员工、技能、团队、审计等）。  
- [ ] **3.2.4** 测试：`/inbox` → title 含「收件箱」。

#### 3.3 加载态：In-scope 保留 Shell + Skeleton

**本轮 In-scope（仅这些）：**

| 页 | 文件 |
|---|---|
| 技能详情 | `skills/detail.tsx` |
| 运行总览 | `run-overview/index.tsx` |
| 流程实例 | `task-launches/.../workflow-river-view.tsx` |
| 团队首屏（若仍整树 LoadingState） | `teams/index.tsx`（配置子页能改则改，子 tab 内局部 Loading 可留） |

**Explicit backlog（本方案不关单）：** 审计、logs/*、system-config、role-vocabulary、runtime、account、approvals 等。

- [ ] **3.3.1** 按上表改，模式：  

  ```
  <ShellPageHeader ... />  // 始终挂（若该页有）
  <Main>
    {isPending && !data ? <DetailSkeleton|CardGridSkeleton|TableSkeleton /> : content}
  </Main>
  ```

  - 刷新：有旧 data 时保留内容，勿整树替换。  
- [ ] **3.3.2** 禁止**新增**「仅 LoadingState 撑满 Main」的列表/详情首屏（可在 PR 描述/ DESIGN 一句约束）。  
- [ ] **3.3.3** 手点验证；截图可选。

**完成定义：** 侧栏=H1「技能市场」；in-scope 切页满足 §3.4；title 随路由变。

---

### Batch 4 — P2 人机状态跨页词表（0.5–1d，可与 2.3 合并）

#### 4.1 词表结构（两列视角，禁止同视角三词同义）

写入 `status-labels.ts` 注释 + 导出函数；DESIGN 挂指针。

| 语义 | 动作视角 | 对象视角 | 废弃/避免 |
|---|---|---|---|
| 当前用户有待办 | **待我处理**（收件箱 KPI/徽标） | — | KPI 不用「待你」 |
| 第二人称进度 | **待你**（仅进度条 label，`inbox` progress） | — | 不进 KPI；可不经 `humanWaitLabel` 但文档钉死 |
| 跨项目决策导流 | **待我决策**（项目 rail） | — | 审批+决策混用 KPI →「待我处理」 |
| 自动化导流切片 | **待你处理** 或收敛为 **待我处理**（实施时与产品二选一，**全自动化面统一**） | — | 禁止 automations 与 inbox KPI 无说明双词并存 |
| 员工运行态 | — | **待人工确认** | 「待确认」单用 |
| 运行总览短标签 | — | **待人工**（badge） | — |
| 运行总览 KPI 带 | — | **待人工** 或 **待人工处理**（**全 KPI 统一一词**，推荐与短标签一致用「待人工」，若保留「处理」则全局 KPI 都用它） | 禁止 KPI「待人工」与「待人工处理」混用 |
| 项目内未关闭决策对象 | — | **待决决策** | 「待确认决策」 |
| 枚举 `waiting_human` | — | 长：**待人工确认**；短：经 `humanWaitLabel` | 删除游离「等待人工」 |

**实现注意：**

- **不要**只改 `STATUS_LABELS.waiting_human` 一刀切：全局 `statusLabel("waiting_human")` 与员工卡长标签会再次分叉。用 **`humanWaitLabel(surface)` + 域函数 override**；全局键改为与长标签一致或标注「仅兜底，工作面禁止直调」。  
- `run-overview/status-maps.ts` 的「待确认」→ 并入员工长/短标签体系。  
- `data-display-kpi='待人工处理'` 等测试选择器随文案统一更新。

- [ ] **4.1.1** 实现 `humanWaitLabel(surface: ...)`（surface 见 §11）。  
- [ ] **4.1.2** 替换：`status-maps.ts`、`operational-status.ts`、项目 rail/KPI、运行总览 KPI/badge、automations 待办文案、词表相关键。  
- [ ] **4.1.3** 更新被文案钉死的测试期望。  
- [ ] **4.1.4** 不改后端枚举值。  

**完成定义：** 同语义在同视角/同 surface 一词；`verify:web` 全绿。

---

## 5. 推荐 PR 切片（降低共享工作树冲突）

| PR | 内容 | 风险 | 验证 |
|---|---|---|---|
| **PR1** | Batch 0：DESIGN + ObjectRef/D3 + 护栏 + 夹具 | 低 | 定向 `verify:web`；可走轻量验证 |
| **PR2** | Batch 1.A + 2.1 + 2.2 纯前端止血 | 低 | web 单测 + 真链路 §7 |
| **PR3** | Batch 1.B（**仅盘点有 CP 缺口时**） | 中 / 可空 | CP + 契约 + API |
| **PR4a/b** | Batch 2.4 summary 按 kind 分批 | 中 | CP 单测 + 新开决策摘要 |
| **PR5** | Batch 3 命名 / title / in-scope 骨架 | 低-中 | web + 手点路由 |
| **PR6** | Batch 4 人机词 | 低（测例多） | 全量 web 文案期望 |

可合并：PR1+PR2；PR5 中侧栏+title 可进 MVP。  
不要：1.B 与大前端文案同一巨 PR；summary 与词表巨揉。  
`status-labels.ts`：只加函数/改必要键，避免与能力投影等会话整文件冲突。

**CHANGELOG：** 每个用户可见行为变化的 PR 写一行（侧栏更名、title、摘要中文、人机词等）。

---

## 6. 文件级触点清单（实施索引）

### Web — 高优先级（MVP / P0）

| 文件 | 改动 |
|---|---|
| `DESIGN.md` | 名缺失 D3、meta、加载、title、人机指针 |
| `apps/web/src/components/superteam/object-ref.tsx` | D3 对齐；export `shortId` 或 `missingObjectLabel` 协作 |
| `apps/web/src/lib/status-labels.ts` + `*.guard.test.ts` | ref meta、humanWait、missingObject、护栏 |
| `apps/web/src/features/inbox/components/inbox-item-list.tsx` | `formatContext`；去整段 mono |
| `apps/web/src/features/inbox/components/inbox-shell.tsx` | related ref meta/label |
| `apps/web/src/features/skills/detail.tsx` | Binding 名/meta；骨架 |
| `apps/web/src/features/skills/index.tsx` | 绑定行 id 展示 |
| `apps/web/src/features/employees/detail.tsx` | blocker 项目指称 |
| `apps/web/src/features/run-overview/components/employee-detail-card.tsx` | `providerDisplayName` |
| `apps/web/src/features/employees/provider-label.ts` | 唯一 Provider 源 |
| `apps/web/src/routes/_authenticated/costs/index.tsx` | 删重复 PROVIDER_LABELS |
| `apps/web/src/components/layout/data/sidebar-data.ts` + 测试 | 技能市场 |
| `apps/web/src/styles/shell-background.test.tsx` 等 | 「技能管理」字面量 |
| `apps/web/src/hooks/use-page-title.ts`（新建）或 routes | document title |
| `apps/web/index.html` | fallback title 保留 |

### Web — Batch 3/4

| 文件 | 改动 |
|---|---|
| `apps/web/src/features/run-overview/index.tsx` | 骨架 |
| `apps/web/src/features/run-overview/status-maps.ts` | 人机词 |
| `apps/web/src/features/run-overview/components/*` KPI/badge | 人机词统一 |
| `apps/web/src/features/employees/operational-status.ts` | 人机词（与 humanWait 对齐） |
| `apps/web/src/features/task-launches/components/workflow-river-view.tsx` | 骨架 |
| `apps/web/src/features/teams/index.tsx` | 骨架（若 in-scope） |
| `apps/web/src/features/automations/components/*` | 「待你处理」与词表对齐 |
| `apps/web/src/components/superteam/patterns.tsx` | 通常只消费，少改 |

### Web — Explicit backlog（不本轮关单）

审计、logs/*、system-config、role-vocabulary、runtime、account 等处的整页 `LoadingState`；其他 mono 技术列。

### Control Plane（可选 / 分批）

| 区域 | 改动 | 触发条件 |
|---|---|---|
| inbox `enrichSourceNames` | 修缺口 + 测例 | 盘点证明 enrich 回归 |
| skill bindings | 通常 **不改**（已 JOIN） | 仅序列化空串语义需澄清时 |
| `SummarySnapshot` 生产者 | 中文模板按清单 | PR4a/b |
| OpenAPI 注释 | 补名语义 | 动契约时 |

---

## 7. 验证计划

### 分层门禁

```text
corepack pnpm verify:web              # PR6 全量；PR1–2 可定向
corepack pnpm verify:control-plane    # 若动 Go
corepack pnpm verify:contracts        # 若动 OpenAPI
```

### 真链路造数（避免「环境无数据」阻塞）

若环境缺少样例，开发会话需先具备其一：

1. **收件箱决策**：在已有项目上触发计划确认/人工决策（或使用 dev 已有 open inbox item）。  
2. **技能绑定**：打开已绑定默认团队（或任意团队）的技能详情；无绑定则先在技能/团队侧装一次。  
3. **运行总览员工**：至少一名带 `provider_type` 的数字员工。

步骤：

1. `scripts/dev-services.sh status` → web/CP 已加载**当前**代码（改后 `restart` 对应服务）。  
2. 收件箱：列表 meta、详情关联区；新开/已有决策看摘要（PR4 后查新开）。  
3. 技能：详情+列表绑定名；侧栏「技能市场」。  
4. 运行总览：员工卡 Provider 显示名；人机词（PR6 后）。  
5. 多标签 title；in-scope 页切换无整页单行加载撑满。  

### 完成门禁

- 走 `superteam-completion-check`。  
- P0 不得仅以单测绿关闭；需真链路证据（截图或 DOM/观察说明）。  
- PR1（仅 DESIGN/护栏）可轻量验证。  
- **本方案「完整关闭」** = MVP + PR5 in-scope + PR6 +（PR3 若需要）+（PR4a 高频 summary）；PR4b 与 Loading backlog 可留 TODO.md。

---

## 8. 风险与依赖

| 风险 | 缓解 |
|---|---|
| 历史 summary 仍是英文 | 不回溯；新开单中文；UI 不承诺改写历史 |
| 把 1.B 做成无效 join PR | 先盘点；技能默认 FE 回退 |
| summary 范围膨胀 | 清单 + PR4a/b；先高频 kind |
| 加载态无限扩 | §0 / §3.3 in-scope 硬边界 |
| 文案测试大面积失败 | Batch 4 独立 PR；统一改 expectation |
| `waiting_human` 全局键一刀切 | `humanWaitLabel` + 域 override |
| 共享工作树 / 并行能力投影 | 显式 `git add`；`status-labels` 最小 diff |
| 护栏误报配置/字段表 | allowlist 见 0.3 |
| 「技能市场 vs 管理」争议 | D1 默认市场；与现网多处一致，低翻转成本 |
| ObjectRef 行为变更影响面 | 单测 + 业务调用方统一 missing label；review 扫 ObjectRef 用法 |
| 真链路无数据 | §7 造数步骤 |

---

## 9. 工作量与顺序建议

```text
Day 0.5   人类精修本方案（尤其 automations「待你处理」vs「待我处理」、KPI 用「待人工」还是「待人工处理」）
Day 1     PR1 护栏/D3/DESIGN + PR2 前端止血（指称 + ref meta + provider + 技能列表/员工 blocker）
Day 1–2   PR5 侧栏 + title + in-scope 骨架（侧栏+title 可提前进 MVP）
Day 2     空名盘点 → PR3 仅在必要时
Day 2–3   PR4a 高频 summary；PR4b 可顺延 TODO
Day 3     PR6 人机词 + 全量 verify:web + 真链路点验 + CHANGELOG
```

**最小可交付（MVP 止血）：** PR1 + PR2 + 技能侧栏更名 + title hook。  
**完整关闭本表：** MVP + in-scope 骨架 + 人机词 +（可选 1.B）+ summary 高频批。

---

## 10. 决策记录（已拍板 / 可翻转）

| # | 决策 | 默认 | 可翻转条件 |
|---|---|---|---|
| D1 | 技能 IA 用词 | **技能市场** | PM 明确要求治理向「技能管理」 |
| D2 | 摘要治理 | **写端中文为主**，前端不做整句 regex | 写端短期不可改时，可加结构化 context 展示层，仍不改写 summary 字段 |
| D3 | 名缺失展示 | **`未命名{类型} (短id)` + 可复制全 id**；同步 DESIGN + ObjectRef | — |
| D4 | Provider | 品牌英文显示名；`provider-label.ts` 唯一源 | — |
| D5 | 人机短标签 | 员工长标签「待人工确认」；KPI/短标签 **全局统一一词**（推荐「待人工」） | 若保留「待人工处理」则 KPI 全用它 |
| D6 | 配置面 `role_key` / 技能 slug | 保留标识，中文托底；不属 P0 中文化 | — |
| D7 | 1.B 补名 | **盘点后可选**；技能 FE 回退优先 | 盘点证明 enrich 缺口则上 PR3 |
| D8 | 加载态范围 | **仅 §3.3 四类页**；其余 backlog | 人类扩大 in-scope 列表 |
| D9 | 自动化「待你处理」 | 实施前人类二选一：保留第二人称导流 **或** 与收件箱统一「待我处理」 | 产品口径 |

---

## 11. 附录：建议新增的 API（示意）

```ts
// status-labels.ts 或 object-ref 旁路（实施时按真实调用收敛）
export function relatedRefMetaLabel(kind: RelatedRefMetaKind): string { /* ... */ }

export function humanWaitLabel(
  surface:
    | "inbox_kpi"
    | "inbox_badge"
    | "inbox_progress_second_person" // →「待你」，或文档规定不走此函数
    | "project_rail"
    | "automations_gate"
    | "run_overview_kpi"
    | "run_overview_badge"
    | "employee_card"
    | "project_object"
): string { /* ... */ }

export function missingObjectLabel(
  kind: "project" | "team" | "employee" | "task" | "demand" | "approval",
  id: string
): string {
  return `未命名${typeTitle(kind)} (${shortId(id)})`;
}

// object-ref.tsx
export function shortId(id: string): string { /* 现有私有实现导出 */ }
```

`providerDisplayName`：**保持 `provider-label.ts` 为唯一源**，成本页删除重复表。

---

## 12. 开发会话接手清单（给下一会话）

1. 读本方案 + `DESIGN.md` §面向用户文本；不要另起规范。  
2. 确认人类是否已裁定 **D5 KPI 用词**、**D9 automations**（未裁定则用本表默认并在 PR 说明）。  
3. 按 PR1 → PR2 →（PR5 可穿插）→ 盘点 → PR3? → PR4a → PR6 推进。  
4. 共享工作树：显式路径暂存；`status-labels.ts` 最小 diff。  
5. 验证：分层 `verify:*` + §7 真链路；收尾 `superteam-completion-check`。  
6. 延后项写根目录 `TODO.md`（PR4b、Loading backlog），勿假装本方案已全站清完 LoadingState。

---

## 13. 一句话总结

> **先对齐 D3/ObjectRef/DESIGN 并前端止血（指称、ref meta、Provider、技能列表/侧栏、title），再有界做骨架与人机词；补名与 summary 写端按盘点/清单分批，不空转 join、不无限扫 Loading——全部落在已有 DESIGN 与 `status-labels` 事实源上。**


---

## 14. 实施记录（2026-08-07）

### 1.B 空名盘点

| 路径 | 结论 |
|---|---|
| Inbox `source_*_name` | 已有 enrich；空名多为源删/未挂项目 → **前端 D3 回退即可**，不开 PR3 |
| Skills bindings | SQL JOIN 已补名；空名以前端 missingObjectLabel 闭环 |
| 其它列表 | 触达即 D3，无单独 CP 缺口 |

### 2.4 summary 生产者清单（PR4a 已改 / PR4b backlog）

| kind / 调用点 | 状态 |
|---|---|
| 高风险分派闸 `riskApprovalSummary` | 已中文 |
| 项目验收 `BuildProjectAcceptancePresentation` | 已中文 |
| 计划确认 `plan_review` Payload.Summary | 写端多为中文规划摘要；长尾不强制 |
| 任务失败决策 `humanReadableFailureSummary` | **PR4a**：英文化 detail 映射 + 默认失败/取消中文 |
| 扩编 `casting_expansion` / gap discovery | **PR4a**：角色显示 `名称（key）` |
| 分派失败恢复 / 长尾 failure family | **PR4b** → TODO.md |

### Loading backlog

审计/logs/runtime/system-config 等整页 LoadingState → TODO.md，不阻塞本方案关闭。
