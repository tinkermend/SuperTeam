-- name: CreateTask :one
INSERT INTO tasks (
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
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks
WHERE id = $1;

-- name: UpdateTaskStatus :one
UPDATE tasks
SET status = $2, updated_at = NOW()
WHERE id = $1
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
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: ListTasks :many
SELECT * FROM tasks
WHERE (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('creator_id')::bigint IS NULL OR creator_id = sqlc.narg('creator_id'))
  AND (sqlc.narg('provider_type')::varchar IS NULL OR provider_type = sqlc.narg('provider_type'))
ORDER BY priority DESC, created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: DeleteTask :exec
DELETE FROM tasks
WHERE id = $1;

-- name: CreateTaskEvent :one
INSERT INTO task_events (
    task_id,
    execution_id,
    event_type,
    sequence_number,
    payload
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: GetLatestTaskEventSequence :one
SELECT COALESCE(MAX(sequence_number), 0)::integer as max_sequence
FROM task_events
WHERE task_id = $1;

-- name: ListTaskEvents :many
SELECT * FROM task_events
WHERE task_id = $1
ORDER BY sequence_number ASC;

-- name: GetTaskEvent :one
SELECT * FROM task_events
WHERE task_id = $1 AND sequence_number = $2;

-- name: CreateTaskStateHistory :one
INSERT INTO task_state_history (
    task_id,
    from_status,
    to_status,
    changed_by,
    reason
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: CreateTaskExecution :one
INSERT INTO task_executions (
    task_id,
    node_id,
    status
) VALUES (
    $1, $2, $3
) RETURNING *;

-- name: CreateTaskArtifact :one
INSERT INTO task_artifacts (
    task_id,
    execution_id,
    artifact_type,
    name,
    storage_url
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: ListTaskArtifacts :many
SELECT * FROM task_artifacts
WHERE task_id = $1
ORDER BY created_at DESC;

-- name: ListPendingTasks :many
SELECT * FROM tasks
WHERE status = 'pending'
  AND (sqlc.narg('provider_type')::varchar IS NULL OR provider_type = sqlc.narg('provider_type'))
ORDER BY priority DESC, created_at ASC
LIMIT sqlc.arg('limit');

-- name: UpdateTaskAssignment :one
UPDATE tasks
SET assigned_node_id = $2, status = 'claimed', updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateTaskExecution :one
UPDATE task_executions
SET status = $2, completed_at = NOW(), updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: CancelTask :one
UPDATE tasks
SET status = 'cancelled', updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: GetTaskArtifact :one
SELECT * FROM task_artifacts
WHERE id = $1;

-- name: DeleteTaskArtifact :exec
DELETE FROM task_artifacts
WHERE id = $1;

-- name: GetTaskExecution :one
SELECT * FROM task_executions
WHERE id = $1;

-- name: ListTaskExecutions :many
SELECT * FROM task_executions
WHERE task_id = $1
ORDER BY created_at DESC;

-- name: GetLatestTaskExecution :one
SELECT * FROM task_executions
WHERE task_id = $1
ORDER BY created_at DESC
LIMIT 1;

-- name: ListTaskStateHistory :many
SELECT * FROM task_state_history
WHERE task_id = $1
ORDER BY created_at DESC;

-- name: UpdateTaskWorkspace :one
UPDATE tasks
SET workspace_path = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;
