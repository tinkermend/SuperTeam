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

-- name: CreateDemandCriterionVerdict :exec
INSERT INTO demand_criterion_verdicts (
    tenant_id,
    project_id,
    demand_id,
    plan_revision_id,
    criterion_id,
    verdict,
    judge_type,
    judge_id,
    reason,
    evidence_refs,
    project_task_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('demand_id')::uuid,
    sqlc.arg('plan_revision_id')::uuid,
    sqlc.arg('criterion_id')::text,
    sqlc.arg('verdict')::varchar,
    sqlc.arg('judge_type')::varchar,
    sqlc.arg('judge_id')::uuid,
    sqlc.arg('reason')::text,
    COALESCE(sqlc.narg('evidence_refs')::jsonb, '[]'::jsonb),
    sqlc.narg('project_task_id')::uuid
)
ON CONFLICT (tenant_id, demand_id, plan_revision_id, criterion_id, project_task_id)
    WHERE project_task_id IS NOT NULL
    DO NOTHING;

-- name: ListDemandCriterionVerdicts :many
SELECT * FROM demand_criterion_verdicts
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid
  AND plan_revision_id = sqlc.arg('plan_revision_id')::uuid
ORDER BY created_at ASC;
