# 团队生命周期收敛 Spec：撤归档/停用，单一删除 + 审核确认制

> 日期：2026-07-18
> 状态：**P1 已落地入 main（2026-07-18 20:50，GATE 四项全 PASS）**；P2 审核确认制待立项。§4 两开放问题已由用户拍板：存量直接清（dev 环境）、restore 端点直接删。实施中 E2E 揪出并修 DeleteTeam 不落审计的既有缺口（team.delete 与软删同事务）。
> 性质：团队状态机收敛 + 两阶段删除。P1 = 收敛与清理；P2 = 管理员审核确认的物理删除。
> 关联：团队删除绑定悬空修复（同日 9434338f，DeleteTeam 事务内清理已落地）；运行总览"不可见团队员工"缺口（`2026-07-18-run-overview-project-overlay-design.md` §7，本 spec 是其根治路径）。

---

## 0. 动因与现状（2026-07-18 实测）

### 三套状态并存的混乱

团队生命周期当前由**两个正交字段**表达，且互不感知：

- `status`：`active` / `archived` / `disabled`（API：`POST /teams/{id}/archive|disable|restore`；UI：团队详情页三个按钮，`allowed_actions` 驱动）
- `deleted_at`：软删标记（API：`DELETE /teams/{id}`，事务内解绑员工+清理技能/MCP 绑定+审计，不可恢复——`SetTenantTeamStatus` 带 `deleted_at IS NULL` 守卫，restore 只针对 archived/disabled）

实测 dev 库：27 个团队里 26 个 `deleted_at` 置位，但其中 25 个 `status` 仍是 `'active'`——软删不改 status，状态列成了尸体残留。此前一次误判（"admin 只能看到 1 个团队是授权 bug"）正是这套并存造成的：按 status 查像是有 23 个 active 团队被藏起来，实际它们全是已删团队。**admin 可见性无 bug**，teams API 正确过滤 `deleted_at`。

### 用户裁定

1. 归档/停用实质上是"隐性删除"——语义重复、状态残留、用户心智负担。**撤掉归档与停用，团队只有一种退出方式：删除。**
2. 删除默认非物理（状态变更，全站不可见），**必须有日志记录**（审计已具备）。
3. **P2 补审核确认制**：删除后进入"待确认"态（不可见），管理员可**恢复**或**确认删除**；确认才物理删除（物理删也留日志）。

### 顺带根治的缺口

运行总览"不可见团队员工"：员工 `team_id` 指向已删团队 → 既无工位也不入候岗大厅 → 地图不可见且与汇总数字不一致。增量路径已修（DeleteTeam 事务解绑员工 → 员工进候岗），但存量有 2 名悬空员工（报告员小王、审查员-csjw，挂在已删团队"abc"上）。生命周期收敛后，"员工指向非存活团队"只剩历史数据一种来源，配一次存量清理即闭环。

---

## 1. P1：收敛与清理

### 1.1 撤归档/停用

- **API**：下线 `POST /teams/{id}/archive`、`POST /teams/{id}/disable`；`restore` 在 P1 一并下线（其现有语义只服务 archived/disabled，P2 重生为"恢复待确认删除的团队"）。openapi 同步 + 契约验证流程。
- **UI**：团队详情页撤"归档/停用/恢复团队"按钮与对应确认弹窗；状态 pill 词表收敛（archived/disabled 视觉映射保留只读兜底，见 1.3 存量）。
- **allowed_actions / authz**：`team.archive`、`team.disable`、`team.restore` 从下发与决策点清单移除（注册表与决策点侧同步，不留死权限位）。
- **删除入口不变**：`DELETE /teams/{id}` 维持软删 + 事务清理（解绑员工→候岗、技能/MCP 绑定清理）+ 审计日志（均已落地）。

### 1.2 status 语义收敛

- 团队对外只有两种可观察状态：**存活**（唯一 status=`active`）与**已删除**（`deleted_at` 置位；P2 细分为 `pending_delete` 与物理删除）。
- 迁移：存量 `status IN ('archived','disabled')` 且未删除的团队（当前 dev 库为 0 个，生产按实际）→ 拍板口径：一律视为"仍存活"翻回 `active`（archived/disabled 本就是隐性删除的半途状态，翻活由管理员再决定删不删）。已删团队的 status 残留值统一刷为 `active`（`deleted_at` 是唯一删除事实源），或加 CHECK 约束禁止新写入 archived/disabled。
- `ProjectDemandStatus` 等其他领域枚举不受影响；`TeamStatus` 类型收敛后 web/api 类型同步 regen。

### 1.3 存量悬空员工清理

- 一次性迁移：`digital_employees.team_id` 指向 `deleted_at IS NOT NULL` 团队的 → 置 NULL（进候岗大厅），与 DeleteTeam 增量口径一致。当前 dev 库 2 名（已在立项当日先行以同口径 SQL 清理，迁移保证其他环境）。
- 运行总览侧无需改动：置 NULL 后员工自然落入候岗大厅（已验证路径）。

### 1.4 P1 GATE（真实 E2E）

- 团队详情页不再出现归档/停用/恢复按钮；对旧端点 curl 404/405。
- 删除团队 → 团队从列表消失、其员工出现在运行总览候岗大厅、审计日志有记录、技能/MCP 绑定清理（复用 9434338f 的验证面）。
- 存量悬空员工清理后，运行总览画面与汇总数字一致。

## 2. P2：审核确认制（后续立项实施）

- 删除进入 `pending_delete`：全站不可见（与现软删同效），但进入管理员"待确认删除"队列。
- 管理员两个动作：**恢复**（团队回 active，员工归属是否回填待定——倾向不回填，员工已进候岗由人工重新编排）与**确认删除**（物理删除行 + 级联清理，操作与结果落审计日志）。
- 未确认的 pending_delete 团队长期滞留策略（提醒/超时自动确认/永不自动）留到 P2 细化时拍板。
- 与审批中心/收件箱的集成形态（是否走既有 approval_requests）P2 拍板。

## 3. 非目标

- 不改团队成员/租户成员模型；不改"员工必须归属团队才能进项目"规则线（另有立项）。
- P1 不做删除审批流（删除仍即时生效），只收敛状态机。

## 4. 开放问题（P1 实施前确认）

1. 存量 archived/disabled 团队翻回 active 的口径（§1.2）——本 spec 按"翻活"写，生产若有大量真实归档团队需用户复核。
2. `restore` 端点 P1 直接删除还是保留返回 410 过渡；倾向直接删除（无外部集成依赖）。
