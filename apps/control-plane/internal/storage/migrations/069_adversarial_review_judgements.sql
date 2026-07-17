-- 对抗判官明细表：一条 adversarial_review 判据的 N 个独立判官各出一行（血缘/可复利/审计）。
-- 同时收紧既有 uq_demand_verdicts_human 索引谓词，为 demand_criterion_verdicts 新增的
-- adversarial 聚合行唯一索引腾出互斥的 partial index 空间（human 行 judge_type='human'，
-- adversarial 行 judge_type='adversarial'，两者 project_task_id 皆为 NULL，原谓词会重叠冲突）。

DROP INDEX IF EXISTS uq_demand_verdicts_human;
CREATE UNIQUE INDEX IF NOT EXISTS uq_demand_verdicts_human
    ON demand_criterion_verdicts(tenant_id, demand_id, plan_revision_id, criterion_id)
    WHERE project_task_id IS NULL AND judge_type = 'human';

CREATE UNIQUE INDEX IF NOT EXISTS uq_demand_verdicts_adversarial
    ON demand_criterion_verdicts(tenant_id, demand_id, plan_revision_id, criterion_id)
    WHERE project_task_id IS NULL AND judge_type = 'adversarial';

COMMENT ON COLUMN demand_criterion_verdicts.judge_type IS '判定来源类型：executor | human | adversarial';
COMMENT ON TABLE demand_criterion_verdicts IS '逐条判据判定记录：executor 投影（按 project_task_id 各自一条）+ 人类签署（project_task_id 为空，全局一条）+ 对抗评审聚合（project_task_id 为空，全局一条）三来源';

CREATE TABLE IF NOT EXISTS demand_adversarial_judgements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    demand_id UUID NOT NULL,
    plan_revision_id UUID NOT NULL,
    criterion_id TEXT NOT NULL,
    reviewed_task_id UUID NOT NULL,
    lens VARCHAR(64) NOT NULL,
    verdict VARCHAR(16) NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_adversarial_judgement UNIQUE (tenant_id, demand_id, plan_revision_id, criterion_id, lens)
);

CREATE INDEX IF NOT EXISTS idx_adversarial_judgements_tenant_demand
    ON demand_adversarial_judgements(tenant_id, demand_id, plan_revision_id);

COMMENT ON TABLE demand_adversarial_judgements IS '对抗判官逐判官明细：一条 adversarial_review 判据由 N 个独立视角（lens）判官各自出一行判定，供聚合、血缘与审计使用，不替代 demand_criterion_verdicts 的聚合行';
COMMENT ON COLUMN demand_adversarial_judgements.id IS '判官明细记录主键 UUID';
COMMENT ON COLUMN demand_adversarial_judgements.tenant_id IS '判官明细所属租户 ID';
COMMENT ON COLUMN demand_adversarial_judgements.project_id IS '判官明细所属项目 ID';
COMMENT ON COLUMN demand_adversarial_judgements.demand_id IS '判官明细所属需求 ID';
COMMENT ON COLUMN demand_adversarial_judgements.plan_revision_id IS '判官明细所属的计划修订版本 ID';
COMMENT ON COLUMN demand_adversarial_judgements.criterion_id IS '被评审的判据 ID，对应 demand_acceptance_criteria.criterion_id';
COMMENT ON COLUMN demand_adversarial_judgements.reviewed_task_id IS '被该判官评审的执行任务 ID';
COMMENT ON COLUMN demand_adversarial_judgements.lens IS '判官评审视角：correctness | security | reproducibility 等，随策略可扩展，应用层校验';
COMMENT ON COLUMN demand_adversarial_judgements.verdict IS '判官判定结论：refuted（证伪）| accepted（未能证伪）';
COMMENT ON COLUMN demand_adversarial_judgements.reason IS '判官判定理由说明，可为空字符串';
COMMENT ON COLUMN demand_adversarial_judgements.evidence_refs IS '判官判定引用的证据指针列表，JSONB 数组';
COMMENT ON COLUMN demand_adversarial_judgements.created_at IS '判官明细记录创建时间';

COMMENT ON INDEX uq_demand_verdicts_adversarial IS '对抗评审聚合行唯一索引：一 criterion 一聚合 verdict（project_task_id 为空，judge_type=adversarial）';
COMMENT ON INDEX idx_adversarial_judgements_tenant_demand IS '判官明细按租户+需求+计划修订版本查询索引';
