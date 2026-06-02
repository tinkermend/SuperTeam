-- name: CreateTenantTeam :one
INSERT INTO tenant_teams (tenant_id, slug, name, status, human_owner_user_id, metadata)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('slug')::varchar,
    sqlc.arg('name')::varchar,
    sqlc.arg('status')::varchar,
    sqlc.narg('human_owner_user_id')::uuid,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
)
RETURNING *;

-- name: ListTenantTeams :many
SELECT *
FROM tenant_teams
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: GetTenantTeam :one
SELECT *
FROM tenant_teams
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL;

-- name: CreateTenantTeamConfigRevision :one
INSERT INTO tenant_team_config_revisions (
    tenant_id,
    team_id,
    revision_number,
    constitution,
    capability_policy,
    context_policy,
    approval_policy,
    artifact_contract,
    internal_collaboration_policy,
    runtime_scope_policy,
    human_owner_user_id,
    status,
    approved_by,
    approved_at
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('team_id')::uuid,
    sqlc.arg('revision_number')::integer,
    COALESCE(sqlc.arg('constitution')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('capability_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('context_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('approval_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('artifact_contract')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('internal_collaboration_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('runtime_scope_policy')::jsonb, '{}'::jsonb),
    sqlc.narg('human_owner_user_id')::uuid,
    sqlc.arg('status')::varchar,
    sqlc.narg('approved_by')::uuid,
    sqlc.narg('approved_at')::timestamptz
)
RETURNING *;

-- name: GetCurrentTenantTeamConfigRevision :one
SELECT *
FROM tenant_team_config_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND status = 'active'
  AND archived_at IS NULL
ORDER BY revision_number DESC
LIMIT 1;

-- name: GetTenantTeamConfigRevision :one
SELECT *
FROM tenant_team_config_revisions
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND archived_at IS NULL;

-- name: GetNextTenantTeamConfigRevisionNumber :one
SELECT (COALESCE(MAX(revision_number), 0) + 1)::integer
FROM tenant_team_config_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid;
