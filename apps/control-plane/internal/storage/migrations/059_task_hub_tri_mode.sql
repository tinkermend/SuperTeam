-- 059_task_hub_tri_mode.sql
-- 任务中枢三模式:run 归类(task/chat)与协调模式(plan/loop)。
-- 见 docs/superpowers/specs/2026-07-13-task-hub-tri-mode-design.md

ALTER TABLE tasks
    ADD COLUMN run_kind VARCHAR(20) NOT NULL DEFAULT 'task',
    ADD COLUMN resume_of_run_id UUID;

ALTER TABLE tasks
    ADD CONSTRAINT chk_tasks_run_kind CHECK (run_kind IN ('task', 'chat'));

CREATE INDEX idx_tasks_tenant_run_kind ON tasks (tenant_id, run_kind);

COMMENT ON COLUMN tasks.run_kind IS 'run 归类:task=任务执行,chat=数字员工单次对话。纯分类标签,不改变执行语义。';
COMMENT ON COLUMN tasks.resume_of_run_id IS 'chat 追问血缘:指向上一个 chat run(task_runs.id),仅审计用,无 FK。';

ALTER TABLE project_demands
    ADD COLUMN coordination_mode VARCHAR(10) NOT NULL DEFAULT 'plan';

ALTER TABLE project_demands
    ADD CONSTRAINT chk_project_demands_coordination_mode CHECK (coordination_mode IN ('plan', 'loop'));

COMMENT ON COLUMN project_demands.coordination_mode IS '协调模式:plan=上游阻塞时报人类决策;loop=自动补链。随需求提交,冻结进 plan revision。';

ALTER TABLE project_plan_revisions
    ADD COLUMN coordination_mode VARCHAR(10);

ALTER TABLE project_plan_revisions
    ADD CONSTRAINT chk_project_plan_revisions_coordination_mode
        CHECK (coordination_mode IS NULL OR coordination_mode IN ('plan', 'loop'));

COMMENT ON COLUMN project_plan_revisions.coordination_mode IS '从 demand 冻结的协调模式;NULL(存量)按 loop 解释。';
