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

-- name: CreateTaskStateHistory :exec
INSERT INTO task_state_history (
    task_id,
    from_status,
    to_status,
    changed_by,
    reason
) VALUES (
    $1, $2, $3, $4, $5
);
