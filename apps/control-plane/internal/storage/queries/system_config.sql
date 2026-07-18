-- name: ListSystemConfigOverrides :many
SELECT o.*, u.display_name AS updated_by_display_name, u.username AS updated_by_username
FROM system_config_overrides o
LEFT JOIN auth_users u ON u.id = o.updated_by
WHERE o.tenant_id = sqlc.arg('tenant_id')::uuid
ORDER BY o.config_key;

-- name: GetSystemConfigOverride :one
SELECT * FROM system_config_overrides
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND config_key = sqlc.arg('config_key')::varchar;

-- name: UpsertSystemConfigOverride :one
INSERT INTO system_config_overrides (
    tenant_id,
    config_key,
    value,
    updated_by
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('config_key')::varchar,
    sqlc.arg('value')::jsonb,
    sqlc.narg('updated_by')::uuid
)
ON CONFLICT (tenant_id, config_key) DO UPDATE SET
    value = EXCLUDED.value,
    updated_by = EXCLUDED.updated_by,
    updated_at = NOW()
RETURNING *;

-- name: DeleteSystemConfigOverride :execrows
DELETE FROM system_config_overrides
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND config_key = sqlc.arg('config_key')::varchar;
