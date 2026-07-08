-- name: RevokeUserProjectTeamScopes :exec
UPDATE user_project_team_scopes
SET status = 'revoked',
    revoked_at = COALESCE(revoked_at, NOW()),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND user_id = sqlc.arg('user_id')::uuid
  AND status = 'active'
  AND NOT (team_id = ANY(COALESCE(sqlc.arg('team_ids')::uuid[], ARRAY[]::uuid[])));

-- name: UpsertUserProjectTeamScope :one
INSERT INTO user_project_team_scopes (
    tenant_id,
    user_id,
    team_id,
    status,
    granted_by_user_id,
    revoked_at
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('user_id')::uuid,
    sqlc.arg('team_id')::uuid,
    'active',
    sqlc.narg('granted_by_user_id')::uuid,
    NULL
)
ON CONFLICT (tenant_id, user_id, team_id) WHERE status = 'active'
DO UPDATE SET
    granted_by_user_id = EXCLUDED.granted_by_user_id,
    revoked_at = NULL,
    updated_at = NOW()
RETURNING *;

-- name: ListUserProjectTeamScopeSummaries :many
WITH employee_counts AS (
  SELECT tenant_id, team_id, COUNT(*)::integer AS digital_employee_count
  FROM digital_employees
  WHERE tenant_id = sqlc.arg('tenant_id')::uuid
    AND deleted_at IS NULL
  GROUP BY tenant_id, team_id
)
SELECT
  s.id,
  s.tenant_id,
  s.user_id,
  s.team_id,
  s.status,
  s.granted_by_user_id,
  s.revoked_at,
  s.created_at,
  s.updated_at,
  tt.slug,
  tt.name,
  tt.status AS team_status,
  COALESCE(ec.digital_employee_count, 0)::integer AS digital_employee_count,
  NULL::integer AS current_revision,
  0::integer AS pending_draft_count,
  CASE
    WHEN tt.constitution = '{}'::jsonb THEN 'not_configured'
    ELSE 'active'
  END::varchar AS governance_status,
  ''::varchar AS risk_summary,
  COALESCE(owner_agg.owners, '[]'::json)::json AS human_owners
FROM user_project_team_scopes s
JOIN tenant_teams tt ON tt.tenant_id = s.tenant_id AND tt.id = s.team_id
LEFT JOIN employee_counts ec ON ec.tenant_id = tt.tenant_id AND ec.team_id = tt.id
LEFT JOIN LATERAL (
  SELECT json_agg(json_build_object(
    'id', o.id,
    'username', o.username,
    'display_name', o.display_name,
    'email', o.email,
    'status', o.status,
    'avatar_provider', o.avatar_provider,
    'avatar_style', o.avatar_style,
    'avatar_seed', o.avatar_seed,
    'avatar_options', o.avatar_options,
    'avatar_asset_id', o.avatar_asset_id
  )) AS owners
  FROM auth_users o
  WHERE o.id = ANY(tt.human_owner_user_ids)
    AND o.deleted_at IS NULL
) owner_agg ON true
WHERE s.tenant_id = sqlc.arg('tenant_id')::uuid
  AND s.user_id = sqlc.arg('user_id')::uuid
  AND s.status = 'active'
  AND tt.deleted_at IS NULL
ORDER BY tt.name ASC, tt.slug ASC;

-- name: UserHasActiveProjectTeamScope :one
SELECT EXISTS (
  SELECT 1
  FROM user_project_team_scopes s
  JOIN tenant_teams tt ON tt.tenant_id = s.tenant_id AND tt.id = s.team_id
  WHERE s.tenant_id = sqlc.arg('tenant_id')::uuid
    AND s.user_id = sqlc.arg('user_id')::uuid
    AND s.team_id = sqlc.arg('team_id')::uuid
    AND s.status = 'active'
    AND tt.deleted_at IS NULL
    AND tt.status = 'active'
)::boolean AS allowed;
