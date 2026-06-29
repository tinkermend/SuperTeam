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
  AND id = sqlc.arg('project_id')::uuid;

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

-- name: CreateProjectTaskAttestation :one
INSERT INTO project_task_attestations (
    tenant_id,
    project_id,
    project_task_id,
    attempt_id,
    runtime_node_id,
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
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('project_task_id')::uuid,
    sqlc.arg('attempt_id')::uuid,
    sqlc.arg('runtime_node_id')::uuid,
    sqlc.narg('provider_session_id')::varchar,
    sqlc.arg('attestation_type')::varchar,
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.narg('command_argv')::jsonb, '[]'::jsonb),
    sqlc.narg('exit_code')::integer,
    sqlc.narg('duration_ms')::bigint,
    sqlc.narg('log_ref')::text,
    sqlc.narg('stdout_sha256')::varchar,
    sqlc.narg('stderr_sha256')::varchar,
    COALESCE(sqlc.narg('artifact_refs')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('artifact_hashes')::jsonb, '{}'::jsonb),
    sqlc.narg('git_branch')::varchar,
    sqlc.narg('git_base_ref')::varchar,
    sqlc.narg('git_head_sha')::varchar,
    sqlc.narg('git_diff_sha256')::varchar,
    COALESCE(sqlc.narg('metadata')::jsonb, '{}'::jsonb),
    sqlc.arg('idempotency_key')::varchar
)
ON CONFLICT (tenant_id, attempt_id, idempotency_key) DO UPDATE SET
    idempotency_key = EXCLUDED.idempotency_key
RETURNING *;

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
    budget_last_heartbeat_at = NOW(),
    budget_consumed_wall_clock_sec = sqlc.arg('consumed_wall_clock_sec')::integer,
    budget_consumed_tokens = sqlc.arg('consumed_tokens')::integer,
    budget_tripped_at = CASE
        WHEN sqlc.narg('trip_reason')::varchar IS NULL THEN budget_tripped_at
        ELSE NOW()
    END,
    budget_trip_reason = COALESCE(sqlc.narg('trip_reason')::varchar, budget_trip_reason)
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_task_id = sqlc.arg('project_task_id')::uuid
  AND id = sqlc.arg('attempt_id')::uuid
RETURNING *;
