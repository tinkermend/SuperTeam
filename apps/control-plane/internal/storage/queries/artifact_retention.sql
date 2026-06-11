-- name: CreateArtifactRetentionHold :one
INSERT INTO artifact_retention_holds (
    tenant_id,
    artifact_id,
    hold_type,
    resource_type,
    resource_id,
    reason,
    status,
    created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('artifact_id')::uuid,
    sqlc.arg('hold_type')::varchar,
    sqlc.arg('resource_type')::varchar,
    sqlc.arg('resource_id')::uuid,
    sqlc.narg('reason')::text,
    sqlc.arg('status')::varchar,
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: ListArtifactRetentionHolds :many
SELECT * FROM artifact_retention_holds
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND artifact_id = sqlc.arg('artifact_id')::uuid
  AND released_at IS NULL
ORDER BY created_at DESC;

-- name: CountActiveArtifactRetentionHolds :one
SELECT COUNT(*)::integer AS active_hold_count
FROM artifact_retention_holds
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND artifact_id = sqlc.arg('artifact_id')::uuid
  AND status = 'active'
  AND released_at IS NULL;
