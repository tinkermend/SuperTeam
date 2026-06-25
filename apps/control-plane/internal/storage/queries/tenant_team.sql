
-- name: BindDigitalEmployeeToTeam :one
UPDATE digital_employees
SET team_id = sqlc.arg('team_id')::uuid,
    updated_at = NOW()
WHERE id = sqlc.arg('employee_id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id IS NULL
  AND deleted_at IS NULL
RETURNING id;

-- name: UnbindTeamDigitalEmployees :exec
UPDATE digital_employees
SET team_id = NULL,
    updated_at = NOW()
WHERE team_id = sqlc.arg('team_id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL;

-- name: SoftDeleteTeam :one
UPDATE tenant_teams
SET deleted_at = NOW(),
    updated_at = NOW()
WHERE id = sqlc.arg('team_id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
RETURNING *;
