-- name: UpsertInboxItem :one
INSERT INTO inbox_items (
    tenant_id,
    team_id,
    target_user_id,
    scope,
    item_type,
    source_type,
    source_id,
    source_project_id,
    source_task_id,
    source_approval_request_id,
    title,
    summary,
    risk_level,
    priority,
    status,
    action_schema,
    context_payload,
    deep_link,
    resolved_at,
    last_activity_at
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.narg('team_id')::uuid,
    sqlc.arg('target_user_id')::uuid,
    sqlc.arg('scope')::varchar,
    sqlc.arg('item_type')::varchar,
    sqlc.arg('source_type')::varchar,
    sqlc.arg('source_id')::uuid,
    sqlc.narg('source_project_id')::uuid,
    sqlc.narg('source_task_id')::uuid,
    sqlc.narg('source_approval_request_id')::uuid,
    sqlc.arg('title')::varchar,
    sqlc.narg('summary')::text,
    sqlc.narg('risk_level')::varchar,
    sqlc.narg('priority')::varchar,
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.narg('action_schema')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('context_payload')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('deep_link')::jsonb, '{}'::jsonb),
    sqlc.narg('resolved_at')::timestamptz,
    sqlc.arg('last_activity_at')::timestamptz
)
ON CONFLICT (tenant_id, source_type, source_id)
DO UPDATE SET
    team_id = EXCLUDED.team_id,
    target_user_id = EXCLUDED.target_user_id,
    scope = EXCLUDED.scope,
    item_type = EXCLUDED.item_type,
    source_project_id = EXCLUDED.source_project_id,
    source_task_id = EXCLUDED.source_task_id,
    source_approval_request_id = EXCLUDED.source_approval_request_id,
    title = EXCLUDED.title,
    summary = EXCLUDED.summary,
    risk_level = EXCLUDED.risk_level,
    priority = EXCLUDED.priority,
    status = EXCLUDED.status,
    action_schema = EXCLUDED.action_schema,
    context_payload = EXCLUDED.context_payload,
    deep_link = EXCLUDED.deep_link,
    resolved_at = EXCLUDED.resolved_at,
    last_activity_at = EXCLUDED.last_activity_at,
    updated_at = NOW()
RETURNING *;

-- name: UpsertInboxItemByApprovalSource :one
INSERT INTO inbox_items (
    tenant_id,
    team_id,
    target_user_id,
    scope,
    item_type,
    source_type,
    source_id,
    source_project_id,
    source_task_id,
    source_approval_request_id,
    title,
    summary,
    risk_level,
    priority,
    status,
    action_schema,
    context_payload,
    deep_link,
    resolved_at,
    last_activity_at
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.narg('team_id')::uuid,
    sqlc.arg('target_user_id')::uuid,
    sqlc.arg('scope')::varchar,
    sqlc.arg('item_type')::varchar,
    sqlc.arg('source_type')::varchar,
    sqlc.arg('source_id')::uuid,
    sqlc.narg('source_project_id')::uuid,
    sqlc.narg('source_task_id')::uuid,
    sqlc.arg('source_approval_request_id')::uuid,
    sqlc.arg('title')::varchar,
    sqlc.narg('summary')::text,
    sqlc.narg('risk_level')::varchar,
    sqlc.narg('priority')::varchar,
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.narg('action_schema')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('context_payload')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('deep_link')::jsonb, '{}'::jsonb),
    sqlc.narg('resolved_at')::timestamptz,
    sqlc.arg('last_activity_at')::timestamptz
)
ON CONFLICT (tenant_id, source_approval_request_id)
WHERE source_approval_request_id IS NOT NULL
DO UPDATE SET
    team_id = EXCLUDED.team_id,
    target_user_id = EXCLUDED.target_user_id,
    scope = EXCLUDED.scope,
    item_type = EXCLUDED.item_type,
    source_type = EXCLUDED.source_type,
    source_id = EXCLUDED.source_id,
    source_project_id = EXCLUDED.source_project_id,
    source_task_id = EXCLUDED.source_task_id,
    title = EXCLUDED.title,
    summary = EXCLUDED.summary,
    risk_level = EXCLUDED.risk_level,
    priority = EXCLUDED.priority,
    status = EXCLUDED.status,
    action_schema = EXCLUDED.action_schema,
    context_payload = EXCLUDED.context_payload,
    deep_link = EXCLUDED.deep_link,
    resolved_at = EXCLUDED.resolved_at,
    last_activity_at = EXCLUDED.last_activity_at,
    updated_at = NOW()
RETURNING *;

-- name: GetInboxItem :one
SELECT * FROM inbox_items
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid;

-- name: ListInboxItems :many
SELECT * FROM inbox_items
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND status = sqlc.arg('status')::varchar
  AND (
    sqlc.narg('target_user_id')::uuid IS NULL
    OR target_user_id = sqlc.narg('target_user_id')::uuid
  )
  AND (
    sqlc.narg('item_type')::varchar IS NULL
    OR item_type = sqlc.narg('item_type')::varchar
  )
  AND (
    sqlc.narg('risk_level')::varchar IS NULL
    OR risk_level = sqlc.narg('risk_level')::varchar
  )
  AND (
    sqlc.narg('source_project_id')::uuid IS NULL
    OR source_project_id = sqlc.narg('source_project_id')::uuid
  )
ORDER BY last_activity_at DESC, created_at DESC, id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountInboxItems :one
SELECT COUNT(*)::bigint FROM inbox_items
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND status = sqlc.arg('status')::varchar
  AND (
    sqlc.narg('target_user_id')::uuid IS NULL
    OR target_user_id = sqlc.narg('target_user_id')::uuid
  )
  AND (
    sqlc.narg('item_type')::varchar IS NULL
    OR item_type = sqlc.narg('item_type')::varchar
  )
  AND (
    sqlc.narg('risk_level')::varchar IS NULL
    OR risk_level = sqlc.narg('risk_level')::varchar
  )
  AND (
    sqlc.narg('source_project_id')::uuid IS NULL
    OR source_project_id = sqlc.narg('source_project_id')::uuid
  );

-- name: CountHighRiskInboxItems :one
SELECT COUNT(*)::bigint FROM inbox_items
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND status = 'open'
  AND risk_level = 'high'
  AND (
    sqlc.narg('target_user_id')::uuid IS NULL
    OR target_user_id = sqlc.narg('target_user_id')::uuid
  );
