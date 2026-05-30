# Changelog

All notable changes to the SuperTeam Control Plane project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

#### Core Summary 状态映射 (2026-05-30)

- 为任务和 Runtime 节点 summary helper 增加稳定状态 tone，供后续 Web 页面复用。
- 为 Runtime 节点 summary 增加负载百分比，避免每个页面重复计算槽位占用。

#### API Client 最小任务与 Runtime 覆盖 (2026-05-30)

- 为 `packages/api-client` 补齐任务详情、任务状态更新、任务取消和 Runtime 节点详情的最小 client 方法。
- 通过 Vitest 锁定这些方法使用的 Control Plane canonical path。

#### Foundation 契约漂移检查 (2026-05-30)

- 新增 `pnpm verify:contracts`，检查 Control Plane OpenAPI、Go route、Rust Control Plane client 和 TypeScript api-client 的关键路径一致性。
- 新增 `pnpm verify:foundation`，聚合契约检查、TypeScript 测试、TypeScript 类型检查和 Runtime Agent Rust 测试。

#### Foundation Readiness 底座收口设计 (2026-05-30)

- 新增 Foundation Readiness 设计文档，明确在进入具体功能开发前采用 Web、Control Plane、Runtime Agent、contracts 与共享 packages 的均衡收口方案。
- 定义本阶段的维护性、可扩展性、可复用性标准，并明确不提前实现登录认证、Temporal、OpenFGA、完整业务页面和生产级 Provider 治理。

#### Web 真实数据接入底座 (2026-05-29)

- 为任务和 Runtime 节点补充最小 API client 与 core summary helper，后续页面可从 mock 数据平滑切换到真实接口。

#### Foundation fake provider 端到端验收 (2026-05-29)

- 新增 fake provider 风格的最小端到端验收，覆盖任务创建、Runtime 注册、claim、事件回传和完成状态。
- 对齐 Runtime Agent 客户端的节点注册/心跳响应模型，移除未在 Control Plane contract 中承诺的内部数据库 `id` 依赖。
- 将 Control Plane Runtime 写入端点接入 Runtime token + `X-Node-ID` 认证，并让 Runtime Agent 对心跳、claim、事件、完成、失败和 lease 请求携带节点身份。
- 修正 Runtime token 生成脚本，使其写入当前 `auth_runtime_tokens(node_id, token_hash, expires_at)` schema。

#### Foundation Hardening 设计 Spec (2026-05-29)

- 新增 Foundation Hardening 设计文档，明确 Control Plane 启动边界、sqlc 生成闭环、契约事实源、Runtime Agent daemon 边界、执行事件流和 Web 真实数据接入底座。

#### Web 根布局 hydration 兼容 (2026-05-29)

- 在 Web 根布局 `<html>` 上启用 `suppressHydrationWarning`，降低浏览器扩展向根节点注入属性时触发的 hydration mismatch 噪音，并补充对应布局测试。

#### Web 控制台通用骨架 (2026-05-29)

- 沉淀 Web 控制台外部系统骨架复用组件：新增 `ConsoleShell`、状态胶囊、图标徽章、指标块、分区面板和时间线项，并将首页改为基于共享组件挂载。

#### Control Plane S3 对象存储接入 (2026-05-29)

- 使用 AWS SDK for Go v2 的 `config`、`credentials`、`service/s3` 初始化控制平面 S3 客户端。
- 新增 `S3ObjectStore` 边界，封装对象上传、下载、存在性检查和删除，并返回稳定的 `s3://bucket/key` 工件引用。
- 补齐 S3 配置校验，启动前检查 endpoint、region、bucket、access key 和 secret key。
- 更新配置模板和配置指南，保留 MinIO/localstack path-style 默认值，并补充 Volcengine TOS virtual-hosted 配置示例。

#### Runtime 任务执行结果 API (2026-05-29)

- 补齐 Runtime task events、complete、fail 和 lease endpoint 的基础处理，支持 Runtime Agent 回传结构化执行事件和终态。

#### Phase 4 - Runtime Agent Control Plane 集成 (2026-05-29)

- 添加 Runtime Agent Control Plane 客户端 (`apps/runtime-agent/src/controlplane/`)
  - client.rs: HTTP 客户端实现
    - ControlPlaneClient 结构：封装 reqwest HTTP 客户端
    - register(): 注册节点到 Control Plane
    - heartbeat(): 发送心跳更新节点状态和负载
    - claim_task(): 长轮询获取任务（支持超时）
    - 完整的错误处理和上下文信息
  - models.rs: API 模型定义
    - TaskStatus 枚举 (pending/claimed/running/completed/failed/cancelled)
    - NodeStatus 枚举 (online/offline)
    - RegisterNodeRequest/Response
    - HeartbeatRequest/Response
    - Task 模型（包含完整任务信息）
    - 所有模型支持 serde 序列化/反序列化
  - mod.rs: 模块导出
- 更新 Cargo.toml
  - 将 reqwest 从 dev-dependencies 移至 dependencies
  - 启用 json 和 rustls-tls 特性
- 添加集成测试 (`apps/runtime-agent/tests/controlplane_client_test.rs`)
  - 客户端创建测试
  - 请求序列化测试
  - 集成测试（需要运行的 Control Plane，默认 ignored）
    - 节点注册测试
    - 心跳更新测试
    - 任务 claim 超时测试
  - 所有单元测试通过

#### Phase 2.3 - 任务调度器 (2026-05-29)

- 添加任务调度器 (`apps/control-plane/internal/runtime/scheduler.go`)
  - Scheduler 结构：负责任务到节点的调度
  - SelectNode 方法：智能节点选择
    - 查询支持指定 Provider 且在线的节点
    - 过滤负载已满的节点 (current_load >= max_slots)
    - 选择负载最低的节点实现负载均衡
    - 自动更新节点 current_load
  - 错误处理：ErrNoAvailableNode
- 添加调度器测试 (`apps/control-plane/internal/runtime/scheduler_test.go`)
  - 单节点调度测试
  - 负载均衡测试（多节点选择最低负载）
  - Provider 过滤测试
  - 容量过滤测试（排除满载节点）
  - 无可用节点错误测试
  - 复杂场景测试（混合 Provider、负载、容量）
  - 11 个测试用例全部通过

#### Phase 2.2 - Runtime 服务 (2026-05-29)

- 添加 Runtime 节点管理服务 (`apps/control-plane/internal/runtime/`)
  - models.go: 领域模型定义
    - NodeStatus 枚举 (online/offline)
    - Node 模型及辅助方法 (IsOnline, HasCapacity, SupportsProvider)
    - RegisterNodeRequest, UpdateHeartbeatRequest 请求模型
    - ListNodesFilter 过滤器模型
    - pgtype 类型转换辅助函数
  - repository.go: 数据访问接口
    - CRUD 操作 (CreateNode, GetNode, ListNodes, UpdateHeartbeat, UpdateLoad, UpdateStatus, DeleteNode)
    - ListOnlineNodes: 查询心跳在阈值内的在线节点
  - service.go: 业务逻辑实现
    - RegisterNode: 注册新节点或更新已存在节点
    - UpdateHeartbeat: 更新心跳和负载，自动检测节点状态
    - GetNode: 根据 ID 查询节点
    - ListNodes: 列出节点，支持状态过滤和分页
    - ListOnlineNodes: 列出在线节点（60秒心跳阈值）
    - JSON 序列化支持 (providers, metadata)
  - service_test.go: 完整的单元测试
    - 使用 testify/mock 实现 MockRepository
    - 覆盖所有服务方法的正向和负向测试用例
    - 输入验证测试
    - 分页和限制测试
    - 15 个测试用例全部通过

#### Phase 2.1 - 任务服务 (2026-05-29)

- 添加任务管理服务 (`apps/control-plane/internal/task/`)
  - models.go: 任务领域模型
  - repository.go: 任务数据访问接口
  - state_machine.go: 任务状态机
  - service.go: 任务服务实现
  - service_test.go: 单元测试

#### Phase 1.3 - 数据层测试 (2026-05-29)

- 添加完整的数据层测试套件 (`apps/control-plane/internal/storage/queries/queries_test.go`)
  - Runtime 节点测试：创建、查询、心跳更新、在线节点列表
  - 任务测试：创建、查询、列表过滤、状态更新、状态转换、事件流
  - 认证测试：用户创建、查询、Runtime token 创建和验证
  - 审计测试：事件创建、列表查询、统计、时间过滤
- 添加 Runtime 节点查询 (`apps/control-plane/internal/storage/queries/runtime.sql`)
  - CreateRuntimeNode, GetRuntimeNode, UpdateRuntimeNodeHeartbeat
  - UpdateRuntimeNodeLoad, UpdateRuntimeNodeStatus
  - ListOnlineNodes, ListRuntimeNodes, DeleteRuntimeNode
- 添加认证查询 (`apps/control-plane/internal/storage/queries/auth.sql`)
  - CreateUser, GetUser, GetUserByUsername, GetUserByEmail
  - UpdateUser, ListUsers, DeleteUser
  - CreateRuntimeToken, GetRuntimeToken, ValidateRuntimeToken, DeleteRuntimeToken
- 添加测试辅助脚本 (`apps/control-plane/test.sh`)
  - 自动检测 Podman socket 位置
  - 设置正确的环境变量
- 添加测试文档 (`apps/control-plane/internal/storage/queries/README.md`)
  - 测试覆盖说明
  - 运行指南
  - 故障排查
- 添加测试依赖
  - testcontainers-go v0.42.0
  - testcontainers-go/modules/postgres v0.42.0
  - stretchr/testify (latest)

#### Phase 1.2 - 配置 sqlc (2026-05-29)

- 配置 sqlc 代码生成 (`apps/control-plane/sqlc.yaml`)
- 生成任务查询代码 (`apps/control-plane/internal/storage/queries/tasks.sql.go`)
- 生成审计查询代码 (`apps/control-plane/internal/storage/queries/audit.sql.go`)

#### Phase 1.1 - 数据库迁移 (2026-05-29)

- 初始数据库 schema (`apps/control-plane/internal/storage/migrations/001_initial.sql`)
  - Runtime 节点表 (runtime_nodes)
  - 认证表 (auth_users, auth_runtime_tokens)
  - 任务表 (tasks, task_executions, task_state_history, task_events, task_artifacts)
  - 审计表 (audit_events)
  - 索引和触发器

### Changed

#### Foundation Readiness 文档收口 (2026-05-30)

- 同步 README、开发指南、API 文档和下一步指引，明确底座阶段的启动、验证、契约守护和 testcontainers 环境边界。

#### Foundation 文档同步 (2026-05-29)

- 同步 README、开发指南、API 文档和下一步指引，使文档状态与已验证的 Foundation baseline 保持一致。

#### Runtime Agent daemon 默认语义 (2026-05-29)

- 将 Runtime Agent 正式运行边界收敛为受 Control Plane 管理的 daemon，并补充认证 token 配置、环境变量和 CLI 覆盖。

#### Runtime API 契约路径收敛 (2026-05-29)

- 将 Runtime 任务 claim、事件、完成、失败和 lease 续约路径统一收敛到 Control Plane 的 `/api/v1/runtime/tasks/...` canonical contract，并将 Runtime Agent 本地契约保留为诊断和本地 run API。

#### Control Plane 启动边界收敛 (2026-05-29)

- 收敛 Control Plane 主启动入口，明确 health-only router 与产品 API server 的边界，并通过统一装配路径连接存储、服务和 handlers。

- 将 Control Plane PostgreSQL 和 Redis 配置示例切换到 `docs/database/conn_info.md` 记录的远端地址，并修正连接验证命令。
- 在远端 PostgreSQL 创建 `superteam` 应用用户、数据库和 schema，并从本地 `127.0.0.1` 的 `superteam` 数据库迁移当前 schema 与迁移记录。

### Deprecated

### Removed

### Fixed

#### Control Plane 迁移命令目录对齐 (2026-05-30)

- 修正 `apps/control-plane/Makefile` 的 Atlas 迁移目录，统一指向实际 schema 源 `internal/storage/migrations`。

#### Control Plane API 响应契约对齐 (2026-05-30)

- 为任务与 Runtime 节点 API 响应补充显式 DTO，统一输出 snake_case 字段，避免直接编码领域模型时泄漏 Go 字段名。
- 将任务响应中的 `params` 规范化为 JSON object；空值、无效 JSON 或非对象输入统一回退为 `{}`，避免返回 base64 字符串。
- 更新 API/e2e 测试，锁定 `create/get/list/update/cancel/claim/complete/fail` 等任务路径及 Runtime 节点路径的真实 JSON shape。
- 收敛 Runtime claim 到 canonical `/api/v1/runtime/tasks/claim`，移除旧别名路由，并同步 API/OpenAPI 文档对 complete 与 lease 当前能力边界的描述。

#### Runtime Agent 配置入口统一 (2026-05-29)

- 统一 Runtime Agent 配置模型，支持 `--config` 加载 TOML 配置文件。
- 将配置优先级收敛为：CLI 参数 > `RUNTIME_AGENT_*` 环境变量 > `config.toml` > 默认值。
- 同步 `.env.example`、`config.example.toml`、README、配置指南和 `dev:runtime-agent` 脚本，避免 `RUNTIME_NODE_ID` / `RUNTIME_AGENT_NODE_ID` 等命名漂移。
- 忽略本地真实配置 `apps/runtime-agent/config.toml` 和 `.superteam/` 运行状态目录，保留可提交示例配置。

### Security

#### 配置文件忽略规则收敛 (2026-05-29)

- 扩展 `.gitignore` 环境配置规则，忽略项目内真实 `.env*` 和 `config.yaml` 文件，仅保留 `.env.example`、`config.example.yaml` 等示例配置可同步。

## [0.1.0] - 2026-05-29

### Added
- 项目初始化
- 基础目录结构
- CLAUDE.md 项目文档
