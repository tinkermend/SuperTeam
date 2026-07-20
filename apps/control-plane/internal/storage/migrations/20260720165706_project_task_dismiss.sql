-- 失败任务软了结(soft dismiss):人类对终态失败/取消任务标记「已清理」,
-- 从活跃视图与项目风险中隐藏,但保留 status、attempts、事件与审计。
-- 不改 status(失败真值保留),不物理删除。

ALTER TABLE project_tasks
  ADD COLUMN IF NOT EXISTS dismissed_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS dismissed_by UUID;

COMMENT ON COLUMN project_tasks.dismissed_at IS '人类了结该终态任务的时间;有值后任务从活跃视图/项目风险/员工运营态中隐藏,status 与审计不变';
COMMENT ON COLUMN project_tasks.dismissed_by IS '了结该任务的用户 ID';

CREATE INDEX IF NOT EXISTS idx_project_tasks_active_not_dismissed
  ON project_tasks (tenant_id, project_id, updated_at DESC)
  WHERE dismissed_at IS NULL;
