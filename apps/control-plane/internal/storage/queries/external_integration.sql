-- name: CreateExternalIntegration :one
INSERT INTO external_integrations (
    tenant_id,
    project_id,
    digital_employee_id,
    name,
    description,
    allow_chat_run,
    allow_demand_submit,
    skill_ids,
    scenario_template_key,
    autonomy_tier,
    max_calls_per_hour,
    status,
    created_by_user_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('name')::varchar,
    sqlc.arg('description')::text,
    sqlc.arg('allow_chat_run')::boolean,
    sqlc.arg('allow_demand_submit')::boolean,
    sqlc.arg('skill_ids')::jsonb,
    sqlc.narg('scenario_template_key')::varchar,
    sqlc.arg('autonomy_tier')::varchar,
    sqlc.arg('max_calls_per_hour')::int,
    sqlc.arg('status')::varchar,
    sqlc.arg('created_by_user_id')::uuid
)
RETURNING *;

-- name: GetExternalIntegration :one
SELECT * FROM external_integrations
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid;

-- name: ListExternalIntegrationsByTenant :many
SELECT * FROM external_integrations
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND (
    sqlc.narg('project_id')::uuid IS NULL
    OR project_id = sqlc.narg('project_id')::uuid
  )
ORDER BY created_at DESC;

-- name: UpdateExternalIntegration :one
UPDATE external_integrations SET
    name = sqlc.arg('name')::varchar,
    description = sqlc.arg('description')::text,
    allow_chat_run = sqlc.arg('allow_chat_run')::boolean,
    allow_demand_submit = sqlc.arg('allow_demand_submit')::boolean,
    skill_ids = sqlc.arg('skill_ids')::jsonb,
    scenario_template_key = sqlc.narg('scenario_template_key')::varchar,
    autonomy_tier = sqlc.arg('autonomy_tier')::varchar,
    max_calls_per_hour = sqlc.arg('max_calls_per_hour')::int,
    status = sqlc.arg('status')::varchar,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: ConsumeExternalIntegrationBudget :one
-- P5 first-version budget: atomic fixed-window hourly counter on the binding row.
-- Zero rows returned on an existing active row means the window is full (429).
UPDATE external_integrations SET
    budget_window_start = CASE
        WHEN budget_window_start IS NULL OR budget_window_start <= NOW() - INTERVAL '1 hour'
        THEN NOW() ELSE budget_window_start END,
    budget_window_count = CASE
        WHEN budget_window_start IS NULL OR budget_window_start <= NOW() - INTERVAL '1 hour'
        THEN 1 ELSE budget_window_count + 1 END,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = 'active'
  AND (
    budget_window_start IS NULL
    OR budget_window_start <= NOW() - INTERVAL '1 hour'
    OR budget_window_count < max_calls_per_hour
  )
RETURNING *;

-- name: CreateExternalIntegrationToken :one
INSERT INTO external_integration_tokens (
    tenant_id,
    integration_id,
    token_sha256
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('integration_id')::uuid,
    sqlc.arg('token_sha256')::char(64)
)
RETURNING *;

-- name: GetActiveExternalIntegrationTokenBySHA :one
SELECT * FROM external_integration_tokens
WHERE token_sha256 = sqlc.arg('token_sha256')::char(64)
  AND status = 'active';

-- name: ListExternalIntegrationTokens :many
-- 管理面列表：含 active/revoked，不回显 token_sha256。
SELECT id, tenant_id, integration_id, status, created_at, last_used_at, revoked_at
FROM external_integration_tokens
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND integration_id = sqlc.arg('integration_id')::uuid
ORDER BY created_at DESC;

-- name: TouchExternalIntegrationTokenLastUsed :exec
UPDATE external_integration_tokens
SET last_used_at = NOW()
WHERE id = sqlc.arg('id')::uuid;

-- name: RevokeExternalIntegrationToken :one
UPDATE external_integration_tokens
SET status = 'revoked', revoked_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND integration_id = sqlc.arg('integration_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = 'active'
RETURNING *;
