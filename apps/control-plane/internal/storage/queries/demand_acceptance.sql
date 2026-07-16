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
-- id 次键让排序全序化：收敛闸的"最新人类判定优先"用切片顺序（最后一条覆盖）实现，
-- created_at 相等时若无次键则两条对立人类判定可任意翻转计数——(created_at, id) 使
-- "最后一条人类判定"成为确定函数（同刻取更大 id）。
SELECT * FROM demand_criterion_verdicts
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid
  AND plan_revision_id = sqlc.arg('plan_revision_id')::uuid
ORDER BY created_at ASC, id ASC;
