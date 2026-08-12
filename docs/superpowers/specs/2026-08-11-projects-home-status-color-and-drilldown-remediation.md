# 项目管理首页整改方案：生命周期色单一事实源 · 关注项精确下钻 · 卡片密度

- 日期：2026-08-11
- 状态：**已实施，未验收**（2026-08-12：P1a–P4 代码落地并经复查+补测）
  - **通过**：web `src/features/{projects,inbox}` + `components/superteam` + `lib` 共 561 测试绿；`tsc -b` 绿。处方 A 经 grep 确认 `projectStatusTone`/`projectStatusIconTone` 全仓零残留、环图硬编码 hex 清除；单任务 GET 的 404 语义追到 `projectRepositoryError`（`pgx.ErrNoRows → ErrProjectNotFound → 404`）成立。
  - **复查补的测试**（原实施只动了 3 个测试文件，处方 B 行为零覆盖）：`project-task-detail-deeplink.test.tsx`（缺页回落 5 例：窗外单查取回 / 窗内不发请求 / 404 / 500 / 无 apiOptions，三种失败态不许互相冒充）、`project-triage-deeplink.test.tsx`（6 类 reason 的 search 构造，含证据落点从 `assets` 纠到 `workbench`）、`status-pill-dot.guard.test.ts`（扫 `<StatusPill>` 开标签拦 `dotClassName` 偷渡语义色）。
  - **复查修的小问题**：证据滚动两处重复（改为容器级归 operational-detail、行级归 evidence panel）；弹层 `!apiOptions` 分支原文案谎称"任务不存在"（改为"无法单查"）；`primitives.test.tsx` 一条自证断言换成真断言。
  - **仍未做（不得据此声称完成）**：
    1. **G3 未跑**——深链到第 21 条以后失败任务的真实链路验证。组件测试用 mock fetcher 钉住了回落逻辑，但**没有证明真实 CP 在真实分页窗口外确实返回该任务**。配方见 §11.6。
    2. **`verify:contracts` 红**——报 `WorkspaceDeleteRequest` 缺失，属**另一工作流**（2026-08-12 工作区供给）的 schema；本方案的 `getProjectTask` 已在生成物中（`control_plane.gen.go:10191`）。跨流生成物碰撞，需与在途那位对齐后再 regenerate，勿单方面执行。
    3. `go test ./internal/project/` 3 例失败（`TestFailProjectTaskAttemptTransientRuntimeSchedulesRetry` 等，`waiting_human` vs `queued`），属另一流的 max_attempts/人工恢复改造，**与本方案无关**，但意味着该包门禁当前不绿。
- 上游前置：
  - `docs/superpowers/specs/2026-08-11-project-portfolio-layered-status-technical-design.md`（**在途未提交**，本方案是它的后继整改，§8 有强制顺序约束）
  - `docs/superpowers/specs/2026-08-10-projects-home-portfolio-hygiene-design.md`（组合职责与 triage 面板的当前形态由它确立）
  - `DESIGN.md`「工作对象界面规则」「面向用户文本与枚举显示」；`docs/design-system/tokens.md`、`data-display.md`、`a11y-and-dark.md`
- 人类拍板记录（2026-08-11）：
  1. 阶段色 → **新增 `--phase-*` token 族**（不复用语义色当分类色）
  2. 下钻落点 → **项目内那一条**，收件箱作次级出口
  3. 页长 → **密度切换 + 默认页大小 12→9**

---

## 1. 触发

人类走查项目管理首页后给出三点观察（原话要点）：

1. 卡片状态「已就绪」「验收中」以及其他状态**看起来都是同一个颜色**，没有做区分；
2. 按当前卡片大小，默认 12 条会**把首页撑得很长**；
3. 点卡片右侧展开后可以看失败任务、处理决策，但**点击只跳到一个列表**——跳到项目内的「任务」tab 或「决策资产」tab。既然点的是某个**具体问题点**，就应该跳到那个具体问题：直接打开对应的任务条目，或者跳到收件箱的对应条目。

三点里第 3 点是缺陷（携带的 id 被静默丢弃），第 1 点是设计债（四套映射漂移），第 2 点是表现层取舍。

---

## 2. 现状核查（as-is，逐条带代码坐标）

### 2.1 状态色：同一「项目状态」有四套互相漂移的映射

| # | 位置 | 代码坐标 | 已就绪 `running` | 验收中 `acceptance` |
|---|---|---|---|---|
| 1 | 卡片图标底（IconTile） | `project-portfolio-grid.tsx:109` `projectStatusIconTone` | `bg-brand-soft` | `bg-info-soft` |
| 2 | 顶栏项目层环图 | `project-risk-home.tsx:47` `PROJECT_LAYER_COLORS` | `var(--brand)` | `var(--info)` |
| 3 | 右栏 triage pill | `project-risk-home.tsx:488` `projectStatusTone` | `ok`（绿） | `brand`（蓝） |
| 4 | 项目详情页 pill | `project-operational-detail.tsx:2237` `projectStatusTone` | `ok`（绿） | **`warn`（黄）** |
| 5 | **卡片生命周期 pill** | `project-portfolio-grid.tsx:488` | **写死 `tone="mute"`** | **写死 `tone="mute"`** |

事实判定：

- #3 与 #4 是**同名重复实现且已经漂移**（`acceptance` 一个 `brand` 一个 `warn`）。人类看到的"全灰"是 #5，但只补 #5 会造出第五套。
- 同一个「验收中」项目，用户从环图（浅青）→ 卡片（灰）→ 右栏（蓝）→ 详情页（黄）一路走下来会看到四种颜色。
- #2 的 `draft`/`archived` 用了硬编码 `#94a3b8` / `#cbd5e1`，违反 `a11y-and-dark.md`「不得硬编码浅色」，且暗色端无对应值。
- #1 让**类别图标按状态换色**，与 `DESIGN.md`「状态色和类别色分家：类别/装饰图标（IconTile 等）默认 mute 或 brand-soft，不按实体类型换色」直接冲突。

顺带清点：全仓 `*Tone(...): Tone` 形态的散装映射函数约 25 处（`grep -rn "): Tone {"`）。本方案**只收敛项目生命周期这一根轴**，不做全站 tone 治理（见 §12）。

### 2.2 下钻：携带的 id 被静默丢弃

链路（右栏 triage「查看失败任务」为例）：

```
ProjectTriageReasonRow            project-risk-home.tsx:467
  → <Link search={{ tab:"tasks", task:<taskId> }}>
      ↓
features/projects/index.tsx:1390  traceTaskId={search.task}
      ↓
project-operational-detail.tsx:594  <ProjectExecutionTracePanel focusTaskId={traceTaskId}>
      ↑ 该面板在「工作台」tab 折叠高级区内；tab=tasks 时根本没渲染
```

结论：**`task` 参数在 `tab=tasks` 下无消费者，id 被静默丢弃**，用户落在任务列表顶部，没有任何定位。任务详情弹层的 `detailTaskId` 是组件内 `useState`（`project-operational-detail.tsx:236`），只认页内点击，从不读 URL。

逐 reason 类型的现状落点（`REASON_META` @ `project-risk-home.tsx:211`，`focus` 计算 @ `:439`）：

| reason 类型 | sourceId 是什么 | 现状 search | 实际效果 |
|---|---|---|---|
| `human_decision` | `decision.id` | `{focus, tab:"approval"}` | 行高亮生效（`:1926` `data-focused-decision`），但**不 scrollIntoView**，长列表里仍要自己找 |
| `execution_failed` | `projectTask.id` | `{tab:"tasks", task}` | **id 丢弃**，落列表顶部 |
| `waiting_human` | `projectTask.id` | `{tab:"tasks", task}` | **id 丢弃**，落列表顶部 |
| `evidence_required` | `evidence.id` | `{tab:"assets"}` | **落点本身就是错的**：`assets` tab 只有 工件/预算/验收 三个子 tab（`project-assets-panel.tsx:26`），**没有证据**。证据面板实际在「工作台 → 折叠高级区 → 治理 tabs 的 evidence 子 tab」（`project-governance-tabs.tsx:86/106`）。用户点「查看证据核验」会落到一个看不到任何证据的页面，且 id 也没进 search |
| `runtime_or_coordination` | `project.id` | `{tab:"overview"}` | 落工作台（`overview` 是 workbench 的合法别名，已有测试固定） |
| `sla_waiting` | `project.id` | `{tab:"overview"}` | 同上；该桶当前恒 0，仅明细态出现 |

**两个比"忘接线"更硬的坑**（决定了本方案不能只改前端）：

1. **分页天花板**：`features/projects/index.tsx:646` `listProjectTasks(..., {limit: 20})`，`:752` `listProjectDecisionRequests(..., {limit: 20})`。任务详情弹层是纯投影（`project-task-detail-dialog.tsx`：`if (!task) return null`）。所以哪怕把 `?task=` 接上，**深链到第 21 条以后的失败任务会静默什么都不发生**——而"很久以前失败、现在才被关注到"恰好是最需要深链的场景。
2. **契约缺口**：`contracts/control-plane/openapi.yaml` 的 `/api/v1/projects/{projectId}/tasks` 只有 `limit/offset/status`；`{taskId}` 下只有 `dismiss`（POST）与 `liveness`（GET），**没有单个 project task 的 GET**。同族的 decision request 也没有单查。

收件箱侧（次级出口的可行性）：

- `routes/_authenticated/inbox/index.tsx:10` 的 `validateSearch` **只收 `sort`**，无条目深链。
- 但 `features/inbox/index.tsx:110` 有 `selectedItemId` state + `onSelectItem`，是现成的 URL 播种缝。
- inbox item 带 `source_id`（= 上游决策/审批请求 id，`internal/inbox/service.go:31`），`listInboxItems` 已支持 `project_id` 过滤 → `/inbox?project=<pid>&source=<decisionId>` 前端定位可行，**无需改 CP 契约**。

### 2.3 页长

- 默认 `pageSize` 12，`pageSizeOptions=[12,20,50]`（`project-portfolio-grid.tsx:682`），栅格 `sm:2 / xl:3` 列（`:659`）。
- 3 列 × 12 条 = 4 行卡。走查截图里 6 条（2 行）已铺满视口 → 12 条约 2 屏。
- 缓解因素（不能忽略）：默认排序已是 `attention`（08-11 spec §4.2 明确「`sort` 是必需参数不是可选增强」），需关注的项目本来就在首屏。所以页长是**扫读成本**问题，不是**漏看**问题——处方按前者定级，不上虚拟滚动。

---

## 3. 根因

三点观察背后其实是**两个**根因，第 1 点和第 3 点各一个。

**根因一（对应观察 1）：生命周期阶段没有紧迫度语义，却被硬映射到紧迫度色板。**

`StatusPill` 自己的注释写得很清楚（`primitives.tsx:116`）：「tone 表达紧迫度/状态，类别请用图标+文字」。而 `draft → configuring → running → acceptance → paused → archived` 是**阶段轴**，不是紧迫度轴——「验收中」既不比「已就绪」更危险，也不更安全。四个实现各自去猜一个紧迫度，于是猜出四个不同答案；写死 `mute` 的那个（#5）反而是唯一没有说谎的。

所以处方不是"给每个状态配个语义色"，而是**把两根轴分开**：紧迫度轴（健康度/关注）独占语义色，阶段轴走一套自己的分类色。

**根因二（对应观察 3）：右栏产出了精确 id，但落点侧没有对应的消费者。**

`deriveProjectRiskSummary` 一直在给每条 reason 带 `sourceId`（`project-risk.ts:252/270/286/316`），生成侧是对的；断的是接收侧——没有"按 id 打开某个对象"的入口，只有"按 tab 打开某个列表"的入口。补的是**对象级入口**，不是链接参数。

---

## 4. 目标与非目标

**目标**

1. 项目生命周期的颜色在全站只有一个事实源，环图 / 卡片 / triage / 详情页四处读同一张表，且不占用语义色预算。
2. 从项目卡右栏的每一条可行动项，都能落到**那一个对象**（任务弹层 / 审批那一行 / 证据那一条），而不是它所在的列表；且深链对不在首页 20 条内的对象同样成立。
3. 首页默认视图不超过约 1.5 屏，且用户可自选密度并被记住。

**非目标**

- 不做全站 25 处 `*Tone` 映射的统一治理（见 §12）。
- 不改任何注意力/健康度的**计数口径**——08-10 spec §6.3 的健康度接线表（`waiting_human_unlinked_count` 只许读这个等）原样保留，本方案只动颜色与跳转。
- 不做卡片虚拟滚动，不改 `sort=attention` 默认。
- 不把决策处理能力从收件箱搬走，也不新建第三个处理出口。

---

## 5. 处方 A：生命周期色单一事实源

### 5.1 两轴分离（承重原则）

| 轴 | 承载什么 | 视觉通道 | 出现位置 |
|---|---|---|---|
| **紧迫度轴** | 健康度 / 关注信号（需关注、正常、识别中、风险待确认） | 语义色 `ok/warn/danger/info/mute` 实底 pill | 卡片健康度 pill、左侧 accent bar、需关注区块 |
| **阶段轴** | 项目生命周期（草稿→配置中→已就绪→验收中→已暂停→已归档） | **中性 pill 体 + `--phase-*` 色点** | 卡片生命周期 pill、环图、triage pill、详情页 pill |

由此，`DESIGN.md`「一行或一卡最多 1 个语义色状态编码」得到满足：卡上唯一的语义色编码是健康度 pill，生命周期只用一个低彩度色点做区分。人类要的"能区分"拿到了，同时「需关注」的红不被抢眼。

### 5.2 `--phase-*` token 族

落 `apps/web/src/styles/theme.css`（token 唯一事实源），并在 `@theme inline` 段注册 `--color-phase-*` 暴露给 Tailwind（与 `--color-artifact` 同法）。

派生规则**沿用文件内已有的 v3.1 OKLCH 公式，但降彩度**，以在视觉层级上永久低于语义色：

- 浅色端：`oklch(0.60 0.08 H)`（语义色是 `C=0.15`）
- 深色端：`oklch(0.75 0.06 H)`（语义色是 `C=0.115`）

| token | 中文 | 浅色 | 对白卡对比 | 深色 | 对暗卡对比 |
|---|---|---|---|---|---|
| `--phase-ready` | 已就绪 | `#697fb1` | 3.98 | `#9baed5` | 7.74 |
| `--phase-acceptance` | 验收中 | `#3f8c9e` | 3.85 | `#82b8c6` | 7.91 |
| `--phase-configuring` | 配置中 | `#a37e53` | 3.71 | `#c8ac8c` | 8.00 |
| `--phase-draft` | 草稿 | `#a68d71` | 3.15 | `#a9947e` | 5.94 |
| `--phase-paused` | 已暂停 | `#7d8088` | 3.95 | `#a1a5ab` | 6.98 |
| `--phase-archived` | 已归档 | `#8f9299` | 3.12 | `#727479` | 3.69 |

（全部 ≥3:1，满足 WCAG 1.4.11 图形对象；且色点是**冗余编码**——pill 文字本身已表达状态，色只是加速扫读。）

**已知弱项**：`configuring` 与 `draft` 同色相（H=70）仅靠明度分离，同屏并置时可辨识度一般。取舍理由：二者语义同族（都是"未就绪"），且实际很少大量并存；若走查发现确实分不开，再把 `draft` 的 H 拉到 40。**不要**为它去借 `--artifact` 紫——那个色位已经被"工件"这一个类别占死。

### 5.3 `StatusPill` 扩展

`primitives.tsx:117` 的 `StatusPill` 目前 `tone` 同时决定文字色、软底和圆点色，无法表达"中性体 + 阶段点"。加一个可选属性：

```tsx
function StatusPill({ tone = "mute", showDot = true, dotClassName, ... })
//                                                    ^^^^^^^^^^^^ 新增
// 圆点 className: cn("size-1.5 rounded-full", dotClassName ?? toneSolidBg[tone])
```

约束：`dotClassName` **只允许传 `bg-phase-*`**，不得用来绕过 tone 传语义色（在 `primitives.test.tsx` 加一条断言 + JSDoc 写明）。不新建 `PhasePill` 组件——阶段是项目域概念，不该进通用基元层。

### 5.4 映射模块与调用点收敛

新建 `apps/web/src/features/projects/project-lifecycle-display.ts`：

```ts
export type ProjectLifecyclePhase = "draft" | "configuring" | "running" | "acceptance" | "paused" | "archived";
export function projectPhaseDotClass(status: string): string;   // → "bg-phase-ready" 等
export function projectPhaseColorVar(status: string): string;   // → "var(--phase-ready)"，环图 conic-gradient 用
```

文案仍走 `lib/status-labels.ts` 的 `projectStatusLabel`（词表事实源不变，本模块只管颜色）。未知状态回退 `--phase-draft` + 灰点，与词表"未知回退原文"同策。

调用点改造：

| 调用点 | 改法 |
|---|---|
| `project-portfolio-grid.tsx:488` | `tone="mute"` + `dotClassName={projectPhaseDotClass(status)}` |
| `project-risk-home.tsx:47` `PROJECT_LAYER_COLORS` | `color` 改读 `projectPhaseColorVar`，删两处硬编码 hex |
| `project-risk-home.tsx:488` `projectStatusTone` | **删除**，pill 改中性体 + 阶段点 |
| `project-operational-detail.tsx:2237` `projectStatusTone` | **删除**，同上 |
| `project-risk-home.tsx:271` `IconTile tone={projectStatusTone(...)}` | 改中性（`tone="mute"`） |

### 5.5 连带：卡片图标底去状态色

`projectStatusIconTone`（`project-portfolio-grid.tsx:109`）删除，IconTile 固定 `bg-card-soft text-ink-3`（或统一 `brand-soft`，走查时二选一定档）。理由见 §2.1——这是既存的规则违背，且它占掉的色彩预算正是阶段点需要的。

---

## 6. 处方 B：关注项精确下钻

**拍板落点：项目内那一条；收件箱作次级出口。** 理由：用户是从项目组合点进来做**项目分诊**的，把他弹到全局收件箱会丢掉项目上下文；但决策的处理事实源在收件箱域，所以在弹层/行内保留「在收件箱处理」链接。

### 6.1 目标落点表

| reason 类型 | 新 search | 落点 | 需要什么 |
|---|---|---|---|
| `execution_failed` | `{tab:"tasks", task:<id>}` | **直接打开该任务的详情弹层** | §6.2 单任务 GET + §6.3 URL 播种 |
| `waiting_human` | 同上 | 同上 | 同上 |
| `human_decision` | `{tab:"approval", focus:<id>}` | 审批 tab 定位到那一行并 **scrollIntoView** | §6.3；+ 缺页兜底 |
| `evidence_required` | `{tab:"workbench", governance:"evidence", evidence:<id>}` | **先纠正落点**（现落 `assets` 是错的，那里没有证据），再定位到那一条 | §6.3 第 4 条 |
| `runtime_or_coordination` | `{tab:"overview"}` | 不变（对象就是项目本身） | — |
| `sla_waiting` | `{tab:"overview"}` | 不变 | — |

### 6.2 契约缺口：补单个 project task 的 GET

新增 `GET /api/v1/projects/{projectId}/tasks/{taskId}` → `ProjectTask`（schema 已存在，直接复用）。

- 契约：`contracts/control-plane/openapi.yaml`（`{taskId}` 路径下已有 `dismiss`/`liveness`，同族增一个 GET）
- CP：`apps/control-plane/internal/project/handler.go` + `service.go` + `internal/storage/queries/project.sql`（按 `project_id + task_id` 单查，租户隔离与 `dismiss` 同判据）
- 生成：`generate:control-plane` → `verify:contracts`
- Web：`lib/api/projects.ts` 加 `getProjectTask`

**决策请求侧同样有缺页问题但不新增接口**：`focus` 命中不到已加载 20 条时，改为把 `listProjectDecisionRequests` 的 limit 提到 50 并在面板顶部显示"该决策不在当前列表"提示 + 收件箱链接。理由：决策数量级远小于任务，且收件箱本来就是它的处理事实源，不值得为它单开接口。

### 6.3 前端接线

1. **`detailTaskId` 受控化**（`project-operational-detail.tsx:236`）：改为 `taskId ?? internalState` 双源——URL 有 `?task=` 时以 URL 为准，页内点击仍走内部 state；关闭弹层时清掉 URL 参数（`navigate({search: prev => ({...prev, task: undefined})})`），避免"关了又被 URL 拉开"的循环。
2. **弹层缺页回落**（`project-task-detail-dialog.tsx`）：`tasks.find` 落空时不再 `return null`，改为 `useQuery(getProjectTask)` 单查；查询中显示骨架，404 显示"任务不存在或已被清理"空态。**这是本处方最关键的一条**——没有它，深链在最需要的场景里静默失效。
3. **决策行 scrollIntoView**（`project-operational-detail.tsx:1926` 已有 `data-focused-decision` 锚点）：与 `?tab=trace` 的既有做法一致（`:256` 用 `setTimeout` 等渲染后滚动），复用同一手法。
4. **证据落点纠正 + 参数打通**（比其它几条多一步，别照抄"补个参数"就完事）：
   - `REASON_META.evidence_required.tab` 从 `"assets"` 改为 `"workbench"`——**现状落点是错的**，`ProjectAssetsPanel` 的三个子 tab 是 工件/预算/验收，根本没有证据（`project-assets-panel.tsx:26`）。
   - 证据面板真实位置是 `ProjectGovernanceTabs` 的 `evidence` 子 tab（`project-governance-tabs.tsx:86/106`），而它在工作台 tab 的**折叠高级区内**（`project-operational-detail.tsx:602`）。所以深链需要：展开高级区 + 把治理 tabs 切到 `evidence` + 滚动定位——与既有 `?tab=trace` 的做法同构（`:256` 展开 `advancedOpen` 后 `setTimeout` 滚动），复用那套手法。
   - `ProjectGovernanceTabs` 当前是非受控 `SoftTabs`，需要加 `initialTab` 入参（参照 `ProjectAssetsPanel:36` 的 `initialTab` 先例）。
   - `ProjectTriageReasonRow` 的 `focus` 计算（`project-risk-home.tsx:439`）当前把 `evidence_required` 排除在外，一并补上。
   - **可选的更好解**：把证据从折叠高级区提到一个用户找得到的位置。这是 IA 变更，超出本方案范围，**若实施者认为有必要须先问人类**，不要顺手改。
5. **`traceTaskId` 语义澄清**：保留 `?tab=trace&task=<id>` 走执行轨迹面板过滤的既有行为（`index.tsx:347` 注释即此意），但 `tab=tasks` 时同一个 `task` 参数改为开弹层。**两种消费共用一个参数名靠 tab 判别**——在 `index.tsx` 该处补注释写明，否则下一个人会重蹈"参数看着接了其实没消费"。

### 6.4 收件箱次级出口

- `routes/_authenticated/inbox/index.tsx:10` 的 `validateSearch` 增加 `project?: string` 与 `source?: string`。
- `features/inbox/index.tsx:110` 用 `source` 在已加载列表里按 `item.source_id` 定位，命中则 `setSelectedItemId` + scrollIntoView；未命中显示"该待办可能已处理或不在当前视图"提示（**不静默**——这正是本方案在修的病）。
- 任务详情弹层与审批行内提供「在收件箱处理 →」链接，带上 `project` + `source`。

---

## 7. 处方 C：卡片密度与页长

1. **默认页大小 12 → 9**（`project-portfolio-grid.tsx` 的 `Pagination`）：3 列 × 3 行，约 1.5 屏。`pageSizeOptions` 改 `[9, 12, 20, 50]`。
   - 服务端：08-11 spec §4.2 的 `limit` 参数本就可变，无契约改动。
2. **密度切换（卡片 / 紧凑列表）**：08-11 spec §6.3 已开口子「卡片/列表切换只改变表现，不改变数据口径」——本方案兑现它。工具栏加 chip 切换，选择存 `localStorage`，紧凑列表复用既有列表态密度习惯（名称+阶段 pill 同行、任务构成条压成单行、健康度 pill 右对齐）。
   - **口径约束**：两种密度渲染**同一份** portfolio 响应，不得因密度不同发不同请求或算不同计数。
3. 不做虚拟滚动；不改 `sort=attention` 默认（它是"不漏看"的现有保障）。

---

## 8. 实施批次

> **顺序硬约束（先读完再排期）**：立项当天（2026-08-11）`git status` 显示 08-11 分层状态方案的一批改动**未提交**，涉及 `project-portfolio-grid.tsx`、`project-risk-home.tsx`、`features/projects/index.tsx`、`internal/project/service.go`、`storage/queries/project.sql`。
>
> **四个批次全部与之有文件重叠——没有一个能"无脑并行"。** 默认结论：**整个方案等在途批次落地（提交/合入）后再开工**。
>
> 唯一可提前动的是 P1 的前半段（见下表 P1a），它与在途文件零重叠。

**开工前必做的一步**：确认在途批次是否已落地——对上表文件跑 `git status --porcelain <路径>`，全部干净才算落地；不干净就去问在途那位（**不要**替他 stash、commit 或切分支）。

| 批次 | 内容 | 主要文件 | 与在途重叠 | 依赖 |
|---|---|---|---|---|
| **P1a** | 处方 A 地基：`--phase-*` token 族 + `@theme inline` 注册 + `StatusPill.dotClassName` + 新建映射模块 | `styles/theme.css`、`components/superteam/primitives.tsx`、`features/projects/project-lifecycle-display.ts`（新建） | **无** | 无——**可立即开工** |
| **P1b** | 处方 A 接线：五处调用点改造 + 图标底去色 | `project-portfolio-grid.tsx`、`project-risk-home.tsx`、`project-operational-detail.tsx` | 前两个**在途** | P1a + 在途落地 |
| **P2** | 处方 B 契约与 CP：单任务 GET | `contracts/control-plane/openapi.yaml`、`internal/project/{handler,service}.go`、`storage/queries/project.sql` | 后两个**在途** | 在途落地 |
| **P3** | 处方 C：默认页大小 + 密度切换 | `project-portfolio-grid.tsx` | **在途** | 在途落地（建议紧随 P1b，同文件） |
| **P4** | 处方 B 前端接线：URL 播种 + 弹层缺页回落 + 决策滚动 + 证据落点纠正 + 收件箱深链 | `features/projects/index.tsx`、`project-operational-detail.tsx`、`project-task-detail-dialog.tsx`、`project-risk-home.tsx`、`project-governance-tabs.tsx`、`features/inbox/index.tsx`、`routes/_authenticated/inbox/index.tsx` | 前两个**在途** | P2 + 在途落地 |

注：P1a 单独落地不产生用户可见变化（只加 token 与可选属性，无调用点），因此**不需要 E2E**，跑组件测试 + typecheck 即可；G1 在 P1b 后验。

---

## 9. 验证门禁

按 CLAUDE.md「默认完成条件是真实端到端」，本方案改交互与数据链路，**不适用轻量例外**。验证期须记 `control-plane`/`web` pid 并在收尾复核 `owner=` 同源。

**G1（处方 A · 浏览器）**：同一个「验收中」项目，环图色点 / 卡片 pill 点 / 右栏 triage pill 点 / 详情页 pill 点**四处取色一致**；卡上仅健康度一处语义色实底。暗色端复验一遍。

**G2（处方 A · 单测）**：`project-lifecycle-display` 六个阶段各出一次；`primitives.test.tsx` 断言 `dotClassName` 不接受语义色类名。

**G3（处方 B · 真实链路，承重）**：造一个**排在第 21 条以后**的失败任务（真实项目，直插或真跑），从首页右栏点「查看失败任务」→ **弹层打开且显示的是那一条**。这一条是整个处方 B 的判别性验证；用前 20 条内的任务验等于没验。

**G4（处方 B）**：`human_decision` 点「处理决策」→ 审批 tab 且目标行进入视口并高亮；`evidence_required` 点进去定位到那一条证据。

**G5（处方 B · 收件箱）**：从任务弹层点「在收件箱处理」→ `/inbox` 打开对应条目的处理面板；再造一个已处理的 source → 显示提示而非静默。

**G6（处方 C）**：默认 9 条 3 行；切紧凑列表后计数与卡片态**逐项相等**（同一份响应，不得漂移）；刷新后密度选择被记住。

**门禁命令**：`corepack pnpm --filter @superteam/web test`（projects + inbox）、`verify:contracts`（P2 后）、`go test ./internal/project/...`、typecheck。

---

## 10. 风险与取舍

| 风险 | 处置 |
|---|---|
| 阶段色新增六个 token，后续状态机变更需同步维护 | 映射函数对未知状态回退灰点，不崩；新增阶段视为"未补色"，走查即见 |
| `configuring` / `draft` 同色相仅明度分离 | 已知弱项，走查确认后再决定是否拉开 H（§5.2） |
| `?task=` 一个参数两种消费（trace 过滤 / 开弹层）靠 tab 判别 | 在 `index.tsx` 与两处消费点写明注释；这是本次修的病本身，注释是防复发的最低成本手段 |
| 单任务 GET 增加一次请求 | 只在深链且缺页时触发，正常页内点击仍走已加载投影，零新增请求 |
| 决策深链缺页只做 limit 提升 + 提示，非确定性 | 明确取舍：决策量级小且处理事实源在收件箱，提示 + 收件箱链接足够；若实测仍常落空再补单查 |
| 与在途 08-11 批次同文件 | §8 顺序硬约束；提交前按共享 checkout 规矩只 `git add` 显式路径并核对暂存 |

---

## 11. 交接须知（给接手实施的会话）

> 本节是把方案交给冷启动会话时的必读项。除 CLAUDE.md / DESIGN.md 的通用约束外，以下是**本方案特有**的坑。

### 11.1 已由人类拍板，不得重开

三处岔口是人类 2026-08-11 明确选定的，**不是待定项，也不是可以"顺手优化"的默认值**：

1. 阶段色走**新增 `--phase-*` token 族**——不要因为"复用 `--info`/`--brand` 更省事"就改回语义色。那正是本方案在修的病。
2. 下钻默认落**项目内那一条**，收件箱是次级出口——不要改成全部跳收件箱。
3. 页长处方是**密度切换 + 默认页大小 12→9**——不要替换成虚拟滚动或压卡片高度。

若实施中发现某一处确实做不通，**停下来问人类**，不要自行换方案。

### 11.2 本方案的代码坐标是"线索"，不是坐标

- 全文 `file.tsx:488` 形式的行号是**对着 2026-08-11 当天的工作树**读出来的，而那棵工作树**含未提交的在途改动**。定位一律**按符号名/内容搜**（`projectStatusTone`、`REASON_META`、`detailTaskId` …），行号只当粗略指引。
- 若某处符号已不存在或语义已变，说明在途批次动过它——**先核对现状再改**，不要按方案描述硬套。
- **§2 现状核查请当"待验证线索"重跑一遍，不要当既成事实。** 立项过程中已两次发现自查不到底的错误并当场修正（证据落点写成了 `ProjectAssetsPanel` 其实那里没有证据；§8 批次依赖一度声称 P1/P3 与在途无重叠其实有）。这类"以为核过其实没核"的瑕疵按定义无法自列清单，§11.7 只覆盖我**知道**自己没验的部分。开工前花十几分钟按符号名复核 §2 的每个论断，成本远低于按错误前提改完再回滚。

### 11.3 在途改动与共享 checkout 纪律（最容易出事的一条）

立项当天 `git status` 有一批**未提交**改动（08-11 分层状态方案：`internal/project/service.go`、`storage/queries/project.sql`、`project-portfolio-grid.tsx`、`project-risk-home.tsx`、`features/projects/index.tsx` 等）。因此：

- **四个批次全部与之有文件重叠，只有 P1a 例外**（§8 顺序硬约束表）。开工前先按 §8 的 `git status --porcelain` 检查确认落地，**不要**替在途那位 stash / commit / 切分支。
- 全仓生成（sqlc / openapi）会吸收他人在途改动——P2 尤其致命。
- 提交只用 `git add <显式路径>`；交织文件只暂存自己的 hunk；**提交前核对暂存内容**。
- 不为他人切/删分支。优先一会话一 worktree。
- 如果从 worktree 干活：**未共享 `SUPERTEAM_DEV_PID_DIR` 时 `dev-services.sh restart` 是退出码 0 的静默空操作**——服务还跑着别人的代码，你会拿到假的验证结论。`status` 的 `owner=` 是真实 cwd，验证前后各看一次。

### 11.4 不需要数据库迁移

P2 只加一个按 `project_id + task_id` 的 **SELECT**，无 schema 变更。**不要建迁移文件**（建了还会引发编号撞号）。改完走 `generate:control-plane` + `verify:contracts` + `migrate-validate` 即可。

### 11.5 截图基线会红

`apps/web/src/features/projects/components/__screenshots__/` 下有 `project-operational-detail.test.tsx` 的基线，而处方 A 会改该文件的 pill 与 IconTile 颜色 → **基线必然失配**。

- 这属于预期内失配，但**必须逐张看 diff 再更新基线**，确认变化只有颜色、没有布局塌陷。
- `project-portfolio-grid.tsx` / `project-risk-home.tsx` **没有**截图基线（已确认），改动只会被行为测试覆盖。
- 测试命令走 `corepack pnpm --filter @superteam/web test`；**禁止 `npx vitest run` / `npx playwright install`**。web 全量测试堆易崩，按既有习惯分块 + `--max-workers=1`。

### 11.6 G3 造数配方（承重验证，不能糊弄）

`ListProjectTasks` 的窗口是 `ORDER BY updated_at DESC LIMIT 20`（`storage/queries/project.sql:568`），且默认排除 `dismissed_at IS NOT NULL`。所以：

- 目标任务 = 某个**失败态、且 `updated_at` 排在该项目第 21 位之后**的 project task。
- 造法：在任务数 >20 的真实项目里挑一条较早失败的任务；或先让 20 条其它任务的 `updated_at` 更新到它之后。执行实例类数据按既有惯例可直插 DB 造。
- **用前 20 条内的任务验 G3 等于没验**——那种情况下即使不做 §6.3 第 2 条（弹层缺页回落）也会"看起来通过"。这是整个处方 B 唯一的判别性验证。
- 附带效应值得知道：窗口按 `updated_at` 滑动，同一条深链在不同时刻可能一会儿在窗口内一会儿不在——这正是必须做单任务 GET 而非"把 limit 调大"的理由。

### 11.7 本方案未经验证的部分（不要当既成事实）

立项过程**全程是静态代码核查，没有启动过任何服务**。以下是推断或估算，实施时须自行确认：

| 项 | 状态 |
|---|---|
| "12 张卡 ≈ 2 屏""9 张 ≈ 1.5 屏" | **目测截图估算**，未实测。若实测差异大，页大小取值可调整（保留"默认不超过约 1.5 屏"这个目标） |
| §5.2 六个 `--phase-*` 色值 | 由 OKLCH 公式**计算**得出、对比度已算过，但**没在真实页面上看过**，暗色端尤其。走查时若观感不成立，允许在保持"低彩度、≥3:1、同一族"前提下微调 |
| `configuring` / `draft` 同色相可辨识度 | 已知弱项，未实测（§5.2 已记处置预案） |
| `StatusPill` 加 `dotClassName` 对既有调用点的影响 | 未跑测试。属新增可选属性，理论上向后兼容，但**须跑一遍 `components/superteam` 的测试**确认 |
| 收件箱 `?source=` 在"目标 item 不在已加载页"时的表现 | 只确认了 `listInboxItems` 支持 `project_id` 过滤、item 带 `source_id`；**未验证**分页边界。§6.4 已要求未命中必须显式提示——这条不能省，否则又造一个静默失败 |
| §6.3 第 4 条的证据落点 | 已核实现状落点是错的（`assets` tab 无证据），但**纠正后的深链路径未实现过**；`ProjectGovernanceTabs` 加 `initialTab` 是按 `ProjectAssetsPanel` 先例推的，实施时确认该组件确实非受控 |

### 11.8 收尾

按 CLAUDE.md：本方案改交互与数据链路，**不适用轻量例外**，默认完成条件是真实端到端（Web + CP + DB，按批次覆盖面）。声称 E2E 通过须证明验证期内服务未被接管（记 pid、复核 `owner=`）。收尾走 `superteam-completion-check`。

---

## 12. 明确不做

- 全站 25 处 `*Tone` 映射的统一治理——本方案只立"两轴分离"这一个可复用原则和项目生命周期一根轴的样板，其余触达时收敛。
- 项目卡虚拟滚动 / 无限滚动。
- 把决策处理搬进项目内成为唯一出口（收件箱仍是处理事实源）。
- 修改任何注意力计数口径（08-10 spec §6.3 接线表原样保留）。
