# 专家团剧本发起与同单接续
> 复核状态：状态：已取代（superseded，2026-07-27）

> **⚠️ 本文已被取代，勿据其分期实施。**
>
> 取代者：`2026-07-27-workspace-and-playbook-alignment-baseline.md`（对齐基线）+ 其 §8 索引的四份实施 spec。
>
> **取代理由**：本文的重心是「发起时该选什么」（一个更好的发起表单）。2026-07-27 后续对齐认定该重心有误——真正的价值载体是**发起之后人回到哪里**（一单处所）。由此产生三处方向修正：
>
> 1. **重心**：从「发起装配」转为「一单工作台」；发起改为轻量，剧本/收口在**计划确认卡**上钉死。
> 2. **成本**：因出口改在计划确认时选，可完全复用现成的 `target_exit_deliverable` 链路——本文 §7.3 单列的「首轮 pin 新建通路」（含 Temporal replay 风险与改动现有断言）**整节作废**。
> 3. **连锁简化**：本文为「无剧本即拒绝创建」引入的 automation 静默失败兜底（§4.2/§5.0）与飞书剧本选择卡（§5.3），在轻发起路线下**均不再需要**——那个洞是该硬规则自身造成的。
>
> **仍然有效、已迁入基线或后续 spec 的内容**：四层资产模型、模板不绑人不绑团队、五发起入口对账、接续继承与血缘不变量、`createRecoveryReplacementTask` 丢血缘缺陷、session 丢失降级、项目对话框来源字段回归风险（先补齐再收敛）。
>
> 本文保留作为对齐过程的历史记录。

- 日期：2026-07-27
- 状态：**已取代（superseded，2026-07-27）**
- 目标读者：接手实施的独立会话（本文自包含，不依赖原讨论线程）
- 基线与承接：
  - `2026-07-13-scenario-template-registry-design.md` / `2026-07-15-scenario-template-p2-contract-governance-design.md`（场景模板 = 角色/能力/骨架/约束/出口；需求级引用；出口剪枝；项目键为默认）
  - `2026-07-13-task-hub-tri-mode-design.md`（任务中枢 plan/loop/chat；chat 转任务）
  - `2026-07-17-feishu-integration-design.md` 与飞书旁路 `SubmitDemand`
  - 既有：`planning_gap`（补员/豁免/关闭）、项目 active executor 池、provider session 按 task lineage resume、chat `resume_of_run_id`
- 破坏性变更授权：进行中开发项目；允许任务中枢发起语义、项目内提需求入口收敛、飞书 SubmitDemand 字段扩展；**不**授权把通用场景模板改成绑定具体员工/团队。

---

## 1. 背景与问题

平台在运维协作上已能承载「人守门 + AI 执行」；在**受控多角色协作**上，入口与生命周期仍错位：

1. **任务中枢「选模板」实际是 Prompt 模板**（复用提示词，`task-launch-form.tsx` → `prompt-template-dialog.tsx`），不是专家团协作剧本（场景模板）。plan/loop 发起与「组哪支专家团、打到哪一仗」脱节。
2. **场景模板模型本身已偏正确**（抽象角色 + 能力 + 骨架 + 人闸 + 出口），且需求可覆盖项目默认。**注意（实施前必读，避免误判为从零建设）**：项目内提需求对话框 `submit-demand-dialog.tsx` **已经能选场景模板**（调 `listScenarioTemplates`，提交 `scenario_template_key`）。真正缺的是三件：任务中枢无剧本选择、**任何入口都无收口（exit）选择**、**任何入口都无编制预检**；出口与缺员今天只在规划后（计划确认卡 / `planning_gap`）才暴露。
3. **企业与个人向 multi-agent「专家团」有根本差别**：数字员工是组织资产，必须先进入**项目执行池**才能派发。剧本需要的角色不在池内时会冲突。冲突处理今天偏规划失败后的 `planning_gap`，而非发起时装配。
4. **若用「每次发起必选手点剧本」一刀切**，会误伤：
   - **同单接续**（任务跑完后基于同一会话继续）——应继承剧本，不应再走新开向导；
   - **飞书弱交互**——不能做中枢同款大表单，但可以做「先项目再短列表剧本」。
5. **发起入口有五个而非两个**（实施前必须全部盘到，见 §5.0）：任务中枢、项目内提需求、chat 转任务、飞书旁路、**自动化任务（automation）**。其中 automation 规则会无人值守地创建 demand（`internal/automation/service.go` → `SubmitDemand`，支持 plan/loop/chat），规则上已有 `scenario_template_key`。任何「无剧本即拒绝创建」的规则若不区分入口，会让定时规则在无人在场时静默失败。
6. 更上层的平台意义是：**开放探索 → 批准范围 → 受控执行 → 验证收口**（相对完全开放与逐步工具授权的中间态）。本 spec 先把「专家团剧本 × 项目编制 × 发起/接续分岔」立住；**批准物一等模型**与执行层硬拦截单列后期，不与 P0 打包。

一句话问题：

> plan/loop 新开一单时，缺少与场景模板同构的「选项目 → 选专家团剧本 → 选收口 → 编制就绪」主路径；同单接续又缺少不重选剧本的入口，导致要么治理空心，要么生命周期断裂。

---

## 2. 目标与非目标

### 2.1 目标

1. **薄模板 + 厚项目**：场景模板只定义可复用的抽象专家团剧本；项目承载成员池、默认/允许剧本、项目标准；发起时装配。
2. **plan/loop 新单（非 chat）**：无论任务中枢还是飞书，均为 **项目 + 专家团剧本** 必达归属（手选或默认），剧本内再选 **本单收口（exit）**（可默认最浅）。
3. **任务中枢**成为 plan/loop **主发起面**；项目内「提需求」收敛为跳转中枢（带 `project_id` 等预填），不维护第二套提交语义。
4. **chat** 不强制剧本；**chat 转任务**进入 plan/loop 发起场，再确认剧本/收口。
5. **同单接续**与「新开一单」拆开：接续不选剧本、继承 demand 已钉剧本与编制上下文，并走既有 task lineage / provider session resume 能力。
6. **编制预检**前移到发起路径（中枢完整；飞书薄提示 + 事后 gap 兜底）。
7. 为后续「阶段批准物 / 权力边界变化处打断」留接缝，但 P0 不实现。

### 2.2 非目标（显式排除）

- 场景模板绑定具体数字员工 UUID 或绑定固定团队。
- 将 Prompt 模板升级为剧本；Prompt 仅保留为「插入提示词」附属能力。
- diff/变更集一等 UI（软件场景附证，另项）。
- 一等 `proposal/authorization` 对象与执行层 action 白名单硬拦截（P2+）。
- 工业级 CI/CD、发布系统对接。
- chat 转任务继承 provider session（保持现网：仅 `source_refs` 血缘；增强另项）。
- 执行中途「升档」exit 的完整产品（P1 可做最小：同 demand 重规划 + 钉新 exit；P0 不做）。
- 模板可视化编排器、从项目蒸馏模板。
- 改变数字员工「不绑死 Runtime、派发时动态解析」等宪法级模型。

---

## 3. 拍板结论（2026-07-27 讨论收敛）

| # | 结论 |
|---|---|
| 1 | 平台内核节奏：**开放段 → 批准范围 → 受控执行 → 验证收口**；场景模板是节奏的可复用骨架，不是花名册。 |
| 2 | **薄模板 + 厚项目**。模板不绑人、不绑团队；团队是人员供给上游；项目是执行池 + 默认/允许剧本 + 项目标准。 |
| 3 | plan/loop **新单**必须有剧本**归属**（用户选择或项目默认），不是「治理上可有可无」。 |
| 4 | **不是**「每次人手必点剧本」：有项目默认时可一键确认；飞书/中枢同构，厚度不同。 |
| 5 | **收口（exit）从属于剧本**，不是全局深度枚举。先选问题类型/剧本族，再选该剧本 exits。无 exits 则不展示收口选择器。 |
| 6 | **项目与剧本都要选**（顺序可项目优先或剧本优先，仅影响默认值填充，不改必选集合）。 |
| 7 | **chat** 不强制剧本；**chat → 转任务**时进入发起场并确认剧本。 |
| 8 | **项目内提需求**不做独立表单闭环，**跳转任务中枢**并预填项目（及项目默认剧本）。 |
| 9 | **同单接续**禁止再走新开向导、禁止重选剧本；复用 lineage/session。 |
| 10 | **飞书**可融合：先项目，再按项目过滤的专家团短列表；不是只能吃默认、也不是做中枢级编制大表。 |
| 11 | 缺员：中枢 P0 **预检 + 默认阻断提交**（给出引入/降收口/换项目出路；引入可先深链项目成员页）。飞书允许提交后 `planning_gap` 兜底。 |
| 12 | 批准物一等模型与硬执行约束：**不进 P0**，进 P2 预留。 |
| 13 | 业务对象不另造 launch：中枢/飞书 plan/loop 仍创建 **project demand**，`scenario_template_key`（及 exit 钉扎）进入既有协调规划链。 |

---

## 4. 核心模型

### 4.1 四层资产与职责

```text
租户：场景模板目录（专家团剧本）
  roles[] + required_capabilities
  skeleton + constraints + exits + defaults
        │
团队：数字员工组织归属与能力供给（不定义剧本）
        │
项目：执行池 + 默认剧本 + 允许剧本集 + 项目标准/资源
        │
发起一单：选项目 × 选剧本 × 选收口 × 编制装配 → demand
        │
同单接续：继承 demand 剧本/exit/lineage → 追加工作（不新开向导）
```

| 层级 | 定义什么 | 不定义什么 |
|---|---|---|
| 场景模板 | 抽象角色、能力要求、步骤骨架、闸、出口族 | 张三/李四、某团队、某项目测试细则 |
| 团队 | 人从哪来、组织能力与权限 | 这一仗怎么打 |
| 项目 | 谁能上阵、默认/允许哪几套剧本、仓与标准 | 通用剧本本体 |
| 发起/接续 | 本单装配或本单延续 | 改写通用模板 |

### 4.2 剧本归属解析（新 demand）

写入 demand 时解析并**落库** `scenario_template_key`（可空仅在策略显式允许 generic 时）：

```text
显式传入（中枢/飞书/转任务表单/automation 规则）
  > 项目默认 projects.scenario_template_key
  > （可选租户策略）generic
  > 否则拒绝创建
```

**「拒绝创建」的适用边界（关键，勿一刀切）**：拒绝只允许发生在**有人在场的交互路径**（中枢提交、飞书发起、chat 转任务、**automation 规则的创建/编辑时**）。

**无人值守的触发路径（automation 定时/事件触发）永不因缺剧本失败**：触发时按 `规则 key > 项目默认 > generic` 回落并落审计事件；校验前移到规则保存那一刻。理由：定时规则的失败没有人在场处理，静默丢单比治理降级更糟。

规划期继续沿用既有：demand 键 > 项目默认 > generic 回落事件；**本 spec 要求主路径尽量在创建 demand 前就解析完**，避免「建了单才发现无剧本」。

`exit_deliverable`：

- 发起时可显式选择；
- 未选时：该剧本 **最浅 exit**（与 planner「prefer shallowest」一致）；
- 人在计划确认卡仍可改选（既有 `target_exit_deliverable` / pin）；
- 单出口或无 exits：不展示收口选择器。

### 4.3 新开 vs 接续（生命周期不变量）

| 动作 | 新 demand？ | 选剧本？ | 选收口？ | Session |
|---|---|---|---|---|
| 中枢/飞书新开 plan/loop | 是 | 要有归属 | 要有归属（或默认最浅） | 新规划/新任务线 |
| chat 转任务 | 是 | 要 | 要（或默认） | 不继承 chat session（P0） |
| **同任务/同 lineage 接续** | **否** | **否（继承）** | 默认继承；升档另议 | **resume 原 provider session** |
| 系统 retry / 对抗返工 / 上游补做 | 否（既有任务图） | 否 | 否 | 既有 resume 逻辑 |
| 无关新目标 | 是 | 要 | 要 | 新 |

**禁止**：把「继续此任务」做成再次打开完整发起向导并清空/重选剧本。

### 4.4 编制装配（角色 → 项目池）

模板 `roles[].required_capabilities` × 项目 active 数字员工成员池 → **编制就绪表**：

| 角色 | 能力要求 | 项目内候选 | 状态 |
|---|---|---|---|
| developer | code_implementation | A, B | 就绪 |
| reviewer | code_review | — | 缺员 |

- **匹配键是能力，不是员工 ID。**  
- 折叠规则（`collapse_rules`）允许的一人多角须在就绪表上**显式标注**（非静默）。  
- 出口剪枝后用不到的角色不参与缺员判定（选浅出口可避开缺员角色）。  
- 中枢 P0：任一**硬缺口** → 阻断提交，并给出：深链补员 / 降低收口 / 更换项目 / 取消。  
- 飞书 P0：可提交；创建后规划若仍缺人走既有 `planning_gap`；卡片上尽量一句风险提示。  
- P1：中枢内嵌补员/刷新预检；能力缺口从 advisory 收到「剧本角色硬缺口」与规划硬失败对齐。

### 4.5 与「专家团固定 bot 包」的差异（产品说明用）

个人向产品：专家团 ≈ 固定人设包 + 流程，可随时「召唤」虚拟角色。  
本平台：专家团剧本 ≈ **协作与治理契约**；上场的是**项目内真实数字员工**；缺员必须组织动作（引入成员），不能假装召唤。

---

## 5. 入口设计

### 5.0 入口全景（五个，实施前逐个对账）

| 入口 | 代码落点 | 剧本 | 收口 | 编制预检 | 缺剧本时 |
|---|---|---|---|---|---|
| 任务中枢 plan/loop | `features/task-launches/` | **必达归属**（选或项目默认） | 选或最浅 | **完整，硬缺口阻断** | 拒绝提交 + 中文提示 |
| 项目内提需求 | `features/projects/components/submit-demand-dialog.tsx` | 已有选择器，补齐同中枢 | 补齐 | 补齐（可弱化） | 拒绝提交 |
| chat 转任务 | `features/task-launches/index.tsx` | 必达归属 | 选或最浅 | 同中枢 | 拒绝提交 |
| 飞书旁路 | `apps/feishu-connector/` + CP `internal/feishu/connector_business.go` | 薄选（项目过滤短列表），可省略 | exits ≤3 用按钮，否则最浅 | 不做大表，一句提示 | 项目默认；仍无 → 拒绝并提示去配项目默认 |
| **自动化任务** | `internal/automation/` | 规则字段已存在（`scenario_template_key`） | 规则可选字段（新增） | **不做** | **规则保存时校验；触发时永不失败**，按 §4.2 回落 |

补充规则：

- **automation 的 chat 模式规则豁免剧本**（与 §4.3 chat 一致），仅 plan/loop 规则受本文约束。
- 五个入口共用同一套 demand 字段与同一套服务端解析（§4.2），**不得各自实现回落顺序**。
- chat 本体（非转任务）不在本表内，永不要求剧本。

### 5.1 任务中枢（主路径）

**plan / loop 模式发起场必含：**

1. 模式：Plan | Loop（chat 仍独立）  
2. **项目**（必选）  
3. **专家团剧本**（场景模板；预填项目默认；选项 = 项目允许集，若未配允许集则租户可用模板列表）  
4. **本单收口**（所选剧本 `exits`；无则隐藏；默认最浅）  
5. **编制就绪**摘要（P0 最少：就绪 / 缺员列表；缺员阻断）  
6. 目标描述  
7. **插入提示词**（原 Prompt 模板能力改名，避免与剧本撞名）

提交：`POST` 创建 project demand，带：

- `project_id`
- `coordination_mode`: plan | loop  
- `scenario_template_key`  
- `source_refs`（若从 chat 转来）  
- 建议新增可选：`target_exit_deliverable`（创建时钉意向出口；规划 snapshot 作 pin 或写入 demand 扩展字段——实现选「demand 字段」或「首轮 pinned 经协调信号/元数据」，但 E2E 必须保证浅选不被 planner 擅自走深）

**chat 模式：** 保持现网；不出现剧本/收口必选。  
**转为任务：** 切到 plan/loop 发起场，预填目标文本与项目（若 chat 带 project），**剧本/收口按 §4.2 必达归属**。

### 5.2 项目内提需求

- 项目详情 CTA「提需求 / 发起任务」→ 路由跳转任务中枢，query 至少：`project_id`，可选 `scenario_template_key`（项目默认）、`mode=plan|loop`。

**收敛顺序有硬前提，不得直接删对话框（否则是功能回归）**：

现有 `submit-demand-dialog.tsx` 支持而任务中枢**当前不支持**的字段：

| 字段 | 对话框 | 任务中枢现状 |
|---|---|---|
| `source_type` | manual/github/ticket/document/log 五选 | **硬编码 `"manual"`**（`task-launch-form.tsx`） |
| `source_refs` | 自由 JSON | 仅 chat 转任务时写 `chat_run_id` |
| `attachments` | 多行文本 | 不收 |

因此分两步：

1. **先补齐**：任务中枢增加可折叠「来源与附件（高级）」区，覆盖上述三字段（默认收起，默认 `manual`，不打扰主路径）。
2. **再收敛**：补齐并 E2E 验证后，项目 CTA 改为跳转，对话框停用/删除。

**在第 1 步完成前不得执行第 2 步**；G5 门禁需同时覆盖「跳转后仍可录入 github 来源需求」。

### 5.3 飞书旁路

现网 `SubmitDemand(title, content, mode)` 过窄。P0 扩展为与中枢同构的**薄参数**：

| 字段 | 必填 | 说明 |
|---|---|---|
| project_id | 是 | 先选项目 |
| coordination_mode | 是 | plan/loop |
| title/content | 是 | 目标 |
| scenario_template_key | 否 | 省略则项目默认；仍空则拒绝或按租户策略（**默认拒绝**，提示配置项目默认剧本） |
| target_exit_deliverable | 否 | 省略则最浅 exit |

**交互建议（不强制像素级）：**

1. 选项目（用户可见项目短列表）  
2. 选专家团（**仅该项目默认 + 允许集**；默认项高亮，可一键确认）  
3. 收口：exits ≤ 3 用按钮；更多则默认最浅并文案「可在控制台计划确认时修改」  
4. 描述 → 提交  

**不做：** 编制矩阵、豁免约束、接续与新开混在同一卡片重选剧本。

**改动面必须覆盖 connector 进程（独立 Go 模块，易被漏）**：

| 位置 | 改动 |
|---|---|
| `apps/feishu-connector/internal/cards/` | 新增剧本选择卡（可参照既有 `ProjectPickCard` 的分页/计数写法）；补 cards 测试 |
| connector 多轮会话态 | 进程内存 + TTL、重启即丢（既有刻意边界）；发起流程从「项目→模式→内容」变为「项目→剧本→(收口)→模式→内容」，**多一步即多一次 TTL 暴露**，需确认超时文案 |
| CP `internal/feishu/connector_business.go` | `SubmitDemand` DTO 扩展（§5.3 表） |
| connector `my-projects` 之外 | 需要一个「按项目列可选剧本」的 connector 端点或复用既有模板列表 + 项目允许集过滤 |

connector 铁律不变：不连库、不持业务状态、不自行判权。

### 5.4 同单接续入口（P0 必做产品切口）

在任务详情 / 执行完成态 / 需求运行态提供 **「继续此任务」**（文案可调，语义固定）：

- 输入：追加目标/指令（必填）  
- 服务端：在**原 project_task 的 lineage root** 上创建接续性工作（优先复用既有 revision/rework 任务创建路径并写入 `revision_root_task_id` / 等价血缘，确保 `FindProviderSessionForTaskRoot` → `resume_session`）  
- **不**创建新 demand（P0）  
- **不**展示剧本/收口选择器  
- 修复已知不一致：人工「重试任务」类 recovery 若新建任务却不写血缘根导致丢 session，与本入口一并归入「接续必须保血缘」不变量（实现时修 `createRecoveryReplacementTask` 一类路径）

验收标准：同一 provider session 在接续 run 上被 resume（或可观测的等价续聊锚点），而非静默 `start_session`。

**session 已丢失时的行为（必须显式定义，否则各实现不一）**：

现网 resume 在 session 被 LRU 清理或 attempt 落到别的 runtime 节点时是 **fail-fast 无降级**（chat 的降级只发生在 Web 前端）。接续入口的规定行为：

1. 优先 resume；
2. resume 不可用时**降级为新会话但不静默**——必须在接续任务上留下结构化标记与用户可见提示（「原会话上下文已不可用，本次为新会话，请在指令中补充必要背景」）；
3. **禁止**两种做法：静默开新会话（用户误以为有记忆）、直接失败让用户无路可走。

G9 门禁需覆盖正向 resume；session 丢失分支可用直插/清理构造，至少验证提示与标记存在。

---

## 6. 项目层最小加厚（支撑薄模板）

P0 项目配置最少：

| 项 | P0 | 说明 |
|---|---|---|
| `scenario_template_key` 默认剧本 | 已有 | 发起预填与飞书回落 |
| 允许剧本集 | **新增（可空=不限制）** | 空 = 租户内可用模板；非空 = 中枢/飞书选项白名单 |
| 默认收口 | 可选 | 不配则剧本最浅 |
| 成员池 | 已有 | 编制预检唯一人员来源 |

P1 可加：角色 → 默认候选人映射（仍是项目私有，不写回通用模板）。

---

## 7. 控制平面与契约（实施要点）

### 7.1 API / OpenAPI

- Demand 创建（中枢、项目跳转复用、飞书网关）统一可选/必填策略见 §5。  
- 新增只读 **编制预检**（推荐）：  
  `GET/POST .../projects/{projectId}/scenario-template-staffing-preview`（术语与仓库既有 `scenario-template` 对齐，勿引入 `playbook` 作为 API 词）  
  body/query：`scenario_template_key`, `target_exit_deliverable?`  
  返回：剪枝后角色列表、候选员工（id+显示名）、缺口、折叠标注。  
  数据来源已具备：`ListProjectMembers`（`principal_type=digital_employee` 且 active）× 员工 planning profile 的 `capability_bindings`（external_capabilities / skills）。  
  中枢提交前调用；飞书/automation 可不调。  
- 接续：新增明确命令或复用/扩展现有 task 决策 API，**禁止**仅靠前端再 POST 一条新 demand 冒充接续。契约上名称与语义须让「非新 demand」不可误解。  
  候选实现路径（实施时二选一并说明理由）：**(a)** 复用协调侧既有返工任务创建路径（`CreateRevisionTaskForResult` 一族，天然写 `revision_of_task_id` / `revision_root_task_id`，session resume 免费获得）；**(b)** 新增独立接续命令但复用同一血缘写入函数。  
  **无论选哪条，都必须一并修复既有不一致**：人工 recovery 路径 `createRecoveryReplacementTask` 新建任务时不写血缘根，导致「人主动重试」反而丢 session（与本文接续目标相反）。
- 飞书 connector `SubmitDemand` DTO 扩展字段；无字段旧客户端走项目默认。  
- 全部进 `contracts/control-plane/openapi.yaml`，`generate:control-plane` + `verify:contracts`。

### 7.2 数据

- demand 已有 `scenario_template_key`：主路径创建时写入。  
- 若需发起时持久化意向出口：优先 demand 可空列或 plan 首 revision 元数据；**不得**只活在前端 state。  
- 项目允许剧本集：项目配置 jsonb 或旁表（实现选简单可迁移方案）；校验创建 demand 时 key ∈ 允许集（若集非空）。  
- **禁止**迁移把员工 ID 写进 `scenario_templates.spec`。

### 7.3 规划链：首轮出口钉扎（**本 spec 风险最高的实施项，单列**）

**现状（已核实，勿假设已具备）**：

- `PinnedExitDeliverable` today **只在重规划路径被赋值**：`workflow.go` 的 replan 分支从 `signal.TargetExitDeliverable`（人在计划卡 request_changes 改选出口）写入 snapshot。
- **首轮规划没有任何 pin 通路**，且存在一条现行断言明确固化该行为：`workflow_test.go` 中 `require.Equal(t, "", planner.snapshots[0].PinnedExitDeliverable, "initial plan must not carry a pin")`。
- 因此 planner 首轮选出口只受 prompt 的「prefer shallowest」软约束，**服务端无强制**。

**要做的（这是新建通路，不是接线）**：

1. demand 承载发起时的出口意向（可空列或等价持久化，§7.2）。
2. `LoadProjectCoordinationSnapshot` 装载时把该意向写入 `snapshot.PinnedExitDeliverable`（首轮）。
3. 既有图校验 `plan.ExitDeliverable != snapshot.PinnedExitDeliverable → invalidRouteDecision` 自然生效（此处确为复用）。
4. **必须修改上述现行测试断言**，并说明语义变更：首轮无意向时仍为空 pin，有意向时携带。

**Temporal 风险与门禁**：

- 该改动触及 coordinator workflow 的规划输入。实施后**必须跑 `apps/control-plane/internal/workflow/projectcoordination/replay_test.go`**，并评估是否需要 `GetVersion` 围栏（判据：是否改变已有历史的 activity 调用序列/入参可见行为；仅新增 snapshot 字段通常安全，但**必须实测 replay 而非推断**）。
- 若 replay 出现分歧，**优先降级方案**：首轮不 pin，改为「planner 选深于用户意向时，在计划确认卡强制标红并要求人确认」——牺牲自动强制，保住 replay 安全。此降级不影响 §9 其余门禁。

其余规划链约束：

- 无模板归属且策略拒绝 generic：不得进入规划。
- `planning_gap` 保留为飞书/automation/竞态/规划期兜底，**不再是中枢主路径的首次缺员体验**。

### 7.4 授权与显示

- 剧本/收口/角色/员工一律中文标签；对象指称「名称 (id)」；枚举走 `status-labels.ts`。  
- 飞书卡片文案自足，不依赖用户猜英文 key。

---

## 8. Web / 飞书 UX 约束

1. 中枢文案：**专家团剧本**（或「协作剧本」），对应 API `scenario_template_*`；原模板选择器改为 **插入提示词**。  
2. 先选项目再过滤剧本列表（推荐默认顺序）；若先选剧本，项目列表可提示「需含匹配成员」，但仍两都必选。  
3. 缺员阻断态必须可理解：缺哪个角色、要什么能力、去哪补员。  
4. 接续入口与发起入口视觉分离，避免同一主按钮复用。  
5. 设计变更前读 `DESIGN.md`；中文与词表护栏测试保持绿。

---

## 9. 分期

### P0 — 主路径打穿（本 spec 默认交付范围）

1. 任务中枢 plan/loop：项目 + 剧本 + 收口 + 编制预检（阻断）+ 提示词降权改名 + **来源与附件高级区**（§5.2 第 1 步）。  
2. 项目提需求 → 补齐后再跳转中枢预填（顺序不可颠倒）。  
3. chat 转任务 → 进入发起场确认剧本/收口。  
4. 飞书 SubmitDemand 扩展 + 薄选（项目→剧本短列表）**+ connector 卡片与会话态改动**；省略剧本吃项目默认。  
5. **automation：规则保存期校验 + 触发期永不失败回落**（§4.2 / §5.0）；chat 规则豁免。  
6. demand 创建落剧本归属；**首轮 exit 钉扎通路（§7.3，含 replay 门禁与降级方案）**。  
7. **继续此任务**接续入口 + 血缘/session 不变量（含 recovery 丢血缘修复 + session 丢失降级提示）。  
8. 项目允许剧本集（可最小实现）。  

**实施顺序建议**（降低返工）：先 6（出口钉扎，风险最高、可能触发降级方案）→ 1/2（中枢与来源字段）→ 5（automation 兜底，防止半夜炸）→ 4（飞书）→ 7（接续）→ 3/8。

**P0 真实 E2E GATE（必须全过才算完）：**

| ID | 场景 | 期望 |
|---|---|---|
| G1 | 中枢 plan + 软件剧本 + 最浅 exit | demand 带 key；计划骨架仅为浅出口祖先闭包；不出现未选的深层必做阶段 |
| G2 | 中枢 loop + 运维分析类剧本 | 同 G1 结构，角色/出口为该剧本语义 |
| G3 | 项目无默认且用户不选剧本 | 中枢拒绝提交，中文提示 |
| G4 | 缺评审角色且 exit 需要 review | 中枢预检阻断；降到不需 review 的 exit 后可通过 |
| G5 | 项目内提需求 CTA + **来源字段不回归** | 进入中枢且 project 预填；且能录入 `source_type=github` + `source_refs` + 附件（§5.2 第 1 步） |
| G6 | chat 转任务 | 必须经过剧本归属确认后才建 demand；`source_refs.chat_run_id` 仍在 |
| G7 | 飞书：只选项目+模式+文案，项目有默认剧本 | 建单成功且 demand 剧本=项目默认 |
| G8 | 飞书：选项目后改选允许集内另一剧本 | demand 为所选 key |
| G9 | 任务完成后「继续此任务」 | 新工作挂原 lineage；runtime/CP 侧 resume 原 session（非新 session 冷启动） |
| G9b | 接续但 session 已不可用 | 降级为新会话**且**有结构化标记 + 用户可见提示，不静默、不失败 |
| G10 | 回归：chat 追问 | 仍不要求剧本；resume 链不被破坏 |
| G11 | **automation 定时规则触发（规则未配剧本、项目有默认）** | 正常建单，剧本=项目默认，**不因缺剧本失败**；审计可见回落 |
| G12 | **automation 规则保存时未配剧本且项目无默认** | 保存阶段即拒绝并中文提示（把失败前移到有人在场时） |
| G13 | **人工 recovery「重试任务」** | 新任务保留血缘根，session 被 resume（修既有丢 session 缺陷） |

### P1 — 企业感变硬

- 中枢内嵌补员/借调后刷新预检。  
- 剧本角色能力硬缺口与规划失败对齐（减少「advisory 却放行」）。  
- 项目默认收口、角色默认候选人。  
- 计划确认卡与发起场字段展示一致（模板、出口、折叠、缺口）。  
- 飞书缺员风险提示文案。  
- （可选）同 demand 升档 exit 的最小重规划。

### P2 — 批准物与平台内核（另开实现切片，本 spec 只留接缝）

- 跨场景 `proposal/authorization`：批准物内容随剧本变化（action 清单、放行范围、发布窗…）。  
- 模板只声明「哪类缝要提案」，不写死内容。  
- 先软约束（下游必带授权上下文），再评估执行硬拦。  
- 与接续边界：授权段内可续；跨权力边界必须再批。

---

## 10. 风险与缓解

| 风险 | 缓解 |
|---|---|
| 员工能力数据稀疏导致预检常红/常绿失真 | P0 预检可先「角色有无任一候选成员」+ 能力弱提示；P1 再硬能力门。实施前抽样真实项目池。 |
| 项目未配默认/允许集，飞书难用 | 创建项目引导必选默认剧本；飞书在无默认时拒绝并中文说明。 |
| Prompt 与剧本撞名残留 | 文案与测试双重改名；文档/CHANGELOG 写清。 |
| 接续与新开再次被做混 | G9 门禁；API 分离；UI 分离。 |
| planner 无视发起 exit | 发起 pin + 服务端 `PinnedExitDeliverable` 校验（既有图校验能力）。 |
| 把 P2 批准物塞进 P0 | 非目标列表与分期强制拆开；评审挡范围膨胀。 |
| **automation 定时规则因新校验静默失败** | 校验前移到规则保存；触发期只回落不失败；G11/G12 门禁。 |
| **首轮 pin 触发 Temporal replay 分歧** | §7.3 已备降级方案（改为计划卡强制标红），replay 未过不合入。 |
| **收敛项目对话框造成来源字段回归** | §5.2 两步顺序 + G5 覆盖 github 来源录入。 |
| connector 多轮会话多一步导致 TTL 超时率上升 | 实施时确认超时文案与 TTL 值；必要时把剧本步与项目步合并为一张卡。 |

---

## 11. 仍开放但**不阻塞 P0** 的细节

实施时可按括号默认值推进，不必再开概念讨论：

1. 中枢表单默认先项目还是先剧本：**先项目**（利好剧本过滤与飞书同构）。  
2. 无允许集时列表范围：租户全部 active 场景模板。  
3. generic 是否对中枢开放：P0 **不对主路径开放**（避免空心专家团）；自动化/存量另策略。  
4. 接续任务标题/审计事件文案：实现时定中文词条。  
5. 编制预检 API 用 GET 还是 POST：按 OpenAPI 既有风格选，无产品差异。

---

## 12. 验收与完成定义

- P0 以 §9 **G1–G13 真实链路**为准（Web + CP + DB；接续含 runtime session 行为；飞书至少打到 connector/CP API 真路径，卡片 UI 若未改版可用 API 级 E2E 声明；automation 用真实规则触发或最小化真实调度验证，不得只跑单测）。  
- `verify:web` / `verify:control-plane` / 契约与迁移校验按仓库惯例。  
- **Temporal replay 测试为硬门禁**（§7.3 触及 coordinator）：`projectcoordination` 的 `replay_test.go` 必须通过；未过则走 §7.3 降级方案，不得强行合入。  
- connector 改动需跑其自身模块测试（`apps/feishu-connector/internal/cards` 等）。  
- 单测/组件测通过 ≠ 完成。  
- 收尾走 `superteam-completion-check`。  
- 延后项（P1/P2、diff 附证、批准物）写入根 `TODO.md` 并回链本文，不得假装已交付。

---

## 13. 文档关系

| 文档 | 关系 |
|---|---|
| 场景模板 P2 契约治理 | 模板 spec/出口/约束/需求级引用的底层；本文消费其模型做**发起与接续产品化** |
| 任务中枢三模式 | 本文修正 plan/loop 发起场字段与 Prompt 模板定位；chat 本体不推翻 |
| 飞书集成 | 本文扩展旁路发起参数与薄选交互；不改审批卡自足原则 |
| 人机批准物 / 执行硬拦 | **未写独立 spec 前以本文 §9 P2 为锚**；开工前另开 spec |

---

## 14. 一句话方案

> **场景模板是可复用的抽象专家团剧本；项目是人与默认的厚容器；plan/loop 新单在中枢与飞书同构地完成「项目 × 剧本 × 收口」装配并预检编制；同单接续继承剧本并 resume 会话；chat 转任务才进入剧本世界；批准物与硬执行是下一层，不阻塞本层落地。**
