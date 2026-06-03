# Runtime Command Execution Layer 设计

日期：2026-06-04

## 1. 背景

SuperTeam 已经完成 Runtime 接入、短期 Runtime Session、Runtime WebSocket 通道、数字员工执行实例和 Provider Session 的基础模型。当前 `apps/runtime-agent` 里 `ensure_instance` 已能创建执行实例目录，但 `start_session`、`resume_session`、`send_input`、`stop_session` 仍只是被反序列化后忽略，没有真正驱动 Provider 执行。

本设计目标是补齐 Runtime Agent 侧命令执行层，让 Control Plane 通过 Runtime WebSocket 下发的命令能够在客户侧或开发者机器上的 Runtime Agent 中变成真实 Provider turn。

## 2. 范围

本次只做 Runtime Agent 侧。

包括：

- 强类型解析 `start_session`、`resume_session`、`send_input`、`stop_session` payload。
- 校验命令 payload 必须携带 `command_id`、`digital_employee_id`、`execution_instance_id`、`provider_type`、`session_policy`、`prompt`、`input`、`context_refs`、`artifact_refs` 字段；输入型命令要求 `prompt` 或 `input` 至少一个非空。
- 将前三类输入命令转成现有 `ProviderAdapter` 执行。
- 将 `stop_session` 转成本地 active provider run cancel。
- 记录本地 run、provider session、execution instance 和 command 的关联。
- 增加 Runtime Agent 单元测试和 WebSocket handler 回归测试。

不包括：

- 不新增 Control Plane 下发命令的业务 API。
- 不新增 Control Plane provider session event 回传接口。
- 不实现租户、团队、权限、上下文策略或审批判断。
- 不实现长期 PTY 或常驻 stdin session actor。
- 不修改 Web 控制台页面。

## 3. 参考项目经验

### 3.1 Paperclip

参考仓库：`/Users/wangpei/src/github/agentic/paperclip`。

可吸收经验：

- 执行目标、运行、日志、spawn 元数据、timeout、结果和工件要显式建模。
- 命令执行层应把本地执行结果转为可审计证据，而不是只依赖进程退出。
- adapter/runtime 边界应保留 provider-neutral contract，业务身份和审批不进入本地执行器。

不吸收内容：

- 不引入 Paperclip 的 company、board、agent approval 或 issue checkout 模型。
- 不把 SuperTeam Runtime Agent 改成 Paperclip 式 agent heartbeat runner。

### 3.2 desktop-cc-gui

参考仓库：`/Users/wangpei/src/github/Tools/desktop-cc-gui`。

可吸收经验：

- Claude/OpenCode 会话要通过本地 session/turn/active process 映射管理。
- provider raw event 应先进入 adapter 转成稳定事件，再进入上层状态。
- `stop` 应按 turn/session 定位到具体 child process，并区分用户主动中断和异常退出。
- session id 可能先是 `pending`，直到 Provider 输出真实 session id 后再更新映射。

不吸收内容：

- 不搬 Tauri command 边界、桌面 UI 状态模型或本地前端 reducer。
- 不把 Runtime Agent 变成 desktop-cc-gui 的多引擎聊天壳。

## 4. 当前状态

Runtime Agent 当前已有：

- `controlplane/ws.rs`：连接 Control Plane Runtime WebSocket，解析 `RuntimeCommand`。
- `instances.rs`：`ensure_instance` 可创建 `agents/<execution_instance_id>/state|sessions|runs`。
- `providers/mod.rs`：`ProviderAdapter`、`ProviderRequest`、`ProviderRun` 和 cancel handle。
- `providers/claude.rs`、`providers/opencode.rs`：短生命周期 CLI per turn provider adapter。
- `runs.rs`：本地 run snapshot、事件记录、cancel 和 `events.jsonl`。
- `server.rs`：本地 `/runs` HTTP smoke path。

缺口：

- `start_session`、`resume_session`、`send_input`、`stop_session` 未执行。
- Runtime command payload 没有强类型 contract。
- run snapshot 缺少 command、digital employee、execution instance、context refs 和 artifact refs 关联。
- WebSocket handler 与执行逻辑耦合，后续继续加命令会变得分散。

## 5. 推荐方案

新增 `RuntimeCommandExecutor`，作为 Runtime WebSocket 与 Provider Adapter 之间的唯一命令处理入口。

结构：

```text
Control Plane Runtime WebSocket
  -> controlplane/ws.rs
  -> RuntimeCommandExecutor
  -> RuntimeCommandPayload parser
  -> ExecutionInstance workspace
  -> ProviderAdapter
  -> RuntimeRunStore
```

`controlplane/ws.rs` 只负责连接、反序列化和投递。`RuntimeCommandExecutor` 负责本地执行语义、run/session 映射和取消。

选择原因：

- 复用当前 `ProviderAdapter` 和 `RuntimeRunStore`，避免重写执行器。
- 让 Runtime command path 和本地 `/runs` smoke path 可以共享底层执行能力。
- 先支持短生命周期 CLI per turn，后续可在 provider adapter 下方升级为常驻 session actor。

## 6. 命令 Payload

Runtime command 外层保持当前结构：

```json
{
  "id": "cmd-001",
  "type": "start_session",
  "payload": {}
}
```

payload 增加强类型结构：

```json
{
  "command_id": "cmd-001",
  "digital_employee_id": "11111111-1111-4111-8111-111111111111",
  "execution_instance_id": "22222222-2222-4222-8222-222222222222",
  "provider_type": "claude-code",
  "session_policy": {
    "mode": "new",
    "provider_session_id": null,
    "recoverable": true
  },
  "prompt": "完成这项任务",
  "input": null,
  "context_refs": [],
  "artifact_refs": [],
  "model": null,
  "metadata": {}
}
```

规则：

- `command_id` 必填，且必须等于外层 `RuntimeCommand.id`。
- `digital_employee_id` 必填，只作为关联 ID，不用于 Runtime 本地权限判断。
- `execution_instance_id` 必填，必须是 UUID 形状，用于定位 `agents/<execution_instance_id>`。
- `provider_type` 必填，目前支持 `claude-code` 和 `opencode`。
- `session_policy` 必填。
- `prompt` 和 `input` 字段必须存在；`start_session`、`resume_session`、`send_input` 要求至少一个非空，Runtime 会将有效文本合并为 provider prompt。
- `stop_session` 可以让 `prompt` 和 `input` 都为空，因为停止命令不产生新的 provider 输入。
- `context_refs` 必填数组，可以为空；Runtime 不解析内容。
- `artifact_refs` 必填数组，可以为空；Runtime 不解析内容。
- `metadata` 可选，只作为本地 run metadata 保存。

`session_policy.mode` 取值：

```text
new
resume
reuse_latest
ephemeral
```

本阶段只机械处理：

- `new`：创建新 provider turn，`continue_session=false`。
- `resume`：要求 `provider_session_id` 非空，`continue_session=true`。
- `reuse_latest`：从本地 registry 找该 `execution_instance_id + provider_type` 最新 provider session，找不到则失败。
- `ephemeral`：创建临时 provider turn，`continue_session=false`，但不把 provider session 标记为可恢复。

## 7. 四类命令语义

### 7.1 start_session

行为：

- 校验 payload。
- 调用 `ensure_instance` 创建或确认执行实例目录。
- 使用 `agent_home_dir` 作为工作目录基准。
- 创建本地 run snapshot。
- 调用 Provider Adapter 执行 prompt。
- Provider 输出真实 `SessionStarted` 后更新 provider session 映射。

ProviderRequest 映射：

```text
prompt = payload.prompt 或 payload.input
workspace_path = agents/<execution_instance_id>
session_id = session_policy.provider_session_id
continue_session = false
model = payload.model
```

### 7.2 resume_session

行为：

- 校验 payload。
- 要求 `session_policy.provider_session_id` 非空。
- 确认执行实例目录存在。
- 创建新 run，但关联同一个 provider session。
- 调用 Provider Adapter 追加一轮。

ProviderRequest 映射：

```text
session_id = session_policy.provider_session_id
continue_session = true
```

### 7.3 send_input

行为：

- 语义等同于“向已有 provider session 追加一轮输入”。
- 优先使用 `session_policy.provider_session_id`。
- 如果缺失且 `session_policy.mode = reuse_latest`，从本地 registry 找最新 session。
- 找不到 provider session 时失败。

本阶段不把 `send_input` 实现为写入长期 stdin 管道，因为当前 Claude/OpenCode adapter 都是短生命周期 CLI per turn。

### 7.4 stop_session

行为：

- 校验 `command_id`、`digital_employee_id`、`execution_instance_id`、`provider_type`、`session_policy`、`prompt`、`input`、`context_refs` 和 `artifact_refs` 字段存在。
- 按优先级查找 active run：
  - `command_id` 对应 run。
  - `session_policy.provider_session_id` 对应 active run。
  - `execution_instance_id + provider_type` 对应 active run。
- 调用 `RuntimeRunStore.cancel_run`，由 provider run handle kill child process。
- 如果找不到 active run，记录 structured error，不做业务补偿。

## 8. 本地状态

新增 Runtime command registry，存放在 Runtime Agent 内存中。

```text
command_id -> run_id
provider_session_id -> latest_run_id
provider_session_id -> active_run_ids
execution_instance_id + provider_type -> latest_provider_session_id
execution_instance_id + provider_type -> active_run_ids
```

用途：

- 支持 `reuse_latest`。
- 支持 `stop_session` 定位 active process。
- 支持本地 `/runs/{id}` 调试时看到 command 关联。

生命周期：

- Runtime Agent 重启后 registry 丢失。
- 重启后 `resume_session` 仍可用，因为 Control Plane payload 携带 provider session id。
- 重启后 `reuse_latest` 如果没有本地记录则失败，由 Control Plane 改发明确 `provider_session_id`。

## 9. Run Snapshot 扩展

`RunSpec` 和 `RunSnapshot` 增加以下字段：

```text
command_id
digital_employee_id
execution_instance_id
provider_type
session_policy
context_refs
artifact_refs
metadata
```

说明：

- `provider_kind` 可继续保留给本地 HTTP `/runs` 兼容，但 command path 使用 `provider_type`。
- `provider_session_id` 仍由 Provider `SessionStarted` 事件更新。
- `context_refs` 和 `artifact_refs` 只作为关联保存，不读取、不拉取、不授权。

## 10. Provider 选择

Runtime Agent 本地只判断 provider 是否已启用：

```text
claude-code -> ClaudeProvider
opencode -> OpenCodeProvider
```

不在 Runtime 中做封闭业务枚举。后续新增 Codex、PI 或客户自定义 provider 时，应扩展 provider registry，不把业务判断散落到命令 handler。

## 11. 事件策略

Provider raw output 继续由 Adapter 归一化为 `ProviderEvent`：

```text
session_started
turn_started
text_delta
tool_started
tool_completed
turn_completed
turn_error
```

Runtime command path 在本地额外记录 command 关联：

- 每条 run event 都能追溯 `command_id`。
- 每条 run event 都能追溯 `digital_employee_id` 和 `execution_instance_id`。
- Provider `SessionStarted` 更新 `provider_session_id`。
- `stop_session` 产生 cancelled 状态。

本阶段不向 Control Plane 推送 provider session event。后续扩展时，使用同一批本地 run event 作为事件回传源。

## 12. 错误处理

机械校验失败：

- 缺必填字段。
- UUID 形状非法。
- `command_id` 与外层 command id 不一致。
- provider 未启用或不支持。
- prompt/input 为空。
- resume/send_input 缺 provider session id 且无法 `reuse_latest`。

处理方式：

- 记录本地 command failed。
- WebSocket loop 继续运行。
- 不重试。

执行失败：

- spawn 失败。
- provider 非零退出。
- stdout/stderr 读取失败。
- provider stream 解析失败。

处理方式：

- run 状态变为 failed。
- 保存错误摘要。
- WebSocket loop 继续运行。

停止失败：

- active run 不存在：记录 not_found/cancel_not_found。
- kill child process 失败：记录 failed，并保留错误摘要。

## 13. 测试计划

Runtime Agent 测试：

- `RuntimeCommandPayload` 解析成功。
- 缺 `command_id` 失败。
- 外层 id 和 payload `command_id` 不一致失败。
- 缺 `digital_employee_id`、`execution_instance_id`、`provider_type`、`session_policy`、`prompt`、`input`、`context_refs`、`artifact_refs` 字段失败。
- `start_session`、`resume_session`、`send_input` 的 `prompt/input` 同时为空失败。
- `start_session` 映射为 `continue_session=false`。
- `resume_session` 映射为 `continue_session=true` 和指定 `session_id`。
- `send_input` 使用 `reuse_latest` 找本地 provider session。
- `stop_session` 可以取消 active run。
- unsupported command 和坏 JSON 不会中断 WebSocket loop。

回归测试：

- 保留现有 `/health`、`/providers`、`POST /runs` smoke path。
- 保留现有 Claude/OpenCode command build tests。
- 保留现有 provider event parse tests。

## 14. 验收标准

- Runtime WebSocket 收到 `start_session` 后会真实启动 provider run。
- Runtime WebSocket 收到 `resume_session` 后会用指定 provider session 继续一轮。
- Runtime WebSocket 收到 `send_input` 后会向已有 provider session 追加一轮。
- Runtime WebSocket 收到 `stop_session` 后会取消匹配 active provider run。
- 命令 payload 必填字段全部被测试覆盖。
- Runtime Agent 不判断租户、团队、审批或业务权限。
- `cargo test --manifest-path apps/runtime-agent/Cargo.toml` 通过。

## 15. 后续扩展

下一阶段可以做：

- Runtime command event 回传到 Control Plane `provider_session_events`。
- Control Plane 数字员工执行 API 下发 runtime command。
- 更完整的 provider registry。
- 对支持长期交互的 Provider 增加常驻 session actor。
- `collect_artifact` 命令。

这些扩展不改变本次 payload 主结构。
