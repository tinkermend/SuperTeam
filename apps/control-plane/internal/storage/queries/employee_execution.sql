-- name: CreateDigitalEmployee :one
INSERT INTO digital_employees (
    tenant_id,
    team_id,
    owner_user_id,
    employee_type,
    provider_type,
    name,
    role,
    description,
    status,
    permission_policy,
    risk_level,
    metadata
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.narg('team_id')::uuid,
    sqlc.arg('owner_user_id')::uuid,
    sqlc.arg('employee_type')::varchar,
    sqlc.arg('provider_type')::varchar,
    sqlc.arg('name')::varchar,
    sqlc.arg('role')::varchar,
    sqlc.narg('description')::text,
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.arg('permission_policy')::jsonb, '{}'::jsonb),
    sqlc.arg('risk_level')::varchar,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
) RETURNING *;

-- name: GetDigitalEmployee :one
SELECT *
FROM digital_employees
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL;

-- name: GetDigitalEmployeeForDelete :one
SELECT *
FROM digital_employees
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
FOR UPDATE;

-- name: ListDigitalEmployees :many
SELECT *
FROM digital_employees
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
  AND (sqlc.narg('team_id')::uuid IS NULL OR team_id = sqlc.narg('team_id')::uuid)
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)
  AND (sqlc.narg('assignment')::varchar IS NULL OR (sqlc.narg('assignment')::varchar = 'unassigned' AND team_id IS NULL))
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: ListDigitalEmployeeTeamAssignments :many
SELECT id, team_id
FROM digital_employees
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = ANY(sqlc.arg('employee_ids')::uuid[])
  AND deleted_at IS NULL;

-- name: GetDigitalEmployeeSchedulingSkillCounts :one
WITH target_employee AS (
    SELECT tenant_id, id AS digital_employee_id, team_id
    FROM digital_employees
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND id = sqlc.arg('digital_employee_id')::uuid
      AND deleted_at IS NULL
),
personal_skills AS (
    SELECT sab.skill_id
    FROM target_employee te
    JOIN skill_agent_bindings sab
      ON sab.tenant_id = te.tenant_id
     AND sab.digital_employee_id = te.digital_employee_id
     AND sab.status = 'enabled'
    JOIN skills s
      ON s.tenant_id = sab.tenant_id
     AND s.id = sab.skill_id
     AND s.deleted_at IS NULL
    WHERE NOT EXISTS (
        SELECT 1
        FROM team_skill_bindings inherited_binding
        WHERE inherited_binding.tenant_id = te.tenant_id
          AND inherited_binding.team_id = te.team_id
          AND inherited_binding.skill_id = sab.skill_id
    )
),
inherited_skills AS (
    SELECT stb.skill_id
    FROM target_employee te
    JOIN team_skill_bindings stb
      ON stb.tenant_id = te.tenant_id
     AND stb.team_id = te.team_id
    JOIN skills s
      ON s.tenant_id = stb.tenant_id
     AND s.id = stb.skill_id
     AND s.deleted_at IS NULL
)
SELECT
    COALESCE((SELECT COUNT(*) FROM personal_skills), 0)::int AS personal_skill_count,
    COALESCE((SELECT COUNT(*) FROM inherited_skills), 0)::int AS inherited_skill_count
FROM target_employee;

-- name: UpdateDigitalEmployeeStatus :one
UPDATE digital_employees
SET status = sqlc.arg('status')::varchar,
    disabled_at = CASE
        WHEN sqlc.arg('status')::varchar = 'disabled' THEN COALESCE(disabled_at, NOW())
        WHEN sqlc.arg('status')::varchar IN ('ready', 'active') THEN NULL
        ELSE disabled_at
    END,
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
RETURNING *;

-- name: UpdateDigitalEmployeeRolePermission :one
-- 权限中心批准员工治理变更(role/permission_policy)后,由 ActivateConfigRevision 写回员工行。
-- 值由审批请求的 ContextPayload 承载(方案2:权限变更不进 config_revision),此查询只落库。
UPDATE digital_employees
SET role = sqlc.arg('role')::varchar,
    permission_policy = COALESCE(sqlc.arg('permission_policy')::jsonb, '{}'::jsonb),
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
RETURNING *;

-- name: ListDigitalEmployeeDeleteRunBlockers :many
SELECT
    'run'::text AS blocker_type,
    tr.id,
    tr.status,
    COALESCE(t.title, tr.task_id::text) AS title,
    tr.id AS run_id,
    pt.project_id
FROM task_runs tr
LEFT JOIN tasks t
  ON t.tenant_id = tr.tenant_id
 AND t.id = tr.task_id
LEFT JOIN project_tasks pt
  ON pt.tenant_id = tr.tenant_id
 AND pt.digital_employee_run_id = tr.id
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND tr.status IN ('queued', 'dispatching', 'running', 'cancelling')
ORDER BY tr.updated_at DESC, tr.created_at DESC
LIMIT 20;

-- name: ListDigitalEmployeeDeleteProjectTaskBlockers :many
SELECT
    'project_task'::text AS blocker_type,
    pt.id,
    pt.status,
    (COALESCE(NULLIF(pt.title, ''), pt.id::text))::text AS title,
    NULL::uuid AS run_id,
    pt.project_id
FROM project_tasks pt
WHERE pt.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pt.assigned_digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND pt.status IN ('queued', 'running', 'in_progress')
ORDER BY pt.updated_at DESC, pt.created_at DESC
LIMIT 20;

-- name: SoftDeleteDigitalEmployeeForDelete :one
UPDATE digital_employees
SET status = 'disabled',
    disabled_at = COALESCE(disabled_at, sqlc.arg('deleted_at')::timestamptz),
    deleted_at = COALESCE(deleted_at, sqlc.arg('deleted_at')::timestamptz),
    updated_at = sqlc.arg('deleted_at')::timestamptz
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteDigitalEmployeeEnvironmentVariablesForDelete :many
UPDATE digital_employee_environment_variables
SET status = 'disabled',
    deleted_at = COALESCE(deleted_at, sqlc.arg('deleted_at')::timestamptz),
    updated_at = sqlc.arg('deleted_at')::timestamptz
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND deleted_at IS NULL
RETURNING id;

-- name: SoftDeleteDigitalEmployeeMCPBindingsV2ForDelete :many
UPDATE digital_employee_mcp_bindings_v2
SET deleted_at = COALESCE(deleted_at, sqlc.arg('deleted_at')::timestamptz),
    updated_at = sqlc.arg('deleted_at')::timestamptz
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND deleted_at IS NULL
RETURNING id;

-- name: DisableSkillAgentBindingsForDelete :many
UPDATE skill_agent_bindings
SET status = 'disabled',
    updated_at = sqlc.arg('deleted_at')::timestamptz
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND status = 'enabled'
RETURNING id;

-- name: ArchiveDigitalEmployeeConfigRevisionsForDelete :many
UPDATE digital_employee_config_revisions
SET status = 'archived',
    archived_at = COALESCE(archived_at, sqlc.arg('deleted_at')::timestamptz),
    updated_at = sqlc.arg('deleted_at')::timestamptz
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND archived_at IS NULL
RETURNING id;

-- name: DeleteProjectEmployeeNodeAffinitiesForEmployeeDelete :many
DELETE FROM project_employee_node_affinity
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
RETURNING id;

-- name: DeleteDigitalEmployee :exec
UPDATE digital_employees
SET deleted_at = COALESCE(deleted_at, NOW()),
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid;

-- name: ListRuntimeProviderOptionsForDigitalEmployeeCreate :many
WITH active_team_config AS (
    SELECT
        tt.id,
        tt.tenant_id,
        tt.constitution,
        '{}'::jsonb AS runtime_scope_policy
    FROM tenant_teams tt
    WHERE tt.tenant_id = sqlc.arg('tenant_id')::uuid
      AND tt.id = sqlc.arg('team_id')::uuid
      AND tt.deleted_at IS NULL
      AND tt.status <> 'archived'
    LIMIT 1
),
runtime_sessions_active AS (
    SELECT DISTINCT re.runtime_node_id
    FROM runtime_sessions rs
    JOIN runtime_enrollments re
      ON re.id = rs.enrollment_id
     AND re.tenant_id = rs.tenant_id
     AND re.runtime_node_id = rs.runtime_node_id
     AND re.status = 'approved'
     AND re.rejected_at IS NULL
     AND re.revoked_at IS NULL
    WHERE rs.tenant_id = sqlc.arg('tenant_id')::uuid
      AND rs.expires_at > NOW()
      AND rs.revoked_at IS NULL
),
provider_capabilities AS (
    SELECT DISTINCT ON (tenant_id, runtime_node_id, provider_type)
        *
    FROM runtime_capabilities
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND capability_type = 'provider'
      AND archived_at IS NULL
    ORDER BY tenant_id, runtime_node_id, provider_type, last_seen_at DESC NULLS LAST, updated_at DESC
)
SELECT
    rn.id AS runtime_node_id,
    rn.node_id,
    rn.name AS runtime_name,
    pc.provider_type,
    rn.status AS runtime_status,
    pc.status AS provider_status,
    pc.health_status,
    rn.current_load,
    rn.max_slots,
    COALESCE(
        pc.details ->> 'agent_home_dir',
        pc.metadata ->> 'agent_home_dir',
        pc.workspace_base_dir,
        rn.metadata ->> 'agent_home_dir',
        ''
    )::text AS agent_home_dir,
	    (
	        active_team_config.id IS NOT NULL
	        AND rn.status = 'online'
	        AND rn.disabled_at IS NULL
	        AND rn.archived_at IS NULL
	        AND pc.available = true
	        AND pc.status = 'healthy'
	        AND pc.health_status = 'healthy'
	        AND runtime_sessions_active.runtime_node_id IS NOT NULL
	        AND CASE
	            WHEN NOT (active_team_config.runtime_scope_policy ? 'allowed_runtime_node_ids') THEN true
	            WHEN jsonb_typeof(active_team_config.runtime_scope_policy -> 'allowed_runtime_node_ids') = 'array' THEN
	                (active_team_config.runtime_scope_policy -> 'allowed_runtime_node_ids') ? rn.id::text
            ELSE false
        END
        AND CASE
            WHEN NOT (active_team_config.runtime_scope_policy ? 'allowed_node_ids') THEN true
            WHEN jsonb_typeof(active_team_config.runtime_scope_policy -> 'allowed_node_ids') = 'array' THEN
                (active_team_config.runtime_scope_policy -> 'allowed_node_ids') ? rn.node_id
            ELSE false
	        END
	    )::boolean AS available,
	    CASE
	        WHEN active_team_config.id IS NULL THEN 'team_required'
	        WHEN rn.status <> 'online' OR rn.disabled_at IS NOT NULL OR rn.archived_at IS NOT NULL THEN 'runtime_not_online'
	        WHEN runtime_sessions_active.runtime_node_id IS NULL THEN 'runtime_session_inactive'
	        WHEN pc.available = false OR pc.status <> 'healthy' OR pc.health_status <> 'healthy' THEN 'provider_unhealthy'
	        WHEN COALESCE(pc.provider_type, '') = '' THEN 'provider_type_missing'
	        WHEN active_team_config.runtime_scope_policy ? 'allowed_runtime_node_ids'
	            AND (
	                jsonb_typeof(active_team_config.runtime_scope_policy -> 'allowed_runtime_node_ids') <> 'array'
	                OR NOT ((active_team_config.runtime_scope_policy -> 'allowed_runtime_node_ids') ? rn.id::text)
            ) THEN 'runtime_node_outside_team_policy'
        WHEN active_team_config.runtime_scope_policy ? 'allowed_node_ids'
            AND (
                jsonb_typeof(active_team_config.runtime_scope_policy -> 'allowed_node_ids') <> 'array'
                OR NOT ((active_team_config.runtime_scope_policy -> 'allowed_node_ids') ? rn.node_id)
            ) THEN 'runtime_node_slug_outside_team_policy'
        ELSE ''
    END::varchar AS disabled_reason
FROM runtime_nodes rn
LEFT JOIN provider_capabilities pc
  ON pc.runtime_node_id = rn.id
 AND pc.tenant_id = rn.tenant_id
LEFT JOIN active_team_config ON TRUE
LEFT JOIN runtime_sessions_active ON runtime_sessions_active.runtime_node_id = rn.id
WHERE rn.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pc.provider_type IS NOT NULL
ORDER BY available DESC, rn.name ASC, pc.provider_type ASC;

-- name: ListRuntimeProviderOptionsForTeamLessCreate :many
-- Team-less variant: no team governance, all providers/runtime nodes allowed.
WITH runtime_sessions_active AS (
    SELECT DISTINCT re.runtime_node_id
    FROM runtime_sessions rs
    JOIN runtime_enrollments re
      ON re.id = rs.enrollment_id
     AND re.tenant_id = rs.tenant_id
     AND re.runtime_node_id = rs.runtime_node_id
     AND re.status = 'approved'
     AND re.rejected_at IS NULL
     AND re.revoked_at IS NULL
    WHERE rs.tenant_id = sqlc.arg('tenant_id')::uuid
      AND rs.expires_at > NOW()
      AND rs.revoked_at IS NULL
),
provider_capabilities AS (
    SELECT DISTINCT ON (tenant_id, runtime_node_id, provider_type)
        *
    FROM runtime_capabilities
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND capability_type = 'provider'
      AND archived_at IS NULL
    ORDER BY tenant_id, runtime_node_id, provider_type, last_seen_at DESC NULLS LAST, updated_at DESC
)
SELECT
    rn.id AS runtime_node_id,
    rn.node_id,
    rn.name AS runtime_name,
    pc.provider_type,
    rn.status AS runtime_status,
    pc.status AS provider_status,
    pc.health_status,
    rn.current_load,
    rn.max_slots,
    COALESCE(
        pc.details ->> 'agent_home_dir',
        pc.metadata ->> 'agent_home_dir',
        pc.workspace_base_dir,
        rn.metadata ->> 'agent_home_dir',
        ''
    )::text AS agent_home_dir,
    (
        rn.status = 'online'
        AND rn.disabled_at IS NULL
        AND rn.archived_at IS NULL
        AND pc.available = true
        AND pc.status = 'healthy'
        AND pc.health_status = 'healthy'
        AND runtime_sessions_active.runtime_node_id IS NOT NULL
    )::boolean AS available,
    CASE
        WHEN rn.status <> 'online' OR rn.disabled_at IS NOT NULL OR rn.archived_at IS NOT NULL THEN 'runtime_not_online'
        WHEN runtime_sessions_active.runtime_node_id IS NULL THEN 'runtime_session_inactive'
        WHEN pc.available = false OR pc.status <> 'healthy' OR pc.health_status <> 'healthy' THEN 'provider_unhealthy'
        WHEN COALESCE(pc.provider_type, '') = '' THEN 'provider_type_missing'
        ELSE ''
    END::varchar AS disabled_reason
FROM runtime_nodes rn
LEFT JOIN provider_capabilities pc
  ON pc.runtime_node_id = rn.id
 AND pc.tenant_id = rn.tenant_id
LEFT JOIN runtime_sessions_active ON runtime_sessions_active.runtime_node_id = rn.id
WHERE rn.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pc.provider_type IS NOT NULL
ORDER BY available DESC, rn.name ASC, pc.provider_type ASC;

-- name: GetDigitalEmployeeRunPreflight :one
-- Standalone/workbench run preflight: least-loaded online tenant node with healthy provider.
SELECT
    de.tenant_id,
    de.team_id,
    de.id AS digital_employee_id,
    de.status AS digital_employee_status,
    rn.id AS runtime_node_id,
    rn.node_id,
    de.provider_type,
    COALESCE(
        provider_capability.details ->> 'agent_home_dir',
        provider_capability.metadata ->> 'agent_home_dir',
        provider_capability.workspace_base_dir,
        workspace_capability.details ->> 'agent_home_dir',
        workspace_capability.metadata ->> 'agent_home_dir',
        workspace_capability.workspace_base_dir,
        rn.metadata ->> 'agent_home_dir',
        provider_capability.details ->> 'workspace_base_dir',
        provider_capability.metadata ->> 'workspace_base_dir',
        workspace_capability.details ->> 'workspace_base_dir',
        workspace_capability.metadata ->> 'workspace_base_dir',
        ''
    )::text AS agent_home_dir,
    jsonb_build_object('source', 'tenant_placement', 'node_id', rn.node_id) AS runtime_selector,
    '{}'::jsonb AS session_policy,
    jsonb_build_object(
        'workspace_base_dir',
        COALESCE(
            provider_capability.workspace_base_dir,
            workspace_capability.workspace_base_dir,
            rn.metadata ->> 'workspace_base_dir',
            ''
        )
    ) AS workspace_policy,
    COALESCE(config_state.budget_policy, '{}'::jsonb)::jsonb AS budget_policy,
    COALESCE(today_usage.usage_tokens_today, 0)::integer AS today_token_usage,
    'Asia/Shanghai'::text AS business_timezone,
    (provider_capability.id IS NOT NULL)::boolean AS provider_healthy
FROM digital_employees de
JOIN LATERAL (
    SELECT rn2.*
    FROM runtime_nodes rn2
    JOIN runtime_capabilities rc
      ON rc.tenant_id = rn2.tenant_id
     AND rc.runtime_node_id = rn2.id
     AND rc.capability_type = 'provider'
     AND rc.provider_type = de.provider_type
     AND rc.available = true
     AND rc.status = 'healthy'
     AND rc.health_status = 'healthy'
     AND rc.archived_at IS NULL
    WHERE rn2.tenant_id = de.tenant_id
      AND rn2.status = 'online'
      AND rn2.disabled_at IS NULL
      AND rn2.archived_at IS NULL
    ORDER BY rn2.current_load ASC, rn2.id ASC
    LIMIT 1
) rn ON TRUE
LEFT JOIN LATERAL (
    SELECT rc.*
    FROM runtime_capabilities rc
    WHERE rc.tenant_id = de.tenant_id
      AND rc.runtime_node_id = rn.id
      AND rc.capability_type = 'provider'
      AND rc.provider_type = de.provider_type
      AND rc.available = true
      AND rc.status = 'healthy'
      AND rc.health_status = 'healthy'
      AND rc.archived_at IS NULL
    ORDER BY rc.last_seen_at DESC NULLS LAST, rc.updated_at DESC
    LIMIT 1
) provider_capability ON TRUE
LEFT JOIN LATERAL (
    SELECT rc.*
    FROM runtime_capabilities rc
    WHERE rc.tenant_id = de.tenant_id
      AND rc.runtime_node_id = rn.id
      AND rc.capability_type = 'workspace'
      AND rc.available = true
      AND rc.archived_at IS NULL
    ORDER BY
      CASE WHEN rc.capability_key = 'base-dir' THEN 0 ELSE 1 END,
      rc.last_seen_at DESC NULLS LAST,
      rc.updated_at DESC
    LIMIT 1
) workspace_capability ON TRUE
LEFT JOIN LATERAL (
    SELECT
        decr.budget_policy
    FROM digital_employee_config_revisions decr
    WHERE decr.tenant_id = de.tenant_id
      AND decr.digital_employee_id = de.id
      AND decr.status = 'active'
      AND decr.archived_at IS NULL
    ORDER BY decr.revision_number DESC, decr.updated_at DESC
    LIMIT 1
) config_state ON true
LEFT JOIN LATERAL (
    SELECT
        LEAST(
            COALESCE(
                SUM(
                    CASE
                        WHEN COALESCE(tr.result #>> '{usage,total_tokens}', tr.result ->> 'total_tokens', '') ~ '^[0-9]+$'
                        THEN COALESCE(tr.result #>> '{usage,total_tokens}', tr.result ->> 'total_tokens', '')::bigint
                        ELSE 0
                    END
                ),
                0
            ),
            2147483647
        )::integer AS usage_tokens_today
    FROM task_runs tr
    WHERE tr.tenant_id = de.tenant_id
      AND tr.digital_employee_id = de.id
      AND COALESCE(tr.finished_at, tr.updated_at, tr.created_at) >= (date_trunc('day', timezone('Asia/Shanghai', now())) AT TIME ZONE 'Asia/Shanghai')
      AND COALESCE(tr.finished_at, tr.updated_at, tr.created_at) < ((date_trunc('day', timezone('Asia/Shanghai', now())) + INTERVAL '1 day') AT TIME ZONE 'Asia/Shanghai')
) today_usage ON true
WHERE de.id = sqlc.arg('digital_employee_id')::uuid
  AND de.tenant_id = sqlc.arg('tenant_id')::uuid
  AND de.deleted_at IS NULL
  AND de.archived_at IS NULL;

-- name: GetProjectTaskRunPreflight :one
-- Discovery-only preflight: used by planning-profile facts and the pre-dispatch
-- gate's runtime snapshot to read health signals for an employee, before any
-- task-specific node has been resolved. It does NOT pin a dispatch to a node —
-- for that, see GetProjectTaskRunPreflightForNode. The node reported here is a
-- deterministic representative of the project's eligibility set (project_runtime_nodes):
-- the least-loaded online node, so an idle/degraded node doesn't look "ready" just
-- because it happens to sort first.
SELECT
    de.tenant_id,
    de.team_id,
    de.id AS digital_employee_id,
    de.status AS digital_employee_status,
    rn.id AS runtime_node_id,
    rn.node_id,
    de.provider_type,
    COALESCE(
        provider_capability.details ->> 'agent_home_dir',
        provider_capability.metadata ->> 'agent_home_dir',
        provider_capability.workspace_base_dir,
        workspace_capability.details ->> 'agent_home_dir',
        workspace_capability.metadata ->> 'agent_home_dir',
        workspace_capability.workspace_base_dir,
        rn.metadata ->> 'agent_home_dir',
        provider_capability.details ->> 'workspace_base_dir',
        provider_capability.metadata ->> 'workspace_base_dir',
        workspace_capability.details ->> 'workspace_base_dir',
        workspace_capability.metadata ->> 'workspace_base_dir',
        ''
    )::text AS workspace_base_dir,
    COALESCE(config_state.budget_policy, '{}'::jsonb)::jsonb AS budget_policy,
    COALESCE(today_usage.usage_tokens_today, 0)::integer AS today_token_usage,
    'Asia/Shanghai'::text AS business_timezone,
    (runtime_session.id IS NOT NULL)::boolean AS runtime_session_active,
    (provider_capability.id IS NOT NULL)::boolean AS provider_healthy
FROM digital_employees de
JOIN LATERAL (
    SELECT rn2.*
    FROM project_runtime_nodes prn
    JOIN runtime_nodes rn2
      ON rn2.id = prn.runtime_node_id
     AND rn2.tenant_id = de.tenant_id
     AND rn2.status = 'online'
     AND rn2.disabled_at IS NULL
     AND rn2.archived_at IS NULL
    WHERE prn.tenant_id = de.tenant_id
      AND prn.project_id = sqlc.arg('project_id')::uuid
    ORDER BY rn2.current_load ASC, rn2.id ASC
    LIMIT 1
) rn ON TRUE
LEFT JOIN LATERAL (
    SELECT rc.*
    FROM runtime_capabilities rc
    WHERE rc.tenant_id = de.tenant_id
      AND rc.runtime_node_id = rn.id
      AND rc.capability_type = 'provider'
      AND rc.provider_type = de.provider_type
      AND rc.available = true
      AND rc.status = 'healthy'
      AND rc.health_status = 'healthy'
      AND rc.archived_at IS NULL
    ORDER BY rc.last_seen_at DESC NULLS LAST, rc.updated_at DESC
    LIMIT 1
) provider_capability ON TRUE
LEFT JOIN LATERAL (
    SELECT rc.*
    FROM runtime_capabilities rc
    WHERE rc.tenant_id = de.tenant_id
      AND rc.runtime_node_id = rn.id
      AND rc.capability_type = 'workspace'
      AND rc.available = true
      AND rc.archived_at IS NULL
    ORDER BY
      CASE WHEN rc.capability_key = 'base-dir' THEN 0 ELSE 1 END,
      rc.last_seen_at DESC NULLS LAST,
      rc.updated_at DESC
    LIMIT 1
) workspace_capability ON TRUE
LEFT JOIN LATERAL (
    SELECT rs.id
    FROM runtime_sessions rs
    JOIN runtime_enrollments re
      ON re.id = rs.enrollment_id
     AND re.tenant_id = rs.tenant_id
     AND re.runtime_node_id = rs.runtime_node_id
     AND re.status = 'approved'
     AND re.rejected_at IS NULL
     AND re.revoked_at IS NULL
    WHERE rs.tenant_id = de.tenant_id
      AND rs.runtime_node_id = rn.id
      AND rs.expires_at > NOW()
      AND rs.revoked_at IS NULL
    ORDER BY rs.last_seen_at DESC, rs.updated_at DESC
    LIMIT 1
) runtime_session ON TRUE
LEFT JOIN LATERAL (
    SELECT
        decr.budget_policy
    FROM digital_employee_config_revisions decr
    WHERE decr.tenant_id = de.tenant_id
      AND decr.digital_employee_id = de.id
      AND decr.status = 'active'
      AND decr.archived_at IS NULL
    ORDER BY decr.revision_number DESC, decr.updated_at DESC
    LIMIT 1
) config_state ON TRUE
LEFT JOIN LATERAL (
    SELECT
        LEAST(
            COALESCE(
                SUM(
                    CASE
                        WHEN COALESCE(tr.result #>> '{usage,total_tokens}', tr.result ->> 'total_tokens', '') ~ '^[0-9]+$'
                        THEN COALESCE(tr.result #>> '{usage,total_tokens}', tr.result ->> 'total_tokens', '')::bigint
                        ELSE 0
                    END
                ),
                0
            ),
            2147483647
        )::integer AS usage_tokens_today
    FROM task_runs tr
    WHERE tr.tenant_id = de.tenant_id
      AND tr.digital_employee_id = de.id
      AND COALESCE(tr.finished_at, tr.updated_at, tr.created_at) >= (date_trunc('day', timezone('Asia/Shanghai', now())) AT TIME ZONE 'Asia/Shanghai')
      AND COALESCE(tr.finished_at, tr.updated_at, tr.created_at) < ((date_trunc('day', timezone('Asia/Shanghai', now())) + INTERVAL '1 day') AT TIME ZONE 'Asia/Shanghai')
) today_usage ON TRUE
WHERE de.id = sqlc.arg('digital_employee_id')::uuid
  AND de.tenant_id = sqlc.arg('tenant_id')::uuid
  AND de.deleted_at IS NULL
  AND de.archived_at IS NULL;

-- name: GetProjectTaskRunPreflightForNode :one
-- Dispatch preflight: the node has already been resolved by the Go-level
-- three-layer resolver (internal/project.Service.ResolveProjectTaskNode), which
-- has already checked eligibility, online status, capacity, and (task) pin
-- semantics. This query only confirms the resolved node is still online and
-- assembles the workspace/budget/session/provider-health facts needed to start
-- a run on it. rn.status/disabled_at/archived_at stay as a final safety net —
-- cheap belt-and-suspenders against a node going offline between resolution and
-- this query running; if that happens this scans zero rows and the caller sees
-- pgx.ErrNoRows like any other preflight-absent case.
SELECT
    de.tenant_id,
    de.team_id,
    de.id AS digital_employee_id,
    de.status AS digital_employee_status,
    rn.id AS runtime_node_id,
    rn.node_id,
    de.provider_type,
    COALESCE(
        provider_capability.details ->> 'agent_home_dir',
        provider_capability.metadata ->> 'agent_home_dir',
        provider_capability.workspace_base_dir,
        workspace_capability.details ->> 'agent_home_dir',
        workspace_capability.metadata ->> 'agent_home_dir',
        workspace_capability.workspace_base_dir,
        rn.metadata ->> 'agent_home_dir',
        provider_capability.details ->> 'workspace_base_dir',
        provider_capability.metadata ->> 'workspace_base_dir',
        workspace_capability.details ->> 'workspace_base_dir',
        workspace_capability.metadata ->> 'workspace_base_dir',
        ''
    )::text AS workspace_base_dir,
    COALESCE(config_state.budget_policy, '{}'::jsonb)::jsonb AS budget_policy,
    COALESCE(today_usage.usage_tokens_today, 0)::integer AS today_token_usage,
    'Asia/Shanghai'::text AS business_timezone,
    (runtime_session.id IS NOT NULL)::boolean AS runtime_session_active,
    (provider_capability.id IS NOT NULL)::boolean AS provider_healthy
FROM digital_employees de
JOIN runtime_nodes rn
  ON rn.id = sqlc.arg('resolved_node_id')::uuid
 AND rn.tenant_id = de.tenant_id
 AND rn.status = 'online'
 AND rn.disabled_at IS NULL
 AND rn.archived_at IS NULL
LEFT JOIN LATERAL (
    SELECT rc.*
    FROM runtime_capabilities rc
    WHERE rc.tenant_id = de.tenant_id
      AND rc.runtime_node_id = rn.id
      AND rc.capability_type = 'provider'
      AND rc.provider_type = de.provider_type
      AND rc.available = true
      AND rc.status = 'healthy'
      AND rc.health_status = 'healthy'
      AND rc.archived_at IS NULL
    ORDER BY rc.last_seen_at DESC NULLS LAST, rc.updated_at DESC
    LIMIT 1
) provider_capability ON TRUE
LEFT JOIN LATERAL (
    SELECT rc.*
    FROM runtime_capabilities rc
    WHERE rc.tenant_id = de.tenant_id
      AND rc.runtime_node_id = rn.id
      AND rc.capability_type = 'workspace'
      AND rc.available = true
      AND rc.archived_at IS NULL
    ORDER BY
      CASE WHEN rc.capability_key = 'base-dir' THEN 0 ELSE 1 END,
      rc.last_seen_at DESC NULLS LAST,
      rc.updated_at DESC
    LIMIT 1
) workspace_capability ON TRUE
LEFT JOIN LATERAL (
    SELECT rs.id
    FROM runtime_sessions rs
    JOIN runtime_enrollments re
      ON re.id = rs.enrollment_id
     AND re.tenant_id = rs.tenant_id
     AND re.runtime_node_id = rs.runtime_node_id
     AND re.status = 'approved'
     AND re.rejected_at IS NULL
     AND re.revoked_at IS NULL
    WHERE rs.tenant_id = de.tenant_id
      AND rs.runtime_node_id = rn.id
      AND rs.expires_at > NOW()
      AND rs.revoked_at IS NULL
    ORDER BY rs.last_seen_at DESC, rs.updated_at DESC
    LIMIT 1
) runtime_session ON TRUE
LEFT JOIN LATERAL (
    SELECT
        decr.budget_policy
    FROM digital_employee_config_revisions decr
    WHERE decr.tenant_id = de.tenant_id
      AND decr.digital_employee_id = de.id
      AND decr.status = 'active'
      AND decr.archived_at IS NULL
    ORDER BY decr.revision_number DESC, decr.updated_at DESC
    LIMIT 1
) config_state ON TRUE
LEFT JOIN LATERAL (
    SELECT
        LEAST(
            COALESCE(
                SUM(
                    CASE
                        WHEN COALESCE(tr.result #>> '{usage,total_tokens}', tr.result ->> 'total_tokens', '') ~ '^[0-9]+$'
                        THEN COALESCE(tr.result #>> '{usage,total_tokens}', tr.result ->> 'total_tokens', '')::bigint
                        ELSE 0
                    END
                ),
                0
            ),
            2147483647
        )::integer AS usage_tokens_today
    FROM task_runs tr
    WHERE tr.tenant_id = de.tenant_id
      AND tr.digital_employee_id = de.id
      AND COALESCE(tr.finished_at, tr.updated_at, tr.created_at) >= (date_trunc('day', timezone('Asia/Shanghai', now())) AT TIME ZONE 'Asia/Shanghai')
      AND COALESCE(tr.finished_at, tr.updated_at, tr.created_at) < ((date_trunc('day', timezone('Asia/Shanghai', now())) + INTERVAL '1 day') AT TIME ZONE 'Asia/Shanghai')
) today_usage ON TRUE
WHERE de.id = sqlc.arg('digital_employee_id')::uuid
  AND de.tenant_id = sqlc.arg('tenant_id')::uuid
  AND de.deleted_at IS NULL
  AND de.archived_at IS NULL;

-- name: GetDigitalEmployeeOverviewSummary :one
WITH overview_args AS (
    SELECT
        sqlc.arg('tenant_id')::uuid AS tenant_id,
        NULLIF(BTRIM(sqlc.narg('q')::text), '') AS q,
        sqlc.narg('team_id')::uuid AS team_id,
        NULLIF(BTRIM(sqlc.narg('status')::text), '') AS status,
        NULLIF(BTRIM(sqlc.narg('employee_type')::text), '') AS employee_type,
        NULLIF(BTRIM(sqlc.narg('provider_type')::text), '') AS provider_type,
        NULLIF(BTRIM(sqlc.narg('risk_level')::text), '') AS risk_level,
        NULLIF(BTRIM(sqlc.narg('run_status')::text), '') AS run_status
),
-- 租户内当前具备在线可用 Runtime 能力的 provider 集合。员工不再绑定 Runtime
-- (运行落点由项目派发时动态解析),就绪判据只看"租户内是否有任一在线节点提供该 provider"。
available_provider_types AS (
    SELECT DISTINCT rc.provider_type
    FROM runtime_capabilities rc
    JOIN overview_args args ON args.tenant_id = rc.tenant_id
    JOIN runtime_nodes rn
      ON rn.id = rc.runtime_node_id
     AND rn.tenant_id = rc.tenant_id
    WHERE rc.capability_type = 'provider'
      AND rc.archived_at IS NULL
      AND rc.available = true
      AND rc.status = 'healthy'
      AND rc.health_status = 'healthy'
      AND rn.status = 'online'
      AND rn.disabled_at IS NULL
      AND rn.archived_at IS NULL
),
latest_runs AS (
    SELECT DISTINCT ON (tr.tenant_id, tr.digital_employee_id)
        tr.tenant_id,
        tr.digital_employee_id,
        tr.status
    FROM task_runs tr
    JOIN overview_args args ON args.tenant_id = tr.tenant_id
    JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
    WHERE tr.digital_employee_id IS NOT NULL
      AND t.deleted_at IS NULL
    ORDER BY tr.tenant_id, tr.digital_employee_id, tr.updated_at DESC, tr.created_at DESC
),
employee_config_states AS (
    SELECT DISTINCT ON (decr.tenant_id, decr.digital_employee_id)
        decr.tenant_id,
        decr.digital_employee_id,
        decr.id AS effective_config_id,
        decr.revision_number AS employee_revision_number,
        CASE
            WHEN decr.status = 'active' AND decr.archived_at IS NULL THEN 'approved'
            WHEN decr.status IN ('draft', 'pending_approval') AND decr.archived_at IS NULL THEN 'pending_approval'
            WHEN decr.status = 'archived' OR decr.archived_at IS NOT NULL THEN 'stale'
            ELSE COALESCE(NULLIF(BTRIM(decr.status), ''), 'missing')
        END::text AS governance_status
    FROM digital_employee_config_revisions decr
    JOIN overview_args args ON args.tenant_id = decr.tenant_id
    ORDER BY decr.tenant_id, decr.digital_employee_id, decr.revision_number DESC, decr.updated_at DESC
),
overview_rows AS (
    SELECT
        de.id,
        de.name,
        de.role,
        de.description,
        de.team_id,
        de.status AS employee_status,
        de.employee_type,
        de.risk_level,
        de.provider_type,
        (de.provider_type IN (SELECT apt.provider_type FROM available_provider_types apt))::boolean AS tenant_provider_available,
        COALESCE(lr.status, 'none')::text AS run_status,
        ecs.effective_config_id,
        COALESCE(ecs.governance_status, 'missing')::text AS governance_status
    FROM digital_employees de
    CROSS JOIN overview_args args
    LEFT JOIN latest_runs lr
      ON lr.tenant_id = de.tenant_id
     AND lr.digital_employee_id = de.id
    LEFT JOIN employee_config_states ecs
      ON ecs.tenant_id = de.tenant_id
     AND ecs.digital_employee_id = de.id
    WHERE de.tenant_id = args.tenant_id
      AND de.deleted_at IS NULL
),
filtered_rows AS (
    SELECT overview_rows.*
    FROM overview_rows
    CROSS JOIN overview_args args
    WHERE (
        args.q IS NULL
        OR overview_rows.name ILIKE '%' || args.q || '%'
        OR overview_rows.role ILIKE '%' || args.q || '%'
        OR overview_rows.description ILIKE '%' || args.q || '%'
    )
      AND (args.team_id IS NULL OR overview_rows.team_id = args.team_id)
      AND (args.status IS NULL OR overview_rows.employee_status = args.status)
      AND (args.employee_type IS NULL OR overview_rows.employee_type = args.employee_type)
      AND (args.provider_type IS NULL OR overview_rows.provider_type = args.provider_type)
      AND (args.risk_level IS NULL OR overview_rows.risk_level = args.risk_level)
      AND (args.run_status IS NULL OR overview_rows.run_status = args.run_status)
)
SELECT
    COUNT(*)::integer AS total_count,
    (COUNT(*) FILTER (
        WHERE employee_status IN ('ready', 'active')
          AND effective_config_id IS NOT NULL
          AND governance_status = 'approved'
          AND tenant_provider_available = true
    ))::integer AS runnable_count,
    (COUNT(*) FILTER (
        WHERE run_status IN ('queued', 'dispatching', 'running', 'cancelling')
    ))::integer AS running_count,
    (COUNT(*) FILTER (
        WHERE tenant_provider_available = false
    ))::integer AS waiting_runtime_count,
    (COUNT(*) FILTER (
        WHERE employee_status IN ('disabled', 'error')
           OR run_status IN ('failed', 'timed_out')
    ))::integer AS error_count,
    (COUNT(*) FILTER (
        WHERE risk_level IN ('high', 'critical')
    ))::integer AS high_risk_count,
    (COUNT(*) FILTER (
        WHERE employee_status IN ('ready', 'active')
          AND effective_config_id IS NOT NULL
          AND governance_status = 'approved'
          AND tenant_provider_available = true
          AND run_status NOT IN ('failed', 'timed_out')
    ))::integer AS ready_count,
    (COUNT(*) FILTER (
        WHERE employee_status NOT IN ('ready', 'active')
           OR governance_status <> 'approved'
    ))::integer AS needs_configuration_count,
    (COUNT(*) FILTER (
        WHERE governance_status IN ('missing', 'pending_approval', 'stale')
    ))::integer AS pending_config_approval_count,
    (COUNT(*) FILTER (
        WHERE run_status IN ('failed', 'timed_out')
    ))::integer AS failed_recent_run_count,
    (COUNT(*) FILTER (
        WHERE governance_status IN ('missing', 'pending_approval', 'stale')
    ))::integer AS stale_config_count
FROM filtered_rows;

-- name: ListDigitalEmployeeOverviewItems :many
WITH overview_args AS (
    SELECT
        sqlc.arg('tenant_id')::uuid AS tenant_id,
        NULLIF(BTRIM(sqlc.narg('q')::text), '') AS q,
        sqlc.narg('team_id')::uuid AS team_id,
        NULLIF(BTRIM(sqlc.narg('status')::text), '') AS status,
        NULLIF(BTRIM(sqlc.narg('employee_type')::text), '') AS employee_type,
        NULLIF(BTRIM(sqlc.narg('provider_type')::text), '') AS provider_type,
        NULLIF(BTRIM(sqlc.narg('risk_level')::text), '') AS risk_level,
        NULLIF(BTRIM(sqlc.narg('run_status')::text), '') AS run_status,
        sqlc.narg('employee_ids')::uuid[] AS employee_ids,
        sqlc.arg('limit')::integer AS limit_value,
        sqlc.arg('offset')::integer AS offset_value
),
-- 租户内当前具备在线可用 Runtime 能力的 provider 集合(判据说明见 GetDigitalEmployeeOverviewSummary)。
available_provider_types AS (
    SELECT DISTINCT rc.provider_type
    FROM runtime_capabilities rc
    JOIN overview_args args ON args.tenant_id = rc.tenant_id
    JOIN runtime_nodes rn
      ON rn.id = rc.runtime_node_id
     AND rn.tenant_id = rc.tenant_id
    WHERE rc.capability_type = 'provider'
      AND rc.archived_at IS NULL
      AND rc.available = true
      AND rc.status = 'healthy'
      AND rc.health_status = 'healthy'
      AND rn.status = 'online'
      AND rn.disabled_at IS NULL
      AND rn.archived_at IS NULL
),
provider_capabilities AS (
    SELECT DISTINCT ON (rc.tenant_id, rc.runtime_node_id, rc.provider_type)
        rc.tenant_id,
        rc.runtime_node_id,
        rc.provider_type,
        rc.status,
        rc.health_status,
        rc.available
    FROM runtime_capabilities rc
    JOIN overview_args args ON args.tenant_id = rc.tenant_id
    WHERE rc.capability_type = 'provider'
      AND rc.archived_at IS NULL
    ORDER BY rc.tenant_id, rc.runtime_node_id, rc.provider_type, rc.last_seen_at DESC NULLS LAST, rc.updated_at DESC
),
latest_runs AS (
    SELECT DISTINCT ON (tr.tenant_id, tr.digital_employee_id)
        tr.tenant_id,
        tr.digital_employee_id,
        tr.id,
        tr.task_id,
        t.title,
        tr.status,
        tr.started_at,
        tr.finished_at,
        tr.updated_at,
        tr.result,
        tr.error_message,
        tr.error_family,
        tr.error_code,
        tr.failure_acknowledged_at,
        tr.created_at,
        tr.runtime_node_id,
        tr.node_id AS run_node_id
    FROM task_runs tr
    JOIN overview_args args ON args.tenant_id = tr.tenant_id
    JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
    WHERE tr.digital_employee_id IS NOT NULL
      AND t.deleted_at IS NULL
    ORDER BY tr.tenant_id, tr.digital_employee_id, tr.updated_at DESC, tr.created_at DESC
),
budget_run_tokens AS (
    SELECT
        tr.tenant_id,
        tr.digital_employee_id,
        COALESCE(tr.result #>> '{usage,total_tokens}', tr.result ->> 'total_tokens', '') AS token_text
    FROM task_runs tr
    JOIN overview_args args ON args.tenant_id = tr.tenant_id
    WHERE tr.digital_employee_id IS NOT NULL
      AND tr.created_at >= NOW() - INTERVAL '30 days'
),
budget_runs AS (
    SELECT
        tenant_id,
        digital_employee_id,
        CASE
            WHEN COUNT(*) FILTER (WHERE token_text ~ '^[0-9]+$') = 0 THEN NULL
            ELSE LEAST(
                SUM(CASE WHEN token_text ~ '^[0-9]+$' THEN token_text::bigint ELSE 0 END),
                2147483647
            )::integer
        END AS budget_usage_tokens_30d,
        COUNT(*)::integer AS budget_run_count_30d
    FROM budget_run_tokens
    GROUP BY tenant_id, digital_employee_id
),
today_budget_usage AS (
    SELECT
        tr.tenant_id,
        tr.digital_employee_id,
        LEAST(
            COALESCE(
                SUM(
                    CASE
                        WHEN COALESCE(tr.result #>> '{usage,total_tokens}', tr.result ->> 'total_tokens', '') ~ '^[0-9]+$'
                        THEN COALESCE(tr.result #>> '{usage,total_tokens}', tr.result ->> 'total_tokens', '')::bigint
                        ELSE 0
                    END
                ),
                0
            ),
            2147483647
        )::integer AS today_budget_usage_tokens
    FROM task_runs tr
    JOIN overview_args args ON args.tenant_id = tr.tenant_id
    WHERE tr.digital_employee_id IS NOT NULL
      AND COALESCE(tr.finished_at, tr.updated_at, tr.created_at) >= (date_trunc('day', timezone('Asia/Shanghai', now())) AT TIME ZONE 'Asia/Shanghai')
      AND COALESCE(tr.finished_at, tr.updated_at, tr.created_at) < ((date_trunc('day', timezone('Asia/Shanghai', now())) + INTERVAL '1 day') AT TIME ZONE 'Asia/Shanghai')
    GROUP BY tr.tenant_id, tr.digital_employee_id
),
employee_config_states AS (
    SELECT DISTINCT ON (decr.tenant_id, decr.digital_employee_id)
        decr.tenant_id,
        decr.digital_employee_id,
        decr.id AS effective_config_id,
        decr.persona_memory_markdown,
        decr.capability_bindings,
        decr.budget_policy,
        NULLIF(decr.budget_policy #>> '{daily_token_limit}', '') AS daily_token_limit_text,
        decr.revision_number AS employee_revision_number,
        CASE
            WHEN decr.status = 'active' AND decr.archived_at IS NULL THEN 'approved'
            WHEN decr.status IN ('draft', 'pending_approval') AND decr.archived_at IS NULL THEN 'pending_approval'
            WHEN decr.status = 'archived' OR decr.archived_at IS NOT NULL THEN 'stale'
            ELSE COALESCE(NULLIF(BTRIM(decr.status), ''), 'missing')
        END::text AS governance_status
    FROM digital_employee_config_revisions decr
    JOIN overview_args args ON args.tenant_id = decr.tenant_id
    ORDER BY decr.tenant_id, decr.digital_employee_id, decr.revision_number DESC, decr.updated_at DESC
),
-- mcp_servers_count 与 skills_count 同口径:员工直挂绑定表计数(能力绑定统一后
-- config revision JSON 不再承载 mcp_servers 声明)。
mcp_counts AS (
    SELECT
        demb.tenant_id,
        demb.digital_employee_id,
        COUNT(*)::integer AS mcp_servers_count
    FROM digital_employee_mcp_bindings_v2 demb
    JOIN overview_args args ON args.tenant_id = demb.tenant_id
    JOIN mcp_servers m
      ON m.id = demb.mcp_server_id
     AND m.tenant_id = demb.tenant_id
     AND m.deleted_at IS NULL
    WHERE demb.deleted_at IS NULL
    GROUP BY demb.tenant_id, demb.digital_employee_id
),
skill_counts AS (
    SELECT
        sab.tenant_id,
        sab.digital_employee_id,
        COUNT(*)::integer AS skills_count
    FROM skill_agent_bindings sab
    JOIN overview_args args ON args.tenant_id = sab.tenant_id
    JOIN skills s
      ON s.id = sab.skill_id
     AND s.tenant_id = sab.tenant_id
     AND s.deleted_at IS NULL
    WHERE sab.status = 'enabled'
    GROUP BY sab.tenant_id, sab.digital_employee_id
),
-- 员工级人工等待判据(2026-07-19 收窄):任务上任一未决决策请求都计入,不再按
-- 决策类型死词表('task_failure_recovery','route_review'——这两个字符串与实际
-- 创建的类型早已脱节,导致这一腿永远不触发)过滤;唯一排除 project_acceptance,
-- 它是项目级 guard,不构成员工级 waiting_human(见 operational_status.go)。
pending_employee_decisions AS (
    SELECT
        pt.tenant_id,
        pt.assigned_digital_employee_id AS digital_employee_id,
        count(*) FILTER (
            WHERE pdr.decision_type <> 'project_acceptance'
        ) > 0 AS has_employee_scoped_human_blocker,
        count(*) FILTER (
            WHERE pdr.decision_type = 'project_acceptance'
        ) > 0 AS has_project_acceptance_blocker
    FROM project_decision_requests pdr
    JOIN project_tasks pt
      ON pt.tenant_id = pdr.tenant_id
     AND pt.id = pdr.project_task_id
    JOIN overview_args args ON args.tenant_id = pt.tenant_id
    WHERE pt.assigned_digital_employee_id IS NOT NULL
      AND pdr.status_snapshot IN ('pending', 'requested')
    GROUP BY pt.tenant_id, pt.assigned_digital_employee_id
),
employee_operational_facts AS (
    SELECT
        de.tenant_id,
        de.id AS digital_employee_id,
        -- 待确认判据收窄(2026-07-19):只认"此刻真的在等人"——任务状态已是
        -- waiting_human/pending_review,或任务上挂着未决决策请求(ped)。
        -- requires_human_approval 且未到审批点的任务(planned/blocked/running)
        -- 不再点亮待确认:到达审批点时任务自身会转 waiting_human。
        (
            coalesce(ped.has_employee_scoped_human_blocker, false)
            OR count(pt.id) FILTER (
                WHERE pt.status IN ('waiting_human', 'pending_review')
            ) > 0
            OR EXISTS (
                SELECT 1
                FROM inbox_items ii
                JOIN task_runs tr_rec
                  ON tr_rec.id = ii.source_id
                 AND tr_rec.tenant_id = ii.tenant_id
                WHERE ii.tenant_id = de.tenant_id
                  AND ii.item_type = 'digital_employee_run_recovery'
                  AND ii.status = 'open'
                  AND tr_rec.digital_employee_id = de.id
            )
        ) AS operational_has_employee_scoped_human_blocker,
        coalesce(ped.has_project_acceptance_blocker, false) AS operational_has_project_acceptance_blocker,
        count(pt.id) FILTER (WHERE pt.status IN ('queued')) > 0 AS operational_has_queued_work,
        count(pt.id) FILTER (WHERE pt.status IN ('running', 'in_progress')) > 0 AS operational_has_working_task,
        count(pt.id) FILTER (
            WHERE (
                pt.requires_human_approval
                AND pt.status NOT IN ('completed', 'done', 'success', 'cancelled', 'failed')
            )
               OR pt.status IN ('pending', 'planned', 'queued', 'blocked', 'running', 'in_progress', 'waiting_human', 'pending_review')
        ) > 0 AS operational_has_active_work,
        -- 失败任务只在「仍需关注」时点亮异常:已有非 pending 的恢复决策
        -- (retry/reassign/cancel 或人类已处理)后不再因历史 failed 行钉死员工。
        count(pt.id) FILTER (
            WHERE pt.status = 'failed'
              AND NOT EXISTS (
                SELECT 1
                FROM project_decision_requests pdr
                WHERE pdr.tenant_id = pt.tenant_id
                  AND pdr.project_task_id = pt.id
                  AND pdr.decision_type IN (
                    'task_failure_recovery',
                    'project_task_recovery',
                    'project_task_runtime_recovery'
                  )
                  AND COALESCE(pdr.status_snapshot, '') NOT IN ('pending', 'requested')
              )
        ) > 0 AS operational_has_task_failure
    FROM digital_employees de
    JOIN overview_args args ON args.tenant_id = de.tenant_id
    LEFT JOIN project_tasks pt
      ON pt.tenant_id = de.tenant_id
     AND pt.assigned_digital_employee_id = de.id
     AND pt.dismissed_at IS NULL
     AND (
         pt.status IN ('pending', 'planned', 'queued', 'blocked', 'running', 'in_progress', 'waiting_human', 'pending_review', 'failed')
         OR (
             pt.requires_human_approval
             AND pt.status NOT IN ('completed', 'done', 'success', 'cancelled', 'failed')
         )
     )
    LEFT JOIN project_task_attempts pta
      ON pta.tenant_id = pt.tenant_id
     AND pta.project_task_id = pt.id
     AND pta.id = pt.current_attempt_id
    LEFT JOIN pending_employee_decisions ped
      ON ped.tenant_id = de.tenant_id
     AND ped.digital_employee_id = de.id
    WHERE de.deleted_at IS NULL
    GROUP BY
        de.tenant_id,
        de.id,
        ped.has_employee_scoped_human_blocker,
        ped.has_project_acceptance_blocker
),
-- 跨视图一致性(P2 3.3b):working 状态的权威成因。取每个员工当前 running/in_progress
-- 的 project_task(与 operational_has_working_task 同源),携其所属项目名,供座位卡精确
-- 显示"正在 X 项目做 Y 任务"并深链——替代前端从 latest_run(task_runs 另一数据源)+
-- project_summary 聚合的启发式拼接(可能指向不同的工作)。多条时取最近更新的一条。
employee_working_task AS (
    SELECT DISTINCT ON (pt.tenant_id, pt.assigned_digital_employee_id)
        pt.tenant_id,
        pt.assigned_digital_employee_id AS digital_employee_id,
        pt.id AS project_task_id,
        pt.title AS project_task_title,
        pt.project_id,
        p.name AS project_name
    FROM project_tasks pt
    JOIN overview_args args ON args.tenant_id = pt.tenant_id
    LEFT JOIN projects p
      ON p.id = pt.project_id
     AND p.tenant_id = pt.tenant_id
    WHERE pt.assigned_digital_employee_id IS NOT NULL
      AND pt.status IN ('running', 'in_progress')
    ORDER BY pt.tenant_id, pt.assigned_digital_employee_id, pt.updated_at DESC
),
overview_rows AS (
    SELECT
        de.id,
        de.tenant_id,
        de.team_id,
        COALESCE(tt.name, '')::text AS team_name,
        de.owner_user_id,
        COALESCE(au.display_name, au.username, '')::text AS owner_display_name,
        de.employee_type,
        de.name,
        de.role,
        de.description,
        de.status,
        de.risk_level,
        de.metadata,
        -- execution_summary 落点改取最近一次 task_runs 真实派发节点(dei 已退役)。
        -- 无运行记录的员工落点为空(候岗),execution_instance_id 恒空。
        NULL::uuid AS execution_instance_id,
        CASE
            WHEN lr.id IS NULL THEN 'missing'
            WHEN lr.status IN ('running', 'dispatching', 'queued', 'cancelling') THEN 'active'
            WHEN lr.status IN ('failed', 'timed_out') THEN 'error'
            ELSE 'ready'
        END::text AS execution_status,
        lr.runtime_node_id,
        COALESCE(rn.node_id, COALESCE(lr.run_node_id, ''))::text AS node_id,
        COALESCE(rn.name, '')::text AS runtime_name,
        COALESCE(rn.status, '')::text AS runtime_status,
        rn.disabled_at AS runtime_disabled_at,
        rn.archived_at AS runtime_archived_at,
        COALESCE(de.provider_type, '')::text AS provider_type,
        de.provider_type AS identity_provider_type,
        (de.provider_type IN (SELECT apt.provider_type FROM available_provider_types apt))::boolean AS tenant_provider_available,
        COALESCE(pc.available, false)::boolean AS provider_available,
        COALESCE(pc.status, 'unknown')::text AS provider_status,
        COALESCE(pc.health_status, 'unknown')::text AS health_status,
        false::boolean AS agent_home_dir_available,
        lr.id AS latest_run_id,
        lr.task_id AS latest_run_task_id,
        CASE
            WHEN lr.status IN ('failed', 'timed_out') AND lr.failure_acknowledged_at IS NOT NULL THEN 'none'
            ELSE COALESCE(lr.status, 'none')
        END::text AS latest_run_status,
        COALESCE(lr.title, '')::text AS latest_run_title,
        lr.started_at AS latest_run_started_at,
        lr.finished_at AS latest_run_finished_at,
        lr.updated_at AS latest_run_updated_at,
        COALESCE((CASE
            WHEN lr.id IS NOT NULL AND lr.finished_at IS NOT NULL THEN
                GREATEST(EXTRACT(EPOCH FROM (lr.finished_at - lr.started_at))::integer, 0)
            ELSE NULL
        END)::text, '')::text AS latest_run_duration_sec,
        COALESCE(lr.result #>> '{usage,total_tokens}', lr.result ->> 'total_tokens', '')::text AS latest_run_token_usage,
        lr.error_message AS latest_run_error_message,
        COALESCE(lr.error_family, '')::text AS latest_run_error_family,
        COALESCE(lr.error_code, '')::text AS latest_run_error_code,
        ecs.effective_config_id,
        COALESCE(ecs.governance_status, 'missing')::text AS governance_status,
        COALESCE(ecs.daily_token_limit_text, '')::text AS daily_token_limit_text,
        NULL::integer AS team_revision_number,
        ecs.employee_revision_number,
        COALESCE(sc.skills_count, 0)::integer AS skills_count,
        COALESCE(mc.mcp_servers_count, 0)::integer AS mcp_servers_count,
        COALESCE(tt.constitution #>> '{ref}', tt.constitution #>> '{document_ref}', '')::text AS constitution_ref,
        COALESCE(tbu.today_budget_usage_tokens, 0)::integer AS today_budget_usage_tokens,
        br.budget_usage_tokens_30d,
        COALESCE(br.budget_run_count_30d, 0)::integer AS budget_run_count_30d,
        coalesce(eof.operational_has_employee_scoped_human_blocker, false)::boolean AS operational_has_employee_scoped_human_blocker,
        coalesce(eof.operational_has_project_acceptance_blocker, false)::boolean AS operational_has_project_acceptance_blocker,
        coalesce(eof.operational_has_queued_work, false)::boolean AS operational_has_queued_work,
        coalesce(eof.operational_has_working_task, false)::boolean AS operational_has_working_task,
        coalesce(eof.operational_has_active_work, false)::boolean AS operational_has_active_work,
        coalesce(eof.operational_has_task_failure, false)::boolean AS operational_has_task_failure,
        ewt.project_id AS working_project_id,
        COALESCE(ewt.project_name, '')::text AS working_project_name,
        ewt.project_task_id AS working_project_task_id,
        COALESCE(ewt.project_task_title, '')::text AS working_project_task_title,
        de.created_at,
        de.updated_at
    FROM digital_employees de
    CROSS JOIN overview_args args
    LEFT JOIN tenant_teams tt
      ON tt.id = de.team_id
     AND tt.tenant_id = de.tenant_id
     AND tt.deleted_at IS NULL
    LEFT JOIN auth_users au
      ON au.id = de.owner_user_id
     AND au.deleted_at IS NULL
    LEFT JOIN latest_runs lr
      ON lr.tenant_id = de.tenant_id
     AND lr.digital_employee_id = de.id
    LEFT JOIN runtime_nodes rn
      ON rn.id = lr.runtime_node_id
     AND rn.tenant_id = de.tenant_id
    LEFT JOIN provider_capabilities pc
      ON pc.tenant_id = de.tenant_id
     AND pc.runtime_node_id = lr.runtime_node_id
     AND pc.provider_type = de.provider_type
    LEFT JOIN budget_runs br
      ON br.tenant_id = de.tenant_id
     AND br.digital_employee_id = de.id
    LEFT JOIN today_budget_usage tbu
      ON tbu.tenant_id = de.tenant_id
     AND tbu.digital_employee_id = de.id
    LEFT JOIN employee_config_states ecs
      ON ecs.tenant_id = de.tenant_id
     AND ecs.digital_employee_id = de.id
    LEFT JOIN skill_counts sc
      ON sc.tenant_id = de.tenant_id
     AND sc.digital_employee_id = de.id
    LEFT JOIN mcp_counts mc
      ON mc.tenant_id = de.tenant_id
     AND mc.digital_employee_id = de.id
    LEFT JOIN employee_operational_facts eof
      ON eof.tenant_id = de.tenant_id
     AND eof.digital_employee_id = de.id
    LEFT JOIN employee_working_task ewt
      ON ewt.tenant_id = de.tenant_id
     AND ewt.digital_employee_id = de.id
    WHERE de.tenant_id = args.tenant_id
      AND de.deleted_at IS NULL
),
filtered_rows AS (
    SELECT overview_rows.*
    FROM overview_rows
    CROSS JOIN overview_args args
    WHERE (
        args.q IS NULL
        OR overview_rows.name ILIKE '%' || args.q || '%'
        OR overview_rows.role ILIKE '%' || args.q || '%'
        OR overview_rows.description ILIKE '%' || args.q || '%'
    )
      AND (args.team_id IS NULL OR overview_rows.team_id = args.team_id)
      AND (args.status IS NULL OR overview_rows.status = args.status)
      AND (args.employee_type IS NULL OR overview_rows.employee_type = args.employee_type)
      AND (args.provider_type IS NULL OR overview_rows.identity_provider_type = args.provider_type)
      AND (args.risk_level IS NULL OR overview_rows.risk_level = args.risk_level)
      AND (args.run_status IS NULL OR overview_rows.latest_run_status = args.run_status)
      -- operational_status 过滤：计算态由 Go 状态机在 operational facts 上裁决后，以命中 ID 集合下推。
      AND (args.employee_ids IS NULL OR overview_rows.id = ANY(args.employee_ids))
),
paged_rows AS (
    SELECT *
    FROM filtered_rows
    ORDER BY created_at DESC, id
    LIMIT (SELECT limit_value FROM overview_args)
    OFFSET (SELECT offset_value FROM overview_args)
),
recent_events AS (
    SELECT
        ranked.tenant_id,
        ranked.digital_employee_id,
        jsonb_agg(
            jsonb_build_object(
                'event_type', ranked.event_type,
                'occurred_at', ranked.occurred_at
            )
            ORDER BY ranked.occurred_at DESC NULLS LAST, ranked.sequence_number DESC
        ) AS recent_events_json
    FROM (
        SELECT
            pr.tenant_id,
            pr.id AS digital_employee_id,
            te.sequence_number,
            -- 事件类型到中文标签/状态的映射收敛在 Go 层（employee.ActivityEventPresentation），SQL 只透传原始 event_type。
            te.event_type,
            COALESCE(te.created_at, tr.updated_at, tr.created_at) AS occurred_at,
            ROW_NUMBER() OVER (
                PARTITION BY pr.tenant_id, pr.id
                ORDER BY COALESCE(te.created_at, tr.updated_at, tr.created_at) DESC, te.sequence_number DESC
            ) AS row_number
        FROM paged_rows pr
        JOIN task_runs tr
          ON tr.tenant_id = pr.tenant_id
         AND tr.digital_employee_id = pr.id
        JOIN tasks t
          ON t.id = tr.task_id
         AND t.tenant_id = tr.tenant_id
         AND t.deleted_at IS NULL
        JOIN task_events te
          ON te.tenant_id = tr.tenant_id
         AND te.run_id = tr.id
    ) ranked
    WHERE ranked.row_number <= 3
    GROUP BY ranked.tenant_id, ranked.digital_employee_id
),
employee_project_links AS (
    SELECT
        pr.tenant_id,
        pr.id AS digital_employee_id,
        pm.project_id,
        TRUE AS is_member
    FROM paged_rows pr
    JOIN project_members pm
      ON pm.tenant_id = pr.tenant_id
     AND pm.principal_type = 'digital_employee'
     AND pm.principal_id = pr.id
     AND pm.status = 'active'
    UNION
    SELECT
        pr.tenant_id,
        pr.id AS digital_employee_id,
        pt.project_id,
        FALSE AS is_member
    FROM paged_rows pr
    JOIN project_tasks pt
      ON pt.tenant_id = pr.tenant_id
     AND pt.assigned_digital_employee_id = pr.id
),
employee_project_stats AS (
    SELECT
        links.tenant_id,
        links.digital_employee_id,
        links.project_id,
        BOOL_OR(links.is_member) AS is_member,
        p.name AS project_name,
        p.status AS project_status,
        COUNT(DISTINCT pt.id) FILTER (
            WHERE pt.status NOT IN ('completed', 'done', 'success', 'cancelled', 'failed')
        )::integer AS active_task_count,
        COUNT(DISTINCT pt.id) FILTER (
            WHERE pt.status IN ('running', 'in_progress')
        )::integer AS working_task_count,
        COUNT(DISTINCT pt.id)::integer AS total_task_count,
        GREATEST(MAX(pt.updated_at), MAX(p.updated_at)) AS last_activity_at
    FROM employee_project_links links
    JOIN projects p
      ON p.id = links.project_id
     AND p.tenant_id = links.tenant_id
     AND p.archived_at IS NULL
    LEFT JOIN project_tasks pt
      ON pt.tenant_id = links.tenant_id
     AND pt.project_id = links.project_id
     AND pt.assigned_digital_employee_id = links.digital_employee_id
     AND pt.dismissed_at IS NULL
    GROUP BY links.tenant_id, links.digital_employee_id, links.project_id, p.name, p.status
),
employee_projects AS (
    SELECT
        s.tenant_id,
        s.digital_employee_id,
        COUNT(*)::integer AS project_count,
        jsonb_agg(
            jsonb_build_object(
                'project_id', s.project_id,
                'name', s.project_name,
                'status', s.project_status,
                'is_member', s.is_member,
                'active_task_count', s.active_task_count,
                'working_task_count', s.working_task_count,
                'total_task_count', s.total_task_count,
                'last_activity_at', s.last_activity_at
            )
            ORDER BY s.last_activity_at DESC NULLS LAST, s.project_id
        ) FILTER (WHERE s.row_number <= 5) AS projects_json
    FROM (
        SELECT
            employee_project_stats.*,
            ROW_NUMBER() OVER (
                PARTITION BY employee_project_stats.tenant_id, employee_project_stats.digital_employee_id
                ORDER BY employee_project_stats.last_activity_at DESC NULLS LAST, employee_project_stats.project_id
            ) AS row_number
        FROM employee_project_stats
    ) s
    GROUP BY s.tenant_id, s.digital_employee_id
)
SELECT
    pr.id,
    pr.tenant_id,
    pr.team_id,
    pr.team_name,
    pr.owner_user_id,
    pr.owner_display_name,
    pr.employee_type,
    pr.name,
    pr.role,
    pr.description,
    pr.status,
    pr.risk_level,
    pr.metadata,
    pr.execution_instance_id,
    pr.execution_status,
    pr.runtime_node_id,
    pr.node_id,
    pr.runtime_name,
    pr.runtime_status,
    pr.runtime_disabled_at,
    pr.runtime_archived_at,
    pr.provider_type,
    pr.identity_provider_type,
    pr.tenant_provider_available,
    pr.provider_available,
    pr.provider_status,
    pr.health_status,
    pr.agent_home_dir_available,
    pr.latest_run_id,
    pr.latest_run_task_id,
    pr.latest_run_status,
    pr.latest_run_title,
    pr.latest_run_started_at,
    pr.latest_run_finished_at,
    pr.latest_run_updated_at,
    pr.latest_run_duration_sec,
    pr.latest_run_token_usage,
    pr.latest_run_error_message,
    pr.latest_run_error_family,
    pr.latest_run_error_code,
    pr.effective_config_id,
    pr.governance_status,
    pr.daily_token_limit_text,
    pr.team_revision_number,
    pr.employee_revision_number,
    pr.skills_count,
    pr.mcp_servers_count,
    pr.constitution_ref,
    pr.today_budget_usage_tokens,
    pr.budget_usage_tokens_30d,
    pr.budget_run_count_30d,
    pr.operational_has_employee_scoped_human_blocker,
    pr.operational_has_project_acceptance_blocker,
    pr.operational_has_queued_work,
    pr.operational_has_working_task,
    pr.operational_has_active_work,
    pr.operational_has_task_failure,
    pr.working_project_id,
    pr.working_project_name,
    pr.working_project_task_id,
    pr.working_project_task_title,
    COALESCE(re.recent_events_json, '[]'::jsonb) AS recent_events_json,
    COALESCE(ep.project_count, 0)::integer AS project_count,
    COALESCE(ep.projects_json, '[]'::jsonb) AS projects_json
FROM paged_rows pr
LEFT JOIN recent_events re
  ON re.tenant_id = pr.tenant_id
 AND re.digital_employee_id = pr.id
LEFT JOIN employee_projects ep
  ON ep.tenant_id = pr.tenant_id
 AND ep.digital_employee_id = pr.id
ORDER BY pr.created_at DESC, pr.id;

-- name: ListDigitalEmployeeOverviewOperationalFacts :many
WITH overview_args AS (
    SELECT
        sqlc.arg('tenant_id')::uuid AS tenant_id,
        NULLIF(BTRIM(sqlc.narg('q')::text), '') AS q,
        sqlc.narg('team_id')::uuid AS team_id,
        NULLIF(BTRIM(sqlc.narg('status')::text), '') AS status,
        NULLIF(BTRIM(sqlc.narg('employee_type')::text), '') AS employee_type,
        NULLIF(BTRIM(sqlc.narg('provider_type')::text), '') AS provider_type,
        NULLIF(BTRIM(sqlc.narg('risk_level')::text), '') AS risk_level,
        NULLIF(BTRIM(sqlc.narg('run_status')::text), '') AS run_status,
        -- 跨视图一致性(P2 3.3a):按单个员工 id 收敛,让员工详情页复用与总览同源的
        -- operational_state 裁决(而非前端本地 hasActiveRun)。总览调用传 NULL 不过滤。
        sqlc.narg('employee_id')::uuid AS employee_id
),
-- 租户内当前具备在线可用 Runtime 能力的 provider 集合(判据说明见 GetDigitalEmployeeOverviewSummary)。
available_provider_types AS (
    SELECT DISTINCT rc.provider_type
    FROM runtime_capabilities rc
    JOIN overview_args args ON args.tenant_id = rc.tenant_id
    JOIN runtime_nodes rn
      ON rn.id = rc.runtime_node_id
     AND rn.tenant_id = rc.tenant_id
    WHERE rc.capability_type = 'provider'
      AND rc.archived_at IS NULL
      AND rc.available = true
      AND rc.status = 'healthy'
      AND rc.health_status = 'healthy'
      AND rn.status = 'online'
      AND rn.disabled_at IS NULL
      AND rn.archived_at IS NULL
),
latest_runs AS (
    SELECT DISTINCT ON (tr.tenant_id, tr.digital_employee_id)
        tr.tenant_id,
        tr.digital_employee_id,
        tr.status,
        tr.error_family,
        tr.error_code,
        tr.failure_acknowledged_at
    FROM task_runs tr
    JOIN overview_args args ON args.tenant_id = tr.tenant_id
    JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
    WHERE tr.digital_employee_id IS NOT NULL
      AND t.deleted_at IS NULL
    ORDER BY tr.tenant_id, tr.digital_employee_id, tr.updated_at DESC, tr.created_at DESC
),
employee_config_states AS (
    SELECT DISTINCT ON (decr.tenant_id, decr.digital_employee_id)
        decr.tenant_id,
        decr.digital_employee_id,
        decr.id AS effective_config_id,
        CASE
            WHEN decr.status = 'active' AND decr.archived_at IS NULL THEN 'approved'
            WHEN decr.status IN ('draft', 'pending_approval') AND decr.archived_at IS NULL THEN 'pending_approval'
            WHEN decr.status = 'archived' OR decr.archived_at IS NOT NULL THEN 'stale'
            ELSE COALESCE(NULLIF(BTRIM(decr.status), ''), 'missing')
        END::text AS governance_status
    FROM digital_employee_config_revisions decr
    JOIN overview_args args ON args.tenant_id = decr.tenant_id
    ORDER BY decr.tenant_id, decr.digital_employee_id, decr.revision_number DESC, decr.updated_at DESC
),
-- 员工级人工等待判据(2026-07-19 收窄):任务上任一未决决策请求都计入,不再按
-- 决策类型死词表('task_failure_recovery','route_review'——这两个字符串与实际
-- 创建的类型早已脱节,导致这一腿永远不触发)过滤;唯一排除 project_acceptance,
-- 它是项目级 guard,不构成员工级 waiting_human(见 operational_status.go)。
pending_employee_decisions AS (
    SELECT
        pt.tenant_id,
        pt.assigned_digital_employee_id AS digital_employee_id,
        count(*) FILTER (
            WHERE pdr.decision_type <> 'project_acceptance'
        ) > 0 AS has_employee_scoped_human_blocker,
        count(*) FILTER (
            WHERE pdr.decision_type = 'project_acceptance'
        ) > 0 AS has_project_acceptance_blocker
    FROM project_decision_requests pdr
    JOIN project_tasks pt
      ON pt.tenant_id = pdr.tenant_id
     AND pt.id = pdr.project_task_id
    JOIN overview_args args ON args.tenant_id = pt.tenant_id
    WHERE pt.assigned_digital_employee_id IS NOT NULL
      AND pdr.status_snapshot IN ('pending', 'requested')
    GROUP BY pt.tenant_id, pt.assigned_digital_employee_id
),
employee_operational_facts AS (
    SELECT
        de.tenant_id,
        de.id AS digital_employee_id,
        -- 待确认判据收窄(2026-07-19):只认"此刻真的在等人"——任务状态已是
        -- waiting_human/pending_review,或任务上挂着未决决策请求(ped)。
        -- requires_human_approval 且未到审批点的任务(planned/blocked/running)
        -- 不再点亮待确认:到达审批点时任务自身会转 waiting_human。
        (
            coalesce(ped.has_employee_scoped_human_blocker, false)
            OR count(pt.id) FILTER (
                WHERE pt.status IN ('waiting_human', 'pending_review')
            ) > 0
            OR EXISTS (
                SELECT 1
                FROM inbox_items ii
                JOIN task_runs tr_rec
                  ON tr_rec.id = ii.source_id
                 AND tr_rec.tenant_id = ii.tenant_id
                WHERE ii.tenant_id = de.tenant_id
                  AND ii.item_type = 'digital_employee_run_recovery'
                  AND ii.status = 'open'
                  AND tr_rec.digital_employee_id = de.id
            )
        ) AS operational_has_employee_scoped_human_blocker,
        coalesce(ped.has_project_acceptance_blocker, false) AS operational_has_project_acceptance_blocker,
        count(pt.id) FILTER (WHERE pt.status IN ('queued')) > 0 AS operational_has_queued_work,
        count(pt.id) FILTER (WHERE pt.status IN ('running', 'in_progress')) > 0 AS operational_has_working_task,
        count(pt.id) FILTER (
            WHERE (
                pt.requires_human_approval
                AND pt.status NOT IN ('completed', 'done', 'success', 'cancelled', 'failed')
            )
               OR pt.status IN ('pending', 'planned', 'queued', 'blocked', 'running', 'in_progress', 'waiting_human', 'pending_review')
        ) > 0 AS operational_has_active_work,
        -- 失败任务只在仍需关注时点亮(已有非 pending 恢复决策则收敛)。
        count(pt.id) FILTER (
            WHERE pt.status = 'failed'
              AND NOT EXISTS (
                SELECT 1
                FROM project_decision_requests pdr
                WHERE pdr.tenant_id = pt.tenant_id
                  AND pdr.project_task_id = pt.id
                  AND pdr.decision_type IN (
                    'task_failure_recovery',
                    'project_task_recovery',
                    'project_task_runtime_recovery'
                  )
                  AND COALESCE(pdr.status_snapshot, '') NOT IN ('pending', 'requested')
              )
        ) > 0 AS operational_has_task_failure
    FROM digital_employees de
    JOIN overview_args args ON args.tenant_id = de.tenant_id
    LEFT JOIN project_tasks pt
      ON pt.tenant_id = de.tenant_id
     AND pt.assigned_digital_employee_id = de.id
     AND pt.dismissed_at IS NULL
     AND (
         pt.status IN ('pending', 'planned', 'queued', 'blocked', 'running', 'in_progress', 'waiting_human', 'pending_review', 'failed')
         OR (
             pt.requires_human_approval
             AND pt.status NOT IN ('completed', 'done', 'success', 'cancelled', 'failed')
         )
     )
    LEFT JOIN project_task_attempts pta
      ON pta.tenant_id = pt.tenant_id
     AND pta.project_task_id = pt.id
     AND pta.id = pt.current_attempt_id
    LEFT JOIN pending_employee_decisions ped
      ON ped.tenant_id = de.tenant_id
     AND ped.digital_employee_id = de.id
    WHERE de.deleted_at IS NULL
    GROUP BY
        de.tenant_id,
        de.id,
        ped.has_employee_scoped_human_blocker,
        ped.has_project_acceptance_blocker
),
overview_rows AS (
    SELECT
        de.id,
        de.name,
        de.role,
        de.description,
        de.team_id,
        de.status,
        de.employee_type,
        de.risk_level,
        de.provider_type,
        (de.provider_type IN (SELECT apt.provider_type FROM available_provider_types apt))::boolean AS tenant_provider_available,
        CASE
            WHEN lr.status IN ('failed', 'timed_out') AND lr.failure_acknowledged_at IS NOT NULL THEN 'none'
            ELSE COALESCE(lr.status, 'none')
        END::text AS latest_run_status,
        COALESCE(lr.error_family, '')::text AS latest_run_error_family,
        COALESCE(lr.error_code, '')::text AS latest_run_error_code,
        ecs.effective_config_id,
        COALESCE(ecs.governance_status, 'missing')::text AS governance_status,
        coalesce(eof.operational_has_employee_scoped_human_blocker, false)::boolean AS operational_has_employee_scoped_human_blocker,
        coalesce(eof.operational_has_project_acceptance_blocker, false)::boolean AS operational_has_project_acceptance_blocker,
        coalesce(eof.operational_has_queued_work, false)::boolean AS operational_has_queued_work,
        coalesce(eof.operational_has_working_task, false)::boolean AS operational_has_working_task,
        coalesce(eof.operational_has_active_work, false)::boolean AS operational_has_active_work,
        coalesce(eof.operational_has_task_failure, false)::boolean AS operational_has_task_failure
    FROM digital_employees de
    CROSS JOIN overview_args args
    LEFT JOIN latest_runs lr
      ON lr.tenant_id = de.tenant_id
     AND lr.digital_employee_id = de.id
    LEFT JOIN employee_config_states ecs
      ON ecs.tenant_id = de.tenant_id
     AND ecs.digital_employee_id = de.id
    LEFT JOIN employee_operational_facts eof
      ON eof.tenant_id = de.tenant_id
     AND eof.digital_employee_id = de.id
    WHERE de.tenant_id = args.tenant_id
      AND de.deleted_at IS NULL
),
filtered_rows AS (
    SELECT overview_rows.*
    FROM overview_rows
    CROSS JOIN overview_args args
    WHERE (
        args.q IS NULL
        OR overview_rows.name ILIKE '%' || args.q || '%'
        OR overview_rows.role ILIKE '%' || args.q || '%'
        OR overview_rows.description ILIKE '%' || args.q || '%'
    )
      AND (args.team_id IS NULL OR overview_rows.team_id = args.team_id)
      AND (args.status IS NULL OR overview_rows.status = args.status)
      AND (args.employee_type IS NULL OR overview_rows.employee_type = args.employee_type)
      AND (args.provider_type IS NULL OR overview_rows.provider_type = args.provider_type)
      AND (args.risk_level IS NULL OR overview_rows.risk_level = args.risk_level)
      AND (args.run_status IS NULL OR overview_rows.latest_run_status = args.run_status)
      AND (args.employee_id IS NULL OR overview_rows.id = args.employee_id)
)
SELECT
    id,
    status,
    provider_type,
    tenant_provider_available,
    latest_run_status,
    latest_run_error_family,
    latest_run_error_code,
    effective_config_id,
    governance_status,
    operational_has_employee_scoped_human_blocker,
    operational_has_project_acceptance_blocker,
    operational_has_queued_work,
    operational_has_working_task,
    operational_has_active_work,
    operational_has_task_failure
FROM filtered_rows
ORDER BY id;

-- name: ListDigitalEmployeeOverviewFilterOptions :many
WITH overview_args AS (
    SELECT sqlc.arg('tenant_id')::uuid AS tenant_id
),
latest_runs AS (
    SELECT DISTINCT ON (tr.tenant_id, tr.digital_employee_id)
        tr.tenant_id,
        tr.digital_employee_id,
        tr.status
    FROM task_runs tr
    JOIN overview_args args ON args.tenant_id = tr.tenant_id
    JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
    WHERE tr.digital_employee_id IS NOT NULL
      AND t.deleted_at IS NULL
    ORDER BY tr.tenant_id, tr.digital_employee_id, tr.updated_at DESC, tr.created_at DESC
),
employee_rows AS (
    SELECT
        de.team_id,
        COALESCE(tt.name, '')::text AS team_name,
        de.employee_type,
        de.status,
        de.risk_level,
        de.provider_type,
        COALESCE(lr.status, 'none')::text AS run_status
    FROM digital_employees de
    CROSS JOIN overview_args args
    LEFT JOIN tenant_teams tt
      ON tt.id = de.team_id
     AND tt.tenant_id = de.tenant_id
     AND tt.deleted_at IS NULL
    LEFT JOIN latest_runs lr
      ON lr.tenant_id = de.tenant_id
     AND lr.digital_employee_id = de.id
    WHERE de.tenant_id = args.tenant_id
      AND de.deleted_at IS NULL
)
SELECT filter_type, value, label
FROM (
    SELECT DISTINCT
        'team'::text AS filter_type,
        COALESCE(team_id::text, '')::text AS value,
        team_name AS label
    FROM employee_rows
    WHERE team_id IS NOT NULL

    UNION ALL

    SELECT DISTINCT
        'employee_type'::text AS filter_type,
        employee_type::text AS value,
        employee_type::text AS label
    FROM employee_rows
    WHERE NULLIF(employee_type, '') IS NOT NULL

    UNION ALL

    SELECT DISTINCT
        'status'::text AS filter_type,
        status::text AS value,
        status::text AS label
    FROM employee_rows
    WHERE NULLIF(status, '') IS NOT NULL

    UNION ALL

    SELECT DISTINCT
        'provider'::text AS filter_type,
        provider_type AS value,
        provider_type AS label
    FROM employee_rows
    WHERE NULLIF(provider_type, '') IS NOT NULL

    UNION ALL

    SELECT DISTINCT
        'risk_level'::text AS filter_type,
        risk_level::text AS value,
        risk_level::text AS label
    FROM employee_rows
    WHERE NULLIF(risk_level, '') IS NOT NULL

    UNION ALL

    SELECT DISTINCT
        'run_status'::text AS filter_type,
        run_status AS value,
        run_status AS label
    FROM employee_rows
    WHERE NULLIF(run_status, '') IS NOT NULL
) options
ORDER BY filter_type, label, value;

-- name: CountDigitalEmployeeOperationalSignals :many
-- Batch load per-employee load and reliability counts from recent project task
-- attempts. Used by the planning profile builder to score how busy and how reliable
-- each candidate digital employee is. Scoped to the last 30 days so the signal
-- reflects recent behaviour rather than lifetime totals. Employees with no recent
-- attempts do not produce a row; callers treat absence as zero.
SELECT
    pt.assigned_digital_employee_id AS digital_employee_id,
    COUNT(*) FILTER (WHERE pta.status IN ('queued', 'running'))::integer AS in_flight_attempt_count,
    COUNT(*) FILTER (WHERE pta.status = 'succeeded')::integer AS recent_success_count,
    COUNT(*) FILTER (WHERE pta.status IN ('failed', 'timed_out', 'lost'))::integer AS recent_failure_count,
    COUNT(*) FILTER (WHERE pta.status IN ('waiting_human', 'cancelled'))::integer AS recent_human_reject_count
FROM project_task_attempts pta
JOIN project_tasks pt
  ON pt.tenant_id = pta.tenant_id
 AND pt.id = pta.project_task_id
WHERE pta.tenant_id = sqlc.arg('tenant_id')
  AND pt.assigned_digital_employee_id IS NOT NULL
  AND pt.assigned_digital_employee_id = ANY(sqlc.arg('digital_employee_ids')::uuid[])
  AND pta.created_at >= NOW() - INTERVAL '30 days'
GROUP BY pt.tenant_id, pt.assigned_digital_employee_id;

-- name: ListDigitalEmployeeActivity :many
-- 跨员工运行动态流：task_events 按时间倒序，游标 (created_at, id) 支持增量拉取（since 之后的新事件）。
-- 事件类型到中文标签/状态的映射在 Go 层（employee.ActivityEventPresentation）统一处理。
SELECT
    te.id AS event_id,
    te.event_type,
    te.created_at AS occurred_at,
    tr.id AS run_id,
    tr.task_id,
    COALESCE(t.title, '')::text AS task_title,
    de.id AS digital_employee_id,
    de.name AS digital_employee_name,
    de.team_id,
    COALESCE(ptp.project_id, '00000000-0000-0000-0000-000000000000'::uuid) AS project_id,
    COALESCE(p.name, '')::text AS project_name
FROM task_events te
JOIN task_runs tr
  ON tr.id = te.run_id
 AND tr.tenant_id = te.tenant_id
JOIN digital_employees de
  ON de.id = tr.digital_employee_id
 AND de.tenant_id = tr.tenant_id
 AND de.deleted_at IS NULL
JOIN tasks t
  ON t.id = tr.task_id
 AND t.tenant_id = tr.tenant_id
 AND t.deleted_at IS NULL
LEFT JOIN LATERAL (
    SELECT project_tasks.project_id
    FROM project_tasks
    WHERE project_tasks.tenant_id = tr.tenant_id
      AND project_tasks.digital_employee_run_id = tr.id
    ORDER BY project_tasks.updated_at DESC
    LIMIT 1
) ptp ON TRUE
LEFT JOIN projects p
  ON p.id = ptp.project_id
 AND p.tenant_id = te.tenant_id
 AND p.archived_at IS NULL
WHERE te.tenant_id = sqlc.arg('tenant_id')::uuid
  AND te.run_id IS NOT NULL
  AND (
      sqlc.narg('since_created_at')::timestamptz IS NULL
      OR (te.created_at, te.id) > (
          sqlc.narg('since_created_at')::timestamptz,
          COALESCE(sqlc.narg('since_id')::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
      )
  )
ORDER BY te.created_at DESC, te.id DESC
LIMIT sqlc.arg('limit')::integer;
