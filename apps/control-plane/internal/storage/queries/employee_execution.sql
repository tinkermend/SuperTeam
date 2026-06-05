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
) SELECT
    de.tenant_id,
    de.id,
    rn.id,
    sqlc.arg('provider_type')::varchar,
    sqlc.arg('agent_home_dir')::text,
    COALESCE(sqlc.arg('workspace_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('session_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('runtime_selector')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('capacity_requirements')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('fallback_policy')::jsonb, '{}'::jsonb),
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
FROM digital_employees de
JOIN runtime_nodes rn
  ON rn.id = sqlc.arg('runtime_node_id')::uuid
 AND rn.tenant_id = de.tenant_id
 AND rn.status = 'online'
 AND rn.disabled_at IS NULL
 AND rn.archived_at IS NULL
WHERE de.id = sqlc.arg('digital_employee_id')::uuid
  AND de.tenant_id = sqlc.arg('tenant_id')::uuid
  AND de.status NOT IN ('disabled', 'error')
  AND de.deleted_at IS NULL
  AND de.archived_at IS NULL
  AND EXISTS (
      SELECT 1
      FROM runtime_enrollments re
      WHERE re.tenant_id = de.tenant_id
        AND re.runtime_node_id = rn.id
        AND re.status = 'approved'
  )
  AND EXISTS (
      SELECT 1
      FROM runtime_capabilities rc
      WHERE rc.tenant_id = de.tenant_id
        AND rc.runtime_node_id = rn.id
        AND rc.capability_type = 'provider'
        AND rc.provider_type = sqlc.arg('provider_type')::varchar
        AND rc.available = true
        AND rc.status = 'healthy'
        AND rc.health_status = 'healthy'
        AND rc.disabled_at IS NULL
        AND rc.archived_at IS NULL
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

-- name: GetRuntimeProvisioningPreflight :one
WITH active_team_config AS (
    SELECT *
    FROM tenant_team_config_revisions
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND team_id = sqlc.arg('team_id')::uuid
      AND status = 'active'
      AND archived_at IS NULL
    ORDER BY revision_number DESC
    LIMIT 1
),
provider_capability AS (
    SELECT *
    FROM runtime_capabilities
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND runtime_node_id = sqlc.arg('runtime_node_id')::uuid
      AND capability_type = 'provider'
      AND provider_type = sqlc.arg('provider_type')::varchar
      AND available = true
      AND status = 'healthy'
      AND health_status = 'healthy'
      AND disabled_at IS NULL
      AND archived_at IS NULL
    ORDER BY last_seen_at DESC NULLS LAST, updated_at DESC
    LIMIT 1
)
SELECT
    tt.tenant_id,
    tt.id AS team_id,
    rn.id AS runtime_node_id,
    rn.node_id,
    COALESCE(
        provider_capability.details ->> 'agent_home_dir',
        provider_capability.metadata ->> 'agent_home_dir',
        provider_capability.workspace_base_dir,
        rn.metadata ->> 'agent_home_dir',
        ''
    )::text AS agent_home_dir,
    COALESCE(
        jsonb_build_object(
            'team_config_revision_id', active_team_config.id,
            'revision_number', active_team_config.revision_number,
            'constitution', active_team_config.constitution,
            'capability_policy', active_team_config.capability_policy,
            'context_policy', active_team_config.context_policy,
            'approval_policy', active_team_config.approval_policy,
            'artifact_contract', active_team_config.artifact_contract,
            'internal_collaboration_policy', active_team_config.internal_collaboration_policy,
            'runtime_scope_policy', active_team_config.runtime_scope_policy,
            'approved_by', active_team_config.approved_by,
            'approved_at', active_team_config.approved_at
        ),
        '{}'::jsonb
    ) AS governance_snapshot,
    (active_team_config.id IS NOT NULL)::boolean AS has_active_team_config,
    (
        rn.status = 'online'
        AND rn.disabled_at IS NULL
        AND rn.archived_at IS NULL
    )::boolean AS runtime_online,
    EXISTS (
        SELECT 1
        FROM runtime_enrollments re
        WHERE re.tenant_id = tt.tenant_id
          AND re.runtime_node_id = rn.id
          AND re.status = 'approved'
          AND re.rejected_at IS NULL
          AND re.revoked_at IS NULL
    )::boolean AS enrollment_approved,
    EXISTS (
        SELECT 1
        FROM runtime_sessions rs
        JOIN runtime_enrollments re
          ON re.id = rs.enrollment_id
         AND re.tenant_id = rs.tenant_id
         AND re.runtime_node_id = rs.runtime_node_id
         AND re.status = 'approved'
         AND re.rejected_at IS NULL
         AND re.revoked_at IS NULL
        WHERE rs.tenant_id = tt.tenant_id
          AND rs.runtime_node_id = rn.id
          AND rs.expires_at > NOW()
          AND rs.revoked_at IS NULL
    )::boolean AS runtime_session_active,
    (provider_capability.id IS NOT NULL)::boolean AS provider_available,
    (
        active_team_config.id IS NOT NULL
        AND jsonb_typeof(active_team_config.capability_policy -> 'allowed_provider_types') = 'array'
        AND (active_team_config.capability_policy -> 'allowed_provider_types') ? sqlc.arg('provider_type')::varchar
    )::boolean AS provider_policy_allowed,
    (
        active_team_config.id IS NOT NULL
        AND (
            (
                active_team_config.runtime_scope_policy ? 'allowed_runtime_node_ids'
                AND jsonb_typeof(active_team_config.runtime_scope_policy -> 'allowed_runtime_node_ids') = 'array'
                AND (active_team_config.runtime_scope_policy -> 'allowed_runtime_node_ids') ? rn.id::text
            )
            OR (
                active_team_config.runtime_scope_policy ? 'allowed_node_ids'
                AND jsonb_typeof(active_team_config.runtime_scope_policy -> 'allowed_node_ids') = 'array'
                AND (active_team_config.runtime_scope_policy -> 'allowed_node_ids') ? rn.node_id
            )
        )
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
    )::boolean AS runtime_policy_allowed
FROM tenant_teams tt
JOIN runtime_nodes rn
  ON rn.id = sqlc.arg('runtime_node_id')::uuid
 AND rn.tenant_id = tt.tenant_id
LEFT JOIN active_team_config ON TRUE
LEFT JOIN provider_capability ON TRUE
WHERE tt.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tt.id = sqlc.arg('team_id')::uuid
  AND tt.status = 'active'
  AND tt.disabled_at IS NULL
  AND tt.archived_at IS NULL
  AND tt.deleted_at IS NULL;

-- name: GetDigitalEmployeeRunPreflight :one
SELECT
    de.tenant_id,
    de.team_id,
    de.id AS digital_employee_id,
    de.status AS digital_employee_status,
    dei.id AS execution_instance_id,
    dei.status AS execution_status,
    dei.runtime_node_id,
    rn.node_id,
    dei.provider_type,
    dei.agent_home_dir,
    dei.runtime_selector,
    dei.session_policy,
    dei.workspace_policy,
    EXISTS (
        SELECT 1
        FROM digital_employee_effective_configs dec
        WHERE dec.tenant_id = de.tenant_id
          AND dec.digital_employee_id = de.id
          AND dec.status = 'approved'
          AND dec.revoked_at IS NULL
    ) AS has_approved_effective_config,
    EXISTS (
        SELECT 1
        FROM runtime_capabilities rc
        WHERE rc.tenant_id = de.tenant_id
          AND rc.runtime_node_id = dei.runtime_node_id
          AND rc.capability_type = 'provider'
          AND rc.provider_type = dei.provider_type
          AND rc.available = true
          AND rc.status = 'healthy'
          AND rc.health_status = 'healthy'
          AND rc.disabled_at IS NULL
          AND rc.archived_at IS NULL
    ) AS provider_healthy
FROM digital_employees de
JOIN digital_employee_execution_instances dei
  ON dei.digital_employee_id = de.id
 AND dei.tenant_id = de.tenant_id
 AND dei.deleted_at IS NULL
JOIN runtime_nodes rn
  ON rn.id = dei.runtime_node_id
 AND rn.tenant_id = de.tenant_id
 AND rn.archived_at IS NULL
WHERE de.id = sqlc.arg('digital_employee_id')::uuid
  AND de.tenant_id = sqlc.arg('tenant_id')::uuid
  AND de.deleted_at IS NULL
  AND de.archived_at IS NULL;

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

-- name: AbortProvisionedDigitalEmployee :exec
WITH aborted_instance AS (
    UPDATE digital_employee_execution_instances
    SET status = 'error',
        error_at = COALESCE(error_at, NOW()),
        error_message = sqlc.arg('reason')::text,
        deleted_at = COALESCE(deleted_at, NOW()),
        updated_at = NOW()
    WHERE id = sqlc.arg('execution_instance_id')::uuid
      AND tenant_id = sqlc.arg('tenant_id')::uuid
      AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
    RETURNING id
),
aborted_employee AS (
    UPDATE digital_employees
    SET status = 'error',
        deleted_at = COALESCE(deleted_at, NOW()),
        updated_at = NOW()
    WHERE id = sqlc.arg('digital_employee_id')::uuid
      AND tenant_id = sqlc.arg('tenant_id')::uuid
    RETURNING id
),
aborted_receipts AS (
    UPDATE runtime_command_receipts
    SET status = CASE
            WHEN status IN ('completed', 'failed', 'cancelled', 'timed_out') THEN status
            ELSE 'failed'
        END,
        error_message = COALESCE(error_message, sqlc.arg('reason')::text),
        completed_at = CASE
            WHEN status IN ('completed', 'failed', 'cancelled', 'timed_out') THEN completed_at
            ELSE COALESCE(completed_at, NOW())
        END,
        updated_at = NOW()
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND resource_type = 'digital_employee_execution_instance'
      AND resource_id = sqlc.arg('execution_instance_id')::uuid
    RETURNING id
)
SELECT 1;
