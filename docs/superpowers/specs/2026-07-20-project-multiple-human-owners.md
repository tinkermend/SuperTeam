# 项目多人类负责人（Multiple Human Owners）设计 spec

- 日期：2026-07-20
- 状态：待人类审阅（spec-only，未实施）
- 触发：用户明确项目管理里应支持**多个平级人类负责人**，都能审批、验收；现状单一 `human_owner` 锚点与之冲突，宪法需一并修订。
- 关联立项：`TODO.md` 2026-07-20「多人类负责人模型」条。

## 1. 背景与冲突

宪法（`CLAUDE.md`「协作模型」）当前写：

> 每个项目必须绑定**固定人类负责人（human_owner）**。项目人类成员同等身份，不划分 leader/验收人等子角色；**human_owner 是必绑锚点与决策兜底路由目标**。人类成员负责最终业务判断、审批、结果验收、驳回、补证要求、汇报接收和验收结论。

其中「人类成员同等身份…人类成员负责审批、验收」这一半**已经**符合用户模型（多个平级人类都能审批验收）。真正冲突的只有 **`human_owner` 是单一锚点 + 单一决策兜底路由目标** 这一处：数据上是单 FK `projects.human_owner_user_id`，语义上是"唯一那个负责人"。

用户的目标模型：一个项目可有**一个或多个**平级人类负责人（owners），**任一**都能审批 / 验收 / 驳回 / 接收兜底路由；**至少保留一个**（可增可删，不得删到 0）。

## 2. 关键先例：团队已经是多负责人

**不必从零设计**——团队侧早已做过同样的单→多转换，直接复用其形态：

- `migration 011_team_multiple_owners.sql`：`tenant_teams.human_owner_user_id UUID` → `human_owner_user_ids UUID[] NOT NULL DEFAULT '{}'`，回填 `= ARRAY[human_owner_user_id]`，再 `DROP COLUMN` 旧标量；config_revisions 同步。
- `tenant/service.go`：`HumanOwnerUserIDs []uuid.UUID`，创建校验 `len==0 → ErrInvalidInput`（即 ≥1 约束的现成写法）。
- `authz`：团队多 owner 授权已生效。

项目侧是唯一还停在单 FK 的实体。本 spec = 把团队的多 owner 形态搬到项目，并解决项目特有的**协调线程决策路由**这一处项目独有的承重点。

## 3. 现状承重点勘察（改动面清单）

`projects.human_owner_user_id`（单 UUID，`migration 003`）当前被以下路径依赖：

| 承重点 | 位置 | 用途 | 多 owner 后 |
|---|---|---|---|
| 决策路由目标 | `workflow/projectcoordination/project_store.go` `TargetUserID`（clarification / task_failure_recovery / acceptance_required 等 ~10 处） | 人类决策卡投给谁 | **本 spec 核心**：单目标 → 扇出/任一可决（见 §5） |
| 自动验收归属 | `project_store.go:2948` `acceptedBy` | 自动验收记录归属 | 归属到实际操作的 owner；自动兜底可记系统或首 owner |
| 派发归属 | `project_store.go` `DispatchUserID`（3181/3289…） | 自动派发的发起人归属 | 归属可取确定性首 owner 或系统 actor（低敏感） |
| 授权判定 | `authz/authorizer.go:562,615` `facts.HumanOwnerUserID == principalID` | 项目级权限点 | `contains(facts.HumanOwnerUserIDs, principalID)`（照团队多 owner authz） |
| Feishu 收件人 | `project/feishu_outbox.go:74` `∪ human_owner` | 通知/签署收件人 | `∪ 全部 owners`（数组后天然成立） |
| 成员耦合 | `project/service.go:1913,7389-7406` | human_owner 必须是一个 active owner-role human_user 成员；owner-role 成员必须是 human_user | 改为"每个 owner ↔ 一个 active owner-role human_user 成员"，多行 |
| 创建/配置校验 | `project/pg_repository.go` `human_owner_user_id` 必填、config 更新 section | 单值必填 | 数组必填且 ≥1 |
| 前端展示 | `apps/web` 概览「项目负责人」单值、config 契约 | 显示单 owner | 显示全部 owners（名称），增删 + ≥1 守卫 |

## 4. 数据模型

方案（照 migration 011）：

- `projects.human_owner_user_id UUID` → **`human_owner_user_ids UUID[] NOT NULL DEFAULT '{}'`**；迁移回填 `= ARRAY[human_owner_user_id]`（已有值全部非空），再 drop 旧列。
- **权威来源二选一**（推荐 A）：
  - **A（推荐，照团队）**：`human_owner_user_ids` 数组为权威；`project_members` 中 owner-role human_user 成员与数组**保持同步**（创建 / `replaceMembers` 同时维护两者，与今天维护单锚点的耦合逻辑一致，见 `service.go:1913/7399`）。路由/授权直接读数组，免 join。
  - B：只留 `project_members(role=owner, type=human_user)` 为权威，删标量，路由/授权处 join 派生。更"单一事实源"但每次路由都要 join，且协调线程 activity 读取成本上升。
  - 结论：取 A——数组是 owner 集合的去规范缓存，`project_members` owner-role 行是成员事实；二者由服务端在同一事务内同步（与现状单锚点同步同构，改造量最小）。
- 契约：`Project.human_owner_user_id`（string）→ `human_owner_user_ids`（string[]，minItems 1）；`CreateProjectInput` / `UpdateProjectConfigInput` 同步；重生成 gen + `verify:contracts`。
- 兼容过渡：保留一个**派生只读访问器 `PrimaryHumanOwner()` = 数组首元素（按加入时间稳定排序）**，供尚未迁移的归属类代码（DispatchUserID 等低敏感处）临时使用，避免大爆炸；路由/授权/校验必须走数组，不得走 Primary。

## 5. 决策路由（本 spec 的真正难点）

现状：每张人类决策卡有**单一** `TargetUserID`（投给 human_owner）。多 owner 后"都能审批"需要重新定义"投给谁 / 谁能决"。

三选一：

- **(A) 扇出 per-owner + 任一可决 + 兄弟自动关闭（推荐）**：一次人类决策对每个 active owner 各生成一条收件项，任一 owner resolve 即生效，其余幂等置为 superseded/auto-closed。复用既有 per-user 收件模型，**无需 inbox 改成组目标**。与已有「any-of-N 双人签署」模式同族（见 feishu any-of-N 立项），可借其收敛/去重经验。代价：需在 resolve 路径加"关闭同一决策的兄弟项"。
- (B) 组目标 + 任一 claim：决策卡 target = "项目 owners 组"，任一 owner 可 claim/resolve。需 inbox/decision 模型支持 group-target（`TargetUserID` 单字段 → target 概念泛化），改造面更大。
- (C) 保留单一 primary 路由 + 任一可 override：路由仍投 primary owner，但 authz 允许任一 owner 越权 resolve。改造最小，但"primary"重新引入用户想消除的不对称，且非 primary owner 收不到通知只能主动查，体验差。

结论：**取 (A)**。它满足"都能收到、都能审批"，且把改造收敛在"决策创建时按 owner 数量扇出 + resolve 时关闭兄弟项"两点，不动 inbox 底层 target 模型。需覆盖的决策族：clarification / missing_context / permission / recovery / runtime_recovery / plan_invalid / budget_approval / human_wait / acceptance_required（即 `project_store.go` 里所有以 `HumanOwnerUserID` 为 `TargetUserID` 的点）。

`acceptedBy` = 实际点验收的那个 owner；无人操作的自动兜底验收（若存在）记系统 actor 或 `PrimaryHumanOwner()`。

**协调线程存量兼容**：路由发生在 activity（读 `projectRecord` 实时 DB 值），不参与 replay，故存量长命协调线程在迁移后立即读到数组、无需 continue-as-new（与 CHANGELOG 46b5050f「activity 不参与 replay，存量线程立即受益」同理）。workflow 层若有基于单 owner 的分支才需 `GetVersion` 围栏——勘察确认路由均在 activity 侧，无 workflow 层分支。

## 6. ≥1 约束（可增可删，不得删到 0）

- 创建：`human_owner_user_ids` 非空且每个 id 是有效 human_user；否则 400（照 `tenant/service.go:81`）。
- `replaceProjectMembers` / owner 增删：**结果集必须仍含 ≥1 个 active owner-role human_user 成员**，且数组与之同步非空；否则 400（结构化 apierror，如 `project_requires_human_owner`）。这是新增护栏——现状单 FK 隐式保证 ≥1，多 owner 后需显式校验。
- 删除/停用最后一个 owner 的路径（成员替换、用户停用、离职）都要过这道闸。

## 7. 前端

- 概览「项目负责人」单值输入 → **owners 列表**：展示全部 owner（`ObjectRef` 名称(id)），支持增/删（删至 1 时禁用删除并提示"至少保留一位负责人"）。owner 选择走用户选择器（`UserSearchSelect` 已存在，需接用户搜索 API）。
- 成员 Tab：人类成员组里 owner 标注"负责人"徽标（已有 isOwner 逻辑，改为按数组判定）。
- 契约类型 `human_owner_user_id` → `human_owner_user_ids: string[]` 全前端调用点同步。
- 与「服务端补名」立项协同：owners 名称走服务端补名，别再堆客户端 join。

## 8. 宪法修订（CLAUDE.md「协作模型」）

将：

> 每个项目必须绑定固定人类负责人（human_owner）…human_owner 是必绑锚点与决策兜底路由目标。

改为（提案）：

> 每个项目必须绑定**至少一个人类负责人（human owner）**，可多个且平级；任一负责人都可审批、验收、驳回、补证要求、接收汇报与兜底路由。负责人集合不得为空（增删自由但至少保留一个）。人类决策默认扇出给全部负责人，任一处理即生效。

（保留"人类成员同等身份、不划分子角色"一句——它与多 owner 平级一致。）

## 9. 分期

- **P1 后端**：迁移（scalar→array，照 011）+ 契约 + 数组同步维护 + authz 数组化 + 决策扇出/兄弟关闭 + ≥1 护栏 + Feishu ∪ 全 owners。真实链路 E2E：造多 owner 项目 → 触发人类决策 → 确认每 owner 各收一项 → 任一 resolve → 兄弟关闭 + 下游释放；删 owner 至 1 时护栏 400。
- **P2 前端**：概览 owners 列表增删 + ≥1 守卫 + 成员徽标 + 契约类型迁移。真实浏览器 E2E。
- **P3 宪法**：更新 CLAUDE.md 协作模型条 + 相关护栏测试/文档。
- P1 与「服务端补名」立项有交集（owners 展示补名），可并轨。

## 10. 风险 / 未决

- **决策扇出的幂等与竞态**：两个 owner 同时 resolve 同一决策——需 resolve 走乐观锁/唯一约束，先到生效、后到识别"已被兄弟处理"而非报错（复用释放死路修复里的 supersede 容忍 0 行经验，CHANGELOG 46b5050f）。
- **通知放大**：N 个 owner × 每决策 = N 条收件/飞书。可接受（本就要"都能收到"），但飞书侧注意 any-of-N 去重。
- **DispatchUserID/acceptedBy 归属语义**：自动动作归属到"某个 owner"在多 owner 下语义变弱，建议长期改记系统 actor；本期用 `PrimaryHumanOwner()` 过渡即可，不阻塞。
- **迁移期读路径**：务必全量切换路由/授权到数组后再 drop 旧标量（或分两迁移：先加数组双写、后删标量），避免中途某路径仍读空标量。
- 待人类确认：决策路由是否接受方案 (A) 扇出（而非组目标 B）；owners 是否等同于"owner-role human 成员"（本 spec 取等同）。
