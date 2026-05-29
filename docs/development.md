# SuperTeam 开发指南

本文档介绍如何在本地搭建 SuperTeam 开发环境并运行服务。

## 前置要求

### 必需工具

- **Go** >= 1.22
- **Rust** >= 1.75
- **Node.js** >= 20
- **pnpm** >= 8
- **Docker** 和 **Docker Compose**
- **PostgreSQL Client** (psql)
- **Atlas** - 数据库迁移工具

### 安装工具

```bash
# macOS
brew install go rust node pnpm postgresql ariga/tap/atlas

# 验证安装
go version
cargo --version
node --version
pnpm --version
psql --version
atlas version
```

## 快速开始

### 1. 克隆仓库

```bash
git clone <repository-url>
cd SuperTeam
```

### 2. 启动基础设施

使用 Docker Compose 启动 PostgreSQL、Redis 和 MinIO：

```bash
docker-compose -f docker-compose.dev.yml up -d
```

验证服务状态：

```bash
docker-compose -f docker-compose.dev.yml ps
```

### 3. 配置环境变量

复制环境变量模板：

```bash
cp apps/control-plane/.env.example apps/control-plane/.env
```

编辑 `.env` 文件，使用本地开发配置：

```bash
# 本地开发环境配置
DATABASE_URL=postgres://superteam:superteam@localhost:5432/superteam?sslmode=disable
REDIS_URL=redis://:superteam@localhost:6379/0
S3_ENDPOINT=http://localhost:9000
S3_ACCESS_KEY_ID=minioadmin
S3_SECRET_ACCESS_KEY=minioadmin
S3_BUCKET=superteam-artifacts
```

### 4. 运行数据库迁移

```bash
export DATABASE_URL=postgres://superteam:superteam@localhost:5432/superteam?sslmode=disable
./scripts/db-migrate.sh
```

### 5. 生成 Runtime Token

为 Runtime Agent 生成认证 token：

```bash
./scripts/generate-runtime-token.sh node-001
```

保存输出的 token，后续启动 Runtime Agent 时需要使用。

### 6. 启动 Control Plane

```bash
cd apps/control-plane
make generate
go run ./cmd/control-plane
```

Control Plane 默认监听 `http://localhost:8080`。

验证服务：

```bash
curl http://localhost:8080/health
```

### 7. 启动 Runtime Agent

在新终端中：

```bash
cp apps/runtime-agent/config.example.toml apps/runtime-agent/config.toml
export RUNTIME_AGENT_AUTH_TOKEN=<your-token-from-step-5>
cargo run --manifest-path apps/runtime-agent/Cargo.toml -- \
  --config apps/runtime-agent/config.toml \
  --node-id node-001
```

也可以不设置环境变量，直接通过 CLI 覆盖认证 token：

```bash
cargo run --manifest-path apps/runtime-agent/Cargo.toml -- \
  --config apps/runtime-agent/config.toml \
  --node-id node-001 \
  --auth-token <your-token-from-step-5>
```

说明：

- Runtime Agent 的产品默认行为就是连接 Control Plane 的 daemon 模式。
- 本地 HTTP API 和 `run` 子命令用于诊断、本地 provider 调试和事件回放，不是业务任务分发主链路。

## 开发工作流

### Control Plane 开发

#### 目录结构

```
apps/control-plane/
├── cmd/control-plane/     # 入口
├── internal/
│   ├── api/              # API 层
│   ├── auth/             # 认证服务
│   ├── task/             # 任务服务
│   ├── runtime/          # Runtime 服务
│   ├── audit/            # 审计服务
│   └── storage/          # 数据层
├── migrations/           # 数据库迁移
└── config/              # 配置文件
```

#### 常用命令

```bash
# 编译
make build

# 运行测试
make test

# 生成 sqlc + OpenAPI 代码
make generate

# 仅生成 sqlc 代码
make generate-sqlc

# 运行 linter
make lint

# 格式化代码
make fmt
```

#### 添加新的 API 端点

1. 更新 `contracts/control-plane/openapi.yaml`
2. 重新生成代码：`make generate`
3. 实现 handler：`internal/api/handlers/`
4. 注册路由：`internal/api/router.go`
5. 编写测试

#### 数据库变更

1. 创建新的迁移文件：`migrations/XXX_description.sql`
2. 运行迁移：`./scripts/db-migrate.sh`
3. 更新 sqlc 查询：`internal/storage/queries/*.sql`
4. 重新生成代码：`make generate-sqlc`

### Runtime Agent 开发

#### 目录结构

```
apps/runtime-agent/
├── src/
│   ├── controlplane/     # Control Plane 客户端
│   ├── lease/           # 租约管理
│   ├── providers/       # Provider 适配器
│   └── daemon.rs        # 主入口
└── tests/               # 集成测试
```

#### 常用命令

```bash
# 编译
cargo build

# 运行测试
cargo test

# 运行 linter
cargo clippy

# 格式化代码
cargo fmt

# 启动受 Control Plane 管理的 daemon
cargo run --manifest-path apps/runtime-agent/Cargo.toml -- --config apps/runtime-agent/config.toml

# 仅做本地 provider 诊断
cargo run --manifest-path apps/runtime-agent/Cargo.toml -- run --provider claude --workspace /abs/path --prompt "hello"
```

#### 添加新的 Provider

1. 在 `src/providers/` 创建新模块
2. 实现 `Provider` trait
3. 在 `daemon.rs` 注册 provider
4. 编写测试

### Web 控制台开发

```bash
cd apps/web
pnpm install
pnpm dev
```

访问 `http://localhost:3000`

## 测试

### 基线验证

```bash
pnpm install
cd apps/control-plane && make generate
go test ./apps/control-plane/...
cargo test --manifest-path apps/runtime-agent/Cargo.toml
pnpm -r --if-present test
```

说明：

- `go test ./apps/control-plane/...` 在当前环境里如果失败，常见原因是 storage/query 测试依赖 testcontainers，而宿主机缺少 rootless Docker，错误通常表现为 `rootless Docker not found, failed to create Docker provider`。
- 这种情况下，先保留失败输出，再补跑不依赖 Docker 的定向验证，例如：

```bash
go test ./apps/control-plane/internal/api ./apps/control-plane/internal/task -count=1
go test -c -o /tmp/superteam-control-plane-queries.test ./apps/control-plane/internal/storage/queries
```

### 集成测试与本地联调

```bash
# 启动所有服务
docker-compose -f docker-compose.dev.yml up -d
./scripts/db-migrate.sh

# 启动 Control Plane
cd apps/control-plane
make generate
go run ./cmd/control-plane &

# 启动 Runtime Agent
RUNTIME_AGENT_AUTH_TOKEN=<token> \
cargo run --manifest-path apps/runtime-agent/Cargo.toml -- \
  --config apps/runtime-agent/config.toml \
  --node-id test-node &

# 运行端到端测试
./scripts/e2e-test.sh
```

### 端到端测试流程

1. 创建任务

```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Test Task",
    "provider_type": "claude-code",
    "params": {
      "prompt": "echo hello world"
    }
  }'
```

2. 查看任务状态

```bash
curl http://localhost:8080/api/v1/tasks/<task-id>
```

3. 查看 Runtime Agent 日志

```bash
# 查看任务执行日志
tail -f apps/runtime-agent/logs/node-001.log
```

## 故障排查

### Control Plane 无法启动

1. 检查数据库连接：

```bash
psql $DATABASE_URL -c "SELECT 1"
```

2. 检查 Redis 连接：

```bash
redis-cli -u $REDIS_URL ping
```

3. 查看日志：

```bash
tail -f apps/control-plane/logs/control-plane.log
```

### Runtime Agent 无法连接

1. 验证 token：

```bash
psql $DATABASE_URL -c "SELECT name, created_at FROM auth_runtime_tokens"
```

2. 检查网络连接：

```bash
curl http://localhost:8080/health
```

3. 查看 Runtime Agent 日志

### 数据库迁移失败

1. 检查迁移状态：

```bash
cd apps/control-plane
atlas migrate status --env local
```

2. 回滚迁移：

```bash
atlas migrate down --env local
```

3. 重新运行迁移：

```bash
./scripts/db-migrate.sh
```

## 环境变量参考

### Control Plane

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `CONTROL_PLANE_ADDR` | HTTP 监听地址 | `:8080` |
| `DATABASE_URL` | PostgreSQL 连接字符串 | 必需 |
| `REDIS_URL` | Redis 连接字符串 | 必需 |
| `S3_ENDPOINT` | S3 端点 | 必需 |
| `S3_BUCKET` | S3 bucket 名称 | `superteam-artifacts` |
| `S3_ACCESS_KEY_ID` | S3 access key | 必需 |
| `S3_SECRET_ACCESS_KEY` | S3 secret key | 必需 |
| `LOG_LEVEL` | 日志级别 | `info` |

### Runtime Agent

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `NODE_ID` | 节点 ID | 必需 |
| `CONTROL_PLANE_URL` | Control Plane URL | 必需 |
| `TOKEN` | 认证 token | 必需 |
| `MAX_CONCURRENT_TASKS` | 最大并发任务数 | `4` |
| `LOG_LEVEL` | 日志级别 | `info` |

## 生产部署

参考 `docs/deployment.md`（待补充）

## 贡献指南

1. Fork 仓库
2. 创建功能分支：`git checkout -b feature/my-feature`
3. 提交变更：`git commit -m "feat: add my feature"`
4. 推送分支：`git push origin feature/my-feature`
5. 创建 Pull Request

### Commit 规范

遵循 [Conventional Commits](https://www.conventionalcommits.org/)：

- `feat:` 新功能
- `fix:` Bug 修复
- `docs:` 文档变更
- `test:` 测试相关
- `refactor:` 重构
- `chore:` 构建/工具变更

## 相关文档

- [API 文档](./api.md)
- [架构设计](../CLAUDE.md)
- [数据库 Schema](./database/schema.md)
- [部署指南](./deployment.md)
