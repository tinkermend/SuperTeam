# Runtime Agent 任务执行循环设计

**设计日期**: 2026-05-29  
**设计目标**: 实现 Runtime Agent 的任务执行循环，让系统能够自动轮询、执行任务并上报结果

---

## 一、设计概述

### 1.1 设计范围

本设计实现 Runtime Agent 的核心执行能力：

- **任务轮询循环**：持续从 Control Plane 长轮询获取任务
- **任务队列管理**：按优先级管理待执行任务
- **任务执行**：根据 provider_type 选择并执行 Provider
- **事件推送**：实时推送执行事件到 Control Plane
- **租约续约**：定期为活跃任务续约
- **优雅关闭**：收到 SIGTERM 时等待任务完成

### 1.2 设计决策

基于需求讨论，做出以下关键决策：

| 决策点 | 选择 | 理由 |
|--------|------|------|
| 并发控制 | 任务队列模型 | 更好地控制执行顺序和状态 |
| 事件推送 | 实时推送 | 用户可以立即看到执行进度 |
| 重试策略 | 混合策略 | 网络错误重试，执行失败不重试 |
| 工作目录 | 独立目录 | 任务间完全隔离，适合并发 |
| 租约续约 | 固定间隔（30秒） | 简单可靠 |
| 关闭机制 | 优雅关闭（60秒超时） | 平衡可靠性和响应速度 |

### 1.3 架构方案

**选择方案 A：单体执行器架构**

核心思路：
- 一个 `TaskExecutor` 组件统一管理任务队列、执行、事件推送、续约
- 使用 Tokio channels 在组件间传递消息
- 所有逻辑集中在一个模块，易于理解和调试

优势：
- 实现简单，所有状态在一个结构体中
- 易于测试和调试
- 适合 MVP 快速迭代

---

## 二、整体架构

### 2.1 核心组件

```rust
pub struct TaskExecutor {
    // 配置和客户端
    config: RuntimeConfig,
    control_plane: ControlPlaneClient,
    
    // 任务队列和状态
    task_queue: Arc<Mutex<PriorityQueue<QueuedTask>>>,
    active_tasks: Arc<Mutex<HashMap<i64, ActiveTask>>>,
    
    // 并发控制
    semaphore: Arc<Semaphore>,
    
    // 优雅关闭
    shutdown_token: CancellationToken,
}

struct QueuedTask {
    task: Task,
    priority: i32,
    queued_at: Instant,
}

// 注：PriorityQueue 使用标准库的 BinaryHeap 实现
// 按 priority 降序排列（高优先级先执行）
// 相同优先级按 queued_at 升序（先入先出）

struct ActiveTask {
    task: Task,
    handle: JoinHandle<()>,
    cancel_token: CancellationToken,
    started_at: Instant,
}
```

### 2.2 三个主要循环

1. **Polling Loop**：从 Control Plane 长轮询获取任务，加入队列
2. **Execution Loop**：从队列取任务，获取 semaphore 许可后执行
3. **Lease Renewal Loop**：每 30 秒为所有活跃任务续约

### 2.3 生命周期

```
启动 → 三个循环并发运行 → 收到 SIGTERM → 
停止轮询 → 等待活跃任务完成（最多 60 秒）→ 强制终止 → 退出
```

---

## 三、任务执行流程

### 3.1 单个任务的执行流程

```rust
async fn execute_task(
    task: Task,
    control_plane: ControlPlaneClient,
    config: RuntimeConfig,
    cancel_token: CancellationToken,
) -> Result<()> {
    // 1. 创建独立工作目录
    let workspace = create_task_workspace(&config, task.id)?;
    
    // 2. 更新任务状态为 running
    control_plane.update_task_status(task.id, TaskStatus::Running).await?;
    
    // 3. 选择 Provider
    let provider = select_provider(&task.provider_type, &config)?;
    
    // 4. 构造 Provider 请求
    let request = ProviderRequest {
        prompt: extract_prompt(&task.params)?,
        workspace_path: workspace.path.clone(),
        session_id: None,
        continue_session: false,
        model: extract_model(&task.params),
    };
    
    // 5. 启动 Provider，获取事件流
    let mut event_stream = provider.run(request).await?;
    
    // 6. 实时推送事件到 Control Plane
    while let Some(event_result) = event_stream.next().await {
        if cancel_token.is_cancelled() {
            return Err(anyhow!("Task cancelled"));
        }
        
        match event_result {
            Ok(event) => {
                // 实时推送，带重试
                push_event_with_retry(&control_plane, task.id, event).await?;
            }
            Err(e) => {
                // Provider 执行失败，不重试
                control_plane.fail_task(task.id, e.to_string()).await?;
                cleanup_workspace(&workspace, &config)?;
                return Err(e);
            }
        }
    }
    
    // 7. 任务完成
    control_plane.complete_task(task.id, json!({"status": "success"})).await?;
    cleanup_workspace(&workspace, &config)?;
    
    Ok(())
}
```

### 3.2 关键点

- 每个任务独立工作目录（`workspaces/task-{id}`）
- 事件实时推送，网络错误自动重试（最多 3 次，指数退避）
- Provider 执行失败直接上报，不重试
- 支持通过 `cancel_token` 取消任务
- 根据 `cleanup_policy` 清理工作目录

---

## 四、三个主要循环

### 4.1 Polling Loop（任务轮询）

```rust
async fn polling_loop(
    control_plane: ControlPlaneClient,
    task_queue: Arc<Mutex<PriorityQueue<QueuedTask>>>,
    shutdown_token: CancellationToken,
) {
    loop {
        tokio::select! {
            _ = shutdown_token.cancelled() => {
                println!("Polling loop shutting down");
                break;
            }
            result = control_plane.claim_task(30) => {
                match result {
                    Ok(Some(task)) => {
                        let mut queue = task_queue.lock().await;
                        queue.push(QueuedTask {
                            priority: task.priority,
                            task,
                            queued_at: Instant::now(),
                        });
                    }
                    Ok(None) => {
                        // 超时，继续轮询
                    }
                    Err(e) => {
                        eprintln!("Poll failed: {}, retrying in 5s", e);
                        tokio::time::sleep(Duration::from_secs(5)).await;
                    }
                }
            }
        }
    }
}
```

**特点**：
- 使用长轮询（30 秒超时）
- 轮询失败后等待 5 秒重试
- 监听 shutdown 信号

### 4.2 Execution Loop（任务执行）

```rust
async fn execution_loop(
    control_plane: ControlPlaneClient,
    config: RuntimeConfig,
    task_queue: Arc<Mutex<PriorityQueue<QueuedTask>>>,
    active_tasks: Arc<Mutex<HashMap<i64, ActiveTask>>>,
    semaphore: Arc<Semaphore>,
    shutdown_token: CancellationToken,
) {
    loop {
        tokio::select! {
            _ = shutdown_token.cancelled() => {
                println!("Execution loop shutting down");
                break;
            }
            _ = tokio::time::sleep(Duration::from_millis(100)) => {
                // 尝试从队列取任务
                let queued_task = {
                    let mut queue = task_queue.lock().await;
                    queue.pop()
                };
                
                if let Some(queued_task) = queued_task {
                    // 获取 semaphore 许可（非阻塞）
                    if let Ok(permit) = semaphore.clone().try_acquire_owned() {
                        let task = queued_task.task;
                        let task_id = task.id;
                        let cancel_token = CancellationToken::new();
                        
                        // 启动任务执行
                        let cp = control_plane.clone();
                        let cfg = config.clone();
                        let ct = cancel_token.clone();
                        let active = active_tasks.clone();
                        
                        let handle = tokio::spawn(async move {
                            let result = execute_task(task, cp.clone(), cfg, ct).await;
                            
                            // 执行完成，释放 permit
                            drop(permit);
                            
                            // 从 active_tasks 移除
                            active.lock().await.remove(&task_id);
                            
                            if let Err(e) = result {
                                eprintln!("Task {} failed: {}", task_id, e);
                            }
                        });
                        
                        // 记录到 active_tasks
                        active_tasks.lock().await.insert(task_id, ActiveTask {
                            task: task.clone(),
                            handle,
                            cancel_token,
                            started_at: Instant::now(),
                        });
                    } else {
                        // 没有可用槽位，放回队列
                        let mut queue = task_queue.lock().await;
                        queue.push(queued_task);
                    }
                }
            }
        }
    }
}
```

**特点**：
- 使用 `try_acquire_owned` 非阻塞获取 semaphore
- 没有槽位时将任务放回队列
- 任务完成后自动释放 permit 和清理状态

### 4.3 Lease Renewal Loop（租约续约）

```rust
async fn lease_renewal_loop(
    control_plane: ControlPlaneClient,
    active_tasks: Arc<Mutex<HashMap<i64, ActiveTask>>>,
    shutdown_token: CancellationToken,
) {
    let mut interval = tokio::time::interval(Duration::from_secs(30));
    
    loop {
        tokio::select! {
            _ = shutdown_token.cancelled() => {
                println!("Lease renewal loop shutting down");
                break;
            }
            _ = interval.tick() => {
                let task_ids: Vec<i64> = {
                    let active = active_tasks.lock().await;
                    active.keys().copied().collect()
                };
                
                for task_id in task_ids {
                    // 续约，带重试
                    if let Err(e) = renew_lease_with_retry(&control_plane, task_id).await {
                        eprintln!("Failed to renew lease for task {}: {}", task_id, e);
                        // 续约失败，取消任务
                        if let Some(active_task) = active_tasks.lock().await.get(&task_id) {
                            active_task.cancel_token.cancel();
                        }
                    }
                }
            }
        }
    }
}
```

**特点**：
- 每 30 秒批量续约所有活跃任务
- 续约失败时取消对应任务
- 带重试机制

---

## 五、错误处理和重试策略

### 5.1 网络错误重试

```rust
async fn push_event_with_retry(
    control_plane: &ControlPlaneClient,
    task_id: i64,
    event: ProviderEvent,
) -> Result<()> {
    let max_retries = 3;
    let mut attempt = 0;
    
    loop {
        match control_plane.push_event(task_id, &event).await {
            Ok(_) => return Ok(()),
            Err(e) if is_retryable_error(&e) && attempt < max_retries => {
                attempt += 1;
                let backoff = Duration::from_millis(100 * 2_u64.pow(attempt - 1));
                eprintln!("Push event failed (attempt {}): {}, retrying in {:?}", 
                         attempt, e, backoff);
                tokio::time::sleep(backoff).await;
            }
            Err(e) => {
                return Err(anyhow!("Push event failed after {} retries: {}", 
                                  max_retries, e));
            }
        }
    }
}

async fn renew_lease_with_retry(
    control_plane: &ControlPlaneClient,
    task_id: i64,
) -> Result<()> {
    let max_retries = 3;
    let mut attempt = 0;
    
    loop {
        match control_plane.renew_lease(task_id).await {
            Ok(_) => return Ok(()),
            Err(e) if is_retryable_error(&e) && attempt < max_retries => {
                attempt += 1;
                let backoff = Duration::from_millis(200 * 2_u64.pow(attempt - 1));
                tokio::time::sleep(backoff).await;
            }
            Err(e) => return Err(e),
        }
    }
}

fn is_retryable_error(error: &anyhow::Error) -> bool {
    let error_str = error.to_string().to_lowercase();
    error_str.contains("timeout") 
        || error_str.contains("connection") 
        || error_str.contains("network")
        || error_str.contains("502")
        || error_str.contains("503")
        || error_str.contains("504")
}
```

### 5.2 错误分类

1. **可重试错误**（自动重试 3 次，指数退避）：
   - 网络超时
   - 连接错误
   - 5xx 服务器错误

2. **不可重试错误**（直接失败）：
   - Provider 执行失败
   - 参数错误
   - 4xx 客户端错误
   - 工作目录创建失败

3. **致命错误**（取消任务）：
   - 租约续约失败
   - 收到取消信号

---

## 六、优雅关闭机制

### 6.1 关闭流程

```rust
impl TaskExecutor {
    pub async fn run(self) -> Result<()> {
        let shutdown_token = self.shutdown_token.clone();
        
        // 监听 SIGTERM 和 SIGINT
        let shutdown_signal = async {
            let mut sigterm = tokio::signal::unix::signal(
                tokio::signal::unix::SignalKind::terminate()
            ).expect("Failed to setup SIGTERM handler");
            let mut sigint = tokio::signal::unix::signal(
                tokio::signal::unix::SignalKind::interrupt()
            ).expect("Failed to setup SIGINT handler");
            
            tokio::select! {
                _ = sigterm.recv() => println!("Received SIGTERM"),
                _ = sigint.recv() => println!("Received SIGINT"),
            }
        };
        
        // 启动三个主循环
        let polling = tokio::spawn(polling_loop(...));
        let execution = tokio::spawn(execution_loop(...));
        let lease_renewal = tokio::spawn(lease_renewal_loop(...));
        
        // 等待关闭信号
        shutdown_signal.await;
        println!("Shutdown signal received, starting graceful shutdown...");
        
        // 1. 停止轮询和续约循环
        shutdown_token.cancel();
        let _ = tokio::join!(polling, lease_renewal);
        
        // 2. 等待活跃任务完成（最多 60 秒）
        let shutdown_timeout = Duration::from_secs(60);
        let start = Instant::now();
        
        loop {
            let active_count = self.active_tasks.lock().await.len();
            
            if active_count == 0 {
                println!("All tasks completed, shutting down");
                break;
            }
            
            if start.elapsed() > shutdown_timeout {
                println!("Shutdown timeout reached, {} tasks still running", active_count);
                
                // 强制取消所有任务
                let active = self.active_tasks.lock().await;
                for (task_id, active_task) in active.iter() {
                    println!("Force cancelling task {}", task_id);
                    active_task.cancel_token.cancel();
                }
                
                tokio::time::sleep(Duration::from_secs(5)).await;
                break;
            }
            
            println!("Waiting for {} tasks to complete...", active_count);
            tokio::time::sleep(Duration::from_secs(5)).await;
        }
        
        // 3. 等待 execution loop 退出
        let _ = execution.await;
        
        println!("TaskExecutor shutdown complete");
        Ok(())
    }
}
```

### 6.2 关闭阶段

1. **Phase 1（立即）**：收到 SIGTERM/SIGINT 信号
   - 取消 `shutdown_token`
   - 停止接受新任务（polling loop 退出）
   - 停止续约（lease renewal loop 退出）

2. **Phase 2（最多 60 秒）**：等待活跃任务完成
   - 每 5 秒检查一次活跃任务数量
   - 任务自然完成后从 `active_tasks` 移除

3. **Phase 3（超时后）**：强制终止
   - 取消所有活跃任务的 `cancel_token`
   - 等待 5 秒让取消生效
   - 退出程序

---

## 七、工作目录管理

### 7.1 工作目录结构

```rust
struct TaskWorkspace {
    path: PathBuf,
    task_id: i64,
    created_at: Instant,
}

fn create_task_workspace(config: &RuntimeConfig, task_id: i64) -> Result<TaskWorkspace> {
    let workspace_path = config.workspace.base_dir.join(format!("task-{}", task_id));
    
    std::fs::create_dir_all(&workspace_path)
        .context("Failed to create task workspace")?;
    
    Ok(TaskWorkspace {
        path: workspace_path,
        task_id,
        created_at: Instant::now(),
    })
}
```

### 7.2 清理策略

```rust
fn cleanup_workspace(workspace: &TaskWorkspace, config: &RuntimeConfig) -> Result<()> {
    match config.workspace.cleanup_policy.as_str() {
        "on_success" => {
            // 只在任务成功时清理
            remove_workspace(workspace)?;
        }
        "on_completion" => {
            // 任务完成就清理（无论成功失败）
            remove_workspace(workspace)?;
        }
        "never" => {
            // 不清理
            println!("Workspace retained at: {:?}", workspace.path);
        }
        policy => {
            eprintln!("Unknown cleanup policy: {}, defaulting to 'on_completion'", policy);
            remove_workspace(workspace)?;
        }
    }
    
    // 检查并清理旧的工作目录
    cleanup_old_workspaces(config)?;
    
    Ok(())
}
```

### 7.3 清理策略说明

1. **on_success**：只在任务成功完成时删除工作目录
   - 失败的任务保留工作目录用于调试

2. **on_completion**：任务完成就删除（无论成功失败）
   - 节省磁盘空间

3. **never**：永不删除
   - 保留所有执行历史
   - 需要配合 `max_retained` 限制数量

4. **max_retained**：保留最近 N 个工作目录
   - 定期清理旧目录
   - 防止磁盘占满

---

## 八、Control Plane API 扩展

### 8.1 新增客户端方法

需要在 `ControlPlaneClient` 中添加以下方法：

```rust
impl ControlPlaneClient {
    /// Update task status
    pub async fn update_task_status(&self, task_id: i64, status: TaskStatus) -> Result<()>
    
    /// Push event to Control Plane
    pub async fn push_event(&self, task_id: i64, event: &ProviderEvent) -> Result<()>
    
    /// Complete task
    pub async fn complete_task(&self, task_id: i64, result: serde_json::Value) -> Result<()>
    
    /// Fail task
    pub async fn fail_task(&self, task_id: i64, error: String) -> Result<()>
    
    /// Renew task lease
    pub async fn renew_lease(&self, task_id: i64) -> Result<()>
}
```

### 8.2 辅助函数

```rust
fn select_provider(provider_type: &str, config: &RuntimeConfig) -> Result<Box<dyn ProviderAdapter>>

fn extract_prompt(params: &serde_json::Value) -> Result<String>

fn extract_model(params: &serde_json::Value) -> Option<String>
```

### 8.3 集成到 daemon

```rust
pub async fn run_daemon(config: RuntimeConfig, token: String) -> Result<()> {
    let client = ControlPlaneClient::new(&config.runtime.control_plane_url, &token);
    
    // 注册节点
    client.register(...).await?;
    
    // 启动心跳循环
    tokio::spawn(heartbeat_loop(client.clone(), config.clone()));
    
    // 启动任务执行器
    let executor = TaskExecutor::new(config, client);
    executor.run().await?;
    
    Ok(())
}
```

---

## 九、文件结构

```
apps/runtime-agent/src/
├── controlplane/
│   ├── client.rs          # 扩展 API 方法（update_task_status、push_event、complete_task、fail_task、renew_lease）
│   └── models.rs          # 现有模型
├── executor/
│   ├── mod.rs             # TaskExecutor 主结构、new()、run() 方法
│   ├── loops.rs           # 三个主循环（polling_loop、execution_loop、lease_renewal_loop）
│   ├── task.rs            # 任务执行逻辑（execute_task、select_provider、extract_prompt/model）
│   ├── retry.rs           # 重试策略（push_event_with_retry、renew_lease_with_retry、is_retryable_error）
│   └── workspace.rs       # 工作目录管理（create_task_workspace、cleanup_workspace、cleanup_old_workspaces）
└── daemon.rs              # 集成入口（run_daemon、heartbeat_loop）
```

**依赖项**：
- `tokio-util` (CancellationToken)
- 现有依赖已满足其他需求

---

## 十、实施顺序

**Phase 1：Control Plane 客户端扩展**（0.5 天）
1. 添加新的 API 方法
2. 编写单元测试

**Phase 2：工作目录管理**（0.5 天）
1. 实现工作目录创建和清理
2. 实现清理策略

**Phase 3：任务执行逻辑**（1 天）
1. 实现 `execute_task` 函数
2. 实现 Provider 选择和参数提取
3. 实现事件推送和重试

**Phase 4：TaskExecutor 和三个循环**（1 天）
1. 实现 `TaskExecutor` 结构
2. 实现三个主循环
3. 实现优雅关闭

**Phase 5：集成和测试**（0.5 天）
1. 集成到 daemon
2. 端到端测试

**总计**：约 3.5 天

---

## 十一、测试策略

### 11.1 单元测试

- Control Plane 客户端方法
- 工作目录管理
- 重试逻辑
- 错误分类

### 11.2 集成测试

- 完整的任务执行流程
- 优雅关闭机制
- 并发任务执行
- 租约续约

### 11.3 端到端测试

- 启动 Control Plane 和 Runtime Agent
- 创建任务并验证执行
- 验证事件推送
- 验证任务完成/失败

---

## 十二、后续优化方向

1. **性能优化**：
   - 事件批量推送（减少网络请求）
   - 更智能的重试策略

2. **可观测性**：
   - 添加 metrics（任务执行时间、成功率等）
   - 结构化日志

3. **高级特性**：
   - 任务优先级动态调整
   - 任务依赖关系
   - 任务暂停/恢复

---

**设计完成日期**: 2026-05-29  
**设计者**: SuperTeam 开发团队
