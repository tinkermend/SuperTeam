# SuperTeam UUID-first 分布式库表重构 Spec

> 日期：2026-06-01
> 状态：设计稿
> 决策：重写初始 schema，早期环境直接重建库
> 参考边界：参考 `/Users/wangpei/src/github/agentic/paperclip/docs/database-design-analysis.md` 中的数据库设计原则，不复用其具体表名与字段。

## 1. 背景

SuperTeam 的定位是企业级数字员工控制平面。后续系统需要支持多实例 Control Plane、分布式 Runtime Agent、多租户、多团队、多数字员工协作、审批、审计、工件和执行链路追踪。

当前 Control Plane 初始 schema 仍然以 `BIGSERIAL` / `BIGINT` 为主键和外键基础。这个设计适合早期单实例 MVP，但不适合分布式扩展：

- 自增 ID 依赖单库序列，跨实例、跨环境合并和异步写入时不够自然。
- API、sqlc 生成模型和 Go domain model 已经深度绑定 `int64`。
- 会话表使用 `VARCHAR` 作为主键，令牌标识和数据库身份混杂。
- 业务表尚未系统性引入租户、团队、数字员工归属字段，不利于多团队 Agent 管理和未来行级隔离。
- 删除策略中存在较多级联删除，对任务历史、审计、工件保留不友好。

本 spec 目标是在早期阶段直接重写初始 schema，形成 UUID-first、tenant-first、team-aware 的分布式基础模型。

## 2. 目标

### 2.1 主键目标

除 Atlas/sqlc/框架自身维护的迁移元数据表外，所有 SuperTeam 自有表均使用统一主键形态：

```sql
id UUID PRIMARY KEY DEFAULT gen_random_uuid()
```

约定：

- 所有 SuperTeam 自有表都必须有 `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`。
- 所有主键列统一命名为 `id`。
- 所有内部外键使用 `UUID`。
- 项目最低 PostgreSQL 13+，`gen_random_uuid()` 已内置；除非后续需要额外密码学函数，不为 UUID 生成启用 `pgcrypto`：

```sql
-- no pgcrypto required for gen_random_uuid() on PostgreSQL 13+
```

- `gen_random_uuid()` 是本项目 SQL migration 中对应 `defaultRandom()` 目标的实现方式。
- ID 不承载排序、编号或业务含义；排序使用 `created_at`、`sequence_number`、业务编号或专用时间字段。

### 2.2 分布式目标

- 多个 Control Plane 实例可以并发写入，不需要协调自增序列。
- Runtime Agent 可以部署在服务器节点、开发者机器或客户侧执行机，通过 claim + lease 领取任务。
- 高频事件不依赖自增主键排序，使用 `run_id + sequence_number` 或 `task_id + sequence_number` 保证局部顺序。
- 所有可重试写入具备幂等键或唯一约束，避免网络重试造成重复任务、重复事件或重复审批。

### 2.3 多租户与多团队目标

- 所有业务核心表必须具备 `tenant_id`，团队级对象具备 `team_id`。
- 租户是数据隔离的第一维度，团队是数字员工协作和权限管理的第二维度。
- 数字员工不是聊天机器人，应围绕任务、输入、输出、权限、上下文策略、风险等级和所属团队建模。
- 用户、数字员工、Runtime 节点、Provider、Capability 通过注册与绑定关系协作，而不是在任务表中散落自由文本。

## 3. 非目标

- 本次不设计完整 OpenFGA 授权模型，只保留统一授权接口和可迁移的数据形态。
- 本次不引入 PostgreSQL ENUM，状态字段继续使用 `VARCHAR` + 应用层校验，必要时可对稳定基础状态增加有限 `CHECK`。
- 本次不做兼容旧库的数据迁移。早期环境允许重建数据库。
- 本次不把客户差异写入核心流程表，客户差异仍进入 Tenant Profile、Connector、Semantic Mapping、Capability 配置和 Policy。

## 4. 当前问题清单

### 4.1 BIGSERIAL 主键

当前以下核心表仍使用 `BIGSERIAL PRIMARY KEY`：

- `runtime_nodes`
- `auth_users`
- `auth_runtime_tokens`
- `tasks`
- `task_executions`
- `task_state_history`
- `task_events`
- `task_artifacts`
- `audit_events`
- `web_login_logs`
- `web_operation_logs`

这些表需要改为 UUID 主键。

### 4.2 BIGINT 外键

当前以下外键链路仍使用 `BIGINT`：

- `auth_sessions.user_id`
- `tasks.creator_id`
- `task_executions.task_id`
- `task_state_history.task_id`
- `task_events.task_id`
- `task_events.execution_id`
- `task_artifacts.task_id`
- `task_artifacts.execution_id`
- `web_login_logs.user_id`
- `web_operation_logs.user_id`

这些外键需要随主键一起改为 UUID。

### 4.3 API 与代码契约绑定 int64

当前 OpenAPI 中 `TaskId`、`Task.id`、`creator_id`、用户 ID 等仍是 `integer/int64`。Go handler 中也存在 `strconv.ParseInt` 解析路径 ID 的逻辑。sqlc 生成模型大量使用 `int64` / `pgtype.Int8`。

目标态应统一为：

- OpenAPI：`type: string`, `format: uuid`
- Go domain：统一使用 `github.com/google/uuid.UUID`
- sqlc：通过 overrides 将 PostgreSQL `uuid` 映射到 Go UUID 类型
- JSON：对外序列化为 UUID 字符串

### 4.4 业务身份与令牌身份混杂

`auth_sessions.id` 当前是 `VARCHAR(255) PRIMARY KEY`。目标态应拆开：

- `auth_sessions.id`：数据库身份，UUID 主键。
- `session_token_hash`：唯一令牌哈希，禁止明文存储。
- cookie 中的 token 不是数据库主键。

Runtime token、API key、Webhook secret 等同理：数据库行身份使用 UUID，外部令牌只保存 hash 或外部引用。

## 5. 目标设计原则

### 5.1 UUID-first

所有表都有 UUID 主键。即使是多对多关系表，也保留 `id UUID PRIMARY KEY`，再用唯一索引约束业务组合。

示例：

```sql
CREATE TABLE employee_team_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    employee_id UUID NOT NULL,
    team_id UUID NOT NULL,
    role VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, employee_id, team_id)
);
```

### 5.2 Tenant-first

除实例级配置、认证底层表等少数全局表外，业务表必须包含 `tenant_id UUID NOT NULL`。

所有租户内查询索引优先以 `tenant_id` 开头：

```sql
CREATE INDEX idx_tasks_tenant_status_created
    ON tasks(tenant_id, status, created_at DESC);
```

### 5.3 Team-aware

团队是多数字员工管理的核心组织单元。任务、数字员工、审批策略、Capability 授权、Runtime 访问范围都应能落到团队维度。

设计要求：

- `tenant_teams` 支持团队树或父子团队。
- 数字员工可以属于多个团队。
- 用户可以属于多个团队，并具备团队角色。
- 任务可以指定团队上下文。
- Runtime 节点可以绑定可服务的租户与团队范围。

### 5.4 Composite FK 保证租户一致性

仅有全局唯一 UUID 还不足以防止跨租户误关联。对租户内强关系，目标 schema 应增加组合唯一约束并使用组合外键：

```sql
ALTER TABLE employees
    ADD CONSTRAINT uq_employees_tenant_id_id UNIQUE (tenant_id, id);

ALTER TABLE tasks
    ADD CONSTRAINT fk_tasks_employee_same_tenant
    FOREIGN KEY (tenant_id, assignee_employee_id)
    REFERENCES employees(tenant_id, id);
```

这样可以在数据库层兜底：任务不能引用另一个租户的数字员工。

### 5.5 软删除与审计优先

核心实体默认使用 `deleted_at`、`archived_at`、`disabled_at`、`cancelled_at` 等时间戳保留历史。避免对任务、审计、工件、执行记录使用重级联删除。

建议：

- 配置子表、短生命周期绑定表可使用 `ON DELETE CASCADE`。
- 任务、执行、审计、工件、审批默认 `RESTRICT` 或 `SET NULL`。
- 删除用户或数字员工时，历史记录保留 actor 快照和 UUID 引用。

### 5.6 条件唯一索引用于活跃唯一与幂等

使用部分唯一索引表达“活跃态唯一”“未删除唯一”“同一幂等键只处理一次”：

```sql
CREATE UNIQUE INDEX uq_tasks_active_idempotency
    ON tasks(tenant_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL
      AND deleted_at IS NULL
      AND status NOT IN ('completed', 'failed', 'cancelled');
```

这类约束适合任务创建、Runtime claim、Webhook 投递、调度触发、审批请求等分布式重复写入场景。

## 6. 目标表域划分

以下是目标域，不要求一次实现所有字段，但初始 schema 应按这些域建立正确骨架。

### 6.1 租户与团队域

建议表：

- `tenants`：租户主表。
- `tenant_profiles`：租户策略、语义映射、客户差异配置。
- `tenant_teams`：团队与团队层级。
- `tenant_members`：用户在租户内的成员关系。
- `tenant_team_members`：用户或数字员工在团队内的成员关系。

关键要求：

- 主键全部 UUID。
- 团队表包含 `tenant_id`。
- 团队自引用使用 `parent_team_id UUID`。
- 成员关系使用 `principal_type + principal_id`，其中内部主体 ID 使用 UUID。
- 对活跃成员建立唯一部分索引，允许历史成员关系保留。

### 6.2 认证与会话域

建议表：

- `auth_users`
- `auth_sessions`
- `auth_runtime_tokens`

关键要求：

- `auth_users.id UUID PRIMARY KEY DEFAULT gen_random_uuid()`。
- `auth_sessions.id UUID PRIMARY KEY DEFAULT gen_random_uuid()`。
- `auth_sessions.user_id UUID NOT NULL REFERENCES auth_users(id)`。
- 会话 token 只存 hash，并有唯一索引。
- Runtime token 行身份使用 UUID；节点身份引用 `runtime_nodes.id` 或使用注册阶段的外部 key。

### 6.3 数字员工域

建议表：

- `employees`：数字员工定义。
- `employee_team_assignments`：数字员工与团队绑定。
- `employee_config_revisions`：数字员工配置版本。
- `employee_capability_bindings`：数字员工可调用能力绑定。
- `employee_runtime_state`：数字员工运行态快照。

关键要求：

- 数字员工属于租户，可被分配到多个团队。
- 数字员工配置变更必须版本化。
- 运行态快照和长期定义分离，避免心跳类字段污染定义表。
- Provider 类型不要写死成封闭枚举，以注册表和服务端校验为准。

### 6.4 Runtime 域

建议表：

- `runtime_nodes`：Runtime Agent 节点注册。
- `runtime_node_scopes`：Runtime 节点可服务的租户和团队范围。
- `runtime_slots`：节点执行槽位。
- `runtime_leases`：任务或运行的租约记录。
- `runtime_heartbeats`：节点心跳历史或聚合记录。

关键要求：

- `runtime_nodes.id` 为 UUID。
- `node_key`、机器指纹或注册 token 只能作为唯一业务字段，不作为主键。
- Runtime claim 使用 `FOR UPDATE SKIP LOCKED` 或等价原子领取机制。
- 租约必须包含 `lease_expires_at`、`renewed_at`、`lost_at` 等字段。
- Runtime 节点不要承载业务策略，只承载执行能力、健康、槽位和租约。

### 6.5 Provider 与 Capability 域

建议表：

- `provider_registrations`：Provider 类型与 adapter 注册。
- `provider_sessions`：Provider 会话。
- `capability_registrations`：外部能力注册。
- `capability_authorizations`：能力授权。
- `capability_invocations`：外部能力调用审计。

关键要求：

- Provider contract 使用结构化 schema 表达输入、事件、结果、工件和错误。
- Capability Integration Layer 只负责注册、授权、HTTP 调用和审计。
- 业务核心不依赖封闭枚举判断 Provider 或 Capability 类型。

### 6.6 任务与执行域

建议表：

- `tasks`：任务主表。
- `task_runs`：一次 Runtime claim 或执行尝试。
- `task_events`：任务事件流。
- `task_state_history`：任务状态流转记录。
- `task_artifacts`：任务产物。

关键要求：

- `tasks.id`、`task_runs.id`、`task_events.id` 均为 UUID。
- `tasks` 包含 `tenant_id`、可选 `team_id`、创建者、执行策略、风险等级、状态。
- `task_runs` 表达一次执行尝试，包含 Runtime 节点、Provider 会话、租约、开始/结束、错误、结果。
- `task_events` 使用 `run_id + sequence_number` 保证局部有序：

```sql
CREATE UNIQUE INDEX uq_task_events_run_sequence
    ON task_events(run_id, sequence_number);
```

- `task_artifacts` 只存对象存储引用、校验值、大小、类型和元数据，不把大文件放进 PostgreSQL。
- `tasks` 与 `task_runs` 分离，避免多次重试覆盖任务事实。

### 6.7 审批、策略与审计域

建议表：

- `approval_requests`
- `approval_decisions`
- `policy_evaluations`
- `audit_events`
- `web_login_logs`
- `web_operation_logs`

关键要求：

- 人类决策是一等对象，审批请求和审批决策都要持久化。
- 审计表主键 UUID，`actor_id` 和 `resource_id` 对内部对象优先使用 UUID。
- 对外部系统对象使用 `external_ref` 或 `resource_ref`，不要混进内部 UUID 字段。
- 高风险动作、需求歧义、权限不足、上线发布、删除写入、测试失败后的业务判断都应能落到审批或审计记录。

## 7. ID、编号和外部引用规范

### 7.1 内部 ID

- 内部实体 ID：UUID。
- 内部 FK：UUID。
- OpenAPI：`type: string`, `format: uuid`。
- Go：`uuid.UUID`。
- JSON：UUID 字符串。

### 7.2 业务编号

可读编号不作为主键。示例：

- `task_number`：租户或团队内递增。
- `task_key`：如 `ST-42` 这类展示标识。
- `run_attempt`：同一任务下的执行尝试序号。

业务编号应通过唯一约束保证作用域内唯一：

```sql
CREATE UNIQUE INDEX uq_tasks_tenant_task_number
    ON tasks(tenant_id, task_number)
    WHERE task_number IS NOT NULL;
```

### 7.3 外部引用

外部系统 ID 不进入内部主键体系：

- `external_system`
- `external_ref`
- `external_url`
- `external_payload`

外部引用使用唯一索引防重复，但不替代内部 UUID。

## 8. Schema 重写策略

本项目选择“重写初始 schema，早期环境直接重建库”。

### 8.1 迁移文件策略

目标：

- 将当前 `001_initial.sql` 改写为 UUID-first 初始 schema。
- 将 `auth_sessions`、Web 登录日志、操作日志、中文注释等早期补丁纳入新的初始 schema。
- 保留或重写 seed migration，但 seed 必须适配 UUID 主键。
- 删除或重排仅为兼容旧 `001_initial.sql` 的 forward-only 迁移，避免新环境创建重复表。

建议迁移结构：

```text
apps/control-plane/internal/storage/migrations/
  001_initial.sql                 # UUID-first 完整初始 schema
  002_seed_dev_admin.sql          # UUID 版开发管理员种子
```

如果仍需单独维护注释，可使用：

```text
003_comments.sql
```

但所有新增表和字段必须有中文表注释与字段注释。

### 8.2 环境重建策略

因为当前处于早期阶段，本次不提供旧库数据迁移。执行实施时需要明确：

- 本地开发库可以 drop/recreate。
- 远端开发库可以 drop/recreate，但执行前必须确认没有需要保留的数据。
- 文档中必须给出重建命令和验证命令。
- 不能在生产或不可丢数据环境执行本策略。

### 8.3 Atlas/sqlc/OpenAPI 生成链路

重写 schema 后必须重新生成：

- sqlc 查询模型。
- OpenAPI server/client 类型。
- Go domain model 中的 ID 类型。
- Web API client 中的 ID 类型。

现有 `pnpm generate:control-plane` 不能替代 sqlc 生成。sqlc 仍需执行 Control Plane 专用生成命令。

## 9. 代码影响范围

### 9.1 SQL 查询

所有查询中的 `::bigint`、`pgtype.Int8`、`BIGINT` 参数需要替换为 UUID。

示例方向：

```sql
WHERE id = sqlc.arg('id')::uuid
```

列表过滤中的 `creator_id`、`task_id`、`execution_id`、`user_id` 等都应同步修改。

### 9.2 Go domain model

目标：

```go
type Task struct {
    ID       uuid.UUID
    TenantID uuid.UUID
    TeamID   *uuid.UUID
}
```

路径参数解析应从 `strconv.ParseInt` 改为 UUID 解析：

```go
id, err := uuid.Parse(idStr)
```

### 9.3 OpenAPI

所有内部 ID schema 统一：

```yaml
type: string
format: uuid
```

路径参数如 `taskId`、`userId`、`nodeId`、`runId` 都应同步更新。

### 9.4 前端

前端不应假设 ID 是数字：

- 表格 row id 使用字符串。
- URL 参数使用 UUID 字符串。
- 测试数据从数字 ID 改为 UUID。
- 排序不依赖 ID 大小。

## 10. 索引策略

### 10.1 通用索引规则

- 租户内查询索引以 `tenant_id` 开头。
- 团队内查询索引以 `(tenant_id, team_id, ...)` 开头。
- 状态列表页使用 `(tenant_id, status, created_at DESC)`。
- Runtime claim 使用 `(tenant_id, status, priority DESC, created_at ASC)` 或适合 claim 的部分索引。
- 事件流使用 `(run_id, sequence_number)`。
- 审计使用 `(tenant_id, created_at DESC)` 与 `(tenant_id, resource_type, resource_id)`。

### 10.2 UUID 性能注意事项

`gen_random_uuid()` 默认生成随机 UUID。它带来分布式唯一性，但不具备顺序写入优势。

当前阶段接受这个取舍。后续如果事件或任务写入量显著增长，可以评估 UUIDv7 或时间分区，但本次不引入额外复杂度。

## 11. 验收标准

### 11.1 Schema 验收

- `001_initial.sql` 中不存在 `BIGSERIAL PRIMARY KEY`。
- 所有业务表主键均为 `UUID PRIMARY KEY DEFAULT gen_random_uuid()`。
- 所有内部 FK 均为 UUID。
- `auth_sessions.id` 为 UUID，session token hash 不再作为主键。
- 新增表和字段都有中文 `COMMENT ON TABLE` 和 `COMMENT ON COLUMN`。
- 核心租户内表包含 `tenant_id`。
- 关键跨租户关系使用组合 FK 保证同租户引用。

### 11.2 代码生成验收

- sqlc 生成模型不再把内部 ID 生成为 `int64`。
- OpenAPI 生成类型不再把内部 ID 暴露为 `integer/int64`。
- Go handler 不再使用 `strconv.ParseInt` 解析内部 ID。
- 前端 API client 和测试不再假设 ID 是数字。

### 11.3 分布式行为验收

- Runtime claim 支持并发领取，不会重复领取同一任务。
- Runtime lease 到期后可被重新领取或恢复。
- `task_events` 支持幂等追加，同一 run 下事件顺序稳定。
- 重试任务会产生新的 `task_runs`，不会覆盖历史 run。
- 审计、审批、工件记录在任务删除或用户禁用后仍可保留。

### 11.4 验证命令

实施完成后至少运行：

```bash
make -C apps/control-plane generate-sqlc
pnpm generate:control-plane
go test ./...
pnpm test
```

并对重建后的数据库执行 schema 检查：

```sql
SELECT table_name, column_name, data_type, column_default
FROM information_schema.columns
WHERE table_schema = current_schema()
  AND column_name = 'id'
ORDER BY table_name;
```

所有业务表的 `id` 应为 `uuid`，默认值应为 `gen_random_uuid()`。

## 12. 实施顺序建议

1. 重写 `001_initial.sql`，建立 UUID-first 基础 schema。
2. 合并早期 auth session、Web log、中文注释迁移到新的初始 schema。
3. 改写 seed migration，使用显式 UUID 或让数据库默认生成 UUID。
4. 更新 sqlc query SQL，将所有内部 ID 参数改为 UUID。
5. 更新 sqlc 配置，将 UUID 映射到 Go UUID 类型。
6. 重新生成 sqlc 代码。
7. 更新 Go domain model、repository、handler、service 和测试。
8. 更新 OpenAPI 契约，将内部 ID 改成 `string/uuid`。
9. 重新生成 OpenAPI server/client。
10. 更新 Web API client、页面与测试数据。
11. 重建本地开发库，执行端到端最小链路验证。
12. 更新 `CHANGELOG.md` 和数据库连接/重建说明。

## 13. 风险与处理

### 13.1 重建库会丢失数据

本方案明确只适用于早期环境。执行前必须确认本地和远端开发库无保留价值，或先导出备份。

### 13.2 改动面大

ID 类型贯穿 SQL、Go、OpenAPI、前端和测试。实施时应按“schema -> sqlc -> Go -> OpenAPI -> Web -> 测试”顺序推进，避免同时修改所有层后难以定位问题。

### 13.3 UUID 随机写入性能

短期可接受。高频事件表通过 `run_id + sequence_number` 和时间字段承担排序，不依赖 UUID 顺序。

### 13.4 租户一致性容易漏

仅添加 `tenant_id` 不够。关键关系必须用组合 FK 和索引兜底，防止任务引用其他租户的数字员工、团队、Runtime 或 Capability。

## 14. 最终决策

SuperTeam 的数据库基础模型应从现在开始切换为：

- UUID-first：所有内部主键和外键使用 UUID。
- Tenant-first：所有业务核心表以租户隔离为第一原则。
- Team-aware：数字员工、任务、权限、Runtime 能力范围都支持团队维度。
- Run/event 分层：任务事实、执行尝试、事件流、工件分别建模。
- Audit-preserving：审批、审计、工件和执行历史优先保留。
- Rebuild-only：早期环境直接重建库，不做旧 BIGSERIAL 数据迁移。
