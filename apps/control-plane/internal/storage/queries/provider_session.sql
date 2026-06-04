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
) SELECT
    dei.tenant_id,
    sqlc.arg('provider_session_id')::varchar,
    dei.digital_employee_id,
    dei.id,
    dei.runtime_node_id,
    dei.provider_type,
    sqlc.arg('status')::varchar,
    sqlc.arg('recoverable')::boolean,
    sqlc.arg('last_active_at')::timestamptz,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
FROM digital_employee_execution_instances dei
JOIN digital_employees de
  ON de.id = dei.digital_employee_id
 AND de.tenant_id = dei.tenant_id
 AND de.status NOT IN ('disabled', 'error')
 AND de.deleted_at IS NULL
 AND de.archived_at IS NULL
JOIN runtime_nodes rn
  ON rn.id = dei.runtime_node_id
 AND rn.tenant_id = dei.tenant_id
 AND rn.status = 'online'
 AND rn.disabled_at IS NULL
 AND rn.archived_at IS NULL
WHERE dei.id = sqlc.arg('execution_instance_id')::uuid
  AND dei.tenant_id = sqlc.arg('tenant_id')::uuid
  AND dei.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND dei.runtime_node_id = sqlc.arg('runtime_node_id')::uuid
  AND dei.provider_type = sqlc.arg('provider_type')::varchar
  AND dei.status IN ('ready', 'active')
  AND dei.deleted_at IS NULL
  AND EXISTS (
      SELECT 1
      FROM runtime_sessions rs
      JOIN runtime_enrollments re
        ON re.id = rs.enrollment_id
       AND re.tenant_id = rs.tenant_id
       AND re.runtime_node_id = rs.runtime_node_id
       AND re.status = 'approved'
       AND re.revoked_at IS NULL
       AND re.rejected_at IS NULL
      WHERE rs.tenant_id = dei.tenant_id
        AND rs.runtime_node_id = rn.id
        AND rs.expires_at > NOW()
        AND rs.revoked_at IS NULL
  )
RETURNING *;

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
JOIN digital_employee_execution_instances dei
  ON dei.id = ps.execution_instance_id
 AND dei.tenant_id = ps.tenant_id
 AND dei.digital_employee_id = ps.digital_employee_id
 AND dei.runtime_node_id = ps.runtime_node_id
 AND dei.provider_type = ps.provider_type
 AND dei.status IN ('ready', 'active')
 AND dei.deleted_at IS NULL
JOIN digital_employees de
  ON de.id = ps.digital_employee_id
 AND de.tenant_id = ps.tenant_id
 AND de.status NOT IN ('disabled', 'error')
 AND de.deleted_at IS NULL
 AND de.archived_at IS NULL
JOIN runtime_nodes rn
  ON rn.id = ps.runtime_node_id
 AND rn.tenant_id = ps.tenant_id
 AND rn.node_id = sqlc.arg('node_id')::varchar
 AND rn.status = 'online'
 AND rn.disabled_at IS NULL
 AND rn.archived_at IS NULL
WHERE ps.id = sqlc.arg('provider_session_id')::uuid
  AND ps.tenant_id = sqlc.arg('tenant_id')::uuid
  AND ps.status IN ('running', 'idle')
  AND ps.closed_at IS NULL
  AND EXISTS (
      SELECT 1
      FROM runtime_sessions rs
      JOIN runtime_enrollments re
        ON re.id = rs.enrollment_id
       AND re.tenant_id = rs.tenant_id
       AND re.runtime_node_id = rs.runtime_node_id
       AND re.status = 'approved'
       AND re.revoked_at IS NULL
       AND re.rejected_at IS NULL
      WHERE rs.tenant_id = ps.tenant_id
        AND rs.runtime_node_id = rn.id
        AND rs.expires_at > NOW()
        AND rs.revoked_at IS NULL
  )
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

-- name: UpsertProviderSessionByExternalID :one
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
    session_display_id,
    session_params,
    session_state,
    last_sequence_number,
    last_command_id,
    last_run_id,
    last_error_family,
    last_runtime_seen_at,
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
    NOW(),
    sqlc.narg('session_display_id')::varchar,
    COALESCE(sqlc.arg('session_params')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('session_state')::jsonb, '{}'::jsonb),
    sqlc.arg('last_sequence_number')::integer,
    sqlc.narg('last_command_id')::varchar,
    sqlc.narg('last_run_id')::uuid,
    sqlc.narg('last_error_family')::varchar,
    NOW(),
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
)
ON CONFLICT (tenant_id, provider_type, provider_session_id) DO UPDATE SET
    status = CASE
        WHEN EXCLUDED.last_sequence_number >= provider_sessions.last_sequence_number THEN EXCLUDED.status
        ELSE provider_sessions.status
    END,
    last_active_at = CASE
        WHEN EXCLUDED.last_sequence_number >= provider_sessions.last_sequence_number THEN NOW()
        ELSE provider_sessions.last_active_at
    END,
    session_display_id = CASE
        WHEN EXCLUDED.last_sequence_number >= provider_sessions.last_sequence_number THEN COALESCE(EXCLUDED.session_display_id, provider_sessions.session_display_id)
        ELSE provider_sessions.session_display_id
    END,
    session_params = CASE
        WHEN EXCLUDED.last_sequence_number >= provider_sessions.last_sequence_number THEN EXCLUDED.session_params
        ELSE provider_sessions.session_params
    END,
    session_state = CASE
        WHEN EXCLUDED.last_sequence_number >= provider_sessions.last_sequence_number THEN EXCLUDED.session_state
        ELSE provider_sessions.session_state
    END,
    last_sequence_number = GREATEST(provider_sessions.last_sequence_number, EXCLUDED.last_sequence_number),
    last_command_id = CASE
        WHEN EXCLUDED.last_sequence_number >= provider_sessions.last_sequence_number THEN EXCLUDED.last_command_id
        ELSE provider_sessions.last_command_id
    END,
    last_run_id = CASE
        WHEN EXCLUDED.last_sequence_number >= provider_sessions.last_sequence_number THEN EXCLUDED.last_run_id
        ELSE provider_sessions.last_run_id
    END,
    last_error_family = CASE
        WHEN EXCLUDED.last_sequence_number >= provider_sessions.last_sequence_number THEN EXCLUDED.last_error_family
        ELSE provider_sessions.last_error_family
    END,
    last_runtime_seen_at = CASE
        WHEN EXCLUDED.last_sequence_number >= provider_sessions.last_sequence_number THEN NOW()
        ELSE provider_sessions.last_runtime_seen_at
    END,
    metadata = CASE
        WHEN EXCLUDED.last_sequence_number >= provider_sessions.last_sequence_number THEN EXCLUDED.metadata
        ELSE provider_sessions.metadata
    END,
    updated_at = CASE
        WHEN EXCLUDED.last_sequence_number >= provider_sessions.last_sequence_number THEN NOW()
        ELSE provider_sessions.updated_at
    END
RETURNING *;

-- name: CreateProviderSessionEventIfAbsent :one
WITH inserted AS (
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
        log_ref,
        session_state_patch,
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
        COALESCE(sqlc.arg('payload')::jsonb, '{}'::jsonb),
        sqlc.narg('request_id')::varchar,
        sqlc.narg('command_id')::varchar,
        sqlc.narg('raw_event_ref')::text,
        sqlc.narg('log_ref')::text,
        COALESCE(sqlc.arg('session_state_patch')::jsonb, '{}'::jsonb),
        COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
    FROM provider_sessions ps
    WHERE ps.id = sqlc.arg('provider_session_uuid')::uuid
      AND ps.tenant_id = sqlc.arg('tenant_id')::uuid
    ON CONFLICT DO NOTHING
    RETURNING *
)
SELECT * FROM inserted
UNION ALL
SELECT *
FROM provider_session_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND provider_session_id = sqlc.arg('provider_session_uuid')::uuid
  AND sequence_number = sqlc.arg('sequence_number')::integer
  AND (
      (
          sqlc.narg('command_id')::varchar IS NOT NULL
          AND command_id = sqlc.narg('command_id')::varchar
      )
      OR (
          sqlc.narg('command_id')::varchar IS NULL
          AND sqlc.narg('request_id')::varchar IS NOT NULL
          AND request_id = sqlc.narg('request_id')::varchar
      )
  )
LIMIT 1;
