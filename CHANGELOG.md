# Changelog

## 2026-05-28

- 更新 `AGENTS.md` 的 Runtime Agent 方案：从 Go daemon 调整为 Rust/Tokio 节点执行宿主，并同步 `apps/runtime-agent` Cargo crate 目录边界。
- 将 Superpowers skills 安装到项目级 `.CodeX/skills` 目录，便于当前项目本地使用。
- 为 Control Plane 增加 YAML 配置文件加载能力，支持 `--config` 指定配置文件并保留环境变量覆盖。
- 新增 `apps/control-plane/config/config.example.yaml`，并忽略本地真实配置 `apps/*/config/local.yaml` 与 `*.local.yaml`。
- 建立 Control Plane 领域包骨架：`task`、`workflow`、`approval`、`artifact`、`audit`、`runtime`、`storage`、`config`。
- 新增 Control Plane 配置加载与 PostgreSQL、Redis、S3 兼容对象存储客户端封装。
- 新增 Control Plane OpenAPI 的 `oapi-codegen` Go server 生成配置和 `pnpm generate:control-plane` 脚本。
- 初始化本地 SuperTeam 数据库连接信息：启动带随机密码的 `superteam-redis`，并在现有 PostgreSQL 实例中创建 `superteam` 数据库、用户和 schema。
- 新增 `docs/database/conn_info.md` 记录本地 PostgreSQL 与 Redis 连接参数和验证命令。

## 2026-05-27

- 初始化 Web 侧 shadcn/ui Radix Nova 配置，新增 Button、Card、Input、Avatar、Badge、Tooltip、Separator 等标准组件。
- 基于参考图搭建 Web 控制台外部系统骨架：左侧主导航、顶部搜索与操作区、首页开发中空状态和健康状态侧栏。
- 增加 Web 首页骨架回归测试，并为 Vitest 补充 `@/*` 路径别名配置。
- 引入 shadcn skill 到项目级 Codex 配置目录，并保留 `skills` 安装器生成的项目级 `.agents` skill。
- 新增项目级 Codex MCP 配置：接入 shadcn MCP server。
- 建立 monorepo 基线：`apps/web`、`apps/desktop`、`apps/control-plane`、`apps/runtime-agent`、`packages/*`、`contracts/*`。
- 增加 Control Plane `/health` 最小实现、Runtime Agent daemon runner、Web 控制台首页骨架。
- 增加 Control Plane 与 Runtime OpenAPI 基线，以及 Provider 契约说明。
- 增加 TS/Go 测试、类型检查和 Web 构建脚本。
- 将 Web dev server 固定为 webpack 模式，并默认绑定 `127.0.0.1:3000`，规避当前 Next 16 Turbopack 在本地 workspace 下的 dev panic 和参数转发问题。
