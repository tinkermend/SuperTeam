# Provider Transcript 与 Tool 事件捕获

- 状态：待评审
- 日期：2026-07-09
- 范围：让数字员工的**执行过程**（工具调用与工具结果）离开执行机，进入控制平面与 Web；并把 provider 原始输出流落盘，作为证据链的原材料。
- 依赖方：`docs/superpowers/specs/2026-07-09-evidence-grounding-artifact-collection-design.md`（证据地基）依赖本 spec 的产物 `raw.jsonl`。本 spec 必须先行落地。

## 1. 问题

Web 控制台无法显示任何数字员工的执行过程。这不是前端缺页面，而是**过程数据从未离开执行机，甚至从未被写下来过**。

### 1.1 parser 只认三种事件，其余就地丢弃

`apps/runtime-agent/src/providers/claude.rs:29` 以 `--output-format stream-json` 运行 claude-code。它的 stdout 里确实含有 `tool_use` 与 `tool_result`——包括命令的真实退出码。但 `parse_claude_event`（`claude.rs:78`）只有三个 match 分支：

| stream-json 输入 | 现有处理 |
|---|---|
| `"system"` | → `SessionStarted` |
| `"assistant"` | 只取 `content` 中**第一个含 `text` 的 block**；同消息内的 `tool_use` block 丢弃 |
| `"result"` | → `TurnCompleted` |
| `"user"` | **没有这个分支** → `_ => Ok(None)`。claude-code 的 `tool_result` 正是包在 `type:"user"` 消息里 |

原始 stdout 行由 `providers/mod.rs:120` 的 `BufReader::lines()` 逐行喂给 parser，返回 `None` 即就地丢弃，**从不落盘**。

### 1.2 parser 签名本身装不下 tool 事件

```rust
pub fn parse_claude_event(value: &str) -> anyhow::Result<Option<ProviderEvent>>
```

返回**至多一个**事件。而一条 `assistant` 消息的 `content` 数组里可以同时含 1 个 `text` block 和 N 个 `tool_use` block。

即使补上 `"user"` 分支，这个签名也物理上放不下「一行产出多个事件」。**这不是漏写了一个 match 分支，而是签名层面的结构性缺陷。**

### 1.3 下游管道全通，源头没接

- `events.rs:24-30` 早已定义 `ProviderEvent::ToolStarted { tool_id, name }` 与 `ToolCompleted { tool_id }`。
- `commands/executor.rs:1518`、`1527` 早已在 match 消费这两个变体。
- **没有任何 provider adapter 生产过它们。**

### 1.4 库中的实证

控制平面 `execution_ledger_events` 共 173 行，其中 `provider.event` 97 行，只有四种：

```
text_delta      43
run_completed   18
turn_completed  18
session_started 18
```

**`tool_use`、`tool_result` 零条。**

`{log_dir}/{run_id}/events.jsonl`（`runs.rs:278` `append_event`）保存的是**解析后**的 `ProviderEvent` 流，因此同样不含 tool 事件。**原始流不存在于任何一层。**

后果可在库中直接看到——某条 `run_completed` 的 `output_summary` 是模型自己写的：

> 「证据：命令退出码 `0`；stdout 为 `final-sanity-20260709014431`。」

**那个 `0` 是数字员工声称的。平台从未见过真正的退出码。**

### 1.5 同类系统的解法（paperclip）

`/Users/tinker/src/github/agentic/paperclip` 是同类的多 agent 编排平台，两点直接对照：

**（a）provider 无关的规范化 transcript 联合类型**（`packages/adapter-utils/src/types.ts:450`）：

```ts
export type TranscriptEntry =
  | { kind: "assistant";   ts; text; delta? }
  | { kind: "thinking";    ts; text; delta? }
  | { kind: "user";        ts; text }
  | { kind: "tool_call";   ts; name; input: unknown; toolUseId? }
  | { kind: "tool_result"; ts; toolUseId; toolName?; content: string; isError: boolean }
  | { kind: "init";        ts; model; sessionId }
  | { kind: "result";      ts; text; inputTokens; outputTokens; cachedTokens; costUsd; isError; errors }
  | { kind: "stdout" | "stderr" | "system"; ts; text }
  | { kind: "diff";        ts; changeType: "add"|"remove"|"context"|"hunk"|"file_header"|"truncation"; text };

export type StdoutLineParser = (line: string, ts: string) => TranscriptEntry[];
```

**返回数组**——正是 1.2 所缺的。`tool_result.isError` 是布尔量，由进程写出，不由模型自述。

**（b）transcript 落库，不是只落对象存储**（`heartbeat_run_events`）：

```
(company_id, run_id, agent_id, seq, event_type, stream, level, color, message, payload jsonb)
index (run_id, seq)
```

append-only、按 `seq` 有序。UI 直接读库；对象存储不参与实时显示。

## 2. 目标与非目标

**目标**

1. `tool_use` / `tool_result` 事件流到控制平面并可在 Web 上按 attempt 回看执行过程。
2. `tool_result.is_error` 与退出码来自 provider 进程写出的字节，**永不从模型文本中提取**。
3. provider 原始 stdout 逐行落 `raw.jsonl`，作为不可伪造的证据原材料。
4. parser 签名支持「一行产出多个事件」。
5. 不新建数据通路——复用既有 `execution_ledger_events` + WS + `GET /projects/{id}/execution-trace`。

**非目标**

- 不做 artifact 采集与对象存储上传（证据地基 spec 负责）。
- 不做 acceptance criterion、verification 判据、人类待办。
- 不改 `issue_comments` 式的 agent 间协作模型——SuperTeam 现有的结构化对象协作已符合 `CLAUDE.md`，无需引入聊天。
- 不为 codex / opencode adapter 补齐 tool 事件（本期只做 claude；其余 adapter 签名跟随改造但解析逻辑保持现状）。

## 3. 架构决策

### 3.1 原始流是证据，解析后的流是 UI feed

两者不可互相替代：

- 解析必然丢信息——未知事件类型、未来的 provider 协议扩展、模型输出的原始 token 计数。
- UI 不应直接消费数十 MB 的原始 JSONL。

因此**同时**产出两条流：

| 文件 | 内容 | 消费者 |
|---|---|---|
| `{log_dir}/{run_id}/raw.jsonl` | provider stdout **每一行原样** append，不经 parser | 证据地基 spec 上传对象存储；人类回放 |
| `{log_dir}/{run_id}/events.jsonl` | 解析后的 `ProviderEvent` 流（现状，新增 tool 事件） | 本地调试 |
| `execution_ledger_events` | 解析后的事件，经 WS 上报 | Web 实时与回看 |

### 3.2 tool 事件走既有通路，不新建管道

`execution_ledger_events` 已有 `project_task_attempt_id`、`provider_type`、`provider_session_id`、`occurred_at`、`metadata`，读端 `GET /projects/{projectId}/execution-trace`（`api/server.go:303`）已存在。

`executor.rs:1518`、`1527` 已在消费 `ToolStarted` / `ToolCompleted`。扩字段后补齐透传即可。**本 spec 不新增任何表、任何端点。**

### 3.3 退出码的唯一合法来源

`is_error` 取自 stream-json 的 `tool_result.is_error` 字段，由 claude-code 进程写出。

模型在 `text` 里写的「退出码 0」是自述，**在任何情况下都不得被解析为 `is_error`**。这条是本 spec 存在的理由，不是实现细节。

## 4. 组件设计

### 4.1 parser 签名改为返回数组

```rust
// providers/mod.rs
pub type ProviderParser = fn(&str) -> anyhow::Result<Vec<ProviderEvent>>;
```

`stream_child_events`（`providers/mod.rs:~120`）改为把返回的每个事件依次投递。空 `Vec` 取代原先的 `Ok(None)`。

三个 adapter（`claude.rs`、`codex.rs`、`opencode.rs`）签名同步改造。本期仅 `claude.rs` 补全 tool 解析，其余包裹现有单事件逻辑为 `vec![]` / `vec![e]`。

### 4.2 `ProviderEvent` 扩字段

`events.rs` 现有变体不足以承载证据：

```rust
ToolStarted {
    tool_id: String,
    name: String,
    input: serde_json::Value,      // 新增
},
ToolCompleted {
    tool_id: String,
    is_error: bool,                // 新增
    output_excerpt: String,        // 新增，截断至 4KB
},
```

`output_excerpt` 只入 ledger 与 UI；完整输出留在 `raw.jsonl`。

### 4.3 `parse_claude_event` 补全

| stream-json 输入 | 产出（可多条） |
|---|---|
| `"system"` | `[SessionStarted]` |
| `"assistant"` | 遍历 **全部** content block：`text` → `TextDelta`；`tool_use` → `ToolStarted{ tool_id: block.id, name: block.name, input: block.input }` |
| `"user"` | 遍历 content block：`tool_result` → `ToolCompleted{ tool_id: block.tool_use_id, is_error: block.is_error, output_excerpt: truncate(block.content) }` |
| `"result"` | `[TurnCompleted]` |
| 其他 | `[]`（原始行仍已写入 `raw.jsonl`） |

`tool_result.content` 在 stream-json 中可以是字符串或 block 数组；两种形态都需归一为文本。`is_error` 缺省时视为 `false`。

### 4.4 原始流落盘

`runs.rs` 中，在现有 `append_event` 之外新增 `append_raw_line(run_id, line)`，写入 `{log_dir}/{run_id}/raw.jsonl`。

写入点在 `stream_child_events` 的 `BufReader::lines()` 循环里，**在调用 parser 之前**——parser 失败或返回空也不影响原始行落盘。

stderr 同样逐行落 `raw.jsonl`，以 `{"__stream":"stderr","line":"..."}` 包裹，与 stdout 行区分。

### 4.5 脱敏

`raw.jsonl` 会包含 tool result 里回显的环境变量、token、密钥。脱敏在**写入时**逐行执行：

- 已知敏感 env 变量名的值（取 provider 进程环境快照的键名，替换其值）
- 常见凭证形态：`sk-[A-Za-z0-9]{20,}`、`ghp_*`、AWS `AKIA`/`ASIA` 前缀、`Bearer <token>`、JWT 三段式
- 替换为 `[REDACTED:{reason}]`

`output_excerpt` 走同一套脱敏函数，保证 ledger 与 raw 一致。

paperclip 另有 home path 脱敏（`log-redaction.ts`，`/Users/xxx` → `/Users/x**`）。本期不做——SuperTeam 的 runtime 工作目录路径本身已是审计信息，且暂无跨租户共享 transcript 的场景。若未来 transcript 对客户侧可见，再补。

### 4.6 Web 显示

不新增数据源。`GET /projects/{projectId}/execution-trace` 返回的事件按 `project_task_attempt_id` 分组，前端时间线渲染三类节点：

| 事件 | 渲染 |
|---|---|
| `text_delta` | 文本气泡（模型旁白，视觉上弱化——它是自述） |
| `tool_use` | 工具调用：名字 + 参数摘要，可展开 |
| `tool_result` | 工具结果：`is_error` 红/绿标记 + `output_excerpt`，可展开 |

前端改动前必须阅读 `DESIGN.md`。完整回放（拉取 `raw.jsonl`）依赖证据地基 spec 的 `GET /artifacts/{id}/content`，本期不实现，时间线上给出「完整日志将在证据地基落地后可下载」的占位说明。

## 5. 错误处理

| 场景 | 行为 |
|---|---|
| parser 对某行抛错 | 记 warn，该行不产事件；`raw.jsonl` 已落盘，不丢证据 |
| `tool_result.content` 是非预期结构 | 归一为 `serde_json::to_string`，不丢事件 |
| `raw.jsonl` 写入失败（磁盘满） | 记 error 并**终止本次 run**——证据不可写时不应继续执行 |
| `output_excerpt` 超 4KB | 截断，尾部加 `…[truncated]` |
| 脱敏正则命中 | 替换，不阻断 |

## 6. 测试策略

**单元（Rust）**

- `parse_claude_event`：一条含 1 个 text + 2 个 tool_use 的 `assistant` 消息 → 产出 3 个事件（正是现有签名做不到的用例）。
- `parse_claude_event`：`type:"user"` 含 `tool_result{is_error:true}` → `ToolCompleted{is_error:true}`。
- `tool_result.content` 为字符串 / 为 block 数组，两种形态均归一。
- 脱敏：含 `sk-` 假 token 的行写入后被替换。
- `raw.jsonl`：parser 返回错误时该行仍已落盘。

**真实 E2E（完成的必要条件）**

`scripts/dev-services.sh start` 起全套服务，派发一个让 claude-code **真实执行 shell 命令**的项目任务（例如 `git status`，以及一个必定失败的命令如 `false`），然后：

1. 查库：`select input_summary, count(*) from execution_ledger_events where event_type='provider.event' group by 1` —— 出现 `tool_use` 与 `tool_result`。
2. 失败命令对应的 `tool_result` 事件 `metadata->>'is_error'` 为 `true`；成功命令为 `false`。**该值不得与模型文本中的自述相关联。**
3. 节点上 `{log_dir}/{run_id}/raw.jsonl` 存在，逐行可 JSON 解析，且含 `"type":"user"` 的 tool_result 行。
4. 浏览器打开项目执行追踪面板，能看到工具调用与工具结果节点，失败节点标红。
5. 在 tool result 中回显一个 `sk-` 前缀假 token，确认 `raw.jsonl` 与 ledger `output_excerpt` 中均已 `[REDACTED:*]`。

第 2 条是本 spec 的核心判据：**平台第一次见到真正的退出码。**

## 7. 后续

- **证据地基**（`2026-07-09-evidence-grounding-artifact-collection-design.md`）：把 `raw.jsonl` 内容寻址上传对象存储，接上 evidence 读模型，使证据可取回。
- **意图与验收判据**（`2026-06-30-intent-acceptance-criteria-design.md`）：`automated` 类 verification 的证据指针指向 `raw.jsonl` 中某条 `tool_result` 事件——本 spec 是其前提。
- **借鉴 paperclip 的 `sourceTrust`**（`shared/src/trust-policy.ts`）：`disposition: "quarantined" | "promoted"`，agent 原始输出默认隔离、须显式提升。比「打 `unverified` 标签」更硬。评估是否引入，属验收判据 spec 范围。
