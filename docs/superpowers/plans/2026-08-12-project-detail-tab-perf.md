# 项目详情页签切换性能：可落地方案

- 日期：2026-08-12
- 状态：已实施
- 关联：`docs/superpowers/specs/2026-08-12-project-detail-dossier-task-table.md`
- 现象：项目管理里切需求 / 点「流程·决策·历史·资产」，尤其「流程」，体感 2～3 秒
- 证据基线（`provider-semantic-e2e-p1`，本 checkout）：
  - `dossier?sibling_pending=true` ≈ **2.2s**（无 sibling ≈ **1.4s**）
  - `task-graph`（2 节点）≈ **1.1s** / ~153KB（含 104 decisions + 100 events）
  - 同需求点「流程」仍会重打 dossier≈1.5s + graph≈1.4s（`staleTime:0` + 页签卸载重挂）
  - 切另一条需求墙钟 ≈ **2.5s**（dossier 主导）

---

## 0. 对「该不该进缓存 / DB」的判定

| 数据 | 放哪 | 结论 | 理由 |
|---|---|---|---|
| 当前选中 demand 的 dossier / task-graph（浏览器会话内） | **前端 React Query 短缓存** | **要做（P0）** | 已有 SSE `useProjectActivityInvalidate` 在活动时 invalidate；再配有界 `staleTime` 不会比今天的「每次重拉」更不一致，只会少打冤枉请求 |
| 左轨 `sibling_pending` 角标 | **仍实时算，但改成一条聚合 SQL** | **要做（P1）** | 事实源仍是 `project_decision_requests`；不物化，只消灭「拉 200 需求 + 500 决策 + 500 任务 + 500 修订进内存」的读放大 |
| task-graph 的 nodes/edges/gates | 继续现算 | **短期不进 DB** | 写路径多（派发/完成/闸门/恢复）；先瘦响应比物化安全 |
| dossier 时间线 / 右轨 / 交接判定 | 继续现算 | **本批不进 DB** | 事件叙事 + 多表拼装，物化表要跟几乎所有协调写路径，一致性成本远高于收益 |
| 全量 dossier / graph 的服务端 Redis 缓存 | 可选后续 | **本批不做** | 失效键难画干净；有 SSE 时前端短缓存已覆盖「切页签」主痛点 |

原则：**能靠「少算、少传、少重挂」解决的，不先上物化表。** 物化只留给「读放大已证伪、且有清晰单一写边界」的字段（本批只有 sibling 计数够格谈，但仍优先 SQL 聚合而非侧表）。

一致性底线（全程遵守）：

1. **权威事实仍在现有表**（decision / task / event / artifact…），读模型只是投影或查询优化。
2. **任何缓存必须有界 + 可失效**：前端靠 SSE invalidate；后端不做跨请求“假装新鲜”的静默缓存。
3. **有缓存数据时禁止因 refetch 清空主界面**（可背景刷新，不可转圈盖住已有图/表）。
4. **不引入第二份待决计数 SoT**（禁止为角标单独写一张会漂的计数表，除非另开设计并覆盖全部 decision 写路径）。

---

## 1. 目标与非目标

### 目标（可验收）

在同等数据规模的 `provider-semantic-e2e-p1` 上：

| 场景 | 现状 | 目标 |
|---|---|---|
| 同需求：任务 → 流程 → 历史 → 任务 | 每次重打 dossier+graph ~1.4s+ | **主内容 <200ms 可见**；网络可有后台刷新，但无「正在加载」清空 |
| 切到另一条 demand | ~2.5s | **首屏有意义内容 <1.2s**（河/任务表或图骨架）；dossier 完整块可稍后补齐 |
| 左轨角标 | 绑在重 dossier 上 | **不依赖**每次完整 dossier；角标接口/字段 P95 明显低于完整 dossier |

### 非目标

- 本批不做 dossier / task-graph 的 DB 物化表或 Redis 读缓存。
- 不改决策写入路径、不改 inbox SoT。
- 不把「流程」页签改成新业务对象。

---

## 2. 分阶段落地

### P0 — 前端：消灭「已有数据还等 2 秒」（效果最大、零一致性风险）

**改什么**

1. `demand-dossier` / `project-task-graph` 查询：
   - `staleTime: 15_000`（或 20_000）
   - 保留现有 SSE invalidate（`useProjectActivityInvalidate`）与写操作后的 invalidate
   - 保留 drive 密度下的长轮询兜底，但 **有 `data` 时 UI 不得因 `isFetching` 卸载主内容**
2. 页签：
   - 「流程」「历史」对 `SoftTabsContent` 使用 `forceMount` + CSS 隐藏，或把 `ProjectDemandsSection` 上提到壳层只切换 `pane`
   - 避免每次点「流程」都重新挂载 `@xyflow/react` + 重新订阅两套 query
3. 「流程」页：
   - 图数据优先用壳层已拉的 `graphQuery`（props 下发），section 内不要在已有同源 cache 时再制造第二套 loading 语义
4. 切 demand：
   - 继续 `placeholderData: keepPreviousData`，但 **过滤上一单 dossier 时显示骨架/河标题用 demand 列表字段**，不要整页「正在加载这一单」挡住河与任务表（任务表可先用新 demand 的 graph）

**一致性**：只影响「多久再自动重拉」；活动 SSE 与 mutation invalidate 仍强制新鲜。最大陈旧窗口 15～20s，且仅在无活动时出现——与今天 30s 兜底轮询同量级，更短。

**验证**

- 浏览器 Performance：同需求连点 流程/历史/任务，`dossier`/`task-graph` **不应每点必打**；有打也须背景进行。
- Vitest：缓存命中时不出现 `正在加载执行图` / `正在加载这一单`（构造已有 queryClient cache）。

---

### P1 — 后端：sibling 从「四表灌内存」改为「一次聚合」（降 dossier ~0.8s，且语义不变）

**现状问题**

`resolveDemandDossierSiblingPending` 每次：

- `ListProjectDemands(limit=200)`
- `ListDecisionRequests(limit=500)`
- `ListProjectTasks(limit=500)`
- `ListPlanRevisions(limit=500)`

只为得到 `demand_id → open_decisions`。

**改法（推荐）**

新增 sqlc 查询（示意，以仓库现有 open 状态集合为准，与 Go `isOpenDecisionStatus` 对齐）：

```sql
-- name: ListProjectDemandOpenDecisionCounts :many
-- 左轨角标：需求级（经 plan_revision）+ 任务级（经 project_task.demand_id）pending 决策计数。
SELECT d.id AS demand_id,
       COUNT(pdr.id)::int AS open_decisions
FROM project_demands d
LEFT JOIN project_decision_requests pdr
  ON pdr.tenant_id = d.tenant_id
 AND pdr.project_id = d.project_id
 AND lower(COALESCE(pdr.status_snapshot, '')) IN ('pending', 'requested', /* 与现口径一致的集合 */)
 AND (
      pdr.project_task_id IN (
        SELECT t.id FROM project_tasks t
        WHERE t.tenant_id = d.tenant_id AND t.project_id = d.project_id AND t.demand_id = d.id
      )
      OR pdr.plan_revision_id IN (
        SELECT r.id FROM project_plan_revisions r
        WHERE r.tenant_id = d.tenant_id AND r.project_id = d.project_id AND r.demand_id = d.id
      )
   )
WHERE d.tenant_id = $1 AND d.project_id = $2
GROUP BY d.id;
```

（实施时用等价、可 EXPLAIN 的写法；必要时拆 task-level / demand-level 两个 COUNT 再合并，避免相关子查询过慢。）

**API 形状（二选一，优先 A）**

- **A（更干净）**：`GET /projects/{id}/demands` console 列表增加可选 `open_decision_count`（或并行 `GET .../demands/pending-counts`），左轨不再依赖 dossier 的 `sibling_pending`。
- **B（改动更小）**：dossier 仍可带 `sibling_pending`，但内部改走聚合 SQL；前端 P0 后切页不再狂打，切 demand 仍受益。

**一致性**：计数仍是读时点查询，与今天相同；只是不再截断在「500 条决策窗口」这种假全集上——**反而可能比现在更准确**（现状 `limit=500` 在大项目会漏计）。

**不做**：`project_demand_open_decision_counts` 侧表 / 触发器（除非 P1 SQL 仍 >300ms 再单开设计）。

**验证**

- Go 单测：与旧 `resolveDemandDossierSiblingPending` 在夹具上逐 demand 计数一致。
- 真库：同一项目旧实现 vs 新 SQL 对比（脚本或临时 dual-read 日志），差异为 0。
- 延迟：`dossier?sibling_pending=true` 应从 ~2.2s 降到接近无 sibling 的 ~1.4s 或更低。

---

### P2 — 后端：瘦 `task-graph` 默认载荷（降流程页 ~1s 与 150KB）

**问题**：2 节点图仍带大量 historical `decision_requests` + `recent_events`，流程画布用不到这么多。

**改法**

1. OpenAPI 为 `GET .../task-graph` 增加显式查询参数，例如：
   - `include=nodes,edges,gates,stage_summaries,employees,handoff`（默认）
   - 可选 `decisions` / `recent_events` / `runs` / `execution_summaries`
2. 控制台流程页与任务表默认 **不**要 `recent_events`；决策页签继续用页面已有 `decision_requests` 列表，不靠 graph 塞历史。
3. 若某 UI 需要图内决策（节点「当前处理」），只返回 **非终态** 或 **每 task 最新一条**，禁止默认全历史。

**一致性**：同一张表、同一 handler；只是响应字段裁剪。旧客户端若依赖默认全量，需在契约写清默认 include，并在 CHANGELOG 标明行为收窄（本仓 console 同步改）。

**验证**

- 契约测试 + 前端流程页：节点/边/闸门/当前处理仍正确。
- 同一 demand：payload 从 ~150KB 降到数十 KB 量级；耗时目标 <400ms（视库而定，但应明显下降）。

---

### P3 — 可选加速（有收益再做，仍避免物化）

1. **dossier 内查询并行化**：`loadDemandLaunchFacts` 里无依赖的 List* 用 `errgroup` 并行（仍读权威表，无缓存）。预期从 ~1.4s 再削一截。
2. **切 demand 时分优先级**：先 `task-graph`（任务表/河）再补 dossier 叙事；UI 已在 P0 支持。
3. **仅当 P1 聚合仍慢**：再评估「pending 计数」物化——必须列出全部 decision 创建/决议/取消写路径的更新点，并有对账任务；**本方案默认不启动**。

---

## 3. 明确拒绝的做法（防一致性事故）

| 做法 | 为何拒绝 |
|---|---|
| 把整份 dossier JSON 存 DB / Redis 当主读 | 写路径极多，失效不全就会时间线/待办撒谎 |
| 前端 `staleTime: Infinity` 且弱化 SSE | 执行中项目会停在旧图 |
| 为抢速度，task-graph 与 list tasks 用两套任务状态口径 | 已踩过计数坑 |
| sibling 计数侧表但只在部分 resolve 路径更新 | 角标假 0 = 漏待办，比慢更严重 |
| 用 `listProjectTasks` 的 20 条窗口估「这一单有多少子任务/待决」 | 宪法与现网已禁止的假计数 |

---

## 4. 实施顺序与工期感

```
P0 前端缓存 + 页签不重挂     → 当天可感：切页签不再「必等 2s」
P1 sibling SQL 聚合          → 切需求/首拉 dossier 降 ~0.5–1s，角标解耦
P2 task-graph include 瘦身   → 流程页网络与解析再降一截
P3 errgroup 并行（可选）     → 进一步压 dossier
```

建议合入策略：P0 可单独 PR；P1、P2 各一 PR（契约变更集中在 P2）。

---

## 5. 验收清单

1. **同需求连切页签**：主内容即时；Network 无「每次点击必发」的 dossier+graph；允许 SSE/定时背景刷新。
2. **切 demand**：河与任务表不整页白屏；dossier 完成后头卡/接续/剧本一致。
3. **角标**：有 pending 的 demand 与收件箱/决策列表交叉抽查一致（P1 后用聚合 SQL）。
4. **流程图**：节点数 = 该 demand graph nodes；闸门/当前处理与任务表不矛盾。
5. **写后新鲜度**：在项目内完成一次决策或任务状态变化后，经 SSE（或 mutation invalidate）后图与卷宗在 1～2s 内更新（与现 throttle 1s 对齐）。
6. **回归**：`go test` dossier sibling / task-graph；web 定向 vitest；真链路上记 pid/cwd。

---

## 6. 预期效果（对照基线）

| 项 | 基线 | P0 后 | P0+P1 后 | P0+P1+P2 后 |
|---|---|---|---|---|
| 同需求切「流程」体感 | 2～3s 或后台仍卡 | **即时** | 即时 | 即时 |
| 切 demand 墙钟 | ~2.5s | ~2.2s（仍打重 dossier） | **~1.2–1.5s** | **≤1.2s**（图更轻） |
| 无活动时数据陈旧上界 | 每次都新，但切换必等 | ≤15～20s，SSE 可提前 | 同左 | 同左 |

---

## 7. 一句话

**该进前端短缓存的，是「当前页正在看的卷宗/图」；该在 DB 侧优化的，是 sibling 这种读放大查询（用聚合，不物化）；整份 dossier 先别落库。** 先 P0/P1 就能把「点流程等三秒」从结构上拆掉，且不引入第二套事实源。
