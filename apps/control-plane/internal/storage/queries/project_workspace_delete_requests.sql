-- Project workspace delete confirmation queue (spec 2026-08-12 P0).

-- name: InsertProjectWorkspaceDeleteRequest :one
INSERT INTO project_workspace_delete_requests (
    id,
    tenant_id,
    project_id,
    runtime_node_id,
    directory_name,
    node_id_snapshot,
    ownership,
    repo_summary,
    status,
    requested_by,
    requested_at,
    reason
) VALUES (
    sqlc.arg('id')::uuid,
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('runtime_node_id')::uuid,
    sqlc.arg('directory_name'),
    sqlc.arg('node_id_snapshot'),
    sqlc.arg('ownership'),
    sqlc.arg('repo_summary'),
    'pending',
    sqlc.arg('requested_by')::uuid,
    sqlc.arg('requested_at'),
    sqlc.narg('reason')
)
ON CONFLICT (project_id, runtime_node_id) WHERE status = 'pending'
DO UPDATE SET
    -- Idempotent re-enqueue: keep existing pending row, refresh reason if provided.
    reason = COALESCE(EXCLUDED.reason, project_workspace_delete_requests.reason)
RETURNING *;

-- name: GetProjectWorkspaceDeleteRequest :one
SELECT *
FROM project_workspace_delete_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid;

-- name: ListPendingProjectWorkspaceDeleteRequests :many
SELECT *
FROM project_workspace_delete_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND status = 'pending'
ORDER BY requested_at ASC;

-- name: ListStalePendingProjectWorkspaceDeleteRequests :many
SELECT *
FROM project_workspace_delete_requests
WHERE status = 'pending'
  AND requested_at < sqlc.arg('stale_before')
ORDER BY requested_at ASC
LIMIT 100;

-- name: CountPendingProjectWorkspaceDeleteByDirectoryName :one
SELECT COUNT(*)::int AS count
FROM project_workspace_delete_requests
WHERE directory_name = sqlc.arg('directory_name')
  AND status = 'pending';

-- name: ConfirmProjectWorkspaceDeleteRequest :one
UPDATE project_workspace_delete_requests
SET status = 'confirmed',
    resolved_by = sqlc.arg('resolved_by')::uuid,
    resolved_at = sqlc.arg('resolved_at')
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = 'pending'
RETURNING *;

-- name: RejectProjectWorkspaceDeleteRequest :one
UPDATE project_workspace_delete_requests
SET status = 'rejected',
    resolved_by = sqlc.arg('resolved_by')::uuid,
    resolved_at = sqlc.arg('resolved_at'),
    reason = COALESCE(sqlc.narg('reason'), reason)
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = 'pending'
RETURNING *;

-- name: ResolveOrphanProjectWorkspaceDeleteReminders :exec
-- Close open inbox reminders whose source request is no longer pending.
UPDATE inbox_items
SET status = 'resolved',
    updated_at = NOW()
WHERE source_type = 'project_workspace_pending_delete'
  AND status = 'open'
  AND NOT EXISTS (
    SELECT 1
    FROM project_workspace_delete_requests r
    WHERE r.id = inbox_items.source_id
      AND r.status = 'pending'
  );
