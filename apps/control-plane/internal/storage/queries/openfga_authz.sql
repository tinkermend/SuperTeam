-- name: ListOpenFGAMembers :many
SELECT
  tenant_id,
  team_id,
  principal_type,
  principal_id,
  role,
  status
FROM tenant_members
WHERE principal_type = 'user'
  AND status = 'active'
  AND disabled_at IS NULL;

-- name: ListOpenFGAProjectTeamScopes :many
SELECT
  tenant_id,
  user_id,
  team_id,
  status
FROM user_project_team_scopes
WHERE status = 'active';
