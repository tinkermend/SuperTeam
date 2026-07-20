-- Standalone digital-employee run failure recovery:
-- 1) task_runs 可标记「失败已确认」,运营态不再因该失败点亮异常;
-- 2) 收件箱以 run id 为 source 投影 digital_employee_run_recovery 事项。

ALTER TABLE task_runs
  ADD COLUMN IF NOT EXISTS failure_acknowledged_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS failure_acknowledged_by UUID;

COMMENT ON COLUMN task_runs.failure_acknowledged_at IS '人类确认关闭该失败 run 的时间;有值后运营态不再因该 run 点亮异常';
COMMENT ON COLUMN task_runs.failure_acknowledged_by IS '确认关闭该失败 run 的用户 ID';

CREATE INDEX IF NOT EXISTS idx_task_runs_failure_ack_pending
  ON task_runs (tenant_id, digital_employee_id, updated_at DESC)
  WHERE status IN ('failed', 'timed_out') AND failure_acknowledged_at IS NULL;
