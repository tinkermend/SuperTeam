# SuperTeam 用户体验架构重设计方案

> 交付方:ArchitectUX(用户体验架构师)
> 基线日期:2026-07-06
> 状态:方案待评审,通过后进入落地

## 一、诊断:12 处体验断点

现状调查覆盖导航结构、路由树、核心业务对象模型、五条主用户旅程。结论:SuperTeam 有完整的业务对象模型(Project 承载协调线程、任务、工件、证据、审批、预算、验收),但 UI 把对象拆成了孤岛。断点归为三类:

### 1.1 项目详情 · 信息孤岛(4 处)

| 断点 | 现状 | 影响 |
| --- | --- | --- |
| 无顶层 Tab | 5 个治理 Tab(证据/工件/预算/验收/归档)埋在折叠"高级项目事实"区 | 治理信息默认不可见,用户以为不存在 |
| 任务行无跳转 | "当前执行"任务行只渲染 `task.title + task.summary` 纯文本 | 无法从任务直达运行/员工/工作流 |
| Member 面板死链 | `MemberPanel` 渲染 `ExternalLink` 图标但未包裹任何 `Link` | 负责人/服务池员工点不进去 |
| 无层级回链 | 只有"返回列表"单一回链,无链接到运行总览,无全局面包屑 | 上下文层级丢失,深页无法回溯 |

### 1.2 人类决策 · 旅程断裂(4 处)

| 断点 | 现状 | 影响 |
| --- | --- | --- |
| `/approvals` 占位 | `UnimplementedPage`,与 `inbox` 审批入口完全脱节 | 导航误导,两入口无关系 |
| 审批 dialog 缺上下文 | `InboxActionDialog` 只显示 `action.label` + 评论框 | 缺发起人/原因/风险/证据链,决策无依据 |
| 查看上下文无锚点 | `resolveInboxHref` 跳项目首页,无 deep-link 锚点 | 用户需自行定位决策事项 |
| 审批后不回流 | 提交后 `invalidateQueries` + 留在 inbox | 上下文单向流失,需手动找回原工作流 |

### 1.3 运行可观测 · 处置分离(4 处)

| 断点 | 现状 | 影响 |
| --- | --- | --- |
| 运行总览零外链 | `/run-overview` 整页无 `<Link>` 或 `useNavigate` | 选中员工/节点无跳转到详情 |
| 异常无处置入口 | 异常只通过"异常优先"过滤器和 StatusLegend 标注 | 用户须手动切 inbox/projects 查找 |
| workflow 与项目平行 | `/workflows/$demandId` 与 `/projects/$projectId` 无互链 | 异常 banner 的 `recommended_action` 无跳转 |
| 员工详情断裂 | 无 run-overview 链接,无"产出工件"区,`QueueRow` 按钮无 onClick | 死按钮,执行情况无法回溯 |

## 二、设计原则

1. **项目即工作中枢**——项目是业务事实聚合容器(AGENTS.md 已定义),详情页升级为完整工作面,而非纵向堆叠 + 折叠。
2. **上下文贯穿**——全局面包屑 + deep-link 锚点 + 动作后回流,杜绝单向流失。
3. **人类决策带上下文**——审批不是孤立动作,dialog 必须携带发起人/原因/风险/证据链。
4. **可观测即处置**——异常标注旁边就是处置入口,不让人在页面间搬运上下文。
5. **空状态即引导**——每个空状态都指向下一步,消灭死胡同。

## 三、信息架构:导航重组

### 现状问题

当前 3 组 21 项:工作区 / 核心导航 / 平台管理。问题:分组语义不清("流程编排"在核心导航,"运行总览"在平台管理);`/tasks`、`/task-launches`、`/settings/account` 路由存在但不在侧边栏;`/approvals` 占位却仍在导航。

### 新分组:按"工作循环"重组

| 分组 | 菜单项 | 设计理由 |
| --- | --- | --- |
| **工作台 · 日常高频** | 收件箱(badge) · 任务中枢(首页) · 运行总览 | 日常工作的三个入口:有何待办、发起什么、执行如何。**运行总览从平台管理提升**——它是日常监控工具,不是治理工具 |
| **协作对象 · 管理实体** | 项目管理 · 数字员工 · 技能管理 · 团队管理 | 业务实体的增删改查 |
| **流程能力 · 配置执行** | 流程编排 · 自动化任务 · 外部能力 · MCP 管理 · 协作集成 | 执行能力的配置层 |
| **治理平台 · 运维审计** | 审批中心 · Runtime 节点 · 权限中心 · 成本管理 · 用户管理 · 审计中心 · 日志管理 | 治理、合规、运维 |

### 导航清理动作

- `/approvals`:实现为"审批中心"(聚合 inbox 中 `item_type: "approval"` 的事项 + 历史决策),或从导航移除,二选一,不留占位。
- `/tasks`:确认是否并入任务中枢,若独立则补回侧边栏。
- `/task-launches/$demandId`:仅为重定向到 `/workflows/$demandId`,考虑移除该路由,直接用 `/workflows/$demandId`。

## 四、项目工作中枢:详情页重构

### 现状

`/projects/$projectId` 是纵向堆叠:Header → RuntimePlacement → PlanGraph → 左主列(当前需求/计划确认/当前执行/最新结果/待负责人处理/当前阻塞/事件流)+ 右侧栏(负责人组/服务池)→ 折叠的"高级项目事实"。治理 Tab 被埋在折叠区。

### 新结构:顶层七 Tab + 全链路面包屑

**顶部面包屑**(全站统一,见第五节):`项目 › {项目名} › {需求} › {任务} › {运行}`

**七 Tab**(默认落地"概览"):

| Tab | 承载内容 | 跳转能力 |
| --- | --- | --- |
| 概览 | 当前需求、计划图、当前执行、最新结果、待负责人处理、当前阻塞、事件流(即现左主列内容) | 当前需求卡 → `/workflows/$demandId`;任务行 → 运行/员工;阻塞项 → 审批 Tab |
| 任务 | 项目下全部 `project_tasks`,支持按状态/员工/风险筛选 | 每行可跳:运行记录、执行员工详情、所属工作流 |
| 工件 | `ProjectArtifactRef` + `ProjectEvidenceRef`(从折叠区提升) | 工件可预览/下载;证据可跳来源任务 |
| 审批 | 项目相关的所有 `decision_requests` + `approval_requests`(聚合,支持在项目内直接审批) | 审批动作带完整上下文(见第六节旅程B);审批后留本 Tab |
| 预算 | `budget_ledger` 流水 + `budget_summary` + 告警 | 异常预算 → 审批 Tab 或 配置 Tab |
| 验收 | `acceptance_record` + 验收结论回写 + 证据归档 | 验收动作 → 工件 Tab 归档 |
| 配置 | 现有 `/projects/$projectId/config` 内容内联:`coordination_policy` / `approval_policy` / `evidence_policy` / 成员管理 / Runtime placement | 成员行可跳员工详情;Runtime 可跳 Runtime 节点 |

**Member 面板修复**:负责人/服务池员工的 `ExternalLink` 图标必须包裹 `Link to="/employees/$employeeId"`。

**右侧栏处理**:负责人组/服务池可作为"概览"Tab 内的边栏保留,或下沉到"配置"Tab。建议保留在概览边栏,但补全跳转。

## 五、全局面包屑规范

在 `shell-page-header.tsx` 引入面包屑组件,所有详情页使用:

| 页面 | 面包屑 |
| --- | --- |
| 项目详情 | `项目 › {项目名}` |
| 项目详情·任务 | `项目 › {项目名} › 任务 › {任务名}` |
| 工作流详情 | `项目 › {项目名} › 需求 › {需求ID}` |
| 运行总览 | `运行总览`(顶层,无父级) |
| 员工详情 | `数字员工 › {员工名}` |
| 技能详情 | `技能管理 › {技能名}` |
| 团队详情 | `团队管理 › {团队名}` |

面包屑每级可点击回上级列表。deepest 级不可点(当前页)。

## 六、核心旅程闭环设计

### 旅程 A:创建并运行项目(已基本通,补全跳转)

现状:新建后直达详情 ✓。断点在详情页内部跳转。

**修复**:
- 概览 Tab"当前需求"卡 → `Link to="/workflows/$demandId"`
- 概览 Tab"当前执行"任务行 → `Link` 到运行记录 + 执行员工
- 概览 Tab 顶部加"在运行总览查看" → `/run-overview?project={projectId}`
- Member 面板补全 `Link`

### 旅程 B:审批闭环(inbox 带上下文 + 回流)

**InboxActionDialog 增强**:展示 `发起人`、`原因摘要`、`风险等级`、`证据链摘要`、`关联项目/任务链接`。数据来源:`InboxItem.source_project_id` / `source_task_id` / `source_approval_request_id`。

**查看上下文 deep-link**:`resolveInboxHref` 改为带锚点跳转——`/projects/$projectId?tab=审批&focus={decisionRequestId}`。项目详情"审批"Tab 识别 `focus` 参数,滚动定位到具体决策事项并高亮。

**审批后回流**:提交成功后,弹出轻量 toast「已处理,返回收件箱 / 回到项目」,默认 3 秒后回流到来源上下文(若有 `source_project_id`),否则留 inbox。

**`/approvals` 决策**:实现为 inbox 审批事项的聚合视图(按项目/状态/风险筛选 + 历史决策),与 inbox 共享数据源;或从导航移除。推荐实现,因为"审批中心"是企业用户的认知锚点。

### 旅程 C:数字员工工作台

**新增"参与项目"区**:员工详情页列出该员工参与的 `project_tasks`(通过 `assigned_digital_employee_id` 反查),每项可跳项目详情。

**新增"产出工件"区**:列出该员工产出(通过 `ProjectArtifactRef` 关联 `project_task_id` → `assigned_digital_employee_id`),可预览。

**Run history 增强**:每条 run 加"在运行总览查看"链接 → `/run-overview?employee={employeeId}`。

**修复死按钮**:
- `QueueRow`"绑定" → `/runtime`(筛选待绑定节点)
- `QueueRow`"审批" → `/inbox?employee={employeeId}`
- `QueueRow`"查看" → `/run-overview?employee={employeeId}`
- `GallerySelectedPanel`"查看审计" → `/audit?actor={employeeId}`

### 旅程 D:运行可观测双向链接

**运行总览补外链**:
- 选中员工 → `Link to="/employees/$employeeId"`
- 选中节点 → `Link to="/runtime?node={nodeId}"`
- 异常项 → "去处理"按钮,根据异常类型路由:阻塞类 → `/inbox?project={projectId}`;运行失败 → 对应 workflow

**异常处置入口**:异常卡片增加 `去处理` 按钮,带 `recommended_action` 文案 + 跳转目标。

**workflow 详情回链**:`/workflows/$demandId` 顶部面包屑显示所属项目(通过 `WorkflowInstanceSummary.project_id`),可回项目详情。异常 `blocking banner` 的 `recommended_action` 改为可点击 `Link`。

### 旅程 E:空状态引导规范

所有空状态从"只描述缺失"改为"描述 + 下一步引导":

| 空状态 | 引导文案 + 动作 |
| --- | --- |
| 项目无需求 | "还没有需求。发起第一个任务来启动协调线程" → `/`(任务中枢) |
| 项目无任务 | "协调线程尚未分派任务。查看计划图或等待协调" → 滚动计划图 |
| 项目无审批 | "暂无待处理决策。新决策会出现在这里" (无需动作) |
| 员工无运行记录 | "该员工尚未执行任务。查看可分派任务" → `/` |
| inbox 空 | "收件箱已清空。新待办会出现在这里" (无需动作) |
| 运行总览无异常 | "所有节点运行正常" (无需动作) |

空状态组件统一用 `V3EmptyState`,通过 `action` prop 传入 `Link` 按钮。

## 七、落地路线图(三期)

### 第一期:补全跳转,消灭死胡同(低风险,高收益)

目标:让现有页面"活"起来,对象间可互达。不改信息架构,只补 `Link`。

- [ ] 项目详情 Member 面板补 `Link to="/employees/$employeeId"`
- [ ] 项目详情"当前需求"卡补 `Link to="/workflows/$demandId"`
- [ ] 项目详情"当前执行"任务行补跳转(运行/员工)
- [ ] 项目详情顶部补"在运行总览查看"
- [ ] 运行总览选中员工/节点补 `Link`
- [ ] 运行总览异常项补"去处理"入口
- [ ] workflow 详情顶部补项目面包屑回链
- [ ] workflow 详情异常 banner 补 `Link`
- [ ] 员工详情补"参与项目"区 + "产出工件"区
- [ ] 员工详情 run history 补"运行总览查看"
- [ ] 修复 `QueueRow` / `GallerySelectedPanel` 死按钮
- [ ] 引入 `shell-page-header` 全局面包屑

**验证**:浏览器逐页检查每个 `Link` 跳转目标正确;`corepack pnpm --filter ./apps/web run test` 全量通过。

### 第二期:项目工作中枢 + 审批闭环(中风险,核心价值)

目标:项目详情重构为七 Tab;inbox 审批带上下文 + 回流。

- [ ] 项目详情引入顶层 `V3Tabs`(概览/任务/工件/审批/预算/验收/配置)
- [ ] 治理 Tab 从折叠区提升到顶层
- [ ] "审批"Tab 聚合项目决策事项,支持项目内直接审批
- [ ] `InboxActionDialog` 增强:发起人/原因/风险/证据链
- [ ] `resolveInboxHref` 改为 deep-link 带锚点 + `?tab=审批&focus=`
- [ ] 项目详情"审批"Tab 识别 `focus` 参数,滚动定位高亮
- [ ] 审批提交后回流 toast(返回收件箱 / 回到项目)
- [ ] 空状态全部补 `action` 引导

**验证**:端到端走通"收件箱审批 → 项目内定位 → 审批 → 回流"完整闭环;`/approvals` 占位处理。

### 第三期:导航重组 + `/approvals` 实现(需评审确认)

目标:导航按工作循环重组;审批中心落地或移除。

- [ ] `sidebar-data.ts` 重组为四组(工作台/协作对象/流程能力/治理平台)
- [ ] 运行总览从平台管理移到工作台
- [ ] `/approvals` 实现为 inbox 审批聚合视图,或从导航移除
- [ ] `/tasks`、`/task-launches` 路由清理
- [ ] 命令中心(Cmd+K)补全孤岛页面可达性

**验证**:全站导航可达性审计;每个一级路由都能从导航或命令中心到达。

## 八、开发者交接清单

### 涉及文件

| 文件 | 改动 |
| --- | --- |
| `apps/web/src/components/layout/data/sidebar-data.ts` | 导航重组(第三期) |
| `apps/web/src/components/layout/shell-page-header.tsx` | 引入面包屑(第一期) |
| `apps/web/src/features/projects/index.tsx` 及详情组件 | 七 Tab 重构(第二期) |
| `apps/web/src/features/inbox/*` | dialog 增强 + deep-link + 回流(第二期) |
| `apps/web/src/features/run-overview/index.tsx` | 补外链 + 异常处置(第一期) |
| `apps/web/src/features/workflows/index.tsx` | 顶部面包屑 + banner 跳转(第一期) |
| `apps/web/src/features/employees/detail.tsx` | 参与项目/工件/死按钮修复(第一期) |

### 设计系统约束

- 所有改动遵循 `DESIGN.md` v3 Soft-Flat 设计语言
- Tab 用 `V3Tabs` + `V3TabList` + `V3Tab`
- 跳转用 TanStack Router `Link` / `navigate`,不用原生 `<a>`
- 空状态用 `V3EmptyState`,带 `action`
- 面包屑为新增组件,沉淀到 `apps/web/src/components/superteam/`

### 验证纪律(遵循 AGENTS.md)

- 每期收尾用 `$superteam-completion-check` skill 做完成前检查
- 真实端到端验证:浏览器走通完整旅程,不停留在 mock/组件测试
- Web 测试用 `corepack pnpm --filter ./apps/web run test`
- 涉及可见页面,用浏览器或端到端工具打开目标路由,检查无横向溢出

---

**下一步**:请评审本方案。确认方向后,建议从第一期(补全跳转)开始——它风险最低、收益最高,且不依赖信息架构重组的评审。第二期(项目工作中枢 + 审批闭环)是核心价值,但需要 Tab 重构,建议单独排期。第三期(导航重组)涉及全站认知变化,需产品确认。
