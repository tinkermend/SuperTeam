-- 拆分展示名(name)与目录名(directory_name);按产品决定清空平台存量项目关联行。

-- ── 1) 级联清理全部未删除项目关联数据(对齐 SoftDeleteProjectCascade 语义) ──
UPDATE project_tasks
SET status = 'cancelled',
    updated_at = NOW()
WHERE status NOT IN ('completed', 'failed', 'cancelled', 'done', 'success')
  AND project_id IN (SELECT id FROM projects WHERE deleted_at IS NULL);

UPDATE project_decision_requests
SET status_snapshot = 'cancelled',
    resolved_at = COALESCE(resolved_at, NOW()),
    updated_at = NOW()
WHERE status_snapshot IN ('pending', 'requested')
  AND project_id IN (SELECT id FROM projects WHERE deleted_at IS NULL);

UPDATE approval_requests ar
SET status = 'cancelled',
    resolved_at = COALESCE(ar.resolved_at, NOW()),
    updated_at = NOW()
FROM project_decision_requests pdr
WHERE ar.id = pdr.approval_request_id
  AND ar.status = 'pending'
  AND pdr.project_id IN (SELECT id FROM projects WHERE deleted_at IS NULL);

UPDATE inbox_items
SET status = 'cancelled',
    resolved_at = COALESCE(resolved_at, NOW()),
    updated_at = NOW()
WHERE status = 'open'
  AND source_project_id IN (SELECT id FROM projects WHERE deleted_at IS NULL);

UPDATE project_members
SET status = 'removed',
    updated_at = NOW()
WHERE status = 'active'
  AND project_id IN (SELECT id FROM projects WHERE deleted_at IS NULL);

DELETE FROM project_employee_node_affinity
WHERE project_id IN (SELECT id FROM projects WHERE deleted_at IS NULL);

DELETE FROM project_runtime_nodes
WHERE project_id IN (SELECT id FROM projects WHERE deleted_at IS NULL);

UPDATE automation_rules
SET enabled = FALSE,
    disabled_reason = COALESCE(disabled_reason, 'project_workspace_reset'),
    updated_at = NOW()
WHERE project_id IN (SELECT id FROM projects WHERE deleted_at IS NULL);

UPDATE projects
SET deleted_at = COALESCE(deleted_at, NOW()),
    updated_at = NOW(),
    coordination_status = 'terminated'
WHERE deleted_at IS NULL;

-- ── 2) 目录名列 + 索引从 name 迁到 directory_name ─────────────────
ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS directory_name VARCHAR(64);

COMMENT ON COLUMN projects.name IS '项目展示名称(允许中文);不作为 Runtime 目录名';
COMMENT ON COLUMN projects.directory_name IS 'Runtime 工作区相对目录名(ASCII,全局唯一);与 name 分离';

UPDATE projects
SET directory_name = 'deleted-' || substr(replace(id::text, '-', ''), 1, 12)
WHERE directory_name IS NULL OR btrim(directory_name) = '';

ALTER TABLE projects
    ALTER COLUMN directory_name SET NOT NULL;

DROP INDEX IF EXISTS uq_projects_name_active;

CREATE UNIQUE INDEX IF NOT EXISTS uq_projects_directory_name_active
    ON projects (directory_name)
    WHERE deleted_at IS NULL;

COMMENT ON INDEX uq_projects_directory_name_active IS '项目目录名全局唯一(未软删行)';
