# 团队管理控制台 Spec

> 日期：2026-06-03
> 状态：待书面评审
> 决策：第一版采用组织/成员管理台，团队治理、能力与知识作为团队详情的附属配置能力

## 1. 背景

SuperTeam 的团队是租户内的长期职能单元，负责承载人类成员、数字员工归属、团队能力边界、宪法、知识库、MCP、外部能力、审批策略和审计。当前项目已经具备团队基础 API、团队治理配置版本、数字员工团队归属、权限中心和 OpenFGA-ready 的统一授权入口。

本设计不把团队页做成数字员工配置页，也不把团队页做成完整权限中心。团队管理第一版的主轴是组织与成员管理：管理员可以创建团队、编辑团队基础信息、维护人类成员和固定业务角色；团队详情中展示数字员工、能力与知识、治理策略和团队管理审计。

权限底座已作为前置条件补齐：团队 API 不再复用 `runtime_scope.manage`，而是使用 `team.create`、`team.read`、`team.update`、`team.disable`、`team.archive`、`team.restore`、`team.member.*`、`team.governance.*`、`team.capability.*`、`team.audit.read` 等业务语义 action。业务代码继续只依赖 `Authorizer.Check(ctx, actor/action/resource)`，后续可替换或混合接入 OpenFGA。

## 2. 目标

- 提供 `/teams` 团队列表页，支持团队浏览、筛选、创建入口、禁用、归档和恢复。
- 提供 `/teams/:teamId` 团队详情页，支持概览、成员、数字员工、能力与知识、治理策略、审计记录 6 个聚合 tab。
- 支持两步创建团队：基础信息 + 初始成员。基础信息中指定团队负责人，初始成员阶段只直接添加普通成员和只读观察者。
- 支持从已有租户用户中添加团队人类成员并分配固定业务角色。
- 支持普通角色直接变更，高权限角色变更走申请与批准。
- 支持团队能力绑定进入治理配置草稿，批准后才生效。
- 支持团队宪法、审批策略、上下文策略、Runtime 范围、工件契约和内部协作边界的表单编辑与 JSON 快照预览。
- 支持团队详情中展示本团队数字员工，并从当前团队快速创建数字员工。
- 支持团队管理自身产生的审计记录。
- 为后续 OpenFGA relation tuple 同步保留清晰业务事实和 action/resource 边界。

## 3. 非目标

- 不实现硬删除团队。
- 不在团队页创建新租户用户或发送用户邀请。
- 不在团队页完整编辑数字员工个人能力、生效配置或执行实例。
- 不在团队页管理知识库文档入库、索引、chunk 状态或失败重试。
- 不在团队页调试 MCP 连接、外部能力 mapping 或 Skill 触发规则。
- 不把 OpenFGA DSL、tuple 或 relation model 暴露给业务用户。
- 不把任务执行审批、数字员工运行审批、高风险能力调用审批复制到团队详情页。

## 4. 页面结构

### 4.1 团队列表页

`/teams` 是高密度表格页面。顶部包含搜索、团队状态筛选、治理状态筛选和创建团队按钮。

表格列：

- 名称和 slug
- 团队状态：`active`、`disabled`、`archived`
- 团队负责人
- 成员数
- 数字员工数
- 能力数
- 治理配置状态：未配置、草稿待批准、已生效、需更新
- 当前治理版本
- 待批准草稿数
- 风险等级或审批强度摘要
- 最近更新时间
- 行操作：查看、编辑、禁用、归档、恢复

危险操作必须使用确认对话框。归档后的团队默认从列表隐藏，可通过状态筛选查看。

### 4.2 团队详情页

`/teams/:teamId` 是独立详情页，支持深链接和复杂 tab 状态。

详情页顶部展示团队名称、slug、状态、负责人、治理状态和关键操作按钮。操作按钮由后端 `allowed_actions` 控制。

详情页包含 6 个 tab：

- **概览**：管理操作优先。提供编辑团队、添加成员、创建治理草稿、处理团队待办、禁用、归档等入口，同时展示成员数、数字员工数、能力数、治理状态、待处理事项等健康摘要。
- **成员**：管理人类成员和固定团队角色，只从已有租户用户中添加。
- **数字员工**：展示归属本团队的数字员工，支持从当前团队快速创建数字员工，深度配置跳转到数字员工详情。
- **能力与知识**：展示 Skills、MCP Server、知识库、外部能力绑定清单，绑定和解绑进入治理配置草稿。
- **治理策略**：通过表单编辑结构化治理配置，并提供只读 JSON 快照预览。
- **审计记录**：展示团队管理自身产生的审计记录。

## 5. 权限与角色

团队管理继续使用统一授权接口，不在 handler、service 或页面中散落角色判断。前端可以根据 `allowed_actions` 决定按钮可见性和禁用原因，但所有写操作必须由后端再次授权。

第一版固定业务角色：

- **团队负责人**：团队最终责任人，可批准治理配置和关键组织变更。
- **团队管理员**：维护团队基础信息、普通成员和治理草稿。
- **审批人**：批准治理配置或团队本体相关审批。
- **普通成员**：参与团队协作并查看必要信息。
- **只读观察者**：查看团队、成员、配置和历史，不能修改。

已定义的团队 action 包括：

```text
team.create
team.read
team.update
team.disable
team.archive
team.restore
team.member.add
team.member.remove
team.member.change_role
team.member.request_privileged_role
team.member.approve_privileged_role
team.governance.read
team.governance.edit
team.governance.approve
team.capability.bind
team.capability.unbind
team.audit.read
```

角色语义：

- `team.create`：租户 owner/admin。
- `team.member.change_role`：租户 owner/admin 或团队 owner/admin 可调整普通角色。
- 直接提升到 owner/admin/approver 必须拒绝，并引导走特权角色申请与批准。
- `team.governance.approve`：租户 owner/admin、团队 owner、团队 approver。

后续 OpenFGA 落地时，团队成员角色、团队负责人关系、能力绑定和治理配置生效事实应作为业务主存储，并由后端同步为 OpenFGA relation tuples。页面仍展示业务角色和业务权限，不展示底层 OpenFGA 模型。

## 6. 成员与审批

成员新增第一版只支持从已有租户用户中搜索并添加。邀请、开户、账号禁用留在用户管理模块。

角色变更规则：

- 团队创建时，团队负责人由基础信息字段指定；初始成员步骤不能直接添加负责人、管理员或审批人。
- 添加或调整为普通成员、只读观察者：有权限者直接生效，并写操作审计。
- 添加或提升为团队负责人、团队管理员、审批人：生成特权角色变更申请，批准后生效。
- 降级或移除高权限角色：需要二次确认。
- 如果操作会导致团队没有负责人，后端必须阻断。

团队详情页的待处理区域只覆盖团队本体审批：

- 特权角色变更申请
- 团队负责人变更
- 治理配置草稿批准
- 禁用、归档、恢复等生命周期确认

任务执行审批、数字员工执行审批和高风险能力调用审批仍在审批中心处理。

## 7. 治理配置

团队治理配置采用草稿 + 批准生效。

- 团队管理员可以编辑草稿。
- 团队负责人、审批人或具备 `team.governance.approve` 的用户批准后，草稿成为 active 配置。
- 批准新 active 配置时，旧 active 配置归档或标记为历史版本。
- 团队能力绑定属于治理配置的一部分，不能直接改变 active 能力边界。

治理策略 tab 使用表单 + 高级 JSON 预览：

- 结构化宪法：原则、硬性规则、禁止行为、必需输出要求。
- Markdown 补充说明：团队背景、工作偏好、示例和解释性说明。
- 审批策略：风险等级、必须暂停给人类的动作、批准角色。
- 上下文策略：知识库、文档、仓库、日志、业务资料范围。
- Runtime 范围：允许的 Runtime 节点、workspace、Provider 类型和执行环境。
- 工件契约：团队要求的 `Finding`、`Risk`、`Artifact`、`DecisionRequest`、`ExecutionResult`、`NextActionProposal`、`Blocker`。
- 内部协作边界：自动问询类型、轮次、参与人数、成本和超预算升级规则。

Markdown 补充不参与自动判断是否削弱规则；后端只把结构化字段作为强治理边界。

## 8. 能力与知识

能力与知识 tab 支持绑定和解绑已有对象：

- Skills
- MCP Server
- 知识库
- 外部能力

展示粒度：

- 名称
- 类型
- 状态
- 风险等级
- 审批要求
- 绑定来源：当前草稿、当前生效配置、继承或手动绑定
- 最近更新时间
- 跳转到对应能力模块

知识库只展示集合粒度：已绑定知识库、可见范围、敏感等级、更新时间和绑定状态。不展示文档、索引或 chunk 细节。

## 9. 数字员工

团队详情的数字员工 tab 支持只读列表和从当前团队快速创建数字员工。

列表字段：

- 名称
- 角色
- 状态
- 风险等级
- 生效配置版本
- 执行实例状态
- 最近更新时间

快速创建数字员工时自动带入 `team_id`，只填写名称、角色、描述等基础字段。个人能力选择、生效配置预览、执行实例绑定和启停操作继续在数字员工详情页完成。

## 10. API 设计

采用 overview 聚合接口 + tab 细粒度接口。

### 10.1 首屏聚合接口

```text
GET /api/v1/teams/{teamId}/overview
```

返回：

- 团队基础信息
- 负责人
- 成员计数
- 数字员工计数
- 能力计数
- 当前 active 治理版本摘要
- 待批准草稿摘要
- 团队待处理事项摘要
- 当前用户 `allowed_actions`

### 10.2 团队列表与基础信息

```text
GET   /api/v1/teams?status=&q=&limit=&offset=
POST  /api/v1/teams
GET   /api/v1/teams/{teamId}
PATCH /api/v1/teams/{teamId}
POST  /api/v1/teams/{teamId}/disable
POST  /api/v1/teams/{teamId}/archive
POST  /api/v1/teams/{teamId}/restore
```

`GET /api/v1/teams` 返回治理导向字段，避免前端为每一行分别请求多个详情接口。

### 10.3 成员接口

```text
GET    /api/v1/teams/{teamId}/members
POST   /api/v1/teams/{teamId}/members
PATCH  /api/v1/teams/{teamId}/members/{memberId}
DELETE /api/v1/teams/{teamId}/members/{memberId}
POST   /api/v1/teams/{teamId}/member-role-requests
POST   /api/v1/teams/{teamId}/member-role-requests/{requestId}/approve
POST   /api/v1/teams/{teamId}/member-role-requests/{requestId}/reject
```

### 10.4 治理配置接口

```text
GET   /api/v1/teams/{teamId}/governance/current
GET   /api/v1/teams/{teamId}/governance/drafts
POST  /api/v1/teams/{teamId}/governance/drafts
PATCH /api/v1/teams/{teamId}/governance/drafts/{draftId}
POST  /api/v1/teams/{teamId}/governance/drafts/{draftId}/approve
POST  /api/v1/teams/{teamId}/governance/drafts/{draftId}/reject
```

草稿 payload 保留：

- `skill_bindings`
- `mcp_bindings`
- `knowledge_base_bindings`
- `external_capability_bindings`
- `constitution`
- `approval_policy`
- `context_policy`
- `runtime_scope_policy`
- `artifact_contract`
- `internal_collaboration_policy`

## 11. 数据模型

沿用当前：

- `tenant_teams`
- `tenant_team_config_revisions`
- `digital_employees`
- `digital_employee_config_revisions`
- `digital_employee_effective_configs`

需要新增或补齐：

- `tenant_team_members`：团队人类成员关系和固定业务角色。
- `tenant_team_member_role_requests`：高权限角色变更申请。
- 团队治理草稿复用 `tenant_team_config_revisions status=draft`，第一版不单独新增草稿表。

团队列表计数第一版优先使用查询聚合。后续如果列表性能不足，再引入物化摘要或异步统计表。

删除语义：

- `disabled_at` 表示临时停用。
- `archived_at` 表示归档并从默认列表隐藏。
- 不做硬删除。
- 历史、审计、成员快照、治理版本和任务引用必须保留。

## 12. 状态与错误处理

- 无权限：前端隐藏或禁用操作，并展示后端返回的原因；后端必须再次授权。
- 无 active 治理配置：概览突出“治理未配置”，引导创建草稿。
- 有待批准草稿：顶部和概览显示待批准状态，负责人可直接处理。
- disabled 团队：禁止新增成员、绑定能力、创建数字员工和批准新治理配置；允许恢复和查看历史。
- archived 团队：默认只读，只允许恢复。
- 最后负责人保护：移除或降级最后一个负责人必须阻断。
- 特权角色直接提升：必须拒绝，并提示走申请/批准。
- 能力绑定引用不存在、不可用、跨租户或未授权：阻断草稿批准。
- 治理草稿结构不合法：允许保存草稿时可给警告，但批准前必须阻断。
- 所有写操作必须写操作审计；关键授权判断继续写授权决策记录。

## 13. 测试范围

后端 service/API：

- 团队创建、编辑、禁用、归档、恢复。
- 团队列表返回治理导向字段。
- overview 返回摘要、计数、待处理和 `allowed_actions`。
- 成员添加、移除、普通角色变更。
- 最后负责人保护。
- 特权角色申请、批准、驳回。
- 治理草稿创建、编辑、批准、驳回。
- 能力绑定只进入治理草稿，不直接改变 active 配置。
- 授权拒绝和错误 resource。

authz：

- 新增团队 actions 的允许、拒绝、无 membership、错误 resource 和审计记录。

OpenAPI：

- 团队 overview、成员、生命周期、治理草稿、角色申请接口和响应 schema。

Web：

- 团队列表筛选和行操作。
- 两步创建团队 Drawer。
- 团队详情 tabs。
- `allowed_actions` 控制按钮状态。
- 成员表、普通角色变更和特权角色申请。
- 治理策略表单、JSON 预览和差异摘要。
- 能力与知识绑定进入草稿。
- disabled、archived、无 active 治理配置、待批准草稿等状态。
- 从团队详情快速创建数字员工，并正确带入 `team_id`。

## 14. 落地分期

### Phase 1：团队列表、详情概览与生命周期

- 扩展团队列表字段。
- 新增 overview 聚合接口。
- 实现详情页顶部和概览 tab。
- 实现编辑、禁用、归档、恢复。

### Phase 2：成员管理与特权角色申请

- 新增或补齐团队成员关系。
- 实现成员 tab。
- 实现普通角色直接变更。
- 实现负责人、管理员、审批人变更申请与批准。

### Phase 3：治理草稿与能力知识绑定

- 实现治理策略表单和 JSON 预览。
- 实现治理草稿保存、差异摘要、批准和驳回。
- 实现 Skills、MCP、知识库、外部能力绑定进入草稿。

### Phase 4：数字员工入口与团队审计

- 实现数字员工 tab。
- 支持从当前团队快速创建数字员工。
- 实现团队管理审计 tab。

### Phase 5：视觉方案图与样式收敛

- 基于本 spec 生成 2-3 张候选页面视觉图。
- 对比团队列表、详情概览、成员 tab 和治理策略 tab 的布局。
- 确认最终 UI 方向后再进入详细实现计划。
