# 重试再派发不通与失败归因被覆盖

- 日期：2026-08-10
- 状态：**立项（人类已批），未实施**
- 来源：`2026-08-09-provider-semantic-unification-design.md` 复查期真实 E2E 实测（§18-1）
- 优先级：**高于 provider 语义层剩余项**——它让**所有** `transient_*` 族失败都拿不到真正的重试
- 迁移：待勘察后定

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

候选根因（需按序证伪，勘察点已备）：

1. **协调线程未发派发信号**——任务从 `failed → queued` 的转换是否 Signal 到 `project-coordinator:{project_id}`；
2. **派发活动静默失败**——Temporal workflow 历史里是否有失败/重试中的 dispatch activity；
3. **幂等去重误命中**——DB 实测：attempt #2 的 `runtime_task_id` / `lease_token` / `digital_employee_run_id` **与 attempt #1 完全相同**（旧值），若派发以 `runtime_task_id` 为幂等键，会被判为「已派发」而跳过。**这是当前最可疑的一条。**

先看 3：确认 attempt 重建时这三列应当置空还是重新生成。

### 2.2 任务级失败归因被最后一次尝试覆盖（副症，可独立修）

现在任务级结论/收件箱卡片取**最后一次** attempt 的 family，于是真因（attempt #1 的 `PROVIDER_NO_TERMINAL_EVENT`）被 watchdog 产生的 `runtime_recovery` 盖掉——人类被指向错误的方向（去查运行环境，而不是 provider 输出漂移）。

改法：任务级归因继承**首个非 `transient_runtime` / 非 watchdog 来源**尝试的 `error_code` + `failure_family`；watchdog 族只作为补充说明。

## 3. 与 provider 语义 spec 的接口

- provider spec §13 议题 11 已因本问题**临时翻回** `PROVIDER_NO_TERMINAL_EVENT` = `retryable=false`（人类 2026-08-10 确认）。本 spec 落地后应连同 2.2 一起把它翻回可重试，并在 provider spec 里销账。
- 不要在本 spec 里改 provider 错误语义；两边只通过「family + retryable」接口耦合。

## 4. 验收（真实链路）

- [ ] 造一次 `transient_provider` 失败：attempt #2 **确实产生 runtime 命令并真的重跑 provider**（`runtime_events` 有第二条 command_event）
- [ ] 重试耗尽后 `waiting_human`，且任务级结论显示的是**首因** `error_code`，不是 `runtime_recovery`
- [ ] 既有 watchdog 行为不回归（runtime 真的失联时仍能标 `lost`）

## 5. 现状锚点

| 项 | 路径 |
|---|---|
| 失败路由 | `apps/control-plane/internal/project/service.go` `projectTaskFailureAction` / `projectTaskRetryIdempotencyKey` |
| 重试建 attempt | 同包 `pg_repository.go`（attempt 重建时对 runtime_* 列的处理） |
| 看门狗 | 「Runtime did not acknowledge ... before deadline」产生点 |
| 协调线程 | `apps/control-plane/internal/workflow/projectcoordination/` |
| 观测入口 | `runtime_events` 表（有无第二条 command_event 即判别器） |

## 6. 同族观察（2026-08-10 复查期实测，未深挖）

高风险预检闸的**人类批准也不释放任务**：任务 `b449a6d5…` 上有两张 `project_task_approval` 卡，一张已被浏览器真实批准（`approval_request` 有值、`approval_decisions` 记到 21:16:12），另一张仍 `pending` 且 **`approval_request_id` 是全零 UUID**，任务因此卡在 `waiting_human/approval_required`、`attempts=0`。表征与 §2.1 同族——「决策/状态已就位，但任务不动」。勘察时一并看：重复开卡的来源，以及全零 approval 引用是哪条路径写的。
