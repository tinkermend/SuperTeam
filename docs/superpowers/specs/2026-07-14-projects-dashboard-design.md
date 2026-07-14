# 项目管理首页升级：运营驾驶舱（多维度铺满）

- 日期：2026-07-14
- 状态：已批准（方向经用户确认；维度取舍按克制默认：决策流 + 运行动态进首页，Runtime 健康/成本留在运行总览，记为可选 P2）
- 前置：布局宪法 P1/P1.1（specs/2026-07-14-layout-constitution-design.md）已落地
- 背景：布局系统治理后大屏仍显空旷，根因是信息架构单薄——首页只有 1.5 个维度（项目队列 + 其从属 triage 栏）。用户明确要求"不管屏幕大小铺满整个屏幕"。设计原则：**铺满靠增加信息维度，不靠拉伸现有内容**（每个面板密度有界）。侧栏收件箱 49 条未读（跨项目审批/人工决策）首页不可见、`listWorkflowInstances` 已请求 50 条却只用于"最近活动"时间戳，均为已有数据未上首页的浪费。

## 目标形态

列表态（`/projects` 无 routeProjectId）升级为驾驶舱；项目详情路由不变。

```
窄容器（现状体验近似）            宽容器（驾驶舱两列）
┌────────────────┐            ┌──────────────────────┬──────────────┐
│ KPI 带          │            │ KPI 带（新增「待我决策」真值卡，横贯全宽）  │
├────────────────┤            ├──────────────────────┼──────────────┤
│ 项目队列        │            │ 项目队列（1fr）        │ 右栏（clamp） │
├────────────────┤            │                      │ ①待我决策 流  │
│ 待我决策        │            │                      │ ②最近运行动态 │
├────────────────┤            └──────────────────────┴──────────────┘
│ 最近运行动态     │            选中项目 → 右栏整体切换为 triage 面板（带关闭钮，
└────────────────┘            关闭返回驾驶舱右栏）；窄容器选中仍走 Sheet。
```

## 决策记录

1. **宽度档位**：列表态从 `wide`(1680) 改为 **canvas（铺满）**。布局宪法 canvas 档适用范围由"仅图形/拓扑画布"扩展为"图形/拓扑画布 + 多面板驾驶舱"；驾驶舱铺满的前提是面板数量/宽度随容器扩展，单面板密度仍有界。layout-density.md 同步。
2. **右栏是一个可切换槽位**：默认装驾驶舱面板（决策流 + 运行动态堆叠），选中项目时切换为 triage 面板。复用 `MasterDetailLayout`，新增 `narrowDetail?: 'sheet' | 'stack'`（默认 sheet）：驾驶舱面板在窄容器堆到主列下方（stack），triage 在窄容器走 Sheet（现状）。页面据 `selected` 切换 detail 内容与 narrowDetail 值。窄容器 Sheet 打开期间驾驶舱面板暂不在流内（被遮罩覆盖，关闭即回），可接受。
3. **右栏宽度**：驾驶舱右栏用 rail="lg"（420 上限，minmax 可压缩）；超宽屏队列 1fr 自然变宽，后续可给队列加列（预算/验收进度）消化宽度，记为待办不在本期。
4. **triage 面板加关闭钮**：右栏现在有默认内容，关闭选中需要显式返回入口（此前空态无此需要）。

## 新面板（全部现有 API，零后端改动）

### ① 待我决策（InboxDecisionsPanel）
- 数据：`listInboxItems({ view:'mine', status:'open', limit:8 })` + summary 计数；轮询/焦点刷新与收件箱页一致口径（React Query 默认即可）。
- 行：IconTile（approval/project_decision 分型）、title（两行 clamp）、项目名（source_project_id → 已加载项目映射，缺省显示 source_type）、相对时间（`last_activity_at`，tabular-nums）、risk_level 高危 tone。整行可点：有 source_project_id 的 project_decision 深链 `/projects/$id?tab=approval&focus=source_id`，其余落 `/inbox`。
- 头部：「待我决策 · N」（N=summary.open_count）；底部「查看收件箱 →」链接。空态：安静文案。
- 表面：不透明 SoftCard/WorkSurface（逐行扫读，禁玻璃）。

### ② 最近运行动态（RunActivityPanel）
- 数据：复用页面已有的 `workflowInstancesQuery`（不新增请求），按 `updated_at` 倒序取 8 条。
- 行：项目名（instance.project_id → 项目映射）、workflow 状态 pill、相对时间；整行链到对应项目。
- 空态：暂无运行记录。

### KPI 带
- 新增第 5 张卡「待我决策」（`getInboxBadge().mine_open_count`，>0 时 danger/warn tone，0 灰），其余 4 卡不变。MetricGrid 已支持 ≥3 卡两端对齐。

## 工作对象规则遵从

- 每行必有主时间字段（相对时间 + tabular-nums），新近优先排序。
- 语义色只用于状态 pill/圆点/accent；一行最多 1 个语义色状态编码。
- 长标题两行 clamp + title 提示；不横向滚动。

## 测试与验证

- index.test.tsx：mock fetcher 增加 `/api/v1/inbox/items`、`/api/v1/inbox/badge`；新增断言——宽容器默认右栏为决策流+运行动态、选中切换为 triage、关闭返回；窄容器面板堆叠 + 选中走 Sheet（复用现有）；决策行深链 href；多视口无溢出门禁沿用。
- MasterDetailLayout `narrowDetail='stack'` 单测。
- 真实浏览器 E2E：宽（2000+）验证驾驶舱两列铺满、决策流数据真实（49 条未读来源）、选中/关闭切换；窄验证堆叠。

## 不做（本期）

- Runtime 健康/成本面板（留运行总览；P2 可选）。
- 队列加列消化超宽（待办）。
- 收件箱行内直接执行 action（首页只导流，不承载审批操作，避免绕过收件箱/审批中心的完整上下文）。
