# 配置与项目运营补完设计

- 日期：2026-08-07
- 状态：**已拍板并实施**（2026-08-07）
- 目标：直接提升「能配项目、敢归档、敢恢复」的日常可用性
- 范围：项目配置页人类成员/负责人 UI；项目成员读路径服务端补名；归档门禁与预览（含 demand / 待决决策口径）
- 非目标：项目成员增量 REST（单人 PATCH/POST/DELETE）；归档后复活收件箱；删除门禁改造；租户角色矩阵；force-archive 运维旁路（若审查要加，单列 follow-up）
- 前置阅读：
  - `docs/superpowers/specs/2026-07-20-project-multiple-human-owners.md`（owners ≡ owner-role 人类成员；≥1；any-of-N）
  - `DESIGN.md`「面向用户文本与枚举显示」
  - `AGENTS.md` 协作模型（至少一名 human owner）
  - 关联 TODO：`TODO.md`「2026-07-20 项目成员名服务端批量补名」
  - 对照先例：团队配置成员 `apps/web/src/features/teams/components/team-config-members.tsx`；项目删除预览 `ProjectDeletePreview`

---

## 0. 已锁定决定（审查通过）

| # | 议题 | 方案默认（推荐） | 备选 | 影响 |
|---|---|---|---|---|
| D1 | 人类成员添加默认角色 | 默认 `observer`（只进池、不扩审批权）；对话框可选 `owner` / `reviewer` / `observer` | 默认 `owner`（添加即负责人） | UI 默认值；后端角色枚举不变 |
| D2 | 人类成员候选池 | 与团队一致：`listAuthzMembers` 中 `console_access && account_status=active`，并排除已在项目池的 user | 全量 `listUsers`（含无控制台访问） | 候选 API 与门禁文案 |
| D3 | 人类角色可在配置页切换的集合 | UI 仅暴露人类语义角色：`owner` / `reviewer` / `observer`；**不**在人类行提供 `executor`（executor 保留给数字员工） | 四角色全开 | 防止人类被标成 executor 的脏数据入口 |
| D4 | 成员名契约形态 | **A**：读路径批量把当前名写入响应的 `display_name_snapshot`（字段变「当前可读名」；写库 snapshot 仍可保留历史写入值，但 API 读出优先当前名） | **B**：新增只读 `display_name`，snapshot 保持写入时快照 | A 前端改动最小；B 语义更干净 |
| D5 | 非终态 demand 对归档 | **硬阻断** | 仅警告 | 硬门禁 + preview |
| D6 | pending/requested 决策对归档 | **硬阻断** | 强警告 + 二次确认后仍可归档 | 硬门禁 + preview；不推荐静默 cancel |
| D7 | 无 demand 的空项目 | **允许归档**（不得误用「全部 demand 终态且 total>0」把空项目挡死） | 禁止归档空项目 | `AreAllProjectDemandsTerminal` 不能原样当硬条件 |
| D8 | `missing_evidence` | 保持 **soft 警告**（不挡 POST /archive） | 升为硬阻断 | 与现状一致即可 |
| D9 | 归档时 open 收件箱 | **维持现状**：同事务 cancel（`CancelInboxItemsForProjectDelete`）；preview **必须写明将取消的 open inbox 数** | 先要求人手工清 inbox 再归档 | 文案与 warnings，不改事务语义 |
| D10 | unarchive | **不改语义**：恢复 `running`，不复活已 cancel 的 inbox/决策；UI 明确提示 | 尝试复活 inbox（明确不做） | 仅文案 |
| D11 | `missing_final_report`（现状 `BuildArchivePreview.BlockedReasons` 已含此 code，新 blocker/warning 枚举表未纳入） | 降级为 warning，并入 `missing_evidence` 同一提示（"材料不全"不单独区分报告/证据） | 保留独立 warning code `missing_final_report` | 选后者需在 §3.3.2 enum 补一项；选前者需在 §3.3.1/§3.3.2 写清"报告缺失"并入哪条文案 |

**审查通过前默认上述推荐项。** 若只改 D1/D4/D6，其余可不动。

> **拍板前必读（阻塞项，不可默认略过）**：D6 的硬阻断复用 `assertProjectReadyToArchive`，而该函数也被"通过并结项"自动归档路径（`tryCloseProjectFromDemandSignOff`）调用。经代码核实，`SignDemandCriterionVerdict` 会先把 demand 状态落库，再调用收敛逻辑触发自动归档——这两步**不在同一事务**（证据：`reconcileTerminalDemandSignOff` 专门用于愈合"demand 已终态但收敛步骤失败"的情形）。若 `assertProjectReadyToArchive` 因 pending decision 报错且该错误照原样冒泡给 `SignDemandCriterionVerdict` 的调用方，人类的签署请求本身会报错；且由于 pending decision 不会自愈，重试会走进 `reconcileTerminalDemandSignOff` 并再次撞上同一个错误，形成死循环——签署其实已经落库成功，但 API 会持续报错直到人类去处理一个八竿子打不着的决策。**实施时 `tryCloseProjectFromDemandSignOff` 遇到 assert 失败必须比照"其他 demand 未终态"分支处理：记录留痕（日志/事件）后返回 `closed=false, err=nil`，不得把该 err 冒泡给 `afterDemandSignOffConvergence`/`convergeDemandSignOff`/`SignDemandCriterionVerdict` 的调用方。** 详见 §3.3.3.2。

---

## 1. 问题与现状事实（带代码锚点）

### 1.1 人类成员 / 负责人 UI 增删 —— 后端齐、前端缺

| 层 | 事实 | 锚点 |
|---|---|---|
| 宪法/模型 | 至少 1 个平级 human owner；owners ≡ `project_role=owner` 的人类成员；`human_owner_user_ids` 为同步缓存 | `AGENTS.md`；多 owner spec |
| 后端写 | `ReplaceProjectMembers`：`ownerMemberIDs` 为空 → `ErrProjectRequiresHumanOwner`；成功后 `SetProjectHumanOwners` | `project/service.go` `ReplaceProjectMembers` |
| 后端 API | `PUT /api/v1/projects/{id}/members` 全量替换已稳定 | openapi `replaceProjectMembers` |
| 前端数字员工 | 已有添加对话框 + 行级移除 | `project-config-page.tsx` `MembersHumanizedPanel` / CHANGELOG 2026-08-03 |
| 前端人类 | **只读**列表 + 负责人 pill；**无**添加/改角色/移除 | 同文件人类 `MemberGroup`；`ConfigMemberRow` 仅 DE 有移除按钮 |
| 文案债 | 概览写「在成员标签页增删负责人」——入口不存在 | 概览「项目负责人」说明 |

结论：这是多 owner spec 残留的 **Web 缺口**，不是模型未定。

### 1.2 成员名服务端补名 —— 违宪的客户端 join

| 层 | 事实 | 锚点 |
|---|---|---|
| 宪法 | 名称由服务端读路径批量补名；前端不做逐行/全量目录猜名 | `DESIGN.md` |
| 契约 | `ProjectMember.display_name_snapshot` 可选 | openapi `ProjectMember` |
| CP 读 | `memberResponses` 原样透传 snapshot，无 users/employees join | `handler.go` `memberResponses` / `projectConfigResponseFromDomain` |
| Web | `listUsers({limit:200})` + `listDigitalEmployees()` 拼名；超出静默裸 id | `project-config-page.tsx`；列表风险信号等亦有类似路径 |
| 登记 | TODO 已记，未做 | `TODO.md` 2026-07-20 |

### 1.3 归档门禁 —— 窄硬门 + 更窄预览，demand/决策未纳入

| 能力 | 现状 | 锚点 |
|---|---|---|
| 硬门禁 | 仅 `active_tasks > 0` 或已归档 | `assertProjectReadyToArchive` |
| 预览 | soft：`missing_evidence` / `active_tasks` / `project_already_archived`；**无** open demand、pending decision、将 cancel 的 inbox 数；无 `can_archive` | `BuildArchivePreview`；`ProjectArchivePreview` |
| 归档副作用 | 同事务 cancel 项目 open inbox | `PgRepository.ArchiveProject` + `CancelInboxItemsForProjectDelete` |
| 需求终态助手 | `AreAllProjectDemandsTerminal`（要求 total>0 且非终态=0）用于「通过并结项」自动归档，**不**用于手动 archive | `tryCloseProjectFromDemandSignOff` |
| 删除对照 | 已有 `can_delete` + structured blockers + `pending_decision_count` warnings | `ProjectDeletePreview` |
| UI | 归档预览 tab 对 `blocked_reasons` 原始展示；详情头归档确认与 preview 结构化 blockers 未对齐 delete 体验 | `project-archive-panel.tsx`；`project-operational-detail.tsx` |

可出现的坏体验：任务全终态但 demand 仍 `acceptance_pending` → 可归档；pending 决策被 inbox cancel 却预览绿灯；人不敢点是因为说不清会怎样。

---

## 2. 目标体验（验收语言）

1. **能配项目**：在 `/projects/{id}/config?tab=members` 可不经高级 JSON，完成人类成员添加、设/取消负责人、移除；试图去掉最后一位 owner 时前端拦截 + 后端 400 双保险；概览文案与真实入口一致。
2. **名可读**：config / members 读路径响应自带可读名；配置页不再依赖 `listUsers limit 200` 补名；正常路径不出现裸 UUID 指称人。
3. **敢归档**：归档前预览明确 `can_archive`、硬阻断原因、警告项（含将取消的 open inbox）；硬条件与 POST 一致（预览绿灯 ⇒ POST 成功，除非并发状态变化）。
4. **敢恢复**：unarchive 入口保持；文案写清「恢复为可运营，不复活归档时取消的待办」。

---

## 3. 方案分轨

### 3.1 轨 A — 人类成员 / 负责人 UI（P0）

#### 3.1.1 API 策略

- **不新开**成员增量 API。
- 继续 `replaceProjectMembers(projectId, nextMembers[])` 全量替换。
- 前端维护「当前成员输入集」：`toMemberInput` 已有；在其上做 add / role-change / remove 后整表提交。
- 提交成功后沿用现逻辑：invalidate config + members；`human_owner_user_ids` 以后端回读为准（replace 已同步）。

#### 3.1.2 角色语义（人类行）

| `project_role` | UI 标签（走 `projectRoleLabel`） | 人类行是否可选 | 说明 |
|---|---|---|---|
| `owner` | 负责人 | 是 | 计入 owner 集合；any-of-N 审批/验收 |
| `reviewer` | 审查人 | 是 | 非 owner 人类参与方 |
| `observer` | 观察者 | 是 | 默认添加角色（D1） |
| `executor` | 执行者 | **否**（人类行） | 数字员工用；人类误选禁止 |

切换为 `owner` / 非 `owner` 即「设为负责人 / 取消负责人」，无需第二套 API。

#### 3.1.3 UI 结构（`project-config-page.tsx`）

人类 `MemberGroup` 升级为与数字员工对称：

1. **标题动作**：「添加人类成员」按钮（归档项目 disabled，与 DE 一致）。
2. **添加对话框**：
   - 候选：`listAuthzMembers`（D2），过滤 `console_access && active`，排除 `humanMembers` 已有 `principal_id`。
   - 复用 `UserSearchSelect` / 团队添加成员交互模式（多选或单选+连续添加；本方案默认 **多选一次提交**，与 DE 对话框一致）。
   - 角色选择：默认 D1；提交时每条 `{ principal_type: "human_user", principal_id, project_role, display_name_snapshot? }`。
   - `display_name_snapshot`：有则带候选显示名（轨 B 落地后可省略，后端读路径补）。
3. **行级动作**（`ConfigMemberRow` 人类分支）：
   - 角色下拉或「设为负责人 / 取消负责人」+ 如需审查人/观察者切换。
   - 「移除」：确认后从全集去掉该 principal。
   - **最后一位 owner**：
     - 取消负责人且无其他 owner → 按钮 disabled + 提示「至少保留一位负责人」。
     - 移除最后一位 owner → 同上。
     - 若移除的是 owner 但仍有其他 owner → 允许。
4. **概览**：
   - 负责人列表只读（已有）。
   - 文案改为：「多个平级负责人，任一可审批/验收；请到「成员」标签管理，至少保留一位。」并链到 `tab=members`（TanStack `Link`/`navigate`）。

#### 3.1.4 错误与权限

- 后端错误透传：`ErrProjectRequiresHumanOwner` → 中文 detail（若 handler 尚未友好，本轨顺手对齐 apierror 文案）。
- 数字员工仍被编制引用不可移除的既有门禁不变。
- 本轨不新增 authz action；沿用项目配置/成员替换既有 `allowed_actions`（与当前 DE 增删相同通道）。若详情头对 config 写无独立 action，保持现状不扩权。
- **D3 澄清（经代码核实）**：`validateMembers` 后端已拒绝 `project_role=executor && principal_type≠digital_employee`，且既有"成员完整替换 JSON"高级面板（`MemberJsonPanel`）在本轨上线后仍会保留、不受 D3 约束。也就是说 human+executor 组合本来就已被后端拦截，D3 只是新版引导式 UI 里的前端可选项收窄（避免用户在下拉里看到不该选的选项），**不是**新增的数据卫生防线——不要在 review/CHANGELOG 里把它写成"补上了一个口子"。

#### 3.1.5 测试

- `config.test.tsx`：
  - 添加人类成员（mock PUT body 含新 human + 保留原 owner）。
  - 设第二人为 owner；取消非最后 owner。
  - 拦截去掉最后 owner（不发 PUT 或 PUT 后展示错误——优先前端不发）。
  - 移除非 owner 人类。
  - 归档项目人类操作 disabled。
- 真实 CP smoke：对可写测试项目 PUT members 增/改/还原。
- 浏览器：成员 tab 可见「添加人类成员」，走通一次添加与移除。

---

### 3.2 轨 B — 成员名服务端批量补名（P2，可与 A 并行后段）

#### 3.2.1 契约（默认 D4=A）

**推荐 A（改动面小）**

- 不改 schema 字段名。
- 规范更新：`display_name_snapshot` 在 **读模型** 上表示「当前用于展示的名称」：
  1. 若 principal 仍存在：用 users/employees 当前名覆盖响应；
  2. 若主体已删/不可见：回落库内 snapshot；
  3. 仍空：省略字段，前端才显示 id。
- 写路径：客户端仍可传 snapshot；服务端 replace 时 **可选** 用权威名回填再落库（增强，非必须第一刀）。

**备选 B**

- `ProjectMember` 增加只读 `display_name`；snapshot 保持写入快照。
- 前端：`display_name || display_name_snapshot || id`。
- 需 openapi + gen + TS 类型。

#### 3.2.2 CP 实现

1. 引入批量解析端口（示意）：

```text
MemberNameDirectory interface {
  LookupUserNames(ctx, tenantID, userIDs []uuid) (map[uuid]string, error)
  LookupEmployeeNames(ctx, tenantID, employeeIDs []uuid) (map[uuid]string, error)
}
```

2. 挂载点（最小集，本批必做）：
   - `GET .../projects/{id}/config` → `projectConfigResponseFromDomain` 前 enrich
   - `GET/PUT .../projects/{id}/members` 列表响应

3. 实现策略：
   - 一次请求内按 type 收集 id → 两次 IN 查询（或现有 auth/employee repo 批量方法）→ 填响应。
   - **不要** N+1。
   - handler 保持薄：enrich 放 service 或 response assembler。

4. 后续可扩（本批不做除非顺手）：overview 列表、风险队列、dossier 里裸成员 id；发现一处记一处，不无限扩大。

#### 3.2.3 Web

- `project-config-page.tsx`：删除「仅用于补名」的 `listUsers limit 200`；DE 目录查询若只为补名且 config 已带名，可缩为「添加 DE 对话框候选」专用（候选仍需要目录 API，这与补名分离）。
- 展示优先级：`member.display_name_snapshot`（A 下已是当前名）→ 候选目录名（仅 picker）→ id。
- 其他页面若仍客户端 join 成员名：本批至少改配置页；其余打开 TODO 或同 PR 顺手若改动面小。

#### 3.2.4 测试

- CP：members 无 snapshot 时 list/config 仍返回当前名；用户改名后再次 GET 见新名（A）。
- Web：config 测试不依赖 users 列表也能显示人类名（fixture 带 snapshot 或 mock 已补名响应）。
- 真实：抽一个 snapshot 空的成员行（或临时清空）GET config 仍见名。

#### 3.2.5 TODO 收口

- 完成后删除 `TODO.md`「2026-07-20 项目成员名服务端批量补名」行。

---

### 3.3 轨 C — 归档门禁与预览补完（P1）

#### 3.3.1 产品口径（默认 D5–D10）

| 条件 | 级别 | code | 说明 |
|---|---|---|---|
| 项目已归档 | 硬阻断 | `already_archived` | 现状 |
| `active_tasks > 0` | 硬阻断 | `active_tasks` | 现状；与 task status counts 同源 |
| 存在非终态 demand | 硬阻断 | `open_demands` | 终态 = completed / failed / cancelled（与 demand 状态机一致）；**0 条 demand 不挡** |
| 存在 pending/requested（及平台认定 open 的）决策 | 硬阻断 | `pending_decisions` | 与删除预览 pending 计数口径对齐；**该阻断同时被"通过并结项"自动归档路径复用，实施时必须遵守 §0 拍板前必读的非冒泡要求，见 §3.3.3.2 第 3 点** |
| 无证据 | 警告 | `missing_evidence` | 不挡归档 |
| 无最终报告（D11，默认并入上一行） | 警告 | `missing_evidence`（并入）或独立 `missing_final_report`（备选，见 D11） | 现状 `BlockedReasons` 已含此 code；不得在迁移中静默丢弃，需按 D11 决议实现 |
| 将取消 open inbox | 警告 | `open_inbox_will_cancel` | 告知副作用；不挡 |
| retention_pending | 信息/警告 | （已有字段） | 不挡 POST /archive |

**硬阻断集合**必须同时用于：

- `assertProjectReadyToArchive`（`POST /archive` 与「通过并结项」自动归档路径里对 assert 的调用）
- `BuildArchivePreview` → `can_archive == (len(blockers)==0 && !already_logic_conflict)`

预览与 POST **同一函数** `evaluateArchiveReadiness(...) (blockers, warnings)`，禁止两处手写分叉。

#### 3.3.2 契约变更

扩展 `ProjectArchivePreview`（对齐 delete preview 精神，保持归档域字段）：

```yaml
# 逻辑形状（写入 openapi 时用正式 schema）
ProjectArchivePreview:
  required: [project_id, can_archive, blockers, warnings, evidence_count, artifact_count, report_count, retention_pending, estimated_object_refs]
  properties:
    project_id: uuid
    can_archive: boolean
    blockers:
      type: array
      items:
        $ref: ProjectArchiveBlocker
    warnings:
      type: array
      items:
        $ref: ProjectArchiveWarning
    # 兼容过渡：保留 blocked_reasons（由 blockers.code 派生的 any 数组）一个版本，标注 deprecated
    blocked_reasons: array
    evidence_count: int64
    artifact_count: int64
    report_count: int64
    retention_pending: boolean
    estimated_object_refs: array
    message: string  # 面向用户的一句总结，如「仍有 2 个未结需求，无法归档」

ProjectArchiveBlocker:
  required: [code, message, count]
  properties:
    code: enum [already_archived, active_tasks, open_demands, pending_decisions]
    message: string   # 中文
    count: integer
    # 可选 samples：前 N 个 id/title 供 UI 深链，本批可先不做 samples

ProjectArchiveWarning:
  required: [code, message, count]
  properties:
    code: enum [missing_evidence, open_inbox_will_cancel]
    # D11=独立 code 时在此加 missing_final_report；D11 默认（并入 missing_evidence）则不加，
    # 由 message 文案区分"缺证据/缺最终报告"
    message: string
    count: integer
```

`POST /archive` 被拒时：

- HTTP 409（经代码核实，`ErrProjectArchiveBlocked` 现状已在 `writeHandlerError` 映射到 `http.StatusConflict`，与 delete blocked 一致——不是待定项，新 blocker 类型沿用 409 即可，无需实现期再判断）。
- body 建议：

```json
{
  "code": "project_archive_blocked",
  "message": "项目仍有未结项条件，无法归档",
  "blockers": [ { "code": "open_demands", "message": "仍有 1 个未结需求", "count": 1 } ]
}
```

前端用 detail/blockers 展示，不靠解析英文 sentinel。

#### 3.3.3 CP 实现要点

1. **计数来源**
   - active tasks：现有 `GetProjectTaskStatusCounts`
   - open demands：`CountProjectDemandsByTerminality` 的 `NonTerminalCount`（注意：空项目 NonTerminal=0,Total=0 → 不阻断）
   - pending decisions：复用 delete preview 的 pending decision 计数 SQL/查询（`status_snapshot IN ('pending','requested',…)` 与删除警告对齐，**同一条件**）
   - open inbox：delete preview 的 `open_inbox_count` 同源
   - evidence：现有 preview 逻辑

2. **`assertProjectReadyToArchive`**
   - 调 `evaluateArchiveReadiness`；`len(blockers)>0` → 返回带 blockers 的错误类型（类似 `ProjectDeleteBlockedError`），替代单一无结构 `ErrProjectArchiveBlocked`（可保留 sentinel 包装以便 `errors.Is`）。

3. **自动结项路径** `tryCloseProjectFromDemandSignOff`（**本轨最容易踩坑的一步，务必对照代码实现**）
   - 在 demand 全终态之后仍走 assert：将自动获得新的 `open_demands`/`pending_decisions` 保护。
   - **但 assert 失败绝不能把 err 原样返回给调用方**。当前 `SignDemandCriterionVerdict` 会先落库 demand 状态推进（`AdvanceProjectDemandStatus`/`RecomputeProjectDemandStatus`），再调用 `afterDemandSignOffConvergence` 触发自动归档；这两步不在同一事务（`reconcileTerminalDemandSignOff` 的存在本身就是"状态已终态但收敛步骤可能失败"的愈合机制）。若 assert 失败的 err 冒泡到 `convergeDemandSignOff`/`SignDemandCriterionVerdict`，会导致：(a) 人类已经签署成功的动作在 API 层报错；(b) pending decision 不会自愈，重试会经 `reconcileTerminalDemandSignOff` 再次调用同一个 `afterDemandSignOffConvergence`，撞上同一阻断，形成死循环。
   - **要求的实现**：`tryCloseProjectFromDemandSignOff` 内捕获 assert 失败，按「demand 全终态但项目未达归档条件」处理——记录留痕（日志 + 一条可观测事件，如 `project_auto_close_deferred`，附 blockers 明细）后，比照"其他 demand 未终态"分支返回 `closed=false, err=nil`；不得改变 `afterDemandSignOffConvergence`/`convergeDemandSignOff`/`SignDemandCriterionVerdict` 的现有成功返回路径。人类侧不会看到签署报错，只会看到项目仍未归档（可在项目详情/归档预览里看到具体 blocker）。
   - 单测必须覆盖：demand 全终态 + `also_close_project=true` + 存在无关 pending decision → `SignDemandCriterionVerdict` 返回 200（签署成功）且项目未归档；重试（模拟 `reconcileTerminalDemandSignOff` 路径）同样不报错、不死循环。

4. **Inbox cancel**
   - 保持 ArchiveProject 事务内 cancel；不在本批改 decision 行终态机，除非审查要求「决策表与 inbox 双写终态」。
   - **默认**：inbox cancel 足够（现状已为收件箱收敛）；决策行若仍 pending 但项目 archived，读路径/工作台应以项目 archived 降权（多数列表已滤 archived）。若实测决策尸检造成困扰，记 follow-up，不阻塞本批。

5. **Unarchive**
   - 服务层不改；UI 文案明确。

#### 3.3.4 Web

1. **归档预览 tab**（`project-archive-panel.tsx`）
   - 状态 pill：`can_archive` →「可归档」/「不可归档」；`retention_pending` 作次要标记。
   - 列表渲染 blockers/warnings 中文，不 dump 原始 JSON。
   - 保留 counts 与快照生成。

2. **详情头「归档项目」**
   - 打开确认前（或确认框内）拉 `getProjectArchivePreview`。
   - `can_archive=false`：主按钮 disabled 或确认 action disabled，列出 blockers；提供去需求/收件箱的导航若有现成深链。
   - `can_archive=true` 且有 warnings：确认文案包含「将取消 N 条待办收件箱」等。
   - POST 409：展示 blockers，不 toast 含糊失败。

3. **词表**
   - archive blocker/warning code → 中文：优先服务端 `message`；前端 `status-labels` 可做兜底 map。

#### 3.3.5 测试矩阵

| 场景 | preview.can_archive | POST /archive |
|---|---|---|
| 干净项目（无 active task / 无 open demand / 无 pending decision） | true | 200 |
| 仅 active_tasks>0 | false | 409 |
| 仅 open demand | false | 409 |
| 仅 pending decision | false | 409 |
| 仅 missing_evidence | true + warning | 200 |
| 已归档 | false | 4xx 已有 |
| 空项目（零 demand） | true（若无其他阻断） | 200 |
| 归档后 inbox open 数 | — | 0（既有副作用） |
| demand 全终态 + 通过并结项 + 存在无关 pending decision | — | `SignDemandCriterionVerdict` 200（签署成功）；项目仍 `running`，未归档；留有可观测记录 |

- 单测：`evaluateArchiveReadiness` / service_test 矩阵。
- 契约：`verify:contracts` 或 foundation 契约脚本。
- 真实 smoke：构造/选用符合矩阵的项目各至少一条 happy + 一条 open_demand 阻断。
- 浏览器：不可归档时确认框/按钮态正确；可归档时警告文案可见。

---

## 4. 分层改动清单（实施时勾选）

### 4.1 轨 A（前端为主）

- [x] `apps/web/src/features/projects/components/project-config-page.tsx` — 人类添加/改角色/移除
- [x] `apps/web/src/features/projects/config.test.tsx` — 用例扩展
- [x] 概览负责人说明文案 + members 深链
- [ ] 如需：`status-labels` 已有 `projectRoleLabel` 复用
- [x] CHANGELOG 一条

### 4.2 轨 B（CP + 薄前端）

- [x] openapi 描述更新（A：description）+ generate
- [x] `apps/control-plane/internal/project` enrich 读路径
- [x] handler/service 测试（归档矩阵；补名靠真链）
- [x] web 去掉配置页补名用 `listUsers`
- [x] 删 TODO 行 + CHANGELOG

### 4.3 轨 C（契约 + CP + Web）

- [x] openapi `ProjectArchivePreview` 等 + generate
- [x] `evaluateArchiveReadiness` + assert/preview/error 接线
- [x] archive panel + detail 归档确认
- [x] 单测矩阵 + 真实 smoke + CHANGELOG

### 4.4 明确不改

- DB migration（三轨默认零迁移）
- Runtime / Provider / Temporal 协调逻辑（归档仍只标项目状态 + cancel inbox）
- `PUT /members` 协议形状
- unarchive 复活语义

---

## 5. 实施顺序与依赖

```text
审查通过
  ├─ 轨 A（P0）独立可合并：纯前端 + 既有 PUT
  ├─ 轨 C（P1）独立可合并：契约 + CP + 归档 UI
  └─ 轨 B（P2）独立可合并：CP 补名 + 配置页去客户端 join
推荐合并顺序：A → C → B
  理由：A 当天提升配置力；C 消除归档恐惧；B 正确性/规模，不挡 A。
A 与 B 同 PR 亦可，但 review 面变大。
```

工作量粗估（单人熟悉本仓）：

| 轨 | 量级 |
|---|---|
| A | 0.5–1 日（含测试与浏览器点通） |
| C | 1–1.5 日（契约+门禁+UI+矩阵） |
| B | 0.5–1 日（批量查询+接线+去 join） |

---

## 6. 风险与缓解

| 风险 | 缓解 |
|---|---|
| 全量 replace 误抹 DE 池或编制成员 | 前端以 config.members 为基底改人类子集；单测断言 DE 条目保留；DE 编制移除门禁已在后端 |
| 候选 `listAuthzMembers` limit 200 仍可能截断 | 与团队一致；搜索组件若支持服务端 search 则用 search；截断时对话框空态说明「未找到，请缩小关键词/联系管理员」 |
| D4=A 让 snapshot 语义从「写入快照」变为「读时名」 | openapi description 写清；审计若依赖历史名应读事件载荷而非成员行 |
| 收紧归档导致以往「任务完即可档」的项目变不可档 | 预期行为；preview 中文说明去结 demand/决策；CHANGELOG 写 cleark break 行为 |
| 预览与 POST 不一致 | 强制单一 `evaluateArchiveReadiness` |
| `pending_decisions` 硬阻断被"通过并结项"自动归档路径复用，assert 失败若冒泡会让已成功的签署请求报错，且经 `reconcileTerminalDemandSignOff` 重试会死循环 | `tryCloseProjectFromDemandSignOff` 捕获 assert 失败后不冒泡 err，比照"其他 demand 未终态"分支返回 `closed=false, err=nil` + 留痕；见 §0 拍板前必读 / §3.3.3.2 第 3 点 |
| 共享工作树 git 踩踏 | 只 `git add` 显式路径；不碰无关脏文件 |

---

## 7. 验证计划（完成定义）

必须满足 `superteam-completion-check` 精神：

1. **轨 A**：`corepack pnpm --filter @superteam/web test` 相关文件绿；真实浏览器成员 tab 添加/移除人类；CP PUT 回读 owners≥1。
2. **轨 C**：`go test` project 包归档矩阵；真实 POST archive 阻断与放行各一；归档后 inbox 无 open；preview 与 POST 一致。
3. **轨 B**：真实 GET config 成员带名；配置页网络面板无「仅补名」的全量 users 依赖（picker 除外）。
4. 契约变更走 `generate:control-plane` + `verify:contracts`（或仓库当前等价脚本）。
5. CHANGELOG 按轨或合并一条写清行为变化。

无法起服务时标记阻塞，不宣称 E2E 完成。

---

## 8. 文档与后续

- 本 spec 审查通过后状态改为「已拍板，实施中」，填入实际 D1–D10 决议表。
- 多 owner spec 可在 Web 缺口关闭后补一句「配置页人类增删已由 2026-08-07 运营补完落地」。
- Follow-up（默认不进本批）：
  - 归档时同步终态化 `project_decision_requests`
  - force-archive 管理员旁路
  - 成员增量 REST
  - 全站其余裸 id 读路径普查

---

## 9. 审查清单（请人类勾选）

- [x] D1 默认角色：observer / owner / 其他 ______
- [x] D2 候选池：authz console_access / 其他 ______
- [x] D3 人类可选角色：owner+reviewer+observer（禁止 executor）/ 其他 ______
- [x] D4 补名：A 覆盖 snapshot / B 新字段 display_name
- [x] D5 open demand：硬阻断
- [x] D6 pending decision： 硬阻断
- [x] D7 空项目可归档：是
- [x] D8 missing_evidence：警告（推荐
- [x] D9 inbox cancel 仅警告告知：是
- [x] D10 unarchive 不复活：确认
- [x] D11 `missing_final_report`：并入 `missing_evidence`（推荐）
- [x] 确认已知悉「通过并结项」+ assert 复用的非冒泡实现要求（§0 拍板前必读 / §3.3.3.2 第 3 点）：是
- [x] 实施顺序：A→C→B
- [x] 是否拆 PR：一轨一 PR（推荐）

审查无问题后，按拍板结果改本文件 §0 为「已锁定」，再开工。
