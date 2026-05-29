# Control Plane Storage Layer Tests

## 概述

本目录包含 SuperTeam 控制平面数据层的完整测试套件。测试使用 testcontainers-go 在隔离的 PostgreSQL 容器中运行，确保测试环境的一致性和可重复性。

## 测试覆盖

### Runtime 节点测试
- `TestCreateRuntimeNode` - 创建 Runtime 节点
- `TestGetRuntimeNode` - 查询节点信息
- `TestUpdateRuntimeNodeHeartbeat` - 更新节点心跳
- `TestListOnlineNodes` - 查询在线节点列表

### 任务测试
- `TestCreateTask` - 创建任务
- `TestGetTask` - 查询任务
- `TestListTasks` - 任务列表查询（支持过滤）
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
- `TestListAuditEvents` - 查询审计事件（支持过滤）
- `TestCountAuditEvents` - 统计审计事件
- `TestAuditEventsTimeFilter` - 时间范围过滤

## 运行测试

### 前置条件

1. 安装 Docker 或 Podman
2. 确保 Docker/Podman daemon 正在运行

### 使用测试脚本（推荐）

```bash
cd apps/control-plane
./test.sh
```

测试脚本会自动检测 Podman socket 位置并设置正确的环境变量。

### 手动运行

#### 使用 Docker

```bash
cd apps/control-plane
go test ./internal/storage/queries -v -timeout 5m
```

#### 使用 Podman

```bash
cd apps/control-plane
export DOCKER_HOST=unix:///var/folders/.../podman/podman-machine-default-api.sock
export TESTCONTAINERS_RYUK_DISABLED=true
go test ./internal/storage/queries -v -timeout 5m
```

## 测试架构

### TestMain 设置

`TestMain` 函数负责：
1. 启动 PostgreSQL 16 容器
2. 建立数据库连接
3. 运行数据库迁移（`001_initial.sql`）
4. 创建 queries 实例
5. 运行所有测试
6. 清理资源

### 测试隔离

每个测试负责：
- 创建测试所需的数据
- 使用 `defer` 清理创建的数据
- 不依赖其他测试的执行顺序

### 数据库迁移

测试使用与生产环境相同的迁移文件（`../migrations/001_initial.sql`），确保测试环境与生产环境的一致性。

## 依赖

- `github.com/testcontainers/testcontainers-go` - 容器化测试环境
- `github.com/testcontainers/testcontainers-go/modules/postgres` - PostgreSQL 模块
- `github.com/stretchr/testify` - 测试断言库
- `github.com/jackc/pgx/v5` - PostgreSQL 驱动

## 故障排查

### Docker/Podman 连接问题

如果遇到 "rootless Docker not found" 错误：

1. 确认 Docker/Podman 正在运行：
   ```bash
   docker ps
   # 或
   podman ps
   ```

2. 对于 Podman，找到 socket 位置：
   ```bash
   ls /var/folders/*/T/podman/podman-machine-default-api.sock
   ```

3. 设置环境变量：
   ```bash
   export DOCKER_HOST=unix:///path/to/podman.sock
   export TESTCONTAINERS_RYUK_DISABLED=true
   ```

### 测试超时

如果测试超时，可以增加超时时间：
```bash
go test ./internal/storage/queries -v -timeout 10m
```

### 容器清理

如果测试中断导致容器未清理，手动清理：
```bash
docker ps -a | grep postgres:16-alpine
docker rm -f <container_id>
```

## 持续集成

在 CI 环境中运行测试时，确保：
1. CI 环境有 Docker 访问权限
2. 设置足够的超时时间（建议 10 分钟）
3. 使用 `-race` 标志检测竞态条件：
   ```bash
   go test ./internal/storage/queries -v -race -timeout 10m
   ```

## 下一步

- [ ] 添加并发测试
- [ ] 添加性能基准测试
- [ ] 添加数据库事务测试
- [ ] 集成到 CI/CD 流水线
