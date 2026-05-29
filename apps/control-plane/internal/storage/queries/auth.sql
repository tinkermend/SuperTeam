-- Auth Queries

-- User Queries

-- name: CreateUser :one
INSERT INTO auth_users (
    username,
    display_name,
    email,
    password_hash,
    status
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: GetUserByUsername :one
SELECT * FROM auth_users
WHERE username = $1;

-- name: GetUserByID :one
SELECT * FROM auth_users
WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM auth_users
WHERE email = $1;

-- name: ListUsers :many
SELECT * FROM auth_users
WHERE ($1::varchar IS NULL OR status = $1)
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: UpdateUser :one
UPDATE auth_users
SET display_name = COALESCE($2, display_name),
    email = COALESCE($3, email),
    status = COALESCE($4, status),
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateUserPassword :one
UPDATE auth_users
SET password_hash = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteUser :exec
DELETE FROM auth_users
WHERE id = $1;

-- Runtime Token Queries

-- name: CreateRuntimeToken :one
INSERT INTO auth_runtime_tokens (
    node_id,
    token_hash,
    expires_at
) VALUES (
    $1, $2, $3
) RETURNING *;

-- name: GetRuntimeTokenByNodeID :one
SELECT * FROM auth_runtime_tokens
WHERE node_id = $1;

-- name: ValidateRuntimeToken :one
SELECT * FROM auth_runtime_tokens
WHERE node_id = $1
  AND (expires_at IS NULL OR expires_at > NOW());

-- name: DeleteRuntimeToken :exec
DELETE FROM auth_runtime_tokens
WHERE node_id = $1;

-- name: DeleteExpiredRuntimeTokens :exec
DELETE FROM auth_runtime_tokens
WHERE expires_at IS NOT NULL
  AND expires_at < NOW();

-- name: ListRuntimeTokens :many
SELECT * FROM auth_runtime_tokens
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;
