-- 飞书集成:服务凭据、应用配置、身份绑定查询。

-- name: CreateServiceToken :one
INSERT INTO auth_service_tokens (
    tenant_id,
    service_name,
    token_hash
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('service_name')::varchar,
    sqlc.arg('token_hash')::varchar
) RETURNING *;

-- name: ListActiveServiceTokensByName :many
SELECT * FROM auth_service_tokens
WHERE service_name = sqlc.arg('service_name')::varchar
  AND status = 'active'
ORDER BY created_at DESC;

-- name: TouchServiceTokenLastUsed :exec
UPDATE auth_service_tokens
SET last_used_at = NOW()
WHERE id = sqlc.arg('id')::uuid;

-- name: RevokeServiceToken :one
UPDATE auth_service_tokens
SET status = 'revoked', revoked_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = 'active'
RETURNING *;

-- name: UpsertFeishuAppConfig :one
INSERT INTO feishu_app_configs (
    tenant_id,
    app_id,
    app_secret_sealed,
    status
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('app_id')::varchar,
    sqlc.arg('app_secret_sealed')::text,
    COALESCE(sqlc.narg('status')::varchar, 'active')
)
ON CONFLICT (tenant_id, app_id) DO UPDATE SET
    app_secret_sealed = EXCLUDED.app_secret_sealed,
    status = EXCLUDED.status,
    updated_at = NOW()
RETURNING *;

-- name: ListActiveFeishuAppConfigs :many
SELECT * FROM feishu_app_configs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND status = 'active'
ORDER BY created_at ASC;

-- name: GetFeishuAppConfig :one
SELECT * FROM feishu_app_configs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid;

-- name: CreateFeishuIdentity :one
INSERT INTO user_feishu_identities (
    tenant_id,
    auth_user_id,
    feishu_app_config_id,
    open_id,
    union_id,
    bound_via
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('auth_user_id')::uuid,
    sqlc.arg('feishu_app_config_id')::uuid,
    sqlc.arg('open_id')::varchar,
    sqlc.narg('union_id')::varchar,
    sqlc.arg('bound_via')::varchar
) RETURNING *;

-- name: GetFeishuIdentityByOpenID :one
SELECT * FROM user_feishu_identities
WHERE feishu_app_config_id = sqlc.arg('feishu_app_config_id')::uuid
  AND open_id = sqlc.arg('open_id')::varchar;

-- name: GetFeishuIdentityByUser :one
SELECT * FROM user_feishu_identities
WHERE feishu_app_config_id = sqlc.arg('feishu_app_config_id')::uuid
  AND auth_user_id = sqlc.arg('auth_user_id')::uuid;

-- name: ListFeishuIdentitiesByUsers :many
SELECT * FROM user_feishu_identities
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND auth_user_id = ANY(sqlc.arg('auth_user_ids')::uuid[]);

-- name: DeleteFeishuIdentityByUser :exec
DELETE FROM user_feishu_identities
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND feishu_app_config_id = sqlc.arg('feishu_app_config_id')::uuid
  AND auth_user_id = sqlc.arg('auth_user_id')::uuid;

-- name: ListFeishuIdentitiesByTenant :many
SELECT * FROM user_feishu_identities
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
ORDER BY created_at DESC;
