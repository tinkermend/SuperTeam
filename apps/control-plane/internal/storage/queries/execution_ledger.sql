-- name: CreateExecutionLedgerEvent :one
INSERT INTO execution_ledger_events (
    tenant_id,
    team_id,
    project_id,
    project_task_id,
    project_task_attempt_id,
    event_type,
    source_type,
    source_id,
    actor_type,
    actor_id,
    runtime_node_id,
    provider_type,
    provider_session_id,
    input_summary,
    output_summary,
    error_family,
    error_code,
    error_message,
    retryable,
    artifact_refs,
    evidence_refs,
    metadata,
    occurred_at,
    idempotency_key
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.narg('team_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.narg('project_task_id')::uuid,
    sqlc.narg('project_task_attempt_id')::uuid,
    sqlc.arg('event_type')::varchar,
    sqlc.arg('source_type')::varchar,
    sqlc.arg('source_id')::varchar,
    sqlc.arg('actor_type')::varchar,
    sqlc.narg('actor_id')::varchar,
    sqlc.narg('runtime_node_id')::uuid,
    sqlc.narg('provider_type')::varchar,
    sqlc.narg('provider_session_id')::varchar,
    sqlc.narg('input_summary')::text,
    sqlc.narg('output_summary')::text,
    sqlc.narg('error_family')::varchar,
    sqlc.narg('error_code')::varchar,
    sqlc.narg('error_message')::text,
    sqlc.narg('retryable')::boolean,
    COALESCE(sqlc.narg('artifact_refs')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('evidence_refs')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('metadata')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('occurred_at')::timestamptz, NOW()),
    sqlc.arg('idempotency_key')::varchar
)
ON CONFLICT (tenant_id, idempotency_key) DO UPDATE SET
    idempotency_key = EXCLUDED.idempotency_key
RETURNING *;

-- name: ListProjectExecutionLedgerEvents :many
SELECT *
FROM execution_ledger_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND (sqlc.narg('project_task_id')::uuid IS NULL OR project_task_id = sqlc.narg('project_task_id')::uuid)
  AND (sqlc.narg('project_task_attempt_id')::uuid IS NULL OR project_task_attempt_id = sqlc.narg('project_task_attempt_id')::uuid)
  AND (sqlc.narg('event_type')::varchar IS NULL OR event_type = sqlc.narg('event_type')::varchar)
  AND (sqlc.narg('error_family')::varchar IS NULL OR error_family = sqlc.narg('error_family')::varchar)
ORDER BY occurred_at ASC, created_at ASC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: ListProjectTaskAttemptsForExecutionTrace :many
SELECT pta.*
FROM project_task_attempts pta
JOIN project_tasks pt
  ON pt.tenant_id = pta.tenant_id
 AND pt.id = pta.project_task_id
WHERE pta.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pt.project_id = sqlc.arg('project_id')::uuid
ORDER BY pt.created_at ASC, pta.attempt_no ASC, pta.created_at ASC;

-- name: CreateProviderSessionEventLedgerEvent :one
INSERT INTO execution_ledger_events (
    tenant_id,
    team_id,
    project_id,
    project_task_id,
    project_task_attempt_id,
    event_type,
    source_type,
    source_id,
    actor_type,
    actor_id,
    runtime_node_id,
    provider_type,
    provider_session_id,
    input_summary,
    output_summary,
    error_family,
    metadata,
    occurred_at,
    idempotency_key
)
SELECT
    pse.tenant_id,
    p.team_id,
    pt.project_id,
    pt.id,
    pta.id,
    'provider.event',
    'provider_session_event',
    pse.id::varchar,
    'provider',
    pse.provider_type,
    pse.runtime_node_id,
    pse.provider_type,
    ps.provider_session_id,
    NULLIF(pse.event_type, ''),
    COALESCE(NULLIF(pse.payload->>'summary', ''), NULLIF(pse.payload->>'text', ''), pse.event_type),
    ps.last_error_family,
    jsonb_build_object(
        'command_id', pse.command_id,
        'sequence_number', pse.sequence_number,
        'raw_event_ref', pse.raw_event_ref,
        'log_ref', pse.log_ref
    ),
    pse.created_at,
    'provider_session_event:' || pse.id::varchar || ':provider.event'
FROM provider_session_events pse
JOIN provider_sessions ps
  ON ps.tenant_id = pse.tenant_id
 AND ps.id = pse.provider_session_id
JOIN project_task_attempts pta
  ON pta.tenant_id = pse.tenant_id
 AND pta.digital_employee_run_id = sqlc.arg('digital_employee_run_id')::uuid
JOIN project_tasks pt
  ON pt.tenant_id = pta.tenant_id
 AND pt.id = pta.project_task_id
JOIN projects p
  ON p.tenant_id = pt.tenant_id
 AND p.id = pt.project_id
WHERE pse.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pse.id = sqlc.arg('provider_session_event_id')::uuid
ON CONFLICT (tenant_id, idempotency_key) DO UPDATE SET
    idempotency_key = EXCLUDED.idempotency_key
RETURNING *;
