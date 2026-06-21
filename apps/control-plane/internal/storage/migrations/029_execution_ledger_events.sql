CREATE TABLE execution_ledger_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    team_id UUID,
    project_id UUID NOT NULL,
    project_task_id UUID,
    project_task_attempt_id UUID,
    event_type VARCHAR(100) NOT NULL,
    source_type VARCHAR(100) NOT NULL,
    source_id VARCHAR(255) NOT NULL,
    actor_type VARCHAR(80) NOT NULL,
    actor_id VARCHAR(255),
    runtime_node_id UUID,
    provider_type VARCHAR(100),
    provider_session_id VARCHAR(255),
    input_summary TEXT,
    output_summary TEXT,
    error_family VARCHAR(100),
    error_code VARCHAR(100),
    error_message TEXT,
    retryable BOOLEAN,
    artifact_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    idempotency_key VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uq_execution_ledger_events_idempotency
    ON execution_ledger_events(tenant_id, idempotency_key);

CREATE INDEX idx_execution_ledger_events_project_time
    ON execution_ledger_events(tenant_id, project_id, occurred_at ASC, created_at ASC);

CREATE INDEX idx_execution_ledger_events_attempt_time
    ON execution_ledger_events(tenant_id, project_task_attempt_id, occurred_at ASC, created_at ASC)
    WHERE project_task_attempt_id IS NOT NULL;

CREATE INDEX idx_execution_ledger_events_project_type
    ON execution_ledger_events(tenant_id, project_id, event_type, occurred_at DESC);

CREATE INDEX idx_execution_ledger_events_project_error
    ON execution_ledger_events(tenant_id, project_id, error_family, occurred_at DESC)
    WHERE error_family IS NOT NULL;

CREATE TRIGGER update_execution_ledger_events_updated_at
    BEFORE UPDATE ON execution_ledger_events
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE execution_ledger_events IS '执行账本事件表，记录项目任务执行、Provider、工具、MCP、外部能力和证据链的统一审计索引。';
COMMENT ON COLUMN execution_ledger_events.id IS '执行账本事件主键 UUID。';
COMMENT ON COLUMN execution_ledger_events.tenant_id IS '执行账本事件所属租户 ID。';
COMMENT ON COLUMN execution_ledger_events.team_id IS '执行账本事件所属团队 ID，可为空以兼容项目未绑定团队的历史数据。';
COMMENT ON COLUMN execution_ledger_events.project_id IS '执行账本事件所属项目 ID。';
COMMENT ON COLUMN execution_ledger_events.project_task_id IS '执行账本事件关联项目任务 ID，可为空表示项目级执行事件。';
COMMENT ON COLUMN execution_ledger_events.project_task_attempt_id IS '执行账本事件关联项目任务执行尝试 ID。';
COMMENT ON COLUMN execution_ledger_events.event_type IS '执行事件类型，例如 attempt.started、provider.event、mcp.tool_call、summary.created。';
COMMENT ON COLUMN execution_ledger_events.source_type IS '来源事实类型，例如 project_task_attempt、provider_session_event、runtime_command_receipt。';
COMMENT ON COLUMN execution_ledger_events.source_id IS '来源事实 ID 或稳定外部 ID。';
COMMENT ON COLUMN execution_ledger_events.actor_type IS '执行事件主体类型，例如 digital_employee、runtime_node、provider、system。';
COMMENT ON COLUMN execution_ledger_events.actor_id IS '执行事件主体 ID 或外部标识。';
COMMENT ON COLUMN execution_ledger_events.runtime_node_id IS '关联 Runtime 节点 ID。';
COMMENT ON COLUMN execution_ledger_events.provider_type IS '关联 Provider 类型，由服务端注册表校验。';
COMMENT ON COLUMN execution_ledger_events.provider_session_id IS 'Provider 外部会话 ID。';
COMMENT ON COLUMN execution_ledger_events.input_summary IS '输入摘要，不保存完整 prompt、secret 或大 payload。';
COMMENT ON COLUMN execution_ledger_events.output_summary IS '输出摘要，不保存完整 raw payload。';
COMMENT ON COLUMN execution_ledger_events.error_family IS '错误分类，例如 provider_error、runtime_error、missing_context、capability_denied。';
COMMENT ON COLUMN execution_ledger_events.error_code IS '细分错误码。';
COMMENT ON COLUMN execution_ledger_events.error_message IS '短错误说明，禁止写入 secret。';
COMMENT ON COLUMN execution_ledger_events.retryable IS '该事件对应失败是否可重试。';
COMMENT ON COLUMN execution_ledger_events.artifact_refs IS '工件引用数组。';
COMMENT ON COLUMN execution_ledger_events.evidence_refs IS '证据引用数组。';
COMMENT ON COLUMN execution_ledger_events.metadata IS '结构化扩展数据，禁止写入 secret 和完整 raw payload。';
COMMENT ON COLUMN execution_ledger_events.occurred_at IS '事件发生时间。';
COMMENT ON COLUMN execution_ledger_events.idempotency_key IS '执行账本事件幂等键。';
COMMENT ON COLUMN execution_ledger_events.created_at IS '执行账本事件创建时间。';
COMMENT ON COLUMN execution_ledger_events.updated_at IS '执行账本事件更新时间。';
