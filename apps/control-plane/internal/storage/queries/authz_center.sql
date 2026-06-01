-- name: ListAuthzDecisions :many
SELECT *
FROM web_operation_logs
WHERE module = 'authz'
  AND (sqlc.narg('result')::varchar IS NULL OR result = sqlc.narg('result')::varchar)
  AND (sqlc.narg('action')::varchar IS NULL OR action = sqlc.narg('action')::varchar)
  AND (sqlc.narg('actor_type')::varchar IS NULL OR details->>'actor_type' = sqlc.narg('actor_type')::varchar OR details->'actor'->>'type' = sqlc.narg('actor_type')::varchar)
  AND (sqlc.narg('actor_id')::varchar IS NULL OR details->>'actor_id' = sqlc.narg('actor_id')::varchar OR details->'actor'->>'id' = sqlc.narg('actor_id')::varchar)
  AND (sqlc.narg('resource_type')::varchar IS NULL OR resource_type = sqlc.narg('resource_type')::varchar)
  AND (sqlc.narg('resource_id')::varchar IS NULL OR resource_id = sqlc.narg('resource_id')::varchar)
  AND (sqlc.narg('request_id')::varchar IS NULL OR request_id = sqlc.narg('request_id')::varchar)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountAuthzDecisionsSince :one
SELECT
  COUNT(*)::bigint AS total,
  COUNT(*) FILTER (WHERE result = 'succeeded')::bigint AS allowed,
  COUNT(*) FILTER (WHERE result = 'failed')::bigint AS denied
FROM web_operation_logs
WHERE module = 'authz'
  AND created_at >= sqlc.arg('since')::timestamptz;

-- name: ListTopDeniedAuthzActionsSince :many
SELECT action, COUNT(*)::bigint AS count
FROM web_operation_logs
WHERE module = 'authz'
  AND result = 'failed'
  AND created_at >= sqlc.arg('since')::timestamptz
GROUP BY action
ORDER BY count DESC, action ASC
LIMIT sqlc.arg('limit');

-- name: ListRuntimeNodesWithScopes :many
SELECT
  rn.id AS runtime_node_id,
  rn.tenant_id AS runtime_tenant_id,
  rn.node_id,
  rn.name,
  rn.supported_providers,
  rn.max_slots,
  rn.current_load,
  rn.status AS runtime_status,
  rn.last_heartbeat_at,
  rns.id AS scope_id,
  rns.tenant_id AS scope_tenant_id,
  rns.runtime_node_id AS scope_runtime_node_id,
  rns.team_id AS scope_team_id,
  rns.scope_type,
  rns.scope_value,
  rns.status AS scope_status,
  rns.disabled_at AS scope_disabled_at,
  rns.created_at AS scope_created_at,
  rns.updated_at AS scope_updated_at
FROM runtime_nodes rn
LEFT JOIN runtime_node_scopes rns ON rns.runtime_node_id = rn.id
WHERE rn.archived_at IS NULL
ORDER BY rn.created_at DESC, rns.created_at DESC;

-- name: CreateRuntimeNodeScope :one
INSERT INTO runtime_node_scopes (
  tenant_id,
  runtime_node_id,
  team_id,
  scope_type,
  scope_value,
  status
) SELECT
  rn.tenant_id,
  rn.id,
  input.team_id,
  input.scope_type,
  input.scope_value,
  'active'
FROM runtime_nodes rn
CROSS JOIN (
  SELECT
    sqlc.arg('tenant_id')::uuid AS tenant_id,
    sqlc.arg('runtime_node_id')::uuid AS runtime_node_id,
    sqlc.narg('team_id')::uuid AS team_id,
    sqlc.arg('scope_type')::varchar AS scope_type,
    sqlc.arg('scope_value')::varchar AS scope_value
) input
LEFT JOIN tenant_teams tt
  ON tt.id = input.team_id
 AND tt.tenant_id = input.tenant_id
WHERE rn.id = input.runtime_node_id
  AND rn.tenant_id = input.tenant_id
  AND rn.archived_at IS NULL
  AND (
    (
      input.scope_type = 'tenant'
      AND input.team_id IS NULL
      AND input.scope_value = input.tenant_id::text
    )
    OR (
      input.scope_type = 'team'
      AND input.team_id IS NOT NULL
      AND input.scope_value = input.team_id::text
      AND tt.id IS NOT NULL
    )
  )
ON CONFLICT (runtime_node_id, scope_type, scope_value) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  team_id = EXCLUDED.team_id,
  status = 'active',
  disabled_at = NULL,
  updated_at = NOW()
RETURNING *;

-- name: UpdateRuntimeNodeScopeStatus :one
UPDATE runtime_node_scopes
SET status = sqlc.arg('status')::varchar,
    disabled_at = CASE
      WHEN sqlc.arg('status')::varchar = 'disabled' THEN COALESCE(disabled_at, NOW())
      WHEN sqlc.arg('status')::varchar = 'active' THEN NULL
      ELSE disabled_at
    END,
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
RETURNING *;

-- name: ListAuthzMembers :many
WITH paged_users AS (
  SELECT
    id,
    username AS user_username,
    display_name AS user_display_name,
    email AS user_email,
    status AS account_status,
    created_at
  FROM auth_users
  WHERE deleted_at IS NULL
  ORDER BY created_at DESC, id DESC
  LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset')
)
SELECT
  pu.id AS user_id,
  pu.user_username,
  pu.user_display_name,
  pu.user_email,
  pu.account_status,
  tm.tenant_id,
  tm.team_id,
  tm.principal_type,
  tm.principal_id,
  tm.role,
  tm.status AS membership_status
FROM paged_users pu
LEFT JOIN tenant_members tm
  ON tm.principal_type = 'user'
 AND tm.principal_id = pu.id
 AND tm.disabled_at IS NULL
ORDER BY pu.created_at DESC, pu.id DESC, tm.created_at DESC, tm.id DESC;
