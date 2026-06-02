-- name: CreateProviderSession :one
INSERT INTO provider_sessions (
    tenant_id,
    provider_session_id,
    digital_employee_id,
    execution_instance_id,
    runtime_node_id,
    provider_type,
    status,
    recoverable,
    last_active_at,
    metadata
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('provider_session_id')::varchar,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('execution_instance_id')::uuid,
    sqlc.arg('runtime_node_id')::uuid,
    sqlc.arg('provider_type')::varchar,
    sqlc.arg('status')::varchar,
    sqlc.arg('recoverable')::boolean,
    sqlc.arg('last_active_at')::timestamptz,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
) RETURNING *;

-- name: GetProviderSession :one
SELECT *
FROM provider_sessions
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid;

-- name: GetProviderSessionByExternalID :one
SELECT *
FROM provider_sessions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND provider_type = sqlc.arg('provider_type')::varchar
  AND provider_session_id = sqlc.arg('provider_session_id')::varchar;

-- name: ListProviderSessionsForDigitalEmployee :many
SELECT *
FROM provider_sessions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)
ORDER BY last_active_at DESC NULLS LAST, created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: UpdateProviderSessionStatus :one
UPDATE provider_sessions
SET status = sqlc.arg('status')::varchar,
    last_active_at = NOW(),
    closed_at = CASE
        WHEN sqlc.arg('status')::varchar IN ('completed', 'failed', 'stopped') THEN COALESCE(closed_at, NOW())
        ELSE closed_at
    END,
    error_message = CASE
        WHEN sqlc.arg('status')::varchar = 'failed' THEN COALESCE(sqlc.narg('error_message')::text, error_message)
        WHEN sqlc.arg('status')::varchar IN ('running', 'idle', 'completed', 'stopped') THEN NULL
        ELSE error_message
    END,
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
RETURNING *;

-- name: CreateProviderSessionEvent :one
INSERT INTO provider_session_events (
    tenant_id,
    provider_session_id,
    digital_employee_id,
    execution_instance_id,
    runtime_node_id,
    provider_type,
    event_type,
    sequence_number,
    payload,
    request_id,
    command_id,
    raw_event_ref,
    metadata
) SELECT
    ps.tenant_id,
    ps.id,
    ps.digital_employee_id,
    ps.execution_instance_id,
    ps.runtime_node_id,
    ps.provider_type,
    sqlc.arg('event_type')::varchar,
    sqlc.arg('sequence_number')::integer,
    sqlc.arg('payload')::jsonb,
    sqlc.narg('request_id')::varchar,
    sqlc.narg('command_id')::varchar,
    sqlc.narg('raw_event_ref')::text,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
FROM provider_sessions ps
WHERE ps.id = sqlc.arg('provider_session_id')::uuid
  AND ps.tenant_id = sqlc.arg('tenant_id')::uuid
RETURNING *;

-- name: ListProviderSessionEvents :many
SELECT *
FROM provider_session_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND provider_session_id = sqlc.arg('provider_session_id')::uuid
ORDER BY sequence_number ASC;

-- name: GetLatestProviderSessionEventSequence :one
SELECT COALESCE(MAX(sequence_number), 0)::integer AS max_sequence
FROM provider_session_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND provider_session_id = sqlc.arg('provider_session_id')::uuid;
