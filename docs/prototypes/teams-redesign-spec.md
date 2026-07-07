# 团队管理首页 · 重设计探索规范

> 本文档是 `DESIGN.md` 的补充规范，只针对"团队管理首页"的**顶部统计区、筛选区、主数据区布局**三块进行设计探索，**团队卡片本身不在本次重设计范围内（现状 GlassCard 玻璃卡视觉保持不变）**。所有色值、圆角、阴影、间距 token 必须沿用 `apps/web/src/styles/theme.css` 的 `--v3-*` 体系。

## 现状诊断

当前 `apps/web/src/features/teams/index.tsx` 的团队管理首页结构：

| 区块 | 现状 | 说明 |
| --- | --- | --- |
| 页头 | `ShellPageHeader` 标题"团队管理" + 副标题 | 标准，无需改 |
| 新建按钮 | 独立一行，`sm:self-auto` 右对齐 | 与摘要信息分离，首屏被挤占一行 |
| 筛选 | `TeamManagementToolbar`：搜索 + 状态下拉 + 治理状态下拉 + 重置 | 两个原生 Select 占纵向，移动端堆叠 |
| 卡片网格 | `TeamCardGrid`（GlassCard 玻璃卡，sm2/lg3/xl4） | **用户确认无需重设计，现状良好** |
| 摘要 | 卡片网格内部 3 个 `StatusPill`（团队/agent/可见成员） | 浮在网格上方居中，信息量有限，无治理视角 |

### 现状问题（聚焦布局与内容，不动卡片）

| 问题 | 现状 | 影响 |
| --- | --- | --- |
| 顶部无概览叙事 | 只有一行标题 + 独立新建按钮，缺团队规模/治理健康度概览 | 首屏无法一眼判断"有几个团队、治理健康吗" |
| 筛选与概览割裂 | 新建按钮独占一行，筛选用两个原生下拉 | 命令区纵向膨胀，首屏可见卡片数减少 |
| 治理视角缺失 | 摘要仅 3 个计数 pill，治理状态（生效/待批/需更新/未配置）未体现 | 首页无法引导"先看哪些团队要管" |
| 卡片与上下文孤立 | 卡片网格是封闭集合，无"待关注"导引 | 治理待办需逐卡扫读才能发现 |

## 设计目标

1. **顶部更轻、更有信息量**：用一条摘要带 / 一组 pill 状态卡 / 一个 signature 三栏 hero 替代孤立的新建按钮行，同时呈现团队规模与治理健康度。
2. **筛选更内敛**：单行命令栏或分段切换，避免两个原生下拉纵向堆叠；次级维度以 chip 表达。
3. **治理视角前置**：把"治理待办 / 需关注"作为首页一等内容，用右侧导轨、分区或顶部横条引导，而不是藏在卡片里。
4. **卡片零改动**：`TeamCard` / `GlassCard` 视觉完全沿用现状，只改变其外围布局与配套内容。
5. **断点平滑**：`lg(3) → xl(4)`，与现有 `sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4` 一致，无跨度过大的窗口。

## 真实数据源

- **API 事实源**：`apps/web/src/lib/api/teams.ts`（`TeamListItem` / `listTeamSummaries` 单接口）
- **现有实现**：`apps/web/src/features/teams/index.tsx`（`TeamsView`）、`team-card-grid.tsx`（`TeamCardGrid` + `TeamCard`）、`team-management-toolbar.tsx`
- **token 事实源**：`apps/web/src/styles/theme.css` 的 `--v3-*`
- **卡片组件**：`GlassCard` / `IconTile` / `StatusPill`（来自 `@/components/superteam`）

### 字段映射（TeamListItem 全部为真实字段）

| 业务字段 | 数据来源 | 用途 |
| --- | --- | --- |
| `name` / `slug` | `TeamListItem.name` / `slug` | 卡片标题、副标题 |
| `status` | `active` / `disabled` / `archived` | 状态筛选、非活跃识别 |
| `human_owners` / `human_owner_user_ids` | `TeamListItem` | 负责人头像与名称 |
| `digital_employee_count` | `TeamListItem` | "N 位数字员工"、代表成员计数 |
| `member_count` | `TeamListItem` | 成员总数聚合 |
| `capability_count` | `TeamListItem` | 能力绑定总数聚合 |
| `governance_status` | `not_configured` / `draft_pending` / `active` / `needs_update` | 治理健康度分区/标识 |
| `pending_draft_count` | `TeamListItem` | 待处理草案聚合 |
| `risk_summary` | `TeamListItem` | 风险摘要（原型以高中低示意） |
| `metadata` | `TeamListItem.metadata` | 卡片图标与色调（现状 `getTeamDisplayConfig`） |

### 派生指标（前端可计算，有真实来源）

| 指标 | 计算 | v3 tone |
| --- | --- | --- |
| 团队总数 | `teams.length` | brand |
| 数字员工总数 | `sum(digital_employee_count)` | info |
| 成员总数 | `sum(member_count)` | ok |
| 能力绑定总数 | `sum(capability_count)` | artifact |
| 治理生效 | `count(governance_status === 'active')` | ok |
| 需更新 | `count(governance_status === 'needs_update')` | warn |
| 草案待批准 | `count(governance_status === 'draft_pending')` | info |
| 未配置 | `count(governance_status === 'not_configured')` | mute |
| 待处理（综合） | `count(gov !== 'active' \|\| status !== 'active')` | danger |
| 待处理草案总数 | `sum(pending_draft_count)` | danger |

> **数据真实性结论**：所有首页层指标均由 `listTeamSummaries()` 单接口返回的 `TeamListItem[]` 派生，无虚构字段。原型中 9 个示例团队数据严格按 `TeamListItem` schema 构造（含 `status` / `governance_status` / `pending_draft_count` / `digital_employee_count` 等），可平移到真实接口。代表成员头像为前端占位（真实实现由 `listDigitalEmployees({team_id})` 二级接口加载，现状已如此）。

## 三种探索方向

### 方向 A · 概览驾驶舱（Linear / Vercel 风）

- **顶部**：1 条横向摘要带（团队/agent/成员/能力 4 个数字，分隔线分组，无额外卡片框，~64px 高）+ 右侧"新建团队"按钮
- **筛选**：单行命令栏，搜索占 1.4fr + 状态 chip 组（全部/活跃/已禁用/已归档）+ 治理 chip 组（草案待批/需更新/未配置）
- **主区**：左主卡片网格（沿用 `TeamCardGrid`，`sm2/lg3/xl4`）+ 右侧 300px 上下文导轨（sticky）
  - 右侧导轨"治理待办"列出 `governance_status !== 'active' || status !== 'active'` 的团队，含状态 pill 与元信息；点击高亮并滚动定位对应卡片
- **内容创新**：把治理风险从卡片里"浮出来"成持久关注列表，让首页有"该关注什么"的叙事
- **适用**：团队 8–20 个，既要扫读又要知道"哪些要管"
- **原型**：`docs/prototypes/teams-overview-cockpit.html`

### 方向 B · 治理健康分区（Stripe / Notion 风）

- **顶部**：4 张 pill 状态卡（团队总数 / agent总数 / 治理生效 / 待处理），左侧 3px accent bar
- **筛选**：搜索 + 分段切换（全部 / 活跃 / 治理待办 / 已禁用归档）
- **主区**：按治理健康度分区
  - 区块一"需要关注"：`governance_status !== 'active' || status !== 'active'` 的团队，卡片加左侧 accent bar（需更新=warn / 草案待批=info / 未配置=mute / 已禁用=danger）+ 风险标签
  - 区块二"健康运行"：`governance_status === 'active' && status === 'active'` 的团队，常态卡片
  - 每个区块有标题、计数、说明
- **内容创新**：从"一堆平等卡片"变成"先看病危的，再看健康的"，符合 `DESIGN.md`「默认安静，例外强调」
- **适用**：治理优先，团队需要按健康度分流
- **原型**：`docs/prototypes/teams-governance-sections.html`

### 方向 C · 关注优先控制台（signature hero 风）

- **顶部**：signature 三栏 hero
  - 左：团队总数大数字（brand-soft 渐变 hero 卡）
  - 中：治理分布水平堆叠条（生效/需更新/草案待批/未配置 四色 + 图例）
  - 右：待处理事项卡（danger-soft 底，列出 2 个高风险/待办团队）
- **筛选**：紧凑命令栏（搜索 + 状态/治理 chip）
- **主区**：顶部"需要关注"横条（at-risk 团队作为紧凑 mini 卡水平排列，左侧 danger accent）+ 下方全量卡片网格（沿用 `TeamCardGrid`）
- **内容创新**：把"必须处理"的团队抽成顶部关注横条优先置顶，其余正常网格；hero 一眼看清规模与治理分布
- **适用**：强调"先看清需要关注的，再处理日常"
- **原型**：`docs/prototypes/teams-attention-console.html`

## 卡片尺寸规范（三向统一，沿用现状）

| 维度 | 三向统一值 |
| --- | --- |
| 卡片类型 | `GlassCard` 玻璃卡（现状，不改） |
| 最小高度 | 260px |
| 圆角 | `--v3-r-card` 22px |
| 阴影 | `--v3-shadow` 常态，`--v3-shadow-pop` hover |
| 内部结构 | IconTile + 名称/slug + L层级 + 负责人 + 代表成员头像栈 + "查看完整部门"页脚（现状） |
| hover | 上浮 ≤ 2px |
| 网格密度 | `sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4` |

> 三向差异只在外围布局（摘要带 / pill 卡 / signature hero）与"待关注"内容的呈现方式（右侧导轨 / 分区 / 顶部横条），**团队卡片本身零改动**。

## 断点策略（三向统一）

| 断点 | 卡片列数 | 方向 A 导轨 | 方向 B 状态卡 | 方向 C hero |
| --- | --- | --- | --- | --- |
| `<760px` (mobile) | 1 | 隐藏 | 2 列 | 单列堆叠 |
| `760–1024px` | 2 | 隐藏 | 2 列 | 2 列 hero |
| `1024–1280px` (lg) | 3 | 显示 | 4 列 | 3 栏 hero |
| `1280px+` (xl) | 4 | 显示 | 4 列 | 3 栏 hero |

## 视觉语言一致性（三向共用）

- **配色**：全部使用 `--v3-*` token，主色 `#2f5fff`，语义色 ok/info/warn/danger/artifact
- **圆角**：卡片 `--v3-r-card` 22px，内层 `--v3-r-inner` 14px，pill 999px，按钮 10px
- **阴影**：`--v3-shadow` 常态，`--v3-shadow-pop` hover/浮层
- **状态色规则**：遵循 `DESIGN.md`「状态色表达语义不表达情绪」——danger 只用于待处理/高风险的导引区，不做大面积暖色背景（除 hero 的待处理卡与顶部关注横条作为有限强调）
- **工作对象界面规则**：整卡可选中（现状页脚为入口），主从详情优先，默认安静例外强调
- **中文优先**：标题、标签、按钮、状态均为简体中文

## 与现有实现的差异

| 维度 | 现状 | 三向改进 |
| --- | --- | --- |
| 顶部 | 标题 + 独立新建按钮行 | A：摘要带+按钮同排 / B：4 pill 卡 / C：signature 三栏 |
| 筛选 | 2 个原生 Select + 重置 | A/B/C：单行命令栏 / 分段 + chip |
| 治理视角 | 仅卡片内体现 | A：右侧导轨 / B：分区+accent bar / C：顶部关注横条+hero 分布 |
| 卡片 | GlassCard（现状） | **三向均零改动** |
| 摘要指标 | 3 个居中 pill | A：4 数摘要带 / B：4 pill 卡 / C：hero 大数字+分布条 |

## 禁止事项

- 不要改动 `TeamCard` / `GlassCard` 的视觉结构与样式（用户明确现状良好）
- 不要为单页引入新的色板或新的 token 体系，所有色值用 `--v3-*`
- 不要把原型内联 CSS 直接复制进生产代码，实现时必须复用 `@/components/superteam` 组件（`GlassCard` / `IconTile` / `StatusPill` / `V3Button` 等）
- 不要在筛选区堆超过 2 行筛选
- 不要用 marketing 风格的入场动画，hover 上浮幅度 ≤ 2px
- 不要在原型中编造 `TeamListItem` schema 之外的字段（如成功率、调用次数等），首页层无此数据来源
- 不要把治理待办内容从首页移除，只改变其呈现位置（导轨 / 分区 / 横条）

## 落地路径

原型文件位于 `docs/prototypes/`：

- `teams-overview-cockpit.html` — 方向 A（概览驾驶舱，Linear 风）
- `teams-governance-sections.html` — 方向 B（治理健康分区，Stripe 风）
- `teams-attention-console.html` — 方向 C（关注优先控制台，signature hero 风）
- `teams-redesign-spec.md` — 本规范

选定方向后，再回到 `apps/web/src/features/teams/index.tsx` 做真实实现：

1. 保留 `listTeamSummaries` / `listDigitalEmployees` 数据层不变
2. 复用 `ShellPageHeader` / `GlassCard` / `TeamCardGrid` / `IconTile` / `StatusPill` / `V3Button` 等项目级组件
3. 按选定方向重构顶部统计区与筛选区（摘要带 / pill 卡 / signature hero + 命令栏 / 分段）
4. 按选定方向重构"治理待办"内容的呈现（右侧导轨 / 分区 accent bar / 顶部关注横条）
5. 真实端到端验证：通过浏览器打开 `/teams` 路由，确认数据来自真实接口而非 mock

---
**UI Designer**: 像素君
**设计探索日期**: 2026-07-07
**数据真实性**: 全部指标由 `listTeamSummaries()` 单接口派生，代表成员由 `listDigitalEmployees()` 二级接口加载（现状已如此）
**实现就绪**: 选定方向后可进入 `apps/web/src/features/teams/index.tsx` 真实改造
