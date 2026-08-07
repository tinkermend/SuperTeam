# 接续会话可观测性硬化（Session Resume Observability）

- 日期：2026-08-07
- 状态：**已实施（2026-08-07）**
- 系列：接续 `2026-08-01-demand-continuation-design.md` 的体验硬化切片（该 spec P1 resume 预检降级之遗留）
- 交付性质：Control Plane 写路径留痕 + 卷宗时间线投影 + 执行轨迹中文一句；**不改** resume 判据、不改 runtime/provider 管道、不改接续血缘模型
- 目标读者：评审人 / 实施会话（本文自包含；实施前必读接续 spec §5/§6.1 与卷宗 `event_narrative` 约定）
- 待拍板（见 §14）：
  1. 时间线用**新事件类型**还是 **kind=other + 中文 summary**
  2. 接续链上「无候选会话」是否也进时间线
  3. 本批是否包含执行轨迹 UI 中文状态（建议包含，成本低）

---

## 1. 背景与目标

### 1.1 问题

接续与 resume 预检已经「能跑」：

- 派发前控制平面预检：会话过期（默认 7 天）或跨 runtime 节点时**主动不下发** `provider_session_id`，正常开新会话，避免 `--resume <失效 id>` 拖垮整单（接续 spec §6.1，2026-08-04 已实施）。
- 降级留痕写在 run 的 `params.metadata`：`session_resume_skipped` + `session_resume_skipped_session_id`。
- 卷宗已有链条展示、左轨按链折叠、「继续这一单」入口。

但人在卷宗里**看不见、看不懂**：

| 事实 | 人能否在卷宗看到 | 后果 |
|---|---|---|
| 预检放弃 resume（过期 / 跨节点） | 否（仅 DB metadata） | 以为「接续没续上上下文」是静默 bug |
| 成功接上该员工上次会话 | 否 | 无法确认「同员工续上了」 |
| 换人导致开新会话 | 否 | 无法区分「该续没续」与「本就不该续」 |

CHANGELOG（2026-08-04）已记遗留：

> 降级目前只落在 run 的 `params.metadata`（可查），尚未投影到卷宗时间线。

本 spec 只补「人看得懂、信得过」，**不重做**接续与预检。

### 1.2 已经对的部分（不要重做）

- resume 判据：`evaluateSessionResume`（`session_stale` / `session_node_mismatch`）
- 会话键：员工 × 任务血缘根；多员工各续各的
- 判据放控制平面；runtime 只消费 `provider_session_id`
- 无候选 ≠ 降级，不留痕（普通新单零噪音）
- 卷宗时间线是**协调叙事**（`project_events` → `event_narrative` 中文），不是完整审计流水；完整执行事件在「执行轨迹」

### 1.3 目标

1. 派发时 resume 的**三种人话结论**进入卷宗时间线（中文一句原因/结果）。
2. 成功与主动降级**对称留痕**（不能只记 skip）。
3. 执行轨迹 / run 详情能与卷宗对上同一结论（同一字段语义）。
4. 不改变：预检阈值、开新会话行为、接续血缘、跨项目禁止、链深上限语义。

### 1.4 非目标

- runtime 在 provider `--resume` 进程失败后再软降级开新会话（独立立项，见 §11）。
- 调整 7 天阈值或链深 10 的默认值（可后续 systemconfig，本批不强制）。
- 跨项目接续 / 跨项目 resume。
- 改 Temporal 协调逻辑、planner、provider adapter。
- 把完整 session id 列表或调试字段堆进卷宗主时间线。
- 历史 run 回填（只保证本批上线后的新派发生效；读路径不依赖回填）。

---

## 2. 一句话方案

> **派发决定 resume 的那一刻，把结论写成结构化 outcome；bind 成功后追加一条带中文 summary 的 project_event 进卷宗；run metadata 与 task_event 同步同一语义——人在卷宗看到「续上了 / 主动新开（原因）」，排障在执行轨迹对得上。**

---

## 3. 概念与状态机

### 3.1 SessionResumeOutcome（派发期结论）

| Status | 含义 | 是否进卷宗（默认） |
|---|---|---|
| `resumed` | 预检通过并下发了 `provider_session_id` | **是** |
| `skipped` | 有候选但主动放弃（原因码见下） | **是** |
| `none` | 无候选或不尝试 resume（新会话，非降级） | **否**（例外见 §3.3） |

### 3.2 SkipReason（仅 `skipped`）

与现码一致，不新增原因码（本批）：

| 码 | 条件 | 中文一句（summary，拍板文案） |
|---|---|---|
| `session_stale` | 候选最后活跃超过 `DefaultSessionResumeMaxIdle`（7d） | `原会话超过7天未活跃，已主动开新会话（未沿用旧会话）` |
| `session_node_mismatch` | 候选绑定节点 ≠ 本次派发目标节点 | `原会话在其他运行节点，已主动开新会话` |

`resumed` 的 summary：

> `已接上该员工上次会话继续执行`

### 3.3 `none` 是否曝光（待拍板，默认否）

| 场景 | 默认 |
|---|---|
| 普通新 demand、无祖先会话 | 不写事件（现 spec） |
| 接续链上、同员工祖先无会话 / 换人 | **默认不写**；若评审要求「换人也要说清楚」，改为仅当 `revision_root` 来自接续上溯时写：`该员工在本链尚无可用会话，已开新会话` |

**推荐默认：否。** 换人开新会话是预期行为；接续弹层已有说明文案。避免每条派发多一条噪音。

### 3.4 与「任务失败」的边界

| 类型 | 任务是否失败 | 本 spec |
|---|---|---|
| 预检主动 `skipped` → 开新会话 | 否 | 卷宗 warn 一句 |
| `resumed` 成功下发 | 否 | 卷宗 info 一句 |
| Provider 仍因失效 id 失败 | **是** | **不在本批修复**；仅保证预检路径可见 |

---

## 4. 时序（必须尊重）

当前派发顺序（不可回退到 queue 时刻写 resume）：

```text
1. QueueProjectTask
   → project_event: project_task.dispatched（「已排队」）
   → 此时尚无 resume 结论

2. StartProjectTaskRun
   → Resolve lineage root
   → FindProviderSessionCandidateForTaskRoot
   → evaluateSessionResume
   → metadata: provider_session_id | session_resume_skipped*
   → 创建 run + run_dispatched task_event

3. BindProjectTaskAttemptRun
   → 任务绑定 run
```

**结论写入点：步骤 2 成功之后、步骤 3 成功之后（推荐）追加 continuity 事件。**

- 若写在步骤 2 成功、3 失败：可能留下「已续会话」但 bind 失败的脏叙事 → **必须在 bind 成功后写**，或与 bind 同事务（若无跨库事务，则 bind 成功后写 + 幂等键防重）。
- 重入 / 重放：同一 `project_task_id` + `attempt_id` 只保留一条 continuity 事件（见 §7）。

```text
StartProjectTaskRun 成功
  → 返回 SessionResumeOutcome（跨包显式字段，禁止 workflow 反查 run metadata）
Bind 成功
  → 若 outcome.Status ∈ {resumed, skipped}（+ 可选 none）
     → AppendProjectEvent(continuity)
  → run_dispatched / metadata 已在步骤 2 对齐
```

---

## 5. 数据与契约

### 5.1 Run metadata（机器事实，已有扩展）

在现有字段上收敛为统一语义（向后兼容：旧字段保留）：

| 字段 | 何时 | 说明 |
|---|---|---|
| `session_resume_status` | 尝试 resume 决策后 | `resumed` \| `skipped` \| `none` |
| `provider_session_id` | 仅 `resumed` | 下发的会话 id（已有） |
| `session_resume_skipped` | 仅 `skipped` | 原因码（已有） |
| `session_resume_skipped_session_id` | 仅 `skipped` | 被放弃的候选 id（已有） |
| `session_resume_session_id` | `resumed` 时可选冗余 | 与 `provider_session_id` 同值，便于排障扫描 |

**规则**：`none` 时不写 skip 字段；无候选不把 `session_resume_skipped` 写成空字符串。

### 5.2 StartProjectTaskRunResult（跨包）

```text
SessionResumeStatus   string  // resumed | skipped | none
SessionResumeSkipReason string // 仅 skipped
SessionResumeSessionID string // resumed=续上的 id；skipped=被放弃的 id；none=空
```

`projectcoordination` 只消费该结构，**不** import 原因码中文表以外的 employee 内部细节；中文 summary 的生成放在：

- **推荐**：`employee` 包导出 `SessionResumeSummary(outcome) string`，workflow 层调用；或
- project 包维护平行映射表（**禁止**，易漂移）→ 否决。

### 5.3 project_events（人话事实）

#### 事件类型（待拍板 A/B）

**方案 A（推荐）：新类型**

```text
ProjectEventType = "project_task.session_continuity"
```

- `event_narrative`：
  - Title：`会话接续`（基础标题；summary 承载一句详情）
  - Kind：新增 `session_continuity` **或** 暂用 `other`（若不想扩 OpenAPI enum）
  - Severity：`resumed` → info；`skipped` → warn
  - Noise：`false`（必须进卷宗）

**方案 B：不改类型枚举**

- 仍用某种已有类型或仅靠 summary——**否决倾向**：语义不清，与「任务开始」混读。

**评审推荐 A + kind 用 `other` 可减少 OpenAPI 枚举变更；若产品要独立筛选/着色再升 kind。**

#### Payload（结构化，卷宗主文案仍用 Summary）

```json
{
  "project_task_id": "…",
  "project_task_attempt_id": "…",
  "digital_employee_id": "…",
  "demand_id": "…",
  "session_resume_status": "skipped",
  "session_resume_skip_reason": "session_stale",
  "provider_session_id": "…",
  "revision_root_task_id": "…"
}
```

- `Summary`：§3.2 中文一句（**唯一用户主文案**）。
- `ResourceType/ID`：`project_task` / task id（便于 dossier 挂实体与打开任务，与 dispatched 一致）。
- `ActorType/ID`：`project_coordinator` / **attempt id**（**不同于** dispatched 事件的 task id，见下方幂等小节的理由；`actor()` 对 `project_coordinator` 类型永远兜底成「协调线程」，不依赖 actor_id 能反查任务，故此处换成 attempt id 不影响展示名解析）。

#### 幂等

```text
唯一业务键（逻辑）：
  project_id + event_type + project_task_attempt_id
```

**评审揪出的坑（务必按此实现，不要照抄 dispatched 事件的写法）**：现有 `ProjectTaskEventExists(eventType, actorID)`（`project/pg_repository.go:3938`）只按 `event_type + actor_id` 查重，没有 attempt 维度；`dispatched` 等既有事件全部拿 **task id** 当 actor_id（`project_store.go:2174/2351/3272`、`predispatch_gate.go:438`）。若 continuity 事件照抄这个惯例（actor_id=task id），同一任务第二个 attempt 的结论（例如第一次 `resumed`、重试后因过期变 `skipped`）会被判定"该 (task, event_type) 已存在"而**静默吞掉**——刚好违背本节开头声明的 attempt 级业务键，也不会被 §10 的 O5（只测同 attempt 重入）测出来。

**实现**：`ActorID` 传 **attempt id**（不是 task id），直接调用 `ProjectTaskEventExists(project_task.session_continuity, attemptID.String())` 即与业务键完全对齐，不需要再退到"payload 内查重"的旁路方案。`ResourceType/ID` 仍用 task id（用于 dossier 实体跳转），与 `ActorID` 语义分离，互不影响。dispatch 重入（同一 attempt 的 activity 重试）不得双写；不同 attempt 各自判重、各自可写。

### 5.4 task_events（`run_dispatched`）

在现有 `run_dispatched` payload 中增加（不新增 event_type，降低 activity 词表膨胀）：

```json
{
  "command_id": "…",
  "node_id": "…",
  "session_resume_status": "resumed|skipped|none",
  "session_resume_skip_reason": "…",
  "provider_session_id": "…"
}
```

员工动态流标签仍可为「命令已下发」；详情/轨迹读 payload。

### 5.5 OpenAPI / 卷宗读模型

| 变更 | 是否必须 |
|---|---|
| 新 `ProjectEventType` 常量 + narrative 覆盖测试 | 是（A） |
| `ProjectDemandDossierTimelineItem.kind` 新枚举值 | 仅当 kind ≠ `other` |
| dossier 响应新增字段 | **否**（用现有 title/summary/severity） |
| 执行轨迹 attempt DTO 增加 `session_resume_status` 等 | **建议**（否则前端只能扫 raw metadata） |

前端卷宗：**零翻译**；只渲染服务端 summary。

### 5.6 中文映射唯一源

```text
employee.SessionResumeUserSummary(outcome) string
```

- 单测锁文案（防漂移）。
- Web `status-labels` **不**承接该映射（服务端已渲染）。
- 若文案含「7 天」：与 `DefaultSessionResumeMaxIdle` 同源格式化（避免改常量忘改文案）。

---

## 6. 展示

### 6.1 卷宗协调时间线

- 出现时机：任务 bind 成功且 outcome 为 `resumed` / `skipped`。
- 呈现：Timeline 一项；title 可用 narrative「会话接续」或直接用 summary 作主标题（实施时二选一，**推荐 title=叙事短名，description=summary 一句**，与现 Timeline 组件一致）。
- 可点实体：任务名 → 打开任务详情（与 `task_dispatched` 相同 open_target）。
- 密度：drive/inspect 均显示（非 noise）。
- 诚实边界不变：时间线旁仍说明「完整执行事件见执行轨迹」。

### 6.2 执行轨迹 / 任务详情

- 现有 `provider_session_id` MetaBlock 旁增加状态：
  - `已接上上次会话`
  - `已开新会话 · 原会话过期`
  - `已开新会话 · 原会话不在本节点`
  - `新会话`（none，可选不显示额外句）
- 文案可与 §3.2 缩短版一致；数据来自 attempt/run 读模型，不解析 event_type。

### 6.3 接续弹层

- 保持现有说明即可；本批**不强制**改文案。
- 可选后续：链上曾出现 skip 的统计——不做。

### 6.4 不做的展示

- 卷宗主时间线刷 session id 全文（可放 payload / 轨迹）。
- 左轨链条筹码上挂 resume 状态（demand 级 UI 不承载 task 级事实）。

---

## 7. 写路径细则

### 7.1 employee：`StartProjectTaskRun`

在现有 `evaluateSessionResume` 分支：

```text
switch decision:
  resumed:
    metadata["provider_session_id"] = id
    metadata["session_resume_status"] = "resumed"
    outcome = {resumed, sessionID: id}
  skipped:
    metadata["session_resume_skipped"] = reason
    metadata["session_resume_skipped_session_id"] = candidate.ID
    metadata["session_resume_status"] = "skipped"
    outcome = {skipped, reason, sessionID: candidate.ID}
  default:
    metadata["session_resume_status"] = "none"  // 仅当 shouldAttemptSessionResume 为真时？见下
    outcome = {none}
```

**`none` 写入 metadata 的范围（推荐）**：

- 仅当 `shouldAttemptSessionResume(sessionPolicy)` 为真时写入 `session_resume_status=none`（表示「尝试过查找」）。
- 政策禁止 resume 的路径：不写 status，outcome=`none` 且不进卷宗。

`run_dispatched` payload 带上 outcome 字段。

Result 返回 outcome 给调用方。

### 7.2 projectcoordination：`DispatchProjectTask` / resume queued start

`runStarter.StartProjectTaskRun` 返回后、`BindProjectTaskAttemptRun` 成功后：

```text
if shouldEmitSessionContinuity(outcome):
  AppendProjectEvent(
    type: project_task.session_continuity,
    summary: SessionResumeUserSummary(outcome),
    payload: {...},
    resource: project_task,
    actor: attempt_id,
  )
```

**评审揪出的坑：不是两处，是三处。** §7.3 要求 continuity 写入失败必须上抛、不得静默吞掉；但上抛会让承载这段逻辑的 activity 失败，Temporal 会重试整个 activity，重新进入 `DispatchProjectTask`。此时任务已经 bind 过（`task.DigitalEmployeeRunID != nil`），会命中 `DispatchProjectTask` 内部**已绑定短路分支**（`project_store.go:3266-3283`，判 `ProjectTaskEventExists(dispatched)` 已存在后直接跳 `advanceDispatchedTaskDemand`），**不会**再走"首次 start"或"queued resume start"这两条路径。若 continuity 写入只挂在这两条路径上，第一次写入失败后就永远补不回来——与"不得静默吞掉"的意图直接矛盾。

三处都要挂（幂等查询见 §5.3，用 attempt id 判重，天然防止已成功写过的路径重复写）：

1. `DispatchProjectTask` 首次派发路径（bind 成功后）
2. `DispatchProjectTask` 已绑定短路分支（`project_store.go:3266-3283`）——**activity 重试后的自愈补写点，最容易漏**
3. `resumeQueuedProjectTaskRunStart`（queued resume 路径，`project_store.go:3497` 起）

### 7.3 失败与重试

| 情况 | 行为 |
|---|---|
| Start 失败 | 无 continuity 事件（无 outcome 或未 bind） |
| Bind 失败 | 不写 continuity；既有 failure 事件路径不变 |
| Continuity Append 失败 | **记录错误并返回**（与 dispatched 事件同级严谨度）；不得静默吞掉——否则又回到「人看不见」 |
| Dispatch 重入已 bind | 先查 continuity 是否已存在；存在则跳过 |

### 7.4 非 project_task 路径

- chat / direct run：本批**不**写 project_event（无 demand 卷宗）。
- 若 chat 也要可见：不在本 spec；metadata + task_event 增强可顺带覆盖 chat 的 run 详情。

---

## 8. 明确不改的边界

| 项 | 现状 | 本批 |
|---|---|---|
| 跨项目接续 | 禁止（同项目硬编码） | **维持永久禁止**（带 session resume 的接续） |
| 链深上限 | 默认 10 | 不改默认；不强制配置化 |
| 预检阈值 7 天 | 常量 | 不改 |
| 无候选不留痕 | 是 | 默认维持（§3.3） |
| runtime resume 失败软降级 | 无 | 不做 |
| 左轨/链条模型 | 已实施 | 不改 |

### 8.1 跨项目为什么继续禁止（评审备忘）

- 工作区、repo binding、coordinator workflow、预算与授权均为项目作用域。
- Provider 会话文件在原节点 agent home，跨项目 resume 语义错误。
- 若未来「工作挪到另一项目」：应为**交接包 / 新 demand 引用产物**，明确 **不** 续 provider session，且不复用 `continues_demand_id` 跨项目。

### 8.2 链展示 / 深度

- 链展示已满足「第 k/n + 折叠」；本批不扩。
- 深度 10 是安全阀；超限已有「请另开一单」。配置化列入 §12 观察项。

---

## 9. 实施切片

### 切片 A — 必做（本 spec 交付定义）

1. `SessionResumeOutcome` + metadata 收敛 + `SessionResumeUserSummary`
2. `StartProjectTaskRunResult` 带回 outcome；`run_dispatched` payload 对齐
3. `ProjectEventTaskSessionContinuity` + `event_narrative` + 覆盖测试
4. `ProjectStore` bind 成功后写事件（三处写入点——首次派发/已绑定短路重试自愈/queued resume，见 §7.2 + attempt id 幂等，见 §5.3）
5. OpenAPI：若新 event 出现在对外枚举/文档则更新；dossier kind 策略按 §5.3 拍板
6. 执行轨迹读模型 + 中文状态一句（若 §14 拍板包含）
7. 测试：预检单测文案锁；派发集成（stale → event+metadata；resumed → event）；dossier 时间线含中文且无 raw event_type 泄露
8. 真链：父单会话时间戳改旧 → 接续派发 → 卷宗可见「主动开新会话」；正常续上可见「已接上…」
9. 门禁：`verify:control-plane`（+ 若改契约 `verify:contracts`；若改 web `verify:web`）

### 切片 B — 不做进 A，观察

- `none` 在接续链上的可选曝光
- max idle / chain depth → systemconfig
- 历史数据回填脚本

### 切片 C — 独立立项

- runtime：resume 进程失败 → 剥离 session id 重开新会话并留痕
- 跨项目交接包（非 resume）

---

## 10. 验收判据

| ID | 操作 | 期望 |
|---|---|---|
| O1 | 候选会话 stale，派发同员工 | 任务 running；metadata `skipped+session_stale`；**卷宗时间线一条中文**含「超过 7 天」或等价；**无** `provider_session_id` 下发 |
| O2 | 候选跨节点 | 同上，文案为节点原因；原因码 `session_node_mismatch` |
| O3 | 候选新鲜且同节点 | metadata `resumed` + 下发同一 session id；卷宗一条「已接上该员工上次会话…」 |
| O4 | 无候选（普通新单） | **无** continuity 事件；行为与现网一致 |
| O5 | 同一 attempt 派发重入（activity 重试，含经由 §7.2 已绑定短路分支重入） | continuity 事件仍一条，不因走短路分支而漏写 |
| O5b | 同一任务两个不同 attempt（如首次 `resumed`、重试后候选过期变 `skipped`） | **各自一条** continuity 事件，内容分别对应各自 outcome，互不覆盖 |
| O6 | dossier API | timeline item 的 title/summary **不含**原始 event_type 蛇形串；中文可读 |
| O7 | 执行轨迹（若做） | 与 O1/O3 状态一致，可对上 session id |
| O8 | `verify:control-plane`（+ 相关 verify） | 全过；`git diff --check` 干净 |
| O9 | 父单终态 / 血缘 / 预检行为 | 与接续 spec G1–G8 无回归 |

**完成定义**：O1–O6、O8–O9 全过；O7 若 §14 纳入则必过。

---

## 11. 风险

| 风险 | 缓解 |
|---|---|
| 写在 queue 时无结论 | §4 钉死 bind 后写 |
| Append continuity 失败被吞 | §7.3 失败上抛 |
| 双路径只改一处 | 首次 start + queued start 同检 O1 |
| 文案与 7 天常量漂移 | summary 函数读同一常量 |
| 时间线噪音 | 默认仅 resumed/skipped；none 不写 |
| 契约枚举膨胀 | kind 可先 `other` |
| 幂等键若照抄 dispatched 用 task id 当 actor，会把 attempt 级去重误伤成 task 级、静默丢重试后的新结论 | ActorID 改用 attempt id（§5.3），O5b 专测 |
| continuity 写入失败上抛后，activity 重试会命中"已绑定"短路分支而非两条主路径，若该分支未挂写入逻辑则永久漏写 | §7.2 明确三处写入点，短路分支必须补上 |
| 共享工作树踩踏 | 显式路径 add；生成物外科手术 |
| 与「任务失败」混淆 | 文档与 UI：降级是 warn 叙事，不是 failed 任务 |

---

## 12. 开放细节（不阻塞立项；实施默认）

1. title 用「会话接续」+ summary 详情 vs summary 升主标题 → 默认前者。
2. `session_resume_status=none` 是否写入 metadata → 仅 `shouldAttemptSessionResume` 时写。
3. 执行轨迹字段名 → `session_resume_status` / `session_resume_label`（服务端给中文 label 更佳，前端零映射）。
4. systemconfig 化阈值 → 观察项，默认值不变。
5. 历史回填 → 不做。

---

## 13. 文档关系

| 文档 | 关系 |
|---|---|
| `2026-08-01-demand-continuation-design.md` | 父 spec；§6.1 预检与本 spec 留痕投影 |
| `2026-07-29-demand-workbench-design.md` | 卷宗容器；时间线叙事约定 |
| `event_narrative.go` | 中文叙事唯一映射源（项目事件） |
| 基线 `2026-07-27-workspace-and-playbook-alignment-baseline.md` | 不突破 §4 不变量；runtime 管道不在本批 |

实施完成后：本文状态改为「已实施」；父 spec §6.1 / CHANGELOG 链到本文。

---

## 14. 审阅检查清单（给人）

- [x] 同意本批只做**可观测性**，不改预检判据与 runtime 软降级
- [x] 同意 **bind 成功后**写 continuity 事件（§4）
- [x] 同意成功 `resumed` 与降级 `skipped` **对称**进卷宗
- [x] 同意默认 **none 不进**卷宗（§3.3）
- [x] 事件类型：选 **A 新类型** / A+kind=other / 其他：________
- [x] 本批是否包含**执行轨迹中文状态**：是
- [x] 同意跨项目接续（带 resume）**继续禁止**
- [x] 同意中文文案以 §3.2 为初值，实施期可微调用词但须单测锁定
- [x] 同意 O1–O9 为验收；真链至少 O1+O3

---

## 15. 建议评审结论写法

- **通过**：按切片 A 开发，拍板记入 §14 勾选结果。
- **有条件通过**：列出文案/kind/O7 差异后开发。
- **驳回**：若要求本批必须含 runtime 软降级或跨项目——应拆回切片 C，不阻塞 A。

---

## 16. 一句话给实施会话

> 在 `StartProjectTaskRun` 产出 `SessionResumeOutcome` 并写入 metadata/task_event；`Bind` 成功后追加一条 `project_task.session_continuity`（中文 summary）；卷宗只展示服务端文案；不碰 runtime、不碰跨项目、不抬默认阈值。
