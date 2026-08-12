-- Bind vs provision separation (spec 2026-08-12 P1).
-- Existing rows are already mkdir/cloned on disk → default provisioned.

ALTER TABLE project_runtime_nodes
    ADD COLUMN IF NOT EXISTS provision_status VARCHAR(24) NOT NULL DEFAULT 'provisioned',
    ADD COLUMN IF NOT EXISTS provisioned_at   TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS provision_source VARCHAR(24);

-- Stamp backfill for rows that predate the column (default status already provisioned).
UPDATE project_runtime_nodes
SET provisioned_at = COALESCE(provisioned_at, created_at),
    provision_source = COALESCE(provision_source, 'legacy_backfill')
WHERE provision_status = 'provisioned'
  AND provisioned_at IS NULL;

ALTER TABLE project_runtime_nodes
    DROP CONSTRAINT IF EXISTS chk_project_runtime_nodes_provision_status;
ALTER TABLE project_runtime_nodes
    ADD CONSTRAINT chk_project_runtime_nodes_provision_status
    CHECK (provision_status IN ('unprovisioned', 'provisioned'));

COMMENT ON COLUMN project_runtime_nodes.provision_status IS
  'unprovisioned = bound candidate only; provisioned = disk ready for dispatch';
COMMENT ON COLUMN project_runtime_nodes.provision_source IS
  'How provision happened: create | confirm | attach_probe | legacy_backfill';
