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

-- name: ListTaskEventsForRun :many
SELECT * FROM task_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND task_id = sqlc.arg('task_id')::uuid
  AND run_id = sqlc.arg('run_id')::uuid
ORDER BY created_at ASC, sequence_number ASC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: GetTaskEvent :one
SELECT * FROM task_events
WHERE task_id = sqlc.arg('task_id')::uuid
  AND sequence_number = sqlc.arg('sequence_number')::integer
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid);

-- name: CreateTaskRun :one
INSERT INTO task_runs (
    tenant_id,
    task_id,
    node_id,
    status,
    project_id
) VALUES (
    COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid),
    sqlc.arg('task_id')::uuid,
    sqlc.arg('node_id')::varchar,
    sqlc.arg('status')::varchar,
    sqlc.arg('project_id')::uuid
) RETURNING *;

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

-- name: GetTaskRun :one
SELECT * FROM task_runs
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid);

-- name: ListTaskRuns :many
SELECT * FROM task_runs
WHERE task_id = sqlc.arg('task_id')::uuid
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
ORDER BY created_at DESC;

-- name: ListTaskRunsByIDs :many
SELECT * FROM task_runs
WHERE id = ANY(sqlc.arg('ids')::uuid[])
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
ORDER BY created_at DESC;

-- name: GetLatestTaskRun :one
SELECT * FROM task_runs
WHERE task_id = sqlc.arg('task_id')::uuid
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdateTaskWorkspace :one
UPDATE tasks
SET workspace_path = sqlc.arg('workspace_path')::text, updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
RETURNING *;

-- name: CreateDigitalEmployeeTaskRun :one
WITH idempotency_input AS (
    SELECT
        sqlc.narg('idempotency_key')::varchar AS idempotency_key,
        sqlc.narg('idempotency_fingerprint')::varchar AS idempotency_fingerprint
),
idempotency_lock AS (
    SELECT pg_advisory_xact_lock(
        hashtextextended(
            sqlc.arg('tenant_id')::uuid::text || ':' ||
            sqlc.arg('digital_employee_id')::uuid::text || ':' ||
            idempotency_input.idempotency_key,
            0
        )
    ) AS locked
    FROM idempotency_input
    WHERE idempotency_input.idempotency_key IS NOT NULL
),
lock_barrier AS (
    SELECT 1 AS ready
    FROM idempotency_input
    WHERE idempotency_input.idempotency_key IS NULL
    UNION ALL
    SELECT 1 AS ready
    FROM idempotency_lock
    LIMIT 1
),
existing_run AS (
    SELECT
        t.id AS task_id,
        tr.id AS run_id,
        tr.command_id,
        t.status AS task_status,
        tr.status AS run_status
    FROM task_runs tr
    JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
    CROSS JOIN idempotency_input
    CROSS JOIN lock_barrier
    WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
      AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
      AND tr.idempotency_key = idempotency_input.idempotency_key
      AND tr.idempotency_fingerprint IS NOT DISTINCT FROM idempotency_input.idempotency_fingerprint
      AND idempotency_input.idempotency_key IS NOT NULL
      AND t.deleted_at IS NULL
    ORDER BY tr.created_at DESC
    LIMIT 1
),
conflicting_run AS (
    SELECT tr.id
    FROM task_runs tr
    CROSS JOIN idempotency_input
    CROSS JOIN lock_barrier
    WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
      AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
      AND tr.idempotency_key = idempotency_input.idempotency_key
      AND tr.idempotency_fingerprint IS DISTINCT FROM idempotency_input.idempotency_fingerprint
      AND idempotency_input.idempotency_key IS NOT NULL
    LIMIT 1
),
created_task AS (
    INSERT INTO tasks (
        id,
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
        risk_level,
        run_kind,
        resume_of_run_id,
        chat_thread_id,
        thread_title
    )
    SELECT
        CASE
            WHEN idempotency_input.idempotency_key IS NOT NULL THEN (
                SUBSTRING(idempotency_hash.value FROM 1 FOR 8) || '-' ||
                SUBSTRING(idempotency_hash.value FROM 9 FOR 4) || '-' ||
                SUBSTRING(idempotency_hash.value FROM 13 FOR 4) || '-' ||
                SUBSTRING(idempotency_hash.value FROM 17 FOR 4) || '-' ||
                SUBSTRING(idempotency_hash.value FROM 21 FOR 12)
            )::uuid
            ELSE gen_random_uuid()
        END,
        sqlc.arg('tenant_id')::uuid,
        sqlc.narg('team_id')::uuid,
        sqlc.arg('title')::varchar,
        sqlc.narg('description')::text,
        'pending',
        sqlc.arg('priority')::integer,
        sqlc.arg('provider_type')::varchar,
        sqlc.narg('creator_id')::uuid,
        sqlc.arg('target_node_id')::varchar,
        sqlc.narg('workspace_path')::text,
        COALESCE(sqlc.arg('params')::jsonb, '{}'::jsonb),
        idempotency_input.idempotency_key,
        COALESCE(sqlc.narg('risk_level')::varchar, 'normal'),
        sqlc.arg('run_kind')::varchar,
        sqlc.narg('resume_of_run_id')::uuid,
        sqlc.narg('chat_thread_id')::uuid,
        sqlc.narg('thread_title')::text
    FROM idempotency_input
    CROSS JOIN lock_barrier
    CROSS JOIN LATERAL (
        SELECT md5(
            sqlc.arg('tenant_id')::uuid::text || ':' ||
            sqlc.arg('digital_employee_id')::uuid::text || ':' ||
            COALESCE(idempotency_input.idempotency_key, gen_random_uuid()::text)
        ) AS value
    ) AS idempotency_hash
    WHERE NOT EXISTS (SELECT 1 FROM existing_run)
      AND NOT EXISTS (SELECT 1 FROM conflicting_run)
    ON CONFLICT (id) DO UPDATE SET id = tasks.id
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
        idempotency_fingerprint,
        timeout_sec,
        grace_sec,
        provider_type,
        project_id
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
        idempotency_input.idempotency_key,
        idempotency_input.idempotency_fingerprint,
        sqlc.narg('timeout_sec')::integer,
        sqlc.narg('grace_sec')::integer,
        sqlc.arg('provider_type')::varchar,
        sqlc.arg('project_id')::uuid
    FROM created_task
    CROSS JOIN idempotency_input
    WHERE NOT EXISTS (SELECT 1 FROM existing_run)
      AND NOT EXISTS (SELECT 1 FROM conflicting_run)
    ON CONFLICT (tenant_id, digital_employee_id, idempotency_key)
    WHERE digital_employee_id IS NOT NULL AND idempotency_key IS NOT NULL
    DO UPDATE SET updated_at = task_runs.updated_at
    WHERE task_runs.idempotency_fingerprint IS NOT DISTINCT FROM EXCLUDED.idempotency_fingerprint
    RETURNING *
)
SELECT
    existing_run.task_id,
    existing_run.run_id,
    existing_run.command_id,
    existing_run.task_status,
    existing_run.run_status
FROM existing_run
UNION ALL
SELECT
    created_task.id AS task_id,
    created_run.id AS run_id,
    created_run.command_id,
    created_task.status AS task_status,
    created_run.status AS run_status
FROM created_run
JOIN created_task ON created_task.id = created_run.task_id
UNION ALL
SELECT
    t.id AS task_id,
    created_run.id AS run_id,
    created_run.command_id,
    t.status AS task_status,
    created_run.status AS run_status
FROM created_run
JOIN tasks t ON t.id = created_run.task_id AND t.tenant_id = created_run.tenant_id
WHERE NOT EXISTS (
    SELECT 1
    FROM created_task
    WHERE created_task.id = created_run.task_id
)
LIMIT 1;

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

-- name: ListStalePreConfirmationDigitalEmployeeRuns :many
-- 看门狗清扫(残债交接 §1 第 2 层):跨租户列出停留在预确认态超过时限的
-- run。running/cancelling 是真实活跃态,不在清扫范围。
SELECT tr.*
FROM task_runs tr
WHERE tr.status IN ('queued', 'dispatching')
  AND tr.updated_at < sqlc.arg('stale_before')::timestamptz
ORDER BY tr.updated_at ASC
LIMIT sqlc.arg('batch_limit')::int;

-- name: GetDigitalEmployeeRun :one
SELECT
    tr.*,
    t.run_kind,
    t.resume_of_run_id,
    t.chat_thread_id,
    p.name AS project_name,
    (p.deleted_at IS NOT NULL)::boolean AS project_deleted
FROM task_runs tr
JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
-- 运行必须归属项目 spec（2026-07-26）：归属读一等列 task_runs.project_id（NOT NULL），
-- 不再经 project_tasks 指针 + metadata 锚点两级 join 兜底。
-- Soft-deleted projects still resolve so run history keeps the name + deleted flag.
LEFT JOIN projects p
  ON p.tenant_id = tr.tenant_id
 AND p.id = tr.project_id
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
SELECT tr.*, t.run_kind, t.resume_of_run_id
FROM task_runs tr
JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND t.deleted_at IS NULL
  AND (sqlc.narg('run_kind')::varchar IS NULL OR t.run_kind = sqlc.narg('run_kind')::varchar)
ORDER BY tr.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: GetDigitalEmployeeRunStats :one
WITH scoped AS (
    SELECT status, started_at, finished_at, created_at
    FROM task_runs
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
),
counts AS (
    SELECT
        COUNT(*)::bigint AS total_count,
        COUNT(*) FILTER (WHERE status = 'completed')::bigint AS succeeded_count,
        COUNT(*) FILTER (WHERE status IN ('failed', 'timed_out'))::bigint AS failed_count,
        COUNT(*) FILTER (WHERE status = 'cancelled')::bigint AS cancelled_count,
        COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '7 days')::bigint AS last_7d_count,
        COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '14 days' AND created_at < NOW() - INTERVAL '7 days')::bigint AS prev_7d_count
    FROM scoped
),
durations AS (
    SELECT
        AVG(EXTRACT(EPOCH FROM (finished_at - started_at))) AS avg_duration_sec,
        PERCENTILE_CONT(0.9) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (finished_at - started_at))) AS p90_duration_sec
    FROM scoped
    WHERE finished_at IS NOT NULL
)
SELECT
    counts.total_count,
    counts.succeeded_count,
    counts.failed_count,
    counts.cancelled_count,
    counts.last_7d_count,
    counts.prev_7d_count,
    durations.avg_duration_sec,
    durations.p90_duration_sec
FROM counts
LEFT JOIN durations ON true;

-- name: ListDigitalEmployeeRunCalendarItems :many
-- 日历看板轻量投影:不含 result/diagnostic/session_state/work_products。
SELECT
    tr.id,
    tr.status,
    tr.created_at,
    tr.project_id,
    t.title AS task_title,
    t.run_kind,
    p.name AS project_name,
    (p.deleted_at IS NOT NULL)::boolean AS project_deleted
FROM task_runs tr
JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
LEFT JOIN projects p
  ON p.tenant_id = tr.tenant_id
 AND p.id = tr.project_id
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND t.deleted_at IS NULL
  AND tr.created_at >= sqlc.arg('from_time')::timestamptz
  AND tr.created_at < sqlc.arg('to_time')::timestamptz
ORDER BY tr.created_at DESC
LIMIT sqlc.arg('limit');

-- name: CountDigitalEmployeeRunCalendarItems :one
SELECT COUNT(*)::bigint AS total_count
FROM task_runs tr
JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND t.deleted_at IS NULL
  AND tr.created_at >= sqlc.arg('from_time')::timestamptz
  AND tr.created_at < sqlc.arg('to_time')::timestamptz;

-- name: ListDigitalEmployeeRunsDetailed :many
SELECT
    tr.id, tr.tenant_id, tr.task_id, tr.digital_employee_id, tr.execution_instance_id,
    tr.runtime_node_id, tr.node_id, tr.command_id, tr.provider_type, tr.provider_session_id,
    tr.provider_session_external_id, tr.status, tr.result, tr.diagnostic, tr.log_ref,
    tr.raw_result_ref, tr.work_products, tr.session_state, tr.error_message, tr.error_code,
    tr.error_family, tr.exit_code, tr.signal, tr.timed_out, tr.idempotency_key,
    tr.timeout_sec, tr.grace_sec, tr.started_at, tr.completed_at, tr.finished_at,
    tr.created_at, tr.updated_at, tr.failure_acknowledged_at, tr.failure_acknowledged_by,
    tr.project_id,
    t.title AS task_title,
    t.run_kind,
    t.resume_of_run_id,
    t.chat_thread_id,
    t.creator_id,
    COALESCE(
      NULLIF(creator.display_name, ''),
      NULLIF(creator.username, ''),
      t.creator_id::text
    )::text AS creator_display_name,
    p.name AS project_name,
    (p.deleted_at IS NOT NULL)::boolean AS project_deleted,
    jsonb_array_length(tr.work_products) AS work_product_count
FROM task_runs tr
JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
LEFT JOIN projects p
  ON p.tenant_id = tr.tenant_id
 AND p.id = tr.project_id
LEFT JOIN auth_users creator
  ON creator.id = t.creator_id AND creator.deleted_at IS NULL
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND t.deleted_at IS NULL
  AND (cardinality(sqlc.arg('statuses')::text[]) = 0 OR tr.status = ANY(sqlc.arg('statuses')::text[]))
  AND (sqlc.narg('project_id')::uuid IS NULL
       OR tr.project_id = sqlc.narg('project_id')::uuid)
  AND (sqlc.narg('from_time')::timestamptz IS NULL OR tr.created_at >= sqlc.narg('from_time')::timestamptz)
  AND (sqlc.narg('to_time')::timestamptz IS NULL OR tr.created_at < sqlc.narg('to_time')::timestamptz)
  AND (sqlc.narg('run_kind')::varchar IS NULL OR t.run_kind = sqlc.narg('run_kind')::varchar)
  AND (sqlc.narg('chat_thread_id')::uuid IS NULL
       OR (t.run_kind = 'chat' AND (t.chat_thread_id = sqlc.narg('chat_thread_id')::uuid OR tr.id = sqlc.narg('chat_thread_id')::uuid)))
ORDER BY tr.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountDigitalEmployeeRunsDetailed :one
SELECT COUNT(*)::bigint AS total_count
FROM task_runs tr
JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
LEFT JOIN projects p
  ON p.tenant_id = tr.tenant_id
 AND p.id = tr.project_id
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND t.deleted_at IS NULL
  AND (cardinality(sqlc.arg('statuses')::text[]) = 0 OR tr.status = ANY(sqlc.arg('statuses')::text[]))
  -- 与 ListDigitalEmployeeRunsDetailed 的过滤语义保持一致。
  AND (sqlc.narg('project_id')::uuid IS NULL
       OR tr.project_id = sqlc.narg('project_id')::uuid)
  AND (sqlc.narg('from_time')::timestamptz IS NULL OR tr.created_at >= sqlc.narg('from_time')::timestamptz)
  AND (sqlc.narg('to_time')::timestamptz IS NULL OR tr.created_at < sqlc.narg('to_time')::timestamptz)
  AND (sqlc.narg('run_kind')::varchar IS NULL OR t.run_kind = sqlc.narg('run_kind')::varchar)
  AND (sqlc.narg('chat_thread_id')::uuid IS NULL
       OR (t.run_kind = 'chat' AND (t.chat_thread_id = sqlc.narg('chat_thread_id')::uuid OR tr.id = sqlc.narg('chat_thread_id')::uuid)));

-- name: ListDigitalEmployeeRunProjectOptions :many
-- 运行必须归属项目 spec（2026-07-26）：归属读一等列，原「指针 UNION metadata 锚点」双路收敛。
SELECT DISTINCT p.id, p.name, (p.deleted_at IS NOT NULL)::boolean AS project_deleted
FROM task_runs tr
JOIN projects p ON p.tenant_id = tr.tenant_id AND p.id = tr.project_id
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
ORDER BY name;

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
    ON CONFLICT (tenant_id, run_id, sequence_number)
    WHERE run_id IS NOT NULL
    DO UPDATE SET event_type = task_events.event_type
    RETURNING *, (xmax = 0) AS inserted
)
SELECT * FROM inserted;

-- name: ListDigitalEmployeeChatThreads :many
-- 聚合 (employee, project) 下全部 chat 会话；标题/发起人/接续人/末问/活跃态。
WITH chat_runs AS (
  SELECT
    tr.id AS run_id,
    tr.status,
    tr.created_at,
    tr.updated_at,
    tr.project_id,
    t.title AS task_title,
    t.creator_id,
    t.thread_title,
    t.chat_thread_id AS stored_thread_id,
    COALESCE(t.chat_thread_id, tr.id) AS thread_id,
    (t.chat_thread_id IS NULL) AS is_root
  FROM task_runs tr
  JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
  WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
    AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
    AND tr.project_id = sqlc.arg('project_id')::uuid
    AND t.run_kind = 'chat'
    AND t.deleted_at IS NULL
),
thread_agg AS (
  SELECT
    thread_id,
    MAX(updated_at) AS last_active_at,
    BOOL_OR(status IN ('queued', 'dispatching', 'running', 'cancelling')) AS has_active_run
  FROM chat_runs
  GROUP BY thread_id
),
root_rows AS (
  SELECT DISTINCT ON (thread_id)
    thread_id,
    run_id AS root_run_id,
    creator_id AS initiator_user_id,
    thread_title,
    task_title AS root_task_title
  FROM chat_runs
  WHERE is_root
  ORDER BY thread_id, created_at ASC
),
last_human AS (
  SELECT DISTINCT ON (thread_id)
    thread_id,
    creator_id AS last_speaker_user_id,
    task_title AS last_prompt
  FROM chat_runs
  ORDER BY thread_id, created_at DESC
),
active_runner AS (
  SELECT DISTINCT ON (thread_id)
    thread_id,
    creator_id AS active_runner_user_id
  FROM chat_runs
  WHERE status IN ('queued', 'dispatching', 'running', 'cancelling')
  ORDER BY thread_id, created_at DESC
)
SELECT
  ta.thread_id AS chat_thread_id,
  COALESCE(NULLIF(rr.thread_title, ''), rr.root_task_title)::text AS thread_title,
  rr.initiator_user_id,
  COALESCE(
    NULLIF(initiator.display_name, ''),
    NULLIF(initiator.username, ''),
    rr.initiator_user_id::text
  )::text AS initiator_display_name,
  lh.last_speaker_user_id,
  COALESCE(
    NULLIF(speaker.display_name, ''),
    NULLIF(speaker.username, ''),
    lh.last_speaker_user_id::text
  )::text AS last_speaker_display_name,
  lh.last_prompt,
  ta.last_active_at,
  ta.has_active_run,
  ar.active_runner_user_id,
  CASE
    WHEN ar.active_runner_user_id IS NULL THEN ''::text
    ELSE COALESCE(
      NULLIF(runner.display_name, ''),
      NULLIF(runner.username, ''),
      ar.active_runner_user_id::text
    )::text
  END AS active_runner_display_name
FROM thread_agg ta
JOIN root_rows rr ON rr.thread_id = ta.thread_id
JOIN last_human lh ON lh.thread_id = ta.thread_id
LEFT JOIN active_runner ar ON ar.thread_id = ta.thread_id
LEFT JOIN auth_users initiator
  ON initiator.id = rr.initiator_user_id AND initiator.deleted_at IS NULL
LEFT JOIN auth_users speaker
  ON speaker.id = lh.last_speaker_user_id AND speaker.deleted_at IS NULL
LEFT JOIN auth_users runner
  ON runner.id = ar.active_runner_user_id AND runner.deleted_at IS NULL
ORDER BY ta.last_active_at DESC;

-- name: GetDigitalEmployeeChatThreadRoot :one
SELECT
  tr.id AS root_run_id,
  tr.project_id,
  t.id AS task_id,
  t.creator_id AS initiator_user_id,
  t.thread_title,
  t.title AS root_task_title,
  COALESCE(
    NULLIF(u.display_name, ''),
    NULLIF(u.username, ''),
    t.creator_id::text
  )::text AS initiator_display_name
FROM task_runs tr
JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
LEFT JOIN auth_users u
  ON u.id = t.creator_id AND u.deleted_at IS NULL
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND tr.id = sqlc.arg('thread_id')::uuid
  AND t.run_kind = 'chat'
  AND t.chat_thread_id IS NULL
  AND t.deleted_at IS NULL;

-- name: UpdateChatThreadTitle :one
UPDATE tasks t
SET thread_title = sqlc.arg('thread_title')::text,
    updated_at = NOW()
FROM task_runs tr
WHERE t.id = tr.task_id
  AND t.tenant_id = tr.tenant_id
  AND tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND tr.id = sqlc.arg('thread_id')::uuid
  AND t.run_kind = 'chat'
  AND t.chat_thread_id IS NULL
  AND t.deleted_at IS NULL
RETURNING t.id, t.thread_title, t.creator_id;

-- name: GetActiveChatRunOnThread :one
SELECT
  tr.id AS run_id,
  t.creator_id AS runner_user_id,
  COALESCE(
    NULLIF(u.display_name, ''),
    NULLIF(u.username, ''),
    t.creator_id::text
  )::text AS runner_display_name,
  tr.status
FROM task_runs tr
JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
LEFT JOIN auth_users u
  ON u.id = t.creator_id AND u.deleted_at IS NULL
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND t.run_kind = 'chat'
  AND t.deleted_at IS NULL
  AND tr.status IN ('queued', 'dispatching', 'running', 'cancelling')
  AND (t.chat_thread_id = sqlc.arg('thread_id')::uuid OR tr.id = sqlc.arg('thread_id')::uuid)
ORDER BY tr.created_at DESC
LIMIT 1;
