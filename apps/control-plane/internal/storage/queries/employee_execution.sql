-- name: CreateDigitalEmployee :one
INSERT INTO digital_employees (
    tenant_id,
    team_id,
    owner_user_id,
    employee_type,
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
    sqlc.arg('owner_user_id')::uuid,
    sqlc.arg('employee_type')::varchar,
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

-- name: ListRuntimeProviderOptionsForDigitalEmployeeCreate :many
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
      AND disabled_at IS NULL
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
        AND COALESCE((
            (
                jsonb_typeof(active_team_config.capability_policy -> 'allowed_provider_types') = 'array'
                AND (active_team_config.capability_policy -> 'allowed_provider_types') ? pc.provider_type
            )
            OR (
                jsonb_typeof(active_team_config.runtime_scope_policy -> 'allowed_provider_types') = 'array'
                AND (active_team_config.runtime_scope_policy -> 'allowed_provider_types') ? pc.provider_type
            )
            OR (
                jsonb_typeof(active_team_config.runtime_scope_policy -> 'provider_types') = 'array'
                AND (active_team_config.runtime_scope_policy -> 'provider_types') ? pc.provider_type
            )
        ), false)
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
        WHEN active_team_config.id IS NULL THEN 'active_team_config_required'
        WHEN rn.status <> 'online' OR rn.disabled_at IS NOT NULL OR rn.archived_at IS NOT NULL THEN 'runtime_not_online'
        WHEN runtime_sessions_active.runtime_node_id IS NULL THEN 'runtime_session_inactive'
        WHEN pc.available = false OR pc.status <> 'healthy' OR pc.health_status <> 'healthy' THEN 'provider_unhealthy'
        WHEN COALESCE(pc.provider_type, '') = '' THEN 'provider_type_missing'
        WHEN NOT COALESCE((
            (
                jsonb_typeof(active_team_config.capability_policy -> 'allowed_provider_types') = 'array'
                AND (active_team_config.capability_policy -> 'allowed_provider_types') ? pc.provider_type
            )
            OR (
                jsonb_typeof(active_team_config.runtime_scope_policy -> 'allowed_provider_types') = 'array'
                AND (active_team_config.runtime_scope_policy -> 'allowed_provider_types') ? pc.provider_type
            )
            OR (
                jsonb_typeof(active_team_config.runtime_scope_policy -> 'provider_types') = 'array'
                AND (active_team_config.runtime_scope_policy -> 'provider_types') ? pc.provider_type
            )
        ), false) THEN 'provider_outside_team_policy'
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
),
workspace_capability AS (
    SELECT *
    FROM runtime_capabilities
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND runtime_node_id = sqlc.arg('runtime_node_id')::uuid
      AND capability_type = 'workspace'
      AND capability_key = 'base-dir'
      AND available = true
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
        workspace_capability.details ->> 'agent_home_dir',
        workspace_capability.metadata ->> 'agent_home_dir',
        workspace_capability.workspace_base_dir,
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
    COALESCE((
        active_team_config.id IS NOT NULL
        AND (
            (
                jsonb_typeof(active_team_config.capability_policy -> 'allowed_provider_types') = 'array'
                AND (active_team_config.capability_policy -> 'allowed_provider_types') ? sqlc.arg('provider_type')::varchar
            )
            OR (
                jsonb_typeof(active_team_config.runtime_scope_policy -> 'allowed_provider_types') = 'array'
                AND (active_team_config.runtime_scope_policy -> 'allowed_provider_types') ? sqlc.arg('provider_type')::varchar
            )
            OR (
                jsonb_typeof(active_team_config.runtime_scope_policy -> 'provider_types') = 'array'
                AND (active_team_config.runtime_scope_policy -> 'provider_types') ? sqlc.arg('provider_type')::varchar
            )
        )
    ), false)::boolean AS provider_policy_allowed,
    COALESCE((
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
            OR (
                jsonb_typeof(active_team_config.runtime_scope_policy -> 'allowed_provider_types') = 'array'
                AND (active_team_config.runtime_scope_policy -> 'allowed_provider_types') ? sqlc.arg('provider_type')::varchar
            )
            OR (
                jsonb_typeof(active_team_config.runtime_scope_policy -> 'provider_types') = 'array'
                AND (active_team_config.runtime_scope_policy -> 'provider_types') ? sqlc.arg('provider_type')::varchar
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
    ), false)::boolean AS runtime_policy_allowed
FROM tenant_teams tt
JOIN runtime_nodes rn
  ON rn.id = sqlc.arg('runtime_node_id')::uuid
 AND rn.tenant_id = tt.tenant_id
LEFT JOIN active_team_config ON TRUE
LEFT JOIN provider_capability ON TRUE
LEFT JOIN workspace_capability ON TRUE
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
    COALESCE(ec.effective_config_snapshot -> 'budget_policy', '{}'::jsonb)::jsonb AS budget_policy,
    COALESCE(today_usage.usage_tokens_today, 0)::integer AS today_token_usage,
    'Asia/Shanghai'::text AS business_timezone,
    (ec.effective_config_id IS NOT NULL)::boolean AS has_approved_effective_config,
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
LEFT JOIN LATERAL (
    SELECT
        dec.id AS effective_config_id,
        dec.effective_config_snapshot
    FROM digital_employee_effective_configs dec
    WHERE dec.tenant_id = de.tenant_id
      AND dec.digital_employee_id = de.id
      AND dec.status = 'approved'
      AND dec.revoked_at IS NULL
    ORDER BY dec.created_at DESC, dec.updated_at DESC
    LIMIT 1
) ec ON true
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
WITH abort_args AS (
    SELECT
        sqlc.arg('reason')::text AS reason,
        sqlc.arg('execution_instance_id')::uuid AS execution_instance_id,
        sqlc.arg('tenant_id')::uuid AS tenant_id,
        sqlc.arg('digital_employee_id')::uuid AS digital_employee_id,
        sqlc.arg('execution_instance_id')::uuid = '00000000-0000-0000-0000-000000000000'::uuid AS abort_by_employee
),
aborted_instance AS (
    UPDATE digital_employee_execution_instances dei
    SET status = 'error',
        error_at = COALESCE(dei.error_at, NOW()),
        error_message = abort_args.reason,
        deleted_at = COALESCE(dei.deleted_at, NOW()),
        updated_at = NOW()
    FROM abort_args
    WHERE dei.id = abort_args.execution_instance_id
      AND dei.tenant_id = abort_args.tenant_id
      AND dei.digital_employee_id = abort_args.digital_employee_id
      AND NOT abort_args.abort_by_employee
    RETURNING dei.id
),
abort_scope AS (
    SELECT abort_args.abort_by_employee OR EXISTS (SELECT 1 FROM aborted_instance) AS matched
    FROM abort_args
),
aborted_employee AS (
    UPDATE digital_employees de
    SET status = 'error',
        deleted_at = COALESCE(de.deleted_at, NOW()),
        updated_at = NOW()
    FROM abort_args
    WHERE de.id = abort_args.digital_employee_id
      AND de.tenant_id = abort_args.tenant_id
      AND EXISTS (SELECT 1 FROM abort_scope WHERE matched)
    RETURNING de.id
),
aborted_configs AS (
    UPDATE digital_employee_config_revisions decr
    SET archived_at = COALESCE(decr.archived_at, NOW()),
        updated_at = NOW()
    FROM abort_args
    WHERE decr.tenant_id = abort_args.tenant_id
      AND decr.digital_employee_id = abort_args.digital_employee_id
      AND decr.archived_at IS NULL
      AND EXISTS (SELECT 1 FROM aborted_employee)
      AND EXISTS (SELECT 1 FROM abort_scope WHERE matched)
    RETURNING decr.id
),
aborted_effective_configs AS (
    UPDATE digital_employee_effective_configs deec
    SET status = CASE
            WHEN deec.status = 'revoked' THEN deec.status
            ELSE 'revoked'
        END,
        revoked_at = COALESCE(deec.revoked_at, NOW()),
        updated_at = NOW()
    FROM abort_args
    WHERE deec.tenant_id = abort_args.tenant_id
      AND deec.digital_employee_id = abort_args.digital_employee_id
      AND deec.revoked_at IS NULL
      AND EXISTS (SELECT 1 FROM aborted_employee)
      AND EXISTS (SELECT 1 FROM abort_scope WHERE matched)
    RETURNING deec.id
),
aborted_receipts AS (
    UPDATE runtime_command_receipts rcr
    SET status = CASE
            WHEN rcr.status IN ('completed', 'failed', 'cancelled', 'timed_out') THEN rcr.status
            ELSE 'failed'
        END,
        error_message = COALESCE(rcr.error_message, abort_args.reason),
        completed_at = CASE
            WHEN rcr.status IN ('completed', 'failed', 'cancelled', 'timed_out') THEN rcr.completed_at
            ELSE COALESCE(rcr.completed_at, NOW())
        END,
        updated_at = NOW()
    FROM abort_args
    JOIN aborted_instance ai ON TRUE
    JOIN aborted_employee ae ON TRUE
    WHERE rcr.tenant_id = abort_args.tenant_id
      AND rcr.resource_type = 'digital_employee_execution_instance'
      AND rcr.resource_id = ai.id
    RETURNING rcr.id
)
SELECT 1;

-- name: GetDigitalEmployeeOverviewSummary :one
WITH overview_args AS (
    SELECT
        sqlc.arg('tenant_id')::uuid AS tenant_id,
        NULLIF(BTRIM(sqlc.narg('q')::text), '') AS q,
        sqlc.narg('team_id')::uuid AS team_id,
        NULLIF(BTRIM(sqlc.narg('status')::text), '') AS status,
        NULLIF(BTRIM(sqlc.narg('employee_type')::text), '') AS employee_type,
        NULLIF(BTRIM(sqlc.narg('provider_type')::text), '') AS provider_type,
        sqlc.narg('runtime_node_id')::uuid AS runtime_node_id,
        NULLIF(BTRIM(sqlc.narg('risk_level')::text), '') AS risk_level,
        NULLIF(BTRIM(sqlc.narg('execution_status')::text), '') AS execution_status,
        NULLIF(BTRIM(sqlc.narg('run_status')::text), '') AS run_status
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
      AND rc.disabled_at IS NULL
      AND rc.archived_at IS NULL
    ORDER BY rc.tenant_id, rc.runtime_node_id, rc.provider_type, rc.last_seen_at DESC NULLS LAST, rc.updated_at DESC
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
effective_configs AS (
    SELECT DISTINCT ON (ec.tenant_id, ec.digital_employee_id)
        ec.tenant_id,
        ec.digital_employee_id,
        ec.id AS effective_config_id,
        ec.status
    FROM digital_employee_effective_configs ec
    JOIN overview_args args ON args.tenant_id = ec.tenant_id
    WHERE ec.status = 'approved'
      AND ec.revoked_at IS NULL
    ORDER BY ec.tenant_id, ec.digital_employee_id, ec.created_at DESC, ec.updated_at DESC
),
governance_configs AS (
    SELECT DISTINCT ON (ec.tenant_id, ec.digital_employee_id)
        ec.tenant_id,
        ec.digital_employee_id,
        ec.status
    FROM digital_employee_effective_configs ec
    JOIN overview_args args ON args.tenant_id = ec.tenant_id
    WHERE ec.revoked_at IS NULL
    ORDER BY ec.tenant_id, ec.digital_employee_id, ec.created_at DESC, ec.updated_at DESC
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
        COALESCE(dei.status, 'missing')::text AS execution_status,
        dei.runtime_node_id,
        rn.status AS runtime_status,
        rn.disabled_at AS runtime_disabled_at,
        rn.archived_at AS runtime_archived_at,
        COALESCE(dei.provider_type, '')::text AS provider_type,
        (NULLIF(BTRIM(COALESCE(dei.agent_home_dir, '')), '') IS NOT NULL)::boolean AS agent_home_dir_available,
        COALESCE(pc.available, false)::boolean AS provider_available,
        COALESCE(pc.status, '')::text AS provider_status,
        COALESCE(pc.health_status, '')::text AS provider_health_status,
        COALESCE(lr.status, 'none')::text AS run_status,
        ec.effective_config_id,
        COALESCE(gc.status, 'missing')::text AS governance_status
    FROM digital_employees de
    CROSS JOIN overview_args args
    LEFT JOIN digital_employee_execution_instances dei
     ON dei.tenant_id = de.tenant_id
     AND dei.digital_employee_id = de.id
     AND dei.deleted_at IS NULL
    LEFT JOIN runtime_nodes rn
      ON rn.id = dei.runtime_node_id
     AND rn.tenant_id = dei.tenant_id
    LEFT JOIN provider_capabilities pc
      ON pc.tenant_id = dei.tenant_id
     AND pc.runtime_node_id = dei.runtime_node_id
     AND pc.provider_type = dei.provider_type
    LEFT JOIN latest_runs lr
      ON lr.tenant_id = de.tenant_id
     AND lr.digital_employee_id = de.id
    LEFT JOIN effective_configs ec
      ON ec.tenant_id = de.tenant_id
     AND ec.digital_employee_id = de.id
    LEFT JOIN governance_configs gc
      ON gc.tenant_id = de.tenant_id
     AND gc.digital_employee_id = de.id
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
      AND (args.runtime_node_id IS NULL OR overview_rows.runtime_node_id = args.runtime_node_id)
      AND (args.risk_level IS NULL OR overview_rows.risk_level = args.risk_level)
      AND (args.execution_status IS NULL OR overview_rows.execution_status = args.execution_status)
      AND (args.run_status IS NULL OR overview_rows.run_status = args.run_status)
)
SELECT
    COUNT(*)::integer AS total_count,
    (COUNT(*) FILTER (
        WHERE employee_status IN ('ready', 'active')
          AND execution_status IN ('ready', 'active')
          AND effective_config_id IS NOT NULL
          AND runtime_node_id IS NOT NULL
          AND runtime_status = 'online'
          AND runtime_disabled_at IS NULL
          AND runtime_archived_at IS NULL
          AND agent_home_dir_available = true
          AND governance_status = 'approved'
          AND provider_available = true
          AND provider_status = 'healthy'
          AND provider_health_status = 'healthy'
    ))::integer AS runnable_count,
    (COUNT(*) FILTER (
        WHERE run_status IN ('queued', 'dispatching', 'running', 'cancelling')
    ))::integer AS running_count,
    (COUNT(*) FILTER (
        WHERE execution_status IN ('missing', 'provisioning')
    ))::integer AS waiting_runtime_count,
    (COUNT(*) FILTER (
        WHERE employee_status IN ('disabled', 'error')
           OR execution_status IN ('disabled', 'error')
           OR run_status IN ('failed', 'timed_out')
           OR (
               runtime_node_id IS NOT NULL
               AND (
                   runtime_status <> 'online'
                   OR runtime_disabled_at IS NOT NULL
                   OR runtime_archived_at IS NOT NULL
               )
           )
           OR (
               provider_type <> ''
               AND (
                   provider_available = false
                   OR provider_status <> 'healthy'
                   OR provider_health_status <> 'healthy'
               )
           )
    ))::integer AS error_count,
    (COUNT(*) FILTER (
        WHERE risk_level IN ('high', 'critical')
    ))::integer AS high_risk_count,
    (COUNT(*) FILTER (
        WHERE employee_status IN ('ready', 'active')
          AND execution_status IN ('ready', 'active')
          AND effective_config_id IS NOT NULL
          AND runtime_node_id IS NOT NULL
          AND runtime_status = 'online'
          AND runtime_disabled_at IS NULL
          AND runtime_archived_at IS NULL
          AND agent_home_dir_available = true
          AND governance_status = 'approved'
          AND provider_available = true
          AND provider_status = 'healthy'
          AND provider_health_status = 'healthy'
          AND run_status NOT IN ('failed', 'timed_out')
    ))::integer AS ready_count,
    (COUNT(*) FILTER (
        WHERE execution_status IN ('missing', 'provisioning')
           OR runtime_node_id IS NULL
           OR provider_type = ''
           OR agent_home_dir_available = false
    ))::integer AS pending_runtime_binding_count,
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
        sqlc.narg('runtime_node_id')::uuid AS runtime_node_id,
        NULLIF(BTRIM(sqlc.narg('risk_level')::text), '') AS risk_level,
        NULLIF(BTRIM(sqlc.narg('execution_status')::text), '') AS execution_status,
        NULLIF(BTRIM(sqlc.narg('run_status')::text), '') AS run_status,
        sqlc.arg('limit')::integer AS limit_value,
        sqlc.arg('offset')::integer AS offset_value
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
      AND rc.disabled_at IS NULL
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
        tr.created_at
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
effective_configs AS (
    SELECT DISTINCT ON (ec.tenant_id, ec.digital_employee_id)
        ec.tenant_id,
        ec.digital_employee_id,
        ec.id AS effective_config_id,
        ec.status,
        ec.effective_config_snapshot -> 'budget_policy' AS budget_policy,
        NULLIF(ec.effective_config_snapshot #>> '{budget_policy,daily_token_limit}', '') AS daily_token_limit_text,
        ttcr.revision_number AS team_revision_number,
        decr.revision_number AS employee_revision_number,
        CASE
            WHEN jsonb_typeof(ec.effective_config_snapshot #> '{capability_selection,enabled_mcp_servers}') = 'array'
            THEN jsonb_array_length(ec.effective_config_snapshot #> '{capability_selection,enabled_mcp_servers}')
            ELSE 0
        END::integer AS mcp_servers_count,
        COALESCE(
            ec.effective_config_snapshot #>> '{constitution,ref}',
            ec.effective_config_snapshot #>> '{constitution,team,ref}',
            ec.effective_config_snapshot #>> '{constitution,team,document_ref}',
            ''
        )::text AS constitution_ref
    FROM digital_employee_effective_configs ec
    JOIN overview_args args ON args.tenant_id = ec.tenant_id
    LEFT JOIN tenant_team_config_revisions ttcr
      ON ttcr.id = ec.tenant_team_config_revision_id
     AND ttcr.tenant_id = ec.tenant_id
    LEFT JOIN digital_employee_config_revisions decr
      ON decr.id = ec.employee_config_revision_id
     AND decr.tenant_id = ec.tenant_id
     AND decr.digital_employee_id = ec.digital_employee_id
    WHERE ec.status = 'approved'
      AND ec.revoked_at IS NULL
    ORDER BY ec.tenant_id, ec.digital_employee_id, ec.created_at DESC, ec.updated_at DESC
),
governance_configs AS (
    SELECT DISTINCT ON (ec.tenant_id, ec.digital_employee_id)
        ec.tenant_id,
        ec.digital_employee_id,
        ec.status
    FROM digital_employee_effective_configs ec
    JOIN overview_args args ON args.tenant_id = ec.tenant_id
    WHERE ec.revoked_at IS NULL
    ORDER BY ec.tenant_id, ec.digital_employee_id, ec.created_at DESC, ec.updated_at DESC
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
        dei.id AS execution_instance_id,
        COALESCE(dei.status, 'missing')::text AS execution_status,
        dei.runtime_node_id,
        COALESCE(rn.node_id, '')::text AS node_id,
        COALESCE(rn.name, '')::text AS runtime_name,
        COALESCE(rn.status, '')::text AS runtime_status,
        rn.disabled_at AS runtime_disabled_at,
        rn.archived_at AS runtime_archived_at,
        COALESCE(dei.provider_type, '')::text AS provider_type,
        COALESCE(pc.available, false)::boolean AS provider_available,
        COALESCE(pc.status, 'unknown')::text AS provider_status,
        COALESCE(pc.health_status, 'unknown')::text AS health_status,
        (NULLIF(BTRIM(COALESCE(dei.agent_home_dir, '')), '') IS NOT NULL)::boolean AS agent_home_dir_available,
        lr.id AS latest_run_id,
        lr.task_id AS latest_run_task_id,
        COALESCE(lr.status, 'none')::text AS latest_run_status,
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
        ec.effective_config_id,
        COALESCE(gc.status, 'missing')::text AS governance_status,
        COALESCE(ec.daily_token_limit_text, '')::text AS daily_token_limit_text,
        ec.team_revision_number,
        ec.employee_revision_number,
        COALESCE(sc.skills_count, 0)::integer AS skills_count,
        COALESCE(ec.mcp_servers_count, 0)::integer AS mcp_servers_count,
        COALESCE(ec.constitution_ref, '')::text AS constitution_ref,
        COALESCE(tbu.today_budget_usage_tokens, 0)::integer AS today_budget_usage_tokens,
        br.budget_usage_tokens_30d,
        COALESCE(br.budget_run_count_30d, 0)::integer AS budget_run_count_30d,
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
    LEFT JOIN digital_employee_execution_instances dei
      ON dei.tenant_id = de.tenant_id
     AND dei.digital_employee_id = de.id
     AND dei.deleted_at IS NULL
    LEFT JOIN runtime_nodes rn
      ON rn.id = dei.runtime_node_id
     AND rn.tenant_id = dei.tenant_id
    LEFT JOIN provider_capabilities pc
      ON pc.tenant_id = dei.tenant_id
     AND pc.runtime_node_id = dei.runtime_node_id
     AND pc.provider_type = dei.provider_type
    LEFT JOIN latest_runs lr
      ON lr.tenant_id = de.tenant_id
     AND lr.digital_employee_id = de.id
    LEFT JOIN budget_runs br
      ON br.tenant_id = de.tenant_id
     AND br.digital_employee_id = de.id
    LEFT JOIN today_budget_usage tbu
      ON tbu.tenant_id = de.tenant_id
     AND tbu.digital_employee_id = de.id
    LEFT JOIN effective_configs ec
      ON ec.tenant_id = de.tenant_id
     AND ec.digital_employee_id = de.id
    LEFT JOIN governance_configs gc
      ON gc.tenant_id = de.tenant_id
     AND gc.digital_employee_id = de.id
    LEFT JOIN skill_counts sc
      ON sc.tenant_id = de.tenant_id
     AND sc.digital_employee_id = de.id
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
      AND (args.runtime_node_id IS NULL OR overview_rows.runtime_node_id = args.runtime_node_id)
      AND (args.risk_level IS NULL OR overview_rows.risk_level = args.risk_level)
      AND (args.execution_status IS NULL OR overview_rows.execution_status = args.execution_status)
      AND (args.run_status IS NULL OR overview_rows.latest_run_status = args.run_status)
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
                'label', ranked.event_label,
                'status', ranked.event_status,
                'occurred_at', ranked.occurred_at
            )
            ORDER BY ranked.occurred_at DESC NULLS LAST, ranked.sequence_number DESC
        ) AS recent_events_json
    FROM (
        SELECT
            pr.tenant_id,
            pr.id AS digital_employee_id,
            te.sequence_number,
            CASE
                WHEN te.event_type = 'run_dispatched' THEN '命令已下发'
                WHEN te.event_type ILIKE '%provider%' THEN 'Provider 输出中'
                WHEN te.event_type ILIKE '%complete%' THEN '等待结果回写'
                ELSE te.event_type
            END AS event_label,
            CASE
                WHEN te.event_type ILIKE '%fail%' THEN 'failed'
                WHEN te.event_type ILIKE '%complete%' THEN 'completed'
                ELSE 'running'
            END AS event_status,
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
    COALESCE(re.recent_events_json, '[]'::jsonb) AS recent_events_json
FROM paged_rows pr
LEFT JOIN recent_events re
  ON re.tenant_id = pr.tenant_id
 AND re.digital_employee_id = pr.id
ORDER BY pr.created_at DESC, pr.id;

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
        COALESCE(dei.provider_type, '')::text AS provider_type,
        dei.runtime_node_id,
        COALESCE(rn.name, rn.node_id, '')::text AS runtime_name,
        COALESCE(dei.status, 'missing')::text AS execution_status,
        COALESCE(lr.status, 'none')::text AS run_status
    FROM digital_employees de
    CROSS JOIN overview_args args
    LEFT JOIN tenant_teams tt
      ON tt.id = de.team_id
     AND tt.tenant_id = de.tenant_id
     AND tt.deleted_at IS NULL
    LEFT JOIN digital_employee_execution_instances dei
      ON dei.tenant_id = de.tenant_id
     AND dei.digital_employee_id = de.id
     AND dei.deleted_at IS NULL
    LEFT JOIN runtime_nodes rn
      ON rn.id = dei.runtime_node_id
     AND rn.tenant_id = dei.tenant_id
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
        'runtime_node'::text AS filter_type,
        COALESCE(runtime_node_id::text, '')::text AS value,
        runtime_name AS label
    FROM employee_rows
    WHERE runtime_node_id IS NOT NULL

    UNION ALL

    SELECT DISTINCT
        'risk_level'::text AS filter_type,
        risk_level::text AS value,
        risk_level::text AS label
    FROM employee_rows
    WHERE NULLIF(risk_level, '') IS NOT NULL

    UNION ALL

    SELECT DISTINCT
        'execution_status'::text AS filter_type,
        execution_status AS value,
        execution_status AS label
    FROM employee_rows
    WHERE NULLIF(execution_status, '') IS NOT NULL

    UNION ALL

    SELECT DISTINCT
        'run_status'::text AS filter_type,
        run_status AS value,
        run_status AS label
    FROM employee_rows
    WHERE NULLIF(run_status, '') IS NOT NULL
) options
ORDER BY filter_type, label, value;
