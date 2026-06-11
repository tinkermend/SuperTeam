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
    COALESCE(sqlc.narg('options')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('context_payload')::jsonb, '{}'::jsonb)
) RETURNING *;

-- name: GetApprovalRequest :one
SELECT * FROM approval_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid;

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
