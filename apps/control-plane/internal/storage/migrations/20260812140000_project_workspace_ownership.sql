-- Workspace ownership (spec 2026-08-12 P2): platform_managed | attached.
ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS workspace_ownership VARCHAR(24) NOT NULL DEFAULT 'platform_managed';

ALTER TABLE projects
    DROP CONSTRAINT IF EXISTS chk_projects_workspace_ownership;
ALTER TABLE projects
    ADD CONSTRAINT chk_projects_workspace_ownership
    CHECK (workspace_ownership IN ('platform_managed', 'attached'));

COMMENT ON COLUMN projects.workspace_ownership IS
  'platform_managed = platform mkdir/clone; attached = claimed existing dir under workspace root';
