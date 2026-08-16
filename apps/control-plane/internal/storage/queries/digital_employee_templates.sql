-- apps/control-plane/internal/storage/queries/digital_employee_templates.sql

-- name: ListEmployeeTemplates :many
SELECT * FROM digital_employee_templates
WHERE tenant_id = $1 AND deleted_at IS NULL
ORDER BY created_at ASC;

-- name: GetEmployeeTemplateByID :one
SELECT * FROM digital_employee_templates
WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: GetEmployeeTemplateByType :one
SELECT * FROM digital_employee_templates
WHERE tenant_id = $1 AND type = $2 AND deleted_at IS NULL;

-- name: CreateEmployeeTemplate :one
INSERT INTO digital_employee_templates (
  tenant_id, type, label, description, default_role, default_role_keys,
  recommended_skills, recommended_mcp_servers, recommended_provider_types,
  persona_memory_markdown, capability_bindings, budget_policy,
  metadata, status, is_system
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, 'active', false
)
RETURNING *;

-- name: UpdateEmployeeTemplate :one
UPDATE digital_employee_templates SET
  label = $3,
  description = $4,
  default_role = $5,
  default_role_keys = $6,
  recommended_skills = $7,
  recommended_mcp_servers = $8,
  recommended_provider_types = $9,
  persona_memory_markdown = $10,
  capability_bindings = $11,
  budget_policy = $12,
  metadata = $13,
  updated_at = now()
WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: SetEmployeeTemplateStatus :one
UPDATE digital_employee_templates SET
  status = $3,
  updated_at = now()
WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteEmployeeTemplate :execrows
UPDATE digital_employee_templates SET
  deleted_at = now(),
  updated_at = now()
WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: ListEmployeeTemplateLabels :many
SELECT type, label FROM digital_employee_templates
WHERE tenant_id = $1;
