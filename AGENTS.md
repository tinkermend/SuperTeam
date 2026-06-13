## 项目定位

SuperTeam 目标是把 AI 执行能力、流程调度、人类审批、上下文、工件和审计纳入统一控制平面。

项目参与者包含数字员工和人类。数字员工是 agent 型业务身份，围绕角色、任务边界、权限、上下文策略和输出契约建模；人类是一等管理、审核、决策和验收参与者，不归入数字员工。

项目不是只表示软件交付项目，也可以表示一次具体问题场景的闭环。项目是目标、负责人、虚拟协调线程、任务、证据、预算、审批和验收结论的聚合容器；流程编排是驱动项目运行的模板，不替代项目作为业务事实入口。

## 架构职责边界

- **Console Layer**：Web 控制台是管理、观察、审批和验收入口；不得承载本机执行能力、业务事实源或长期业务状态。
- **Control Plane Layer**：Go 后端负责业务状态、任务、审批、审计、流程调度、上下文、工件、Runtime 注册和外部能力注册。对 Console 提供 API，也对 Runtime Agent 提供任务和心跳 API。
- **Runtime Layer**：部署在服务器节点、开发者机器或客户侧执行机上的 daemon。负责领取任务、维护租约、管理本机 Provider 进程/会话、工作目录、日志、工件和执行槽位；不承载控制台 UI、业务策略和长期业务状态。
- **Provider Layer**：Claude Code、OpenCode、Codex、Pi 等具体执行器。它们只在 Runtime Agent 管理下工作，不承载平台级状态；Provider 协议必须保持语言无关，使用结构化 schema 描述输入、事件、结果、工件和错误，Rust 只是一种 adapter 实现语言。
- **Capability Integration Layer**：外部能力接入层。平台只负责外部能力的注册、授权、HTTP 调用和审计。

## 主栈边界

- 技术栈以当前 workspace、契约和构建脚本为准；不得在没有明确共识时引入替代主栈的并行框架或重复基础设施。

## 数据库设计规则

- 数据库表设计、字段类型、UUID-first、租户/团队、索引、迁移、sqlc 与 OpenAPI 规则统一遵循根目录 `DATABASE_DESIGN.md`。

## 目录边界

- Web 控制台代码放在 `apps/web/`，不把 Control Plane、Runtime 或 Provider 的业务状态搬进前端。
- Control Plane Go 服务放在 `apps/control-plane/`，负责业务状态、策略、审批、审计、调度和 API，不直接执行本地命令。
- Runtime Agent 放在 `apps/runtime-agent/`，只负责节点执行、租约、Provider 进程/会话、工作目录、日志和工件。
- API 契约放在 `contracts/`；修改契约后必须走生成与契约验证流程。
- 外部能力接入放在 `connectors/` 或 Capability 配置边界内，客户专属逻辑不得硬编码进核心流程。
- HTML 原型放在 `docs/prototypes/`。

## 协作规则

- Project 是面向具体目标或问题场景的业务闭环容器，不在核心模型里定义封闭项目类型枚举。场景差异通过场景模板、Workflow Template、项目画像、标签、Policy 和服务端注册校验表达。
- 一个 Project 绑定一个虚拟协调线程。虚拟协调线程由 Temporal Workflow 承载（WorkflowID = `project-coordinator:{project_id}`），是项目内置的独占协调状态机，不是数字员工实体，不出现在数字员工列表中。它通过 Signal 接收事件、串行处理协调决策、并发分派数字员工执行任务，所有协调动作必须产出结构化的 RouteDecision、ProjectTask 和审计记录。
- 每个项目必须绑定固定人类负责人（human_owner），并可绑定 leader 或验收人。人类负责人负责最终业务判断、审批、驳回、补证要求、汇报接收和验收结论。
- 人类员工和数字员工都是项目成员，但职责不同。人类可以作为负责人、审批人、验收人、或观察者；数字员工只作为项目内可调度的执行员工，从项目数字员工池中选取。不要把人类管理职责建模成数字员工能力，也不要让数字员工绕过人类决策。
- Agent 之间不直接自由聊天，通过结构化对象协作；每个阶段都必须产出可持久化的工件、证据、决策或交接包。
- 人类决策是一等对象。高风险动作、需求歧义、权限不足、上线发布、删除写入、测试失败后的业务判断等场景必须暂停并等待确认。
- 全局上下文由控制平面持久化；执行时只注入当前任务需要的上下文切片；关键结论必须结构化回写。
- 客户差异不要进入核心流程代码，应放入 Tenant Profile、Connector、Semantic Mapping、Capability 配置和 Policy。
- 外部能力类型和 Provider 类型不要在业务核心里依赖封闭枚举；以注册表和服务端校验为准。
- Provider 接入优先走统一且语言无关的 `provider` contract；当 Provider 协议不完整时，再使用 CLI、stdio、JSON stream、PTY 或 HTTP adapter 兜底。
- 控制平面不直接执行本地命令。Runtime Agent 只负责节点执行，不负责业务策略、人类审批策略和长期业务状态。

## 开发规则

- 不要盲目猜测；如果存在无法从本地上下文确认且会影响架构或业务判断的不确定点，先与人类沟通。
- 前端页面、布局或样式变更前必须阅读 `DESIGN.md`。
- 每次功能、修复、合并或跨层联调任务收尾前，必须使用项目内 skill `$superteam-completion-check`（`.codex/skills/superteam-completion-check/SKILL.md`）做完成前检查；不得把 mock、组件测试、单元测试或构建通过表述为真实链路已验证。
