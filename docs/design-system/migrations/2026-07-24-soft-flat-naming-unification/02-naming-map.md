# 02 · 命名映射表（唯一命名真相）

> 实现必须遵守本表。表外名字 = 缺陷，先改文档。  
> **状态**：草案，待人类在 `11-human-review-checklist` 确认「开放决策」列。

## 0. 命名总原则

1. 去掉版本前缀 `V3` / `v3-` / `--v3-`。
2. 使用**语义英文**（brand, ink, line, surface…），不使用 `st-` / `ds-` / `sf-` 作为永久前缀（开放决策若否决见 §8）。
3. 已有好名字（`SoftCard` 等）不改。
4. 与 shadcn 冲突时：业务公共 API 优先 Soft-Flat 语义名；shadcn 模块改为内部导入路径或改名，见 §2。
5. `data-slot` 与组件公共名对齐，但**不要**与 shadcn 的 `data-slot="button"` 无差别冲突——业务 Button 使用 `data-slot="st-button"` 或 `data-slot="sf-button"`？  
   - **推荐草案**：业务组件 slot 用**无前缀语义**且与 shadcn 错开：`soft-card`, `work-surface`, `status-pill`, `app-button`（见开放决策 Q3）。

---

## 1. 组件与类型映射

### 1.1 导出组件 / 类型

| 现名 | 目标名 | 过渡 alias | 备注 |
| --- | --- | --- | --- |
| `V3Button` | `Button` | `V3Button` → `Button` 1 个发布窗口 | 见 §2 与 shadcn Button 冲突处理 |
| `V3ButtonVariant` | `ButtonVariant` | 同左 | `"primary"\|"outline"\|"ghost"\|"danger"\|"glass"` |
| `V3ButtonSize` | `ButtonSize` | 同左 | `"default"\|"sm"\|"icon"` |
| `V3IconButton` | `IconButton` | 同左 | |
| `V3Chip` | `Chip` | 同左 | |
| `V3Tone` | `Tone` | 同左 | 若全局太泛，备选 `SemanticTone`（开放决策 Q1） |
| `V3Density` | `Density` | 同左 | `"comfortable"\|"compact"` |
| `V3Table` | `DataTable` | 同左 | **不用** `Table`，避 `ui/table`（开放决策 Q2） |
| `V3Th` | `DataTableTh` 或 `Th` | 同左 | 推荐短名 `Th`（仅从 superteam 导出时安全） |
| `V3Td` | `Td` | 同左 | |
| `V3Tr` | `Tr` | 同左 | |
| `V3EmptyState` | `EmptyState` | 同左 | |
| `V3LoadingState` | `LoadingState` | 同左 | |
| `V3ErrorState` | `ErrorState` | 同左 | |
| `V3PermissionDenied` | `PermissionDenied` | 同左 | |
| `V3StateSurface` | `StateSurface` | 同左 | |
| `V3MetricCard` | `MetricCard` | 同左 | |
| `V3Segmented` | `Segmented` | 同左 | 备选 `SegmentedControl` |
| `V3Tabs` | `PageTabs` | 同左 | **不用**裸 `Tabs`，避 `ui/tabs` |
| `V3TabList` | `PageTabList` | 同左 | |
| `V3Tab` | `PageTab` | 同左 | |
| `V3PageHeader` | `PageHeader` | 同左 | 注意 layout 是否已有 shell header 概念 |
| `V3ToolbarSearch` | `ToolbarSearch` | 同左 | |
| `V3Pagination` | `Pagination` | 同左 | 与 data-table 分页组件名冲突时改为 `TablePagination` vs 本 `Pagination` 分文件 |

### 1.2 保持不变

| 名 | 原因 |
| --- | --- |
| `SoftCard` `GlassCard` `SignatureCard` `WorkSurface` | 已是语义名 |
| `StatusPill` `IconTile` | 已是语义名 |
| `ObjectRef` `ObjectIdChip` | 已是语义名 |
| `MasterDetailLayout` `MetricGrid` | 已是语义名 |
| `MarkdownProse` 及 team/user 组件 | 无关 V3 |

### 1.3 文件名

| 现路径 | 目标路径 | 备注 |
| --- | --- | --- |
| `components/superteam/v3-components.tsx` | `components/superteam/primitives.tsx`（或拆分 `button.tsx` `table.tsx` `states.tsx`） | 阶段 B 可先改名单文件，拆分可选 |
| `components/superteam/v3-components.test.tsx` | 与实现同名 | |
| `styles/v3-shell-background.test.tsx` | `styles/shell-background.test.tsx` | 阶段 E/F |

### 1.4 过渡策略（组件）

```ts
// primitives.tsx
export function Button(...) { ... }
/** @deprecated 使用 Button */
export const V3Button = Button
```

- 窗口期：阶段 B 合并后保留 alias，直到阶段 D 结束且 rg 无 `V3Button` 业务引用再删。
- 测试：可保留 1 个用例断言 deprecated 仍可导入（可选）。

---

## 2. 与 shadcn 重名冲突 — 决议表

| 冲突 | 推荐决议（草案） | 替代决议 |
| --- | --- | --- |
| `Button` | **业务 `Button` = superteam 导出**；`ui/button.tsx` 继续存在但：① 样式同源 `buttonVariants`；② features/routes **禁止** import；③ 仅 ui 内部、data-table、layout 等白名单可 import `ui/button` | 将 `ui/button` 改名为 `UiButton` / `PrimitiveButton` 并改所有内部引用 |
| `Table` | superteam 用 **`DataTable` 包装名** + `Th`/`Td`/`Tr`，不导出 `Table` | 或 `ui/table` 改内部名 |
| `Tabs` | superteam 用 **`PageTabs`**；`ui/tabs` 保留给 Radix tabs 场景 | 逐步让业务也迁到 PageTabs（非本迁移必达） |
| `Pagination` | superteam `Pagination`；data-table 内联按钮分页保持未导出冲突 | 冲突则 superteam 名 `SimplePagination` |
| `Badge` | 不引入 `Badge` 作为状态组件；状态用 `StatusPill`，筛选用 `Chip`；`ui/badge` 仅白名单 | |
| `Card` | 业务用 `SoftCard`/`WorkSurface`；`ui/card` 白名单或清零 features | |

**Import 规则（目标）**：

```text
允许：
  features/routes → @/components/superteam
  features/routes → @/components/ui/{dialog,sheet,input,select,…}（非 button/badge/card）
  components/ui/** → @/components/ui/button（内部）
  components/data-table/** → @/components/ui/button（阶段 D 后可改为 superteam Button）
  components/superteam/** → 可依赖 ui 非视觉双轨件（input 等）；按钮应自用 Button

禁止：
  features/**, routes/** → @/components/ui/button|badge|card
```

是否阶段 D 后连 data-table 也改 superteam `Button`：开放决策 Q4（建议 **是**，但可后置）。

---

## 3. CSS 变量映射（`--v3-*` → 语义名）

### 3.1 核心面与品牌

| 现 CSS 变量 | 目标 CSS 变量 | Tailwind 目标色名（`bg-`/`text-`/`border-`） | 说明 |
| --- | --- | --- | --- |
| `--v3-bg` | `--bg` 或与 `--background` 合并 | `bg` 或继续 `background` | **Q5**：是否合并进 shadcn `--background` |
| `--v3-card` | `--surface` **或** 对齐 `--card` | `surface` 或 `card` | **Q5**：与 shadcn `--card` 合并推荐 |
| `--v3-card-soft` | `--surface-soft` | `surface-soft` | |
| `--v3-card-inner` | `--surface-inner` | `surface-inner` | |
| `--v3-ink` | `--ink` | `ink` | |
| `--v3-ink-2` | `--ink-2` | `ink-2` | |
| `--v3-ink-3` | `--ink-3` | `ink-3` | |
| `--v3-line` | `--line` | `line` | |
| `--v3-line-strong` | `--line-strong` | `line-strong` | |
| `--v3-brand` | `--brand` | `brand` | 可与 `--primary` 互指 |
| `--v3-brand-deep` | `--brand-deep` | `brand-deep` | |
| `--v3-brand-soft` | `--brand-soft` | `brand-soft` | |
| `--v3-brand-grad` | `--brand-grad` | （gradient，多 var 引用） | |

**已决（Q5 = A 真值合并 · 2026-07-24）**：

- `--v3-brand` 真值写入，`--primary: var(--brand)`（或反向：brand 别名 primary）——保证主色单一真值。
- `--v3-card` 与 `--card` 合并为同一真值；Tailwind 业务新代码用 `bg-card` **或** 双暴露 `bg-surface`=`bg-card`。  
  为减少 class 大爆发，可：**变量去 v3，Tailwind 类名 `bg-v3-card`→`bg-card`**（若与现 shadcn `bg-card` 已一致则只是去前缀）。

### 3.2 语义色

| 现 | 目标 | TW |
| --- | --- | --- |
| `--v3-info` / `-text` / `-soft` | `--info` / `--info-text` / `--info-soft` | `info`… |
| `--v3-ok` … | `--ok` … | `ok`… |
| `--v3-warn` … | `--warn` … | `warn`… |
| `--v3-danger` … | `--danger` … | `danger`…（注意 vs `destructive`） |
| `--v3-artifact` … | `--artifact` … | `artifact`… |
| `--v3-mute` … | `--mute` … | `mute`… |

`destructive`：保持 shadcn 名；值可 `var(--danger)`。本迁移不删 `destructive`。

### 3.3 圆角与阴影

| 现 | 目标 | TW |
| --- | --- | --- |
| `--v3-r-card` | `--radius-card` | `rounded-card`（`@theme --radius-card`） |
| `--v3-r-inner` | `--radius-inner` | `rounded-inner` |
| `--v3-shadow` | `--shadow-card` | `shadow-card` |
| `--v3-shadow-pop` | `--shadow-pop` | `shadow-pop` |

### 3.4 Shell

| 现 | 目标 |
| --- | --- |
| `--v3-shell-bg` | `--shell-bg` |
| `--v3-shell-background` | `--shell-background` |
| `--v3-shell-grid` | `--shell-grid` |
| `--v3-shell-glass` | `--shell-glass` |
| `--v3-shell-glass-strong` | `--shell-glass-strong` |
| `--v3-shell-glass-border` | `--shell-glass-border` |
| `--v3-shell-glass-blur` | `--shell-glass-blur` |
| `--v3-shell-glass-saturate` | `--shell-glass-saturate` |
| `--v3-shell-control` | `--shell-control` |
| `--v3-shell-control-hover` | `--shell-control-hover` |
| `--v3-shell-control-active` | `--shell-control-active` |
| `--v3-shell-control-border` | `--shell-control-border` |
| `--v3-shell-menu-hover` | `--shell-menu-hover` |
| `--v3-shell-search` | `--shell-search` |
| `--v3-shell-search-border` | `--shell-search-border` |
| `--v3-shell-search-shadow` | `--shell-search-shadow` |

### 3.5 Signature

| 现 | 目标 |
| --- | --- |
| `--v3-signature-*` | `--signature-*`（后缀不变） |

### 3.6 Aurora

| 现 | 目标 |
| --- | --- |
| `--v3-aurora-*` | `--aurora-*` |

### 3.7 Layout / metric

| 现 | 目标 |
| --- | --- |
| `--v3-layout-contained` | `--layout-contained` |
| `--v3-layout-wide` | `--layout-wide` |
| `--v3-layout-rail` | `--layout-rail` |
| `--v3-layout-rail-lg` | `--layout-rail-lg` |
| `--v3-metric-min` | `--metric-min` |
| `--v3-metric-max` | `--metric-max` |

### 3.8 双挂期写法（阶段 E 强制）

```css
:root {
  --brand: #2f5fff;
  --v3-brand: var(--brand); /* deprecated alias */
}
@theme inline {
  --color-brand: var(--brand);
  --color-v3-brand: var(--brand); /* deprecated */
}
```

删除旧名的前提：`rg --v3-` / `v3-brand` 在 `apps/web/src` 为 0（不含本迁移文档）。

---

## 4. Tailwind 类名映射（高频）

| 现类名片段 | 目标 |
| --- | --- |
| `text-v3-ink` / `text-v3-ink-2` / `text-v3-ink-3` | `text-ink` / `text-ink-2` / `text-ink-3` |
| `bg-v3-card` / `bg-v3-card-soft` / `bg-v3-card-inner` | `bg-card` 或 `bg-surface*`（随 Q5） |
| `border-v3-line` / `border-v3-line-strong` | `border-line` / `border-line-strong` |
| `bg-v3-brand` / `text-v3-brand` / `text-v3-brand-deep` | `bg-brand` / `text-brand` / `text-brand-deep` |
| `bg-v3-brand-soft` | `bg-brand-soft` |
| `text-v3-ok` / `bg-v3-ok-soft` / `text-v3-ok-text` | 去 `v3-` 段 |
| 同理 warn/danger/info/artifact/mute | 去 `v3-` 段 |
| `rounded-v3-card` / `rounded-v3-inner` | `rounded-card` / `rounded-inner` |
| `shadow-v3` / `shadow-v3-pop` | `shadow-card` / `shadow-pop` |
| `bg-v3-bg` | `bg-bg` 或 `bg-background`（Q5） |
| `var(--v3-layout-*)` | `var(--layout-*)` |
| `var(--v3-shell-*)` | `var(--shell-*)` |
| `var(--v3-aurora-*)` | `var(--aurora-*)` |

**Codemod 注意**：

- 只替换独立 token 段，避免误伤 `design-direction-v3` 路径字符串（应用路径白名单：主要 `apps/web/src`）。
- `v3-inner` 来自 `rounded-v3-inner`，不要全局把英文句子里的 `v3` 清掉。

---

## 5. 全局 CSS 类映射

| 现 | 目标 |
| --- | --- |
| `.v3-aurora-background` | `.aurora-background` |
| `.v3-glass` | `.glass` 或 `.surface-glass`（**Q6**，推荐 `.glass`） |
| `.v3-glass-inner` | `.glass-inner` |

过渡：双 class 同规则，或 `@apply` 旧名指向新名，再删旧名。

---

## 6. `data-slot` 映射

| 现 | 目标（草案） |
| --- | --- |
| `v3-soft-card` | `soft-card` |
| `v3-glass-card` | `glass-card` |
| `v3-signature-card` | `signature-card` |
| `v3-work-surface` | `work-surface` |
| `v3-icon-tile` | `icon-tile` |
| `v3-status-pill` | `status-pill` |
| `v3-metric-card` | `metric-card` |
| `v3-table` | `data-table` 或 `work-table` |
| `v3-button` | `app-button`（避 shadcn `button`）**Q3** |
| `v3-icon-button` | `icon-button` |
| `v3-chip` | `chip` |
| `v3-tabs` | `page-tabs` |
| `v3-tab-list` | `page-tab-list` |
| `v3-tab` | `page-tab` |
| `v3-segmented` | `segmented` |
| `v3-page-header` | `page-header` |
| `v3-toolbar-search` | `toolbar-search` |
| `v3-pagination` | `pagination` |
| `v3-empty-state` | `empty-state` |
| `v3-loading-state` | `loading-state` |
| `v3-error-state` | `error-state` |
| `v3-permission-denied` | `permission-denied` |
| `v3-authenticated-shell` | `authenticated-shell` |
| `v3-auth-shell` | `auth-shell` |
| `v3-shell-header` | `shell-header` |
| `v3-aurora-background` | `aurora-background` |

`index.css` 中所有选择器同步改；测试同步改。

---

## 7. 文档用语映射

| 现用语 | 目标用语 |
| --- | --- |
| v3 Soft-Flat / v3 基线 | Soft-Flat / 设计基线 |
| V3 组件 / V3Button | 项目组件 / `Button`（`@/components/superteam`） |
| v3 token / `--v3-*` | Soft-Flat token / `--brand` 等 |
| design-direction-v3（路径） | 保留路径；称呼「Soft-Flat 方向历史原型」 |
| 「Button 或 V3Button」 | 仅 `superteam` 的 `Button` |

---

## 8. 开放决策汇总（必须人类点头）

| ID | 问题 | 草案推荐 | 影响面 |
| --- | --- | --- | --- |
| Q1 | `V3Tone` → `Tone` 还是 `SemanticTone`？ | `Tone` | 类型名 ~142 处 |
| Q2 | 表格套件是否避开 `Table` 用 `DataTable`+`Th/Td/Tr`？ | 是 | 中 |
| Q3 | 业务按钮 `data-slot` 用 `app-button` 还是 `button`？ | `app-button` | 测试 |
| Q4 | data-table 是否最终也改用 superteam `Button`？ | **已决：否（本迁移不强制）**；C 同源后 data-table 白名单继续 `ui/button`；可选后续清洁 PR | data-table |
| Q5 | `--v3-card/bg` 与 shadcn `--card/--background` 合并策略？ | **已决：选项 A 真值合并**；`bg-v3-card`→`bg-card`，`bg-v3-bg`→`bg-background`；独有 token 去前缀 | **最大** |
| Q6 | `.v3-glass` → `.glass` 还是 `.surface-glass`？ | `.glass` | 小 |
| Q7 | 是否允许永久 `--st-*` 前缀作为备选？ | **否** | 全局 |
| Q8 | alias 窗口最短/最长？ | 最短：B 后全仓已无旧引用即可删；最长不超过 E 结束 | 工程 |
| Q9 | 阶段 C 对齐 ui/Button 到 Soft-Flat 尺寸（h-10、rounded-xl、去掉 sm pill）是否批准为「非改版的消歧」？ | **已决：是（人类批准）** | 截图 |
| Q10 | 历史 prototypes 路径是否改名？ | **否**，仅文档声明 | 低 |

---

## 9. 映射完整性自检清单

- [ ] 每一 `V3*` 导出符号有行
- [ ] theme.css 每一个 `--v3-*` 有行（含 aurora/shell/signature/layout）
- [ ] 每一个 `data-slot="v3-*"` 有行
- [ ] 三个全局 `.v3-*` class 有行
- [ ] `@theme` 中每一 `--color-v3-*` / `--radius-v3-*` / `--shadow-v3*` 有对应目标
- [ ] 冲突表覆盖 button/table/tabs/badge/card/pagination
- [ ] 开放决策均有推荐答案
