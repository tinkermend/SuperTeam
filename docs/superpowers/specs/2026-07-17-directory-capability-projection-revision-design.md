# 目录与能力投影语义修订 设计

日期：2026-07-17
状态：已与人类确认设计方向；三分期已全部落地并经真实 E2E（Phase 1/3/2 分别于 2026-07-17 合并 main,GATE 记录见 CHANGELOG 同日条目与 §8/§5 附注）
范围来源：用户发现"员工家目录版本化拉取 + 项目目录 + 任务时加载/卸载"的闭环在实现上走偏；经源码勘察证实三处断点 + chat 目录语义缺失，随后与人类逐项拍板能力投影模型与冲突策略。本 spec 是对 `2026-06-29-project-code-workspace-runtime-affinity-design.md`（下称主 spec）、`2026-06-30-runtime-digital-employee-capability-cache-auth-design.md`（下称 cache spec）的修订与最小落地，并与 `2026-07-15-skill-mcp-dependency-and-unload-design.md`（下称 unload spec，已落地）保持兼容。

## 背景与断点勘察（已核实源码，2026-07-17）

主 spec Phase 1 骨架已部分落地（项目 repo binding、`repos/{project_id}` 仓库缓存、`workspaces/{proj}/{task}/{attempt}` worktree、CWD=任务工作区、会话级 MCP 注入/回滚），但闭环断在四处：

1. **派发链路把员工家目录一次性化**。runtime 有稳定派生 `{base}/(teams/{t}/)employees/{emp}`（`apps/runtime-agent/src/instances.rs:18`），ProvisionInstance / InstallSkills 沿用它；但项目任务派发由控制平面按次派生 `{base}/project-tasks/{proj}/{task}/{attempt}/employees/{emp}`（`run_service.go:1212 projectTaskAgentHomeDir`），chat 派发按 nonce 派生 `{base}/chat/{proj}/{emp}/{nonce}`（`run_service.go:1244 chatAgentHomeDir`），runtime 会话路径原样采用 payload 的 `agent_home_dir`（`executor.rs:1009-1017`）。结果：每 attempt/每消息全新空"家"，技能 checksum 缓存（`skills.rs:103`）永久 miss，人格记忆每次全量重物化，"本地版本比平台旧才拉新"机制被击穿。
2. **会话路径不消费 `payload.skills`，技能疑似到不了任务工作区**。`materialize_skills` 仅在 ProvisionInstance 路径被调用（`executor.rs:647`）；当前 chat/项目任务派发流程（`run_service.go`）不含任何 provision 步骤；`link_provider_skills` 对缺失源静默 `continue`（`project_workspace.rs:101`）。三者叠加：一次性家目录里从未装过技能 → 软链源不存在 → 静默跳过 → provider 无技能启动且无报错。需一次真实 smoke 定罪（不排除未扫到的旁路，代码层面未找到）。
3. **卸载/清理只有 MCP 家注入一段闭环**。unload spec 落地的会话级 MCP 注入 manifest+回滚工作正常；其余不清理：技能无卸载路径、worktree 只建不删、任务级 MCP 投影（`<workspace>/.superteam/mcp/`）不删；`workspace.cleanup_policy` / `max_retained` 是死配置——仅在心跳能力上报（`daemon.rs:389`），全仓无任何按其删除目录的代码。
4. **chat 目录语义劈成两半，均未锚定项目**。chat 派发 metadata 只写 `anchor_project_id`（`run_service.go:315`，§13 修订定义的"被动锚点"），runtime 从 metadata 读 `project_id` 键 miss（`payload.rs:340`）→ 走缺省 `unscoped/manual/attempt`（`project_workspace.rs:44`）→ **节点上所有项目、所有员工的所有 chat 共用同一个 CWD**；家目录则按消息一次性。锚定项目对 chat 的执行能力零增益（摸不到仓库、任务产物、上一条消息的文件）。已知附带事实：chat 多轮会话恢复（chat_thread_id 链，2026-07-16 落地）能工作恰恰依赖这个共享 CWD 稳定——claude 会话文件按 CWD 落盘，改目录方案必须保住线程内 CWD 稳定，否则当场弄坏已验收的会话持久化。

## 已拍板的设计决策（2026-07-17，与人类逐项确认）

| 决策点 | 结论 |
|---|---|
| 缓存层级 | 节点缓存为**员工级**（`employees/{emp}`，跨项目去重），不做项目级复制 |
| 能力归属 | 能力**只属于数字员工**（角色建模决定携带什么）；项目不声明、不绑定技能；**无项目过滤轴**——能力最小化靠员工角色设计，不靠平台裁剪 |
| 加载作用域 | 每任务把员工全套能力投影进**该项目的任务工作区**，完成后随工作区消亡（作用域天然项目级） |
| 项目原生技能 | 仓库里已有的技能是既成事实的"项目公共能力"：容忍、员工能用就用；同 skill key 冲突时**项目侧优先、员工侧跳过**，但必须写入派发记录/attestation **不得静默** |
| 项目 MCP | 仓库 `.mcp.json` 等原生 MCP 配置**维持屏蔽**（理由见 §3.2）；项目公共 MCP 走能力注册表**项目级绑定**正门；凭据仍只下发 env 变量名 |
| MCP 生效模型 | 事实源在控制平面；节点上**每 run 一次性投影、随 run 消亡**，不存在被反复改写的实例级常驻配置 |
| requires 定位 | 场景模板 task-type requires = **充分性门禁**（派发前验"带没带够"，不够阻断），不是过滤器（从不裁剪员工多带的部分） |
| chat 目录 | 按 **chat_thread_id** 键（迁移 067 现成主键）；repo 项目 chat 给 **readonly worktree**；不给 branch 写（chat 无 attestation 血缘，改代码升级为项目任务）；改写主 spec §13"被动锚点" |
| 前提条件 | provider 框架支持注入通道。claude 已真实 E2E；**codex `CODEX_HOME` 技能重定位、opencode 全链 smoke 仍欠**，列为本 spec 落地验收项 |

## 1. 目录模型修订

节点 `{base_dir}` 下四分区（前三个沿用主 spec / cache spec，第四个新增）：

```
{base_dir}/
├── (teams/{team_id}/)employees/{emp}/      # 员工能力缓存：稳定、跨任务复用（修订核心）
│   ├── <provider_root>/skills/{key}/       # 技能，checksum 按需物化
│   ├── 人格记忆.md 等 workspace files
│   └── .superteam/mcp-session-manifest.json # 会话 MCP 注入清单（unload spec 机制原样保留）
├── repos/{project_id}/                     # 项目主仓库缓存（不变）
├── workspaces/{proj}/{task}/{attempt}/     # 任务工作区 worktree（不变）
└── chat/{proj}/{thread_id}/                # chat 工作目录（新，按线程键）
```

- **控制平面停止派生一次性家目录**：删除 `projectTaskAgentHomeDir` / `chatAgentHomeDir` 的按次目录语义，chat 与项目任务派发统一下发稳定员工缓存路径，派生规则与 runtime `instances.rs` 一致化（`workspace_base_dir ⊕ (team_id) ⊕ employee_id`）。绝对路径仍运行时派生、不入库（主 spec §3.3 不变）。
- 并发安全前提沿用：控制平面保证同员工同时仅一个活跃 run，稳定家目录上的会话级 MCP 注入/回滚（unload spec）与人格物化不会并发互踩。该前提一旦放宽（同员工多并发），须先给家目录写路径加锁或回到 per-run 投影，列入 §8 未决关联项。
- `chatDispatchNonce` 保留其幂等语义用于兼容执行实例 ID，不再参与目录派生。

## 2. 会话技能物化（修断点 2）

`ensure_command_instance`（`executor.rs:1004`）在 MCP 注入之后、workspace 物化前后新增技能步骤：

1. **按需物化**：对 `payload.skills[]` 逐项以 `archive_checksum_sha256` 对员工缓存做比对（复用 `install_skills` 的 `.skill-checksum` 跳过逻辑），缺失或不匹配才经 presign 下载、校验、原子替换。稳定家目录使该缓存真正跨任务生效。
2. **逐 skill key 软链**（替代整目录软链）：`link_provider_skills` 从"软链整个 `<provider_root>/skills` 目录、target 存在即整体放弃"改为逐 key 建链：工作区已存在同 key 目录（项目仓库原生技能）→ 跳过该 key 并记录；其余 key 逐个软链。
3. **落点统一**：ProvisionInstance 的 `materialize_skills` 落 `<home>/skills/`（无 provider 前缀，`skills.rs:78`）与 InstallSkills 落 `<home>/<provider_root>/skills/` 不一致，统一为后者；会话物化同。
4. **静默失败禁止**：软链源缺失不再无声 `continue`——payload 声明了技能但物化/软链失败的，按 run 级错误上报（技能是任务能力的一部分，缺了不是可忽略事件）。

## 3. 冲突策略

### 3.1 技能：项目优先、按 key、留痕

- 冲突定义：任务工作区（项目仓库检出内容）中 `<provider_root>/skills/{key}` 已存在，即与员工同 key 技能冲突。**同 key 即冲突，不比版本**（项目锚定旧版是可复现性的一部分）。
- 处置：跳过员工侧该 key 的软链，项目原生技能生效（与仓库代码同信任域，员工反正要读要跑项目代码，边际风险可容忍）。
- 留痕：冲突结果（skill key、被跳过的员工侧 revision/checksum、项目侧来源=repo）写入 run 派发记录并纳入 attestation 投影，供审计与验收查看。**不得静默**——项目侧技能未经平台治理（无 risk_level、无依赖门禁），静默覆盖等于供应链口子。

### 3.2 MCP：注册表是唯一入口，仓库原生配置维持屏蔽

屏蔽理由（勘察结论）：

- **多 provider 发现规则碎片化**：claude 读 CWD `.mcp.json`（现被 `--strict-mcp-config` 有意关闭）、opencode 合并项目根 `opencode.json`、codex 无项目级发现只认 `CODEX_HOME`。"仓库文件直接拉"= 同一能力三家三种行为、需维护三份格式、codex 永远缺席；平台投影（注册表一份配置 → 三家各自格式）正是宪法"Provider 协议语言无关"的落法。
- **凭据外流口子**："仓库任意 URL + 任意 env 变量名引用 × 平台注值"组合下，一次普通 commit 即可把平台为正规 MCP 注入的凭据路由到任意外部地址。
- **stdio 风险**：仓库常见 stdio 型 MCP 配置 = 仓库内容在会话启动时自动于节点拉起任意进程（连 readonly 审查任务也一样）；平台投影链有意只支持 http/streamable_http（`mcp_config.rs:230`），维持。

正门：能力注册表新增**项目级绑定**维度（与既有 team/employee 绑定并列）。投影集合 = 员工绑定 ∪ 项目绑定，同 server key 时**项目绑定优先**（两者均在治理链内，冲突在治理体系内解决）。凭据加密、authz、审计走既有 capability 链路，payload 仍只带 env 变量名。

可选后续（本期不做）：仓库放 provider 无关声明文件（如 `.superteam/mcp.yaml`）→ 平台导入流程登记进注册表、人类确认一次——"配置随仓库走"的编辑体验 + 治理链内的生效路径。

### 3.3 依赖门禁不变形

技能↔MCP 依赖校验（unload spec 落地）语义不变：对员工携带集合验证依赖闭包、缺失阻断派发。因无项目过滤轴，不存在"技能在、其依赖 MCP 被裁剪"的半残态。

## 4. chat 目录语义（改写主 spec §13"被动锚点"）

- **CWD = `{base_dir}/chat/{proj}/{thread_id}`**：线程内每条消息同一目录 → 文件跨消息连续、claude 会话文件落点稳定（resume 不破）；新对话新目录，与 chat_thread_id"新对话断链"语义天然对齐。
- 控制平面 chat 派发 metadata 补齐 `project_id` / `chat_thread_id` / `workspace_mode`，runtime 据此走 chat 目录派生，不再落 `unscoped/manual/attempt` 缺省洞。
- **repo 绑定项目的 chat 给 readonly worktree**（主 spec §5 角色→模式映射"检索/分析=readonly"适用），复用 `repos/{proj}` 缓存与 worktree 机制物化到 chat 目录；**不给 branch 写**——chat 无 task/attempt 血缘，产出代码变更须升级为项目任务。未绑 repo 的项目 chat 目录为空工作目录。
- 锚点语义修订为：chat 的项目锚点获得**文件系统语义**（CWD 归属 + readonly worktree），但仍不落节点亲和（`ResolveProjectTaskNode` DryRun 保留）。
- 员工能力照常投影进 chat 目录（技能软链、任务级 MCP 投影文件），与任务工作区同一套 §2/§3 规则。
- **迁移决策（2026-07-17 人类拍板）**：一刀切——切换后既存线程视同"新对话断链"重开，不做旧共享 CWD 兼容期，CHANGELOG 记录。

## 5. 清理闭环（修断点 3）

- **`cleanup_policy` 真执行**：run 终态回执后按策略处理该 attempt 的任务工作区——`on_success`（默认）成功清、失败保留供排障；`always` / `never` 字面义。清理 = `git worktree remove` + 目录删除 + `git worktree prune`；任务级 MCP 投影随工作区消亡（无需独立删除逻辑）。
- **`max_retained` 生效**：按项目维度对保留的 attempt 工作区做 LRU 裁剪，运行中 attempt 引用的不删（cache spec §5 同款约束）。
- **chat 目录清理**：纯本地 TTL——线程目录 N 天无新活动即删（缺省 7 天，挂 runtime config），另加条数兜底。不引入控制平面归档事件依赖，runtime 单侧闭环；超期旧线程续聊时文件丢失、resume 断链，与 §4 迁移的一刀切语义一致（旧线程不承诺无限期可续）。**实现口径调整（2026-07-17 Phase 3 落地）**：目录布局 `chat/{proj}/{thread_id}` 不编码员工，原定"每（项目,员工）保留 5"落地为**每项目条数兜底**（`chat_max_retained` 缺省 20）；"无活动"判定=目录自身与一级子项 mtime 最大值（深层写入不必然更新根 mtime，一级深度在准确性与扫描成本间折中，误差方向早删由天级 TTL 吸收）。终态清理与后台清扫实现于 `workspace_cleanup.rs`（终态钩子在 artifacts/attestation 与终态回写之后；早退路径由 30 分钟周期 janitor 兜底；活跃 run 引用目录一律跳过）。
- **技能缓存清理**：绑定移除后投影层天然不再软链（无需即刻物理删除）；员工缓存的物理 LRU 属 cache spec Phase 2 全量范围,本期不做。

## 6. 对上游 spec 的修订关系

| 上游 | 关系 |
|---|---|
| 主 spec（2026-06-29） | 修订 §13 chat"被动锚点"（本 spec §4）；家目录语义按本 spec §1 收敛（稳定员工缓存，派发不再一次性化）；其余（repo binding、worktree、路径派生、角色→模式）不变 |
| cache spec（2026-06-30） | 落地其 Phase 2 的**最小子集**：稳定员工缓存 + item 级 checksum 按需物化。**不做** `manifest.version.json` / manifest_version 全量清单机制——现阶段技能级 `revision_id = archive sha256` 已满足"本地版本比平台旧才拉新"，员工级统一版本号待真实需要再启 |
| unload spec（2026-07-15） | 机制原样保留（注入清单 manifest、共同收尾回滚、残留兜底）；仅注入落点从一次性 home 变为稳定员工缓存，其并发安全前提（同员工单活跃 run）不变 |

## 7. 落地分期

- **Phase 1（修功能断裂，最紧急）**：§1 稳定家目录统一派生（控制平面 + runtime）＋ §2 会话技能物化（checksum 按需 + 逐 key 软链 + 失败上报）＋ §4 chat 目录按线程键与 metadata 补齐。验收（真实 E2E 为默认完成条件）：claude 全链——技能真实出现在任务工作区并被 provider 发现调用、同员工连续两个任务第二次命中缓存零下载、chat 同线程多轮文件连续且 resume 不破；**codex `CODEX_HOME` 技能重定位 smoke、opencode 全链 smoke**（含确认 `OPENCODE_CONFIG` 生效时项目根 `opencode.json` 是否仍被合并——若合并,需评估屏蔽手段）。
- **Phase 2（治理增强）**：§3.1 冲突留痕进派发记录/attestation ＋ §3.2 注册表项目级 MCP 绑定与投影合并优先级。
- **Phase 3（清理闭环）**：§5 全部（cleanup_policy 执行、max_retained LRU、chat TTL）。

Phase 1 与 2/3 可并行立项；Phase 1 内部控制平面与 runtime 改动需同 PR 或先 runtime 后控制平面（runtime 对新旧 `agent_home_dir` 形态均可接受,控制平面切换即生效）。

## 明确不做（YAGNI）

- 员工级统一 manifest_version 全量能力清单（cache spec Phase 2 全量）——技能级 checksum 已够用。
- 项目级能力过滤轴（激活时裁剪员工携带集）——已否决,能力最小化靠员工角色建模。
- 仓库 MCP 声明文件 IaC 导入流程——可选后续,本期只留设计占位。
- 多员工共享项目工作区——per-attempt worktree 下员工间无文件系统冲突,无此需求。
- 员工级 Provider auth（cache spec Phase 4）——不在本范围。

## 8. 未决处置记录（2026-07-17 人类确认）

1. §4 迁移决策：**一刀切**，既存 chat 线程断链重开，不做旧 CWD 兼容期。
2. chat 目录清理：**纯本地 TTL（缺省 7 天）+ 条数兜底（缺省 5）**，不引入控制平面归档事件（见 §5）。
3. opencode 项目根 `opencode.json` 合并行为：**已实测（2026-07-17 GATE）——确实加载**。CWD 下故意写坏的 `opencode.json` 使 opencode 直接报错退出（"Config file … is not valid JSON(C)"），证明项目检出里的 `opencode.json`（含其 `mcp` 段）会被 opencode 进程加载——claude 有 `--strict-mcp-config` 屏蔽、codex 不读项目配置，**opencode 是三 provider 中唯一裸奔的**。屏蔽已于 Phase 2 落地（2026-07-17）：实测 `OPENCODE_CONFIG` 不抑制项目配置、发现跟 `--dir` 走、无官方禁用开关,采用 **skip-worktree+删除** 在 worktree 物化时移除根级 `opencode.json(c)`（git status/diff 零痕迹、重放幂等、其余 provider 不受影响,E2E 实证）。

**Phase 1 实施与 GATE 记录（2026-07-17，已合并 main 并真实 E2E）**：实现于 `feat/directory-capability-projection-p1`（合并 commit c3fe0a8f 系列，CHANGELOG 同日条目）。GATE 全项 PASS：① claude chat 全链——稳定家目录物化技能+逐 key 软链进 `chat/{proj}/{thread}` 工作区、readonly worktree、provider 真实发现技能；② 同线程二轮 resume——文件连续、provider 记忆延续、presign 恰 1 次（checksum 缓存命中零重复下载）；③ 项目任务路径——低风险需求零人类触点直达 completed，工作区 `workspaces/{proj}/{task}/{attempt}` 真实产物+技能软链→稳定家目录，一次性 `project-tasks/` 目录零新增；④ codex smoke——`.agents/skills` 软链被 codex 发现并自述按技能指引核对；⑤ opencode smoke——修复两处既有缺陷后全链通过：spawn 未关 stdin 致继承管道时永挂（stdin 置 null）、事件 schema 漂移（1.17 实际输出 `step_start`/`text`/`step_finish`，parser 原全部丢弃致 run 永滞 dispatching，补映射+真实样本单测）；⑥ 见上第 3 条 opencode.json 合并实测。附带发现（既有，待立项）：provider 早退/零事件时 run 永滞 `dispatching` 无看门狗，靠 stale reap 兜底。
