-- name: CreateExecutionLedgerEvent :one
WITH inserted AS (
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
    ON CONFLICT (tenant_id, idempotency_key) DO NOTHING
    RETURNING *
)
SELECT *
FROM inserted
UNION ALL
SELECT existing.*
FROM execution_ledger_events existing
WHERE existing.tenant_id = sqlc.arg('tenant_id')::uuid
  AND existing.idempotency_key = sqlc.arg('idempotency_key')::varchar
  AND NOT EXISTS (SELECT 1 FROM inserted)
LIMIT 1;

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
        'provider_session_event:' || pse.id::varchar || ':provider.event' AS idempotency_key
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
),
inserted AS (
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
    FROM source_event
    ON CONFLICT (tenant_id, idempotency_key) DO NOTHING
    RETURNING *
)
SELECT *
FROM inserted
UNION ALL
SELECT existing.*
FROM execution_ledger_events existing
JOIN source_event source
  ON source.tenant_id = existing.tenant_id
 AND source.idempotency_key = existing.idempotency_key
WHERE NOT EXISTS (SELECT 1 FROM inserted)
LIMIT 1;
