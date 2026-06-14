ALTER TABLE project_tasks
    ADD COLUMN coordination_job_id UUID,
    ADD COLUMN route_decision_id UUID,
    ADD COLUMN planned_task_key VARCHAR(100),
    ADD COLUMN task_kind VARCHAR(100),
    ADD COLUMN stage_index INTEGER,
    ADD COLUMN expected_outputs JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN input_requirements JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN handoff_contract JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN planner_metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN project_tasks.coordination_job_id IS '创建该任务的项目协调作业ID，由应用层校验租户和项目范围';
COMMENT ON COLUMN project_tasks.route_decision_id IS '创建该任务的路由决策ID，由应用层校验租户和项目范围';
COMMENT ON COLUMN project_tasks.planned_task_key IS 'Planner在同一协调图内生成的稳定任务键，用于幂等重放和图展示';
COMMENT ON COLUMN project_tasks.task_kind IS '任务类型开放字符串，例如 analysis、implementation、review、test 或 summary，由应用层注册校验';
COMMENT ON COLUMN project_tasks.stage_index IS '任务在规划图中的展示阶段序号，执行事实仍以依赖边为准';
COMMENT ON COLUMN project_tasks.expected_outputs IS '任务级输出契约数组，用于完成前校验和下游交接';
COMMENT ON COLUMN project_tasks.input_requirements IS '任务输入要求JSON，描述执行该任务需要的上下文切片';
COMMENT ON COLUMN project_tasks.handoff_contract IS '任务交接契约JSON，描述下游可消费的证据、工件、结论和引用要求';
COMMENT ON COLUMN project_tasks.planner_metadata IS 'Planner审计摘要JSON，不保存长prompt或模型原文';
COMMENT ON COLUMN project_tasks.status IS '任务状态：pending, planned, blocked, assigned, running, waiting_human, completed, failed, cancelled';

CREATE UNIQUE INDEX uq_project_coordination_jobs_trigger
    ON project_coordination_jobs(tenant_id, workflow_id, trigger_event_id, job_type)
    WHERE trigger_event_id IS NOT NULL;

CREATE UNIQUE INDEX uq_project_route_decisions_job
    ON project_route_decisions(tenant_id, coordination_job_id);

CREATE TABLE project_task_dependencies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    coordination_job_id UUID,
    dependent_task_id UUID NOT NULL,
    blocker_task_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_task_dependencies IS '项目任务依赖边，记录一个任务被另一个任务完成结果阻塞的DAG关系';
COMMENT ON COLUMN project_task_dependencies.id IS '任务依赖边ID';
COMMENT ON COLUMN project_task_dependencies.tenant_id IS '租户ID';
COMMENT ON COLUMN project_task_dependencies.project_id IS '所属项目ID';
COMMENT ON COLUMN project_task_dependencies.coordination_job_id IS '生成该依赖边的协调作业ID';
COMMENT ON COLUMN project_task_dependencies.dependent_task_id IS '被阻塞的项目任务ID';
COMMENT ON COLUMN project_task_dependencies.blocker_task_id IS '必须先完成的前置项目任务ID';
COMMENT ON COLUMN project_task_dependencies.created_at IS '依赖边创建时间';

CREATE UNIQUE INDEX uq_project_tasks_coordination_planned_key
    ON project_tasks(tenant_id, project_id, coordination_job_id, planned_task_key)
    WHERE coordination_job_id IS NOT NULL AND planned_task_key IS NOT NULL;

CREATE UNIQUE INDEX uq_ptd_edge
    ON project_task_dependencies(tenant_id, dependent_task_id, blocker_task_id);

CREATE INDEX idx_ptd_tenant_project_dependent
    ON project_task_dependencies(tenant_id, project_id, dependent_task_id);

CREATE INDEX idx_ptd_blocker
    ON project_task_dependencies(tenant_id, blocker_task_id);

CREATE INDEX idx_project_tasks_coordination_job
    ON project_tasks(tenant_id, project_id, coordination_job_id, stage_index);

CREATE INDEX idx_ptd_coordination_job
    ON project_task_dependencies(tenant_id, project_id, coordination_job_id);
