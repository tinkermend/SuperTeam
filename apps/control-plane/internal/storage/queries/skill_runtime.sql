-- name: ListRequiredToolsForNode :many
WITH target_node AS (
    SELECT id
    FROM runtime_nodes
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND node_id = sqlc.arg('node_id')::varchar
      AND archived_at IS NULL
),
mounted_skills AS (
    SELECT s.metadata
    FROM target_node tn
    JOIN digital_employee_execution_instances dei
      ON dei.runtime_node_id = tn.id
     AND dei.tenant_id = sqlc.arg('tenant_id')::uuid
     AND dei.deleted_at IS NULL
     AND dei.status IN ('provisioning', 'ready', 'active')
    JOIN skill_agent_bindings sab
      ON sab.tenant_id = dei.tenant_id
     AND sab.digital_employee_id = dei.digital_employee_id
     AND sab.status = 'enabled'
    JOIN skills s
      ON s.tenant_id = sab.tenant_id
     AND s.id = sab.skill_id
     AND s.deleted_at IS NULL
    UNION
    SELECT s.metadata
    FROM target_node tn
    JOIN digital_employee_execution_instances dei
      ON dei.runtime_node_id = tn.id
     AND dei.tenant_id = sqlc.arg('tenant_id')::uuid
     AND dei.deleted_at IS NULL
     AND dei.status IN ('provisioning', 'ready', 'active')
    JOIN digital_employees de
      ON de.tenant_id = dei.tenant_id
     AND de.id = dei.digital_employee_id
     AND de.deleted_at IS NULL
    JOIN team_skill_bindings stb
      ON stb.tenant_id = de.tenant_id
     AND stb.team_id = de.team_id
    JOIN skills s
      ON s.tenant_id = stb.tenant_id
     AND s.id = stb.skill_id
     AND s.deleted_at IS NULL
)
SELECT DISTINCT tool::text
FROM mounted_skills ms,
     LATERAL jsonb_array_elements_text(
        COALESCE(ms.metadata->'runtime_dependencies'->'tools', '[]'::jsonb)
     ) AS tool
WHERE btrim(tool) <> ''
ORDER BY tool;
