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
cd apps/control-plane && make generate
go test ./apps/control-plane/...
cargo test --manifest-path apps/runtime-agent/Cargo.toml
pnpm -r --if-present test
pnpm -r --if-present typecheck
pnpm dev:control-plane
pnpm dev:web
cargo run --manifest-path apps/runtime-agent/Cargo.toml -- --config apps/runtime-agent/config.toml
```

## 当前基线

- `apps/control-plane` 通过统一启动入口装配存储、服务、handlers 和 API routes，并提供完整产品 API 与 `GET /health`。
- `apps/runtime-agent` 默认作为受 Control Plane 管理的 daemon 运行；本地 provider `run` 子命令仅用于诊断，不是业务任务分发入口。
- `contracts/control-plane/openapi.yaml` 描述 Console 与 Runtime Agent 调用的 Control Plane API。
- `apps/runtime-agent` 支持通过 `--config apps/runtime-agent/config.toml` 加载本地可见配置；示例文件为 `apps/runtime-agent/config.example.toml`。
- `apps/web` 当前重点是 API client / core 数据边界，使用 Next.js App Router 组合 `packages/views` 与 `packages/core`。
- `apps/desktop` 只保留空壳边界，待 Web 主链路完整后再实现。
