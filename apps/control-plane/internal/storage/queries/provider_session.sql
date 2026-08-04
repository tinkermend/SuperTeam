-- CreateProviderSession retired (2026-07-21).

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

-- CreateProviderSessionEvent retired (2026-07-21).

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

-- name: FindProviderSessionCandidateForTaskRoot :one
-- 同 FindProviderSessionForTaskRoot，但把 resume 预检需要的事实一并带出：
-- 会话绑在哪个 runtime 节点、最后一次被 runtime 看到是什么时候。
-- 判据留在控制平面（spec 2026-08-01 §6.1）——runtime 侧不做 resume 兜底，
-- 那属于 provider 管道，越界。
SELECT provider_session_id, runtime_node_id, last_runtime_seen_at, last_active_at
FROM provider_sessions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND project_task_root_id = sqlc.arg('project_task_root_id')::uuid
  AND recoverable = true
  AND status IN ('active', 'idle', 'completed')
ORDER BY last_active_at DESC
LIMIT 1;

-- name: FindProviderSessionForTaskRoot :one
-- Resume the latest recoverable session for this lineage root, including
-- completed ones. Upstream work often finishes (status=completed) before a
-- revision/supplement under the same root is dispatched; requiring 'active'
-- made Plan 6's resume path a no-op in the real upstream-supplement flow.
SELECT provider_session_id
FROM provider_sessions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND project_task_root_id = sqlc.arg('project_task_root_id')::uuid
  AND recoverable = true
  AND status IN ('active', 'idle', 'completed')
ORDER BY last_active_at DESC
LIMIT 1;

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
    metadata,
    project_task_root_id
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
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb),
    sqlc.narg('project_task_root_id')::uuid
)
ON CONFLICT (tenant_id, provider_type, provider_session_id) DO UPDATE SET
    status = CASE
        WHEN EXCLUDED.last_sequence_number > provider_sessions.last_sequence_number THEN EXCLUDED.status
        ELSE provider_sessions.status
    END,
    last_active_at = CASE
        WHEN EXCLUDED.last_sequence_number > provider_sessions.last_sequence_number THEN NOW()
        ELSE provider_sessions.last_active_at
    END,
    session_display_id = CASE
        WHEN EXCLUDED.last_sequence_number > provider_sessions.last_sequence_number THEN COALESCE(EXCLUDED.session_display_id, provider_sessions.session_display_id)
        ELSE provider_sessions.session_display_id
    END,
    session_params = CASE
        WHEN EXCLUDED.last_sequence_number > provider_sessions.last_sequence_number AND EXCLUDED.session_params <> '{}'::jsonb THEN EXCLUDED.session_params
        ELSE provider_sessions.session_params
    END,
    session_state = CASE
        WHEN EXCLUDED.last_sequence_number > provider_sessions.last_sequence_number THEN COALESCE(provider_sessions.session_state, '{}'::jsonb) || COALESCE(EXCLUDED.session_state, '{}'::jsonb)
        ELSE provider_sessions.session_state
    END,
    last_sequence_number = GREATEST(provider_sessions.last_sequence_number, EXCLUDED.last_sequence_number),
    last_command_id = CASE
        WHEN EXCLUDED.last_sequence_number > provider_sessions.last_sequence_number THEN EXCLUDED.last_command_id
        ELSE provider_sessions.last_command_id
    END,
    last_run_id = CASE
        WHEN EXCLUDED.last_sequence_number > provider_sessions.last_sequence_number THEN EXCLUDED.last_run_id
        ELSE provider_sessions.last_run_id
    END,
    last_error_family = CASE
        WHEN EXCLUDED.last_sequence_number > provider_sessions.last_sequence_number THEN EXCLUDED.last_error_family
        ELSE provider_sessions.last_error_family
    END,
    last_runtime_seen_at = CASE
        WHEN EXCLUDED.last_sequence_number > provider_sessions.last_sequence_number THEN NOW()
        ELSE provider_sessions.last_runtime_seen_at
    END,
    metadata = CASE
        WHEN EXCLUDED.last_sequence_number > provider_sessions.last_sequence_number THEN COALESCE(provider_sessions.metadata, '{}'::jsonb) || COALESCE(EXCLUDED.metadata, '{}'::jsonb)
        ELSE provider_sessions.metadata
    END,
    project_task_root_id = CASE
        WHEN EXCLUDED.project_task_root_id IS NOT NULL THEN EXCLUDED.project_task_root_id
        ELSE provider_sessions.project_task_root_id
    END,
    updated_at = CASE
        WHEN EXCLUDED.last_sequence_number > provider_sessions.last_sequence_number THEN NOW()
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
