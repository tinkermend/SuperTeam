# #1 一单工作台（Demand Workbench）

- 日期：2026-07-29
- 状态：**立项（未实施）——等人审阅后决定是否开工**
- 系列：对齐基线 `2026-07-27-workspace-and-playbook-alignment-baseline.md` §8 第 1 项
- 交付性质：CP **只读聚合 API**（OpenAPI 契约变更，无迁移、无写路径）+ Web 项目详情需求区升级为处所
- 目标读者：实施会话（本文自包含；实施前必读基线 §3/§4/§6/§7）
- 已拍板（2026-07-29）：
  1. **中栏主叙事 = 协调时间线**；权威流程图降为可切换副视图
  2. **新增 demand 级只读工作台 API**，不在前端拼装多接口
  3. **先审本 spec，再决定是否实施**

---

## 1. 背景与目标

### 1.1 问题

一单协作结束后（或中途），人要能回到「这一单」：做了什么、产出/验证如何、还需要我做什么、以及将来在此基础上继续。今天这些信息散在项目详情、执行轨迹、收件箱；`ProjectDemandsSection` 已有 demand 深链与流程图，但：

- 中栏是**图**，不是「这一单发生了什么」的叙事
- **没有右轨**承载剧本驱动的事实状态
- 没有**密度**（驱动态 / 巡检态）
- Web 为拼出一单要打 launch-detail + task-graph +（可选）acceptance-criteria + scenario-template，语义分散、补名不一致、难测

### 1.2 目标

1. 在**不新建第五观察面**的前提下，把项目详情需求视图升级为**一单处所**。
2. 提供 **demand 级只读工作台 API**：一次返回时间线、右轨槽位、待你处理、密度建议、剧本摘要；服务端批量补名。
3. Web 以时间线为主、图为辅；右轨按 `produces_defaults[].kind` 渲染；两种密度可区分。
4. 统一深链身份：`/projects/{projectId}?tab=demands&demand={demandId}`。
5. 为 #2 接续、#3 变更可见、#4 轻发起预留挂点，**本轮不实现**其写路径与真数据填充。

### 1.3 一句话

> **每一单在项目详情里有稳定处所；中栏讲故事，右轨讲事实，密度跟人是否需要驻留走，数据由 CP 只读聚合一次给齐。**

---

## 2. 非目标（本 spec 一律不做）

| 不做 | 归属 |
|---|---|
| 「继续此任务」真 API / 血缘写入 / recovery 丢血缘修复 / session 降级 | **#2** |
| git base/commit 后 diff、numstat 文件清单、attestation git 字段回填、`text/x-diff` 预览 | **#3** |
| 任务中枢轻发起、计划确认卡钉剧本/收口、Prompt 模板降权、项目允许剧本集 | **#4** |
| 批准物一等模型、执行层 action 白名单 | 基线 §7 待立项 |
| 逐行 diff review / side-by-side | 基线已否决 |
| 新建 `/workbench` 路由或菜单项 | 基线 §4.5 / §5 否决 |
| 改 Temporal coordinator、Runtime、Provider 管道 | 基线 §4.8 |
| 废弃 `GET .../launch-detail` 或 `GET .../task-graph`（可继续被 redirect / 图视图复用） | 本轮只**转移主消费方** |
| 写路径决策（批准/驳回）搬进工作台内联完成（可链到收件箱/既有就地卡） | 避免范围膨胀；待你处理以深链/摘要为主 |
| 前端 SSE 协议变更（沿用项目活动 SSE invalidate 即可） | — |

---

## 3. 现状地基（实施必读；行号会漂，以符号为准）

| 事实 | 锚点 |
|---|---|
| URL 身份已存在 | `?tab=demands&demand=` → `initialDemandId`；`project-detail-section.ts` |
| 需求区容器 | `apps/web/src/features/projects/components/project-demands-section.tsx`：左列表 + 状态头 + `FlowGraphCanvas` + `DemandCriteriaPanel` + 待决列表 |
| 旧流程深链兼容 | `apps/web/src/routes/_authenticated/workflows/$demandId.tsx` → launch-detail 解析 project 后 redirect |
| launch-detail 聚合 | `Service.GetDemandLaunchDetail`：demand/project/jobs/routes/tasks/summaries/decisions/events（**无** handoff、**无**模板、**无**时间线归一、**无**证据切片） |
| task-graph + handoff | `GetProjectTaskGraph?demand_id=` + `buildProjectTaskGraphHandoffAssessments`；deliverable kind 来自结果契约 / planner produces |
| 验收血缘 | `GET /api/v1/project-demands/{demandId}/acceptance-criteria` + `DemandCriteriaPanel`（**保留**，可收进右轨「验收」或中栏下方，不删能力） |
| 场景模板 | `GET /api/v1/scenario-templates/{templateKey}`；spec v2 `skeleton[].produces_defaults[{name,kind}]`；demand/project 均有 `scenario_template_key` |
| 证据/工件列表 | 项目级 `ListEvidence` / `ListArtifacts`，**无 demand 过滤**；工作台需在服务端按本单 task_id 集过滤 |
| 活动刷新 | `useProjectActivityInvalidate` 已接项目维度 SSE |
| 名称补名宪法 | 读路径批量补名；禁止裸 UUID 面向用户 |
| 中文枚举 | `apps/web/src/lib/status-labels.ts`；缺键补词表 |

**有效剧本 key 解析（本 spec 钉死）**：

```text
effective_scenario_template_key =
  demand.scenario_template_key
  ?? project.scenario_template_key
  ?? null   // null → 右轨仅按实际 handoff/产物推导，不假装 generic 剧本 UI 文案
```

---

## 4. 产品结构

### 4.1 壳（领域中性）

落在 `ProjectDemandsSection` 升级后的布局（宽屏）：

```text
┌────────────┬──────────────────────────────┬────────────────────┐
│ 左：本项目  │ 中：单头 + 密度区               │ 右：事实轨          │
│ 需求列表    │  - 待你处理（置顶条）            │  按 kind 分槽       │
│ （增强摘要） │  - 主：协调时间线               │  + 交付判定汇总     │
│            │  - 切：权威流程图               │  + 验收入口/摘要     │
│            │  - 任务点开 → 既有任务详情弹层   │                    │
└────────────┴──────────────────────────────┴────────────────────┘
```

窄屏：右轨叠到中栏下方或 Sheet；左列表可折叠为顶栏 demand 切换。遵循 `DESIGN.md` MasterDetail / SoftCard / WorkSurface，**禁止**手写固定 px 主从网格。

### 4.2 左轨：需求列表

在现有 Link 列表上增加（均来自 workbench list 摘要或父页 demands + workbench 轻字段）：

- 状态 pill（已有）
- **待你处理**角标（open decision 数 > 0）
- 剧本短名（有则显示；无则不显示「未选剧本」恐吓文案——automation 路径合法）
- 相对时间（已有）

列表本身可继续用父页 `demands[]`；角标若不想 N+1，见 §5.3 可选 `?include=sibling_summaries`（**P1 可砍**：先不加角标剧本名，只保留状态）。

### 4.3 单头（中上）

- 标题、demand 状态、有效剧本名（链到场景模板只读页可选）
- 模式：plan/loop/chat（若 demand/coordination 可判定；不可判定则省略）
- 主操作区：
  - 有 open decision → 主 CTA「去处理」深链收件箱或就地展开既有决策卡（**优先复用**项目详情已有计划确认就地能力，不重做审批 UI）
  - **「继续此任务」**：#1 只放 **disabled** 入口 + 中文说明「同单接续将在后续版本开放」；有多个可续任务时不展开真列表（避免假交互）
- 次操作：复制深链、在图/时间线间切换（切换控件也可放时间线标题旁）

### 4.4 中栏：时间线为主，图为辅

**默认视图 = `timeline`**。`graph` 为同一中栏的二次切换，状态进 URL 可选：

| 方案 | 选择 |
|---|---|
| URL | **推荐** `?tab=demands&demand=&view=timeline\|graph`（缺省 timeline）；无 view 当 timeline |
| 仅组件 state | 不推荐：刷新丢失、深链无法指到图 |

时间线条目（用户可见）必须：

- 中文标题（经 status-labels 或本 API 已渲染的 `title`）
- 相对时间 + 绝对时间 title
- 可选副文案（结论摘要、失败原因截断、处理人**显示名**）
- 可点击时打开任务详情弹层 / 决策深链（`target` 结构化，见 §5）

**不是**把 `recent_events` 原样刷日志；要做**协调叙事归一**（§5.2）。

流程图视图：继续复用 `FlowGraphCanvas` + 现有 `getProjectTaskGraph`（或 workbench 内嵌的 graph 引用）。**#1 不删除图能力**，只降权默认。

### 4.5 右轨：按 kind 插槽

槽位顺序：

1. 有效模板 skeleton 中 `produces_defaults[].kind` **去重保序**
2. 实际 handoff/证据里出现、但不在模板中的 kind **追加在后**
3. 无模板且无产物 → 单块空态：「本单尚未形成可展示的交付事实」

| kind | 槽标题（中文） | #1 数据来源 | #1 诚实边界 |
|---|---|---|---|
| `conclusion` | 结论 | handoff deliverables + execution_summaries 结论字段 | 无则 empty |
| `evidence_ref` | 证据 | 本单 task 关联 evidence_refs + handoff | 项目级列表服务端过滤 |
| `artifact_ref` | 工件 | 本单 task 关联 artifact_refs + handoff | 同上 |
| `branch_ref` | 分支 | handoff ref / 结果契约 | 有 ref 展示；无结构化变更清单 |
| `git_commit` | 提交 | 同上 | 同上；**不**承诺文件级 diff（#3） |
| 其他/未知 kind | 用 kind 原文作技术键，标题走词表或「交付物」 | handoff | 不发明领域文案 |

**交付判定汇总**（右轨顶部固定块）：

- 聚合本单全部 `handoff_assessments`：fulfilled / partial / unfulfilled / unknown 计数
- unknown **不得**渲染成失败；文案「暂无声明，无法判定」
- 明细可按任务名展开（任务显示名，禁止裸 UUID）

**验收**：保留 `DemandCriteriaPanel` 能力——#1 默认放在右轨底部折叠区或「验收」槽，避免中栏再堆第三叙事。若面板过重，巡检态只显示「N 条判据 · 待签 M」+ 展开。

### 4.6 两种密度

| 密度 | 布局 | 推导（服务端给 `density` + `density_reasons[]`，前端可覆盖仅用于调试） |
|---|---|---|
| **drive（驱动）** | 时间线默认展开；右轨常显；单头展示继续入口占位；待你处理置顶 | 任一：demand 状态 ∈ {submitted, planned, executing, acceptance_pending}；存在 status=open 的 decision；存在非终态 task |
| **inspect（巡检）** | 时间线默认折叠为「最近 3 条 + 展开全部」；突出结论 + 交付判定 + 待你处理（若仍有） | demand 终态 ∈ {completed, failed, cancelled} 且无 open decision 且无非终态 task |

自动化触发的已完成单 → inspect：无假聊天窗，但仍是稳定落脚点。

前端**不得**仅按「是不是软件交付」切换密度。

---

## 5. 只读 API 设计

### 5.1 端点

```http
GET /api/v1/project-demands/{demandId}/workbench
```

- **operationId**：`getProjectDemandWorkbench`
- **鉴权**：与 `getProjectDemandLaunchDetail` 同级（租户内项目读；沿用 demand 所属 project 的既有读权限，不新开 authz 关系，除非现网 launch-detail 已有更细检查——实施时对齐，禁止放宽）
- **404**：demand 不存在或不在租户
- **无**请求体；可选 query：

| query | 默认 | 说明 |
|---|---|---|
| `timeline_limit` | 80 | 上限 200；归一后截断，**不是** raw events 条数 |
| `include_graph` | `false` | `true` 时附带与 `GetProjectTaskGraph(demand_id)` 等价的 graph 载荷，便于单次请求；默认 false，图视图继续独立拉 graph 以控制体积 |

**不**做分页游标（P0）；超限在响应 `timeline.truncated=true`。

### 5.2 响应模型（契约级）

```yaml
ProjectDemandWorkbench:
  required: [demand, project, effective_playbook, density, density_reasons,
             pending_actions, timeline, rail, handoff_summary, actions]
  properties:
    demand: ProjectDemand          # 须含显示用字段；submitted_by 若暴露须已是可读信息或并列 display_name
    project:
      type: object
      required: [id, name]
      properties:
        id: { type: string, format: uuid }
        name: { type: string }
        status: { type: string }
        scenario_template_key: { type: string, nullable: true }
    effective_playbook:
      type: object
      required: [template_key, source]
      properties:
        template_key: { type: string, nullable: true }
        source: { type: string, enum: [demand, project, none] }
        name: { type: string }          # 无模板时 ""
        produce_kinds:                 # 去重保序
          type: array
          items: { type: string }
    density: { type: string, enum: [drive, inspect] }
    density_reasons:
      type: array
      items: { type: string }          # 机器码，如 open_decisions / active_tasks / demand_status_executing
    pending_actions:
      type: array
      items:
        type: object
        required: [id, kind, title, status]
        properties:
          id: { type: string, format: uuid }
          kind: { type: string }       # decision_type 原值
          title: { type: string }      # 中文友好，已是 snapshot 标题
          status: { type: string }
          created_at: { type: string, format: date-time }
          href:
            type: object               # 前端可路由，禁止只给模糊字符串
            properties:
              type: { type: string, enum: [inbox, project_demand, decision] }
              decision_id: { type: string, format: uuid }
              demand_id: { type: string, format: uuid }
              project_id: { type: string, format: uuid }
    timeline:
      type: object
      required: [items, truncated]
      properties:
        truncated: { type: boolean }
        items:
          type: array
          items:
            $ref: "#/components/schemas/ProjectDemandWorkbenchTimelineItem"
    rail:
      type: object
      required: [slots]
      properties:
        slots:
          type: array
          items:
            $ref: "#/components/schemas/ProjectDemandWorkbenchRailSlot"
    handoff_summary:
      type: object
      required: [fulfilled, partial, unfulfilled, unknown, assessments]
      properties:
        fulfilled: { type: integer }
        partial: { type: integer }
        unfulfilled: { type: integer }
        unknown: { type: integer }
        assessments:
          type: array
          items:
            allOf:
              - $ref: "#/components/schemas/ProjectTaskGraphHandoffAssessment"
              - type: object
                properties:
                  project_task_name: { type: string }  # 补名
    acceptance:
      description: 可选瘦摘要；明细仍可由既有 acceptance-criteria 端点拉取
      type: object
      properties:
        demand_status: { type: string }
        criteria_total: { type: integer }
        pending_human_judgment: { type: integer }
    actions:
      type: object
      required: [continue_task]
      properties:
        continue_task:
          type: object
          required: [available, reason_code]
          properties:
            available: { type: boolean }   # #1 恒 false
            reason_code: { type: string }  # not_implemented_yet
            reason_message: { type: string } # 中文
    graph:
      description: 仅 include_graph=true
      $ref: "#/components/schemas/ProjectTaskGraph"
    name_directory:
      description: 可选；本单涉及的员工/用户 id→display_name，供前端兜底
      type: object
      additionalProperties: { type: string }
```

```yaml
ProjectDemandWorkbenchTimelineItem:
  required: [id, occurred_at, kind, title]
  properties:
    id: { type: string }                 # 稳定幂等键：优选 event_id，否则 kind+entity_id
    occurred_at: { type: string, format: date-time }
    kind:
      type: string
      enum:
        - demand_submitted
        - coordination_started
        - plan_ready
        - plan_accepted
        - plan_rejected
        - plan_change_requested
        - task_created
        - task_dispatched
        - task_waiting_human
        - task_completed
        - task_failed
        - task_cancelled
        - result_recorded
        - result_accepted
        - result_rejected
        - decision_opened
        - decision_resolved
        - dispatch_blocked
        - staffing_gap
        - coordination_blocked
        - other
    title: { type: string }              # 已是中文主文案
    summary: { type: string }
    severity: { type: string, enum: [info, success, warn, danger, mute] }
    actor_display_name: { type: string }
    entity:
      type: object
      properties:
        type: { type: string, enum: [task, decision, demand, job, event] }
        id: { type: string }
        name: { type: string }
    open_target:
      type: object
      properties:
        type: { type: string, enum: [task_detail, decision, none] }
        task_id: { type: string, format: uuid }
        decision_id: { type: string, format: uuid }
```

```yaml
ProjectDemandWorkbenchRailSlot:
  required: [kind, title, items]
  properties:
    kind: { type: string }
    title: { type: string }              # 中文槽名
    items:
      type: array
      items:
        type: object
        required: [id, title, state]
        properties:
          id: { type: string }
          title: { type: string }
          summary: { type: string }
          state: { type: string, enum: [delivered, missing, unknown, info] }
          ref: { type: string }
          project_task_id: { type: string, format: uuid }
          project_task_name: { type: string }
          preview_hint:
            type: string
            description: >
              #1 对 branch_ref/git_commit 可给
              "变更文件清单将在后续版本提供"；不得假装已有 diff
```

### 5.3 服务端聚合步骤（实现顺序）

落点建议：`apps/control-plane/internal/project/service_workbench.go`（或 `GetDemandWorkbench` 旁路文件）+ handler + OpenAPI。

1. `GetProjectDemand` + `GetProject`；404 对齐 launch-detail  
2. 解析 `effective_playbook`：读场景模板活跃 spec（仓库内已有 template 服务/仓储则 **调用**，禁止 project 包复制解析逻辑）；失败时 `source=none` 降级，不 500  
3. 复用 launch-detail 同源列表：jobs / route_decisions / tasks / execution_summaries / decision_requests / events（可抽私有 `loadDemandLaunchFacts` 供两者调用，避免双分叉）  
4. 调既有 task-graph 构建（至少 tasks+handoff+blocking；`include_graph=false` 时可走瘦路径只算 handoff，**允许**内部调用 `GetProjectTaskGraph` 后丢 nodes 以换开发速度，但须在 PR 注明；理想是抽 `buildHandoffAssessmentsForDemand`）  
5. 按 task_id 集过滤 evidence/artifact（若仓储无 by-task API：**短期** List by project 再过滤，limit 要足够或加 SQL `WHERE project_task_id = ANY(...)`——**推荐**加 sqlc 查询，无 schema 迁移）  
6. 归一时间线（§5.4）  
7. 装 rail slots（§4.5）  
8. 算 density  
9. 批量补名：任务 title、员工、用户、模板 name  
10. `actions.continue_task = {available:false, reason_code:"not_implemented_yet", reason_message:"同单接续将在后续版本开放"}`

**事务**：只读；无写锁。

### 5.4 时间线归一规则

原则：

- **时间倒序**（新在上），主时间 = 事件 `created_at` / 实体关键时间  
- 同一业务事实不双计：若同时有 `project_task.completed` 事件与 task 状态，**优先事件流**；无事件时用实体状态回填一条（`id` 用 `synthetic:task_completed:{task_id}`）  
- `other` 仅兜底；实施时把高频 `ProjectEventType` 映射表写进代码常量，Web **不再**解析 raw event_type  
- `title` 示例：
  - `task_completed` →「任务完成 · {task_name}」
  - `decision_opened` + plan_review →「待计划确认」
  - `dispatch_blocked` →「派发受阻 · {短原因}」
- 映射表必须有单测：每种 kind 至少一条；未知 event_type → `other` + 不暴露英文蛇形原串给 `title`（可用「协调更新」）

**事件噪音**：下列默认 **不**进时间线（可进 debug 日志）：纯 `workflow.signaled`、高频 gate.checked 成功、workspace 心跳类（若混在 demand 事件里）。blocking / gap / failed / waiting_human / decision / 任务终态 **必须**进。

### 5.5 与既有端点关系

| 端点 | #1 之后 |
|---|---|
| `GET .../workbench` | **工作台主读模型** |
| `GET .../launch-detail` | 保留；workflows redirect 可改为打 workbench 或仍用 launch-detail（最小改：仍 launch-detail 只取 `project.id`） |
| `GET .../task-graph` | 图视图默认继续用；`include_graph=true` 可选合并 |
| `GET .../acceptance-criteria` | 右轨展开验收明细时用；workbench 只带计数摘要 |

### 5.6 契约与生成

1. 改 `contracts/control-plane/openapi.yaml`  
2. `corepack pnpm generate:control-plane`  
3. `corepack pnpm verify:contracts`（及 `verify:control-plane` 相关）  
4. handler 注册于 project routes 旁，路径前缀与 launch-detail 一致  

---

## 6. Web 实现

### 6.1 文件落点（建议）

| 路径 | 职责 |
|---|---|
| `apps/web/src/lib/api/projects.ts` | `getProjectDemandWorkbench` + 类型 |
| `apps/web/src/features/projects/components/project-demands-section.tsx` | 容器改版：接 workbench query；中栏 view 切换 |
| `.../demand-workbench-header.tsx` | 单头 + 密度 + 动作 |
| `.../demand-workbench-timeline.tsx` | 时间线 |
| `.../demand-workbench-rail.tsx` | 右轨 + handoff 汇总 |
| `.../demand-workbench-density.ts` | 若需前端兜底（原则上信服务端 density） |
| `apps/web/src/lib/status-labels.ts` | timeline kind / rail kind / density 中文 |
| 测试 | `project-demands-section.test.tsx` 扩展 + 新组件单测；API client 单测 |

### 6.2 数据活性

- queryKey：`["demand-workbench", apiBaseUrl, demandId]`  
- 继续 `useProjectActivityInvalidate`：invalidate 上述 key + 既有 `project-task-graph` / `workflow-detail`  
- `keepPreviousData` 切 demand 不闪空  
- 图视图独立 query 可保留

### 6.3 深链对账（#1 必做清单）

| 来源 | 期望 |
|---|---|
| 需求列表 Link | 已是 canonical — 回归 |
| `/workflows/:demandId` | 仍 redirect 到 canonical — 回归 |
| 收件箱 human task | 若 `primary_surface` / `deep_link.route` 能解析到 demand，应落到 canonical；**实施时盘点** inbox 构造处，能改的改为 project demand URL，不能改的在 Web 解析兼容 |
| 飞书卡片 | 历史 `/workflows/{id}` 靠 redirect；新发卡若改 URL 属飞书通道范围，**#1 不强制改 connector**，只保证 redirect |
| 运行总览 / 任务行 | 若有「所属需求」链，统一 canonical |

验收时列一张表：入口 → 最终 URL → 是否选中正确 demand → 默认 timeline。

### 6.4 文案与空态

- 时间线空：规划未开始 →「协调尚未产生可展示节点」  
- 右轨空：见 §4.5  
- workbench 500/403：SoftCard 错误空态 + 重试，不回退到半残旧布局硬撑  
- 禁用「继续此任务」：`title`/`aria-disabled` + reason_message  

### 6.5 设计约束

- 实施前重读 `DESIGN.md` 相关页型与主从布局  
- 用户可见枚举走 `status-labels.ts`  
- 变更后 `corepack pnpm verify:web`；涉及设计系统则按其规则  

---

## 7. 分期（若审阅后一次做不完）

| 切片 | 内容 | 可独立验收 |
|---|---|---|
| **P0a** | OpenAPI + `GetDemandWorkbench` + 单测（时间线映射/补名/density/rail） | curl 真库一条 demand |
| **P0b** | Web 容器改版：单头 + 时间线默认 + 右轨 + density；图切换 | 浏览器真链路 |
| **P0c** | 深链对账 + 验收面板归位 + 禁用接续挂点 | 入口表全绿 |

**默认交付 = P0a+P0b+P0c 全做完**才算 #1 完成。若排期砍刀：不得只做 API 无 UI，也不得只做 UI 假数据。

---

## 8. 验收 GATE（真实 E2E）

环境：当前代码的 Web + CP + DB；项目活动 SSE 可用更佳。

| ID | 步骤 | 期望 |
|---|---|---|
| G1 | `GET /api/v1/project-demands/{id}/workbench` 对真实 running demand | 200；`timeline.items` 非空或诚实空；无裸 UUID 在 title/actor/task_name；`density` 有值 |
| G2 | 对已完成 automation/人工单 | `density=inspect`；右轨/判定可读 |
| G3 | 对有 open plan_review 的 demand | `pending_actions` ≥ 1；`density=drive` |
| G4 | 浏览器打开 canonical URL | 进入项目详情需求处所；**默认时间线**；左列表高亮正确 demand |
| G5 | 切换「流程图」 | 图可点任务开弹层；再切回时间线状态保持（URL view） |
| G6 | 右轨 | 有模板时槽按 produce_kinds；handoff unknown 不显示为失败 |
| G7 | `/workflows/{demandId}` | redirect 到 canonical 且 workbench 正常 |
| G8 | 「继续此任务」 | 可见但不可用，中文说明存在 |
| G9 | 项目 SSE 或任务状态变化后 | workbench 刷新（invalidate）后时间线/待处理更新 |
| G10 | `verify:contracts` + 定向 CP 测 + `verify:web` | 通过 |

**完成定义**：G1–G10 全过；基线 §4 不变量无破坏；本 spec 非目标未偷做。

---

## 9. 风险与缓解

| 风险 | 缓解 |
|---|---|
| 时间线映射不全 → 用户觉得「丢事件」 | 映射表单测 + `other` 兜底不露英文蛇形；上线前用 2–3 条真实 demand 抽查 |
| `include_graph=false` 仍内部建全图 → 延迟 | 允许 P0 先全量复用；慢则加瘦 handoff 路径（记 TODO） |
| 项目级 evidence list 过滤不全 | 优先 sqlc by task_ids；单测覆盖「别的 demand 的证据不出现」 |
| 与 launch-detail 双维护 | 抽取 `loadDemandLaunchFacts` |
| 前端把 workbench 当可写 | 契约与 UI 均无写字段；接续 available=false |
| 范围膨胀做内联审批 | pending 只深链/复用既有卡 |
| 共享工作树 git 踩踏 | 显式路径 add；不 `git add -A` |

---

## 10. 开放细节（不阻塞立项；实施默认值）

1. `view` query 名：`view=timeline|graph`（默认 timeline）。  
2. 左列表剧本名/待办角标：P0 **做 pending 角标**；剧本名有 cost 则 P0 可省略。  
3. acceptance 摘要：workbench 带计数；点开再拉明细。  
4. `name_directory`：若各实体已内联 display_name 可省略。  
5. chat 模式 demand：同样进处所；时间线 kind 仍通用；不强制剧本槽。  

---

## 11. 文档关系

| 文档 | 关系 |
|---|---|
| `2026-07-27-workspace-and-playbook-alignment-baseline.md` | **基线**；本 spec 是其 §8 #1 |
| `2026-07-27-expert-playbook-launch-and-continue-design.md` | 已 superseded；**不**指导本实施 |
| 后续 #2 / #3 / #4 | 挂本处所：接续动作激活、rail git 真数据、发起后落地 URL |

审阅通过并实施完成后：将本文状态改为「已实施」，并在基线 §8 链到本文路径（若尚未链接）。

---

## 12. 审阅检查清单（给人）

- [ ] 同意默认中栏 = 时间线，图为 `view=graph`
- [ ] 同意新端点 `GET /project-demands/{id}/workbench` 字段粒度（是否要砍 `include_graph` / `acceptance` 摘要）
- [ ] 同意 #1 接续仅 disabled 挂点
- [ ] 同意验收面板进右轨、不在中栏与时间线并列为第三主叙事
- [ ] 同意 P0 必须 API+UI 真链路，不接受半边交付

---

## 13. 一句话方案

> **新增 demand 工作台只读聚合；项目详情需求区升级为左单列表、中时间线（图可切）、右剧本 kind 事实轨；密度服务端判定；深链统一；接续与变更真能力留给 #2/#3。**
