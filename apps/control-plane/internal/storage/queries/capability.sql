-- ============================================================================
-- MCP HTTP capability registry (migration 037)
-- ============================================================================

-- name: CreateMCPServerDefinition :one
INSERT INTO mcp_servers (
    tenant_id,
    name,
    server_key,
    description,
    transport,
    url,
    auth_strategy,
    required_env_vars,
    optional_env_vars,
    provider_visibility,
    tool_allowlist,
    risk_level,
    metadata,
    created_by
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('name')::text,
    sqlc.arg('server_key')::text,
    COALESCE(sqlc.arg('description')::text, ''),
    sqlc.arg('transport')::varchar,
    sqlc.arg('url')::text,
    sqlc.arg('auth_strategy')::varchar,
    COALESCE(sqlc.arg('required_env_vars')::text[], ARRAY[]::TEXT[]),
    COALESCE(sqlc.arg('optional_env_vars')::text[], ARRAY[]::TEXT[]),
    COALESCE(sqlc.arg('provider_visibility')::jsonb, '{"codex":true,"claude-code":true,"opencode":true}'::jsonb),
    COALESCE(sqlc.arg('tool_allowlist')::text[], ARRAY[]::TEXT[]),
    COALESCE(sqlc.arg('risk_level')::varchar, 'medium'),
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb),
    sqlc.narg('created_by')::uuid
)
RETURNING *;

-- name: ListMCPServerDefinitions :many
SELECT *
FROM mcp_servers
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
ORDER BY created_at DESC, name ASC;

-- name: GetMCPServerDefinition :one
SELECT *
FROM mcp_servers
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL;

-- name: DeleteMCPServerDefinition :exec
UPDATE mcp_servers
SET deleted_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL;

-- name: CreateTeamMCPBinding :one
INSERT INTO team_mcp_bindings (
    tenant_id,
    team_id,
    mcp_server_id,
    credential_env_var,
    metadata,
    created_by
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('team_id')::uuid,
    sqlc.arg('mcp_server_id')::uuid,
    sqlc.narg('credential_env_var')::text,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb),
    sqlc.narg('created_by')::uuid
)
RETURNING *;

-- name: ListTeamMCPBindings :many
SELECT
    tb.*,
    m.name AS server_name,
    m.server_key,
    m.url,
    m.transport,
    m.auth_strategy,
    m.required_env_vars,
    m.risk_level
FROM team_mcp_bindings tb
JOIN mcp_servers m ON m.id = tb.mcp_server_id
    AND m.tenant_id = tb.tenant_id
    AND m.deleted_at IS NULL
WHERE tb.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tb.team_id = sqlc.arg('team_id')::uuid
  AND tb.deleted_at IS NULL
ORDER BY tb.created_at DESC;

-- name: DeleteTeamMCPBinding :exec
UPDATE team_mcp_bindings
SET deleted_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL;

-- name: CreateEmployeeMCPBindingV2 :one
INSERT INTO digital_employee_mcp_bindings_v2 (
    tenant_id,
    digital_employee_id,
    mcp_server_id,
    credential_env_var,
    metadata,
    created_by
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('mcp_server_id')::uuid,
    sqlc.narg('credential_env_var')::text,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb),
    sqlc.narg('created_by')::uuid
)
RETURNING *;

-- name: ListEmployeeMCPBindingsV2 :many
SELECT
    eb.*,
    m.name AS server_name,
    m.server_key,
    m.url,
    m.transport,
    m.auth_strategy,
    m.required_env_vars,
    m.risk_level
FROM digital_employee_mcp_bindings_v2 eb
JOIN mcp_servers m ON m.id = eb.mcp_server_id
    AND m.tenant_id = eb.tenant_id
    AND m.deleted_at IS NULL
WHERE eb.tenant_id = sqlc.arg('tenant_id')::uuid
  AND eb.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND eb.deleted_at IS NULL
ORDER BY eb.created_at DESC;

-- name: DeleteEmployeeMCPBindingV2 :exec
UPDATE digital_employee_mcp_bindings_v2
SET deleted_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL;

-- name: ListConfiguredEmployeeEnvVarNames :many
SELECT name
FROM digital_employee_environment_variables
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND status = 'active'
  AND deleted_at IS NULL;

-- name: ListEffectiveMCPBindingsV2ForEmployee :many
-- Effective MCP bindings for an employee: team-inherited plus personal, joined to the
-- registry definition. The caller computes missing required env vars by intersecting
-- required_env_vars with ListConfiguredEmployeeEnvVarNames. credential values are never
-- returned here.
WITH target_employee AS (
    SELECT tenant_id, id AS digital_employee_id, team_id
    FROM digital_employees
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND id = sqlc.arg('digital_employee_id')::uuid
      AND deleted_at IS NULL
)
SELECT
    m.id AS server_id,
    m.tenant_id,
    target_employee.digital_employee_id,
    m.name,
    m.server_key,
    m.transport,
    m.url,
    m.auth_strategy,
    m.required_env_vars,
    m.tool_allowlist,
    m.risk_level,
    tb.credential_env_var,
    'team'::text AS source_scope
FROM target_employee
JOIN team_mcp_bindings tb ON tb.tenant_id = target_employee.tenant_id
    AND tb.team_id = target_employee.team_id
    AND tb.deleted_at IS NULL
JOIN mcp_servers m ON m.id = tb.mcp_server_id
    AND m.tenant_id = tb.tenant_id
    AND m.deleted_at IS NULL
UNION ALL
SELECT
    m.id AS server_id,
    m.tenant_id,
    target_employee.digital_employee_id,
    m.name,
    m.server_key,
    m.transport,
    m.url,
    m.auth_strategy,
    m.required_env_vars,
    m.tool_allowlist,
    m.risk_level,
    eb.credential_env_var,
    'employee'::text AS source_scope
FROM target_employee
JOIN digital_employee_mcp_bindings_v2 eb ON eb.tenant_id = target_employee.tenant_id
    AND eb.digital_employee_id = target_employee.digital_employee_id
    AND eb.deleted_at IS NULL
JOIN mcp_servers m ON m.id = eb.mcp_server_id
    AND m.tenant_id = eb.tenant_id
    AND m.deleted_at IS NULL
WHERE NOT EXISTS (
    SELECT 1
    FROM team_mcp_bindings team_duplicate
    WHERE team_duplicate.tenant_id = target_employee.tenant_id
      AND team_duplicate.team_id = target_employee.team_id
      AND team_duplicate.mcp_server_id = eb.mcp_server_id
      AND team_duplicate.deleted_at IS NULL
)
ORDER BY source_scope ASC, name ASC;


-- name: ListTeamMCPTakeoverTargets :many
-- 团队绑定某 MCP 前/时的接管清单：本团队成员里已自行绑定同一 MCP 的个人绑定。
-- 预览与执行共用这一条，保证"看到的"和"接管的"是同一批。
SELECT
    de.id AS digital_employee_id,
    de.name AS employee_name,
    eb.id AS binding_id,
    COALESCE(eb.credential_env_var, '')::text AS credential_env_var
FROM digital_employees de
JOIN digital_employee_mcp_bindings_v2 eb
  ON eb.tenant_id = de.tenant_id
 AND eb.digital_employee_id = de.id
 AND eb.deleted_at IS NULL
WHERE de.tenant_id = sqlc.arg('tenant_id')::uuid
  AND de.team_id = sqlc.arg('team_id')::uuid
  AND de.deleted_at IS NULL
  AND eb.mcp_server_id = sqlc.arg('mcp_server_id')::uuid
ORDER BY de.name ASC;

-- name: TakeoverTeamMCPBindings :exec
-- 团队接管：软删本团队成员的同 MCP 个人绑定。团队基线胜出，同一能力只留一份。
UPDATE digital_employee_mcp_bindings_v2 eb
SET deleted_at = NOW(),
    updated_at = NOW()
FROM digital_employees de
WHERE de.id = eb.digital_employee_id
  AND de.tenant_id = eb.tenant_id
  AND de.team_id = sqlc.arg('team_id')::uuid
  AND de.deleted_at IS NULL
  AND eb.tenant_id = sqlc.arg('tenant_id')::uuid
  AND eb.mcp_server_id = sqlc.arg('mcp_server_id')::uuid
  AND eb.deleted_at IS NULL;

-- name: TeamProvidesMCPServer :one
-- 员工侧绑定前的冲突判据：该员工所属团队是否已经提供同一个 MCP。
SELECT EXISTS(
    SELECT 1
    FROM digital_employees de
    JOIN team_mcp_bindings tmb
      ON tmb.tenant_id = de.tenant_id
     AND tmb.team_id = de.team_id
     AND tmb.deleted_at IS NULL
    WHERE de.tenant_id = sqlc.arg('tenant_id')::uuid
      AND de.id = sqlc.arg('digital_employee_id')::uuid
      AND de.deleted_at IS NULL
      AND tmb.mcp_server_id = sqlc.arg('mcp_server_id')::uuid
) AS team_provides;

-- name: ListTeamMCPReadiness :many
-- 团队 MCP 就绪矩阵：本团队每个 MCP 绑定 × 每名数字员工，算出该员工还缺哪些必需
-- 环境变量。变量名由绑定/注册表定义，值只存在员工级，所以就绪与否天然是逐员工的。
WITH team_employees AS (
    SELECT id AS digital_employee_id, name
    FROM digital_employees
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND team_id = sqlc.arg('team_id')::uuid
      AND deleted_at IS NULL
      AND archived_at IS NULL
),
team_mcps AS (
    SELECT m.id AS mcp_server_id, m.server_key, m.name, m.required_env_vars
    FROM team_mcp_bindings tmb
    JOIN mcp_servers m
      ON m.id = tmb.mcp_server_id
     AND m.tenant_id = tmb.tenant_id
     AND m.deleted_at IS NULL
    WHERE tmb.tenant_id = sqlc.arg('tenant_id')::uuid
      AND tmb.team_id = sqlc.arg('team_id')::uuid
      AND tmb.deleted_at IS NULL
)
SELECT
    tm.mcp_server_id,
    tm.server_key,
    tm.name AS server_name,
    tm.required_env_vars,
    te.digital_employee_id,
    te.name AS employee_name,
    ARRAY(
        SELECT required
        FROM unnest(tm.required_env_vars) AS required
        WHERE NOT EXISTS (
            SELECT 1
            FROM digital_employee_environment_variables ev
            WHERE ev.tenant_id = sqlc.arg('tenant_id')::uuid
              AND ev.digital_employee_id = te.digital_employee_id
              AND ev.name = required
              AND ev.status = 'active'
              AND ev.deleted_at IS NULL
        )
    )::text[] AS missing_env_vars
FROM team_mcps tm
CROSS JOIN team_employees te
ORDER BY tm.server_key ASC, te.name ASC;

-- name: CreateProjectMCPBinding :one
INSERT INTO project_mcp_bindings (
    tenant_id,
    project_id,
    mcp_server_id,
    credential_env_var,
    metadata,
    created_by
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('mcp_server_id')::uuid,
    sqlc.narg('credential_env_var')::text,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb),
    sqlc.narg('created_by')::uuid
)
RETURNING *;

-- name: ListProjectMCPBindings :many
SELECT
    pb.*,
    m.name AS server_name,
    m.server_key,
    m.url,
    m.transport,
    m.auth_strategy,
    m.required_env_vars,
    m.risk_level
FROM project_mcp_bindings pb
JOIN mcp_servers m ON m.id = pb.mcp_server_id
    AND m.tenant_id = pb.tenant_id
    AND m.deleted_at IS NULL
WHERE pb.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pb.project_id = sqlc.arg('project_id')::uuid
  AND pb.deleted_at IS NULL
ORDER BY pb.created_at DESC;

-- name: SoftDeleteProjectMCPBindingsForProject :exec
UPDATE project_mcp_bindings
SET deleted_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND deleted_at IS NULL;

-- name: ListEffectiveProjectMCPBindingsForRuntime :many
-- 项目绑定的运行时投影行：只取未删除绑定 × 未删除注册表定义。缺失 env 判定由调用方
-- 用目标员工的已配置 env 集合完成，凭据值不经此路。
SELECT
    m.id AS server_id,
    m.tenant_id,
    m.name,
    m.server_key,
    m.transport,
    m.url,
    m.auth_strategy,
    m.required_env_vars,
    m.tool_allowlist,
    m.risk_level,
    pb.credential_env_var,
    'project'::text AS source_scope
FROM project_mcp_bindings pb
JOIN mcp_servers m ON m.id = pb.mcp_server_id
    AND m.tenant_id = pb.tenant_id
    AND m.deleted_at IS NULL
WHERE pb.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pb.project_id = sqlc.arg('project_id')::uuid
  AND pb.deleted_at IS NULL
ORDER BY m.name ASC;


-- name: ListMCPServerProjectBindings :many
SELECT
    pb.mcp_server_id,
    pb.project_id,
    COALESCE(p.name, '')::text AS project_name
FROM project_mcp_bindings pb
LEFT JOIN projects p ON p.id = pb.project_id
    AND p.tenant_id = pb.tenant_id
    AND p.deleted_at IS NULL
WHERE pb.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pb.deleted_at IS NULL
  AND pb.mcp_server_id = ANY(sqlc.arg('mcp_server_ids')::uuid[])
ORDER BY project_name ASC, pb.project_id ASC;
