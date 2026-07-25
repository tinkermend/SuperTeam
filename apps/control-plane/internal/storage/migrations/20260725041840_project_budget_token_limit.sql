-- 项目 token 预算额度(P1-A 简化版预算熔断)。
--
-- 只加一列:项目的 token 上限。可空,NULL = 不限(现状,对存量项目零影响)。
-- 「已消耗」不落新列——由 project_task_attempts.budget_consumed_tokens(runtime 心跳
-- 实时累加)按项目求和得出,无需 ledger。
--
-- 熔断只有一道闸,在派发前(RunPreDispatchGate):项目已消耗 >= 额度时挡下"开新工",
-- 已在跑的 attempt 不受影响(闸在派发前)。见 spec 2026-07-25-p1-platform-hardening §3。

ALTER TABLE projects ADD COLUMN budget_token_limit BIGINT;

COMMENT ON COLUMN projects.budget_token_limit IS
    '项目 token 预算上限;NULL 表示不限。已消耗达到此值后,派发前闸阻止开启新任务(运行中任务不打断)。';
