# 收口批：编制事实源闭环 + 无人值守失败通知 + 工程收尾

> 复核状态：状态行已过时（写"已立项、已拍板、可直接开工"），实际已在commit 16a4c3ea全量实施入main（编制事实源双向闭环、无人值守失败通知、E2E断言收敛为casting-suite.mjs），真实链路验证casting-suite五stage全PASS。建议将文件状态行更新为"已实施"并注明commit与§7实现备注中的B方案覆盖决定。

- 日期：2026-08-06
- 状态：**已立项、已拍板、可直接开工**（交接给实施会话）
- 系列：剧本可落地化收口批（批一 `7a7064b5` 词表两侧对齐；批二 `7b0369de`+`1f36c944` 角色词表与编制；批三 `2c6784c2`+`9bf7ed2c` 语义扩编 + 角色治理台）
- 交付性质：**封两个静默洞 + 一次工程收敛 + 路线图对齐**。不新增业务能力，不改剧本，不改 provider 管道
- 目标读者：实施会话（本文自包含；实施前必读基线 `2026-07-27-workspace-and-playbook-alignment-baseline.md` §4）
- **零迁移**（见 §3 地基核对第 5 条）

---

## 0.0 开工须知（实施会话先读这一节）

### 环境

服务用 `./scripts/dev-services.sh start|status|restart <service>` 管理（OpenFGA 需单独管）。

| 项 | 值 |
|---|---|
| Web | `http://127.0.0.1:3100`（**不是 3000**） |
| Control Plane | `http://127.0.0.1:8080` |
| 登录 | `POST /api/auth/login`，`admin` / `admin` |
| 数据库 | `apps/control-plane/config/config.yaml` 的 `postgres.url`（远程 dev 库，**无备份**） |
| 门禁 | `verify:contracts` / `verify:control-plane` / `verify:web`；`verify:runtime-agent` 有既有并发 flake（本批 runtime 零改动，不是你弄坏的） |

### 数据现状（2026-08-05 实测，批三 spec §0.0 仍有效）

- 租户 `00000000-0000-0000-0000-000000000001`，默认团队 `…0101`
- 角色词表 15 条（10 个 seed + E2E 会话注册的 `network_diagnostics` 等）
- 数字员工 5 个，角色绑定见批三 spec §0.0 表
- **没有任何员工持有 `operator`**——这是批二 G2 的判别条件，**不要"顺手"给谁补上**
- 项目 `批二基线项目 P1` = `ca82b054-de2d-4810-9a2b-dd41f5e50a2c`；`incident_response` 缺 operator 编制

### 本批的性质

这三条**都不是新功能**，是上一批交付时留下的口子：

1. 编制硬校验只守住了写入侧，角色移除侧敞着（§4）
2. 基线 §4.7「automation 编制失效必须通知」只落了留痕，没落通知（§5）
3. 19 个一次性 E2E 脚本没有共享断言，已经导致过一次"绿着漏"（§7）

**不要在本批里扩展语义能力。** 发现新的产品缺口就记 TODO 或另行立项。

---

## 0. 已拍板（2026-08-06 人类决定，不重议）

| # | 决定 |
|---|---|
| 1 | 移除员工角色 / 停用词表角色时：**预检弹窗 + 确认后级联解除编制并通知**。不采用"拒绝移除"（角色是组织事实，不该被一条项目编制锁死），也不采用"保留编制行标 invalid"（不引入第三种编制状态） |
| 2 | automation 失败通知发给**项目负责人集合，any-of-N**（复用 `human_owner_user_ids`）。任一负责人处理或规则恢复即全体自动 resolve |
| 3 | 本批范围 = A 封洞 + D E2E 收敛 + E 基线补登记。**#4 轻发起与剧本归属另行立项**，不在本批 |

---

## 1. 三个问题（均已核到代码）

### 1.1 编制事实源只守住了写入侧（承重）

2026-08-05 拍板的 `PutCasting` 硬校验（`project/casting.go:214`，持有校验在 :259 起）保证**写入时**被编制员工持有该角色。理由是原话：

> 编制是「谁能干这个角色」的事实源，可达收口、缺员拦截、扩编候选全从这里读；允许 API 绕过去写等于允许这些判断静默失真。

但同一份失真可以从另外三扇门进来：

| 门 | 位置 | 现状 |
|---|---|---|
| 移除员工角色 | `employee/service.go:77` `ReplaceEmployeeRoles` | 不查、不级联既有编制行 |
| 停用词表角色 | `rolevocab/service.go` 改 status | 只挡新编制，既有编制行照旧（`references` 端点让人看得见，比前者轻一档） |
| 读路径不自证 | `project/casting.go:544` `ValidatePlaybookCastingComplete` / `:728` `missingRoles` | 前者只查员工 `status ∈ {active, ready}`，**不复查是否仍持有 role_key**；后者见到 casting 行就 `continue`，两项都不查 |

后果：把「开发-A」的 `developer` 一摘，`software_delivery` 的编制行还在 → 剧本选择器照样显示最深档可达、缺员拦截不触发、automation 照跑，**派的还是那个不再持有该角色的人**。这正是基线 §4.6「缺员必须显式处理，不得静默降级为找个人凑合派」要挡的事。

对照证据：**成员池移除有护栏**（`project/service.go:1535`：仍被编制引用的数字员工拒绝移出），角色移除没有。这个不对称说明是漏掉，不是有意。

### 1.2 基线 §4.7 只兑现了留痕，没兑现通知

基线 §4.7（2026-08-04 人类拍板修订）原话：

> 这类失效**允许 automation 失败**，但必须**发失败通知并写明原因**（缺哪个角色、原选谁、为何失效）。

实现（`automation/service.go:419-427` 闸 → `:599` `failFire`）只做了：写 `automation_fires` 行（status=failed + error_code + error_message）+ 累加连续失败计数，够 `MaxConsecutiveFailures` 就自动停用规则。`automation.Service` 的依赖里（`service.go:16`）根本没有 inbox/notifier。

即：**失败只能靠人主动打开规则详情页翻 fires 列表才能看见**——而这是无人值守路径，没人会去翻。错误文本也只有 `剧本编制不完整或编制员工不可用: <role> (员工不可用)`，**不含"原选谁"**。

而 7b0369de 的 commit 消息写着「兑现基线 §4.7 修订版『允许失败但必须通知』」——文档与实现不一致，这本身也要修。

### 1.3 一次性 E2E 脚本没有共享断言，已经漏过一次

`scripts/e2e/` 一周新增 19 个脚本、9432 行（产品代码同期 12990 行）。其中 12 个是 `browser-casting-*` 的重叠变体（`g10-g12` / `g10-g11-g12` / `gates-g10-g11-g12-finish` 并存）；`run-casting-design-full-suite.mjs:22-30` 硬编码项目与员工 UUID；每个脚本各自重写 `cpLogin` 与 fetch 封装；**没有一个共享模块**。

批三 CHANGELOG 自己记的教训：

> 三个 E2E 脚本对任务终态零断言，这一路是"绿着"漏过去的。

断言集不累积，是这套打法的结构性结果，不是那次的疏忽。

---

## 2. 非目标

| 不做 | 理由 |
|---|---|
| #4 轻发起与剧本归属 | 已拍板另行立项；本批只封洞 |
| 回填历史 9 条扩编决策的 `project_task_id` | 历史数据，成本 > 收益；时间线已由 `decisionsByApproval` 索引复原，只有"该单待办归属"缺。记 TODO，不做 |
| automation 失败推飞书 | 需新 outbox kind + connector 卡片模板（跨仓）。P1 只做 Console 收件箱，飞书推送记 TODO |
| 引入第三种编制状态（invalid） | §0 决定 1 已否决 |
| 把 automation 失败做成需人裁决的决策卡 | 它是告警不是决策，无审批动词——照 `channel_alert` 先例走空动作告警项 |
| 给谁补发 `operator` 角色让 `incident_response` 编制变完整 | 批三 §0.0 明令禁止：它是缺员判别条件 |

---

## 3. 地基核对（2026-08-06 核实，勿重复勘察）

1. **收件箱已有"非决策告警项"先例**：`inbox.ItemTypeChannelAlert` / `SourceTypeChannelAlert`（`inbox/types.go:63-79`）。写入模式见 `app/feishu_channel_watchdog.go:70-113`：每个收件人一条 personal item、`Actions: []`（`inbox/service.go:417` 对 channel_alert 豁免默认动作、`:450` `DefaultActions` 返回 nil）、带 `DeepLink` 与 `ContextPayload`、恢复时 `ResolveOpenItemsBySource` 批量关闭。**本批的 automation 告警照抄这条路径即可。**
2. **`inbox/service.go:360-380` 有 itemType↔sourceType 配对校验**，新增类型必须在这里加一 case，否则 `ErrInvalidItem`。
3. **`inbox_items.item_type` 是 `VARCHAR(100)` 无 CHECK 约束**（`migrations/016_inbox_items.sql`），词表由应用层持有 → **新增告警类型零迁移**。§4 的级联删除也只动既有表的行 → **本批整体零迁移**。
4. **前端 item_type 词表有三处**：`features/inbox/index.tsx:70-71`（可见类型白名单）、`components/inbox-shell.tsx:1202-1203`（过滤器）、`components/inbox-item-list.tsx:39,48`（两个 label map）。三处都要补，漏一处该类型就会静默不显示。
5. **`/automations` 页没有 URL 选中参数**：`features/automations/index.tsx:85` 的 `selectedId` 是组件本地 state。告警深链要能落到具体规则，需给该页加 `?rule=<id>` search param 并初始选中——**这是本批唯一的额外前端改动**，别漏。
6. **词表停用影响面端点已存在**：`GET /api/v1/role-vocabulary/{roleKey}/references`（openapi.yaml:1463，schema `RoleVocabularyReferences` 在 :12417），返回引用该角色的剧本、持有员工、编制行数。前端停用弹窗已经在用。**员工侧缺同类端点**，§4.1 补。
7. **员工角色写入端点**：`PUT /api/v1/digital-employees/{employeeId}/roles`（openapi.yaml:4758），整套替换，描述里明写"即时生效，无需审批"。
8. **编制变更已写项目事件**：`project/casting.go:312` `AppendProjectEvent(ProjectEventConfigChanged, "project.casting.changed")`。级联解除复用同一事件类型、换 payload event 名。
9. **any-of-N 收件人口径**：项目负责人集合 = `human_owner_user_ids[]`（`human_owner_user_id` 是 primary 指针，不是唯一负责人）。飞书侧的 `listFeishuRecipientsWithQueries`（`project/feishu_outbox.go:65`）把 owner + active 人类成员都算收件人——**本批只发负责人集合，不发全体人类成员**，避免噪音。

---

## 4. A 封洞一：编制事实源双向闭环

### 4.1 预检端点（新）

```
GET /api/v1/digital-employees/{employeeId}/role-impact?role_keys=developer,reviewer
```

语义：**若把这些 role_key 从该员工身上移除**，哪些编制行会被解除。响应形状与 `RoleVocabularyReferences` 对称：

```jsonc
{
  "affected_castings": [
    {
      "project_id": "…", "project_name": "批二基线项目 P1",
      "scenario_template_key": "software_delivery", "template_name": "软件交付",
      "role_key": "developer"
    }
  ],
  "affected_count": 1
}
```

`role_keys` 省略 = 把该员工**全部**角色移除的影响面（供"清空绑定"用）。

### 4.2 写路径级联（改）

`PUT /digital-employees/{employeeId}/roles` 请求体增 `confirm_impact: boolean`（默认 false）：

- 计算本次替换会**减少**的 role_key 集合 → 查受影响编制行
- 有影响且 `confirm_impact != true` → **400**，body 带 `affected_castings`（前端据此弹预检窗）。理由与 `PutCasting` 硬校验同源：**不允许 API 绕过去写**
- 有影响且 `confirm_impact == true`，或无影响 → 同事务内：
  1. 替换角色绑定
  2. 删除受影响编制行
  3. 每个受影响项目写一条 `project.casting.invalidated` 事件（payload 含 role_key / employee_id / employee_name / 触发人）
  4. 给每个受影响项目的负责人集合发收件箱告警（复用 §5 的通道，`item_type=casting_invalidated`）

`rolevocab` 停用路径（`PUT /role-vocabulary/{roleKey}` status→disabled）同规则：`casting_count > 0` 且无 `confirm_impact` → 400 带影响面；确认后级联 + 事件 + 通知。前端弹窗已存在，只接 `confirm_impact` 与结果提示。

### 4.3 读路径自证（改，承重）

**即使级联做了，读路径也必须自己复查**——直改 DB、迁移遗留、并发窗口都能造出失真行，而这三处是所有下游判断的入口：

| 位置 | 现状 | 改成 |
|---|---|---|
| `casting.go:544` `ValidatePlaybookCastingComplete` | 只查员工 status | 增持有校验：员工不再持有该 role_key → 计入 missing |
| `casting.go:728` `missingRoles` | 见 casting 行即 `continue` | 复查持有 + 员工可用，任一不成立不算满足 |

同时把 `MissingCastingRoles` 的返回从 `[]string` 改为结构化：

```go
type CastingInvalidation struct {
    RoleKey      string
    EmployeeID   uuid.UUID  // 原选谁（可能为 nil = 从未编制）
    EmployeeName string
    Reason       string     // "not_cast" | "employee_unavailable" | "role_not_held"
}
```

理由：§5 的通知要"写明原因（缺哪个角色、**原选谁**、为何失效）"，字符串拼不出来。`automation.ProjectGateway` 接口签名随之改。

### 4.4 判据

- 移除角色 → 预检看得见 → 确认后编制行消失、项目事件可查、负责人收到告警
- **直改 DB 造一条失真行（不走 API）→ 剧本选择器该档位立刻显示缺角色、automation fire 失败**（这条是自证，比级联更重要）

---

## 5. A 封洞二：无人值守失败必须推到人

### 5.1 新增收件箱告警类型

照 `channel_alert` 先例（§3 第 1 条），零迁移：

| 字段 | 值 |
|---|---|
| `ItemType` | `automation_alert`（automation fire 失败）/ `casting_invalidated`（§4.2 级联解除） |
| `SourceType` | `automation_rule` / `project_casting` |
| `SourceID` | rule.ID / project.ID |
| `TargetUserID` | 项目负责人集合逐人一条（any-of-N） |
| `Scope` | `personal` |
| `Actions` | `[]`（告警无裁决动词，须在 `inbox/service.go:417` 的豁免条件里加上这两类） |
| `Priority` | `high` |
| `DeepLink` | `{"type":"automation_fire_failed","route":"/automations?rule=<id>"}` / `{"route":"/projects/<id>/config?tab=casting"}` |

`inbox/service.go:360-380` 的配对校验加两个 case。前端三处词表（§3 第 4 条）加中文标签：「自动化告警」「编制失效」，并同步 `lib/status-labels.ts`。

### 5.2 触发与自动关闭

- **触发点：`failFire`（`automation/service.go:599`）统一发**——它是 automation 唯一的失败出口，且连续失败会自动停用规则，那更该通知。`skipped_overlap` / `skipped_disabled` 不发。
- **消息文本**：标题「自动化规则执行失败：<规则名>」；摘要按 error_code 分支，casting 类用 §4.3 的结构化结果拼成「缺角色 operator（从未编制）；角色 developer 原编制 开发-A，已不再持有该角色」。
- **自动关闭**：下次 fire 成功时 `ResolveOpenItemsBySource(tenant, SourceTypeAutomationRule, ruleID)`——与 channel_alert 恢复即关的语义一致，不需要人手动清。
- **幂等**：`UpsertItem` 按 (target_user, source_type, source_id) 收敛，连续失败只刷新同一条，不堆卡。

### 5.3 依赖注入

`automation.NewService` 增一个 `AlertNotifier` 接口参数（`OpenRuleFailureAlert` / `ResolveRuleAlerts`），实现放 `app/` 层接 `inbox.Service`——与 watchdog 同层，避免 automation 直接依赖 inbox 包。

---

## 6. 顺带清掉的两条小账

- **R5 补验**（角色治理台 spec §9.1 唯一未验项）：停用一个被剧本引用的角色后，改版该剧本应被 `validateSpecRoles` 点名拒绝。本批改停用路径，顺手在 GATE 里走一次。
- **文档纠正**：批二 commit 声称「兑现基线 §4.7」为不实，本批 CHANGELOG 要显式写明"§4.7 的通知半边到本批才真正落地"，不要含糊带过。

---

## 7. D 工程收尾：E2E 收敛

> **实现备注（2026-08-06）**：落地时将完成定义收敛为「本批 GATE + 共享断言库 `assert-graph` + mjs≤6」。
> 12 个历史 `browser-casting-*` 场景**未**全量迁入 `--stage`（见 `scripts/e2e/README.md`）。
> 编制失效深链落地为 `/projects/{id}/config?tab=casting`（原方案 `?tab=config` 与现网 IA 不符）。
> 角色替换/词表停用的编制删除与主写在同一 DB 事务；项目事件与收件箱告警在 commit 后发送。
>
> **验收复检后的覆盖决定（2026-08-06，人类拍板 B 方案）**：复检发现 §7.2「对不上的不许删」
> 被跨过，逐个核对 13 个被删文件后重新分工——
> - 4 个自带 `@deprecated`、1 个是编排器：删除无损；
> - G5/G6/G7 降级为 Go 单测（`project/casting_closure_test.go`、`automation/casting_gate_test.go`）；
> - G8 与 **G12 的耐久产品面**进 suite（`automation-fire` / `sod`，均自恢复）；
> - G11 与 G12 的判定逻辑复核确认**已有** Temporal `workflow_test.go`（`ForcePendingReview` +
>   `RequestPlanRevisionReview` + 不 decompose）与 `activities_test.go` 覆盖，不重复造 E2E；
> - **G9 协调线程自动提请真路径仍无自动化覆盖**，已记 TODO。
>
> G12 需要新建探针剧本 `e2e_sod_probe`：现网已无可用 SoD 夹具（`operator` 无人持有且不得补发，
> `software_delivery` 的 developer+reviewer 已迁 `adversarial_review`）。这本身是个信号——
> 经典 `role_independence` 在现网剧本里已无实际执行样本，是否还在被真正执行值得单独确认。
>
> **§7.1 的 `assert-graph` 必须吃 `/task-graph` 的 nodes+edges**：首版接了 `/projects/{id}/tasks`，
> 而 `ProjectTask` 没有任何依赖字段——条件①永远无法求值，合法 `blocked` 反被误报，
> C10 反向只因夹具注入了 API 从不返回的字段名才"通过"。已改为真实边结构 + 零边即判失败。


### 7.1 建共享层

新建 `scripts/e2e/lib/`：

| 模块 | 内容 |
|---|---|
| `cp-client.mjs` | login / 带 cookie 的 fetch / JSON 断言；CP URL 与凭据走 env，有默认值 |
| `fixtures.mjs` | 项目、员工、角色 id **按名字查库/查 API 解析**，不再硬编码 UUID（`run-casting-design-full-suite.mjs:22-30` 是反面教材） |
| `assert-graph.mjs` | **图终态断言**：① 不存在 blocker 已全解却仍 `blocked` 的任务；② 每次批量派发后无"静默丢失"的任务（有 dispatch 失败必有对应事件） |

### 7.2 合并脚本

把 12 个 `browser-casting-*` 变体合成一个 `scripts/e2e/casting-suite.mjs`，用 `--stage=g9,g10,h2,h12` 选跑，缺省全跑。**每个 stage 结束强制调用 `assert-graph.mjs`**——这是上次漏检的直接补丁。

删除的脚本归 git 历史，不保留副本。判据：`ls scripts/e2e/*.mjs | wc -l` 从 19 降到 ≤6。

### 7.3 定位

E2E 套件**不进 `verify:*`**（需真实服务 + 模型 + 真库），但必须有单一入口和 `scripts/e2e/README.md` 一节写清前置条件与造数配方。`verify:*` 仍是提交前分层门禁，二者不互相替代。

---

## 8. E 路线图对齐

改基线 `2026-07-27-workspace-and-playbook-alignment-baseline.md`：

1. **§8 实施拆分索引补登记**批一/批二/批三/角色治理台/本收口批，各注明入库 commit 与状态。
2. **注明 #4 已被吃掉多少**：剧本归属与编制矩阵已由批二/批三落地，#4 剩余范围 = 中枢轻发起 + Prompt 模板降权改名 + 来源字段补齐后收敛项目对话框 + 项目允许剧本集。
3. **§6 地基事实增补**角色词表/编制/扩编的既成事实（词表为主、一角色一人、`PutCasting` 双向硬校验、扩编 = 带新编制的重规划、发现器触发闸），供后续会话免于重勘。
4. TODO.md 增两行：历史 9 条扩编决策未回填；automation 失败飞书推送待做。

**不做这一步的后果**：下一个接手会话读 §8 会以为路线还停在四份 spec 上。

---

## 9. 分期

| 切片 | 内容 | 可独立验收 |
|---|---|---|
| **P0a** | §4.3 读路径自证（含 `CastingInvalidation` 结构化） | 单测 + 直改 DB 造失真行的真实链路 |
| **P0b** | §4.1 预检端点 + §4.2 写路径级联（员工侧 + 词表侧） | 浏览器 + API |
| **P0c** | §5 收件箱告警（两类型 + 三处前端词表 + `?rule=` 深链） | 浏览器 |
| **P1** | §7 E2E 收敛 | 反向验证：故意造滞留 blocked，断言必须 FAIL |
| **P2** | §8 文档对齐 | 人读 |

P0a 必须先于 P0b——先让读路径自证，级联才是"顺手把脏数据清掉"而不是唯一防线。

---

## 10. 验收 GATE（真实 E2E）

| ID | 步骤 | 期望 |
|---|---|---|
| C1 | 对持有 `developer` 且已被编制的员工调 role-impact 预检 | 返回受影响编制行（项目名 + 剧本名 + 角色位），`affected_count` 正确 |
| C2 | 不带 `confirm_impact` 移除该角色 | 400，body 带影响面；编制行**未**被删 |
| C3 | 带 `confirm_impact` 重放 | 200；编制行消失；项目事件 `project.casting.invalidated` 可查；该项目负责人各收到一条「编制失效」收件箱项 |
| C4 | **直改 DB** 插一条失真编制行（员工不持有该角色，绕过 API） | 剧本选择器该档位显示缺角色；`ValidatePlaybookCastingComplete` 判为 missing（读路径自证，不依赖级联） |
| C5 | 停用一个被剧本引用的角色 | 弹窗列出引用剧本 + 持有员工数 + 编制数；确认后级联解除、候选中消失 |
| C6 | 停用后改版引用它的剧本（补验 R5） | `validateSpecRoles` 拒绝并点名该角色 |
| C7 | 让绑定剧本的 automation 规则在编制失效状态下触发 | fire failed；**每个项目负责人**各一条「自动化告警」收件箱项；摘要含缺哪个角色 + 原选谁 + 为何失效 |
| C8 | 修好编制后等下一次 fire 成功 | 全部相关告警自动 resolve，无需人工清 |
| C9 | 从告警深链点进去 | 落到 `/automations?rule=<id>` 并**选中该规则**（不是列表首项） |
| C10 | 跑新 `casting-suite.mjs` 全量 | 通过；且**故意造一个 blocker 已解仍 blocked 的任务时该套件必须 FAIL**（反向验证断言有效） |
| C11 | `verify:contracts` + `verify:control-plane` + `verify:web` | 全过（本批零迁移，`migrate-validate` 无变更） |

**完成定义**：C1–C11 全过。C4 与 C10 是本批的两条硬判据——前者证明读路径不再依赖上游守规矩，后者证明断言真的能抓到东西。

---

## 11. 风险

| 风险 | 缓解 |
|---|---|
| 级联删编制被当成"平台擅自改我的配置" | 预检 + `confirm_impact` 双确认 + 项目事件 + 收件箱告警，三处留痕 |
| 读路径加持有校验后，存量失真行让原本"可达"的剧本突然变"缺角色" | 这是**预期行为**（原本就是失真）。上线前跑一次全库失真行普查并在 CHANGELOG 列出受影响项目 |
| 告警刷屏（规则每小时失败一次） | `UpsertItem` 按 source 幂等收敛为一条；连续失败 N 次规则自动停用，天然封顶 |
| 删 12 个 E2E 脚本丢掉某个只在其中一个里覆盖的场景 | 合并前逐个脚本提取 stage 清单落进新套件的 `--stage` 列表，对不上的不许删 |
| `MissingCastingRoles` 签名变更波及 automation 与 project 两包 | 一次性改完并跑 `verify:control-plane`；接口只有一个实现（`ProjectServiceGateway`） |

---

## 12. 一句话方案

> **编制的两侧都要守：写入时校验持有，移除时级联并通知，读路径无论如何自己再验一遍；无人值守的失败必须推到人手上，而不是躺在 fires 表里等人翻；E2E 的断言要沉淀成库，否则下一次还会绿着漏。**
