# 批三：语义扩编与角色词表注入（Semantic Casting Expansion）

- 日期：2026-08-05
- 状态：**已实施、部分验收**（2026-08-05：H1/H2/H4/H7/H8/H9/H9a/H9c/H10/H11 有真实验证；H3/H5/H6 以单测为主；P1 额外任务 UI 可见性仍弱）
- 系列：剧本可落地化第三批（批一 `7a7064b5` 词表两侧对齐；批二 `7b0369de` + `1f36c944` 角色词表与编制）
- 交付性质：新增「缺口发现器」（受约束的 LLM 调用）+ 判官触发点 + planner 角色词表注入
- 目标读者：实施会话（本文自包含；实施前必读批二 spec `2026-08-04-role-vocabulary-and-casting-design.md` §7）
- **剧本本身一行不改；扩编的写路径、决策类型、审批链路批二已建好，本批只补生产者**
- **前置依赖**：`2026-08-05-role-governance-console-design.md`（角色词表与员工角色绑定的管理界面）。
  本批 §3.4 的「去注册角色」出口依赖它；不先做，语义扩编发现了词表外的角色也落不了地。

---

## 0.0 开工须知（实施会话先读这一节）

### 两份 spec 的关系

| | 谁先 | 说明 |
|---|---|---|
| `2026-08-05-role-governance-console-design.md`（角色治理界面） | **先** | 其 P0a+P0b 是本文的硬前置 |
| 本文（批三 语义扩编） | 后 | H2/H9b 判据依赖上者的 P0a/P0b |

**阻塞点只有两条**：批三的「去注册角色」深链需要本文 P0a 的页面做落点；批三"发现器建议某角色 → 人选人"需要本文 P0b 让员工真的持有该角色，否则候选列表是空的。
治理界面的 **P0c（剧本只读视图）不阻塞本文**。本文的 P0a（发现器纯引擎，DB-free）与 P1（planner 注入）**不依赖治理界面**，可先做。

### 环境

服务用 `./scripts/dev-services.sh start|status|restart <service>` 管理（OpenFGA 需单独管）。

| 项 | 值 |
|---|---|
| Web | `http://127.0.0.1:3100`（**不是 3000**） |
| Control Plane | `http://127.0.0.1:8080` |
| 登录 | `POST /api/auth/login`，`admin` / `admin` |
| 数据库 | `apps/control-plane/config/config.yaml` 的 `postgres.url`（远程 dev 库，**无备份**） |
| 迁移校验 | 本机无 docker；`make -C apps/control-plane migrate-validate` 需自建干净 PG16 并传 `DEV_URL`。改迁移后必须 `atlas migrate hash` |
| 门禁 | `verify:contracts` / `verify:control-plane` / `verify:web` / `verify:runtime-agent` |

**`verify:runtime-agent` 有既有并发 flake**：全量跑可能随机挂一条，单独跑该用例即通过。**不是你弄坏的**，本系列 runtime 目录零改动。

### 当前数据现状（2026-08-05 实测，批二 spec §10 已过时，以本节为准）

租户 `00000000-0000-0000-0000-000000000001`，团队「默认团队」`00000000-0000-0000-0000-000000000101`。

**角色词表 10 项**：developer / reviewer / tester / collector / analyst / diagnostician / operator / verifier / researcher / writer

**数字员工 5 个**（角色绑定已被 E2E 会话扩充，**与批二 §10.2 写的不一致**）：

| 名称 | id | 已绑角色 | 能力 |
|---|---|---|---|
| 开发-A | `0be393bb-9dfd-48c8-b010-4b5abb114f23` | developer, diagnostician | code_implementation |
| 审查-B | `7a16f593-9a99-490e-bcab-77bb8b326afa` | reviewer, verifier | code_review |
| 测试-C | `157b1a2c-b2af-4a08-99f3-f16abe291ed1` | tester | test_execution |
| 运维-D | `9a623b40-c9ec-4d7d-99a4-17b1f569b52e` | analyst, collector, diagnostician | log_analysis |
| 诊断-E | `3683f032-2e24-43da-af06-5af1b8ce71a4` | diagnostician, verifier | incident_triage |

**没有任何员工持有 `operator`** —— 这是批二 G2 的判别条件，**不要"顺手"给谁补上**。

**项目**：`批二基线项目 P1` = `ca82b054-de2d-4810-9a2b-dd41f5e50a2c`
**现有编制 5 条**：`software_delivery` 三角色齐全；`incident_response` 只有
diagnostician + verifier（**没有 operator**，见下）
**demands = 0，open inbox = 0**（历史 E2E 数据已硬删）

### 编制硬校验已落地（2026-08-05，人类拍板）

`PutCasting` 现在**硬校验被编制的员工持有该 role_key**（`ErrCastingRoleNotHeld` → 400）。
理由：编制是「谁能干这个角色」的事实源，可达收口、缺员拦截、扩编候选全从这里读；
前端候选列表按角色过滤只是便利，允许 API 绕过去写等于允许这些判断静默失真。

随之清理了唯一一条存量违规行：`incident_response` 的 `operator` 位原本编制的是
**不持有该角色的开发-A**。按本节「不要顺手给谁补 operator」的原则，处置是**删掉
那条编制行**而不是发角色 —— 这同时恢复了批二 §10.3 的期望表（「故障排查只能走到
仅诊断根因，缺 operator」）在当前数据下重新成立。

对实施会话的影响：编制任何角色前先确认目标员工在 `digital_employee_roles` 里持有它，
否则 PUT 会 400。

---

## 0. 为什么还要这一批

批二把扩编做成了**编制补全助手**：协调线程在任务完成后按模板取「下一个还没编制的角色」提请补人。这有用，但它只覆盖了一半——**剧本里已经写明的角色**。

原设计的动机场景是另一半：

> 故障分诊做完，发现还涉及**网络问题核查**。剧本里没有这个角色，平台接不住。

这一半现在仍然接不住。表现是：

- `casting_expansion.go` 的请求结构里有 `SuggestedRoleKey from judge constrained output`、`NeedsExternalRole`、`ActorType "judge"` —— **结构齐备但没有任何生产者**
- `openai_compatible_planner.go` 与 `adversarial_review.go` **都没有注入角色词表**

也就是说：**接口铺好了，语义那一端是空的。** 本批只补生产者，不动写路径。

---

## 1. 目标与非目标

### 1.1 目标

1. 任务完成后，系统能从产出里**语义地**发现「还需要某个角色参与」，并映射到**角色词表**里的一项。
2. 词表外的需要，降级为自然语言提请，由**人**翻译——绝不允许模型自造角色。
3. planner 为骨架外新增的任务打上**已注册角色**，使这些任务也能进入编制与可达收口的视野。

### 1.2 非目标

| 不做 | 理由 |
|---|---|
| 让剧本表达分支/返工 | 岔路的本质是规划时信息不足，对应机制是重规划（批二已建），不是更大的剧本 |
| 模型自建角色 | 词表是注册表，扩词表是人的动作 |
| 改扩编的写路径/决策类型/审批链路 | 批二已建好，本批只接生产者 |
| 让缺口发现器直接改编制 | 它只有**建议权**，人批准才写编制（基线：数字员工不得绕过人类决策） |
| 注入**能力**词表到 planner | 能力已降为提示（批二 §0 第 9 条），注入价值低；本批只注入**角色**词表 |
| 每个任务都跑一次发现器 | 成本与噪音，见 §3.3 触发闸 |

---

## 2. 地基核对（2026-08-05 核实）

| 事实 | 锚点 |
|---|---|
| 扩编写路径已备齐 | `project.RequestCastingExpansion`：校验 `suggested_role_key` 必须是 active 词表项，否则要 `needs_external_role=true`；产出 approval + decision_request + inbox；**不改 demand 状态** |
| 扩编结构预留了判官字段 | `RequestCastingExpansionRequest.{SuggestedRoleKey, NeedsExternalRole, Reason, ActorType}`，`ActorType` 注释已写 `"system" \| "coordinator" \| "judge"` |
| 当前唯一生产者是确定性的 | `nextUncastRoleForTemplate`：按模板 exits 由浅到深取第一个未编制角色，与任务产出内容无关 |
| 幂等已有 | 每个 demand 同时只允许一张 open 的 `casting_expansion`（`hasOpenCastingExpansionForDemand`） |
| 判官是**逐判据证伪**，不是缺口发现 | `buildJudgeSystemPrompt`：「你的任务是**证伪**」；输出 schema 仅 `{"verdict":"refuted\|accepted","reason":"一句话理由"}`；按 lens 并发多次 |
| 判官触发点 | `workflow.go` 的 `AdversarialReviewForTask` Activity，在任务完成后跑 |
| 角色词表可读 | `rolevocab` 包 + `RoleVocabularyActiveKeys.UnknownKeys`；批二已给默认租户 seed 10 个角色 |
| planner 提示的骨架指令 | `openai_compatible_planner.go` 中「You may add tasks the demand genuinely needs beyond the skeleton」——这些额外任务**没有角色** |

---

## 3. 缺口发现器（本批核心）

### 3.1 不要把它塞进判官

判官的职责是**对某一条判据证伪**，按 lens 并发跑多次。把「还需要什么角色」挂到它的输出上有三个问题：

1. **语义错位**：「还需要查网络」不是对判据的证伪，是范围发现
2. **N 个 lens 会给出 N 个互相冲突的角色建议**，没有合并规则
3. 判官的 schema 是承重的（`parseJudgeResponse` 解析失败会保守判 refuted），加字段会牵动它的失败语义

**结论：独立一个调用**，职责单一——读任务产出，判断是否需要词表里的某个角色补充参与。

`ActorType` 仍用 `"judge"`（沿用批二预留值，语义是"由模型判断得出"），避免再加枚举值。

### 3.2 约束输出

这是本批对「符合当前 AI 发展现状」的兑现：**让模型从枚举里选，而不是让它自由判断组织该有什么角色。**

系统提示要点：

```text
你在评估一个刚完成的任务产出，判断为了把这一单继续做下去，
是否还需要某个**当前未参与**的角色补充进来。

可选角色仅限以下清单（role_key + 中文名 + 说明）：
  <注入租户 active 角色词表>

已参与本单的角色：<注入当前编制>

只返回一个 JSON 对象：
  {"needed": false}                                    // 不需要补人
  {"needed": true, "role_key": "<清单中的 key>", "reason": "一句话"}
  {"needed": true, "role_key": "", "external": true, "reason": "一句话"}
                                                        // 需要清单外的角色
```

**三条硬规则**：

| # | 规则 | 不这么做的后果 |
|---|---|---|
| R1 | `role_key` 必须在注入的清单里；不在则**服务端丢弃该建议并按 external 处理** | 模型编出的角色会污染扩编卡，人得先辨真伪 |
| R2 | 已参与的角色**不得**再被建议 | 会反复提请补一个已经在场的角色 |
| R3 | 解析失败 = `{"needed": false}`（保守不打扰） | 判官的 parse 失败保守判 refuted 是因为那是**闸**；这里是**建议**，失败该静默 |

R1 的服务端二次校验不能省：提示里给了清单不等于模型一定遵守，而 `RequestCastingExpansion` 已经会校验词表——本批要保证**校验失败时降级为 external 提请，而不是丢掉整条发现**。

### 3.3 触发闸（不能每个任务都跑）

发现器是一次额外的模型调用，且大多数任务完成时并不需要补人。触发条件（全部满足才跑）：

1. 任务 `complete_accepted`（与批二的协调线程提请同一时机）
2. 该 demand **没有** open 的 `casting_expansion`（复用既有幂等）
3. 该 demand 的剧本**编制已完整**——**编制不全时让批二的确定性提请先走**，它更便宜也更准
4. 本单尚未达到发现器调用次数上限（建议 3 次，走系统配置）

第 3 条是关键的分工线：

| 情形 | 谁提请 |
|---|---|
| 剧本里的角色还没编制满 | **协调线程**（批二，确定性，零模型成本） |
| 编制已满但产出显示还需要别的角色 | **发现器**（本批，语义） |

两者不会同时提请（幂等 + 分工条件），因此不需要合并规则。

### 3.4 词表外的需要

模型返回 `external: true` 时：

- 以 `NeedsExternalRole=true` + `Reason=<自然语言>` 提请（**不带** role_key）
- 决策卡上呈现为「需要一个词表外的角色」+ 原文理由 + **两个动作**：
  - 「去注册角色」→ 深链到角色词表管理，人注册后回来重开扩编
  - 「驳回」
- **绝不自动创建角色**

---

## 4. planner 注入角色词表

### 4.1 问题

planner 被允许「add tasks the demand genuinely needs beyond the skeleton」，但这些额外任务**没有角色**。后果：

- 它们不出现在编制里（编制按角色组织）
- 可达收口计算看不见它们（`distinctRolesFromSteps` 只读骨架）
- 于是「计划里有一个没人认领角色的任务」这种状态无法被治理层表达

### 4.2 改法

在 planner 系统提示里注入租户 active 角色词表，并要求：**骨架外新增的任务必须带一个已注册 `role_key`**；骨架内的任务角色继续来自模板，不受影响。

服务端在 `EnforceScenarioTemplateGovernance` 之后追加一条校验：额外任务的 `role_key` 必须已注册；未注册则**降级为无角色 + 记 `constraint_notes`**（不拒绝整个计划——planner 重试成本高，且这只是标注问题，不是治理违规）。

### 4.3 与批二的关系

批二的可达收口只算骨架角色，本批不改这个口径——额外任务的角色用于**编制可见性与审计**，不参与可达收口判定。理由：可达收口回答的是"这个剧本能走多深"，那是剧本的性质，不该被单次计划的临时任务改写。

---

## 5. API 与数据

**无 schema 变更。** 复用批二的 `project_playbook_casting`、`role_vocabulary`、`casting_expansion` 决策类型。

新增内部端口（不暴露 HTTP）：

```go
// CastingGapDiscoverer 读任务产出，在角色词表内判断是否需要补人。
// 返回 needed=false 表示不打扰；解析失败按 needed=false 处理。
type CastingGapDiscoverer interface {
    DiscoverCastingGap(ctx context.Context, in CastingGapInput) (CastingGapSuggestion, error)
}
```

实现放 `projectcoordination`（与判官同侧，复用 `chatCompletionClient`），由 app 层注入；**未注入时整条路径静默跳过**（与包内其他可选端口同约定），使无模型环境下门禁仍可跑。

---

## 5.1 UI 同步改动

本批新增三处界面改动（治理界面本身归前置 spec，不在此列）：

**① 扩编卡区分「谁提请的」**

现在卡上不显示 `actor_type`。两种提请的可信度不同，人应该看得出来：

| `actor_type` | 卡上应呈现 |
|---|---|
| `coordinator` | 「剧本里还有角色没编制：<角色>」——确定性，照剧本办 |
| `judge` | 「根据产出判断还需要：<角色>」+ **自然语言理由** ——语义推断，需要人判断可信度 |

不区分的后果是双向的：人对确定性提请过度审慎，对语义提请又过度信任。

**② 词表外提请的第二个出口**

现有卡在 `needs_external_role` 时让人从词表里挑一个角色——这是「人工翻译」，对。但缺「词表里真的没有」这条出口：需要「去注册角色」深链到角色词表管理页（前置 spec 提供），注册完回来重开扩编。

**③ 次数上限触顶要可见**

§3.3 的调用上限触顶后不再提请。界面上若毫无痕迹，人会以为「系统没发现问题」，而实际是「额度用完了」。至少在卷宗时间线留一条。

**另需确认（可能是批二遗留）**：`event_narrative.go` 里 `casting` 零命中——扩编的提请与批准**是否进卷宗时间线**待核。一单中途换了人却在时间线上看不见，是叙事断裂。若批三新增事件类型，批一的扫源码护栏测试会直接拦下漏登记。

---

## 6. 分期

| 切片 | 内容 | 可独立验收 |
|---|---|---|
| **P0a** | 缺口发现器纯引擎（提示构造 + 解析 + R1–R3），DB-free 可单测 | 假 client 单测覆盖三种返回与解析失败 |
| **P0b** | 触发闸（§3.3 四条）+ 接上 `RequestCastingExpansion` | 真实任务完成后产出扩编卡，`actor_type=judge` |
| **P0c** | 扩编卡区分 actor_type + 词表外「去注册角色」深链 + 上限触顶留痕（§5.1） | 浏览器 |
| **P1** | planner 注入角色词表 + 额外任务角色校验 | 真实规划产出带 role_key 的额外任务 |

P0a–P0c 是一个可交付单元；P1 可独立后置。

---

## 7. 验收 GATE（真实 E2E）

| ID | 步骤 | 期望 |
|---|---|---|
| H1 | 编制不满时完成一个任务 | 走**批二**的确定性提请（`actor_type=coordinator`），发现器**不触发** |
| H2 | 编制已满，完成一个产出里明显需要别的角色的任务（**配方见 §7.1**） | 发现器触发，扩编卡 `actor_type=judge`、`suggested_role_key` **在词表内**、reason 是自然语言 |
| H3 | 构造模型返回词表外 role_key | 服务端降级为 `needs_external_role=true`，**不**把编造的 key 写进卡 |
| H4 | 模型返回 `external: true` | 卡上呈现「需要词表外的角色」+ 原文理由 + 「去注册角色」深链，且**未**自动建角色 |
| H5 | 模型返回已参与的角色（R2 违规） | 服务端丢弃，不提请 |
| H6 | 模型返回不可解析的内容 | 静默跳过，任务流程不受影响，无扩编卡 |
| H7 | 同一 demand 已有 open 扩编卡时再完成一个任务 | 不重复提请（幂等） |
| H8 | 发现器调用次数达上限后 | 不再触发，留痕可查 |
| H9 | 人批准 H2 的扩编卡并选人 | 走批二既有链路：写编制 → 重规划 → 新任务派给新人；越界仍按 §7.4 拦 |
| H9a | 浏览器看 `coordinator` 与 `judge` 两种扩编卡 | 文案可区分；`judge` 卡带自然语言理由 |
| H9b | 词表外提请的卡 | 有「去注册角色」深链且可达角色词表页；注册后回来可完成扩编 |
| H9c | 上限触顶后 | 卷宗时间线可见留痕，不是静默无事发生 |
| H10（P1） | planner 产出骨架外任务 | 带已注册 `role_key`；未注册时降级为无角色 + `constraint_notes` 留痕，计划不被拒 |
| H12 | 扩编 replan 后检查图终态（§7.0 ①②） | 无 blocker 已全解却仍 `blocked` 的任务；单个任务终态拒绝不影响同批其余任务派发 |
| H11 | `verify:contracts` + `verify:control-plane` + `verify:web` | 全过 |


### 7.0 复检揪出的两个缺陷（2026-08-05，已修）

真实 E2E 全链跑通（项目 `56de8016`：发现器提请 → 人批准 → 写编制 → replan → 越界转计划确认 → 建任务派发 → 完成 → 下一轮答"无需补人"），但**终态图没人检查**，漏掉两处：

**① `planned_task_key` 合并会造出永久滞留的 blocked 任务（承重）**

`DecomposeAcceptedPlanRevision` 建任务时只看"有没有依赖声明"就置 `blocked`
（`project_store.go` 原 766 行），而解锁只由**上游任务完成信号**驱动
（`ResolveReadyDownstream`）。扩编 replan 的正常形态恰恰是把**已经完成**的上游
原样合并留在图里——挂在它下面的新任务建出来就是 `blocked`，而那个完成信号早已
发生过、不会再来。实测该项目的 评审/测试/发布 三个任务因此永久滞留，无任何看门狗覆盖。

修法：`ListDispatchableTasks` 把 `blocked` 也纳入候选，按 `ListUnresolvedBlockersForTasks`
的**真实** blocker 判定，无未解阻塞的用与 `ResolveReadyDownstream` 同一把 CAS 提回
`planned`。活动签名不变，不产生 replay 分歧。

**② 单个任务终态拒绝会拖停同批派发**

`dispatchProjectTasks` 遇到未被记录的失败直接 `return err`，整批中止。批量派发发生在
计划确认之后，一次中止就把同批新建的兄弟任务全部留在图里无人问津。实测留下一条
`workflow.signal_failed: project task dispatch rejected ... project conflict`，且**没有**
对应的 `dispatch_failed` 事件。

修法：新增 `dispatchFailureTerminal`（识别 activity 侧包出的 `ProjectTaskDispatchTerminal`），
终态拒绝只废掉该任务、留一条可见事件后继续派发其余任务；非终态错误仍按原样返回让
Temporal 重试。控制流变更用 `GetVersion("dispatch-terminal-not-batch-fatal")` 围栏。

**③ 扩编决策在一单里没有主体归属（承重，比叙事更深）**

追叙事问题时发现的真正根因：**9 条扩编决策没有一条带 `coordination_job_id` 或
`project_task_id`**。一单卷宗的决策读路径（`ListDemandLaunchDecisionRequests`）正是按
这两个外键反查的——两者都空 = 这条决策在这一单里根本找不回来。后果不止文案：
扩编卡**不出现在该单的待办**里，时间线上也只剩通用「待人工决策」。

修法两层：
1. `RequestCastingExpansionRequest` 增 `ProjectTaskID`，两个内部触发点（coordinator
   确定性提请 / judge 发现器）一律带上触发它的那个已完成任务；HTTP 入口接受可选
   `project_task_id`。
2. 卷宗时间线增 `decisionsByApproval` 索引：扩编事件既无 `resource_*` 也无
   `decision_request_id`，payload 里只有 `approval_request_id`。按它反查可让**既有**
   事件一并复原，无需数据迁移。

**④ 扩编在卷宗时间线上认不出来（叙事）**

`refineDecisionNarrative` 只细分 `plan_review` / `demand_acceptance`，扩编决策落在通用
「待人工决策」里——一单中途换过人在时间线上看不见。已补 `casting_expansion` 分支：
待扩编批准 / 扩编已批准 / 扩编被驳回，kind 归 `TimelineKindStaffingGap`。

> 注：③④ 修复只对**修复后新建**的扩编决策生效。此前 9 条历史决策缺 `project_task_id`，
> 仍显示为通用「待人工决策」，未做回填。

> **收尾教训**：三个 E2E 脚本对任务终态零断言，所以这一路是"绿着"漏过去的。
> 扩编类改动的 GATE 必须包含**图终态检查**（无 `blocked` 且 blocker 已全解的任务）。

### 7.1 H2 的造数配方

「产出里明显需要别的角色的任务」不能靠碰运气，按下面造：

```text
1) 项目：批二基线项目 P1（ca82b054-…）
2) 剧本：ops_analysis（运维分析）——只有 collector / analyst 两角色，编制容易做满
3) 编制：collector = 运维-D，analyst = 运维-D（兼任亦可）
   → 满足触发闸第 3 条「编制已完整」，确保走的是发现器而不是批二的确定性提请
4) 提需求，正文明确诱导出一个跨角色的结论，例如：
   「排查昨日 API 超时。已知应用日志无异常，请给出结论。」
5) 让 collector/analyst 的任务产出一份结论，内容里出现
   「应用侧无异常，疑似**网络链路**问题，需要网络侧进一步核查」
   —— 若真实 provider 产出不稳定，可直接写结论到 execution_summary 造数据
6) 该任务 complete_accepted 后，期望发现器提请
```

**期望的 `suggested_role_key`**：词表里没有 `network_diagnostics`（默认 seed 只有 10 个角色），因此**正确行为是 `external: true`**，即走 H4 的路径。若要验 H2（词表内命中），先在角色词表里注册 `network_diagnostics` 并让某员工持有它——这正好也验证了治理界面 P0a/P0b 的价值。

**注意**：`incident_response` 现在**缺 operator 编制**（无人持有该角色，见 §0.0），用它造数会撞上缺员拦截而不是发现器，**优先用 `ops_analysis`**。


**完成定义**：H1–H9 + H11 全过（P1 交付时补 H10）。

---

## 8. 风险

| 风险 | 缓解 |
|---|---|
| **发现器变成噪音源**（动不动就提请补人） | §3.3 四条触发闸 + 次数上限；H1 保证编制不全时它不抢戏 |
| 模型编造角色污染扩编卡 | R1 服务端二次校验 + H3；`RequestCastingExpansion` 本来就会拒未注册 key |
| 反复建议已在场的角色 | R2 + H5 |
| 解析失败拖垮任务流程 | R3 静默跳过 + H6；发现器是**建议**不是闸，失败不该有后果 |
| 与批二确定性提请打架 | 分工条件（§3.3 第 3 条）+ 既有幂等 |
| 额外任务角色校验拒掉整个计划 | §4.2 降级为标注，不拒绝——planner 重试成本高 |
| 模型成本上涨 | 触发闸 + 上限；H8 |

---

## 9. 开放细节（不阻塞立项）

1. 发现器用哪个模型：默认复用判官的 `chatCompletionClient` 配置，不单独配。
2. 次数上限默认 3，走系统配置 key。
3. 发现器输入包含什么：任务标题 + 结论摘要 + 交付物名 + 当前编制角色。**不含**完整工件内容（成本与泄漏面）。
4. 是否把发现器的"不需要补人"结论也留痕：默认不留（噪音），只在 needed=true 时记事件。

---

## 10. 一句话方案

> **判官之外单开一个缺口发现器：读刚完成的任务产出，在角色词表这个封闭清单里判断还缺谁，编制不全时让位给确定性提请，词表外的需要交给人翻译；planner 的骨架外任务也必须带已注册角色。**
