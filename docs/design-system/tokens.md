# Tokens

## 何时阅读

当修改颜色、语义状态色、圆角、边框、阴影、焦点环或项目级 CSS 变量时，阅读本文件。

## 代码事实源

`apps/web/src/styles/theme.css` 是 token 的代码事实源。本文件只说明使用意图，不覆盖 CSS 文件。

`docs/prototypes/design-direction-v3/` 仅作为视觉参考，不是 token 事实源；如果 token 变化，先更新 `theme.css`，再同步本文件。

## 核心 Token

- `--primary`：主操作、激活态、焦点状态和关键强调色。
- `--background`：默认页面底色，应保持白色、近白或极淡冷色。
- `--foreground`：主要文字色。
- `--muted` / `--muted-foreground`：次级区域和辅助文字。
- `--card` / `--card-foreground`：卡片和面板表面。
- `--popover` / `--popover-foreground`：浮层表面。
- `--border` / `--input` / `--ring`：边框、表单控件和焦点状态。
- `--sidebar-*`：侧边栏及其状态，应表达浅色 Soft-Flat Shell，而不是深色实心侧栏。
- `--chart-*`：图表和辅助可视化颜色，应接近语义色。
- `--radius`：基础圆角。

## v3 色彩基准（当前唯一基线，Soft-Flat 蓝）

v3 为当前唯一设计基线，主色为蓝色。下表色值为 v3 `--v3-*` token 的浅色基准（暗色端在 `theme.css` 的 `.dark`/`[data-theme=dark]` 内另给一套）：

矩枢平台的色彩使用边界见 `visual-language.md`：蓝色用于品牌、主操作、焦点、选中和小面积节点/线条；大面积蓝紫渐变不再作为默认页面识别方式。

| 用途 | 浅色基准 | 软底（soft） | 使用范围 |
| --- | --- | --- | --- |
| 品牌主色 `--v3-brand` | `#2F5FFF` | `#E9EFFF` | 主按钮、导航激活、焦点、关键链接、signature 线条和局部 accent |
| 品牌深色 `--v3-brand-deep` | `#2348E0` | — | 强调文字、按钮 hover、焦点边缘、小面积渐变终点 |
| 中性底 `--v3-bg` | `#F8FAFC` | — | 页面底色（近白冷灰，不带色相） |
| 卡片面 `--v3-card` | `#FFFFFF` | `--v3-card-soft #F7F8FA` | 柔和卡片、面板、表格容器外壳 |
| 主文字 `--v3-ink` | `#0B0D12` | `--v3-ink-2 #697586` / `--v3-ink-3 #9AA4B2` | 黑色粗体大数字与标题、次级、三级文字 |
| 边框 `--v3-line` | `#EEF1F4` | `--v3-line-strong #DFE4EA` | 卡片内分隔线、表格行线、表头底线 |
| 运行 / 信息 `--v3-info` | `#2563EB` | `#E8EFFD` | 运行中、队列、Runtime、系统信息 |
| 成功 / 通过 `--v3-ok` | `#15A06B` | `#E4F6EE` | 成功、在线、健康、已通过、验收通过 |
| 预警 / 等待 `--v3-warn` | `#CF7A14` | `#FDF0DB` | SLA、待处理、待确认、阈值预警 |
| 危险 / 阻断 `--v3-danger` | `#E5484D` | `#FDE8E9` | 失败、高风险、不可逆、阻断、危险操作 |
| 工件 / 产物 `--v3-artifact` | `#7C5CFF` | `#EFEAFF` | Artifact、报告、附件、生成物 |
| 中性 / 审计 `--v3-mute` | `#64748B` | `#EEF1F4` | 审计、历史、说明、低优先级、排队 |

注意：v3 把“颜色 = 紧迫度/状态”收敛为 5 个状态色（info/ok/warn/danger/mute）+ artifact，**类别（task/runtime/employee 等）改用图标 + 文字编码，不再每类各占一个色**。色彩比例：约 70% 中性面、20% 灰蓝文字与边框、10% 主色与语义色。

### v3 圆角、阴影、字体

- 圆角：`--v3-r-card ~22px`、`--v3-r-inner ~14px`、按钮/输入控件 `10px-12px`、pill/keycap `~8px`。
- 边框：默认 `--v3-line`，强调或控件边框 `--v3-line-strong`，焦点边框 `--v3-brand`。
- 阴影（弥散浅）：`--v3-shadow: 0 1px 2px rgba(16,24,40,.04), 0 12px 30px rgba(16,24,40,.055)`；hover 上浮用更强一档，不使用厚重投影。
- 间距：以 4px 基线倍数组织页面外边距、卡片内边距、表单组、步骤链和工具栏。
- 数字：比较场景一律 `font-variant-numeric: tabular-nums`；ID / UUID / 路径 / 哈希用等宽字体 `--mono`。

## 项目级语义变量

优先通过 `theme.css` 使用或扩展项目变量，不要在页面内重复拼复杂视觉 class：

- `--v3-brand`
- `--v3-brand-deep`
- `--v3-bg`
- `--v3-card`
- `--v3-card-soft`
- `--v3-card-inner`
- `--v3-ink`
- `--v3-ink-2`
- `--v3-ink-3`
- `--v3-line`
- `--v3-line-strong`
- `--v3-info`
- `--v3-ok`
- `--v3-warn`
- `--v3-danger`
- `--v3-artifact`
- `--v3-mute`
- `--v3-r-card`
- `--v3-r-inner`
- `--v3-shadow`
- `--v3-shadow-hover`

## 使用规则

- 主强调色用于品牌、激活、主操作和焦点，不承载所有业务语义。
- 状态色必须全站语义一致。
- 语义色优先出现在图标、状态点、Badge、边框和小面积背景中。
- 不在单个功能里新增近似重复色。
- 默认工作区背景不使用深色。
- 暗色主题必须使用独立暗色值，不复用浅色主题的大面积白色高光。
