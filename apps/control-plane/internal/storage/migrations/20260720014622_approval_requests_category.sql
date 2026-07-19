-- Separate 权限审批 (permission center) from 项目任务验收 (inbox) on the shared
-- approval_requests fact table. Existing rows backfill to 'project_task' via the
-- NOT NULL DEFAULT so the inbox projection keeps seeing them unchanged.
ALTER TABLE approval_requests
    ADD COLUMN category VARCHAR(50) NOT NULL DEFAULT 'project_task';

COMMENT ON COLUMN approval_requests.category IS '审批分类:permission(操作权限审批,走权限中心,不投影收件箱) | project_task(项目任务/验收,走收件箱);存量回填 project_task';

-- Permission-center read path filters by (tenant, category, status) ordered by recency.
CREATE INDEX idx_approval_requests_category_status
    ON approval_requests(tenant_id, category, status, created_at DESC);
