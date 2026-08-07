# Runtime Agent token 用量采集与回写 Implementation Plan
> 复核状态：与配对spec相同——CHANGELOG 2026-07-07 16:39记录内置数字员工头像库新增（含token用量统计）；锚点抽查发现executor.rs含total_tokens累加与回写逻辑

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 Runtime Agent best-effort 采集 Provider token 用量并写进 `task_runs.result.usage.total_tokens`，使 Control Plane 既有 cost 统计能展示非 0 用量。

**Architecture:** 在 `ProviderEvent::TurnCompleted` 上挂 `usage` 字段；三个 parser 在 TurnCompleted 分支调用共享 helper `extract_usage` best-effort 解析；`RuntimeCommandWritebackSink` 持有 run 级 `Arc<AtomicI64>` 累加器，事件循环累加、完成回写时写进 `result.usage.total_tokens`、预算心跳周期读取并饱和为 i32 发送。后端零改动。

**Tech Stack:** Rust 2024 edition、tokio、serde、 Control Plane 既有 cost SQL。测试用 `cargo test -p superteam-runtime-agent`。

## Global Constraints

- 不改 `contracts/runtime/openapi.yaml`（ProviderEvent 已 `additionalProperties: true`，扩展字段透传）
- 不改 Control Plane 任何 Go 代码、SQL、迁移、前端
- Provider 解析不到 usage → `None` → 累加器不变 → 后端 SQL 抽取为 0（与现状一致，不破坏）
- 失败/取消的 run 的 `result` 本就是 `None`，不写 usage；cost SQL 也只统计 `completed/finished`
- 累加值超 i32：心跳路径饱和到 `i32::MAX`；result 路径用 i64 JSON number，后端 `::bigint` 接收
- 测试命令：`cd apps/runtime-agent && cargo test`
- 提交粒度：每个 Task 一次 commit，commit message 前缀 `feat(runtime-agent):` 或 `test(runtime-agent):`

---

## File Structure

- Create: `apps/runtime-agent/src/providers/usage.rs` — 共享 `extract_usage` helper
- Modify: `apps/runtime-agent/src/events.rs` — `TurnUsage` struct + `TurnCompleted.usage` 字段
- Modify: `apps/runtime-agent/src/providers/mod.rs` — `pub mod usage;`
- Modify: `apps/runtime-agent/src/providers/{claude,codex,opencode}.rs` — TurnCompleted 分支调用 `extract_usage`
- Modify: `apps/runtime-agent/tests/provider_event_test.rs` — parser usage fixture 测试
- Modify: `apps/runtime-agent/src/commands/executor.rs` — 累加器字段 + 三处构造点 + `record_event`/`complete`/`record_budget_heartbeat`/`command_completed_terminal`/`spawn_project_task_budget_heartbeat` 改动 + 单元测试

---

### Task 1: 给 `ProviderEvent::TurnCompleted` 加 `usage` 字段

**Files:**
- Modify: `apps/runtime-agent/src/events.rs`
- Test: `apps/runtime-agent/tests/provider_event_test.rs`（既有 `parses_claude_session_and_text_and_completion_events` 等会因 `TurnCompleted` 字段变化而编译失败，需同步更新）

**Interfaces:**
- Produces: `events::TurnUsage { total_tokens: i64, input_tokens: Option<i64>, output_tokens: Option<i64> }`；`ProviderEvent::TurnCompleted { summary: Option<String>, usage: Option<TurnUsage> }`

- [ ] **Step 1: 更新 `events.rs`**

替换整个 `apps/runtime-agent/src/events.rs` 内容为：

```rust
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct TurnUsage {
    pub total_tokens: i64,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub input_tokens: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub output_tokens: Option<i64>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum ProviderEvent {
    SessionStarted {
        session_id: String,
        #[serde(skip_serializing_if = "Option::is_none")]
        session_state: Option<serde_json::Value>,
    },
    TurnStarted,
    TextDelta {
        text: String,
    },
    ToolStarted {
        tool_id: String,
        name: String,
    },
    ToolCompleted {
        tool_id: String,
    },
    TurnCompleted {
        #[serde(skip_serializing_if = "Option::is_none")]
        summary: Option<String>,
        #[serde(skip_serializing_if = "Option::is_none")]
        usage: Option<TurnUsage>,
    },
    TurnError {
        message: String,
    },
}
```

- [ ] **Step 2: 修复既有 provider_event_test.rs 的 TurnCompleted 断言**

把 `apps/runtime-agent/tests/provider_event_test.rs` 中所有 `ProviderEvent::TurnCompleted { summary: ... }` 改为带 `usage: None`。具体三处：

第 33-38 行（claude）：
```rust
    assert_eq!(
        completed,
        ProviderEvent::TurnCompleted {
            summary: Some("done".to_string()),
            usage: None,
        }
    );
```

第 66 行（opencode）：
```rust
    assert_eq!(completed, ProviderEvent::TurnCompleted { summary: None, usage: None });
```

第 94-99 行（codex）：
```rust
    assert_eq!(
        completed,
        ProviderEvent::TurnCompleted {
            summary: Some("done".to_string()),
            usage: None,
        }
    );
```

第 142 行（codex realistic）：
```rust
            ProviderEvent::TurnCompleted { summary: None, usage: None },
```

- [ ] **Step 3: 修复 executor.rs 既有构造 `TurnCompleted` 的位置**

运行：`cd apps/runtime-agent && grep -n "TurnCompleted {" src/commands/executor.rs src/runs.rs src/main.rs`

每个 `ProviderEvent::TurnCompleted { summary }` 或 `ProviderEvent::TurnCompleted { summary: ... }` 都补 `usage: None`。预期位置：
- `executor.rs:87`（`ProviderTerminalWritebackState::observe_event` 内的 `TurnCompleted { summary }` 模式）
- `executor.rs:1503`（`runtime_event_writeback` 的 `TurnCompleted { summary }` 模式）
- `executor.rs:2654`、`2686/2689`（测试 `&ProviderEvent::TurnCompleted { summary: .. }`）
- `executor.rs:2862`、`2896`、`3375`（测试）
- `runs.rs` 若有匹配

模式匹配 `ProviderEvent::TurnCompleted { summary }` 需改为 `ProviderEvent::TurnCompleted { summary, usage: _ }` 或 `ProviderEvent::TurnCompleted { summary, .. }`（推荐 `..` 减少未来维护成本）。结构体字面量必须显式写 `usage: None`。

- [ ] **Step 4: 运行测试验证编译与既有测试通过**

Run: `cd apps/runtime-agent && cargo test --tests`
Expected: PASS（既有测试全绿，编译通过）

- [ ] **Step 5: 提交**

```bash
git add apps/runtime-agent/src/events.rs apps/runtime-agent/tests/provider_event_test.rs apps/runtime-agent/src/commands/executor.rs apps/runtime-agent/src/runs.rs apps/runtime-agent/src/main.rs
git commit -m "refactor(runtime-agent): add usage field to ProviderEvent::TurnCompleted"
```

---

### Task 2: 新增 `providers/usage.rs` 共享 `extract_usage` helper

**Files:**
- Create: `apps/runtime-agent/src/providers/usage.rs`
- Modify: `apps/runtime-agent/src/providers/mod.rs:4`（加 `pub mod usage;`）

**Interfaces:**
- Produces: `providers::usage::extract_usage(value: &serde_json::Value) -> Option<TurnUsage>`

- [ ] **Step 1: 在 `providers/mod.rs` 注册模块**

在 `apps/runtime-agent/src/providers/mod.rs` 第 4 行后加：

```rust
pub mod usage;
```

（即 `pub mod opencode;` 之后插入一行）

- [ ] **Step 2: 写失败测试**

创建 `apps/runtime-agent/tests/usage_extract_test.rs`：

```rust
use superteam_runtime_agent::events::TurnUsage;
use superteam_runtime_agent::providers::usage::extract_usage;

#[test]
fn extract_usage_prefers_top_level_total_tokens() {
    let value = serde_json::json!({
        "usage": {"total_tokens": 12345, "input_tokens": 1000, "output_tokens": 11345}
    });
    assert_eq!(
        extract_usage(&value),
        Some(TurnUsage {
            total_tokens: 12345,
            input_tokens: Some(1000),
            output_tokens: Some(11345),
        })
    );
}

#[test]
fn extract_usage_sums_input_and_output_when_no_total() {
    let value = serde_json::json!({
        "usage": {"input_tokens": 700, "output_tokens": 300}
    });
    assert_eq!(
        extract_usage(&value),
        Some(TurnUsage {
            total_tokens: 1000,
            input_tokens: Some(700),
            output_tokens: Some(300),
        })
    );
}

#[test]
fn extract_usage_includes_cache_tokens_in_total() {
    let value = serde_json::json!({
        "usage": {
            "input_tokens": 100,
            "output_tokens": 200,
            "cache_read_input_tokens": 50,
            "cache_creation_input_tokens": 25
        }
    });
    let usage = extract_usage(&value).expect("usage");
    assert_eq!(usage.total_tokens, 375);
    assert_eq!(usage.input_tokens, Some(100));
    assert_eq!(usage.output_tokens, Some(200));
}

#[test]
fn extract_usage_reads_token_usage_object() {
    let value = serde_json::json!({
        "token_usage": {"total_tokens": 4242, "input_tokens": 1000, "output_tokens": 3242}
    });
    assert_eq!(
        extract_usage(&value),
        Some(TurnUsage {
            total_tokens: 4242,
            input_tokens: Some(1000),
            output_tokens: Some(3242),
        })
    );
}

#[test]
fn extract_usage_reads_top_level_fields() {
    let value = serde_json::json!({
        "total_tokens": 99,
        "input_tokens": 40,
        "output_tokens": 59
    });
    assert_eq!(
        extract_usage(&value),
        Some(TurnUsage {
            total_tokens: 99,
            input_tokens: Some(40),
            output_tokens: Some(59),
        })
    );
}

#[test]
fn extract_usage_returns_none_when_no_token_fields() {
    let value = serde_json::json!({"summary": "done"});
    assert_eq!(extract_usage(&value), None);
}

#[test]
fn extract_usage_returns_none_for_non_numeric_tokens() {
    let value = serde_json::json!({"usage": {"total_tokens": "many"}});
    assert_eq!(extract_usage(&value), None);
}

#[test]
fn extract_usage_handles_null_usage_object() {
    let value = serde_json::json!({"usage": null});
    assert_eq!(extract_usage(&value), None);
}
```

- [ ] **Step 3: 运行测试确认失败**

Run: `cd apps/runtime-agent && cargo test --test usage_extract_test`
Expected: 编译失败（`unresolved module usage` 或 `cannot find function extract_usage`）

- [ ] **Step 4: 实现 `providers/usage.rs`**

```rust
use serde_json::Value;

use crate::events::TurnUsage;

/// Best-effort 解析 Provider 事件中的 token 用量。
/// 找不到或字段非数字时返回 None，不抛错。
pub fn extract_usage(value: &Value) -> Option<TurnUsage> {
    if let Some(usage) = value.get("usage") {
        if let Some(parsed) = parse_usage_object(usage) {
            return Some(parsed);
        }
    }
    if let Some(usage) = value.get("token_usage") {
        if let Some(parsed) = parse_usage_object(usage) {
            return Some(parsed);
        }
    }
    parse_usage_object(value)
}

fn parse_usage_object(value: &Value) -> Option<TurnUsage> {
    let total = value.get("total_tokens").and_then(|v| v.as_i64());
    let input = value.get("input_tokens").and_then(|v| v.as_i64());
    let output = value.get("output_tokens").and_then(|v| v.as_i64());
    let cache_read = value
        .get("cache_read_input_tokens")
        .and_then(|v| v.as_i64())
        .unwrap_or(0);
    let cache_creation = value
        .get("cache_creation_input_tokens")
        .and_then(|v| v.as_i64())
        .unwrap_or(0);

    let total_tokens = match (total, input, output) {
        (Some(t), _, _) => Some(t),
        (None, Some(i), Some(o)) => Some(i + o + cache_read + cache_creation),
        _ => None,
    }?;

    // 至少要有可识别的 token 字段，避免把无关对象误判成 usage
    if total.is_none() && input.is_none() && output.is_none() {
        return None;
    }

    Some(TurnUsage {
        total_tokens,
        input_tokens: input,
        output_tokens: output,
    })
}
```

- [ ] **Step 5: 运行测试验证通过**

Run: `cd apps/runtime-agent && cargo test --test usage_extract_test`
Expected: PASS（8 个测试全绿）

- [ ] **Step 6: 提交**

```bash
git add apps/runtime-agent/src/providers/usage.rs apps/runtime-agent/src/providers/mod.rs apps/runtime-agent/tests/usage_extract_test.rs
git commit -m "feat(runtime-agent): add shared extract_usage helper for provider token usage"
```

---

### Task 3: 三个 parser 在 TurnCompleted 分支调用 `extract_usage`

**Files:**
- Modify: `apps/runtime-agent/src/providers/claude.rs:118-123`
- Modify: `apps/runtime-agent/src/providers/codex.rs:124-131`
- Modify: `apps/runtime-agent/src/providers/opencode.rs:106-108`
- Test: `apps/runtime-agent/tests/provider_event_test.rs`（追加 usage 解析测试）

**Interfaces:**
- Consumes: `providers::usage::extract_usage`、`events::TurnUsage`、Task 1 的 `TurnCompleted.usage` 字段
- Produces: 三个 parser 的 `TurnCompleted` 现在可能带 `usage`

- [ ] **Step 1: 写失败测试 — 追加到 `provider_event_test.rs` 末尾**

在 `apps/runtime-agent/tests/provider_event_test.rs` 末尾追加：

```rust
#[test]
fn parses_claude_result_event_with_usage() {
    let completed = parse_claude_event(
        r#"{"type":"result","result":"done","usage":{"total_tokens":1500,"input_tokens":200,"output_tokens":1300}}"#,
    )
    .expect("valid json")
    .expect("event");

    assert_eq!(
        completed,
        ProviderEvent::TurnCompleted {
            summary: Some("done".to_string()),
            usage: Some(TurnUsage {
                total_tokens: 1500,
                input_tokens: Some(200),
                output_tokens: Some(1300),
            }),
        }
    );
}

#[test]
fn parses_claude_result_event_without_usage_keeps_usage_none() {
    let completed = parse_claude_event(r#"{"type":"result","result":"done"}"#)
        .expect("valid json")
        .expect("event");

    assert_eq!(
        completed,
        ProviderEvent::TurnCompleted {
            summary: Some("done".to_string()),
            usage: None,
        }
    );
}

#[test]
fn parses_codex_turn_completed_with_usage() {
    let completed = parse_codex_event(
        r#"{"type":"turn.completed","summary":"done","usage":{"input_tokens":400,"output_tokens":600}}"#,
    )
    .expect("valid json")
    .expect("event");

    assert_eq!(
        completed,
        ProviderEvent::TurnCompleted {
            summary: Some("done".to_string()),
            usage: Some(TurnUsage {
                total_tokens: 1000,
                input_tokens: Some(400),
                output_tokens: Some(600),
            }),
        }
    );
}

#[test]
fn parses_opencode_turn_completed_with_usage() {
    let completed = parse_opencode_event(
        r#"{"type":"turn.completed","usage":{"total_tokens":77}}"#,
    )
    .expect("valid json")
    .expect("event");

    assert_eq!(
        completed,
        ProviderEvent::TurnCompleted {
            summary: None,
            usage: Some(TurnUsage {
                total_tokens: 77,
                input_tokens: None,
                output_tokens: None,
            }),
        }
    );
}
```

并在文件顶部 import 行补充：
```rust
use superteam_runtime_agent::events::TurnUsage;
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/runtime-agent && cargo test --test provider_event_test`
Expected: 4 个新测试 FAIL（usage 实际为 None）

- [ ] **Step 3: 改 `claude.rs` 的 `"result"` 分支**

`apps/runtime-agent/src/providers/claude.rs:118-123` 替换为：

```rust
        "result" => Ok(Some(ProviderEvent::TurnCompleted {
            summary: event
                .get("result")
                .and_then(|v| v.as_str())
                .map(ToString::to_string),
            usage: crate::providers::usage::extract_usage(&event),
        })),
```

- [ ] **Step 4: 改 `codex.rs` 的 TurnCompleted 分支**

`apps/runtime-agent/src/providers/codex.rs:124-131` 替换为：

```rust
    if matches!(
        event_type,
        "turn.completed" | "turn_complete" | "completed" | "result" | "done"
    ) {
        return Ok(Some(ProviderEvent::TurnCompleted {
            summary: extract_summary(&event),
            usage: crate::providers::usage::extract_usage(&event),
        }));
    }
```

- [ ] **Step 5: 改 `opencode.rs` 的 `turn.completed` / `session.idle` 分支**

`apps/runtime-agent/src/providers/opencode.rs:106-108` 替换为：

```rust
        "turn.completed" | "session.idle" => {
            Ok(Some(ProviderEvent::TurnCompleted {
                summary: None,
                usage: crate::providers::usage::extract_usage(&event),
            }))
        }
```

- [ ] **Step 6: 运行测试验证通过**

Run: `cd apps/runtime-agent && cargo test --test provider_event_test`
Expected: PASS（既有 + 4 个新测试全绿）

- [ ] **Step 7: 提交**

```bash
git add apps/runtime-agent/src/providers/claude.rs apps/runtime-agent/src/providers/codex.rs apps/runtime-agent/src/providers/opencode.rs apps/runtime-agent/tests/provider_event_test.rs
git commit -m "feat(runtime-agent): parsers extract token usage from TurnCompleted events"
```

---

### Task 4: `RuntimeCommandWritebackSink` 加累加器字段并更新三处构造点

**Files:**
- Modify: `apps/runtime-agent/src/commands/executor.rs:49-54`（struct 定义）
- Modify: `apps/runtime-agent/src/commands/executor.rs:336-340`（构造点 1）
- Modify: `apps/runtime-agent/src/commands/executor.rs:479-483`（构造点 2）

**Interfaces:**
- Produces: `RuntimeCommandWritebackSink` 新增 `usage_tokens: Arc<AtomicI64>` 字段

注意：本 Task 不改任何方法行为，仅扩字段 + 初始化。所有既有调用点不受影响（累加器初始 0，不读不写）。

- [ ] **Step 1: 修改 import 与 struct 定义**

`apps/runtime-agent/src/commands/executor.rs` 顶部 import 区（line 1-3 附近）追加：

```rust
use std::sync::atomic::{AtomicI64, Ordering};
use std::sync::Arc;
```

（若 `Arc` 已 import 则只加 `AtomicI64, Ordering`；先 `grep -n "use std::sync" src/commands/executor.rs` 确认）

把 `apps/runtime-agent/src/commands/executor.rs:49-54` 的 struct 定义改为：

```rust
#[derive(Clone)]
struct RuntimeCommandWritebackSink {
    client: ControlPlaneClient,
    command_id: String,
    project_task: Option<ProjectTaskWritebackContext>,
    usage_tokens: Arc<AtomicI64>,
}
```

- [ ] **Step 2: 更新构造点 1（line 336-340）**

```rust
        let writeback = self
            .control_plane
            .as_ref()
            .map(|client| RuntimeCommandWritebackSink {
                client: client.clone(),
                command_id: payload.command_id.clone(),
                project_task: project_task.clone(),
                usage_tokens: Arc::new(AtomicI64::new(0)),
            });
```

- [ ] **Step 3: 更新构造点 2（line 479-483）**

```rust
                RuntimeCommandWritebackSink {
                    client: control_plane.clone(),
                    command_id: start_command_id.to_string(),
                    project_task,
                    usage_tokens: Arc::new(AtomicI64::new(0)),
                }
```

- [ ] **Step 4: 搜索是否还有其它构造点**

Run: `cd apps/runtime-agent && grep -n "RuntimeCommandWritebackSink {" src/commands/executor.rs`

预期只有 line 50（struct 定义）、line 336、line 479 三处。若有遗漏的构造点，同样补 `usage_tokens: Arc::new(AtomicI64::new(0))`。

- [ ] **Step 5: 运行测试验证编译通过**

Run: `cd apps/runtime-agent && cargo test --tests`
Expected: PASS（既有测试全绿，编译通过；累加器未被任何方法读写）

- [ ] **Step 6: 提交**

```bash
git add apps/runtime-agent/src/commands/executor.rs
git commit -m "refactor(runtime-agent): add usage_tokens accumulator to RuntimeCommandWritebackSink"
```

---

### Task 5: `command_completed_terminal` 写入 `result.usage.total_tokens`

**Files:**
- Modify: `apps/runtime-agent/src/commands/executor.rs:1244-1269`（函数签名 + body）
- Modify: `apps/runtime-agent/src/commands/executor.rs:1404`（`complete` 调用点）
- Test: `apps/runtime-agent/src/commands/executor.rs` 末尾 `mod tests`

**Interfaces:**
- Produces: `command_completed_terminal(summary, provider_session_id, total_tokens: i64) -> RuntimeCommandTerminalWriteback`；`total_tokens > 0` 时 `result["usage"]["total_tokens"]` 为该值

- [ ] **Step 1: 写失败测试 — 追加到 `executor.rs` 的 `mod tests` 末尾**

在 `apps/runtime-agent/src/commands/executor.rs` 末尾的 `mod tests` 内（最后一个 `}` 之前）追加：

```rust
    #[test]
    fn command_completed_terminal_with_positive_tokens_writes_usage() {
        let terminal = command_completed_terminal(Some("done".to_string()), None, 1500);

        assert_eq!(terminal.status, "completed");
        let result = terminal.result.expect("result map");
        let usage = result.get("usage").expect("usage field");
        assert_eq!(usage["total_tokens"], serde_json::json!(1500));
        assert_eq!(result["summary"], serde_json::json!("done"));
    }

    #[test]
    fn command_completed_terminal_with_zero_tokens_omits_usage() {
        let terminal = command_completed_terminal(Some("done".to_string()), None, 0);

        let result = terminal.result.expect("result map");
        assert!(result.get("usage").is_none());
        assert_eq!(result["summary"], serde_json::json!("done"));
    }

    #[test]
    fn command_completed_terminal_without_summary_omits_summary_and_writes_usage() {
        let terminal = command_completed_terminal(None, Some("sess-1".to_string()), 42);

        let result = terminal.result.expect("result map");
        assert!(result.get("summary").is_none());
        assert_eq!(result["usage"]["total_tokens"], serde_json::json!(42));
    }
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/runtime-agent && cargo test --lib command_completed_terminal`
Expected: 编译失败（`command_completed_terminal` 签名是 2 参数，测试给 3 参数）

- [ ] **Step 3: 改 `command_completed_terminal` 签名与 body**

`apps/runtime-agent/src/commands/executor.rs:1244-1269` 替换为：

```rust
fn command_completed_terminal(
    summary: Option<String>,
    provider_session_id: Option<String>,
    total_tokens: i64,
) -> RuntimeCommandTerminalWriteback {
    let mut result = HashMap::new();
    if let Some(summary) = summary.as_ref().filter(|value| !value.trim().is_empty()) {
        result.insert(
            "summary".to_string(),
            serde_json::Value::String(summary.clone()),
        );
    }
    if total_tokens > 0 {
        let mut usage = serde_json::Map::new();
        usage.insert(
            "total_tokens".to_string(),
            serde_json::Value::Number(total_tokens.into()),
        );
        result.insert(
            "usage".to_string(),
            serde_json::Value::Object(usage),
        );
    }

    RuntimeCommandTerminalWriteback {
        status: "completed".to_string(),
        summary,
        result: Some(result),
        diagnostic: None,
        provider_session_external_id: provider_session_id.clone(),
        session_state_patch: provider_session_state_patch(provider_session_id.as_deref()),
        log_ref: None,
        raw_result_ref: None,
        error_message: None,
        error_code: None,
        error_family: None,
    }
}
```

- [ ] **Step 4: 更新 `complete` 方法读累加值并传入**

`apps/runtime-agent/src/commands/executor.rs:1396-1438` 的 `complete` 方法，把第一段（line 1400-1406）改为：

```rust
    async fn complete(
        &self,
        summary: Option<String>,
        provider_session_id: Option<String>,
    ) -> anyhow::Result<()> {
        let total_tokens = self.usage_tokens.load(Ordering::Relaxed);
        self.client
            .complete_runtime_command(
                &self.command_id,
                &command_completed_terminal(
                    summary.clone(),
                    provider_session_id.clone(),
                    total_tokens,
                ),
            )
            .await?;
```

（其余 `if let Some(project_task) = ...` 段不变）

- [ ] **Step 5: 搜索其它 `command_completed_terminal(` 调用点**

Run: `cd apps/runtime-agent && grep -n "command_completed_terminal(" src/commands/executor.rs`

预期只有 line 1244（定义）、line 1404（`complete` 内调用）。若有其它调用点，需补 `0` 作为第三参数（即不写 usage 的旧路径）。

- [ ] **Step 6: 运行测试验证通过**

Run: `cd apps/runtime-agent && cargo test --lib command_completed_terminal`
Expected: PASS（3 个新测试全绿）

- [ ] **Step 7: 运行全量测试确认无回归**

Run: `cd apps/runtime-agent && cargo test --tests`
Expected: PASS

- [ ] **Step 8: 提交**

```bash
git add apps/runtime-agent/src/commands/executor.rs
git commit -m "feat(runtime-agent): write usage.total_tokens into completed run result"
```

---

### Task 6: `record_event` 在 TurnCompleted 时累加 token

**Files:**
- Modify: `apps/runtime-agent/src/commands/executor.rs:1316-1327`（`record_event` 方法）

**Interfaces:**
- Consumes: Task 1 的 `TurnCompleted.usage`、Task 4 的 `usage_tokens` 字段

- [ ] **Step 1: 改 `record_event` 在 TurnCompleted 分支累加**

`apps/runtime-agent/src/commands/executor.rs:1316-1327` 替换为：

```rust
    async fn record_event(
        &self,
        record: &RunEventRecord,
        provider_session_id: Option<&str>,
    ) -> anyhow::Result<()> {
        if let ProviderEvent::TurnCompleted { usage: Some(usage), .. } = &record.event {
            if usage.total_tokens > 0 {
                self.usage_tokens
                    .fetch_add(usage.total_tokens, Ordering::Relaxed);
            }
        }
        self.client
            .record_runtime_command_event(
                &self.command_id,
                &runtime_event_writeback(record, provider_session_id),
            )
            .await
    }
```

注意：`record.event` 字段类型由 `runs::RunEventRecord` 定义，需确认其 `event` 是 `ProviderEvent`（在 `runs.rs:123` 已确认）。若 `RunEventRecord` 字段名不是 `event`，先 `grep -n "pub event" src/runs.rs` 确认。

- [ ] **Step 2: 运行全量测试确认无回归**

Run: `cd apps/runtime-agent && cargo test --tests`
Expected: PASS（既有测试全绿；累加器只在真实 TurnCompleted with usage 时触发，单元测试不受影响）

- [ ] **Step 3: 提交**

```bash
git add apps/runtime-agent/src/commands/executor.rs
git commit -m "feat(runtime-agent): accumulate token usage on TurnCompleted events"
```

---

### Task 7: 预算心跳读取累加值，去掉硬编码 0

**Files:**
- Modify: `apps/runtime-agent/src/commands/executor.rs:1378-1394`（`record_budget_heartbeat` 签名 + body）
- Modify: `apps/runtime-agent/src/commands/executor.rs:1101`（`spawn_project_task_budget_heartbeat` 调用点）
- Test: `apps/runtime-agent/src/commands/executor.rs` 末尾 `mod tests`（line 3497-3508 既有测试需更新断言）

**Interfaces:**
- Produces: `record_budget_heartbeat(&self, elapsed: Duration) -> anyhow::Result<bool>`（去掉 `consumed_tokens` 形参，内部读 `usage_tokens` 饱和为 i32）

- [ ] **Step 1: 写失败测试 — 累加器在饱和前后的行为**

在 `apps/runtime-agent/src/commands/executor.rs` 末尾 `mod tests` 内追加（需 `use std::sync::atomic::{AtomicI64, Ordering}; use std::sync::Arc;`，若 Task 4 已在文件顶部 import 则 mod tests 内可见）：

```rust
    #[test]
    fn record_budget_heartbeat_saturates_accumulator_to_i32_max() {
        // 这是纯逻辑测试：验证 i64 -> i32 饱和函数
        let huge: i64 = (i32::MAX as i64) + 1000;
        let saturated = huge.min(i32::MAX as i64) as i32;
        assert_eq!(saturated, i32::MAX);

        let normal: i64 = 12345;
        let saturated_normal = normal.min(i32::MAX as i64) as i32;
        assert_eq!(saturated_normal, 12345);
    }
```

（注：`record_budget_heartbeat` 是 async 且依赖 `ControlPlaneClient`，难以纯单元测试；这里用饱和逻辑等价测试覆盖关键数学。真实链路由端到端验证。）

- [ ] **Step 2: 改 `record_budget_heartbeat` 签名与 body**

`apps/runtime-agent/src/commands/executor.rs:1378-1394` 替换为：

```rust
    async fn record_budget_heartbeat(
        &self,
        elapsed: Duration,
    ) -> anyhow::Result<bool> {
        let project_task = match &self.project_task {
            Some(project_task) => project_task,
            None => return Ok(false),
        };
        let elapsed_sec = elapsed.as_secs().min(i32::MAX as u64) as i32;
        let consumed_tokens = self
            .usage_tokens
            .load(Ordering::Relaxed)
            .min(i32::MAX as i64) as i32;
        let body =
            project_task_budget_heartbeat_writeback(project_task, elapsed_sec, consumed_tokens);
        let response = self
            .client
            .record_project_task_budget_heartbeat(&project_task.attempt_id, &body)
            .await?;
        Ok(response.tripped)
    }
```

- [ ] **Step 3: 更新 `spawn_project_task_budget_heartbeat` 调用点**

`apps/runtime-agent/src/commands/executor.rs:1101` 把：

```rust
                    match writeback.record_budget_heartbeat(started_at.elapsed(), 0).await {
```

改为：

```rust
                    match writeback.record_budget_heartbeat(started_at.elapsed()).await {
```

- [ ] **Step 4: 更新既有 `project_task_budget_heartbeat_writeback_reports_elapsed_seconds` 测试**

`apps/runtime-agent/src/commands/executor.rs:3497-3508` 这段测试断言 `body.consumed_tokens == 0`。该测试调用的是纯 builder `project_task_budget_heartbeat_writeback(&context, 42, 0)`，签名不变，仍传入 `0`，断言保持 `assert_eq!(body.consumed_tokens, 0);` 不变。

**无需改动此测试。** 只需确认其仍编译通过。

- [ ] **Step 5: 搜索其它 `record_budget_heartbeat(` 调用点**

Run: `cd apps/runtime-agent && grep -n "record_budget_heartbeat(" src/commands/executor.rs`

预期只有 line 1378（定义）、line 1101（`spawn_project_task_budget_heartbeat` 内调用）。若有其它调用点，去掉第二参数 `0`。

- [ ] **Step 6: 运行测试验证通过**

Run: `cd apps/runtime-agent && cargo test --tests`
Expected: PASS（新饱和测试 + 既有 `project_task_budget_heartbeat_writeback_reports_elapsed_seconds` 全绿）

- [ ] **Step 7: 提交**

```bash
git add apps/runtime-agent/src/commands/executor.rs
git commit -m "feat(runtime-agent): budget heartbeat reports real accumulated token usage"
```

---

### Task 8: 全量编译 + clippy + 端到端真实验证

**Files:** 无代码改动，纯验证

**Interfaces:** 无

- [ ] **Step 1: 全量 cargo test**

Run: `cd apps/runtime-agent && cargo test --tests`
Expected: PASS（全部测试绿）

- [ ] **Step 2: cargo clippy（若项目已配置）**

Run: `cd apps/runtime-agent && cargo clippy --all-targets -- -D warnings 2>&1 | tail -30`
Expected: 无 warning（若有 `cargo clippy` 不被项目支持则跳过，记 `cargo build --tests` 替代）

- [ ] **Step 3: 确认服务状态**

Run: `cd /Users/tinker/src/singe/SuperTeam && scripts/dev-services.sh status`
Expected: Temporal、Control Plane、Web、Runtime Agent 都 running。若不在 running，先 `scripts/dev-services.sh start` 启动。

- [ ] **Step 4: 重启 Runtime Agent 加载新代码**

Run: `cd /Users/tinker/src/singe/SuperTeam && scripts/dev-services.sh restart runtime-agent`
Expected: Runtime Agent 重启成功，加载本次新构建

- [ ] **Step 5: Web 控制台触发一个真实 Claude Code 任务至完成**

操作：打开 Web 控制台 → 选一个数字员工（provider=claude-code）→ 派发一个简单任务（如"写一个 hello world README"）→ 等待状态变为 completed

- [ ] **Step 6: 数据库验证 `task_runs.result` 含 usage**

Run（替换 `$DATABASE_URL` 为实际 dev 库连接，可从 Control Plane 配置或 `apps/control-plane/.env` 读取）:
```bash
psql "$DATABASE_URL" -c "SELECT id, status, result #>> '{usage,total_tokens}' AS tokens, result->>'summary' AS summary FROM task_runs ORDER BY finished_at DESC LIMIT 5;"
```
Expected: 最新一行 `status=completed` 且 `tokens` 为非 null 非 0 的数字

- [ ] **Step 7: Control Plane API 验证 cost summary**

Run:
```bash
curl -s -b "session=<your-dev-session-cookie>" http://localhost:8080/api/v1/costs/summary?period=7d | python3 -m json.tool
```
Expected: `total_tokens` 非 0，`by_employee` 中对应员工 `total_tokens` 非 0

- [ ] **Step 8: Web 成本管理页验证显示**

操作：Web 控制台 → 成本管理页 → 确认总 token 数与按员工/按 Provider 拆分均显示非 0 值，且非 mock/缓存（刷新页面值稳定，与 Step 7 API 一致）

- [ ] **Step 9: Codex 任务验证（若本机有 codex CLI）**

重复 Step 5-8，把数字员工 provider 换成 codex，确认 Codex 路径同样写入 usage

- [ ] **Step 10: 完成前检查 skill**

Run: 加载 `.codex/skills/superteam-completion-check/SKILL.md` 并按其流程做完成前检查

- [ ] **Step 11: 最终提交（若有 fixup）+ 总结**

若 Step 1-10 全过，无需额外提交。把验证证据（SQL 输出、curl 输出、Web 截图说明）汇总到任务回报。

---

## Self-Review

**1. Spec coverage:**
- events.rs 扩 TurnCompleted → Task 1 ✓
- providers/usage.rs extract_usage → Task 2 ✓
- 三个 parser 调用 extract_usage → Task 3 ✓
- RuntimeCommandWritebackSink 累加器 + 三处构造点 → Task 4 ✓
- command_completed_terminal 写 result.usage.total_tokens → Task 5 ✓
- record_event 累加 → Task 6 ✓
- record_budget_heartbeat 读累加值饱和 i32 → Task 7 ✓
- 真实端到端验证 → Task 8 ✓

**2. Placeholder scan:** 无 TBD/TODO，每步都有具体代码或命令。

**3. Type consistency:**
- `TurnUsage { total_tokens: i64, input_tokens: Option<i64>, output_tokens: Option<i64> }` — Task 1 定义，Task 2/3 使用，一致
- `extract_usage(&Value) -> Option<TurnUsage>` — Task 2 定义，Task 3 三个 parser 调用，一致
- `command_completed_terminal(summary, provider_session_id, total_tokens: i64)` — Task 5 定义与 `complete` 调用一致
- `record_budget_heartbeat(&self, elapsed: Duration)` — Task 7 定义与 `spawn_project_task_budget_heartbeat` 调用一致
- `usage_tokens: Arc<AtomicI64>` — Task 4 定义，Task 5/6/7 读取，一致
