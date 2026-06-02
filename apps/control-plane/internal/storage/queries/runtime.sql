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

-- name: UpsertRuntimeNodeForTenant :one
WITH updated AS (
    UPDATE runtime_nodes
    SET name = sqlc.arg('name')::varchar,
        supported_providers = sqlc.arg('supported_providers')::jsonb,
        max_slots = sqlc.arg('max_slots')::integer,
        current_load = sqlc.arg('current_load')::integer,
        status = sqlc.arg('status')::varchar,
        metadata = COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb),
        last_heartbeat_at = sqlc.arg('last_heartbeat_at')::timestamptz,
        disabled_at = NULL,
        archived_at = NULL,
        updated_at = NOW()
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND node_id = sqlc.arg('node_id')::varchar
    RETURNING *
),
inserted AS (
    INSERT INTO runtime_nodes (
        tenant_id,
        node_id,
        name,
        supported_providers,
        max_slots,
        current_load,
        status,
        metadata,
        last_heartbeat_at
    )
    SELECT
        sqlc.arg('tenant_id')::uuid,
        sqlc.arg('node_id')::varchar,
        sqlc.arg('name')::varchar,
        sqlc.arg('supported_providers')::jsonb,
        sqlc.arg('max_slots')::integer,
        sqlc.arg('current_load')::integer,
        sqlc.arg('status')::varchar,
        COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb),
        sqlc.arg('last_heartbeat_at')::timestamptz
    WHERE NOT EXISTS (
        SELECT 1
        FROM runtime_nodes
        WHERE node_id = sqlc.arg('node_id')::varchar
    )
    RETURNING *
)
SELECT * FROM updated
UNION ALL
SELECT * FROM inserted
LIMIT 1;

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
