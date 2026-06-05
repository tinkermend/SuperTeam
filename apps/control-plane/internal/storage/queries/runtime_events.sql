-- name: CreateRuntimeEvent :one
INSERT INTO runtime_events (
    tenant_id,
    runtime_node_id,
    node_id,
    event_type,
    severity,
    source,
    title,
    description,
    provider_type,
    correlation_type,
    correlation_id,
    payload
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.narg('runtime_node_id')::uuid,
    sqlc.narg('node_id')::varchar,
    sqlc.arg('event_type')::varchar,
    sqlc.arg('severity')::varchar,
    sqlc.arg('source')::varchar,
    sqlc.arg('title')::varchar,
    sqlc.narg('description')::text,
    sqlc.narg('provider_type')::varchar,
    sqlc.narg('correlation_type')::varchar,
    sqlc.narg('correlation_id')::varchar,
    COALESCE(sqlc.arg('payload')::jsonb, '{}'::jsonb)
) RETURNING *;

-- name: ListRuntimeEvents :many
SELECT *
FROM runtime_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND (sqlc.narg('event_type')::varchar IS NULL OR event_type = sqlc.narg('event_type')::varchar)
  AND (sqlc.narg('severity')::varchar IS NULL OR severity = sqlc.narg('severity')::varchar)
  AND (sqlc.narg('node_id')::varchar IS NULL OR node_id = sqlc.narg('node_id')::varchar)
  AND (sqlc.narg('provider_type')::varchar IS NULL OR provider_type = sqlc.narg('provider_type')::varchar)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountBlockedRuntimeEventsSince :one
SELECT COUNT(*)::bigint
FROM runtime_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND created_at >= sqlc.arg('created_since')::timestamptz
  AND (
      severity IN ('warning', 'error')
      OR event_type IN ('capability_degraded', 'command_failed', 'command_cancelled', 'command_timed_out')
  );

-- name: CountActiveProviderSessionsForTenant :one
SELECT COUNT(*)::bigint
FROM provider_sessions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND status IN ('running', 'active', 'idle')
  AND closed_at IS NULL;

-- name: ListRuntimeProviderCapabilitiesForTenant :many
SELECT
    rc.provider_type,
    COUNT(DISTINCT rc.runtime_node_id)::bigint AS node_count,
    COUNT(*) FILTER (WHERE rc.available = true)::bigint AS available_count,
    COUNT(*) FILTER (WHERE rc.health_status IN ('healthy', 'ok'))::bigint AS healthy_count,
    MAX(rc.last_seen_at)::timestamptz AS last_seen_at
FROM runtime_capabilities rc
JOIN runtime_nodes rn
  ON rn.id = rc.runtime_node_id
 AND rn.tenant_id = rc.tenant_id
 AND rn.archived_at IS NULL
WHERE rc.tenant_id = sqlc.arg('tenant_id')::uuid
  AND rc.capability_type = 'provider'
  AND rc.archived_at IS NULL
GROUP BY rc.provider_type
ORDER BY rc.provider_type ASC;

-- name: ListRuntimeCapabilitiesForNode :many
SELECT rc.*
FROM runtime_capabilities rc
JOIN runtime_nodes rn
  ON rn.id = rc.runtime_node_id
 AND rn.tenant_id = rc.tenant_id
 AND rn.archived_at IS NULL
WHERE rc.tenant_id = sqlc.arg('tenant_id')::uuid
  AND rn.node_id = sqlc.arg('node_id')::varchar
  AND rc.capability_type = 'provider'
  AND rc.archived_at IS NULL
ORDER BY rc.provider_type ASC, rc.capability_key ASC;
