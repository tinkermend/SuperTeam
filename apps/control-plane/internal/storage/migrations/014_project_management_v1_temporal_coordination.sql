-- 014_project_management_v1_temporal_coordination.sql
-- 项目管理 V1：Temporal 协调、路由决策、执行回写、转派请求和人类决策投影

CREATE TABLE approval_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    resource_id UUID NOT NULL,
    requester_type VARCHAR(50) NOT NULL,
    requester_id UUID,
    target_user_id UUID NOT NULL,
    decision_type VARCHAR(100) NOT NULL,
    title VARCHAR(255) NOT NULL,
    summary TEXT,
    risk_level VARCHAR(50),
    status VARCHAR(50) NOT NULL,
    options JSONB NOT NULL DEFAULT '[]'::jsonb,
    context_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);

COMMENT ON TABLE approval_requests IS '全局审批请求事实表，保存人类决策请求的事实源';
COMMENT ON COLUMN approval_requests.id IS '审批请求ID';
COMMENT ON COLUMN approval_requests.tenant_id IS '租户ID';
COMMENT ON COLUMN approval_requests.resource_type IS '审批关联资源类型，由应用层校验';
COMMENT ON COLUMN approval_requests.resource_id IS '审批关联资源ID';
COMMENT ON COLUMN approval_requests.requester_type IS '发起者类型，例如 project_coordinator、human_user 或 system';
COMMENT ON COLUMN approval_requests.requester_id IS '发起者ID，可为空表示系统发起';
COMMENT ON COLUMN approval_requests.target_user_id IS '目标处理人用户ID';
COMMENT ON COLUMN approval_requests.decision_type IS '决策类型，由应用层注册和校验';
COMMENT ON COLUMN approval_requests.title IS '审批标题快照';
COMMENT ON COLUMN approval_requests.summary IS '审批摘要快照';
COMMENT ON COLUMN approval_requests.risk_level IS '风险等级快照';
COMMENT ON COLUMN approval_requests.status IS '审批状态：pending、approved、rejected、needs_more_evidence、cancelled';
COMMENT ON COLUMN approval_requests.options IS '可选决策项 JSON 数组';
COMMENT ON COLUMN approval_requests.context_payload IS '审批上下文快照';
COMMENT ON COLUMN approval_requests.created_at IS '审批创建时间';
COMMENT ON COLUMN approval_requests.updated_at IS '审批更新时间';
COMMENT ON COLUMN approval_requests.resolved_at IS '审批处理完成时间';

CREATE INDEX idx_approval_requests_tenant_status_created ON approval_requests(tenant_id, status, created_at DESC);
CREATE INDEX idx_approval_requests_tenant_resource ON approval_requests(tenant_id, resource_type, resource_id);
CREATE TRIGGER update_approval_requests_updated_at BEFORE UPDATE ON approval_requests FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE approval_decisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    approval_request_id UUID NOT NULL,
    decided_by_user_id UUID NOT NULL,
    decision VARCHAR(100) NOT NULL,
    comment TEXT,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE approval_decisions IS '全局审批处理记录表，保存人类处理动作和意见';
COMMENT ON COLUMN approval_decisions.id IS '审批处理记录ID';
COMMENT ON COLUMN approval_decisions.tenant_id IS '租户ID';
COMMENT ON COLUMN approval_decisions.approval_request_id IS '关联审批请求ID，由应用层校验租户和状态';
COMMENT ON COLUMN approval_decisions.decided_by_user_id IS '处理审批的人类用户ID';
COMMENT ON COLUMN approval_decisions.decision IS '处理结论：approved、rejected、needs_more_evidence';
COMMENT ON COLUMN approval_decisions.comment IS '处理意见';
COMMENT ON COLUMN approval_decisions.payload IS '处理时提交的结构化补充信息';
COMMENT ON COLUMN approval_decisions.created_at IS '处理记录创建时间';

CREATE INDEX idx_approval_decisions_tenant_request_created ON approval_decisions(tenant_id, approval_request_id, created_at DESC);

CREATE TABLE project_coordination_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    workflow_id VARCHAR(255) NOT NULL,
    trigger_event_id UUID,
    job_type VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL,
    input_snapshot_ref JSONB NOT NULL DEFAULT '{}'::jsonb,
    output_event_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_coordination_jobs IS '项目协调作业记录，追踪一次 Workflow 协调决策的输入、状态和输出事件';
COMMENT ON COLUMN project_coordination_jobs.id IS '协调作业ID';
COMMENT ON COLUMN project_coordination_jobs.tenant_id IS '租户ID';
COMMENT ON COLUMN project_coordination_jobs.project_id IS '所属项目ID';
COMMENT ON COLUMN project_coordination_jobs.workflow_id IS 'Temporal Workflow ID';
COMMENT ON COLUMN project_coordination_jobs.trigger_event_id IS '触发该作业的项目事件ID';
COMMENT ON COLUMN project_coordination_jobs.job_type IS '协调作业类型，例如 demand_route、transfer_review、human_decision';
COMMENT ON COLUMN project_coordination_jobs.status IS '协调作业状态：running、completed、failed、noop';
COMMENT ON COLUMN project_coordination_jobs.input_snapshot_ref IS '输入快照引用或小型快照 JSON';
COMMENT ON COLUMN project_coordination_jobs.output_event_ids IS '该作业产生的项目事件ID列表';
COMMENT ON COLUMN project_coordination_jobs.started_at IS '协调作业开始时间';
COMMENT ON COLUMN project_coordination_jobs.finished_at IS '协调作业结束时间';
COMMENT ON COLUMN project_coordination_jobs.created_at IS '协调作业创建时间';

CREATE INDEX idx_project_coordination_jobs_tenant_project_created ON project_coordination_jobs(tenant_id, project_id, created_at DESC);
CREATE INDEX idx_project_coordination_jobs_tenant_workflow ON project_coordination_jobs(tenant_id, workflow_id, created_at DESC);

CREATE TABLE project_route_decisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    coordination_job_id UUID NOT NULL,
    demand_id UUID,
    candidate_digital_employee_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    selected_digital_employee_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    reason TEXT NOT NULL,
    input_requirements JSONB NOT NULL DEFAULT '{}'::jsonb,
    expected_outputs JSONB NOT NULL DEFAULT '[]'::jsonb,
    budget_estimate JSONB NOT NULL DEFAULT '{}'::jsonb,
    requires_human_review BOOLEAN NOT NULL DEFAULT false,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_route_decisions IS '项目需求路由决策表，保存虚拟协调线程选择执行员工和输出契约的结构化结论';
COMMENT ON COLUMN project_route_decisions.id IS '路由决策ID';
COMMENT ON COLUMN project_route_decisions.tenant_id IS '租户ID';
COMMENT ON COLUMN project_route_decisions.project_id IS '所属项目ID';
COMMENT ON COLUMN project_route_decisions.coordination_job_id IS '产生该决策的协调作业ID';
COMMENT ON COLUMN project_route_decisions.demand_id IS '关联需求ID';
COMMENT ON COLUMN project_route_decisions.candidate_digital_employee_ids IS '候选数字员工ID数组';
COMMENT ON COLUMN project_route_decisions.selected_digital_employee_ids IS '选中的数字员工ID数组';
COMMENT ON COLUMN project_route_decisions.reason IS '路由理由';
COMMENT ON COLUMN project_route_decisions.input_requirements IS '任务输入要求';
COMMENT ON COLUMN project_route_decisions.expected_outputs IS '期望输出契约数组';
COMMENT ON COLUMN project_route_decisions.budget_estimate IS '预算估算快照';
COMMENT ON COLUMN project_route_decisions.requires_human_review IS '是否需要人类先审核该决策';
COMMENT ON COLUMN project_route_decisions.created_event_id IS '创建该决策时产生的项目事件ID';
COMMENT ON COLUMN project_route_decisions.created_at IS '路由决策创建时间';

CREATE INDEX idx_project_route_decisions_tenant_project_created ON project_route_decisions(tenant_id, project_id, created_at DESC);
CREATE INDEX idx_project_route_decisions_tenant_demand ON project_route_decisions(tenant_id, demand_id);
CREATE INDEX idx_project_route_decisions_tenant_job ON project_route_decisions(tenant_id, coordination_job_id);

CREATE TABLE project_execution_summaries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    project_task_id UUID NOT NULL,
    digital_employee_id UUID NOT NULL,
    conclusion TEXT NOT NULL,
    evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    artifact_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    confidence_factors JSONB NOT NULL DEFAULT '{}'::jsonb,
    uncertainty TEXT,
    missing_information JSONB NOT NULL DEFAULT '[]'::jsonb,
    recommended_next_action TEXT,
    requires_human_review BOOLEAN NOT NULL DEFAULT false,
    transfer_request_id UUID,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_execution_summaries IS '项目任务执行摘要表，保存数字员工回写的结论、证据、工件和不确定性';
COMMENT ON COLUMN project_execution_summaries.id IS '执行摘要ID';
COMMENT ON COLUMN project_execution_summaries.tenant_id IS '租户ID';
COMMENT ON COLUMN project_execution_summaries.project_id IS '所属项目ID';
COMMENT ON COLUMN project_execution_summaries.project_task_id IS '关联项目任务ID';
COMMENT ON COLUMN project_execution_summaries.digital_employee_id IS '回写摘要的数字员工ID';
COMMENT ON COLUMN project_execution_summaries.conclusion IS '执行结论';
COMMENT ON COLUMN project_execution_summaries.evidence_refs IS '证据引用数组';
COMMENT ON COLUMN project_execution_summaries.artifact_refs IS '工件引用数组';
COMMENT ON COLUMN project_execution_summaries.confidence_factors IS '置信度因素快照';
COMMENT ON COLUMN project_execution_summaries.uncertainty IS '不确定性说明';
COMMENT ON COLUMN project_execution_summaries.missing_information IS '缺失信息数组';
COMMENT ON COLUMN project_execution_summaries.recommended_next_action IS '建议下一步动作';
COMMENT ON COLUMN project_execution_summaries.requires_human_review IS '是否需要人类复核';
COMMENT ON COLUMN project_execution_summaries.transfer_request_id IS '关联转派请求ID';
COMMENT ON COLUMN project_execution_summaries.created_event_id IS '创建该摘要时产生的项目事件ID';
COMMENT ON COLUMN project_execution_summaries.created_at IS '执行摘要创建时间';

CREATE INDEX idx_project_execution_summaries_tenant_project_created ON project_execution_summaries(tenant_id, project_id, created_at DESC);
CREATE INDEX idx_project_execution_summaries_tenant_task ON project_execution_summaries(tenant_id, project_task_id);
CREATE INDEX idx_project_execution_summaries_tenant_employee ON project_execution_summaries(tenant_id, digital_employee_id, created_at DESC);

CREATE TABLE project_transfer_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    project_task_id UUID NOT NULL,
    requested_by_digital_employee_id UUID NOT NULL,
    reason TEXT NOT NULL,
    suggested_employee_type VARCHAR(100),
    suggested_digital_employee_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    missing_context_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    status VARCHAR(50) NOT NULL,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_transfer_requests IS '项目任务转派请求表，保存数字员工发起的结构化转派事实';
COMMENT ON COLUMN project_transfer_requests.id IS '转派请求ID';
COMMENT ON COLUMN project_transfer_requests.tenant_id IS '租户ID';
COMMENT ON COLUMN project_transfer_requests.project_id IS '所属项目ID';
COMMENT ON COLUMN project_transfer_requests.project_task_id IS '关联项目任务ID';
COMMENT ON COLUMN project_transfer_requests.requested_by_digital_employee_id IS '发起转派请求的数字员工ID';
COMMENT ON COLUMN project_transfer_requests.reason IS '转派理由';
COMMENT ON COLUMN project_transfer_requests.suggested_employee_type IS '建议员工类型';
COMMENT ON COLUMN project_transfer_requests.suggested_digital_employee_ids IS '建议数字员工ID数组';
COMMENT ON COLUMN project_transfer_requests.missing_context_refs IS '缺失上下文引用数组';
COMMENT ON COLUMN project_transfer_requests.status IS '转派请求状态：requested、accepted、rejected、cancelled';
COMMENT ON COLUMN project_transfer_requests.created_event_id IS '创建该请求时产生的项目事件ID';
COMMENT ON COLUMN project_transfer_requests.created_at IS '转派请求创建时间';
COMMENT ON COLUMN project_transfer_requests.updated_at IS '转派请求更新时间';

CREATE INDEX idx_project_transfer_requests_tenant_project_status ON project_transfer_requests(tenant_id, project_id, status, created_at DESC);
CREATE INDEX idx_project_transfer_requests_tenant_task ON project_transfer_requests(tenant_id, project_task_id);
CREATE INDEX idx_project_transfer_requests_tenant_requester ON project_transfer_requests(tenant_id, requested_by_digital_employee_id, created_at DESC);
CREATE TRIGGER update_project_transfer_requests_updated_at BEFORE UPDATE ON project_transfer_requests FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE project_decision_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    approval_request_id UUID NOT NULL,
    coordination_job_id UUID,
    project_task_id UUID,
    target_user_id UUID NOT NULL,
    decision_type VARCHAR(100) NOT NULL,
    title_snapshot VARCHAR(255) NOT NULL,
    summary_snapshot TEXT,
    risk_level_snapshot VARCHAR(50),
    status_snapshot VARCHAR(50) NOT NULL,
    created_event_id UUID,
    resolved_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);

COMMENT ON TABLE project_decision_requests IS '项目侧人类决策查询投影，审批事实源归 approval_requests 与 approval_decisions';
COMMENT ON COLUMN project_decision_requests.id IS '项目决策请求投影ID';
COMMENT ON COLUMN project_decision_requests.tenant_id IS '租户ID';
COMMENT ON COLUMN project_decision_requests.project_id IS '所属项目ID';
COMMENT ON COLUMN project_decision_requests.approval_request_id IS '全局审批请求ID，审批事实源引用';
COMMENT ON COLUMN project_decision_requests.coordination_job_id IS '关联协调作业ID';
COMMENT ON COLUMN project_decision_requests.project_task_id IS '关联项目任务ID';
COMMENT ON COLUMN project_decision_requests.target_user_id IS '目标处理人用户ID';
COMMENT ON COLUMN project_decision_requests.decision_type IS '决策类型';
COMMENT ON COLUMN project_decision_requests.title_snapshot IS '决策标题快照';
COMMENT ON COLUMN project_decision_requests.summary_snapshot IS '决策摘要快照';
COMMENT ON COLUMN project_decision_requests.risk_level_snapshot IS '风险等级快照';
COMMENT ON COLUMN project_decision_requests.status_snapshot IS '审批状态快照';
COMMENT ON COLUMN project_decision_requests.created_event_id IS '创建该投影时产生的项目事件ID';
COMMENT ON COLUMN project_decision_requests.resolved_event_id IS '处理该投影时产生的项目事件ID';
COMMENT ON COLUMN project_decision_requests.created_at IS '投影创建时间';
COMMENT ON COLUMN project_decision_requests.updated_at IS '投影更新时间';
COMMENT ON COLUMN project_decision_requests.resolved_at IS '投影处理完成时间';

CREATE INDEX idx_project_decision_requests_tenant_project_status ON project_decision_requests(tenant_id, project_id, status_snapshot, created_at DESC);
CREATE INDEX idx_project_decision_requests_tenant_approval ON project_decision_requests(tenant_id, approval_request_id);
CREATE INDEX idx_project_decision_requests_tenant_target ON project_decision_requests(tenant_id, target_user_id, status_snapshot, created_at DESC);
CREATE TRIGGER update_project_decision_requests_updated_at BEFORE UPDATE ON project_decision_requests FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
