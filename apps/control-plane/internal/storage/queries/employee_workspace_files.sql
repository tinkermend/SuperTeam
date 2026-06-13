-- name: CreateDigitalEmployeeWorkspaceFile :one
INSERT INTO digital_employee_workspace_files (
    tenant_id,
    team_id,
    digital_employee_id,
    path,
    file_role,
    mime_type,
    sync_policy,
    status,
    metadata,
    created_by
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('team_id')::uuid,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('path')::text,
    sqlc.arg('file_role')::varchar,
    sqlc.arg('mime_type')::varchar,
    sqlc.arg('sync_policy')::varchar,
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb),
    sqlc.narg('created_by')::uuid
) RETURNING *;

-- name: CreateDigitalEmployeeWorkspaceFileRevision :one
INSERT INTO digital_employee_workspace_file_revisions (
    tenant_id,
    file_id,
    revision_number,
    content_text,
    content_hash,
    size_bytes,
    storage_backend,
    object_key,
    created_by,
    change_note,
    metadata
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('file_id')::uuid,
    sqlc.arg('revision_number')::integer,
    sqlc.narg('content_text')::text,
    sqlc.arg('content_hash')::varchar,
    sqlc.arg('size_bytes')::integer,
    sqlc.arg('storage_backend')::varchar,
    sqlc.narg('object_key')::text,
    sqlc.narg('created_by')::uuid,
    sqlc.narg('change_note')::text,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
) RETURNING *;

-- name: ActivateDigitalEmployeeWorkspaceFileRevision :one
UPDATE digital_employee_workspace_files
SET current_revision_id = sqlc.arg('revision_id')::uuid,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('file_id')::uuid
  AND deleted_at IS NULL
  AND EXISTS (
    SELECT 1
    FROM digital_employee_workspace_file_revisions r
    WHERE r.id = sqlc.arg('revision_id')::uuid
      AND r.tenant_id = digital_employee_workspace_files.tenant_id
      AND r.file_id = digital_employee_workspace_files.id
  )
RETURNING *;

-- name: ListCurrentDigitalEmployeeWorkspaceFilesForSync :many
SELECT
    f.id AS file_id,
    f.tenant_id,
    f.team_id,
    f.digital_employee_id,
    f.path,
    f.file_role,
    f.mime_type,
    f.sync_policy,
    f.status,
    f.metadata AS file_metadata,
    r.id AS revision_id,
    r.revision_number,
    r.content_text,
    r.content_hash,
    r.size_bytes,
    r.storage_backend,
    r.object_key,
    r.metadata AS revision_metadata
FROM digital_employee_workspace_files f
JOIN digital_employee_workspace_file_revisions r
  ON r.id = f.current_revision_id
 AND r.tenant_id = f.tenant_id
 AND r.file_id = f.id
WHERE f.tenant_id = sqlc.arg('tenant_id')::uuid
  AND f.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND f.status = 'active'
  AND f.deleted_at IS NULL
  AND f.sync_policy <> 'disabled'
ORDER BY f.path ASC;

-- name: UpsertDigitalEmployeeWorkspaceFileSync :exec
INSERT INTO digital_employee_workspace_file_syncs (
    tenant_id,
    digital_employee_id,
    execution_instance_id,
    file_id,
    revision_id,
    runtime_node_id,
    status,
    synced_hash,
    error_message,
    last_command_id,
    last_synced_at
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('execution_instance_id')::uuid,
    sqlc.arg('file_id')::uuid,
    sqlc.arg('revision_id')::uuid,
    sqlc.arg('runtime_node_id')::uuid,
    sqlc.arg('status')::varchar,
    sqlc.narg('synced_hash')::varchar,
    sqlc.narg('error_message')::text,
    sqlc.narg('last_command_id')::varchar,
    sqlc.narg('last_synced_at')::timestamptz
) ON CONFLICT (tenant_id, digital_employee_id, execution_instance_id, file_id) DO UPDATE SET
    revision_id = EXCLUDED.revision_id,
    runtime_node_id = EXCLUDED.runtime_node_id,
    status = EXCLUDED.status,
    synced_hash = EXCLUDED.synced_hash,
    error_message = EXCLUDED.error_message,
    last_command_id = EXCLUDED.last_command_id,
    last_synced_at = EXCLUDED.last_synced_at,
    updated_at = NOW();
