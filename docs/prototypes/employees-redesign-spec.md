# 数字员工目录页 · 设计补充规范

> 本文档是 `DESIGN.md` 的补充规范，只针对"数字员工目录页"的顶部统计区、筛选区、卡片网格三块进行设计探索，不替换 v3 Soft-Flat 主基线。所有色值、圆角、阴影、间距 token 必须沿用 `apps/web/src/styles/theme.css` 的 `--v3-*` 体系。

## 现状诊断

当前 `apps/web/src/features/employees/index.tsx` 存在以下视觉问题：

| 问题 | 现状 | 影响 |
| --- | --- | --- |
| 顶部统计过于松散 | 5 个 `V3MetricCard` 在 `xl:grid-cols-5` 下每张卡太宽，单数字单图标占一整张 | 视觉重量过大，挤压主区域 |
| 筛选区两层堆叠 | `SoftCard` 包裹 5+3 两行筛选，每行高度 ~96px，总高 ~210px | 首屏可见卡片数量减少 |
| 卡片过宽 | `md:grid-cols-2 / 2xl:grid-cols-3` + `min-h-[240px]` | 每张卡 ~360-420px 宽，信息密度低，4 列装不下 |
| 断点不和谐 | `md` 是 768px、`2xl` 是 1536px，中间 `lg/xl` 全是 2 列 | 1280-1535px 区间卡片过宽 |
| 右侧栏抢空间 | `xl:grid-cols-[minmax(0,1fr)_320px]` 在 xl 就吃掉 320px | 主网格在 1280px 实际只有 ~960px，2 列卡都嫌窄 |

## 设计目标

1. **4 列为目标密度**：在 `lg(1024px)+` 显示 4 列卡片，每张卡 `220-260px` 宽。
2. **顶部更轻**：统计区从 5 张大卡降为 1 条紧凑摘要带，高度 ≤ 80px。
3. **筛选更内敛**：主筛选 1 行（搜索 + 3 高频维度），次级筛选折叠到「更多」。
4. **断点平滑**：`md(2) → lg(3) → xl(4) → 2xl(4 更宽)`，无 2 列跨度过大的窗口。
5. **右侧栏让位**：默认隐藏右侧 WorkbenchRail，改为选中后抽屉/侧滑，把横向空间还给网格。

## 三种探索方向

### 方向 A · 高密度数据网格（Linear / Vercel 风）

- **顶部**：1 条横向摘要带（5 个数字 + 图标，无卡片框，仅靠分隔线分组）
- **筛选**：单行命令栏，搜索占 1.4fr + 3 个 Select + 「更多」折叠
- **卡片**：4 列紧凑卡（220px 宽 × 168px 高），头像 + 名字 + 状态 + 3 行关键指标，无按钮
- **交互**：点击卡片打开侧滑详情抽屉，释放主区域
- **适用**：员工数量 30+ 的高密度场景，扫读优先

### 方向 B · 指挥矩阵（Stripe / Notion 风）

- **顶部**：5 个 pill 式状态卡（小尺寸，icon+数字+标签，紧凑横排）
- **筛选**：搜索突出（左 1.4fr）+ chip 式多选筛选 + 排序下拉
- **卡片**：4 列中等卡（244px 宽 × 220px 高），左侧状态 accent bar + 头像 + 名字 + 状态 + 预算条 + 操作按钮
- **交互**：选中卡片高亮 + 顶部出现快捷操作条，详情走 Drawer
- **适用**：兼顾密度与信息量，每张卡是独立"小型仪表盘"

### 方向 C · 沉浸画廊（Figma / Airbnb 风）

- **顶部**：1 行视觉化统计带（带迷你趋势 sparkline，弱化数字、强调趋势）
- **筛选**：浮动玻璃面板（backdrop-filter），分段式 Tab + 智能搜索
- **卡片**：4 列头像优先卡（240px 宽 × 244px 高），大头像 + 名字 + 角色徽章 + 状态环 + 最近运行时间
- **交互**：hover 上浮 + 选中后右侧抽屉滑入（不挤压网格）
- **适用**：强调数字员工的"身份感"和品牌识别

## 卡片尺寸规范（三向共用）

| 维度 | 方向 A | 方向 B | 方向 C |
| --- | --- | --- | --- |
| 宽度（4 列 @ 1280px） | ~220px | ~244px | ~240px |
| 最小高度 | 168px | 220px | 244px |
| 内边距 | 14px | 16px | 18px |
| 头像尺寸 | 32px | 40px | 48px |
| 圆角 | `--r-inner` 14px | `--r-card` 22px | `--r-card` 22px |
| 阴影 | `--shadow`（轻） | `--shadow` + hover `--shadow-pop` | `--shadow` + hover `--shadow-pop` |
| 选中态 | 左 3px brand bar + brand-soft 底 | 左 3px brand bar + brand-soft 底 | brand border 2px + brand-soft 底 |

## 断点策略（统一）

| 断点 | 列数 | 容器 padding | 间距 |
| --- | --- | --- | --- |
| `<640px` (mobile) | 1 | 16px | 12px |
| `sm 640px` | 2 | 20px | 14px |
| `md 768px` | 2 | 24px | 16px |
| `lg 1024px` | 3 | 28px | 16px |
| `xl 1280px` | 4 | 32px | 16px |
| `2xl 1536px+` | 4 | 32px | 18px |

## 真实数据源

- `apps/web/src/lib/api/employees.ts` — `DigitalEmployeeOverview` / `DigitalEmployeeOverviewItem`
- 字段映射：
  - `summary.ready_count` / `pending_runtime_binding_count` / `error_count` / `pending_config_approval_count` / `failed_recent_run_count`
  - `item.identity_summary.{id,name,role,employee_type_label,team_name}`
  - `item.operational_state.status` → 7 种状态映射
  - `item.workbench_status` → ready / pending_binding / error
  - `item.execution_summary.{runtime_node_id,provider_type,node_id,runtime_name}`
  - `item.latest_run_summary.{status,started_at,finished_at,duration_sec}`
  - `item.budget_summary.{daily_token_limit,usage_tokens_today,usage_percent_today,limit_exceeded}`
  - `item.governance_summary.{status,employee_revision_number,skills_count,mcp_servers_count}`
  - `item.recent_events[].{label,occurred_at,status}`
  - `overview.queue_summary.{pending_runtime_binding_count,stale_config_count,failed_recent_run_count}`

## 禁止事项

- 不要为单页引入新的色板或新的 token 体系，所有色值用 `--v3-*`。
- 不要为了 4 列把卡片高度压到 < 160px，会丢失关键信息。
- 不要把右侧 WorkbenchRail 直接删掉，要保留「待处理队列」入口（可移到顶部摘要或工具栏 chip）。
- 不要在筛选区堆超过 2 行筛选，第 2 行必须可折叠。
- 不要用 marketing 风格的入场动画，hover 上浮幅度 ≤ 2px。

## 落地路径

原型文件位于 `docs/prototypes/`：

- `employees-dense-grid.html` — 方向 A
- `employees-command-matrix.html` — 方向 B
- `employees-immersive-gallery.html` — 方向 C

选定方向后，再回到 `apps/web/src/features/employees/index.tsx` 做真实实现，禁止把原型内联 CSS 直接复制进生产代码。
