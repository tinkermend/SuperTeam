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
    disabled_at = CASE
        WHEN $4::varchar = 'disabled' THEN COALESCE(disabled_at, NOW())
        WHEN $4::varchar = 'active' THEN NULL
        ELSE disabled_at
    END,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ListUsers :many
SELECT * FROM auth_users
WHERE deleted_at IS NULL
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)
  AND (
    sqlc.narg('q')::text IS NULL
    OR username ILIKE '%' || sqlc.narg('q')::text || '%'
    OR COALESCE(display_name, '') ILIKE '%' || sqlc.narg('q')::text || '%'
    OR COALESCE(email, '') ILIKE '%' || sqlc.narg('q')::text || '%'
  )
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: DeleteUser :exec
UPDATE auth_users
SET deleted_at = COALESCE(deleted_at, NOW()),
    disabled_at = COALESCE(disabled_at, NOW()),
    status = 'disabled',
    updated_at = NOW()
WHERE id = $1;

-- name: CreateRuntimeToken :one
INSERT INTO auth_runtime_tokens (
    node_id,
    token_hash,
    expires_at
) VALUES (
    $1, $2, $3
)
ON CONFLICT (node_id) WHERE revoked_at IS NULL DO UPDATE SET
    token_hash = EXCLUDED.token_hash,
    expires_at = EXCLUDED.expires_at,
    revoked_at = NULL
RETURNING *;

-- name: GetRuntimeToken :one
SELECT * FROM auth_runtime_tokens
WHERE node_id = $1;

-- name: ValidateRuntimeToken :one
SELECT * FROM auth_runtime_tokens
WHERE node_id = $1
  AND token_hash = $2
  AND revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > NOW());

-- name: DeleteRuntimeToken :exec
UPDATE auth_runtime_tokens
SET revoked_at = COALESCE(revoked_at, NOW())
WHERE node_id = $1;

-- name: GetRuntimeTokenByNodeID :one
SELECT * FROM auth_runtime_tokens
WHERE node_id = $1
  AND revoked_at IS NULL;

-- name: ListRuntimeTokens :many
SELECT * FROM auth_runtime_tokens
WHERE revoked_at IS NULL
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
    user_id,
    token_hash,
    expires_at,
    last_seen_at,
    client_ip,
    user_agent
) VALUES (
    sqlc.arg('user_id')::uuid,
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
