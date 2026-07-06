# Runtime Agent token 用量采集与回写

- 日期：2026-07-06
- 范围：Runtime Agent（Rust） + 与之对接的 Control Plane 既有 cost 查询路径
- 目标：让任务执行后，成本管理页能统计到 token 用量（不需要非常精准）
- 非目标：后端 cost 模块 / SQL / API / 前端改造（已验证完整，零改动）；runtime openapi 契约改动；项目任务 `token_limit_exceeded` 熔断判定

## 背景与根因

任务能跑完，但成本管理页 token 用量始终为 0。根因不是后端能力缺失，而是 Runtime Agent 从未把 token 用量写进 `task_runs.result`：

1. `apps/runtime-agent/src/events.rs:5` 的 `ProviderEvent` 枚举没有 token/usage 字段
2. `apps/runtime-agent/src/providers/{claude,codex,opencode}.rs` 的 TurnCompleted 分支只取 `summary`，丢弃了 usage
3. `apps/runtime-agent/src/commands/executor.rs:1244` 的 `command_completed_terminal` 只往 `result` 写 `summary`，没有 `usage.total_tokens`
4. `apps/runtime-agent/src/commands/executor.rs:1101` 的预算心跳硬编码 `consumed_tokens: 0`

后端读取侧（`apps/control-plane/internal/cost/pg_repository.go:11-55`、`employee_execution.sql.go`）的 SQL 完整地从 `task_runs.result #>> '{usage,total_tokens}'` 或 `result->>'total_tokens'` 抽取，`/api/v1/costs/summary`、`/api/v1/costs/employees` 接口齐全，前端 `apps/web/src/lib/api/costs.ts` 已对接。`run_repository_test.go:702` 用 `'{"usage":{"total_tokens":700}}'::jsonb` 验证过 SQL，证明后端只认这个 schema，但生产链路没人写入。

## 设计

单点采集、单点累加、两处回写。在 Runtime Agent 事件流里 best-effort 解析每个 Provider 的 token 用量，用一个 run 级 `Arc<AtomicI64>` 累加器汇总；完成回写时把累加值写进 `task_runs.result.usage.total_tokens`（命中后端 cost SQL），预算心跳时把累加值饱和为 i32 发送给 Control Plane。后端零改动。

### 数据流

```
Provider stdout JSON
  → parse_{claude,codex,opencode}_event 解析 TurnCompleted.usage (best-effort)
  → drain_provider_events 循环
  → writeback.record_event 把 TurnCompleted.usage.total_tokens 累加进 Arc<AtomicI64>
  → 流结束: writeback.complete 读累加值 → command_completed_terminal 写 result.usage.total_tokens
       (并发) spawn_project_task_budget_heartbeat 周期读累加值 → record_budget_heartbeat
  → Control Plane run_writeback.Complete 持久化进 task_runs.result
  → cost pg_repository SQL 读 result #>> '{usage,total_tokens}' → /api/v1/costs/summary
```

## 组件改动

### 1. `apps/runtime-agent/src/events.rs` — 扩 `TurnCompleted`

新增：

```rust
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct TurnUsage {
    pub total_tokens: i64,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub input_tokens: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub output_tokens: Option<i64>,
}
```

给 `TurnCompleted` 加 `usage: Option<TurnUsage>`（`skip_serializing_if = "Option::is_none"`）。runtime openapi 契约不动（`additionalProperties: true` 已允许扩展字段透传）。

### 2. 新增 `apps/runtime-agent/src/providers/usage.rs` — 共享解析 helper

```rust
pub fn extract_usage(value: &serde_json::Value) -> Option<TurnUsage>
```

按优先级尝试：
1. `usage.total_tokens`（Claude Code result 事件）
2. `usage.input_tokens + usage.output_tokens`（+ `cache_read_input_tokens` / `cache_creation_input_tokens` 若存在）
3. `token_usage.total_tokens` / `token_usage.{input,output}_tokens`
4. 顶层 `total_tokens` / 顶层 `input_tokens + output_tokens`

找不到返回 `None`。三个 parser 在 TurnCompleted 分支调用它。

### 3. 三个 parser — 在 TurnCompleted 分支调用 `extract_usage`

- `apps/runtime-agent/src/providers/claude.rs:118` 的 `"result"` 分支：`usage: extract_usage(&event)`
- `apps/runtime-agent/src/providers/codex.rs:128` 的 `turn.completed` 等分支：`usage: extract_usage(&event)`
- `apps/runtime-agent/src/providers/opencode.rs:106` 的 `turn.completed`/`session.idle` 分支：`usage: extract_usage(&event)`

解析不到就 `None`，不影响主流程。

### 4. `apps/runtime-agent/src/commands/executor.rs` — 累加器 + 两处回写

- `RuntimeCommandWritebackSink`（line 50）加字段 `usage_tokens: Arc<AtomicI64>`
- 三个构造点（line 336 / 479 / 任何其它构造点）初始化 `Arc::new(AtomicI64::new(0))`
- `record_event`（line 1316）在 `ProviderEvent::TurnCompleted` 分支：若 `usage.total_tokens > 0`，`self.usage_tokens.fetch_add(total_tokens, Ordering::Relaxed)`
- `command_completed_terminal`（line 1244）新增参数 `total_tokens: i64`；当 `> 0` 时 `result.insert("usage", json!({"total_tokens": total_tokens}))`
- `complete`（line 1396）读 `self.usage_tokens.load(Ordering::Relaxed)` 传入 `command_completed_terminal`
- `record_budget_heartbeat`（line 1378）改为内部读累加值（饱和到 `i32::MAX`），去掉 `consumed_tokens` 形参
- `spawn_project_task_budget_heartbeat`（line 1101）调用改为 `record_budget_heartbeat(elapsed)`

## 错误处理 / 边界

- Parser 解析不到 usage → `None` → 累加器不变 → result 无 `usage` 字段 → 后端 SQL 抽取为 0（与现状一致，不破坏）
- 失败/取消的 run：`command_failed_terminal` / `command_cancelled_terminal` 的 `result` 本就是 `None`，不写 usage；cost SQL 也只统计 `completed/finished`，一致
- 多 `TurnCompleted` 累加：Claude 只发 1 个 result 事件（累计用量），Codex 按回合发 delta，求和均正确；OpenCode 若发累计型再单独修 parser
- 累加值超 i32：心跳路径饱和到 `i32::MAX`；result 路径用 i64 JSON number，后端 `::bigint` 接收

## 测试

### 单元测试

- 三个 parser 的 fixture JSON 测试（带 / 不带 usage 两种情形）
- `command_completed_terminal`：`total_tokens > 0` → result 含 `usage.total_tokens`；`total_tokens == 0` → result 不含 `usage`
- 累加器跨多个 `TurnCompleted` 求和
- 心跳：累加值 > `i32::MAX` 时饱和到 `i32::MAX`

### 现有测试不破坏

- `project_task_budget_heartbeat_writeback`（line 1895）保持纯 builder fn 签名不变，line 3498 测试保持绿
- `record_event` / `complete` 既有调用点行为不变（usage 为 None 时累加器不变）

### 真实端到端验证（AGENTS.md 要求）

1. `scripts/dev-services.sh start` 启动 Temporal、Control Plane、Web、Runtime Agent
2. Web 控制台触发一个 Claude Code 真实任务至完成
3. `SELECT result #>> '{usage,total_tokens}' FROM task_runs ORDER BY finished_at DESC LIMIT 5;` 看到 non-null 非 0 值
4. `curl /api/v1/costs/summary?period=7d` 返回 `total_tokens` 非 0
5. Web 成本管理页显示用量（非 mock、非缓存）
6. 同样跑一个 Codex 任务验证（OpenCode 若本机有真实 CLI 也跑一次）

## 不在本次范围

- 后端 cost 模块、SQL、API、前端（已验证完整，零改动）
- runtime openapi 契约（不改）
- 项目任务 `token_limit_exceeded` 熔断逻辑（`apps/control-plane/internal/project/service.go:514` 当前只判 wall_clock；本次只让 `budget_consumed_tokens` 准确，熔断判定留后续）
- per-turn usage 写进 provider session event 列表（契约不改，不做）
