# 任务中枢 UI/UX 缺陷整治与发起闭环（Task Hub UX Remediation）

- 日期：2026-08-07
- 状态：**待实施**（本文只定方案，不含实现）
- 系列：独立切片，不属于 IA 四阶段重构；与 `2026-07-26-workflow-ia-restructure-*` 的「流程实例归属」问题**刻意留白**（见 §7）
- 交付性质：Web 前端**单层**改动（文案 / 结构 / 交互 / 数据取数 / 就地闭环）；不改契约、不改数据库、不改 `coordination_mode` 枚举值
  - 例外：§12 的三模式实现审计发现两处后端缺陷，**已在本 spec 之外就地修复并真实 E2E 验证**（未提交），见 §12
- 目标读者：实施会话（本文自包含，含逐项证据行号与验收判据）
- 已拍板（用户 2026-08-07）：
  1. 范围＝批次一（可客观判定的缺陷）＋ 就地闭环；**不动双页签结构**
  2. 模式文案＝中文主 + 英文括注
  3. 「保存草稿」＝直接删除，不做 localStorage、不立项

---

## 1. 背景

一份外部 UI/UX 审查对 `/`（任务中枢）提出 5 条问题与 1 条重构方向。逐条核对代码后：3 条属实、2 条前提不准确、重构方向**建立在错误前提上**。同时代码里另有 4 条审查未覆盖、但严重度更高的缺陷。

本 spec 收拢**可客观判定**的缺陷（违反 CLAUDE.md/DESIGN.md 硬护栏、死交互、假约束、无障碍缺失），加上一条真实的体验断裂（提交后被弹走），不处理需要 IA 级决策的部分。

### 1.1 原审查逐条核实结论

| 原判 | 核实 | 本 spec 处置 |
|---|---|---|
| P1 双 H1 | 属实（且「提出任务」同词三现） | §3.1 收 |
| P1 模式名中英混用 | 属实，但**建议译名不可采用**（见 §3.2） | §3.2 收（改口径） |
| P2 术语堆叠 | **部分属实**（「命令面板」是 `sr-only`，非可见；实为两者堆叠） | §3.3 收 |
| P2 项目选择可扩展性 | **前提不实**（已是 Popover+搜索，非卡片单选）；底下另有真问题 | §5.1 收（改问题定义） |
| P2 保存草稿 | 属实，**且低估**（是死按钮，非"未标明"） | §3.4 收 |
| 双态壳/密度重构 | **部分属实，归因错误**（根因是三个壳参数同变，非密度） | §6 收壳统一；重构前提问题见 §7 留白 |

### 1.2 审查未覆盖、本 spec 纳入的缺陷

- §4.1 模板插入用**串联原生 `window.confirm`**，取消语义嵌套反转 —— 本页最重交互缺陷
- §4.2 `/ 5000` 是**假约束**（无 `maxLength`、无提交校验）
- §4.3 模式卡 `role="radio"` 但**缺方向键导航**；页签 `tablist` 缺 `aria-controls`
- §5.2 无项目时是**死路**（选择器空、无新建入口、提交只报错）

---

## 2. 范围边界

### 做

文案与结构（§3）、交互缺陷（§4）、数据与边界（§5）、壳统一与就地闭环（§6）。全部限于 `apps/web/src/features/task-launches/` + `apps/web/src/lib/status-labels.ts`。

### 明确不做

| 不做项 | 原因 |
|---|---|
| 「流程实例」页签迁出任务中枢 | 需与 IA 四阶段结论对齐，用户已选择本批不动（§7 留白） |
| 需求草稿（前端或服务端） | 用户拍板直接删按钮，不立项、不进 TODO.md |
| 「最近项目」列表 | 原审查建议项；无后端支撑（无 `last_used_at` 维度），本批不做 |
| 后端 content 长度校验 | 本 spec 是纯前端切片；§4.2 有**待确认项**，见 §9 |
| `coordination_mode` 枚举值改名 | 契约值 `plan`/`loop` 保持不变，只改显示层 |

---

## 3. 文案与结构（批 A · 零交互风险）

### 3.1 双 H1 收敛

**现状**：壳 `<h1>` 由 `PageHeader` 渲染（[primitives.tsx:568](../../../apps/web/src/components/layout/primitives.tsx#L568)，`ShellPageHeader` 透传 `variant="shell"`），内容区又有 `<h1 className="tl-title">提出任务</h1>`（[task-launch-form.tsx:161](../../../apps/web/src/features/task-launches/components/task-launch-form.tsx#L161)）。

**改法**：`<h1 className="tl-title">` → `<h2 className="tl-title">`。

**为什么只改标签不改文案**：页签选中态「提出任务」与区块标题同词，是正常的选中呼应，不是冗余。真正违规的只是层级。

**样式安全性**：`.tl-title` 在 [task-launch-aurora.css:33](../../../apps/web/src/features/task-launches/components/task-launch-aurora.css#L33) 是**纯 class 选择器**（非 `h1.tl-title`），换标签不影响视觉。

**判据**：页面 DOM 内 `document.querySelectorAll('h1')` 长度 == 1，且其文本为「任务中枢」。

### 3.2 模式文案中文化 + 强制走词表

**现状**：`"Plan 任务" / "Loop 任务" / "对话"` **硬编码**在 `MODE_CARDS`（[task-launch-form.tsx:40,47,54](../../../apps/web/src/features/task-launches/components/task-launch-form.tsx#L40)）。`status-labels.ts` 中**无对应词条**（已 grep 确认，只有 `coordination_blocked`/`coordination_started` 两个无关事件键）。

**这是护栏违规**，不只是文案偏好：CLAUDE.md 要求用户可见枚举必经 `status-labels.ts`，缺键补词表而非组件内翻译。

**三模式的设计锚点（人类 2026-08-07 澄清）**：这三个模式**就是照着 Claude Code / Codex 的「对话 / plan / loop」设计的**。译名必须回到这个锚点，不能从卡片描述或运行机制反向发明新词。

**实现核对**（权威源：`2026-07-13-task-hub-tri-mode-design.md`，07-14 全链 live E2E 通过；代码复核 2026-08-07）：

| 模式 | Claude Code / Codex 锚点 | 本系统实现 | 代码位置 |
|---|---|---|---|
| plan | plan mode：先出计划，人类确认后再执行 | 上游阻塞时**不自动补链**，把补链提案包成项目决策请求送人类；批准→执行补链+派发，驳回→停在阻塞态 | [workflow.go:851-862](../../../apps/control-plane/internal/workflow/projectcoordination/workflow.go#L851) |
| loop | /loop：自动循环重复推进，直到完成或到达上限 | **有界循环**：自动补链 → `DispatchReasonRetry` 重跑下游 → 再判 → 再补；`defaultMaxPlanIterations = 3` 兜底，**耗尽才转人类**（`requestProjectTaskIterationExhaustedReview`） | [workflow.go:863-878](../../../apps/control-plane/internal/workflow/projectcoordination/workflow.go#L863)（源码内即有中文注释「loop 模式:以下为既有自动补链路径,原样保留」） |
| chat | 普通对话 | 数字员工一次 standalone run（挂项目锚），一问一答 + 可追问，产出默认 quarantined，唯一正式出口是「转为任务」 | tri-mode spec §5 / §13 |

**定名**：`计划任务（Plan）` / `循环任务（Loop）` / `对话（Chat）`。

**被否决的译名及原因**：
- 原审查建议「闭环补做」——「闭环」不是 loop 的锚点。
- 本 spec 初稿写过「自动补做模式」——**同样错误**。它是从 `isAutonomousCoordinationMode` 反推的（loop/chat 属 autonomous、可免人类确认自动派发计划），把一个**次要分流点**当成了定义。锚点是 Claude Code 的 /loop，即「循环」。

**改法**：

1. `status-labels.ts` 新增（注意函数名用 `launchMode` 而非 `coordinationMode`：`LaunchMode` 是 UI 三值 `plan|loop|chat`，而契约 `coordination_mode` 只有 `plan|loop`，`chat` 不映射到它）：

   ```
   LAUNCH_MODE_LABELS: { plan: "计划任务（Plan）", loop: "循环任务（Loop）", chat: "对话（Chat）" }
   export function launchModeLabel(mode: string | undefined): string
   ```

2. `MODE_CARDS[].label` 改为从 `launchModeLabel(value)` 取值。

**判据**：组件内不再出现模式名字面量；`status-labels.guard.test.ts` 保持绿；切模式后卡片名为中文主 + 英文括注。

### 3.2.1 模式卡描述文案：**核对后不改**（本 spec 初稿此处判断错误，已撤回）

本 spec 初稿曾主张 `MODE_CARDS[].desc` 的 plan/loop 两句「只描述遇上游阻塞时这一个分支，把特例讲成了全部」，要求改写。**该判断经代码核对不成立，撤回。**

现状三句（[task-launch-form.tsx:38,46,53](../../../apps/web/src/features/task-launches/components/task-launch-form.tsx#L38)）：

- plan：「遇上游阻塞时暂停，提案报你决策后再补做」
- loop：「遇上游阻塞时自动补做上游任务并重跑下游」
- chat：「与指定数字员工单次对话，结果不进入项目流转」

**为什么准确**：`blocked_resolvable_upstream` **就是** plan/loop 的唯一分流点，不是"其中一个分支"。`workflow.go:846` 的 `if` 之外，两模式走完全相同的路径；`CoordinationMode` 在该分支之外只被传入 `PersistPlanRevision`。tri-mode spec §8.3 亦明写两模式的差异定义在这一处。三句描述**如实反映实现**，不构成假约束。

**实施须知**：改 §3.2 的 label 时**不要顺手改 desc**。若实施中觉得 loop 一句可补「到达迭代上限（默认 3 轮）后转人类」以更完整，属**可选增补**，需与 `defaultMaxPlanIterations` 的实际取值及项目级 `coordination_policy` 覆盖保持一致；不做也不算缺陷。

### 3.3 术语堆叠收敛

**现状**：`<span>中枢指令区</span>`（[:199](../../../apps/web/src/features/task-launches/components/task-launch-form.tsx#L199)）+ `<span className="tl-pill">命令中心</span>`（[:202](../../../apps/web/src/features/task-launches/components/task-launch-form.tsx#L202)）同屏。

**改法**：
- 「中枢指令区」→「需求描述」
- 删除「命令中心」pill

**为什么是「需求描述」而非其他措辞**：同文件内部**已经在用这个词**——textarea 的 `aria-label="需求描述"`（[:205](../../../apps/web/src/features/task-launches/components/task-launch-form.tsx#L205)），校验错误文案「需求描述不能为空」（[:121](../../../apps/web/src/features/task-launches/components/task-launch-form.tsx#L121)）。当前是**可见标签与无障碍标签、错误提示三者不一致**，改后三处归一。

**注意**：原审查称「顶栏 ⌘K 命令面板」构成第三重堆叠，**不成立**——`command.tsx:44` 的 `DialogHeader` 带 `className="sr-only"`，视觉不可见；顶栏可见文案是「搜索任务、数字员工…」+ `⌘K`。此项无需改动。

### 3.4 删除「保存草稿」死按钮

**现状**：[:244-247](../../../apps/web/src/features/task-launches/components/task-launch-form.tsx#L244) 的 `<button className="tl-btn-draft">` **无 `onClick`**、组件内无草稿 state、无 API 调用。全仓无草稿列表入口。

**改法**：删除该 `<button>` 整块，及其 `PencilLine` import（若无他用）。`.tl-btn-draft` CSS 规则一并清除。「提交任务」独占 `.tl-actions`（右对齐或全宽由实施时按视觉判断）。

**判据**：页面无「保存草稿」文本；`rg "tl-btn-draft"` 零结果。

---

## 4. 交互缺陷（批 B）

### 4.1 模板插入：串联 `window.confirm` → 单个三选一对话框

**现状**（[:143-156](../../../apps/web/src/features/task-launches/components/task-launch-form.tsx#L143)）：内容非空时先 `confirm("覆盖当前内容？")`，点**取消**后**又**弹 `confirm("是否追加到末尾？")`，**再取消**才放弃。取消语义嵌套反转，且原生弹窗与产品视觉完全脱节。

**改法**：改为项目内已有的 `Dialog` 组件，一次呈现三个动作：**覆盖 / 追加到末尾 / 取消**。仅在 `content.trim()` 非空时弹出；为空时保持现有直接插入行为不变。`applyPromptTemplate` 的埋点调用在「覆盖」与「追加」两分支都保留，「取消」不调用。

**判据**：非空内容下插入模板，只出现一次对话框；三个动作各自行为正确；`window.confirm` 在本 feature 目录零残留。

### 4.2 `/ 5000` 假约束

**现状**：`<span className="tl-counter">{content.length} / 5000</span>`（[:220](../../../apps/web/src/features/task-launches/components/task-launch-form.tsx#L220)），但 textarea 无 `maxLength`，`handleSubmit`（[:116-141](../../../apps/web/src/features/task-launches/components/task-launch-form.tsx#L116)）也不校验长度。

**改法（两选一，取决于 §9 待确认项 U1 的核查结果）**：
- **若后端确有上限**：textarea 加 `maxLength`，值与后端一致；计数在 ≥90% 时变警示色。
- **若后端无上限**（当前 grep 未在契约与 `internal/project/` 中找到校验）：**删除 `/ 5000`**，只保留字数计数 `{content.length} 字`。**不得**保留一个前端自造、无处强制的数字。

**判据**：展示的上限数字必须与实际强制点一致；不存在「显示上限但可超出」的状态。

### 4.3 无障碍

**现状**：
- 模式卡：`role="radiogroup"` + 三个 `<button role="radio">`（[:167-179](../../../apps/web/src/features/task-launches/components/task-launch-form.tsx#L167)），三个按钮均可 Tab 聚焦，**无方向键切换**——违反 radiogroup 的键盘契约。
- `ProjectPicker` 内列表同样是 `role="radiogroup"` + `role="radio"` 按钮（[:323-345](../../../apps/web/src/features/task-launches/components/task-launch-form.tsx#L323)），同问题。
- 页签 `PageTabs role="tablist"`（[index.tsx:46](../../../apps/web/src/features/task-launches/index.tsx#L46)）缺 `aria-controls` / 对应 `role="tabpanel"` + `id`。

**改法**：
1. 模式卡实现 roving tabindex：选中项 `tabIndex=0`，其余 `tabIndex=-1`；`ArrowLeft`/`ArrowRight`（及 `ArrowUp`/`ArrowDown`）在三项间循环移动并同步选中。
2. `ProjectPicker` 列表同法（`ArrowUp`/`ArrowDown`），`Enter` 选中，`Esc` 关闭 Popover。
3. 两个页签补 `id` + `aria-controls`，各自内容容器补 `role="tabpanel"` + `aria-labelledby`。

**判据**：键盘可完成「选模式 → 选项目 → 填写 → 提交」全流程，不依赖鼠标；焦点顺序不出现三连 Tab 停在同一 radiogroup 内。

---

## 5. 数据与边界（批 C）

### 5.1 项目搜索改服务端（**修正原审查的问题定义**）

**原审查说**「项目多时卡片单选会拥挤，保留搜索」——前提不实：已是 Popover + 搜索框（[:301-322](../../../apps/web/src/features/task-launches/components/task-launch-form.tsx#L301)），不是卡片单选。

**真问题**：
- `listProjects(apiOptions, { limit: 50, offset: 0 })`（[index.tsx:165](../../../apps/web/src/features/task-launches/index.tsx#L165)）只取前 50 条；
- 搜索是 `filtered` 客户端过滤（[:283-291](../../../apps/web/src/features/task-launches/components/task-launch-form.tsx#L283)），**只能在这 50 条内搜**；
- 项目超 50 个时，搜一个存在但不在前 50 的项目，会显示「无匹配项目」（[:347](../../../apps/web/src/features/task-launches/components/task-launch-form.tsx#L347)）——**把"没搜到"谎报成"不存在"**；
- 归档项目在客户端 filter（[index.tsx:169](../../../apps/web/src/features/task-launches/index.tsx#L169)、[form:89-92](../../../apps/web/src/features/task-launches/components/task-launch-form.tsx#L89)），进一步压缩这 50 条里的可用数量。

**好消息（本 spec 成本远低于预估）**：服务端与前端 API 层**已完全支持**服务端搜索与状态过滤，只是本页没用：
- 契约：`GET /api/v1/projects` 已有 `q`、`status`、`limit`、`offset`（[openapi.yaml:886-898](../../../contracts/control-plane/openapi.yaml#L886)）
- 前端：`ListProjectsFilters` 已含 `q`/`status`，`projectListPath` 已拼参（[projects.ts:1264-1321](../../../apps/web/src/lib/api/projects.ts#L1264)）

**改法**：
1. 项目查询改为服务端过滤：传 `status`（取活跃口径）而非客户端 filter archived。
2. `ProjectPicker` 的搜索词提升为查询输入：输入防抖（建议 250–300ms）后带 `q` 重新请求，`placeholderData: keepPreviousData` 避免列表闪烁。
3. 空结果文案区分两态：**关键词非空**→「无匹配项目」；**关键词为空且结果为 0**→ 走 §5.2 空态。
4. **注意复用面**：`ProjectPicker` 同时被 `chat-panel.tsx` 引用（`import { LaunchChip, ProjectPicker } from "./task-launch-form"`）。改造需保持对外 props 兼容，或同步调整 chat 侧调用点——实施前先确认两处行为一致。

**判据**：造 >50 个项目，搜索第 51+ 个项目的名称，能搜到并可选中（此项必须真实链路验证，见 §8）。

### 5.2 零项目死路

**现状**：`activeProjects` 为空时 `projectId=""`（[:100](../../../apps/web/src/features/task-launches/components/task-launch-form.tsx#L100)），提交报「请选择项目」（[:124-127](../../../apps/web/src/features/task-launches/components/task-launch-form.tsx#L124)），但选择器里空无一物，页面无任何出路。

**改法**：项目列表为空（且非加载中、非搜索态）时，`ProjectPicker` 位置渲染空态：一句说明 + 指向 `/projects` 的**新建项目**入口。同时禁用「提交任务」并给出原因，而不是让用户点了才知道。

**约束**：跳转必须用 TanStack Router 的 `Link`/`navigate`（CLAUDE.md 硬规则），不得用 `<a href>`。

**判据**：零项目租户下页面给出可点击的下一步，不出现"能填能点但永远提交不了"的状态。

---

## 6. 壳统一与就地闭环（批 D）

### 6.1 双页签壳统一（**修正原审查的归因**）

**原审查归因**「跟踪信息密度远高于发起 → 应同壳不同 body 密度」。**实际根因不是密度**，是两个分支各自搭壳、三个壳参数同时变：

| 壳参数 | 提出任务 | 流程实例 |
|---|---|---|
| 容器 | `TaskLaunchShell` → `Main width="canvas" className="tl-aurora p-0"`（[shell:29](../../../apps/web/src/features/task-launches/components/task-launch-shell.tsx#L29)） | 直接 `Main width="wide"`（[index.tsx:107](../../../apps/web/src/features/task-launches/index.tsx#L107)） |
| 背景 | `tl-aurora` 极光 | 无 |
| 副标题 | 「提交需求到项目，由项目协调线程编排后续任务」（[index.tsx:245](../../../apps/web/src/features/task-launches/index.tsx#L245)） | 「跨项目监控流程实例的运行、阻塞与结果」（[index.tsx:104](../../../apps/web/src/features/task-launches/index.tsx#L104)） |

**改法**：
1. 两分支统一走 `TaskLaunchShell`，`width` 提为该组件的 prop（compose 用 `canvas`、instances 用 `wide`——宽度差异是**内容的真实需求**，保留）。
2. `tl-aurora` 背景：两态一致。二选一由实施时按视觉定，但**必须一致**，不能切页签时整块背景消失。
3. 壳副标题改为**一句覆盖双态**的中性描述（例：「提交需求并跟踪流程实例的运行与阻塞」），不再随页签换整句。页签自身承担"当前在哪"的表达。
4. 页签条渲染位置与间距在两态一致（当前 compose 在 `max-w-[940px]` 居中容器内、instances 在 `flex-col gap-5` 里，视觉起点不同）。

**判据**：来回切页签，H1、副标题、背景、页签条位置**不发生跳变**；只有页签选中态与下方内容变化。

### 6.2 提交后就地闭环（**本批唯一的体验型改动**）

**现状**：提交成功后 `navigate` 直接弹到 `/projects/$projectId?tab=demands&demand=<id>`（[index.tsx:212-219](../../../apps/web/src/features/task-launches/index.tsx#L212)）。用户被移出任务中枢，且没有任何成功反馈——页面直接换了。连续提交多个需求要每次重新导航回来。

**改法**：
1. 提交成功后**留在本页**，重置输入区，在其位置渲染**结果确认块**：需求标题、所属项目名、模式，以及两个动作——
   - 「查看需求卷宗」→ `Link` 到原深链 `/projects/$projectId?tab=demands&demand=<id>`（原跳转降级为**显式动作**，不丢失）
   - 「再提一个」→ 清空结果块，回到输入态（保留已选项目与模式）
2. 同时发一条 `toast.success`（项目已装配 `sonner`，见 [main.tsx:9](../../../apps/web/src/main.tsx#L9)）。
3. 失败态：`submitMutation` 出错时在表单内联展示错误（复用现有 `.tl-err`），不静默。

**为什么这是"闭环"而非"少一次跳转"**：当前「提出任务」页签提交后即弃用本页，而「流程实例」页签跟踪的是 **workflow instances**——与刚提交的 **demand** 不是同一类对象。所以页面标称的"发起 + 跟踪"从未成立。就地结果块是**在不动 IA 的前提下**，让"我提交了什么、去哪看"这条最短链路先闭上。

**判据**：提交成功后 URL 不变、页面仍在任务中枢、结果块含可点深链；点「再提一个」可连续提交第二条。

---

## 7. 刻意留白：双页签的 IA 归属

原审查的重构方向（「同一壳、不同 body 密度」）建立在**"这页是发起 + 跟踪双态"**的前提上。该前提不成立：

- 「提出任务」产出的是 **demand**，其跟踪落点在项目卷宗（`/projects/$projectId?tab=demands`）
- 「流程实例」跟踪的是 **workflow instance**，与 demand 不同源

所以真问题不是"怎么让两个页签视觉统一"，而是**"这两个页签为什么在同一个页面里"**。

本批按用户拍板**不动**，但记录于此：后续若重启 IA 讨论，需先回答归属问题，再谈视觉。§6.1 的壳统一是**止血**（消除跳变），不构成对该 IA 结构的认可。

---

## 8. 验证方案

按 CLAUDE.md「真实端到端验证是默认完成条件」，本 spec 含数据取数与提交链路改动，**不适用轻量验证例外**。

### 8.1 分层门禁（提交前）

- `corepack pnpm verify:web`
- 若触碰设计系统 token/原型：`verify:design-system` / `verify:design-prototypes`

### 8.2 受影响的既有测试（改文案必红，需同步更新）

| 文件:行 | 断言 | 受哪项影响 |
|---|---|---|
| [index.test.tsx:515](../../../apps/web/src/features/task-launches/index.test.tsx#L515) | `clickButton("Loop 任务")` | §3.2 |
| [index.test.tsx:717](../../../apps/web/src/features/task-launches/index.test.tsx#L717) | `clickButton("Plan 任务")` | §3.2 |
| [index.test.tsx:533,537](../../../apps/web/src/features/task-launches/index.test.tsx#L533) | `getByText("中枢指令区")` | §3.3 |
| [index.test.tsx:538](../../../apps/web/src/features/task-launches/index.test.tsx#L538) | `getByText("命令中心")` | §3.3 |
| [index.test.tsx:539](../../../apps/web/src/features/task-launches/index.test.tsx#L539) | `getByText("保存草稿")` | §3.4 |
| [index.test.tsx:535](../../../apps/web/src/features/task-launches/index.test.tsx#L535) | `getByText("提出任务")` | §3.1（注意：`TaskLaunchView` 直渲时无页签，改 h2 后仍单点匹配；若实施中让页签一并进入该测试，此断言会因多元素匹配抛错） |
| [workflow-instances.test.tsx:294](../../../apps/web/src/features/task-launches/workflow-instances.test.tsx#L294) | 页签 `aria-selected` | §4.3（补 `aria-controls` 不应影响，回归确认） |

**另需新增测试**：模板三选一对话框（§4.1）、模式卡方向键（§4.3）、零项目空态（§5.2）、提交后就地结果块（§6.2）。

### 8.3 真实端到端（必做，浏览器 + 真实 CP + 真库）

前置：`scripts/dev-services.sh status` 确认服务在跑且已加载当前代码（改动后 `restart web`）。

| # | 场景 | 通过判据 |
|---|---|---|
| G1 | 打开 `/`，检查 DOM | `h1` 唯一且为「任务中枢」；无「保存草稿」；无「命令中心」；指令区标签为「需求描述」 |
| G2 | 三个模式卡 | 名称为「计划任务（Plan）/ 循环任务（Loop）/ 对话（Chat）」；三句 desc **原样未改**（§3.2.1）；键盘方向键可切换 |
| G3 | 项目 >50 时搜索第 51+ 个项目 | 能搜到并选中（**需真实造够项目数**，不得用 mock 断言） |
| G4 | 内容非空时插入模板 | 只弹一次对话框；覆盖 / 追加 / 取消三分支行为正确 |
| G5 | 真实提交一条需求 | 停留本页、出现结果块与 toast、深链可点进卷宗且能看到该需求；点「再提一个」可连续提交第二条 |
| G6 | 来回切两个页签 | H1/副标题/背景/页签条位置无跳变 |
| G7 | 零项目租户（或临时归档全部项目） | 出现空态与新建入口，提交按钮禁用且给出原因 |
| G8 | 键盘全流程 | 不用鼠标完成选模式→选项目→填写→提交 |

**阻塞处置**：G3 需要 >50 个真实项目，若造数受限，**标记为阻塞并说明**，不得以"单测通过"替代。

### 8.4 收尾门禁

按 CLAUDE.md，任务收尾前必须走 `.codex/skills/superteam-completion-check/SKILL.md`（Claude Code 会话直接 `Read` 该文件并照步骤执行）。

---

## 9. 待确认项（实施前需核实或与人类确认）

| # | 事项 | 影响 | 建议处置 |
|---|---|---|---|
| **U1** | 后端对需求 `content` 是否真有长度上限？本次 grep 在 `contracts/control-plane/openapi.yaml` 的 `SubmitProjectDemand` schema 与 `apps/control-plane/internal/project/` 中**均未找到** `maxLength` / 长度校验，但**未穷举** handler 与中间件 | 决定 §4.2 走"对齐后端"还是"删掉假数字" | 实施首步先核实；查不到即按**删除 `/ 5000`** 执行 |
| **U2** | `tl-aurora` 极光背景是否扩展到「流程实例」态 | §6.1 的视觉一致性方案 | 实施时出两版截图给人类选；默认取**弱化后两态一致** |
| **U3** | `ProjectPicker` 改服务端搜索后，`chat-panel.tsx` 的调用点是否需要同等改造 | 复用面一致性 | 实施前对比两处需求；若 chat 侧也受 50 条限制，同批修 |
| **U4** | 「提交任务」删除草稿按钮后独占操作区的视觉处置（右对齐 / 全宽 / 居中） | 纯视觉 | 按 DESIGN.md 与相邻页面惯例定，无需回来问 |

---

## 10. 实施顺序建议

严格按批次推进，每批可独立提交、独立回滚：

1. **批 A（§3）** —— 文案与结构，零交互风险。同批修 §8.2 的既有测试。
2. **批 B（§4）** —— 交互缺陷。§4.2 先解 U1。
3. **批 C（§5）** —— 数据与边界。注意 §5.1 的 `chat-panel` 复用面（U3）。
4. **批 D（§6）** —— 壳统一与就地闭环。改动面最大，放最后，便于前三批的验证结果不被污染。

批 A–C 之间无依赖，可乱序；批 D 依赖批 A（壳内文案已定）。

---

## 11. 风险

| 风险 | 等级 | 缓解 |
|---|---|---|
| §6.2 就地闭环改变了用户已形成的肌肉记忆（提交即跳项目页） | 中 | 深链保留为显式动作，不丢失原路径；toast 明确告知已提交 |
| §5.1 服务端搜索改造波及 `chat-panel` 复用点 | 中 | U3 实施前确认；保持 props 兼容优先 |
| §4.3 roving tabindex 实现不当会破坏现有点击选中 | 低 | 新增键盘路径，不改 `onClick` 分支；测试覆盖两种输入方式 |
| §3.2 词表新增键与既有 `coordination_*` 事件键混淆 | 低 | 函数命名 `launchModeLabel`（非 `coordinationModeLabel`），注释写明 UI 三值 vs 契约两值的差别 |

---

## 12. 三模式实现审计（2026-08-07，人类要求）

在为 §3.2 定名而核对实现时，人类要求进一步审计「代码层是否有地方破坏了 plan / loop / chat 这三个定义」。逐层核对结果如下。

### 12.1 loop 层：完好

有界循环三要素齐备：`createUpstreamSupplementTasks` 自动补链 → `DispatchReasonRetry` 重跑下游 → 再判 → 再补；`defaultMaxPlanIterations = 3` 兜底，耗尽转人类（`requestProjectTaskIterationExhaustedReview`）。上限可被 `projects.coordination_policy` 覆盖。无破坏。

### 12.2 F2（plan 层）：模式解析 fail-open —— **已修复**

**问题**：`InspectTaskResultDecision` 里，`GetPlanRevision` 的错误被 `err == nil &&` 短路吞掉，静默落 `loop`。后果是**读取失败时，plan 模式需求会走自动补链 + 自动重跑下游，绕过人类闸且零留痕**。人类门禁必须 fail-closed，这里方向反了。

`tri-mode spec §8.4` 只授权了「存量 plan revision **无 mode 值** → 按 loop 解释」（存量兼容），**没有**授权「读取失败 → 按 loop」；两件事被同一个 `if` 混在一起。

**修复方向的选择**：起初改为直接抛错，被否——错误会让 Temporal 无限重试该 activity，协调线程静默卡死，而目前**没有 coordinator 存活告警**（属待立项项）。最终按 spec §8.4 自己的原则处置：

> 新提交需求缺省 `plan`。理由：与"人类决策一等对象"一致；**plan 缺省误报最坏多问一次，loop 缺省误判最坏烧预算跑歪图**。

读取失败正是"不确定"，故 **fail-closed 落 plan**：既安全（人类闸生效）又保活（人类在收件箱看到决策请求，是可见可操作信号）。两个"不知道模式"的分支现已分离：

- revision 读到但 mode 为 nil → loop（§8.4 存量兼容，保留）
- revision 读不到 → plan（新，fail-closed）

**改动**：`project_store.go` `InspectTaskResultDecision`。既有测试 `TestInspectTaskResultDecisionSwallowsPlanRevisionLookupError`（**测试名直接写着 Swallows，吞错是被写进测试的**，但无任何注释说明为何安全）已改名为 `...FailsClosedToPlanOnRevisionLookupError` 并补充立意注释。

### 12.3 F3（chat 层）：项目级聚合漏算 chat —— **已修复**

**问题**：`ListProjectRunSummaries` 与 `CountTaskRunsCompletedToday` 统计「今日完成运行」时**不过滤 `run_kind`**，全 `project.sql` 里 `run_kind` 零出现。chat run 自 tri-mode spec §13 改为挂项目锚后 `project_id` 非空，跑完即 `completed`，于是**与数字员工的闲聊被计入锚项目的业务「今日完成」与大屏 KPI**，违反 §5 不变量 2「chat 产出默认无业务效力」。

**根因**：run-project-affiliation 重构把读路径从「经 `project_tasks` 到达 run」切成「直接用 `task_runs.project_id` 一等列」。前者天然不含 chat（chat 无 ProjectTask，不变量 1），后者丢了这个隐式过滤。同文件的 `ListProjectDeleteRunBlockers` 仍走 `INNER JOIN project_tasks`，**未受影响**——可作对照佐证该归因。

**改动**：两个查询补 `JOIN tasks` + `run_kind <> 'chat'`（`run_kind` 在 `tasks` 表，迁移 059；`task_runs.task_id` 非空，INNER JOIN 安全）。**无需迁移**。sqlc 重生成，签名未变。

**审计边界**：已确认无其他泄漏点——`GetProjectTaskRunRuntimeNodeID`、`ListProjectDeleteRunBlockers`、`StartProjectTaskAttempt` 均经 `project_tasks`；`employee_working_task` 取自 `project_tasks`。`employee_execution.sql` 的员工级 latest_run 会因 chat 显示"忙"，属**正确行为**（chat 确实占用该员工），不在不变量约束内。

### 12.4 验证记录（真实链路，2026-08-07）

分层门禁：`corepack pnpm verify:control-plane` 全绿。

真实 E2E（真库 + 真 CP + 真 runtime + claude-code provider）：

| 步骤 | 结果 |
|---|---|
| 历史数据直证（08-06 窗口，新旧口径对比） | 三个项目「今日完成」显示 **3 / 2 / 2**，真实业务完成数实为 **0 / 0 / 0**——全部是 chat 泄漏；第四个项目（真实 task run）1→1 不受影响，证明过滤未误伤 |
| 基线 | 今日 5 条完成运行全为 task；API 项目计数 5、KPI 5 |
| 造真实 chat run | `POST /digital-employees/{id}/runs`（`run_kind=chat` + 项目锚）→ 201 → claude-code 真实执行 → `completed` |
| 库内确认 | 今日窗口 `chat=1, task=5` |
| **修复后真实 API** | 项目「今日完成」**仍为 5**、KPI **仍为 5** —— chat 未被计入 |
| 反证 | 同一时刻直查：旧口径 **6**、新口径 **5**；实时 API 返回 5，与新口径一致 |

**F2 的验证边界（如实记录）**：F2 由单元测试覆盖（`...FailsClosedToPlanOnRevisionLookupError` 及既有模式解析表测试），**其 fail-closed 分支未经真实工作流走通**——触发它需要构造「任务指向一个读不到的 plan revision」的损坏数据态，成本与风险都不成比例。非错误路径（正常 plan/loop 模式解析）由既有协调测试覆盖且未改变。

### 12.5 F1（plan 层）：plan 模式确认闸被有条件绕过 —— **人类已裁决并实施（2026-08-07）**

`PersistPlanRevision` 走 plan 分支时调 `planRequiresHumanConfirmation`。当以下**全部**成立时，plan 模式的计划直接置 `Accepted` **自动派发，不报人类**：

- validation 未标 review required，且 route decision 未要求 human review
- 不触碰高风险信号（`planTouchesHighRisk`）
- 无 `human_judgment` 判据
- 所选出口不是最深的那个（`planExitAtOrBeyondConfirmationDepth` 为 false）

源码注释自陈：

> Plan mode **used to require human confirmation unconditionally**; it now scales with exit depth — shallow, non-high-risk, no-human-criterion exits **may auto-dispatch**.

这与「plan = 出计划 → 人类确认 → 执行」的锚点直接冲突。但它**不是代码走偏**，而是 autonomy posture calibration Phase A Task 2 的有意设计（已拍板并合并，07-16 `5e3d7b1a`）。

即：**两个都已拍板的设计互相冲突**，不能由实施会话单方面判定谁对。经陈述后人类选定 **A（plan 锚点优先）**。

**实测频次（裁决依据）**：库内 19 份 plan 模式计划中，**15 份停下等确认、4 份直接跑了**。不是理论风险，已在发生。

#### 裁决理由（人类 2026-08-07）

> 很难判定哪些是高风险、哪些是低风险。这应该由人类来决策，不应该由 AI 来决策。

补充两条支撑该判断的代码事实：

1. **风控判据是 AI 给自己打的分**。`planTouchesHighRisk` 读的 `RequiresHumanReview` / `RequiresHumanApproval` / `RiskLevel` 全部由 planner（LLM）在生成计划时自填，即"AI 写完计划，再由 AI 判断这计划要不要给人看"，是循环论证。最需要人看的计划恰是模型没识别出风险的那些，而它们必然自报低风险。
2. **自治已有显式开关，就是 loop 模式**。plan 再隐式自动派发，等于存在两套自治机制——一套人选（loop）、一套 AI 选（跳过确认）。砍掉隐式那套，自治与否重新由人在提交时决定；这不是牺牲效率换安全，是把开关交回人手里。

另注：Phase A 的 **Task 4**（把出口深度升级为模板声明的阶段卡点）**从未实施**，所以"最深一档才停"这个自陈的保守回退（源码注释：*a conservative fallback ahead of Task 4*）实际上是永久状态，从未被精化。

#### 实施内容

- `PersistPlanRevision`：删除 plan 模式的自动派发分支。plan 模式现在只可能落 `ValidationFailed` 或 `PendingReview`。
- 删除 `planRequiresHumanConfirmation`、`planExitAtOrBeyondConfirmationDepth` 两个函数。
- **`planTouchesHighRisk` 保留**——它在 `acceptance_criteria.go` 另有用途（为高风险计划注入宪法级 human_judgment 判据，与模式无关），删除会误伤。
- 测试 `TestShallowExitPlanModeAutoDispatches` **反转**为 `TestShallowExitPlanModeStillHoldsForConfirm`（而非删除），把新不变量永久钉住，防止提速逻辑被无声重新引入。
- 回写 `docs/superpowers/plans/2026-07-16-autonomy-posture-phase-a.md`：标注 Task 2 已撤销、Task 4 针对确认闸的部分已无对象、**Task 5 Step 1 判据已反转**（低风险浅出口现在必然产生 plan_review 卡；照原文复跑该 GATE 会误判为回归）。

#### 验证（真实链路）

`verify:control-plane` 全绿。真实 E2E 用 `software_delivery` 模板（声明 3 出口）提交需求，引导选中最浅出口：

| 字段 | 值 | 意义 |
|---|---|---|
| 选中出口 | `branch_ref` | 3 个声明出口中最浅 |
| 出口数 | 3 | 深度已知，不触发"未知则保守"兜底 |
| review_required | false | 无风险触发器点火 |
| review_reason | 无 | 无其他扣留理由 |
| **status** | **pending_review** | **仍然停下** |

这正是撤销前会置 `Accepted` 直接派发的形状。收件箱确认出现 `plan_review` 卡（`open`），闭环成立。

**验证方法学教训**：首次验证提交的需求**未声明出口**（出口数 0），旧代码遇"深度未知"本就会停，**不构成判别性证据**；必须用多出口模板 + 浅出口才能验到差异。记录于此以免复验时重蹈。

#### 三模式定义现状

本轮之后，三个模式在代码层自洽：

- **plan** — 出计划 → 必停 → 人类确认 → 才执行。无例外。
- **loop** — 自动循环推进，上限 `defaultMaxPlanIterations = 3`，耗尽转人类。
- **chat** — 单次对话，不进项目流转（§12.3 修复后真正不进）。

自治与否只剩一个开关：提交时选 plan 还是 loop，由人握。
