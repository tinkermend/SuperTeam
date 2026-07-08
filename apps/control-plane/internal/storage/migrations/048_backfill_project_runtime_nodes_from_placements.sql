-- Backfill the runtime-node eligibility set for projects that only have a
-- legacy active placement, so they don't silently become undispatchable
-- once dispatch/gate/readiness stop reading project_placements directly.
INSERT INTO project_runtime_nodes (tenant_id, project_id, runtime_node_id)
SELECT pp.tenant_id, pp.project_id, pp.runtime_node_id
FROM project_placements pp
WHERE pp.placement_status = 'active'
  AND pp.released_at IS NULL
ON CONFLICT (project_id, runtime_node_id) DO NOTHING;
