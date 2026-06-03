# 团队管理体验缺口补齐设计

日期：2026-06-04 01:50（Asia/Shanghai）

## 背景

团队管理已经完成 MVP 主链路：团队列表、状态与治理筛选、新建团队、负责人选择、初始普通成员/观察者、事务创建、列表刷新、高亮新团队、团队详情与成员角色管理基础能力。

当前缺口主要来自目标界面和当前实现之间的产品体验差异：

1. 用户身份展示不足：头像、展示名、邮箱没有形成可复用展示组件。
2. 团队展示信息不足：团队图标、团队语义色、运维/研发/测试/安全等视觉识别还没有稳定约定。
3. 创建团队抽屉仍偏 MVP：缺 stepper、基础信息确认卡、成员勾选表、角色筛选、已选成员可删除/改角色。
4. 团队列表缺弱分页、团队图标、负责人身份展示和行级导航菜单。
5. 团队详情成员页仍要求输入 raw user UUID。
6. 状态、角色、用户身份等 UI 模式还没有沉淀成团队管理复用组件。

本设计采用分阶段总设计，第一阶段只做团队管理体验补齐，不实现用户头像上传和完整分页总数。

## 目标

第一阶段目标是把团队管理从 MVP 页面提升到可日常使用的组织/成员管理控制台：

- 团队列表具备团队图标、负责人身份展示、弱分页和行级查看详情入口。
- 新建团队抽屉具备可确认、可扫描、可撤销的两步创建体验。
- 团队详情成员页不再要求用户手输 UUID，而是复用用户搜索选择。
- 用户身份、团队图标、团队角色和团队状态组件可复用。
- 设计风格继续符合 `DESIGN.md` 的浅色液态玻璃企业控制台方向。

## 非目标

第一阶段不做以下事情：

- 不实现头像上传、头像对象存储、用户资料编辑。
- 不在团队管理任务中修改用户头像存储方案；头像字段由用户管理任务提供。
- 不把团队类型做成强枚举或独立 registry。
- 不把团队列表响应升级为 `{ items, total }`。
- 不实现准确总数分页。
- 不在团队列表行级菜单里直接执行禁用、归档、恢复等生命周期动作。
- 不替代现有团队详情页中的治理、成员审批、审计等主链路。

## 已确认决策

- 总体方案采用分阶段增强。
- 用户头像能力作为用户管理模块外部依赖；团队管理只消费 `UserSummary` / `UserIdentity`，并提供 initials fallback。
- 团队图标和色调第一阶段使用 `metadata.display.icon_key/color_tone` 约定。
- 创建团队抽屉第一阶段贴近目标图：stepper、基础信息确认卡、候选成员搜索、角色筛选、checkbox 选择、已选成员表、删除/改角色。
- 列表分页第一阶段使用弱分页：`limit/offset` + 上一页/下一页 + 每页数量，不显示准确总数。
- 团队详情成员页第一阶段纳入：直接添加和高权限申请都改为用户搜索选择。
- 列表行级菜单第一阶段只做安全导航：查看详情。生命周期动作继续放在详情页，由 `TeamOverview.allowed_actions` 控制。

## 外部依赖与冲突规避

用户管理头像能力正在独立会话中推进。本设计不抢占该工作范围：

- 不新增或修改头像上传接口。
- 不决定头像文件存储位置。
- 不实现头像裁剪、删除、替换或权限策略。
- 团队管理只定义消费模型和 fallback。

如果用户管理先完成 `avatar_url/display_name/email` contract，团队管理在实现时直接接入；如果尚未完成，团队管理仍以 `username` 和 initials fallback 交付第一阶段体验。

## 阶段划分

### Phase 1：团队管理体验补齐

交付范围：

- 团队列表：团队图标、负责人身份、弱分页、查看详情行菜单。
- 创建团队：完整两步抽屉交互。
- 成员详情页：用户搜索替代 raw UUID 输入。
- 组件层：沉淀用户身份、团队图标、角色和角色选择组件。
- 数据策略：使用 `metadata.display`，不新增团队表 typed 字段。

### Phase 2：用户身份真实接入

由用户管理模块提供头像和资料字段后，团队管理消费：

- `avatar_url`
- `display_name`
- `email`

团队页不负责头像上传或用户资料生命周期。

### Phase 3：完整分页与深链接扩展

后续如果需要准确页码和总数，再升级列表响应为 `{ items, total }` 或等价协议。行级菜单可扩展到成员管理、治理草稿、审计记录、数字员工跳转等深链接。

## API 与数据设计

### 用户身份消费模型

前端定义消费型用户身份数据：

```ts
type UserIdentityData = {
  id: string
  username: string
  status?: "active" | "disabled" | string
  display_name?: string | null
  email?: string | null
  avatar_url?: string | null
}
```

降级规则：

1. 主名称优先级：`display_name` -> `username` -> `email` -> `id`。
2. 次级信息优先级：`email` -> `username` -> `id`。
3. 头像优先级：`avatar_url` -> initials。
4. `avatar_url` 加载失败时回退 initials，不显示 broken image。

第一阶段不要求后端立即提供 `avatar_url`。如果另一个用户管理会话已经扩展 `UserSummary`，团队管理直接消费；如果没有扩展，组件仍能用 `id/username/status` 工作。

### 团队展示元数据

继续使用 `tenant_teams.metadata`，约定：

```json
{
  "display": {
    "icon_key": "ops",
    "color_tone": "cyan"
  }
}
```

前端支持的第一阶段 key：

| icon_key | 语义 | 建议图标 | 默认色调 |
| --- | --- | --- | --- |
| `ops` | 运维团队 | `ServerCog` | cyan |
| `dev` | 研发团队 | `Code2` | blue |
| `qa` | 测试团队 | `FlaskConical` | violet |
| `security` | 安全团队 | `Shield` | teal |
| unknown | 默认团队 | `UsersRound` | neutral |

服务端不把这些 key 作为封闭业务枚举。未知 key 不报错，前端 fallback。

建议服务端轻校验：

- `metadata` 必须是 JSON object。
- `metadata.display` 如果存在，必须是 object。
- `display.icon_key` / `display.color_tone` 如果存在，必须是长度不超过 40 的字符串。

### 团队列表弱分页

Web API client 扩展：

```ts
type ListTeamSummariesFilters = {
  status?: TeamStatus
  governance_status?: GovernanceSummaryStatus
  q?: string
  limit?: number
  offset?: number
}
```

交互规则：

- `pageSize` 默认 20。
- `offset = pageIndex * pageSize`。
- 上一页：`pageIndex > 0` 时启用。
- 下一页：`items.length === pageSize` 时启用。
- 不显示准确总数，只显示当前页序号和每页数量。
- 筛选条件变化时重置到第一页。

后端现有 `limit/offset`、默认 limit 和上限逻辑保留。

### 创建团队请求

沿用现有请求：

```ts
type CreateTeamInput = {
  slug: string
  name: string
  human_owner_user_id: string
  initial_members?: Array<{ user_id: string; role: "member" | "viewer" }>
  status?: TeamStatus
  metadata?: Record<string, unknown>
}
```

创建抽屉提交时写入：

```json
{
  "metadata": {
    "display": {
      "icon_key": "security",
      "color_tone": "teal"
    }
  }
}
```

如果用户没有显式选择团队图标，则前端按 slug/name 推断默认 display 并提交；例如 `ops` -> `ops/cyan`，`security` -> `security/teal`，无法推断时使用 `default/neutral`。

## 前端组件设计

### `UserIdentity`

位置建议：`apps/web/src/components/superteam/user-identity.tsx`

职责：

- 统一展示人类用户身份。
- 支持紧凑模式和完整模式。
- 支持头像 URL 和 initials fallback。
- 供团队列表、创建团队抽屉、团队详情成员页复用。

接口建议：

```ts
type UserIdentityProps = {
  user: UserIdentityData
  size?: "sm" | "md"
  showSecondary?: boolean
}
```

### `TeamIconTile`

位置建议：`apps/web/src/components/superteam/team-icon-tile.tsx`

职责：

- 根据 `metadata.display` 渲染团队语义图标。
- 支持列表行和基础信息确认卡。
- 未知 icon/tone 使用默认图标和 neutral tone。

### `TeamRoleBadge`

位置建议：`apps/web/src/features/teams/components/team-role-badge.tsx` 或 `components/superteam`。

职责：

- 固定角色中文展示。
- 角色语义：
  - `owner`：负责人
  - `admin`：管理员
  - `approver`：审批人
  - `member`：普通成员
  - `viewer`：只读观察者

如果跨多个页面复用超过团队模块，再迁移到 `components/superteam`。

### `TeamRoleSelect`

职责：

- 角色选择器，避免每个页面手写 select。
- 支持模式：
  - `direct`：只允许 `member` / `viewer`。
  - `privileged`：只允许 `owner` / `admin` / `approver`。

第一阶段可以基于 shadcn/Radix Select；如果当前 registry 组件缺失，先加 shadcn Select 后使用。

### `UserSearchSelect`

职责：

- 搜索 active 用户。
- 展示 `UserIdentity`。
- 返回完整用户对象。
- 创建团队负责人选择、初始成员选择、详情页直接添加、详情页高权限申请共用。

参数建议：

```ts
type UserSearchSelectProps = {
  apiBaseUrl: string
  fetcher?: typeof fetch
  value?: UserIdentityData
  excludedUserIds?: string[]
  onSelect: (user: UserIdentityData) => void
  placeholder?: string
}
```

行为：

- 输入 q 后调用 `/api/auth/users?q=&status=active&limit=20&offset=0`。
- 空输入可展示前 20 个 active 用户。
- `excludedUserIds` 中的用户禁用或隐藏。
- 加载、错误、空态都有内联反馈。

## 页面设计

### 团队列表页

变更点：

- 第一列改为 `TeamIconTile + 团队名称/slug/status`。
- 负责人列改为 `UserIdentity` compact 模式。
- 治理状态用语义 tone：
  - `active`：success
  - `draft_pending`：warning
  - `not_configured`：info-neutral
  - `needs_update`：warning
- 增加分页 footer：
  - 上一页
  - 当前页
  - 下一页
  - 每页数量
- 行尾增加 `MoreHorizontal` 菜单，第一阶段只包含“查看详情”。

不在列表行里直接禁用、归档、恢复。原因：列表响应当前没有 per-row `allowed_actions`，生命周期动作继续在详情页执行，避免没有权限时露出错误动作。

### 创建团队抽屉

抽屉仍为两步。

Step 1：基础信息

- 团队名称。
- 团队 slug。
- 团队图标/色调选择。
- 负责人搜索选择。
- 校验：名称、slug、负责人必填。

Step 2：初始成员

- 顶部基础信息确认卡：
  - 团队图标
  - 名称
  - slug
  - 负责人
  - 状态
- 信息提示：负责人、管理员、审批人需创建后发起特权角色申请。
- 候选成员区：
  - 搜索用户。
  - 角色筛选：全部、普通成员、只读观察者。
  - 表格式候选列表：checkbox、用户身份、角色 select。
  - 负责人不可作为初始成员。
- 已选成员区：
  - 用户身份。
  - 分配角色。
  - 删除操作。
  - 可修改角色。

提交 payload：

- `human_owner_user_id` 来自负责人。
- `initial_members` 来自已选成员表。
- `metadata.display` 来自图标选择或推断。

### 团队详情成员页

变更点：

- `直接添加` panel：
  - `UserSearchSelect`
  - `TeamRoleSelect(mode="direct")`
  - 提交 `user_id` 和 `role`
- `高权限申请` panel：
  - `UserSearchSelect`
  - `TeamRoleSelect(mode="privileged")`
  - `reason`
  - 提交 `target_user_id`、`requested_role`、`reason`
- 名册行使用 `UserIdentity`。
- 最终负责人保护 alert 保留。
- 待审批卡如果后端仍只返回 `target_user_id`，先显示 ID；后续可扩展 role request response 带 target user summary。

## 权限边界

- 创建团队仍走 `team.create`。
- 创建时负责人自动写入 owner。
- 创建时初始成员只允许 `member` / `viewer`。
- 详情页直接添加走 `team.member.add`，只允许 `member` / `viewer`。
- 高权限申请走 `team.member.request_privileged_role`，只允许 `owner` / `admin` / `approver`。
- 审批走 `team.member.approve_privileged_role`。
- 最后一位负责人保护仍由后端强约束。
- 用户搜索仍走现有 `/api/auth/users` 权限，不在团队页绕过用户管理授权。

## 错误处理

- 用户搜索失败：候选区显示内联错误，不清空已选内容。
- 创建团队失败：抽屉保持打开，表单和已选成员保留，在 footer 上方显示错误。
- 弱分页失败：列表区域显示错误，筛选工具条保留。
- `metadata.display` 缺失或未知：使用默认团队图标和 neutral tone。
- 头像加载失败：降级 initials。
- 详情页添加成员失败：对应 panel 显示错误，表单内容保留。
- 特权角色在创建时不可选，后端仍兜底拒绝非法 role。

## 测试策略

### 前端单元与组件测试

- `UserIdentity`
  - 有 `avatar_url`。
  - `avatar_url` 缺失时 initials fallback。
  - 有 `display_name/email`。
  - 只有 `username`。
- `TeamIconTile`
  - 已知 icon key。
  - 未知 icon key。
  - 缺 `metadata.display`。
- `TeamRoleSelect`
  - `direct` 模式只出现 `member/viewer`。
  - `privileged` 模式只出现 `owner/admin/approver`。

### 前端页面测试

- 创建团队抽屉：
  - stepper 校验基础信息。
  - 搜索并选择负责人。
  - 选择团队图标/色调。
  - 搜索候选成员。
  - checkbox 选择成员。
  - 修改已选成员角色。
  - 删除已选成员。
  - submit payload 包含 `metadata.display` 和 `initial_members`。
- 团队列表：
  - 请求包含 `limit/offset`。
  - 筛选变化重置第一页。
  - 上一页/下一页禁用状态正确。
  - 图标、负责人身份、治理状态 tone 渲染。
  - 行菜单只有查看详情。
- 成员详情页：
  - 直接添加通过用户搜索提交 `user_id`。
  - 高权限申请通过用户搜索提交 `target_user_id`。
  - direct/privileged role 选项被限制。

### 后端测试

- 创建团队接受并返回 `metadata.display`。
- `metadata.display` 非 object 时返回 invalid input。
- `display.icon_key/color_tone` 超长时返回 invalid input。
- 创建团队仍拒绝：
  - 重复初始成员。
  - 负责人作为初始成员。
  - 初始成员使用 `owner/admin/approver`。
- 团队列表 `limit/offset` 行为保持默认和上限。

### 视觉验收

- 团队列表首屏保留标题、主操作、筛选工具条和核心数据。
- 表格行高稳定，不因图标、头像、菜单造成跳动。
- 创建抽屉按钮区不被内容挤压，长候选列表只滚动内容区。
- 人员、角色、治理状态在视觉上可扫描。
- 页面不出现装饰性 orb、重渐变或低对比文本。

## 验收标准

第一阶段完成时应满足：

- 团队列表可显示团队图标、负责人身份、弱分页和查看详情菜单。
- 创建团队可完成目标图中的主要交互：两步流程、基础确认、候选成员勾选、角色分配、已选成员修改和删除。
- 团队详情成员页不再要求手输 raw UUID。
- 头像字段缺失时页面仍专业可用。
- 不要求真实头像上传。
- 不要求准确总数分页。
- 不新增团队类型强枚举。
- 现有团队详情、治理、成员审批和审计链路不回退。

## 实施顺序建议

1. 补前端类型和复用组件：`UserIdentity`、`TeamIconTile`、`TeamRoleBadge`、`TeamRoleSelect`、`UserSearchSelect`。
2. 补团队列表弱分页、图标、负责人身份、查看详情菜单。
3. 升级创建团队抽屉交互和 payload。
4. 升级团队详情成员页用户搜索。
5. 补后端 metadata display 轻校验和测试。
6. 补视觉 smoke 截图和回归测试。

## 后续待设计项

- 用户管理头像上传、存储、裁剪和头像权限策略。
- 用户资料字段正式进入 `UserSummary` 的 contract 收敛。
- 团队展示 registry 是否需要服务端注册表。
- 准确分页总数响应协议。
- 团队列表生命周期动作是否需要 per-row `allowed_actions`。
