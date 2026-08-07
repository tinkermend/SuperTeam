# 租户成员与控制台访问

> 复核状态：状态：已落地（P0-P2；团队内角色变更/高权限申请UI见根TODO）

> 日期：2026-07-25  
> 状态：已落地（P0–P2；团队内角色变更/高权限申请 UI 见根 TODO）  
> 修订：有意修订 [2026-06-16 用户管理项目团队范围](2026-06-16-user-management-project-team-scope-design.md) 中「创建用户不写 membership」——改为创建时必选租户角色并同事务写入租户级 `tenant_members`。

## 1. 背景

平台存在四层正交关系：

| 关系 | 事实 | 作用 |
|------|------|------|
| 账号 | `auth_users` | 登录身份 |
| 租户成员 | `tenant_members`（`team_id IS NULL`） | `console.access` / 租户访问 |
| 团队成员 | `tenant_members`（`team_id` 非空） | 团队内角色 |
| 项目可选团队 | `user_project_team_scopes` | 创建项目时可选团队 |

此前用户管理只写账号与 scopes，无租户成员管理 API。新建用户可通过登录，但 `/api/auth/me` 因无 `console.access` 返回 403；前端误报为密码错误。Seed 仅给 `admin` 写入租户级 owner。

## 2. 目标

- 创建平台用户时**必须**选择租户角色（`owner` / `admin` / `member` / `viewer`），同事务写入租户级成员。
- 用户详情可变更或撤销租户成员（撤销后保留账号，失去控制台访问）。
- 保护最后一个活跃租户 `owner`。
- `owner` 角色仅可由现有租户 `owner` 授予。
- 权限中心 `console_access` 与 `DBAuthorizer` 闸门一致（仅租户级成员）。
- 登录错误区分：密码错误 / 账号禁用 / 无控制台访问。
- 写入路径在 OpenFGA 启用时调用既有 `SyncMembership`。

## 3. 非目标

- 多租户创建、列表、切换。
- 邀请链接、MFA、SSO、邮箱验证。
- 在权限中心做 IAM 编辑器（编辑落在用户管理）。
- 将 `console.access` 放宽为「任意团队成员即可」。
- 本 spec 不强制完成团队角色变更 UI（见计划 P1）。

## 4. API

- `POST /api/auth/users`：`tenant_role` 必填。
- `GET /api/auth/users/{id}/tenant-membership`：当前租户级成员（无则 404）。
- `PUT /api/auth/users/{id}/tenant-membership`：设置/变更角色。
- `DELETE /api/auth/users/{id}/tenant-membership`：撤销租户成员。

授权：租户 admin/owner（复用现有 tenant admin 检查）；授予 `owner` 时服务层再要求操作者为 owner。

## 5. 数据

沿用 `tenant_members`。新增部分唯一索引：同一租户下每个 principal 至多一条**活跃**租户级成员（`team_id IS NULL AND disabled_at IS NULL`）。

角色变更：更新同一行的 `role`（或先禁用再插入），并同步 OpenFGA：删除旧 relation tuple、写入新 tuple。

## 6. 与 2026-06-16 的关系

`user_project_team_scopes` 语义不变，仍不等于成员身份。创建用户 UX 一次提交同时写：

1. `auth_users`
2. 租户级 `tenant_members`（本 spec 新增）
3. 可选 `user_project_team_scopes`

文案须区分「租户角色 / 控制台访问」与「创建项目时可选择的团队」。
