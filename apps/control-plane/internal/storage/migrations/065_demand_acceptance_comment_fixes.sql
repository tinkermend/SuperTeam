-- 修正 064 遗留的错误列注释（纯文档，不改结构）：064 已应用不可回改，此处以后续迁移覆写正确词汇。
-- verification_method / severity 举例值原文与真实词汇不符；verdict 词汇追加 not_applicable（executor 投影的第三态，非阻断）。

COMMENT ON COLUMN demand_acceptance_criteria.verification_method IS '判据的验证方式：automated_test | human_judgment';
COMMENT ON COLUMN demand_acceptance_criteria.severity IS '判据严重度：blocking | non_blocking';
COMMENT ON COLUMN demand_criterion_verdicts.verdict IS '判定结论：satisfied | unsatisfied | not_applicable（not_applicable 仅由 executor 对 automated_test 判据投影，非阻断，仍需人类兜底判据签署）';
