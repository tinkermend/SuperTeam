# Changelog

All notable changes to the SuperTeam Control Plane project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

#### Phase 2.1 - 任务服务 (2026-05-29)

- 实现任务服务核心业务逻辑 (`apps/control-plane/internal/task/`)
  - 领域模型 (`models.go`)：TaskStatus 枚举、Task 领域对象、请求/响应类型
  - 状态机 (`state_machine.go`)：定义和验证任务状态转换规则
    - pending → claimed → running → completed/failed/cancelled
    - 支持 unclaim (claimed → pending)
    - 终态：completed, failed, cancelled
  - 数据访问接口 (`repository.go`)：Repository 接口定义，解耦业务逻辑和数据层
  - 任务服务 (`service.go`)：实现核心业务方法
    - CreateTask：创建任务，默认优先级为 5
    - GetTask：根据 ID 查询任务
    - ListTasks：支持按状态、创建者、Provider 类型过滤
    - UpdateTaskStatus：状态更新，带状态机验证
    - CancelTask：取消任务，记录原因
    - AssignTask：分配任务到 Runtime 节点
  - 状态历史追踪：所有状态变更记录审计日志
- 添加完整的单元测试
  - 服务测试 (`service_test.go`)：Mock repository，测试所有业务方法
  - 状态机测试 (`state_machine_test.go`)：测试所有状态转换规则
  - 测试覆盖率：89.7%
- 添加状态历史 SQL 查询 (`apps/control-plane/internal/storage/queries/tasks.sql`)
  - CreateTaskStateHistory：记录状态变更历史

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
