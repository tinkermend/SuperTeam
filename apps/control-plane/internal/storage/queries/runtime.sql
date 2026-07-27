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
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('node_id')::varchar,
    sqlc.arg('name')::varchar,
    sqlc.arg('supported_providers')::jsonb,
    sqlc.arg('max_slots')::integer,
    sqlc.arg('current_load')::integer,
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb),
    sqlc.arg('last_heartbeat_at')::timestamptz
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
WHERE runtime_nodes.tenant_id = EXCLUDED.tenant_id
RETURNING *;

-- name: GetRuntimeNode :one
SELECT * FROM runtime_nodes
WHERE node_id = $1
  AND archived_at IS NULL;

-- name: GetRuntimeNodeByID :one
SELECT * FROM runtime_nodes
WHERE id = sqlc.arg('id')::uuid
  AND archived_at IS NULL;

-- name: UpdateRuntimeNodeHeartbeat :one
UPDATE runtime_nodes
SET last_heartbeat_at = $2,
    disabled_at = NULL,
    updated_at = NOW()
WHERE node_id = $1
  AND archived_at IS NULL
RETURNING *;

-- ApplyRuntimeNodeHeartbeat folds the hot heartbeat path into one row write:
-- last_seen + reported load + force-online. Callers that only need a pure
-- last_seen bump (enrollment reconnect) still use UpdateRuntimeNodeHeartbeat.
-- name: ApplyRuntimeNodeHeartbeat :one
UPDATE runtime_nodes
SET last_heartbeat_at = sqlc.arg('last_heartbeat_at')::timestamptz,
    current_load = sqlc.arg('current_load'),
    status = 'online',
    disabled_at = NULL,
    updated_at = NOW()
WHERE node_id = sqlc.arg('node_id')::varchar
  AND archived_at IS NULL
RETURNING *;

-- PatchRuntimeNodeMetadata merges a JSON object into node metadata (jsonb ||).
-- Used by heartbeat to persist capability self-reports (e.g.
-- supports_platform_limits) only when the value changes.
-- name: PatchRuntimeNodeMetadata :one
UPDATE runtime_nodes
SET metadata = COALESCE(metadata, '{}'::jsonb) || sqlc.arg('patch')::jsonb,
    updated_at = NOW()
WHERE node_id = sqlc.arg('node_id')::varchar
  AND archived_at IS NULL
RETURNING *;

-- CountOnlineLegacyLimitRuntimeNodesForTenant counts online nodes that have
-- not self-reported supports_platform_limits. The artifact presign version-skew
-- guard clamps the file-size cap while any such node is online.
-- name: CountOnlineLegacyLimitRuntimeNodesForTenant :one
SELECT COUNT(*)::bigint
FROM runtime_nodes
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND status = 'online'
  AND last_heartbeat_at > sqlc.arg('last_heartbeat_at')::timestamptz
  AND disabled_at IS NULL
  AND archived_at IS NULL
  AND COALESCE(metadata->>'supports_platform_limits', '') <> 'true';

-- name: UpdateRuntimeNodeLoad :one
UPDATE runtime_nodes
SET current_load = $2, updated_at = NOW()
WHERE node_id = $1
  AND archived_at IS NULL
RETURNING *;

-- TryAcquireRuntimeNodeSlot atomically reserves one execution slot on a node.
-- The capacity guard (current_load < max_slots) and liveness guards
-- (status = 'online', fresh heartbeat) live inside the same UPDATE statement,
-- so concurrent acquires serialize on the row lock and PostgreSQL re-evaluates
-- the WHERE clause against the latest row version. Returns no rows when the
-- node is full, offline, stale, or archived; callers must treat pgx.ErrNoRows
-- as "slot unavailable" and try the next candidate.
-- name: TryAcquireRuntimeNodeSlot :one
UPDATE runtime_nodes
SET current_load = current_load + 1,
    updated_at = NOW()
WHERE node_id = $1
  AND archived_at IS NULL
  AND disabled_at IS NULL
  AND status = 'online'
  AND last_heartbeat_at > $2
  AND current_load < max_slots
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

-- name: ListRuntimeNodesForTenant :many
SELECT * FROM runtime_nodes
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)
  AND archived_at IS NULL
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: DeleteRuntimeNode :exec
UPDATE runtime_nodes
SET status = 'offline',
    disabled_at = COALESCE(disabled_at, NOW()),
    archived_at = COALESCE(archived_at, NOW()),
    updated_at = NOW()
WHERE node_id = $1;

-- name: CountRuntimeNodesForTenant :one
SELECT COUNT(*)::bigint
FROM runtime_nodes
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND archived_at IS NULL;

-- name: CountOnlineRuntimeNodesForTenant :one
SELECT COUNT(*)::bigint
FROM runtime_nodes
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND status = 'online'
  AND last_heartbeat_at > sqlc.arg('last_heartbeat_at')::timestamptz
  AND disabled_at IS NULL
  AND archived_at IS NULL;
