-- name: InsertProjectRuntimeNode :one
INSERT INTO project_runtime_nodes (
    tenant_id,
    project_id,
    runtime_node_id,
    provision_status,
    provisioned_at,
    provision_source
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('runtime_node_id')::uuid,
    sqlc.arg('provision_status'),
    sqlc.narg('provisioned_at'),
    sqlc.narg('provision_source')
)
ON CONFLICT (project_id, runtime_node_id)
DO UPDATE SET runtime_node_id = EXCLUDED.runtime_node_id
RETURNING *;

-- name: ListProjectRuntimeNodes :many
SELECT * FROM project_runtime_nodes
WHERE tenant_id = sqlc.arg('tenant_id')::uuid AND project_id = sqlc.arg('project_id')::uuid
ORDER BY created_at ASC;

-- name: MarkProjectRuntimeNodeProvisioned :one
UPDATE project_runtime_nodes
SET provision_status = 'provisioned',
    provisioned_at = sqlc.arg('provisioned_at'),
    provision_source = sqlc.arg('provision_source')
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND runtime_node_id = sqlc.arg('runtime_node_id')::uuid
RETURNING *;

-- name: RemoveProjectRuntimeNode :exec
DELETE FROM project_runtime_nodes
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND runtime_node_id = sqlc.arg('runtime_node_id')::uuid;

-- name: UpsertProjectEmployeeNodeAffinity :one
INSERT INTO project_employee_node_affinity (tenant_id, project_id, digital_employee_id, runtime_node_id)
VALUES (sqlc.arg('tenant_id')::uuid, sqlc.arg('project_id')::uuid, sqlc.arg('digital_employee_id')::uuid, sqlc.arg('runtime_node_id')::uuid)
ON CONFLICT (project_id, digital_employee_id)
DO UPDATE SET runtime_node_id = EXCLUDED.runtime_node_id, last_run_at = NOW(), updated_at = NOW()
RETURNING *;

-- name: GetProjectEmployeeNodeAffinity :one
SELECT * FROM project_employee_node_affinity
WHERE tenant_id = sqlc.arg('tenant_id')::uuid AND project_id = sqlc.arg('project_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid;

-- name: DeleteProjectRuntimeNodesForDelete :many
DELETE FROM project_runtime_nodes
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
RETURNING id;

-- name: DeleteProjectEmployeeNodeAffinitiesForDelete :many
DELETE FROM project_employee_node_affinity
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
RETURNING id;
