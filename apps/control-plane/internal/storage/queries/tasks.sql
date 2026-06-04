-- name: CreateTask :one
INSERT INTO tasks (
    tenant_id,
    team_id,
    title,
    description,
    status,
    priority,
    provider_type,
    creator_id,
    target_node_id,
    workspace_path,
    params
) VALUES (
    COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid),
    sqlc.narg('team_id')::uuid,
    sqlc.arg('title')::varchar,
    sqlc.narg('description')::text,
    sqlc.arg('status')::varchar,
    sqlc.arg('priority')::integer,
    sqlc.arg('provider_type')::varchar,
    sqlc.narg('creator_id')::uuid,
    sqlc.narg('target_node_id')::varchar,
    sqlc.narg('workspace_path')::text,
    COALESCE(sqlc.arg('params')::jsonb, '{}'::jsonb)
) RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid);

-- name: UpdateTaskStatus :one
UPDATE tasks
SET
    status = sqlc.arg('status')::varchar,
    cancelled_at = CASE
        WHEN sqlc.arg('status')::varchar = 'cancelled' THEN COALESCE(cancelled_at, NOW())
        ELSE cancelled_at
    END,
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
RETURNING *;

-- name: UpdateTask :one
UPDATE tasks
SET
    title = COALESCE(sqlc.narg('title'), title),
    description = COALESCE(sqlc.narg('description'), description),
    status = COALESCE(sqlc.narg('status'), status),
    priority = COALESCE(sqlc.narg('priority'), priority),
    target_node_id = COALESCE(sqlc.narg('target_node_id'), target_node_id),
    assigned_node_id = COALESCE(sqlc.narg('assigned_node_id'), assigned_node_id),
    workspace_path = COALESCE(sqlc.narg('workspace_path'), workspace_path),
    params = COALESCE(sqlc.narg('params'), params),
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
RETURNING *;

-- name: ListTasks :many
SELECT * FROM tasks
WHERE tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
  AND deleted_at IS NULL
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)
  AND (sqlc.narg('creator_id')::uuid IS NULL OR creator_id = sqlc.narg('creator_id')::uuid)
  AND (sqlc.narg('provider_type')::varchar IS NULL OR provider_type = sqlc.narg('provider_type')::varchar)
ORDER BY priority DESC, created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: DeleteTask :exec
UPDATE tasks
SET deleted_at = COALESCE(deleted_at, NOW()), updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid);

-- name: CreateTaskEvent :one
INSERT INTO task_events (
    tenant_id,
    task_id,
    run_id,
    event_type,
    sequence_number,
    payload
) VALUES (
    COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid),
    sqlc.arg('task_id')::uuid,
    sqlc.narg('run_id')::uuid,
    sqlc.arg('event_type')::varchar,
    sqlc.arg('sequence_number')::integer,
    sqlc.arg('payload')::jsonb
) RETURNING *;

-- name: GetLatestTaskEventSequence :one
SELECT COALESCE(MAX(sequence_number), 0)::integer as max_sequence
FROM task_events
WHERE task_id = sqlc.arg('task_id')::uuid
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid);

-- name: ListTaskEvents :many
SELECT * FROM task_events
WHERE task_id = sqlc.arg('task_id')::uuid
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
ORDER BY sequence_number ASC;

-- name: GetTaskEvent :one
SELECT * FROM task_events
WHERE task_id = sqlc.arg('task_id')::uuid
  AND sequence_number = sqlc.arg('sequence_number')::integer
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid);

-- name: CreateTaskStateHistory :one
INSERT INTO task_state_history (
    tenant_id,
    task_id,
    from_status,
    to_status,
    changed_by,
    reason
) VALUES (
    COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid),
    sqlc.arg('task_id')::uuid,
    sqlc.narg('from_status')::varchar,
    sqlc.arg('to_status')::varchar,
    sqlc.narg('changed_by')::varchar,
    sqlc.narg('reason')::text
) RETURNING *;

-- name: CreateTaskRun :one
INSERT INTO task_runs (
    tenant_id,
    task_id,
    node_id,
    status
) VALUES (
    COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid),
    sqlc.arg('task_id')::uuid,
    sqlc.arg('node_id')::varchar,
    sqlc.arg('status')::varchar
) RETURNING *;

-- name: CreateTaskArtifact :one
INSERT INTO task_artifacts (
    tenant_id,
    task_id,
    run_id,
    artifact_type,
    name,
    storage_url
) VALUES (
    COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid),
    sqlc.arg('task_id')::uuid,
    sqlc.narg('run_id')::uuid,
    sqlc.arg('artifact_type')::varchar,
    sqlc.arg('name')::varchar,
    sqlc.arg('storage_url')::text
) RETURNING *;

-- name: ListTaskArtifacts :many
SELECT * FROM task_artifacts
WHERE task_id = sqlc.arg('task_id')::uuid
  AND deleted_at IS NULL
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
ORDER BY created_at DESC;

-- name: ListPendingTasks :many
SELECT * FROM tasks
WHERE tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
  AND deleted_at IS NULL
  AND status = 'pending'
  AND (target_node_id IS NULL OR target_node_id = sqlc.narg('target_node_id')::varchar)
ORDER BY priority DESC, created_at ASC
LIMIT sqlc.arg('limit');

-- name: UpdateTaskAssignment :one
UPDATE tasks
SET assigned_node_id = sqlc.arg('assigned_node_id')::varchar, status = 'claimed', updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
RETURNING *;

-- name: UpdateTaskRun :one
UPDATE task_runs
SET status = sqlc.arg('status')::varchar,
    completed_at = NOW(),
    finished_at = NOW(),
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
RETURNING *;

-- name: CancelTask :one
UPDATE tasks
SET status = 'cancelled',
    cancelled_at = COALESCE(cancelled_at, NOW()),
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
RETURNING *;

-- name: GetTaskArtifact :one
SELECT * FROM task_artifacts
WHERE id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid);

-- name: DeleteTaskArtifact :exec
UPDATE task_artifacts
SET deleted_at = COALESCE(deleted_at, NOW())
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid);

-- name: GetTaskRun :one
SELECT * FROM task_runs
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid);

-- name: ListTaskRuns :many
SELECT * FROM task_runs
WHERE task_id = sqlc.arg('task_id')::uuid
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
ORDER BY created_at DESC;

-- name: GetLatestTaskRun :one
SELECT * FROM task_runs
WHERE task_id = sqlc.arg('task_id')::uuid
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
ORDER BY created_at DESC
LIMIT 1;

-- name: ListTaskStateHistory :many
SELECT * FROM task_state_history
WHERE task_id = sqlc.arg('task_id')::uuid
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
ORDER BY created_at DESC;

-- name: UpdateTaskWorkspace :one
UPDATE tasks
SET workspace_path = sqlc.arg('workspace_path')::text, updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
RETURNING *;

-- name: CreateDigitalEmployeeTaskRun :one
WITH created_task AS (
    INSERT INTO tasks (
        tenant_id,
        team_id,
        title,
        description,
        status,
        priority,
        provider_type,
        creator_id,
        target_node_id,
        workspace_path,
        params,
        idempotency_key,
        risk_level
    ) VALUES (
        sqlc.arg('tenant_id')::uuid,
        sqlc.arg('team_id')::uuid,
        sqlc.arg('title')::varchar,
        sqlc.narg('description')::text,
        'pending',
        sqlc.arg('priority')::integer,
        sqlc.arg('provider_type')::varchar,
        sqlc.narg('creator_id')::uuid,
        sqlc.arg('target_node_id')::varchar,
        sqlc.narg('workspace_path')::text,
        COALESCE(sqlc.arg('params')::jsonb, '{}'::jsonb),
        sqlc.narg('idempotency_key')::varchar,
        COALESCE(sqlc.narg('risk_level')::varchar, 'normal')
    )
    RETURNING *
),
created_run AS (
    INSERT INTO task_runs (
        tenant_id,
        task_id,
        node_id,
        runtime_node_id,
        provider_session_id,
        status,
        command_id,
        digital_employee_id,
        execution_instance_id,
        idempotency_key,
        timeout_sec,
        grace_sec
    )
    SELECT
        created_task.tenant_id,
        created_task.id,
        sqlc.arg('node_id')::varchar,
        sqlc.arg('runtime_node_id')::uuid,
        sqlc.narg('provider_session_id')::varchar,
        sqlc.arg('run_status')::varchar,
        sqlc.arg('command_id')::varchar,
        sqlc.arg('digital_employee_id')::uuid,
        sqlc.arg('execution_instance_id')::uuid,
        sqlc.narg('idempotency_key')::varchar,
        sqlc.narg('timeout_sec')::integer,
        sqlc.narg('grace_sec')::integer
    FROM created_task
    RETURNING *
)
SELECT
    created_task.id AS task_id,
    created_run.id AS run_id,
    created_run.command_id,
    created_task.status AS task_status,
    created_run.status AS run_status
FROM created_task
JOIN created_run ON created_run.task_id = created_task.id;

-- name: GetActiveDigitalEmployeeRun :one
SELECT tr.*
FROM task_runs tr
JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND tr.status IN ('queued', 'dispatching', 'running', 'cancelling')
  AND t.deleted_at IS NULL
ORDER BY tr.created_at DESC
LIMIT 1;

-- name: GetDigitalEmployeeRun :one
SELECT tr.*
FROM task_runs tr
JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND tr.id = sqlc.arg('run_id')::uuid
  AND t.deleted_at IS NULL;

-- name: GetDigitalEmployeeRunByCommandID :one
SELECT tr.*
FROM task_runs tr
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.command_id = sqlc.arg('command_id')::varchar;

-- name: ListDigitalEmployeeRuns :many
SELECT tr.*
FROM task_runs tr
JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND t.deleted_at IS NULL
ORDER BY tr.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: UpdateDigitalEmployeeRunStatus :one
UPDATE task_runs
SET status = sqlc.arg('status')::varchar,
    result = COALESCE(sqlc.narg('result')::jsonb, result),
    error_message = sqlc.narg('error_message')::text,
    diagnostic = COALESCE(sqlc.narg('diagnostic')::jsonb, diagnostic),
    log_ref = COALESCE(sqlc.narg('log_ref')::text, log_ref),
    raw_result_ref = COALESCE(sqlc.narg('raw_result_ref')::text, raw_result_ref),
    work_products = COALESCE(sqlc.narg('work_products')::jsonb, work_products),
    session_state = COALESCE(sqlc.narg('session_state')::jsonb, session_state),
    error_code = COALESCE(sqlc.narg('error_code')::varchar, error_code),
    error_family = COALESCE(sqlc.narg('error_family')::varchar, error_family),
    exit_code = COALESCE(sqlc.narg('exit_code')::integer, exit_code),
    signal = COALESCE(sqlc.narg('signal')::varchar, signal),
    timed_out = CASE WHEN sqlc.arg('status')::varchar = 'timed_out' THEN true ELSE timed_out END,
    provider_session_external_id = COALESCE(sqlc.narg('provider_session_external_id')::varchar, provider_session_external_id),
    completed_at = CASE
        WHEN sqlc.arg('status')::varchar = 'completed' THEN COALESCE(completed_at, NOW())
        ELSE completed_at
    END,
    finished_at = CASE
        WHEN sqlc.arg('status')::varchar IN ('completed', 'failed', 'cancelled', 'timed_out') THEN COALESCE(finished_at, NOW())
        ELSE finished_at
    END,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('run_id')::uuid
RETURNING *;

-- name: CreateTaskEventIfAbsent :one
WITH inserted AS (
    INSERT INTO task_events (
        tenant_id,
        task_id,
        run_id,
        event_type,
        sequence_number,
        payload,
        command_id,
        raw_event_ref,
        log_ref,
        metadata
    ) VALUES (
        sqlc.arg('tenant_id')::uuid,
        sqlc.arg('task_id')::uuid,
        sqlc.arg('run_id')::uuid,
        sqlc.arg('event_type')::varchar,
        sqlc.arg('sequence_number')::integer,
        COALESCE(sqlc.arg('payload')::jsonb, '{}'::jsonb),
        sqlc.narg('command_id')::varchar,
        sqlc.narg('raw_event_ref')::text,
        sqlc.narg('log_ref')::text,
        COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
    )
    ON CONFLICT DO NOTHING
    RETURNING *
)
SELECT * FROM inserted
UNION ALL
SELECT *
FROM task_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND run_id = sqlc.arg('run_id')::uuid
  AND sequence_number = sqlc.arg('sequence_number')::integer
LIMIT 1;
