# 收件箱 UI/UX 缺陷整治与裁决工作台收敛（Inbox UX Remediation）

- 日期：2026-08-07
- 状态：**已完结**（2026-08-08 `361aa3ac` 落地批 A/B/C；§5.2 以本页内嵌 Project/User 选择器兑现，未等待 task-hub 共享 Picker 抽离。**2026-08-09 补跑 §7.3 真实 E2E 12/12 全过，并修复迁移引入的定高工作台回归**——见 §7.3 执行记录）
- 系列：与 `2026-08-07-task-hub-ux-remediation-design.md` 同批（同一份外部逐页审查的第 2 页）；两份 spec **无代码交集**，可并行实施
- 交付性质：**跨两层**——Web 前端（显示/布局/交互）+ Control Plane 读路径补名（§3.1）。**不改契约**（`context` 在 [openapi.yaml](../../../contracts/control-plane/openapi.yaml) 里是自由 `type: object`）、**不改数据库**、**无迁移**
- 目标读者：实施会话（本文自包含，含逐项证据行号、线上实测数据与验收判据）
- 核实基线：代码 = `main` @ `4cb267cd`；线上数据 = 开发库 `inbox_items` 50 行（12 open），2026-08-07 实测

---

## 1. 背景

同一份外部 UI/UX 审查对 `/inbox` 提出 6 条问题（1×P0、3×P1、2×P2）与 1 条重构方向。逐条核对代码 + 线上库实测后：**4 条属实、1 条前提不实、1 条已在上一次提交修复**；重构方向对，但其落法（「列表内一键决断」）与服务端动作契约冲突，须改口径。

同时代码里另有 3 条审查未覆盖的缺陷，其中 1 条（布局宪法违规）严重度高于原审查的任何一条。

### 1.1 原审查逐条核实结论

| 原判 | 核实 | 本 spec 处置 |
|---|---|---|
| P0 列表 meta 裸 UUID | **属实，且归因错、范围被低估**：不是 `formatContext` 回退链坏，是数据侧缺 `demand_title`；**线上 12 条 open 里 6 条命中** | §3.1 收（改问题定义 + 改修法层） |
| P0 planner 摘要需人类可读化 | **前提不实**：`summary` 是**用户自己写的需求正文**，不是系统生成串 | §6.1 不做（说明原因） |
| P1 卡片信息过载 | **属实，但重复项列错了；实际更强**：`current_node` 在线上 **50/50 缺失**，meta 节点**必然**等于分组表头 | §3.3 收（改问题定义） |
| P1 主从动作路径偏长 | **半属实，建议不可直接采用**：弹窗承载决策框架 + 后果说明 + 强制备注 + 扩编选人，不是确认框 | §5.1 收（降级为「弹窗快捷入口」） |
| P1 关联引用 meta 英文键 `demand_id ↗` | **已修，审查看的是旧版**（落在 `4cb267cd`） | §6.2 不做 |
| P2 更多筛选含 UUID 字段 | 属实 | §5.2 收 |
| P2 右侧空态偏长 | **位置说错（是中栏），且低估**：真问题是整页违反布局宪法三条硬规则 | §4 收（升级为布局宪法整改） |
| 重构方向：收件箱＝裁决工作台 | **方向对**，但「列表扫读→一键决断」须按 §5.1 改口径 | §4 + §5 合并承载 |

### 1.2 审查未覆盖、本 spec 纳入的缺陷

- §3.2 `kind = project_task_runtime_recovery` **不在任何词表**，meta 行原样吐英文枚举 —— 违反 CLAUDE.md 硬护栏，线上 3 条
- §3.4 `summary` 与 `why` **同句渲染两遍**（`humanTaskWhy` 对未登记 kind 直接 `return summary`），线上实测命中
- §3.5 右栏「快速跳转」两条目指向**同一 URL**
- §4 布局宪法三条违规（手写主从栅格 / 常驻空态栏 / 手写 KPI 栅格）—— **本页最重缺陷**

---

## 2. 范围边界

### 做

| 批 | 内容 | 触及层 |
|---|---|---|
| A（§3） | 显示层缺陷：需求补名、词表漏键、meta 去重、why 重复、跳转重复 | Web + **CP 读路径**（仅 §3.1） |
| B（§4） | 布局宪法整改：主从栅格 / 常驻空态 / KPI 栅格 / 视口断点 | Web |
| C（§5） | 交互与筛选：行内 CTA（降级口径）、筛选器去 UUID | Web |

文件面：`apps/web/src/features/inbox/`、`apps/web/src/lib/status-labels.ts`、`apps/control-plane/internal/inbox/`、`apps/control-plane/internal/storage/queries/inbox.sql`。

### 明确不做

| 不做项 | 原因 |
|---|---|
| 改写 `summary`（原审查 P0 后半） | 它是用户原话，见 §6.1 |
| 关联引用 meta 中文化 | 已在 `4cb267cd` 完成，见 §6.2 |
| 在 ~20 处 `InboxContext` 构造点补 `demand_title` | 治不了存量卡，且违反「读时补名」既定约定，见 §3.1 |
| 列表内直接提交决策（跳过弹窗） | 无任何动作满足安全条件，见 §5.1 |
| 收件箱分页/虚拟滚动 | 原审查未提，当前无证据表明是瓶颈 |
| `verify:design-system` 增加手写栅格扫描护栏 | 见 §8 U4：值得做但属独立切片，不塞进本批 |

---

## 3. 显示层缺陷（批 A）

### 3.1 需求补名：消灭裸 UUID（**修正原审查的归因与修法层**）

**原审查说**「修 `formatContext` 回退链」——**修法层错了**。回退链本身按 D3 规范写得正确，问题是它拿到的数据里就没有名字。

**现状链路**：

1. `readDemandRefs` 在 `context.demands` 缺席、只有 `demand_id` 时，返回 `{ id, title: demandTitle ?? demandId! }`（[inbox-item-list.tsx:351-362](../../../apps/web/src/features/inbox/components/inbox-item-list.tsx#L351)）——**把 UUID 当成了标题**。
2. `context.demands[]` 存在但条目 `title` 为空时同样回退 `id`（[:320-322](../../../apps/web/src/features/inbox/components/inbox-item-list.tsx#L320)）。
3. `formatContext` 优先取 `primaryDemandLabel`（[:381-388](../../../apps/web/src/features/inbox/components/inbox-item-list.tsx#L381)），于是 UUID 直接成为列表 meta 主文本。
4. 同一个 UUID 继续流向：详情面板「关联对象」（[inbox-shell.tsx:688](../../../apps/web/src/features/inbox/components/inbox-shell.tsx#L688) `formatContext(item) ?? item.source_id`）、「关联引用 · 关联需求 · \<uuid\>」（[:697-713](../../../apps/web/src/features/inbox/components/inbox-shell.tsx#L697)）。

**线上实测（开发库，2026-08-07）**：

```
select status, context_payload->>'kind', context_payload->>'demand_id', title
from inbox_items
where (context_payload ? 'demand_id' or context_payload ? 'primary_demand_id')
  and not (context_payload ? 'demand_title') and not (context_payload ? 'demands');
```

返回 **6 行，全部 `status=open`**（3× `plan_review`「确认项目计划版本」+ 3× `casting_expansion`「扩编请求」）。当前 open 事项共 12 条 —— **半数卡片的关联对象显示为裸 UUID**。

**根因**：`demand_id` 写入 `InboxContext` 的构造点在 `project_store.go` 里有 **23 处**，只有 **2 处**顺带写了 `demand_title`。本次命中的两处均未写：`planRevisionReviewContext`（plan_review）与 [casting_expansion.go:90](../../../apps/control-plane/internal/project/casting_expansion.go#L90)（casting_expansion）。

> **实施须知**：`apps/control-plane/internal/workflow/projectcoordination/project_store.go` 正被并发会话改动（本文核实期间行号已漂移约 40 行），故此处**按符号名而非行号引用**。实施时用 `grep -n '"demand_id"' / '"demand_title"' / 'planRevisionReviewContext'` 现查。本 spec 的改动**不触碰该文件**，只是把它作为根因证据。

**为什么修在读路径而不是这 20 个写入点**：

- 写入点修法**治不了存量**——已落库的 6 张卡永远是 UUID，除非再做数据回填；
- CLAUDE.md 与 DESIGN.md 的既定约定就是「名称由服务端读路径批量补名」，`source_project_name` / `source_task_name` 已是该模式的先例（[service.go:136-201](../../../apps/control-plane/internal/inbox/service.go#L136) `enrichSourceNames`，注释自陈「读时解析,不入库快照」）；
- 一处修，覆盖所有 kind、所有历史卡。

**改法**：

1. **新增 sqlc 查询**（[inbox.sql:309-319](../../../apps/control-plane/internal/storage/queries/inbox.sql#L309) 已有两条同形先例，照抄）：

   ```sql
   -- name: ListInboxDemandTitles :many
   -- 收件箱来源补名:批量取需求标题(读时解析,不入库快照)。
   SELECT id, title FROM project_demands
   WHERE tenant_id = sqlc.arg('tenant_id')::uuid
     AND id = ANY(sqlc.arg('ids')::uuid[]);
   ```

2. **Repository 接口 + PgRepository** 增 `DemandTitles(ctx, tenantID, ids) (map[uuid.UUID]string, error)`，与 [repository.go:20-21](../../../apps/control-plane/internal/inbox/repository.go#L20)、[pg_repository.go:133-160](../../../apps/control-plane/internal/inbox/pg_repository.go#L133) 的两个既有方法同形。

3. **`enrichSourceNames` 扩展**：扫描每条 item 的 `ContextPayload`，收集 `primary_demand_id`、`demand_id`、`demands[].id` 三处 UUID；批量查回标题后**写回内存中的 `ContextPayload`**：
   - `demand_title` 缺失时补上；
   - `demands[i].title` 为空或等于其 `id` 时补上。
   - 该 map 只用于本次响应序列化，**不回写数据库**（与既有两个补名字段同语义）。
   - 查不到（需求已删）时**保持不写**，交给前端 §3.1.4 兜底。

4. **前端兜底**（这一层仍要改，否则需求被删时又回到裸 UUID）：`readDemandRefs` 两处 `title` 回退由 `id` 改为 `missingObjectLabel("demand", id)`。词表**已支持 `demand` 这个 kind**（[status-labels.ts:570-587](../../../apps/web/src/lib/status-labels.ts#L570)），只是从未接到需求上。同时 [inbox-shell.tsx:688](../../../apps/web/src/features/inbox/components/inbox-shell.tsx#L688) 的 `?? item.source_id` 改为 `?? missingObjectLabel("object", item.source_id)`。

**注意护栏测试的既有空洞**：[format-context.test.ts](../../../apps/web/src/features/inbox/format-context.test.ts) 的「D3 不得出现完整 UUID」只覆盖了**项目**分支（:26-37），需求分支从未被断言 —— 线上 bug 正长在这个洞里。本项必须补上需求分支的对称用例。

**判据**：
- 线上那 6 条 open 卡片，列表 meta、详情「关联对象」、「关联引用」三处均显示需求标题，无 36 位 UUID；
- 手工把某条卡的 `demand_id` 改为一个不存在的 UUID，三处显示为「未命名需求 (xxxxxxxx…)」而非裸 id；
- `rg` 全站收件箱代码，`title` 回退分支不存在直接返回 `id` 的路径。

### 3.2 `kind` 词表漏键：meta 行裸英文枚举

**现状**：[formatCurrentNode](../../../apps/web/src/features/inbox/components/inbox-item-list.tsx#L418) 回退到 `humanTaskKindLabel(item.kind)`，后者在两张词表都查不到时**原样返回入参**（[status-labels.ts:266-273](../../../apps/web/src/lib/status-labels.ts#L266) → [:242-248](../../../apps/web/src/lib/status-labels.ts#L242)）。

线上实测 kind 分布中，`project_task_runtime_recovery`（**3 条**）既不在 `HUMAN_TASK_KIND_LABELS`（[:254-264](../../../apps/web/src/lib/status-labels.ts#L254)）也不在 `DECISION_TYPE_LABELS`（[:227-239](../../../apps/web/src/lib/status-labels.ts#L227)，只有近似的 `project_task_recovery`）。于是 meta 行渲染出 `… · project_task_runtime_recovery`。

**这是护栏违规**，不只是文案瑕疵：CLAUDE.md 明写「前端用户可见的状态/枚举一律经 `status-labels.ts` 映射为中文，缺键补词表而非在组件内翻译」，`formatCurrentNode` 的源码注释本身也写着「meta 行禁止裸英文技术枚举」。

**改法**：按 [decision_action_registry.go:115-122](../../../apps/control-plane/internal/inbox/decision_action_registry.go#L115) 声明的 task human-wait 家族**全量补齐** `DECISION_TYPE_LABELS`，不要只补线上命中的那一个：

| key | 建议中文 |
|---|---|
| `project_task_runtime_recovery` | 运行时恢复 |
| `project_task_missing_context` | 缺失上下文 |
| `project_task_permission` | 权限确认 |
| `project_task_plan_invalid` | 计划失效 |
| `project_task_budget_approval` | 预算审批确认 |
| `project_task_human_wait` | 人工等待 |

**实施须知**：`HUMAN_TASK_KIND_LABELS` 有契约同源护栏（[human-task-kind-labels.guard.test.ts](../../../apps/web/src/lib/human-task-kind-labels.guard.test.ts) 断言与 [contracts/control-plane/human-task-kind-labels.json](../../../contracts/control-plane/human-task-kind-labels.json) 逐键逐值一致）。上述键属 `decision_type` 层，**应加到 `DECISION_TYPE_LABELS`，不要加到 `HUMAN_TASK_KIND_LABELS`**，否则会打破契约同源断言。

**判据**：`select distinct context_payload->>'kind' from inbox_items` 的每个非空值，经 `humanTaskKindLabel` 后均为中文；页面 meta 行 `rg` 不出现下划线英文串。

### 3.3 meta 行去重（**修正原审查的重复项枚举**）

**原审查说**「标题『确认项目计划版本』+ pill『计划确认』+ 进度『计划确认 待你』+ 类型 pill『项目决策』」四项重复。**枚举不准**：类型 pill 是 `item_type`（「项目决策」），与 `kind` 正交，**不是重复**；而它漏掉了分组表头。

**实际重复**（以线上 `plan_review` 卡为例，三处同词）：

| 位置 | 文本 | 代码 |
|---|---|---|
| 分组表头 | 计划确认 | [inbox-item-list.tsx:63-69,241-244](../../../apps/web/src/features/inbox/components/inbox-item-list.tsx#L63) |
| meta 节点 | 计划确认 | [:204](../../../apps/web/src/features/inbox/components/inbox-item-list.tsx#L204) `formatCurrentNode` |
| 进度条 label | 计划确认 待你 → 执行 未开始 → … | [adapters.go:285](../../../apps/control-plane/internal/inbox/adapters.go#L285) |

**关键实测（把「偶发」证成「必然」）**：

```
select count(*) filter (where context_payload ?| array['current_node','node_title','workflow_node','stage']) as any_node,
       count(*) as total from inbox_items;
→ any_node=0, total=50
```

`formatCurrentNode` 的**首选数据源在线上 0/50 存在**，因此它**永远**落到 `humanTaskKindLabel(item.kind)`，而分组表头正是按同一个 `kind` 生成的 —— 两者恒等，100% 命中，不是个例。

异常处理组同理反向命中：`kind` 为空的告警类卡（线上 7 条：4× `automation_alert`、2× `casting_invalidated`、1× `channel_alert`）走到 `formatCurrentNode` 末尾的 `formatItemType(item)`，于是 meta 文本与紧邻的类型 pill 逐字相同（例：pill「自动化告警」+ meta「自动化规则 · 自动化告警」）。

**改法**：`formatCurrentNode` 拆成两个函数，语义分离：

1. 保留一个**只读真实节点**的版本（`current_node` / `node_title` / `workflow_node` / `stage` 四键），查不到返回 `undefined`；
2. 列表行 meta 只在它有值时渲染 ` · {node}` 段；无值时 meta 只留来源/上下文与时间；
3. 详情面板「当前节点」字段（[inbox-shell.tsx:518-519](../../../apps/web/src/features/inbox/components/inbox-shell.tsx#L518)）保留 kind 回退——**详情面板没有分组表头，不构成重复**，此处回退是有价值的；
4. 进度条 label 由服务端下发，**不改**（它表达的是四步闭环位置，与单点标签不同源）。

**判据**：任一列表行内，分组表头文本不再于该行 meta 中重复出现；告警类卡的类型 pill 与 meta 文本不再逐字相同；详情面板「当前节点」仍有值。

### 3.4 `summary` 与 `why` 同句渲染两遍

**现状**：列表行把 `item.summary` 与 `item.why` 渲染为两段并列的 `line-clamp-2` 段落（[inbox-item-list.tsx:184-193](../../../apps/web/src/features/inbox/components/inbox-item-list.tsx#L184)）。而服务端 [humanTaskWhy](../../../apps/control-plane/internal/inbox/adapters.go#L241) 对未登记的 kind **直接 `return summary`**（[:262-265](../../../apps/control-plane/internal/inbox/adapters.go#L262)），`why` 再由 handler 提升为顶层字段（[handler.go:522](../../../apps/control-plane/internal/inbox/handler.go#L522)）。

**线上实证**（`project_task_runtime_recovery` 那条）：

```
summary = 系统补建人工决策卡：任务已停在待人工确认，但缺少可处理的决策（原因：需要恢复 Runtime）
why     = 系统补建人工决策卡：任务已停在待人工确认，但缺少可处理的决策（原因：需要恢复 Runtime）
```

**改法（前端，一行判断）**：`item.why` 与 `item.summary` trim 后相等时只渲染一段。

**为什么不在服务端改成「未登记 kind 不回填 why」**：`why` 的兜底回填对**没有 summary 的卡**仍有价值，且 `why` 是契约里的具名字段（消费方不止收件箱列表）。前端去重是最小且无副作用的修法。若实施中发现服务端确无第二消费方，可作为可选清理，**不属本 spec 判据**。

**判据**：线上那条 runtime_recovery 卡只显示一段说明；`plan_review` 卡（summary ≠ why）仍显示两段。

### 3.5 「快速跳转」重复条目

**现状**：[inbox-shell.tsx:906-913](../../../apps/web/src/features/inbox/components/inbox-shell.tsx#L906) 两个 `QuickLink`——「查看完整详情」与「查看关联任务」——`to` **都是同一个 `detailHref`**。后者是无意义副本。

**改法**：删除「查看关联任务」条目。任务身份已在详情面板「关联引用」区以 `ObjectRef` + 可点行呈现（[:727-735](../../../apps/web/src/features/inbox/components/inbox-shell.tsx#L727)），不需要在快速跳转再来一次同 URL 的入口。

**判据**：右栏快速跳转区不存在两个 `href` 相同的链接。

---

## 4. 布局宪法整改（批 B · **本页最重缺陷**）

原审查只看到「右侧空态偏长 / 偏说教」，且位置说错了（那三块在**中栏**详情空态 [inbox-shell.tsx:1079-1117](../../../apps/web/src/features/inbox/components/inbox-shell.tsx#L1079)，不在右栏）。底下的真问题是：**这一页同时违反布局宪法三条硬规则**。

宪法出处：[docs/design-system/layout-density.md:20-36](../../../docs/design-system/layout-density.md#L20)、[DESIGN.md:111,113,114](../../../DESIGN.md#L111)、[layout.tsx:58-61](../../../apps/web/src/components/superteam/layout.tsx#L58)。

### 4.1 手写主从栅格 → `MasterDetailLayout`

**违规条文**：layout-density.md:28「必须用 `MasterDetailLayout`，禁止新增手写 `grid-cols-[minmax(0,1fr)_NNNpx]`」；:32 更进一步宣称「页面级手写主从栅格**已全部迁移**」。

**现状**：[inbox-shell.tsx:199](../../../apps/web/src/features/inbox/components/inbox-shell.tsx#L199)

```
xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.15fr)_300px]
```

全站已有 16 个 feature 文件在用 `MasterDetailLayout`（`rg -l MasterDetailLayout apps/web/src`），收件箱是**漏网的那个**——宪法里那句「已全部迁移」对本页是假的。

**改法**：三栏 → `MasterDetailLayout` 两栏：

- `master` = 列表（`InboxItemList`）
- `detail` = **详情 + 动作合并**为一栏，动作区置顶（对应重构方向的「裁决工作台」：选中即见可执行动作，不必再扫到第三栏）
- `rail` 取 `lg`（420px 档）——原三栏的详情 + 动作合计远宽于 340px

### 4.2 撤除常驻空态栏

**违规条文**：layout-density.md:29「详情层按需渲染：未选中对象时不传 `detail`，主列独占全宽；**禁止常驻『请选择』空态占位栏**」。

**现状**：未选中时中栏渲染 `InboxEmptyDetailPanel`（[:1079-1117](../../../apps/web/src/features/inbox/components/inbox-shell.tsx#L1079)）、右栏渲染「选择事项后可操作」卡（[:824-837](../../../apps/web/src/features/inbox/components/inbox-shell.tsx#L824)）——**两个常驻空态栏，宪法明文禁止的正是这个**。

**改法**：未选中时不传 `detail`，列表独占全宽。三块空态内容处置：

| 块 | 处置 | 原因 |
|---|---|---|
| 今日待处理摘要（[:1088-1095](../../../apps/web/src/features/inbox/components/inbox-shell.tsx#L1088)） | **删除** | 三个数字（开放/高风险/阻断）与顶部 KPI 卡逐个重复，同屏两份 |
| 处理顺序建议（[:1096-1103](../../../apps/web/src/features/inbox/components/inbox-shell.tsx#L1096)） | **删除** | 静态三句说教；列表已按风险排序 + accent bar + 高风险计数，规则已经内建在排序里 |
| 选择事项后可执行（[:1104-1114](../../../apps/web/src/features/inbox/components/inbox-shell.tsx#L1104)） | **删除** | 教用户「点一下会看到什么」，点一下即知 |

原审查说这三块「对熟练用户偏说教」——判断对，但结论应是**随常驻空态栏一起删除**，而不是缩短。

### 4.3 KPI 栅格 → `MetricGrid` + `MetricCard`

**违规条文**：layout-density.md:36「必须用 `MetricGrid`，禁止新增手写 `sm:grid-cols-2 xl:grid-cols-4` 式指标栅格」；DESIGN.md:111「概览指标卡（大数字 + 标签）→ `MetricCard`」、:114「KPI 指标带 → `MetricGrid`」。

**现状**：`InboxSummaryCards`（[:250-330](../../../apps/web/src/features/inbox/components/inbox-shell.tsx#L250)）手写 `grid-cols-2 xl:grid-cols-4`（[:293-296](../../../apps/web/src/features/inbox/components/inbox-shell.tsx#L293)），且在 `SoftCard` 上手搓了顶条 / 图标圆底 / 语义色映射三张 Record（`summaryCardSoftBg` / `summaryCardNumText` / `summaryCardAccent`），完整重造了 `MetricCard` 已有的东西。

**改法**：改用 `MetricGrid` 包 4 个 `MetricCard`（`label` / `value` / `icon` / `iconTone` / `loud`，见 [primitives.tsx:162+](../../../apps/web/src/components/superteam/primitives.tsx#L162)），删除三张样式 Record。

**必须保留的语义**：「语义色只在 >0 时点亮，0 保持灰阶」（[:275-276](../../../apps/web/src/features/inbox/components/inbox-shell.tsx#L275) 的注释与 DESIGN.md「数值为 0 或无需人工介入的指标不点亮语义色」）→ 映射为 `loud={highRisk > 0}` / `iconTone` 灰阶档，不得因为换组件而丢掉。

### 4.4 视口断点 → 容器断点

**违规条文**：layout-density.md:20-24「内容区的布局折叠一律用**容器断点**；视口断点仅允许用于 shell 级结构。存量属待偿债务，**触达时迁移**」。

**现状**：`inbox-shell.tsx` 7 处 `xl:`、`inbox-item-list.tsx` 1 处，全在 `Main` 之内。

**改法**：本批正在改这两个文件的布局，**正是「触达时」**。`MasterDetailLayout` 自带 `@container/master-detail` 命名容器，迁移后大部分 `xl:` 自然消失；剩余的改用 `@container/content` 变体。

### 4.5 关于「没有护栏所以没被发现」

`verify:design-system`（[docs/design-system/verify-design-system.mjs](../../../docs/design-system/verify-design-system.mjs)）**不扫描 feature 代码**，`grid-cols` / `MasterDetail` 在该脚本里零出现。§4.1–4.4 四条全部是**纯散文规范、无自动化护栏**，所以能在全站迁移后仍留一个漏网页面而门禁全绿。

补护栏是对的方向，但属独立切片（涉及全站扫描 + 存量豁免清单），**不塞进本批**，见 §8 U4。

---

## 5. 交互与筛选（批 C）

### 5.1 行内 CTA：降级为「弹窗快捷入口」（**修正原审查的落法**）

**原审查说**「高风险卡应列表内主 CTA（批准 / 驳回入口）」，重构方向进一步要求「列表扫读 → 一键决断」。

**为什么不能照做**：当前弹窗不是确认框，它承载四类**不可跳过**的内容：

| 内容 | 代码 | 举例 |
|---|---|---|
| 决策框架（你在确认什么） | [inbox-action-dialog.tsx:586-616](../../../apps/web/src/features/inbox/components/inbox-action-dialog.tsx#L586) | project_acceptance：「这是项目级验收闸…同意后项目归档，不是单独再验收一次需求」 |
| 动作后果 | [:618-651](../../../apps/web/src/features/inbox/components/inbox-action-dialog.tsx#L618) | demand_acceptance 同意：「若其为项目最后一条终态需求，可能接着打开项目验收」 |
| 强制备注 | [:70,81](../../../apps/web/src/features/inbox/components/inbox-action-dialog.tsx#L70) + 服务端 [service.go:289](../../../apps/control-plane/internal/inbox/service.go#L289) | 所有 `rejected` / `needs_more_evidence` / `close_demand` / `cancel_downstream`，服务端缺备注直接拒 |
| 扩编选人 | [:75-81](../../../apps/web/src/features/inbox/components/inbox-action-dialog.tsx#L75) `castingReady` | `casting_expansion` 的 `approved` **必须**带 `digital_employee_id` + `role_key` |

逐个核对 [decision_action_registry.go:76-141](../../../apps/control-plane/internal/inbox/decision_action_registry.go#L76) 的全部动作后结论是：**没有任何一个动作满足「跳过弹窗直接提交」的安全条件**。免备注的动作（`approved`、`restaffed`、`exempted`、`retry_planning`、`retry`、planning_gap 的 `rejected`「关闭」）恰恰都是**影响面最大的那一批**——归档项目、关闭需求、重新规划。

**改法（真正省下的那一步）**：当前路径是 3 击 —— 选中行 → 右栏点动作 → 弹窗确认。行内 CTA 的价值在于**省掉「选中」这一跳**，而不是省掉弹窗：

1. 列表行在 `risk_level ∈ {blocked, high}` 且 `status === "open"` 且 `view === "mine"` 时，渲染该卡 `actions[]` 的**首个 positive 动作**为行内主按钮；
2. 点击 = `onSelect(item)` + 直接打开弹窗并预选该动作（等价于把现有 `onAction(item, action)` 提到行内），**不提交**；
3. 其余动作仍从详情栏进入；
4. 按 DESIGN.md「对象内的独立链接或按钮应保持自己的动作边界」，该按钮需 `stopPropagation`，与已有的「查看上下文」链接同处置。

**「查看上下文」的定位**：原审查要求降为次动作 —— 同意，但**不是删除**。它已经是弱视觉（小字链接，[inbox-item-list.tsx:210-217](../../../apps/web/src/features/inbox/components/inbox-item-list.tsx#L210)），新增主 CTA 后主次自然成立，无需额外改动。

**判据**：高风险 open 卡在列表内可一击打开决策弹窗（跳过选中步）；弹窗内容与从详情栏进入时**完全一致**；`casting_expansion` 卡的批准仍强制选人才能提交；任一 `requires_comment` 动作缺备注时前后端都拒。

### 5.2 筛选器去 UUID

**现状**：「更多筛选」展开后是两个裸 UUID 输入框 + 「请输入有效 UUID」错误（[inbox-shell.tsx:1260-1282](../../../apps/web/src/features/inbox/components/inbox-shell.tsx#L1260)），校验逻辑在 [index.tsx:301-312](../../../apps/web/src/features/inbox/index.tsx#L301)。要求用户手工粘贴 UUID 才能按项目筛选，与 CLAUDE.md「不得裸 UUID」的显示约定同源违和。

**改法**：

1. **项目**：换成 Popover + 搜索的选择器。[task-launch-form.tsx:271](../../../apps/web/src/features/task-launches/components/task-launch-form.tsx#L271) 已有 `ProjectPicker`，抽到共享位置复用。
   - **与同批 spec 的交接点**：`2026-08-07-task-hub-ux-remediation-design.md` §5.1 正在把 `ProjectPicker` 改造为服务端搜索。**两批不要各改一份**——实施顺序见 §9。
2. **目标用户**：换成成员选择器；且该筛选**只在「团队待办」视图有意义**（`view=mine` 时服务端已按当前用户收敛），改为随 `view` 显隐。
3. 保留 `filters` 状态形状与请求参数不变（服务端契约 [handler.go:312-318](../../../apps/control-plane/internal/inbox/handler.go#L312) 不动），只换输入控件；`uuidFilterDrafts` / `uuidFilterErrors` 及其校验随之删除。

**判据**：筛选区无任何需要手输 UUID 的输入；按项目筛选可通过搜索名称完成；「我的待办」视图下不出现目标用户筛选。

---

## 6. 明确不做（原审查两条）

### 6.1 `summary` 人类可读化：**前提不实**

原审查以线上一条英文 summary 为据，要求「planner 摘要需人类可读化或列表只展示 title+why」。

**核实**：`summary` 的来源是 `decision.SummarySnapshot`（[adapters.go:172](../../../apps/control-plane/internal/inbox/adapters.go#L172)），对 `plan_review` 而言就是 `input.Payload.Summary`（`project_store.go` 内 `Title: "确认项目计划版本"` 那处审批请求构造，行号见 §3.1 的并发漂移提示）——**用户提交需求时自己写的正文**。线上那句 `Demand for H10 planner role_key check with scenario software_delivery…` 是开发期 E2E 夹具留下的，不是系统生成的技术串。

同库另有中文 summary 佐证：「在父单基础上继续一小步，验证 stale 会话降级与卷宗中文留痕…」——同一字段、同一 kind，可读性完全取决于用户怎么写。

**结论**：按建议「可读化」等于改写用户原话。不做。§3.4 的 why/summary 去重已经解决了这里真实存在的那部分噪音。

### 6.2 关联引用 meta 中文化：**已完成**

原审查要求把 `demand_id ↗` 改为「打开需求」「打开任务」。该改动**已落在 main 最近一次提交 `4cb267cd`**：`relatedRefMetaLabel` 返回「需求 / 打开需求 / 任务 / 打开任务 / 打开项目 / 打开审批 / 审计」，词表注释即写着「禁止 API 字段名（demand_id ↗ 等）」（[status-labels.ts:609-631](../../../apps/web/src/lib/status-labels.ts#L609)）。

无需动作。实施会话若看到审查截图与代码不符，以代码为准。

---

## 7. 验证方案

本 spec 含 CP 读路径与提交链路相关改动，**不适用 CLAUDE.md 的轻量验证例外**。

### 7.1 分层门禁（提交前）

- `corepack pnpm verify:control-plane`（§3.1 改了 sqlc 查询与 repository）
- `corepack pnpm verify:web`
- `corepack pnpm verify:design-system`（§4 触碰布局基元用法）
- **不需要** `migrate-validate`：本批无迁移

### 7.2 受影响的既有测试

| 文件:行 | 断言 | 受哪项影响 |
|---|---|---|
| [index.test.tsx:273-274](../../../apps/web/src/features/inbox/index.test.tsx#L273) | `getByText("今日待处理摘要")` / `getByText("选择事项后可执行")` | §4.2（空态整块删除，**测试须改写为「未选中时不渲染详情栏」**） |
| [index.test.tsx:605-636](../../../apps/web/src/features/inbox/index.test.tsx#L605) | `getByLabelText("项目 ID")` / `("目标用户 ID")` + UUID 校验三例 | §5.2 |
| [index.test.tsx:924](../../../apps/web/src/features/inbox/index.test.tsx#L924) | `sections.find(key==="plan_review").label === "计划确认"` | §3.3（分组表头不变，应保持绿；若变红说明改过头） |
| [index.test.tsx:690,706,725](../../../apps/web/src/features/inbox/index.test.tsx#L690) | `getByRole("link", { name: "查看上下文" })` | §5.1（链接保留，回归确认不被新 CTA 挤掉） |
| [format-context.test.ts:26-48](../../../apps/web/src/features/inbox/format-context.test.ts#L26) | D3 项目分支 | §3.1（保持绿 + **补需求分支对称用例**） |
| `__screenshots__/index.test.tsx/*` | 21 张基线截图 | §4 全部需重拍 |
| [service_test.go:951-959](../../../apps/control-plane/internal/inbox/service_test.go#L951) | `enrichSourceNames` 项目名补名 / 悬垂时为 nil | §3.1（**补需求标题的对称用例，含悬垂需求返回 nil**） |

**另需新增测试**：需求补名读路径（Go）、`missingObjectLabel("demand")` 兜底（TS）、why/summary 同句去重（TS）、meta 节点无真实节点时不渲染（TS）、高风险行内 CTA 打开弹窗且不提交（TS）。

### 7.3 真实端到端（必做，浏览器 + 真实 CP + 真库）

> **执行记录（2026-08-09 23:29）：12/12 全过。** 实施提交 `361aa3ac`（08-08）当时**未执行本节**（其 CHANGELOG 只记了分层门禁），08-09 补跑并揪出 1 个回归：§4.1 迁移丢了定高管道，导致滚动后选中卡片时决策按钮被顶到视口上方 863px。已修（`MasterDetailLayout` 新增可选 `fill` + 收件箱外层 `overflow-hidden`），详见 CHANGELOG 2026-08-09 23:29 条。验证期内 `control-plane` pid=209 / `web` pid=804 未变且无 `owner=`。

前置：`scripts/dev-services.sh status` 确认在跑；改 CP 后 `restart control-plane`，改 Web 后 `restart web`。**造数用线上已存在的那 6 条 open 卡**，无需新造。

| # | 场景 | 通过判据 |
|---|---|---|
| G1 | 打开 `/inbox`，检查那 6 条 `demand_id`-only 卡 | 列表 meta / 详情「关联对象」/「关联引用」三处均为需求标题，页面全文 `rg` 不出现 36 位 UUID |
| G2 | 直改一条卡的 `context_payload.demand_id` 为不存在的 UUID，刷新 | 三处显示「未命名需求 (xxxxxxxx…)」，不崩、不裸 id |
| G3 | 查看 `project_task_runtime_recovery` 那 3 条 | meta 无英文枚举；说明文字只出现一段（why==summary 已去重） |
| G4 | 任一 `plan_review` 卡 | 分组表头「计划确认」在该行 meta 中不再重复；进度条 label 不变；详情面板「当前节点」仍有值 |
| G5 | 未选中任何事项 | 列表独占全宽；**无**中栏/右栏空态占位；无「今日待处理摘要 / 处理顺序建议 / 选择事项后可执行」 |
| G6 | 选中一条事项 | 宽容器下右栏 in-flow 展开（详情 + 动作合一）；窄容器下改为 Sheet 抽屉且可关闭并清除选中态 |
| G7 | 收起侧栏 / 改窗宽 | 布局折叠随**内容宽度**响应（容器断点生效），不因侧栏展开收起而错位 |
| G8 | KPI 四卡 | 0 值保持灰阶、>0 点亮语义色；换 `MetricCard` 后该规则未丢 |
| G9 | 高风险 open 卡行内 CTA | 一击打开弹窗（跳过选中步）；弹窗内容与从详情栏进入完全一致；**未提交任何决策** |
| G10 | `casting_expansion` 卡走完整批准 | 弹窗强制选角色 + 选员工才能提交；真实提交后卡片转终态、协调线程重规划 |
| G11 | 任一 `rejected` 动作不填备注提交 | 前端阻止；绕过前端直调 API 时服务端 400 |
| G12 | 按项目筛选 | 通过搜索项目名完成，无 UUID 输入；「我的待办」下无目标用户筛选，「团队待办」下有 |

**阻塞处置**：G10 需要一条真实处于扩编待决的需求。线上已有 3 条 `casting_expansion` open 卡可用；若批准后无法复现，**标记为阻塞并说明**，不得以单测替代。

### 7.4 收尾门禁

按 CLAUDE.md，收尾前走 `.codex/skills/superteam-completion-check/SKILL.md`（Claude Code 会话直接 `Read` 该文件并照步骤执行）。

---

## 8. 待确认项

| # | 事项 | 影响 | 建议处置 |
|---|---|---|---|
| **U1** | 详情 + 动作合并进一栏后的内部顺序：动作区置顶，还是「为什么需要你处理」置顶？ | §4.1 的 detail 栏结构 | 建议**动作置顶**（裁决工作台口径：选中即见可决断项）。出两版截图给人类选；不阻塞其余各项 |
| **U2** | 「等待时长」圆环（[:920-968](../../../apps/web/src/features/inbox/components/inbox-shell.tsx#L920)）在合并栏里的去留 | §4.1 信息密度 | 它是原右栏第一块、占位不小。建议压缩为详情头的一行 meta（复用 KPI 的「等待最久」口径），但**不得丢掉 open/终态两种算法的区分**（[:1153-1162](../../../apps/web/src/features/inbox/components/inbox-shell.tsx#L1153)） |
| **U3** | `ProjectPicker` 抽共享的落点与改造归属 | §5.2 与 task-hub spec §5.1 的交接 | 见 §9：**task-hub 批先做**，本批直接消费其成果。若两批并行，本批需等对方合入后再起 §5.2 |
| **U4** | 是否给 `verify:design-system` 增加「feature 代码禁手写主从/指标栅格」扫描 | 防止 §4 类违规再次漏网 | **不在本批**。值得独立立项（需全站扫描 + 存量豁免清单）。若人类要求，写入根目录 `TODO.md` |
| **U5** | §3.2 六个新词条的中文定名 | 用户可见文案 | 表中已给建议值；如与 `project_task_*` 家族在别处已有约定用词冲突，以既有约定为准 |

---

## 9. 实施顺序

```
批 A（§3）  ──┬──> 批 B（§4）
              │
task-hub §5.1 ─┴──> 批 C §5.2
              批 C §5.1 独立
```

1. **批 A（§3）** —— 显示层缺陷。§3.1 跨 CP + Web，先做（它是唯一的 P0，且改完立即可用线上 6 条卡验证）。§3.2–3.5 纯前端小改，同批。
2. **批 C §5.1（行内 CTA）** —— 与 A、B 均无依赖，可任意插入。
3. **批 B（§4）** —— 布局宪法整改。**放在批 A 之后**：A 改的是行内文本与 meta 结构，B 改的是外层容器；反序会让 A 的判据在 B 的截图重拍中被淹没。B 改动面最大，20 张基线截图重拍集中在这里。
4. **批 C §5.2（筛选器）** —— **依赖 task-hub spec §5.1**（`ProjectPicker` 服务端搜索改造）。对方未合入前不要起，否则会产生两份分叉的 Picker。

各批可独立提交、独立回滚。

---

## 10. 风险

| 风险 | 等级 | 缓解 |
|---|---|---|
| §3.1 读路径补名给列表查询增加一次 DB 往返 | 低 | 与既有两次补名并入同一个 `errgroup`（[service.go:164-185](../../../apps/control-plane/internal/inbox/service.go#L164)），并发执行不串行；id 集合已去重 |
| §3.1 就地改写 `ContextPayload` 可能被误当作持久化快照 | 中 | 与 `SourceProjectName` 同一语义，但那两个是**独立字段**、这个是**改 map**。必须在函数与字段处补注释写明「仅本次响应，不回写库」；并在 Go 测试中断言 repository 无写调用 |
| §4 三栏 → 两栏丢失信息 | 中 | 合并的是「详情」与「动作」两块，不删任何字段；U1/U2 先出截图确认 |
| §4 截图基线大面积重拍（21 张）掩盖真实回归 | 中 | 批 B 单独提交；重拍前先跑一次批 A 后的基线，两次 diff 分开审 |
| 共享工作树并发会话（`project_store.go` 正被他人改动） | 中 | 提交只用显式路径 `git add <path>`，禁 `git add -A`；提交前 `git symbolic-ref HEAD` 复核分支；本批不触碰 `project_store.go`，天然无交织 |
| §5.1 行内 CTA 被误实现为直接提交 | **高** | 判据 G9 明确要求「未提交任何决策」；实现上直接复用现有 `onAction(item, action)` 回调，不新增提交路径 |
| §5.2 与 task-hub 批并行导致 Picker 分叉 | 中 | §9 已排死顺序；实施前先确认对方分支状态 |
| §3.3 改 `formatCurrentNode` 波及详情面板「当前节点」显示为空 | 低 | 拆两个函数，详情面板保留 kind 回退；G4 判据显式覆盖 |
