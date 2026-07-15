-- name: ListScenarioTemplates :many
SELECT * FROM scenario_templates
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
ORDER BY created_at ASC, template_key ASC;

-- name: GetScenarioTemplateByKey :one
SELECT * FROM scenario_templates
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND template_key = sqlc.arg('template_key')::text
  AND deleted_at IS NULL;

-- name: CreateScenarioTemplate :one
INSERT INTO scenario_templates (
    tenant_id, template_key, name, description, spec, status, active_version, created_by
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('template_key')::text,
    sqlc.arg('name')::text,
    COALESCE(sqlc.arg('description')::text, ''),
    sqlc.arg('spec')::jsonb,
    'active',
    1,
    sqlc.narg('created_by')::uuid
)
RETURNING *;

-- name: CreateScenarioTemplateVersion :one
INSERT INTO scenario_template_versions (
    tenant_id, template_id, version, spec, created_by
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('template_id')::uuid,
    sqlc.arg('version')::int,
    sqlc.arg('spec')::jsonb,
    sqlc.narg('created_by')::uuid
)
RETURNING *;

-- name: UpdateScenarioTemplateActiveSpec :one
UPDATE scenario_templates
SET spec = sqlc.arg('spec')::jsonb,
    active_version = sqlc.arg('active_version')::int
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL
RETURNING *;

-- name: UpdateScenarioTemplateStatus :one
UPDATE scenario_templates
SET status = sqlc.arg('status')::varchar,
    name = sqlc.arg('name')::text,
    description = sqlc.arg('description')::text
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL
RETURNING *;

-- name: ListScenarioTemplateVersions :many
SELECT * FROM scenario_template_versions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND template_id = sqlc.arg('template_id')::uuid
ORDER BY version DESC;
