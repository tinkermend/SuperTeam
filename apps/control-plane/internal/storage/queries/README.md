# Control Plane Storage Layer Tests

## 概述

本目录包含 SuperTeam 控制平面数据层的 sqlc 查询集成测试。测试不再自动启动本机容器，只在显式配置远端或专用测试数据库时运行，避免本地 `go test ./...` 依赖 Docker/Podman。

未配置 `TEST_DATABASE_URL` 和 `TEST_REDIS_URL` 时，本包测试会跳过。确认当前配置的数据库允许迁移和清理测试数据后，也可以设置 `ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1`，让测试复用 `DATABASE_URL` 和 `REDIS_URL`。

## 测试覆盖

### Runtime 节点测试

- `TestCreateRuntimeNode` - 创建 Runtime 节点
- `TestGetRuntimeNode` - 查询节点信息
- `TestUpdateRuntimeNodeHeartbeat` - 更新节点心跳
- `TestListOnlineNodes` - 查询在线节点列表

### 任务测试

- `TestCreateTask` - 创建任务
- `TestGetTask` - 查询任务
- `TestListTasks` - 任务列表查询，支持过滤
- `TestUpdateTaskStatus` - 更新任务状态
- `TestTaskStateTransition` - 任务状态转换
- `TestTaskEvents` - 任务事件流

### 认证测试

- `TestCreateUser` - 创建用户
- `TestGetUserByUsername` - 按用户名查询
- `TestCreateRuntimeToken` - 创建 Runtime token
- `TestValidateRuntimeToken` - 验证 token
- `TestExpiredRuntimeToken` - 过期 token 验证

### 审计测试

- `TestCreateAuditEvent` - 创建审计事件
- `TestListAuditEvents` - 查询审计事件，支持过滤
- `TestCountAuditEvents` - 统计审计事件
- `TestAuditEventsTimeFilter` - 时间范围过滤

## 运行测试

### 前置条件

1. 准备 PostgreSQL 测试库，连接串通过 `TEST_DATABASE_URL` 提供。
2. 准备 Redis 测试实例，连接串通过 `TEST_REDIS_URL` 提供。
3. 确认测试库可以被迁移和清理数据。

> 注意：测试会执行 `internal/storage/migrations/*.sql`，并在部分用例中截断核心业务表。不要把生产库或不可清理的共享库配置到 `TEST_DATABASE_URL`。

### 使用远端测试环境

```bash
cd apps/control-plane
export TEST_DATABASE_URL='postgres://user:password@host:5432/superteam_test?sslmode=disable'
export TEST_REDIS_URL='redis://:password@host:6379/0'
go test ./internal/storage/queries -v -timeout 5m
```

如需使用项目当前配置的开发环境，可从 `doc/database/conn_info.md` 获取连接信息后显式导出为 `TEST_DATABASE_URL` 和 `TEST_REDIS_URL`。只有确认该环境允许迁移和清理测试数据时才应执行。

也可以显式允许测试复用应用配置：

```bash
cd apps/control-plane
export ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1
export DATABASE_URL='postgres://user:password@host:5432/superteam_test?sslmode=disable'
export REDIS_URL='redis://:password@host:6379/0'
go test ./internal/storage/queries -v -timeout 5m
```

## 测试架构

### TestMain 设置

`TestMain` 函数负责：

1. 检查 `TEST_DATABASE_URL` 和 `TEST_REDIS_URL`；或在 `ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1` 时读取 `DATABASE_URL` 和 `REDIS_URL`。
2. 连接并 ping PostgreSQL。
3. 连接并 ping Redis，确保测试环境配置完整。
4. 运行数据库迁移。
5. 创建 queries 实例。
6. 运行所有测试。
7. 关闭数据库连接。

### 测试隔离

每个测试负责：

- 创建测试所需的数据
- 使用 `defer` 清理创建的数据
- 不依赖其他测试的执行顺序

部分审计测试会调用 `cleanupTestData` 截断相关表，以避免统计和列表测试被历史数据污染。

### 数据库迁移

测试使用与应用环境相同的迁移文件（`../migrations/*.sql`），确保 sqlc 查询与当前 schema 保持一致。

## 依赖

- `github.com/jackc/pgx/v5` - PostgreSQL 驱动
- `github.com/redis/go-redis/v9` - Redis 连接校验
- `github.com/stretchr/testify` - 测试断言库

## 故障排查

### 测试被跳过

如果看到以下输出，说明没有显式配置测试环境：

```text
skipping storage query integration tests: set TEST_DATABASE_URL and TEST_REDIS_URL, or set ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 with DATABASE_URL and REDIS_URL
```

导出两个环境变量后重新运行即可。

### PostgreSQL 或 Redis 连接失败

先单独验证连接：

```bash
psql "$TEST_DATABASE_URL" -c 'select current_user, current_database(), current_schema();'
redis-cli -u "$TEST_REDIS_URL" ping
```

### 测试超时

如果远端环境网络较慢，可以增加超时时间：

```bash
go test ./internal/storage/queries -v -timeout 10m
```

## 持续集成

CI 中可以有两种策略：

- 默认不配置 `TEST_DATABASE_URL` 和 `TEST_REDIS_URL`，让本包集成测试跳过。
- 在专用集成测试任务中配置隔离数据库和 Redis，并运行：

```bash
go test ./internal/storage/queries -v -race -timeout 10m
```
