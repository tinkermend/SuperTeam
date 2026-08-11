# 重试再派发不通与失败归因被覆盖

- 日期：2026-08-10
- 状态：**已实施（E2E 取证通过）**
- 来源：`2026-08-09-provider-semantic-unification-design.md` 复查期真实 E2E 实测（§18-1）
- 优先级：**高于 provider 语义层剩余项**——它让**所有** `transient_*` 族失败都拿不到真正的重试
- 迁移：无需库表迁移；`ScheduleProjectTaskRetry` SQL 清零派发身份列

---

## 0. 勘察结论（2026-08-10）

### 实际根因（两条叠加，缺一不可）

| # | 候选 | 结论 | 证据 |
|---|---|---|---|
| 1 | 协调线程未发派发信号 | **成立** | `FailProjectTaskAttempt` 在 `status=queued` 时直接 `return`，**不** `SignalEmployeeTaskFailed`（正确——该信号走人类恢复），也**没有**任何「再派发」信号。`scheduleDispatchRetry` 只挂在 dispatch 失败恢复路径上，attempt 级 requeue 无人唤醒 coordinator。看门狗 5 分钟后把 queued attempt 标 `lost`，循环至 max_attempts。 |
| 2 | 派发活动静默失败 | **排除** | 既无 `DispatchProjectTask` 被调用（无信号），也就不存在 activity 失败/重试历史。Runtime Agent 侧只有 attempt #1 的命令，与「派发从未发生」一致，而非派发后 silent fail。 |
| 3 | 幂等去重误命中 / 复用派发身份 | **成立（结构根因）** | `scheduleProjectTaskRetryWithQueries` / `resumeProjectTaskAfterHumanWaitWithQueries` 把失败 attempt 的 `DigitalEmployeeRunID`/`RuntimeTaskID`/`RuntimeNodeID` **原样拷到**新 attempt；`ScheduleProjectTaskRetry` **不清** task 行上的同名列。`DispatchProjectTask` 见 `task.DigitalEmployeeRunID != nil` 且已有 `project_task.dispatched` 事件 → 直接 `advanceDispatchedTaskDemand`，**不** `StartProjectTaskRun`。判别器：`runtime_events` 无第二条 `command_event`。 |

### 修复要点

1. 重试 attempt：**不**拷贝派发身份；`ScheduleProjectTaskRetry` 将 task 的 `runtime_task_id`/`digital_employee_run_id` 置 NULL（`provider_session_id` 仍由 `FindProviderSessionForTaskRoot` 在派发时续旧）。
2. 新增信号 `ProjectTaskRetryScheduled`：attempt 级 requeue / 看门狗 requeue 后唤醒 coordinator → `DispatchProjectTask`（可带 `RetryNotBefore` 退避）。
3. 任务级 waiting_human 卡片：首因取**首个非环境噪声** attempt 的 family/summary；环境噪声族显式清单见 `EnvironmentNoiseFailureFamilies`。
4. `PROVIDER_NO_TERMINAL_EVENT` 翻回 `retryable=true`（provider spec §13 议题 11 / §5.2 / §18-1 销账）。

---

## 1. 现象（真实链路实测，非推断）

需求 `6665b38e-aecf-4230-b803-df504fd8ac18`，假 provider 只发一行 `system` 后 exit 0：

| 尝试 | 结果 |
|---|---|
| #1 | `PROVIDER_NO_TERMINAL_EVENT` / `transient_provider` / `retryable=true` → 任务 requeue |
| #2 | **`lost`**——CP 看门狗「Runtime did not acknowledge project task attempt start before deadline」 |
| #3 | **`lost`**（同上） |
| 终态 | 12 分钟后 `waiting_human` / **`runtime_recovery`** |

关键证据：`runtime_events` 在 attempt #1 结束（17:30:24）之后**再无任何记录**——CP 从未为 attempt #2/#3 创建 runtime 命令。Runtime Agent 侧日志同样只有 attempt #1 的命令。**重试从未离开控制平面。**

不是本次环境偶发：同期另一批 8 条 `RATE_LIMIT` 任务（12:03–16:43）**全部同形态**（`attempts=3/3` → `waiting_human` / `runtime_recovery`）。

## 2. 两个可分别交付的问题

### 2.1 重试 attempt 不产生 runtime 命令（主症）

**第一交付物是诊断，不是补丁**：下面三条是候选根因，实施会话必须先证伪/证实再动手，不得直接照着改。

1. **协调线程未发派发信号**——任务从 `failed → queued` 的转换是否 Signal 到 `project-coordinator:{project_id}`；
2. **派发活动静默失败**——Temporal workflow 历史里是否有失败/重试中的 dispatch activity；
3. **幂等去重误命中**——DB 实测：attempt #2 的 `runtime_task_id` / `lease_token` / `digital_employee_run_id` **与 attempt #1 完全相同**（旧值），若派发以 `runtime_task_id` 为幂等键，会被判为「已派发」而跳过。**这是当前最可疑的一条。**

先看 3：确认 attempt 重建时这三列应当置空还是重新生成。

### 2.1.1 两个"身份"必须分开，别改错（重要）

| 身份 | 是什么 | 重试时应当 |
|---|---|---|
| `runtime_task_id` / `lease_token` / `digital_employee_run_id` | **派发内部身份** | **全新**。复用是上面第 3 条嫌疑的本体 |
| `provider_session_id` | **模型上下文会话** | **续用旧的**——已是既定行为，不要改 |

`provider_session_id` 续用不是待议题：`apps/control-plane/internal/employee/run_service.go` `standaloneDispatchCommandType` 的注释已写明，凡带 `provider_session_id` 的派发都是会话延续，**明确包含 retries**（`FindProviderSessionForTaskRoot`「covering retries and revision tasks」），并解释了为什么不能开新会话——重试若按 `start_session` 下发，runtime 会用一个已用过的 id 去 `claude --session-id <id>` **创建**会话，provider 直接拒（"Session ID already in use"），而每次尝试都注入同一个血缘会话，于是永远循环失败。

**风险**：修派发身份时"顺手"把会话也重开，正好踩进上面那个死循环。改动必须做到：派发身份全新、provider 会话续旧。

### 2.1.2 派发修好之后的第二层障碍（预告，不是本次症状成因）

resume 的已知代价是 fail fast：会话被 LRU 清理或 attempt 落到别的节点时返回 "no conversation found"（见同处注释的 Known trade-off）。而本次的失败形态是 provider 打了个 session id 就 exit——会话其实从未真正建立，重试去 resume 大概率同样失败。本次症状的成因是"根本没产生 runtime 命令"，与此无关；但派发通了之后这条会立刻浮上来，勘察时一并想清楚：**对"会话从未建立"的失败，重试应当降级为 `start_session` + 新会话 id 吗**。

### 2.2 任务级失败归因被最后一次尝试覆盖（副症，可独立修）

现在任务级结论/收件箱卡片取**最后一次** attempt 的 family，于是真因（attempt #1 的 `PROVIDER_NO_TERMINAL_EVENT`）被 watchdog 产生的 `runtime_recovery` 盖掉——人类被指向错误的方向（去查运行环境，而不是 provider 输出漂移）。

改法：任务级归因继承**首个非环境噪声**尝试的 `error_code` + `failure_family`；环境噪声族只作为补充说明。

**"环境噪声族"必须是一份显式清单，不能长成 contains 链**。按现有常量，属于环境噪声的是：`transient_runtime`、`runtime_lease_lost`、`runtime_start_timeout`、`dispatch_transient`（均由 CP 派发/看门狗侧产生，与 provider 实际执行无关）。`transient_provider` **不算**——它是 provider 真的跑了并失败。清单落在 `project/types.go` 与 family 词表同处，新增族时同步。

## 3. 与 provider 语义 spec 的接口

- provider spec §13 议题 11 已因本问题**临时翻回** `PROVIDER_NO_TERMINAL_EVENT` = `retryable=false`（人类 2026-08-10 确认）。本 spec 落地后应连同 2.2 一起把它翻回可重试，并在 provider spec 里销账。
- 不要在本 spec 里改 provider 错误语义；两边只通过「family + retryable」接口耦合。

## 4. 验收（真实链路）

- [x] 造一次 `transient_provider` 失败：attempt #2 **确实产生 runtime 命令并真的重跑 provider**（`runtime_events` 有第二条 command_event）  
  取证 2026-08-10：任务 `6844e174…`，3 个 attempt 均 `PROVIDER_NO_TERMINAL_EVENT`，3 个 distinct `digital_employee_run_id`，3 组 `command_event`/`command_failed`（cmd-d391…/eb640…/f08b…）
- [x] 重试耗尽后 `waiting_human`，且任务级结论显示的是**首因** `error_code`，不是 `runtime_recovery`  
  决策卡：`执行器启动或运行失败（PROVIDER_NO_TERMINAL_EVENT）：provider exited without a terminal event`；`waiting_reason=clarification`
- [ ] 既有 watchdog 行为不回归（runtime 真的失联时仍能标 `lost`）——本次未单独造「真失联」；逻辑未改看门狗判定，仅改 requeue 后身份与信号

## 5. 现状锚点

| 项 | 路径 |
|---|---|
| 失败路由 | `apps/control-plane/internal/project/service.go` `projectTaskFailureAction` / `projectTaskRetryIdempotencyKey` |
| 重试建 attempt | 同包 `pg_repository.go`（attempt 重建时对 runtime_* 列的处理） |
| 看门狗 | 「Runtime did not acknowledge ... before deadline」产生点 |
| 协调线程 | `apps/control-plane/internal/workflow/projectcoordination/` |
| 观测入口 | `runtime_events` 表（有无第二条 command_event 即判别器） |

## 6. 同族观察 → 已深挖（2026-08-11）：**已单独立项**

> **正式规格**：`docs/superpowers/specs/2026-08-11-predispatch-approval-orphan-zombie-design.md`  
> 下文保留勘察结论摘要；实施与验收以独立 spec 为准。

### 6.1 现象（仍成立）

高风险预检闸批准后任务不走：任务 `b449a6d5…` 等上有两张 `project_task_approval`——一张真闸卡（有 `approval_request_id` + `dispatch_gate_result_id`、已 `approved`），一张 **`approval_request_id` 全零、无 gate 链接、仍 `pending`**；`waiting_request_id` 指僵尸卡，任务卡在 `waiting_human/approval_required`、`attempts=0`。

同形现网不止一条（2026-08-11 抽样）：`b449a6d5` / `a17e0214` / `b9ce0207` 均为「真闸卡已批 + 系统补建僵尸卡 pending + gate=`retry_later`/`runtime.provider_unavailable`」。

### 6.2 全零 UUID **不是** gate 开卡写的

库内全部 `project_task_approval` 且 `approval_request_id=0000…` 的摘要一致：

> `系统补建人工决策卡：任务已停在待人工确认，但缺少可处理的决策（原因：需要人工审批）`

唯一生产者：`Service.repairOrphanWaitingHumanProjectTask`（`SweepOrphanWaitingHumanProjectTasks`）——硬编码 `ApprovalRequestID: uuid.Nil`，`DecisionType` 由 `waiting_reason` 映射；当 reason=`approval_required` 时就是 `project_task_approval`，**不**创建 `approval_requests` 行，**不**绑 `dispatch_gate_result_id`。

### 6.3 因果链（以 `b449a6d5` 时间线钉死）

| 时间 | 事实 |
|---|---|
| 21:12:29 | gate 判高风险 → 真卡 `19f46fdc`（approval=`01f97a9c`，gate=`31d11b21`） |
| 21:16:12 | 人类批准真 approval；21:16:13 真卡 → `approved` |
| 21:16:15 | gate 重评 → **`retry_later`**，blocker=`runtime.provider_unavailable`（会话/执行器暂不可用）；事件 `dispatch_gate.retry_later` + 多次 `dispatch_blocked` |
| 21:16:39 | orphan 看门狗补建僵尸卡 `483f806a`（全零 approval、无 gate），并 **改挂** `waiting_request_id` |

**上游缺陷 A（门闸批准后状态机）**：`ApplyPreDispatchGateDecision` 对**已绑定 gate 的** `project_task_approval`+`approved` 只返回 `ReadyTaskIDs` 去再派发，**不** `ReleaseProjectTaskHumanWait` 清掉 `waiting_human`。`projectTaskDispatchAllowed` 允许在 `waiting_human` 上继续 Dispatch。若随后 gate 变成 `retry_later`（仅写事件 + 返回 `ErrProjectTaskDispatchRetryLater`），任务**继续停在 `waiting_human`，且 `waiting_request_id` 仍指向已 approved 的真卡**。

**中游缺陷 B（orphan 误诊）**：`ListOrphanWaitingHumanProjectTasks` 条件是  
`waiting_human AND (waiting_request_id IS NULL OR 指向的决策非 open)`。  
「真卡已批、任务仍 waiting_human」被当成孤儿 → 补建。

**下游缺陷 C（补建卡不可用）**：补建对 `approval_required` 也走零 approval / 无 gate 的 clarification 式卡。人类若只在审批中心看到已批真卡、或再批真卡：`applyTaskHumanWaitRelease` 在 `waiting_request_id != decision.ID` 时 **直接 no-op**（已改挂到僵尸）。人类批僵尸卡理论上可走 release→再派发，但 (1) 文案像系统噪音、易被忽略；(2) 再派发仍可能 `provider_unavailable`→`retry_later`，若再次落回 waiting_human 且指针又成非 open，会循环补建。

### 6.4 与 §2.1 的关系

| | 重试再派发 (§2.1) | 僵尸审批 (§6) |
|---|---|---|
| 层 | attempt 失败 requeue | pre-dispatch 高风险闸 + orphan 看门狗 |
| 已修复？ | **是**（本 spec） | **否** |
| 是否同一补丁？ | 否——改动面与验收路径都不重叠 | |

表征同族（「状态已就位但任务不动」），**根因不同，应单独立项**，不要塞进本 spec 的 follow-up 清单里 silently 修。

### 6.5 立项

已成文：`2026-08-11-predispatch-approval-orphan-zombie-design.md`（状态：立项未实施）。实施顺序建议：先止血 orphan → 再修批准后状态机 → 存量收敛 → E2E。
