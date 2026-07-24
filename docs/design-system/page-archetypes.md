# 页面类型蓝图

## 何时阅读

新建菜单页、改造列表/详情/向导/工作台，或不确定该用 `contained` / `wide` / `canvas`、SoftCard / WorkSurface / GlassCard 时阅读。与 `layout-density.md`（宽度与断点）、`surfaces.md`（容器）配套使用。

## 目标

把「容器选择」落成可抄的页面骨架，减少各 feature 自造 KPI 带、筛条、空态和 Tab 顺序。

业务字段、接口、状态机仍来自当前功能；本文件只约束结构与槽位。

## 总表（默认骨架）

| 页面类型 | `Main` 宽度 | 主容器 | 页头 | 主区 | 辅区 | 主 CTA |
| --- | --- | --- | --- | --- | --- | --- |
| 实体目录 | `wide` | SoftCard 包网格或表 | `ShellPageHeader` | 筛条 + 卡/表 | 可选侧栏队列 | 新建/创建（唯一主按钮） |
| 主从工作台 | `wide` + 常 `fixed` | 左密表/列表 + 右详情 | 同上 | `MasterDetailLayout` | 详情按需，无选中不占空栏 | 上下文动作，避免双主 CTA |
| 创建/上传向导 | `contained` 或 `canvas`（沉浸） | SoftCard 或 Tier A `GlassCard` | 步骤标题 + 返回 | 分步表单 | 右侧摘要/预检（可选） | 下一步 / 提交 |
| 运营/对象详情 | `wide` | 头卡 SoftCard + 多 Tab | 对象头（名+状态+主操作） | Tab 内容 | 风险/待办/成员轨 | 随状态变化，同时仅一个主 CTA |
| 密集审计/日志 | `wide` 或 `canvas`（多 Tab 日志） | **仅** WorkSurface | 筛选 + 说明 | 密表、sticky 头 | 默认无重详情栏 | 导出/刷新为次要 |
| 设置/单对象编辑 | `contained` | SoftCard | 标题+说明 | 表单分组 | 无 | 保存 |
| 命令/发起画布 | `canvas` | Tier A 玻璃外壳 + 实底输入 | 轻页头或融入画布 | 命令面/模式选择 | 项目/上下文选择 | 提交/发送 |
| 占位/未实现 | `contained` | SoftCard + `EmptyState` 类 | 标题+原因 | 说明 + 返回入口 | 无 | 返回/文档链（次要） |

代码锚点：

- 宽度：`Main`（`apps/web/src/components/layout/main.tsx`）
- 页头：`ShellPageHeader` / `ShellPageHeaderBack`
- 主从：`MasterDetailLayout`（`components/superteam/layout.tsx`）
- 容器：`SoftCard` / `WorkSurface` / `GlassCard`（`primitives.tsx`）
- 四态：`LoadingState` / `EmptyState` / `ErrorState` / `PermissionDenied` / `StateSurface`
- 筛条：`ListToolbar`（+ `ToolbarSearch` / `Chip` / `Segmented`）
- 浮层：`SoftDialog*` / `SoftSheet*` / `ConfirmDialog`；行内提示：`Callout`
- 目录/详情：`EntityCard` / `ObjectHeader` / `DescriptionList` / `ActionMenu`
- 局部骨架：`TableSkeleton` / `CardGridSkeleton` / `DetailSkeleton`
- 向导/审计：`Stepper` / `Timeline` / `Progress` / `FileDropzone` / `CopyableMono`

## 各类型细则

### 1. 实体目录（项目/员工/技能/团队列表）

- **必须**：`ShellPageHeader`（标题、一句职责说明、主 CTA）；筛选用 `ListToolbar`（可折叠实现仍可本地）；空列表用 `EmptyState` 且给「创建」路径。
- **推荐**：顶部 KPI 仅 3～6 个，用 `MetricCard` 或统一指标带；目录卡用 `EntityCard`，禁止每页自造第三套 pill 卡。
- **禁止**：营销 hero；目录页整页玻璃；主 CTA 与次要「模板管理」同权重实心蓝。

### 2. 主从工作台（收件箱、部分权限/运行面）

- **必须**：未选中不渲染空详情柱；窄容器详情进 Sheet（`MasterDetailLayout` 已处理）。
- **推荐**：列表行一主信息 + 状态 + 时间；详情区固定「摘要 → 上下文 → 动作」。
- **禁止**：左右同时多个实心主按钮争抢。

### 3. 创建/上传向导

- **必须**：可见步骤用 `Stepper`；返回列表用 `ShellPageHeaderBack`；提交中按钮 loading 且防重复。
- **Tier A**：仅发起/上传/登录类沉浸页可用 `GlassCard`；表单项实底。
- **禁止**：向导内嵌密集审计表还套玻璃。

### 4. 运营/对象详情

- **必须**：头区用 `ObjectHeader`（名称经 `ObjectRef`），UUID 仅 chip；状态经 `status-labels`。
- **Tab**：业务 Tab 用路由或受控 Tabs；避免同页两套 Tab 视觉（优先 `PageTabs` 或统一 Radix Tabs 皮肤）。
- **禁止**：详情页复制一套与目录完全不同的按钮圆角/颜色体系。

### 5. 审计/日志

- **必须**：WorkSurface + 实底；筛选与表一体；错误/空态区分「无数据」与「加载失败」。
- **禁止**：Glass；装饰性大图；主 CTA 抢镜。

### 6. 命令/发起画布（任务中枢等）

- **必须**：一种发起模式有清晰选中态；提交前校验项目/必填；与全局命令菜单（⌘K）视觉同源（见 `visual-language.md` / `actions.md`）。
- **可以**：`canvas` 宽、玻璃外壳。
- **禁止**：把画布页做成无结构的自由拖拽营销页。

## 选型流程（实现时）

1. 是否逐行扫读比较？→ 是则内容本体 **WorkSurface**。
2. 是否 Tier A 沉浸入口？→ 才考虑 **GlassCard**。
3. 其余外壳 → **SoftCard**。
4. 选 `Main` 宽度：表单/设置 `contained`；目录/主从/密表 `wide`；画布/多面板 `canvas`。
5. 对照上表补齐：页头、主 CTA、空/错/载（见 `feedback.md`）。

## 检查清单

- [ ] 能指出本页属于上表哪一类（或写明为何例外）
- [ ] `Main width` 与类型一致
- [ ] 主容器符合三容器规则
- [ ] 有且仅有一个主 CTA 视觉权重（或说明无主 CTA 的只读页）
- [ ] 未选中主从不留空右栏
- [ ] 审计/日志未使用玻璃
