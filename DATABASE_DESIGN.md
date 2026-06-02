# SuperTeam 数据库设计规范

> 适用范围：SuperTeam 自有数据库 schema、SQL migration、sqlc 查询、OpenAPI 暴露的内部 ID、Web/API 测试数据。
> 本文是后续数据库表设计的长期规范；一次性重构计划、任务拆分和临时验收步骤应放在 `docs/superpowers/` 或对应 issue 中。

## 1. 设计总原则

SuperTeam 是企业级数字员工控制平面，数据库模型必须服务于多租户、多团队、分布式 Runtime、结构化协作、审计保留和后续授权演进。

- **UUID-first**：除 Atlas/sqlc/框架自身维护的迁移元数据表外，所有 SuperTeam 自有表都使用 `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`。
- **Tenant-first**：业务核心表必须以租户隔离为第一维度，包含 `tenant_id UUID NOT NULL`，租户内查询索引优先以 `tenant_id` 开头。
- **Team-aware**：团队是数字员工协作、权限和 Runtime 服务范围的组织单元。团队级对象必须包含 `team_id` 或有明确的团队绑定表。
- **Run/event 分层**：任务事实、执行尝试、事件流、工件分别建模，避免一次执行重试覆盖任务长期事实。
- **Audit-preserving**：审批、审计、工件和执行历史优先保留，避免对核心历史表使用重级联删除。
- **Registry-first**：Provider 类型、Capability 类型和外部能力类型不在业务核心里写死封闭枚举，以注册表和服务端校验为准。
- **Application-controlled first**：平台优先由应用层控制业务关系、权限、租户/团队范围、状态流转、删除语义和跨模块协作；数据库约束只作为身份、幂等、唯一性、查询性能和少量高价值完整性兜底。
- **Selective DB constraints**：每张自有表仍必须有数据库主键，但外键、级联和组合外键不是默认选项。只有当关系稳定、同模块、同生命周期，并且数据库约束明显优于应用校验时，才使用数据库 FK。

## 2. 命名规范

### 2.1 表名

- 表名使用小写 + 下划线：`snake_case`。
- 默认使用模块前缀分组：`{module}_{entity}`。
- 核心业务表可简化前缀，例如任务主表使用 `tasks`，不使用 `task_tasks`。
- 新增模块前缀前，先确认它是否属于现有边界，避免创建含义接近的并行命名。

当前模块前缀：

- `tenant_*`：租户、团队、成员、租户画像。
- `auth_*`：用户、会话、认证令牌。
- `employee_*`：数字员工定义、团队分配、配置版本、能力绑定。
- `runtime_*`：Runtime Agent 节点、槽位、租约、心跳、服务范围。
- `provider_*`：Provider 类型、adapter 注册、Provider 会话。
- `capability_*`：外部能力注册、授权、调用审计。
- `tasks` / `task_*`：任务主表、任务运行、任务事件、任务状态历史、任务产物。
- `workflow_*`：工作流模板、流程运行、Temporal 关联。
- `approval_*`：审批请求、审批决策。
- `policy_*`：策略评估、风险判断、授权判断快照。
- `audit_*`：审计事件。
- `artifact_*`：跨任务复用的工件、附件、报告索引。

### 2.2 字段名

- 主键统一命名为 `id`。
- 外键使用被引用实体名 + `_id`，例如 `tenant_id`、`team_id`、`task_id`、`runtime_node_id`。
- 时间戳统一使用 `created_at`、`updated_at`、`deleted_at`；按语义补充 `archived_at`、`disabled_at`、`cancelled_at`、`started_at`、`finished_at`、`expires_at`。
- JSON 元数据字段优先命名为 `metadata`；外部原始响应可命名为 `raw_payload`、`raw_response`。
- 状态字段命名为 `status`；类型字段命名为 `{domain}_type` 或 `type`，但不能依赖数据库封闭枚举表达可扩展类型注册。

### 2.3 注释

- 所有新增表必须有中文 `COMMENT ON TABLE`。
- 所有新增字段必须有中文 `COMMENT ON COLUMN`。
- 注释应描述业务含义和边界，不只重复字段名。

## 3. 字段类型规范

- 内部 ID：`UUID`。
- 内部外键：`UUID`。
- 时间戳：`TIMESTAMPTZ`，新增表默认包含 `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()` 和 `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`。
- JSON 数据：`JSONB`。
- 枚举/状态：`VARCHAR` + 应用层校验；只对长期稳定、低变化的基础状态增加有限 `CHECK`。
- 布尔值：`BOOLEAN NOT NULL DEFAULT false`，除非三态语义明确。
- 计数、序号、重试次数：`INTEGER` 或 `BIGINT`，但不能作为内部实体主键。
- 金额、比例和配额：按业务精度使用 `NUMERIC`，不得使用浮点数保存精确值。
- 大文件、日志正文、报告附件：不直接写入 PostgreSQL，使用 S3 兼容存储；表中只保存对象引用、大小、校验值、类型和元数据。

各 schema migration 按需启用 UUID 生成能力（项目最低 PostgreSQL 13+，`gen_random_uuid()` 已内置，无需 `pgcrypto` 扩展。pgcrypto 扩展仅当需要额外密码学函数时才启用）：

```sql
-- PostgreSQL 13+ 已内置 gen_random_uuid()，无需额外扩展
-- CREATE EXTENSION IF NOT EXISTS pgcrypto;  -- 仅在需要 pgp_sym_encrypt 等密码学函数时启用
```

所有 SuperTeam 自有表的主键形态：

```sql
id UUID PRIMARY KEY DEFAULT gen_random_uuid()
```

禁止新增 `BIGSERIAL PRIMARY KEY` 或内部 ID 的 `BIGINT` 外键。

## 4. 应用控制与数据库约束边界

### 4.1 默认立场

SuperTeam 的关系完整性默认由应用层负责。数据库保存清晰的 UUID 引用、必要索引、唯一约束和审计快照；服务层负责判断这些引用在当前租户、团队、权限、状态和工作流上下文中是否有效。

这意味着：

- 表仍然必须有 `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`，用于数据库行身份、索引和稳定引用。
- 引用字段仍使用 `*_id UUID` 命名，并按查询路径建立索引。
- 是否添加数据库 `FOREIGN KEY` 需要逐项判断，不因字段名是 `*_id` 就自动添加。
- 业务校验必须在应用层有明确代码路径和测试覆盖，不能只依赖数据库报错表达业务失败。

### 4.2 优先使用应用层控制的场景

以下场景默认不使用数据库 FK，或只保留宽松 UUID 引用与审计快照：

- 跨模块、跨 bounded context 的协作关系，例如任务引用数字员工、Runtime、Provider、Capability、审批策略。
- 审计、日志、事件、工件、执行历史等必须长期保留的历史表。
- 外部系统、Webhook、Connector、Capability invocation 等可能异步到达或最终一致的关系。
- 多态 actor/resource 引用，例如 `actor_type + actor_id`、`resource_type + resource_id`。
- 目标对象可能被禁用、归档、软删除，但历史记录仍必须可解释的关系。
- 需要由权限、租户画像、策略版本或运行态上下文共同判断有效性的关系。

这些场景应使用应用层校验、事务边界、幂等键、状态机测试、审计快照和必要索引保证一致性。

### 4.3 可以使用数据库 FK 的场景

只有同时满足以下条件时，才优先考虑数据库 FK：

- 关系处在同一模块或同一聚合内。
- 子记录不能脱离父记录独立存在。
- 父子生命周期一致，删除/归档语义简单明确。
- 数据库拒绝写入比应用层延迟校验更清晰、更可靠。
- FK 不会破坏审计保留、异步写入、导入恢复、测试夹具和模块演进。

典型可选场景包括：配置子表引用所属配置主表、同一聚合内的版本表引用主实体、短生命周期绑定表引用两端实体。即便使用 FK，也应避免对任务、审批、审计、工件和执行历史使用重级联删除。

### 4.4 必须记录 FK 决策

新增数据库 FK 或组合 FK 时，迁移或相邻文档中必须能看出选择原因：

- 为什么应用层校验不够。
- 为什么该关系生命周期稳定。
- 删除、软删除、归档时如何处理历史。
- 该 FK 是否会影响异步写入、导入恢复或跨模块演进。

## 5. ID、业务编号与外部引用

### 5.1 内部 ID

- OpenAPI 内部 ID 统一为 `type: string` + `format: uuid`。
- Go domain model 统一使用 `github.com/google/uuid.UUID`；可空 UUID 使用项目约定的 nullable UUID 类型。
- JSON 对外序列化为 UUID 字符串。
- 前端不得假设 ID 是数字，不得依赖 ID 大小排序。

### 5.2 业务编号

可读编号不作为主键。需要人类可读标识时，使用独立字段并约束作用域内唯一：

- `task_number`：租户或团队内递增编号。
- `task_key`：如 `ST-42` 的展示标识。
- `run_attempt`：同一任务下的执行尝试序号。
- `sequence_number`：同一 run 或同一任务事件流内的局部顺序。

示例：

```sql
CREATE UNIQUE INDEX uq_tasks_tenant_task_number
    ON tasks(tenant_id, task_number)
    WHERE task_number IS NOT NULL;
```

### 5.3 外部引用

外部系统 ID 不进入内部主键体系。使用独立字段保存：

- `external_system`
- `external_ref`
- `external_url`
- `external_payload`

外部引用可用唯一索引防重复，但不能替代内部 UUID。

## 6. 租户与团队规范

### 6.1 Tenant-first

除实例级配置、认证底层表、迁移元数据等少数全局表外，业务核心表必须包含：

```sql
tenant_id UUID NOT NULL
```

租户内高频查询索引必须以 `tenant_id` 开头：

```sql
CREATE INDEX idx_tasks_tenant_status_created
    ON tasks(tenant_id, status, created_at DESC);
```

### 6.2 Team-aware

团队是多数字员工管理和权限收敛的核心组织单元：

- `tenant_teams` 支持团队树或父子团队。
- 用户可以属于多个团队，并具备团队角色。
- 数字员工可以属于多个团队。
- 任务可以指定团队上下文。
- Runtime 节点可以绑定可服务的租户与团队范围。
- Capability 授权和审批策略应能落到团队维度。

### 6.3 租户一致性

全局唯一 UUID 不能单独防止跨租户误关联。默认由应用层校验租户和团队一致性：写入前必须确认被引用对象属于同一租户、同一团队范围或被当前策略允许。

对少数同模块、同生命周期、强归属关系，可以增加组合唯一约束和组合外键作为数据库兜底：

```sql
-- 示例：同模块内组合约束（如 employees 自身）
ALTER TABLE employees
    ADD CONSTRAINT uq_employees_tenant_id_id UNIQUE (tenant_id, id);

-- 以下仅为语法参考，展示组合 FK 的写法。tasks 与 employees 属于不同模块，
-- 跨模块关系默认不推荐使用组合 FK，实际项目中不要照搬此例。
-- ALTER TABLE tasks
--     ADD CONSTRAINT fk_tasks_employee_same_tenant
--     FOREIGN KEY (tenant_id, assignee_employee_id)
--     REFERENCES employees(tenant_id, id);
```

涉及任务、数字员工、团队、Runtime、Capability、审批策略的跨模块关系，默认不要直接上组合 FK；应先使用应用层授权/策略校验、事务测试和审计快照。只有在明确证明数据库组合 FK 是更好的选择时，才添加组合 FK。

## 7. 关系、删除与历史保留

- 配置子表、短生命周期绑定表可以使用 `ON DELETE CASCADE`。
- 任务、执行、审计、审批、工件、登录日志和操作日志默认不使用重 FK 依赖和级联删除；必要时使用 `RESTRICT`、`SET NULL`、软删除或纯 UUID 引用。
- 用户、数字员工、Runtime 节点被禁用或删除时，历史记录必须保留 actor 快照、UUID 引用或外部展示名。
- 核心实体优先使用 `deleted_at`、`archived_at`、`disabled_at`、`cancelled_at` 等时间戳表达生命周期。
- 审计表不能依赖目标资源仍然存在才能解释历史事件。

## 8. 索引、唯一约束与幂等

### 8.1 通用索引

- 租户列表页：`(tenant_id, status, created_at DESC)`。
- 团队列表页：`(tenant_id, team_id, status, created_at DESC)`。
- Runtime claim：根据领取策略建立 `(tenant_id, status, priority DESC, created_at ASC)` 或更窄的部分索引。
- 事件流：`(run_id, sequence_number)`。
- 审计查询：`(tenant_id, created_at DESC)`、`(tenant_id, resource_type, resource_id)`。
- Web 日志：`(created_at DESC)` 以及按用户/结果过滤需要的组合索引。

### 8.2 条件唯一索引

使用部分唯一索引表达“活跃态唯一”“未删除唯一”“同一幂等键只处理一次”：

```sql
CREATE UNIQUE INDEX uq_tasks_active_idempotency
    ON tasks(tenant_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL
      AND deleted_at IS NULL
      AND status NOT IN ('completed', 'failed', 'cancelled');
```

适用场景：

- 任务创建幂等。
- Runtime claim / lease 幂等。
- Webhook 投递去重。
- 调度触发去重。
- 审批请求去重。

### 8.3 UUID 性能

当前默认使用 `gen_random_uuid()`。它适合分布式唯一性，但不具备顺序写入优势。高频事件表必须使用 `run_id + sequence_number` 或时间字段承担排序，不依赖 UUID 顺序。后续写入量显著增长时，再评估 UUIDv7 或分区表。

## 9. 领域表设计指南

### 9.1 租户与团队

建议边界：

- `tenants`：租户主表。
- `tenant_profiles`：租户策略、语义映射、客户差异配置。
- `tenant_teams`：团队和团队层级。
- `tenant_members`：用户在租户内的成员关系。
- `tenant_team_members`：用户或数字员工在团队内的成员关系。

客户差异不得进入核心流程代码，应放入 Tenant Profile、Connector、Semantic Mapping、Capability 配置和 Policy。

### 9.2 认证与会话

建议边界：

- `auth_users`
- `auth_sessions`
- `auth_runtime_tokens`

要求：

- 用户、会话、Runtime token 行身份都使用 UUID。
- `auth_sessions.id` 是数据库身份，不是 cookie token。
- 会话 token、Runtime token、API key、Webhook secret 只保存 hash 或外部引用，禁止明文存储。
- token hash 必须有唯一索引。

### 9.3 数字员工

建议边界：

- `employees`
- `employee_team_assignments`
- `employee_config_revisions`
- `employee_capability_bindings`
- `employee_runtime_state`

要求：

- 数字员工属于租户，可被分配到多个团队。
- 数字员工配置变更必须版本化。
- 长期定义与运行态快照分离，避免心跳类字段污染定义表。
- 数字员工不是聊天机器人，应围绕任务、输入、输出、权限、上下文策略、风险等级和所属团队建模。

### 9.4 Runtime

建议边界：

- `runtime_nodes`
- `runtime_node_scopes`
- `runtime_slots`
- `runtime_leases`
- `runtime_heartbeats`

要求：

- `runtime_nodes.id` 使用 UUID。
- `node_key`、机器指纹或注册 token 只能作为唯一业务字段，不作为主键。
- Runtime claim 使用 `FOR UPDATE SKIP LOCKED` 或等价原子领取机制。
- 租约必须包含 `lease_expires_at`、`renewed_at`、`lost_at` 等字段。
- Runtime 节点只承载执行能力、健康、槽位和租约，不承载业务策略、人类审批策略和长期业务状态。

### 9.5 Provider 与 Capability

建议边界：

- `provider_registrations`
- `provider_sessions`
- `capability_registrations`
- `capability_authorizations`
- `capability_invocations`

要求：

- Provider contract 使用结构化 schema 表达输入、事件、结果、工件和错误。
- Capability Integration Layer 只负责外部能力注册、授权、HTTP 调用和审计。
- 下游工作流或 Agent 协议不可控时，SuperTeam 侧保存 endpoint、auth、sample I/O、input/output mapping、risk/approval policy、raw response 和归一化结果。

### 9.6 任务与执行

建议边界：

- `tasks`：任务主表。
- `task_runs`：一次 Runtime claim 或执行尝试。
- `task_events`：任务事件流。
- `task_state_history`：任务状态流转记录。
- `task_artifacts`：任务产物。

要求：

- `tasks` 包含 `tenant_id`、可选 `team_id`、创建者、执行策略、风险等级和状态。
- `task_runs` 表达一次执行尝试，包含 Runtime 节点、Provider 会话、租约、开始/结束、错误和结果。
- `task_events` 使用 `run_id + sequence_number` 保证局部有序。
- `task_artifacts` 只保存对象存储引用、校验值、大小、类型和元数据。
- 重试任务产生新的 `task_runs`，不得覆盖历史 run。

示例：

```sql
CREATE UNIQUE INDEX uq_task_events_run_sequence
    ON task_events(run_id, sequence_number);
```

### 9.7 审批、策略与审计

建议边界：

- `approval_requests`
- `approval_decisions`
- `policy_evaluations`
- `audit_events`
- `web_login_logs`
- `web_operation_logs`

要求：

- 人类决策是一等对象，审批请求和审批决策都要持久化。
- 审计表主键使用 UUID。
- 内部 actor/resource 优先保存 UUID；外部对象使用 `external_ref` 或 `resource_ref`。
- 高风险动作、需求歧义、权限不足、上线发布、删除写入、测试失败后的业务判断，都应能落到审批或审计记录。

## 10. Migration、sqlc 与 OpenAPI

### 10.1 Migration 策略

当前默认策略：除非用户在本次任务中明确要求 `rebuild-only`、重写初始 schema，或确认当前数据库可丢弃并重建，否则所有新增表、字段、索引、注释和数据结构变更都必须创建新的 forward migration，不得修改已存在于 `atlas.sum` 的迁移文件。

- 普通功能开发必须使用下一个编号的 forward migration，例如 `003_team_governance.sql`。
- 不得在普通功能开发中修改 `001_initial.sql` 或其他已共享迁移。
- 早期 UUID-first 基础重构允许重写初始 schema 并重建开发库，但这是例外流程，不是默认迁移策略。
- 启用 rebuild-only 例外前，必须在 plan 或 issue 中写明重建原因、备份命令、重建命令和验证命令，并确认没有需要保留的数据或已完成备份。
- 修改、删除或重排迁移文件后，必须重新生成 `atlas.sum` 并验证 Atlas 迁移状态。
- 所有新增表和字段必须同步中文注释。
- seed migration 必须适配 UUID 主键，可使用显式固定 UUID 或数据库默认生成 UUID。
- 不得在生产或不可丢数据环境执行 rebuild-only 策略。

### 10.2 生成链路

schema 变更后必须检查并按需重新生成：

- sqlc 查询模型。
- OpenAPI server/client 类型。
- Go domain model 中的 ID 类型。
- Web API client 中的 ID 类型。

`pnpm generate:control-plane` 不能替代 sqlc 生成；sqlc 变更需要执行 Control Plane 专用生成命令：

```bash
make -C apps/control-plane generate-sqlc
```

### 10.3 查询与代码约束

- SQL 查询不得新增内部 ID 的 `::bigint` 强转。
- Go handler 不得使用 `strconv.ParseInt` 解析内部 UUID 路径参数。
- OpenAPI 路径参数如 `taskId`、`userId`、`nodeId`、`runId` 必须使用 UUID 字符串。
- 前端测试数据必须使用 UUID 字符串，不使用数字 ID fixture。

## 11. 设计评审清单

新增或修改数据库表时，至少检查：

- 是否使用 `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`。
- 是否误用了 `BIGSERIAL`、内部 `BIGINT` ID 或数字型 OpenAPI ID。
- 是否试图修改已存在于 `atlas.sum` 的迁移文件；如果是，是否有用户在本次任务中明确批准 rebuild-only。
- 普通功能开发是否使用下一个编号的 forward migration，而不是回写 `001_initial.sql`。
- schema 变更是否规划或执行了 `atlas migrate hash`、`atlas migrate status`、`atlas migrate apply` 或等价验证，并完成必要的 live schema readback。
- 是否包含必要的 `tenant_id` 和团队维度。
- 关系完整性是否可以由应用层控制，并有明确代码路径和测试覆盖。
- 如果新增 FK、组合 FK 或级联删除，是否说明了为什么数据库约束是更好的选择。
- 租户内强关系是否需要组合唯一约束和组合外键；如果不需要，应用层如何校验租户一致性。
- 是否有中文表注释和字段注释。
- 是否区分了内部 UUID、业务编号和外部引用。
- token、secret、API key 是否只保存 hash 或外部引用。
- 是否保留审计、审批、工件和执行历史。
- 是否有幂等键、部分唯一索引或重复写入防护。
- 是否按查询路径设计了租户优先索引。
- 是否需要更新 sqlc、OpenAPI、Go domain model、Web client 和测试。
- 是否需要更新本文档中的长期规则。
