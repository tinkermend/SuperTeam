# Runtime Agent Codex Provider 设计

日期：2026-06-13

## 1. 背景

Runtime Agent 当前只把 Claude Code 和 OpenCode 建模成可执行 Provider。`apps/runtime-agent` 里的配置、capability 上报、命令 payload 校验、Provider 选择、workspace home materialization、本地 HTTP smoke path 和 CLI run path 都只覆盖 `claude-code` 与 `opencode`。

这会导致 Control Plane 即使把数字员工或任务配置为 Codex，也无法从 Runtime Agent 看到 `codex` capability，更无法下发真实 Codex 执行。

本设计目标是把 Codex 作为 Runtime Agent 的一等 Provider 接入，并顺手收敛 Provider 元信息分散硬编码的问题。

## 2. 范围

包括：

- 新增 `provider_type = "codex"` 的 Runtime Agent 配置、健康探测、capability 上报和 supported provider 上报。
- 新增 Codex CLI adapter，支持新会话和真实 resume。
- 让 Runtime WebSocket session command path 支持 Codex。
- 让 legacy task executor、本地 HTTP `/runs` smoke path、CLI `run` path 同步支持 Codex。
- 增加 Codex workspace provider home `.codex`。
- 增加 Runtime Agent 测试覆盖。

不包括：

- 不新增 Control Plane provider 类型枚举或业务策略分支。
- 不修改 Web 控制台页面。
- 不实现 Codex Cloud、remote app-server、MCP server 或 desktop app 集成。
- 不把 Runtime Agent 改成动态插件系统。
- 不新增新的 Runtime 事件类型；Codex raw JSONL 仍由 adapter 映射到现有稳定事件。

## 3. 推荐架构

采用集中 provider catalog/selector，而不是在每个调用点继续添加分散 `match` 分支。

新增或等价实现一个 Runtime Agent 内部 provider catalog，集中描述：

- provider type：`claude-code`、`opencode`、`codex`
- provider kind：`claude`、`opencode`、`codex`
- config section
- binary path
- enabled 状态
- health probe kind
- workspace home kind
- adapter factory

主要调用点改为复用 catalog/selector：

- `daemon.rs`：构建 `supported_providers` 与 provider capabilities。
- `commands/payload.rs`：校验 `provider_type` 是否受支持，并映射 provider kind。
- `commands/executor.rs`：按 payload 选择 Provider adapter。
- `executor/task.rs`：legacy task executor 选择 Provider adapter。
- `server.rs`：本地 `/providers` 与 `/runs` smoke path 支持 Codex。
- `main.rs`：CLI `run --provider codex` 支持 Codex。
- `workspace_files.rs`：按 provider type 选择 provider private home。

这样本次新增 Codex 的同时减少 Provider 信息遗漏风险。后续接入 Pi 或其他 Provider 时，主要扩展 catalog 和 adapter。

## 4. 配置

`RuntimeConfig.providers` 增加：

```yaml
providers:
  codex:
    enabled: false
    binary_path: codex
    timeout: 3600
```

默认值：

- `enabled = false`
- `binary_path = "codex"`
- `timeout = 3600`

环境变量：

- `RUNTIME_AGENT_PROVIDER_CODEX_ENABLED`
- `RUNTIME_AGENT_PROVIDER_CODEX_BINARY`
- `RUNTIME_AGENT_PROVIDER_CODEX_TIMEOUT`

CLI override 增加：

- `--codex-bin <PATH>`

配置校验继续要求已建模 provider 的 binary path 非空，但不要求 Codex 必须启用。

## 5. Capability 上报

启动时 Runtime Agent 使用 catalog 构建：

- `supported_providers`：只包含已启用 provider，例如 `["claude-code", "codex"]`。
- provider capability：为每个已建模 provider 上报一条 `capability_type = "provider"` 记录。

Codex capability 字段：

- `capability_type = "provider"`
- `capability_key = "codex"`
- `provider_type = "codex"`
- `binary_path = providers.codex.binary_path`
- `provider_version = codex --version` 的 stdout 摘要，探测成功时填写
- `available = enabled && health.available`
- `status = disabled | healthy | unavailable`
- `health_status = disabled | healthy | unhealthy`
- `details.error`：探测失败时填写错误信息

即使 Codex disabled 或 binary 不可用，也上报 capability。这样 Control Plane 和 Console 能看到节点为什么不能调度 Codex。

## 6. Codex Adapter

新增 `providers/codex.rs`，实现现有 `ProviderAdapter` trait。

### 6.1 新会话

`session_policy.mode = new | ephemeral` 时构建：

```text
codex exec --json \
  --cd <workspace> \
  --ask-for-approval never \
  --sandbox danger-full-access \
  [--model <model>] \
  <prompt>
```

`Command.current_dir(<workspace>)` 同时设置为执行目录，避免相对路径行为依赖调用方当前目录。

### 6.2 续接会话

`session_policy.mode = resume` 或 `reuse_latest` 查到 session 后构建：

```text
codex exec resume <session_id> \
  --json \
  --dangerously-bypass-approvals-and-sandbox \
  [--model <model>] \
  <prompt>
```

`codex exec resume --help` 不暴露 `--cd`、`--sandbox`、`--ask-for-approval`，所以 adapter 必须使用 `Command.current_dir(<workspace>)` 固定工作目录。resume path 使用 CLI 暴露的 `--dangerously-bypass-approvals-and-sandbox`，保持“Runtime 已治理，本机 Provider 不再二次询问”的执行语义。

### 6.3 执行权限语义

Codex 默认按 Runtime 受控执行语义运行：

- 新会话：`--ask-for-approval never --sandbox danger-full-access`
- resume：`--dangerously-bypass-approvals-and-sandbox`

这不是新的业务授权入口。任务是否能调度、谁能触发、应该在哪个 Runtime 上运行，仍由 Control Plane、Runtime capability、workspace 隔离、审计和人类审批流程治理。

## 7. Session 语义

Codex 必须支持真实 resume。

- `new`：创建新 Codex session，`continue_session = false`。
- `ephemeral`：创建新 Codex session，但不标记为可恢复。
- `resume`：要求 `provider_session_id` 非空，调用 `codex exec resume <session_id>`。
- `reuse_latest`：沿用现有 registry，通过 `execution_instance_id + provider_type` 查最新 provider session，再调用 `codex exec resume <session_id>`。
- `send_input`：语义等同于向已有 Codex session 追加一轮输入，优先使用 payload 的 `provider_session_id`，缺失时只在 `reuse_latest` 下查 registry。

Codex adapter 从 stdout JSONL 事件中提取真实 session/thread id，并转成 `ProviderEvent::SessionStarted`，由现有 registry 记录。

## 8. Workspace Materialization

`ProviderHomeKind` 增加 `Codex`，对应 provider private directory：

```text
.codex
```

`provider_home_kind("codex")` 返回 `ProviderHomeKind::Codex`。

workspace file path 安全规则继续阻止任意写入 provider 私有目录，新增 `.codex` 到禁止路径首段列表。Runtime materialization 只负责确保 `.codex` 目录存在，不把 Control Plane 下发的 workspace file 写入 Codex 私有状态目录。

## 9. Codex JSONL 事件映射

Codex stdout 使用 `--json` JSONL。Adapter 不透传 raw event，而是映射到 Runtime Agent 已有稳定事件：

- 能识别 session/thread id 的事件转为：
  - `ProviderEvent::SessionStarted { session_id, session_state: None }`
- assistant/final message/text delta 类事件转为：
  - `ProviderEvent::TextDelta { text }`
- turn 完成类事件转为：
  - `ProviderEvent::TurnCompleted { summary }`
- error/failure 类事件返回 adapter error，由现有 run failure/writeback 路径记录。
- 未识别但 JSON 格式合法的事件忽略。
- JSON 解析失败返回错误，避免把损坏 stdout 当成成功执行。

解析逻辑应兼容 Codex CLI JSONL 字段命名变化，优先从常见字段中提取：

- session id：`session_id`、`sessionId`、`thread_id`、`threadId`、嵌套 `session.id`、`thread.id`
- text：`text`、`delta`、`content`、嵌套 `message.content`
- summary：`summary`、`result`、`final_message`

字段兼容是 adapter 内部容错，不改变 provider-run contract。

## 10. 错误处理

沿用现有 child process stream 错误模型：

- spawn 失败：返回 `failed to spawn codex`。
- stdout 捕获失败：返回 `failed to capture codex stdout`。
- stderr 捕获失败：返回 `failed to capture codex stderr`。
- 非零退出：返回 `codex exited with status <code>: <stderr>`。
- JSONL error event：返回 adapter error，message 来自 Codex 事件中的错误字段。

Runtime command path 中错误继续通过现有 `recorded_error`、run failure 和 command writeback 路径呈现为 provider failure，不新增旁路状态。

## 11. 测试设计

新增或扩展以下测试：

- `provider_command_test`
  - Codex new command 包含 `exec --json --cd <workspace> --ask-for-approval never --sandbox danger-full-access`。
  - Codex resume command 包含 `exec resume <session_id> --json --dangerously-bypass-approvals-and-sandbox`。
  - model 参数被正确传递。
- `provider_event_test`
  - Codex session/text/completion JSONL 映射到 `ProviderEvent`。
  - Codex error JSONL 返回错误。
- `provider_spawn_test`
  - fake Codex CLI 输出 JSONL，adapter 可流式消费。
- `runtime_command_payload_test`
  - `provider_type = "codex"` 被识别为 supported provider。
  - unsupported provider 仍失败。
- `runtime_command_executor_test`
  - Codex provider enabled 时可被 selector 选中。
  - Codex disabled 时命令失败且错误清晰。
- `daemon_test`
  - `supported_providers` 包含启用的 `codex`。
  - capability upsert body 包含 Codex provider capability。
  - Codex binary 不可用时仍上报 unavailable capability。
- `workspace_files_test`
  - `provider_home_kind("codex")` 创建 `.codex`。
  - workspace file 不能写入 `.codex/...`。
- `http_server_test`
  - `/providers` 返回 Codex health。
  - `/runs` 接受 `provider_kind = "codex"`。
- CLI 相关测试
  - `run --provider codex` 使用 Codex adapter。

验证命令：

```bash
corepack pnpm verify:runtime-agent
```

必要时先跑更窄的 Rust 测试子集，再跑完整 runtime-agent verify。

## 12. 兼容性与迁移

- 默认 `providers.codex.enabled = false`，不会改变现有 Runtime Agent 行为。
- 现有 `claude-code` 与 `opencode` provider type 保持不变。
- Control Plane 不需要新增封闭枚举；`codex` 通过现有 provider type 字符串、capability 和服务端校验路径接入。
- 已有 Runtime 节点配置不需要立刻修改。要启用 Codex 时，在本机 Runtime config 或环境变量中打开 `providers.codex.enabled` 并确认 `codex` binary 可用。

## 13. 完成标准

- Runtime Agent 能上报 Codex provider capability。
- `supported_providers` 在 Codex enabled 时包含 `codex`。
- Runtime WebSocket command path 能执行 `provider_type = "codex"` 的新会话和真实 resume。
- legacy task executor、本地 HTTP smoke path、CLI run path 都能选择 Codex。
- Workspace materialization 支持 `.codex` provider home，并保护 `.codex` 私有目录不被 workspace file 覆盖。
- `corepack pnpm verify:runtime-agent` 通过。
