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
    ON CONFLICT (tenant_id, attempt_id, idempotency_key) DO UPDATE SET
        status = EXCLUDED.status,
        command_argv = EXCLUDED.command_argv,
        exit_code = EXCLUDED.exit_code,
        duration_ms = EXCLUDED.duration_ms,
        log_ref = EXCLUDED.log_ref,
        stdout_sha256 = EXCLUDED.stdout_sha256,
        stderr_sha256 = EXCLUDED.stderr_sha256,
        artifact_refs = EXCLUDED.artifact_refs,
        artifact_hashes = EXCLUDED.artifact_hashes,
        git_branch = EXCLUDED.git_branch,
        git_base_ref = EXCLUDED.git_base_ref,
        git_head_sha = EXCLUDED.git_head_sha,
        git_diff_sha256 = EXCLUDED.git_diff_sha256,
        metadata = EXCLUDED.metadata,
        capability_manifest_version = EXCLUDED.capability_manifest_version,
        provider_auth_mode = EXCLUDED.provider_auth_mode,
        provider_session_id = EXCLUDED.provider_session_id
    RETURNING *
)
SELECT *
FROM inserted;

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
