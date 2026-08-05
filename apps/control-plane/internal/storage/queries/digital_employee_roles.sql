-- name: ListDigitalEmployeeRoles :many
SELECT tenant_id, digital_employee_id, role_key, created_at
FROM digital_employee_roles
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
ORDER BY role_key ASC;

-- name: ListDigitalEmployeeRolesByEmployees :many
SELECT tenant_id, digital_employee_id, role_key, created_at
FROM digital_employee_roles
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = ANY(sqlc.arg('digital_employee_ids')::uuid[])
ORDER BY digital_employee_id, role_key ASC;

-- name: ListDigitalEmployeesByRoleKey :many
SELECT der.tenant_id, der.digital_employee_id, der.role_key, der.created_at
FROM digital_employee_roles der
JOIN digital_employees de ON de.id = der.digital_employee_id
WHERE der.tenant_id = sqlc.arg('tenant_id')::uuid
  AND der.role_key = sqlc.arg('role_key')::text
  AND de.deleted_at IS NULL
  AND de.status IN ('ready', 'active')
ORDER BY de.name ASC;

-- name: ReplaceDigitalEmployeeRolesDelete :exec
DELETE FROM digital_employee_roles
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid;

-- name: InsertDigitalEmployeeRole :exec
INSERT INTO digital_employee_roles (
    tenant_id,
    digital_employee_id,
    role_key
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('role_key')::text
) ON CONFLICT (digital_employee_id, role_key) DO NOTHING;

-- name: ListTenantRoleKeysHeldByEmployees :many
SELECT DISTINCT role_key
FROM digital_employee_roles
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
ORDER BY role_key ASC;
