# 浮层

## 何时阅读

当修改 Dialog、AlertDialog、Sheet、Drawer、Popover、Dropdown、Tooltip、Toast 或类似弹窗确认流程时，阅读本文件。

## 范围边界

浮层只定义视觉结构和交互层级，不定义具体业务字段。标题、说明、状态提示、辅助信息、上下文入口和元信息都应作为可选槽位，由当前业务按需提供。

## 浮层类型

- Dialog：确认、短表单和强打断信息。
- AlertDialog：高注意力确认，尤其是危险或不可逆动作。
- Drawer/Sheet：详情、编辑和长内容。
- Popover/Dropdown：轻量上下文动作或辅助内容。
- Tooltip：短说明。
- Toast：短反馈。

Toast 只承载短反馈，不替代页面内错误、表单校验和需要决策的信息。

## Dialog 容器

Dialog、AlertDialog、短表单和确认框应优先复用 `components/superteam` 的 Soft-Flat 组合组件；组件不足时在 `primitives.tsx` 扩展。项目级组合样式负责遮罩、实底容器、标题区、内容区、底部操作区和关闭按钮；不得定义业务字段。

### 代码锚点（Batch A）

| 场景 | 组件 |
| --- | --- |
| 自定义内容 Dialog | `SoftDialog` + `SoftDialogContent` + `SoftDialogHeader` / `Title` / `Description` + `SoftDialogBody` + `SoftDialogFooter` |
| 详情 / 编辑抽屉 | `SoftSheet` + `SoftSheetContent` + `Header` / `Body` / `Footer` |
| 简单确认 / 危险确认 | `ConfirmDialog`（AlertDialog 语义 + SoftDialog 同款视觉；一站式 API） |
| 底层 Radix | `components/ui/dialog`、`components/ui/sheet` 仅作 primitive；**业务 feature 新代码不要直接拼默认 `DialogContent` 样式** |

`SoftDialogContent` 宽度：`size` 取 `sm` / `md` / `lg`（短确认 / 短表单 / 需更宽内容）。`SoftSheetContent`：`side` 默认 `right`，`size` 取 `md` / `lg`。

关闭按钮必须中文可访问名「关闭」；底栏主操作唯一最高权重，主按钮 `Button variant="primary"`，危险用 `danger`。


Dialog 和 AlertDialog 应采用 Soft-Flat 实底外壳：

- 背景：`--card`
- 边框：`--line` / `--line-strong`
- 圆角：`--token-radius-card`（Tailwind：`rounded-card`）
- 阴影：普通弹窗不超过 `--token-shadow-pop`（Tailwind：`shadow-pop`）
- 避免在单个弹窗内重造独立背景、边框或阴影体系

宽度按内容复杂度分级：

- 短确认使用紧凑宽度
- 短表单使用中等宽度
- 只有真的需要可选辅助区域时，才加宽弹窗

不要为了让弹窗显得丰富而强行加入业务摘要区。

## 遮罩

- 使用低压暗化和可选背景模糊，例如冷灰/青黑半透明底叠加轻量 `backdrop-filter`。
- 遮罩服务聚焦，不制造舞台灯光。
- 避免厚重黑幕、高饱和背景或装饰光效。

## 标题区

- 标题区节奏：左侧可选语义图标或小徽章，中间为标题和说明，右侧为关闭按钮或少量辅助操作。
- 没有图标或说明时，也要保持对齐稳定。
- Dialog 标题通常使用 `20px` 到 `24px` 的强字重。
- 说明文字使用灰蓝色 `14px` 到 `16px`。
- 不在浮层内使用 hero 级大标题。

## 关闭按钮

- 关闭按钮必须是独立图标按钮。
- 使用 `lucide-react`。
- 提供可访问名称。
- 具备 Soft-Flat 轻量 hover、清晰 focus-visible 和必要的 disabled 状态。
- 避免裸露、过大、过浅或未样式化的默认关闭图标。

## 内容区

- 优先通过留白、分组、细分割线和浅色面建立层级。
- 可选信息区使用轻量行、胶囊或分隔布局。
- 不把每个信息点做成独立卡片。
- 避免在 Dialog 内容区卡片套卡片。
- 长内容应可读、可滚动，并在需要时保持头部/底部操作区稳定。

## 底部操作区

- 底部操作区应有稳定分隔和对齐。
- 桌面端主/次操作通常右对齐。
- 业务需要时，可在左侧放可选说明或状态。
- 移动端按钮可纵向堆叠或全宽显示。
- 只有一个操作拥有最高视觉权重。
- 主操作使用 Soft-Flat 品牌蓝按钮（`Button variant="primary"`）。
- 取消和次要操作使用 outline、secondary 或 ghost。
- 危险动作使用 destructive 语义，不套用主色渐变。

## 浮层反馈

- 错误、警告、成功和处理中反馈应靠近相关内容或底部操作区。
- 使用语义色、小图标和短文案。
- 反馈不能让弹窗宽度不可预期地变化，也不能让主操作跳动。

## Popover、Dropdown 与 Tooltip

- 这些轻量浮层应比 Dialog 更轻。
- 使用更小圆角、更弱阴影和更紧凑的 Soft-Flat 实底面。
- 不要把轻量浮层做成完整弹窗质感。

## 命令菜单

顶部命令中心展开的搜索结果、快捷动作和任务发起建议使用 Popover / 命令菜单级别的轻量浮层。

- 容器使用 `--card` 实底、`--line` 边框和轻量阴影，不使用大面积蓝紫渐变或透明科技面板。
- 分组标题、快捷键 keycap、状态 pill 和图标容器应复用 `navigation.md`、`actions.md`、`data-display.md` 的规则。
- 命令项 hover 和 active 态只改变背景、边框或左侧品牌线，不改变行高。
- 命令菜单只组织当前可执行入口、搜索结果和真实状态，不新增业务字段或伪造数据。
