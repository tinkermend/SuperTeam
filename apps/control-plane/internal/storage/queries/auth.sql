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
    password_hash = COALESCE($4, password_hash),
    status = COALESCE($5, status),
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ListUsers :many
SELECT * FROM auth_users
WHERE ($1::varchar IS NULL OR status = $1)
ORDER BY created_at DESC
LIMIT $3 OFFSET $2;

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
SET password_hash = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM auth_users
WHERE id = $1;
