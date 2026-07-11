ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

COMMENT ON COLUMN projects.deleted_at IS '软删除时间；非空表示项目已从当前管理面移除';

CREATE INDEX IF NOT EXISTS idx_projects_tenant_deleted_created
    ON projects (tenant_id, created_at DESC)
    WHERE deleted_at IS NULL;
