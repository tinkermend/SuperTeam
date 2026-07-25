# 用户管理可选团队授权设计

> **修订（2026-07-25）**：创建用户现须同时写入租户级成员（`tenant_role` → `tenant_members`，`team_id IS NULL`）以授予控制台访问。`user_project_team_scopes` 语义不变，仍不等于成员身份。详见 [2026-07-25-tenant-membership-and-console-access](2026-07-25-tenant-membership-and-console-access.md)。


日期：2026-06-16
状态：已确认，待实现计划

## 1. 背景

SuperTeam 中存在两类参与者：

- 人类用户是平台账号，负责管理、审核、审批、创建项目和做业务判断。
- 数字员工是 agent 型执行身份，归属于虚拟团队，并在项目中作为执行能力被选择。

用户管理页创建的是人类平台用户，不是数字员工。创建人类用户时选择团队，不表示这个用户加入团队，也不表示给这个用户分配团队角色；它表示项目发起授权边界：这个人类用户创建项目时，可以选择哪些虚拟团队。

## 2. 目标

- 重设计用户管理中的创建人类用户流程。
- 保持最小账号字段：用户名、名称、密码、头像。
- 把“头像种子”输入改成从现有头像库选择头像。
- 把“可调用团队员工池”改成“选择可选团队”。
- 把可选团队保存成明确的用户-团队授权关系。
- 在创建项目时强制执行该授权关系：用户只能选择自己被授权的团队。
- 不在此流程中引入团队成员身份、团队角色、MFA 或邀请链接。

## 3. 非目标

本轮不做：

- 不在用户管理页创建或修改数字员工。
- 不分配团队 owner、admin、member、viewer、approver 等成员角色。
- 不增加 MFA、邀请链接、邮件验证或临时密码邀请状态。
- 不把用户 metadata 作为可选团队的事实源。
- 不只依赖前端过滤来保证创建项目授权。

## 4. 用户体验设计

用户管理页保持现有 Console 结构：左侧用户列表，中间用户详情，右侧创建用户抽屉。页面副标题建议改为：

```text
人类用户账号、项目发起范围、可选团队与登录审计
```

### 4.1 创建用户抽屉

抽屉分三段。

#### 4.1.1 账号信息

字段：

- 用户名：登录账号。
- 名称：中文姓名或展示名称。
- 密码：管理员直接设置的初始密码。
- 头像：从现有头像资产库中网格选择。

抽屉中不得出现“发送邀请链接”“MFA”“Authenticator”“临时密码”“头像种子”等字段或文案。密码字段 label 使用“密码”，不使用“临时密码”。

#### 4.1.2 选择可选团队

该区域列出当前已经创建的团队，支持多选。

每行展示：

- 团队名称和 slug。
- 团队负责人。
- 数字员工数量。
- 治理状态。
- 当前配置版本或风险摘要。

区域说明文案要直接表达授权含义：

```text
该用户创建项目时，只能从已选择的团队中选择项目团队范围。
```

建议控件：

- 团队搜索。
- 选择或清空当前筛选结果。
- 已选数量。
- 禁用或归档团队标识。

中文 UI 统一使用“选择可选团队”或“可选团队”，不得使用“可调用团队员工池”。

#### 4.1.3 预检与创建

预检项：

- 用户名已填写且唯一。
- 名称已填写。
- 密码满足后端策略。
- 已选择头像。
- 至少选择一个可选团队。
- 已选团队仍存在且可使用。

主按钮为“创建用户”。创建成功后关闭抽屉，选中刚创建的用户，并在用户详情中展示其可选团队。

### 4.2 用户详情

当前“团队与角色”页签改成“可选团队”。这样避免让用户误以为创建人类用户时同时分配了团队成员身份。

该页签展示：

- 已授权团队列表。
- 每个团队的负责人和数字员工摘要。
- 编辑可选团队入口。
- 没有授权团队时的审计友好空态。

团队角色调整仍保留在团队管理页，不在这里操作。

### 4.3 项目创建

创建项目页只能展示当前人类用户被授权选择的团队。如果用户没有任何可选团队，项目创建页显示：

```text
你还没有可用于创建项目的团队，请联系管理员授权。
```

前端按授权过滤团队选择器以提升体验；后端仍是最终授权判断来源。

## 5. 数据模型

新增一张一等授权关系表：

```text
user_project_team_scopes
```

字段：

- `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`
- `tenant_id UUID NOT NULL`
- `user_id UUID NOT NULL`
- `team_id UUID NOT NULL`
- `status VARCHAR(50) NOT NULL DEFAULT 'active'`
- `granted_by_user_id UUID`
- `revoked_at TIMESTAMPTZ`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`
- `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`

约束和索引：

- `(tenant_id, user_id, team_id)` 在 `status = 'active'` 时唯一。
- 查询索引 `(tenant_id, user_id, status)`。
- 查询索引 `(tenant_id, team_id, status)`。
- `status` 限定为 `active` 或 `revoked`。

服务层校验引用的用户和团队必须属于同一租户。该关系不是 `tenant_members`，也不是 `tenant_team_members`；它不授予团队成员身份或团队权限。

同时给 `auth_users` 增加：

```text
avatar_asset_id VARCHAR(100)
```

该字段保存人类用户选择的头像资产 ID。现有 DiceBear 头像字段可以保留做兼容，但创建用户流程应保存并渲染 `avatar_asset_id`。

## 6. API 设计

创建用户请求扩展为：

```json
{
  "username": "zhoumin",
  "display_name": "周敏",
  "password": "initial-password",
  "avatar_asset_id": "engineer-f-01",
  "selectable_team_ids": ["team-id-1", "team-id-2"]
}
```

新增读取和替换可选团队授权的接口：

```text
GET /api/auth/users/{id}/project-team-scopes
PUT /api/auth/users/{id}/project-team-scopes
```

scope 响应直接嵌入用户详情页需要的团队摘要字段：

- team ID
- slug
- name
- status
- owner summaries
- digital employee count
- governance status
- current revision
- risk summary

这样用户管理页切换选中用户时，不需要再额外对全量团队列表做本地 join。

创建项目接口必须校验：

- 请求中的 `team_id` 属于当前 `ActorUserID` 的 active 可选团队。
- 请求中所有 `principal_type = "team"` 的 `ProjectMemberInput` 都属于当前 `ActorUserID` 的 active 可选团队。
- 禁用、归档、删除或不存在的团队即使被旧前端提交，也必须被后端拒绝。

授权失败时返回明确错误：

```text
当前用户无权使用该团队创建项目。
```

## 7. 头像处理

UI 复用现有头像库资产。人类用户创建时选择头像资产 ID，而不是输入自由文本种子。

用户创建 UI 不再暴露 DiceBear seed。如果现有用户头像响应仍为了兼容返回 DiceBear descriptor，用户管理流程仍以 `avatar_asset_id` 作为人类可见的头像事实来源。

## 8. 错误处理

创建用户抽屉错误：

- 用户名重复：显示在用户名字段下。
- 密码不合规：显示在密码字段下。
- 未选择头像：显示在头像区域。
- 未选择团队：禁用创建按钮，并提示“至少选择一个可选团队”。
- 团队已不可用：显示在“选择可选团队”区域，并刷新团队列表。

项目创建错误：

- 没有任何可选团队：提交前显示空态。
- 提交了未授权团队：后端返回授权错误，前端展示在团队选择区域。
- 团队后来被禁用或归档：刷新后从可选项中移除，并要求用户重新选择。

## 9. 测试与验收

单元和集成测试覆盖：

- 创建用户能保存 `display_name`、`avatar_asset_id` 和 `selectable_team_ids`。
- 用户详情能读取和更新可选团队。
- 替换 scope 时，移除的团队被撤销，新增的团队被激活，不产生重复 active 行。
- 使用已授权团队创建项目成功。
- 使用未授权、禁用、归档、删除或不存在的团队创建项目失败。
- 用户创建抽屉不再渲染“发送邀请链接”“MFA”“临时密码”“头像种子”“可调用团队员工池”等文案。
- 项目创建页只展示当前用户被授权的团队。

真实链路验收：

- 启动真实 Web 和 Control Plane 服务。
- 在用户管理页创建一个人类用户。
- 从现有头像库选择头像。
- 选择多个可选团队。
- 使用该用户登录或以该用户身份创建项目。
- 打开项目创建页，确认只显示被授权团队。
- 使用已授权团队提交项目并确认成功。
- 通过 API 提交未授权团队并确认后端拒绝。

## 10. 实现备注

- 当前项目服务类型已经包含 `ActorUserID`，后端可以在创建项目服务层做授权校验。
- 用户可选团队 scope 响应嵌入用户管理页所需的团队摘要字段。
- 数据库变更必须使用新的 forward migration，不得修改已经纳入 Atlas 的历史 migration。
