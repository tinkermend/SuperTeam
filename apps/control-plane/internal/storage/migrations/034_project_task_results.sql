CREATE UNIQUE INDEX uq_project_execution_summaries_tenant_project_task_id
    ON project_execution_summaries(tenant_id, project_id, project_task_id, id);

CREATE TABLE project_task_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    project_task_id UUID NOT NULL,
    attempt_id UUID,
    execution_summary_id UUID,
    result_status VARCHAR(32) NOT NULL,
    validation_status VARCHAR(32) NOT NULL,
    decision VARCHAR(64) NOT NULL,
    contract_payload JSONB NOT NULL,
    validation_errors JSONB NOT NULL DEFAULT '[]'::jsonb,
    validation_warnings JSONB NOT NULL DEFAULT '[]'::jsonb,
    idempotency_key VARCHAR(255) NOT NULL,
    human_review_request JSONB NOT NULL DEFAULT '{}'::jsonb,
    replan_request JSONB NOT NULL DEFAULT '{}'::jsonb,
    revision_request JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_event_id UUID,
    decision_request_id UUID,
    revision_task_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_project_task_results_task
        FOREIGN KEY (tenant_id, project_id, project_task_id)
        REFERENCES project_tasks(tenant_id, project_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_project_task_results_attempt
        FOREIGN KEY (tenant_id, project_task_id, attempt_id)
        REFERENCES project_task_attempts(tenant_id, project_task_id, id),
    CONSTRAINT fk_project_task_results_execution_summary
        FOREIGN KEY (tenant_id, project_id, project_task_id, execution_summary_id)
        REFERENCES project_execution_summaries(tenant_id, project_id, project_task_id, id),
    CONSTRAINT chk_project_task_results_status CHECK (
        result_status IN ('completed', 'revision_needed', 'blocked', 'failed', 'cancelled')
    ),
    CONSTRAINT chk_project_task_results_validation_status CHECK (
        validation_status IN ('accepted', 'rejected')
    ),
    CONSTRAINT chk_project_task_results_decision CHECK (
        decision IN (
            'validation_failed',
            'complete_accepted',
            'waiting_human_review',
            'revision_attempt',
            'revision_task',
            'blocked_waiting_human',
            'failed_retryable',
            'failed_recovery',
            'cancelled_terminal',
            'replan_requested'
        )
    )
);

CREATE UNIQUE INDEX uq_project_task_results_idempotency
    ON project_task_results(tenant_id, project_task_id, attempt_id, idempotency_key)
    WHERE attempt_id IS NOT NULL;

CREATE UNIQUE INDEX uq_project_task_results_manual_idempotency
    ON project_task_results(tenant_id, project_task_id, idempotency_key)
    WHERE attempt_id IS NULL;

CREATE UNIQUE INDEX uq_project_task_results_idempotency_any_attempt
    ON project_task_results(tenant_id, project_task_id, COALESCE(attempt_id, '00000000-0000-0000-0000-000000000000'::uuid), idempotency_key);

CREATE UNIQUE INDEX uq_project_task_results_tenant_task_id
    ON project_task_results(tenant_id, project_task_id, id);

CREATE UNIQUE INDEX uq_project_task_results_tenant_project_id
    ON project_task_results(tenant_id, project_id, id);

CREATE INDEX idx_project_task_results_tenant_task_created
    ON project_task_results(tenant_id, project_id, project_task_id, created_at DESC);

CREATE INDEX idx_project_task_results_decision
    ON project_task_results(tenant_id, project_id, decision, created_at DESC);

ALTER TABLE project_tasks
    ADD COLUMN revision_of_task_id UUID,
    ADD COLUMN latest_task_result_id UUID;

ALTER TABLE project_tasks
    ADD CONSTRAINT fk_project_tasks_revision_of
    FOREIGN KEY (tenant_id, revision_of_task_id)
    REFERENCES project_tasks(tenant_id, id);

ALTER TABLE project_tasks
    ADD CONSTRAINT fk_project_tasks_latest_task_result
    FOREIGN KEY (tenant_id, id, latest_task_result_id)
    REFERENCES project_task_results(tenant_id, project_task_id, id);

CREATE INDEX idx_project_tasks_revision_of
    ON project_tasks(tenant_id, project_id, revision_of_task_id)
    WHERE revision_of_task_id IS NOT NULL;

CREATE INDEX idx_project_tasks_latest_task_result
    ON project_tasks(tenant_id, latest_task_result_id)
    WHERE latest_task_result_id IS NOT NULL;

ALTER TABLE project_decision_requests
    ADD COLUMN project_task_result_id UUID;

ALTER TABLE project_decision_requests
    ADD CONSTRAINT fk_project_decision_requests_task_result
    FOREIGN KEY (tenant_id, project_id, project_task_result_id)
    REFERENCES project_task_results(tenant_id, project_id, id);

CREATE INDEX idx_project_decision_requests_task_result
    ON project_decision_requests(tenant_id, project_task_result_id)
    WHERE project_task_result_id IS NOT NULL;

CREATE UNIQUE INDEX uq_project_demands_tenant_project_id
    ON project_demands(tenant_id, project_id, id);

CREATE UNIQUE INDEX uq_project_report_refs_tenant_project_id
    ON project_report_refs(tenant_id, project_id, id);

CREATE TABLE project_demand_summaries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    demand_id UUID NOT NULL,
    status VARCHAR(32) NOT NULL,
    conclusion TEXT NOT NULL,
    summary_payload JSONB NOT NULL,
    report_ref_id UUID,
    acceptance_required BOOLEAN NOT NULL DEFAULT true,
    idempotency_key VARCHAR(255) NOT NULL,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_project_demand_summaries_demand
        FOREIGN KEY (tenant_id, project_id, demand_id)
        REFERENCES project_demands(tenant_id, project_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_project_demand_summaries_report
        FOREIGN KEY (tenant_id, project_id, report_ref_id)
        REFERENCES project_report_refs(tenant_id, project_id, id),
    CONSTRAINT chk_project_demand_summaries_status CHECK (
        status IN ('completed', 'blocked', 'failed', 'cancelled')
    )
);

CREATE UNIQUE INDEX uq_project_demand_summaries_idempotency
    ON project_demand_summaries(tenant_id, demand_id, idempotency_key);

CREATE INDEX idx_project_demand_summaries_tenant_demand_created
    ON project_demand_summaries(tenant_id, project_id, demand_id, created_at DESC);

CREATE TRIGGER update_project_task_results_updated_at
    BEFORE UPDATE ON project_task_results
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_project_demand_summaries_updated_at
    BEFORE UPDATE ON project_demand_summaries
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE project_task_results IS 'ProjectTask 结构化结果契约记录，保存 Runtime/Provider 写回后的可审计结果、校验状态和调度决策。';
COMMENT ON COLUMN project_task_results.contract_payload IS 'TaskResultContract JSON，不保存完整日志、密钥、连接串或大段 prompt。';
COMMENT ON COLUMN project_task_results.validation_errors IS '结果契约校验错误数组。';
COMMENT ON COLUMN project_task_results.human_review_request IS '结果需要人类判断时的请求摘要，不作为审批事实源。';
COMMENT ON COLUMN project_task_results.replan_request IS '任务结果触发重规划时的结构化原因和约束。';
COMMENT ON COLUMN project_task_results.revision_request IS '任务结果触发修订时的结构化原因和建议。';
COMMENT ON COLUMN project_tasks.revision_of_task_id IS '该任务是否为另一个 ProjectTask 的 append-only 修订任务。';
COMMENT ON COLUMN project_tasks.latest_task_result_id IS '该任务最新结构化结果记录ID。';
COMMENT ON COLUMN project_decision_requests.project_task_result_id IS '该人类决策由哪个结构化任务结果触发。';
COMMENT ON TABLE project_demand_summaries IS '项目需求最终总结记录，按 demand 生成 append-only 总结和报告引用。';
COMMENT ON COLUMN project_demand_summaries.summary_payload IS '最终需求总结 JSON，包含目标、结论、任务状态、证据、人工决策、验证、风险和下一步建议。';
