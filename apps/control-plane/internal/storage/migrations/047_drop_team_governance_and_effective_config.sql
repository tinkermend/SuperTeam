-- Team governance revisions and the effective-config approval snapshot are
-- replaced by team baseline (bindings + constitution) composed at read time.
CREATE OR REPLACE VIEW digital_employee_runtime_readiness AS
WITH latest_runs AS (
    SELECT DISTINCT ON (tr.tenant_id, tr.digital_employee_id)
        tr.tenant_id,
        tr.digital_employee_id,
        tr.status
    FROM task_runs tr
    JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
    WHERE tr.digital_employee_id IS NOT NULL
      AND t.deleted_at IS NULL
    ORDER BY tr.tenant_id, tr.digital_employee_id, tr.updated_at DESC, tr.created_at DESC
),
employee_config_states AS (
    SELECT DISTINCT ON (decr.tenant_id, decr.digital_employee_id)
        decr.tenant_id,
        decr.digital_employee_id,
        decr.id AS effective_config_id,
        CASE
            WHEN decr.status = 'active' AND decr.archived_at IS NULL THEN 'approved'
            WHEN decr.status IN ('draft', 'pending_approval') AND decr.archived_at IS NULL THEN 'pending_approval'
            WHEN decr.status = 'archived' OR decr.archived_at IS NOT NULL THEN 'stale'
            ELSE COALESCE(NULLIF(BTRIM(decr.status), ''), 'missing')
        END::text AS governance_status
    FROM digital_employee_config_revisions decr
    ORDER BY decr.tenant_id, decr.digital_employee_id, decr.revision_number DESC, decr.updated_at DESC
),
provider_capabilities AS (
    SELECT DISTINCT ON (rc.tenant_id, rc.runtime_node_id, rc.provider_type)
        rc.tenant_id,
        rc.runtime_node_id,
        rc.provider_type,
        rc.available,
        rc.status,
        rc.health_status
    FROM runtime_capabilities rc
    WHERE rc.capability_type = 'provider'
      AND rc.disabled_at IS NULL
      AND rc.archived_at IS NULL
    ORDER BY rc.tenant_id, rc.runtime_node_id, rc.provider_type, rc.last_seen_at DESC NULLS LAST, rc.updated_at DESC
)
SELECT
    de.tenant_id,
    de.id AS digital_employee_id,
    (
        de.status IN ('ready', 'active')
        AND COALESCE(dei.status, 'missing')::text IN ('ready', 'active')
        AND ecs.effective_config_id IS NOT NULL
        AND dei.runtime_node_id IS NOT NULL
        AND rn.status = 'online'
        AND rn.disabled_at IS NULL
        AND rn.archived_at IS NULL
        AND NULLIF(BTRIM(COALESCE(dei.agent_home_dir, '')), '') IS NOT NULL
        AND COALESCE(ecs.governance_status, 'missing')::text = 'approved'
        AND COALESCE(pc.available, false)::boolean = true
        AND COALESCE(pc.status, '')::text = 'healthy'
        AND COALESCE(pc.health_status, '')::text = 'healthy'
    )::boolean AS is_runtime_ready
FROM digital_employees de
LEFT JOIN digital_employee_execution_instances dei
  ON dei.tenant_id = de.tenant_id
 AND dei.digital_employee_id = de.id
 AND dei.deleted_at IS NULL
LEFT JOIN runtime_nodes rn
  ON rn.id = dei.runtime_node_id
 AND rn.tenant_id = dei.tenant_id
LEFT JOIN provider_capabilities pc
  ON pc.tenant_id = dei.tenant_id
 AND pc.runtime_node_id = dei.runtime_node_id
 AND pc.provider_type = dei.provider_type
LEFT JOIN employee_config_states ecs
  ON ecs.tenant_id = de.tenant_id
 AND ecs.digital_employee_id = de.id
WHERE de.deleted_at IS NULL;

COMMENT ON VIEW digital_employee_runtime_readiness IS '数字员工 runtime 就绪读视图,is_runtime_ready 与 overview runnable 判定一致,供协调器执行人选择过滤';

DROP TABLE IF EXISTS digital_employee_effective_configs;
DROP TABLE IF EXISTS tenant_team_config_revisions;
