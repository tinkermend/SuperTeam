CREATE TABLE project_plan_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    team_id UUID,
    project_id UUID NOT NULL,
    demand_id UUID NOT NULL,
    coordination_job_id UUID,
    route_decision_id UUID,
    revision_number INTEGER NOT NULL,
    status VARCHAR(32) NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    planner_provider VARCHAR(120),
    planner_model VARCHAR(180),
    planner_input_hash VARCHAR(128),
    plan_fingerprint VARCHAR(128) NOT NULL,
    validation_errors JSONB NOT NULL DEFAULT '[]'::jsonb,
    validation_warnings JSONB NOT NULL DEFAULT '[]'::jsonb,
    review_required BOOLEAN NOT NULL DEFAULT false,
    review_reason TEXT,
    accepted_by UUID,
    accepted_at TIMESTAMPTZ,
    rejected_by UUID,
    rejected_at TIMESTAMPTZ,
    rejection_reason TEXT,
    superseded_by_revision_id UUID,
    decomposition_claim_id UUID,
    created_task_ids UUID[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_project_plan_revisions_status CHECK (
        status IN (
            'draft',
            'validation_failed',
            'pending_review',
            'accepted',
            'rejected',
            'superseded',
            'decomposing',
            'decomposed'
        )
    )
);

CREATE UNIQUE INDEX uq_project_plan_revisions_revision_number
    ON project_plan_revisions(tenant_id, project_id, demand_id, revision_number);

CREATE UNIQUE INDEX uq_project_plan_revisions_fingerprint
    ON project_plan_revisions(tenant_id, project_id, demand_id, plan_fingerprint);

CREATE UNIQUE INDEX uq_project_plan_revisions_current_accepted
    ON project_plan_revisions(tenant_id, project_id, demand_id)
    WHERE status IN ('accepted', 'decomposing', 'decomposed');

CREATE INDEX idx_project_plan_revisions_project_status
    ON project_plan_revisions(tenant_id, project_id, status, created_at DESC);

CREATE INDEX idx_project_plan_revisions_demand_created
    ON project_plan_revisions(tenant_id, project_id, demand_id, created_at DESC);

CREATE TRIGGER update_project_plan_revisions_updated_at
    BEFORE UPDATE ON project_plan_revisions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE project_plan_decomposition_claims (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    demand_id UUID NOT NULL,
    accepted_plan_revision_id UUID NOT NULL,
    plan_fingerprint VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'in_flight',
    created_task_ids UUID[] NOT NULL DEFAULT '{}',
    error JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_project_plan_decomposition_claims_status CHECK (
        status IN ('in_flight', 'completed', 'failed')
    )
);

CREATE UNIQUE INDEX uq_project_plan_decomposition_claims_revision
    ON project_plan_decomposition_claims(tenant_id, project_id, demand_id, accepted_plan_revision_id);

CREATE INDEX idx_project_plan_decomposition_claims_status
    ON project_plan_decomposition_claims(tenant_id, project_id, status, created_at DESC);

CREATE TRIGGER update_project_plan_decomposition_claims_updated_at
    BEFORE UPDATE ON project_plan_decomposition_claims
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE project_plan_revisions IS '项目计划版本表，保存 planner 生成并经服务端校验的人类可审核计划版本。';
COMMENT ON COLUMN project_plan_revisions.id IS '计划版本ID。';
COMMENT ON COLUMN project_plan_revisions.tenant_id IS '租户ID，所有查询必须以租户隔离。';
COMMENT ON COLUMN project_plan_revisions.team_id IS '项目所属团队ID；历史项目可为空，由应用层从项目校验。';
COMMENT ON COLUMN project_plan_revisions.project_id IS '计划版本所属项目ID。';
COMMENT ON COLUMN project_plan_revisions.demand_id IS '计划版本关联的用户需求或触发事件ID。';
COMMENT ON COLUMN project_plan_revisions.coordination_job_id IS '生成该计划版本的项目协调作业ID。';
COMMENT ON COLUMN project_plan_revisions.route_decision_id IS '兼容现有路由决策读模型的关联决策ID。';
COMMENT ON COLUMN project_plan_revisions.revision_number IS '同一项目需求下从 1 开始递增的计划版本号。';
COMMENT ON COLUMN project_plan_revisions.status IS '计划版本状态：draft, validation_failed, pending_review, accepted, rejected, superseded, decomposing, decomposed。';
COMMENT ON COLUMN project_plan_revisions.payload IS '结构化 PlanRevisionPayload，不保存长 prompt 或原始模型全文。';
COMMENT ON COLUMN project_plan_revisions.planner_provider IS '生成计划的 planner provider。';
COMMENT ON COLUMN project_plan_revisions.planner_model IS '生成计划的 planner model。';
COMMENT ON COLUMN project_plan_revisions.planner_input_hash IS 'PlanningSnapshot 输入摘要 hash。';
COMMENT ON COLUMN project_plan_revisions.plan_fingerprint IS 'canonical payload hash，用于幂等与审计。';
COMMENT ON COLUMN project_plan_revisions.validation_errors IS '服务端校验 hard error 列表。';
COMMENT ON COLUMN project_plan_revisions.validation_warnings IS '服务端校验 warning 列表。';
COMMENT ON COLUMN project_plan_revisions.review_required IS '该版本是否需要人类 review。';
COMMENT ON COLUMN project_plan_revisions.review_reason IS '需要人类 review 的摘要原因。';
COMMENT ON COLUMN project_plan_revisions.accepted_by IS '接受该计划版本的人类用户ID；策略自动接受时为空。';
COMMENT ON COLUMN project_plan_revisions.accepted_at IS '计划版本被接受时间。';
COMMENT ON COLUMN project_plan_revisions.rejected_by IS '驳回该计划版本的人类用户ID。';
COMMENT ON COLUMN project_plan_revisions.rejected_at IS '计划版本被驳回时间。';
COMMENT ON COLUMN project_plan_revisions.rejection_reason IS '驳回或要求修改的原因。';
COMMENT ON COLUMN project_plan_revisions.superseded_by_revision_id IS '替代该版本的新计划版本ID。';
COMMENT ON COLUMN project_plan_revisions.decomposition_claim_id IS '分解该版本的幂等 claim ID。';
COMMENT ON COLUMN project_plan_revisions.created_task_ids IS '该计划版本分解后创建或重放的 ProjectTask ID。';
COMMENT ON COLUMN project_plan_revisions.created_at IS '计划版本创建时间。';
COMMENT ON COLUMN project_plan_revisions.updated_at IS '计划版本最近更新时间。';

COMMENT ON TABLE project_plan_decomposition_claims IS '计划版本分解幂等声明表，保证 accepted PlanRevision 精确一次转换为 ProjectTask DAG。';
COMMENT ON COLUMN project_plan_decomposition_claims.id IS '分解声明ID。';
COMMENT ON COLUMN project_plan_decomposition_claims.tenant_id IS '租户ID。';
COMMENT ON COLUMN project_plan_decomposition_claims.project_id IS '项目ID。';
COMMENT ON COLUMN project_plan_decomposition_claims.demand_id IS '需求ID。';
COMMENT ON COLUMN project_plan_decomposition_claims.accepted_plan_revision_id IS '被分解的 accepted PlanRevision ID。';
COMMENT ON COLUMN project_plan_decomposition_claims.plan_fingerprint IS '被分解计划的 canonical hash。';
COMMENT ON COLUMN project_plan_decomposition_claims.status IS '分解状态：in_flight, completed, failed。';
COMMENT ON COLUMN project_plan_decomposition_claims.created_task_ids IS '分解成功或恢复后对应的 ProjectTask ID。';
COMMENT ON COLUMN project_plan_decomposition_claims.error IS '分解失败时记录的结构化错误。';
COMMENT ON COLUMN project_plan_decomposition_claims.created_at IS '分解声明创建时间。';
COMMENT ON COLUMN project_plan_decomposition_claims.updated_at IS '分解声明最近更新时间。';
