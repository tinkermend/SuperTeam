-- name: UpsertRuntimeProviderNativeConfig :one
INSERT INTO runtime_provider_native_configs (
    tenant_id,
    runtime_node_id,
    node_id,
    provider_type,
    config_key,
    resolved_path,
    format,
    managed_values,
    file_content_hash,
    exists_on_node,
    manageable,
    unmanageable_reason,
    source,
    node_mtime,
    snapshot_at,
    last_pulled_at,
    last_pushed_at,
    last_pushed_by
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('runtime_node_id')::uuid,
    sqlc.arg('node_id')::varchar,
    sqlc.arg('provider_type')::varchar,
    sqlc.arg('config_key')::varchar,
    sqlc.narg('resolved_path')::text,
    sqlc.arg('format')::varchar,
    COALESCE(sqlc.arg('managed_values')::jsonb, '{}'::jsonb),
    sqlc.narg('file_content_hash')::varchar,
    sqlc.arg('exists_on_node')::boolean,
    sqlc.arg('manageable')::boolean,
    sqlc.narg('unmanageable_reason')::varchar,
    sqlc.arg('source')::varchar,
    sqlc.narg('node_mtime')::timestamptz,
    COALESCE(sqlc.arg('snapshot_at')::timestamptz, NOW()),
    sqlc.narg('last_pulled_at')::timestamptz,
    sqlc.narg('last_pushed_at')::timestamptz,
    sqlc.narg('last_pushed_by')::uuid
)
ON CONFLICT (tenant_id, runtime_node_id, provider_type, config_key) DO UPDATE SET
    node_id = EXCLUDED.node_id,
    resolved_path = EXCLUDED.resolved_path,
    format = EXCLUDED.format,
    managed_values = EXCLUDED.managed_values,
    file_content_hash = EXCLUDED.file_content_hash,
    exists_on_node = EXCLUDED.exists_on_node,
    manageable = EXCLUDED.manageable,
    unmanageable_reason = EXCLUDED.unmanageable_reason,
    source = EXCLUDED.source,
    node_mtime = EXCLUDED.node_mtime,
    snapshot_at = EXCLUDED.snapshot_at,
    last_pulled_at = COALESCE(EXCLUDED.last_pulled_at, runtime_provider_native_configs.last_pulled_at),
    last_pushed_at = COALESCE(EXCLUDED.last_pushed_at, runtime_provider_native_configs.last_pushed_at),
    last_pushed_by = COALESCE(EXCLUDED.last_pushed_by, runtime_provider_native_configs.last_pushed_by),
    updated_at = NOW()
RETURNING *;

-- name: ListRuntimeProviderNativeConfigsForNode :many
SELECT *
FROM runtime_provider_native_configs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND node_id = sqlc.arg('node_id')::varchar
ORDER BY provider_type ASC, config_key ASC;

-- name: GetRuntimeProviderNativeConfig :one
SELECT *
FROM runtime_provider_native_configs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND node_id = sqlc.arg('node_id')::varchar
  AND provider_type = sqlc.arg('provider_type')::varchar
  AND config_key = sqlc.arg('config_key')::varchar;
