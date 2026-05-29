# 控制面板核心骨架实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 SuperTeam 控制面板的核心骨架，建立 Control Plane 与 Runtime Agent 的协作机制

**Architecture:** 数据库优先方案 - 先建立完整的 PostgreSQL schema，然后实现 Go 领域服务和 API 层，最后集成 Rust Runtime Agent。使用 HTTP 长轮询代替 WebSocket（降低复杂度）。

**Tech Stack:** Go + chi + pgx + sqlc + Atlas + oapi-codegen, Rust + Tokio + reqwest, PostgreSQL + Redis + MinIO

**参考设计文档:** `docs/superpowers/specs/2026-05-29-control-plane-core-design.md`

---

## 实施说明

本计划分为 4 个 Phase，每个 Phase 可以独立完成并测试。由于涉及大量代码，建议：

1. **使用 worktree 隔离开发**：每个 Phase 在独立的 worktree 中开发
2. **TDD 驱动**：先写测试，再写实现
3. **频繁提交**：每完成一个小功能就提交
4. **Phase 间验证**：每个 Phase 完成后运行集成测试

---

## Phase 1: 数据层（预计 1-2 天）

### 目标
建立完整的数据库 schema 和类型安全的查询代码。

### 文件清单
- `apps/control-plane/atlas.hcl` - Atlas 配置
- `apps/control-plane/internal/storage/migrations/001_initial.sql` - 数据库迁移
- `apps/control-plane/sqlc.yaml` - sqlc 配置
- `apps/control-plane/internal/storage/queries/*.sql` - SQL 查询定义
- `apps/control-plane/internal/storage/queries/*.go` - sqlc 生成的 Go 代码

### 任务分解

#### Task 1.1: 数据库迁移
- [ ] 创建 `atlas.hcl` 配置文件
- [ ] 编写 `001_initial.sql` 迁移文件（包含所有表：runtime_nodes, auth_users, auth_runtime_tokens, tasks, task_executions, task_state_history, task_events, task_artifacts, audit_events）
- [ ] 运行迁移：`cd apps/control-plane && atlas migrate apply --env local`
- [ ] 验证表结构：`psql` 连接数据库检查
- [ ] Commit: `feat(control-plane): add database schema migration`

#### Task 1.2: 配置 sqlc
- [ ] 创建 `sqlc.yaml` 配置
- [ ] 创建查询文件：
  - `queries/runtime.sql` - Runtime 节点 CRUD
  - `queries/tasks.sql` - 任务、执行、事件、工件 CRUD
  - `queries/auth.sql` - 用户和 token CRUD
  - `queries/audit.sql` - 审计日志 CRUD
- [ ] 生成代码：`sqlc generate`
- [ ] 验证编译：`go build ./internal/storage/queries`
- [ ] Commit: `feat(control-plane): add sqlc queries`

#### Task 1.3: 数据层测试
- [ ] 创建 `internal/storage/queries/queries_test.go`
- [ ] 使用 testcontainers 启动 PostgreSQL
- [ ] 测试 Runtime 节点 CRUD
- [ ] 测试任务 CRUD
- [ ] 测试状态转换
- [ ] 运行测试：`go test ./internal/storage/queries -v`
- [ ] Commit: `test(control-plane): add storage layer tests`

---

## Phase 2: 领域服务（预计 2-3 天）

### 目标
实现核心业务逻辑：任务管理、Runtime 节点管理、任务调度、长轮询、认证和审计。

### 文件清单
- `apps/control-plane/internal/task/*.go` - 任务服务
- `apps/control-plane/internal/runtime/*.go` - Runtime 服务
- `apps/control-plane/internal/auth/*.go` - 认证服务
- `apps/control-plane/internal/audit/*.go` - 审计服务

### 任务分解

#### Task 2.1: 任务服务
- [ ] 创建 `internal/task/models.go` - 领域模型
- [ ] 创建 `internal/task/repository.go` - 数据访问接口
- [ ] 创建 `internal/task/state_machine.go` - 状态机（定义允许的状态转换）
- [ ] 创建 `internal/task/service.go` - 任务服务（CreateTask, GetTask, ListTasks, UpdateTaskStatus, CancelTask）
- [ ] 创建 `internal/task/service_test.go` - 单元测试（Mock repository）
- [ ] 运行测试：`go test ./internal/task -v`
- [ ] Commit: `feat(control-plane): add task service`

#### Task 2.2: Runtime 服务
- [ ] 创建 `internal/runtime/models.go` - Runtime 模型
- [ ] 创建 `internal/runtime/repository.go` - 数据访问接口
- [ ] 创建 `internal/runtime/service.go` - Runtime 服务（RegisterNode, UpdateHeartbeat, GetNode, ListNodes）
- [ ] 创建 `internal/runtime/service_test.go` - 单元测试
- [ ] 运行测试：`go test ./internal/runtime -v`
- [ ] Commit: `feat(control-plane): add runtime service`

#### Task 2.3: 任务调度器
- [ ] 创建 `internal/runtime/scheduler.go` - 调度器（SelectNode 根据 provider_type 和负载选择节点）
- [ ] 创建 `internal/runtime/scheduler_test.go` - 测试负载均衡逻辑
- [ ] 运行测试：`go test ./internal/runtime -v`
- [ ] Commit: `feat(control-plane): add task scheduler`

#### Task 2.4: 长轮询管理
- [ ] 创建 `internal/runtime/poller.go` - Poller（WaitForTask, NotifyTask）
- [ ] 创建 `internal/runtime/poller_test.go` - 测试超时和唤醒
- [ ] 运行测试：`go test ./internal/runtime -v`
- [ ] Commit: `feat(control-plane): add long polling poller`

#### Task 2.5: 认证服务
- [ ] 创建 `internal/auth/models.go` - 认证模型
- [ ] 创建 `internal/auth/token.go` - Token 生成和验证（bcrypt hash）
- [ ] 创建 `internal/auth/service.go` - 认证服务（CreateUser, AuthenticateUser, GenerateRuntimeToken, ValidateRuntimeToken）
- [ ] 创建 `internal/auth/service_test.go` - 单元测试
- [ ] 运行测试：`go test ./internal/auth -v`
- [ ] Commit: `feat(control-plane): add auth service`

#### Task 2.6: 审计服务
- [ ] 创建 `internal/audit/service.go` - 审计服务（LogEvent, ListEvents）
- [ ] 创建 `internal/audit/service_test.go` - 单元测试
- [ ] 运行测试：`go test ./internal/audit -v`
- [ ] Commit: `feat(control-plane): add audit service`

---

## Phase 3: API 层（预计 2-3 天）

### 目标
实现 Console API 和 Runtime API，使用 oapi-codegen 生成类型和接口。

### 文件清单
- `contracts/control-plane/openapi.yaml` - OpenAPI 定义
- `apps/control-plane/oapi-codegen.yaml` - 代码生成配置
- `apps/control-plane/internal/api/generated/*.go` - 生成的代码
- `apps/control-plane/internal/api/handlers/**/*.go` - API handlers
- `apps/control-plane/internal/api/middleware.go` - 中间件
- `apps/control-plane/internal/api/router.go` - 路由

### 任务分解

#### Task 3.1: 扩展 OpenAPI 定义
- [ ] 读取现有 `contracts/control-plane/openapi.yaml`
- [ ] 添加 Console API 路径（/api/v1/tasks, /api/v1/runtime/nodes, /api/v1/auth）
- [ ] 添加 Runtime API 路径（/api/v1/runtime/register, /api/v1/runtime/heartbeat, /api/v1/runtime/tasks/poll）
- [ ] 定义所有请求/响应 schema
- [ ] 使用 tags 区分 Console 和 Runtime API
- [ ] Commit: `feat(control-plane): extend OpenAPI definition`

#### Task 3.2: 配置 oapi-codegen
- [ ] 创建 `oapi-codegen.yaml` 配置
- [ ] 生成 Go server 代码：`oapi-codegen -config oapi-codegen.yaml contracts/control-plane/openapi.yaml`
- [ ] 验证生成的代码编译：`go build ./internal/api/generated`
- [ ] Commit: `feat(control-plane): add oapi-codegen config and generate code`

#### Task 3.3: 实现 Console API handlers
- [ ] 创建 `internal/api/handlers/console/tasks.go` - 任务 CRUD handlers
- [ ] 创建 `internal/api/handlers/console/runtime.go` - Runtime 节点查询 handlers
- [ ] 创建 `internal/api/handlers/console/auth.go` - 认证 handlers
- [ ] 创建 `internal/api/handlers/console/handlers_test.go` - 集成测试
- [ ] 运行测试：`go test ./internal/api/handlers/console -v`
- [ ] Commit: `feat(control-plane): add console API handlers`

#### Task 3.4: 实现 Runtime API handlers
- [ ] 创建 `internal/api/handlers/runtime/register.go` - 节点注册 handler
- [ ] 创建 `internal/api/handlers/runtime/heartbeat.go` - 心跳 handler
- [ ] 创建 `internal/api/handlers/runtime/tasks.go` - 任务轮询、领取、事件推送、完成 handlers
- [ ] 创建 `internal/api/handlers/runtime/handlers_test.go` - 集成测试
- [ ] 运行测试：`go test ./internal/api/handlers/runtime -v`
- [ ] Commit: `feat(control-plane): add runtime API handlers`

#### Task 3.5: 实现中间件和路由
- [ ] 创建 `internal/api/middleware.go` - 认证中间件、日志中间件、CORS
- [ ] 更新 `internal/api/router.go` - 注册所有路由
- [ ] 更新 `cmd/control-plane/main.go` - 集成所有服务
- [ ] 启动服务测试：`go run cmd/control-plane/main.go -config config/local.yaml`
- [ ] 手动测试 API：`curl http://localhost:8080/health`
- [ ] Commit: `feat(control-plane): add middleware and router`

#### Task 3.6: 生成 TypeScript client
- [ ] 配置 oapi-codegen 生成 TS 类型
- [ ] 生成到 `packages/api-client/src/generated/`
- [ ] 创建 `packages/api-client/src/index.ts` - 导出类型
- [ ] Commit: `feat(api-client): generate TypeScript client from OpenAPI`

---

## Phase 4: Runtime Agent 集成（预计 1-2 天）

### 目标
Runtime Agent 增加 Control Plane 客户端，实现注册、心跳、任务轮询、事件推送和租约管理。

### 文件清单
- `apps/runtime-agent/src/controlplane/*.rs` - Control Plane 客户端
- `apps/runtime-agent/src/lease/*.rs` - 租约管理
- `apps/runtime-agent/src/config.rs` - 配置更新
- `apps/runtime-agent/src/daemon.rs` - 集成客户端

### 任务分解

#### Task 4.1: Control Plane 客户端
- [ ] 创建 `src/controlplane/mod.rs` - 模块入口
- [ ] 创建 `src/controlplane/models.rs` - API 请求/响应模型（对应 OpenAPI schema）
- [ ] 创建 `src/controlplane/client.rs` - HTTP 客户端（register, heartbeat, poll_task, claim_task, push_events, complete_task, fail_task）
- [ ] 添加依赖：`reqwest`, `serde_json`
- [ ] 创建 `tests/controlplane_client_test.rs` - Mock HTTP server 测试
- [ ] 运行测试：`cargo test --test controlplane_client_test`
- [ ] Commit: `feat(runtime-agent): add control plane client`

#### Task 4.2: 任务轮询
- [ ] 创建 `src/controlplane/poller.rs` - 长轮询循环（task_polling_loop）
- [ ] 处理超时和重试
- [ ] 创建 `tests/poller_test.rs` - 测试轮询逻辑
- [ ] 运行测试：`cargo test --test poller_test`
- [ ] Commit: `feat(runtime-agent): add task polling loop`

#### Task 4.3: 事件推送
- [ ] 创建 `src/controlplane/event_pusher.rs` - 事件批量推送（batch_push_events）
- [ ] 实现缓冲和定时推送
- [ ] 创建 `tests/event_pusher_test.rs` - 测试批量推送
- [ ] 运行测试：`cargo test --test event_pusher_test`
- [ ] Commit: `feat(runtime-agent): add event pusher`

#### Task 4.4: 租约管理
- [ ] 创建 `src/lease/mod.rs` - 模块入口
- [ ] 创建 `src/lease/renewer.rs` - 租约续约循环（lease_renew_loop）
- [ ] 创建 `tests/lease_test.rs` - 测试续约逻辑
- [ ] 运行测试：`cargo test --test lease_test`
- [ ] Commit: `feat(runtime-agent): add lease renewer`

#### Task 4.5: 配置更新
- [ ] 更新 `src/config.rs` - 添加 `control_plane_url`, `token`, `name`
- [ ] 更新配置解析和验证
- [ ] 创建 `config.example.yaml` - 示例配置
- [ ] Commit: `feat(runtime-agent): add control plane config`

#### Task 4.6: 集成到 daemon
- [ ] 更新 `src/daemon.rs` - 在启动时调用 register
- [ ] 启动心跳循环（tokio::spawn heartbeat_loop）
- [ ] 启动任务轮询循环（tokio::spawn task_polling_loop）
- [ ] 集成任务执行流程（execute_task 中推送事件、续约、完成上报）
- [ ] 创建 `tests/daemon_integration_test.rs` - 端到端测试
- [ ] 运行测试：`cargo test --test daemon_integration_test`
- [ ] Commit: `feat(runtime-agent): integrate control plane client into daemon`

---

## Phase 5: 端到端测试和文档（预计 1 天）

### 目标
验证完整流程，编写部署文档和开发指南。

### 任务分解

#### Task 5.1: 端到端测试
- [ ] 启动 PostgreSQL, Redis, MinIO：`docker-compose up -d`
- [ ] 运行数据库迁移：`./scripts/db-migrate.sh`
- [ ] 启动 Control Plane：`go run cmd/control-plane/main.go -config config/local.yaml`
- [ ] 生成 Runtime token：`./scripts/generate-runtime-token.sh node-001`
- [ ] 启动 Runtime Agent：`cargo run -- daemon --node-id node-001 --control-plane-url http://localhost:8080 --token <token>`
- [ ] 通过 API 创建任务：`curl -X POST http://localhost:8080/api/v1/tasks -d '{"title":"test","provider_type":"claude-code","params":{"prompt":"hello"}}'`
- [ ] 验证 Runtime Agent 领取并执行任务
- [ ] 验证事件回传和任务完成
- [ ] 记录测试结果
- [ ] Commit: `test: add end-to-end test documentation`

#### Task 5.2: 编写部署脚本
- [ ] 创建 `scripts/db-migrate.sh` - 数据库迁移脚本
- [ ] 创建 `scripts/generate-runtime-token.sh` - Token 生成脚本
- [ ] 创建 `docker-compose.dev.yml` - 本地开发环境
- [ ] 测试所有脚本
- [ ] Commit: `chore: add deployment scripts`

#### Task 5.3: 更新文档
- [ ] 更新 `README.md` - 添加快速开始指南
- [ ] 创建 `docs/development.md` - 开发指南
- [ ] 创建 `docs/api.md` - API 文档（基于 OpenAPI）
- [ ] 更新 `CHANGELOG.md` - 记录所有变更
- [ ] Commit: `docs: add development and API documentation`

---

## 验收标准

### Phase 1 完成标准
- [ ] 所有数据库表创建成功
- [ ] sqlc 生成的代码编译通过
- [ ] 数据层测试全部通过

### Phase 2 完成标准
- [ ] 所有领域服务单元测试通过
- [ ] 状态机正确处理所有转换
- [ ] 长轮询能正确超时和唤醒

### Phase 3 完成标准
- [ ] OpenAPI 定义完整且有效
- [ ] 所有 API handlers 集成测试通过
- [ ] Control Plane 能成功启动并响应请求

### Phase 4 完成标准
- [ ] Runtime Agent 能成功注册到 Control Plane
- [ ] 心跳正常工作
- [ ] 能轮询、领取、执行任务并回传结果

### Phase 5 完成标准
- [ ] 端到端流程验证通过
- [ ] 所有脚本可用
- [ ] 文档完整

---

## 注意事项

1. **TDD 优先**：每个功能先写测试，再写实现
2. **频繁提交**：每完成一个小任务就提交，commit message 遵循 conventional commits
3. **错误处理**：所有外部调用都要有错误处理和重试逻辑
4. **日志记录**：关键操作记录日志，便于调试
5. **配置验证**：启动时验证所有必需配置
6. **数据库事务**：涉及多表操作使用事务
7. **并发安全**：Poller 等共享状态使用锁保护
8. **资源清理**：goroutine 和 tokio task 要正确清理

---

## 后续工作

Phase 1-4 完成后，可以继续：
- Web 控制台开发（基于设计文档的 Web 规范）
- WebSocket 推送优化（替换 HTTP 长轮询）
- 任务调度器优化（更复杂的负载均衡策略）
- 审批流程和 Workflow 编排
