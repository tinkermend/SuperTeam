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
  AND (sqlc.narg('status')::varchar IS NOT NULL OR status <> 'archived')
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
  AND tenant_id = sqlc.arg('tenant_id')::uuid;

-- name: ListTenantTeamConfigDrafts :many
SELECT *
FROM tenant_team_config_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND status = 'draft'
  AND archived_at IS NULL
ORDER BY revision_number DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: UpdateTenantTeamConfigRevisionDraft :one
UPDATE tenant_team_config_revisions
SET
  constitution = COALESCE(sqlc.arg('constitution')::jsonb, constitution),
  capability_policy = COALESCE(sqlc.arg('capability_policy')::jsonb, capability_policy),
  context_policy = COALESCE(sqlc.arg('context_policy')::jsonb, context_policy),
  approval_policy = COALESCE(sqlc.arg('approval_policy')::jsonb, approval_policy),
  artifact_contract = COALESCE(sqlc.arg('artifact_contract')::jsonb, artifact_contract),
  internal_collaboration_policy = COALESCE(sqlc.arg('internal_collaboration_policy')::jsonb, internal_collaboration_policy),
  runtime_scope_policy = COALESCE(sqlc.arg('runtime_scope_policy')::jsonb, runtime_scope_policy),
  human_owner_user_id = COALESCE(sqlc.narg('human_owner_user_id')::uuid, human_owner_user_id)
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND status = 'draft'
  AND archived_at IS NULL
RETURNING *;

-- name: ArchiveActiveTenantTeamConfigRevision :many
UPDATE tenant_team_config_revisions
SET status = 'archived',
    archived_at = COALESCE(archived_at, NOW())
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND status = 'active'
  AND archived_at IS NULL
RETURNING *;

-- name: ActivateTenantTeamConfigRevision :one
UPDATE tenant_team_config_revisions
SET status = 'active',
    approved_by = sqlc.arg('approved_by')::uuid,
    approved_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND status = 'draft'
  AND archived_at IS NULL
RETURNING *;

-- name: RejectTenantTeamConfigRevision :one
UPDATE tenant_team_config_revisions
SET status = 'rejected',
    archived_at = COALESCE(archived_at, NOW())
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND status = 'draft'
  AND archived_at IS NULL
RETURNING *;

-- name: GetNextTenantTeamConfigRevisionNumber :one
SELECT (COALESCE(MAX(revision_number), 0) + 1)::integer
FROM tenant_team_config_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid;

-- name: ListTenantTeamSummaries :many
WITH current_config AS (
  SELECT DISTINCT ON (tenant_id, team_id)
    tenant_id,
    team_id,
    revision_number,
    capability_policy,
    approval_policy
  FROM tenant_team_config_revisions
  WHERE status = 'active'
    AND archived_at IS NULL
  ORDER BY tenant_id, team_id, revision_number DESC
),
draft_counts AS (
  SELECT tenant_id, team_id, COUNT(*)::integer AS pending_draft_count
  FROM tenant_team_config_revisions
  WHERE status = 'draft'
    AND archived_at IS NULL
  GROUP BY tenant_id, team_id
),
member_counts AS (
  SELECT tenant_id, team_id, COUNT(DISTINCT principal_id)::integer AS member_count
  FROM tenant_members
  WHERE team_id IS NOT NULL
    AND principal_type = 'user'
    AND status = 'active'
    AND disabled_at IS NULL
  GROUP BY tenant_id, team_id
),
employee_counts AS (
  SELECT tenant_id, team_id, COUNT(*)::integer AS digital_employee_count
  FROM digital_employees
  WHERE team_id IS NOT NULL
    AND deleted_at IS NULL
    AND archived_at IS NULL
  GROUP BY tenant_id, team_id
)
SELECT
  tt.*,
  owner.id AS owner_user_id,
  owner.username AS owner_username,
  owner.display_name AS owner_display_name,
  owner.email AS owner_email,
  owner.status AS owner_status,
  owner.avatar_provider AS owner_avatar_provider,
  owner.avatar_style AS owner_avatar_style,
  owner.avatar_seed AS owner_avatar_seed,
  owner.avatar_options AS owner_avatar_options,
  COALESCE(mc.member_count, 0)::integer AS member_count,
  COALESCE(ec.digital_employee_count, 0)::integer AS digital_employee_count,
  (
    COALESCE(jsonb_array_length(cc.capability_policy->'skill_bindings'), 0) +
    COALESCE(jsonb_array_length(cc.capability_policy->'mcp_bindings'), 0) +
    COALESCE(jsonb_array_length(cc.capability_policy->'knowledge_base_bindings'), 0) +
    COALESCE(jsonb_array_length(cc.capability_policy->'external_capability_bindings'), 0) +
    COALESCE(jsonb_array_length(cc.capability_policy->'allowed_skills'), 0) +
    COALESCE(jsonb_array_length(cc.capability_policy->'allowed_mcp_servers'), 0) +
    COALESCE(jsonb_array_length(cc.capability_policy->'allowed_plugins'), 0) +
    COALESCE(jsonb_array_length(cc.capability_policy->'allowed_provider_types'), 0)
  )::integer AS capability_count,
  cc.revision_number AS current_revision,
  COALESCE(dc.pending_draft_count, 0)::integer AS pending_draft_count,
  CASE
    WHEN cc.team_id IS NULL THEN 'not_configured'
    WHEN COALESCE(dc.pending_draft_count, 0) > 0 THEN 'draft_pending'
    ELSE 'active'
  END::varchar AS governance_status,
  COALESCE(cc.approval_policy->>'risk_summary', '')::varchar AS risk_summary
FROM tenant_teams tt
LEFT JOIN current_config cc ON cc.tenant_id = tt.tenant_id AND cc.team_id = tt.id
LEFT JOIN draft_counts dc ON dc.tenant_id = tt.tenant_id AND dc.team_id = tt.id
LEFT JOIN member_counts mc ON mc.tenant_id = tt.tenant_id AND mc.team_id = tt.id
LEFT JOIN employee_counts ec ON ec.tenant_id = tt.tenant_id AND ec.team_id = tt.id
LEFT JOIN auth_users owner ON owner.id = tt.human_owner_user_id AND owner.deleted_at IS NULL
WHERE tt.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tt.deleted_at IS NULL
  AND (sqlc.narg('status')::varchar IS NULL OR tt.status = sqlc.narg('status')::varchar)
  AND (sqlc.narg('status')::varchar IS NOT NULL OR tt.status <> 'archived')
  AND (
    sqlc.narg('governance_status')::varchar IS NULL
    OR CASE
      WHEN cc.team_id IS NULL THEN 'not_configured'
      WHEN COALESCE(dc.pending_draft_count, 0) > 0 THEN 'draft_pending'
      ELSE 'active'
    END = sqlc.narg('governance_status')::varchar
  )
  AND (
    sqlc.narg('q')::varchar IS NULL
    OR tt.name ILIKE '%' || sqlc.narg('q')::varchar || '%'
    OR tt.slug ILIKE '%' || sqlc.narg('q')::varchar || '%'
    OR owner.username ILIKE '%' || sqlc.narg('q')::varchar || '%'
    OR owner.display_name ILIKE '%' || sqlc.narg('q')::varchar || '%'
    OR owner.email ILIKE '%' || sqlc.narg('q')::varchar || '%'
  )
ORDER BY tt.updated_at DESC, tt.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: GetTenantTeamSummary :one
WITH current_config AS (
  SELECT DISTINCT ON (tenant_id, team_id)
    tenant_id,
    team_id,
    revision_number,
    capability_policy,
    approval_policy
  FROM tenant_team_config_revisions
  WHERE status = 'active'
    AND archived_at IS NULL
  ORDER BY tenant_id, team_id, revision_number DESC
),
draft_counts AS (
  SELECT tenant_id, team_id, COUNT(*)::integer AS pending_draft_count
  FROM tenant_team_config_revisions
  WHERE status = 'draft'
    AND archived_at IS NULL
  GROUP BY tenant_id, team_id
),
member_counts AS (
  SELECT tenant_id, team_id, COUNT(DISTINCT principal_id)::integer AS member_count
  FROM tenant_members
  WHERE team_id IS NOT NULL
    AND principal_type = 'user'
    AND status = 'active'
    AND disabled_at IS NULL
  GROUP BY tenant_id, team_id
),
employee_counts AS (
  SELECT tenant_id, team_id, COUNT(*)::integer AS digital_employee_count
  FROM digital_employees
  WHERE team_id IS NOT NULL
    AND deleted_at IS NULL
    AND archived_at IS NULL
  GROUP BY tenant_id, team_id
)
SELECT
  tt.*,
  owner.id AS owner_user_id,
  owner.username AS owner_username,
  owner.display_name AS owner_display_name,
  owner.email AS owner_email,
  owner.status AS owner_status,
  owner.avatar_provider AS owner_avatar_provider,
  owner.avatar_style AS owner_avatar_style,
  owner.avatar_seed AS owner_avatar_seed,
  owner.avatar_options AS owner_avatar_options,
  COALESCE(mc.member_count, 0)::integer AS member_count,
  COALESCE(ec.digital_employee_count, 0)::integer AS digital_employee_count,
  (
    COALESCE(jsonb_array_length(cc.capability_policy->'skill_bindings'), 0) +
    COALESCE(jsonb_array_length(cc.capability_policy->'mcp_bindings'), 0) +
    COALESCE(jsonb_array_length(cc.capability_policy->'knowledge_base_bindings'), 0) +
    COALESCE(jsonb_array_length(cc.capability_policy->'external_capability_bindings'), 0) +
    COALESCE(jsonb_array_length(cc.capability_policy->'allowed_skills'), 0) +
    COALESCE(jsonb_array_length(cc.capability_policy->'allowed_mcp_servers'), 0) +
    COALESCE(jsonb_array_length(cc.capability_policy->'allowed_plugins'), 0) +
    COALESCE(jsonb_array_length(cc.capability_policy->'allowed_provider_types'), 0)
  )::integer AS capability_count,
  cc.revision_number AS current_revision,
  COALESCE(dc.pending_draft_count, 0)::integer AS pending_draft_count,
  CASE
    WHEN cc.team_id IS NULL THEN 'not_configured'
    WHEN COALESCE(dc.pending_draft_count, 0) > 0 THEN 'draft_pending'
    ELSE 'active'
  END::varchar AS governance_status,
  COALESCE(cc.approval_policy->>'risk_summary', '')::varchar AS risk_summary
FROM tenant_teams tt
LEFT JOIN current_config cc ON cc.tenant_id = tt.tenant_id AND cc.team_id = tt.id
LEFT JOIN draft_counts dc ON dc.tenant_id = tt.tenant_id AND dc.team_id = tt.id
LEFT JOIN member_counts mc ON mc.tenant_id = tt.tenant_id AND mc.team_id = tt.id
LEFT JOIN employee_counts ec ON ec.tenant_id = tt.tenant_id AND ec.team_id = tt.id
LEFT JOIN auth_users owner ON owner.id = tt.human_owner_user_id AND owner.deleted_at IS NULL
WHERE tt.id = sqlc.arg('id')::uuid
  AND tt.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tt.deleted_at IS NULL;

-- name: UpdateTenantTeam :one
UPDATE tenant_teams
SET
  slug = sqlc.arg('slug')::varchar,
  name = sqlc.arg('name')::varchar,
  human_owner_user_id = sqlc.narg('human_owner_user_id')::uuid,
  metadata = COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb),
  updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
RETURNING *;

-- name: ListTeamMembers :many
SELECT
  tm.id AS membership_id,
  tm.tenant_id,
  tm.team_id,
  tm.principal_id AS user_id,
  au.username,
  au.display_name,
  au.email,
  au.status AS account_status,
  au.avatar_provider,
  au.avatar_style,
  au.avatar_seed,
  au.avatar_options,
  tm.role,
  tm.status AS membership_status,
  tm.disabled_at,
  tm.created_at,
  tm.updated_at
FROM tenant_members tm
JOIN auth_users au ON au.id = tm.principal_id
WHERE tm.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tm.team_id = sqlc.arg('team_id')::uuid
  AND tm.principal_type = 'user'
  AND tm.status = 'active'
  AND tm.disabled_at IS NULL
  AND au.deleted_at IS NULL
ORDER BY
  CASE tm.role WHEN 'owner' THEN 1 WHEN 'admin' THEN 2 WHEN 'approver' THEN 3 WHEN 'member' THEN 4 WHEN 'viewer' THEN 5 ELSE 6 END,
  au.display_name NULLS LAST,
  au.username
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: GetTeamMember :one
SELECT
  tm.id AS membership_id,
  tm.tenant_id,
  tm.team_id,
  tm.principal_id AS user_id,
  au.username,
  au.display_name,
  au.email,
  au.status AS account_status,
  au.avatar_provider,
  au.avatar_style,
  au.avatar_seed,
  au.avatar_options,
  tm.role,
  tm.status AS membership_status,
  tm.disabled_at,
  tm.created_at,
  tm.updated_at
FROM tenant_members tm
JOIN auth_users au ON au.id = tm.principal_id
WHERE tm.id = sqlc.arg('membership_id')::uuid
  AND tm.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tm.team_id = sqlc.arg('team_id')::uuid
  AND tm.principal_type = 'user'
  AND tm.status = 'active'
  AND tm.disabled_at IS NULL
  AND au.deleted_at IS NULL;

-- name: AddTeamMember :one
INSERT INTO tenant_members (
    tenant_id,
    team_id,
    principal_type,
    principal_id,
    role,
    status
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('team_id')::uuid,
    'user',
    sqlc.arg('user_id')::uuid,
    sqlc.arg('role')::varchar,
    'active'
)
ON CONFLICT (tenant_id, team_id, principal_type, principal_id, role)
DO UPDATE SET
    status = 'active',
    disabled_at = NULL,
    updated_at = NOW()
RETURNING *;

-- name: GetActiveTenantUserForTeamCreate :one
SELECT au.id, au.username, au.display_name, au.email, au.status
FROM auth_users au
JOIN tenant_members tm ON tm.principal_id = au.id
WHERE au.id = sqlc.arg('id')::uuid
  AND au.status = 'active'
  AND au.deleted_at IS NULL
  AND tm.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tm.principal_type = 'user'
  AND tm.status = 'active'
  AND tm.disabled_at IS NULL
LIMIT 1;

-- name: AddTeamOwnerMembership :one
INSERT INTO tenant_members (
    tenant_id,
    team_id,
    principal_type,
    principal_id,
    role,
    status
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('team_id')::uuid,
    'user',
    sqlc.arg('user_id')::uuid,
    'owner',
    'active'
)
ON CONFLICT (tenant_id, team_id, principal_type, principal_id, role)
DO UPDATE SET
    status = 'active',
    disabled_at = NULL,
    updated_at = NOW()
RETURNING *;

-- name: DisableTeamMemberRole :one
UPDATE tenant_members
SET
  status = 'disabled',
  disabled_at = COALESCE(disabled_at, NOW()),
  updated_at = NOW()
WHERE id = sqlc.arg('membership_id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND principal_type = 'user'
  AND disabled_at IS NULL
RETURNING *;

-- name: CountTeamOwners :one
SELECT COUNT(*)::integer
FROM tenant_members
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND principal_type = 'user'
  AND role = 'owner'
  AND status = 'active'
  AND disabled_at IS NULL;

-- name: CreateTeamMemberRoleRequest :one
INSERT INTO tenant_team_member_role_requests (
    tenant_id,
    team_id,
    target_user_id,
    requested_role,
    requested_by,
    reason
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('team_id')::uuid,
    sqlc.arg('target_user_id')::uuid,
    sqlc.arg('requested_role')::varchar,
    sqlc.arg('requested_by')::uuid,
    sqlc.arg('reason')::text
)
RETURNING *;

-- name: ListTeamMemberRoleRequests :many
SELECT *
FROM tenant_team_member_role_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: GetTeamMemberRoleRequest :one
SELECT *
FROM tenant_team_member_role_requests
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND status = 'pending';

-- name: DecideTeamMemberRoleRequest :one
UPDATE tenant_team_member_role_requests
SET
  status = sqlc.arg('status')::varchar,
  decided_by = sqlc.arg('decided_by')::uuid,
  decided_at = NOW(),
  decision_reason = sqlc.arg('decision_reason')::text,
  updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND status = 'pending'
  AND sqlc.arg('status')::varchar IN ('approved', 'rejected')
RETURNING *;

-- name: SetTenantTeamStatus :one
UPDATE tenant_teams
SET
  status = sqlc.arg('status')::varchar,
  disabled_at = CASE
    WHEN sqlc.arg('status')::varchar = 'disabled' THEN COALESCE(disabled_at, NOW())
    WHEN sqlc.arg('status')::varchar = 'active' THEN NULL
    ELSE disabled_at
  END,
  archived_at = CASE
    WHEN sqlc.arg('status')::varchar = 'archived' THEN COALESCE(archived_at, NOW())
    WHEN sqlc.arg('status')::varchar = 'active' THEN NULL
    ELSE archived_at
  END,
  updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
RETURNING *;
