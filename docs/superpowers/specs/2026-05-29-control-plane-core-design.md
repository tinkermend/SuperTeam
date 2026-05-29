# SuperTeam 控制面板核心骨架设计

**设计日期**: 2026-05-29  
**设计目标**: 实现控制面板的核心骨架和基础功能，建立 Control Plane 与 Runtime Agent 的协作机制

## 一、设计概述

### 1.1 设计范围

本设计聚焦于 SuperTeam 控制面板的 MVP 核心功能：

- **任务驱动的执行链路**：人类创建任务 → Control Plane 分配 → Runtime Agent 执行 → 回传结果
- **Runtime Agent 管理**：节点注册、心跳、状态监控
- **任务生命周期管理**：创建、分配、执行、状态流转、事件回传、工件管理
- **基础认证**：用户认证和 Runtime Agent token 认证
- **审计日志**：关键操作的审计记录

**暂不包含**：
- 数字员工分析和复杂审批流程（后续迭代）
- Workflow 编排（后续迭代）
- 多租户和权限管理（后续迭代）

### 1.2 技术方案选择

**方案 A：数据库优先 + 最小 API（已选择）**

核心思路：
- 先建立完整的数据库 schema 和领域模型
- API 层保持最小化
- 使用 HTTP 长轮询代替 WebSocket push（降低复杂度）
- 后续可平滑升级到 WebSocket

优势：
- 数据模型稳定后，上层 API 和 UI 都好做
- HTTP 长轮询实现简单、可靠
- 渐进式演进，降低风险

## 二、数据库设计

### 2.1 表命名规范

**规范原则**：
- 使用模块前缀分组：`{module}_{entity}` 格式
- 核心业务表可简化前缀（如 `tasks` 而非 `task_tasks`）
- 所有表名使用小写 + 下划线（snake_case）
- 主键统一命名为 `id`
- 时间戳字段统一使用 `created_at`, `updated_at`, `deleted_at`
- 外键字段命名为 `{referenced_table}_id`

**模块前缀定义**：
- `runtime_*` - Runtime Agent 节点管理
- `task_*` 或 `tasks` - 任务管理（核心表简化为 `tasks`）
- `workflow_*` - 工作流编排（未来）
- `approval_*` - 审批流程（未来）
- `audit_*` - 审计日志
- `auth_*` - 认证授权
- `tenant_*` - 租户管理（未来）
- `employee_*` - 数字员工定义（未来）
- `capability_*` - 外部能力注册（未来）

**字段类型约定**：
- ID 使用 `BIGSERIAL`（MVP 阶段）
- 时间戳使用 `TIMESTAMPTZ`
- JSON 数据使用 `JSONB`
- 枚举使用 `VARCHAR` + 应用层校验

### 2.2 核心表结构

#### Runtime 模块

```sql
-- Runtime Agent 节点注册表
CREATE TABLE runtime_nodes (
    id BIGSERIAL PRIMARY KEY,
    node_id VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    supported_providers JSONB NOT NULL,
    max_slots INTEGER NOT NULL DEFAULT 1,
    current_load INTEGER NOT NULL DEFAULT 0,
    status VARCHAR(50) NOT NULL,
    metadata JSONB,
    last_heartbeat_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_runtime_nodes_status ON runtime_nodes(status);
CREATE INDEX idx_runtime_nodes_last_heartbeat ON runtime_nodes(last_heartbeat_at);
```

#### Auth 模块

```sql
-- 用户表
CREATE TABLE auth_users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(255) UNIQUE NOT NULL,
    display_name VARCHAR(255),
    email VARCHAR(255),
    password_hash VARCHAR(255),
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Runtime Agent 认证 token 表
CREATE TABLE auth_runtime_tokens (
    id BIGSERIAL PRIMARY KEY,
    node_id VARCHAR(255) UNIQUE NOT NULL,
    token_hash VARCHAR(255) NOT NULL,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_auth_runtime_tokens_node_id ON auth_runtime_tokens(node_id);
```

#### Task 模块

```sql
-- 任务主表
CREATE TABLE tasks (
    id BIGSERIAL PRIMARY KEY,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    creator_id BIGINT REFERENCES auth_users(id),
    provider_type VARCHAR(50) NOT NULL,
    target_node_id VARCHAR(255),
    assigned_node_id VARCHAR(255),
    status VARCHAR(50) NOT NULL,
    workspace_path TEXT,
    params JSONB NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_provider_type ON tasks(provider_type);
CREATE INDEX idx_tasks_assigned_node_id ON tasks(assigned_node_id);
CREATE INDEX idx_tasks_creator_id ON tasks(creator_id);

-- 任务执行记录
CREATE TABLE task_executions (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    node_id VARCHAR(255) NOT NULL,
    provider_session_id VARCHAR(255),
    status VARCHAR(50) NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    result JSONB,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_task_executions_task_id ON task_executions(task_id);
CREATE INDEX idx_task_executions_status ON task_executions(status);

-- 任务状态变更历史
CREATE TABLE task_state_history (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    from_status VARCHAR(50),
    to_status VARCHAR(50) NOT NULL,
    changed_by VARCHAR(255),
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_task_state_history_task_id ON task_state_history(task_id);

-- 任务事件流
CREATE TABLE task_events (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    execution_id BIGINT REFERENCES task_executions(id) ON DELETE CASCADE,
    event_type VARCHAR(100) NOT NULL,
    sequence_number INTEGER NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_task_events_task_id ON task_events(task_id);
CREATE INDEX idx_task_events_execution_id ON task_events(execution_id);
CREATE INDEX idx_task_events_sequence ON task_events(task_id, sequence_number);

-- 任务工件
CREATE TABLE task_artifacts (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    execution_id BIGINT REFERENCES task_executions(id) ON DELETE CASCADE,
    artifact_type VARCHAR(100) NOT NULL,
    name VARCHAR(500) NOT NULL,
    storage_url TEXT NOT NULL,
    size_bytes BIGINT,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_task_artifacts_task_id ON task_artifacts(task_id);
CREATE INDEX idx_task_artifacts_type ON task_artifacts(artifact_type);
```

#### Audit 模块

```sql
-- 审计事件
CREATE TABLE audit_events (
    id BIGSERIAL PRIMARY KEY,
    event_type VARCHAR(100) NOT NULL,
    actor_type VARCHAR(50) NOT NULL,
    actor_id VARCHAR(255) NOT NULL,
    resource_type VARCHAR(50),
    resource_id VARCHAR(255),
    action VARCHAR(100) NOT NULL,
    details JSONB,
    ip_address INET,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_events_actor ON audit_events(actor_type, actor_id);
CREATE INDEX idx_audit_events_resource ON audit_events(resource_type, resource_id);
CREATE INDEX idx_audit_events_created_at ON audit_events(created_at);
```

### 2.3 状态定义

**任务状态流转**：
```
pending → claimed → running → completed
                            → failed
                            → cancelled
```

**Runtime 节点状态**：
- `online`: 最近 60 秒内有心跳
- `offline`: 超过 60 秒未心跳
- `unhealthy`: 心跳正常但节点报告异常

## 三、API 设计

### 3.1 API 路径规范

所有 API 使用统一的 OpenAPI 定义，通过 tags 区分 Console API 和 Runtime API。

**Console API（Web 控制台调用）**：
```
POST   /api/v1/auth/login
GET    /api/v1/auth/me

GET    /api/v1/tasks
POST   /api/v1/tasks
GET    /api/v1/tasks/{id}
PATCH  /api/v1/tasks/{id}
DELETE /api/v1/tasks/{id}
GET    /api/v1/tasks/{id}/events
GET    /api/v1/tasks/{id}/artifacts
GET    /api/v1/tasks/{id}/history

GET    /api/v1/runtime/nodes
GET    /api/v1/runtime/nodes/{node_id}
```

**Runtime API（Runtime Agent 调用）**：
```
POST   /api/v1/runtime/register
POST   /api/v1/runtime/heartbeat
GET    /api/v1/runtime/tasks/poll
POST   /api/v1/runtime/tasks/{id}/claim
POST   /api/v1/runtime/tasks/{id}/renew
POST   /api/v1/runtime/tasks/{id}/events
POST   /api/v1/runtime/tasks/{id}/complete
POST   /api/v1/runtime/tasks/{id}/fail
POST   /api/v1/runtime/tasks/{id}/artifacts
```

### 3.2 认证机制

- **Console API**: Session cookie 或 JWT（MVP 可简化为 API key）
- **Runtime API**: `Authorization: Bearer <token>` header

### 3.3 长轮询机制

Runtime Agent 调用 `/api/v1/runtime/tasks/poll?node_id=xxx&timeout=30`：
- Control Plane 根据 node_id 的 supported_providers 和负载选择任务
- 有任务立即返回，无任务则 hold 最多 30 秒
- 超时返回 204 No Content
- Runtime Agent 收到任务后调用 `/claim` 获取租约

### 3.4 关键请求示例

**创建任务**：
```json
POST /api/v1/tasks
{
  "title": "修复登录页面样式问题",
  "description": "按钮对齐不正确",
  "provider_type": "claude-code",
  "target_node_id": null,
  "workspace_path": "/workspace/project",
  "params": {
    "prompt": "修复 login.tsx 中的按钮对齐问题",
    "environment_variables": {"DEBUG": "true"},
    "timeout": 300
  }
}
```

**推送事件**：
```json
POST /api/v1/runtime/tasks/123/events
{
  "execution_id": 456,
  "events": [
    {
      "sequence_number": 1,
      "event_type": "tool_call",
      "payload": {"tool": "Read", "args": {...}}
    }
  ]
}
```

## 四、Go 代码结构

### 4.1 目录结构

```
apps/control-plane/
  cmd/control-plane/
    main.go
  internal/
    api/
      router.go
      middleware.go
      handlers/
        console/
          tasks.go
          runtime.go
          auth.go
        runtime/
          register.go
          heartbeat.go
          tasks.go
      generated/
        types.gen.go
        server.gen.go
    auth/
      service.go
      token.go
    task/
      service.go
      models.go
      repository.go
      state_machine.go
    runtime/
      service.go
      scheduler.go
      poller.go
    audit/
      service.go
    storage/
      storage.go
      queries/
        queries.sql.go
      migrations/
        001_initial.sql
    config/
      config.go
```

### 4.2 实现顺序

**Phase 1: 数据层（1-2 天）**
1. 编写 Atlas migration SQL
2. 配置 sqlc，生成查询代码
3. 实现 repository 接口
4. 编写单元测试

**Phase 2: 领域服务（2-3 天）**
1. 实现 task.Service（CRUD、状态机）
2. 实现 runtime.Service（节点管理）
3. 实现 runtime.Scheduler（任务分配）
4. 实现 runtime.Poller（长轮询）
5. 实现 auth.Service（认证）
6. 编写单元测试

**Phase 3: API 层（2-3 天）**
1. 扩展 OpenAPI 定义
2. 配置 oapi-codegen
3. 实现 handlers
4. 实现认证中间件
5. 编写集成测试

**Phase 4: Runtime Agent 集成（1-2 天）**
1. 增加 Control Plane 客户端
2. 实现注册、心跳、长轮询
3. 实现事件推送
4. 端到端测试

### 4.3 关键技术点

**长轮询实现**：
```go
type Poller struct {
    waiters map[string]chan *Task
    mu      sync.RWMutex
}

func (p *Poller) WaitForTask(ctx context.Context, nodeID string) (*Task, error) {
    ch := make(chan *Task, 1)
    p.mu.Lock()
    p.waiters[nodeID] = ch
    p.mu.Unlock()
    
    defer func() {
        p.mu.Lock()
        delete(p.waiters, nodeID)
        p.mu.Unlock()
    }()
    
    select {
    case task := <-ch:
        return task, nil
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}
```

**任务调度器**：
```go
func (s *Scheduler) SelectNode(providerType string) (*RuntimeNode, error) {
    // 1. 查询支持该 Provider 且 online 的节点
    // 2. 过滤负载已满的节点
    // 3. 选择负载最低的节点
    // 4. 更新节点 current_load
}
```

**状态机**：
```go
var allowedTransitions = map[string][]string{
    "pending":   {"claimed", "cancelled"},
    "claimed":   {"running", "cancelled"},
    "running":   {"completed", "failed", "cancelled"},
}

func (sm *StateMachine) Transition(taskID int64, toStatus string) error {
    // 1. 查询当前状态
    // 2. 检查转换合法性
    // 3. 更新状态
    // 4. 记录历史
    // 5. 审计日志
}
```

## 五、Runtime Agent 集成

### 5.1 新增模块

```
apps/runtime-agent/src/
  controlplane/
    mod.rs
    client.rs
    models.rs
    poller.rs
    event_pusher.rs
  lease/
    mod.rs
    renewer.rs
```

### 5.2 核心流程

**启动和注册**：
```rust
async fn run_daemon(config: RuntimeConfig) -> Result<()> {
    let cp_client = ControlPlaneClient::new(&config.control_plane_url, &config.token);
    
    cp_client.register(RegisterRequest {
        node_id: config.node_id.clone(),
        name: config.name.clone(),
        supported_providers: vec!["claude-code".into(), "opencode".into()],
        max_slots: config.max_slots,
        metadata: json!({"version": env!("CARGO_PKG_VERSION")}),
    }).await?;
    
    tokio::spawn(heartbeat_loop(cp_client.clone(), config.node_id.clone()));
    tokio::spawn(task_polling_loop(cp_client.clone(), config.clone()));
    
    Ok(())
}
```

**心跳循环**：
```rust
async fn heartbeat_loop(client: ControlPlaneClient, node_id: String) -> Result<()> {
    let mut interval = tokio::time::interval(Duration::from_secs(30));
    
    loop {
        interval.tick().await;
        let current_load = get_current_load();
        client.heartbeat(HeartbeatRequest {
            node_id: node_id.clone(),
            current_load,
            status: "online".into(),
        }).await?;
    }
}
```

**任务轮询和执行**：
```rust
async fn task_polling_loop(client: ControlPlaneClient, config: RuntimeConfig) -> Result<()> {
    loop {
        match client.poll_task(&config.node_id, Duration::from_secs(30)).await {
            Ok(Some(task)) => {
                client.claim_task(task.id).await?;
                tokio::spawn(execute_task(client.clone(), task));
            }
            Ok(None) => {}
            Err(e) => {
                eprintln!("Poll failed: {}", e);
                tokio::time::sleep(Duration::from_secs(5)).await;
            }
        }
    }
}
```

**任务执行和事件推送**：
```rust
async fn execute_task(client: ControlPlaneClient, task: Task) -> Result<()> {
    let provider: Box<dyn ProviderAdapter> = match task.provider_type.as_str() {
        "claude-code" => Box::new(ClaudeProvider::new(/* ... */)),
        "opencode" => Box::new(OpenCodeProvider::new(/* ... */)),
        _ => return Err(anyhow!("Unsupported provider")),
    };
    
    let mut event_stream = provider.run(provider_req).await?;
    let execution_id = client.start_execution(task.id).await?;
    
    let renew_handle = tokio::spawn(lease_renew_loop(client.clone(), task.id));
    
    let mut sequence = 0;
    let mut event_buffer = Vec::new();
    
    while let Some(event) = event_stream.next().await {
        sequence += 1;
        event_buffer.push(TaskEvent {
            sequence_number: sequence,
            event_type: event.event_type.clone(),
            payload: event.payload.clone(),
        });
        
        if event_buffer.len() >= 10 {
            client.push_events(task.id, execution_id, event_buffer.clone()).await?;
            event_buffer.clear();
        }
    }
    
    if !event_buffer.is_empty() {
        client.push_events(task.id, execution_id, event_buffer).await?;
    }
    
    renew_handle.abort();
    client.complete_task(task.id, execution_id, json!({"success": true})).await?;
    
    Ok(())
}
```

**租约续约**：
```rust
async fn lease_renew_loop(client: ControlPlaneClient, task_id: i64) -> Result<()> {
    let mut interval = tokio::time::interval(Duration::from_secs(60));
    
    loop {
        interval.tick().await;
        if let Err(e) = client.renew_lease(task_id).await {
            eprintln!("Renew failed: {}", e);
            return Err(e.into());
        }
    }
}
```

### 5.3 配置变更

```rust
pub struct RuntimeConfig {
    pub node_id: String,
    pub name: String,
    pub control_plane_url: String,
    pub token: String,
    pub max_slots: usize,
    pub run_log_dir: PathBuf,
    pub claude_bin: PathBuf,
    pub opencode_bin: PathBuf,
    pub http_addr: SocketAddr,
}
```

## 六、Web 控制台功能规范

### 6.1 核心页面

1. **任务列表页** (`/tasks`)
   - 表格显示所有任务
   - 筛选：状态、Provider 类型、创建者、时间
   - 排序和分页
   - 状态 Badge 颜色编码

2. **创建任务页** (`/tasks/new`)
   - 表单：标题、描述、Provider、节点、工作目录、参数
   - React Hook Form + Zod 校验
   - 提交后跳转详情页

3. **任务详情页** (`/tasks/:id`)
   - 任务基本信息
   - 实时执行日志（xterm.js 或滚动 div）
   - 状态变更历史
   - 工件列表（下载、预览）
   - 取消任务按钮

4. **Runtime Agent 管理页** (`/runtime/nodes`)
   - 节点列表表格
   - 状态、负载、支持的 Provider
   - 最后心跳时间

5. **节点详情页** (`/runtime/nodes/:node_id`)
   - 节点基本信息
   - 正在执行的任务
   - 历史统计

### 6.2 技术栈

- TanStack Table（表格）
- TanStack Query（数据获取）
- shadcn/ui（UI 组件）
- React Hook Form + Zod（表单）
- xterm.js（终端日志）
- oapi-codegen 生成的 TS client（`packages/api-client/`）

### 6.3 实时更新

- 任务详情页：轮询或 WebSocket 获取实时事件
- 任务列表页：TanStack Query 的 `refetchInterval`

## 七、测试策略

### 7.1 测试分层

1. **数据层测试**（Go）
   - 使用 testcontainers 启动 PostgreSQL
   - 测试 CRUD 和状态转换

2. **领域服务测试**（Go）
   - Mock repository
   - 测试业务逻辑

3. **API 集成测试**（Go）
   - 测试 HTTP server
   - 测试完整请求-响应流程

4. **Runtime Agent 测试**（Rust）
   - Mock Control Plane HTTP server
   - 测试注册、心跳、任务执行

5. **端到端测试**
   - 启动真实服务
   - 验证完整流程

## 八、部署和配置

### 8.1 Control Plane 配置

```yaml
http:
  addr: "0.0.0.0:8080"

postgres:
  url: "postgres://superteam:password@localhost:5432/superteam?sslmode=disable"

redis:
  url: "redis://localhost:6379/0"

object_store:
  endpoint: "http://localhost:9000"
  region: "us-east-1"
  bucket: "superteam-artifacts"
  access_key_id: "minioadmin"
  secret_access_key: "minioadmin"
  force_path_style: true

auth:
  jwt_secret: "change-me-in-production"
  runtime_token_ttl: "720h"
```

### 8.2 Runtime Agent 配置

```yaml
node_id: "node-001"
name: "Dev Machine 01"
control_plane_url: "http://localhost:8080"
token: "runtime-token-xxx"
max_slots: 5
run_log_dir: "/var/log/superteam/runtime"
claude_bin: "/usr/local/bin/claude"
opencode_bin: "/usr/local/bin/opencode"
http_addr: "127.0.0.1:9090"
```

### 8.3 开发工作流

1. 启动依赖服务：`docker-compose -f docker-compose.dev.yml up -d`
2. 运行数据库迁移：`./scripts/db-migrate.sh`
3. 启动 Control Plane：`go run cmd/control-plane/main.go -config config/local.yaml`
4. 生成 Runtime token：`./scripts/generate-runtime-token.sh node-001`
5. 启动 Runtime Agent：`cargo run -- daemon --node-id node-001 --control-plane-url http://localhost:8080 --token <token>`
6. 启动 Web 控制台：`pnpm dev`

## 九、实施里程碑

**Week 1: 数据层和领域服务**
- Atlas migration 定义
- sqlc 配置和代码生成
- Task/Runtime/Auth service 实现
- 单元测试覆盖

**Week 2: API 层**
- OpenAPI 契约扩展
- oapi-codegen 配置
- Console/Runtime API handlers
- 认证中间件
- 集成测试

**Week 3: Runtime Agent 集成**
- Control Plane 客户端
- 注册、心跳、长轮询
- 事件推送、租约管理
- 端到端测试

**Week 4: Web 控制台（后续）**
- 任务列表页
- 创建任务页
- 任务详情页
- Runtime 节点管理页

## 十、后续演进路径

**Phase 1: MVP（当前设计）**
- HTTP 长轮询 + 数据库
- 任务驱动的基础执行链路

**Phase 2: 实时优化**
- 升级为 WebSocket 推送
- 保持数据层不变

**Phase 3: 高级特性**
- 任务调度器优化
- 负载均衡
- 数字员工分析
- 审批流程
- Workflow 编排
