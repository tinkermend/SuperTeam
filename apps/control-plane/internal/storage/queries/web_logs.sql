-- name: CreateWebLoginLog :one
INSERT INTO web_login_logs (
    event_type,
    user_id,
    username,
    session_id,
    client_ip,
    user_agent,
    result,
    failure_reason,
    details
) VALUES (
    sqlc.arg('event_type')::varchar,
    sqlc.narg('user_id')::bigint,
    sqlc.arg('username')::varchar,
    sqlc.narg('session_id')::varchar,
    sqlc.narg('client_ip')::varchar,
    sqlc.narg('user_agent')::text,
    sqlc.arg('result')::varchar,
    sqlc.narg('failure_reason')::varchar,
    COALESCE(sqlc.narg('details')::jsonb, '{}'::jsonb)
) RETURNING *;

-- name: ListWebLoginLogs :many
SELECT * FROM web_login_logs
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CreateWebOperationLog :one
INSERT INTO web_operation_logs (
    user_id,
    username,
    module,
    resource_type,
    resource_id,
    action,
    result,
    request_id,
    client_ip,
    user_agent,
    details
) VALUES (
    sqlc.narg('user_id')::bigint,
    sqlc.narg('username')::varchar,
    sqlc.arg('module')::varchar,
    sqlc.narg('resource_type')::varchar,
    sqlc.narg('resource_id')::varchar,
    sqlc.arg('action')::varchar,
    sqlc.arg('result')::varchar,
    sqlc.narg('request_id')::varchar,
    sqlc.narg('client_ip')::varchar,
    sqlc.narg('user_agent')::text,
    COALESCE(sqlc.narg('details')::jsonb, '{}'::jsonb)
) RETURNING *;
