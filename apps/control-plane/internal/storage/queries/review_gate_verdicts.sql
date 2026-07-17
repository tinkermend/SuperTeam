-- 检测门聚合行写入：demand_criterion_verdicts, judge_type=review_gate, project_task_id 为空，
-- 一 criterion 一行。upsert 供任务重试/重跑幂等。镜像 CreateAdversarialVerdict，只是 judge_type
-- 与命中的 partial unique index 不同（uq_demand_verdicts_review_gate，见迁移 073）。

-- name: CreateReviewGateVerdict :exec
-- 检测门聚合行：judge_type 固定 review_gate、project_task_id 恒为 NULL，命中 uq_demand_verdicts_review_gate
-- 唯一索引（谓词 project_task_id IS NULL AND judge_type='review_gate'）。不能复用 CreateDemandCriterionVerdict/
-- CreateAdversarialVerdict，二者 ON CONFLICT 谓词各自只对 executor/adversarial 行去重，对本聚合行不命中。
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
    'review_gate'::varchar,
    sqlc.arg('judge_id')::uuid,
    sqlc.arg('reason')::text,
    COALESCE(sqlc.narg('evidence_refs')::jsonb, '[]'::jsonb),
    NULL
)
ON CONFLICT (tenant_id, demand_id, plan_revision_id, criterion_id)
    WHERE project_task_id IS NULL AND judge_type = 'review_gate'
    DO UPDATE SET
        verdict = EXCLUDED.verdict,
        reason = EXCLUDED.reason,
        evidence_refs = EXCLUDED.evidence_refs,
        created_at = NOW();
