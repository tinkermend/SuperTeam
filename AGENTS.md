# SuperTeam 工程宪法

面向实现与协作的**硬规范**。进度、设计论证、操作手册不写在此——见对应设计稿、`CHANGELOG.md`、`docs/`。

## 项目定位

统一控制平面：AI 执行、流程调度、人类审批、上下文、工件与审计。

- **数字员工**：可调度的 agent 业务身份（角色、边界、权限、上下文策略、输出契约）。运行落点在派发时解析（项目 Runtime placement + 员工 Provider），员工本身不绑固定 Runtime/执行实例。
- **人类**：一等参与者（管理、审批、决策、验收）；不得把人类职责建模成数字员工，也不得让数字员工绕过人类决策。
- **Project**：面向目标/问题场景的业务闭环容器；流程模板只驱动运行，不替代 Project 作为业务事实入口。场景差异用模板、画像、标签、Policy 与服务端校验表达，**不**在核心模型做封闭项目类型枚举。

## 架构分层

| 层 | 目录 | 职责 | 禁止 |
|---|---|---|---|
| Console | `apps/web/` | 管理、观察、审批、验收 | 本机执行、业务事实源、长期业务状态 |
| Control Plane | `apps/control-plane/` | 业务状态、任务、审批、审计、调度、上下文、工件、Runtime/能力注册 | 直接执行本地命令 |
| Runtime | `apps/runtime-agent/` | 领任务、租约、Provider 进程/会话、工作目录、日志、工件 | 控制台 UI、业务策略、长期业务状态 |
| Provider | Claude Code / OpenCode / Codex 等 | 在 Runtime 管理下执行 | 承载平台级状态 |
| Capability | `apps/control-plane/internal/capability/` | 外部能力与 MCP 的注册、绑定、凭据、授权、调用 | 硬编码客户专属逻辑 |
| Authorization | `internal/authz/`、`authzcenter/` | 关系型授权（OpenFGA 为外部后端） | 业务代码绕过决策点自行判权 |

**契约与扩展**

- API wire 契约在 `contracts/`（control-plane openapi 等）；修改后走生成与 `verify:contracts`。
- **Provider 语义**契约在 `contracts/provider/`（JSON Schema + fixtures/golden）；`verify:contracts` 会消费。Rust/Go 类型是实现，不得只改实现不改 schema/门禁。CP/协调/Web **只消费平台语义**（事件 type、ErrorEnvelope code/family 等），禁止在业务核心解析 Provider CLI 方言。
- Provider 类型与外部能力类型 **不以业务核心封闭枚举为准**，以注册表与服务端校验为准。
- 客户差异进 Tenant Profile / Connector / Semantic Mapping / Capability / Policy，**不进**核心流程代码。

**已知债**（现状 ≠ 规范，不得据现状反推规范）

- `contracts/runtime/openapi.yaml` 尚未与 control-plane 同等纳入自动契约门禁；改 runtime 契约时需人工核对下游。

## 协作模型

- 每 Project 一个虚拟协调线程（Temporal，`WorkflowID = project-coordinator:{project_id}`）：独占协调状态机，非数字员工实体；协调动作须产出结构化 RouteDecision / ProjectTask / 审计。
- 项目 **至少一名**人类负责人（可多、平级、any-of-N）；人类成员负责最终业务判断与验收，不划分子角色特权。
- Agent 之间不靠自由聊天协作，靠结构化对象；阶段产出须可持久化（工件、证据、决策、交接包）。
- 全局上下文由控制平面持久化；执行只注入任务所需切片；关键结论结构化回写。

## 工程约定

- 技术栈以当前 workspace 与 `package.json` 脚本为准；不擅自引入并行主栈。命令优先 `corepack pnpm <script>`，不发明未登记命令。
- 验证走仓库 `verify:*` / `generate:control-plane`；启停用 `scripts/dev-services.sh`。worktree 并行联调读 `docs/PARALLEL_DEVELOPMENT.md`（共享 `SUPERTEAM_DEV_PID_DIR`；`status` 的 `owner=` 为真实 cwd）。
- 库表与迁移遵循 `DATABASE_DESIGN.md`；生产迁移仅 `apps/control-plane/internal/storage/migrations/`，变更后校验 `atlas.sum` / `migrate-validate`。
- 用户可见中文：状态/枚举经 `apps/web/src/lib/status-labels.ts`（含 `failure_family` 等）；业务指称用名称而非裸 UUID。前端样式/布局先读 `DESIGN.md`；站内跳转用 TanStack Router。
- **共享 checkout 时**：只用 `git add <显式路径>`；禁止为他人切/删分支；交织文件只暂存自己的 hunk（`git diff --no-ext-diff`）；全仓生成（sqlc/openapi）会吸收他人在途改动，提交前核对暂存。优先一会话一 worktree。
- 影响架构或业务的不确定点先问人，不猜。

## 验证与收尾

- **默认完成条件是真实端到端**：Web + Control Plane + DB + Runtime + Provider（按变更面）。mock/单测/构建通过 ≠ 真链路已验证。无法验证须标阻塞并说明依赖。
- **声称 E2E 通过须证明验证期内服务未被接管**（记下并复核 `control-plane`/`web` 的 pid；`owner=` 异源则结论作废）。
- **轻量例外**：纯文案/单点样式/仅改规范文且不改交互数据权限持久化链路时，可只做 diff/定向测试。
- 人类明确延后的事项写入根目录 `TODO.md`（一行一条，完成即删）。
- 收尾走 `superteam-completion-check`（`.codex/skills/superteam-completion-check/SKILL.md`）。
- 合入 `main` 后须在 main 代码上再做真实验证，通过后再删分支/worktree。
