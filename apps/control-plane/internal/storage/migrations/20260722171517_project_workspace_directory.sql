-- 项目目录落地(spec 2026-07-23):项目名=目录名全局唯一;首启工作区就绪态与主节点。
-- 存量同名项目先消歧(追加短 id 后缀)再建全局唯一索引,避免迁移因历史重名失败。

UPDATE projects AS p
SET name = left(p.name, 246) || '-' || substr(replace(p.id::text, '-', ''), 1, 8),
    updated_at = NOW()
WHERE p.deleted_at IS NULL
  AND EXISTS (
      SELECT 1
      FROM projects AS o
      WHERE o.deleted_at IS NULL
        AND o.name = p.name
        AND (o.created_at, o.id) < (p.created_at, p.id)
  );

CREATE UNIQUE INDEX uq_projects_name_active
    ON projects (name)
    WHERE deleted_at IS NULL;

ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS workspace_ready_status VARCHAR(32) NOT NULL DEFAULT 'ready',
    ADD COLUMN IF NOT EXISTS primary_runtime_node_id UUID,
    ADD COLUMN IF NOT EXISTS workspace_ready_error TEXT,
    ADD COLUMN IF NOT EXISTS workspace_ready_at TIMESTAMPTZ;

ALTER TABLE projects
    DROP CONSTRAINT IF EXISTS chk_projects_workspace_ready_status;

ALTER TABLE projects
    ADD CONSTRAINT chk_projects_workspace_ready_status
    CHECK (workspace_ready_status IN ('pending', 'ready', 'error'));

COMMENT ON COLUMN projects.workspace_ready_status IS '工作区首启就绪:pending|ready|error;未就绪只挡派发';
COMMENT ON COLUMN projects.primary_runtime_node_id IS '高亲和主 Runtime 节点(首个可用/clone 成功);可空';
COMMENT ON COLUMN projects.workspace_ready_error IS '最近一次工作区供给失败原因(供人工处理)';
COMMENT ON COLUMN projects.workspace_ready_at IS '首次进入 ready 的时间';
COMMENT ON INDEX uq_projects_name_active IS '项目目录名(=项目名)全局唯一(未软删行)';
