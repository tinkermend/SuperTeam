# 团队管理与创建团队页面差距审计

审计时间：2026-06-04 01:40（Asia/Shanghai）

## 截图证据

- `01-team-list-current.png`：当前团队列表页
- `02-create-basic-current.png`：新建团队基础信息步骤
- `03-create-members-current.png`：新建团队初始成员步骤

本次截图使用当前代码 + Playwright mock API 数据捕获，目标对照为用户提供的团队管理页面截图和仓库根目录 `DESIGN.md`。

## 总体结论

当前实现已经完成团队管理 MVP 主链路：团队列表、状态/治理筛选、创建团队、负责人选择、初始普通成员/观察者、事务创建、列表刷新、高亮新团队、团队详情与成员角色管理基础能力。

但和目标图里的“完整组织成员管理控制台”相比，仍缺以下几类能力：

1. 人类用户展示资料不足：头像、展示名、邮箱没有统一进入 `UserSummary` / 用户选择器。
2. 团队展示资料不足：团队图标、语义类型、视觉色调目前没有 typed contract。
3. 创建团队抽屉还偏 MVP：缺步骤进度、基础信息确认卡、成员勾选表格、角色筛选、已选成员可删除/改角色。
4. 团队列表还缺分页、行级更多操作、语义图标、负责人头像。
5. 团队详情成员页已有角色后端，但 UI 仍需要从“输入用户 UUID”升级为搜索选择用户。

## 已完成能力

- 团队列表可展示名称、slug、负责人、成员数、数字员工数、能力数、治理状态、当前版本、待批准数和更新时间。
- 列表支持 `status`、`governance_status`、`q` 筛选。
- 创建团队支持：
  - 团队名称、slug。
  - 独立负责人搜索。
  - 初始成员添加为 `member` 或 `viewer`。
  - 创建时后端事务写入团队、负责人、初始成员和审计事件。
- 后端团队角色模型已有固定角色：`owner`、`admin`、`approver`、`member`、`viewer`。
- 后端限制创建时只能直接添加 `member` / `viewer`，高权限角色走申请审批。
- 团队详情成员页已有直接添加普通角色、高权限申请、待审批申请、审批/拒绝和最终负责人保护提示。

## P0/P1 功能缺口

### P0：用户资料 contract 不足，头像不能只在团队页补

当前 `auth_users` 只有 `username`、`display_name`、`email`、`status` 等字段，没有头像字段；`auth.yaml` 的 `UserSummary` 只要求 `id`、`username`、`status`。因此创建团队候选用户只能展示用户名和状态，无法像目标图一样展示头像、姓名、邮箱。

建议归属：用户管理模块。

建议切片：

1. `auth_users` 增加用户资料字段，至少包括 `avatar_url` 或 `avatar_object_key`，并明确是否由 S3/TOS 存储。
2. `UserSummary` / `/api/auth/users` 返回 `display_name`、`email`、`avatar_url`。
3. Web 增加统一 `UserIdentity` / `UserAvatar` 组件，头像缺失时用 initials fallback。
4. 团队列表、创建团队抽屉、成员页复用同一个用户身份组件。

### P1：团队图标/类型缺失

目标图里运维、研发、测试、安全团队都有不同语义图标和颜色。当前 `tenant_teams` 只有 `metadata` 可承载非结构化信息，`Team` contract 没有 `icon_key`、`team_kind`、`color_tone` 等 typed 字段。前端目前列表第一列只展示文字，没有团队图标。

建议不要把核心业务代码写死成封闭枚举。更适合的做法：

1. 短期：在 `metadata` 中约定 `display.icon_key` / `display.color_tone`，由服务端透传，前端提供安全 fallback。
2. 中期：如团队类型要参与筛选、策略或模板，再升级为服务端校验的 registry，而不是随意字符串。
3. 前端提供 `TeamIconTile` 组件，内置 `ops/dev/qa/security/default` 的视觉映射；未知 icon 使用默认团队图标。

### P1：创建团队成员选择交互仍偏 MVP

当前成员步骤是候选用户卡片 + 两个按钮；目标图是表格式勾选 + 角色下拉 + 已选成员列表。当前实现存在这些差距：

- 候选用户没有头像、展示名、邮箱。
- 没有搜索现有租户用户的输入框。
- 没有角色筛选。
- 不能在已选成员区修改角色，只能重复点击添加按钮。
- 已选成员显示的是 user_id，不是用户展示名。
- 已选成员不能删除。

后端能力基本足够，因为创建接口已接受 `initial_members[{user_id, role}]`，但前端需要维护完整候选用户对象，而不只是 user_id。

建议切片：

1. 创建抽屉成员步骤增加候选搜索与角色筛选。
2. 候选列表改成表格式：checkbox、用户身份、角色 select。
3. 已选成员表展示用户身份、角色、删除操作。
4. 保持高权限角色不可在创建时直接分配，只在 UI 上解释 owner/admin/approver 需创建后申请。

### P1：团队列表分页缺失

Control Plane `ListTeams` 已接受 `limit` / `offset`，服务层默认 `limit=50` 且上限 100；但 Web `ListTeamSummariesFilters` 还没有 `limit` / `offset`，列表页也没有分页状态和分页控件。目标图右下角分页还没实现。

建议切片：

1. Web API client 的 `ListTeamSummariesFilters` 增加 `limit` / `offset`。
2. `TeamsView` 增加页码和每页数量状态。
3. `TeamListTable` 下方增加分页区。
4. 如果需要准确总数，后端需要从数组响应升级为 `{ items, total }` 或追加 `X-Total-Count`；否则只能做弱分页。

## P2 功能与 polish 缺口

### 团队列表行级操作缺失

目标图行尾有更多操作。当前列表行只支持点击团队名称进入详情，没有 `...` 菜单。建议后续按 `allowed_actions` 展示禁用、归档、恢复、查看详情等操作；避免没有权限的动作露出。

### 创建抽屉缺步骤进度和确认卡

目标图右侧有 1/2 步骤进度和基础信息确认卡。当前抽屉只有标题和内容，没有进度指示。建议加一个轻量 stepper，并在成员步骤顶部展示基础信息摘要卡，方便创建前确认负责人、slug、状态。

### 团队详情成员页仍使用 raw UUID 输入

详情页直接添加成员和高权限申请目前要求用户输入 auth user uuid。这个和当前创建抽屉已完成的用户搜索体验不一致，也不符合控制台使用预期。建议复用用户搜索组件。

### 角色展示可解释性还可以加强

角色 label 已有，但目标图更像每行可分配角色。当前成员页的固定角色规则分散在文案、select 和后端约束里。建议沉淀 `TeamRoleBadge` / `TeamRoleSelect`，明确：

- `owner/admin/approver`：特权角色，需审批。
- `member/viewer`：直接生效角色。
- 最后一位 owner 保护是后端强约束。

### 状态色语义可更细

当前 `GovernanceStatusBadge` 只有 active 用 primary，其它都 secondary。目标图里草案待批准、已批准、已发布、草稿有不同语义色。建议用 `StatusBadge` tone 映射：active/success、draft_pending/warning、not_configured/info-neutral、needs_update/warning。

## 设计风格审计

### 符合项

- 整体仍是 `DESIGN.md` 要求的浅色液态玻璃控制台：浅色侧栏、青绿色主操作、低饱和背景、表格对比度足够。
- 首屏呈现了标题、主操作、筛选工具条和核心数据区，符合后台工具优先原则。
- 表格密度克制，行高稳定，没有营销页式大标题、大留白或装饰性光斑。
- 创建抽屉作为工作流浮层是合理的，没有把页面改成大表单。
- 主按钮、输入框、边框、抽屉 footer 基本沿用 shadcn/ui + 项目 token。

### 不足项

- 团队列表视觉识别弱：缺团队图标和语义色，行与行之间只能靠文本区分。
- 用户身份表达弱：负责人和成员没有头像/initials，人员扫描效率低。
- 创建抽屉基础步留白偏多；目标图通过 stepper 和基础信息卡填补了流程上下文。
- 成员步骤的按钮文本较长，卡片列表在候选用户多时扫描效率不如表格 + checkbox + select。
- 原生 `<select>` 和目标图里的 Radix/shadcn select 质感不一致；功能没问题，但视觉上略粗。

## 建议后续实施顺序

1. 用户身份基础：用户管理补 `display_name/email/avatar` contract，团队页只消费。
2. 团队显示元数据：约定团队 `metadata.display`，前端实现 `TeamIconTile`。
3. 创建团队抽屉升级：stepper、基础信息确认卡、成员表格勾选、角色下拉、已选成员删除。
4. 团队列表补分页和行级操作菜单。
5. 成员详情页复用用户搜索，替换 raw UUID 输入。
6. 统一角色/状态组件：`TeamRoleBadge`、`TeamRoleSelect`、治理状态 tone 映射。

