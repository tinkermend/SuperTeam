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
