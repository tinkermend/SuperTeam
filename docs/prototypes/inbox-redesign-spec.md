# 收件箱首页 UI 重设计 · 四种风格方案

> 基于 SuperTeam v3 Soft-Flat 设计基线与真实数据源 `GET /api/v1/inbox/items`，为收件箱首页探索四种不同布局样式的原型，供选型参考。

## 数据源（真实字段）

所有原型的数据均来自 `apps/web/src/lib/api/inbox.ts`，不引入虚构字段：

- **InboxListResponse** = `items[] + pagination + summary`
- **summary**：`open_count` / `high_risk_count` / `blocked_count`
- **InboxItem**：`id` / `item_type`(approval|project_decision) / `source_type` / `source_id` / `source_project_id` / `source_task_id` / `title` / `summary` / `status` / `risk_level`(blocked|high|medium|low) / `actions[]` / `context{}` / `deep_link{}` / `last_activity_at`
- **InboxViewMode**：`mine` | `team`

四个原型共用同一套 mock 数据（8 条事项，覆盖 approval/project_decision × blocked/high/medium/low），保证可比性。

## 四种风格对比

| 维度 | 风格一 · 决策控制台 | 风格二 · 优先级泳道 | 风格三 · 决策卡片网格 | 风格四 · 聚焦时间轴 |
|---|---|---|---|---|
| 布局 | 三栏：列表 \| 详情 \| 操作 | 四条横向泳道 | 2 列卡片网格 | 时间轴 \| 聚焦详情 |
| 信息密度 | ★★★★★ 最高 | ★★★ 中 | ★★★ 中 | ★★★★ 高 |
| 主交互 | 主从详情联动 | 横向滚动扫读 | 逐张独立处理 | 时间驱动逐条聚焦 |
| 适用场景 | 高效批量处理、控制台式工作 | 按优先级分桶、并行处理 | 逐条深度阅读、独立决策 | 按时间序逐条审阅、决策记录 |
| 视觉重心 | 中间详情面板 | 顶部阻断泳道 | 每张卡片自包含 | 右侧聚焦大卡 |
| 动作位置 | 右栏独立操作面板 | 卡片底部内联按钮 | 卡片底部内联按钮组 | 底部固定动作栏 |
| SLA 强调 | 右栏 SLA 倒计时环 | 阻断泳道顶置 | 卡片右上角标签 | 独立 SLA 进度条卡 |
| 类比 | Outlook 三栏 / Linear | Trello / Jira 看板 | Notion 卡片视图 | Gmail 阅读窗 + 时间线 |

## 风格详解

### 风格一 · 决策控制台（Decision Console）
**文件**：`inbox-decision-console.html`

- **布局**：顶部摘要指标(4 卡) → 工具栏(Tabs+筛选) → 三栏 `minmax(0,1fr) | minmax(0,1.05fr) | 290px`
- **左栏**：紧凑列表，每行带风险 accent bar，选中高亮，hover 浮起
- **中栏**：详情面板，分 4 段（标题头、为什么需要处理、过程记录时间线、证据列表）
- **右栏**：sticky 操作面板（SLA 倒计时环、动作按钮矩阵、快速跳转链接）
- **核心价值**：信息密度最高，一屏看全列表 + 详情 + 动作，适合重度后台操作员
- **风险**：三栏对屏宽要求高，<1280px 需折叠右栏

### 风格二 · 优先级泳道（Priority Lanes）
**文件**：`inbox-priority-lanes.html`

- **布局**：顶部摘要指标 → 工具栏 → 四条横向泳道（阻断 → 高风险 → 普通 → 已处理）
- **泳道**：左侧色条 + 图标 + 标题 + 计数 + 提示，右侧横向卡片流
- **卡片**：固定宽 318px，自包含标题/摘要/上下文/时间/动作，hover 浮起
- **核心价值**：优先级可视化最强，一眼看清"先处理哪条泳道"，适合多任务并行调度
- **风险**：横向滚动在窄屏不友好；已处理泳道可能浪费纵向空间

### 风格三 · 决策卡片网格（Decision Grid）
**文件**：`inbox-decision-grid.html`

- **布局**：横条摘要 → 工具栏(含视图切换) → 按风险分组的 2 列卡片网格
- **卡片**：大卡自包含完整决策上下文（KI 编号、标题、摘要、4 项元数据、证据 chip、动作按钮组），顶部 3px 风险色条
- **分组**：阻断与高风险 / 中低风险，组头条 + 计数 + 说明
- **核心价值**：可读性最佳，每张卡是完整决策单元，适合移动端友好、逐条审阅
- **风险**：信息密度偏低，一屏显示事项少；动作入口分散在各卡

### 风格四 · 聚焦时间轴（Focus Timeline）
**文件**：`inbox-focus-timeline.html`

- **布局**：Hero(摘要 + 今日聚焦卡) → 工具栏 → 双栏 `400px | 1fr`
- **左栏**：sticky 垂直时间轴，按"今天/更早"分组，节点带风险色圆点，选中高亮
- **右栏**：聚焦详情大卡（渐变头 + 大标题）+ 2×2 网格(SLA 进度条、上下文、过程记录、证据) + 底部 sticky 动作栏
- **核心价值**：时间感最强，单条深度处理体验最佳，动作栏固定底部便于连续决策
- **风险**：时间轴在小屏需折叠为抽屉；动作栏 sticky 可能遮挡底部内容

## 选型建议

| 你的诉求 | 推荐 |
|---|---|
| 重度后台操作员，每天处理 20+ 事项，要效率 | **风格一** 决策控制台 |
| 多项目并行，要一眼看清优先级分布 | **风格二** 优先级泳道 |
| 移动端友好、逐条深度审阅、决策记录可追溯 | **风格三** 决策卡片网格 |
| 按时间序处理、强调 SLA 与过程追溯 | **风格四** 聚焦时间轴 |
| 兼顾效率与可读性 | **风格一** 或 **风格四** |

## 设计一致性

四个原型均遵循 v3 Soft-Flat 基线：
- 复用 `apps/web/src/styles/theme.css` 的 `--v3-*` token（brand `#2F5FFF`、近白冷灰底、大圆角白卡、弥散阴影）
- 语义状态色分家：danger/warn/ok/info/artifact/mute 各司其职，状态 pill + accent bar + 圆点，不做大面积暖色底
- 整对象可选中、主从详情优先、默认安静例外强调
- 简体中文优先，文案来自真实业务（订单中台、支付网关、技能治理等）

## 下一步

选定风格后，可在 `apps/web/src/features/inbox/` 落地实现：
- 复用 `InboxShell` / `InboxItemList` / `InboxActionDialog` 现有组件
- 按 DESIGN.md「软壳装脆数据」规则组合 `SoftCard` / `WorkSurface` / `V3Table` / `V3MetricCard`
- 动作仍走 `POST /api/v1/inbox/items/{id}/actions`，不引入新接口
