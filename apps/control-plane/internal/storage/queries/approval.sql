-- name: CreateApprovalRequest :one
INSERT INTO approval_requests (
    tenant_id,
    resource_type,
    resource_id,
    requester_type,
    requester_id,
    target_user_id,
    decision_type,
    title,
    summary,
    risk_level,
    status,
    category,
    options,
    context_payload
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('resource_type')::varchar,
    sqlc.arg('resource_id')::uuid,
    sqlc.arg('requester_type')::varchar,
    sqlc.narg('requester_id')::uuid,
    sqlc.arg('target_user_id')::uuid,
    sqlc.arg('decision_type')::varchar,
    sqlc.arg('title')::varchar,
    sqlc.narg('summary')::text,
    sqlc.narg('risk_level')::varchar,
    sqlc.arg('status')::varchar,
    sqlc.arg('category')::varchar,
    COALESCE(sqlc.narg('options')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('context_payload')::jsonb, '{}'::jsonb)
) RETURNING *;

-- name: ListPermissionApprovals :many
-- Permission-center read path: reads the approval domain directly (never via the
-- inbox projection). view=mine → target_user_id = actor; view=team → target_user_id NULL.
SELECT * FROM approval_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND category = 'permission'
  AND (
    sqlc.narg('status')::varchar IS NULL
    OR status = sqlc.narg('status')::varchar
  )
  AND (
    sqlc.narg('risk_level')::varchar IS NULL
    OR risk_level = sqlc.narg('risk_level')::varchar
  )
  AND (
    sqlc.narg('resource_type')::varchar IS NULL
    OR resource_type = sqlc.narg('resource_type')::varchar
  )
  AND (
    sqlc.narg('target_user_id')::uuid IS NULL
    OR target_user_id = sqlc.narg('target_user_id')::uuid
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: PermissionApprovalSummary :one
-- Metric-card totals over the view scope (independent of the status filter):
-- open = pending, high_risk = pending & high/critical, blocked = needs_more_evidence.
SELECT
    COUNT(*) FILTER (WHERE status = 'pending')::bigint AS open_count,
    COUNT(*) FILTER (WHERE status = 'pending' AND risk_level IN ('high', 'critical'))::bigint AS high_risk_count,
    COUNT(*) FILTER (WHERE status = 'needs_more_evidence')::bigint AS blocked_count
FROM approval_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND category = 'permission'
  AND (
    sqlc.narg('target_user_id')::uuid IS NULL
    OR target_user_id = sqlc.narg('target_user_id')::uuid
  );

-- name: GetApprovalRequest :one
SELECT * FROM approval_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid;

-- name: GetApprovalRequestByResource :one
SELECT * FROM approval_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND resource_type = sqlc.arg('resource_type')::varchar
  AND resource_id = sqlc.arg('resource_id')::uuid
  AND status = 'pending'
ORDER BY created_at DESC
LIMIT 1;

-- name: ResolveApprovalRequest :one
UPDATE approval_requests
SET status = sqlc.arg('status')::varchar,
    resolved_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = 'pending'
RETURNING *;

-- name: CreateApprovalDecision :one
INSERT INTO approval_decisions (
    tenant_id,
    approval_request_id,
    decided_by_user_id,
    decision,
    comment,
    payload
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('approval_request_id')::uuid,
    sqlc.arg('decided_by_user_id')::uuid,
    sqlc.arg('decision')::varchar,
    sqlc.narg('comment')::text,
    COALESCE(sqlc.narg('payload')::jsonb, '{}'::jsonb)
) RETURNING *;

-- name: ListApprovalDecisionsForRequest :many
SELECT * FROM approval_decisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND approval_request_id = sqlc.arg('approval_request_id')::uuid
ORDER BY created_at DESC;
