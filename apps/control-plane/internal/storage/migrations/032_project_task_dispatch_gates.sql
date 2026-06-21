-- Supports the gate table FK that verifies the stored project_id matches the
-- referenced task's project_id.
CREATE UNIQUE INDEX uq_project_tasks_tenant_project_task_id
    ON project_tasks(tenant_id, project_id, id);

CREATE TABLE project_task_dispatch_gate_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    project_task_id UUID NOT NULL,
    accepted_plan_revision_id UUID,
    planned_task_key VARCHAR(100),
    selected_employee_id UUID NOT NULL,
    attempt_no INTEGER NOT NULL,
    dispatch_reason VARCHAR(80) NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL,
    dispatch_token VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL,
    checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    checks JSONB NOT NULL DEFAULT '[]'::jsonb,
    blockers JSONB NOT NULL DEFAULT '[]'::jsonb,
    human_action_request JSONB NOT NULL DEFAULT '{}'::jsonb,
    retry_after TIMESTAMPTZ,
    attempt_id UUID,
    decision_request_id UUID,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_project_task_dispatch_gate_results_task
        FOREIGN KEY (tenant_id, project_id, project_task_id) REFERENCES project_tasks(tenant_id, project_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_project_task_dispatch_gate_results_status CHECK (
        status IN ('passed', 'waiting_human', 'blocked', 'retry_later', 'replan_required')
    )
);

CREATE UNIQUE INDEX uq_project_task_dispatch_gate_results_key
    ON project_task_dispatch_gate_results(tenant_id, project_task_id, idempotency_key);

-- Required target for the composite FKs added below from project_task_attempts and
-- project_tasks. PostgreSQL requires the referenced column set to be backed by a unique
-- constraint or unique index; the primary key on (id) alone is not sufficient for a
-- three-column reference.
CREATE UNIQUE INDEX uq_project_task_dispatch_gate_results_tenant_task_id
    ON project_task_dispatch_gate_results(tenant_id, project_task_id, id);

-- Required target for the project-scoped FK from project_decision_requests.
CREATE UNIQUE INDEX uq_project_task_dispatch_gate_results_tenant_project_id
    ON project_task_dispatch_gate_results(tenant_id, project_id, id);

CREATE INDEX idx_project_task_dispatch_gate_results_task_created
    ON project_task_dispatch_gate_results(tenant_id, project_id, project_task_id, created_at DESC);

CREATE INDEX idx_project_task_dispatch_gate_results_status
    ON project_task_dispatch_gate_results(tenant_id, project_id, status, created_at DESC);

CREATE INDEX idx_project_task_dispatch_gate_results_decision
    ON project_task_dispatch_gate_results(tenant_id, decision_request_id)
    WHERE decision_request_id IS NOT NULL;

ALTER TABLE project_task_attempts
    ADD COLUMN dispatch_gate_result_id UUID;

ALTER TABLE project_task_attempts
    ADD CONSTRAINT fk_project_task_attempts_dispatch_gate
    FOREIGN KEY (tenant_id, project_task_id, dispatch_gate_result_id)
    REFERENCES project_task_dispatch_gate_results(tenant_id, project_task_id, id);

CREATE INDEX idx_project_task_attempts_dispatch_gate
    ON project_task_attempts(tenant_id, dispatch_gate_result_id)
    WHERE dispatch_gate_result_id IS NOT NULL;

ALTER TABLE project_tasks
    ADD COLUMN latest_dispatch_gate_result_id UUID;

ALTER TABLE project_tasks
    ADD CONSTRAINT fk_project_tasks_latest_dispatch_gate
    FOREIGN KEY (tenant_id, id, latest_dispatch_gate_result_id)
    REFERENCES project_task_dispatch_gate_results(tenant_id, project_task_id, id);

CREATE INDEX idx_project_tasks_latest_dispatch_gate
    ON project_tasks(tenant_id, latest_dispatch_gate_result_id)
    WHERE latest_dispatch_gate_result_id IS NOT NULL;

ALTER TABLE project_decision_requests
    ADD COLUMN dispatch_gate_result_id UUID;

ALTER TABLE project_decision_requests
    ADD CONSTRAINT fk_project_decision_requests_dispatch_gate
    FOREIGN KEY (tenant_id, project_id, dispatch_gate_result_id)
    REFERENCES project_task_dispatch_gate_results(tenant_id, project_id, id);

CREATE INDEX idx_project_decision_requests_dispatch_gate
    ON project_decision_requests(tenant_id, dispatch_gate_result_id)
    WHERE dispatch_gate_result_id IS NOT NULL;

CREATE TRIGGER update_project_task_dispatch_gate_results_updated_at
    BEFORE UPDATE ON project_task_dispatch_gate_results
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE project_task_dispatch_gate_results IS 'ProjectTask 分派 Runtime 前的 Control Plane gate 结果，保存可审计的检查摘要、阻塞原因和后续 attempt 或人类请求引用。';
COMMENT ON COLUMN project_task_dispatch_gate_results.id IS 'Gate 结果ID。';
COMMENT ON COLUMN project_task_dispatch_gate_results.tenant_id IS '租户ID。';
COMMENT ON COLUMN project_task_dispatch_gate_results.project_id IS '项目ID。';
COMMENT ON COLUMN project_task_dispatch_gate_results.project_task_id IS '被检查的 ProjectTask ID。';
COMMENT ON COLUMN project_task_dispatch_gate_results.accepted_plan_revision_id IS '生成任务的 accepted PlanRevision ID。';
COMMENT ON COLUMN project_task_dispatch_gate_results.planned_task_key IS 'PlanRevision payload 内稳定任务键。';
COMMENT ON COLUMN project_task_dispatch_gate_results.selected_employee_id IS 'Gate 检查时的被选数字员工ID。';
COMMENT ON COLUMN project_task_dispatch_gate_results.attempt_no IS '本次 gate 预期创建的执行尝试序号。';
COMMENT ON COLUMN project_task_dispatch_gate_results.dispatch_reason IS '触发 gate 的原因，例如 root_ready、dependency_unlocked、human_resolved、retry。';
COMMENT ON COLUMN project_task_dispatch_gate_results.idempotency_key IS 'Gate 幂等键，同一任务同一分派原因和尝试序号只保留一个结果。';
COMMENT ON COLUMN project_task_dispatch_gate_results.dispatch_token IS '通过 gate 后交给 dispatch 的稳定 token，不包含密钥。';
COMMENT ON COLUMN project_task_dispatch_gate_results.status IS 'Gate 状态：passed, waiting_human, blocked, retry_later, replan_required。';
COMMENT ON COLUMN project_task_dispatch_gate_results.checked_at IS 'Gate 检查时间。';
COMMENT ON COLUMN project_task_dispatch_gate_results.checks IS '检查项摘要 JSON 数组，禁止保存密钥、完整连接串、敏感 SQL 或完整日志。';
COMMENT ON COLUMN project_task_dispatch_gate_results.blockers IS '阻塞原因 JSON 数组，禁止保存密钥、完整连接串、敏感 SQL 或完整日志。';
COMMENT ON COLUMN project_task_dispatch_gate_results.human_action_request IS '需要人类动作时的请求摘要，不作为审批事实源。';
COMMENT ON COLUMN project_task_dispatch_gate_results.retry_after IS '暂态不满足时建议下次 gate 时间。';
COMMENT ON COLUMN project_task_dispatch_gate_results.attempt_id IS 'Gate 通过后创建的 ProjectTaskAttempt ID。';
COMMENT ON COLUMN project_task_dispatch_gate_results.decision_request_id IS 'Gate 创建的人类决策请求投影ID。';
COMMENT ON COLUMN project_task_dispatch_gate_results.created_event_id IS '记录该 gate 结果时产生的项目事件ID。';
COMMENT ON COLUMN project_task_dispatch_gate_results.created_at IS 'Gate 结果创建时间。';
COMMENT ON COLUMN project_task_dispatch_gate_results.updated_at IS 'Gate 结果最近更新时间。';
COMMENT ON COLUMN project_task_attempts.dispatch_gate_result_id IS '创建该尝试前通过的 gate 结果ID。';
COMMENT ON COLUMN project_tasks.latest_dispatch_gate_result_id IS '该任务最近一次 gate 结果ID。';
COMMENT ON COLUMN project_decision_requests.dispatch_gate_result_id IS '该人类决策由哪个 dispatch gate 结果创建。';
