# 场景模板首页 · 链入表落地方案

状态：待实现。对照原型：`c-inline-rail-table.html`（已按真实接口校正）。编辑页方案见 `docs/prototypes/scenario-template-composer/redesign-spec.md`，本文只改 **列表页** `/scenario-templates`。

## 0. 结论

首页改成 **软壳装脆数据的链入表**：一行一个模板，骨架画在行内，**去掉展开层和版本历史**。数据只吃 `GET /api/v1/scenario-templates` 返回的主表 + `spec`，**不**再为展开去打 versions / role-view。

作者心智与编辑页一致：点「编辑」改当前配置；「保存更改」后台仍写不可变版本行，只给进行中规划钉 spec，**产品面不出现版本号**。

## 1. 和真实接口的对照（原型里哪些能落地）

列表项来自 `listScenarioTemplates`（`ScenarioTemplate`）：`name`、`description`、`template_key`、`status`、`updated_at`、`spec`。`spec` 内已有 `roles[]`、`skeleton[]`、`exits[]`。

| 原型列 | 落地口径 | 接口 |
|---|---|---|
| 模板名称 / 描述 | 原样 | `name` / `description` |
| key | 等宽补充，不作主指称 | `template_key` |
| 骨架链 | **派生**，不是 spec 上的独立字段 | `spec.skeleton` + `spec.exits` + `spec.roles` |
| 席位 | 名称，不是 key | `spec.roles[].title`，缺则 `key` |
| 最深收口 | `exits` 数组**最后一项**的 `label`（浅→深，与现展开区文案一致） | `spec.exits` |
| 状态 | `StatusPill` + 词表 | `status`：`active` / `disabled` → `statusLabel`（启用中 / 已禁用） |
| 更新 | 相对时间 | `updated_at` |
| 筛选全部/启用/停用 | **前端滤** | 列表 API 无 query；不要新接口 |
| 启停 / 删除 | 现有确认框文案保留 | `PATCH` status；`DELETE` 软删 |

### 1.1 骨架链怎么从 spec 算（禁止再发明 `step.exit`）

骨架站 **没有** `exit` 布尔字段。是否可收口与编辑器同一规则（`spec-composer.ts` 的 `composerFromSpec`）：

- 站的产物名 = `skeleton[].produces_defaults[0].name`，没有则 `${step}_outcome`
- 若 `exits[].deliverable` 等于该产物名，该站可收口
- 站上展示名 = `skeleton[].title`（若有）→ 否则该站 `role` 在 `spec.roles` 里的 `title` → 否则 `step` 键。**禁止**在列表里手写「接入 / 定界」这类种子故事名。

可收口用 **品牌色小圆点**（与编辑页「蓝点可收口」同一语言），不要在每个节点上写「· 可收口」长文案。

串行：`depends_on` 恰好是上一站 → 画 `A → B → C`。判定复用已有 `isSerialSkeleton(spec)`。

并行（种子 `software_delivery`：审查 ∥ 测试都依赖 develop）：**禁止**拉成假的一条直线。按 `depends_on` 把同依赖的站画成 `开发 →（审查 ∥ 测试）→ 发布`。一期编辑器仍不改并行图；列表只如实预览。

链过长（建议 **>5 个可见节点**，并行组算一组）：行内显示前 4 组 + `+N`，完整链进 `title` / tooltip。禁止为读完一条链而横向滚整表。

无骨架（generic 或空 `skeleton`）：行内灰字 **「无骨架 · generic 行为」**，与停用确认框里的 generic 回落语义一致。不要编造占位站。

### 1.2 明确不做（原型曾暗示、接口也有、但首页不该用）

| 能力 | 为什么列表不用 |
|---|---|
| `GET .../versions`、`active_version` | 产品面无「升版」；钉 spec 是规划内部的事 |
| `GET .../role-view` | 出口所需角色、独立性、租户持有者人数是编制/规划语义；编辑页方案已定 **模板页不掺项目就绪度**。列表再 N+1 打 role-view 无必要 |
| `playbook-readiness` | 绑定项目；列表无项目上下文 |
| 展开区验收判据 | `default_acceptance_criteria` 在 spec 里，给编辑/规划用；首页扫的是链和收口 |
| 列表筛选 API、服务端排序 | 现网租户模板量小；先客户端滤 `status`、按 `updated_at` 倒序（工作对象默认新近优先） |

`listScenarioTemplateVersions` 可留在 API 客户端供以后审计用，**本页不再调用**。

## 2. 页面结构（Soft-Flat）

Tier：实体目录。容器：**柔和白卡外壳 + 内部脆数据表**（`WorkSurface` + `DataTable`）。不要玻璃卡、不要两张空洞 MetricCard。

```
[ShellPageHeader 场景模板]
[右上 新建模板 → /scenario-templates/new]

[工具条：全部 n | 启用中 n | 已禁用 n]   ← chip，aria-pressed
[WorkSurface
  表头 sticky
  行：模板 | 骨架 | 席位 | 最深收口 | 状态 | 更新 | 操作
]
```

- Header 副文案改为能说清职责，例如：「声明这类活要经过哪些席位、收到哪一档；新规划只匹配启用中的模板。」
- 计数写在 chip 上，不再单独占两张指标卡。
- 空 / 加载 / 失败仍用现有 `EmptyState` / `LoadingState` / `ErrorState`，落在 WorkSurface 内。
- 密度：跟随全站表格「舒适 / 紧凑」若壳层已有则接上；本页不单独做一套。

## 3. 列与交互

**模板**  
主名 `name`（truncate）。下一行 `description` 两行 clamp。再下一行等宽 `template_key`（`text-ink-3`）。对象指称用名称。

**骨架**  
见 §1.1。节点：`card-soft` 底、小圆角、12px；箭头 `ink-3`。可收口站左边品牌点。并行组外一层弱线框。

**席位**  
`roles[].title` 用 ` · ` 连接；空则「无约束」。过长 truncate + title 列出全称。不展示 `required_capabilities`、不展示持有人数。

**最深收口**  
有 `exits`：最后一项 `label`，空 label 则 `deliverable`（用户可见优先 label）。无 exits：mute 文案「generic」。不要写「规划走 generic 行为」这种长句进单元格。

**状态**  
`StatusPill`：`active` → tone `ok`；`disabled` → `mute`。文案走 `statusLabel`。

**更新**  
`formatRelativeTime(updated_at)`，`tabular-nums`。

**操作**  
行内保留 **编辑**（`Link` → `/scenario-templates/$templateKey/edit`）、**停用/启用**。删除进「更多」或保持现按钮但视觉降为 ghost；确认框文案不改。`stopPropagation` 避免误触。

点击行：不展开。若点击落在非按钮区，与「编辑」同一路由（整行可进对象）。键盘：操作按钮可 tab；行 click 不是唯一入口。

启停/删除继续用现 `ConfirmDialog`。停用后新规划回落 generic、已实例化项目不受影响——这句话留在确认框，不要写进每一行。

## 4. 样式约束（落地时比 HTML 原型更产品化）

对照 `DESIGN.md` / `docs/design-system/data-display.md`，不要把原型里的内联 CSS 抄进 feature：

- Token：`--brand #2F5FFF`、冷灰底、白卡 `--r-card` ~22px、内层 ~14px、按钮 ~12px、弥散阴影只在外壳。
- 组件：`ShellPageHeader`、`Main width="wide"`、`Button`、`StatusPill`、`WorkSurface`、`DataTable`/`Th`/`Td`/`Tr`、`ConfirmDialog`。链节点用本页小组件，颜色只吃 CSS 变量，不写死第二套蓝。
- 一行最多一个语义色编码：状态 pill；链上的蓝点是品牌强调，不算第二套状态色。
- 停用行：降低名称对比（`text-ink-2`），不要整行大红底。
- 动效：hover 行背景即可；不要卡片上浮、不要入场动画。
- 中文：状态进 `status-labels.ts`；骨架站名来自 spec/词表映射，不在 JSX 里写死种子剧情。

## 5. 测试与范围

改 `apps/web/src/features/scenario-templates/index.tsx` 与 `index.test.tsx`：

- 去掉展开、versions mock、验收判据展开用例。
- 断言：串行链可见站名/角色名；可收口点来自 exit↔produce 匹配；并行行出现 `∥` 或等价结构，而不是假直线。
- 断言：页面 **没有**「版本历史」「当前版本」「v2」。
- 启停/删除用例保留。
- 筛选 chip 按 status 过滤。

**不改**：OpenAPI、sqlc、versions 表、role-view、编辑页 composer、Control Plane 保存语义。

**轻量验证**：纯 Console 列表信息架构；`corepack pnpm --filter @superteam/web test` 覆盖本页即可，不强制 Runtime/Provider。

## 6. 分步

1. 抽列表用的只读派生（或直接复用 `composerFromSpec` + `isSerialSkeleton`，避免第二套 exit 匹配）。
2. 链节点 UI + 并行压缩。
3. 拆掉 `ScenarioTemplateRow` 展开与 versions/role-view query。
4. 工具条筛选替换 MetricCard。
5. 更新测试与本页副文案。
