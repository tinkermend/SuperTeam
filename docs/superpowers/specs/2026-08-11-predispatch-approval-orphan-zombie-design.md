# 预检闸批准后状态机与 orphan 僵尸审批卡

- 日期：2026-08-11
- 状态：**已实施（单测绿；E2E 待补跑高风险闸 + provider_unavailable）**
- 来源：`2026-08-10-retry-redispatch-and-failure-attribution.md` §6 深挖；provider 语义复查期 E2E 阻塞（§18-3 预算熔断腿）
- 优先级：**中高**——高风险任务在 runtime 抖动 / 假 provider 场景下必现；挡住后续 E2E 取证
- 迁移：预期**零库表迁移**；可能需要一次性存量回挂脚本（非 DDL）

---

## 1. 现象（真实链路，非推断）

### 1.1 人类视角

高风险预检闸任务需要人类批准后才能派发。实测：

1. 人类在收件箱/审批中心**真实批准**了闸卡；
2. 任务仍停在 `waiting_human` / `approval_required`，`attempts=0`；
3. 同一任务上出现**两张** `project_task_approval` 决策卡——一张已批，一张仍 pending，且 pending 卡的 `approval_request_id` 为**全零 UUID**。

### 1.2 钉死样例

| 任务 | 真闸卡 | 僵尸卡 | gate 终态 |
|---|---|---|---|
| `b449a6d5-562b-40df-9a32-e9fab9ed1140` | `19f46fdc…` approved，approval=`01f97a9c…`，gate=`31d11b21…` | `483f806a…` pending，approval=`0000…`，无 gate | `retry_later` / `runtime.provider_unavailable` |
| `a17e0214-5dd5-42fe-8af7-4c776d39f423` | 同形 | 同形 | 同形 |
| `b9ce0207-e840-44a4-8900-422259207b15` | 同形 | 同形 | 同形 |

`b449a6d5` 时间线（2026-08-10）：

| 时间 | 事实 |
|---|---|
| 21:12:29 | gate 判高风险 → 真卡 + approval + gate 行 |
| 21:16:12 | 人类批准真 approval；21:16:13 真卡 → `approved` |
| 21:16:15 | gate 重评 → **`retry_later`**（`runtime.provider_unavailable`）；事件 `dispatch_gate.retry_later` + 多次 `dispatch_blocked` |
| 21:16:39 | orphan 看门狗补建僵尸卡并**改挂** `waiting_request_id` |

### 1.3 全零 UUID 的出处

库内全部 `project_task_approval` 且 `approval_request_id = 00000000-…` 的摘要一致：

> `系统补建人工决策卡：任务已停在待人工确认，但缺少可处理的决策（原因：需要人工审批）`

**唯一生产者**：`Service.repairOrphanWaitingHumanProjectTask`（`SweepOrphanWaitingHumanProjectTasks`）——硬编码 `ApprovalRequestID: uuid.Nil`，不创建 `approval_requests` 行，不绑 `dispatch_gate_result_id`。当 `waiting_reason = approval_required` 时 `DecisionType` 映射为 `project_task_approval`，伪装成预检闸审批卡。

**不是** `createGateHumanAction` 主路径写坏（主路径会 `findOrCreateGateApprovalRequest` 并写入非零 approval）。

---

## 2. 根因（四条叠加）

### 2.1 A — 门闸批准后状态机不清 `waiting_human`

`ApplyPreDispatchGateDecision` 对**已绑定** `dispatch_gate_result_id` 的 `project_task_approval` + `approved`：

- 只返回 `ReadyTaskIDs`，让 workflow 再跑 `DispatchProjectTask`；
- **不**调用 `ReleaseProjectTaskHumanWaitForRedispatch` 清掉 `waiting_human` / `waiting_request_id`。

`projectTaskDispatchAllowed` 故意允许 `waiting_human` 上继续 Dispatch（gate 等待期间可再派）。这本身可接受，但：

当随后 `RunPreDispatchGate` 落到 **`retry_later`**（仅 `recordDispatchBlocked` 写事件 + 返回 `ErrProjectTaskDispatchRetryLater`）时：

- 任务**继续停在** `waiting_human`；
- `waiting_request_id` **仍指向已 approved 的真卡**。

于是出现合法但危险的中间态：「任务 waiting_human，指针却指着一张非 open 的决策」。

### 2.2 B — orphan 列表把中间态当孤儿

`ListOrphanWaitingHumanProjectTasks`（`project.sql`）：

```sql
status = 'waiting_human'
AND (
  waiting_request_id IS NULL
  OR NOT EXISTS (
    -- 指向的决策仍是 open
    lower(status_snapshot) IN ('pending', 'waiting', 'requested', 'open')
  )
)
```

「真卡已批、任务仍 waiting_human」**精确命中**第二条 → 进入补建路径。

`GetOpenProjectDecisionRequestByTask` 只找 open 卡 → 找不到 → `CreateDecisionRequest(ApprovalRequestID: uuid.Nil)` → 僵尸卡 → `BindProjectTaskWaitingRequest` **改挂**指针。

### 2.3 C — 补建卡对 `approval_required` 不可操作 / 误操作

| 人类动作 | 结果 |
|---|---|
| 再批**真卡**（审批中心 / 已批 approval） | `waiting_request_id` 已是僵尸；`applyTaskHumanWaitRelease` 在 `waiting_request_id != decision.ID` 时 **no-op** |
| 批**僵尸卡** | 理论上可 release→再派发；文案像系统噪音，且再派发仍可能 `provider_unavailable`→`retry_later`，状态机不修好会再循环 |

### 2.4 D — `retry_later` 不进入恢复路径（审查追加）

> 此根因在初版方案中被遗漏。初版 §4.1 step 3 假设「由既有 `scheduleDispatchRetry` / 恢复路径自动再派发」，但代码追踪证明该假设不成立。

`DispatchProjectTask` 对 `retry_later` 返回**裸** `ErrProjectTaskDispatchRetryLater`（`project_store.go:3304`），不是 `*ProjectTaskDispatchError`：

```go
case gate.Retryable:
    return ErrProjectTaskDispatchRetryLater
```

Activity wrapper（`activities.go:327`）因 `dispatchErrorRetryable` = true 不包 NonRetryable，Temporal 按 retry policy 重试 3 次（~7s）。全部失败后错误传到 workflow 的 `dispatchProjectTasks`（`workflow.go:1411`）：

```go
if !dispatchFailureRecorded(err) {   // ← false：ErrProjectTaskDispatchRetryLater 不是 *ProjectTaskDispatchError
    if dispatchFailureTerminal(err) { continue }  // ← 也不匹配
    return err                        // ← 走这里
}
// RecoverTaskDispatchFailure 分支永远不会到达
```

`dispatchFailureRecorded`（`types.go:536-548`）只认 `*ProjectTaskDispatchError` 或 Type `"ProjectTaskDispatchError"` 的 ApplicationError——裸 sentinel error 两者都不是。

**后果**：即使修了 A（清 `waiting_human` → 任务回到 `planned`），若 gate 紧接着落 `retry_later`，`RecoverTaskDispatchFailure` 不会被调用，`scheduleDispatchRetry` 不会被调用，`recordDispatchBlocked` 也不设 `retry_not_before`（只写事件）。任务停在 `planned`，协调线程纯信号驱动无定时器扫 `planned` 任务——**从「假等人」变成「沉默搁置」**。

---

## 3. 目标与非目标

### 3.1 目标

1. 人类批准**真**高风险闸卡后：任务不得永久卡在 `waiting_human` 且指针挂在已 approved 卡上。
2. orphan 补建**不得**再制造全零 `approval_request_id` 的 `project_task_approval` 僵尸卡。
3. 存量僵尸可被一次性/看门狗收敛，任务能回到可派发或诚实退避。
4. `retry_later`（含 `runtime.provider_unavailable`）与「等人」语义分离：前者进入恢复路径自动重试（有退避唤醒），后者必须有可操作的 open 决策卡。
5. `retry_later` 后任务不得沉默搁置于 `planned`——必须有 `scheduleDispatchRetry` 定时唤醒或转 `waiting_human` 真 human card。

### 3.2 非目标

- 不重做收件箱 / 审批中心 UI。
- 不改高风险闸业务判据（何时需要人类批准）。
- 不解决 runtime 真不可用本身（那是运维/provider 健康问题）；只解决「不可用时状态机与孤儿补建把任务做死」。
- 不并入 `2026-08-10-retry-redispatch-and-failure-attribution` 的实施范围（根因不同，已关闭）。
- 不改 `WaitHumanProjectTaskAttempt`（runtime 中途暂停）路径——它也用 `ApprovalRequestID: uuid.Nil` 造决策卡（`service.go:5554`），同一缺陷家族但属执行期不在预检闸范围内；§4.2 的硬约束应在实施时确认覆盖面或另案处理。

---

## 4. 方案

### 4.1 批准 gate 链接卡后的状态机（主修 A）

对 `project_task_approval` 且 `DispatchGateResultID` 非空 + 人类 `approved`：

**推荐（拍板倾向）**：

| 步骤 | 行为 |
|---|---|
| 1 | 在返回 `ReadyTaskIDs` **之前或同时**，将任务从 `waiting_human` 释放到可派发态（`planned` 或等价），**清空** `waiting_request_id` / `waiting_reason`（或仅当指针指向本 decision 时清空） |
| 2 | 再 `DispatchProjectTask` |
| 3 | 若 gate 变为 `retry_later`：任务保持 **非** `waiting_human`（例如 `planned` + `retry_not_before`），由 §4.5 的恢复路径接入自动再派发；**不要**把已批 decision 重新挂回 `waiting_request_id` |

这样 orphan 条件「waiting_human + 非 open 指针」在正常批准路径上**不再被制造**。

实现上最简路径：让 gate-linked approved 也走 `applyTaskHumanWaitRelease`（它已经 `ReleaseProjectTaskHumanWaitForRedispatch` → `planned` + 返回 `ReadyTaskIDs`）。当前代码在 `predispatch_gate.go:322-329` 对这类卡穿透 switch 直接返回 `ReadyTaskIDs`——只需改为先 release 再返回。`applyTaskHumanWaitRelease` 的 `waiting_request_id` 匹配检查（`project_store.go:1888`）会通过，因为此时指针仍指向正被 resolve 的真卡。

备选（若释放语义牵动过大）：保留 `waiting_human`，但批准后立即把 `waiting_request_id` 置 NULL，并保证 gate `retry_later` 不依赖「指针指向 approved 卡」。orphan 列表同步排除「存在同 task 已 approved + gate 链接的 `project_task_approval`」。

### 4.2 orphan 列表与补建（主修 B/C）

**列表收紧**（`ListOrphanWaitingHumanProjectTasks` 或 service 层过滤，二选一，优先 SQL 一次做对）：

对候选任务，若存在：

```text
project_decision_requests
  decision_type = 'project_task_approval'
  status_snapshot = 'approved'  -- 或 resolved 族
  dispatch_gate_result_id IS NOT NULL
  project_task_id = 本任务
```

则 **不得** 当作「缺卡孤儿」去 `CreateDecisionRequest`。

正确动作（按优先级）：

1. **Signal 再派发**（`ProjectTaskRetryScheduled` 或 `DispatchReasonHumanResolved` / `Retry`），依赖 4.1 的 durable risk grant（`preDispatchRiskApprovalGranted` 已扫同 task 的 approved gate 卡）；
2. 若 gate 明确仍是人类闸且**没有**任何 approved gate 卡：才允许补建，且必须走 **真** approval 创建（非零 `approval_request_id` + 尽量回挂当前 gate result）；
3. 若仅 runtime 瞬时不可用：转 `retry_later` 退避，**不要**开 `project_task_approval` 卡。

**补建硬约束**（`repairOrphanWaitingHumanProjectTask`）：

| waiting_reason / 映射 DecisionType | 允许的补建 |
|---|---|
| `approval_required` → `project_task_approval` | **必须** `approvals.CreateRequest` 得非零 ID；否则不得用该 DecisionType，应降级为 `project_task_clarification` 并写清「系统无法重建审批对象，请从任务详情处理」 |
| 其他 recovery / clarification | 可继续零 approval（本就不是审批对象），但摘要不得伪装成预检闸风险批准 |

禁止：`ApprovalRequestID: uuid.Nil` + `DecisionType: project_task_approval`。

### 4.3 `applyTaskHumanWaitRelease` 匹配

现状：`waiting_request_id != decision.ID` → no-op。

在 4.1/4.2 落地后，主路径不再依赖「批僵尸卡」。仍建议：

- 若人类批准的是**同 task 上带 gate 的 approved/刚刚 resolved 的真卡**，即使 `waiting_request_id` 已指僵尸，也应 **优先认 durable grant 并 redispatch**，而不是 silent no-op；
- 或：发现指针指向「系统补建」卡且存在真 gate 已批卡时，自动 rebind 回真卡并触发 4.1。

### 4.4 存量收敛

一次性（看门狗一趟或运维脚本）扫描：

```text
project_tasks.status = waiting_human
waiting_request_id → decision.summary 含「系统补建人工决策卡」
同 task 存在 approved + dispatch_gate_result_id 非空的 project_task_approval
```

动作：

1. 将僵尸卡标 `cancelled` / `superseded`（或等价终态，避免收件箱继续展示）；
2. 清空或改挂 `waiting_request_id`；
3. 任务落到 `planned`（或保持可派发态）并 signal 再派发。

存量扫描的僵尸卡匹配条件建议**三重校验**（比初版更精确，即使文案被未来修改仍可靠）：

1. `approval_request_id = 00000000-0000-0000-0000-000000000000`；
2. summary 前缀匹配「系统补建人工决策卡」；
3. 同 task 存在 approved + `dispatch_gate_result_id` 非空的 `project_task_approval` 卡。

不要求手工逐条点批僵尸卡。

### 4.5 `retry_later` 恢复路径接入（主修 D）

> 初版方案遗漏此节。根因 D（§2.4）证明：不修此节，fix A 只是把死法从「僵尸卡」换成「沉默搁置于 planned」。

**问题**：`DispatchProjectTask` 对 gate `retry_later` 返回裸 `ErrProjectTaskDispatchRetryLater`，不是 `*ProjectTaskDispatchError`。`dispatchFailureRecorded`（`types.go:536`）不认它 → `dispatchProjectTasks`（`workflow.go:1411`）走 `return err` → `RecoverTaskDispatchFailure` 分支永不到达 → 无 `scheduleDispatchRetry`、无 `retry_not_before`。

**推荐修法**（单点改动，语义一致）：

在 `DispatchProjectTask`（`project_store.go:3298-3309`）的 `retry_later` 分支，把裸返回包成 `*ProjectTaskDispatchError`：

```go
case gate.Retryable:
    return &ProjectTaskDispatchError{FailureRecorded: true, Err: ErrProjectTaskDispatchRetryLater}
```

`recordDispatchBlocked` 已经写了事件（= failure 已记录），`FailureRecorded: true` 语义正确。

这样 `dispatchFailureRecorded` → true → workflow 进入 `RecoverTaskDispatchFailure`（`workflow.go:1426-1437`）→ `projectTaskDispatchRecoveryAction` 根据失败计数决定：

- `retry_scheduled`：设 `retry_not_before` + `scheduleDispatchRetry` 定时唤醒；
- 达到上限：转 `waiting_human` + 真 human card。

**需确认的接入点**：`RecoverProjectTaskDispatchFailure`（`service.go:5922`）在 `task.Status == waiting_human && task.WaitingRequestID != nil` 时返回 Noop（`service.go:5940-5941`）。fix A 后任务处于 `planned`（非 `waiting_human`），不会被这条早退拦截。但需确认 `projectTaskDispatchRecoveryAction` 对 gate 级 `retry_later`（非 attempt 级失败）的计数语义正确——当前 `CountProjectTaskDispatchFailureEvents` 统计 `dispatch_failed` 事件，而 `retry_later` 写的是 `dispatch_blocked` 事件（`recordDispatchBlocked`，`project_store.go:3578`）。两种选择：

- a) 让 `recordDispatchBlocked` 也写 `dispatch_failed`（或改 `CountProjectTaskDispatchFailureEvents` 统计两种事件）；
- b) `retry_later` 的恢复走独立轻量路径——不经过 `RecoverProjectTaskDispatchFailure`，而在 `dispatchProjectTasks` 里直接 `scheduleDispatchRetry`（用 gate 的 `RetryAfter` 做 `retry_not_before`）。

倾向 (b)：gate `retry_later` 不是执行失败，是瞬时不可用退避，走完整 recovery（可能转 `waiting_human`）语义不对。在 `dispatchProjectTasks` 的 `dispatchFailureRecorded` 分支内，对 `retry_later` 子类直接 schedule retry 而不调 Recover：

```go
if dispatchFailureRecorded(err) {
    var recovery RecoverTaskDispatchFailureResult
    if errors.Is(err, ErrProjectTaskDispatchRetryLater) {
        // gate retry_later: 直接退避重试，不走 failure recovery
        scheduleDispatchRetry(ctx, tenantID, projectID, taskID, gateRetryAfter)
        continue
    }
    // 其他 dispatch failure: 走完整恢复路径
    if recoverErr := workflow.ExecuteActivity(...).Get(ctx, &recovery); recoverErr != nil { ... }
    ...
}
```

gate 的 `RetryAfter` 已由 `EvaluatePreDispatchGate` 在 `retry_later` 时填入（`predispatch_gate.go:408-410`），需要透传到 workflow 层——可通过 `DispatchProjectTask` 的返回值或 `ProjectTaskDispatchError` 携带。

**备选修法**（若不想改 error type）：在 `dispatchProjectTasks` 的 `!dispatchFailureRecorded(err)` 分支内、`return err` 之前，对 `retry_later` 子类单独 `scheduleDispatchRetry` 而不 return。这样 `DispatchProjectTask` 本身不动。

---

## 5. 锚点

| 项 | 路径 |
|---|---|
| 批准后路由 | `workflow/projectcoordination/predispatch_gate.go` `ApplyPreDispatchGateDecision` / `preDispatchRiskApprovalGranted` / `gateRiskApprovalGranted` |
| 门闸等待挂卡 | 同文件 `createGateHumanAction` / `findOrCreateGateApprovalRequest` |
| 派发遇 retry_later | `project_store.go` `DispatchProjectTask` `!AllowRunStart` 分支（`:3298-3309`） |
| retry_later 错误分类 | `types.go` `ErrProjectTaskDispatchRetryLater`（`:520`）/ `dispatchFailureRecorded`（`:536`）/ `dispatchFailureTerminal`（`:551`） |
| 派发失败恢复路径 | `workflow.go` `dispatchProjectTasks`（`:1403-1441`）/ `scheduleDispatchRetry`（`:1448`） |
| 恢复决策 | `service.go` `RecoverProjectTaskDispatchFailure`（`:5922`）/ `projectTaskDispatchRecoveryAction` |
| 任务等人释放 | `project_store.go` `applyTaskHumanWaitRelease`（`:1864`） |
| orphan 列表 | `storage/queries/project.sql` `ListOrphanWaitingHumanProjectTasks`（`:1756`） |
| orphan 补建 | `project/service.go` `SweepOrphanWaitingHumanProjectTasks`（`:6153`）/ `repairOrphanWaitingHumanProjectTask`（`:6182`） |
| 决策类型映射 | `projectTaskHumanWaitDecisionType`（`:6741`） |
| dispatch_blocked 事件 | `project_store.go` `recordDispatchBlocked`（`:3562`）——只写事件，不设 `retry_not_before` |
| durable grant | `predispatch_gate.go` `preDispatchRiskApprovalGranted`（`:351`）——快路径 + 扫 `ListDemandLaunchDecisionRequests` |

---

## 6. 验收

### 6.1 真实链路（不可降级）

前置：项目存在会触发高风险预检闸的任务；runtime 可故意制造短时 `provider_unavailable`（或使用已知会 retry_later 的夹具），**确认全机仅一个 runtime-agent**。

- [ ] 人类批准**真**闸卡后：任务在有限时间内离开「挂着已批卡的 waiting_human」——要么进入执行 attempt，要么进入可观测的自动重试（`retry_later` / `retry_not_before`），**不得**再生成「系统补建…需要人工审批」且 `approval_request_id=0` 的第二张 `project_task_approval`。
- [ ] 批准后即使紧接着 `runtime.provider_unavailable`：不得出现僵尸卡；provider 恢复后无需人类再批一次即可派发（durable grant 仍生效）。
- [ ] **`retry_later` 恢复路径（新增）**：fix A 释放后若 gate 落 `retry_later`，任务不得沉默搁置于 `planned`——必须出现 `scheduleDispatchRetry` 定时器（或等价退避唤醒），provider 恢复后自动再派发成功；持续不可用时按退避重试到上限后转 `waiting_human` 真 human card，不得造僵尸卡。
- [ ] orphan 看门狗对「真卡已批 + 任务短暂 waiting_human」的任务：**零**新建 decision。
- [ ] 存量扫描：上述三样例（或等价）被收敛后可再派发或诚实失败，收件箱无 pending 僵尸审批卡。

### 6.2 单测

- [x] `ListOrphan` / service：存在 approved gate 链接卡时不补建（改为 heal 再派发）。
- [x] `repairOrphan`：禁止 `project_task_approval` + 零 approval（降级 `project_task_clarification`）。
- [x] `ApplyPreDispatchGateDecision`：gate 链接卡 approved 走 `applyTaskHumanWaitRelease`（`Release` 被调用）。
- [x] **`retry_later` 恢复**：`isProjectTaskDispatchRetryLater` + workflow `GetVersion(gate-retry-later-schedule)` 分支 `scheduleDispatchRetry`。

### 6.3 门禁

提交态 `go test` 覆盖 `project` + `projectcoordination`；无契约变更则不必 `verify:contracts`。共享 checkout 仅 `git add` 显式路径。

---

## 7. 风险与回滚

| 风险 | 缓解 |
|---|---|
| 过早清空 `waiting_human` 导致 UI 闪断 | 释放与 ReadyTaskIDs 同事务/同 activity；inbox 以 decision 终态为准 |
| orphan 收紧后真孤儿无人补卡 | 仅排除「已有 approved gate 链接卡」；真缺卡仍补建，且审批类必须非零 approval |
| 与 attempt 级 `ProjectTaskRetryScheduled` 信号重复派发 | 幂等：Dispatch 短路径与 `retry_not_before` 守卫；单测覆盖双信号 |
| 存量脚本误 cancel 真卡 | 三重校验：零 approval + summary 前缀匹配 + 同 task 存在 approved gate 卡 |
| **`retry_later` 恢复路径改动引入双重派发（新增）** | 幂等保证：`projectTaskDispatchAllowed` + `retry_not_before` 守卫 + Dispatch activity 重试自身也是幂等的（gate idempotency key 去重）；单测覆盖 gate retry_later → scheduleRetry → 再派发全链路 |
| **gate `RetryAfter` 透传到 workflow 层可能需要改 activity 返回值（新增）** | 若走 §4.5 备选（workflow 层独立 schedule），则不需透传——用固定退避即可；若走推荐方案需改返回值，评估是否触发契约变更 |

回滚：恢复 orphan SQL/补建逻辑与批准后不清 waiting 的旧行为即可；`retry_later` 恢复路径改动同理还原为裸返回；无 DDL。

---

## 8. 与相邻工作的接口

| 文档 | 关系 |
|---|---|
| `2026-08-10-retry-redispatch-and-failure-attribution.md` | 同源观察入口；主症已修。本 spec 接棒 §6。 |
| `2026-08-09-provider-semantic-unification-design.md` §18-3 | 预算熔断腿曾因预检闸 + 僵尸卡阻塞；本 spec 落地后应能补跑。 |
| `2026-07-02-project-task-dispatch-runtime-recovery-design.md` | `retry_later` / recovery 语义；本 spec 要求 retry_later **不要**伪装成再等人，且 §4.5 确保其进入退避重试而非沉默搁置。 |
| `2026-07-19-stuck-task-reconciliation-design.md` | orphan waiting_human 看门狗出处；本 spec 修正其误诊与补建契约。 |

---

## 9. 实施顺序建议

1. **止血 orphan**：列表排除「已有 approved gate 链接卡」+ 禁止零 approval 的 `project_task_approval` 补建（小 diff，立刻停造僵尸）。
2. **修批准后状态机**（4.1）：去掉「approved 真卡挂在 waiting_request_id」中间态。
3. **修 `retry_later` 恢复路径**（4.5）：确保释放后的任务在 gate retry_later 时有退避唤醒，不沉默搁置。此步与 2 有逻辑依赖（2 先把任务从 waiting_human 释放到 planned，3 才能在 planned 上做退避重试），但也可独立测试。
4. **存量收敛**（4.4）。
5. **E2E**：高风险闸 + 人为 provider 不可用 → 恢复 → 无需二批；持续不可用 → 退避重试到上限 → 真 human card（非僵尸卡）。

---

## 10. 开放问题（实施前若有争议再拍）

1. 批准后目标态用 `planned` 还是保留某种 `queued_for_dispatch` 显式态？（建议复用 `planned` + 可选 `retry_not_before`，少引入状态。）
2. 僵尸卡终态用 `cancelled` 还是新 snapshot 值 `superseded`？（建议 `cancelled` + resolution 注释，避免词表扩张。）
3. 补建 `approval_required` 时是否必须重新走完整 `CreateRequest` 审批流，还是允许「仅决策卡、无独立 approval 行」但 **DecisionType 不得叫** `project_task_approval`？（倾向后者更安全：类型诚实。）
4. §4.5 推荐方案 vs 备选方案：改 `DispatchProjectTask` 返回值（让 `dispatchFailureRecorded` 认 `retry_later`）还是 workflow 层独立 schedule？（倾向备选：workflow 层独立 schedule，不动 activity 返回值/不冒契约风险，且 gate `retry_later` 走完整 failure recovery 语义不正确。）
