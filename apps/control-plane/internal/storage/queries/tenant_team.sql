
-- name: BindDigitalEmployeeToTeam :one
UPDATE digital_employees
SET team_id = sqlc.arg('team_id')::uuid,
    updated_at = NOW()
WHERE id = sqlc.arg('employee_id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id IS NULL
  AND deleted_at IS NULL
RETURNING id;

-- name: ReassignDigitalEmployeeTeam :one
UPDATE digital_employees
SET team_id = sqlc.arg('team_id')::uuid,
    updated_at = NOW()
WHERE id = sqlc.arg('employee_id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
RETURNING id, team_id;

-- name: UnbindTeamDigitalEmployees :exec
UPDATE digital_employees
SET team_id = NULL,
    updated_at = NOW()
WHERE team_id = sqlc.arg('team_id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL;

-- name: DeleteTeamSkillBindings :exec
DELETE FROM team_skill_bindings
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid;

-- name: SoftDeleteTeamMCPBindings :exec
UPDATE team_mcp_bindings
SET deleted_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND deleted_at IS NULL;

-- name: SoftDeleteTeam :one
-- 删除进入待确认态:全站不可见(deleted_at),管理员恢复或确认后才物理删除。
UPDATE tenant_teams
SET status = 'pending_delete',
    deleted_at = NOW(),
    delete_requested_by = sqlc.arg('delete_requested_by')::uuid,
    updated_at = NOW()
WHERE id = sqlc.arg('team_id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
RETURNING *;

-- name: RestorePendingDeleteTeam :one
UPDATE tenant_teams
SET status = 'active',
    deleted_at = NULL,
    delete_requested_by = NULL,
    updated_at = NOW()
WHERE id = sqlc.arg('team_id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND status = 'pending_delete'
RETURNING *;

-- name: ListPendingDeleteTeams :many
SELECT * FROM tenant_teams
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND status = 'pending_delete'
ORDER BY deleted_at ASC;

-- name: ListStalePendingDeleteTeams :many
-- 滞留催办扫描(跨租户):待确认超过阈值仍无人处理的团队。
SELECT * FROM tenant_teams
WHERE status = 'pending_delete'
  AND deleted_at < sqlc.arg('stale_before')::timestamptz
ORDER BY deleted_at ASC
LIMIT 100;

-- name: HardDeleteTeam :one
-- 仅允许物理删除待确认态团队;P1 前的遗留软删终态(status=active)不可经此路径删除。
DELETE FROM tenant_teams
WHERE id = sqlc.arg('team_id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND status = 'pending_delete'
RETURNING *;

-- name: HardDeleteTeamMembers :exec
DELETE FROM tenant_members
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid;

-- name: HardDeleteTeamMemberRoleRequests :exec
DELETE FROM tenant_team_member_role_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid;

-- name: HardDeleteTeamMCPBindings :exec
DELETE FROM team_mcp_bindings
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid;

-- name: HardDeleteTeamMCPServers :exec
DELETE FROM team_mcp_servers
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid;

-- name: HardDeleteTeamLendingPolicies :exec
DELETE FROM team_lending_policy
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid;

-- name: HardDeleteTeamLendingRequests :exec
DELETE FROM team_lending_request
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid;

-- name: HardDeleteTeamRuntimeNodeScopes :exec
DELETE FROM runtime_node_scopes
WHERE team_id = sqlc.arg('team_id')::uuid;

-- name: HardDeleteTeamUserProjectTeamScopes :exec
DELETE FROM user_project_team_scopes
WHERE team_id = sqlc.arg('team_id')::uuid;

-- name: ClearProjectsTeamRef :exec
UPDATE projects
SET team_id = NULL, updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid;

-- name: ClearDigitalEmployeesTeamRef :exec
-- 兜底:软删时 UnbindTeamDigitalEmployees 已清存活员工;这里连已删员工的历史引用一并清。
UPDATE digital_employees
SET team_id = NULL, updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid;
