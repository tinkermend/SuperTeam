# 恢复路径协调模式错位与员工活跃 run 毒化：修复方案

- 状态：R1+R2 已实现并全部验证（含 R1①③ fake provider 补验，落地记录见 §6）；R3 待排期
- 日期：2026-08-17
- 来源：交接包 H1b 真链验证发现（`2026-08-16-stage-handoff-package-design.md` 落地记录；TODO.md 2026-08-17 两条）。两个问题相互独立，拆开评审与实施。
- 范围：① 恢复替换任务（及一切动态任务）的协调模式解析错位；② provider 崩溃后员工活跃 run 毒化（含失败写回 500 泄漏与自愈缺口）。
- 修订（2026-08-17 评审后 v2）：决策表补 catch-all 行与 031→059 漏窗承认；§2.1 补命令级写回盲区（L1 消灭的不是 run 毒化的全部触发器）；L1 收窄 ErrNoRows 映射范围并扩至兄弟写回事务；L2 补终态族映射与 `sameIdempotentRun` 前插要求；L3 终态集显式化、宽限口径改挂 `finished_at`；L4 符号名修正；R1③ 验收措辞修正。

## 1. 问题一：恢复替换任务协调模式错位

### 1.1 因果链（代码证据）

1. `createRecoveryReplacementTask`（`project_store.go:2334`）的 `CreateProjectTaskRequest` **不设 `AcceptedPlanRevisionID`**——五个动态建任务路径（revision / 对抗返工 / 补做 / 扩编 / 恢复替换）中唯一不带的。
2. `InspectTaskResultDecision`（`project_store.go:1044-1052`）模式解析：`AcceptedPlanRevisionID == nil` → 默认 `loop`。注意：该处代码注释只声明了「修订可读但 mode 为 nil」与「修订读不出」两种情况的意图，**任务修订指针为 nil 时落 loop 是未被注释声明的默认行为**——它把"恢复替换没带修订"与"史前计划（059 前无模式列，行为就是自动补链）"混为一谈。
3. 后果：plan 模式需求的任务崩溃 → 人类批准恢复 → 替换任务 blocked 申报 → 协调线程按 loop 自动补链，**绕过 plan 模式的人工卡承诺**。真链实证：demand `3b644046`（plan 模式）走了 loop 补链。

### 1.2 根因定性

模式本质是**需求级属性**（059 设计：随需求提交冻结进 plan revision；`project_demands.coordination_mode` 列 NOT NULL 自 059 起存在）。现在的"任务 → 计划修订 → 模式"是条间接代理链，对动态任务天然脆弱：今天是恢复替换忘了继承，未来任何新动态路径同样会踩。

### 1.3 方案

**P1-A（语义修复，主）**：`InspectTaskResultDecision` 的模式解析改为以需求为权威，决策表如下（自上而下首个命中；"修订可读"沿用现有 `GetPlanRevision` 错误语义，读失败 ≠ 无修订）：

| # | 条件 | 模式 | 理由 |
|---|---|---|---|
| 1 | 修订指针非 nil 且修订可读 | 按修订（mode nil → loop，§8.4 回兼容；现状不变） | 现代链路主路径，不动；此行**不需要读 demand** |
| 2 | 修订指针非 nil 但读失败 | `plan` | 现状的保守回退保留（宁可多问人） |
| 3 | 指针 nil、demand 可读、该 demand 图上**存在任何**带修订的任务 | 按 `demand.coordination_mode` | 动态任务（恢复替换等）：demand 是 tri-mode 后的，模式以需求为准 |
| 4 | 指针 nil、demand 可读、全图无修订 | `loop` | 史前需求（024 列引入前无修订指针、059 前无模式列，回填 'plan' 不是其真实语义），保持 §8.4 回兼容 |
| 5 | 指针 nil、demand 不可读 | `plan` | catch-all：保守回退，覆盖「修订 nil + demand 读失败 / `DemandID`·`CoordinationJobID` 为 nil 无法判图」的组合 |

实现要点：

- demand「可读」= `GetProjectDemand` 成功；`ProjectDemand.CoordinationMode` 是非指针 string（值域 plan/loop，059 CHECK）。
- `InspectTaskResultDecision` 已持有 task（含 `DemandID` 与 `CoordinationJobID`）；规则 3/4 的判定加一次 `GetProjectDemand` 与一次 `ListProjectTasksByCoordinationJob`（仓库方法均现成）。协调信号路径上这两个查询频率低，成本可接受。`task.CoordinationJobID` 为 nil 的老任务无法判图，按规则 5 处理。
- 表是语义判定表；实现按「先看任务修订指针」短路，规则 1 不触发 demand 查询。

**P1-B（数据一致性，辅）**：`createRecoveryReplacementTask` 补 `AcceptedPlanRevisionID: source.AcceptedPlanRevisionID`，与其余四个动态路径对齐。独立价值：派发闸 `task.accepted_plan_revision_changed` 漂移检测对 nil 是跳过的（`predispatch_gate.go:218`），补上后替换任务同样受计划漂移保护。`findExistingRecoveryReplacement` 的幂等匹配键（PlannedTaskKey + planner metadata）不受影响；也因此 **Temporal 重放复用既有 nil 修订行时 P1-B 不回填该行**——存量/重放命中的替换任务由规则 3 兜底。

> P1-A 与 P1-B 双管齐下后：规则 3 命中的场景从此带修订直接走规则 1；规则 3 本身作为兜底继续覆盖未来漏继承的新路径与存量行。

### 1.4 风险与边界

- 行为变化面：仅"plan 模式需求 + 动态任务 + blocked 申报"这一组合从自动补链变为人工卡——正是被绕过的语义承诺本身；loop 需求行为不变。
- Temporal 回放：`InspectTaskResultDecision` 是 activity（非 workflow 内联决策），旧 history 重放不受影响；workflow 分支（`coordination-mode-branch` 栅栏）不动。
- 旧数据：规则 4 只保证**全图无修订**的史前需求不因回填值改变行为。存在一个残余漏窗：**031→059 之间创建的需求**（图上有修订、demand 无真实模式，回填 'plan'）若再出现指针 nil 的动态任务，规则 3 会把 loop 误升 plan（保守方向）。该窗口属早期开发期数据、开发库已于 07-2x B 档清空，实际风险≈0，接受不改表。

## 2. 问题二：provider 崩溃后员工活跃 run 毒化

### 2.1 因果链（代码证据）

**attempt 级失败写回泄漏（真链已实证）**：

1. **崩溃**：provider 进程在事件流未关时退出（`provider process exited while the event stream was still open`，真链 demand `1207acc4`/`84110bad` 两次实证）。
2. **写回 500**：runtime 的失败写回 `POST /runtime/project-task-attempts/{id}/fail` → `Service.FailProjectTaskAttempt`（`service.go:5580`）→ 事务 `RecoverProjectTaskAttemptFailureWriteback`——事务内守卫 UPDATE（`FinishProjectTaskAttempt` 等，`:one` + `WHERE status IN ('queued','running') AND lease_token`）在并发取代窗口 0 行命中，`pgx.ErrNoRows` 未收敛为域错误，裸传至 handler 500 兜底（响应体是 "internal server error"；`no rows in result set` 出现在 CP 服务端日志 `handler.go:2333`）。入口校验层（`validateAttemptRuntimeRequest`，`service.go:9113-9117`）对"attempt 已被取代/任务不再接受写回"**已有正确的 409 语义**——但它在事务外（读-判-写 TOCTOU），挡不住校验后、事务前的取代。
3. **重试被丢弃**：runtime 持久重试拿到 `409 superseded` 后按设计丢弃——终态永远没落到 run 行。
4. **毒化**：run 行停留 `queued/dispatching/running/cancelling` → `GetActiveDigitalEmployeeRun`（`tasks.sql:267`）对同员工每次新派发返回 `employee conflict`（`run_service.go:605`）。

**结构性盲区（评审补充：L1 修不掉的毒化通路）**：run 行终态化的真正载体是**命令级写回** `POST /runtime/commands/{id}/fail`（`recordTerminalLocked` → `UpdateRunStatus`），而：

- runtime 持久写回队列只覆盖 attempt 级（`ProjectTaskAttemptWritebackKind` 仅 Complete/Fail/WaitHuman），**命令级终态写回没有任何持久重试**；
- provider 崩溃路径（`executor.rs:1747`）是 `let _ = writeback.fail_with_envelope(...)` 吞错；且 `fail_with_envelope` 先发 command-fail（`?` 短路）再发 attempt-fail——**command-fail 瞬时失败时 attempt-fail 根本不会发**。

即：一次 command-fail 的网络抖动/5xx 就足以毒化 run 行，与 attempt 写回是否 500 无关。**L2/L3（attempt 终态 = 死亡证据）才是 run 行的真正防线**；L1 消灭的是 attempt 侧的 500 噪音触发器与写回黑洞，不是毒化本身。

### 2.2 现有自愈为何没接住（逐项对照）

| 机制 | 位置 | 缺口 |
|---|---|---|
| 派发时回执对账 `reconcileTerminalReceipt` | `run_service.go:1496` | 仅当 `runtime_command_receipts` **已有终态回执**才收口；崩溃场景回执未到终态（含命令写回根本没送达），无米下锅 |
| 预确认清扫 `isStalePreConfirmationRun` | `run_service.go:1531` | 只扫 queued/dispatching + TTL；注释明确排除 running/cancelling（"真实活跃态"）——该假设在"runtime 知道进程死了但写不回"时破产 |
| 交叉核对 | **不存在** | "run 行活跃 × 其 project attempt 已终态"是确定性死亡证据，无任何机制检查；CP 侧两个任务看门狗（`SweepExpiredRunningProjectTaskAttempts` / `SweepStuckOrphanProjectTasks`）都只动 task/attempt，不碰 run 行 |

### 2.3 方案：分层防御

**L1 修触发器（必做）**：审计 `RecoverProjectTaskAttemptFailureWriteback` 事务内的 no-rows 泄漏点，将守卫 UPDATE（`FinishProjectTaskAttempt` / `ScheduleProjectTaskRetry` / `MoveProjectTaskToWaitingHuman` / `UpdateProjectTaskStatus`）的 0 行命中收敛为 `ErrProjectConflict`（409）——runtime 对 409 superseded 的处理是成熟正确的，500 才是它循环重试的元凶。**按语句收窄映射，不要在事务边界 blanket-map `ErrNoRows`**：`appendProjectEventWithQueries` 内 `GetLatestProjectEventSequence` 等读失败若一并映射，会伪装成 superseded。同族泄漏不止 fail 事务——complete/result/wait-human 写回事务同构裸传（如 `completeProjectTaskAttemptWritebackWithQueries:5122`），本期至少一并审计（修 fail 是本 incident 正主，兄弟路径修不修视审计结论）。定位手段：fake provider（`scripts/e2e/fake-providers/`）制造崩溃 + 并发取代窗口复现；修复后该路径 409。

**L1.5（明确不做，记 TODO）**：把命令级终态写回纳入 runtime 持久重试队列。它能从源头减少毒化，但 L2/L3 已兜底、且改 runtime 队列面更大——本期不扩，TODO.md 留一行。

**L2 派发时自愈（性价比最高）**：`CreateRun` 的活跃 run 冲突路径（`run_service.go:575-607`）在回执对账失败后加一层死亡证据核对：

```
activeRun 冲突时：
  1. reconcileTerminalReceipt（现状）→ 终态则放行
  2. 新增死亡证据核对（必须插在 sameIdempotentRun 之前）：
     解析 run.CommandID → runtime_command_receipts.payload->'metadata'->>'project_task_attempt_id'
     （派发期 runMetadata 写入，H1a 真链已实证该字段存在于回执 payload）
     → 查该 attempt 状态，已终态且过宽限 → UpdateRunStatus（终态按 attempt 终态族映射，见下）+ 观测事件 → 放行本次派发
     解析不出 attempt id（诚实边界）→ 跳过本层
  3. sameIdempotentRun（现状）→ 同幂等重试走 resume 分支
  4. isStalePreConfirmationRun（现状）→ TTL 清扫
  5. 否则维持冲突（现状）
```

实施要点：

- **插在 `sameIdempotentRun`（现状 `run_service.go:592`）之前**：否则同幂等重试先走 resume 分支，绕过死亡核对。
- **run 终态按 attempt 终态族映射**：`succeeded→completed`、`failed/lost/timed_out→failed`、`cancelled→cancelled`。固定写 failed 会把"attempt 已成功、命令还在收尾"的 run 记成失败，run 列表观感错误。
- 跨域注入沿真先例 `SetProjectDispatchFactsReader` + app 装配适配器（`app.go:737`）：可选接口（如 `RunAttemptTerminalChecker`）由 app 装配时用 project 域实现（`GetProjectTaskAttempt` 现成），nil 时跳过，既有测试 fake 不必实现。
- 宽限：attempt 终态即死亡证据，**不依赖时钟**；仅对"终态发生至今"（`pta.finished_at`）设短宽限容忍在途的命令收尾/对账/写回（取值见拍板点 3）。

**L3 看门狗兜底**：清扫器家族新增交叉核对扫描（SQL 草图）：

```sql
-- name: ListOrphanedActiveDigitalEmployeeRuns :many
-- 终态集显式：waiting_human 是否纳入见拍板点 5；若纳入，其 finished_at 可能为 NULL，
-- 宽限条件需 COALESCE(pta.finished_at, pta.updated_at)。
SELECT tr.*
FROM task_runs tr
JOIN runtime_command_receipts rcr
  ON rcr.tenant_id = tr.tenant_id AND rcr.command_id = tr.command_id
JOIN project_task_attempts pta
  ON pta.id::text = rcr.payload->'metadata'->>'project_task_attempt_id'
WHERE tr.status IN ('queued','dispatching','running','cancelling')
  AND pta.status IN ('succeeded','failed','cancelled','lost','timed_out')
  AND pta.finished_at < now() - interval '5 minutes'
LIMIT 100;
```

与 v1 草图的两处修订：终态集显式定为 `{succeeded,failed,cancelled,lost,timed_out}`；宽限口径从 `tr.updated_at`（行触摸）改为 **`pta.finished_at`**（与 L2 同源——长工件上传期间 run 行不被触摸也不会误扫）。收口动作复用 `UpdateRunStatus`（终态映射同 L2）；挂在既有看门狗周期（`app.go:1080`，1 分钟一轮）。**关键原则：以 attempt 死亡证据为准而非时钟**——真活跃 run 的 attempt 必然非终态，不会被误扫，这绕开了"running 是真实活跃态"假设中正确的部分。

**L4 恢复卡时序（排查项，单列）**：派发失败恢复卡（`project_task_recovery`）批准后，观察到 waiting_human 任务仍被派发并终态拒绝 `invalid project`（demand `70b88ff0`，05:31:57 dispatch_blocked）。怀疑批准路径与工作流重派之间有时序或释放失败。真实符号：决策类型判别 `isTaskHumanWaitRedispatchDecisionType`（`service.go:6966`）+ 释放事务 `ReleaseProjectTaskWaitingHumanForRedispatch`（`project.sql:2199`：清 current_attempt_id/run 绑定、回 planned，走协调线程正常派发管线）——v1 引用的 `taskHumanWaitDecisionActions`/`applyTaskHumanWaitRelease` 是注释里的说法，不是可 grep 的符号。本期仅排查 + 复现 + 报告，修复视结论定（可能随 L1 消失）。

### 2.4 风险与边界

- L2/L3 的 run→attempt 关联依赖回执 payload 的 `metadata.project_task_attempt_id`——只有经现代派发管线（`projectTaskRunPrompt` 路径）创建的 run 才有；历史/非项目 run、`task_runs.command_id` 为 NULL 的老行解析不出即自然跳过（诚实边界，不误伤）。
- L3 误扫面：attempt 终态是充分死亡证据；宽限 5 分钟挂 `finished_at` 与滞留看门狗口径对齐。若极端情况下 attempt 被错误终态化（另一缺陷族），L3 会把 run 一并收口——两行一起错比一行活一行死更可诊断。
- error_family 词表：收口 run 建议复用现有家族（如 `execution_failed`）不加新词，避免 openapi/状态标签联动；如需区分观测，用 error_code 承载（开放拍板点 2）。

## 3. 分期与验收（done looks like）

| 期 | 内容 | 验收（真链） |
|---|---|---|
| **R1** | P1-A + P1-B + L1（含兄弟写回事务审计） | ① 复现 `3b644046` 场景（plan 模式 + fake provider 崩溃 + 批准恢复 + blocked 申报）断言落 `upstream_supplement_review` 卡而非补做；② loop 模式同场景回归断言补链照常；③ fake provider 崩溃 + 取代窗口下失败写回返回 409（非 500），runtime 写回队列最终收敛（409 后 live 路径仍会入队一次、下轮 drain 丢弃，属预期），无 5xx 循环重试 |
| **R2** | L2 + L3 | ① fake provider 崩溃制造毒化 → 不经任何人工介入，同员工下一次任务派发成功（L2 内联自愈）且 run 行落终态；② 停用 L2（仅 L3）时看门狗一轮内（分钟级）run 行落终态；③ 正常长任务运行中看门狗不误扫（attempt 非终态） |
| **R3** | L4 排查 + H1b 遗留补验 | ① L4 复现报告与结论（修或转 TODO）；② fan-in 双槽双源真链（`70b88ff0` 场景回归）；③ plan 模式卡真链（`3b644046` 场景） |

R1 独立可先发；R2 与 L1 无技术依赖（L2/L3 的死亡证据不经过 attempt 写回是否成功），但按 R1→R2 顺序验证更干净——先消灭 500 噪音，E2E 判据才不被污染；R3 收尾。

## 4. 测试策略

**单元（Go）**：模式解析决策表五行逐行覆盖（含 demand/修订/全图修订的各种可读性组合，**含规则 5 catch-all：指针 nil + demand 不可读**）；替换任务携带修订断言；L2 核对的纯逻辑（回执 payload 解析、终态判定、**attempt 终态族 → run 状态映射**、宽限）；L3 SQL 用 testenv 建三表夹具（活跃 run + 终态 attempt + 宽限过期/未过期）。
**契约**：无 openapi 变更预期（error_code 而非新枚举；若拍板新增 family 则走 openapi 两处 + `generate:control-plane` + `verify:contracts`）。
**真链**：`scripts/e2e/fake-providers/` 崩溃脚本（复用 `provider process exited` 制造手段）+ PulseAI 项目探针 demand，按 §3 各期判据。

## 5. 开放拍板点

1. **P1-A 决策表**：规则 3/4 的分界（"全图无修订=史前"）是否接受；替代方案是给 `project_demands` 加"模式生效于 059 之后"的判定列（更精确但动迁移，不建议——031→059 漏窗实际风险≈0，见 §1.4）。
2. **L2/L3 收口的 error 词表**：复用现有 family + error_code 区分（建议），还是新增 `orphaned_run` family（需 openapi/状态标签联动）。
3. **宽限取值与口径**：L2/L3 统一挂 `pta.finished_at`；L2 建议 ≥2 分钟（attempt 终态→命令终态的正常窗口是秒级，但大工件上传可到分钟级，2 分钟是下限不是定数）；L3 5 分钟对齐滞留看门狗。
4. **L4 是否纳入本期**（建议：排查纳入 R3，修复视结论）。
5. **L3 终态集是否含 `waiting_human`**（新增）：倾向纳入（释放必走新 attempt、旧 run 不会再进展），但它是活会话暂停态，纳入即接受"人类批准前 run 行已收口"的观感——需拍板。

## 6. 落地记录（2026-08-17，R1+R2）

拍板点按建议倾向执行：#1 决策表照发；#2 复用 family + `error_code=orphaned_run_reaped`；#3 L2 宽限 2 分钟、L3 5 分钟，统一挂 `finished_at`（L3 SQL 用 `COALESCE(finished_at, updated_at)`）；#5 `waiting_human` 纳入 L3 终态集并纳入 L2 终态族映射（→ run completed）。

**真链已验证**：
- L2（验收 R2①）：外科构造完整毒化签名（run=running + 回执=dispatched 非终态 + attempt=succeeded 终态超宽限）→ 同员工聊天 run 创建 **201 成功**（未被 `employee conflict` 挡住）→ 内联收口（日志 `orphaned run reap: run … closed as completed`）→ 终态族映射正确（attempt succeeded → run completed，未误标 failed）。
- L3（验收 R2②）：构造毒化（attempt 终态 >5 分钟）→ 看门狗 **48 秒**内收口，`error_code=orphaned_run_reaped`，按终态族映射。
- 不误扫（验收 R2③）：验证期间 develop 真活跃 run（attempt 非终态）全程未被两层收口触碰。
- P1-B：崩溃恢复批准后新建替换任务 `develop#2` 的 `accepted_plan_revision_id` 与源一致（对照修复前同场景为 NULL）——崩溃替换路径从此按决策表 r1 解析模式。

**单测覆盖**：决策表五行（`TestResolveTaskCoordinationModeDecisionTable`，含规则 5 catch-all）；替换携带修订（`TestRecoveryReplacementCarriesAcceptedPlanRevision`）；守卫收敛（`TestGuardWritebackConflict`）；L2 映射/解析/收口四分支（宽限内、非终态、未注入 checker 均不收口）；L3 清扫三分支（命中、二次核验跳过、无 lister 零操作）。旧行为断言 `no accepted plan revision → loop` 已按规则 5 更新为 plan 并注明出处。

**自查修订（2026-08-17 同日复审）**：① L2 补齐方案明确要求而首版遗漏的观测事件——`run_reaped_orphaned`（生命周期序号 -4，`CreateTaskEventIfAbsent` + 审计，对齐 `reapStaleRun` 先例），L2/L3 两处共用；② L2 适配器对 `finished_at` 为 NULL 的 waiting_human attempt 回退 `updated_at`，与 L3 SQL 的 COALESCE 口径对齐（首版两层不一致：L3 收口而 L2 跳过）；③ 补 §4 要求的三表夹具集成测试 `TestListOrphanedActiveRuns`（TEST_DATABASE_URL 门禁、schema 隔离，真库通过：命中/非终态/宽限未到/waiting_human-COALESCE/已收敛干扰项五形态）——顺带发现 `update_project_task_attempts_updated_at` 触发器会覆盖事后回拨，夹具必须写入时指定；④ L1 兄弟审计收口：`ResolveProjectTaskHumanWaitWriteback`（经已映射的助手）与 `QueueProjectTaskWithAttempt`（自带锁与冲突语义）无同族泄漏；⑤ 收口为 completed/cancelled 的 run 不再带 `error_family`（只有 failed 收口带 `execution_failed`），避免 run 列表错误归因误导；⑥ L2 验证时外科翻转的命令回执已还原。

**R1①③ 补验完成（2026-08-17 下午，fake provider 路径——§4 规定的验证方式，真 LLM 529 与此无关）**：
- R1① 核心：plan 模式需求（demand `eec0a154`）+ 假 provider 崩溃/申报脚本（`任务标题:` 行锚定分流，避免 notes 需求回传段的"审查"字样误判）：develop 完成（deliverables+判据）→ review 输出 blocked 契约（`missing_inputs=["risk_notes"]` 白名单接受）→ **落 `upstream_supplement_review` 卡、零补做任务**（demand `4c6b1e84` 复验：崩溃分类为可重试 → 同任务自动重试 → 重试轮申报 → 同样落卡）。
- R1① 替换腿：恢复替换路径的构成性修复已各自真链验证（P1-B 替换携修订 + Leg A 的携修订任务申报→卡），携修订任务的模式解析同为 r1 代码路径，不再重复构造非重试崩溃形态。
- R1③：`TestFailWritebackGuardConflictOnSupersededAttempt` 真库集成测试（取代窗口：校验放行 + 守卫 UPDATE 0 行）断言收敛 `ErrProjectConflict`（409），通过。
- 验证期假 provider 踩坑记录：假 provider 必须应答 `--version`（README 已有）；验收判据在 handoff_contract 无 acceptance_criteria 时回退 expected_outputs（判据文本须按任务实际判据写）。

R2 验收②的"停用 L2 仅 L3"对照未单独构造（L2/L3 同宽限源，L3 已独立验证）。
