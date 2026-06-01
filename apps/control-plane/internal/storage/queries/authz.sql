-- name: GetActiveTenantMembership :one
SELECT *
FROM tenant_members
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id IS NULL
  AND principal_type = sqlc.arg('principal_type')::varchar
  AND principal_id = sqlc.arg('principal_id')::uuid
  AND status = 'active'
  AND disabled_at IS NULL
ORDER BY
  CASE role
    WHEN 'owner' THEN 1
    WHEN 'admin' THEN 2
    WHEN 'member' THEN 3
    WHEN 'viewer' THEN 4
    ELSE 5
  END
LIMIT 1;

-- name: GetActiveTeamMembership :one
SELECT *
FROM tenant_members
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND principal_type = sqlc.arg('principal_type')::varchar
  AND principal_id = sqlc.arg('principal_id')::uuid
  AND status = 'active'
  AND disabled_at IS NULL
ORDER BY
  CASE role
    WHEN 'owner' THEN 1
    WHEN 'admin' THEN 2
    WHEN 'member' THEN 3
    WHEN 'viewer' THEN 4
    ELSE 5
  END
LIMIT 1;

-- name: RuntimeNodeCoversTaskScope :one
SELECT EXISTS (
  SELECT 1
  FROM tasks t
  JOIN runtime_nodes rn ON rn.tenant_id = t.tenant_id
  JOIN runtime_node_scopes rns ON rns.runtime_node_id = rn.id
  WHERE t.id = sqlc.arg('task_id')::uuid
    AND t.tenant_id = sqlc.arg('tenant_id')::uuid
    AND t.team_id IS NOT DISTINCT FROM sqlc.narg('team_id')::uuid
    AND t.deleted_at IS NULL
    AND rn.node_id = sqlc.arg('node_id')::varchar
    AND rn.status = 'online'
    AND rn.disabled_at IS NULL
    AND rn.archived_at IS NULL
    AND rns.tenant_id = t.tenant_id
    AND rns.status = 'active'
    AND rns.disabled_at IS NULL
    AND (
      (
        rns.scope_type = 'tenant'
        AND rns.team_id IS NULL
        AND rns.scope_value = t.tenant_id::text
      )
      OR (
        rns.scope_type = 'team'
        AND t.team_id IS NOT NULL
        AND rns.team_id = t.team_id
        AND rns.scope_value = t.team_id::text
      )
    )
);
