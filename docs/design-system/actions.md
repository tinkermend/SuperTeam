# 操作控件

## 何时阅读

当修改按钮、图标按钮、操作链接、loading、disabled、危险操作或主操作层级时，阅读本文件。

## 按钮层级

- 一个区域只应有一个最高权重主操作。
- 主操作使用 v3 品牌蓝实底或 `--v3-brand-grad`，并具备清晰 hover、active、focus-visible、loading 和 disabled 状态。
- 次要操作使用 `secondary`、`outline` 或 `ghost`。
- 危险操作使用 destructive / `--v3-danger` 语义，不使用品牌主按钮样式。
- 工具型操作优先使用图标按钮；不常见图标需要 tooltip 或可访问标签。

## 主按钮

主按钮使用 `Button` 的 v3 默认 variant，或在项目级组合场景中使用 `V3Button`。只用于局部流程里的主要正向动作。

建议视觉：

- 品牌色：`--v3-brand`，需要 signature 强调时可使用 `--v3-brand-grad`。
- 文字和图标对比度必须充足
- 不叠加高光层或局部装饰光效。
- 阴影应柔和且受控，使用 `--v3-shadow` 或更轻的焦点 ring。
- active 态应像按下，而不是跳动

不要将主按钮用于失败、删除、撤销或不可逆危险操作。

## 次要与 Ghost 操作

- `outline` 应有稳定边框和可读前景色。
- `secondary` 使用轻量表面，不使用高饱和填充。
- `ghost` 保持低权重，hover 不改变布局尺寸。
- 取消操作通常使用 `outline` 或 `ghost`，取决于周围对比度。

## 图标按钮

- 优先使用 `lucide-react`。
- 图标尺寸跟随按钮尺寸。
- 图标按钮必须提供可访问名称。
- hover、active、focus-visible 和 disabled 状态必须可见。
- 熟悉工具动作优先用图标，不要强行使用带文字的圆角矩形。

## Loading 与 Disabled

- loading 状态必须避免重复提交。
- disabled 原因不明显时，应提供说明。
- loading 文案应尽量保持按钮宽度稳定。
- 避免在状态切换时使用长度差异过大的按钮文案，造成布局跳动。

## 危险操作

- 危险操作使用红色/危险语义和明确文案。
- 高风险流程应由当前业务说明影响范围和后果。
- 不只靠颜色表达危险，必须配合图标、文字或结构。
