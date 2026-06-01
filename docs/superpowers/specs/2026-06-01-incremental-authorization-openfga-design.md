# SuperTeam 渐进式权限与 OpenFGA 演进 Spec

> 日期：2026-06-01  
> 状态：设计稿  
> 决策：先建设统一授权接口与 DB-backed MVP，后续按资源域逐步接入 OpenFGA  
> 适用范围：Control Plane 登录后的授权判断、租户/团队权限、Runtime 服务范围、Capability 授权、权限管理页面与后续 OpenFGA 迁移路径。

## 1. 背景

SuperTeam 是企业级数字员工控制平面。平台需要把 Web 控制台、Control Plane、Runtime Agent、Provider、Capability、审批、审计和工件纳入统一治理。

权限模型不能只覆盖传统后台菜单和 API，还要表达以下关系：

- 用户属于租户和团队，并在不同团队内具备不同职责。
- 数字员工属于团队，具备能力边界、上下文策略、风险等级和输出契约。
- Runtime Agent 运行在服务器节点、开发者机器或客户侧执行机，只能服务被授权的租户和团队范围。
- Capability 是一等外部能力，需要注册、授权、审批策略和调用审计。
- 任务、审批、工件和审计都需要保留 actor、resource、action、decision 和上下文快照。

项目长期目标是企业级授权，技术方向为 OpenFGA。但当前阶段仍在建设 UUID-first、tenant-first、team-aware 的基础模型，不适合一次性设计完整 OpenFGA 模型。权限能力应以可演进的方式落地。

## 2. 核心判断

登录认证和权限授权必须分开：

- **认证 AuthN**：确认用户是谁，负责密码、会话、cookie、token、账号状态和登录日志。
- **授权 AuthZ**：确认 actor 是否可以对 resource 执行 action，负责租户访问、团队权限、资源操作、Runtime 服务范围、Capability 授权和审批前置判断。

OpenFGA 不负责登录本身。登录成功后，Control Plane 从 session 中得到 `user_id`，再通过统一授权接口判断该用户是否可以访问租户、团队、页面和资源。

## 3. 目标

### 3.1 MVP 目标

- 建立统一 `Authorizer` 接口，所有服务层权限判断都通过该接口进入。
- 第一版使用 PostgreSQL 作为权限事实存储和判断后端，不强依赖 OpenFGA 服务。
- 支持最小租户、团队和控制台访问判断。
- 支持 Runtime 节点服务范围判断，避免 Runtime 领取超出授权范围的任务。
- 支持 Capability 授权判断，为后续外部能力页面和审批策略预留结构。
- 为每次关键授权判断保留可审计的 decision 结果和上下文快照。

### 3.2 演进目标

- 后续可以将 `Authorizer` 实现替换为 OpenFGA-backed 或 DB + OpenFGA 混合实现。
- 权限管理页面面向业务对象，不暴露 OpenFGA DSL、tuple 或底层 relation model。
- OpenFGA 模型按资源域逐步扩展：tenant/team -> employee/task -> runtime/capability -> artifact/approval。
- 业务代码不依赖 Casbin/OpenFGA 等具体引擎 API。

## 4. 非目标

- 不在本阶段设计完整 OpenFGA 授权模型。
- 不把登录、密码校验、session 存储交给 OpenFGA。
- 不提供面向最终用户的 OpenFGA 模型编辑器。
- 不用封闭枚举限制 Provider 类型、Capability 类型或外部能力类型。
- 不把风险审批策略混入授权引擎。授权判断和风险/审批判断保持分层。
- 不在业务 handler 中散落角色判断，例如到处写 `if role == admin`。

## 5. 方案选择

### 5.1 备选方案

**方案 A：立即完整接入 OpenFGA**

优点是长期方向明确，可以早期验证 relation model。缺点是当前资源边界还在演进，容易过早固化模型；本地开发、测试和部署也会增加一个外部依赖。

**方案 B：先用 Casbin 做平台权限**

优点是轻量，适合快速实现 RBAC/ABAC。缺点是 SuperTeam 后续需要表达租户、团队层级、数字员工代理权限、Runtime 服务范围、Capability 授权和资源继承，长期会偏离 OpenFGA 目标。

**方案 C：统一授权接口 + DB-backed MVP + OpenFGA 后置**

优点是先把业务代码和授权入口隔离，当前可以快速落地，后续能按资源域迁移到 OpenFGA。缺点是需要在接口设计阶段克制一点，避免把 DB 实现细节泄漏到业务层。

### 5.2 决策

采用方案 C。

第一阶段只实现统一授权接口和 DB-backed 权限判断。OpenFGA 是目标后端，不是第一阶段硬依赖。权限管理页面先管理业务权限配置，底层是否同步到 OpenFGA 由后端实现决定。

## 6. 架构设计

### 6.1 模块边界

新增或收敛 `apps/control-plane/internal/policy` 或 `apps/control-plane/internal/authz` 作为授权入口。推荐命名为 `authz`，避免与风险策略、审批策略的 `policy` 模块混淆。

建议边界：

- `internal/auth`：认证，负责登录、session、cookie、用户状态和登录日志。
- `internal/authz`：授权，负责 actor/action/resource 的 allow/deny 判断。
- `internal/policy`：风险策略、审批策略、执行前置策略和策略评估快照。
- `internal/audit`：审计事件和操作记录。
- `internal/tenant`：租户、团队、成员关系。
- `internal/capability`：外部能力注册、授权配置和调用审计。
- `internal/runtime`：Runtime 注册、心跳、claim、lease 和服务范围。

### 6.2 授权接口

授权接口面向业务语义，而不是面向某个引擎：

```go
type Authorizer interface {
    Check(ctx context.Context, req CheckRequest) (Decision, error)
}

type CheckRequest struct {
    Actor       ActorRef
    Action      string
    Resource    ResourceRef
    TenantID    uuid.UUID
    TeamID      *uuid.UUID
    Context     map[string]any
    AuditReason string
}

type Decision struct {
    Allowed      bool
    Reason       string
    MatchedRule   string
    RequiresAudit bool
    Snapshot      map[string]any
}
```

设计要求：

- `ActorRef` 支持 `user`、`runtime_node`、`employee`、`service_account` 等类型。
- `ResourceRef` 支持 `tenant`、`team`、`console`、`employee`、`task`、`runtime_node`、`capability`、`artifact`、`approval` 等类型。
- `Action` 使用服务端稳定字符串，例如 `tenant.access`、`team.manage`、`task.view`、`task.claim`、`capability.invoke`。
- `Context` 只放当前判断需要的运行态上下文，不能成为长期事实存储。
- `Decision.Snapshot` 用于审计，不要求和 OpenFGA 响应结构一致。

### 6.3 第一版后端

第一版实现 `DBAuthorizer`：

- 用户访问租户：查 `tenant_members`。
- 用户访问团队：查 `tenant_members`、`tenant_team_members` 或等价团队成员表。
- 控制台访问：用户必须拥有可访问租户，并处于 active 状态。
- Runtime claim 任务：Runtime 节点必须存在 active scope，scope 覆盖任务的 `tenant_id` 和 `team_id`。
- Capability 调用：查 capability 授权表，确认 actor 或 team 可以调用该 capability。

第一版可以先用少量固定角色表达：

- `owner`：租户级所有者。
- `admin`：租户或团队管理员。
- `member`：普通成员。
- `viewer`：只读成员。

角色字符串由应用层校验，不使用 PostgreSQL ENUM。

### 6.4 OpenFGA 适配点

后续新增 `OpenFGAAuthorizer`，保持 `Authorizer` 接口不变。

建议迁移策略：

- Postgres 继续作为业务配置主存储。
- 配置变更时由后端同步 relation tuple 到 OpenFGA。
- 授权判断优先通过 OpenFGA，失败时按配置决定 fail-closed 或使用只读缓存兜底。
- `policy_evaluations` 或专门授权决策表记录 OpenFGA model version、store id、tuple version 和 decision snapshot。

OpenFGA 不直接替代业务配置表。业务页面展示和编辑仍以 SuperTeam 自有模型为准。

## 7. 数据模型建议

本 spec 不展开完整 migration，但约束后续数据形态。

### 7.1 租户与团队

- `tenants`
- `tenant_profiles`
- `tenant_teams`
- `tenant_members`
- `tenant_team_members`

要求：

- 业务核心记录保留 `tenant_id`。
- 团队级对象保留 `team_id` 或独立绑定表。
- 用户可以属于多个团队。
- 团队可支持父子关系，但第一版授权判断只需支持直接成员和租户管理员继承。

### 7.2 Runtime 服务范围

- `runtime_nodes`
- `runtime_node_scopes`

要求：

- Runtime 节点可以绑定租户范围。
- Runtime 节点可以绑定团队范围。
- Runtime claim 任务前必须通过 `runtime_node -> task.tenant_id/team_id` 的授权判断。
- Runtime 不承载业务策略，只执行授权范围内的任务。

### 7.3 Capability 授权

- `capability_registry`
- `capability_authorizations`
- `capability_invocations`

要求：

- Capability 类型以注册表和服务端校验为准，不使用封闭数据库枚举。
- 授权可落到租户、团队、数字员工或服务账号。
- 高风险 Capability 调用先通过授权判断，再进入风险策略和审批策略判断。

### 7.4 授权决策审计

可复用 `policy_evaluations`，也可以后续拆出 `authz_decisions`。

必须记录：

- `tenant_id`
- `team_id`
- `actor_type`
- `actor_id`
- `action`
- `resource_type`
- `resource_id`
- `allowed`
- `reason`
- `engine`
- `engine_version`
- `snapshot`
- `created_at`

## 8. 权限管理页面

权限管理页面不直接暴露 OpenFGA 概念。页面面向业务管理员：

- 租户成员：用户、角色、状态。
- 团队成员：团队、成员、团队角色。
- Runtime 服务范围：节点、可服务租户、可服务团队、状态。
- Capability 授权：能力、授权对象、调用范围、风险等级、审批策略。
- 审计视图：授权失败、权限变更、高风险授权和 Capability 调用记录。

页面文案应使用业务语言，例如“可访问团队”“可调用能力”“需要审批”，不使用 tuple、relation、model schema 作为主要概念。

## 9. 关键流程

### 9.1 Web 登录后访问控制台

1. 用户提交登录请求。
2. `auth` 模块验证账号、密码和状态。
3. 登录成功后创建 session，写入登录日志。
4. 前端调用 `/api/auth/me`。
5. Control Plane 根据 session 得到 `user_id`。
6. 服务层调用 `Authorizer.Check(user, "console.access", tenant/console)`。
7. 允许则返回用户、租户和团队上下文；拒绝则返回 403，并记录授权失败审计。

### 9.2 Runtime claim 任务

1. Runtime 使用 node token 通过认证中间件。
2. 服务层读取候选任务的 `tenant_id` 和 `team_id`。
3. 调用 `Authorizer.Check(runtime_node, "task.claim", task)`。
4. 允许则创建 lease；拒绝则跳过该任务或返回无可领取任务。
5. 关键拒绝原因写入审计或 runtime event。

### 9.3 Capability 调用

1. 数字员工或任务请求调用 Capability。
2. Control Plane 判断 actor 是否有 `capability.invoke` 权限。
3. 授权通过后进入风险策略和审批策略。
4. 若需要审批，创建 `DecisionRequest` 或审批请求。
5. 审批通过后由 Runtime 或 connector 发起调用。
6. 调用结果、raw response 和归一化结果写入审计和工件链路。

## 10. 错误处理

- 认证失败返回 401。
- 已认证但无权限返回 403。
- 授权引擎不可用时，默认 fail-closed；只允许对明确配置的只读低风险路径使用缓存兜底。
- 授权判断缺少必要上下文时返回明确错误，不做隐式放行。
- 高风险动作授权通过后仍可能被风险策略要求人工审批。

## 11. 测试策略

### 11.1 单元测试

- `DBAuthorizer` 覆盖租户成员、团队成员、租户管理员继承、Runtime scope、Capability 授权。
- `auth` 登录测试确认登录成功不等于自动拥有所有租户权限。
- `policy` 测试确认授权和风险审批是两个独立阶段。

### 11.2 API 测试

- `/api/auth/me` 在无租户访问权限时返回 403。
- 用户只能访问所属租户和团队资源。
- Runtime 不能 claim 超出 scope 的任务。
- Capability 无授权时不能调用。

### 11.3 回归测试

- 现有登录、任务、Runtime claim、审计链路继续通过。
- 权限拒绝必须有可解释原因和审计记录。
- OpenFGA 适配后，接口层测试不需要重写，只替换 Authorizer 实现的测试夹具。

## 12. 分阶段实施计划

### 阶段一：授权边界

- 新增 `internal/authz` 模块。
- 定义 `Authorizer`、`CheckRequest`、`Decision`。
- 实现 DB-backed MVP。
- 将 `/api/auth/me`、核心租户访问和 Runtime claim 接入授权检查。

### 阶段二：权限配置

- 完善租户成员、团队成员、Runtime scope、Capability authorization 的数据模型和 API。
- Web 增加权限管理入口，先覆盖只读列表和基础编辑。
- 所有权限变更写入操作审计。

### 阶段三：资源域扩展

- 数字员工、任务、工件、审批逐步接入 `Authorizer`。
- Capability 调用纳入授权 + 风险策略 + 人类审批链路。
- 增加授权决策审计视图。

### 阶段四：OpenFGA 后端

- 引入 OpenFGA store 和 model version 配置。
- 将租户、团队、成员和资源关系同步为 tuple。
- 新增 `OpenFGAAuthorizer`。
- 小范围资源域灰度切换到 OpenFGA-backed 判断。
- 保留 DB 配置为源数据，OpenFGA 作为授权查询引擎。

## 13. 验收标准

- 业务代码只依赖 `Authorizer` 接口，不直接调用 OpenFGA、Casbin 或散落角色判断。
- 登录成功的用户如果没有租户或团队授权，不能访问受保护资源。
- Runtime Agent 不能领取超出服务范围的任务。
- Capability 调用必须先经过授权判断，再进入风险和审批策略。
- 权限管理页面表达业务权限配置，不暴露底层 OpenFGA DSL。
- 后续引入 OpenFGA 时，不需要重写 Web 页面和大部分业务服务接口。

## 14. 风险与约束

- 如果第一版 `Action` 命名过细，会导致授权判断难以维护；第一阶段应只定义核心动作。
- 如果页面直接暴露 OpenFGA 模型，业务用户理解成本会过高；必须保持业务配置视角。
- 如果授权和风险审批混在同一个模块，后续审计会难以解释；两者必须分层。
- 如果 Runtime 侧缓存授权结果，必须有过期时间和租约边界，避免权限变更后继续执行高风险任务。
- 如果 OpenFGA 同步失败，必须按动作风险等级决定 fail-closed 策略，默认不能静默放行。

## 15. 决策记录

- OpenFGA 是长期授权目标，不是第一阶段硬依赖。
- Casbin 不作为平台级主授权框架。
- 登录属于认证，OpenFGA 只参与登录后的资源授权。
- PostgreSQL 是第一版权限事实存储，后续 OpenFGA 从这些业务配置同步关系。
- 权限管理页面管理 SuperTeam 业务对象，不管理 OpenFGA 底层模型。
