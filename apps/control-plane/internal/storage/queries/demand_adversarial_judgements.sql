-- 对抗判官投影写入：聚合行（demand_criterion_verdicts, judge_type=adversarial, project_task_id 为空，
-- 一 criterion 一行）+ 逐判官明细行（demand_adversarial_judgements, 一 lens 一行）。
-- 二者均 upsert，供任务重试幂等重跑。

-- name: CreateAdversarialVerdict :exec
-- 对抗评审聚合行：judge_type 固定 adversarial、project_task_id 恒为 NULL，命中 uq_demand_verdicts_adversarial
-- 唯一索引（谓词 project_task_id IS NULL AND judge_type='adversarial'）。不能复用 CreateDemandCriterionVerdict，
-- 后者 ON CONFLICT 只对 project_task_id IS NOT NULL 的 executor 行去重，对本聚合行不命中。
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
    'adversarial'::varchar,
    sqlc.arg('judge_id')::uuid,
    sqlc.arg('reason')::text,
    COALESCE(sqlc.narg('evidence_refs')::jsonb, '[]'::jsonb),
    NULL
)
ON CONFLICT (tenant_id, demand_id, plan_revision_id, criterion_id)
    WHERE project_task_id IS NULL AND judge_type = 'adversarial'
    DO UPDATE SET
        verdict = EXCLUDED.verdict,
        reason = EXCLUDED.reason,
        evidence_refs = EXCLUDED.evidence_refs,
        created_at = NOW();

-- name: CreateAdversarialJudgement :exec
-- 单个判官明细行：一 lens 一行，ON CONFLICT 命中 uq_adversarial_judgement
-- (tenant_id, demand_id, plan_revision_id, criterion_id, lens)，重跑覆盖同 lens 判定。
INSERT INTO demand_adversarial_judgements (
    tenant_id,
    project_id,
    demand_id,
    plan_revision_id,
    criterion_id,
    reviewed_task_id,
    lens,
    verdict,
    reason,
    evidence_refs
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('demand_id')::uuid,
    sqlc.arg('plan_revision_id')::uuid,
    sqlc.arg('criterion_id')::text,
    sqlc.arg('reviewed_task_id')::uuid,
    sqlc.arg('lens')::varchar,
    sqlc.arg('verdict')::varchar,
    sqlc.arg('reason')::text,
    COALESCE(sqlc.narg('evidence_refs')::jsonb, '[]'::jsonb)
)
ON CONFLICT (tenant_id, demand_id, plan_revision_id, criterion_id, lens)
    DO UPDATE SET
        reviewed_task_id = EXCLUDED.reviewed_task_id,
        verdict = EXCLUDED.verdict,
        reason = EXCLUDED.reason,
        evidence_refs = EXCLUDED.evidence_refs,
        created_at = NOW();

-- name: ListAdversarialJudgements :many
-- 逐判官明细回读：血缘/审计面板按租户+需求+计划修订版本列出全部 lens 判定。
SELECT * FROM demand_adversarial_judgements
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid
  AND plan_revision_id = sqlc.arg('plan_revision_id')::uuid
ORDER BY created_at ASC, id ASC;
