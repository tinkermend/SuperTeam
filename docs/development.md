# 开发指南

## 本地开发

### Web 控制台

Web 控制台位于 `apps/web`，使用 Vite、React、TanStack Router、TanStack Query 和 shadcn/ui。

本地启动：

```bash
pnpm dev:web
```

浏览器环境变量使用 `VITE_CONTROL_PLANE_URL` 指向 Control Plane，例如：

```dotenv
VITE_CONTROL_PLANE_URL=http://localhost:8080
```

验证：

```bash
pnpm verify:web
```

### Control Plane

Control Plane 本地开发使用被 Git 忽略的 `apps/control-plane/config/config.yaml`。可从 `apps/control-plane/config/config.example.yaml` 复制后填入本地 PostgreSQL、Redis 和 S3 兼容存储配置。

本地启动：

```bash
pnpm dev:control-plane
```

如需临时指定其他配置文件：

```bash
pnpm dev:control-plane -- --config apps/control-plane/config/config.yaml
```

环境变量仅作为部署平台或临时运行覆盖层使用，优先级高于 YAML；Control Plane 不再维护 `.env.example` 作为本地开发入口。

### Runtime Agent

Runtime Agent 本地开发使用被 Git 忽略的 `apps/runtime-agent/config.yaml`。可从 `apps/runtime-agent/config.example.yaml` 复制后填入本机 node_id、runtime token、Provider 路径和工作目录配置。

本地启动：

```bash
pnpm dev:runtime-agent
```

如需临时指定其他配置文件：

```bash
cargo run --manifest-path apps/runtime-agent/Cargo.toml -- --config apps/runtime-agent/config.yaml
```

Runtime Agent 与 Control Plane 统一使用 YAML 作为本地配置文件格式。环境变量 `RUNTIME_AGENT_*` 仅作为部署平台或临时运行覆盖层使用，不再维护 `.env.example`。
