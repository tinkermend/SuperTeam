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

-- name: ListServiceTokensByTenant :many
-- 管理面列表:含 active/revoked,不回显 token_hash。
SELECT * FROM auth_service_tokens
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
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

-- name: ListFeishuAppConfigs :many
-- 管理面列表:含 active/unverified/disabled。
SELECT * FROM feishu_app_configs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
ORDER BY created_at ASC;

-- name: UpdateFeishuAppConfigStatus :one
UPDATE feishu_app_configs
SET status = sqlc.arg('status')::varchar,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

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

-- name: CreateFeishuOutbox :one
INSERT INTO feishu_outbox (
    tenant_id,
    project_id,
    kind,
    resource_type,
    resource_id,
    recipient_user_id,
    recipient_open_id,
    payload,
    status
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.narg('project_id')::uuid,
    sqlc.arg('kind')::varchar,
    sqlc.arg('resource_type')::varchar,
    sqlc.arg('resource_id')::uuid,
    sqlc.arg('recipient_user_id')::uuid,
    sqlc.arg('recipient_open_id')::varchar,
    COALESCE(sqlc.narg('payload')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('status')::varchar, 'pending')
) RETURNING *;

-- name: ListPendingFeishuOutbox :many
SELECT * FROM feishu_outbox
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND status = 'pending'
ORDER BY created_at ASC
LIMIT sqlc.arg('limit');

-- name: MarkFeishuOutboxSent :one
UPDATE feishu_outbox
SET status = 'sent',
    attempts = attempts + 1,
    feishu_message_id = sqlc.narg('feishu_message_id')::varchar,
    last_error = NULL,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = 'pending'
RETURNING *;

-- name: MarkFeishuOutboxFailed :one
UPDATE feishu_outbox
SET attempts = attempts + 1,
    last_error = sqlc.arg('last_error')::text,
    status = CASE WHEN attempts + 1 >= sqlc.arg('max_attempts')::int THEN 'failed' ELSE 'pending' END,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = 'pending'
RETURNING *;

-- name: SupersedePendingFeishuOutboxByResource :exec
UPDATE feishu_outbox
SET status = 'superseded', updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND resource_type = sqlc.arg('resource_type')::varchar
  AND resource_id = sqlc.arg('resource_id')::uuid
  AND status = 'pending';

-- name: ListSentFeishuOutboxByResource :many
SELECT * FROM feishu_outbox
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND resource_type = sqlc.arg('resource_type')::varchar
  AND resource_id = sqlc.arg('resource_id')::uuid
  AND status = 'sent'
  AND kind = 'decision_card';

-- name: ListProjectsForHumanMember :many
SELECT p.id, p.name
FROM projects p
WHERE p.tenant_id = sqlc.arg('tenant_id')::uuid
  AND p.deleted_at IS NULL
  AND p.status NOT IN ('archived')
  AND (
    p.human_owner_user_id = sqlc.arg('actor_user_id')::uuid
    OR EXISTS (
      SELECT 1 FROM project_members pm
      WHERE pm.tenant_id = p.tenant_id
        AND pm.project_id = p.id
        AND pm.principal_type = 'human_user'
        AND pm.principal_id = sqlc.arg('actor_user_id')::uuid
        AND pm.status = 'active'
    )
  )
ORDER BY p.updated_at DESC
LIMIT 50;
