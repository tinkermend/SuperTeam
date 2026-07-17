-- review_gate 占位 verdict（P1.1 竞态修复）：被审任务完成写回路径同步写入 verdict='pending' 的
-- review_gate 聚合行（judge_type=review_gate, project_task_id 为空，命中迁移 073 的
-- uq_demand_verdicts_review_gate），让收敛闸在异步检测器（~13s LLM 调用）出结论前先 HOLD 住需求；
-- 检测器随后把同一行 upsert 成 satisfied（放行）或 unsatisfied（留人类）。无 DDL，仅更新列注释以
-- 保持 verdict 取值口径准确。

COMMENT ON COLUMN demand_criterion_verdicts.verdict IS '判定结论：satisfied | unsatisfied | not_applicable | pending（not_applicable 仅由 executor 对 automated_test 判据投影；pending 仅 review_gate 聚合行——完成时同步占位，检测器出结论前保持 HOLD）';
