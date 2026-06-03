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
WHERE tt.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tt.deleted_at IS NULL
  AND (sqlc.narg('status')::varchar IS NULL OR tt.status = sqlc.narg('status')::varchar)
  AND (sqlc.narg('status')::varchar IS NOT NULL OR tt.status <> 'archived')
  AND (
    sqlc.narg('q')::varchar IS NULL
    OR tt.name ILIKE '%' || sqlc.narg('q')::varchar || '%'
    OR tt.slug ILIKE '%' || sqlc.narg('q')::varchar || '%'
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
