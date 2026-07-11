-- name: GetProjectRepoBinding :one
SELECT
    id,
    tenant_id,
    repo_url,
    repo_default_branch,
    repo_git_credential_ref,
    repo_scope,
    repo_binding_status
FROM projects
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('project_id')::uuid
  AND deleted_at IS NULL;

-- name: UpsertProjectPlacement :one
INSERT INTO project_placements (
    tenant_id,
    project_id,
    runtime_node_id,
    placement_status,
    placement_reason
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('runtime_node_id')::uuid,
    'active',
    sqlc.narg('placement_reason')::varchar
)
ON CONFLICT (tenant_id, project_id) WHERE placement_status = 'active'
DO UPDATE SET
    runtime_node_id = EXCLUDED.runtime_node_id,
    placement_reason = EXCLUDED.placement_reason,
    assigned_at = NOW(),
    updated_at = NOW()
RETURNING *;

-- name: GetActiveProjectPlacement :one
SELECT *
FROM project_placements
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND placement_status = 'active';

-- name: ReleaseProjectPlacement :one
UPDATE project_placements
SET
    placement_status = 'released',
    released_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND placement_status = 'active'
RETURNING *;

-- name: CreateProjectTaskAttestation :one
WITH input AS (
    SELECT
        sqlc.arg('tenant_id')::uuid AS tenant_id,
        sqlc.arg('project_id')::uuid AS project_id,
        sqlc.arg('project_task_id')::uuid AS project_task_id,
        sqlc.arg('attempt_id')::uuid AS attempt_id,
        sqlc.arg('runtime_node_id')::uuid AS runtime_node_id,
        sqlc.arg('digital_employee_id')::uuid AS digital_employee_id,
        sqlc.narg('capability_manifest_version')::varchar AS capability_manifest_version,
        COALESCE(sqlc.narg('provider_auth_mode')::varchar, 'host') AS provider_auth_mode,
        sqlc.narg('provider_session_id')::varchar AS provider_session_id,
        sqlc.arg('attestation_type')::varchar AS attestation_type,
        sqlc.arg('status')::varchar AS status,
        COALESCE(sqlc.narg('command_argv')::jsonb, '[]'::jsonb) AS command_argv,
        sqlc.narg('exit_code')::integer AS exit_code,
        sqlc.narg('duration_ms')::bigint AS duration_ms,
        sqlc.narg('log_ref')::text AS log_ref,
        sqlc.narg('stdout_sha256')::varchar AS stdout_sha256,
        sqlc.narg('stderr_sha256')::varchar AS stderr_sha256,
        COALESCE(sqlc.narg('artifact_refs')::jsonb, '[]'::jsonb) AS artifact_refs,
        COALESCE(sqlc.narg('artifact_hashes')::jsonb, '{}'::jsonb) AS artifact_hashes,
        sqlc.narg('git_branch')::varchar AS git_branch,
        sqlc.narg('git_base_ref')::varchar AS git_base_ref,
        sqlc.narg('git_head_sha')::varchar AS git_head_sha,
        sqlc.narg('git_diff_sha256')::varchar AS git_diff_sha256,
        COALESCE(sqlc.narg('metadata')::jsonb, '{}'::jsonb) AS metadata,
        sqlc.arg('idempotency_key')::varchar AS idempotency_key
),
inserted AS (
    INSERT INTO project_task_attestations (
        tenant_id,
        project_id,
        project_task_id,
        attempt_id,
        runtime_node_id,
        digital_employee_id,
        capability_manifest_version,
        provider_auth_mode,
        provider_session_id,
        attestation_type,
        status,
        command_argv,
        exit_code,
        duration_ms,
        log_ref,
        stdout_sha256,
        stderr_sha256,
        artifact_refs,
        artifact_hashes,
        git_branch,
        git_base_ref,
        git_head_sha,
        git_diff_sha256,
        metadata,
        idempotency_key
    )
    SELECT
        tenant_id,
        project_id,
        project_task_id,
        attempt_id,
        runtime_node_id,
        digital_employee_id,
        capability_manifest_version,
        provider_auth_mode,
        provider_session_id,
        attestation_type,
        status,
        command_argv,
        exit_code,
        duration_ms,
        log_ref,
        stdout_sha256,
        stderr_sha256,
        artifact_refs,
        artifact_hashes,
        git_branch,
        git_base_ref,
        git_head_sha,
        git_diff_sha256,
        metadata,
        idempotency_key
    FROM input
    ON CONFLICT (tenant_id, attempt_id, idempotency_key) DO NOTHING
    RETURNING *
)
SELECT *
FROM inserted
UNION ALL
SELECT existing.*
FROM project_task_attestations existing
JOIN input
  ON existing.tenant_id = input.tenant_id
 AND existing.attempt_id = input.attempt_id
 AND existing.idempotency_key = input.idempotency_key
WHERE NOT EXISTS (SELECT 1 FROM inserted)
  AND existing.project_id = input.project_id
  AND existing.project_task_id = input.project_task_id
  AND existing.runtime_node_id = input.runtime_node_id
  AND existing.digital_employee_id = input.digital_employee_id
  AND existing.capability_manifest_version IS NOT DISTINCT FROM input.capability_manifest_version
  AND existing.provider_auth_mode = input.provider_auth_mode
  AND existing.provider_session_id IS NOT DISTINCT FROM input.provider_session_id
  AND existing.attestation_type = input.attestation_type
  AND existing.status = input.status
  AND existing.command_argv = input.command_argv
  AND existing.exit_code IS NOT DISTINCT FROM input.exit_code
  AND existing.duration_ms IS NOT DISTINCT FROM input.duration_ms
  AND existing.log_ref IS NOT DISTINCT FROM input.log_ref
  AND existing.stdout_sha256 IS NOT DISTINCT FROM input.stdout_sha256
  AND existing.stderr_sha256 IS NOT DISTINCT FROM input.stderr_sha256
  AND existing.artifact_refs = input.artifact_refs
  AND existing.artifact_hashes = input.artifact_hashes
  AND existing.git_branch IS NOT DISTINCT FROM input.git_branch
  AND existing.git_base_ref IS NOT DISTINCT FROM input.git_base_ref
  AND existing.git_head_sha IS NOT DISTINCT FROM input.git_head_sha
  AND existing.git_diff_sha256 IS NOT DISTINCT FROM input.git_diff_sha256
  AND existing.metadata = input.metadata;

-- name: ListProjectTaskAttestations :many
SELECT *
FROM project_task_attestations
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND project_task_id = sqlc.arg('project_task_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: UpdateProjectTaskAttemptBudgetHeartbeat :one
UPDATE project_task_attempts
SET
    budget_last_heartbeat_at = GREATEST(
        COALESCE(budget_last_heartbeat_at, statement_timestamp()),
        statement_timestamp()
    ),
    budget_consumed_wall_clock_sec = GREATEST(
        budget_consumed_wall_clock_sec,
        sqlc.arg('consumed_wall_clock_sec')::integer
    ),
    budget_consumed_tokens = GREATEST(
        budget_consumed_tokens,
        sqlc.arg('consumed_tokens')::integer
    ),
    budget_tripped_at = CASE
        WHEN sqlc.narg('trip_reason')::varchar IS NULL THEN budget_tripped_at
        ELSE COALESCE(budget_tripped_at, statement_timestamp())
    END,
    budget_trip_reason = COALESCE(budget_trip_reason, sqlc.narg('trip_reason')::varchar)
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_task_id = sqlc.arg('project_task_id')::uuid
  AND id = sqlc.arg('attempt_id')::uuid
  AND sqlc.arg('consumed_wall_clock_sec')::integer >= 0
  AND sqlc.arg('consumed_tokens')::integer >= 0
RETURNING *;
