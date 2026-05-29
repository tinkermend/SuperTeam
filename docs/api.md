# SuperTeam Control Plane API 文档

本文档描述 SuperTeam Control Plane 的 REST API。

## 基础信息

- **Base URL**: `http://localhost:8080` (开发环境)
- **API Version**: v1
- **Content-Type**: `application/json`

## 认证

### Console API 认证

当前 foundation 阶段的 product server 尚未挂载 Console session 认证路由；本文档只描述当前已注册的任务和 Runtime API。Cookie-based Session 登录、当前用户和登出接口将在 auth router 接入后补齐。

### Runtime API 认证

Runtime API 使用 Bearer Token 认证：

```
Authorization: Bearer <runtime-token>
X-Node-ID: <runtime-node-id>
```

使用 `scripts/generate-runtime-token.sh <node-id> [token] [ttl]` 生成 token；默认 TTL 为 `30 days`。
`X-Node-ID` 必须和 token 对应的 Runtime 节点一致；注册接口中的 `node_id` 也必须与该 header 一致。

Runtime Agent 默认通过 Control Plane Runtime API 领取、续租和回传任务结果；本地 Runtime Agent HTTP API 仅用于诊断和本地 provider run 结果查看，不是业务任务分发入口。

## API 端点

### 健康检查

#### GET /health

检查 Control Plane 服务状态。

**请求示例**

```bash
curl http://localhost:8080/health
```

**响应示例**

```json
{
  "status": "ok",
  "service": "control-plane"
}
```

---

## Console API

Console API 供 Web 控制台使用。当前 foundation 阶段的 product server 已注册任务管理端点，Console session 认证路由尚未挂载。

### 任务管理

#### POST /api/v1/tasks

创建新任务。

**请求体**

```json
{
  "title": "分析代码库",
  "description": "分析 SuperTeam 代码库并生成报告",
  "provider_type": "claude-code",
  "params": {
    "prompt": "分析当前代码库的架构",
    "context": {
      "repo_path": "/path/to/repo"
    }
  },
  "priority": 5
}
```

**字段说明**

- `title` (string, required): 任务标题
- `description` (string, optional): 任务描述
- `provider_type` (string, required): Provider 类型，如 `claude-code`, `opencode`, `codex`
- `params` (object, required): Provider 特定参数
- `priority` (integer, optional): 任务优先级，数值越大越优先；未传时由服务端使用默认值

**响应示例**

```json
{
  "id": 1,
  "title": "分析代码库",
  "description": "分析 SuperTeam 代码库并生成报告",
  "status": "pending",
  "provider_type": "claude-code",
  "params": {
    "prompt": "分析当前代码库的架构",
    "context": {
      "repo_path": "/path/to/repo"
    }
  },
  "priority": 5,
  "created_at": "2026-05-29T10:00:00Z",
  "updated_at": "2026-05-29T10:00:00Z"
}
```

#### GET /api/v1/tasks/{taskId}

获取任务详情。

**请求示例**

```bash
curl http://localhost:8080/api/v1/tasks/1
```

**响应示例**

```json
{
  "id": 1,
  "title": "分析代码库",
  "status": "running",
  "provider_type": "claude-code",
  "assigned_node_id": "node-001",
  "params": {
    "prompt": "分析当前代码库的架构"
  },
  "created_at": "2026-05-29T10:00:00Z",
  "updated_at": "2026-05-29T10:01:30Z"
}
```

#### GET /api/v1/tasks

列出任务。

**查询参数**

- `limit` (integer, optional): 返回数量，默认 20，最大 100
- `offset` (integer, optional): 偏移量，默认 0

**请求示例**

```bash
curl "http://localhost:8080/api/v1/tasks?limit=10"
```

**响应示例**

```json
[
  {
    "id": 1,
    "title": "分析代码库",
    "status": "running",
    "provider_type": "claude-code",
    "params": {
      "prompt": "分析当前代码库的架构"
    },
    "priority": 5,
    "created_at": "2026-05-29T10:00:00Z",
    "updated_at": "2026-05-29T10:01:30Z"
  }
]
```

#### PUT /api/v1/tasks/{taskId}/status

更新任务状态。

**请求体**

```json
{
  "status": "cancelled"
}
```

**响应示例**

```json
{
  "id": 1,
  "status": "cancelled",
  "updated_at": "2026-05-29T10:05:00Z"
}
```

### Runtime 节点管理

#### GET /api/v1/runtime/nodes

列出 Runtime 节点。

**查询参数**

- `limit` (integer, optional): 返回数量上限
- `offset` (integer, optional): 分页偏移

**请求示例**

```bash
curl http://localhost:8080/api/v1/runtime/nodes
```

**响应示例**

```json
[
  {
    "node_id": "node-001",
    "name": "Node 001",
    "supported_providers": ["claude-code", "opencode"],
    "max_slots": 4,
    "current_load": 2,
    "status": "online",
    "metadata": {
      "hostname": "dev-machine",
      "os": "darwin",
      "arch": "arm64"
    },
    "last_heartbeat_at": "2026-05-29T10:04:50Z",
    "created_at": "2026-05-29T09:00:00Z",
    "updated_at": "2026-05-29T10:04:50Z"
  }
]
```

#### GET /api/v1/runtime/nodes/{nodeId}

获取节点详情。

**响应示例**

```json
{
  "node_id": "node-001",
  "name": "Node 001",
  "supported_providers": ["claude-code", "opencode"],
  "max_slots": 4,
  "current_load": 2,
  "status": "online",
  "metadata": {
    "hostname": "dev-machine",
    "os": "darwin",
    "arch": "arm64"
  },
  "last_heartbeat_at": "2026-05-29T10:04:50Z",
  "created_at": "2026-05-29T09:00:00Z",
  "updated_at": "2026-05-29T10:04:50Z"
}
```

---

## Runtime API

Runtime API 供 Runtime Agent 调用 Control Plane 使用，需要 Runtime Token 认证。

### 节点管理

#### POST /api/v1/runtime/register

注册 Runtime 节点。

**请求头**

```
Authorization: Bearer <runtime-token>
X-Node-ID: node-001
```

**请求体**

```json
{
  "node_id": "node-001",
  "name": "Node 001",
  "supported_providers": ["claude-code", "opencode"],
  "max_slots": 4,
  "metadata": {
    "hostname": "dev-machine",
    "os": "darwin",
    "arch": "arm64"
  }
}
```

**响应示例**

```json
{
  "node_id": "node-001",
  "name": "Node 001",
  "supported_providers": ["claude-code", "opencode"],
  "max_slots": 4,
  "current_load": 0,
  "status": "online",
  "metadata": {
    "hostname": "dev-machine",
    "os": "darwin",
    "arch": "arm64"
  },
  "last_heartbeat_at": "2026-05-29T09:00:00Z",
  "created_at": "2026-05-29T09:00:00Z",
  "updated_at": "2026-05-29T09:00:00Z"
}
```

#### POST /api/v1/runtime/heartbeat

发送心跳。

**请求头**

```
Authorization: Bearer <runtime-token>
X-Node-ID: node-001
```

**请求体**

```json
{
  "current_load": 2,
  "status": "online"
}
```

**响应示例**

```json
{
  "node_id": "node-001",
  "name": "Node 001",
  "supported_providers": ["claude-code", "opencode"],
  "max_slots": 4,
  "current_load": 2,
  "status": "online",
  "metadata": {
    "hostname": "dev-machine",
    "os": "darwin",
    "arch": "arm64"
  },
  "last_heartbeat_at": "2026-05-29T10:05:00Z",
  "created_at": "2026-05-29T09:00:00Z",
  "updated_at": "2026-05-29T10:05:00Z"
}
```

### Runtime 任务主链路

当前 Runtime 任务主链路的 canonical Control Plane 路径为：

```text
POST /api/v1/runtime/tasks/claim
POST /api/v1/runtime/tasks/{taskId}/events
POST /api/v1/runtime/tasks/{taskId}/complete
POST /api/v1/runtime/tasks/{taskId}/fail
POST /api/v1/runtime/tasks/{taskId}/lease
```

说明：

- Runtime Agent 通过这些 Control Plane endpoint 完成 claim、事件回传、完成、失败和 lease 续约。
- Console 和其他客户端不应直接把业务任务派发到 Runtime Agent 本地接口。

#### POST /api/v1/runtime/tasks/claim

领取下一个可执行任务。

**请求头**

```
Authorization: Bearer <runtime-token>
X-Node-ID: node-001
```

**查询参数**

- `timeout` (integer, optional): 长轮询超时时间（秒），默认由服务端控制，最大 60

**响应示例**

```json
{
  "id": 1,
  "title": "分析代码库",
  "status": "claimed",
  "provider_type": "claude-code",
  "assigned_node_id": "node-001",
  "params": {
    "prompt": "分析当前代码库的架构"
  },
  "priority": 5,
  "created_at": "2026-05-29T10:00:00Z",
  "updated_at": "2026-05-29T10:01:00Z"
}
```

### 任务执行

#### POST /api/v1/runtime/tasks/{taskId}/events

推送任务事件。

**请求头**

```
Authorization: Bearer <runtime-token>
X-Node-ID: node-001
```

**请求体**

```json
{
  "events": [
    {
      "type": "text_delta",
      "text": "正在分析文件..."
    }
  ]
}
```

**响应**

- `202 Accepted`
- 空响应体

#### POST /api/v1/runtime/tasks/{taskId}/complete

标记任务完成。

**请求头**

```
Authorization: Bearer <runtime-token>
X-Node-ID: node-001
```

**请求体**

```json
{
  "result": {
    "summary": "分析完成"
  }
}
```

说明：

- 当前 foundation 阶段，`complete` 请求体可以为空；如果传入请求体，必须是合法 JSON。
- Control Plane 当前会校验请求体 JSON 并把任务状态更新为 `completed`，但不会持久化 `result` 内容；结果持久化属于后续能力。

**响应示例**

```json
{
  "id": 1,
  "title": "分析代码库",
  "status": "completed",
  "provider_type": "claude-code",
  "priority": 5,
  "assigned_node_id": "node-001",
  "updated_at": "2026-05-29T10:06:00Z"
}
```

#### POST /api/v1/runtime/tasks/{taskId}/fail

标记任务失败。

**请求头**

```
Authorization: Bearer <runtime-token>
X-Node-ID: node-001
```

**请求体**

```json
{
  "error": "provider exited"
}
```

**响应示例**

```json
{
  "id": 1,
  "title": "分析代码库",
  "status": "failed",
  "provider_type": "claude-code",
  "priority": 5,
  "assigned_node_id": "node-001",
  "updated_at": "2026-05-29T10:06:00Z"
}
```

#### POST /api/v1/runtime/tasks/{taskId}/lease

续约任务租约。

**请求头**

```
Authorization: Bearer <runtime-token>
X-Node-ID: node-001
```

**请求体**

无，请直接发送空请求体。

**响应**

- `204 No Content`

说明：

- 当前 foundation 阶段，lease endpoint 仅用于确认任务存在并返回 `204`。
- 该接口尚未持久化租约续约记录，也不提供独立 lease 元数据返回。

---

## 数据模型

### 任务状态

- `pending`: 等待分配
- `claimed`: 已被 Runtime Agent 领取
- `running`: 执行中
- `completed`: 已完成
- `failed`: 失败
- `cancelled`: 已取消

### 任务优先级

- `0-4`: 较低优先级
- `5`: 默认优先级
- `6+`: 更高优先级

### Provider 类型

- `claude-code`: Claude Code
- `opencode`: OpenCode
- `codex`: Codex
- `pi`: Pi

### 事件类型

- `task_started`: 任务开始
- `progress_update`: 进度更新
- `log_output`: 日志输出
- `artifact_created`: 产物创建
- `task_completed`: 任务完成
- `task_failed`: 任务失败

### 产物类型

- `report`: 报告
- `code`: 代码
- `log`: 日志
- `screenshot`: 截图
- `file`: 通用文件

---

## 错误处理

当前 foundation 阶段，错误响应使用 `text/plain` 响应体返回错误文本；统一 JSON 错误 envelope 属于后续 API 标准化能力。

### 常见错误码

- `400 Bad Request`: 请求参数错误
- `401 Unauthorized`: 未认证或认证失败
- `403 Forbidden`: 无权限
- `404 Not Found`: 资源不存在
- `409 Conflict`: 资源冲突（如任务已被领取）
- `500 Internal Server Error`: 服务器内部错误

---

## 速率限制

当前 foundation server 尚未启用速率限制 middleware。Console/API 配额、Runtime 节点级限流和 `429 Too Many Requests` 响应属于后续策略层能力。

---

## 示例工作流

### 创建并执行任务

1. **Console 创建任务**

```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "title": "测试任务",
    "provider_type": "claude-code",
    "params": {"prompt": "echo hello"}
  }'
```

2. **Runtime Agent claim 任务**

```bash
curl -X POST "http://localhost:8080/api/v1/runtime/tasks/claim?timeout=30" \
  -H "Authorization: Bearer <token>" \
  -H "X-Node-ID: node-001"
```

3. **Runtime Agent 推送事件**

```bash
curl -X POST http://localhost:8080/api/v1/runtime/tasks/<task-id>/events \
  -H "Authorization: Bearer <token>" \
  -H "X-Node-ID: node-001" \
  -H "Content-Type: application/json" \
  -d '{
    "events": [{
      "type": "text_delta",
      "text": "hello"
    }]
  }'
```

4. **Runtime Agent 完成任务**

```bash
curl -X POST http://localhost:8080/api/v1/runtime/tasks/<task-id>/complete \
  -H "Authorization: Bearer <token>" \
  -H "X-Node-ID: node-001" \
  -H "Content-Type: application/json" \
  -d '{
    "result": {"summary": "完成"}
  }'
```

5. **Console 查看结果**

```bash
curl http://localhost:8080/api/v1/tasks/<task-id>
```

---

## 相关文档

- [开发指南](./development.md)
- [OpenAPI 规范](../contracts/control-plane/openapi.yaml)
- [架构设计](../CLAUDE.md)
