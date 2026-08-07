# 动态项目规划 V1 Phase 4：Result Contract、Revision Loop 与最终总结

> 复核状态：06-21动态项目编排v1设计落地

日期：2026-06-21
状态：已确认，待实施计划
上级设计：`2026-06-21-dynamic-project-planning-orchestration-v1-design.md`

## 1. 背景

Phase 3 解决任务分派前的安全性，但真正的动态任务编排还需要根据执行结果继续推进。数字员工执行完任务后，Control Plane 不能只看一段自由文本总结就释放下游任务，否则会出现：

- 任务说完成了，但验收标准没有逐项证明；
- 上游实际阻塞，下游仍被释放；
- 需要返工的任务被当成成功；
- 需要人工判断的结果没有进入审批；
- 最终 summary 缺少证据、风险和未完成事项。

Phase 4 的目标是统一 Runtime/Provider 写回的结构化结果，让 Control Plane 可以可靠判断任务完成、阻塞、失败、需要 revision、需要 replan 或需要人工 review，并最终生成项目级结果汇报。

## 2. 目标

- 定义 `TaskResultContract`，作为 Runtime/Provider 写回 ProjectTask 的统一结构。
- 校验每个 acceptance criterion 的完成证据。
- 根据结构化结果释放 downstream、创建 revision task、触发 replan 或等待人类。
- 支持 task-level revision loop 和 plan-level append-only replan。
- 生成 final demand summary，汇总结论、证据、任务状态、风险和后续建议。
- 保持历史 append-only，不改写已经执行的 task 和 attempt。

## 3. 非目标

- 不让 Provider 直接修改 ProjectTask graph。
- 不让自由文本日志决定依赖释放。
- 不在本阶段实现完整 execution ledger 采集；可与 execution ledger 读模型对接。
- 不把失败一律自动重试；重试要受 policy、attempt、风险和人工决策约束。
- 不把人类验收替换为模型自评。

## 4. TaskResultContract

建议结构：

```json
{
  "status": "completed",
  "summary": "完成了数据库异常分析，发现订单表中某类状态聚合异常。",
  "acceptance_results": [
    {
      "criterion": "列出关键 SQL 或查询摘要",
      "status": "passed",
      "evidence_refs": ["artifact:sql-summary"],
      "notes": "..."
    }
  ],
  "evidence_refs": [
    {
      "type": "query_result",
      "ref": "artifact:query-result-1",
      "summary": "..."
    }
  ],
  "artifact_refs": [
    {
      "type": "markdown",
      "ref": "artifact:analysis-report"
    }
  ],
  "changes_made": [],
  "verification": [
    {
      "type": "database_query",
      "status": "passed",
      "summary": "只读查询成功"
    }
  ],
  "risks": [
    {
      "level": "medium",
      "description": "仅覆盖 2026-06-20 至 2026-06-21 数据"
    }
  ],
  "follow_up_requests": [],
  "human_review_request": null,
  "replan_request": null
}
```

允许 status：

- `completed`
- `revision_needed`
- `blocked`
- `failed`
- `cancelled`

## 5. Result Validation

写回时必须校验：

- status 合法；
- summary 非空；
- 每个 acceptance criterion 有对应 `acceptance_results`；
- `completed` 状态下所有必须 criterion 通过或有人工接受理由；
- evidence/artifact ref 可解析；
- verification 符合 task 的 verification requirements；
- `revision_needed` 必须给出 revision reason；
- `blocked` 必须给出 blocker 和需要谁处理；
- `failed` 必须给出 error family、retryable 和恢复建议；
- 需要人类判断时必须包含 structured human review request。

校验失败：

- 不推进 task 到 completed；
- attempt 记录 writeback validation error；
- task 进入 waiting_human 或 failed/recovery；
- 写入 project event 供排障。

## 6. 结果状态处理

### completed

处理规则：

- 校验 acceptance results；
- 如 task 风险低且所有 criterion 通过，task 进入 completed；
- 如 task 需要人类验收，进入 waiting_human/result_review，人工接受后才释放下游；
- 写入 execution summary 和 project event；
- 释放 downstream ready 检查，进入 Phase 3 gate。

### revision_needed

适用场景：

- 任务完成一部分，但不满足验收标准；
- 发现输入不完整，需要同一任务返工；
- 实现需要补测试、补文档、补证据；
- 代码 review 或人工验收要求修改。

处理规则：

- 不释放下游；
- 创建 revision task 或同 ProjectTask 下新 attempt，取决于契约是否变化；
- 如果契约不变，优先同 task 新 attempt；
- 如果契约变化，append-only 新 ProjectTask，标记 `revision_of_task_id`；
- 保留旧 attempt 和结果。

### blocked

适用场景：

- 缺外部权限；
- 等待用户输入；
- 上游系统不可用；
- 数据缺失；
- 需要人工业务判断。

处理规则：

- task 进入 waiting_human 或 blocked equivalent；
- 创建 blocker/human request；
- downstream 保持阻塞；
- blocker 解除后重新走 Phase 3 gate。

### failed

适用场景：

- Provider 执行失败；
- 测试失败；
- 查询失败；
- 工具调用失败；
- 任务目标无法达成。

处理规则：

- 按 error family 和 retryable 判断是否可重试；
- 暂态失败可在同 task 下新 attempt；
- 业务契约不成立时触发 replan decision；
- 高风险或多次失败进入人工判断；
- downstream 不释放。

### cancelled

适用场景：

- 用户取消；
- 计划被替代；
- 人工判断不再需要；
- 上游结果使本任务失效。

处理规则：

- task 终态化；
- downstream 根据 dependency policy 取消或等待 replan；
- 写入取消原因；
- 不删除历史。

## 7. Dependency Unlock

downstream 释放规则：

- 所有 upstream dependency 必须 completed；
- upstream 如果需要人工验收，必须已经 accepted；
- upstream acceptance criteria 必须通过或有人工 override；
- upstream revision_needed/blocked/failed/cancelled 不释放 downstream；
- downstream ready 后进入 Phase 3 PreDispatchGate，而不是直接 Runtime 分派。

依赖释放不得通过读取自由文本 summary 判断。

## 8. Revision Loop

### Task-level revision

用于任务契约仍成立，只需补做、修正或补证的情况。

方式：

- 同 ProjectTask 创建新 attempt；或
- 追加 `revision_of_task_id` 的 ProjectTask。

选择规则：

- 输入、目标、员工、权限和验收标准基本不变：同 task 新 attempt；
- 需要换员工、换目标、换权限、换依赖或换验收标准：追加 revision task。

### Plan-level replan

用于原计划整体不再成立：

- 用户需求变化；
- 发现新的根因路径；
- 原任务拆解错误；
- 多个关键任务失败；
- 权限或环境约束改变；
- 人工要求重新规划。

处理规则：

1. 创建 replan decision 或 recovery request。
2. 加载当前 ProjectTask 状态、结果、blocker、artifacts 和 human decisions。
3. 生成新的 PlanRevision。
4. 新 revision 走 Phase 2 review。
5. accepted 后 append-only 分解新 task。
6. 旧 task 只能取消、完成或保留历史，不能被改写。

## 9. 人类结果验收

以下情况必须或建议进入人工 review：

- 高风险任务完成；
- acceptance criterion 未全部自动通过；
- 数字员工请求人工判断；
- 涉及业务结论、上线、删除、迁移、权限扩大；
- 多次失败后需要决定继续、取消或重规划；
- 最终 demand summary 需要负责人验收。

review 动作：

- `accept_result`
- `request_revision`
- `mark_blocked`
- `request_replan`
- `cancel_task`

所有动作必须写入 audit 和 project event。

## 10. Final Demand Summary

当任务图达到终态后，生成最终总结。

触发条件：

- 所有必须任务 completed/accepted；
- 或存在 blocked/failed/cancelled 且人类选择结束；
- 或用户取消 demand。

summary 至少包含：

- 用户原始目标；
- 最终结论；
- 已完成任务列表；
- 未完成、取消或阻塞任务；
- 关键证据和 artifact；
- 重要决策和人工确认；
- 变更内容；
- 实际验证；
- 剩余风险；
- 建议下一步。

不同任务类型的 summary 要求：

数据库分析：

- 数据范围；
- 查询或统计摘要；
- 结论；
- 不确定性；
- 证据。

系统问题诊断：

- 现象；
- 根因或候选根因；
- 验证证据；
- 缓解措施；
- 修复建议；
- 风险。

功能开发：

- 实现内容；
- 影响文件或模块；
- 测试和真实验证；
- 未完成事项；
- 发布或回滚注意点。

## 11. 审计与证据链

TaskResultContract 应与已有或规划中的 execution ledger 对齐：

- attempt started/completed/failed；
- provider session；
- tool/MCP/capability invocation；
- artifact linked；
- evidence linked；
- summary created；
- human review decision。

Result 不需要保存大日志正文，但必须保存可追溯 ref 和摘要。

## 12. 验证

单元测试：

- result payload schema validation；
- completed 缺 acceptance result 时失败；
- revision_needed 创建 revision path；
- blocked 创建 blocker/human request；
- failed 按 retryable policy 处理；
- completed result 释放 downstream；
- human review 未接受时不释放 downstream；
- final summary 聚合正确状态。

集成测试：

- Runtime complete writeback 存储结构化结果；
- upstream completed 后 downstream 进入 PreDispatchGate；
- failed criterion 触发 revision 或 human review；
- append-only replan 保留旧 task；
- 终态任务图生成 final summary。

真实 smoke：

- 创建至少两个有依赖的 task；
- 第一个 task 真实 Runtime/Provider 完成并写回 result；
- Control Plane 根据 result 释放第二个 task；
- 第二个 task 完成后生成 final summary；
- summary 包含 evidence refs、task statuses 和风险。

## 13. 验收标准

- Runtime/Provider 写回结果是结构化、可校验的。
- Control Plane 不依赖自由文本日志释放下游。
- revision_needed、blocked、failed 都有明确恢复路径。
- 人类 review 能介入结果验收和重规划决策。
- plan-level replan 采用 append-only，不改写旧任务历史。
- final demand summary 能解释做了什么、证据是什么、还剩什么风险。
