-- Audit Queries

-- name: CreateAuditEvent :one
INSERT INTO audit_events (
    event_type,
    actor_type,
    actor_id,
    resource_type,
    resource_id,
    action,
    details,
    ip_address
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: GetAuditEvent :one
SELECT * FROM audit_events
WHERE id = $1;

-- name: ListAuditEvents :many
SELECT * FROM audit_events
WHERE ($1::varchar IS NULL OR event_type = $1)
  AND ($2::varchar IS NULL OR actor_type = $2)
  AND ($3::varchar IS NULL OR actor_id = $3)
  AND ($4::varchar IS NULL OR resource_type = $4)
  AND ($5::varchar IS NULL OR resource_id = $5)
  AND ($6::timestamptz IS NULL OR created_at >= $6)
  AND ($7::timestamptz IS NULL OR created_at <= $7)
ORDER BY created_at DESC
LIMIT $8 OFFSET $9;

-- name: ListAuditEventsByActor :many
SELECT * FROM audit_events
WHERE actor_type = $1
  AND actor_id = $2
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: ListAuditEventsByResource :many
SELECT * FROM audit_events
WHERE resource_type = $1
  AND resource_id = $2
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: CountAuditEvents :one
SELECT COUNT(*) FROM audit_events
WHERE ($1::varchar IS NULL OR event_type = $1)
  AND ($2::varchar IS NULL OR actor_type = $2)
  AND ($3::varchar IS NULL OR actor_id = $3)
  AND ($4::varchar IS NULL OR resource_type = $4)
  AND ($5::varchar IS NULL OR resource_id = $5)
  AND ($6::timestamptz IS NULL OR created_at >= $6)
  AND ($7::timestamptz IS NULL OR created_at <= $7);

-- name: DeleteOldAuditEvents :exec
DELETE FROM audit_events
WHERE created_at < $1;
