# 项目代码工作区与 Runtime 项目亲和 Spec

> 日期：2026-06-29（落地分期 §2.5、认证/缓存边界补充于 2026-06-30）
> 状态：待评审
> 决策：项目锚定节点、员工漂浮；员工能力缓存（skills/MCP/context）与任务工作区（代码 worktree）分离；Provider 认证默认复用宿主统一配置；项目身份存 DB、绝对路径运行时派生；中央 git remote 作持久化兜底
> 落地顺序：先做「项目级显式 repo 绑定（可空）+ mode 按角色自动推」，资源池/场景模板 requires 列为后续扩展（见 §2.5）
> 配套：员工能力缓存与 Provider auth 边界见 `2026-06-30-runtime-digital-employee-capability-cache-auth-design.md`

## 1. 背景与问题

开发类项目涉及源代码仓库。数字员工是 agent 型业务身份（角色 + 权限 + 上下文策略 + 输出契约），每个员工有自己专属的技能和 MCP 能力。由此产生一个长期纠结的问题：

- 源代码仓库该挂到每个数字员工下面，还是让员工通过绝对路径访问项目？
- 挂到员工下面 → 仓库副本份数膨胀、依赖/编译重复。
- 绝对路径访问 → 容易漏路径、一致性差。

进一步排查发现，问题根因是**分层错位**：把"业务引用""物理工作副本""员工能力"三件事压到了一层来想。本 spec 给出最终分层与决策。

### 当前实现的事实（排查结论）

- 运行工作区 `executor/workspace.rs::create_run_workspace` 生成 `base_dir/instances/{instance}/runs/{run}/workspace`，是 per-run 的、隔离的。但只被简单的 `execute_task` 路径使用。
- 当前代码中 `agent_home_dir` 实际值为 `workspaces/employees/{digital_employee_id}`：技能装进 `{agent_home}/{.claude|.agents|.opencode}/skills/{key}`，MCP config 写进 `{agent_home}/{.mcp.json|.codex/config.toml|opencode.json}`。这是历史实现事实，不是目标态的 Provider 认证模型。
- **关键问题**：command 执行路径的 `ensure_command_instance`（`commands/executor.rs`）直接把 `agent_home_dir` 当作 `workspace_path` 返回，provider 再 `current_dir(workspace_path)`。结果 **Provider 的 CWD = 员工能力目录**，技能、MCP、工作文件全堆在一个 per-employee 目录里；若再把该目录当 Provider home，还会切断宿主统一认证。能力目录、认证 home 与 CWD 被合并，这正是"放代码就只能往每个员工 clone"和 Provider auth 401 的根源。

## 2. 核心原则

> **项目锚定节点，员工漂浮。**
> 仓库是「文件系统状态」，搬不动 → **项目**绑节点。
> 技能/MCP 是「数据」（在 DB / 对象存储）→ **员工**不绑节点，飘到项目所在节点上现场物化。

由此衍生两条子原则：

1. **员工能力（技能+MCP）跟着员工走、可重建；项目代码跟着项目走、每节点一份；每次派工临时拼到一个工地里拉起 provider。**
2. **绝对路径不是业务事实，不被任何地方持久化，运行时派生。**

## 2.5 落地分期：先做哪层（重要）

§3–§4 描述的是目标态。**实现不要一上来就做"项目资源池 + 场景模板 requires"那套理想三层**，那是后期扩展。先做最小可用层：

### Phase 1（先做）—— 项目级显式 repo 绑定 + mode 按角色自动推

- **显式定义的事实源 = 项目管理里的「关联源码库」配置（可空）**：
  - 项目绑了 repo（repo_url、默认分支）→ 代码类项目，对它发起的任务**默认依赖该源码**。
  - 项目**没绑** repo → 普通任务类项目，任务不种 worktree，员工纯靠自身 MCP/技能干活。
- **"怎么用源码"（mode）不需要在任务上定义，由员工角色自动推导**：开发=branch worktree、审查=readonly diff、测试=detached-run、不碰代码的任务（汇报/审批/分析说明）=不种代码。
- Runtime 收到派工单：看任务关联项目有没有 repo 绑定 → 有就按"角色推出的 mode"种 worktree，没有就只开空工作目录。

> 体验上：**项目有没有源码 = 用户显式定义；这个任务怎么用源码 = 系统按角色自动定。**

### Phase 2（后续扩展）—— 资源池 + 场景模板 requires

当出现**非源码资源**（监控、日志、CMDB 等 connector）需要按场景搭配时，再把"项目级单 repo 绑定"一般化为 §3.2 的**项目资源池**，并给 Workflow Template 的 task-type 加 `requires` + `output_contract`（§4-理想态、原 §"任务资源装配"思路）。届时"代码仓库"降级为资源池里的一种资源类型，与 connector 平级。Phase 1 的"项目级 repo 绑定"是 Phase 2 资源池里 `type: git_repo` 那条的特例，可平滑演进。

> 原则不变：不建"代码/非代码"二分类型枚举；差异由"项目绑了哪些资源"自然表达（接 §2 与 CLAUDE.md「无封闭类型枚举」）。

## 3. 目录与三层职责

### 3.1 Runtime 节点磁盘布局

```
{data_root}/                              # Runtime 本地配置里的唯一根
├── employees/{employee_id}/              # 员工能力缓存：skills/MCP/context + manifest
│   ├── manifest.version.json
│   ├── skills/{skill_key}/{revision_id}/...
│   ├── mcp/{server_key}/{revision_id}/...
│   └── context/{content_hash}/...
├── repos/{project_id}/.git               # 项目主仓库：每节点一份，共享 object store
└── workspaces/{project_id}/{task_id}/    # 任务工作区：per-task git worktree（工地）
```

- 员工能力缓存**只有可从控制平面重建的能力素材，没有项目代码，也不承载默认 Provider 登录态** → 不会每个员工 clone 一份仓库，也不会因为员工目录切换而丢失宿主统一认证。
- 项目代码每节点只有一份 `repos/`；每个任务在 `workspaces/` 开一个 worktree，共享 `.git`，不复制历史 → 不膨胀。
- `node_modules`、编译产物不在 git 里，worktree 不带；依赖走**共享缓存**。

### 3.2 三层各存什么

**Runtime 本地配置 —— 只描述自己，不知道任何项目**
```
runtime_node:
  id: node-A
  data_root: /data/st
  execution_slots: 8
  labels: [linux, has-claude-code, zone-cn]
```
向控制平面注册自身（id、容量、能力标签、心跳）。**不列项目。**

**控制平面 DB —— 业务/注册状态**
- `projects`：project_id、名称、**仓库绑定**（repo_url、默认分支、`git_credential_ref`、scope）—— Phase 1 即为单 repo 绑定，可空；多资源池为 Phase 2（见 §2.5）
- `runtime_nodes`：节点注册表（id、状态、容量、labels、心跳）
- `project_placement`：项目当前落在哪个节点（**动态**协调状态，调度决定，非静态文件）

**派工单 —— 每任务下发**
project_id、repo_url、`git_credential_ref`、base_ref、scope、workspace_mode、employee_id、`capability_manifest_version`。

### 3.3 绝对路径派生，不存储

```
工作区绝对路径 = data_root ⊕ project_id ⊕ task
              = /data/st / {project_id} / {task_id}
                ↑本地配置   ↑来自 DB/派工单   ↑运行时拼
```
绝对路径节点相关、项目迁节点即失效，因此**DB 也不存**。DB 存 project_id（稳定业务键）+ 仓库绑定；Runtime 存 data_root；执行那一刻拼出绝对路径，Agent 永远只在工地根下用相对路径。

## 4. Provider 能力投影与认证边界（目标态）

三个 Provider 对工作目录、MCP、技能和认证 home 的支持不同。目标态必须区分两件事：

- **能力投影**：Runtime 把员工的 skills/MCP/context 从能力缓存投影到任务工作区或 provider-specific config。
- **Provider 认证**：默认复用 Runtime 所在服务器的统一 Provider 配置，不默认按员工改写 auth home。

| | 工作目录（放 repo / task workspace） | 能力投影 | 默认认证来源 |
|---|---|---|---|
| claude-code | `--add-dir <repo>` 或 cwd | 生成任务级 `--mcp-config <path>`；skills 软链/复制进 provider 可读位置 | 宿主 Claude Code 登录态 |
| codex | `-C/--cd <DIR>` | 生成任务级配置/上下文；skills 通过工作区投影或非 auth overlay 加载 | 宿主 Codex 配置 |
| opencode | `--dir <DIR>` | 生成任务级配置/上下文；避免覆盖宿主 auth config | 宿主 OpenCode 配置 |

**落地规则：**
- Provider 的 `current_dir` / `--dir` / `-C` 指向任务工作区，不指向员工能力缓存。
- Runtime 可以生成任务级 MCP config、skills symlink、上下文包索引，但这些文件是能力投影，不是 Provider auth home。
- 不默认设置 `CODEX_HOME=<employee_cache>/.codex` 或 `OPENCODE_CONFIG_DIR=<employee_cache>/.opencode`。只有未来显式启用 `provider_auth_mode=employee` 时，才允许员工级 Provider auth materialization。
- 若某 Provider 的 skills 只能随 home 加载，adapter 必须优先寻找不覆盖 auth 的 overlay/工作区加载方式；找不到时应把该能力标为待实现，而不是通过改写认证 home 兜底。

待真实 smoke 验证项：① Codex 在 host auth 下能否加载任务级 skills/MCP 投影；② Claude Code `--mcp-config` + 软链技能在 cwd=repo 时确实加载；③ OpenCode 在不覆盖宿主 auth config 的情况下加载任务级 MCP/skills。

## 5. 任务访问模式（按角色，不一刀切建 worktree）

「建 worktree + 分支」只绑定"产出代码变更"的输出契约。其余用更轻的工作区。`workspace_mode` 是业务事实，编排时由员工角色/输出契约算出，塞进派工单。

| 角色 | 访问模式 | Runtime 给什么 | 建分支 | 产出 |
|---|---|---|---|---|
| 检索/分析 | readonly | 某 ref 只读 checkout，多读者共享一份 | 否 | 分析结论 |
| 审查 | diff | diff/patch + base 树只读，通常无需完整 checkout | 否 | 审查意见 |
| 测试/构建 | detached-run | detached HEAD worktree + 可写 scratch + 共享依赖缓存 | 否 | 测试报告 |
| 开发/修复 | branch | 独立 worktree + 独立分支，提交、出 PR | 是 | 代码提交 / PR |

> 注意：detached-run 仅 git 语义轻，**执行风险不轻**——跑测试 = 装依赖（postinstall）、编译、起服务、占端口/连库，与开发员同样需要沙箱与端口/库隔离（见 §8）。

## 6. monorepo 范围与 worktree 语义

- worktree **不是 clone**：共享主仓库 `.git`（历史大头不复制），只铺当前版本的源码文件。N 并发 worktree = N 份源码（小）+ 一份 `.git`。
- checkout **哪个版本**：base_ref，来自仓库绑定默认分支 / 任务指定分支，随派工单下发，非 Runtime 猜测。
- checkout **哪些目录**（monorepo）：用 git **sparse-checkout**，范围来自仓库绑定 scope / 员工角色范围。**scope 必须含传递依赖闭包**（如 `apps/web` 依赖 `packages/shared`，只铺 `apps/web` 会构建失败），不能只给"自己那个目录"。

## 7. 跨任务交接与血缘（审查进哪个分支）

"谁的产出该被谁审"由协调线程在**任务血缘**里维护，审查员不自己扫分支。

```
开发任务A ──产出──> 交接单A(branch, head_commit, base_ref) ──编排引用──> 审查任务A'
开发任务B ──产出──> 交接单B ──────────────────────────────> 审查任务B'
```

- 开发干完产出**结构化交接单**（分支名、head commit、base ref），挂在任务 id 上回写控制平面。
- 协调线程据此**建审查任务并把上游引用写进派工单**；审查员只认派工单里那个 ref，Runtime 按该 ref 物化 diff。
- "整体审一次" = 编排先合到集成分支再建审查任务，仍是编排显式决定、引用推下去。

## 8. 已识别的洞与处置

| # | 洞 | 严重度 | 处置 |
|---|---|---|---|
| 1 | 分支在本地节点、跨节点不可见 | 🔴 | 项目锚定节点解决本地共享；**中央 remote 作真源/兜底**，开发"提交即 push origin" |
| 2 | git 凭据无家 | 🔴 | 归入 Runtime↔项目绑定，DB 存 `git_credential_ref`，与员工 MCP 凭据分离 |
| 3 | 节点亲和未定 | 🔴 | 亲和单位 = 项目；placement 为控制平面动态状态 |
| 4 | 等人类审批时 worktree 被进程退出清掉 | 🟠 | 工地生命周期绑**任务终态**，非进程退出；暂停/等待人类则保留 |
| 5 | 测试是带副作用的任意执行 | 🟠 | detached-run 仍需沙箱 + 端口/库隔离 |
| 6 | sparse scope 只给本目录会挂 | 🟠 | scope 含传递依赖闭包 |
| 7 | 共享依赖缓存供应链投毒 | 🟡 | 按信任级别/项目隔离缓存 |
| 8 | git 并发 + 能力缓存更新竞态 | 🟡 | 同分支不能双 worktree；shared `.git` 操作串行化；能力缓存按 (员工, manifest_version) 键，任务记录所用能力 manifest 版本 |
| 9 | 并行开发共享契约冲突 | 🟡 | 编排识别共享面做依赖排序；合并冲突 owner = 人类负责人 |

## 9. 中央 remote 的定位

中央 git remote **没有被项目亲和消灭**，而是**降级为持久化 / 故障转移层**：
- 本地共享仓库 = 快路径（同项目员工本地即可 handoff）。
- origin = 兜底：① 节点宕机后项目可迁移续作（前提是已 push）；② 一个项目挂多节点（容量/HA）时的真源。
- 因此"提交即 push origin"设为默认，即使项目单挂一个节点。

## 10. 待决策项

1. 一个项目允许挂几个 Runtime（单挂=简单+单点；多挂=必需中央 remote）。
2. 项目并行度超过单节点槽位时：排队 vs 溢出到另一挂载节点。
3. 员工能力缓存失效与版本固定策略（可复现性）：已由 `2026-06-30-runtime-digital-employee-capability-cache-auth-design.md` 给出目标态，待实现。

## 11. 对当前代码的影响点（后续实现）

- `ProviderRequest` 增加 `capability_manifest_version` 和能力投影引用；若保留 `agent_home_dir` 字段，语义必须收窄为 employee capability cache / overlay source，不代表 Provider auth home。
- `ensure_command_instance` 不再返回 `agent_home_dir` 当 workspace_path，改为派生 per-task 工作区（复用/扩展 `create_run_workspace`）。
- provider adapter（`claude.rs` / `codex.rs` / `opencode.rs`）spawn 时：`current_dir` = 任务工作区；按 §4 注入任务级 MCP/skills/context 投影；默认复用宿主 Provider auth，不默认改写 `CODEX_HOME` / `OPENCODE_CONFIG_DIR` 到员工目录。
- 派工单 / 契约：增加 base_ref、scope、workspace_mode、`capability_manifest_version`；交接单结构（branch/head_commit/base_ref）；审查任务 payload 的上游引用。
- DB：projects 仓库绑定（repo_url、默认分支、git_credential_ref、scope）；project_placement。

## 12. 跨 spec 已知缺口（本设计明确不覆盖，勿默认已设计）

> 与外环 spec（`2026-06-30-autonomous-outer-loop-iteration-attestation-budget-design.md`）共享本清单。两份 spec 都偏「代码交付 + 乐观路径」思维，下列项是两份共同的盲区，列出以划清边界——实现者不得默认它们已被设计。每项均为**独立专题，待立项**。

1. **验收标准 / 意图规格（最大语义空洞）**：两份都未定义"什么算验收通过、acceptance criteria 从哪来、怎么流到审查/测试员"。无意图规格则 attestation 与"收敛"皆悬空（证明"跑了"≠"做对了目标"）。
2. **Agent 执行隔离 + 密钥泄漏**：共享 Runtime 节点跑近乎不可信的 agent 代码（尤其客户侧执行机），沙箱模型未设计；MCP/git 密钥真实值注入进程环境后，agent 可读取并外泄，密钥生命周期/防外泄未设计。（本 spec 洞 5/7 点到未解。）
3. **非 git 副作用（数据库/迁移/外部调用）**：worktree 只隔离代码；迁移、改库、外部 API 等有状态、不可回滚副作用不在分支里，两份都未覆盖其隔离/attestation/预算纳管。直接关联宪法「删除写入、迁移、上线发布为高风险」。
4. **崩溃恢复与幂等**：Temporal 给 workflow 级持久性，但执行级未定义——节点半途崩溃（worktree/commit/迁移/attestation 各写一半）后的恢复语义、有副作用步骤的幂等性（at-least-once vs exactly-once）未答。
5. **人类飞行中可观测性 + 多分支并行收敛**：① attestation 是事后的，缺实时观测与中途叫停（与「人类决策一等」相悖）；② 迭代状态机隐含单分支，多分支并行开发→集成→整体收敛未建模。

> 处置建议：#1 与 #3 优先立独立专题；其余随实现阶段补。本 spec 的假设前提显式声明为「目标已定义、产出为代码分支、执行环境可信且成功」。
> 进度：**#1 已立项** → `2026-06-30-intent-acceptance-criteria-design.md`（意图层，定义"什么算做对了"）。#2/#3/#4/#5 待立项。

## 13. 实施同步记录

- 2026-06-30：`docs/superpowers/plans/2026-06-30-project-workspace-autonomous-outer-loop.md` 对本 spec 的 Phase 1 约束进行实现同步。本 worktree/plan 已实现或正在实现的约束包括：项目工作区与员工能力缓存/Provider auth 分离、Provider host auth 默认、项目 repo binding、Runtime attestation、budget heartbeat。真实链路验证尚未完成，当前记录只能作为实现同步说明，不能声明功能可用。
