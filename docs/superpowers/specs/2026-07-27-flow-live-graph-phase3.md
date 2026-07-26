# 数据流活图 Phase 3 P1：ReactFlow 权威组件上的概念 A 实现

- 日期：2026-07-27
- 状态：**立项（实施中）**
- 拍板：人类选定概念 A（粒子流动，`docs/prototypes/flow-live-graph/concept-a-flowing-edges.html`），并指出原型缺时间单位——时间显示为本期一等需求。
- 前置：IA Phase 1/2 已完结（flow-graph 单一权威组件 + run 时间/错误投影 + 需求流程区落项目详情）。
- 交付性质：纯 Web（`features/flow-graph/` 扩展 + 需求流程区接线），无契约变更、无迁移、无新端点。

## 1. 范围（P1）与非目标

P1 做四件事，全部落在现有 ReactFlow 权威组件上：

1. **活性边（概念 A 粒子流）**：自定义 edge 组件，血缘上游任务 `running` 时该出边渲染粒子沿边流动（SVG + rAF 或 CSS motion-path，跟 xyflow 的 edge path 几何走）；`failed` 边红色停流；终态完成边呈静态已通电样式。粒子层必须尊重 `prefers-reduced-motion`（降级为概念 B 式呼吸边）。
2. **时间单位（人类指出的缺口）**：节点卡显示运行起止/耗时（Phase 1 已投影 `runs[].started_at/finished_at`，格式化用既有 `format-time`/`formatRunDuration`）；运行中节点显示"已运行 X 分"滚动时长；交接对照浮层显示交付时间。
3. **交接对照浮层**：点击边弹出浮层——左=交接契约（blocker 节点 `expected_outputs`/`handoff_contract.acceptance_criteria`），右=实际执行结论与产出（`execution_summaries` 的 conclusion + 已加载的交付物数据，勘察 launch-detail/task graph 现有字段为准）。**verdict 责任方拍板默认**：v1 前端只做"有/无"层面的浅呈现（已交付/暂无产出），**不得**在数据不支持时编造"不符"判定——升级为 CP 结构化 verdict 属后续（见 §3）。
4. **数据活性**：需求流程区的 graph query 补 `refetchInterval: 5000`（与平台既有轮询口径一致）；动画状态纯由 graph 数据推导（无独立动画状态机）。SSE 升级属后续。

**失败边语义拍板默认**：边的视觉状态严格从权威任务/运行状态推导（blocker failed → 边红停流；下游 cancelled → 边灰），不引入"交接不符"等新边语义——避免与状态机/对抗评审/返工循环打架。原型里的"回放执行"**不在 P1**（需要事件时序数据设计，独立拍板）。

## 2. 落点

- `features/flow-graph/`：新自定义 edge（如 `flow-live-edge.tsx`）+ adapter 给 edge 附 activity 状态 + 节点卡时间区扩展 + 交接浮层组件。开关式接入：`FlowGraphCanvas` 加 `live?: boolean` prop，项目需求流程区开启；其他消费方（任务弹层内嵌等）默认不变。
- `features/projects/components/project-demands-section.tsx`：开 live + refetchInterval。
- 视觉贴 DESIGN.md；动画色用状态色 token，不新增 token。

## 3. 后续（非 P1，触发时另拍）

- 交接 verdict 升级：CP 读路径给结构化"符合/不符"（可复用到验收链路，需新端点）。
- SSE 驱动替代轮询；回放执行时序；大屏模式（可与概念 B 分层："默认 A、超大图降 B"）。

## 5. P2 分期（2026-07-27 人类指示"进行 §3 后续项"后立项）

两批推进，批内文件面互斥：

### 批 1

**P2-V 结构化交接 verdict（CP 读路径 + 契约 + 浮层消费）**
- 计算落点：task-graph 读路径（`GetProjectTaskGraph` 组装层），**不新增端点、不持久化**（v1 纯读投影；持久化与验收链路复用属后续）。响应新增 `handoff_assessments[]`（按 `project_task_id`）：对每个已声明交付物（`TaskResultContract.Deliverables`，v2 声明管道含 Ref 回填）给出 `delivered | missing`（Ref 已回填=delivered），汇总 `status: fulfilled | partial | unfulfilled | unknown`——**无声明数据时必须 unknown，禁止启发式猜测**（诚实边界与 P1 浮层一致）。
- **本轮硬约束**：并发会话在 `storage/queries/`（含共享生成物 models.go/querier.go）有未提交改动，**禁止新增 sqlc 查询/重新生成 sqlc**；verdict 必须从读路径已加载数据（execution summaries / result contract / attempt payload）计算，取不到的维度如实降级 unknown 并在报告注明数据缺口。
- 契约：`ProjectTaskGraph` schema 增补 + `generate:control-plane`（openapi 生成物与 sqlc 无关，允许）。
- 前端：`flow-handoff-overlay.tsx` 右列按交付物逐条显示 verdict 徽章（已交付/缺失，unknown 维持"暂无"文案）；底注在有结构化 verdict 时改为"符合性由控制平面按声明交付物核对"。

**P2-S 大图性能分层（A/B 自动降级，纯 Web）**
- adapter/canvas 阈值：节点+边总数超阈值（默认 40，常量可调）时活性边自动降级为概念 B 呼吸边（复用 reduced-motion 降级路径，`data-live-degraded="scale"` 可测）；阈值内维持粒子。与 reduced-motion 取或。

### 批 2（批 1 落地后）

**P2-E SSE 替代轮询**：勘察既有 SSE 面（inbox / run-overview activity）能否复用项目维度事件；能则 demands 区图查询改事件驱动 invalidate（保底轮询降频为 30s），不能则记录缺口另立 CP 项，不自造通道。
**P2-R 回放执行时序**：基于 task graph 节点 started_at/finished_at + recent_events 的确定性时间轴（P1 原型的回放按钮语义），纯前端时间缩放播放；事件数据不足以支撑完整时序时先出降级版（按起止时间插值点亮），报告数据缺口。

验收（真实端到端）：V1=真实带声明交付物的任务浮层显示逐条 verdict 与汇总；V2=无声明数据任务维持"暂无"；S1=直插造 41+ 节点图（或组件测试）验证自动降级；E/R 判据批 2 立项时补。

## 4. 验收（真实端到端）

- L1：真实项目需求流程区，存在 running 任务时对应边粒子流动、failed 边红停流；`prefers-reduced-motion` 下无粒子。
- L2：节点卡显示真实起止/耗时；运行中节点时长随刷新推进。
- L3：点边浮层显示契约与实际产出对照，时间齐备；无产出时呈现"暂无"而非伪 verdict。
- L4：5s 轮询下状态变化（可直插 DB 改任务状态）驱动边动画切换。
- `verify:web` 全绿。
