# 2026-06-10-project-management-v2-governance-archive-design

## 1. 阶段目标

V2 的目标是把项目从“可运行的协调容器”增强为“可审计、可验收、可归档、可复盘的业务闭环容器”。

V2 聚焦：

- 证据链。
- 工件与报告引用。
- 预算流水。
- 验收结论。
- 归档快照。
- 配置修订历史。
- 审计中心和成本中心联动。

V2 不重做 V0 的项目管理结构，也不重写 V1 的协调 Workflow。它在已有项目事实和协调事件上增加治理闭环。

## 2. 前置条件

V2 依赖：

- V0 的项目、成员、任务、需求、事件和配置修订。
- V1 的 RouteDecision、ExecutionSummary、TransferRequest、DecisionRequest 和 Workflow signal。
- 项目事件流已经能覆盖需求、规划、执行、转派、人类决策和配置变更。

如果 V1 未完成，不应实施 V2。

## 3. 范围边界

### 3.1 包含

后端：

- EvidenceRef。
- ArtifactRef。
- ReportRef。
- BudgetLedger。
- ProjectAcceptance。
- ProjectArchiveSnapshot。
- ProjectConfigRevision 增强。
- 审计中心 project_id 关联。
- 成本中心 project_id 关联。

前端：

- 证据链视图。
- 工件与报告列表。
- 成本和预算视图。
- 验收处理。
- 归档预览和归档结果。
- 配置修订历史。

### 3.2 不包含

V2 不做：

- 替代外部文档管理系统。
- 完整 BI 报表平台。
- 自动事实真伪判定。
- 跨租户项目归档迁移。
- 复杂合规保留策略引擎。

## 4. 数据模型

`project_evidence_refs`：

- `id`
- `tenant_id`
- `project_id`
- `project_task_id`
- `route_decision_id`
- `execution_summary_id`
- `evidence_type`
- `title`
- `summary`
- `source_type`
- `source_ref`
- `artifact_ref_id`
- `submitted_by_type`
- `submitted_by_id`
- `verification_status`
- `metadata`
- `created_event_id`
- `created_at`

`project_artifact_refs`：

- `id`
- `tenant_id`
- `project_id`
- `project_task_id`
- `artifact_id`
- `artifact_type`
- `title`
- `object_ref`
- `content_type`
- `size_bytes`
- `checksum`
- `retention_status`
- `retention_hold_id`
- `metadata`
- `created_event_id`
- `created_at`

`project_report_refs`：

- `id`
- `tenant_id`
- `project_id`
- `report_type`
- `title`
- `summary`
- `object_ref`
- `format`
- `generated_by_type`
- `generated_by_id`
- `created_event_id`
- `created_at`

`project_budget_ledger`：

- `id`
- `tenant_id`
- `project_id`
- `coordination_job_id`
- `project_task_id`
- `digital_employee_id`
- `cost_type`
- `estimated_tokens`
- `actual_tokens`
- `estimated_cost`
- `actual_cost`
- `source`
- `reason`
- `created_event_id`
- `created_at`

`project_acceptance_records`：

- `id`
- `tenant_id`
- `project_id`
- `accepted_by_user_id`
- `status`
- `conclusion`
- `summary`
- `evidence_ref_ids`
- `report_ref_ids`
- `created_event_id`
- `created_at`

`project_archive_snapshots`：

- `id`
- `tenant_id`
- `project_id`
- `snapshot_type`
- `status`
- `object_ref`
- `summary`
- `included_counts`
- `retained_artifact_ids`
- `retention_lock_event_id`
- `created_by_user_id`
- `created_event_id`
- `created_at`

## 5. 证据链设计

证据不是装饰字段，而是项目验收和复盘的基础事实。

每个关键对象都可以挂证据：

- ProjectDemand：需求来源、附件、上下文。
- RouteDecision：规划输入、候选员工、选择理由。
- ProjectTask：执行输入、输出、日志、测试结果。
- ExecutionSummary：结论、工件、不确定性、缺失信息。
- DecisionRequest：人类判断依据和批准结果。
- ProjectAcceptance：最终验收证据。

证据状态：

```text
submitted
linked
verified
rejected
superseded
```

V2 只做证据引用和校验状态，不做自动真伪鉴定。

## 6. 工件保留与归档锁定

项目侧不直接拥有 S3 对象生命周期。底层对象仍由 `artifact` 模块统一管理，`project_artifact_refs.artifact_id` 指向全局工件记录，`object_ref`、`checksum`、`size_bytes` 等字段只是项目侧审计快照。

归档项目时，V2 必须调用 `artifact` 模块为所有被 EvidenceRef、ReportRef、ArchiveSnapshot 引用的工件设置保留锁或等效 GC 保护标记。建议语义为：

```text
retention_status = project_archive_hold
retention_hold_id = artifact 模块返回的保留锁标识
```

归档快照必须记录 `retained_artifact_ids` 和 `retention_lock_event_id`。全局对象 GC 策略不得删除处于项目归档保留状态的工件；如果保留锁失败，项目归档应失败或进入 `archive_pending_retention` 状态，不能生成看似成功但证据可能被清理的归档。

## 7. 预算与成本

BudgetLedger 记录项目级成本流水。

记录来源：

- 协调规划。
- RouteDecision。
- 数字员工执行。
- 人类复核辅助。
- 报告生成。

展示维度：

- 按项目。
- 按需求。
- 按数字员工。
- 按任务。
- 按成本类型。

V2 成本中心联动只要求能按 `project_id` 查询与汇总，不要求复杂预测。

## 8. 验收与归档

### 8.1 验收

验收动作由人类负责人或验收人执行。

验收结果：

```text
accepted
rejected
needs_more_evidence
partially_accepted
```

验收必须引用：

- 最终报告。
- 关键证据。
- 未解决风险。
- 人类结论。

### 8.2 归档

项目归档前必须生成归档预览：

```text
需求数
任务数
RouteDecision 数
ExecutionSummary 数
DecisionRequest 数
EvidenceRef 数
ArtifactRef 数
ReportRef 数
预算流水数
未关闭风险
```

归档后：

- 项目不可再提交需求。
- 项目不可再修改配置。
- 项目可查看归档快照。
- 审计、证据、工件、预算和报告保留。
- 所有关联工件必须已获得 artifact 模块的保留锁或 GC 保护。

## 9. 前端设计

项目详情新增 Tab 或区域：

- 证据。
- 工件。
- 成本。
- 验收。
- 归档。

配置页新增：

- 配置修订历史。
- 策略版本对比。
- 证据归档规则执行状态。

运行态页新增：

- 证据完整度可点击查看来源。
- 任务行展示证据状态。
- 人类决策卡片展示证据和风险。

归档页：

- 归档预览。
- 风险提示。
- 生成归档快照。
- 下载或查看归档报告。

## 10. API 设计

V2 新增：

```text
GET    /api/v1/projects/{projectId}/evidence
POST   /api/v1/projects/{projectId}/evidence
PATCH  /api/v1/projects/{projectId}/evidence/{evidenceId}

GET    /api/v1/projects/{projectId}/artifacts
GET    /api/v1/projects/{projectId}/reports

GET    /api/v1/projects/{projectId}/budget-ledger
GET    /api/v1/projects/{projectId}/budget-summary

POST   /api/v1/projects/{projectId}/acceptance
GET    /api/v1/projects/{projectId}/acceptance

GET    /api/v1/projects/{projectId}/archive-preview
POST   /api/v1/projects/{projectId}/archive-snapshot
GET    /api/v1/projects/{projectId}/archive-snapshots

GET    /api/v1/projects/{projectId}/config-revisions
GET    /api/v1/projects/{projectId}/config-revisions/{revisionId}
```

所有写操作必须写 ProjectEvent。

## 11. 审计与联动

审计中心：

- 按 `resource_type = project` 与 `resource_id = project_id` 查询。
- 支持从审计事件跳转到项目、任务、决策或证据。

成本中心：

- 按 `project_id` 聚合预算流水。
- 支持从成本记录跳转到项目任务或协调 job。

审批中心：

- 项目侧人类决策投影必须通过 `approval_request_id` 指向全局审批请求。
- 全局 approval 模块是审批事实源，项目侧只负责展示项目上下文、事件关联和跳转。
- 审批中心处理结果必须能回到项目事件流。

## 12. 验收标准

功能验收：

- 项目任务、执行结果、人类决策能挂证据。
- 项目详情能查看证据链和工件。
- 项目能生成预算流水和成本汇总。
- 项目能提交验收结论。
- 项目归档前有预览，归档后有快照。
- 项目归档会锁定所有被证据、报告和归档快照引用的 artifact，防止全局 GC 清理。
- 配置修订历史可查看。

技术验收：

- 所有新增对象包含 `tenant_id`。
- 所有新增写操作写 ProjectEvent。
- 审计中心能按 project_id 查询关键动作。
- 成本中心能按 project_id 聚合。
- artifact 模块能按 project archive hold 阻止对象 GC。
- 归档后历史仍可解释，不依赖被删除对象。
- 前端 Playwright 覆盖证据、验收和归档主路径。

## 13. 风险

- 证据链如果只做 UI 标签，会失去审计价值。必须落结构化引用。
- 成本如果直接混入任务表，会破坏运行事实。必须用 ledger。
- 归档如果只改项目状态，没有快照，后续复盘会受对象变更影响。
- 归档如果没有调用 artifact 保留锁，全局 GC 可能删除项目证据，必须阻断。
- V2 不应反向修改 V0/V1 的核心心智，只增强治理闭环。
