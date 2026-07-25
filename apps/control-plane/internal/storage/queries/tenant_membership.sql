-- name: GetActiveTenantLevelMembership :one
SELECT id, tenant_id, team_id, principal_type, principal_id, role, status, disabled_at, created_at, updated_at
FROM tenant_members
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id IS NULL
  AND principal_type = 'user'
  AND principal_id = sqlc.arg('user_id')::uuid
  AND status = 'active'
  AND disabled_at IS NULL
ORDER BY
  CASE role
    WHEN 'owner' THEN 1
    WHEN 'admin' THEN 2
    WHEN 'member' THEN 3
    WHEN 'viewer' THEN 4
    ELSE 5
  END
LIMIT 1;

-- name: InsertTenantLevelMembership :one
INSERT INTO tenant_members (
    tenant_id,
    team_id,
    principal_type,
    principal_id,
    role,
    status
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    NULL,
    'user',
    sqlc.arg('user_id')::uuid,
    sqlc.arg('role')::varchar,
    'active'
)
RETURNING id, tenant_id, team_id, principal_type, principal_id, role, status, disabled_at, created_at, updated_at;

-- name: UpdateTenantLevelMembershipRole :one
UPDATE tenant_members
SET
  role = sqlc.arg('role')::varchar,
  status = 'active',
  disabled_at = NULL,
  updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id IS NULL
RETURNING id, tenant_id, team_id, principal_type, principal_id, role, status, disabled_at, created_at, updated_at;

-- name: DisableTenantLevelMembership :one
UPDATE tenant_members
SET
  status = 'disabled',
  disabled_at = COALESCE(disabled_at, NOW()),
  updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id IS NULL
  AND disabled_at IS NULL
RETURNING id, tenant_id, team_id, principal_type, principal_id, role, status, disabled_at, created_at, updated_at;

-- name: CountActiveTenantLevelOwners :one
SELECT COUNT(*)::integer AS owner_count
FROM tenant_members
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id IS NULL
  AND principal_type = 'user'
  AND role = 'owner'
  AND status = 'active'
  AND disabled_at IS NULL;

-- name: CountActiveUsersWithoutTenantMembership :one
SELECT COUNT(*)::integer AS ghost_count
FROM auth_users au
WHERE au.deleted_at IS NULL
  AND au.status = 'active'
  AND NOT EXISTS (
    SELECT 1
    FROM tenant_members tm
    WHERE tm.principal_type = 'user'
      AND tm.principal_id = au.id
      AND tm.tenant_id = sqlc.arg('tenant_id')::uuid
      AND tm.team_id IS NULL
      AND tm.status = 'active'
      AND tm.disabled_at IS NULL
  );

-- name: CountActiveTenantLevelMemberships :one
SELECT COUNT(*)::integer AS member_count
FROM tenant_members
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id IS NULL
  AND principal_type = 'user'
  AND status = 'active'
  AND disabled_at IS NULL;
