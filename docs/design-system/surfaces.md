# 表面与质感

## 何时阅读

当修改页面背景、Shell 表面、顶栏半透明手势、卡片、面板或整体视觉层级时，阅读本文件。

## v3 表面模型：一套语言，三种容器（当前目标）

v3 用**三种容器**承载所有内容，按"页面 Tier 分类 + 是否需要逐行扫读"选择，三者共用同一套 token：

矩枢平台的表面语言见 `visual-language.md`：品牌识别优先来自细线、节点、低对比网格、中枢路径和克制 accent，而不是大面积蓝紫渐变。

### 1. 柔和卡片（Soft Card）— 外壳与概览

- 用途：页面外壳、概览指标卡、实体目录卡（项目/数字员工/技能）、详情头卡、signature 卡。
- 形态：`background: var(--v3-card)`（纯白/暗色实底），`border-radius: var(--v3-r-card ~22px)`，弥散浅阴影 `var(--v3-shadow)`，无硬边框或仅极淡 `--v3-line`。
- 内容：黑色粗体大数字 + 灰色小标签；自带“状态 pill + 关键指标 + 一个主操作”；hover 可轻微上浮。
- signature：每个概览屏最多一块 signature 卡做视觉锚点与重心，承载该页最有故事性的信息（环形进度、步骤、聚合分布、命令入口）。
  - 表面优先使用实底 `--v3-card` / `--v3-signature-surface` + 品牌顶线 + 低对比网格 + 克制节点/连线。
  - 渐变只允许作为小面积 accent、角落轻光感或主操作强调；不要用高饱和亮蓝/蓝紫渐变做大面积底。
  - 目标气质：前卫、有科技感，但克制不夸张；文本必须在实底上清晰可读。

### 2. 脆数据面（Work Surface）— 密集数据本体

- 用途：需要逐行扫读、对齐、比较的数据（任务表、审计流水、日志、证据、diff、密集表单）。
- 形态：**实底、不透明、不模糊、零装饰**。表头 `--v3-card-soft` + sticky；行线 `--v3-line`；hover 行用 `--v3-card-inner`。
- 纪律：数字 `tabular-nums`；ID/UUID/路径用等宽字体；危险/失败行用**左侧实色 accent bar（`box-shadow: inset 3px 0 0 var(--v3-danger)`）+ 实底浅红**，不靠半透明削弱；提供"舒适/紧凑"密度切换（行内边距 token 化）。

### 3. 玻璃卡（Glass Card）— 低密度沉浸面板

- 用途：**低密度面**的内容外壳——入口/创建画布（任务发起、数字员工创建、技能上传向导、登录/onboarding 空状态）、KPI/概览卡、字段有限的实体详情卡、摘要/预检面板。在极光背景上叠加半透明玻璃层，营造沉浸质感。**用不用看内容密度，不看页面身份。**
- 形态：半透明白底 + `backdrop-filter: blur + saturate` + 内侧高光 + 品牌色细边框。
- **禁止场景（按密度，与页面无关）**：高密度 / 审计面——密集表格、审计、日志、收件箱、流程编排、逐行需扫读比较的数据本体——一律不得使用玻璃卡；密集数据面即使被玻璃外壳包住，也必须退回实底 WorkSurface。理由：① 半透明下文字对比度随背景漂移，密集小字/日志最先不可读；② `backdrop-filter` 滚动重绘，长表格/日志卡顿；③ 审计面需确凿实底可信感。
- **性能纪律**：`backdrop-filter` 是主导成本，每屏玻璃卡数量 ≤ 4 块；不得给每行/每格套玻璃；低端设备按 `prefers-reduced-motion` 降级。

**Token 映射（浅色 / 深色自动切换，事实源在 `theme.css`）**：

| 属性 | Token | 浅色值 | 深色值 |
| --- | --- | --- | --- |
| 玻璃卡背景 | `--v3-aurora-glass` | `rgba(255,255,255,0.55)` | `rgba(22,28,44,0.55)` |
| 玻璃卡边框 | `--v3-aurora-glass-border` | `rgba(255,255,255,0.8)` | `rgba(120,150,255,0.16)` |
| 内层面板背景 | `--v3-aurora-panel` | `rgba(255,255,255,0.7)` | `rgba(18,24,40,0.6)` |
| 内层面板边框 | `--v3-aurora-panel-border` | `rgba(47,95,255,0.18)` | `rgba(120,150,255,0.2)` |
| 模糊半径 | `--v3-shell-glass-blur` | `22px` | `22px` |
| 饱和度增强 | `--v3-shell-glass-saturate` | `1.35` | `1.24` |
| 卡片圆角 | `--v3-aurora-r-card` | `24px` | `24px` |
| 内层圆角 | `--v3-aurora-r-inner` | `16px` | `16px` |

**唯一实现（不要重抄、不要新建平行 `*-glass` 类）**：

- 组件：`GlassCard`（`@/components/superteam`）——Tier A 页面统一用 `<GlassCard>` 做玻璃外壳。
- 样式类：`.v3-glass` / `.v3-glass-inner`，定义在 `apps/web/src/styles/index.css` 的 `@layer components`，色值/圆角/模糊全部取自 `--v3-aurora-*` / `--v3-shell-glass-*` token（含 `prefers-reduced-motion` 降级）。
- feature 内的 `*aurora.css`（`.tl-*` 等）只承载**页面专属布局**，不得重新声明玻璃表面。

`.v3-glass` 等价样式（仅供理解，实际以 `index.css` 为准）：

```css
.v3-glass {
  background: var(--v3-aurora-glass);
  border: 1px solid var(--v3-aurora-glass-border);
  border-radius: var(--v3-aurora-r-card);
  backdrop-filter: blur(var(--v3-shell-glass-blur)) saturate(var(--v3-shell-glass-saturate));
  box-shadow: 0 1px 0 rgba(255,255,255,0.9) inset, 0 24px 56px -24px rgba(36,58,140,0.35);
  overflow: hidden;
}

.v3-glass-inner {
  background: var(--v3-aurora-panel);
  border: 1px solid var(--v3-aurora-panel-border);
  border-radius: var(--v3-aurora-r-inner);
}
```

**内层结构**：
- 玻璃卡内部可嵌套 `.v3-glass-inner` 做二级面板（表单区、摘要卡、预检面板），保持层级一致。
- 玻璃卡内部的表单输入框、select 使用 `rgba(255,255,255,0.9)` 实底（非半透明），保证可读性。
- 玻璃卡内部的按钮遵循 `V3Button` 规范：次级按钮用 `variant="glass"`（半透明 aurora 面板 + 品牌 accent，融入玻璃），主操作可用 `linear-gradient(180deg, #4f8cff, #2f5fff)` 品牌渐变；不要在玻璃上放不透明白底按钮。
- 玻璃卡内部的二级面板（当前任务、消耗、摘要等）用 `.v3-glass-inner`；进度条/分隔等细线用 `--v3-aurora-hairline`，不要用纯白实底。

**装饰光斑**（可选，≤ 3 个）：
- `position: fixed` + `pointer-events: none` + `filter: blur(48px)` + `opacity ≤ 0.5`
- 颜色限青/紫/品牌蓝三色系
- 须 static 定位以合成一次缓存，不随滚动重绘

### 融合规则（核心）

- **软壳装脆数据**：密集表格被装进一张柔和白卡（`--v3-r-card` 圆角 + 弥散阴影）里——外缘柔、内部逐行脆。
- **玻璃壳装实底内核**：低密度面可用玻璃卡做外壳，但内部表单输入、密集列表和逐行数据仍须退回实底（`rgba(255,255,255,0.9)` 或 `--v3-card`），不把每行/每格套半透明。
- **内容区必须实底**：轻量半透明手势只允许出现在顶栏控件或吸顶兜底层；顶栏容器可以透明融入 Shell，但一旦进入逐行内容，背景必须实底高对比。
- **默认安静、例外才喧哗**：界面大面积保持中性灰，只有需要人介入的对象（待审批、失败、危险）才着色/加粗跳出。

## 背景

- 控制台外壳使用 Acrylic Shell：`--v3-shell-bg` 做柔和底色，`--v3-shell-background` 承载低对比渐变 wash，`--v3-shell-grid` 只作为极淡网格层。
- 顶部 Header 默认透明，让 Shell 背景贯穿；搜索框、主题、通知和账户等控件使用独立 acrylic surface，其中搜索框用更强的 `--v3-shell-search*` token 承担视觉中心。
- 主内容里的卡片、表格、表单和详情面仍使用 `--v3-card` / `--v3-card-soft` / `--v3-bg` 等实底 token，不直接透出背景纹理。
- Shell 可以带非常轻的冷灰 wash、低饱和蓝绿光感或低对比网格，但不做高饱和环境色。
- 背景纹理不能影响表格、图表和表单的辨识度。
- 不要散落装饰性光斑、独立 orb 或 bokeh。
- 节点、网络和调度线只能服务页面结构、步骤链、拓扑或命令入口，不作为无意义背景装饰。

## 卡片与面板

- 面板用于组织页面结构，卡片用于承载独立信息单元。
- 避免把大页面切成过多装饰性卡片。
- 面板标题区应包含标题、必要说明和少量操作。
- 复杂操作应进入工具栏、菜单、抽屉或详情区。

独立功能卡片可以使用高级卡片质感：

- 极微弱对角渐变，例如从 `--v3-card` 过渡到 `--v3-card-soft` 或 `--v3-brand` 的浅软底。
- 细粒度内侧高光，例如 `ring-1 ring-white/60`
- hover 上浮，例如 `hover:-translate-y-1`
- 使用 `--v3-shadow-hover` 的柔和阴影

高级卡片质感只用于真正独立的卡片对象，不用于每个行、字段或小分组。

## 复用表面组件

- 登录页和控制台外层背景应复用同一套 v3 Shell 组合。
- 通用卡片应复用 v3 Soft Card 组合。
- 搜索框、Tab 容器、次要胶囊按钮、轻量筛选器和非状态型短标签应复用 v3 pill 组合。

## 检查问题

- 文本在表面上是否仍然清晰？
- 阴影是否表达层级，而不是显得厚重？
- 顶栏半透明手势是否服务工作流，而不是装饰页面？
- 这是复用表面，还是单页 class 堆叠？
- 玻璃卡是否只在 Tier A 页面使用？Tier B/C 是否误用了半透明？
- 玻璃卡内部的数据面是否退回了实底？每行/每格是否避免了 `backdrop-filter`？
- 玻璃卡数量是否 ≤ 4 块？装饰光斑是否 `position: fixed` + `pointer-events: none`？
