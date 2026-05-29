# Changelog

## 2026-05-29

- 清理 `DESIGN.md` 中偏项目化的设计描述，改为通用企业控制台设计风格规范。
- 补充状态与反馈规则，覆盖 loading、empty、error、权限不足、禁用态和长任务执行反馈。
- 为 `DESIGN.md` 补充基础组件风格规则，覆盖字体层级、间距密度、按钮、表单、面板、标签、表格、浮层和图标。
- 在 `AGENTS.md` 中补充 Web 开发设计风格引用，要求后续页面、组件和视觉改造参考 `DESIGN.md`。

## 2026-05-28

- 为 Rust Runtime Agent 增加 HTTP/WS 执行宿主能力：新增 `/health`、`/providers`、`/runs`、`/runs/{id}`、`/runs/{id}/events`、`/runs/{id}/cancel` 和 `/ws`。
- 新增 Runtime run registry，记录平台 run、Provider session id、事件序号、状态和本机 `events.jsonl` 日志，支持事件回放和取消 active Provider 子进程。
- 新增 Provider health probe，支持探测 Claude Code/OpenCode binary 与 `--version`，并通过结构化 JSON 暴露可用性与错误。
- 将 `apps/runtime-agent` 迁移为 Rust/Tokio 可执行 crate，新增 `runtime-agent run` Provider 调用入口，支持 Claude Code/OpenCode CLI JSON stream 事件归一化。
- 为 Runtime Agent 增加 Rust 测试覆盖：Provider 命令构造、事件解析、fake CLI 流式输出、非零退出 stderr 暴露，以及 CLI JSONL 输出。
- 同步仓库脚本与 Go workspace：`pnpm dev:runtime-agent` 改走 Cargo，`pnpm test` 增加 `test:rust`，Go 测试只覆盖 Control Plane。
- 更新 Runtime/Provider 契约说明，明确 Rust Runtime Agent 与语言无关 Provider contract 的边界。
- 更新 `AGENTS.md` 的 Runtime Agent 方案：Control Plane 继续 Go，Runtime Agent 改为 Rust/Tokio 节点执行宿主，Provider contract 保持语言无关；Claude/OpenCode adapter 第一版参考 `desktop-cc-gui`，Runtime 外部 HTTP/WS contract 参考 `AionUi`。
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
