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

-- name: ListScenarioTemplatesReferencingRole :many
-- Active/disabled templates whose current spec roles[].key includes role_key.
SELECT template_key, name
FROM scenario_templates
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
  AND EXISTS (
    SELECT 1
    FROM jsonb_array_elements(COALESCE(spec->'roles', '[]'::jsonb)) AS role_elem
    WHERE role_elem->>'key' = sqlc.arg('role_key')::text
  )
ORDER BY template_key ASC;

-- name: ListEmployeesHoldingRole :many
-- Non-deleted employees that hold role_key (any status; for disable impact).
SELECT de.id, de.name
FROM digital_employee_roles der
JOIN digital_employees de ON de.id = der.digital_employee_id AND de.tenant_id = der.tenant_id
WHERE der.tenant_id = sqlc.arg('tenant_id')::uuid
  AND der.role_key = sqlc.arg('role_key')::text
  AND de.deleted_at IS NULL
ORDER BY de.name ASC;

-- name: CountCastingsForRole :one
SELECT COUNT(*)::int AS count
FROM project_playbook_casting
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND role_key = sqlc.arg('role_key')::text;

-- name: CountEmployeesHoldingRole :one
SELECT COUNT(*)::int AS count
FROM digital_employee_roles der
JOIN digital_employees de ON de.id = der.digital_employee_id AND de.tenant_id = der.tenant_id
WHERE der.tenant_id = sqlc.arg('tenant_id')::uuid
  AND der.role_key = sqlc.arg('role_key')::text
  AND de.deleted_at IS NULL
  AND de.status IN ('ready', 'active');
