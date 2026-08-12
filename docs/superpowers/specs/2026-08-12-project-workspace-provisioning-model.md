# 项目工作区供给模型：来源三态 · 绑定与供给分离 · 供给与撤销均经人工确认

- 日期：2026-08-12
- 状态：**已实施**（P0–P3 串行落地 2026-08-12）
- 上游前置：
  - `docs/superpowers/specs/2026-07-23-project-directory-workspace-design.md`——项目目录主链、工作区根、首启就绪、粘滞亲和。本 spec **有意推翻**其 §0.4 / §0.10 / §0.15 / §2 / §5.3 的若干条，逐条见 §3
  - `docs/superpowers/specs/2026-06-29-project-code-workspace-runtime-affinity-design.md`——项目锚定节点、员工漂浮；绝对路径非业务事实（**本 spec 不动这条**）
  - `docs/superpowers/specs/2026-07-18-team-lifecycle-convergence.md` 一族——团队 `pending_delete` → 管理员确认 → 物理删 + 滞留催办，本 spec 的删除确认队列照抄这套形状
  - CLAUDE.md「已知债」：`contracts/runtime/openapi.yaml` 未纳入自动契约门禁，改 runtime 命令契约须人工核对下游
- 人类拍板记录（2026-08-12 对话，逐条不得静默偏离）：
  1. 第三种来源 = **认领工作区根下已存在的目录**；用户只填**目录名**，不填绝对路径（绝对路径方案明确否决）
  2. 漂移判据 = **目标节点上有该目录即认为相等；没有则任务发不出去**。不做内容指纹、不写 marker 文件、**不为 attached 单立比平台自建更严的标准**（平台自建目录的内容同样是人填的，平台从来不验）
  3. 多节点**不预先 clone / 不预先 mkdir**；主节点挂了发起人工确认再供给到备节点；**接受「一段时间内项目不可用」**
  4. **删项目不直接删目录**：产生「删除目录」确认待办 → 确认后级联删 → 拒绝则平台放手。此条对**全部来源**生效，不只 attached
  5. 平台**不碰 git 工作树**：不 fetch / pull / push / sparse-checkout / 自动 checkout 分支 / 不屏蔽 provider 配置。git 的推拉由**场景模板定义的数字员工角色**（测试、推送等）在会话内自行执行——这是分层，不是缺口（详见 §5.7）
  6. 确认人当前 = **管理员**；「项目负责人 / 管理员」独立角色待角色权限体系完善后再做（已入 `TODO.md`）

---

## 1. 触发

人类提出创建项目应有第三种源码来源——「加载已存在的项目」（类比 Claude Code / OpenCode 打开已有目录），限定条件是不给本地文件选择器、只让用户输入目录，完成时探测目录是否存在、是否 git 仓库。

核查后确认：这不是「多加一个选项」，而是撞上 `2026-07-23` spec 明文写下的非目标（attach 语义），并连带暴露出**现有多节点供给模型的一个无声缺陷**（§2.3）。人类拍板一并整改，范围因此从「加一种来源」扩为「工作区供给模型」。

---

## 2. 现状核查（as-is，逐条带代码坐标）

### 2.1 来源只有两种，attach 被硬拒

- Console 只有 `directory`（非 Git 空目录）/ `git` 两种：`apps/web/src/features/projects/components/create-project/create-project-draft.ts:28,48`、`project-basics-step.tsx:99-100`
- Runtime 侧对已存在的非空目录**明确拒绝认领**：`apps/runtime-agent/src/project_workspace.rs:156-170`，错误文案即 `project directory already exists (will not attach)`
- 设计层面是刻意的：`2026-07-23` spec §0.15「**不认领节点上已有同名目录**（已存在即冲突）」、§2 非目标「不认领/绑定节点上『已存在的任意业务目录』（attach 语义）」

**结论**：要做第三种来源，必须显式推翻这两条，不能当作增量。

### 2.2 删项目：DB 软删，磁盘硬删（不对称）

- `DeleteProject` 先向各节点下发删目录命令（`apps/control-plane/internal/project/service.go:1557`），再 `SoftDeleteProjectCascade` 只置 `deleted_at`（`pg_repository.go:455`）
- Runtime 侧是 `std::fs::remove_dir_all`（`project_workspace.rs:174-185`）
- 摘 Runtime 节点同样立即删目录：`service.go:625`
- 现状注释已经在认命：「目录回滚不净不挡软删;人工收尾」（`service.go:1563`）

**结论**：现状是「数据可恢复、代码不可恢复」，正好反了。

### 2.3 多节点：全节点预先 clone，且平台从不 fetch / push（**无声陈旧**）

- 创建时向**所有**绑定节点 fan-out clone：`workspace_git_clone.go:36-38`（`enqueueProjectGitClone` → `dispatchProjectGitClones`，循环全部 `runtimeNodeIDs`）
- 加节点时立刻给新节点 clone：`service.go:584`
- 稳定目录路径上，`ensure_git_in_stable_project_dir` **只在 `.git` 缺失时 clone 一次**，此后再不碰 git 状态：`project_workspace.rs:437-480`
- 全仓唯一的 `fetch` 在 `ensure_repo_cache`（`project_workspace.rs:497,519`），那是 §8 历史兼容的 `repos/{project_id}` 缓存路径，稳定目录永不走。**`push` 一个都没有**

于是真实图景：节点 A 干了几周活，提交只在 A 的磁盘上；节点 B 的 clone **自绑定那一刻永久冻结**；A 挂掉后 `filterCandidatesByWorkspace(requireGit=true)` 看到 B 有 `.git` → **校验通过** → 数字员工在一份冻龄代码上继续干活，全程无红灯。

**结论**：不是「同步可能异常」，是**没有任何同步机制，且失败是无声的**。

### 2.4 加节点无条件 mkdir → 「有目录」这个判据是平台自己造的

`AddProjectRuntimeNode` 对新节点无条件下发 mkdir（`service.go:577`）。于是人类拍板的漂移规则（「有目录即相等」）在今天会被平台自己污染：运维一绑 node B，平台就把「有」造出来，人什么都没放。非 git 项目 `requireGit=false`（`node_resolver.go:153`），派发前校验只查「路径存在 + 是目录」（`project_workspace.rs:258-279`），**空目录一路绿灯**。

另有一条同向的：runtime 侧 `resolve_project_workspace` 有防御性 `create_dir_all`（`project_workspace.rs:58`），任何绕过 CP 校验的路径上「目录缺失」都会被静默补成空目录。

### 2.5 reclone = force → 先 `remove_dir_all`

`RecloneProjectWorkspace` 传 `force=true`（`workspace_git_clone.go:359`）→ runtime 先 `remove_dir_all` 再 clone（`project_workspace.rs:207-219`）。这不是「删项目」路径，却同样是无声删盘。

### 2.6 可直接复用的既有件（本 spec 不新造轮子）

| 件 | 坐标 | 复用点 |
|---|---|---|
| `validate_project_workspace` 命令（带 `require_git`） | `commands/executor.rs:886`、`workspace_provision.go:262` | 探测与派发前校验 |
| 命令回执 `Result` 是自由 map | `workspace_provision.go:52` | 探测事实增量返回，不改回执结构 |
| 团队 `pending_delete` 全套 | `tenant/service.go:406-440`、`inbox/types.go:67`、`app/team_pending_delete_reminder.go`、迁移 076/078 | 删除确认队列的形状 |
| `NodeResolutionReasonWorkspaceUnavailable` | `node_resolver.go:163` | 「项目暂不可用」的派发失败原因码 |
| `ProjectRuntimePlacementStatusWorkspacePending` | `types.go:540` | 「已绑定未供给」的就绪面展示位 |
| `WorkspaceReadyStatus`（pending/ready/error） | `types.go:471-478` | 项目级工作区可用性轴（**不新增 disable，不复用业务态 `paused`**） |
| 目录名校验 | `project_name.go:38-62` | attach 只填目录名 ⇒ 路径逃逸面天然收敛 |

---

## 3. 对 `2026-07-23` spec 的推翻清单

该 spec §0 标注「已拍板决策（不再重议）」，本 spec 是**有意重议**，逐条点名：

| 原决策 | 本 spec | 理由 |
|---|---|---|
| §0.4 创建成功门闩 = **全部**关联节点 mkdir 成功 | 门闩 = **主节点**供给成功 | 绑定 ≠ 供给（§5.2） |
| §0.10 加节点：同等 mkdir（Git 则 clone） | **撤销**——加节点只加候选资格，不碰磁盘 | §2.3 / §2.4 的无声陈旧与假可用 |
| §0.15 / §2 不认领已有目录（attach 为非目标） | **推翻**——新增 attach 来源（限工作区根下、按目录名） | 人类拍板 1 |
| §5.3 主节点离线 → 直接考虑其它已绑定且校验通过的节点 | 主节点离线 → 工作区不可用 + 发起**供给确认**待办 | 人类拍板 3 |
| §5.4 删除项目/摘节点 → 级联删目录 | 一律改为**删除确认待办** | 人类拍板 4 |
| §0.7 主节点存活则一直派它，主挂才落备用 | **保留**，但「备用」必须先被供给 | — |
| §0.2 绝对路径不是业务事实 | **保留不变** | attach 只填目录名，模型不动 |
| §0.9 同目录并发不加锁 | **保留不变** | — |

---

## 4. 目标与非目标

### 目标

1. 创建项目支持第三种来源：**认领工作区根下已存在的目录**（只填目录名 + 选节点 + 探测）
2. 把「绑定节点」与「供给工作区」拆成两件事，供给恒为一次**人工确认过**的动作
3. 主节点不可用时，项目工作区进入**不可用**并发起供给确认待办；不再静默漂移到陈旧副本
4. 删除磁盘目录（删项目 / 摘节点）一律经**管理员确认队列**，与团队删除同形
5. 平台对 git 工作树**只读不写**；`repo_binding` 对 attached 退化为观测事实

### 非目标

- 不支持任意绝对路径认领（明确否决；如日后要，须先有「允许认领根」白名单 + canonical 校验，另立 spec）
- 不做内容指纹 / marker 文件 / 目录身份凭证（人类拍板 2）
- 不做自动 push / pull / 定期同步；不做跨节点工作区同步
- 不新增项目 `disable` 状态，不复用业务态 `paused` 表达磁盘可用性
- 不做角色体系（确认人固定为管理员，见 `TODO.md`）
- 不改「项目锚定节点、员工漂浮」总原则，不把绝对路径写入业务表

---

## 5. 模型

### 5.1 来源三态

| 来源 | Console 标签 | 目录名 | 供给动作 | 初始 ready |
|---|---|---|---|---|
| `none` | 非 Git（空目录） | 手填 | 主节点 mkdir | `ready` |
| `git` | Git 仓库 | 默认由 URL basename 推导（`DirectoryNameFromGitURL`） | 主节点 clone | `pending` → clone 成功转 `ready` |
| `attach`（新） | 认领已有目录 | 手填，须与节点上已存在目录同名 | 主节点**探测**（不写盘） | 探测通过即 `ready` |

三者共用同一套目录名约束与全局唯一索引（`ValidateProjectDirectoryName`、`uq_projects_directory_name_active`），因此**同名撞车天然不可能**，无需新的去重机制。

### 5.2 绑定 ≠ 供给（本 spec 的核心）

- **绑定**（`project_runtime_nodes`）= 候选资格，纯元数据，**不碰磁盘**
- **供给** = mkdir / clone / 认领探测，常态**只对主节点做一次**
- 每条绑定记录带供给状态：`unprovisioned | provisioned`（+ `provisioned_at`、`provision_source`）
- 只有 `provisioned` 的节点才进派发候选；`unprovisioned` 在就绪面按 `workspace_pending` 展示

### 5.3 供给的发起与确认

| 场景 | 谁发起 | 是否需确认 |
|---|---|---|
| 创建项目 → 主节点 | 创建流程 | 否（创建本身即人的意图） |
| 绑定新节点 | 运维 | 否——**只绑定，不供给** |
| 主节点不可用 → 备节点 | 派发失败时**懒触发**，幂等 | **是**（管理员确认） |
| 人工主动预供给备节点 | 管理员在项目配置页 | 是（即确认动作本身） |

懒触发而非后台看门狗：项目不可用只有在有人要派发时才有业务意义，避免空转告警。同一 (project, node) 的待办幂等，不重复开卡。

**供给确认待办的文案必须包含**（否则人在盲签）：

> 节点 B 上将得到 `{repo_url}` 远端当前内容；**节点 A 上未推送的改动不在其中**。

attached 项目的供给确认则说明：平台**不会**在 B 上创建或填充目录，需人工先把目录放好，确认后仅做探测。

### 5.4 漂移与不可用

主节点不可用时：

1. 派发按既有 `filterCandidatesByWorkspace` 过滤（`node_resolver.go:205-213`）——`unprovisioned` 节点不在候选内
2. 无可用候选 → 派发失败，原因 `NodeResolutionReasonWorkspaceUnavailable`（既有码）
3. 同时幂等开出「供给到节点 B？」待办
4. **项目级 `workspace_ready_status` 不因单节点抖动翻转**（保留 `2026-07-23` §0.8 的松模型）；仅当全部候选皆不可用且供给待办被拒绝时置 `error`

**接受**：主节点挂到人确认之间，项目不可发起任务（人类拍板 3）。

配套改动（让「有目录 = 相等」这条判据的输入不被平台污染）：

- `AddProjectRuntimeNode` **不再 mkdir / clone**（撤 `service.go:577,584`）
- attached 项目关闭 runtime 侧防御性 `create_dir_all`（`project_workspace.rs:58`），缺失即失败，与 CP 侧校验一口径
- 派发前校验对 attached 保持现状强度（存在 + 是目录），**不加非空要求**（人类拍板 2）

### 5.5 撤销供给与删除确认

删磁盘目录的**全部**入口收敛到一条确认队列：

| 入口 | 现状 | 改为 |
|---|---|---|
| 删项目（`service.go:1557`） | 立即 `remove_dir_all` | 每节点一条「删除目录」待办 |
| 摘 Runtime 节点（`service.go:625`） | 立即 `remove_dir_all` | 该节点一条待办 |
| 重新 clone（`workspace_git_clone.go:359`, force） | 无声删盘 | attached **禁用**；`git` 来源保留但需二次确认 |
| 创建失败回滚（`rollbackCreatedProject`） | 立即删 | **保持直接删**——删的是平台刚建的必然为空的目录，进队列只制造噪音 |

队列语义（照抄团队 `pending_delete`）：

- 记录粒度 = **(项目, 节点) 一条**，项目多节点则多条
- 记录必须**自带快照**（`directory_name`、`node_id`、`repo_binding` 摘要、`ownership`）——项目已软删，不能靠 join 活行取值
- 确认 → 逐节点级联删；某节点离线 → 该条留队等节点回来（幂等重试），**不整体失败**
- 拒绝 → **平台放手**：写审计、队列项关闭、不再管这个目录、**不留悬挂待办**（避免团队那套要额外写 `ResolveOrphanPendingDeleteReminders` 的老问题）
- 滞留催办进收件箱，新增 `ItemTypeProjectWorkspacePendingDelete`（命名对齐 `inbox/types.go:67`）
- 确认人 = **管理员**

**由删除延后引入的新缺陷（必须一并处理）**：`uq_projects_directory_name_active` 是 partial 唯一索引，项目软删后目录名**立即可被复用**，而磁盘目录可能还在待删队列里。若新项目此时 attach 同名目录，会拿到旧项目的残留内容。**处理**：目录名占用判定须并上待删队列——队列中存在同 `directory_name` 的未决记录时，拒绝新项目使用该名（错误文案指向队列）。

### 5.6 `ownership` 的（有限）作用

新增 `projects.workspace_ownership`：`platform_managed | attached`。删除路径**不再**依赖它做判据（一律走确认队列），它只剩两个用途：

1. 禁止 attached 走 force reclone（§5.5）
2. 审计与 Console 标注「此目录不是平台创建的」，让点确认删除的人有判断依据

### 5.7 平台不碰 git 工作树

对**全部**来源生效：

- 不 fetch、不 pull、不 push、不定期同步
- 不 `apply_sparse_scope`（`project_workspace.rs:748-764`）——对 attached 直接关闭；`git` 来源的 scope 能力保留但仅在**首次 clone** 时生效，之后不再重放
- 不 `shield_repo_configs`（`project_workspace.rs:723-746`，现状会 `rm opencode.json` + `update-index --skip-worktree`）——对 attached 关闭
- 不自动 checkout 分支
- 要拉要推由数字员工在会话内自行执行（它本来就有 shell）
- attached 的 `repo_binding` 退化为**观测值**（探测读到的 origin / 当前分支），只用于展示，**不驱动任何动作**

**push / pull 的归属（人类拍板 2026-08-12）**：git 的推拉不是平台工作区层的职责，而是**场景模板层**的职责——由模板定义数字员工角色（测试、提交推送等），角色在会话内自行执行 `git push` / 拉取。因此本 spec 的「平台不碰 git 工作树」不是缺口，而是**分层**：工作区层只保证「目录在、内容是人/模板放的」，代码流转由模板角色承担。

**由此产生的边界**：模板若未定义承担推送的角色，该项目的工作就只存在于主节点磁盘上，主节点损毁即丢失。代码的持久化事实源是 git remote / 平台工件，不是节点磁盘。这条应在场景模板侧体现（模板设计时要回答「谁负责把成果推出去」），本 spec 不代劳。

### 5.8 能力投影对 attached 目录的污染

技能软链（`.claude/skills/*` 等）、MCP 投影、`.superteam/` 会话清单会出现在被认领的真实仓库工作树里。**处理**：会话开始时把投影路径写入 `.git/info/exclude`（**不动用户的 `.gitignore`**），会话结束按既有清单 unlink（`2026-07-23` §6.2 机制不变）。

---

## 6. 数据与契约影响

### 迁移

1. `projects` 增 `workspace_ownership`（默认 `platform_managed`）
2. `project_runtime_nodes` 增 `provision_status` / `provisioned_at` / `provision_source`；存量行回填 `provisioned`（现状确实都 mkdir 过）
3. 新表 `project_workspace_delete_requests`（自带快照字段，见 §5.5）
4. `inbox` 新 item type 常量（无表结构变更）

### 契约

- `contracts/control-plane/openapi.yaml`：创建项目请求增来源判别式（`source_kind: none|git|attach`）；新增探测端点、供给确认端点、删除确认队列端点。走 `verify:contracts` + `generate:control-plane`
- **Runtime 命令**：新增 `probe_project_directory`。注意——**认领探测发生在项目创建之前，此时还没有 `project_id`**，而 `dispatchProjectWorkspaceCommand` 要求 `ResourceType`/`ResourceID`（`workspace_provision.go:206-219`）。实现须用 `resource_type=project_workspace_probe` + 探测请求 id 作 `resource_id`，不得伪造 project_id
- 探测返回事实（放回执自由 map，不改回执结构）：`exists` / `is_dir` / `is_symlink` / `is_git_repo` / `origin_url` / `current_branch` / `detached` / `dirty` / `head_commit`
- `contracts/runtime/openapi.yaml` 是 CLAUDE.md 记的已知债（不在自动门禁内），改后须人工核对下游

### 安全

探测与校验都用 `symlink_metadata`（现状 `project_workspace.rs:266` 已如此，软链目录会被 `!meta.is_dir()` 挡掉）——认领路径必须用同一把尺子，防止工作区根下的软链指向 `~/.ssh` 之类。

---

## 7. Console 影响

- 创建向导：来源三选一（`create-project-draft.ts` 的 `sourceKind` 增 `attach`）；attach 分支先选 Runtime 节点、填目录名、点「探测」，把探测事实回显给人确认后才允许提交；提交时**再探一次**兜 TOCTOU
- 项目配置页：节点列表按 `provision_status` 分「已供给 / 已绑定未供给」；提供「供给到此节点」动作（走确认）
- 新增管理员「工作区删除确认」队列页（或并入既有待确认删除队列）
- 状态文案全部进 `apps/web/src/lib/status-labels.ts`（`provision_status`、新 item type、新原因码）
- 项目详情标注 `ownership`，attached 显式提示「目录由人工维护，平台不创建、不填充、不改 git 状态」

---

## 8. 实施分期

| 分期 | 内容 | 可独立验收 |
|---|---|---|
| **P0** | 删除确认队列（删项目 / 摘节点 / 队列 / 催办 / 拒绝放手 / 目录名占用并查） | 是 |
| **P1** | 绑定与供给分离（`provision_status`、加节点不再 mkdir、候选过滤、供给确认待办、懒触发） | 是 |
| **P2** | attach 来源（探测命令、创建向导、`ownership`、投影 exclude、禁 force reclone） | 是 |
| **P3** | 平台不碰 git 工作树（关 sparse/shield 重放、attached repo_binding 降为观测） | 是 |

顺序有依赖：**P0 必须先落**——P1/P2 都会更频繁地产生「目录该不该删」的判断，队列不在位时这些路径仍会无声删盘。

---

## 9. 验收门槛（真实端到端，按 CLAUDE.md 默认完成条件）

Web + Control Plane + DB + Runtime（+ Provider，涉执行的项）全链，不接受 mock/单测代替：

1. **删项目不删盘**：删一个 git 项目 → 节点上目录仍在 → 队列出现每节点一条待办 → 管理员确认 → 目录消失；另一条**拒绝** → 目录保留、队列关闭、审计有记录
2. **摘节点不删盘**：同上路径走 `RemoveProjectRuntimeNode`
3. **目录名占用并查**：删项目（未确认删目录）后，用同名目录名建新项目 → 被拒且文案指向待删队列
4. **加节点不碰磁盘**：绑 node B → B 上**无**该项目目录；就绪面显示「已绑定未供给」；派发不会落到 B
5. **主节点不可用 → 不可用而非静默漂移**：停掉 node A → 派发失败且原因为工作区不可用 → 收件箱出现供给确认待办（文案含「A 上未推送改动不在其中」）→ 管理员确认 → B 完成 clone → 派发落 B 成功
6. **attach 正路**：节点上手工放一个已有 git 仓库 → 创建向导探测返回真实 origin/分支/dirty → 创建成功且 `ready` → 派发任务 CWD 为该目录 → 会话结束后技能软链与 MCP 片段清理干净、**业务文件与 git 状态未被平台改动**（`git status` 与认领前一致，`.superteam/` 与投影路径不出现在 `git status`）
7. **attach 反路**：目录不存在 → 探测失败、创建被拒；目录是软链 → 被拒；目录名已被占用 → 被拒
8. **attached 禁 force reclone**：对 attached 项目调重新 clone → 被拒，目录完好
9. **漂移判据不被平台污染**：attached 项目绑 node B（B 上无目录）→ 停 A → 派发失败（而非在空目录里跑）→ 人工在 B 放好目录 → 确认供给 → 派发成功

真实 E2E 期间须记录并复核 `control-plane` / `web` pid，`owner=` 异源则结论作废（CLAUDE.md）。

---

## 10. 风险与接受项

1. **主节点挂到人确认之间项目停摆**——人类明确接受（拍板 3）。夜间无人值守会放大停摆时长
2. **供给到备节点拿到的是远端内容**，主节点上未推送的工作不会跟过去。平台不 push（§5.7 分层），故障转移能挽回多少取决于**场景模板是否定义了承担推送的角色**——模板没定义就会丢
3. **「有目录即相等」的残留误判**：运维在备节点手工建了空目录或放错内容，平台照认。已明确为人的责任，与平台自建项目同等待遇（拍板 2）
4. **删除延后带来的磁盘滞留**：拒绝确认后平台不再管，目录长期留在节点上，靠运维纪律清理
5. **存量项目回填**：现有多节点项目在磁盘上确实都已 mkdir/clone，回填为 `provisioned` 后，那些陈旧副本仍然存在——P1 落地只阻止**新增**陈旧副本，不追溯清理。是否对存量备节点做一次性提示，实施时定
6. **Runtime 契约债**：新增 `probe_project_directory` 不在自动门禁内，须人工核对下游

---

## 11. 施工交接（面向执行会话）

本节是给**冷启动执行会话**的交接面。每一期的交接语见 §11.8，可直接粘贴。

### 11.1 承重不变量（不得"顺手优化"掉）

以下每一条都是在设计讨论中被推翻过一次才定下来的，文档里看不出理由，**下一个人极易改回去**。改动前必须问人：

1. **不得为 attached 目录加内容校验**（非空、指纹、marker 文件都不行）。理由：平台自建目录的内容同样是人填的、平台从来不验，为 attached 单立更严标准是双标
2. **不得新增项目 `disable` 状态，也不得复用业务态 `paused`** 表达磁盘可用性。可用性轴是 `workspace_ready_status`
3. **不得让平台执行 git 写操作**（push/pull/fetch/sparse-checkout/checkout/删配置文件）。推拉是场景模板角色的职责（§5.7）
4. **不得引入绝对路径**。attach 只填目录名，路径永远是「节点生效 base ⊕ directory_name」
5. **不得在绑定节点时碰磁盘**。绑定 = 候选资格；供给 = 另一件需人工确认的事
6. **拒绝删除确认后不得留悬挂待办**（平台放手，写审计即止）。否则要额外写孤儿回收，团队那套已经踩过
7. **单节点不可用不得翻转项目级 `workspace_ready_status`**（保留 `2026-07-23` §0.8 松模型）
8. **创建失败回滚仍直接删目录**，不进确认队列（删的是平台刚建的空目录）

### 11.2 分期与并行约束

**严格串行 P0 → P1 → P2 → P3，不可并行。** 四期重叠在同一批文件上：

| 文件 | P0 | P1 | P2 | P3 |
|---|---|---|---|---|
| `internal/project/service.go` | ✓ | ✓ | ✓ | |
| `internal/project/node_resolver.go` | | ✓ | | |
| `internal/project/workspace_provision.go` | ✓ | ✓ | ✓ | |
| `internal/project/workspace_git_clone.go` | ✓ | ✓ | ✓ | ✓ |
| `runtime-agent/src/project_workspace.rs` | | ✓ | ✓ | ✓ |
| `runtime-agent/src/commands/executor.rs` | | | ✓ | |
| `apps/web` 创建向导 / 配置页 | ✓ | ✓ | ✓ | |

**一会话一 worktree**（CLAUDE.md「工程约定」+ `docs/PARALLEL_DEVELOPMENT.md`）。若确实要并行开发环境：未共享 `SUPERTEAM_DEV_PID_DIR` 时从 worktree 执行 `restart` 是退出码 0 的**静默空操作**，服务仍跑别人的代码 → 会产出假的验证结论。

### 11.3 改动点清单

#### P0 — 删除确认队列

| 位置 | 现状 | 改为 |
|---|---|---|
| `project/service.go:1557`（`DeleteProject`） | 调 `removeProjectDirectoriesOnNodes` | 改为按节点写 `project_workspace_delete_requests`（每节点一条，带快照），不下发删除命令 |
| `project/service.go:625`（`RemoveProjectRuntimeNode`） | 同上 | 同上，单节点一条 |
| `project/service.go:272-288`（`CreateProject` 目录名校验） | 只查 `projects` 唯一索引 | 并查未决删除队列的同名 `directory_name`，命中则拒绝并指向队列 |
| `project/workspace_provision.go`（新增） | — | `ConfirmWorkspaceDelete` / `RejectWorkspaceDelete`：确认逐节点下发既有 `remove_project_directory`；节点离线该条留队；拒绝写审计并关闭 |
| `inbox/types.go:67,81` 附近 | — | 加 `ItemTypeProjectWorkspacePendingDelete` / `SourceTypeProjectWorkspacePendingDelete`，并在 `inbox/service.go:548,704,713` 三处白名单同步登记 |
| `app/app.go:1064` 附近 | 只起团队催办 | 加同形的工作区删除滞留催办 goroutine（照抄 `app/team_pending_delete_reminder.go`） |
| `project/service.go:407`（`rollbackCreatedProject`） | 直接删 | **不动**（不变量 8） |

#### P1 — 绑定与供给分离

| 位置 | 现状 | 改为 |
|---|---|---|
| `project/service.go:337`（`CreateProject`） | 对**全部** `runtimeNodeIDs` mkdir | 只对主节点（`runtimeNodeIDs[0]`）供给；其余仅绑定，`provision_status=unprovisioned` |
| `project/service.go:577,584`（`AddProjectRuntimeNode`） | mkdir + clone | **删掉这两步**，只插绑定行 |
| `project/workspace_git_clone.go:36-38`（`enqueueProjectGitClone`） | fan-out 全节点 | 只对主节点 |
| `project/node_resolver.go:205-213`（`filterCandidatesByWorkspace`） | 按磁盘校验过滤 | 前置过滤 `provision_status=provisioned`，未供给节点不进候选 |
| `project/node_resolver.go:163` 返回 `WorkspaceUnavailable` 处 | 仅返回原因 | 顺带**幂等**开出「供给到节点 X？」待办（懒触发，同 (project,node) 不重复开卡） |
| `project/workspace_provision.go`（新增） | — | `ProvisionWorkspaceOnNode`（确认后执行：非 git mkdir / git clone / attached 仅探测），成功置 `provisioned` |
| `runtime-agent/src/project_workspace.rs:58` | 防御性 `create_dir_all` | attached 关闭（缺失即失败）；其余来源保留 |

#### P2 — attach 来源

| 位置 | 改为 |
|---|---|
| `runtime-agent/src/project_workspace.rs:156-170`（`ensure_stable_project_directory`） | 非空目录不再一律拒绝：新增 attach 分支（存在即可，**不查非空**） |
| `runtime-agent/src/project_workspace.rs`（新增 `probe_project_directory`） | 返回 §6 的事实集；用 `symlink_metadata`，软链目录判失败 |
| `runtime-agent/src/commands/executor.rs:264-273` + `controlplane/models.rs:420-423` | 注册新命令类型与 handler |
| `project/types.go:524` 附近 | `CreateProjectRequest` 加 `SourceKind`；`Project` 加 `WorkspaceOwnership` |
| `project/workspace_git_clone.go:315-330`（`RecloneProjectWorkspace`） | attached 直接拒绝（`ownership=attached` → 400） |
| `runtime-agent` 会话投影处 | 把投影路径写进 `.git/info/exclude`（不动用户 `.gitignore`） |
| `apps/web` `create-project-draft.ts:28,48` / `project-basics-step.tsx:99-100` | `sourceKind` 增 `attach`；探测→回显→确认→提交时再探一次 |

#### P3 — 平台不碰 git 工作树

| 位置 | 改为 |
|---|---|
| `runtime-agent/src/project_workspace.rs:477-478`（`ensure_git_in_stable_project_dir` 尾部） | `apply_sparse_scope` / `shield_repo_configs` 不再每次派发重放；scope 仅首次 clone 生效，attached 一律不调 |
| `project/types.go:516-529` 消费处 | attached 的 `repo_binding` 降为观测值（展示用），不驱动 clone/reclone/scope |

### 11.4 迁移

**命名规范**：时间戳式 `YYYYMMDDHHMMSS_slug.sql`（最新参照 `20260811180000_project_task_portfolio_bucket.sql`）。仓库里的 `076_` / `078_` 是**历史编号式，不要照抄**。生产迁移只放 `apps/control-plane/internal/storage/migrations/`，改完校验 `atlas.sum` 与 `migrate-validate`（CLAUDE.md）。

三个迁移（可合并为一，按分期拆更稳）：

```sql
-- P1
ALTER TABLE project_runtime_nodes
    ADD COLUMN IF NOT EXISTS provision_status  VARCHAR(24)  NOT NULL DEFAULT 'provisioned',
    ADD COLUMN IF NOT EXISTS provisioned_at    TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS provision_source  VARCHAR(24);
-- 存量回填：现状确实都已 mkdir/clone，默认值即回填（风险见 §10-5）

-- P2
ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS workspace_ownership VARCHAR(24) NOT NULL DEFAULT 'platform_managed';

-- P0
CREATE TABLE IF NOT EXISTS project_workspace_delete_requests (
    id               UUID PRIMARY KEY,
    tenant_id        UUID        NOT NULL,
    project_id       UUID        NOT NULL,
    runtime_node_id  UUID        NOT NULL,
    -- 快照：项目已软删，不能靠 join 活行取值
    directory_name   VARCHAR(64) NOT NULL,
    node_id_snapshot TEXT        NOT NULL,
    ownership        VARCHAR(24) NOT NULL,
    repo_summary     JSONB,
    status           VARCHAR(24) NOT NULL,  -- pending | confirmed | rejected
    requested_by     UUID        NOT NULL,
    requested_at     TIMESTAMPTZ NOT NULL,
    resolved_by      UUID,
    resolved_at      TIMESTAMPTZ,
    reason           TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_pwdr_pending
    ON project_workspace_delete_requests (project_id, runtime_node_id)
    WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS ix_pwdr_dirname_pending
    ON project_workspace_delete_requests (directory_name)
    WHERE status = 'pending';  -- 供 §5.5 目录名占用并查
```

### 11.5 契约与端点

路由风格对齐 `internal/api/server.go:413-458`（项目族）与 `:534`（团队待确认删除）：

```
POST   /projects/workspace/probe                          # 探测（项目尚不存在，不能挂 {projectId} 下）
GET    /projects/workspace-delete-requests                # 管理员队列，参照 /teams/pending-deletes
POST   /projects/workspace-delete-requests/{requestId}/confirm
POST   /projects/workspace-delete-requests/{requestId}/reject
POST   /projects/{projectId}/runtime-nodes/{runtimeNodeId}/provision   # 供给确认
```

- **探测的 receipt 坑**：`dispatchProjectWorkspaceCommand` 要求 `ResourceType`/`ResourceID`（`workspace_provision.go:206-219`），而探测时还没有 project_id → 用 `resource_type=project_workspace_probe` + 探测请求 id 作 `resource_id`，**不得伪造 project_id**
- 生成与门禁：改 `contracts/control-plane/openapi.yaml` 后跑 `corepack pnpm generate:control-plane` + `corepack pnpm verify:contracts`；Go 侧 `corepack pnpm verify:control-plane`
- `contracts/runtime/openapi.yaml` 不在自动门禁内（CLAUDE.md 已知债），新增 `probe_project_directory` 后**人工核对下游**

### 11.6 中文词表新增键

用户可见枚举必须经 `apps/web/src/lib/status-labels.ts`（CLAUDE.md 硬约束），新增：

| 枚举 | 键 → 中文 | 期 |
|---|---|---|
| `source_kind` | `none` 非 Git（空目录） / `git` Git 仓库 / `attach` 认领已有目录 | P2 |
| `provision_status` | `provisioned` 已供给 / `unprovisioned` 已绑定未供给 | P1 |
| `workspace_ownership` | `platform_managed` 平台创建 / `attached` 认领已有目录 | P2 |
| 删除请求 `status` | `pending` 待确认 / `confirmed` 已确认删除 / `rejected` 已拒绝（平台放手） | P0 |
| 收件箱条目类型 | `project_workspace_pending_delete` 工作区目录待删除确认 | P0 |

另复核 `NodeResolutionReasonWorkspaceUnavailable` 的中文是否已在词表内，缺则一并补（P1）。对象指称用「名称 (id)」，不得暴露裸 UUID。

### 11.7 E2E 前提：第二个 Runtime 节点

验收第 4 / 5 / 9 条都要两个节点。第二个 runtime-agent 用 env 覆盖即可，**四个键必须都改**否则会和第一个撞：

```
RUNTIME_AGENT_NODE_ID=<另一个 id>
RUNTIME_AGENT_HTTP_ADDR=<另一个端口>
RUNTIME_AGENT_WORKSPACE_DIR=<另一个工作区根>
RUNTIME_AGENT_RUN_LOG_DIR=<另一个日志目录>
```

（env 键见 `runtime-agent/src/config.rs:322-348`；工作区根覆盖优先级见 `2026-07-23` spec §4。）

**若第二节点起不来**：P1 的核心验收（漂移 → 不可用 → 确认供给 → 派发落 B）**标阻塞**，不得用单节点造 fixture 冒充通过——绿单测 ≠ loop 真跑。其余各期验收不受影响。

### 11.8 每期交接语（可直接粘给执行会话）

> 读 `docs/superpowers/specs/2026-08-12-project-workspace-provisioning-model.md`：先读 §11.1 承重不变量与 §0 人类拍板记录（这两节的每一条都不得自行推翻，有异议先问人），再读 §11.3 的第 N 期改动点清单。实施完按 §9 里属于该期的验收条目做**真实端到端**验证（Web + Control Plane + DB + Runtime，按 §11.7 起第二节点），验证期记下 `control-plane`/`web` 的 pid 并在收尾复核 `owner=`。收尾走 `superteam-completion-check`。上一期未合入前不要开工。

---

## 12. 修订记录

- 2026-08-12：初版。收录来源三态、绑定/供给分离、供给与撤销双向人工确认、平台不碰 git 工作树，以及对 `2026-07-23` spec 五条已拍板决策的推翻。
- 2026-08-12（补）：§5.7 push/pull 归属改为「场景模板角色职责」（人类拍板），不再表述为平台缺口；新增 §11 施工交接。
- 2026-08-12（实施）：串行落地 P0→P1→P2→P3。
  - P0：`project_workspace_delete_requests` 队列、删项目/摘节点改入队、目录名占用并查、确认/拒绝端点、滞留催办、Console 队列区。
  - P1：`provision_status` 绑定/供给分离、创建仅主节点供给、加节点不碰盘、候选过滤、懒触发供给待办、`POST …/provision`。
  - P2：`workspace_ownership`、`source_kind=attach`、`probe_project_directory`、attached 禁 force reclone、创建向导三选一、投影 exclude 已有。
  - P3：首次 clone 后不再重放 sparse/shield；attached 不碰 git 工作树；`repo_binding` 对 attached 仅观测（派发 metadata 带 ownership）。
