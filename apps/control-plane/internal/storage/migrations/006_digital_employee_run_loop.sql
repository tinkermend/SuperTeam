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

COMMENT ON TABLE runtime_command_receipts IS 'Runtime command dispatch and HTTP writeback receipts';
COMMENT ON COLUMN task_runs.command_id IS 'Control Plane generated runtime command ID';
COMMENT ON COLUMN task_runs.digital_employee_id IS 'Digital employee that owns this run';
COMMENT ON COLUMN task_runs.execution_instance_id IS 'Execution instance used by this run';
COMMENT ON COLUMN task_runs.work_products IS 'Structured run outputs indexed for Web and workflow consumption';
COMMENT ON COLUMN provider_sessions.session_state IS 'Adapter-defined recoverable provider session state';
