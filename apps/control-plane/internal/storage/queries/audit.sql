-- name: CreateAuditEvent :one
INSERT INTO audit_events (
    tenant_id,
    event_type,
    actor_type,
    actor_id,
    resource_type,
    resource_id,
    action,
    details,
    ip_address
) VALUES (
    COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid),
    sqlc.arg('event_type')::varchar,
    sqlc.arg('actor_type')::varchar,
    sqlc.arg('actor_id')::varchar,
    sqlc.narg('resource_type')::varchar,
    sqlc.narg('resource_id')::varchar,
    sqlc.arg('action')::varchar,
    sqlc.narg('details')::jsonb,
    sqlc.narg('ip_address')::inet
) RETURNING *;

-- name: GetAuditEvent :one
SELECT * FROM audit_events
WHERE id = $1;

-- name: ListAuditEvents :many
SELECT * FROM audit_events
WHERE (sqlc.narg('event_type')::varchar IS NULL OR event_type = sqlc.narg('event_type'))
  AND (sqlc.narg('actor_type')::varchar IS NULL OR actor_type = sqlc.narg('actor_type'))
  AND (sqlc.narg('actor_id')::varchar IS NULL OR actor_id = sqlc.narg('actor_id'))
  AND (sqlc.narg('resource_type')::varchar IS NULL OR resource_type = sqlc.narg('resource_type'))
  AND (sqlc.narg('resource_id')::varchar IS NULL OR resource_id = sqlc.narg('resource_id'))
  AND (sqlc.narg('start_time')::timestamptz IS NULL OR created_at >= sqlc.narg('start_time'))
  AND (sqlc.narg('end_time')::timestamptz IS NULL OR created_at <= sqlc.narg('end_time'))
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountAuditEvents :one
SELECT COUNT(*) FROM audit_events
WHERE (sqlc.narg('event_type')::varchar IS NULL OR event_type = sqlc.narg('event_type'))
  AND (sqlc.narg('actor_type')::varchar IS NULL OR actor_type = sqlc.narg('actor_type'))
  AND (sqlc.narg('actor_id')::varchar IS NULL OR actor_id = sqlc.narg('actor_id'))
  AND (sqlc.narg('resource_type')::varchar IS NULL OR resource_type = sqlc.narg('resource_type'))
  AND (sqlc.narg('resource_id')::varchar IS NULL OR resource_id = sqlc.narg('resource_id'))
  AND (sqlc.narg('start_time')::timestamptz IS NULL OR created_at >= sqlc.narg('start_time'))
  AND (sqlc.narg('end_time')::timestamptz IS NULL OR created_at <= sqlc.narg('end_time'));
