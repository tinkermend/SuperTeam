# 动态项目规划 V1 Phase 2：PlanRevision 与精确一次分解

日期：2026-06-21
状态：已确认，待实施计划
上级设计：`2026-06-21-dynamic-project-planning-orchestration-v1-design.md`

## 1. 背景

Phase 1 让 planner 的数字员工选择变得可解释，但 plan 仍需要成为正式业务对象。当前项目协调链路更接近“生成 route decision 后直接创建 task”，缺少版本化计划、人工 review 和 accepted plan 精确一次分解。

真实业务里，用户发起的任务经常需要：

- 看一眼计划是否符合业务目标；
- 要求 planner 补充遗漏任务；
- 驳回错误任务拆解；
- 确认高风险操作；
- 在需求变化后生成新计划；
- 确保 workflow retry 不会重复创建 ProjectTask。

Phase 2 的目标是把 planner 生成的结构化计划持久化为 `PlanRevision`，并且只允许被接受的 revision 精确一次转换为 ProjectTask DAG。

## 2. 目标

- 新增或建立 `PlanRevision` 业务对象。
- 支持 plan draft、validation failed、pending review、accepted、rejected、superseded、decomposing、decomposed 等状态。
- 让人类 owner 可以接受、驳回或要求修改整版 plan。
- accepted revision 使用幂等 claim 精确一次分解为 ProjectTask DAG。
- 分解后的 ProjectTask 保留 plan contract、员工选择依据、依赖关系和验收标准。
- 支持 planner 生成新 revision 时 supersede 旧 draft/pending revision。

## 3. 非目标

- V1 不支持部分接受 plan，也不支持只勾选某几个任务分解。
- V1 不在同一个 PlanRevision 内原地修改任务；修改必须产生新 revision。
- 不把 Temporal workflow memory 当作 plan 事实源。
- 不在本阶段实现 PreDispatchGate；分解后的 task 仍可进入后续阶段处理。
- 不在本阶段实现完整 Web 计划图编辑器；只需要 review 所需的读模型和动作。

## 4. PlanRevision 状态机

建议状态：

| 状态 | 含义 |
| --- | --- |
| `draft` | planner 已生成，尚未完成服务端校验 |
| `validation_failed` | 服务端校验失败，不能接受 |
| `pending_review` | 等待人类 review |
| `accepted` | 已被人类或策略接受 |
| `rejected` | 被人类驳回 |
| `superseded` | 被更新 revision 替代 |
| `decomposing` | accepted revision 正在分解 task |
| `decomposed` | 已成功分解为 ProjectTask DAG |

允许转换：

- `draft -> validation_failed`
- `draft -> pending_review`
- `draft -> accepted`
- `pending_review -> accepted`
- `pending_review -> rejected`
- `pending_review -> superseded`
- `validation_failed -> superseded`
- `accepted -> decomposing`
- `decomposing -> decomposed`

原则：

- `accepted` 后不能修改 payload。
- `decomposed` 后不能回滚成 draft。
- 新 revision 只能 supersede 非终态或未分解的旧 revision。
- 已分解产生的 ProjectTask 不因新 revision 被删除，只能 append-only 追加、取消或替代。

## 5. 数据模型

建议表或实体：`project_plan_revisions`

| 字段 | 说明 |
| --- | --- |
| `id` | UUID |
| `tenant_id` | 租户 ID |
| `team_id` | 团队 ID |
| `project_id` | 项目 ID |
| `demand_id` | 用户需求、触发事件或协调请求 ID |
| `revision_number` | 从 1 递增 |
| `status` | 状态 |
| `payload` | 结构化 PlanRevisionPayload |
| `planner_provider` | planner provider |
| `planner_model` | planner model |
| `planner_input_hash` | PlanningSnapshot hash |
| `plan_fingerprint` | canonical payload hash |
| `validation_errors` | 校验错误 |
| `validation_warnings` | 校验警告 |
| `review_required` | 是否需要人工 review |
| `review_reason` | review 原因 |
| `accepted_by` | 接受人或系统 |
| `accepted_at` | 接受时间 |
| `superseded_by_revision_id` | 替代 revision |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |

唯一约束建议：

- `(project_id, demand_id, revision_number)`
- `(project_id, demand_id, plan_fingerprint)` 用于避免同一计划重复创建；
- accepted revision 可通过 partial unique 约束或服务端事务保证同一 demand 只有一个当前 accepted revision。

## 6. PlanRevisionPayload

建议结构：

```json
{
  "summary": "计划摘要",
  "assumptions": [],
  "risk_assessment": {
    "level": "medium",
    "reasons": []
  },
  "human_review": {
    "required": true,
    "reasons": []
  },
  "tasks": [
    {
      "planned_task_key": "inspect-db",
      "title": "检查数据库异常数据",
      "objective": "...",
      "task_type": "database_analysis",
      "selected_employee_id": "uuid",
      "employee_selection_reason": "...",
      "required_capabilities": [],
      "matched_capabilities": [],
      "missing_capabilities": [],
      "permission_requirements": [],
      "tool_requirements": [],
      "runtime_requirements": [],
      "input_context_refs": [],
      "expected_outputs": [],
      "acceptance_criteria": [],
      "verification_requirements": [],
      "depends_on": [],
      "risk_level": "medium",
      "human_review_required": false
    }
  ],
  "final_summary_contract": {
    "required_sections": [
      "conclusion",
      "evidence",
      "risks",
      "next_steps"
    ]
  }
}
```

服务端 canonicalization 要求：

- task 顺序稳定；
- `depends_on` 引用 `planned_task_key`；
- hash 时排除非语义字段，例如生成时间；
- `plan_fingerprint` 用于幂等和审计。

## 7. Plan 校验

在进入 `pending_review` 或 `accepted` 前必须校验：

- payload schema 完整；
- `planned_task_key` 唯一；
- dependency 不引用不存在的 key；
- dependency graph 无环；
- root task 至少一个；
- selected employee 在候选池；
- employee capability match 满足 Phase 1 规则；
- expected outputs 和 acceptance criteria 非空；
- 高风险任务带 human review 或 approval 标记；
- final summary contract 存在。

校验结果：

- hard error：`validation_failed`；
- warning：允许 pending review，但不能自动 accepted；
- clean 且低风险：可由 policy 自动 accepted；
- high risk：进入 `pending_review`。

## 8. 人工 Review

V1 的 review 粒度是整版 plan。

支持动作：

- `accept`：接受当前 revision，并绑定 accepted revision id；
- `reject`：驳回，记录原因；
- `request_changes`：记录修改要求，触发新 revision；
- `cancel`：终止 demand 或等待用户后续输入。

review UI/read model 至少展示：

- plan summary；
- task DAG；
- 每个 task 的数字员工选择理由；
- required/matched/missing capabilities；
- 权限和 Runtime 需求；
- 风险和需要人工确认的动作；
- expected outputs 和 acceptance criteria。

review 事件必须写入：

- project event；
- audit log；
- approval/decision 对象；
- PlanRevision 状态。

## 9. 精确一次分解

Accepted revision 转 ProjectTask 必须通过 decomposition claim。

建议新增表或约束：`project_plan_decomposition_claims`

| 字段 | 说明 |
| --- | --- |
| `id` | UUID |
| `project_id` | 项目 ID |
| `demand_id` | demand ID |
| `accepted_plan_revision_id` | accepted revision |
| `plan_fingerprint` | plan hash |
| `status` | `in_flight / completed / failed` |
| `created_task_ids` | 已创建 ProjectTask ID |
| `error` | 失败原因 |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |

唯一键：

- `(project_id, demand_id, accepted_plan_revision_id)`

分解流程：

1. 事务内确认 revision 状态为 `accepted`。
2. 插入 decomposition claim；若已存在则读取现有结果。
3. 将 revision 状态推进到 `decomposing`。
4. 按 `planned_task_key` 创建 ProjectTask。
5. 创建 `project_task_dependencies`。
6. 写入 planner metadata、employee selection snapshot、expected outputs、acceptance criteria。
7. claim 标记 `completed` 并保存 task IDs。
8. revision 状态推进到 `decomposed`。

如果 workflow retry 再次执行：

- claim completed：直接返回已有 task IDs；
- claim in_flight 且部分 task 已存在：按 stable key 恢复，不重复创建；
- claim failed：根据错误类型允许人工恢复或重新分解。

## 10. ProjectTask 映射

每个 ProjectTask 必须保存：

- `accepted_plan_revision_id`
- `planned_task_key`
- `title`
- `objective`
- `task_type`
- `assigned_digital_employee_id` 或 equivalent owner field
- `planner_metadata`
- `required_capabilities`
- `matched_capabilities`
- `missing_capabilities`
- `permission_requirements`
- `tool_requirements`
- `runtime_requirements`
- `input_context_refs`
- `expected_outputs`
- `acceptance_criteria`
- `verification_requirements`
- `risk_level`
- `human_review_required`

状态初始为 `planned`。是否进入 `queued` 由 Phase 3 PreDispatchGate 决定。

## 11. 依赖语义

Plan 内 `depends_on` 转成显式 `project_task_dependencies`。

规则：

- dependency 表示执行阻塞，不表示 UI 层级；
- parent/section 不能替代 dependency；
- dependency 只能从 upstream completed/accepted result 解锁；
- blocked/failed 的 upstream 必须阻塞 downstream，直到恢复策略处理；
- append-only replan 只能新增 task 和 dependency，不能改写已执行 dependency 历史。

## 12. 恢复策略

Planner 输出无效：

- 保存 `validation_failed` revision；
- 不创建 task；
- 写入 validation errors；
- 可要求 planner 生成新 revision。

人工要求修改：

- 当前 revision `superseded`；
- 新 revision_number + 1；
- 新 revision 重新校验和 review。

分解中断：

- claim 保持 `in_flight`；
- 已创建 task 保留；
- retry 按 stable task key 补齐；
- 完成后 claim `completed`。

重复接受：

- 若同 demand 已有 accepted/decomposed revision，拒绝新接受或要求先走 replan 决策；
- 不允许两个 accepted revision 同时分解到同一 demand。

## 13. 验证

单元测试：

- PlanRevision 状态转换；
- validation failed 不能接受；
- rejected/superseded revision 不能分解；
- accepted revision payload 不能修改；
- plan fingerprint 稳定；
- dependency graph 无环校验；
- duplicate planned_task_key 拒绝；
- decomposition claim 幂等。

集成测试：

- 创建 demand 后生成 revision；
- human accept 指向具体 revision id；
- accepted revision 创建 ProjectTask DAG；
- workflow retry 不重复创建 task；
- request changes 生成新 revision 并 supersede 旧 revision；
- ProjectTask 保存 plan contract 字段。

Phase 2 不要求真实 Runtime 执行，但必须证明 task 分解事实源正确。

## 14. 验收标准

- 一个 demand 可以拥有多个版本化 plan revision。
- 人类 review 接受的是具体 revision，不是浮动的最新 plan。
- 只有 accepted revision 可以分解 ProjectTask。
- 分解具备精确一次语义。
- ProjectTask DAG 保留完整任务契约、员工选择依据和依赖关系。
- 历史 revision 和已创建 task 不被新 plan 改写。
