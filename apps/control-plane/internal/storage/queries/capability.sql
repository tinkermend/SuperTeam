-- name: CreateUserCredential :one
INSERT INTO user_credentials (
    tenant_id,
    user_id,
    name,
    credential_type,
    encrypted_value,
    last_four,
    metadata
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('user_id')::uuid,
    sqlc.arg('name')::text,
    sqlc.arg('credential_type')::varchar,
    sqlc.arg('encrypted_value')::text,
    sqlc.arg('last_four')::varchar,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
)
RETURNING *;

-- name: ListUserCredentials :many
SELECT *
FROM user_credentials
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND user_id = sqlc.arg('user_id')::uuid
  AND deleted_at IS NULL
  AND (
    sqlc.narg('credential_type')::varchar IS NULL
    OR credential_type = sqlc.narg('credential_type')::varchar
  )
ORDER BY created_at DESC, name ASC;

-- name: GetUserCredential :one
SELECT *
FROM user_credentials
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND user_id = sqlc.arg('user_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL;

-- name: CreateTeamMCPServer :one
INSERT INTO team_mcp_servers (
    tenant_id,
    team_id,
    name,
    url,
    credential_id,
    metadata,
    created_by
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('team_id')::uuid,
    sqlc.arg('name')::text,
    sqlc.arg('url')::text,
    sqlc.narg('credential_id')::uuid,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb),
    sqlc.narg('created_by')::uuid
)
RETURNING *;

-- name: ListTeamMCPServers :many
SELECT
    tm.id,
    tm.tenant_id,
    tm.team_id,
    tm.name,
    tm.url,
    tm.credential_id,
    tm.status,
    tm.metadata,
    tm.disabled_at,
    tm.deleted_at,
    tm.created_by,
    tm.created_at,
    tm.updated_at,
    COALESCE(uc.name, '') AS credential_name,
    COALESCE(uc.credential_type, '') AS credential_type,
    COALESCE(uc.last_four, '') AS credential_last_four
FROM team_mcp_servers tm
LEFT JOIN user_credentials uc ON uc.tenant_id = tm.tenant_id
    AND uc.id = tm.credential_id
    AND uc.deleted_at IS NULL
WHERE tm.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tm.team_id = sqlc.arg('team_id')::uuid
  AND tm.deleted_at IS NULL
ORDER BY tm.created_at DESC, tm.name ASC;

-- name: DeleteTeamMCPServer :exec
UPDATE team_mcp_servers
SET deleted_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL;

-- name: CreateDigitalEmployeeMCPBinding :one
INSERT INTO digital_employee_mcp_bindings (
    tenant_id,
    digital_employee_id,
    name,
    url,
    credential_id,
    metadata,
    created_by
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('name')::text,
    sqlc.arg('url')::text,
    sqlc.narg('credential_id')::uuid,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb),
    sqlc.narg('created_by')::uuid
)
RETURNING *;

-- name: ListDigitalEmployeeMCPBindings :many
SELECT
    em.id,
    em.tenant_id,
    em.digital_employee_id,
    em.name,
    em.url,
    em.credential_id,
    em.status,
    em.metadata,
    em.disabled_at,
    em.deleted_at,
    em.created_by,
    em.created_at,
    em.updated_at,
    COALESCE(uc.name, '') AS credential_name,
    COALESCE(uc.credential_type, '') AS credential_type,
    COALESCE(uc.last_four, '') AS credential_last_four
FROM digital_employee_mcp_bindings em
LEFT JOIN user_credentials uc ON uc.tenant_id = em.tenant_id
    AND uc.id = em.credential_id
    AND uc.deleted_at IS NULL
WHERE em.tenant_id = sqlc.arg('tenant_id')::uuid
  AND em.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND em.deleted_at IS NULL
ORDER BY em.created_at DESC, em.name ASC;

-- name: DeleteDigitalEmployeeMCPBinding :exec
UPDATE digital_employee_mcp_bindings
SET deleted_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL;

-- name: ListEffectiveMCPServersForEmployee :many
WITH target_employee AS (
    SELECT tenant_id, id AS digital_employee_id, team_id
    FROM digital_employees
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND id = sqlc.arg('digital_employee_id')::uuid
      AND deleted_at IS NULL
)
SELECT
    tm.id,
    tm.tenant_id,
    target_employee.digital_employee_id,
    target_employee.team_id,
    tm.name,
    tm.url,
    tm.credential_id,
    tm.status,
    'team'::text AS source_scope,
    true AS inherited,
    COALESCE(uc.name, '') AS credential_name,
    COALESCE(uc.credential_type, '') AS credential_type,
    COALESCE(uc.last_four, '') AS credential_last_four,
    tm.created_at,
    tm.updated_at
FROM target_employee
JOIN team_mcp_servers tm ON tm.tenant_id = target_employee.tenant_id
    AND tm.team_id = target_employee.team_id
    AND tm.deleted_at IS NULL
LEFT JOIN user_credentials uc ON uc.tenant_id = tm.tenant_id
    AND uc.id = tm.credential_id
    AND uc.deleted_at IS NULL
UNION ALL
SELECT
    em.id,
    em.tenant_id,
    em.digital_employee_id,
    NULL::uuid AS team_id,
    em.name,
    em.url,
    em.credential_id,
    em.status,
    'employee'::text AS source_scope,
    false AS inherited,
    COALESCE(uc.name, '') AS credential_name,
    COALESCE(uc.credential_type, '') AS credential_type,
    COALESCE(uc.last_four, '') AS credential_last_four,
    em.created_at,
    em.updated_at
FROM target_employee
JOIN digital_employee_mcp_bindings em ON em.tenant_id = target_employee.tenant_id
    AND em.digital_employee_id = target_employee.digital_employee_id
    AND em.deleted_at IS NULL
LEFT JOIN user_credentials uc ON uc.tenant_id = em.tenant_id
    AND uc.id = em.credential_id
    AND uc.deleted_at IS NULL
WHERE NOT EXISTS (
    SELECT 1
    FROM team_mcp_servers team_duplicate
    WHERE team_duplicate.tenant_id = target_employee.tenant_id
      AND team_duplicate.team_id = target_employee.team_id
      AND team_duplicate.name = em.name
      AND team_duplicate.url = em.url
      AND team_duplicate.deleted_at IS NULL
)
ORDER BY inherited DESC, name ASC;

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
    m.risk_level,
    m.status AS server_status
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
    m.risk_level,
    m.status AS server_status
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
    'team'::text AS source_scope,
    tb.status AS binding_status
FROM target_employee
JOIN team_mcp_bindings tb ON tb.tenant_id = target_employee.tenant_id
    AND tb.team_id = target_employee.team_id
    AND tb.deleted_at IS NULL
    AND tb.status = 'active'
JOIN mcp_servers m ON m.id = tb.mcp_server_id
    AND m.tenant_id = tb.tenant_id
    AND m.deleted_at IS NULL
    AND m.status = 'active'
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
    'employee'::text AS source_scope,
    eb.status AS binding_status
FROM target_employee
JOIN digital_employee_mcp_bindings_v2 eb ON eb.tenant_id = target_employee.tenant_id
    AND eb.digital_employee_id = target_employee.digital_employee_id
    AND eb.deleted_at IS NULL
    AND eb.status = 'active'
JOIN mcp_servers m ON m.id = eb.mcp_server_id
    AND m.tenant_id = eb.tenant_id
    AND m.deleted_at IS NULL
    AND m.status = 'active'
WHERE NOT EXISTS (
    SELECT 1
    FROM team_mcp_bindings team_duplicate
    WHERE team_duplicate.tenant_id = target_employee.tenant_id
      AND team_duplicate.team_id = target_employee.team_id
      AND team_duplicate.mcp_server_id = eb.mcp_server_id
      AND team_duplicate.deleted_at IS NULL
      AND team_duplicate.status = 'active'
)
ORDER BY source_scope ASC, name ASC;

-- 项目级 MCP 绑定（迁移 072，目录与能力投影修订 spec §3.2）。
-- 项目绑定是声明式全量替换（PUT 语义）：先软删项目下全部活跃绑定，再逐条插入
-- 期望集合；部分失败向"更少能力"收敛（fail-closed），不会静默多授权。

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
    m.risk_level,
    m.status AS server_status
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
-- 项目绑定的运行时投影行：只取活跃绑定 × 活跃注册表定义。缺失 env 判定由调用方
-- 用目标员工的已配置 env 集合完成（与员工侧投影同一套过滤逻辑），凭据值不经此路。
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
    'project'::text AS source_scope,
    pb.status AS binding_status
FROM project_mcp_bindings pb
JOIN mcp_servers m ON m.id = pb.mcp_server_id
    AND m.tenant_id = pb.tenant_id
    AND m.deleted_at IS NULL
    AND m.status = 'active'
WHERE pb.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pb.project_id = sqlc.arg('project_id')::uuid
  AND pb.deleted_at IS NULL
  AND pb.status = 'active'
ORDER BY m.name ASC;
