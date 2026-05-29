# SuperTeam 下一步工作指引

**文档更新日期**: 2026-05-29  
**当前状态**: Phase 1-5 核心骨架已完成

---

## 📍 当前项目状态

### ✅ 已完成的工作

**Phase 1-5 (核心骨架) - 100% 完成**
- ✅ 数据层（数据库 + sqlc + 测试）
- ✅ 领域服务（任务、Runtime、调度、长轮询、认证、审计）
- ✅ API 层（服务器 + 中间件 + handlers）
- ✅ Runtime Agent Control Plane 客户端（Rust）
- ✅ 部署脚本和文档

**详细信息查看：**
- 实施总结：`docs/summary/2026-05-29-control-plane-implementation.md`
- 开发指南：`docs/development.md`
- API 文档：`docs/api.md`

### 🔄 系统可用性

**当前可以做的：**
- ✅ 启动 Control Plane 服务器
- ✅ 注册 Runtime Agent 节点
- ✅ 创建任务（通过 API）
- ✅ 任务调度和分发
- ✅ 长轮询获取任务

**还不能做的：**
- ❌ Runtime Agent 自动执行任务（缺少执行循环）
- ❌ 事件实时推送到 Control Plane
- ❌ Web 控制台界面
- ❌ 完整的端到端测试

---

## 🎯 下一步工作优先级

### 优先级 1：Runtime Agent 任务执行循环（关键）

**目标**: 让 Runtime Agent 能够自动轮询、执行任务并上报结果

**需要实现的功能：**

1. **任务轮询循环** (`apps/runtime-agent/src/controlplane/poller.rs`)
   ```rust
   async fn task_polling_loop(client: ControlPlaneClient, config: RuntimeConfig) {
       loop {
           match client.claim_task(Duration::from_secs(30)).await {
               Ok(Some(task)) => {
                   tokio::spawn(execute_task(client.clone(), task));
               }
               Ok(None) => {} // 超时，继续轮询
               Err(e) => {
                   eprintln!("Poll failed: {}", e);
                   tokio::time::sleep(Duration::from_secs(5)).await;
               }
           }
       }
   }
   ```

2. **任务执行** (`apps/runtime-agent/src/controlplane/executor.rs`)
   ```rust
   async fn execute_task(client: ControlPlaneClient, task: Task) {
       // 1. 根据 provider_type 选择 Provider
       let provider = match task.provider_type.as_str() {
           "claude-code" => ClaudeProvider::new(...),
           "opencode" => OpenCodeProvider::new(...),
           _ => return Err(...),
       };
       
       // 2. 启动 Provider 执行
       let mut event_stream = provider.run(task.params).await?;
       
       // 3. 推送事件到 Control Plane
       while let Some(event) = event_stream.next().await {
           client.push_event(task.id, event).await?;
       }
       
       // 4. 上报完成
       client.complete_task(task.id, result).await?;
   }
   ```

3. **事件推送** (`apps/runtime-agent/src/controlplane/client.rs`)
   ```rust
   impl ControlPlaneClient {
       pub async fn push_event(&self, task_id: i64, event: TaskEvent) -> Result<()>
       pub async fn complete_task(&self, task_id: i64, result: TaskResult) -> Result<()>
       pub async fn fail_task(&self, task_id: i64, error: String) -> Result<()>
   }
   ```

4. **集成到 daemon** (`apps/runtime-agent/src/daemon.rs`)
   ```rust
   pub async fn run_daemon(config: RuntimeConfig) -> Result<()> {
       let client = ControlPlaneClient::new(&config.control_plane_url, &config.token);
       
       // 注册节点
       client.register(...).await?;
       
       // 启动心跳循环
       tokio::spawn(heartbeat_loop(client.clone(), config.node_id.clone()));
       
       // 启动任务轮询循环
       tokio::spawn(task_polling_loop(client.clone(), config.clone()));
       
       // 保持运行
       signal::ctrl_c().await?;
       Ok(())
   }
   ```

**预计工作量**: 1-2 天

**参考文档**: 
- `docs/superpowers/specs/2026-05-29-control-plane-core-design.md` - Section 五、Runtime Agent 集成

---

### 优先级 2：Control Plane 事件接收 API

**目标**: Control Plane 能够接收 Runtime Agent 推送的事件

**需要实现的功能：**

1. **事件推送 API** (`apps/control-plane/internal/api/handlers/runtime.go`)
   ```go
   // POST /api/v1/runtime/tasks/:id/events
   func (h *RuntimeHandler) PushEvents(w http.ResponseWriter, r *http.Request) {
       var req struct {
           Events []TaskEvent `json:"events"`
       }
       // 1. 解析请求
       // 2. 验证任务存在
       // 3. 批量插入事件
       // 4. 返回成功
   }
   ```

2. **任务完成 API** (`apps/control-plane/internal/api/handlers/runtime.go`)
   ```go
   // POST /api/v1/runtime/tasks/:id/complete
   func (h *RuntimeHandler) CompleteTask(w http.ResponseWriter, r *http.Request) {
       var req struct {
           Result json.RawMessage `json:"result"`
       }
       // 1. 更新任务状态为 completed
       // 2. 记录执行结果
       // 3. 更新节点负载
   }
   
   // POST /api/v1/runtime/tasks/:id/fail
   func (h *RuntimeHandler) FailTask(w http.ResponseWriter, r *http.Request) {
       var req struct {
           Error string `json:"error"`
       }
       // 1. 更新任务状态为 failed
       // 2. 记录错误信息
       // 3. 更新节点负载
   }
   ```

**预计工作量**: 0.5 天

---

### 优先级 3：端到端测试

**目标**: 验证完整的任务执行流程

**测试场景：**

1. **基础流程测试**
   ```bash
   # 1. 启动所有服务
   docker-compose -f docker-compose.dev.yml up -d
   ./scripts/db-migrate.sh
   go run apps/control-plane/cmd/server/main.go &
   cargo run --manifest-path apps/runtime-agent/Cargo.toml -- daemon ... &
   
   # 2. 创建任务
   curl -X POST http://localhost:8080/api/v1/tasks \
     -H "Content-Type: application/json" \
     -d '{"title":"测试","provider_type":"claude-code","params":{"prompt":"echo hello"}}'
   
   # 3. 验证任务执行
   # - 检查任务状态变为 claimed
   # - 检查任务状态变为 running
   # - 检查事件流
   # - 检查任务状态变为 completed
   
   # 4. 验证结果
   curl http://localhost:8080/api/v1/tasks/{id}
   curl http://localhost:8080/api/v1/tasks/{id}/events
   ```

2. **错误处理测试**
   - Provider 执行失败
   - 网络中断
   - 超时处理

3. **并发测试**
   - 多个任务同时执行
   - 节点负载均衡

**预计工作量**: 1 天

---

### 优先级 4：Web 控制台（可选）

**目标**: 提供可视化的任务管理界面

**核心页面：**

1. **任务列表页** (`/tasks`)
   - 显示所有任务
   - 筛选（状态、Provider、创建者）
   - 排序和分页

2. **创建任务页** (`/tasks/new`)
   - 表单输入
   - Provider 选择
   - 参数配置

3. **任务详情页** (`/tasks/:id`)
   - 任务信息
   - 实时日志
   - 状态历史
   - 工件列表

4. **节点管理页** (`/runtime/nodes`)
   - 节点列表
   - 状态监控
   - 负载显示

**技术栈：**
- Next.js + React
- shadcn/ui + Tailwind CSS
- TanStack Query + TanStack Table

**预计工作量**: 3-5 天

---

## 🚀 快速开始新会话

### 方式 1：直接告诉我要做什么

```
我想继续开发 SuperTeam 项目。当前 Phase 1-5 已完成（核心骨架）。
下一步我想实现 Runtime Agent 的任务执行循环，让系统能够真正执行任务。

请查看 docs/NEXT_STEPS.md 了解详细信息。
```

### 方式 2：指定具体任务

```
我想实现 SuperTeam Runtime Agent 的任务轮询和执行功能。
具体需要：
1. 任务轮询循环
2. 任务执行逻辑
3. 事件推送到 Control Plane
4. 集成到 daemon

参考 docs/NEXT_STEPS.md 的"优先级 1"部分。
```

### 方式 3：从测试开始

```
我想为 SuperTeam 编写端到端测试，验证从创建任务到执行完成的完整流程。
参考 docs/NEXT_STEPS.md 的"优先级 3"部分。
```

---

## 📚 重要文档索引

### 设计和规划
- **设计文档**: `docs/superpowers/specs/2026-05-29-control-plane-core-design.md`
- **实施计划**: `docs/superpowers/plans/2026-05-29-control-plane-core.md`
- **实施总结**: `docs/summary/2026-05-29-control-plane-implementation.md`

### 开发指南
- **开发指南**: `docs/development.md`
- **API 文档**: `docs/api.md`
- **数据库信息**: `docs/database/conn_info.md`

### 项目规范
- **项目定位**: `CLAUDE.md` 或 `AGENTS.md`
- **变更日志**: `CHANGELOG.md`

---

## 🔧 开发环境快速启动

```bash
# 1. 启动依赖服务
docker-compose -f docker-compose.dev.yml up -d

# 2. 运行数据库迁移
export DATABASE_URL="postgres://superteam:superteam_dev_password@localhost:5432/superteam?sslmode=disable"
./scripts/db-migrate.sh

# 3. 生成 Runtime token
./scripts/generate-runtime-token.sh node-001

# 4. 启动 Control Plane（终端 1）
cd apps/control-plane
go run cmd/server/main.go

# 5. 启动 Runtime Agent（终端 2）
cd apps/runtime-agent
cargo run -- daemon \
  --node-id node-001 \
  --control-plane-url http://localhost:8080 \
  --token <从步骤3获取>

# 6. 测试 API（终端 3）
curl http://localhost:8080/health
curl http://localhost:8080/api/v1/runtime/nodes
```

---

## 💡 开发建议

### 使用 worktree 隔离开发

```bash
# 创建新的 worktree
git worktree add .claude/worktrees/task-execution -b feature/task-execution

# 开发完成后合并
cd /path/to/main/repo
git merge feature/task-execution
git worktree remove .claude/worktrees/task-execution
git branch -d feature/task-execution
```

### 使用 subagent 模式

对于复杂任务，建议使用 subagent 模式：
```
请使用 subagent 模式实现 Runtime Agent 的任务执行循环。
参考 docs/NEXT_STEPS.md 的"优先级 1"部分。
```

### 测试驱动开发

```
请使用 TDD 方式实现任务轮询循环：
1. 先写测试
2. 再写实现
3. 确保测试通过
```

---

## 📞 需要帮助？

如果遇到问题，可以：
1. 查看 `docs/development.md` 的故障排查部分
2. 查看 `docs/summary/2026-05-29-control-plane-implementation.md` 了解已实现的功能
3. 查看 `CHANGELOG.md` 了解最近的变更

---

**最后更新**: 2026-05-29  
**维护者**: SuperTeam 开发团队
