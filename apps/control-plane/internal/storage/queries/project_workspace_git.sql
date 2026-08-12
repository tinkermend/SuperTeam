-- name: GetProjectWorkspaceGitSnapshot :one
SELECT *
FROM project_workspace_git_snapshots
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid;

-- name: ListProjectWorkspaceGitSnapshots :many
SELECT *
FROM project_workspace_git_snapshots
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = ANY(sqlc.arg('project_ids')::uuid[]);

-- name: UpsertProjectWorkspaceGitSnapshotSuccess :exec
INSERT INTO project_workspace_git_snapshots (
    tenant_id,
    project_id,
    is_git_repo,
    is_clean,
    head_commit,
    current_branch,
    detached,
    repo_state,
    uncommitted_count,
    uncommitted_entries,
    uncommitted_truncated,
    uncommitted_omitted,
    sampled_at,
    sampled_runtime_node_id,
    sampled_node_id,
    sample_error,
    last_attempt_at,
    inflight_at,
    inflight_command_id,
    updated_at
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.narg('is_git_repo')::boolean,
    sqlc.narg('is_clean')::boolean,
    sqlc.narg('head_commit')::varchar,
    sqlc.narg('current_branch')::varchar,
    sqlc.arg('detached')::boolean,
    sqlc.narg('repo_state')::varchar,
    sqlc.arg('uncommitted_count')::integer,
    COALESCE(sqlc.narg('uncommitted_entries')::jsonb, '[]'::jsonb),
    sqlc.arg('uncommitted_truncated')::boolean,
    sqlc.arg('uncommitted_omitted')::integer,
    sqlc.arg('sampled_at')::timestamptz,
    sqlc.narg('sampled_runtime_node_id')::uuid,
    sqlc.narg('sampled_node_id')::varchar,
    NULL,
    sqlc.arg('sampled_at')::timestamptz,
    NULL,
    NULL,
    NOW()
)
ON CONFLICT (project_id) DO UPDATE SET
    is_git_repo = EXCLUDED.is_git_repo,
    is_clean = EXCLUDED.is_clean,
    head_commit = EXCLUDED.head_commit,
    current_branch = EXCLUDED.current_branch,
    detached = EXCLUDED.detached,
    repo_state = EXCLUDED.repo_state,
    uncommitted_count = EXCLUDED.uncommitted_count,
    uncommitted_entries = EXCLUDED.uncommitted_entries,
    uncommitted_truncated = EXCLUDED.uncommitted_truncated,
    uncommitted_omitted = EXCLUDED.uncommitted_omitted,
    sampled_at = EXCLUDED.sampled_at,
    sampled_runtime_node_id = EXCLUDED.sampled_runtime_node_id,
    sampled_node_id = EXCLUDED.sampled_node_id,
    sample_error = NULL,
    last_attempt_at = EXCLUDED.last_attempt_at,
    inflight_at = NULL,
    inflight_command_id = NULL,
    updated_at = NOW()
WHERE project_workspace_git_snapshots.sampled_at IS NULL
   OR project_workspace_git_snapshots.sampled_at < EXCLUDED.sampled_at;

-- name: MarkProjectWorkspaceGitSnapshotFailed :exec
INSERT INTO project_workspace_git_snapshots (
    tenant_id,
    project_id,
    sample_error,
    last_attempt_at,
    inflight_at,
    inflight_command_id,
    updated_at
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('sample_error')::text,
    sqlc.arg('attempted_at')::timestamptz,
    NULL,
    NULL,
    NOW()
)
ON CONFLICT (project_id) DO UPDATE SET
    sample_error = EXCLUDED.sample_error,
    last_attempt_at = EXCLUDED.last_attempt_at,
    inflight_at = NULL,
    inflight_command_id = NULL,
    updated_at = NOW();

-- name: MarkProjectWorkspaceGitProbeInflight :exec
INSERT INTO project_workspace_git_snapshots (
    tenant_id,
    project_id,
    inflight_at,
    inflight_command_id,
    last_attempt_at,
    updated_at
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('inflight_at')::timestamptz,
    sqlc.arg('inflight_command_id')::varchar,
    sqlc.arg('inflight_at')::timestamptz,
    NOW()
)
ON CONFLICT (project_id) DO UPDATE SET
    inflight_at = EXCLUDED.inflight_at,
    inflight_command_id = EXCLUDED.inflight_command_id,
    last_attempt_at = EXCLUDED.last_attempt_at,
    updated_at = NOW();

-- name: ListProjectsDueForWorkspaceGitSample :many
SELECT
    p.id,
    p.tenant_id,
    p.directory_name,
    p.primary_runtime_node_id,
    s.sampled_at,
    s.is_clean,
    s.sample_error,
    s.inflight_at
FROM projects p
LEFT JOIN project_workspace_git_snapshots s
  ON s.project_id = p.id AND s.tenant_id = p.tenant_id
WHERE p.deleted_at IS NULL
  AND p.archived_at IS NULL
  AND p.workspace_ready_status = 'ready'
  AND p.primary_runtime_node_id IS NOT NULL
  AND NULLIF(BTRIM(p.directory_name), '') IS NOT NULL
  AND (
        s.inflight_at IS NULL
        OR s.inflight_at < sqlc.arg('inflight_stale_before')::timestamptz
      )
  AND (
        s.sampled_at IS NULL
        OR (
            COALESCE(s.is_clean, false) = true
            AND s.sample_error IS NULL
            AND s.sampled_at < sqlc.arg('idle_stale_before')::timestamptz
        )
        OR (
            (COALESCE(s.is_clean, false) = false OR s.sample_error IS NOT NULL)
            AND s.sampled_at < sqlc.arg('stale_before')::timestamptz
        )
      )
ORDER BY s.sampled_at NULLS FIRST, p.updated_at ASC
LIMIT sqlc.arg('limit_count')::integer;
