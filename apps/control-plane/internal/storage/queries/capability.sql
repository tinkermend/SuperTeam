-- name: CreateUserCredential :one
INSERT INTO user_credentials (
    tenant_id,
    user_id,
    name,
    credential_type,
    encrypted_value,
    last_four,
    metadata
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('user_id')::uuid,
    sqlc.arg('name')::text,
    sqlc.arg('credential_type')::varchar,
    sqlc.arg('encrypted_value')::text,
    sqlc.arg('last_four')::varchar,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
)
RETURNING *;

-- name: ListUserCredentials :many
SELECT *
FROM user_credentials
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND user_id = sqlc.arg('user_id')::uuid
  AND deleted_at IS NULL
  AND (
    sqlc.narg('credential_type')::varchar IS NULL
    OR credential_type = sqlc.narg('credential_type')::varchar
  )
ORDER BY created_at DESC, name ASC;

-- name: GetUserCredential :one
SELECT *
FROM user_credentials
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND user_id = sqlc.arg('user_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL;

-- name: CreateTeamMCPServer :one
INSERT INTO team_mcp_servers (
    tenant_id,
    team_id,
    name,
    url,
    credential_id,
    metadata,
    created_by
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('team_id')::uuid,
    sqlc.arg('name')::text,
    sqlc.arg('url')::text,
    sqlc.narg('credential_id')::uuid,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb),
    sqlc.narg('created_by')::uuid
)
RETURNING *;

-- name: ListTeamMCPServers :many
SELECT
    tm.id,
    tm.tenant_id,
    tm.team_id,
    tm.name,
    tm.url,
    tm.credential_id,
    tm.status,
    tm.metadata,
    tm.disabled_at,
    tm.deleted_at,
    tm.created_by,
    tm.created_at,
    tm.updated_at,
    COALESCE(uc.name, '') AS credential_name,
    COALESCE(uc.credential_type, '') AS credential_type,
    COALESCE(uc.last_four, '') AS credential_last_four
FROM team_mcp_servers tm
LEFT JOIN user_credentials uc ON uc.tenant_id = tm.tenant_id
    AND uc.id = tm.credential_id
    AND uc.deleted_at IS NULL
WHERE tm.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tm.team_id = sqlc.arg('team_id')::uuid
  AND tm.deleted_at IS NULL
ORDER BY tm.created_at DESC, tm.name ASC;

-- name: DeleteTeamMCPServer :exec
UPDATE team_mcp_servers
SET deleted_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL;

-- name: CreateDigitalEmployeeMCPBinding :one
INSERT INTO digital_employee_mcp_bindings (
    tenant_id,
    digital_employee_id,
    name,
    url,
    credential_id,
    metadata,
    created_by
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('name')::text,
    sqlc.arg('url')::text,
    sqlc.narg('credential_id')::uuid,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb),
    sqlc.narg('created_by')::uuid
)
RETURNING *;

-- name: ListDigitalEmployeeMCPBindings :many
SELECT
    em.id,
    em.tenant_id,
    em.digital_employee_id,
    em.name,
    em.url,
    em.credential_id,
    em.status,
    em.metadata,
    em.disabled_at,
    em.deleted_at,
    em.created_by,
    em.created_at,
    em.updated_at,
    COALESCE(uc.name, '') AS credential_name,
    COALESCE(uc.credential_type, '') AS credential_type,
    COALESCE(uc.last_four, '') AS credential_last_four
FROM digital_employee_mcp_bindings em
LEFT JOIN user_credentials uc ON uc.tenant_id = em.tenant_id
    AND uc.id = em.credential_id
    AND uc.deleted_at IS NULL
WHERE em.tenant_id = sqlc.arg('tenant_id')::uuid
  AND em.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND em.deleted_at IS NULL
ORDER BY em.created_at DESC, em.name ASC;

-- name: DeleteDigitalEmployeeMCPBinding :exec
UPDATE digital_employee_mcp_bindings
SET deleted_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL;

-- name: ListEffectiveMCPServersForEmployee :many
WITH target_employee AS (
    SELECT tenant_id, id AS digital_employee_id, team_id
    FROM digital_employees
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND id = sqlc.arg('digital_employee_id')::uuid
      AND deleted_at IS NULL
)
SELECT
    tm.id,
    tm.tenant_id,
    target_employee.digital_employee_id,
    target_employee.team_id,
    tm.name,
    tm.url,
    tm.credential_id,
    tm.status,
    'team'::text AS source_scope,
    true AS inherited,
    COALESCE(uc.name, '') AS credential_name,
    COALESCE(uc.credential_type, '') AS credential_type,
    COALESCE(uc.last_four, '') AS credential_last_four,
    tm.created_at,
    tm.updated_at
FROM target_employee
JOIN team_mcp_servers tm ON tm.tenant_id = target_employee.tenant_id
    AND tm.team_id = target_employee.team_id
    AND tm.deleted_at IS NULL
LEFT JOIN user_credentials uc ON uc.tenant_id = tm.tenant_id
    AND uc.id = tm.credential_id
    AND uc.deleted_at IS NULL
UNION ALL
SELECT
    em.id,
    em.tenant_id,
    em.digital_employee_id,
    NULL::uuid AS team_id,
    em.name,
    em.url,
    em.credential_id,
    em.status,
    'employee'::text AS source_scope,
    false AS inherited,
    COALESCE(uc.name, '') AS credential_name,
    COALESCE(uc.credential_type, '') AS credential_type,
    COALESCE(uc.last_four, '') AS credential_last_four,
    em.created_at,
    em.updated_at
FROM target_employee
JOIN digital_employee_mcp_bindings em ON em.tenant_id = target_employee.tenant_id
    AND em.digital_employee_id = target_employee.digital_employee_id
    AND em.deleted_at IS NULL
LEFT JOIN user_credentials uc ON uc.tenant_id = em.tenant_id
    AND uc.id = em.credential_id
    AND uc.deleted_at IS NULL
WHERE NOT EXISTS (
    SELECT 1
    FROM team_mcp_servers team_duplicate
    WHERE team_duplicate.tenant_id = target_employee.tenant_id
      AND team_duplicate.team_id = target_employee.team_id
      AND team_duplicate.name = em.name
      AND team_duplicate.url = em.url
      AND team_duplicate.deleted_at IS NULL
)
ORDER BY inherited DESC, name ASC;
