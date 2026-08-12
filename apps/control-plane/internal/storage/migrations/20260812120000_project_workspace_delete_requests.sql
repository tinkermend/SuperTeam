-- Project workspace delete confirmation queue
-- (spec 2026-08-12-project-workspace-provisioning-model §5.5 / P0).
-- Disk directory removal is deferred: DeleteProject / RemoveProjectRuntimeNode
-- enqueue rows here; admin confirm dispatches remove_project_directory;
-- reject hands off (audit only, no hanging orphans).

CREATE TABLE IF NOT EXISTS project_workspace_delete_requests (
    id               UUID PRIMARY KEY,
    tenant_id        UUID        NOT NULL,
    project_id       UUID        NOT NULL,
    runtime_node_id  UUID        NOT NULL,
    -- Snapshots: project may already be soft-deleted; do not join live rows.
    directory_name   VARCHAR(64) NOT NULL,
    node_id_snapshot TEXT        NOT NULL,
    ownership        VARCHAR(24) NOT NULL,
    repo_summary     JSONB,
    status           VARCHAR(24) NOT NULL,
    requested_by     UUID        NOT NULL,
    requested_at     TIMESTAMPTZ NOT NULL,
    resolved_by      UUID,
    resolved_at      TIMESTAMPTZ,
    reason           TEXT,
    CONSTRAINT chk_pwdr_status CHECK (status IN ('pending', 'confirmed', 'rejected')),
    CONSTRAINT chk_pwdr_ownership CHECK (ownership IN ('platform_managed', 'attached'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_pwdr_pending
    ON project_workspace_delete_requests (project_id, runtime_node_id)
    WHERE status = 'pending';

-- Directory-name occupancy check while pending disk cleanup (§5.5).
CREATE INDEX IF NOT EXISTS ix_pwdr_dirname_pending
    ON project_workspace_delete_requests (directory_name)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS ix_pwdr_tenant_pending
    ON project_workspace_delete_requests (tenant_id, requested_at)
    WHERE status = 'pending';

COMMENT ON TABLE project_workspace_delete_requests IS
  'Per-(project, runtime node) workspace directory delete confirmation queue. Confirm → remove_project_directory; reject → platform hands off.';
COMMENT ON COLUMN project_workspace_delete_requests.directory_name IS
  'Snapshot of projects.directory_name at enqueue time.';
COMMENT ON COLUMN project_workspace_delete_requests.node_id_snapshot IS
  'Snapshot of runtime_nodes.node_id at enqueue time (string node identity).';
COMMENT ON COLUMN project_workspace_delete_requests.ownership IS
  'Snapshot of workspace ownership for admin judgment (platform_managed | attached).';
