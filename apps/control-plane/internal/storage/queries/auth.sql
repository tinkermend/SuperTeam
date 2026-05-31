-- name: CreateUser :one
INSERT INTO auth_users (
    username,
    display_name,
    email,
    password_hash,
    status
) VALUES (
    sqlc.arg('username')::varchar,
    sqlc.narg('display_name')::varchar,
    sqlc.narg('email')::varchar,
    sqlc.arg('password_hash')::varchar,
    sqlc.arg('status')::varchar
) RETURNING *;

-- name: GetUser :one
SELECT * FROM auth_users
WHERE id = $1;

-- name: GetUserByUsername :one
SELECT * FROM auth_users
WHERE username = $1;

-- name: GetUserByEmail :one
SELECT * FROM auth_users
WHERE email = $1;

-- name: UpdateUser :one
UPDATE auth_users
SET
    display_name = COALESCE($2, display_name),
    email = COALESCE($3, email),
    status = COALESCE($4, status),
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ListUsers :many
SELECT * FROM auth_users
WHERE (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: DeleteUser :exec
DELETE FROM auth_users
WHERE id = $1;

-- name: CreateRuntimeToken :one
INSERT INTO auth_runtime_tokens (
    node_id,
    token_hash,
    expires_at
) VALUES (
    $1, $2, $3
) RETURNING *;

-- name: GetRuntimeToken :one
SELECT * FROM auth_runtime_tokens
WHERE node_id = $1;

-- name: ValidateRuntimeToken :one
SELECT * FROM auth_runtime_tokens
WHERE node_id = $1
  AND token_hash = $2
  AND (expires_at IS NULL OR expires_at > NOW());

-- name: DeleteRuntimeToken :exec
DELETE FROM auth_runtime_tokens
WHERE node_id = $1;

-- name: GetRuntimeTokenByNodeID :one
SELECT * FROM auth_runtime_tokens
WHERE node_id = $1;

-- name: ListRuntimeTokens :many
SELECT * FROM auth_runtime_tokens
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: DeleteExpiredRuntimeTokens :exec
DELETE FROM auth_runtime_tokens
WHERE expires_at IS NOT NULL AND expires_at < NOW();

-- name: UpdateUserPassword :one
UPDATE auth_users
SET password_hash = sqlc.arg('password_hash')::varchar, updated_at = NOW()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM auth_users
WHERE id = $1;

-- name: CreateSession :one
INSERT INTO auth_sessions (
    id,
    user_id,
    token_hash,
    expires_at,
    last_seen_at,
    client_ip,
    user_agent
) VALUES (
    sqlc.arg('id')::varchar,
    sqlc.arg('user_id'),
    sqlc.arg('token_hash')::varchar,
    sqlc.arg('expires_at'),
    sqlc.arg('last_seen_at'),
    sqlc.narg('client_ip')::varchar,
    sqlc.narg('user_agent')::text
) RETURNING *;

-- name: GetSessionByTokenHash :one
SELECT * FROM auth_sessions
WHERE token_hash = $1;

-- name: DeleteSessionByTokenHash :exec
DELETE FROM auth_sessions
WHERE token_hash = $1;

-- name: UpdateSessionLastSeen :one
UPDATE auth_sessions
SET last_seen_at = $2
WHERE token_hash = $1
RETURNING *;

-- name: DeleteExpiredSessions :exec
DELETE FROM auth_sessions
WHERE expires_at < NOW();
