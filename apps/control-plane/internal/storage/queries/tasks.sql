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
