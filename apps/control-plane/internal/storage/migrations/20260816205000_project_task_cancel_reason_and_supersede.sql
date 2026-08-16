-- 场景模板闭环 A 期：取消原因分型 + 恢复替换指针。
--
-- 现场（PulseAI 复跑 3）：开发步因工件上传失败被标 failed 后，滞留收敛看门狗把
-- review/security/commit/push 从 blocked 收成 cancelled；人类随后批重试只新建了
-- 替换任务，rewireRecoverableDependents 对终态下游直接跳过，替换任务后面挂不上
-- 任何步骤——develop#2 已 completed，需求仍永久 failed。
--
-- cancel_reason 区分「系统滞留收敛」与「人类驳回下游」：只有前者允许在重试时被
-- 复活重挂边；NULL 表示未分型，同样不复活（保守，不误活人类已判死的分支）。
ALTER TABLE project_tasks
  ADD COLUMN IF NOT EXISTS cancel_reason VARCHAR(32);

-- superseded_by_task_id 指向恢复替换任务。被取代的失败任务不再计入需求状态推导，
-- 否则 deriveDemandStatusFromTaskCounts 里旧的 failed 行会把需求永久钉在 failed。
ALTER TABLE project_tasks
  ADD COLUMN IF NOT EXISTS superseded_by_task_id UUID;

COMMENT ON COLUMN project_tasks.cancel_reason IS '取消原因：system_stranded(看门狗滞留收敛,重试时可复活) / human_reject(人类驳回下游,终态不复活);NULL 表示未分型且不复活';
COMMENT ON COLUMN project_tasks.superseded_by_task_id IS '恢复替换任务ID;非空表示本任务已被取代,不再计入需求状态推导';
