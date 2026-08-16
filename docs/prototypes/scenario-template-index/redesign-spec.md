# 场景模板首页 · 链入表落地方案

状态：已落地（Console 列表页）。对照原型：`c-inline-rail-table.html`。编辑页方案见 `docs/prototypes/scenario-template-composer/redesign-spec.md`，本文只改 **列表页** `/scenario-templates`。

## 0. 已拍板

1. 布局用 **链入表**（方案 C）。不做剧目卡、不做主从预览栏。
2. **去掉展开层和版本历史**。产品面无「升版」；`active_version` / versions API 本页不出现、不请求。
3. 数据只吃 `GET /api/v1/scenario-templates`（主表 + `spec`）。不打 `role-view`、`versions`、`playbook-readiness`。
4. **顶部**：`ShellPageHeader`（标题 + 一句职责）+ `Main` 内主 CTA「新建模板」。**不要 MetricCard**。
5. **筛选**：状态三档（全部 / 启用中 / 已禁用，默认全部）+ **名称搜索**（客户端，匹配 `name` 与 `template_key`）。不要按席位/步数筛。状态 chip 个数来自未过滤全集。
6. **分页**：客户端切页，默认每页 10，可选 10/20/50。筛/搜/改页大小回到第 1 页。不新增列表 API query。
6. 启停 / 删除确认框文案保持现网，不把停用后果写进每一行。

## 1. 结论

首页是治理用的实体目录：扫有哪些骨架、最深能收到哪一档，然后编辑 / 启停 / 删除。新规划只匹配启用中的模板——这句话放页头副文案和停用确认框，不靠指标卡重复。

作者心智与编辑页一致：点「编辑」改当前配置；「保存更改」后台仍写不可变版本行，只给进行中规划钉 spec。

## 2. 接口对照

列表项来自 `listScenarioTemplates`：`name`、`description`、`template_key`、`status`、`updated_at`、`spec`（`roles` / `skeleton` / `exits`）。

| 列 / 控件 | 口径 | 来源 |
|---|---|---|
| 模板名称 / 描述 | 原样 | `name` / `description` |
| key | 等宽补充，不作主指称 | `template_key` |
| 骨架链 | **派生**，见 §3 | `spec.skeleton` + `exits` + `roles` |
| 最深收口 | `exits` 数组最后一项的 label，空则 deliverable | `spec.exits`（浅→深，与现展开区、规划确认卡同一顺序约定） |
| 状态 pill | `statusLabel` | `active` → 启用中；`disabled` → 已禁用 |
| 更新 | 相对时间 | `updated_at`（只展示，**不**拿来重排） |
| 名称搜索 | 前端滤 name / template_key | 列表 API 无 query |
| 分页 | 前端 slice | 默认 10，选项 10/20/50 |
| 启停 / 删除 | 现确认框 | `PATCH` status；`DELETE` 软删 |

列表 SQL 已是 `ORDER BY created_at ASC, template_key ASC`。本页 **保持服务端顺序**，不要改成 `updated_at` 倒序——这是注册表不是任务队列，种子/创建顺序更稳。`DESIGN.md`「工作对象默认新近优先」适用于任务/审批，不套到模板登记。

### 2.1 明确不做

| 能力 | 原因 |
|---|---|
| `GET .../versions`、`active_version` | 产品面无升版 |
| `GET .../role-view` | 出口所需角色、独立性、持有人数是编制/规划语义；禁止 N+1 |
| `playbook-readiness` | 绑定项目；列表无项目 |
| 展开验收判据 | 给编辑/规划用 |
| 人类门 / 独立验证标记 | 检修台的事，列表不画第二套约束 |
| 列表筛选 API、按更新时间重排 | 量小；顺序跟登记 |
| 本页搜索 / URL 同步筛选 | 量小；Cmd+K 已在壳上。筛选只活在组件 state |
| 按 `template_key === "generic"` 特判 | 只认空 `skeleton`，文案走 §3.4 |

`listScenarioTemplateVersions` 可留在 API 客户端，本页不再调用。

## 3. 骨架链派生（禁止第二套语义）

### 3.1 不要直接 `composerFromSpec`

`composerFromSpec` 在空骨架时会 `emptyComposerDraft()` **塞一个占位站**，那是编辑器起步用的。列表若复用，generic 会画出假节点。

抽只读函数（建议 `spec-composer.ts` 旁或同文件导出，供列表与单测共用），例如 `listChainFromSpec(spec)`：

- 复用 `isSerialSkeleton`
- **不要**调用 `composerFromSpec` / `emptyComposerDraft`

### 3.2 可收口

骨架站没有 `exit` 字段。与规划侧 `StepByProduce` 对齐：**该站 `produces_defaults` 任一名**等于某个 `exits[].deliverable`，则该站可收口。

不要只对 `produces_defaults[0]`（编辑器 `composerFromSpec` 目前如此）。种子 `software_delivery` 的 develop 同时产 `branch_ref` 与 `head_commit`，出口可能对其中任一个。

没有 `produces_defaults` 时：不要用编辑器的 `${step}_outcome` 去猜出口。对不上就不点蓝点。v1 无 `exits` 的模板：整链无蓝点，最深收口列走 §3.5。

蓝点：品牌色小圆，`aria-label="可收口"`，`title` 用该站匹配到的 `exits[].label`。不要在节点上写「· 可收口」。**不要**页级图例条。

### 3.3 站名

`skeleton[].title`（若有）→ 该站 `role` 在 `spec.roles` 的 `title` → 否则 `step` 键。禁止手写种子剧情名。不请求角色词表。

### 3.4 串行 / 并行 / 空

- 空 `skeleton`：**「无骨架 · generic 行为」**。不编造占位站。
- `isSerialSkeleton` 为真：一站一组，`A → B → C`。
- 否则按 **相同 `depends_on` 集合**（忽略顺序、空数组当「无依赖」）分组，组的出现顺序 = 骨架数组里该依赖集合第一次出现的位置。同组画 `（审查 ∥ 测试）`。  
  例：`develop []` → `review [develop]`、`test [develop]` → `release [review,test]` 得到 `开发 →（审查 ∥ 测试）→ 发布`。  
  两个都无依赖的根会并成一组，这是「无边」骨架的如实预览，不是 bug。
- 更乱的图（交叉依赖、多父不对称）**不要**做完整分层拓扑。组内按骨架原顺序；组间按首次出现。看不清的，tooltip 用 `站名（角色）` 按骨架顺序列全。
- 一期编辑器仍不改并行；列表只预览。非串行模板进编辑页会被拒存——列表不拦截「编辑」，点进去由编辑页说明。

链过长：**可见组 >5** 时行内前 4 组 + `+N`，完整链进该单元格 `title`。禁止为读链而让整页横滚。

### 3.5 最深收口列

- 有 `exits`：最后一项 `label`，空则 `deliverable`。
- 无 `exits`（含 v1、generic）：mute **「generic」**。不要写长句。
- 有骨架但出口与产物对不上：列仍用 `exits` 最后一项（规划认 exits 数组）；链上可以没有蓝点。不要为了对齐蓝点去改列口径。

**列表不单独开席位列。** 站名默认就是角色 title，并排会和骨架重复。编制要填哪些人，进编辑页看。`spec.roles` 仍用于站名回退。

## 4. 页面结构

Tier：实体目录（`page-archetypes.md`）。`Main width="wide"`。**柔和白卡外壳 + 内部脆表**（`WorkSurface` + `DataTable`）。不要玻璃。

```
[ShellPageHeader 图标 + 标题 + 副文案]
[Main
  右对齐 新建模板 → /scenario-templates/new     ← 唯一实心主 CTA
  ListToolbar：ToolbarSearch 名称 + Chip 全部 n | 启用中 n | 已禁用 n
  WorkSurface
    加载：TableSkeleton（不要整页空白）
    失败：ErrorState
    全集空：EmptyState + 指向新建
    筛选空：EmptyNoMatch（例如没有已禁用的）+ 动作「查看全部」
    有行：表
]
```

- 副文案：「声明席位与收口档位。新规划只匹配启用中的模板。」
- Chip 用现成 `Chip` / `ListToolbar`，`aria-pressed`。计数永远相对 **未过滤** 列表。过滤后表格变空 ≠ 租户没有模板。
- 密度：不为本页单做舒适/紧凑开关。
- 不改角色词表等其它目录的 MetricCard。

## 5. 列、交互、响应式

**模板**  
`name` truncate。`description` 两行 clamp，空则省略第二行（不要「—」占行）。等宽 `template_key`。

**骨架 / 最深收口 / 状态 / 更新**  
见上。状态 pill 与按钮文案分开：pill 用 `statusLabel`（启用中 / 已禁用）；按钮仍是 **停用 / 启用**（动作）。

**操作**  
行内：**编辑**（`Link` → `/scenario-templates/$templateKey/edit`）、**停用或启用**。  
**删除** 进 `ActionMenu`（`destructive`），不要三颗同权按钮。确认框不改。菜单与按钮 `click` 都 `stopPropagation`。

**整行进入编辑**  
不展开。点非控件区域 `navigate` 到同一编辑路由。名称也是 `Link`，保证键盘和中键新标签。不要把 `<a>` 包整行 `<tr>`。

**响应式**  
`md` 以下：保留 模板、骨架、状态、操作；最深收口 / 更新可 `hidden md:table-cell`。骨架格允许换行，不靠整表横滚当第一策略。

**停用行**  
名称、描述用 `text-ink-2`。不要整行红底、不要左侧 danger bar（停用不是阻断事故）。

启停/删除继续 `ConfirmDialog`。权限：与现页相同，路由级；本页不新做 `PermissionDenied` 分支，除非现网列表已有。

## 6. 样式

对照 `DESIGN.md` / `data-display.md` / `page-archetypes.md`。不要把原型内联 CSS 抄进 feature。

- Token 与组件：`ShellPageHeader`、`Button`、`Chip`、`ListToolbar`、`StatusPill`、`WorkSurface`、`DataTable`、`ActionMenu`、`ConfirmDialog`、`EmptyNoMatch`。链节点本页小组件，颜色只吃 CSS 变量。
- 一行一个语义色：状态 pill。蓝点是品牌强调，不是第二套状态色。
- hover 行背景即可。不要入场动画、不要页脚图例。
- `generic` 作为用户可见词仅出现在「无骨架 · generic 行为」和最深收口「generic」——这是现网规划回落用语，不要改成「默认模板」之类新词。

## 7. 测试与范围

改 `index.tsx`、`index.test.tsx`；派生函数单测（可放 `spec-composer.test.ts`）。

- 不再 mock / 调用 `listScenarioTemplateVersions`、`getScenarioTemplateRoleView`。
- 无「版本历史」「当前版本」「v2」作为版本号（模板 key 里带 v2 仍可出现）。
- 空骨架 → 无占位站，文案 generic。
- 串行链站名来自 role title；蓝点来自 **任一** produce ↔ exit。
- 并行 fixture（审查/测试同依赖 develop）出现 `∥`，不是假直线。
- 筛选：点「已禁用」只剩 disabled；计数在过滤后仍显示全集的已禁用个数。
- 筛选空 ≠ 新建空态。
- 启停/删除用例保留。
- 不渲染验收判据展开句。

**不改**：OpenAPI、sqlc、versions 表、role-view、编辑页保存语义、列表 SQL 顺序。

**验证**：纯 Console 信息架构。`corepack pnpm --filter @superteam/web test` 覆盖本页与派生函数。不强制 Runtime/Provider。CHANGELOG 记用户可见变化（展开/版本历史去掉、链入表、状态筛选）。

## 8. 分步

1. `listChainFromSpec`（含并行分组、exit 匹配、空骨架）+ 单测。
2. 链节点 UI（蓝点、∥、+N）。
3. 拆掉展开、versions、role-view、MetricCard、验收区。
4. `ListToolbar` 状态筛选 + 两种空态。
5. 操作列：编辑 / 启停 / `ActionMenu` 删除；行进编辑。
6. 更新测试、副文案、`CHANGELOG.md`。
