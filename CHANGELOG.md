# Changelog

All notable changes to the SuperTeam Control Plane project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- 2026-06-05 00:52：Runtime command writeback 改为校验已认证 Runtime 节点身份，终态与 provisioning 回写在事务内锁定 command receipt 并修补重放投影，同时合并 Provider session patch 并脱敏 Runtime 来源的结构化敏感字段。
- 2026-06-05 00:30：Runtime command writeback 补齐数字员工执行实例 provisioning 成功/失败的 ready 与清理状态回写，并允许终态 run 重放已持久化事件序号保持幂等。
- 2026-06-05 00:08：数字员工 run Console API 分页参数改为按 int32 范围解析，避免超大 `offset` 在下传查询前发生整数截断。
- 2026-06-05 00:03：数字员工 run Console API 移除响应中的内部 `idempotency_fingerprint`，补齐列表与事件分页参数校验和默认值，并将 Runtime dispatch 失败统一映射为运行时不可用。
- 2026-06-04 14:49：Runtime Agent 删除旧版 `auth_token` 配置兼容入口，YAML、`RUNTIME_AGENT_AUTH_TOKEN` 环境变量和 CLI `--auth-token` 不再作为 `bootstrap_key` 兜底。
- 2026-06-04 14:33：数字员工 API 授权从复用 `runtime_scope.manage` 改为独立 `employee.*` 业务 action，并按 tenant 集合资源和单个 employee 资源记录授权决策，为后续 OpenFGA 渐进接入保留稳定边界。
- 2026-06-04 11:00：Web `Main` 布局组件改为默认铺满内容区，并新增 `contained` 窄版选项，后续控制台页面无需逐页传 `fluid` 即可复用权限中心式全宽布局。
- 2026-06-03 23:15：`AGENTS.md` 明确要求每条新增 `CHANGELOG.md` 变更记录带具体时间，默认使用本地 `Asia/Shanghai` 时间。
- Web 设计系统收敛为浅色液态玻璃控制台方向，`DESIGN.md` 明确蓝绿色主强调、液态玻璃卡片、胶囊按钮、语义图标和“只迁移 UI/UX 样式、不带入示例业务内容”的规则。
- Web 登录页、认证后 Shell 和工作台首页按浅色液态玻璃设计语言更新，统一按钮、卡片、背景、导航激活态、搜索框和输入控件质感。
- `DESIGN.md` 补充页面级和模块级 Tab 设计规则，明确玻璃胶囊激活态、底边流光锚点、中文长标签适配和 SuperTeam 组合组件复用边界。
- Web 主按钮和侧栏激活菜单降低左侧白色反光强度，避免高光压住图标和中文文字。
- Web Shell 背景色彩调整为参考图方向的淡青绿、暖米色和冷绿低饱和 wash，并将认证后控制台背景收敛为单一 `sidebar-wrapper` 背景源；顶部栏和内容容器保持透明，不再在刷新完成后覆盖外层背景，侧栏面板补充柔和 tint 并弱化与主页面之间的深色分割线。
- Web 侧栏 icon 折叠态收紧内容区内边距并居中导航按钮，修复图标贴近右侧边界的问题。
- Web 侧栏菜单默认字号从 14px 提升到 16px，并将菜单行高调高到 44px，提升导航可读性。
- Web 侧栏选中菜单改为清晰白色文字和图标，移除文字阴影并避免默认 active utility 覆盖导致文字发暗发糊。
- Web 暗色主题补充独立玻璃覆盖，修复左侧菜单、卡片和图标容器复用浅色白色高光导致的刺眼与可读性问题。
- Web 登录页暗色主题补充独立 Auth Shell、品牌标识和登录卡片覆盖，避免浅色背景与暗色文字 token 混用。
- Control Plane 初始数据库 schema 重写为 UUID-first 形态，合并早期 auth session、Web 登录日志、操作日志和中文注释迁移，并新增默认租户/团队骨架以支撑后续分布式与多团队数字员工管理。
- 任务、执行、审计、工件、Web 登录日志和操作日志改为应用层校验的 UUID 引用，避免跨模块重 FK 和级联删除；任务、工件、用户、Runtime 节点等核心实体补充软删除、禁用、归档、取消或终止时间戳。
- Runtime 任务状态机允许已领取任务直接进入 completed/failed，修复当前 claim -> events -> complete/fail HTTP 合约没有单独 running 接口时的完成链路阻断。
- 将数据库表设计规则从 `AGENTS.md` 沉淀到根目录 `DATABASE_DESIGN.md`，统一后续 UUID-first、租户/团队、索引、迁移、sqlc 与 OpenAPI 设计规范。
- 数据库迁移规范明确普通功能开发必须新增 forward migration，禁止回写已存在于 `atlas.sum` 的迁移文件，并将 rebuild-only 限定为需当次确认和写明备份、重建、验证命令的例外流程。
- Control Plane 请求日志新增 `remote`、`ua` 和 `referer` 字段，便于定位未知请求来源。
- Control Plane 与 Runtime Agent 本地开发配置统一收敛为 YAML 文件：Control Plane 使用被 Git 忽略的 `apps/control-plane/config/config.yaml`，Runtime Agent 使用被 Git 忽略的 `apps/runtime-agent/config.yaml`；Runtime Agent 示例配置从 TOML 切换为 `config.example.yaml`，并移除 Control Plane / Runtime Agent 的 `.env.example` 示例入口。
- Web 与 Control Plane 示例配置同步本地默认端口到 `8081`，避免前端示例 API 地址和后端示例监听端口不一致导致开发登录误报为用户名或密码错误。
- Control Plane 本地开发脚本默认加载 `apps/control-plane/config/config.yaml`，并兼容 `pnpm dev:control-plane -- --config ...` 的参数传递形式；配置入口统一以 YAML 文件为准。
- Web 控制台从旧 Next.js + 前端 workspace packages 结构激进重铺为 Vite + TanStack Router + shadcn-admin 单应用结构；前端 API client、认证状态、页面和 UI 组件集中到 `apps/web/src`，后端 Control Plane API 契约保持不变。
- Web 控制台移除 shadcn-admin demo 路由和 mock 数据页面，改为 SuperTeam 工作台、用户管理和任务/审批/审计等领域入口。
- Web shadcn-admin 路由接入 `AuthProvider`，认证守卫统一未登录跳转 `/login`，并将登出流程切换为 Control Plane session logout。
- Web shadcn-admin 登录表单从 mock cookie/token store 切换为 Control Plane cookie session auth，新增 `AuthProvider` / `useAuth` 负责加载当前用户、登录、登出和窗口聚焦后的会话刷新。
- 将一键验收脚本扩展为开发门禁入口：`pnpm verify:foundation` 现在聚合契约、TypeScript、Go 和 Rust 基础验证，并新增 `verify:web`、`verify:control-plane`、`verify:runtime-agent`、`verify:db` 领域门禁。
- 在 `docs/development.md` 中新增“开发验证门禁”，定义基础门禁、领域门禁、场景 smoke 和后续功能开发时的动态更新规则。
- Web 控制台在 Vite 环境下使用 `VITE_CONTROL_PLANE_URL` 配置 Control Plane 地址；未显式配置时继续跟随当前浏览器 host 推导，避免本地开发时 `127.0.0.1` 与 `localhost` 混用导致登录 Cookie 不被后续请求携带。
- 调整 Control Plane storage sqlc 查询集成测试：
  - 移除 testcontainers 本机容器 fallback，避免完整 Go 测试依赖 Docker/Podman。
  - 测试仅在显式配置 `TEST_DATABASE_URL` 和 `TEST_REDIS_URL` 时连接远端或专用测试环境运行；也支持通过 `ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1` 复用 `DATABASE_URL` 和 `REDIS_URL`。
  - 未配置测试环境时跳过 `apps/control-plane/internal/storage/queries` 集成测试。
- Provider 会话事件新增关联 ID 约束，要求 `request_id` 或 `command_id` 至少填写一个，避免无法回溯到平台请求或命令的事件进入审计链路。
- 数字员工执行链路补强 Runtime 与 Provider 可用性校验：执行实例绑定前要求 Runtime 已批准且 Provider 能力可用，Provider 会话和事件写入会拒绝禁用、错误或失效的执行上下文。
- Provider 会话事件追加时重新校验会话、数字员工与执行实例仍处于可接收事件状态，防止关闭、禁用或错误的执行上下文继续写入事件流。
- Provider 会话创建与事件追加只允许绑定 `ready` / `active` 执行实例，防止 `provisioning` 等未就绪状态进入 Provider 执行链路。
- Runtime Enrollment 和 Runtime Session 查询补强租户、Bootstrap Key、批准状态与撤销边界校验，移除可绕过执行前置条件的数字员工执行实例直接插入查询。
- Runtime hello 接入写入固定为 pending 状态，并要求有效 Bootstrap Key 与 Runtime 节点外部 ID 匹配，防止 hello 绕过人工审批或错绑节点。
- Runtime Agent 启动链路从旧版长生命周期 runtime token 注册切换为 bootstrap key hello 接入，批准后使用短期 runtime session token 访问 heartbeat、任务领取和事件回传接口，并按真实 Control Plane contract 上报扁平 capabilities。
- Runtime Agent 建立短期 session 后会主动连接 Control Plane Runtime WebSocket 命令通道，并支持接收 `ensure_instance` 命令创建数字员工执行实例目录。

### Added

- 2026-06-05 00:24：Control Plane 新增 Runtime command HTTP writeback API，Runtime session auth 可回写 provider 事件、完成、失败、取消和超时终态，并将事实持久化到 command receipt、task run/event 与 Provider session/event 双投影。
- 2026-06-04 23:48：Control Plane 暴露数字员工 run Console API 路由，支持创建、列表、详情、事件查询和停止，并接入独立 `employee.run.*` 授权 action 与真实 app wiring。
- 2026-06-04 14:28：新增 `scripts/dev-services.sh` 本地开发服务管理脚本，支持 Control Plane、Web 和 Runtime Agent 的状态检查、启动、停止和重启，并以 `.scratch/dev-services` 记录 PID 与日志；新增脚本级测试覆盖启停与重启流程。
- 2026-06-04 10:38：用户管理页按“用户 360 详情台”方向升级为主从详情工作台，使用权限中心一致的铺满页面布局，并调整为更宽的用户列表和三等宽概览卡片；页面接入用户列表、权限中心成员角色、登录日志和授权拒绝记录，并补齐新建用户、启用/禁用账号和重置密码入口。
- 2026-06-04 05:20：团队列表搜索补齐负责人用户名、显示名和邮箱匹配，并修复新建团队失败后关闭重开抽屉仍显示旧错误的问题。
- 2026-06-04 05:02：团队管理体验补齐团队图标、用户头像身份展示、弱分页、目标化两步创建抽屉和详情成员页用户搜索，继续沿用浅色液态玻璃企业控制台设计风格。
- 2026-06-04 04:02：Runtime Agent 新增 runtime command execution layer，支持 `start_session`、`resume_session`、`send_input`、`stop_session` 在本地解析 payload、创建执行实例目录、驱动 Provider run、维护 command/session/run 映射并取消 active run；Runtime 仅执行本地命令，不判断租户、团队或审批。
- 2026-06-04 02:29：团队接口头像回显兼容历史用户空头像种子，按用户名生成稳定 fallback，并修复团队 metadata display 规范化时修改调用方输入的问题。
- 2026-06-04 01:49：用户管理补齐 DiceBear adventurer 头像配置字段、OpenAPI 响应和 Web 列表展示；Control Plane 存储头像来源、样式、种子和扩展选项，前端使用本地 DiceBear JS 包生成稳定 SVG 头像。

#### 团队管理权限底座 (2026-06-03)

- Control Plane `authz` 新增 OpenFGA-ready 团队管理 action 语义，覆盖团队 CRUD、禁用/归档/恢复、成员增删改、特权角色申请/批准、治理配置读写/批准、能力绑定和团队审计读取。
- `DBAuthorizer` 新增团队管理授权矩阵：租户 owner/admin 可创建并管理所有团队；团队 owner/admin 可维护本团队基础信息和普通成员；团队 owner 或 approver 可批准治理配置；直接添加/提升 owner/admin/approver 会要求走特权角色申请/批准语义，并保留最后团队负责人保护。
- 团队 API 路由停止复用 `runtime_scope.manage`，改为按路由发送 `team.create`、`team.read`、`team.governance.edit`、`team.governance.approve` 和 `team.governance.read` 等业务 action。
- 权限中心诊断契约和 Web 表单补齐团队管理 action 枚举，能按 action 自动派生 tenant/team resource，为后续 OpenFGA Authorizer 或 tuple 同步保留稳定 actor/action/resource 边界。

#### Web 数字员工创建流程 (2026-06-03)

- 数字员工页面新增创建草稿员工并预览生效配置流程，可基于团队当前治理配置生成个人配置修订，成功预览后提示可提交负责人确认，阻断错误或预览失败时给出页面反馈。

#### Web 团队管理页面 (2026-06-03)

- 新增团队管理控制台基础计划的列表摘要、详情概览、生命周期操作和前端详情框架。
- 新增团队成员管理、普通角色直接变更和高权限角色申请审批流程。
- Web 控制台新增“团队管理”侧栏入口和 `/teams` 页面，可查看团队负责人、当前治理配置修订号、宪法硬性规则、能力边界和内部协作自动轮次；当前治理配置缺失或加载失败时按单行降级提示，不阻断团队列表展示。
- 新增团队治理草稿、能力与知识绑定、治理策略编辑和批准生效流程。
- 新增团队详情中的数字员工入口和团队管理审计记录。
- 2026-06-03 23:29：团队列表 API 新增 `governance_status` 筛选和负责人摘要响应，前端后续可按设计稿展示负责人姓名/邮箱并区分草案待批准、已生效、未配置等治理状态。
- 2026-06-04 00:52：团队管理页补齐真实筛选工具条和右侧两步新建团队抽屉，创建团队时通过 Control Plane 事务一次性写入团队、负责人和初始成员，并以真实接口刷新列表。
- 2026-06-04 01:08：新建团队抽屉将负责人选择改为独立搜索，避免团队名称和负责人候选查询耦合，补齐加载、空态和选中态反馈。

#### 团队治理后端 (2026-06-03)

- Control Plane 新增租户团队领域服务、PostgreSQL repository 和 API 路由，支持团队负责人、共享治理配置版本创建与当前版本查询；相关路由由 Web 控制台会话认证保护，并统一经过 `team.*` 业务授权校验。
- 数字员工服务新增个人配置版本、生效配置预览与校验、审批落库能力；预览会阻断团队能力白名单外的个人能力、团队上下文范围外的上下文覆盖以及降低团队审批要求的个人审批覆盖。
- 数字员工创建新增同租户团队存在性校验；生效配置预览与审批只接受 active 团队治理配置版本，避免 draft 配置绕过团队负责人确认。
- 团队治理配置版本创建不再接受客户端指定 `approved_by`，审批归属由当前 Web 控制台登录用户在服务端注入。
- 新增 `003_add_team_governance_config.sql` forward migration，补齐已执行旧版 001/002 的远端库中的团队治理配置、数字员工个人配置和生效配置快照表。
- 数字员工生效配置审批要求已有 ready 或 active 的唯一执行实例，并新增 `/api/v1/digital-employees/{employeeId}/config-revisions`、`/effective-configs/preview`、`/effective-configs/approve` API 路由。
- OpenAPI 与基础契约守卫补齐团队治理、数字员工配置版本和生效配置预览/审批 API，确保新增后端路由纳入契约核验。
- Web API client 新增团队列表、团队创建、团队治理配置版本创建与当前版本查询能力，并补齐数字员工个人配置版本、生效配置预览和审批调用入口。

#### Web 液态玻璃组件化 (2026-06-03)

- 新增 `apps/web/src/components/superteam` 项目级设计组件层，沉淀 `LiquidCard`、`LiquidPill`、`PrimaryLiquidButton`、`SemanticIconTile`、`StatusBadge` 和 `MetricCard`。
- 新增 `LiquidTabsList` 和 `LiquidTabsTrigger` 复用组件，将页面 Tab 统一为玻璃胶囊选中态与底边流光指示，并迁移权限中心和团队详情页。
- 工作台首页指标卡改为复用 `MetricCard`，减少页面内手写玻璃卡片、语义图标和状态胶囊样式。
- 为液态玻璃设计组件补充 Vitest 浏览器组件测试，锁定组件 slot、核心 class 和基础渲染行为。

#### 数字员工后端与执行实例服务 (2026-06-02)

- 新增 Control Plane 数字员工领域服务、PostgreSQL repository 和 HTTP handler，支持创建草稿、列表、详情、状态更新以及唯一执行实例查询和绑定。
- 数字员工执行实例绑定复用现有 sqlc upsert 查询，默认绑定为 ready 状态，并通过 Web 控制台 cookie session 注入租户上下文保护 `/api/v1/digital-employees` 路由。
- Web 控制台新增数字员工页面和 API client，支持查看数字员工业务身份、状态、风险等级和唯一执行实例绑定信息。

#### Runtime 接入与短期会话服务 (2026-06-02)

- 新增 Control Plane Runtime Enrollment / Runtime Session 领域服务，支持 Runtime hello 接入、人工批准/拒绝/撤销、短期 session token 签发、校验与续期。
- Runtime session token 采用确定性 lookup hash 加 bcrypt secret hash 的双哈希模型，避免原始 token 或可直接校验的单一 hash 明文落库。
- Runtime hello 会扫描有效 Bootstrap Key 并用 bcrypt 校验原始 secret；pending 接入不会返回 session，approved 且已挂接 Runtime 节点的接入才签发短期 session。
- Runtime enrollment 撤销会使关联 active session 失效，session 校验与续期会重新检查接入仍处于 approved 且未撤销状态。
- 接通 Runtime Enrollment、短期 Session 续期与 Capability 上报 HTTP 路由；`/api/v1/runtime/enrollments/hello` 支持公开 bootstrap hello，`/api/v1/runtime/session/renew` 和能力上报要求短期 session token。
- 新增 Runtime session middleware，并让 heartbeat、claim、task event、complete、fail 和 lease 路由在迁移期同时接受短期 session token 或旧版 `Authorization + X-Node-ID` runtime token。
- Runtime enrollment 管理路由改为 Web 用户 cookie session 保护，并将 canonical session renew 与 capability upsert 成功响应对齐 OpenAPI 契约。
- Runtime enrollment 管理路由补充 `runtime_scope.manage` 授权校验，并将当前 Web 用户与租户上下文传入批准、拒绝和撤销操作。
- 修正 Runtime 接入服务的多租户路径：hello 阶段不再创建默认租户 Runtime 节点，改为仅写入 pending enrollment；批准阶段按租户创建或复用 Runtime 节点并 attach，session 校验改为按全局 lookup hash 查找，支持非默认租户续期。
- 将 Runtime enrollment 批准改为单条 SQL 原子完成 pending 校验、tenant-safe Runtime 节点 upsert 和 enrollment attach，避免并发拒绝/撤销后留下未挂接节点，并修复 tenant-aware node upsert 的全局 `node_id` 冲突竞态。
- 将 Runtime session token 默认有效期修正为 12 小时，并补充签发和续期过期时间断言。
- 新增 Runtime WebSocket Command Channel：Control Plane 维护 Runtime 连接注册表，短期 session 认证的 Runtime 可通过 `/api/v1/runtime/ws` 建立命令通道并接收 JSON command；旧版 Runtime token 不能访问该通道。
- 加固 Runtime WebSocket Command Channel 连接注册表：Dispatch 不再持有全局锁等待满载 channel，Runtime 重连和断开清理不会被阻塞，并补充 registry 未配置与 client close 后注销测试。
- Web 控制台新增 Runtime 节点页面，支持查看待接入 Runtime enrollment、批准接入请求以及查看已接入 Runtime 节点状态和 Provider 能力。

#### Control Plane 渐进式授权边界 (2026-06-01)

- 新增 Control Plane 渐进式授权边界：`internal/authz` 统一 `Authorizer` 接口，第一版使用 PostgreSQL 权限事实判断 Web 控制台访问和 Runtime claim 范围。
- `/api/auth/me` 登录后增加 `console.access` 授权检查，认证和授权保持分层。
- Runtime claim 任务前增加 `task.claim` 范围检查，Runtime 节点不能领取超出 `runtime_node_scopes` 的任务。
- 授权决策接入 `web_operation_logs`，记录允许/拒绝结果、授权引擎、命中规则、Actor、资源和租户/团队上下文，为后续权限审计视图和 OpenFGA backend 留出稳定审计底座。

#### 权限中心 MVP (2026-06-01)

- 新增 Control Plane 权限中心 API 契约和 `internal/authzcenter` 应用服务层，提供授权概览、授权审计、Runtime 范围、成员角色只读视图和权限诊断接口。
- 新增 Web 一级菜单“权限中心”和 `/permissions` 页面，包含“授权概览”“授权审计”“Runtime 范围”“成员角色”“权限诊断”五个 Tab。
- Runtime 范围管理支持新增租户/团队 scope 以及启用、禁用已有 scope；Web 表单按租户或团队自动派生只读范围值，避免提交不符合后端约束的 payload。
- Runtime scope 写操作统一经过 `runtime_scope.manage` 授权检查并写入 `web_operation_logs`，权限中心读接口通过 `authz_center.read` 做租户边界控制。
- 权限诊断通过统一 `Authorizer.Check` dry-run 返回授权结果，并按 action 自动匹配资源类型与必要字段校验。
- Web 权限中心 API client 与页面补充 Vitest 覆盖，锁定请求方法、请求体、Runtime scope 写入确认和诊断行为。

#### Web Vite 控制台重铺 (2026-05-31)

- 新 Web 壳接入 `shadcn-admin` 的侧边栏、顶部栏、主题、命令面板和响应式布局。
- 保留真实 Control Plane 登录、当前用户、退出登录和路由保护主链路，继续使用 cookie session 与 `credentials: "include"`。
- 新增 Vite 环境变量 `VITE_CONTROL_PLANE_URL`，保留本地 `localhost` / `127.0.0.1` host 对齐策略。
- 删除不再服务 Web 的旧 UI、views、core 和 api-client 前端 workspace 拆分。

#### Web 外部能力占位入口 (2026-05-31)

- 新增 Web 控制台一级菜单“外部能力”及 `/capabilities` 占位页。
- 页面先说明后续外部能力扩展范围，包括 Dify Workflow、Deephub Agent、企业内部 HTTP 接口、数据分析服务、ITSM、CMDB/监控/日志平台、自研脚本服务和 MCP Server/Connector。
- `/capabilities` 当前作为公开占位说明页，不要求登录态，避免点击一级菜单后被全局登录守卫重定向到登录页。
- 为工作台、任务中心、数字员工、流程编排、审批中心和审计日志补齐公开占位页；除用户管理这类真实管理功能外，未开发一级菜单不再跳转登录页。

#### Web 平台 Shell 完善 (2026-05-31)

- 新增路由驱动的 `ConsoleAppShell`，统一 Web 控制台菜单 active、面包屑、登录用户展示和登出入口。
- `ConsoleShell` 支持面包屑渲染，并保留业务页面通过插槽注入页面操作。
- Web 控制台新增平台通用 empty、loading、error、forbidden 状态组件，用户管理页接入列表加载、空数据和加载失败状态。
- 新增 Web 控制台 `not-found`、`error`、`loading` 和 `/forbidden` 页面，先形成公共异常与权限不足入口。

#### Web 用户管理 MVP (2026-05-31)

- 新增 Web 控制台用户管理页 `/users`，支持用户列表、创建用户、启用/禁用用户和重置密码。
- Control Plane Auth API 新增用户管理接口：`GET/POST /api/auth/users`、`PATCH /api/auth/users/{id}/status`、`POST /api/auth/users/{id}/reset-password`。
- 用户管理写操作接入 `web_operation_logs`，记录创建用户、启用/禁用用户和重置密码操作。
- `apps/web/src/lib/api` 新增用户管理 client 方法，并将认证用户 ID 对齐为后端用户主键 `int64`。
- 新增迁移为 `auth_users` 与 `web_operation_logs` 补充中文表注释和字段注释。

#### Web 会话闭环体验 (2026-05-31)

- Web 控制台 Shell 新增右上角用户菜单插槽，支持展示当前登录用户和账户操作。
- 首页接入 `useAuth()` 当前用户状态，移除顶部账户区域的硬编码用户展示，并提供退出登录操作。
- Web 认证状态在窗口重新聚焦时复查 `/api/auth/me`，遇到 401 会清空当前用户并交由认证守卫回到登录页。
- 前端 API client 增加带 HTTP status 的 `ApiRequestError`，供前端认证层稳定识别会话失效。

#### Web 登录主链路 (2026-05-31)

- 接入 Web 控制台登录、当前用户和登出 API，使用 `auth_sessions` 持久化浏览器会话。
- 在 `apps/web/src` 内新增 auth client、AuthProvider/useAuth 和登录页。
- Web 根布局接入认证守卫，未登录用户进入 `/login`，登录成功后返回控制台首页。
- 将浏览器 session token 以 SHA-256 hash 写入 `auth_sessions.token_hash`，避免原始 cookie token 明文入库。
- 收紧 Control Plane CORS，使携带 cookie 的本地 Web 调用只允许 `localhost:3000` 和 `127.0.0.1:3000`。
- 新增幂等开发账号迁移，方便本地使用 `admin / admin` 完成 Web 登录烟测。

#### Web 登录日志与操作日志底座 (2026-05-31)

- 新增 `web_login_logs` 表，独立记录 Web 控制台登录成功、登录失败和登出事件，不复用人工审核相关的 `audit_events`。
- 新增 `web_operation_logs` 表，预留 Web 控制台后续功能操作日志、资源操作结果和请求上下文记录。
- 登录、登录失败和登出链路接入 `web_login_logs` 写入；日志写入异常不阻断主登录链路。
- 新增 `GET /api/auth/login-logs` 查询接口，要求有效登录 Cookie，并支持 `limit` / `offset` 分页参数。
- 前端 API client 新增 `listLoginLogs`，供 Web 端调用登录日志接口。

#### Core Summary 状态映射 (2026-05-30)

- 为任务和 Runtime 节点 summary helper 增加稳定状态 tone，供后续 Web 页面复用。
- 为 Runtime 节点 summary 增加负载百分比，避免每个页面重复计算槽位占用。

#### API Client 最小任务与 Runtime 覆盖 (2026-05-30)

- 为前端 API client 补齐任务详情、任务状态更新、任务取消和 Runtime 节点详情的最小 client 方法。
- 通过 Vitest 锁定这些方法使用的 Control Plane canonical path。

#### Foundation 契约漂移检查 (2026-05-30)

- 新增 `pnpm verify:contracts`，检查 Control Plane OpenAPI、Go route、Rust Control Plane client 和 TypeScript api-client 的关键路径一致性。
- 新增 `pnpm verify:foundation`，聚合契约检查、TypeScript 测试、TypeScript 类型检查和 Runtime Agent Rust 测试。

#### Foundation Readiness 底座收口设计 (2026-05-30)

- 新增 Foundation Readiness 设计文档，明确在进入具体功能开发前采用 Web、Control Plane、Runtime Agent、contracts 与共享 packages 的均衡收口方案。
- 定义本阶段的维护性、可扩展性、可复用性标准，并明确不提前实现登录认证、Temporal、OpenFGA、完整业务页面和生产级 Provider 治理。

#### Web 真实数据接入底座 (2026-05-29)

- 为任务和 Runtime 节点补充最小 API client 与 core summary helper，后续页面可从 mock 数据平滑切换到真实接口。

#### Foundation fake provider 端到端验收 (2026-05-29)

- 新增 fake provider 风格的最小端到端验收，覆盖任务创建、Runtime 注册、claim、事件回传和完成状态。
- 对齐 Runtime Agent 客户端的节点注册/心跳响应模型，移除未在 Control Plane contract 中承诺的内部数据库 `id` 依赖。
- 将 Control Plane Runtime 写入端点接入 Runtime token + `X-Node-ID` 认证，并让 Runtime Agent 对心跳、claim、事件、完成、失败和 lease 请求携带节点身份。
- 修正 Runtime token 生成脚本，使其写入当前 `auth_runtime_tokens(node_id, token_hash, expires_at)` schema。

#### Foundation Hardening 设计 Spec (2026-05-29)

- 新增 Foundation Hardening 设计文档，明确 Control Plane 启动边界、sqlc 生成闭环、契约事实源、Runtime Agent daemon 边界、执行事件流和 Web 真实数据接入底座。

#### Web 根布局 hydration 兼容 (2026-05-29)

- 在 Web 根布局 `<html>` 上启用 `suppressHydrationWarning`，降低浏览器扩展向根节点注入属性时触发的 hydration mismatch 噪音，并补充对应布局测试。

#### Web 控制台通用骨架 (2026-05-29)

- 沉淀 Web 控制台外部系统骨架复用组件：新增 `ConsoleShell`、状态胶囊、图标徽章、指标块、分区面板和时间线项，并将首页改为基于共享组件挂载。

#### Control Plane S3 对象存储接入 (2026-05-29)

- 使用 AWS SDK for Go v2 的 `config`、`credentials`、`service/s3` 初始化控制平面 S3 客户端。
- 新增 `S3ObjectStore` 边界，封装对象上传、下载、存在性检查和删除，并返回稳定的 `s3://bucket/key` 工件引用。
- 补齐 S3 配置校验，启动前检查 endpoint、region、bucket、access key 和 secret key。
- 更新配置模板和配置指南，保留 MinIO/localstack path-style 默认值，并补充 Volcengine TOS virtual-hosted 配置示例。

#### Runtime 任务执行结果 API (2026-05-29)

- 补齐 Runtime task events、complete、fail 和 lease endpoint 的基础处理，支持 Runtime Agent 回传结构化执行事件和终态。

#### Phase 4 - Runtime Agent Control Plane 集成 (2026-05-29)

- 添加 Runtime Agent Control Plane 客户端 (`apps/runtime-agent/src/controlplane/`)
  - client.rs: HTTP 客户端实现
    - ControlPlaneClient 结构：封装 reqwest HTTP 客户端
    - register(): 注册节点到 Control Plane
    - heartbeat(): 发送心跳更新节点状态和负载
    - claim_task(): 长轮询获取任务（支持超时）
    - 完整的错误处理和上下文信息
  - models.rs: API 模型定义
    - TaskStatus 枚举 (pending/claimed/running/completed/failed/cancelled)
    - NodeStatus 枚举 (online/offline)
    - RegisterNodeRequest/Response
    - HeartbeatRequest/Response
    - Task 模型（包含完整任务信息）
    - 所有模型支持 serde 序列化/反序列化
  - mod.rs: 模块导出
- 更新 Cargo.toml
  - 将 reqwest 从 dev-dependencies 移至 dependencies
  - 启用 json 和 rustls-tls 特性
- 添加集成测试 (`apps/runtime-agent/tests/controlplane_client_test.rs`)
  - 客户端创建测试
  - 请求序列化测试
  - 集成测试（需要运行的 Control Plane，默认 ignored）
    - 节点注册测试
    - 心跳更新测试
    - 任务 claim 超时测试
  - 所有单元测试通过

#### Phase 2.3 - 任务调度器 (2026-05-29)

- 添加任务调度器 (`apps/control-plane/internal/runtime/scheduler.go`)
  - Scheduler 结构：负责任务到节点的调度
  - SelectNode 方法：智能节点选择
    - 查询支持指定 Provider 且在线的节点
    - 过滤负载已满的节点 (current_load >= max_slots)
    - 选择负载最低的节点实现负载均衡
    - 自动更新节点 current_load
  - 错误处理：ErrNoAvailableNode
- 添加调度器测试 (`apps/control-plane/internal/runtime/scheduler_test.go`)
  - 单节点调度测试
  - 负载均衡测试（多节点选择最低负载）
  - Provider 过滤测试
  - 容量过滤测试（排除满载节点）
  - 无可用节点错误测试
  - 复杂场景测试（混合 Provider、负载、容量）
  - 11 个测试用例全部通过

#### Phase 2.2 - Runtime 服务 (2026-05-29)

- 添加 Runtime 节点管理服务 (`apps/control-plane/internal/runtime/`)
  - models.go: 领域模型定义
    - NodeStatus 枚举 (online/offline)
    - Node 模型及辅助方法 (IsOnline, HasCapacity, SupportsProvider)
    - RegisterNodeRequest, UpdateHeartbeatRequest 请求模型
    - ListNodesFilter 过滤器模型
    - pgtype 类型转换辅助函数
  - repository.go: 数据访问接口
    - CRUD 操作 (CreateNode, GetNode, ListNodes, UpdateHeartbeat, UpdateLoad, UpdateStatus, DeleteNode)
    - ListOnlineNodes: 查询心跳在阈值内的在线节点
  - service.go: 业务逻辑实现
    - RegisterNode: 注册新节点或更新已存在节点
    - UpdateHeartbeat: 更新心跳和负载，自动检测节点状态
    - GetNode: 根据 ID 查询节点
    - ListNodes: 列出节点，支持状态过滤和分页
    - ListOnlineNodes: 列出在线节点（60秒心跳阈值）
    - JSON 序列化支持 (providers, metadata)
  - service_test.go: 完整的单元测试
    - 使用 testify/mock 实现 MockRepository
    - 覆盖所有服务方法的正向和负向测试用例
    - 输入验证测试
    - 分页和限制测试
    - 15 个测试用例全部通过

#### Phase 2.1 - 任务服务 (2026-05-29)

- 添加任务管理服务 (`apps/control-plane/internal/task/`)
  - models.go: 任务领域模型
  - repository.go: 任务数据访问接口
  - state_machine.go: 任务状态机
  - service.go: 任务服务实现
  - service_test.go: 单元测试

#### Phase 1.3 - 数据层测试 (2026-05-29)

- 添加完整的数据层测试套件 (`apps/control-plane/internal/storage/queries/queries_test.go`)
  - Runtime 节点测试：创建、查询、心跳更新、在线节点列表
  - 任务测试：创建、查询、列表过滤、状态更新、状态转换、事件流
  - 认证测试：用户创建、查询、Runtime token 创建和验证
  - 审计测试：事件创建、列表查询、统计、时间过滤
- 添加 Runtime 节点查询 (`apps/control-plane/internal/storage/queries/runtime.sql`)
  - CreateRuntimeNode, GetRuntimeNode, UpdateRuntimeNodeHeartbeat
  - UpdateRuntimeNodeLoad, UpdateRuntimeNodeStatus
  - ListOnlineNodes, ListRuntimeNodes, DeleteRuntimeNode
- 添加认证查询 (`apps/control-plane/internal/storage/queries/auth.sql`)
  - CreateUser, GetUser, GetUserByUsername, GetUserByEmail
  - UpdateUser, ListUsers, DeleteUser
  - CreateRuntimeToken, GetRuntimeToken, ValidateRuntimeToken, DeleteRuntimeToken
- 添加测试辅助脚本 (`apps/control-plane/test.sh`)
  - 基于显式远端或专用测试环境变量运行 storage 查询集成测试
- 添加测试文档 (`apps/control-plane/internal/storage/queries/README.md`)
  - 测试覆盖说明
  - 运行指南
  - 故障排查
- 添加测试依赖
  - stretchr/testify (latest)

#### Phase 1.2 - 配置 sqlc (2026-05-29)

- 配置 sqlc 代码生成 (`apps/control-plane/sqlc.yaml`)
- 生成任务查询代码 (`apps/control-plane/internal/storage/queries/tasks.sql.go`)
- 生成审计查询代码 (`apps/control-plane/internal/storage/queries/audit.sql.go`)

#### Phase 1.1 - 数据库迁移 (2026-05-29)

- 初始数据库 schema (`apps/control-plane/internal/storage/migrations/001_initial.sql`)
  - Runtime 节点表 (runtime_nodes)
  - 认证表 (auth_users, auth_runtime_tokens)
  - 任务表 (tasks, task_executions, task_state_history, task_events, task_artifacts)
  - 审计表 (audit_events)
  - 索引和触发器

### Changed

#### Foundation Readiness 文档收口 (2026-05-30)

- 同步 README、开发指南、API 文档和下一步指引，明确底座阶段的启动、验证和契约守护边界。

#### Foundation 文档同步 (2026-05-29)

- 同步 README、开发指南、API 文档和下一步指引，使文档状态与已验证的 Foundation baseline 保持一致。

#### Runtime Agent daemon 默认语义 (2026-05-29)

- 将 Runtime Agent 正式运行边界收敛为受 Control Plane 管理的 daemon，并补充认证 token 配置、环境变量和 CLI 覆盖。

#### Runtime API 契约路径收敛 (2026-05-29)

- 将 Runtime 任务 claim、事件、完成、失败和 lease 续约路径统一收敛到 Control Plane 的 `/api/v1/runtime/tasks/...` canonical contract，并将 Runtime Agent 本地契约保留为诊断和本地 run API。

#### Control Plane 启动边界收敛 (2026-05-29)

- 收敛 Control Plane 主启动入口，明确 health-only router 与产品 API server 的边界，并通过统一装配路径连接存储、服务和 handlers。

- 将 Control Plane PostgreSQL 和 Redis 配置示例切换到 `doc/database/conn_info.md` 记录的远端地址，并修正连接验证命令。
- 在远端 PostgreSQL 创建 `superteam` 应用用户、数据库和 schema，并从本地 `127.0.0.1` 的 `superteam` 数据库迁移当前 schema 与迁移记录。

### Deprecated

### Removed

### Fixed

#### Control Plane 迁移命令目录对齐 (2026-05-30)

- 修正 `apps/control-plane/Makefile` 的 Atlas 迁移目录，统一指向实际 schema 源 `internal/storage/migrations`。

#### Control Plane API 响应契约对齐 (2026-05-30)

- 为任务与 Runtime 节点 API 响应补充显式 DTO，统一输出 snake_case 字段，避免直接编码领域模型时泄漏 Go 字段名。
- 将任务响应中的 `params` 规范化为 JSON object；空值、无效 JSON 或非对象输入统一回退为 `{}`，避免返回 base64 字符串。
- 更新 API/e2e 测试，锁定 `create/get/list/update/cancel/claim/complete/fail` 等任务路径及 Runtime 节点路径的真实 JSON shape。
- 收敛 Runtime claim 到 canonical `/api/v1/runtime/tasks/claim`，移除旧别名路由，并同步 API/OpenAPI 文档对 complete 与 lease 当前能力边界的描述。

#### Runtime Agent 配置入口统一 (2026-05-29)

- 统一 Runtime Agent 配置模型，支持 `--config` 加载 TOML 配置文件。
- 将配置优先级收敛为：CLI 参数 > `RUNTIME_AGENT_*` 环境变量 > `config.toml` > 默认值。
- 同步 `.env.example`、`config.example.toml`、README、配置指南和 `dev:runtime-agent` 脚本，避免 `RUNTIME_NODE_ID` / `RUNTIME_AGENT_NODE_ID` 等命名漂移。
- 忽略本地真实配置 `apps/runtime-agent/config.toml` 和 `.superteam/` 运行状态目录，保留可提交示例配置。

### Security

#### 配置文件忽略规则收敛 (2026-05-29)

- 扩展 `.gitignore` 环境配置规则，忽略项目内真实 `.env*` 和 `config.yaml` 文件，仅保留 `.env.example`、`config.example.yaml` 等示例配置可同步。

## [0.1.0] - 2026-05-29

### Added
- 项目初始化
- 基础目录结构
- CLAUDE.md 项目文档
