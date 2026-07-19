# 卡死任务收敛与跨视图状态一致性 设计（立项）

- 状态：立项（未实施）
- 日期：2026-07-19
- 缺陷家族：韧性家族「任务卡 running 无自愈」
- 归并来源：
  - `docs/superpowers/plans/2026-07-17-feishu-integration-p1.md` 遗留缺陷#1（写回失败无持久重试，任务卡 running 无恢复通路，实证 task ec160de3）
  - `TODO.md` 2026-07-19「runtime 写回失败无持久重试」条（本 spec 落地后由该条指向此文档）

## 1. 问题现象（真实触发）

运行总览显示某数字员工「工作中/正在运行」，点进其对应项目却看不到进行中的工作、观感是「已完成」。实证：员工 **P2a测试员**（`17e3a6ee`）在总览显示 working。

根因（实时库坐实，2026-07-19）：

- 其名下项目任务 `e2e20000-…-000b`「执行回归测试」status = `running`，`current_attempt_id` 为空、零 `project_task_attempts`、零 `task_runs`；created=updated=2026-07-18 14:26，之后从未变动。属直插 E2E 夹具，从未派发/执行，无任何生命周期把它推向终态。
- 运行总览「working」判据（`employee_execution.sql` `ListDigitalEmployeeOverviewOperationalFacts` + `operational_status.go:65`）：
  ```
  working = (该员工任一 project_task.status ∈ {running,in_progress})   -- operational_has_working_task
         OR (该员工最新 task_run.status ∈ {running,cancelling})
  ```
  信号1 命中 → 判 working。

止血已于 2026-07-19 完成：该项目 `4be304f7` 内 `e2e20000-…-000b/000c/000d` 三条零执行孤儿夹具收敛为 `cancelled`。本 spec 处理治本。

## 2. 结构性缺口（本 spec 要修的）

1. **孤儿/僵尸任务无收敛兜底**：`project_task` 可停在 `running`/`in_progress` 且零 attempt/零活跃 run，无任何 reconcile 把它推向终态或转人工。触发路径至少两类：
   - runtime 写回 400/失联后结果永久丢失（遗留缺陷#1，实证 task ec160de3，重启 runtime-agent 不自愈）；
   - 直插夹具 / coordinator 协调线程死亡 / 派发后 attempt 未落地，任务从未进入执行但已置 running。
2. **员工级「working」是跨项目 OR 聚合且不可解释**：总览座位显示 working 不等于「某个特定项目在跑」——可能来自另一项目。用户进一个项目看到完成，与员工级 working 并存，不必然是矛盾，但当前 UI 无法解释 working 来自哪个项目/任务。
3. **员工详情页第三套算法**：详情页用前端本地 `hasActiveRun`（读 `/runs` 列表），与总览/列表的 SQL 聚合口径不同，是独立分叉源。
4. **项目无 `completed` 终态词**：项目状态枚举仅 `draft/configuring/running/paused/acceptance/archived`（`types.go:129-137`），真正终态是 `acceptance→archived`，由「所有 demand 终态」驱动。用户看到的「项目已完成」实为 completed 任务徽标 + 完成计数的人眼观感，非项目状态。UI 需消除这一措辞落差。

## 3. 治本设计

### 3.1 僵尸任务检测看门狗（核心，控制平面侧）

在 CP 增加周期性 reconcile（复用系统配置中心心跳/看门狗基建，参考 `system-config-p2-runtime-limits`），attempt/task 级检测并收敛：

- 判据：`project_task.status ∈ {running,in_progress}` 且满足以下之一超过可配阈值：
  - `current_attempt_id IS NULL`（从未落 attempt）；
  - 有 attempt 但无对应活跃 `task_run`，且 attempt 停滞（`updated_at` 老于阈值）；
  - 有 run 但 run 已终态（completed/failed），task 未同步。
- 动作（分级，均产出结构化审计 + ProjectEvent）：
  - 从未执行（零 attempt/零 run）→ 直接置 `cancelled`（附 reconcile 原因），或按项目策略转 `pending` 重新入选角池；
  - 执行过但写回丢失 → 优先触发 runtime 侧持久重试；重试耗尽 → attempt 标 failed + 开 `task_failure_recovery` 决策卡转人工（对齐既有 SoD/人类守门路径）。
- 阈值走系统配置中心 key 注册表（如 `task.stuck_running_timeout`），不硬编码。

### 3.2 runtime 写回持久重试（遗留缺陷#1 本体）

runtime-agent 写回 CP 失败（400/失联）时，结果落本地持久队列并重试，避免结果永久丢失导致 task 卡 running。与 3.1 互补：3.2 减少僵尸产生，3.1 兜底已产生的僵尸。

### 3.3 跨视图一致性（统一出口）

- **消灭第三套算法**：员工详情页改为消费 overview 同源 `operational_state`（或后端提供单一「员工当前作业状态」投影），前端不再本地算 `hasActiveRun`。
- **working 可解释**：overview 座位 item 增补 working 来源（project_id + task title），总览座位可展开「正在 X 项目做 Y 任务」，消除「进项目却看不到在跑」的错觉。
- **项目措辞对齐**：项目详情不把任务级 completed / 完成计数呈现为项目「已完成」；项目层文案统一为「进行中/验收中/已归档」，任务完成度以计数或进度条表达。细则遵循 `DESIGN.md`「面向用户文本与枚举显示」，词表补键走 `status-labels.ts`。

## 4. 分期建议

- P1：3.1 僵尸看门狗（止血面最大、治本核心）+ 3.2 写回重试（消除主产源）。
- P2：3.3 一致性（详情页同源 + working 可解释 + 项目措辞）。

## 5. 验证方式（真实端到端，非单测）

- 造零执行孤儿 running 任务（直插）→ 看门狗到期 → 确认转 cancelled + 审计 + 总览座位翻空闲。
- 造 runtime 写回失联（断连 runtime-agent）→ 确认持久重试；重试耗尽 → 转人工决策卡进收件箱。
- 员工同时在 A 项目跑、B 项目完成 → 总览座位可展开显示 working 来源为 A 项目；进 B 项目无误导。
- 走真实 Web + CP + DB + runtime 路径，遵循 CLAUDE.md「验证与收尾」。

## 5.1 实施状态(2026-07-19)

**关键设计修正(勘察后)**:
- 3.1/3.2 的收敛能力**大部分已存在但从未接线**:`SweepExpiredRunningProjectTaskAttempts`(租约过期的 running attempt,即遗留缺陷#1 本体,键在 `pta.lease_expires_at < now`)、`SweepStaleQueuedProjectTaskAttempts`、`RecoverProjectTaskDispatchFailure` 三者生产零调用者,是死代码。治本从"发明收敛"降级为"把死代码接上周期调度",风险大减。
- coordinator 边界安全性坐实:signal 客户端用 `SignalWithStartWorkflow`(client.go:113),signal 到已死/从未存在的协调线程会**透明拉起**并从存储重建状态。故看门狗"写终态 + SignalEmployeeTaskFailed"对任何卡死任务(含 orphan、含协调线程已死)都安全,不失配不丢信号。orphan 收敛复用通用 `FailProjectTaskWriteback`(system actor,不经 `taskAndProjectForWriteback` 的 runtime-node 假设)。

**已落地并真实 E2E 验证(P1 orphan 路径)**:
- 配置 key `task.stuck_running_timeout_seconds`(duration,默认 900s,界 120s–6h,DomainExecution)—— `systemconfig/registry.go`。
- 新查询 `ListStuckOrphanProjectTasks`(跨租户)+ `ListTenantsWithRecoverableProjectTaskAttempts`(`storage/queries/project.sql`,sqlc 已生成)。
- 服务:`SweepStuckOrphanProjectTasks` + `reapStuckOrphanProjectTask`(system-actor 失败写回 + `SignalEmployeeTaskFailed`)、`SweepStuckProjectTaskAttemptsAllTenants`(逐租户驱动既有两 sweep)—— `project/service.go`。
- 看门狗 `startStuckTaskReconciler`(`app/stuck_task_reconciler.go`,tick=1min,启动即扫一轮)+ `app.go` 装配(注入 `SystemConfig` 到 Container)。
- **真实 E2E(重启 CP 加载新码 → 全链走真实 CP+Temporal+DB+API)**:直插 30min 龄 orphan running 任务(P2a测试员)→ 启动扫一轮 `reaped count=1` → 任务置 failed+terminal event(actor=system)→ `SignalWithStart` 驱动真实协调线程开 `task_failure_recovery` 卡(pending)→ 收件箱 API 可见 `open`「处理项目任务失败」→ overview API:P2a测试员 由 working 转 `waiting_human`、`running_count=0`;清理夹具后回 `idle`。`verify:control-plane` 全绿。

**已落地 + 真实 E2E 验证(遗留缺陷#1 attempt 路径)**:两个既有 attempt 恢复 sweep 由看门狗 `SweepStuckProjectTaskAttemptsAllTenants` 逐租户驱动。E2E:直插 running task + running attempt(lease_expires_at 过期 10min,attempt_count=1),看门狗 1min tick → 日志 `recovered ... count=1` → attempt 置 `lost`、task 转 `waiting_human`(reason runtime_recovery)→ 开 `project_task_recovery` 卡(pending)→ 收件箱 API `open` → overview P2a测试员 `waiting_human`/`running_count=0`;清夹具后回 idle。**至此遗留缺陷#1(runtime 死亡致任务卡 running 无自愈)在 CP 侧已被证明治好。**

**已落地 + 真实 E2E 验证(P2 3.3b + 3.3c)**:
- 3.3b working 权威可解释:overview items SQL 新增 `employee_working_task` CTE,取每个员工当前 running/in_progress 的 project_task(与 working 判定同源)携所属项目名,经 item.current_work(project_id/name + project_task_id/name)下发。前端座位卡 `currentProjectOf` 优先用权威 current_work(替代 latest_run+project_summary 启发式拼接)。真实浏览器 E2E:唯一 working 员工 P2a测试员,座位卡"所属项目"深链 href = current_work.project_id(4be304f7),与 API 权威值精确一致。
- 3.3c 项目措辞一致:删除 project-operational-detail 里的局部 `projectPhaseLabel` 映射(违反 DESIGN 词表单一源且与状态胶囊不一致:running→执行中 vs 运行中),阶段格改用 `projectStatusLabel`。真实浏览器 E2E:项目页状态胶囊与"当前阶段"格均"运行中","执行中"不再出现。
- 门禁:verify:control-plane 全绿;web typecheck 净 + 定向测试 367 全过。

**未实施(优先级已下降)**:3.2 runtime 侧写回持久重试(Rust runtime-agent):CP 侧安全网已证明能兜住 runtime 死亡,此项降为"减少僵尸产生 + 结果不丢"优化,非紧急,TODO 记后续。3.3a 员工详情页第三套算法(hasActiveRun 只门禁启动按钮、不驱动徽标、用户不可见分叉,统一需后端补单员工 facts)——低性价比,用户拍板不做。

## 6. 关联

- 心跳/看门狗基建：`docs/superpowers/specs/2026-07-19-system-config-p2-runtime-limits-design.md`
- 转人工决策/SoD 路径：autonomy-posture 系列
- overview 取数链条：`apps/control-plane/internal/storage/queries/employee_execution.sql`（`ListDigitalEmployeeOverviewOperationalFacts`）、`internal/employee/operational_status.go`、`internal/employee/pg_repository.go`
