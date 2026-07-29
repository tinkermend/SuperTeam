-- 回填：清掉终态任务上残留的等待指针。
--
-- waiting_reason / waiting_request_id 只描述"当前在等什么"。四条"回活跃"的查询
-- （QueueProjectTask / ScheduleProjectTaskRetry / ScheduleProjectTaskDispatchRetry /
-- ReleaseProjectTaskWaitingHumanForRedispatch）一直会清它们，但进终态的
-- UpdateProjectTaskStatus 历史上不清，于是已完成/已取消的任务永久带着上一次等待的
-- 决策 id，投影出"已终态却又在等某个决策"的自相矛盾状态。写侧已在同批修复，这里
-- 把存量清干净，让该列只有一种含义。
--
-- 人类决策溯源不依赖该列：结项摘要（buildFinalDemandSummaryPayload）已改为从
-- project_decision_requests 取，先拆依赖再清列，不会丢审计痕迹。
UPDATE project_tasks
SET waiting_reason = NULL,
    waiting_request_id = NULL
WHERE status IN ('completed', 'done', 'success', 'cancelled', 'failed')
  AND (waiting_reason IS NOT NULL OR waiting_request_id IS NOT NULL);
