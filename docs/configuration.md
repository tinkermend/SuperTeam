# SuperTeam 配置管理指南

## 配置文件结构

```
SuperTeam/
├── .env.example                          # 全局配置模板
├── apps/
│   ├── control-plane/
│   │   ├── config/
│   │   │   └── config.example.yaml       # YAML 配置模板
│   │   └── .env.example                  # 环境变量模板
│   ├── web/
│   │   ├── .env.example                  # 环境变量模板
│   │   ├── .env.development              # 开发环境默认值
│   │   └── .env.production               # 生产环境默认值
│   └── runtime-agent/
│       ├── config.example.toml           # TOML 配置模板
│       └── .env.example                  # 环境变量模板
```

## 配置优先级

所有应用遵循统一的配置优先级（从高到低）：

1. **环境变量** - 最高优先级，用于生产部署和 CI/CD
2. **配置文件** - 本地开发和默认值
3. **代码默认值** - 兜底配置

## 统一的环境变量命名规范

### 命名原则

- 使用 `大写字母 + 下划线` 格式
- 按模块分组，使用统一前缀
- 避免同一概念使用不同名称

### 核心变量命名表

| 变量名 | 用途 | 使用方 | 示例值 |
|--------|------|--------|--------|
| `CONTROL_PLANE_ADDR` | Control Plane HTTP 监听地址 | control-plane | `:8080` |
| `DATABASE_URL` | PostgreSQL 连接串 | control-plane | `postgres://user:pass@host:5432/db` |
| `REDIS_URL` | Redis 连接串 | control-plane | `redis://:pass@host:6379/0` |
| `S3_ENDPOINT` | S3 兼容存储端点 | control-plane | `http://localhost:9000` |
| `S3_REGION` | S3 区域 | control-plane | `us-east-1` |
| `S3_BUCKET` | S3 存储桶名称 | control-plane | `superteam-artifacts` |
| `S3_ACCESS_KEY_ID` | S3 访问密钥 ID | control-plane | `minioadmin` |
| `S3_SECRET_ACCESS_KEY` | S3 访问密钥 | control-plane | `minioadmin` |
| `S3_FORCE_PATH_STYLE` | S3 强制路径风格 | control-plane | `true` |

### Web Console 变量（前缀：`NEXT_PUBLIC_`）

| 变量名 | 用途 | 示例值 |
|--------|------|--------|
| `NEXT_PUBLIC_CONTROL_PLANE_URL` | Control Plane API 地址（浏览器访问） | `http://localhost:8080` |
| `NEXT_PUBLIC_WS_URL` | WebSocket 地址 | `ws://localhost:8080/ws` |
| `NEXT_PUBLIC_ENABLE_MOCK` | 是否启用 Mock 数据 | `false` |
| `NEXT_PUBLIC_LOG_LEVEL` | 前端日志级别 | `info` |

> **注意**：Next.js 的 `NEXT_PUBLIC_` 前缀变量会暴露到浏览器，不要存放敏感信息。

### Runtime Agent 变量（前缀：`RUNTIME_AGENT_`）

| 变量名 | 用途 | 示例值 |
|--------|------|--------|
| `RUNTIME_AGENT_NODE_ID` | 节点 ID（留空自动生成） | `node-001` |
| `RUNTIME_AGENT_CONTROL_PLANE_URL` | Control Plane API 地址 | `http://localhost:8080` |
| `RUNTIME_AGENT_HEARTBEAT_INTERVAL` | 心跳间隔（秒） | `30` |
| `RUNTIME_AGENT_MAX_CONCURRENT_TASKS` | 最大并发任务数 | `3` |
| `RUNTIME_AGENT_WORKSPACE_DIR` | 工作目录基础路径 | `/var/superteam/workspaces` |
| `RUNTIME_AGENT_CLEANUP_POLICY` | 清理策略 | `on_success` |
| `RUNTIME_AGENT_PROVIDER_CLAUDE_CODE_ENABLED` | 启用 Claude Code | `true` |
| `RUNTIME_AGENT_PROVIDER_CLAUDE_CODE_BINARY` | Claude Code 二进制路径 | `/usr/local/bin/claude` |
| `RUNTIME_AGENT_PROVIDER_OPENCODE_ENABLED` | 启用 OpenCode | `false` |
| `RUNTIME_AGENT_PROVIDER_OPENCODE_BINARY` | OpenCode 二进制路径 | `/usr/local/bin/opencode` |
| `RUNTIME_AGENT_LOG_LEVEL` | 日志级别 | `info` |

### 共享变量

| 变量名 | 用途 | 示例值 |
|--------|------|--------|
| `SUPERTEAM_ENV` | 环境标识 | `development` / `staging` / `production` |
| `LOG_LEVEL` | 后端日志级别 | `debug` / `info` / `warn` / `error` |

## 本地开发配置

### 1. Control Plane

```bash
cd apps/control-plane

# 方式一：使用 YAML 配置文件
cp config/config.example.yaml config/config.yaml
# 编辑 config.yaml 填入实际值

# 方式二：使用环境变量（优先级更高）
cp .env.example .env.local
# 编辑 .env.local 填入实际值
```

### 2. Web Console

```bash
cd apps/web

# 创建本地配置
cp .env.example .env.local
# 编辑 .env.local 填入实际值

# 开发环境会自动加载 .env.development
npm run dev
```

### 3. Runtime Agent

```bash
cd apps/runtime-agent

# 方式一：使用 TOML 配置文件
cp config.example.toml config.toml
# 编辑 config.toml 填入实际值

# 方式二：使用环境变量（优先级更高）
cp .env.example .env
# 编辑 .env 填入实际值
```

## 生产部署配置

### 推荐方式：环境变量

生产环境建议通过环境变量注入配置，不提交配置文件到代码仓库。

```bash
# Kubernetes ConfigMap/Secret
kubectl create configmap superteam-config \
  --from-literal=DATABASE_URL="postgres://..." \
  --from-literal=REDIS_URL="redis://..."

# Docker Compose
docker-compose up -d \
  -e DATABASE_URL="postgres://..." \
  -e REDIS_URL="redis://..."

# Systemd Service
Environment="DATABASE_URL=postgres://..."
Environment="REDIS_URL=redis://..."
```

### 配置文件方式

如果使用配置文件，确保：
1. 配置文件不提交到 Git（已在 `.gitignore` 中）
2. 通过配置管理工具（Ansible、Terraform）部署
3. 敏感信息使用密钥管理服务（Vault、AWS Secrets Manager）

## 配置校验

### Control Plane

```bash
cd apps/control-plane
go run cmd/control-plane/main.go --config config/config.yaml --validate
```

### Runtime Agent

```bash
cd apps/runtime-agent
cargo run -- --config config.toml --validate
```

## 安全注意事项

### 敏感信息保护

1. **不要提交敏感信息到 Git**
   - `.env.local`、`config.yaml`、`config.toml` 已在 `.gitignore` 中
   - 只提交 `.example` 模板文件

2. **生产环境密钥管理**
   - 使用密钥管理服务（Vault、AWS Secrets Manager、Azure Key Vault）
   - 定期轮换密钥
   - 最小权限原则

3. **开发环境隔离**
   - 开发环境使用独立的数据库和存储
   - 不要在开发环境使用生产密钥

### 配置文件权限

```bash
# 限制配置文件权限
chmod 600 config/config.yaml
chmod 600 .env.local
chmod 600 config.toml
```

## 常见问题

### Q: 为什么有些变量有前缀，有些没有？

A: 
- `CONTROL_PLANE_*` - Control Plane 专用配置
- `RUNTIME_AGENT_*` - Runtime Agent 专用配置
- `NEXT_PUBLIC_*` - Next.js 要求的浏览器可见变量前缀
- `DATABASE_URL`、`REDIS_URL` - 业界通用命名，无前缀
- `S3_*` - AWS SDK 标准命名，无前缀

### Q: 环境变量和配置文件冲突时怎么办？

A: 环境变量优先级更高，会覆盖配置文件的值。

### Q: 如何在 CI/CD 中使用配置？

A: 通过环境变量注入，不要在 CI/CD 中使用配置文件。

```yaml
# GitHub Actions 示例
env:
  DATABASE_URL: ${{ secrets.DATABASE_URL }}
  REDIS_URL: ${{ secrets.REDIS_URL }}
```

### Q: 如何验证配置是否正确？

A: 每个应用启动时会自动校验必需配置项，缺失时会报错并退出。

## 配置变更记录

| 日期 | 变更内容 | 影响范围 |
|------|----------|----------|
| 2026-05-29 | 统一配置管理和命名规范 | 全局 |
