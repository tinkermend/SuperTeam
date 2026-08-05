-- name: ListProjectPlaybookCastings :many
SELECT *
FROM project_playbook_casting
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND (
    sqlc.narg('scenario_template_key')::text IS NULL
    OR scenario_template_key = sqlc.narg('scenario_template_key')::text
  )
ORDER BY scenario_template_key ASC, role_key ASC;

-- name: ListProjectPlaybookCastingsByTemplate :many
SELECT *
FROM project_playbook_casting
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND scenario_template_key = sqlc.arg('scenario_template_key')::text
ORDER BY role_key ASC;

-- name: DeleteProjectPlaybookCastingsByTemplate :exec
DELETE FROM project_playbook_casting
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND scenario_template_key = sqlc.arg('scenario_template_key')::text;

-- name: InsertProjectPlaybookCasting :one
INSERT INTO project_playbook_casting (
    id,
    tenant_id,
    project_id,
    scenario_template_key,
    role_key,
    digital_employee_id,
    cast_by_user_id
) VALUES (
    sqlc.arg('id')::uuid,
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('scenario_template_key')::text,
    sqlc.arg('role_key')::text,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('cast_by_user_id')::uuid
) RETURNING *;

-- name: CountProjectPlaybookCastingsForEmployee :one
SELECT COUNT(*)::int AS count
FROM project_playbook_casting
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid;

-- name: ListProjectsCastingEmployee :many
SELECT DISTINCT project_id
FROM project_playbook_casting
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid;
