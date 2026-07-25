# P1 平台加固 Spec（预算熔断 / 数据保留 / 连接池与 SSE / 前端入口包）

- 日期：2026-07-25
- 状态：**设计待拍板**，四个子域可独立开工
- 目标读者：接手实施的独立会话（不共享原始对话上下文，本文档需自包含）
- 交付性质：跨层（migration + CP + Web + 运维作业），但四个子域之间**无硬依赖**，可并行或分批
- 来源：2026-07-25 的一次平台体检，四项均为体检结论中的 P1；同批的 P0（无 CI/无部署产物、CP 硬单实例、零可观测性）**不在本 spec 范围**，见 §6

---

## 0. 本文档中的事实都是实测的

下表是设计所依据的实测数据，全部取自 2026-07-25 的真实 dev 环境（CP:8080 + 远程 PG 115.190.247.9:35432 + 真实 Temporal）。**设计判断可以争论，这些数字不要重新猜。**

| 观测 | 实测值 | 取数方式 |
|---|---|---|
| CP↔DB 网络 RTT | **39ms**（min 38.5 / max 39.2） | `ping -c 4` |
| pgxpool 配置 | **零调优**，走 pgx 默认 `MaxConns = max(4, NumCPU)` | `storage.go:47-53` 只有 `ParseConfig` + `NewWithConfig` |
| SSE 端点数 / 轮询间隔 | 2 个，均 **2s** 每连接一次 DB 探测 | `inbox/handler.go:31`、`employee/handler.go:55` |
| `web_operation_logs` | **177,115 行 / 137 MB** | `pg_stat_user_tables` |
| ↑ 其中 authz 放行 | 177,090 succeeded vs **19 failed** | 按 module/result 分组 |
| ↑ 最后写入 | **2026-07-22**（最近 1 小时 0 条） | 按天分组 |
| 其余 append-only 表 | task_events 4020 / runtime_events 3972 / execution_ledger_events 3002 / provider_session_events 2146 | 同上 |
| 保留/清理机制 | **无分区、无 purge 查询**；`DeleteExpiredSessions` 已写但**零调用方** | 全仓 grep |
| attempt 预算列 | 6 列俱全（limit/heartbeat/consumed_wall_clock/consumed_tokens/tripped_at/trip_reason） | `information_schema` |
| ↑ 实际数据 | 46 条 attempt，**0 条有 limit、0 条 tripped**，但 `budget_consumed_tokens` 合计 **96,365** | 直查 |
| `project_budget_ledger` | 表结构完整（estimated/actual tokens+cost、cost_type、source），**零生产调用方** | grep `CreateBudgetLedgerEntry` |
| `projects` 表 | **没有任何 budget/cost/limit 列** | `information_schema` |
| Web 入口包 | `index-*.js` **1508 KB raw / 469 KB gzip**，CSS 220 KB | `dist/assets` + gzip |
| ↑ 入口块里的重依赖 | `@xyflow/react`、`@dicebear/*` | grep 构建产物 |
| Vite 分包配置 | **无 `manualChunks`** | `vite.config.ts` |

---

## 1. P1-C：连接池与 SSE（**建议第一个做**）

排第一不是因为它最严重，而是因为它是其他三项的地基：预算熔断要加写路径、保留作业要加后台查询，都会吃连接池。

### 1.1 问题

**(a) 连接池从未被配置。** `storage.NewClients` 直接 `pgxpool.ParseConfig` → `NewWithConfig`，`MaxConns` 取 pgx 默认 `max(4, NumCPU)`。

关键在于**约束不是 CPU 而是 RTT**：39ms 往返意味着单条连接每秒最多约 25 次往返，池的吞吐上限 ≈ `MaxConns × (1/RTT)`。4~10 条连接 = **100~250 次 DB 往返/秒**的天花板，与机器多强无关。

**(b) SSE 每连接每 2 秒打一次库。** 收件箱 SSE 已提升到全局布局，**每个打开的标签页一条长连接**，每条各自 `PeekInboxChange`（含 `project_members` EXISTS 子查询）。50 个在线标签页 = 25 QPS 常驻纯轮询，且每次都要占用池连接一个 RTT。叠加 (a) 的天花板，几十个用户就可能打满。

**(c) 远程 RTT 放大 N+1。** 近期 `perf(inbox): parallelize list queries`（108b1c6a）用 errgroup 并行化是对症的，但只覆盖了收件箱一处，其余读路径仍是串行往返。

### 1.2 方案

**C1 显式池配置（半天，零风险）**

`storage.Config` 增加池参数，`NewClients` 显式设置，全部可经 config.yaml + env 覆盖：

| 参数 | 建议默认 | 理由 |
|---|---|---|
| `MaxConns` | 25 | 39ms RTT 下给出约 640 往返/秒上限；远低于 PG 默认 100 连接上限，留出多副本余量 |
| `MinConns` | 5 | 冷连接建立在 39ms RTT 下要多次往返（TLS + 认证），预热避免首请求抖动 |
| `MaxConnLifetime` | 30m | 防止远程链路上的僵死连接 |
| `MaxConnIdleTime` | 5m | — |
| `HealthCheckPeriod` | 30s | 远程链路必需 |

**默认值必须写进 config.yaml 并加注释说明"这是 RTT 约束不是 CPU 约束"**，否则下一个人还会以为不用配。

**C2 SSE 去轮询，改 LISTEN/NOTIFY（2~3 天，主要收益）**

- 写侧：`inbox_items` 变更处发 `pg_notify('inbox_changed', <tenant_id>)`。**放在应用层而非触发器**——触发器会让 CP 的写路径对 DB 内部逻辑产生隐式依赖，与"业务事实在 CP"的分层冲突。
- 读侧：CP 起**一条**专用长连接 LISTEN，收到通知后在进程内 fan-out 给所有 SSE goroutine。fan-out 注册表复用 `runtime.ConnectionRegistry` 的现成模式。
- 效果：N 条 SSE 的 DB 成本从 `N × 0.5 QPS` 降到**一条常驻连接**，且延迟从"最多 2 秒"变成近实时。
- **对多副本友好**：每个 CP 副本各自 LISTEN 同一 channel，PG 会广播给所有监听者。这与 P0#2（CP 硬单实例）不冲突，反而是少数天然支持多副本的机制。
- 保留现有游标兜底：NOTIFY 不保证送达（连接断开期间的通知会丢），所以**保留低频兜底轮询**（30~60s）+ 现有的 `onopen` 立即 invalidate 自愈逻辑。这条不能省。

**C3 过渡方案（若 C2 暂不做）**：间隔 2s → 5~10s，进 system_config 可调，并加随机抖动避免所有标签页同相位打点。**这是止血不是治本**，因为成本仍随连接数线性增长。

### 1.3 验收

- C1：`SHOW max_connections` 与实际 `pg_stat_activity` 计数吻合预期；压测 60 并发下无 `context deadline exceeded`。
- C2：开 20 个标签页，`pg_stat_statements` 里 `PeekInboxChange` 调用量从约 600 次/分降到接近 0；DB 侧新增待办后前端 ≤1s 出现（当前最多 2s）；**断开 CP↔PG 连接再恢复，验证兜底轮询能自愈**（这是最容易漏测的一条）。

---

## 2. P1-B：数据保留策略

### 2.1 问题

106 张表，全仓**无 `PARTITION`、无一条 purge 语句**。`DeleteExpiredSessions` 写好了但从未被调用。项目删除是软删（`SoftDeleteProject`），不清行也没有物理清除通道。

现状最大的一块是 `web_operation_logs` **177k 行 / 137 MB**，而且性质特殊：

- 它**只有一个写入方**（module 全部是 `authz`）
- 177,090 条 succeeded vs 19 条 failed —— 记录的绝大部分是"读放行"，审计价值接近零，真正的信号（拒绝）只有 19 条
- **07-22 之后已停写**（最近 1 小时 0 条），说明读路径的写放大在此前的权限中心重构里已被消掉

所以这 137MB 是**历史死重**，不是还在涨的问题。但其余 append-only 表（runtime_events / execution_ledger_events / provider_session_events / task_events / runtime_command_receipts）**仍在随每次执行增长且无任何上限**，dev 环境量小掩盖了这一点。

### 2.2 方案

**B1 表分类（先定策略再写代码）**

**已拍板（2026-07-25，人类授权"该删就删"，以下为定稿分类）：**

| 类别 | 表 | 保留 | 依据（已核实） |
|---|---|---|---|
| 业务事实 | `project_events` | **不删** | `project.sql` 有 11 处读路径，项目时间线的事实来源 |
| 业务事实 | `execution_ledger_events` | **不删** | `ListProjectExecutionLedgerEvents` → 执行轨迹面板，是活的证据链 |
| 业务事实 | `audit_events` | **不删** | 审计基线 |
| 运维遥测 | `runtime_events` | 30 天 | 只服务 Runtime 概览的近期视图 |
| 运维遥测 | `provider_session_events` | 30 天 | 排障用 |
| 运维遥测 | `runtime_command_receipts` | 30 天 | 命令回执，过期无价值 |
| 运维遥测 | `task_events` | 90 天 | 老 task 体系，保守取 90 天 |
| 运维遥测 | `web_operation_logs` | 见 B3 + 之后 90 天 | 只有 authz 一个写入方 |
| 会话/临时 | `auth_sessions` | 到期即删 | 接上现成的 `DeleteExpiredSessions` |

**外加一条比调保留天数更有价值的规则**：三张"不删"的表，其**已软删项目**的行在软删满 30 天后一并清除。项目软删后在应用层已完全不可见（`SoftDeleteProject` 只置 `deleted_at`，不清行），这些行既回收不了空间也没有读者——这是"该删就删"里唯一既能显著回收空间又不伤业务事实的部分。实施时按 `project_id IN (SELECT id FROM projects WHERE deleted_at < now() - 30d)` 批量清理。

**B2 表驱动的保留作业**

新增 `internal/retention` 模块：

```
注册表：表名 + 时间列 + 保留天数(system_config key) + hold 保护开关
作业：ticker（建议 1 小时）→ 逐表分批 DELETE ... WHERE <时间列> < now() - interval N day LIMIT 5000
```

要点：
- **分批 + LIMIT**，避免长事务和表锁；每批之间让出，别把远程链路占死
- 保留天数走 **system_config**（仓库已有 13 项 key 的注册表机制，直接扩展，不新建配置通道）
- 与 `artifact_retention_holds` 协调：被 hold 的工件相关行不删
- **单实例保护**：这个作业绝不能在多副本上并发跑。在 P0#2 的 leader 选举落地前，用**会话级 `pg_advisory_lock`** 做单跑保护——一行代码，不阻塞本项落地，且 leader 选举做好后可平滑替换

**B3 一次性清理 `web_operation_logs`**

177,090 条 authz 放行记录先归档或直接清空，只保留 19 条 failed。**这一步需要人类明确批准**（审计表清理）。清完再让 B2 接管。

**B4 分区（第二阶段，非必须）**

真正上量后，对最大的 1~2 张表按月 RANGE 分区，用 `DROP PARTITION` 替代 `DELETE`。**第一阶段不要做**——分区改造要动 sqlc 生成物和所有查询，收益不如 B2 直接，先把增长止住。

**B5 补齐项目物理清除通道**

当前项目只能软删，测试/废弃项目的行永久堆积。建议增加管理员确认的物理删除（级联 + 快照），与团队生命周期已有的 `pending_delete → 确认物理删` 模式对齐（见 `team-lifecycle-convergence`）。**优先级低于 B2**。

### 2.3 验收

- 造超期数据 → 跑一轮作业 → 行数下降且业务事实表未被误删
- 两个 CP 进程同时起，确认 advisory lock 下只有一个真的在删
- `artifact_retention_holds` 保护的行在作业后仍在

---

## 3. P1-A：预算熔断

### 3.1 问题

平台的核心卖点是数字员工自治执行 + 自动返工循环（对抗评审 held → 回灌 → 重跑），**但没有任何金额或 token 维度的熔断**。

机制其实是半成品，而不是完全空白——这一点很重要，决定了工作量：

- ✅ `project_task_attempts` 已有完整的 6 列预算字段，含 `budget_tripped_at` / `budget_trip_reason` 跳闸机制
- ✅ `budget_consumed_tokens` **确实在被写入**（46 条 attempt 合计 96,365 tokens）
- ✅ `project_budget_ledger` 表结构完整
- ❌ **46 条 attempt 里 0 条设了 limit** → 跳闸机制永远不可能触发，因为没有上游来源
- ❌ `service.go:798` 的跳闸判断**只看 wall clock**，不看 token/成本
- ❌ `CreateBudgetLedgerEntry` **零生产调用方**，ledger 是死表
- ❌ `projects` 表**没有任何预算列**，无处配置额度
- ❌ `cost` 模块只读、只统 token 不折算金额，且 `WHERE tr.status IN ('completed','finished')` **把失败 run 排除在外**——而返工循环里最烧钱的恰恰是失败的那些

### 3.2 方案（2026-07-25 人类拍板简化版）

人类明确要简单版，不要原来的 ledger + 软闸 + 三层限额。核心一句话：**给项目一个 token 额度，超了就不再让它开新任务；已在跑的不打断，跑完了下一次开工才拦。**

已核实的前提（决定了这版能简化到什么程度）：
- `budget_consumed_tokens` **确实在按 attempt 实时写入**——`UpdateProjectTaskAttemptBudgetHeartbeat`（`project_runtime_affinity.sql:155`）由 runtime 心跳上报、`GREATEST` 单调累加。
- 所以「项目已消耗」= 对项目下所有 task 的所有 attempt 的 `budget_consumed_tokens` 求和，**数据现成，不需要 ledger**。

**A1 项目额度列（新 migration，极小）**

`projects` 只加一列 `budget_token_limit BIGINT`（可空；NULL = 不限，即现状，对存量项目零影响）。不加 cost/period/attempt 限额列。

**A2 项目已消耗聚合（一条 SQL）**

`SELECT COALESCE(SUM(a.budget_consumed_tokens),0) FROM project_task_attempts a JOIN project_tasks t ON ... WHERE t.project_id = $1`。**按 attempt 求和天然把失败与返工的消耗算进去**——不需要碰 run status 过滤问题（那是 cost 模块的老毛病，本期不动它）。

**A3 单一闸：派发前拦「开新工」**

只有**一道硬闸**，落在 `RunPreDispatchGate`：派发一个 task 前，若「项目已消耗 ≥ 额度」→ block reason `budget_exhausted` + 人类待办（提额 / 终止）。

这一道就同时满足了人类的三条要求：
- **允许运行中任务超标**——闸在派发前，已经在跑的 attempt 不受影响，心跳继续累加，不打断。
- **任务结束后下次开工才拦**——每次 task 派发都过这道闸，超额后的下一次派发被挡。
- **覆盖所有入口**——协调线程、自动化触发、chat/loop 都经 pre-dispatch，一处拦全。

**不加软闸**（原 A3 的 `service.go:798` 心跳跳闸不扩展 token）——人类明确「允许运行中的任务超标」，运行中不该打断。`service.go:798` 维持只看 wall clock。

**A4 前端拦「发起」+ 读模型露预算**

- 项目读模型加 `budget`：`{ token_limit, consumed_tokens, exhausted }`（consumed 走 A2 聚合）。
- 前端在「发起任务 / 提交需求」入口按 `exhausted` 禁用按钮并给出说明（人类原话「前端禁止用户再发起任务请求」）。
- **前端禁用只是 UX，不是执行边界**——真正的强制在 A3 后端硬闸（自动化不走前端）。两者都要，别只做前端。

**明确不做**（相对原设计砍掉的）：
- ❌ ledger（`project_budget_ledger` 保持死表，A2 直接 SUM attempt）
- ❌ 软闸 / 运行中打断
- ❌ 金额维度 / 单价表 / `budget_cost_limit`
- ❌ `budget_period`（口径固定 total）
- ❌ attempt 级限额列传播
- ❌ cost 模块改造（`status IN (...)` 老毛病本期不碰）

日后要加金额或周期都是纯增量，不动这版落的 `budget_token_limit` 列与 pre-dispatch 闸位置。

### 3.3 验收

真实链路：造一个低 `budget_token_limit` 的项目 → 真跑任务累加 token 到超额 → 依次确认：
1. 运行中的 attempt **不被打断**（心跳继续，consumed 继续涨过额度）；
2. 该任务结束后，**下一次 task 派发被 pre-dispatch gate 挡**（block reason `budget_exhausted`）；
3. 收件箱出现「预算耗尽」人类待办，可提额（改大 `budget_token_limit`）后放行；
4. 项目读模型 `budget.exhausted=true`，前端发起按钮被禁用；提额后恢复；
5. 负向对照：`budget_token_limit=NULL` 的项目行为与现状完全一致，永不被此闸拦。

### 3.4 员工按天 token 额度 —— **已存在，勿重复造**（2026-07-25 核查）

预算有两个维度：**项目总量**（§3.1–3.3，本 spec 新建）和**员工按天**。后者人类 2026-07-25 提出时以为是新需求，核查发现**平台已完整实现并强制**，不要再加列或查询：

- **存储**：员工 config 的 `budget_policy.daily_token_limit`（JSON），设值经 `employee/service.go` 校验（正整数）。**不是 `digital_employees` 的列**。
- **当日用量**：`today_budget_usage_tokens`，从 `task_runs` 按 `Asia/Shanghai` 日界求和（`employee_execution.sql:1155`），每日自动重置。
- **强制**：`employee/run_service.go` 的 `validateDailyTokenBudget` 在**三个接单入口**（245/305/409，`CreateRun`/项目任务 run/chat run）检查 `TodayTokenUsage >= daily_token_limit`，超则拒单（`ErrInvalidInput`「employee daily token budget exceeded」），不派发、不建 run。
- **展示**：`/api/v1/digital-employees/overview` 的 `budget_summary{daily_token_limit, usage_tokens_today, limit_exceeded}`。
- **测试**：`TestCreateRunRejectsWhenDailyTokenBudgetExceeded` / `...Unset` 已覆盖拒单/放行。

即「员工按天超额则不能再接新任务」这条需求**已满足**。与项目额度是两套独立机制（员工级 vs 项目级、按天 vs 总量、run-start 拦 vs pre-dispatch 拦、task_runs 计量 vs project_task_attempts 计量），共存不冲突。本轮曾误建员工 `daily_token_limit` 列 + `SumEmployeeConsumedTokensToday` 查询，发现重复后已全部回退。

---

## 4. P1-D：Web 入口包瘦身

### 4.1 问题

入口 `index-*.js` **1508 KB raw / 469 KB gzip** + CSS 220 KB，每个用户首屏必载。`vite.config.ts` **无 `manualChunks`**，实测 `@xyflow/react`（流程图）和 `@dicebear/*`（头像生成）被打进入口块——而它们只服务少数路由。TanStack Router 的路由级分包已生效，但重型 vendor 没被剥出去。`teams` 路由块单独 348 KB / 155 KB gzip。

### 4.2 落地 plan（2026-07-25 实测锚定，待人类审核后开工）

**已测事实**（当前 dist，`grep` 入口块 + `du`/`gzip`）：
- 入口 `index-*.js` = **1508 KB raw / 469 KB gzip**，CSS 220 KB。
- 入口块里的重依赖只有两个：**`@xyflow/react`**（7 处命中，流程图）和 **`@dicebear/*`**（1 处命中，头像生成）。`monaco-editor`、`recharts`、`react-markdown`、`pinyin` **均已不在入口块**（已被路由分包或已 lazy）——所以不用碰它们，D 的目标收窄到这两个。
- 静态导入点：`@xyflow/react` → `workflows/components/{workflow-graph-canvas,workflow-blocking-node,workflow-task-node}.tsx` + `projects/components/plan-graph-canvas.tsx`（全是图可视化，非首屏）；`@dicebear/*` → `components/superteam/user-identity.tsx`（`createAvatar`+`adventurer`，头像组件，用得广）。
- `teams` 路由块 348 KB（已在自己的块里，不进入口；第二梯队，本轮不追）。

**Step 0 · 基线量化（先做，不改码）**
跑一次 `corepack pnpm build:web` 拿当前真实入口 gzip 数（其他会话动过 web，earlier 的 469 KB 可能已变），并用 `rollup-plugin-visualizer`（或 `--analyze`）确认入口块 top 模块。**所有后续步骤的收益都对这个基线读数，不对旧数。**

**Step 1 · xyflow 退出入口（预计最大单项收益）**
4 个用到 `@xyflow/react` 的组件改 `React.lazy` + Suspense 包裹。它们都是流程图/计划图可视化，只在 workflows/项目详情图 tab 出现，首屏不需要。
- 落点：把 4 个组件的静态 `import` 改为在其消费处 `React.lazy(() => import(...))`，配 loading 占位。
- 风险：这些组件可能被同步引用（如条件渲染但同模块 import）；需确认改后仍能渲染。真实浏览器验证流程图 tab 正常。

**Step 2 · dicebear 退出入口 —— 已定 2b（存渲染后 SVG + 自愈 + 回填，2026-07-25 拍板）**

硬约束：**后端是 Go，dicebear 是 JS-only**，服务端无法实时渲染 dicebear。dicebear 头像是种子的确定性函数，故"预生成"= 把渲染好的 data-URI 存库，前端读存好的、不再在浏览器算。落地：
- **Schema**：`users` 加 `avatar_svg TEXT`（可空）。存渲染后的 data-URI。
- **回填（主力无闪机制）**：Node 构建/CLI 脚本（工具链有 Node），列全部用户 → 按各自 seed（`user:${username}` 或存的 seed）用 dicebear 渲染 → 写回 `avatar_svg`。一次性覆盖所有存量用户。
- **自愈（仅本人）**：新增 `PUT /users/me/avatar-svg`，仅允许写**当前用户自己**的头像。原因:后端跑不了 dicebear、无法校验他人提交的 SVG 是否真是该用户种子的确定性产物,放开写他人头像不安全。当前用户查看自己头像且 `avatar_svg` 缺失时,前端懒生成并写回,覆盖"回填后新建的用户"。
- **前端**：`UserIdentityAvatar` 优先渲染 `user.avatar_svg`(零 dicebear、无闪)；缺失时 `import()` 懒加载 dicebear 生成渲染(独立 chunk、不进入口),本人则顺带自愈写回。
- **结果**：dicebear 移出入口包；已回填用户展示零 dicebear、无闪；未回填的靠懒加载兜底渲染(dicebear 永不进入口),不会无谓只显首字母。
- 契约:`User`/`TeamUserAvatar` 相关读模型加只读 `avatar_svg`；新增 `PUT /users/me/avatar-svg`。

**Step 3 · `manualChunks` 收尾**
Step 1/2 让重依赖离开静态入口图后，补 `vite.config.ts` 的 `manualChunks` 把剩余 vendor（react/router/query/radix 等）按稳定性切块，改善缓存命中。**注意**：`vite.config.ts` 正被其他会话改（加了 fixture alias），改这里要按共享工作树规则只动 `build.rollupOptions` 段、不碰它的 alias hunk。

**Step 4 · 体积护栏防回潮**
build 后校验入口 chunk gzip ≤ 阈值（Step 0 达成后按实际水位定，暂定 300 KB 起、逐步收紧），超限 fail，挂进 `verify:web`。风格对齐 `status-labels.guard.test.ts` / `design-import.guard.test.ts` / `navigation-rules.test.ts`。

### 4.3 验收

- 入口 chunk gzip 相对 Step 0 基线明显下降（xyflow+dicebear 两块离开入口即为硬指标）；
- 真实浏览器：流程图/计划图 tab 正常渲染（Step 1）、头像正常显示无长时空白（Step 2）；
- 护栏在人为超限时确实 fail；
- `verify:web` 全绿。

### 4.4 明确不做（本轮）

- 不碰 monaco/recharts/markdown/pinyin（已不在入口）；
- 不追 teams 路由 348 KB（已独立分块，非首屏阻塞）；
- 2b 服务端头像若审核未选则记为后续；
- 不引入新的构建框架/打包器。

---

## 5. 建议顺序与工作量

| 顺序 | 子域 | 粗估 | 理由 |
|---|---|---|---|
| 1 | **C1 池配置** | 0.5 天 | 一次改动、零风险、立刻抬高天花板 |
| 2 | **B2+B3 保留作业 + 一次性清理** | 2~3 天 | 止住无限增长，回收 137MB |
| 3 | **C2 SSE 去轮询** | 2~3 天 | 消掉随用户数线性增长的 DB 负载 |
| 4 | **A 预算熔断（简化版）** | 1.5~2 天 | 人类砍到只剩「项目额度列 + SUM 聚合 + pre-dispatch 单闸 + 前端禁用」，无 ledger/软闸/金额 |
| 5 | **D1~D4 前端瘦身** | 1~2 天 | 独立、低风险，可与任何一项并行 |

C1 已完成（2026-07-25），B2+B3、C2 已完成（2026-07-25）；剩 A、D。B4/B5 为第二梯队。

---

## 6. 明确不在本 spec 范围

同批体检的 P0 三项**不在这里**，它们比本 spec 的任何一项都更影响平台能否进生产，但性质是工程基建而非功能设计：

1. **无 CI、无 Dockerfile、无部署清单** —— 全仓只有 `docker-compose.dev.yml`
2. **CP 硬单实例** —— `runtime.ConnectionRegistry` 是进程内 map（`connection.go:33`），派发前 `IsConnected` 判断（`run_service.go:248`）；后台 reconciler 无 leader 选举
3. **零可观测性** —— go.mod 无 prometheus/otel，无 `/metrics`，`/health` 不探 DB/Temporal

本 spec 的 B2 单实例保护、C2 的多副本友好性，都是为 P0#2 预留的接缝。

## 7. 拍板结论（2026-07-25）

| # | 议题 | 结论 |
|---|---|---|
| 1 | 保留分类表 | **人类授权"该删就删"**，定稿见 §2.2 B1；额外增加"已软删项目满 30 天连同其事实表行一并清除"规则 |
| 2 | 清理 177,090 条历史 authz 放行 | 含在 #1 授权内，按 B3 执行；**保留 19 条 failed** |
| 3 | 预算口径 | **`total`（项目总量）**。周期口径需要窗口重置与跨期未结语义，token 维度下总量最简单且够用；日后加周期不影响本期表结构 |
| 4 | 金额维度 | **本期不做，只做 token 维度**，细则见 §3.2 |
| 5 | 顺序 / P0 | 认可 §5 顺序；**P0 三项人类明确表示当前不是优先级**，不插队，本 spec 按序推进 |
| 6 | 预算方案复杂度 | **人类要简单版**（2026-07-25 追加）：只保留「项目 token 额度 + SUM 现有 attempt 消耗 + pre-dispatch 单闸拦开新工 + 前端禁用发起」；**砍掉 ledger、软闸/运行中打断、attempt 级限额传播**。已核实 `budget_consumed_tokens` 由心跳实时累加，无需 ledger。详见 §3.2 简化版 |

### 7.1 因决定而收窄的范围

- 不引入 provider 单价表 / `provider.pricing` 配置
- `projects` 只加 `budget_token_limit` 一列，不加 `budget_cost_limit` / `budget_period` / attempt 级限额列
- `project_budget_ledger` 保持死表，不复活（已消耗直接 SUM `project_task_attempts.budget_consumed_tokens`）
- 不做软闸，运行中任务不打断；`service.go:798` 维持只看 wall clock
- 不做 `cost_exceeded` / `token_exceeded` 心跳跳闸原因；唯一闸是 pre-dispatch 的 `budget_exhausted`
- cost 模块 `status IN (...)` 老毛病本期不碰
