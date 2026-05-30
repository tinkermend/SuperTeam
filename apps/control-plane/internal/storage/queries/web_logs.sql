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
