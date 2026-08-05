-- name: ListRoleVocabulary :many
SELECT * FROM role_vocabulary
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
ORDER BY role_key ASC;

-- name: ListActiveRoleVocabulary :many
SELECT * FROM role_vocabulary
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
  AND status = 'active'
ORDER BY role_key ASC;

-- name: GetRoleVocabularyByKey :one
SELECT * FROM role_vocabulary
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND role_key = sqlc.arg('role_key')::text
  AND deleted_at IS NULL;

-- name: GetActiveRoleVocabularyByKeys :many
SELECT * FROM role_vocabulary
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND role_key = ANY(sqlc.arg('role_keys')::text[])
  AND deleted_at IS NULL
  AND status = 'active';

-- name: CreateRoleVocabulary :one
INSERT INTO role_vocabulary (
    id,
    tenant_id,
    role_key,
    title,
    description,
    status
) VALUES (
    sqlc.arg('id')::uuid,
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('role_key')::text,
    sqlc.arg('title')::text,
    COALESCE(sqlc.narg('description')::text, ''),
    COALESCE(sqlc.narg('status')::varchar, 'active')
) RETURNING *;

-- name: UpdateRoleVocabulary :one
UPDATE role_vocabulary
SET
    title = COALESCE(sqlc.narg('title')::text, title),
    description = COALESCE(sqlc.narg('description')::text, description),
    status = COALESCE(sqlc.narg('status')::varchar, status),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND role_key = sqlc.arg('role_key')::text
  AND deleted_at IS NULL
RETURNING *;
