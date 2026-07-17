DO NOT send optional commentary. Keep only necessary facts, blockers, verification evidence, risks, and actionable recommendations that directly affect the user's next decision or task outcome.

## 项目定位

SuperTeam 把 AI 执行能力、流程调度、人类审批、上下文、工件和审计纳入统一控制平面。

- **数字员工**是 agent 型业务身份，围绕角色、任务边界、权限、上下文策略和输出契约建模，只作为项目内可调度的执行者。
- **人类**是一等的管理、审批、决策和验收参与者，不归入数字员工。不要把人类管理职责建模成数字员工能力，也不要让数字员工绕过人类决策。
- **Project** 是面向具体目标或问题场景的业务闭环容器（不限于软件交付），聚合目标、负责人、虚拟协调线程、任务、证据、预算、审批和验收结论。流程编排只是驱动项目运行的模板，不替代 Project 作为业务事实入口。核心模型不定义封闭的项目类型枚举，场景差异通过场景模板、Workflow Template、项目画像、标签、Policy 和服务端注册校验表达。

## 架构分层

| 层 | 目录 | 职责 | 禁止 |
|---|---|---|---|
| Console | `apps/web/` | 管理、观察、审批、验收入口 | 本机执行能力、业务事实源、长期业务状态 |
| Control Plane | `apps/control-plane/` | 业务状态、任务、审批、审计、流程调度、上下文、工件、Runtime 与能力注册；对 Console 和 Runtime 提供 API | 直接执行本地命令 |
| Runtime | `apps/runtime-agent/` | 领任务、维护租约、管理本机 Provider 进程/会话、工作目录、日志、工件、执行槽位 | 控制台 UI、业务策略、长期业务状态 |
| Provider | Claude Code / OpenCode / Codex | 在 Runtime Agent 管理下执行 | 承载平台级状态 |
| Capability | `apps/control-plane/internal/capability/` | 外部能力与 MCP server 的注册、绑定、凭据加密、授权和审计；HTTP 与 MCP transport 调用 | 硬编码客户专属逻辑 |
| Authorization | `apps/control-plane/internal/authz/`、`internal/authzcenter/` | 关系型授权决策点、runtime scope、成员与决策记录；后端为 OpenFGA（外部服务，独立启停） | 在业务代码里绕过决策点自行判权 |

- API 契约放在 `contracts/`；修改契约后必须走生成与契约验证流程。
- Provider 协议必须语言无关，用结构化 schema 描述输入、事件、结果、工件和错误；Rust 只是一种 adapter 实现语言。接入优先走统一 `provider` contract，协议不完整时再用 CLI、stdio、JSON stream、PTY 或 HTTP adapter 兜底。
- Provider 类型和外部能力类型不要在业务核心里依赖封闭枚举，以注册表和服务端校验为准。
- 客户差异不进核心流程代码，放入 Tenant Profile、Connector、Semantic Mapping、Capability 配置和 Policy。

### 已知债（现状与本章规范不符，不得据现状反推规范）

- `contracts/provider/` 目前只有散文 README，没有机器可读 schema；事实上的 Provider 协议活在 `apps/runtime-agent/src/providers/` 的 Rust 类型里。这与"协议必须语言无关"相悖，属待偿债务，不是可依赖的先例。
- 契约验证只覆盖 `contracts/control-plane/openapi.yaml`（见 `scripts/verify-foundation-contracts.mjs`）；`contracts/runtime/openapi.yaml` 与 `contracts/provider/` 未纳入。改这两处契约时，"走契约验证流程"目前无自动化可依，需人工核对下游生成物。

## 协作模型

- 一个 Project 绑定一个虚拟协调线程，由 Temporal Workflow 承载（WorkflowID = `project-coordinator:{project_id}`）。它是项目内置的独占协调状态机，不是数字员工实体，不出现在数字员工列表中；通过 Signal 接收事件、串行处理协调决策、并发分派执行任务，所有协调动作必须产出结构化的 RouteDecision、ProjectTask 和审计记录。
- 每个项目必须绑定固定人类负责人（human_owner）。项目人类成员同等身份，不划分 leader/验收人等子角色；human_owner 是必绑锚点与决策兜底路由目标。人类成员负责最终业务判断、审批、结果验收、驳回、补证要求、汇报接收和验收结论。
- Agent 之间不直接自由聊天，通过结构化对象协作；每个阶段必须产出可持久化的工件、证据、决策或交接包。
- 全局上下文由控制平面持久化；执行时只注入当前任务需要的上下文切片；关键结论必须结构化回写。

## 工程约定

- 技术栈以当前 workspace、契约和构建脚本为准；不得在没有明确共识时引入替代主栈的并行框架或重复基础设施。根级命令以 `package.json` 为准，优先通过 `corepack pnpm <script>` 运行；不要记录未在仓库脚本、Makefile 或 helper script 中确认过的命令。
- 验证一律走仓库已有的 `verify:*` 脚本，不要手拼等价命令：`verify:foundation`（契约 + TS/Go/Rust 全量）、`verify:web`、`verify:control-plane`、`verify:runtime-agent`、`verify:db`、`verify:contracts`、`verify:design-system`、`verify:design-prototypes`。契约代码生成用 `generate:control-plane`。
- 启停用 `scripts/dev-services.sh start|status|restart|stop`；默认 `all` 含 Temporal、Control Plane、Web、Runtime Agent，OpenFGA 需单独管理。联调前后先 `status` 确认实际状态，代码变更后优先定向 `restart <service>`（脚本只管理自己写入 pid 文件的进程）。`start|restart control-plane` 会先自动执行 Atlas 迁移，仅在明确需要时用 `SUPERTEAM_DEV_SKIP_MIGRATIONS=1` 跳过。
- 数据库表设计、字段类型、UUID-first、租户/团队、索引、迁移、sqlc 与 OpenAPI 规则统一遵循 `DATABASE_DESIGN.md`。生产迁移唯一目录是 `apps/control-plane/internal/storage/migrations/`；变更后必须更新 `atlas.sum`，并用 `make -C apps/control-plane migrate-validate` 校验（本地非 Docker dev 库可覆盖 `DEV_URL`）。
- 代码发现优先使用 codebase-memory-mcp：`search_graph` 查符号、`trace_path` 追调用、`get_code_snippet` 读实现，复杂模式用 `query_graph` / `get_architecture`；工具不可用、结果不足或搜索字符串/配置/非代码文件时才回退 `rg` / 文件读取。
- 前端页面、布局或样式变更前必须阅读 `DESIGN.md`；改设计系统或原型后跑 `verify:design-system` / `verify:design-prototypes`。Web 测试走 `corepack pnpm verify:web`（只跑测试时 `corepack pnpm --filter @superteam/web test`），禁止 `npx playwright install` 或 `npx vitest run`。Web 内部跳转必须用 TanStack Router 的 `Link` 或 `navigate`；只有外链、下载、同页锚点或明确需要整页刷新才允许原生 `<a href>` / `window.location`。
- 不要盲目猜测；存在无法从本地上下文确认且影响架构或业务判断的不确定点时，先与人类沟通。

## 验证与收尾

- **真实端到端验证是默认完成条件**，适用于功能、修复、合并、前后端联调、Runtime/Provider 接入、数据库/迁移变更，以及任何声称“功能可用”的任务。必须让当前代码通过真实 Web、Control Plane、数据库、Runtime、Provider 路径运行；不得把 mock、组件测试、单元测试、构建通过或代码审查表述为真实链路已验证。前后端变更需确认运行中的服务已加载当前代码，并通过浏览器或 curl 走真实接口确认结果不是 mock、缓存或旧服务；Runtime/Provider 变更需至少一次真实 smoke。Web 仿真测试用 codex chrome plug。`verify:*` 脚本是提交前的分层门禁，通过它们不等于完成了端到端验证。
- **无法验证时标记为阻塞**并说明缺失依赖（服务未启动、认证缺失、Provider 不可用、迁移未准备、环境不安全），不得以“未做真实链路验证”的状态交付，除非人类明确把范围限定为纯单层局部验证。
- **轻量验证例外**：纯文案替换、单个低风险样式对齐、设计规范/宪法补充等不改变交互、数据提交、路由、接口、状态流、权限、持久化或运行链路的局部变更，只需 `rg` 回查、`git diff --check`，必要时跑受影响的定向测试；不重启服务、不跑全量测试、不做端到端验证。
- 任务收尾前使用项目内 skill `$superteam-completion-check`（`.codex/skills/superteam-completion-check/SKILL.md`）。
- 分支收尾：合并到 `main` 后，基于 `main` 当前代码完成端到端真实验证，通过后再删除分支和 worktree。验证阻塞时不得删除分支或声明完成。
