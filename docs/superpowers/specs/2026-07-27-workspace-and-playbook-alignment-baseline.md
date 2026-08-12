# 对齐基线：一单处所与专家团剧本

- 日期：2026-07-27
- 状态：**基线已对齐**（本文只承载「为什么」与「不变量」，不承载实现）
- 目标读者：本系列全部实施 spec 的接手会话；阅读任一实施 spec 前应先读本文
- 性质：**稳定文档**。实施 spec 可以修订、作废、重排；本文的结论除非被人类显式推翻，不重议。

---

## 0. 本文的作用

本系列由 1 份基线 + 4 份实施 spec 构成。基线回答「为什么这样切」，实施 spec 回答「怎么做、怎么验」。

**不重议原则**：§4 的不变量与 §5 的已否决方案，是多轮对齐的结论。实施过程中若认为某条应推翻，先改本文并说明理由，不得在实施 spec 里悄悄绕开。

---

## 1. 平台内核判断

平台的价值定位是**在两个极端之间取企业可落地的中间态**：

| 极端 | 问题 |
|---|---|
| 完全开放的 AI 协作 | 责任边界糊，企业不敢用 |
| 每个动作都要人授权 | 人退化为工具调用审批器，用不起来 |

中间态的节奏是：

```text
开放探索 / 分析 / 起草
      ↓
人（或等价门禁）确认「下一步允许做什么」
      ↓
在已批准的范围内执行
      ↓
用验证结果收口（或进入下一闸）
```

要点：

- **批的是意图与范围**（要执行哪些操作、做到哪一步、允许动哪些对象），不是每一次工具调用。
- **打断人的时机是「权力边界变化」**：从只读到可写、从建议到对外部生效、从开发到发布、从单环境到生产、从可逆到难回滚。不是每个任务完成，也不是每个 tool call。
- 场景模板（剧本）是这套节奏的**可复用骨架**；不同领域（软件交付、运维分析、故障处置、研究报告）只是同一节奏的不同装载。

**人类如何验收**（已与人类对齐的事实前提）：AI 产出的规模与速度使得逐行审查不再现实。人更多依据**验证结果与结论**做判断，而不是通读实现细节。因此平台应优先保证「验证证据可信、范围可感知」，而不是提供更强的逐行审查工具。

---

## 2. 四层资产模型

```text
租户：场景模板目录（专家团剧本）
        roles[] + required_capabilities + skeleton + constraints + exits
              │
团队：数字员工的组织归属与能力供给
              │
项目：本仗的执行池 + 默认/允许剧本 + 项目标准与资源
              │
发起一单：选项目 × 落到某个剧本 × 收口深度 → demand
              │
同单接续：继承该单的剧本与血缘，追加工作
```

| 层 | 定义什么 | **不**定义什么 |
|---|---|---|
| 场景模板 | 抽象角色、能力要求、步骤骨架、闸、出口族 | 具体员工、具体团队、某项目的测试细则 |
| 团队 | 人从哪来、能力与权限的组织供给 | 这一仗怎么打 |
| 项目 | 谁有资格上阵、默认玩法、仓与标准 | 通用剧本本体 |
| 发起 / 接续 | 本单装配或本单延续 | 改写通用模板 |

**薄模板 + 厚项目**：共性进模板（所有软件交付都有 develop/review/test 这类缝），项目特性进项目配置与成员池（测什么、谁够格、能否上线），单次特性进发起参数（这次只分析不处置）。

**与个人向 multi-agent 产品的结构差异**：个人产品里「专家团」= 固定人设包，可随时召唤虚拟角色。本平台的数字员工是**组织资产**，必须先进入项目执行池才能被派发；剧本需要的角色不在池内时是**真实冲突**，必须显式处理（补员 / 换人 / 降收口 / 折叠 / 取消），不能假装召唤。

---

## 3. 处所模型（一单一处所）

### 3.1 一单必须有处所

一次协作跑完后，人要能回到「这一单」——看它做了什么、产出什么、验证如何、还需要我做什么、以及**在此基础上继续**。

今天这些信息散在项目详情、执行轨迹、收件箱三处，没有「这一单」的落脚点。这是接续断裂、变更不可见、流程感缺失的**共同根因**——它们不是三个独立缺陷，是同一个缺失的三种表现。

### 3.2 壳通用，右轨由剧本驱动

处所的骨架是领域中性的：

- 左 / 上：可回访的工作单
- 中：驱动这一单的协调时间线（**不是单一 conversation**——一单可能跨多个任务与多个数字员工）
- 右：这一单的当前事实状态

领域差异**只体现在右轨装什么**，且由场景模板的 `produces_defaults[].kind` 决定：

| 剧本 | 右轨内容 |
|---|---|
| 软件交付 | 变更范围、检查结果、环境 |
| 运维分析 | 数据来源、分析结论、证据引用 |
| 故障处置 | 待批 action、执行回执、目标系统状态 |
| 研究报告 | 来源清单、稿件版本 |

因此**开发与运维共用一个壳**——差异已被剧本吸收，不需要两套界面。

### 3.3 分界是驻留度，不是领域

需要人持续驻留的单（复杂排查、边看边改）与不需要驻留的单（自动化触发、单次查询）都存在，且**跨领域**：开发有定时自动修复，运维也有全程盯盘的故障排查。

因此不按领域拆页面，而是同一处所两种密度：

- **驱动态**：时间线展开、输入框常驻、右轨常显
- **巡检态**：折叠为结论 + 交付判定 + 待你处理的动作

自动化触发的单不需要聊天窗，但**需要稳定的落脚点**——出事时能进去看、能接手、能纠偏。

---

## 4. 硬性不变量（不重议）

1. **模板不绑具体员工、不绑固定团队**。绑的是抽象角色与能力要求。
2. **收口（exit）从属于剧本**，不存在跨剧本的全局「深度」枚举。先定问题类型，才谈走多深。
3. **接续继承，不重选**。同单接续禁止走新开向导、禁止重选剧本；必须保血缘并尽力 resume 原会话。
   - **接续 = 新 demand + 血缘链**（2026-08-01 拍板）。原 demand 的生命周期状态**永不回退**——`ProjectDemandStatusCanAdvance` 的单调性是承重不变量，验收闸与终态通知都建立在「终态只到达一次」之上。
   - 因此**「一单」的用户身份是血缘链，不是单个 demand 行**：卷宗的左轨、URL 与叙事必须按链聚合，否则每接续一次就多出一个「单」。
4. **chat 不强制剧本**；chat 转任务时才进入受控协作。**chat 不产生 demand，因而不进卷宗**（2026-08-01 拍板）——chat 是 `employee` 域的 `task_run`，处所只覆盖受控协作。
5. **不新建第五个观察面**。处所是对项目详情需求视图的升级，不是新页面、不新增菜单、不复活已退役的独立需求页。
6. **缺员必须显式处理**，不得静默降级为「找个人凑合派」。
7. **无人值守路径（automation 触发）不因**规则自身的**治理配置缺失而失败**；校验前移到有人在场的时刻（规则保存）。
   - **2026-08-04 修订（人类拍板）**：本条原为「永不因治理配置缺失而失败」。引入**编制**（项目 × 剧本 × 角色 → 具体员工）后出现一类新失败源：编制在保存之后因**运行期资源变化**而失效——员工被停用/删除、员工角色被改、剧本改版新增角色。
   - 这类失效**允许 automation 失败**，但必须**发失败通知并写明原因**（缺哪个角色、原选谁、为何失效）。理由：编制指向的是**具体的人**，人被停用是组织事实，不是"配置没填"；此时静默降级会让无人值守路径悄悄换人执行，比失败更危险。
   - 边界不变：**规则保存时**仍须校验编制完整，把可预防的缺失挡在有人在场的时刻。第一阶段只做「失败 + 通知」，后续按真实使用再优化（例如降收口到不需要缺失角色的那一档）。
8. **本系列属组织层与证据层**，不改 provider 管道。若实现中发现必须改 runtime 的 provider 行为，停下来评估是否越界。

---

## 5. 已否决的方案（连同理由，防止重议）

| 方案 | 否决理由 |
|---|---|
| 模板绑定具体数字员工（张三开发、李四测试） | 换项目即废、人一调岗模板集体报废、同角色多候选无法表达 |
| 模板绑定固定团队 | 团队是组织编制单元，不是「这一仗怎么打」；同团队不同项目标准仍不同，且与项目成员池抢主权 |
| 全局「目标深度」下拉框 | 深度只在某个剧本内有意义；运维分析与软件交付的出口语义完全不同 |
| 每次发起必须人手点选剧本 | 误伤接续与弱交互入口；正确做法是**可推断 + 在计划确认时钉死** |
| 「无剧本归属即拒绝创建 demand」 | 会让 automation 定时规则在无人在场时静默失败；该硬规则是设计引入的洞，已撤 |
| 飞书上做完整专家团表单（编制矩阵、豁免、多级深度） | 交互载体不匹配；且剧本已可在计划确认卡上定，旁路无需承担发起期治理 |
| 新建独立「工作台」页面 | 会推翻已完成的 IA 收敛，制造第五个观察面 |
| 逐行 diff review 工具 | 人已无法逐行审查 AI 产出；真正有用的是**范围感知**（改了哪 3 个文件、增删多少），两者不是一回事 |
| 首轮规划新建出口 pin 通路 | 触及 Temporal workflow 输入、需改现有断言、有 replay 风险；改在计划确认时选出口可完全复用现成链路 |
| 把批准物一等模型塞进第一批实施 | 范围爆炸；先立处所与接续，为批准物留接缝 |
| 接续 = 在原 demand 上追加任务 | demand 状态单调不可回退（completed/failed 同为最高 rank），要支持就得同时放开单调不变量、让收敛闸二次触发 `acceptance_pending`、把终态通知改成按「终态代次」幂等——三处承重改动换一个可用新 demand 表达的语义。且追加工作会落在**已消费过的验收闸之后**，等于产出永不过闸 |
| 让 chat 落 demand（新增 `coordination_mode=chat`）以便进卷宗 | 协调线程要额外处理一类不产生 plan 的 demand，收敛闸/验收/status recompute 全线判空；现网存量 chat run 无法回填。chat 的受控化出口已经存在且更正确：**chat 转任务** |

---

## 6. 已核实的地基事实（各实施 spec 共用，勿重复勘察）

以下事实在 2026-07-27 对齐期间经代码核实。行号会漂，以符号为准。

**场景模板**

- spec v2 结构：`roles[].required_capabilities` / `skeleton[].{step,role,depends_on,produces_defaults,required_inputs_defaults}` / `exits[]` / `constraints[]`（仅 `role_independence`、`stage_required`、`human_gate` 三种 kind）/ `collapse_rules` / `default_acceptance_criteria`。
- `produces_defaults[].kind` 已有值域：`branch_ref` / `git_commit` / `conclusion` / `evidence_ref` / `artifact_ref`——这是右轨渲染器的天然键。
- 模板解析顺序：需求级 key > 项目默认 > generic（失败落项目事件）。
- 内置 5 个模板 seed 硬编码在迁移里且**只写给默认租户**，新租户不会自动拥有。
- 模板管理有完整 CRUD + 版本化 API；Console 编辑是**裸 JSON textarea**。

**规划与出口**

- `target_exit_deliverable` 全链已通：handler → service 校验（`validateTargetExitDeliverable`）→ coordination signal → workflow 的 `PinnedExitDeliverable`。仅 `request_changes` 时有意义。
- **首轮规划没有 pin 通路**，且有现行断言固化该行为（`"initial plan must not carry a pin"`）。
- 下游解锁是 SQL 硬条件：上游 `completed + complete_accepted + validation accepted`。
- `human_gate` 主要作用于**派发前**（`RequiresHumanApproval`）；`RequiresHumanApproval` 单独不构成完成后的 post-gate。
- 缺员走 `planning_gap`（restaffed / exempted / rejected），发生在**规划之后**。

**处所与交付判定**

- 项目详情已有需求深链 `?tab=demands&demand=<id>` → `initialDemandId`，**一单的 URL 身份已存在**。
- `handoff_assessments` 已提供 delivered / missing / unknown 的结构化交付判定（delivered 判据 = Ref 已回填或 Value 非空；无声明必为 unknown）——领域中性版的「Checks」。
- IA 已完成收敛：`/workflows` 菜单已撤，`/workflows/:demandId` 退为重定向壳（飞书深链兼容）。

**会话与接续**

- provider session 按**任务血缘根**（`provider_sessions.project_task_root_id`）留存；同任务 retry、对抗返工、上游补做在派发时会自动 `resume_session`。
- chat 有独立续聊链（`chat_thread_id` + `resume_of_run_id`），`CreateRun` 的 resume 已收敛为 chat 专用（`run_kind=task` 返回 400）。
- **plan/loop 无任何「基于原会话追加指令」的 API 或 UI 入口**。
- **已知缺陷**：人工 recovery 路径 `createRecoveryReplacementTask` 新建任务时不写血缘根，导致人主动重试反而丢 session。
- resume 不可用时现网 fail-fast 无降级（chat 的降级只在 Web 前端）。
- runtime 已实现 `send_input` 命令，但控制平面从不下发。
- **demand 状态单调**：`ProjectDemandStatusCanAdvance`（`project/types.go`）按 rank 只升不降，`advanceProjectDemandStatusWithQueries`（`pg_repository.go`）不满足即静默 `return nil`；`completed/failed/cancelled` 同为最高 rank。收敛闸 `gatedCompletionStatusWithQueries` 与 `enqueueDemandResultNoticeWithQueries` 都建立在此之上。→ §4.3 的接续结论由此而来。
- **chat 无 demand**：`CoordinationMode` 只有 `plan`/`loop`（`isAutonomousCoordinationMode` 的注释 "chat once it lands" 是未落地的设想）；chat 是 `employee` 域 `task_run(run_kind=chat)`，挂 `task_id`/`project_id`，无 demand 关联。→ §4.4 的排除结论由此而来。
- **人类决策路由已从 DB 重建，不依赖 workflow 内存**：`route-human-decision-from-store` 版本之后走 `handleHumanDecisionSubmittedFromStore`（`workflow.go`），按 `decision_type` 分发；continue-as-new 丢失内存 map 不再影响新执行。**#2 的 Temporal 风险因此低于原估**：接续可复用「写 DB + 发 signal + workflow 从 store 解析」的既有模式，不改 workflow 输入、无 replay 风险。往既有 demand 图追加任务的机制也已存在（`revisionTaskCreator` / `upstreamSupplementTaskCreator` / `reworkFromAdversarialCreator`）。

**变更采集**

- 任务 workspace 是真 git worktree；`workspace_mode` 已有 `none/readonly/diff/detached_run/branch`，按 task kind 映射。
- runtime 执行结束会跑 `git diff HEAD` 并作为 `artifact_type="diff"` 的证据工件上传（落库为 `code_change` 证据），已走脱敏与截断。
- **只采未提交的 tracked 变更**；员工自行 commit 后 diff 工件为空（源码注释标为待接线）。
- attestation 的 `git_branch/git_base_ref/git_head_sha/git_diff_sha256` 四列**已于 2026-08-01 真实回填**（`collect_workspace_git_facts` → `executor.rs` 的 attestation 写回）。**但 `git_base_ref` 落的是派发下发的 `default_branch` 名字、不是基线 SHA**（`project_store.go` → `runs.rs` → attestation），故「相对哪次提交」的参照系仍然缺失；`2026-08-12` 供给模型之后平台不再 checkout，该名字更不能当测量原点。
- 无结构化 changed-files 模型；Web 端 `text/x-diff` 连预览都不支持。
- **项目工作区没有任何 git 状态事实**（2026-08-12 核实）：`WorkspaceReadyStatus` 只答「目录在不在、能否派发」；全仓唯一读过 `git status --porcelain` 的地方是认领向导的一次性 `probe_project_directory`，结果折成一个 bool 且不进项目信息。
- **`deliverables/` 采集后从不删除**（2026-08-12 核实），而终态清理明确跳过稳定项目目录 ⇒ 凡产出过声明式交付物的项目 `git status` 永久非空；且该目录无 attempt 归属，后续任务会把前任务的遗留文件当成自己的声明式交付物重报。
- 变更采集基于 git 而非 provider 事件流，因此三个 provider 天然同权（Codex/OpenCode 不产生 tool 事件）。

**发起入口（共五个，勿漏）**

| 入口 | 落点 |
|---|---|
| 任务中枢 plan/loop | `features/task-launches/`；其「选模板」是 **Prompt 模板**，不是场景模板 |
| 项目内提需求 | `submit-demand-dialog.tsx`；**已能选场景模板**，另有 `source_type`（五选）/`source_refs`/`attachments`，而任务中枢把 `source_type` 硬编码为 `manual` 且不收后两者 |
| chat 转任务 | `task-launches/index.tsx`；只带 `source_refs.chat_run_id` 血缘，不继承 session |
| 飞书旁路 | connector 已有 `ProjectPickCard`；CP 侧 `SubmitDemand(title, content, mode)` 字段偏窄；`plan_review` 卡已有 `request_changes` 按钮 |
| 自动化任务 | `internal/automation/`；支持 plan/loop/chat，规则上**已有** `scenario_template_key` |

---

### 角色词表 / 编制 / 扩编（2026-08-05～08-06 收口批核实）

- **角色词表为主**：员工 `role_keys` 与剧本 `roles[].key` 都挂租户角色词表；停用角色挡新编制与模板改版校验（`validateSpecRoles`）。
- **一角色一人**：`project_playbook_casting` 对 `(project, template, role_key)` 唯一；`PutCasting` 整套替换。
- **PutCasting 双向硬校验**：写入时员工必须持有该 `role_key`（`ErrCastingRoleNotHeld`）；移除员工角色 / 停用词表角色须 `confirm_impact`，确认后级联删编制行 + `project.casting.invalidated` 事件 + 负责人收件箱告警。
- **读路径自证**：`ValidatePlaybookCastingComplete` / readiness `missingRoles` 复查「持有 + active/ready」；直改 DB 的失真行也会被判 missing（不依赖上游守规矩）。
- **扩编 = 带新编制的重规划**：`casting_expansion` 决策批准后写入编制并 replan；发现器有触发闸（编制已满才语义探测）。
- **automation 编制失效**：fire 允许失败；`failFire` 统一发 `automation_alert` 给项目负责人集合 any-of-N；下次成功自动 resolve。

## 7. 全局非目标（本系列四份 spec 一律不做）

- 批准物（proposal / authorization）一等模型与阶段验证门禁 → 待立项
- 批准范围在执行层的硬拦截（action 白名单）→ 待立项
- 逐行 diff review 工具、side-by-side 差异查看器
- CI/CD、发布系统、变更管理系统对接
- 模板可视化编辑器、从成功项目蒸馏模板
- chat 转任务继承 provider session
- 把场景模板改成绑定具体员工或团队
- provider 协议契约化（`contracts/provider/` 的散文债）→ 独立立项

---

## 8. 实施拆分索引

| # | 文档 | 范围 | 排序理由 |
|---|---|---|---|
| 0 | 本文 | 为什么与不变量 | 所有 spec 的共同前提 |
| 1 | 一单卷宗 → [`2026-07-29-demand-workbench-design.md`](./2026-07-29-demand-workbench-design.md)（**已实施** 2026-07-31，R2 范围） | 项目详情需求视图升级为处所；中栏时间线为主/图为辅；右轨按剧本 kind；密度前端可切；统一深链；**新增** demand 只读 dossier API | **容器先行**：接续、变更、治理动作都挂它；CP 只读聚合 + Web，无写路径/无迁移 |
| 2 | 同单接续 → [`2026-08-01-demand-continuation-design.md`](./2026-08-01-demand-continuation-design.md)（**立项未实施**） | 「继续这一单」入口；**接续 = 新 demand + 血缘链**（§4.3）；**派发期按员工逐代上溯取回各自会话根**；卷宗按链折叠；recovery 丢血缘修复；resume 降级 | 用户最痛；挂在 #1 的容器上。原「待勘察」已收敛：决策路由已从 store 重建（§6），接续走既有 signal 模式即可，不改 workflow 输入、不改 planner |
| 3 | 项目工作区 git 状态可观测 → [`2026-08-12-project-workspace-git-observability.md`](./2026-08-12-project-workspace-git-observability.md)（**立项未实施**；**2026-08-12 人类拍板重定义**，原范围见下） | P0 平台产物收进 `.superteam/` 隐藏目录（顺带修声明式交付物无 attempt 归属的串味）；P1 项目一等信息新增「是否干净 / HEAD 哈希 / 未提交清单」，主动定时采 + 手动刷新 + 任务终态收尾，调度在 CP、执行走既有 `probe_project_directory` | 平台是项目目录的**唯一操作方**，却看不见现场；对运维场景无关，可独立后置 |
| 4 | 轻发起与剧本归属 | 中枢轻发起；剧本/收口/编制缺口在计划确认卡一屏定；Prompt 模板降权改名；来源字段补齐后收敛项目对话框；项目允许剧本集 | 主要是**简化与收敛**而非新建能力；依赖 #1 已存在（发起完落到哪） |

**#3 的 2026-08-12 重定义（人类拍板，按 §0「不重议原则」记录理由）**

- **原范围**：「base 记录 + commit 后 diff + numstat 文件清单；attestation git 字段回填；右轨渲染器接真数据」。
- **改为**：项目工作区 git 状态可观测（干净与否 + HEAD 哈希 + 未提交清单）+ 平台产物隐藏目录收敛。
- **理由三条**：① 原范围的 `base` 落不了地——`git_base_ref` 拿到的是 `default_branch` 名字而非 SHA（§6），且平台已不再 checkout；② 稳定项目目录多任务共用且不加锁（`2026-07-23` §0.9，本文 §3 未改）⇒「哪几行是哪个员工写的」无法诚实归属，**不得**按 commit author 猜、**不得**为采集加锁或恢复 per-task worktree；③ 人类判断：核心价值是「这个项目现在有没有遗留未提交、停在哪次提交」，而非「这一单改了什么」——前者是平台作为唯一操作方必须能答的，后者人本来也不会逐行看。
- **后置另立**（已入根目录 `TODO.md`）：demand 级变更范围感知（这一单改了哪些文件、增删多少）；右轨接平台测量事实（届时声明与测量**并列**，不一致本身是有用信号）；`attestation.git_base_ref` 语义正名。
- §5 已否决的「逐行 diff review 工具」与 §7 全局非目标「side-by-side 差异查看器」**不受影响，仍然否决**。

### 收口批登记（剧本可落地化，2026-08）

| 批 | 文档 / commit | 状态 | 摘要 |
|---|---|---|---|
| 批一 | 词表两侧对齐（能力/角色）`7a7064b5` | **已入库** | 模板与员工共用词表校验 |
| 批二 | 角色词表与编制 `7b0369de`+`1f36c944` | **已入库** | 编制矩阵、PutCasting、成员池同事务、G2 缺 operator |
| 批三 | 语义扩编 + 角色治理台 `2c6784c2`+`9bf7ed2c` | **已入库** | 扩编决策、发现器、role-view、停用 references |
| 角色治理台 | 批三内 | **已入库** | 词表 CRUD / 停用影响面 / 模板角色视图 |
| 收口批 | [`2026-08-06-casting-truth-source-closure.md`](./2026-08-06-casting-truth-source-closure.md) | **本批** | 编制事实源双向闭环 + automation 失败通知 + E2E 收敛 |

**#4 剩余范围**（剧本归属与编制矩阵已由批二/批三落地，不再算 #4）：

- 中枢轻发起
- Prompt 模板降权改名
- 来源字段补齐后收敛项目对话框
- 项目允许剧本集

**被本系列取代的文档**：`2026-07-27-expert-playbook-launch-and-continue-design.md`（其内容已拆入本文与 #2 / #4；原文重心偏在「发起表单」，勿据其分期实施）。

---

## 9. 一句话基线

> **每一次受控协作都要有处所；处所的壳是通用的、右轨由剧本决定、密度由驻留需求决定；剧本定义怎么协作而非谁来干；接续继承而不重选（新 demand 接血缘链，原单终态不回退）；治理长在处所里，不挡在入口前。**
