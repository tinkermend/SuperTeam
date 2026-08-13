# 任务中枢工作台（对话面 + 任务面）

- 日期：2026-08-13
- 状态：**评审定稿**（讨论 + 原型核对后收口；实施前对照本文，不对照已淘汰草稿）
- 原型：`docs/prototypes/task-hub-chat-workbench.html`（可点示意，非实现；示意数据须能映射到下表真实能力）
- 视觉：对照 `DESIGN.md`（Soft-Flat；工作台为高密度工作对象面，用 Soft Card / 脆数据，不把整页做成任务发起玻璃画布）
- 用户可见中文：状态/枚举经 `apps/web/src/lib/status-labels.ts`；对象用名称，不裸 UUID

**淘汰并取代**：`2026-08-13-task-hub-chat-workbench-ui-draft.md`（已删）。

**承接 / 改写**：

| 文档 | 本文关系 |
|---|---|
| `2026-08-07-task-hub-ux-remediation-design.md` §7 | 兑现其留白的 IA 重构 |
| `2026-07-13-task-hub-tri-mode-design.md` | **保留** Chat 隔离不变量与 Plan/Loop 语义；**改写**「三张对等模式卡 + 默认 Plan 表单」的壳 |
| `2026-07-16-chat-session-persistence-design.md` | **保留** `chat_thread_id` / resume；**改写**「只恢复最新一条链、不做会话列表」 |
| `2026-07-27-workspace-and-playbook-alignment-baseline.md` | **保留** Chat 不强制剧本（不变量 4）；**改写**任务中枢 / 转为任务 / 项目提需求的人手发起：改为 **必选场景模板**（不再允许空 key → generic） |
| `2026-08-13-autonomy-envelope-policy-playbook-design.md` | Chat 副作用可见面已落地（B0–B2 / P0–P4）；本文不重复发明治理模型 |
| `2026-08-12-project-workspace-git-observability.md` | 右栏 **直接复用** 项目管理已落地的项目目录 git 面板（同一 `workspace_git`、同一组件），不另造 git UI |

---

## 1. 问题

当前任务中枢是发起台：选 Plan/Loop/Chat → 填表 → 提交。进度、阻塞、会话、git 要到项目、员工、收件箱、运行总览去拼。首页不是工作记忆的锚点。

目标心智：**选一个项目工作上下文 → 中间持续交互 → 右侧挂该上下文的产物与状态。**

---

## 2. 产品定义

> **Chat** = 项目锚点上、对人负责、可持久的旁路协作；产出默认不进需求/审批/验收，除非显式晋升。  
> **任务面** = 需求主轨：Plan/Loop 只是阻塞策略，**场景模板**约束任务骨架与数字员工编制。

- 会话不临时（可跨天续问），契约相对主轨是临时的。
- Chat 与 Plan/Loop 的差别不是长短，而是**进不进主轨**。
- 场景模板是灵活与可控之间的稳定器：既不过分现场推理，也不走死 Workflow 编排。
- 相对个人 IDE（如 Codex）：会话是**项目级共享**（有项目权限的人类成员可读可写），每轮可审计是谁发的；不是每人一份私聊。

---

## 3. 锁定决策

| 项 | 结论 |
|---|---|
| 首页 | `/` 默认 **对话面** |
| 壳 | 一页两面：顶部分段 **对话 \| 任务**；共享项目上下文与右栏 git。不再用三张对等模式卡 |
| 对话左栏 | **先数字员工、后该员工在本项目的会话**。换员工 = 换一组会话，禁止跨员工续同一 `chat_thread_id` |
| 会话可见/续写 | 该项目所有人类成员可读可写（与「能看该项目 Chat」同一授权，不另开管理员特权） |
| 发起人 | 线程根创建人，不随接手改变。改名权仅发起人 |
| 标题 | 只在 SuperTeam 落库，不推 Claude Code / OpenCode |
| 列表条目 | 标题；发起人 · 接续人；最近一次人类提问两行缩略；相对时间；进行中 pill。过滤「全部 / 我发起的」是视图不是隔离 |
| 互斥 | 同一 thread 有未结束 chat run 时，任何人发送失败，提示「某某正在这条会话里」。不排队、不插话 |
| Provider 过期 | 续写留在同一 `chat_thread_id`，只提示「上下文未延续」 |
| 任务发起顺序 | **Plan 或 Loop → 必选场景模板 → 需求表单**（含转为任务、项目内提需求）。无「不绑定（通用）」 |
| Chat 本身 | 不先选剧本 |

---

## 4. 布局

```
┌──────┬─────────────────────────────────────────────────────────────┐
│ 全局 │ 任务中枢          [ 对话 | 任务 ]                            │
│ 侧栏 ├────────────┬────────────────────────┬───────────────────────┤
│      │ 左栏随面变  │ 中栏随面变              │ 右栏：项目 git + 当面 │
└──────┴────────────┴────────────────────────┴───────────────────────┘
```

**对话面**：项目切换 → 员工列表 → 会话列表 | 线程 + 技能 chip + 输入 | **项目目录 git（与项目管理同一面板）** + 技能投影 + 重命名/转为任务。

**任务面**：在途流程实例 | 发起三步或一条实例的摘要 | **同一项目目录 git**。中栏不要做成假聊天。

右栏 git 不是新能力：项目管理（项目详情）已经展示当前项目工作目录的 git 采样（分支、干净/脏、HEAD、未提交清单、采样时间、刷新现场）。任务中枢右栏 **原样挂上** `ProjectWorkspaceGitPanel`，数据仍是 `getProject.workspace_git` + `POST .../workspace/git-status/refresh`。现有 `ChatPanel` 右轨已经这样接了，工作台只是把同一块留在三栏右栏。

默认钉：进项目选最近聊过的员工（无历史则列表首位）；选员工打开最近活跃会话；「新会话」是显式动作。

---

## 5. 对话面行为

1. 员工列表 = 该项目 `active` 的 `digital_employee` 成员（与现 `ChatPanel` / `createChatRun` 门禁同口径）。
2. 会话列表 = 该 (employee, project) 下全部 `chat_thread_id`，不是只恢复最新一条。
3. 线程正文复用现 Chat：`listRuns(chat_thread_id)` + events/result；**每条人类消息必须标出发言人名称**（共享线程，不是私人气泡）。
4. 发送：`POST .../digital-employees/{id}/runs`，`run_kind=chat`，追问带 `resume_of_run_id`；技能 chip / 轻确认沿用已落地 B1/B2（勾选技能或项目 `autonomy_ceiling=pause_at_gate` → 会话内 SoftDialog，不进收件箱）。空选技能 = 项目默认面，不是「不选就没能力」。
5. 互斥：服务端拒绝；文案用占用该 run 的人类**名称**。
6. resume 因 provider 会话不可用而失败：保持同一 thread 再发（无 resume），提示上下文未延续；**禁止**前端拆新 `chat_thread_id`。
7. 转为任务：切任务面，带草稿与 `source_refs.chat_run_id`，仍走 Plan/Loop → 场景模板 → 提交。

---

## 6. 任务面行为

1. 左栏：当前项目的在途流程实例（收编今日「流程实例」页签）。状态用 `status-labels` 映射（如 `waiting_human` → 待人工确认），提交人用已有 `submitted_by_display_name`。
2. 中栏空选时走发起三步；选中实例时展示该条摘要/阻塞（数据来自现 `WorkflowInstanceSummary`：title、status、blocker、progress）。完整编制与长期资产仍在项目详情；跨项目决策仍在收件箱。
3. 场景模板卡片：`name` + `description`（`listScenarioTemplates`）；骨架摘要可从 template `spec` 已有字段取；编制人名来自该项目 `listProjectCastings` + 员工名，**不是**模板自带具体员工。`getPlaybookReadiness` 展示能否跑（缺角色警告）；第一版不另造「必须编制齐全才能提交」的新闸，除非现协调已闸。
4. 提交：`submitProjectDemand` 必带 `coordination_mode` 与 **非空** `scenario_template_key`（须为租户 active 模板）。服务端拒空。人手路径（中枢 + 项目对话框）去掉「不绑定（通用）」。存量空 key demand 不回填、跑完即止。自动化/外放仍在配置期预钉 key，与「最终 demand 必须有模板」同一不变量。

不恢复已删的 `projects.scenario_template_key`。

---

## 7. 数据对照（原型不得凭空创造）

判定：

- **已有**：现成 endpoint/字段，UI 直接接。
- **升级**：平台已有事实源，缺列表投影、聚合或校验变严。允许做。
- **不做**：现实现没有，且与平台方向不符。原型若曾画出，实施时删除。

### 7.1 已有（直接接）

| 原型块 | 能力 |
|---|---|
| 项目切换 | `listProjects` / `getProject`（名称） |
| 数字员工 | `listProjectMembers`（`digital_employee` + `active`）∩ `listDigitalEmployees`（name/role/description/avatar）；`EmployeeAvatar` |
| 开跑 / 追问 | `POST /api/v1/digital-employees/{id}/runs`：`objective`、`project_id`、`skill_ids`、`resume_of_run_id`、`interactive_confirmed` |
| 线程正文 | `GET .../runs?chat_thread_id=`；`task_title` = 该轮问题；回答从 run `result` / events（现 `ChatPanel`） |
| 技能 chip / 右栏技能面 | `GET .../projects/{id}/skill-bindings`；服务端 ⊆ 项目 Chat 允许集 |
| 轻确认 | 已落地；非技能风险标签 |
| 右栏 git | **已有，项目管理与现 Chat 右轨已接线**。组件 `ProjectWorkspaceGitPanel`；数据 `getProject.workspace_git`（`applicable` / `is_clean` / `current_branch` / `head_commit` / `repo_state` / `uncommitted_count` / `uncommitted_entries` / `sampled_at` / `sample_error` / `refresh_pending`）；刷新 `POST .../projects/{id}/workspace/git-status/refresh`。实施时 **复用该组件全文案与交互**（含「刷新现场」、展开未提交清单），不要再画一套精简 git |
| Plan / Loop | `submitProjectDemand.coordination_mode` = `plan` \| `loop` |
| 场景模板目录 | `GET /api/v1/scenario-templates`（name/description/status/spec） |
| 编制 / 可跑性 | `GET .../projects/{id}/castings`、`GET .../playbook-readiness` |
| 转为任务血缘 | `source_refs.chat_run_id`（现前端已能带） |
| 在途实例 | `listWorkflowInstances`：title、status、`submitted_by_display_name`、blocker、progress |
| 提需求字段 | `scenario_template_key` **字段已存在**，今日只是可选 |

### 7.2 升级（有事实源，缺读模型或闸门）

这些不是新业务对象，是把已有 `task_runs` / `tasks` / demand 投影成工作台能用的形状。

| 缺口 | 事实源 | 需要 |
|---|---|---|
| 会话列表 | `chat_thread_id` 已在 tasks/runs；今日 UI 只取最新一条链 | 按 (employee, project) **聚合全部 thread**：id、last_active、最近人类 `task_title` 缩略、是否有未结束 run |
| 会话标题 | 无 thread 级标题（每轮 `tasks.title` 是该轮问题，不能当会话名） | 根线程一等字段（建议根 chat task 新列或等价存储）+ 仅发起人可 PATCH；缺省自首问截断 |
| 发起人 / 接续人 / 「我发起的」 | CreateRun 已写 `tasks.creator_id`，**run JSON 不投影** | 列表与正文投影 `initiator_user_id`（根）与每轮 `creator_id`，读路径补 **display_name**（与流程实例 `submitted_by_display_name` 同口径） |
| 线程互斥 | 可扫未结束 run；今日不拒绝并发发送 | CreateRun：同 `chat_thread_id` 已有非终态 chat run → 409/400，正文带占用者名称 |
| 过期后续写 | Runtime chat 目录 TTL 默认 7 天、每项目 20 条；前端 400 后去掉 resume **会拆新 thread** | 同一 `chat_thread_id` 继续写；列表不提供「已过期」服务端布尔。UI 可用 last_active 相对 TTL 做弱提示，权威信号仍是 resume 失败 |
| 最近聊过的员工 | 无现成维度 | 用会话列表的 max(last_active) 即可，不必新实体；第一版也允许前端记住上次选中 |
| 强制场景模板 | 字段可选，空 → generic | SubmitDemand **拒空**；中枢与项目对话框去掉「通用」 |
| 实例行上的 Plan/Loop | `listWorkflowInstances` 无 `coordination_mode`；demand 列表有 | 左栏优先实例；需要策略标签时 join demand 或给 instance summary 补字段，不新造状态机 |
| 发言人名称 | 有 user id 无 list 字段 | 与上「投影 + 补名」同一升级 |

**推荐读模型（实施时走契约生成）**：`GET /api/v1/digital-employees/{id}/chat-threads?project_id=`，返回聚合项；不要让 Web 拉全量 runs 再在浏览器里 DISTINCT。标题 PATCH 挂在 thread 资源上，鉴权：发起人 = 根 `creator_id`。

### 7.3 不做（方向不符或没有能力）

| 原型/讨论中出现过 | 原因 |
|---|---|
| git **ahead/behind**（相对远程领先/落后） | `workspace_git` **没有这两项**，项目管理面板也没有。右栏要的是项目目录现场（脏/分支/HEAD/未提交文件），不是远程跟踪。**项目目录 git 本身要做、而且用现成面板** |
| 把会话名推到 Claude Code / OpenCode | 标题事实源在控制平面；provider session 有 TTL，且各 CLI 方言不一 |
| 跨数字员工续同一 thread | 员工是权限/技能/人格边界；provider session 绑在该员工落点 |
| 技能风险标签驱动确认 | 与已落地 B2 冲突 |
| Codex 式跨员工扁平时间线 | 与「先选有边界的员工」冲突 |
| Chat 先选剧本才能说话 | 破坏 Chat 旁路；剧本钉在晋升主轨时 |
| 恢复 `projects.scenario_template_key` | P0 已删；钉在 demand |
| 运行总览 / 验收 / 工件全集塞进右栏 | 会把中枢做成项目管理缩略版 |

原型里的项目名、员工名、会话正文、模板名均为**示意**；实施必须换成上述接口的真实对象。技能名来自项目 skill-bindings，不是写死「告警只读」这类展示字符串。

---

## 8. 边界

- 中枢只收敛「正在干」：对话 + 发起 + 在途实例。项目详情仍是编制/策略/资产；收件箱仍是跨项目决策队列。
- Chat 轻确认不进收件箱。
- 不把人类职责建模成数字员工。

---

## 9. 验收（实施后）

对话面：

- `/` 默认对话面；能按项目列出可聊员工，按员工列出全部历史会话（含他人发起的）。
- 会话行：标题、发起人与接续人**名称**、提问两行缩略（长粘贴不撑破）、相对时间；无裸 UUID。
- 项目成员可续他人会话；每轮气泡能看出是谁发的。
- 仅发起人可改名；名称刷新后仍在。
- 未结束 run 时发送失败并出现「{名称}正在这条会话里」。
- Provider 过期后续问：同一会话仍在，有「上下文未延续」，不出现第二条无标题孤儿会话。
- 技能 chip / git / 轻确认行为与现 ChatPanel 已验收口径一致。

任务面：

- 转为任务或切到任务面：必须先选 Plan 或 Loop，再选一个 active 场景模板，才能提交；无「通用」。
- 空模板提交被服务端拒绝（中枢与项目对话框同一不变量）。
- 左栏能列出本项目在途实例，状态与提交人为中文名称。

---

## 10. 实施顺序

1. 契约：chat-threads 列表 + 标题 PATCH；run/thread 投影 user id 与 display_name；CreateRun 互斥；resume 失败不拆 thread；SubmitDemand 拒空 `scenario_template_key`。
2. 任务中枢壳：两面替换三模式卡；对话面接列表与共享续写；任务面接强制模板与流程实例左栏。
3. 项目「提交需求」对话框与中枢同一必选模板。
4. 对照原型做视觉密度（员工区高度让给会话、技能 chip 与右栏技能面是否去重）——属 `DESIGN.md` 细化，不改本节对象模型。
