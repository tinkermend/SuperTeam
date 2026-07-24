# 无障碍与暗色

## 何时阅读

改交互控件、状态展示、玻璃/半透明表面、暗色主题，或做可访问性/对比度验收时阅读。token 见 `tokens.md`，表面见 `surfaces.md`，图标见 `icons.md`。

## 目标

- **无障碍**：键盘可完成主路径；焦点可见；信息不只靠颜色；图标按钮有名字。
- **暗色**：可切换且可读，禁止靠 `bg-white/80` 等浅色硬编码「凑合」。

本文件定规则；具体对比度数值以 token 中 text/solid/soft 分层为准（soft 底上的字用 `*-text` / `brand-deep`）。

## 无障碍

### 键盘与焦点

- 可点控件必须可 Tab 到达；自定义可点 `div` 应具备 role、tabIndex、Enter/Space，**优先用 button/a**。
- 焦点环使用品牌 ring（如 `focus-visible:ring-brand`），不得 `outline-none` 且无替代。
- Dialog/Sheet：焦点陷阱在开层内；关闭后焦点回触发器（Radix 默认，勿破坏）。
- Esc 关闭浮层与取消同意图一致；有 dirty 时先确认（`form-flows.md`）。

### 名与标签

- 仅图标的按钮：`aria-label` 或可视 `sr-only` 文案（中文）。
- 表单控件与 `Label` 关联（`htmlFor`/包裹）。
- 链接目的从文案可懂（避免仅「点击这里」）。

### 颜色与状态

- 状态 = **文案或图标 + 颜色**（`StatusPill` 的点+文字是默认范式）。
- 勿用单独红/绿块表达对错而无文字。
- 图表与地图：色盲友好优先；关键序列有直接标注。

### 动效

- 装饰性动效服从 `prefers-reduced-motion`（玻璃 blur 已降级；新增动效必须同样处理）。
- 不闪烁高频；加载指示用 spinner/文案，不用狂闪渐变。

### Chip / 卡片点击

- 筛选 Chip 需要 `onClick` 时用 button 语义。
- 展示型标签用 span（`Chip` 无 onClick 时已降级为 span）；**禁止 button 嵌 button**。
- 整卡可点：优先 `<Link>`/`<a>` 包卡，或单层 button，避免卡内再套主按钮抢事件（卡内次要动作须 stopPropagation 且可键盘操作）。

### 跳过与结构

- 保留「跳到主内容」：`SkipToMain`。
- 标题层级近似大纲，不跳过 h1→h3 滥用。

### 检查清单（a11y）

- [ ] 主路径仅键盘可走通
- [ ] 焦点环可见
- [ ] 图标按钮有中文名
- [ ] 状态不只靠颜色
- [ ] 浮层焦点与 Esc 正常
- [ ] 无 button 嵌套 button

## 暗色主题

### 原则

- 暗色通过 `theme.css` 的 `.dark` / `[data-theme=dark]` **同一语义 token 换值**，组件不写死第二套 class。
- 内容区、表格、表单保持**实底**高对比；半透明仅 Shell/顶栏控件/Tier A 玻璃。
- soft 底上的文字必须用 text 层 token（如 `text-ok-text`），不用低对比 solid。

### 禁止（实现高频债）

| 禁止 | 改用 |
| --- | --- |
| `bg-white` / `bg-white/80` / `bg-white/90` 大面积铺内容 | `bg-card` / `bg-card-soft` / `bg-background` |
| `text-black` / `border-white` 硬编码 | `text-ink*` / `border-line*` |
| 浅色渐变画布写死在 feature（如 `#f8fbff`） | token 或 `bg-card` + 允许的 signature/aurora token |
| 暗色下靠提高模糊「看不清也凑合」 | 降透明、改实底 |

### 玻璃与 Shell

- 暗色下玻璃与侧栏仍走 `--aurora-*` / `--shell-*`；不得另写一套 hex。
- 密表/审计在暗色同样**禁止**玻璃。
- `prefers-reduced-motion` 时去掉 blur 的规则在暗色同样生效。

### 状态色

- 仍用 `Tone`：`info|ok|warn|danger|mute|artifact|brand`。
- 暗色 solid/soft/text 以 theme 为准；验收时看 soft 底上文案是否可读。

### 检查清单（dark）

- [ ] 开 dark 后主列表/表单/对话框可读
- [ ] 无大面积白纸块或浅色硬编码
- [ ] 状态 pill 与链接对比可辨
- [ ] 玻璃页（若有）不超过 Tier A 且可读
- [ ] 未引入 feature 级暗色专用 class 分叉

## 与测试/护栏

- 文案与枚举：`status-labels` + guard。
- 文档旧 token：`verify-design-system` Soft-Flat 护栏。
- 组件交互：优先角色定位（`getByRole`）而非纯 class。

## 事实源

- token 与 dark 值：`apps/web/src/styles/theme.css`
- 玻璃与 reduced-motion：`apps/web/src/styles/index.css`
- 组件：`apps/web/src/components/superteam/primitives.tsx`
