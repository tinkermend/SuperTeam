-- name: CreateDemandAcceptanceCriterion :exec
INSERT INTO demand_acceptance_criteria (
    tenant_id,
    project_id,
    demand_id,
    plan_revision_id,
    criterion_id,
    statement,
    verification_method,
    severity,
    satisfied_by
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('demand_id')::uuid,
    sqlc.arg('plan_revision_id')::uuid,
    sqlc.arg('criterion_id')::text,
    sqlc.arg('statement')::text,
    sqlc.arg('verification_method')::varchar,
    sqlc.arg('severity')::varchar,
    COALESCE(sqlc.narg('satisfied_by')::jsonb, '[]'::jsonb)
)
ON CONFLICT (tenant_id, demand_id, plan_revision_id, criterion_id) DO NOTHING;

-- name: ListDemandAcceptanceCriteria :many
SELECT * FROM demand_acceptance_criteria
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid
  AND plan_revision_id = sqlc.arg('plan_revision_id')::uuid
ORDER BY created_at ASC;
