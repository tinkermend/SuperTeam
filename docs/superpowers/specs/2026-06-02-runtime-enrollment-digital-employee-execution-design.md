# Runtime 接入与数字员工执行实例 Spec

> 日期：2026-06-02
> 状态：待评审
> 决策：采用 Runtime 接入 + 数字员工唯一执行实例模型，预留自动调度扩展点

## 1. 背景

SuperTeam 是企业级数字员工控制平面。当前 Runtime Agent 已能注册、心跳、claim 任务，并能探测本机 Provider 能力。但现有接入方式把 Runtime Agent 启动、正式认证、节点注册和任务执行身份混在一起：

- Runtime Agent 启动前必须已经拥有正式 runtime token。
- 如果数据库中缺少该 token，Runtime Agent 无法进入“待接入”状态。
- Runtime 节点容易被误建模为业务执行身份，并承担租户/团队范围。
- Claude Code、OpenCode 等 Provider 会话输出和平台事件边界需要更明确。

目标态应区分四类对象：

- Runtime Agent：客户虚机或服务器上的执行管理平面。
- Provider：Claude Code、OpenCode、Codex、PI 等本机执行引擎能力。
- Digital Employee：业务层数字员工身份。
- Provider Session：Provider 自身的可新建、恢复、继续交互的会话。

## 2. 目标

本 spec 目标是定义 Runtime 接入、数字员工唯一执行实例、会话交互和 Web 页面分工。

核心目标：

- Runtime Agent 可以先自发现并进入待接入状态，而不是必须预先有正式节点 token。
- Runtime Agent 长期只保存环境级 bootstrap key，不长期保存正式 runtime token。
- Runtime Agent 使用短期 runtime session token 连接 Control Plane，并自动续期。
- Runtime Agent 只作为宿主能力接入，不直接绑定租户/团队业务范围。
- 一个数字员工只绑定一个执行实例。
- 数字员工的执行实例绑定 Runtime Agent、Provider、workspace 策略和 session 策略。
- Claude Code、OpenCode 等 Provider 不直接连接 Control Plane 或 Web。
- 所有 Provider 输出都必须经过 Runtime Agent 回传 Control Plane，再由 Web 展示。
- 设计保留 runtime selector、labels、capacity 和 fallback 扩展点，但 MVP 不实现自动调度。

## 3. 非目标

本 spec 不做以下事项：

- 不实现一个数字员工绑定多个执行实例。
- 不实现跨 Runtime 自动迁移。
- 不实现复杂 fallback、实例池或容量调度。
- 不让 Provider 直接持有平台 token。
- 不让 Web 直接连接 Runtime Agent 或 Provider。
- 不自动扫描客户机器上的任意 workspace。
- 不把 Runtime Agent 配置文件作为数字员工业务身份来源。
- 不在本阶段展开人类员工完整模型，但命名和权限模型必须预留人类员工与数字员工并存。

## 4. 方案选择

### 4.1 方案 A：最小修正现有 Runtime Node 模型

保留现有 `runtime_nodes`、`auth_runtime_tokens`、`runtime_node_scopes`，只增加 pending 状态和 Web 接入按钮。

优点：

- 改动较小。
- 能快速解决 token 不存在时 Runtime 无法进入可见状态的问题。

缺点：

- 仍然把 Runtime Agent 当成业务执行身份。
- 仍然让 Runtime 节点承担租户/团队范围。
- 后续数字员工、Provider Session 和业务权限模型会继续返工。

结论：不采用。

### 4.2 方案 B：Runtime 接入 + 数字员工唯一执行实例

Runtime Agent 作为宿主执行管理平面接入。数字员工作为业务身份，每个数字员工最多绑定一个执行实例。执行实例绑定 Runtime Agent、Provider、workspace 策略和 session 策略。

优点：

- 符合客户侧 Runtime Agent 执行模型。
- 区分宿主能力、业务身份、执行实例和 Provider Session。
- 能保留当前阶段可控复杂度。
- 适合 MVP。

缺点：

- 需要重构现有 Runtime scope 语义。
- 需要新增 enrollment、runtime session、execution instance 和 provider session 数据结构。

结论：采用。

### 4.3 方案 C：完整调度平台模型

在方案 B 基础上增加自动调度、Runtime selector、多个执行实例、实例池、fallback 和跨节点迁移。

优点：

- 长期能力强。
- 支持复杂企业部署。

缺点：

- 当前阶段过重。
- 与“一个数字员工只绑定一个运行实例”的阶段性决策不匹配。

结论：暂不实现，只预留扩展字段。

## 5. 架构边界

固定通信链路：

```text
Web
  <-> Control Plane
    <-> Runtime Agent
      <-> Provider Session
```

边界规则：

- Web 只和 Control Plane 交互。
- Web 不直接连接 Runtime Agent。
- Web 不直接连接 Claude Code、OpenCode、Codex 或 PI。
- Control Plane 管业务身份、数字员工、接入审批、短期 runtime session、权限、上下文、审计、任务和事件持久化。
- Runtime Agent 是客户虚机或服务器上的执行管理平面，负责 Provider 探测、workspace 管理、进程和 session 管理、日志、事件、工件回传、槽位和健康状态。
- Provider 是 Runtime Agent 管理下的本地执行进程或会话。
- Provider 不持有平台 token。
- Provider 不直接上报平台事件。
- Provider 不直接访问 Web 或 Control Plane。

核心原则：

```text
Runtime Agent 接入的是一台执行宿主。
Digital Employee 才是业务执行身份。
Execution Instance 是数字员工和 Runtime 宿主之间的唯一绑定。
Provider Session 是底层执行会话。
```

## 6. 数据模型

### 6.1 Runtime 宿主接入

建议新增或调整以下数据结构：

```text
runtime_nodes
runtime_enrollments
runtime_sessions
runtime_capabilities
```

`runtime_nodes` 表示已知 Runtime Agent 宿主：

- `id`：数据库内部 UUID。
- `node_id`：Runtime Agent 外部稳定 ID。
- `name`：节点名称。
- `version`：Runtime Agent 版本。
- `status`：连接状态，例如 online、offline、degraded。
- `last_heartbeat_at`：最后心跳时间。
- `capacity`：槽位与负载信息。
- `labels`：预留给后续自动匹配。

`runtime_enrollments` 表示接入审批状态：

- `runtime_node_id`。
- `tenant_id` 或客户环境 ID。
- `status`：pending、approved、rejected、revoked。
- `bootstrap_key_id` 或 key 引用。
- 审批人、审批时间、拒绝原因、撤销原因。

`runtime_sessions` 表示短期 Runtime 会话：

- `runtime_node_id`。
- `session_token_hash`。
- `expires_at`。
- `last_seen_at`。
- `revoked_at`。
- 默认 TTL 为 12 小时。
- Runtime 在过期前自动续期。

`runtime_capabilities` 表示 Runtime 上报的宿主能力：

- Provider 类型。
- Provider 版本。
- binary path 是否可用。
- workspace base_dir。
- 容量。
- 标签。
- 健康状态。

### 6.2 数字员工与执行实例

建议新增或调整以下数据结构：

```text
digital_employees
digital_employee_execution_instances
```

`digital_employees` 表示业务身份：

- 名称、职责、描述。
- 租户、团队。
- 权限策略。
- 上下文策略。
- 审批策略。
- 风险等级。
- 状态：draft、ready、active、disabled、error。

`digital_employee_execution_instances` 表示数字员工唯一执行实例：

- `digital_employee_id`。
- `runtime_node_id`。
- `provider_type`。
- `agent_home_dir`。
- `workspace_policy`。
- `session_policy`。
- `status`。
- `runtime_selector`，预留自动匹配。
- `capacity_requirements`，预留容量调度。
- `fallback_policy`，预留 fallback。

约束：

```text
digital_employee_id unique
```

一个数字员工最多一个执行实例。如果企业需要多个类似执行体，应创建多个数字员工。

### 6.3 Provider 会话

建议新增以下数据结构：

```text
provider_sessions
provider_session_events
```

`provider_sessions` 保存 Provider 会话映射：

- `provider_session_id`。
- `digital_employee_id`。
- `execution_instance_id`。
- `runtime_node_id`。
- `provider_type`。
- `status`。
- `recoverable`。
- `last_active_at`。

`provider_session_events` 保存 Runtime Agent 回传事件：

- 结构化消息。
- stdout/stderr 摘要或引用。
- tool call。
- 工件引用。
- 风险事件。
- 错误事件。
- 原始输出引用。

每条事件至少关联：

```text
digital_employee_id
execution_instance_id
runtime_node_id
provider_type
provider_session_id
request_id 或 command_id
```

### 6.4 Runtime Scope 语义修正

现有 `runtime_node_scopes` 如果表达“Runtime 节点能服务哪些租户/团队”，需要降级、迁移或替换。

目标态：

- 租户、团队、权限、上下文和审批策略绑定到数字员工。
- 执行实例继承数字员工治理边界。
- Runtime Agent 不直接承载业务身份范围。

## 7. Runtime 接入流程

Runtime Agent 长期配置示例：

```yaml
runtime:
  node_id: customer-vm-01
  control_plane_url: https://control-plane.example.com
  bootstrap_key: env-level-bootstrap-key

workspace:
  base_dir: /data/superteam/workspaces

providers:
  claude_code:
    enabled: true
    binary_path: claude
    timeout: 3600
  opencode:
    enabled: false
    binary_path: opencode
    timeout: 3600
```

`providers` 只表示宿主能力，不表示数字员工身份。

启动流程：

```text
1. Runtime Agent 启动。
2. Runtime Agent 使用 bootstrap_key 调用 enroll/hello。
3. Control Plane 校验 bootstrap_key。
4. Control Plane 写入或刷新 runtime_enrollments。
5. 如果 enrollment 是 pending，Runtime Agent 保持等待，继续周期性 hello 或 heartbeat-lite。
6. Web Runtime 节点页面显示待接入节点。
7. 管理员点击接入。
8. enrollment 变为 approved。
9. Runtime Agent 再次 hello 时获得短期 runtime session token。
10. Runtime Agent 使用 session token 建立 outbound WebSocket。
11. Runtime Agent 上报 capabilities、heartbeat、Provider 健康和 workspace 能力。
```

安全边界：

- `bootstrap_key` 是环境级共享密钥。
- `bootstrap_key` 只允许创建或刷新 pending enrollment。
- `bootstrap_key` 不允许 claim 任务。
- `bootstrap_key` 不允许读取业务数据。
- `bootstrap_key` 不允许上传 Provider 事件。
- `runtime_session_token` 是短期 token，默认 12 小时。
- Runtime session 自动续期。
- 管理员可以撤销 enrollment 或 runtime session。
- Runtime session 被撤销后，WebSocket 断开，后续 claim、session 和 command 操作全部拒绝。

接入审批状态机：

```text
pending
  -> approved
  -> revoked

pending
  -> rejected

approved
  -> revoked
```

连接状态单独计算：

```text
online / offline / degraded
```

连接状态来自 heartbeat 和 WebSocket 状态，不与接入审批状态混用。

## 8. 数字员工执行实例流程

数字员工由 Control Plane 或 Web 创建和治理，Runtime Agent 不创建业务身份。

数字员工状态：

```text
draft
ready
active
disabled
error
```

创建流程：

```text
1. Web 创建数字员工草稿。
2. 管理员配置名称、职责、租户、团队、权限、上下文策略和审批策略。
3. 管理员选择 Runtime Agent。
4. 管理员选择 Provider。
5. 管理员配置 workspace policy 和 session policy。
6. Control Plane 校验 Runtime 已 approved。
7. Control Plane 校验 Runtime online 或最近可用。
8. Control Plane 校验 Provider capability 可用。
9. Control Plane 校验 workspace base_dir 合法。
10. Control Plane 校验用户有权限创建或绑定数字员工。
11. Control Plane 创建 execution instance。
12. Control Plane 通过 WebSocket 下发 ensure_instance。
13. Runtime Agent 创建或确认 agent_home_dir。
14. Runtime Agent 回传 instance_ready 或 instance_error。
15. Control Plane 更新数字员工状态。
```

目录模型：

```text
workspace.base_dir/
  agents/
    <digital_employee_id 或 execution_instance_id>/
      state/
      sessions/
      runs/
        <run_id>/
```

规则：

- 默认以 `agent_home_dir` 和 `sessions/` 为主。
- `runs/<run_id>` 是保留能力。
- `runs/<run_id>` 用于高风险隔离、审计回放、一次性实验或临时隔离。
- 当前设计不以短期任务目录为中心，而以长期数字员工实例和 Provider Session 为中心。

Runtime Agent 接收 Control Plane 指令：

```text
ensure_instance
start_session
resume_session
send_input
stop_session
collect_artifact
```

Runtime Agent 不决定数字员工属于哪个租户或团队，也不决定权限策略。

## 9. Provider Session 与消息交互

Provider Session 是 Claude Code、OpenCode、Codex 或 PI 自身的会话，由 Runtime Agent 管理，Control Plane 持久化映射和事件。

会话策略：

```text
new
  新建 provider session

resume
  使用指定 provider_session_id 恢复

reuse_latest
  使用该数字员工最近一个可恢复 session

ephemeral
  单次会话，不保留恢复能力
```

交互流程：

```text
1. Web 用户向数字员工发起输入。
2. Control Plane 创建 interaction 或 command。
3. Control Plane 检查权限、上下文策略和审批策略。
4. Control Plane 通过 Runtime WebSocket 下发 start_session、resume_session 或 send_input。
5. Runtime Agent 在对应 agent_home_dir 或 session_dir 调用 Provider。
6. Provider 输出 stdout、stderr、JSON stream 或 PTY events。
7. Runtime Agent 捕获、解析、脱敏并结构化。
8. Runtime Agent 回传 provider_session_events。
9. Control Plane 持久化事件、状态、日志和工件引用。
10. Web 订阅或轮询 Control Plane 展示结果。
```

禁止链路：

```text
Provider -> Control Plane
Provider -> Web
Web -> Provider
```

事件分层：

```text
raw_event
  原始输出引用或摘要，用于排障

normalized_event
  平台可理解事件，例如 message_delta、tool_call、artifact_created、risk_detected、error

audit_event
  谁触发、哪个数字员工、哪个 Runtime、哪个 Provider、哪个 session、是否审批
```

Web 展示的是数字员工会话事件，不是 Provider 直接发来的消息。

## 10. Runtime 连接与降级

Runtime Agent 主动连接 Control Plane：

```text
Runtime Agent
  -> 使用 bootstrap_key 获取或续期 runtime session
  -> 主动连接 Control Plane WebSocket
  -> 接收 Control Plane 指令
  -> 回传 Provider Session 事件、日志、状态和工件引用
```

降级路径：

```text
heartbeat
poll commands
push events over HTTP
```

正式控制通道以 Runtime Agent 主动出站连接为主。Runtime Agent 本机 HTTP 地址可以保留为健康检查和调试入口，但不是平台正式反向控制入口。

## 11. Web 页面分工

### 11.1 Runtime 节点

Runtime 节点页面负责宿主接入和能力观察：

- 待接入节点。
- 已接入节点。
- 在线、离线、degraded。
- Provider 能力。
- Workspace 能力。
- 容量和槽位。
- 当前承载的数字员工实例。
- 接入、拒绝、撤销。

Runtime 页面不配置租户或团队业务权限，只展示宿主和连接状态。

### 11.2 数字员工

数字员工页面负责业务身份和唯一执行实例：

- 创建草稿数字员工。
- 配置角色、职责、租户和团队。
- 配置权限、上下文策略和审批策略。
- 选择 Runtime Agent。
- 选择 Provider。
- 配置 session policy。
- 启动、禁用、恢复会话。
- 查看会话事件、工件和审计。

数字员工页面是执行配置主入口。

## 12. MVP 范围

第一阶段实现：

- Runtime enroll、approve、reject、revoke。
- Runtime session token 签发和续期。
- Runtime outbound WebSocket。
- Runtime capabilities 上报。
- 数字员工 draft、ready、active、disabled、error 状态。
- 一个数字员工绑定一个 execution instance。
- `ensure_instance`。
- `start_session`、`resume_session`、`send_input`。
- Provider events 回传并在 Web 展示。

第一阶段不实现：

- 自动调度。
- 一个数字员工绑定多个 execution instance。
- 跨 Runtime 迁移。
- 复杂 fallback。
- 自动扫描本机 workspace。
- Provider 直接连接平台。

预留扩展：

- `runtime_selector`。
- `runtime_labels`。
- `capacity_requirements`。
- `fallback_policy`。

## 13. 测试与验收

后端验收：

- 未批准 Runtime 使用 bootstrap key 后进入 pending enrollment。
- pending Runtime 不能 claim 任务，不能上传 Provider 事件。
- Web 管理员批准后，Runtime 可以获取短期 session token。
- Runtime session token 可以续期。
- Runtime session 被撤销后，WebSocket 和后续命令失败。
- Runtime capabilities 能正确上报 Provider 和 workspace 能力。
- 数字员工只能绑定一个 execution instance。
- disabled 或 error 数字员工不能执行。
- `ensure_instance` 成功后数字员工进入 ready 或 active。
- Provider Session 事件必须关联 digital_employee、execution_instance、runtime_node、provider 和 provider_session。

Runtime Agent 验收：

- Runtime 配置只需要 node_id、control_plane_url、bootstrap_key、workspace 和 providers。
- Runtime Agent 能在 pending 状态下保持等待。
- Runtime Agent 能在 approved 后建立 outbound WebSocket。
- Runtime Agent 能创建或确认 agent_home_dir。
- Runtime Agent 能新建、恢复和继续 Provider Session。
- Runtime Agent 能捕获 Provider 输出并回传结构化事件。

Web 验收：

- Runtime 节点页面能展示待接入、已接入、在线、离线、degraded。
- Runtime 节点页面能执行接入、拒绝、撤销。
- Runtime 节点页面能展示 Provider 和 workspace 能力。
- 数字员工页面能创建 draft。
- 数字员工页面能配置唯一执行实例。
- 数字员工页面能展示 Provider Session 事件、工件和审计。

## 14. 命名约束

SuperTeam 同时包含人类员工和数字员工。后续命名必须避免把 `employee` 默认等同于 AI Agent。

建议约束：

- 人类用户继续通过 `auth_users`、tenant members、team members 或 human employee 相关模型表达。
- 数字员工使用 `digital_employees`。
- 执行实例使用 `digital_employee_execution_instances`。
- Runtime Agent 不命名为 employee。
- Provider Session 不命名为 employee。

## 15. 后续实施顺序建议

建议按以下顺序拆分实现：

1. Runtime enrollment 和 session token。
2. Runtime outbound WebSocket 和 HTTP polling 降级。
3. Runtime capabilities 上报。
4. 数字员工唯一 execution instance。
5. `ensure_instance` 指令和 Runtime agent_home_dir 创建。
6. Provider Session 新建、恢复和事件回传。
7. Runtime 节点页面接入审批。
8. 数字员工页面执行配置和会话展示。

每个阶段都应同步更新 OpenAPI 契约、sqlc 查询、Go/Rust 测试、Web API client 和 `CHANGELOG.md`。
