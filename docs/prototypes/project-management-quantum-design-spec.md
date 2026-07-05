# 项目管理页面视觉焕新 — 设计说明

## 设计概念：Quantum Command Center

本次原型完全抛开现有 v3 Soft-Flat 浅色蓝色设计语言，采用**深色玻璃态控制台**风格，将项目管理界面重新定位为 AI 编排的"任务控制中心"。

### 核心设计哲学

| 维度 | 现有 v3 Soft-Flat | 全新 Quantum Command Center |
| --- | --- | --- |
| 底色 | 近白冷灰 (#F8FAFC) | 深空黑 (#08090F) |
| 主强调 | 品牌蓝 (#2F5FFF) | 靛紫 (#6366F1 → #818CF8) |
| 容器质感 | 白卡 + 弥散浅阴影 | 玻璃态 + 背景模糊 + 发光边框 |
| 状态表达 | 扁平 pill + 文字 | 发光圆点 + 脉冲动画 + 软底胶囊 |
| 信息层级 | 中性面 70% + 灰蓝 20% + 强调 10% | 深空面 60% + 玻璃面 25% + 发光强调 15% |
| 整体气质 | 克制、明亮、扁平 | 沉浸、精密、科技感 |

---

## 色彩系统

### 表面层级

| Token | 值 | 用途 |
| --- | --- | --- |
| `--bg-void` | `#08090F` | 页面最底层（深空） |
| `--bg-deep` | `#0B0D16` | 侧边栏、顶栏底色 |
| `--bg-elevated` | `#12141F` | 输入框、下拉选中态 |
| `--bg-card` | `rgba(22, 25, 38, 0.55)` | 玻璃态卡片（半透明 + 模糊） |
| `--bg-card-solid` | `#161926` | 不透明卡片（回退） |
| `--bg-hover` | `rgba(38, 42, 62, 0.5)` | 行/卡片 hover |
| `--bg-input` | `rgba(15, 17, 27, 0.8)` | 输入框底色 |

### 玻璃效果

| Token | 值 | 用途 |
| --- | --- | --- |
| `--glass-blur` | `20px` | `backdrop-filter` 模糊半径 |
| `--glass-border` | `rgba(255,255,255,0.07)` | 默认玻璃边框 |
| `--glass-border-hi` | `rgba(255,255,255,0.12)` | hover/焦点边框 |
| `--glass-shadow` | `0 8px 32px rgba(0,0,0,0.4)` | 卡片投影 |

### 文字层级

| Token | 值 | 用途 |
| --- | --- | --- |
| `--text-primary` | `#F0F2F8` | 标题、主要数据 |
| `--text-secondary` | `#A8AEC0` | 副标题、辅助文字 |
| `--text-tertiary` | `#6B7180` | ID、时间、说明 |

### 品牌强调色

| Token | 值 | 用途 |
| --- | --- | --- |
| `--accent` | `#818CF8` | 主操作、激活态、焦点环 |
| `--accent-deep` | `#6366F1` | 按钮、渐变起点 |
| `--accent-glow` | `rgba(129,140,248,0.35)` | 发光阴影 |
| `--accent-soft` | `rgba(99,102,241,0.12)` | 软底背景 |

### 语义状态色（发光体系）

| 状态 | 主色 | 发光 | 软底 | 用途 |
| --- | --- | --- | --- | --- |
| 运行/成功 | `#34D399` | `rgba(52,211,153,0.4)` | `rgba(52,211,153,0.10)` | 运行中、健康 |
| 等待/预警 | `#FBBF24` | `rgba(251,191,36,0.4)` | `rgba(251,191,36,0.10)` | 人工决策、等待 |
| 阻塞/失败 | `#F87171` | `rgba(248,113,113,0.4)` | `rgba(248,113,113,0.10)` | 失败、高风险 |
| 信息/配置 | `#38BDF8` | `rgba(56,189,248,0.4)` | `rgba(56,189,248,0.10)` | 配置中、草稿 |
| 完成/归档 | `#818CF8` | — | `rgba(129,140,248,0.10)` | 已完成 |
| 中性/静默 | `#64748B` | — | `rgba(100,116,139,0.10)` | 已归档、空闲 |

---

## 排版系统

| 层级 | 字号 | 字重 | 用途 |
| --- | --- | --- | --- |
| H1 | 28px | 800 | 页面标题 |
| H2 | 15px | 700 | 卡片标题 |
| Body | 13px | 400 | 表格内容、描述 |
| Small | 12px | 600 | 标签、pill |
| Caption | 11px | 700 | 表头、UPPERCASE |
| Metric | 32px | 800 | 风险指标大数字 |
| Mono | 11-12px | 400 | ID、时间戳、技术标识 |

- **UI 字体**：Inter / system-ui（回退）
- **等宽字体**：JetBrains Mono / SF Mono（用于 ID、时间、节点标识）
- **数字**：`font-variant-numeric: tabular-nums`（指标卡、进度数字）

---

## 组件架构

### 1. 玻璃态卡片 (Glass Card)
```
背景: rgba(22, 25, 38, 0.55) + backdrop-blur(20px)
边框: 1px solid rgba(255, 255, 255, 0.07)
圆角: 16px
hover: border-color 提亮 + translateY(-2px) + shadow 加深
```

### 2. 风险指标卡 (Risk Metric Card)
- 顶部 2px 彩色发光条（对应状态色）
- 右上角脉冲圆点（危险/预警时动画）
- 大数字 32px + 标签 12px
- hover 时发光条亮度提升

### 3. 状态胶囊 (Status Pill)
- 软底 + 状态色文字 + 发光圆点
- 运行中/阻塞状态的圆点有 blink 动画
- 圆角 20px（pill 形态）

### 4. 风险强调条 (Risk Accent Bar)
- 行左侧 3px 实色条
- danger: 红色 + box-shadow 发光
- warn: 琥珀色 + 轻发光
- ok: 绿色 40% 透明度

### 5. 迷你进度条 (Progress Mini Bar)
- 3px 高，圆角 2px
- 填充色对应执行状态
- 运行中填充有发光效果

### 6. 筛选芯片 (Filter Chip)
- pill 形态，28px 高
- 激活态：靛紫软底 + 靛紫文字 + 靛紫边框
- 带计数徽章（激活时反色）

---

## 交互动效

| 动效 | 触发 | 实现 |
| --- | --- | --- |
| 卡片上浮 | hover | `translateY(-2px)` + shadow 240ms |
| 脉冲发光 | 风险卡加载 | `box-shadow` 扩散 2s 循环 |
| 状态闪烁 | 运行中/阻塞 | `opacity` 1→0.3 1.5s 循环 |
| 焦点环 | focus-visible | 2px solid accent + offset 2px |
| 行 hover | mouseenter | 背景过渡 120ms |

所有动效遵守 `prefers-reduced-motion: reduce`，在用户偏好减少动效时禁用。

---

## 布局结构

```
┌─────────────────────────────────────────────────┐
│ Sidebar │  Topbar (breadcrumb + search + avatar) │
│  64px   ├────────────────────────────────────────┤
│         │  Content (max 1480px, centered)         │
│  icons  │                                         │
│  active │  ┌─ Page Header ──────────────────────┐ │
│  badge  │  │  Title + Subtitle    [Actions]     │ │
│         │  └────────────────────────────────────┘ │
│         │  ┌─ Risk Strip (6 cards) ─────────────┐ │
│         │  │  [阻塞] [人工] [失败] [证据] [超时]  │ │
│         │  └────────────────────────────────────┘ │
│         │  ┌─ Filter Bar ───────────────────────┐ │
│         │  │  [Search] [Status] [Risk Chips...] │ │
│         │  ├────────────────────────────────────┤ │
│         │  │  Project Table                     │ │
│         │  │  ┌──┬────┬────┬────┬────┬────┬───┐ │ │
│         │  │  │项│任务│节点│处理│状态│时间│操作│ │ │
│         │  │  │目│    │    │者  │    │    │    │ │ │
│         │  │  └──┴────┴────┴────┴────┴────┴───┘ │ │
│         │  ├────────────────────────────────────┤ │
│         │  │  Pagination                        │ │
│         │  └────────────────────────────────────┘ │
└─────────┴─────────────────────────────────────────┘
```

---

## 可访问性

- **对比度**：所有文字与背景对比度 ≥ 4.5:1（WCAG AA）
- **键盘导航**：全量 `:focus-visible` 焦点环
- **ARIA**：所有交互元素带 `aria-label`
- **语义化**：`<nav>`, `<header>`, `<main>`, `<table>` 语义标签
- **减少动效**：`@media (prefers-reduced-motion: reduce)` 全局降级
- **触控目标**：所有按钮 ≥ 32px 高度

---

## 与现有数据的映射

原型中的 10 个示例项目完整反映了真实业务字段：

| 原型字段 | 对应 API 字段 | 说明 |
| --- | --- | --- |
| 项目名称 | `project.name` | 2 行截断 |
| 项目 ID | `project.id` | 等宽字体显示 |
| 负责人 | `project.human_owner_user_id` | 等宽 code 标签 |
| 项目状态 | `project.status` | 6 种状态图标+色调 |
| 当前任务 | `workflow.title` / `riskSummary.currentTask` | 2 行截断 |
| 当前节点 | `currentTask.title` / `currentTask.task_kind` | 等宽 task_kind |
| 当前处理者 | `riskSummary.currentHandler.label` | 人工/AI/空闲徽章 |
| 执行状态 | `workflow.status` | 发光 pill + reason |
| 最后运行 | `workflow.updated_at` | 等宽时间 |
| 风险等级 | `riskSummary.level` | 左侧 accent bar |
| 进度 | `workflow.progress` | 迷你进度条 |

---

## 开发者落地建议

1. **Token 迁移**：如采纳此方向，在 `theme.css` 新增 `[data-theme="quantum"]` 作用域，与 v3 并存
2. **组件扩展**：玻璃态卡片可封装为 `QuantumCard`，状态 pill 可复用现有 `StatusPill` 扩展 `glow` prop
3. **渐进迁移**：先在项目管理页面试点，验证用户接受度后再推广
4. **暗色适配**：本设计本身就是暗色主题，无需额外 dark mode 适配
5. **性能注意**：`backdrop-filter` 在大量卡片场景可能影响性能，建议数据表格区域使用不透明背景

---

**设计者**：UI Designer
**日期**：2026-07-05
**状态**：高保真原型 · 待评审
