# 技能管理首页 · 重设计探索规范

> 本文档是 `DESIGN.md` 的补充规范，只针对"技能市场首页"的顶部统计区、筛选区、主数据区三块进行设计探索，不替换 v3 Soft-Flat 主基线。所有色值、圆角、阴影、间距 token 必须沿用 `apps/web/src/styles/theme.css` 的 `--v3-*` 体系。

## 现状诊断

当前 `apps/web/src/features/skills/index.tsx` 存在以下视觉问题：

| 问题 | 现状 | 影响 |
| --- | --- | --- |
| 顶部指标过于松散 | 6 个 `V3MetricCard` 在 `xl:grid-cols-6` 下每张卡占满一列，单数字单图标 | 视觉重量过大，挤压主区域，6 列在 1280px 每张仅 ~190px 却仍是大卡 |
| 工具栏筛选项多 | 4 个 `FilterSelect`（风险/范围/依赖/状态）+ 搜索 + 视图切换，移动端堆叠为 2 行 | 命令栏高度膨胀，首屏可见数据减少 |
| 表格列宽偏宽 | 7 列 `min-w-[300px]/[180px]/[220px]` 等，单行横向占用大 | 1280px 以下易横向滚动或挤压描述列 |
| 网格视图密度低 | `md:grid-cols-2 / xl:grid-cols-3`，卡片偏宽 | 1024-1280px 区间只有 2-3 列，扫读效率低 |
| 安装记录固定底部 | `SelectedSkillInstallations` 始终占主区下方纵向空间 | 主数据区被压缩，需上下滚动才能看完表格与安装记录 |
| 风险/状态语义混杂 | 表格行用 `tone="danger"` accent bar，但可绑定/已绑定/需审批三态视觉区分度可加强 | 高风险需审批项不够醒目 |

## 设计目标

1. **顶部更轻**：统计区从 6 张大卡降为 1 条紧凑摘要带或 4 张 pill 状态卡，高度 ≤ 80px。
2. **筛选更内敛**：主筛选 1 行（搜索 + 高频维度），次级维度折叠到「更多」或边栏。
3. **密度可调**：提供"表格扫读"与"卡片概览"两种节奏，4 列为目标密度（lg+ 显示 4 列）。
4. **安装记录让位**：默认不占主区纵向空间，改为抽屉或详情面板常驻，把空间还给主数据。
5. **高风险可识别**：需审批项通过左侧 accent bar + danger soft 底 + 待审批聚合卡三重强调。
6. **断点平滑**：`md(2) → lg(3/4) → xl(4) → 2xl(4 更宽)`，无 2 列跨度过大的窗口。

## 真实数据源

- **API 事实源**：`apps/web/src/lib/api/skills.ts`
- **现有实现**：`apps/web/src/features/skills/index.tsx`（`SkillsView`）
- **token 事实源**：`apps/web/src/styles/theme.css` 的 `--v3-*`

### 字段映射（Skill 类型）

| 业务字段 | 数据来源 | 派生方式 |
| --- | --- | --- |
| 技能名称 / 描述 / 版本 | `Skill.name` / `description` / `version` | 直接字段 |
| 来源标识 | `Skill.source` | 直接字段 |
| 风险等级 | `Skill.risk_level` | `normalizeRisk()` → high/medium/low |
| 图标 / 颜色 | `Skill.icon_key` / `color_token` | `iconMap` + `toneByColor` 映射到 v3 tone |
| 标签 | `Skill.tags[]` | 直接字段 |
| 团队绑定 | `Skill.team_bindings[]` | `length` 计数 + 列表 |
| 数字员工绑定 | `Skill.agent_bindings[]` | `length` 计数 + 列表 |
| 运行依赖 | `Skill.runtime_dependencies.{tools[],env[]}` | `tools.length + env.length` |
| 创建人 / 时间 | `Skill.created_by_name` / `created_at` | 直接字段 |
| 安装记录 | `listSkillInstallations(skillId)` → `SkillInstallation[]` | 二级接口，选中后加载 |

### 派生指标（前端可计算，有真实来源）

| 指标 | 计算 | v3 tone |
| --- | --- | --- |
| 技能总数 | `skills.length` | brand |
| 可绑定 | `statusDisplay === "available"`（无绑定 + 非高风险） | info |
| 已绑定 | `statusDisplay === "installed"`（有 team 或 agent bindings） | ok |
| 需审批 | `needsApproval`（`risk_level === "high"`） | danger |
| 有运行依赖 | `runtimeDependencyCount > 0` | info |
| 团队绑定总数 | `sum(team_bindings.length)` | info |
| 数字员工绑定总数 | `sum(agent_bindings.length)` | artifact |

### SkillInstallation 字段

- `provider_type`：`opencode` / `codex` / `claude-code` → 映射 brand / brand / artifact tone
- `installed_path` / `runtime_node_id` / `node_id`：等宽字体展示
- `employee_name` / `digital_employee_id`：安装目标
- `installed_at` / `archive_checksum_sha256`：时间与校验

> **数据真实性结论**：所有首页层指标均由 `listSkills()` 单接口返回的 `Skill[]` 派生，无虚构字段。安装记录是选中后的二级接口 `listSkillInstallations()`，不参与首页层聚合指标。原型中的 9 个示例技能数据均严格按 `Skill` schema 构造，可平移到真实接口。

## 三种探索方向

### 方向 A · 高密度目录表（Linear / Vercel 风）

- **顶部**：1 条横向摘要带（6 个数字 + 图标，无卡片框，靠分隔线分组，高度 ~64px）
- **筛选**：单行命令栏，搜索占 1.4fr + 4 个 chip 式 Select + 视图切换分段
- **主区**：高密度表格（行高 ~52px），7 列紧凑布局，sticky 表头，hover 行高亮
- **强调**：需审批行左侧 3px danger accent bar；有运行依赖行内嵌 info 依赖标记
- **安装记录**：点击行 → 右侧 460px 抽屉滑出（不挤压表格），含基础信息 / 运行依赖 / 安装记录
- **适用**：技能数量 20+ 的高密度场景，扫读优先，企业级工具感
- **原型**：`docs/prototypes/skills-dense-table.html`

### 方向 B · 能力矩阵（Stripe / Notion 风）

- **顶部**：4 张 pill 式状态卡（左侧 3px accent bar + icon + 大数字 + 标签，紧凑横排）
- **筛选**：搜索突出（左 1.4fr）+ chip 多选筛选（全部/已绑定/可绑定/需审批 + 风险/依赖/排序折叠）
- **主区**：4 列中等卡片网格（~244px 宽 × 220px 高），每张卡是独立"小型仪表盘"
  - 左侧 3px 状态 accent bar（ok/info/danger）
  - 图标 + 名称 + 版本/来源
  - 描述（2 行截断）
  - 风险 pill + 状态 pill + 依赖 pill
  - 标签 chip
  - 底部：团队/员工绑定计数 + 详情/安装按钮
- **交互**：点击卡片选中（Cmd/Shift 多选），顶部出现快捷操作条（批量安装/取消），双击进详情抽屉
- **适用**：兼顾密度与信息量，强调每个技能的"能力画像"，概览优先
- **原型**：`docs/prototypes/skills-capability-matrix.html`

### 方向 C · 治理工作台（主从详情风）

- **顶部**：signature 强调区（三栏）
  - 左：技能总数大数字 + 副标题（brand-soft 渐变 hero 卡）
  - 中：风险等级分布水平堆叠条（低/中/高三色，含图例）
  - 右：需审批高亮卡（danger-soft 底 + 列出 2 个高风险技能名）
- **主区**：三栏工作台（208px 筛选边栏 + 340px 列表 + flex 详情面板）
  - 左：筛选边栏（搜索 + 风险/状态/范围/依赖复选 + 标签 chip 云），多维度组合筛选
  - 中：紧凑列表行（图标 + 名称 + 版本 + 绑定数 + 风险/状态 pill），选中高亮，danger 行 accent bar
  - 右：详情面板常驻（头卡 + 4 Tab：概览/安装记录/运行依赖/绑定范围）
- **交互**：列表与详情面板稳定共存，无需抽屉打断；筛选边栏实时联动列表
- **适用**：治理优先，强调"理解-决策-动作"工作流，适合审批、合规、绑定治理场景
- **原型**：`docs/prototypes/skills-governance-workbench.html`

## 卡片/表格尺寸规范

| 维度 | 方向 A（表格） | 方向 B（卡片） | 方向 C（列表+详情） |
| --- | --- | --- | --- |
| 主区单元高度 | 行高 ~52px | 卡片 ~220px | 列表行 ~56px |
| 单元宽度（4列@1280） | 表格自适应 | ~244px | 列表 340px + 详情 flex |
| 内边距 | 单元格 9px 14px | 16px | 列表 10px 14px / 详情 18px 20px |
| 图标尺寸 | 34px tile | 38px tile | 32px tile |
| 圆角 | 表头 `--r-inner` | `--r-card` 22px | `--r-card` 22px |
| 阴影 | `--shadow` | `--shadow` + hover `--shadow-pop` | `--shadow` |
| 选中态 | brand-soft 行底 | brand border 2px + brand-soft 底 | brand-soft 行底 + 左 3px brand bar |
| 危险态 | 左 3px danger bar | 左 3px danger bar + danger pill | 左 3px danger bar + danger pill |

## 断点策略（三向统一）

| 断点 | 方向 A 表格列 | 方向 B 卡片列数 | 方向 C 布局 |
| --- | --- | --- | --- |
| `<760px` (mobile) | 隐藏标签/绑定列 | 1 列 | 单栏堆叠，侧栏隐藏 |
| `760-1024px` | 隐藏标签/绑定列 | 2 列 | 单栏，筛选边栏降序 |
| `1024-1280px` (lg) | 全列 | 3 列 | 三栏（边栏 188 + 列表 320 + 详情） |
| `1280px+` (xl) | 全列 | 4 列 | 三栏（208 + 340 + flex） |
| `1536px+` (2xl) | 全列 | 4 列更宽 | 三栏更宽 |

## 视觉语言一致性（三向共用）

- **配色**：全部使用 `--v3-*` token，主色 `#2f5fff`，语义色 ok/info/warn/danger/artifact
- **圆角**：卡片 `--v3-r-card` 22px，内层 `--v3-r-inner` 14px，pill 999px，按钮 10px
- **阴影**：`--v3-shadow` 常态，`--v3-shadow-pop` hover/浮层
- **字体**：Inter 主字体，JetBrains Mono 用于版本/路径/ID/checksum
- **状态色规则**：遵循 DESIGN.md「状态色表达语义不表达情绪」——danger 只用于需审批/高风险，不做大面积暖色背景（除 signature 待审批聚合卡）
- **工作对象界面规则**：整行/整卡可选中，主从详情优先，默认安静例外强调

## 与现有实现的差异

| 维度 | 现状 | 三向改进 |
| --- | --- | --- |
| 顶部统计 | 6 张大 `V3MetricCard`（`xl:grid-cols-6`） | A：1 条摘要带 / B：4 张 pill 卡 / C：signature 三栏 |
| 筛选 | 4 个 `FilterSelect` + 搜索，移动端 2 行 | A：单行命令栏 / B：搜索 + chip 多选 / C：左侧边栏 |
| 主数据 | 表格 7 列宽 + 网格 md:2/xl:3 | A：紧凑表格 / B：4 列卡片 / C：列表+详情面板 |
| 安装记录 | 固定底部 `WorkSurface` 占纵向 | A/B：右侧抽屉 / C：详情面板 Tab |
| 视图切换 | 列表/网格 `V3Segmented` | A/B 保留切换 / C 固定列表+详情 |
| 高风险强调 | 表格行 `tone="danger"` | 三向均加左侧 accent bar + 待审批聚合 |

## 禁止事项

- 不要为单页引入新的色板或新的 token 体系，所有色值用 `--v3-*`。
- 不要把原型内联 CSS 直接复制进生产代码，实现时必须复用 `@/components/superteam` 组件。
- 不要在筛选区堆超过 2 行筛选，第 2 行必须可折叠（方向 C 例外，因边栏本身就是纵向）。
- 不要用 marketing 风格的入场动画，hover 上浮幅度 ≤ 2px。
- 不要删除现有的"列表/网格视图切换"能力（方向 A/B 保留），方向 C 用列表+详情替代。
- 不要把安装记录从首页层移除，只是改变其呈现位置（抽屉/详情面板），保留 `listSkillInstallations` 调用。
- 不要在原型中编造 `Skill` schema 之外的字段（如成功率、调用次数等），首页层无此数据来源。

## 落地路径

原型文件位于 `docs/prototypes/`：

- `skills-dense-table.html` — 方向 A（高密度目录表，Linear 风）
- `skills-capability-matrix.html` — 方向 B（能力矩阵，Stripe 风）
- `skills-governance-workbench.html` — 方向 C（治理工作台，主从详情风）
- `skills-redesign-spec.md` — 本规范

选定方向后，再回到 `apps/web/src/features/skills/index.tsx` 做真实实现：

1. 保留 `listSkills` / `listSkillInstallations` 数据层不变
2. 复用 `V3PageHeader` / `WorkSurface` / `V3Table` / `SoftCard` / `StatusPill` / `IconTile` / `V3MetricCard` / `V3Button` 等项目级组件
3. 顶部统计区按选定方向重构（摘要带 / pill 卡 / signature 区）
4. 筛选区按选定方向重构（命令栏 / chip + 搜索 / 边栏）
5. 主数据区按选定方向重构（紧凑表 / 4 列卡 / 列表+详情）
6. 安装记录从底部 `WorkSurface` 迁移到抽屉或详情面板
7. 真实端到端验证：通过浏览器打开 `/skills` 路由，确认数据来自真实接口而非 mock

---

**UI Designer**: 像素君
**设计探索日期**: 2026-07-06
**数据真实性**: 全部指标由 `listSkills()` 单接口派生，安装记录由 `listSkillInstallations()` 二级接口加载
**实现就绪**: 选定方向后可进入 `apps/web/src/features/skills/index.tsx` 真实改造
