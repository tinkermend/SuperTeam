-- 检测门（review_gate）聚合行唯一索引：一条判据一条 review_gate verdict（project_task_id 为空，
-- judge_type=review_gate）。镜像迁移 069 的 uq_demand_verdicts_adversarial，与 human/adversarial 的
-- partial index 因 judge_type 谓词不同而互斥，三来源聚合行可在同一 criterion 上共存。

CREATE UNIQUE INDEX IF NOT EXISTS uq_demand_verdicts_review_gate
    ON demand_criterion_verdicts(tenant_id, demand_id, plan_revision_id, criterion_id)
    WHERE project_task_id IS NULL AND judge_type = 'review_gate';

COMMENT ON INDEX uq_demand_verdicts_review_gate IS '检测门聚合行唯一索引：一 criterion 一 review_gate verdict（project_task_id 为空，judge_type=review_gate）';
COMMENT ON COLUMN demand_criterion_verdicts.judge_type IS '判定来源类型：executor | human | adversarial | review_gate';
COMMENT ON TABLE demand_criterion_verdicts IS '逐条判据判定记录：executor 投影（按 project_task_id 各自一条）+ 人类签署（project_task_id 为空，全局一条）+ 对抗评审聚合（project_task_id 为空，全局一条）+ 检测门聚合（project_task_id 为空，全局一条）四来源';
