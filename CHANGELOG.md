# Changelog

All notable changes to the SuperTeam Control Plane project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

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

### Deprecated

### Removed

### Fixed

### Security

## [0.1.0] - 2026-05-29

### Added
- 项目初始化
- 基础目录结构
- CLAUDE.md 项目文档
