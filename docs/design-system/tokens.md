# Tokens

## 何时阅读

当修改颜色、语义状态色、圆角、边框、阴影、焦点环或项目级 CSS 变量时，阅读本文件。

## 代码事实源

`apps/web/src/styles/theme.css` 是 token 的代码事实源。本文件只说明使用意图，不覆盖 CSS 文件。

静态原型使用 `docs/prototypes/design-system/design-system-prototypes.css` 镜像浅色 token，目的是验证设计文档拆分后的生成效果；如果 token 变化，先更新 `theme.css`，再同步原型 CSS。

## 核心 Token

- `--primary`：主操作、激活态、焦点状态和关键强调色。
- `--background`：默认页面底色，应保持白色、近白或极淡冷色。
- `--foreground`：主要文字色。
- `--muted` / `--muted-foreground`：次级区域和辅助文字。
- `--card` / `--card-foreground`：卡片和面板表面。
- `--popover` / `--popover-foreground`：浮层表面。
- `--border` / `--input` / `--ring`：边框、表单控件和焦点状态。
- `--sidebar-*`：侧边栏及其状态，应表达浅色液态玻璃侧栏，而不是深色实心侧栏。
- `--chart-*`：图表和辅助可视化颜色，应接近语义色。
- `--radius`：基础圆角。

## 推荐色彩基准

| 用途 | 建议色值 | 使用范围 |
| --- | --- | --- |
| 品牌主色 | `#2CC7AA` | 主按钮、导航激活、焦点态、关键链接 |
| 品牌深色 | `#0A806F` | 主按钮渐变终点、强调文字、焦点边缘 |
| 主色浅背景 | `#E6FBF5` | 激活菜单、选中表格行、轻量提示背景 |
| Shell 淡色背景 | `#F8FBF7` / `#EEF8F4` | 页面底色、侧栏玻璃底色 |
| 信息 / Runtime | `#0891B2` | Runtime、信息提示、连接状态 |
| 成功 / 审批通过 | `#16A34A` | 成功、健康、已通过 |
| 预警 / 外部能力 | `#F59E0B` | 提醒、等待、外部能力调用 |
| 危险 / 风险 | `#EF4444` | 失败、危险、高风险、阻断 |
| 工件 / 智能工作项 | `#8B5CF6` | 工件、生成物、智能处理结果 |
| 决策 / 人类确认 | `#10B981` | 决策请求、确认、人工验收 |
| 中性 / 审计 | `#64748B` | 审计、历史、说明、弱状态 |

色彩比例建议：页面 70% 使用白色/近白色/淡青绿/淡蓝背景，20% 使用灰蓝文字与边框，10% 用于主色和语义色。

## 项目级语义变量

优先通过 `theme.css` 使用或扩展项目变量，不要在页面内重复拼复杂视觉 class：

- `--superteam-shell-bg`
- `--superteam-glass-bg`
- `--superteam-liquid-bg`
- `--superteam-liquid-strong-bg`
- `--superteam-glass-border`
- `--superteam-glass-highlight`
- `--superteam-shadow-low`
- `--superteam-shadow-mid`
- `--superteam-shadow-glow`
- `--superteam-menu-accent-soft`
- `--superteam-menu-accent`
- `--superteam-menu-accent-deep`
- `--superteam-sidebar-active`
- `--superteam-sidebar-hover`
- `--superteam-info`
- `--superteam-success`
- `--superteam-warning`
- `--superteam-danger`
- `--superteam-artifact`
- `--superteam-decision`
- `--superteam-neutral`

## 使用规则

- 主强调色用于品牌、激活、主操作和焦点，不承载所有业务语义。
- 状态色必须全站语义一致。
- 语义色优先出现在图标、状态点、Badge、边框和小面积背景中。
- 不在单个功能里新增近似重复色。
- 默认工作区背景不使用深色。
- 暗色主题必须使用独立暗色玻璃值，不复用浅色主题的大面积白色/绿色高光。
