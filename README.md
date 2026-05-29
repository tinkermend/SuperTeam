# SuperTeam

SuperTeam 是企业级数字员工控制平面。第一阶段优先建立外部 Web 控制台、Control Plane、Runtime Agent 和契约基线。

## 工程结构

```text
apps/
  web/             # Next.js Web 控制台入口
  desktop/         # 桌面端占位；第一阶段不做业务适配
  control-plane/   # Go Control Plane 服务
  runtime-agent/   # Rust Runtime Agent daemon
packages/
  ui/              # 纯 UI 组件
  views/           # Web 优先的共享业务视图
  core/            # 领域状态和组合逻辑
  api-client/      # API 客户端边界
contracts/
  control-plane/   # Console <-> Control Plane OpenAPI
  runtime/         # Runtime Agent <-> Control Plane OpenAPI
  provider/        # Runtime Agent <-> Provider 契约说明
```

## 本地命令

```bash
pnpm install
pnpm test:ts
pnpm typecheck
go test ./apps/control-plane/...
cargo test --manifest-path apps/runtime-agent/Cargo.toml
pnpm build:web
pnpm dev:web
pnpm dev:control-plane
pnpm dev:runtime-agent
```

## 当前基线

- `apps/control-plane` 提供 `GET /health`，并由 `contracts/control-plane/openapi.yaml` 描述。
- `apps/runtime-agent` 提供 Rust daemon runner，可通过 `--config apps/runtime-agent/config.example.toml` 加载可见配置；本地真实配置可复制为 `apps/runtime-agent/config.toml`，该文件不会提交。
- `apps/web` 使用 Next.js App Router，组合 `packages/views` 与 `packages/core`。
- `apps/desktop` 只保留空壳边界，待 Web 主链路完整后再实现。
