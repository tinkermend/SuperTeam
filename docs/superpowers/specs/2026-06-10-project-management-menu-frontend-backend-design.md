# 2026-06-10-project-management-menu-frontend-backend-design

## 1. 文档定位

本文是项目管理菜单与阶段规格的总索引，只记录跨阶段共同决策和执行约束，不作为直接开发清单。

后续实施必须选择某一个阶段规格作为输入：

- `docs/superpowers/specs/2026-06-10-project-management-v0-foundation-design.md`
- `docs/superpowers/specs/2026-06-10-project-management-v1-temporal-coordination-design.md`
- `docs/superpowers/specs/2026-06-10-project-management-v2-governance-archive-design.md`

每次 implementation plan 只能绑定一个阶段规格。V0 不实现 V1/V2 的业务能力；V1 不重做 V0 的页面心智；V2 不反向修改 V0/V1 的核心模型。

## 2. 设计依据

本轮设计基于以下材料：

- `docs/design/projectManager/temporal-project-coordination-design.md`
- `docs/design/projectManager/project-management-operations-detail-gpt-image2.png`
- `docs/design/projectManager/project-management-configuration-governance-gpt-image2.png`

项目管理要解决的是项目本身的创建、配置、运行状态、成员、任务、事件、审批、证据和归档，不替代未来独立的“任务发起”或“需求提交”入口。

## 3. 共同产品决策

### 3.1 左侧菜单

左侧全局菜单只保留一个一级入口：

```text
项目管理 -> /projects
```

不在全局左侧菜单展开项目任务、项目成员、项目配置、项目事件等二级菜单。这些能力属于具体项目，放在项目详情内部。

### 3.2 页面层级

项目管理采用“运行态详情为默认页，配置治理为独立页”的结构：

```text
/projects
  项目管理首页，左侧项目列表，右侧显示选中项目运行态。

/projects/$projectId
  指定项目运行态详情，可直接分享 URL。

/projects/$projectId/config
  指定项目配置治理页。
```

第二张参考图对应 `/projects` 和 `/projects/$projectId` 的运行态方向。第三张参考图对应 `/projects/$projectId/config` 的配置治理方向。

### 3.3 项目协调线程

项目不定义“项目协调员数字员工”。每个项目绑定一个虚拟协调线程：

```text
WorkflowID = project-coordinator:{project_id}
```

该线程由 Temporal Workflow 承载，是项目内置的独占协调状态机。它不是数字员工，不出现在数字员工列表中，也不是项目成员。

项目需要定义的是：

- 人类负责人。
- 可选 leader。
- 验收人。
- 项目内可调度数字员工池。
- 协调策略、审批策略和证据策略。

### 3.4 项目事实源

Control Plane 的 `project` 模块是项目事实源。它负责项目、成员、需求、项目任务、事件、配置、路由决策、执行结果、证据和归档等项目侧事实。

以下边界保持独立：

- 数字员工定义归 `employee` 模块。
- 底层 Runtime Task 归 `task` 模块。
- Runtime 节点、claim、lease 归 `runtime` 模块。
- 企业审批中心归 `approval` 模块。
- 工件存储归 `artifact` 模块。
- Temporal worker 和 workflow runtime 归 `workflow` 模块。

## 4. 阶段划分

### 4.1 V0：项目管理可用骨架

规格文档：

```text
docs/superpowers/specs/2026-06-10-project-management-v0-foundation-design.md
```

目标：

- 把 `/projects` 从占位页变成真实项目管理入口。
- 实现项目列表、项目详情运行态、项目配置治理页。
- 建立项目、成员、项目任务、项目需求、项目事件和配置修订基础表。
- 提供 ProjectOverview 和 ProjectConfig API。

明确不做：

- 真实 Temporal Workflow Worker。
- LLM 规划。
- 数字员工自动分派。
- ExecutionSummary、TransferRequest、DecisionRequest。
- 证据归档、预算流水、验收归档。

### 4.2 V1：Temporal 项目协调接入

规格文档：

```text
docs/superpowers/specs/2026-06-10-project-management-v1-temporal-coordination-design.md
```

目标：

- 接入 `project-coordinator:{project_id}` Workflow。
- 让需求提交、成员变更、策略变更、数字员工结果、人类决策通过 signal 驱动项目运行态。
- 引入 CoordinationJob、RouteDecision、ExecutionSummary、TransferRequest、DecisionRequest。
- 保证同一项目协调决策串行，数字员工任务并发执行。

明确不做：

- 复杂流程图编辑器。
- 跨项目资源排班。
- 自动学习路由策略。
- 高级证据评分和成本预测。

### 4.3 V2：治理、证据与归档增强

规格文档：

```text
docs/superpowers/specs/2026-06-10-project-management-v2-governance-archive-design.md
```

目标：

- 增加证据链、工件引用、报告引用、预算流水、验收结论、归档快照和配置修订历史。
- 让项目支持审计中心、成本中心和审批中心按 `project_id` 追踪。
- 让项目归档后仍可复盘需求、任务、决策、证据、审批、报告和成本。

明确不做：

- 替代外部文档管理系统。
- 完整 BI 报表平台。
- 自动事实真伪判定。
- 跨租户项目归档迁移。

## 5. 执行规则

实施计划必须遵守：

- V0 只做真实项目管理骨架，不实现自动协调。
- V1 只在 V0 事实源上接入 Temporal，不重构项目管理入口心智。
- V2 只增强治理、证据、预算和归档，不改变项目核心聚合边界。
- 所有阶段都必须保留 `ProjectEvent`，关键写操作必须产生事件。
- 所有阶段都必须遵守 `DATABASE_DESIGN.md` 的 UUID-first、Tenant-first、Team-aware 和审计保留规则。

如果阶段规格与本文冲突，以阶段规格为准；如果阶段规格与 `AGENTS.md` 或 `DATABASE_DESIGN.md` 冲突，以项目级规则为准。
