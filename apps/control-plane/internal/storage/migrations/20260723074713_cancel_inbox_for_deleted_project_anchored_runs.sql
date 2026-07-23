-- 删项目时原只 cancel source_project_id 命中的 open 收件箱;
-- chat/standalone run 失败恢复卡常只锚在 tasks.params.metadata.anchor_project_id,
-- 导致项目软删后角标残留。补清已删项目上的这类 open 卡。

UPDATE inbox_items ii
SET status = 'cancelled',
    resolved_at = COALESCE(ii.resolved_at, NOW()),
    updated_at = NOW()
WHERE ii.status = 'open'
  AND ii.item_type = 'digital_employee_run_recovery'
  AND EXISTS (
    SELECT 1
    FROM task_runs tr
    JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
    JOIN projects p ON p.tenant_id = tr.tenant_id
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
    WHERE tr.id = ii.source_id
      AND tr.tenant_id = ii.tenant_id
  );

-- 顺带回填仍 open/resolved 的 recovery 卡 source_project_id,便于后续级联走显式列。
UPDATE inbox_items ii
SET source_project_id = COALESCE(
      ii.source_project_id,
      CASE
        WHEN (t.params #>> '{metadata,anchor_project_id}') ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
          THEN (t.params #>> '{metadata,anchor_project_id}')::uuid
        WHEN (t.params #>> '{metadata,project_id}') ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
          THEN (t.params #>> '{metadata,project_id}')::uuid
        ELSE NULL
      END
    ),
    updated_at = NOW()
FROM task_runs tr
JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
WHERE ii.item_type = 'digital_employee_run_recovery'
  AND ii.source_id = tr.id
  AND ii.tenant_id = tr.tenant_id
  AND ii.source_project_id IS NULL
  AND (
    (t.params #>> '{metadata,anchor_project_id}') ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
    OR (t.params #>> '{metadata,project_id}') ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
  );
