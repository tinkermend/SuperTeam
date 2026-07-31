# #1 一单卷宗（Demand Dossier）

- 日期：2026-07-29（**2026-07-31 修订 R2：范围收窄 + 地基核对**）
- 状态：**已实施（2026-07-31，R2 范围）**——GATE G1–G12 真实 E2E 全过（唯一例外见文末）
- 系列：对齐基线 `2026-07-27-workspace-and-playbook-alignment-baseline.md` §8 第 1 项
- 交付性质：CP **只读聚合 API**（OpenAPI 契约变更）+ **1 条 sqlc 查询（无迁移，索引已存在）** + Web 项目详情需求区升级
- 目标读者：实施会话（本文自包含；实施前必读基线 §3/§4/§6/§7）

---

## 0. R2 修订记录（相对 07-29 立项版）

R2 的动机：立项版有三处**成本假设错误**（都是低估反面：以为要造的东西已经有了）和四处**范围虚胖**。逐条核对代码后重写。

### 0.1 砍掉的（4 项）

| 砍 | 理由 |
|---|---|
| **disabled「继续此任务」入口** + `actions.continue_task` 字段 | #2 未排期时这是一颗承诺型死按钮。容器属性靠**契约与组件边界**留，不靠 UI 占位留。#2 开工时加一个按钮的成本 ≈ 0。 |
| **`include_graph=true`** | 图视图已有独立 query 且与页面预载共享缓存 key（`["project-task-graph", projectId, demandId]`，见 `project-demands-section.tsx` / `features/projects/index.tsx`）。合并只增体积与第二条码路。 |
| **服务端硬判 `density`** | 密度是**用户偏好**，不是服务端事实。硬判必然回头补「用户覆盖 + 持久化」。改为：服务端只回 `signals`，密度由前端算并允许用户切换（记 localStorage）。 |
| **`name_directory`** | 立项版 §10.4 自己就说实体内联 display_name 即可省。省。补名一律内联到各实体字段。 |

### 0.2 加进来的（3 项，均为落地必需）

| 加 | 理由 |
|---|---|
| **项目级事件文案复用同一张归一表** | `project-ops-home.tsx` 的 `opsEventTitle` 只映射 8 个事件类型，default 分支 `return { title: event.event_type }`——**现网正在把 `project_task.dispatch_gate.replan_required` 这类英文蛇形串直接吐给用户**，违反中文优先宪法。而 `ProjectEventType` 常量有 ~60 个。归一表放服务端后，项目级也必须改读它，否则同一事件三处三种说法。 |
| **时间线诚实边界（叙事 ≠ 审计）** | 事件是尽力而为写入的（dispatch conflict 曾静默丢 ledger 有前科），~60 类型映射到 ~20 kind 必有落 `other` 的。必须显式标注并给「查看全部执行事件」出口（深链已存在的 `?tab=trace`）。 |
| **evidence by-task sqlc 查询（硬性）** | 立项版 §5.3-5 允许「短期 List by project 再过滤」。**拒绝**：证据多的项目会被 limit 静默截断，右轨于是把系统正常状态渲染成「交付缺失」——假阴性比不显示更坏。 |

### 0.3 更名（1 项，必须先做）

**不叫「工作台」。** `ProjectDetailSection` 的默认区段名就是 `"workbench"`（`features/projects/lib/project-detail-section.ts`），承载 2654 行的 `ProjectOperationalDetail`；`?tab=overview` 也已被 legacy 映射到它。再造一个 demand 级「工作台」= 同页两个工作台，用户混、代码更混。

本 spec 全文改用 **dossier（卷宗）**：

- 端点 `GET /api/v1/project-demands/{demandId}/dossier`
- Go `Service.GetDemandDossier` / `DemandDossier`
- Web `demand-dossier-*.tsx`
- **面向用户的中文一律叫「这一单」**，不暴露 dossier 字样

已确认 `dossier` 在仓库中零占用（`rg -ril dossier` 无命中）。

### 0.4 地基核对结论（**推翻立项版的成本估计**）

| 立项版假设 | 实际 | 影响 |
|---|---|---|
| handoff 判定要么建全图、要么抽新函数 | `buildProjectTaskGraphHandoffAssessments(tasks, latestContracts)` **已是纯函数**（`task_graph_handoff.go`），仓储层调用点在 `pg_repository.go` 的 `GetProjectTaskGraph` 内 | 瘦路径直接可走，**不需要**「先建全图再丢 nodes」的丑陋兜底 |
| 工件按任务过滤要新查询 | `Repository.ListDeclaredArtifactsByTaskIDs(ctx, tenantID, projectID, taskIDs)` **已存在**（`repository.go`，验收面板 v2 §4 P2 用） | 工件槽零新增查询 |
| 证据按任务过滤可能要迁移 | `project_evidence_refs.project_task_id` 列存在，且 `idx_project_evidence_refs_tenant_task` 索引**已存在**（迁移 015） | 只需 1 条 sqlc 查询，**无迁移**，性能有索引兜底 |
| 剧本解析要新端口或跨包复制 | `project.ScenarioTemplateResolver` 端口**已存在**（`SetScenarioTemplateResolver`），adapter 在 `app.go` 的 `scenarioTemplateResolverAdapter` | 只需给端口加**一个方法**返回 produce kinds；`ScenarioTemplateBinding` 现有 `{Key,Name,Status}` 无 spec |
| 时间线事件条数受限要改仓储签名 | `ListDemandLaunchEvents(..., limit int32)` **已有 limit 参数**，两个调用点只是都硬编码传 50 | 传 200 即可，**无签名变更** |
| 加 `?view=` 要改路由 schema | `features/projects/index.tsx` 用 `useSearch({ strict: false })` 松散读取 | 加一个可选字段，一行 |

净效果：**P0a 的服务端工作量比立项版估计低约一半**，真正的新增只有「时间线归一表」+「rail 组装」+「1 条 sqlc」+「端口加一法」。

---

## 1. 背景与目标

### 1.1 问题（已按现状核对，删去被夸大的三条）

一单协作结束后（或中途），人要能回到「这一单」：做了什么、产出/验证如何、还需要我做什么。今天 `ProjectDemandsSection` 给的是：状态头 + 阻塞横幅 + 权威流程图 + 验收血缘 + 待决列表——**全是状态快照**。三条真缺口：

1. **单级叙事不存在。** 想看「先后发生了什么」只能去 `ProjectExecutionTracePanel`，而它是**项目级、不按 demand 过滤**的。
2. **交付事实藏在图里且散。** handoff 判定（fulfilled/partial/unfulfilled/unknown）已实现，但只活在 `flow-handoff-overlay.tsx` 的节点浮层里——**必须点开某个节点才看得到**；工件在 `project-assets-panel` 的项目级 tab，**无 demand 维度**。「这一单交出了什么」目前无一处可一眼答。
3. **中文宪法现网违规。** 见 §0.2 的 `opsEventTitle`。

> 立项版还列了「深链不统一」「Web 打多个接口」「没有密度」——核对后：canonical URL 与 `/workflows/:demandId` redirect 已在跑；Web 只有 2 个 query 不是 4 个；密度是 spec 自造概念不是用户问题。这三条在 R2 里**降级为附带收益**，不作为立项理由。

### 1.2 目标

1. 项目详情需求区从「状态快照」升级为**单级叙事 + 交付事实汇总**。
2. 提供 demand 级只读 dossier API：一次给齐时间线、事实轨、待你处理、剧本摘要、密度信号；**服务端批量补名**。
3. 事件→中文归一表**建在服务端并被项目级复用**，堵掉英文串泄漏。
4. 为 #2/#3/#4 预留挂点——**只在契约与组件边界预留，不在 UI 放死按钮**。

### 1.3 一句话

> **每一单有稳定处所：中栏讲发生了什么（诚实标注为叙事非审计），右轨讲交出了什么，数据由 CP 只读聚合一次给齐、服务端补名。**

---

## 2. 非目标（一律不做）

| 不做 | 归属 |
|---|---|
| 「继续此任务」**任何形态的 UI**（含 disabled 占位）、真 API、血缘写入 | **#2** |
| git base/commit 后 diff、numstat 文件清单、attestation git 字段回填 | **#3** |
| 任务中枢轻发起、计划确认卡钉剧本、项目允许剧本集 | **#4** |
| 批准物一等模型、执行层 action 白名单 | 基线 §7 待立项 |
| 逐行 diff review / side-by-side | 基线已否决 |
| 新建 `/workbench` 路由或菜单项 | 基线 §4.5 / §5 否决 |
| 改 Temporal coordinator、Runtime、Provider 管道 | 基线 §4.8 |
| 废弃 `launch-detail` / `task-graph` | 只转移主消费方 |
| 内联审批写路径（批准/驳回） | 待你处理只做深链 + 复用既有就地卡 |
| 前端 SSE 协议变更 | 沿用项目活动 SSE invalidate |
| **数据库迁移** | R2 已确认零迁移需求（§0.4） |

---

## 3. 现状地基（实施必读；以符号为准，行号会漂）

| 事实 | 锚点 |
|---|---|
| URL 身份已存在 | `?tab=demands&demand=` → `normalizeProjectDetailSection`；`features/projects/lib/project-detail-section.ts` |
| 需求区容器 | `features/projects/components/project-demands-section.tsx`（598 行）：左列表 + 状态头 + 阻塞横幅 + `FlowGraphCanvas` + `DemandCriteriaPanel` + 待决列表 |
| search 参数松散读取 | `features/projects/index.tsx` 的 `useSearch({ strict: false })`，现有 `demand` / `tab` / `task` |
| 旧流程深链兼容 | `routes/_authenticated/workflows/$demandId.tsx` → launch-detail 解析 project 后 redirect |
| launch-detail 聚合 | `Service.GetDemandLaunchDetail` → `DemandLaunchDetail{Demand,Project,Reviewer,CoordinationJobs,RouteDecisions,ProjectTasks,ExecutionSummaries,DecisionRequests,RecentEvents}`；events 走 `ListDemandLaunchEvents(..., limit)`（现硬编码 50） |
| handoff 判定纯函数 | `buildProjectTaskGraphHandoffAssessments(tasks []ProjectTask, latestContracts map[uuid.UUID]*TaskResultContract) []ProjectTaskGraphHandoffAssessment`（`task_graph_handoff.go`）；契约取数在 `PgRepository.projectTaskGraphLatestResultContracts` |
| handoff 结构 | `ProjectTaskGraphHandoffAssessment{ProjectTaskID, Status, Deliverables}`；`ProjectTaskGraphHandoffDeliverable{Name, Kind, Verdict, Ref, Summary}`（`task_graph_types.go`） |
| 工件按任务 | `Repository.ListDeclaredArtifactsByTaskIDs`（已存在） |
| 证据按任务 | **缺**读查询；列 `project_evidence_refs.project_task_id` + 索引 `idx_project_evidence_refs_tenant_task` 已存在（迁移 015） |
| 剧本端口 | `project.ScenarioTemplateResolver{ResolveScenarioTemplate → ScenarioTemplateBinding{Key,Name,Status}}`；adapter `scenarioTemplateResolverAdapter`（`app.go`）；模板服务 `scenariotemplate.Service.GetByKey` 返回 `Spec map[string]any` + `ActiveVersion` |
| 剧本 spec 解析 | `scenariotemplate/spec.go`：`SpecStep.ProducesDefaults []SpecProduce{Name,Kind}`，已有 map→struct 解析器 |
| demand 剧本键 | `ProjectDemand.ScenarioTemplateKey *string`（nil 回落项目） |
| 事件类型 | `ProjectEventType` ~60 个常量（`types.go`）；`ProjectEvent{ID,SequenceNumber,EventType,ActorType,ActorID,ResourceType,ResourceID,Summary,Payload,CreatedAt}` |
| 项目级事件文案（**待修**） | `project-ops-home.tsx` 的 `opsEventTitle`：8 分支 + default 吐 `event.event_type` |
| 活动刷新 | `useProjectActivityInvalidate` 已接项目维度 SSE；demands 区另有 30s 兜底轮询 |
| 中文枚举 | `lib/status-labels.ts`（453 行，~29 个 `*Label` 导出）；缺键补词表 |

**有效剧本 key 解析（钉死）**：

```text
effective_playbook.template_key =
  demand.scenario_template_key            → source=demand
  ?? project.scenario_template_key        → source=project
  ?? null                                 → source=none
```

`source=none` 时右轨**只按实际 handoff/产物推导**，不假装 generic 剧本 UI 文案。resolver 未接线（nil）或解析失败时同样降级 `source=none`，**不 500**。

---

## 4. 产品结构

### 4.1 壳

```text
┌────────────┬──────────────────────────────┬────────────────────┐
│ 左：本项目  │ 中：单头 + 密度区               │ 右：事实轨          │
│ 需求列表    │  - 待你处理（置顶条）            │  交付判定汇总（顶）  │
│ （+待办角标）│  - 主：协调时间线（叙事标注）     │  按 kind 分槽       │
│            │  - 切：权威流程图               │  验收摘要（底，可展开）│
│            │  - 任务点开 → 既有任务详情弹层   │                    │
└────────────┴──────────────────────────────┴────────────────────┘
```

窄屏：右轨叠到中栏下方；左列表折叠为顶栏 demand 切换。遵循 `DESIGN.md` MasterDetail / SoftCard / WorkSurface，**禁止**手写固定 px 主从网格（现有 `lg:grid-cols-[260px_minmax(0,1fr)]` 若不合规一并对齐）。

### 4.2 左轨：需求列表

在现有 Link 列表上只加 **待你处理角标**（该 demand 的 open decision 数 > 0）。数据来自父页已有 demands + 一个轻量计数（见 §5.1 `sibling_pending`）。

剧本短名 **P0 不加**（立项版 §10.2 已允许省）。

### 4.3 单头

- 标题、demand 状态、有效剧本名（`source=none` 时不显示，**不显示「未选剧本」恐吓文案**——automation 路径合法）
- 待你处理：有 open decision → 主 CTA「去处理」，深链收件箱或就地展开**既有**计划确认卡（复用，不重做审批 UI）
- 次操作：复制深链、时间线/流程图切换、密度切换（见 §4.6）
- **无「继续此任务」任何形态**

### 4.4 中栏：时间线为主，图为辅

默认视图 = `timeline`；`?view=graph` 切图（`useSearch` 松散读取，缺省当 timeline）。

时间线条目必须：

- 中文标题（服务端已渲染的 `title`，前端不解析 raw event_type）
- 相对时间 + 绝对时间 title
- 可选副文案（结论摘要、失败原因截断、处理人**显示名**）
- 可点击时打开任务详情弹层 / 决策深链（`open_target` 结构化）

**诚实边界（R2 新增，必做）**：时间线区标题旁固定一行浅色说明——

> 「协调叙事视图，按关键节点归纳；完整执行事件见[执行轨迹]」

「执行轨迹」链到 `?tab=trace&task=`（已存在）。**不得**让用户以为时间线是完整审计流水。

流程图视图：继续复用 `FlowGraphCanvas` + 现有 `project-task-graph` query。**不删图能力**，只降权默认。

### 4.5 右轨：按 kind 插槽

槽位顺序：

1. 有效模板 skeleton 中 `produces_defaults[].kind` **去重保序**
2. 实际 handoff/证据里出现、但不在模板中的 kind **追加在后**
3. 无模板且无产物 → 单块空态：「本单尚未形成可展示的交付事实」

| kind | 槽标题 | 数据来源 | 诚实边界 |
|---|---|---|---|
| `conclusion` | 结论 | handoff deliverables + `ExecutionSummary` 结论字段 | 无则 empty |
| `evidence_ref` | 证据 | **新** sqlc by task_ids + handoff | 见 §5.3-5 |
| `artifact_ref` | 工件 | `ListDeclaredArtifactsByTaskIDs` + handoff | 已有查询 |
| `branch_ref` | 分支 | handoff `Ref` | 有 ref 展示；**无**结构化变更清单 |
| `git_commit` | 提交 | handoff `Ref` | 同上；**不**承诺文件级 diff（#3） |
| 未知 kind | kind 原文作技术键，标题走词表，缺键回落「交付物」 | handoff | 不发明领域文案 |

**交付判定汇总**（右轨顶部固定块）：

- 聚合本单全部 `handoff_assessments`：fulfilled / partial / unfulfilled / unknown 计数
- **unknown 不得渲染成失败**；文案「暂无声明，无法判定」
- 明细按任务名展开（显示名，禁止裸 UUID）

**验收**：保留 `DemandCriteriaPanel`，放右轨底部折叠区；折叠态显示「N 条判据 · 待签 M」（来自 dossier 的 `acceptance` 瘦摘要），展开时才拉既有 `acceptance-criteria` 端点明细。

### 4.6 密度（R2 改为前端决策）

服务端**只给信号**，不给结论：

```yaml
signals:
  has_open_decisions: bool
  active_task_count: int
  demand_terminal: bool
```

前端默认推导（`demand-dossier-density.ts`，可单测）：

```text
drive   ← has_open_decisions || active_task_count > 0 || !demand_terminal
inspect ← 否则
```

- **drive**：时间线默认展开；右轨常显；待你处理置顶
- **inspect**：时间线折叠为「最近 3 条 + 展开全部」；突出结论 + 交付判定

**用户可手动切换，选择记 localStorage（键含 demandId 前缀的全局偏好，不是 per-demand）**。前端不得仅按「是不是软件交付」切密度。

---

## 5. 只读 API 设计

### 5.1 端点

```http
GET /api/v1/project-demands/{demandId}/dossier
```

- **operationId**：`getProjectDemandDossier`
- **鉴权**：与 `getProjectDemandLaunchDetail` **完全同级**（实施时对齐其现有检查，禁止放宽）
- **404**：demand 不存在或不在租户
- query：

| query | 默认 | 说明 |
|---|---|---|
| `timeline_limit` | 60 | 上限 200；**归一后**截断，不是 raw events 条数 |
| `sibling_pending` | `false` | `true` 时附带同项目各 demand 的 open decision 计数（左轨角标用），形如 `[{demand_id, open_decisions}]`；默认关，避免每次切单都算 |

**不**做分页游标；超限在 `timeline.truncated=true`。

**已砍**：`include_graph`（见 §0.1）。

### 5.2 响应模型

```yaml
ProjectDemandDossier:
  required: [demand, project, effective_playbook, signals,
             pending_actions, timeline, rail, handoff_summary]
  properties:
    demand: ProjectDemand              # 复用既有 schema；submitted_by 须并列 display_name
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
        name: { type: string }         # source=none 时 ""
        produce_kinds:                 # 去重保序；source=none 时 []
          type: array
          items: { type: string }
    signals:
      type: object
      required: [has_open_decisions, active_task_count, demand_terminal]
      properties:
        has_open_decisions: { type: boolean }
        active_task_count: { type: integer }
        demand_terminal: { type: boolean }
    pending_actions:
      type: array
      items:
        type: object
        required: [id, kind, title, status]
        properties:
          id: { type: string, format: uuid }
          kind: { type: string }       # decision_type 原值
          title: { type: string }      # 中文，已渲染
          status: { type: string }
          created_at: { type: string, format: date-time }
          href:
            type: object
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
          items: { $ref: "#/components/schemas/ProjectDemandDossierTimelineItem" }
    rail:
      type: object
      required: [slots]
      properties:
        slots:
          type: array
          items: { $ref: "#/components/schemas/ProjectDemandDossierRailSlot" }
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
                  project_task_name: { type: string }   # 补名
    acceptance:
      description: 瘦摘要；明细仍由既有 acceptance-criteria 端点拉取
      type: object
      properties:
        demand_status: { type: string }
        criteria_total: { type: integer }
        pending_human_judgment: { type: integer }
    sibling_pending:
      description: 仅 sibling_pending=true
      type: array
      items:
        type: object
        properties:
          demand_id: { type: string, format: uuid }
          open_decisions: { type: integer }
```

**已砍**：`density` / `density_reasons`（→ `signals`）、`actions.continue_task`、`graph`、`name_directory`。

```yaml
ProjectDemandDossierTimelineItem:
  required: [id, occurred_at, kind, title]
  properties:
    id: { type: string }        # 幂等键：event_id，或 "synthetic:{kind}:{entity_id}"
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
    title: { type: string }     # 中文主文案
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
ProjectDemandDossierRailSlot:
  required: [kind, title, items]
  properties:
    kind: { type: string }
    title: { type: string }
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
```

**已砍**：`preview_hint`（「变更文件清单将在后续版本提供」属于对 #3 的 UI 承诺，同 §0.1 理由删除；分支/提交槽只展示 ref）。

### 5.3 服务端实现步骤（按此顺序，落点 `internal/project/service_dossier.go`）

1. **抽 `loadDemandLaunchFacts`（必做，非「可」）**——把 `GetDemandLaunchDetail` 现有的 demand/project/jobs/routes/tasks/summaries/decisions/events 取数抽成私有方法，`GetDemandLaunchDetail` 与 `GetDemandDossier` **同调一处**。events limit 参数化（dossier 传 `min(timeline_limit*3, 200)`，launch-detail 保持 50）。
   > 不抽就是第三份 demand 聚合逻辑，必然漂移。

2. **`effective_playbook`**：扩展既有端口——

   ```go
   // project 包
   type ScenarioTemplateResolver interface {
       ResolveScenarioTemplate(ctx, tenantID, key) (ScenarioTemplateBinding, error)
       // R2 新增
       ResolveScenarioTemplateProduceKinds(ctx, tenantID, key) ([]string, error)
   }
   ```

   adapter（`app.go` 的 `scenarioTemplateResolverAdapter`）用 `scenariotemplate.Service.GetByKey` + `scenariotemplate/spec.go` 既有解析器取 `skeleton[].produces_defaults[].Kind`，去重保序返回。
   **禁止**在 project 包内复制 spec 解析逻辑。resolver == nil 或返错 → `source=none`，记 warn 日志，**不 500**。

3. **handoff 瘦路径**：新增仓储方法 `ListLatestTaskResultContractsByTaskIDs(ctx, tenantID, projectID, taskIDs)`（把 `projectTaskGraphLatestResultContracts` 的 SQL 提到可复用的公开方法），再调既有纯函数 `buildProjectTaskGraphHandoffAssessments(tasks, contracts)`。
   **不走**「建全图再丢 nodes」。

4. **工件**：`ListDeclaredArtifactsByTaskIDs`（已存在，直接用）。

5. **证据**：**新增 1 条 sqlc 查询**（无迁移，索引已存在）：

   ```sql
   -- name: ListProjectEvidenceRefsByTaskIDs :many
   SELECT * FROM project_evidence_refs
   WHERE tenant_id = sqlc.arg('tenant_id')::uuid
     AND project_id = sqlc.arg('project_id')::uuid
     AND project_task_id = ANY(sqlc.arg('project_task_ids')::uuid[])
   ORDER BY created_at DESC;
   ```

   **禁止**「List by project 再前端/内存过滤」——limit 截断会把系统正常状态渲染成「交付缺失」。

6. **时间线归一**（§5.4）。

7. **装 rail slots**（§4.5）。

8. **补名**：任务 title、员工/用户 display_name、模板 name。走既有读路径批量补名工具，**面向用户字段零裸 UUID**。

9. **`signals`**：`has_open_decisions` 来自 pending decisions；`active_task_count` 用**既有五值终态集**判定（`isTerminalProjectTaskStatus`，含 done/success——不要新拼三值集，CHANGELOG 07-29 已因此踩过一次）；`demand_terminal` 用 `ProjectDemandStatus` 终态口径。

**事务**：只读，无写锁。

### 5.4 时间线归一规则

- **时间倒序**；主时间 = 事件 `created_at` / 实体关键时间
- 同一业务事实不双计：有事件流优先事件流；无事件时用实体状态回填一条（`id = "synthetic:task_completed:{task_id}"`）
- 映射表写成**服务端代码常量**（`internal/project/event_narrative.go`），Web 不再解析 raw event_type
- `title` 示例：
  - `project_task.completed` →「任务完成 · {task_name}」
  - `decision.requested` + plan_review →「待计划确认」
  - `project_task.dispatch_blocked` →「派发受阻 · {短原因}」
- **未知 event_type → `other` + `title` 走通用中文「协调更新」，绝不吐英文蛇形原串**

**噪音过滤**（默认不进时间线）：`workflow.signaled`、`project_task.dispatch_gate.checked` 的成功态、workspace 心跳类。
**必须进**：blocking / gap / failed / waiting_human / decision / 任务终态 / dispatch_blocked。

**单测（硬性）**：
- 每种 timeline kind 至少一条用例
- **一条遍历 `ProjectEventType` 全部常量的表驱动测试**，断言：映射结果要么是已知 kind，要么是 `other`，且 `title` **不包含 `.` 或 `_`**（英文串泄漏的机械判别器）

### 5.5 项目级文案复用（R2 新增，必做）

`opsEventTitle`（`project-ops-home.tsx`）改为消费同一张归一表。两条路任选，实施时择一并在 PR 注明：

- **A（推荐）**：项目级事件读路径也回 `title`/`kind`/`severity` 三字段（服务端投影），前端删 `opsEventTitle` 的 switch
- **B（次选）**：前端建 `lib/event-narrative-labels.ts` 词表，与服务端常量表**共用同一份 kind 枚举**，`opsEventTitle` default 分支改回落中文

**不接受**：保留现有 `return { title: event.event_type }`。

### 5.6 与既有端点关系

| 端点 | R2 之后 |
|---|---|
| `GET .../dossier` | **一单主读模型** |
| `GET .../launch-detail` | 保留；与 dossier 共用 `loadDemandLaunchFacts`；`/workflows` redirect 维持现状（只取 `project.id`） |
| `GET .../task-graph` | 图视图继续独立用 |
| `GET .../acceptance-criteria` | 右轨展开验收明细时用；dossier 只带计数摘要 |

### 5.7 契约与生成

1. 改 `contracts/control-plane/openapi.yaml`
2. `corepack pnpm generate:control-plane`
3. `corepack pnpm verify:contracts`
4. handler 注册在 `internal/api/server.go` 的 `/project-demands/{demandId}/launch-detail` 旁

---

## 6. Web 实现

### 6.1 文件落点

| 路径 | 职责 |
|---|---|
| `lib/api/projects.ts` | `getProjectDemandDossier` + 类型 |
| `features/projects/components/project-demands-section.tsx` | 容器改版：接 dossier query；中栏 view 切换 |
| `.../demand-dossier-header.tsx` | 单头 + 待你处理 + 密度切换 |
| `.../demand-dossier-timeline.tsx` | 时间线 + 叙事边界说明 |
| `.../demand-dossier-rail.tsx` | 右轨 + 交付判定汇总 |
| `.../demand-dossier-density.ts` | 密度推导 + localStorage 偏好（纯函数，可单测） |
| `lib/status-labels.ts` | timeline kind / rail kind / density 中文 |
| `features/projects/index.tsx` | `useSearch` 增 `view?: string` |
| 测试 | `project-demands-section.test.tsx` 扩展 + 三个新组件单测 + density 单测 + API client 单测 |

### 6.2 数据活性（R2 收紧）

- queryKey：`["demand-dossier", apiBaseUrl, demandId]`
- `useProjectActivityInvalidate` 失效：`demand-dossier` + 既有 `project-task-graph` / `workflow-detail`
- `keepPreviousData` 切 demand 不闪空
- **兜底轮询只在 `drive` 密度下开（30s）；`inspect` 下关轮询**——dossier 是重聚合，已有「风险信号洪峰 53→33 请求」的前车之鉴
- 图视图独立 query 保留（当前 30s 轮询维持不变）

### 6.3 深链对账（回归清单，非新建设）

| 来源 | 期望 |
|---|---|
| 需求列表 Link | 已 canonical — 回归 |
| `/workflows/:demandId` | 仍 redirect 到 canonical — 回归 |
| 收件箱 human task | 能解析到 demand 的落 canonical；**实施时盘点** inbox 构造处，能改的改，不能改的 Web 侧兼容 |
| 飞书卡片 | 历史 `/workflows/{id}` 靠 redirect；**不改 connector** |
| 运行总览 / 任务行 | 若有「所属需求」链，统一 canonical |

验收列表：入口 → 最终 URL → 是否选中正确 demand → 默认 timeline。

### 6.4 文案与空态

- 时间线空：「协调尚未产生可展示节点」
- 右轨空：见 §4.5
- dossier 500/403：SoftCard 错误空态 + 重试，**不回退半残旧布局硬撑**
- 时间线诚实边界说明常显（§4.4）

### 6.5 设计约束

- 实施前重读 `DESIGN.md` 相关页型与主从布局
- 用户可见枚举走 `status-labels.ts`
- 变更后 `corepack pnpm verify:web`

---

## 7. 分期

| 切片 | 内容 | 独立验收 |
|---|---|---|
| **P0a** | `loadDemandLaunchFacts` 抽取 + 归一表 + sqlc 证据查询 + 端口扩展 + `GetDemandDossier` + OpenAPI + 单测（含 §5.4 全枚举表驱动测试） | curl 真库一条 demand |
| **P0b** | Web 容器改版：单头 + 时间线（含边界说明）+ 右轨 + 密度切换 + 图切换 | 浏览器真链路 |
| **P0c** | 项目级 `opsEventTitle` 复用归一表（§5.5）+ 深链对账 + 验收面板归位 | 入口表全绿 + 项目工作台无英文串 |

**默认交付 = P0a+P0b+P0c 全做完**。不得只做 API 无 UI，也不得只做 UI 假数据。P0c 不做则英文串泄漏与三份文案漂移都留在现网——不接受单独砍。

---

## 8. 验收 GATE（真实 E2E）

环境：当前代码的 Web + CP + DB；项目活动 SSE 可用更佳。

| ID | 步骤 | 期望 |
|---|---|---|
| G1 | `GET /api/v1/project-demands/{id}/dossier` 对真实 running demand | 200；`timeline.items` 非空或诚实空；**title/actor/task_name 无裸 UUID、无英文蛇形串**；`signals` 三字段有值 |
| G2 | 对已完成 automation 单 | 前端呈 inspect；右轨/判定可读 |
| G3 | 对有 open plan_review 的 demand | `pending_actions` ≥ 1；`signals.has_open_decisions=true`；前端呈 drive |
| G4 | 浏览器打开 canonical URL | 进入需求处所；**默认时间线**；左列表高亮正确 demand；叙事边界说明可见 |
| G5 | 切换「流程图」 | 图可点任务开弹层；切回时间线经 URL `view` 保持 |
| G6 | 右轨 | 有模板时槽按 `produce_kinds` 保序；**handoff unknown 不显示为失败** |
| G7 | **证据隔离**（R2 新增） | 同项目另一 demand 的证据/工件**不出现**在本单右轨；构造两单各带证据实测 |
| G8 | `/workflows/{demandId}` | redirect 到 canonical 且 dossier 正常 |
| G9 | 密度手动切换 | 切换生效并跨刷新保持（localStorage） |
| G10 | 项目 SSE 或任务状态变化后 | dossier 刷新后时间线/待处理更新；**inspect 密度下无 30s 轮询请求**（Network 面板核对） |
| G11 | **项目工作台事件流**（R2 新增） | 触发一条冷门事件（如 `project_task.dispatch_gate.replan_required`），项目工作台与时间线**均显示中文**，无 `.`/`_` 串 |
| G12 | `verify:contracts` + `verify:control-plane` + `verify:web` | 通过 |

**完成定义**：G1–G12 全过；基线 §4 不变量无破坏；§2 非目标未偷做（特别是**页面上不得出现任何形态的「继续此任务」**）。

---

## 9. 风险与缓解

| 风险 | 缓解 |
|---|---|
| 时间线映射不全 → 用户觉得「丢事件」 | 全枚举表驱动单测 + `other` 兜底不露英文串 + §4.4 叙事边界说明 + 「执行轨迹」出口；上线前用 2–3 条真实 demand 抽查 |
| dossier 重聚合拖慢需求区 | 瘦 handoff 路径（不建全图）+ 证据/工件走索引化 by-task 查询 + inspect 下关轮询；P0a 后测一条大 demand 的响应时间并记录 |
| 与 launch-detail 双维护 | `loadDemandLaunchFacts` **必抽**（§5.3-1），非可选 |
| 归一表三处漂移 | P0c 强制项目级复用（§5.5），且 kind 枚举单一来源 |
| 前端把 dossier 当可写 | 契约与 UI 均无写字段；无接续入口 |
| 范围膨胀做内联审批 | pending 只深链/复用既有卡 |
| 「工作台」概念二义 | §0.3 全面改名 dossier；用户侧统一叫「这一单」 |
| 共享工作树 git 踩踏 | 显式路径 `git add <path>`；不 `git add -A`；提交前 `git symbolic-ref HEAD` 复核 |

---

## 10. 开放细节（不阻塞开工；实施默认值）

1. `view` query 名：`view=timeline|graph`（默认 timeline）
2. 左轨角标：P0 做（`sibling_pending=true`）；剧本短名不做
3. acceptance：dossier 带计数，点开再拉明细
4. §5.5 走 A 还是 B：实施会话按改动面择一，PR 注明
5. chat 模式 demand：同进处所；时间线 kind 通用；不强制剧本槽

---

## 11. 文档关系

| 文档 | 关系 |
|---|---|
| `2026-07-27-workspace-and-playbook-alignment-baseline.md` | **基线**；本 spec 是其 §8 #1（基线表格中的「一单工作台」应随本次改名同步为「一单卷宗」） |
| `2026-07-27-expert-playbook-launch-and-continue-design.md` | 已 superseded；**不**指导本实施 |
| 后续 #2 / #3 / #4 | 挂本处所：接续动作（届时新增入口）、rail git 真数据、发起后落地 URL |

实施完成后：本文状态改「已实施」，并同步基线 §8 的名称与链接。

---

## 12. 审阅检查清单

- [ ] 同意改名 dossier（弃用「工作台」二义）
- [ ] 同意砍掉 disabled 接续入口 / `include_graph` / 服务端 density / `name_directory` / `preview_hint`
- [ ] 同意 P0c（项目级文案复用）**不可单独砍**
- [ ] 同意密度改前端决策 + 用户可切 + localStorage
- [ ] 同意证据查询走新 sqlc（拒绝 list-then-filter）
- [ ] 同意 P0 必须 API+UI 真链路，不接受半边交付

---

## 13. 一句话方案

> **新增 demand 级 dossier 只读聚合（复用已有的 handoff 纯函数 / 工件 by-task 查询 / 剧本端口，新增仅归一表 + 1 条证据 sqlc）；需求区升级为左单列表、中时间线（标注为叙事、图可切）、右剧本 kind 事实轨；密度前端可切；同一归一表回灌项目级堵掉英文串泄漏；接续与变更真能力留给 #2/#3。**

---

## 14. 实施记录（2026-07-31）

**范围**：R2 全量（P0a + P0b + P0c）。零迁移，如 §0.4 预判。

**落点**（新增/改动主档）：

- CP：`internal/project/event_narrative.go`（事件叙事归一表，唯一词表源）、`service_dossier.go`（聚合）、`service.go` 的 `loadDemandLaunchFacts`（launch-detail 与卷宗同源取数）、`handler.go`（`GetDemandDossier` + 响应映射 + `ProjectEvent.narrative` 投影）、`api/server.go` 路由
- 仓储：`ListProjectEvidenceRefsByTaskIDs` / `ListProjectArtifactRefsByTaskIDs`（新 sqlc，走既有索引）、`ListLatestTaskResultContractsByTasks`（暴露既有私有取数）
- 端口：`ScenarioTemplateResolver.ResolveScenarioTemplateProduceKinds` + `scenariotemplate.Service.ProduceKinds`（解析留在模板包内）
- 契约：`ProjectDemandDossier` / `…TimelineItem` / `…RailSlot` + `ProjectEvent.narrative`
- Web：`demand-dossier-{header,timeline,rail,density}`、`project-demands-section` 改版、`getProjectDemandDossier`、`status-labels` 补四张词表、`opsEventTitle` 改读服务端 narrative、收件箱需求引用改指 canonical

**GATE 结果**：G1–G12 全过。含一次**完整真实闭环**：提交需求 → 计划确认卡开出（`待计划确认`）→ 人类批准 → 真实数字员工执行 → 任务完成 → 需求 completed，卷宗全程正确投影（时间线 7 条全中文带任务名、右轨 conclusion/evidence/artifact 三槽、交付判定 fulfilled=1、密度自动 drive→inspect）。G7 用两条真实需求实测交集为空。

**真实 E2E 揪出并修掉的 3 个缺陷**（单测当时全绿，均属集成缺口）：

1. **SSE 漏刷卷宗**——`useProjectActivityInvalidate` 只 invalidate 图与 launch-detail，卷宗停在旧值只能等 30s 兜底轮询。已补 key，并加了会真失败的回归测试（摘掉修复即挂）。
2. **决策身份只在 payload 里**——现网 `decision.requested` 不带 `resource_type/resource_id`，只认 `resource_*` 会让「待计划确认」退回成通用「待人工决策」。已补 payload 外键兜底（`decision_request_id` / `plan_revision_id` / `project_task_id` / `demand_id`）。
3. **协调线程 actor 无名**——`project_coordinator` 类 actor 的 `actor_id` 是 workflow/job 标识，匹配不到项目成员。已补角色化中文回落。

另修一处呈现瑕疵：右轨纯 UUID 引用加「引用 ·」前缀，避免裸标识符被读成对象名称。

**残留（不阻塞，未偷做）**：

- `project-operational-detail.tsx` 内另有一张项目事件文案表（`labels[event.event_type]`，约 12 条，带更细的 summary）。它的 default 已是中文「项目动态已更新」，**不泄漏英文**；收敛它会丢掉那些更细的 summary 文案，故本轮保留，属已知重复。
- Web 全量门禁 1 条失败：`index.test.tsx > renders the project homepage as a project-first master-detail triage queue`，报错是队列「当前处理者」列渲染出 `等待审核 · 负责人甲`——来自**并发会话正在改的球权语义**（工作树里 `project-risk.ts` / `project-risk-home.tsx` / `index.test.tsx` 均为其未提交改动），与本次改动无关，未代其修改。
