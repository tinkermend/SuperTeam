# 项目详情卷宗 + 关联任务表落地

- 日期：2026-08-12
- 状态：已实施
- 原型：`docs/prototypes/project-detail-tab-layouts/concept-fused-dossier-river.html`
- 目标读者：实施会话
- 交付性质：Web 项目详情 IA 重组为主；Control Plane **不新造读模型**。唯一契约/查询变更是需求列表的排序（及可选的 `demand_id` 任务过滤，见 §4）

---

## 0. 结论

**现有接口足以支撑原型落地。不需要为「主/子关联」补一张新表或一个新聚合 API。**

关联已经在库里：

| 原型概念 | 权威对象 | 已有字段 / 接口 |
|---|---|---|
| 需求流程（主，一次对话） | `project_demands` | `GET /projects/{id}/demands`：`id/title/status/updated_at/continues_demand_id` |
| 子任务 | `project_tasks.demand_id` | `GET /projects/{id}/task-graph?demand_id=` 的 `nodes`；列表项也带 `demand_id` |
| 当前处理（决策/放行） | `project_decision_requests.project_task_id` | 图里的 `decision_requests` + `dispatch_gates` |
| 关联资产 | `project_artifact_refs.project_task_id` 等 | 卷宗 `rail` / `handoff_assessments` |
| 待你处理角标 | 同项目 open 决策 | 卷宗 `?sibling_pending=true` |
| 阶段河 | 需求状态 + 图阶段 + 验收摘要 | `demand.status`、`stage_summaries`、`dossier.acceptance` |
| 接续链 | `continues_demand_id` | 前端 `foldDemandChains`（已落地） |

缺的不是数据，是**把已有读模型按「这一单」拼到同一张子任务表上**，以及需求列表现在按 `created_at` 排、与已拍板的级联排序不一致。

---

## 1. 对象口径（实施不得再混）

```
需求流程 Demand     = 一次对话发起（主）
  └── 子任务 ProjectTask = 该对话下的可执行节点
        ├── 当前处理     = 挂在该任务上的 decision_request / 派发闸门
        └── 关联         = 该任务的工件 / 证据 / 报告
```

禁止：

- 把 ProjectTask 当成与 Demand 平级的「项目任务清单」
- 为任务表再造一份「子任务」实体（库里没有、也不该有）
- 用 `listProjectTasks` 的 20 条窗口去数「这一单有几个子任务」或「这条需求待决几项」（与概览计数踩过的坑同类）

决策写入口仍走收件箱。项目页只读、定位、跳转。

---

## 2. 界面怎么拼（不新开端点）

默认范围是 **这一单**。选中左轨某条需求流程后：

| 区块 | 读 | 拼法 |
|---|---|---|
| 左轨 | `GET .../demands?limit=&offset=` + 当前卷宗 `sibling_pending` | 行 = 折叠后的链（`latest`）；角标只用 `open_decisions` |
| 阶段河 | 选中需求的 `status` + `task-graph.stage_summaries` + `dossier.acceptance` + playbook/exit + `lineage.continue_demand` | 纯前端投影，四格：需求 / 计划 / 执行 / 结果；河下方挂意图摘要、剧本/收口与「继续这一单」 |
| 任务表 | `GET .../task-graph?demand_id=` | **主行**用当前 Demand；**子行**用 `nodes`。`当前处理` 左连 `decision_requests`（`project_task_id`）与 `dispatch_gates`；`关联` 左连卷宗 `rail.items` / `handoff_assessments`（均已带 `project_task_id`） |
| 流程页签 | 同一份 task-graph | 图与表同源；点节点 = 选中同一子行 |
| 决策页签 | 同一份 `decision_requests`（可再滤 demand 级无 `project_task_id` 的卡，如计划确认 / 验收签署） | 与「当前处理」列同一条记录的两种读法 |
| 历史页签 | `dossier.timeline` | 叙事，不是审计。完整流水仍走已有执行轨迹。时间线疏密由 `signals` 自动推导，**无**「驱动/巡检」人工切换 |
| 资产页签 | 既有 artifacts / budget / acceptance | 验收判据细节落在这里（阶段河只做摘要指针） |

**已退役：**「需求」页签（旧 `?tab=demands` 深链 remap 到 `?tab=tasks` 并保留 `demand=`）。不另做「驱动/巡检」密度产品开关。

员工显示名：图里已有 `employees`；项目页现有 `principalNamesById` 可复用。

任务详情弹层：沿用 `GET .../tasks/{taskId}`（深链缺页回落已存在）。

---

## 3. 要不要补接口：逐项判定

### 3.1 必须改（不是新数据）

**需求列表排序。**

现状：`ListProjectDemands` SQL 为 `ORDER BY created_at DESC`。Web 文案「最新在前」吃的是创建时间。已拍板的级联是：

1. `updated_at DESC`（最近活动）
2. 非终态先于终态（`submitted/recorded/planning_pending/planned/executing/acceptance_pending` 先于 `completed/failed/cancelled/planning_failed`）

`updated_at` 与 `status` 已在 `ProjectDemand` 契约里，**不用迁移、不用新字段**。

实施约束：

- 不要静默改仓储里那条被结项文案 / sibling_pending / 协调线程共用的 `ListProjectDemands` 默认序——那些调用是扫全表再按 id 归组，不该被 Console 排序绑死。
- Console 列表单独一条 sqlc（或显式 `sort=` 参数），HTTP `GET /projects/{id}/demands` 的 Console 调用改走新序。
- 契约补一句排序说明，避免下游以为仍是 `created_at`。

**左轨加载更多。**

`limit/offset` 已在契约里。用 `limit+1` 判断还有没有下一截即可，**不必**先上分页信封。首屏建议 8～20；不要在 228px 左轨放页码。

### 3.2 看起来缺、其实不该补

| 想法 | 为何不补 |
|---|---|
| 新「需求索引」聚合（每条需求带 `task_count`） | 左轨用 `sibling_pending.open_decisions` 足够。子任务个数以任务表为准。用 20 条任务窗去填「3 子任务」会假计数 |
| 新「项目工作台」一次返回轨+表+河 | 卷宗 spec 已拒绝把图画进 dossier。轨 / 图 / 表继续三问，前端拼。禁止平行读模型 |
| 把 `waiting_request_id` 补进 `ProjectTask` 列表契约 | 这一单的「当前处理」从图的 `decision_requests` / `dispatch_gates` 就能连。列表 schema 现在没有这两列，不是这一单的阻塞 |
| 新 `GET artifacts?task_ids=` | 这一单的关联已在卷宗轨与 handoff 判定里。按项目分页再过滤会截断——卷宗 R2 已否决这条路 |
| 搜索 `q=` | 需求条数通常是几十。第一期搜索只滤已加载页；点名翻历史靠加载更多。N 变大再加服务端 `q` |
| 列表改成分页信封 `{items,has_more,total}` | 加载更多不需要 total。有需要时再加，不阻塞第一期 |

原型左轨上的「3 子任务」是示意，**落地不要做成列表字段**。选中后任务表有几行就是几条。

### 3.3 有 SQL、无 HTTP：按范围决定

`ListProjectTasksByDemand` 已在 sqlc 里，被 launch-detail / 图 / 飞书使用，**没有**挂到 `GET /projects/{id}/tasks`。

| 范围 | 要不要暴露 `demand_id` |
|---|---|
| 默认「这一单」 | **不要。** `task-graph?demand_id=` 已按需求返回全部节点（按 `stage_index`） |
| 「全项目」要把每条需求下的子任务收全 | 才值得把 `demand_id` 接到 listProjectTasks。那是查询参数，不是新资源 |

第一期「全项目」若要做：继续用现有 `listProjectTasks`（`ORDER BY updated_at DESC`，现网 limit 20），**按 `demand_id` 分组展示**，并写明「最近更新窗口，不是全量子任务」。禁止把窗口长度显示成需求下的子任务总数。

任务表分页只出现在「全项目」。脆数据面用页码；与左轨加载更多分工，见原型。

---

## 4. 第一期 / 第二期

### 第一期（足以修错配）

1. 项目详情默认骨架改为：左轨需求流程 + 阶段河 + 页签（任务默认）。
2. 任务表：选中需求的 graph nodes 分组（主行 Demand，子行 Task + 当前处理 + 关联）。
3. 需求列表走新排序；左轨加载更多；角标继续 `sibling_pending`。
4. 流程 / 决策 / 历史 / 资产页签消费同一 demand 的 dossier + graph，不各拉一份任务清单；接续入口在阶段河。
5. 深链：`?tab=tasks&demand=` 选中左轨并展开该单子任务；`?task=` 仍开既有弹层。

### 第二期（明确后再做）

- `GET /tasks?demand_id=`：全项目要收全每条需求的子任务时再暴露。
- `GET /demands?q=`：左轨点名搜索。
- 若产品坚持左轨显示子任务数：单独做 **按 demand 的 COUNT 聚合**（全表，禁止分页片），不要塞进 dossier。

---

## 5. 已知诚实边界（落地必须写在 UI 上）

1. **需求列表分页会拆接续链。** `foldDemandChains` 已规定：父单不在本页则该行当链头。加载更多后可再折。不要为第一期做跨页递归取链。
2. **`sibling_pending` 内部扫 demands 200 + decisions 500 + tasks 500。** 左轨角标不是无限精确；超窗与今天行为相同，不在本方案扩大扫描。
3. **任务列表 20 条窗口**只用于「全项目」降级，不用于「这一单」表体。
4. **时间线 ≠ 审计。** 历史页签用 dossier 叙事；要全事件走执行轨迹。

---

## 6. 验证（实施时，不是本文件）

本文件只定口径。实施完成后才做真实链路：

- 同一需求下，任务表子行数 = 该 demand 的 graph nodes（非 dismissed）。
- 子行「当前处理」与收件箱同一 `decision_request`（类型中文走 `status-labels`）。
- 刚关闭的需求排在更早、仍在执行的需求之上（`updated_at` 第一键）。
- 左轨加载更多不丢当前选中项；搜索仅作用于已加载集时，文案不得暗示全库检索。

---

## 7. 非目标

- 不改决策写入路径、不改协调线程、不改 Runtime。
- 不把人类决策建模成数字员工任务。
- 不在核心模型增加「主任务 / 子任务」封闭枚举；主/子是这一屏的展示层级（Demand / ProjectTask）。
