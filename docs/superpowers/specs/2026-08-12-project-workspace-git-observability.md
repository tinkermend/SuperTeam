# 项目工作区 git 状态可观测（含平台产物隐藏目录收敛）

- 日期：2026-08-12
- 状态：**已实施（定向测试绿；P1 刷新 API 真实链路已点通；P0 声明交付物派发 E2E 本轮未跑）**
- 性质：两批串行。**P0 前置批**收敛平台产物落点；**P1 主批**把「项目现在脏不脏、停在哪次提交」做成项目一等信息。
- 上游前置：
  - `docs/superpowers/specs/2026-07-27-workspace-and-playbook-alignment-baseline.md`——§6「变更采集」的地基事实、§8 第 3 项「变更范围可见」。**本 spec 重定义 #3，见 §3**
  - `docs/superpowers/specs/2026-07-23-project-directory-workspace-design.md`——稳定项目目录 = Provider CWD（§0.9 同目录并发不加锁，本 spec 不动）
  - `docs/superpowers/specs/2026-08-12-project-workspace-provisioning-model.md`——平台不碰 git 工作树（§5.7）、投影写 `.git/info/exclude` 不动用户 `.gitignore`（§5.8）、供给状态与主节点（§5.2）
  - 声明式交付物 v2（`deliverables/` 管道）、输出附件 spec §1（兜底附件白名单）

## 0. 人类拍板记录（2026-08-12 对话，逐条不得静默偏离）

1. **核心对象是「当前项目有没有未提交的东西，以及具体有哪些」**。项目的 git 状态是平台级核心指标——全部操作由平台发起，没有它就不知道任务到底提交了没有、有没有遗留。
2. **不给发起/派发链路加采集负担**。派发前、attempt 开始都不采。
3. 采集三点：**主动定时（展示的时钟）+ 手动刷新按钮（人要看当下）+ 任务完全结束收尾一次**。
4. 「脏」= 相对 HEAD 的 tracked 未提交改动 + 未跟踪文件，**排除平台产物**；平台产物应收进**隐藏目录**（Codex / Claude Code / OpenCode 每个项目建一个私有隐藏目录的惯例）。
5. **本轮只做项目级**；demand 级「这一单改了哪些文件」后置另立。
6. **P0 前置批先落**，指标随后——否则指标上线时要先写一段将来必删的路径特例。
7. 「这一棒到底提交没有」**不需要硬结论**，不为此保留开工基线快照。

## 1. 问题

平台是项目目录的唯一操作方，却看不见这个目录的 git 现场。今天最接近的两片都不成立：

| 现有 | 实际是什么 | 缺口 |
|---|---|---|
| 认领探测的 `dirty: bool`（`project_workspace.rs:277-288`，经 `workspace_provision.go:422` 落回执） | 创建向导里一次性探测 | 不进项目信息、无文件清单、此后永不更新 |
| attempt 结束的 `git diff HEAD`（`artifacts.rs:793`） | 未提交 tracked 的补丁，走证据工件 | 员工一 commit 就空，且空/非空从未变成项目级指标 |
| 卷宗右轨 `branch_ref` / `git_commit`（`service_dossier.go:981-984`） | handoff 自报的指针 | 是声明，不是平台量到的现场 |

项目上现有的 `WorkspaceReadyStatus`（`types.go:476-482`）只回答「目录在不在、能不能派发」，不回答里面脏不脏。

**并且现状有一个会直接废掉该指标的底噪**：`deliverables/` 采集完只上传、从不删（`executor.rs:2127-2139`），而终态清理明确跳过稳定项目目录（`workspace_cleanup.rs` 断言 "stable `{base}/{project_name}` must not enter attempt cleanup"）。于是凡产出过声明式交付物的项目，`git status --porcelain` 永远挂着 `?? deliverables/`。

**顺带定罪一个既有缺陷**：`collect_declared_deliverables`（`artifacts.rs:361`）扫整个 `deliverables/` 目录、无 attempt 归属、无基线对比。稳定项目目录长期共用 ⇒ **任务 A 的遗留文件会被任务 B 的采集当成 B 自己的声明式交付物重新上报**（证据串味）。这不是本 spec 引入的，是现在就在发生的。

## 2. 目标与非目标

### 目标

1. 平台产物统一收进项目内的**平台私有隐藏目录**，业务工作树零污染；声明管道按会话隔离，消除证据串味。
2. 项目一等信息新增两项可观测事实：**是否干净**、**当前 HEAD 哈希**；脏时给出**未提交清单**。
3. 该信息由**主动定时**养新（展示 SLA 靠它），另有手动刷新与任务终态收尾两个补点。
4. 调度在控制平面、执行在 Runtime；采样节奏可配。

### 非目标

- 逐行 diff / side-by-side 查看器（基线 §5 / §7 已否决）
- demand 级「这一单改了哪些文件 / 增删多少」（人类拍板 5，后置另立）
- 「这一棒提交了没有」的硬结论与开工基线快照（人类拍板 7）
- 内容指纹 / marker 文件 / 目录身份凭证（08-12 spec 不变量 1）
- 为采集加锁、串行化任务、或恢复 per-task worktree（07-23 §0.9 保留）
- 遗留未提交进收件箱告警（第一轮只做可观测指标；attached 目录长期脏，天天开卡会训练人忽略待办）
- 多节点状态聚合（只报主节点，见 §6 不变量 5）
- 改 attestation 四列语义（`executor.rs:2867-2870`；`git_base_ref` 现塞 repo binding 的 `default_branch` 名字、不是测量基线——**记债不动**，见 §9）

## 3. 对基线 §8 #3 的重定义（**需人类点头后由人类或本 spec 实施者改基线文件**）

基线 §8 第 3 项原文范围是「base 记录 + commit 后 diff + numstat 文件清单；attestation git 字段回填；右轨渲染器接真数据」。

**改为**：「项目工作区 git 状态可观测（干净与否 + HEAD + 未提交清单）；平台产物隐藏目录收敛。demand 级变更范围与右轨接真数据另立。」

理由（三条，均在 2026-08-12 对话中核实）：

1. 原 #3 的 `base` 落不了地。`git_base_ref` 拿到的是 `default_branch` 名字（`project_store.go:3354` → `runs.rs:66-68` → `executor.rs:2868`），不是 SHA；且 08-12 §5.7 之后平台不再 checkout，这个名字更不能当测量原点。
2. 稳定项目目录 + 不加锁 ⇒ 「哪几行是哪个员工写的」无法诚实归属。可诚实测量的是**目录的当前状态**。
3. 人类拍板把核心价值从「这一单改了什么」移到「项目现在有没有遗留未提交」——后者才是平台作为唯一操作方必须能回答的。

基线 §4/§5 的其它不变量与已否决项**不受影响**，本 spec 不重议。

## 4. P0 前置批：平台产物隐藏目录收敛

### 4.1 落点

`.superteam/` 已是平台在项目里的私有区，且**已有按会话分的子树**：

```
const SESSIONS_DIR: &str = ".superteam/sessions";   // project_session.rs:24
.superteam/sessions/{command_id}/mcp/claude.mcp.json
```

声明式交付物迁入同一棵树的**会话子目录**（建议 `.superteam/sessions/{command_id}/deliverables/`，最终名以实施时与既有常量一致为准）。选 `command_id` 而非 `attempt_id`：该子树已按 command 建立，且 chat 有 command 无 attempt，一套路径覆盖两类执行。

由此**同时**得到：

- `.superteam/` 与 `.superteam/**` 已被永久写入 `.git/info/exclude`（`project_workspace.rs:765-769`，append-only 幂等、会话结束不撤）⇒ 不需要为 `deliverables/` 新增任何屏蔽
- 采集范围天然限于本次会话子目录 ⇒ §1 的证据串味消失
- 员工不再看到 `?? deliverables/` ⇒ 不会误把平台管道产物 commit 进客户仓库

### 4.2 必须同步改的三处（漏一处即静默失效）

| 位置 | 现状 | 改为 |
|---|---|---|
| `projectcoordination/project_store.go:3715` | 指令文案「文件形态的交付物必须写入工作目录 deliverables/ 目录……」 | 指向新路径；**只提那一个输出子目录**，不得把 agent 领到 `.superteam/` 根（同树有 MCP 配置，可能含凭据） |
| `artifacts.rs:551-557` `has_excluded_component` | 「路径任一段以 `.` 开头即整条排除」（理由见 E2E 2026-07-19 抓到 `.superteam/mcp/claude.mcp.json` 被当附件） | 声明管道的隐藏判断改为**相对声明根**（根内部仍排除隐藏文件）；否则新路径下一个交付物都采不到 |
| `project/task_result_contract.go:528-529` | 按 `deliverables/` 前缀剥离后匹配 declared 工件 | 兼容新前缀；存量已入库的 ref 仍是旧前缀，解析须两者都认 |

### 4.3 过渡与清理

- **双读**：新路径为主；旧 `deliverables/` 若有文件**仍然采集**（丢交付物比串味更坏），但标 `legacy_path` 来源并产出**可见提示**——不得静默兼容。撤销条件：线上不再出现该提示。
- **保留策略**：隐藏 ⇒ 看不见 ⇒ 会静默长大，而稳定项目目录从不被清理（既有 janitor 只管 `{base}/workspaces/` attempt 目录与 chat 线程目录）。本批必须给 `.superteam/sessions/` 输出子树一个保留上限（按会话数或总量，形状照既有 janitor）。
- **兜底附件不搬**：那是 agent 在任意位置产生的无计划文件，路径不可预测。它们继续作为未跟踪文件**计入脏**（人类拍板 4）。改完后分工是：**走声明管道的不吵，散在仓库里的算脏**——这让「脏」对员工产生正向约束。

## 5. P1 主批：项目工作区 git 状态

### 5.1 对象与语义

一句话：**最近一次观测到的项目目录 git 状态**，带采样时间；人能看出这份是不是旧的。

| 字段 | 语义 |
|---|---|
| 是否 git 仓库 | 非 git 项目（`source_kind=directory`）⇒ 指标**不适用**，不是「干净」 |
| 干净与否 | 相对当前 HEAD：无 tracked 未提交改动、无未跟踪业务文件（平台私有隐藏目录整棵不参与） |
| HEAD 哈希 | `rev-parse HEAD` 实际提交 ID（承重项；分支名会漂、只作附属，detached 如实标） |
| 仓库状态 | `ok` / `detached` / `rebase 中` / `merge 冲突` 等中间态——「卡在半路」与「有遗留」对人的行动指引不同，不得糊成一个「脏」 |
| 未提交数量 | 瘦字段，列表页可用 |
| 未提交清单 | 路径 + 类别（已修改/已暂存/未跟踪/已删除/重命名）；**必须截断**（前 N 条 + 「另有 M 个未列出」，形状照 `artifacts.rs:588` 的 5000 兜底） |
| 采样时间 / 采自哪个节点 / 未采到原因 | 诚实边界：探测失败绝不写成「干净」 |

### 5.2 采集三点（均只读）

```
定时（主时钟）   CP 看门狗按 sampled_at 挑项目 → 下发 probe → 回执写快照
手动刷新（人）   同一条链路，强制绕过 sampled_at 阈值
任务完全结束     task 落终态（completed/failed/cancelled）后异步补一刀
```

- 派发前 / attempt 开始**不采**（人类拍板 2）。
- 「完全结束」= **task 终态**，不是每个 attempt；retry 三次只在最终终态采一次。
- 收尾采集 **best-effort**：节点离线/超时只记跳过，**不影响任务成败、不进写回事务关键路径**（与 attestation「git 采集失败降级 None」同原则）。

### 5.3 调度在控制平面，执行在 Runtime

git 只能在有盘那侧读，所以执行必然是 Runtime 的现成命令 `probe_project_directory`（`workspace_provision.go:21`、`executor.rs:947`）。**节奏归 CP**：挑哪些项目、多久一次，依赖「已供给 / 主节点是谁 / 是否 git / `workspace_ready` / 租户 / 上次采样多久前」——全是业务状态，且 Runtime 自扫需要 CP 把项目目录清单下发进心跳（工作区根下还有 `employees/`、`chat/`、legacy `workspaces/`，不能盲扫），会把业务清单塞进 `contracts/runtime/openapi.yaml`（CLAUDE.md 记的已知债，不在自动门禁内）。

**判据放数据上，不放定时器上**：ticker 只负责唤醒，每轮取「已就绪 + 主节点在线 + `sampled_at` 早于阈值」的项目，取前 N 个探测。由此多 CP 实例各跑一份也无害（只读幂等，且抢不到已新鲜的项目）、重启不用补偿、积压后续轮次消化、空闲项目靠大阈值自然降频。形状照 `app/stuck_task_reconciler.go`（ticker + 逐轮批量上限 `stuckTaskReapBatchLimit = 50` + 失败只记日志下轮重试）。

### 5.4 探测本身要改的两处

1. **不得抢 `index.lock`**：`git status` 会顺手刷新并写 index。一次性探测无所谓，但每分钟一拍且项目目录里同时有员工在 `git add` / `commit` 时会互撞——员工侧偶发失败，排查会误指 provider。必须 `--no-optional-locks`（或 `GIT_OPTIONAL_LOCKS=0`）。现状 `project_workspace.rs:277-288` 是裸命令。
2. **回执结果扩展**：现有 facts 已有 `dirty` / `head_commit` / `current_branch` / `detached`（`workspace_provision.go:300-311`），本批补未提交清单、仓库中间态、截断标记。回执是自由 map，不改回执结构。

### 5.5 数据与契约

- **存储**：`projects` 主表不加十余列。用 1:1 侧表存最新快照（覆盖写，清单落 JSONB）；列表页 LEFT JOIN 取瘦字段。**API 上仍挂在项目对象里**（人类拍板 1「放在项目信息内」是产品面要求）。表名/列名/索引须对 `DATABASE_DESIGN.md`；迁移只进 `internal/storage/migrations/`，改后校验 `atlas.sum` / `migrate-validate`。
- **契约**：`contracts/control-plane/openapi.yaml` 项目对象增该块 + 手动刷新端点（命名随既有路由风格，参考 `markProjectWorkspaceReady` 一族）；走 `generate:control-plane` + `verify:contracts`。
- **Runtime 命令**：`probe_project_directory` 载荷/回执扩展，属已知债范围，改后人工核对下游。
- **系统配置**：只加**一个**采样间隔键（`DomainExecution`，命名随 `runtime.heartbeat_timeout_seconds` / `task.stuck_running_timeout_seconds` 形状，见 `systemconfig/registry.go`）；空闲降频用内部固定倍数，单轮批量上限当常量。先不要两个键。
- **回执膨胀**：定时这一路每项目每拍写一行 command receipt = 把回执表当日志写。二选一并写死：定时路径不落 receipt（只手动刷新与收尾留痕），或纳入既有 retention 清理（`internal/retention/`）。**不定死则上线一周先炸的是这张表。**

### 5.6 Console

- 项目详情：脏/净 + HEAD（短哈希）+ 采样时间（相对时间）+ 「刷新现场」按钮；脏时可展开未提交清单
- 项目列表：瘦徽标（脏/净/不适用/未采到），**不触发探测**
- 手动刷新是**异步**的（走命令通道，Runtime 领取后执行再回执）：需要 pending 态、同项目在飞节流、失败/离线降级为「节点离线，显示的是 X 分钟前的现场」。可用既有 `dispatchProjectWorkspaceCommand` 的 wait+短超时，超时转异步提示
- 权限：只读探测，项目读权限即可（靠节流防滥用）
- 全部文案进 `apps/web/src/lib/status-labels.ts`（脏/净/不适用/未采到/中间态/文件类别）；样式先读 `DESIGN.md`

## 6. 承重不变量（不得「顺手优化」掉）

1. 探测**只读**，且**不写 index**（`--no-optional-locks`）。这是不变量不是实现细节。
2. 快照按 **`sampled_at` 单调**写入，旧回执静默丢弃。否则人点刷新会看到状态倒退。
3. 探测失败 / 节点离线 ⇒ **标未采到 + 保留上次快照**，绝不写成「干净」。
4. 非 git 项目 ⇒ **不适用**，不是干净。
5. **只报主节点**，标明来源节点，**不做多节点聚合**（存量多节点项目磁盘上仍有陈旧副本，08-12 spec §10.5 明说不追溯清理）。
6. 平台产物**只写 `.superteam/` 子树**；**不得为任何业务路径新增 git 屏蔽**。特别是不得对业务路径用 `update-index --skip-worktree`（`project_workspace.rs:771-784` 的回退对技能软链安全，对业务目录会让真实改动集体沉默——那是指标骗人，比有底噪更严重）。
7. 收尾采集 best-effort，**不影响任务成败**、不在写回事务关键路径上。
8. 定时路径**不得把 command receipt 当日志写**（§5.5 二选一）。
9. 旧 `deliverables/` 路径双读期**必须可见提示**，不得静默兼容。
10. 未提交清单**必须截断且显示总数**。
11. 不改 attestation 四列语义；不做 demand 级变更范围（本轮范围外）。

## 7. 分期与并行约束

**严格串行 P0 → P1。** P0 改的是「脏」的定义前提；先上 P1 等于先写一段将来必删的路径特例代码（人类拍板 6）。

| 文件 | P0 | P1 |
|---|---|---|
| `runtime-agent/src/artifacts.rs` | ✓ | |
| `runtime-agent/src/project_session.rs` | ✓ | |
| `runtime-agent/src/project_workspace.rs` | | ✓ |
| `runtime-agent/src/commands/executor.rs` | ✓ | ✓ |
| `runtime-agent/src/workspace_cleanup.rs` | ✓ | |
| `internal/workflow/projectcoordination/project_store.go` | ✓ | |
| `internal/project/task_result_contract.go` | ✓ | |
| `internal/project/workspace_provision.go` | | ✓ |
| `internal/app/`（新 reconciler + app.go 接线） | | ✓ |
| `internal/storage/`（迁移 + sqlc） | | ✓ |
| `apps/web` 项目详情 / 列表 | | ✓ |

**开工前必做**：本 checkout 会话开始时 `git status` 显示在途未提交改动，且与 P1 高度重叠——`internal/app/stuck_task_reconciler.go`、`internal/project/pg_repository.go`、`internal/project/types.go`、`internal/storage/queries/project.sql`、`apps/web/.../project-operational-detail.tsx`，以及两个未跟踪的新 reconciler 文件。照 `2026-08-11` spec §8 的做法：对上表文件跑 `git status --porcelain <路径>` 确认在途批次是否落地；**不要**替在途那位 stash / commit / 切分支。**一会话一 worktree**（`docs/PARALLEL_DEVELOPMENT.md`；未共享 `SUPERTEAM_DEV_PID_DIR` 时从 worktree 执行 `restart` 是退出码 0 的静默空操作，会产出假的验证结论）。

## 8. 验收门槛（真实端到端，单节点可完成）

本 spec **不接受单测级完成**——与 08-12 那批被双节点环境卡住的判据不同，这里单节点就能走全链路。

**P0**

1. 派发一个会产出声明式交付物的任务 → 文件落在 `.superteam/` 下的会话子目录；`git status --porcelain` **干净**（无 `?? deliverables/`）；工件在 Console 可取回
2. 同项目**再跑第二个**任务（不产出交付物）→ 其声明式交付物**为空**，不再重报上一个任务的文件（串味修复）
3. 手工在旧 `deliverables/` 放一个文件 → 仍被采集、标 `legacy_path`、有可见提示
4. 会话结束后保留策略生效：输出子树不无界增长

**P1**

5. 真实项目目录里手改一个 tracked 文件 → 等一拍 → 项目页显示**脏**、清单含该文件、采样时间刷新
6. 员工在会话内 `git commit` → 下一拍显示**干净**且 HEAD 哈希变化
7. 点「刷新现场」→ 立刻（或短超时内）更新；连点被节流
8. 停掉主节点 → 页面显示**上次快照 + 过期标注**，不是「干净」
9. 非 git 项目 → 显示**不适用**
10. 制造 rebase 中间态 → 显示中间态而非单纯「脏」
11. 定时探测与员工会话内 git 操作并发 → 员工侧无 `index.lock` 失败（`--no-optional-locks` 生效）
12. 回执/存储口径按 §5.5 抽样核对，确认未把回执表当日志写

真实 E2E 期间须记录并复核 `control-plane` / `web` 的 pid，`owner=` 异源则结论作废（CLAUDE.md）。

## 9. 记债与后续（写入根目录 `TODO.md`）

- demand 级「这一单改了哪些文件 / 增删多少」范围感知——人类 2026-08-12 明确后置
- `attestation.git_base_ref` 语义偏差：现塞 repo binding 的 `default_branch` 名字，不是测量基线。本轮不动
- 卷宗右轨 `branch_ref` / `git_commit` 仍是 handoff 自报声明，未接平台测量事实

## 10. 风险与接受项

1. **未跟踪文件计入脏**是人类拍板的口径：agent 产出的散落 `report.html` 之类会让项目显示脏。接受——由人判断是否收进仓库；想不吵就走声明管道
2. **attached（人工认领）项目可能长期脏**：目录由人维护，平台只如实报。因此第一轮不做告警（§2 非目标）
3. **主节点离线期间指标停更**：显示过期快照。与 08-12 §10.1 接受的「项目停摆」同源
4. **隐藏目录降低本地可发现性**：人在节点上手翻不易看到产物。缓解——取回路径本来就是平台（presigned GET），本地可发现性不承重
5. **agent 被指向隐藏目录写文件**：`.superteam/` 同树有 MCP 配置。指令只提输出子目录；agent 本就有 shell，这是软边界不是隔离
6. **旧路径双读期存在一次串味窗口**：`legacy_path` 标记 + 可见提示换取「不丢交付物」
