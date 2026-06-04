CREATE TABLE runtime_command_receipts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid,
    command_id VARCHAR(255) NOT NULL,
    command_type VARCHAR(100) NOT NULL,
    runtime_node_id UUID NOT NULL,
    node_id VARCHAR(255) NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    resource_id UUID NOT NULL,
    status VARCHAR(50) NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    result JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_message TEXT,
    dispatched_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, command_id)
);

ALTER TABLE task_runs
    ADD COLUMN command_id VARCHAR(255),
    ADD COLUMN digital_employee_id UUID,
    ADD COLUMN execution_instance_id UUID,
    ADD COLUMN idempotency_key VARCHAR(255),
    ADD COLUMN idempotency_fingerprint VARCHAR(255),
    ADD COLUMN timeout_sec INTEGER,
    ADD COLUMN grace_sec INTEGER,
    ADD COLUMN diagnostic JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN log_ref TEXT,
    ADD COLUMN raw_result_ref TEXT,
    ADD COLUMN work_products JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN session_state JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN error_code VARCHAR(100),
    ADD COLUMN error_family VARCHAR(100),
    ADD COLUMN exit_code INTEGER,
    ADD COLUMN signal VARCHAR(100),
    ADD COLUMN timed_out BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN provider_session_external_id VARCHAR(255);

ALTER TABLE task_events
    ADD COLUMN command_id VARCHAR(255),
    ADD COLUMN raw_event_ref TEXT,
    ADD COLUMN log_ref TEXT,
    ADD COLUMN metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE provider_sessions
    ADD COLUMN session_display_id VARCHAR(255),
    ADD COLUMN session_params JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN session_state JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN last_sequence_number INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN last_command_id VARCHAR(255),
    ADD COLUMN last_run_id UUID,
    ADD COLUMN last_error_family VARCHAR(100),
    ADD COLUMN last_runtime_seen_at TIMESTAMPTZ;

ALTER TABLE provider_session_events
    ADD COLUMN log_ref TEXT,
    ADD COLUMN session_state_patch JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE UNIQUE INDEX uq_task_runs_command_id
    ON task_runs(tenant_id, command_id)
    WHERE command_id IS NOT NULL;

CREATE UNIQUE INDEX uq_task_runs_employee_idempotency
    ON task_runs(tenant_id, digital_employee_id, idempotency_key)
    WHERE digital_employee_id IS NOT NULL AND idempotency_key IS NOT NULL;

CREATE INDEX idx_task_runs_employee_status
    ON task_runs(tenant_id, digital_employee_id, status, created_at DESC)
    WHERE digital_employee_id IS NOT NULL;

DROP INDEX IF EXISTS uq_task_events_task_sequence;
DROP INDEX IF EXISTS uq_task_events_run_sequence;
CREATE UNIQUE INDEX uq_task_events_run_sequence
    ON task_events(tenant_id, run_id, sequence_number)
    WHERE run_id IS NOT NULL;

CREATE UNIQUE INDEX uq_provider_session_events_command_sequence
    ON provider_session_events(tenant_id, command_id, sequence_number)
    WHERE command_id IS NOT NULL;

CREATE INDEX idx_runtime_command_receipts_resource
    ON runtime_command_receipts(tenant_id, resource_type, resource_id, created_at DESC);

CREATE TRIGGER update_runtime_command_receipts_updated_at BEFORE UPDATE ON runtime_command_receipts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE runtime_command_receipts IS 'Runtime 命令回执表，记录下发、回写和终态结果';
COMMENT ON COLUMN runtime_command_receipts.id IS '命令回执ID';
COMMENT ON COLUMN runtime_command_receipts.tenant_id IS '租户ID';
COMMENT ON COLUMN runtime_command_receipts.command_id IS '控制平面生成的命令ID';
COMMENT ON COLUMN runtime_command_receipts.command_type IS '命令类型';
COMMENT ON COLUMN runtime_command_receipts.runtime_node_id IS '目标Runtime节点UUID';
COMMENT ON COLUMN runtime_command_receipts.node_id IS '目标Runtime业务节点ID';
COMMENT ON COLUMN runtime_command_receipts.resource_type IS '命令关联资源类型';
COMMENT ON COLUMN runtime_command_receipts.resource_id IS '命令关联资源ID';
COMMENT ON COLUMN runtime_command_receipts.status IS '命令状态';
COMMENT ON COLUMN runtime_command_receipts.payload IS '命令下发负载';
COMMENT ON COLUMN runtime_command_receipts.result IS '命令回写结果';
COMMENT ON COLUMN runtime_command_receipts.error_message IS '命令错误信息';
COMMENT ON COLUMN runtime_command_receipts.dispatched_at IS '命令下发时间';
COMMENT ON COLUMN runtime_command_receipts.completed_at IS '命令完成时间';
COMMENT ON COLUMN runtime_command_receipts.created_at IS '创建时间';
COMMENT ON COLUMN runtime_command_receipts.updated_at IS '更新时间';

COMMENT ON COLUMN task_runs.command_id IS '运行关联的Runtime命令ID';
COMMENT ON COLUMN task_runs.digital_employee_id IS '运行所属数字员工ID';
COMMENT ON COLUMN task_runs.execution_instance_id IS '运行使用的执行实例ID';
COMMENT ON COLUMN task_runs.idempotency_key IS '运行创建幂等键';
COMMENT ON COLUMN task_runs.idempotency_fingerprint IS '运行创建幂等指纹';
COMMENT ON COLUMN task_runs.timeout_sec IS '运行超时时间秒数';
COMMENT ON COLUMN task_runs.grace_sec IS '停止宽限时间秒数';
COMMENT ON COLUMN task_runs.diagnostic IS '运行诊断信息';
COMMENT ON COLUMN task_runs.log_ref IS '运行日志对象引用';
COMMENT ON COLUMN task_runs.raw_result_ref IS '原始结果对象引用';
COMMENT ON COLUMN task_runs.work_products IS '结构化工作产物列表';
COMMENT ON COLUMN task_runs.session_state IS 'Provider会话状态快照';
COMMENT ON COLUMN task_runs.error_code IS '运行错误码';
COMMENT ON COLUMN task_runs.error_family IS '运行错误分类';
COMMENT ON COLUMN task_runs.exit_code IS 'Provider进程退出码';
COMMENT ON COLUMN task_runs.signal IS 'Provider进程退出信号';
COMMENT ON COLUMN task_runs.timed_out IS '运行是否因超时终止';
COMMENT ON COLUMN task_runs.provider_session_external_id IS 'Provider外部会话ID';

COMMENT ON COLUMN task_events.command_id IS '事件关联的Runtime命令ID';
COMMENT ON COLUMN task_events.raw_event_ref IS '原始事件对象引用';
COMMENT ON COLUMN task_events.log_ref IS '事件日志对象引用';
COMMENT ON COLUMN task_events.metadata IS '任务事件扩展元数据';

COMMENT ON COLUMN provider_sessions.session_display_id IS 'Provider会话展示ID';
COMMENT ON COLUMN provider_sessions.session_params IS 'Provider会话启动参数';
COMMENT ON COLUMN provider_sessions.session_state IS 'Provider适配器可恢复的会话状态';
COMMENT ON COLUMN provider_sessions.last_sequence_number IS '最后处理的Provider事件序号';
COMMENT ON COLUMN provider_sessions.last_command_id IS '最后关联的Runtime命令ID';
COMMENT ON COLUMN provider_sessions.last_run_id IS '最后关联的任务运行ID';
COMMENT ON COLUMN provider_sessions.last_error_family IS '最后一次错误分类';
COMMENT ON COLUMN provider_sessions.last_runtime_seen_at IS 'Runtime最后回写会话时间';

COMMENT ON COLUMN provider_session_events.log_ref IS 'Provider事件日志对象引用';
COMMENT ON COLUMN provider_session_events.session_state_patch IS '事件携带的会话状态增量';
