-- name: CreateRuntimeNode :one
INSERT INTO runtime_nodes (
    node_id,
    name,
    supported_providers,
    max_slots,
    current_load,
    status,
    metadata,
    last_heartbeat_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: GetRuntimeNode :one
SELECT * FROM runtime_nodes
WHERE node_id = $1;

-- name: UpdateRuntimeNodeHeartbeat :one
UPDATE runtime_nodes
SET last_heartbeat_at = $2, updated_at = NOW()
WHERE node_id = $1
RETURNING *;

-- name: UpdateRuntimeNodeLoad :one
UPDATE runtime_nodes
SET current_load = $2, updated_at = NOW()
WHERE node_id = $1
RETURNING *;

-- name: UpdateRuntimeNodeStatus :one
UPDATE runtime_nodes
SET status = $2, updated_at = NOW()
WHERE node_id = $1
RETURNING *;

-- name: ListOnlineNodes :many
SELECT * FROM runtime_nodes
WHERE status = 'online'
  AND last_heartbeat_at > $1
ORDER BY current_load ASC, created_at ASC;

-- name: ListRuntimeNodes :many
SELECT * FROM runtime_nodes
WHERE ($1::varchar IS NULL OR status = $1)
ORDER BY created_at DESC
LIMIT $3 OFFSET $2;

-- name: DeleteRuntimeNode :exec
DELETE FROM runtime_nodes
WHERE node_id = $1;
