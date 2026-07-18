# 运行总览维度裁决 Spec：团队底座 + 项目覆盖层

> 日期：2026-07-18
> 状态：**已落地入 main（2026-07-18 14:31，GATE 六项 PASS）**，实施记录见文末 §7
> 性质：对运行总览（`/run-overview`）的维度定位裁决 + 两期落地计划。不重构页面分组逻辑，底座保持团队维度，项目以覆盖层（透镜）形式叠加。
> 关联：运行总览实时数据化（07-16, main 61648981）；项目运行详情 `project-operational-detail`（07-14 projects-dashboard spec）。

---

## 0. 动因：三个问题与一个建模错位

当前运行总览是团队维度——"楼层 → 团队工位 → 座位"的空间隐喻，员工按 `team_id` 落座。暴露三个问题：

1. **未归属团队的员工在地图上不可见。** 代码可确认：`runtime-overview-adapter.ts` 里 `teamId` 落到 `"unassigned"` 后，`workspaceByTeamId.get("unassigned")` 拿不到工位 → `seat` 为 undefined → 地图不渲染；但汇总数字（`employeeCount` 等）计入了它，画面与数字不一致。
2. **看不到项目内的任务串联。** 任务围绕项目组织（A→B→C 交接），团队维度只能看到"哪些人在忙"，串不出"这个项目的任务走到哪一环、堵在谁手里"。activity 流虽已带 `project_name`/`task_title`，但只是文字流，与空间画面无联动。
3. **若改成项目维度，会出现重复头像。** 员工只属一个团队，但可属多个项目——项目当"分组容器"用，容器就破了。

**裁决依据**：重复头像不是展示技巧问题，是建模错位的信号。两个维度回答不同的问题——

- 团队维度 = **资源视角**：谁存在、谁忙谁闲、容量与预算消耗。员工是唯一实体，一人一座，办公室隐喻天然成立。
- 项目维度 = **工作流视角**：业务目标推进到哪、任务怎么交接、堵在哪。对应核心模型里 Project 作为业务闭环容器 + 协调线程 + RouteDecision。

**结论：项目不建模成"容器"，建模成"透镜/覆盖层"。** 地图底座保持团队维度（员工唯一），选中项目后高亮其参与者并在座位间画任务交接连线。员工仍只出现一次，项目是画在他们之间的"线"，不是装他们的"框"。单项目的任务级深度视图归项目详情页，运行总览只做"选中 → 高亮链路 → 点击跳转"。

---

## 1. 现状盘点（实现锚点）

| 部件 | 位置 | 现状 |
|---|---|---|
| 页面入口 | `apps/web/src/features/run-overview/index.tsx` | overview + teams + activity 三查询轮询 10s，SSE 秒级插队，轮播焦点 |
| 布局模型 | `runtime-overview-layout.ts` | 3 楼层 × 8 工位/层的静态坐标（polygon + seatGrid），已有 `floorConnectorPaths` 路径绘制视觉语言 |
| 适配器 | `runtime-overview-adapter.ts` | 团队分楼层、员工落座；`"unassigned"` 无工位（问题 1 根因） |
| 模型 | `runtime-overview-model.ts` | `RuntimeOverviewPath { points, tone }` 已存在；员工节点已带 `projects[]`（含 `activeTaskCount` 等） |
| 地图渲染 | `components/runtime-map-stage.tsx`、`runtime-map-svg-layer.tsx`、`team-workspace-renderer.tsx`、`employee-avatar-node.tsx` | SVG 层已支持画路径 |
| 项目任务图数据 | `apps/web/src/lib/api/projects.ts` `getProjectTaskGraph(projectId)` → `/task-graph` | 已有：节点（`assigned_digital_employee_id`、`stage_index`、`status`、`current_blocker`、started/finished）、边（`dependent_task_id`/`blocker_task_id`/`edge_status`）、参与员工（含头像） |
| 项目列表 | `listProjects` | 已有，可做项目选择器数据源 |

**关键事实：覆盖层不需要任何新后端。** 任务图 API 是项目详情页在用的现成端点，覆盖层是把这条已有数据线接到地图上。

---

## 2. P1：候岗区（修问题 1，小）

未入团队的员工坐进固定"候岗区/大厅"，语义自然（"还没分配部门的新员工在大厅"）。

- **布局**：`runtime-overview-layout.ts` 在 floor-1 增加一个候岗区专属 slot（`decorationVariant` 增加 `"lobby"` 或复用 `"standard"`，视觉上与团队工位区分——无团队卡片、区域名固定"候岗区"）。
- **适配器**：`"unassigned"` 伪团队获得该工位；员工照常落座。**不计入** `teamCount`、楼层 `capacityTotal`/`capacityUsed` 与顶部容量汇总（候岗不是容量），但员工状态计数照常计入。
- **溢出**：候岗员工数超过座位数时，多余的以"+N"徽标聚合在区域角落（不静默丢弃——P1 的底线是地图与汇总数字一致）。
- **交互**：候岗员工可选中、进轮播、出现在侧栏动态流，与普通员工无差别；侧栏详情卡团队字段显示"未归属团队"。

## 3. P2：项目覆盖层（修问题 2，主体）

### 3.1 交互模型

- **项目选择器**：侧栏（`runtime-overview-side-panel.tsx`）增加项目区块，列出关联项目（数据源 `listProjects`，按活跃排序；员工节点已有 `projects[]` 可反向聚合兜底）。选中一个项目进入"透镜态"。
- **透镜态**：
  - 该项目参与员工的头像高亮，其余员工降暗（dim，不隐藏——底座仍是全景）；
  - 座位之间按任务图绘制**有向交接连线**：`ProjectTaskGraphEdge`（blocker → dependent）投影为 `RuntimeOverviewPath`，起终点 = 两任务 `assigned_digital_employee_id` 对应员工的座位坐标；
  - 连线着色语义：已完成边 `muted`、当前活跃边 `primary`、阻塞（`current_blocker` 或 error）`warning`；当前停留环节的员工头像加脉冲标记；
  - 未派发（无 assignee）的任务不画线，在项目卡上以"待派发 N"计数呈现。
- **轮播互斥**：进入透镜态即暂停轮播（复用现有 interacted/pause 机制）；退出透镜（取消选中/超时）恢复。
- **跨楼层**：参与者不在当前楼层时，连线画到楼层切换指示（画布边缘出口图标 + 目标楼层徽标），点击徽标切楼层；楼层 tab 上给参与楼层加项目色点。
- **点击跳转**：透镜态的项目卡提供"查看项目详情"入口（TanStack Router `Link`），任务级操作一律在项目详情页做——运行总览不做任务操作（非目标，见 §5）。
- **URL 状态**：复用现有 `?project=` search param（`index.tsx` 已声明未消费），支持从项目详情页反向链入透镜态。

### 3.2 数据流

- 选中项目时按需 `useQuery` 拉 `getProjectTaskGraph(projectId)`，轮询与 overview 同频（10s）；SSE activity 事件到达时一并 invalidate。未选中不拉，零常驻成本。
- 任务图员工 ↔ 地图座位的连接键 = `digital_employee_id`；参与员工不在 overview 100 条内或无座位（候岗溢出）时，连线端点退化为区域锚点 + 列表兜底展示，不崩溃。

### 3.3 模型与组件改动清单

| 文件 | 改动 |
|---|---|
| `runtime-overview-model.ts` | 新增 `RuntimeOverviewProjectLens`（projectId、参与员工、边投影、阻塞摘要）；`RuntimeOverviewPath` 复用 |
| 新增 `runtime-overview-project-lens.ts` | 任务图 → 透镜投影纯函数（可单测：多任务同员工去重、无 assignee 过滤、跨楼层拆边） |
| `runtime-map-svg-layer.tsx` | 渲染透镜边（箭头、色调、脉冲） |
| `employee-avatar-node.tsx` | 高亮/降暗态 |
| `runtime-overview-side-panel.tsx` | 项目选择区块 + 项目卡（阻塞摘要、待派发计数、详情入口） |
| `index.tsx` | 透镜态 state、`?project=` 消费、任务图查询、轮播互斥 |

---

## 4. 验收（GATE，真实 E2E）

前置：`scripts/dev-services.sh status` 确认全栈运行；浏览器走真实 Control Plane 数据（非 fixture）。

- **P1-G1 候岗可见**：造一个无团队的数字员工（真实 API 创建），运行总览地图候岗区出现其头像，可选中、详情卡显示"未归属团队"；顶部员工数与地图可见数一致。
- **P1-G2 容量不污染**：候岗员工不改变任何团队/楼层容量数字。
- **P2-G1 透镜高亮与链路**：选一个有多任务依赖链的真实项目（可用既有 E2E 项目或新造：A→B 两任务派给不同员工），选中后两员工高亮、之间出现有向连线，其余员工降暗。
- **P2-G2 状态着色**：项目内造一个阻塞/错误任务，对应边为 warning 且项目卡显示阻塞摘要。
- **P2-G3 跨楼层**：参与者分属不同楼层的项目，当前楼层显示出口指示，点击徽标切到目标楼层。
- **P2-G4 跳转与反向链入**：项目卡跳项目详情（Router Link）；带 `?project=` 直开 URL 进入透镜态。
- **P2-G5 退出恢复**：取消选中后高亮/连线清除、轮播恢复。

分层门禁：`corepack pnpm verify:web`（组件/单测，透镜投影纯函数必须有单测）；`__screenshots__` 快照更新。实现前必须阅读 `DESIGN.md`。

---

## 5. 非目标与风险

**非目标**：
- 不做项目维度的页面重构（无项目泳道/看板视图）；跨项目对比需求出现时另立 spec，且那时"重复头像 = 参与关系"的语义已由本 spec 铺垫。
- 运行总览不承载任务级操作（派发、返工、签署），一律跳项目详情。
- 不动 activity/SSE 通道与轮播机制本身。

**风险与债**：
- overview 员工查询 `limit: 100` 截断债依旧（本 spec 不扩，透镜端点退化逻辑已兜住）；超过 100 员工时地图本身已失真，属既有债。
- 任务图边基于 blocker 依赖，与"实际交接顺序"在并行分支上不完全等价——连线语义标注为"依赖/交接"，不承诺时序。
- 静态座位坐标下高密度连线可能交叉难读；P2 先按最短路径直线 + 悬停聚焦（悬停某边时其余边降暗），不做自动布线。

## 6. 开放问题（实现前拍板）

1. 候岗区放 floor-1 固定位（挤占现有 8 工位布局需微调坐标）还是每层一个？——倾向 floor-1 单点，简单且语义是"全公司大厅"。
2. 项目选择器只列"当前有活跃任务的项目"还是全部活跃项目？——倾向前者 + "显示全部"展开，控制列表长度。
3. 同一员工在链路中承担多个任务节点时，连线是否合并同端点边？——倾向合并 + 徽标计数，避免重线。

## 7. 实施记录（2026-07-18）

P1+P2 一次落地入 main（merge 27911caf，feature commit 5c4c36f6），§6 三问题均按倾向执行。与 spec 的偏差与实况：

- **候岗区形态（15:10 修订，用户裁定）**：初版放 floor-1 右侧竖条空带（左上是会议室，布局测试禁放）被否——底图上无对应办公区语义、仅 3 座多数候岗员工仍不可见。终版为**独立"大厅"楼层**：仅有未归属员工时出现在楼层 tab（人数徽标"大厅 · N"），层内 10 座开放区候岗工位 + 溢出 +N；候岗员工与团队员工的透镜链自然跨层走出口徽标。**底图已到位（20:05，main 829e656c）**：用户提供 `floor-lobby-office-v4.png` 并将候岗区改为动态承载区（无固定 seatId、12 个 3×4 展示锚点、超出 +N）；终验揪修 1 个重构回归——透镜端点解算依赖 seatId 致候岗参与者的交接边 unlocated、跨层徽标消失，修法为锚点表提升到 layout 层导出、渲染与透镜共用同组锚点同序对应（锚点对应单测锁定）。
- **"零新后端"修正**：task-graph 端点强制 demand/coordination_job 域（服务端硬校验），不存在项目全量任务图。透镜与项目详情页同规则取最新 demand（`listProjectDemands limit 1` → `getProjectTaskGraph(demandId)`）。多 demand 项目的透镜只显示最新 demand 链路——若需全项目视图需后端新端点，暂不立项。
- **路由**：`/run-overview` 补 `validateSearch`（employee/project 可选），项目详情页既有"在运行总览查看"链接（`?project=`）与之打通。
- **GATE 结果**：P1-G1/G2、P2-G1/G2/G4/G5 真实浏览器 E2E 全 PASS（真实 CP+PG，fixture 直插锚点项目 4be304f7）。**P2-G3 于 15:05 补验 PASS**：大厅楼层改造后候岗参与者与团队参与者天然跨层——大厅层"自 1层"入口徽标点击切至 floor-1，同层边 + "转 大厅"出口徽标均正确（初版曾因 teams 授权可见性残缺无法构造跨楼层布局而阻塞，独立大厅层使该场景不再依赖多团队环境）。
- **E2E 新发现既有缺口（非本次引入）**：员工归属的团队若对当前用户不可见或非 active（teams API 不返回该团队），该员工既无团队工位也不入候岗区（候岗区只覆盖 `team_id` 为空者），地图上仍不可见且与汇总数字不一致。修复方向候选：候岗区兜底口径从"无 team_id"扩为"无可见工位"，或在地图角落加"不可见团队 N 人"计数。待立项。
- **E2E fixture 留存**：锚点项目内 demand/任务/依赖/成员（id 前缀 `e2e1`/`e2e2`），task-c 已还原 pending；员工 5ad34075、17e3a6ee 移入默认团队（后者原属团队 38c325cf"abc"，该团队对 admin 不可见）。清理 SQL：按 id 前缀删 project_tasks/project_task_dependencies/project_demands 即可。
