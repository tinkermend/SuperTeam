-- 删项目时原先保留 failed project_tasks、且不 ack 锚在该项目的失败 run,
-- 导致员工运行总览长期「异常」(待人工恢复)但收件箱/项目已不可达。
-- 对齐 SoftDeleteProjectCascade: cancel failed 任务 + ack 失败/超时 run。

UPDATE project_tasks pt
SET status = 'cancelled',
    updated_at = NOW()
FROM projects p
WHERE p.id = pt.project_id
  AND p.tenant_id = pt.tenant_id
  AND p.deleted_at IS NOT NULL
  AND pt.status = 'failed';

UPDATE task_runs tr
SET failure_acknowledged_at = COALESCE(tr.failure_acknowledged_at, NOW()),
    updated_at = NOW()
FROM tasks t
WHERE t.id = tr.task_id
  AND t.tenant_id = tr.tenant_id
  AND tr.status IN ('failed', 'timed_out')
  AND tr.failure_acknowledged_at IS NULL
  AND (
    EXISTS (
      SELECT 1
      FROM project_tasks pt
      JOIN projects p
        ON p.id = pt.project_id
       AND p.tenant_id = pt.tenant_id
       AND p.deleted_at IS NOT NULL
      WHERE pt.tenant_id = tr.tenant_id
        AND pt.digital_employee_run_id = tr.id
    )
    OR EXISTS (
      SELECT 1
      FROM projects p
      WHERE p.tenant_id = tr.tenant_id
        AND p.deleted_at IS NOT NULL
        AND (
          (
            (t.params #>> '{metadata,anchor_project_id}') ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
            AND (t.params #>> '{metadata,anchor_project_id}')::uuid = p.id
          )
          OR (
            (t.params #>> '{metadata,project_id}') ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
            AND (t.params #>> '{metadata,project_id}')::uuid = p.id
          )
        )
    )
  );
