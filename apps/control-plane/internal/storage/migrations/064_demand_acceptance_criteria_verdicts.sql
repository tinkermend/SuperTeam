-- 需求判据快照表 + 逐条 verdict 表：意图/验收判据 P1
-- 判据快照：计划批准分解时从 payload 固化，收敛闸/血缘/签署全部查表不解析 JSONB。
-- verdict 表：executor 投影 + 人类签署两来源的逐条判定记录，各自一条唯一索引防重复写入。

CREATE TABLE IF NOT EXISTS demand_acceptance_criteria (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    demand_id UUID NOT NULL,
    plan_revision_id UUID NOT NULL,
    criterion_id TEXT NOT NULL,
    statement TEXT NOT NULL,
    verification_method VARCHAR(64) NOT NULL,
    severity VARCHAR(32) NOT NULL,
    satisfied_by JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_demand_criteria_revision UNIQUE (tenant_id, demand_id, plan_revision_id, criterion_id)
);

CREATE INDEX IF NOT EXISTS idx_demand_acceptance_criteria_tenant_demand
    ON demand_acceptance_criteria(tenant_id, demand_id, plan_revision_id);

COMMENT ON TABLE demand_acceptance_criteria IS '需求判据快照：计划批准分解时从 payload 固化的验收判据，收敛闸/血缘/签署全部查表不解析 JSONB';
COMMENT ON COLUMN demand_acceptance_criteria.id IS '判据快照主键 UUID';
COMMENT ON COLUMN demand_acceptance_criteria.tenant_id IS '判据快照所属租户 ID';
COMMENT ON COLUMN demand_acceptance_criteria.project_id IS '判据快照所属项目 ID';
COMMENT ON COLUMN demand_acceptance_criteria.demand_id IS '判据快照所属需求 ID';
COMMENT ON COLUMN demand_acceptance_criteria.plan_revision_id IS '判据固化时所属的计划修订版本 ID';
COMMENT ON COLUMN demand_acceptance_criteria.criterion_id IS '判据在 payload 内的原始 ID，租户+需求+计划修订范围内唯一';
COMMENT ON COLUMN demand_acceptance_criteria.statement IS '判据的自然语言陈述：什么算做对了';
COMMENT ON COLUMN demand_acceptance_criteria.verification_method IS '判据的验证方式（如 test、review、manual_check）';
COMMENT ON COLUMN demand_acceptance_criteria.severity IS '判据严重度（如 blocking、advisory）';
COMMENT ON COLUMN demand_acceptance_criteria.satisfied_by IS '判据关联的满足来源列表（如任务/证据指针），JSONB 数组';
COMMENT ON COLUMN demand_acceptance_criteria.created_at IS '判据快照固化时间';

CREATE TABLE IF NOT EXISTS demand_criterion_verdicts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    demand_id UUID NOT NULL,
    plan_revision_id UUID NOT NULL,
    criterion_id TEXT NOT NULL,
    verdict VARCHAR(32) NOT NULL,
    judge_type VARCHAR(32) NOT NULL,
    judge_id UUID NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    project_task_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_demand_verdicts_task
    ON demand_criterion_verdicts(tenant_id, demand_id, plan_revision_id, criterion_id, project_task_id)
    WHERE project_task_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_demand_verdicts_human
    ON demand_criterion_verdicts(tenant_id, demand_id, plan_revision_id, criterion_id)
    WHERE project_task_id IS NULL;

COMMENT ON TABLE demand_criterion_verdicts IS '逐条判据判定记录：executor 投影（按 project_task_id 各自一条）+ 人类签署（project_task_id 为空，全局一条）两来源';
COMMENT ON COLUMN demand_criterion_verdicts.id IS '判定记录主键 UUID';
COMMENT ON COLUMN demand_criterion_verdicts.tenant_id IS '判定记录所属租户 ID';
COMMENT ON COLUMN demand_criterion_verdicts.project_id IS '判定记录所属项目 ID';
COMMENT ON COLUMN demand_criterion_verdicts.demand_id IS '判定记录所属需求 ID';
COMMENT ON COLUMN demand_criterion_verdicts.plan_revision_id IS '判定记录所属的计划修订版本 ID';
COMMENT ON COLUMN demand_criterion_verdicts.criterion_id IS '被判定的判据 ID，对应 demand_acceptance_criteria.criterion_id';
COMMENT ON COLUMN demand_criterion_verdicts.verdict IS '判定结论：satisfied | unsatisfied';
COMMENT ON COLUMN demand_criterion_verdicts.judge_type IS '判定来源类型：executor | human';
COMMENT ON COLUMN demand_criterion_verdicts.judge_id IS '判定人/执行者 ID（人类用户 ID 或数字员工 ID）';
COMMENT ON COLUMN demand_criterion_verdicts.reason IS '判定理由说明，可为空字符串';
COMMENT ON COLUMN demand_criterion_verdicts.evidence_refs IS '判定引用的证据指针列表，JSONB 数组';
COMMENT ON COLUMN demand_criterion_verdicts.project_task_id IS '关联的执行任务 ID；executor 投影必填，人类签署为空表示全局判定';
COMMENT ON COLUMN demand_criterion_verdicts.created_at IS '判定记录创建时间';

-- project_demands.status 追加 acceptance_pending 终态前置值：任务全部完成后进入验收待签署，
-- 人类对逐条判据签署确认后才推进至 completed，不再由任务终态直接判定需求完成。
COMMENT ON COLUMN project_demands.status IS '需求状态：submitted, recorded, planning_pending, planned, executing, acceptance_pending, completed, failed, cancelled';
