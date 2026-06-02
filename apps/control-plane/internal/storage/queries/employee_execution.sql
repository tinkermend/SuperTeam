-- name: CreateDigitalEmployee :one
INSERT INTO digital_employees (
    tenant_id,
    team_id,
    name,
    role,
    description,
    status,
    permission_policy,
    context_policy,
    approval_policy,
    risk_level,
    metadata
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.narg('team_id')::uuid,
    sqlc.arg('name')::varchar,
    sqlc.arg('role')::varchar,
    sqlc.narg('description')::text,
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.arg('permission_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('context_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('approval_policy')::jsonb, '{}'::jsonb),
    sqlc.arg('risk_level')::varchar,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
) RETURNING *;

-- name: GetDigitalEmployee :one
SELECT *
FROM digital_employees
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL;

-- name: ListDigitalEmployees :many
SELECT *
FROM digital_employees
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
  AND (sqlc.narg('team_id')::uuid IS NULL OR team_id = sqlc.narg('team_id')::uuid)
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

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

-- name: DeleteDigitalEmployee :exec
UPDATE digital_employees
SET deleted_at = COALESCE(deleted_at, NOW()),
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid;

-- name: CreateDigitalEmployeeExecutionInstance :one
INSERT INTO digital_employee_execution_instances (
    tenant_id,
    digital_employee_id,
    runtime_node_id,
    provider_type,
    agent_home_dir,
    workspace_policy,
    session_policy,
    runtime_selector,
    capacity_requirements,
    fallback_policy,
    status,
    metadata
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('runtime_node_id')::uuid,
    sqlc.arg('provider_type')::varchar,
    sqlc.arg('agent_home_dir')::text,
    COALESCE(sqlc.arg('workspace_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('session_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('runtime_selector')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('capacity_requirements')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('fallback_policy')::jsonb, '{}'::jsonb),
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
) RETURNING *;

-- name: UpsertDigitalEmployeeExecutionInstance :one
INSERT INTO digital_employee_execution_instances (
    tenant_id,
    digital_employee_id,
    runtime_node_id,
    provider_type,
    agent_home_dir,
    workspace_policy,
    session_policy,
    runtime_selector,
    capacity_requirements,
    fallback_policy,
    status,
    metadata
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('runtime_node_id')::uuid,
    sqlc.arg('provider_type')::varchar,
    sqlc.arg('agent_home_dir')::text,
    COALESCE(sqlc.arg('workspace_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('session_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('runtime_selector')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('capacity_requirements')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('fallback_policy')::jsonb, '{}'::jsonb),
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
)
ON CONFLICT (tenant_id, digital_employee_id) WHERE deleted_at IS NULL DO UPDATE SET
    runtime_node_id = EXCLUDED.runtime_node_id,
    provider_type = EXCLUDED.provider_type,
    agent_home_dir = EXCLUDED.agent_home_dir,
    workspace_policy = EXCLUDED.workspace_policy,
    session_policy = EXCLUDED.session_policy,
    runtime_selector = EXCLUDED.runtime_selector,
    capacity_requirements = EXCLUDED.capacity_requirements,
    fallback_policy = EXCLUDED.fallback_policy,
    status = EXCLUDED.status,
    metadata = EXCLUDED.metadata,
    updated_at = NOW()
RETURNING *;

-- name: GetDigitalEmployeeExecutionInstance :one
SELECT *
FROM digital_employee_execution_instances
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL;

-- name: GetDigitalEmployeeExecutionInstanceByEmployeeID :one
SELECT *
FROM digital_employee_execution_instances
WHERE digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL;

-- name: ListDigitalEmployeeExecutionInstances :many
SELECT *
FROM digital_employee_execution_instances
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
  AND (sqlc.narg('runtime_node_id')::uuid IS NULL OR runtime_node_id = sqlc.narg('runtime_node_id')::uuid)
  AND (sqlc.narg('provider_type')::varchar IS NULL OR provider_type = sqlc.narg('provider_type')::varchar)
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: UpdateDigitalEmployeeExecutionInstanceStatus :one
UPDATE digital_employee_execution_instances
SET status = sqlc.arg('status')::varchar,
    ready_at = CASE
        WHEN sqlc.arg('status')::varchar = 'ready' THEN COALESCE(ready_at, NOW())
        ELSE ready_at
    END,
    disabled_at = CASE
        WHEN sqlc.arg('status')::varchar = 'disabled' THEN COALESCE(disabled_at, NOW())
        WHEN sqlc.arg('status')::varchar IN ('provisioning', 'ready', 'active') THEN NULL
        ELSE disabled_at
    END,
    error_at = CASE
        WHEN sqlc.arg('status')::varchar = 'error' THEN COALESCE(error_at, NOW())
        WHEN sqlc.arg('status')::varchar IN ('provisioning', 'ready', 'active') THEN NULL
        ELSE error_at
    END,
    error_message = CASE
        WHEN sqlc.arg('status')::varchar = 'error' THEN COALESCE(sqlc.narg('error_message')::text, error_message)
        WHEN sqlc.arg('status')::varchar IN ('provisioning', 'ready', 'active') THEN NULL
        ELSE error_message
    END,
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
RETURNING *;

-- name: DeleteDigitalEmployeeExecutionInstance :exec
UPDATE digital_employee_execution_instances
SET deleted_at = COALESCE(deleted_at, NOW()),
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid;
