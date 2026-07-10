# Provider Transcript 与 Tool 事件捕获

- 状态：待实现（设计决策已对齐；§3.4/§3.5 安全评审已完成，2026-07-09）
- 日期：2026-07-09
- 范围：让数字员工的**执行过程**（工具调用与工具结果）离开执行机，进入控制平面与 Web；并把 provider 原始输出流上传对象存储，作为证据链的原材料。
- 依赖方：`docs/superpowers/specs/2026-07-09-evidence-grounding-artifact-collection-design.md`（证据地基）依赖本 spec 产出的 raw 对象。本 spec 必须先行落地。

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

原始 stdout 行由 `providers/mod.rs:127` 的 `BufReader::lines()` 逐行喂给 parser，返回 `None` 即就地丢弃，**从不落盘**。

### 1.2 parser 签名本身装不下 tool 事件

```rust
pub fn parse_claude_event(value: &str) -> anyhow::Result<Option<ProviderEvent>>
```

返回**至多一个**事件。而一条 `assistant` 消息的 `content` 数组里可以同时含 1 个 `text` block 和 N 个 `tool_use` block。

即使补上 `"user"` 分支，这个签名也物理上放不下「一行产出多个事件」。**这不是漏写了一个 match 分支，而是签名层面的结构性缺陷。**

### 1.3 下游管道全通，源头没接

- `events.rs:24-30` 早已定义 `ProviderEvent::ToolStarted { tool_id, name }` 与 `ToolCompleted { tool_id }`。
- `commands/executor.rs:1518`、`1527` 早已在 match 消费这两个变体，产出 `event_type` 为 `tool_started` / `tool_completed`。
- **没有任何 provider adapter 生产过它们。**

### 1.4 库中的实证

控制平面 `execution_ledger_events` 共 173 行，其中 `provider.event` 97 行，只有四种：

```
text_delta      43
run_completed   18
turn_completed  18
session_started 18
```

**`tool_started`、`tool_completed` 零条。**

`{log_dir}/{run_id}/events.jsonl`（`runs.rs:278` `append_event`）保存的是**解析后**的 `ProviderEvent` 流，因此同样不含 tool 事件。**原始流不存在于任何一层。**

后果可在库中直接看到——某条 `run_completed` 的 `output_summary` 是模型自己写的：

> 「证据：命令退出码 `0`；stdout 为 `final-sanity-20260709014431`。」

**那个 `0` 是数字员工声称的。平台从未见过真正的退出码。**

### 1.5 同类系统的解法（paperclip）

`/Users/tinker/src/github/agentic/paperclip` 是同类的多 agent 编排平台。三点直接对照：

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

**（b）解析后的事件落库，原始日志不落库**

`heartbeat_run_events`（`packages/db/src/schema/heartbeat_run_events.ts`）：

```
(company_id, run_id, agent_id, seq, event_type, stream, level, color, message, payload jsonb)
index (run_id, seq)
```

append-only、按 `seq` 有序，UI 直接读库。这张表在 `doc/spec/agent-runs.md:484` 上有一句关键限定：**"Append-only per-run lightweight event timeline (no full raw log chunks)."** 原始日志不进这张表。

**（c）原始日志走存储抽象，指针落在 run 行上**

`doc/spec/agent-runs.md:204` 定义 `RunLogStore`：

```ts
interface RunLogStore {
  begin(input: { companyId; agentId; runId }): Promise<RunLogHandle>;
  append(handle, event: { stream: "stdout"|"stderr"|"system"; chunk: string; ts: string }): Promise<void>;
  finalize(handle, summary: { bytes: number; sha256?: string; compressed: boolean }): Promise<void>;
  read(handle, opts?: { offset?; limitBytes? }): Promise<{ content: string; nextOffset? }>;
  delete?(handle): Promise<void>;
}
```

句柄 `RunLogHandle { store, logRef }`，`logRef` 明确要求「opaque and provider-neutral at API boundaries」。三个后端：`local_file`（dev 默认）、`object_store`（云上默认）、`postgres`（带大小上限的兜底）。

指针落在 run 行上（`packages/db/src/schema/heartbeat_runs.ts:26-33`）：

```
log_store, log_ref, log_bytes, log_sha256, log_compressed, stdout_excerpt, stderr_excerpt
```

**因此「解析后事件进库、原始流进对象存储、指针挂 run 行」是 paperclip 的原设计，本 spec 直接对齐。**

**（d）一处抄不来的差异：paperclip 少一跳网络**

paperclip 的 run 是 server 进程直接 spawn 的——`heartbeat_runs` 上有 `process_pid`、`process_group_id`、`last_output_seq`。所以它的 `RunLogStore` 天然在 server 侧，天然握有对象存储凭证，**不存在跨机上传问题**。

SuperTeam 的 raw 产生在 Runtime Agent 上。**这不是换一个存储后端，是新增一条 paperclip 没有的跨机上传腿。** 该腿的凭证与保密性决策见 §3.4 威胁模型、§3.5 凭证与保密性。

## 2. 目标与非目标

**目标**

1. `tool_use` / `tool_result` 事件流到控制平面并可在 Web 上按 attempt 回看执行过程。
2. `tool_result.is_error` 与退出码来自 provider 进程写出的字节，**永不从模型文本中提取**。
3. provider 原始 stdout/stderr 逐行分段上传对象存储，作为**模型无法伪造**的证据原材料（限定见 §3.4 威胁模型）。
4. parser 签名支持「一行产出多个事件」。
5. 事件通路复用既有 `execution_ledger_events` + WS + `GET /projects/{id}/execution-trace`；raw 指针复用既有 `complete_project_task_attempt` 回写。

**非目标**

- 不做 artifact 采集与证据读模型（证据地基 spec 负责）。
- 不做 acceptance criterion、verification 判据、人类待办。
- 不改 `issue_comments` 式的 agent 间协作模型——SuperTeam 现有的结构化对象协作已符合 `CLAUDE.md`，无需引入聊天。
- 不为 codex / opencode adapter 补齐 tool 事件（本期只做 claude；其余 adapter 签名跟随改造但解析逻辑保持现状）。
- 不实现 `RunLogStore` 的 `postgres` 后端。

**新增表：无。新增端点：无。** 本 spec 新增一次迁移（`project_task_attempts` 加列）与一处契约字段扩展。

## 3. 架构决策

### 3.1 原始流是证据，解析后的流是 UI feed

两者不可互相替代：

- 解析必然丢信息——未知事件类型、未来的 provider 协议扩展、模型输出的原始 token 计数。
- UI 不应直接消费数十 MB 的原始 JSONL。

因此**同时**产出两条流：

| 目的地 | 内容 | 脱敏 | 消费者 |
|---|---|---|---|
| 对象存储 `runs/{tenant_id}/{attempt_id}/raw.part-NNNN.jsonl` | provider stdout/stderr **每一行原样**，不经 parser | **否**（§3.5） | 证据地基 spec；人类回放 |
| `{log_dir}/{run_id}/raw.jsonl`（本地分段缓冲） | 同上，上传成功后即可回收 | 否 | 崩溃恢复；本地调试 |
| `{log_dir}/{run_id}/events.jsonl` | 解析后的 `ProviderEvent` 流（现状，新增 tool 事件） | 是 | 本地调试 |
| `execution_ledger_events` | 解析后的事件，经 WS 上报 | **是** | Web 实时与回看 |

本地文件是**分段上传的缓冲**，不是长期存储。

### 3.2 tool 事件走既有通路，不新建管道

`execution_ledger_events` 已有 `project_task_attempt_id`、`provider_type`、`provider_session_id`、`occurred_at`、`metadata`，读端 `GET /projects/{projectId}/execution-trace`（`api/server.go:303`）已存在。

`executor.rs:1518`、`1527` 已在消费 `ToolStarted` / `ToolCompleted`，产出 `event_type` 为 `tool_started` / `tool_completed`。扩字段后补齐透传即可。

**但读端会丢字段。** `queries/execution_ledger.sql` 里 provider 事件的 `metadata` 原本是硬编码的 `jsonb_build_object('command_id', …, 'sequence_number', …, 'raw_event_ref', …, 'log_ref', …)`，provider `payload` 只有 `->>'summary'` 与 `->>'text'` 两个键被取进 `output_summary`。`is_error`、`tool_id`、`*_excerpt` 会在此静默丢失。

修正为 `COALESCE(payload, '{}'::jsonb) || jsonb_build_object(…)`：payload 并入 metadata，ledger 自有键后置以防被 payload 遮蔽。改后 `metadata->>'is_error'` 才成立。

### 3.3 退出码的唯一合法来源

`is_error` 取自 stream-json 的 `tool_result.is_error` 字段，由 claude-code 进程写出。

模型在 `text` 里写的「退出码 0」是自述，**在任何情况下都不得被解析为 `is_error`**。这条是本 spec 存在的理由，不是实现细节。

### 3.4 威胁模型

本 spec 的证据完整性主张有一个明确且有限的对手：

> **对手是数字员工的模型输出。不是被攻破的 Runtime Agent 进程。**

模型可以在 `text` 里写「退出码 0」，但它写不了 `tool_result.is_error` 那个字节——那是 claude-code 进程写的；它也覆盖不了已上传的 raw 对象。**针对这个对手，本 spec 的方案是成立的。**

若对手是被攻破的 Runtime 进程，则没有任何方案成立：raw 的字节本来就由它产生，`sha256` 也由它计算。这是执行架构的固有边界，不是本 spec 能解决的问题，也不应假装解决。

这条界线决定了下面所有取舍的评价标准。§3.5 的残余风险属于**保密性**，不属于本节的完整性主张。

### 3.5 raw 的凭证与保密性（已决策，含残余风险）

**决策（2026-07-09 确认）：**

1. Runtime Agent 直接持有对象存储凭证，**复用 `apps/control-plane/config/config.yaml` 中 `objectStore` 段的那一组 `accessKeyId` / `secretAccessKey`**。
2. 该凭证是 bucket-wide 的，**不做前缀限定**，所有节点共用同一份。
3. raw 以明文原样上传，不脱敏。

采纳理由：

- 字节级原样是最强的证据形态。脱敏正则一旦误杀，证据永久损坏且不可恢复。
- 数十 MB 的 raw 不穿控制平面，避免其成为带宽与内存瓶颈。
- 复用现有凭证，不引入子账号、STS、presign 签发端点，实现路径最短。

**前置条件（已确认 2026-07-09）：Runtime Agent 只部署在内部服务器环境，无客户侧执行机、无客户托管节点。**

`CLAUDE.md` 描述 Runtime 可部署在「服务器节点、开发者机器或客户侧执行机」。本决策**收窄**了这个范围：只要凭证下发方案在用，客户侧执行机就不是受支持的部署形态。

#### 3.5.1 无租户隔离（已知并接受）

`runtime_nodes` 带 `tenant_id`（`001_initial.sql`），`runtime_node_scopes` 亦收在租户与团队内——**节点在业务上归属单一租户**。因此按 `runs/{tenant_id}/*` 前缀限定凭证在结构上是可行的。

**本期不做。** 所有节点共用一份 bucket-wide 凭证，前缀隔离**不存在**——不是「靠约定维持」，是根本没有该机制。任一执行机可对 bucket 内任意 key 执行读、写、覆盖、删除。

因此原计划的 403 前缀隔离验收用例被移除：它在本方案下按定义必然失败，保留它只会制造「有防护」的错觉。

#### 3.5.2 残余风险：全 bucket、全租户

信任边界从「控制平面」扩大到「所有执行机」，范围是**整个 bucket**，不限于 raw：

1. **raw 明文泄露。** raw 含 §4.5 列举的凭证形态（`sk-*`、`ghp_*`、AWS `AKIA`/`ASIA`、`Bearer <token>`、JWT），长期驻留对象存储且不脱敏。**ledger 脱敏而 raw 不脱敏，任何取得 bucket 读权限的路径都等价于绕过脱敏。**
2. **跨租户。** 单台执行机被攻破 = **全部租户**的 raw 泄露，不是「该租户的」。
3. **超出 raw 的爆炸半径。** 同一个 bucket 还存着 skill 包——`skill/service.go:166` 写 `skills/{tenant_id}/{slug}/{checksum}.zip`。持有该凭证的执行机可以读、覆盖、删除**任意租户的 skill 包**。覆盖 skill 包意味着可以向其他租户的数字员工投毒。

第 3 条是本决策相对于「只泄露 raw」的额外代价，落地前应让相关方知情。

#### 3.5.3 加固

**仍然必须实施（与凭证范围无关）：**

- 控制平面在**首次读取** raw 时自行重算 `sha256` 并与 `log_sha256` 比对，不无条件信任 runtime 上报值（§4.6）。在 bucket-wide 凭证下这条更重要——它是**唯一**能发现分段被覆盖的机制。

**待验证后实施：**

- `runs/` 前缀开 bucket versioning + Object Lock（WORM），使已上传分段不可覆盖。**火山引擎 TOS 对 Object Lock 与版本控制的支持情况本地无法确认，落地前须核实。** 若支持，它是 §3.5.2 第 3 条之外唯一能限制被攻破节点破坏半径的控制；若不支持，则 raw 分段可被任意执行机静默覆盖，只能靠上一条的 sha256 比对事后发现。

**基线加固（与本决策无关，不要记为它的缓解措施）：**

- bucket 开启 SSE-KMS。它防的是取得底层存储的人，**防不住持有合法凭证的调用方**——而后者正是本决策新造出来的角色。把它列为对 §3.5 的缓解是误导。

**保留期：** 绑 attempt 与验收生命周期（例如「验收结论达成后 N 天」），**不设无差别 TTL**。理由：§7 的验收判据 spec 要把 `automated` verification 的证据指针指向 raw 中某条 `tool_result`；raw 过期则验收结论悬空。证据的保留期是业务决策，不是存储成本决策。

#### 3.5.4 必须重新评审的触发条件

任一成立，本决策失效：

- 引入客户侧执行机，或用开发者机器承载生产 runtime。
- raw transcript 需要跨租户共享，或需要对客户可见。
- 平台开始承载互不信任的租户（本决策隐含假设所有租户对彼此的 raw 与 skill 包泄露是可接受的）。

**最便宜的一步改进（本期未做，建议尽早）：** 把 raw 与 skills 分到不同 bucket，各自一组凭证。这不需要 STS 或前缀条件策略，只需两个 bucket，就能把 §3.5.2 第 3 条（skill 包投毒）整条消掉。

> 更彻底的替代方案（已评估，本期未采纳）：（a）控制平面按分段签发短时效 presigned PUT，执行机不持长期凭证；（b）客户端信封加密后上传密文，`sha256(明文)` 仍可验证。二者可同时保住字节原样与租户隔离，代价是 `S3ObjectStore` 需加 Presign、控制平面需加解封端点。

## 4. 组件设计

### 4.1 parser 签名改为返回数组

```rust
// providers/mod.rs
pub type ProviderParser = fn(&str) -> anyhow::Result<Vec<ProviderEvent>>;
```

`stream_child_events`（`providers/mod.rs:99`）改为把返回的每个事件依次投递。空 `Vec` 取代原先的 `Ok(None)`。

三个 adapter（`claude.rs`、`codex.rs`、`opencode.rs`）签名同步改造。本期仅 `claude.rs` 补全 tool 解析，其余包裹现有单事件逻辑为 `vec![]` / `vec![e]`。

**行为变更（修正）：** 「解析失败」与「provider 报告失败」必须分开，不能都走 `Err`。`codex.rs` 用 `anyhow::bail!` 把 `turn.failed` 变成流上的 `Err`——那是故意的失败信号。若在 `stream_child_events` 里统一吞掉 `Err`，codex 失败的 run 会静默变成功。

因此：**JSON 解析失败在 parser 内部处理**（`providers::parse_line_json` 记 warn 并返回空 `Vec`），`Err` 通道保留给 provider 报告的真实失败，仍然向下游传播并终止 run。

### 4.2 `ProviderEvent` 扩字段

`events.rs:24-30` 现有变体不足以承载证据：

```rust
ToolStarted {
    tool_id: String,
    name: String,
    input_excerpt: String,              // 新增，JSON 序列化后截断至 4KB
    input_truncated: bool,              // 新增
},
ToolCompleted {
    tool_id: String,
    is_error: bool,                     // 新增
    output_excerpt: String,             // 新增，截断至 4KB
    output_truncated: bool,             // 新增
},
```

`input_excerpt` 是 `String` 而非 `serde_json::Value`：`ProviderEvent` derive 了 `Eq`，而 `serde_json::Value` 不实现 `Eq`；且截断后的 JSON 本就不是合法 JSON。

**input 与 output 对称截断。** 理由：claude-code 的 `Write` / `Edit` 工具的 `input` 里就是整个文件内容，单次调用可达数百 KB。`execution_ledger_events` 是热表且经 WS 实时广播，行大小必须可控。

截断的完整内容留在对象存储的 raw 中。截断时 `*_truncated = true`，UI 据此提示「完整参数见原始日志」。

不按工具名分策略——那会引入对 provider 工具名的硬编码知识，违反 `CLAUDE.md`「不依赖封闭枚举」，且 claude-code 改工具名即失效。

### 4.3 `parse_claude_event` 补全

| stream-json 输入 | 产出（可多条） |
|---|---|
| `"system"` | `[SessionStarted]` |
| `"assistant"` | 遍历 **全部** content block：`text` → `TextDelta`；`tool_use` → `ToolStarted{ tool_id: block.id, name: block.name, input_excerpt: truncate(block.input) }` |
| `"user"` | 遍历 content block：`tool_result` → `ToolCompleted{ tool_id: block.tool_use_id, is_error: block.is_error, output_excerpt: truncate(block.content) }` |
| `"result"` | `[TurnCompleted]` |
| 其他 | `[]`（原始行仍已进 raw） |

`tool_result.content` 在 stream-json 中可以是字符串或 block 数组；两种形态都需归一为文本。`is_error` 缺省时视为 `false`。

### 4.4 raw 流：注入 sink、分段上传

**注入方式。** `stream_child_events` 当前签名（`providers/mod.rs:99`）只有 `provider_name / parser / child / stdout / stderr`，对「哪个 run」一无所知。新增一个参数：

```rust
pub trait RawLineSink: Send + Sync {
    fn write_line(&self, stream: RawStream, line: &str) -> anyhow::Result<()>;
}

fn stream_child_events(
    provider_name: &'static str,
    parser: ProviderParser,
    raw_sink: Arc<dyn RawLineSink>,   // 新增
    child: Child,
    stdout: ChildStdout,
    stderr: ChildStderr,
) -> ProviderRun
```

由 run 层传入绑定了 `run_id` / `attempt_id` 的实现。provider 层只依赖这个窄 trait，不依赖 `RunStore`，不依赖对象存储；测试传 no-op sink。

不选「parser 返回原始行、消费侧落盘」：一行可产出多个事件，原始行需去重穿过整条事件流管道，成本高于注入一个 sink。

**写入点。** 在 `BufReader::lines()` 循环里、**调用 parser 之前**——parser 失败或返回空都不影响原始行落盘。

**stderr 逐行 tee。** `providers/mod.rs:110` 现在是 `reader.read_to_string(&mut stderr_text)`，整段读到进程结束，仅供 `provider_exit_result`。改为 `BufReader::lines()` 逐行：每行以 `{"__stream":"stderr","ts":"...","line":"..."}` 包裹进 raw，同时累积成 `String` 供 `provider_exit_result`。**累积缓冲设上限（默认 256KB，超出则只保留尾部），防止 stderr 刷屏打爆内存。**

保留逐行是为了 stderr 与 stdout 能按时序交错，还原真实执行过程；整段读会丢掉行级时序，且进程被 kill 时可能整段拿不到。

**runtime 侧对象存储配置。** runtime-agent 需要新增 `objectStore` 配置段（endpoint / region / bucket / accessKeyId / secretAccessKey / forcePathStyle），取值与 `apps/control-plane/config/config.yaml` 的 `objectStore` 段相同（§3.5 决策 1）。

`Cargo.toml` 已有 `aws-sdk-s3 = "1.55"` 与 `aws-config = "1.5"`，但**当前无任何 `aws_config` 初始化**——`apps/runtime-agent/src/artifacts.rs` 的 `ArtifactCollector` 是死代码，全仓无一处 `ArtifactCollector::new`。本 spec 是 runtime 首次真正持有对象存储凭证。

**前缀不一致须统一。** `artifacts.rs` 那段死代码用 `runtime/{tenant_id}/{run_id}/{type}/{file}`，本 spec 用 `runs/{tenant_id}/{attempt_id}/`。证据地基 spec 会复用 `artifacts.rs`，两者落地前必须选定同一套前缀约定，不要各写各的。

**分段上传。** `RawLineSink` 的实现：逐行写本地 `{log_dir}/{run_id}/raw.jsonl`；每累积 N MB（默认 8MB）或 M 秒（默认 30s）封一个分段，计算该分段 `sha256`，上传为 `runs/{tenant_id}/{attempt_id}/raw.part-NNNN.jsonl`。run 结束时封最后一段并上传 `manifest.json`：

```json
{
  "attempt_id": "...",
  "parts": [{"key": "...part-0001.jsonl", "bytes": 8388608, "sha256": "..."}],
  "total_bytes": 12582912,
  "total_sha256": "...",
  "complete": true
}
```

选择分段而非「run 结束一次性上传」：进程被 kill、节点断电、磁盘满——正是最需要证据的场景，而一次性上传在这些场景下永久丢失全部 raw。选择分段而非 multipart API：分段是普通 PUT，每段各自内容寻址，无未完成上传需要清理。

**崩溃恢复。** 分段 key 由 `attempt_id` 完全确定，不依赖回写。run 被 kill 导致 `complete_project_task_attempt` 未执行时，已上传的分段仍可按前缀 `runs/{tenant_id}/{attempt_id}/` 枚举取回，此时无 `manifest.json`，视为 `complete: false`。

### 4.5 脱敏：只在上报路径

**raw（本地缓冲与对象存储）不脱敏**，见 §3.5。

**进 `execution_ledger_events` 与 WS 的 `input_excerpt` / `output_excerpt` 必须脱敏**，在截断之后、入 ledger 之前逐值执行：

- 已知敏感 env 变量名的值（取 provider 进程环境快照的键名，替换其值）
- 常见凭证形态：`sk-[A-Za-z0-9]{20,}`、`ghp_*`、AWS `AKIA`/`ASIA` 前缀、`Bearer <token>`、JWT 三段式
- 替换为 `[REDACTED:{reason}]`

本地 `{log_dir}` 下的文件权限设 `0600`。

paperclip 另有 home path 脱敏（`log-redaction.ts`，`/Users/xxx` → `/Users/x**`）。本期不做——SuperTeam 的 runtime 工作目录路径本身已是审计信息。

### 4.6 raw 指针落 `project_task_attempts`

新增迁移 `051_project_task_attempt_raw_log.sql`，对齐 paperclip 的 `heartbeat_runs`：

```sql
ALTER TABLE project_task_attempts
  ADD COLUMN log_store      text,
  ADD COLUMN log_ref        text,
  ADD COLUMN log_bytes      bigint,
  ADD COLUMN log_sha256     text,
  ADD COLUMN log_compressed boolean NOT NULL DEFAULT false;
```

`log_ref` 是不透明字符串（此处为 manifest 的对象 key），API 边界上不得假设文件系统语义。

**不新增端点。** 这组字段搭 `executor.rs:1451` 已有的 `complete_project_task_attempt` 回写（`controlplane/client.rs:406`），在 `contracts/control-plane/openapi.yaml` 里扩展请求体。修改契约后走生成与契约验证流程。

不选「发一条 `raw_log_stored` 事件」：取某次 attempt 的 raw 就得扫事件表取最后一条；paperclip 明确把这组字段放在 run 行而非事件表。

**`log_sha256` 是 runtime 上报值，控制平面不得无条件信任。** 产出字节的、算哈希的、写对象的是同一个主体，因此该值只是校验和，不是防篡改凭证。控制平面在**首次读取** raw 时（由证据地基 spec 的读路径触发）必须自行重算 sha256 并与 `log_sha256` 比对：

- 一致 → 正常返回。
- 不一致 → 拒绝返回该 raw，记 audit，标记该 attempt 的证据为不可信。

按 §3.4 的威胁模型，这条防的不是模型（模型覆盖不了对象），而是让分段被覆盖或截断时**能被发现**，与 §3.5.3 的 Object Lock 互为表里。

### 4.7 Web 显示

不新增数据源。`GET /projects/{projectId}/execution-trace` 返回的事件按 `project_task_attempt_id` 分组，前端时间线渲染三类节点：

| `event_type` | 渲染 |
|---|---|
| `text_delta` | 文本气泡（模型旁白，视觉上弱化——它是自述） |
| `tool_started` | 工具调用：名字 + `input_excerpt` 摘要，可展开；`input_truncated` 时提示完整参数在原始日志 |
| `tool_completed` | 工具结果：`is_error` 红/绿标记 + `output_excerpt`，可展开 |

前端改动前必须阅读 `DESIGN.md`。完整回放（取回 raw 分段）依赖证据地基 spec 的读模型，本期不实现，时间线上给出「完整日志将在证据地基落地后可下载」的占位说明。

## 5. 错误处理

| 场景 | 行为 |
|---|---|
| 某行 JSON 解析失败 | parser 内 `parse_line_json` 记 warn，返回空 `Vec`，继续下一行；raw 已落盘，不丢证据 |
| parser 返回 `Err`（provider 报告失败，如 codex `turn.failed`） | 向下游传播，终止 run。**不得与解析失败混为一谈** |
| `tool_result.content` 是非预期结构 | 归一为 `serde_json::to_string`，不丢事件 |
| 本地 raw 写入失败（磁盘满） | 记 error 并**终止本次 run**——证据不可写时不应继续执行 |
| 分段上传失败 | 指数退避重试 3 次；仍失败则记 error、保留本地分段、**不终止 run**，`manifest.complete=false` |
| `input_excerpt` / `output_excerpt` 超 4KB | 截断，尾部加 `…[truncated]`，置 `*_truncated=true` |
| stderr 累积超 256KB | 只保留尾部供 `provider_exit_result`；raw 中仍是全量逐行 |
| 脱敏正则命中（仅 ledger 路径） | 替换，不阻断 |

本地写失败终止 run、上传失败不终止 run：前者意味着证据从此刻起完全不存在；后者证据仍在本地，可事后补传。

## 6. 测试策略

**单元（Rust）**

- `parse_claude_event`：一条含 1 个 text + 2 个 tool_use 的 `assistant` 消息 → 产出 3 个事件（正是现有签名做不到的用例）。
- `parse_claude_event`：`type:"user"` 含 `tool_result{is_error:true}` → `ToolCompleted{is_error:true}`。
- `tool_result.content` 为字符串 / 为 block 数组，两种形态均归一。
- `input_excerpt`：超 4KB 的 `Write` 工具 input 被截断且 `input_truncated=true`。
- 脱敏：含 `sk-` 假 token 的 `output_excerpt` 被替换；**同一行在 raw 中保持原样**。
- raw sink：parser 返回错误时该行仍已写入；stderr 行带 `__stream:"stderr"`。
- 分段：写满阈值后封段并调用上传；上传失败重试后 `manifest.complete=false`。

**真实 E2E（完成的必要条件）**

`scripts/dev-services.sh start` 起全套服务，派发一个让 claude-code **真实执行 shell 命令**的项目任务（例如 `git status`，以及一个必定失败的命令如 `false`），然后：

1. 查库：`select input_summary, count(*) from execution_ledger_events where event_type='provider.event' group by 1` —— 出现 `tool_started` 与 `tool_completed`。
2. 失败命令对应的 `tool_completed` 事件 `metadata->>'is_error'` 为 `true`；成功命令为 `false`。**该值不得与模型文本中的自述相关联。**
3. 对象存储 `runs/{tenant_id}/{attempt_id}/` 下存在 `raw.part-0001.jsonl` 与 `manifest.json`；分段逐行可 JSON 解析，含 `"type":"user"` 的 tool_result 行；`manifest.total_sha256` 与拼接后内容的 sha256 一致。
4. `project_task_attempts` 该行的 `log_ref` / `log_sha256` / `log_bytes` 已回写且与 manifest 一致。
5. 浏览器打开项目执行追踪面板，能看到工具调用与工具结果节点，失败节点标红。
6. 在 tool result 中回显一个 `sk-` 前缀假 token，确认 ledger `output_excerpt` 已 `[REDACTED:*]`，而**对象存储中的 raw 保留原样**（本条同时是 §3.5 决策的验收）。
7. **哈希校验验收（§4.6）：** 直接改写对象存储上的某个分段，控制平面首次读取时必须检出 `sha256` 不匹配、拒绝返回、并标记该 attempt 证据不可信。

**无前缀隔离验收用例**（原第 7 条已移除）：§3.5.1 决定所有节点共用 bucket-wide 凭证，该用例按定义必然失败。

**Object Lock 验收（条件性）：** 仅在核实 TOS 支持 Object Lock 后追加——用节点凭证对已上传的 `raw.part-0001.jsonl` 再次 PUT 不同内容，必须被拒绝。若 TOS 不支持，本条不存在，此时第 7 条是唯一的完整性防线。

第 2 条是本 spec 的核心判据：**平台第一次见到真正的退出码。**

第 7 条在本方案下地位上升：由于凭证不做前缀限定、Object Lock 支持性未知，**sha256 事后比对是唯一能发现 raw 被覆盖的机制**，不得省略。

> E2E 在真实 TOS bucket（`config.yaml` 的 `objectStore`）上执行。写入前确认所用 `tenant_id` / `attempt_id` 前缀不与既有数据冲突；**不得为验证目的删除或覆盖既有对象**。

## 7. 后续

- **核实 TOS 的 Object Lock / 版本控制支持**（§3.5.3）。这是本方案下唯一能主动阻止 raw 被覆盖的机制；不支持则只剩 sha256 事后比对。
- **raw 与 skills 分 bucket**（§3.5.4）。最便宜的一步风险收敛：消掉「执行机可覆盖任意租户 skill 包」这条爆炸半径，不需要 STS 或前缀策略。
- **§3.5.4 触发条件监视**：一旦引入客户侧执行机、用开发者机器跑生产 runtime、需要跨租户共享 raw，或平台开始承载互不信任的租户，回到 §3.5.4 的替代方案（presigned 分段直传 + 信封加密）重新评审。
- **证据地基**（`2026-07-09-evidence-grounding-artifact-collection-design.md`）：接上 evidence 读模型，使 raw 分段可按 `log_ref` 取回。
- **意图与验收判据**（`2026-06-30-intent-acceptance-criteria-design.md`）：`automated` 类 verification 的证据指针指向 raw 中某条 `tool_result` 事件——本 spec 是其前提。
- **借鉴 paperclip 的 `sourceTrust`**（`shared/src/trust-policy.ts`）：`disposition: "quarantined" | "promoted"`，agent 原始输出默认隔离、须显式提升。比「打 `unverified` 标签」更硬。评估是否引入，属验收判据 spec 范围。
