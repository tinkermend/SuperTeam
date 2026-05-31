## 项目定位

SuperTeam 是企业级数字员工控制平面。目标是把 AI 执行能力、流程调度、人类审批、上下文、工件和审计纳入统一控制平面。

## 架构分层

- **Console Layer**：第一阶段只实现 Web 控制台主链路。Desktop 仅保留空壳或占位，暂不做业务适配；待 Web 主链路完整后，再作为原生壳承载通知和快速查看，不承担本机执行能力。
- **Control Plane Layer**：Go 后端。负责任务、审批、审计、流程调度、上下文、工件、Runtime 注册和外部能力注册。对 Console 提供 API，也对 Runtime Agent 提供任务和心跳 API
- **Runtime Layer**：部署在服务器节点、开发者机器或客户侧执行机上的 Rust daemon。负责领取任务、维护租约、管理本机 Provider 进程/会话、工作目录、日志、工件和执行槽位；Runtime 对外 HTTP/WS contract 参考 `AionUi` 的 WebUI/remote agent host 思路，但不承载控制台 UI 和业务策略。
- **Provider Layer**：Claude Code、OpenCode、Codex、Pi 等具体执行器。它们只在 Runtime Agent 管理下工作，不承载平台级状态；Provider 协议必须保持语言无关，使用结构化 schema 描述输入、事件、结果、工件和错误，Rust 只是一种 adapter 实现语言。
- **Capability Integration Layer**：外部能力接入层。平台只负责外部能力的注册、授权、HTTP 调用和审计。

## 技术选型

- Web：shadcn admin 脚手架 + React + shadcn/ui + Radix UI + Tailwind CSS
- Control Plane：Go + chi/net/http；REST/OpenAPI 为主，使用 `oapi-codegen` 生成契约与客户端
- Runtime Agent：Rust + Tokio + clap；HTTP claim + lease；WebSocket 回传实时事件；对外 HTTP/WS contract 参考 `AionUi` 的 WebUI/remote agent host；本机 Provider 通过语言无关的 `provider` contract 接入，Claude Code 和 OpenCode adapter 第一版参考 `desktop-cc-gui` 的 Rust/Tauri 后端会话、命令构造和事件桥；NATS 后续在多节点事件总线需要时再引入
- 工作流：Temporal
- 数据层：PostgreSQL 为主存储；Redis 用于缓存、唤醒和轻量队列；S3 兼容存储用于日志、报告、附件和执行产物
- Go 数据访问：pgx + sqlc + Atlas
- 权限：先保留统一授权接口，避免业务代码散落权限判断；企业级授权目标为 OpenFGA
- 前端状态与交互：TanStack Query、TanStack Table、xyflow、Monaco Editor、xterm.js
- 表单校验：React Hook Form + Zod
- 图标：lucide-react
- 测试：Vitest、Playwright、Go test + testify、Rust cargo test、Temporal workflow test suite

## 数据库表命名规范

**规范原则**：

- 使用模块前缀分组：`{module}_{entity}` 格式
- 核心业务表可简化前缀（如 `tasks` 而非 `task_tasks`）
- 所有表名使用小写 + 下划线（snake_case）
- 主键统一命名为 `id`
- 时间戳字段统一使用 `created_at`, `updated_at`, `deleted_at`
- 外键字段命名为 `{referenced_table}_id`

**模块前缀定义**：

- `runtime_*` - Runtime Agent 节点管理
- `task_*` 或 `tasks` - 任务管理（核心表简化为 `tasks`）
- `workflow_*` - 工作流编排
- `approval_*` - 审批流程
- `audit_*` - 审计日志
- `auth_*` - 认证授权
- `tenant_*` - 租户管理
- `employee_*` - 数字员工定义
- `capability_*` - 外部能力注册

**字段类型约定**：

- ID 使用 `UUID`
- 时间戳使用 `TIMESTAMPTZ`
- JSON 数据使用 `JSONB`
- 枚举使用 `VARCHAR` + 应用层校验

## 目录边界

```text
apps/
  web/             # Web 控制台入口
  control-plane/   # Control Plane Go 服务
    cmd/control-plane/
    internal/
      api/          # REST/OpenAPI 路由、请求响应适配
      auth/         # 登录、会话、访问控制入口
      tenant/       # 租户、成员、工作区
      employee/     # 数字员工定义、技能绑定、权限边界
      task/         # 任务生命周期、状态流转
      workflow/     # 流程模板、Temporal 调度、节点状态
      approval/     # 人类审批、暂停/恢复
      runtime/      # Runtime Agent 注册、心跳、claim、lease
      artifact/     # 工件、日志、报告
      audit/        # 审计事件、操作记录
      capability/   # 外部能力注册、授权、HTTP 调用
      policy/       # 风险策略、审批策略、权限判断
      storage/      # DB、Redis、S3 封装
      config/       # 配置加载
  runtime-agent/   # Rust Runtime Agent daemon
    src/
      providers/    # Claude Code / OpenCode / Codex 等 Provider 适配
      controlplane/ # 调用 Control Plane 的客户端，按实际需要创建
      lease/        # claim、renew、heartbeat、任务租约，按实际需要创建
      slots/        # 本机并发执行槽位，按实际需要创建
      workspace/    # 本机工作目录、仓库、文件权限，按实际需要创建
      artifact/     # 工件收集、上传，按实际需要创建
      secrets/      # 本机密钥读取和脱敏，按实际需要创建
      health/       # 节点健康检查、环境探测，按实际需要创建

contracts/
  control-plane/   # Console <-> Control Plane REST/OpenAPI 契约
  runtime/         # Runtime Agent <-> Control Plane HTTP/WS 契约
  provider/        # Runtime Agent <-> Provider 契约

connectors/
  http/            # 通用 HTTP 外部能力接入
  custom/          # 客户专属连接器隔离放置
```

## 协作规则

- 数字员工不是聊天机器人，应围绕任务、输入、输出、权限、上下文策略和风险等级建模。
- Agent 之间不直接自由聊天，通过 `Finding`、`Artifact`、`Handoff`、`Blocker`、`DecisionRequest`、`ExecutionResult`、`Risk`、`NextActionProposal` 等结构化对象协作。
- Coordinator 负责判断下一步调用哪个数字员工、是否需要补充上下文、是否需要人类审批、是否回退或结束流程。
- 协作规则长期采用 Coordinator + 结构化对象；MVP 执行链路先固定为 `需求分析 -> 人类确认 -> Runtime Agent 执行 -> 回传结果和工件 -> 人类验收`。
- 每个阶段都必须产出可持久化的工件或交接包，不能只留在模型上下文里。
- 人类决策是一等对象。高风险动作、需求歧义、权限不足、上线发布、删除写入、测试失败后的业务判断等场景必须暂停并等待确认。
- 全局上下文由控制平面持久化；执行时只注入当前任务需要的上下文切片；关键结论必须结构化回写。
- 客户差异不要进入核心流程代码，应放入 Tenant Profile、Connector、Semantic Mapping、Capability 配置和 Policy。
- 外部能力类型和 Provider 类型不要在业务核心里依赖封闭枚举；以注册表和服务端校验为准。
- Provider 接入优先走统一且语言无关的 `provider` contract；当 Provider 协议不完整时，再使用 CLI、stdio、JSON stream、PTY 或 HTTP adapter 兜底。Claude Code 和 OpenCode adapter 第一版参考 `desktop-cc-gui`，但不得把 `desktop-cc-gui` 的 UI、Tauri command 边界或本地状态模型搬进平台核心。
- 控制平面不直接执行本地命令。Runtime Agent 只负责节点执行，不负责业务策略、人类审批策略和长期业务状态。

## 开发规则

- 每次功能开发完成写`CHANGELOG.md` 记录变更日志
- 后续 Web 页面开发、UI 组件新增或视觉改造时，参考 `DESIGN.md` 的设计风格、基础组件风格和实施检查清单。
- 不要盲目猜测,如果有不确定的地方与人类进行沟通
- 开发完成后做好必要的功能测试,单元测试,回归测试
- 数据库信息 见 doc/database/conn_info.md
- 核心数据完整性应该让 PostgreSQL 兜底；复杂状态机、权限、审批和跨系统一
  致性放应用层
- 新增的数据库表增加对应的中文注释包含字段注释与表注释

## Agent skills

### Issue tracker

Issue 以本地 markdown 文件形式存放在 `.scratch/<feature-slug>/` 下。详见 `docs/agents/issue-tracker.md`。

### Triage labels

五个分类标签均使用默认值：`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`。详见 `docs/agents/triage-labels.md`。

### Domain docs

单上下文布局：`CONTEXT.md` + `docs/adr/` 位于仓库根目录。详见 `docs/agents/domain.md`。

## 其他规则

- 编写的文档语言都以简体中文输出
