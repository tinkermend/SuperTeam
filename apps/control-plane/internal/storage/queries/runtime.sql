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
)
ON CONFLICT (node_id) DO UPDATE SET
    name = EXCLUDED.name,
    supported_providers = EXCLUDED.supported_providers,
    max_slots = EXCLUDED.max_slots,
    current_load = EXCLUDED.current_load,
    status = EXCLUDED.status,
    metadata = EXCLUDED.metadata,
    last_heartbeat_at = EXCLUDED.last_heartbeat_at,
    disabled_at = NULL,
    archived_at = NULL,
    updated_at = NOW()
RETURNING *;

-- name: GetRuntimeNode :one
SELECT * FROM runtime_nodes
WHERE node_id = $1
  AND archived_at IS NULL;

-- name: UpdateRuntimeNodeHeartbeat :one
UPDATE runtime_nodes
SET last_heartbeat_at = $2,
    disabled_at = NULL,
    updated_at = NOW()
WHERE node_id = $1
  AND archived_at IS NULL
RETURNING *;

-- name: UpdateRuntimeNodeLoad :one
UPDATE runtime_nodes
SET current_load = $2, updated_at = NOW()
WHERE node_id = $1
  AND archived_at IS NULL
RETURNING *;

-- name: UpdateRuntimeNodeStatus :one
UPDATE runtime_nodes
SET status = $2,
    disabled_at = CASE
        WHEN $2::varchar = 'offline' THEN COALESCE(disabled_at, NOW())
        WHEN $2::varchar = 'online' THEN NULL
        ELSE disabled_at
    END,
    updated_at = NOW()
WHERE node_id = $1
  AND archived_at IS NULL
RETURNING *;

-- name: ListOnlineRuntimeNodes :many
SELECT * FROM runtime_nodes
WHERE status = 'online'
  AND last_heartbeat_at > $1
  AND disabled_at IS NULL
  AND archived_at IS NULL
ORDER BY current_load ASC, created_at ASC;

-- name: ListOnlineNodes :many
SELECT * FROM runtime_nodes
WHERE status = 'online'
  AND last_heartbeat_at > $1
  AND disabled_at IS NULL
  AND archived_at IS NULL
ORDER BY current_load ASC, created_at ASC;

-- name: ListRuntimeNodes :many
SELECT * FROM runtime_nodes
WHERE archived_at IS NULL
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status'))
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: DeleteRuntimeNode :exec
UPDATE runtime_nodes
SET status = 'offline',
    disabled_at = COALESCE(disabled_at, NOW()),
    archived_at = COALESCE(archived_at, NOW()),
    updated_at = NOW()
WHERE node_id = $1;
