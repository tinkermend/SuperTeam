-- name: GetActiveTenantMembership :one
SELECT *
FROM tenant_members
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id IS NULL
  AND principal_type = sqlc.arg('principal_type')::varchar
  AND principal_id = sqlc.arg('principal_id')::uuid
  AND status = 'active'
  AND disabled_at IS NULL
ORDER BY
  CASE role
    WHEN 'owner' THEN 1
    WHEN 'admin' THEN 2
    WHEN 'approver' THEN 3
    WHEN 'member' THEN 4
    WHEN 'viewer' THEN 5
    ELSE 6
  END
LIMIT 1;

-- name: GetActiveTeamMembership :one
SELECT *
FROM tenant_members
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND principal_type = sqlc.arg('principal_type')::varchar
  AND principal_id = sqlc.arg('principal_id')::uuid
  AND status = 'active'
  AND disabled_at IS NULL
ORDER BY
  CASE role
    WHEN 'owner' THEN 1
    WHEN 'admin' THEN 2
    WHEN 'approver' THEN 3
    WHEN 'member' THEN 4
    WHEN 'viewer' THEN 5
    ELSE 6
  END
LIMIT 1;

-- name: GetDigitalEmployeeAuthzScope :one
SELECT
  tenant_id,
  id AS employee_id,
  owner_user_id,
  team_id
FROM digital_employees
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('employee_id')::uuid
  AND deleted_at IS NULL;

-- name: GetProjectAuthzFacts :one
SELECT
  p.human_owner_user_ids,
  EXISTS(
    SELECT 1 FROM project_members pm
    WHERE pm.project_id = p.id AND pm.principal_id = sqlc.arg('user_id')::uuid
      AND pm.principal_type = sqlc.arg('principal_type')::varchar
      AND pm.status = 'active'
  ) AS is_member,
  p.team_id
FROM projects p
WHERE p.tenant_id = sqlc.arg('tenant_id')::uuid
  AND p.id = sqlc.arg('project_id')::uuid
  AND p.deleted_at IS NULL;
