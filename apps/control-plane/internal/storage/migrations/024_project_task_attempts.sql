ALTER TABLE project_tasks
    ADD COLUMN current_attempt_id UUID,
    ADD COLUMN accepted_plan_revision_id UUID,
    ADD COLUMN decomposition_claim_key VARCHAR(255),
    ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN max_attempts INTEGER,
    ADD COLUMN retry_not_before TIMESTAMPTZ,
    ADD COLUMN waiting_reason VARCHAR(100),
    ADD COLUMN waiting_request_id UUID,
    ADD COLUMN terminal_reason VARCHAR(100),
    ADD COLUMN terminal_event_id UUID,
    ADD COLUMN cancelled_by VARCHAR(100),
    ADD COLUMN failed_by VARCHAR(100),
    ADD COLUMN status_changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

UPDATE project_tasks
SET status = 'queued',
    status_changed_at = NOW()
WHERE status = 'assigned';

CREATE UNIQUE INDEX uq_project_tasks_tenant_id
    ON project_tasks(tenant_id, id);

CREATE TABLE project_task_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_task_id UUID NOT NULL,
    attempt_no INTEGER NOT NULL,
    status VARCHAR(50) NOT NULL,
    digital_employee_run_id UUID,
    runtime_task_id UUID,
    runtime_node_id UUID,
    provider_session_id VARCHAR(255),
    execution_context_packet JSONB NOT NULL DEFAULT '{}'::jsonb,
    execution_context_packet_version VARCHAR(50) NOT NULL DEFAULT 'v1',
    lease_token VARCHAR(255) NOT NULL,
    lease_expires_at TIMESTAMPTZ,
    renewed_at TIMESTAMPTZ,
    lost_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    timeout_at TIMESTAMPTZ,
    retryable BOOLEAN,
    failure_family VARCHAR(100),
    failure_message TEXT,
    idempotency_key VARCHAR(255) NOT NULL,
    created_event_id UUID REFERENCES project_events(id) ON DELETE SET NULL,
    terminal_event_id UUID REFERENCES project_events(id) ON DELETE SET NULL,
    CONSTRAINT fk_project_task_attempts_project_task
        FOREIGN KEY (tenant_id, project_task_id) REFERENCES project_tasks(tenant_id, id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_project_task_attempts_status CHECK (
        status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled', 'lost', 'timed_out', 'waiting_human')
    )
);

CREATE UNIQUE INDEX uq_project_task_attempts_task_attempt_no
    ON project_task_attempts(tenant_id, project_task_id, attempt_no);

CREATE UNIQUE INDEX uq_project_task_attempts_idempotency_key
    ON project_task_attempts(tenant_id, idempotency_key);

CREATE UNIQUE INDEX uq_project_task_attempts_tenant_task_id
    ON project_task_attempts(tenant_id, project_task_id, id);

CREATE INDEX idx_project_task_attempts_task_status
    ON project_task_attempts(tenant_id, project_task_id, status);

CREATE UNIQUE INDEX uq_project_task_attempts_active
    ON project_task_attempts(tenant_id, project_task_id)
    WHERE status IN ('queued', 'running', 'waiting_human');

ALTER TABLE project_tasks
    ADD CONSTRAINT fk_project_tasks_current_attempt
    FOREIGN KEY (tenant_id, id, current_attempt_id)
    REFERENCES project_task_attempts(tenant_id, project_task_id, id);

CREATE INDEX idx_project_tasks_current_attempt
    ON project_tasks(tenant_id, current_attempt_id)
    WHERE current_attempt_id IS NOT NULL;

CREATE UNIQUE INDEX uq_project_tasks_accepted_plan_decomposition
    ON project_tasks(tenant_id, project_id, demand_id, accepted_plan_revision_id, planned_task_key)
    WHERE accepted_plan_revision_id IS NOT NULL
      AND demand_id IS NOT NULL
      AND planned_task_key IS NOT NULL;

COMMENT ON COLUMN project_tasks.status IS '任务状态：planned, queued, running, waiting_human, completed, failed, cancelled。';
COMMENT ON COLUMN project_tasks.current_attempt_id IS '当前项目任务执行尝试ID，指向当前 queued、running 或 waiting_human 尝试。';
COMMENT ON COLUMN project_tasks.accepted_plan_revision_id IS '生成该任务的已接受计划版本ID。';
COMMENT ON COLUMN project_tasks.decomposition_claim_key IS '计划分解幂等声明键，用于同一计划分解重复写入去重。';
COMMENT ON COLUMN project_tasks.attempt_count IS '任务执行尝试次数。';
COMMENT ON COLUMN project_tasks.max_attempts IS '任务允许的最大执行尝试次数。';
COMMENT ON COLUMN project_tasks.retry_not_before IS '任务下次可重试时间。';
COMMENT ON COLUMN project_tasks.waiting_reason IS '任务等待人类或外部条件的原因。';
COMMENT ON COLUMN project_tasks.waiting_request_id IS '任务等待的人类决策或请求ID。';
COMMENT ON COLUMN project_tasks.terminal_reason IS '任务进入终态的原因。';
COMMENT ON COLUMN project_tasks.terminal_event_id IS '任务进入终态时写入的项目事件ID。';
COMMENT ON COLUMN project_tasks.cancelled_by IS '取消任务的主体类型或来源。';
COMMENT ON COLUMN project_tasks.failed_by IS '标记任务失败的主体类型或来源。';
COMMENT ON COLUMN project_tasks.status_changed_at IS '任务状态最近一次变化时间。';
COMMENT ON TABLE project_task_attempts IS '项目任务执行尝试表，记录项目任务调度、租约、重试和终态回写。';
COMMENT ON COLUMN project_task_attempts.id IS '执行尝试主键ID。';
COMMENT ON COLUMN project_task_attempts.tenant_id IS '执行尝试所属租户ID。';
COMMENT ON COLUMN project_task_attempts.project_task_id IS '执行尝试所属项目任务ID。';
COMMENT ON COLUMN project_task_attempts.attempt_no IS '尝试序号，从 1 开始递增。';
COMMENT ON COLUMN project_task_attempts.status IS '尝试状态：queued, running, succeeded, failed, cancelled, lost, timed_out, waiting_human。';
COMMENT ON COLUMN project_task_attempts.digital_employee_run_id IS '执行尝试关联的数字员工运行ID。';
COMMENT ON COLUMN project_task_attempts.runtime_task_id IS '执行尝试关联的 Runtime 任务ID。';
COMMENT ON COLUMN project_task_attempts.runtime_node_id IS '执行尝试所在 Runtime 节点ID。';
COMMENT ON COLUMN project_task_attempts.provider_session_id IS '执行尝试关联的 Provider 会话ID。';
COMMENT ON COLUMN project_task_attempts.execution_context_packet IS '执行上下文包 JSON，保存下发给 Runtime 的任务上下文切片。';
COMMENT ON COLUMN project_task_attempts.execution_context_packet_version IS '执行上下文包版本。';
COMMENT ON COLUMN project_task_attempts.lease_token IS 'Runtime 租约令牌。';
COMMENT ON COLUMN project_task_attempts.lease_expires_at IS 'Runtime 租约过期时间。';
COMMENT ON COLUMN project_task_attempts.renewed_at IS 'Runtime 租约最近续约时间。';
COMMENT ON COLUMN project_task_attempts.lost_at IS '执行尝试被判定丢失的时间。';
COMMENT ON COLUMN project_task_attempts.started_at IS '执行尝试开始时间。';
COMMENT ON COLUMN project_task_attempts.finished_at IS '执行尝试结束时间。';
COMMENT ON COLUMN project_task_attempts.timeout_at IS '执行尝试超时时间。';
COMMENT ON COLUMN project_task_attempts.retryable IS '执行失败后是否允许重试。';
COMMENT ON COLUMN project_task_attempts.failure_family IS '执行失败分类。';
COMMENT ON COLUMN project_task_attempts.failure_message IS '执行失败说明。';
COMMENT ON COLUMN project_task_attempts.idempotency_key IS '执行尝试创建幂等键。';
COMMENT ON COLUMN project_task_attempts.created_event_id IS '执行尝试创建时写入的项目事件ID。';
COMMENT ON COLUMN project_task_attempts.terminal_event_id IS '执行尝试进入终态时写入的项目事件ID。';
COMMENT ON COLUMN project_task_attempts.created_at IS '执行尝试创建时间。';
COMMENT ON COLUMN project_task_attempts.updated_at IS '执行尝试最近更新时间。';

CREATE TRIGGER update_project_task_attempts_updated_at
    BEFORE UPDATE ON project_task_attempts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
