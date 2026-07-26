# 流程图单一权威组件 + 图数据增补 Spec（IA 重构 Phase 1）

- 日期：2026-07-26
- 状态：**立项（未实施）**
- 交付性质：Web 组件合并 + CP 读投影增补（含契约变更，无迁移）
- 目标读者：实施会话
- 上下文：本 spec 是「按视角轴重划 Console 信息架构」四阶段中的第一阶段。四阶段全景见 §8；本阶段**不动任何菜单与路由**，纯合并 + 增补，独立可验收。

---

## 1. 背景：2026-07-26 真实走查结论

用真实浏览器走查 `/workflows` 列表页与两个实例详情页（等待人工实例 `abab0c3d`、失败实例 `e7274b71`），并对照代码，坐实两类结构问题：

**A. 一份数据、三个消费方、两套画布。** `getProjectTaskGraph` 被三处消费：

| 消费方 | 画布/投影 | 文件 |
|---|---|---|
| 流程编排详情 | `WorkflowGraphCanvas`（可点节点 → 抽屉，含 blocking/attachment 节点、MiniMap/Controls） | `features/workflows/components/workflow-graph-canvas.tsx` |
| 项目详情 | `PlanGraphCanvas`（**不可点**、无抽屉、无 blocking/attachment）+ `PlanTaskGraph` 阶段列表 | `features/projects/components/plan-graph-canvas.tsx`、`plan-task-graph.tsx` |
| 运行总览 | 项目透镜（自有投影，物理轴，不在本 spec 范围） | `features/run-overview/` |

勘察修正：`PlanGraphCanvas` 已复用 workflows 的 `WorkflowTaskNode`/`WorkflowStageLabelNode` 和 adapter，真正的分叉是：①两个 build 函数（`buildPlanTaskGraphElements` vs `buildWorkflowGraphElements`，`workflow-graph-adapter.ts`）；②项目侧无节点点击交互；③项目侧缺 blocking/attachment 节点渲染。合并成本低于最初估计。

**B. 图上"看不出为什么、看不到发生了什么"。** 实测失败实例的节点抽屉（`workflow-node-inspector.tsx`）：

- 失败 run 无错误信息、无起止时间、无耗时——数据层 `ProjectTaskGraphRun`（`task_graph_types.go:62`）只投影 status/provider_type/node 摘要，而 `task_runs` 表**本来就有** `started_at`/`completed_at`/`finished_at`/`error_message` 列（migration 001），是投影没带出来，非数据缺失。
- "Runtime" 按钮链到泛化 `/runtime` 页，不指向本次运行。
- 阻塞行自指："扫描项目目录并统计文件 · project_task · fe2679cb…"，resource_id 即任务自身 ID，显示为"任务被自己阻塞"。
- "人工决策：cancel_downstream" 英文原值直出（`status-labels.ts` 词表缺键；且该值疑似 resolution 语义混入 status_snapshot，实施时先查证再定键）。
- "输入：required_inputs: []"、"交接契约：acceptance_criteria: […]"——`formatValue` 把内部 schema 键名直出给中文用户。
- 列表行"提交 36e24cb9-…"裸 UUID：`project.sql:310` 的 `COALESCE(source_refs->>'submitted_by_display_name', submitted_by_user_id::text)`，Web 提交的需求不带该键必然回退 UUID，违反 CLAUDE.md「不得裸 UUID，名称由服务端读路径批量补名」。
- 验收血缘展开显示任务裸 UUID（`criteria-panel.tsx`）。

## 2. 目标

1. 流程图收敛为**单一权威组件**：一个画布、一个 adapter build 函数、一套节点状态色/词算法；项目详情与流程编排详情消费同一组件，行为一致（都可点开节点抽屉）。
2. 节点抽屉能回答「为什么失败、什么时候跑的」：错误摘要 + 起止时间/耗时。
3. 清偿走查坐实的显示债（裸 UUID、词表缺键、schema 键直出、阻塞自指）。

## 3. 非目标（后续阶段，勿在本轮做）

- 菜单/路由变更、河道列表迁移、`/workflows` 去留（Phase 2）。
- `ListWorkflowInstances` 的 `q`/`project_id` 过滤参数接线到前端（SQL 已支持，Phase 2 随河道迁移一起做）。
- 数据流活图 / SSE 驱动边流动（Phase 3，先出原型再立项）。
- 验收血缘面板归位验收域（Phase 2）。
- 运行总览项目透镜改造（不同视角轴，保留自有投影）。
- priority/risk/SLA 幽灵字段的录入机制（另行拍板：补录入口或摘除 KPI）。

## 4. 设计

### 4.1 CP：图读投影增补（无迁移，契约变更）

`ProjectTaskGraphRun` 增补字段（`task_graph_types.go:62` → `pg_repository.go:3184 projectTaskGraphRunsFromRows` → `handler.go:3405 taskGraphRunResponses` → `contracts/control-plane/openapi.yaml:7941 ProjectTaskGraphRun`）：

- `started_at`、`finished_at`（nullable timestamptz；`finished_at` 取 `COALESCE(finished_at, completed_at)`）
- `error_message`（nullable string；**脱敏走既有 prose 脱敏出口**，与 execution ledger 同规则，不得把含密钥的原始 stderr 直出）

契约变更走 `generate:control-plane` + 契约验证（`verify:foundation-contracts` 覆盖 control-plane openapi）。

`ListWorkflowInstances` 提交人补名：`project.sql:310` 改为 join `users` 取 display_name，`source_refs->>'submitted_by_display_name'`（飞书等外部通道写入）优先、users 表次之、UUID 仅作最终兜底。sqlc 再生成。

### 4.2 Web：画布合并

- 新建中立目录 `apps/web/src/features/flow-graph/`（命名可在实施时定，不叫 workflows 也不叫 projects，为 Phase 2 搬迁解耦），迁入：画布、`workflow-task-node`、`workflow-blocking-node`、node inspector、adapter。workflows 与 projects 只 import。
- adapter 两个 build 函数合并为一个，blocking/attachment 渲染由参数开关（项目详情也应看到 pending 决策 attachment——信息增益，非行为回退）。
- `PlanGraphCanvas` 删除，项目详情（`project-operational-detail.tsx:505`）改用权威画布并接上节点抽屉（`selectedNodeId` 状态 + Dialog，参照 `workflow-detail.tsx:154-179`）。`PlanTaskGraph` 阶段列表保留（它是列表形态，不与画布冲突；TODO 07-22 的横向管道可视化届时基于权威组件做，属 Phase 2）。
- 节点状态色/词：`taskStatusTone`（`workflow-node-inspector.tsx:114`）迁入 flow-graph 并成为唯一出口；词一律经 `status-labels.ts`。这是 `2026-07-19-stuck-task-reconciliation-design.md` §3.3「消灭多套算法」在 task-graph 前端的延伸。

### 4.3 Web：节点抽屉增强与显示债

- 运行区：显示 起止时间 + 耗时 + 失败时 `error_message`（折叠长文本）；"Runtime" 深链改为「查看执行轨迹」→ 项目详情执行轨迹面板（`project-execution-trace-panel.tsx`）按任务过滤定位，泛化 `/runtime` 链接删除。
- 阻塞行：blocker `resource_id == task.id` 时不再渲染自指（该 fact 语义是"实例当前停在本节点"，节点抽屉里显示为状态而非阻塞）；`type` 原值（`project_task`）不直出，走词表。
- `formatValue`：`required_inputs`/`acceptance_criteria` 等已知键渲染为中文标签 + 条目列表；未知键兜底保持现状。
- 词表补键：`cancel_downstream` 等（先查证 status_snapshot 里混入 resolution 值的根因：若是写入侧混用，修写入侧并补历史兼容显示；若是合法状态，补词表即可）。
- 验收血缘（`criteria-panel.tsx`）：任务指称改「任务名 (id)」，名取自同页已有的 task graph nodes，无命中再退 id。

## 5. 分期与并发风险

- P1a：CP 投影 + 契约 + 补名（可独立合入验收）。
- P1b：画布合并 + 抽屉增强（依赖 P1a 字段）。
- P1c：显示债清偿（可与 P1b 同批）。
- **并发风险**：项目详情属高频并发改动区（2026-07-26 走查时另一会话正在编辑 `project-ops-home.tsx`）。P1b 动 `project-operational-detail.tsx` 前先核对共享 checkout 未提交改动，交织时按 CLAUDE.md 共享工作树规则只暂存自己的 hunk 或走独立 worktree。

## 6. 验收判据（真实端到端）

- **G1 失败可诊断**：真实失败实例（现存 `e7274b71` 或新造）在流程编排详情点开失败节点，抽屉显示错误摘要与起止时间；DB 中 `task_runs.error_message` 与页面显示一致（脱敏后）。
- **G2 同源一致**：同一需求在项目详情与流程编排详情点开同一节点，抽屉内容一致（同一组件实例化）；项目详情节点可点击（新能力）。
- **G3 补名**：浏览器流程编排列表"提交"处显示中文显示名；直连 API 确认 `submitted_by_display_name` 非 UUID；飞书来源需求（带 source_refs 名）仍优先显示原值。
- **G4 显示债回归**：失败节点抽屉无自指阻塞、无 `cancel_downstream` 英文原值、输入/交接契约无 schema 键名直出；血缘面板无裸 UUID。`status-labels.guard.test.ts` 护栏通过。
- 门禁：`verify:control-plane`、`verify:web`、契约验证全绿；按 CLAUDE.md，`verify:*` 通过不等于完成，G1–G4 必须走真实浏览器 + curl + DB 三路。

## 7. 关联

- `2026-07-24-human-task-unification.md` — Phase 0（已落地部分见其状态头），行动入口唯一化。
- `2026-07-19-stuck-task-reconciliation-design.md` §3.3 — 跨视图一致性统一出口，本 spec 是其 task-graph 前端延伸。
- TODO 07-22 项目详情横向管道可视化 — Phase 2 与本组件合并做，勿另起画布。

## 8. 附：IA 重构四阶段全景（2026-07-26 与人类对齐）

病根：五个页面（收件箱/项目管理/流程编排/运行总览/数字员工）按页面分工而非按视角轴分工，同一份 task graph 摊派给三个页面。目标终态按视角轴：收件箱=人轴（唯一行动入口）；项目=业务轴（权威流程图落点）；运行总览=物理/实时轴；数字员工=配置轴；"需求实例"不再是独立轴。

- **Phase 0**：人类任务统一（spec 已有，部分已落地）。
- **Phase 1**：本 spec。
- **Phase 2**：权威图落进项目详情需求维度（合并 TODO 07-22 横向管道）；河道列表迁任务中枢并接通 `q`/`project_id` 过滤；`/workflows/:demandId` 留重定向壳（飞书深链兼容），菜单撤"流程编排"；血缘面板归位验收域；KPI 口径收敛（含"已归档"页签更名、恒零"已完成"卡处置）。
- **Phase 3**：数据流活图（SSE 驱动边流动 + 交接契约 vs 实际交付物对照）。先走 design-prototypes 出原型给人类拍板，值得再立项；成则可重新赢回独立入口。
