-- name: CreateExecutionLedgerEvent :one
SELECT ledger.*
FROM create_execution_ledger_event(
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
    sqlc.narg('artifact_refs')::jsonb,
    sqlc.narg('evidence_refs')::jsonb,
    sqlc.narg('metadata')::jsonb,
    sqlc.narg('occurred_at')::timestamptz,
    sqlc.arg('idempotency_key')::varchar
) AS ledger;

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
WITH source_event AS (
    SELECT
        pse.tenant_id,
        p.team_id,
        pt.project_id,
        pt.id AS project_task_id,
        pta.id AS project_task_attempt_id,
        'provider.event'::varchar AS event_type,
        'provider_session_event'::varchar AS source_type,
        pse.id::varchar AS source_id,
        'provider'::varchar AS actor_type,
        pse.provider_type::varchar AS actor_id,
        pse.runtime_node_id,
        pse.provider_type,
        ps.provider_session_id,
        NULLIF(pse.event_type, '')::text AS input_summary,
        COALESCE(NULLIF(pse.payload->>'summary', ''), NULLIF(pse.payload->>'text', ''), pse.event_type)::text AS output_summary,
        ps.last_error_family AS error_family,
        jsonb_build_object(
            'command_id', pse.command_id,
            'sequence_number', pse.sequence_number,
            'raw_event_ref', pse.raw_event_ref,
            'log_ref', pse.log_ref
        ) AS metadata,
        pse.created_at AS occurred_at,
        'provider_session_event:' || pse.id::varchar || ':provider.event' AS idempotency_key,
        COUNT(*) OVER () AS match_count
    FROM provider_session_events pse
    JOIN provider_sessions ps
      ON ps.tenant_id = pse.tenant_id
     AND ps.id = pse.provider_session_id
    JOIN project_task_attempts pta
      ON pta.tenant_id = pse.tenant_id
     AND pta.digital_employee_run_id = sqlc.arg('digital_employee_run_id')::uuid
     AND pta.provider_session_id = ps.provider_session_id
    JOIN project_tasks pt
      ON pt.tenant_id = pta.tenant_id
     AND pt.id = pta.project_task_id
    JOIN projects p
      ON p.tenant_id = pt.tenant_id
     AND p.id = pt.project_id
    WHERE pse.tenant_id = sqlc.arg('tenant_id')::uuid
      AND pse.id = sqlc.arg('provider_session_event_id')::uuid
)
SELECT ledger.*
FROM source_event source
CROSS JOIN LATERAL create_execution_ledger_event(
    source.tenant_id,
    source.team_id,
    source.project_id,
    source.project_task_id,
    source.project_task_attempt_id,
    source.event_type,
    source.source_type,
    source.source_id,
    source.actor_type,
    source.actor_id,
    source.runtime_node_id,
    source.provider_type,
    source.provider_session_id,
    source.input_summary,
    source.output_summary,
    source.error_family,
    NULL::varchar,
    NULL::text,
    NULL::boolean,
    NULL::jsonb,
    NULL::jsonb,
    source.metadata,
    source.occurred_at,
    source.idempotency_key
) AS ledger
WHERE source.match_count = 1;
