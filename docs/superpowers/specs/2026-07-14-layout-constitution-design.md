# 布局宪法（第一期）：宽度档位 + 布局基元 + 项目管理页试点

- 日期：2026-07-14
- 状态：已批准（本期只做第一步；`Main` 默认值翻转、存量 19 处魔法栅格迁移、多视口自动化门禁均为后续期）
- 背景：全站页面在不同分辨率下观感差异过大。根因是布局层缺治理——`Main` 默认流体无上限（32 处调用无人用 `contained`）、20+ 处手写 `grid-cols-[minmax(0,1fr)_NNNpx]` 魔法栅格（右栏宽度 280/320/324/340/420 五种）、内容区大量使用视口断点而 shell 已就位的 `@container/content` 几乎无人使用、`layout-density.md` 只有散文无可执行规则。

## 目标

1. 建立可执行的布局宪法：宽度档位、断点口径、主从布局、指标带四条规则，token 与组件均单一事实源。
2. 沉淀两个布局基元组件，登记进 DESIGN.md 概念表。
3. 用项目管理页（`/projects` 列表态）完成首个试点并真实浏览器验证。

## 决策记录

- 宽度档位采用三档：contained ≈1280 / wide ≈1680 / canvas 不限（用户已选定）。
- 换纯 shadcn 方案已评估并否决：v3 是架在 shadcn 上的薄皮（~2000 行组件 + ~750 行样式），重写代价（99 文件 2528 处引用 + 30 测试文件 + 52 页面回归）远大于补布局层，且 shadcn 不含布局系统，换完问题依旧。

## 1. 宽度档位 token（`apps/web/src/styles/theme.css`）

`:root` 新增（浅/深色不区分）：

```css
/* v3 布局宪法 token：页面宽度档位 / 主从右栏档位 / 指标卡宽度界 */
--v3-layout-contained: 80rem;   /* 1280px 表单、设置、单对象详情 */
--v3-layout-wide: 105rem;       /* 1680px 主从工作台（队列+右栏） */
--v3-layout-rail: 21.25rem;     /* 340px 标准右栏 */
--v3-layout-rail-lg: 26.25rem;  /* 420px 宽右栏（承载完整 triage 面板时） */
--v3-metric-min: 13rem;         /* 208px KPI 卡宽度下限 */
--v3-metric-max: 21rem;         /* 336px KPI 卡宽度上限 */
```

canvas 档不设 token，语义即"不限宽"（仅图形/拓扑画布）。消费方式为 Tailwind v4 任意值语法（如 `max-w-(--v3-layout-wide)`），不新增 `@theme` 映射。

## 2. `Main` 宽度档位（`apps/web/src/components/layout/main.tsx`）

新增 `width?: 'contained' | 'wide' | 'canvas'`：

- `contained`：等价现有 `contained` prop 行为（`@7xl/content:mx-auto @7xl/content:max-w-7xl`）。
- `wide`：`mx-auto w-full max-w-(--v3-layout-wide)`。
- `canvas` 或未传：维持现行为（全宽）。
- 兼容：现有 `contained`/`fluid` prop 继续生效，映射到对应档位；本期不翻转默认值，现有 32 处调用零行为变化。

## 3. 布局基元（`apps/web/src/components/superteam/layout.tsx`，从 `index.ts` 导出）

### MasterDetailLayout

队列 + 右栏主从布局，取代手写 `xl:grid-cols-[minmax(0,1fr)_NNNpx]`。

- Props：`master: ReactNode`（或 children 二选一，定案用 `master`/`detail` 两个显式槽位）、`detail: ReactNode`、`rail?: 'md' | 'lg'`（默认 `md`=340px，`lg`=420px）、`className?`。
- 结构：外层 wrapper 设 `@container/master-detail`（`container-type: inline-size`），内层 grid 默认单列（detail 落到 master 下方），容器宽度达阈值时变双列 `grid-cols-[minmax(0,1fr)_var(--v3-layout-rail[-lg])]`。
- 折叠阈值按 rail 档位内置：`md` → `@4xl/master-detail`（56rem），`lg` → `@5xl/master-detail`（64rem）。保证 master 列 ≥ 约 600px 才展开双列。
- 右栏 sticky 由调用方自决，但必须用 `@4xl/master-detail:` / `@5xl/master-detail:` 容器变体，禁止视口断点。

### MetricGrid

KPI 指标带，取代手写 `sm:grid-cols-2 xl:grid-cols-4`。

- Props：`children`、`className?`、可选 `aria-label`（渲染为 `section`）。
- 实现：`grid gap-3 grid-cols-[repeat(auto-fit,minmax(min(100%,var(--v3-metric-min)),var(--v3-metric-max)))]`。
- 行为：卡片宽度被限制在 208–336px 区间，自动换行，左对齐，超宽屏允许尾部留白——KPI 卡在任何分辨率下密度一致，不再被拉成 400px+ 空卡。

两个组件均补单元测试（渲染结构、rail 档位对应的 class、槽位内容）。

## 4. 文档落地

- `docs/design-system/layout-density.md` 新增四个可执行章节 + 存量迁移指引：
  1. **宽度档位**：三档定义、每档适用页面类型、token 名、超上限居中留白；新页面必须显式选档。
  2. **断点口径**：内容区（`Main` 之内）响应一律用容器断点（shell 已提供 `@container/content`，局部布局用组件自带容器）；视口断点（`sm:/md:/lg:/xl:`）仅允许用于 shell 级（侧栏、顶栏）；新增代码禁止在内容区用视口断点做布局折叠。
  3. **主从布局**：必须用 `MasterDetailLayout`；右栏只有 340/420 两档；禁止新增手写 `grid-cols-[minmax(0,1fr)_NNNpx]`。
  4. **指标带**：必须用 `MetricGrid`；KPI 卡宽度有界；数量不足时左对齐留白，不拉伸填充。
  - 迁移指引：列出存量魔法栅格属于待偿债务，触达时必须顺手迁移，不得据现状反推规范。
- `DESIGN.md` 概念到代码表登记 `MasterDetailLayout`、`MetricGrid`，并在表中补一行"页面宽度档位 → `Main width`"。

## 5. 试点：项目管理页

`apps/web/src/features/projects/index.tsx` 及 `features/projects/components/project-risk-home.tsx`：

- `ProjectsView` 的 `<Main>` 加 `width="wide"`（列表态与项目详情态同为 wide 档）。
- 列表态 `grid min-w-0 items-start gap-5 xl:grid-cols-[minmax(0,1fr)_420px]` → `MasterDetailLayout rail="lg"`（master=`ProjectRiskQueue`，detail=`ProjectTriagePanel`）。
- `ProjectPortfolioSummaryBar` 的 `grid gap-3 sm:grid-cols-2 xl:grid-cols-4` → `MetricGrid`。
- `ProjectTriagePanel` 的 `xl:sticky xl:top-4` → `@5xl/master-detail:sticky @5xl/master-detail:top-4`。
- 其余 19 处魔法栅格与其它页面本期不动。

## 6. 验证

- 新组件单测 + `corepack pnpm --filter @superteam/web test` 中 projects 相关既有测试。
- `corepack pnpm verify:design-system`（文档引用校验）。
- 真实浏览器（playwright）访问 `/projects`，在 1280 / 1536 / 2000+ 三档视口宽度下确认：wide 档居中生效、KPI 卡宽度有界、主从双列/单列折叠正确、无横向溢出。
- 多视口自动化门禁（纳入 `verify:web`）为后续期工作，本期不做。

## 风险与边界

- `container-type: inline-size` wrapper 会建立包含上下文，若 detail 槽内有 `position: fixed` 相对视口的假设可能受影响；试点页右栏仅 sticky，无此问题。
- 试点页既有测试断言了 `grid-cols-[minmax(0,1fr)_420px]`（`features/projects/index.test.tsx:1608`），迁移时同步更新断言。
- `Main` 本期不翻转默认值，未触达页面在超宽屏行为不变（仍全宽），属预期。
