-- Task Queries

-- name: CreateTask :one
INSERT INTO tasks (
    title,
    description,
    creator_id,
    provider_type,
    target_node_id,
    status,
    workspace_path,
    params,
    priority
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks
WHERE id = $1;

-- name: ListTasks :many
SELECT * FROM tasks
WHERE ($1::varchar IS NULL OR status = $1)
  AND ($2::bigint IS NULL OR creator_id = $2)
  AND ($3::varchar IS NULL OR provider_type = $3)
ORDER BY priority DESC, created_at DESC
LIMIT $4 OFFSET $5;

-- name: UpdateTaskStatus :one
UPDATE tasks
SET status = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateTaskAssignment :one
UPDATE tasks
SET assigned_node_id = $2,
    status = $3,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ListPendingTasks :many
SELECT * FROM tasks
WHERE status = 'pending'
  AND ($1::varchar IS NULL OR provider_type = $1)
  AND (target_node_id IS NULL OR target_node_id = $2::varchar)
ORDER BY priority DESC, created_at ASC
LIMIT $3;

-- name: CancelTask :one
UPDATE tasks
SET status = 'cancelled',
    updated_at = NOW()
WHERE id = $1
  AND status NOT IN ('completed', 'failed', 'cancelled')
RETURNING *;

-- name: UpdateTaskWorkspace :one
UPDATE tasks
SET workspace_path = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- Task Execution Queries

-- name: CreateTaskExecution :one
INSERT INTO task_executions (
    task_id,
    node_id,
    provider_session_id,
    status,
    started_at
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: GetTaskExecution :one
SELECT * FROM task_executions
WHERE id = $1;

-- name: GetLatestTaskExecution :one
SELECT * FROM task_executions
WHERE task_id = $1
ORDER BY started_at DESC
LIMIT 1;

-- name: UpdateTaskExecution :one
UPDATE task_executions
SET status = $2,
    completed_at = $3,
    result = $4,
    error_message = $5
WHERE id = $1
RETURNING *;

-- name: ListTaskExecutions :many
SELECT * FROM task_executions
WHERE task_id = $1
ORDER BY started_at DESC;

-- Task State History Queries

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

-- name: ListTaskStateHistory :many
SELECT * FROM task_state_history
WHERE task_id = $1
ORDER BY created_at ASC;

-- Task Event Queries

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

-- name: ListTaskEvents :many
SELECT * FROM task_events
WHERE task_id = $1
  AND ($2::bigint IS NULL OR execution_id = $2)
ORDER BY sequence_number ASC
LIMIT $3 OFFSET $4;

-- name: GetLatestTaskEventSequence :one
SELECT COALESCE(MAX(sequence_number), 0) as max_sequence
FROM task_events
WHERE task_id = $1;

-- Task Artifact Queries

-- name: CreateTaskArtifact :one
INSERT INTO task_artifacts (
    task_id,
    execution_id,
    artifact_type,
    name,
    storage_url,
    size_bytes,
    metadata
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: ListTaskArtifacts :many
SELECT * FROM task_artifacts
WHERE task_id = $1
  AND ($2::bigint IS NULL OR execution_id = $2)
  AND ($3::varchar IS NULL OR artifact_type = $3)
ORDER BY created_at DESC;

-- name: GetTaskArtifact :one
SELECT * FROM task_artifacts
WHERE id = $1;

-- name: DeleteTaskArtifact :exec
DELETE FROM task_artifacts
WHERE id = $1;
