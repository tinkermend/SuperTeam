# 能力供给三层模型：团队 / 员工 / 项目

- 日期：2026-08-06
- 状态：**已实施并验收**（P0–P2 入库 `4f4a3033`；S10 承重判据 2026-08-07 真实链路通过，见 §11 验收记录）
- 交付性质：能力（技能 / MCP）的**供给轴**从两层扩为三层，并给出合并、过滤、冲突、物化与投影的完整语义
- 目标读者：实施会话（本文自包含；实施前必读 `2026-07-17-directory-capability-projection-revision-design.md` §1–§3）
- **本文推翻 2026-07-17 的「无项目过滤轴」与 2026-07-22 的「项目级 MCP 绑定退役」**，理由与替代方案见 §9

---

## 0.0 开工须知（实施会话先读这一节）

### 环境

服务用 `./scripts/dev-services.sh start|status|restart <service>` 管理（OpenFGA 需单独管）。

| 项 | 值 |
|---|---|
| Web | `http://127.0.0.1:3100`（**不是 3000**） |
| Control Plane | `http://127.0.0.1:8080` |
| 登录 | `POST /api/auth/login`，`admin` / `admin` |
| 数据库 | `apps/control-plane/config/config.yaml` 的 `postgres.url`（远程 dev 库，**无备份**；schema 是 `superteam`，不是 `public`） |
| 节点工作区 | `/var/superteam/workspaces/`（`apps/runtime-agent/config.yaml` 的 `base_dir`） |
| 门禁 | `verify:contracts` / `verify:control-plane` / `verify:web` / `verify:runtime-agent` |

### 踩过的坑，别重复

- **表名不是望文生义的**：是 `team_skill_bindings`（不是 `skill_team_bindings`）、`digital_employee_mcp_bindings_v2`（v1 已于迁移 080 删除）、`skill_agent_bindings`、`skill_mcp_dependencies`
- `project_mcp_bindings` 在迁移 072 建过、在 `20260722003600` 被 DROP。**重建时不要新造形状**，直接照 072 的定义复原，省得契约和 sqlc 又漂一次
- 改迁移后必须 `atlas migrate hash`；`make -C apps/control-plane migrate-validate` 需自建干净 PG16 并传 `DEV_URL`（本机无 docker）
- sqlc 不解析 `DROP COLUMN IF EXISTS`（须 `DROP COLUMN`）；`go generate .` 不含 sqlc，需显式 `sqlc generate`

### 当前真实数据（2026-08-06 实测）

```
team_mcp_bindings                 10      ← 能力供给全部发生在团队层
team_skill_bindings                1
digital_employee_mcp_bindings_v2   0      ← 员工层一条都没有
skill_agent_bindings               0
project_*_bindings                 —      ← 表不存在
```

**实施影响**：本批的真实链路验证需要先造员工级与项目级绑定数据，不能指望现网存量。

---

## 0. 已拍板（2026-08-06 与人类逐轮确认，不重议）

| # | 决定 |
|---|---|
| 1 | 能力供给分**三层载体**：团队（按人群批发）/ 员工（个体特化）/ 项目（按场地供给） |
| 2 | 三层是**互补**关系，合并用**并集**。「重叠取交集」已明确否决——三者承载不同的东西，取交必为空集 |
| 3 | 用**绑定**（多对多），不用**归属**（一对一）。归属会让通用技能被单个项目占有，跨项目复用即断 |
| 4 | 项目层**只做供给**（提供能力给进来的人）；**门禁留在剧本层** `requires`，两者不得混入同一集合 |
| 5 | 物化走**懒加载**（首次派发按 checksum 拉），绑定时只做**逻辑校验**（验包存在/可取，不推字节） |
| 6 | 分层是**控制平面的概念**；runtime 只认合并后的 `payload.skills[]`。团队层今天已经如此，项目层照做 |
| 7 | 第一版**不引入** `{base}/projects/{proj}/` 独立缓存，物化仍进 `employees/{emp}/`。磁盘去重是后续的纯存储优化，不影响语义 |
| 8 | **准入仍由人定**（成员池 + 编制）。能力绑定只影响投影与候选推荐，**不做"携带即可进项目"的推导** |
| 9 | 仓库原生 `.mcp.json` / `opencode.json` **继续屏蔽**；项目 MCP 走注册表绑定正门 |
| 10 | 第一期技能均为公共，**项目负责人可绑**（`project.config.edit`），须留审计与项目事件 |

---

## 1. 背景

### 1.1 今天项目级能力两条路都不通

| 日期 | 事 |
|---|---|
| 07-17 | 建 `project_mcp_bindings`（迁移 072）；投影 = 员工侧 ∪ 项目侧，同 key 项目优先。**真实 E2E 验过**：员工零 MCP 绑定时，工作区 `.superteam/mcp/claude.mcp.json` 里是纯项目投影 |
| 07-18 | 项目配置页「MCP 绑定」tab 落地，浏览器 E2E 过 |
| 07-22 | 退役（`2026-07-22-project-mcp-bindings-retirement.md`）。理由：「平台侧项目 MCP 配置统一走 `.mcp.json` 文件管理」+「该表从未有运行时消费者」 |

退役 spec 的两条理由都有问题：

1. 「从未有运行时消费者」与 07-17 的 GATE 证据直接矛盾——它被真实验证过是工作的。
2. **替代路径至今没兑现**：`providers/claude.rs:36` 仍带 `--strict-mcp-config`（仓库 `.mcp.json` 硬屏蔽），`project_workspace.rs` 的 `shield_repo_configs` 仍对 opencode 做 skip-worktree + 删除。

净结果：平台侧的路拆了，仓库侧的路还锁着，**项目级 MCP 今天无路可走**。屏蔽本身有三条仍然成立的理由（凭据外流、stdio 型 MCP 在只读任务里也会拉起任意进程、三 provider 发现规则碎片化且 codex 无项目级发现），所以正解是回到平台侧绑定，而不是解除屏蔽。

### 1.2 缺的那个轴

能力随两种东西变化，而今天只有一种能表达：

| 变化维度 | 谁承载 | 今天有吗 |
|---|---|---|
| 随**人**变（我会 MySQL，他会 Oracle） | 员工 / 团队 | ✅ |
| 随**场地**变（A 项目的数据源、内部系统、构建工具） | 项目 | ❌ |

只有人轴时，"这个数据源只在 A 项目能碰"无法表达；只有场地轴时，同一项目内所有员工能力完全相同，个体特化消失。**两个轴不能互相推导。**

### 1.3 一句话

> **能力按三层供给、按并集合并、按场地过滤、同名项目优先；分层只存在于控制平面，执行链路仍只有一层。**

---

## 2. 非目标

| 不做 | 理由 |
|---|---|
| 解除仓库原生 MCP/技能配置的屏蔽 | §1.1 三条理由仍成立；正门是注册表绑定 |
| `{base}/projects/{proj}/` 独立缓存 | 决策 7：只换磁盘去重、不换语义，等磁盘真成问题再做 |
| 项目级门禁（要求进来的人必须具备某能力） | 决策 4：门禁留剧本层 `requires`；真出现项目级合规要求再立项 |
| 「携带某项目能力 ⇒ 自动可进该项目」的准入推导 | 决策 8：准入是治理动作要人签字，改配置不该等于改准入 |
| 场景模板 `requires` 的语义变更 | 07-17 定的「充分性门禁，从不裁剪」保持不变 |
| 团队层退役 | 团队仍是唯一「按人群批发」的轴，且独立承载宪法与权限分组（§9.3） |

---

## 3. 地基核对（2026-08-06 核实，勿重复勘察）

**目录模型**（`apps/runtime-agent/src/`）

```
{base}/employees/{emp}/                      员工能力缓存：稳定、跨任务复用、checksum 跳过
{base}/repos/{project_id}/                   项目仓库缓存
{base}/workspaces/{proj}/{task}/{attempt}/   任务工作区 worktree ← 投影落点，随任务生灭
{base}/chat/{proj}/{thread_id}/              chat 工作目录，TTL 清理
```

**技能投影**：`link_provider_skills`（`project_workspace.rs:335`）逐 skill key 软链；provider 落点不同——claude `.claude/skills`、codex `.agents/skills`、opencode `.opencode/skills`（`provider_skills_rel:326`）。**这是项目层不能预先投影的根因：投影落点依赖执行者的 provider，而绑定时不知道谁来执行。**

**技能物化**：会话开始按 `archive_checksum_sha256` 增量物化进员工缓存，命中即跳过。07-17 GATE 实测：同线程第二轮 presign 仍为 1。

**MCP 投影**：每 run 一次性投影到 `<workspace>/.superteam/mcp/`，随 run 消亡；payload 只带 env 变量名不带值。

**冲突策略（已实现，本文复用）**：工作区已存在同 skill key（仓库原生技能）→ 跳过员工侧该 key、项目侧生效，冲突清单经 CommandWorkspace→RunSpec 进 attestation `skill_conflicts`，**不得静默**。

**git 可见性（本文要补的洞）**：`shield_repo_configs`（`project_workspace.rs:613`）只屏蔽**仓库原生**配置，对我们自己软链进去的东西没有任何处理。实测 `/var/superteam/workspaces/batch3-h1h4-1785924961`：`.claude/skills` 与 `.superteam/sessions` 都在，`git status` 干净——**但那是因为它们是空目录**（git 不跟踪空目录），空是因为该员工零技能绑定。一旦真投，`.claude/skills/{key}` 就是 untracked 软链，`git add -A` 会带进仓库。

**依赖门禁**：`skill_mcp_dependencies` 声明技能依赖的 MCP；07-17 对员工携带集合验证依赖闭包、缺失阻断派发。**这是当初否掉「项目过滤轴」的唯一技术理由**（怕裁出"技能在、依赖 MCP 被裁"的半残态）。本文的解法见 §4.3。

---

## 4. 数据模型

### 4.1 两张绑定表

```sql
project_skill_bindings(id, tenant_id, project_id, skill_id, created_by_user_id, created_at)
  UNIQUE (tenant_id, project_id, skill_id)

project_mcp_bindings(...)   -- 照迁移 072 原样复原，不新造形状
  UNIQUE (tenant_id, project_id, mcp_server_id) WHERE 未删除
```

**不在 `skills` 表上加 `project_id`**（决策 3）：归属是一对一，会让"数据库通用巡检"被某个项目占有。

### 4.2 一张表，两种作用

`project_skill_bindings` 同时表达「场地限定」与「场地供给」，靠投影规则区分：

```
技能 s 在项目 P 的任务中被投影  ⟺
    ① s 没有任何项目绑定  或  P ∈ s 的项目绑定集合          ← 场地限定
  且 ② s 被该员工携带（团队继承 ∪ 个人）  或  s 绑定了 P    ← 供给来源
```

三种典型情况的真值表：

| 技能 | 项目绑定 | 员工携带 | 在 A 项目 | 在 B 项目 |
|---|---|---|---|---|
| 数据库通用巡检 | 无 | 是 | ①真空满足 ②员工带 → **投** | **投** |
| A 项目数据源 MCP | {A} | 否 | ①满足 ②绑了 A → **投** | ①不满足 → **不投** |
| B 项目旧系统迁移 | {B} | 是 | ①不满足 → **不投** | **投** |

第三行正是人类要的隔离：「非本项目的技能不会在项目任务里加载，也不会落到 Runtime Agent 上」。

### 4.3 依赖闭包必须跟着走（承重）

过滤的单位是**依赖闭包，不是单个条目**：

> 一个技能通过 §4.2 进入投影集合时，它在 `skill_mcp_dependencies` 里声明的 MCP **必须一并进入**，即使那些 MCP 没有绑定当前项目。

不这样做就会造出「技能在、依赖 MCP 被裁」的半残态——这正是 07-17 否掉项目过滤轴的理由。

**但闭包只补「env 已满足」的依赖**（2026-08-06 复检更正）。原文只写了「技能进则依赖 MCP 必须进」，没跟既有门禁对账，导致首版实现无条件补全，后果是：

`validateSkillMCPDependencies` 的契约（写在它自己的注释里）是 "**dependency validates, never grants**"——技能的 MCP 依赖必须**已经**在 env 满足的有效集合里，否则阻断派发。而闭包在它之前运行、恰好把它要找的东西塞进 `deps.runtimeMCP`，于是这道闸**永真**，「缺失阻断派发」再也不会触发；且 `ResolveRuntimeMCPServer` 只按 serverID 解析注册表定义，凭据 env 没配的 MCP 也会被投影，故障从派发期清晰拦截退化成运行期莫名失败。

**正确分工**：

| 情形 | 处置 |
|---|---|
| 注册表里有、没绑给员工/项目、**env 已配齐** | 闭包补（这才是「技能不能半残」的本意） |
| **env 没配齐** | **不补**，交回 `validateSkillMCPDependencies` 拦截并报可读原因 |
| 注册表里查不到定义 | 不补，落日志（不得静默） |

`deps.runtimeEnv` 在闭包之前已装载，env 满足性可就地判定，不需要额外查询。回归用例见 `employee/dependency_closure_test.go`（正反两向，且经反向验证非真空）。

---

## 5. 投影规则（控制平面）

```
候选 = 团队公共 ∪ 员工特定 ∪ 项目绑定          ← 并集，不是交集
过滤 = 候选中满足 §4.2 两条的项
闭包 = 过滤结果 ∪ 其依赖的 MCP（§4.3）
去重 = 同一能力多来源 → 投一次
冲突 = 同 skill key / 同 server_key 不同来源 → 项目侧优先，且必须留痕
payload.skills[] / payload.mcp_servers[] = 上述结果
```

三点强调：

1. **冲突决策在控制平面解，不下放 runtime**。那是治理动作，要进派发记录与 attestation；runtime 只执行已解好的结果。
2. **runtime 一行不用改**（除了 §7 的 git 屏蔽）。它照旧对 `payload.skills[]` 按 checksum 物化进员工缓存、照旧逐 key 软链。项目层对它完全透明——**与团队层今天的处理方式完全一致**。
3. 冲突留痕沿用既有 `skill_conflicts` 通道，新增来源标记 `project_binding`。

---

## 6. 生命周期

必须分清两件事，它们的时机完全不同：

| | 物化 materialize | 投影 project |
|---|---|---|
| 做什么 | 下载技能包、校验 checksum、解压落盘 | 建软链 + 生成 provider 配置文件 |
| 成本 | 贵（网络 + 磁盘） | ≈0（软链是指针） |
| 落点 | `employees/{emp}/`（决策 7：不建项目缓存） | 任务工作区，随工作区消亡 |
| 时机 | **懒加载**：首次派发按 checksum 拉，之后命中跳过 | **每 run** |
| 撤销 | 不需要（见下） | 工作区消亡即撤销 |

**为什么投影必须 per-run**（不能像人类最初设想的"项目建立时固定投影"）：

1. 投影落点依赖 provider（`.claude/skills` vs `.agents/skills` vs `.opencode/skills`），绑定时不知道谁来执行
2. 任务工作区是 per-attempt 的 worktree，项目创建时根本没有可投的地方
3. MCP 配置带凭据 env 变量名，07-17 的安全模型是"每 run 一次性投影、随 run 消亡，不存在被反复改写的实例级常驻配置"

**解绑后旧包怎么办**：不用管。**投影集合是权威**——解绑后 payload 里就没有它，下次 run 即失效。缓存残留只占磁盘，交给既有 LRU/TTL。这与 MCP 的"删注册表项，挂靠员工下一次运行即不再有该配置"是同一个结构性保证。

**绑定时的逻辑校验**（决策 5）：绑定 API 同步校验——技能/MCP 存在且未删除、当前租户可见、技能包 artifact 可取（presign 可签发）、依赖闭包内的 MCP 均存在。校验失败即 400，**不推字节**。这样"绑完就知道对不对"，又不引入预热通道。

---

## 7. git 屏蔽（runtime 唯一新增工作）

投影后对写入工作区的路径做本地屏蔽，使 `git status` 沉默、`git add -A` 带不走：

- 落点：`<provider_root>/skills/{key}`（软链）与 `.superteam/mcp/`（配置文件）
- 手段：优先 `.git/info/exclude` 追加（本地生效、不入库、不改用户的 `.gitignore`）；已被 git 跟踪的路径退化用 `git update-index --skip-worktree`
- 幂等：重放/续聊时已存在则跳过
- 仅对 repo 型工作区执行；非 repo 的项目目录无此问题

变更采集走 `git diff HEAD`（只采 tracked），本来就不受污染——本条防的是**员工自己手滑提交**。

---

## 8. API 与权限

| 端点 | 权限 | 说明 |
|---|---|---|
| `GET /api/v1/projects/{projectId}/skill-bindings` | `project.config.read` | 列出项目绑定的技能 |
| `PUT /api/v1/projects/{projectId}/skill-bindings` | `project.config.edit` | 声明式全量替换，body `{items:[{skill_id}]}` |
| `GET /api/v1/projects/{projectId}/mcp-bindings` | `project.config.read` | 复原 072 时代的端点与契约 |
| `PUT /api/v1/projects/{projectId}/mcp-bindings` | `project.config.edit` | 同上；**补 072 遗留的小债**：缺 `items` 键须 400，不得宽容为清空 |

- 项目负责人即可操作（决策 10，第一期技能均为公共）；每次写入落 `project_events`（`project.capability_binding.changed`）与审计
- 归档项目 disabled

### 8.1 读路径必须同批带上（否则 UI 期会被迫二次改契约）

UI 设计另立一份 spec（见 §10 说明），但**它依赖的读字段必须在 P0 就定好**，否则契约要改两遍：

1. **`Skill` schema 增 `project_bindings: SkillProjectBinding[]`**，与既有 `team_bindings` / `agent_bindings` **对称内联**。
   - 用途一：员工配置页显示"这个技能限定了哪些项目"——没有它，场地过滤对用户就是黑盒，会复现 `project_placements` 时代"绑了没效果"那类误导交互。
   - 用途二：技能停用/删除前的影响面（形态可抄角色词表的 `references`）。
2. **MCP server 读模型同样补 `project_bindings`**，理由同上。
3. **依赖闭包对前端可算**：确认 `SkillRuntimeDependencies` 是否已含 MCP 依赖项；含则前端可直接据它做"绑定该技能将一并投影 X、Y"的预览，**不需要新端点**；不含则在 P0 补上该字段（仍不新增端点）。

这三条都是**字段级增量**，不改端点形状，放进 P0 的 codegen 一并出。

### 8.2 项目配置页 tab 现状（实施前确认）

当前 tab 只有 **概览 / 成员 / 剧本编制 / 协调策略**——07-18 落地的「MCP 绑定」tab 已随 07-22 退役一并删除。UI spec 落地时是**复原 + 扩为「能力绑定」**（技能与 MCP 两个区同 tab），不是"在既有 tab 旁新增"。相关前端代码可从 git 历史 `05567dec` 附近检出参考。

---

## 9. 与既有决策的关系

### 9.1 被本文推翻的

| 出处 | 原决策 | 推翻理由 | 替代 |
|---|---|---|---|
| 2026-07-17 §决策表 | 能力只属员工，项目不绑定，**无项目过滤轴** | 唯一技术理由是依赖闭包被裁成半残态 | §4.3 按闭包过滤，理由消解 |
| 2026-07-22 退役 spec | 项目级 MCP 绑定退役，「统一走 `.mcp.json`」 | 该替代路径至今未兑现（`--strict-mcp-config` 仍在），且「从未有运行时消费者」与 07-17 GATE 证据矛盾 | §4.1 复原绑定表与端点 |

**落地时必须**在 `2026-07-22-project-mcp-bindings-retirement.md` 顶部加一行状态标注：「**已被 2026-08-06-capability-supply-three-layer-design.md 取代；本文两条理由均不成立，见其 §1.1**」——否则下一个会话读到它会再拆一遍。

### 9.2 明确保留不动的

- 场景模板 `requires` = **充分性门禁**，验带没带够、不够阻断，从不裁剪（07-17）
- 仓库原生 MCP/技能配置**维持屏蔽**（07-17 §3.2 三条理由）
- 员工能力缓存为**员工级、跨项目去重**，不做项目级复制（07-17 §1）
- 同 key 冲突**项目侧优先 + 必须留痕**（07-17 §3.1）

### 9.3 团队层的定位（澄清，不改代码）

团队在能力上**不是必需的**，它是「按人群批发」的快捷方式——正如项目层是「按场地批发」的快捷方式。两者都能被"逐个员工绑"替代，但都会丢掉同一个东西：**后加入的人自动获得**。

团队另外独立承载两样与能力无关的东西：**宪法**（职业规范跟人走，换项目照样成立）与**权限分组**（`user_project_team_scopes`）。因此三层保留。

---

## 10. 分期

| 切片 | 内容 | 可独立验收 |
|---|---|---|
| **P0** | 迁移（`project_skill_bindings` 新建 + `project_mcp_bindings` 复原）+ 两组 CRUD API + 绑定时逻辑校验 + **§8.1 三条读字段** + 契约与 codegen | API + `verify:contracts` |
| **P1** | 控制平面投影合并：§5 五步（并集 → 过滤 → 闭包 → 去重 → 冲突留痕），落进 `payload.skills[]` / `payload.mcp_servers[]` | 单测覆盖 §4.2 真值表三行 + 闭包补全 |
| **P2** | runtime git 屏蔽（§7） | runtime 单测 + 真实工作区 `git status` |

P0 必须先于 P1（没有绑定数据就没法验合并）。P2 可与 P1 并行。

**UI 不在本文范围**（人类拍板 2026-08-06）：P0–P2 落地并经复检之后，另立 UI spec。**配置面**已落地：`2026-08-06-capability-binding-console-design.md`（P0–P2）。**派发投影可见性**另立：`2026-08-07-capability-projection-visibility-design.md`（原 UI §7 P3；需小后端读路径，非纯展示）。本文 §8.1 的读字段是配置面 UI 的前置，**已在 P0 就位**。

---

## 11. 验收 GATE（真实 E2E）

| ID | 步骤 | 期望 |
|---|---|---|
| S1 | 绑一个不存在的 skill_id | 400，绑定时逻辑校验生效 |
| S2 | 绑一个依赖了未注册 MCP 的技能 | 400 并点名缺失的依赖 |
| S3 | 员工带通用技能（无项目绑定），在 A、B 两项目各跑一次任务 | 两边工作区**都**出现该技能软链 |
| S4 | 技能绑定 A 项目，员工**不**携带，派 A 项目任务 | 工作区出现该技能（项目供给生效） |
| S5 | 技能绑定 B 项目，员工**携带**它，派 **A** 项目任务 | 工作区**没有**该技能（场地过滤生效）——本批的承重判据 |
| S6 | S5 的同一员工改派 B 项目任务 | 工作区出现该技能 |
| S7 | 项目与员工绑了同 skill key 的不同版本 | 项目侧生效；`skill_conflicts` 在 attestation 里可查，来源标记 `project_binding` |
| S8 | 技能进投影、依赖 MCP 未绑该项目但**员工 env 已配齐** | 闭包补全一并投影（`source=dependency_closure`），派发放行 |
| S8b | 同上但**员工 env 未配齐** | **闭包不补**，`validateSkillMCPDependencies` 拦住派发并点名 server_key——闸不得被闭包架空 |
| S9 | 解绑该项目技能后再派一次任务 | 工作区不再出现；缓存残留不影响 |
| S10 | repo 型项目**任务运行中**在工作区执行 `git status --porcelain` | 干净；`.claude/skills/{key}` 与 `.superteam/` 不出现在 untracked 列表。**必须在运行中采样**——会话结束时 unload 会撤掉技能软链，事后采样只能验到 `.superteam/` 一半，会得出偏弱的结论 |
| S11 | 项目 MCP 绑定 PUT 缺 `items` 键 | 400（补 072 遗留小债），不得宽容为清空 |
| S12 | `verify:contracts` / `verify:control-plane` / `verify:web` / `verify:runtime-agent` / `migrate-validate` | 全过 |

> **S10 验收记录（2026-08-07，已通过）**：真实链路——提需求 → 规划 → 人类批准计划 → 派发 →
> runtime 建稳定项目目录并检出仓库 → 投影技能。运行中采样：工作区内
> `diagnose-smoke`、`linux` 两条软链在场，`git status --porcelain` 为空。
> 反向验证（摘掉 `.git/info/exclude` 的屏蔽段 → `?? .claude/` + `?? .superteam/`；还原后恢复干净）
> 在软链仍在的产物上完成。
>
> **排查中澄清的一处布局误读（勿再重踩）**：工作区是**稳定项目目录** `{base}/{project_name}`，
> 不是 `{base}/workspaces/{proj}/{task}/{attempt}`——后者是 `resolve_project_workspace`
> 里显式标注的 legacy 分支（spec 2026-07-23 §8 / P2），且稳定目录项目**不会**创建
> `repos/` 缓存。按旧布局去找会扑空，并误判成"runtime 没建工作区"。

**完成定义**：S1–S12 全过。**S5 与 S10 是承重判据**——前者证明场地隔离真的生效（这是人类提出本设计的核心诉求），后者证明投影不污染用户仓库。

**造数提示**：现网 `skill_agent_bindings = 0`、`team_skill_bindings = 1`，必须先自建员工级与项目级绑定，不能指望存量。

---

## 12. 风险

| 风险 | 缓解 |
|---|---|
| 项目层能绑一切 → 人图省事全绑项目，团队层退化成只剩宪法 | 治理而非模型问题：现阶段技能均公共故放开；出现高风险技能后按 `risk_level` 分流到团队/租户管理员 |
| 闭包补全把未绑本项目的 MCP 也投进来，等于绕过场地隔离 | 这是**有意**的：技能不能半残。但闭包补全的 MCP 必须在派发记录里单独标注来源为 `dependency_closure`，可审计 |
| 复原 072 时形状漂移，契约与 sqlc 再对不上 | 照原迁移复原，不新造；`verify:contracts` 把关 |
| git 屏蔽对已跟踪路径失效 | `.git/info/exclude` 只对 untracked 生效；已跟踪路径退化用 `skip-worktree`，S10 覆盖两种情况 |
| 同一项目技能被 N 个员工各存一份 | 决策 7 已知并接受；磁盘成问题时再引入项目缓存，属纯存储优化不动语义 |

---

## 13. 一句话方案

> **能力按三层供给（团队按人群、员工按个体、项目按场地），合并取并集、按场地过滤、依赖闭包必须跟着走、同名项目优先并留痕；分层只活在控制平面，runtime 仍然只认一份合并好的清单；物化懒加载并长驻，投影每 run 重做并随工作区消亡。**
