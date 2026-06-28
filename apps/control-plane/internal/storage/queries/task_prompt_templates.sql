-- name: ListPromptTemplates :many
SELECT *
FROM task_prompt_templates
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
  AND (
        scope = 'SYSTEM'
     OR (scope = 'TEAM'     AND team_id    = ANY(sqlc.arg('team_ids')::uuid[]))
     OR (scope = 'PERSONAL' AND creator_id = sqlc.arg('user_id')::uuid)
  )
ORDER BY use_count DESC, created_at DESC;

-- name: CreatePromptTemplate :one
INSERT INTO task_prompt_templates (
    tenant_id, title, content, category_code, scope, team_id, creator_id, variables
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('title')::text,
    sqlc.arg('content')::text,
    COALESCE(sqlc.narg('category_code')::text, 'general'),
    sqlc.arg('scope')::varchar,
    sqlc.narg('team_id')::uuid,
    sqlc.arg('creator_id')::uuid,
    COALESCE(sqlc.narg('variables')::jsonb, '[]'::jsonb)
)
RETURNING *;

-- name: IncrementPromptTemplateUseCount :exec
UPDATE task_prompt_templates
SET use_count = use_count + 1, updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL;
