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

-- name: ListCastingsForEmployeeRoles :many
-- Impact preview: castings held by employee for the given role_keys.
-- Empty role_keys array means all roles for that employee.
SELECT
    c.id,
    c.tenant_id,
    c.project_id,
    c.scenario_template_key,
    c.role_key,
    c.digital_employee_id,
    c.cast_by_user_id,
    c.created_at,
    c.updated_at,
    p.name AS project_name,
    COALESCE(st.name, c.scenario_template_key) AS template_name
FROM project_playbook_casting c
JOIN projects p ON p.id = c.project_id AND p.tenant_id = c.tenant_id
LEFT JOIN scenario_templates st
  ON st.tenant_id = c.tenant_id
 AND st.template_key = c.scenario_template_key
 AND st.deleted_at IS NULL
WHERE c.tenant_id = sqlc.arg('tenant_id')::uuid
  AND c.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND (
    COALESCE(cardinality(sqlc.arg('role_keys')::text[]), 0) = 0
    OR c.role_key = ANY(sqlc.arg('role_keys')::text[])
  )
ORDER BY p.name ASC, c.scenario_template_key ASC, c.role_key ASC;

-- name: DeleteCastingsForEmployeeRoles :exec
DELETE FROM project_playbook_casting
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND role_key = ANY(sqlc.arg('role_keys')::text[]);

-- name: ListCastingsForRoleKey :many
SELECT
    c.id,
    c.tenant_id,
    c.project_id,
    c.scenario_template_key,
    c.role_key,
    c.digital_employee_id,
    c.cast_by_user_id,
    c.created_at,
    c.updated_at,
    p.name AS project_name,
    COALESCE(st.name, c.scenario_template_key) AS template_name
FROM project_playbook_casting c
JOIN projects p ON p.id = c.project_id AND p.tenant_id = c.tenant_id
LEFT JOIN scenario_templates st
  ON st.tenant_id = c.tenant_id
 AND st.template_key = c.scenario_template_key
 AND st.deleted_at IS NULL
WHERE c.tenant_id = sqlc.arg('tenant_id')::uuid
  AND c.role_key = sqlc.arg('role_key')::text
ORDER BY p.name ASC, c.scenario_template_key ASC;

-- name: DeleteCastingsForRoleKey :exec
DELETE FROM project_playbook_casting
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND role_key = sqlc.arg('role_key')::text;
