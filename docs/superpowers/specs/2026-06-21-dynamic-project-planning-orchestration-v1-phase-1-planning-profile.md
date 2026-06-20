# 动态项目规划 V1 Phase 1：Planning Profile 与可解释选人

日期：2026-06-21
状态：已确认，待实施计划
上级设计：`2026-06-21-dynamic-project-planning-orchestration-v1-design.md`

## 1. 背景

当前 planner 可以从项目成员中选择数字员工，但输入信息不足。模型通常只能看到成员 ID、角色、状态、展示名等浅层信息，无法稳定判断谁具备数据库分析能力、代码修改能力、Runtime 执行条件、MCP 工具、权限范围或近期可靠性。

这会造成三类问题：

- 计划看起来合理，但选人依据不可审计；
- 缺权限、缺工具、Runtime 不可用等问题到执行时才暴露；
- 不同任务类型无法稳定选择不同数字员工，planner 容易只凭角色名猜测。

Phase 1 的目标是先把“选择谁执行任务”变成可解释、可校验、可复核的能力匹配问题。

## 2. 目标

- 为项目内可调度数字员工构造 `DigitalEmployeePlanningProfile`。
- 扩展 `PlanningSnapshot`，让 planner 看到能力、工具、权限、Runtime、负载和可靠性摘要。
- 扩展 planner 输出，让每个 PlannedTask 都说明所需能力、所选员工、匹配能力、缺失能力和选择原因。
- 在服务端实现 deterministic scoring 和 hard validation。
- 让不满足硬条件的员工选择被拒绝或进入人工 review，而不是直接落库分派。

## 3. 非目标

- 不新增完整数字员工管理后台。
- 不在本阶段实现 PlanRevision 表和人工 review 流程；Phase 1 可先落在现有 route decision/task metadata 里。
- 不在本阶段实现 dispatch gate；缺权限等信息可以先体现在 validation error 或 review required。
- 不把 planning profile 当作长期事实源；它是可重建的 read model 或 snapshot。
- 不引入封闭 capability enum；能力标签通过注册表、配置和服务端校验演进。

## 4. DigitalEmployeePlanningProfile

建议结构：

```json
{
  "digital_employee_id": "uuid",
  "display_name": "Data Analyst",
  "role_profile": {
    "primary_role": "data_analyst",
    "secondary_roles": ["incident_investigator"],
    "description": "..."
  },
  "capabilities": [
    {
      "key": "database.read",
      "level": "strong",
      "source": "capability_registry",
      "confidence": 0.9
    }
  ],
  "skills": [
    {
      "key": "sql.analysis",
      "source": "skill_binding"
    }
  ],
  "tool_bindings": [
    {
      "type": "mcp",
      "key": "postgres.readonly",
      "status": "available"
    }
  ],
  "runtime_requirements": {
    "provider_types": ["codex"],
    "workspace_scopes": ["repo:SuperTeam"],
    "runtime_status": "ready"
  },
  "permissions": [
    {
      "scope": "database.read",
      "resource": "dev_database",
      "status": "granted"
    }
  ],
  "context_policy": {
    "allows_sensitive_context": false,
    "max_context_classification": "internal"
  },
  "load_state": {
    "running_tasks": 0,
    "available_slots": 1,
    "lendable": true
  },
  "reliability_signals": {
    "recent_success_count": 8,
    "recent_failure_count": 1,
    "recent_human_reject_count": 0
  },
  "profile_freshness": {
    "generated_at": "2026-06-21T00:00:00Z",
    "source_versions": {}
  }
}
```

字段规则：

- `unknown` 必须显式表达，不能等同于 `granted` 或 `available`。
- profile 必须区分 `not_configured`、`unavailable`、`denied`、`unknown`。
- profile 中的 capability 需要带来源，方便审核和排障。
- 高风险权限只摘要注入 planner，不能把密钥、连接串或敏感详情注入 prompt。

## 5. 画像来源

优先从现有事实聚合：

- project member：项目内哪些数字员工可调度；
- digital employee registry：员工身份、角色、状态；
- capability registry：平台已注册能力；
- skill binding：员工可用技能或方法论；
- MCP / connector binding：工具、外部能力和授权状态；
- Runtime registry：Runtime 节点、Provider 类型、工作区和槽位；
- project/task history：近期完成、失败、超时、人工驳回；
- policy：敏感操作、写操作、高风险动作的审批规则。

缺少来源时，profile builder 必须输出 unknown，并让 validation 或 review 处理。

## 6. PlanningSnapshot 扩展

Planner 输入应从当前薄 member list 扩展为：

```json
{
  "project": {
    "id": "uuid",
    "goal": "...",
    "human_owner_id": "uuid",
    "risk_level": "medium"
  },
  "demand": {
    "id": "uuid",
    "raw_text": "...",
    "task_type_hint": "database_analysis"
  },
  "candidate_employees": [
    {
      "profile": {}
    }
  ],
  "available_capabilities": [],
  "context_refs": [],
  "policy_summary": {},
  "known_blockers": []
}
```

输入控制要求：

- 候选员工按项目成员池和可调度状态过滤；
- profile 只保留 planner 需要的摘要字段；
- 上下文使用引用和摘要，不直接塞大日志或大文档；
- 候选过多时先用规则预筛，再交给模型排序。

## 7. Planner 输出扩展

每个 PlannedTask 至少输出：

```json
{
  "planned_task_key": "analyze-db-usage",
  "title": "分析数据库使用异常",
  "objective": "...",
  "task_type": "database_analysis",
  "selected_employee_id": "uuid",
  "employee_selection_reason": "具备 database.read 和 sql.analysis，Runtime ready，当前无运行任务",
  "required_capabilities": ["database.read", "sql.analysis"],
  "matched_capabilities": ["database.read", "sql.analysis"],
  "missing_capabilities": [],
  "permission_requirements": ["database.read:dev_database"],
  "tool_requirements": ["mcp:postgres.readonly"],
  "runtime_requirements": ["provider:codex"],
  "acceptance_criteria": [
    "列出关键 SQL 或查询摘要",
    "说明数据时间范围",
    "给出结论和不确定性"
  ],
  "verification_requirements": [
    "只读查询成功",
    "结果包含证据引用"
  ],
  "risk_level": "medium"
}
```

输出校验：

- `selected_employee_id` 必须来自候选池；
- `employee_selection_reason` 不能为空；
- `required_capabilities` 与 task_type 必须一致或有解释；
- `missing_capabilities` 非空时不能直接自动分派；
- 高风险 task 必须标记 review 或 approval。

## 8. Deterministic Scoring

服务端给每个候选员工计算匹配分，供 planner 输入、输出校验和人工 review 使用。

建议权重：

| 维度 | 权重 | 判断方式 |
| --- | ---: | --- |
| capability / skill | 40 | required capability 是否满足，是否 strong match |
| role fit | 20 | primary/secondary role 是否符合 task_type |
| runtime / workspace | 15 | Provider、Runtime、workspace、槽位是否可用 |
| permission / tool | 10 | 权限、MCP、connector 是否 granted/available |
| load | 10 | 当前并发、排队、可借调状态 |
| reliability | 5 | 最近成功、失败、超时、人工驳回 |

硬性失败：

- 不在项目数字员工池；
- 员工状态不可调度；
- 缺少必须权限；
- 必须工具 unavailable 或 denied；
- Runtime/Provider 不满足任务硬要求；
- context policy 不允许接收该任务上下文。

硬性失败不能靠高分抵消。

## 9. 按任务类型的默认能力

### 数据库分析

默认 required capabilities：

- `database.read`
- `sql.analysis`
- `data.quality.check`
- `business.metric.interpretation`

可能触发 review：

- 查询敏感表；
- 需要跨租户数据；
- 需要写入或修复数据；
- 分析范围不清楚。

### 系统问题诊断

默认 required capabilities：

- `incident.triage`
- `log.analysis`
- `metrics.analysis`
- `code.path.tracing`
- `runtime.diagnostics`

可能触发 review：

- 需要访问生产日志；
- 需要执行重启、降级、扩容或回滚；
- 需要跨系统凭据。

### 功能开发

默认 required capabilities：

- `codebase.analysis`
- `implementation`
- `testing.verification`
- `artifact.reporting`

按子任务补充：

- 后端：`backend.implementation`
- 前端：`frontend.implementation`
- 数据库：`database.migration`
- 契约：`api.contract.generation`
- Runtime：`runtime.provider.integration`

可能触发 review：

- 数据库迁移；
- 生产发布；
- 删除或覆盖用户数据；
- 安全、权限或计费逻辑变更。

## 10. 存储策略

Phase 1 可以先不新增核心表，优先复用或扩展现有 metadata：

- route decision metadata 保存 profile snapshot hash；
- ProjectTask planner metadata 保存选择原因、分数、匹配能力和缺失能力；
- project event 保存 planner validation warning；
- 如果现有结构不足，再增加 planning profile read model 或 JSONB snapshot。

必须保证员工配置变化后，历史计划仍能解释当时为什么选择该员工。

## 11. 服务端校验

Plan validation 需要新增或扩展：

- candidate employee existence check；
- active executor pool check；
- required capability hard match；
- permission/tool/runtime hard match；
- task_type 与 required capabilities consistency；
- missing capability review requirement；
- selection score threshold；
- duplicate planned_task_key check；
- expected output 和 acceptance criteria completeness。

校验失败结果：

- 硬错误：不能落库或不能分派；
- 软错误：进入人工 review；
- 可修正错误：要求 planner 生成新 revision。

## 12. 验证

单元测试：

- profile builder 能聚合 role、capability、skill、tool、permission、runtime、load；
- unknown 状态不会被当成 available；
- scoring 能区分 full match、partial match 和 hard missing；
- planner 输出缺少 selection reason 时失败；
- 选择不在候选池内的员工时失败；
- 缺少硬 capability 时进入 validation failed 或 review required。

集成测试：

- `PlanningSnapshot` prompt 包含 bounded candidate profiles；
- 数据库分析 demand 优先选择具备 database read 的数字员工；
- 缺少数据库权限的员工不会被自动分派；
- profile snapshot hash 能写入 route decision 或 task metadata。

本阶段不要求真实 Runtime smoke，但不能声称完整动态调度可用。

## 13. 验收标准

- planner 能看到足够的数字员工能力画像。
- plan 中每个任务都有明确选人理由。
- 服务端能拒绝明显不满足能力、权限或 Runtime 约束的选择。
- 人类 review 能看到选择依据、匹配能力和缺失能力。
- 现有 planner/route/task 测试保持通过。
