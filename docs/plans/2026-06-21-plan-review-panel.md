# 计划评审面板（决策态）实施计划 — v1

> 复核状态：计划审查面板未落地

**日期：** 2026-06-21
**范围目标：** 把协调线程为某条具体需求生成、且因 `RequiresHumanReview` 停在下发前的那版
`RouteDecisionPlan`，变成项目里一等的「可视化 + 审批」面板，闭合金路径中"人类塑形计划"的空档。

## 已确认的范围决策

1. **v1 只做可视化 + 批/驳**，复用现成 `resolve` 接口，**后端零改动**。"人工编辑计划后再批"留作 v2（届时需让 `handleRouteReviewDecision` 消费 `signal.Payload`）。
2. **DAG 用结构化分组列表承载**（按阶段/依赖分组 + 依赖标注），不引入图形画布。
3. **共享组件，两处复用**：升级现有 `task-launch-detail` 为共享「计划评审」组件，任务发起详情与项目详情新增的协调 Tab 都用它。**入口在任务发起，归属在项目。**

## 背景：当前已有什么（不要重做）

**后端（全部现成，v1 不动）：**
- 读：`GET /api/v1/projects/{projectId}/route-decisions`、`.../decisions`（待审）、`.../task-graph`（DAG：nodes/edges/employees/runs/stage_summaries）、`.../coordination-jobs`、`GET /api/v1/project-demands/{demandId}/launch-detail`。
- 审批闭环：`POST /api/v1/projects/{projectId}/decisions/{decisionId}/resolve`（body: `decision` / `comment` / `payload`）→ `project.Service.ResolveDecision`（service.go:2470）→ `SignalHumanDecisionSubmitted` → workflow `handleRouteReviewDecision`（workflow.go:226）→ approved 自动 `dispatchProjectTasks` / rejected 终止 job。
- 注意：`handleRouteReviewDecision` 目前**忽略 `signal.Payload`**——这正是 v2 编辑能力的接入点，v1 不触碰。

**前端（部分现成）：**
- `apps/web/src/lib/api/projects.ts` 已有类型：`ProjectTaskGraph`、`ProjectRouteDecision`、`ProjectDecisionRequest`、`ProjectDemandLaunchDetail`。
- `apps/web/src/features/task-launches/components/task-launch-detail.tsx` 已把 route_decisions / project_tasks / decision_requests 渲染为**只读平铺列表**——无 DAG 分组、无审批动作。
- `project-governance-tabs.tsx` 现有 Tab：evidence / artifacts / budget / acceptance / archive（缺 coordination）。

**缺口（v1 要补的）：** ① `resolveDecision` 前端变更方法；② 把只读列表升级为结构化分组 + 审批动作的共享面板；③ 项目详情挂载协调 Tab。

## 实施步骤

### 步骤 1 — 前端 API 客户端：resolveDecision
- 文件：`apps/web/src/lib/api/projects.ts`
- 新增 `resolveDecision(opts, { projectId, decisionId, decision, comment })`，POST 到 `.../decisions/{decisionId}/resolve`，`decision ∈ {"approved","rejected"}`，`payload` 传 `{}`（v1 不带编辑）。
- 复用现有 `getProjectTaskGraph` / `listDecisionRequests`（若无则补只读 GET 封装）。
- 同步补 `projects.test.ts` 对 resolveDecision 的请求体/路径断言。

### 步骤 2 — 共享组件：PlanReviewPanel
- 新建 `apps/web/src/features/projects/components/plan-review-panel.tsx`（放 projects 下，task-launches 引用它，避免反向依赖）。
- **先读 `docs/design`（DESIGN.md）**，沿用 `@/components/superteam`（LiquidCard / SemanticIconTile / StatusBadge / LiquidTabs）与既有 tone 语义，不自创样式。
- 输入：`{ projectId, routeDecisions, taskGraph, decisionRequests, onResolved }`。
- 渲染分区：
  1. **决策摘要**：reason、requires_human_review、budget_estimate、template_key、planner metadata。
  2. **任务计划（结构化分组列表）**：按 `stage_summaries` / `stage_index` 分组，每个任务卡显示 title、指派的数字员工（employees 映射）、risk_level、requires_human_approval、expected_outputs；用 `edges` / `blocked_by` 在卡上标注"依赖：X、Y"，被阻塞/可分发用 StatusBadge 区分。
  3. **审批动作区**：仅当存在 `requires_human_review` 的待审 decision_request 时显示「批准下发 / 驳回」按钮 + 备注输入；调用步骤 1 的 `resolveDecision`，成功后 invalidate 相关 query 并触发 `onResolved`。
- 空态/加载/错误沿用 `project-empty-states` 风格。

### 步骤 3 — 任务发起复用
- 改 `task-launch-detail.tsx`：用 `PlanReviewPanel` 替换现有三段只读列表（保留页头/状态徽标）。
- 确认 `task-launches/index.tsx` 已取 launch-detail；按需补 task-graph 查询。
- 首版确认后深链进入项目协调 Tab（`/projects?projectId=...&tab=coordination` 或对应路由参数）。

### 步骤 4 — 项目详情挂载协调 Tab
- 改 `project-governance-tabs.tsx`：新增 `value="coordination"` 的 `LiquidTabsTrigger` + `TabsContent`，内嵌 `PlanReviewPanel`。
- 数据从项目详情已有的 query 注入（route-decisions / task-graph / decisions）；如父层未取，在 `projects/index.tsx` 或对应 loader 补查询。
- Tab 文案与 `aria-label` 与既有风格一致（如"协调计划"）。

### 步骤 5 — 收件箱深链（轻量）
- 确认 `inbox-item-list` 中路由评审类条目点击后深链到项目协调 Tab（而非自渲染）。若已支持则仅核对，不扩张范围。

### 步骤 6 — 测试
- 组件测试：`plan-review-panel.test.tsx`（分组渲染、依赖标注、审批按钮仅在 requires_human_review 时出现、resolve 调用与回调）。
- 复用/更新 `task-launches/index.test.tsx`、`projects/index.test.tsx` 快照与交互断言。
- 运行：`corepack pnpm --filter ./apps/web run test`（禁用 npx）。

## 真实端到端验证（完成条件，不可省）

按 CLAUDE.md：组件/单测通过 ≠ 链路验证。收尾必须：
1. `scripts/dev-services.sh status` 确认 Temporal / Control Plane / Web / Runtime 实际在跑；变更后 `restart` 对应服务。
2. 走真实链路：任务发起提交一条需求 → 协调线程生成带 `requires_human_review` 的路由决策 → 在面板看到结构化任务计划与依赖 → 点"批准下发" → 经真实 `resolve` API → 观察 workflow `dispatchProjectTasks` 真实下发任务、project_tasks 状态推进。
3. 用浏览器（codex chrome 插件）或 curl 核对最终页面/API 不是 mock/缓存/旧服务结果。
4. 收尾跑项目内 skill `$superteam-completion-check`。
5. 若 DeepSeek planner key 缺失 / 服务未起 / 无法生成需人工评审的决策 → 标记**阻塞**并说明缺失依赖，不得把"未做真实链路验证"当完成交付。

## 明确不在 v1 范围（后续）
- v2：人工编辑计划（改指派 / 调依赖 / 增删任务）后再批 —— 需 `handleRouteReviewDecision` 消费 `Payload` + service 校验 + workflow 应用变更。
- v3：深度规划（无合适员工→升级为人类决策而非硬塞）、迭代重规划。
- 图形化 DAG 画布（v1 用结构化分组列表替代）。

## 关键文件清单
- `apps/web/src/lib/api/projects.ts`（+ resolveDecision）
- `apps/web/src/features/projects/components/plan-review-panel.tsx`（新建，共享）
- `apps/web/src/features/projects/components/project-governance-tabs.tsx`（+ coordination Tab）
- `apps/web/src/features/task-launches/components/task-launch-detail.tsx`（改用共享面板）
- 后端 v1 不改动（仅复用 server.go:251-254 既有路由）。
