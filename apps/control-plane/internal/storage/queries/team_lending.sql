-- 团队借调查询：供给侧策略（每团队一条 active，upsert）+ 需求侧借调请求生命周期。
-- D1 团队级 (project, team) 粒度；D2 超纲强制转人工；D3 请求自带 status。

-- name: UpsertTeamLendingPolicy :one
-- 每团队一条 active 策略：存在则更新，不存在则插入。
INSERT INTO team_lending_policy (
    tenant_id,
    team_id,
    allow_lending,
    approval_mode,
    budget_ceiling,
    capability_ceiling,
    project_match,
    status,
    created_by_user_id,
    updated_by_user_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('team_id')::uuid,
    sqlc.arg('allow_lending')::bool,
    sqlc.arg('approval_mode')::varchar,
    sqlc.narg('budget_ceiling')::numeric,
    COALESCE(sqlc.narg('capability_ceiling')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('project_match')::jsonb, '{}'::jsonb),
    'active',
    sqlc.narg('created_by_user_id')::uuid,
    sqlc.narg('updated_by_user_id')::uuid
)
ON CONFLICT (tenant_id, team_id) WHERE status = 'active'
DO UPDATE SET
    allow_lending = EXCLUDED.allow_lending,
    approval_mode = EXCLUDED.approval_mode,
    budget_ceiling = EXCLUDED.budget_ceiling,
    capability_ceiling = EXCLUDED.capability_ceiling,
    project_match = EXCLUDED.project_match,
    updated_by_user_id = EXCLUDED.updated_by_user_id,
    updated_at = NOW()
RETURNING *;

-- name: GetTeamLendingPolicy :one
SELECT *
FROM team_lending_policy
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND status = 'active';

-- name: CreateTeamLendingRequest :one
INSERT INTO team_lending_request (
    tenant_id,
    team_id,
    project_id,
    status,
    requested_by_user_id,
    request_reason,
    requested_budget,
    requested_capability,
    granted_budget,
    granted_capability,
    is_exception
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('team_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('status')::varchar,
    sqlc.arg('requested_by_user_id')::uuid,
    sqlc.arg('request_reason')::text,
    sqlc.narg('requested_budget')::numeric,
    COALESCE(sqlc.narg('requested_capability')::jsonb, '{}'::jsonb),
    sqlc.narg('granted_budget')::numeric,
    COALESCE(sqlc.narg('granted_capability')::jsonb, '{}'::jsonb),
    sqlc.arg('is_exception')::bool
)
RETURNING *;

-- name: GetTeamLendingRequest :one
SELECT *
FROM team_lending_request
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid;

-- name: ListTeamLendingRequestsByTeam :many
SELECT *
FROM team_lending_request
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: ListTeamLendingRequestsByProject :many
SELECT *
FROM team_lending_request
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: ListEffectiveLendingTeamsForProject :many
-- 协调线程挑数字员工前的借调闸门：项目当前持有有效（approved/auto_approved）借调授权的团队集合。
SELECT DISTINCT team_id
FROM team_lending_request
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND status IN ('approved', 'auto_approved');

-- name: ApproveTeamLendingRequest :one
-- 仅 pending（manual/超纲转人工）请求可被人工通过；通过时落定授予额度。
UPDATE team_lending_request SET
    status = 'approved',
    granted_budget = COALESCE(sqlc.narg('granted_budget')::numeric, requested_budget),
    granted_capability = COALESCE(sqlc.narg('granted_capability')::jsonb, requested_capability),
    decided_by_user_id = sqlc.arg('decided_by_user_id')::uuid,
    decided_at = NOW(),
    decision_reason = sqlc.arg('decision_reason')::text,
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND status = 'pending'
RETURNING *;

-- name: RejectTeamLendingRequest :one
UPDATE team_lending_request SET
    status = 'rejected',
    decided_by_user_id = sqlc.arg('decided_by_user_id')::uuid,
    decided_at = NOW(),
    decision_reason = sqlc.arg('decision_reason')::text,
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND status = 'pending'
RETURNING *;

-- name: RevokeTeamLendingRequest :one
-- 已生效（approved/auto_approved）的借调可被团队负责人撤销。
UPDATE team_lending_request SET
    status = 'revoked',
    decided_by_user_id = sqlc.arg('decided_by_user_id')::uuid,
    decided_at = NOW(),
    decision_reason = sqlc.arg('decision_reason')::text,
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND status IN ('approved', 'auto_approved')
RETURNING *;
