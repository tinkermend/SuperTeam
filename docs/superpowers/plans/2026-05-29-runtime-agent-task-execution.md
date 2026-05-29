# Runtime Agent 任务执行循环实施计划

**计划日期**: 2026-05-29  
**设计文档**: `docs/superpowers/specs/2026-05-29-runtime-agent-task-execution-design.md`  
**预计工期**: 3.5 天

---

## 概述

本计划实现 Runtime Agent 的任务执行循环，包括：
- 任务轮询、队列管理、执行
- 事件实时推送到 Control Plane
- 租约续约机制
- 优雅关闭

**架构方案**: 单体执行器 + 三个并发循环（polling、execution、lease renewal）

---

## Phase 1: Control Plane 客户端扩展

**预计时间**: 0.5 天  
**目标**: 为 ControlPlaneClient 添加任务执行所需的 API 方法

### 任务 1.1: 添加新的 API 方法

**文件**: `apps/runtime-agent/src/controlplane/client.rs`

需要添加 5 个新方法：

1. `update_task_status` - 更新任务状态
2. `push_event` - 推送事件
3. `complete_task` - 标记任务完成
4. `fail_task` - 标记任务失败
5. `renew_lease` - 续约任务租约

**实现要点**:
- 所有方法使用 bearer token 认证
- 错误处理：非 2xx 状态码返回错误
- 使用 `context()` 包装错误信息

### 任务 1.2: 编写单元测试

**文件**: `apps/runtime-agent/src/controlplane/client.rs` (tests 模块)

测试内容：
- 客户端创建
- URL 构造正确性
- 请求头设置（bearer token）

---

## Phase 2: 工作目录管理

**预计时间**: 0.5 天  
**目标**: 实现任务工作目录的创建、清理和管理

### 任务 2.1: 创建 workspace 模块

**文件**: `apps/runtime-agent/src/executor/workspace.rs`

需要实现：

1. `TaskWorkspace` 结构体
2. `create_task_workspace()` - 创建独立工作目录
3. `cleanup_workspace()` - 根据策略清理
4. `remove_workspace()` - 删除目录
5. `cleanup_old_workspaces()` - 清理旧目录

**清理策略**:
- `on_success`: 只在成功时清理
- `on_completion`: 完成就清理
- `never`: 不清理

### 任务 2.2: 编写单元测试

测试内容：
- 工作目录创建
- 各种清理策略
- 旧目录清理（保留最近 N 个）

---

## Phase 3: 任务执行逻辑

**预计时间**: 1 天  
**目标**: 实现单个任务的完整执行流程

### 任务 3.1: 创建 task 模块

**文件**: `apps/runtime-agent/src/executor/task.rs`

核心函数：`execute_task()`

执行流程：
1. 创建工作目录
2. 更新状态为 running
3. 选择 Provider
4. 构造请求
5. 启动 Provider
6. 循环推送事件
7. 完成或失败

### 任务 3.2: 实现辅助函数

同文件实现：

1. `select_provider()` - 根据 provider_type 选择
2. `extract_prompt()` - 从 params 提取 prompt
3. `extract_model()` - 从 params 提取 model

### 任务 3.3: 创建 retry 模块

**文件**: `apps/runtime-agent/src/executor/retry.rs`

实现：

1. `push_event_with_retry()` - 事件推送重试
2. `renew_lease_with_retry()` - 续约重试
3. `is_retryable_error()` - 判断是否可重试

**重试策略**:
- 最多 3 次
- 指数退避（100ms * 2^attempt）
- 只重试网络错误

### 任务 3.4: 编写单元测试

测试内容：
- Provider 选择逻辑
- 参数提取
- 重试逻辑
- 错误分类

---

## Phase 4: TaskExecutor 和三个循环

**预计时间**: 1 天  
**目标**: 实现核心执行器和三个主循环

### 任务 4.1: 创建 executor 模块

**文件**: `apps/runtime-agent/src/executor/mod.rs`

实现 `TaskExecutor` 结构体：

```rust
pub struct TaskExecutor {
    config: RuntimeConfig,
    control_plane: ControlPlaneClient,
    task_queue: Arc<Mutex<BinaryHeap<QueuedTask>>>,
    active_tasks: Arc<Mutex<HashMap<i64, ActiveTask>>>,
    semaphore: Arc<Semaphore>,
    shutdown_token: CancellationToken,
}
```

方法：
- `new()` - 创建执行器
- `run()` - 启动执行器（包含优雅关闭）

### 任务 4.2: 实现三个循环

**文件**: `apps/runtime-agent/src/executor/loops.rs`

实现：

1. `polling_loop()` - 长轮询获取任务
2. `execution_loop()` - 执行任务
3. `lease_renewal_loop()` - 续约

**关键点**:
- 使用 `tokio::select!` 监听 shutdown
- Execution loop 使用 `try_acquire_owned`
- 续约失败时取消任务

### 任务 4.3: 实现优雅关闭

在 `TaskExecutor::run()` 中实现：

1. 监听 SIGTERM/SIGINT
2. 取消 shutdown_token
3. 等待活跃任务（最多 60 秒）
4. 超时后强制取消

### 任务 4.4: 添加依赖

**文件**: `apps/runtime-agent/Cargo.toml`

添加：
```toml
tokio-util = "0.7"
```

### 任务 4.5: 编写集成测试

测试内容：
- 任务队列优先级
- 并发控制（semaphore）
- 优雅关闭机制

---

## Phase 5: 集成和测试

**预计时间**: 0.5 天  
**目标**: 集成到 daemon 并进行端到端测试

### 任务 5.1: 更新 daemon

**文件**: `apps/runtime-agent/src/daemon.rs`

修改 `run_daemon()`:

1. 创建 ControlPlaneClient
2. 注册节点
3. 启动心跳循环
4. 创建并启动 TaskExecutor

### 任务 5.2: 更新 lib.rs

**文件**: `apps/runtime-agent/src/lib.rs`

添加：
```rust
pub mod executor;
```

### 任务 5.3: 端到端测试

测试场景：

1. **基础流程**:
   - 启动 Control Plane
   - 启动 Runtime Agent
   - 创建任务
   - 验证任务执行
   - 验证事件推送
   - 验证任务完成

2. **错误处理**:
   - Provider 执行失败
   - 网络中断（事件推送重试）
   - 续约失败（任务取消）

3. **并发测试**:
   - 创建多个任务
   - 验证并发限制
   - 验证优先级排序

4. **优雅关闭**:
   - 发送 SIGTERM
   - 验证停止接受新任务
   - 验证等待活跃任务
   - 验证超时强制终止

---

## 实施检查清单

### Phase 1: Control Plane 客户端扩展
- [ ] 添加 5 个新 API 方法
- [ ] 编写单元测试
- [ ] 测试通过

### Phase 2: 工作目录管理
- [ ] 创建 workspace.rs
- [ ] 实现创建和清理逻辑
- [ ] 实现清理策略
- [ ] 编写单元测试
- [ ] 测试通过

### Phase 3: 任务执行逻辑
- [ ] 创建 task.rs
- [ ] 实现 execute_task()
- [ ] 实现辅助函数
- [ ] 创建 retry.rs
- [ ] 实现重试逻辑
- [ ] 编写单元测试
- [ ] 测试通过

### Phase 4: TaskExecutor 和三个循环
- [ ] 创建 executor/mod.rs
- [ ] 实现 TaskExecutor 结构
- [ ] 创建 loops.rs
- [ ] 实现三个循环
- [ ] 实现优雅关闭
- [ ] 添加 tokio-util 依赖
- [ ] 编写集成测试
- [ ] 测试通过

### Phase 5: 集成和测试
- [ ] 更新 daemon.rs
- [ ] 更新 lib.rs
- [ ] 端到端测试：基础流程
- [ ] 端到端测试：错误处理
- [ ] 端到端测试：并发
- [ ] 端到端测试：优雅关闭
- [ ] 所有测试通过

---

## 风险和注意事项

### 技术风险

1. **PriorityQueue 实现**
   - 使用 `BinaryHeap` 需要实现 `Ord` trait
   - 需要按 priority 降序 + queued_at 升序排列

2. **Semaphore 所有权**
   - 使用 `try_acquire_owned()` 获取 owned permit
   - Permit 需要在 spawn 的任务中持有

3. **优雅关闭时序**
   - 确保 polling loop 先停止
   - 确保 execution loop 最后停止
   - 避免死锁

### 实施注意事项

1. **最小化实现**
   - 只实现设计中的功能
   - 不添加额外特性
   - 保持代码简洁

2. **错误处理**
   - 所有网络调用都要处理错误
   - 使用 `anyhow::Context` 添加上下文
   - 区分可重试和不可重试错误

3. **测试覆盖**
   - 每个模块都要有单元测试
   - 关键流程要有集成测试
   - 端到端测试覆盖主要场景

---

## 后续工作

完成本计划后，下一步工作：

1. **Control Plane 事件接收 API**
   - 实现 `/api/v1/runtime/tasks/:id/events`
   - 实现 `/api/v1/runtime/tasks/:id/complete`
   - 实现 `/api/v1/runtime/tasks/:id/fail`
   - 实现 `/api/v1/runtime/tasks/:id/renew`

2. **端到端测试**
   - 完整的任务执行流程测试
   - 性能测试
   - 压力测试

3. **Web 控制台**（可选）
   - 任务列表页
   - 任务详情页
   - Runtime 节点管理页

---

**计划创建日期**: 2026-05-29  
**计划创建者**: SuperTeam 开发团队
