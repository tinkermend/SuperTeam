-- name: CreateTenantTeam :one
INSERT INTO tenant_teams (tenant_id, slug, name, description, status, human_owner_user_ids, metadata)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('slug')::varchar,
    sqlc.arg('name')::varchar,
    sqlc.arg('description')::varchar,
    sqlc.arg('status')::varchar,
    sqlc.arg('human_owner_user_ids')::uuid[],
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

-- name: ListTenantTeamSummaries :many
WITH member_counts AS (
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
),
-- 团队能力基线计数 = 技能绑定 + MCP 绑定。两者都要排掉指向已删注册项的绑定行，
-- 口径与生效列表(ListEffectiveMCPBindingsV2ForEmployee / ListEffectiveEmployeeSkills)一致。
skill_binding_counts AS (
  SELECT tsb.tenant_id, tsb.team_id, COUNT(*)::integer AS skill_count
  FROM team_skill_bindings tsb
  JOIN skills s
    ON s.tenant_id = tsb.tenant_id
   AND s.id = tsb.skill_id
   AND s.deleted_at IS NULL
  GROUP BY tsb.tenant_id, tsb.team_id
),
mcp_binding_counts AS (
  SELECT tmb.tenant_id, tmb.team_id, COUNT(*)::integer AS mcp_count
  FROM team_mcp_bindings tmb
  JOIN mcp_servers m
    ON m.tenant_id = tmb.tenant_id
   AND m.id = tmb.mcp_server_id
   AND m.deleted_at IS NULL
  WHERE tmb.deleted_at IS NULL
  GROUP BY tmb.tenant_id, tmb.team_id
),
-- 本团队相关的待处理审批数（D6）：原先硬编码 0，头卡「待审批 N」pill 永不出现。
-- 团队维度审批（如特权角色申请）把 team_id 放在 context_payload 里，故按 jsonb 取。
pending_approval_counts AS (
  SELECT
    ar.tenant_id,
    (ar.context_payload->>'team_id')::uuid AS team_id,
    COUNT(*)::integer AS pending_count
  FROM approval_requests ar
  WHERE ar.status = 'pending'
    AND ar.context_payload->>'team_id' IS NOT NULL
  GROUP BY ar.tenant_id, (ar.context_payload->>'team_id')::uuid
)
SELECT
  tt.*,
  COALESCE(owner_agg.owners, '[]'::json) AS human_owners,
  COALESCE(mc.member_count, 0)::integer AS member_count,
  COALESCE(ec.digital_employee_count, 0)::integer AS digital_employee_count,
  (COALESCE(sbc.skill_count, 0) + COALESCE(mbc.mcp_count, 0))::integer AS capability_count,
  COALESCE(pac.pending_count, 0)::integer AS pending_draft_count,
  CASE
    WHEN tt.constitution = '{}'::jsonb THEN 'not_configured'
    ELSE 'active'
  END::varchar AS governance_status,
  ''::varchar AS risk_summary
FROM tenant_teams tt
LEFT JOIN member_counts mc ON mc.tenant_id = tt.tenant_id AND mc.team_id = tt.id
LEFT JOIN employee_counts ec ON ec.tenant_id = tt.tenant_id AND ec.team_id = tt.id
LEFT JOIN skill_binding_counts sbc ON sbc.tenant_id = tt.tenant_id AND sbc.team_id = tt.id
LEFT JOIN mcp_binding_counts mbc ON mbc.tenant_id = tt.tenant_id AND mbc.team_id = tt.id
LEFT JOIN pending_approval_counts pac ON pac.tenant_id = tt.tenant_id AND pac.team_id = tt.id
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
    'avatar_options', o.avatar_options
  )) AS owners
  FROM auth_users o
  WHERE o.id = ANY(tt.human_owner_user_ids)
    AND o.deleted_at IS NULL
) owner_agg ON true
WHERE tt.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tt.deleted_at IS NULL
  AND (sqlc.narg('status')::varchar IS NULL OR tt.status = sqlc.narg('status')::varchar)
  AND (sqlc.narg('status')::varchar IS NOT NULL OR tt.status <> 'archived')
  AND (
    sqlc.narg('governance_status')::varchar IS NULL
    OR CASE
      WHEN tt.constitution = '{}'::jsonb THEN 'not_configured'
      ELSE 'active'
    END = sqlc.narg('governance_status')::varchar
  )
  AND (
    sqlc.narg('q')::varchar IS NULL
    OR tt.name ILIKE '%' || sqlc.narg('q')::varchar || '%'
    OR tt.slug ILIKE '%' || sqlc.narg('q')::varchar || '%'
    OR EXISTS (
      SELECT 1 FROM auth_users o
      WHERE o.id = ANY(tt.human_owner_user_ids)
        AND o.deleted_at IS NULL
        AND (
          o.username ILIKE '%' || sqlc.narg('q')::varchar || '%'
          OR o.display_name ILIKE '%' || sqlc.narg('q')::varchar || '%'
          OR o.email ILIKE '%' || sqlc.narg('q')::varchar || '%'
        )
    )
  )
ORDER BY tt.updated_at DESC, tt.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: GetTenantTeamSummary :one
WITH member_counts AS (
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
),
-- 口径与 ListTenantTeamSummaries 保持一致：技能绑定 + MCP 绑定，排掉指向已删注册项的行。
skill_binding_counts AS (
  SELECT tsb.tenant_id, tsb.team_id, COUNT(*)::integer AS skill_count
  FROM team_skill_bindings tsb
  JOIN skills s
    ON s.tenant_id = tsb.tenant_id
   AND s.id = tsb.skill_id
   AND s.deleted_at IS NULL
  GROUP BY tsb.tenant_id, tsb.team_id
),
mcp_binding_counts AS (
  SELECT tmb.tenant_id, tmb.team_id, COUNT(*)::integer AS mcp_count
  FROM team_mcp_bindings tmb
  JOIN mcp_servers m
    ON m.tenant_id = tmb.tenant_id
   AND m.id = tmb.mcp_server_id
   AND m.deleted_at IS NULL
  WHERE tmb.deleted_at IS NULL
  GROUP BY tmb.tenant_id, tmb.team_id
),
-- 本团队相关的待处理审批数（D6）：原先硬编码 0，头卡「待审批 N」pill 永不出现。
-- 团队维度审批（如特权角色申请）把 team_id 放在 context_payload 里，故按 jsonb 取。
pending_approval_counts AS (
  SELECT
    ar.tenant_id,
    (ar.context_payload->>'team_id')::uuid AS team_id,
    COUNT(*)::integer AS pending_count
  FROM approval_requests ar
  WHERE ar.status = 'pending'
    AND ar.context_payload->>'team_id' IS NOT NULL
  GROUP BY ar.tenant_id, (ar.context_payload->>'team_id')::uuid
)
SELECT
  tt.*,
  COALESCE(owner_agg.owners, '[]'::json) AS human_owners,
  COALESCE(mc.member_count, 0)::integer AS member_count,
  COALESCE(ec.digital_employee_count, 0)::integer AS digital_employee_count,
  (COALESCE(sbc.skill_count, 0) + COALESCE(mbc.mcp_count, 0))::integer AS capability_count,
  COALESCE(pac.pending_count, 0)::integer AS pending_draft_count,
  CASE
    WHEN tt.constitution = '{}'::jsonb THEN 'not_configured'
    ELSE 'active'
  END::varchar AS governance_status,
  ''::varchar AS risk_summary
FROM tenant_teams tt
LEFT JOIN member_counts mc ON mc.tenant_id = tt.tenant_id AND mc.team_id = tt.id
LEFT JOIN employee_counts ec ON ec.tenant_id = tt.tenant_id AND ec.team_id = tt.id
LEFT JOIN skill_binding_counts sbc ON sbc.tenant_id = tt.tenant_id AND sbc.team_id = tt.id
LEFT JOIN mcp_binding_counts mbc ON mbc.tenant_id = tt.tenant_id AND mbc.team_id = tt.id
LEFT JOIN pending_approval_counts pac ON pac.tenant_id = tt.tenant_id AND pac.team_id = tt.id
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
    'avatar_options', o.avatar_options
  )) AS owners
  FROM auth_users o
  WHERE o.id = ANY(tt.human_owner_user_ids)
    AND o.deleted_at IS NULL
) owner_agg ON true
WHERE tt.id = sqlc.arg('id')::uuid
  AND tt.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tt.deleted_at IS NULL;

-- name: UpdateTenantTeam :one
UPDATE tenant_teams
SET
  slug = sqlc.arg('slug')::varchar,
  name = sqlc.arg('name')::varchar,
  description = sqlc.arg('description')::varchar,
  human_owner_user_ids = COALESCE(sqlc.arg('human_owner_user_ids')::uuid[], human_owner_user_ids),
  metadata = COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb),
  updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
RETURNING *;

-- name: UpdateTenantTeamConstitution :one
UPDATE tenant_teams
SET
  constitution = COALESCE(sqlc.arg('constitution')::jsonb, '{}'::jsonb),
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

-- name: CreateTeamConstitutionRevision :one
-- 宪法保存 = 追加一个新版本（版本号在同团队内递增）。回滚也是新版本，不改写历史。
INSERT INTO team_constitution_revisions (tenant_id, team_id, revision_number, rules, change_note, created_by)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('team_id')::uuid,
    COALESCE(
        (SELECT MAX(revision_number) FROM team_constitution_revisions
         WHERE tenant_id = sqlc.arg('tenant_id')::uuid AND team_id = sqlc.arg('team_id')::uuid),
        0
    ) + 1,
    sqlc.arg('rules')::jsonb,
    sqlc.arg('change_note')::text,
    sqlc.narg('created_by')::uuid
)
RETURNING *;

-- name: ListTeamConstitutionRevisions :many
SELECT r.*, COALESCE(au.display_name, au.username, '')::varchar AS created_by_name
FROM team_constitution_revisions r
LEFT JOIN auth_users au ON au.id = r.created_by
WHERE r.tenant_id = sqlc.arg('tenant_id')::uuid
  AND r.team_id = sqlc.arg('team_id')::uuid
ORDER BY r.revision_number DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: GetTeamConstitutionRevision :one
SELECT r.*, COALESCE(au.display_name, au.username, '')::varchar AS created_by_name
FROM team_constitution_revisions r
LEFT JOIN auth_users au ON au.id = r.created_by
WHERE r.tenant_id = sqlc.arg('tenant_id')::uuid
  AND r.team_id = sqlc.arg('team_id')::uuid
  AND r.revision_number = sqlc.arg('revision_number')::integer;

-- name: GetCurrentTeamConstitutionRevisionNumber :one
-- 派发注入时随执行留痕：这条任务当时受哪一版宪法约束。无版本时返回 0。
SELECT COALESCE(MAX(revision_number), 0)::integer
FROM team_constitution_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid;

-- name: GetTeamConstitutionForDispatch :one
-- 派发注入用：一次取回团队当前生效宪法与其版本号。版本号随执行留痕，
-- 使"这条任务当时受哪一版宪法约束"可回溯（spec §5.3，D9 仅文本注入）。
SELECT
    tt.constitution,
    COALESCE(
        (SELECT MAX(revision_number) FROM team_constitution_revisions r
         WHERE r.tenant_id = tt.tenant_id AND r.team_id = tt.id),
        0
    )::integer AS revision_number
FROM tenant_teams tt
WHERE tt.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tt.id = sqlc.arg('team_id')::uuid
  AND tt.deleted_at IS NULL;
