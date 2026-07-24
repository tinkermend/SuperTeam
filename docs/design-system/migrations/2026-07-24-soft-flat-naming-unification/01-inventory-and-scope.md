# 01 · 现状盘点与范围

> **快照日期**：2026-07-24（起草时）  
> **刷新要求**：任一实现阶段开工前，重跑文末「Inventory 命令」并更新本节数字与日期。  
> **范围根**：`/Users/tinker/src/singe/SuperTeam`，代码主战场 `apps/web`。

## 1. 范围纳入

| 区域 | 路径 | 纳入原因 |
| --- | --- | --- |
| 项目组件 | `apps/web/src/components/superteam/**` | `V3*` 定义与导出 |
| 业务功能 | `apps/web/src/features/**` | 大量 `V3*` 与 `v3-*` class |
| 路由页 | `apps/web/src/routes/**` | 同上 + 测试 data-slot |
| 布局壳 | `apps/web/src/components/layout/**` | `data-slot=v3-*`、shell token |
| 样式 | `apps/web/src/styles/theme.css`, `index.css` | token 与 `.v3-glass` |
| shadcn ui | `apps/web/src/components/ui/{button,badge,card,table,tabs}.tsx` | 双轨与重名冲突 |
| data-table | `apps/web/src/components/data-table/**` | 依赖 `ui/button` |
| 设计文档 | `DESIGN.md`, `docs/design-system/*` | 现行规范语言 |
| 校验脚本 | `docs/design-system/verify-design-system.mjs` | 字符串/路径校验 |
| Web 测试 | `apps/web/src/**/*.test.tsx` 等 | data-slot / class 断言 |

## 2. 范围排除（默认不动）

| 区域 | 原因 |
| --- | --- |
| `docs/prototypes/**` 文件路径与历史 HTML | 视觉参考稿；阶段 F 仅加说明，不强制改路径 |
| `docs/superpowers/plans|specs/**` 历史方案 | 考古文档；不批量改写 |
| `CHANGELOG.md` 历史条目正文 | 避免伪造历史；可选文首注记 |
| `apps/control-plane`, `apps/runtime-agent`, `contracts` | 无本迁移目标 |
| `packages/ui` 空包 | 不在本迁移激活 |
| 业务逻辑、API、文案内容（非组件名） | 非目标 |

## 3. 组件标识符盘点（`V3*`）

统计口径：`apps/web/src` 内 `\bV3[A-Za-z0-9]+\b`（含测试；排除 `__screenshots__`）。

| 标识符 | 约计出现次数 | 角色 |
| --- | --- | --- |
| `V3Button` | 430 | 公共按钮 |
| `V3Td` | 424 | 表格单元格 |
| `V3Th` | 387 | 表头 |
| `V3Tr` | 151 | 表行 |
| `V3Tone` | 142 | 语义色调类型 |
| `V3Table` | 110 | 表格容器 |
| `V3EmptyState` | 109 | 空态 |
| `V3ErrorState` | 91 | 错误态 |
| `V3LoadingState` | 90 | 加载态 |
| `V3MetricCard` | 55 | 指标卡 |
| `V3Chip` | 38 | 筛选 chip |
| `V3StateSurface` | 26 | 状态面组合 |
| `V3Tab` / `V3Tabs` / `V3TabList` | 16 / 15 / 12 | 页内 tabs |
| `V3Segmented` | 15 | 分段控件 |
| `V3Pagination` | 13 | 分页 |
| `V3IconButton` | 11 | 图标按钮 |
| `V3ToolbarSearch` | 11 | 工具条搜索 |
| `V3PageHeader` | 5 | 页头 |
| `V3PermissionDenied` | 5 | 无权限 |
| `V3ButtonVariant` / `V3ButtonSize` / `V3Density` | 3 / 3 / 1 | 类型 |

**定义文件**：`apps/web/src/components/superteam/v3-components.tsx`  
**桶导出**：`apps/web/src/components/superteam/index.ts` → `export * from './v3-components'`

**已无 V3 前缀、应保持的好名字**（对照范本）：

- `SoftCard`, `GlassCard`, `SignatureCard`, `WorkSurface`
- `StatusPill`, `IconTile`
- `ObjectRef`, `ObjectIdChip`
- `MasterDetailLayout`, `MetricGrid`
- `UserIdentity*`, `TeamIcon*`, `TeamRole*`

## 4. CSS 变量盘点（`--v3-*`）

`theme.css` 中定义约 **90** 个 `--v3-*` 名（含 light 定义；dark 覆写同名）。

族：

| 族 | 示例 | 备注 |
| --- | --- | --- |
| 基础面/字/线 | `--v3-bg/card/ink/line/brand…` | 高频 Tailwind 颜色 |
| 语义色 | `--v3-info/ok/warn/danger/artifact/mute` + `-soft/-text` | |
| 圆角阴影 | `--v3-r-card/r-inner`, `--v3-shadow/shadow-pop` | `@theme` → `rounded-v3-*` / `shadow-v3*` |
| Shell | `--v3-shell-*` | 多通过 `var(--v3-…)`，未必都有 `bg-v3-shell-*` 类 |
| Signature | `--v3-signature-*` | 多 inline/var |
| Aurora | `--v3-aurora-*` | 玻璃与极光 |
| Layout | `--v3-layout-contained/wide/rail…` | Main / MasterDetail |
| Metric | `--v3-metric-min/max` | MetricGrid |

`@theme inline` 当前暴露的 Tailwind 色/半径/阴影（节选）：

- `--color-v3-{bg,card,card-soft,card-inner,ink,ink-2,ink-3,line,line-strong,brand,…语义色全套}`
- `--radius-v3-card`, `--radius-v3-inner`
- `--shadow-v3`, `--shadow-v3-pop`

**注意**：`--v3-shell-*`、`--v3-aurora-*`、`--v3-signature-*`、`--v3-layout-*` **多数未**以 `bg-v3-shell-…` 形式进 `@theme`，调用点常用 `var(--v3-…)` / arbitrary value。token 改名必须覆盖这两种形式。

## 5. 工具类 / 字符串盘点（`v3-*`）

`apps/web/src` 内 `\bv3-[a-z0-9-]+\b` 约 **128** 个不同 token 字符串；最高频：

| token | 约计 |
| --- | --- |
| `v3-ink` / `v3-ink-2` / `v3-ink-3` | 501 / 440 / 447 |
| `v3-line` / `v3-line-strong` | 347 / 71 |
| `v3-brand` 及相关 | 249+ |
| `v3-card` 及相关 | 186+ |
| `v3-danger/warn/ok/info…` | 见 inventory 脚本 |
| `v3-inner`（来自 `rounded-v3-inner`） | 100 |
| `v3-glass-inner` / `v3-glass` | class 与字符串 |

全局 CSS 类：

- `.v3-aurora-background`
- `.v3-glass`
- `.v3-glass-inner`

## 6. `data-slot` 盘点

测试大量依赖 `data-slot="v3-…"`。完整列表见 `02-naming-map.md`。

高频：`v3-soft-card`, `v3-status-pill`, `v3-table`, `v3-work-surface`, `v3-page-header`, `v3-icon-tile`, `v3-authenticated-shell`, `v3-shell-header`, `v3-auth-shell`…

**风险**：改 slot 而不改测试 = 红；只改测试不改组件 = 假绿。必须同 PR。

## 7. 双轨（shadcn vs superteam）盘点

### 7.1 `ui/button` 引用（非测试）

约 36 文件，分布：

- **基础设施**：`data-table/*`, `ui/alert-dialog`, `ui/calendar`, `ui/sidebar`, `layout/top-nav`, `config-drawer`, `date-picker`, `profile-dropdown`, `learn-more`
- **superteam 内部**：`user-search-select`, `team-icon-picker` ← 设计层反向依赖 ui，优先清理
- **features**（约 20）：teams 创建流、projects 创建/提交、employees 部分、errors/*、runtime、users drawer、task-launches prompt dialog

### 7.2 `ui/badge` / `ui/card`

- badge：features 3 + data-table + `superteam/team-role`
- card：features 仅 2（`skills/upload`, `employee-capabilities-panel`）

### 7.3 其它潜在重名

| shadcn | superteam V3 | 冲突若去前缀 |
| --- | --- | --- |
| `ui/button` → `Button` | `V3Button` → `Button` | **硬冲突** |
| `ui/table` | `V3Table` | 导出名冲突（table 引用面目前很小） |
| `ui/tabs` | `V3Tabs` | features 多处用 ui/tabs |
| `ui/badge` | （无直接 V3Badge；有 StatusPill/Chip） | 语义重叠 |
| `ui/card` | SoftCard（已不同名） | 较安全 |

### 7.4 文档制度问题

`docs/design-system/actions.md` 现行写：主按钮可用 `Button` **或** `V3Button`。  
`docs/design-system/tokens.md` 仍以 v3 为章节标题与类名教学。

## 8. 测试触面

至少 **34** 个测试文件直接出现 `V3` / `v3-` / `--v3`，包括：

- `components/superteam/v3-components.test.tsx`（核心）
- `styles/v3-shell-background.test.tsx`
- layout / auth / workflows / permissions / runtime / audit / costs / logs 等

许多断言绑定：

- `data-slot="v3-…"`
- class：`bg-v3-card`, `text-v3-ink-2`, `rounded-v3-card`
- 偶发 `bg-[var(--v3-shell-control)]`

截图目录 `__screenshots__` 可能在 class 变化后像素差；阶段 B 纯组件改名通常不改 class；阶段 E 会大面积影响。

## 9. 文档触面

- 现行必更新：`DESIGN.md` + `docs/design-system/*.md` + `verify-design-system.mjs`
- 提及 v3 的 docs 文件起草时约 **86** 个（含 prototypes 与历史 plans）——**不要求**全部改写

## 10. 已知「幽灵 / 不一致」点（实施时要处理或登记）

| 项 | 说明 |
| --- | --- |
| `shadow-v3-soft` | 代码/文档偶有提及；`@theme` 仅有 `shadow-v3` / `shadow-v3-pop`。改名时核对是否无效 class |
| tokens.md 与 theme 色值 | 例如 warn 表格与 theme 注释公式可能有历史差；**本迁移不借机改色**，只搬家 |
| `unimplemented-page` LegacyTone | `primary/success/warning/neutral/decision` → 映射到 `V3Tone`；改名时保留映射 |
| `Main` deprecated `contained`/`fluid` | 与本迁移无关，勿顺手删（除非单独 PR） |
| feature 局部 CSS | `task-launch-aurora.css` 等可能含 token；E 阶段必须扫 `*.css` |
| 空 `packages/ui` | 勿在迁移中「顺便填满」 |

## 11. Inventory 刷新命令（开工前必跑）

在仓库根目录：

```bash
# 1) V3 标识符
rg -o --no-filename '\bV3[A-Za-z0-9]+\b' apps/web/src -g '!**/__screenshots__/**' | sort | uniq -c | sort -rn

# 2) --v3 变量（theme 定义）
rg -o --no-filename '--v3-[a-z0-9-]+' apps/web/src/styles/theme.css | sort -u | wc -l

# 3) v3-* 字符串
rg -o --no-filename '\bv3-[a-z0-9-]+' apps/web/src -g '!**/__screenshots__/**' | sort | uniq -c | sort -rn | head -60

# 4) data-slot
rg -o --no-filename 'data-slot=["'\'']v3-[^"'\'']+["'\'']' apps/web/src | sort | uniq -c | sort -rn

# 5) ui/button 业务引用
rg -l "from [\"']@/components/ui/button[\"']" apps/web/src/features apps/web/src/routes

# 6) 测试触面
rg -l 'V3|v3-|--v3' apps/web/src -g '**/*.{test,spec}.*'
```

（若 shell 对 glob 敏感，按本机 `rg` 习惯微调；以无 `__screenshots__` 为准。）

将新输出摘要贴回本节顶部「快照日期」下。

## 12. 范围冻结规则

- 实现 PR 不得扩大到「顺便重构 inbox 布局」等。
- 若必须碰映射表外符号：先 PR/提交更新 `02-naming-map.md`，再写代码。
- 与其它会话文件交织时：只 stage 本迁移路径；冲突文件拆 worktree。
